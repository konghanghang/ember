# Emby Gateway 客户端兼容矩阵

本文记录 `ember-gateway` 面向 Emby 客户端的能力合同。兼容以 HTTP 协议形态为核心，不按播放器名称堆行为分支；User-Agent 只用于脱敏观察和可选访问控制。

## 1. 证据等级

| 等级 | 含义 |
| --- | --- |
| 目标环境实测 | 当前目标 Emby `4.9.3.0` 与客户端真实请求日志直接证明 |
| 用户测试反馈 | 用户报告目标客户端使用结果，但尚未记录完整版本、分项用例或请求证据；不能扩展为全部协议形态已验证 |
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
| `Authorization` / `X-Emby-Authorization` / `X-MediaBrowser-Authorization` | 有界 `Emby ... Token="..."` 或 `MediaBrowser ... Token="..."` grammar，scheme 大小写不敏感 | 读取唯一 Token，不要求完整应用元数据；上游仍决定原生认证是否有效 |
| 任意 Bearer、Quick Connect、PIN、插件 Token | 无版本化合同 | 不接受 |

同一请求出现多个非空候选时，只有所有字节完全一致才接受；重复身份来源、空直接 Token、冲突、未知 scheme、非法 quoted-string 均返回 `401`；非身份元数据缺失、空值、扩展和重复不触发拒绝。Gateway 使用唯一候选执行 HMAC 映射和用户状态检查，原始 Header/query 不改写并继续透明转发。

- scheme 的大小写语义依据 [RFC 9110 §11.1](https://www.rfc-editor.org/rfc/rfc9110.html#section-11.1)；仅比较 `Emby` / `MediaBrowser` 时忽略大小写，Token 值仍逐字节区分大小写，字段 grammar、长度、重复和冲突保护保持。诊断使用同一识别规则，只记录固定分类，不记录 Header 原值。
- 精确 `AuthenticateByName` 请求由 Emby 判定，不再因元数据解析失败、多个元数据载体或旧 Token 在 Gateway 返回登录 `401`。上游成功响应才建立本地映射，元数据旁路失败仅记固定 Debug 原因。
- Public users/无 Index 用户头像无 Token 时直接转发，由 Emby 决定响应；携 Token 时继续校验映射、撤销和用户资格。公开范围不扩展到其他用户接口。
- 元数据采集与 Token 判定分离，完整应用头只作为可选审计 fallback。原生请求 grammar 不可解析、多个应用头或冲突身份仍不能绕过本地 Token 门控。

query Token 可能被外层代理 access log 记录。部署必须只记录 `$uri` 和 query key 形态，禁止 `$request`、`$request_uri`、`$args` 等包含 query value 的字段。

### 2.2 路径与 query 大小写

- 支持 OpenAPI 顶层 API family 的 root 和可选 `/emby` 前缀；family 与 `/emby` 比较大小写不敏感，只规范化前缀，保留其余 path 字节。
- 认证、SystemInfoPublic、公共用户、PlaybackInfo、视频固定路径的语义段大小写不敏感；尾斜杠、额外层级和 alternate escaping 不放宽。
- Gateway 自己读取的 `UserId`、`MediaSourceId`、`PlaySessionId`、`Static`、`Container` query key 大小写不敏感；相同逻辑 key 的大小写重复仍视为歧义。
- 三个固定会话 POST 端点的语义路径同样大小写不敏感，成功上游响应后的租约续期、停止释放和许可撤销保持一致；编码、尾斜杠和额外层级不会借此触发本地事件。
- query 原始 key/value 与顺序继续交给上游，Gateway 不做 MediaWarp 式全局小写重写。
- 无 Token 的 `GET/HEAD /`、`/favicon.ico` 与 `/web` 页面/静态资源由数据库 Web 开关控制；单层有界 `/web/strings/{locale}.json` 是目标 Web 已确认的静态资源，精确 `/web/ConfigurationPage(s)|strings|stringset` API、其他深层变体、携带 Token 的 Web path 和根 WebSocket Upgrade 仍走通用 Token 门控，不能借 Web UI Surface 绕过本地撤销。
- 精确 `GET/HEAD /emby/Branding/Css.css` 是目标 Web 已确认的登录前匿名请求；精确 `GET /emby/Branding/Configuration` 只接受严格 Web query 应用元数据并受 Web Surface 开关控制。其他 Branding API、方法、尾斜杠和深层路径不继承登录前权限。
- 目标 Web 登录后批量请求的无 Token `GET/HEAD /emby/Items/{Id}/Images/{Type}`，以及追加一个规范非负十进制 int32 `{Index}` 的形态，只按层级精确、动态段有界的 `/emby` 路径进入独立 `item_image` 代理，不读取网页开关；携 Token 请求仍受保护，root、非法 Index、修改、深层和 encoded path 不继承权限。固定 SDK 的认证标记与目标请求形态存在冲突；无 Index Primary 与 Backdrop Index `0/1` 已分别取得真实上游 `200` 和非零图片字节。

### 2.3 响应与客户端名称

- 普通 API、图片、字幕、会话和未知受保护 Surface 默认走同一个透明代理。
- AuthenticationResult 支持 JSON/XML，PlaybackInfo 保留 JSON 旁路；两者只对有界旁路副本按 `identity/gzip/deflate/br` 解码，客户端收到的状态、Content-Encoding、Header 和原始字节不重新编码。目标 Infuse 已确认 deflate，目标 Emby Web `4.9.3.0` 已确认 Brotli 登录响应。
- 层级精确的用户条目响应同样保持原字节，只缓存当前 mapping 下的 Item/MediaSource Container。合格 GET/HEAD 在显式 `Static=true`、唯一非空 MediaSourceId 且完全无 PlaySessionId key 时，可使用当前用户 Token 向同一 Emby 补取 PlaybackInfo。除旧 plain stream 外，新增 `stream.{Container}` 与固定 `original.mp4/mkv`；新增形态只许身份 Token、MediaSourceId、Static、Container 及可选唯一匹配 Principal 的 DeviceId，容器须单一合法且不是 manifest，stream 的额外 Container 必须一致，original 必须显式带同扩展名 Container。完整条件见[播放合同 4.5](./emby-playback-proxy-contract.md#45-缺失-playsessionid-的按需-playbackinfo)。
- 无会话补查分离 115 决策与 Emby fallback：旧 plain stream 继续使用严格验证并移除 URL Token 的 DirectStreamUrl，缺失时使用 `stream.{Container}`；新增扩展名 stream / 固定 original 无论补查还是 DirectPlay 失败，都保留客户端原 URL、RawQuery、Range 和 Header。无 Static 的 original 缺会话、缺源、多源歧义与未知参数继续回源；固定会话无 proof/live binding 也不借此补查。
- PlaybackInfo 的多 MediaSources、音轨/字幕、空数组与未知字段均保留原响应字节，内部按明确 MediaSourceId 选源，不默认使用第一个源。`MediaStreams: []` 不会被 `omitempty` 丢失；这是 MediaWarp 源码标注的 [Yamby 兼容点](https://github.com/AkimioJR/MediaWarp/blob/070ad99cb32e940b2b8ccb7c55b6efb7d311eac5/internal/service/emby/schema.go#L224-L233)。
- 普通视频 fallback 保留 Range / If-Range 与上游 200/206/416、ETag、Last-Modified、Content-Range 和 HEAD 无正文语义；实际视频请求的 UA 传入 DirectPlay，不借用前序 PlaybackInfo 的 UA。普通字幕、HLS、Audio、Download 和 Sync 仍是受保护透传；Audio/Sync 只证明通用代理能力，Download 只证明上游授权/拒绝响应保持，不宣称新增套餐下载权益。
- 日志识别 Infuse Direct/Library、SenPlayer、Yamby、VidHub、Fileball、Conflux 和官方 Emby family；未知 UA 仍代理，不因名称进入不同认证或播放逻辑。
- 默认严格模式仍要求明确起播才能首次转存；管理员选择全局 `P115_TRANSFER_TRIGGER_MODE=compatible` 后，合格原文件 GET 可按独立策略准入，接受包含 Infuse 在内的预读副作用，HEAD 禁止新增转存。固定会话 proof 过期后，仅已成功准入且由真实租约事件续活的媒体/session binding 可重新核验；不延长旧证明、不重授严格许可。

## 3. 客户端证据矩阵

| 客户端 | 已确认 | 尚未确认 |
| --- | --- | --- |
| Infuse `8.5.x` | 目标环境已确认 root API、MediaBrowser 应用头、deflate AuthenticationResult、内嵌 Token、普通资源 API `200`。2026-08-29 Infuse `8.5.2` 的 `Size=0` 条目在解耦版本中得到 `proofAccepted=true`，完成 source 前缀/相对路径解析、Provider 权威 Size 转存，并由 Gateway 首次及多次复用返回 `302`。2026-08-31 macOS Infuse `8.5.2` 进一步确认本地扩展名 fallback `206`、首次/复用 `302` 实际播放、外挂/内嵌字幕，以及 Playing/Progress/Stopped `204`。账号运行期历史四类回写、1 分钟共享冷却和半开单探测已有 fake/race 与 PostgreSQL 集成证据；2026-10-17 新增文件分类、断连释放、设备/容量隔离与阶段预算修复仅有 fake/race/SQL mock 证据，不扩展为新一轮真实客户端或数据库验收 | 115 CDN 完整响应头/Range/全文件字节取证，以及自然发生的生产 Provider 冷却/恢复时长；其他平台仍需按目标版本记录 |
| Infuse `8.5.5`（2026-09-29） | 生产日志确认显式 JSON POST `IsPlayback=true` 的响应能力为 false，随后内部 GET 能力为 true 且会话不同；另有源查询成功后因缺少许可退出、随后源账号不可用的样本。本次用 mock 锁定显式意图受限继承、原会话停止撤销与 30 秒到期，用 PostgreSQL 专项验证源探测成功后恢复及并发控制保护 | 修复后的真实首次转存、115 重定向及播放器完成播放待部署后验收；10 秒源路径超时的网络原因未证实 |
| SenPlayer | 2026-09-21 用户反馈正常，不作为后续修复回归。2026-09-29 `6.0.4` / Emby `4.9.3.0` 已确认缺失 Static 的 MP4 响应头合同，受限适配已有 fake 回归，见播放代理合同 5.1。2026-10-14 同客户端、Gateway `f18c84e` 抓包确认：3 次 MKV 播放共 14 个 `original.mkv` GET 均无 Static；媒体证明支持 DirectPlay/DirectStream，视频类型、Range 总长度及 ETag 一致，全部以 request_not_eligible 回退 Emby 200/206；13 个播放事件得到 204，详见 [合同 5.2](./emby-playback-proxy-contract.md#52-senplayer-originalmkv-抓包证据2026-10-14) | 平台仍未记录；MKV 兼容已完成实现与 mock/race 验证，待部署后实机验收。2026-10-14 用户选择仅兼容已有目标，保留预读不新增转存。2026-10-25 新增默认严格、管理员可开启的全局兼容模式：query false/body 缺字段仍不产生严格许可，兼容模式以独立准入允许合格原文件 GET 首次转存，包括预读；真实新模式尚未验收。真实 115 302、CDN Range、完整字节、字幕及修复后首帧未验收 |
| Yamby | MediaWarp 固定源码证明空 `MediaStreams` 数组不能丢，Ember 保留原响应字节。2026-09-29 本地 Emby `4.9.3.0` 调试定位到 UserId 元数据没有双引号；支持有界无引号元数据后，真实 Yamby 登录、SystemInfo、Views、Items、Resume 和 Latest 均返回 `200`，未再出现解析错误或 `401` | 历史无 Token 图片曾被 Web 关闭开关返回 `404`；2026-10-25 已分离图片路由并完成 fake 回归，真实首页显示尚未复测；客户端精确版本未记录；用户确认播放此前正常，本次不复验；无引号元数据与诊断已补 mock 回归 |
| Emby Web | 目标 Gateway 日志已确认浏览器请求 `/`、`/favicon.ico`、完整静态资源、单层语言 JSON、Branding CSS，以及携四个必填 `X-Emby-*` query 元数据的 Public users、Branding Configuration 和 AuthenticateByName。2026-08-31 真实浏览器进一步确认完整页面资源、query 登录、Primary、Backdrop Index `0/1` 与登录后 WebSocket OPEN；Web 关闭时 GET/HEAD 中文 `404`、禁缓存和 Infuse 不受影响均通过 | Web 播放 115 文件按用户当前无使用场景排除，不表述为已兼容；Cloudflare Browser Insights 注入被 CSP 拦截，属于接受的部署层偏差 |
| iOS Emby / Conflux / Fileball / VidHub | MediaWarp README 声明已测试这些客户端；Ember 有通用透明代理和 UA 观察 | 目标环境实机登录、资源、播放和 WebSocket |

不能把上游 README 或 Ember fake 测试写成目标环境已通过。新增客户端兼容必须先捕获脱敏请求形态，再扩展协议矩阵；禁止仅凭 UA 添加认证绕过。

### 3.1 2026-09-21 认证透明化修复的验证记录

对应提交：`e611675`。用户当日反馈 Infuse、SenPlayer 测试正常，Yamby 无法登录；提供的日志确认存在 Gateway 本地认证元数据拒绝，但没有足够请求形态把某一种认证头确定为 Yamby 根因。

本次代码验证在 `services/api` 执行，以下命令均通过：

```bash
go test ./... -skip 'Integration|PostgreSQL' -count=1
go test -race ./internal/integrations/emby ./internal/playbackgateway ./internal/services/embytoken -skip 'Integration|PostgreSQL' -count=1
go vet ./...
go build ./...
```

覆盖登录成功/失败响应保留、可选元数据、旧 Token 原样转发、JSON/XML 与压缩响应映射、XML 非法/超限和实体边界、登录后 Token 检查、公开登录接口以及日志脱敏。外部依赖均使用 fake；数据库集成用例被显式排除，没有验证真实 PostgreSQL、Emby、115 或播放器行为。

截至 2026-09-21，修复后 Yamby 登录、资源加载与播放，以及 Infuse/SenPlayer 的修复后回归仍待执行；真实 Emby XML 响应也未验证。2026-09-29 的 Yamby 登录与列表实测结果已更新到上表，不能把该历史待验证状态当作最新结论。实机取证按部署排障手册的客户端登录排障与复测步骤执行。后续记录至少包含部署提交、Emby/客户端精确版本、平台、日期/业务时区和各步骤“通过/失败/未执行”；本条不能用历史数据库集成或其他客户端结果代替。

### 3.2 2026-10-22 本地 Yamby 空密码请求排查

目标 Emby 版本由本地 Gateway 启动日志确认为 `4.9.3.0`，客户端精确版本未记录。用户确认同一 Yamby 直连本地配置的源站可登录；本轮没有改动认证决定或请求字段值，只增加 Debug 脱敏旁路诊断。

- `21:40:35` 至 `21:40:39` 的四次登录：JSON 完整有效，`Username` 非空、`Pw/Password` 为空，query key 数为零；相同 `requestId` 的 `authentication_upstream_response` 明确记录上游 `401`。
- 用户确认重新输入了密码；`21:44:01` 的 Yamby 登录中 `Pw/Password` 变为非空，上游与最终响应均为 `200`，随后受保护接口、条目详情和 PlaybackInfo 返回 `200`。
- 以上证明本地重新输入密码后登录恢复；客户端此前为何生成空字段尚未证实，不能直接归因于缓存。该结果不代替 hz 入口复测，也不扩展为实际播放验收。

诊断只记录固定字段状态，不记录用户名、密码、字段长度或摘要；mock 回归、Gateway race、Go vet/build 已通过，外部请求由用户在现有本地进程上发起。

### 3.3 2026-10-25 后续协议兼容增量

首轮三批次实现基线为 `master@30fb9a4d`。后续增量包括三种应用头的 scheme 大小写、复杂 PlaybackInfo/多媒体源、视频 UA、条件 Range、非加速接口透明代理，以及 2.3 中有合同依据的无会话首请求补查。`authorization_scheme_test.go`、`client_protocol_regression_test.go`、`stream_first_test.go` 的定向回归和既有补查/原文件/许可/固定会话回归通过；随后全量非数据库 Go 测试、Gateway/DirectPlay/EmbyToken race、vet/build 与独立代码审查完成，详细结果见[主计划第 7 节](../plan/architecture/gateway-client-compatibility.md#7-后续协议兼容改进2026-10-25)。这些结果属于本地自动化验证。

本轮未增加 Web UI、配置、数据库 migration 或 CORS 支持；跨域 Web、115 音频/下载加速、云端 HLS 和字幕转换保持独立后续范围。以上不是新一轮实机验收，表中的历史客户端证据不据此升级；仍须记录实际部署提交、Emby/客户端版本、平台，以及登录、选源、字幕、拖动/续播和停止的分项结果。

## 4. Infuse 扫库与 Token Store

2026-10-25 客户端兼容收口：独立图片路由、会话大小写、可信固定会话重新核验和可选兼容准入已完成代码与本地 fake/race 验证；设置中心复用既有 enum 控件并覆盖严格→兼容→严格保存，未新增生产 UI 特判。真实 Yamby 首页、SenPlayer 首次/复用转存、CDN Range、字幕和长时间续播仍未复验，不能把这些自动化结果填成目标客户端已通过。实施与剩余验收见 [Gateway 客户端兼容计划](../plan/architecture/gateway-client-compatibility.md)。

Infuse 会并发请求 Views、VirtualFolders、DisplayPreferences、Items、Latest 和 Resume。每个请求仍执行当前 Token 映射、撤销与用户状态检查，不增加 Principal TTL 缓存：

- `context.Canceled` 返回固定 `499`/`token_request_canceled`，不再误报存储故障。
- `context.DeadlineExceeded` 返回 `504`/`token_request_deadline_exceeded`。
- 坏连接、连接关闭、EOF、网络/pgconn 超时等已分类连接故障对幂等 SELECT 最多重试一次；请求取消/deadline、PostgreSQL 响应错误、业务错误和写操作不重试。
- 最终真实存储失败记录固定 `reasonCode + retryable`、SQLSTATE（如有）及 database/sql `maxOpen/open/inUse/idle/waitCount/waitMs`，禁止 DSN、SQL 参数、Token digest 或错误原文。

这只消除取消误报和安全可重试的瞬时读失败，不缓存 Principal，也不延迟封禁、撤销或到期生效。
