# 认证与账号完整性加固方案

> 状态：P0 + P1 部分已落地（批次 1 + review 修复）；剩余 P1+/P2/P3 待后续批次
> 负责人：Ember
> 更新时间：2026-04-26

## 落地进度

批次 1（commits `333b926` + `043401a`）+ review 修复（待提交）已完成本方案的 P0 / 部分 P1 红线项：

- ✅ 邮箱验证码"校验即消费"（事务 + FOR UPDATE + DELETE）
- ✅ 忘记密码反账号枚举（双重收口）：service 层 reset 路径无论邮箱是否注册都消耗同一套 IP / email 限流配额；handler 除 SMTP 未配置外所有错误一律折叠为 200 + 统一文案，攻击者无法借状态码或限流差异（200 vs 429）枚举注册邮箱
- ✅ 删除登录链路反向覆盖 Emby 密码的副作用
- ✅ EmbyID 错配显式拒登 + ERROR 日志
- ✅ `findLoginUser` / `ensureRegisterUserUnique` / `findUserByUsername` / `findUserByEmail` / SendVerificationCode 用户存在判断统一改 `lower(...)` 比较
- ✅ Schema 层补 `lower(username)` / `lower(email)` 函数唯一索引（`20260426_02_users_lower_unique_indexes.sql`，含预检 fail-fast 与排查 SQL），DB 兜底逻辑重复账号
- ✅ IP 限流 SQL 增加 `"type" = ?` 过滤；清理 `validateVerificationRateLimits` 之前的死分支与已无调用点的 `validateVerificationRecipient`

剩余项（注册回滚补偿 / ConfigService 敏感配置回显 / InternalAuth 常数时间比较 / handler `err.Error()` 字符串匹配整改 / `CheckExpiredUsers` cancel + 失败上限）按 P1+/P2/P3 待后续批次。

## 背景

2026-04-25 系统性 review 在认证 / 用户 / 配置中心 / 邮件四个子系统集中暴露多类硬伤，整体品味评分 🔴：

- 邮箱验证码无消费机制，同一码 10 分钟内可重复用于注册和重置；忘记密码接口直接区分"邮箱已注册 / 未注册"，是教科书级账号枚举面。
- 普通用户登录链路在"本地校验通过"分支里反向把请求里的明文密码同步覆盖到 Emby，导致一次登录就能静默改密。
- 后台 / 普通注册回滚链路依赖 `_ = embyService.DeleteUser(...)` 静默吞错，失败时 Emby 端可能残留账号，下一次同名注册彻底卡死。
- 邮箱 IP 限流 SQL 不带 codeType，与 `docs/system-architecture.md §5.13` 文案不一致；`validateVerificationRateLimits` 第一段判断已成死代码。
- ConfigService 对敏感配置写入后无法回显；`email_verification` 等布尔配置未做 Normalize，`"True"` / `" true "` 静默失效。
- `InternalAuth` 启动期一次性读 env 与"配置中心可热更"心智冲突，且使用 `==` 字符串比较存在时序攻击面。
- 登录链路 `username = ?` 大小写敏感，叠加 Emby 端 case-insensitive 行为，存在跨账号串号风险；EmbyID 不一致时静默走兜底分支，无任何告警。
- `CheckExpiredUsers` 无 cancel、无并发控制、错误数组无上限。
- handler 层大量 `switch err.Error()` 字符串匹配状态码，`Logout` 是空 stub，`SystemHandler.GetSystemInfo` 用 `success+info` 包破坏统一响应规范。

如果不收口，会出现"一次登录改 Emby 密码"、"验证码可重放注册多账号"、"账号枚举"、"Emby 孤儿账号导致永久无法注册"等真实可触发的用户可见错乱。

## 目标

本方案要实现：

1. 邮箱验证码"校验即消费"，并把发送侧 IP 限流与文档约定（按 codeType 隔离）对齐
2. 忘记密码 / 重置密码接口对外行为不区分账号是否存在，杜绝枚举
3. 登录链路彻底删除"用本次请求密码反向覆盖 Emby"的副作用，并对 EmbyID 错配显式告警
4. 注册（普通 / 后台）回滚补持久化补偿，确保失败时 Emby 不留孤儿账号
5. ConfigService 敏感配置回显语义明确；布尔配置全部走 Normalize；`InternalAuth` 与 ConfigService 边界对齐，secret 比较改为常数时间
6. 登录与注册在 username / email 上统一做大小写归一，避免跨账号串号
7. handler 层错误匹配从 `err.Error()` 字符串切换到 sentinel + `errors.Is`
8. `CheckExpiredUsers` 引入 cancel context 与失败上限保护

## 非目标

本次明确不做：

- 不重写 JWT、不引入 refresh token / token blacklist（独立计划承接）
- 不调整 `User` 模型的字段语义（password / email 持久化形式不变）
- 不改 Turnstile 的接入位置或适配新的人机验证
- 不重构 `ConfigService` 的定义注册表，仅补敏感回显与布尔归一
- 不调整邮件 SMTP 接入方式，仅修计数与限流
- 不做 `services/web` 端的展示层修改（前端整改放 `console-admin/web-frontend-auth-and-design-baseline-fix.md`）

## 当前事实

以当前代码和现行文档为准，现状如下：

- 相关文档：
  - `docs/system-architecture.md` §5.1 / §5.2 / §5.5 / §5.6 / §5.13
  - `docs/reference/api-response-standard.md`
- 相关服务：
  - `services/api/internal/services/auth/login.go`、`register.go`、`register_persist.go`、`register_notify.go`、`service.go`
  - `services/api/internal/services/user/admin.go`、`profile.go`、`password.go`、`password_reset.go`、`create.go`
  - `services/api/internal/services/email/service.go`、`verification.go`、`sender.go`
  - `services/api/internal/services/system/expiry.go`
  - `services/api/internal/config/config.go`
- 相关中间件：
  - `services/api/internal/middleware/jwt.go`、`internal_auth.go`
- 相关 handler：
  - `services/api/internal/handlers/auth.go`、`user.go`、`config.go`、`setting.go`、`system.go`
- 涉及表：
  - `users`（含 `email`、`username`、`embyId`、`isActive`、`embyDisabled`、`expiresAt`）
  - `email_verifications`（按 `email + type + createdAt` 限流）
  - `settings`（运行期配置 KV 存储，敏感项加密）
- 当前行为：
  - 注册 / 重置接口调用 `EmailService.VerifyCode` 后**不删除** verification 记录
  - 忘记密码接口未注册时直接返回 `ErrEmailNotRegistered`
  - 登录路径在 `EmbyAuthErr != nil` 与 `local hash 通过` 的兜底分支里同步调 `embyService.UpdateUserPassword`
  - 注册失败时 `_ = embyService.DeleteUser(embyUser.ID)`，错误吞默
  - 邮箱限流 SQL 已按 codeType 隔离；IP 限流 SQL 不按 codeType 隔离
  - `IsEmailVerificationEnabled()` 用 `s.GetString("email_verification") == "true"` 严格匹配
  - `InternalAuth()` 在 `routes.go` 注册期一次性读 `os.Getenv("INTERNAL_API_SECRET")`，闭包持有
  - `findLoginUser` 用 `Where("username = ?")` 大小写敏感
- 现有限制：
  - 线上长期 `AUTO_MIGRATE=false`，新增字段 / 索引必须配套 SQL migration
  - JWT 7 天有效期，无主动失活机制
  - `EmailService` / `AuthService` / `UserService` 都在内部 `New*Service()` 装配，没有依赖注入入口
  - 设置中心改 `INTERNAL_API_SECRET` 在 `ConfigDefinition` 里被声明 `Editable=false`，文档说明与代码行为长期错位

## 方案设计

### 1. 用户可见行为

- 注册 / 重置成功后，对应邮箱验证码立刻失效，再次提交同一码返回"验证码无效或已过期"
- `POST /api/v1/forgot-password/send-code` 无论邮箱是否注册都返回 200（成功文案统一），不再泄漏存在性
- 普通用户登录不再出现"一次登录后 Emby 端密码被覆盖"的副作用；管理员重置 / 用户主动改密路径行为不变
- 注册过程中 Emby 创号成功但本地落库失败时，账号要么本地与 Emby 都存在，要么都不存在；不会出现"同名用户永远无法注册"的卡死
- 配置中心保存敏感项后，UI 拿到统一的"已设置 / 未设置 / 来源"状态语义，不会再出现"保存了但看不到任何变化"
- 设置中心改 `INTERNAL_API_SECRET`（如保留为只读），UI 显式说明"该项仅可通过环境变量提供"，不再让管理员误以为可以热更
- 登录用户名 / 注册邮箱在前端展示与后端比对时按统一大小写归一；老用户 username 大小写差异在 migration 后唯一保留一份
- handler 错误响应文案不变，但 HTTP 状态码与 `error code` 字段稳定

### 2. 数据与模型

- `users.email` 与 `users.username` 改为 partial unique（`WHERE email IS NOT NULL AND email <> ''`）+ 大小写不敏感（建议 `lower(email)` / `lower(username)` 上的唯一索引）
- `email_verifications` 不新增字段；但对接 `email + type` 与 `ip + type` 复合索引以支撑限流 SQL
- `settings` 不新增字段；但对 `email_verification`、`turnstile_login_enabled` 等布尔项补 `Normalize` 函数
- 新增表 `failed_emby_provisions`（仅在 P0 修复需要补偿队列时引入）：

  | 字段 | 类型 | 说明 |
  |---|---|---|
  | id | string(25) | CUID |
  | embyUserId | string(50) | 待清理的 Emby 账号 ID |
  | reason | string(255) | 失败原因（注册回滚 / Emby 创号但本地落库失败） |
  | retries | int | 重试次数 |
  | nextAttemptAt | time.Time | 下次重试时间 |
  | createdAt | time.Time | 自动 |

- 必须配套 SQL migration（baseline 之后新增顶层增量）：
  - `YYYYMMDD_NN_users_case_insensitive_unique.sql`
  - `YYYYMMDD_NN_email_verification_indexes.sql`
  - `YYYYMMDD_NN_failed_emby_provisions.sql`（如确认引入补偿表）
- 所有迁移必须幂等（`CREATE INDEX IF NOT EXISTS`、`CREATE TABLE IF NOT EXISTS`），并提供下行 rollback 注释

### 3. 接口与边界

- `POST /api/v1/forgot-password/send-code`
  - 入参不变
  - 出参：恒返回 `{ "message": "如果该邮箱已注册，验证码已发送" }`，无差别
- `POST /api/v1/forgot-password/reset`
  - 内部行为：验证码"先校验即删除（事务内）"→ Emby 改密 → 本地 hash 改 → 返回结果
  - 失败时不再用 `RowsAffected==0` 反推验证码无效
- `POST /api/v1/login`
  - 删除"本地通过 + Emby 失败时反向覆盖 Emby 密码"分支
  - EmbyID 错配（`embyUser.ID != user.EmbyID`）：记 `WARN` 日志（含 `embyUserID`、`localEmbyID`、`username`），返回与"用户名或密码错误"相同文案；不再静默走兜底
- `POST /api/v1/user/register`、`POST /api/v1/admin/users`
  - 入参不变
  - 流程包入事务 + Emby 创号在事务前置占位行 + 失败时把待清理 Emby 账号写入 `failed_emby_provisions`
- `GET /api/v1/admin/configs/:key`
  - 敏感项响应统一带 `hasValue: bool`、`source: "database"|"env"|"default"`、`maskedValue: string`（仅展示尾 4 位或固定占位）
- `PATCH /api/v1/admin/configs/:key`
  - 写空字符串到敏感项的语义改为"显式清空"，并在响应中明确说明"清空后回退到 env / default"
- `Internal API`：`InternalAuth` 不变接口，但 secret 来源改为通过 `ConfigService.ResolveString("INTERNAL_API_SECRET")` 在每次请求时读取，结合 `crypto/subtle.ConstantTimeCompare`

### 4. 关键流程

#### 4.1 邮箱验证码消费（注册 / 重置共用）

1. handler 接收请求并调 `AuthService.RegisterUser` / `UserService.ResetPasswordByCode`
2. service 开事务：
   - `SELECT ... FOR UPDATE` 锁定 `email_verifications` 该 `email + code + type` 行
   - 校验 `expiresAt > now`；不通过返回 `ErrEmailCodeInvalid`
   - 立刻 `DELETE` 该行
3. 在同一事务里完成业务写入（创建 user / 改密码）
4. commit 后再触发副作用（火忘式通知、Emby 调用）

#### 4.2 注册回滚补偿

1. service 在事务前置：
   - 锁 `lower(username)` / `lower(email)`，已存在则直接返回
   - 写一条 `users` 占位行（status=`provisioning`），CUID 生成本地 ID
2. 调用 `EmbyService.CreateEmbyUser`：
   - 成功：把 `embyId` 回填到占位行，状态置为 `active`，事务 commit
   - 失败：删除占位行后返回错误
3. 占位行写入与 Emby 创号失败的回滚路径里，调用 `EmbyService.DeleteUser`：
   - 成功：直接结束
   - 失败：写入 `failed_emby_provisions(embyUserId, reason)`，由独立 cron 重试
4. cron `cleanupFailedEmbyProvisions`（建议每 10 分钟一次）按 `nextAttemptAt` 拉取并重试，指数退避，超过 6 次记录告警

#### 4.3 登录链路收口

1. `findLoginUser(username)` 改为 `Where("lower(username) = ?", strings.ToLower(username))`
2. 查到 user 后：
   - admin：本地 bcrypt 校验
   - 普通用户：本地 hash 优先；本地为空时降级 Emby 认证（保留），认证成功后**只在本地 hash 为空时**补存 hash；不再调 `UpdateUserPassword`
3. EmbyID 错配走 4xx + 统一文案 + 结构化日志

#### 4.4 ConfigService 敏感回显

1. `applyResolvedValue` 在 `def.Sensitive` 时填充 `MaskedValue`、`HasValue`、`Source`
2. `Update` 方法在写入后用 `s.Get(key)` 返回带 mask 的 item
3. 布尔型配置统一在 `ConfigDefinition.Normalize = strings.ToLower(strings.TrimSpace(...))` 后再 validate；`IsEmailVerificationEnabled` 改用 `strings.EqualFold`

#### 4.5 限流计数对齐

1. IP 限流 SQL 加 `AND "type" = ?`，与文档"按 codeType 隔离"对齐
2. 删除 `validateVerificationRateLimits` 之前的死分支，统一通过该函数判定
3. 限流计数 + insert 放进事务（`SELECT ... FOR UPDATE` 或 `INSERT ... ON CONFLICT DO NOTHING` + 重读计数）

### 5. 失败路径与边界条件

- **验证码并发消费**：两个请求同时拿到同一码 → 事务 `FOR UPDATE` 后第二个请求看到已 DELETE，返回 `ErrEmailCodeInvalid`
- **Emby 创号成功 + 本地事务 commit 失败**：进入补偿队列；运营人员可以从 `failed_emby_provisions` 查到待清理项
- **EmbyID 错配 + 用户密码恰好与本地 hash 一致**：旧逻辑会反向覆盖 Emby；新逻辑直接拒登并 WARN
- **配置中心写入空字符串到敏感项**：明确语义为"清空 + 回退"，UI 显式提示
- **InternalAuth secret 未配置**：返回 503，错误体不再泄漏配置状态（统一文案"内部认证不可用"）
- **CheckExpiredUsers cron 中断**：context.Canceled 时立即返回，已处理批次不回滚（业务上幂等）
- **大小写归一 migration 期间出现 username 冲突**：migration 必须先 `SELECT lower(username), count(*) FROM users GROUP BY 1 HAVING count(*) > 1` 输出冲突清单 → 人工合并 → 再加唯一索引
- **`Logout` 空 stub**：本轮不引入 token blacklist，但需要明确文案为"本端清除登录态，token 在到期前仍可被服务端接受"，避免未来误读

## 影响范围

- API：
  - 修改：`auth/*.go`、`user/admin.go`、`user/password_reset.go`、`email/service.go`、`email/verification.go`、`config/config.go`、`middleware/internal_auth.go`、`system/expiry.go`、`handlers/auth.go`、`handlers/config.go`
  - 新增：`services/account/provisions.go`（如确认引入补偿队列）
- Web：
  - 设置中心需要消费新的 `hasValue / source / maskedValue` 字段（实际改动放 `console-admin/web-frontend-auth-and-design-baseline-fix.md`）
- Bot：
  - 不改 Bot 端代码，但 `INTERNAL_API_SECRET` 切到 ConfigService 读取后，Bot 启动配置文档需要同步说明
- 配置 / 部署：
  - `infrastructure/database/` 新增 3 份 SQL migration
  - 文档同步说明：`INTERNAL_API_SECRET` 来源改为运行期解析（仍只允许 env 注入）
- 文档：
  - `docs/system-architecture.md` §5.13 邮件限流章节重写"按 codeType 隔离"实现细节
  - `docs/reference/api-response-standard.md` 增补 `ConfigItem` 敏感字段语义
  - `docs/runbooks/deployment-environment.md` 增补 `failed_emby_provisions` 表说明

## 验证方式

### 编译 / 测试

- `cd services/api && go build ./...`
- `cd services/api && go test ./internal/services/auth/... ./internal/services/user/... ./internal/services/email/... ./internal/config/... ./internal/middleware/...`
- `cd services/api && go test ./internal/handlers/... -run "TestAuth|TestUser|TestConfig"`

### 手工验证

#### 邮箱验证码消费
- 申请一条验证码 → 用同一码尝试两次注册：第二次必须返回 `ErrEmailCodeInvalid`
- 申请一条 `register` 码 → 拿来调 `reset` 接口：必须拒绝（type 不匹配）

#### 账号枚举
- `POST /api/v1/forgot-password/send-code` 用未注册邮箱 → 返回 200 + 与已注册邮箱一致的成功文案

#### 登录链路
- 普通用户本地 hash 已存在，临时停掉 Emby → 登录成功，Emby 端密码不变
- 在 Emby 后台手动重建用户（embyId 改变）→ 登录拒绝，日志出现 EmbyID 错配 WARN

#### 注册回滚
- mock Emby 创号成功后人为让本地落库失败 → 重启服务，cron 触发 → `failed_emby_provisions` 被清空，Emby 端账号被删

#### 配置中心
- 用 `True` / ` true ` / `1` 写入 `email_verification` → 全部成功并按布尔生效
- 保存敏感配置 `EMBY_API_KEY` → UI 看到 `hasValue=true / source=database / maskedValue=****1234`
- 删除 `EMBY_API_KEY` 数据库值 → UI 显示 `source=env` / 回退后能继续工作

#### InternalAuth
- 通过设置中心改 `INTERNAL_API_SECRET`（确认拒绝写入并提示"只读"）
- env 改新 secret 后无需重启 → Bot 调用立即用新 secret 通过

#### 大小写归一
- 历史库存在 `Tom` 和 `tom` 两条 → migration 报告冲突 → 人工合并后 migration 才允许通过
- 注册 `Alice@example.com` → 再注册 `alice@example.com` → 第二次拒绝

### 修复后验证清单

- [ ] `go build ./...` 与 `go test ./internal/services/...` 全绿
- [ ] 所有新增 SQL migration 在临时库重灌通过
- [ ] `failed_emby_provisions` cron 在测试环境跑一轮空表无报错
- [ ] 设置中心 UI 能正确展示敏感项 `hasValue / source / maskedValue`
- [ ] `INTERNAL_API_SECRET` 改 env 后 Bot 调用 internal API 不重启即可生效
- [ ] 关键日志含 `userId` / `username` / `embyUserId` / `requestId`，且不出现明文 `password` / `code`
- [ ] `Logout` handler 文档明确说明"仅清本端"

### 二次暴露检查清单

修复后必须 sweep 以下同类点，避免边修边漏：

- [ ] 所有调 `embyService.*` 后 `_ = err` 的位置（注册、密码重置、过期 cron、订阅审批），统一改为日志 + 必要时回滚
- [ ] 所有 `Where("xxx = ?", username)` / `Where("email = ?", email)` 全部 sweep，确认是否需要 `lower(...)` 归一
- [ ] 所有 `errors.New("中文")` + handler `switch err.Error()` 改为 sentinel + `errors.Is`（含 `services/redemption`、`services/payment`、`services/subscription`）
- [ ] 所有 `s.GetString(key) == "true"` / `== "false"` 字面比较改 `EqualFold`
- [ ] handler 层禁止把 raw `err.Error()` 透传给 `c.JSON`，统一过 `internalError(c, err)`
- [ ] 任何在事务中调外部 IO 的位置（`payment/service.go:840`、`redemption/service.go:65` 同类问题在 `billing-redemption` 计划中处理）

## 落地后文档处理

- 落地后把"邮箱验证码消费契约"、"注册补偿队列"、"InternalAuth 运行期读取"提炼到 `docs/system-architecture.md` §5
- 把"配置中心敏感项展示语义"提炼到 `docs/reference/api-response-standard.md`
- 本方案在 P0+P1 全部完成、回归测试通过后移入 `docs/archive/plan/access-auth/`
- P2 / P3 在本方案落地过程中如未顺手收口，单独立小型计划或纳入下一轮治理 backlog

## 附录：问题清单与本方案条目映射

| review 编号 | 问题概述 | 本方案条目 |
|---|---|---|
| P0-1 | 邮箱验证码无消费 | §4.1 |
| P0-2 | 忘记密码账号枚举 | §4 / §3 |
| P0-3 | 登录反向覆盖 Emby | §4.3 |
| P0-4 | 注册回滚不可靠 | §4.2 + §2 表 `failed_emby_provisions` |
| P0-5 | IP 限流不按 codeType 隔离 | §4.5 |
| P1-1 | `ResetPasswordByCode` 用 `RowsAffected==0` 反推 | §4.1 |
| P1-2 | 注册并发 race | §4.2 |
| P1-3 | 敏感配置写后 Get 不回显 | §4.4 / §3 |
| P1-4 | 布尔配置无 Normalize | §4.4 |
| P1-5 | `InternalAuth` 启动期一次性读 env | §3 / §4 |
| P1-6 | 登录 username 大小写敏感 | §2 / §4.3 |
| P1-7 | EmbyID 错配静默走兜底 | §4.3 |
| P2-1 | 注册 email 大小写归一 | §2 |
| P2-2 | `UpdateProfile` / `UpdateEmail` 无事务 | §4 + 二次暴露清单 |
| P2-3 | `CheckExpiredUsers` 无 cancel | 目标 §8 + 二次暴露清单 |
| P2-4 | 限流 TOCTOU | §4.5 |
| P2-5 | `setDefaults()` 隐式 lazy init | 二次暴露清单（依赖注入改造） |
| P2-6 | `IsActive=false` 也允许登录 | §4.3 + 文档对齐 |
| P3-1 | handler `switch err.Error()` | 二次暴露清单 |
| P3-2 | `Logout` stub | §5（文档说明） |
| P3-3 | `SystemHandler` 响应不一致 | 二次暴露清单 |
| P3-4 | 邮箱写日志 | §4 + 二次暴露清单 |
| P3-5 | 服务装配缺 DI | 二次暴露清单（后续独立计划） |
