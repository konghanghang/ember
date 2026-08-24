import asyncio
import logging
import os
import sys
import socket
import time
from contextlib import asynccontextmanager, suppress
from hmac import compare_digest
from logging.handlers import TimedRotatingFileHandler
from pathlib import Path
from uuid import uuid4

import uvicorn
from fastapi import FastAPI, Request, Response
from fastapi.responses import JSONResponse
from telegram import (
    BotCommand,
    BotCommandScopeChat,
    BotCommandScopeChatAdministrators,
    BotCommandScopeAllGroupChats,
    BotCommandScopeAllPrivateChats,
    BotCommandScopeDefault,
    Update,
)
from telegram.ext import (
    Application,
    CallbackQueryHandler,
    CommandHandler,
    ContextTypes,
    MessageHandler,
    TypeHandler,
    filters,
)

from app.clients import api_client
from app.config import (
    BOT_PORT,
    INTERNAL_API_SECRET,
    TELEGRAM_ADMIN_CHAT_ID,
    TELEGRAM_BOT_TOKEN,
    TELEGRAM_GROUP_CHAT_ID,
    TELEGRAM_UPDATE_MODE,
    TELEGRAM_UPDATE_MODE_POLLING,
    TELEGRAM_UPDATE_MODE_WEBHOOK,
    TELEGRAM_WEBHOOK_SECRET,
    WEBHOOK_URL,
)
from app.handlers.telegram_handler import (
    handle_bind,
    handle_callback,
    handle_count,
    handle_info,
    handle_libraries,
    handle_libraries_callback,
    handle_new_member,
    handle_pending_reject_reason,
    handle_refresh_menu,
    handle_refresh_menu_chat,
    handle_redeem,
    handle_resetpw,
    handle_search,
    handle_search_callback,
    send_payment_notification,
    send_registration_notification,
    send_auto_approved_subscription_notification,
    send_ranking_notification,
    send_subscription_notification,
    send_subscription_result_notification,
    sync_subscription_admin_messages,
)
from app.menu_sync import schedule_group_menu_sync
from app.runtime_settings import runtime_settings_service

LOG_DIR = Path("logs")
LOG_FILE = LOG_DIR / "bot.log"


def resolve_log_level(raw: str | None) -> tuple[int, str, bool]:
    """Resolve the shared info/debug contract without failing service startup."""
    normalized = (raw or "").strip().lower()
    if normalized in ("", "info"):
        return logging.INFO, "info", False
    if normalized == "debug":
        return logging.DEBUG, "debug", False
    return logging.INFO, "info", True


def configure_third_party_loggers() -> None:
    """Keep third-party HTTP namespaces above the project Debug boundary."""
    # Telegram/httpx debug output may contain complete Bot API URLs or wire
    # context, so project Debug never lowers these third-party namespaces.
    for name in ("httpx", "httpcore", "telegram"):
        logging.getLogger(name).setLevel(logging.WARNING)


def request_log_level(debug_enabled: bool, status_code: int) -> int | None:
    """Select a safe Bot request-summary level for the completed response."""
    if status_code >= 400:
        return logging.INFO
    if debug_enabled:
        return logging.DEBUG
    return None


def configure_logging() -> bool:
    """Configure Bot stdout/file logging and return whether Debug is active."""
    LOG_DIR.mkdir(parents=True, exist_ok=True)

    level, level_name, invalid = resolve_log_level(os.getenv("LOG_LEVEL"))

    formatter = logging.Formatter("%(asctime)s %(levelname)s %(name)s: %(message)s")
    stream_handler = logging.StreamHandler(sys.stdout)
    stream_handler.setFormatter(formatter)

    file_handler = TimedRotatingFileHandler(
        LOG_FILE,
        when="midnight",
        backupCount=14,
        encoding="utf-8",
    )
    file_handler.setFormatter(formatter)

    logging.basicConfig(
        level=level,
        handlers=[stream_handler, file_handler],
        force=True,
    )
    logging.getLogger("uvicorn.access").disabled = True
    configure_third_party_loggers()
    bootstrap_logger = logging.getLogger(__name__)
    if invalid:
        bootstrap_logger.warning("code=log_level_invalid fallbackLevel=info")
    bootstrap_logger.info("code=logging_initialized processRole=bot logLevel=%s", level_name)
    return level == logging.DEBUG


BOT_DEBUG_LOGGING = configure_logging()
logger = logging.getLogger(__name__)

POLLING_LOCK_LEASE_SECONDS = 90
POLLING_LOCK_RENEW_INTERVAL_SECONDS = 30
WEBHOOK_REGISTER_MAX_ATTEMPTS = 6

_webhook_registration_state = {
    "mode": TELEGRAM_UPDATE_MODE,
    "registered": TELEGRAM_UPDATE_MODE != TELEGRAM_UPDATE_MODE_WEBHOOK,
    "attempts": 0,
    "lastError": "",
}

tg_app = Application.builder().token(TELEGRAM_BOT_TOKEN).build()


async def handle_update_lifecycle(update: Update, context: ContextTypes.DEFAULT_TYPE) -> None:
    await schedule_group_menu_sync(context.bot, update)


tg_app.add_handler(TypeHandler(Update, handle_update_lifecycle), group=-1)
tg_app.add_handler(CallbackQueryHandler(handle_callback, pattern=r"^(approve|reject):"))
tg_app.add_handler(CallbackQueryHandler(handle_search_callback, pattern=r"^sub:"))
tg_app.add_handler(CallbackQueryHandler(handle_libraries_callback, pattern=r"^lib:"))
tg_app.add_handler(MessageHandler(filters.StatusUpdate.NEW_CHAT_MEMBERS, handle_new_member))
tg_app.add_handler(MessageHandler(filters.TEXT & ~filters.COMMAND, handle_pending_reject_reason))
tg_app.add_handler(CommandHandler("bind", handle_bind))
tg_app.add_handler(CommandHandler("count", handle_count))
tg_app.add_handler(CommandHandler("info", handle_info))
tg_app.add_handler(CommandHandler("libraries", handle_libraries))
tg_app.add_handler(CommandHandler("redeem", handle_redeem))
tg_app.add_handler(CommandHandler("resetpw", handle_resetpw))
tg_app.add_handler(CommandHandler("search", handle_search))
tg_app.add_handler(CommandHandler("refresh_menu", handle_refresh_menu))
tg_app.add_handler(CommandHandler("refresh_menu_chat", handle_refresh_menu_chat))


async def register_webhook_with_retry(stop_event: asyncio.Event) -> bool:
    webhook_url = f"{WEBHOOK_URL}/telegram/webhook"
    retry_delay = 2
    attempt = 1
    _webhook_registration_state["mode"] = TELEGRAM_UPDATE_MODE
    _webhook_registration_state["registered"] = False
    _webhook_registration_state["attempts"] = 0
    _webhook_registration_state["lastError"] = ""

    while not stop_event.is_set():
        try:
            _webhook_registration_state["attempts"] = attempt
            await tg_app.bot.set_webhook(
                url=webhook_url,
                secret_token=TELEGRAM_WEBHOOK_SECRET,
            )
            _webhook_registration_state["registered"] = True
            _webhook_registration_state["lastError"] = ""
            logger.info("Telegram webhook 已注册: %s", webhook_url)
            return True
        except Exception as err:
            _webhook_registration_state["registered"] = False
            _webhook_registration_state["lastError"] = str(err)
            logger.warning(
                "Telegram webhook 注册失败，%s 秒后重试 attempt=%d/%d error=%s",
                retry_delay,
                attempt,
                WEBHOOK_REGISTER_MAX_ATTEMPTS,
                err,
            )
            if attempt >= WEBHOOK_REGISTER_MAX_ATTEMPTS:
                logger.error(
                    "Telegram webhook 注册达到最大重试次数，停止继续重试 webhookUrl=%s attempts=%d",
                    webhook_url,
                    attempt,
                )
                return False
            try:
                await asyncio.wait_for(stop_event.wait(), timeout=retry_delay)
            except asyncio.TimeoutError:
                retry_delay = min(retry_delay * 2, 60)
                attempt += 1
    return False


def build_polling_owner_id() -> str:
    return f"{socket.gethostname()}:{os.getpid()}:{uuid4().hex[:12]}"


async def renew_polling_lock_loop(owner_id: str, stop_event: asyncio.Event) -> None:
    while not stop_event.is_set():
        try:
            await asyncio.wait_for(stop_event.wait(), timeout=POLLING_LOCK_RENEW_INTERVAL_SECONDS)
            return
        except asyncio.TimeoutError:
            pass

        renewed = await api_client.renew_polling_lock(owner_id, POLLING_LOCK_LEASE_SECONDS)
        if renewed:
            continue

        logger.critical("Bot polling 锁续租失败，当前实例将停止 polling ownerId=%s", owner_id)
        if tg_app.updater is not None:
            await tg_app.updater.stop()
        stop_event.set()
        return


async def start_polling_updates(stop_event: asyncio.Event) -> tuple[str, asyncio.Task[None]]:
    if tg_app.updater is None:
        raise RuntimeError("Telegram updater is not available in polling mode")

    owner_id = build_polling_owner_id()
    acquired = await api_client.acquire_polling_lock(owner_id, POLLING_LOCK_LEASE_SECONDS)
    if not acquired:
        raise RuntimeError("Polling 模式已有其他 Bot 实例持锁，当前实例拒绝启动")

    logger.info("Telegram 更新模式为 polling，开始清理旧 webhook")
    await tg_app.bot.delete_webhook(drop_pending_updates=False)
    await tg_app.updater.start_polling(drop_pending_updates=False)
    renew_task = asyncio.create_task(renew_polling_lock_loop(owner_id, stop_event))
    logger.info("Telegram polling 已启动 ownerId=%s", owner_id)
    return owner_id, renew_task


async def resolve_command_scope_chat_ids() -> tuple[int | None, int | None]:
    try:
        settings = await runtime_settings_service.get(force_refresh=True)
        return settings.admin_chat_id, settings.group_chat_id
    except Exception as err:
        logger.warning("读取运行期 Telegram Chat ID 失败，回退到环境变量: %s", err)
        return TELEGRAM_ADMIN_CHAT_ID, TELEGRAM_GROUP_CHAT_ID


async def sync_bot_commands() -> None:
    commands = [
        BotCommand("search", "搜索影视"),
        BotCommand("bind", "绑定 Ember 账号"),
        BotCommand("count", "查看媒体库统计"),
        BotCommand("info", "查看账号信息"),
        BotCommand("libraries", "管理媒体库显示"),
        BotCommand("redeem", "兑换续期码"),
        BotCommand("resetpw", "重置密码"),
    ]
    admin_chat_id, group_chat_id = await resolve_command_scope_chat_ids()

    with suppress(Exception):
        await tg_app.bot.delete_my_commands(scope=BotCommandScopeDefault())
    await tg_app.bot.set_my_commands(
        commands,
        scope=BotCommandScopeAllPrivateChats(),
    )

    scopes_to_clear = [
        BotCommandScopeAllGroupChats(),
    ]
    if admin_chat_id is not None:
        scopes_to_clear.append(BotCommandScopeChat(chat_id=admin_chat_id))
    if group_chat_id is not None:
        scopes_to_clear.append(BotCommandScopeChat(chat_id=group_chat_id))
        scopes_to_clear.append(BotCommandScopeChatAdministrators(chat_id=group_chat_id))

    cleared_scopes: list[str] = []
    for scope in scopes_to_clear:
        try:
            await tg_app.bot.delete_my_commands(scope=scope)
            cleared_scopes.append(type(scope).__name__)
        except Exception as err:
            logger.warning("清理旧命令作用域失败 scope=%s: %s", type(scope).__name__, err)

    logger.info(
        "Bot 命令菜单已同步: cleared=%s private_count=%d admin_chat_id=%s group_chat_id=%s",
        cleared_scopes,
        len(commands),
        admin_chat_id,
        group_chat_id,
    )


def _verify_internal_secret(request: Request) -> JSONResponse | None:
    secret = request.headers.get("X-Internal-Secret", "")
    if compare_digest(secret, INTERNAL_API_SECRET):
        return None
    return JSONResponse(status_code=401, content={"error": "unauthorized"})


@asynccontextmanager
async def lifespan(app: FastAPI):
    del app
    initialized = False
    started = False
    polling_started = False
    polling_lock_owner_id: str | None = None
    polling_lock_stop_event: asyncio.Event | None = None
    polling_lock_task: asyncio.Task | None = None
    stop_event: asyncio.Event | None = None
    webhook_task: asyncio.Task | None = None

    await api_client.init()
    try:
        await tg_app.initialize()
        initialized = True
        try:
            await sync_bot_commands()
        except Exception as err:
            logger.warning("Bot 命令菜单同步失败，不影响服务运行: %s", err)
        await tg_app.start()
        started = True
        if TELEGRAM_UPDATE_MODE == TELEGRAM_UPDATE_MODE_POLLING:
            polling_lock_stop_event = asyncio.Event()
            polling_lock_owner_id, polling_lock_task = await start_polling_updates(polling_lock_stop_event)
            polling_started = True

        if TELEGRAM_UPDATE_MODE == TELEGRAM_UPDATE_MODE_WEBHOOK:
            logger.info("Telegram Bot 服务已启动，更新模式=webhook，开始异步注册 webhook")
            stop_event = asyncio.Event()
            webhook_task = asyncio.create_task(register_webhook_with_retry(stop_event))
        else:
            _webhook_registration_state["registered"] = True
            _webhook_registration_state["attempts"] = 0
            _webhook_registration_state["lastError"] = ""
            logger.info("Telegram Bot 服务已启动，更新模式=polling，内部通知入口保持可用")
        yield
    finally:
        if polling_lock_stop_event is not None:
            polling_lock_stop_event.set()
        if polling_lock_task is not None:
            polling_lock_task.cancel()
            with suppress(asyncio.CancelledError):
                await polling_lock_task
        if stop_event is not None:
            stop_event.set()
        if webhook_task is not None:
            webhook_task.cancel()
            with suppress(asyncio.CancelledError):
                await webhook_task
        if polling_started and tg_app.updater is not None:
            await tg_app.updater.stop()
        if polling_lock_owner_id is not None:
            await api_client.release_polling_lock(polling_lock_owner_id)
        if started:
            await tg_app.stop()
        if initialized:
            await tg_app.shutdown()
        await api_client.close()


app = FastAPI(lifespan=lifespan)


@app.middleware("http")
async def log_http_request(request: Request, call_next):
    """Log only a route template and bounded result metadata, never raw query."""
    started_at = time.monotonic()
    response = await call_next(request)
    if request.url.path == "/health":
        return response
    level = request_log_level(BOT_DEBUG_LOGGING, response.status_code)
    if level is None:
        return response
    route = request.scope.get("route")
    route_path = getattr(route, "path", "unmatched") or "unmatched"
    route_path = route_path[:1024]
    duration_ms = max(0, int((time.monotonic() - started_at) * 1000))
    code = "request_failed" if response.status_code >= 400 else "request_completed"
    logger.log(
        level,
        "code=%s method=%s path=%s statusCode=%d durationMs=%d",
        code,
        request.method[:32],
        route_path,
        response.status_code,
        duration_ms,
    )
    return response


@app.get("/health")
async def health():
    if TELEGRAM_UPDATE_MODE == TELEGRAM_UPDATE_MODE_WEBHOOK and not _webhook_registration_state["registered"]:
        return {
            "status": "degraded",
            "telegramUpdateMode": TELEGRAM_UPDATE_MODE,
            "webhookRegistered": False,
            "webhookRegisterAttempts": _webhook_registration_state["attempts"],
            "lastWebhookRegisterError": _webhook_registration_state["lastError"],
        }
    return {
        "status": "ok",
        "telegramUpdateMode": TELEGRAM_UPDATE_MODE,
        "webhookRegistered": bool(_webhook_registration_state["registered"]),
    }


@app.post("/telegram/webhook")
async def telegram_webhook(request: Request):
    if TELEGRAM_UPDATE_MODE != TELEGRAM_UPDATE_MODE_WEBHOOK:
        logger.warning("收到 Telegram webhook 请求，但当前更新模式为 %s", TELEGRAM_UPDATE_MODE)
        return JSONResponse(
            status_code=409,
            content={"error": f"telegram update mode is {TELEGRAM_UPDATE_MODE}"},
        )

    secret = request.headers.get("X-Telegram-Bot-Api-Secret-Token", "")
    if not compare_digest(secret, TELEGRAM_WEBHOOK_SECRET):
        return JSONResponse(status_code=401, content={"error": "unauthorized"})

    data = await request.json()
    update = Update.de_json(data, tg_app.bot)
    await tg_app.process_update(update)
    return Response(status_code=200)


@app.post("/notify/subscription")
async def notify_subscription(request: Request):
    unauthorized = _verify_internal_secret(request)
    if unauthorized is not None:
        return unauthorized

    data = await request.json()
    deliveries = await send_subscription_notification(tg_app.bot, data)
    return {"ok": True, "deliveries": deliveries}


@app.post("/notify/subscription-auto-approved")
async def notify_subscription_auto_approved(request: Request):
    unauthorized = _verify_internal_secret(request)
    if unauthorized is not None:
        return unauthorized

    data = await request.json()
    await send_auto_approved_subscription_notification(tg_app.bot, data)
    return {"ok": True}


@app.post("/notify/subscription-admin-sync")
async def notify_subscription_admin_sync(request: Request):
    unauthorized = _verify_internal_secret(request)
    if unauthorized is not None:
        return unauthorized

    data = await request.json()
    results = await sync_subscription_admin_messages(tg_app.bot, data)
    return {"ok": True, "results": results}


@app.post("/notify/subscription-result")
async def notify_subscription_result(request: Request):
    unauthorized = _verify_internal_secret(request)
    if unauthorized is not None:
        return unauthorized

    data = await request.json()
    await send_subscription_result_notification(tg_app.bot, data)
    return {"ok": True}


@app.post("/notify/registration")
async def notify_registration(request: Request):
    unauthorized = _verify_internal_secret(request)
    if unauthorized is not None:
        return unauthorized

    data = await request.json()
    await send_registration_notification(tg_app.bot, data)
    return {"ok": True}


@app.post("/notify/payment")
async def notify_payment(request: Request):
    unauthorized = _verify_internal_secret(request)
    if unauthorized is not None:
        return unauthorized

    data = await request.json()
    await send_payment_notification(tg_app.bot, data)
    return {"ok": True}


@app.post("/notify/ranking")
async def notify_ranking(request: Request):
    unauthorized = _verify_internal_secret(request)
    if unauthorized is not None:
        return unauthorized

    data = await request.json()
    await send_ranking_notification(tg_app.bot, data)
    return {"ok": True}


def run() -> None:
    uvicorn.run(app, host="0.0.0.0", port=BOT_PORT, log_config=None, access_log=False)


if __name__ == "__main__":
    run()
