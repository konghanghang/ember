# GitHub 开放问题修复计划

> 状态：实施中（已创建 goal；按独立语义切片验证并提交）
> 负责人：Ember
> 更新时间：2026-09-16
> 核对基线：本地 HEAD 与 GitHub 默认分支均为 `e678531dcef109d5bbb4b61967a8ca40e847eb22`
> 实施约束：基于现有结构做最小修复；本轮方案不新增 SQL migration、表、列、索引或持久化版本机制

## 背景

GitHub 当前有 11 个未关闭 issue：#8、#10–#19。#10–#19 来自 `bb1cbc6` 基线审计；当前代码已有后续修复，不能直接把开放数量当作待开发数量。

本轮读取了全部 issue 正文及评论、当前相关调用链、现有测试和版本化外部合同。除专项测试另有记录外，判断依据为静态代码；不把 issue 中的历史复现描述成本轮实测，不推断线上已发生事故。

## 目标与非目标

目标：

1. 优先保护已付款权益、续期累计和续期后的播放权限。
2. 消除登录故障循环、配置清空不生效、审批重试丢上下文和通知格式损坏。
3. 对已修复问题补齐验收证据，按独立语义单元交付。

非目标：重写支付系统、引入新的消息平台、改变永久用户与套餐业务规则、启动项目服务、真实调用 Emby/Stripe/Telegram/115、直接修复历史生产数据、自动提交或关闭 GitHub issue。

### 最小改动原则（2026-09-16 收紧）

- 先复用现有字段、状态、事务、失败记录和重试入口。此前提出的支付待处理新状态/页面、Policy revision 和 Bot 幂等新增字段均撤出本轮。
- “不新增 SQL”指不新增数据库迁移和 schema 变更；修复并发仍会调整查询、事务和锁 SQL，不能为避免改查询而牺牲正确性。
- 每项先用失败测试证明缺口，再验证最小修复满足原 issue 的验收条件；不附带任务系统重构、通用幂等框架或完整崩溃恢复升级。
- 如实施测试证明确有现有结构无法表达的必要状态，应说明具体失败场景再调整方案；不能预先为假设需求加字段，也不能为坚持零 migration 而漏掉必要功能。

## 当前事实与问题总表

优先级沿用 issue 原标注；#8 根据目前只有误分类、额外解码和日志噪声的证据按 P3 排期。P0 暂无证据。

| 优先级 | Issue | 当前判断 | 处理方向 |
| --- | --- | --- | --- |
| P1 | [#10 支付成功被本地过期状态忽略](https://github.com/konghanghang/ember/issues/10) | 代码仍有该分支；现有测试还把它当预期 | 支付事实驱动幂等履约，异常必须可追踪、可处理 |
| P1 | [#11 并发续期丢失天数](https://github.com/konghanghang/ember/issues/11) | 兑换与管理员续期仍先读后写绝对日期，无统一用户行锁；实际数据库交错待复现 | 所有累加入口先锁用户再计算 |
| P1 | [#12 旧 Policy 禁用已续期用户](https://github.com/konghanghang/ember/issues/12) | 统一入口仍无用户级跨副本串行；受控交错待复现 | 完整同步串行、取得资格后重读、复用现有失败重试 |
| P1 | [#13 profile 故障循环跳转](https://github.com/konghanghang/ember/issues/13) | 任意资料加载失败仍跳 login，login 又按 token 跳回控制台 | 区分认证失效和暂时故障，提供稳定重试状态 |
| P2 | [#14 内容锁耗尽共享连接池](https://github.com/konghanghang/ember/issues/14) | `9663ec8` 已有修复及 fake SQL 驱动测试 | 验收已有修复；真实 PostgreSQL 证据待补 |
| P2 | [#15 租约过期后返回直链](https://github.com/konghanghang/ember/issues/15) | `9663ec8` 已有心跳、最终确认和处理预算 | 验收已有修复，补精确竞争场景缺口 |
| P2 | [#16 清空群配置仍向旧群发送](https://github.com/konghanghang/ember/issues/16) | 空值仍回退当前缓存，问题分支存在 | 成功空值清除目的地；因涉及二次信息暴露，提前处理 |
| P2 | [#17 拒绝失败丢失待输入上下文](https://github.com/konghanghang/ember/issues/17) | pop 删除与拒绝是两次 API 调用 | 非破坏读取上下文，以现有记录 ID 与订阅终态实现重试 |
| P2 | [#18 HTML 通知截断](https://github.com/konghanghang/ember/issues/18) | formatter 仍对完整 HTML 做切片 | 在字段层预算长度，渲染后不切标签和实体 |
| P2 | [#19 本地管理员重置密码依赖 Emby](https://github.com/konghanghang/ember/issues/19) | ResetPassword 无条件调用 Emby，与自助改密分支不一致 | 管理员走本地密码合同 |
| P3（建议） | [#8 Latest 误判条目详情](https://github.com/konghanghang/ember/issues/8) | 六段路径匹配仍把静态段当 ItemId | 按固定版本静态路由表排除 Latest/Resume/Root |

证据入口：

- 支付：`services/api/internal/services/payment/service.go` 中 `successfulPaymentFulfillmentSkipReason`、`fulfillPayment`、`HandleWebhook` 周边；清理位于 `payment/cron.go`。
- 续期：`services/api/internal/services/redemption/service.go:redeemCodeWithDB`、`services/api/internal/services/user/admin.go:ExtendExpiry`；支付已在履约事务中锁用户行。
- Policy：`services/api/internal/services/policy/effective_policy.go:ApplyEffectiveUserPolicy` 与 `sync_worker.go`；过期、支付、兑换、人工操作均需纳入。
- Web：`services/web/src/router/index.ts` 导航守卫、`src/api/request.ts` 401 处理。
- Bot：`services/bot/app/runtime_settings.py`、`app/formatters/message_formatter.py`、`app/handlers/telegram_handler.py`；审批持久化位于 `services/api/internal/services/telegram/pending_reject.go`。
- Gateway：`services/api/internal/playbackgateway/item_snapshot.go:userItemDetailPath`；锁和租约位于 `services/api/internal/services/directplay/`、`p115quota/`。

## 实施顺序与交付切片

| 阶段 | 工作 | 依赖与完成标志 |
| --- | --- | --- |
| 0 | 核验 #14/#15，记录已有提交、覆盖范围与缺口 | 不重复实现；真实数据库未验收不得写成完成 |
| 1 | #11 → #10 → #12 | 先统一权益累加，再修付款履约，最后收口外部同步；作为同一轮权益链路验收 |
| 2 | #13、#16 | 与权益后端实现无直接依赖，可提前独立交付；#16 优先于一般 P2 |
| 3 | #17 → #18；#19 独立 | 审批状态与通知重试联测，密码重置单独验收 |
| 4 | #8；补齐 #14/#15 缺失的专项证据 | 日志误分类修复不阻塞 P1 |

每个 issue 原则上一个独立分支/PR；同一问题的代码、测试、文档一起交付，本轮不预设 migration。#11/#10/#12 可以依次叠加，但不得只合入付款分支而忽略续期竞争与 Policy 覆盖问题。

## 方案设计

### #11：统一续期事务边界

- 兑换、支付、管理员累加有效期，均在事务中先对同一用户执行 `SELECT ... FOR UPDATE`，取得最新到期日后再计算、更新。锁顺序要同时检查兑换码、订单、套餐锁，防止消除丢更新后引入死锁。
- 保持兑换记录、兑换次数和有效期在同一事务内提交；外部 Policy 同步放到事务提交后。
- 现有三个计算入口对空到期日都从当前时间起算；本次先用特征测试锁定空值、已过期、仍有效以及边界相等时的真实行为，不顺手改变永久账号语义。业务日期运算遵循 `CRON_TIMEZONE`，UTC 只作为存储/外部边界。
- 普通后台“指定到期日”属于绝对赋值，不擅自改成累加；应与累加入口按明确串行顺序生效。
- 默认无需 schema 变更。必须通过专用 PostgreSQL、同步屏障验证“两枚不同码”“兑换与支付”“兑换与人工续期”，同时断言最终天数、记录数和次数消耗。

### #10：把付款事实与本地支付窗口分开

- `expired` 表示本地订单窗口结束，不能证明没有收款；`updated_at` 也不能代表 Stripe 事件因果顺序。Stripe 官方明确不保证事件按生成顺序投递，且不应按 `created` 判重或判断先后，见 [Webhook 文档](https://docs.stripe.com/webhooks)。
- 对已验签、关联订单且满足付款合同的成功事件，以订单级锁和已履约状态保证一次发放；保留 webhook event ID 去重。`completed` 保持终态，晚到失败/过期事件不能撤销已发权益。
- 移除仅凭本地 `expired`、`failed` 或通用更新时间吞掉成功付款的规则；仍须校验订单/会话关联和既有金额、币种边界，不把不匹配事件当成正常付款。
- 当前套餐分组变化分支也会把已付款订单置为 expired 后返回成功，应在同一切片改为明确返回履约错误，不再静默 processed。复用 `stripe_webhook_events.status=failed`、`event_type`、`error_message` 和既有重投分发逻辑，记录固定原因及本地 paymentId，保留可定位、可重新处理的成功付款事件；不新增订单状态、错误列、管理页面或重试 API。
- `HandleWebhook` 已将业务错误持久化为 failed，并允许 failed/received 事件再次分发。该路径可承接短暂错误和人工处理后的重投；补运行手册说明按 eventId/paymentId 定位、排除分组等业务冲突、再通过 Stripe 既有重投入口处理。有限自动重试不能代替人工处理，持续失败必须明确保留；本轮不自动改用户分组、不自动退款。
- 测试覆盖“付款事件生成 → 本地清理 → 晚到回调 → 重复回调”、成功/失败/过期乱序、两个不同成功事件作用于同一订单、分组不匹配保留 failed 及恢复后重投。订单权益只发放一次；错误必须贯穿 service/handler 返回而非被外层吞掉。状态机单测之外必须验证事务内实际权益变化。
- 历史受影响订单另列只读对账任务；未获真实链路授权前不查 Stripe、不补发、不修改线上账务。新代码修复不能宣称历史权益已恢复。

### #12：用户级完整 Policy 同步串行

- 推荐先使用数据库支持的用户级跨副本互斥，在获得执行资格后重读用户、分组模板和媒体库偏好，串行覆盖“重读 → 获取远端 Policy → 写远端 → 本地成功记录”。进程内 mutex 或只给本地 UPDATE 加 CAS 都不足以修复远端旧写覆盖。
- 所有普通用户同步入口必须进入同一串行边界，包括 cron、支付/兑换、人工启停/续期、分组更新、worker 和失败重试；仍保留管理员不接管等现有规则。
- 使用 PostgreSQL session advisory lock，无需建表或加列；采用与现有锁不同的用途命名空间。加锁失败及时归还连接，本地排队状态由同一数据库依赖共享，不能每次 `NewService` 新建一个互不协调的容量控制器。同步 SQL 优先复用持锁连接，检查 token 撤销等嵌套操作，避免持锁后再从耗尽的共享池取连接。不持有续期业务事务等待网络；等待、执行和解锁有界，未知锁状态连接不能归还普通池继续复用。
- 保持现有支付/兑换提交后调用、worker 和人工重试结构。每次实际同步请求都排队执行并在取得锁后重读，不以“该用户已有正在同步的请求”为由直接丢弃本次同步；A 先持锁时 B 后执行，B 先持锁时 A 随后也读取 B 提交后的状态。
- 失败、取消和未确认的远端结果继续使用现有 `emby_policy_sync_tasks` 的 failed/last_error 与后台人工重试；成功收口与失败记录的顺序纳入检查，避免旧失败覆盖后来的成功。未知远端结果不得记为同步成功。
- 不新增 revision，不把所有入口改造成持久化队列，不在本轮承诺原系统尚无的“业务提交后任意时刻进程崩溃均自动恢复”。跨进程调用串行与现有失败可处理路径是本 issue 的修复边界。
- 固定版本用户 Policy 合同见 [Emby 用户与条目路由合同补充](../../reference/emby-user-policy-and-item-route-contract.md)。保持非托管字段，撤销 Gateway token 的前置安全语义也要参与交错测试。
- 用 fake Emby 屏障固定 A/B 两种获锁顺序，断言两次成功调用完成后远端 Policy 与本地缓存均反映续期后状态；覆盖两实例竞争、执行中再次续期、小连接池、远端成功本地失败、取消、解锁失败和现有失败任务重试。锁失败不能成为静默跳过同步。

### #13：认证失败与资料加载失败分开

- 401 继续统一清理身份并进入登录；网络/5xx 保留 token 和原目标路由，允许停留在现有登录页的会话恢复状态，不自动往返 login/dashboard。
- 在既有 LoginView 中增加一条失败说明和“重试”动作，不新建恢复路由或页面。守卫不能仅因 token 存在就把资料加载失败的会话踢回控制台；恢复状态以实际加载结果判断，不能把 query 标记当成鉴权依据。没有成功取得 profile 前不渲染受保护业务内容。
- 重试成功后重新校验服务端角色、强制改密状态并回到合法目标；失败维持故障状态，限制重复点击和并发请求，错误提示只出现一次。
- 前端实现必须遵守 Ember 风格，设计与交互基线以 [Web 设计规范](../../reference/web-design-guide.md) 为准；不引入新的设计系统或规范例外。失败说明需要可被辅助技术感知。
- 使用 Vue Router memory history + Pinia + fake API 做真实守卫集成测试，覆盖冷刷新、401、503、网络失败、恢复、角色不符、强制改密和后退；不能仅测路由元数据。

### #16：配置的空值必须具有清除语义

- 成功返回 `TELEGRAM_GROUP_CHAT_ID: ""` 时将 group 设置为 `None`，发送选择自然回退管理员；不重新启用已被配置合同禁用的环境变量回退。
- 区分字段缺失、读取失败、合法 ID、明确空值和非法值；失败/缺失沿用最近可信缓存的既有策略，明确空值覆盖旧缓存。
- 测试不能只断言 settings 对象，要覆盖排行榜发送目标：旧群 → 清空 → 管理员，并核对其余消费该配置的通知入口。
- 不改管理员 Chat ID 的不可清空校验。无需数据库变更。

### #17：保留上下文，复用订阅终态实现重试

- 现有 `bot_pending_reject_requests` 已有稳定 `id`、操作者、订阅 ID、消息上下文与 `expires_at`；`subscriptions` 已有 `status`、`reject_reason`、`reviewed_at`。这些字段足以表达本问题需要的定位、最终结果和清理边界，不新增 requestId、完成标记、结果表或索引。
- 增加非破坏读取上下文的 Internal 入口（peek），返回现有记录 ID；Bot 再按 `pendingRequestId`、`chatId`、`adminUserId`、`reason` 提交 complete。两次请求之间没有破坏性动作，网络失败时原上下文仍存在；提交期间用户又点击另一订阅也不会把已绑定 ID 的请求套到新记录上。
- complete 在事务中锁定并校验该上下文，再对订阅执行 PENDING 到 REJECTED 的条件更新。数据库失败回滚；只在本次真实状态转换成功提交后触发既有通知。不把 Web 拒绝入口的所有“已处理”错误全局改为成功，事务内审批操作只做必要提炼。
- 请求超时但服务端已拒绝时，重试读取订阅已保存的状态与原因，返回“已处理”及权威结果，不再次修改原因或触发通知；无法证明是同一管理员先前操作时，不宣称“你的请求已成功”。另一管理员已通过则返回对应状态，不能再拒绝。
- 成功后上下文暂不物理删除，复用现有五分钟有效窗口和定时清理。订阅终态使它不再是待审批操作，但保留记录让重发原因仍命中刚才那条记录，不误落到更早的待输入请求。peek 不得过滤终态订阅后向旧记录回退；超过窗口明确提示上下文过期，重新点击时仍先检查订阅状态，不产生重复副作用。保留现有“最新一次点击”的普通输入规则，不新增强制回复交互。
- 当前有正常审批消息回写路径，重试回写必须使用数据库里的最终原因/状态；消息编辑失败不回滚审批。本轮不改造 fire-and-forget 为可靠消息队列，不承诺通知恰好送达一次，但重复审批请求不能重复触发通知。
- 先部署支持 peek/complete 的 API，再升级 Bot；过渡期保留旧 pop 语义供旧 Bot 使用，新 Bot 全部切换且无旧版本回滚需求后删除旧调用/入口。混用旧 Bot 时不宣称旧 pop 流程已获得本次保证。
- 测试覆盖失败后重发、响应丢失后重试、不同原因重试不覆盖、多人审批竞争、同一管理员多条记录、已完成记录不回退到旧请求、五分钟过期及清理、Bot 重启和通知不重复触发。

### #18：通知格式化在字段层控制预算

- 不截断完整 HTML。名称、用户名、备注、拒绝原因等原始字段按预算裁剪后转义，标签、链接整体生成；最终审批状态优先保留，先缩短可选备注。
- 统一检查求片、自动通过、结果编辑等使用 `_clamp_telegram_text` 的 HTML 路径，不能只修一种通知；海报 caption 和文本降级分别使用对应预算。
- Telegram 长度限制按实体解析后的文本计算，不能把 HTML 原串长度当唯一断言；以官方 [Bot API](https://core.telegram.org/bots/api#sendphoto) 合同补 fixture，覆盖引号、`&<>`、emoji、临界长度、完整实体和最终审批状态。
- 保持 HTML 时必须结构合法；如选择纯文本降级，必须同步取消 parse_mode 并输出纯文本，不能把破损 HTML 原样再发一次。无需 schema 变更。

### #19：管理员重置密码遵循本地认证合同

- 与 `UpdatePassword` 对齐：管理员重置只更新本地 hash、清除强制改密标记；管理员绑定 Emby 不意味着该操作必须修改远端密码。
- 普通用户仍按现有远端同步顺序处理，缺少 EmbyID 返回明确错误；不扩展为所有账号都跳过 Emby。
- 测试覆盖无 Emby 配置且无 EmbyID 的管理员成功、管理员已绑定的行为、普通用户正常/远端失败、本地保存失败、旧 JWT 的 pwdSig 失效。无需 schema 变更。

### #8：静态路由不进入 ItemId 快照

- 固定 SDK 确认：`Latest` 返回数组，`Resume` 返回分页结果，`Root` 返回单个根条目；三者都是静态端点，不能把字面值作为 ItemId 快照键。
- 优先显式排除合同中的静态段，保持现有动态 ItemId 兼容性，不用“只能数字 ID”等无证据限制修问题。
- 增加路由分类和代理响应测试，覆盖 root/`/emby` 规范化、现有大小写规则、HEAD/GET、压缩列表、真正单条详情，断言透明响应不变且不会产生无效快照日志。
- 同步移除端到端参考文档中已解决的偏差说明；无需 schema 变更。

### #14 / #15：验收已有修复

- #14 已有本地按 key 排队、获取失败归还连接、锁持有者容量预留，以及未知锁状态丢弃连接。现有 `lock_pool_test.go` 使用 fake SQL driver，不能代替 PostgreSQL session advisory lock 的真实行为验证。
- 补专用 PostgreSQL 小连接池、多 locker/实例、同内容及不同内容竞争、取消/异常解锁测试；断言普通 SQL 可继续执行，连接最终归还。
- #15 已有 GET 预留续租、返回前原子确认、会话串行化、两分钟处理预算；HEAD 保持不创建/不续租，Playing 只推进已有租约。
- 现有 `lease_lifecycle_test.go` 覆盖失效拒绝与慢解析心跳；`lease_confirmation_test.go` 覆盖 memory/miniredis Lua 合同。补齐 issue 指定的“时钟推进超过 30 秒、B 占满额度、A 随后完成”组合场景，并保留重试/HEAD/Playing/取消回归。
- 当前合同只有账号准入上限，用户维度用于归因；不因 issue 旧措辞新增用户级总并发限额。
- 关闭时附已有修复提交、实际执行命令和证据等级；不宣称完成真实 Redis、115、播放器或生产负载验收。

## 影响范围与验证方式

- API：payment、redemption、user、policy、telegram/subscription、Gateway；保持现有持久化状态，必要的 Internal API 同步 DTO 和端点目录。
- Web：既有登录页的恢复状态与守卫；支付不新增页面、状态展示或重试按钮。保持列表 `data` 与字段 camelCase。
- Bot：运行期配置、拒绝原因客户端/handler、消息格式化。
- 数据库：本轮所有 issue 均按零新增 migration 设计，复用现有表/列/索引；不改历史 baseline，不依赖 AUTO_MIGRATE。锁和事务 SQL 属于业务修复，不是 schema 迁移。
- 配置/部署：不新增独立业务时区、不改变配置优先级；#17 需要 API 先于 Bot 升级。Policy 串行保证要求所有执行实例使用新代码，旧实例退出前不能宣称混合版本已满足互斥；不通过新增版本字段解决发布切换问题。

每个切片先补失败测试，再最小实现，再执行相应子系统检查：

| 工作目录 | 检查 |
| --- | --- |
| `services/api` | 定向 `go test`；交付前 `go test ./...`、`go vet ./...`、`go build ./...`；并发相关包补 `go test -race` |
| `services/web` | 守卫/组件/接口状态测试，`npm run test`、`npm run build` |
| `services/bot` | fake HTTP/Telegram 的测试，`python -m pytest tests`、`python -m py_compile main.py` |

数据库并发与事务验收使用 `EMBER_INTEGRATION_DATABASE_URL` 指定的专用集成库，由 harness 创建/清理隔离 schema；不连接共享开发库。环境缺失或用例跳过必须标为未验证。所有第三方业务调用都使用 fake/fixture，不启动项目服务。无 schema 变更时不新增无意义的迁移测试。

## 本轮完成项与剩余项

- [x] 读取 11 个开放 issue，确认本地与 GitHub 当前基线一致。
- [x] 对照当前代码，识别 #14/#15 已有修复及其测试层级。
- [x] 核对固定 Emby 4.9.3.0 SDK 的条目、Policy 和密码接口，补充参考合同。
- [x] 制定逐项方案、依赖顺序、状态/兼容边界和验收要求。
- [x] 按最小改动要求复核既有结构，撤出新增支付状态/页面、Policy revision 与 Bot 幂等字段，本轮改为零新增 migration 方案。
- [ ] 实施剩余 9 项代码修复。
- [ ] 补齐 #14/#15 尚缺的精确场景与数据库证据。
- [ ] 按切片同步文档、完成测试和独立 review。
- [ ] 获得相应授权后提交、推送/PR、更新或关闭 GitHub issue。

本轮最初执行三个播放相关包的整包测试时，环境中已设置的集成数据库变量触发了真实 PostgreSQL 用例，数据库连接连续超时后中止该轮。`p115quota` 与 `playbackgateway` 当轮通过，`directplay` 整包未通过；不能把该现象判断成内容锁死锁，也不能宣称数据库验收完成。

随后在 `services/api` 显式执行以下不依赖数据库的范围，三个包全部通过（2026-09-16）；跳过的集成用例仍是未验证项：

```bash
go test ./internal/services/directplay ./internal/services/p115quota ./internal/playbackgateway -skip Integration -count=1 -timeout=60s
```

本轮文档相对链接与空白检查通过；未修改业务代码。

## 落地后文档处理与归档条件

- 代码落实后同步 `docs/system-architecture.md`、`data-model-reference.md`、`api-endpoint-catalog.md`、`web-information-architecture.md`、`bot-architecture-reference.md` 和相关部署/测试 runbook。
- 通用 Web 规范不变，只复用现有规则；若实施确有通用规则变化，再同步 `web-design-guide.md`。
- 支付既有失败事件重投、Policy 串行与现有失败重试、拒绝终态重放合同提炼到现行参考文档；不让本计划永久承担系统合同。
- 全部 issue 有修复/撤回结论、验证证据和关联交付，兼容清理条件已满足或有明确后续跟踪后，收口状态并归档到 `docs/archive/plan/architecture/`，同步索引与直接引用。
