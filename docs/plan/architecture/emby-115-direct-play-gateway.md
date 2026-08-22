# Emby 115 直连播放网关实现方案

> 状态：进行中
> 负责人：Ember
> 更新时间：2026-08-21

## 背景

Ember 已经管理用户生命周期、Emby 账号、套餐分组、设备和播放记录，但视频播放仍经过 `115 -> 挂载/FUSE -> Emby -> 播放器`。远端探测、Range、拖动和高码率视频会把延迟与带宽压力集中到 Emby 主机。

目标链路是在 Ember 完成用户和播放策略校验后，将 115 源文件按 SHA1 秒传到一个专用播放小号，再把兼容的 115 CDN 下载地址通过 HTTP 302 返回给客户端。用户仍按原方式登录 Emby，不感知 115 账号。

Ember 当前没有 115 OpenAPI AppID，因此首期不能按 OpenAPI 授权方案实施。当前决定是独立实现一个原生 Go Cookie Provider，并保留 Provider 边界，待 AppID 获批后再增加 OpenAPI Provider。

## 目标

1. 新增独立播放网关，作为客户端访问 Emby 的公网入口；普通 Emby 请求透明代理，固定合同中的原始视频流请求进入直连编排。
2. 首期使用一个管理员源账号和一个管理员播放小号，通过 Cookie/Web API 完成查重、秒传、目标复核和兼容条件成立时的 302。
3. 复用 Ember 用户状态、套餐分组、客户端黑名单和并发策略，禁止绕过既有用户管理。
4. 保持 Emby 登录、媒体信息、图片、字幕、播放进度和停止事件正常工作。
5. 用固定版本合同、fake Provider、fixture、加密向量和数据库集成测试锁定关键链路；自动化测试不调用真实 Emby 或 115。
6. 让 Cookie Provider 可被后续 OpenAPI Provider 替换，不让非官方协议泄漏到业务层。

## 非目标

首版明确不做：

- 不支持普通用户绑定 115 账号，也不增加用户二维码授权、Cookie 抓取或解绑页面。
- 不支持多个源账号、多个播放小号、智能选路、主备切换或账号借用。
- 不支持 Jellyfin、多 Emby Server 或多网盘 Provider 市场。
- 不自动猜测 STRM、OpenList、WebDAV 或挂载路径格式。
- 不改写 HLS、DASH、转码 manifest 或转码分片。
- 不下载完整视频到 Ember 再上传到 115。
- 不在播放小号不可用时改用源账号签发直链，也不静默回退 Emby 视频中转。
- 不在首版实现下一集预热、批量预转存、复杂计费或用户自有存储。
- 不承诺 302 发出后能立即终止已建立的 115 CDN 连接。

## 当前事实

### 已固定合同

- Emby Server 固定基线为 `4.9.3.0`：
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
- 已落地 `p115_accounts`、共享 Cookie 加密组件、账号管理 Service、JWT-only 管理 API、管理员 Web 账号页面，以及完整可注入的 `CookieProvider`；其 fake HTTP 合同覆盖 Cookie 登录、上传信息、源路径解析、SHA1 查重、秒传初始化、目标目录复核、下载 URL、受限 Range Hash 和串行删除；尚未实现播放网关。2026-08-22 本地真实检查已通过 source 只读、playback 保留式写入和 preexisting 复用链路。
- 当前仍没有播放数据面进程、Emby AccessToken 到 Ember 用户的映射、秒传任务或直连会话模型。

### 外部证据与未确认项

- Cookie 协议参考固定为 `p115client` 提交 `608a44396fea08d36131a68beb245be1fe17aa6d`、包版本 `0.0.9.6.4`；它仅是调查和测试向量来源，不是运行时依赖。
- 上传加密参考同一提交内 `p115cipher` `0.0.5.4` 黑盒输出；Go 固定向量和 fake HTTP 上传初始化已通过，尚未请求真实 HTTP 端点。
- `emby-toolkit` `v10.8.63` 只用于理解播放小号的账号选择和失败语义；不得复制其 AGPL 代码。
- `p115client` 固定提交根许可声明为 MIT，但 `p115cipher` 模块许可证和源码声明为 GPLv3；当前按 GPLv3 保守边界处理，不复制或逐行翻译源码、不引入 Python 运行时，只使用临时黑盒输出的兼容向量独立实现 Go 协议层。
- 2026-08-22 本地一次性只读检查已确认两个 Cookie 登录、`uploadinfo`、source 路径/size 解析、playback SHA1 查询、source downurl 和精确 128 KiB Range；source URL 为 `cdnfhnfile.115cdn.net`、`f=1`、并发上限 `2`。playback 未命中同内容，因此 playback 最终下载 URL，以及风控、限流和 Infuse 行为仍保持“未实机确认”。
- Infuse 不设长期固定版本。每次受控验收使用目标平台当时的稳定最新版，并记录平台、精确版本、日期和结果。

## 已确认决策

| 维度 | 首期决策 |
| --- | --- |
| Emby | 固定 `4.9.3.0` 合同 |
| Infuse | 验收时使用目标平台当前稳定版并记录精确版本，不把版本写成永久合同 |
| 115 认证 | 管理员手工配置 Cookie，默认视为非官方兼容模式 |
| 账号拓扑 | 恰好一个源账号和一个播放小号 |
| 用户侧 | 用户继续使用 Emby，不展示或绑定 115 账号 |
| Provider | Ember 原生 Go 实现；不运行 Python，不依赖 `p115client` |
| playback 目标目录 | 管理员按路径输入或目录选择器操作，后端解析并校验；运行时、秒传、复核和清理只信任 `targetParentId`，禁止只保存路径 |
| 第一阶段文件保留 | playback 专用目录作为持久缓存；只秒传、复核、复用和签发直链，不因 Stopped/会话 TTL 自动调用 `DeleteFile` |
| 失败策略 | 播放小号或直链不兼容时明确返回 `503`，不使用源账号播放 |
| 凭证 | Cookie 使用 `CONFIG_ENCRYPTION_KEY` 加密，写入后不可回显 |
| OpenAPI | 保留独立 Provider 扩展点，AppID 获批后另行实现 |

## 实施进度

截至 2026-08-22 已完成账号控制面和 Cookie Provider 离线 PoC：

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
- 新增前端 API/类型合同和组件交互测试，覆盖 API 路径与 payload、待验证账号启用闸门、创建、验证、启用和 Cookie 替换流程；`npm run test` 与 `npm run build` 已通过。
- 新增 `integrations/p115/p115cipher` 离线 PoC，固定 `k_ec` token/CRC、AES-CBC 请求、LZ4 响应解压、上传 `sig/token` 和排序表单密文；向量不含真实账号信息，不访问 115。
- 新增 `CookieHTTPAdapter.GetUploadInfo` 和 `SearchBySHA1`，用 `httptest` 固定 method、query、Cookie/User-Agent Header、Web 短字段/app2 长字段响应、严格内容匹配和脱敏错误；真实 preexisting 连续两次暴露旧全局 `shasearch` 不适合目标目录查重，改为有 parent 时复用目录作用域 `/files/search` 后真实复跑通过。
- 新增 `CookieHTTPAdapter.InitRapidUpload`，把完整 filename/preID/topupload 固定向量接入 fake POST 端点，并锁定 `status=1/2/7`、未知状态、Range 边界和脱敏失败映射。
- 新增 `CookieHTTPAdapter.FindTargetFile`，固定目标目录搜索短字段和完整身份校验，并用 fake clock 锁定立即查询、500ms 轮询、10s 最终截止查询、取消、歧义和错误不重试。
- 新增 `CookieHTTPAdapter.GetDownloadURL`，固定 Chrome downurl RSA request/response seam、真实客户端 UA、HTTPS 115 域名 allowlist、`t/c/f`、并发上限、过期和脱敏错误映射。
- 新增 `CookieHTTPAdapter.DeleteFile`，固定单文件 `fid` 表单和响应映射，并按 Provider UID 使用进程内共享锁串行删除；跨 UID 可并行，锁等待支持取消。
- 新增具体 `CookieProvider`，组合账号验证与全部数据面 Adapter，并通过编译期断言保证完整实现 Provider-neutral 接口；生产账号控制面已注入该实现。
- 新增 `CookieHTTPAdapter.ResolveFileByPath`，在显式 `rootId` 下逐级分页列举 `/files`，精确匹配目录和最终文件名/size，拒绝无效 cid 回退、重名歧义、分页漂移和超大目录。
- 新增 `CookieHTTPAdapter.ResolveDirectoryByPath`，支持 `/EmberPlayback` 形式的 playback 路径，逐级只接受唯一目录并返回内部 ID；不创建目录，目录 API/Web 体验仍待实现。
- 新增 `CookieHTTPAdapter.HashFileRange`，在 Provider 内获取源账号签名 URL，只读取最大 `1 MiB` 的指定 Range，严格校验 `206`、`Content-Range`、`Content-Length` 与 HeaderMode，只向业务层返回 SHA1 和读取字节数。
- 新增一次性 `cmd/p115-contract-check`，使用不包含上传和删除的窄接口完成真实只读检查；CI、缺少显式确认值或缺少终端环境输入时拒绝运行，脱敏报告不输出 Cookie、账号标识、路径、pickCode、完整 SHA1 或签名 URL。fake Provider 自动化验证和 2026-08-22 本地真实只读完整运行均已通过。
- 首次真实只读运行在 source download 安全策略处发现 `cdnfhnfile.115cdn.net`；类型化安全证据只输出 reason/scheme/hostname。真实响应、阿里云 OSS DNS 和 115 公司 TLS 证书构成一致证据后，仅加入该精确 hostname，并保留兄弟域名与后缀绕过拒绝测试。
- allowlist 收口后第二次真实运行返回 `outcome=passed`：source 文件 size 为 10,747,391,752 字节，下载 URL 为 `same_user_agent`、并发上限 `2`，精确读取 `0-131071` 共 `131072` 字节并完成 Hash；playback 查重正常未命中，因此没有验证 playback 最终直链。
- 新增 `cmd/p115-transfer-contract-check` 和 fake Provider 编排，覆盖 playback 目录解析、双重查重、preID、单次 challenge、目标复核、playback downurl/Range 与文件保留；命令没有 `DeleteFile` 能力，也不连接 PostgreSQL。
- 修复真实上传响应的 AES 12 字节短尾部与 LZ4 终止语义后，2026-08-22 保留式写入返回 `outcome=passed`：双重查重未命中，`range_challenge → reused` 仅执行一次 challenge，目标约 1,179ms 可见；playback downurl 为 `same_user_agent`、并发上限 `2`，128 KiB Range 与 source SHA1 前缀一致。报告确认 `writePerformed=true`、`created=true`、`retained=true`、`cleanup.attempted=false` 和 `databaseLockValidated=false`。
- 用保留文件再次运行后返回 `outcome=passed`、`writePerformed=false`、`preexisting=true`、`created=false` 和 `challengeCount=0`；没有再次计算 preID、调用上传初始化或目标轮询，只重新签发 playback downurl 并验证 Range，确认第一期重复打开复用语义成立。

仍未完成：

- playback 目录的 Provider 路径解析已完成，但管理员 API/Web 仍要求手工填写内部 ID；后续按“路径交互、ID 真相源”完成友好配置，见本文“后续 TODO：playback 目标目录友好配置”。
- 播放网关、按角色加载运行期账号、路径映射持久化、秒传任务、任务所有权、直连会话和运营能力；Cookie Provider 离线合同本身已收口。自动清理和跨副本清理锁明确推迟到第二阶段。
- 生产数据库唯一任务/advisory lock，以及真实 Emby / Infuse 验证；本地一次写入成功不能证明长期风控、配额或 `hz-sb` 出口行为。

## 方案设计

### 1. 用户可见行为

#### 1.1 普通用户

- 继续使用现有 Emby 地址、用户名和密码登录播放器。
- 不需要知道、填写或绑定 115 账号。
- 播放符合直连条件的媒体时：
  - 播放小号已有相同文件：直接获取播放小号直链并 302。
  - 播放小号缺文件：等待 SHA1 秒传、目标复核后 302。
  - 小号凭证失效、秒传失败或直链需要客户端无法提供的 Header：返回明确失败。
- 用户过期、停用、Emby 访问禁用、客户端黑名单或并发超限时，不生成新直链。

#### 1.2 管理员

- 系统设置新增“直连播放”分组：
  - 网关开关和公开地址。
  - 一个源账号和一个播放小号的 Cookie、客户端类型和脱敏状态。
  - 播放小号目标目录；页面展示路径/目录选择器，不要求管理员手工查找内部 ID。
  - 路径映射和 CDN 域名 allowlist。
  - 账号手工验证和 Cookie 替换。
- 套餐分组增加直连开关、网关并发、下载、本地媒体回退和云端失败策略。
- 播放分析增加直连会话和秒传任务视图，展示阶段、耗时和脱敏错误。
- 设备管理继续承载客户端黑名单和设备注销，不增加平行风控页面。

#### 1.3 保持不变

- 现有登录、续期、封禁、套餐分组和 Emby Policy 同步保持不变。
- 图片、媒体信息、字幕、Playing、Progress、Stopped 和非视频 API 继续转发。
- 本地媒体只按显式路径规则决定是否保留原始 Emby 播放。
- 前端实现必须遵守 Ember 风格，以 `docs/reference/web-design-guide.md` 为设计与交互基线；当前没有偏离规范的特例。

### 2. 目标架构

```text
Infuse / Emby Client
          |
          v
ember-playback-gateway
  |-- 登录、媒体信息、图片、字幕、播放事件 --> Emby 4.9.3.0
  |
  `-- 原始视频流请求
       |-- Emby Token -> Ember 用户
       |-- 用户 / 套餐 / 设备 / 并发策略
       |-- ItemId + MediaSourceId -> 源文件身份
       |-- 播放小号按 SHA1 + size 查重
       |-- 缺失时源账号 Range -> 播放小号秒传
       |-- 播放小号目标复核
       |-- 获取兼容下载地址
       `-- HTTP 302
                |
                v
            115 CDN

Ember API / Web
  |-- 账号、路径规则、套餐策略、会话和任务管理
  `-- PostgreSQL：加密凭证、映射、任务、会话和审计
```

边界：

- `services/api`：配置、账号管理、路径规则、策略和管理查询。
- `ember-playback-gateway`：Emby 兼容代理、身份解析、播放策略和 302 数据面。
- PostgreSQL：账号、任务、会话和跨副本互斥的真相源。
- 视频字节：302 后只在客户端与 115 CDN 之间传输；Ember 只允许读取秒传 challenge 指定的有界 Range。

### 3. 代码边界

在现有 Go Module 内新增独立二进制：

```text
services/api/cmd/playback-gateway/main.go
services/api/internal/playbackgateway/
services/api/internal/services/directplay/
services/api/internal/integrations/p115/
```

职责：

- `cmd/playback-gateway`：配置、数据库、依赖和 HTTP Server 装配。
- `internal/playbackgateway`：Emby 反向代理、路由分类、Header 和 302 输出。
- `internal/services/directplay`：资格判断、媒体解析、查重、秒传、并发和会话状态机。
- `internal/integrations/p115`：Provider 接口、Cookie HTTP 适配、上传加密和原始 DTO 映射。
- `internal/integrations/emby`：补充固定合同中的播放信息查询，不把网关业务塞进现有 Emby Client。

业务层只依赖 `ValidateCredential`、`GetUploadInfo`、`ResolveFileByPath`、`SearchBySHA1`、`InitRapidUpload`、`GetDownloadURL`、`FindTargetFile`、`HashFileRange` 和 `DeleteFile` 等内部语义。Cookie 端点、原始状态码、签名 URL、Range 源字节和加密算法不能泄漏到 Service。

首版不新增 Redis。任务幂等和账号级互斥使用 PostgreSQL 唯一约束与 advisory lock；真实负载证明不足时再另行提案。

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
- 不预建未使用的 OpenAPI Token 字段；OpenAPI Provider 落地时通过新 migration 增加自己的凭证结构。

#### 4.2 `emby_access_tokens`

建立 Emby AccessToken 到 Ember 用户的映射：

- `id`、`server_id`、`token_hash`
- `emby_user_id`、`user_id`
- `device_id`、`client_name`
- `last_seen_at`、`revoked_at`
- `created_at`、`updated_at`

只保存 Token 哈希；`server_id + token_hash` 唯一。用户停用、访问禁用或解绑时可批量撤销。

#### 4.3 `playback_path_mappings`

字段至少包括：

- `id`、`name`、`source_prefix`
- `provider`、`source_account_id`、`source_root_id`
- `local_origin_allowed`、`priority`、`enabled`
- `created_at`、`updated_at`

首版只支持一种明确路径格式，使用最长前缀和优先级匹配，禁止运行时猜测多种格式。

#### 4.4 `playback_media_cache`

缓存 `emby_server_id + emby_item_id + media_source_id` 到源账号 `fileId`、`pickCode`、SHA1、size 和路径的映射。缓存命中仍需检查过期时间；生成源下载地址失败时失效并重解析一次。

#### 4.5 `playback_transfer_tasks`

记录播放小号秒传：

- 源账号、播放账号、SHA1、size、文件名和目标目录。
- 状态：`pending`、`checking_target`、`initializing`、`challenging`、`verifying`、`succeeded`、`failed`、`cooling_down`。
- 目标 fileId、pickCode、尝试次数、脱敏错误、起止时间和 `last_accessed_at`。

活动任务唯一键为 `playback_account_id + sha1 + size`。

#### 4.6 `direct_play_sessions`

记录用户、ItemId、MediaSourceId、PlaySessionId、设备、客户端、源账号、播放账号、任务和状态。状态为 `requested`、`resolving`、`transferring`、`redirect_issued`、`playing`、`stopped`、`expired`、`failed`。

不保存完整 115 直链；IP 如需持久化必须哈希或脱敏。

#### 4.7 `plan_group_direct_play_policies`

直连策略与 Emby Policy 分表维护：`enabled`、`simultaneous_stream_limit`、`allow_download`、`allow_local_origin_fallback`、`cloud_failure_mode`。首版云端失败默认并固定为 `fail_closed`。

### 5. API 与边界

列表接口统一返回 `data`，字段使用 camelCase。账号 Cookie 只存在于创建或更新请求，不出现在任何响应。

#### 5.1 管理员 115 账号

- `GET /api/v1/admin/p115-accounts`
- `POST /api/v1/admin/p115-accounts`
- `GET /api/v1/admin/p115-accounts/:id`
- `PUT /api/v1/admin/p115-accounts/:id/cookie`
- `POST /api/v1/admin/p115-accounts/:id/validate`
- `PUT /api/v1/admin/p115-accounts/:id/enabled`

创建接口接收角色、别名、Cookie、appType、User-Agent 和播放小号目标目录；Cookie 替换与启停使用独立接口。返回只包含脱敏账号标识、状态、最后验证时间和脱敏错误。

首期不提供 `/api/v1/user/p115/*`，也不创建授权会话或二维码轮询 API。

#### 5.2 路径、策略和运维接口

- `GET|POST /api/v1/admin/direct-play/path-mappings`
- `PUT|DELETE /api/v1/admin/direct-play/path-mappings/:id`
- `GET|PUT /api/v1/admin/plan-groups/:key/direct-play-policy`
- `GET /api/v1/admin/direct-play/sessions`
- `GET /api/v1/admin/direct-play/transfers`
- `POST /api/v1/admin/direct-play/transfers/:id/retry`
- `POST /api/v1/admin/direct-play/media-cache/:itemId/refresh`

手工重试必须复用任务幂等键，不能绕过账号冷却和并发限制。策略更新只影响新请求，不能承诺撤销已签发链接。

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
2. 网关转发给 Emby 4.9.3.0。
3. 成功后读取 `User.Id`、`AccessToken` 和 `ServerId`。
4. 只保存 AccessToken 哈希，并按 `users.emby_id` 映射 Ember 用户。
5. 原始认证响应不修改地返回客户端。

#### 6.3 播放小号已有文件

1. PlaybackInfo 透明转发并记录 ItemId、MediaSourceId 和 PlaySessionId。
2. 原始视频流请求到达后，校验 Token、用户、套餐、黑名单和并发。
3. 从缓存读取源文件身份，或把 Emby `Path + Size` 经显式路径映射转换为 `rootId + relativePath + size`，调用源账号 `ResolveFileByPath` 得到 fileId、pickCode、SHA1 和 size。
4. 播放小号按 SHA1 查询，并再次校验 size 和非目录类型。
5. 命中后更新对应任务/缓存的 `lastAccessedAt`，使用播放小号 pickCode 和真实客户端 UA 获取下载地址。
6. 校验过期时间、Header 要求和域名 allowlist，兼容时返回 302。

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
5. 不兼容时返回明确 `503`，不暴露 Cookie，不改用源账号，不回退视频中转。

#### 6.6 会话与并发

1. 按 PlaySessionId、用户和设备归并 `HEAD`、Range、预加载和重连。
2. `redirect_issued` 后等待 Playing/Progress 进入 `playing`。
3. Progress 更新最后活动时间，Stopped 收口为 `stopped`。
4. 客户端未上报停止时，由复用 `CRON_TIMEZONE` 的 TTL 任务收口为 `expired`。
5. 套餐并发按活跃网关会话执行，不能只读取 Emby Session 数量。

#### 6.7 Cookie 失效与冷却

1. Provider 识别凭证失效后，将账号标记为 `expired`，阻止新任务和新直链。
2. 限流或风控按账号进入共享 `cooling_down`，记录截止时间和脱敏原因。
3. 冷却期间不为每个播放请求重复探测外部接口。
4. 管理员更新 Cookie 后必须重新验证，成功才恢复 `active`。

### 7. 失败与安全边界

- Emby Token 无法映射：拒绝，不信任请求参数里的 `UserId`。
- 用户过期、停用或访问禁用：不生成新直链或新秒传任务。
- 源账号或播放小号未配置、未验证、过期或冷却：返回明确 `503`。
- 路径未命中：本地媒体只按显式规则回退；未知云端路径失败关闭。
- SHA1 搜索候选的 size 或类型不符：视为未命中。
- challenge 越界、Range 获取失败或初始化要求普通上传：任务失败。
- HTTP `2xx` 但没有明确复用：不能判定成功。
- 直链域名不在 allowlist、已过期或要求额外 Cookie：拒绝 302。
- 客户端请求云端转码：明确失败，不让 Emby 静默中转视频。
- 播放小号失败：禁止使用源账号向最终客户端签发直链。
- 用户播放中被封禁：阻止新请求；已建立 CDN 连接可能持续到链接过期。
- 多副本并发：数据库唯一约束和 advisory lock 保证任务幂等。
- 未进入固定合同的 Emby/115 行为保持“未证实”，不能用一次偶然成功替代合同。

日志可以记录 userId、itemId、playSessionId、账号 ID、SHA1 前缀、文件大小、阶段耗时和脱敏错误码。禁止记录 Emby AccessToken、115 Cookie、上传加密材料、完整 SHA1 与账号组合、完整下载直链、`Set-Cookie` 或完整 Provider 响应。

### 8. 配置与部署

部署拓扑：

- 新增 `ember-playback-gateway` 构建 target 和容器。
- 公网反向代理只暴露播放网关；原始 Emby 端口仅对内网或运维 allowlist 开放。
- `EMBY_URL` 继续指向内部 Emby；`NEXT_PUBLIC_EMBY_URL` 改为网关公开地址。

数据库运行期配置建议：

- `PLAYBACK_GATEWAY_ENABLED`
- `PLAYBACK_GATEWAY_PUBLIC_URL`
- `P115_COOKIE_COMPAT_ENABLED`
- 直链域名 allowlist
- 媒体缓存、会话、任务和账号冷却 TTL

部署期环境变量：

- `PLAYBACK_GATEWAY_LISTEN_ADDR`
- `CONFIG_ENCRYPTION_KEY`

Cookie 不进入环境变量。Cookie 以密文保存；播放小号目标目录、appType 和 User-Agent 作为普通账号元数据管理。具体配置键在实现时同步 `docs/reference/configuration-reference.md`，明确生效方式和重启要求。

## 分阶段落地

### 阶段 0：合同与离线 PoC

- 固定 Emby 4.9.3.0、Cookie Provider 和 OpenAPI Provider 三份合同。
- 从固定 `p115client`/`p115cipher` 提取无敏感信息 fixture 和加密向量。
- 建立 fake Emby、fake 115、状态映射和直链兼容测试。
- 在用户明确授权后，以一次性命令验证一个源账号、一个播放小号和测试文件。

完成条件：所有 method、path、请求字段、响应映射、加密向量和未确认项均有固定证据；不能靠猜测进入实现。

当前进度：账号控制面、完整 `CookieProvider` 组合、Cookie 登录状态合同、PostgreSQL 竞态测试、上传/RSA 固定向量，以及上传信息、源路径解析、查重、秒传初始化、目标复核、下载 URL、受限 Range Hash 和串行删除 fake HTTP Adapter 已完成；本地真实只读检查已通过账号、上传信息、源解析、查重、source downurl 和 128 KiB Range。2026-08-22 受控写入在补齐 AES 短尾部和 LZ4 终止语义后返回 `outcome=passed`，已覆盖一次 Range challenge、秒传复用、目标复核、playback 最终直链、128 KiB Range 和文件保留；随后目录作用域 preexisting 复跑通过，确认不再次上传。数据库锁与 Infuse 验证尚未完成；删除只保留为第二阶段能力，因此阶段 0 仍为进行中。

### 阶段 1：最小闭环

- 一个 Emby 4.9.3.0 Server。
- 一个管理员源账号和一个管理员播放小号，手工录入 Cookie。
- 一种明确路径映射。
- 目标平台当前稳定版 Infuse Direct Play。
- Token 映射、目标查重、秒传、目标复核、直链检查和 302。
- playback 文件作为持久缓存保留；重复播放命中后跳过秒传并刷新 `lastAccessedAt`，第一阶段不启用自动清理。
- 基础会话、并发、冷却、日志和管理员查询。

完成条件：小号已有文件和缺失秒传两条链路均通过；重复播放复用同一 playback 文件且不重复秒传；Stopped/会话过期不删除文件；视频字节不经过 Ember/Emby；用户状态和策略能阻止新播放；任何失败都不借源账号播放。

### 阶段 2：运营与稳定性

- 管理端完整账号、路径、策略、会话和任务管理。
- 账号健康检查、Bot 告警、失败趋势，以及基于 `lastAccessedAt`、无活跃会话和容量水位的串行清理任务。
- 多网关副本和跨副本互斥。
- 经过验证的本地媒体回退与云端失败策略。

### 阶段 3：OpenAPI 与账号池

- AppID 获批后新增独立 OpenAPI Provider 和 Token 生命周期，不修改 Cookie Provider 语义。
- 评估多源账号、多播放小号、粘性选路、并发和熔断。
- 普通用户自有账号、二维码登录、下一集预热和其他 Provider 均需独立计划。

## 后续 TODO：playback 目标目录友好配置

> 状态：待实现
>
> 负责人：Ember
>
> 更新时间：2026-08-22

### 当前事实与决策

- 当前模型、SQL migration、API 和管理员页面都只使用 `targetParentId`；创建 playback 账号时，页面直接要求管理员填写“目标目录 ID”。Provider `ResolveDirectoryByPath` 与 fake 合同已实现，可直接复用到后续解析接口。
- 首期 115 账号仍是管理员控制面，普通 Ember 用户不会看到或配置 115 目录；“路径友好”是后台可用性改进，不改变用户侧边界。
- 目录路径可能因重命名、移动、同名目录和特殊字符而漂移，不能作为秒传、目标复核或删除的真相源。
- 最终决策固定为“交互使用目录路径/选择器，运行使用目录 ID”：`targetParentId` 继续作为持久化和运行时权威字段，路径只负责输入与展示。
- 前端实现必须遵守 Ember 风格，设计与交互基线以 [Web 设计规范](../../reference/web-design-guide.md) 为准；当前没有偏离规范的特例。

### 第一阶段：路径输入并解析为现有 ID

目标是不改变现有数据库 schema 和 `POST /api/v1/admin/p115-accounts` 的 `targetParentId` 合同：

1. 创建 playback 账号的表单把主字段从“目标目录 ID”改为“目标目录”，提供根目录相对路径输入和明确的“解析目录”次操作；解析成功前禁止提交。
2. 新增管理员 JWT-only 只读接口 `POST /api/v1/admin/p115-directory-resolutions`，请求接收 write-only Cookie、User-Agent 和目录路径，返回唯一的规范化目录：

   ```json
   {
     "targetParentId": "123456789",
     "targetParentPath": "/EmberPlayback"
   }
   ```

3. Resolver 只允许查询现有目录，不自动创建、移动或重命名目录；Cookie 不进入日志、错误、响应或持久化，Admin API Key 返回 `403`。
4. 后端必须确认结果存在、是目录、属于当前 Cookie 账号且唯一；无结果、同名歧义、字段非法、无效 cid 回退、Provider 拒绝和网络错误全部失败关闭。
5. 前端把解析得到的 `targetParentId` 保存在弹窗内的隐藏状态，提交账号创建时继续发送现有 `targetParentId`；Cookie 和解析结果在提交成功或关闭弹窗后一起清空。
6. 账号卡片主要展示本次解析的规范化路径，目录 ID 只作为次要诊断信息；页面刷新后没有持久化路径时允许回退显示 ID，不为展示路径增加运行时 115 请求。
7. 现有直接提交 `targetParentId` 的 API 客户端保持兼容，不能因 Web 体验改动破坏 userspace。

第一阶段暂不做目录树浏览器。若后续需要目录树选择，必须通过后端只读目录 API 使用同一安全边界，前端不能直接访问 115，也不能把 Cookie 保存到 Store 或浏览器持久化。

### 第二阶段：可选路径快照

如果真实使用证明账号卡片长期展示路径有价值，再增加可选 `target_parent_path`：

- 模型使用 `TargetParentPath`，JSON 为 `targetParentPath`，GORM 显式指定 `gorm:"column:target_parent_path"`。
- 同步提供幂等 SQL migration；路径只是规范化展示快照，`target_parent_id` 仍是唯一运行时真相源。
- 目录重命名或移动后，旧路径快照允许暂时过期；只能通过显式只读刷新更新，禁止运行时因路径变化自动改写 ID。
- 删除目录或 ID 失效时阻止新秒传并返回明确错误，不能退回根目录、按旧路径猜测新目录或自动创建替代目录。

### 影响范围

- API：新增目录解析 DTO、handler 和 Cookie Provider 目录解析边界；handler 只做绑定、Service 调用和错误映射。
- Web：修改 `P115AccountsView.vue` 创建弹窗、集中 API 类型和 mock 交互测试；优先复用 `EmberFormDialog` 与既有表单控件。
- 数据库：第一阶段无变更；第二阶段只有确认需要路径快照时才增加模型字段与 SQL migration。
- 安全：Cookie 继续 write-only；解析接口不记录请求体、目录完整响应或 Provider 原始错误。
- 文档：实现时同步系统架构、数据模型、API 端点目录、Web 信息架构和本计划进度。

### 验证与完成条件

- Go TDD：路径规范化、目录/文件区分、无结果、同名歧义、cid 回退、Provider 错误脱敏，以及 Admin API Key `403`。
- Web Vitest：playback 表单只展示目录路径交互、解析前禁止提交、解析成功后提交隐藏 ID、失败提示、关闭/成功后 Cookie 与解析状态清空。
- 兼容性：既有 `targetParentId` 创建请求继续通过；source 账号仍拒绝任何目标目录字段。
- 第一阶段完成条件：管理员无需手工获取 ID 即可绑定唯一目录，数据库仍只以 `targetParentId` 驱动秒传/复核/清理，且 `npm run test`、`npm run build`、`go test ./...`、`go vet ./...`、`go build ./...` 通过。
- 第二阶段只有在路径快照 migration、真实 PostgreSQL 集成测试和文档同步全部完成后才算落地；未实施时保持“可选后续”，不能预建空字段。

## 影响范围

- API：新增播放网关、direct play Service、Cookie Provider、账号/路径/策略/会话/任务接口。
- Web：账号控制面使用独立的管理员 115 账号页面；后续直连策略再触达系统设置、套餐分组和播放分析，首期不改用户账号中心。
- Bot：阶段 2 可增加账号失效和连续失败告警。
- 数据库：新增账号、Token 映射、路径、缓存、任务、会话和策略表，全部提供 SQL migration。
- 配置/部署：新增网关进程、公开入口和原始 Emby 网络隔离。
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
- `cd services/web && npm run test`（当前 170 个测试通过、3 个按既有条件跳过）
- `cd services/web && npm run build`

上述集成测试使用真实 PostgreSQL 和应用内真实 HTTP 路由，但用 fake `CredentialValidator` 隔离 115；它证明账号控制面状态和数据库约束能够闭环，不证明目标 115 账号、Cookie/Web API 或播放链路真实可用。

测试分层：

- Emby 合同：固定认证、PlaybackInfo、视频流、字幕和播放事件 fixture。
- 115 合同：Cookie 脱敏、源路径逐级解析、SHA1 查重、`status=2`、`status=7`、受限 Range Hash、`status=1`、下载 Header 和错误映射。
- 保留式秒传检查器：双重查重、preID、零/一次 challenge、重复 challenge 失败关闭、目标复核、playback UA Range、`retained=true`、`cleanup.attempted=false` 和 `databaseLockValidated=false`。
- 加密合同：请求加密、响应解密、LZ4、签名和 token 固定向量。
- Service：用户资格、路径、状态机、冷却、直链兼容和 fail-closed。
- PostgreSQL：迁移、角色唯一性、Cookie 密文、任务幂等、advisory lock 和会话收口。
- 网关：透明代理、Header、GET/HEAD、302 allowlist 和 `503`。
- Web：账号只写交互、脱敏状态、路径、套餐策略和会话列表。

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
10. 用户状态、黑名单和并发限制阻止新播放。
11. 源账号或播放小号失效、限流和秒传失败均返回明确错误，不回退源账号或 Emby 视频中转。
12. Cookie、Emby Token、完整下载链接和完整 Provider 响应不出现在日志、API 或普通数据库字段。

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
- `docs/reference/data-model-reference.md`：账号、映射、任务、会话和策略表。
- `docs/reference/api-endpoint-catalog.md`：管理员账号、路径、策略和运维 API。
- `docs/reference/web-information-architecture.md`：管理员设置、套餐和播放分析页面职责。
- 部署与测试 runbook：公网入口、原始 Emby 隔离、Cookie 轮换和故障排查。

阶段 1 和阶段 2 的实现、测试与现行文档收口后，将本计划移入 `docs/archive/plan/architecture/`。OpenAPI 和多账号池未完成不阻碍归档，但必须拆成新的现行计划。
