import asyncio
import logging
from html import escape

from telegram import InputMediaPhoto, Update
from telegram.ext import ContextTypes

from app.clients import api_client
from app.config import (
    TELEGRAM_ADMIN_CHAT_ID,
    TELEGRAM_GROUP_CHAT_ID,
    TMDB_IMAGE_BASE,
    TMDB_IMAGE_BASE_W500,
    TMDB_NO_POSTER_URL,
)
from app.formatters.message_formatter import (
    format_account_info,
    format_bind_success,
    format_ranking_message,
    format_registration_message,
    format_redeem_success,
    format_result_message,
    format_search_detail,
    format_search_results,
    format_subscription_message,
    make_detail_keyboard,
)
from app.handlers.search_cache import (
    SearchSession,
    delete_session,
    get_session,
    set_session,
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

    if query.from_user is None or query.from_user.id != TELEGRAM_ADMIN_CHAT_ID:
        await query.answer("你没有权限操作", show_alert=True)
        return

    parts = query.data.split(":", 1)
    if len(parts) != 2:
        await query.answer("操作无效", show_alert=True)
        return

    action, subscription_id = parts
    if action not in ("approve", "reject"):
        await query.answer("操作无效", show_alert=True)
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

    await query.answer()

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


# ==================== 搜索与订阅 ====================


async def handle_search(update: Update, context: ContextTypes.DEFAULT_TYPE) -> None:
    """处理 /search <关键词> 命令"""
    message = update.message
    if message is None or message.from_user is None:
        return

    if message.chat.type != "private":
        await message.reply_text("⚠️ 请在私聊中使用此命令")
        return

    args = context.args or []
    if not args:
        await message.reply_text(
            "📝 <b>使用方式</b>\n\n"
            "/search <code>关键词</code>\n\n"
            "例如：/search 搏击俱乐部",
            parse_mode="HTML",
        )
        return

    query = " ".join(args)
    user_id = message.from_user.id

    info = await api_client.get_account_info(user_id)
    if info is None:
        await message.reply_text("❌ 服务暂不可用，请稍后重试")
        return
    if "error" in info:
        error_text = str(info.get("error", "") or "")
        status_code = int(info.get("status", 500) or 500)
        if status_code == 400:
            await message.reply_text(
                "❌ 请先绑定 Telegram 账号后再使用搜索功能\n\n"
                "使用 /bind <code>验证码</code> 绑定",
                parse_mode="HTML",
            )
        else:
            await message.reply_text(
                f"❌ {escape(error_text or '查询账号信息失败，请稍后重试')}",
                parse_mode="HTML",
            )
        return

    await _do_search(message, user_id, query, "movie")


async def _do_search(message, user_id: int, query: str, media_type: str) -> None:
    """执行 TMDB 搜索并发送新消息（仅用于首次 /search 命令）"""
    result = await api_client.search_tmdb(query, media_type)
    if result is None:
        await message.reply_text("❌ 搜索服务暂不可用，请稍后重试")
        return
    if "error" in result:
        await message.reply_text(
            f"❌ {escape(str(result['error']))}", parse_mode="HTML"
        )
        return

    all_results = result.get("results", [])
    if not all_results:
        type_label = "电影" if media_type == "movie" else "电视剧"
        await message.reply_text(f"😔 未找到相关{type_label}，请尝试其他关键词")
        return

    results = all_results[:8]

    session = SearchSession(
        results=results,
        media_type=media_type,
        query=query,
    )

    caption, keyboard = format_search_results(results, media_type, query)

    first_poster = results[0].get("posterPath")
    if first_poster:
        poster_url = f"{TMDB_IMAGE_BASE_W500}{first_poster}"
        try:
            sent = await message.reply_photo(
                photo=poster_url,
                caption=caption,
                parse_mode="HTML",
                reply_markup=keyboard,
            )
            session.message_id = sent.message_id
            session.chat_id = sent.chat_id
            set_session(user_id, session)
            return
        except Exception:
            logger.exception("发送搜索海报失败，降级为文本消息")

    sent = await message.reply_text(
        text=caption,
        parse_mode="HTML",
        reply_markup=keyboard,
        disable_web_page_preview=True,
    )
    session.message_id = sent.message_id
    session.chat_id = sent.chat_id
    set_session(user_id, session)


async def handle_search_callback(
    update: Update, context: ContextTypes.DEFAULT_TYPE
) -> None:
    """处理所有 sub: 前缀的 callback query"""
    del context
    query = update.callback_query
    if query is None or query.data is None or query.from_user is None:
        return

    user_id = query.from_user.id
    data = query.data

    session = get_session(user_id)
    if session is None:
        await query.answer("搜索已过期，请重新搜索", show_alert=True)
        return

    if query.message is None:
        await query.answer("搜索已过期，请重新搜索", show_alert=True)
        return
    if (
        query.message.chat_id != session.chat_id
        or query.message.message_id != session.message_id
    ):
        await query.answer("该搜索已失效，请使用最新搜索结果", show_alert=True)
        return

    if data.startswith("sub:pick:"):
        await _handle_pick(query, session, user_id, data)
    elif data == "sub:type":
        await _handle_toggle_type(query, session, user_id)
    elif data == "sub:ok":
        await _handle_subscribe(query, session, user_id)
    elif data == "sub:note":
        await _handle_request_note(query, session, user_id)
    elif data == "sub:back":
        await _handle_back(query, session, user_id)
    else:
        await query.answer()


async def _handle_pick(query, session: SearchSession, user_id: int, data: str) -> None:
    """用户点击编号按钮，编辑消息为详情页"""
    try:
        index = int(data.split(":")[-1])
    except (ValueError, IndexError):
        await query.answer("该操作已失效，请重新选择", show_alert=True)
        return

    if index < 0 or index >= len(session.results):
        await query.answer("该操作已失效，请重新选择", show_alert=True)
        return

    session.selected_index = index
    session.waiting_for_note = False
    set_session(user_id, session)

    item = session.results[index]
    caption = format_search_detail(item, session.media_type)
    keyboard = make_detail_keyboard()

    await query.answer()

    poster_path = item.get("posterPath")
    no_poster_prefix = "（该影片暂无海报）\n\n"

    if query.message and query.message.photo:
        if poster_path:
            poster_url = f"{TMDB_IMAGE_BASE_W500}{poster_path}"
            final_caption = caption
        else:
            poster_url = TMDB_NO_POSTER_URL
            final_caption = no_poster_prefix + caption
        try:
            await query.edit_message_media(
                media=InputMediaPhoto(
                    media=poster_url,
                    caption=final_caption,
                    parse_mode="HTML",
                ),
                reply_markup=keyboard,
            )
            return
        except Exception:
            logger.exception("编辑海报失败，降级为编辑文本")

    final_caption = (no_poster_prefix + caption) if not poster_path else caption
    if query.message and query.message.photo:
        await query.edit_message_caption(
            caption=final_caption, parse_mode="HTML", reply_markup=keyboard
        )
    else:
        await query.edit_message_text(
            text=final_caption,
            parse_mode="HTML",
            reply_markup=keyboard,
            disable_web_page_preview=True,
        )


async def _handle_toggle_type(
    query, session: SearchSession, user_id: int
) -> None:
    """切换搜索类型（电影 ↔ 电视剧），重新搜索并编辑消息"""
    new_type = "tv" if session.media_type == "movie" else "movie"

    result = await api_client.search_tmdb(session.query, new_type)
    if result is None or "error" in result:
        await query.answer("搜索失败，请重试", show_alert=True)
        return

    all_results = result.get("results", [])
    if not all_results:
        type_label = "电影" if new_type == "movie" else "电视剧"
        await query.answer(f"未找到相关{type_label}", show_alert=True)
        return

    results = all_results[:8]
    new_session = SearchSession(
        results=results,
        media_type=new_type,
        query=session.query,
        message_id=session.message_id,
        chat_id=session.chat_id,
    )

    caption, keyboard = format_search_results(results, new_type, session.query)
    first_poster = results[0].get("posterPath")

    await query.answer()

    if query.message and query.message.photo:
        if first_poster:
            poster_url = f"{TMDB_IMAGE_BASE_W500}{first_poster}"
        else:
            poster_url = TMDB_NO_POSTER_URL
        try:
            await query.edit_message_media(
                media=InputMediaPhoto(
                    media=poster_url,
                    caption=caption,
                    parse_mode="HTML",
                ),
                reply_markup=keyboard,
            )
            set_session(user_id, new_session)
            return
        except Exception:
            logger.exception("切换类型编辑海报失败")

    final_caption = caption if first_poster else f"（暂无海报）\n\n{caption}"
    if query.message and query.message.photo:
        await query.edit_message_caption(
            caption=final_caption, parse_mode="HTML", reply_markup=keyboard
        )
    else:
        await query.edit_message_text(
            text=final_caption,
            parse_mode="HTML",
            reply_markup=keyboard,
            disable_web_page_preview=True,
        )
    set_session(user_id, new_session)


async def _handle_subscribe(
    query, session: SearchSession, user_id: int
) -> None:
    """直接订阅（无备注）"""
    if session.selected_index < 0 or session.selected_index >= len(session.results):
        await query.answer("请先选择一个结果", show_alert=True)
        return

    item = session.results[session.selected_index]
    media_type_upper = "MOVIE" if session.media_type == "movie" else "TV"

    result = await api_client.subscribe_by_telegram(
        telegram_id=user_id,
        media_type=media_type_upper,
        name=item.get("title", ""),
        tmdb_id=str(item.get("id", "")),
        poster_path=item.get("posterPath") or "",
    )

    if result is None:
        await query.answer("服务暂不可用，请稍后重试", show_alert=True)
        return

    if "error" in result:
        error_text = str(result["error"])
        if result.get("status") == 409:
            error_text = "该影片已有用户提交订阅，无需重复提交"
        await query.answer(error_text, show_alert=True)
        return

    title = escape(str(item.get("title", "")))
    success_text = (
        f"✅ <b>订阅成功</b>\n\n"
        f"📌 {title}\n\n"
        "已提交求片请求，请等待管理员审核。"
    )

    await query.answer()

    try:
        if query.message and query.message.photo:
            await query.edit_message_caption(
                caption=success_text, parse_mode="HTML", reply_markup=None
            )
        else:
            await query.edit_message_text(text=success_text, parse_mode="HTML")
    except Exception:
        logger.exception("订阅成功但编辑消息失败，降级为 reply_text")
        try:
            if query.message is not None:
                await query.message.reply_text("✅ 订阅成功，请等待管理员审核。")
            else:
                logger.warning("订阅成功但无法发送反馈：query.message 为空")
        except Exception:
            pass

    delete_session(user_id)


async def _handle_request_note(
    query, session: SearchSession, user_id: int
) -> None:
    """用户点击「添加备注」，切换到等待文本输入状态"""
    if session.selected_index < 0:
        await query.answer("请先选择一个结果", show_alert=True)
        return

    session.waiting_for_note = True
    set_session(user_id, session)

    item = session.results[session.selected_index]
    title = escape(str(item.get("title", "")))

    text = (
        f"📝 请输入订阅备注（直接发送文字即可）\n\n"
        f"📌 {title}\n\n"
        "发送 /cancel 取消"
    )

    await query.answer()

    if query.message and query.message.photo:
        await query.edit_message_caption(
            caption=text, parse_mode="HTML", reply_markup=None
        )
    else:
        await query.edit_message_text(text=text, parse_mode="HTML")


async def _handle_back(query, session: SearchSession, user_id: int) -> None:
    """返回搜索结果列表"""
    session.selected_index = -1
    session.waiting_for_note = False
    set_session(user_id, session)

    caption, keyboard = format_search_results(
        session.results, session.media_type, session.query
    )
    first_poster = (
        session.results[0].get("posterPath") if session.results else None
    )

    await query.answer()

    if query.message and query.message.photo:
        if first_poster:
            poster_url = f"{TMDB_IMAGE_BASE_W500}{first_poster}"
        else:
            poster_url = TMDB_NO_POSTER_URL
        try:
            await query.edit_message_media(
                media=InputMediaPhoto(
                    media=poster_url,
                    caption=caption,
                    parse_mode="HTML",
                ),
                reply_markup=keyboard,
            )
            return
        except Exception:
            logger.exception("返回列表编辑海报失败")

    final_caption = caption if first_poster else f"（暂无海报）\n\n{caption}"
    if query.message and query.message.photo:
        await query.edit_message_caption(
            caption=final_caption, parse_mode="HTML", reply_markup=keyboard
        )
    else:
        await query.edit_message_text(
            text=final_caption,
            parse_mode="HTML",
            reply_markup=keyboard,
            disable_web_page_preview=True,
        )


async def handle_cancel_note(
    update: Update, context: ContextTypes.DEFAULT_TYPE
) -> None:
    """处理 /cancel：取消备注输入并恢复详情页按钮"""
    message = update.message
    if message is None or message.from_user is None:
        return

    if message.chat.type != "private":
        return

    user_id = message.from_user.id
    session = get_session(user_id)
    if session is None or not session.waiting_for_note:
        await message.reply_text("当前没有待取消的备注输入。")
        return

    if session.selected_index < 0 or session.selected_index >= len(session.results):
        delete_session(user_id)
        await message.reply_text("搜索会话已失效，请重新发起 /search。")
        return

    session.waiting_for_note = False
    set_session(user_id, session)

    item = session.results[session.selected_index]
    raw_caption = format_search_detail(item, session.media_type)
    keyboard = make_detail_keyboard()
    poster_path = item.get("posterPath")

    if poster_path:
        poster_url = f"{TMDB_IMAGE_BASE_W500}{poster_path}"
        caption = raw_caption
    else:
        poster_url = TMDB_NO_POSTER_URL
        caption = "（该影片暂无海报）\n\n" + raw_caption

    try:
        await context.bot.edit_message_media(
            chat_id=session.chat_id,
            message_id=session.message_id,
            media=InputMediaPhoto(
                media=poster_url,
                caption=caption,
                parse_mode="HTML",
            ),
            reply_markup=keyboard,
        )
        return
    except Exception:
        logger.exception("取消备注后恢复详情海报失败，降级为编辑文本")

    try:
        await context.bot.edit_message_caption(
            chat_id=session.chat_id,
            message_id=session.message_id,
            caption=caption,
            parse_mode="HTML",
            reply_markup=keyboard,
        )
    except Exception:
        try:
            await context.bot.edit_message_text(
                chat_id=session.chat_id,
                message_id=session.message_id,
                text=caption,
                parse_mode="HTML",
                reply_markup=keyboard,
                disable_web_page_preview=True,
            )
        except Exception:
            logger.exception("取消备注后恢复详情页完全失败")
            await message.reply_text(
                "❌ 恢复详情页失败，请重新使用 /search 搜索。"
            )


async def handle_text_message(
    update: Update, context: ContextTypes.DEFAULT_TYPE
) -> None:
    """处理私聊文本消息（仅用于接收备注输入）"""
    del context
    message = update.message
    if message is None or message.from_user is None or message.text is None:
        return

    user_id = message.from_user.id
    session = get_session(user_id)

    if session is None or not session.waiting_for_note:
        return

    if session.selected_index < 0 or session.selected_index >= len(session.results):
        delete_session(user_id)
        return

    note = message.text.strip()
    if not note:
        await message.reply_text("备注不能为空，请重新输入，或发送 /cancel 取消。")
        return

    item = session.results[session.selected_index]
    media_type_upper = "MOVIE" if session.media_type == "movie" else "TV"

    result = await api_client.subscribe_by_telegram(
        telegram_id=user_id,
        media_type=media_type_upper,
        name=item.get("title", ""),
        tmdb_id=str(item.get("id", "")),
        poster_path=item.get("posterPath") or "",
        note=note,
    )

    if result is None:
        await message.reply_text("❌ 服务暂不可用，请稍后重试，或发送 /cancel 返回详情页。")
        return
    if "error" in result:
        error_text = str(result["error"])
        if result.get("status") == 409:
            error_text = "该影片已有用户提交订阅，无需重复提交"
        await message.reply_text(
            f"❌ {escape(error_text)}\n\n发送 /cancel 返回详情页，或直接修改备注后重试。",
            parse_mode="HTML",
        )
        return

    title = escape(str(item.get("title", "")))
    try:
        await message.reply_text(
            f"✅ <b>订阅成功</b>\n\n"
            f"📌 {title}\n"
            f"💬 备注：{escape(note)}\n\n"
            "已提交求片请求，请等待管理员审核。",
            parse_mode="HTML",
        )
    except Exception:
        logger.exception("订阅成功但发送成功消息失败")
        try:
            await message.reply_text("✅ 订阅成功，请等待管理员审核。")
        except Exception:
            pass

    delete_session(user_id)


async def _delete_later(bot, chat_id: int, message_id: int, delay: int) -> None:
    await asyncio.sleep(delay)
    try:
        await bot.delete_message(chat_id=chat_id, message_id=message_id)
    except Exception:
        pass
