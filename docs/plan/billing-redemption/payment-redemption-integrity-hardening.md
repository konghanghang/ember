# 支付与兑换码完整性加固方案

> 状态：主干完成，保留尾项（P0 + P1 已落地）
> 负责人：Ember
> 更新时间：2026-04-28

## 落地进度

批次 2 + 后续 review 修复已完成本方案绝大部分 P0 / P1 主干项：

- ✅ pending payment partial unique、事务内占位 + Stripe `Idempotency-Key` 已落地
- ✅ Stripe webhook `event.id` 去重、`checkout.session.expired` 收口和 pending 过期 cron 已落地
- ✅ 支付 / 兑换的 Emby 调权已移出事务；失败统一落到 `failed_emby_async_ops` 补偿队列，而不是原计划里的独立 `failed_emby_unbans` 表
- ✅ 模板用户 Policy 白名单收口、`expirePendingPayments*` 命名区分、兑换码状态语义复用已落地
- ✅ 多币种口径、支付索引去重与架构文档同步已落地

当前剩余项主要是模型 / 文档治理尾项：

- `PlanGroup` 展示态字段仍挂在 `models.PlanGroup` 的 `gorm:"-"` 字段上，尚未彻底拆成独立 DTO
- 计划正文里 `failed_emby_unbans` 的设计已被统一补偿队列替代，后文仍需继续清理旧表述

## 归档判断

- 当前不适合归档。
- 原因：`PlanGroup` DTO 还没拆干净，且方案正文仍有旧补偿表设计，先继续保留在 `docs/plan/` 更安全。

## 背景

2026-04-25 系统性 review 在支付 / 套餐分组 / 兑换码三个子系统暴露多类资金线红线问题，整体品味评分 🟡：

- `payments` 表缺 `(userId, planId) WHERE status='pending'` 部分唯一索引；`shouldReusePendingPayment` 与 `Create` 之间无锁，并发结账可造出多条 pending 与多个 Stripe Checkout Session，存在重复扣费风险。
- `fulfillPayment` 与 `RedeemCode` 在 DB 事务里同步调 Emby 解封：Emby 慢响应会持有 user / payment / plan 三行 `FOR UPDATE` 锁；事务回滚时 Emby 已解封不可逆；事务 commit 失败时 Emby 已解封但本地无写入。
- Stripe webhook 未做事件级幂等去重（无 `event.id` 表）；`checkout.session.expired` 未处理，本地 30 分钟 TTL 与 Stripe 24h 寿命不一致。
- `fulfillPayment` 中 `Plan.Select("id","name","planGroup")` 在 PG 双引号列名场景下存在折叠为 `plangroup` 报错的可能（待运行期验证），一旦命中所有 webhook 履约链路全挂。
- 兑换码模板用户 Emby Policy 复制白名单含 `MaxParentalRating` 等敏感字段，模板用户被错误授权时全量复制。
- `PlanGroup` 模型把展示态 `PlanCount / UserCount / FollowingUserCount` 暴露在 JSON tag，依赖手动 `gorm:"-"` 兜底，新人改动易写穿。
- `expirePendingPaymentsByScope` 与 `ExpirePendingPaymentsForUsersFollowingDefault` 命名相近，一个全跳一个全杀，调用错就清空全表。
- `RedemptionCode.IsValid` 与 `applyRedemptionCodeStatusFilter` 维护两份语义，存在漂移风险。
- baseline `payments` 表对 `stripeSessionId` 重复建了两个 unique index（`idx_payments_stripe_session` + `idx_payments_stripe_session_id`）。
- 多币种允许 `usd / hkd / cny` 混合存在，但 `payments.currency` 默认 `usd`，跨分组结算时缺乏汇率口径声明。

如果不收口，会出现"用户连点结账被扣两笔"、"webhook 重试导致重复履约"、"模板用户解锁未成年用户 R 级内容"、"PG 双引号列名导致全量履约失败"等真实可触发的资金 / 权限事故。

## 目标

本方案要实现：

1. 消除支付链路的并发与幂等漏洞，让"用户连点结账"不产生重复 Stripe Session 与重复扣费
2. 把 Emby 调权外部 IO 移出 DB 事务边界，保证事务可回滚 / 外部副作用可补偿
3. Stripe webhook 引入 `event.id` 级别幂等去重，并补 `checkout.session.expired` 处理
4. 验证并修复 `fulfillPayment` 中 `Plan.Select` 列名拼写在 PG 下的真实行为
5. 兑换码模板用户 Policy 复制白名单显式收口"内容范围 / 转码限制"等纯播放参数，禁止复制管理员 / 家长分级位
6. `PlanGroup` 持久化模型与展示 DTO 拆分，模型只承载 schema 字段
7. 命名相近且语义相反的 `expirePendingPayments*` 函数显式区分 / 加注释
8. `RedemptionCode.IsValid` 与状态过滤共用同一份语义函数
9. 清理 baseline 中 `payments` 重复索引
10. 多币种结算口径写入 `docs/system-architecture.md` 并在前端展示统一币种

## 非目标

本次明确不做：

- 不引入退款 / chargeback 处理流程（后续独立计划）
- 不调整 Stripe 支付方式（仍是一次性 Checkout，`stripe_allowed_payment_methods` 配置不动）
- 不重写套餐分组管理 UI（前端改放 `console-admin/web-frontend-auth-and-design-baseline-fix.md`）
- 不变更 `redemptions` 模型字段
- 不引入第三方汇率服务，仅声明结算币种口径

## 当前事实

以当前代码和现行文档为准，现状如下：

- 相关文档：`docs/system-architecture.md` §4.2 / §4.3 / §4.7 / §4.7.1 / §4.8 / §5.3 / §5.4 / §5.15
- 相关服务：
  - `services/api/internal/services/payment/service.go`
  - `services/api/internal/services/payment/plan_groups.go`
  - `services/api/internal/services/redemption/service.go`
  - `services/api/internal/services/redemption/code_service.go`
- 相关 handler：
  - `services/api/internal/handlers/payment.go`
  - `services/api/internal/handlers/redemption_code.go`
- 模型：
  - `services/api/internal/models/plan.go`、`plan_group.go`、`payment.go`、`redemption_code.go`、`redemption.go`
- 涉及表：`plans`、`plan_groups`、`payments`、`redemption_codes`、`redemptions`
- Stripe 集成：
  - `verifyStripeSignature` 已做时间戳 ±300s 校验
  - webhook 入口未持久化 `event.id`
  - `CreateCheckoutSession` 不携带 `Idempotency-Key`
- 当前行为：
  - `findReusablePendingPayment` + `Create` 之间无事务、无唯一索引
  - `fulfillPayment` 在事务内调 `embyService.SetUserPolicy`
  - `RedeemCode` 在事务内调 `embyService.SetUserPolicy`
  - webhook switch `default: return nil`，不处理 `checkout.session.expired`
  - 模板用户 Policy 复制白名单覆盖 `EnabledFolders / EnableContentDownloading / EnableSyncTranscoding / MaxParentalRating` 等
- 现有限制：
  - 线上 `AUTO_MIGRATE=false`，新增索引 / 表必须配套 SQL migration
  - `PlanGroup` 默认分组全局唯一约束在应用层维护
  - 套餐归属与用户有效分组的对齐发生在 `CreateCheckoutSession` 与 `fulfillPayment` 两处

## 方案设计

### 1. 用户可见行为

- 用户在续费中心连点结账，Stripe Dashboard 上至多产生一条 Checkout Session；30 分钟内复用，过期后自动失效
- 用户支付成功后，权益（有效期延长 + Emby 解封）必然到账；若 Emby 解封暂时失败，用户登录态正确、控制台展示"权益已开通，Emby 同步中"，运营侧能从待补偿队列看到具体记录
- 兑换码续期被 ban 用户时，本地权益与 Emby 解封同步生效；事务失败时不会出现"权益没生效但兑换码次数已扣"的错乱
- 管理员下架套餐后，已开始的 Checkout Session 仍能完成履约，但新订单不会再出现该套餐
- 套餐分组切换默认时，跟随默认分组的用户 pending 支付被显式收口
- 设置新模板用户后，新注册用户的 Emby Policy 不会被授予管理员 / 家长分级位
- 多币种套餐在控制台展示时显式标注币种，不做隐式合计

### 2. 数据与模型

#### 新增 / 修改索引（必须配套 SQL migration）

| 表 | 索引 | 类型 | 用途 |
|---|---|---|---|
| `payments` | `(userId, planId) WHERE status='pending'` | partial unique | 防并发创建多条 pending |
| `payments` | 删除 `idx_payments_stripe_session`，保留 `idx_payments_stripe_session_id` | unique | 去重 |

#### 新增表 `stripe_webhook_events`

| 字段 | 类型 | 说明 |
|---|---|---|
| eventId | string(64) | Stripe `event.id`，主键 |
| eventType | string(64) | 事件类型 |
| livemode | bool | 区分 live / test |
| receivedAt | time.Time | 收到时间 |
| processedAt | *time.Time | 处理完成时间，可空 |
| status | string(20) | `received` / `processed` / `skipped` / `failed` |
| errorMessage | *string(500) | 失败原因 |

#### 新增表 `failed_emby_unbans`（用于支付 / 兑换 commit 后 Emby 调权失败的补偿）

| 字段 | 类型 | 说明 |
|---|---|---|
| id | string(25) | CUID |
| userId | string(25) | 待解封的用户 |
| origin | string(20) | `payment` / `redemption` |
| originRefId | string(25) | 关联记录 ID |
| retries | int | 重试次数 |
| nextAttemptAt | time.Time | 下次重试时间 |
| createdAt | time.Time | 自动 |

#### 模型拆分

- `models/plan_group.go` 只保留持久化字段；`PlanCount / UserCount / FollowingUserCount` 拆到 `services/payment/types.go` 的 `PlanGroupView`
- `models/plan.go` 同步 sweep（review 已确认 `PlanView` 已正确拆，本次复核）

#### Migration 命名

- `YYYYMMDD_NN_payments_pending_unique.sql`
- `YYYYMMDD_NN_payments_dedupe_indexes.sql`
- `YYYYMMDD_NN_stripe_webhook_events.sql`
- `YYYYMMDD_NN_failed_emby_unbans.sql`

所有 migration 必须幂等。

### 3. 接口与边界

- `POST /api/v1/payments/checkout`
  - 入参不变
  - 内部行为：开事务 → `INSERT INTO payments ... ON CONFLICT (userId, planId) WHERE status='pending' DO NOTHING` → 冲突时回查复用现有 pending → 否则调 Stripe `CreateSession` 并写回 `stripeSessionId`
  - Stripe `CreateSession` 调用必须携带 `Idempotency-Key = "checkout:" + paymentId`
- `POST /api/v1/webhooks/stripe`
  - 接收事件后第一步：`INSERT INTO stripe_webhook_events ... ON CONFLICT (eventId) DO NOTHING`，冲突直接 200
  - switch 新增 `case "checkout.session.expired"`：本地对应 pending 收口为 `expired`
  - `markPaymentFailed` 与 `fulfillPayment` 引入 `event.created` 时间戳保护，仅当事件时间晚于 `payment.updatedAt` 才覆盖
- `POST /api/v1/internal/redeem` / `POST /api/v1/redeem`
  - 入参 / 出参不变
  - 业务流程见 §4.2
- 兑换码模板用户复制：保留接口，但白名单收紧（详见 §4.3）
- 套餐分组管理接口不变；后端实现里把 `expirePendingPayments*` 重命名

### 4. 关键流程

#### 4.1 支付链路

1. handler 接收 `CreateCheckoutSession` → service 开事务
2. 在事务里：
   - 锁 user + plan 行
   - 强校验"用户有效分组 = 套餐当前分组"
   - `INSERT payments (status='pending', expiresAt=now()+30min) ON CONFLICT DO NOTHING`
   - 冲突时 `SELECT` 现有 pending（含 `checkoutUrl`）
3. 事务 commit 后：
   - 若是新建 pending：调 Stripe 创建 Session（带 Idempotency-Key），把 `stripeSessionId / checkoutUrl` 回写
   - 若是复用 pending：直接返回 `checkoutUrl`
4. webhook 入口：
   - 第一步去重写 `stripe_webhook_events`
   - 第二步事务内更新 `payment` 状态（仅修字段、不调 Emby）
   - commit 后异步触发 Emby 解封；失败写 `failed_emby_unbans`
5. 火忘式通知 Bot 仍然发，但包 `safeFireAndForget`（见 `bot-telegram` 计划）

#### 4.2 兑换链路

1. handler 接收 `RedeemCode` → service 开事务
2. 在事务里：
   - `SELECT redemption_codes ... FOR UPDATE`，校验 `IsValid()`
   - 检查 `redemptions(userId, code)` 是否已存在
   - 计算新 `expiresAt`，写 `users.expiresAt`（仅这一列，不动 `embyDisabled`）
   - 写 `redemptions` 记录
   - 原子递增 `usedCount`（带 `WHERE usedCount < maxUses AND (expiresAt IS NULL OR expiresAt > now)`）
3. 事务 commit
4. commit 后异步：若用户原 `embyDisabled=true`，调 Emby 解封；失败写 `failed_emby_unbans`，下一次登录或 cron 触发时重试

#### 4.3 模板用户 Policy 白名单收口

- 允许复制：`EnabledFolders / EnableSyncTranscoding / EnableMediaPlayback / EnableAudioPlaybackTranscoding / EnableVideoPlaybackTranscoding / SimultaneousStreamLimit / MaxActiveSessions`
- 显式禁止复制：`IsAdministrator / EnableUserPreferenceAccess / EnableContentDeletion / EnableContentDownloading / EnableLiveTvManagement / EnableLiveTvAccess / EnableMediaConversion / EnableSubtitleManagement / MaxParentalRating / BlockedTags / AllowedTags`
- `validateTemplateUserID` 增加校验：模板用户 EmbyPolicy 中 `IsAdministrator=true` 时拒绝设为模板

#### 4.4 命名收口

- `expirePendingPaymentsByScope(userID, planID)`：当两个参数都为空时，直接 `panic("expirePendingPaymentsByScope requires userId or planId")`，禁止当作"全表"使用
- `ExpirePendingPaymentsForUsersFollowingDefault` → `ExpireAllPendingPaymentsForFollowingDefaultUsers`，文档明确说明"全表 EXISTS 子查询"

#### 4.5 webhook 幂等流程

1. 收到 webhook → `verifyStripeSignature`（保留）
2. 解析 `event.id / event.type / event.livemode / event.created`
3. `INSERT stripe_webhook_events ON CONFLICT (eventId) DO NOTHING`
4. `RowsAffected = 0` → 直接返回 200（已处理过）
5. 否则按事件类型分发：
   - `checkout.session.completed`（payment_status=paid） → fulfillPayment
   - `checkout.session.expired` → 收口本地 pending 为 `expired`
   - `checkout.session.async_payment_succeeded` → fulfillPayment
   - `checkout.session.async_payment_failed` → markPaymentFailed
   - 其余：写 `status=skipped`
6. 处理完成后更新 `processedAt`、`status`

### 5. 失败路径与边界条件

- **Stripe webhook 重试同一 event.id**：第二次 INSERT 冲突直接 200，不重复履约
- **Stripe webhook 重试不同 event.id 但相同 paymentIntent**：fulfillPayment 在事务里二次校验 `payment.status`，已 completed 直接 noop
- **Stripe Checkout Session 超 24h 后用户付款**：Stripe 拒收，不会触发 paid webhook；本地 pending 因 30 分钟 TTL 已 expired，业务无需处理
- **同 userID + planID 并发结账**：partial unique 拒绝第二条 INSERT，回查复用，不会产生第二条 Stripe Session
- **fulfillPayment commit 后 Emby 解封失败**：写 `failed_emby_unbans`，cron 重试，超 6 次记告警
- **RedeemCode 事务 commit 成功但用户处于 `embyDisabled=true`**：Emby 解封异步重试；用户控制台显式标注"Emby 同步中"
- **模板用户 EmbyPolicy 含 `IsAdministrator=true`**：`validateTemplateUserID` 拒绝
- **PlanGroup 切换默认且并发支付**：保持现有"切换时收口跟随默认 pending"的语义，但加 advisory lock 防止切换与新建 pending 并发
- **Plan 软删除后 webhook 到达**：fulfillPayment 不校验 `plan.IsActive`，仍履约（business contract）；DeletePlan 同步收口同 plan 的 pending
- **多币种**：保持 plan 自带 `currency`，结算与展示按各自币种；管理后台不做"汇总金额"展示

## 影响范围

- API：
  - 修改：`payment/service.go`、`payment/plan_groups.go`、`redemption/service.go`、`redemption/code_service.go`、`auth/service.go`（模板 Policy 白名单）、`handlers/payment.go`、`handlers/redemption_code.go`
  - 新增：`services/payment/webhook_dedupe.go`、`services/account/emby_unban_compensation.go`（与 access-auth 计划共用 cron 入口时合并）
- Web：
  - 续费中心 / 支付中心展示需要消费 `payment.status='expired'` 与"Emby 同步中"语义（前端改放 `console-admin/web-frontend-auth-and-design-baseline-fix.md`）
- Bot：
  - 不变 Bot 端代码；fulfillPayment 通知通道改用 `safeFireAndForget`（在 bot-telegram 计划中实现）
- 配置 / 部署：
  - 新增 4 份 SQL migration
  - cron 调度新增"Emby 解封补偿"
- 文档：
  - `docs/system-architecture.md` §5.15 改写支付流程，明确"事务内不调 Emby"约束
  - `docs/system-architecture.md` §5.4 改写兑换流程
  - 新增"多币种结算口径"段落
  - `docs/runbooks/deployment-environment.md` 新增 `stripe_webhook_events` / `failed_emby_unbans` 表说明

## 验证方式

### 编译 / 测试

- `cd services/api && go build ./...`
- `cd services/api && go test ./internal/services/payment/... ./internal/services/redemption/...`
- `cd services/api && go test ./internal/handlers/... -run "TestPayment|TestRedemption"`

### 手工验证

#### 支付幂等
- 用户在两个标签页同时点结账：Stripe Dashboard 看到只有 1 条 Session，本地 `payments` 只有 1 条 pending
- 复用 30 分钟内的 pending Session：两次结账返回同一 `checkoutUrl`
- Stripe Dashboard 手动 Resend webhook 同一 event：第二次返回 200 但不重复履约
- 模拟 Stripe `checkout.session.expired`：本地 pending 收口为 expired

#### 事务边界
- 关闭 Emby → 正常完成支付：用户 `expiresAt` 已延长，`failed_emby_unbans` 记录新建；恢复 Emby + cron 触发 → 用户 `embyDisabled=false`
- 同样场景对兑换码续期重复一次

#### Plan.Select 列名
- 触发任意 webhook 履约 → 观察 SQL 日志，确认 `Plan.Select` 不报"column does not exist"
- 若运行期确实报错，立刻按 P0 修复（Select 改全字段或显式加引号）

#### 模板用户白名单
- 准备一个含 `MaxParentalRating=10` / `EnableContentDownloading=true` 的模板用户
- 通过兑换码注册新用户 → 检查新用户 Emby Policy：上述字段不被复制
- 把模板用户设为 `IsAdministrator=true` → 后台保存模板时被拒

#### PlanGroup 模型
- `db.Save(&group)` 只写持久化字段，无 PG 报错
- 后台列表与详情仍能看到 `PlanCount / UserCount`

#### 命名收口
- 直接调用 `expirePendingPaymentsByScope("","",nil)` 在测试里 panic
- `ExpireAllPendingPaymentsForFollowingDefaultUsers` 文档说明"全表"

#### baseline 索引
- 新装环境只剩一份 `stripeSessionId` unique index
- `\d+ payments` 输出无重复

#### 多币种
- 创建 `usd` 与 `cny` 两个 plan，分别完成支付 → 各自记录 currency 一致；后台无"汇总金额"误导

### 修复后验证清单

- [ ] `go build ./...` 与 `go test ./internal/services/payment/...` 全绿
- [ ] 4 份 SQL migration 在临时库重灌通过
- [ ] `stripe_webhook_events` 在测试环境记录至少 5 条不同事件（含一次重放）
- [ ] `failed_emby_unbans` cron 在测试环境跑一轮空表无报错
- [ ] 关键日志含 `paymentId / userId / planId / stripeSessionId / eventId`，且 Stripe 错误日志不泄漏 secret
- [ ] 模板用户白名单写入 `docs/system-architecture.md` §5.4
- [ ] 多币种结算口径写入文档

### 二次暴露检查清单

- [ ] sweep 所有"事务内调外部 IO"位置：`subscription` 审批通过路径（`subscription/service.go`）、`media_gap dispatch`、`tv_calendar` 同步路径
- [ ] sweep 所有 GORM `Save(&xxx)` 全字段写入位置（在 `subscription`、`tv_calendar`、`device` 中可能也存在并发覆盖）
- [ ] sweep 所有 `Plan.Select` / `User.Select` / `Subscription.Select` 字符串拼接列名，确认 PG 双引号语义无折叠风险
- [ ] sweep 所有"火忘 goroutine"，与 `bot-telegram` 计划的 `safeFireAndForget` 收口对齐
- [ ] sweep 所有"GORM 模型 + 展示态字段"，确认是否需要拆 DTO（`Plan`、`PlanGroup`、`User`、`Subscription`）
- [ ] 复核 `RedemptionCode.IsValid` 与 `applyRedemptionCodeStatusFilter` 是否共享同一份语义函数

## 落地后文档处理

- 落地后把"支付幂等契约"、"事务外补偿队列"、"Stripe webhook 去重"、"模板用户白名单"提炼到 `docs/system-architecture.md` §5
- 多币种结算口径写入 `docs/system-architecture.md` 与前端展示规范
- 本方案在 P0+P1 全部完成、回归测试通过后移入 `docs/archive/plan/billing-redemption/`
- P2 / P3 中未顺手收口的项纳入下一轮治理

## 附录：问题清单与本方案条目映射

| review 编号 | 问题 | 本方案条目 |
|---|---|---|
| P0-1 | fulfillPayment 事务内调 Emby | §4.1 + 表 `failed_emby_unbans` |
| P0-2 | 待支付订单复用并发窗口 | §2 partial unique + §4.1 |
| P0-3 | RedeemCode 事务内调 Emby | §4.2 |
| P0-4 | Stripe webhook 无事件去重 | §2 表 `stripe_webhook_events` + §4.5 |
| P1-1 | 未处理 `checkout.session.expired` | §4.5 |
| P1-2 | `Plan.Select` 列名拼写 | §4 / 验证清单 |
| P1-3 | Plan 软删除履约语义 | §5 / DeletePlan 收口 |
| P1-4 | markPaymentFailed 与 fulfill 时间戳 | §4.5 |
| P1-5 | 模板 Policy 白名单 | §4.3 |
| P1-6 | PlanGroup 展示态污染模型 | §2 模型拆分 |
| P1-7 | `expirePendingPayments*` 命名相近 | §4.4 |
| P2-1 | Stripe Idempotency-Key 缺失 | §4.1 |
| P2-2 | CreateCheckoutSession 不锁行 | §4.1 |
| P2-3 | `IsValid` 与 SQL 时基不一致 | §2 + 二次暴露 |
| P2-5 | webhook 日志缺 event.id | §4.5 |
| P2-7 | 多币种口径 | §5 + 文档 |
| P3-4 | baseline 重复索引 | §2 |
| P3-2 | `applyRedemptionCodeStatusFilter` vs `IsValid` 双份语义 | 二次暴露清单 |

## 批次 2 已落地（2026-04-27）

按"批次 2 收口实施计划"完成：

- ✅ `failed_emby_async_ops` 表 + `services/account/emby_compensation.go` + cron `emby-async-compensation @every 10m`
- ✅ `stripe_webhook_events` 表 + HandleWebhook event.id 级别去重
- ✅ `payments` 同 (userId, planId) status='pending' partial unique（`uq_payments_pending_user_plan`）+ 历史脏数据预检（migration 内置 `RAISE EXCEPTION`）
- ✅ `payments.stripeSessionId` 改为 partial unique 排除空串，并清理 baseline 重复索引
- ✅ `CreateCheckoutSession` 改为"事务内 ON CONFLICT 占位 → 事务外调 Stripe（带 Idempotency-Key=checkout:paymentId）→ 单事务回填"模式
- ✅ `fulfillPayment` / `RedeemCode`：Emby 调权移到 commit 后异步执行（`async.SafeGo` + `EmbyCompensation.EnsureUnbanned`），失败入补偿队列
- ✅ 新增 `checkout.session.expired` webhook 处理（`MarkPaymentExpired`）
- ✅ `markPaymentFailed` / `fulfillPayment` 引入 `event.created < payment.updatedAt` 的乱序保护
- ✅ 模板用户 Policy 复制白名单收紧：移除 `EnableContentDownloading` / `MaxParentalRating`
- ✅ `expirePendingPaymentsByScope` 入口在双空时 panic（防止全表收口误用）
- ✅ `ExpirePendingPaymentsForUsersFollowingDefault` → `ExpireAllPendingPaymentsForFollowingDefaultUsers` 重命名
- ✅ `handlers/payment.go` + `handlers/redemption_code.go` 全部 `c.JSON(StatusInternalServerError, gin.H{"error": err.Error()})` 改为 `httpx.InternalError`

不在本批：

- ConfigService 敏感回显推到批次 5
- PlanGroup DTO 拆分（已确认 `gorm:"-"` 标记到位，不重做）
- `Plan.Select` 列名修复（GORM 自动加引号、未实证报错，仅在验证清单跑 SQL 日志确认）
- 多币种结算文档独立成稿

### 批次 2 review 修复（2026-04-27）

- ✅ **Stripe webhook 失败重试状态机**：`HandleWebhook` 命中冲突时回查 status；只有 `processed / skipped` 直接 200，`received / failed` 必须允许 Stripe 重试重新分发，避免首次失败 / 进程崩溃后资金链路永远不再履约。新增 `shouldRedispatchWebhook` 纯函数 + 状态机单测覆盖。

## 批次 5 第一阶段已落地（2026-04-28）

- ✅ `services/payment/errors.go` / `service.go` 为 Stripe webhook 签名与解析失败补 sentinel；`handlers/payment.go` 不再靠 `strings.Contains(err.Error(), "签名"/"解析")` 做分支
- ✅ `services/payment/service.go` 去掉 `UpdatePlan`、`fulfillPayment`、`markPaymentFailed` 等路径的 `Save(&plan)` / `Save(&payment)` / `Save(&user)` 全字段写入，统一改按字段 `Updates(map)`

仍未完成：

- 多币种结算口径文档
- PlanGroup DTO 拆分
- 更大范围的支付/套餐治理尾项
