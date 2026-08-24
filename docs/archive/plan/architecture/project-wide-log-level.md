# 项目级日志级别实现方案

> 状态：已完成
> 负责人：Ember
> 更新时间：2026-08-24

## 背景

Ember 当前没有统一的应用日志级别：

- Go API 与 Gateway 通过标准库 `log.Printf` 写入 stdout 和按日文件，现有日志初始化只区分进程角色，不区分级别。
- Gateway 为每个请求无条件打印完整脱敏 `request_completed`，Infuse 扫库、条目详情、Range 和进度上报会产生大量正常成功日志；这些请求形态对新播放器兼容排查有价值，但不应默认输出。
- GORM 固定使用 `logger.Info`，默认打印所有参数化 SQL；Gin 默认记录除 `/`、`/health` 外的全部请求。
- Bot 已使用 Python `logging` 的标准级别，但根级别固定为 `INFO`，无法由部署环境统一切换。
- Compose 已分别隔离 API、Gateway、Bot 的文件日志和 stdout 轮转，但没有一个项目级变量统一控制 Ember 应用日志详细程度。

如果直接删除详细日志，后续接入新播放器时会失去 Header 载体、路径模式、query key、压缩方式和请求时序证据；如果保持现状，正常扫库与播放会持续刷屏并放大日志存储量。

## 目标

1. 新增唯一项目级环境变量 `LOG_LEVEL`，同时控制 Ember API、Gateway 和 Bot 的应用日志详细程度。
2. 第一版只支持 `info` 与 `debug`，默认 `info`；详细请求形态、SQL、缓存命中和正常高频访问归入 Debug。
3. 保留启动、状态流转、外部调用失败、视频最终决策和安全拒绝等 Info 证据，不因降噪丢失关键排障信息。
4. 保持所有级别的敏感信息边界一致；Debug 不能输出 Token、Cookie、Authorization 原值、query value、完整外部 URL、媒体 Path 或完整响应体。
5. 让非法配置安全回退 `info` 并记录一次固定警告，不因日志配置错误阻断服务。

## 非目标

- 第一版不开放 `warn`、`error` 作为过滤阈值；历史 Go 日志完成级别迁移前，不能冒险隐藏仍使用 `log.Printf` 的关键失败信息。
- 不在本次一次性迁移全部历史 Go `log.Printf`；未显式分级的旧日志继续按 Info 兼容。
- 不让 `LOG_LEVEL` 控制 Vue 浏览器控制台、Nginx、Docker logging driver 或 PostgreSQL 自身日志；它们不是 Ember 应用进程日志。
- 不增加设置中心配置、管理 API 或 Web 开关；日志级别是跨进程启动参数，修改后通过重启目标服务生效。
- 不在本次处理文件日志保留天数、自动删除或日志表。

## 当前事实

- 系统架构入口：`docs/system-architecture.md`
- 配置真相入口：`docs/reference/configuration-reference.md`
- 部署入口：`infrastructure/docker/docker-compose.yml`、`infrastructure/docker/.env.example`
- Go 日志入口：`services/api/internal/logging/logger.go`
- API HTTP 日志：`services/api/internal/app/server.go`
- GORM 日志：`services/api/internal/db/db.go`
- Gateway 诊断日志：`services/api/internal/playbackgateway/request_log.go` 及相邻编排文件
- Bot 日志入口：`services/bot/app/server.py`
- 当前 Go API 约有 483 处标准输出/日志调用，分布在约 60 个文件；Bot 已有约 51 处标准级别日志调用。
- `request_completed` 已做字段边界与凭证脱敏，问题是默认无条件输出，不是字段安全合同本身失效。
- Compose 已为各容器 stdout 配置 `10m × 3` 轮转，并为 API、Gateway、Bot 挂载独立日志卷；本次保持该拓扑不变。

## 方案设计

### 1. 用户可见行为

- 未配置 `LOG_LEVEL` 时保持服务可用，最终级别为 `info`。
- `LOG_LEVEL=info` 时：
  - API 保留现有业务 Info、启动、失败与状态流转日志；普通成功 Gin access log 不逐请求输出。
  - GORM 只输出慢 SQL 与错误，不输出每条正常 SQL。
  - Gateway 保留认证映射、实际 PlaybackInfo 补取、最终视频 `redirect/fallback/reject`、失败和启动日志；完整请求摘要、扫描和缓存命中不输出。
  - Bot 保留现有 Info 及更高等级日志。
- `LOG_LEVEL=debug` 时，在上述基础上增加安全的详细请求摘要、正常 access log、参数化 SQL、Gateway 缓存/请求形态和 Bot 应用 Debug 日志。
- 修改环境变量后重启对应容器生效，不承诺热更新。

### 2. 数据与模型

> 本次不涉及数据模型变更。

- 不新增数据库字段、表、索引或 migration。
- `LOG_LEVEL` 只来自进程环境变量，不写入 `settings` 表。

### 3. 配置与接口边界

新增项目级环境变量：

```text
LOG_LEVEL=info|debug
```

固定语义：

| 输入 | 最终级别 | 行为 |
| --- | --- | --- |
| 空值 | `info` | 默认正常运行 |
| `info`，大小写与首尾空格不敏感 | `info` | 关键事件与失败日志 |
| `debug`，大小写与首尾空格不敏感 | `debug` | 在 Info 基础上增加详细诊断 |
| 其他值 | `info` | 记录一次 `log_level_invalid` 后回退，不阻断服务 |

Compose 将同一值传入 `ember-api`、`ember-gateway`、`ember-bot`。本次不修改 HTTP API、Internal API、Bot webhook 或前端接口。

### 4. Go 日志边界

`internal/logging` 负责：

1. 在进程日志初始化时解析 `LOG_LEVEL`，保留当前按角色选择 `api/gateway` 文件前缀和 stdout 双写行为。
2. 提供可测试的 `Debugf`、`Infof` 与 `DebugEnabled`；旧 `log.Printf` 继续按 Info 兼容。
3. 初始化完成后记录固定 `logging_initialized processRole logLevel`；非法值只记录固定 code 与 fallback，不回显任意长原始输入。
4. GORM 根据最终级别映射：`info → logger.Warn`、`debug → logger.Info`，继续启用 `ParameterizedQueries`。
5. Gin 使用安全 access logger：正常请求只在 Debug 输出；Info 下只保留失败或慢请求摘要，禁止记录 query value、Authorization、Cookie 或请求体。

### 5. Gateway 日志分级

- Debug：
  - 完整脱敏 `request_completed`
  - `item_container_snapshot_recorded`
  - `playback_info_reused_on_demand`
  - 普通成功 bootstrap/protected/item detail/Playing/Progress 请求形态
- Info：
  - listener / 上游身份就绪
  - 认证映射
  - `playback_info_resolved_on_demand`
  - 每个视频请求唯一的 `redirect/fallback/reject` 最终决策
  - 固定失败、拒绝和上游不可用信息
- 重复事件继续收口：客户端取消不得同时由 Store、Gateway 和 request completion 打三条同义日志；视频 Info 决策不因 Debug 开启而重复生成第二条决策。

### 6. Bot 日志分级

- `configure_logging()` 解析相同的 `LOG_LEVEL`，设置 Ember `app.*` 日志级别。
- 即使 `LOG_LEVEL=debug`，`httpx`、`httpcore`、`telegram` 等第三方 logger 仍至少保持 `WARNING`，避免第三方 HTTP 调试日志输出 Telegram Bot API URL、Header 或其他敏感上下文。
- Uvicorn 原生 access log 固定关闭，避免其完整 path/query 绕过 Ember 脱敏边界；由 Bot 应用中间件在 Debug 输出 method、路由模板、状态和耗时，Info 只保留失败摘要，`/health` 继续跳过。
- stdout 与按日文件、14 份 Bot 文件备份策略保持不变。

### 7. 关键流程

1. Compose 或本地环境提供可选 `LOG_LEVEL`。
2. API/Gateway 在业务依赖初始化前解析最终级别并配置共享日志 sink。
3. API 按最终级别装配 GORM 与 Gin 日志；Gateway 在同一 Go 日志边界上输出 Info 或 Debug。
4. Bot 在模块初始化期解析同一变量并配置应用与第三方 logger。
5. 新播放器排查时，运维设置 `LOG_LEVEL=debug`、重启相关服务、复现并收集脱敏证据；排查结束后恢复 `info`。

### 8. 失败路径与边界条件

- `LOG_LEVEL` 缺失：静默使用 `info`。
- `LOG_LEVEL` 非法：服务继续启动，输出一次固定 `log_level_invalid fallbackLevel=info`。
- Debug 关闭：不能影响请求路由、状态码、代理、定时任务、Bot 更新模式或数据库行为。
- Debug 开启：不能改变响应、外部调用或状态流转，也不能输出任何秘密或完整外部响应。
- 第三方 logger：不能因根 logger 切到 Debug 而放宽到可能泄密的 HTTP wire 日志。
- 测试并发：全局日志级别测试必须可隔离恢复，不能污染其他测试。

## 影响范围

- API：有；共享日志包、entrypoint、Gin access、GORM logger 以及针对性高频日志分级。
- Gateway：有；请求摘要、缓存/证明日志和最终视频决策的级别归类。
- Web：无业务代码改动；浏览器 console 不受 `LOG_LEVEL` 控制。
- Bot：有；根日志配置、Uvicorn access 与第三方 logger 上限。
- 配置/部署：有；Compose 与 API/Bot/Docker 示例环境变量增加 `LOG_LEVEL`。
- 数据库：无。
- 文档：同步系统架构、配置参考、部署环境、Gateway 代理合同和 115 端到端流程。

## 分阶段落地

### 阶段 1：全局级别基础与高噪声入口

- Go 日志级别解析、Info/Debug 边界和初始化证据。
- GORM/Gin 根据全局级别装配。
- Gateway 详细请求日志归 Debug、最终决策保留 Info。
- Bot 读取同一变量并固定第三方 logger 安全上限。
- Compose、示例配置和稳定文档同步。

完成条件：默认 Info 不再打印普通成功请求、全部 SQL和 Gateway 详细请求摘要；Debug 能恢复这些脱敏诊断；API/Gateway/Bot 使用同一输入语义。

### 阶段 2：历史日志持续分级

- 修改具体业务模块时，把相关历史 `log.Printf` 迁移到显式 Info/Debug/Warning/Error 语义。
- 全部关键历史日志完成审计前，不开放 `warn/error` 过滤阈值。
- 阶段 2 是持续治理，不阻塞阶段 1 交付和本计划主体归档；剩余清单进入稳定日志规范维护。

## 验证方式

### 自动化验证

- Go TDD：级别解析、默认值、非法回退、Debug 过滤、进程角色文件隔离。
- GORM：Info 映射为 Warn、Debug 映射为 Info，参数化查询保持开启。
- Gin：Info 下正常 `2xx` 不输出，失败/慢请求输出安全摘要；Debug 下正常请求输出但不含 query value。
- Gateway：Info 下无完整 `request_completed`/缓存命中，视频决策和失败仍存在；Debug 下恢复完整脱敏摘要；每个视频请求仍只有一条决策日志。
- Bot：Info/Debug 解析、非法回退、第三方 logger 不低于 Warning、health access filter 保持。
- 验证命令：

```bash
go -C services/api test ./internal/logging ./internal/db ./internal/app ./internal/entrypoint ./internal/playbackgateway ./internal/services/embytoken
go -C services/api test -race ./internal/logging ./internal/playbackgateway ./internal/services/embytoken
go -C services/api test ./...
go -C services/api vet ./...
go -C services/api build ./...
services/bot/.venv/bin/python -m py_compile services/bot/main.py
services/bot/.venv/bin/python -m pytest services/bot/tests
docker compose -f infrastructure/docker/docker-compose.yml config
```

所有自动化测试必须 fake 外部依赖，不请求真实 Emby、115、Telegram 或其他外网服务，不启动项目服务。

### 手工验证

- `LOG_LEVEL=info`：普通 API、Infuse 扫库、Playing/Progress 和 GORM 正常 SQL 不刷详细日志；视频最终决策、登录和失败仍可见。
- `LOG_LEVEL=debug`：上述详细脱敏日志恢复，API/Gateway/Bot 都显示相同最终级别。
- 非法值：三个服务继续启动并回退 Info，日志只出现固定警告。
- 手工运行只在用户明确授权部署/启动后执行；本次实现默认只做编译、测试和 Compose 静态校验。

## 实施结果

- `LOG_LEVEL=info|debug` 已由 API、Gateway、Bot 共用，Compose 和三个示例环境文件已同步；默认与非法回退均为 `info`。
- Go 共享日志边界已提供 Info/Debug 过滤；Gin 正常成功 access 与 Gateway 详细请求摘要归 Debug，API access 使用路由模板而非实际 path 参数，避免兑换码等路径值进入日志。
- GORM 在 Info 下只保留慢 SQL/错误，在 Debug 下恢复全部参数化 SQL；绑定值继续隐藏。
- Gateway 的 `request_completed`、Container 快照成功和 PlaybackInfo 复用归 Debug；视频最终决策继续在 Info 输出；普通请求取消不再由 Store、Gateway 和完成摘要重复输出三次。
- Bot 已读取同一变量，Uvicorn 原生 access 固定关闭并由安全路由模板摘要替代，`httpx/httpcore/telegram` 固定不低于 Warning。
- 已通过 API `go test ./...`、目标包 race、`go vet ./...`、`go build ./...`；Bot `47 passed` 与 `py_compile`；Compose 静态配置和 `git diff --check`。本轮未启动服务，未执行改造后的运行时手工切换验证。
- 2026-08-24 首次本地启动验收发现 entrypoint 在 `InitDB` 加载 `.env` 之前已初始化日志，导致写在 `.env` 中的 `LOG_LEVEL=debug` 被当作空值并回退 Info；后续已将可选 dotenv bootstrap 提前到日志初始化之前，并保留 `InitDB` 直接调用方的静默兜底。该修复已有加载优先级、环境覆盖和 entrypoint 顺序测试，用户再次启动已确认 `.env` 中的 Debug 级别正确生效。

## 落地后文档处理

- 稳定配置进入 `docs/reference/configuration-reference.md`。
- 进程日志边界进入 `docs/system-architecture.md` 与 `docs/runbooks/deployment-environment.md`。
- Gateway Debug/Info 合同同步到 `docs/reference/emby-playback-proxy-contract.md` 和 `docs/reference/p115-playback-end-to-end-flow.md`。
- 阶段 1 已完成，稳定事实已进入上述现行文档；本文已迁入 `docs/archive/plan/architecture/`，只保留实现边界和验证过程的历史追溯价值。历史日志持续分级由稳定规范承接。
