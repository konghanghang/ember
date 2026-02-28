# Telegram 搜索改为电影+电视剧混合展示

## Context

这是对 `docs/plan/telegram-search-subscribe.md` 功能的优化补充。

**当前问题**：
- 用户 `/search 权力的游戏` → 默认只搜电影 → 没结果 → 必须手动点 `[📺 搜索电视剧]` 才能找到
- 对于不确定类型的内容，用户需要两步操作才能找到目标

**优化方案**：
- 改为一次搜索同时展示电影和电视剧结果
- 每条结果后标注类型（🎬 电影 / 📺 电视剧）
- 去掉底部的类型切换按钮
- 用户直接从混合列表中选择目标内容

**UX 对比**：

改前：
```
🔍 搜索 "搏击俱乐部" 的电影结果：

1. 搏击俱乐部 (1999) - Fight Club

[1] [2] [3] [4]
[📺 搜索电视剧]  ← 用户需要点这里切换
```

改后：
```
🔍 搜索 "搏击俱乐部" 的结果：

1. 搏击俱乐部 (1999) 🎬 电影 - Fight Club
2. 权力的游戏 (2011) 📺 电视剧 - Game of Thrones

[1] [2] [3] [4]
```

---

## 一、Go API — 支持 `multi` 类型搜索

### 1.1 修改 `TMDBSearchResult` 结构体

**文件**：`services/api/internal/handlers/tmdb.go`

在第 26-37 行，新增 `MediaType` 字段：

```go
// TMDBSearchResult TMDB 搜索结果
type TMDBSearchResult struct {
	ID            int     `json:"id"`
	Title         *string `json:"title"`
	Name          *string `json:"name"`
	OriginalTitle *string `json:"original_title"`
	OriginalName  *string `json:"original_name"`
	Overview      string  `json:"overview"`
	PosterPath    *string `json:"poster_path"`
	ReleaseDate   *string `json:"release_date"`
	FirstAirDate  *string `json:"first_air_date"`
	MediaType     *string `json:"media_type"` // 新增：TMDB multi 搜索返回的类型
}
```

**说明**：TMDB 的 `search/multi` 端点返回的每条结果带 `media_type` 字段（值为 `"movie"` 或 `"tv"`），需要在结构体中接收。

### 1.2 修改 `Search` 函数

将第 58-163 行的 `Search` 函数修改为：

```go
// Search TMDB 搜索 API 代理
// GET /api/v1/tmdb/search?query=xxx&type=multi
func (h *TMDBHandler) Search(c *gin.Context) {
	// 获取查询参数
	query := c.Query("query")
	mediaType := c.DefaultQuery("type", "movie") // 默认保持 movie，由调用方显式传 multi

	if query == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "缺少搜索关键词",
		})
		return
	}

	if h.apiKey == "" {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "TMDB API 未配置",
		})
		return
	}

	// 根据类型选择搜索端点
	endpoint := "search/multi"
	if mediaType == "movie" {
		endpoint = "search/movie"
	} else if mediaType == "tv" {
		endpoint = "search/tv"
	} else if mediaType != "multi" {
		// 非法 type 参数，返回 400
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "type 参数必须是 movie、tv 或 multi",
		})
		return
	}

	// 构建 TMDB API URL
	tmdbURL := fmt.Sprintf(
		"https://api.themoviedb.org/3/%s?api_key=%s&query=%s&language=zh-CN&page=1",
		endpoint,
		h.apiKey,
		url.QueryEscape(query),
	)

	// 调用 TMDB API
	resp, err := http.Get(tmdbURL)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "TMDB API 请求失败",
		})
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": fmt.Sprintf("TMDB API 返回错误: %d", resp.StatusCode),
		})
		return
	}

	// 解析响应
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "读取响应失败",
		})
		return
	}

	var tmdbResp TMDBApiResponse
	if err := json.Unmarshal(body, &tmdbResp); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "解析响应失败",
		})
		return
	}

	// 转换为统一格式
	results := make([]UnifiedSearchResult, 0, len(tmdbResp.Results))
	for _, item := range tmdbResp.Results {
		// multi 搜索时，从 TMDB 响应读取实际类型
		itemMediaType := mediaType
		if mediaType == "multi" && item.MediaType != nil {
			itemMediaType = *item.MediaType
		}

		// 过滤掉 person 类型（演员/导演等人物）
		if itemMediaType == "person" {
			continue
		}

		// 电影用 title，电视剧用 name
		title := ""
		if item.Title != nil {
			title = *item.Title
		} else if item.Name != nil {
			title = *item.Name
		}

		originalTitle := ""
		if item.OriginalTitle != nil {
			originalTitle = *item.OriginalTitle
		} else if item.OriginalName != nil {
			originalTitle = *item.OriginalName
		}

		releaseDate := item.ReleaseDate
		if releaseDate == nil {
			releaseDate = item.FirstAirDate
		}

		results = append(results, UnifiedSearchResult{
			ID:            item.ID,
			Title:         title,
			OriginalTitle: originalTitle,
			Overview:      item.Overview,
			PosterPath:    item.PosterPath,
			ReleaseDate:   releaseDate,
			MediaType:     itemMediaType,
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"results": results,
		"total":   tmdbResp.TotalResults,
	})
}
```

**关键变化**：
1. API 默认 `type` 保持 `"movie"` 不变，由 Bot 显式传 `"multi"`（避免破坏性变更）
2. 支持 `search/multi` 端点（同时搜索电影和剧集）
3. 新增 `type` 参数白名单校验，拒绝非法值
4. 从 TMDB 响应的 `media_type` 字段读取实际类型并填充到 `UnifiedSearchResult.MediaType`
5. 过滤掉 `person` 类型结果

---

## 二、Bot — 修改搜索和展示逻辑

### 2.1 修改 `_do_search` 过滤逻辑（关键修改）

**文件**：`services/bot/app/handlers/telegram_handler.py`

在 `handle_search` 函数末尾（约第 698 行），将：

```python
# 默认搜索电影
await _do_search(message, user_id, query, "movie")
```

改为：

```python
# 默认搜索 multi（电影+电视剧混合）
await _do_search(message, user_id, query, "multi")
```

在 `_do_search` 函数中（约第 704-737 行），修改结果处理逻辑：

```python
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

    # ⚠️ 关键修改：先过滤再截断，避免"截断错杀"
    # 先过滤出所有 movie/tv 结果，再取前 8 条
    valid_results = [
        item for item in all_results
        if item.get("mediaType") in ("movie", "tv")
    ][:8]

    if not valid_results:
        await message.reply_text("😔 未找到相关内容，请尝试其他关键词")
        return

    # 构建搜索会话（只保存过滤后的结果）
    session = SearchSession(
        results=valid_results,  # 只保存 valid_results
        media_type=media_type,
        query=query,
    )

    # 格式化消息
    caption, keyboard = format_search_results(valid_results, query)

    # 尝试发送带海报的图片消息
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

**关键变化**：
1. 默认搜索类型改为 `"multi"`
2. **在 `_do_search` 中过滤 `valid_results`**，只保留 `mediaType in ("movie", "tv")` 的结果
3. **先过滤再截断**：先从全部结果中过滤出 movie/tv，再取前 8 条，避免 person 占位导致合法结果被截断
3. **`session.results` 只保存 `valid_results`**（单一真相源）
4. 无结果提示改为"未找到相关内容"（不再区分类型）
5. 后续所有环节（格式化、按钮生成、callback 取值）都基于同一个过滤后的数组，确保索引一致性

### 2.2 简化 `format_search_results`（不再二次过滤）

**文件**：`services/bot/app/formatters/message_formatter.py`

将第 489-540 行的 `format_search_results` 函数替换为：

```python
def format_search_results(
    results: list[dict],
    query: str,
) -> tuple[str, InlineKeyboardMarkup]:
    """格式化搜索结果列表（混合展示电影和电视剧）

    返回 (caption_html, inline_keyboard)

    注意：results 已在 _do_search 中过滤，只包含 movie/tv，无需二次过滤
    """
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

        # 从结果中读取实际类型（已在 _do_search 过滤，必定是 movie 或 tv）
        item_media_type = item.get("mediaType", "movie")
        if item_media_type == "movie":
            type_emoji = "🎬"
            type_label = "电影"
            tmdb_path = "movie"
        else:  # tv
            type_emoji = "📺"
            type_label = "电视剧"
            tmdb_path = "tv"

        # 格式：1. 搏击俱乐部 (1999) 🎬 电影 - Fight Club
        line = f"<b>{i + 1}.</b> <a href='https://www.themoviedb.org/{tmdb_path}/{tmdb_id}'>{title}</a>"
        if year:
            line += f" ({year})"
        line += f" {type_emoji} {type_label}"
        if original_title and original_title != str(item.get("title", "")):
            line += f" - {escape(original_title)}"
        lines.append(line)

    # 构建数字按钮：基于 results，每行 4 个
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
```

**关键变化**：
1. **去掉 `media_type` 参数**：不再需要，类型信息从 `item["mediaType"]` 读取
2. 标题改为"搜索 XX 的结果"（不再区分"电影结果"/"电视剧结果"）
3. 添加类型 emoji（🎬/📺）和文字标识
4. 按钮基于 `results` 生成，与 `session.results` 索引完全一致
5. 去掉底部的 `[📺 搜索电视剧]` 切换按钮
6. 调用方需要同步修改：`format_search_results(valid_results, query)` 而非 `format_search_results(valid_results, media_type, query)`

### 2.3 修改 `format_search_detail` 读取实际类型并清理冗余参数

**文件**：`services/bot/app/formatters/message_formatter.py`

`format_search_detail` 函数（约第 506-544 行）当前根据 `media_type` 参数判断类型，但在混合搜索模式下 `media_type` 是 `"multi"`，会导致类型判断错误。

需要修改函数签名，去掉 `media_type` 参数，直接从 `item` 读取实际类型：

```python
def format_search_detail(item: dict) -> str:
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

    # 从 item 读取实际类型（session.results 已过滤，正常情况下必定是 movie 或 tv）
    item_media_type = item.get("mediaType", "movie")
    if item_media_type == "movie":
        type_label = "电影"
        tmdb_path = "movie"
    elif item_media_type == "tv":
        type_label = "电视剧"
        tmdb_path = "tv"
    else:
        # 防御性兜底：不应到达此分支（_do_search 已过滤）
        type_label = "未知"
        tmdb_path = "movie"

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

**关键变化**：
- **去掉 `media_type` 参数**：不再需要，直接从 `item["mediaType"]` 读取
- 从 `item["mediaType"]` 读取实际类型，确保 TMDB 链接路径（`/movie/` 或 `/tv/`）与实际类型匹配
- 调用方需要同步修改：`format_search_detail(item)` 而非 `format_search_detail(item, session.media_type)`

### 2.4 修改 `_do_search` 无结果提示

**文件**：`services/bot/app/handlers/telegram_handler.py`

在 `_do_search` 函数中（约第 692 行），将：

```python
all_results = result.get("results", [])
if not all_results:
    type_label = "电影" if media_type == "movie" else "电视剧"
    await message.reply_text(f"😔 未找到相关{type_label}，请尝试其他关键词")
    return
```

改为：

```python
all_results = result.get("results", [])
if not all_results:
    await message.reply_text("😔 未找到相关内容，请尝试其他关键词")
    return
```

**原因**：混合搜索模式下不再区分"电影"或"电视剧"。

### 2.5 删除 `_handle_toggle_type` 函数

**文件**：`services/bot/app/handlers/telegram_handler.py`

删除第 862-977 行的 `_handle_toggle_type` 函数（约 115 行，包含注释）。

**原因**：不再需要切换类型功能。

### 2.6 修改 `handle_search_callback` 路由

**文件**：`services/bot/app/handlers/telegram_handler.py`

在 `handle_search_callback` 函数中（约第 780-791 行），删除：

```python
elif data == "sub:type":
    await _handle_toggle_type(query, session, user_id)
```

修改后的路由逻辑：

```python
if data.startswith("sub:pick:"):
    await _handle_pick(query, session, user_id, data)
elif data == "sub:ok":
    await _handle_subscribe(query, session, user_id)
elif data == "sub:note":
    await _handle_request_note(query, session, user_id)
elif data == "sub:back":
    await _handle_back(query, session, user_id)
else:
    await query.answer()
```

### 2.7 修改订阅逻辑读取实际类型

**文件**：`services/bot/app/handlers/telegram_handler.py`

#### 在 `_handle_subscribe` 函数中

约第 990 行，将：

```python
item = session.results[session.selected_index]
media_type_upper = "MOVIE" if session.media_type == "movie" else "TV"
```

改为：

```python
item = session.results[session.selected_index]
# 从选中结果读取实际类型（session.media_type 现在是 "multi"）
item_media_type = item.get("mediaType", "movie")
media_type_upper = "MOVIE" if item_media_type == "movie" else "TV"
```

#### 在 `handle_text_message` 函数中

约第 1241 行，做同样修改：

```python
item = session.results[session.selected_index]
# 从选中结果读取实际类型
item_media_type = item.get("mediaType", "movie")
media_type_upper = "MOVIE" if item_media_type == "movie" else "TV"
```

### 2.8 修改 `_handle_back` 返回列表逻辑

**文件**：`services/bot/app/handlers/telegram_handler.py`

`_handle_back` 函数无需修改，因为它调用 `format_search_results` 时传入的 `session.media_type` 现在是 `"multi"`，格式化函数会正确处理。

---

## 三、更新文档

### 3.1 更新 Callback Data 格式表

**文件**：`docs/plan/telegram-search-subscribe.md`

在"Callback Data 格式一览"章节（约第 1380 行），删除：

```
| `sub:type` | 切换搜索类型 | `handle_search_callback` | 8 bytes |
```

### 3.2 更新验证方式

**文件**：`docs/plan/telegram-search-subscribe.md`

在"验证方式"章节（约第 1460 行），删除：

```
- 点击 [📺 搜索电视剧] → 确认切换类型并重新搜索
```

新增：

```
- `/search 权力的游戏` → 确认电视剧结果直接出现在混合列表中，无需切换类型
- 点击电影结果 → 确认详情页显示"类型：电影"，订阅时 type=MOVIE
- 点击电视剧结果 → 确认详情页显示"类型：电视剧"，订阅时 type=TV
- 确认底部不再有 `[📺 搜索电视剧]` 按钮
```

### 3.3 更新 UX 流程图

**文件**：`docs/plan/telegram-search-subscribe.md`

在"UX 流程"章节（约第 10-50 行），将步骤 2 的示例改为：

```
2. Bot 发送图片消息：
   ┌─────────────────────────┐
   │     [第一条结果的海报]     │
   ├─────────────────────────┤
   │ 🔍 搜索 "搏击俱乐部" 的  │
   │    结果：                  │
   │                           │
   │ 1. 搏击俱乐部 (1999)      │
   │    🎬 电影 - Fight Club   │
   │ 2. 权力的游戏 (2011)      │
   │    📺 电视剧 - Game of... │
   ├─────────────────────────┤
   │ [1] [2] [3] [4]          │
   └─────────────────────────┘
```

去掉 `[📺 搜索电视剧]` 按钮。

---

## 涉及文件清单

| 文件 | 操作 | 改动量 |
|------|------|--------|
| `services/api/internal/handlers/tmdb.go` | 修改 | +1 字段，修改 Search 函数逻辑（约 10 行） |
| `services/bot/app/handlers/telegram_handler.py` | 修改 | 删除 `_handle_toggle_type`（-115 行），修改 3 处订阅逻辑（+6 行） |
| `services/bot/app/formatters/message_formatter.py` | 修改 | 重写 `format_search_results`（约 20 行变化） |
| `docs/plan/telegram-search-subscribe.md` | 修改 | 更新 3 处文档说明 |

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

3. **端到端测试**：
   - `/search 搏击俱乐部` → 确认显示混合结果（电影 + 电视剧），每条带类型标识（🎬/📺）
   - `/search 权力的游戏` → 确认电视剧结果直接出现在列表中，无需切换类型
   - 点击电影结果（如搏击俱乐部）→ 确认详情页显示"类型：电影"，点击订阅后管理员收到通知显示 `type=MOVIE`
   - 点击电视剧结果（如权力的游戏）→ 确认详情页显示"类型：电视剧"，点击订阅后管理员收到通知显示 `type=TV`
   - 确认底部不再有 `[📺 搜索电视剧]` 按钮
   - 搜索无结果的关键词 → 确认提示"未找到相关内容"（不再区分类型）
   - 搜索同名电影和剧集（如"老友记"）→ 确认两种类型都出现在列表中，可以通过年份和类型标识区分

---

## 优势与权衡

### 优势

1. **减少交互步骤**：从"搜索 → 切换类型 → 再搜索"变为"搜索 → 直接选择"
2. **符合用户心智模型**：用户搜索时想的是"这个内容"，而不是"这个电影"或"这个电视剧"
3. **更好的发现性**：用户可能意外发现同名的电影和剧集版本（如"老友记"有电影版和剧集版）
4. **降低认知负担**：不需要用户预先判断内容类型

### 权衡

1. **结果更杂**：同名电影和剧集混在一起，但通过年份 + 类型标识可以清晰区分
2. **8 条限制的影响**：电影和剧集各占一半，但这正是用户想要的全面视图
3. **TMDB API 调用**：`search/multi` 端点稳定可用，无额外成本

### 兼容性

- TMDB API 的 `search/multi` 端点稳定可用
- 现有 `TMDBSearchResult` 结构已兼容电影和剧集字段（`Title/Name`、`ReleaseDate/FirstAirDate`）
- Bot 侧只需调整展示逻辑，核心订阅流程不变
- **向后兼容性**：API 默认值保持 `movie` 不变，仅由 Bot 显式传 `multi`，不影响其他调用方（如 Web 前端）。新增 `type` 参数白名单校验，拒绝非法值。

---

## 关键问题与修正

### 问题 A：`person` 类型未过滤

**风险**：TMDB `search/multi` 返回 3 种类型：`movie`、`tv`、`person`（演员/导演等人物）。原计划的 else 分支会把 `person` 错误归类为电视剧，导致用户搜索"成龙"时看到人物结果被标记为"📺 电视剧"，点击订阅会以 `type=TV` 提交。

**修正**：
- Go API 侧：在结果转换循环中，`if itemMediaType == "person" { continue }` 过滤掉人物结果
- Bot 侧：在 `_do_search` 中统一过滤，`format_search_results` 不再二次过滤，纯展示逻辑

### 问题 B：索引错位风险

**风险**：如果在 `format_search_results` 中用 `continue` 跳过非法类型，会导致展示列表序号与 `session.results` 索引不一致。用户点击按钮 [2] 时，callback 索引可能指向错误的结果。

**修正**：在 `_do_search` 中统一过滤并保存到 `session.results`，确保单一真相源。`format_search_results` 直接展示，不再过滤。

### 问题 C：过滤顺序导致"截断错杀"

**风险**：如果先截断 `all_results[:8]` 再过滤，当前 8 条中夹杂 person 时，会把可展示结果压缩到很少，而第 9 条以后的合法 movie/tv 被丢掉。

**修正**：先过滤出所有 movie/tv 结果，再取前 8 条：`[item for item in all_results if ...][:8]`

### 问题 D：详情页类型判断错误

**风险**：`format_search_detail` 函数根据 `media_type` 参数判断类型，但在混合搜索模式下 `media_type` 是 `"multi"`，会导致：
- 详情页显示"类型：电视剧"（即使是电影）
- TMDB 链接错误（`/tv/550` 而非 `/movie/550`）

**修正**：
- 去掉 `media_type` 参数（已冗余）
- 直接从 `item["mediaType"]` 读取实际类型
- 调用方同步修改为 `format_search_detail(item)`

### 问题 E：无结果提示文案不一致

**风险**：`_do_search` 中的无结果提示仍然是"未找到相关电影/电视剧"，与混合搜索的目标不一致。

**修正**：改为"未找到相关内容，请尝试其他关键词"。

### 问题 F：`type` 参数校验缺失

**风险**：没有 `type` 参数白名单校验，`type=abc` 会走到 `search/abc` 端点（TMDB 会返回 404）。

**修正**：在 Go API 中新增 `else if mediaType != "multi"` 分支，返回 400 错误"type 参数必须是 movie、tv 或 multi"。

