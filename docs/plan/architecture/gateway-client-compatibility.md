# Gateway 客户端兼容与首次转存策略方案

> 状态：方案已确认，待实施
> 负责人：Ember
> 更新时间：2026-10-25

## 背景

用户确认 Infuse 当前正常，Yamby 登录已恢复但首页图片无法加载，SenPlayer 能登录和播放却不新增 115 转存。代码调查发现图片与网页开关耦合、会话路径大小写漏处理、固定会话续播缺少证明重新核验，以及严格起播策略与已记录 SenPlayer 请求形态不匹配。

2026-10-25 用户确认采用三批次方案：先修图片和会话，再完善续播，最后增加管理员可开启的全局兼容模式；默认保持严格模式，并接受兼容模式开启后包括 Infuse 在内的预读也可能产生转存。本文记录待实施方案，未改变运行行为，不代表已提交、部署或完成客户端验收。

## 目标

1. 网页访问关闭时，现有合同允许的媒体图片仍可交由 Emby 响应；三个固定播放事件端点按大小写不敏感方式执行本地处理。
2. 合格固定流请求的媒体证明过期后可以受限重新核验，继续复用有效 115 目标和直链，并保持原播放会话关联。
3. 提供严格默认、显式开启的全局兼容模式，让合格原文件视频 GET 在目标缺失时可首次转存，准入来源与明确起播许可分开表达。

## 非目标

- 不扩展 root 形态无 Token 图片、其他匿名 API、认证方式或任意 `original.*`；不按 User-Agent 放行，不注入管理员 API Key。
- 不重写客户端 `IsPlayback`、query、body、媒体能力字段或原始响应；不把 GET、Range、Playing 推断为可靠的用户起播证明。
- 不允许 HEAD 新增转存，不放宽原文件资格、身份、账号、租约、配额和任务所有权，不恢复已失效的在途租约。
- 不引入新的媒体传输 Provider、远程视频中转、自动删除、后台预热或跨进程证明缓存；不启动服务或自动实施真实外部验证。

## 当前事实

- 调查基线为 `master@98b20c98841554b100bb1cf4f02114c2f6267c5a`。版本依据为 Emby `4.9.3.0` SDK `6ee0155063bc85578196489926359a8f37419502`，运行版本范围与证据边界沿用 [播放代理合同](../../reference/emby-playback-proxy-contract.md)。
- `services/api/internal/playbackgateway/web_surface.go` 的 `isEmbyWebSurfaceRequest` 将无 Token 精确图片归为 Web Surface；`serveWebSurface` 在开关关闭时返回 `404 HTML`，不请求 Emby。这与 `internal/config/config.go` 的网页开关说明不符；Yamby 历史图片 404 已记录于 [客户端矩阵](../../reference/emby-client-compatibility-matrix.md)，本轮未读取部署当前配置和图片请求。
- `playback_session_log.go` 的 `playbackSessionEventKindForRequest` 只匹配标准大小写路径；小写事件仍可代理成功，但不触发本地租约与停止处理。
- `playback_proof.go` 的证明期限为五分钟；`playback_info_resolver.go` 的 `onDemandPlaybackInfoCandidate` 只接纳不带 `PlaySessionId` 的 plain static stream。固定会话的 `original.mp4/mkv` 和 `stream.*` 过期后在 `video.go` 回退，Progress 不刷新证明。
- `playback_info.go` 排除 query 起播字段，`playback_transfer_intent.go` 要求 JSON 唯一布尔 `IsPlayback=true`；`services/directplay/service.go` 在锁内二次查重后检查许可，先于配额、任务、preID/challenge 和秒传。已记录 SenPlayer query `IsPlayback=false`、body 缺该字段，只能复用已有目标。
- 严格模式依赖 [起播许可方案 §1–§3](./p115-transfer-playback-intent.md)；原文件资格依赖 [SenPlayer 方案 §1–§2](./senplayer-original-stream-compatibility.md)。两份旧方案的历史实现与验证记录保留，本方案新增兼容策略，不将其写成旧行为已经失效。
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
- 重新核验必须匹配当前 server/user/mapping/device、item/source、源路径、容器和已知 Size；保留原客户端会话与内部查询会话的可信关联。证据不足或查询失败按原请求语义回退，不强行采用新媒体源。
- 不简单延长旧证明 TTL；复用现有请求合并、发布代次、取消和停止隔离约束。旧异步结果不能覆盖新证明、恢复已失效许可或在途租约。
- 重新核验本身不产生严格模式起播许可；兼容模式下是否可新增转存仍由本次 GET 的独立准入判断决定。

### 4. 配置、数据与管理入口

- 拟定配置键 `P115_TRANSFER_TRIGGER_MODE`，值为 `strict` / `compatible`，默认 `strict`；只通过既有设置中心及 `settings` 表管理，不读取环境变量，不新增专用配置入口。
- 复用 Gateway 五秒配置缓存合同；数据库读取正常时，保存后后续请求最多经过一个五秒缓存窗口收敛。过期读取失败或非法值不沿用兼容授权，不阻断本可按严格规则完成的播放；配置异常不回写 115 账号故障。
- 在开始新增转存的准入边界使用当前有效策略；复用内容锁与二次查重，随后才进入现有配额、任务和 Provider 流程。模式切换不改变已存在目标或已开始任务的归属。
- 设置中心复用现有枚举字段展示，只保留一条影响决策的短说明，例如“兼容模式下，视频预读也可能触发转存”。前端实现必须遵守 Ember 风格。设计与交互基线以 [web-design-guide.md](../../reference/web-design-guide.md) 为准。偏离规范必须写明原因和收口条件；本方案不预设偏离。
- 本次不涉及数据模型、表、列、索引或约束变更，预计无需 SQL migration；若实施发现必须改变 schema，应先更新计划并补对应幂等 migration，不能依赖 AUTO_MIGRATE。

### 5. 失败、并发与诊断边界

- 所有模式复用账号健康隔离、Redis 播放准入、转存配额、目标唯一性复核及失败释放；并发请求在锁内发现目标已出现时直接复用，不重复转存或计入转存用量。
- Token 撤销、用户禁用、账号配置变更、旧请求取消及 proof 替换仍须在各原有边界生效；兼容准入不得借用其他用户、设备、媒体源或新请求的授权。
- 模式关闭或许可过期只约束尚未开始的转存；已经发出的外部操作不承诺回滚。部分写入及取消沿现有可恢复任务与配额收口语义处理。
- 唯一视频决策日志须区分明确起播、兼容请求、既有目标复用与策略拒绝；不得把兼容准入标为明确起播，不输出 Token、Cookie、完整直链或原始请求/响应正文。

### 6. 分批实施与进度

- [x] 完成代码、版本合同、历史请求与公开实现调查；复现现有兼容缺口。
- [x] 用户确认严格默认、全局兼容可选，并接受兼容模式的预读副作用。
- [ ] 完成以下三批次实现、正式回归、独立审查与现行文档同步。

| 批次 | 启动依赖 | 完成与验收条件 | 本批次归档条件 |
| --- | --- | --- | --- |
| 1：图片与会话 | 现有匿名图片/Token 合同与本计划范围已确认 | 网页开关不再决定合法图片响应；标准/小写/混合大小写事件具有相同租约与停止效果；负向路径和响应透传回归通过 | 本批次代码、测试及稳定合同同步完成，真实图片/事件验收完成或用户明确接受未验证边界；随整份计划归档 |
| 2：续播重新核验 | 批次 1 的会话处理已完成本地验证 | 固定流在证明过期后重新核验；错误身份、源变化、并发、停止/取消与严格意图不重授予回归通过 | 本批次代码、测试及合同同步完成，真实拖动/重播验收完成或用户明确接受未验证边界；随整份计划归档 |
| 3：首次转存兼容 | 批次 1、2 本地验证完成，配置与准入表已收口 | 严格默认、兼容首次转存、HEAD 禁写、配置刷新/失败/切换、配额/去重/失败清理和日志归因回归通过 | 三批次全部收口；设置保存生效和真实首次转存验收完成或用户明确接受未验证边界，再按末节整体归档 |

## 影响范围

| 子系统 | 影响 |
| --- | --- |
| API / Gateway | 有：`internal/playbackgateway/` 路由、会话、证明重新核验与准入；`internal/services/directplay/` 策略边界；`internal/config/` 设置定义、读取与测试 |
| Web | 有：既有设置中心的枚举配置展示与保存验收；优先复用动态表单，不新增页面或播放器专用开关 |
| Bot | 无：不新增命令、通知或 Internal API |
| 配置 / 部署 | 有数据库运行期配置；默认严格、不依赖环境变量，预计不改 Compose、镜像或 schema；部署仍单独执行 |
| 文档 | 有：本计划、两份关联计划、系统架构、播放代理合同、客户端矩阵、115 端到端参考、配置参考及测试/排障入口 |

## 验证方式

- 先基于调查中的最小场景补充预期正确行为的失败回归，再做最小实现，同时保留严格模式和无关请求的特征测试；新增或重写方法必须补有价值的方法注释，复杂方法体补必要说明，并覆盖正常、边界、错误和状态变化测试。
- 图片覆盖开关开/关、无 Token/有效/无效 Token、GET/HEAD、Index、非法路径和上游失败；会话覆盖三事件的大小写、2xx/非 2xx、取消及租约缺失。
- 续播使用 fake clock 覆盖五分钟边界，验证原会话、媒体身份、并发补查与停止/取消隔离；兼容模式覆盖预读可准入、严格模式仍拒绝、HEAD 永不写、锁内查重复用和配置失败不放宽。
- 2026-10-25 调查阶段已通过 `go test -count=1 -timeout=120s ./internal/playbackgateway ./internal/integrations/emby ./internal/services/directplay ./internal/services/p115account -skip 'Integration|PostgreSQL'`；该结果仅针对调查基线，不替代实施后的验证。
- 实施后在 `services/api` 运行相关包定向测试与 race、`go test ./... -skip 'Integration|PostgreSQL' -count=1`、`go vet ./...`、`go build ./...`。如修改 Web 逻辑，在 `services/web` 执行相关 Vitest 与 `npm run build`，不依赖真实后端。
- 自动化必须 fake Emby、115、Redis 和数据库访问，不启动服务；只有确需且获得相应授权时才单独执行真实集成。测试命令与隔离要求沿用 [测试指南](../../runbooks/testing.md)。
- 真实验收按 [Gateway 抓包手册](../../runbooks/gateway-packet-capture.md) 另行授权：记录部署提交、Emby/客户端版本及平台，分别确认 Yamby 图片、SenPlayer 首次/复用转存、Infuse 严格模式回归、兼容预读、五分钟后拖动、字幕、暂停/停止及重播。302、206、事件成功和首帧/完整播放分别记录。

## 落地后文档处理

1. 各批次完成后同步 [系统架构](../../system-architecture.md)、[播放代理合同](../../reference/emby-playback-proxy-contract.md)、[客户端矩阵](../../reference/emby-client-compatibility-matrix.md)、[115 端到端参考](../../reference/p115-playback-end-to-end-flow.md)；批次 3 同步 [配置参考](../../reference/configuration-reference.md)。未实施时不得将方案写成现行行为。
2. 更新两份关联计划的适用边界与本计划入口，保留原严格策略和旧客户端样本的历史来源；同步测试与排障文档，不把旧真实记录当成新行为验收。
3. 当三个批次均满足各自归档条件，代码/测试/文档已收口，真实验收完成或用户明确接受具体未验证边界后，本计划迁入 `docs/archive/plan/architecture/`；同步计划、归档、盘点索引及全部直接引用。未满足时保留“待实施/待验收”的准确状态。
