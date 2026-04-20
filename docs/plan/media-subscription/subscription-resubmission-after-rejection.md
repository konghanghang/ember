# 订阅拒绝后重新发起实现方案

> 状态：草稿
> 负责人：Ember
> 更新时间：2026-04-18

## 背景

这个问题为什么现在要解决：

- 当前 `subscriptions` 对 `type + tmdbId + season` 使用全局唯一约束，被拒绝记录会持续占用唯一位。
- 用户订阅被拒绝后，无法针对同一作品再次提交，即使实际情况已经变化，例如资源版本更新、此前命名不准确或需要补充说明。
- 管理员拒绝后给出的 `rejectReason` 只能作为终态展示，无法自然进入“用户补充说明后重新发起”的下一轮工作流。
- 现有行为把“历史拒绝记录”和“活跃提交约束”混在一起，不符合“历史保留、活跃去重”的边界设计。

## 目标

本方案要实现：

1. 用户可以从 `REJECTED` 订阅记录再次发起一条新的订阅申请。
2. 被拒绝记录保留为历史，不被覆盖或改写。
3. 唯一约束只作用于活跃订阅，允许历史拒绝记录存在多条。
4. 用户重新发起时，必须看到上一次拒绝原因，并补充新的提交说明。
5. 保持现有管理员审批、MoviePilot 下发、Emby webhook 回写链路不变。

## 非目标

本次明确不做：

- 不引入“订阅主表 + attempts 子表”的重型工单模型。
- 不把 `REJECTED` 记录直接改回 `PENDING`。
- 不允许用户无说明地一键无限重提。
- 不修改 `APPROVED` / `INGESTED` 状态的业务语义。
- 不处理“已入库后仍允许补版本/补画质”的新需求边界。

## 当前事实

以当前代码和现行文档为准，先写清现状：

- 相关文档：
  - `docs/system-architecture.md`
  - `docs/archive/plan/media-subscription/subscription-status-and-notification.md`
- 相关服务/页面/模型：
  - `services/api/internal/models/subscription.go`
  - `services/api/internal/services/subscription/service.go`
  - `services/api/internal/handlers/subscription.go`
  - `services/web/src/views/console/SubscriptionsView.vue`
  - `services/web/src/views/console/NewSubscriptionView.vue`
  - `infrastructure/database/20260415_00_schema_baseline.sql`
- 当前行为：
  - `subscriptions` 通过唯一索引 `uk_subscription_media` 对 `type + tmdbId + season` 做全局唯一约束。
  - API 创建订阅前，服务层也会按同样条件查询并阻止重复创建。
  - 普通用户只能删除 `PENDING` 订阅，`REJECTED` 订阅只能查看，不能再次发起。
  - `rejectReason` 已可见，但只作为结果展示，不参与后续提交流程。
- 现有限制：
  - 同一作品一旦被拒绝，当前用户无法再次发起。
  - 更严重的是，其他用户也会被同一条被拒绝记录挡住。
  - 历史拒绝记录与活跃去重策略耦合，导致数据边界错误。

## 方案设计

### 1. 用户可见行为

- 新增能力：
  - 用户在 `REJECTED` 卡片上可以看到 `再次提交` 入口。
  - 点击 `再次提交` 后，进入基于原记录预填的重新发起流程。
  - 页面明确展示“上次拒绝原因”。
  - 用户必须补充“本次提交说明”后才能再次提交。
- 修改现有行为：
  - 同一 `type + tmdbId + season` 的历史拒绝记录不再阻止新建订阅。
  - 用户重新发起时，不再手工重新搜索和重新选作品。
- 哪些现有行为必须保持不变：
  - `REJECTED` 记录保留，仍展示原拒绝原因和原审核时间。
  - 活跃状态下仍不允许出现重复订阅。
  - 管理员仍只审批新生成的 `PENDING` 记录，不审批已拒绝历史。
- 前端约束：
  - 前端实现必须遵守 Ember 风格。
  - 设计与交互基线以 `docs/reference/web-design-guide.md` 为准。
  - `再次提交` 入口应挂在 `REJECTED` 卡片上下文内，不新增脱离当前页的新入口。
  - 拒绝原因展示与新说明输入要明确区分“上次管理员理由”和“本次用户补充”，不能混成同一字段语义。

### 2. 数据与模型

- 新增或修改 `subscriptions` 结构：
  - 新增可空字段 `retryFromId`，指向上一次被拒绝的订阅 ID。
- 修改哪些现有结构：
  - 取消当前全局唯一索引 `uk_subscription_media`。
  - 改为只约束活跃状态的部分唯一索引，例如：
    - `(type, tmdbId, season) WHERE status IN ('PENDING', 'APPROVED', 'INGESTED')`
  - `note` 继续表示当前这次提交的用户说明，不改变字段语义。
  - 重新发起生成的是一条新记录，不修改原 `REJECTED` 记录。
- 是否需要迁移：
  - 需要 SQL migration，放在 `infrastructure/database/`
  - 迁移脚本要求幂等
  - 需要同步修改 GORM 模型，去掉旧全局唯一索引声明
  - 需要同步更新系统架构文档中的 `Subscription` 模型与唯一约束说明

### 3. 接口与边界

- 新增或修改哪些 API / Internal API / webhook / 命令：
  - `POST /api/v1/subscriptions`
    - 创建前重复检查改为只检查活跃状态
  - `POST /api/v1/subscriptions/:id/resubmit`
    - 新增“从拒绝记录重新发起”接口
    - 仅允许当前用户针对自己的 `REJECTED` 记录调用
    - 请求体至少包含本次 `note`
    - 服务端复用原记录的 `type`、`tmdbId`、`season`、`name`、`posterPath`
  - `GET /api/v1/subscriptions`
    - 返回字段新增 `retryFromId`
- 请求参数与响应字段怎么变：
  - `resubmit` 请求体：
    - `note: string`
    - 可选 `confirmExisting: boolean`
  - 订阅响应对象新增 `retryFromId`
- 哪些调用方会受影响：
  - Web 用户端订阅页
  - Web 新建订阅页 / 再次提交流程
  - API 创建订阅与重复检查逻辑

### 4. 关键流程

按顺序写清主链路，不需要贴代码：

1. 用户打开订阅列表，看到一条 `REJECTED` 记录。
2. 用户点击 `再次提交`。
3. 前端进入重新发起流程，并展示：
   - 原作品信息
   - 上次拒绝原因
   - 本次说明输入框
4. 用户填写本次说明后提交。
5. 后端校验原记录必须属于当前用户，且状态为 `REJECTED`。
6. 后端按活跃状态检查是否已存在相同作品的 `PENDING / APPROVED / INGESTED` 记录：
   - 若存在，则拒绝重新发起
   - 若不存在，则创建一条新的 `PENDING` 记录，并写入 `retryFromId`
7. 管理员后续只处理新的 `PENDING` 记录；旧 `REJECTED` 记录继续保留为历史。

### 5. 失败路径与边界条件

- 原记录不是 `REJECTED`：拒绝重新发起。
- 原记录不属于当前用户：返回无权操作。
- 本次说明为空：前端阻止提交，后端也返回 `400`。
- 同一作品已经存在活跃记录：返回明确错误，不再生成新的待审核记录。
- 原记录已被管理员删除：返回“订阅不存在”。
- 重新发起时若库内存在性检测命中：仍保留现有“检测 -> 二次确认”链路，不单独绕过。
- 兼容性约束：
  - 不能把历史 `REJECTED` 记录改写回 `PENDING`
  - 不能破坏现有 MoviePilot 审批通过链路
  - 不能破坏现有 Emby webhook 对 `APPROVED -> INGESTED` 的回写逻辑

## 影响范围

涉及的子系统：

- API：有
  - 订阅模型、重复检查逻辑、重新发起接口、SQL migration
- Web：有
  - `REJECTED` 卡片的再次提交入口、重新发起表单与上下文展示
- Bot：无
  - 本次不改 Bot 审批交互
- 配置/部署：无新增环境变量
- 文档：需要更新
  - `docs/system-architecture.md`

## 验证方式

### 编译/测试

- `cd services/api && go test ./...`
- `cd services/api && go build ./...`
- `cd services/web && npm run build`

按改动补充针对性测试：

- API：活跃状态重复检查、`REJECTED` 重提创建、`retryFromId` 写入
- Web：`再次提交` 入口、拒绝原因展示、本次说明必填

### 手工验证

- 用户有一条 `REJECTED` 订阅时，卡片上可见 `再次提交`
- 点击后能看到上次拒绝原因
- 不填写本次说明时不能提交
- 填写说明后会生成一条新的 `PENDING` 记录，旧 `REJECTED` 记录保留
- 若同一作品已有 `PENDING / APPROVED / INGESTED` 记录，重新发起会被阻止

## 落地后文档处理

落地后应同步处理：

- 将稳定结论同步到 `docs/system-architecture.md`
  - `subscriptions.retryFromId`
  - 活跃状态唯一约束
  - 拒绝后重新发起流程
- 功能落地、编译验证和手工链路验证完成后，将本方案迁入 `docs/archive/plan/media-subscription/`
