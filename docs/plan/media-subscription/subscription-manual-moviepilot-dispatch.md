# 订阅手动补偿下载实现方案

> 状态：草稿
> 负责人：Ember
> 更新时间：2026-06-09

## 背景

这个问题为什么现在要解决：

- Ember 当前求片订阅流程是用户提交订阅，管理员审核通过后由系统把订阅交给 MoviePilot 自动处理。
- MoviePilot 自动订阅链路会使用订阅规则、订阅过滤组和订阅自身参数，可能出现“管理员已经同意，但 MoviePilot 自动搜索不到可下载资源”的情况。
- 管理员需要一个轻量的人工兜底入口：基于 Ember 已知的 `tmdbId` 主动搜索候选资源，选中一个可用资源后直接交给 MoviePilot 下载。

## 目标

本方案要实现：

1. 在订阅管理中增加管理员手动补偿下载能力，只服务已通过但未入库的订阅。
2. 手动补偿搜索优先使用 MoviePilot 的 TMDB 精确搜索能力，避免重新按标题模糊猜测媒体。
3. 抽出 MoviePilot 通用搜索和下载方法，让订阅手动补偿与缺集下发共用下载入口。
4. 保持首版简单，不新增 `subscriptions` 字段，不新增订阅状态，不引入新的持久化表。

## 非目标

本次明确不做：

- 不实现 Ember 自己的资源站搜索、索引器、下载器或规则引擎。
- 不调用 MoviePilot 的订阅搜索接口作为手动补偿入口。
- 不新增 `subscriptions` 字段记录搜索快照、下发快照或手动补偿历史。
- 不改变现有订阅状态机，手动下发后订阅仍保持 `APPROVED`，等待 Emby webhook 收口为 `INGESTED`。
- 不把订阅手动补偿混入缺集工单模型。
- 不为整剧订阅自动盲下整剧资源；首版需要管理员明确季范围后再搜。

## 当前事实

以当前代码和现行文档为准，先写清现状：

- 相关文档：
  - `docs/system-architecture.md`
  - `docs/reference/web-design-guide.md`
  - `docs/archive/plan/media-subscription/gap-management-and-precision-download.md`
- 相关服务/页面/模型：
  - `services/api/internal/services/subscription/service.go`
  - `services/api/internal/handlers/subscription.go`
  - `services/api/internal/services/mediagap/service.go`
  - `services/api/internal/handlers/media_gap.go`
  - `services/api/internal/integrations/moviepilot/client.go`
  - `services/api/internal/models/subscription.go`
  - `services/web/src/views/console/SubscriptionsView.vue`
  - `services/web/src/views/admin/MediaGapsView.vue`
  - `services/web/src/api/admin.ts`
  - `services/web/src/types/api.ts`
- 当前行为：
  - 管理员批准订阅后，Ember 调用 MoviePilot `POST /api/v1/subscribe/` 创建订阅；MoviePilot 后续按自身订阅链路搜索和下载。
  - 订阅调用失败时，Ember 写入现有 `mpError`；状态仍保持 `APPROVED`。
  - 管理员可通过 `redispatch` 重试 MoviePilot 订阅创建，但这仍然是重试订阅入口，不是人工选择资源下载。
  - 缺集管理当前搜索走 `GET /api/v1/search/title?keyword=<剧名 SxxEyy>`，无结果时回退 `<剧名 Sxx>`，没有使用 `tmdbId` 精确搜索。
  - 缺集管理当前下发走 MoviePilot `POST /api/v1/download/add`，但请求体只传 `torrent_in`，没有携带 `tmdbid`。
- 现有限制：
  - 订阅自动搜索失败后，管理员缺少在 Ember 内手动补偿下载的入口。
  - `redispatch` 只能重试 MoviePilot 订阅创建，无法绕开订阅规则让管理员人工选择候选。
  - 缺集和未来订阅手动补偿都需要下发下载，但当前 MoviePilot client 的下发方法命名和请求结构偏缺集专用。

## 方案设计

### 1. 用户可见行为

- 新增能力：
  - 管理员在订阅列表中，对 `APPROVED` 且未 `INGESTED` 的订阅执行“手动下载”。
  - 弹窗内按订阅的 `tmdbId` 搜索 MoviePilot 候选资源。
  - 管理员从候选列表中选择一个资源并确认下发。
  - 下发成功后提示“已下发，等待入库”；订阅状态不立即变更。
- 修改现有行为：
  - 缺集下发和订阅手动补偿下发统一通过 MoviePilot 通用下载方法执行。
  - 缺集下发时应附带 `tmdbid`，提高 MoviePilot 下载阶段识别准确性。
- 哪些现有行为必须保持不变：
  - 用户提交订阅、管理员审批、Bot 审批通知和 Emby webhook 入库收口保持现有语义。
  - `redispatch` 继续只表达“重试 MoviePilot 订阅创建”，不改造成手动补偿。
  - 缺集搜索首版仍保持单集关键词优先，不强行改为 TMDB 季级搜索。
- 前端约束：
  - 前端实现必须遵守 Ember 风格。
  - 设计与交互基线以 `docs/reference/web-design-guide.md` 为准。
  - 订阅页已有控制台列表和管理员操作入口，首版优先在 `SubscriptionsView.vue` 中增加轻量弹窗，不新增独立页面。
  - 候选列表交互可参考 `MediaGapsView.vue` 的候选弹窗，但不要复制成重视觉搜索页。

### 2. 数据与模型

> 本次不涉及数据模型变更。

- 不新增 `subscriptions` 字段。
- 不新增 SQL migration。
- 不新增订阅状态。
- `mpError` 继续只表示自动 MoviePilot 订阅创建链路的失败：
  - 手动下发成功后可清空 `mpError`，避免 UI 持续显示旧的自动订阅失败。
  - 手动下发失败不写回 `mpError`，只通过 API 同步返回错误。
  - 原因：`redispatch` 的可见条件依赖 `mpError` 非空，但它重试的是自动订阅创建，不是手动下载下发。

### 3. 接口与边界

- MoviePilot client 建议抽成通用能力：
  - `CreateSubscription(req)`：现有订阅审核通过后使用。
  - `SearchMediaCandidates(req)`：按 `tmdbId + mediaType + season` 精确搜索，订阅手动补偿使用。
  - `SearchTitleCandidates(req)`：按关键词搜索，缺集搜索继续使用。
  - `DispatchDownloadCandidate(req)`：统一下发下载，缺集和订阅手动补偿共用。
- 现有缺集方法保留为业务封装：
  - `SearchGapCandidates(req)` 继续先搜 `<剧名 SxxEyy>`，再回退 `<剧名 Sxx>`。
  - `DispatchGapCandidate(req)` 可以作为兼容包装，内部调用通用下载方法。
- 新增后台 API：
  - `POST /api/v1/admin/subscriptions/:id/manual-search`
    - 只允许管理员调用。
    - 只允许 `APPROVED` 状态订阅。
    - 电影调用 MoviePilot `GET /api/v1/search/media/tmdb:<tmdbId>?mtype=movie`。
    - 电视剧指定季调用 MoviePilot `GET /api/v1/search/media/tmdb:<tmdbId>?mtype=tv&season=N`。
    - 整剧订阅 `season=0` 首版不直接搜索下发；需要请求中显式传入季号，或返回错误提示管理员先选择季。
  - `POST /api/v1/admin/subscriptions/:id/manual-dispatch`
    - 接收管理员选择的候选 `candidatePayload`。
    - 整剧订阅必须同时提交搜索时使用的 `season`，避免搜索 / 下发两阶段季号脱节。
    - 调用 MoviePilot `POST /api/v1/download/add`。
    - 请求体带 `torrent_in`、`tmdbid`，电视剧下发同时带 `season`。
- 响应约束：
  - Ember API 响应统一使用 `data` 字段。
  - 搜索响应返回候选摘要，不把 MoviePilot 原始大响应直接裸透给前端。
  - 下发响应只表达是否已提交下载，不承诺已入库。

### 4. 关键流程

按顺序写清主链路，不需要贴代码：

1. 用户通过 Web 或 Bot 提交求片订阅。
2. 管理员审核通过订阅。
3. Ember 创建 MoviePilot 订阅，MoviePilot 按订阅规则执行自动搜索和下载。
4. 若 MoviePilot 自动订阅链路未找到资源，订阅停留在 `APPROVED`。
5. 管理员在订阅列表点击“手动下载”。
6. Ember API 读取订阅并校验管理员权限、订阅状态和季号边界。
7. Ember API 调 MoviePilot 精确搜索接口 `/api/v1/search/media/tmdb:<tmdbId>`，传入 `mtype` 和必要的 `season`。
8. 前端展示候选列表，默认按 MoviePilot 返回顺序和 Ember 轻量排序展示。
9. 管理员选择一个候选并确认下发。
10. Ember API 调 MoviePilot `/api/v1/download/add`，请求体带 `torrent_in`、`tmdbid`，电视剧下发同时带 `season`。
11. 下发成功后订阅保持 `APPROVED`，等待 Emby webhook 按现有逻辑收口为 `INGESTED`。

### 5. 失败路径与边界条件

- MoviePilot 未配置：返回现有上游错误语义，不展示手动候选。
- 订阅不存在：返回 404。
- 订阅不是 `APPROVED`：返回 409 或 400，避免对待审核、已拒绝、已入库订阅重复下发。
- 电影缺少有效 `tmdbId`：返回 400。
- 电视剧 `season=0` 且未显式传入季号：返回 400，要求管理员选择季后再搜。
- MoviePilot 精确搜索返回空：前端展示空候选；首版不自动 fallback 到 title 搜索，避免偏离“严格按 tmdbId”的设计。
- MoviePilot 搜索过滤规则组：
  - 手动补偿依赖 MoviePilot “搜索和下载”菜单下的搜索过滤规则组。
  - 若管理员希望放宽候选，应在 MoviePilot 中不勾选搜索过滤规则组。
  - Ember 不在首版绕过 MoviePilot 的搜索过滤配置。
- 下发失败：返回脱敏错误；不写回 `mpError`，不改变订阅状态。
- 下发成功但长时间未入库：订阅继续保持 `APPROVED`，不伪装完成。
- 兼容性约束：
  - 不能破坏现有审批、通知、`redispatch` 和 webhook 入库回写。
  - 不能把手动补偿动作自动化成静默下载；必须由管理员确认候选。

## 影响范围

涉及的子系统：

- API：有
  - 扩展 `MoviePilotClient` 的通用搜索和下载方法。
  - 新增订阅手动搜索和手动下发 service / handler 方法。
  - 调整缺集下发，让 `/download/add` 携带 `tmdbid`。
- Web：有
  - `SubscriptionsView.vue` 增加管理员手动下载入口和候选弹窗。
  - `api/admin.ts` 和 `types/api.ts` 增加对应请求与响应类型。
- Bot：无
  - 首版不在 Bot 管理员审批消息里加手动补偿入口。
- 配置/部署：无新增 Ember 环境变量
  - 但使用效果受 MoviePilot “搜索和下载”过滤规则组配置影响。
- 数据库：无
  - 不新增字段，不需要 migration。
- 文档：需要更新
  - `docs/system-architecture.md`
  - 必要时补充 `docs/reference/api-endpoint-catalog.md`

## 验证方式

### 编译/测试

- `cd services/api && go test ./...`
- `cd services/api && go build ./...`
- `cd services/web && npm run build`

按改动补充针对性测试：

- MoviePilot client 精确搜索 URL、查询参数和 `X-API-KEY` 请求头。
- MoviePilot client `/download/add` 请求体包含 `torrent_in` 和 `tmdbid`。
- 订阅手动搜索只允许 `APPROVED`。
- 电视剧整剧订阅未指定季号时拒绝搜索。
- 下发失败时错误脱敏且不改变订阅状态。
- 缺集下发传入 `gap.TmdbID`。

### 手工验证

- 电影订阅处于 `APPROVED` 后，执行手动下载，确认请求按 `tmdbId + movie` 搜索并返回候选。
- 指定季电视剧订阅处于 `APPROVED` 后，确认请求按 `tmdbId + tv + season` 搜索。
- 整剧订阅 `season=0` 点击手动下载，确认要求先选择季，不直接下发。
- MoviePilot 搜索和下载过滤规则组为空时，确认候选不会被搜索规则组额外过滤。
- 选择候选下发后，确认 MoviePilot 收到 `/download/add` 请求且带 `tmdbid`。
- 下载入库后，现有 Emby webhook 能继续把订阅收口为 `INGESTED`。
- 缺集管理下发一条候选，确认现有缺集状态推进不受影响。

## 落地后文档处理

落地后应同步处理：

- 将稳定结论同步到 `docs/system-architecture.md`：
  - 订阅自动链路与手动补偿链路边界。
  - MoviePilot client 通用搜索 / 下载能力。
  - 缺集下发携带 `tmdbid` 的新行为。
- 如新增 API 纳入长期使用，同步更新 `docs/reference/api-endpoint-catalog.md`。
- 功能稳定后，将本方案移入 `docs/archive/plan/media-subscription/`。
