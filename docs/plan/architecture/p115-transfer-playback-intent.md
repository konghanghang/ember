# 115 新增转存起播许可实现方案

> 状态：代码与自动化验证完成，待受控客户端验收
> 负责人：Ember
> 更新时间：2026-09-22

## 背景与当前事实

- 用户确认只打开 Infuse 详情，2026-09-22 本机日志仍出现单集视频 GET、Gateway GET PlaybackInfo 补查、一次新转存和四次复用，没有 Playing/Progress/Stopped。正式播放的对照样本在视频 GET 前存在客户端 POST PlaybackInfo，携带 `IsPlayback=true`。
- Gateway proof 表示用户可访问媒体及其播放能力，不能证明用户点击播放；优化前意图字段只有诊断用途，DirectPlay 目标不存在便可进入新转存。本次保留普通 proof 资格，另加短期新增转存许可。
- 目标 Emby `4.9.3.0`，字段和时间语义沿用 [版本化播放代理合同](../../reference/emby-playback-proxy-contract.md)。客户端样本不代表其他版本或其他播放器必然提供同样信号。
- 后续确认当前部署只有 STRM 中的本地路径，没有把对应媒体挂载进 Emby。2026-09-22 20:26 的详情探测两次命中 `playback_intent_required`，没有新转存，Emby 回退返回 404；用户接受该探测结果，本任务不增加挂载或新取流链路。正式首次播放和重播仍待新版本客户端验收。
- 用户进一步提出“单集缺少 thumb，Infuse 详情页因此读取视频生成缩略图”的假设，尚未核对图片状态或执行补图对照，因果关系未证实。现象、可能的缩略图影响与验证方向记录于 [端到端参考：Infuse 详情页读取视频](../../reference/p115-playback-end-to-end-flow.md#14-infuse-详情页读取视频与缺失-thumb-的排查记录)。
- 后续 review 用固定时钟与 channel 控制的 fake 请求复现 P2：较早的内部 GET 晚于客户端明确起播 POST 返回，会无条件清除较新的起播 proof，导致正式播放回调从允许变为拒绝。此前自动化未覆盖此响应乱序；同时 P3 日志将策略跳过误写为“115直链失败”，本次一并收口。

## 目标与非目标

1. 未取得明确起播许可的请求不能创建新转存任务、申请转存配额或发起秒传；目标缺失时继续既有 Emby fallback。
2. 已有目标文件和十分钟跨播放会话缓存继续复用，权限、账号、租约等检查照旧。
3. 许可按身份、条目、媒体源和会话隔离；停止、过期和 proof 替换不能让旧请求借新许可继续创建任务。

不在本次范围：完全禁止详情页访问 115、取消所有预留并发、修改播放事件顺序、客户端名称特判、删除已转存文件、调整缓存期限、改变 Emby 回退协议。未明确起播的已存在文件仍可能返回 115 直链并短暂占用播放 reservation。

## 方案设计

### 1. 起播许可与 proof

- 复用进程内 proof 缓存，不新增数据库模型、表、migration、Redis key 或配置项。
- 客户端 POST PlaybackInfo 的 `IsPlayback` 必须是唯一且明确的布尔 `true`；重复逻辑字段（含大小写变体）、`null`、类型错误均不授予许可，但不改变既有透明代理和普通 proof 资格。
- 起播字段只按版本合同读取 JSON body；POST query 另带 `IsPlayback` 或 `MediaSourceId` 时不授予许可，不猜测其与 body 的绑定优先级。
- 仅在 Emby 返回合法成功响应并形成 proof 后授予许可。请求指定 `MediaSourceId` 时只授予匹配 source；未指定时只对唯一、合格的响应 source 授予，歧义不授予。
- Gateway 内部 GET、客户端 GET、视频 GET/HEAD 和证明/媒体缓存命中均不产生或延长许可。
- 许可从成功响应记录时固定有效 30 秒，同时受普通 proof 有效期限制；不使用媒体缓存的十分钟期限。它是短期重试许可，不是一次性票据。
- proof 每次写入具有新的进程内代次。视频请求绑定最初 proof，执行时重新核对当前身份、条目、source、session、路径和代次，防止旧排队请求借用后来同键的新许可。

### 2. DirectPlay 边界

- 内部 `MediaPathResolveRequest` / `ResolveRequest` 传入 `CanCreateTransfer` 回调，只返回当前是否仍可开始新增转存；缺失回调默认不允许。
- 先复用有效媒体缓存或已有目标。只有内容锁内二次查重仍不存在时，才检查许可，且检查早于转存配额、任务创建和 preID/challenge 等步骤。
- 许可不满足返回 `ErrPlaybackIntentRequired`；Gateway 使用 `reasonCode=playback_intent_required` 记录既有唯一最终决策并走权威 Emby fallback。它不属于 Provider/账号故障。
- HEAD 仅允许复用现有目标，即使已有租约也不允许新转存。
- 拒绝沿现有错误路径释放本次新建 reservation，不释放其他既有活跃租约；失败不占转存额度。
- 准入时许可有效便可继续已开始的本次转存。之后到达的 Stopped 或期限届满不会撤回已开始的外部操作；不承诺取消或回滚已经发送到 115 的请求。TTL 内失败可重试，成功后的并发请求继续由既有内容锁和查重收口。

### 3. 停止与兼容性

- 经身份校验且被 Emby 成功接受的 Stopped 撤销当时已记录的对应许可；该动作不依赖 Redis 是否找到租约或租约服务是否可用。不取消在途 PlaybackInfo；之后成功返回的显式客户端起播 POST 仍是新的授予事件，不新增停止 tombstone/epoch。
- MediaSourceId 存在时清精确 source；缺失时清同身份、条目和 session 的所有 source。其他用户、设备或 session 不受影响。
- 保留普通媒体 proof 与十分钟媒体/下载地址缓存，下一次客户端明确起播可取得新许可并复用缓存。
- 不等待 Playing，也不通过 Range、Purpose 或缺少 PlaySessionId 猜测播放意图。
- 未提供明确起播字段的播放器在目标不存在时回退 Emby；本次不承诺这些客户端仍能首次 115 加速。

### 4. 响应乱序与日志分类修复

- 内部 GET 在实际 singleflight owner 中同锁复查当前 proof，未命中才登记请求期发布令牌；缓存首次查询与登记不能留下锁外间隙。
- 令牌复用 proof 缓存的 mutex，区分内部 GET 与客户端响应观察，按 mapping/item 隔离且共用有界容量；失效但尚未退出的观察继续占容量，不淘汰仍在途令牌。实际 owner/响应观察所有退出路径释放，不保留持久 tombstone 或后台状态。
- 客户端响应入口原子执行旧快照/两类旧令牌失效，并登记本次客户端观察；终态只在该令牌仍有效时原子替换整个 item，失败、空或无效响应也以空快照结束。另一客户端已先开始更新时，旧响应成功或失败均不再修改新版快照。
- 内部 GET 发布时在同一锁内检查令牌并替换整 item 快照，保持原先完整响应使旧 source 失效的合同；只废弃同 item 其他内部令牌，不撤销仍在读取 body 的客户端观察。客户端终态会再次废弃读 body 期间新登记的内部发布，堵住失效/读取/写入之间的窗口；无关 mapping/item 不受影响。
- 过时的内部结果直接丢弃并走既有 fallback，不把新版客户端 proof/起播许可借给旧请求，也不延长许可或放宽 generation 检查。
- 该请求期保护不扩展 Stopped 语义：停止仍撤销当时已记录的许可，不取消尚未完成的新显式 POST。
- `playback_intent_required` 保持 `code=direct_play_fallback`，但输出 `directPlayResult=skipped` 与“无起播许可，跳过新增转存”；其他真实 DirectPlay 错误继续 `failure`。`fallbackResult`、HTTP 状态和 info/warn 仍由实际 Emby 响应决定，不把所有 404 静默降级。

## 影响范围

- Gateway：请求旁路解析、proof 缓存、视频请求与成功停止事件。
- DirectPlay：内部请求合同、锁内新增转存准入、稳定错误类型及回归测试。
- Web / Bot / 对外 API / 部署 / 数据库 schema：无变更。
- 文档：系统架构、Emby 播放合同、115 端到端参考与本实施方案。

## 验证与进度

- [x] 日志及代码路径确认；后续已确认无媒体挂载，用户接受详情探测的 Emby 404。
- [x] 确认变更边界与共享内部合同。
- [x] Gateway 先补失败测试，再实现严格意图解析、许可隔离和停止失效。
- [x] DirectPlay 先补失败测试，再实现无任务/无配额的拒绝与现有目标复用；该包 fake 与 race 已通过。
- [x] 覆盖重复字段、多 source、过期、时钟回拨、停止、同键 proof 替换、并发等待和 HEAD。
- [x] 全量 Go 非集成测试、相关 race、vet、build 与文档链接检查。
- [x] 补出旧内部 GET 覆盖新起播、恢复已失效证明以及策略跳过误分类的失败测试。
- [x] 响应发布原子保护、两类请求期令牌清理与日志分类修复；旧客户端 body 晚完成覆盖新许可的新增回归也已收口。
- [x] 补充修复后的全量 Go 非集成、关键 race、vet/build 和文档一致性复验。
- [ ] 受控 Infuse 操作与日志对照；未启动服务或真实触发 Emby/115。
- [ ] 核对单集 thumb 状态并进行补图前后对照，控制 Infuse 缓存影响；该假设验证与正式播放验收分别记录，不以详情探测 404 推断播放无影响。

验证命令在 `services/api` 工作目录执行：

```bash
go test ./... -skip Integration -count=1
go test -race ./internal/playbackgateway ./internal/services/directplay ./internal/services/p115quota ./internal/services/p115account ./internal/services/embytoken ./internal/integrations/emby -skip Integration -count=1
go vet ./...
go build ./...
```

自动化使用 fake Emby / Provider / Redis，不启动服务，不请求真实外部系统。真实 Infuse 验收需后续受控执行：只打开未转存单集、点击播放、停止后再次浏览和再次播放；实际播放器行为、CDN 与生产性能不由单元测试代替。

2026-09-22 首次实现的上述四条命令全部通过，集成测试显式排除；`git diff --check` 与现行文档相对链接检查通过。后续定向复核发现并复现了上面的 P2/P3，首次通过记录不覆盖本次新增乱序回归。没有变更 schema，也未执行 PostgreSQL 专项或真实 Redis/Emby/115 主动验证；用户产生的详情探测日志只证明门控生效，不代表正式播放已通过。

同日补充修复后重新执行上述四条命令，全部通过；Gateway/DirectPlay/配额/账号/Token/Emby 六个关键包 race 均通过。新增 fake 回归验证旧内部 GET、客户端 body 读取窗口及旧客户端终态乱序，空/失败响应不恢复旧证明，不同 source 竞争、容量和所有退出路径清理，以及日志 skipped 与真实上游结果分离。最终定向审查未发现仍需修复的问题，diff 和相对链接检查通过。未启动服务或执行真实播放取证。

## 落地后文档处理

稳定合同已同步到系统架构、Emby 播放合同与 115 端到端参考，本次乱序与日志修复的自动化清单已收口，保留正式客户端验收项。真实客户端取证完成，或用户明确接受该未验证边界后，归档至 `docs/archive/plan/architecture/` 并同步计划、归档和盘点入口。
