import logging
from hmac import compare_digest
from contextlib import asynccontextmanager

import uvicorn
from fastapi import FastAPI, Request, Response
from fastapi.responses import JSONResponse
from telegram import Update
from telegram.ext import Application, CallbackQueryHandler

from app.config import (
    BOT_PORT,
    INTERNAL_API_SECRET,
    TELEGRAM_BOT_TOKEN,
    TELEGRAM_WEBHOOK_SECRET,
    WEBHOOK_URL,
)
from app.handlers.telegram_handler import handle_callback, send_subscription_notification

logging.basicConfig(
    level=logging.INFO,
    format="%(asctime)s %(levelname)s %(name)s: %(message)s",
)
logger = logging.getLogger(__name__)

tg_app = Application.builder().token(TELEGRAM_BOT_TOKEN).build()
tg_app.add_handler(CallbackQueryHandler(handle_callback))


@asynccontextmanager
async def lifespan(app: FastAPI):
    await tg_app.initialize()
    await tg_app.bot.set_webhook(
        url=f"{WEBHOOK_URL}/telegram/webhook",
        secret_token=TELEGRAM_WEBHOOK_SECRET,
    )
    await tg_app.start()
    logger.info("Telegram webhook 已注册")

    try:
        yield
    finally:
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


def run() -> None:
    uvicorn.run(app, host="0.0.0.0", port=BOT_PORT)


if __name__ == "__main__":
    run()
