# 用户套餐分组实现方案

> 状态：已归档
> 负责人：Ember
> 更新时间：2026-04-12

## 归档说明

本方案已完成落地，当前只保留追溯价值。

稳定结论已同步到：

- `docs/system-architecture.md`
- `docs/runbooks/stripe-payment-testing.md`

当前代码已经具备：

- `plan_groups` 实体和默认分组
- 用户显式分组 / 跟随默认分组两种模式
- 后台套餐分组管理
- 套餐、续费中心、checkout、webhook 按用户有效分组工作

因此这份文档不再承担现行实现说明职责。

## 背景

当前代码虽然已经给 `users` / `plans` 增加了 `planGroup`，也补了支付链路上的分组隔离，但本质上还是把套餐分组写死在代码和初始化里，而不是做成正式可管理的实体。

这有三个问题：

- 分组不是实体，后台不能创建、命名、排序或删除分组，后续再加新组就得继续改代码和迁移
- “默认分组”不存在，用户只能被硬绑到某个值；一旦希望一批用户跟随默认分组切换，就只能批量改用户数据
- 现有支付隔离是建立在硬编码分组值上的，功能能跑，但边界不稳，后续扩展会继续返工

现在需要把它收口成正式的“套餐分组管理”能力：

- 先创建套餐分组，再把套餐分配到某个分组
- 用户可以显式关联分组，也可以不关联
- 用户未显式关联时，系统按“默认分组”决定其可见/可购套餐
- 支付链路继续沿用现有 Stripe Checkout，但必须改按“有效分组”校验

## 目标

本方案要实现：

1. 新增可管理的 `plan_groups` 实体，后台支持创建、编辑、删除、设置默认分组
2. 套餐改为绑定已有分组，不再写死固定分组值
3. 用户支持“显式绑定分组”或“跟随默认分组”两种模式
4. 续费中心、checkout、webhook 履约统一按用户“有效分组”工作
5. 默认分组切换时，不需要批量改所有跟随默认的用户数据

## 非目标

本次明确不做：

- 不做自动分组规则（如按渠道、注册来源、标签自动分配）
- 不做分组级价格模板、区域价格规则引擎
- 不做多支付 provider 抽象
- 不做用户分组批量迁移工具；老用户是否改成“跟随默认”由管理员后续按需处理

## 当前事实

以当前代码和现行文档为准，先写清现状：

- 相关文档：`docs/system-architecture.md`
- 相关服务/页面/模型：
  - `services/api/internal/models/user.go`
  - `services/api/internal/models/plan.go`
  - `services/api/internal/services/payment/service.go`
  - `services/api/internal/services/user/admin.go`
  - `services/web/src/views/admin/PaymentCenterView.vue`
  - `services/web/src/views/admin/PlansView.vue`
  - `services/web/src/views/admin/UsersView.vue`
  - `services/web/src/views/console/RenewalCenterView.vue`
- 当前行为：
  - `plan_groups` 已经是正式实体，后台支持创建、编辑、删除和切换默认分组
  - 用户 `planGroup` 为空时表示“跟随默认分组”，显式分组优先级高于默认分组
  - 登录态套餐接口、checkout、webhook 履约都按用户有效分组工作
  - 默认分组切换和用户/套餐分组变更都会同步收口相关 `pending` 支付
- 已完成收口：
  - 后台已有套餐分组管理页面
  - 套餐管理和用户管理都已接入分组选择
  - 支付记录、续费中心和履约边界已与分组模型保持一致

## 方案设计

### 1. 用户可见行为

- 新增能力：
  - 管理员可在支付中心维护套餐分组
  - 用户支持“显式绑定分组”或“跟随默认分组”
- 修改行为：
  - 套餐不再依赖写死分组值，而是依赖后台维护的分组 key
  - 用户未显式绑定分组时，续费中心展示默认分组的套餐
- 保持不变：
  - 续费中心仍走现有 Stripe Checkout
  - 支付成功后仍按现有逻辑续期 `users.expiresAt`
  - 支付链路仍要求组隔离，不能通过旧 Checkout Session 越过当前分组

### 2. 数据与模型

#### 新增 `plan_groups` 表

新增 `plan_groups`：

- `key`：主键，分组稳定标识，后台创建时填写，后续不可修改
- `name`：分组展示名称
- `description`：可选说明
- `isDefault`：是否默认分组，全局唯一
- `sortOrder`：排序
- `createdAt` / `updatedAt`

约束：

- `key` 全局唯一
- 任意时刻必须且只能有一个默认分组
- 默认分组不能删除

#### 用户模型

修改 `services/api/internal/models/user.go`：

- `planGroup` 改为可空字符串
- 语义变为“显式绑定的分组 key”
- `NULL` 表示“跟随默认分组”

额外返回的计算字段：

- `effectivePlanGroup`：当前用户实际生效的分组 key
- `effectivePlanGroupName`：当前用户实际生效的分组名称
- `isUsingDefaultPlanGroup`：是否正在跟随默认分组

#### 套餐模型

修改 `services/api/internal/models/plan.go`：

- `planGroup` 保持为非空字符串，但语义改为“所属分组 key”
- 不再限制为固定枚举值
- 必须指向已存在的分组

#### 数据库迁移

更新 `infrastructure/database/20260408_01_add_plan_grouping.sql`：

- 新增 `plan_groups` 表
- 初始化一个默认分组 `DEFAULT`
- `plans.planGroup` 扩展为普通分组 key，并回填到 `DEFAULT`
- `users.planGroup` 改为可空；历史用户统一保持 `NULL`，让现有用户默认跟随默认分组
- 给用户/套餐分组字段补索引，不在数据库层增加外键

> 本次不使用 surrogate id，直接以分组 `key` 作为稳定引用，减少迁移和接口噪音。

### 3. 接口与边界

#### 新增后台分组管理接口

- `GET /api/v1/admin/plan-groups`
- `POST /api/v1/admin/plan-groups`
- `PUT /api/v1/admin/plan-groups/:key`
- `DELETE /api/v1/admin/plan-groups/:key`

返回字段至少包括：

- `key`
- `name`
- `description`
- `isDefault`
- `sortOrder`
- `planCount`
- `userCount`

说明：

- `key` 创建后不可修改
- `PUT` 允许把某个分组设为新的默认分组
- `DELETE` 必须拒绝删除默认分组或仍被用户/套餐引用的分组
- 分组存在性和引用检查全部放在应用层，不依赖数据库外键

#### 用户接口

- `GET /api/v1/admin/users`
  - 继续支持按分组筛选
  - 筛选语义改为“按有效分组筛选”
- `PUT /api/v1/admin/users/:id`
  - `planGroup` 支持三种语义：
    - 不传：不修改
    - 传有效 key：显式绑定到该分组
    - 传空字符串：清空显式绑定，改为跟随默认分组

#### 套餐接口

- `GET /api/v1/admin/plans`
  - 继续支持按分组筛选
- `POST /api/v1/admin/plans`
  - `planGroup` 必须是已存在的分组 key
- `PUT /api/v1/admin/plans/:id`
  - `planGroup` 改为已存在的分组 key

#### 用户侧套餐与支付接口

- `GET /api/v1/payments/plans`
- `GET /api/v1/plans`（认证兼容别名）
- `POST /api/v1/payments/checkout`

统一边界：

- 先解析用户有效分组：`用户显式分组 ?? 默认分组`
- 只返回该有效分组下的启用套餐
- checkout 只能购买该有效分组下的套餐
- webhook 履约前再次按当前有效分组校验

### 4. 关键流程

#### 后台创建分组

1. 管理员创建新的套餐分组，填写 `key/name`
2. 如果设为默认分组，系统在事务里取消旧默认分组
3. 新分组立即可用于套餐和用户配置

#### 用户查看套餐

1. 用户进入续费中心
2. 前端请求 `GET /api/v1/payments/plans`
3. 后端读取用户显式分组
4. 若用户未显式分组，则回退到默认分组
5. 返回该有效分组下的启用套餐

#### 用户购买套餐

1. 用户提交 `planId`
2. 后端解析用户当前有效分组
3. 后端校验套餐是否属于该分组
4. 校验通过后创建 Checkout Session
5. webhook 履约前再次复核当前有效分组

#### 默认分组切换

1. 管理员把默认分组从旧组切到新组
2. 显式绑定用户不受影响
3. 跟随默认的用户立即切到新组
4. 系统同步把“跟随默认”用户关联的 `pending` 支付标记为 `expired`
5. 即使有漏网之鱼，webhook 履约前也会再复核当前有效分组

### 5. 失败路径与边界条件

- 没有默认分组：套餐查询、checkout、webhook 履约都必须报错，不能默默放行
- 用户传了不存在的分组 key：更新用户/创建套餐/更新套餐都必须拒绝
- 删除默认分组：必须拒绝
- 删除仍被引用的分组：必须拒绝，不能把用户或套餐静默改到别的组
- 默认分组切换后，跟随默认的用户旧 pending 支付仍可继续：系统必须同步收口
- 显式绑定某分组的用户在默认分组切换后被误切到新默认：不能发生，显式绑定优先级必须高于默认分组
- 兼容性约束：
  - 不能破坏现有 Stripe 支付记录、续期逻辑、支付审计
  - 不能让旧 Checkout Session 越过当前有效分组继续履约

## 影响范围

涉及的子系统：

- API：有，涉及分组实体、用户管理、套餐管理、支付链路
- Web：有，涉及支付中心新增分组管理、用户编辑、套餐编辑、TS 类型
- Bot：无
- 配置/部署：有，需要更新 SQL migration；不新增环境变量
- 文档：需要同步 `docs/system-architecture.md`

## 落地验证与收口

已完成的关键收口：

- `infrastructure/database/20260408_01_add_plan_grouping.sql` 已落地
- `services/api/internal/services/payment/plan_groups.go` 已承载分组实体与默认分组切换逻辑
- `services/api/internal/services/payment/service.go` 已按有效分组收口套餐查询、checkout 和 webhook 履约
- `services/api/internal/services/user/admin.go` 已按有效分组支持后台筛选和编辑
- `services/web/src/views/admin/PlanGroupsView.vue`、`services/web/src/views/admin/PlansView.vue`、`services/web/src/views/admin/UsersView.vue` 已完成页面接入
- `docs/system-architecture.md` 已同步实体、接口和关键服务边界

保留的历史验证清单：

- 创建新分组，确认套餐管理可选中该分组，列表展示名称正确
- 把某用户显式绑定到某分组，确认续费中心只看到该组套餐
- 把某用户分组清空为“跟随默认”，切换默认分组后确认无需改用户数据即可看到新组套餐
- 手工构造 checkout 请求，让用户购买其他分组套餐，确认后端拒绝
- 切换默认分组后，确认跟随默认用户的待支付订单被收口，旧 Stripe 页面无法再成功续期
- 删除被用户或套餐引用的分组，确认后台收到拒绝

## 文档处理结果

已完成：

- `plan_groups`、默认分组、有效分组解析、后台分组管理接口等稳定事实已同步到 `docs/system-architecture.md`
- 本文档已移入 `docs/archive/plan/billing-redemption/`
