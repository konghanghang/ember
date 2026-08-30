# 运行期配置缓存演进方案

> 状态：草稿（观察期，尚未排期）
> 负责人：Ember
> 更新时间：2026-08-30

## 背景

Ember 已为数据库 `settings` 建立按 key 的进程内缓存，解决同一配置被高频重复查询的问题。现有实现满足“短时间复用 + 写后失效”，但只使用基于加载时间的惰性 TTL：缓存到期后由下一次访问同步回源，访问不会延长空闲寿命，长期无人访问的条目也不会主动淘汰。

当前没有证据表明这套实现已经造成容量、延迟或可用性问题，因此本计划不立即替换代码。它只记录后续演进方向、候选库、适用边界和启动条件，避免未来因单个调用点需要异步刷新而继续在自研缓存中叠加特殊分支。

如果未来需要类似 Caffeine 的 `refreshAfterWrite + expireAfterAccess` 语义，应先按本计划重新核对运行数据和依赖版本，再决定是扩展现有实现还是引入成熟库。

## 目标

本方案要实现：

1. 记录 API、Gateway 和 Bot 当前涉及数据库运行期配置的缓存行为与一致性边界。
2. 定义普通配置、动态开关、安全配置和启动期配置各自允许采用的缓存策略。
3. 明确引入 Caffeine 风格 Go 缓存库的候选、前置条件、迁移步骤和验证门槛。
4. 保留设置中心写后立即生效、敏感值保护和现有对外协议，不让缓存替换改变业务语义。

## 非目标

本次明确不做：

- 不在本轮修改 `services/api`、`services/bot` 或引入新的 Go 依赖。
- 不把媒体统计、PlaybackInfo 证明、TMDB、排行榜、搜索会话等业务缓存统一迁入本方案。
- 不承诺跨 API/Gateway/Bot 进程或多副本的立即失效；进程间一致性需要单独设计。
- 不缓存 Admin API Key 实时校验等安全敏感读取，也不让缓存刷新代替启动期组件重建。
- 不预先确定最终刷新时间；文中的候选值需要以查询量、更新频率和故障行为实证后确认。

## 当前事实

### 相关文档与实现

- 已归档的当前实现方案：[settings 按 Key 本地缓存方案](../../archive/plan/architecture/settings-key-cache.md)
- 系统边界：[系统架构](../../system-architecture.md)
- 配置来源与生效规则：[配置参考](../../reference/configuration-reference.md)
- Bot 运行期配置边界：[Bot 架构参考](../../reference/bot-architecture-reference.md)
- Gateway 独立进程配置背景：[Ember Gateway 透明代理与 Web 访问方案](./ember-gateway-transparent-proxy-and-web-access.md)
- Go 通用缓存实现：`services/api/internal/config/settings_cache.go`
- Go 配置解析入口：`services/api/internal/config/config.go`
- API 到 Bot 通知配置缓存：`services/api/internal/integrations/notifier/notifier.go`
- Bot 运行期配置缓存：`services/bot/app/runtime_settings.py`

### 当前缓存行为

| 范围 | 当前策略 | 刷新方式 | 失败语义 | 空闲淘汰 |
| --- | --- | --- | --- | --- |
| 普通 `ConfigService` 数据库 key | 加载后 `60s` 内命中；包含 negative cache | TTL 到期后的下一次访问同步查询数据库 | 数据库错误不缓存，下次访问继续重试 | 无；过期条目保留到再次读取或主动失效 |
| `LOG_LEVEL`、`PLAYBACK_GATEWAY_WEB_ENABLED` | Gateway 专用 `5s` 进程缓存 | 下一次相关 Gateway 请求同步刷新；同 key 并发合并 | 错误退避 `5s`；日志级别保留上一次有效运行值，Web 开关读取失败返回 `503` | 无 |
| Bot Telegram 运行期配置 | Bot 聚合结果缓存 `30s` | 下一次 Bot 调用通过 Internal API 刷新 | 保留上一次有效值，不以空结果覆盖 | 无 |
| API `BOT_NOTIFY_URL` | `30s` 刷新节流，底层仍经过 `ConfigService` | 下一次通知调用触发检查，支持显式 `Reload()` | 沿用 `ConfigService.GetString` 的空值语义 | 无 |

普通 Go 缓存还具备以下边界：

- 缓存原始 `models.Setting`，不缓存解密后的敏感明文，也不改变 database/env/default 解析优先级。
- 单 key miss/过期支持并发回源合并；批量 `List()` 多 key 同时过期不等价于完整的逐 key refresh single-flight。
- 设置中心、排行榜 allowlist 和 Admin API Key 等已知写路径成功后会失效当前 API 进程对应 key。
- 失效使用 generation 防止已经在途的旧查询结果重新填回缓存。
- 缓存只在当前进程有效；其他 API 实例没有跨进程失效，普通 key 最多保留一个 `60s` 的旧值窗口。

### 不应被通用缓存覆盖的读取

- Admin API Key 认证直接读取数据库 hash，以保持禁用和轮换的实时安全语义。
- 排行榜媒体库 allowlist 由排行榜领域直接读取，不以 `ConfigService` 缓存作为业务真相。
- Cron 开关、表达式和调度时区在 API 启动时装配；更新缓存不会自动重建 cron。
- Gateway 的 `EMBY_URL` / `EMBY_API_KEY` 在 Gateway 初始化时用于上游身份核对和代理装配，不能只刷新一个字符串缓存就宣称运行期切换完成。

## 候选技术

以下结论核对于 2026-08-30；进入实现前必须重新检查最新稳定版、Go 版本要求、公开 API 和维护状态。

### 首选候选：Otter v2

[Otter v2](https://github.com/maypok86/otter) 是当前最接近目标语义的 Go 进程内缓存候选：

- `ExpiryAccessing` 可表达访问后空闲过期。
- `RefreshWriting` 可表达写入或加载后的刷新窗口。
- stale key 首次访问可先返回旧值并触发异步刷新。
- 内置 loader、同 key 并发合并、容量限制、W-TinyLFU、统计和主动失效能力。

当前稳定版 [v2.3.0](https://github.com/maypok86/otter/releases/tag/v2.3.0) 的 [go.mod](https://github.com/maypok86/otter/blob/v2.3.0/go.mod) 要求 Go `1.24`。Ember 当前模块声明 `go 1.23`，同时固定 `toolchain go1.24.13`；如采用 Otter v2，需要把“最低 Go 语言版本是否提升到 1.24”作为独立兼容性决策，并同步检查 Docker 构建镜像、CI、开发文档和发布基线。

### 备选：Theine

[Theine](https://github.com/Yiling-J/theine-go) 支持 W-TinyLFU、TTL、LoadingCache 和 singleflight，最低 Go 版本要求更宽松，但其公开 API 没有直接覆盖本计划需要的 `refreshAfterWrite + expireAfterAccess` 组合。若仍需自行补刷新编排，它相对现有实现的收益不足，不作为当前首选。

### 暂不选择轻量 TTL 库

只提供固定 TTL、访问续期或惰性 loader 的轻量库无法同时解决异步刷新、stale value、失败退避和失效竞态。若目标只是维持当前 60 秒惰性 TTL，继续保留现有实现比更换一个同能力依赖更简单。

## 方案设计

### 1. 用户可见行为

- 设置中心 API、字段名、来源优先级、敏感值掩码和保存响应保持不变。
- 当前 API 进程通过设置中心保存配置后仍应立即使用新值，不能等待 refresh window。
- 其他进程或副本的最终一致窗口必须明确记录，不得把进程内缓存描述为跨进程同步。
- 配置刷新失败时，不同 key 按业务风险选择“保留旧值”或“失败关闭”，不能统一吞错。

### 2. 数据与模型

> 本计划不涉及数据模型变更；如后续只替换进程内缓存，也不需要 SQL migration。

缓存继续保存原始 `models.Setting + found` 语义：

- `found=true`：数据库中存在该 key，值可能是密文。
- `found=false`：negative cache，供上层继续执行 env/default fallback。
- 缓存层不得保存解密后的敏感值、Token、Cookie 或完整数据库错误响应。

### 3. 缓存策略分类

| 配置类别 | 候选策略 | 说明 |
| --- | --- | --- |
| 普通动态运行期配置 | 候选 `refreshAfterWrite=60s`、`expireAfterAccess=10m` | 首次加载同步；刷新窗口到期后访问先返回旧值并异步刷新；设置中心写后立即失效 |
| Gateway 高频动态开关 | 保留独立 `5s` freshness policy | `LOG_LEVEL` 与 Web Surface 对时效和失败结果有明确合同，不应被普通 60 秒策略覆盖 |
| Bot 聚合运行期配置 | 首期保持 Bot 当前 `30s` 缓存 | 跨语言、跨进程且已有保留旧值合同，不纳入 Go 缓存首批替换 |
| 安全实时配置 | 不缓存或使用专用强失效策略 | Admin API Key 禁用/轮换必须立即影响认证，不接受普通最终一致窗口 |
| 启动期配置 | 不做自动 refresh | Cron、监听、Gateway 上游身份等需要重建组件，继续通过 `restartRequired` 表达 |

候选时间只是初始评估值。最终值至少要结合以下数据确认：

- `settings` 查询频率、命中率和数据库耗时。
- 配置真实修改频率及可接受的跨进程生效窗口。
- 刷新错误持续时间和旧值是否安全。
- 当前缓存条目数量、长期空闲 key 数量和实际内存占用。

### 4. 接口与边界

- 不修改外部 API 或 Internal API。
- 在 `internal/config` 内保留项目自己的窄缓存接口，业务层不得直接依赖第三方缓存类型。
- loader 只负责按 key 读取 `settings`；解密、normalize、env fallback 和 default fallback 继续留在 `ConfigService`。
- 写后失效必须继续覆盖所有直接 upsert/delete `settings` 的路径；替换库不能丢掉现有 generation/in-flight 竞态保护语义。
- 第三方缓存的后台任务必须接入进程生命周期，测试和正常 shutdown 后不得遗留 goroutine。

### 5. 关键流程

1. 首次访问普通 key 时，通过 loader 同步读取数据库并缓存原始记录或 negative entry。
2. refresh window 内直接返回缓存值。
3. refresh window 到期后的首次访问返回上一次有效值，并且只启动一个异步刷新任务。
4. 刷新成功后原子替换缓存值；刷新失败时按 key policy 保留旧值并执行有界退避。
5. expire-after-access 窗口内没有任何访问时淘汰条目；下次访问重新同步加载。
6. 当前进程成功写入或删除配置后立即失效对应 key；在途旧 refresh 不能覆盖失效后的新一代值。

### 6. 失败路径与边界条件

- 初次加载失败：没有旧值可用，返回原有数据库错误，不伪造默认值。
- 异步刷新失败：普通配置保留旧值并记录固定 key/阶段/错误类型；禁止记录配置值和原始敏感错误内容。
- negative entry 刷新失败：不得把数据库故障继续解释为“配置不存在”。
- 设置中心写入与异步刷新竞态：写后失效优先，旧 refresh 结果不得重新填充。
- Gateway Web 开关读取失败：继续遵守现行 fail-closed `503` 合同，不因通用 stale policy 改成继续开放。
- 多实例部署：除非另行实现 PostgreSQL `LISTEN/NOTIFY`、消息总线或外部缓存，否则仍只承诺最终一致，文档必须写明窗口。
- 依赖版本：引入 Otter v2 前必须明确批准 Go `1.24` 基线变化，不能由 `go get` 顺手修改模块合同。

## 替换启动条件

满足以下任一类实证后，才进入实现阶段：

1. 参数化 SQL 或数据库指标证明 `settings` 查询仍是可观测热点，现有 TTL 无法满足负载。
2. 请求延迟或错误日志证明同步过期回源影响关键链路，需要 stale-while-refresh。
3. 新增多个 key 需要访问后过期、失败退避、异步刷新，继续扩展自研状态机会明显增加维护成本。
4. 当前缓存出现可复现的失效竞态、内存滞留或并发批量回源问题。
5. Ember 已正式接受 Go `1.24+` 为最低构建基线，并完成 Docker/CI/发布链验证。

如果没有上述证据，保持现有实现，不为了“换库”而换库。

## 影响范围

- API：未来有，首批只涉及 `internal/config`、所有 `settings` 写后失效入口及配置测试。
- Web：无协议或页面变化。
- Bot：首批无代码变化，继续保持现有 30 秒聚合缓存和失败保留旧值语义。
- 配置/部署：采用 Otter v2 时可能要求 Go 最低版本提升到 1.24；不新增环境变量或数据库配置项。
- 文档：实现后需同步 `docs/system-architecture.md`、`docs/reference/configuration-reference.md`、测试 runbook 和依赖/构建基线说明。

## 验证方式

### 编译/测试

- `cd services/api && go test ./internal/config -count=1`
- `cd services/api && go test -race ./internal/config -count=1`
- `cd services/api && go test ./...`
- `cd services/api && go vet ./...`
- `cd services/api && go build ./...`

测试必须覆盖：

- 首次加载、命中、negative cache 和加载失败。
- refresh window 到期后返回旧值并且只触发一次异步刷新。
- refresh 成功、失败保留旧值和失败退避。
- expire-after-access 访问续期及空闲淘汰。
- 写后失效、删除后失效和在途 refresh/失效竞态。
- 普通 `60s` policy 与 Gateway `5s` policy 隔离。
- 敏感配置只缓存原始密文记录，不在日志或测试输出中泄露值。

### 非真实外部验证

- 使用 fake loader 和可控时钟验证刷新与过期，不连接真实 Emby、TMDB、MoviePilot、Stripe 或 Telegram。
- 通过 loader 调用计数证明同 key 并发刷新只回源一次。
- 通过故障注入证明数据库异常不会被错误记录成 negative cache。
- 如决定升级 Go 基线，使用当前 Docker 构建和 CI 验证依赖可编译；不以本机 toolchain 通过代替发布链验证。

## 已完成项、剩余项与归档条件

### 已完成

- 已盘点普通 ConfigService、Gateway 特殊 key、Bot runtime settings 和 BotNotifier 的缓存现状。
- 已确认当前实现明确不包含后台刷新和 expire-after-access。
- 已记录 Otter v2、Theine 的能力边界以及 Otter v2 的 Go 1.24 前置条件。
- 已确定“无实证不替换”的启动门槛。

### 剩余

- 收集 `settings` 查询频率、延迟、错误和缓存命中实证。
- 决定 Ember 最低 Go 版本是否提升到 1.24。
- 在实现时重新核对候选库版本、许可证、维护状态和 API。
- 根据证据确定最终 refresh/expiry 时间和不同 key 的失败策略。
- 完成 TDD、实现、全量测试、构建验证及现行文档同步。

### 归档条件

满足以下任一条件后移入 `docs/archive/plan/architecture/`：

1. 已完成替换并通过计划内验证，稳定行为已提炼到系统架构和配置参考；或
2. 经指标确认现有缓存足够，明确记录不替换结论及重新评估条件。

## 落地后文档处理

落地后应同步处理：

- 在 `docs/system-architecture.md` 固化最终缓存策略、进程边界和失败语义。
- 在 `docs/reference/configuration-reference.md` 标明各类运行期配置的实际生效窗口。
- 如提升 Go 最低版本，同步构建、部署、开发约定和发布文档。
- 完成态从 `docs/plan/README.md` 移除并迁入 `docs/archive/plan/architecture/`，同时更新直接引用路径。
