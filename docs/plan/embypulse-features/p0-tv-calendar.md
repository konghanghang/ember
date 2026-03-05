# P0-1: 追剧日历（TV Calendar）

## 功能描述

自动同步 TMDB 剧集排期，并与 Emby 媒体库比对，展示剧集入库状态（已入库/缺失/待播）。

本方案采用三层缓存：
- 内存缓存（短 TTL）
- PostgreSQL 持久化缓存
- TMDB 远程查询

并支持 Emby Webhook 入库联动，减少“红灯已补货但页面未更新”的延迟。

**优先级**：P0

---

## Ember 对齐要点

1. 所有主键使用 `string`（CUID），不使用 `uint`
2. 列名使用 camelCase（如 `tmdbId`、`airDate`）
3. API 统一前缀 `/api/v1`
4. 前端路由挂载到 `/console/*`
5. Webhook 统一挂载 `/api/v1/webhooks/*`

---

## 数据模型设计

### 1. 日历条目表 `tv_calendar_items`

```go
type TVCalendarItem struct {
    ID          string    `json:"id" gorm:"column:id;type:varchar(25);primaryKey"`
    TmdbID      string    `json:"tmdbId" gorm:"column:tmdbId;size:50;not null;index;uniqueIndex:uk_tv_calendar_episode,priority:1"`
    SeriesID    string    `json:"seriesId" gorm:"column:seriesId;size:50;index"` // Emby SeriesId
    Season      int       `json:"season" gorm:"column:season;not null;uniqueIndex:uk_tv_calendar_episode,priority:2"`
    Episode     int       `json:"episode" gorm:"column:episode;not null;uniqueIndex:uk_tv_calendar_episode,priority:3"`
    AirDate     time.Time `json:"airDate" gorm:"column:airDate;not null;index"`
    EpisodeName string    `json:"episodeName" gorm:"column:episodeName;size:255"`
    Status      string    `json:"status" gorm:"column:status;size:20;not null;default:'upcoming'"` // ready/missing/upcoming/today
    EmbyItemID  string    `json:"embyItemId,omitempty" gorm:"column:embyItemId;size:50"`
    LastChecked time.Time `json:"lastChecked" gorm:"column:lastChecked"`
    CreatedAt   time.Time `json:"createdAt" gorm:"column:createdAt;autoCreateTime"`
    UpdatedAt   time.Time `json:"updatedAt" gorm:"column:updatedAt;autoUpdateTime"`
}
```

约束：
- 唯一索引：`(tmdbId, season, episode)`
- `airDate` 按“日期语义”落库，统一归一化为 UTC `00:00:00`，避免时区偏移导致跨天

### 2. 用户追剧订阅表 `tv_calendar_subscriptions`

```go
type TVCalendarSubscription struct {
    ID        string    `json:"id" gorm:"column:id;type:varchar(25);primaryKey"`
    UserID    string    `json:"userId" gorm:"column:userId;type:varchar(25);not null;index;uniqueIndex:uk_tv_calendar_subscription,priority:1"`
    TmdbID    string    `json:"tmdbId" gorm:"column:tmdbId;size:50;not null;index;uniqueIndex:uk_tv_calendar_subscription,priority:2"`
    ShowName  string    `json:"showName" gorm:"column:showName;size:255;not null"`
    PosterURL string    `json:"posterUrl" gorm:"column:posterUrl;size:500"`
    CreatedAt time.Time `json:"createdAt" gorm:"column:createdAt;autoCreateTime"`
}
```

约束：
- 唯一索引：`(userId, tmdbId)`

### 3. TMDB 缓存表 `tmdb_cache`

```go
type TMDBCache struct {
    ID         string    `json:"id" gorm:"column:id;type:varchar(25);primaryKey"`
    CacheKey   string    `json:"cacheKey" gorm:"column:cacheKey;size:255;uniqueIndex;not null"`
    CacheValue string    `json:"cacheValue" gorm:"column:cacheValue;type:text;not null"`
    ExpiresAt  time.Time `json:"expiresAt" gorm:"column:expiresAt;index"`
    CreatedAt  time.Time `json:"createdAt" gorm:"column:createdAt;autoCreateTime"`
}
```

---

## API 端点设计

### 1. 获取日历（用户）

`GET /api/v1/tv-calendar`

Query:
- `startDate` (`YYYY-MM-DD`)
- `endDate` (`YYYY-MM-DD`)
- `status`（可选）

Response:

```json
{
  "data": [
    {
      "id": "cuid_xxx",
      "tmdbId": "1399",
      "season": 3,
      "episode": 5,
      "airDate": "2026-03-10T00:00:00Z",
      "episodeName": "...",
      "status": "ready",
      "embyItemId": "abc123",
      "showName": "Game of Thrones",
      "posterUrl": "https://..."
    }
  ]
}
```

### 2. 订阅剧集（用户）

`POST /api/v1/tv-calendar/subscriptions`

```json
{
  "tmdbId": "1399",
  "showName": "Game of Thrones",
  "posterUrl": "https://..."
}
```

### 3. 取消订阅（用户）

`DELETE /api/v1/tv-calendar/subscriptions/:tmdbId`

### 4. 手动刷新（管理员）

`POST /api/v1/admin/tv-calendar/refresh`

```json
{
  "tmdbId": "1399",
  "force": false
}
```

### 5. Emby Webhook 联动

`POST /api/v1/webhooks/emby?token=<WEBHOOK_TOKEN>`

说明：
- 复用统一 webhook 安全策略（token 校验）
- 仅处理 `library.new / item.added` 中 `Episode` 事件

---

## 核心实现建议

### 服务层方法建议

```go
// FetchCalendar 获取时间范围内日历（按当前用户订阅过滤）
func (s *TVCalendarService) FetchCalendar(ctx context.Context, userID string, startDate, endDate time.Time, status string) ([]TVCalendarDTO, error)

// RefreshCalendar 管理员触发刷新
func (s *TVCalendarService) RefreshCalendar(ctx context.Context, tmdbID *string, force bool) (int, error)

// MarkEpisodeReadyByWebhook Webhook 点亮剧集状态
func (s *TVCalendarService) MarkEpisodeReadyByWebhook(ctx context.Context, seriesID string, season, episode int) error
```

### 与 Emby 对齐的关键点

1. 物理文件校验必须过滤虚拟占位符：
- `LocationType == Virtual` 视为未入库
- `IsMissing == true` 视为未入库
- 必须有 `Path` 或 `MediaSources`

2. 避免全量重复拉取：
- 先查持久化缓存
- 仅对过期订阅刷新

3. Webhook 只做“状态变更”，不做重型拉取。

---

## 前端页面设计

### 路由

```ts
{
  path: '/console/tv-calendar',
  name: 'console-tv-calendar',
  meta: { requiresAuth: true },
  component: () => import('@/views/console/TVCalendarView.vue')
}
```

### 页面能力

- 日期范围筛选
- 状态筛选
- 订阅管理弹窗
- 管理员可见“手动刷新”按钮

---

## 验证清单

- [ ] 模型可迁移且字段命名符合 camelCase
- [ ] 用户只能看到自己订阅剧集
- [ ] Webhook 到达后状态可被即时点亮
- [ ] 关闭 TMDB Key 时返回明确错误信息
- [ ] 大量订阅情况下接口延迟可控

**预计工作量**：7-10 天
