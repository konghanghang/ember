import asyncio
import logging
from html import escape
from typing import Any, Awaitable, Callable

from telegram import InputMediaPhoto, Update
from telegram.ext import ContextTypes

from app.clients import api_client
from app.config import (
    TMDB_IMAGE_BASE,
    TMDB_IMAGE_BASE_W500,
    TMDB_NO_POSTER_URL,
)
from app.formatters.message_formatter import (
    format_account_info,
    format_bind_success,
    format_payment_message,
    format_ranking_message,
    format_registration_message,
    format_redeem_success,
    format_result_message,
    format_search_detail,
    format_search_results,
    format_subscription_message,
    make_movie_detail_keyboard,
    make_tv_confirm_keyboard,
    make_tv_season_keyboard,
)
from app.handlers.search_cache import (
    SearchSession,
    delete_session,
    get_session,
    set_session,
)
from app.menu_sync import (
    force_chat_member_menu_cleanup,
    force_group_menu_sync,
    is_menu_refresh_allowed,
)
from app.runtime_settings import runtime_settings_service

logger = logging.getLogger(__name__)

WELCOME_MESSAGE_NAMES_PLACEHOLDER = "{names}"
WELCOME_MESSAGE_NOTIFY_LINK_PLACEHOLDER = "{notifyGroupLink}"


def _private_only_tip() -> str:
    return "⚠️ 请在私聊中使用此命令"


def _group_only_tip() -> str:
    return "⚠️ 请在群聊中使用此命令"


def _render_welcome_message(template: str, names: str, notify_group_link: str) -> str:
    return (
        template.replace(WELCOME_MESSAGE_NAMES_PLACEHOLDER, names)
        .replace(WELCOME_MESSAGE_NOTIFY_LINK_PLACEHOLDER, notify_group_link)
        .strip()
    )


def _format_command_help(title: str, command: str, example: str, steps: list[str] | None = None, note: str | None = None) -> str:
    lines = [
        f"❌ <b>{escape(title)}</b>",
        "",
        "正确用法：",
        f"<code>{escape(command)}</code>",
        "",
        "示例：",
        f"<code>{escape(example)}</code>",
    ]

    if steps:
        lines.extend(["", "操作步骤："])
        for index, step in enumerate(steps, start=1):
            lines.append(f"{index}. {escape(step)}")

    if note:
        lines.extend(["", escape(note)])

    return "\n".join(lines)


async def _call_api_and_reply(
    message,
    request_coro: Awaitable[dict[str, Any] | None],
    on_success: Callable[[dict[str, Any]], str],
) -> None:
    result = await request_coro
    if result is None:
        await message.reply_text("❌ 服务暂不可用，请稍后重试")
        return
    if "error" in result:
        await message.reply_text(f"❌ {escape(str(result['error']))}", parse_mode="HTML")
        return

    await message.reply_text(on_success(result), parse_mode="HTML")


async def _edit_search_message(
    *,
    bot,
    chat_id: int,
    message_id: int,
    prefer_media: bool,
    poster_url: str | None,
    caption: str,
    reply_markup,
    media_failure_log: str | None = None,
    final_failure_log: str | None = None,
    raise_on_failure: bool = False,
) -> bool:
    if prefer_media and poster_url:
        try:
            await bot.edit_message_media(
                chat_id=chat_id,
                message_id=message_id,
                media=InputMediaPhoto(
                    media=poster_url,
                    caption=caption,
                    parse_mode="HTML",
                ),
                reply_markup=reply_markup,
            )
            return True
        except Exception:
            if media_failure_log:
                logger.exception(media_failure_log)

    if prefer_media:
        try:
            await bot.edit_message_caption(
                chat_id=chat_id,
                message_id=message_id,
                caption=caption,
                parse_mode="HTML",
                reply_markup=reply_markup,
            )
            return True
        except Exception:
            pass

    try:
        await bot.edit_message_text(
            chat_id=chat_id,
            message_id=message_id,
            text=caption,
            parse_mode="HTML",
            reply_markup=reply_markup,
            disable_web_page_preview=True,
        )
        return True
    except Exception:
        if final_failure_log:
            logger.exception(final_failure_log)
        if raise_on_failure:
            raise
        return False


async def send_subscription_notification(bot, data: dict) -> None:
    admin_chat_id, _ = await runtime_settings_service.get_chat_ids()
    if admin_chat_id is None:
        logger.warning("TELEGRAM_ADMIN_CHAT_ID 未配置，跳过订阅通知")
        return

    text, keyboard = format_subscription_message(data)
    poster_path = data.get("posterPath")

    if poster_path:
        poster_url = f"{TMDB_IMAGE_BASE}{poster_path}"
        try:
            await bot.send_photo(
                chat_id=admin_chat_id,
                photo=poster_url,
                caption=text,
                parse_mode="HTML",
                reply_markup=keyboard,
            )
            return
        except Exception:
            logger.exception("发送海报消息失败，降级为文本消息")

    await bot.send_message(
        chat_id=admin_chat_id,
        text=text,
        parse_mode="HTML",
        reply_markup=keyboard,
    )


async def send_registration_notification(bot, data: dict) -> None:
    admin_chat_id, _ = await runtime_settings_service.get_chat_ids()
    if admin_chat_id is None:
        logger.warning("TELEGRAM_ADMIN_CHAT_ID 未配置，跳过注册通知")
        return

    text = format_registration_message(data)
    await bot.send_message(
        chat_id=admin_chat_id,
        text=text,
        parse_mode="HTML",
    )


async def send_payment_notification(bot, data: dict) -> None:
    admin_chat_id, _ = await runtime_settings_service.get_chat_ids()
    if admin_chat_id is None:
        logger.warning("TELEGRAM_ADMIN_CHAT_ID 未配置，跳过支付通知")
        return

    text = format_payment_message(data)
    await bot.send_message(
        chat_id=admin_chat_id,
        text=text,
        parse_mode="HTML",
    )


async def send_ranking_notification(bot, data: dict) -> None:
    """发送排行榜到 Telegram 群组

    为了不破坏既有部署：
    - 未配置 TELEGRAM_GROUP_CHAT_ID 时，回退到管理员 chat 推送
    """
    admin_chat_id, group_chat_id = await runtime_settings_service.get_chat_ids()
    if admin_chat_id is None and group_chat_id is None:
        logger.warning("TELEGRAM_ADMIN_CHAT_ID 和 TELEGRAM_GROUP_CHAT_ID 均未配置，跳过排行榜通知")
        return

    text = format_ranking_message(data)
    chat_id = group_chat_id or admin_chat_id
    if group_chat_id is None:
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

    admin_chat_id, _ = await runtime_settings_service.get_chat_ids()
    if query.from_user is None or admin_chat_id is None or query.from_user.id != admin_chat_id:
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

    settings = await runtime_settings_service.get()
    template = settings.welcome_message_template.strip()
    if not template:
        return

    names = ", ".join(escape(user.first_name or user.full_name) for user in new_users)
    notify_link = escape(settings.notify_group_link)
    if WELCOME_MESSAGE_NOTIFY_LINK_PLACEHOLDER in template and not notify_link:
        return

    text = _render_welcome_message(template, names, notify_link)
    if not text:
        return

    sent = await message.reply_text(text, parse_mode="HTML")
    asyncio.create_task(_delete_later(context.bot, sent.chat_id, sent.message_id, 30))


async def handle_bind(update: Update, context: ContextTypes.DEFAULT_TYPE) -> None:
    message = update.message
    if message is None or message.from_user is None:
        return

    if message.chat.type != "private":
        await message.reply_text(_private_only_tip())
        return

    args = context.args or []
    if len(args) != 1 or len(args[0]) != 6 or not args[0].isdigit():
        await message.reply_text(
            _format_command_help(
                title="缺少绑定验证码或格式不正确",
                command="/bind 123456",
                example="/bind 123456",
                steps=[
                    "打开 Ember 控制台",
                    "生成 Telegram 绑定验证码",
                    "把 6 位数字填在 /bind 后面发送",
                ],
                note="验证码必须是 6 位数字。",
            ),
            parse_mode="HTML",
        )
        return

    await _call_api_and_reply(
        message,
        api_client.verify_telegram_bind(message.from_user.id, args[0]),
        format_bind_success,
    )


async def handle_info(update: Update, context: ContextTypes.DEFAULT_TYPE) -> None:
    del context
    message = update.message
    if message is None or message.from_user is None:
        return

    if message.chat.type != "private":
        await message.reply_text(_private_only_tip())
        return

    await _call_api_and_reply(
        message,
        api_client.get_account_info(message.from_user.id),
        format_account_info,
    )


async def handle_count(update: Update, context: ContextTypes.DEFAULT_TYPE) -> None:
    del context
    message = update.message
    if message is None or message.from_user is None:
        return

    if message.chat.type != "private":
        await message.reply_text(_private_only_tip())
        return

    result = await api_client.get_media_stats()
    if result is None:
        await message.reply_text("❌ 媒体库统计暂不可用，请稍后重试")
        return
    if not result.get("success") or "data" not in result:
        error_text = str(result.get("error", "查询媒体库统计失败"))
        await message.reply_text(f"❌ {escape(error_text)}", parse_mode="HTML")
        return

    stats = result["data"]
    movie_count = int(stats.get("MovieCount", 0) or 0)
    series_count = int(stats.get("SeriesCount", 0) or 0)
    episode_count = int(stats.get("EpisodeCount", 0) or 0)

    await message.reply_text(
        "🎬 <b>当前媒体库统计</b>\n\n"
        f"电影：<b>{movie_count}</b>\n"
        f"剧集：<b>{series_count}</b>\n"
        f"总集数：<b>{episode_count}</b>",
        parse_mode="HTML",
    )


async def handle_redeem(update: Update, context: ContextTypes.DEFAULT_TYPE) -> None:
    message = update.message
    if message is None or message.from_user is None:
        return

    if message.chat.type != "private":
        await message.reply_text(_private_only_tip())
        return

    args = context.args or []
    if len(args) != 1:
        await message.reply_text(
            _format_command_help(
                title="缺少兑换码",
                command="/redeem ABCD1234",
                example="/redeem ABCD1234",
                note="请把兑换码直接写在 /redeem 后面发送。",
            ),
            parse_mode="HTML",
        )
        return

    await _call_api_and_reply(
        message,
        api_client.redeem_by_telegram(message.from_user.id, args[0]),
        format_redeem_success,
    )


async def handle_resetpw(update: Update, context: ContextTypes.DEFAULT_TYPE) -> None:
    message = update.message
    if message is None or message.from_user is None:
        return

    if message.chat.type != "private":
        await message.reply_text(_private_only_tip())
        return

    args = context.args or []
    if len(args) != 1 or len(args[0]) < 6:
        await message.reply_text(
            _format_command_help(
                title="缺少新密码或密码太短",
                command="/resetpw mynewpass123",
                example="/resetpw mynewpass123",
                note="密码至少 6 位，会同时更新 Ember 和 Emby 登录密码。",
            ),
            parse_mode="HTML",
        )
        return

    await _call_api_and_reply(
        message,
        api_client.reset_password_by_telegram(message.from_user.id, args[0]),
        lambda _result: (
            "✅ <b>密码重置成功</b>\n\n"
            "新密码已同步到 Ember 和 Emby，请使用新密码登录。"
        ),
    )


async def handle_refresh_menu(update: Update, context: ContextTypes.DEFAULT_TYPE) -> None:
    message = update.message
    if message is None or message.from_user is None:
        return

    if message.chat.type not in ("group", "supergroup"):
        await message.reply_text(_group_only_tip())
        return

    allowed, reason = await is_menu_refresh_allowed(context.bot, message.chat.id, message.from_user.id)
    if not allowed:
        await message.reply_text(f"❌ {escape(reason or '只有群管理员或配置的管理员账号可以刷新菜单')}", parse_mode="HTML")
        return

    ok, result = await force_group_menu_sync(context.bot, message.chat.id, message.chat.type)
    if not ok:
        await message.reply_text(f"❌ {escape(result)}", parse_mode="HTML")
        return

    await message.reply_text("✅ 当前群菜单作用域已刷新，旧命令应已清理")


async def handle_refresh_menu_chat(update: Update, context: ContextTypes.DEFAULT_TYPE) -> None:
    message = update.message
    if message is None or message.from_user is None:
        return

    if message.chat.type != "private":
        await message.reply_text(_private_only_tip())
        return

    admin_chat_id, _ = await runtime_settings_service.get_chat_ids()
    if admin_chat_id is None or message.from_user.id != admin_chat_id:
        await message.reply_text("❌ 只有配置的管理员账号可以执行此命令", parse_mode="HTML")
        return

    args = context.args or []
    if len(args) not in (1, 2):
        await message.reply_text(
            _format_command_help(
                title="缺少群 ID 或格式不正确",
                command="/refresh_menu_chat -1001234567890",
                example="/refresh_menu_chat -1001234567890",
                steps=[
                    "打开出现旧命令的 Telegram 群",
                    "复制该群的 chat_id",
                    "把 chat_id 填在命令后面发送",
                ],
                note="可选第二个参数用于指定 user_id：/refresh_menu_chat -1001234567890 123456789",
            ),
            parse_mode="HTML",
        )
        return

    raw_chat_id = args[0].strip()
    raw_user_id = args[1].strip() if len(args) == 2 else str(message.from_user.id)
    if not raw_chat_id or not raw_user_id:
        await message.reply_text("❌ chat_id 或 user_id 不能为空", parse_mode="HTML")
        return

    try:
        target_chat_id = int(raw_chat_id)
        target_user_id = int(raw_user_id)
    except ValueError:
        await message.reply_text("❌ chat_id 和 user_id 必须是整数", parse_mode="HTML")
        return

    ok, result = await force_chat_member_menu_cleanup(context.bot, target_chat_id, target_user_id)
    if not ok:
        await message.reply_text(f"❌ {escape(result)}", parse_mode="HTML")
        return

    await message.reply_text(
        "✅ 已清理指定群的菜单作用域\n\n"
        f"<code>chat_id={target_chat_id}</code>\n"
        f"<code>user_id={target_user_id}</code>\n\n"
        "请在 Telegram 客户端里重新打开该群确认命令是否已消失。",
        parse_mode="HTML",
    )


# ==================== 搜索与订阅 ====================


async def handle_search(update: Update, context: ContextTypes.DEFAULT_TYPE) -> None:
    """处理 /search <关键词> 命令"""
    message = update.message
    if message is None or message.from_user is None:
        return

    if message.chat.type != "private":
        await message.reply_text(_private_only_tip())
        return

    args = context.args or []
    if not args:
        await message.reply_text(
            _format_command_help(
                title="缺少搜索关键词",
                command="/search 搏击俱乐部",
                example="/search 搏击俱乐部",
                note="可以输入电影名、剧名或中英文关键词。",
            ),
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
                "正确用法：\n"
                "<code>/bind 123456</code>\n\n"
                "请先去 Ember 控制台生成 6 位绑定验证码，再回来绑定。",
                parse_mode="HTML",
            )
        else:
            await message.reply_text(
                f"❌ {escape(error_text or '查询账号信息失败，请稍后重试')}",
                parse_mode="HTML",
            )
        return

    await _do_search(message, user_id, query, "multi")


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
        await message.reply_text("😔 未找到相关内容，请尝试其他关键词")
        return

    valid_results = [
        item for item in all_results if item.get("mediaType") in ("movie", "tv")
    ][:8]
    if not valid_results:
        await message.reply_text("😔 未找到相关内容，请尝试其他关键词")
        return

    session = SearchSession(
        results=valid_results,
        media_type=media_type,
        query=query,
    )

    caption, keyboard = format_search_results(valid_results, query)

    first_poster = valid_results[0].get("posterPath")
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
        await _handle_pick(context.bot, query, session, user_id, data)
    elif data.startswith("sub:season:"):
        await _handle_select_season(context.bot, query, session, user_id, data)
    elif data == "sub:ok":
        await _handle_subscribe(query, session, user_id)
    elif data == "sub:back:season":
        await _handle_back_to_seasons(context.bot, query, session, user_id)
    elif data == "sub:back":
        await _handle_back(context.bot, query, session, user_id)
    else:
        await query.answer()


async def _render_search_message(
    *,
    bot,
    query,
    caption: str,
    reply_markup,
    poster_path: str | None,
) -> None:
    prefer_media = bool(query.message and query.message.photo)
    poster_url = f"{TMDB_IMAGE_BASE_W500}{poster_path}" if poster_path else TMDB_NO_POSTER_URL
    final_caption = caption if poster_path else f"（该影片暂无海报）\n\n{caption}"

    await _edit_search_message(
        bot=bot,
        chat_id=query.message.chat_id,
        message_id=query.message.message_id,
        prefer_media=prefer_media,
        poster_url=poster_url,
        caption=final_caption,
        reply_markup=reply_markup,
        media_failure_log="编辑海报失败，降级为编辑文本",
        raise_on_failure=True,
    )


async def _load_tv_seasons(tmdb_id: int | str) -> tuple[list[int] | None, str | None]:
    result = await api_client.get_tmdb_tv_seasons(tmdb_id)
    if result is None:
        return None, "服务暂不可用，请稍后重试"
    if "error" in result:
        return None, str(result["error"])

    data = result.get("data") or {}
    seasons_raw = data.get("seasons") or []
    seasons = [
        int(season)
        for season in seasons_raw
        if str(season).isdigit() and int(season) > 0
    ]
    if not seasons:
        return None, "TMDB 未返回可订阅季数"

    return seasons, None


async def _handle_pick(bot, query, session: SearchSession, user_id: int, data: str) -> None:
    """用户点击编号按钮，进入电影确认页或电视剧选季页"""
    try:
        index = int(data.split(":")[-1])
    except (ValueError, IndexError):
        await query.answer("该操作已失效，请重新选择", show_alert=True)
        return

    if index < 0 or index >= len(session.results):
        await query.answer("该操作已失效，请重新选择", show_alert=True)
        return

    session.selected_index = index
    item = session.results[index]
    session.selected_season = None
    session.season_options = []

    item_media_type = item.get("mediaType", "movie")
    if item_media_type == "tv":
        seasons, err_msg = await _load_tv_seasons(item.get("id", ""))
        if err_msg:
            await query.answer(err_msg, show_alert=True)
            return
        session.season_options = seasons or []
        set_session(user_id, session)

        caption = f"{format_search_detail(item)}\n\n📺 请选择要订阅的季数"
        keyboard = make_tv_season_keyboard(session.season_options)
    else:
        set_session(user_id, session)
        caption = format_search_detail(item)
        keyboard = make_movie_detail_keyboard()

    await query.answer()
    await _render_search_message(
        bot=bot,
        query=query,
        caption=caption,
        reply_markup=keyboard,
        poster_path=item.get("posterPath"),
    )


async def _handle_select_season(bot, query, session: SearchSession, user_id: int, data: str) -> None:
    """电视剧选择季数后进入确认页"""
    if session.selected_index < 0 or session.selected_index >= len(session.results):
        await query.answer("请先选择一个结果", show_alert=True)
        return

    try:
        season = int(data.split(":")[-1])
    except (ValueError, IndexError):
        await query.answer("季数无效，请重新选择", show_alert=True)
        return

    item = session.results[session.selected_index]
    if item.get("mediaType") != "tv":
        await query.answer("当前内容不是电视剧", show_alert=True)
        return

    if season not in session.season_options:
        await query.answer("该季数不可用，请重新选择", show_alert=True)
        return

    session.selected_season = season
    set_session(user_id, session)

    caption = format_search_detail(item, selected_season=season)
    keyboard = make_tv_confirm_keyboard(season)

    await query.answer()
    await _render_search_message(
        bot=bot,
        query=query,
        caption=caption,
        reply_markup=keyboard,
        poster_path=item.get("posterPath"),
    )


async def _handle_subscribe(
    query, session: SearchSession, user_id: int
) -> None:
    """处理最终订阅确认"""
    if session.selected_index < 0 or session.selected_index >= len(session.results):
        await query.answer("请先选择一个结果", show_alert=True)
        return

    item = session.results[session.selected_index]
    item_media_type = item.get("mediaType", "movie")
    media_type_upper = "MOVIE" if item_media_type == "movie" else "TV"
    season = 0
    if item_media_type == "tv":
        if session.selected_season is None:
            await query.answer("请先选择季数", show_alert=True)
            return
        season = session.selected_season

    result = await api_client.subscribe_by_telegram(
        telegram_id=user_id,
        media_type=media_type_upper,
        name=item.get("title", ""),
        tmdb_id=str(item.get("id", "")),
        season=season,
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
        f"📌 {title}\n"
        + (f"📺 第 {season} 季\n" if item_media_type == "tv" else "")
        + "\n"
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


async def _handle_back(bot, query, session: SearchSession, user_id: int) -> None:
    """返回搜索结果列表"""
    session.selected_index = -1
    session.selected_season = None
    session.season_options = []
    set_session(user_id, session)

    caption, keyboard = format_search_results(
        session.results, session.query
    )
    first_poster = (
        session.results[0].get("posterPath") if session.results else None
    )

    await query.answer()

    prefer_media = bool(query.message and query.message.photo)
    poster_url = f"{TMDB_IMAGE_BASE_W500}{first_poster}" if first_poster else TMDB_NO_POSTER_URL
    final_caption = caption if first_poster else f"（暂无海报）\n\n{caption}"

    await _edit_search_message(
        bot=bot,
        chat_id=query.message.chat_id,
        message_id=query.message.message_id,
        prefer_media=prefer_media,
        poster_url=poster_url,
        caption=final_caption,
        reply_markup=keyboard,
        media_failure_log="返回列表编辑海报失败",
        raise_on_failure=True,
    )


async def _handle_back_to_seasons(bot, query, session: SearchSession, user_id: int) -> None:
    """电视剧从确认页回到选季页"""
    if session.selected_index < 0 or session.selected_index >= len(session.results):
        await query.answer("请先选择一个结果", show_alert=True)
        return

    item = session.results[session.selected_index]
    if item.get("mediaType") != "tv":
        await query.answer("当前内容不是电视剧", show_alert=True)
        return

    if not session.season_options:
        seasons, err_msg = await _load_tv_seasons(item.get("id", ""))
        if err_msg:
            await query.answer(err_msg, show_alert=True)
            return
        session.season_options = seasons or []

    session.selected_season = None
    set_session(user_id, session)

    caption = f"{format_search_detail(item)}\n\n📺 请选择要订阅的季数"
    keyboard = make_tv_season_keyboard(session.season_options)

    await query.answer()
    await _render_search_message(
        bot=bot,
        query=query,
        caption=caption,
        reply_markup=keyboard,
        poster_path=item.get("posterPath"),
    )


async def _delete_later(bot, chat_id: int, message_id: int, delay: int) -> None:
    await asyncio.sleep(delay)
    try:
        await bot.delete_message(chat_id=chat_id, message_id=message_id)
    except Exception:
        pass
