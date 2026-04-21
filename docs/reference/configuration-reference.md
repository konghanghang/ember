# Ember 配置参考

> 本文档用于说明 Ember 当前配置的来源、用途、敏感性和生效方式，作为后续排查和维护的统一参考。

---

## 1. 先看结论

当前配置分成三类：

1. **API 运行期数据库配置**
   - 由设置中心管理
   - 保存到数据库 `settings` 表
   - 其中大多数修改后可立即生效
   - 调度相关配置修改后需要重启 API 才会生效

2. **API 部署期环境变量**
   - 用于数据库连接、签名密钥、Webhook 验签、内部服务鉴权、加密主密钥等边界配置
   - 不应放进设置中心

3. **Bot 启动环境变量**
   - Bot 进程启动时直接读取
   - `TELEGRAM_ADMIN_CHAT_ID`、`TELEGRAM_GROUP_CHAT_ID`、`notify_group_link`、`telegram_welcome_message_template` 在运行期通过 API 设置中心读取，并带短 TTL 缓存
   - `TELEGRAM_ADMIN_CHAT_ID`、`TELEGRAM_GROUP_CHAT_ID` 仍支持本地 env 回退，但它们属于可选兜底，不再作为 `.env.example` 默认项

---

## 2. API 数据库配置

以下配置由 API 的 `ConfigService` 统一解析，并由设置中心托管。

### 2.1 基础业务

| 配置项 | 敏感 | 需重启 | 说明 |
|--------|------|--------|------|
| `registration_mode` | 否 | 否 | 注册模式，`open` 或 `invite` |
| `default_trial_days` | 否 | 否 | 开放注册时默认试用天数 |
| `notify_group_link` | 否 | 否 | Telegram 欢迎消息中的群组链接 |
| `telegram_welcome_message_template` | 否 | 否 | Telegram 入群欢迎语模板，支持 `{names}` 和 `{notifyGroupLink}` 占位符 |
| `email_verification` | 否 | 否 | 是否启用注册邮箱验证码 |
| `turnstile_login_enabled` | 否 | 否 | 是否启用登录 Turnstile 人机校验 |
| `turnstile_site_key` | 否 | 否 | 登录页渲染 Turnstile 使用的公开 Site Key |
| `turnstile_expected_hostname` | 否 | 否 | 登录 Turnstile 服务端校验时要求匹配的 hostname，允许置空 |
| `stripe_allowed_payment_methods` | 否 | 否 | Stripe 支付方式限制列表 |

### 2.2 媒体集成

| 配置项 | 敏感 | 需重启 | 说明 |
|--------|------|--------|------|
| `EMBY_URL` | 否 | 否 | API 访问 Emby 的基础地址 |
| `EMBY_API_KEY` | 是 | 否 | Emby API 鉴权密钥 |
| `NEXT_PUBLIC_EMBY_URL` | 否 | 否 | 预留前端公网地址；当前控制台图片与地址展示不读取该项 |
| `TMDB_API_KEY` | 是 | 否 | TMDB 接口密钥 |
| `MOVIEPILOT_URL` | 否 | 否 | MoviePilot 地址 |
| `MOVIEPILOT_API_KEY` | 是 | 否 | MoviePilot API Key（X-API-KEY） |

说明：

- 旧版 `MOVIEPILOT_USERNAME` / `MOVIEPILOT_PASSWORD` 已废弃。
- 若历史实例仍保存旧用户名密码配置，而 `MOVIEPILOT_API_KEY` 为空，设置中心测试应视为需要迁移，而不是“未配置成功”。

### 2.3 邮件服务

| 配置项 | 敏感 | 需重启 | 说明 |
|--------|------|--------|------|
| `SMTP_HOST` | 否 | 否 | SMTP 主机 |
| `SMTP_PORT` | 否 | 否 | SMTP 端口 |
| `SMTP_USERNAME` | 是 | 否 | SMTP 用户名 |
| `SMTP_PASSWORD` | 是 | 否 | SMTP 密码 |
| `SMTP_FROM` | 否 | 否 | 发件人地址，允许置空后回退到 `SMTP_USERNAME` |
| `EMAIL_CODE_EXPIRY_MINUTES` | 否 | 否 | 邮箱验证码有效期 |
| `EMAIL_CODE_DAILY_LIMIT` | 否 | 否 | 单邮箱日发送上限 |
| `EMAIL_CODE_IP_DAILY_LIMIT` | 否 | 否 | 单 IP 日发送上限 |

### 2.4 通知与支付

| 配置项 | 敏感 | 需重启 | 说明 |
|--------|------|--------|------|
| `BOT_NOTIFY_URL` | 否 | 否 | API 推送通知到 Bot 的地址 |
| `TELEGRAM_ADMIN_CHAT_ID` | 否 | 否 | 管理员通知 Chat ID |
| `TELEGRAM_GROUP_CHAT_ID` | 否 | 否 | 群推送 Chat ID，允许置空后回退到管理员 Chat ID |
| `STRIPE_SECRET_KEY` | 是 | 否 | Stripe 服务端密钥 |
| `STRIPE_SUCCESS_URL` | 否 | 否 | Stripe 支付成功跳转地址 |
| `STRIPE_CANCEL_URL` | 否 | 否 | Stripe 支付取消跳转地址 |

### 2.5 调度配置

| 配置项 | 敏感 | 需重启 | 说明 |
|--------|------|--------|------|
| `CRON_ENABLED` | 否 | 是 | API 内置 cron 总开关 |
| `CRON_SCHEDULE` | 否 | 是 | 过期检查 cron 表达式 |
| `CRON_TIMEZONE` | 否 | 是 | cron 执行时区，同时作为追剧日历 `today / upcoming / missing` 的状态判定时区 |
| `RANKING_CRON_ENABLED` | 否 | 是 | 播放排行榜 cron 开关 |
| `RANKING_DAILY_SCHEDULE` | 否 | 是 | 日榜 cron 表达式 |
| `RANKING_WEEKLY_SCHEDULE` | 否 | 是 | 周榜 cron 表达式 |
| `TV_CALENDAR_STARTUP_SYNC_ENABLED` | 否 | 是 | API 启动后是否自动执行一次追剧日历补偿同步 |
| `TV_CALENDAR_SYNC_SCHEDULE` | 否 | 是 | 追剧日历同步 cron 表达式 |

说明：

- `CRON_TIMEZONE` 同时作用于追剧日历用户可见状态和周范围判定，不只是 cron 触发时间本身。

- 上述配置已经由设置中心数据库托管，API 不再依赖 Docker 环境变量回退。
- 调度相关配置虽然也在数据库中，但当前仍是“启动时装配调度器”的模型，所以修改后需要重启 API。

---

## 3. API 环境变量

以下配置仍然应通过环境变量注入，不放进设置中心。

### 3.1 `.env.example` 默认保留项

这些项要么是 API 启动硬依赖，要么是只能放环境变量里的密钥，因此会保留在 `services/api/.env.example`。

| 配置项 | 敏感 | 说明 | 原因 |
|--------|------|------|------|
| `DATABASE_URL` | 是 | PostgreSQL 连接串 | API 启动硬依赖 |
| `JWT_SECRET` | 是 | 用户 JWT 签名密钥 | 登录态信任根，不应在线修改 |
| `CONFIG_ENCRYPTION_KEY` | 是 | 敏感配置加密主密钥 | 数据库敏感配置加解密根密钥 |
| `INTERNAL_API_SECRET` | 是 | API 与 Bot 内部调用共享密钥 | 服务间鉴权根密钥 |
| `STRIPE_WEBHOOK_SECRET` | 是 | Stripe Webhook 签名密钥 | 仅启用 Stripe Webhook 时需要，且只能走环境变量 |
| `TURNSTILE_SECRET_KEY` | 是 | 登录 Turnstile 服务端校验密钥 | 仅启用 Turnstile 登录校验时需要，且不应进入设置中心 |
| `EMBY_WEBHOOK_TOKEN` | 是 | Emby Webhook token | 仅启用 Emby Webhook 回写追剧日历时需要，且只能走环境变量 |

### 3.2 仍是环境变量来源，但不放进 `.env.example` 的项

这些值仍然可能由部署环境控制，但不是“默认必须写进示例文件”的项。

| 配置项 | 敏感 | 说明 | 原因 |
|--------|------|------|------|
| `PORT` | 否 | API 监听端口 | 有默认值 `8080`，属于进程启动参数 |
| `AUTO_MIGRATE` | 否 | 是否自动迁移数据库 | 有默认值 `false`，属于启动期控制项 |
| `ADMIN_USERNAME` | 否 | 首次初始化管理员用户名 | 仅首次启动且需要初始化管理员时才有意义 |
| `ADMIN_PASSWORD` | 是 | 首次初始化管理员密码 | 仅首次启动且需要初始化管理员时才有意义 |

说明：

- `WEBHOOK_TOKEN` 已废弃，当前只保留 `EMBY_WEBHOOK_TOKEN`。
- `CONFIG_ENCRYPTION_KEY` 不参与认证，它只负责数据库敏感配置的加密和解密。
- `EMBY_URL`、`EMBY_API_KEY`、`TMDB_API_KEY`、`MOVIEPILOT_*`、`SMTP_*`、`CRON_*`、`BOT_NOTIFY_URL`、`TELEGRAM_ADMIN_CHAT_ID`、`TELEGRAM_GROUP_CHAT_ID` 已按设置中心模型管理，不再作为 API `.env.example` 的默认项。

---

## 4. Bot 环境变量

Bot 进程当前仍主要依赖环境变量启动，但 `.env.example` 只保留启动硬依赖。

### 4.1 `.env.example` 默认保留项

| 配置项 | 敏感 | 默认值 | 说明 |
|--------|------|--------|------|
| `TELEGRAM_BOT_TOKEN` | 是 | — | Telegram Bot Token |
| `TELEGRAM_UPDATE_MODE` | 否 | `webhook` | Telegram 更新接入模式，`webhook` 或 `polling` |
| `TELEGRAM_WEBHOOK_SECRET` | 条件敏感 | — | `webhook` 模式下的 Telegram Webhook 校验密钥 |
| `INTERNAL_API_SECRET` | 是 | — | 与 API 共享的内部调用密钥 |
| `WEBHOOK_URL` | 条件敏感 | — | `webhook` 模式下的 Bot 对外 Webhook 地址 |

### 4.2 仍支持环境变量回退，但不放进 `.env.example` 的项

| 配置项 | 敏感 | 默认值 | 说明 |
|--------|------|--------|------|
| `TELEGRAM_ADMIN_CHAT_ID` | 否 | — | 管理员 Chat ID；运行期优先读 API 设置中心，env 仅作回退 |
| `TELEGRAM_GROUP_CHAT_ID` | 否 | — | 群组 Chat ID；运行期优先读 API 设置中心，env 仅作回退 |
| `API_URL` | 否 | `http://localhost:8080` | API 地址；有默认值，不必写进示例 |
| `BOT_PORT` | 否 | `8000` | Bot 监听端口；有默认值，不必写进示例 |

说明：

- Bot 在运行期会通过 API 内部接口读取 `TELEGRAM_ADMIN_CHAT_ID`、`TELEGRAM_GROUP_CHAT_ID`、`notify_group_link` 和 `telegram_welcome_message_template`，并做短 TTL 缓存。
- 当 API 未返回值时，Bot 会回退到本地环境变量中的 Chat ID。
- `polling` 模式下不再需要 Telegram 使用的公网域名和 HTTPS 回调入口，但 Bot 自身仍要保留内网可达地址，供 API 通过 `BOT_NOTIFY_URL` 调用 `/notify/*`。
- 因此 `TELEGRAM_ADMIN_CHAT_ID`、`TELEGRAM_GROUP_CHAT_ID` 对 Bot 来说仍可保留 env 兜底，但不再属于推荐写进示例文件的默认项。

---

## 5. 关键密钥说明

### `JWT_SECRET`

- 用途：签发和校验用户登录 JWT
- 影响面：所有前台/后台用户登录态
- 备注：修改后，旧登录态会全部失效

### `INTERNAL_API_SECRET`

- 用途：API 与 Bot 之间的内部接口鉴权
- 使用方式：`X-Internal-Secret` 请求头
- 备注：这是服务间信任根，不应放进设置中心

### `STRIPE_WEBHOOK_SECRET`

- 用途：校验 Stripe Webhook 签名
- 备注：这是 Stripe 外部平台定义的签名密钥，不应与其他密钥复用

### `EMBY_WEBHOOK_TOKEN`

- 用途：保护 `/api/v1/webhooks/emby?token=...`
- 备注：这是项目内自定义的 Webhook 口令，不参与 JWT 或内部服务鉴权

### `CONFIG_ENCRYPTION_KEY`

- 用途：加密/解密数据库中的敏感配置
- 影响面：`EMBY_API_KEY`、`SMTP_PASSWORD`、`STRIPE_SECRET_KEY` 等敏感值的持久化
- 备注：如果该值缺失或变更错误，数据库里已有的敏感配置会无法解密

---

## 6. 运维建议

1. 修改数据库配置前，先确认 `CONFIG_ENCRYPTION_KEY` 已正确注入。
2. 修改调度配置后，记得重启 API。
3. 不要把 `JWT_SECRET`、`INTERNAL_API_SECRET`、`STRIPE_WEBHOOK_SECRET`、`CONFIG_ENCRYPTION_KEY` 合并成同一个值。
4. Bot 部署时，应显式提供 `INTERNAL_API_SECRET`、`TELEGRAM_BOT_TOKEN`；若使用 `webhook` 模式，还需额外提供 `TELEGRAM_WEBHOOK_SECRET` 和 `WEBHOOK_URL`。
5. 如果需要把旧环境变量导入数据库，优先使用设置中心现有的导入能力，而不是手工改表。
