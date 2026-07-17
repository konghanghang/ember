# Emby 115 直连播放网关实现方案

> 状态：草稿
> 负责人：Ember
> 更新时间：2026-07-17

## 背景

这个问题为什么现在要解决：

- Ember 已经具备用户生命周期、Emby 账号绑定、套餐分组、设备管理、客户端黑名单、活跃会话和播放历史能力，但没有接管视频播放数据面。
- 当前云盘挂载非 302 播放链路通常是 `115 -> 挂载/FUSE -> Emby -> 播放器`，远端 Range、探测、拖动和高码率视频会把延迟与带宽压力集中到 Emby 主机。
- 目标能力是让客户端在通过 Ember 用户、套餐和设备策略校验后，直接从用户自己的 115 账号/CDN 播放；目标账号缺文件时，通过 SHA1 秒传从系统源账号快速落盘。
- 该能力同时涉及 Emby 协议、115 授权与秒传、用户状态、并发、凭证安全、部署入口和客户端兼容，不能作为普通 API handler 局部增加。

## 目标

本方案要实现：

1. 在 Ember 仓库中新增独立播放网关，作为公网唯一 Emby 入口，透明代理普通 Emby 请求并拦截符合合同的视频播放请求。
2. 用户绑定自己的 115 账号后，播放网关能够查找目标文件、必要时从系统源账号秒传、验证目标文件并返回 115 直链 302。
3. 复用 Ember 现有用户状态、套餐分组、设备黑名单和播放分析能力，执行直连开关、并发、下载和回退策略。
4. 保持 Emby 播放进度、停止事件、字幕、图片、媒体信息和现有控制台能力继续工作。
5. 使用固定版本合同、外部依赖 mock、PostgreSQL 集成测试和安全日志证明主链路，不依赖真实外部接口完成自动化测试。

## 非目标

首版明确不做：

- 不支持 Jellyfin 或其他媒体服务器。
- 不实现多网盘 Provider 插件市场，只保留清晰的 115 集成边界。
- 不支持多 Emby Server。
- 不支持多系统源账号、管理员共享账号池和智能线路选择。
- 不支持任意 STRM、OpenList、WebDAV 和挂载路径格式自动猜测。
- 不改写 HLS、DASH、视频转码 manifest 和转码分片链路。
- 不通过 Ember 下载完整视频后再上传到用户 115。
- 不承诺在 302 发出后立即终止已经建立的 115 CDN 连接。
- 不在首版实现下一集预热、批量预转存、积分奖励或复杂流量计费。

## 当前事实

以当前代码和现行文档为准：

- 相关合同：
  - `docs/reference/emby-playback-proxy-contract.md`
  - `docs/reference/p115-direct-play-contract.md`
  - `docs/reference/playback-reporting-api-contract.md`
- 相关架构和规范：
  - `docs/system-architecture.md`
  - `docs/reference/api-development-conventions.md`
  - `docs/reference/testing-strategy.md`
  - `docs/reference/web-information-architecture.md`
  - `docs/reference/web-design-guide.md`
- 相关模型和服务：
  - `services/api/internal/models/user.go`
  - `services/api/internal/models/media_library_policy.go`
  - `services/api/internal/integrations/emby/emby.go`
  - `services/api/internal/services/device/service.go`
  - `services/api/internal/services/playback/`
- 当前行为：
  - 用户模型已有 `embyId`、过期时间、激活状态、Emby 禁用状态和套餐分组。
  - 套餐 Emby Policy 模板已有并发播放、下载、转码和远程访问配置。
  - 管理端已有活跃会话、播放历史、用户画像、设备管理、客户端黑名单和设备操作日志。
  - `EMBY_URL` 是 API 访问 Emby 的内部地址；`NEXT_PUBLIC_EMBY_URL` 是控制台展示与用户跳转地址。
  - 设置中心已有敏感值加密能力，但 `settings` 表不适合承载大量用户 115 凭证。
- 现有限制：
  - Ember 没有播放数据面进程，也没有 Emby AccessToken 到本地用户的映射。
  - Ember 没有 115 OpenAPI 授权、Token 生命周期、文件查重、秒传和下载直链适配。
  - 当前 Emby 版本合同覆盖排行榜，不覆盖播放代理；本方案已新增独立合同作为实施前置。
  - 目标 Emby、Infuse 和 115 账号的真实运行行为尚未验证，必须保持“未实机确认”标记。

## 方案设计

### 1. 用户可见行为

#### 1.1 普通用户

- 账号中心新增“115 播放账号”区域：
  - 查看绑定状态、账号标识、授权时间、最后验证时间和脱敏错误。
  - 发起二维码授权。
  - 重新授权和解绑。
- 用户继续使用现有 Emby 地址、用户名和密码登录播放器，不需要手工填写 115 直链。
- 播放符合直连条件的媒体时：
  - 用户目标账号已有文件：直接获取目标账号直链并 302。
  - 用户目标账号没有文件：等待系统秒传后 302。
  - 账号失效、秒传失败或策略拒绝：返回明确失败，不静默走服务器中转。

#### 1.2 管理员

- 系统设置增加“直连播放”配置分组：
  - 网关开关和公开地址。
  - 系统源 115 账号。
  - 目标接收目录。
  - 路径映射规则。
  - Provider 连接测试和脱敏状态。
- 套餐分组增加直连权益：
  - 是否允许直连。
  - 网关并发限制。
  - 是否允许下载。
  - 是否允许本地媒体原始回退。
  - 云端媒体解析失败时的失败策略。
- 播放分析增加“直连会话”Tab：
  - 展示用户、客户端、文件、解析/查重/秒传/直链阶段、耗时和脱敏错误。
- 设备管理继续承载客户端黑名单和设备注销，不增加平行风控页面。

#### 1.3 必须保持不变的行为

- 现有 Ember 登录、用户生命周期、续期、封禁、套餐分组和 Emby Policy 同步行为保持不变。
- Emby 图片、媒体信息、字幕、播放进度、停止事件和非视频 API 继续工作。
- 本地媒体可按显式路径规则保持原始 Emby 播放，不要求全部迁移到 115。
- 前端实现必须遵守 Ember 风格；设计与交互基线以 `docs/reference/web-design-guide.md` 为准，不新增独立视觉系统或堆叠大段说明文案。

### 2. 目标架构

```text
Infuse / Emby Client
          |
          v
ember-playback-gateway
  |-- 登录、媒体信息、图片、字幕、播放进度 --> Emby Server
  |
  `-- 原始视频流请求
       |-- Token -> Ember 用户
       |-- 用户/套餐/设备/并发策略
       |-- ItemId + MediaSourceId -> 路径和源文件元数据
       |-- 目标 115 按 SHA1 + size 查重
       |-- 必要时源账号 -> 目标账号秒传
       |-- 获取目标账号下载直链
       `-- HTTP 302
                |
                v
            115 CDN

Ember API / Web
  |-- 账号授权、配置、路径规则、套餐策略、会话查询
  `-- PostgreSQL：凭证、映射、任务、会话和审计
```

控制面与数据面分离：

- `services/api` 现有 API：负责配置、授权编排、管理接口和控制台数据。
- 新播放网关进程：负责 Emby 兼容代理、身份解析、播放策略和 302。
- PostgreSQL：作为用户、凭证、策略、任务和跨副本互斥的真相源。
- 视频字节：302 成功后只在客户端与 115 CDN 之间传输。

### 3. 代码边界

建议在现有 Go Module 内新增独立二进制：

```text
services/api/cmd/playback-gateway/main.go
services/api/internal/playbackgateway/
services/api/internal/services/directplay/
services/api/internal/integrations/p115/
```

职责：

- `cmd/playback-gateway`：只负责配置、数据库、依赖和 HTTP Server 装配。
- `internal/playbackgateway`：Emby 兼容反向代理、路由分类、Header 和 302 输出。
- `internal/services/directplay`：用户资格、媒体解析、查重、秒传、并发和会话状态机。
- `internal/integrations/p115`：OpenAPI 授权、搜索、上传初始化、Range challenge 和下载地址 DTO 适配。
- `internal/integrations/emby`：补充固定合同中的播放信息和必要条目查询，不把网关业务塞入现有 Emby Client。

首版不新增 Redis。单副本网关使用数据库唯一约束和 PostgreSQL advisory lock；只有真实负载证明 PostgreSQL 不能满足时，才另行提案引入缓存基础设施。

### 4. 数据与模型

所有模型字段必须显式指定 `gorm:"column:xxx"`，JSON 字段使用 camelCase；所有表结构通过幂等 SQL migration 落地，不能依赖 `AUTO_MIGRATE`。

迁移文件放在 `infrastructure/database/`，使用 `YYYYMMDD_NN_<description>.sql` 命名。时间戳统一按 UTC 存储；授权清理、会话过期和临时文件清理等调度统一复用全局 `CRON_TIMEZONE`，不新增播放网关专用业务时区。

#### 4.1 `p115_accounts`

保存系统源账号和用户播放账号：

- `id`
- `owner_type`：`system`、`user`
- `owner_user_id`：用户账号必填，系统账号为空
- `alias`
- `auth_mode`：首版固定 `open_api`，兼容模式预留 `legacy_cookie`
- `app_id`
- `provider_user_id`
- `access_token_ciphertext`
- `refresh_token_ciphertext`
- `token_expires_at`
- `status`：`pending`、`active`、`expired`、`revoked`、`error`
- `enabled`
- `authorized_at`
- `last_validated_at`
- `last_error_code`
- `last_error_message`
- `created_at`
- `updated_at`

约束：

- 用户首版只允许一个启用的 115 播放账号。
- 明文 Token 不出 Repository 边界，不出现在 JSON 和日志中。
- Token 刷新必须按账号加锁，并原子替换 refresh token。

#### 4.2 `emby_access_tokens`

建立 Emby AccessToken 到 Ember 用户的映射：

- `id`
- `server_id`
- `token_hash`
- `emby_user_id`
- `user_id`
- `device_id`
- `client_name`
- `last_seen_at`
- `revoked_at`
- `created_at`
- `updated_at`

约束：

- 只保存 Token 哈希。
- `server_id + token_hash` 唯一。
- 用户解绑、停用或访问禁用时可批量撤销。

#### 4.3 `playback_path_mappings`

定义 Emby 路径如何映射到系统源账号：

- `id`
- `name`
- `source_prefix`
- `provider`
- `source_account_id`
- `source_root_id`
- `local_origin_allowed`
- `priority`
- `enabled`
- `created_at`
- `updated_at`

首版只支持一种明确路径格式，使用最长前缀和优先级匹配；禁止运行时遍历多种猜测规则。

#### 4.4 `playback_media_cache`

缓存 Emby 媒体源与 115 源文件关系：

- `id`
- `emby_server_id`
- `emby_item_id`
- `media_source_id`
- `emby_path`
- `source_account_id`
- `source_file_id`
- `source_pick_code`
- `file_name`
- `sha1`
- `size`
- `verified_at`
- `expires_at`
- `created_at`
- `updated_at`

唯一键：

```text
emby_server_id + emby_item_id + media_source_id
```

#### 4.5 `playback_transfer_tasks`

记录目标账号秒传任务：

- `id`
- `target_account_id`
- `source_account_id`
- `sha1`
- `size`
- `file_name`
- `target_parent_id`
- `status`：`pending`、`checking_target`、`initializing`、`challenging`、`verifying`、`succeeded`、`failed`、`cooling_down`
- `target_file_id`
- `target_pick_code`
- `attempt_count`
- `last_error_code`
- `last_error_message`
- `started_at`
- `finished_at`
- `created_at`
- `updated_at`

活动任务唯一键：

```text
target_account_id + sha1 + size
```

#### 4.6 `direct_play_sessions`

记录网关直连会话：

- `id`
- `play_session_id`
- `emby_session_id`
- `user_id`
- `emby_item_id`
- `media_source_id`
- `device_id`
- `client_name`
- `remote_address_hash`
- `source_account_id`
- `target_account_id`
- `transfer_task_id`
- `status`：`requested`、`resolving`、`transferring`、`redirect_issued`、`playing`、`stopped`、`expired`、`failed`
- `redirected_at`
- `last_progress_at`
- `ended_at`
- `error_code`
- `error_message`
- `created_at`
- `updated_at`

不保存完整 115 直链；IP 如需持久化应哈希或脱敏。

#### 4.7 `plan_group_direct_play_policies`

直连网关策略与现有 Emby Policy 模板分表维护：

- `plan_group_key`
- `enabled`
- `simultaneous_stream_limit`
- `allow_download`
- `allow_local_origin_fallback`
- `cloud_failure_mode`：首版固定或默认 `fail_closed`
- `created_at`
- `updated_at`

展示层可以与 Emby Policy 同页编辑，但两者执行边界不同，不能把网关策略全部塞入 `plan_group_emby_policy_templates`。

### 5. API 与边界

列表接口统一返回 `data`，对外字段使用 camelCase。

#### 5.1 用户 115 授权

- `GET /api/v1/user/p115/account`
  - 返回绑定状态和脱敏信息。
- `POST /api/v1/user/p115/authorizations`
  - 创建 PKCE 二维码授权会话。
- `GET /api/v1/user/p115/authorizations/:id`
  - 查询授权状态；不返回 Provider Token。
- `DELETE /api/v1/user/p115/account`
  - 撤销本地凭证并禁止后续直连；是否同步撤销 Provider 授权以实际合同为准。

#### 5.2 管理员账号和路径规则

- `GET /api/v1/admin/p115/accounts`
- `POST /api/v1/admin/p115/authorizations`
- `GET /api/v1/admin/p115/authorizations/:id`
- `POST /api/v1/admin/p115/accounts/:id/validate`
- `DELETE /api/v1/admin/p115/accounts/:id`
- `GET /api/v1/admin/direct-play/path-mappings`
- `POST /api/v1/admin/direct-play/path-mappings`
- `PUT /api/v1/admin/direct-play/path-mappings/:id`
- `DELETE /api/v1/admin/direct-play/path-mappings/:id`

首版只允许一个启用的系统源账号，接口结构保留列表形式便于审计，不提前实现账号池选择。

#### 5.3 套餐直连策略

- `GET /api/v1/admin/plan-groups/:key/direct-play-policy`
- `PUT /api/v1/admin/plan-groups/:key/direct-play-policy`

保存后只影响新播放请求；已发出的 CDN 链接不保证立即撤销。

#### 5.4 直连会话和任务

- `GET /api/v1/admin/direct-play/sessions`
- `GET /api/v1/admin/direct-play/transfers`
- `POST /api/v1/admin/direct-play/transfers/:id/retry`
- `POST /api/v1/admin/direct-play/media-cache/:itemId/refresh`

重试必须复用原任务幂等键，不能通过手工接口绕过账号冷却和并发约束。

#### 5.5 播放网关公开接口

网关对外暴露 Emby 兼容路径，不新增客户端专用 `/api/v1` 播放协议：

- 普通请求透明转发。
- `AuthenticateByName` 响应旁路建立 Token 哈希映射。
- 固定合同中的视频流路径执行直连编排。
- 字幕和 Playing/Progress/Stopped 继续转发并旁路更新会话。

### 6. 关键流程

#### 6.1 用户授权 115

1. 用户在账号中心发起授权。
2. API 创建短期授权会话和 PKCE verifier，调用 115 获取二维码。
3. Web 展示二维码并轮询 Ember 授权状态接口。
4. 用户扫码确认后，API 获取 access token 和 refresh token。
5. API 在事务内加密保存凭证、用户标识和授权状态。
6. API 立即执行一次只读账号验证，成功后状态改为 `active`。

#### 6.2 Emby 登录与 Token 映射

1. 客户端通过网关调用 `AuthenticateByName`。
2. 网关将请求转发给 Emby。
3. Emby 成功响应后，网关读取 `User.Id`、`AccessToken` 和 `ServerId`。
4. 网关只保存 AccessToken 哈希，根据 `users.emby_id` 映射 Ember 用户。
5. 原始认证响应不修改地返回客户端。

#### 6.3 目标账号已有文件

1. 客户端获取 PlaybackInfo，网关透明转发并记录 `ItemId + MediaSourceId + PlaySessionId`。
2. 客户端请求原始视频流。
3. 网关校验 Token、用户状态、套餐、客户端黑名单和并发。
4. 网关从媒体缓存或 Emby 媒体源解析路径、SHA1 和 size。
5. 使用用户目标账号按 SHA1 + size 查重。
6. 命中目标文件后，使用目标 pickCode 和真实客户端 UA 获取直链。
7. 校验直链域名和过期时间，返回 302。

#### 6.4 目标账号缺文件并秒传

1. 网关创建或复用 `targetAccountId + SHA1 + size` 秒传任务。
2. 获取数据库互斥后再次查重。
3. 目标账号调用 OpenAPI 上传初始化。
4. 如果直接复用成功，进入目标文件验证。
5. 如果收到 Range challenge：
   - 使用源账号 pickCode 获取源下载地址。
   - 读取 `signCheck` 指定字节范围。
   - 计算范围 SHA1 并提交 `signKey + signValue`。
6. 目标账号重新查询并确认文件存在。
7. 写入目标文件缓存，获取目标直链并返回 302。
8. 最终响应不是明确复用时停止，不执行完整文件中转上传。

#### 6.5 播放会话与并发

1. 网关以 `PlaySessionId + 用户 + 设备` 归并 `HEAD`、Range、预加载和重连请求。
2. `redirect_issued` 后等待 Playing/Progress 事件进入 `playing`。
3. Progress 更新 `last_progress_at`。
4. Stopped 将会话收口为 `stopped`。
5. 客户端未上报停止时，由 TTL 任务收口为 `expired`。
6. 套餐并发按活跃网关会话执行，不能只读取 Emby Session 数量。

#### 6.6 Token 刷新

1. Provider 调用发现 access token 需要刷新。
2. 对 `p115_accounts.id` 获取数据库锁。
3. 锁内重新读取 Token，避免其他请求已经刷新。
4. 使用 refresh token 获取新 Token。
5. 在同一事务内替换 access token、refresh token 和过期时间。
6. 结果不确定或 refresh token 失效时将账号标记为 `expired`，要求重新授权。

### 7. 失败路径与边界条件

- Emby Token 无法映射：拒绝播放，不信任请求参数里的 `UserId`。
- 用户过期、停用或 Emby 访问禁用：禁止生成新直链和新秒传任务。
- 用户未绑定 115：首版返回明确不可用，不自动使用管理员共享账号。
- 路径未命中：本地媒体按显式规则回退；未知云端路径失败关闭。
- 媒体缓存失效：重新解析一次；仍失败则返回脱敏错误。
- 目标 SHA1 搜索失败：使用短负缓存和账号级冷却，不无限遍历。
- 秒传 challenge 越界或 Range 获取失败：任务失败，不继续上传完整文件。
- 上传初始化 HTTP 成功但未复用：不能判定成功，必须验证目标文件。
- 直链域名不在 allowlist：拒绝 302 并记录安全事件。
- UA 不匹配：按真实客户端 UA 重新获取一次，不跨 UA 复用缓存。
- 客户端请求转码：首版云端媒体拒绝；不能静默让 Emby 中转视频。
- Provider 限流：账号进入共享冷却，返回可重试状态并带内部错误码。
- Token 并发刷新：只允许一个刷新任务，避免新 refresh token 被并发请求作废。
- 用户在播放中解绑或被封禁：阻止新请求，但已发出的 CDN 连接可能继续到链接过期。
- 多副本并发：数据库唯一约束和 advisory lock 必须保证任务幂等。
- 兼容性约束：未进入固定合同的 Emby/115 行为一律标记未证实，不以客户端偶然成功替代合同。

### 8. 日志、指标与审计

关键阶段日志：

- 请求接收
- Token 映射和策略结果
- 媒体缓存命中/失效
- 目标 SHA1 查重
- 秒传初始化和 Range challenge
- 目标文件验证
- 直链获取和 302 签发
- Provider 冷却和失败分类
- 会话开始、进度、停止和 TTL 过期

日志应记录：

- `userId`
- `itemId`
- `mediaSourceId`
- `playSessionId`
- `deviceId`
- `clientName`
- `sourceAccountId`
- `targetAccountId`
- SHA1 前缀
- 文件大小
- 阶段耗时
- 脱敏错误码

禁止记录：

- Emby AccessToken
- 115 access token / refresh token
- Cookie
- 完整 SHA1 与账号凭证的组合日志
- 完整 115 下载直链
- Provider 完整响应体

首版指标：

- 媒体缓存命中率
- 目标账号文件命中率
- 秒传成功率和耗时
- Range challenge 比例和耗时
- 302 成功率和总耗时
- Provider API 错误和冷却次数
- 原始回退次数
- 活跃直连会话和并发拒绝次数

### 9. 配置与部署

#### 9.1 部署拓扑

- 新增 `ember-playback-gateway` 镜像或现有 Go 构建中的独立 target。
- 公网反向代理只暴露播放网关。
- 原始 Emby Server 端口限制为内网或运维 allowlist。
- `EMBY_URL` 继续指向内部 Emby Server。
- `NEXT_PUBLIC_EMBY_URL` 改为播放网关公开地址。

#### 9.2 配置建议

数据库运行期配置：

- `PLAYBACK_GATEWAY_ENABLED`
- `PLAYBACK_GATEWAY_PUBLIC_URL`
- `P115_APP_ID`
- `P115_TARGET_DIRECTORY_NAME`
- 直链域名 allowlist
- 缓存、会话和冷却 TTL

部署期环境变量：

- `PLAYBACK_GATEWAY_LISTEN_ADDR`
- `P115_APP_SECRET`：仅授权码模式使用
- `CONFIG_ENCRYPTION_KEY`：沿用现有信任根

具体配置键在实现前要补入 `docs/reference/configuration-reference.md`，并明确哪些修改需要重启网关。

### 10. 分阶段落地

#### 阶段 0：合同和受控 PoC

- 完成本文及两个 reference 合同。
- 使用 fake Emby 和 fake 115 固定请求/响应 fixture。
- 在用户明确授权后，使用受控测试账号验证目标 Emby、Infuse 和 115 OpenAPI。
- 申请 Ember 自有 115 AppID；第三方 AppID 只能用于技术 PoC。

完成条件：

- Emby 请求路径、115 授权、搜索、上传初始化、Range challenge 和直链约束都有版本化证据或明确的未确认标记。

#### 阶段 1：最小闭环

- 一个 Emby Server。
- 一个系统源 115 账号。
- 每个用户一个 115 OpenAPI 账号。
- 一种路径映射。
- Infuse Direct Play。
- Token 映射、目标查重、秒传、目标验证和 302。
- 基础会话、并发、冷却、日志和管理查询。

完成条件：

- 用户目标账号命中和缺失两条链路都能稳定返回 302。
- 视频字节不经过 Ember/Emby，播放进度仍能到达 Emby。
- 用户状态、黑名单和并发限制能够阻止新播放。

#### 阶段 2：运营能力

- 管理端完整配置、用户绑定状态、直连会话和任务重试。
- 账号健康检查、Bot 告警、失败趋势和清理任务。
- 多网关副本和跨副本任务互斥。
- 显式的本地媒体回退和云端失败策略。

#### 阶段 3：账号池与预热

- 多系统源账号和管理员账号池。
- 套餐允许账号范围、粘性选路、最少并发和熔断。
- 全域 SHA1 到账号位置索引。
- 下一集预热、元数据预解析和短期直链缓存优化。
- 多来源路径和其他 Provider 仅在真实需求证明后另行设计。

## 影响范围

涉及的子系统：

- API：有
  - 115 授权、账号、路径映射、套餐策略、会话和任务管理。
  - 新增播放网关二进制和 direct play 业务服务。
  - 补充 Emby 播放信息适配。
- Web：有
  - 账号中心 115 授权。
  - 系统设置直连分组。
  - 套餐分组直连权益。
  - 播放分析直连会话 Tab。
- Bot：阶段 2 可选
  - 账号失效、连续秒传失败和账号池耗尽告警。
- 数据库：有
  - 新增账号、Token 映射、路径、缓存、任务、会话和策略表。
  - 所有变更需要 SQL migration。
- 配置/部署：有
  - 新增播放网关进程、容器、公开入口和原始 Emby 网络隔离。
- 文档：有
  - `docs/system-architecture.md`
  - `docs/reference/configuration-reference.md`
  - `docs/reference/data-model-reference.md`
  - `docs/reference/api-endpoint-catalog.md`
  - `docs/reference/web-information-architecture.md`
  - 部署和测试 runbook

## 验证方式

### 编译与自动化测试

- `cd services/api && go test ./...`
- `cd services/api && go vet ./...`
- `cd services/api && go build ./...`
- `cd services/web && npm run test`
- `cd services/web && npm run build`

测试分层：

- Emby 合同测试：fake Emby 固定认证、PlaybackInfo、流、字幕和会话事件。
- 115 合同测试：fake Provider 固定搜索、直接复用、Range challenge、Token 刷新和直链。
- Service 单元测试：用户资格、路径规则、状态机、错误分类和回退策略。
- PostgreSQL 集成测试：迁移、唯一约束、Token 轮换、任务幂等、advisory lock 和会话收口。
- 网关组件测试：透明代理、Header、GET/HEAD、302、域名 allowlist 和错误响应。
- Web Vitest：授权状态、二维码轮询、套餐策略和直连会话列表。

涉及 Token 轮换、用户播放资格、任务状态机、DTO 转换和 Provider 适配时按 TDD 推进：先补失败测试，再做最小实现，最后重构。

所有测试必须 mock Emby 和 115，禁止真实外部请求。

### MVP 验收场景

1. 目标账号已有相同 SHA1 + size 文件，跳过秒传并返回目标账号直链 302。
2. 目标账号缺文件，上传初始化直接复用成功，验证目标文件后返回 302。
3. 秒传触发 Range challenge，只读取源文件指定范围，验证后返回 302。
4. 同一文件的并发 `HEAD`、预加载和 Range 请求只创建一个秒传任务。
5. 直链 `Location` 命中 allowlist，视频字节不经过 Ember 和 Emby。
6. Playing、Progress、Stopped 仍被 Emby 接收，播放历史和进度正常。
7. 用户过期、停用、访问禁用、客户端黑名单和并发超限阻止新播放。
8. 秒传失败、Token 失效和 Provider 限流不会静默回退服务器中转。
9. access token、refresh token、Cookie 和完整下载链接不出现在日志、API 和数据库普通字段中。
10. 不支持的转码请求明确失败，不被误判为 Direct Play。

### 受控真实验证

真实 Emby/115 验证只在用户明确授权后执行，并且：

- 使用测试账号和测试文件。
- 不启动项目后台服务作为验证手段。
- 先做只读版本、授权和搜索验证，再做单文件秒传与直链验证。
- 记录请求合同、脱敏响应字段、耗时和实际数据路径。
- 验证完成后撤销临时授权、分享和测试文件。

## 落地后文档处理

主体落地后应同步：

- 将播放网关进程、数据流、模型和配置入口写入 `docs/system-architecture.md`。
- 将稳定配置写入 `docs/reference/configuration-reference.md`。
- 将表关系写入 `docs/reference/data-model-reference.md`。
- 将 API 写入 `docs/reference/api-endpoint-catalog.md`。
- 将账号中心、套餐分组和播放分析页面职责写入 `docs/reference/web-information-architecture.md`。
- 将部署拓扑、原始 Emby 隔离和故障排查写入 runbook。
- 当阶段 1 与阶段 2 的实现、测试和现行文档都收口后，将本方案移入 `docs/archive/plan/architecture/`；未完成阶段 3 不影响前两阶段归档，但剩余内容必须拆成新的现行计划。
