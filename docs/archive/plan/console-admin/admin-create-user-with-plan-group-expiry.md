# 后台创建用户并设置套餐组与到期时间实现方案

> 状态：已归档
> 负责人：Ember
> 更新时间：2026-04-12

## 归档说明

本方案已完成落地，当前只保留历史追溯价值。

稳定结论已同步到：

- `docs/system-architecture.md`

当前代码已经具备：

- 后台 `用户管理` 页面新建用户入口
- `POST /api/v1/admin/users`
- 后台创建用户时显式设置 `planGroup`
- 后台创建用户时设置 `expiresAt` 或 `neverExpire`

因此这份文档不再承担现行实现说明职责。

## 背景

当前后台 `用户管理` 只能管理已存在账号，不能直接创建新用户。现在真正会建号的只有公开注册链路：

- `POST /api/v1/user/register` 受 `registration_mode`、邮箱验证码、邀请码约束
- 注册链路会先创建 Emby 账号，再落本地 `users`
- 管理员如果想给线下付款、人工审核通过或迁移过来的用户直接开账号，只能绕到公开注册或手工改库，做法很脏

同时，这轮需求已经收口清楚了：用户长期绑定的不是 `planId`，而是 `planGroup`。

按当前系统真实模型：

- 用户状态是 `planGroup + expiresAt`
- `plans` 是可售方案，不是用户订阅快照
- `payments` 是 Stripe 订单账本，不是后台人工开通记录

所以，后台创建用户不应该再拿 `planId` 当核心字段。要做的就是直接创建一个普通用户，并一次性设置：

- 用户所属 `planGroup`
- 用户 `expiresAt`

这才和当前模型、续费中心、支付链路保持一致。

## 目标

本方案要实现：

1. 在 `用户管理` 页面直接创建普通用户
2. 创建时直接指定用户所属 `planGroup`
3. 创建时直接指定用户到期时间 `expiresAt`
4. 后端复用现有 Emby 建号 + 本地建号顺序，失败时正确回滚
5. 不改变现有公开注册、Stripe 支付、续费中心的既有语义

## 非目标

本次明确不做：

- 不做用户与 `planId` 的长期绑定
- 不做管理员批量导入 / 批量开通
- 不把后台人工开通伪装成 Stripe `payments` 记录
- 不新增 `currentPlanId`、`effectiveDays` 之类和当前模型不一致的字段
- 不改现有用户编辑页、续费中心、支付履约的核心语义

## 当前事实

以当前代码和现行文档为准，先写清现状：

- 相关文档：
  - `docs/system-architecture.md`
  - `docs/reference/web-design-guide.md`
- 相关服务/页面/模型：
  - `services/web/src/views/admin/UsersView.vue`
  - `services/web/src/api/admin.ts`
  - `services/web/src/types/api.ts`
  - `services/api/internal/app/routes.go`
  - `services/api/internal/handlers/user.go`
  - `services/api/internal/services/user/admin.go`
  - `services/api/internal/services/auth/register.go`
  - `services/api/internal/services/auth/register_persist.go`
  - `services/api/internal/services/payment/plan_groups.go`
  - `services/api/internal/models/user.go`
- 当前行为：
  - `UsersView` 已支持“新建用户”入口和创建弹窗
  - 后端已提供 `POST /api/v1/admin/users`
  - 后台创建会创建 Emby 用户并落本地 `users`
  - 创建表单和用户状态语义已经按 `planGroup + expiresAt/neverExpire` 收口
  - 续费中心仍按用户有效 `planGroup` 返回可购方案，未引入 `planId` 长期绑定
- 已完成收口：
  - 后台人工开通不再借道公开注册
  - 后台创建用户不再伪造 Stripe `payments`
  - 创建链路与当前用户模型边界保持一致

## 方案设计

### 1. 用户可见行为

- 在 `services/web/src/views/admin/UsersView.vue` 的标题区右侧新增主操作按钮 `新建用户`
- 点击后打开创建弹窗，字段固定为：
  - `username`
  - `email`
  - `password`
  - `planGroup`
  - `neverExpire`
  - `expiresAt`
- `password` 支持管理员手输，也支持前端一键随机生成
- `planGroup` 直接读取现有套餐组列表，管理员明确选择用户所属组
- 到期时间使用 `datetime` 选择器，直接设置绝对时间，而不是先填“有效天数”再换算
- 当 `neverExpire = true` 时，隐藏 `expiresAt` 输入，并按“永不过期”创建
- 创建成功后，展示一次性成功结果，提醒管理员复制账号和密码，然后刷新用户列表
- 现有编辑用户、延长有效期、切换套餐组、重置密码等能力保持不变

这样做的原因很直接：

- 创建表单直接对应用户真实状态
- 管理员看到的就是最终生效结果
- 不再需要在 `plan.days`、`effectiveDays`、`expiresAt` 之间做二次换算

### 2. 数据与模型

> 本次不涉及数据模型变更。

保持当前模型边界：

- `users` 继续只存：
  - `planGroup`：用户显式绑定的套餐组
  - `expiresAt`：当前有效期，`nil` 表示永不过期
- `plans` 继续只表示可售方案
- `payments` 继续只表示 Stripe 订单

后台创建时直接落用户真实状态：

- `planGroup = 管理员选择的套餐组`
- `expiresAt = 管理员选择的时间`，或 `nil`

这里不允许再引入“后台创建时选了哪个 `planId`”这种持久化信息。因为那不是当前用户模型的一部分，硬塞进去只会制造二义性。

### 3. 接口与边界

新增后台接口：

- `POST /api/v1/admin/users`

请求体：

- `username`：必填，沿用注册链路的用户名规则（3~50 位，仅字母数字）
- `email`：必填，合法邮箱，且全局唯一
- `password`：必填，最少 6 位
- `planGroup`：必填，必须是已存在的套餐组 key
- `expiresAt`：可选，RFC3339；当 `neverExpire=false` 时必填
- `neverExpire`：可选，`true` 表示创建为永不过期

请求约束：

- `neverExpire=true` 时，不允许再传 `expiresAt`
- `neverExpire=false` 时，必须传合法的 `expiresAt`
- `planGroup` 在创建时要求显式选择，不走“跟随默认分组”语义

响应：

- 返回单个 `User` 对象，保持与现有 `GET /api/v1/admin/users/:id` / `PUT /api/v1/admin/users/:id` 一致的单实体风格

后端边界：

- 路由注册放在 `services/api/internal/app/routes.go`
- handler 放在 `services/api/internal/handlers/user.go`
- 业务逻辑放在 `services/api/internal/services/user/`
- `UserService.CreateUserByAdmin()` 负责：
  - 参数校验
  - 用户名 / 邮箱唯一性校验
  - 套餐组存在性校验
  - Emby 建号
  - 本地用户落库
  - 失败回滚

与现有注册链路的关系：

- 公开注册仍走 `AuthService.RegisterUser()`
- 后台创建不经过 `registration_mode`、邮箱验证码、邀请码、模板用户权限复制
- 但后台创建应沿用与注册一致的用户名规则、密码最小长度、Emby 建号顺序和本地 hash 写法
- 实现时可以抽最小共享 helper，避免后台创建复制一整套 Emby 建号 / 本地落库 / 失败删除 Emby 的脏逻辑；不要为这点复用引入新的大抽象层

前端边界：

- `services/web/src/api/admin.ts` 新增 `createAdminUser()`
- `services/web/src/types/api.ts` 新增 `CreateAdminUserRequest`
- `UsersView` 继续复用现有 `getPlanGroups()` 结果，不再额外依赖 `plans`
- 创建弹窗在视觉上沿用当前编辑弹窗的表单语言，避免同页出现两套用户状态语义

### 4. 关键流程

1. 管理员进入 `用户管理` 页面，点击 `新建用户`
2. 前端展示创建弹窗，管理员填写 `username/email/password`，选择 `planGroup`
3. 管理员选择：
   - 具体 `expiresAt`；或
   - 打开 `neverExpire`
4. 提交 `POST /api/v1/admin/users`
5. 后端校验用户名、邮箱、密码、套餐组和过期参数
6. 后端创建 Emby 用户
7. 后端创建本地 `users` 记录：
   - `role = user`
   - `planGroup = req.planGroup`
   - `expiresAt = req.expiresAt` 或 `nil`
   - `isActive = true`
   - `embyDisabled = false`
8. 如果本地落库失败，后端 best-effort 删除刚创建的 Emby 用户
9. 创建成功后返回用户对象，前端关闭表单并刷新列表

### 5. 失败路径与边界条件

- 用户名已存在：直接拒绝，不能先建 Emby 再说
- 邮箱已存在：直接拒绝，不能制造半成功状态
- 套餐组不存在：直接拒绝；前端下拉只是辅助，后端必须兜底
- `neverExpire=true` 且同时传了 `expiresAt`：直接拒绝
- `neverExpire=false` 但 `expiresAt` 缺失或格式错误：直接拒绝
- Emby 创建成功但本地落库失败：必须 best-effort 删除 Emby 用户，避免出现孤儿 Emby 账号
- 后台创建用户后，不应自动写 Stripe 支付记录：当前 `payments` 的语义就是第三方支付账本，不能拿来记后台开通
- 兼容性约束：
  - 不能改变公开注册流程
  - 不能改变续费中心“按有效套餐组显示方案”的规则
  - 不能因为后台创建功能，把用户状态模型扩展成“分组 + 到期时间 + 方案”三套并行真相

## 影响范围

涉及的子系统：

- API：有，涉及后台用户管理新增创建接口和建号流程
- Web：有，涉及 `UsersView` 新建入口、表单状态、套餐组下拉和过期时间输入
- Bot：无，当前不新增专门通知；也不复用“新用户自注册”通知，避免误报来源
- 配置/部署：无，不新增环境变量，不新增 SQL migration
- 文档：落地后需要同步 `docs/system-architecture.md`

## 落地验证与收口

已完成的关键收口：

- `services/api/internal/app/routes.go` 已注册 `POST /api/v1/admin/users`
- `services/api/internal/services/user/create.go` 已实现 `CreateUserByAdmin`
- `services/api/internal/services/user/create_test.go` 已覆盖后台创建用户核心场景
- `services/web/src/views/admin/UsersView.vue` 已接入新建用户弹窗和套餐组选择
- `docs/system-architecture.md` 已同步后台创建用户接口和用户状态语义

保留的历史验证清单：

- 在后台用户管理打开“新建用户”弹窗，创建一个普通用户，确认成功出现在列表中
- 创建一个绑定到非默认 `planGroup` 的用户，确认用户列表展示为该显式分组，而不是“跟随默认”
- 分别验证“指定到期时间”和“永不过期”两种创建路径
- 用新账号登录，确认续费中心看到的是该 `planGroup` 下的启用方案集合
- 人为制造本地落库失败场景时，确认 Emby 没有残留孤儿账号

## 文档处理结果

已完成：

- 新增 `POST /api/v1/admin/users`、后台创建流程、用户状态语义已补进 `docs/system-architecture.md`
- 本文档已移入 `docs/archive/plan/console-admin/`
