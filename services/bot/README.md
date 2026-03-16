# Ember Telegram Bot

Telegram 通知服务，负责两类能力：

1. 通知能力：接收 Go API 的通知并推送到 Telegram（订阅审批、新用户注册、播放排行榜）  
2. 自助能力：接收 Telegram 用户命令并调用 Go API 内部接口（账号绑定、信息查询、兑换续期）

## 目录结构

```
services/bot/
├── main.py
├── app/
│   ├── __init__.py
│   ├── config.py
│   ├── server.py
│   ├── clients/
│   │   ├── __init__.py
│   │   └── api_client.py
│   ├── handlers/
│   │   ├── __init__.py
│   │   └── telegram_handler.py
│   └── formatters/
│       ├── __init__.py
│       └── message_formatter.py
├── requirements.txt
└── Dockerfile
```

## 环境变量

必填：

- `TELEGRAM_BOT_TOKEN`
- `TELEGRAM_WEBHOOK_SECRET`
- `INTERNAL_API_SECRET`
- `WEBHOOK_URL`

可选：

- `TELEGRAM_ADMIN_CHAT_ID`（管理员通知目标；运行期优先读取 API 设置中心，env 仅作回退）
- `TELEGRAM_GROUP_CHAT_ID`（用于播放排行榜等群推送；未配置时会回退推送到管理员）
- `API_URL`（默认 `http://localhost:8080`）
- `BOT_PORT`（默认 `8000`）

## 配置分层

Bot 当前配置分成两层：

1. 启动期配置（必须来自 env）
   - `TELEGRAM_BOT_TOKEN`
   - `TELEGRAM_WEBHOOK_SECRET`
   - `INTERNAL_API_SECRET`
   - `WEBHOOK_URL`
   - `API_URL`
   - `BOT_PORT`

2. 运行期设置（优先从 API 设置中心读取）
   - `TELEGRAM_ADMIN_CHAT_ID`
   - `TELEGRAM_GROUP_CHAT_ID`
   - `notify_group_link`

运行期设置会通过 Go API 的 Internal API 获取，并带短 TTL 缓存；当 API 未返回值时，回退到本地 env。

## 本地运行

```bash
cd services/bot
pip install -r requirements.txt
# 首次本地调试建议使用本地配置文件
cp .env.example .env.local
# 编辑 .env.local 填入本地测试配置
python main.py
```

配置优先级说明：
- 系统环境变量优先（Docker/生产环境）
- `.env.local` 次之（本地调试）
- `.env` 兜底兼容（可选）

## HTTP 端点

- `GET /health`：健康检查
- `POST /notify/subscription`：Go API 通知入口（需 `X-Internal-Secret`）
- `POST /notify/registration`：Go API 注册通知入口（需 `X-Internal-Secret`）
- `POST /notify/payment`：Go API 支付成功通知入口（需 `X-Internal-Secret`）
- `POST /notify/ranking`：Go API 播放排行榜通知入口（需 `X-Internal-Secret`）
- `POST /telegram/webhook`：Telegram webhook 入口

## Telegram 用户命令

- `/bind <6位验证码>`：绑定 Telegram 与 Ember 账号
- `/info`：查看当前绑定账号信息（用户名、邮箱、状态、有效期）
- `/redeem <兑换码>`：为当前绑定账号兑换续期
- `/refresh_menu`：群聊中由群管理员或配置的管理员账号强制刷新当前群菜单作用域

## 命令菜单同步

Bot 启动时会主动同步 Telegram 命令菜单，而不是依赖 BotFather 的历史配置：

- 先清理 default scope、群聊以及已知管理员/群组 chat scope 的旧命令
- 只重新写入当前私聊命令菜单，不向 default scope 写命令
- 如果历史上某个群被设置过更具体的旧作用域菜单，Bot 首次收到该群消息时会按群清理一次，并在当前进程内缓存该群已同步状态
- 群管理员也可以在群内执行 `/refresh_menu` 手动强刷当前群菜单
- 如果 Bot 在该群里不是管理员，则无法可靠识别普通群管理员；这种情况下仅配置的管理员账号可用 `/refresh_menu`

如果菜单仍未更新，优先检查：

1. 是否部署了最新 Bot 代码
2. `TELEGRAM_ADMIN_CHAT_ID` / `TELEGRAM_GROUP_CHAT_ID` 是否正确
3. 启动日志里是否出现“Bot 命令菜单已同步”

约束：
- 以上命令仅支持私聊 Bot 使用
- 群聊触发会提示用户改为私聊
- 群聊默认不展示命令菜单；`/refresh_menu` 仅用于管理员维护群菜单状态
- Bot 重启后，同一群可能会再次触发一次群菜单清理，这是当前轻量方案的预期行为
- `/refresh_menu` 强刷时会额外重试清理 `default scope` 与 `all group scope`，用于修复启动期全局清理失败后的继承问题

## 绑定流程

1. 用户在 Ember 网站控制台点击“生成绑定验证码”
2. 网站调用 `POST /api/v1/telegram/bindcode` 返回 6 位验证码（5 分钟有效）
3. 用户向 Bot 发送 `/bind 123456`
4. Bot 调用 `POST /api/v1/internal/telegram/bind` 完成绑定
5. 绑定成功后可使用 `/info` 与 `/redeem`

## Bot 依赖的 Internal API

除原有订阅审批接口外，Bot 还依赖以下 Go API 内部端点（均需 `X-Internal-Secret`）：

- `POST /api/v1/internal/telegram/bind`
- `POST /api/v1/internal/telegram/info`
- `POST /api/v1/internal/telegram/redeem`
