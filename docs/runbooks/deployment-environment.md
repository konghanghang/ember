# 部署环境与配置

这份文档只负责解释部署时必须准备什么，不重复写启动命令。

## Compose 事实

当前 [`docker-compose.yml`](../../infrastructure/docker/docker-compose.yml) 默认会启动四个服务：

- `postgres`
- `ember-api`
- `ember-web`
- `ember-bot`

这意味着：

- 如果你不准备启用 Telegram Bot，不能假装相关配置不存在
- 要么补齐 Bot 所需变量
- 要么在部署前手动注释 `ember-bot` 服务

## 必填变量

### 基础设施与 API 启动期变量

这些变量不完整，API 基本起不来；它们也是当前 compose 明确注入给 `ember-api` 的启动边界变量：

| 变量 | 作用 |
|------|------|
| `DATABASE_URL` | PostgreSQL 连接串 |
| `JWT_SECRET` | JWT 签名密钥 |
| `CONFIG_ENCRYPTION_KEY` | 设置中心敏感值加密主密钥 |
| `ADMIN_PASSWORD` | 首次启动管理员初始化密码 |
| `EMBY_WEBHOOK_TOKEN` | Emby Webhook 验签口令 |
| `INTERNAL_API_SECRET` | API 与 Bot 的内部调用共享密钥 |

### Bot 与 Webhook

默认 compose 会启动 `ember-bot`，因此这组变量也应视为必填：

| 变量 | 作用 |
|------|------|
| `TELEGRAM_BOT_TOKEN` | Telegram Bot Token |
| `TELEGRAM_WEBHOOK_SECRET` | Bot Webhook 验签密钥 |
| `WEBHOOK_URL` | Telegram 对外回调地址 |

## 常用可选变量

| 变量 | 默认值 / 说明 |
|------|---------------|
| `ADMIN_USERNAME` | 默认 `admin` |
| `AUTO_MIGRATE` | 默认 `true`，API 启动时自动迁移 |
| `TELEGRAM_ADMIN_CHAT_ID` | 管理员通知目标 |
| `TELEGRAM_GROUP_CHAT_ID` | 群推送目标，未填时回退到管理员 |
| `TMDB_API_KEY` | TMDB 搜索、追剧日历依赖 |
| `TURNSTILE_SECRET_KEY` | 登录 Turnstile 服务端校验密钥 |
| `MOVIEPILOT_URL` / `MOVIEPILOT_API_KEY` | 求片审批同步到 MoviePilot 时需要 |
| `CRON_ENABLED` | API 内置 Cron 开关 |
| `RANKING_CRON_ENABLED` | 播放排行 Cron 开关 |
| `RANKING_DAILY_SCHEDULE` / `RANKING_WEEKLY_SCHEDULE` | 排行定时表达式 |
| `TV_CALENDAR_SYNC_SCHEDULE` | 追剧日历同步表达式 |

更完整的配置边界见 [配置参考](../reference/configuration-reference.md)。

## 首次启动后必须补齐的运行期配置

下列配置不再依赖 compose 直接注入 `ember-api`，而是由设置中心数据库托管：

- `EMBY_URL`
- `NEXT_PUBLIC_EMBY_URL`
- `EMBY_API_KEY`
- `TMDB_API_KEY`
- `MOVIEPILOT_URL`
- `MOVIEPILOT_API_KEY`
- `BOT_NOTIFY_URL`
- `CRON_*`
- `RANKING_*`
- `TV_CALENDAR_*`

这意味着：

- 新环境启动后，API 可以先起来；
- 但媒体、MoviePilot、Bot fire-and-forget、追剧日历和调度能力，要在设置中心补齐配置后才会真正可用；
- 不要再把这些项当成 compose 自动注入的前提变量。

## `.env.example` 的已知缺口

当前 [`infrastructure/docker/.env.example`](../../infrastructure/docker/.env.example) 只覆盖启动期环境变量，不是“所有运行期能力都能开箱即用”的完整模板。

如果你希望首次启动后直接使用媒体相关能力，还需要额外准备一份设置中心初始化方案或在后台手动补齐。

另外，模板里至少缺少：

- `DATABASE_URL`
- `ADMIN_USERNAME`
- `ADMIN_PASSWORD`
- `NEXT_PUBLIC_EMBY_URL`

部署前必须手动补齐，别把模板当真相来源。

## 数据库迁移策略

### 空数据库首次部署

当前 compose 会把 `infrastructure/database/` 挂载到 PostgreSQL 的 `/docker-entrypoint-initdb.d`：

- 仅当数据库数据卷为空时自动执行一次
- 适合全新环境初始化
- 当前顶层入口是 `20260415_00_schema_baseline.sql` + baseline 之后的增量 migration

如果同时保留 `AUTO_MIGRATE=true`，API 启动后还会执行模型自动迁移；这对开发环境和新环境通常没问题。

### 已有数据库升级

生产环境或已有数据库升级时，推荐改成显式迁移：

1. 阅读 [`infrastructure/database/README.md`](../../infrastructure/database/README.md)
2. 只手动执行 README 指定的顶层可执行 SQL
3. 评估是否关闭 `AUTO_MIGRATE`
4. 再启动或重启 API

不要指望“改了 compose 就自动补所有历史迁移”。

截至 `2026-04-15`，baseline 之后暂无新增 migration；已经在当前版本的数据库无需额外执行 SQL。

## 管理员初始化

API 启动时会检查是否已有 `role=admin` 的用户：

- 已存在：admin 初始化自动跳过
- 不存在：读取 `ADMIN_USERNAME` 和 `ADMIN_PASSWORD` 创建首个管理员

注意：

- `ADMIN_USERNAME` 不填时默认 `admin`
- `ADMIN_PASSWORD` 不填时会跳过初始化，并在日志中给出警告
- 该流程是幂等的，多次重启不会重复创建管理员

## 启用可选功能前的最低要求

### TMDB / 追剧日历 / 搜索

- `TMDB_API_KEY`
- Emby 可访问

### 登录 Turnstile 保护

- API 环境变量：
  - `TURNSTILE_SECRET_KEY`
- API 设置中心运行期配置：
  - `turnstile_login_enabled`
  - `turnstile_site_key`
  - `turnstile_expected_hostname`

说明：

- `turnstile_site_key` 是前端公开值，走设置中心即可。
- `TURNSTILE_SECRET_KEY` 是服务端校验密钥，必须保留在环境变量，不进入设置中心。

### Telegram Bot

- `TELEGRAM_BOT_TOKEN`
- `TELEGRAM_WEBHOOK_SECRET`
- `WEBHOOK_URL`
- `INTERNAL_API_SECRET`

本地联调见 [Cloudflared 本地联调](./cloudflared-local-testing.md)。

### MoviePilot

- `MOVIEPILOT_URL`
- `MOVIEPILOT_API_KEY`

未配置时，相关同步能力应视为关闭，而不是“自动降级成功”。

## 启动前自检

- `DATABASE_URL` 已指向可访问的 PostgreSQL
- 所有密钥都不是示例值
- `NEXT_PUBLIC_EMBY_URL` 指向用户真实可访问的 Emby 地址
- 如果保留 `ember-bot` 服务，Telegram 相关变量已补齐
- 如果是升级环境，手动 SQL 已执行完毕

## 相关文档

- [部署指南](./deployment.md)
- [部署排障](./deployment-troubleshooting.md)
- [配置参考](../reference/configuration-reference.md)
- [数据库迁移说明](../../infrastructure/database/README.md)
