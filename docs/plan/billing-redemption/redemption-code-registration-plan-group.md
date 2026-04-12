# 注册码绑定套餐分组实现方案

> 状态：草稿
> 负责人：Ember
> 更新时间：2026-04-12

## 背景

现在用户已经支持长期绑定 `planGroup`，续费中心、Checkout 和 Stripe webhook 也都按用户有效套餐分组工作。

但当前注册码链路还停留在“只决定天数和模板用户”的阶段，这会带来三个现实问题：

- 管理员给付费用户预制注册码时，不能顺手把用户应该进入的付费方案组一起绑定，注册后还得再去后台手工修用户 `planGroup`
- 注册码同时承担“注册门控”和“续期工具”两种职责，如果直接给 `redemption_codes` 塞一个模糊的 `planGroup` 字段，后面很容易把续期语义也污染掉
- 套餐分组已经是正式实体，删除/切换分组时当前只检查用户和套餐引用；如果注册码也开始引用分组，这条链路不补齐就会留下脏引用

这轮需求的核心不是“注册码也有套餐分组”，而是：**注册码在注册场景下可选地指定用户注册后应绑定到哪个付费方案组；直接续期场景继续只管延长时长，不改用户分组。**

## 目标

本方案要实现：

1. 管理员在生成或编辑注册码时，可选绑定一个“注册后生效”的套餐分组
2. 用户使用该注册码注册时，系统把所选套餐分组写入新用户的显式 `planGroup`
3. 用户使用同一类注册码做直接续期时，忽略注册码上的套餐分组，仅按现有逻辑延长有效期
4. 后台兑换码管理页和注册页能明确展示该绑定关系，避免黑盒行为
5. 套餐分组治理链路补齐，避免删除已被注册码引用的分组

## 非目标

本次明确不做：

- 不把兑换码拆成“注册码”和“续期码”两套模型
- 不让直接续期行为顺带修改用户 `planGroup`
- 不批量回填历史注册码或历史用户分组
- 不重做现有 Stripe 支付、续费中心或套餐分组管理的大结构

## 当前事实

以当前代码和现行文档为准，先写清现状：

- 相关文档：`docs/system-architecture.md`、`docs/reference/web-design-guide.md`
- 相关服务/页面/模型：
  - `services/api/internal/models/redemption_code.go`
  - `services/api/internal/services/redemption/code_service.go`
  - `services/api/internal/services/redemption/service.go`
  - `services/api/internal/services/auth/register.go`
  - `services/api/internal/services/auth/register_persist.go`
  - `services/api/internal/services/payment/plan_groups.go`
  - `services/web/src/views/admin/RedemptionCodesView.vue`
  - `services/web/src/views/user/RegisterView.vue`
  - `services/web/src/views/console/RenewalCenterView.vue`
- 当前行为：
  - `redemption_codes` 只保存 `defaultDays`、`templateUserId`、`expiresAt`、`notes` 等信息，没有绑定套餐分组
  - 邀请注册模式下，`AuthService.prepareRegister()` 只读取注册码有效性和 `defaultDays`，不会决定新用户 `planGroup`
  - 注册成功时，`buildRegisteredUser()` 不写 `users.planGroup`，所以用户默认跟随当前默认套餐分组
  - 直接续期 `RedeemCode()` 只延长 `expiresAt` 并记录兑换历史，不处理套餐分组
  - 注册页 `/register` 和续费页 `/console/renewal` 目前都复用同一个 `ValidateCode()` 语义
- 现有限制：
  - 需求里“注册使用绑定分组，续期忽略绑定分组”已经要求两条链路分开看待；继续共用一个模糊字段和一个通用校验方法，后面一定继续长特殊情况

## 方案设计

### 前端约束

- 前端实现必须遵守 Ember 风格
- 设计与交互基线以 `docs/reference/web-design-guide.md` 为准
- 若存在偏离规范的特例，必须单独写明原因、范围和收口条件

### 1. 用户可见行为

- 新增能力：
  - 管理员在 `兑换中心 -> 兑换码管理` 创建、批量创建、编辑注册码时，可选选择“注册套餐分组”
  - 注册页在预验证成功后，展示该注册码注册后将绑定的套餐分组；未绑定时明确显示“注册后跟随默认分组”
- 修改行为：
  - 邀请注册模式下，如果注册码绑定了注册套餐分组，新用户注册成功后直接落到该显式分组，而不是跟随默认分组
  - 兑换码列表增加“注册套餐分组”展示字段；建议同时支持按该字段筛选，避免后台无法盘点
- 保持不变：
  - `/console/renewal` 兑换码续期仍只增加天数，不改 `users.planGroup`
  - 现有支付链路仍以用户当前有效套餐分组为准，不读取兑换码信息
  - 未绑定注册套餐分组的旧码和新码保持原有语义：注册后继续跟随默认分组

### 2. 数据与模型

#### `redemption_codes` 新增注册场景专用字段

给 `services/api/internal/models/redemption_code.go` 新增可空字段：

- `registrationPlanGroup`：注册码在“注册场景”下要写入新用户的显式套餐分组 key

命名上明确使用 `registrationPlanGroup`，不要偷懒直接叫 `planGroup`。原因很简单：这个字段**只对注册生效**，对续期无效，名字不收口，逻辑边界就会烂掉。

推荐模型变化：

- 持久化字段：`RegistrationPlanGroup *string  gorm:"column:registrationPlanGroup;size:50;index"`
- 展示字段：`RegistrationPlanGroupName *string gorm:"-"`

约束：

- 允许为空；为空表示注册后仍跟随默认分组
- 非空时必须引用当前存在的 `plan_groups.key`
- 不在数据库层加外键，继续沿用当前应用层校验方式

#### 迁移脚本

新增 SQL migration，例如：`infrastructure/database/20260412_01_add_registration_plan_group_to_redemption_codes.sql`

迁移要求：

- 给 `redemption_codes` 增加可空列 `registrationPlanGroup`
- 给该列增加索引
- 不回填历史数据
- 脚本幂等，可重复执行

#### 其他模型

- `users`：不新增字段，继续复用现有 `planGroup`
- `redemptions`：本次不新增字段，继续记录 `code` 和 `days`

这里故意不往 `redemptions` 再塞一份分组快照。当前业务真正长期生效的是 `users.planGroup`，注册码只负责注册瞬间的落点。先把边界收口，别急着堆审计字段。

### 3. 接口与边界

#### 后台兑换码接口

以下接口新增或透出字段：

- `GET /api/v1/admin/redemption-codes`
- `POST /api/v1/admin/redemption-codes`
- `POST /api/v1/admin/redemption-codes/batch`
- `PUT /api/v1/admin/redemption-codes/:id`

新增请求/响应字段：

- `registrationPlanGroup`
- `registrationPlanGroupName`

建议查询参数补充：

- `registrationPlanGroup`：后台按绑定分组筛选注册码

校验边界：

- 创建/编辑时若传了 `registrationPlanGroup`，必须先校验分组 key 合法且分组存在
- 返回列表时补齐 `registrationPlanGroupName`，避免前端再自行拼装

#### 注册校验接口与续期校验接口分离语义

当前 `/register/code/:code/validate` 和 `/redeem/:code/validate` 都复用 `ValidateCode()`，这轮必须拆语义：

- `GET /api/v1/register/code/:code/validate`
  - 校验兑换码本身有效
  - 若 `registrationPlanGroup` 非空，还要校验该分组仍存在
  - 返回注册页需要展示的分组信息
- `GET /api/v1/redeem/:code/validate`
  - 只校验兑换码本身有效
  - 忽略 `registrationPlanGroup`

实现方式可以是：

- `RedemptionCodeService.ValidateRegistrationCode()`
- `RedemptionCodeService.ValidateRenewalCode()`

不要继续拿一个 `ValidateCode()` 加布尔参数凑合；代码一旦允许“有的调用方看分组，有的调用方不看分组”，语义应该进函数名，不该藏在调用点猜。

#### 注册接口

- `POST /api/v1/user/register`

请求结构不变，仍然只传 `code`

服务端变化：

- invite 模式下读取注册码后，若 `registrationPlanGroup` 非空，则在创建 `users` 记录时把它写入 `user.planGroup`
- 若为空，则保持当前行为，不写显式分组，让用户跟随默认分组

#### 套餐分组管理接口

现有计划分组治理链路要补一条引用检查：

- `DELETE /api/v1/admin/plan-groups/:key`

删除分组时，除了检查 `plans` 和 `users`，还必须检查：

- `redemption_codes.registrationPlanGroup`

否则管理员删掉一个仍被注册码引用的分组，后面注册校验就会炸。

错误语义也要同步收口为：

- “套餐分组仍被用户、套餐或注册码引用，不能删除”

### 4. 关键流程

#### 管理员生成注册码

1. 管理员在兑换码管理页填写基础信息：数量、次数、天数、模板用户、过期时间、备注
2. 可选选择 `registrationPlanGroup`
3. 后端校验该分组存在后写入 `redemption_codes.registrationPlanGroup`
4. 列表和批量结果中返回该绑定分组信息

#### 用户使用注册码注册

1. 注册页请求 `/register/code/:code/validate`
2. 后端校验兑换码有效，并校验 `registrationPlanGroup` 是否仍存在
3. 用户提交注册请求
4. `AuthService.prepareRegister()` 读取注册码的 `defaultDays` 与 `registrationPlanGroup`
5. `buildRegisteredUser()` 或等价构造步骤在建用户时写入：
   - 绑定分组时：`users.planGroup = registrationPlanGroup`
   - 未绑定分组时：`users.planGroup = nil`
6. 事务内创建用户、记录兑换历史、递增 `usedCount`
7. 注册完成后，用户续费中心直接看到该分组下的可购方案

#### 用户使用注册码直接续期

1. 用户在 `/console/renewal` 或 Bot 续期入口提交兑换码
2. 后端只做兑换码有效性校验和重复兑换校验
3. 系统延长 `users.expiresAt`、必要时解封 Emby、记录兑换历史
4. **不读取也不修改** `registrationPlanGroup` / `users.planGroup`

#### 管理员删除套餐分组

1. 管理员尝试删除某个分组
2. 后端在事务里检查 `plans`、`users`、`redemption_codes`
3. 只要任一链路仍有引用，立即拒绝删除

### 5. 失败路径与边界条件

- 绑定分组后来被删除：
  - 注册校验和正式注册都应失败，提示注册码绑定的套餐分组无效
  - 直接续期仍允许成功，因为续期不依赖该字段
- 多次可用注册码被管理员中途改了绑定分组：
  - 只影响后续尚未注册的使用次数
  - 已经注册成功的用户保持自己的 `users.planGroup`，不做反向改写
- 注册码未绑定分组：
  - 保持历史兼容语义，新用户继续跟随默认分组
- 模板用户与注册分组同时存在：
  - 模板用户只负责复制 Emby 权限
  - `registrationPlanGroup` 只负责决定注册后的账单分组，两者不互相覆盖
- 兼容性约束：
  - 不能破坏现有续期行为
  - 不能让默认分组切换意外改写“通过注册码显式绑定过分组”的用户
  - 不能继续允许删除仍被注册码引用的套餐分组

## 影响范围

涉及的子系统：

- API：有，涉及 `redemption`、`auth`、`payment plan_groups` 三条链路
- Web：有，涉及 `services/web/src/views/admin/RedemptionCodesView.vue`、`services/web/src/views/user/RegisterView.vue`；续费页只做回归验证，不新增行为
- Bot：低影响，若 Bot 端存在兑换码预校验或提示文案，只需确认续期语义未被误伤；本次不新增 Bot 功能
- 配置/部署：有，涉及新增 SQL migration，需要按现有数据库变更流程执行
- 文档：落地后同步更新 `docs/system-architecture.md`；若前端交互口径有稳定结论，可补充到 `docs/reference/web-design-guide.md` 或现有后台页面规范文档

## 验证方式

### 编译/测试

- `cd services/api && go test ./...`
- `cd services/api && go build ./...`
- `cd services/web && npm run build`

### 手工验证

- 后台创建一个绑定 `VIP_A` 的注册码，列表中能看到绑定分组名称，编辑后可正确保存
- invite 模式下用绑定 `VIP_A` 的注册码注册新用户，注册成功后该用户在后台显示为显式 `VIP_A`，续费中心只看到 `VIP_A` 下的方案
- invite 模式下用未绑定分组的注册码注册，确认仍跟随默认分组
- 使用绑定了分组的注册码给已有用户直接续期，确认只增加天数，不修改该用户当前 `planGroup`
- 尝试删除仍被注册码引用的套餐分组，确认后端拒绝
- 删除一个无用户、无套餐、无注册码引用的套餐分组，确认仍可成功

## 落地后文档处理

落地后应同步处理：

- 将 `redemption_codes.registrationPlanGroup`、注册与续期校验语义分离、套餐分组删除引用面补充到 `docs/system-architecture.md`
- 这份方案在代码、迁移、验证、文档同步都完成后，移入 `docs/archive/plan/billing-redemption/`
