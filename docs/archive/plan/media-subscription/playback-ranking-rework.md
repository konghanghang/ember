# 播放排行榜最小重构方案

> 状态：草稿
> 负责人：AI
> 更新时间：2026-03-21

## 背景

- 当前播放排行榜已经落地，但“最新榜”“历史榜”“预览榜”三套行为的时间语义不一致，用户可见结果会互相打架
- `latest` 和 `history` 都围绕 `snapshot_at` 查询，而不是围绕统计周期查询，管理员补算历史区间时会污染最新榜
- 电影榜和剧集榜当前分别按 `category` 各自取“最新一份”，前端再自行拼装，存在同一页面混入两期数据的风险
- 排行聚合键当前使用 `ItemName`，同名电影、重制版、特别篇或剧集重名内容会被合并，统计结果不可靠
- 如果不修，后续继续加海报、跳转、详情页、Bot 增强推送，只会把错误语义继续放大

## 目标

1. 定义“同一期排行榜”的稳定身份，消除 `snapshotAt` 与统计周期混用的问题
2. 让最新榜、历史榜、预览榜三条链路的时间语义一致，查询围绕统计周期而不是生成时刻
3. 保证电影榜和剧集榜总是来自同一个排行榜批次，不允许前端拼接不同批次的数据
4. 将排行聚合键从 `ItemName` 改为稳定媒体标识，消除同名内容误合并
5. 在不改变页面入口和 Bot 推送能力的前提下，完成最小范围重构

## 非目标

- 不重做排行榜页面视觉设计
- 不引入实时排行榜或流式更新
- 不新增用户个人排行、设备排行、客户端排行
- 不改 cron 默认时间策略，仍保持日榜 `20:00`、周榜周日 `20:30`
- 不扩展 Telegram 消息样式，只修正其数据来源

## 当前事实

- 相关文档：
  - `docs/system-architecture.md`
  - `docs/archive/plan/playback-ranking.md`
- 相关服务/页面/模型：
  - `services/api/internal/models/playback_ranking.go`
  - `services/api/internal/services/playback/ranking.go`
  - `services/api/internal/handlers/ranking.go`
  - `services/api/internal/app/cron.go`
  - `services/web/src/views/console/RankingsView.vue`
  - `services/bot/app/formatters/message_formatter.py`
- 当前行为：
  - 定时任务按 `daily` / `weekly` 从 Emby `PlaybackActivity` 计算电影和剧集 `TOP 10`
  - 计算结果落到 `playback_rankings`
  - Web 的“最新榜”按 `period + category` 读取 `MAX(snapshot_at)` 的那一批行
  - Web 的“历史榜”按用户选中的日期换算自然日或自然周，再读取该范围内 `MAX(snapshot_at)` 的快照
  - 管理员“预览榜”直接即时查询 Emby，不入库、不推送
- 现有限制：
  - “最新榜”可能被历史补算结果污染
  - 电影榜和剧集榜可能不是同一次生成结果
  - 历史榜查的是“当时最后一次生成动作”，不是“该统计周期本身”
  - 聚合键不稳定，无法安全扩展海报、详情跳转和去重逻辑

## 方案设计

### 1. 用户可见行为

- 控制台仍保留：
  - `日榜`
  - `周榜`
  - `查看历史`
  - 管理员 `预览生成`
- “最新榜”语义改为：
  - 展示最近一个已完成并可发布的统计周期
  - 若当前周期尚未生成，则回退展示上一期已完成周期
  - 页面继续显示真实的 `periodStart ~ periodEnd` 和 `cutoffAt`
- “历史榜”语义改为：
  - 用户选择某天或某周后，返回该统计周期对应的一期榜单
  - 如果该周期尚无榜单，则返回空结果和该周期范围，不再按 `snapshotAt` 模糊猜测
- “预览榜”语义保持不变：
  - 仅管理员可用
  - 只计算当前周期，不入库、不推送
- Bot 推送仍只推送正式生成且已发布的榜单

### 2. 数据与模型

本次涉及数据模型变更。

#### 2.1 为排行榜行新增批次标识

在 `playback_rankings` 上新增字段：

| 字段 | 类型 | 说明 |
|------|------|------|
| `batchId` | `varchar(25)` | 同一次生成的批次号，电影榜和剧集榜共享同一个值 |
| `itemKey` | `varchar(128)` | 稳定聚合键，电影优先用 `ItemId`，剧集优先用 `SeriesId`，缺失时回退 `ItemId` |
| `itemSourceType` | `varchar(32)` | 记录聚合键来源，例如 `movie_item`、`series`、`episode_item` |

保留现有字段：

- `period`
- `category`
- `rank`
- `itemName`
- `playCount`
- `duration`
- `snapshotAt`
- `periodStart`
- `periodEnd`

#### 2.2 索引调整

新增或调整索引：

- `idx_ranking_batch(batch_id, category, rank)`
- `idx_ranking_period_window(period, period_start, period_end, snapshot_at)`
- `idx_ranking_item(period, category, item_key, period_start, period_end)`

目的：

- 按批次一次性读取同一期电影榜和剧集榜
- 按统计周期定位最新榜和历史榜
- 为后续去重和核查提供稳定检索键

#### 2.3 数据迁移策略

- 数据库迁移只补新字段和索引，不删旧字段
- 旧数据不做全量回填 `itemKey`
- 旧数据继续可读，但会被标记为“旧格式快照”，不再作为最新榜候选
- 新版发布后，至少重新生成一次日榜和一次周榜，建立首批带 `batchId` / `itemKey` 的正式快照

> 原则：不做脏回填，不猜旧数据的真实媒体标识。旧数据保留审计价值，但不冒充新语义数据。

### 3. 接口与边界

#### 3.1 统一排行榜响应结构

后端新增统一响应对象，供 `latest`、`history`、`preview` 共用：

| 字段 | 说明 |
|------|------|
| `period` | `daily` 或 `weekly` |
| `batchId` | 正式榜单批次 ID；预览接口可为空 |
| `snapshotAt` | 本次计算时间 |
| `periodStart` | 统计起始 |
| `periodEnd` | 统计结束 |
| `cutoffAt` | 页面展示用截止时间 |
| `movies` | 电影排行数组 |
| `episodes` | 剧集排行数组 |

排行项新增字段：

| 字段 | 说明 |
|------|------|
| `rank` | 排名 |
| `itemKey` | 稳定媒体键 |
| `itemName` | 展示名称 |
| `playCount` | 播放次数 |
| `duration` | 总播放时长 |

#### 3.2 对外 API 调整

保留现有路由，不扩大入口：

- `GET /api/v1/rankings/latest`
- `GET /api/v1/rankings/history`
- `POST /api/v1/admin/rankings/preview`
- `POST /api/v1/admin/cron/generate-ranking`

行为调整如下：

- `GET /rankings/latest`
  - 不再要求 `category`
  - 直接返回同一批次下的 `movies + episodes`
  - 查询规则：按当前时刻推导目标周期；若当前周期无已发布批次，则回退上一已完成周期
- `GET /rankings/history`
  - 继续接收 `period + date`
  - 改为按推导出的 `periodStart + periodEnd` 精确查找对应批次
  - 不再按 `snapshot_at` 落在哪一天或哪一周来猜
- `POST /admin/rankings/preview`
  - 返回统一响应结构
  - 不写库、不生成 `batchId`
- `POST /admin/cron/generate-ranking`
  - 继续支持 `type`
  - 保留 `start` / `end`
  - 明确生成的是一批正式榜单，写入同一个 `batchId`
  - 若生成范围不是当前或上一已完成周期，则不自动覆盖 latest 候选

#### 3.3 前端调用边界

`RankingsView.vue` 改为单请求模型：

- 最新榜：调用一次 `getLatestRanking(period)`
- 历史榜：调用一次 `getRankingHistory(period, date)`
- 预览榜：调用一次 `previewRanking(period)`

前端不再分电影和剧集分别请求，更不再自行假设两组数据属于同一期。

#### 3.4 Bot 边界

- Bot 内部通知格式保持现有字段，不增加复杂格式
- API 侧仅在正式生成成功后发送 Bot 通知
- 预览和无发布资格的历史补算不触发 Bot 推送

### 4. 关键流程

#### 4.1 预检：校验 PlaybackActivity 可用列

在排行生成和预览前新增轻量校验：

1. 读取 `PlaybackActivity` 可查询列
2. 校验电影排行需要的稳定字段：
   - `ItemId`
   - `ItemName`
3. 校验剧集排行优先字段：
   - `SeriesId`
   - `SeriesName`
   - 若缺 `SeriesId`，允许退化到 `ItemId`
4. 如稳定键字段缺失，直接返回明确错误，拒绝继续按 `ItemName` 聚合

> 不允许静默回退到 `ItemName` 聚合。缺字段就暴露问题，而不是继续制造错误榜单。

#### 4.2 生成正式榜单

1. 根据 `period` 和可选 `start/end` 计算统计窗口
2. 生成一个新的 `batchId`
3. 电影榜按稳定键聚合：
   - 分组键：`ItemId`
   - 展示名：`ItemName`
4. 剧集榜按稳定键聚合：
   - 分组键：`COALESCE(SeriesId, ItemId)`
   - 展示名优先 `SeriesName`，否则退回 `ItemName`
5. 为电影榜和剧集榜所有条目写入同一个 `batchId`
6. 根据统计窗口判断该批次是否可作为 latest 候选
7. 如果是正式可发布批次，异步通知 Bot

#### 4.3 查询最新榜单

1. 读取当前时刻和时区
2. 先推导“当前周期”的理论 `periodStart/periodEnd`
3. 在正式榜单中查找与该周期完全匹配的最新批次
4. 若不存在，则回退到上一已完成周期对应的最新批次
5. 依据 `batchId` 一次性取出 `movies + episodes`

#### 4.4 查询历史榜单

1. 将用户选择的 `date` 推导为日榜或周榜的目标周期
2. 用 `period + periodStart + periodEnd` 精确查找批次
3. 若存在，按 `batchId` 返回该期榜单
4. 若不存在，返回空榜和该周期范围

#### 4.5 查询预览榜单

1. 按当前周期即时计算
2. 不写库
3. 不生成正式 `batchId`
4. 直接返回统一结构，供前端展示“预览中”

### 5. 失败路径与边界条件

- Playback Reporting 插件不可用：
  - `preview` 和 `generate` 直接失败并返回明确错误
  - `latest` / `history` 继续读取已落库快照，不受影响
- `PlaybackActivity` 缺少 `ItemId` / `SeriesId` 等稳定键列：
  - 拒绝继续生成
  - 错误信息明确指出缺失列名
- 电影榜有数据、剧集榜为空，或相反：
  - 仍写入同一 `batchId`
  - 返回时允许某一类为空，但批次元信息必须一致
- 补算历史区间：
  - 允许写入正式快照
  - 不得因为 `snapshotAt` 更新而污染最新榜查询结果
- 兼容性约束：
  - 不改变日榜、周榜、历史查看、管理员预览这四个页面入口
  - 不改变 Bot 文案结构
  - 不改变 cron 默认计划和时区配置入口

## 影响范围

- API：有
  - `PlaybackRanking` 模型新增字段与索引
  - `PlaybackRankingService` 查询与生成逻辑重写
  - `RankingHandler` 返回结构调整
- Web：有
  - `services/web/src/api/console.ts`
  - `services/web/src/api/admin.ts`
  - `services/web/src/types/api.ts`
  - `services/web/src/views/console/RankingsView.vue`
- Bot：有
  - 数据来源语义变更，但消息格式基本不变
  - `services/bot/app/formatters/message_formatter.py` 原则上仅需适配新增字段时忽略即可
- 配置/部署：低影响
  - 不新增环境变量
  - 需要数据库迁移
  - 需要确认 Emby Playback Reporting 插件暴露稳定媒体字段
- 文档：需要更新
  - `docs/system-architecture.md`
  - 如实现中补充了对插件字段的要求，应同步到 `docs/reference/emby-api-guide.md`

## 验证方式

### 编译/测试

- `cd services/api && go test ./...`
- `cd services/api && go build ./...`
- `cd services/web && npm run build`
- `cd services/bot && python -m py_compile main.py`

### 手工验证

1. 定时或手动生成当天日榜，确认：
   - 电影榜和剧集榜返回同一个 `batchId`
   - 页面展示的 `periodStart`、`periodEnd`、`cutoffAt` 一致
2. 手动补算一个历史日期范围，确认：
   - 新增历史快照成功
   - 最新榜仍保持当前或上一已完成周期，不被补算污染
3. 选择某天查看历史，确认：
   - 若该周期存在正式榜单，返回精确对应那一期
   - 若不存在，返回空榜和正确周期范围
4. 模拟电影有榜、剧集无榜的场景，确认页面不会拼接两期数据
5. 使用存在同名内容的测试数据，确认榜单按稳定媒体标识分开统计
6. 触发正式生成后检查 Bot 推送，确认消息日期范围和 Web 页面一致

## 落地后文档处理

- 将新的排行榜快照语义、接口结构和 cron 行为更新到 `docs/system-architecture.md`
- 如实现中补充了对 Playback Reporting 插件字段的硬性要求，同步更新 `docs/reference/emby-api-guide.md`
- 本方案落地后移入 `docs/archive/`，保留旧的首次实现方案作为历史记录
- 若本方案顺利落地，将本文移入 `docs/archive/`
- 旧的 `docs/archive/plan/playback-ranking.md` 保留为首次实现方案，不删除
