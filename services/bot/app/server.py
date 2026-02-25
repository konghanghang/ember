import asyncio
import logging
from contextlib import asynccontextmanager, suppress
from hmac import compare_digest

import uvicorn
from fastapi import FastAPI, Request, Response
from fastapi.responses import JSONResponse
from telegram import Update
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
    TELEGRAM_BOT_TOKEN,
    TELEGRAM_WEBHOOK_SECRET,
    WEBHOOK_URL,
)
from app.handlers.telegram_handler import (
    handle_bind,
    handle_callback,
    handle_info,
    handle_new_member,
    handle_redeem,
    send_registration_notification,
    send_ranking_notification,
    send_subscription_notification,
)

logging.basicConfig(
    level=logging.INFO,
    format="%(asctime)s %(levelname)s %(name)s: %(message)s",
)
logger = logging.getLogger(__name__)

tg_app = Application.builder().token(TELEGRAM_BOT_TOKEN).build()
tg_app.add_handler(CallbackQueryHandler(handle_callback))
tg_app.add_handler(MessageHandler(filters.StatusUpdate.NEW_CHAT_MEMBERS, handle_new_member))
tg_app.add_handler(CommandHandler("bind", handle_bind))
tg_app.add_handler(CommandHandler("info", handle_info))
tg_app.add_handler(CommandHandler("redeem", handle_redeem))


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


@asynccontextmanager
async def lifespan(app: FastAPI):
    del app
    await tg_app.initialize()
    await tg_app.start()
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


@app.post("/notify/ranking")
async def notify_ranking(request: Request):
    secret = request.headers.get("X-Internal-Secret")
    if secret != INTERNAL_API_SECRET:
        return JSONResponse(status_code=401, content={"error": "unauthorized"})

    data = await request.json()
    await send_ranking_notification(tg_app.bot, data)
    return {"ok": True}


def run() -> None:
    uvicorn.run(app, host="0.0.0.0", port=BOT_PORT)


if __name__ == "__main__":
    run()
