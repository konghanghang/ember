# 订阅状态机与 webhook 命中精度加固方案

> 状态：主干完成，保留尾项（P0 + P1 已落地）
> 负责人：Ember
> 更新时间：2026-04-28

## 落地进度

批次 2 + 后续 review 修复已完成本方案的大部分主干项：

- ✅ 订阅审批 / 拒绝 / 管理员手动收口改为原子状态转移，避免旧快照覆盖新状态
- ✅ 创建 / 重提交改为 advisory lock + 活跃唯一索引幂等收口
- ✅ `RedispatchSubscription`、`DISPATCH_FAILED`、`lastDispatchError`、`media_gap_scans` 已落地
- ✅ `MarkSubscriptionIngestedAsAdmin` 与 webhook 路径用户通知对齐
- ✅ handler 内部错误收口到统一错误响应，不再裸透 SQL 错误
- ✅ 缺集扫描跨副本互斥已通过 PostgreSQL advisory lock + `media_gap_scans` 表落地
- ✅ review 补丁已收口：缺集扫描不再整行 `Save` 回滚并发状态；命中 `IGNORED` 时不再自动复活；系统忽略不再计入整剧人工排除分母

当前剩余项主要是模型 / 季选择治理尾项：

- `ignoreReasonCode` 尚未单独落库，当前仍以 `ignoreReason` 文本区分系统忽略与人工忽略
- `pickTargetSeasonNumbers` / 文档事实 / 观察性说明仍需继续收口

## 归档判断

- 当前不适合归档。
- 原因：`ignoreReasonCode` 和季选择治理仍未收口，继续保留在 `docs/plan/` 更符合当前职责边界。

## 背景

2026-04-25 系统性 review 在求片订阅 / 缺集（mediagap）链路暴露多类状态机硬伤，整体品味评分 🔴：

- 审批 / 拒绝 / 校验入库链路用 `Save(&subscription)` 全字段覆盖。当并发 Emby webhook 已经把同一条订阅写入 `INGESTED + ingestedAt`，审批路径 Save 会用旧的 status / mpError 把它打回 APPROVED，丢 `ingestedAt`，丢审计。
- `MarkSubscriptionsIngestedByWebhook` 用 `season = 0 OR season = ?` 匹配，整剧订阅（season=0）只要任意一集入库就被错误收口为 INGESTED；用户和管理员都以为整剧已完成。
- 缺集 webhook `MarkIngestedByWebhook` 无条件复活 `IGNORED` 工单：管理员的人工忽略决定被静默撤销，且 `ignoreReason / ignoredAt` 被清空，审计丢失。
- MoviePilot 调用失败仅记 `mpError`，无重试入口、无后台再下发、无告警，APPROVED 长期挂死，需要管理员手动 reject + 用户 resubmit 才能恢复。
- `hasActiveSubscription` 检查与 `Create` 之间无事务、无锁，并发"重新提交"或快速重试可能造出多条同 retryFromId 的边界状态；唯一索引兜底但错误信息不再是业务语义。
- 缺集异步扫描使用进程内 `mediaGapScanManager`，多副本部署时并发跑全库扫描，对 Emby / TMDB 流量做 N 倍放大。
- `MarkIngestedByWebhook` 对每条 gap 各自 `Save`，N 集批量入库触发 N 次 round-trip。
- `DispatchGapCandidate` 失败后 gap 状态不更新为可重试，前端只能依赖 toast；状态机里没有 `DISPATCH_FAILED`。
- handler 直接 `c.JSON(StatusInternalServerError, gin.H{"error": err.Error()})`，把内部 SQL 错误透传给客户端。
- `cleanupInactiveSeasonGaps` 把"季未激活"的工单一律改为 IGNORED 且 `ignoreReason` 是固定英文字符串，前端无法区分系统 IGNORE 与人工 IGNORE。
- `pickTargetSeasonNumbers` 仅取最近季，老剧补集时会漏季。
- `MarkSubscriptionIngestedAsAdmin` 路径不发 `notifyIngested`，与 webhook 路径用户体验不一致。
- 通知协程裸 `go`，无超时、无 panic recover；该问题与 `bot-telegram` 计划共用 `safeFireAndForget` 收口。

> 注：TMDB 密钥泄漏、追剧日历相关的链路问题集中在 `media-subscription/tv-calendar-and-tmdb-key-protection.md` 中处理，本计划只覆盖订阅与缺集状态机。

## 目标

本方案要实现：

1. 订阅审批 / 拒绝 / 校验入库 / 收口全部改为带 `WHERE status=?` 的 `Updates(map)`，原子状态转移 + 字段最小化
2. webhook 命中精度：把"整剧 (season=0)"与"指定季 (season=N)"在订阅匹配中拆开；整剧只在所有 TMDB 已播出集均存在前不自动 INGESTED
3. 缺集 webhook 命中已 IGNORED 工单时只记日志，不再静默复活；保留 `ignoreReason / ignoredAt` 审计
4. MoviePilot 调用失败可恢复：管理员订阅列表展示 `mpError`，提供"重新下发"接口；可选 cron 重试
5. 创建 / 重提交订阅：`hasActiveSubscription` 与 `Create` 包入事务，唯一冲突走业务幂等成功路径
6. 缺集异步扫描跨副本互斥：用 PostgreSQL advisory lock 或 `media_gap_scans` 表替代进程内 manager
7. 缺集状态机扩展 `DISPATCH_FAILED`，前端可重试
8. handler 错误响应统一过 `internalError(c, err)`，禁止裸透 SQL 错误
9. `MarkSubscriptionIngestedAsAdmin` 与 webhook 路径用户通知行为对齐
10. `cleanupInactiveSeasonGaps` 引入 `ignoreReasonCode` 字段区分人工 / 系统忽略
11. `pickTargetSeasonNumbers` 至少覆盖最近 2 季 + Next 季；force=true 时回退全季

## 非目标

本次明确不做：

- 不新增订阅"自动重审"入口（仅提供"重新下发"按钮）
- 不替换 MoviePilot 集成方式
- 不引入消息队列承接异步任务
- 不改 TMDB 搜索 / 候选海报展示
- 不重写追剧日历同步逻辑（独立计划承接）
- 不动 `subscriptions.note` 字段语义（接口必填、DB 仍允许空）

## 当前事实

以当前代码和现行文档为准，现状如下：

- 相关文档：`docs/system-architecture.md` §4.5 / §5.9 / §5.11 / §5.12
- 相关服务：
  - `services/api/internal/services/subscription/service.go`
  - `services/api/internal/services/mediagap/service.go`
  - `services/api/internal/services/mediagap/types.go`
  - `services/api/internal/services/mediagap/errors.go`
- 集成层：
  - `services/api/internal/integrations/moviepilot/client.go`
- 相关 handler：
  - `services/api/internal/handlers/subscription.go`
  - `services/api/internal/handlers/media_gap.go`
- webhook 入口：
  - Emby webhook handler（按 `tmdbId / seriesId / season / episode` 解析）
- 模型：
  - `services/api/internal/models/subscription.go`
  - `services/api/internal/models/media_gap.go`
- 涉及表：`subscriptions`、`media_gaps`、`media_gap_scans`（如有）
- 当前行为：
  - `subscriptions` 活跃唯一索引 `uq_subscriptions_active_media` 仅覆盖 `PENDING / APPROVED / INGESTED`
  - 拒绝订阅必须携带 `reason`
  - 用户可基于自己 `REJECTED` 记录调用 resubmit，新记录写 `retryFromId`
  - Emby webhook 命中已通过订阅时按"剧集 tmdbId + season"匹配
  - `mediaGapScanManager` 是进程内单实例锁
- 现有限制：
  - 线上 `AUTO_MIGRATE=false`
  - 通知 Bot 是火忘式，相关 recover 收口由 `bot-telegram` 计划提供
  - MoviePilot 失败时 `mpError` 仅写入数据库，没有跟进通知

## 方案设计

### 1. 用户可见行为

- 用户审批后再次访问订阅详情：`status / ingestedAt / mpError` 不会出现"已收口又被打回 APPROVED"
- 用户提交整剧订阅（season=0），只有所有 TMDB 已播出集都入库后才显示"已入库"；中间过程显示"下载中 X / Y 集"
- 管理员对单条缺集点"忽略"，后续 Emby 即使补片也不会自动复活该工单；UI 仍显示 IGNORED + 原 reason
- 管理员订阅列表对 MoviePilot 失败的 APPROVED 显式标红，提供"重新下发"按钮；点击后 status 不变，仅尝试再次调 MP，失败更新 `mpError`，成功清空
- 用户连点"重新提交"不会出现两条同 retryFromId 共存；超过去重窗口后第二次点击直接返回原订阅
- 管理员手动校验 Emby 后命中的订阅 INGESTED 也会触发用户通知，与 webhook 路径行为一致
- 缺集页面对"系统忽略（季未激活）"与"人工忽略"展示不同标签
- 错误响应不再出现 `pq: column "xxx" does not exist`

### 2. 数据与模型

#### `subscriptions` 表

新增字段：

| 字段 | 类型 | 列名 | 说明 |
|---|---|---|---|
| IngestProgress | *string(50) | `ingestProgress` | 整剧订阅的入库进度快照（如 `5/10`），可空 |

新增索引：

| 索引 | 类型 | 用途 |
|---|---|---|
| `(status, retryFromId)` | btree | 加速 retryFromId 链路查询 |

#### `media_gaps` 表

新增字段：

| 字段 | 类型 | 列名 | 说明 |
|---|---|---|---|
| IgnoreReasonCode | *string(50) | `ignoreReasonCode` | `manual` / `season_not_activated` / `dispatch_failed_giveup`，可空 |
| LastDispatchError | *string(500) | `lastDispatchError` | 最近一次 dispatch 失败原因 |

新增 / 修改状态机：

- 状态枚举增加 `DISPATCH_FAILED`
- `DispatchGap` 失败时写入 `DISPATCH_FAILED + lastDispatchError`，前端可触发重试
- webhook 命中 `DISPATCH_FAILED` 工单时正常收口为 `INGESTED`

#### 新增表 `media_gap_scans`

| 字段 | 类型 | 说明 |
|---|---|---|
| id | string(25) | CUID |
| status | string(20) | `running` / `success` / `failed` |
| nodeId | string(64) | 启动该次扫描的进程标识（hostname + pid） |
| startedAt | time.Time | 启动时间 |
| finishedAt | *time.Time | 结束时间 |
| errorMessage | *string(500) | 失败信息 |

并辅以 PostgreSQL advisory lock：`pg_try_advisory_lock(hashtext('media_gap_scan'))`，跨进程互斥。

#### Migration 命名

- `YYYYMMDD_NN_subscriptions_ingest_progress.sql`
- `YYYYMMDD_NN_media_gaps_ignore_reason_code.sql`
- `YYYYMMDD_NN_media_gaps_dispatch_failed.sql`
- `YYYYMMDD_NN_media_gap_scans.sql`

所有迁移幂等。

### 3. 接口与边界

- `PUT /api/v1/admin/subscriptions/:id/approve`
  - 请求体不变
  - 内部行为：`UPDATE subscriptions SET status='APPROVED', reviewedAt=now(), mpError=$err WHERE id=$id AND status='PENDING'`
- `PUT /api/v1/admin/subscriptions/:id/reject`
  - 请求体携带 `reason` 不变
  - 内部行为：`UPDATE ... SET status='REJECTED', reviewedAt=now(), rejectReason=$reason WHERE id=$id AND status='PENDING'`
  - **不动** `mpError`
- `PUT /api/v1/admin/subscriptions/:id/ingest`
  - 仅 `APPROVED` 可用（保留契约）
  - 校验命中 Emby 后：`UPDATE ... SET status='INGESTED', ingestedAt=now() WHERE id=$id AND status='APPROVED'`
  - 触发 `notifyIngested`（与 webhook 路径一致）
- 新增 `POST /api/v1/admin/subscriptions/:id/redispatch`
  - 触发条件：APPROVED 且 `mpError != null`
  - 内部行为：再次调 MoviePilot，更新 `mpError`，状态不变
- `POST /api/v1/subscriptions`、`POST /api/v1/subscriptions/:id/resubmit`
  - 入参不变
  - 服务层包入事务，`hasActiveSubscription` 与 `Create` 在同一事务里
  - 唯一索引冲突走幂等：返回当前活跃订阅 ID 与 status，HTTP 200
- Emby webhook handler
  - 整剧订阅命中策略详见 §4.2
  - mediagap 命中策略详见 §4.3
- 缺集异步扫描接口不变，但内部走 advisory lock
- handler 错误响应：所有 `c.JSON(500, gin.H{"error": err.Error()})` 改为 `internalError(c, err)`，仅暴露错误码与脱敏文案

### 4. 关键流程

#### 4.1 审批路径原子状态转移

1. 接收审批 → 服务层开短事务
2. `SELECT subscriptions ... FOR UPDATE WHERE id=?`
3. 校验当前 `status` 与目标转移合法
4. `UPDATE subscriptions SET ... WHERE id=? AND status=?`，`RowsAffected=0` 视为并发冲突，返回 409
5. commit 后异步触发：MoviePilot 调用 + Bot 通知（用 `safeFireAndForget`）
6. MoviePilot 失败 → 单独事务 `UPDATE subscriptions SET mpError=?, updatedAt=now() WHERE id=?`

#### 4.2 webhook 整剧订阅命中

1. 解析 webhook payload，得到 `tmdbId / season / episode`
2. 命中规则：
   - 指定季 (season=N) 订阅：直接按 `(tmdbId, season=N)` 匹配 APPROVED 订阅 → 收口 INGESTED
   - 整剧 (season=0) 订阅：先查询 TMDB 缓存或 Emby SeriesId 拉取"已播出总集数 X"
     - 若 Emby 已入库集数 ≥ X：收口 INGESTED + `ingestProgress=X/X`
     - 否则：仅更新 `ingestProgress`，状态保持 APPROVED
3. 命中失败时记 INFO 日志（含 `tmdbId / season / episode`），不报错

#### 4.3 mediagap webhook 命中

1. 按 `tmdbId / season / episode` 命中 `media_gaps` 工单
2. 命中状态分支：
   - `MISSING / REQUESTED / DISPATCH_FAILED`：收口为 `INGESTED + ingestedAt`
   - `INGESTED`：noop
   - `IGNORED`：仅记 INFO 日志，不动状态、不动 `ignoreReason / ignoredAt`
3. 批量命中改为 `UPDATE ... WHERE id IN (...)` 一次写

#### 4.4 创建 / 重提交事务

1. 服务层开事务
2. 锁 `(type, tmdbId, season)` 维度（advisory lock 或表级锁）
3. `hasActiveSubscription` → 已有则返回原订阅
4. resubmit：校验 `retryFromId` 是用户自己 `REJECTED` 订阅
5. `INSERT subscriptions ... ON CONFLICT (type,tmdbId,season) WHERE status IN (PENDING,APPROVED,INGESTED) DO NOTHING`
6. 冲突时回查活跃订阅，HTTP 200 返回幂等结果
7. commit 后火忘通知 Bot

#### 4.5 缺集异步扫描跨副本互斥

1. handler 触发扫描 → service 在新 goroutine 中
2. 抢 advisory lock：`pg_try_advisory_lock(hashtext('media_gap_scan'))`
3. 成功：写 `media_gap_scans (status='running', nodeId)` → 执行扫描 → 完成后写 `success/failed` + 释放 lock
4. 失败：返回 409，告知调用方"另一个节点正在扫描"

#### 4.6 cleanupInactiveSeasonGaps 区分人工 / 系统忽略

1. 系统识别"季未激活" → `UPDATE ... SET status='IGNORED', ignoreReason='season not activated', ignoreReasonCode='season_not_activated'`
2. 人工 IGNORE → `ignoreReasonCode='manual'`
3. 前端按 `ignoreReasonCode` 渲染不同标签

### 5. 失败路径与边界条件

- **审批 + webhook 同时收口**：advisory lock + `WHERE status=?` 保证只有一个写入成功
- **整剧订阅在 TMDB 数据缺失时**：无法判定"已播出总集数 X" → 保留 APPROVED + `ingestProgress=null`，由后续 webhook 或 cron 校验补齐
- **MoviePilot 长期不可用**：管理员可手动触发 `redispatch`；建议补 cron 周期重试
- **DISPATCH_FAILED 后 Emby 自然入库**：webhook 仍按命中 → `INGESTED`
- **resubmit 用户 ID 不匹配**：拒绝并返回业务错误
- **advisory lock 持有者 crash**：PostgreSQL 自动释放，下一次扫描可正常获取
- **`media_gap_scans` 表膨胀**：cron 每周清理 7 天前 `success/failed` 记录
- **handler 错误透传**：替换 `err.Error()` 为业务错误码 + 调试日志（带 requestId）

## 影响范围

- API：
  - 修改：`subscription/service.go`、`mediagap/service.go`、`mediagap/types.go`、`handlers/subscription.go`、`handlers/media_gap.go`、Emby webhook handler、`integrations/moviepilot/client.go`（仅错误处理路径）
  - 新增：`services/subscription/state_machine.go`（封装原子状态转移）、`services/mediagap/scan_lock.go`
- Web：
  - 订阅列表展示 `mpError`、"重新下发"按钮（前端改放 `console-admin/web-frontend-auth-and-design-baseline-fix.md`）
  - 缺集列表按 `ignoreReasonCode` 显示标签
- Bot：
  - 不变 Bot 端代码；通知通道改用 `safeFireAndForget`（在 bot-telegram 计划中实现）
- 配置 / 部署：
  - 新增 4 份 SQL migration
- 文档：
  - `docs/system-architecture.md` §4.5 字段表新增 `ingestProgress`
  - §5.9 改写订阅状态机说明
  - §5.11 改写缺集 webhook 命中策略与 `DISPATCH_FAILED` 状态
  - 缺集异步扫描跨副本互斥章节

## 验证方式

### 编译 / 测试

- `cd services/api && go build ./...`
- `cd services/api && go test ./internal/services/subscription/... ./internal/services/mediagap/...`
- `cd services/api && go test ./internal/handlers/... -run "TestSubscription|TestMediaGap"`

### 手工验证

#### 审批原子化
- 同时触发 webhook 与管理员审批：观察 `subscriptions.status` 不会从 INGESTED 回退到 APPROVED
- 拒绝订阅后再次触发审批：返回 409 / 业务错误，状态保持 REJECTED

#### 整剧订阅
- 创建整剧订阅 → mock TMDB 总集数 10 → 入库 5 集后 webhook 触发：status 仍 APPROVED，`ingestProgress=5/10`
- 入库第 10 集后：status=INGESTED，`ingestProgress=10/10`

#### IGNORED 不复活
- 管理员忽略一条缺集 → mock Emby webhook 入库该集
- 验证：`media_gaps.status=IGNORED`，`ignoreReason / ignoredAt` 不变

#### MoviePilot 失败可恢复
- mock MoviePilot 返回 5xx → 审批：status=APPROVED + `mpError != null`
- 调 `redispatch` → 恢复 MoviePilot：status=APPROVED + `mpError=null`

#### 重提交事务
- 用户连点两次 resubmit：第二次返回原订阅 ID
- 多副本 API 同时 resubmit：仅一个生效

#### 跨副本扫描互斥
- 启 2 个 API 副本 → 同时触发扫描：第二个返回 409
- 第一个扫描中途 kill：advisory lock 自动释放，下一次扫描正常

#### `DISPATCH_FAILED`
- mock MoviePilot 下载入口失败：gap status=`DISPATCH_FAILED + lastDispatchError`
- 前端"重试"按钮 → 调用 `DispatchGap` → status=`REQUESTED`

#### 系统 IGNORE vs 人工 IGNORE
- 季未激活触发系统 IGNORE：`ignoreReasonCode=season_not_activated`
- 管理员手动 IGNORE：`ignoreReasonCode=manual`

#### handler 错误透传
- 故意触发 SQL 错误：响应体不出现 `pq:`、`column does not exist` 等内部信息

### 修复后验证清单

- [ ] `go build ./...` 与 `go test ./internal/services/subscription/... ./internal/services/mediagap/...` 全绿
- [ ] 4 份 SQL migration 在临时库重灌通过
- [ ] `media_gap_scans` 在多副本测试环境完成至少 3 次互斥扫描
- [ ] 审批 / webhook 并发场景手工验证通过
- [ ] 关键日志含 `subscriptionId / tmdbId / season / episode / userId / requestId`
- [ ] handler 响应不含 SQL / 内部错误细节
- [ ] `MarkSubscriptionIngestedAsAdmin` 触发用户通知

### 二次暴露检查清单

- [ ] sweep 所有 `db.Save(&xxx)` 全字段写入位置（`subscription`、`mediagap`、`tv_calendar`、`device`）
- [ ] sweep 所有 `c.JSON(500, gin.H{"error": err.Error()})`，统一改为 `internalError(c, err)`
- [ ] sweep 所有"火忘 goroutine"，与 `bot-telegram` 计划的 `safeFireAndForget` 收口对齐
- [ ] sweep 所有"事务内调外部 IO"位置（订阅审批 → MoviePilot；mediagap dispatch；与 billing-redemption 的 fulfillPayment / RedeemCode 同类问题对齐）
- [ ] 复核所有 status 转移路径是否都带 `WHERE status=?`，避免并发覆盖
- [ ] 复核 webhook 解析 `tmdbId / seriesId / season / episode` 的健壮性（`extractInt` 失败默默 0 → 改为显式拒绝并 metric）
- [ ] 复核 `pickTargetSeasonNumbers` 实际覆盖范围；老剧补集回归测试

## 落地后文档处理

- 落地后把"订阅状态机原子转移契约"、"整剧订阅入库进度"、"缺集 IGNORED 不复活"、"DISPATCH_FAILED 重试入口"提炼到 `docs/system-architecture.md` §4.5 / §5.9 / §5.11
- 把"缺集异步扫描跨副本互斥（advisory lock + media_gap_scans）"提炼到运行手册
- 本方案在 P0+P1 全部完成、回归测试通过后移入 `docs/archive/plan/media-subscription/`
- P2 / P3 中未顺手收口的项纳入下一轮治理

## 附录：问题清单与本方案条目映射

| review 编号 | 问题 | 本方案条目 |
|---|---|---|
| P0-3 | 整剧订阅 season=0 一集即收口 | §4.2 + 字段 `ingestProgress` |
| P0-4 | `Save(&subscription)` 全字段覆盖 | §3 + §4.1 |
| P1-5 | mediagap webhook 复活 IGNORED | §4.3 |
| P1-6 | MoviePilot 失败无后续兜底 | §3 `redispatch` + §5 |
| P1-7 | 创建 / 重提交 TOCTOU | §4.4 |
| P1-9 | 缺集异步扫描进程内单实例锁 | §4.5 + 表 `media_gap_scans` |
| P2-13 | webhook 串行三链路 | §4.2 / §4.3（拆分批量更新） |
| P2-16 | `pickTargetSeasonNumbers` 漏季 | 目标 §11 |
| P2-17 | `cleanupInactiveSeasonGaps` 系统 vs 人工 | §4.6 + 字段 `ignoreReasonCode` |
| P2-18 | `MarkIngestedByWebhook` N 次 round-trip | §4.3 批量写 |
| P2-19 | dispatch 失败无 `DISPATCH_FAILED` | §2 状态机扩展 |
| P2-20 | handler 透传 SQL 错误 | §3 + §5 |
| P3-22 | active 订阅查询参数顺序 | 二次暴露清单 |
| P3-23 | 单剧 Emby 抖动静默跳过 | 二次暴露清单（独立 metric） |
| P3-24 | webhook `extractInt` 失败默默 0 | 二次暴露清单 |
| P3-25 | `MarkSubscriptionIngestedAsAdmin` 不发通知 | §3 + §4 |

## 批次 2 已落地（2026-04-27）

按"批次 2 收口实施计划"完成：

- ✅ `Approve / Reject` 改原子状态转移（`UPDATE ... WHERE status='PENDING'` + `RowsAffected=0` → `ErrSubscriptionStateConflict` → 409）
- ✅ MoviePilot 调用从同步路径剥离到 commit 后 `async.SafeGo("subscription.dispatchMoviePilot", ...)`，失败仅写 `mpError`，状态保持 APPROVED
- ✅ `subscriptions.ingestProgress` 列 + 整剧 webhook 命中策略拆分：
  - 单季订阅 (season=N)：webhook 命中即 INGEST
  - 整剧订阅 (season=0)：按 TMDB 已播出剧集总量 `X` 与 Emby 实际库存 `Y` 写 `ingestProgress=Y/X`
  - 当 `Y >= X` 时自动收口为 INGESTED；TMDB / Emby 任一侧缺失时 fallback 记最近一集，由管理员显式确认
- ✅ mediagap webhook 命中 IGNORED 仅 INFO 日志，不再清空 `ignoredAt / ignoreReason`
- ✅ `media_gaps.lastDispatchError` 列 + `MediaGapStatusDispatchFailed` 枚举：`DispatchGap` 失败时写入该列并切换状态，错误经 `upstream.SafeUpstreamError` 脱敏
- ✅ `RedispatchSubscription` service + `PUT /api/v1/admin/subscriptions/:id/redispatch` 路由：仅在 APPROVED 且 `mpError != nil` 时允许，状态保持 APPROVED
- ✅ `Create / Resubmit` 包入事务并使用 `pg_advisory_xact_lock(hashtextextended('subscription:type:tmdbId:season', 0))` 序列化；命中已有活跃订阅返回 `AlreadyExists=true` 幂等成功
- ✅ `MarkSubscriptionIngestedAsAdmin` 触发与 webhook 一致的 `notifyIngested` 通知
- ✅ `handlers/media_gap_async.go` 裸 `go m.run` 改为 `async.SafeGo`
- ✅ `handlers/subscription.go` 全部 500 路径改为 `httpx.InternalError`

不在本批：

- subscription `pickTargetSeasonNumbers` / `cleanupInactiveSeasonGaps` ignoreReasonCode 推到下一轮

### 批次 2 review 修复（2026-04-27）

- ✅ **整剧 ingestProgress 完整收口**：`computeWholeShowProgress` 改为用 TMDB 已播出剧集清单计算 `X`，再结合 Emby 当前实际库存与 `IGNORED` 集排除项计算 `Y`，写 `Y/X` 进度；`Y >= X` 时自动收口为 INGESTED 并触发用户通知。TMDB / Emby 任一侧缺失时 fallback 记最近一集，等管理员显式确认。
- ✅ **redispatch + DISPATCH_FAILED 前端闭环**：
  - `services/web/src/api/admin.ts` 补 `redispatchSubscription`
  - `services/web/src/types/api.ts` 补 `DISPATCH_FAILED` 状态枚举、`lastDispatchError` 字段、`ingestProgress` 字段、`dispatchFailedCount` 摘要
  - `SubscriptionsView.vue` APPROVED + `mpError != null` 时显示"重试 MoviePilot"按钮；整剧 APPROVED 显示 `ingestProgress` 进度
  - `MediaGapsView.vue` 状态筛选 / 状态色（`statusMeta`、`episode-chip-dispatch-failed` 红色）/ 排序 / 统计卡 / 分组摘要均覆盖 `DISPATCH_FAILED`；选中工单为 DISPATCH_FAILED 时展示 `lastDispatchError`
- ✅ **缺集扫描终态用独立 ctx**：`mediaGapScanManager.run` 在调 `FinishAndReleaseHolder` 前新建 `context.WithTimeout(context.Background(), 30s)`，避免扫描业务 ctx 因超时 / cancel 失效后写不进 `media_gap_scans` 终态、留下假 running 行误导排障。新增 sentinel 比对的回归测试。
