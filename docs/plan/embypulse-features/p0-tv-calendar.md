# P0-1: 追剧日历（TV Calendar）

## 功能描述

自动同步 TMDB 剧集排期，与 Emby 媒体库实时比对，显示剧集入库状态（红绿灯）。支持三层缓存架构（内存 → PostgreSQL → TMDB API），严格物理文件校验（过滤虚拟占位符），Webhook 联动（新剧集入库自动更新状态）。

**核心价值**：
- 用户可提前知道追剧进度，避免频繁检查媒体库
- 管理员可快速识别缺失剧集，优化资源采购
- 自动化程度高，减少人工维护成本

**优先级**：P0（强烈推荐）⭐⭐⭐⭐⭐

---

## 数据模型设计

### 1. TV Calendar 表（tv_calendar）

```go
// TVCalendar 追剧日历条目
type TVCalendar struct {
    ID          uint      `gorm:"primaryKey;column:id" json:"id"`
    TMDBId      int       `gorm:"column:tmdb_id;not null;index" json:"tmdbId"`
    Season      int       `gorm:"column:season;not null" json:"season"`
    Episode     int       `gorm:"column:episode;not null" json:"episode"`
    AirDate     time.Time `gorm:"column:air_date;not null;index" json:"airDate"`
    EpisodeName string    `gorm:"column:episode_name;size:255" json:"episodeName"`
    Status      string    `gorm:"column:status;size:20;default:'pending'" json:"status"` // pending/available/missing
    EmbyItemId  string    `gorm:"column:emby_item_id;size:50" json:"embyItemId"`
    LastChecked time.Time `gorm:"column:last_checked" json:"lastChecked"`
    CreatedAt   time.Time `gorm:"column:created_at" json:"createdAt"`
    UpdatedAt   time.Time `gorm:"column:updated_at" json:"updatedAt"`
}

// 联合唯一索引
// CREATE UNIQUE INDEX idx_tv_calendar_unique ON tv_calendar(tmdb_id, season, episode);
```

### 2. TV Show Subscription 表（tv_show_subscriptions）

```go
// TVShowSubscription 用户订阅的剧集
type TVShowSubscription struct {
    ID        uint      `gorm:"primaryKey;column:id" json:"id"`
    UserID    uint      `gorm:"column:user_id;not null;index" json:"userId"`
    TMDBId    int       `gorm:"column:tmdb_id;not null;index" json:"tmdbId"`
    ShowName  string    `gorm:"column:show_name;size:255" json:"showName"`
    PosterURL string    `gorm:"column:poster_url;size:500" json:"posterUrl"`
    CreatedAt time.Time `gorm:"column:created_at" json:"createdAt"`
}

// 联合唯一索引
// CREATE UNIQUE INDEX idx_tv_subscription_unique ON tv_show_subscriptions(user_id, tmdb_id);
```

### 3. TMDB Cache 表（tmdb_cache）

```go
// TMDBCache TMDB API 响应缓存
type TMDBCache struct {
    ID         uint      `gorm:"primaryKey;column:id" json:"id"`
    CacheKey   string    `gorm:"column:cache_key;size:255;uniqueIndex" json:"cacheKey"`
    CacheValue string    `gorm:"column:cache_value;type:text" json:"cacheValue"`
    ExpiresAt  time.Time `gorm:"column:expires_at;index" json:"expiresAt"`
    CreatedAt  time.Time `gorm:"column:created_at" json:"createdAt"`
}
```

---

## API 端点设计

### 1. 获取追剧日历

```
GET /api/tv-calendar
Query Parameters:
  - startDate: string (YYYY-MM-DD, 默认今天)
  - endDate: string (YYYY-MM-DD, 默认 startDate + 7 天)
  - status: string (pending/available/missing, 可选)
  - userId: int (可选，仅返回该用户订阅的剧集)

Response:
{
  "data": [
    {
      "id": 1,
      "tmdbId": 12345,
      "season": 3,
      "episode": 5,
      "airDate": "2026-03-10T00:00:00Z",
      "episodeName": "The Rains of Castamere",
      "status": "available",
      "embyItemId": "abc123",
      "showName": "Game of Thrones",
      "posterUrl": "https://image.tmdb.org/t/p/w500/xxx.jpg"
    }
  ]
}
```

### 2. 订阅剧集

```
POST /api/tv-calendar/subscribe
Body:
{
  "tmdbId": 12345,
  "showName": "Game of Thrones",
  "posterUrl": "https://..."
}

Response:
{
  "message": "订阅成功"
}
```

### 3. 取消订阅

```
DELETE /api/tv-calendar/subscribe/:tmdbId

Response:
{
  "message": "取消订阅成功"
}
```

### 4. 手动刷新日历

```
POST /api/tv-calendar/refresh
Body:
{
  "tmdbId": 12345,  // 可选，不传则刷新所有订阅
  "force": false    // 是否强制刷新（跳过缓存）
}

Response:
{
  "message": "刷新成功",
  "updated": 15
}
```

### 5. Webhook 回调（Emby 新剧集入库）

```
POST /api/webhooks/emby/library-update
Headers:
  X-Webhook-Secret: <配置的密钥>
Body:
{
  "event": "library.new",
  "item": {
    "id": "abc123",
    "type": "Episode",
    "seriesId": "series_456",
    "seasonNumber": 3,
    "episodeNumber": 5,
    "providerIds": {
      "Tmdb": "12345"
    }
  }
}

Response:
{
  "message": "处理成功"
}
```

---

## 前端页面设计

### 路由配置

```typescript
// services/web/src/router/index.ts
{
  path: '/tv-calendar',
  name: 'TVCalendar',
  component: () => import('@/views/tv-calendar/index.vue'),
  meta: { title: '追剧日历', requiresAuth: true }
}
```

### 页面结构

```vue
<!-- services/web/src/views/tv-calendar/index.vue -->
<template>
  <div class="tv-calendar">
    <!-- 顶部工具栏 -->
    <el-row :gutter="20" class="toolbar">
      <el-col :span="12">
        <el-date-picker
          v-model="dateRange"
          type="daterange"
          range-separator="至"
          start-placeholder="开始日期"
          end-placeholder="结束日期"
          @change="fetchCalendar"
        />
      </el-col>
      <el-col :span="6">
        <el-select v-model="statusFilter" placeholder="状态筛选" @change="fetchCalendar">
          <el-option label="全部" value="" />
          <el-option label="待播出" value="pending" />
          <el-option label="已入库" value="available" />
          <el-option label="缺失" value="missing" />
        </el-select>
      </el-col>
      <el-col :span="6">
        <el-button type="primary" @click="refreshCalendar">刷新日历</el-button>
        <el-button @click="manageSubscriptions">管理订阅</el-button>
      </el-col>
    </el-row>

    <!-- 日历视图 -->
    <el-calendar v-model="currentDate">
      <template #date-cell="{ data }">
        <div class="calendar-day">
          <div class="date">{{ data.day.split('-').slice(2).join('') }}</div>
          <div class="episodes">
            <div
              v-for="episode in getEpisodesForDate(data.day)"
              :key="episode.id"
              :class="['episode-badge', `status-${episode.status}`]"
              @click="showEpisodeDetail(episode)"
            >
              {{ episode.showName }} S{{ episode.season }}E{{ episode.episode }}
            </div>
          </div>
        </div>
      </template>
    </el-calendar>

    <!-- 订阅管理对话框 -->
    <el-dialog v-model="subscriptionDialogVisible" title="管理订阅">
      <el-input
        v-model="searchKeyword"
        placeholder="搜索剧集（TMDB）"
        @input="searchTMDB"
      />
      <el-table :data="subscriptions" style="margin-top: 20px">
        <el-table-column prop="showName" label="剧集名称" />
        <el-table-column prop="tmdbId" label="TMDB ID" width="100" />
        <el-table-column label="操作" width="100">
          <template #default="{ row }">
            <el-button type="danger" size="small" @click="unsubscribe(row.tmdbId)">
              取消订阅
            </el-button>
          </template>
        </el-table-column>
      </el-table>
    </el-dialog>
  </div>
</template>

<style scoped>
.episode-badge {
  font-size: 12px;
  padding: 2px 4px;
  margin: 2px 0;
  border-radius: 4px;
  cursor: pointer;
}
.status-pending { background: #e6a23c; color: white; }
.status-available { background: #67c23a; color: white; }
.status-missing { background: #f56c6c; color: white; }
</style>
```

---

## 核心逻辑实现

### 服务层关键方法

```go
// services/api/internal/services/tv_calendar_service.go

// FetchCalendar 获取追剧日历
func (s *TVCalendarService) FetchCalendar(ctx context.Context, startDate, endDate time.Time, status string, userID *uint) ([]models.TVCalendar, error)

// RefreshCalendar 刷新日历（从 TMDB 同步）
func (s *TVCalendarService) RefreshCalendar(ctx context.Context, tmdbID *int, force bool) (int, error)

// syncShowFromTMDB 从 TMDB 同步单个剧集
func (s *TVCalendarService) syncShowFromTMDB(ctx context.Context, tmdbID int, force bool) (int, error)

// updateCalendarStatus 更新日历状态（与 Emby 比对）
func (s *TVCalendarService) updateCalendarStatus(ctx context.Context) error

// HandleWebhook 处理 Emby Webhook（新剧集入库）
func (s *TVCalendarService) HandleWebhook(ctx context.Context, event string, item map[string]interface{}) error
```

### TMDB 客户端

```go
// services/api/pkg/tmdb/client.go

type Client struct {
    apiKey     string
    httpClient *http.Client
}

func (c *Client) GetTVShowDetails(ctx context.Context, tmdbID int) (*TVShow, error)
func (c *Client) GetSeasonDetails(ctx context.Context, tmdbID, seasonNumber int) (*SeasonDetails, error)
```

---

## 验证方式

### 1. 数据库迁移

```bash
cd services/api
go run cmd/migrate/main.go create add_tv_calendar_tables
```

### 2. API 测试

```bash
# 订阅剧集
curl -X POST http://localhost:8080/api/tv-calendar/subscribe \
  -H "Authorization: Bearer <token>" \
  -d '{"tmdbId": 1399, "showName": "Game of Thrones"}'

# 获取日历
curl "http://localhost:8080/api/tv-calendar?startDate=2026-03-01&endDate=2026-03-31" \
  -H "Authorization: Bearer <token>"

# 刷新日历
curl -X POST http://localhost:8080/api/tv-calendar/refresh \
  -H "Authorization: Bearer <token>"
```

### 3. 前端测试

- 访问 `/tv-calendar` 页面
- 测试日期范围筛选
- 测试状态筛选（待播出/已入库/缺失）
- 测试订阅管理（添加/删除订阅）
- 测试日历刷新

### 4. Webhook 测试

```bash
curl -X POST http://localhost:8080/api/webhooks/emby/library-update \
  -H "X-Webhook-Secret: <secret>" \
  -d '{
    "event": "library.new",
    "item": {
      "id": "abc123",
      "type": "Episode",
      "seasonNumber": 3,
      "episodeNumber": 5,
      "providerIds": {"Tmdb": "1399"}
    }
  }'
```

---

## 环境变量配置

```env
# .env
TMDB_API_KEY=your_tmdb_api_key_here
TMDB_LANGUAGE=zh-CN
WEBHOOK_SECRET=your_webhook_secret_here
```

---

## 实施清单

- [ ] 创建数据模型（3 个表）
- [ ] 编写数据库迁移脚本
- [ ] 实现 TMDB 客户端
- [ ] 实现服务层逻辑
- [ ] 实现 API 端点（5 个）
- [ ] 实现前端页面
- [ ] 配置 Webhook
- [ ] 编写单元测试
- [ ] 编写集成测试
- [ ] 更新系统架构文档

**预计工作量**：5-7 天
