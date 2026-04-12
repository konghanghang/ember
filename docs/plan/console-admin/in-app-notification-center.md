# 站内通知中心实现方案

> 状态：草稿
> 负责人：Ember
> 更新时间：2026-04-12

## 背景

这个问题为什么现在要解决：

- Ember 现在的通知主要依赖 Telegram Bot 和用户手动刷新页面，站内没有统一的消息承载层。
- 求片审批、支付成功、注册成功、设备处置等事件已经存在，但结果只在日志或 Bot 中流动，未沉淀为用户和管理员都能回看的状态。
- 后续若继续扩展订阅结果通知、设备风控告警、系统任务结果，没有站内通知中心就只能继续堆外部推送，噪音会越来越大。

## 目标

本方案要实现：

1. 为 Ember 增加统一的站内通知中心，支持未读统计、已读、全部已读和跳转动作。
2. 让关键业务事件先写入站内通知，再按需要同步到 Telegram，避免“只有外部推送，没有站内留痕”。
3. 同时覆盖管理员和普通用户，作为后续订阅结果、支付和风控等事件的统一承载层。

## 非目标

本次明确不做：

- 不实现站内聊天、评论、工单会话。
- 不实现 Web Push、邮件推送中心或移动端推送聚合。
- 不做复杂消息模板编辑器；首版只支持代码内定义的固定事件模板。
- 不做“按角色动态广播”抽象层，首版直接写入具体目标用户。

## 当前事实

以当前代码和现行文档为准，先写清现状：

- 相关文档：
  - `docs/system-architecture.md`
  - `docs/plan/media-subscription/subscription-status-and-notification.md`
- 相关服务/页面/模型：
  - `services/api/internal/integrations/notifier/notifier.go`
  - `services/api/internal/services/auth/register_notify.go`
  - `services/api/internal/services/payment/service.go`
  - `services/api/internal/services/subscription/service.go`
  - `services/bot/app/handlers/telegram_handler.py`
  - `services/web/src/components/console/TopBar.vue`
- 当前行为：
  - 新注册、支付成功、求片提交、排行榜推送会通过 Bot HTTP 通道发往 Telegram。
  - Web 控制台没有通知列表、未读角标、消息详情或动作跳转。
  - 后端没有通用的通知模型、通知 API 或通知写入服务。
- 现有限制：
  - 未绑定 Telegram 的用户收不到任何结果通知。
  - 管理员错过 Telegram 消息后，系统内没有统一回看入口。
  - 新增通知场景只能继续堆 Bot 消息，无法做用户侧消息历史和已读状态。

## 方案设计

### 1. 用户可见行为

- 新增能力：
  - 控制台顶部增加站内通知入口，显示未读数量。
  - 用户和管理员都可在 `/console/notifications` 查看自己的消息列表。
  - 通知支持按 `全部 / 未读 / 类型` 筛选，并可执行“标记已读”“全部已读”。
  - 通知项支持 `actionUrl`，点击后跳转到目标页面，如订阅详情、支付记录、设备页。
- 修改现有行为：
  - 关键业务事件优先写入站内通知，Telegram 只作为额外通道而不是唯一承载层。
- 哪些现有行为必须保持不变：
  - Telegram 现有管理员通知能力保持不变。
  - 现有业务接口返回值不因引入通知中心而改变。
  - 未开启或未消费站内通知时，原有业务主流程不能被阻塞。
- 前端约束：
  - 前端实现必须遵守 Ember 风格。
  - 设计与交互基线以 `docs/reference/web-design-guide.md` 为准。
  - 新页面默认采用“Header Card + List Card”骨架，不单独发明第二套通知页视觉语言。
  - 若存在偏离规范的特例，必须单独写明原因、范围和收口条件。

### 2. 数据与模型

- 新增 `notifications` 表：
  - `id`
  - `userId`
  - `category`：如 `subscription`、`payment`、`system`、`risk`
  - `level`：如 `info`、`success`、`warning`、`error`
  - `title`
  - `body`
  - `actionUrl`
  - `payload`：补充结构化上下文，JSON
  - `isRead`
  - `readAt`
  - `createdAt`
- 约束建议：
  - `userId + createdAt` 建索引，保证列表查询
  - `isRead` 建辅助索引，保证未读计数
- 是否需要迁移：
  - 需要 SQL migration，放在 `infrastructure/database/`
  - 需要新增 GORM 模型和基础读写服务

### 3. 接口与边界

- 新增或修改哪些 API / Internal API / webhook / 命令：
  - `GET /api/v1/notifications`
    - 返回当前用户通知列表和未读统计
  - `POST /api/v1/notifications/:id/read`
    - 标记单条已读
  - `POST /api/v1/notifications/read-all`
    - 标记当前用户全部已读
  - 可选：`GET /api/v1/notifications/unread-count`
    - 若前端需要独立轮询角标
- 请求参数与响应字段怎么变：
  - 列表接口统一返回 `data`
  - 通知对象字段使用 camelCase
  - 不暴露其他用户的通知
- 哪些调用方会受影响：
  - `TopBar` 需要新增未读角标
  - 控制台新增通知页
  - 注册、支付、订阅、后续风控等服务要接入通知写入服务

### 4. 关键流程

按顺序写清主链路，不需要贴代码：

1. 某个业务事件成功完成，例如支付成功或订阅审核结果产生。
2. 领域服务调用 `NotificationService`，按目标用户写入一条或多条站内通知。
3. 控制台加载时获取未读数，并在顶部显示角标。
4. 用户进入通知页，读取分页列表，系统按当前登录用户过滤。
5. 用户点击通知项时，先标记已读，再跳转到 `actionUrl`。
6. 用户执行“全部已读”后，未读数同步归零。

### 5. 失败路径与边界条件

- 通知写入失败：只记日志，不回滚主业务流程。
- 通知 `actionUrl` 失效：允许通知仍展示，但前端需要优雅降级，避免白屏。
- 同一事件被重复触发：首版允许重复通知，后续如需幂等再按 `payload` 引入去重键。
- 用户删除账号或失效：不再新建通知；历史通知按用户清理策略统一处理。
- 兼容性约束：
  - 不能破坏现有 Telegram 通知链路。
  - 不能把通知中心做成只有管理员可见的单侧功能，用户结果通知必须可承载。

## 影响范围

涉及的子系统：

- API：有
  - 新增通知模型、服务、接口
  - 业务服务接入通知写入
- Web：有
  - `TopBar` 未读角标
  - 新增通知列表页
- Bot：低影响
  - 现有 Bot 通知逻辑不改，但后续可复用站内通知模板来源
- 配置/部署：无新增环境变量
- 文档：需要更新
  - `docs/system-architecture.md`
  - 如落地稳定，可补充到用户文档

## 验证方式

### 编译/测试

- `cd services/api && go test ./...`
- `cd services/api && go build ./...`
- `cd services/web && npm run build`

按改动补充针对性测试：

- API：通知列表、已读、读全部、权限隔离
- Web：顶部未读数、通知跳转、已读状态渲染

### 手工验证

- 注册成功后，管理员站内通知新增一条“新用户注册”
- 支付成功后，用户站内通知新增一条“支付成功”
- 订阅审核结果产生后，用户通知列表可见对应结果
- 顶部未读角标在进入通知页并读消息后同步减少
- 不同用户登录时，只能看到自己的通知

## 落地后文档处理

落地后应同步处理：

- 将稳定结论同步到 `docs/system-architecture.md`
  - 通知模型
  - 通知 API
  - 关键业务事件与通知关系
- 视最终落地情况补充用户文档中的“通知中心”说明
- 主体稳定后移入 `docs/archive/plan/console-admin/`
