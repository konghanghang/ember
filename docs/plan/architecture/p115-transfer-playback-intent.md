# 115 新增转存起播许可实现方案

> 状态：代码与自动化验证完成，待受控客户端验收
> 负责人：Ember
> 更新时间：2026-09-22

## 背景与当前事实

- 用户确认只打开 Infuse 详情，2026-09-22 本机日志仍出现单集视频 GET、Gateway GET PlaybackInfo 补查、一次新转存和四次复用，没有 Playing/Progress/Stopped。正式播放的对照样本在视频 GET 前存在客户端 POST PlaybackInfo，携带 `IsPlayback=true`。
- Gateway proof 表示用户可访问媒体及其播放能力，不能证明用户点击播放；优化前意图字段只有诊断用途，DirectPlay 目标不存在便可进入新转存。本次保留普通 proof 资格，另加短期新增转存许可。
- 目标 Emby `4.9.3.0`，字段和时间语义沿用 [版本化播放代理合同](../../reference/emby-playback-proxy-contract.md)。客户端样本不代表其他版本或其他播放器必然提供同样信号。
- 用户已确认先前 Emby fallback 问题解决，本任务不重新排查或改写回退路径。

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

## 影响范围

- Gateway：请求旁路解析、proof 缓存、视频请求与成功停止事件。
- DirectPlay：内部请求合同、锁内新增转存准入、稳定错误类型及回归测试。
- Web / Bot / 对外 API / 部署 / 数据库 schema：无变更。
- 文档：系统架构、Emby 播放合同、115 端到端参考与本实施方案。

## 验证与进度

- [x] 日志及代码路径确认，明确已解决的 Emby fallback 不属于本次范围。
- [x] 确认变更边界与共享内部合同。
- [x] Gateway 先补失败测试，再实现严格意图解析、许可隔离和停止失效。
- [x] DirectPlay 先补失败测试，再实现无任务/无配额的拒绝与现有目标复用；该包 fake 与 race 已通过。
- [x] 覆盖重复字段、多 source、过期、时钟回拨、停止、同键 proof 替换、并发等待和 HEAD。
- [x] 全量 Go 非集成测试、相关 race、vet、build 与文档链接检查。
- [ ] 受控 Infuse 操作与日志对照；未启动服务或真实触发 Emby/115。

验证命令在 `services/api` 工作目录执行：

```bash
go test ./... -skip Integration -count=1
go test -race ./internal/playbackgateway ./internal/services/directplay ./internal/services/p115quota ./internal/services/p115account ./internal/services/embytoken ./internal/integrations/emby -skip Integration -count=1
go vet ./...
go build ./...
```

自动化使用 fake Emby / Provider / Redis，不启动服务，不请求真实外部系统。真实 Infuse 验收需后续受控执行：只打开未转存单集、点击播放、停止后再次浏览和再次播放；实际播放器行为、CDN 与生产性能不由单元测试代替。

2026-09-22 上述四条命令全部通过，集成测试显式排除；`git diff --check` 与现行文档相对链接检查通过。定向系统审查未发现需要修复的问题。没有变更 schema，也未执行 PostgreSQL 专项或真实 Redis/Emby/115/Infuse 验证。

## 落地后文档处理

稳定合同已同步到系统架构、Emby 播放合同与 115 端到端参考，代码及自动化清单已收口。本方案仅保留受控客户端验收项；真实客户端取证完成，或用户明确接受该未验证边界后，归档至 `docs/archive/plan/architecture/` 并同步计划、归档和盘点入口。
