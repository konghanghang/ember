# Gateway 115 直链冷启动耗时优化方案

> 状态：待开发
> 负责人：Ember
> 更新时间：2026-11-26

## 背景

在跨区域部署场景（播放网关所在区域与 115 服务端之间基础单向 RTT 约 `200~230 ms`）下，命中 115 映射路径但尚未在目标播放账号中存在的新片首次播放，冷启动耗时通常在 `7.8s ~ 17.5s` 之间，存在以下真实问题：

- **跨区域网络与冷建连抖动放大严重**：单次冷转存需串行执行约 `14` 次 115 外部 HTTP 请求，非复用连接在 DNS 解析、TCP 建连与 TLS 握手阶段易出现秒级抖动（实测单次建连可达 `3~4s`）。
- **协议与编排链路存在确定的冗余往返**：
  - `InitRapidUpload` 每次调用均无条件请求 `GetUploadInfo`（首传与 challenge 重试各打 1 次）。
  - `challenge` 阶段的 `HashFileRange` 未复用 `preID` 阶段刚签发的源下载链接，重复调用 `GetDownloadURL`。
  - `directplay.Service` 在锁前与锁后无条件执行两次 `searchTarget`，在无并发竞争时多出 1 次完整外部往返。
  - `FindTargetFile` 首次未命中后固定等待 `500ms` 再轮询，直接增加半秒固定延迟。
- **用户起播前的浏览窗口未利用**：客户端发送 `PlaybackInfo` 与实际发起视频流请求之间通常存在 `3~9s` 窗口，当前未做只读预热，导致所有冷查询与 `preID` 读取集中在点击播放后的同步阻塞路径上。

如果不做优化，跨区域部署实例的新片首播等待时间过长，容易触发播放器超时或显著影响观看体验（同文件缓存命中后的拖拽/二次播放已在 `5~10ms` 级别，无需重复优化）。

## 目标

本方案按投入产出比分为三个阶段推进：

1. **阶段一（部署与传输层，零代码改动）**：通过近源出口代理与宿主机 DNS 缓存降低单次外部 RTT 与冷建连抖动，目标将跨区冷启动耗时从 `8~17s` 压降至 `2~4s`。
2. **阶段二（代码层冗余往返裁剪，低风险高确定性）**：将单次冷转存的外部 115 串行请求从 `14` 次压缩至 `9~10` 次，消除固定轮询空等，在跨区网络下再额外节省 `1.5~2.5s`。
3. **阶段三（异步只读预热，按需启用）**：利用 `PlaybackInfo` 到视频流请求之间的窗口异步预热源目录与 `preID`（及连接池），在不提前消耗转存配额的前提下，进一步压缩用户点击播放后的同步等待。

## 非目标

本次明确不做：

- 不改变 115 账号池路由、播放租约、并发流限制与转存配额扣减语义。
- 不在 `PlaybackInfo` 阶段触发 `InitRapidUpload` 等任何写操作或提前占用转存配额。
- 不为 `.iso` 等 Emby 标记不支持直链的片源或 HLS 转码路由强行接入 115 直链。
- 不破坏现有 `direct_play_redirect` / `direct_play_fallback` 的结构化诊断字段与错误分类语义。
- 不引入新的数据库表、字段或 SQL migration。

## 当前事实

以当前代码和现行文档为准：

- 相关文档：
  - `docs/plan/architecture/emby-115-direct-play-gateway.md`
  - `docs/plan/architecture/gateway-read-path-optimization.md`
- 相关服务与模块：
  - 传输层与代理：`services/api/internal/integrations/p115/cookie_transport.go`
  - 115 Cookie 适配器：`services/api/internal/integrations/p115/cookie_http_adapter.go`
  - 直链与转存编排：`services/api/internal/services/directplay/service.go`
  - `PlaybackInfo` 观测入口：`services/api/internal/playbackgateway/playback_info.go`
- 当前行为：
  - `cookie_transport.go` 已启用 HTTP Keep-Alive（`MaxIdleConns=64`、`MaxIdleConnsPerHost=16`、`IdleConnTimeout=90s`），且默认挂载 `Proxy: http.ProxyFromEnvironment`。
  - `CookieHTTPAdapter.InitRapidUpload` 每次执行均先调用 `GetUploadInfo`。
  - `CookieHTTPAdapter.HashFileRange` 每次执行均先调用 `GetDownloadURL`，再执行 `readFileRange`。
  - `CookieHTTPAdapter.FindTargetFile` 轮询间隔固定为 `targetVisibilityPollInterval = 500ms`（总超时 `10s`）。
  - `directplay.Service` 锁前执行一次 `searchTarget`，进入 `resolveUnderLock` 后无论是否发生锁等待或并发写入，均再次执行 `searchTarget`。
  - `PlaybackGateway` 在 `PlaybackInfo` 响应解析阶段已记录 `playback_info_proof_recorded`（包含 `mediaPath`、大小与 `proofAccepted`），但未触发任何异步预热。
- 现有限制：
  - 115 协议层多步强依赖串行状态，单次 RTT 过高时，任何多余往返都会线性放大首播延迟。

## 方案设计

### 1. 用户可见行为

- **保持不变**：
  - 播放接口协议、状态码（`302` 直链 / 回退 Emby）、播放租约、并发限制、转存配额记账、失败回退策略全部保持不变。
  - 缓存命中时的拖拽与续播仍走现有缓存路径（`5~10ms`）。
- **变化**：
  - 新片首次起播等待时间显著缩短。
  - 阶段三（可选）引入默认保守的只读预热机制，不影响未播放时的转存配额。

### 2. 数据与模型

> 本次不涉及数据模型变更，无需 SQL migration。

仅涉及进程内短期缓存与可选运行期/环境变量配置（出口代理环境变量、预热开关与并发上限）。

### 3. 接口与边界

- 对外 HTTP API、Emby 兼容路由、Internal API 与 Bot 命令均无契约变更。
- 内部接口边界调整：
  - `p115.CookieHTTPAdapter` 内部维护进程内轻量缓存：
    - `UploadInfo` 缓存：Key 为 `(accountID, cookieFingerprint)`，TTL `15~30m`，遇鉴权拒绝或上传协议错误时主动失效并允许单次刷新。
    - 源 `DownloadURL` 短期缓存（或 `HashFileRange` 支持复用同一转存任务内的签名下载链接）：Key 为 `(accountID, pickCode, userAgent)`，过期时间严格遵守 URL 自带 `t` 过期时间减去安全余量（如 `30s`），且仅限同进程源站 Range 读取复用；遇 `403` / 过期错误时失效并回退重签一次。
  - `directplay.Service` 锁机制边界：
    - 单文件锁 `Acquire` 返回是否经历等待或锁代际（generation）变化；仅在发生锁等待或同一 `(playbackAccountID, sha1, size, targetParentID)` 在锁前查询后有过写完成记录时，才在 `resolveUnderLock` 中执行二次 `searchTarget`。
  - `PlaybackGateway` → `directplay.Service`（阶段三）：
    - 新增非阻塞只读预热入口（例如 `WarmSourceReadOnly`），由 `PlaybackInfo` 在 `proofAccepted=true` 且路径命中 115 映射前缀后异步投递。

### 4. 关键流程

#### 阶段一：部署与网络出口优化（先于代码改动执行）

1. **近源出口代理**：利用 `cookie_transport.go` 既有的 `http.ProxyFromEnvironment`，为播放网关进程配置指向低延迟出口节点的 `HTTPS_PROXY` / `HTTP_PROXY`（同时通过 `NO_PROXY` 排除 Emby 源站、PostgreSQL、Redis 等内网地址，避免本地流量绕行）。
2. **宿主机 DNS 缓存**：检查宿主机 DNS 解析链路，启用本地缓存（或固定低延迟递归 DNS），消除单次解析超过 1 秒的抖动。
3. **验证基准**：对比开启代理前后 `[P115]` 日志中的 `dnsMs`、`connectMs`、`tlsMs`、`firstByteMs` 与 `directPlayMs`。

#### 阶段二：代码层冗余往返裁剪（4 项独立可拆提交）

1. **`GetUploadInfo` 进程内缓存**（预计省 `~460~500ms`，砍 2 次请求）：
   - `InitRapidUpload` 优先命中内存缓存的 `UploadInfo`；缓存未命中时走单飞（singleflight）拉取。
   - 当 Cookie 变更（指纹变化）或 `InitRapidUpload` 返回凭证/协议失效错误时，清理缓存并按既有重试/失败语义处理。
2. **`challenge` 复用 `preID` 阶段源 `DownloadURL`**（预计省 `~350~400ms`，砍 1 次请求）：
   - 在 `HashFileRange`（或 `CookieHTTPAdapter` 内部）为源站 Range 读取增加短期签名 URL 复用；`preID` 刚签发的源链接在 `challenge` 阶段直接复用，省去第二次 `GetDownloadURL` 并提高 CDN 连接复用率。
   - 若 CDN 返回 `403` / 签名失效，自动失效缓存并重新签发一次再读取。
3. **无并发竞争时跳过锁内二次 `targetSearch`**（预计省 `~270~540ms`，砍 1 次请求）：
   - 锁前 `searchTarget` 未命中后，记录当前目标键的本地完成代际号再获取内容锁。
   - 加锁后若 `waited == false` 且代际号未变化，直接跳过锁内二次 `targetSearch`，进入转存授权与任务创建流程；若发生过锁等待或代际变化，保持原二次查询逻辑以确保不重复转存。
4. **`FindTargetFile` 渐进退避轮询**（预计省 `~350ms`）：
   - 将 `targetVisibilityPollInterval` 由固定 `500ms` 调整为渐进退避序列（例如 `150ms → 300ms → 500ms`，后续封顶 `500ms`），总超时仍保持 `10s` 与上下文取消语义不变。

#### 阶段三：基于 `PlaybackInfo` 的异步只读预热（可选，视阶段一/二收益决定）

1. 在 `PlaybackInfo` 处理完成且满足以下全部条件时，向有界异步队列投递只读预热任务：
   - 预热开关开启；
   - `proofAccepted == true` 且 `mediaPath` 命中已配置的 115 映射前缀；
   - 未命中已有 `preIDCache` / `mediaResolutionCache`。
2. 预热任务只执行只读操作：`sourceResolve`（填充目录/文件元数据缓存）与 `preID` 读取（填充 `preIDCache`），顺带预热到 115 域名与 CDN 的 Keep-Alive 连接。
3. 严格约束：
   - 严禁在预热路径调用 `InitRapidUpload`、占用转存配额或写入转存任务表。
   - 必须有单飞去重（同文件不重复预热）、全局并发上限（如 `2~4`）和较短超时，避免列表批量预拉 `PlaybackInfo` 时挤占真实播放的 115 限流配额（`limiter`）。

### 5. 失败路径与边界条件

- **代理配置误伤内网流量**：必须明确配置 `NO_PROXY` 覆盖内网 Emby、DB、Redis 网段，否则会导致正常接口和回退流量延迟升高。
- **`UploadInfo` 缓存陈旧**：若账号 Cookie 更新或服务端 `userkey` 轮换，缓存必须通过 Cookie 指纹自动隔离，并在遇到 `ErrCredentialRejected` / 协议错误时清除缓存。
- **源 `DownloadURL` 提前失效**：`challenge` 复用源链接遇 CDN `403` 或过期时，只允许自动刷新 1 次，避免死循环；刷新后仍失败则按原 `challenge_failed` 路径回退 Emby。
- **跳过二次 `targetSearch` 的并发正确性**：仅在单机单 Gateway 实例且无锁等待、无代际变更时才可跳过（符合当前单副本部署假设）；一旦发生锁等待或进程内存在同目标完成记录，必须重新查询目标。
- **只读预热触发 115 限流/风控**：预热任务须使用独立受限并发池；当账号处于限流冷却或风控冷却期时直接跳过预热，绝不抢占真实起播请求。
- **兼容性约束**：不改变现有单测中对外暴露的诊断日志字段与失败分类；新增缓存命中/跳过信息仅补充字段，不删除已有字段。

## 影响范围

- **API（`services/api`）**：
  - `internal/integrations/p115/cookie_http_adapter.go`（`UploadInfo` 缓存、源 `DownloadURL` 复用、`FindTargetFile` 渐进退避）
  - `internal/services/directplay/service.go` 及锁模块（无竞争跳过二次 `targetSearch`、阶段三只读预热入口）
  - `internal/playbackgateway/playback_info.go`（阶段三：异步投递只读预热）
  - 对应单元测试文件（`cookie_*_test.go`、`service_test.go`、`playback_info_test.go`）
- **Web**：无
- **Bot**：无
- **配置/部署**：
  - 阶段一：`docker-compose.yml` / 部署环境变量补充可选 `HTTPS_PROXY`、`NO_PROXY` 说明；宿主机 DNS 缓存配置。
  - 阶段三（若实施）：新增预热开关与并发上限配置项（默认关闭或保守开启）。
- **文档**：
  - 若新增环境变量或调整 115 数据面缓存策略，同步更新 `docs/system-architecture.md` 与部署参考文档。

## 当前进度

- **已完成项**：
  - 完成跨区冷启动耗时基线拆解与代码路径核对，形成三阶段优化方案并落位 `docs/plan/architecture/`。
- **剩余项**：
  - 阶段一：部署层近源出口代理与宿主机 DNS 缓存配置及前后基线对比。
  - 阶段二：`GetUploadInfo` 缓存、`challenge` 复用源 `DownloadURL`、无竞争跳过二次 `targetSearch`、`FindTargetFile` 渐进退避及对应单测。
  - 阶段三（可选）：`PlaybackInfo` 异步只读预热及限流保护单测。
- **归档条件**：
  - 已采纳阶段的代码改动、单元测试、文档同步与上线后只读日志耗时对比均完成，且不再有待开发子项时，移入 `docs/archive/plan/architecture/`。

## 验证方式

### 编译与单元测试（所有测试必须 mock 115 外部依赖）

在 `services/api` 目录下执行：

- `go test ./internal/integrations/p115/... ./internal/services/directplay/... ./internal/playbackgateway/...`
- `go test ./...`
- `go vet ./...`
- `go build ./...`

核心新增/更新测试用例覆盖：
1. `UploadInfo` 缓存命中、Cookie 变更自动失效、鉴权失败失效重拉。
2. `HashFileRange` 连续读取同一源文件的 `preID` 与 `challenge` 时仅调用 1 次 `GetDownloadURL`，以及 CDN 返回 `403`/过期时的单次重签回退。
3. 无锁竞争跳过二次 `targetSearch`；存在并发锁等待或同目标写入后仍执行二次 `targetSearch` 且不重复转存。
4. `FindTargetFile` 渐进退避间隔序列、总超时上限与上下文取消行为。
5. （阶段三）`PlaybackInfo` 异步只读预热不触发转存、不扣配额、并发去重及冷却期跳过。

### 上线后观测验证（只读日志对比）

通过网关 `[PlaybackGateway]` 与 `[P115]` 结构化日志对比优化前后同一指标：
- `directPlayMs`（冷启动总耗时）
- `rapidUploadCalls` / `cookie_get` 次数（确认 `GetUploadInfo` 缓存生效）
- `challengeMs` 中 `sourceDownloadMs`（确认 `download_url` 复用生效）
- `targetSearchCalls`（无并发时从 `2` 降至 `1`）
- `targetVerifyMs`（确认首次未命中后的等待耗时下降）
- `dnsMs` / `connectMs` / `tlsMs`（确认代理与 DNS 优化生效）

## 落地后文档处理

- 阶段二 / 阶段三落地并验证稳定后，将 115 缓存边界、退避策略与代理环境变量说明沉淀至 `docs/system-architecture.md` 或对应 `docs/reference/` 文档。
- 全部已采纳阶段收口并完成验证后，将本方案移入 `docs/archive/plan/architecture/gateway-direct-play-latency-optimization.md`，并同步更新 `docs/plan/README.md`。
