# 管理员 API Key 实现方案

> 状态：已完成，已归档
> 负责人：Ember
> 更新时间：2026-07-09

## 落地状态

- 已实现后端 `AdminCredentialAuth()`、Admin API Key 生成 / 状态 / 禁用接口，以及 `external_api_key_hash` 配置项。
- 已实现设置中心 Admin API Key 管理区，生成后只展示一次明文，禁用和重新生成会立即刷新状态。
- 已补 Go 单元测试覆盖 JWT 管理员、JWT 普通用户、API Key 成功 / 失败 / 配置错误、生成 / 禁用和自管理拦截。
- 已同步 `docs/system-architecture.md`、`docs/reference/configuration-reference.md`、`docs/reference/api-endpoint-catalog.md`。
- 已完成全量验证并确认当前实现无需继续调整，本稿只保留历史追溯价值。

## 背景

当前 Ember API 主要依赖用户登录后的 JWT 访问：

- 管理员接口需要 `JWTAuth()`、`PasswordResetRequired()`、`AdminOnly()` 串联校验。
- Bot 到 API 的内部调用已有 `InternalAuth()`，但它是服务间固定密钥，不适合作为后台可生成的外部调用凭证。
- 如果外部自动化脚本需要调用管理员接口，只能先模拟管理员登录拿 JWT，调用体验差，也会把用户会话语义带入机器调用。

本方案提供一个轻量第一版：在配置表中保存单一全局 Admin API Key 的 hash，让管理员可以生成、替换和禁用该 key，用于调用管理类接口。

## 目标

1. 支持管理员在后台生成一个全局 Admin API Key，生成后只展示一次明文。
2. API Key 只保存 hash 到配置表，不新建 `api_keys` 表。
3. API Key 可通过清空配置禁用，可通过重新生成完成轮换。
4. 允许 API Key 访问 `/api/v1/admin/*` 管理员接口。
5. 保持现有 JWT 登录、管理员密码重置拦截、用户接口行为不变。

## 非目标

- 不做多 API Key。
- 不做 scope / 权限范围细分。
- 不做过期时间。
- 不做独立审计表。
- 不允许 API Key 访问依赖真实用户身份的 `/api/v1/user/*`、用户控制台、账号中心、自助兑换、支付下单等接口。
- 不复用 `InternalAuth()`，也不把 `X-Internal-Secret` 暴露为后台可管理能力。

## 当前事实

- 相关文档：
  - `docs/system-architecture.md`
  - `docs/reference/api-endpoint-catalog.md`
  - `docs/reference/api-response-standard.md`
  - `docs/reference/configuration-reference.md`
  - `docs/reference/web-design-guide.md`
- 相关服务 / 页面 / 模型：
  - `services/api/internal/middleware/jwt.go`
  - `services/api/internal/middleware/password_reset_required.go`
  - `services/api/internal/middleware/internal_auth.go`
  - `services/api/internal/app/routes.go`
  - `services/api/internal/config/config.go`
  - `services/api/internal/models/setting.go`
  - `services/web/src/views/admin/SettingsView.vue`
- 当前行为：
  - `/api/v1/admin/*` 当前统一走 `JWTAuth()` → `PasswordResetRequired()` → `AdminOnly()`。
  - `PasswordResetRequired()` 会用 `userID` 回查 `users` 表，并校验账号状态、角色和密码签名。
  - 设置中心通过 `ConfigService` 管理运行期配置，敏感项支持不回显。
- 现有限制：
  - 不能简单把 API Key 伪装成 `userID=api_key` 后继续走 `PasswordResetRequired()`，否则会因查不到真实用户而失败。
  - API Key 没有真实当前用户语义，不能进入用户侧接口。

## 方案设计

### 1. 用户可见行为

- 管理员在设置中心看到“Admin API Key”配置区。
- 未配置时显示未启用状态。
- 点击“生成”后，后端生成随机 key，前端弹窗只展示一次明文。
- 再次点击“重新生成”会覆盖旧 hash，旧 key 立即失效。
- 点击“禁用”会清空配置项，所有 API Key 请求立即失效。
- 已有管理员登录、用户登录、Bot Internal API 调用行为保持不变。
- 前端实现必须遵守 Ember 风格；设计与交互基线以 `docs/reference/web-design-guide.md` 为准。该区域只保留必要风险提示，不堆解释性文案。

### 2. 数据与模型

本次不新增数据表，不新增 GORM 模型，不提供 SQL migration。

新增一个设置中心配置项：

```text
external_api_key_hash
```

语义：

- 空值：Admin API Key 未启用。
- 非空：启用 Admin API Key 校验。
- 更换 key：服务端生成新 key，并覆盖该 hash。
- 禁用 key：清空该配置项。

配置项属性建议：

- `group`: `deployment` 或新增 `access` 分组。第一版优先复用 `deployment`，避免为了单项配置扩展前端分组。
- `type`: `secret`
- `sensitive`: `true`
- `editable`: `false` 或通过专用接口管理，避免管理员手填 hash / 明文造成语义混乱。
- `allowEmpty`: `true`
- `emptyValueMode`: `disable`
- `restartRequired`: `false`
- `DisableEnvFallback`: `true`

hash 规则：

- 明文 key 使用服务端随机生成，格式为 `ember_sk_` + 高熵随机字符串。
- 配置表只保存 hash，不保存明文。
- 校验时对请求 key 重新计算 hash，并使用 constant-time compare。
- hash 算法第一版可使用 `sha256`；如果实现时希望绑定服务端密钥，可使用 `HMAC-SHA256(CONFIG_ENCRYPTION_KEY, key)`，但必须处理未配置 `CONFIG_ENCRYPTION_KEY` 的 fresh-install 语义。

### 3. 接口与边界

新增管理员接口：

```text
GET    /api/v1/admin/external-api-key
POST   /api/v1/admin/external-api-key
DELETE /api/v1/admin/external-api-key
```

建议响应：

```json
{
  "success": true,
  "data": {
    "configured": true
  }
}
```

生成接口额外返回一次性明文：

```json
{
  "success": true,
  "data": {
    "configured": true,
    "apiKey": "ember_sk_xxx"
  }
}
```

请求调用方式：

```text
Authorization: Bearer ember_sk_xxx
```

管理员路由认证边界：

- 新增管理员凭证中间件，例如 `AdminCredentialAuth()`。
- JWT 请求保持现有链路：解析 JWT → 回查用户状态和密码签名 → 校验 `role=admin`。
- API Key 请求走配置项 hash 校验，校验通过后注入：
  - `role=admin`
  - `authType=api_key`
  - `userID=api_key`
- `PasswordResetRequired()` 不应被 API Key 路径直接复用，避免查 `users` 表失败或污染用户会话语义。

调用方影响：

- Web 设置中心新增 API Key 管理入口。
- 外部脚本可直接用 `Authorization: Bearer ember_sk_xxx` 调用 `/api/v1/admin/*`。
- Bot 仍使用 `X-Internal-Secret`，不受影响。
- 普通用户控制台仍使用 JWT，不受影响。

### 4. 关键流程

生成 / 轮换：

1. 管理员使用现有 JWT 登录后台。
2. 前端调用 `POST /api/v1/admin/external-api-key`。
3. 后端生成 `ember_sk_` 前缀的高熵随机 key。
4. 后端计算 hash，写入配置项 `external_api_key_hash`。
5. 后端只在本次响应返回明文 key。
6. 前端展示一次性明文，并提示离开后无法再次查看。

禁用：

1. 管理员点击禁用。
2. 前端调用 `DELETE /api/v1/admin/external-api-key`。
3. 后端清空 `external_api_key_hash`。
4. 后续 API Key 请求全部返回 401。

API Key 调用管理员接口：

1. 请求携带 `Authorization: Bearer ember_sk_xxx`。
2. 管理员凭证中间件识别 `ember_sk_` 前缀。
3. 后端读取 `external_api_key_hash`。
4. 空值返回 401。
5. 非空时计算请求 key 的 hash，并做 constant-time compare。
6. 校验通过后注入管理员 API Key 主体并继续处理请求。

### 5. 失败路径与边界条件

- 未配置 key：返回 401，不把未配置当作 500。
- key 格式不合法：返回 401。
- key 校验失败：返回 401，日志不输出明文 key。
- 配置读取失败：返回 500，并记录配置读取失败原因。
- 管理员重新生成 key：旧 key 立即失效。
- 管理员禁用 key：清空配置后立即失效，不需要重启。
- API Key 调用户身份接口：不支持，保持 401 或 403。
- API Key 调 `/api/v1/internal/*`：不支持，Internal API 仍只认 `X-Internal-Secret`。
- API Key 调公共接口：不需要支持，公共接口继续按现状访问。
- 日志记录只允许输出 `authType=api_key`、path、method、clientIP、校验结果，不输出 key 明文和 hash。

## 影响范围

- API：有。新增配置项、API Key 生成 / 禁用接口、管理员凭证中间件，并调整 `/api/v1/admin/*` 的认证链路。
- Web：有。设置中心增加 Admin API Key 管理入口，按 `docs/reference/web-design-guide.md` 使用现有后台页面骨架和克制文案。
- Bot：无。Bot Internal API 认证不变。
- 配置 / 部署：有。新增运行期配置项 `external_api_key_hash`，不要求重启，不依赖环境变量。
- 数据库：无新增表和 migration；仅使用现有 `settings` KV 表。
- 文档：落地后同步 `docs/system-architecture.md`、`docs/reference/configuration-reference.md`、`docs/reference/api-endpoint-catalog.md`，必要时补充 `docs/reference/api-response-standard.md` 的认证说明。

## 验证方式

### 编译 / 测试

- `cd services/api && go test ./...`
- `cd services/api && go build ./...`
- `cd services/web && npm run build`

### 测试覆盖

- Go 中间件测试：
  - JWT 管理员仍可访问管理员接口。
  - JWT 普通用户不能访问管理员接口。
  - 未配置 `external_api_key_hash` 时 API Key 返回 401。
  - 配置 hash 匹配时 API Key 可访问管理员接口。
  - 配置 hash 不匹配时返回 401。
  - API Key 不触发 `PasswordResetRequired()` 的用户表回查。
- Go handler / service 测试：
  - 生成 key 只返回一次明文。
  - 生成 key 后配置项保存 hash。
  - 禁用接口清空配置项。
  - 日志路径不输出明文 key。
- 前端测试：
  - 未配置状态展示。
  - 生成后弹窗展示一次性 key。
  - 禁用操作调用正确接口并刷新状态。

### 手工验证

- 管理员登录后台，生成 API Key，复制后可调用 `/api/v1/admin/system/info`。
- 使用旧 key 在重新生成后调用同一接口，返回 401。
- 禁用后使用最新 key 调用管理员接口，返回 401。
- 使用 API Key 调 `/api/v1/profile` 或 `/api/v1/user/profile`，不能通过用户身份认证。
- Bot 内部接口仍只接受 `X-Internal-Secret`。

## 落地后文档处理

已同步处理：

- `docs/system-architecture.md`：补充管理员 API Key 认证边界和与 JWT / InternalAuth 的关系。
- `docs/reference/configuration-reference.md`：登记 `external_api_key_hash`。
- `docs/reference/api-endpoint-catalog.md`：登记新增管理员 API Key 管理接口。
- 未调整通用 API 错误语义，`docs/reference/api-response-standard.md` 无需变更。

功能已上线并完成验证，本稿已迁入 `docs/archive/plan/access-auth/admin-api-key.md`。
