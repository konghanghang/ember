# Gateway 只读路径优化方案

> 状态：本地实现与验证完成，待部署和真实收益验收
> 负责人：Ember
> 更新时间：2026-11-03

## 背景

同一媒体的稳定取链失败可能让连续视频请求重复执行源读取与失败任务写入。首页和列表同时产生的图片短请求，也存在连接复用、重复读取与并发合并的优化空间。用户已授权实施失败短期退避、专用 Emby HTTP 连接池、图片条件缓存三项优化。

本方案依据现行代码及固定上游源码对照，不将候选性能收益写成已证实结果。上游参考沿用[客户端兼容主计划](./gateway-client-compatibility.md)中的固定提交；不引入上游详情预热、管理员凭证注入或宽缓存键。

## 目标

1. 对选定的类型化稳定 URL 政策错误提供有界短期退避，减少重复源读取，同时保留正常目标与成功缓存复用。
2. 为 Gateway 构造专用 Emby HTTP Transport，改善短请求连接复用，并保留流式视频、WebSocket、取消和自定义 Transport 注入。
3. 在现有请求门控之后，为精确图片请求提供有界内存缓存、条件重验证和并发请求合并，保持 HTTP 响应语义。

## 非目标

- 不改变 strict / compatible 首次转存策略，不把图片预热或视频 GET 当作明确起播证据；正常转存、已有文件和成功缓存复用继续沿用现行合同。
- 不改变身份、用户资格、账号健康、HEAD 禁新增、租约、配额和任务所有权；不放宽 115 下载域名白名单。
- 不增加字幕缓存、后台预热、跨进程缓存、持久化缓存、数据库模型、SQL migration、环境变量或设置中心入口。
- 不改 Web / Bot 页面和接口，不启动项目服务，不请求真实 Emby / 115，不执行生产部署、数据库写入或 Git 提交。

## 当前事实

本轮调查基线为 `master@dbfd118266c9269655476616c95ce000a3982918`；下表记录实施前行为。

| 范围 | 已确认行为与代码入口 |
| --- | --- |
| 源 preID 缓存 | `services/api/internal/services/directplay/preid_cache.go` 的 `sourcePreID` 只保存成功结果；并发门串行化后，失败后的下一请求仍可重新读取。 |
| 转存编排 | `directplay/service.go` 的 `resolveUnderLock` 在锁内查重和准入后开始任务，再执行 preID；已有目标与媒体缓存有独立复用路径。 |
| HTTP 装配 | `playbackgateway/process.go` 未注入生产 Transport，`runtime.go` 转交依赖，`gateway.go` 的 `New` 在缺省时使用 `http.DefaultTransport`。 |
| 图片路径 | `playbackgateway/web_surface.go` 已限定匿名图片范围；`gateway.go` 对精确匿名图片直接代理，受保护请求解析 Principal 后代理，当前没有图片响应体缓存或图片并发合并。 |
| 现有保护 | 同身份 PlaybackInfo 合并、直链/媒体/preID 成功缓存、源目录在途合并、账号限流、停止撤销和内容锁内查重均已存在，不重复实现。 |

Emby method、path、权限和 DTO 继续复用[播放代理 API 合同](../../reference/emby-playback-proxy-contract.md)的 `4.9.3.0` 固定证据与已确认运行范围。时区沿用全局 `CRON_TIMEZONE`；HTTP 日期在协议边界按 HTTP 合同处理，不新增子系统时区。

## 方案设计

### 1. 用户可见行为与接口

对外路由、请求和响应字段保持不变。成功视频仍沿既有直链或 Emby 回退处理；优化不把失败伪装为播放成功，不改变客户端 PlaybackInfo 和视频响应。

缓存命中只减少内部工作，不能代替实时身份或权限校验。未满足缓存条件的图片保持原请求透明转发；本轮不扩大匿名资源范围。

### 2. 数据与资源边界

本次不涉及数据库模型变更，不需要 SQL migration。新增状态仅存在当前进程内，明确限制条目数量、单项大小、总内存及在途共享工作；进程退出后自然释放。

缓存标识使用不可逆摘要，日志仅记录固定原因、结果、数量和必要请求标识，不保留或输出明文 Token、Cookie、签名直链、完整上游响应或未知错误内容。容量耗尽和不适用条件采用明确旁路，不影响正常代理。

### 3. 稳定失败短期退避

- 仅覆盖源 preID 返回的类型化 `DownloadURLPolicyError(host_not_allowed)`，固定 `10s/1024` 条 LRU，不把任意 Provider error 放入成功 preID 缓存。
- 缓存按身份与路由边界、源账号凭证和配置代次、已验证文件身份隔离；凭证、配置或文件身份变化不能继承旧失败。
- 正常目标和成功缓存复用优先；退避检查不能越过原有身份、账号、许可、HEAD 和配额边界，也不能因此产生新的秒传或成功记账。
- 取消、deadline、临时上游错误、Redis / 数据库错误、缺少许可和账号并发拒绝不进入共享失败状态。短期窗口过后允许重新尝试，失败命中不得无限续期。
- 保留现有类型化诊断与 Emby 回退，不以退避替代 URL 合同核对，不修改账号全局健康分类。

### 4. 专用 Emby HTTP Transport

生产缺省路径基于标准 Transport 克隆构造独立连接池，全局 / 每主机空闲连接 `128/32`、空闲期限 `90s`、连接与 TLS 握手各 `5s`；不修改进程共享的默认 Transport。

显式注入的 mock 或自定义 Transport 保持有效。普通代理与内部 PlaybackInfo 使用一致的上游传输边界，关闭过程清理自有空闲连接，不擅自关闭调用方拥有的 Transport。

保留 TLS 证书校验、HTTP 协议协商、客户端取消、WebSocket 升级及流式媒体；不增加截断长视频的总请求时限。参数以本地 fake 对照和资源边界验证为依据，不直接复制上游 CPU 倍数。

### 5. 图片条件缓存与请求合并

- 仅精确图片 GET / HEAD 可参与；匿名与受保护范围沿用当前路由合同。受保护缓存命中仍先执行 Principal 与用户资格门控。
- 缓存键保留身份摘要、上游、完整路径及尺寸、质量、格式、tag、索引等变体，并尊重内容协商与 `Vary`；不同 Token / 用户身份不能共享受保护响应。
- 只保存合适的 `200 image/*`，固定 256 条、正文总量 32 MiB、单体 2 MiB、元数据 16 KiB；新鲜最多 5 分钟、保留最多 10 分钟。遵守 `Cache-Control`、`Age` 和新鲜度；要求重验证时不能直接作为新鲜结果返回。
- 重验证保留 ETag / Last-Modified 和 304 语义；已有有效 GET 表示可用于 HEAD，正文仍为空。HEAD 未命中时保持上游 HEAD 行为。
- Range / If-Range、客户端条件与强制刷新请求、Set-Cookie、不可缓存响应、超大图片及未知资源采用旁路。强制刷新不加入普通读取；内部 304 若变为不可缓存，保留原正文与新增响应头，仅交给一个等待者。未缓存正文和上游编码保持透明，不缓存认证失败。
- 同键共享工作中，每个等待者可独立取消；最多 16 个受控候选读取、每键 64 位等待者，满载旁路，退出和失败释放资源。取消或临时错误不得形成共享负缓存。
- 图片路径不调用视频准入或 DirectPlay，不因缓存 miss 或重验证创建 115 文件。

## 影响范围

| 子系统 | 有 / 无 | 触达范围 |
| --- | --- | --- |
| API / Gateway | 有 | DirectPlay 失败退避、Gateway 上游传输装配、图片代理缓存与相关测试。 |
| Web | 无 | 没有页面、视觉或交互变更。 |
| Bot | 无 | 没有命令、通知或 Internal API 变更。 |
| 配置 / 部署 | 无新增入口 | 沿用现有启动和依赖注入；本轮不部署，不新增环境变量或动态设置。 |
| 数据库 | 无 | 无模型、索引、约束、迁移或回填。 |
| 文档 | 有 | 系统架构、播放代理合同、115 Cookie / 端到端参考、测试入口与本计划的稳定行为同步。 |

## 验证方式

采用 TDD：先补失败行为或特征测试，再做最小实现。新增 / 重写方法必须提供有价值的方法注释；复杂分支、状态和恢复路径补必要说明，并补正常、边界、错误与并发测试。

| 验证项 | 关键覆盖 |
| --- | --- |
| 失败退避 | 连续 / 并发同文件稳定失败的实际读取计数、短 TTL 到期、凭证 / 配置 / 文件隔离、取消不污染、正常已有目标与成功缓存、任务及配额边界。 |
| 连接池 | fake 上游的连接复用对照、自定义 Transport 注入、TLS / 取消、WebSocket 与流式媒体不被新增总时限截断。 |
| 图片缓存 | 同身份复用与并发合并、变体 / Token / 用户隔离、撤销门控、Vary、ETag / Last-Modified / Age / 304、HEAD、不可缓存和旁路条件、资源上限、取消与失败清理。 |
| 整体回归 | strict / compatible、HEAD 禁新增、租约及配额、原始响应 / Range 保真，缓存优化不增加 115 写入。 |

在 `services/api` 工作目录执行：

```bash
go test ./internal/playbackgateway ./internal/services/directplay ./internal/integrations/p115 ./internal/services/p115account ./internal/services/p115quota -skip 'Integration|PostgreSQL' -count=1 -timeout=180s
go test -race ./internal/playbackgateway ./internal/services/directplay ./internal/integrations/p115 ./internal/services/p115account ./internal/services/p115quota -skip 'Integration|PostgreSQL' -count=1 -timeout=180s
go test ./... -skip 'Integration|PostgreSQL' -count=1
go vet ./...
go build ./...
```

复用[测试指南](../../runbooks/testing.md)的 fake/mock 边界；新增诊断字段如影响采集合同，还须运行既有采集离线和跨语言合同检查。纯文档检查覆盖相对链接、路径和 `git diff --check`。

这些验证不连接真实 Emby / 115 / Redis / PostgreSQL，不启动项目服务。生产命中率、首屏或起播收益未测量时继续标记未证实，不把 fake 请求计数变化称为真实客户端验收。

## 进度与收口

三项实施可并行，彼此没有运行时阶段依赖；共享 Gateway 装配与正式文档由主代理整合。整体验证依赖三项实现均已完成，不能用某一项定向通过代替最终结果。

- [x] 源码与现行合同对照；三项实施范围已获授权。
- [x] 稳定失败短期退避及必要回归完成。
- [x] 专用 Transport 及连接复用、生命周期验证完成。
- [x] 图片条件缓存、请求合并与协议边界回归完成；三条独立审查问题均已修复并复核。
- [x] 全量非数据库测试、相关 race、vet / build 和系统性差异审查完成。
- [x] 现行文档与计划事实、验证结果同步；索引、相对链接和锚点已核对。

### 本地验证结果（2026-11-03）

- API 全量非数据库测试 `go test ./... -skip 'Integration|PostgreSQL' -count=1 -timeout=180s`、Gateway / DirectPlay / Provider / 账号 / 配额五包 race、`go vet ./...` 与 `go build ./...` 均通过；未执行真实数据库集成。
- 图片装配先由重复请求回归复现未缓存，再验证缓存后的 Token 撤销 / 用户过期仍逐次门控，匿名和受保护表示不能混用。并发 fake 用例中六个相同图片请求只读取一次上游。
- 专用连接池的本地 fake 对照：第二批八个并发请求，标准池新增六个连接，专用池为零；同时验证流式取消、WebSocket、依赖注入和自有资源关闭。这不是生产延迟或首屏提升证明。
- 相同类型化拒绝的连续两次请求，preID 实际读取和失败任务均由两次降为一次；正常源解析和目标查重仍会执行，不把这部分工作说成被省略。
- 独立审查发现并修复强制刷新加入普通读取、内部不可缓存 304 丢失元数据、多值 Header 结构摘要碰撞三条 P2；分别新增红绿回归并重新完成全量 / race。最终系统链路和图片专项复核无未收口 P0 / P1 / P2。
- Python 采集工具 41 项离线测试通过；Go 实际诊断到 Python 过滤 / 摘要的跨语言检查通过，文档审计无必补缺口。Go 默认缓存目录曾因沙箱写权限失败，最终改用任务专属临时缓存重跑通过，没有改变全局 Go 环境。
- 未启动项目服务、请求真实 Emby / 115 / Redis / PostgreSQL，未提交、推送或部署。继续保留本计划承接经授权部署后的命中率、上游建连、图片条件请求、正常播放和真实收益验收；这些事项未被本地 fake 结果替代。

## 落地后文档处理

稳定行为分别提炼到 [system-architecture.md](../../system-architecture.md)、[播放代理合同](../../reference/emby-playback-proxy-contract.md)、[115 Cookie 合同](../../reference/p115-cookie-playback-contract.md)、[115 端到端参考](../../reference/p115-playback-end-to-end-flow.md)及[测试指南](../../runbooks/testing.md)；不写维护者实例标识、观看统计或逐次运维流水。

三项各自满足实现、必要测试、资源与兼容边界验证后标记完成；当三项均收口、现行文档同步、最终审查无未处理 P0 / P1 / P2，且剩余真实验收项已有明确承接时，本计划迁入 `docs/archive/plan/architecture/`。归档前先修正状态、当前事实和验证记录，再按[归档治理规范](../../reference/archive-governance.md)同步入口与直接引用。
