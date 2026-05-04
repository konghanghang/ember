# 管理员 Emby 账号绑定方案

> 状态：草稿
> 负责人：Ember
> 更新时间：2026-05-04

## 背景

这个问题为什么现在要解决：

- 管理员账号通过 `ADMIN_USERNAME` / `ADMIN_PASSWORD` 在启动期由 `seedDefaultAdmin` 种子，`EmbyID` 默认留空。
- 系统目前没有任何"管理员补绑 Emby"的接口或前端入口，搜遍 routes / handlers / services 均未发现。
- 因此管理员账号被多个 Emby 相关功能拒之门外：
  - `services/api/internal/handlers/media.go:192`：媒体页 latest items / poster 直接返回"用户未绑定 Emby 账号"。
  - `services/api/internal/services/playback/profile.go:271`：个人播放档案降级为本地 user ID，查不到真实播放数据。
  - `services/api/internal/services/redemption/code_service.go:327`：自助兑换激活码失败。
- 不解决会导致一种长期割裂状态：管理员能管别人，自己却用不了媒体相关功能。
- 曾考虑过"管理员用户名 = Emby 第一个用户名"自动绑定，但 Emby 用户顺序不稳定、且会把启动期 seed 与 Emby 服务可用性强行耦合，已确认放弃。

## 目标

本方案要实现：

1. 管理员可以在控制台自助把当前本地账号与一个真实 Emby 用户建立 1 对 1 关联。
2. 关联依据 Emby 凭据校验，不依赖用户名相同或用户列表序号这类脆弱假设。
3. 关联完成后，管理员获得与普通用户一致的 Emby 相关读权限（媒体 latest、个人播放档案、自助兑换等）。

## 非目标

本次明确不做：

- 不改管理员登录方式：仍走本地密码，绑 Emby 不影响登录链路。
- 不改普通用户"注册即绑定"链路。
- 不在启动期 `seedDefaultAdmin` 内调用 Emby。
- 不批量自动绑定 Emby 已有用户。
- 不在绑定时把 Emby 密码同步成 Ember 本地哈希。
- 不修改 `ADMIN_USERNAME` / `ADMIN_PASSWORD` 的语义与作用范围。

## 当前事实

以当前代码和现行文档为准：

- 相关文档：
  - `docs/system-architecture.md`
  - `docs/reference/web-design-guide.md`
  - `docs/reference/api-response-standard.md`
- 相关服务 / 页面 / 模型：
  - `services/api/internal/db/db.go`（`seedDefaultAdmin`）
  - `services/api/internal/services/auth/login.go`（`authenticateLoginUser`，管理员走本地密码）
  - `services/api/internal/integrations/emby/emby.go`（`AuthenticateUser`，是绑定校验复用对象）
  - `services/api/internal/models/user.go:18`（`User.EmbyID` 字段，目前是普通索引）
  - `services/api/internal/handlers/media.go`、`services/api/internal/services/playback/profile.go`、`services/api/internal/services/redemption/code_service.go`（依赖 `EmbyID` 的下游链路）
  - `services/api/internal/app/routes.go`（`registerAdminRoutes`，新接口的归属位置）
  - `services/web/src/views/console/AccountCenterView.vue`（管理员 / 普通用户共用基本信息卡，目前只只读展示 `embyId`）
  - `services/web/src/api/admin.ts`、`services/web/src/store/user.ts`（前端调用与状态刷新入口）
- 当前行为：
  - 管理员账号 `EmbyID` 默认为空，且没有任何更新入口。
  - 普通用户在 `register_persist.go:65` 由 Emby 写回 `EmbyID`，后续也无修改入口。
  - `users.emby_id` 仅是普通索引，没有唯一约束。
- 现有限制：
  - 没有任何 API 能变更 `EmbyID`。
  - 没有 DB 层防护两个本地账号绑同一 Emby 用户的兜底。

## 方案设计

### 1. 用户可见行为

- 新增能力：
  - 管理员在「账户中心」基本信息卡片下新增「关联 Emby 账号」操作。
  - 未绑定状态：`Emby ID` 字段下出现「关联 Emby 账号」按钮；点击后弹窗输入 Emby 用户名 + Emby 密码。
  - 已绑定状态：原 `embyId` 只读展示保留，下方提供「解除关联」按钮（带二次确认）。
- 修改的现有行为：
  - 管理员绑定后，`EmbyID != ""` 自然解锁 media / playback profile / redemption 等下游链路，下游条件判断不需要为管理员单独加分支。
- 必须保持不变：
  - 管理员登录链路（本地密码校验）。
  - 启动期 `seedDefaultAdmin` 纯本地行为。
  - 普通用户「注册即绑定」流程。

> 前端实现必须遵守 Ember 风格，设计与交互基线以 `docs/reference/web-design-guide.md` 为准。绑定卡片不堆解释性文案，按钮和状态文字按文案克制原则只保留必要信息。本计划不存在偏离规范的特例。

### 2. 数据与模型

- 不新增表，不新增字段。
- `users.emby_id` 唯一性收口：
  - 新增 SQL migration：`infrastructure/database/<YYYYMMDD>_NN_users-emby-id-unique.sql`。
  - 索引语义：`emby_id IS NOT NULL AND emby_id <> ''` 时唯一（PostgreSQL 偏唯一索引）。
  - 文件要求：使用 `CREATE UNIQUE INDEX IF NOT EXISTS ...`，幂等可重复执行；上线前确认现网无重复 `emby_id`，否则迁移会失败。
  - 不依赖 GORM AutoMigrate。
- GORM 模型层无字段语义变化，不做 `gorm:"unique"` 标注（避免 AutoMigrate 误覆盖现有 schema）。

### 3. 接口与边界

新增两个管理员侧接口，挂在 `admin` group 下，复用 `JWTAuth + PasswordResetRequired + AdminOnly` 中间件链：

| 方法 | 路径 | 用途 |
|---|---|---|
| `PUT` | `/api/v1/admin/current/emby-binding` | 校验 Emby 凭据并把 Emby 用户绑定到当前管理员 |
| `DELETE` | `/api/v1/admin/current/emby-binding` | 解除当前管理员的 Emby 关联 |

请求 / 响应（字段统一 camelCase，列表沿用 `data` 包装规则不适用此处单对象响应）：

- `PUT` 请求体：
  - `embyUsername: string`（必填）
  - `embyPassword: string`（必填）
- `PUT` 响应体（成功）：
  - `embyId: string`
  - `embyUsername: string`
- `DELETE` 响应体：标准成功响应。

错误语义（沿用现有 `success/error` 包装与 HTTP 状态码语义）：

- `400`：参数缺失或格式错误。
- `401`：Emby 凭据校验失败。
- `409`：
  - 该 Emby 用户已被其他本地账号占用（提示冲突方本地用户名，不暴露内部 ID）。
  - 当前账号已绑定，需先解绑再切换（首版不支持一步覆盖）。
- `502`：Emby 服务暂不可用。

调用方影响：

- 前端 `AccountCenterView.vue` 在管理员视角下增加按钮和弹窗。
- 现有依赖 `EmbyID` 的下游链路无需改动；它们仍按 `EmbyID != ""` 判断。

### 4. 关键流程

绑定：

1. 管理员在账户中心点击「关联 Emby 账号」，输入 Emby 用户名 / 密码。
2. 前端调用 `PUT /api/v1/admin/current/emby-binding`。
3. 后端 handler 取出 JWT 中的本地 userID，再次校验角色为 `admin`。
4. service 调 `embyService.AuthenticateUser(embyUsername, embyPassword)` 拿到 `embyUser.ID`。
5. 应用层先查一次 `emby_id` 占用情况：
   - 已被同一管理员持有 → 直接成功（幂等）。
   - 已被其他本地账号持有 → 返回 409。
6. 校验当前账号 `EmbyID` 是否已绑定其他值：是则返回 409，要求先解绑。
7. 写入 `EmbyID = embyUser.ID`，由 DB 唯一索引兜底并发冲突。
8. 返回 `embyId + embyUsername`，前端刷新当前用户上下文。

解绑：

1. 管理员点击「解除关联」并通过二次确认。
2. 前端调用 `DELETE /api/v1/admin/current/emby-binding`。
3. 后端清空当前管理员 `EmbyID`，不删除 Emby 真实用户，不修改 Emby 任何属性。
4. 返回成功，前端刷新当前用户上下文。

### 5. 失败路径与边界条件

- Emby 服务不可用：返回 502，不写库；后端日志记录失败原因。
- Emby 凭据错误：返回 401，不写库。
- Emby 用户已被其他本地账号绑定：返回 409，提示冲突方本地用户名。
- 并发同一 Emby 用户被两个管理员同时绑定：DB 偏唯一索引兜底；应用层捕获唯一约束错误并翻译为同 409 错误。
- 解绑后访问 media / playback：与未绑定用户一致返回"未绑定 Emby"，不允许半绑定中间态。
- 管理员账号已被禁用 / 已过期：仍允许调用绑定 / 解绑接口，因为 admin 不应被业务过期机制影响（沿用现有中间件行为）。
- 兼容性约束：
  - 不破坏普通用户注册即绑定链路。
  - 不破坏 `seedDefaultAdmin` 启动期纯本地行为。
  - 不破坏现有 `EmbyID != ""` 下游判断逻辑。
- 审计与日志：
  - 后端按 Go 日志规则在 `[Admin Emby Binding]` 前缀下记录绑定 / 解绑事件，包含本地 userID、目标 embyId、操作类型、结果。
  - 严禁输出 Emby 密码、Emby 完整返回体、Token。

## 影响范围

- API：
  - 新增 `admin` 路由两条 + 对应 handler、service 方法。
  - 新增一份 SQL migration 给 `users.emby_id` 加偏唯一索引。
  - 复用 `embyService.AuthenticateUser` 不做改动。
- Web：
  - `services/web/src/views/console/AccountCenterView.vue` 在管理员视角下新增绑定 / 解绑入口与弹窗。
  - `services/web/src/api/admin.ts` 新增两条 API 客户端方法。
  - `services/web/src/store/user.ts` 复用现有当前用户刷新入口（如不存在则补一个最小函数，不引入新模式）。
- Bot：无。
- 配置 / 部署：无。
- 文档：
  - `docs/system-architecture.md`：管理员账号相关章节补充"管理员可自助关联 Emby 账号"。
  - 落地后归档本计划至 `docs/archive/plan/console-admin/`。

## 验证方式

### 编译 / 测试

- `cd services/api && go build ./...`
- `cd services/api && go test ./...`
- `cd services/web && npm run build`

### 手工验证

- 管理员未绑定时访问媒体页 → 返回"未绑定 Emby"；绑定后再访问 → 返回真实数据。
- 管理员未绑定时访问个人播放档案 → 数据为空；绑定后 → 返回真实播放数据。
- 同一 Emby 用户尝试绑定到另一个本地账号 → 返回 409。
- 输错 Emby 密码 → 返回 401，库未变更。
- 关闭 Emby 服务后绑定 → 返回 502，库未变更。
- 解绑后再访问媒体页 → 回退到未绑定行为。
- 启动期未配置 Emby 时管理员仍可正常初始化、登录、进入控制台。
- DB 偏唯一索引迁移可重复执行不报错。

## 落地后文档处理

落地后应同步处理：

- `docs/system-architecture.md` 管理员相关章节沉淀本能力描述。
- 至少一次回归验证通过后，将本计划移入 `docs/archive/plan/console-admin/`。
- 若未来需要"绑定 / 解绑审计列表"或"管理员强制覆盖绑定"，新建独立计划，不在本方案扩展。
