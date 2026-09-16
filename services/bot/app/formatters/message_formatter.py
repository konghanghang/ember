import os
from datetime import datetime, timezone
from html import escape, unescape
from zoneinfo import ZoneInfo, ZoneInfoNotFoundError

from telegram import InlineKeyboardButton, InlineKeyboardMarkup

TELEGRAM_TEXT_LIMIT = 4096
TELEGRAM_CAPTION_LIMIT = 1024
_SUBSCRIPTION_NAME_LIMIT = 160
_SUBSCRIPTION_NOTE_LIMIT = 500
_RESULT_REASON_LIMIT = 500
_SEARCH_OVERVIEW_LIMIT = 300
_SEARCH_QUERY_LIMIT_CAPTION = 80
_SEARCH_QUERY_LIMIT_TEXT = 160
_TEXT_TRUNCATION_SUFFIX = "..."
_DEFAULT_DISPLAY_TIMEZONE = "Asia/Shanghai"


def _format_media_type(media_type: str) -> str:
    if media_type == "MOVIE":
        return "电影"
    if media_type == "TV":
        return "电视剧"
    return media_type


def _format_registration_mode(mode: str) -> str:
    if mode == "invite":
        return "邀请码注册"
    if mode == "open":
        return "开放注册"
    return mode or "-"


def _truncate_text(value: str, limit: int) -> str:
    if limit <= 0:
        return ""
    if len(value) <= limit:
        return value
    if limit <= len(_TEXT_TRUNCATION_SUFFIX):
        return value[:limit]
    return value[: limit - len(_TEXT_TRUNCATION_SUFFIX)] + _TEXT_TRUNCATION_SUFFIX


def _escape_truncated_text(value: str, limit: int) -> str:
    """按解析后文本预算裁剪字段，再转义成 Telegram HTML。"""
    if limit <= 0:
        return ""
    if len(value) <= limit:
        return escape(value)
    if limit <= len(_TEXT_TRUNCATION_SUFFIX):
        return escape(_TEXT_TRUNCATION_SUFFIX[:limit])
    return escape(value[: limit - len(_TEXT_TRUNCATION_SUFFIX)] + _TEXT_TRUNCATION_SUFFIX)


def _html_text_length(value: str) -> int:
    """计算 Telegram HTML 解析后的可见文本长度，用于 Bot API 长度预算。"""
    length = 0
    index = 0
    while index < len(value):
        char = value[index]
        if char == "<":
            end = value.find(">", index + 1)
            if end == -1:
                length += 1
                index += 1
            else:
                index = end + 1
            continue
        if char == "&":
            end = value.find(";", index + 1)
            if end != -1:
                length += len(unescape(value[index : end + 1]))
                index = end + 1
            else:
                length += 1
                index += 1
            continue
        length += 1
        index += 1
    return length


def _clamp_html_to_text_budget(value: str, limit: int) -> str:
    """按 Telegram 解析后文本预算裁剪 HTML，保留合法标签和完整实体。"""
    if limit <= 0:
        return ""
    if _html_text_length(value) <= limit:
        return value
    if limit <= len(_TEXT_TRUNCATION_SUFFIX):
        return _TEXT_TRUNCATION_SUFFIX[:limit]

    suffix = _TEXT_TRUNCATION_SUFFIX
    text_budget = limit - len(suffix)
    stack: list[str] = []
    result: list[str] = []
    visible_length = 0
    index = 0

    while index < len(value) and visible_length < text_budget:
        char = value[index]
        if char == "<":
            end = value.find(">", index + 1)
            if end == -1:
                break
            token = value[index : end + 1]
            lower = token.lower()
            if lower.startswith("</"):
                tag = lower[2:-1].strip().split()[0] if lower[2:-1].strip() else ""
                result.append(token)
                if stack and stack[-1] == tag:
                    stack.pop()
            else:
                tag = lower[1:-1].strip().split()[0].rstrip("/") if lower[1:-1].strip() else ""
                result.append(token)
                if tag and not lower.endswith("/>"):
                    stack.append(tag)
            index = end + 1
            continue

        if char == "&":
            end = value.find(";", index + 1)
            token = value[index : end + 1] if end != -1 else char
            token_visible_length = len(unescape(token)) if end != -1 else 1
            next_index = end + 1 if end != -1 else index + 1
        else:
            token = char
            token_visible_length = 1
            next_index = index + 1

        if visible_length + token_visible_length > text_budget:
            break
        result.append(token)
        visible_length += token_visible_length
        index = next_index

    result.append(suffix)
    result.extend(f"</{tag}>" for tag in reversed(stack))
    return "".join(result)


def clamp_telegram_html(value: str, *, is_caption: bool = False) -> str:
    """按 Telegram text/caption 解析后文本预算裁剪，并保持 HTML 结构完整。"""
    limit = TELEGRAM_CAPTION_LIMIT if is_caption else TELEGRAM_TEXT_LIMIT
    return _clamp_html_to_text_budget(value, limit)


def _format_result_with_budget(original_text: str, result: str, *, is_caption: bool) -> str:
    """保留审批终态文本，必要时只压缩前面的原审批消息。"""
    limit = TELEGRAM_CAPTION_LIMIT if is_caption else TELEGRAM_TEXT_LIMIT
    separator = "\n\n────────────────────\n"
    result_text = _clamp_html_to_text_budget(result, max(0, limit - _html_text_length(separator)))
    original_limit = max(0, limit - _html_text_length(separator) - _html_text_length(result_text))
    original = _clamp_html_to_text_budget(original_text.strip(), original_limit)
    return f"{original}{separator}{result_text}" if original else result_text


def format_subscription_message(data: dict) -> tuple[str, InlineKeyboardMarkup]:
    media_type = _format_media_type(data.get("type", ""))
    name = escape(_truncate_text(str(data.get("name", "")), _SUBSCRIPTION_NAME_LIMIT))
    user_name = escape(str(data.get("userName", "") or "-"))
    tmdb_id = escape(str(data.get("tmdbId", "")))
    season = int(data.get("season", 0) or 0)
    note = _truncate_text(str(data.get("note", "") or "").strip(), _SUBSCRIPTION_NOTE_LIMIT)

    lines = [
        "🎬 <b>新的求片请求</b>",
        "",
        f"📌 <b>{name}</b>",
        f"🎭 类型：{media_type}",
        f"👤 用户：{user_name}",
        f"🔗 TMDB：<a href='https://www.themoviedb.org/{'movie' if data.get('type') == 'MOVIE' else 'tv'}/{tmdb_id}'>#{tmdb_id}</a>",
    ]

    if season > 0:
        lines.append(f"📺 季：第 {season} 季")

    if note != "":
        lines.append(f"💬 备注：{escape(note)}")

    keyboard = InlineKeyboardMarkup(
        [
            [
                InlineKeyboardButton(
                    "✅ 通过", callback_data=f"approve:{data.get('id', '')}"
                ),
                InlineKeyboardButton(
                    "❌ 拒绝", callback_data=f"reject:{data.get('id', '')}"
                ),
            ]
        ]
    )

    return clamp_telegram_html("\n".join(lines), is_caption=True), keyboard


def format_auto_approved_subscription_message(data: dict) -> str:
    media_type = _format_media_type(data.get("type", ""))
    name = escape(_truncate_text(str(data.get("name", "")), _SUBSCRIPTION_NAME_LIMIT))
    user_name = escape(str(data.get("userName", "") or "-"))
    tmdb_id = escape(str(data.get("tmdbId", "")))
    season = int(data.get("season", 0) or 0)
    note = _truncate_text(str(data.get("note", "") or "").strip(), _SUBSCRIPTION_NOTE_LIMIT)
    plan_group_name = escape(str(data.get("planGroupName", "") or "").strip())
    plan_group_key = escape(str(data.get("planGroupKey", "") or "").strip())
    ordinal = int(data.get("autoApprovedOrdinal", 0) or 0)
    daily_limit = int(data.get("dailyLimit", 0) or 0)
    reviewed_at = _format_expiry(data.get("reviewedAt"))

    plan_group_display = plan_group_name or plan_group_key or "-"
    quota_display = "-"
    if ordinal > 0 and daily_limit > 0:
        quota_display = f"今日第 {ordinal}/{daily_limit} 条"
    elif daily_limit > 0:
        quota_display = f"每日额度 {daily_limit} 条"

    lines = [
        "⚡ <b>订阅已自动通过</b>",
        "",
        f"📌 <b>{name}</b>",
        f"🎭 类型：{media_type}",
        f"👤 用户：{user_name}",
        f"🧩 分组：{plan_group_display}",
        f"📊 原因：命中分组自动通过额度（{quota_display}）",
        f"🔗 TMDB：<a href='https://www.themoviedb.org/{'movie' if data.get('type') == 'MOVIE' else 'tv'}/{tmdb_id}'>#{tmdb_id}</a>",
    ]

    if season > 0:
        lines.append(f"📺 季：第 {season} 季")
    if reviewed_at != "永不过期":
        lines.append(f"🕒 通过时间：{reviewed_at}")
    if note != "":
        lines.append(f"💬 备注：{escape(note)}")

    return clamp_telegram_html("\n".join(lines), is_caption=True)


def format_registration_message(data: dict) -> str:
    user_name = escape(str(data.get("userName", "") or "-"))
    email = escape(str(data.get("email", "") or "-"))
    emby_id = escape(str(data.get("embyId", "") or "-"))
    mode = escape(_format_registration_mode(str(data.get("registrationMode", ""))))
    expires_at = _format_expiry(data.get("expiresAt"))

    lines = [
        "🆕 <b>新用户注册成功</b>",
        "",
        f"👤 用户名：{user_name}",
        f"📧 邮箱：{email}",
        f"🧾 Emby ID：<code>{emby_id}</code>",
        f"🛂 注册方式：{mode}",
        f"⏳ 到期时间：{expires_at}",
    ]
    return clamp_telegram_html("\n".join(lines))


def _format_currency(amount: int, currency: str) -> str:
    normalized = (currency or "USD").upper()
    symbol_map = {
        "USD": "$",
        "CNY": "¥",
        "HKD": "HK$",
    }
    symbol = symbol_map.get(normalized, f"{normalized} ")
    major = amount / 100
    return f"{symbol}{major:.2f}"


def _get_display_timezone() -> ZoneInfo | timezone:
    tz_name = (os.getenv("TZ", _DEFAULT_DISPLAY_TIMEZONE) or "").strip() or _DEFAULT_DISPLAY_TIMEZONE
    try:
        return ZoneInfo(tz_name)
    except ZoneInfoNotFoundError:
        try:
            return ZoneInfo(_DEFAULT_DISPLAY_TIMEZONE)
        except ZoneInfoNotFoundError:
            return timezone.utc


def _parse_datetime(value: str) -> datetime | None:
    normalized = value.strip()
    if normalized == "":
        return None
    if normalized.endswith("Z"):
        normalized = normalized[:-1] + "+00:00"
    try:
        parsed = datetime.fromisoformat(normalized)
    except ValueError:
        return None
    if parsed.tzinfo is None:
        return parsed.replace(tzinfo=timezone.utc)
    return parsed


def _format_expiry(value: str | None, *, date_only: bool = False) -> str:
    raw = str(value or "").strip()
    if raw == "":
        return "永不过期"
    parsed = _parse_datetime(raw)
    if parsed is None:
        fallback = raw.replace("T", " ")
        return escape(fallback[:10] if date_only else fallback[:19])
    local_value = parsed.astimezone(_get_display_timezone())
    pattern = "%Y-%m-%d" if date_only else "%Y-%m-%d %H:%M:%S"
    return escape(local_value.strftime(pattern))


def format_payment_message(data: dict) -> str:
    user_name = escape(str(data.get("userName", "") or "-"))
    plan_name = escape(str(data.get("planName", "") or "-"))
    amount = int(data.get("amount", 0) or 0)
    currency = str(data.get("currency", "") or "USD")
    days = int(data.get("days", 0) or 0)
    payment_id = escape(str(data.get("paymentId", "") or "-"))
    old_expires_at = _format_expiry(data.get("oldExpiresAt"))
    new_expires_at = _format_expiry(data.get("newExpiresAt"))

    lines = [
        "💰 <b>支付成功</b>",
        "",
        f"👤 用户：{user_name}",
        f"📦 方案：{plan_name}",
        f"💵 金额：{escape(_format_currency(amount, currency))}",
        f"📅 延长：<b>{days}</b> 天",
        f"⏳ 原到期：{old_expires_at}",
        f"✅ 新到期：{new_expires_at}",
        f"🧾 支付记录：<code>{payment_id}</code>",
    ]

    return clamp_telegram_html("\n".join(lines))


def format_result_message(original_text: str, action: str, reason: str | None = None) -> str:
    result = "✅ 已通过" if action == "approve" else "❌ 已拒绝"
    if action == "reject" and reason:
        reason_budget = max(0, TELEGRAM_CAPTION_LIMIT - len(result) - len("\n📝 原因："))
        result = f"{result}\n📝 原因：{_escape_truncated_text(reason.strip(), min(_RESULT_REASON_LIMIT, reason_budget))}"
    return _format_result_with_budget(original_text, result, is_caption=True)


def format_subscription_result_message(data: dict) -> str:
    status = str(data.get("status", "") or "").upper()
    media_type = _format_media_type(str(data.get("type", "") or ""))
    name = escape(_truncate_text(str(data.get("name", "") or "-"), _SUBSCRIPTION_NAME_LIMIT))
    tmdb_id = escape(str(data.get("tmdbId", "") or "-"))
    season = int(data.get("season", 0) or 0)
    reject_reason = str(data.get("rejectReason", "") or "").strip()
    reviewed_at = _format_expiry(data.get("reviewedAt"))
    ingested_at = _format_expiry(data.get("ingestedAt"))

    if status == "APPROVED":
        title = "✅ <b>订阅已审核通过</b>"
        status_line = "当前状态：已通过，等待入库"
    elif status == "REJECTED":
        title = "❌ <b>订阅已被拒绝</b>"
        status_line = "当前状态：已拒绝"
    elif status == "INGESTED":
        title = "📦 <b>订阅内容已入库</b>"
        status_line = "当前状态：已入库"
    else:
        title = "ℹ️ <b>订阅状态更新</b>"
        status_line = f"当前状态：{escape(status or '-')}"

    lines = [
        title,
        "",
        f"📌 <b>{name}</b>",
        f"🎭 类型：{media_type}",
        f"🔗 TMDB：<a href='https://www.themoviedb.org/{'movie' if data.get('type') == 'MOVIE' else 'tv'}/{tmdb_id}'>#{tmdb_id}</a>",
    ]

    if season > 0:
        lines.append(f"📺 季：第 {season} 季")

    lines.extend(["", status_line])

    if status in ("APPROVED", "REJECTED") and reviewed_at != "永不过期":
        lines.append(f"🕒 审核时间：{reviewed_at}")
    if status == "REJECTED" and reject_reason:
        prefix = "📝 拒绝原因："
        base_text = "\n".join(lines + [prefix])
        reason_budget = max(0, TELEGRAM_CAPTION_LIMIT - _html_text_length(base_text))
        lines.append(f"{prefix}{_escape_truncated_text(reject_reason, min(_RESULT_REASON_LIMIT, reason_budget))}")
    if status == "INGESTED" and ingested_at != "永不过期":
        lines.append(f"📥 入库时间：{ingested_at}")

    return clamp_telegram_html("\n".join(lines), is_caption=True)


def format_bind_success(data: dict) -> str:
    username = escape(str(data.get("username", "") or ""))
    return clamp_telegram_html(
        (
        "✅ <b>绑定成功</b>\n\n"
        f"👤 已绑定账号：<b>{username}</b>"
        )
    )


def format_account_info(data: dict) -> str:
    username = escape(str(data.get("username", "") or ""))
    email = escape(str(data.get("email", "") or "-"))
    is_expired = bool(data.get("isExpired", False))
    is_active = bool(data.get("isActive", True))
    emby_disabled = bool(data.get("embyDisabled", False))
    expires_at = str(data.get("expiresAt", "") or "")

    expires_display = _format_expiry(expires_at, date_only=True) if expires_at else "永久有效"

    if not is_expired and is_active and not emby_disabled:
        status_emoji = "🟢"
        status_text = "正常"
    elif is_expired:
        status_emoji = "🔴"
        status_text = "已过期"
    else:
        status_emoji = "🔴"
        status_text = "已禁用"

    lines = [
        "📋 <b>账号信息</b>",
        "",
        f"👤 用户名：<b>{username}</b>",
        f"📧 邮箱：{email}",
        f"{status_emoji} 状态：{status_text}",
        f"⏳ 有效期至：{expires_display}",
    ]

    if is_expired:
        lines.append("")
        lines.append("💡 使用 /redeem <code>兑换码</code> 续期")

    return clamp_telegram_html("\n".join(lines))


def format_redeem_success(data: dict) -> str:
    days = int(data.get("days", 0) or 0)
    expires_at = str(data.get("expiresAt", "") or "")
    expires_display = _format_expiry(expires_at, date_only=True) if expires_at else "-"

    return clamp_telegram_html(
        (
        "🎉 <b>兑换成功</b>\n\n"
        f"📅 续期天数：<b>{days}</b> 天\n"
        f"⏳ 新到期时间：{expires_display}"
        )
    )


def _format_duration(seconds: int) -> str:
    """将秒数格式化为可读时长"""
    if seconds <= 0:
        return "0m"

    minutes = (seconds + 59) // 60
    hours = minutes // 60
    minutes = minutes % 60
    if hours > 24:
        days = hours // 24
        hours = hours % 24
        return f"{days}天{hours}h{minutes}m"
    if hours > 0:
        return f"{hours}h{minutes}m"
    return f"{minutes}m"


def format_ranking_message(data: dict) -> str:
    """格式化排行榜消息"""
    period = data.get("period", "daily")
    title = "日榜" if period == "daily" else "周榜"
    period_start = str(data.get("periodStart", "") or "")
    period_end = str(data.get("periodEnd", "") or "")
    cutoff_at = str(data.get("cutoffAt", "") or "").strip()
    total_duration = int(data.get("totalDuration", 0) or 0)

    date_line = (
        f"📅 {escape(period_start)}"
        if period_start == period_end
        else f"📅 {escape(period_start)} ~ {escape(period_end)}"
    )
    if cutoff_at:
        date_line = f"{date_line} 截至 {escape(cutoff_at)}"

    lines: list[str] = [
        f"🏆 <b>Ember 播放{title}</b>",
        date_line,
        f"⏱ <b>总播放时长</b>：{escape(_format_duration(total_duration))}",
        "",
    ]

    medals = {1: "🥇", 2: "🥈", 3: "🥉"}

    movies = data.get("movies", []) or []
    if movies:
        lines.append("🎬 <b>电影 TOP 10</b>")
        for item in movies:
            rank = int(item.get("rank", 0) or 0)
            medal = medals.get(rank, f"{rank}.")
            name = escape(str(item.get("name", "") or ""))
            duration = _format_duration(int(item.get("duration", 0) or 0))
            count = int(item.get("count", 0) or 0)
            lines.append(f"  {medal} {name}  ⏱{duration}  ▶{count}次")
        lines.append("")

    episodes = data.get("episodes", []) or []
    if episodes:
        lines.append("📺 <b>剧集 TOP 10</b>")
        for item in episodes:
            rank = int(item.get("rank", 0) or 0)
            medal = medals.get(rank, f"{rank}.")
            name = escape(str(item.get("name", "") or ""))
            duration = _format_duration(int(item.get("duration", 0) or 0))
            count = int(item.get("count", 0) or 0)
            lines.append(f"  {medal} {name}  ⏱{duration}  ▶{count}次")

    if not movies and not episodes:
        lines.append("📭 暂无播放数据")

    return clamp_telegram_html("\n".join(lines))


def format_search_results(
    results: list[dict],
    query: str,
    *,
    is_caption: bool = True,
) -> tuple[str, InlineKeyboardMarkup]:
    """格式化搜索结果列表，并按目标载荷类型控制字段预算。"""
    limit = TELEGRAM_CAPTION_LIMIT if is_caption else TELEGRAM_TEXT_LIMIT
    query_limit = _SEARCH_QUERY_LIMIT_CAPTION if is_caption else _SEARCH_QUERY_LIMIT_TEXT

    lines = [
        f"🔍 搜索 <b>{_escape_truncated_text(query, query_limit)}</b> 的结果：",
        "",
    ]
    row_count = max(len(results), 1)
    header_budget = _html_text_length("\n".join(lines))
    row_budget = max(24, (limit - header_budget) // row_count)
    title_budget = max(8, min(56 if is_caption else 120, row_budget - 20))
    original_budget = max(0, min(32 if is_caption else 80, row_budget - title_budget - 18))

    for i, item in enumerate(results):
        raw_title = str(item.get("title", ""))
        title = _escape_truncated_text(raw_title, title_budget)
        original_title = str(item.get("originalTitle", "") or "")
        tmdb_id = item.get("id", "")
        release_date = str(item.get("releaseDate", "") or "")
        year = release_date[:4] if len(release_date) >= 4 else ""
        item_media_type = item.get("mediaType", "movie")
        if item_media_type == "movie":
            type_emoji = "🎬"
            type_label = "电影"
            tmdb_path = "movie"
        else:
            type_emoji = "📺"
            type_label = "电视剧"
            tmdb_path = "tv"

        line = f"<b>{i + 1}.</b> <a href='https://www.themoviedb.org/{tmdb_path}/{tmdb_id}'>{title}</a>"
        if year:
            line += f" ({year})"
        line += f" {type_emoji} {type_label}"
        if original_budget > 0 and original_title and original_title != raw_title:
            line += f" - {_escape_truncated_text(original_title, original_budget)}"
        lines.append(line)

    buttons: list[list[InlineKeyboardButton]] = []
    row: list[InlineKeyboardButton] = []
    for i in range(len(results)):
        row.append(InlineKeyboardButton(str(i + 1), callback_data=f"sub:pick:{i}"))
        if len(row) == 4:
            buttons.append(row)
            row = []
    if row:
        buttons.append(row)

    return clamp_telegram_html("\n".join(lines), is_caption=is_caption), InlineKeyboardMarkup(buttons)


def format_search_detail(item: dict, selected_season: int | None = None) -> str:
    """格式化选中结果的详情"""
    raw_title = str(item.get("title", ""))
    title = _escape_truncated_text(raw_title, _SUBSCRIPTION_NAME_LIMIT)
    original_title = str(item.get("originalTitle", "") or "")
    tmdb_id = item.get("id", "")
    release_date = str(item.get("releaseDate", "") or "")
    year = release_date[:4] if len(release_date) >= 4 else ""
    overview = str(item.get("overview", "") or "")
    item_media_type = item.get("mediaType", "movie")
    if item_media_type == "movie":
        type_label = "电影"
        tmdb_path = "movie"
    elif item_media_type == "tv":
        type_label = "电视剧"
        tmdb_path = "tv"
    else:
        type_label = "未知"
        tmdb_path = "movie"

    if len(overview) > _SEARCH_OVERVIEW_LIMIT:
        overview = _truncate_text(overview, _SEARCH_OVERVIEW_LIMIT)

    lines = [f"📌 <b>{title}</b>"]
    if original_title and original_title != raw_title:
        lines.append(f"   {_escape_truncated_text(original_title, _SUBSCRIPTION_NAME_LIMIT)}")
    lines.append(f"🎭 类型：{type_label}")
    if year:
        lines.append(f"📅 年份：{year}")
    lines.append(
        f"🔗 <a href='https://www.themoviedb.org/{tmdb_path}/{tmdb_id}'>TMDB #{tmdb_id}</a>"
    )
    if item_media_type == "tv" and selected_season:
        lines.append(f"📺 已选季：第 {selected_season} 季")
    if overview:
        lines.append("")
        lines.append(escape(overview))

    return clamp_telegram_html("\n".join(lines), is_caption=True)


def make_movie_detail_keyboard() -> InlineKeyboardMarkup:
    """电影详情页的操作按钮"""
    return InlineKeyboardMarkup([
        [
            InlineKeyboardButton("✅ 订阅", callback_data="sub:ok"),
        ],
        [
            InlineKeyboardButton("🔙 返回", callback_data="sub:back"),
        ],
    ])


def make_tv_season_keyboard(seasons: list[int]) -> InlineKeyboardMarkup:
    """电视剧选季页按钮"""
    buttons: list[list[InlineKeyboardButton]] = []
    row: list[InlineKeyboardButton] = []
    for season in seasons:
        row.append(
            InlineKeyboardButton(
                f"第{season}季",
                callback_data=f"sub:season:{season}",
            )
        )
        if len(row) == 3:
            buttons.append(row)
            row = []
    if row:
        buttons.append(row)
    buttons.append([InlineKeyboardButton("🔙 返回", callback_data="sub:back")])
    return InlineKeyboardMarkup(buttons)


def make_tv_confirm_keyboard(selected_season: int) -> InlineKeyboardMarkup:
    """电视剧确认页按钮"""
    return InlineKeyboardMarkup([
        [
            InlineKeyboardButton(
                f"✅ 订阅第 {selected_season} 季",
                callback_data="sub:ok",
            ),
        ],
        [
            InlineKeyboardButton("🔙 重新选季", callback_data="sub:back:season"),
            InlineKeyboardButton("↩ 返回结果", callback_data="sub:back"),
        ],
    ])
