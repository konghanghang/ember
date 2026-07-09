# settings 按 Key 本地缓存方案

> 状态：已完成，已归档
> 负责人：Ember
> 更新时间：2026-07-09

## 落地状态

- 已实现 `settings` 按 key 的进程内 TTL 缓存、negative cache 和单 key 并发合并。
- 已为设置中心更新、排行榜 allowlist、Admin API Key 等写路径接入缓存失效。
- 已补 `services/api/internal/config/settings_cache_test.go` 覆盖重复读取、negative cache、写后失效、并发合并与失效竞态。
- 已完成 `go test ./internal/config`、`go build ./...` 和全仓 `scripts/test/all.sh` 验证。
- 稳定结论已同步到 `docs/system-architecture.md`，本稿现在只保留方案追溯价值。

## 背景

当前 API 进程对 `settings` 表的读取过于频繁，同一个请求链路里会重复读取同一个配置 key，导致数据库出现大量低价值热点查询。

- `ConfigService.GetString()` / `ResolveString()` 在 `settingsMap == nil` 时，会为单个 key 单独调用一次 `loadSettings()`，从数据库读取该 key。
- 排行榜、配置探测、订阅、追剧日历等链路会重复读取 `EMBY_URL`、`EMBY_API_KEY`、`CRON_TIMEZONE` 等同一个 key。
- 当前系统里已经有进程内缓存实践（如媒体统计、排行榜运行期缓存），但 `ConfigService` 还没有对应的 key 级缓存。
- 如果不收口，配置中心会持续把 `settings` 表打成热点，增加请求时延，并放大 GORM SQL 日志噪音。

## 目标

本方案要实现：

1. 为 `settings` 表提供按 `key` 维度的进程内 TTL 缓存，默认 TTL 为 `60s`。
2. 同一个 key 在 TTL 有效期内命中缓存，不再重复打数据库。
3. 缓存支持 negative cache：数据库不存在的 key 也缓存 `60s`，避免反复查空。
4. 所有通过 API 进程写入或删除 `settings` 的路径，在成功后主动失效对应 key，避免 60 秒内继续读到旧值。

## 非目标

本次明确不做：

- 不做整张 `settings` 表快照缓存。
- 不做后台自动刷新、异步 reload 或类似 Caffeine refresh-after-write 机制。
- 不做跨实例缓存同步；多实例之间仍接受最多 `60s` 的最终一致窗口。
- 不引入 Redis、Memcached 或其他外部缓存。
- 不修改 `settings` 表 schema，不新增数据库 migration。

## 当前事实

以当前代码和现行文档为准，现状如下：

- 相关文档：
  - [docs/system-architecture.md](../../system-architecture.md)
  - [docs/archive/plan/architecture/settings-center.md](../../archive/plan/architecture/settings-center.md)
- 相关服务/文件：
  - `services/api/internal/config/config.go`
  - `services/api/internal/models/setting.go`
  - `services/api/internal/services/playback/ranking_allowlist.go`
  - `services/api/internal/services/accessauth/admin_api_key.go`
- 当前行为：
  - `ConfigService.GetString()` 通过 `ResolveString()` 读取配置。
  - `resolveRawValue()` 在未提供 `settingsMap` 时，会按单个 definition 调用 `loadSettings()`。
  - `loadSettings()` 当前直接查询数据库，不带缓存。
- 现有限制：
  - 同一个请求里多次 `NewConfigService().GetString("EMBY_URL")` 会重复打数据库。
  - 当前不仅 `ConfigService.Update()` 会写 `settings`，业务代码里还有直接写 `models.Setting` 的路径；如果只给读取加缓存而不处理写后失效，缓存一定会脏。

## 方案设计

### 1. 用户可见行为

- 管理员通过设置中心修改配置后，当前 API 实例内应立即生效，不要求等待 TTL 到期。
- 普通用户、管理员和 Bot/Internal API 的对外协议都不变。
- 现有 env fallback、默认值、敏感项解密和校验逻辑必须保持不变。

### 2. 数据与模型

> 本次不涉及数据模型变更。

新增运行期内存结构：

- `settingsKeyCache`
  - key: `settings.key`
  - value:
    - `setting models.Setting`
    - `found bool`
    - `loadedAt time.Time`
- TTL：`60s`

说明：

- `found=false` 表示 negative cache，用于缓存“数据库里没有这个 key”。
- 缓存原始 `models.Setting`，不缓存最终解析后的字符串，避免破坏现有解密 / env fallback / 默认值逻辑。

### 3. 接口与边界

- 不新增外部 API。
- `ConfigService` 内部读取逻辑改为：
  - 先查 key 级缓存
  - miss 或过期时再查数据库
- 所有写 `settings` 的路径必须接入统一失效入口。

边界约束：

- 只缓存数据库层 `settings` 读取结果，不缓存环境变量读取结果。
- `List()` / `Get()` / `GetString()` 继续走现有定义解析逻辑，只把底层 DB 命中改成缓存命中。
- negative cache 的 key 也必须在写入成功后失效，否则“先查不到、后写入”会被卡住到 TTL 结束。

### 4. 关键流程

#### 4.1 读取

1. `ConfigService.GetString()` / `ResolveString()` 进入 `resolveRawValue()`。
2. 如果需要从数据库读取某个 key：
   - 先查本地 `settingsKeyCache`。
   - 若命中且未过期，直接返回缓存结果。
   - 若未命中或已过期，进入数据库查询。
3. 查询结果写回缓存：
   - 查到记录：`found=true`
   - 查不到记录：`found=false`
4. 上层继续沿用现有逻辑做解密、env fallback、默认值回退。

#### 4.2 并发控制

1. 同一个 key 在 TTL 过期时，多个并发请求可能同时 miss。
2. 为避免同 key 并发回源，缓存层对单个 key 使用轻量 single-flight 合并同批读取。
3. 该 single-flight 只用于“同 key 同时 miss”的去重，不承担后台刷新职责。

#### 4.3 写入与删除

1. 任意路径成功写入 `settings.key` 后，主动失效该 key 的缓存。
2. 任意路径成功删除 `settings.key` 后，主动失效该 key 的缓存。
3. 下一次读取该 key 时重新按 DB → env/default 逻辑解析。

#### 4.4 覆盖范围

首批必须接入缓存失效的写路径：

1. `ConfigService.Update()`
2. 直接 upsert / delete `models.Setting` 的业务路径
   - 如排行榜 allowlist
   - 如 Admin API Key 相关 key
   - 其他仓库内直接写 `settings` 的逻辑

未接入这些写路径之前，不允许把读取缓存视为完成态。

### 5. 失败路径与边界条件

- 缓存 miss + DB 查询失败：直接返回原有错误，不写缓存。
- 缓存命中但已过期：按 miss 处理。
- 缓存里是 `found=false`，TTL 未过期：直接视为数据库无记录，再走 env/default fallback。
- 写入数据库成功但缓存失效失败：
  - 当前请求不回滚数据库写入；
  - 记录错误日志；
  - 最迟在 `60s` 后自然过期恢复。
- 多实例部署：
  - 只保证单实例写后立即生效；
  - 其他实例最多保留 `60s` 旧值窗口。
- 敏感值：
  - 缓存数据库原始密文记录，不在缓存层额外持久化解密结果。

## 影响范围

涉及的子系统：

- API：有
  - `ConfigService` 读取路径
  - 所有通过 API 进程写 `settings` 的路径
- Web：无直接协议变化
- Bot：无直接协议变化
- 配置/部署：无新增环境变量，无 migration
- 文档：有
  - 落地后需同步 `docs/system-architecture.md`

## 验证方式

### 编译/测试

- `cd services/api && go test ./...`
- `cd services/api && go build ./...`
- `cd services/api && go vet ./...`

### 手工验证

- 同一个请求链路重复读取 `EMBY_URL` / `EMBY_API_KEY` / `CRON_TIMEZONE` 时，SQL 日志不再重复查询同一个 key。
- 首次读取不存在的 key 后，`60s` 内再次读取不再重复查空。
- 管理员在设置中心修改某个配置后，当前实例后续请求立即读到新值。
- 通过业务入口写入 `settings`（例如排行榜 allowlist）后，后续请求立即读到新值，不受 `60s` 旧缓存影响。

## 落地后文档处理

已同步处理：

- “ConfigService 对 `settings` 使用按 key 的进程内 TTL 缓存，TTL=60s，写后失效”的稳定结论已提炼到 `docs/system-architecture.md`
- 若后续确认仍需更强缓存能力（批量快照、跨实例同步、外部缓存），另起新方案，不在本稿扩展
- 本稿已迁入 `docs/archive/plan/architecture/`
