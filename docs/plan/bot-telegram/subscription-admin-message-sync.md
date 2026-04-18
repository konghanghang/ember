# 多管理员订阅通知消息同步实现方案

> 状态：草稿
> 负责人：Ember
> 更新时间：2026-04-18

## 背景

这个问题为什么现在要解决：

- 当前订阅创建后，Bot 只负责把待审批消息发送给管理员，但系统不会持久化每条 Telegram 管理员消息的引用信息。
- 当管理员直接在 Telegram 中点击按钮时，Bot 还能就地编辑当前那条消息；但当管理员在 Web 后台审批时，后端无法定位到 Telegram 中对应的待审批消息，因此原消息状态不会同步更新。
- 后续若支持多个管理员同时接收订阅通知，一条订阅会对应多条管理员消息；任一管理员处理后，其余管理员手里的消息也必须同步变成已处理状态。
- 如果不做消息投递建模，多管理员场景下会出现“同一订阅已处理，但其他管理员消息仍显示待审批”的状态漂移。

## 目标

本方案要实现：

1. 为每条订阅管理员通知持久化 Telegram 消息引用，而不是依赖 Bot 内存。
2. 支持一条订阅对应多条管理员通知消息。
3. 无论审批动作发生在 Telegram 还是 Web 后台，所有管理员通知消息都能同步更新。
4. 任一管理员处理成功后，其余管理员的消息按钮失效并更新为最终结果。
5. 保持现有用户侧审核结果通知链路不变。

## 非目标

本次明确不做：

- 不引入站内消息中心。
- 不扩展到 Telegram 之外的通知渠道。
- 不实现复杂的消息重试队列或补偿调度系统。
- 不改变现有订阅审批的业务语义，只处理管理员通知消息同步。

## 当前事实

以当前代码和现行文档为准，先写清现状：

- 相关文档：
  - `docs/system-architecture.md`
  - `docs/archive/plan/bot-telegram/telegram-subscription-notification.md`
- 相关服务/页面/模型：
  - `services/api/internal/integrations/notifier/notifier.go`
  - `services/api/internal/services/subscription/service.go`
  - `services/bot/app/handlers/telegram_handler.py`
  - `services/bot/app/server.py`
- 当前行为：
  - API 通过 `BotNotifier.NotifyNewSubscription()` fire-and-forget 通知 Bot 发送管理员待审批消息。
  - Bot 发送消息后不会把 `chatId/messageId` 回写到 API 或数据库。
  - Telegram 内点击按钮时，Bot 直接用当前回调上下文编辑当前消息。
  - Web 后台审批时，只会更新订阅状态和用户侧结果通知，不会更新 Telegram 管理员消息。
- 现有限制：
  - 一条订阅没有稳定的“管理员消息投递记录”。
  - 后续若存在多个管理员，每个管理员会收到不同消息，但系统无法统一追踪这些消息。
  - Bot 重启后也无法回溯历史待审批消息和其投递状态。

## 方案设计

### 1. 用户可见行为

- 新增能力：
  - 任一管理员在 Web 或 Telegram 中处理订阅后，所有管理员收到的待审批消息都会同步更新为最终结果。
  - 已被其他管理员处理的消息，不再保留可点击按钮。
  - 管理员可从已更新的消息中看到“已通过 / 已拒绝”结果，必要时可显示处理人。
- 修改现有行为：
  - Telegram 审批消息不再只是当前会话上下文内可编辑，而是变成“与订阅绑定的可同步消息”。
- 哪些现有行为必须保持不变：
  - 用户提交通知仍然是异步 fire-and-forget，不阻塞主请求。
  - 用户侧审核结果私聊通知保持现有行为。
  - Telegram 内一键审批交互保留。
- 前端约束：
  - Web 后台不新增新的操作入口；仍复用现有审批按钮。
  - 这次不改前端视觉，仅保证后台审批后状态能同步到 Telegram。

### 2. 数据与模型

- 新增 `subscription_admin_notifications` 表：
  - `id`
  - `subscriptionId`
  - `adminTelegramId`
  - `chatId`
  - `messageId`
  - `hasPhoto`
  - `deliveryStatus`
    - 例如：`sent`、`edit_failed`、`deleted`
  - `createdAt`
  - `updatedAt`
- 修改哪些现有结构：
  - 不再把管理员消息引用尝试塞进 `subscriptions` 主表。
  - 一条订阅对应多条管理员消息投递记录。
- 是否需要迁移：
  - 需要 SQL migration，放在 `infrastructure/database/`
  - 迁移脚本要求幂等
  - 需要新增 GORM 模型并更新系统架构文档

### 3. 接口与边界

- 新增或修改哪些 API / Internal API / webhook / 命令：
  - `POST /notify/subscription`
    - Bot 发送管理员消息后，不再只返回 `200 OK` 空结果
    - 需要把发送结果回传给 API，或 API 改为同步拿回消息引用结果
  - 新增 Bot 内部接口：
    - 例如 `POST /notify/subscription-admin-sync`
    - 由 API 在审批成功后调用
    - 入参至少包含：
      - `subscriptionId`
      - 订阅当前结果状态
      - 可选 `handledBy`
  - API 新增管理员消息投递记录写入与查询逻辑
- 请求参数与响应字段怎么变：
  - 新订阅通知结果至少应能拿到：
    - `chatId`
    - `messageId`
    - `hasPhoto`
    - `adminTelegramId`
- 哪些调用方会受影响：
  - API `BotNotifier`
  - Bot `send_subscription_notification`
  - 订阅审批服务

### 4. 关键流程

按顺序写清主链路，不需要贴代码：

1. 用户创建订阅。
2. API 生成一条 `subscriptions` 记录。
3. API 遍历当前应接收通知的管理员列表，调用 Bot 发送管理员待审批消息。
4. Bot 每发送成功一条消息，就把 `chatId/messageId/hasPhoto/adminTelegramId` 返回给 API。
5. API 为每条消息落一条 `subscription_admin_notifications`。
6. 某个管理员在 Telegram 或 Web 后台处理该订阅。
7. API 用“仅允许 `PENDING -> 已处理` 一次”的原子更新完成真正审批。
8. 若本次审批成功，API 查询该订阅对应的所有管理员消息投递记录。
9. API 调 Bot“同步管理员消息”接口。
10. Bot 逐条编辑这些消息，统一改成最终结果并移除按钮。
11. 若某条消息编辑失败，记录为 `edit_failed`，不回滚主审批状态。

### 5. 失败路径与边界条件

- Bot 发送某个管理员消息失败：只影响该管理员的通知记录，不阻塞订阅创建。
- 某个管理员消息编辑失败：记录失败状态，不回滚订阅审批结果。
- 订阅已被其他管理员先处理：后续审批请求返回明确错误，并可触发一次管理员消息同步修正。
- Telegram 消息已被删除：将对应投递记录标记为 `deleted` 或 `edit_failed`。
- Bot 重启：不影响后续同步，因为消息引用已落库。
- 兼容性约束：
  - 不能把“是否已处理”的真相放在 Bot 内存里。
  - 不能依赖当前回调上下文作为唯一消息定位方式。
  - 不能让 Web 审批和 Telegram 审批各走一套互不一致的最终状态逻辑。

## 影响范围

涉及的子系统：

- API：有
  - Bot 通知客户端、订阅审批服务、管理员消息投递模型与存储
- Web：无
  - 本次不改页面交互，只复用现有后台审批入口
- Bot：有
  - 新增消息发送结果回传与批量编辑能力
- 配置/部署：可能无新增环境变量
  - 若管理员列表来源已有现成配置，可直接复用；若后续引入多管理员配置，再单独开方案
- 文档：需要更新
  - `docs/system-architecture.md`

## 验证方式

### 编译/测试

- `cd services/api && go test ./...`
- `cd services/api && go build ./...`
- `cd services/web && npm run build`
- `cd services/bot && python -m py_compile main.py`

按改动补充针对性测试：

- API：订阅审批原子更新、投递记录创建与查询
- Bot：消息发送结果回传、批量编辑成功/失败分支

### 手工验证

- 配置两个管理员都接收订阅通知，创建一条新订阅，确认两人都收到待审批消息
- 管理员 A 在 Telegram 中点击通过，确认管理员 B 的消息也同步更新为已通过
- 管理员 A 在 Web 后台点击拒绝，确认所有管理员 Telegram 消息都同步更新为已拒绝
- 某条管理员消息被手动删除后，再处理订阅，确认主审批成功且失败消息被记录

## 落地后文档处理

落地后应同步处理：

- 将稳定结论同步到 `docs/system-architecture.md`
  - `subscription_admin_notifications` 模型
  - 多管理员通知投递与同步机制
  - Web / Telegram 共用的审批结果同步链路
- 功能落地、编译验证和手工链路验证完成后，将本方案迁入 `docs/archive/plan/bot-telegram/`
