import asyncio
import logging
import sys
from contextlib import asynccontextmanager, suppress
from hmac import compare_digest
from logging.handlers import TimedRotatingFileHandler
from pathlib import Path

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
    MessageHandler,
    filters,
)

from app.config import (
    BOT_PORT,
    INTERNAL_API_SECRET,
    TELEGRAM_ADMIN_CHAT_ID,
    TELEGRAM_BOT_TOKEN,
    TELEGRAM_GROUP_CHAT_ID,
    TELEGRAM_WEBHOOK_SECRET,
    WEBHOOK_URL,
)
from app.handlers.telegram_handler import (
    handle_bind,
    handle_cancel_note,
    handle_callback,
    handle_info,
    handle_new_member,
    handle_refresh_menu,
    handle_redeem,
    handle_resetpw,
    handle_search,
    handle_search_callback,
    handle_text_message,
    send_payment_notification,
    send_registration_notification,
    send_ranking_notification,
    send_subscription_notification,
)
from app.menu_sync import schedule_group_menu_sync
from app.runtime_settings import runtime_settings_service

LOG_DIR = Path("logs")
LOG_FILE = LOG_DIR / "bot.log"


class SkipHealthAccessFilter(logging.Filter):
    def filter(self, record: logging.LogRecord) -> bool:
        message = record.getMessage()
        return "GET /health" not in message


def configure_logging() -> None:
    LOG_DIR.mkdir(parents=True, exist_ok=True)

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
        level=logging.INFO,
        handlers=[stream_handler, file_handler],
        force=True,
    )
    logging.getLogger("uvicorn.access").addFilter(SkipHealthAccessFilter())


configure_logging()
logger = logging.getLogger(__name__)

tg_app = Application.builder().token(TELEGRAM_BOT_TOKEN).build()
tg_app.add_handler(CallbackQueryHandler(handle_callback, pattern=r"^(approve|reject):"))
tg_app.add_handler(CallbackQueryHandler(handle_search_callback, pattern=r"^sub:"))
tg_app.add_handler(MessageHandler(filters.StatusUpdate.NEW_CHAT_MEMBERS, handle_new_member))
tg_app.add_handler(CommandHandler("bind", handle_bind))
tg_app.add_handler(CommandHandler("info", handle_info))
tg_app.add_handler(CommandHandler("redeem", handle_redeem))
tg_app.add_handler(CommandHandler("resetpw", handle_resetpw))
tg_app.add_handler(CommandHandler("search", handle_search))
tg_app.add_handler(CommandHandler("cancel", handle_cancel_note))
tg_app.add_handler(CommandHandler("refresh_menu", handle_refresh_menu))
tg_app.add_handler(MessageHandler(
    filters.TEXT & ~filters.COMMAND & filters.ChatType.PRIVATE,
    handle_text_message,
))


async def register_webhook_with_retry(stop_event: asyncio.Event) -> None:
    webhook_url = f"{WEBHOOK_URL}/telegram/webhook"
    retry_delay = 2

    while not stop_event.is_set():
        try:
            await tg_app.bot.set_webhook(
                url=webhook_url,
                secret_token=TELEGRAM_WEBHOOK_SECRET,
            )
            logger.info("Telegram webhook 已注册: %s", webhook_url)
            return
        except Exception as err:
            logger.warning(
                "Telegram webhook 注册失败，%s 秒后重试: %s",
                retry_delay,
                err,
            )
            try:
                await asyncio.wait_for(stop_event.wait(), timeout=retry_delay)
            except asyncio.TimeoutError:
                retry_delay = min(retry_delay * 2, 60)


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
        BotCommand("info", "查看账号信息"),
        BotCommand("redeem", "兑换续期码"),
        BotCommand("resetpw", "重置密码"),
        BotCommand("cancel", "取消备注输入"),
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


@asynccontextmanager
async def lifespan(app: FastAPI):
    del app
    await tg_app.initialize()
    await tg_app.start()
    try:
        await sync_bot_commands()
    except Exception as err:
        logger.warning("Bot 命令菜单同步失败，不影响服务运行: %s", err)
    logger.info("Telegram Bot 服务已启动，开始异步注册 webhook")

    stop_event = asyncio.Event()
    webhook_task = asyncio.create_task(register_webhook_with_retry(stop_event))

    try:
        yield
    finally:
        stop_event.set()
        webhook_task.cancel()
        with suppress(asyncio.CancelledError):
            await webhook_task
        await tg_app.stop()
        await tg_app.shutdown()


app = FastAPI(lifespan=lifespan)


@app.get("/health")
async def health():
    return {"status": "ok"}


@app.post("/telegram/webhook")
async def telegram_webhook(request: Request):
    secret = request.headers.get("X-Telegram-Bot-Api-Secret-Token", "")
    if not compare_digest(secret, TELEGRAM_WEBHOOK_SECRET):
        return JSONResponse(status_code=401, content={"error": "unauthorized"})

    data = await request.json()
    update = Update.de_json(data, tg_app.bot)
    await schedule_group_menu_sync(tg_app.bot, update)
    await tg_app.process_update(update)
    return Response(status_code=200)


@app.post("/notify/subscription")
async def notify_subscription(request: Request):
    secret = request.headers.get("X-Internal-Secret")
    if secret != INTERNAL_API_SECRET:
        return JSONResponse(status_code=401, content={"error": "unauthorized"})

    data = await request.json()
    await send_subscription_notification(tg_app.bot, data)
    return {"ok": True}


@app.post("/notify/registration")
async def notify_registration(request: Request):
    secret = request.headers.get("X-Internal-Secret")
    if secret != INTERNAL_API_SECRET:
        return JSONResponse(status_code=401, content={"error": "unauthorized"})

    data = await request.json()
    await send_registration_notification(tg_app.bot, data)
    return {"ok": True}


@app.post("/notify/payment")
async def notify_payment(request: Request):
    secret = request.headers.get("X-Internal-Secret")
    if secret != INTERNAL_API_SECRET:
        return JSONResponse(status_code=401, content={"error": "unauthorized"})

    data = await request.json()
    await send_payment_notification(tg_app.bot, data)
    return {"ok": True}


@app.post("/notify/ranking")
async def notify_ranking(request: Request):
    secret = request.headers.get("X-Internal-Secret")
    if secret != INTERNAL_API_SECRET:
        return JSONResponse(status_code=401, content={"error": "unauthorized"})

    data = await request.json()
    await send_ranking_notification(tg_app.bot, data)
    return {"ok": True}


def run() -> None:
    uvicorn.run(app, host="0.0.0.0", port=BOT_PORT, log_config=None)


if __name__ == "__main__":
    run()
