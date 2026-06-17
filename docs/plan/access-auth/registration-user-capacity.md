# 自助注册人数上限实现方案

> 状态：草稿
> 负责人：Ember
> 更新时间：2026-06-17

## 背景

Ember 当前支持开放注册、邀请码注册、邮箱验证码和注册邮箱域名白名单，但缺少“自助注册容量”控制：

- 小型 Emby 服通常有明确承载人数，管理员需要在后台设置最多允许多少普通用户通过注册页进入系统。
- 只在前端隐藏入口不能防止绕过；真正的门控必须落在注册 Service 创建 Emby 账号之前。
- 如果没有清晰统计口径，后台显示人数、注册页状态和实际注册结果容易不一致。

成熟自托管系统通常把这类能力放在“新用户账号限制”下。例如 GitLab Self-Managed 将关闭注册、管理员审核、邮箱确认、邮箱域名限制和 `User cap` 归为同一组新用户限制；`User cap` 留空表示不限制。Ember 应复用现有“基础业务”注册配置分组，而不是新增孤立页面。

## 目标

1. 支持管理员在设置中心配置自助注册人数上限。
2. 达到上限后，注册页停止提交新账号，并禁用验证码发送和邀请码预验证等前置动作。
3. 后端在注册主链路中硬性拦截满员注册，且拦截发生在创建 Emby 用户之前。
4. 后台和注册页使用同一份容量口径，避免显示可注册但实际失败。
5. 保持现有开放注册、邀请码注册、邮箱验证码、邮箱域名白名单和管理员后台创建用户行为不变。

## 非目标

- 不做候补队列、注册审批队列或等待名单。
- 不做自动切换注册模式，例如满员后自动改成邀请码注册。
- 不做多种统计口径配置，第一版不提供“只统计有效用户 / 未过期用户 / Emby 可用用户”等选项。
- 不限制管理员后台手动创建用户；后台创建属于管理动作，不纳入自助注册门控。
- 不让邀请码默认绕过上限；邀请码注册仍属于自助注册入口。
- 不新增独立容量管理页面。

## 当前事实

- 相关文档：
  - `docs/system-architecture.md`
  - `docs/reference/api-response-standard.md`
  - `docs/reference/web-design-guide.md`
  - `docs/reference/configuration-reference.md`
- 相关服务 / 页面 / 模型：
  - `services/api/internal/config/config.go`
  - `services/api/internal/handlers/setting.go`
  - `services/api/internal/services/auth/register.go`
  - `services/api/internal/services/auth/service.go`
  - `services/api/internal/models/user.go`
  - `services/web/src/views/user/RegisterView.vue`
  - `services/web/src/views/admin/SettingsView.vue`
  - `services/web/src/types/api.ts`
- 当前行为：
  - `/api/v1/register/mode` 返回注册模式、开放注册默认试用天数、邮箱验证码开关和注册邮箱域名白名单。
  - 注册 Service 会依次校验请求、邮箱域名、邮箱验证码、注册模式 / 邀请码、用户名和邮箱唯一性，然后创建 Emby 用户并落库。
  - 设置中心通过 `ConfigService` 暴露 `business` 分组，已有 `registration_mode`、`default_trial_days`、`email_verification` 和 `registration_allowed_email_domains`。
  - `users` 表通过 `role` 区分 `admin` 和普通 `user`，通过 `is_active`、`expires_at`、`emby_disabled` 等字段表达状态。
- 现有限制：
  - 当前没有公开容量状态，注册页无法在提交前判断是否满员。
  - 当前注册链路没有容量前置检查，若未来只做前端限制会被直接调用 API 绕过。

## 方案设计

### 1. 用户可见行为

后台设置中心：

- 在“设置中心 > 基础业务”新增配置项“自助注册人数上限”。
- 默认值为 `0`，表示不限制。
- 大于 `0` 时，限制通过注册页创建的普通用户数量。
- 文案保持克制：说明“达到上限后注册页停止提交新账号；管理员后台创建用户不受此限制”即可。

注册页：

- 未满员时保持现有注册体验。
- 满员时表单进入不可提交状态，主按钮显示“名额已满”。
- 满员时禁用“发送验证码”和“预验证”按钮，避免用户先完成前置动作后才被拒绝。
- 顶部展示一条短状态：“当前注册名额已满”。
- 不展示复杂容量解释，不把页面写成操作说明。

现有行为保持不变：

- 开放注册仍按 `default_trial_days` 计算试用期。
- 邀请码注册仍校验兑换码可用性和绑定套餐分组。
- 邮箱验证码和邮箱域名白名单继续按现有规则执行。
- 管理员后台创建用户不受自助注册人数上限限制。

前端实现必须遵守 Ember 风格；设计与交互基线以 `docs/reference/web-design-guide.md` 为准。注册页和设置中心只保留用户决策必要信息，不新增解释型长文案。

### 2. 数据与模型

本次不新增数据表，不修改 `users` 表，不新增 GORM 模型字段，因此不需要 SQL migration。

新增运行期配置项：

```text
registration_user_limit
```

建议配置定义：

- `group`: `business`
- `label`: `自助注册人数上限`
- `type`: `integer`
- `defaultValue`: `0`
- `editable`: `true`
- `minValue`: `0`
- `maxValue`: `100000`
- `validate`: 整数范围校验
- `description`: `达到上限后注册页停止提交新账号；0 表示不限制`

容量统计口径固定为：

```sql
role = 'user'
```

语义：

- 只统计普通用户。
- 排除管理员。
- 包含已过期、已禁用、Emby 已禁用、未绑定 Telegram 的普通用户。
- 只有删除普通用户才释放名额。

这个口径足够简单，后台显示、注册页状态和后端拦截可以保持一致。不要第一版引入“有效用户数”这类动态口径，否则过期检查、封禁、Emby 同步失败都会影响容量判断。

### 3. 接口与边界

修改公开注册模式接口：

```text
GET /api/v1/register/mode
```

响应新增字段：

```json
{
  "mode": "open",
  "defaultTrialDays": 7,
  "emailVerification": true,
  "allowedEmailDomains": ["example.com"],
  "registrationLimit": 100,
  "registeredUserCount": 87,
  "registrationFull": false
}
```

字段语义：

- `registrationLimit`: 当前自助注册人数上限；`0` 表示不限制。
- `registeredUserCount`: 当前普通用户数量，按 `role='user'` 统计。
- `registrationFull`: 当 `registrationLimit > 0 && registeredUserCount >= registrationLimit` 时为 `true`。

修改注册接口行为：

```text
POST /api/v1/user/register
```

- 满员时返回 400，错误文案建议为“当前注册名额已满”。
- 错误应纳入现有 `isAuthRegisterBadRequest` 分支，保持注册页错误处理一致。
- 后端必须在创建 Emby 用户之前检查容量。

不新增 Bot Internal API，不修改 webhook，不修改支付和兑换接口。

### 4. 关键流程

公开容量状态：

1. 注册页进入时请求 `/api/v1/register/mode`。
2. 后端读取 `registration_user_limit`。
3. 后端统计 `users.role = 'user'` 数量。
4. 后端返回 `registrationLimit`、`registeredUserCount` 和 `registrationFull`。
5. 前端根据 `registrationFull` 控制表单、验证码按钮和邀请码预验证按钮。

注册提交：

1. 后端校验用户名、密码、邮箱等基础请求字段。
2. 后端执行注册邮箱域名白名单校验。
3. 后端执行邮箱验证码校验。
4. 后端解析注册模式；邀请码模式下校验邀请码。
5. 后端检查自助注册容量。
6. 容量已满时直接返回业务错误，不创建 Emby 用户，不消耗邀请码，不写本地用户。
7. 容量未满时继续现有用户名 / 邮箱唯一性检查。
8. 后续创建 Emby 用户、落库、通知和 token 生成沿用现有流程。

设置更新：

1. 管理员在设置中心修改“自助注册人数上限”。
2. 前端按现有配置中心保存本组配置。
3. 后端通过 `ConfigService.Update` 校验整数范围并写入 `settings`。
4. 新值即时影响 `/register/mode` 和注册 Service，不要求重启。

### 5. 失败路径与边界条件

- `registration_user_limit = 0`：不限注册人数，`registrationFull=false`。
- `registration_user_limit` 配置缺失：按默认值 `0` 处理。
- `registration_user_limit` 配置脏数据：`ConfigService` 读取方法应兜底为 `0`，不要因为脏配置打死注册页。
- 满员时请求发送邮箱验证码：第一版可继续允许验证码发送，也可新增发送验证码前置容量检查；推荐同步拦截，避免无效邮件发送。若实现同步拦截，必须补测试。
- 满员时邀请码预验证：可返回兑换码本身可用，但注册提交仍失败；推荐前端满员时直接禁用预验证，后端兑换码校验接口不改变语义。
- 并发注册：容量检查必须尽量靠近注册事务；第一版至少要保证创建 Emby 用户前检查。若需要严格防止并发超卖，应在注册落库事务中对容量检查加锁或使用事务内可重复统计，但这会增加实现复杂度。
- 邀请码模式满员：注册提交失败，不消耗邀请码。
- 管理员后台创建用户：不受限制，但后台用户列表可在后续增强中展示容量占用。
- 日志：满员拦截日志可记录 `limit`、`count`、`mode`，不得记录密码、验证码、token。

## 影响范围

- API：有。新增配置读取方法、容量统计方法、注册 Service 前置门控、`/register/mode` 响应字段和注册错误映射。
- Web：有。注册页根据容量状态禁用表单动作；设置中心自然渲染新增整数配置项；类型定义同步新增字段。
- Bot：无。不影响 Telegram Bot、通知入口和 Internal API。
- 配置 / 部署：有。新增运行期配置项 `registration_user_limit`，通过现有 `settings` 表保存，不需要重启，不需要环境变量。
- 数据库：无 schema 变更，不需要 SQL migration。
- 文档：落地后同步 `docs/system-architecture.md`、`docs/reference/configuration-reference.md`，必要时同步 API 目录或响应字段说明。

## 验证方式

### 编译 / 测试

- `cd services/api && go test ./...`
- `cd services/api && go build ./...`
- `cd services/web && npm run test`
- `cd services/web && npm run build`

### 重点测试用例

API：

- `registration_user_limit=0` 时允许注册继续进入现有流程。
- 当前普通用户数小于上限时允许注册继续进入现有流程。
- 当前普通用户数等于上限时注册返回“当前注册名额已满”，且不调用 Emby 创建用户。
- 当前普通用户数大于上限时注册返回“当前注册名额已满”。
- 管理员用户不计入容量。
- 已过期 / 已禁用普通用户仍计入容量。
- 邀请码模式满员时不消耗邀请码。
- `/register/mode` 返回 `registrationLimit`、`registeredUserCount`、`registrationFull`，且列表字段仍使用现有命名风格。

Web：

- 注册页未满员时保持现有开放注册和邀请码注册交互。
- 注册页满员时主按钮显示“名额已满”，验证码发送和邀请码预验证不可点。
- 满员状态下表单不提交 `POST /user/register`。
- 设置中心能编辑并保存“自助注册人数上限”，整数输入校验与现有配置项一致。

手工验证：

- 设置上限为 `0`，注册页显示正常注册入口。
- 设置上限为当前普通用户数，刷新注册页后显示满员状态。
- 删除一个普通用户后，刷新注册页恢复可注册状态。

## 落地后文档处理

落地后应同步处理：

- 将稳定配置项语义补充到 `docs/reference/configuration-reference.md`。
- 将注册容量门控补充到 `docs/system-architecture.md` 的用户注册 / 登录能力说明。
- 如项目维护 API 端点目录，补充 `/register/mode` 新增响应字段。
- 实现、测试和文档同步完成后，将本文状态改为“已实现，待归档”；确认无需继续调整后归档到 `docs/archive/plan/access-auth/registration-user-capacity.md`。
