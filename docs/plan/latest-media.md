# 最近入库功能实现方案

## 背景与目标

Ember 已经通过 `EmbyService.GetMediaStats()` 获取媒体库统计数（电影/剧集数量），但缺少一个让用户直观浏览"最近入库了什么"的功能。

**本方案实现**：
1. 后端新增 Emby `/Users/{userId}/Items/Latest` API 调用 + 带缓存的 `MediaService` 方法 + HTTP handler
2. 前端新增「最近入库」页面，以海报网格展示最近入库的电影和剧集
3. 所有登录用户均可访问

**不做的事**：
- 不做搜索/筛选功能（后续可扩展）
- 不做详情页跳转（直接展示列表）
- 不做分页（一次加载最近 N 条，足够展示）

---

## 一、Emby API 选型

### 1.1 ~~废弃方案~~：`GET /emby/Items` + `SortBy`

原计划使用通用查询端点 `/emby/Items`，电影用 `SortBy=DateCreated`，剧集用 `SortBy=DateLastMediaAdded`。

**实测结论**：
- `DateLastMediaAdded` **不是 Emby 合法的 SortBy 值**（来源：[Emby 源码 BaseItemsRequest.cs](https://github.com/MediaBrowser/Emby/blob/master/MediaBrowser.Api/UserLibrary/BaseItemsRequest.cs)），导致 SQLite 500 异常
- Emby 合法的 SortBy 值仅有：`Album, AlbumArtist, Artist, Budget, CommunityRating, CriticRating, DateCreated, DatePlayed, PlayCount, PremiereDate, ProductionYear, SortName, Random, Revenue, Runtime`
- 用 `SortBy=DateCreated` 查 Series 虽然不报错，但返回的是"Series 首次创建时间"排序，无法反映"最近有新集更新"

### 1.2 正确方案：`GET /emby/Users/{userId}/Items/Latest`

Emby 提供了专用的"最近入库"端点（[官方文档](https://dev.emby.media/doc/restapi/Latest-Items.html)）：

```
GET /emby/Users/{userId}/Items/Latest
    ?IncludeItemTypes=Movie
    &Limit=20
    &GroupItems=true
    &Fields=Overview,DateCreated,ProductionYear,CommunityRating
    &api_key={apiKey}
```

**关键参数 `GroupItems`**（默认 `true`）：
- 同一个 Series 下的新 Episode 会**自动聚合到 Series 层级**返回
- 返回的 `ChildCount` 字段表示新增了多少集
- 完美解决了"持续更新的剧集应出现在最近入库列表"的问题

**需要 `userId`**：该端点需要用户 ID。Ember 每个用户注册时都会创建 Emby 账号，`EmbyID` 存在数据库 `users.embyId` 字段中。handler 通过 JWT context 获取当前用户 → 查询 `EmbyID` → 传给 Emby API。

### 1.3 返回数据结构

`/Users/{userId}/Items/Latest` 返回一个**数组**（注意：不是包裹在 `Items` 中的对象）：

```json
[
  {
    "Name": "电影名称",
    "Id": "abc123",
    "Type": "Movie",
    "ProductionYear": 2024,
    "DateCreated": "2024-01-15T10:30:00.0000000Z",
    "CommunityRating": 7.5,
    "OfficialRating": "PG-13",
    "Overview": "剧情简介...",
    "ImageTags": {
      "Primary": "tag-hash"
    },
    "ChildCount": 0
  },
  {
    "Name": "剧集名称",
    "Id": "def456",
    "Type": "Series",
    "ProductionYear": 2023,
    "DateCreated": "2024-01-01T00:00:00.0000000Z",
    "CommunityRating": 8.2,
    "Overview": "剧集简介...",
    "ImageTags": {
      "Primary": "tag-hash"
    },
    "ChildCount": 3
  }
]
```

> ⚠️ 注意与 `/emby/Items` 的区别：`/Items/Latest` 返回的是**裸数组 `[]EmbyItem`**，而非 `{"Items": [...], "TotalRecordCount": N}` 包裹对象。

### 1.4 海报图片 URL

前端直接访问 Emby 图片 API（无需后端代理）：

```
{embyPublicUrl}/emby/Items/{itemId}/Images/Primary?maxHeight=400&quality=90
```

- 大多数 Emby 部署对图片接口不要求认证
- `embyPublicUrl` 已有现成获取链路：`GET /api/v1/emby/config` → 前端 `embyUrl`

---

## 二、实施方案

### 2.1 后端：EmbyService 新增方法

**修改文件**：`services/api/internal/services/emby.go`

新增结构体：

```go
// EmbyItem Emby 媒体项（/Items/Latest 返回结构）
type EmbyItem struct {
	ID              string            `json:"Id"`
	Name            string            `json:"Name"`
	Type            string            `json:"Type"`               // Movie / Series
	ProductionYear  int               `json:"ProductionYear"`
	DateCreated     string            `json:"DateCreated"`
	CommunityRating *float64          `json:"CommunityRating,omitempty"`
	OfficialRating  *string           `json:"OfficialRating,omitempty"`
	Overview        *string           `json:"Overview,omitempty"`
	ImageTags       map[string]string `json:"ImageTags"`
	ChildCount      int               `json:"ChildCount"`         // GroupItems=true 时的新增子项数
}
```

新增方法：

```go
// GetLatestItems 获取用户视角的最近入库媒体
// 使用 /emby/Users/{userId}/Items/Latest 端点
func (s *EmbyService) GetLatestItems(embyUserID string, itemType string, limit int) ([]EmbyItem, error) {
	if s.baseURL == "" || s.apiKey == "" {
		return nil, errors.New("Emby 配置未设置")
	}

	// 构建 URL：/emby/Users/{userId}/Items/Latest?...
	url := fmt.Sprintf(
		"%s/emby/Users/%s/Items/Latest?IncludeItemTypes=%s&Limit=%d&GroupItems=true&Fields=Overview,DateCreated,ProductionYear,CommunityRating,OfficialRating&api_key=%s",
		s.baseURL, embyUserID, itemType, limit, s.apiKey,
	)

	// HTTP GET + JSON 解析
	// 注意：返回的是裸数组 []EmbyItem，不是 {"Items": [...]}
	var items []EmbyItem
	if err := json.Unmarshal(body, &items); err != nil {
		return nil, err
	}
	return items, nil
}
```

沿用现有的 HTTP 请求模式（参考 `GetMediaStats()` 第 386-420 行）：`http.NewRequest → client.Do → io.ReadAll → json.Unmarshal`。

### 2.2 后端：MediaService 带缓存

**修改文件**：`services/api/internal/services/media.go`

缓存策略调整——因为 `/Items/Latest` 需要 `embyUserID`，不同用户可能看到不同的"最新"列表（取决于 Emby 的库权限配置）。但在大多数部署中，所有用户看到的媒体库是一样的。

**务实方案**：按 `itemType` 缓存（不按用户区分），使用第一个请求的 `embyUserID` 刷新缓存。

> ⚠️ 重要说明（数据隔离假设）
>
> 该缓存策略依赖一个前提：所有普通用户在 Emby 侧看到的媒体库内容一致（无用户级可见性差异）。
> 如果未来启用了 Emby 的用户级权限控制（家长控制、库权限、标签过滤等），必须把缓存隔离到用户维度
>（例如 key = `embyUserID + itemType`），否则会出现“内容串联”，严重时属于越权信息暴露。

```go
type MediaService struct {
	embyService    *EmbyService
	// ... 现有 stats 缓存字段 ...

	// 最近入库缓存（按 itemType 区分）
	latestCache      map[string]*latestCacheEntry // key: "Movie" / "Series"
	latestCacheMutex sync.RWMutex
	latestCacheTTL   time.Duration // 10 分钟
}

type latestCacheEntry struct {
	items     []EmbyItem
	timestamp time.Time
}

// GetLatestItems 获取最近入库项（带缓存）
func (s *MediaService) GetLatestItems(embyUserID string, itemType string, limit int) ([]EmbyItem, error) {
	s.latestCacheMutex.RLock()
	if entry, ok := s.latestCache[itemType]; ok && time.Since(entry.timestamp) < s.latestCacheTTL {
		s.latestCacheMutex.RUnlock()
		return entry.items, nil
	}
	s.latestCacheMutex.RUnlock()

	items, err := s.embyService.GetLatestItems(embyUserID, itemType, limit)
	if err != nil {
		return nil, err
	}

	s.latestCacheMutex.Lock()
	s.latestCache[itemType] = &latestCacheEntry{items: items, timestamp: time.Now()}
	s.latestCacheMutex.Unlock()

	return items, nil
}
```

### 2.3 后端：Handler + 路由

**修改文件**：`services/api/internal/handlers/media.go`

handler 需要从 JWT context 获取当前用户 → 查询 EmbyID → 传递给 MediaService：

```go
// GetLatestItems 获取最近入库的媒体
// GET /api/v1/media/latest?type=Movie&limit=20
func (h *MediaHandler) GetLatestItems(c *gin.Context) {
	// 1. 从 JWT context 获取用户 ID
	userID, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"success": false, "error": "未认证"})
		return
	}

	// 2. 查询用户的 EmbyID
	var user models.User
	if err := db.DB.Select("embyId").Where("id = ?", userID).First(&user).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "error": "用户不存在"})
		return
	}
	if user.EmbyID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "用户未绑定 Emby 账号"})
		return
	}

	// 3. 解析查询参数
	itemType := c.DefaultQuery("type", "Movie")
	if itemType != "Movie" && itemType != "Series" {
		itemType = "Movie"
	}
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
	if limit <= 0 || limit > 50 {
		limit = 20
	}

	// 4. 获取数据（带缓存）
	items, err := h.service.GetLatestItems(user.EmbyID, itemType, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}

	// 5. 转换为 camelCase 响应（Emby 返回 PascalCase）
	c.JSON(http.StatusOK, gin.H{"success": true, "data": convertItems(items)})
}
```

> 注：`convertItems()` 将 Emby 的 PascalCase 字段映射为 Ember API 规范的 camelCase。也可以直接用 struct tag 控制 JSON 序列化来实现。

**修改文件**：`services/api/cmd/server/main.go`

在 `authenticated` 路由组（第 121-142 行）中添加：

```go
authenticated.GET("/media/latest", mediaHandler.GetLatestItems)
```

### 2.4 后端 API 响应格式

```json
// GET /api/v1/media/latest?type=Movie&limit=20
{
  "success": true,
  "data": [
    {
      "id": "abc123",
      "name": "电影名称",
      "type": "Movie",
      "productionYear": 2024,
      "dateCreated": "2024-01-15T10:30:00Z",
      "communityRating": 7.5,
      "officialRating": "PG-13",
      "overview": "剧情简介...",
      "childCount": 0
    },
    {
      "id": "def456",
      "name": "剧集名称",
      "type": "Series",
      "productionYear": 2023,
      "dateCreated": "2024-01-01T00:00:00Z",
      "communityRating": 8.2,
      "overview": "剧集简介...",
      "childCount": 3
    }
  ]
}
```

图片 URL 不在响应中返回——前端根据 `id` 自行拼接 `{embyPublicUrl}/emby/Items/{id}/Images/Primary?maxHeight=400`。

### 2.5 前端：API 层

**修改文件**：`services/web/src/api/console.ts`

```typescript
// ==================== 最近入库 ====================
export function getLatestMedia(
  type: 'Movie' | 'Series',
  limit: number = 20
): Promise<{ data: LatestMediaItem[] }> {
  return request({
    url: '/media/latest',
    method: 'get',
    params: { type, limit }
  })
}
```

**修改文件**：`services/web/src/types/api.ts`

```typescript
export interface LatestMediaItem {
  id: string
  name: string
  type: 'Movie' | 'Series'
  productionYear: number
  dateCreated: string
  communityRating?: number
  officialRating?: string
  overview?: string
  childCount: number  // Series: 最近新增的集数
}
```

### 2.6 前端：新增页面

**新增文件**：`services/web/src/views/console/LibraryView.vue`

页面结构：

```
┌─────────────────────────────────────────────────┐
│  📺 媒体库                                       │
│  浏览 Emby 服务器最近入库的影视内容                   │
│                                                   │
│  [电影] [剧集]                    ← Tab 切换       │
│                                                   │
│  ┌─────┐ ┌─────┐ ┌─────┐ ┌─────┐ ┌─────┐       │
│  │     │ │     │ │     │ │     │ │     │       │
│  │海报  │ │海报  │ │海报  │ │海报  │ │海报  │       │
│  │     │ │     │ │     │ │     │ │     │       │
│  ├─────┤ ├─────┤ ├─────┤ ├─────┤ ├─────┤       │
│  │名称  │ │名称  │ │名称  │ │名称  │ │名称  │       │
│  │年份  │ │+3集 │ │年份  │ │年份  │ │年份  │       │
│  └─────┘ └─────┘ └─────┘ └─────┘ └─────┘       │
└─────────────────────────────────────────────────┘
```

遵循设计规范中的**媒体网格模式**（`docs/reference/WEB_DESIGN_GUIDE.md` §5）：
- 网格：`grid grid-cols-2 md:grid-cols-4 lg:grid-cols-5 xl:grid-cols-6 gap-6`
- 海报：`aspect-[2/3] object-cover`
- 悬停：`group-hover:scale-110` 放大 + 遮罩
- 加载：骨架屏或 loading 状态

海报图片 URL 拼接：
```typescript
const getImageUrl = (itemId: string) => {
  return embyUrl.value
    ? `${embyUrl.value}/emby/Items/${itemId}/Images/Primary?maxHeight=400&quality=90`
    : 'https://via.placeholder.com/300x450?text=No+Poster'
}
```

卡片信息展示：
- 标题（name）
- 年份（productionYear）
- 评分（communityRating，可选显示）
- Series 特有：显示 `childCount`（如"新增 3 集"徽章）

### 2.7 前端：路由 + 侧边栏

**修改文件**：`services/web/src/router/index.ts`

在 console children 中添加（放在 rankings 后面）：

```typescript
{
  path: 'library',
  name: 'console-library',
  component: () => import('../views/console/LibraryView.vue'),
},
```

**修改文件**：`services/web/src/components/console/Sidebar.vue`

在 menuItems 中添加（放在"播放排行榜"后面）：

```typescript
{
  title: '媒体库',
  path: '/console/library',
  icon: Film,      // 从 @element-plus/icons-vue 引入
  role: 'user'
},
```

需要在 import 中添加 `Film` 图标。

---

## 三、执行顺序

```
1. emby.go       — 新增 EmbyItem 结构体 + GetLatestItems(embyUserID, itemType, limit)
2. media.go      — 新增缓存结构 + GetLatestItems() 带缓存方法
3. media.go(h)   — handler 新增 GetLatestItems()（从 JWT 获取 userID → 查 EmbyID → 调用 service）
4. main.go       — 注册路由 authenticated.GET("/media/latest", ...)
5. go build      — 编译验证后端
6. api.ts        — 前端 LatestMediaItem 类型定义
7. console.ts    — 前端 getLatestMedia() API 函数
8. LibraryView   — 新增页面组件
9. router        — 注册 /console/library
10. Sidebar      — 添加"媒体库"导航项
11. npm build    — 编译验证前端
```

---

## 四、涉及文件汇总

| 文件路径 | 改动类型 |
|---------|---------|
| `services/api/internal/services/emby.go` | 新增 `EmbyItem` + `GetLatestItems()` |
| `services/api/internal/services/media.go` | 新增缓存 + `GetLatestItems()` 带缓存 |
| `services/api/internal/handlers/media.go` | 新增 handler（含 JWT→EmbyID 查询链路） |
| `services/api/cmd/server/main.go` | 注册 `GET /media/latest` 路由 |
| `services/web/src/types/api.ts` | 新增 `LatestMediaItem` 接口 |
| `services/web/src/api/console.ts` | 新增 `getLatestMedia()` |
| `services/web/src/views/console/LibraryView.vue` | **新增**页面 |
| `services/web/src/router/index.ts` | 注册路由 |
| `services/web/src/components/console/Sidebar.vue` | 添加导航项 |

---

## 五、验证方式

1. **后端编译**：`cd services/api && go build ./...`
2. **前端编译**：`cd services/web && npm run build`
3. **Emby API 直测**（确认端点可用）：
   ```
   {EMBY_URL}/emby/Users/{任意EmbyUserID}/Items/Latest?IncludeItemTypes=Movie&Limit=5&api_key={KEY}
   {EMBY_URL}/emby/Users/{任意EmbyUserID}/Items/Latest?IncludeItemTypes=Series&Limit=5&GroupItems=true&api_key={KEY}
   ```
4. **Ember API 验证**（由用户启动服务后测试）：
   ```bash
   curl -H "Authorization: Bearer {token}" \
     "http://localhost:8080/api/v1/media/latest?type=Movie&limit=10"
   curl -H "Authorization: Bearer {token}" \
     "http://localhost:8080/api/v1/media/latest?type=Series&limit=10"
   ```
   预期：返回 `{"success": true, "data": [...]}`
5. **页面验证**：
   - 访问 `/console/library`
   - 确认侧边栏"媒体库"导航项
   - 切换电影/剧集 Tab
   - 海报图片正常显示
   - Series 卡片显示 `childCount`（如"新增 3 集"）
   - 移动端网格自适应

---

## 六、与旧方案的对比

| | 旧方案（已废弃） | 新方案 |
|---|---|---|
| 端点 | `GET /emby/Items` | `GET /emby/Users/{id}/Items/Latest` |
| 排序 | `SortBy=DateLastMediaAdded`（不合法，500 错误） | 端点内部自动处理排序 |
| 剧集更新 | ❌ 只能按 Series 创建时间排序 | ✅ 自动聚合新 Episode，返回 ChildCount |
| 需要 userId | ❌ 不需要 | ✅ 需要（从 JWT→DB 获取 EmbyID） |
| 响应格式 | `{"Items": [...], "TotalRecordCount": N}` | `[...]`（裸数组） |
