# 追剧日历同步与 TMDB 密钥保护方案

> 状态：主干完成，保留尾项（P0 + P1 已落地）
> 负责人：Ember
> 更新时间：2026-04-29

## 落地进度

批次 1（commit `564ea20`）+ 后续 review 修复已完成本方案的 P0 / P1 主干项：

- ✅ 新增 `internal/common/upstream.SafeUpstreamError` / `SafeUpstreamHTTPError`，剥离 `*url.Error` 中含 `api_key` 的请求 URL 与上游响应体；同时新增 `internal/common/httpx.InternalError` 收口 handler 内部错误
- ✅ TMDB / MoviePilot 调用链路（`tvcalendar/service.go` `fetchTMDBJSON`、`mediagap/service.go` `fetchTMDBJSON`、`integrations/moviepilot/client.go` `doRequest` / `TestConnection` / `searchTitle` / `DispatchGapCandidate` / `CreateSubscription`）全部替换为脱敏 helper
- ✅ `tv_calendar.go` / `tmdb.go` / `media_gap.go` handler 中所有裸透 `err.Error()` 的 500 响应改走 `httpx.InternalError`；客户端只看到 `上游服务暂不可用` 统一文案，完整 err 落服务端日志
- ✅ 配置中心媒体测试链路（`config.go` `testEmbyConnection` / `testMoviePilotConnection`）也统一走 `SafeUpstreamError` / `SafeUpstreamHTTPError`；Emby 测试改用 `X-Emby-Token` 头携带密钥，避免 `*url.Error` 把含 `api_key` 的 URL 写进错误文本
- ✅ `MarkEpisodeReadyByWebhook` 缺主剧 `tmdbId` 时不再直接按 `seriesId` 宽匹配改库；必须先解析唯一追剧源 `tmdbId`
- ✅ 当前周 `ready` 纠偏已通过 debouncer 批量回写 `tv_calendar_items`，不再只改返回值
- ✅ `tmdb_cache` GC、`lastFullSyncAt` / `lastCorrectionAt` sync marker、cron 字符串布尔解析容错已落地

剩余项（三层缓存收口 / `pickTargetSeasonNumbers` / `resolveSeriesTMDBIDBySeriesID` 缓存 / Stripe / SMTP 错误脱敏 sweep）按 P2/P3 待后续批次。

## 归档判断

- 当前不适合归档。
- 原因：`pickTargetSeasonNumbers` 与缓存治理仍在本方案边界内，不能只因为 P0/P1 完成就提前退场。

## 背景

2026-04-25 系统性 review 在追剧日历 / TMDB 集成 / 缺集 TMDB 调用链路暴露多类硬伤，整体品味评分 🔴：

- TMDB 错误 wrap 把含 `api_key` 的完整 URL 一路写进容器日志和 API 响应：`tvcalendar/service.go:1218` 与 `mediagap/service.go:1015`。任何能触发 TMDB 调用的渠道都能拿到密钥。
- `MarkEpisodeReadyByWebhook` 在 webhook 缺 tmdbId 时降级用 `seriesId + season + episode` 走 `Updates`，没有 tmdbId 维度限制，可能跨剧污染状态。
- `tmdb_cache` 表只写不清（`expiresAt` 字段建了索引但没人调度清理）；fetch 路径只在查询时跳过过期，无 GC 任务。
- `correctCurrentWeekReadyItems` 在 GET 接口里同步 `Updates` 写库 + 顺带刷新 `lastEpisodeIngestedAt`，是只读路径写放大热点。
- `loadReadyEpisodesBySeries` 为每个 series 触发完整 Emby 分页扫描，单次读请求可能放大成几十次 Emby 调用。
- `loadSourcesForSync`"无 activity marker 时回退全量源"叠加启动补偿同步，等价于服务每次重启都全库 force 同步一次。
- `pickTargetSeasonNumbers` 仅取最近季，不覆盖历史已激活但 TMDB 标记 ended 的季；老剧补集时会漏季。
- `resolveSeriesTMDBIDBySeriesID` 在每次剧集 webhook 都触发 2 次 Emby 调用查找主 TMDB ID，结果未缓存。
- `cron.go` 把 `tvCalendarStartupSyncEnabled == "true"` 与字符串字面量比较，`True` / `TRUE` / 空格变体直接走 false 分支。
- 同步链路单剧 Emby 抖动只 log 跳过、`continue`，没有 metric / 告警可观察。
- TMDB 调用错误信息原样透传到 handler 的 `c.JSON(... err.Error() ...)`，存在内部错误外泄。

如果不收口，会出现"TMDB 密钥被运维日志/接口响应批量泄漏"、"追剧日历状态被跨剧污染"、"TMDB cache 表无界增长"、"GET 周历接口高并发时 Emby 被打爆"等真实可触发的安全 / 性能事故。

> 注：订阅与缺集状态机相关问题在 `media-subscription/subscription-state-machine-hardening.md` 中处理；本计划聚焦追剧日历 / TMDB 集成 / 跨剧污染。

## 目标

本方案要实现：

1. 把 TMDB 与 MoviePilot 调用错误的 wrap 收口为"仅 HTTP 状态 + cache key + 业务错误码"，禁止把含 query string 的 URL 透出日志或响应
2. `MarkEpisodeReadyByWebhook` 必须按 `(tmdbId, season, episode)` 四元组定位；缺 tmdbId 时主动通过 `seriesId` 解析得到唯一 tmdbId 才允许 Updates，否则拒绝并 metric
3. `tmdb_cache` 引入定期 GC（每天清 7 天前过期记录）+ 监控表大小
4. 当前周读时纠偏改为：纠正只在内存返回结构里完成；持久化纠偏改为后台合并节流（30s 内不重复）
5. `loadReadyEpisodesBySeries` 引入按 `MinDateLastSaved` 或日期窗口的过滤，并设候选剧上限 N
6. `loadSourcesForSync` 增加 `lastFullSyncAt` 字段，避免每次重启都全库 force 同步
7. `pickTargetSeasonNumbers` 至少覆盖最近 2 季 + Next 季；force=true 时回退全季
8. `resolveSeriesTMDBIDBySeriesID` 加 LRU 缓存（按 seriesId, 5 min TTL）
9. cron 字符串布尔解析改用 `strconv.ParseBool`，统一容错
10. 同步链路单剧失败落 metric counter + 关键日志补 `tmdbId / seriesId`
11. handler 错误响应统一过 `internalError(c, err)`，禁止透传内部 SQL / URL

## 非目标

本次明确不做：

- 不重写追剧日历的"全库发现 + 周历同步 + Webhook 点亮"主链路结构
- 不调整 TMDB 三层缓存（内存 + DB + TMDB）的整体策略
- 不改前端 TVCalendar 视图的强业务实现（独立计划承接）
- 不引入消息队列承接同步任务
- 不调整 Emby webhook 接入入口
- 不动 `tv_calendar_subscriptions` 用户关注关系
- 不动订阅状态机（在订阅状态机计划处理）

## 当前事实

以当前代码和现行文档为准，现状如下：

- 相关文档：`docs/system-architecture.md` §4.13 / §5.18
- 相关服务：
  - `services/api/internal/services/tvcalendar/service.go`
  - `services/api/internal/services/mediagap/service.go`（仅 TMDB 调用错误处理部分）
- 相关 handler：
  - `services/api/internal/handlers/tv_calendar.go`
  - `services/api/internal/handlers/tmdb.go`
- cron 入口：
  - `services/api/cmd/server/main.go`（启动补偿同步分支）
- 模型：
  - `services/api/internal/models/tv_calendar.go`
- 涉及表：
  - `tv_calendar_sources`
  - `tv_calendar_items`
  - `tv_calendar_subscriptions`
  - `tmdb_cache`
- 当前行为：
  - TMDB 调用错误用 `fmt.Errorf("请求 TMDB 失败: %w", err)` wrap，url.Error 含 api_key 完整 URL
  - `MarkEpisodeReadyByWebhook` 缺 tmdbId 时直接走 `seriesId+season+episode` 的 Updates
  - `tmdb_cache` 仅查询时跳过过期，无 GC
  - `correctCurrentWeekReadyItems` 在读路径同步 Updates + 推进 `lastEpisodeIngestedAt`
  - `loadSourcesForSync` 无 marker 时全量回退
  - `tvCalendarStartupSyncEnabled` 用 `== "true"` 字符串比较
- 现有限制：
  - 线上 `AUTO_MIGRATE=false`
  - cron 默认 `TV_CALENDAR_SYNC_SCHEDULE=0 */12 * * *`
  - 启动补偿同步默认 enabled

## 方案设计

### 1. 用户可见行为

- 追剧日历前端展示行为不变（`ready / today / upcoming / missing` 状态判定不变）
- TMDB / MoviePilot 报错时，前端看到统一的"上游媒体服务暂时不可用"，不再泄漏密钥或内部 URL
- Webhook 入库主链路点亮速度不变，但跨剧污染修复后历史脏状态需要单次重跑同步
- 多副本 / 高并发场景下，GET 周历接口不再因为读时纠偏导致 Emby 被打爆
- 管理员可在设置中心看到"上次全量同步时间 / 上次自动同步时间"，确认是否需要 force
- 老剧补集场景：当 TMDB 仍把已结束剧标 ended，本地仍能通过手动同步覆盖最近 2 季 + Next

### 2. 数据与模型

#### `tv_calendar_sources` 表

新增字段：

| 字段 | 类型 | 列名 | 说明 |
|---|---|---|---|
| LastFullSyncAt | *time.Time | `lastFullSyncAt` | 最近一次 force / 启动补偿全量同步时间 |
| LastCorrectionAt | *time.Time | `lastCorrectionAt` | 最近一次 GET 接口写入纠偏时间，用于 30s 节流 |

#### 新增表 `series_tmdb_lookup_cache`（可选；若 LRU 内存缓存足够则省略表）

| 字段 | 类型 | 说明 |
|---|---|---|
| seriesId | string(50) | Emby SeriesId，主键 |
| tmdbId | string(50) | 解析得到的主 TMDB ID |
| resolvedAt | time.Time | 解析时间 |

> 默认实现：进程内 LRU + 5 分钟 TTL，无需表；多副本之间各自缓存，可接受短暂不一致。

#### TMDB 缓存清理 cron

新增 cron 任务：每日 04:00（默认 `0 4 * * *`，可通过 `TMDB_CACHE_GC_SCHEDULE` 调整）：

- `DELETE FROM tmdb_cache WHERE "expiresAt" < now() - interval '7 days'`
- 输出 metric：删除条数

#### Migration 命名

- `YYYYMMDD_NN_tv_calendar_sources_sync_markers.sql`

#### 配置项新增 / 修改

- `TMDB_CACHE_GC_SCHEDULE` 默认 `0 4 * * *`，默认开启
- `TV_CALENDAR_CORRECTION_DEBOUNCE_SECONDS` 默认 `30`
- `TV_CALENDAR_CORRECTION_MAX_SERIES_PER_REQUEST` 默认 `10`
- `cron` 内的字符串布尔配置统一改 `strconv.ParseBool`

### 3. 接口与边界

- TMDB / MoviePilot 调用统一过 `safeUpstreamError(err)`：
  - 如果是 `*url.Error`：返回 `fmt.Errorf("upstream tmdb failed: %s", urlErr.Op)`
  - 如果是 HTTP 4xx/5xx：返回 `fmt.Errorf("upstream tmdb http %d", statusCode)`
  - 不允许 wrap 原始 URL
- handler `c.JSON(StatusInternalServerError, gin.H{"error": err.Error()})` → `internalError(c, err)`：业务错误码 + 调试日志（带 requestId）
- `GET /api/v1/tv-calendar/global` / `GET /api/v1/tv-calendar/following`
  - 响应字段不变
  - 内部行为：纠偏只改返回结构内的 status；持久化纠偏改异步合并写
- `POST /api/v1/admin/tv-calendar/sync`
  - 请求体不变；`force=true` 时跳过节流并写 `lastFullSyncAt`
- Emby webhook
  - `MarkEpisodeReadyByWebhook(payload)`：缺 tmdbId 时主动通过 `seriesId` 解析；解析得到 0 或 ≥2 条匹配则拒绝并 metric

### 4. 关键流程

#### 4.1 TMDB / MoviePilot 错误收敛

1. 所有 `httpClient.Do(req)` 失败路径必须先经 `safeUpstreamError`
2. handler 层接到 `safeUpstreamError` 包装的错误后，仅返回业务码（如 `upstream_tmdb_unavailable`）+ 通用文案
3. 关键日志记录 `cacheKey / requestId / statusCode`，禁止记 URL 全文

#### 4.2 webhook 命中精度

1. webhook 到达 → 解析 payload，得 `tmdbId / seriesId / season / episode`
2. 若 `tmdbId` 缺失：
   - 调 `resolveSeriesTMDBIDBySeriesID(seriesId)`
   - 命中唯一 tmdbId：填回 payload 继续
   - 命中 0 / ≥2：返回 422 + metric `tv_calendar_webhook_resolve_fail`，不动 DB
3. `UPDATE tv_calendar_items SET status='ready' WHERE "tmdbId"=? AND season=? AND episode=?`
4. 同步刷新 `tv_calendar_sources.lastEpisodeIngestedAt`

#### 4.3 读时纠偏改异步合并

1. GET 周历接口在内存层完成 `today/upcoming/missing` 重新判定
2. 命中"需持久化纠偏"的剧集集合 → push 到 `correctionDebouncer` channel
3. 后台 worker 按 30s 节流批量 `UPDATE tv_calendar_items` + `UPDATE tv_calendar_sources.lastCorrectionAt`
4. 同一 seriesId 在 30s 内只触发一次写

#### 4.4 同步链路收口

1. `loadSourcesForSync` 检查 `lastFullSyncAt`：
   - 距今 < 24h：仅同步活跃剧（`lastEpisodeIngestedAt` 在最近 30 天内）
   - ≥ 24h 或 force：全量同步
2. 启动补偿同步：默认仅触发活跃剧同步，不再退化为全库 force
3. 单剧失败：metric counter + 日志带 `tmdbId / seriesId`，继续下一剧
4. `pickTargetSeasonNumbers`：
   - 默认覆盖最近 2 季 + Next 季 + 当前周相关季
   - force=true：全季

#### 4.5 cron 布尔解析统一

1. 所有 `os.Getenv` / `ConfigService` 读取布尔的入口统一用 `strconv.ParseBool`
2. 解析失败 fallback 到默认值并 WARN 日志

#### 4.6 TMDB cache GC

1. cron 触发 → service 执行 `DELETE FROM tmdb_cache WHERE "expiresAt" < now() - interval '7 days'`
2. 输出 metric：本次删除条数 + 当前表行数

### 5. 失败路径与边界条件

- **TMDB 全网不可达**：错误统一返回 `upstream_tmdb_unavailable`；同步任务跳过该剧、metric +1，不影响其他剧
- **`resolveSeriesTMDBIDBySeriesID` 失败**：webhook 返回 422，metric +1；管理员可通过 force sync 兜底
- **多副本 LRU 缓存不一致**：可接受（5 分钟内一致即可）
- **30s 节流期间 Emby 又入库一集**：第一次纠偏写后再次写覆盖，状态最终一致
- **TMDB cache GC 失败**：metric +1，记 ERROR；不阻断业务
- **启动补偿同步关闭**：仅依赖 cron 周期同步，文档明确说明影响
- **handler 错误透传**：禁止 `err.Error()` 直接 `c.JSON`，统一过 `internalError(c, err)`

## 影响范围

- API：
  - 修改：`tvcalendar/service.go`、`mediagap/service.go`（错误 wrap 路径）、`handlers/tv_calendar.go`、`handlers/tmdb.go`、`cmd/server/main.go`、`cmd/server/cron.go`
  - 新增：`services/tvcalendar/correction_debouncer.go`、`pkg/upstream/error.go`（统一 `safeUpstreamError`）
- Web：
  - 设置中心需要展示新增配置项（`TMDB_CACHE_GC_SCHEDULE` 等），由 `console-admin/web-frontend-auth-and-design-baseline-fix.md` 实施
- Bot：
  - 不变 Bot 端代码
- 配置 / 部署：
  - 新增 1 份 SQL migration
  - 新增 cron 调度
  - 新增 3 个 ConfigDefinition
- 文档：
  - `docs/system-architecture.md` §5.18 改写"读时纠偏 + 同步链路"
  - 新增"TMDB cache GC"章节
  - 新增"webhook 命中精度（按 tmdbId 四元组）"章节
  - `docs/runbooks/deployment-environment.md` 新增 cron 列表

## 验证方式

### 编译 / 测试

- `cd services/api && go build ./...`
- `cd services/api && go test ./internal/services/tvcalendar/...`
- `cd services/api && go test ./internal/handlers/... -run "TestTVCalendar|TestTMDB"`

### 手工验证

#### TMDB 密钥不外泄
- mock TMDB 返回 5xx → 触发同步：API 响应、容器日志均不出现 `api_key=`
- mock TMDB 网络中断 → grep 容器日志 `api_key`：0 命中

#### webhook 命中精度
- 准备两部不同剧但相同 seriesId 的脏数据 → mock webhook 缺 tmdbId：返回 422，DB 不变
- 同 webhook 带 tmdbId：仅命中目标剧

#### 读时纠偏
- 高并发请求 GET `/api/v1/tv-calendar/global` 100 次/s → 观察 `tv_calendar_items` 写入 QPS：30s 节流后单剧写入 ≤ 1 次 / 30s
- 30s 内 Emby 真实入库 → 第二次纠偏覆盖，状态最终一致

#### 同步链路
- 启动 API：观察日志，`lastFullSyncAt` 24h 内不再触发全量同步
- force=true 触发：写入 `lastFullSyncAt`
- 单剧 mock TMDB 失败：metric `tv_calendar_sync_fail` +1，其他剧继续

#### TMDB cache GC
- 写入 1000 条过期记录 → 触发 cron：表清空，metric 输出 1000

#### 老剧补集
- mock TMDB 标记 ended，强制 force sync：覆盖最近 2 季 + Next + 历史已激活季

#### cron 布尔
- `TV_CALENDAR_STARTUP_SYNC_ENABLED=True / TRUE / 1` 全部正确解析为 true
- `=invalid` → fallback default + WARN 日志

#### handler 错误
- mock SQL 错误 → 响应不出现 `pq:` / SQL 错误细节

### 修复后验证清单

- [ ] `go build ./...` 与 `go test ./internal/services/tvcalendar/...` 全绿
- [ ] 1 份 SQL migration 在临时库重灌通过
- [ ] grep 容器日志确认 `api_key=` 0 命中
- [ ] webhook 命中精度回归（含跨剧污染脏数据场景）
- [ ] TMDB cache GC cron 在测试环境执行 3 次正常
- [ ] 关键日志含 `tmdbId / seriesId / requestId`，不含 URL 全文 / api_key
- [ ] 多副本部署验证读时纠偏不重复写

### 二次暴露检查清单

- [ ] sweep 所有外部 HTTP 调用错误 wrap 位置（Emby / MoviePilot / TMDB / Stripe / SMTP / Bot），统一过 `safeUpstreamError`
- [ ] sweep 所有 webhook 处理路径，确认输入字段缺失时不退化为单维度 update
- [ ] sweep 所有 GET 接口在 service 层是否有写库副作用（除业务必要之外应一律拒绝）
- [ ] sweep 所有 cron 启动逻辑，确认布尔解析统一
- [ ] sweep 所有"无 marker 回退全量"位置（除追剧日历外，是否在排行榜 / 缺集扫描中也存在）
- [ ] sweep 所有缓存表 GC：`tmdb_cache`、`media_quality_caches`、`playback_rankings`（旧 batch）、`email_verifications`、`telegram_bind_codes`、`device_actions`，确认是否都有调度

## 落地后文档处理

- 落地后把"TMDB / MoviePilot 错误统一收敛"、"webhook 命中精度（四元组）"、"读时纠偏 30s 节流"、"TMDB cache GC"提炼到 `docs/system-architecture.md` §5.18
- 新增 ConfigDefinition 提炼到设置中心说明
- 本方案在 P0+P1 全部完成、回归测试通过后移入 `docs/archive/plan/media-subscription/`
- P2 / P3 中未顺手收口的项纳入下一轮治理

## 附录：问题清单与本方案条目映射

| review 编号 | 问题 | 本方案条目 |
|---|---|---|
| P0-1 (订阅) | TMDB 错误 wrap 泄漏 api_key | §4.1 + 二次暴露 |
| P0-2 (订阅) | `MarkEpisodeReadyByWebhook` seriesId 跨剧污染 | §4.2 |
| P1-8 (订阅) | TMDB cache 无 GC | §4.6 + cron |
| P1-10 (订阅) | 读时纠偏写放大 | §4.3 + `correctionDebouncer` |
| P1-11 (订阅) | `loadReadyEpisodesBySeries` 放大 Emby 调用 | §4.3 + 上限 |
| P2-13 | webhook 串行三链路 | §4.2 |
| P2-14 | `resolveSeriesTMDBIDBySeriesID` 未缓存 | §2 LRU + §4.2 |
| P2-15 | `loadSourcesForSync` 无 marker 全量回退 | §2 + §4.4 |
| P2-16 | `pickTargetSeasonNumbers` 漏季 | §4.4 |
| P2-20 | handler 透传 SQL 错误 | §3 + §5 |
| P3-21 | cron 字符串布尔比较 | §4.5 |
| P3-23 | 单剧 Emby 抖动静默跳过 | §4.4 + metric |
| P3-24 | webhook `extractInt` 失败默默 0 | 二次暴露清单 |
