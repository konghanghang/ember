# user-email-auth 职责切分方案

> 状态：草稿
> 负责人：Ember
> 更新时间：2026-03-26

## 背景

这个问题为什么现在要解决：

- `services/api/internal/services/user.go` 同时承担管理员用户管理、用户资料、自助改密、管理员重置密码、邮箱验证码重置密码、Emby 状态同步，职责已经明显失控。
- `EmailService` 原先把 SMTP 配置读取、验证码生成与频控、邮件发送、验证码校验、连接探活混在一起；虽然现在已拆到 `services/email/`，但 `auth/user` 的调用边界仍需要继续收薄。
- `services/api/internal/services/auth.go` 作为编排层，本应保持薄，但当前仍直接依赖 `EmailService`、Emby、Notifier、兑换码校验和模板用户策略，边界过于粗糙。
- 兼容层已经清理完成，后续再不处理这三块，目录重构就会卡在“结构看起来清楚，但核心领域仍是大文件”的阶段。

## 目标

本方案要实现：

1. 明确 `user`、`email`、`auth` 三者的职责边界，避免继续互相吞逻辑。
2. 在不改变现有 API 和用户可见行为的前提下，为后续代码切分和目录化提供稳定迁移顺序。
3. 先解决边界问题，再决定哪些能力适合目录化，哪些继续保留为编排层。

## 非目标

本次明确不做：

- 不修改登录、注册、邮箱验证码、密码重置、用户管理的现有 HTTP API 路由和字段。
- 不修改数据库 schema、配置项名字、验证码规则、Emby 同步规则。
- 不顺手重构 `redemption`、`telegram`、`subscription` 等已经相对收口的领域。

## 当前事实

以当前代码和现行文档为准，先写清现状：

- 相关文档：
  - `docs/reference/api-development-conventions.md`
  - `docs/proposals/api-directory-refactor.md`
  - `docs/system-architecture.md`
- 相关服务/文件：
  - `services/api/internal/services/user.go`
  - `services/api/internal/services/email/service.go`
  - `services/api/internal/services/email/verification.go`
  - `services/api/internal/services/email/sender.go`
  - `services/api/internal/services/auth.go`
  - `services/api/internal/services/auth_login.go`
  - `services/api/internal/services/auth_register.go`
  - `services/api/internal/handlers/user.go`
  - `services/api/internal/handlers/auth.go`
- 当前行为：
  - `AuthService` 负责登录、注册、当前用户查询。
  - `UserService` 负责管理员用户管理、个人资料、密码与邮箱修改、通过邮箱验证码重置密码、Emby 状态同步。
  - `EmailService` 已收口到 `services/email/`，负责验证码发送/校验/清理和 SMTP 连接测试。
- 现有限制：
  - `UserService.ResetPasswordByCode()` 虽然已经改成显式依赖验证码校验能力，但 `UserService` 仍未正式目录化。
  - `AuthHandler` 同时持有 `AuthService` 和 `EmailService`，而 `AuthService` 自己又持有 `EmailService`。
  - `AuthService.RegisterUser()` 虽然已拆到独立文件，但注册链路仍直接持有较多基础设施依赖。
  - `AuthService.Login()` 虽然已拆到独立文件，但 `auth` 仍停留在根 `services`，尚未决定是否保持编排层形态。

## 方案设计

### 1. 用户可见行为

- 登录、注册、邮箱验证码发送、密码重置、用户管理、个人资料页行为保持不变。
- 当前已有错误提示和状态码语义保持不变。
- 本次切分只调整服务边界，不改变任何前端调用方式。

### 2. 数据与模型

> 本次不涉及数据模型变更。

### 3. 接口与边界

边界原则：

- `auth` 是编排层，不承担用户资料、验证码持久化、SMTP 细节。
- `user` 只承载用户域逻辑，不直接承担邮箱验证码基础设施。
- `email` 负责邮件与验证码能力，但不直接承担注册/登录编排。

建议的目标形态：

```text
services/api/internal/services/
  auth.go                    # 暂时保留编排入口，负责 current-user 与共享装配
  auth_login.go              # 登录链路编排
  auth_register.go           # 注册链路编排
  email/
    service.go               # 对外 facade
    sender.go                # 纯邮件发送能力
    verification.go          # 验证码生成/发送/校验/清理
  user/
    admin.go                 # 管理员用户 CRUD、启停、续期、删除
    profile.go               # 获取资料、更新资料、更新邮箱
    password.go              # 自助改密、管理员重置密码
    password_reset.go        # 通过邮箱验证码重置密码
    emby_sync.go             # Emby 状态与密码同步
    types.go                 # 请求/响应结构
    errors.go                # 用户领域错误
```

补充约束：

- `AuthService` 不直接 new `EmailService` 的具体实现，而是依赖邮件验证码能力接口。
- `UserService.ResetPasswordByCode` 不再在函数内部 new `EmailService`。
- `EmailService` 对外暴露稳定 facade，但内部拆基础 SMTP 与验证码逻辑。
- `auth` 暂时不做目录化，仍作为编排层保留在根 `services`，直到 `user/email` 边界稳定。

### 4. 关键流程

按顺序写清主链路，不需要贴代码：

1. 先在方案层面冻结职责边界，明确哪些逻辑属于 `auth`、`user`、`email`。
2. 优先切 `user` 内部职责，把管理员用户管理、资料修改、密码能力、Emby 同步拆成独立文件或子包职责。
3. 把 `ResetPasswordByCode` 从“用户服务 + 内部 new EmailService”改为“用户密码重置服务 + 显式依赖验证码校验能力”。
4. 再切 `email` 内部职责，把 SMTP 发送、配置读取、验证码业务分层，并正式目录化到 `services/email/`。
5. 最后收薄 `AuthService`，让登录和注册只保留编排，不直接持有过多基础设施细节。

### 5. 失败路径与边界条件

- 若直接把 `user.go`、`email/`、`auth.go` 机械搬目录，不先拆职责：会把现有耦合原样复制到新目录，问题只会换路径，不会消失。
- 若 `AuthService` 继续直接操作验证码持久化和 SMTP 细节：编排层会继续变胖，目录再漂亮也没有意义。
- 若 `UserService` 再次回到方法内部直接实例化 `EmailService`：后续很难测试，也很难抽离密码重置能力。
- 兼容性约束：登录校验顺序、注册邀请码逻辑、邮箱验证码频控、密码重置语义都不能改变。

## 影响范围

涉及的子系统：

- API：有，主要是服务边界、handler 依赖和内部装配方式。
- Web：无直接接口变更，但后续若出现错误文案调整，需要回归验证用户相关页面。
- Bot：无直接影响。
- 配置/部署：无新增配置项。
- 文档：需同步更新 `docs/system-architecture.md` 和 `docs/reference/api-development-conventions.md`。

## 验证方式

### 编译/测试

- `cd services/api && go test ./...`
- `cd services/api && go build ./...`

### 手工验证

- 登录 admin 和普通用户，确认登录逻辑与本地密码兜底逻辑不变。
- 邀请注册与开放注册各走一遍，确认验证码、邀请码和 Emby 用户创建行为不变。
- 通过邮箱验证码重置密码，确认验证码校验、Emby 密码同步、本地密码保存和验证码删除都正常。
- 用户中心修改密码、修改邮箱，管理员后台修改用户状态/到期时间/重置密码都正常。

## 落地后文档处理

落地后应同步处理：

- 将稳定后的 `user`、`email`、`auth` 边界更新到 `docs/system-architecture.md`。
- 将“编排层不得直接吞基础设施细节”和“先切职责再目录化”的经验补到 `docs/reference/api-development-conventions.md`。
- 稳定后将本方案移入 `docs/archive/`。
