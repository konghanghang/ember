# Ember Bot

Telegram Bot 服务，负责两类能力：

1. 接收 Go API 通知并推送到 Telegram
2. 接收 Telegram 用户命令并调用 Go API Internal API

## 本地验证

```bash
cd services/bot
python3.11 -m venv .venv
source .venv/bin/activate
pip install -r requirements-dev.txt
python -m py_compile main.py
python -m pytest tests
```

也可以从仓库根目录直接跑：

```bash
services/bot/.venv/bin/python -m pytest services/bot/tests
```

约定：

- Bot 的本地运行和测试默认都使用 `services/bot/.venv`
- 如果 `.venv` 不存在，先执行 `python3.11 -m venv .venv`
- 仓库根的 `make setup`、`make test-bot`、`make test-bot-report` 也依赖这个虚拟环境路径

本地运行：

```bash
python main.py
```

## 目录骨架

```text
services/bot/
├── main.py
├── app/config.py              # 启动期配置
├── app/runtime_settings.py    # 运行期设置读取与缓存
├── app/server.py              # FastAPI + Telegram Application
├── app/clients/api_client.py  # Go API 内部客户端
├── app/handlers/              # Telegram 命令与回调处理
├── app/menu_sync.py           # 命令菜单同步
└── app/formatters/            # 消息格式化
```

## 核心环境变量

本地开发可从 [`.env.example`](./.env.example) 起步：

```bash
cp .env.example .env.local
```

`.env.example` 只保留 Bot 启动时必须依赖环境变量的项。

默认必填：

- `TELEGRAM_BOT_TOKEN`
- `INTERNAL_API_SECRET`

按模式必填：

- `TELEGRAM_UPDATE_MODE`，默认 `webhook`
- `TELEGRAM_WEBHOOK_SECRET`，仅 `webhook` 模式必填
- `WEBHOOK_URL`，仅 `webhook` 模式必填

仍支持环境变量回退，但不再放进 `.env.example` 的项：

- `TELEGRAM_ADMIN_CHAT_ID`、`TELEGRAM_GROUP_CHAT_ID`
- `API_URL`，默认 `http://localhost:8080`
- `BOT_PORT`，默认 `8000`

其中 `TELEGRAM_ADMIN_CHAT_ID`、`TELEGRAM_GROUP_CHAT_ID` 在运行期优先从 Go API 设置中心读取，env 只作为兜底。

配置边界见 [配置参考](/Users/konghang/data/me/github/ember/docs/reference/configuration-reference.md)。

## 当前职责

### 通知入口

- `POST /notify/subscription`
- `POST /notify/registration`
- `POST /notify/payment`
- `POST /notify/ranking`

以上入口都要求 `X-Internal-Secret`。

### Telegram 更新入口

- `POST /telegram/webhook`
- `GET /health`

### 用户命令

- `/bind`
- `/info`
- `/libraries`
- `/redeem`
- `/resetpw`
- `/search`（电影直接确认订阅，电视剧先选季再确认）
- `/refresh_menu`

## 运行期设置

Bot 启动期配置来自环境变量；运行期设置优先从 Go API Internal API 读取，并带短 TTL 缓存。当前重点依赖：

- `TELEGRAM_ADMIN_CHAT_ID`
- `TELEGRAM_GROUP_CHAT_ID`
- `notify_group_link`

## 更新模式

- `webhook`：默认模式，需要公网 HTTPS 地址供 Telegram 回调
- `polling`：Bot 主动从 Telegram 拉取更新，不再需要 Telegram 使用的公网域名
- 无论哪种模式，Bot 都会继续保留 `/notify/*` HTTP 入口，供 Ember API 通过服务名或内网地址推送通知
- `polling` 只适合单实例部署。当前实现会在启动前通过 API Internal 路由申请数据库租约锁；拿不到锁的实例直接拒绝启动，续租失败的实例会主动停止 polling

## 菜单与群行为

- 私聊命令菜单由 Bot 启动时主动同步
- 群聊默认不展示命令菜单
- `/refresh_menu` 用于管理员强制刷新当前群菜单状态
- 新成员欢迎消息依赖 `notify_group_link`

更细的菜单历史和设计背景已经归档，不再放在服务入口文档里。

## 依赖的 Internal API

- `POST /api/v1/internal/telegram/bind`
- `POST /api/v1/internal/telegram/info`
- `POST /api/v1/internal/telegram/media-libraries`
- `PUT /api/v1/internal/telegram/media-libraries/:libraryId/toggle`
- `DELETE /api/v1/internal/telegram/media-libraries/preferences`
- `POST /api/v1/internal/telegram/redeem`
- `POST /api/v1/internal/telegram/reset-password`
- `POST /api/v1/internal/telegram/subscribe`
- `GET /api/v1/internal/tmdb/search`
- `GET /api/v1/internal/tmdb/tv/:id/seasons`
- `PUT /api/v1/internal/subscriptions/:id/approve`
- `PUT /api/v1/internal/subscriptions/:id/reject`

## 相关文档

- [系统架构](/Users/konghang/data/me/github/ember/docs/system-architecture.md)
- [服务入口 README](/Users/konghang/data/me/github/ember/services/api/README.md)
- [Cloudflared 本地联调](/Users/konghang/data/me/github/ember/docs/runbooks/cloudflared-local-testing.md)
- [部署指南](/Users/konghang/data/me/github/ember/docs/runbooks/deployment.md)
