# 登录 Turnstile 人机校验实现方案（已落地）

> 状态：已归档（已落地）
> 负责人：Ember
> 更新时间：2026-03-28

## 背景

这个问题为什么现在要解决：

- 当前登录接口 `POST /api/v1/login` 只校验用户名和密码，没有任何人机校验，容易被撞库和低成本脚本扫射。
- 现有登录链路同时承担 Ember 本地密码校验和普通用户的 Emby 联动认证，攻击面比单一站点登录更敏感。
- 传统图片验证码体验差、无障碍成本高、后续维护收益低，不适合作为当前 Ember 的优先方案。
- 如果继续放任登录入口裸露，后续即使再补限流和 WAF，仍然缺少表单级的人机校验闭环。
- Turnstile 的前端 `site key` 和启停策略如果全部放在环境变量层，每次开关或更换站点 key 都要动部署，运营成本偏高。

## 目标

本方案要实现：

1. 在登录流程中接入 Cloudflare Turnstile，增加表单级人机校验能力。
2. 保持未启用时的现有登录行为不变，避免强行破坏现有部署。
3. 校验逻辑必须以后端服务端验证为准，不能把安全性建立在前端组件是否展示上。
4. 将低敏感配置（开关、site key、hostname）纳入后台设置中心，减少部署层改动频率。

## 非目标

本次明确不做：

- 不实现自研图片验证码、短信验证码或邮箱二次验证码。
- 不把 Turnstile `secret key` 做进设置中心或数据库配置。
- 不顺手实现登录限流、IP 黑名单、Cloudflare WAF 规则编排；这些可作为后续独立安全治理项。
- 不改注册、忘记密码、Bot、内部 API 等其他入口的人机校验。

## 当前事实

以当前代码和现行文档为准，先写清现状：

- 相关文档：`docs/system-architecture.md`、`docs/runbooks/deployment.md`
- 相关服务/页面/模型：
  - `services/api/internal/config/config.go`
  - `services/api/internal/handlers/auth.go`
  - `services/api/internal/services/auth/login.go`
  - `services/web/src/views/LoginView.vue`
  - `services/web/src/api/auth.ts`
  - `services/web/src/store/auth.ts`
  - `services/web/src/types/api.ts`
- 当前行为：
  - Web 登录页仅提交 `username` 和 `password`
  - Go API `Login` 处理器只做参数绑定后调用 `AuthService.Login`
  - 登录成功返回 `{ token, user, isExpired }`
  - 登录失败统一返回 `401` 和错误信息
- 现有限制：
  - API 当前已有设置中心与 `ConfigService`，适合承接运行期开关和低敏感配置，但敏感密钥仍更适合留在环境变量。
  - Web 当前未使用任何第三方前端安全组件，也没有现成的 Turnstile 组件封装。
  - Cloudflare Turnstile 官方要求 token 以后端 `siteverify` 为准，token 具备时效性且不可重复使用。

## 方案设计

### 1. 用户可见行为

- 当登录保护启用时：
  - 登录页展示 Cloudflare Turnstile 校验组件。
  - 用户只有在拿到有效 token 后，才允许提交登录请求。
  - 校验失败或过期时，登录页提示“人机校验失败，请重试”或等价的简洁文案。
- 当登录保护未启用时：
  - 登录页保持当前样式和交互，不新增额外阻断步骤。
- 以下现有行为必须保持不变：
  - 用户名/密码错误时，仍然保持当前统一错误语义，不泄露更多账号状态。
  - 登录成功响应结构不变，前端 `authStore` 的持久化逻辑不变。
  - 管理员和普通用户仍然共用同一个 `/api/v1/login` 入口。

### 2. 数据与模型

本次不新增业务模型表，但会扩展现有设置中心配置定义。

运行期配置（进入设置中心 / 数据库配置）：

- `turnstile_login_enabled`
- `turnstile_site_key`
- `turnstile_expected_hostname`（可选，但建议）

部署层敏感配置（保留环境变量）：

- `TURNSTILE_SECRET_KEY`

设计约束：

- `turnstile_site_key` 是公开值，可通过 API 返回给前端。
- `TURNSTILE_SECRET_KEY` 仅允许 API 进程读取，禁止暴露到 Web 构建产物、禁止进入设置中心。
- `turnstile_login_enabled=true` 但后台未配置 `turnstile_site_key` 或 API 未配置 `TURNSTILE_SECRET_KEY` 时，应视为配置不完整并拒绝启用登录保护。

### 3. 接口与边界

- 保持登录路由不变：`POST /api/v1/login`
- 修改登录请求体：

```json
{
  "username": "alice",
  "password": "secret",
  "turnstileToken": "cf-turnstile-token"
}
```

- 字段约束：
  - `turnstileToken` 在 `turnstile_login_enabled=true` 时必填
  - 未启用时可省略
- 新增一个公开配置读取接口，供登录页获取公开配置：
  - 最小返回字段：

```json
{
  "turnstileLoginEnabled": true,
  "turnstileSiteKey": "0x4AAAA...",
  "turnstileExpectedHostname": "ember.example.com"
}
```

- 响应边界：
  - 登录成功响应结构不变
  - 请求体缺失校验 token、token 无效、token 过期、action/hostname 不匹配时，返回 `400` 和通用错误文案，例如 `{"error":"人机校验失败，请重试"}`
  - 用户名或密码错误继续返回 `401`
- 新增后端内部边界：
  - 新增一个独立的 `TurnstileVerifier` 或等价轻量服务，负责调用 Cloudflare `siteverify`
  - `AuthService.Login` 不直接拼 HTTP 请求，避免把登录编排和第三方校验耦死
  - 前端不直接依赖环境变量读取 site key，而是通过后端公开配置接口获取

### 4. 关键流程

按顺序写清主链路，不需要贴代码：

1. API 通过 `ConfigService` 暴露登录保护的公开配置：是否启用、公开 `site key`、可选 hostname。
2. Web 登录页加载时先读取这组公开配置，判断是否渲染 Turnstile 组件。
3. 若启用，则前端使用返回的 `site key` 渲染 Turnstile，并在校验通过后拿到 `turnstileToken`。
4. 用户点击登录时，前端将 `username/password/turnstileToken` 一并提交到 `/api/v1/login`。
5. API `AuthHandler.Login` 绑定请求体后，先判断运行期配置 `turnstile_login_enabled` 是否开启。
6. 若开启，则调用 `TurnstileVerifier` 对 token 做服务端校验：
   - 请求 `https://challenges.cloudflare.com/turnstile/v0/siteverify`
   - 使用环境变量 `TURNSTILE_SECRET_KEY`
   - 校验 `success`
   - 校验 `action == "login"`（前端需显式声明 action）
   - 若后台配置了 `turnstile_expected_hostname`，则同时校验 `hostname`
7. Turnstile 校验通过后，继续执行现有 `AuthService.Login` 用户名/密码链路。
8. 登录成功后，前端按当前逻辑保存 token、写入 store 并跳转控制台。

### 5. 失败路径与边界条件

- 登录保护未启用：系统完全沿用当前登录行为，不要求前端渲染组件，也不要求请求体带 token。
- 登录保护已启用但后台未配置 `turnstile_site_key`：
  - 登录页应视为配置错误，禁止继续提交，并给出明确前端提示。
- 登录保护已启用但 API 未配置 `TURNSTILE_SECRET_KEY`：
  - 方案建议 API 启动时直接失败，避免进入“前端展示了校验组件、后端却不校验”的假安全状态。
- 登录保护已启用且后台配置了 `turnstile_expected_hostname`，但 hostname 不匹配：
  - 按校验失败处理，不降级放行。
- Cloudflare `siteverify` 超时或网络异常：
  - 登录直接失败，返回通用错误文案，不降级放行。
- 校验通过但用户名/密码错误：
  - 仍走当前 `401` 语义，不把账号存在性、角色、Emby 绑定状态暴露给攻击者。
- token 重放或过期：
  - 后端按校验失败处理，前端提示用户重新完成校验。
- 兼容性约束：
  - 使用现有设置中心 KV，不新增独立业务表，不引入额外 schema migration。
  - 不改变现有 JWT 生成和登录成功响应结构。
  - 不要求 Bot 或其他服务改动。

## 影响范围

涉及的子系统：

- API：有，涉及设置中心公开配置读取、登录处理器、认证服务编排、新增 Turnstile 校验客户端/服务
- Web：有，涉及登录页、认证请求层、类型定义、Turnstile 组件接入与公开配置读取
- Bot：无
- 配置/部署：有，需补充设置中心字段与 `TURNSTILE_SECRET_KEY` 环境变量文档
- 文档：需更新 `docs/system-architecture.md`、部署文档、必要时补设置中心相关参考文档

## 验证方式

### 编译/测试

- `cd services/api && go test ./...`
- `cd services/api && go build ./...`
- `cd services/web && npm run build`

如实现时补了 Turnstile 校验服务的单元测试，至少覆盖：

- token 缺失
- siteverify 返回 `success=false`
- action 不匹配
- hostname 不匹配
- siteverify 超时/网络错误
- 运行期开关关闭时跳过校验
- 运行期开关开启但后台缺少 `site key` 时拒绝启用

### 手工验证

- 未启用 `turnstile_login_enabled` 时，登录页和当前完全一致，正常用户名密码可登录。
- 后台启用后，登录页可正确拉取 `site key` 并渲染 Turnstile。
- 启用后，未完成 Turnstile 校验时，登录请求不能成功发出或会被前端拦截。
- 启用后，完成校验且用户名密码正确时，可正常登录。
- 启用后，完成校验但用户名密码错误时，仍返回当前统一登录失败提示。
- 强制让 token 过期或复用旧 token，确认后端拒绝并提示重新校验。
- 人为制造 `siteverify` 超时/失败，确认后端 fail-closed，不会放行登录。
- 后台关闭登录保护后，登录页无需重新部署即可恢复到无 Turnstile 状态。

## 落地后文档处理

落地后应同步处理：

- 将登录接入 Turnstile、登录请求新增 `turnstileToken`、公开配置读取方式的稳定结论同步到 `docs/system-architecture.md`
- 将部署所需的 `TURNSTILE_SECRET_KEY` 补充到部署环境文档
- 将设置中心新增的 `turnstile_login_enabled`、`turnstile_site_key`、`turnstile_expected_hostname` 补充到现行配置文档
- 若后续决定把“登录限流 + Turnstile + Cloudflare WAF”合并治理，再另开安全提案；本方案完成后移入 `docs/archive/`
