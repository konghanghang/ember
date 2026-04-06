# 用户套餐分组实现方案

> 状态：草稿
> 负责人：Ember
> 更新时间：2026-04-06

## 背景

当前续费中心对所有登录用户展示同一批套餐，无法实现“部分用户看 A 类套餐，另一部分用户看 B 类套餐”的定向售卖。

现在已经明确的业务需求是：

- 用户按“直绑”方式归属某个套餐组
- 套餐分为 A / B 两组
- A/B 两组完全隔离
- 两组套餐结构相同，主要是价格不同
- 支付链路继续复用现有 Stripe Checkout

如果不做这层分组，后台只能维护一套统一价格，无法按用户分层运营；如果只在前端隐藏而不改后端校验，用户仍可能越权购买不属于自己的套餐。

## 目标

本方案要实现：

1. 支持用户归属 A / B 套餐组
2. 支持套餐归属 A / B 套餐组
3. 用户续费中心只展示自己所属组的套餐
4. 后端 checkout 阶段拒绝购买其他组套餐
5. 后台支持维护用户套餐组和套餐套餐组

## 非目标

本次明确不做：

- 不重构为多支付 provider 架构
- 不做套餐模板表、价格版本表、区域价格规则引擎
- 不改 Stripe webhook 和支付履约主链路
- 不做自动分组规则，本次只支持管理员按用户直绑

## 当前事实

以当前代码和现行文档为准，先写清现状：

- 相关文档：`docs/system-architecture.md`
- 相关服务/页面/模型：
  - `services/api/internal/models/user.go`
  - `services/api/internal/models/plan.go`
  - `services/api/internal/services/payment/service.go`
  - `services/web/src/views/console/RenewalCenterView.vue`
  - `services/web/src/views/admin/UsersView.vue`
  - `services/web/src/views/admin/PlansView.vue`
- 当前行为：续费中心通过现有套餐接口展示启用套餐，所有用户看到同一批套餐，并通过 Stripe Checkout 下单
- 现有限制：`User` 和 `Plan` 均没有套餐分组字段，`CreateCheckoutSession(...)` 也没有按用户归属校验套餐可购买范围

## 方案设计

### 1. 用户可见行为

- 新增能力：不同用户登录后看到不同套餐组
- 修改行为：续费中心不再显示全量套餐，而是只显示当前用户所属组的套餐
- 保持不变：支付仍然走现有 Stripe Checkout，支付成功后仍按现有逻辑续期 `users.expiresAt`

### 2. 数据与模型

#### 用户模型

修改 `services/api/internal/models/user.go`：

- 新增字段 `planGroup`
- 类型：字符串
- 约束：非空，默认值 `A`
- 语义：表示该用户可见并可购买的套餐组

#### 套餐模型

修改 `services/api/internal/models/plan.go`：

- 新增字段 `planGroup`
- 类型：字符串
- 约束：非空，默认值 `A`
- 语义：表示该套餐属于 A 组或 B 组

#### 数据库迁移

新增 SQL migration 到 `infrastructure/database/`：

- 给 `users` 增加 `planGroup` 列，默认 `A`
- 给 `plans` 增加 `planGroup` 列，默认 `A`
- 回填历史空值为 `A`
- 视需要为两个字段补索引

> 本次不新增新表。

### 3. 接口与边界

#### 用户侧套餐接口

当前公开 `GET /plans` 不适合作为购买页主接口，因为它没有用户上下文。

建议新增登录态套餐接口，例如：

- `GET /api/v1/payments/plans`

行为：

- 从当前登录用户读取 `planGroup`
- 只返回 `isActive = true` 且 `planGroup = 当前用户.planGroup` 的套餐

#### checkout 接口

继续复用：

- `POST /api/v1/payments/checkout`

新增边界约束：

- 后端在 `CreateCheckoutSession(...)` 中先查询当前用户的 `planGroup`
- 查询套餐时必须附加 `planGroup = 当前用户.planGroup`
- 如果套餐不属于当前用户组，则拒绝下单

#### 管理后台接口

用户管理接口需要支持：

- 返回 `planGroup`
- 更新 `planGroup`
- 可选按 `planGroup` 筛选列表

套餐管理接口需要支持：

- 创建套餐时填写 `planGroup`
- 编辑套餐时修改 `planGroup`
- 列表可按 `planGroup` 筛选

### 4. 关键流程

#### 用户查看套餐

1. 用户登录后进入续费中心
2. 前端调用登录态套餐接口
3. 后端读取当前用户 `planGroup`
4. 后端返回该组启用套餐
5. 前端只渲染这一组套餐

#### 用户购买套餐

1. 用户在续费中心选择套餐并提交 `planId`
2. 后端在 `CreateCheckoutSession(...)` 中读取当前用户 `planGroup`
3. 后端查询该 `planId` 时同时校验套餐组归属
4. 校验通过后创建 Stripe Checkout Session
5. 支付成功后继续复用现有 webhook 与续期流程

#### 后台管理分组

1. 管理员在用户管理页编辑用户 `planGroup`
2. 管理员在套餐管理页创建或编辑 A/B 套餐
3. 用户下次进入续费中心时按最新分组看到对应套餐

### 5. 失败路径与边界条件

- 用户提交了其他组的 `planId`：后端必须拒绝，不能创建 checkout session
- 历史用户和历史套餐没有分组：迁移时统一回填到 `A`，避免老数据不可用
- 用户切换套餐组后，前端展示应以下次拉取的接口结果为准，不依赖本地缓存拼装
- 兼容性约束：不能破坏现有 Stripe webhook、支付记录查询和续期逻辑

## 影响范围

涉及的子系统：

- API：有，涉及用户模型、套餐模型、套餐查询、checkout 校验、后台管理接口
- Web：有，涉及续费中心、用户管理、套餐管理、TS 类型定义
- Bot：无
- 配置/部署：有，需要新增 SQL migration，但不新增运行期环境变量
- 文档：后续如该方案落地，需要同步 `docs/system-architecture.md`

## 验证方式

### 编译/测试

- `cd services/api && go test ./...`
- `cd services/api && go build ./...`
- `cd services/web && npm run build`

### 手工验证

- 保持历史用户和历史套餐为 A 组，确认现有 A 组用户仍可正常看到原套餐并下单
- 新建 B 组套餐，并把部分用户切到 B 组，确认他们只能看到 B 组套餐
- 手工构造请求，让 A 组用户提交 B 组 `planId` 到 checkout，确认后端拒绝
- 后台修改用户 `planGroup` 后，重新进入续费中心，确认显示套餐随分组切换
- 后台创建/编辑套餐时可正确选择 A/B，列表展示和筛选正确

## 落地后文档处理

落地后应同步处理：

- 将用户套餐分组、套餐分组、登录态套餐接口等稳定事实同步到 `docs/system-architecture.md`
- 当功能上线并完成验证后，将本方案移入 `docs/archive/plan/billing-redemption/`
