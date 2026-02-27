import asyncio
import logging
from html import escape

from telegram import Update
from telegram.ext import ContextTypes

from app.clients import api_client
from app.config import TELEGRAM_ADMIN_CHAT_ID, TELEGRAM_GROUP_CHAT_ID, TMDB_IMAGE_BASE
from app.formatters.message_formatter import (
    format_account_info,
    format_bind_success,
    format_ranking_message,
    format_registration_message,
    format_redeem_success,
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


async def send_registration_notification(bot, data: dict) -> None:
    text = format_registration_message(data)
    await bot.send_message(
        chat_id=TELEGRAM_ADMIN_CHAT_ID,
        text=text,
        parse_mode="HTML",
    )


async def send_ranking_notification(bot, data: dict) -> None:
    """发送排行榜到 Telegram 群组

    为了不破坏既有部署：
    - 未配置 TELEGRAM_GROUP_CHAT_ID 时，回退到管理员 chat 推送
    """
    text = format_ranking_message(data)
    chat_id = TELEGRAM_GROUP_CHAT_ID or TELEGRAM_ADMIN_CHAT_ID
    if TELEGRAM_GROUP_CHAT_ID is None:
        logger.warning("TELEGRAM_GROUP_CHAT_ID 未配置，排行榜消息将回退推送到管理员")
    await bot.send_message(
        chat_id=chat_id,
        text=text,
        parse_mode="HTML",
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


async def handle_new_member(update: Update, context: ContextTypes.DEFAULT_TYPE) -> None:
    message = update.message
    if message is None or message.new_chat_members is None:
        return

    new_users = [user for user in message.new_chat_members if not user.is_bot]
    if not new_users:
        return

    notify_link = await api_client.get_setting("notify_group_link")
    if not notify_link:
        return

    names = ", ".join(escape(user.first_name or user.full_name) for user in new_users)
    text = (
        f"👋 欢迎 <b>{names}</b> 加入！\n\n"
        f"📢 入库通知群组：{escape(notify_link)}\n"
        "⏳ 本消息将在 30 秒后自动删除"
    )

    sent = await message.reply_text(text, parse_mode="HTML")
    asyncio.create_task(_delete_later(context.bot, sent.chat_id, sent.message_id, 30))


async def handle_bind(update: Update, context: ContextTypes.DEFAULT_TYPE) -> None:
    message = update.message
    if message is None or message.from_user is None:
        return

    if message.chat.type != "private":
        await message.reply_text("⚠️ 请在私聊中使用此命令")
        return

    args = context.args or []
    if len(args) != 1 or len(args[0]) != 6:
        await message.reply_text(
            "📝 <b>使用方式</b>\n\n"
            "/bind <code>验证码</code>\n\n"
            "请先在 Ember 网站生成绑定验证码。",
            parse_mode="HTML",
        )
        return

    result = await api_client.verify_telegram_bind(message.from_user.id, args[0])
    if result is None:
        await message.reply_text("❌ 服务暂不可用，请稍后重试")
        return
    if "error" in result:
        await message.reply_text(f"❌ {escape(str(result['error']))}", parse_mode="HTML")
        return

    await message.reply_text(format_bind_success(result), parse_mode="HTML")


async def handle_info(update: Update, context: ContextTypes.DEFAULT_TYPE) -> None:
    del context
    message = update.message
    if message is None or message.from_user is None:
        return

    if message.chat.type != "private":
        await message.reply_text("⚠️ 请在私聊中使用此命令")
        return

    result = await api_client.get_account_info(message.from_user.id)
    if result is None:
        await message.reply_text("❌ 服务暂不可用，请稍后重试")
        return
    if "error" in result:
        await message.reply_text(f"❌ {escape(str(result['error']))}", parse_mode="HTML")
        return

    await message.reply_text(format_account_info(result), parse_mode="HTML")


async def handle_redeem(update: Update, context: ContextTypes.DEFAULT_TYPE) -> None:
    message = update.message
    if message is None or message.from_user is None:
        return

    if message.chat.type != "private":
        await message.reply_text("⚠️ 请在私聊中使用此命令")
        return

    args = context.args or []
    if len(args) != 1:
        await message.reply_text(
            "📝 <b>使用方式</b>\n\n"
            "/redeem <code>兑换码</code>",
            parse_mode="HTML",
        )
        return

    result = await api_client.redeem_by_telegram(message.from_user.id, args[0])
    if result is None:
        await message.reply_text("❌ 服务暂不可用，请稍后重试")
        return
    if "error" in result:
        await message.reply_text(f"❌ {escape(str(result['error']))}", parse_mode="HTML")
        return

    await message.reply_text(format_redeem_success(result), parse_mode="HTML")


async def handle_resetpw(update: Update, context: ContextTypes.DEFAULT_TYPE) -> None:
    message = update.message
    if message is None or message.from_user is None:
        return

    if message.chat.type != "private":
        await message.reply_text("⚠️ 请在私聊中使用此命令")
        return

    args = context.args or []
    if len(args) != 1 or len(args[0]) < 6:
        await message.reply_text(
            "📝 <b>使用方式</b>\n\n"
            "/resetpw <code>新密码</code>\n\n"
            "密码至少 6 位，将同时更新 Ember 和 Emby 登录密码。",
            parse_mode="HTML",
        )
        return

    result = await api_client.reset_password_by_telegram(message.from_user.id, args[0])
    if result is None:
        await message.reply_text("❌ 服务暂不可用，请稍后重试")
        return
    if "error" in result:
        await message.reply_text(f"❌ {escape(str(result['error']))}", parse_mode="HTML")
        return

    await message.reply_text(
        "✅ <b>密码重置成功</b>\n\n"
        "新密码已同步到 Ember 和 Emby，请使用新密码登录。",
        parse_mode="HTML",
    )


async def _delete_later(bot, chat_id: int, message_id: int, delay: int) -> None:
    await asyncio.sleep(delay)
    try:
        await bot.delete_message(chat_id=chat_id, message_id=message_id)
    except Exception:
        pass
