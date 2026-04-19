# 注册邮箱域名白名单方案

> 状态：草稿
> 负责人：Ember
> 更新时间：2026-04-19

## 背景

这个问题为什么现在要解决：

- 当前注册链路只校验邮箱格式、唯一性和可选邮箱验证码，不限制邮箱提供商，任何合法域名都可以注册。
- 邮箱验证码只能证明“这个邮箱能收信”，不能解决“是否允许这个邮箱提供商进入系统”的门控问题。
- 如果不加这层限制，开放注册和邀请码注册都会继续接受一次性邮箱或不在运营范围内的邮箱提供商，管理员只能在注册后被动处理。

## 目标

本方案要实现：

1. 让管理员可以在运行期配置“允许注册的邮箱域名列表”，不需要重启服务。
2. 让注册验证码发送入口和正式注册入口共用同一套域名门控，避免出现“验证码能发，注册却被拦”的裂缝。
3. 让注册页能提前展示当前限制范围，但后端仍然是唯一可信校验点。

## 非目标

本次明确不做：

- 不回溯清理或封禁已经存在的用户邮箱。
- 不把限制扩大到后台创建用户、用户修改邮箱、管理员修改邮箱、密码重置验证码发送。
- 不做通配符、后缀匹配或“邮箱提供商家族”自动推导；本次只做精确域名匹配，例如 `gmail.com`、`outlook.com`。

## 当前事实

以当前代码和现行文档为准，先写清现状：

- 相关文档：
  - `docs/system-architecture.md`
  - `docs/reference/configuration-reference.md`
  - `docs/reference/web-design-guide.md`
- 相关服务/页面：
  - `services/api/internal/services/auth/register.go`
  - `services/api/internal/services/email/verification.go`
  - `services/api/internal/config/config.go`
  - `services/api/internal/handlers/setting.go`
  - `services/web/src/views/user/RegisterView.vue`
  - `services/web/src/types/api.ts`
- 当前行为：
  - `POST /api/v1/user/register` 的 `RegisterUserRequest` 只要求 `email` 满足 `binding:"required,email"`，`AuthService.validateRegisterRequest()` 只补了用户名校验，没有邮箱域名门控。
  - `POST /api/v1/register/send-code` 会先判断邮箱是否已注册、再做频控、再发注册验证码，同样没有邮箱域名门控。
  - `GET /api/v1/register/mode` 目前只返回 `mode`、`defaultTrialDays`、`emailVerification`，注册页无法得知允许哪些邮箱域名。
  - 设置中心已经支持运行期业务配置，但 `json_list` 在前端当前只支持基于固定 `options` 的复选框，不适合录入任意邮箱域名。
- 现有限制：
  - 如果直接把域名白名单塞进前端校验，调用方绕过 Web 直接请求 API 仍然能注册，等于没做。
  - 如果只在正式注册时拦截，不在发送验证码时拦截，会浪费 SMTP 并制造错误体验。

## 方案设计

### 1. 用户可见行为

- 新增“注册邮箱域名白名单”运行期配置；为空时表示不限制，保持当前行为不变。
- 当白名单非空时，注册页展示允许的域名列表或提示文案，例如“当前仅支持以下邮箱域名注册：gmail.com、outlook.com”。
- 用户输入不在白名单内的邮箱时：
  - 发送注册验证码直接失败，不发邮件。
  - 提交注册直接失败，不创建 Emby 用户，不写本地用户，不消耗邀请码。
- 现有 `registration_mode`、`email_verification`、邀请码注册逻辑保持不变；只是多了一层注册邮箱域名门控。

### 2. 数据与模型

> 本次不涉及数据模型变更。

运行期配置新增一个业务配置项：

- 配置 key：`registration_allowed_email_domains`
- 存储方式：沿用 `settings` 表
- 值格式：多行字符串，每行一个域名
- 空值语义：空值表示关闭限制，允许任意合法邮箱域名注册

设计选择：

- 不用 `json_list`
  - 原因：当前设置中心对 `json_list` 的编辑器是复选框，只适合固定枚举，不适合录入任意域名。
- 采用多行字符串
  - 原因：复用现有 `multiline` 编辑器即可落地，不需要额外增加设置中心控件。

规范化规则：

- 保存时对每行做 `trim`、转小写、去重、排序。
- 只接受主机名格式，拒绝空行、协议、端口、路径、通配符。
- 精确匹配域名，不做后缀匹配；`mail.gmail.com` 不会自动匹配 `gmail.com`。

### 3. 接口与边界

配置层新增能力：

- 在 `ConfigService` 中新增 `registration_allowed_email_domains` 配置定义。
- 新增域名列表解析与规范化函数，例如：
  - `GetRegistrationAllowedEmailDomains() []string`
  - `IsRegistrationEmailAllowed(email string) error`
- 校验逻辑统一放在配置/门控辅助层，不把同一份规则分别手写在 `auth` 和 `email` 两处。

公开接口调整：

- `GET /api/v1/register/mode`
  - 响应新增 `allowedEmailDomains` 字段，类型为 `string[]`
  - 空数组或缺省表示当前不限制

注册相关入口调整：

- `POST /api/v1/register/send-code`
  - 当 `codeType=register` 时，在“是否已注册”和“频控”之前先校验域名白名单
  - 不允许的域名直接返回 `400`
- `POST /api/v1/user/register`
  - 在用户名整理完成后、验证码校验前增加同一套域名白名单校验
  - 不允许的域名直接返回 `400`

调用方影响：

- Web 注册页需要更新 `RegistrationModeResponse` 类型和页面提示，但不需要新增路由。
- Bot 无影响。
- 后台设置中心只新增一个配置项，不需要新增页面。

### 4. 关键流程

1. 管理员在设置中心保存 `registration_allowed_email_domains`，每行一个允许注册的邮箱域名。
2. `ConfigService` 在保存时完成域名列表规范化，持久化到 `settings`。
3. 注册页初始化调用 `GET /api/v1/register/mode`，拿到 `allowedEmailDomains` 后显示提示文案，并可做本地预警。
4. 用户点击“发送验证码”时，后端先校验邮箱域名是否允许；不允许则直接返回错误，不发送邮件。
5. 用户提交注册时，`AuthService` 再次做同一套域名校验；不允许则直接返回错误，不创建 Emby 用户、不落库用户。
6. 当白名单为空时，两条链路都直接跳过域名门控，保持当前行为。

### 5. 失败路径与边界条件

- 白名单为空：视为功能关闭，保持当前开放行为，不能把空值解释成“全部拒绝”。
- 邮箱大小写或带显示名：先走地址解析，再按小写域名比较；比较对象只看真正的 `Address` 域名部分。
- 管理员配置了 `gmail.com`，用户填写 `user@GMAIL.COM`：应视为允许。
- 管理员只配置了 `gmail.com`，用户填写 `user@mail.gmail.com`：应视为不允许；这是精确匹配语义，不做隐式放宽。
- 配置中存在非法域名：设置中心保存失败，不接受脏配置进入运行期。
- 兼容性约束：
  - 不改变邀请码校验、试用天数分配、Emby 用户创建顺序和失败回滚逻辑。
  - 不改变密码重置验证码发送逻辑。
  - 不改变后台创建用户与用户修改邮箱行为。

## 影响范围

涉及的子系统：

- API：有，涉及 `config`、`setting handler`、`auth/register`、`email/verification` 以及对应测试。
- Web：有，涉及注册页提示、`RegistrationModeResponse` 类型；设置中心只新增配置项，不需要新增控件。
- Bot：无。
- 配置/部署：有，新增运行期数据库配置；不新增环境变量，不需要重启。
- 文档：落地时需同步 `docs/system-architecture.md` 与 `docs/reference/configuration-reference.md`。

## 验证方式

### 编译/测试

- `cd services/api && go test ./...`
- `cd services/api && go build ./...`
- `cd services/web && npm run build`

### 手工验证

- 白名单为空时，使用 `user@example.com` 正常发送验证码并注册成功，确认现有行为不变。
- 白名单设置为 `gmail.com` 后，`user@gmail.com` 可以发送验证码并注册成功。
- 白名单设置为 `gmail.com` 后，`user@outlook.com` 在发送验证码阶段直接失败，且未发送邮件。
- 白名单设置为 `gmail.com` 后，绕过前端直接调用 `POST /api/v1/user/register` 提交 `user@outlook.com`，后端仍然拒绝。
- 白名单配置为 `Gmail.com` 与 `outlook.com` 混合大小写时，保存后应被规范化为去重、小写后的稳定格式。
- 已存在用户使用非白名单邮箱时，密码重置验证码功能仍正常，确认本次限制没有误伤非注册链路。

## 落地后文档处理

落地后应同步处理：

- 提炼配置项、公开接口字段和注册链路新门控到 `docs/reference/configuration-reference.md` 与 `docs/system-architecture.md`
- 功能上线并验证稳定后，将本方案移入 `docs/archive/plan/access-auth/`
