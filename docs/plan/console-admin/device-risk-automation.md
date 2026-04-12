# 设备风控自动化实现方案

> 状态：草稿
> 负责人：Ember
> 更新时间：2026-04-12

## 背景

这个问题为什么现在要解决：

- Ember 已经有设备管理、客户端黑名单和批量下线能力，但目前仍以人工操作为主，缺少自动识别和自动告警。
- 设备风险事件现在不会沉淀成独立风险记录，难以回看“谁、何时、为什么被处置”。
- `/api/v1/webhooks/emby` 已经是稳定入口，但当前只服务于 TV Calendar 和订阅入库链路，没有承接设备与播放风控。

## 目标

本方案要实现：

1. 在 Ember 内增加自动化设备风控链路，优先覆盖“黑名单客户端在线”和“并发超限”两类场景。
2. 风险事件必须有审计记录、后台可视化总览和明确的手动处置入口。
3. 保持现有设备管理页、黑名单和设备下线接口可继续工作，不破坏既有运维入口。

## 非目标

本次明确不做：

- 不实现复杂风控规则 DSL、地理位置识别、机器学习评分。
- 不默认自动封禁用户；首版只做告警和可选自动下线。
- 不改造现有用户模型为“VIP 并发特权系统”。

## 当前事实

以当前代码和现行文档为准，先写清现状：

- 相关文档：
  - `docs/system-architecture.md`
- 相关服务/页面/模型：
  - `services/api/internal/services/device/service.go`
  - `services/api/internal/models/client_blacklist.go`
  - `services/api/internal/models/device_action.go`
  - `services/api/internal/handlers/device.go`
  - `services/api/internal/handlers/tv_calendar.go`
  - `services/web/src/views/admin/DevicesView.vue`
- 当前行为：
  - 设备页支持查看设备、查看客户端黑名单、手动注销设备、批量注销黑名单设备。
  - 黑名单与设备操作会记录到 `client_blacklists` 和 `device_actions`。
  - 系统没有自动风控扫描、没有风险专用事件表、没有风险摘要接口。
  - Emby webhook 当前不承接播放风控逻辑。
- 现有限制：
  - 风险发现依赖管理员人工盯设备页。
  - 手动下线后只能在设备操作日志里侧面回看，无法区分“普通运维动作”和“风险事件”。
  - 黑名单客户端即使重新上线，也不会自动触发处置。

## 方案设计

### 1. 用户可见行为

- 新增能力：
  - 管理后台设备页增加“风险概览”区域，展示当前活跃风险、今日风险数和最近处置结果。
  - 当发现黑名单客户端在线或并发超限时，后台出现风险事件记录，并可一键跳转到处置动作。
  - 可选地向管理员 Telegram 推送风险告警摘要。
- 修改现有行为：
  - 黑名单从“静态配置”升级为“可自动触发下线”的主动防御规则。
- 哪些现有行为必须保持不变：
  - 手动设备下线、黑名单增删、设备统计接口继续保持可用。
  - TV Calendar 与订阅 webhook 现有逻辑不能被破坏。
- 前端约束：
  - 前端实现必须遵守 Ember 风格。
  - 设计与交互基线以 `docs/reference/web-design-guide.md` 为准。
  - 风险概览区优先作为 `DevicesView` 内部扩展，不额外发明深色大屏样式。
  - 若存在偏离规范的特例，必须单独写明原因、范围和收口条件。

### 2. 数据与模型

- 新增 `device_risk_events` 表：
  - `id`
  - `type`：`blacklisted_client`、`concurrency_limit`
  - `userId`
  - `deviceId`
  - `clientName`
  - `status`：`open`、`resolved`、`ignored`
  - `severity`
  - `detail`：补充上下文，JSON 或 text
  - `handledAction`：如 `logout`、`notify_only`
  - `createdAt`
  - `handledAt`
- 新增运行期配置：
  - `DEVICE_RISK_ENABLED`
  - `DEVICE_DEFAULT_MAX_CONCURRENT`
  - `DEVICE_RISK_AUTO_LOGOUT_BLACKLISTED`
  - `DEVICE_RISK_NOTIFY_ADMIN`
- 是否需要迁移：
  - 需要 SQL migration，放在 `infrastructure/database/`
  - `client_blacklists`、`device_actions` 现有表不改结构

### 3. 接口与边界

- 新增或修改哪些 API / Internal API / webhook / 命令：
  - `GET /api/v1/admin/device-risk/summary`
    - 返回当前活跃风险、今日统计和配置快照
  - `GET /api/v1/admin/device-risk/events`
    - 返回风险事件列表
  - `POST /api/v1/admin/device-risk/config`
    - 保存风控配置
  - `POST /api/v1/admin/device-risk/scan`
    - 手动触发一次扫描
  - `POST /api/v1/webhooks/emby`
    - 在现有链路中补充播放相关事件分发，不改 URL
- 请求参数与响应字段怎么变：
  - 风险事件接口统一返回 `data`
  - 风险摘要接口返回明确统计字段，便于设备页渲染
- 哪些调用方会受影响：
  - 设备后台页
  - Emby webhook 处理逻辑
  - 可选的 Bot 告警通知

### 4. 关键流程

按顺序写清主链路，不需要贴代码：

1. 系统通过 webhook 事件或定时兜底扫描获取当前活跃会话。
2. 风控服务基于黑名单和并发阈值判断是否触发风险。
3. 若命中风险，写入 `device_risk_events`，并根据配置决定是否自动下线设备。
4. 若开启管理员告警，再向 Telegram 或站内通知写入告警摘要。
5. 管理员在设备页查看风险概览和事件列表，并使用现有“设备下线”“移出黑名单”“禁用用户”等动作收口。
6. 风险被处理后，事件更新为 `resolved`。

### 5. 失败路径与边界条件

- Emby 会话接口失败：记录日志并保留上次扫描状态，不影响其他业务。
- webhook 未提供足够上下文：回退到定时扫描兜底，不直接报错。
- 自动下线失败：风险事件仍保留为 `open`，管理员可手动处置。
- 并发波动导致短时间重复命中：同一用户同一风险类型需要做短时间幂等去重，避免刷屏。
- 兼容性约束：
  - 不能影响现有 `DevicesView` 的基础查询能力。
  - 不能让 TV Calendar webhook 因风险逻辑失败而中断主流程。

## 影响范围

涉及的子系统：

- API：有
  - 风控事件模型、扫描服务、摘要接口
  - webhook 处理扩展
- Web：有
  - 设备页风险概览与风险列表
- Bot：可选
  - 管理员告警摘要
- 配置/部署：有
  - 新增风控相关设置项
- 文档：需要更新
  - `docs/system-architecture.md`

## 验证方式

### 编译/测试

- `cd services/api && go test ./...`
- `cd services/api && go build ./...`
- `cd services/web && npm run build`

按改动补充针对性测试：

- 风控判定逻辑
- 风险事件幂等去重
- webhook 与定时扫描的兜底关系

### 手工验证

- 将某客户端加入黑名单，模拟在线会话，确认系统发现并产生风险事件
- 打开多个会话超过默认并发阈值，确认后台出现并发风险
- 自动下线开启时，确认命中黑名单可自动触发设备注销
- 自动下线关闭时，确认只告警不自动操作
- 设备页仍可正常查看黑名单、设备列表和操作日志

## 落地后文档处理

落地后应同步处理：

- 将稳定结论同步到 `docs/system-architecture.md`
  - 风控事件模型
  - webhook 与设备风控关系
- 如形成稳定运维流程，可补一份 runbook
- 主体稳定后移入 `docs/archive/plan/console-admin/`
