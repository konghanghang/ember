# API 目录重构后续计划

> 本文档用于记录 `services/api` 目录重构的当前状态、剩余范围和后续推进顺序，避免后续重构失控或重复判断。

---

## 1. 当前目标

目录重构的目标不是“为了好看搬文件”，而是解决三个真实问题：

1. `internal/services` 过去是一个大杂烩，业务服务、配置系统、第三方集成、错误定义都混在一起
2. 文件体积过大，尤其是 `config`、`tv_calendar`、`emby`、`payment`、`user`
3. 边界不清，导致后续继续拆分时很容易出现循环依赖

当前已经完成第一阶段与第二阶段的大部分工作，但还没有彻底收口。

---

## 2. 已完成部分

### 2.1 已拆出的顶层职责目录

当前 `services/api/internal` 已拆出：

- `app/`：服务启动装配（路由、handler、cron）
- `config/`：运行期配置定义、解析、校验、导入
- `integrations/`：第三方系统集成

其中：

- `integrations/emby/`
- `integrations/moviepilot/`
- `integrations/notifier/`

已经完成拆分并稳定工作。

### 2.2 已按业务子目录拆分的服务

当前已经完成子目录分组并通过测试的服务：

- `services/auth/`
- `services/device/`
- `services/email/`
- `services/media/`
- `services/payment/`
- `services/playback/`
- `services/redemption/`
- `services/subscription/`
- `services/telegram/`
- `services/tvcalendar/`
- `services/user/`

### 2.3 已按领域拆分的错误定义

原有统一的 `services/errors.go` 已拆掉，当前改为按业务分布在：

- `services/email/errors.go`
- `services/media_quality_errors.go`
- `services/payment/errors.go`
- `services/redemption/errors.go`
- `services/subscription/errors.go`
- `services/telegram/errors.go`
- `services/device/errors.go`
- `services/tvcalendar/errors.go`
- `services/playback/errors.go`

---

## 3. 当前残留结构

截至目前，`internal/services` 中仍保留在根层、尚未进一步目录化或不准备继续目录化的主要文件有：

- `system.go`

其中：

- `system.go` 才是当前真正还没处理的根层业务文件。

---

## 4. 剩余模块的判断

### 4.1 `telegram`

状态：

- 已完成目录拆分
- 已补服务边界收口

结论：

- 当前可视为已进入稳定状态
- 后续不优先继续动

### 4.2 `user`

状态：

- 已收口到 `services/user/`
- 管理、资料、密码、密码重置、Emby 同步已拆成独立文件
- 关键密码链路测试已补齐

问题：

- 仍然直接依赖 `email` 与 `emby`，但边界已经比原先清楚
- 是否继续抽 `errors.go` / `types.go`，现在属于可选收口，不再阻塞整体目录重构

结论：

- 当前目录化已完成
- 后续如继续处理，只是局部整理，不再属于目录重构主线

建议最终形态：

```text
internal/services/user/
  service.go
  admin.go
  profile.go
  password.go
  password_reset.go
```

详细落地方案见：

- `docs/archive/plan/user-email-auth-boundary-refactor.md`

### 4.3 `auth`

状态：

- 已收口到 `services/auth/`
- 已拆成 `service.go`、`login.go`、`register.go`、`register_persist.go`、`register_notify.go`
- `GetCurrentUser` 已回收到 `user`
- 当前职责仍然是“编排层”

结论：

- 当前目录化已完成
- 后续如继续处理，重点只剩是否进一步削减编排步骤，而不是继续调整目录结构

### 4.4 `email`

状态：

- 已收口到 `services/email/`
- 配置读取、验证码业务、SMTP 发送已分层，但对外仍保持 `EmailService` 入口
- 关键频控与配置判断测试已补齐

结论：

- 当前目录化已完成
- 后续如继续处理，也只是局部能力增强，不再属于目录重构主线

详细落地方案见：

- `docs/archive/plan/user-email-auth-boundary-refactor.md`

### 4.5 `redemption` / `redemption_code`

状态：

- 已收口到 `services/redemption/`
- 错误、类型、实现已一起迁入子目录

结论：

- 当前目录化已完成
- 后续只需在去 `compat` 阶段继续清理调用面

最终形态：

```text
internal/services/redemption/
  service.go
  code_service.go
  errors.go
  types.go
```

详细落地方案见：

- `docs/archive/plan/redemption-directory-refactor.md`

### 4.6 `system`

状态：

- 仍为根层单文件
- 主要承载系统统计 + 过期检查

结论：

- 是当前目录重构主线里最后一个还没被明确处理的根层业务文件
- 是否要拆，取决于后续是否还值得继续投入目录治理成本

---

## 5. compat 文件清理计划

当前 compat 已清理完成。

这些文件的职责只有一个：

- 让旧调用方继续通过 `services.*` 访问新子目录实现

### 5.1 最终目标

**所有 compat 文件最终都应删除。**

这件事必须明确写进计划里，否则目录永远只是“看起来重构过”，实际上调用面还是旧的。

### 5.2 删除 compat 的前提

删除某个 `*_compat.go` 前，必须先完成：

1. 全部调用方改为直接依赖新子包
2. handler 中的错误判断改为引用新子包错误
3. `app/` 中的构造逻辑改为直接引用新子包
4. `go test ./...` 通过

### 5.3 推荐删除顺序

按风险从低到高：

> 当前 compat 已全部清理完成。

---

## 6. 推荐的后续顺序

如果继续推进，建议按这个顺序：

1. 判断是否处理 `system.go`
2. 若继续治理，再决定 `user` 是否补 `types/errors`
3. `auth` 保持根层编排入口，默认不再目录化

---

## 7. 当前建议

最稳的下一步不是继续大搬家，而是：

1. 接受当前目录重构主体已经完成
2. 只有在 `system.go` 形成真实维护痛点时再继续动

原因很简单：

- 目录结构现在已经足够清楚
- compat 已清理完
- `email`、`user`、`redemption` 都已完成目录化
- `auth` 已证明更适合作为根层编排入口，而不是继续追求目录统一
