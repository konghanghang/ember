import logging

from telegram import Update
from telegram.ext import ContextTypes

from app.clients import api_client
from app.config import TELEGRAM_ADMIN_CHAT_ID, TMDB_IMAGE_BASE
from app.formatters.message_formatter import (
    format_result_message,
    format_subscription_message,
)

logger = logging.getLogger(__name__)


async def send_subscription_notification(bot, data: dict) -> None:
    text, keyboard = format_subscription_message(data)
    poster_path = data.get("posterPath")

    if poster_path:
        poster_url = f"{TMDB_IMAGE_BASE}{poster_path}"
        try:
            await bot.send_photo(
                chat_id=TELEGRAM_ADMIN_CHAT_ID,
                photo=poster_url,
                caption=text,
                parse_mode="HTML",
                reply_markup=keyboard,
            )
            return
        except Exception:
            logger.exception("发送海报消息失败，降级为文本消息")

    await bot.send_message(
        chat_id=TELEGRAM_ADMIN_CHAT_ID,
        text=text,
        parse_mode="HTML",
        reply_markup=keyboard,
    )


async def handle_callback(update: Update, context: ContextTypes.DEFAULT_TYPE) -> None:
    query = update.callback_query
    if query is None or query.data is None:
        return

    await query.answer()

    if query.from_user is None or query.from_user.id != TELEGRAM_ADMIN_CHAT_ID:
        await query.answer("你没有权限操作", show_alert=True)
        return

    parts = query.data.split(":", 1)
    if len(parts) != 2:
        return

    action, subscription_id = parts
    if action not in ("approve", "reject"):
        return

    success = await (
        api_client.approve_subscription(subscription_id)
        if action == "approve"
        else api_client.reject_subscription(subscription_id)
    )
    if not success:
        await query.answer("操作失败，请重试", show_alert=True)
        return

    message = query.message
    original_text = ""
    if message is not None:
        original_text = message.text or message.caption or ""

    result_text = format_result_message(original_text, action)

    if message is not None and message.photo:
        await query.edit_message_caption(caption=result_text, parse_mode="HTML")
        return

    await query.edit_message_text(text=result_text, parse_mode="HTML")
