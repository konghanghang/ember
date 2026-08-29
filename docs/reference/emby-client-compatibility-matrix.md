# Emby Gateway 客户端兼容矩阵

本文记录 `ember-gateway` 面向 Emby 客户端的能力合同。兼容以 HTTP 协议形态为核心，不按播放器名称堆行为分支；User-Agent 只用于脱敏观察和可选访问控制。

## 1. 证据等级

| 等级 | 含义 |
| --- | --- |
| 目标环境实测 | 当前目标 Emby `4.9.3.0` 与客户端真实请求日志直接证明 |
| 固定上游源码 | 外部代理项目固定提交中的实现、测试或明确注释 |
| Ember fake 合同 | Ember 使用 `httptest`、fixture 或 fake Store 锁定，不代表目标客户端已实机通过 |
| 未证实 | 尚无对应客户端请求或目标环境结果，不能写成已兼容 |

外部参考固定为：

- `hbq0405/emby-toolkit@00786dcd632c08a25016276d2531d650ab9ee00c`：其 [Token 提取](https://github.com/hbq0405/emby-toolkit/blob/00786dcd632c08a25016276d2531d650ab9ee00c/reverse_proxy.py#L411-L433) 覆盖 query、直接 Header 和应用认证头，[代理入口](https://github.com/hbq0405/emby-toolkit/blob/00786dcd632c08a25016276d2531d650ab9ee00c/reverse_proxy.py#L1703-L1729) 覆盖普通 HTTP 与 WebSocket。
- `AkimioJR/MediaWarp@070ad99cb32e940b2b8ccb7c55b6efb7d311eac5`：默认 [未命中特殊路由即透明代理](https://github.com/AkimioJR/MediaWarp/blob/070ad99cb32e940b2b8ccb7c55b6efb7d311eac5/internal/router/router.go#L93-L110)，特殊路由 [大小写不敏感并兼容可选 `/emby`](https://github.com/AkimioJR/MediaWarp/blob/070ad99cb32e940b2b8ccb7c55b6efb7d311eac5/constants/regexp.go#L28-L44)，并有 [大小写不敏感 query Token 测试入口](https://github.com/AkimioJR/MediaWarp/blob/070ad99cb32e940b2b8ccb7c55b6efb7d311eac5/utils/string.go#L89-L109)。

外部项目只作为协议形态证据。Ember 不复制 `emby-toolkit` 的“首个来源优先”或向上游注入管理员 API Key，也不复制会改变所有 query key 的全局重写。

## 2. 当前能力合同

### 2.1 Token 载体

受保护请求会收集以下候选：

| 来源 | 匹配规则 | 当前处理 |
| --- | --- | --- |
| `X-Emby-Token` Header | Header 名由 HTTP 大小写不敏感语义处理 | 接受唯一非空值 |
| `X-MediaBrowser-Token` Header | 同上 | 接受唯一非空值 |
| `api_key` query | key 大小写不敏感 | 接受唯一非空值 |
| `X-Emby-Token` query | key 大小写不敏感 | 接受唯一非空值 |
| `X-MediaBrowser-Token` query | key 大小写不敏感 | 接受唯一非空值 |
| `AccessToken` query | key 大小写不敏感 | 接受唯一非空值 |
| `Authorization` / `X-Emby-Authorization` | 严格 `Emby ... Token="..."` grammar | 接受完整合法字段 |
| `X-Emby-Authorization` / `X-MediaBrowser-Authorization` | 严格 `MediaBrowser ... Token="..."` grammar | 接受完整合法字段 |
| 任意 Bearer、Quick Connect、PIN、插件 Token | 无版本化合同 | 不接受 |

同一请求出现多个非空候选时，只有所有字节完全一致才接受；重复逻辑来源、空值、冲突、未知 scheme、缺字段、非法 quoted-string 均返回 `401`。Gateway 使用唯一候选执行 HMAC 映射和用户状态检查，原始 Header/query 不改写并继续透明转发。

- `AuthenticateByName` 必须使用严格应用头且所有外部 Token carrier 缺失，避免旧 Token 改变重新登录语义。
- Public users/无 Index 用户头像在登录前接受严格空 Token 应用头，登录后也接受已经映射的通用 Token carrier；非法应用头不能借 query Token 绕过。

query Token 可能被外层代理 access log 记录。部署必须只记录 `$uri` 和 query key 形态，禁止 `$request`、`$request_uri`、`$args` 等包含 query value 的字段。

### 2.2 路径与 query 大小写

- 支持 OpenAPI 顶层 API family 的 root 和可选 `/emby` 前缀；family 与 `/emby` 比较大小写不敏感，只规范化前缀，保留其余 path 字节。
- 认证、SystemInfoPublic、公共用户、PlaybackInfo、视频固定路径的语义段大小写不敏感；尾斜杠、额外层级和 alternate escaping 不放宽。
- Gateway 自己读取的 `UserId`、`MediaSourceId`、`PlaySessionId`、`Static`、`Container` query key 大小写不敏感；相同逻辑 key 的大小写重复仍视为歧义。
- query 原始 key/value 与顺序继续交给上游，Gateway 不做 MediaWarp 式全局小写重写。
- 无 Token 的 `GET/HEAD /`、`/favicon.ico` 与 `/web` 页面/静态资源由数据库 Web 开关控制；固定 `/web/ConfigurationPage(s)|strings|stringset` API、携带 Token 的 Web path 和根 WebSocket Upgrade 仍走通用 Token 门控，不能借 Web UI Surface 绕过本地撤销。

### 2.3 响应与客户端名称

- 普通 API、图片、字幕、会话和未知受保护 Surface 默认走同一个透明代理。
- AuthenticationResult 与 PlaybackInfo 只对有界旁路副本按 `identity/gzip/deflate` 解码，客户端收到的状态、Content-Encoding、Header 和原始字节不重新编码。
- 层级精确的用户条目响应同样保持原字节，只缓存当前 mapping 下的 Item/MediaSource Container。plain `/Videos/{Id}/stream` 缺 PlaySessionId 时，Gateway 优先使用当前用户 Token 向同一 Emby 补取 PlaybackInfo；成功后分离 115 决策请求与 Emby fallback，后者使用严格验证并移除 URL Token 的 DirectStreamUrl，缺失时使用 `stream.{Container}`。
- 因此 `MediaStreams: []` 等空数组不会被 `omitempty` 丢失；这是 MediaWarp 源码标注的 [Yamby 兼容点](https://github.com/AkimioJR/MediaWarp/blob/070ad99cb32e940b2b8ccb7c55b6efb7d311eac5/internal/service/emby/schema.go#L224-L233)。
- 日志识别 Infuse Direct/Library、SenPlayer、Yamby、VidHub、Fileball、Conflux 和官方 Emby family；未知 UA 仍代理，不因名称进入不同认证或播放逻辑。

## 3. 客户端证据矩阵

| 客户端 | 已确认 | 尚未确认 |
| --- | --- | --- |
| Infuse `8.5.x` | 目标环境已确认 root API、MediaBrowser 应用头、deflate AuthenticationResult、内嵌 Token、普通资源 API `200`。2026-08-29 Infuse `8.5.2` 的 `Size=0` 条目在解耦版本中得到 `proofAccepted=true`，完成 source 前缀/相对路径解析、Provider 权威 Size 转存，并由 Gateway 首次及多次复用返回 `302`；Playing 返回 `204` | 115 CDN 实际媒体字节/Range、稳定 Provider 重试或冷却、DirectStreamUrl/扩展名 Emby fallback、字幕、Progress/Stopped 完整链路 |
| SenPlayer | `emby-toolkit` 固定源码将其列为 native client；Ember 有 UA 与通用载体 fake 测试 | 真实 Header/query/path、播放和字幕行为 |
| Yamby | MediaWarp 固定源码证明空 `MediaStreams` 数组不能丢；Ember 有原字节保持 fake 测试 | 真实 Token 载体、路径与播放行为 |
| Emby Web | 目标 Gateway 日志已确认浏览器请求 `/` 与 `/favicon.ico`；固定 4.9.3 SDK 确认四个受保护 `/web` API 和根 WebSocket query 合同；Ember fake 测试覆盖后台开关实时变化、页面/静态资源代理、API 门控和真实 HTTP `101` Upgrade | 目标 Emby 根响应、Location、完整静态资源、登录、播放和 WebSocket 实机链路 |
| iOS Emby / Conflux / Fileball / VidHub | MediaWarp README 声明已测试这些客户端；Ember 有通用透明代理和 UA 观察 | 目标环境实机登录、资源、播放和 WebSocket |

不能把上游 README 或 Ember fake 测试写成目标环境已通过。新增客户端兼容必须先捕获脱敏请求形态，再扩展协议矩阵；禁止仅凭 UA 添加认证绕过。

## 4. Infuse 扫库与 Token Store

Infuse 会并发请求 Views、VirtualFolders、DisplayPreferences、Items、Latest 和 Resume。每个请求仍执行当前 Token 映射、撤销与用户状态检查，不增加 Principal TTL 缓存：

- `context.Canceled` 返回固定 `499`/`token_request_canceled`，不再误报存储故障。
- `context.DeadlineExceeded` 返回 `504`/`token_request_deadline_exceeded`。
- 坏连接、连接关闭、EOF、网络/pgconn 超时等已分类连接故障对幂等 SELECT 最多重试一次；请求取消/deadline、PostgreSQL 响应错误、业务错误和写操作不重试。
- 最终真实存储失败记录固定 `reasonCode + retryable`、SQLSTATE（如有）及 database/sql `maxOpen/open/inUse/idle/waitCount/waitMs`，禁止 DSN、SQL 参数、Token digest 或错误原文。

这只消除取消误报和安全可重试的瞬时读失败，不缓存 Principal，也不延迟封禁、撤销或到期生效。
