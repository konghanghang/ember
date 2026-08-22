# Emby 4.9.3.0 播放代理 API 合同

本文档记录 Ember 播放网关依赖的 Emby 原生认证、播放信息、原始视频流、字幕和播放会话接口。目标是为后续 115 直连播放提供固定版本证据，禁止根据客户端表现或其他 Emby 版本经验猜测协议。

## 1. 适用范围与证据等级

当前兼容基线：

| 组件 | 已确认版本 | 证据 | 结论 |
| --- | --- | --- | --- |
| Emby Server / SDK | `4.9.3.0 Release` | Emby.SDK 提交 `6ee0155063bc85578196489926359a8f37419502` | 本文列出的 method、path 和 DTO 字段按该提交确认 |
| 目标 Emby Server | 未确认 | 尚未调用目标实例 `GET /emby/System/Info` | 不能断言目标实例与本文基线完全一致 |
| Infuse 客户端行为 | 未确认 | 尚未对目标版本 Infuse 做合同夹具或真实只读验证 | 不能断言所有请求顺序、重定向和 Header 行为已经覆盖 |

证据等级：

- **固定版本源码确认**：由 Emby `4.9.3.0` SDK OpenAPI 直接证明。
- **HTTP 传输要求**：由 HTTP 标准和反向代理边界决定，但仍需客户端合同测试锁定。
- **未实机确认**：OpenAPI 允许该用法，但尚未在目标 Emby 与 Infuse 组合上验证。

固定版本证据：

- [Emby.SDK 4.9.3.0 OpenAPI](https://github.com/MediaBrowser/Emby.SDK/blob/6ee0155063bc85578196489926359a8f37419502/Resources/OpenApi/openapi_v3.json)
- [Emby.SDK 4.9.3.0 User Authentication](https://github.com/MediaBrowser/Emby.SDK/blob/6ee0155063bc85578196489926359a8f37419502/Documentation/doc/restapi/User-Authentication.html)
- [Emby.SDK 4.9.3.0 Password Authenticator](https://github.com/MediaBrowser/Emby.SDK/blob/6ee0155063bc85578196489926359a8f37419502/SampleCode/RestApi/Emby.ApiClient/Emby.ApiClient/Client/Authentication/EmbyPasswordAuthenticator.cs)
- [Emby.SDK 4.9.3.0 SystemInfoPublic](https://github.com/MediaBrowser/Emby.SDK/blob/6ee0155063bc85578196489926359a8f37419502/Documentation/reference/RestAPI/SystemService/getSystemInfoPublic.html)
- [Ember Playback Reporting 合同](./playback-reporting-api-contract.md)

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

### 2.1 启动期上游身份核对

固定 `4.9.3.0` OpenAPI 声明：

```http
GET /emby/System/Info
X-Emby-Token: <admin-api-key>
```

成功响应是 `SystemInfo`，当前启动边界只读取：

| 字段 | 类型 | 用途 |
| --- | --- | --- |
| `Id` | `string` | 固定当前上游 ServerId，并传给 `EmbyTokenService` |
| `Version` | `string` | 必须精确等于当前兼容基线 `4.9.3.0` |
| `ServerName` | `string` | 可选的脱敏启动诊断，不参与身份判断 |

运行要求：

- Gateway 只能在身份核对成功后进入监听状态；不能使用第一次任意登录响应静默定义 ServerId。
- `Id` 必须非空且满足长度/控制字符边界，`Version` 必须精确匹配；`ServerName` 可空但必须有界。
- 上游重定向、非 `200`、非 JSON、响应超过 `256 KiB`、字段缺失、版本不符、超时或取消全部失败关闭。
- API Key 只存在于请求 Header，禁止进入错误、日志、响应、指标或持久化；上游 URL 和完整响应体同样不得进入错误或日志。
- 自动化只使用 fake Emby；目标实例完成显式只读验证前，仍不能宣称其版本与 ServerId 已确认。

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

首期解析约束：

- 两个 Header 只能出现一个且只能有一个值；同时出现、重复值或空值都失败关闭。
- scheme 固定为大小写敏感的 `Emby`；要求唯一的 `Client`、`Device`、`DeviceId` 和 `Version`，`UserId` 可空。
- 登录前 `Token` 只允许缺失或空字符串；非空 Token 不能替代已经固定的 `X-Emby-Token` 门控。
- 值使用有界 quoted-string；重复字段、未知字段、控制字符、非法转义和超长值全部拒绝。
- `Client` 保存为非权威 `clientName`，`DeviceId` 保存为非权威 `deviceId`；二者只用于审计和设备撤销，不能替代 `User.Id + ServerId + AccessToken` 身份绑定。

网关处理要求：

1. 先验证应用/设备授权头，再将认证请求和响应透明转发，不修改 Emby 返回体。
2. 成功响应后提取 `User.Id`、`AccessToken` 和 `ServerId`。
3. 使用从 `CONFIG_ENCRYPTION_KEY` 按 `emby-access-token` purpose 派生的密钥计算 HMAC-SHA256；数据库只保存 32 字节摘要，不保存明文。
4. 根据 `users.emby_id` 查找 Ember 用户，并记录设备、客户端和最后访问时间。
5. 映射写入失败不能篡改 Emby 已成功的认证响应；该 Token 保持未映射，后续受保护请求和直连失败关闭。
6. 用户过期只做动态资格拒绝，不立即硬撤销映射；用户停用、Emby 访问禁用、Emby 账号解绑或删除时硬撤销。已发出的短期 CDN 链接不保证可以立即终止。

### 3.2 登录前 bootstrap

固定 `4.9.3.0` User Authentication 文档明确把以下调用放在用户认证之前：

| Method | Path | 用途 | 首期网关处理 |
| --- | --- | --- | --- |
| `GET` | `/emby/Users/Public` | 获取允许显示在登录页的公开用户 | 验证应用/设备授权头后透明转发 |
| `GET`, `HEAD` | `/emby/Users/{Id}/Images/{Type}` | 可选公开用户头像 | 验证应用/设备授权头和精确路径形态后透明转发 |

边界：

- bootstrap 表示“不要求已映射 AccessToken”，不表示任意匿名请求；仍必须携带上节固定的应用/设备授权头。
- public 用户头像只放行文档明确引用的无 `Index` 形态；上传、删除、`/Delete`、带额外 path segment 或其他用户接口仍受 Token 门控。
- `/emby/System/Info/Public` 虽返回 `PublicSystemInfo`，但固定提交的参考页明确标记 `Requires authentication as user`，当前不能因路径名包含 `Public` 就加入 bootstrap allowlist。
- Branding、服务器发现、Quick Connect 和其他登录前路径没有进入本次固定证据，继续失败关闭；目标 Infuse 实际请求到这些路径时，必须先补合同。

### 3.3 Token 哈希映射与本地撤销

`emby_access_tokens` 是 Emby 身份到 Ember 用户的桥接，不是新的 Token，也不代替 Emby 验证：

- 映射主键语义为 `ServerId + HMAC-SHA256(AccessToken)`；同一 Server/Token 重复认证必须幂等 upsert。
- 认证响应的 `ServerId` 必须与网关启动期版本化核对得到的当前上游 ServerId 一致；请求参数或第一次任意登录响应不能静默定义网关身份。
- `tokenHash` 使用 `BYTEA(32)`，不进入 JSON、日志、错误、指标 label 或管理页面；数据库摘要不能作为 Emby Token 重放。
- `embyUserId` 必须等于当前 `users.emby_id`，`userId` 外键指向 Ember 用户；客户端提交的 `UserId`、`DeviceId` 和客户端名称都不能替代这一身份绑定。
- `deviceId/clientName` 只作为设备归组和审计元数据。一个用户允许多个 Token；一个设备可能因重复登录存在多个活动 Token。
- 当前固定合同只确认 `X-Emby-Token`。query token、`Authorization` 的其他格式、Quick Connect 或插件 Token 在实机确认并补合同前不得猜测式提取。
- `lastSeenAt` 只在成功通过映射和用户资格检查后更新，并至少按 5 分钟窗口限频，避免 `HEAD`、Range 和预加载制造逐请求数据库写入。

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
- “本地撤销”只保证 Ember Playback Gateway 拒绝该映射，不等于 Emby Server 已撤销原始 Token。Emby 4.9.3.0 原生会话/Token 撤销接口尚未完成版本化核对，因此当前必须标记“未证实”。

控制面联动边界：

- API 使用只依赖 PostgreSQL 的 `ControlPlaneRevoker`，不需要 AccessToken 明文、HMAC 密钥、实时 Emby 请求或 Gateway 进程内 ServerId。
- 设备撤销按 `userId + deviceId` 匹配，并保守撤销该主体所有历史 ServerId 下的活动映射；同 DeviceId 的其他用户不受影响。
- 手工/黑名单设备退出、本地用户停用或恢复、Emby 访问禁用或恢复、绑定前清理、解绑、删除和过期封禁都先完成本地撤销，再执行后续本地状态或 Emby 副作用。
- 本地撤销失败时后续状态与外部副作用不执行；本地撤销成功但 Emby 远端操作失败时不回滚本地安全结果。
- 恢复、重新启用或重新绑定不会清除历史 `revokedAt`，只允许新的成功认证建立新映射。

要让设备强制退出成立，所有受保护的公网 Emby 请求都必须先通过 Token 映射检查，而不是只在 115 视频分支检查。部署时原始 Emby 端口必须只对网关和运维网络开放；否则客户端可以绕过 Ember 使用仍被 Emby 接受的原 Token。首次切换到网关后，历史 Emby Token 没有明文可安全回填，客户端需要重新登录一次建立映射。

截至 2026-08-22，`emby_access_tokens` migration、purpose 隔离 HMAC、并发安全 upsert、实时用户资格解析、三种 Gateway 撤销、控制面状态联动、认证代理和独立进程均已实现并通过 fake/独立 PostgreSQL 测试。目标 Emby 原生设备退出是否撤销 Server Token、公开部署和 Infuse 实机行为仍未证实。

### 3.4 当前 HTTP 门控核心

在独立网关进程和部署入口落地前，当前 `internal/playbackgateway` 固定以下可测试行为：

- 精确 `POST /emby/Users/AuthenticateByName` 和上节固定的 bootstrap 路由不要求已映射 Token，但必须先通过应用/设备授权头；其余路径默认受保护。
- 认证上游只有 `200` 才旁路解析；响应检查上限为 `1 MiB`。合法响应逐字节恢复后返回，字段顺序、空白和未知字段不重编码。
- 不合法、超过检查上限或映射写入失败的成功响应仍原样返回，但该 Token 不建立映射，下一次受保护请求失败关闭。
- 受保护请求只接受唯一的 `X-Emby-Token`。缺失、重复、未映射、已撤销和身份错配返回空体 `401`；当前用户不可用或到期返回空体 `403`；身份存储不可用返回空体 `503`。
- 上游网络和 transport 失败返回空体 `502`。日志只允许固定错误 code 和 Go 错误类型，禁止写入请求 URL、密码、AccessToken、认证响应体或上游原始错误文本。
- 固定 SDK 已确认标准应用/设备授权头及 `/Users/Public` 登录流程；目标 Infuse 是否实际使用同一 Header 名、scheme、字段和请求顺序仍未实机确认。`System/Info/Public` 等未进入 allowlist 的路径继续失败关闭。

当前 HTTP 核心还会观察经自身代理的 PlaybackInfo 并生成进程内短期证明，但仍不包含视频路由、302 或持久播放会话，不表示目标 Emby/Infuse 已可用。

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

### 4.3 PlaybackInfoResponse 关键字段

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
| `DirectStreamUrl` | `string` | Emby 计算出的直接流地址，不能未经验证直接信任为 115 文件定位依据 |
| `TranscodingUrl` | `string` | 转码地址；首版 115 直连不改写该链路 |

网关必须使用 `ItemId + MediaSourceId` 作为媒体源缓存主键，不能只按 `ItemId` 假设条目永远只有一个文件。

### 4.4 单实例短期授权证明

首期不重复调用 PlaybackInfo，也不为证明或媒体快照建表。Gateway 已经代理 Infuse 的 PlaybackInfo，因此在上游成功响应经过时同时执行：

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
- MediaSource 必须具备唯一非空 Id、非空有界 Path、正数 Size 和 `SupportsDirectPlay=true`；重复 MediaSourceId 使整次响应不产生证明。
- 固定 TTL 为 5 分钟，最大 4096 条；写入和查询都延迟清理过期项，满载时淘汰最早过期项，不启动后台 goroutine。
- 每个有资格形成证明的新版 PlaybackInfo 响应都会先清除相同 `mappingId + itemId` 的旧证明；非 `200`、错误或不可用响应不能继续复用旧成功结果。
- 视频请求仍必须先重新执行 `ResolvePrincipal`，再用完全相同的 mapping/item/mediaSource/playSession 查询；缓存不能绕过撤销或用户实时状态。
- 进程重启会丢失证明；此时视频请求不能获得 115 302，但应 fallback 到 Emby，Infuse 再次调用 PlaybackInfo 后重建证明。多 Gateway、副本共享和持久播放会话推迟到后续阶段。
- Token、完整 PlaybackInfo 响应和 Path 不进入日志；缓存对象只存在于 Gateway 进程内，不序列化为 API。
- 响应无效、过大、解析失败或没有合格 MediaSource 时，Emby 原始响应仍逐字节返回，只是不产生证明。

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
- 身份存储不可用：拒绝，不把基础设施故障降级成未受控代理。
- Principal 合法但没有近期 PlaybackInfo、证明过期、视频参数不完整或 MediaSource 不支持 Direct Play：不尝试 115 或停止尝试，透明转发原始请求到 Emby。
- 路径未命中 115 source、套餐未启用加速、客户端不适合直连、账号未配置/未验证/冷却、查重/Range challenge/秒传/目标复核失败：透明转发原始请求到 Emby。
- 直链域名、过期时间、HeaderMode 或并发信息不兼容：不返回 302，透明转发原始请求到 Emby。
- fallback 必须保留客户端原始 method、path、query、Range、User-Agent、`X-Emby-Token` 和其他 Emby Header，不能重新拼装缩水版视频请求。
- fallback 只允许使用 Emby 正常代理，禁止改用 source 账号向客户端签发 115 直链。
- 已发出 302 后用户被禁用：阻止后续直链和 Token 使用，但不保证立即切断已建立的 CDN 连接。
- Emby 会话事件转发失败：记录失败并允许网关会话 TTL 收口，不能伪造成功。
- 每个视频请求只写一条最终决策日志：`decision=redirect|fallback|reject`，同时记录固定 `stage/reasonCode` 和必要 ID/耗时；日志不建表、不进入数据库。
- 决策日志禁止记录 Token、Cookie、完整 Path、完整 SHA1、115 URL、PlaybackInfo 原始响应或 Provider 原始错误。

首期决策日志枚举固定如下；实现可以在同一 `reasonCode` 下补充脱敏上下文字段，但不能把原始错误字符串当作新枚举：

| decision | stage | reasonCode |
| --- | --- | --- |
| `reject` | `identity` | `token_missing`、`token_ambiguous`、`token_unmapped`、`token_revoked`、`identity_mismatch`、`identity_store_unavailable` |
| `reject` | `user_state` | `user_missing`、`user_inactive`、`user_expired`、`emby_disabled`、`emby_access_disabled`、`device_blocked` |
| `fallback` | `route` | `route_not_accelerated`、`request_not_eligible` |
| `fallback` | `proof` | `playback_proof_missing`、`playback_proof_expired`、`playback_proof_mismatch` |
| `fallback` | `eligibility` | `direct_play_disabled`、`client_incompatible`、`concurrency_limited`、`media_not_direct_play` |
| `fallback` | `direct_play` | `invalid_request`、`path_not_mapped`、`account_unavailable`、`accounts_same`、`provider_unavailable`、`provider_protocol`、`rapid_upload_unavailable`、`target_unavailable`、`download_incompatible`、`store_unavailable`、`lock_unavailable` |
| `redirect` | `direct_play` | `direct_play_ready` |

所有决策日志记录 `statusCode`；fallback 已取得 Emby 响应时再记录 `upstreamStatus`，代理传输失败只记录固定 `proxyErrorCode`，禁止输出上游原始错误。该日志描述 Gateway 对本次请求选择的路径，不把“已选择 fallback”误写成“Emby 已成功播放”。

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
9. 单 Token、单设备和用户全部撤销只影响各自范围；硬撤销后请求拒绝，动态到期/套餐拒绝不错误写入 `revokedAt`。
10. `lastSeenAt` 限频、ServerId/EmbyID 错配拒绝、并发登录幂等 upsert，以及数据库/日志/JSON 均不包含 Token 明文。
11. 302 要求相同 Token 的近期成功 PlaybackInfo；缺少/过期证明时合法请求 fallback Emby，撤销后缓存重用和原始 Emby 公网绕过仍失败关闭。
12. 每个视频请求只产生一条脱敏 `redirect/fallback/reject` 日志，且任何 fallback 都保持原始请求字节和 Header 语义。

所有测试必须使用 fake Emby Server 或固定 fixture，禁止请求真实 Emby。

## 11. 待实机确认清单

真实只读验证需要用户明确授权，至少确认：

- 目标 Emby Server 的 `Version`、`Id` 和 `ServerName`。
- 目标 Infuse 版本实际调用的认证和流路径。
- Infuse 实际通过哪个 Header 或 query 参数携带 AccessToken，以及 DeviceId/客户端名称是否稳定。
- Infuse 对 `302`、`HEAD`、`Range`、UA 和文件名的处理。
- 客户端是否始终携带 `PlaySessionId`、`MediaSourceId` 和设备标识。
- Direct Play、Direct Stream 与 Transcode 三种情况下的实际请求差异。
- 目标 Emby 4.9.3.0 是否提供可安全调用的单 Token、单设备或会话撤销接口；确认前只能宣称 Ember 网关本地撤销。

以上未确认项不能作为实现完成的依据。
