# Emby 115 直连播放网关实现方案

> 状态：进行中（阶段 1 核心闭环已落地并取得真实 302，完整 E2E 与阶段 2 未完成）
> 负责人：Ember
> 更新时间：2026-09-01

## 背景

Ember 已经管理用户生命周期、Emby 账号、套餐分组、设备和播放记录，但视频播放仍经过 `115 -> 挂载/FUSE -> Emby -> 播放器`。远端探测、Range、拖动和高码率视频会把延迟与带宽压力集中到 Emby 主机。

目标链路是在 Ember 完成用户和播放策略校验后，将 115 源文件按 SHA1 秒传到一个专用播放小号，再把兼容的 115 CDN 下载地址通过 HTTP 302 返回给客户端。用户仍按原方式登录 Emby，不感知 115 账号。

Ember 当前没有 115 OpenAPI AppID，因此首期不能按 OpenAPI 授权方案实施。当前决定是独立实现一个原生 Go Cookie Provider，并保留 Provider 边界，待 AppID 获批后再增加 OpenAPI Provider。

当前实现的完整组件图、请求时序、状态机、数据边界和系统性审查结果统一维护在 [115 Cookie 直连播放端到端流程参考](../../reference/p115-playback-end-to-end-flow.md)；本计划只保留阶段目标和未完成项。

Gateway 的通用透明代理、客户端根路径兼容、登录前 bootstrap 和 Emby Web Surface 控制已由 [Ember Gateway 透明代理与 Web 访问控制实现方案](../../archive/plan/architecture/ember-gateway-transparent-proxy-and-web-access.md) 完成并归档；本计划只负责 115 Provider、DirectPlay 决策和正常 Emby 播放回退。

## 目标

1. 新增独立播放网关，作为客户端访问 Emby 的公网入口；Emby 正常代理播放是基线，固定合同中的原始视频流请求尝试 115 直连，未完整成功时透明回退原始 Emby 请求。
2. 首期使用一个管理员源账号和一个管理员播放小号，通过 Cookie/Web API 完成查重、秒传、目标复核和兼容条件成立时的 302。
3. 复用 Ember 用户状态和客户端黑名单，禁止绕过既有用户管理；套餐账号来源、用户自有账号和 Redis 配额由独立后续计划承接。
4. 保持 Emby 登录、媒体信息、图片、字幕、播放进度和停止事件正常工作。
5. 用固定版本合同、fake Provider、fixture、加密向量和数据库集成测试锁定关键链路；自动化测试不调用真实 Emby 或 115。
6. 让 Cookie Provider 可被后续 OpenAPI Provider 替换，不让非官方协议泄漏到业务层。

## 非目标

首版明确不做：

- 本计划不支持普通用户绑定 115 账号，也不增加用户二维码授权、Cookie 抓取或解绑页面；已确认的普通用户 Cookie 绑定与账号路由见 [115 用户自有账号路由与 Redis 配额实现方案](./p115-personal-account-routing-and-redis-quotas.md)。
- 不支持多个源账号、多个播放小号、智能选路、主备切换或账号借用。
- 不支持 Jellyfin、多 Emby Server 或多网盘 Provider 市场。
- 不自动猜测 STRM、OpenList、WebDAV 或挂载路径格式。
- 不改写 HLS、DASH、转码 manifest 或转码分片。
- 不下载完整视频到 Ember 再上传到 115。
- 不在播放小号不可用时改用 source 账号签发直链；115 加速不适用或失败时回退 Emby 正常代理播放。
- 不在本计划实现下一集预热、批量预转存或复杂计费；用户自有 playback 存储已拆入独立计划。
- 不承诺 302 发出后能立即终止已建立的 115 CDN 连接。

## 当前事实

### 已固定合同

- Emby 播放代理协议以 `4.9.3.0` 为证据基线，Gateway 运行兼容范围固定为 `>= 4.9.0.0 && < 4.10.0.0`：
  - `docs/reference/emby-playback-proxy-contract.md`
  - `docs/reference/playback-reporting-api-contract.md`
- 115 当前与未来 Provider 合同：
  - `docs/reference/p115-cookie-playback-contract.md`
  - `docs/reference/p115-direct-play-contract.md`
- 架构与开发规范：
  - `docs/system-architecture.md`
  - `docs/reference/api-development-conventions.md`
  - `docs/reference/testing-strategy.md`
  - `docs/reference/web-information-architecture.md`
  - `docs/reference/web-design-guide.md`

### 当前系统能力

- 用户模型已有 `embyId`、过期时间、激活状态、Emby 禁用状态和套餐分组。
- 套餐 Emby Policy 已有并发、下载、转码和远程访问配置。
- 管理端已有活跃会话、播放历史、设备管理、客户端黑名单和设备操作日志。
- `EMBY_URL` 是 Ember API 访问 Emby 的内部地址；`NEXT_PUBLIC_EMBY_URL` 是控制台展示和用户跳转地址。
- 系统已有基于 `CONFIG_ENCRYPTION_KEY` 的敏感值加密能力，但普通 `settings` 表不适合保存账号 Cookie。
- 已落地 `p115_accounts`、共享 Cookie 加密组件、账号管理 Service、JWT-only 管理 API、管理员 Web 账号页面，以及完整可注入的 `CookieProvider`；其 fake HTTP 合同覆盖 Cookie 登录、上传信息、源路径解析、SHA1 查重、秒传初始化、目标目录复核、下载 URL、受限 Range Hash 和串行删除。Gateway 数据面、统一单二进制、Compose profile、预览镜像、外部 HTTPS 和目标 Infuse/Web 基础验收均已完成。2026-08-22 本地真实检查已通过 source 只读、playback 保留式写入和 preexisting 复用链路。
- 当前已有统一 `cmd/ember`、`api/gateway` 子命令和同镜像双容器 Compose 入口；`emby_access_tokens`、Emby Token 身份核心、认证透明代理/Token 门控、启动期上游身份核对、控制面硬状态撤销、`playback_transfer_tasks`、DirectPlay 传输核心、外部 HTTPS Gateway 和目标 Infuse/Web 基础验收已落地；用户自有账号与 Redis 当前播放租约尚未接入。

### 外部证据与未确认项

- Cookie 协议参考固定为 `p115client` 提交 `608a44396fea08d36131a68beb245be1fe17aa6d`、包版本 `0.0.9.6.4`；它仅是调查和测试向量来源，不是运行时依赖。
- 上传加密参考同一提交内 `p115cipher` `0.0.5.4` 黑盒输出；Go 固定向量、fake HTTP 和 2026-08-22 受控真实上传初始化均已通过。该次检查只证明 Provider 写入、目标复核和有界 Range，不独立证明 Gateway/Infuse；后续组合实证见本节 2026-08-29、2026-08-31 记录。
- `emby-toolkit` `v10.8.63` 只用于理解播放小号的账号选择和失败语义；不得复制其 AGPL 代码。
- `p115client` 固定提交根许可声明为 MIT，但 `p115cipher` 模块许可证和源码声明为 GPLv3；当前按 GPLv3 保守边界处理，不复制或逐行翻译源码、不引入 Python 运行时，只使用临时黑盒输出的兼容向量独立实现 Go 协议层。
- 2026-08-22 本地一次性只读检查已确认两个 Cookie 登录、`uploadinfo`、source 路径/size 解析、playback SHA1 查询、source downurl 和精确 128 KiB Range；source URL 为 `cdnfhnfile.115cdn.net`、`f=1`、并发上限 `2`。playback 未命中同内容，因此 playback 最终下载 URL，以及风控、限流和 Infuse 行为仍保持“未实机确认”。
- 2026-08-24 Infuse `8.5` 已通过 Gateway 完成登录、普通资源 API、PlaybackInfo 和 Emby 扩展名 fallback `206` 播放；Playing/Progress 返回 `204`。
- 2026-08-29 Infuse `8.5.2` 已确认 `Size=0` 不阻断 proof，source 路径映射以 Provider 权威 Size 完成首次保留式转存并返回 `302`，后续多次 `preexisting=true` 复用也返回 `302`。该证据仍不能证明客户端已完整读取 115 CDN 媒体字节。
- 2026-08-31 macOS Infuse `8.5.2` 已确认本地 fallback `206`、115 首次/复用 `302` 实际播放、外挂/内嵌字幕和 Playing/Progress/Stopped `204`。该实机观察仍不替代 115 CDN 完整响应头、HEAD/Range、全文件字节、UA/IP 绑定和长期风控取证。
- Infuse 不设长期固定版本。每次受控验收使用目标平台当时的稳定最新版，并记录平台、精确版本、日期和结果。

## 已确认决策

| 维度 | 首期决策 |
| --- | --- |
| Emby | `4.9.3.0` 协议证据基线；Gateway 接受 `>= 4.9.0.0 && < 4.10.0.0` |
| Infuse | 验收时使用目标平台当前稳定版并记录精确版本，不把版本写成永久合同 |
| 115 认证 | 管理员手工配置 Cookie，默认视为非官方兼容模式 |
| 账号拓扑 | 恰好一个源账号和一个播放小号 |
| 用户侧 | 用户继续使用 Emby，不展示或绑定 115 账号 |
| Provider | Ember 原生 Go 实现；不运行 Python，不依赖 `p115client` |
| playback 目标目录 | 管理员按路径输入或目录选择器操作，后端解析并校验；运行时、秒传、复核和清理只信任 `targetParentId`，禁止只保存路径 |
| 第一阶段文件保留 | playback 专用目录作为持久缓存；只秒传、复核、复用和签发直链，不因 Stopped/会话 TTL 自动调用 `DeleteFile` |
| 失败策略 | 身份/硬状态失败拒绝；合法用户的 115 加速不适用或失败时透明回退 Emby，绝不借 source 账号签发直链 |
| 进程与镜像 | 保持一个 `ember-api` 镜像和一个 `ember` 二进制，通过 `ember api` / `ember gateway` 启动两个独立容器进程；Gateway 服务名固定为 `ember-gateway`，不新增第四个镜像或独立版本线 |
| 凭证 | Cookie 使用 `CONFIG_ENCRYPTION_KEY` 加密，写入后不可回显 |
| OpenAPI | 保留独立 Provider 扩展点，AppID 获批后另行实现 |

## 实施进度

截至 2026-09-01 已完成账号控制面、Cookie Provider 合同验证、DirectPlay 生产编排和 Gateway/Infuse 基础播放闭环：

- 新增 `p115_accounts` 模型、幂等 SQL migration、角色/目标目录检查和启用账号唯一索引。
- 将 ConfigService 历史 AES-GCM 格式下沉到共享 `security/secretbox`，已有 settings 密文保持兼容；115 Cookie 使用用途隔离派生密钥。
- 新增 `services/p115account`，支持安全概要查询、加密创建、Cookie 轮换、显式验证、事务启停和活动账号凭证读取。
- 新增 Provider-neutral 类型与接口，固定秒传状态和下载 Header 模式的 Ember 内部语义。
- 新增 JWT 管理员账号 API：列表、详情、创建、Cookie 替换、验证和启停；Admin API Key 不得管理 115 凭证。
- 根据固定 `p115client` 源码实现 `CookieCredentialValidator`，只读检查登录状态并规范化 Cookie `UID`；fake 合同和 2026-08-22 两个真实 Cookie 均已通过。
- 已用单元测试覆盖密文兼容、Cookie 不回显、输入边界、轮换状态重置、验证状态机、Cookie 并发替换、启用约束、HTTP 错误脱敏和路由注册。
- 已补真实 Gin router、JWT middleware、Service、GORM 和 PostgreSQL 的 API 进程内集成测试；115 校验器使用 fake，不访问真实 115。
- 集成测试覆盖账号创建、列表、详情、验证、启停、Cookie 替换与重新验证，以及未验证账号启用、同角色启用冲突、跨角色 Provider UID 冲突、凭证失效、Provider 故障和 Admin API Key `403`。
- 集成测试确认 Cookie 只以密文落库且不通过 API 回显；每个用例使用独立 `itest_*` schema，完整执行 migration 与 `VerifySchema` 并在结束后清理。
- PostgreSQL 竞态集成测试确认同角色并发启用只允许一个成功，验证期间替换 Cookie 后旧验证结果返回冲突且不能覆盖新凭证状态。
- 新增管理员页面 `/console/p115-accounts` 和侧边栏入口，支持安全摘要、创建、Cookie 替换、显式验证和启停；Cookie 不回填，提交成功或关闭弹窗后立即清空。
- 创建和替换 Cookie 时已从 `UID.ssoent` 自动识别客户端类型并复用 `p115_accounts.app_type`；已知编码覆盖请求值，未知编码才开放人工兜底，识别不调用 115 设备接口。
- 新增前端 API/类型合同和组件交互测试，覆盖 API 路径与 payload、待验证账号启用闸门、创建、验证、启用和 Cookie 替换流程；`npm run test` 与 `npm run build` 已通过。
- 新增 `integrations/p115/p115cipher` 离线 PoC，固定 `k_ec` token/CRC、AES-CBC 请求、LZ4 响应解压、上传 `sig/token` 和排序表单密文；向量不含真实账号信息，不访问 115。
- 新增 `CookieHTTPAdapter.GetUploadInfo` 和 `SearchBySHA1`，用 `httptest` 固定 method、query、Cookie/User-Agent Header、Web 短字段/app2 长字段响应、严格内容匹配和脱敏错误；真实 preexisting 连续两次暴露旧全局 `shasearch` 不适合目标目录查重，改为有 parent 时复用目录作用域 `/files/search` 后真实复跑通过。
- 新增 `CookieHTTPAdapter.InitRapidUpload`，把完整 filename/preID/topupload 固定向量接入 fake POST 端点，并锁定 `status=1/2/7`、未知状态、Range 边界和脱敏失败映射。
- 新增 `CookieHTTPAdapter.FindTargetFile`，固定目标目录搜索短字段和完整身份校验，并用 fake clock 锁定立即查询、500ms 轮询、10s 最终截止查询、取消、歧义和错误不重试。
- 新增 `CookieHTTPAdapter.GetDownloadURL`，固定 Chrome downurl RSA request/response seam、真实客户端 UA、HTTPS 115 域名 allowlist、`t/c/f`、并发上限、过期和脱敏错误映射。
- 新增 `CookieHTTPAdapter.DeleteFile`，固定单文件 `fid` 表单和响应映射，并按 Provider UID 使用进程内共享锁串行删除；跨 UID 可并行，锁等待支持取消。
- 新增具体 `CookieProvider`，组合账号验证与全部数据面 Adapter，并通过编译期断言保证完整实现 Provider-neutral 接口；生产账号控制面已注入该实现。
- 新增 `CookieHTTPAdapter.ResolveFileByPath`，在显式 `rootId` 下逐级分页列举 `/files`，精确匹配目录链和最终文件名，拒绝无效 cid 回退、同名歧义、分页漂移和超大目录；2026-08-29 起不再使用 Emby Size 过滤，唯一 Provider 文件的 Size/SHA1 才是权威身份。
- 新增 `CookieHTTPAdapter.ResolveDirectoryByPath`，支持 `/EmberPlayback` 形式的 playback 路径，逐级只接受唯一目录并返回内部 ID；不创建目录，目录 API/Web 体验仍待实现。
- 新增 `CookieHTTPAdapter.HashFileRange`，在 Provider 内获取源账号签名 URL，只读取最大 `1 MiB` 的指定 Range，严格校验 `206`、`Content-Range`、`Content-Length` 与 HeaderMode，只向业务层返回 SHA1 和读取字节数。
- 新增一次性 `cmd/p115-contract-check`，使用不包含上传和删除的窄接口完成真实只读检查；CI、缺少显式确认值或缺少终端环境输入时拒绝运行，脱敏报告不输出 Cookie、账号标识、路径、pickCode、完整 SHA1 或签名 URL。fake Provider 自动化验证和 2026-08-22 本地真实只读完整运行均已通过。
- 首次真实只读运行在 source download 安全策略处发现 `cdnfhnfile.115cdn.net`；类型化安全证据只输出 reason/scheme/hostname。真实响应、阿里云 OSS DNS 和 115 公司 TLS 证书构成一致证据后，仅加入该精确 hostname，并保留兄弟域名与后缀绕过拒绝测试。
- allowlist 收口后第二次真实运行返回 `outcome=passed`：source 文件 size 为 10,747,391,752 字节，下载 URL 为 `same_user_agent`、并发上限 `2`，精确读取 `0-131071` 共 `131072` 字节并完成 Hash；playback 查重正常未命中，因此没有验证 playback 最终直链。
- 新增 `cmd/p115-transfer-contract-check` 和 fake Provider 编排，覆盖 playback 目录解析、双重查重、preID、单次 challenge、目标复核、playback downurl/Range 与文件保留；命令没有 `DeleteFile` 能力，也不连接 PostgreSQL。
- 修复真实上传响应的 AES 12 字节短尾部与 LZ4 终止语义后，2026-08-22 保留式写入返回 `outcome=passed`：双重查重未命中，`range_challenge → reused` 仅执行一次 challenge，目标约 1,179ms 可见；playback downurl 为 `same_user_agent`、并发上限 `2`，128 KiB Range 与 source SHA1 前缀一致。报告确认 `writePerformed=true`、`created=true`、`retained=true`、`cleanup.attempted=false` 和 `databaseLockValidated=false`。
- 用保留文件再次运行后返回 `outcome=passed`、`writePerformed=false`、`preexisting=true`、`created=false` 和 `challengeCount=0`；没有再次计算 preID、调用上传初始化或目标轮询，只重新签发 playback downurl 并验证 Range，确认第一期重复打开复用语义成立。
- 新增 `playback_transfer_tasks` 模型、幂等 migration、活动内容 partial unique、终态 provenance 和 `lastAccessedAt`；`VerifySchema` 同步校验新表、代表性列和三个索引。
- 新增不暴露 HTTP 入口的 `internal/services/directplay`，按角色加载活动账号，以 `playbackAccountId + SHA1 + size` 获取 PostgreSQL session advisory lock，锁内二次查重后编排 preID、一次 challenge、秒传、目标复核和锁外直链签发；其窄 Provider 接口不包含 `DeleteFile`。
- 专用 PostgreSQL 集成数据库的独立 schema 已验证 migration 可重复执行、两个相同内容并发请求只调用一次 fake `InitRapidUpload`、challenge 将 `attemptCount` 记为 2、普通上传要求落为脱敏失败终态；测试不访问真实 115。
- DirectPlay 已通过窄 `AccountHealthReporter` 回写实际调用账号：完整候选成功后更新 source/playback `lastSucceededAt`，凭证失效进入 `expired + disabled`，临时 Provider 故障进入固定 1 分钟 `cooling_down`，协议错误进入 `error`；冷却期间不请求 115，过期后通过 PostgreSQL 行锁只放行一个半开探测。
- 运行期健康回写同时匹配请求加载时的 Cookie 密文和 `updatedAt`，旧请求不能覆盖 Cookie 替换、显式验证、手工启停或较新的运行期结果；回写使用独立 2 秒上限，失败不改变原 302/fallback 结果。fake/race 与 PostgreSQL 集成测试已覆盖冷却阻断、单探测、成功恢复、凭证停用和旧结果丢弃。
- source 账号新增 `embyPathPrefix/sourceRootId` 一对一运行位置、独立更新接口和管理员表单；`ResolveMediaPath` 已按完整目录边界转换 Emby 路径，拒绝兄弟前缀、空相对路径、`.`/`..`、反斜杠和非规范 root ID。
- 新增 `emby_access_tokens` 模型和幂等 migration，只保存按 `emby-access-token` purpose 派生的 32 字节 HMAC；`VerifySchema` 同步校验代表性列和四个索引。
- 新增无 HTTP 入口的 `internal/services/embytoken`，实现成功认证结果绑定、实时用户资格解析、`lastSeenAt` 限频，以及单 Token、单设备和用户全部登录软撤销；返回值、JSON、日志和错误均不包含 Token 明文或摘要。
- 专用 PostgreSQL 集成数据库已验证 8 路并发认证只生成一条摘要映射、活动摘要不能换绑身份、三种撤销粒度、撤销后重新认证、动态到期，以及用户删除后已撤销审计保留；测试不请求真实 Emby。
- 新增无监听器的 `internal/playbackgateway` 标准 HTTP Handler：认证路由透明代理并旁路写入 Token 映射，固定登录文档中的 public 用户列表/头像进入 bootstrap allowlist，其余请求使用唯一 `X-Emby-Token` 调用 `ResolvePrincipal` 后再转发；认证响应、普通 Header 和未知 JSON 字段不重编码。
- 按 SDK 固定提交实现 `Authorization/X-Emby-Authorization` 的严格 `Emby` 应用头解析，要求 `Client/Device/DeviceId/Version`，拒绝重复/未知字段、非空内嵌 Token、非法 quoted-string 和 Header 歧义；`Client/DeviceId` 只作为非权威映射元数据。
- Gateway fake 测试覆盖认证 `200/401/403/500` 原样返回、应用头与 public bootstrap、旁路写入失败、无效/超大成功响应、Token 缺失/重复/撤销/到期、路由大小写/尾斜杠/escaped path 绕过、上游 transport 错误和日志脱敏；不请求真实 Emby。
- 新增 `integrations/emby.ServerIdentityVerifier`，在监听前调用固定 `/emby/System/Info`；Runtime 只接受四段数字版本 `>= 4.9.0.0 && < 4.10.0.0` 与有界非空 ServerId，重定向、非 JSON、超大响应、状态失败、超时、版本越界和字段异常全部返回不含 URL/API Key/响应体的固定错误。
- 新增 `cmd/ember` 和 `internal/entrypoint`，无参数/`api` 启动 API，`gateway` 启动 Gateway，help/未知参数保持无副作用；外部调用方确认不需要历史兼容后，`cmd/server` 与 `cmd/playback-gateway` 薄包装已删除。
- Gateway production runtime 提供独立 `/health`、HTTP Header/空闲边界和 context graceful shutdown；进程复用 migration/VerifySchema 与 ConfigService，但不初始化 JWT、Bot、cron 或默认管理员。
- 新增不持有 Token/HMAC/runtime ServerId 的 `ControlPlaneRevoker`；设备按用户/设备跨历史 Server 撤销，用户按主体全部撤销，恢复路径同样清理遗留活动映射。
- 设备手工/黑名单退出、用户 toggle/admin edit、Emby 访问开关、绑定前清理、解绑、删除和过期 cron 已按“本地撤销成功后再执行状态或 Emby 副作用”接入；远端失败不回滚本地安全结果。
- Gateway 已精确分类 GET/POST PlaybackInfo、保留 ResolvePrincipal 结果、透明观察成功响应，并在 5 分钟/4096 条有界进程内缓存中保存 mapping/item/mediaSource/playSession 证明与 MediaSource 快照；没有重复请求 Emby。
- PlaybackInfo 请求/响应不符合证明条件时仍透明代理但不缓存；缓存并发、TTL、容量淘汰、Token 隔离和请求/响应字节保持已有 fake 测试。2026-08-29 起响应级合同成立后的唯一 MediaSource 均按运维要求记录完整合法 Path、Size/播放能力和 proof 接受/拒绝原因，不再依赖 proof 写入成功；Token 与完整响应仍保持脱敏。

仍未完成（仅统计本计划边界）：

- 115 CDN 完整响应头、HEAD/Range、全文件字节、UA/IP 绑定和长期风控仍缺受控证据；现有首次/复用 `302` 与 Infuse 实际播放不能替代这些独立合同取证。
- `playback_transfer_tasks` 已落库，但管理员 transfer 查询、手工重试和失败趋势尚未实现。
- 被动运行期健康回写和共享冷却已完成；主动账号健康检查、Bot 告警和自然发生的生产故障恢复观察尚未完成。
- playback 文件仍无限保留；基于 `lastAccessedAt`、无活跃播放和容量水位的 dry-run/串行清理、删除前 provenance 复核与跨副本清理锁尚未实现。`DeleteFile` 当前只有 Adapter 和测试，没有生产业务调用方。
- HEAD/预加载仍会同步进入完整 DirectPlay 查重、秒传和下载 URL 签发；短期直链缓存以及与同一播放 session 的归并尚未实现。
- 两次历史 Token Store error 已补分类诊断，但底层原因仍需后续自然日志复验；这属于观察项，不把旧日志猜测为连接池耗尽。

已拆分、不再计入本计划欠账：

- playback 目录的 Provider 路径解析已完成，当前管理员 API/Web 仍要求手工填写内部 ID；系统/个人 playback 的统一路径输入体验已迁入 [用户自有账号与 Redis 配额计划](./p115-personal-account-routing-and-redis-quotas.md)。
- 普通用户自有 playback 账号、套餐组 `personal|system` 路由、Redis account/user 当前活跃数和小时/每日转存配额也由该独立计划承接。

## 方案设计

### 1. 用户可见行为

#### 1.1 普通用户

- 继续使用现有 Emby 地址、用户名和密码登录播放器。
- 当前系统内置链路不需要用户知道、填写或绑定 115 账号；未来只有 `personal` 套餐组需要用户在控制台绑定自己的 playback 账号。
- 播放符合直连条件的媒体时：
  - 播放小号已有相同文件：直接获取播放小号直链并 302。
  - 播放小号缺文件：等待 SHA1 秒传、目标复核后 302。
  - 小号凭证失效、秒传失败或直链需要客户端无法提供的 Header：透明回退 Emby 正常播放。
- 当前账号不可用或客户端不适合直连时回退 Emby；未来个人 playback 或管理员共享 playback 达到自身最大播放路数、Redis 不可用或用户转存配额已满时同样回退 Emby。用户停用、Emby 访问禁用和本地 Token 撤销仍按安全门控拒绝。

#### 1.2 管理员

- 系统设置新增“直连播放”分组：
  - 网关开关和公开地址。
  - 一个源账号和一个播放小号的 Cookie、客户端类型和脱敏状态。
  - 播放小号目标目录；页面展示路径/目录选择器，不要求管理员手工查找内部 ID。
  - 路径映射和 CDN 域名 allowlist。
  - 账号手工验证和 Cookie 替换。
- 套餐分组后续只增加 `personal|system` playback 账号来源和用户小时/每日转存配额，不增加套餐播放并发；最大播放路数配置在具体 playback 账号上。
- 当前计划只保留秒传任务运维边界；Redis 当前活跃数和用户自有账号页面由独立计划设计，不在数据库中建立直连会话历史。
- 设备管理继续承载客户端黑名单和设备注销，不增加平行风控页面。

#### 1.3 保持不变

- 现有登录、续期、封禁、套餐分组和 Emby Policy 同步保持不变。
- 图片、媒体信息、字幕、Playing、Progress、Stopped 和非视频 API 继续转发。
- 本地媒体、路径未映射媒体和所有不适合 115 加速的媒体继续使用原始 Emby 播放。
- 前端实现必须遵守 Ember 风格，以 `docs/reference/web-design-guide.md` 为设计与交互基线；当前没有偏离规范的特例。

### 2. 目标架构

```text
Infuse / Emby Client
          |
          v
ember-gateway (`ember gateway`)
  |-- 登录、媒体信息、图片、字幕、播放事件 --> Emby 4.9.x
  |
  `-- 原始视频流请求
       |-- 身份/硬状态失败 ----------------------> reject
       `-- Principal 合法
            |-- 证明/资格/115 任一步不满足 ------> Emby 正常代理
            `-- 直连完整成功
                 |-- 播放小号查重/秒传/目标复核
                 |-- 获取兼容下载地址
                 `-- HTTP 302 --> 115 CDN

Ember API / Web
  |-- 账号、路径规则、套餐策略、会话和任务管理
  `-- PostgreSQL：加密凭证、映射、任务、会话和审计
```

边界：

- `services/api`：配置、账号管理、路径规则、策略和管理查询。
- `ember-gateway` 容器：运行 `ember gateway`，承担 Emby 兼容代理、身份解析、可选 115 加速和 302 数据面；正常 Emby 视频代理始终是合法用户的基线。
- PostgreSQL：账号、任务、会话和跨副本互斥的真相源。
- 视频字节：302 后只在客户端与 115 CDN 之间传输；fallback 时从 Emby 经 Gateway 返回客户端。Ember 只允许额外读取秒传 challenge 指定的有界 Range。

### 3. 代码边界

现有 Go Module 已收口为一个服务进程入口：

```text
services/api/cmd/ember/main.go              # 统一生产入口：api / gateway 子命令
services/api/internal/entrypoint/
services/api/internal/playbackgateway/
services/api/internal/services/directplay/
services/api/internal/integrations/p115/
```

职责：

- `cmd/ember`：只把参数交给 `internal/entrypoint` 并返回退出码；无参数默认 `api`，未知子命令 fail-fast。
- `internal/entrypoint`：解析 `api/gateway/help`、初始化日志、为 Gateway 建立信号 context，并分别调用 API/Gateway `RunProcess`；API 在具备 context graceful shutdown 前保持操作系统原有信号行为。
- `internal/playbackgateway`：数据库/配置/HTTP 生命周期装配、Emby 反向代理、路由分类、Header 和 302 输出。
- `internal/services/directplay`：资格判断、媒体解析、查重和秒传；后续账号路由、Redis 活跃租约与转存配额通过窄接口接入，不把 Redis 协议混入 Provider。
- `internal/integrations/p115`：Provider 接口、Cookie HTTP 适配、上传加密和原始 DTO 映射。
- `internal/integrations/emby`：补充固定合同中的播放信息查询，不把网关业务塞进现有 Emby Client。

业务层只依赖 `ValidateCredential`、`GetUploadInfo`、`ResolveFileByPath`、`SearchBySHA1`、`InitRapidUpload`、`GetDownloadURL`、`FindTargetFile`、`HashFileRange` 和 `DeleteFile` 等内部语义。Cookie 端点、原始状态码、签名 URL、Range 源字节和加密算法不能泄漏到 Service。

当前已实现的系统内置链路不使用 Redis；任务幂等和账号级互斥继续使用 PostgreSQL 唯一约束与 advisory lock。后续已确认引入 Redis，但只承载用户/账号当前活跃播放和用户转存配额，不替代账号密文、Token 摘要或 transfer provenance，见 [独立计划](./p115-personal-account-routing-and-redis-quotas.md)。

### 4. 数据与模型

所有 GORM 字段显式指定 `gorm:"column:xxx"`，JSON 使用 camelCase。所有表结构通过 `infrastructure/database/YYYYMMDD_NN_<description>.sql` 幂等迁移落地，不能依赖 `AUTO_MIGRATE`。

数据库时间戳使用 UTC；授权检查、会话过期和临时文件清理统一复用 `CRON_TIMEZONE`，不新增子系统时区。

#### 4.1 `p115_accounts`

首期保存两条管理员账号记录：

- `id`
- `role`：`source`、`playback`
- `alias`
- `auth_mode`：首期固定 `legacy_cookie`
- `provider_user_id`
- `cookie_ciphertext`
- `app_type`
- `user_agent`
- `emby_path_prefix`：仅 source 使用
- `source_root_id`：仅 source 使用
- `target_parent_id`：仅播放小号使用
- `status`：`pending`、`active`、`expired`、`error`、`cooling_down`
- `enabled`
- `last_validated_at`
- `last_succeeded_at`
- `cooldown_until`
- `last_error_code`
- `last_error_message`
- `created_at`
- `updated_at`

约束：

- 数据库对每个角色至多允许一条启用记录；网关就绪要求两个角色各有一条启用记录。
- 两条启用记录验证出的非空 `provider_user_id` 不得相同；停用历史记录可保留同一账号标识。
- Cookie 加密后落库，明文不得离开写入和 Provider 调用边界。
- Cookie 更新使用覆盖语义，查询接口永不返回密文或明文。
- 新建和重新启用 source 要求 `emby_path_prefix + source_root_id` 完整；历史账号不猜默认值，缺失时运行期失败关闭。
- playback 的 source 位置字段必须为空；source 的 `target_parent_id` 必须为空。
- 不预建未使用的 OpenAPI Token 字段；OpenAPI Provider 落地时通过新 migration 增加自己的凭证结构。

#### 4.2 `emby_access_tokens`

建立 Emby AccessToken 到 Ember 用户的映射：

- `id`、`server_id`、`token_hash BYTEA(32)`
- `emby_user_id`、`user_id`
- `device_id`、`client_name`
- `last_seen_at`、`revoked_at`、`revoked_reason`、`revoked_by`
- `created_at`、`updated_at`

只保存使用 `derive(CONFIG_ENCRYPTION_KEY, "emby-access-token")` 计算的 HMAC-SHA256；`server_id + token_hash` 唯一，模型 JSON 隐藏摘要。`device_id/client_name` 只做设备归组和审计，不参与用户身份判断。

服务边界：

- `RecordAuthenticationResult`：只接收固定 `AuthenticationResult` 的 `User.Id/AccessToken/ServerId` 和请求设备元数据，先核对响应 ServerId 等于网关启动期确认的上游 ServerId，再按 `users.emby_id` 找到唯一用户并发安全 upsert；原始响应仍由网关逐字节透明返回。
- `ResolvePrincipal`：从已确认的 Token 载体提取明文，计算 HMAC 后按当前 ServerId 查询未撤销映射，再实时读取用户状态；客户端 `UserId` 永不作为身份输入。
- `RevokeToken`：撤销一条 `server_id + token_hash` 映射。
- `RevokeDevice`：撤销 `server_id + user_id + device_id` 下全部活动映射，使单个设备重新登录。
- `RevokeUserTokens`：撤销用户全部活动映射，用于全部退出、用户停用、Emby 访问禁用、解绑和安全处置。
- `TouchLastSeen`：只有成功认证后且旧值早于 5 分钟窗口才更新，避免播放器请求放大数据库写入。

撤销是软状态：写入 `revoked_at/revoked_reason/revoked_by`，不删除审计记录。到期、套餐拒绝、并发已满和设备策略拒绝只做动态资格检查，不硬撤销；用户停用、`emby_disabled`、`emby_access_disabled`、解绑和删除触发硬撤销。恢复普通状态不自动清除撤销，重新成功登录后才允许建立或重新激活映射。

本地撤销不宣称 Emby Server 原始 Token 已被撤销。目标版本的原生撤销接口完成独立版本化合同前，管理员设备退出只保证 Playback Gateway 拒绝；原始 Emby 端口必须对公网隔离。切换网关时既有 Token 无法从 hash 反推或安全回填，客户端需重新登录一次。

#### 4.3 source 账号运行位置

首期只有一个启用 source 账号，Emby 挂载前缀和 115 root 是该账号的一对一运行属性，直接保存在 `p115_accounts.emby_path_prefix/source_root_id`，不创建独立映射表：

- 新建 source 时两个字段必填；playback 必须为空。
- 历史 source 不猜默认值、不自动回填；运行期加载和重新启用要求显式补齐。
- `emby_path_prefix` 使用完整目录边界匹配，不做 `path.Clean`；`source_root_id` 使用规范十进制 ID。
- 如果未来出现“一个 source 账号对应多个本地前缀/root”的真实需求，再新增映射表，不在首期预建多对多结构。

#### 4.4 `playback_media_cache`

> 状态：首期推迟，不创建表。

Gateway 已代理客户端 PlaybackInfo，当前先用有界 5 分钟进程内证明同时保存 MediaSource `Path/Container`、Emby Size 观察值和播放能力；Emby Size 不影响 proof 或路径解析。不重复请求 Emby，也不把短期快照落库。只有后续真实负载证明跨请求长期复用源文件身份有收益时，再实现持久缓存。

#### 4.5 `playback_transfer_tasks`

记录播放小号秒传：

- 源账号、播放账号、SHA1、size、文件名和目标目录。
- 当前生产状态：`pending`、`initializing`、`challenging`、`verifying`、`succeeded`、`failed`；目标查重发生在任务创建前，账号冷却继续由 `p115_accounts` 承担，不在任务表预建空状态。
- 目标 fileId、pickCode、尝试次数、脱敏错误、起止时间和 `last_accessed_at`。

活动任务唯一键为 `playback_account_id + sha1 + size`。

#### 4.6 Redis 当前播放租约（独立计划）

> 状态：需求边界已确认，尚未实现；不创建 `direct_play_sessions` 表。

当前切片只维护 `mappingId + ItemId + MediaSourceId + PlaySessionId` 的进程内 PlaybackInfo 短期证明。后续 Redis 会把准入占用和真实活跃拆开：只有合格 `GET` 能在 302 前建立 `30s reservation` 并占用账号名额，`HEAD` 无既有租约时直接 fallback；成功的 Playing/Progress 才把 reservation 晋级为 `active`，暂停使用更长 TTL 且继续占用，Stopped 成功转发后释放，无后续事件时自然过期。

Redis 不保存完整 115 直链、Cookie、Token、完整 SHA1 或播放历史；详细 Key、原子更新和故障回退见 [115 用户自有账号路由与 Redis 配额实现方案](./p115-personal-account-routing-and-redis-quotas.md)。

#### 4.7 套餐账号来源与转存配额（独立计划）

不建立 `plan_group_direct_play_policies` 表，也不按套餐限制播放并发。后续直接扩展 `plan_groups`：

- `p115PlaybackMode=personal|system`，所有套餐组默认 `personal`，`system` 由管理员主动设置。
- `p115TransferHourlyLimit` 和 `p115TransferDailyLimit` 由管理员配置，Redis 按用户记录用量。

`system` 只是套餐选择管理员共享 playback 的路由值，不是 `p115_accounts` 的账号类型或 scope。最大播放路数属于具体 playback 账号：个人账号由本人设置，管理员共享 playback 由管理员设置。账号达到上限、Redis 不可用、个人账号未绑定或转存配额已满时统一 `fallback_to_emby`；只有 Token、本地撤销、身份错配和用户硬状态继续 `fail_closed`。

### 5. API 与边界

列表接口统一返回 `data`，字段使用 camelCase。账号 Cookie 只存在于创建或更新请求，不出现在任何响应。

#### 5.1 管理员 115 账号

- `GET /api/v1/admin/p115-accounts`
- `POST /api/v1/admin/p115-accounts`
- `GET /api/v1/admin/p115-accounts/:id`
- `PUT /api/v1/admin/p115-accounts/:id/cookie`
- `POST /api/v1/admin/p115-accounts/:id/validate`
- `PUT /api/v1/admin/p115-accounts/:id/enabled`

创建接口接收角色、别名、Cookie、可选 appType 兜底、User-Agent 和播放小号目标目录；Cookie 替换接口同样接受可选 appType 兜底，已知 `UID.ssoent` 始终以自动识别结果为准。启停使用独立接口。返回只包含脱敏账号标识、状态、最后验证时间和脱敏错误。

source 创建同时接收 `embyPathPrefix/sourceRootId`；已有 source 使用 `PUT /api/v1/admin/p115-accounts/:id/source-location` 更新。位置更新不改变 Cookie 或验证状态。

首期不提供 `/api/v1/user/p115/*`，也不创建授权会话或二维码轮询 API。

#### 5.2 路径和运维接口

- `GET /api/v1/admin/direct-play/transfers`
- `POST /api/v1/admin/direct-play/transfers/:id/retry`

套餐组账号来源、普通用户个人账号、Redis 当前用量接口统一由 [独立计划](./p115-personal-account-routing-and-redis-quotas.md) 定义；本计划不预建未实现路由，也不提供数据库会话列表。

Token 撤销已复用现有设备/用户管理入口，没有创建第二套设备页面：设备强制退出和黑名单批处理调用控制面设备撤销，用户硬状态入口调用用户全部撤销；单 Token 安全处置仍由后续会话/审计详情触发。现有 HTTP DTO 保持兼容，不新增平行路由。

手工重试必须复用任务幂等键，不能绕过账号冷却和用户转存配额。账号来源、最大播放路数或配额更新只影响新请求，不能承诺撤销已签发链接。

#### 5.3 播放网关公开接口

网关不新增客户端专用播放协议：

- 普通 Emby 请求透明转发。
- `AuthenticateByName` 成功响应旁路建立 Token 哈希映射。
- 固定合同中的原始视频流路径进入直连编排。
- 字幕和 Playing、Progress、Stopped 继续转发并旁路更新会话。

### 6. 关键流程

#### 6.1 管理员配置账号

1. 管理员通过 HTTPS 管理端录入源账号或播放小号 Cookie。
2. API 在事务中加密 Cookie 并保存账号角色和兼容参数。
3. 管理员显式触发只读验证；Provider 返回脱敏账号标识。
4. API 校验两个角色不是同一 Provider 账号。
5. 验证成功后账号进入 `active`；失败时保留脱敏错误，Cookie 不回显。
6. Cookie 失效后由管理员覆盖更新并重新验证，不自动扫码或续期。

#### 6.2 Emby 登录与 Token 映射

1. 客户端通过网关调用 `AuthenticateByName`。
2. 网关转发给支持范围内的 Emby 4.9 Server。
3. 成功后读取 `User.Id`、`AccessToken` 和 `ServerId`。
4. 按 `users.emby_id` 映射唯一 Ember 用户，使用 purpose 隔离的 HMAC-SHA256 计算摘要，并按 `serverId + tokenHash` upsert 设备元数据和最近访问时间。
5. 用户不存在、EmbyID 错配或处于硬禁用状态时不建立可用映射；用户仅到期时可以保留身份映射，但后续直连动态拒绝，续期后无需因到期本身强制重登。
6. 原始认证响应不修改地返回客户端；旁路持久化失败只记录脱敏错误，Token 保持未映射，不能把 Emby 成功响应改写为 Ember 自造响应。
7. 首期只提取固定合同确认的 `X-Emby-Token`；Infuse 的其他 Token 载体必须实机确认后再加入。

#### 6.3 播放小号已有文件

1. PlaybackInfo 透明转发；只有当前 Emby Server 成功接受相同 Token 后，才记录 Token 映射、ItemId、MediaSourceId 和 PlaySessionId 的短期授权证明。
2. 原始视频流请求到达后先校验 Token、用户和本地硬状态；失败时 `reject`，不能回退绕过门控。
3. Principal 合法后再检查证明、加速资格、客户端和并发；首期只有固定 `GET/HEAD` 视频路径同时带唯一 `MediaSourceId`、唯一 `PlaySessionId`、精确 `Static=true` 且容器匹配时尝试 115，任何不满足均 `fallback` 到原始 Emby 请求。
4. 从进程内 PlaybackInfo 快照取得 Path，按 source 账号的 `embyPathPrefix/sourceRootId` 调用 `ResolveMediaPath`；Emby Size 仅留在观察日志。
5. 播放小号按 SHA1 查询，并再次校验 size 和非目录类型。
6. 命中后更新对应任务的 `lastAccessedAt`，使用播放小号 pickCode 和真实客户端 UA 获取下载地址。
7. 校验过期时间、Header 要求和域名 allowlist，兼容时返回空体 302；任一步失败都透明 fallback Emby。

#### 6.4 播放小号缺文件

1. 创建或复用 `playbackAccountId + SHA1 + size` 任务。
2. 获取数据库互斥后再次查重。
3. 使用 playback Cookie 和该账号明确配置的 `targetParentId` 调用上传初始化，禁止默认写入根目录。
4. `status=2`：进入目标文件复核。
5. `status=7`：使用源账号 `HashFileRange` 在 Provider 内读取指定 Range，只取得 SHA1 后再次初始化；源直链、Cookie 和字节内容不进入 Service。
6. `status=1`：需要普通上传，明确失败；禁止完整文件中转。
7. 其他状态：映射脱敏错误并失败。
8. 只有明确复用且在目标目录确认 parent、SHA1、size、非目录一致后，才记录任务 provenance 和初始 `lastAccessedAt`；预存命中文件不归任务所有。
9. 使用 playback Cookie、复核后的 pickCode 和真实播放器 UA 获取最终直链并返回 302；source 直链永远不能返回给客户端。
10. 第一阶段文件保留在 playback 专用目录；Stopped、会话过期和用户短期重复打开都不触发 `DeleteFile`。外部手工删除后，下一次实时查重未命中时允许重新秒传。

稳定的 Provider 调用顺序、单次 `status=7` challenge、最终直链来源和清理边界统一以 [115 Cookie 播放兼容合同 §8.1](../../reference/p115-cookie-playback-contract.md#81-完整秒传到-playback-并播放流程) 为准，本计划不复制协议细节。

#### 6.5 直链兼容与 302

1. 直链按播放账号、pickCode 和真实客户端 UA 隔离缓存。
2. `t` 决定最大缓存时间，并预留安全窗口。
3. `f=0`、`f=1` 等模式只有通过合同测试和受控 Infuse 验证后才允许。
4. `f=3` 或其他要求播放器额外携带 Cookie 的链接，首期按不兼容处理。
5. 不兼容时不暴露 Cookie、不改用 source 账号，记录固定 fallback 原因并透明转发 Emby。

#### 6.6 会话与并发

本计划只固定 Emby 事件透明转发和当前管理员共享账号播放合同；后续 Redis 语义由独立计划承接：

1. 按 `PlaySessionId + Ember 用户 + 设备` 归并 GET、HEAD、预加载和重连，同一会话不能重复计数；HEAD 只能复用既有租约，不能创建新租约。
2. 合格 GET 在 302 前建立短期 `reservation`，同时进入 playback 账号与用户的 `leases` 占用索引；个人账号按配置值与正数套餐 `SimultaneousStreamLimit` 的较小值、管理员共享账号按自身配置值，以 `reservation + active + paused` 总占用数门控。
3. 只有成功转发给 Emby 的 Playing 或 Progress 才能把既有 reservation 晋级为 `active`；`IsPaused=true` 切换为 `paused`，继续占用并使用更长 TTL。
4. API 展示的真实活跃数只统计 `active + paused`，不把 reservation 表述为正在播放；用户活跃数只用于展示和归因，不建立第二套用户并发门控。
5. Stopped 只有成功转发给 Emby 后才释放占用/活跃索引；客户端未上报停止时由对应 Redis TTL 自然收口。
6. 不新增 Gateway 用户级总并发门控，用户索引只用于展示和归因。115 `302` 后视频字节不经过 Emby，Emby `SimultaneousStreamLimit` 能否限制该分流播放仍未证实；套餐模板只约束个人账号配置值和运行时有效上限，详细规则以独立计划为准。

#### 6.7 Cookie 失效与冷却

当前状态：被动运行期健康回写与共享冷却已实现；主动定时健康检查和告警仍属于阶段 2。

1. Provider 识别凭证失效后，将账号标记为 `expired + disabled`，阻止新任务和新直链。
2. 临时 Provider 不可用按实际 source/playback 账号进入固定 1 分钟 `cooling_down`，记录截止时间和脱敏原因；协议错误进入 `error`，不伪装成凭证失效。
3. 冷却期间不读取 Cookie、不重复探测外部接口；到期后以 PostgreSQL 行锁和条件更新发放一个 1 分钟半开探测租约，跨 Gateway 副本也只有一个请求放行。
4. 完整 DirectPlay 候选签发并完成必要持久化后，source/playback 都恢复 `active`、更新 `lastSucceededAt` 并清空冷却/错误；运行期回写失败不得改写原始 302/fallback 结果。
5. 所有回写匹配加载时的 Cookie 密文和 `updatedAt`；管理员更新 Cookie 后仍必须重新验证，旧播放请求不能覆盖新凭证的 `pending` 状态。

### 7. 失败与安全边界

- Emby Token 无法映射：拒绝，不信任请求参数里的 `UserId`。
- Emby Token 已本地撤销：所有受保护的网关请求拒绝，不继续转发给 Emby 作为回退。
- Token 只有历史登录映射但没有近期成功 PlaybackInfo：不生成 302，合法请求透明 fallback Emby。
- 用户过期、停用或访问禁用：按现有用户资格门控拒绝，不生成新直链，也不能用 fallback 绕过 Ember 状态。
- Principal 合法但当前系统源账号或系统播放小号未配置、未验证、过期或冷却：记录固定原因并 fallback Emby。
- 路径未命中、MediaSource 不支持 Direct Play 或视频参数不完整：fallback Emby。
- SHA1 搜索候选的 size 或类型不符：视为加速未命中；后续无法完成秒传/复核时 fallback Emby。
- challenge 越界、Range 获取失败或初始化要求普通上传：任务失败并 fallback Emby。
- HTTP `2xx` 但没有明确复用：不能判定成功。
- 直链域名不在 allowlist、已过期或要求额外 Cookie：拒绝 302 并 fallback Emby。
- HLS、DASH、转码 manifest、转码分片和其他非原始直连视频请求：直接透明转发 Emby。
- 播放小号失败：禁止使用源账号向最终客户端签发直链。
- 用户播放中被封禁：阻止新请求；已建立 CDN 连接可能持续到链接过期。
- 单设备退出撤销该设备全部活动映射，用户全部退出撤销该用户全部映射；已签发的 CDN URL 只能通过停止重签和等待过期收口。
- 多副本文件秒传：数据库唯一约束和 advisory lock 保证现有 transfer 任务幂等；后续播放活跃数与用户转存配额使用 Redis 原子语义，不能混成数据库会话表。
- 未进入固定合同的 Emby/115 行为保持“未证实”，不能用一次偶然成功替代合同。

每个视频请求只直接打印一条最终决策日志：`decision=redirect|fallback|reject`。人工结论放在行首，使用稳定 `code/result/statusCode` 配合中文 `message` 明示“115直链成功”“115直链失败，Emby回退成功/失败”或“播放请求已拒绝”；直链成功额外记录 `targetState=created|reused`。`stage/reasonCode` 统一使用 `docs/reference/emby-playback-proxy-contract.md` 的固定枚举；日志可以记录 requestId、method、userId、mappingId、deviceId、clientName、itemId、mediaSourceId、playSessionId、stage、reasonCode、taskId、preexisting、statusCode、upstreamStatus、proxyErrorCode、durationMs，以及进入 DirectPlay 后的完整 `mediaPath/embyPathPrefix/sourceRootId/mappedRelativePath`。Provider 失败只记录固定 `providerOperation`，账号加载失败只记录 `accountRole=source|playback`；未知诊断和无意义空字段不打印。禁止新建播放决策日志表或 migration；禁止记录 Emby AccessToken、115 Cookie、上传加密材料、完整 SHA1、完整下载直链、`Set-Cookie`、PlaybackInfo 原文、Provider 原始错误或 Emby 代理原始错误。

### 8. 配置与部署

当前部署拓扑：

- `services/api/Dockerfile` 只构建一个 `ember`；镜像使用 `ENTRYPOINT ["./ember"]` 和默认 `CMD ["api"]`。
- `ember-api` 与 `ember-gateway` 复用完全相同的 `EMBER_API_IMAGE`；前者运行 `ember api`，后者以 Compose `command: ["gateway"]` 覆盖默认子命令。
- API 固定使用容器内 `8080`，Gateway 固定监听容器内 `8081`；Compose 只允许通过 `PLAYBACK_GATEWAY_PORT` 调整 Gateway 的宿主机回环映射端口。
- `ember-gateway` 使用显式 `gateway` profile；当前 Compose 默认 Tag 仍可能指向不支持子命令的旧发行镜像，普通 `docker compose up` 必须保持原 API userspace，启用 Gateway 前先切换到新镜像或本地构建。
- 不新增 Gateway GHCR 镜像、构建工作流、Tag 或独立版本号；API 与 Gateway 必须以同一镜像 digest 部署。
- 公网反向代理只暴露播放网关；原始 Emby 端口仅对内网或运维 allowlist 开放。
- `EMBY_URL` 继续指向内部 Emby；`NEXT_PUBLIC_EMBY_URL` 改为网关公开地址。

数据库运行期配置建议：

- `PLAYBACK_GATEWAY_ENABLED`
- `PLAYBACK_GATEWAY_PUBLIC_URL`
- `P115_COOKIE_COMPAT_ENABLED`
- 直链域名 allowlist

网关切换说明：历史 Emby Token 没有可安全迁移的明文，切换后客户端需重新登录一次建立映射；Quick Connect、PIN、Emby Connect 和未确认的 query token 不做兼容猜测。
- 媒体缓存、任务和账号冷却 TTL

部署期环境变量：

- `CONFIG_ENCRYPTION_KEY`

Cookie 不进入环境变量。Cookie 以密文保存；播放小号目标目录、appType 和 User-Agent 作为普通账号元数据管理。具体配置键在实现时同步 `docs/reference/configuration-reference.md`，明确生效方式和重启要求。

个人账号路由落地时新增 Redis 与 `REDIS_URL`，但 Redis 只承载当前播放租约和转存配额；Redis 不可用时 Gateway 仍提供 Emby 透明代理并停止签发新的 115 直链。该部署变更不属于当前已实现事实，见 [独立计划](./p115-personal-account-routing-and-redis-quotas.md)。

## 分阶段落地

### 阶段 0：合同与离线 PoC

- 固定 Emby 4.9 协议基线与运行兼容范围、Cookie Provider 和 OpenAPI Provider 三份合同。
- 从固定 `p115client`/`p115cipher` 提取无敏感信息 fixture 和加密向量。
- 建立 fake Emby、fake 115、状态映射和直链兼容测试。
- 在用户明确授权后，以一次性命令验证一个源账号、一个播放小号和测试文件。

完成条件：所有 method、path、请求字段、响应映射、加密向量和未确认项均有固定证据；不能靠猜测进入实现。

当前进度：账号控制面、完整 `CookieProvider`、固定协议向量、fake HTTP Adapter、真实只读、保留式写入和 preexisting 复跑均已完成；方法、字段、加密、Range、目标复核、playback 直链和文件保留已有固定证据。阶段 0 已完成。删除只保留为第二阶段能力。

### 阶段 1：最小闭环

- 一个满足 `>= 4.9.0.0 && < 4.10.0.0` 的 Emby Server。
- 一个管理员源账号和一个管理员播放小号，手工录入 Cookie。
- 一种明确路径映射。
- 目标平台当前稳定版 Infuse Direct Play。
- Token 映射、目标查重、秒传、目标复核、直链检查和 302。
- 单 Token、单设备和用户全部登录撤销；设备强制退出后允许重新登录，用户硬禁用后新登录也不能恢复直连。
- playback 文件作为持久缓存保留；重复播放命中后跳过秒传并刷新 `lastAccessedAt`，第一阶段不启用自动清理。
- 基础账号冷却、日志和 transfer 任务持久化；用户自有账号、Redis 当前播放和转存配额不作为本计划阶段 1 的完成条件。

完成条件：小号已有文件和缺失秒传两条加速链路均通过；重复播放复用同一 playback 文件且不重复秒传；Stopped/会话过期不删除文件；302 分支的视频字节不经过 Ember/Emby；合法用户在任一加速失败时仍可 fallback Emby 正常播放；身份和硬状态能阻止未授权播放；任何失败都不借 source 账号播放。

当前进度：`emby_access_tokens`、purpose 隔离 HMAC、并发安全映射、三种 Gateway 撤销、控制面硬状态联动、认证透明代理与 Token 门控、固定 SDK 的应用头解析/public bootstrap、启动期 Emby 身份核对、单 `ember` 二进制、`api/gateway` 子命令、同镜像 `ember-api/ember-gateway` Compose、外部 HTTPS、进程内 PlaybackInfo 当前授权证明与 MediaSource 快照、固定视频路由消费证明、生产 DirectPlay 装配、空体 302、权威 Emby fallback、单条脱敏决策日志、`playback_transfer_tasks`、session advisory lock、source 账号位置、账号按角色加载、direct play 传输编排、被动运行期健康回写和 1 分钟共享冷却已完成；对应 fake 单元/race 测试和 PostgreSQL 集成测试通过。实机已获得本地 fallback `206`、首次/复用 Gateway `302`、字幕和 Playing/Progress/Stopped `204`；原始 Emby 隔离由部署管理员确认。完整 CDN 响应合同与阶段 2 运维能力仍待完成；用户自有账号、套餐来源和 Redis 配额已拆入独立计划，不阻塞本系统内置链路的阶段 1 收口。

### 阶段 2：运营与稳定性

- 管理端 transfer 任务查询、手工重试与失败趋势。
- 主动账号健康检查、Bot 告警、失败趋势，以及基于 `lastAccessedAt`、无活跃会话和容量水位的串行清理任务；被动播放结果回写和共享冷却不再属于本阶段剩余项。
- transfer 秒传已由 PostgreSQL advisory lock 支持多 Gateway；仍需为清理任务补跨副本互斥，并在后续 Redis 租约可用后接入“无活跃播放”条件。
- 本地媒体 fallback 已有 `206` 实机证据；仍需补 115 CDN 完整合同和自然发生的云端故障恢复证据。
- 收口 HEAD/预加载重复进入完整 DirectPlay 的 Provider 开销：HEAD 无既有 Redis 租约时直接 fallback，有既有租约时按账号、目标文件和真实 UA 复用有界短期直链缓存；GET reservation 与缓存统一按 sessionFingerprint 归并。

### 阶段 3：OpenAPI 与账号池

- AppID 获批后新增独立 OpenAPI Provider 和 Token 生命周期，不修改 Cookie Provider 语义。
- 评估多源账号、多播放小号、粘性选路、并发和熔断。
- 普通用户 Cookie 账号已由独立现行计划承接；二维码登录、下一集预热和其他 Provider 仍需独立计划。

## 已拆分的后续范围

管理员共享/个人 playback 的统一目录路径输入、`targetParentPath` 展示快照、个人 Cookie 控制面、账号最大播放路数、套餐账号来源和 Redis 配额均由 [115 用户自有账号路由与 Redis 配额实现方案](./p115-personal-account-routing-and-redis-quotas.md) 承接。本计划只保留当前管理员全局账号运行事实，不再复制该方案的 API、Web、migration 和验证清单。

Emby 只维护 115 STRM、外部系统另行维护同相对路径本地硬链接的场景，由 [STRM 本地媒体回退播放实现方案](./strm-local-media-fallback.md) 承接。该方案只在本计划的 115 DirectPlay fallback 出口增加“本地精确路径 → Emby/CloudDrive2”的选择，不覆盖成功 `302`，也不把 MoviePilot 上传纳入 Ember。

## 影响范围

- API：新增播放网关、direct play Service、Cookie Provider、管理员全局账号/路径和任务接口。
- Web：当前账号控制面使用独立的管理员 115 账号页面；用户账号、套餐组来源和 Redis 用量页面由独立计划承接。
- Bot：阶段 2 可增加账号失效和连续失败告警。
- 数据库：账号表 source 位置字段、秒传任务表和 Token 摘要映射表已落地；本计划不再规划数据库播放会话或套餐直连策略表。
- 配置/部署：当前新增网关进程、公开入口和原始 Emby 网络隔离；后续 Redis 由独立计划处理。
- 文档：实现时同步系统架构、配置、数据模型、API 目录、Web 信息架构、部署和测试 runbook。

## 验证方式

### 自动化验证

- `cd services/api && go test ./...`
- `cd services/api && go vet ./...`
- `cd services/api && go build ./...`
- `cd services/web && npm run test`
- `cd services/web && npm run build`

阶段 0 已完成能力的验证：

- `cd services/api && go test -count=1 ./...`
- `cd services/api && go test -count=1 ./internal/integrations/p115`
- `cd services/api && go test -count=1 ./internal/integrations/p115/p115cipher`
- `cd services/api && go vet ./...`
- `cd services/api && go build ./...`
- 设置 `EMBER_INTEGRATION_DATABASE_URL` 后执行 `go test -count=1 -run '^TestIntegrationP115Account' ./internal/app`
- 设置 `EMBER_INTEGRATION_DATABASE_URL` 后执行完整 `go test -count=1 ./internal/app`
- `cd services/web && npm run test`（当前 175 个测试通过、3 个按既有条件跳过）
- `cd services/web && npm run build`

上述集成测试使用真实 PostgreSQL 和应用内真实 HTTP 路由，但用 fake `CredentialValidator` 隔离 115；它证明账号控制面状态和数据库约束能够闭环，不证明目标 115 账号、Cookie/Web API 或播放链路真实可用。

2026-08-22 Token 身份核心验证：

- 设置专用 `EMBER_INTEGRATION_DATABASE_URL` 后执行 `go test -count=1 ./...`，`embytoken` 与 `directplay` PostgreSQL 集成测试实际运行并通过。
- `go test -race -count=1 ./internal/security/tokenhash ./internal/services/embytoken ./internal/services/directplay` 通过。
- `go vet ./...`、`go build ./...` 与 `git diff --check` 通过。
- 全部自动化使用固定输入或专用 PostgreSQL schema，没有启动服务、请求真实 Emby，或把 fake/数据库证据表述为网关和 Infuse 实机可用。

2026-08-22 Gateway 认证代理与门控核心验证：

- `go test -count=1 ./internal/playbackgateway` 通过，fake Emby 覆盖认证透明性、标准应用头、public 用户/头像 bootstrap、旁路失败、Token 门控、路由分类和脱敏 transport 错误。
- 同一 Gateway fake/race 测试覆盖 GET/POST PlaybackInfo 请求与响应字节保持、Principal/UserId 绑定、多 MediaSource 快照、复合键隔离、5 分钟 TTL、4096 条容量、并发访问、无效请求/响应不缓存、完整 Path quoted 日志和 Token 脱敏。
- `go test -race -count=1 ./internal/playbackgateway ./internal/services/embytoken` 通过。
- 设置专用 `EMBER_INTEGRATION_DATABASE_URL` 后执行 `go test -count=1 ./...`，新 Gateway 包和既有 PostgreSQL 集成测试实际执行并通过；`go vet ./...` 与 `go build ./...` 通过。
- 自动化没有创建真实 Listener，也没有请求真实 Emby/115 或执行 Infuse 验收；固定 SDK 的 public bootstrap 与应用头合同已确认并由 fake 锁定，目标 Server 版本和 Infuse 实际请求顺序仍保持未证实。
- `ServerIdentityVerifier` 与 production runtime fake 测试覆盖精确版本/ServerId、Header 与响应边界、配置缺失、`/health`、无真实 Listener 的 graceful shutdown 和监听错误脱敏。
- 统一入口测试覆盖无参数默认 API、显式 `api/gateway`、help、未知/额外参数、日志初始化和进程失败脱敏；`go build ./cmd/ember` 通过，Compose 已确认两个服务复用同一镜像 Tag。2026-08-23 `pre_release` 的 API GitHub Actions 已实际构建并推送单二进制预览镜像。
- 控制面撤销单元/PostgreSQL/Gin 集成测试覆盖设备跨 Server 但不跨用户、手工与黑名单本地优先、toggle/admin edit/恢复、Emby 访问禁用、绑定前清理、解绑、删除审计保留和过期 cron 失败关闭；所有 Emby 副作用使用 fake。

2026-09-01 账号运行期健康与共享冷却验证：

- `go test -count=1 ./...` 通过；专用 PostgreSQL schema 用例实际运行，覆盖临时 Provider 故障进入冷却、冷却期间不触达 Provider、过期冷却并发只放行一个探测、成功恢复、凭证失效停用和旧 Cookie 结果丢弃。
- `go test -race -count=1 ./internal/services/p115account ./internal/services/directplay`、`go vet ./...` 和 `go build ./...` 通过。
- 全部 Provider/DirectPlay 自动化使用 fake，不启动服务、不请求真实 Emby/115；这证明状态机、数据库条件更新和并发租约，不证明生产 115 限流错误码或 1 分钟冷却时长最优。

测试分层：

- Emby 合同：固定认证、PlaybackInfo、视频流、字幕和播放事件 fixture。
- Token 身份：HMAC 固定向量、认证响应透明、并发 upsert、ServerId/EmbyID 错配、明文不落库、`lastSeenAt` 限频和 revoked 查询。
- Token 撤销：单 Token、单设备、用户全部撤销，以及停用/访问禁用/解绑的硬撤销联动；到期和套餐拒绝保持动态判断。
- 115 合同：Cookie 脱敏、源路径逐级解析、SHA1 查重、`status=2`、`status=7`、受限 Range Hash、`status=1`、下载 Header 和错误映射。
- 保留式秒传检查器：双重查重、preID、零/一次 challenge、重复 challenge 失败关闭、目标复核、playback UA Range、`retained=true`、`cleanup.attempted=false` 和 `databaseLockValidated=false`。
- 加密合同：请求加密、响应解密、LZ4、签名和 token 固定向量。
- Service/Gateway：身份安全 fail-closed；路径、账号、Provider、冷却和直链不兼容时 fallback-to-Emby。
- PostgreSQL：迁移、角色唯一性、Cookie 密文、Token 唯一性/撤销审计、任务幂等和 advisory lock。
- 运行期健康：成功、凭证失效、Provider 临时不可用和协议错误映射；冷却期间不触达 Provider、过期冷却单探测、成功恢复、旧 Cookie/旧状态结果丢弃，以及回写失败不改变 302/fallback。
- 网关：认证响应透明、受保护请求 Token 门控、近期 PlaybackInfo 证明、Header、GET/HEAD、安全 reject、115 redirect 和原始请求 Emby fallback。
- Web：当前管理员全局账号只写交互、脱敏状态和路径；套餐账号来源、个人账号和 Redis 当前用量由独立计划覆盖。

涉及账号状态、用户资格、任务状态机、DTO 或 Provider 适配时按 TDD 推进。所有外部依赖必须 fake，禁止测试真实 Emby、115 或 Infuse。

### MVP 验收场景

1. 播放小号已有相同 SHA1 + size 文件，跳过秒传并返回小号直链 302。
2. 小号缺文件，`status=2` 复用后目标复核成功并返回 302。
3. `status=7` 只读取源账号指定 Range，再次初始化并复核目标文件。
4. `status=1` 明确失败，绝不下载和上传完整视频。
5. 并发 `HEAD`、预加载和 Range 只创建一个秒传任务。
6. 重复播放命中同一 playback 文件，跳过秒传、刷新最后访问时间并签发新临时直链；Stopped 和会话 TTL 不调用删除。
7. 下载链接通过过期时间、UA、Header 要求和域名 allowlist 校验。
8. `f=3` 或需要额外 Cookie 的链接明确失败，不泄露凭证。
9. Playing、Progress、Stopped 仍由 Emby 接收，播放进度正常。
10. 用户硬状态和已撤销设备阻止新播放；当前账号或客户端条件不满足时 fallback Emby。后续套餐来源、账号并发和配额失败语义由独立计划测试。
11. source 或 playback 账号失效、限流、秒传和直链失败均不得借 source 账号播放；合法用户必须透明 fallback Emby 并记录固定原因。
12. Cookie、Emby AccessToken 明文、完整下载链接和完整 Provider 响应不出现在日志、API 或数据库；Token 表只保存 purpose 隔离的 HMAC 摘要。
13. 同一 Token 并发登录只产生一条映射；数据库只含 32 字节 HMAC，不含 AccessToken 明文。
14. 单设备撤销只影响目标设备，用户全部撤销影响所有设备；硬禁用后重新请求仍拒绝，普通到期续期后不要求因到期本身重新登录。
15. 未映射、已撤销、ServerId/EmbyID 错配和缺少近期成功 PlaybackInfo 的请求都不能获得 302。
16. 网关切换后历史 Token 要求重新登录，原始 Emby 公网入口不可绕过本地撤销。
17. 每个视频请求只打印一条 `redirect/fallback/reject` 决策日志；行首明确显示中文结果、稳定 code/result/statusCode 和首次/复用状态，失败时只补固定 Provider 步骤或账号角色。日志不落表，进入 DirectPlay 后包含完整原始/映射路径，但不包含 Token、Cookie、SHA1、115 URL 或上游原文。

### 受控真实验证

真实验证只在用户明确授权后执行，并且：

- 使用测试账号和测试文件，以一次性命令运行，不启动项目服务或后台进程。
- 先做只读账号、文件和路径验证，再做单文件秒传、playback 下载地址、Range 和保留验证。
- 写入验证必须使用 playback 专用 `targetParentId`，并严格按 Cookie 合同 §8.1 保存和复核 provenance；检查器不持有 `DeleteFile` 能力，报告必须显示 `retained=true` 与 `cleanup.attempted=false`，source 和 playback 文件均不自动删除。
- 验证 `status=2`、`status=7`、目标可见延迟，以及 `t`、`c`、`f`、UA、Cookie、IP 约束。
- 使用目标平台当时的稳定版 Infuse，记录平台、精确版本、日期、`HEAD`/Range/302 行为和结果。
- 记录脱敏请求字段、响应语义、耗时和实际数据路径。
- 完成后保留 playback 文件继续验证已有文件快速路径和 Infuse 重复播放；全部验证结束后如需删除，由管理员手工处理。替换或撤销临时 Cookie。

## 落地后文档处理

主体落地后同步：

- `docs/system-architecture.md`：网关进程、控制面/数据面和关键数据流。
- `docs/reference/configuration-reference.md`：稳定配置、来源、加密和生效方式。
- `docs/reference/data-model-reference.md`：管理员全局账号、映射和任务表。
- `docs/reference/api-endpoint-catalog.md`：管理员全局账号、路径和运维 API。
- `docs/reference/web-information-architecture.md`：管理员全局账号和播放分析页面职责。
- 部署与测试 runbook：公网入口、原始 Emby 隔离、Cookie 轮换和故障排查。

阶段 1 系统内置链路和阶段 2 运维能力完成测试与现行文档收口后，将本计划移入 `docs/archive/plan/architecture/`。用户自有账号、Redis 配额、OpenAPI 和多账号池由各自独立计划承接，不阻碍本计划按既定边界归档。
