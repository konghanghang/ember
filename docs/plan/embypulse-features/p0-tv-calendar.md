# P0-1: 追剧日历（TV Calendar）

## 功能定位

将 Ember 当前“按用户订阅驱动”的追剧日历，调整为更接近 `emby-pulse` 的“全库自动发现 + 周历展示 + 后台同步 + Webhook 即时点亮”模式。

目标不是简单复刻 UI，而是纠正数据入口：

- `emby-pulse` 的主数据源是 Emby 库内所有正在连载的剧集
- Ember 当前主数据源是 `tv_calendar_subscriptions`
- 这两种模型会直接导致完全不同的用户体验

当前 Ember 的真实问题不是“功能没写”，而是默认路径太差：

- 用户未订阅任何剧时，页面天然为空
- 用户订阅了，但默认日期区间没命中时，页面仍然为空
- TMDB 不可用时，旧实现甚至会把已有缓存数据一并挡掉

这套行为和 `emby-pulse` 的“打开就有结果”相反，因此本次文档更新的核心任务是把产品模型纠正过来。

**优先级**：P0

---

## 目标

### 核心目标

1. 默认展示全局追剧日历，不要求用户先手动订阅
2. 自动从 Emby 中发现 `Continuing` 状态且带 `Tmdb` Provider ID 的剧集
3. 以后端定时同步为主，页面访问只做轻量查询，不承担重型首刷
4. 保留用户“关注/收藏剧集”的能力，但不再让其驱动主数据
5. 保留 Webhook 点亮状态能力，减少“已入库但页面还在红灯”的延迟

### 非目标

1. 本期不做跨周无限滚动，只支持 `上周 / 本周 / 下周`
2. 本期不做播放页深链跳转增强，先保留后续扩展位
3. 本期不废弃现有订阅接口，只做语义降级和前端入口调整

---

## 与 emby-pulse 的关键差异

### emby-pulse 当前模型

- 自动扫描 Emby 中所有 `Continuing` 剧集
- 以“周”为单位拉取 TMDB 排期
- 后台每 12 小时同步本周和下周
- 页面默认展示全局结果
- 用户不需要先做任何订阅动作

### Ember 当前模型

- 仅查询当前用户已订阅的 `tmdbId`
- 以“开始日期 / 结束日期”查询平铺表格
- 无后台自动同步，主要依赖访问时按需刷新
- 页面空数据原因混杂
- 用户需要手工输入 `tmdbId` 才能建立可见数据

### 本次改造结论

- 全局自动日历应成为主路径
- 用户关注列表应成为附加过滤维度
- 旧订阅接口保留，但不再承担“让页面默认有数据”的职责

---

## 设计原则

### 1. 好品味

消灭“先订阅才能看日历”这个多余前置条件。全局日历应当基于库内事实自动生成，而不是基于用户是否事先填过一张关注表。

### 2. Never break userspace

不能直接删除现有接口或清空现有数据。必须新增全局主链路，并让旧接口继续可用：

- `tv_calendar_subscriptions` 保留
- 现有订阅/取消订阅接口保留
- 现有数据继续有效

### 3. 实用主义

优先做出 `emby-pulse` 最重要的能力：

- 默认可见数据
- 周视图
- 自动同步
- Webhook 点亮

不要在 P0 阶段引入过多花哨交互。

### 4. 简洁

后端职责拆清楚：

- 发现源
- 同步条目
- 查询视图
- 关注过滤
- Webhook 状态更新

不要让一个 `FetchCalendar` 既负责刷新、又负责聚合、又负责用户过滤、又负责异常兼容。

---

## Ember 对齐约束

1. 所有主键使用 `string`（CUID），不使用 `uint`
2. 列名使用 camelCase（如 `tmdbId`、`airDate`）
3. API 统一前缀 `/api/v1`
4. 前端路由挂载到 `/console/*`
5. Webhook 统一挂载 `/api/v1/webhooks/*`
6. 列表响应统一使用 `data` 字段
7. 文档中的兼容方案优先于“一次性推翻重写”

---

## 新的功能模型

### 主模型：全局追剧日历

系统自动扫描 Emby 中正在连载的剧集，生成“上周 / 本周 / 下周”的周历数据。用户进入页面时默认看到的是全局结果。

### 附加模型：我的关注

用户可以将全局结果中的某部剧加入“我的关注”，随后切换到“我的关注”视图，只看自己关心的剧。

### 状态来源

- `ready`：Emby 已存在物理文件
- `missing`：已播出但未入库
- `today`：今日播出
- `upcoming`：未来播出

---

## 数据模型设计

### 1. 连载剧源表 `tv_calendar_sources`

用于记录“系统发现到的连载中剧集”，这是全局日历的源数据。

```go
type TVCalendarSource struct {
    ID           string    `json:"id" gorm:"column:id;type:varchar(25);primaryKey"`
    TmdbID       string    `json:"tmdbId" gorm:"column:tmdbId;size:50;not null;uniqueIndex"`
    SeriesID     string    `json:"seriesId" gorm:"column:seriesId;size:50;index"`
    ShowName     string    `json:"showName" gorm:"column:showName;size:255;not null"`
    PosterURL    string    `json:"posterUrl" gorm:"column:posterUrl;size:500"`
    Overview     string    `json:"overview" gorm:"column:overview;type:text"`
    EmbyStatus   string    `json:"embyStatus" gorm:"column:embyStatus;size:20;not null;default:'continuing'"`
    LastSyncedAt *time.Time `json:"lastSyncedAt,omitempty" gorm:"column:lastSyncedAt"`
    CreatedAt    time.Time `json:"createdAt" gorm:"column:createdAt;autoCreateTime"`
    UpdatedAt    time.Time `json:"updatedAt" gorm:"column:updatedAt;autoUpdateTime"`
}
```

约束：

- `tmdbId` 全局唯一
- `seriesId` 允许为空，但一旦识别成功需要写回
- 该表只表达“全局发现源”，不承载用户维度

### 2. 日历条目表 `tv_calendar_items`

保留现有表，但语义调整为“全局排期缓存条目”，不再依赖用户订阅。

```go
type TVCalendarItem struct {
    ID          string    `json:"id" gorm:"column:id;type:varchar(25);primaryKey"`
    TmdbID      string    `json:"tmdbId" gorm:"column:tmdbId;size:50;not null;index;uniqueIndex:uk_tv_calendar_episode,priority:1"`
    SeriesID    string    `json:"seriesId" gorm:"column:seriesId;size:50;index"`
    Season      int       `json:"season" gorm:"column:season;not null;uniqueIndex:uk_tv_calendar_episode,priority:2"`
    Episode     int       `json:"episode" gorm:"column:episode;not null;uniqueIndex:uk_tv_calendar_episode,priority:3"`
    AirDate     time.Time `json:"airDate" gorm:"column:airDate;not null;index"`
    EpisodeName string    `json:"episodeName" gorm:"column:episodeName;size:255"`
    Overview    string    `json:"overview" gorm:"column:overview;type:text"`
    Status      string    `json:"status" gorm:"column:status;size:20;not null;default:'upcoming'"`
    EmbyItemID  string    `json:"embyItemId,omitempty" gorm:"column:embyItemId;size:50"`
    LastChecked time.Time `json:"lastChecked" gorm:"column:lastChecked"`
    CreatedAt   time.Time `json:"createdAt" gorm:"column:createdAt;autoCreateTime"`
    UpdatedAt   time.Time `json:"updatedAt" gorm:"column:updatedAt;autoUpdateTime"`
}
```

约束：

- 唯一索引：`(tmdbId, season, episode)`
- `airDate` 统一归一化为 UTC `00:00:00`
- `status=ready` 时，常规同步不得覆盖为其他状态，只有物理校验确认缺失或管理员强制同步时才允许回退

### 3. 用户关注表 `tv_calendar_subscriptions`

保留现有表，但语义从“主数据源”调整为“我的关注”。

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
- 该表仅负责关注过滤，不参与“全局源发现”

### 4. TMDB 缓存表 `tmdb_cache`

保留现有表，用于 TMDB 响应持久化缓存。

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

## 同步链路设计

### 1. 全局发现

从 Emby 拉取所有：

- `IncludeItemTypes=Series`
- `Status=Continuing`
- 带 `ProviderIds.Tmdb`
- 非虚拟条目

写入 `tv_calendar_sources`。

### 2. 周历同步

按周同步，仅处理：

- 上周 `weekOffset=-1`
- 本周 `weekOffset=0`
- 下周 `weekOffset=1`

每部剧只抓目标季：

- `last_episode_to_air.season_number`
- `next_episode_to_air.season_number`
- 如果两者都不存在，回退到最后一季

这样做是为了接近 `emby-pulse` 的成本控制，不在 P0 阶段把整个历史季都重刷一遍。

### 3. 物理文件校验

与 Emby 对齐的硬规则：

- `LocationType == Virtual` 视为未入库
- `IsMissing == true` 视为未入库
- 必须有 `Path` 或 `MediaSources`

### 4. Webhook 联动

`POST /api/v1/webhooks/emby?token=<WEBHOOK_TOKEN>`

处理：

- `library.new`
- `item.added`
- 仅 `Episode`

Webhook 只做轻量状态点亮：

- 命中 `(tmdbId, season, episode)` 或 `(seriesId, season, episode)`
- 更新 `status=ready`
- 写回 `embyItemId`

Webhook 不负责重型拉取。

### 5. 后台定时任务

基于项目已有 cron 机制增加任务：

- 服务启动后延迟执行一次全局发现与周历同步
- 每 12 小时刷新一次 `上周 / 本周 / 下周`

页面访问不再承担主同步逻辑，只允许：

- 读缓存
- 在缓存过期时做轻量补偿刷新

---

## API 设计

### 1. 获取全局周历

`GET /api/v1/tv-calendar/global`

Query：

- `weekOffset`：`-1 | 0 | 1`
- `status`：可选，`ready | missing | upcoming | today`

Response：

```json
{
  "data": {
    "dateRange": "03/09 - 03/15",
    "days": [
      {
        "date": "2026-03-11",
        "weekdayCn": "周三",
        "isToday": true,
        "items": [
          {
            "tmdbId": "1399",
            "seriesId": "emby_series_xxx",
            "showName": "Game of Thrones",
            "posterUrl": "https://...",
            "season": 3,
            "episode": "1-2",
            "airDate": "2026-03-11",
            "status": "missing",
            "episodeName": "",
            "overview": "..."
          }
        ]
      }
    ]
  }
}
```

说明：

- 周历视图负责按天分组
- 同日同剧同季连续集数允许聚合为 `"1-2"`

### 2. 获取我的关注周历

`GET /api/v1/tv-calendar/following`

Query：

- `weekOffset`：`-1 | 0 | 1`
- `status`：可选

说明：

- 在全局结果基础上，按当前用户的 `tv_calendar_subscriptions` 过滤
- 返回结构与全局周历一致

### 3. 获取我的关注列表

`GET /api/v1/tv-calendar/subscriptions`

保留现有接口。

### 4. 关注剧集

`POST /api/v1/tv-calendar/subscriptions`

```json
{
  "tmdbId": "1399",
  "showName": "Game of Thrones",
  "posterUrl": "https://..."
}
```

说明：

- 保留现有接口
- 允许从全局周历卡片一键关注

### 5. 取消关注

`DELETE /api/v1/tv-calendar/subscriptions/:tmdbId`

保留现有接口。

### 6. 管理员手动同步

`POST /api/v1/admin/tv-calendar/sync`

```json
{
  "weekOffsets": [-1, 0, 1],
  "force": false,
  "tmdbId": "1399"
}
```

说明：

- `tmdbId` 可选，传入时只同步单剧
- `weekOffsets` 可选，默认同步 `[-1, 0, 1]`
- 空 body 视为同步全部全局源

### 7. 兼容接口策略

现有 `GET /api/v1/tv-calendar` 不立刻删除，改造阶段保持兼容：

- 方案 A：继续返回旧的“日期区间平铺表格”
- 方案 B：内部复用新数据源，仅保留旧响应格式

P0 推荐方案 B。

---

## 服务层拆分建议

```go
// DiscoverContinuingSeries 从 Emby 发现所有正在连载且带 TMDB ID 的剧集
func (s *TVCalendarService) DiscoverContinuingSeries(ctx context.Context) (int, error)

// SyncWeeklyCalendar 同步指定周偏移的全局周历数据
func (s *TVCalendarService) SyncWeeklyCalendar(ctx context.Context, weekOffset int, tmdbID *string, force bool) (int, error)

// GetGlobalWeeklyCalendar 查询全局周历视图
func (s *TVCalendarService) GetGlobalWeeklyCalendar(ctx context.Context, weekOffset int, status string) (*TVCalendarWeeklyDTO, error)

// GetFollowingWeeklyCalendar 查询当前用户关注周历视图
func (s *TVCalendarService) GetFollowingWeeklyCalendar(ctx context.Context, userID string, weekOffset int, status string) (*TVCalendarWeeklyDTO, error)

// Subscribe 关注剧集
func (s *TVCalendarService) Subscribe(userID string, req CreateTVCalendarSubscriptionRequest) error

// Unsubscribe 取消关注
func (s *TVCalendarService) Unsubscribe(userID, tmdbID string) error

// MarkEpisodeReadyByWebhook Webhook 点亮剧集状态
func (s *TVCalendarService) MarkEpisodeReadyByWebhook(ctx context.Context, tmdbID, seriesID string, season, episode int, embyItemID string) (int64, error)
```

---

## 前端页面改造

### 路由

保持：

```ts
{
  path: '/console/tv-calendar',
  name: 'console-tv-calendar',
  meta: { requiresAuth: true },
  component: () => import('@/views/console/TVCalendarView.vue')
}
```

### 页面模式

页面默认进入“全部在更”。

顶部能力：

- 视图切换：`全部在更` / `我的关注`
- 周切换：`上周` / `本周` / `下周`
- 状态筛选
- 管理员可见“手动同步”按钮

卡片能力：

- 显示海报、剧名、季集、播出日、状态
- 从“全部在更”卡片一键加入关注
- 从“我的关注”卡片取消关注

空态要求：

- 全局模式无数据：提示“当前服务器中没有识别到正在连载的剧集”
- 我的关注无数据：提示“你还没有关注任何剧集”
- TMDB 不可用：明确提示配置问题

禁止继续使用“纯表格 + 手动填 tmdbId 才能有内容”作为默认交互。

---

## 迁移计划

### 阶段 1：后端数据层改造

1. 新增 `tv_calendar_sources`
2. 保留 `tv_calendar_items`
3. 保留 `tv_calendar_subscriptions`
4. 编写数据迁移与 AutoMigrate

### 阶段 2：发现与同步链路

1. 实现 `DiscoverContinuingSeries`
2. 实现 `SyncWeeklyCalendar`
3. 增加启动后延迟同步
4. 增加 12 小时 cron 同步

### 阶段 3：查询接口

1. 新增 `/api/v1/tv-calendar/global`
2. 新增 `/api/v1/tv-calendar/following`
3. 保持旧订阅接口不变
4. 兼容旧 `/api/v1/tv-calendar`

### 阶段 4：前端重构

1. 周历栅格视图替换表格视图
2. 默认展示全局模式
3. 增加“我的关注”切换
4. 增加一键关注/取消关注

### 阶段 5：联动与补偿

1. Webhook 点亮状态
2. 管理员手动同步
3. TMDB 不可用时保持旧缓存可读

---

## 兼容性要求

1. 不删除现有 `tv_calendar_subscriptions` 数据
2. 不删除现有关注接口
3. 旧页面书签仍然指向 `/console/tv-calendar`
4. 后端在 TMDB 不可用时，如果已有缓存，必须优先返回缓存数据
5. 旧 `GET /api/v1/tv-calendar` 在迁移期继续可用

---

## 风险与应对

### 风险 1：全库扫描成本过高

应对：

- 只扫描 `Continuing`
- 只抓目标季
- 只同步 `上周 / 本周 / 下周`

### 风险 2：Emby `Status` 不可靠

应对：

- P0 先按 `Continuing` 实现
- 后续可补“活跃剧集兜底发现”策略

### 风险 3：TMDB 不可用导致页面彻底空白

应对：

- 后端查询优先返回持久化缓存
- 前端必须把“缓存展示”和“同步失败”区分开

### 风险 4：旧接口与新模型并存导致代码发散

应对：

- 新接口统一读同一套全局数据源
- 旧接口只做兼容适配层，不允许另起一套逻辑

---

## 验收标准

- [ ] 不做任何用户关注操作，进入追剧日历页即可看到全局在更剧集
- [ ] 页面默认按周展示，而不是平铺表格
- [ ] 支持查看上周、本周、下周
- [ ] “我的关注”只在全局结果基础上做过滤，不驱动主数据
- [ ] 后端启动后可自动同步周历数据
- [ ] 每 12 小时自动刷新一次周历数据
- [ ] TMDB 不可用时，已有缓存仍可展示
- [ ] Webhook 到达后目标剧集状态可即时点亮
- [ ] 物理文件校验能正确过滤虚拟占位符和缺失条目
- [ ] 现有关注接口和数据不被破坏

---

## 工作量评估

**预计工作量**：8-12 天

拆分建议：

- 后端数据模型与同步链路：3-4 天
- 查询接口与兼容层：2-3 天
- 前端周历页面重构：2-3 天
- 联调、回归、文档补齐：1-2 天
