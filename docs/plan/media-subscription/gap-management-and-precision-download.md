# 缺集管理与精准补集实现方案

> 状态：草稿
> 负责人：Ember
> 更新时间：2026-04-18

## 背景

这个问题为什么现在要解决：

- Ember 已有求片订阅、追剧日历和 Emby 入库 webhook，但还没有“库内已上线剧集缺口”的独立治理链路。
- 用户求片处理的是“想看但库里没有”，缺集治理处理的是“库里已有该剧，但已播集断更、漏集或补档不完整”，两者不是同一问题。
- 如果后续要把 TV Calendar、MoviePilot 和入库 webhook 收口成可维护的媒体治理闭环，必须先把缺集工单边界独立出来。

## 目标

本方案要实现：

1. 为 Emby 库中的连载剧建立独立的缺集工单模型，识别 TMDB 已播但库内未入的剧集缺口。
2. 在管理后台提供缺集列表、搜索候选、人工下发、入库核销的完整运维链路。
3. 为后续“整季包精准补集”预留清晰扩展边界，但不把下载器接管能力绑进首版主链路。

## 非目标

本次明确不做：

- 不把缺集工单与用户求片订阅合并为同一个模型或同一张表。
- 不把 TV Calendar 改造成缺集事实表；追剧展示与后台工单继续分离。
- 不在首版接入 qBittorrent / Transmission 截胡、文件优先级控制或下载器托管。
- 不在首版默认对全库所有历史完结剧做长期全量扫描；默认范围以 Emby 中 `Continuing` 且可识别 `tmdbId` 的剧集为主。

## 当前事实

以当前代码和现行文档为准，先写清现状：

- 相关文档：
  - `docs/system-architecture.md`
- 相关服务/页面/模型：
  - `services/api/internal/services/tvcalendar/service.go`
  - `services/api/internal/handlers/tv_calendar.go`
  - `services/api/internal/services/subscription/service.go`
  - `services/api/internal/services/subscription/existing.go`
  - `services/api/internal/integrations/moviepilot/client.go`
  - `services/api/internal/models/tv_calendar.go`
  - `services/web/src/views/console/TVCalendarView.vue`
  - `services/web/src/views/admin/MediaQualityView.vue`
- 当前行为：
  - `TVCalendar` 已能发现 Emby 中的连载剧、按周同步 TMDB 季集信息、展示 `ready / missing / today / upcoming` 状态，并在 Emby webhook 到达时点亮已入库剧集。
  - `subscription` 已有独立的用户求片流程，审批通过后会调用 MoviePilot 创建订阅，并在 Emby webhook 到达时回写为 `INGESTED`。
  - Emby webhook 当前是统一入库入口，已经同时服务 TV Calendar 和求片订阅入库回写。
  - MoviePilot 集成当前仅支持“创建订阅”，不支持“搜索候选”和“下发指定候选资源”。
- 现有限制：
  - 现有 `tv_calendar_items` 是围绕日期窗口和追剧展示设计，不适合作为后台缺集工单真相源。
  - 现有库内剧集库存判断未显式处理 `IndexNumberEnd` 这类多集合并文件，缺集扫描若直接沿用现状，容易出现稳定误报。
  - 后台还没有独立的缺集管理页，服主无法集中查看缺口、忽略误报或人工推进补货。
  - 缺集治理的真正目标是“已开始追的季出现断层”，不是“把整部剧历史上没收的所有季都算成缺集”。

## 方案设计

### 1. 用户可见行为

- 新增能力：
  - 管理后台新增“缺集管理”页面，作为独立于“追剧日历”和“订阅管理”的后台运维能力。
  - 页面展示缺集工单列表，支持按剧名、状态、播出日期筛选，并展示剧名、季集、播出日期、状态、最近搜索/下发时间等信息。
  - 管理员可以对单条缺集执行“搜索候选”“下发补货”“忽略工单”动作。
  - 聚合视图中的剧卡底部动作明确绑定“当前选中”的单条工单，文案使用“搜索当前集 / 忽略当前集”，避免与整剧动作混淆。
  - “搜索当前集”进入候选弹窗后，管理员在同一弹窗内完成候选查看、重新搜索与确认下发；不再额外保留独立的“下发当前集”入口。
  - 聚合视图中的季级动作仅保留“忽略本季缺集”，用于整季范围收口；搜索与下发统一走“选中单条工单后进入候选弹窗处理”的主路径，不再提供“本季首条”快捷入口。
  - 聚合视图默认只展示 `MISSING / SEARCHED / REQUESTED` 的待处理缺口；`IGNORED / INGESTED` 不再以灰色集数 chip 混入季内主列表，只在卡片摘要中保留统计。
- 首版范围：
  - 首版优先支持单条工单搜索与下发，批量操作仅保留“批量忽略”。
  - 全库扫描与单剧补扫均由后台触发；单剧补扫可同步返回，全库扫描按后台任务处理。
- 二阶段预留：
  - 若首版链路稳定，再扩展“整季包精准补集”，在“下发补货”阶段增加下载器文件筛选或截胡能力。
  - 二阶段能力仍挂在缺集工单下游，不反向污染扫描、列表和核销模型。
- 必须保持不变的现有行为：
  - 用户求片订阅工作流继续独立存在，不与缺集工单混表、混状态、混页面。
  - TV Calendar 继续负责追剧展示与用户侧订阅视图，不承担后台缺集工单职责。
  - 现有 Emby webhook 路径和订阅入库回写逻辑保持可用，只在原入口上补充缺集核销。
- 前端约束：
  - 前端实现必须遵守 Ember 风格。
  - 设计与交互基线以 `docs/reference/web-design-guide.md` 为准。
  - 缺集管理页按后台列表页标准骨架实现：`EmberPageHeaderCard` + `EmberFilterPanel` + `EmberTableCard`。
  - 首版不做海报墙式自由布局，不把“搜索候选列表”做成重视觉编排页。

### 2. 数据与模型

- 新增 `media_gaps` 表，作为缺集工单唯一事实表。
- 建议字段：
  - `id`
  - `tmdbId`
  - `embySeriesId`
  - `seriesName`
  - `season`
  - `episode`
  - `airDate`
  - `status`：`MISSING`、`SEARCHED`、`REQUESTED`、`INGESTED`、`IGNORED`
  - `searchSnapshot`：最近一次搜索候选摘要，JSON 字符串
  - `dispatchSnapshot`：最近一次下发摘要，JSON 字符串
  - `lastScannedAt`
  - `lastSearchedAt`
  - `requestedAt`
  - `ingestedAt`
  - `ignoredAt`
  - `ignoreReason`
  - `createdAt`
  - `updatedAt`
- 约束建议：
  - `tmdbId + season + episode` 唯一，防止同一缺口重复建单。
  - `status + airDate` 建索引，支撑后台列表查询。
  - `embySeriesId` 建索引，便于 webhook 按 `seriesId` 回退核销。
- 状态约束：
  - 主链路状态为 `MISSING -> SEARCHED -> REQUESTED -> INGESTED`。
  - `IGNORED` 为旁路终态，不参与自动推进。
  - 扫描时不允许把 `REQUESTED` 或 `IGNORED` 直接覆盖回 `MISSING`。
  - webhook 核销只在首次成功时写入 `ingestedAt`，保证幂等。
- 扫描边界约束：
  - 缺集扫描只针对 Emby 库中“已激活季”进行。
  - “已激活季”定义为：该剧该季在 Emby 中至少存在 1 集物理媒体。
  - 完全没有任何本地集数的季，不纳入缺集工单。
  - 旧规则产生的非激活季 `MISSING / SEARCHED` 工单，在重扫时应自动收口，避免历史误报持续污染列表。
- 模型边界：
  - 不复用 `tv_calendar_items` 做缺集事实表。
  - 不修改 `subscriptions` 语义，不新增“缺集订阅”一类混合状态。
- 是否需要迁移：
  - 需要新增 GORM 模型。
  - 需要新增 SQL migration，文件放在 `infrastructure/database/`。
  - migration 默认要求幂等，并写明新增表、索引、约束和快照字段用途。

### 3. 接口与边界

- 新增领域边界：
  - 新增 `services/api/internal/services/mediagap/`，负责缺集扫描、工单查询、搜索快照、下发状态和 webhook 核销。
  - 新增 `services/api/internal/handlers/media_gap.go`，承接后台管理接口。
  - `TVCalendar` 继续负责追剧展示、连载剧发现和周视图同步。
  - `MoviePilot` 集成继续负责对外 HTTP 调用，但扩展“搜索候选”和“下发候选”能力。
- 后台 API：
  - `POST /api/v1/admin/media-gaps/scan`
    - 触发缺集扫描。
    - 支持可选 `tmdbId`；传入时表示单剧补扫，不传表示全库扫描。
  - `GET /api/v1/admin/media-gaps`
    - 获取缺集列表。
    - 支持 `keyword`、`status`、`airDateFrom`、`airDateTo`、`page`、`pageSize`。
  - `POST /api/v1/admin/media-gaps/:id/search`
    - 调用 MoviePilot 搜索候选并更新 `searchSnapshot`。
  - `POST /api/v1/admin/media-gaps/:id/dispatch`
    - 选择一个候选资源下发补货，并写入 `dispatchSnapshot`。
  - `POST /api/v1/admin/media-gaps/:id/ignore`
    - 将工单标记为 `IGNORED`。
- 现有接口变更：
  - `POST /api/v1/webhooks/emby` 路径不变，但处理逻辑增加缺集工单核销。
- 请求与响应约束：
  - Ember API 响应统一使用 `data` 字段。
  - 首版搜索接口返回候选摘要，不把 MoviePilot 原始响应直接透传给前端。
  - 首版下发接口接受“候选标识 + 关键摘要”或“当前选择候选快照”，不要求前端自行构造完整下载请求。
- 调用方影响：
  - API：新增 `mediagap` handler / service / model / migration，并扩展 MoviePilot 客户端。
  - Web：新增缺集管理页、后台菜单、API 请求与类型定义。
  - Bot：首版无强制改动；如后续要做运维通知，再单独纳入计划。

### 4. 关键流程

按顺序写清主链路，不需要贴代码：

1. 管理员在后台触发缺集扫描。
2. `mediagap` 服务从 Emby 中拉取候选剧集，默认仅扫描 `Continuing` 且具备 `tmdbId` 的剧集。
3. 扫描服务按剧集拉取 Emby 实际剧集库存，先识别哪些季属于“已激活季”。
4. 扫描服务按剧集拉取 TMDB 剧集详情与季详情，只对“已激活季”里 `airDate < today` 的已播集做缺口对账。
5. 管理员在“缺集管理”页查看工单，对单条缺集执行 MoviePilot 搜索。
6. API 调用 MoviePilot 搜索接口，提炼候选摘要并写入 `searchSnapshot`，工单状态更新为 `SEARCHED`。
7. 管理员选择候选资源执行下发，API 调用 MoviePilot 下载下发接口，工单状态更新为 `REQUESTED`。
8. Emby webhook 收到真实入库事件后，先继续回写现有 `subscription` 和 `TVCalendar` 状态，再由 `mediagap` 按 `tmdbId + season + episode` 或 `seriesId + season + episode` 命中工单并核销为 `INGESTED`。
9. 若二阶段开启精准补集，则在第 7 步之后由独立下载器集成接管“文件筛选 / 截胡”，但不改变第 1 到第 8 步的工单主链路。

### 5. 失败路径与边界条件

- TMDB 数据缺失、季详情异常或单季请求失败：
  - 跳过当前剧或当前季并记录日志，不阻断整次扫描。
- Emby 条目缺少 `tmdbId`：
  - 不纳入默认扫描范围；如需补录或特殊处理，后续单独设计，不在首版兜底猜测。
- 多集合并文件：
  - 库存对账必须支持 `IndexNumberEnd` 范围展开，避免把一个文件覆盖多集的情况误判为缺口。
- 完全未开始收集的季：
  - 不纳入缺集工单；这属于“整季未收录”，不是“缺集断层”。
- 旧规则残留的非激活季误报：
  - 在后续重扫时应自动收口，避免同一部剧的历史季长期占满列表。
- `season = 0` 的 specials：
  - 首版默认跳过，不纳入缺集工单。
- 忽略工单：
  - `IGNORED` 工单在后续扫描仍然缺失时不自动恢复，避免反复打扰。
- 搜索失败：
  - 保留原工单，不进入 `REQUESTED`；若此前未搜索成功，状态保持 `MISSING`。
- 下发成功但长时间未入库：
  - 工单保持 `REQUESTED`，不能伪装为完成。
- webhook 重复到达：
  - `INGESTED` 状态只写一次，保证幂等。
- 全库扫描耗时较长：
  - 不要求通过单次同步 HTTP 请求完成；允许以后台任务方式推进。
- 兼容性约束：
  - 不破坏现有 TV Calendar 页面和接口语义。
  - 不破坏现有求片订阅审批和入库回写。
  - 不把 MoviePilot 搜索 / 下发逻辑反向耦合到扫描服务内部。

## 影响范围

涉及的子系统：

- API：有
  - 新增 `mediagap` 服务、handler、模型、SQL migration
  - 扩展 MoviePilot 搜索 / 下发能力
  - 在现有 Emby webhook 中补充缺集核销
  - 抽离或复用 Emby / TMDB 库存与季集对账能力
- Web：有
  - 新增后台缺集管理页
  - 新增后台路由、菜单、API 方法、类型定义
- Bot：无
  - 首版不改 Bot；后续若加运维通知，再单独评估
- 配置/部署：当前阶段无新增强制配置
  - 二阶段若接入下载器截胡，再新增下载器配置入口
- 文档：需要更新
  - `docs/system-architecture.md`

## 验证方式

### 编译/测试

- `cd services/api && go test ./...`
- `cd services/api && go build ./...`
- `cd services/web && npm run build`

按改动补充针对性测试：

- 缺集扫描对账逻辑
- 已激活季识别逻辑
- `IndexNumberEnd` 多集合并库存展开
- 非激活季不生成缺集工单
- 重扫时旧规则误报工单的自动收口
- 工单扫描幂等与状态保留规则
- Emby webhook 缺集核销幂等
- MoviePilot 搜索与下发客户端
- handler 参数校验与错误返回

### 手工验证

- 选择一部 Emby 中存在、TMDB 已播且库内缺集的连载剧，执行单剧补扫，确认只对已开始收集的季生成缺集工单。
- 准备一部“只收了第 5 季、前 4 季完全没收”的剧，确认扫描后不会把前 4 季全部打成缺集。
- 对同一剧集重复扫描，确认不会生成重复工单，且 `REQUESTED / IGNORED` 状态不会被错误覆盖。
- 对单条缺集执行搜索，确认页面能看到候选资源摘要且工单进入 `SEARCHED`。
- 选择候选资源执行下发，确认工单进入 `REQUESTED`。
- 模拟对应剧集入库 webhook，确认现有订阅 / TV Calendar 逻辑仍然正常，同时缺集工单进入 `INGESTED`。
- 对一条误报工单执行忽略，确认后续扫描不重复激活该工单。
- 准备一个单文件覆盖多集的样本，确认不会被误判为多个缺口。

## 当前推进状态

- 已完成：
  - 方案边界收口
  - 首版与二阶段拆分
  - 数据模型、状态机、接口草案收口
- 剩余项：
  - `mediagap` 模型与 migration
  - 扫描服务与共享库存对账能力
  - MoviePilot 搜索 / 下发扩展
  - 后台缺集管理页
  - 测试、编译验证和架构文档同步
- 归档条件：
  - 首版“扫描 -> 搜索 -> 下发 -> webhook 核销”链路完成
  - 相关编译 / 测试通过
  - `docs/system-architecture.md` 已同步

## 落地后文档处理

落地后应同步处理：

- 将稳定结论同步到 `docs/system-architecture.md`
  - `media_gaps` 模型与状态机
  - TV Calendar、Media Gap、Subscription、Emby webhook 之间的职责边界
  - MoviePilot 搜索 / 下发扩展能力
- 如果二阶段要接入下载器截胡，优先新增独立计划文档，不继续无限膨胀本计划正文。
- 首版完成并完成文档同步后，将本计划移入 `docs/archive/plan/media-subscription/`。
