# 订阅状态可见性与结果通知实现方案

> 状态：草稿
> 负责人：Ember
> 更新时间：2026-04-12

## 背景

这个问题为什么现在要解决：

- 用户提交订阅后，只能看到粗粒度状态，无法确认审核是否通过。
- 有些用户没有先检查库内是否已存在资源，就直接提交订阅，导致管理员收到本不该出现的重复请求。
- 管理员拒绝订阅时，系统没有面向用户的拒绝原因承载位，用户只能知道“被拒绝”，不知道为什么。
- 审核通过后，系统只表示“已批准”，没有把“已真正入库”作为独立状态暴露给用户。
- 已绑定 Telegram 的用户无法收到审核结果或入库结果通知，状态追踪完全依赖手动刷新页面。
- 现有 Emby webhook 已能点亮追剧日历剧集状态，但没有把这条真实入库事件回写到订阅主链路。

## 目标

本方案要实现：

1. 用户可以在 Web 端查看订阅从提交到审核结果再到入库的完整状态。
2. 管理员拒绝订阅时必须填写拒绝原因，用户可以在 Web 和 Telegram 中看到该原因。
3. 已绑定 Telegram 的用户在“审核通过 / 审核拒绝 / 已入库”三个节点收到私聊通知。
4. 以真实 Emby webhook 作为“已入库”唯一可信事件源，避免轮询猜测导致误报。
5. 用户提交订阅前，系统先基于 Emby 库内实际资源做存在性检测；若已存在，则先提示、再允许二次确认后继续提交。
6. 保持现有管理员审批链路和 TV Calendar webhook 点亮链路可继续工作，不破坏既有行为。

## 非目标

本次明确不做：

- 不发送“订阅已提交”Telegram 通知。
- 不引入站内消息中心、未读数、消息历史表。
- 不做标题模糊匹配、最近入库轮询推断或人工猜测式入库确认。
- 不修改现有“同一 `type + tmdbId + season` 全局唯一”的订阅去重策略。
- 不实现“整季全部入库后才算完成”的重型剧集完结判断。
- 不把“库内已存在”做成强制硬拦截；用户二次确认后仍允许提交。

## 当前事实

以当前代码和现行文档为准，先写清现状：

- 相关文档：
  - `docs/system-architecture.md`
  - `docs/public/features/subscriptions.md`
- 相关服务/页面/模型：
  - `services/api/internal/models/subscription.go`
  - `services/api/internal/services/subscription/service.go`
  - `services/api/internal/handlers/subscription.go`
  - `services/api/internal/integrations/emby/library.go`
  - `services/api/internal/handlers/tv_calendar.go`
  - `services/web/src/views/console/NewSubscriptionView.vue`
  - `services/web/src/views/console/SubscriptionsView.vue`
  - `services/bot/app/handlers/telegram_handler.py`
  - `services/bot/app/formatters/message_formatter.py`
- 当前行为：
  - Web 用户在 `NewSubscriptionView` 里选中 TMDB 条目后，确认弹窗只收集季数和备注，不检查 Emby 库中是否已存在资源。
  - 订阅状态只有 `PENDING`、`APPROVED`、`REJECTED` 三态。
  - `note` 是用户提交备注，不是管理员拒绝原因。
  - 审核通过后调用 MoviePilot，结果失败仅记录到 `mpError`，状态仍改为 `APPROVED`。
  - Telegram 目前只通知管理员“有新的待审批订阅”，不会通知提交用户。
  - Emby webhook 当前只处理剧集 `episode` 入库，并用于点亮 TV Calendar，不回写订阅状态。
- 现有限制：
  - 用户提交前无法知道库中是否已有同一电影、整剧或目标季的资源。
  - 管理员会收到“库里已有资源但用户仍提交”的无效订阅，增加审批噪音。
  - 用户无法知道拒绝原因。
  - 用户无法知道“审核通过但还未入库”和“已经入库”的区别。
  - 已绑定 Telegram 的用户没有结果通知。
  - 电影入库目前没有现成的订阅状态回写逻辑。
  - Bot 的拒绝操作仍是一键拒绝，不支持输入拒绝原因。

## 方案设计

### 1. 用户可见行为

- 新增能力：
  - 用户在提交订阅前，系统先检测 Emby 库内是否已存在对应资源；若命中，则弹出二次确认提示。
  - 二次确认弹窗展示库内已存在摘要，例如电影已存在、整剧已存在、目标季已存在、已入库季数/集数概况。
  - 用户在订阅列表中看到四个正式状态：`审核中`、`已通过，等待入库`、`已拒绝`、`已入库`。
  - 用户在 `已拒绝` 的卡片或详情区看到明确拒绝原因。
  - 用户在 `已入库` 状态下看到入库时间。
  - 已绑定 Telegram 的用户在以下节点收到私聊：审核通过、审核拒绝（带原因）、已入库。
- 修改现有行为：
  - 用户点击“提交订阅”不再直接落库，而是先走“库内存在性检查 → 若命中则二次确认 → 最终提交”。
  - 管理员拒绝订阅不再允许空拒绝，必须填写原因。
  - `APPROVED` 的语义从“事情结束”改为“审核通过，但还未确认 Emby 已有内容”。
  - 用户侧筛选增加 `已入库` 选项。
- 哪些现有行为必须保持不变：
  - 即使系统检测到库内已存在，用户仍可在明确二次确认后继续提交。
  - 用户创建订阅后默认仍进入待审核，不自动通过。
  - MoviePilot 调用失败仍不阻塞审核通过，只记录下游异常。
  - TV Calendar 的 webhook 点亮行为保留。
  - 未绑定 Telegram 的用户仍可完整通过 Web 查看状态，不因为没有 TG 而丢失信息。
- 前端约束：
  - 前端实现必须遵守 Ember 风格。
  - 设计与交互基线以 `docs/reference/web-design-guide.md` 为准。
  - 提交前存在性提示应复用 `NewSubscriptionView` 的现有确认链路，不新增脱离上下文的独立页面。
  - 若需要增加“查看媒体库”跳转或补充提示卡片，必须保持现有对话框层级、按钮语义和主次操作样式一致。

### 2. 数据与模型

- 新增或修改 `subscriptions` 结构：
  - 扩展状态枚举，新增 `INGESTED`
  - 新增字段 `rejectReason`，用于管理员拒绝原因，`text nullable`
  - 新增字段 `reviewedAt`，记录审批时间，`timestamp nullable`
  - 新增字段 `ingestedAt`，记录入库确认时间，`timestamp nullable`
- 修改哪些现有结构：
  - `note` 继续表示用户提交备注，不改语义。
  - `rejectReason` 只在 `REJECTED` 时有值。
  - `reviewedAt` 在 `APPROVED` 或 `REJECTED` 时写入。
  - `ingestedAt` 只在 `INGESTED` 时写入。
- 是否需要迁移：
  - 需要 SQL migration，放在 `infrastructure/database/`
  - 迁移脚本要求幂等
  - 需要同步修改 GORM 模型
  - 需要同步更新系统架构文档中的 `Subscription` 模型说明

### 3. 接口与边界

- 新增或修改哪些 API / Internal API / webhook / 命令：
  - `POST /api/v1/subscriptions/check-existing`
    - 新增提交前检测接口
    - 请求体沿用订阅核心字段：`type`、`tmdbId`、`season`
    - 返回是否命中库内资源，以及命中的摘要信息
  - `POST /api/v1/subscriptions`
    - 请求体新增可选字段 `confirmExisting`
    - 当检测到库内已存在且 `confirmExisting != true` 时，不直接创建订阅，而是返回“需要二次确认”的结构化响应
  - `GET /api/v1/subscriptions`
    - 返回字段新增 `rejectReason`、`reviewedAt`、`ingestedAt`
    - `status` 支持 `INGESTED`
  - `PUT /api/v1/admin/subscriptions/:id/approve`
    - 路径不变
    - 行为变为写入 `reviewedAt` 并触发用户“审核通过”通知
  - `PUT /api/v1/admin/subscriptions/:id/reject`
    - 请求体改为 `{ reason: string }`
    - 原因为空返回 `400`
    - 成功后触发用户“审核拒绝”通知
  - `PUT /api/v1/internal/subscriptions/:id/approve`
    - 路径不变
    - 与公开管理员审批保持同语义
  - `PUT /api/v1/internal/subscriptions/:id/reject`
    - 请求体同样改为 `{ reason: string }`
    - 给 Telegram Bot 调用
  - `POST /api/v1/webhooks/emby?token=...`
    - 在现有 TV Calendar 点亮逻辑后，补一段“订阅入库确认”逻辑
    - 仅处理真实入库 webhook，不新增轮询任务
- 请求参数与响应字段怎么变：
  - 提交前检测响应新增 `existsInLibrary`、`existingSummary`
  - `existingSummary` 至少包含：
    - `matchType`：`movie`、`series`、`season`
    - `embyItemId` 或可跳转目标
    - `message`
    - 对剧集可选返回 `availableSeasons`、`episodeCount`
  - 订阅响应对象新增 `rejectReason`、`reviewedAt`、`ingestedAt`
  - 管理端和 Internal `reject` 都必须接受 `reason`
- 哪些调用方会受影响：
  - Web 新建订阅页提交交互
  - Web 用户端订阅页
  - Web 管理员端订阅审核交互
  - Telegram Bot 审批回调
  - Emby webhook 处理链路

### 4. 关键流程

按顺序写清主链路，不需要贴代码：

1. 用户在 Web 端选中 TMDB 条目并点击提交时，前端先调用 `check-existing`。
2. 后端基于 Emby 实际资源做存在性检测：
   - 电影：按 `tmdbId` 命中同一电影即视为已存在
   - 剧集整剧（`season=0`）：按 `tmdbId` 命中该剧任意已入库内容即返回“已存在部分或全部内容”的提示
   - 剧集指定季：按 `tmdbId + season` 检查目标季是否已有已入库剧集，并返回已存在摘要
3. 若未命中库内资源，或用户在弹窗中明确二次确认，则创建订阅，记录落库为 `PENDING`。
4. 管理员在 Web 或 Telegram 审批：
   - 通过：调用 `ApproveSubscription`
   - 拒绝：调用 `RejectSubscription(id, reason)`
5. `ApproveSubscription` 校验当前状态必须是 `PENDING`，调用 MoviePilot，同步写入 `APPROVED`、`reviewedAt`，保留 `mpError`，若用户已绑定 TG，则推送“已通过，等待入库”。
6. `RejectSubscription` 校验当前状态必须是 `PENDING`，写入 `REJECTED`、`rejectReason`、`reviewedAt`，若用户已绑定 TG，则推送“已拒绝”及原因。
7. Emby webhook 到达：
   - 电影事件：若能提取出可靠 TMDB ID，并匹配到 `APPROVED` 的电影订阅，则将其转为 `INGESTED`
   - 剧集事件：若能提取出可靠 TMDB ID 和季号，并匹配到 `APPROVED` 的剧集订阅，则将其转为 `INGESTED`
8. 订阅首次转为 `INGESTED` 时，写入 `ingestedAt`，并对绑定 TG 的用户推送“已入库”。
9. Web 订阅列表读取统一分页接口，直接展示最新状态、拒绝原因和时间字段。

剧集“已入库”判定规则：

- 若订阅是 `season=0`，任意季的首个真实剧集入库事件命中即可标记为 `INGESTED`
- 若订阅是指定季，只有该季首个真实剧集入库事件命中才标记为 `INGESTED`

### 5. 失败路径与边界条件

- 提交前检测命中库内已存在，且用户未二次确认：接口返回“需要确认”响应，前端弹窗提示，不直接落库。
- 检测结果显示“已存在部分内容”，但用户是为了补画质、补版本或补缺季而继续提交：允许携带 `confirmExisting=true` 继续落库。
- 检测使用的 Emby 查询失败：前端提示“库内检测失败，是否仍继续提交”，用户明确确认后允许继续提交，避免因为探测失败把订阅入口完全堵死。
- 管理员拒绝时未填写原因：接口返回 `400`，前端阻止提交，Bot 拒绝流程不得落库。
- 订阅已被处理后再次审批：返回明确错误，不覆盖原状态。
- MoviePilot 调用失败：状态仍为 `APPROVED`，用户端提示“已通过但下游提交异常”，避免假装一切正常。
- 用户未绑定 Telegram：跳过私聊推送，不影响主流程。
- Telegram 发送失败或用户屏蔽 Bot：只记日志，不回滚状态。
- webhook 缺失可用 TMDB ID：不改订阅状态，只保留 TV Calendar 现有逻辑。
- webhook 重复到达：`INGESTED` 只允许首次写入，避免重复通知。
- 兼容性约束：
  - 不能破坏现有 `/api/v1/webhooks/emby` 对 TV Calendar 的更新能力
  - 不能把 `note` 的语义从“用户备注”篡改成“管理员理由”
  - 不能改变未被明确要求的用户创建、删除待审核订阅等现有行为

## 影响范围

涉及的子系统：

- API：有
  - 订阅模型、订阅服务、订阅 handler、库内存在性检测、webhook 链路、Bot notifier
- Web：有
  - 新建订阅页二次确认交互、用户订阅列表展示、管理员拒绝交互、类型定义、API 调用
- Bot：有
  - 审批回调、拒绝原因输入流程、用户私聊通知格式化
- 配置/部署：无新增环境变量
  - 继续依赖现有 `BOT_NOTIFY_URL`、`INTERNAL_API_SECRET`、`EMBY_WEBHOOK_TOKEN`
- 文档：需要更新
  - `docs/system-architecture.md`
  - `docs/public/features/subscriptions.md`

## 验证方式

### 编译/测试

- `cd services/api && go test ./...`
- `cd services/api && go build ./...`
- `cd services/web && npm run build`
- `cd services/bot && python -m py_compile main.py`

按改动补充针对性测试：

- API：订阅状态迁移、拒绝原因校验、库内存在性检测、webhook 幂等
- Web：新建订阅页二次确认、订阅状态展示与拒绝原因输入
- Bot：消息格式和拒绝两步交互

### 手工验证

- 电影在 Emby 库中已存在时，用户首次提交会收到明确提示；点击二次确认后仍可成功创建订阅
- 剧集目标季在 Emby 库中已存在时，用户首次提交会收到“已存在目标季”的提示
- 剧集整剧提交但库中仅有部分季时，提示文案能明确表达“已存在部分内容”，不误导成“完整可看”
- 用户创建订阅后，在 Web 看到 `审核中`
- 管理员在 Web 批准后，用户页变为 `已通过，等待入库`
- 管理员在 Web 拒绝并填写原因后，用户页展示拒绝原因
- 用户已绑定 Telegram 时，审核通过会收到私聊
- 用户已绑定 Telegram 时，审核拒绝会收到带原因的私聊
- 模拟 Emby 电影入库 webhook，命中电影订阅后状态变为 `已入库`
- 模拟 Emby 剧集入库 webhook，命中整剧或指定季订阅后状态变为 `已入库`
- 重复发送同一 webhook，不会重复发送“已入库”通知
- TV Calendar 原有 webhook 点亮链路仍然可用

## 落地后文档处理

落地后应同步处理：

- 将稳定结论同步到 `docs/system-architecture.md`
  - `Subscription` 模型字段
  - 订阅状态机
  - Emby webhook 与订阅入库确认关系
  - Bot 用户通知能力
- 将用户可见行为同步到 `docs/public/features/subscriptions.md`
  - 新状态定义
  - 拒绝原因可见
  - 审核通过不等于已入库
- 功能落地、编译验证和手工链路验证完成后，将本方案迁入 `docs/archive/plan/media-subscription/`
