# Ember API

Go API 服务，负责认证、用户生命周期、兑换码、订阅、支付、媒体能力、设置中心和 Bot 内部接口。

## 入口与验证

- 入口：`cmd/server/main.go`
- 默认端口：`8080`
- 健康检查：`GET /health`

最小验证：

```bash
cd services/api
go vet ./...
go test ./...
go build ./...
```

## 环境变量

本地开发可从 [`.env.example`](./.env.example) 起步：

```bash
cp .env.example .env
```

本地启动或执行迁移时会自动读取当前目录下的 `.env`；从仓库根目录启动时，也会读取 `services/api/.env`。例如：

```bash
go run cmd/server/main.go
```

`.env.example` 现在只保留“必须通过环境变量提供”的项。

默认保留的核心项是：

- `DATABASE_URL`
- `JWT_SECRET`
- `CONFIG_ENCRYPTION_KEY`
- `INTERNAL_API_SECRET`

按功能启用、且只能走环境变量注入的项：

- `STRIPE_WEBHOOK_SECRET`
- `TURNSTILE_SECRET_KEY`
- `EMBY_WEBHOOK_TOKEN`

以下配置虽然代码仍可能支持环境变量来源，但不再建议写进 `.env.example`：

- `PORT`
- `ADMIN_USERNAME`、`ADMIN_PASSWORD`
- `EMBY_URL`、`EMBY_API_KEY`、`TMDB_API_KEY`
- `MOVIEPILOT_*`、`SMTP_*`、`CRON_*`
- `BOT_NOTIFY_URL`、`TELEGRAM_ADMIN_CHAT_ID`、`TELEGRAM_GROUP_CHAT_ID`

因为这些值当前默认应由设置中心数据库托管，或者本身有默认值，只在特定部署场景下才需要额外注入。

更完整的部署边界见 [配置参考](/Users/konghang/data/me/github/ember/docs/reference/configuration-reference.md)。

## 目录骨架

```text
services/api/
├── cmd/server/                # 进程入口
├── internal/app/              # 启动装配、路由、cron
├── internal/config/           # 配置定义与解析
├── internal/db/               # 数据库初始化、启动期自动迁移（migrate.go）、VerifySchema、Bootstrap
├── internal/models/           # GORM 模型
├── internal/integrations/     # Emby / MoviePilot / BotNotifier
├── internal/services/         # 业务服务
├── internal/handlers/         # HTTP 处理层
├── internal/middleware/       # JWT / InternalAuth
└── Dockerfile
```

目录约束与拆分规则见 [API 开发与目录规范](/Users/konghang/data/me/github/ember/docs/reference/api-development-conventions.md)。

## 当前职责

- 公开接口：登录、注册、忘记密码、Stripe Webhook、Emby Webhook、TMDB 搜索
- 统一认证接口：个人信息、兑换、媒体统计、最近入库、排行、支付、Telegram 绑定码、追剧日历
- 管理员接口：用户、兑换码、配置中心、订阅、会话、播放历史、媒体质量、设备、方案、支付、cron、Emby 账号自助绑定
- 内部接口：Bot 绑定/查询/兑换/重置密码/订阅，订阅审批

完整接口面以 [系统架构文档](/Users/konghang/data/me/github/ember/docs/system-architecture.md) 为准。

## 相关文档

- [系统架构](/Users/konghang/data/me/github/ember/docs/system-architecture.md)
- [API 响应规范](/Users/konghang/data/me/github/ember/docs/reference/api-response-standard.md)
- [开发指南](/Users/konghang/data/me/github/ember/docs/reference/development-guide.md)
- [部署指南](/Users/konghang/data/me/github/ember/docs/runbooks/deployment.md)
- [API_GUIDE.md](./API_GUIDE.md)
