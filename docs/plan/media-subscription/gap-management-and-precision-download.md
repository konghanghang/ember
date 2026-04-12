# 缺集管理与精准补集实现方案

> 状态：草稿
> 负责人：Ember
> 更新时间：2026-04-12

## 背景

这个问题为什么现在要解决：

- Ember 现在有求片订阅和追剧日历，但还没有“库里明明缺了已播剧集，系统能主动识别并补货”的链路。
- 用户求片解决的是“想看但没资源”，缺集治理解决的是“本来就在追，但库里断更或漏集”，两者不是一回事。
- 如果未来要把 TV Calendar、MoviePilot 和入库 webhook 做成媒体治理闭环，缺集管理是必需中间层。

## 目标

本方案要实现：

1. 识别 Emby 库中已上线剧集与 TMDB 已播数据之间的缺口，生成可管理的缺集工单。
2. 允许管理员对缺集触发 MoviePilot 搜索与补货下发。
3. 为后续“整季包精准下载”预留能力边界，但不破坏现有订阅主链路。

## 非目标

本次明确不做：

- 不把缺集管理和用户求片订阅混成一个模型。
- 不在首版就做全自动无人值守补货；保留管理员确认动作。
- 不在首版强制接管所有下载器；精准下载能力按阶段推进。

## 当前事实

以当前代码和现行文档为准，先写清现状：

- 相关文档：
  - `docs/system-architecture.md`
  - `docs/public/features/subscriptions.md`
- 相关服务/页面/模型：
  - `services/api/internal/services/tvcalendar/service.go`
  - `services/api/internal/handlers/tv_calendar.go`
  - `services/api/internal/integrations/moviepilot/client.go`
  - `services/web/src/views/console/TVCalendarView.vue`
  - `services/web/src/views/console/LibraryView.vue`
- 当前行为：
  - TV Calendar 负责追剧日历和已入库状态展示，但范围基于关注和日历链路，不是全库缺集治理。
  - MoviePilot 集成当前只支持审批通过后创建订阅，不支持缺集搜索和下载下发。
  - Emby webhook 已能点亮日历状态，但不会产出“缺集工单”模型。
- 现有限制：
  - 服主无法集中看到“哪些剧缺了哪些集”。
  - 缺集处理完全依赖人工发现和外部工具，不在 Ember 内收口。
  - 订阅状态和缺集治理没有边界划分，容易被误用成同一个功能。

## 方案设计

### 1. 用户可见行为

- 新增能力：
  - 管理后台增加“缺集管理”页面，展示缺集列表、状态、搜索候选和补货动作。
  - 缺集项支持 `缺失 / 已搜索 / 已下发 / 已入库 / 已忽略` 等状态。
  - 管理员可对单条或整组选中项触发 MoviePilot 搜索和下载下发。
- 修改现有行为：
  - TV Calendar 仍负责用户侧追剧视图；缺集治理作为后台运维能力独立存在。
- 哪些现有行为必须保持不变：
  - 用户求片订阅工作流保持独立，不与缺集工单混表。
  - 现有 TV Calendar 页面、Emby webhook 和 MoviePilot 订阅审批逻辑保持可用。
- 前端约束：
  - 前端实现必须遵守 Ember 风格。
  - 设计与交互基线以 `docs/reference/web-design-guide.md` 为准。
  - 新页面采用后台列表页标准骨架，不做海报墙式自由布局。
  - 若后续加入候选资源弹层，仍需遵守现有表单和筛选规范。

### 2. 数据与模型

- 新增 `media_gaps` 表：
  - `id`
  - `tmdbId`
  - `seriesName`
  - `season`
  - `episode`
  - `airDate`
  - `status`：`MISSING`、`SEARCHED`、`REQUESTED`、`INGESTED`、`IGNORED`
  - `embySeriesId`
  - `searchSnapshot`：最近一次搜索结果摘要，JSON
  - `requestedAt`
  - `ingestedAt`
  - `createdAt`
  - `updatedAt`
- 约束建议：
  - `tmdbId + season + episode` 唯一
  - `status + airDate` 索引，便于后台查询
- 是否需要迁移：
  - 需要 SQL migration，放在 `infrastructure/database/`
  - 需要新增 GORM 模型和扫描服务

### 3. 接口与边界

- 新增或修改哪些 API / Internal API / webhook / 命令：
  - `POST /api/v1/admin/media-gaps/scan`
    - 触发缺集扫描
  - `GET /api/v1/admin/media-gaps`
    - 获取缺集列表
  - `POST /api/v1/admin/media-gaps/:id/search`
    - 触发对单条缺集的 MoviePilot 搜索
  - `POST /api/v1/admin/media-gaps/:id/dispatch`
    - 选择候选资源并下发到 MoviePilot
  - `POST /api/v1/admin/media-gaps/:id/ignore`
    - 忽略误报或暂不处理项
  - `POST /api/v1/webhooks/emby`
    - 在现有入库 webhook 里补充缺集自动核销逻辑
- 请求参数与响应字段怎么变：
  - Ember 自身 API 使用统一 `data` 字段
  - MoviePilot 集成侧需要扩展搜索和下载下发客户端能力
- 哪些调用方会受影响：
  - API 的 TVCalendar / MoviePilot 集成
  - 后台新增缺集管理页
  - 可选的 Bot 运维通知

### 4. 关键流程

按顺序写清主链路，不需要贴代码：

1. 管理员触发缺集扫描。
2. 系统对比 Emby 实际已入库剧集和 TMDB 已播剧集，生成或更新 `media_gaps` 记录。
3. 管理员在缺集页查看结果，选择某条缺集执行 MoviePilot 搜索。
4. API 调用 MoviePilot 搜索接口，保存候选结果摘要。
5. 管理员从候选中选择资源并下发下载，缺集状态更新为 `REQUESTED`。
6. Emby webhook 收到真实入库事件后，命中对应季集，缺集状态更新为 `INGESTED`。
7. 若后续开启精准下载扩展，再在“下发下载”阶段接入下载器文件筛选。

### 5. 失败路径与边界条件

- TMDB 数据缺失或季集结构异常：缺集扫描跳过该条并记录日志，不阻断全局扫描。
- MoviePilot 搜索失败：缺集保留在 `MISSING` 或 `SEARCHED`，等待重试。
- 下载下发成功但迟迟未入库：缺集保持 `REQUESTED`，不伪装成已完成。
- webhook 重复到达：`INGESTED` 状态只允许首次写入，避免重复核销。
- 兼容性约束：
  - 不能让缺集工单和用户订阅工单混用同一张表。
  - 不能破坏现有 TV Calendar 和订阅入库确认逻辑。

## 影响范围

涉及的子系统：

- API：有
  - 缺集模型、扫描服务、MoviePilot 搜索/下发扩展、webhook 核销
- Web：有
  - 新增后台缺集管理页
- Bot：可选
  - 后台运维提醒
- 配置/部署：可能有
  - 若后续接入精准下载，需要新增下载器配置
- 文档：需要更新
  - `docs/system-architecture.md`

## 验证方式

### 编译/测试

- `cd services/api && go test ./...`
- `cd services/api && go build ./...`
- `cd services/web && npm run build`

按改动补充针对性测试：

- 缺集扫描比对逻辑
- webhook 核销幂等
- MoviePilot 搜索与下载下发客户端

### 手工验证

- 选择一部已播但库内缺集的剧，扫描后确认生成缺集记录
- 对单条缺集触发搜索，确认可看到候选资源
- 选择候选资源下发后，状态变为 `REQUESTED`
- 模拟对应剧集入库 webhook，确认状态变为 `INGESTED`
- 忽略一条误报缺集后，确认列表可正确隐藏或标识

## 落地后文档处理

落地后应同步处理：

- 将稳定结论同步到 `docs/system-architecture.md`
  - 缺集模型
  - 缺集扫描与 webhook 核销关系
  - MoviePilot 搜索/下发扩展能力
- 如面向用户有可见影响，再补充 public feature 文档
- 主体稳定后移入 `docs/archive/plan/media-subscription/`
