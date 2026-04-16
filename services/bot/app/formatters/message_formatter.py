from html import escape

from telegram import InlineKeyboardButton, InlineKeyboardMarkup


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


def format_subscription_message(data: dict) -> tuple[str, InlineKeyboardMarkup]:
    media_type = _format_media_type(data.get("type", ""))
    name = escape(str(data.get("name", "")))
    user_name = escape(str(data.get("userName", "") or "-"))
    tmdb_id = escape(str(data.get("tmdbId", "")))
    season = int(data.get("season", 0) or 0)
    note = str(data.get("note", "") or "").strip()

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

    return "\n".join(lines), keyboard


def format_registration_message(data: dict) -> str:
    user_name = escape(str(data.get("userName", "") or "-"))
    email = escape(str(data.get("email", "") or "-"))
    emby_id = escape(str(data.get("embyId", "") or "-"))
    mode = escape(_format_registration_mode(str(data.get("registrationMode", ""))))
    expires_at = escape(str(data.get("expiresAt", "") or "永不过期"))

    lines = [
        "🆕 <b>新用户注册成功</b>",
        "",
        f"👤 用户名：{user_name}",
        f"📧 邮箱：{email}",
        f"🧾 Emby ID：<code>{emby_id}</code>",
        f"🛂 注册方式：{mode}",
        f"⏳ 到期时间：{expires_at}",
    ]
    return "\n".join(lines)


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


def _format_expiry(value: str | None) -> str:
    raw = str(value or "").strip()
    if raw == "":
        return "永不过期"
    return escape(raw.replace("T", " ")[:19])


def format_payment_message(data: dict) -> str:
    user_name = escape(str(data.get("userName", "") or "-"))
    email = escape(str(data.get("email", "") or "-"))
    plan_name = escape(str(data.get("planName", "") or "-"))
    amount = int(data.get("amount", 0) or 0)
    currency = str(data.get("currency", "") or "USD")
    days = int(data.get("days", 0) or 0)
    payment_id = escape(str(data.get("paymentId", "") or "-"))
    session_id = escape(str(data.get("stripeSessionId", "") or "-"))
    old_expires_at = _format_expiry(data.get("oldExpiresAt"))
    new_expires_at = _format_expiry(data.get("newExpiresAt"))

    lines = [
        "💰 <b>支付成功</b>",
        "",
        f"👤 用户：{user_name}",
        f"📧 邮箱：{email}",
        f"📦 方案：{plan_name}",
        f"💵 金额：{escape(_format_currency(amount, currency))}",
        f"📅 延长：<b>{days}</b> 天",
        f"⏳ 原到期：{old_expires_at}",
        f"✅ 新到期：{new_expires_at}",
        f"🧾 支付记录：<code>{payment_id}</code>",
        f"🔗 Session：<code>{session_id}</code>",
    ]

    return "\n".join(lines)


def format_result_message(original_text: str, action: str, reason: str | None = None) -> str:
    result = "✅ 已通过" if action == "approve" else "❌ 已拒绝"
    text = original_text.strip()
    if action == "reject" and reason:
        result = f"{result}\n📝 原因：{escape(reason.strip())}"
    return f"{text}\n\n────────────────────\n{result}"


def format_subscription_result_message(data: dict) -> str:
    status = str(data.get("status", "") or "").upper()
    media_type = _format_media_type(str(data.get("type", "") or ""))
    name = escape(str(data.get("name", "") or "-"))
    tmdb_id = escape(str(data.get("tmdbId", "") or "-"))
    season = int(data.get("season", 0) or 0)
    reject_reason = escape(str(data.get("rejectReason", "") or "").strip())
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
        lines.append(f"📝 拒绝原因：{reject_reason}")
    if status == "INGESTED" and ingested_at != "永不过期":
        lines.append(f"📥 入库时间：{ingested_at}")

    return "\n".join(lines)


def format_bind_success(data: dict) -> str:
    username = escape(str(data.get("username", "") or ""))
    return (
        "✅ <b>绑定成功</b>\n\n"
        f"👤 已绑定账号：<b>{username}</b>"
    )


def format_account_info(data: dict) -> str:
    username = escape(str(data.get("username", "") or ""))
    email = escape(str(data.get("email", "") or "-"))
    is_expired = bool(data.get("isExpired", False))
    is_active = bool(data.get("isActive", True))
    emby_disabled = bool(data.get("embyDisabled", False))
    expires_at = str(data.get("expiresAt", "") or "")

    expires_display = escape(expires_at[:10]) if expires_at else "永久有效"

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

    return "\n".join(lines)


def format_redeem_success(data: dict) -> str:
    days = int(data.get("days", 0) or 0)
    expires_at = str(data.get("expiresAt", "") or "")
    expires_display = escape(expires_at[:10]) if expires_at else "-"

    return (
        "🎉 <b>兑换成功</b>\n\n"
        f"📅 续期天数：<b>{days}</b> 天\n"
        f"⏳ 新到期时间：{expires_display}"
    )


def _format_duration(seconds: int) -> str:
    """将秒数格式化为可读时长"""
    if seconds <= 0:
        return "0m"

    hours = seconds // 3600
    minutes = (seconds % 3600) // 60
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

    return "\n".join(lines)


def format_search_results(
    results: list[dict],
    query: str,
) -> tuple[str, InlineKeyboardMarkup]:
    """格式化搜索结果列表（混合展示电影和电视剧）"""

    lines = [
        f"🔍 搜索 <b>{escape(query)}</b> 的结果：",
        "",
    ]

    for i, item in enumerate(results):
        title = escape(str(item.get("title", "")))
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
        if original_title and original_title != str(item.get("title", "")):
            line += f" - {escape(original_title)}"
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

    return "\n".join(lines), InlineKeyboardMarkup(buttons)


def format_search_detail(item: dict, selected_season: int | None = None) -> str:
    """格式化选中结果的详情"""
    title = escape(str(item.get("title", "")))
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

    if len(overview) > 300:
        overview = overview[:300] + "..."

    lines = [f"📌 <b>{title}</b>"]
    if original_title and original_title != str(item.get("title", "")):
        lines.append(f"   {escape(original_title)}")
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

    return "\n".join(lines)


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
