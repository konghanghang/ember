# 管理员 Emby 账号绑定方案

> 状态：已归档
> 负责人：Ember
> 更新时间：2026-05-17
>
> 归档说明：管理员 Emby 账号自助绑定已进入 `v1.5.1`，API、前端、测试、系统架构和 API 端点目录均已同步；真实环境已完成候选查询、绑定和解绑回归。本方案只保留历史设计与决策追溯价值。

## 背景

这个问题为什么现在要解决：

- 管理员账号通过 `ADMIN_USERNAME` / `ADMIN_PASSWORD` 在启动期由 `seedDefaultAdmin` 种子，`EmbyID` 默认留空。
- 系统已实现管理员补绑 Emby 的接口与前端入口，但当前方案要求管理员输入 Emby 用户名和密码完成校验。
- 因此管理员账号被多个 Emby 相关功能拒之门外：
  - `services/api/internal/handlers/media.go:192`：媒体页 latest items / poster 直接返回"用户未绑定 Emby 账号"。
  - `services/api/internal/services/playback/profile.go:271`：个人播放档案降级为本地 user ID，查不到真实播放数据。
  - `services/api/internal/services/redemption/code_service.go:327`：自助兑换激活码失败。
- 不解决会导致一种长期割裂状态：管理员能管别人，自己却用不了媒体相关功能。
- 曾考虑过"管理员用户名 = Emby 第一个用户名"自动绑定，但 Emby 用户顺序不稳定、且会把启动期 seed 与 Emby 服务可用性强行耦合，已确认放弃。
- 2026-05-17 GitHub Issue #1 暴露出当前密码绑定流的状态码问题：输错 Emby 凭据时后端返回 401，前端全局拦截器将其当作 Ember 登录态失效并退出登录。这个问题表面是 HTTP 状态码，根因是"外部系统凭据校验失败"与"当前 Ember 会话失效"语义混在一起。
- 更深层的问题是：管理员已经持有 Ember 管理权限，系统也已配置 Emby API Key，不应再要求管理员把某个 Emby 用户的明文密码交给 Ember 后端。

## 目标

本方案要实现：

1. 管理员可以在控制台自助把当前本地账号与一个真实 Emby 用户建立 1 对 1 关联。
2. 关联依据管理员选择的 Emby 用户 ID，并由后端通过已配置的 Emby API Key 校验该用户存在。
3. 前端提供 Emby 用户列表选择，不要求输入 Emby 密码，也不依赖用户名相同或用户列表序号这类脆弱假设。
4. 关联完成后，管理员获得与普通用户一致的 Emby 相关读权限（媒体 latest、个人播放档案、自助兑换等）。
5. 修正当前错误语义：外部 Emby 绑定失败不得触发 Ember 登录态清理。

## 非目标

本次明确不做：

- 不改管理员登录方式：仍走本地密码，绑 Emby 不影响登录链路。
- 不改普通用户"注册即绑定"链路。
- 不在启动期 `seedDefaultAdmin` 内调用 Emby。
- 不批量自动绑定 Emby 已有用户。
- 不在绑定时把 Emby 密码同步成 Ember 本地哈希。
- 不要求管理员输入、提交或存储 Emby 明文密码。
- 不开放普通用户自助选择任意 Emby 用户绑定；普通用户如果需要后续绑定能力，必须另行设计所有权证明。
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
  - `services/api/internal/integrations/emby/emby.go`（`GetUsers` 已可通过 Emby API Key 获取用户列表；`GetUserByID` 可校验单个 Emby 用户存在）
  - `services/api/internal/models/user.go:18`（`User.EmbyID` 字段，GORM 模型不声明唯一约束）
  - `infrastructure/database/20260504_00_users-emby-id-unique.sql`（`users.emby_id` 非空偏唯一索引）
  - `services/api/internal/handlers/media.go`、`services/api/internal/services/playback/profile.go`、`services/api/internal/services/redemption/code_service.go`（依赖 `EmbyID` 的下游链路）
  - `services/api/internal/app/routes.go`（`registerAdminRoutes`，新接口的归属位置）
  - `services/web/src/views/console/AccountCenterView.vue`（管理员 / 普通用户共用基本信息卡，目前只只读展示 `embyId`）
  - `services/web/src/api/admin.ts`、`services/web/src/store/user.ts`（前端调用与状态刷新入口）
- 当前行为：
  - 管理员账号 `EmbyID` 默认为空。
  - 当前已有 `GET /api/v1/admin/emby-users`、`PUT /api/v1/admin/current/emby-binding` 与 `DELETE /api/v1/admin/current/emby-binding`。
  - 当前 `PUT` 使用 `embyId`，不再接收 `embyUsername + embyPassword`。
  - 旧版请求体缺少 `embyId` 时返回 400，不再触发前端退出登录。
  - 普通用户在 `register_persist.go:65` 由 Emby 写回 `EmbyID`，后续也无修改入口。
  - `users.emby_id` 已有非空偏唯一索引 migration，后续应继续保留该 DB 兜底；目标环境未执行 migration 时，仍由现有迁移机制负责应用。
- 现有限制：
  - 普通用户仍不能自助选择任意 Emby 用户绑定。
  - 首版不支持一步覆盖绑定；管理员如需更换 Emby 用户，仍需先解除关联。

## 方案设计

### 1. 用户可见行为

- 新增能力：
  - 管理员在「账户中心」基本信息卡片下新增「关联 Emby 账号」操作。
  - 未绑定状态：`Emby ID` 字段下出现「关联 Emby 账号」按钮；点击后弹窗提供 Emby 用户搜索框，输入至少 2 个字符后加载有限候选并选择目标用户。
  - 候选列表展示 Emby 用户名、短 ID 与绑定状态；已被其他本地账号绑定的 Emby 用户禁用选择，并显示冲突方本地用户名。
  - 已绑定状态：原 `embyId` 只读展示保留，下方提供「解除关联」按钮（带二次确认）。
- 修改的现有行为：
  - 管理员绑定后，`EmbyID != ""` 自然解锁 media / playback profile / redemption 等下游链路，下游条件判断不需要为管理员单独加分支。
  - 输错 Emby 凭据导致退出登录的问题自然退场，因为绑定流不再提交 Emby 密码，也不再把外部认证失败映射为 401。
- 必须保持不变：
  - 管理员登录链路（本地密码校验）。
  - 启动期 `seedDefaultAdmin` 纯本地行为。
  - 普通用户「注册即绑定」流程。

> 前端实现必须遵守 Ember 风格，设计与交互基线以 `docs/reference/web-design-guide.md` 为准。绑定卡片不堆解释性文案，按钮和状态文字按文案克制原则只保留必要信息。本计划不存在偏离规范的特例。

### 2. 数据与模型

- 不新增表，不新增字段。
- `users.emby_id` 唯一性收口：
  - 复用现有 SQL migration：`infrastructure/database/20260504_00_users-emby-id-unique.sql`。
  - 索引语义：`emby_id IS NOT NULL AND emby_id <> ''` 时唯一（PostgreSQL 偏唯一索引）。
  - 文件要求：保持 `CREATE UNIQUE INDEX IF NOT EXISTS ...` 幂等可重复执行；上线前确认现网无重复 `emby_id`，否则迁移会失败。
  - 不依赖 GORM AutoMigrate。
- GORM 模型层无字段语义变化，不做 `gorm:"unique"` 标注（避免 AutoMigrate 误覆盖现有 schema）。

### 3. 接口与边界

新增 / 调整三个管理员侧接口，挂在 `admin` group 下，复用 `JWTAuth + PasswordResetRequired + AdminOnly` 中间件链：

| 方法 | 路径 | 用途 |
|---|---|---|
| `GET` | `/api/v1/admin/emby-users` | 返回可供管理员选择绑定的 Emby 用户列表 |
| `PUT` | `/api/v1/admin/current/emby-binding` | 按 Emby 用户 ID 绑定到当前管理员 |
| `DELETE` | `/api/v1/admin/current/emby-binding` | 解除当前管理员的 Emby 关联 |

请求 / 响应（字段统一 camelCase）：

- `GET /admin/emby-users` 响应体：
  - `data: AdminEmbyUserOption[]`
- `GET /admin/emby-users` 查询参数：
  - `query: string`（必填，至少 2 个字符，按 Emby 用户名或 ID 搜索）
  - `limit: number`（可选，默认 20，最大 50）
- `AdminEmbyUserOption`：
  - `embyId: string`
  - `name: string`
  - `hasPassword: boolean`
  - `boundUsername?: string`
  - `boundToCurrent: boolean`
  - `available: boolean`

- `PUT` 请求体：
  - `embyId: string`（必填）
- `PUT` 响应体（成功）：
  - `embyId: string`
  - `embyUsername: string`
- `DELETE` 响应体：标准成功响应。

错误语义（沿用现有 `success/error` 包装与 HTTP 状态码语义）：

- `400`：参数缺失或格式错误。
- `401`：仅表示当前 Ember JWT 无效、过期或账号状态不允许继续使用；不得用于 Emby 用户选择 / 绑定业务失败。
- `404`：目标 Emby 用户不存在或已被删除。
- `409`：
  - 该 Emby 用户已被其他本地账号占用（提示冲突方本地用户名，不暴露内部 ID）。
  - 当前账号已绑定，需先解绑再切换（首版不支持一步覆盖）。
- `502`：Emby 服务暂不可用。

调用方影响：

- 前端 `AccountCenterView.vue` 在管理员视角下增加按钮和弹窗。
- 前端 `admin.ts` 新增 Emby 用户列表 API，并把绑定请求类型改为 `embyId`。
- 现有依赖 `EmbyID` 的下游链路无需改动；它们仍按 `EmbyID != ""` 判断。

### 4. 关键流程

绑定：

1. 管理员在账户中心点击「关联 Emby 账号」。
2. 前端只打开搜索弹窗，不自动加载全部 Emby 用户。
3. 管理员输入至少 2 个字符后，前端调用 `GET /api/v1/admin/emby-users?query=...&limit=20` 拉取有限候选。
4. 后端通过 `embyService.GetUsers()` 获取 Emby 用户列表，在服务端按用户名 / ID 过滤并截断到 limit，再批量查询本地 `users.emby_id` 占用情况。
5. 前端展示可选用户；已被其他本地账号占用的用户不可选，已绑定当前管理员的用户标记为当前绑定。
6. 管理员选择目标 Emby 用户后，前端调用 `PUT /api/v1/admin/current/emby-binding`，请求体只提交 `embyId`。
7. 后端 handler 取出 JWT 中的本地 userID，再次校验角色为 `admin`。
8. service 使用 `embyService.GetUserByID(embyId)` 校验目标 Emby 用户仍存在，避免列表加载后被删除导致脏绑定。
9. 应用层先查一次 `emby_id` 占用情况：
   - 已被同一管理员持有 → 直接成功（幂等）。
   - 已被其他本地账号持有 → 返回 409。
10. 校验当前账号 `EmbyID` 是否已绑定其他值：是则返回 409，要求先解绑。
11. 写入 `EmbyID = embyId`，由 DB 唯一索引兜底并发冲突。
12. 返回 `embyId + embyUsername`，前端刷新当前用户上下文。

解绑：

1. 管理员点击「解除关联」并通过二次确认。
2. 前端调用 `DELETE /api/v1/admin/current/emby-binding`。
3. 后端清空当前管理员 `EmbyID`，不删除 Emby 真实用户，不修改 Emby 任何属性。
4. 返回成功，前端刷新当前用户上下文。

### 5. 失败路径与边界条件

- Emby 服务不可用：返回 502，不写库；后端日志记录失败原因。
- Emby 用户候选搜索缺少关键词或关键词过短：返回 400，不调用 Emby。
- Emby 用户列表为空：返回 `data: []`，前端展示空状态，不写库。
- 目标 Emby 用户不存在：返回 404，不写库。
- Emby 用户已被其他本地账号绑定：返回 409，提示冲突方本地用户名。
- 并发同一 Emby 用户被两个管理员同时绑定：DB 偏唯一索引兜底；应用层捕获唯一约束错误并翻译为同 409 错误。
- 绑定接口不再接收 `embyUsername` / `embyPassword`；旧请求体应按参数错误返回 400，不做兼容兜底。
- 解绑后访问 media / playback：与未绑定用户一致返回"未绑定 Emby"，不允许半绑定中间态。
- 管理员账号已被禁用 / 已过期：仍允许调用绑定 / 解绑接口，因为 admin 不应被业务过期机制影响（沿用现有中间件行为）。
- 兼容性约束：
  - 不破坏普通用户注册即绑定链路。
  - 不破坏 `seedDefaultAdmin` 启动期纯本地行为。
  - 不破坏现有 `EmbyID != ""` 下游判断逻辑。
- 审计与日志：
  - 后端按 Go 日志规则在 `[Admin Emby Binding]` 前缀下记录绑定 / 解绑事件，包含本地 userID、目标 embyId、操作类型、结果。
  - 严禁输出 Emby API Key、Emby 完整返回体、Token。

## 影响范围

- API：
  - 新增 `GET /admin/emby-users` 路由 + 对应 handler、service 方法。
  - 调整 `PUT /admin/current/emby-binding` 请求体，从 `embyUsername + embyPassword` 改为 `embyId`。
  - 保留并依赖既有 `users.emby_id` 偏唯一索引 migration，不新增重复 migration。
  - 复用 `embyService.GetUsers` / `GetUserByID`，绑定流不再调用 `AuthenticateUser`。
- Web：
  - `services/web/src/views/console/AccountCenterView.vue` 将管理员绑定弹窗改成 Emby 用户选择器，不再出现密码输入框。
  - `services/web/src/api/admin.ts` 新增 Emby 用户列表 API，并更新绑定 API 请求类型。
  - `services/web/src/types/api.ts` 新增 `AdminEmbyUserOption` 等契约类型。
  - `services/web/src/store/user.ts` 复用现有当前用户刷新入口（如不存在则补一个最小函数，不引入新模式）。
- Bot：无。
- 配置 / 部署：无。
- 文档：
  - `docs/system-architecture.md`：管理员账号相关章节补充"管理员可自助关联 Emby 账号"。
  - 落地后归档本计划至 `docs/archive/plan/console-admin/`。

## 已完成项

- 后端新增 `GET /api/v1/admin/emby-users`，按关键词限量返回 Emby 用户候选列表与本地绑定占用状态。
- 后端 `PUT /api/v1/admin/current/emby-binding` 已改为按 `embyId` 绑定，不再调用 `AuthenticateUser`，不接收 Emby 密码。
- 后端错误映射已收口：缺少 `embyId` 返回 400，目标 Emby 用户不存在返回 404，Emby 不可用返回 502，绑定冲突返回 409。
- 前端账号中心绑定弹窗已改为 Emby 用户搜索选择器，不再打开弹窗即拉取全部用户；已被其他本地账号绑定的用户不可选。
- 前端 API 类型已新增 `AdminEmbyUserOption` / `AdminEmbyUserListResponse`，绑定请求类型已改为 `{ embyId }`。
- `docs/system-architecture.md` 已同步管理员 Emby 绑定新契约。

## 归档说明

- 代码落点已进入 release tag `v1.5.1`。
- `docs/system-architecture.md` 与 `docs/reference/api-endpoint-catalog.md` 已收录当前契约。
- 2026-05-17 真实环境日志已覆盖 `GET /api/v1/admin/emby-users`、`PUT /api/v1/admin/current/emby-binding` 和 `DELETE /api/v1/admin/current/emby-binding` 成功路径。

## 验证方式

### 编译 / 测试

- [x] `cd services/api && go test ./internal/services/auth ./internal/handlers`
- [x] `cd services/api && go test ./...`
- [x] `cd services/web && npx vitest run src/views/console/AccountCenterView.spec.ts`
- [x] `cd services/web && npm run build`

### 手工验证

- 管理员未绑定时访问媒体页 → 返回"未绑定 Emby"；绑定后再访问 → 返回真实数据。
- 管理员未绑定时访问个人播放档案 → 数据为空；绑定后 → 返回真实播放数据。
- 管理员打开绑定弹窗 → 能看到 Emby 用户列表；已绑定其他本地账号的用户不可选。
- 管理员选择可用 Emby 用户并确认 → 写入 `users.emby_id`，前端刷新当前用户上下文。
- 同一 Emby 用户尝试绑定到另一个本地账号 → 返回 409。
- 绑定请求提交不存在的 `embyId` → 返回 404，库未变更。
- 旧版请求体只提交 `embyUsername` / `embyPassword` → 返回 400，且不得触发前端退出登录。
- 关闭 Emby 服务后绑定 → 返回 502，库未变更。
- 解绑后再访问媒体页 → 回退到未绑定行为。
- 启动期未配置 Emby 时管理员仍可正常初始化、登录、进入控制台。
- DB 偏唯一索引迁移可重复执行不报错。

## 落地后文档处理

落地后应同步处理：

- `docs/system-architecture.md` 管理员相关章节沉淀本能力描述。
- 已完成回归验证并移入 `docs/archive/plan/console-admin/`。
- 若未来需要"绑定 / 解绑审计列表"或"管理员强制覆盖绑定"，新建独立计划，不在本方案扩展。
