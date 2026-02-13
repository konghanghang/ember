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
    note = str(data.get("note", "") or "").strip()

    lines = [
        "🎬 <b>新的求片请求</b>",
        "",
        f"📌 <b>{name}</b>",
        f"🎭 类型：{media_type}",
        f"👤 用户：{user_name}",
        f"🔗 TMDB：<a href='https://www.themoviedb.org/{'movie' if data.get('type') == 'MOVIE' else 'tv'}/{tmdb_id}'>#{tmdb_id}</a>",
    ]

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


def format_result_message(original_text: str, action: str) -> str:
    result = "✅ 已通过" if action == "approve" else "❌ 已拒绝"
    text = original_text.strip()
    return f"{text}\n\n────────────────────\n{result}"
