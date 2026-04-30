# 播放观察与设备链路加固方案

> 状态：继续进行中（主干已落地，剩余项已缩窄）
> 负责人：Ember
> 更新时间：2026-04-30

## 落地进度

批次 3-A + 后续 review 修复已完成本方案绝大多数 P0 / P1 主干项：

- ✅ `playback_rankings.batch_id` 扩位 + 幂等唯一索引、自然日 / 自然周取数已落地
- ✅ `media_quality_caches` 的 `schemaVersion` / `inflightUntil`、force inflight 拒绝和残留清理 cron 已落地
- ✅ 5min stats / 最近入库缓存 single-flight、`LATEST_CACHE_PER_USER` 分桶和 EmbyService 共享单例已落地
- ✅ 黑名单批量注销结构化返回、`normalizeClientName` 强化、`device_actions.operatorId` 审计已落地
- ✅ 92 天范围限制、overview 行数硬上限、keyword escaping 与 `pauseDuration` 解析已落地
- ✅ `libraryId=all` 失败库继续返回其余结果并附 `failedLibraries` 元数据；前端失败库提示已同步落地
- ✅ 排行榜 episode 回查已补批次超时 / 退避 / partial failure 容错
- ✅ `dedupeLatestItems` 已不再使用 `Type+Name+Year` 的粗糙兜底 key，当前实现已改为更精确特征组合
- ✅ 与本方案相关的关键 handler 已统一改走 `httpx.InternalError`，不再裸透 `err.Error()`

当前剩余项已缩窄为少量真实尾项：

- `generateRankingBatchID` 已不再截断到 25 字符，但实现仍是随机 base32 26 位 ID，不是计划正文写的标准 ULID
- `playback/history.go` 的 wildcard / 本地分页 fallback 仍保留，`loadPlaybackRowsByLocalPagination` 仍可能在极端情况下全量拉回内存后本地排序分页
- `parsePlaybackRows` 仍保留 `fallbackIndexes`，尚未收口到“无稳定 columns 即拒绝”的更硬边界
- `_ = db.DB.Create/Save` 这类静默吞错 sweep 还没系统性做完

## 归档判断

- 当前明确不适合归档。
- 原因：虽然主干已经落地，但 playback fallback 与 batchId 策略仍是实打实的实现尾项，不只是文档收尾。

## 背景

2026-04-25 系统性 review 在播放排行 / 历史 / 画像 / 设备 / 媒体质量盘点链路集中发现多类硬伤，整体品味评分 🔴：

- 媒体质量缓存 `shouldRefreshLegacyMediaQualityCache` 命中即重扫：缓存形同虚设，多用户并发触发整库扫描，"all libraryId" 路径会顺序拉所有库的全部条目。
- 排行榜入库无幂等键：cron 重跑 / 手动补跑会写入重复 batch；`GetHistoryRanking` 按 `period_start = ?` 命中后 `First`，多 batch 并存时返回过期快照。
- `LogoutBlacklistedDevices` 部分失败时返回 500 + body 含 count；前端按状态码分流不会读 count，重试会重复打设备。
- 自定义日期范围 92 天上限按时点差比较，可达 92 天 0 秒；overview 路径 `SELECT *` 全量拉回内存内分页，无上限保护。
- 排行榜 episode 链路 `GetItemsByIDs` 顺序请求无超时无重试间隔，单批失败整次任务挂掉。
- 媒体质量 `libraryId=all` 顺序拉所有库，单库失败整体失败，已拉的库丢弃。
- 5min stats 缓存"获取-检查-写入"竞争无 single-flight，冷启动 / TTL 过期瞬间多用户并发打 Emby。
- 黑名单 `unblacklist` / `logout` 操作日志元数据缺失（deviceId / userId / actor 全空），无法回溯。
- `normalizeClientName` 仅 `ToLower + Trim`，无法覆盖版本尾缀 / 全角空格 / NFC，黑名单实际命中率远低于预期。
- 排行榜 weekly `period_end` 用 `now` 而非自然周末，跨周末 cron 漂移导致历史接口漏命中。
- `loadPlaybackRowsByLocalPagination` 大数据量下全量拉回内存排序分页，无上限保护。
- `playbackKeywordPattern` 不允许 `& + ( ) :` 等常见片名字符；`escapeLikePattern` 是死代码，依赖正则白名单挡污染。
- `latestCacheEntry` 跨用户共享，第一个登录用户决定全局可见性，启用 Emby 库权限差异即越权（注释已自承）。
- 媒体质量 force 入口无 inflight 控制，连点导致并发整库扫描互相覆盖。
- `dedupeLatestItems` 兜底 key 用 `Type+Name+Year`，相同年份同名作品（不同语言版本）会被错误去重。
- `recordDeviceAction` 失败被 `_ =` 吞掉，关键审计静默丢失。
- `generateRankingBatchID` 截断 `[:25]` 只剩 6 位随机熵，cron 同秒重跑会撞 batchId。
- `parsePlaybackRows` 在 wildcard fallback 路径下 `pauseDuration` 永远拿不到。
- 多 service 各持一份 EmbyService，HTTP 客户端连接池碎片化。
- handler 直接 `c.JSON(500, gin.H{"error": err.Error()})`，可能透出 SQL 错误。

如果不收口，会出现"媒体质量每次请求都全库扫"、"排行榜历史拿到旧 batch"、"重复扣黑名单注销 Emby 设备"、"普通用户看到管理员可见的最近入库"、"高并发打爆 Emby"等真实可触发的性能 / 安全 / 数据正确性事故。

> 注：前端展示行为（排行榜页面、用户画像页面、设备页面 UI 等）由 `docs/archive/plan/console-admin/web-frontend-auth-and-design-baseline-fix.md` 处理。

## 目标

本方案要实现：

1. 排行榜入库引入幂等键（按 `period + periodStart + periodEnd`），cron 重跑 / 补跑不再产生重复 batch；`GetLatestRanking` / `GetHistoryRanking` 按 `batchId` 取数避免拿到旧快照
2. 删除 `shouldRefreshLegacyMediaQualityCache` 启发式判断，改为 cache 行 `schemaVersion` 字段；缓存命中时不再触发重扫
3. 媒体质量 force 入口加 single-flight：同 cacheKey 正在扫描时直接返回 409 + 当前进度；多用户连点不并发触发
4. 媒体质量 `libraryId=all` 单库失败转告警继续，最终响应附 `failedLibraries` 元数据
5. 5min stats 缓存与最近入库缓存引入 single-flight，杜绝缓存击穿
6. `latestCacheEntry` 默认按 `embyUserID + itemType` 分桶；保留全局共享开关 `LATEST_CACHE_PER_USER`，默认 true
7. `LogoutBlacklistedDevices` 部分失败返回 207/200 + 结构化 body（含 `successDeviceIds / failedDeviceIds`），前端不再误重试
8. 自定义日期范围 92 天上限按"自然日"判定；overview 路径强制分页 SQL + 硬上限
9. 排行榜 episode 详情回查 `GetItemsByIDs` 加 ctx 超时 + 失败 batch 跳过累积错误
10. 排行榜 `dayRange / weekRange` 按"自然日 / 自然周末"边界取数，不用 `now`
11. `generateRankingBatchID` 改 ULID 并扩字段到 `varchar(32)`
12. 黑名单 `normalizeClientName` 强化（strip 版本号 / 全角归一 / NFC），归一规则写入文档
13. `recordDeviceAction` 增加 `OperatorID` 字段；service 层签名加 actor 参数；handler 从中间件注入；写库失败必须日志
14. `playbackKeywordPattern` 扩展白名单字符；`escapeLikePattern` 与 keyword 校验保持单一来源
15. `dedupeLatestItems` 兜底 key 加 `ParentID` 或不去重
16. `loadPlaybackRowsByLocalPagination` 加硬上限 + 拒绝信号
17. `parsePlaybackRows` 废弃 `fallbackIndexes`，wildcard 路径无 columns 时直接拒绝
18. EmbyService 单例化，复用 HTTP 连接池
19. handler 错误响应统一过 `internalError(c, err)`

## 非目标

本次明确不做：

- 不重写 Playback Reporting 插件接入方式
- 不引入新的数据库（如 ClickHouse / OLAP）
- 不重写设备 UI / 排行榜 UI（前端改放 `docs/archive/plan/console-admin/web-frontend-auth-and-design-baseline-fix.md`）
- 不调整媒体质量扫描算法本身（仍按分辨率 / 编码 / HDR 分布）
- 不改 Emby 集成认证方式

## 当前事实

以当前代码和现行文档为准，现状如下：

- 相关文档：`docs/system-architecture.md` §4.9 / §4.10 / §4.11 / §5.10 / §5.15 / §5.19 / §5.20 / §5.21 / §5.22
- 相关服务：
  - `services/api/internal/services/playback/ranking.go`
  - `services/api/internal/services/playback/history.go`
  - `services/api/internal/services/playback/profile.go`
  - `services/api/internal/services/playback/profile_list.go`
  - `services/api/internal/services/device/service.go`
  - `services/api/internal/services/media/service.go`
  - `services/api/internal/services/media/quality.go`
- 集成层：
  - `services/api/internal/integrations/emby/library.go`
  - `services/api/internal/integrations/emby/emby.go`
- handler：
  - `services/api/internal/handlers/ranking.go`、`playback_history.go`、`user_playback_profile.go`、`device.go`、`session.go`、`media.go`、`media_quality.go`
- 模型：
  - `services/api/internal/models/playback_ranking.go`
  - `services/api/internal/models/media_quality_cache.go`
  - `services/api/internal/models/client_blacklist.go`
  - `services/api/internal/models/device_action.go`
- 涉及表：`playback_rankings`、`media_quality_caches`、`client_blacklists`、`device_actions`
- 当前行为：
  - `playback_rankings.batch_id` 模型与增量 migration 已扩到 `varchar(32)`；当前 `generateRankingBatchID` 生成的是随机 base32 26 位 ID，不是 ULID
  - `media_quality_caches` 已使用 `schemaVersion` / `inflightUntil` 管理缓存与 force inflight，缓存命中不再因旧启发式逻辑重扫
  - 5min stats / 最近入库已补 single-flight；最近入库默认按 `LATEST_CACHE_PER_USER=true` 按用户分桶缓存
  - `libraryId=all` 路径已在单库失败时返回 `failedLibraries`，不再整批失败
  - `GetItemsByIDs` 已补 batch timeout / retry / partial failure 容错
  - `normalizeClientName` 已补版本尾缀去除、全角空格与 NFC 归一；`recordDeviceAction` 写库失败已记日志
  - Playback history 仍保留 wildcard / 本地分页 fallback，overview 仍通过 `COUNT + SELECT *` 加 `maxPlaybackProfileOverviewRows` 上限保护，而不是彻底改成 SQL 层分页聚合
- 现有限制：
  - 线上 `AUTO_MIGRATE=false`
  - PlaybackActivity 通过 Emby 插件 SQL 查询，无参数化能力，仅靠白名单 + escape

## 方案设计

### 1. 用户可见行为

- 管理员页面：媒体质量"刷新"按钮在扫描中禁用；多人同时点不会并发触发整库扫描
- 排行榜历史页面：按日期查询不再返回旧快照
- 黑名单批量注销：失败设备显式标红，部分成功的设备不被重复注销
- 自定义日期范围：超过 92 自然日的请求被拒绝，提示"请缩小时间窗口"
- 普通用户首页"最近入库"：每个用户按各自 Emby 库权限展示
- 高并发首页刷新：Emby 不被打爆
- 黑名单生效率：Emby 客户端版本号差异不影响匹配
- 错误响应不出现 SQL 内部细节

### 2. 数据与模型

#### `playback_rankings` 表

修改字段：

| 字段 | 修改 | 说明 |
|---|---|---|
| `batch_id` | `varchar(25)` → `varchar(32)` | 容纳 ULID |

新增索引：

| 索引 | 类型 | 用途 |
|---|---|---|
| `(period, period_start, period_end)` | unique | 幂等键，防重复 batch |

#### `media_quality_caches` 表

新增字段：

| 字段 | 类型 | 列名 | 说明 |
|---|---|---|---|
| SchemaVersion | int | `schemaVersion` | 缓存行 schema 版本，默认 1 |
| InflightUntil | *time.Time | `inflightUntil` | 当前扫描预期完成时间，用于 single-flight 拒绝 |

#### `device_actions` 表

新增字段：

| 字段 | 类型 | 列名 | 说明 |
|---|---|---|---|
| OperatorID | *string(25) | `operatorId` | 操作者 user ID（如 admin），可空 |

#### `client_blacklists` 表

无字段变更，但 `normalizedClientName` 写入路径要重写归一规则。

#### Migration 命名

- `YYYYMMDD_NN_playback_rankings_batch_id_widen.sql`
- `YYYYMMDD_NN_playback_rankings_idempotency_unique.sql`
- `YYYYMMDD_NN_media_quality_caches_schema_version.sql`
- `YYYYMMDD_NN_device_actions_operator_id.sql`

所有 migration 幂等。

#### 配置项新增 / 修改

- `LATEST_CACHE_PER_USER` 默认 `true`
- `MEDIA_QUALITY_FORCE_INFLIGHT_TIMEOUT_SECONDS` 默认 `1800`
- `PLAYBACK_OVERVIEW_ROW_LIMIT` 默认 `5000`
- `PLAYBACK_KEYWORD_ALLOWED_EXTRA_CHARS` 默认 `&+():!/`

### 3. 接口与边界

- `GET /api/v1/admin/media-quality/libraries/:libraryId`
  - 入参不变；`force=true` 时若已有 inflight 返回 409 + `inflightUntil`
  - 响应新增 `failedLibraries`（仅 `libraryId=all` 路径）
- `POST /api/v1/admin/media-quality/libraries/:libraryId/scan`
  - 入参不变；并发触发返回 409
- `POST /api/v1/admin/devices/blacklist/logout-all`
  - 响应改为 `{ "successDeviceIds": [...], "failedDeviceIds": [{deviceId, error}] }`
  - HTTP 状态：全成功 200；部分成功 200（body 显式标）；全失败 502
- `GET /api/v1/admin/playback-history`
  - 入参 `keyword` 白名单扩展 `& + ( ) : ! /`
- `GET /api/v1/admin/playback-profiles`
  - 自定义日期范围超过 92 自然日返回 400
  - overview 内部按 `PLAYBACK_OVERVIEW_ROW_LIMIT` 拒绝过大窗口
- `GET /api/v1/media/latest`
  - 默认按 `embyUserID + itemType` 分桶；`LATEST_CACHE_PER_USER=false` 时回到全局共享
- `POST /api/v1/admin/cron/generate-ranking`
  - 内部行为：`INSERT playback_rankings ... ON CONFLICT (period, period_start, period_end) DO NOTHING`
- handler 错误统一过 `internalError(c, err)`

### 4. 关键流程

#### 4.1 排行榜幂等

1. cron 触发 → `dayRange / weekRange` 按"自然日 / 自然周末"取边界
2. 生成 ULID 作为 `batchId`
3. `INSERT INTO playback_rankings ... ON CONFLICT (period, period_start, period_end) DO NOTHING`
4. 冲突时直接 noop + INFO 日志
5. 成功插入时触发 Bot 通知；冲突不通知

#### 4.2 媒体质量缓存

1. 删除 `shouldRefreshLegacyMediaQualityCache`
2. 缓存读取：按 `cacheKey` + `schemaVersion` 命中
3. force 入口：
   - 在事务内 `SELECT ... FOR UPDATE` 检查 `inflightUntil > now()`
   - 命中 → 返回 409 + `inflightUntil`
   - 否则 → 写 `inflightUntil = now() + timeout` → 异步执行扫描 → 完成后清空 `inflightUntil`
4. `libraryId=all` 路径：聚合多库结果，单库失败仅 metric + 加入 `failedLibraries`，不中断

#### 4.3 5min stats / 最近入库 single-flight

1. 引入 `golang.org/x/sync/singleflight`
2. 缓存过期时所有并发 reader 共用一个底层调用
3. 缓存写入路径加 Lock 内二次时间戳检查

#### 4.4 LogoutBlacklistedDevices 结构化返回

1. service 内逐个尝试注销，累积成功 / 失败列表
2. handler 按 successCount / failedCount 决定 HTTP 状态
3. 前端按 body 渲染 toast / 列表

#### 4.5 黑名单归一

1. `normalizeClientName(raw)`:
   - `strings.ToLower(strings.TrimSpace(raw))`
   - 正则去除尾缀版本号 `\s+v?\d+(\.\d+)*$`
   - 全角空格 → 半角
   - NFC 归一
2. 文档说明命名规则与 Emby 客户端常见样本

#### 4.6 设备审计 actor

1. handler 从 JWT context 取 `userID` → 透传给 service
2. service 写 `device_actions(operatorId=...)`
3. 写库失败：ERROR 日志（含 action / deviceId / operatorId）

#### 4.7 自定义日期窗口校验

1. service 入口校验：`endDate.AddDate(0,0,1).Sub(startDate) > 92*24h` → 拒绝
2. overview 路径：`SELECT ... LIMIT $PLAYBACK_OVERVIEW_ROW_LIMIT`，超过返回 `tooManyRows` 错误，让前端提示缩小时间窗口

#### 4.8 EmbyService 单例

1. 在 `app/wire.go` 或启动入口构造单例 `*emby.Service`
2. 所有需要 EmbyService 的服务通过依赖注入获取，不再自行 `NewEmbyService()`

### 5. 失败路径与边界条件

- **排行榜唯一索引冲突**：noop + INFO；不报错
- **媒体质量 force inflight 期间 service 重启**：`inflightUntil` 残留 → 等待超时后下次 force 可继续；建议 cron 每 30 分钟清理过期 `inflightUntil`
- **5min stats single-flight 期间 Emby 抖动**：所有 reader 共享错误，下一次过期再尝试
- **`LATEST_CACHE_PER_USER=false` 切换风险**：文档明确"仅在所有用户库一致时打开"
- **黑名单归一变更影响历史数据**：上线前跑一次 migration 重写 `normalizedClientName`
- **自定义日期窗口边界**：92 自然日起算（含起止日），不接受 92 天 + 1 秒
- **EmbyService 单例失败重连**：连接池透明处理，业务层不感知
- **handler 错误透传**：禁止 `err.Error()` 直接 `c.JSON`

## 影响范围

- API：
  - 修改：`playback/ranking.go`、`playback/history.go`、`playback/profile.go`、`playback/profile_list.go`、`device/service.go`、`media/service.go`、`media/quality.go`、`integrations/emby/*.go`（单例化）、`handlers/*.go`（错误透传 + actor 注入）
  - 新增：`pkg/cache/singleflight.go`、`services/media/inflight.go`
- Web：
  - 媒体质量页 inflight 状态 + 黑名单批量注销结构化结果（在 `docs/archive/plan/console-admin/web-frontend-auth-and-design-baseline-fix.md` 中实施）
- Bot：
  - 不变 Bot 端代码
- 配置 / 部署：
  - 新增 4 份 SQL migration
  - 新增 4 个 ConfigDefinition
  - cron 新增"清理过期 `inflightUntil`"
- 文档：
  - `docs/system-architecture.md` §5.15 / §5.22 改写
  - 新增"客户端归一规则"章节
  - 新增"媒体质量 force inflight 语义"章节
  - 新增"排行榜幂等键"章节

## 验证方式

### 编译 / 测试

- `cd services/api && go build ./...`
- `cd services/api && go test ./internal/services/playback/... ./internal/services/device/... ./internal/services/media/...`
- `cd services/api && go test ./internal/handlers/... -run "TestRanking|TestPlayback|TestDevice|TestMedia"`

### 手工验证

#### 排行榜幂等
- cron 触发后立即手动重跑：`playback_rankings` 行数不增
- 历史接口按日期查询：返回唯一 batch
- 跨周末 cron 漂移：周日 23:50 跑 / 周一 00:10 跑 → 各自命中正确周

#### 媒体质量缓存
- 多人同时点 force：仅一次扫描，其他返回 409
- 缓存命中：不再触发重扫
- `libraryId=all` 中单库 mock 失败：响应含 `failedLibraries`，其他库结果正常返回

#### 5min stats / 最近入库
- 高并发请求 `/media/stats`：Emby 调用次数 = 1
- TTL 过期瞬间 100 并发：Emby 调用次数 = 1

#### 黑名单批量注销
- 准备 5 条黑名单，mock 2 条 Emby 注销失败：响应 `successDeviceIds(3) / failedDeviceIds(2)`，HTTP 200
- 前端可见失败列表，重试只针对失败项

#### 自定义日期范围
- 92 自然日：通过
- 92 天 + 1 秒：拒绝
- overview 超大时间窗口（mock 几十万行）：返回 `tooManyRows`

#### latestCacheEntry
- `LATEST_CACHE_PER_USER=true`：两个不同库权限的用户看到不同结果
- `=false`：共享缓存（与现状一致）

#### 黑名单归一
- "Emby for iOS 1.2.3" 与 "Emby for iOS" 归一后一致
- 全角空格 / NFC 差异归一

#### 设备审计 actor
- admin A 操作黑名单：`device_actions.operatorId = A.userId`
- admin A 强制注销：同上

#### 排行榜 batchId
- 新装环境 `batchId` 长度 26；若继续保留现实现，则验证随机 base32 ID 在同秒并发下不撞 ID；若改 ULID，则同步校验 ULID 形态

#### keyword 白名单
- 搜索 "Mission: Impossible" / "Spider-Man (2002)" / "Fast & Furious"：通过
- 搜索 `' OR 1=1 --`：拒绝

#### EmbyService 单例
- 启动后 `netstat` 看与 Emby 的 TCP 连接数：稳定复用而非每个 service 各持一份

#### handler 错误透传
- 故意触发 SQL 错误：响应不含 `pq:`

### 修复后验证清单

- [ ] `go build ./...` 与 `go test ./internal/services/playback/...` `./internal/services/device/...` `./internal/services/media/...` 全绿
- [ ] 4 份 SQL migration 在临时库重灌通过
- [ ] cron 触发排行榜重跑：行数不增；通知不重复
- [ ] 媒体质量 force inflight cron 清理跑一轮空表无报错
- [ ] 关键日志含 `userId / requestId / batchId / cacheKey / operatorId`，且不含 SQL 错误细节
- [ ] 文档同步：客户端归一规则、排行榜幂等键、`LATEST_CACHE_PER_USER` 语义
- [ ] 若继续推进计划 5 尾项：补针对 playback wildcard / 本地分页 fallback 的测试与收口方案

### 二次暴露检查清单

- [ ] sweep 所有"读 GET 接口写 DB"位置（排除 `correctCurrentWeekReadyItems` 已在追剧日历计划中处理）
- [ ] sweep 所有"全表 / 全量数据拉回内存排序分页"位置（playback / 设备 / 媒体质量）
- [ ] sweep 所有"force / refresh"入口的并发控制
- [ ] sweep 所有 `_ = db.DB.Create(&xxx).Error` / `_ = db.DB.Save(&xxx).Error` 静默吞错
- [ ] sweep 所有 `c.JSON(500, gin.H{"error": err.Error()})`，统一改 `internalError(c, err)`
- [ ] sweep 所有 keyword / orderBy 字符串拼接，确认白名单 + escape 单一来源
- [ ] 复核所有 `time.Now()` 在跨进程聚合场景是否取了正确时区（CRON_TIMEZONE）
- [ ] 复核 EmbyService / TMDBService 实例化是否都收口到单例

## 落地后文档处理

- 落地后把"排行榜幂等键"、"媒体质量 force inflight"、"`LATEST_CACHE_PER_USER` 语义"、"客户端归一规则"提炼到 `docs/system-architecture.md`
- 黑名单归一规则写入运行手册
- 本方案在 P0+P1 全部完成、回归测试通过后移入 `docs/archive/plan/console-admin/`
- P2 / P3 中未顺手收口的项纳入下一轮治理

## 批次 5 当前收口状态（2026-04-30）

- ✅ `services/device/service.go` `AddClientToBlacklist` 去掉 `Save(&blacklist)`，改为按字段 `Updates(map)`，避免黑名单整行回写
- ✅ 与本方案相关的 handler `500` 裸透已在批次 5 第一阶段统一改走 `httpx.InternalError`
- ✅ 文档盘点后确认：`failedLibraries`、episode 回查 timeout/retry、`LATEST_CACHE_PER_USER`、`dedupeLatestItems` 精确化、设备审计 actor 等已从“待完成”转为“已落地”

仍未完成：

- `generateRankingBatchID` 是否改 ULID 尚未定案
- playback history wildcard / 本地分页 fallback 尚未彻底收口
- `_ = db.DB.Create/Save` 静默吞错 sweep 尚未系统化完成

## 附录：问题清单与本方案条目映射

| review 编号 | 问题 | 本方案条目 |
|---|---|---|
| P0-1 (播放) | 媒体质量缓存命中即重扫 | §4.2 |
| P0-2 (播放) | 排行榜入库无幂等键 | §4.1 + 唯一索引 |
| P0-3 (播放) | 黑名单批量注销返回 500 丢 successCount | §4.4 |
| P1-1 (播放) | 92 天上限假强校验 | §4.7 |
| P1-2 (播放) | episode 详情回查无超时无并发控制 | §4 + ctx 超时 |
| P1-3 (播放) | 媒体质量 all 单库失败即整体失败 | §4.2 |
| P1-4 (播放) | 5min stats 缓存 single-flight | §4.3 |
| P1-5 (播放) | 黑名单审计元数据缺失 | §4.6 + actor |
| P1-6 (播放) | normalizeClientName 覆盖不足 | §4.5 |
| P1-7 (播放) | weekly 历史 period_end 漂移 | §4.1 自然周末 |
| P1-8 (播放) | 本地分页全量拉回内存 | §4.7 + 上限 |
| P2-1 (播放) | dayRange/weekRange 用 now 截止 | §4.1 |
| P2-3 (播放) | escapeLikePattern 死代码 | §4 + 白名单单一来源 |
| P2-4 (播放) | dedupeLatestItems 兜底 key 错误 | §4 + 兜底加 ParentID |
| P2-5 (播放) | latestCacheEntry 跨用户共享 | §4 + LATEST_CACHE_PER_USER |
| P2-6 (播放) | force 扫描无 inflight | §4.2 |
| P2-8 (播放) | recordDeviceAction 静默吞错 | §4.6 |
| P2-9 (播放) | generateRankingBatchID 截断丢熵 | §2 batch_id varchar(32) |
| P2-10 (播放) | keyword 拒绝常见片名字符 | §4 + 配置项 |
| P2-11 (播放) | fallback 路径丢 PauseDuration | 二次暴露清单 |
| P2-12 (播放) | GroupLowQualityDetails 重复 paginate | 二次暴露清单 |
| P3-1~P3-7 (播放) | 各类风格 / 索引 / 日志 | 二次暴露清单 |
