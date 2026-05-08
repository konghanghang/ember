# Ember Bot 架构参考

> 本文档承接 Ember Telegram Bot 当前的运行模式、通信边界、端点、命令处理器和环境变量语义。
> Bot 与 API 之间的服务边界、错误语义和链路约束仍应结合 `docs/system-architecture.md` 的高层说明一起理解。

## 1. 技术栈

- Python 3.11 + python-telegram-bot（支持 `webhook` / `polling` 双模式，默认 `webhook`）
- FastAPI 作为 HTTP 服务器（接收 API 通知；`webhook` 模式下同时接收 Telegram Webhook）
- 与 Go API 通过 `X-Internal-Secret` 双向通信

## 2. 通信模式

```
用户操作 → Go API → BotNotifier（火忘式 POST）→ Bot FastAPI → Telegram Bot → 发送消息
Telegram 用户操作 → Telegram → Bot Webhook → Bot 处理 → 调用 Go Internal API → 返回结果
```

`polling` 模式下第二条链路改为：

```
Telegram 用户操作 → Telegram → Bot Polling → Bot 处理 → 调用 Go Internal API → 返回结果
```

## 3. Bot 端点

| 端点 | 用途 |
|------|------|
| `GET /health` | 健康检查 |
| `POST /telegram/webhook` | Telegram Webhook 入口 |
| `POST /notify/subscription` | 接收新订阅通知 |
| `POST /notify/registration` | 接收新注册通知 |
| `POST /notify/payment` | 接收支付成功通知 |
| `POST /notify/ranking` | 接收排行榜通知 |

## 4. 命令与处理器

- **CallbackQuery**：订阅审批按钮（approve/reject → 调用 Internal API）
- **NewChatMembers**：群组欢迎消息（读取 `notify_group_link` 与 `telegram_welcome_message_template` 配置）
- **Commands**：`/search`（搜索影视并订阅；电影直接确认，电视剧先选季再确认）、`/bind`（绑定账号）、`/info`（查看账号信息）、`/redeem`（兑换续期码）、`/resetpw`（重置密码）、`/refresh_menu`（管理员强制刷新当前群菜单）
- **群菜单策略**：仅私聊作用域写入命令菜单；default/group scope 保持为空，群聊默认不展示命令菜单，首次收到群消息时按群清理旧作用域菜单，并在当前 Bot 进程内缓存已同步群；`/refresh_menu` 强刷会额外重试清理 default / all-group 作用域
- **通知格式化**：`message_formatter.py` 统一格式化 Telegram 消息（HTML 模式）；`format_payment_message` 不再渲染 `email` / `stripeSessionId`，admin 通知载荷已在 API 侧脱敏（详见 `docs/system-architecture.md` §5.14）

## 5. 环境变量

| 变量 | 必需 | 默认值 | 说明 |
|------|------|--------|------|
| `TELEGRAM_BOT_TOKEN` | ✅ | — | Bot Token（@BotFather 获取）|
| `TELEGRAM_UPDATE_MODE` | — | `webhook` | Telegram 更新接入模式：`webhook` 或 `polling` |
| `TELEGRAM_ADMIN_CHAT_ID` | — | — | 管理员 Chat ID；可被设置中心数据库值覆盖，env 仅作兜底 |
| `TELEGRAM_GROUP_CHAT_ID` | — | — | 群组 Chat ID（排行榜推送）；可被设置中心数据库值覆盖，env 仅作兜底 |
| `TELEGRAM_WEBHOOK_SECRET` | 条件必需 | — | `webhook` 模式下用于 Webhook 签名校验 |
| `INTERNAL_API_SECRET` | ✅ | — | 与 Go API 共享密钥 |
| `WEBHOOK_URL` | 条件必需 | — | `webhook` 模式下的公开 HTTPS Webhook URL |
| `API_URL` | — | `http://localhost:8080` | Ember API 地址 |
| `BOT_PORT` | — | `8000` | Bot 服务端口 |

说明：

- Bot 在运行期通过 Internal API 读取 `TELEGRAM_ADMIN_CHAT_ID`、`TELEGRAM_GROUP_CHAT_ID`、`notify_group_link` 和 `telegram_welcome_message_template`，并做短 TTL 缓存；刷新失败时保留旧值，不把有效缓存覆盖为空
- 当 API 未返回值时，Chat ID 回退到本地 env
- `polling` 模式下可移除 Telegram 使用的公网域名和 HTTPS 回调入口，但 Bot 仍需保留内网 HTTP 地址供 API 访问 `/notify/*`
- 批次 4 起，Bot 在 `polling` 模式启动前会通过 Internal API 申请 `bot_runtime_locks(name='telegram_polling')` 租约锁，并每 30 秒续租一次；拿不到锁的实例直接拒绝启动，续租失败的实例会主动停止 polling，避免多副本重复消费更新
- `webhook` 模式下注册采用有限重试策略；达到最大重试次数仍失败时，Bot 停止继续重试，`GET /health` 返回 `degraded` 并附带最近错误与重试次数，便于部署侧探活与告警
