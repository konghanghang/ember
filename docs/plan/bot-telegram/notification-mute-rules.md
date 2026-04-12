# Telegram 通知静音规则实现方案

> 状态：草稿
> 负责人：Ember
> 更新时间：2026-04-12

## 背景

这个问题为什么现在要解决：

- Ember 当前 Telegram 通知没有“按事件类型降噪”的治理能力，管理员只能全收或手动忽略。
- 已有注册、订阅、支付、排行榜等多类 Bot 通知，后续还会增加订阅结果、风控等用户侧消息，噪音会继续上升。
- 没有静音规则时，通知越多越容易让真正重要的消息被淹没。

## 目标

本方案要实现：

1. 支持管理员按事件类型配置 Telegram 通知是否发送到管理员私聊或群组。
2. 支持用户为自己的 Telegram 私聊通知配置基础开关，承接后续订阅结果等用户侧通知。
3. 让 Bot 发送前统一经过静音规则判断，而不是在各个 handler 中分散硬编码。

## 非目标

本次明确不做：

- 不实现自由条件表达式、优先级计算或复杂规则引擎。
- 不扩展到邮件、Web Push 等全部通知渠道；首版只覆盖 Telegram。
- 不做消息频率节流算法；本次只做静音开关和事件粒度过滤。

## 当前事实

以当前代码和现行文档为准，先写清现状：

- 相关文档：
  - `docs/system-architecture.md`
  - `docs/plan/media-subscription/subscription-status-and-notification.md`
- 相关服务/页面/模型：
  - `services/api/internal/integrations/notifier/notifier.go`
  - `services/bot/app/handlers/telegram_handler.py`
  - `services/bot/app/server.py`
  - `services/bot/app/runtime_settings.py`
  - `services/web/src/views/admin/SettingsView.vue`
  - `services/web/src/views/console/AccountCenterView.vue`
- 当前行为：
  - 管理员通知包括注册、订阅、支付、排行榜，发送前没有事件级过滤。
  - Bot 运行期只拉取 chat id 和欢迎语等基础设置，不拉取通知静音规则。
  - 用户侧 Telegram 只支持绑定、查询、续期、搜索订阅，没有个人通知偏好设置。
- 现有限制：
  - 管理员无法关闭低价值通知类型。
  - 后续如果给用户推送订阅结果，没有个人偏好入口会很难收口。
  - 通知决策点分散在多个 handler 中，不利于扩展。

## 方案设计

### 1. 用户可见行为

- 新增能力：
  - 管理员可在设置中心配置 Telegram 通知矩阵，按事件类型控制“管理员私聊 / 群组推送”是否启用。
  - 用户可在账号中心配置自己的 Telegram 通知偏好，如“订阅审核结果”“已入库提醒”。
- 修改现有行为：
  - Bot 在发送前统一调用静音规则判断，命中静音则跳过发送。
- 哪些现有行为必须保持不变：
  - 未配置任何规则时，保持当前默认发送行为，不改变现有通知覆盖面。
  - Bot 现有绑定、查询、续期、搜索命令不受影响。
- 前端约束：
  - 前端实现必须遵守 Ember 风格。
  - 设计与交互基线以 `docs/reference/web-design-guide.md` 为准。
  - 管理员配置页应复用现有设置中心交互，不单独发明“通知控制台”新视觉体系。
  - 用户侧偏好入口应落在现有 `AccountCenterView`，避免再开孤立页面。

### 2. 数据与模型

- 新增 `notification_rules` 表：
  - `id`
  - `scopeType`：`admin_chat`、`group_chat`、`user`
  - `scopeRef`：对 `user` 保存 `userId`，系统级 scope 可为空或固定值
  - `channel`：首版固定 `telegram`
  - `eventType`：如 `subscription_created`、`registration_created`、`payment_success`、`ranking_ready`、`subscription_approved`、`subscription_rejected`、`subscription_ingested`
  - `enabled`
  - `createdAt`
  - `updatedAt`
- 是否需要迁移：
  - 需要 SQL migration，放在 `infrastructure/database/`
  - 不改现有 `settings` 表结构

### 3. 接口与边界

- 新增或修改哪些 API / Internal API / webhook / 命令：
  - `GET /api/v1/admin/notification-rules`
  - `PUT /api/v1/admin/notification-rules`
  - `GET /api/v1/profile/notification-rules`
  - `PUT /api/v1/profile/notification-rules`
  - 可选 Internal API：`GET /api/v1/internal/notification-rules`
    - 供 Bot 统一拉取缓存规则
- 请求参数与响应字段怎么变：
  - 管理员接口返回系统级 Telegram 通知矩阵
  - 用户接口只返回当前用户可操作的个人通知事件
- 哪些调用方会受影响：
  - Bot 运行期设置服务
  - 设置中心
  - 账号中心
  - 后续用户侧结果通知链路

### 4. 关键流程

按顺序写清主链路，不需要贴代码：

1. 管理员在设置中心配置 Telegram 系统级静音规则。
2. 用户在账号中心配置自己的 Telegram 通知偏好。
3. Bot 进程按 TTL 缓存从 API 读取通知规则。
4. 当某个通知事件产生时，Bot 先按 `scope + eventType` 判断是否允许发送。
5. 若规则允许，继续发送 Telegram 消息；若命中静音，直接跳过并记录 debug 日志。

### 5. 失败路径与边界条件

- 规则接口不可用：Bot 回退到默认行为，不因规则服务短暂失败导致所有通知中断。
- 规则缓存过期但刷新失败：继续使用最近一次成功缓存，避免抖动。
- 用户未绑定 Telegram：即使个人通知规则为开启，也不发送，主流程不报错。
- 新增事件类型但未配置规则：走默认开启，避免上线后静默失联。
- 兼容性约束：
  - 不能改变现有管理员通知默认覆盖面。
  - 不能把通知静音逻辑散落到各个 handler，必须统一收口。

## 影响范围

涉及的子系统：

- API：有
  - 通知规则模型与读写接口
- Web：有
  - 设置中心和账号中心偏好区
- Bot：有
  - 发送前规则判断与缓存
- 配置/部署：无新增环境变量
- 文档：需要更新
  - `docs/system-architecture.md`

## 验证方式

### 编译/测试

- `cd services/api && go test ./...`
- `cd services/api && go build ./...`
- `cd services/web && npm run build`
- `cd services/bot && python -m py_compile main.py`

按改动补充针对性测试：

- API：规则读写、权限隔离、默认值
- Bot：命中静音规则时正确跳过发送
- Web：设置项保存和回显

### 手工验证

- 关闭管理员“支付成功”通知，模拟支付完成，确认 Telegram 不再推送
- 保留“订阅待审批”通知，创建订阅后确认管理员仍能收到
- 用户关闭“订阅已入库”通知后，后续命中该事件时不再私聊推送
- 规则接口短暂不可用时，Bot 仍可按默认行为发送通知

## 落地后文档处理

落地后应同步处理：

- 将稳定结论同步到 `docs/system-architecture.md`
  - Telegram 通知规则模型
  - Bot 运行期规则读取方式
- 若用户侧通知正式上线，再补充账号中心说明文档
- 主体稳定后移入 `docs/archive/plan/bot-telegram/`
