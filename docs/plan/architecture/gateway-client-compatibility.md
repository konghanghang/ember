# Gateway 客户端兼容与首次转存策略方案

> 状态：本地验证、远端 CI 与 hz 部署完成，待真实客户端及性能验收
> 负责人：Ember
> 更新时间：2026-10-25

## 背景

用户确认 Infuse 当前正常，Yamby 登录已恢复但首页图片无法加载，SenPlayer 能登录和播放却不新增 115 转存。代码调查发现图片与网页开关耦合、会话路径大小写漏处理、固定会话续播缺少证明重新核验，以及严格起播策略与已记录 SenPlayer 请求形态不匹配。

2026-10-25 用户确认采用三批次方案并授权实施：先修图片和会话，再完善续播，最后增加管理员可开启的全局兼容模式；默认保持严格模式，并接受兼容模式开启后包括 Infuse 在内的预读也可能产生转存。三个批次已在 `master` 完成实现和本地验证，随后随 `e78d8f80` 完成 hz 部署；真实客户端验收未执行，部署记录见第 10 节。

首轮实现已提交为 `30fb9a4d75dfb2d7371f55c39d967a42e72b82d9`。随后用户授权继续按顺序补认证协议与回归矩阵，再扩展有合同依据的视频首请求补查；这部分已完成实现和本地验证，见后述“后续协议兼容改进”。首轮及后续增量已随第 10 节目标镜像部署，尚未取得本轮真实客户端分项结果。

## 目标

1. 网页访问关闭时，现有合同允许的媒体图片仍可交由 Emby 响应；三个固定播放事件端点按大小写不敏感方式执行本地处理。
2. 合格固定流请求的媒体证明过期后可以受限重新核验，继续复用有效 115 目标和直链，并保持原播放会话关联。
3. 提供严格默认、显式开启的全局兼容模式，让合格原文件视频 GET 在目标缺失时可首次转存，准入来源与明确起播许可分开表达。

## 非目标

- 不扩展 root 形态无 Token 图片、其他匿名 API、认证方式或任意 `original.*`；不按 User-Agent 放行，不注入管理员 API Key。
- 不重写客户端 `IsPlayback`、query、body、媒体能力字段或原始响应；不把 GET、Range、Playing 推断为可靠的用户起播证明。
- 不允许 HEAD 新增转存，不放宽原文件资格、身份、账号、租约、配额和任务所有权，不恢复已失效的在途租约。
- 不引入新的媒体传输 Provider、远程视频中转、自动删除、后台预热或跨进程证明缓存；不启动服务或自动实施真实外部验证。
- 用户已明确当前只运行单个 Gateway，本计划不扩展多实例协调、分布式缓存或跨进程在途任务合并。

## 当前事实

以下旧行为和临时复现对应实施前调查基线；当前代码完成情况、现行准入边界和剩余验收见后续方案设计及进度，不能将调查中的失败现象当作修复后的行为。

- 调查基线为 `master@98b20c98841554b100bb1cf4f02114c2f6267c5a`。版本依据为 Emby `4.9.3.0` SDK `6ee0155063bc85578196489926359a8f37419502`，运行版本范围与证据边界沿用 [播放代理合同](../../reference/emby-playback-proxy-contract.md)。
- `services/api/internal/playbackgateway/web_surface.go` 的 `isEmbyWebSurfaceRequest` 将无 Token 精确图片归为 Web Surface；`serveWebSurface` 在开关关闭时返回 `404 HTML`，不请求 Emby。这与 `internal/config/config.go` 的网页开关说明不符；Yamby 历史图片 404 已记录于 [客户端矩阵](../../reference/emby-client-compatibility-matrix.md)，本轮未读取部署当前配置和图片请求。
- `playback_session_log.go` 的 `playbackSessionEventKindForRequest` 只匹配标准大小写路径；小写事件仍可代理成功，但不触发本地租约与停止处理。
- `playback_proof.go` 的证明期限为五分钟；`playback_info_resolver.go` 的 `onDemandPlaybackInfoCandidate` 只接纳不带 `PlaySessionId` 的 plain static stream。固定会话的 `original.mp4/mkv` 和 `stream.*` 过期后在 `video.go` 回退，Progress 不刷新证明。
- `playback_info.go` 排除 query 起播字段，`playback_transfer_intent.go` 要求 JSON 唯一布尔 `IsPlayback=true`；`services/directplay/service.go` 在锁内二次查重后检查许可，先于配额、任务、preID/challenge 和秒传。已记录 SenPlayer query `IsPlayback=false`、body 缺该字段，只能复用已有目标。
- 严格模式现行合同见 [端到端参考 1.3](../../reference/p115-playback-end-to-end-flow.md#13-新增转存起播许可) 与 [Emby 播放合同](../../reference/emby-playback-proxy-contract.md#新增转存起播许可ember-内部策略)；原文件资格见 [播放合同 5.2](../../reference/emby-playback-proxy-contract.md#52-senplayer-originalmkv-抓包证据2026-10-14)。[起播许可方案](../../archive/plan/architecture/p115-transfer-playback-intent.md) 与 [SenPlayer 方案](../../archive/plan/architecture/senplayer-original-stream-compatibility.md) 仅保留历史实现和验证记录，未完成验收由本计划的 [承接清单](#历史计划验收承接) 维护。
- 调查阶段四个相关包的 fake 测试通过；图片定向测试锁定关闭时 `404/upstream=0`。仓库外临时九个场景证明：六种标准/小写会话路径的本地事件次数为 `1/0`；三种视频路径在 Progress `204` 后，五分一秒时 Range 从首次 `302` 转为 Emby `206`。这些是现状复现，不是修复完成或真实客户端证据。

外部参考固定为 [MediaWarp `09fac019`](https://github.com/AkimioJR/MediaWarp/blob/09fac019f228999dadd400d4421ab1c25be7fc20/constants/regexp.go)、[embyExternalUrl `91798cc2`](https://github.com/bpking1/embyExternalUrl/blob/91798cc2ce5e14646c6b939b9f5d1d8d137541e9/emby2Alist/nginx/conf.d/emby.conf) 和 [emby-toolkit `cbc60e7a`](https://github.com/hbq0405/emby-toolkit/blob/cbc60e7a8f3af63f83eecae18b7a68c40f7864fc/reverse_proxy.py#L1924-L1994)。仅参考协议形态、图片/视频职责分离与按需准备流程，不复制上游实现、管理员 Key 注入或 UA 绕过，也不据上游声明承诺 Ember 客户端兼容。

## 方案设计

### 1. 已确认决策与准入合同

| 场景 | 严格模式（默认） | 兼容模式（管理员开启） |
| --- | --- | --- |
| 有效目标或媒体/直链缓存可复用 | 通过当前身份、账号、租约等检查后复用 | 相同 |
| 合格原文件 GET，目标缺失，有有效明确起播许可 | 依既有流程检查配额并转存 | 相同，记录明确起播来源 |
| 合格原文件 GET，目标缺失，无明确起播许可 | `playback_intent_required`，回退 Emby | 以兼容策略准入，再检查配额并转存；可能是预读 |
| HEAD | 只沿现行租约/目标复用路径，绝不新转存 | 相同 |
| 身份、媒体证明、原文件资格或账号不合格 | 沿既有 reject/fallback 分类 | 相同，不因兼容模式绕过 |
| 配置缺失、非法或读取失败 | 使用严格规则 | 不给出额外兼容准入；已有明确起播仍按严格合同处理 |

兼容模式作用于所有客户端，不维护播放器白名单。它是独立的转存准入策略，不生成或续期“用户明确起播”的证明；客户端 query false 或 body 缺失保持原样。模式关闭后，在配置缓存窗口内收敛到严格规则，不承诺撤销已经开始的外部转存。

### 2. 图片与会话职责

- 将现有精确 `/emby/Items/{Id}/Images/{Type}` 及可选规范非负 int32 Index 的 GET/HEAD 图片路由，从网页访问开关中分离；保留层级、长度、编码路径与尾斜杠限制。
- 携 Token 的图片仍检查本地映射、撤销和用户状态；空、重复或冲突 Token 不得退化为匿名请求。无 Token 图片原样交给 Emby，不注入额外凭据。网页关闭行为及其他受保护路径保持。
- 只对 `Sessions/Playing`、`Sessions/Playing/Progress`、`Sessions/Playing/Stopped` 三个固定 POST 端点的语义路径做大小写不敏感分类；保留方法、层级、尾斜杠边界和原请求转发。
- 只有上游成功接受的事件才续租、释放和撤销许可；失败、取消与未找到租约继续遵守现行状态边界，不从事件重建丢失租约。

### 3. 固定会话的证明重新核验

- 对当前原文件资格允许的固定流请求，在证明过期时通过当前用户 Token 向同一 Emby 重新查询；不使用管理员身份，不扩大转码、选流、歧义参数或未知文件名资格。
- 实现复用既有 live session binding，不增加过期 proof 缓存：所有成功 302 都可记录初始 30 秒媒体关联，GET 不延长活动期，只有成功命中既有租约的 Playing/Progress 按 active/paused 续活。proof 已过期或被清理但关联仍活跃时可重新核验；缺少、过期或停止后的关联仍回退。
- 重新核验必须匹配当前 server/user/mapping/device、item/source、源路径、容器和已知 Size；保留原客户端会话与内部查询会话的可信关联。证据不足或查询失败按原请求语义回退，不强行采用新媒体源。
- 不简单延长旧证明 TTL；复用现有请求合并、发布代次、取消和停止隔离约束。旧异步结果不能覆盖新证明、恢复已失效许可或在途租约。
- 重新核验本身不产生严格模式起播许可；兼容模式下是否可新增转存仍由本次 GET 的独立准入判断决定。

### 4. 配置、数据与管理入口

- 配置键 `P115_TRANSFER_TRIGGER_MODE`，值为 `strict` / `compatible`，默认 `strict`；只通过既有设置中心及 `settings` 表管理，不读取环境变量，不新增专用配置入口。
- 复用 Gateway 五秒配置缓存合同；数据库读取正常时，保存后后续请求最多经过一个五秒缓存窗口收敛。过期读取失败或非法值不沿用兼容授权，不阻断本可按严格规则完成的播放；配置异常不回写 115 账号故障。
- 在开始新增转存的准入边界使用当前有效策略；复用内容锁与二次查重，随后才进入现有配额、任务和 Provider 流程。模式切换不改变已存在目标或已开始任务的归属。
- 设置中心复用现有枚举字段展示，只保留一条影响决策的短说明，例如“兼容模式下，视频预读也可能触发转存”。前端实现必须遵守 Ember 风格。设计与交互基线以 [web-design-guide.md](../../reference/web-design-guide.md) 为准。偏离规范必须写明原因和收口条件；本方案不预设偏离。
- 本次未改变数据模型、表、列、索引或约束，不需要 SQL migration；新设置缺失时按定义返回严格默认值，保存仍使用既有 settings 接口。

### 5. 失败、并发与诊断边界

- 所有模式复用账号健康隔离、Redis 播放准入、转存配额、目标唯一性复核及失败释放；并发请求在锁内发现目标已出现时直接复用，不重复转存或计入转存用量。
- Token 撤销、用户禁用、账号配置变更、旧请求取消及 proof 替换仍须在各原有边界生效；兼容准入不得借用其他用户、设备、媒体源或新请求的授权。
- 模式关闭或许可过期只约束尚未开始的转存；已经发出的外部操作不承诺回滚。部分写入及取消沿现有可恢复任务与配额收口语义处理。
- 唯一视频决策日志须区分明确起播、兼容请求、既有目标复用与策略拒绝；不得把兼容准入标为明确起播，不输出 Token、Cookie、完整直链或原始请求/响应正文。

### 6. 分批实施与进度

- [x] 完成代码、版本合同、历史请求与公开实现调查；复现现有兼容缺口。
- [x] 用户确认严格默认、全局兼容可选，并接受兼容模式的预读副作用。
- [x] 完成批次 1 图片/会话修复，取得失败回归后完成实现，保留 Token 与路径负向边界。
- [x] 完成批次 2 live binding 重新核验与停止/取消/乱序/身份变化回归。
- [x] 完成批次 3 配置、运行装配、锁内策略与日志归因，设置中心复用既有枚举控件。
- [x] 完成本地测试和独立代码审查；发现的旧 plain 补查覆盖新会话别名问题已先补失败回归再修复，复核后无剩余 P0/P1/P2。
- [x] 同步现行合同、架构、配置、客户端矩阵和排障入口。
- [x] 经授权将 e78d8f80 部署到 hz，四容器 revision/健康及稳定性检查完成。
- [ ] 完成真实客户端分项验收；部署健康不等于播放验收，继续保留计划。

| 批次 | 启动依赖 | 完成与验收条件 | 本批次归档条件 |
| --- | --- | --- | --- |
| 1：图片与会话 | 现有匿名图片/Token 合同与本计划范围已确认 | 网页开关不再决定合法图片响应；标准/小写/混合大小写事件具有相同租约与停止效果；负向路径和响应透传回归通过 | 本批次代码、测试及稳定合同同步完成，真实图片/事件验收完成或用户明确接受未验证边界；随整份计划归档 |
| 2：续播重新核验 | 批次 1 的会话处理已完成本地验证 | 固定流在证明过期后重新核验；错误身份、源变化、并发、停止/取消与严格意图不重授予回归通过 | 本批次代码、测试及合同同步完成，真实拖动/重播验收完成或用户明确接受未验证边界；随整份计划归档 |
| 3：首次转存兼容 | 批次 1、2 本地验证完成，配置与准入表已收口 | 严格默认、兼容首次转存、HEAD 禁写、配置刷新/失败/切换、配额/去重/失败清理和日志归因回归通过 | 三批次全部收口；设置保存生效和真实首次转存验收完成或用户明确接受未验证边界，再按末节整体归档 |

### 7. 后续协议兼容改进（2026-10-25）

调查基线为 `master@30fb9a4d`。重新核对三个公开仓库的默认分支，固定提交与本计划引用一致；这些实现不能替代 Ember 的本地身份、媒体源、HEAD 和转存准入合同。

- 认证 scheme 当前按大小写敏感方式识别，12 个纯 mock 场景确认标准写法通过而小写/混合写法被本地拒绝；[RFC 9110 §11.1](https://www.rfc-editor.org/rfc/rfc9110.html#section-11.1) 要求 scheme 大小写不敏感。修补仅影响 scheme 比较及诊断分类，Token 原字节、字段 grammar、重复和冲突保护保持。
- 新增请求回归矩阵锁定多媒体源按 ID 选择、音轨/字幕/空数组/未知字段原字节保留、实际视频 UA、条件 Range 的 200/206/416 和非加速接口透传。Audio/Sync 只证明通用代理行为，Download 只证明上游授权响应保留，不据此宣称新增权益实现。
- 14 个临时 mock 场景确认 plain static stream 无 session 可补查，而扩展名流和 original 无 session 仍回源。对照固定 SDK 后，新增候选限定为显式 `Static=true`、明确唯一 MediaSourceId、完全没有 PlaySessionId 的 `stream.{Container}` 与固定 `original.mp4/mkv`；新增 original 还必须提供 SDK 声明必填且与扩展名一致的 Container。仅允许身份 Token、MediaSourceId、Static、Container 和与 Principal 一致的 DeviceId；容器合法且不为 manifest，其他参数继续回源。新形态失败时保留原客户端 URL、Range 和 Header，不复用 plain stream 的权威地址重写。
- 无 Static 的 original 缺会话、缺 MediaSourceId、多源歧义、固定会话冷 proof 且无 live binding、未知参数的可忽略语义均没有新增证据，不据此扩展准入。严格/兼容模式、HEAD 禁新增、现有 proof/许可/租约寿命及停止/取消隔离保持。
- 跨域 Web 的 OPTIONS 预检 401 已用 mock 确认，但跨域 Web、115 下载/音频加速、云端 HLS 和字幕转换仍作为独立后续范围；本次不增加配置、匿名入口或管理员凭据注入。

进度：

- [x] 完成认证 scheme 红绿回归与最小修补；三种应用 Header 的大小写、Token 保真/冲突、grammar 和日志脱敏共 48 个子用例及相关认证回归通过。
- [x] 完成协议请求矩阵并验证现有代理边界；多源选择/实际视频 UA、条件 Range/HEAD、非加速媒体与拒绝透传的定向测试通过。
- [x] 核对扩展名首请求合同，9 个关键场景先失败后通过；新增五组行为测试及旧补查/原文件/许可/固定会话/发布取消定向回归通过。
- [x] 完成 Gateway/DirectPlay/embytoken 三包 race、全量非数据库测试、vet/build、独立代码审查及现行文档同步；本轮增量未发现未收口代码问题，CORS 等明确排除的已有边界不视为已解决。
- [ ] 按实际部署提交、客户端和版本执行真实验收；目前仍保持未验收边界。

验证版本为 `master@30fb9a4d75dfb2d7371f55c39d967a42e72b82d9` 加本轮后续增量。`go test ./... -skip 'Integration|PostgreSQL' -count=1 -timeout=180s`、`go test -race ./internal/playbackgateway ./internal/services/directplay ./internal/services/embytoken -skip 'Integration|PostgreSQL' -count=1 -timeout=180s`、`go vet ./...` 和 `go build ./...` 均通过。新增测试使用合成 fixture、进程内 Transport/Recorder，不代表任何新增客户端版本已实机验收；本次无配置、模型或 schema 变更，不新增 migration，也未重新执行未变更的 Web/Bot 验证。

### 8. 采集、缓存诊断与 preID 复用（2026-10-25）

前述协议增量已提交为 `8799688ac555e91ba584bfe4c3ee41dc8de9bcb4`。用户随后授权按顺序优化以下三项；基线只读调查与离线样本不代表性能改善或真实客户端验收。

1. **采集工具补齐**：安全接纳现有认证诊断和通用请求摘要；PCAP 增加有界客户端 family/version 与登录、图片、字幕等固定路由类别。旧输出字段保持，在离线结果中增加诊断摘要，区分本地最终状态、明确上游状态和 unknown。只在唯一且一致的 requestId 内关联 Gateway 日志，重复或冲突事件标记歧义，不按时间猜测 PCAP 与日志会话，不输出凭据、原始 body、任意路径或 URL。
2. **缓存原因诊断**：保留媒体/直链 hit、miss、bypass 结果，增加有界固定原因，仅描述当前 lookup 可证实的缺失、过期、时钟回退、新鲜度或依赖 URL 失效；无法归因的 miss 不猜测配置变化或容量淘汰。容量驱逐计数为实际触及 cache 的进程内累计快照，不是每请求增量；离线摘要不得求和。缓存键、容量、TTL、淘汰顺序和准入保持。
3. **preID 成功复用**：先用两个用户、不同个人 playback 目标且相同 source 文件的 fake 请求测定前缀读取次数，保持两份目标写入、任务与配额工作量相同，再实现有界进程内成功缓存和同键在途读取串行/复查。只缓存经过 SHA1/字节数校验的前至多 128 KiB 哈希；源账号、Provider 身份、干净配置代次、精确凭据及 Provider 参数、文件 ID/pickCode/完整 SHA1/Size 与读取范围共同隔离。实现采用固定 1024 条和 10 分钟非滑动 TTL，无持久化、后台刷新或新配置；source 恢复探测/未知版本直接 bypass。

preID 查询仍位于目标双查、转存许可、配额预留及任务开始之后；实际读取成功且调用未取消才发布成功结果。结果无需等待目标上传结束，目标流程失败不撤销已证实的内容哈希。复用现有 requestGate，持有者使用自己的请求上下文，取消/失败不发布，后续等待者可重新读取；不复制任务、配额、健康结果或 owner 的 Provider 计时。`preIDCalls` 只计实际 HashFileRange；缓存命中与等待单独用固定状态/阶段表达。challenge 仍实时读取，不复用 preID 缓存。

本轮不增加图片/字幕内容缓存、详情预热、通用失败缓存、连接池调参或新的取消状态机；不修改 API/数据库 schema、配置入口和前端。source 实际路径观察继续按原合同报告健康，preID 缓存不得冒充网络恢复成功。

进度与证据：

- [x] 采集补齐与脱敏、歧义、离线摘要红绿验证通过；Python 3.9.6 原目录及独立复制的 39 项测试通过，10 个源码编译/语法检查通过，编译产物仅在临时目录。嵌套 JSON、合法旧原因被过滤和有可信绑定的双会话被误判三项审查问题已修复并复核。
- [x] 缓存原因及容量快照七组红绿测试通过，DirectPlay 非集成全包/race/vet/build 通过，原命中/失效和 Debug 事件行为保持。
- [x] 两个个人目标的等工作量 preID 基线与优化后读取次数验证通过；顺序和真实 gate 等待的重叠场景均由两次降为一次，源解析 2、目标查重 4、上传/目标复核/取链各 2、独立任务 2、每用户配额 1 保持。缓存/取消/恢复探测/门控/配额/任务隔离及同范围 challenge 实读回归通过。
- [x] 全量非数据库测试、相关五包 race、vet/build、离线 Python 测试和独立代码审查通过；现行文档同步完成，Go 实际 LogFields 经 Python 过滤及摘要的跨语言复验通过。
- [ ] 在已部署版本上按授权衡量真实命中率与准备耗时；不据 fake 调用次数或容器健康宣称首帧改善。

验证基线为 `master@8799688ac555e91ba584bfe4c3ee41dc8de9bcb4` 加本节增量。在 `services/api` 执行 `go test ./... -skip 'Integration|PostgreSQL' -count=1 -timeout=180s`、`go test -race ./internal/services/directplay ./internal/services/p115account ./internal/services/p115quota ./internal/integrations/p115 ./internal/playbackgateway -skip 'Integration|PostgreSQL' -count=1 -timeout=180s`、`go vet ./...`、`go build ./...` 均通过；Python 使用合成报文/日志和 mocked SSH。该批本地验证没有启动项目服务或连接真实 Emby/115/数据库，也没有部署或取得实际客户端性能数据；后续部署结果见第 10 节。

### 9. 持续自动化检查（2026-10-25）

诊断与 preID 优化已提交为 `3e1cd42c528bea8a14a0c41329c31f17d86aab18`。用户确认按单 Gateway 范围继续落实以下三项；本节仅增加测试和 CI 入口，不改变播放、认证、转存、缓存或部署行为。

1. **采集工具离线测试接入 CI**：Python 3.9/3.11 分别运行工具测试和测试入口自身的受控测试。路径过滤必须覆盖单独修改 `scripts/gateway-capture/` 的情况，统一 Makefile 和报告入口提供同名专项。
2. **关键 Go 包 race 常态化**：Gateway、DirectPlay、115 账号、配额与 Provider 五包使用 `-race`、非数据库范围和显式超时，在独立 job 执行；不启动项目服务或为本项新增数据库服务。
3. **永久 Go→Python 日志合同**：Go 测试经真实诊断格式化/日志方法产生合成记录，Python 使用采集工具实际过滤与摘要方法验证字段、枚举、脱敏及关联语义。缺少 producer、Go 测试失败、字段遗漏或摘要语义变化都应使检查失败，不能以空输入通过。普通 Go 测试不依赖 Python，独立复制的采集工具仍不依赖 Go。

三个入口统一保留各自日志和退出状态，CI 汇总明确区分成功、失败、跳过与取消。API、采集工具、对应 runner 和 workflow 改动应触发实际受影响的检查；契约检查覆盖两端变化。本地仅执行新增离线入口，不运行可能启动服务或访问数据库的旧整套入口。

- [x] 完成三个 runner、CI job 的触发与汇总配置，以及统一 Makefile/报告入口；`make test` 复用原 API 全包 race，不重复五包专项。
- [x] 完成 Go producer/Python consumer 合同及异常、脱敏、关联边界回归；Go stdout/stderr 分开保存，依赖下载提示不混入 JSON。
- [x] 完成本地专项、入口失败传播、配置一致性验证与两轮独立审查，同步测试指南、架构与计划索引。
- [x] 修正采集矩阵 runner 后，Python 3.9 已实际执行并通过，整条 Test 工作流通过；首次失败记录保留如下。

验证基线为 `master@3e1cd42c528bea8a14a0c41329c31f17d86aab18` 加本节增量。实际执行三个新增 runner：Python 3.9.6/3.11.15 各通过 39 项采集测试和 20 项 runner/校验器测试；Go 1.24.13 五包 race 通过；Go→Python 合同通过 3 个 timing 场景、31 个字段、4 个 Gateway 场景及 6 类会话冲突。两个新增 Go 测试所在包的 `go vet`、shell 语法、Makefile dry-run、YAML 结构/路径检查、汇总状态模拟及文档链接检查通过，独立审查未发现剩余 P0/P1/P2/P3。本地使用已有 Go/Python 与临时编译缓存，关闭 Go 模块下载；未执行旧完整 API/Web/Bot 入口，未启动项目服务或连接真实业务依赖。该次本地验证未执行 GitHub 工作流、部署或真实播放；首次远端结果另记如下。

2026-10-25 首次远端 [Test 工作流 40187234649](https://github.com/konghanghang/ember/actions/runs/40187234649) 对应 `95269111`：API、Web、Bot、Python 3.11 采集、五包 race 和跨语言契约均成功；Python 3.9 在 Setup Python 阶段失败，原因是 Ubuntu 26.04 没有匹配构建，工具测试未执行，随后缺少 artifact 为连带结果。采集矩阵已修复为固定 Ubuntu 24.04，保留 3.9/3.11 覆盖；修复提交 e78d8f80 的后续复验已全部通过，见第 10 节；首次运行本身仍记为失败。

本节随主计划保留；完成本地自动化不替代部署、真实客户端与性能验收，也不触发多实例实现或自动外部验证。

### 10. CI 复验与 hz 部署（2026-10-25）

- 目标提交为 `e78d8f8058bc6708465606093c03af81d7fcb017`。[Test 40188457875](https://github.com/konghanghang/ember/actions/runs/40188457875) 全部通过，包含实际执行的 Python 3.9/3.11、API、Web、Bot、五包 race 和日志契约；随后 master 快进合入 pre_release 并推送。[API](https://github.com/konghanghang/ember/actions/runs/40188866570)、[Web](https://github.com/konghanghang/ember/actions/runs/40188866545)、[Bot](https://github.com/konghanghang/ember/actions/runs/40188866534) 三个镜像工作流均以同一 SHA 成功完成。
- 经 `local → dmit → hz` 更新 `hz-fsn` 的四个应用容器，实际 Compose 入口为 `/usr/local/program/ember/docker-compose.yaml`；拉取后的镜像 OCI revision 与目标 SHA 一致，版本为 `preview-e78d8f8`。从当时运行的 `b1f524f5` 到目标提交，没有 SQL、数据模型或 Compose 配置变更；未改生产配置或执行人工数据库写入。
- API、Gateway、Web、Bot 均运行目标镜像且健康。首次 Compose 等待因 Bot unhealthy 返回失败，日志出现 polling 租约占用提示；容器自动重试后恢复，当前进程已启动 polling。只读事务核对租约有效、owner 与当前 Bot 启动记录匹配，后续检查确认更新时间推进。
- 在 11:45:24–11:49:24 UTC 的 4 分钟观察窗口内，容器 ID、启动时间和重启次数保持不变，四容器健康；当时重启次数分别为 API 0、Gateway 0、Web 1、Bot 10。Gateway 启动日志确认本次执行 0 份 SQL；API 迁移执行计数未取得日志证据。
- 只读检查确认 `P115_TRANSFER_TRIGGER_MODE` 没有数据库记录，按现行合同使用默认 `strict`，没有开启兼容模式。本次只完成部署和运行健康核对，Yamby 首页图片、SenPlayer 首次/复用转存、Infuse 回归、CDN/字幕、个人账号/Redis 故障路径及真实性能收益仍未实测，下面承接清单继续保留未验收状态。

## 历史计划验收承接

负责人：Ember。2026-10-25 承接 [起播许可历史计划](../../archive/plan/architecture/p115-transfer-playback-intent.md) 和 [SenPlayer 原文件历史计划](../../archive/plan/architecture/senplayer-original-stream-compatibility.md) 的全部未完成客户端分项，两份旧计划归档后在此维护结果。个人 Cookie、账号路由与 Redis/配额的受控清单独立由 [端到端参考 12.1](../../reference/p115-playback-end-to-end-flow.md#121-个人账号与-redis-待验收清单) 承接。

验收基线与证据范围：

- 2026-09-22 用户反馈正常播放没有问题；当日详情探测日志证明无明确许可时跳过新增转存并回退 Emby `404`。这些记录不覆盖首次新增转存、停止后浏览/重播或 thumb 因果验证。
- 2026-10-14 SenPlayer `6.0.4` 样本为旧 Gateway 的 Emby 回退，受限 MKV 复用提交 `4c9730808f32dd64783a44ae36b0eb2aa3089dc6` 的已有目标/缓存、无目标拒绝及 HEAD 仅有本地 fake/race 证据。
- 2026-10-23 经 `local → dmit → hz` 更新四个业务容器的历史 revision 为 `b1f524f5a81b5507d505b272b96355c82ce7da38`，原 revision 为 `2a213f35`；Git 祖先关系确认其中包含上述 MKV 和既有账号实现。这是截至该日的部署记录，不是本轮线上版本读数，也不补齐任何客户端分项验收。
- 2026-10-25 的兼容实现 `30fb9a4`、协议增量 `8799688`、诊断/preID `3e1cd42` 和持续自动化 `33eb11f7` 仍只有前文记录的本地证据；未取得本轮远端 CI、部署、真实客户端或性能结果。后续验收须先记录实际部署 revision，不能用 10 月 23 日版本验收这些增量。

后续真实操作继续单独授权，并按 [抓包手册](../../runbooks/gateway-packet-capture.md) 记录客户端精确版本/平台、Emby 版本、实际模式、账号路由、目标是否已存在、缓存条件、操作时序和脱敏日志。以下状态均为“未验收”；旧计划的“已转交”只表示交接完成，不表示操作通过。

| 编号 / 原待验收项 | 操作与所需证据 | 当前状态 |
| --- | --- | --- |
| GW-H01：严格模式首次新转存 | 使用缺失目标，在 `strict` 下对照客户端明确起播 POST、有效许可、锁内目标检查、一次任务/配额提交与最终视频决策；记录实际播放器结果。不能由普通播放反馈推断本次确实新建了目标。 | 未验收；许可/无副作用门控已有 fake，9 月 22 日正常播放反馈未分离首次目标场景。 |
| GW-H02：严格模式停止后再次浏览 | 成功 Stopped 后只浏览详情，不重新明确起播；核对旧许可已撤销，缺失目标不新建任务/配额/转存，已有目标仍可按合同复用。记录实际详情请求、上游停止结果和后续决策。 | 未验收；停止撤销和旧请求隔离已有 fake/race，未取得分项实机对照。 |
| GW-H03：严格模式重播 | 停止后重新点击播放，记录新的明确起播、原/新会话、旧缓存复用或新目标准入；核对旧许可没有被补查或缓存命中续期，并记录重播实际效果。 | 未验收；自动化覆盖停止、过期、代次替换，不替代客户端重播。 |
| GW-H04：SenPlayer 已有目标 | 选定账号已有匹配文件、媒体缓存未命中时，请求合格 `original.mp4/mkv`；确认目标查重复用、无新增任务/转存计费及实际 `302`。同时记录严格或兼容模式，避免把模式变化误记为原文件识别修复。 | 未验收；10 月 14 日分层 fake 覆盖已有/锁内出现目标，旧实机样本只有 Emby 回退。 |
| GW-H05：SenPlayer 缓存复用 | 先取得有效目标与缓存，再同会话或跨会话访问；分别记录媒体/直链 hit、身份/租约检查和目标未重复写入，停止后重播仍须独立满足当前许可/复用合同。 | 未验收；缓存、租约和跨会话复用已有 fake，未取得真实命中与播放结果对照。 |
| GW-H06：SenPlayer 缺失目标 | 缺少明确起播许可时，`strict` 应以 `playback_intent_required` 回退且不执行任务/配额/preID/challenge/秒传；另在已部署且经授权开启 `compatible` 后核对独立兼容准入，允许预读产生首次转存。HEAD 两种模式都不新增。 | 未验收；严格拒绝与兼容准入各有本地回归，10 月 23 日部署不包含 10 月 25 日兼容增量。 |
| GW-H07：详情浏览 | 分别记录 Infuse/SenPlayer 只进详情而不点击播放时的图片、PlaybackInfo、视频及事件请求；区分已有目标复用、严格拒绝和兼容预读转存，不把视频 GET 视为明确起播。Emby `404` 的部署媒体可读性与客户端显示效果分别记录。 | 未验收；Infuse 有 9 月 22 日详情探测日志，SenPlayer 尚无本轮分项对照；thumb 原因见下方独立观察项。 |
| GW-H08：拖动与续播 | 对照首次读取、连续 Range、五分钟 proof 到期后拖动及重新核验；记录原会话关联、最终 302/Emby 回退、CDN 状态与实际跳转/恢复效果。 | 未验收；本地时间控制回归不证明真实客户端与 CDN 的续播表现。 |
| GW-H09：暂停、停止与重播 | 分别操作暂停/恢复、停止、再次播放，对照 Playing/Progress/Stopped 时序和成功响应、租约续期/释放、许可撤销、新会话与实际播放；事件 `204`、租约更新和播放器结果分别记录。 | 未验收；已有事件/TTL fake，客户端暂停上报间隔和这些操作的完整实机链路未分项取得。 |
| GW-H10：真实 302 与 CDN Range | 确认 SenPlayer 实际跟随 playback `302`，按实际请求核对 CDN `200/206/416`、Content-Range、字节范围、总长度和 UA 适配；按手册脱敏保存证据，不把 Gateway 空体 `302` 或旧 Emby `206` 当作 CDN 通过。 | 未验收；10 月 14 日抓包是旧部署回退，10 月 23 日镜像部署不提供 CDN 字节证据。 |
| GW-H11：字幕与完整播放 | 分别观察所用内嵌/外挂字幕、首帧、拖动后显示和完整播放；记录客户端/媒体条件与操作者结果。短 Range、Playing、响应头到达或首帧均不替代完整播放验收。 | 未验收；历史 Infuse 样本不扩展为 SenPlayer 或 10 月 25 日新增形态通过。 |

### 独立观察：Infuse thumb 与详情视频探测

该项从起播许可计划独立转交，当前为 **未证实**，不作为必须补图的实施任务。2026-09-22 用户提出“单集缺少 thumb 诱发 Infuse 详情视频探测”的假设；已确认的是详情视频 GET 和严格门控后的 fallback `404`，尚未核对图片文件/Emby 元数据，亦未执行补图前后对照。原始现象与版本范围保留在 [端到端参考 1.4](../../reference/p115-playback-end-to-end-flow.md#14-infuse-详情页读取视频与缺失-thumb-的排查记录)。

若后续获授权，先核对同一单集 thumb 是否存在且能由 Emby 提供；确有缺图且决定做对照时，再按既有刮削流程补齐。保持客户端版本、浏览动作和媒体条件一致，记录 Infuse 缓存状态，并使用此前未打开的单集作对照，分别比较图片请求、视频探测和详情显示。只有该对照才能补充因果证据；正常播放通过或探测 404 均不能证明假设，也不能把该观察项记为正式播放验收通过。

## 影响范围

| 子系统 | 影响 |
| --- | --- |
| API / Gateway | 有：`internal/playbackgateway/` 路由、会话、证明重新核验与准入；`internal/services/directplay/` 策略边界；`internal/config/` 设置定义、读取与测试 |
| Web | 有：既有设置中心的枚举配置展示与保存验收；优先复用动态表单，不新增页面或播放器专用开关 |
| Bot | 无：不新增命令、通知或 Internal API |
| 配置 / 部署 | 有数据库运行期配置；默认严格、不依赖环境变量，预计不改 Compose、镜像或 schema；部署仍单独执行 |
| 测试 / CI | 第 9 节增加采集离线、五包 race、跨语言日志合同的专项入口、路径触发与结果汇总 |
| 文档 | 有：本计划、两份关联计划、系统架构、播放代理合同、客户端矩阵、115 端到端参考、配置参考及测试/排障入口 |

## 验证方式

- 先基于调查中的最小场景补充预期正确行为的失败回归，再做最小实现，同时保留严格模式和无关请求的特征测试；新增或重写方法必须补有价值的方法注释，复杂方法体补必要说明，并覆盖正常、边界、错误和状态变化测试。
- 图片覆盖开关开/关、无 Token/有效/无效 Token、GET/HEAD、Index、非法路径和上游失败；会话覆盖三事件的大小写、2xx/非 2xx、取消及租约缺失。
- 续播使用 fake clock 覆盖五分钟边界，验证原会话、媒体身份、并发补查与停止/取消隔离；兼容模式覆盖预读可准入、严格模式仍拒绝、HEAD 永不写、锁内查重复用和配置失败不放宽。
- 2026-10-25 调查阶段已通过 `go test -count=1 -timeout=120s ./internal/playbackgateway ./internal/integrations/emby ./internal/services/directplay ./internal/services/p115account -skip 'Integration|PostgreSQL'`；该结果仅针对调查基线，不替代实施后的验证。
- 验证版本为 `master@4e3768ec9d5698adc092f8693f2f7cfda2d2f4c7` 加本次实现改动：Go 全量非数据库测试、config/Gateway/directplay/p115quota/p115account 五包 race、全量 vet/build 通过；设置中心 10 项组件测试及 Web 生产构建通过；抓包工具 19 项离线测试通过。最后的 Gateway 发布竞态修补另做相关回归/race 复验。全部外部依赖为 fake/mock，没有启动项目服务、连接真实数据库或请求真实 Emby/115。
- 实施后在 `services/api` 运行相关包定向测试与 race、`go test ./... -skip 'Integration|PostgreSQL' -count=1`、`go vet ./...`、`go build ./...`。如修改 Web 逻辑，在 `services/web` 执行相关 Vitest 与 `npm run build`，不依赖真实后端。
- 自动化必须 fake Emby、115、Redis 和数据库访问，不启动服务；只有确需且获得相应授权时才单独执行真实集成。测试命令与隔离要求沿用 [测试指南](../../runbooks/testing.md)。
- 真实验收按 [Gateway 抓包手册](../../runbooks/gateway-packet-capture.md) 另行授权：记录部署提交、Emby/客户端版本及平台，分别确认 Yamby 图片、SenPlayer 首次/复用转存、Infuse 严格模式回归、兼容预读、五分钟后拖动、字幕、暂停/停止及重播。302、206、事件成功和首帧/完整播放分别记录。

## 落地后文档处理

1. 各批次完成后同步 [系统架构](../../system-architecture.md)、[播放代理合同](../../reference/emby-playback-proxy-contract.md)、[客户端矩阵](../../reference/emby-client-compatibility-matrix.md)、[115 端到端参考](../../reference/p115-playback-end-to-end-flow.md)；批次 3 同步 [配置参考](../../reference/configuration-reference.md)。未实施时不得将方案写成现行行为。
2. 两份关联计划已完成历史状态收口，2026-10-25 归档并把未完成项交给本计划的 [历史计划验收承接](#历史计划验收承接)；严格策略和原文件资格的当前规则维护在现行参考，旧稿只保留过程证据，不把旧真实记录当成新行为验收。
3. 当各实施批次、后续协议/性能与持续自动化项满足各自完成条件，代码/测试/文档已收口，主计划及承接清单的真实验收完成或用户明确接受具体未验证边界后，本计划迁入 `docs/archive/plan/architecture/`；thumb 假设单独记录观察结论或明确未证实边界。同步计划、归档、盘点索引及全部直接引用。未满足时继续保留准确的待验收状态。
