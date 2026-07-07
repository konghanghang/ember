# 按套餐分组的订阅自动通过额度实现方案

> 状态：草稿
> 负责人：Ember
> 更新时间：2026-07-07

## 背景

这个问题为什么现在要解决：

- Ember 当前求片订阅默认一律进入人工审核，管理员待办会混入大量低风险、重复模式明显的普通请求。
- 现有系统已经用 `PlanGroup` 表达用户长期权益边界，但订阅审核策略还没有进入这套分组治理，导致“会员分层”和“订阅审核”脱节。
- 当前登录态中间件明确不拦 `emby_disabled`，所以 Emby 侧已禁用或未绑定 Emby 的账号，理论上仍可进入订阅提交链路；这和“只有 Emby 可用账号才允许提交”不一致。
- 如果直接在创建后再补一次自动审批，而不改通知与审计语义，会产生假的管理员待办、错误的按钮消息，以及无法统计“今天哪些请求是系统自动通过”的问题。

## 目标

本方案要实现：

1. 支持按 `PlanGroup` 配置“每个用户每天可自动通过的订阅数”，在额度内直接通过订阅并触发现有 MoviePilot 下发链路。
2. 超过额度的订阅继续保持现有人工审核语义，不破坏管理员 `approve / reject` 工作流。
3. 只有存在 Emby 账号且当前未被 Emby 禁用的账号允许提交或重新提交订阅。
4. 自动通过的订阅只发送只读通知，不再生成带操作按钮的管理员审批消息。
5. 为自动通过与人工通过保留可审计来源，支持后续统计、排障和后台展示。

## 非目标

本次明确不做：

- 不把该能力挂到 `Plan`；订阅自动通过额度只属于 `PlanGroup` 权益。
- 不做每个用户单独覆盖额度，也不做白名单、黑名单或更复杂的审批策略引擎。
- 不因为超额而自动拒绝订阅；超额时仍进入人工审核。
- 不改现有 `APPROVED / INGESTED / REJECTED` 订阅状态机，也不新增新的用户可见状态。
- 不改 MoviePilot 自动订阅、手动补偿下载和 Emby webhook 入库收口的既有边界。
- 不新增独立设置中心页面；首版复用现有套餐分组管理页。

## 当前事实

以当前代码和现行文档为准，先写清现状：

- 相关文档：
  - `docs/system-architecture.md`
  - `docs/reference/data-model-reference.md`
  - `docs/reference/api-endpoint-catalog.md`
  - `docs/archive/plan/media-subscription/subscription-status-and-notification.md`
  - `docs/reference/web-design-guide.md`
- 相关服务/页面/模型：
  - `services/api/internal/services/subscription/service.go`
  - `services/api/internal/models/subscription.go`
  - `services/api/internal/models/user.go`
  - `services/api/internal/models/plan_group.go`
  - `services/api/internal/services/payment/plan_groups.go`
  - `services/api/internal/services/payment/plan_group_view.go`
  - `services/api/internal/handlers/payment.go`
  - `services/api/internal/middleware/password_reset_required.go`
  - `services/api/internal/services/telegram/wiring.go`
  - `services/web/src/views/admin/PlanGroupsView.vue`
  - `services/web/src/views/console/NewSubscriptionView.vue`
  - `services/web/src/views/console/SubscriptionsView.vue`
  - `services/web/src/types/api.ts`
- 当前行为：
  - 用户创建订阅时，`CreateSubscriptionWithResult` 会先做库内存在性检测，然后固定落库为 `PENDING`，再发送管理员待审批通知。
  - 管理员人工审批后，订阅转为 `APPROVED`，随后异步调用 MoviePilot；MoviePilot 失败只写 `mpError`，不回滚审批状态。
  - `PlanGroup` 已经是正式实体，负责长期用户权益边界；`Plan` 只是挂在 `PlanGroup` 下的售卖方案。
  - Bot 提交订阅复用同一个 subscription service，因此任何创建规则改动都会同时影响 Web 和 Bot。
  - 当前登录态中间件只校验 `is_active`，不校验 `emby_disabled` 或是否存在 `emby_id`。
- 现有限制：
  - 无法按用户分组差异化处理订阅审核。
  - 自动通过如果沿用当前通知路径，会给管理员发出假的待审批消息。
  - 当前 `subscriptions` 缺少“审批来源”字段，无法可靠区分手动通过与系统自动通过。

## 方案设计

### 1. 用户可见行为

- 新增能力：
  - 管理员可在套餐分组管理中为每个 `PlanGroup` 配置 `每日自动通过订阅数`。
  - 当用户当天在其有效分组额度内提交订阅时，订阅直接进入 `APPROVED`，用户侧立即看到“已通过，等待入库”。
  - 超过额度后，订阅继续进入 `PENDING`，管理员按现有方式人工处理。
  - 自动通过成功后，管理员只收到一条只读通知，明确说明“该订阅按分组额度自动通过”，不带 `通过 / 拒绝` 操作按钮。
  - 已绑定 Telegram 的用户继续收到现有“审核通过”结果通知；自动通过与人工通过在用户侧状态语义保持一致。
- 修改现有行为：
  - 创建订阅不再默认必经人工审核，而是先经过“账号可提交校验 + 分组额度判断”。
  - `PlanGroupsView.vue` 的新建/编辑分组表单增加额度字段，`0` 表示该分组关闭自动通过，全部走人工审核。
  - 订阅列表响应对象新增审批来源字段，后台可区分“自动通过”和“人工通过”。
- 哪些现有行为必须保持不变：
  - 提交前库内存在性检测、去重策略、拒绝后重新提交、MoviePilot 自动创建、手动补偿下载和 Emby webhook 入库收口保持现有主链路。
  - 超额订阅仍进入人工审核，不因为分组策略变化而自动拒绝。
  - 管理员人工审批入口、审批按钮和拒绝原因要求仅对 `PENDING` 订阅保留。
  - 自动通过不会改变用户侧的正式状态集合，仍然只使用 `PENDING / APPROVED / REJECTED / INGESTED`。
- 前端约束：
  - 前端实现必须遵守 Ember 风格。
  - 设计与交互基线以 `docs/reference/web-design-guide.md` 为准。
  - 套餐分组额度字段应复用现有 `PlanGroupsView.vue` 的表单骨架，不新增独立页面。
  - 若订阅列表展示审批来源，应以轻量 badge 或短标签表达，不增加解释性长文案。

### 2. 数据与模型

- 新增 `plan_groups` 字段：
  - `subscription_auto_approve_daily_limit integer not null default 0`
  - Go 字段建议：`SubscriptionAutoApproveDailyLimit int`
  - JSON 字段建议：`subscriptionAutoApproveDailyLimit`
- 新增 `subscriptions` 字段：
  - `review_source varchar(20) null`
  - Go 字段建议：`ReviewSource *SubscriptionReviewSource`
  - 枚举建议：
    - `MANUAL`：人工审核通过
    - `AUTO_QUOTA`：命中分组日额度后系统自动通过
  - `PENDING` 记录保持 `NULL`，直到真正被审核通过。
- 新增索引：
  - 为按用户 + 日期统计自动通过用量，建议增加面向 `AUTO_QUOTA` 的查询索引。
  - 示例方向：`(user_id, reviewed_at)` 上的部分索引，条件为 `review_source = 'AUTO_QUOTA'`。
- 修改哪些现有结构：
  - `PlanGroupView`、`ManagedPlanGroup`、PlanGroup 相关创建/更新 DTO 补充 `subscriptionAutoApproveDailyLimit`。
  - `Subscription` 响应对象补充 `reviewSource`，供后台列表和审计使用。
  - `ApproveSubscription` 人工审批成功时写入 `reviewSource=MANUAL`。
  - 自动通过路径写入 `reviewSource=AUTO_QUOTA`。
- 是否需要迁移：
  - 需要 SQL migration，放在 `infrastructure/database/`。
  - 迁移脚本要求幂等。
  - 需要同步修改 GORM 模型、TypeScript 类型和文档模型说明。

### 3. 接口与边界

- 计划修改的后台套餐分组接口：
  - `GET /api/v1/admin/plan-groups`
    - 返回每个分组的 `subscriptionAutoApproveDailyLimit`
  - `POST /api/v1/admin/plan-groups`
    - 请求体新增 `subscriptionAutoApproveDailyLimit`
  - `PUT /api/v1/admin/plan-groups/:key`
    - 请求体新增 `subscriptionAutoApproveDailyLimit`
- 计划修改的订阅接口：
  - `POST /api/v1/subscriptions`
  - `POST /api/v1/subscriptions/:id/resubmit`
  - 保持现有成功语义，但建议补充结构化返回字段：
    - `subscriptionId`
    - `status`
    - `autoApproved`
  - 为兼容现有前端，保留 `success` 顶层字段。
- 计划修改的订阅列表接口：
  - `GET /api/v1/subscriptions`
  - `GET /api/v1/admin/subscriptions`
  - 返回对象新增 `reviewSource`
- 计划保持不变的接口：
  - `PUT /api/v1/admin/subscriptions/:id/approve`
  - `PUT /api/v1/admin/subscriptions/:id/reject`
  - `PUT /api/v1/admin/subscriptions/:id/ingest`
  - `PUT /api/v1/admin/subscriptions/:id/redispatch`
  - 它们继续只服务人工审核或后续补偿链路。
- Bot / notifier 边界：
  - 需要为管理员新增“订阅已自动通过”的只读通知能力。
  - 自动通过通知不带审批操作，不应写入 `subscription_admin_notifications` 这类“等待后续消息编辑同步”的投递引用。
  - 用户侧“审核通过”通知仍复用现有结果通知能力。
- 哪些调用方会受影响：
  - Web 套餐分组管理页
  - Web 新建订阅页和订阅列表页
  - Telegram Bot 提交订阅入口
  - Bot 管理员通知格式化链路

### 4. 关键流程

按顺序写清主链路，不需要贴代码：

1. 用户通过 Web 或 Bot 提交订阅，先走现有参数校验和库内存在性检测。
2. Subscription service 在创建前查询当前用户：
   - 必须存在 `emby_id`
   - 必须满足 `emby_disabled=false`
   - 不满足则直接拒绝创建
3. service 解析用户有效分组：
   - 用户显式 `planGroup` 优先
   - 历史 `NULL` 用户回退到默认分组
4. 读取该 `PlanGroup.subscriptionAutoApproveDailyLimit`。
5. 若额度 `<= 0`，沿用现有人工审核路径：创建 `PENDING`，发送管理员待审批通知。
6. 若额度 `> 0`，在事务内先获取“用户 + 自然日”维度的 advisory lock，再统计该用户当日 `reviewSource='AUTO_QUOTA'` 的已通过数量。
7. 若当日自动通过数量 `< limit`：
   - 创建订阅时直接写入 `APPROVED`
   - 同步写入 `reviewedAt=now`
   - 写入 `reviewSource=AUTO_QUOTA`
   - 不发送带按钮的管理员审批消息
8. 自动通过事务提交后，异步执行：
   - MoviePilot 自动订阅创建
   - 用户“审核通过”结果通知
   - 管理员只读信息通知
9. 若当日自动通过数量 `>= limit`：
   - 创建订阅时保持 `PENDING`
   - `reviewSource` 保持 `NULL`
   - 继续发送现有管理员待审批消息
10. 管理员后续人工审批 `PENDING` 订阅时：
   - `ApproveSubscription` 写入 `reviewSource=MANUAL`
   - 其余审批语义保持不变
11. 拒绝后重新提交时，沿用与首次提交相同的账号校验和额度判断逻辑。
12. 后续 `APPROVED -> INGESTED` 收口、手动补偿下载、`redispatch` 和 webhook 入库逻辑保持不变。

关于“每天”的统计口径：

- 首版统一按系统运行时的同一自然日口径统计，不按浏览器时区或 Telegram chat 时区切换。
- 推荐复用系统既有时区配置约定，避免出现 Web、Bot、cron 各算各的一天。

### 5. 失败路径与边界条件

- 用户没有 `emby_id`：返回明确业务错误，不允许创建或重新提交订阅。
- 用户 `emby_disabled=true`：返回明确业务错误，不允许创建或重新提交订阅。
- 同一用户同一时刻并发提交多条订阅：
  - 必须通过“用户 + 日期”锁序列化额度判断，避免超发。
  - 现有“媒体资源去重锁”仍需保留，避免同一作品并发插入多条活跃订阅。
- 自动通过额度在当天中途被管理员下调：
  - 已自动通过的订阅不回滚。
  - 新提交请求按最新额度判断；若当日已用数量已超过新额度，后续全部进入人工审核。
- 用户在当天中途被切换分组：
  - 只影响切换后的新请求。
  - 已创建订阅保持原状态，不因分组变化回退。
- MoviePilot 自动订阅创建失败：
  - 自动通过订阅仍保持 `APPROVED`
  - 继续写 `mpError`
  - 不因为下游失败回滚审批结果
- 管理员只读通知发送失败：
  - 只记录日志，不回滚已通过订阅
- 兼容性约束：
  - 不得再给自动通过订阅发送带 `approve / reject` 操作的管理员消息。
  - 不得破坏现有 `PENDING` 订阅的人工审批和 Bot 审批链路。
  - 测试必须 mock Emby、MoviePilot 和 Telegram，不允许真实外调。

## 影响范围

涉及的子系统：

- API：有
  - `subscription` service 的创建、重提、审批来源写入和额度判断
  - `payment` / `plan_groups` 的 DTO、查询和更新接口
  - `models/subscription.go`、`models/plan_group.go`
  - 可能新增只读管理员通知 notifier 能力
- Web：有
  - `PlanGroupsView.vue` 增加额度字段
  - `types/api.ts`、`api/admin.ts`、订阅相关类型同步更新
  - 订阅列表可选展示 `reviewSource`
- Bot：有
  - 管理员订阅通知格式需要支持“只读自动通过通知”
  - 用户侧订阅成功结果通知路径需验证自动通过场景
- 配置/部署：无新增环境变量
  - 但“每天”的统计口径需要复用现有系统时区约定
- 数据库：有
  - 新增 `plan_groups` 字段、`subscriptions.review_source` 字段及相关索引
- 文档：需要更新
  - `docs/system-architecture.md`
  - `docs/reference/data-model-reference.md`
  - `docs/reference/api-endpoint-catalog.md`

## 验证方式

### 编译/测试

- `cd services/api && go test ./...`
- `cd services/api && go build ./...`
- `cd services/api && go vet ./...`
- `cd services/web && npm run test`
- `cd services/web && npm run build`
- `cd services/bot && python -m py_compile main.py`
- `cd services/bot && python -m pytest tests`

按改动补充针对性测试：

- `PlanGroup` 创建、更新、查询正确读写 `subscriptionAutoApproveDailyLimit`
- 订阅创建在额度内直接转 `APPROVED`，并写入 `reviewSource=AUTO_QUOTA`
- 超额后订阅保持 `PENDING`，并继续触发管理员待审批通知
- `ApproveSubscription` 人工通过时写入 `reviewSource=MANUAL`
- Emby 未绑定或已禁用账号提交订阅时被拒绝
- Bot 提交订阅与 Web 提交订阅在额度判断上保持一致
- 自动通过通知不包含审批按钮，也不要求管理员消息同步引用落库
- 并发提交不会突破单用户日额度

### 手工验证

- 为某个 `PlanGroup` 设置额度 `2`，同一用户当天连续提交 3 条订阅：
  - 前 2 条直接显示为“已通过，等待入库”
  - 第 3 条进入“审核中”
- 将用户 Emby 访问置为不可用后提交订阅，确认接口直接拒绝。
- 自动通过后，确认管理员只收到只读通知，不出现 `通过 / 拒绝` 按钮。
- 超额后的 `PENDING` 订阅仍可由管理员正常人工通过或拒绝。
- 自动通过后若 MoviePilot 失败，确认订阅保持 `APPROVED` 且后台可见 `mpError`。
- 拒绝后重新提交的订阅，确认仍参与当天额度判断。

## 落地后文档处理

落地后应同步处理：

- 将稳定结论同步到 `docs/system-architecture.md`：
  - `PlanGroup` 承载订阅自动通过额度
  - 订阅创建入口的“账号可提交校验 + 自动通过额度判断”链路
  - 自动通过与人工审批的通知边界
- 将字段与枚举同步到 `docs/reference/data-model-reference.md`
- 若创建订阅成功响应补充了 `status / autoApproved / subscriptionId`，同步到 `docs/reference/api-endpoint-catalog.md`
- 当代码、测试、文档同步完成，且自动通过与人工审核行为稳定后，将本方案移入 `docs/archive/plan/media-subscription/`
