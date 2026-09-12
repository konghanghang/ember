# Ember 代码审计记录

## 审计基线

- 日期：2026-09-12。
- 状态：首轮深度审计第一阶段完成；核心高风险链路已形成结论，**不是全仓逐行覆盖或安全认证**。
- 仓库：`konghanghang/ember`。
- 基线：`master@bb1cbc6ae955221ab9b99531721ecb3c7fc331d9`。
- 审计分支：`docs/code-audit-baseline`；本次变更仅新增本文件。
- 执行边界：只审计，不修改业务代码；没有启动服务、接触共享数据库或调用真实 Emby、115、TMDB、MoviePilot、Stripe、Telegram。
- 提交流程：审计文档独立分支及 PR，不直接提交 master、不自动合并。问题修复须另行确认范围，再建立 Issue 与修复 PR。
- 所有位置均针对上述基线；后续提交可能改变行号。未运行、缺少依赖、被跳过的测试不能记为通过。

## 总体判断

本轮记录 4 项 P1、6 项 P2。优先处理支付履约、续期并发、策略同步顺序和登录故障恢复；它们直接影响用户权益或主要入口。未在已审查范围内确认 P0，不代表没有其他严重问题。

证据分为“原代码离线复现”“静态调用链确认”和“静态并发/时序风险”。后两类没有真实 PostgreSQL、Redis 或外部系统复现实验；不要将可推演交错描述为已经发生的生产事故。

| 编号 | 等级 | 功能与问题 | 验证方式 | 状态 |
|---|---|---|---|---|
| AUD-001 | P1 | 订单本地过期后，延迟的支付成功通知不再履约 | 静态调用链确认 | 待修复确认 |
| AUD-002 | P1 | 不同兑换码等续期入口并发，丢失有效期累加 | 静态并发路径确认 | 待回归测试及修复确认 |
| AUD-003 | P1 | 旧策略同步覆盖续期后的启用状态 | 静态并发路径确认 | 待回归测试及修复确认 |
| AUD-004 | P1 | 资料接口 5xx 导致登录与控制台往返跳转 | 原导航守卫离线复现 | 待修复确认 |
| AUD-005 | P2 | DirectPlay 锁等待占满同一数据库连接池 | 静态并发风险 | 待压测及修复确认 |
| AUD-006 | P2 | 慢直链解析耗尽预留租约，返回 302 前未重新准入 | 静态时序风险 | 待受控时钟测试及修复确认 |
| AUD-007 | P2 | 清空 Telegram 群配置仍保留旧群目的地 | 原模块离线复现 | 待修复确认 |
| AUD-008 | P2 | 拒绝审批失败后丢失上下文，提示重试却无效 | 原 handler 离线复现 | 待修复确认 |
| AUD-009 | P2 | HTML 通知整串截断破坏标签或实体 | 原 formatter 离线复现 | 待修复确认 |
| AUD-010 | P2 | 本地管理员密码重置被不必要的 Emby 依赖阻断 | 静态调用链确认 | 待修复确认 |

## 详细问题

### AUD-001 / P1：本地订单过期后吞掉支付成功回调

- 位置：`services/api/internal/services/payment/service.go` 的 `successfulPaymentFulfillmentSkipReason`（530 行起）、`fulfillPayment`（1243 行起）、webhook 处理（1057 行起）；`services/api/internal/services/payment/cron.go`（18 行起）。
- 触发：用户在本地截止时间前付款，成功 webhook 因故延迟；每 5 分钟执行的清理先将超过 30 分钟的 pending 订单标为 expired，随后支付成功事件才到达。
- 证据：成功事件遇到 expired 直接返回 nil，不进入用户续期；外层把无错误结果记录为 processed，后续相同事件不会再次履约。本地更新时间还参与旧事件判断，需一并审查。
- 影响：已付款订单仍过期，用户没有获得有效期，重试相同 webhook 不能恢复。结论不依赖任何 Stripe 默认会话有效期假设。
- 建议：区分本地付款窗口和实际支付结果。支付成功应进入幂等履约，或明确记录为待对账/退款异常，不能当作已处理成功静默忽略。业务条件不允许履约时也需有已付款异常处理闭环。
- 回归：付款事件产生 → 本地清理 → 延迟回调 → 重复回调；断言权益只增加一次，或订单留下可操作的已付款异常状态。现有 `payment/service_test.go` 把 expired 阻断成功作为预期，需修正生命周期测试，不能仅让原断言继续通过。

### AUD-002 / P1：并发续期丢失天数

- 位置：`services/api/internal/services/redemption/service.go` 的 `redeemCodeWithDB`（99 行起读取用户、115 行写入到期日）；`services/api/internal/services/user/admin.go` 的 `ExtendExpiry`（393 行起）。
- 触发：同一用户并发兑换两枚不同的有效码，或兑换与支付/管理员续期交错。
- 证据：兑换在事务内普通 SELECT 用户，然后根据旧值计算绝对日期并 UPDATE，没有先锁用户行。两个请求都读到 E，各写 E+30，两个兑换记录及次数可以均成功提交。唯一索引只保护同一 user/code，不能保护不同码。支付自己的用户行锁不能修复另一个入口已经读取的旧快照。
- 影响：消耗两次兑换或支付权益，但最终只增加一次天数；管理员延长有效期也有同类读改写窗口。
- 建议：所有累加有效期入口统一在事务内先锁用户行再计算，或使用正确处理空值、已过期及永久用户语义的数据库原子更新。不能只修支付入口。
- 回归：使用隔离 PostgreSQL 和同步屏障让两枚不同码都在更新前读到同一日期；验证最终累计天数和两条记录。另补兑换与支付、兑换与管理员续期的交错。未执行真实数据库并发测试。

### AUD-003 / P1：旧 Policy 同步重新禁用已续期用户

- 位置：`services/api/internal/services/policy/effective_policy.go`（56 行读取快照、115 行远端写入、119 行本地回写）；调用入口包括 `services/system/expiry.go` 的到期处理、兑换/支付提交后的异步同步、`policy/sync_worker.go`。
- 触发：到期任务 A 读取过期用户后阻塞在外部 I/O；续期 B 提交并先完成启用同步；A 随后继续以旧快照禁用用户。
- 证据：完整同步没有用户级串行化；远端写入和本地 `emby_disabled` 更新没有保证按最新用户状态执行。两次外部调用均成功时，不会产生失败重试；已续期用户也不再满足过期任务筛选条件。
- 影响：有效期在未来，播放账号仍被禁用，且不会因本次同步失败记录自动暴露。
- 建议：完整同步按用户串行，在取得执行资格后重读状态，并兼容跨副本；或建立版本化同步任务和过时结果纠正机制。只给本地 UPDATE 加 CAS 不足以纠正已写错的远端状态。
- 回归：mock Emby 客户端用屏障控制 A/B 的读写顺序；最终远端和本地都必须反映续期后的状态。此项针对本地并发顺序，不推断未核实的 Emby 版本协议。

### AUD-004 / P1：profile 故障造成往返重定向

- 位置：`services/web/src/router/index.ts`（270–272、281–286 行）；`services/web/src/api/request.ts` 的认证失败处理。
- 触发：浏览器保留 token，刷新后 profile 为空，资料请求返回 5xx 或网络错误。
- 证据：资料加载的任意异常均跳到 login；login 又因 token 存在跳回 console-dashboard。非 401 故障没有清除身份，因此再次请求资料并重复该决策。
- 离线复现：直接提取原 `router.beforeEach` callback，mock 已认证且 profile 为空的 store，让 fetchProfile 抛出 503。执行六次导航决策，得到六次控制台/login 交替跳转和三次 profile 请求，token 始终保留。
- 影响：暂时后端故障放大为重复请求和错误提示，无法停在可恢复页面。未运行真实 Vue Router；是否最终被路由器循环保护中止不能称为实测。
- 建议：401 才走认证失效；网络/5xx 中止导航并提供重试页面或空态。补守卫失败分支测试，现有路由测试只校验元数据。

### AUD-005 / P2：锁等待与业务操作竞争同一连接池

- 位置：`services/api/internal/services/directplay/lock.go`（43–71 行）；`directplay/service.go` 的构造（219 行起）、获取锁与 `resolveUnderLock`、`BeginAttempt`（692 行）；`directplay/store.go` 的事务入口；`services/api/internal/db/db.go:65`。
- 触发：高并发冷解析中，锁持有者尚未开始后续数据库事务，其他锁申请者已经占用剩余连接。
- 证据：每个 Acquire 先独占一个 sql.Conn，未拿到锁也一直保留连接并轮询；锁持有者进入 store 事务时，又向同一个最多 30 个连接的池申请连接。可形成“等待者持连接等锁，持锁者等空闲连接”的循环等待。同一播放会话的重复请求也能复用预留，无需假设 30 个不同会话都被准入。
- 影响：直链解析及共享池上的 API 操作可能停顿，直到请求取消等因素释放资源；PostgreSQL 无法仅靠数据库锁死锁检测发现应用连接池等待。
- 建议：未获锁时不长期占用连接，或设计独立且受限的锁连接资源，或让受锁操作复用持锁连接；同时设置整个解析阶段的有界截止时间。单个 provider 请求超时不能覆盖数据库池等待。
- 验证状态：静态并发风险，未经压测，不宣称已有生产死锁。回归应使用小连接池、可控屏障及相同内容并发请求，检查完成性、取消后的连接归还和其他查询可用性。

### AUD-006 / P2：预留租约先过期，直链仍被返回

- 位置：`services/api/internal/services/p115quota/contract.go:16`；`directplay/service.go`（422 行 Reserve、449 行 resolveWithAccounts、461 行返回）；`p115quota/redis_lease.go`（74–83 行及 Advance）；`services/api/internal/playbackgateway/video.go`（157 行调用、202 行返回 302）。
- 触发：请求 A 在 t0 取得 30 秒预留，源路径解析、锁等待或转存使成功返回晚于 t0+30 秒；期间未收到有效续租事件。
- 证据：成功路径在解析完成后没有使用当前时间重新检查/申请租约。过期或丢失的会话不能靠 Advance 重新建账；网关仍可对成功 candidate 返回 302。
- 影响：限额为 1 时，A 预留过期后 B 可被准入，A 随后也拿到直链；A 的后续 Playing 不能恢复已丢失的记录，形成并发统计漏记及超额窗口。
- 建议：长解析期间受控续租，或在返回前重新做原子准入；丢失租约时不可直接返回成功。续租/重建必须重新遵守用户与账号额度，不能无条件补账。
- 验证状态：静态时序风险，未运行 Redis/Go 测试。应以 fake clock 将源解析推进 31 秒，期间让 B 占满额度，再释放 A；断言 A 不会在无有效租约时返回成功，并覆盖 HEAD、重试及 Playing。

### AUD-007 / P2：成功清空群配置却继续使用旧群

- 位置：`services/bot/app/runtime_settings.py`（114–117 行）；对照 `services/api/internal/config/config.go`（1758–1760 行）、`services/api/internal/handlers/setting.go`（83–87 行）；消费者 `telegram_handler.py`（692–702 行）。
- 触发：Bot 曾缓存群 ID，管理员随后在设置中心将 TELEGRAM_GROUP_CHAT_ID 保存为空。
- 证据：后端允许空值，语义为回退管理员；读取接口明确返回空字符串，但 Bot 将空字符串视为应沿用旧缓存。排行榜仍选择旧 group_chat_id。
- 离线复现：原 RuntimeSettingsService，fake API 第一次返回群 -1002002、管理员 1001，第二次明确返回群空字符串；强制刷新后 group 仍为 -1002002。
- 影响：通知继续发向已移除的群，可能继续暴露播放排行；等待缓存 TTL 不能解决。
- 建议：区分读取失败/字段缺失与明确空值，后者必须清除群目的地并回退管理员。不要退回旧环境值；此设置已禁用环境回退。
- 范围：管理员 Chat ID 的设置校验不允许空值，本条不宣称可通过设置中心清空管理员权限。

### AUD-008 / P2：拒绝失败后无法按提示重试

- 位置：`services/bot/app/handlers/telegram_handler.py`（801–820 行）；`services/api/internal/services/telegram/pending_reject.go`（44–53 行）。
- 触发：管理员输入拒绝原因，pop 上下文成功后，reject 请求网络失败或后端报错。
- 证据：Go 的 pop 在事务内读取并删除记录；Bot 后续 reject 失败只提示重试，没有恢复上下文。第二次输入原因时记录已不存在，直接返回。
- 离线复现：调用原 handler 两次，fake pop 按后端删除合同返回 record、None，fake reject 失败；pop 调用两次、reject 仅一次，第二次静默忽略。
- 影响：短暂故障中断审批；重发原因无效，必须重新点击拒绝按钮。
- 建议：API 原子执行读取/拒绝/消费，或采用领取—确认机制；恢复上下文方案必须处理请求超时但服务端已经成功的情况，不能产生重复副作用。补失败重试与幂等测试。

### AUD-009 / P2：HTML 通知在标签中间被截断

- 位置：`services/bot/app/formatters/message_formatter.py`（44–46、51–85 行）；`telegram_handler.py`（380–400 行）。
- 触发：名称等合法输入经 HTML escape 后使完整 HTML 超过 caption 限额。
- 证据：先拼好 b/a 标签和实体，再按原始字符串切片到 1024。海报发送和文本降级复用同一字符串且都指定 HTML parse_mode，降级不能修复损坏的标签。
- 离线复现：原 formatter 输入 name 为 160 个双引号、普通 userName/tmdbId/note；结果长 1024，末尾落在未闭合的 a 标签中，结构解析失败。后端求片 name 仅要求非空，没有拒绝此输入。
- 影响：生成非法 HTML，管理员通知存在发送失败路径；未真实请求 Telegram，不把远端报错描述为实测。
- 建议：在拼装前按字段预算裁剪，保留完整标签、实体及最终审批状态；或超长时转为不使用 parse_mode 的纯文本。测试除长度外还需检查结构和关键内容。

### AUD-010 / P2：重置本地管理员密码被 Emby 阻断

- 位置：`services/api/internal/services/user/password.go`（29–39 行）；对照同文件 UpdatePassword（52–65 行）；`services/web/src/views/admin/UsersView.vue` 的重置动作。
- 触发：未绑定 Emby 的本地管理员，在尚未配置 Emby 时通过后台重置密码。
- 证据：后台展示该动作；ResetPassword 无条件先调用 Emby，ensureConfigured 失败后直接返回，尚未更新本地 hash。自助改密已有管理员本地分支，两个入口不一致。
- 影响：本应支持本地认证的管理员无法通过该后台操作重置密码。无需推断空 EmbyID 在真实远端的行为即可成立。
- 建议：管理员重置按本地认证分支执行；普通用户再按绑定情况处理外部同步。补“无 Emby 配置、无 EmbyID 的本地管理员”回归；现有 ResetPassword 成功测试仅覆盖带 EmbyID 用户。

## 验证记录

| 检查 | 结果 | 边界 |
|---|---|---|
| 既有 Bot formatter 测试 | 7 通过 | 单独进程、仓库既有外部依赖 stub |
| 既有 Bot handler 测试 | 22 通过 | 同上 |
| 既有 Bot search_cache 测试 | 3 通过 | 同上 |
| Bot api_client 测试 | 未运行成功 | 导入缺少 httpx，不是断言失败 |
| AUD-004 原导航守卫探针 | 成功复现故障决策 | Node vm + mock store，不是 Vue 集成测试 |
| AUD-007 / AUD-009 原模块探针 | 两个缺陷见证测试通过 | 证明缺陷存在，不代表业务功能正确 |
| AUD-008 原 handler 探针 | 成功复现失败重试丢失 | fake API，没有真实网络 |
| Go 全套及 PostgreSQL/Redis 集成测试 | 未运行 | 当前环境没有 Go 可执行文件 |
| Web 完整测试/构建 | 未运行 | 当前环境缺少项目依赖 |
| 基线既有 GitHub Test 工作流 | Go、Web job 成功，Bot job 跳过 | 读取历史状态，非本轮触发，不覆盖新增缺陷场景 |

Bot 已执行命令（在 services/bot 中，分别运行）：

```sh
python -B -m unittest discover -s tests -p test_message_formatter.py -v
python -B -m unittest discover -s tests -p test_telegram_handler.py
python -B -m unittest discover -s tests -p test_search_cache.py
```

[基线既有 CI 运行记录](https://github.com/konghanghang/ember/actions/runs/34697921044)。通过已有测试不能排除尚无断言覆盖的生命周期、并发和故障分支。本次离线探针未加入业务仓库；上述每项记录输入、调用方式及结果，修复时应转成正式回归测试。

## 已检查范围与剩余范围

| 范围 | 本轮追踪内容 | 尚未完成 |
|---|---|---|
| API 用户与权益 | 登录/注册关键路径、JWT 回查、后台用户操作、密码、兑换、支付履约、过期处理、Policy 同步 | 全部套餐组 CRUD、每种用户状态组合 |
| 播放与配额 | DirectPlay 路由/锁/转存状态机、账号路由及健康分类、Redis 会话准入/事件推进、网关 302 出口 | 全量 Emby 协议合同逐项核对、115 provider 全实现及真实故障测试 |
| Web | 请求层、认证/profile store、导航守卫，续费/日历及后台重置相关交互 | 所有页面与 API DTO 逐字段核对、真实浏览器集成 |
| Bot | Internal API client、运行设置、通知/格式化、拒绝审批、绑定/改密入口、生命周期、缓存 | Telegram 绑定完整并发流、真实平台交互 |
| 数据库与部署 | 迁移入口/窗口文档、users/payments/redemptions 相关约束、测试工作流、Compose 入口 | 全量 schema 对照、空库回灌/升级演练、部署实测 |
| 其他业务 | 订阅审批通知的相关持久化/补偿对照 | 订阅完整生命周期、媒体缺失扫描、排行榜全链路、日历同步全链路、清理恢复任务 |

已知 [Issue #8](https://github.com/konghanghang/ember/issues/8)（Gateway 将 Items/Latest 误判为详情及无效诊断日志）单独保留，不重复计入这 10 项新发现，也不在本轮擅自关闭。

## 后续检查点

- [x] 固定 baseline，读取 AGENTS.md、系统架构、测试与 Git 协作约定。
- [x] 第一阶段核心高风险链路审查及交叉复核。
- [x] 可运行的 Bot 测试与 4 项 Web/Bot 离线缺陷复现。
- [x] 记录结论、证据等级、最小修改建议及测试缺口。
- [ ] 用户确认修复范围后，按 AUD 编号建立 Issue，填写 Issue/PR/修复 commit。
- [ ] 优先修复 AUD-001/002/003/004；每类问题独立或紧密相关的小 PR，不自动合并。
- [ ] 为 AUD-005/006 做隔离数据库/Redis 压测与可控时间验证，再确定最终修复。
- [ ] 续扫订阅、媒体缺失扫描、排行榜、日历及清理恢复；进一步完成网关外部协议合同审查。
- [ ] 完成可重复的 Go/Web/Bot 全套测试与空库迁移验证。

续扫时先读取本文件及当前目标 commit，比较基线后的代码差异；已变更模块重新确认，不把旧问题直接套在新代码上。更新覆盖表、问题状态和相关 Issue/PR，保留未完成项目，不依赖聊天上下文作为唯一记录。
