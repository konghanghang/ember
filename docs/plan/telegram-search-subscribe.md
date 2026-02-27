# Telegram 影视搜索与订阅

## 背景

用户绑定 Telegram 后，查看信息、兑换、改密均可在 Bot 内完成，唯独求片订阅需要切回网页。本功能补齐这个缺口，让用户在 Telegram 内完成「搜索 → 选择 → 订阅」的完整流程。

交互模式借鉴 MoviePilot：一条图片消息承载搜索结果（第一条结果的海报 + 编号列表 + TMDB 链接），底部数字按钮快速选择，选中后展示详情并可一键订阅或添加备注。

## UX 流程

```
1. /search 搏击俱乐部
   ↓
2. Bot 发送图片消息：
   ┌─────────────────────────┐
   │     [第一条结果的海报]     │
   ├─────────────────────────┤
   │ 🔍 搜索 "搏击俱乐部" 的  │
   │    电影结果：              │
   │                           │
   │ 1. 搏击俱乐部 (1999)      │
   │    - Fight Club            │
   │ 2. ...                     │
   ├─────────────────────────┤
   │ [1] [2] [3] [4]          │
   │ [📺 搜索电视剧]           │
   └─────────────────────────┘
   ↓
3. 用户点击数字按钮 → 编辑消息为详情页：
   ┌─────────────────────────┐
   │     [选中结果的海报]       │
   ├─────────────────────────┤
   │ 📌 搏击俱乐部             │
   │    Fight Club              │
   │ 🎭 类型：电影              │
   │ 📅 年份：1999              │
   │ 🔗 TMDB #550              │
   │                            │
   │ 一个失眠症患者和...        │
   ├─────────────────────────┤
   │ [✅ 订阅] [📝 添加备注]    │
   │ [🔙 返回]                  │
   └─────────────────────────┘
   ↓
4a. 点击 [✅ 订阅] → 创建订阅 → 显示成功
4b. 点击 [📝 添加备注] → Bot 提示输入 → 用户发送文本 → 创建订阅
4c. 点击 [🔙 返回] → 编辑回结果列表
```

---

## 实现顺序

1. Go API：新增 Request + Service + Handler + 路由 → `go build ./...` 编译验证
2. Bot：新建 `search_cache.py`
3. Bot：`api_client.py` 新增 2 个方法
4. Bot：`config.py` 新增常量
5. Bot：`message_formatter.py` 新增 3 个格式化函数 + 精简 `format_bind_success`
6. Bot：`telegram_handler.py` 新增所有 handler
7. Bot：`server.py` 拆分 CallbackQueryHandler + 注册新 handlers + 命令菜单
8. 更新 `docs/SYSTEM-ARCHITECTURE.md`

---

## 一、Go API — 新增内部端点

共修改 3 个文件，新增 1 个 Request 结构体、1 个 Service 方法、1 个 Handler 方法、1 行路由注册。

### 1.1 新增 Request 结构体 + Service 方法

**文件**：`services/api/internal/services/telegram.go`

在文件末尾 `CleanupExpiredBindCodes` 方法之前（约第 249 行），插入以下内容：

```go
// TelegramSubscribeRequest Bot 调 Internal API 创建求片订阅
type TelegramSubscribeRequest struct {
	TelegramID int64  `json:"telegramId" binding:"required"`
	Type       string `json:"type" binding:"required"`
	Name       string `json:"name" binding:"required"`
	TmdbID     string `json:"tmdbId" binding:"required"`
	PosterPath string `json:"posterPath"`
	Note       string `json:"note"`
}

// SubscribeByTelegram 通过 Telegram 身份创建求片订阅
func (s *TelegramService) SubscribeByTelegram(req TelegramSubscribeRequest) error {
	// 按 telegramId 查找用户（与 RedeemByTelegram 相同模式）
	var user models.User
	if err := db.DB.Where("\"telegramId\" = ?", req.TelegramID).First(&user).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrTelegramNotBound
		}
		return errors.New("订阅失败，请稍后重试")
	}

	// 构造 CreateSubscriptionRequest（PosterPath/Note 为 *string 指针）
	var posterPath *string
	if req.PosterPath != "" {
		posterPath = &req.PosterPath
	}
	var note *string
	if req.Note != "" {
		note = &req.Note
	}

	// 委托给 SubscriptionService（复用去重逻辑 + 通知 Bot）
	subService := NewSubscriptionService()
	return subService.CreateSubscription(user.ID, CreateSubscriptionRequest{
		Type:       models.MediaType(req.Type),
		Name:       req.Name,
		TmdbID:     req.TmdbID,
		PosterPath: posterPath,
		Note:       note,
	})
}
```

**复用说明**：
- 用户查找模式与 `RedeemByTelegram`（第 209-220 行）完全一致
- `CreateSubscriptionRequest` 定义在 `services/subscription.go:27`
- `ErrTelegramNotBound` 定义在 `services/errors.go:25`
- `ErrSubscriptionDuplicated` 定义在 `services/errors.go:12`，由 `CreateSubscription` 内部返回
- `CreateSubscription` 内部已包含全局去重检查和火忘式 Bot 通知

### 1.2 新增 Handler 方法

**文件**：`services/api/internal/handlers/telegram.go`

在文件末尾（`ResetPassword` 方法之后，约第 161 行），插入以下内容：

```go
// SubscribeByTelegram Bot 通过 Telegram 创建求片订阅
func (h *TelegramHandler) SubscribeByTelegram(c *gin.Context) {
	var req services.TelegramSubscribeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数错误"})
		return
	}

	if err := h.telegramService.SubscribeByTelegram(req); err != nil {
		statusCode := http.StatusInternalServerError
		switch {
		case errors.Is(err, services.ErrTelegramNotBound):
			statusCode = http.StatusBadRequest
		case errors.Is(err, services.ErrSubscriptionDuplicated):
			statusCode = http.StatusConflict
		}
		c.JSON(statusCode, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "订阅创建成功"})
}
```

**模式说明**：与 `RedeemByTelegram`（第 117-141 行）结构一致。错误映射：`ErrTelegramNotBound` → 400，`ErrSubscriptionDuplicated` → 409，其他 → 500。

### 1.3 注册路由

**文件**：`services/api/cmd/server/main.go`

在第 138 行（`internal.POST("/telegram/reset-password", telegramHandler.ResetPassword)`）之后新增一行：

```go
			internal.POST("/telegram/subscribe", telegramHandler.SubscribeByTelegram)
```

**API 契约**：

```
POST /api/v1/internal/telegram/subscribe
Header: X-Internal-Secret: {INTERNAL_API_SECRET}
Content-Type: application/json

Request Body:
{
  "telegramId": 123456789,     // int64, required
  "type": "MOVIE",             // string, required, "MOVIE" 或 "TV"
  "name": "搏击俱乐部",         // string, required
  "tmdbId": "550",             // string, required
  "posterPath": "/xxx.jpg",    // string, optional
  "note": "用户备注"            // string, optional
}

Success Response (200):
{"message": "订阅创建成功"}

Error Responses:
400: {"error": "尚未绑定 Telegram 账号"}
400: {"error": "请求参数错误"}
409: {"error": "该影片已提交订阅，请勿重复提交"}
500: {"error": "订阅失败，请稍后重试"}
```

---

## 二、Bot — 搜索状态缓存（新建文件）

**文件**：`services/bot/app/handlers/search_cache.py`（新建）

```python
"""每用户搜索会话缓存，TTL 自动清理"""

import time
from dataclasses import dataclass, field
from threading import Lock


@dataclass
class SearchSession:
    """单个用户的搜索会话"""

    results: list[dict]  # TMDB 搜索结果列表（最多 8 条）
    media_type: str  # "movie" 或 "tv"
    query: str  # 原始搜索关键词
    selected_index: int = -1  # 用户选中的结果索引（-1 = 未选中）
    waiting_for_note: bool = False  # 是否在等待用户输入备注
    message_id: int = 0  # Bot 发送的搜索结果消息 ID（用于 edit_message）
    chat_id: int = 0  # 会话 chat_id
    created_at: float = field(default_factory=time.time)


# 全局缓存：telegram_user_id -> SearchSession
_cache: dict[int, SearchSession] = {}
_lock = Lock()

# 会话 TTL：10 分钟
SESSION_TTL = 600


def get_session(user_id: int) -> SearchSession | None:
    """获取用户的搜索会话，过期则返回 None"""
    with _lock:
        session = _cache.get(user_id)
        if session is None:
            return None
        if time.time() - session.created_at > SESSION_TTL:
            del _cache[user_id]
            return None
        return session


def set_session(user_id: int, session: SearchSession) -> None:
    """设置用户的搜索会话（覆盖旧会话），同时惰性清理过期条目"""
    with _lock:
        _cleanup_expired()
        _cache[user_id] = session


def delete_session(user_id: int) -> None:
    """删除用户的搜索会话"""
    with _lock:
        _cache.pop(user_id, None)


def _cleanup_expired() -> None:
    """惰性清理：每次 set 时顺便清理过期条目，避免内存泄漏"""
    now = time.time()
    expired = [uid for uid, s in _cache.items() if now - s.created_at > SESSION_TTL]
    for uid in expired:
        del _cache[uid]
```

**设计说明**：
- `Lock` 足够：Bot 是单进程 asyncio，锁竞争极低
- 惰性清理：不需要后台定时器，每次 `set_session` 时顺便清理
- 用户发起新搜索时，旧会话自然被覆盖
- `SearchSession.message_id` 用于后续 `edit_message_media/caption` 操作

---

## 三、Bot — API Client 新增方法

**文件**：`services/bot/app/clients/api_client.py`

在文件末尾（`get_setting` 函数之后，约第 122 行）追加以下两个函数：

```python
async def search_tmdb(query: str, media_type: str = "movie") -> Optional[dict]:
    """调用公开 TMDB 搜索 API（无需鉴权，直接 GET）

    返回格式（成功时）：
    {
        "results": [
            {
                "id": 550,
                "title": "搏击俱乐部",
                "originalTitle": "Fight Club",
                "overview": "...",
                "posterPath": "/xxx.jpg",   # 可能为 null
                "releaseDate": "1999-10-15", # 可能为 null
                "mediaType": "movie"
            },
            ...
        ],
        "total": 42
    }

    返回格式（失败时）：{"error": "错误信息"}
    返回 None 表示网络异常
    """
    url = f"{API_URL}/api/v1/tmdb/search"
    params = {"query": query, "type": media_type}

    try:
        async with httpx.AsyncClient(timeout=10.0) as client:
            resp = await client.get(url, params=params)
        if resp.status_code == 200:
            return resp.json()
        return {"error": resp.json().get("error", "搜索失败")}
    except Exception:
        return None


async def subscribe_by_telegram(
    telegram_id: int,
    media_type: str,
    name: str,
    tmdb_id: str,
    poster_path: str = "",
    note: str = "",
) -> Optional[dict]:
    """通过 Telegram 身份创建求片订阅

    media_type: "MOVIE" 或 "TV"（大写，与 Go API 一致）
    返回格式（成功时）：{"message": "订阅创建成功"}
    返回格式（失败时）：{"error": "错误信息"}
    返回 None 表示网络异常
    """
    url = f"{API_URL}/api/v1/internal/telegram/subscribe"
    headers = {"X-Internal-Secret": INTERNAL_API_SECRET}

    payload = {
        "telegramId": telegram_id,
        "type": media_type,
        "name": name,
        "tmdbId": str(tmdb_id),
        "posterPath": poster_path,
        "note": note,
    }

    try:
        async with httpx.AsyncClient(timeout=10.0) as client:
            resp = await client.post(url, headers=headers, json=payload)
        if resp.status_code == 200:
            return resp.json()
        return {"error": resp.json().get("error", "订阅失败")}
    except Exception:
        return None
```

**模式说明**：`subscribe_by_telegram` 与 `redeem_by_telegram`（第 66-84 行）结构一致。`search_tmdb` 走公开 API 无需 `X-Internal-Secret`。

---

## 四、Bot — Config 新增常量

**文件**：`services/bot/app/config.py`

在第 21 行（`TMDB_IMAGE_BASE = "https://image.tmdb.org/t/p/w300"`）之后新增：

```python
TMDB_IMAGE_BASE_W500 = "https://image.tmdb.org/t/p/w500"
```

说明：`w300` 用于管理员通知（小图），`w500` 用于用户搜索结果展示（清晰大图）。

---

## 五、Bot — 消息格式化

**文件**：`services/bot/app/formatters/message_formatter.py`

### 5.1 新增 import

在文件顶部第 3 行（`from telegram import InlineKeyboardButton, InlineKeyboardMarkup`）修改为：

```python
from telegram import InlineKeyboardButton, InlineKeyboardMarkup, InputMediaPhoto
```

### 5.2 精简 `format_bind_success`

将第 82-90 行的 `format_bind_success` 函数替换为：

```python
def format_bind_success(data: dict) -> str:
    username = escape(str(data.get("username", "") or ""))
    return (
        "✅ <b>绑定成功</b>\n\n"
        f"👤 已绑定账号：<b>{username}</b>"
    )
```

去掉命令列表，命令发现由 Bot 菜单统一承担。

### 5.3 新增 `format_search_results`

在文件末尾追加：

```python
def format_search_results(
    results: list[dict],
    media_type: str,
    query: str,
) -> tuple[str, InlineKeyboardMarkup]:
    """格式化搜索结果列表

    返回 (caption_html, inline_keyboard)

    caption 示例：
        🔍 搜索 "搏击俱乐部" 的电影结果：

        1. 搏击俱乐部 (1999) - Fight Club
        2. 搏击俱乐部2 (2025)

    keyboard 示例：
        [1] [2] [3] [4]
        [5] [6] [7] [8]
        [📺 搜索电视剧]
    """
    type_label = "电影" if media_type == "movie" else "电视剧"
    tmdb_path = "movie" if media_type == "movie" else "tv"

    lines = [
        f"🔍 搜索 <b>{escape(query)}</b> 的{type_label}结果：",
        "",
    ]

    for i, item in enumerate(results):
        title = escape(str(item.get("title", "")))
        original_title = str(item.get("originalTitle", "") or "")
        tmdb_id = item.get("id", "")
        release_date = str(item.get("releaseDate", "") or "")
        year = release_date[:4] if len(release_date) >= 4 else ""

        # 格式：1. 搏击俱乐部 (1999) - Fight Club
        # 标题是 TMDB 超链接
        line = f"<b>{i + 1}.</b> <a href='https://www.themoviedb.org/{tmdb_path}/{tmdb_id}'>{title}</a>"
        if year:
            line += f" ({year})"
        if original_title and original_title != str(item.get("title", "")):
            line += f" - {escape(original_title)}"
        lines.append(line)

    # 构建数字按钮：每行 4 个
    buttons: list[list[InlineKeyboardButton]] = []
    row: list[InlineKeyboardButton] = []
    for i in range(len(results)):
        row.append(InlineKeyboardButton(str(i + 1), callback_data=f"sub:pick:{i}"))
        if len(row) == 4:
            buttons.append(row)
            row = []
    if row:
        buttons.append(row)

    # 底部：切换类型按钮
    toggle_label = "📺 搜索电视剧" if media_type == "movie" else "🎬 搜索电影"
    buttons.append([InlineKeyboardButton(toggle_label, callback_data="sub:type")])

    return "\n".join(lines), InlineKeyboardMarkup(buttons)
```

### 5.4 新增 `format_search_detail`

```python
def format_search_detail(item: dict, media_type: str) -> str:
    """格式化选中结果的详情（用于 edit_message_caption）

    输出示例：
        📌 搏击俱乐部
           Fight Club
        🎭 类型：电影
        📅 年份：1999
        🔗 TMDB #550

        一个失眠症患者和一个肥皂销售员...
    """
    title = escape(str(item.get("title", "")))
    original_title = str(item.get("originalTitle", "") or "")
    tmdb_id = item.get("id", "")
    release_date = str(item.get("releaseDate", "") or "")
    year = release_date[:4] if len(release_date) >= 4 else ""
    overview = str(item.get("overview", "") or "")
    type_label = "电影" if media_type == "movie" else "电视剧"
    tmdb_path = "movie" if media_type == "movie" else "tv"

    # Telegram photo caption 上限 1024 字符，预留空间给标题/元信息
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
    if overview:
        lines.append("")
        lines.append(escape(overview))

    return "\n".join(lines)
```

### 5.5 新增 `make_detail_keyboard`

```python
def make_detail_keyboard() -> InlineKeyboardMarkup:
    """详情页的操作按钮

    布局：
        [✅ 订阅] [📝 添加备注]
        [🔙 返回]
    """
    return InlineKeyboardMarkup([
        [
            InlineKeyboardButton("✅ 订阅", callback_data="sub:ok"),
            InlineKeyboardButton("📝 添加备注", callback_data="sub:note"),
        ],
        [
            InlineKeyboardButton("🔙 返回", callback_data="sub:back"),
        ],
    ])
```

---

## 六、Bot — Handler 实现

**文件**：`services/bot/app/handlers/telegram_handler.py`

### 6.1 修改 import 区域

将文件顶部（第 1-18 行）替换为：

```python
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
```

新增内容：`InputMediaPhoto`, `TMDB_IMAGE_BASE_W500`, 三个搜索格式化函数, `search_cache` 模块。

### 6.2 新增 `handle_search` 命令处理

在 `handle_resetpw` 函数之后（第 253 行之后），追加以下所有代码：

```python
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

    # 检查用户是否已绑定（复用 get_account_info 端点）
    info = await api_client.get_account_info(user_id)
    if info is None:
        await message.reply_text("❌ 服务暂不可用，请稍后重试")
        return
    if "error" in info:
        await message.reply_text(
            "❌ 请先绑定 Telegram 账号后再使用搜索功能\n\n"
            "使用 /bind <code>验证码</code> 绑定",
            parse_mode="HTML",
        )
        return

    # 默认搜索电影
    await _do_search(message, user_id, query, "movie")
```

### 6.3 新增 `_do_search` 内部函数

```python
async def _do_search(message, user_id: int, query: str, media_type: str) -> None:
    """执行 TMDB 搜索并发送结果（复用于首次搜索和切换类型）

    message: 可以是 Update.message 或 CallbackQuery.message
    """
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

    # 最多取 8 条结果
    results = all_results[:8]

    # 构建搜索会话
    session = SearchSession(
        results=results,
        media_type=media_type,
        query=query,
    )

    # 格式化消息
    caption, keyboard = format_search_results(results, media_type, query)

    # 尝试发送带海报的图片消息
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

    # 无海报或发送失败：退化为纯文本消息
    sent = await message.reply_text(
        text=caption,
        parse_mode="HTML",
        reply_markup=keyboard,
        disable_web_page_preview=True,
    )
    session.message_id = sent.message_id
    session.chat_id = sent.chat_id
    set_session(user_id, session)
```

### 6.4 新增 `handle_search_callback` 回调处理

```python
async def handle_search_callback(
    update: Update, context: ContextTypes.DEFAULT_TYPE
) -> None:
    """处理所有 sub: 前缀的 callback query

    路由规则：
    - sub:pick:{index} → 选中搜索结果
    - sub:type         → 切换电影/电视剧
    - sub:ok           → 确认订阅
    - sub:note         → 请求输入备注
    - sub:back         → 返回搜索列表
    """
    del context
    query = update.callback_query
    if query is None or query.data is None or query.from_user is None:
        return

    await query.answer()
    user_id = query.from_user.id
    data = query.data

    session = get_session(user_id)
    if session is None:
        await query.answer("搜索已过期，请重新搜索", show_alert=True)
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
```

### 6.5 新增 `_handle_pick` — 选中搜索结果

```python
async def _handle_pick(query, session: SearchSession, user_id: int, data: str) -> None:
    """用户点击编号按钮，编辑消息为详情页"""
    try:
        index = int(data.split(":")[-1])
    except (ValueError, IndexError):
        return

    if index < 0 or index >= len(session.results):
        return

    session.selected_index = index
    session.waiting_for_note = False
    set_session(user_id, session)

    item = session.results[index]
    caption = format_search_detail(item, session.media_type)
    keyboard = make_detail_keyboard()

    # 尝试编辑消息媒体（切换海报）
    poster_path = item.get("posterPath")
    if poster_path and query.message and query.message.photo:
        poster_url = f"{TMDB_IMAGE_BASE_W500}{poster_path}"
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
            logger.exception("编辑海报失败，降级为编辑文本")

    # 降级：只编辑文本/caption
    if query.message and query.message.photo:
        await query.edit_message_caption(
            caption=caption, parse_mode="HTML", reply_markup=keyboard
        )
    else:
        await query.edit_message_text(
            text=caption,
            parse_mode="HTML",
            reply_markup=keyboard,
            disable_web_page_preview=True,
        )
```

### 6.6 新增 `_handle_toggle_type` — 切换搜索类型

```python
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

    # 尝试编辑消息媒体（切换海报）
    if first_poster and query.message and query.message.photo:
        poster_url = f"{TMDB_IMAGE_BASE_W500}{first_poster}"
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

    # 降级
    if query.message and query.message.photo:
        await query.edit_message_caption(
            caption=caption, parse_mode="HTML", reply_markup=keyboard
        )
    else:
        await query.edit_message_text(
            text=caption,
            parse_mode="HTML",
            reply_markup=keyboard,
            disable_web_page_preview=True,
        )
    set_session(user_id, new_session)
```

### 6.7 新增 `_handle_subscribe` — 确认订阅

```python
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
        await query.answer(str(result["error"]), show_alert=True)
        return

    # 成功：编辑消息显示结果，清除 session
    title = escape(str(item.get("title", "")))
    success_text = (
        f"✅ <b>订阅成功</b>\n\n"
        f"📌 {title}\n\n"
        "已提交求片请求，请等待管理员审核。"
    )
    delete_session(user_id)

    if query.message and query.message.photo:
        await query.edit_message_caption(
            caption=success_text, parse_mode="HTML", reply_markup=None
        )
    else:
        await query.edit_message_text(text=success_text, parse_mode="HTML")
```

### 6.8 新增 `_handle_request_note` — 请求备注输入

```python
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
    if query.message and query.message.photo:
        await query.edit_message_caption(
            caption=text, parse_mode="HTML", reply_markup=None
        )
    else:
        await query.edit_message_text(text=text, parse_mode="HTML")
```

### 6.9 新增 `_handle_back` — 返回搜索列表

```python
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

    if first_poster and query.message and query.message.photo:
        poster_url = f"{TMDB_IMAGE_BASE_W500}{first_poster}"
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

    if query.message and query.message.photo:
        await query.edit_message_caption(
            caption=caption, parse_mode="HTML", reply_markup=keyboard
        )
    else:
        await query.edit_message_text(
            text=caption,
            parse_mode="HTML",
            reply_markup=keyboard,
            disable_web_page_preview=True,
        )
```

### 6.10 新增 `handle_text_message` — 接收备注输入

```python
async def handle_text_message(
    update: Update, context: ContextTypes.DEFAULT_TYPE
) -> None:
    """处理私聊文本消息（仅用于接收备注输入）

    仅在用户有 waiting_for_note=True 的搜索会话时处理，
    否则静默忽略（不干扰其他消息流）。
    """
    del context
    message = update.message
    if message is None or message.from_user is None or message.text is None:
        return

    user_id = message.from_user.id
    session = get_session(user_id)

    # 没有等待备注的会话，静默忽略
    if session is None or not session.waiting_for_note:
        return

    if session.selected_index < 0 or session.selected_index >= len(session.results):
        delete_session(user_id)
        return

    note = message.text.strip()

    # /cancel 取消备注输入
    if note.lower() == "/cancel":
        session.waiting_for_note = False
        set_session(user_id, session)
        await message.reply_text("已取消备注输入。你可以重新发起 /search 搜索。")
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

    delete_session(user_id)

    if result is None:
        await message.reply_text("❌ 服务暂不可用，请稍后重试")
        return
    if "error" in result:
        await message.reply_text(
            f"❌ {escape(str(result['error']))}", parse_mode="HTML"
        )
        return

    title = escape(str(item.get("title", "")))
    await message.reply_text(
        f"✅ <b>订阅成功</b>\n\n"
        f"📌 {title}\n"
        f"💬 备注：{escape(note)}\n\n"
        "已提交求片请求，请等待管理员审核。",
        parse_mode="HTML",
    )
```

---

## 七、Bot — Handler 注册 + 命令菜单

**文件**：`services/bot/app/server.py`

### 7.1 修改 import 区域

将第 25-35 行替换为：

```python
from app.handlers.telegram_handler import (
    handle_bind,
    handle_callback,
    handle_info,
    handle_new_member,
    handle_redeem,
    handle_resetpw,
    handle_search,
    handle_search_callback,
    handle_text_message,
    send_registration_notification,
    send_ranking_notification,
    send_subscription_notification,
)
```

新增 3 个 import：`handle_search`, `handle_search_callback`, `handle_text_message`。

### 7.2 修改 handler 注册（第 43-49 行）

将第 43-49 行替换为：

```python
tg_app = Application.builder().token(TELEGRAM_BOT_TOKEN).build()

# ⚠️ 关键改动：原来的 CallbackQueryHandler(handle_callback) 无 pattern，
# 会吃掉所有 callback query。现在拆分为两个带 pattern 的 handler。
tg_app.add_handler(CallbackQueryHandler(handle_callback, pattern=r"^(approve|reject):"))
tg_app.add_handler(CallbackQueryHandler(handle_search_callback, pattern=r"^sub:"))

tg_app.add_handler(MessageHandler(filters.StatusUpdate.NEW_CHAT_MEMBERS, handle_new_member))
tg_app.add_handler(CommandHandler("bind", handle_bind))
tg_app.add_handler(CommandHandler("info", handle_info))
tg_app.add_handler(CommandHandler("redeem", handle_redeem))
tg_app.add_handler(CommandHandler("resetpw", handle_resetpw))
tg_app.add_handler(CommandHandler("search", handle_search))

# 文本消息 handler 放在最后：仅处理 waiting_for_note 状态下的备注输入
# 匹配：私聊 + 纯文本 + 非命令
tg_app.add_handler(MessageHandler(
    filters.TEXT & ~filters.COMMAND & filters.ChatType.PRIVATE,
    handle_text_message,
))
```

**⚠️ 破坏性改动说明**：
- 第 44 行原来是 `CallbackQueryHandler(handle_callback)` 没有 `pattern` 参数
- 必须改为 `pattern=r"^(approve|reject):"` 否则它会吞掉 `sub:*` 回调
- 这是唯一影响现有功能的改动点，`approve:xxx` 和 `reject:xxx` 的回调行为完全不变

### 7.3 修改 `lifespan` 函数（第 76-94 行）

将第 76-94 行替换为：

```python
@asynccontextmanager
async def lifespan(app: FastAPI):
    del app
    await tg_app.initialize()
    await tg_app.start()
    logger.info("Telegram Bot 服务已启动，开始异步注册 webhook")

    # 注册 Bot 命令菜单
    from telegram import BotCommand

    await tg_app.bot.set_my_commands([
        BotCommand("search", "搜索影视"),
        BotCommand("bind", "绑定 Ember 账号"),
        BotCommand("info", "查看账号信息"),
        BotCommand("redeem", "兑换续期码"),
        BotCommand("resetpw", "重置密码"),
    ])
    logger.info("Bot 命令菜单已注册")

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
```

新增内容：`set_my_commands` 调用，放在 `tg_app.start()` 之后、webhook 注册之前。

---

## 八、更新架构文档

**文件**：`docs/SYSTEM-ARCHITECTURE.md`

需要更新以下几处：

1. **第 573-583 行 内部服务路由表**：新增一行
   ```
   | POST | `/api/v1/internal/telegram/subscribe` | Bot 创建求片订阅 |
   ```

2. **第 672-677 行 Bot 命令与处理器**：新增 `/search` 命令说明
   ```
   - **Commands**：`/search`（搜索影视并订阅）、`/bind`（绑定账号）、`/info`（查看账号信息）、`/redeem`（兑换续期码）、`/resetpw`（重置密码）
   ```

3. **Bot 文件结构**（第 126-138 行）：新增 `search_cache.py`
   ```
      ├─ handlers/
      │  ├─ telegram_handler.py  # 消息/回调处理
      │  └─ search_cache.py      # 搜索会话缓存
   ```

---

## Callback Data 格式一览

| callback_data | 含义 | handler | 最大长度 |
|---|---|---|---|
| `approve:{cuid}` | 审批通过（现有） | `handle_callback` | ~35 bytes |
| `reject:{cuid}` | 审批拒绝（现有） | `handle_callback` | ~34 bytes |
| `sub:pick:{0-7}` | 选中搜索结果 | `handle_search_callback` | 10 bytes |
| `sub:type` | 切换搜索类型 | `handle_search_callback` | 8 bytes |
| `sub:ok` | 确认订阅 | `handle_search_callback` | 6 bytes |
| `sub:note` | 请求输入备注 | `handle_search_callback` | 8 bytes |
| `sub:back` | 返回搜索列表 | `handle_search_callback` | 8 bytes |

Telegram callback_data 上限 64 bytes，所有格式均安全。

---

## 错误处理矩阵

| 场景 | 错误来源 | 处理方式 |
|------|----------|----------|
| 用户未绑定 | `get_account_info` 返回 `{"error": "..."}` | 提示先绑定 + 引导 `/bind` |
| TMDB 搜索无结果 | API 返回 `results: []` | 提示更换关键词 |
| TMDB API 不可用 | `search_tmdb` 返回 `None` | 提示稍后重试 |
| 搜索会话过期（10min） | `get_session` 返回 `None` | `callback.answer("搜索已过期")` |
| 重复订阅 | Go API 返回 409 + `ErrSubscriptionDuplicated` | `callback.answer` 显示错误 |
| 无海报（posterPath 为 null） | TMDB 数据缺失 | 退化为纯文本 `send_message` |
| 海报 URL 请求失败 | Telegram API `send_photo` 异常 | catch exception → 降级文本 |
| 编辑消息失败 | `edit_message_media` 异常 | catch exception → 降级 `edit_caption` |
| 用户连续搜索 | 同一用户快速发 `/search` | 新搜索覆盖旧 session |
| 备注输入时发送 /cancel | 用户主动取消 | 重置 `waiting_for_note`，提示已取消 |

---

## 涉及文件清单

| 文件 | 操作 | 改动量 |
|------|------|--------|
| `services/api/internal/services/telegram.go` | 修改 | +35 行（struct + method） |
| `services/api/internal/handlers/telegram.go` | 修改 | +25 行（handler） |
| `services/api/cmd/server/main.go` | 修改 | +1 行（路由） |
| `services/bot/app/handlers/search_cache.py` | **新建** | ~60 行 |
| `services/bot/app/clients/api_client.py` | 修改 | +50 行（2 个函数） |
| `services/bot/app/config.py` | 修改 | +1 行 |
| `services/bot/app/formatters/message_formatter.py` | 修改 | +90 行（3 个新函数 + 精简 1 个） |
| `services/bot/app/handlers/telegram_handler.py` | 修改 | +250 行（import + 10 个函数） |
| `services/bot/app/server.py` | 修改 | +20 行（import + handler 注册 + 菜单） |
| `docs/SYSTEM-ARCHITECTURE.md` | 修改 | 更新 3 处 |

---

## 验证方式

1. **Go API 编译验证**：
   ```bash
   cd services/api && go build ./...
   ```

2. **Bot import 验证**：
   ```bash
   cd services/bot && python -c "from app.server import app; print('ok')"
   ```

3. **部署后端到端测试**：
   - `/search 搏击俱乐部` → 确认显示海报 + 结果列表 + 按钮
   - 点击数字按钮 → 确认详情页正确（海报切换、标题、TMDB 链接）
   - 点击 [📺 搜索电视剧] → 确认切换类型并重新搜索
   - 点击 [🔙 返回] → 确认回到结果列表
   - 点击 [✅ 订阅] → 确认订阅创建成功 + 管理员收到审批通知
   - 点击 [📝 添加备注] → 输入文本 → 确认带备注订阅成功
   - 重复订阅同一影片 → 确认提示「该影片已提交订阅，请勿重复提交」
   - 未绑定用户执行 `/search` → 确认提示先绑定
   - 等待 10 分钟后点击按钮 → 确认提示「搜索已过期」
   - 搜索无结果的关键词 → 确认提示更换关键词
