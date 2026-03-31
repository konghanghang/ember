# Telegram Bot Polling 模式实现方案（已落地）

> 状态：已归档（已落地）
> 负责人：Ember
> 更新时间：2026-03-29

## 背景

当前 Bot 只支持 Telegram `webhook` 模式，部署时必须提供公网 HTTPS 域名，并由 Telegram 主动回调 Bot。

- 这增加了本地调试和低成本部署门槛，尤其是在只需要 Bot 主动拉取更新时。
- Bot 目前同时承担两类职责：接收 Telegram 更新、接收 Ember API 的内部通知；其中只有前者强依赖公网 `webhook`。
- 在当前部署边界下，公网域名只是为了让 Telegram 能回调 Bot，并不是 API 与 Bot 通信所必需。
- 如果继续把“收 Telegram 更新”和“收内部通知”绑定在单一 `webhook` 配置上，后续接入方式切换会持续污染启动边界。

## 目标

本方案要实现：

1. 为 `services/bot` 增加可选的 Telegram `polling` 模式。
2. 默认保持现有 `webhook` 行为不变，避免影响已部署环境。
3. 将 Telegram 更新接入方式收敛为独立启动边界，不影响现有 `/notify/*` 内部通知入口。

## 非目标

本次明确不做：

- 不重写现有 FastAPI 服务结构，不拆分成多进程或多服务。
- 不修改 Bot 命令处理器、Internal API 协议和通知消息格式。
- 不解决多实例 `polling` 消费协调问题；`polling` 默认只面向单实例部署。

## 当前事实

以当前代码和现行文档为准，现状如下：

- 相关文档：
  [docs/system-architecture.md](/Users/konghang/data/project/github/ember/docs/system-architecture.md)
  [services/bot/README.md](/Users/konghang/data/project/github/ember/services/bot/README.md)
- 相关服务：
  [services/bot/app/server.py](/Users/konghang/data/project/github/ember/services/bot/app/server.py)
  [services/bot/app/config.py](/Users/konghang/data/project/github/ember/services/bot/app/config.py)
  [services/bot/main.py](/Users/konghang/data/project/github/ember/services/bot/main.py)
- 当前行为：Bot 启动后初始化 `python-telegram-bot` 的 `Application`，同步命令菜单，并异步注册 `set_webhook`；Telegram 更新通过 `POST /telegram/webhook` 进入。
- 当前行为：Go API 通过 `POST /notify/subscription`、`/notify/registration`、`/notify/payment`、`/notify/ranking` 将业务通知推给 Bot。
- 现有限制：`WEBHOOK_URL` 与 `TELEGRAM_WEBHOOK_SECRET` 被视为启动期必填项，导致即使只想使用 `polling`，当前实现也无法启动。

## 方案设计

### 1. 用户可见行为

- 新增能力：Bot 支持通过配置在 `webhook` 与 `polling` 两种 Telegram 更新接入模式之间切换。
- 保持不变：现有命令、回调按钮、欢迎消息、审批流程、内部通知入口、健康检查接口保持不变。
- 兼容性约束：未显式切换模式时，继续走当前 `webhook` 模式，避免影响现有部署和现网文档习惯。

### 2. 数据与模型

> 本次不涉及数据模型变更。

### 3. 接口与边界

- 新增启动期配置：`TELEGRAM_UPDATE_MODE=webhook|polling`
  - 默认值：`webhook`
  - 作用：决定 Bot 从 Telegram 接收更新的方式
- 配置约束调整：
  - `webhook` 模式下：`WEBHOOK_URL`、`TELEGRAM_WEBHOOK_SECRET` 继续必填
  - `polling` 模式下：`WEBHOOK_URL` 非必填，`TELEGRAM_WEBHOOK_SECRET` 仅用于兼容保留，不参与主链路
- HTTP 接口保持：
  - `GET /health`
  - `POST /notify/*`
  - `POST /telegram/webhook`
- 边界约束：
  - `polling` 模式切换完成后，可以移除面向 Telegram 的公网域名解析、HTTPS 反向代理和 `webhook` 暴露入口；这些能力只服务于 Telegram 回调。
  - API 与 Bot 的通信边界保持为内网 HTTP 服务，部署上继续通过服务名访问 Bot，例如容器网络中的 `http://bot:8000`。
  - `POST /telegram/webhook` 在 `polling` 模式下不作为主链路，但接口保留，避免反向代理、监控或旧部署脚本立刻失效
  - `polling` 模式启动时主动执行 `deleteWebhook`，清理 Telegram 侧旧配置，避免更新仍被投递到旧地址

### 4. 关键流程

#### 4.1 `webhook` 模式

1. 读取配置，判定 `TELEGRAM_UPDATE_MODE=webhook`。
2. 初始化 `tg_app`、HTTP client、命令菜单同步逻辑。
3. 启动 FastAPI 服务。
4. 在生命周期中异步注册 `set_webhook(url, secret_token)`。
5. Telegram 将更新推送到 `POST /telegram/webhook`，Bot 复用现有 `process_update` 处理。

#### 4.2 `polling` 模式

1. 读取配置，判定 `TELEGRAM_UPDATE_MODE=polling`。
2. 初始化 `tg_app`、HTTP client、命令菜单同步逻辑。
3. 启动 FastAPI 服务，仅承担 `/notify/*` 和 `/health`。
4. 生命周期内先调用 `deleteWebhook(drop_pending_updates=False)` 清理 Telegram 旧 `webhook`。
5. 启动后台 `polling` 任务持续拉取 Telegram 更新，并复用现有 handler 处理。
6. 服务退出时停止 `polling` 任务，再关闭 `tg_app` 与 API client。

### 5. 失败路径与边界条件

- `webhook` 模式缺少 `WEBHOOK_URL` 或 `TELEGRAM_WEBHOOK_SECRET`：启动期直接失败，保持当前显式报错行为。
- `polling` 模式下 `deleteWebhook` 失败：记录告警并按失败退出处理，不继续带着不确定状态启动，避免出现 Telegram 仍在投递旧 `webhook`。
- `polling` 任务异常退出：记录错误并终止进程，由部署层重启；不做静默吞错。
- 旧部署仍访问 `POST /telegram/webhook`：接口保留，但在 `polling` 模式下不再承担主链路，日志中应明确提示当前模式，便于排障。
- 去掉公网域名后：Bot 仍必须对 Ember API 所在网络可达；如果 API 失去对 Bot 服务名或内网地址的访问能力，`/notify/*` 通知链路会直接失效。
- 多实例部署：`polling` 模式不保证并发实例安全，文档中必须明确“仅单实例”约束，避免重复消费或更新竞争。
- 兼容性约束：`/notify/*` 入口、`X-Internal-Secret` 校验、Internal API 调用、菜单同步逻辑都不能被这次改动破坏。

## 影响范围

涉及的子系统：

- API：无接口变更，现有 `BotNotifier` 继续使用 Bot HTTP 通知入口。
- Web：无。
- Bot：有，涉及启动配置、生命周期管理、Telegram 更新接入方式。
- 配置/部署：有，需要新增 `TELEGRAM_UPDATE_MODE`，并按模式调整 `WEBHOOK_URL` 的必填约束；切到 `polling` 后可移除 Telegram 用的公网域名解析，但 Bot 服务仍需保留内网可达地址供 API 通过服务名访问。
- 文档：落地后需要更新
  [docs/system-architecture.md](/Users/konghang/data/project/github/ember/docs/system-architecture.md)
  [services/bot/README.md](/Users/konghang/data/project/github/ember/services/bot/README.md)
  如存在配置参考文档，也要同步补充模式说明和部署约束。

## 验证方式

### 编译/测试

- `cd services/bot && python -m py_compile main.py app/*.py app/handlers/*.py app/clients/*.py`

### 手工验证

- `webhook` 模式下启动 Bot，确认仍会注册 `set_webhook`，并且 `POST /telegram/webhook` 可以正常处理 `/bind`、`/info` 等命令。
- `polling` 模式下启动 Bot，确认无需 `WEBHOOK_URL` 也能启动，且 Telegram 私聊命令能被正常消费。
- `polling` 模式下确认 Go API 仍可通过 `/notify/subscription` 等入口推送通知。
- `polling` 模式部署为内网服务后，确认 API 能通过服务名访问 Bot，例如容器网络内 `http://bot:8000` 可正常承接 `/notify/*`。
- 从 `webhook` 切换到 `polling` 后，确认 Telegram 侧旧 `webhook` 已被清理，不再向旧地址投递更新。
- 异常验证：`polling` 模式下模拟 Telegram 不可达或 `deleteWebhook` 失败，确认日志和退出行为符合预期。

## 落地后文档处理

落地后应同步处理：

- 将“Bot 支持 `webhook/polling` 双模式，默认 `webhook`，`polling` 仅支持单实例”的稳定结论同步到
  [docs/system-architecture.md](/Users/konghang/data/project/github/ember/docs/system-architecture.md)
  和
  [services/bot/README.md](/Users/konghang/data/project/github/ember/services/bot/README.md)。
- 若配置参考文档已记录 Bot 环境变量，补充 `TELEGRAM_UPDATE_MODE` 与模式差异。
- 功能落地、编译验证、文档同步完成后，将本方案移入 `docs/archive/plan/`。
