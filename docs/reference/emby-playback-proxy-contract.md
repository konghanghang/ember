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
- [Ember Playback Reporting 合同](./playback-reporting-api-contract.md)

## 2. 网关职责与信任边界

播放网关位于 Emby 客户端与 Emby Server 之间：

```text
Emby 客户端
  -> Ember Playback Gateway
       -> 普通 Emby API、字幕、播放进度：透明转发到 Emby Server
       -> 符合直连条件的视频流：解析媒体、执行策略、返回 115 直链 302
```

边界约束：

- 网关必须成为公网唯一 Emby 入口；原始 Emby Server 只允许内网或受控运维访问。
- 已通过 Token 门控的普通接口默认透明转发，不做猜测式改写；未认证 public bootstrap 路由必须先完成同版本合同核对，再加入显式 allowlist。
- 网关不得使用客户端提交的 `UserId` 作为最终身份依据，必须由 Emby AccessToken 映射到 Ember 用户。
- 视频字节在 302 成功后由客户端直接向 115 CDN 请求，不经过 Ember API、播放网关或 Emby Server。
- 网关不得记录 AccessToken、完整 115 直链、Cookie 或其他可复用凭证。

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

网关处理要求：

1. 将认证请求和响应透明转发，不修改 Emby 返回体。
2. 成功响应后提取 `User.Id`、`AccessToken` 和 `ServerId`。
3. 使用从 `CONFIG_ENCRYPTION_KEY` 按 `emby-access-token` purpose 派生的密钥计算 HMAC-SHA256；数据库只保存 32 字节摘要，不保存明文。
4. 根据 `users.emby_id` 查找 Ember 用户，并记录设备、客户端和最后访问时间。
5. 映射写入失败不能篡改 Emby 已成功的认证响应；该 Token 保持未映射，后续受保护请求和直连失败关闭。
6. 用户过期只做动态资格拒绝，不立即硬撤销映射；用户停用、Emby 访问禁用、Emby 账号解绑或删除时硬撤销。已发出的短期 CDN 链接不保证可以立即终止。

### 3.2 Token 哈希映射与本地撤销

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

撤销使用 `revokedAt + revokedReason + revokedBy` 软删除并保留审计，不直接删除行。固定原因至少包括 `manual_token_logout`、`manual_device_logout`、`manual_user_logout`、`user_disabled`、`emby_access_disabled`、`emby_unbound` 和 `security_revoke`。

状态边界：

- 到期、套餐不允许、并发已满和设备策略拒绝属于动态资格失败，不写 `revokedAt`；续期或策略恢复后原映射可以再次通过。
- 用户停用、`emby_disabled=true`、`emby_access_disabled=true`、解绑或删除属于硬撤销；普通状态恢复不能静默清除历史撤销，重新通过 `AuthenticateByName` 才能建立或重新激活映射。
- “本地撤销”只保证 Ember Playback Gateway 拒绝该映射，不等于 Emby Server 已撤销原始 Token。Emby 4.9.3.0 原生会话/Token 撤销接口尚未完成版本化核对，因此当前必须标记“未证实”。

要让设备强制退出成立，所有受保护的公网 Emby 请求都必须先通过 Token 映射检查，而不是只在 115 视频分支检查。部署时原始 Emby 端口必须只对网关和运维网络开放；否则客户端可以绕过 Ember 使用仍被 Emby 接受的原 Token。首次切换到网关后，历史 Emby Token 没有明文可安全回填，客户端需要重新登录一次建立映射。

截至 2026-08-22，`emby_access_tokens` migration、purpose 隔离 HMAC、并发安全 upsert、实时用户资格解析和三种本地撤销 Service 已实现并通过 fake/独立 PostgreSQL 测试。`internal/playbackgateway` 已接入认证响应旁路映射和 `X-Emby-Token` 门控核心，但尚无进程入口、状态联动和真实 Emby 请求；当前实现仍不能单独证明设备已被强制退出。

### 3.3 当前 HTTP 门控核心

在独立网关进程和部署入口落地前，当前 `internal/playbackgateway` 固定以下可测试行为：

- 只有 method、大小写、尾斜杠和 escaped path 都精确匹配的 `POST /emby/Users/AuthenticateByName` 免 Token 门控；其余路径默认受保护。
- 认证上游只有 `200` 才旁路解析；响应检查上限为 `1 MiB`。合法响应逐字节恢复后返回，字段顺序、空白和未知字段不重编码。
- 不合法、超过检查上限或映射写入失败的成功响应仍原样返回，但该 Token 不建立映射，下一次受保护请求失败关闭。
- 受保护请求只接受唯一的 `X-Emby-Token`。缺失、重复、未映射、已撤销和身份错配返回空体 `401`；当前用户不可用或到期返回空体 `403`；身份存储不可用返回空体 `503`。
- 上游网络和 transport 失败返回空体 `502`。日志只允许固定错误 code 和 Go 错误类型，禁止写入请求 URL、密码、AccessToken、认证响应体或上游原始错误文本。
- 目标 Infuse 的登录前 public bootstrap 路径和设备元数据载体尚未确认。当前不放行 `System/Info/Public` 等猜测路径，也不解析 `X-Emby-Authorization`；后续必须先补版本化合同和 fake fixture。

这仍是无监听器的内部核心，不表示目标 Emby/Infuse 已可用，也不包含 PlaybackInfo、视频 302 或播放会话。

### 3.4 暂不覆盖的认证方式

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

### 4.2 POST PlaybackInfo

```http
POST /emby/Items/{Id}/PlaybackInfo
Content-Type: application/json
X-Emby-Token: <access-token>

<PlaybackInfoRequest>
```

响应同样是 `PlaybackInfoResponse`。播放网关不得自行拼装一个缩水版响应；首版应透明转发 Emby 响应，仅在内部读取必要字段。

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

## 5. 原始视频流合同

`4.9.3.0` OpenAPI 明确列出以下视频流入口：

| Method | Path | 处理策略 |
| --- | --- | --- |
| `GET`, `HEAD` | `/emby/Videos/{Id}/stream` | 满足直连条件时可返回 302，否则按策略透明转发或拒绝 |
| `GET`, `HEAD` | `/emby/Videos/{Id}/stream.{Container}` | 同上，`Container` 是 path 参数 |
| `GET`, `HEAD` | `/emby/Videos/{Id}/{StreamFileName}` | 同上，保留客户端请求文件名 |
| `GET` | `/emby/Items/{Id}/Download` | 必须额外执行 `EnableContentDownloading` 和网关下载策略 |

`/Videos/{Id}/stream` 的 `Container` 是必填 query 参数；其余常见参数包括 `DeviceId`、`Static`、`StartTimeTicks`、音视频编码、码率、分辨率和字幕流索引。

处理约束：

- 不能看到 `/Videos/` 就一律返回 302；必须先判断 Token、用户状态、媒体源、路径规则和 Direct Play 能力。
- `GET` 与 `HEAD` 都可能由客户端用于探测，不得把每个请求都计为独立播放会话。
- 客户端可能在获得 302 后向 115 CDN 发出 `Range` 请求；网关必须通过客户端合同测试确认重定向、UA 和 Range 行为。
- `/Items/{Id}/Download` 与播放流不是同一权益，必须复用套餐下载开关并单独审计。
- 首版不改写 HLS、DASH、转码 manifest 和转码分片接口；云端媒体默认失败关闭，不静默回退为服务器中转。

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

Token 映射只证明“该 Token 曾由该 Server 签发给该 Emby 用户”，不能单独证明 Token 此刻仍被 Emby 接受。首期必须把近期成功的 GET/POST `PlaybackInfo` 作为当前授权证据；客户端绕过 PlaybackInfo 直接请求云端视频时失败关闭。Token 被撤销或用户状态变化后清除对应的直链缓存和未签发会话，但已经建立的 115 CDN 连接只能等到断线、重连或链接过期。

## 9. 失败与回退语义

- Token 无法映射：拒绝直连，不使用客户端 `UserId` 猜测身份。
- Token 已在 Ember 本地撤销：所有受保护的网关请求拒绝；不能把请求继续转发给 Emby 作为隐式回退。
- Token 有历史映射但没有近期成功的 PlaybackInfo：拒绝 302；不能把历史登录记录当作当前 Emby 授权。
- MediaSource 缺失或不支持 Direct Play：按套餐策略拒绝或显式回退，不能静默中转。
- 路径未命中 115 映射：本地媒体可按规则透明转发；未知来源默认拒绝直连。
- 115 解析或秒传失败：返回上游不可用语义，并记录脱敏错误码。
- 已发出 302 后用户被禁用：阻止后续直链和 Token 使用，但不保证立即切断已建立的 CDN 连接。
- Emby 会话事件转发失败：记录失败并允许网关会话 TTL 收口，不能伪造成功。

## 10. 合同测试要求

实现前后至少覆盖：

1. `AuthenticateByName` 响应透明转发、HMAC-SHA256 映射、明文不落库，以及映射写入失败不篡改原响应。
2. GET/POST `PlaybackInfo` 的请求和关键响应字段夹具。
3. 三类原始视频流路径的 `GET`、`HEAD`、query 和 302 行为。
4. 下载接口与普通播放权限分离。
5. 字幕接口不被视频拦截器误判。
6. Playing、Progress、Stopped 事件继续到达 fake Emby。
7. 重复 `HEAD`、Range 和重连不会重复计并发。
8. Quick Connect 等未覆盖入口不会被错误当作已支持。
9. 单 Token、单设备和用户全部撤销只影响各自范围；硬撤销后请求拒绝，动态到期/套餐拒绝不错误写入 `revokedAt`。
10. `lastSeenAt` 限频、ServerId/EmbyID 错配拒绝、并发登录幂等 upsert，以及数据库/日志/JSON 均不包含 Token 明文。
11. 302 要求相同 Token 的近期成功 PlaybackInfo；绕过 PlaybackInfo、撤销后缓存重用和原始 Emby 公网绕过均失败关闭。

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
