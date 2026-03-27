# user-email-auth 职责切分方案

> 状态：已完成
> 负责人：Ember
> 更新时间：2026-03-27

## 背景

这个问题为什么当时要解决：

- `UserService` 原先把管理员用户管理、用户资料、自助改密、管理员重置密码、邮箱验证码重置密码、Emby 状态同步全部堆在根 `services`，职责明显失控。
- `EmailService` 原先把 SMTP 配置读取、验证码生成与频控、邮件发送、验证码校验、连接探活混在一起。
- `AuthService` 原先既负责登录/注册编排，也承担了过多基础设施细节。
- 如果不处理这三块，目录重构就会卡在“结构看起来清楚，但核心领域仍是大文件”的阶段。

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

以当前代码和现行文档为准，写清完成后的现状：

- 相关文档：
  - `docs/reference/api-development-conventions.md`
  - `docs/proposals/api-directory-refactor.md`
  - `docs/system-architecture.md`
- 相关服务/文件：
  - `services/api/internal/services/user/service.go`
  - `services/api/internal/services/user/admin.go`
  - `services/api/internal/services/user/profile.go`
  - `services/api/internal/services/user/password.go`
  - `services/api/internal/services/user/password_reset.go`
  - `services/api/internal/services/email/service.go`
  - `services/api/internal/services/email/verification.go`
  - `services/api/internal/services/email/sender.go`
  - `services/api/internal/services/auth.go`
  - `services/api/internal/services/auth_login.go`
  - `services/api/internal/services/auth_register.go`
  - `services/api/internal/services/auth_register_persist.go`
  - `services/api/internal/services/auth_register_notify.go`
  - `services/api/internal/handlers/user.go`
  - `services/api/internal/handlers/auth.go`
- 当前行为：
  - `AuthService` 负责登录、注册编排；`GetCurrentUser` 已回收到 `UserService`。
  - `UserService` 已收口到 `services/user/`，负责管理员用户管理、个人资料、密码与邮箱修改、通过邮箱验证码重置密码、Emby 状态同步。
  - `EmailService` 已收口到 `services/email/`，负责验证码发送/校验/清理和 SMTP 连接测试。
- 已完成项：
  - `email` 已正式目录化到 `services/email/`
  - `user` 已正式目录化到 `services/user/`
  - `auth` 已拆成登录、注册、注册持久化、注册通知等多文件
  - `ResetPasswordByCode` 已切开对 `EmailService` 的内部实例化依赖
  - `GetCurrentUser` 已从 `AuthService` 回收到 `UserService`
  - `AuthService` 与 `UserService` 已补关键服务测试
  - `EmailService` 已补 SMTP 配置判断、收件人校验、频控判断测试
  - `go test ./...` 与 `go build ./...` 已通过
  - 相关架构和目录文档已同步到当前代码现状

## 方案设计

### 1. 用户可见行为

- 登录、注册、邮箱验证码发送、密码重置、用户管理、个人资料页行为保持不变。
- 当前已有错误提示和状态码语义保持不变。
- 本次切分只调整服务边界，不改变任何前端调用方式。

### 2. 数据与模型

> 本次不涉及数据模型变更。

### 3. 接口与边界

最终落地形态：

```text
services/api/internal/services/
  auth.go
  auth_login.go
  auth_register.go
  auth_register_persist.go
  auth_register_notify.go
  email/
    service.go
    sender.go
    verification.go
    errors.go
  user/
    service.go
    admin.go
    profile.go
    password.go
    password_reset.go
```

边界约束：

- `auth` 作为编排层保留在根 `services`，不再为了目录整齐单独目录化。
- `user` 只承载用户域逻辑。
- `email` 负责邮件与验证码能力，但不承担注册/登录编排。
- `GetCurrentUser` 已回收到 `user`。

### 4. 关键流程

按顺序完成了以下主链路：

1. 冻结 `auth`、`user`、`email` 边界。
2. 拆出 `user` 的资料、密码、密码重置职责。
3. 切开 `ResetPasswordByCode` 对 `EmailService` 的硬依赖。
4. 将 `email` 分层并目录化。
5. 将 `auth` 拆成登录、注册、注册持久化、注册通知等多文件。
6. 将 `GetCurrentUser` 从 `auth` 回收到 `user`。
7. 补关键服务测试并完成全量编译验证。

### 5. 失败路径与边界条件

- 若直接机械搬目录而不先拆职责：会把耦合原样复制到新目录。
- 若 `AuthService` 继续直接吞验证码持久化和 SMTP 细节：编排层会继续变胖。
- 若 `UserService` 再次在方法内部直接实例化 `EmailService`：会重新制造不可测硬耦合。
- 兼容性约束：登录校验顺序、注册邀请码逻辑、邮箱验证码频控、密码重置语义都保持不变。

## 影响范围

涉及的子系统：

- API：有，主要是服务边界、handler 依赖和内部装配方式。
- Web：无直接接口变更。
- Bot：无直接影响。
- 配置/部署：无新增配置项。
- 文档：已同步 `docs/system-architecture.md`、`docs/reference/api-development-conventions.md`、`docs/proposals/api-directory-refactor.md`

## 验证方式

### 编译/测试

- `cd services/api && go test ./...`
- `cd services/api && go build ./...`

### 手工验证

- 登录 admin 和普通用户，确认登录逻辑与本地密码兜底逻辑不变。
- 邀请注册与开放注册各走一遍，确认验证码、邀请码和 Emby 用户创建行为不变。
- 通过邮箱验证码重置密码，确认验证码校验、Emby 密码同步、本地密码保存和验证码删除都正常。
- 用户中心修改密码、修改邮箱，管理员后台修改用户状态/到期时间/重置密码都正常。

当前收尾判断：

- 当前接受自动化测试覆盖作为收尾条件。
- 本方案已满足归档条件。

## 落地后文档处理

已同步处理：

- `docs/system-architecture.md`
- `docs/reference/api-development-conventions.md`
- `docs/proposals/api-directory-refactor.md`

本方案现已移入 `docs/archive/`。
