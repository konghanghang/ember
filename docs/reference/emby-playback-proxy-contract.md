# Emby 4.9 系列播放代理 API 合同

本文档记录 Ember 播放网关依赖的 Emby 原生认证、播放信息、原始视频流、字幕和播放会话接口。目标是为后续 115 直连播放提供版本化证据，禁止根据客户端表现或其他 Emby 版本经验猜测协议。

## 1. 适用范围与证据等级

当前协议基线与运行兼容范围：

| 组件 | 版本 | 证据 | 结论 |
| --- | --- | --- | --- |
| 协议证据基线 | `4.9.3.0 Release` | Emby.SDK 提交 `6ee0155063bc85578196489926359a8f37419502` | 本文列出的 method、path 和 DTO 字段以该提交为主要出处 |
| Gateway 运行兼容范围 | `>= 4.9.0.0 && < 4.10.0.0` | 官方 `4.9.0.70` 至 `4.9.5.0` 的 9 个稳定 SDK Tag 对当前使用的 12 个 path 及核心 DTO 做过语义比对 | 四段数字版本落在该半开区间即可启动；`4.9.3.0` 不是唯一允许版本 |
| 目标 Emby Server | 部分确认 | 2026-08-23 Gateway 生产启动日志确认 `4.9.3.0` 与非空 ServerId；同一实例的 `GET /System/Info/Public` 已确认无登录返回 `PublicSystemInfo` | 已确认目标版本和公开发现语义；ServerId 原值及其他 API 运行行为未公开或未验证 |
| Infuse 客户端行为 | 部分确认 | 2026-08-23 本地实测确认 `Infuse-Direct/8.5` 使用根 API path、`X-Emby-Authorization: MediaBrowser ...`，认证成功响应为 `deflate` JSON，并在登录后通过同一 Header 的非空 `Token` 字段请求普通资源。2026-08-29 Infuse `8.5.2` 已确认按需 PlaybackInfo、`Size=0` proof、source 映射、首次转存、已有目标复用和 Gateway `302` | 登录、普通资源、PlaybackInfo 与 Gateway 302 已有实机证据；gzip、115 CDN 媒体字节/Range、字幕及完整 Progress/Stopped 仍未完成目标环境验证 |

证据等级：

- **固定版本源码确认**：由 Emby `4.9.3.0` SDK OpenAPI 直接证明。
- **4.9 系列兼容确认**：官方 9 个稳定 `4.9` SDK Tag 中，Gateway 当前依赖的 12 个 path 以及 `SystemInfo`、`AuthenticationResult`、`UserDto`、`PlaybackInfoResponse`、`MediaSourceInfo` 核心字段保持一致。
- **HTTP 传输要求**：由 HTTP 标准和反向代理边界决定，但仍需客户端合同测试锁定。
- **受控生产日志确认**：只证明日志中可直接观察到的 method、path、状态、客户端标识和 Gateway 分支，不扩展推断未记录的 Header 或后续请求行为。
- **未实机确认**：OpenAPI 允许该用法，但尚未在目标 Emby 与 Infuse 组合上验证。

固定版本证据：

- [Emby.SDK 4.9.3.0 OpenAPI](https://github.com/MediaBrowser/Emby.SDK/blob/6ee0155063bc85578196489926359a8f37419502/Resources/OpenApi/openapi_v3.json)
- [Emby.SDK 4.9.5.0 OpenAPI](https://github.com/MediaBrowser/Emby.SDK/blob/4.9.5.0/Resources/OpenApi/openapi_v3.json)
- [Emby.SDK 官方 Tags](https://github.com/MediaBrowser/Emby.SDK/tags)
- [Emby.SDK 4.9.3.0 User Authentication](https://github.com/MediaBrowser/Emby.SDK/blob/6ee0155063bc85578196489926359a8f37419502/Documentation/doc/restapi/User-Authentication.html)
- [Emby.SDK 4.9.3.0 Password Authenticator](https://github.com/MediaBrowser/Emby.SDK/blob/6ee0155063bc85578196489926359a8f37419502/SampleCode/RestApi/Emby.ApiClient/Emby.ApiClient/Client/Authentication/EmbyPasswordAuthenticator.cs)
- [Emby.SDK 4.9.3.0 SystemInfoPublic](https://github.com/MediaBrowser/Emby.SDK/blob/6ee0155063bc85578196489926359a8f37419502/Documentation/reference/RestAPI/SystemService/getSystemInfoPublic.html)
- [Emby.SDK 4.9.3.0 GET PlaybackInfo 客户端合同](https://github.com/MediaBrowser/Emby.SDK/blob/6ee0155063bc85578196489926359a8f37419502/SampleCode/RestApi/Emby.ApiClient/Emby.ApiClient/Api/MediaInfoServiceApi.cs#L45-L82)
- [Emby 官方 Web 播放器 DirectStreamUrl 选择逻辑](https://github.com/MediaBrowser/emby-webcomponents/blob/69877ad3319a7b422d0e1f8289dc5ca234c4040d/playback/playbackmanager.js#L2644-L2668)
- [Ember Playback Reporting 合同](./playback-reporting-api-contract.md)
- [Emby Gateway 客户端兼容矩阵](./emby-client-compatibility-matrix.md)

## 2. 网关职责与信任边界

播放网关位于 Emby 客户端与 Emby Server 之间：

```text
Emby 客户端
  -> Ember Playback Gateway
       -> 普通 Emby API、字幕、播放进度：透明转发到 Emby Server
       -> 视频流：先做本地身份门控，再尝试 115 加速
            -> 加速成功：返回 115 直链 302
            -> 不适用或失败：透明转发原始请求到 Emby Server
```

边界约束：

- 网关必须成为公网唯一 Emby 入口；原始 Emby Server 只允许内网或受控运维访问。
- 已通过 Token 门控的普通接口默认透明转发，不做猜测式改写；未认证 public bootstrap 路由必须先完成同版本合同核对，再加入显式 allowlist。
- 网关不得使用客户端提交的 `UserId` 作为最终身份依据，必须由 Emby AccessToken 映射到 Ember 用户。
- 视频字节在 302 成功后由客户端直接向 115 CDN 请求；fallback 时仍按普通反向代理链路由 Emby 经 Gateway 返回客户端。
- Emby 正常代理播放是基线能力，115 直连是可选加速；合法 Principal 不能因为没有 115 条件、路径未映射或 Provider 故障而失去正常播放能力。
- 网关不得记录 AccessToken、完整 115 直链、Cookie 或其他可复用凭证。

### 2.1 客户端 root path 与上游 base path

固定 `4.9.3.0` OpenAPI 把 server base 声明为 `http://emby.media/emby`，同时把 operation path 声明为 `/System/Info/Public`、`/Users/AuthenticateByName`、`/Items/{Id}/PlaybackInfo`、`/Videos/{Id}/stream` 等根路径。9 个稳定 `4.9` SDK Tag 的顶层 family 只有 `Parties` 在 `4.9.1.90` 及之后新增，其余保持一致；Gateway 使用这 9 个版本的并集。生产 Infuse `8.5` 已确认直接请求根 `/System/Info/Public`，因此 Gateway 固定以下兼容边界：

| 客户端 path | Gateway 内部/上游 path | 处理 |
| --- | --- | --- |
| 固定 OpenAPI 顶层 API family 的根路径，例如 `/System/...` | `/emby/System/...` | 先规范化，再执行同一路由分类和门控 |
| 已带 `/emby/...` | 保持单一 `/emby/...` | 兼容历史客户端地址，不重复添加前缀 |
| 精确重复 `/emby/emby/...` | 不转发 | 返回空体 `400` 并记录固定 `request_path_invalid` |
| 根 `/web/...` 与未知 Surface | 不做 API 规范化 | 当前继续受保护透传；Web/static/WebSocket 合同另行实现 |

规范化只改 `URL.Path`，必须保留 method、query、Header 和 body。存在 alternate escaping 的请求不能通过 root 规范化继承认证、bootstrap、PlaybackInfo 或视频特殊处理；大小写、尾斜杠和额外层级同样不能放宽精确路由。

### 2.2 启动期上游身份核对

协议基线 `4.9.3.0` OpenAPI 声明：

```http
GET /emby/System/Info
X-Emby-Token: <admin-api-key>
```

成功响应是 `SystemInfo`，当前启动边界只读取：

| 字段 | 类型 | 用途 |
| --- | --- | --- |
| `Id` | `string` | 固定当前上游 ServerId，并传给 `EmbyTokenService` |
| `Version` | `string` | 必须是四段数字版本，且满足 `>= 4.9.0.0 && < 4.10.0.0` |
| `ServerName` | `string` | 可选的脱敏启动诊断，不参与身份判断 |

运行要求：

- Gateway 只能在身份核对成功后进入监听状态；不能使用第一次任意登录响应静默定义 ServerId。
- `Id` 必须非空且满足长度/控制字符边界；`Version` 必须精确由四段无符号十进制数组成并落在 `[4.9.0.0, 4.10.0.0)`，前导零、后缀、空白、缺段或多段全部拒绝；`ServerName` 可空但必须有界。
- 上游重定向、非 `200`、非 JSON、响应超过 `256 KiB`、字段缺失、版本不符、超时或取消全部失败关闭。
- API Key 只存在于请求 Header，禁止进入错误、日志、响应、指标或持久化；上游 URL 和完整响应体同样不得进入错误或日志。
- 自动化只使用 fake Emby；目标实例完成显式只读验证前，仍不能宣称其版本、ServerId 和运行行为已确认。

## 3. 用户认证合同

### 3.1 用户名密码认证

```http
POST /emby/Users/AuthenticateByName
Content-Type: application/json

{
  "Username": "example",
  "Pw": "<password>"
}
```

`4.9.3.0` OpenAPI 的请求模型是 `AuthenticateUserByName`，成功响应是 `Authentication.AuthenticationResult`：

| 字段 | 类型 | 网关用途 |
| --- | --- | --- |
| `User` | `UserDto` | 读取 `User.Id`，映射 Ember `users.emby_id` |
| `SessionInfo` | `SessionInfo` | 保留给 Emby 客户端，不作为 Ember 身份真相源 |
| `AccessToken` | `string` | 计算不可逆哈希后建立 Token 到 Ember 用户映射 |
| `ServerId` | `string` | 识别目标 Emby Server，避免跨 Server Token 混用 |

固定提交同时要求认证请求携带应用/设备授权头，Header 名允许二选一：

```http
Authorization: Emby UserId="", Client="Infuse", Device="iPhone", DeviceId="device-id", Version="version", Token=""
```

或：

```http
X-Emby-Authorization: Emby UserId="", Client="Infuse", Device="iPhone", DeviceId="device-id", Version="version", Token=""
```

目标 Infuse `8.5` 的实际认证请求使用：

```http
X-Emby-Authorization: MediaBrowser UserId="", Client="Infuse", Device="...", DeviceId="...", Version="8.5", Token=""
```

首期解析约束：

- `Authorization`、`X-Emby-Authorization`、`X-MediaBrowser-Authorization` 只能出现一个且只能有一个值；同时出现、重复值或空值都失败关闭。
- `Emby` scheme 允许用于固定 SDK 声明的 `Authorization` 或 `X-Emby-Authorization`；`MediaBrowser` 允许用于目标 Infuse 实测的 `X-Emby-Authorization` 和兼容 `X-MediaBrowser-Authorization`，标准 `Authorization: MediaBrowser ...` 继续拒绝。scheme 大小写敏感；要求唯一的 `Client`、`Device`、`DeviceId` 和 `Version`，`UserId` 可空，其他组合失败关闭。
- 登录前 AuthenticateByName 的 `Token` 只允许缺失或空字符串；登录后受保护请求可以把非空 Token 放在严格合法的 `Emby` 应用头中，目标 Infuse `8.5` 实测还会放在 `X-Emby-Authorization: MediaBrowser ...` 中。
- AuthenticateByName 同时禁止直接 Token Header 和固定 query Token aliases；public users/无 Index 头像在登录前可使用空 Token 应用头，登录后可改用已经映射的通用 Token carrier。
- 值使用有界 quoted-string；重复字段、未知字段、控制字符、非法转义和超长值全部拒绝。
- `Client` 保存为非权威 `clientName`，`DeviceId` 保存为非权威 `deviceId`；二者只用于审计和设备撤销，不能替代 `User.Id + ServerId + AccessToken` 身份绑定。

网关处理要求：

1. 先验证应用/设备授权头，再将认证请求和响应透明转发，不修改 Emby 返回体。
2. 成功响应后提取 `User.Id`、`AccessToken` 和 `ServerId`。
3. 使用从 `CONFIG_ENCRYPTION_KEY` 按 `emby-access-token` purpose 派生的密钥计算 HMAC-SHA256；数据库只保存 32 字节摘要，不保存明文。
4. 根据 `users.emby_id` 查找 Ember 用户，并记录设备、客户端和最后访问时间。
5. 映射写入失败不能篡改 Emby 已成功的认证响应；该 Token 保持未映射，后续受保护请求和直连失败关闭。
6. 用户过期只做动态资格拒绝，不立即硬撤销映射；用户停用、Emby 访问禁用、Emby 账号解绑或删除时硬撤销。已发出的短期 CDN 链接不保证可以立即终止。

认证响应传输边界：

- 目标 Emby/Infuse 实测组合返回 `Content-Encoding: deflate`。Gateway 的旁路检查白名单为 `identity`、`gzip` 和 `deflate`；必须原样保留响应 Header 和字节，只解码 Token 映射使用的旁路副本。gzip 是 fake 合同覆盖的兼容能力，不代表目标环境已返回 gzip。
- `gzip` 使用标准 gzip 格式，`deflate` 同时兼容 zlib-wrapped 和 legacy raw DEFLATE；编码响应读取与解码后 JSON 都受 `1 MiB` 上限约束，防止压缩炸弹。
- 无效 gzip/deflate、解码后超限或白名单外 Content-Encoding 不能改写 Emby 成功响应，只是不建立映射，并记录固定 `contentEncoding + reasonCode + errorType`；禁止输出响应体、AccessToken 或原始 Header 值。

### 3.2 登录前 bootstrap

固定 `4.9.3.0` User Authentication 文档明确把以下调用放在用户认证之前：

| Method | Path | 用途 | 首期网关处理 |
| --- | --- | --- | --- |
| `GET` | `/System/Info/Public` 或 `/emby/System/Info/Public` | Infuse 登录前读取服务器公开信息 | 不做本地鉴权，规范化后透明转发并由 Emby 决定响应 |
| `GET` | `/emby/Users/Public` | 获取允许显示在登录页的公开用户 | 验证应用/设备授权头后透明转发 |
| `GET`, `HEAD` | `/emby/Users/{Id}/Images/{Type}` | 可选公开用户头像 | 验证应用/设备授权头和精确路径形态后透明转发 |

边界：

- bootstrap 表示“不要求已映射 AccessToken”，不表示任意匿名请求；只有层级精确的 `GET System/Info/Public` 不要求应用头。公开用户与头像在登录前必须携带严格应用/设备授权头，登录后也可携带已经映射的兼容矩阵 Token carrier。
- public 用户头像只放行文档明确引用的无 `Index` 形态；上传、删除、`/Delete`、带额外 path segment 或其他用户接口仍受 Token 门控。
- `System/Info/Public` 固定参考页标记 `Requires authentication as user`，但 2026-08-23 两份运行证据推翻了这一生成标记：Infuse `8.5` 在取得用户 Token 前不带可解析应用头请求该路径；同一目标 Emby `4.9.3.0` 的精确接口在无登录请求下直接返回 `PublicSystemInfo`。Gateway 因此只对这个精确 `GET` 取消本地应用头和 Token 门控，保留原始 Header 并让 Emby 响应保持权威；其他匿名路径没有扩大。
- Branding、服务器发现、Quick Connect 和其他登录前路径没有进入本次固定证据，继续失败关闭；目标 Infuse 实际请求到这些路径时，必须先补合同。

### 3.3 Token 哈希映射与本地撤销

`emby_access_tokens` 是 Emby 身份到 Ember 用户的桥接，不是新的 Token，也不代替 Emby 验证：

- 映射主键语义为 `ServerId + HMAC-SHA256(AccessToken)`；同一 Server/Token 重复认证必须幂等 upsert。
- 认证响应的 `ServerId` 必须与网关启动期版本化核对得到的当前上游 ServerId 一致；请求参数或第一次任意登录响应不能静默定义网关身份。
- `tokenHash` 使用 `BYTEA(32)`，不进入 JSON、日志、错误、指标 label 或管理页面；数据库摘要不能作为 Emby Token 重放。
- `embyUserId` 必须等于当前 `users.emby_id`，`userId` 外键指向 Ember 用户；客户端提交的 `UserId`、`DeviceId` 和客户端名称都不能替代这一身份绑定。
- `deviceId/clientName` 只作为设备归组和审计元数据。一个用户允许多个 Token；一个设备可能因重复登录存在多个活动 Token。
- 受保护请求按 [客户端兼容矩阵](./emby-client-compatibility-matrix.md) 收集直接 Token Header、严格应用认证头和大小写不敏感的固定 query Token aliases；所有非空候选必须逐字节相同。重复逻辑来源、空值、冲突、未知 scheme、字段不全和非法 quoted-string 失败关闭；任意 Bearer、Quick Connect 和插件 Token 不作为 Gateway 身份来源。
- `lastSeenAt` 只在成功通过映射和用户资格检查后更新，并至少按 5 分钟窗口限频，避免 `HEAD`、Range 和预加载制造逐请求数据库写入。
- Infuse 扫库中的 `context.Canceled` 返回固定 `499/token_request_canceled`，deadline 返回 `504/token_request_deadline_exceeded`，不再伪装成数据库不可用；只有 driver 保证未发送到 PostgreSQL 的幂等读失败才重试一次，最终存储错误记录固定原因与连接池统计。

Ember 本地撤销固定三种粒度：

| 操作 | 匹配范围 | 语义 |
| --- | --- | --- |
| 单 Token 撤销 | `serverId + tokenHash` | 只使一次登录失效 |
| 单设备撤销 | `serverId + userId + deviceId` 下全部未撤销映射 | 强制该设备重新登录，不影响其他设备 |
| 用户全部撤销 | `userId` 下全部未撤销映射 | 使该用户所有设备重新登录 |

撤销使用 `revokedAt + revokedReason + revokedBy` 软删除并保留审计，不直接删除行。固定原因至少包括 `manual_token_logout`、`manual_device_logout`、`manual_user_logout`、`user_disabled`、`emby_disabled`、`emby_access_disabled`、`emby_unbound`、`user_deleted` 和 `security_revoke`。

状态边界：

- 到期、套餐不允许、并发已满和设备策略拒绝属于动态资格失败，不写 `revokedAt`；续期或策略恢复后原映射可以再次通过。
- 用户停用、`emby_disabled=true`、`emby_access_disabled=true`、解绑或删除属于硬撤销；普通状态恢复不能静默清除历史撤销，重新通过 `AuthenticateByName` 才能建立或重新激活映射。
- “本地撤销”只保证 Ember Playback Gateway 拒绝该映射，不等于 Emby Server 已撤销原始 Token。兼容范围内 Emby 4.9 的原生会话/Token 撤销接口尚未完成版本化核对，因此当前必须标记“未证实”。

控制面联动边界：

- API 使用只依赖 PostgreSQL 的 `ControlPlaneRevoker`，不需要 AccessToken 明文、HMAC 密钥、实时 Emby 请求或 Gateway 进程内 ServerId。
- 设备撤销按 `userId + deviceId` 匹配，并保守撤销该主体所有历史 ServerId 下的活动映射；同 DeviceId 的其他用户不受影响。
- 手工/黑名单设备退出、本地用户停用或恢复、Emby 访问禁用或恢复、绑定前清理、解绑、删除和过期封禁都先完成本地撤销，再执行后续本地状态或 Emby 副作用。
- 本地撤销失败时后续状态与外部副作用不执行；本地撤销成功但 Emby 远端操作失败时不回滚本地安全结果。
- 恢复、重新启用或重新绑定不会清除历史 `revokedAt`，只允许新的成功认证建立新映射。

要让设备强制退出成立，所有受保护的公网 Emby 请求都必须先通过 Token 映射检查，而不是只在 115 视频分支检查。部署时原始 Emby 端口必须只对网关和运维网络开放；否则客户端可以绕过 Ember 使用仍被 Emby 接受的原 Token。首次切换到网关后，历史 Emby Token 没有明文可安全回填，客户端需要重新登录一次建立映射。

截至 2026-08-22，`emby_access_tokens` migration、purpose 隔离 HMAC、并发安全 upsert、实时用户资格解析、三种 Gateway 撤销、控制面状态联动、认证代理和独立进程均已实现并通过 fake/独立 PostgreSQL 测试。目标 Emby 原生设备退出是否撤销 Server Token、公开部署和 Infuse 实机行为仍未证实。

### 3.4 当前 HTTP 门控核心

当前 `internal/playbackgateway` 固定以下可测试行为：

- 固定 OpenAPI 顶层 API family 的 root path 先规范化为单一 `/emby/...`；family 与 `/emby` 前缀比较大小写不敏感，已有规范前缀保持，重复大小写变体 `/emby/emby/...` 返回空体 `400`，根 `/web/...` 和未知 Surface 不参与规范化。
- root 或 `/emby` 形态的 `GET System/Info/Public` 在固定语义段上大小写不敏感且不做本地鉴权；`POST Users/AuthenticateByName`、公开用户和无 Index 公开头像不要求已映射 Token，但仍必须先通过应用/设备授权头。尾斜杠、额外层级和 alternate escaping 不放宽，其余路径默认受保护。
- 认证上游只有 `200` 才旁路解析；编码响应与解码副本检查上限均为 `1 MiB`。identity、gzip 或 deflate 响应逐字节恢复后返回，字段顺序、压缩编码和未知 JSON 字段不重编码。
- 不合法、超过检查上限或映射写入失败的成功响应仍原样返回，但该 Token 不建立映射，下一次受保护请求失败关闭。
- 受保护请求按兼容矩阵取得唯一 AccessToken，再调用 `ResolvePrincipal`；Token 来源缺失、非法、重复、冲突、未映射、已撤销和身份错配返回空体 `401`，当前用户不可用或到期返回空体 `403`，身份存储不可用返回空体 `503`。请求取消和 deadline 分别使用 `499/504`，不计作 Store outage。
- 上游网络和 transport 失败返回空体 `502`。日志只允许固定错误 code 和 Go 错误类型，禁止写入请求 URL、密码、AccessToken、认证响应体或上游原始错误文本。
- 固定 SDK 已确认 `Authorization/X-Emby-Authorization` 的 `Emby` scheme，目标 Infuse `8.5` 已确认 `X-Emby-Authorization: MediaBrowser ...`；兼容矩阵额外接受严格 `X-MediaBrowser-Authorization: MediaBrowser ...`，但不扩展到任意 Header/scheme。真实目标环境同时确认 SystemInfoPublic 无登录可访问，Gateway 对该公开路由记录不含 Header value、URL query 或响应体的上游状态日志。

当前 HTTP 核心还会让大小写兼容的 root PlaybackInfo、视频和普通进度请求复用既有证明、115 `302`、Emby fallback 与默认代理处理器；持久播放会话和 Web Surface 仍未实现。Infuse 登录、普通资源 API、按需 PlaybackInfo、source 映射、转存和 Gateway `302` 已有实机证据，通用载体/大小写矩阵已有 fake 证据；115 CDN 媒体字节、字幕、完整会话以及 SenPlayer、Yamby 等其他客户端仍未实机确认。通用透明代理和 Web Surface 的后续计划见 [Ember Gateway 透明代理与 Web 访问控制实现方案](../plan/architecture/ember-gateway-transparent-proxy-and-web-access.md)。

### 3.5 暂不覆盖的认证方式

首版不根据经验实现以下认证方式：

- Quick Connect
- PIN 登录
- Emby Connect 交换登录
- 其他插件认证入口

如果后续需要支持，必须先把对应版本的 method、path、请求、响应、过期与错误语义补入本文档，再实现 Token 映射。

## 4. 播放信息合同

### 4.1 GET PlaybackInfo

```http
GET /emby/Items/{Id}/PlaybackInfo?UserId={UserId}
X-Emby-Token: <access-token>
```

`4.9.3.0` OpenAPI 声明：

- `Id`：必填 path 参数。
- `UserId`：必填 query 参数。
- 响应模型：`PlaybackInfoResponse`。

Gateway 不把 query `UserId` 当身份来源；只有它唯一存在且等于当前 `ResolvePrincipal.User.EmbyID` 时，成功响应才有资格形成短期证明。缺失或错配请求仍透明交给 Emby 处理，但不会进入直连证明缓存。

### 4.2 POST PlaybackInfo

```http
POST /emby/Items/{Id}/PlaybackInfo
Content-Type: application/json
X-Emby-Token: <access-token>

<PlaybackInfoRequest>
```

响应同样是 `PlaybackInfoResponse`。播放网关不得自行拼装一个缩水版响应；首版应透明转发 Emby 响应，仅在内部读取必要字段。

`PlaybackInfoRequest.UserId` 在 OpenAPI 中可选。Gateway 对不超过 `1 MiB` 的 JSON 请求体做旁路检查：字段缺失允许由 Emby 按原合同处理，非空时必须等于当前 Principal 的 EmbyID；无效、超大或错配请求仍透明转发，但不形成直连证明。

PlaybackInfo 成功响应的编码体与解码旁路副本上限均为 `2 MiB`，支持 `identity/gzip/deflate`（含 zlib-wrapped/raw DEFLATE）。解码失败、超限或未知编码只使本次证明不可用，客户端仍收到原压缩字节、Header 和状态；Gateway 不通过删除 `Accept-Encoding` 换取旁路解析。

### 4.3 用户条目 Container 兼容快照

固定 `4.9.3.0` OpenAPI 的 [`GET /Users/{UserId}/Items/{Id}`](https://github.com/MediaBrowser/Emby.SDK/blob/6ee0155063bc85578196489926359a8f37419502/Resources/OpenApi/openapi_v3.json#L92839-L92889) 返回 `BaseItemDto`；该 DTO 明确包含顶层 `Container` 与 [`MediaSources`](https://github.com/MediaBrowser/Emby.SDK/blob/6ee0155063bc85578196489926359a8f37419502/Resources/OpenApi/openapi_v3.json#L103723-L103755)，MediaSourceInfo 又包含 [`Id/Container/Path/Size/SupportsDirectPlay`](https://github.com/MediaBrowser/Emby.SDK/blob/6ee0155063bc85578196489926359a8f37419502/Resources/OpenApi/openapi_v3.json#L104395-L104471)。

目标 Infuse `8.5` 已实测在视频请求前成功读取用户条目，随后直接请求 `/Videos/{Id}/stream?MediaSourceId=...&Static=true`，不携带 `Container` 或 `PlaySessionId`。而同版本 `/Videos/{Id}/stream` 把 `Container` 声明为必填 query；原始 fallback 因此得到上游 `404`。

Gateway 对层级精确的用户条目 `200 application/json` 响应执行以下兼容观察：

- path UserId 必须等于当前 Principal.EmbyID，响应 `Id` 必须等于 path ItemId。
- 编码体和 `identity/gzip/deflate` 解码副本均限制为 `2 MiB`，客户端仍收到原状态、Header 与压缩字节。
- 优先只缓存 `mappingId + itemId + mediaSourceId -> container`；响应没有任何可用 MediaSource 时才保存同一条目的顶层 Container，禁止用顶层值覆盖一个不匹配的已知 MediaSourceId。不缓存 Token、Path、Size、响应体或用户可见元数据。
- Container 只允许有界小写字母、数字、逗号和连字符；重复 MediaSourceId 使整次响应不写缓存。
- 缓存 TTL 5 分钟、最多 4096 条、无后台 goroutine；每个视频请求仍先重新执行 `ResolvePrincipal`，撤销和用户状态不能被缓存绕过。
- 该快照只用于补齐正常 Emby fallback 的必填 Container，绝不是 PlaybackInfo/PlaySession 授权证明，不能凭它获得 115 `302`。
- 旁路 JSON 解析失败、响应 `Id` 缺失和响应 `Id` 与 path 不一致时，分别记录 `response_json_invalid`、`response_item_id_missing`、`response_item_id_mismatch`；日志带 quoted `mappingId/itemId` 和有界响应元数据，不记录响应体，Emby 原状态/Header/Body 仍保持权威。

### 4.4 PlaybackInfoResponse 关键字段

| 字段 | 类型 | 用途 |
| --- | --- | --- |
| `MediaSources` | `MediaSourceInfo[]` | 确定媒体源、路径、容器和播放能力 |
| `PlaySessionId` | `string` | 关联播放请求、进度事件和网关会话 |
| `ErrorCode` | `PlaybackErrorCode` | Emby 自身播放能力错误 |

`MediaSourceInfo` 至少关注：

| 字段 | 类型 | 用途 |
| --- | --- | --- |
| `Id` | `string` | `MediaSourceId` |
| `ItemId` | `string` | 媒体条目 ID |
| `Path` | `string` | 路径映射和源文件解析 |
| `Container` | `string` | 判断流端点和客户端请求容器 |
| `Size` | `int64` | 与 115 SHA1 一起确认文件身份 |
| `IsRemote` | `bool` | 辅助判断远端媒体，但不能替代路径规则 |
| `SupportsDirectPlay` | `bool` | 判断是否允许直接播放 |
| `SupportsDirectStream` | `bool` | 判断是否可能发生封装转换 |
| `SupportsTranscoding` | `bool` | 判断客户端是否可能发起转码请求 |
| `RequiredHttpHeaders` | `object` | 上游媒体源可能要求的请求头 |
| `DirectStreamUrl` | `string` | Emby 计算出的直接流地址；不能作为 115 文件定位依据，但可在严格限定为当前 Item 的相对视频路径后作为正常 Emby fallback 权威地址 |
| `AddApiKeyToDirectStreamUrl` | `bool` | 客户端是否需要为 DirectStreamUrl 附加 API Key；Gateway 不向 URL 写 Token，统一复用当前映射 Token Header |
| `TranscodingUrl` | `string` | 转码地址；首版 115 直连不改写该链路 |

网关必须使用 `ItemId + MediaSourceId` 作为媒体源缓存主键，不能只按 `ItemId` 假设条目永远只有一个文件。

### 4.5 缺失 PlaySessionId 的按需 PlaybackInfo

目标 Infuse `8.5` 的 plain stream 不带 PlaySessionId；仅从用户条目恢复 Container 后，真实 Emby fallback 仍返回 `404`，证明 Container-only 方案不足。参考 `emby-toolkit` 的 [缺缓存时补取 PlaybackInfo](https://github.com/hbq0405/emby-toolkit/blob/00786dcd632c08a25016276d2531d650ab9ee00c/reverse_proxy.py#L1867-L1885)，Gateway 固定以下用户态补全边界：

- 只处理 plain `/Videos/{Id}/stream`、唯一非空 MediaSourceId、`Static=true`、完全没有 PlaySessionId key 的请求。
- 每次仍先完成 Token 映射和实时用户状态检查；优先复用同一 `mappingId + itemId + mediaSourceId` 下最新、未过期，且 `serverId/userId/embyUserId/deviceId` 与当前 Principal 完全一致的现有 PlaybackInfo 证明。身份不一致时重新向 Emby 解析，不能把旧 PlaySessionId 拼进 fallback。
- 无可复用证明时，Gateway 使用当前请求已经归一的用户 AccessToken，对同一 Emby 上游执行 `GET /emby/Items/{Id}/PlaybackInfo?UserId={Principal.EmbyID}`。不使用部署管理员 API Key，不把 Token 放入内部 URL，也不调用 Gateway 自身公网入口。
- 保留合法 Emby/MediaBrowser 应用头和客户端/设备元数据；只有原请求没有可用内嵌 Token 时，内部请求才使用 `X-Emby-Token` 携带同一个已映射 Token。内部请求不继承客户端任意压缩声明，只广告 Gateway 可有界解码的 `gzip, deflate`。
- 单次内部调用固定 10 秒超时；相同 mapping/item/mediaSource 的并发请求使用进程内 singleflight 合并。每个等待方可独立取消，resolver 使用自有超时，panic 转为固定 `internal_failure`。
- 只接受无重定向 `200 application/json`、`identity/gzip/deflate`、非空有界 PlaySessionId、无重复 MediaSourceId，以及与请求 item/source 精确匹配且 Container 合法的 MediaSource。
- 合格 DirectPlay MediaSource 继续通过现有 `buildPlaybackProofs` 写入原证明缓存；Path、Container、SupportsDirectPlay 或身份合同不合格时仍可用 PlaySessionId+Container 修复正常 Emby fallback，但不能获得 115 证明。Emby Size 只作为观察字段，零、缺失、负数或与 Provider 不一致都不影响 proof。
- Gateway 将 115 决策请求与正常 Emby fallback 请求分开：决策请求只追加缺失的 Container/PlaySessionId；fallback 优先使用所选 MediaSource 的 `SupportsDirectStream=true + DirectStreamUrl`。
- DirectStreamUrl 只接受无 scheme/host/user/fragment、当前 Item、固定 `/Videos/{Id}/stream[.{Container}]` 或文件名形态、匹配的 MediaSourceId/PlaySessionId/Static/Container。全部 URL Token aliases 在转发前删除，并由当前已映射用户 Token Header 替代；未知 Item、绝对 URL、编码 path、重复/错配参数和 manifest 全部拒绝采用。
- DirectStreamUrl 缺失或未通过校验时，按 Emby 官方 Web 客户端行为把 plain stream 改为 `/Videos/{Id}/stream.{Container}`；无法形成单一安全扩展名时才保留补齐参数后的 plain stream。
- 权威 fallback 保留原 method、Range、应用认证 Header 和非播放身份 query；DirectStreamUrl 自己的 MediaSourceId/PlaySessionId/Container/Static 不被不完整客户端参数覆盖。
- 补全后重新进入现有视频决策：证明和 115 条件齐全则可 `302`；115 未配置、不适用或失败时，也必须使用独立的权威 fallback 请求代理 Emby。resolver 失败时才保持原请求（或已有条目 Container fallback），不伪造成功。
- Info 记录 `playback_info_resolved_on_demand`、`playback_info_resolve_failed`、`fallbackSource`、mappingId、itemId、proofCount、固定 reason 和上游 status。响应级合同成立后，每个唯一有效 MediaSource ID 都记录 `code=playback_info_media_source_observed`：quoted `mediaPath`、`pathPresent/pathTruncated`、`sizePresent/size`、DirectPlay/DirectStream 能力、`proofAccepted` 与固定 `proofRejectReason`；不再依赖 proof 写入成功。高频 `playback_info_reused_on_demand` 只在 Debug 输出。所有级别仍禁止 Token、UserId query value、完整响应体或完整 URL。

### 4.6 单实例短期授权证明

首期不为已经携带完整播放上下文的请求重复调用 PlaybackInfo，也不为证明或媒体快照建表。Gateway 正常代理客户端 PlaybackInfo，并只在 4.5 的缺 PlaySessionId 形态下按需补取；两条路径都在上游成功响应后执行同一套证明记录：

```text
透明返回原始响应
  + 在进程内记录短期证明和 MediaSource 快照
```

缓存键固定为：

```text
mappingId + itemId + mediaSourceId + playSessionId
```

缓存值包含当前 Principal 的 `userId/embyUserId/serverId/deviceId/clientName`，以及本次响应的 `path/size/container/direct-play` 能力。约束：

- 只有 `200 application/json`、空 `ErrorCode`、非空有界 `PlaySessionId` 和至少一个合格 MediaSource 才记录。
- 请求 path `ItemId` 是条目真相；MediaSource.ItemId 可空，非空时必须与 path 一致。
- MediaSource 必须具备唯一非空 Id、非空有界 Path 和 `SupportsDirectPlay=true`；重复 MediaSourceId 使整次响应不产生证明。Size 不参与 proof，115 候选归属由 Path 的 source 前缀映射决定。
- 固定 TTL 为 5 分钟，最大 4096 条；写入和查询都延迟清理过期项，满载时淘汰最早过期项，不启动后台 goroutine。
- 每个有资格形成证明的新版 PlaybackInfo 响应都会先清除相同 `mappingId + itemId` 的旧证明；非 `200`、错误或不可用响应不能继续复用旧成功结果。
- 视频请求仍必须先重新执行 `ResolvePrincipal`，再用完全相同的 mapping/item/mediaSource/playSession 查询；缓存不能绕过撤销或用户实时状态。
- 进程重启会丢失证明；此时视频请求不能获得 115 302，但应 fallback 到 Emby，Infuse 再次调用 PlaybackInfo 后重建证明。多 Gateway、副本共享和持久播放会话推迟到后续阶段。
- Token 和完整 PlaybackInfo 响应不进入日志；完整 Path 按运维授权进入上述 MediaSource 观察与最终视频决策日志。缓存对象只存在于 Gateway 进程内，不序列化为 API。
- 响应无效、过大、解析失败或没有合格 MediaSource 时，Emby 原始响应仍逐字节返回，只是不产生证明。

MediaSource 观察日志的 proof 结果固定为 `proofAccepted=true + proofRejectReason=none`，或 `proofAccepted=false` 搭配 `identity_invalid`、`item_mismatch`、`path_missing`、`path_invalid`、`container_invalid`、`direct_play_unsupported`。`sizePresent/size` 只说明 Emby 观察值，不产生 proof 拒绝原因。合法 Path 完整记录；超过 proof 上限的异常 Path 使用 `pathTruncated=true` 有界记录。

## 5. 原始视频流合同

`4.9.3.0` OpenAPI 明确列出以下视频流入口：

| Method | Path | 处理策略 |
| --- | --- | --- |
| `GET`, `HEAD` | `/emby/Videos/{Id}/stream` | 满足直连条件时返回 302，否则透明转发 Emby |
| `GET`, `HEAD` | `/emby/Videos/{Id}/stream.{Container}` | 同上，`Container` 是 path 参数 |
| `GET`, `HEAD` | `/emby/Videos/{Id}/{StreamFileName}` | 同上，保留客户端请求文件名 |
| `GET` | `/emby/Items/{Id}/Download` | 必须额外执行 `EnableContentDownloading` 和网关下载策略 |

`/Videos/{Id}/stream` 的 `Container` 是必填 query 参数；其余常见参数包括 `DeviceId`、`Static`、`StartTimeTicks`、音视频编码、码率、分辨率和字幕流索引。

处理约束：

- 不能看到 `/Videos/` 就一律返回 302；必须先判断 Token、用户状态、媒体源、路径规则和 Direct Play 能力。
- 首期只有 query 同时提供唯一非空 `MediaSourceId`、唯一非空 `PlaySessionId` 和精确 `Static=true` 时才尝试 115；`/stream` 还必须提供唯一非空 `Container`。缺失、重复或其他值均透明 fallback Emby。
- plain `/stream` 缺 PlaySessionId 时先按 4.5 补取权威 PlaybackInfo。只有 resolver 失败、请求也完全没有 Container key、且同一 mapping/item/mediaSource 有近期用户条目快照时，才克隆 Emby fallback 并追加有界 Container；该降级分支固定 `container_recovered` 且不尝试 115。
- 按需 PlaybackInfo 成功后，115 决策使用补齐参数的请求；任何 115 fallback 使用 4.5 的 DirectStreamUrl/扩展名权威 Emby 请求。二者不能共用同一个 URL，否则本地视频会继续继承客户端 plain stream 缺口。
- 没有可用 Container 快照、客户端已经提交任意大小写的 Container key、快照过期或响应歧义时不猜测容器，继续使用原请求 fallback。
- `stream.{Container}` 和 `{StreamFileName}` 从最后一个扩展名取得容器并与 PlaybackInfo 证明中的 Container 核对；不一致时 fallback Emby。`m3u8`、`mpd`、`m4s` 明确不进入 115 编排。
- `GET` 与 `HEAD` 都可能由客户端用于探测，不得把每个请求都计为独立播放会话。
- 客户端可能在获得 302 后向 115 CDN 发出 `Range` 请求；网关必须通过客户端合同测试确认重定向、UA 和 Range 行为。
- `/Items/{Id}/Download` 与播放流不是同一权益，必须复用套餐下载开关并单独审计。
- 首版不改写 HLS、DASH、转码 manifest 和转码分片接口；这些请求继续透明转发 Emby，保持正常播放和转码能力。

## 6. 字幕合同

`4.9.3.0` OpenAPI 列出以下字幕流入口，均支持 `GET` 和 `HEAD`：

```text
/emby/Items/{Id}/{MediaSourceId}/Subtitles/{Index}/Stream.{Format}
/emby/Videos/{Id}/{MediaSourceId}/Subtitles/{Index}/Stream.{Format}
/emby/Items/{Id}/{MediaSourceId}/Subtitles/{Index}/{StartPositionTicks}/Stream.{Format}
/emby/Videos/{Id}/{MediaSourceId}/Subtitles/{Index}/{StartPositionTicks}/Stream.{Format}
```

首版处理策略：

- 字幕请求透明转发给 Emby Server。
- 不能把字幕请求计入视频播放并发。
- 如果后续媒体源只存在于 115 且 Emby 无法提供外挂字幕，应另行补充字幕来源合同，不能在视频 302 逻辑中临时拼接。

## 7. 播放会话合同

### 7.1 开始播放

```http
POST /emby/Sessions/Playing
Content-Type: application/json

<PlaybackStartInfo>
```

### 7.2 播放进度

```http
POST /emby/Sessions/Playing/Progress
Content-Type: application/json

<PlaybackProgressInfo>
```

### 7.3 停止播放

```http
POST /emby/Sessions/Playing/Stopped
Content-Type: application/json

<PlaybackStopInfo>
```

上述 DTO 共同关注：

- `SessionId`
- `PlaySessionId`
- `ItemId`
- `MediaSourceId`
- `PositionTicks`

开始和进度 DTO 还包含 `PlayMethod`、`IsPaused`、音轨和字幕流索引等字段；停止 DTO 还包含 `Failed` 和 `IsAutomated`。

网关处理要求：

- 三类会话请求必须继续转发给 Emby，保持播放历史和进度能力。
- 网关可以旁路观察事件，更新 `direct_play_sessions`，但不能篡改客户端上报内容。
- 并发统计以 `PlaySessionId + Ember 用户 + 设备` 为主要维度，并使用 TTL 处理客户端未上报停止事件的情况。
- `HEAD`、重复 `Range` 和预加载请求不能单独创建新的活跃播放会话。

## 8. 身份与访问策略

网关在签发 302 前必须同时检查：

- AccessToken 哈希存在且未撤销。
- Token 绑定的 `ServerId` 与当前 Emby Server 一致。
- Ember 用户存在，且 `emby_id` 与 Token 用户一致。
- `is_active = true`。
- 用户未过期。
- `emby_disabled = false`。
- `emby_access_disabled = false`。
- 客户端未命中黑名单。
- 套餐允许直连播放，且未超过网关并发限制。
- 下载接口额外满足内容下载权限。
- 相同 Token 最近一次 `PlaybackInfo` 已被当前 Emby Server 成功接受，且 ItemId、MediaSourceId、PlaySessionId 与本次直连请求一致。

不能仅依赖 Emby 自身 `SimultaneousStreamLimit`，因为 302 后视频字节不再经过 Emby，网关需要维护自己的会话状态。

Token 映射只证明“该 Token 曾由该 Server 签发给该 Emby 用户”，不能单独证明 Token 此刻仍被 Emby 接受。首期必须把近期成功的 GET/POST `PlaybackInfo` 作为 115 加速授权证据；缺少证明的合法请求不能获得 302，但仍 fallback 到 Emby。Token 被撤销或用户状态变化后拒绝所有受保护请求并清除对应直链缓存和未签发会话；已经建立的 115 CDN 连接只能等到断线、重连或链接过期。

## 9. 失败与回退语义

- Token 缺失、重复、无法映射、已本地撤销、身份错配或用户硬状态拒绝：返回固定拒绝响应，不能继续转发 Emby，否则会绕过 Ember 本地安全门控。
- 请求已取消或 deadline：分别记录 `499/504`，不误报 Store outage；真实身份存储不可用仍拒绝，不能降级成未受控代理。
- Principal 合法但没有近期 PlaybackInfo、证明过期、视频参数不完整或 MediaSource 不支持 Direct Play：不尝试 115 或停止尝试，透明转发原始请求到 Emby。
- 路径未命中 115 source、套餐未启用加速、客户端不适合直连、账号未配置/未验证/冷却、查重/Range challenge/秒传/目标复核失败：透明转发原始请求到 Emby。
- 直链域名、过期时间、HeaderMode 或并发信息不兼容：不返回 302，透明转发原始请求到 Emby。
- fallback 必须保留客户端原始 method、path、query、Range、User-Agent、`X-Emby-Token`、应用认证 Header 和其他 Emby Header，不能重新拼装缩水版视频请求。
- fallback 只允许使用 Emby 正常代理，禁止改用 source 账号向客户端签发 115 直链。
- 已发出 302 后用户被禁用：阻止后续直链和 Token 使用，但不保证立即切断已建立的 CDN 连接。
- Emby 会话事件转发失败：记录失败并允许网关会话 TTL 收口，不能伪造成功。
- `LOG_LEVEL=debug` 时，每个经过 Gateway Handler 的请求收尾写一条 `code=request_completed` 脱敏摘要：记录有界 method/Host/原始 path、query key 名称/数量、route、pathMode、statusCode、success/failure、耗时、直接 Token Header 数量、应用头 scheme/Token presence、query Token source 数量/状态、已知 User-Agent family/version。默认 `info` 不逐请求打印该详细摘要；任何级别都不得记录 query value、Header 原值、Cookie、Token 或 Authorization 内容。
- 每个视频请求在默认 Info 额外只写一条最终决策日志，并把人工可读结论放在行首：直链成功使用 `code=direct_play_redirect message="115直链成功" result=success statusCode=302 target=p115 targetState=created|reused`；DirectPlay 失败使用 `code=direct_play_fallback message="115直链失败，Emby回退成功|失败" directPlayResult=failure fallbackResult=success|failure`；其他 fallback 和 reject 分别使用 `code=playback_fallback`、`code=playback_rejected`。全部继续记录 `decision=redirect|fallback|reject`、固定 `stage/reasonCode/fallbackSource` 和必要 ID/耗时；进入 DirectPlay 后还记录 quoted `mediaPath/embyPathPrefix/sourceRootId/mappedRelativePath`。Debug 不重复生成第二条决策，日志不建表、不进入数据库。
- 完整媒体 Path 已按运维排障需求明确允许进入持久日志；仍禁止记录 Token、Cookie、完整 SHA1、115 URL、PlaybackInfo 原始响应、Provider 原始错误或 Emby 代理原始错误。
- Provider 失败只允许补充固定 `providerOperation=resolve_source_path|hash_source_preid|rapid_upload|hash_source_challenge|rapid_upload_retry|verify_playback_target|search_playback_target|get_download_url`；账号加载失败只允许补充 `accountRole=source|playback`。未知诊断值必须丢弃，不能进入日志。

首期决策日志枚举固定如下；实现可以在同一 `reasonCode` 下补充脱敏上下文字段，但不能把原始错误字符串当作新枚举：

| decision | stage | reasonCode |
| --- | --- | --- |
| `reject` | `identity` | `token_missing`、`token_invalid`、`token_ambiguous`、`token_unmapped`、`token_revoked`、`identity_mismatch`、`identity_store_unavailable`、`request_canceled`、`request_deadline_exceeded` |
| `reject` | `user_state` | `user_unavailable`、`user_expired` |
| `fallback` | `route` | `route_not_accelerated`、`request_not_eligible` |
| `fallback` | `proof` | `playback_proof_missing`、`playback_proof_expired`、`playback_proof_mismatch` |
| `fallback` | `eligibility` | `direct_play_disabled`、`client_incompatible`、`concurrency_limited`、`media_not_direct_play` |
| `fallback` | `direct_play` | `invalid_request`、`path_not_mapped`、`account_unavailable`、`accounts_same`、`provider_unavailable`、`provider_protocol`、`rapid_upload_unavailable`、`target_unavailable`、`download_incompatible`、`store_unavailable`、`lock_unavailable` |
| `redirect` | `direct_play` | `direct_play_ready` |

`fallback` 决策的 `fallbackSource` 只允许固定值：`client_request`、`container_recovered`、`playback_info_direct_stream`、`playback_info_extension_stream`、`playback_info_augmented_stream`；redirect/reject 留空。它只描述交给 Emby 的请求来源，不包含 URL、Container、Token 或媒体路径。

所有决策日志记录 `statusCode`；fallback 以最终 Emby 状态明确生成 `fallbackResult=success|failure`，已取得 Emby 响应时再记录 `upstreamStatus`，代理传输失败只记录固定 `proxyErrorCode`，禁止输出上游原始错误。redirect 不打印空 `fallbackSource`、零值 `upstreamStatus` 或空 `proxyErrorCode`，其他结果也省略无意义的空 task/映射字段。该日志描述 Gateway 对本次请求选择的路径和 HTTP 结果；`302` 仍只证明 Gateway 已返回重定向，不证明客户端已从 115 CDN 读取媒体字节。

## 10. 合同测试要求

实现前后至少覆盖：

1. `AuthenticateByName` 响应透明转发、HMAC-SHA256 映射、明文不落库，以及映射写入失败不篡改原响应。
2. GET/POST `PlaybackInfo` 的请求和关键响应字段夹具。
3. 三类原始视频流路径的 `GET`、`HEAD`、query、302、Emby fallback 和安全 reject 行为。
4. 下载接口与普通播放权限分离。
5. 字幕接口不被视频拦截器误判。
6. Playing、Progress、Stopped 事件继续到达 fake Emby。
7. 重复 `HEAD`、Range 和重连不会重复计并发。
8. Quick Connect 等未覆盖入口不会被错误当作已支持。
9. Debug 请求完成日志覆盖上游成功与本地拒绝，能区分 `X-Emby-Token` 缺失/空值/存在/歧义和应用认证头内嵌 Token 状态；Info 默认不输出该详细摘要，两种级别都不包含任何凭证或 query value。
10. Infuse 内嵌 Token、`X-Emby/X-MediaBrowser` Token Header、固定 query aliases、大小写变体、多来源同值、冲突、重复、空值和非法应用头保持上述接受/拒绝语义。
11. 单 Token、单设备和用户全部撤销只影响各自范围；硬撤销后请求拒绝，动态到期/套餐拒绝不错误写入 `revokedAt`。
12. `lastSeenAt` 限频、ServerId/EmbyID 错配拒绝、并发登录幂等 upsert，以及数据库/日志/JSON 均不包含 Token 明文。
13. 302 要求相同 Token 的近期成功 PlaybackInfo；缺少/过期证明时合法请求 fallback Emby，撤销后缓存重用和原始 Emby 公网绕过仍失败关闭。
14. 每个视频请求只产生一条脱敏 `redirect/fallback/reject` 日志；普通 fallback 保持原始请求，按需兼容分支只允许按 4.5 重建权威路径，同时保持 method、Range、应用 Header 和非身份参数语义。
15. 三种原始视频路径只有完整静态播放参数和匹配 Container 时调用 fake DirectPlay；manifest、参数缺失、无效候选和所有类型化 DirectPlay 错误均进入 fake Emby fallback。
16. 特殊 path 与 Gateway 消费的 query key 大小写不敏感但不重写原始请求；尾斜杠、额外层级、duplicate logical key 和 alternate escaping 继续失败关闭。
17. PlaybackInfo 含 `MediaStreams: []` 时客户端响应逐字节保持，SenPlayer/Yamby 等 UA 不改变认证或代理路径。
18. Store 请求取消/deadline、一次安全只读重试、非重试 PostgreSQL 错误和最终连接池诊断均有固定测试。
19. 用户条目 identity/gzip/deflate 响应逐字节保持；按需 PlaybackInfo 失败、仅由条目 Container 恢复 plain stream fallback 时，DirectPlay 调用次数必须为零。
20. 缺 PlaySessionId 的 plain stream 使用当前用户 Token 补取 PlaybackInfo；压缩响应、source/item 错配、重复 source、非法 Container、上游非 200、取消/超时、singleflight 和 proof 复用均有 fake 测试，内部 URL 与日志不含 Token。
21. 115 不可用时，合法 DirectStreamUrl fallback 返回 fake Emby `200/206`；绝对 URL、错 Item/source/session/container、encoded path、manifest 被拒绝，URL Token 被删除并替换为当前用户 Header；DirectStreamUrl 缺失时固定使用 `stream.{Container}`。

所有测试必须使用 fake Emby Server 或固定 fixture，禁止请求真实 Emby。

## 11. 待实机确认清单

真实只读验证需要用户明确授权，至少确认：

- 目标 Emby Server 的 `ServerName` 和完整可公开身份边界；Version 与非空 Id 已确认。
- Infuse PlaybackInfo、视频、字幕和进度的实际 path/query/Token 组合；登录和普通资源路径已确认。
- SenPlayer、Yamby、VidHub、Fileball、Conflux 与官方 Emby 客户端实际使用的 Token carriers、大小写和 DeviceId/客户端名称稳定性。
- Infuse 对 `302`、`HEAD`、`Range`、UA 和文件名的处理。
- 客户端是否始终携带 `PlaySessionId`、`MediaSourceId` 和设备标识。
- Direct Play、Direct Stream 与 Transcode 三种情况下的实际请求差异。
- 目标 Emby 4.9 实例是否提供可安全调用的单 Token、单设备或会话撤销接口；确认前只能宣称 Ember 网关本地撤销。

以上未确认项不能作为实现完成的依据。
