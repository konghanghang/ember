# 播放排行榜媒体库 allowlist 实现方案

> 状态：已完成，已归档
> 负责人：Ember
> 更新时间：2026-07-09

## 落地状态

- 已实现排行榜媒体库 allowlist 配置接口、管理员页面配置区块，以及 latest/history/preview/正式生成/Telegram 通知统一口径。
- 已将第一版方案收口为“候选扩窗 + 运行期归属缓存”，并修复“切换 allowlist 后实体归属缓存污染”的回归问题。
- 已补 `services/api/internal/services/playback/ranking_test.go`、`services/api/internal/app/integration_policy_test.go` 与 `services/web/src/integration/media-library-policy.flow.spec.ts` 覆盖配置保存与联动链路。
- 已完成 `go test ./internal/services/playback`、API/Web 集成测试与全仓 `scripts/test/all.sh` 验证。
- 稳定结论已同步到 `docs/system-architecture.md`，本稿现在只保留历史追溯价值。

## 背景

当前播放排行榜（日榜 / 周榜）用于控制台展示和 Telegram 通知，业务定位是“全站统一榜单”，不需要做到不同用户看到不同结果。但现有实现直接基于 Emby `PlaybackActivity` 全量播放记录聚合，无法控制哪些媒体库参与统计。

- 现状会把所有媒体库的播放都纳入榜单与通知，缺少“只统计指定片库”的能力。
- 管理员已经能在 Ember 中管理媒体库模板、用户媒体库偏好和质量扫描范围，但排行榜仍然没有媒体库维度的显式控制入口。
- 如果不做 allowlist，通知型排行榜会持续混入不希望对外广播的媒体库内容，后续无法通过配置收口。
- 现有第一版实现思路是“按 allowlist 逐库拉取全部条目，再构造允许参与排行的条目集合”；这条链路的成本跟库内总条目数绑定，管理员首屏、预览生成和正式榜单计算都容易被大库拖慢。

## 目标

本方案要实现：

1. 为播放排行榜增加“参与统计的媒体库 allowlist”能力，由管理员显式选择哪些 Emby 媒体库参与排行计算。
2. 统一收口排行榜相关结果：控制台“播放排行榜”页面、历史榜、预览生成、正式入库结果、Telegram 通知全部使用同一套 allowlist 口径。
3. 在不改动现有 `PlaybackActivity` 数据源和不新增数据库表的前提下完成第一版落地。
4. 第一版不再采用“逐库全量扫条目”的做法，改为“先取排行榜候选，再按 Emby 条目层级回溯归属媒体库”的轻量方案。

## 非目标

本次明确不做：

- 不把排行榜改成“每个用户按自己的可见媒体库生成一份”。
- 不修改 Emby / Playback Reporting 插件 schema，也不假设 `PlaybackActivity` 存在稳定的 `libraryId` 字段。
- 不新增本地持久化的条目归库表或长期缓存表；第一版先保证行为正确，再评估性能是否需要额外缓存。
- 不在请求链路里扫描 allowlist 下每个媒体库的全部条目。
- 不调整排行榜通知的频道、文案风格或推送频率。

## 当前事实

以当前代码和现行文档为准，现状如下：

- 相关文档：
  - [docs/system-architecture.md](../../system-architecture.md)
  - [docs/reference/web-information-architecture.md](../../reference/web-information-architecture.md)
- 相关服务/页面/模型：
  - `services/api/internal/services/playback/ranking.go`
  - `services/api/internal/handlers/ranking.go`
  - `services/web/src/views/console/RankingsView.vue`
  - `services/api/internal/integrations/emby/library.go`
  - `services/api/internal/integrations/notifier/notifier.go`
- 当前行为：
  - 排行榜直接从 Emby `PlaybackActivity` 聚合电影和剧集播放记录，不区分媒体库范围。
  - 日榜 / 周榜的正式入库、历史榜读取、预览生成和 Telegram 通知都复用同一套排行计算结果。
- 现有限制：
  - 当前 `PlaybackActivity` 读取链路只稳定识别 `ItemId`、`ItemName` 等字段，公开资料和现有测试都没有证据表明存在稳定可依赖的 `libraryId` 字段。
  - 电影榜按 `ItemId` 聚合；剧集榜先按 episode `ItemId` 聚合，再回查 Emby 条目详情按 `SeriesId` 归并。
  - 当前总播放时长 `totalDuration` 也是按全量 `PlaybackActivity` 记录统计，和未来 allowlist 语义不一致。
  - Emby 现有公开接口能够提供：
    - 顶层媒体库列表（`/Users/{adminUserId}/Views` 或等价 VirtualFolders 读取）
    - 单个条目详情（`/Items/{id}`）
    - 条目祖先链（`/Items/{id}/Ancestors`）
  - 当前仓库已经有进程内 TTL + single-flight 缓存基础设施，可复用于排行榜的“条目/剧集归属媒体库”解析缓存。

## 方案设计

### 1. 用户可见行为

- 管理员在“播放排行榜”页面新增一个“参与统计的媒体库”配置区块，用于勾选哪些 Emby 媒体库参与排行计算。
- 该区块只对管理员显示；普通用户继续只看到榜单结果。
- 未选择任何媒体库时，默认视为“全部媒体库参与统计”，保持升级兼容，不让现有榜单突然变空。
- 选择并保存 allowlist 后：
  - 排行榜页面的最新榜、历史榜、预览生成结果全部按 allowlist 口径展示。
  - 后续定时生成的日榜 / 周榜入库结果按 allowlist 口径写入。
  - Telegram 通知同步按 allowlist 口径推送。
- 必须保持不变的现有行为：
  - 排行榜仍是“全站统一榜单”，不是按用户拆分。
  - 普通用户的排行榜访问方式、路由和页面结构不因 allowlist 发生额外权限变化。

### 2. 数据与模型

本次不涉及数据模型变更。

持久化策略：

- 继续复用 `settings` 表保存排行榜 allowlist。
- 新增配置项 key：`playback_ranking_library_allowlist`
- 值格式：JSON array，例如 `["lib_movies","lib_anime"]`
- 语义：
  - 空值 / 空数组：表示统计全部媒体库
  - 非空数组：只统计配置中的媒体库

不新增 SQL migration，原因是：

- 只使用现有 `settings` 表保存配置，不引入新表、新字段、新索引。

### 3. 接口与边界

新增管理员专用接口：

1. `GET /api/v1/admin/rankings/library-allowlist`
   - 返回当前排行榜 allowlist 配置及 Emby 当前媒体库列表。
   - 响应建议：
     - `allowAll: boolean`
     - `libraryIds: string[]`
     - `libraries: MediaLibraryOption[]`

2. `PUT /api/v1/admin/rankings/library-allowlist`
   - 请求体：`{ libraryIds: string[] }`
   - 语义：
     - `[]` 表示恢复为“全部媒体库参与统计”
   - 保存前必须校验所有 `libraryIds` 都存在于当前 Emby 媒体库列表中。
   - 响应返回保存后的完整配置视图，便于前端直接回填。

复用边界：

- Emby 媒体库列表继续复用现有 `policy.Service.GetAdminMediaLibraries()` 和 `GET /api/v1/admin/media-libraries` 的同一套媒体库识别规则。
- 排行榜读取接口 `GET /api/v1/rankings/latest`、`GET /api/v1/rankings/history` 不改协议；它们读取的仍是入库结果，但生成口径改为 allowlist。
- 管理员预览接口 `POST /api/v1/admin/rankings/preview` 不改协议；它返回的内容改为 allowlist 过滤后的预览结果。
- 需要在 Emby 集成层补充或复用下面的轻量查询能力：
  - 读取管理员可见顶层媒体库（`Views`）
  - 读取单个 item/series 详情
  - 读取 item/series 祖先链

### 4. 关键流程

按链路分三部分收口。

#### 4.1 配置读取与保存

1. 管理员进入“播放排行榜”页面。
2. 前端调用新的 allowlist 读取接口。
3. 后端读取 `settings.playback_ranking_library_allowlist`，并读取 Emby 当前媒体库列表。
4. 后端返回当前 allowlist 和可选媒体库。
5. 管理员勾选并保存后，后端校验媒体库 ID 合法性，写回 `settings` 表。

配置链路约束：

- 配置读取只返回媒体库列表和当前 allowlist，不读取媒体库条目内容。
- 管理员首屏加载排行榜时，不应被“读取可选媒体库条目集合”阻塞；媒体库配置面板和榜单结果必须解耦。

#### 4.2 排行榜生成

1. 排行榜计算开始时读取当前 allowlist。
2. 如果 allowlist 为空，则沿用“全部媒体库参与统计”的兼容语义。
3. 如果 allowlist 非空，不再扫描这些媒体库下的全部条目；改为“候选扩窗 + 归属解析”。
4. 电影榜：
   - 先从 `PlaybackActivity` 取更大的 movie 聚合候选集，而不是直接只取前 `10`。
   - 对候选中的唯一 `itemId` 解析归属媒体库：
     - 优先查 `/Items/{id}`；
     - 信息不足时回退查 `/Items/{id}/Ancestors`。
   - 过滤掉不在 allowlist 中的候选项。
   - 如果过滤后不足 `10` 条，继续扩大候选窗口，直到补足或达到预设上限。
5. 剧集榜：
   - 先从 `PlaybackActivity` 取更大的 episode 聚合候选集，而不是先扫描 allowlist 下所有 episode 条目。
   - 对候选中的唯一 episode `itemId` 批量回查条目详情，拿到 `SeriesId`。
   - 先按 `SeriesId` 聚并出候选剧集榜，再对唯一 `seriesId` 解析归属媒体库：
     - 优先查 `/Items/{seriesId}`；
     - 信息不足时回退查 `/Items/{seriesId}/Ancestors`。
   - 过滤掉不在 allowlist 中的 series。
   - 如果过滤后不足 `10` 条，继续扩大 episode 候选窗口，直到补足或达到预设上限。
6. 归属解析口径：
   - 先读取管理员可见顶层媒体库列表，得到一组稳定 `libraryIds`。
   - item 或 series 的祖先链中，命中某个 `libraryId` 时，视为该内容归属该媒体库。
   - 归属无法确定时，标记为 `unknown`，默认不参与 allowlist 榜单。
7. 缓存：
   - 使用进程内 TTL + single-flight 缓存保存：
     - `library views`
     - `movie itemId -> libraryId`
     - `seriesId -> libraryId`
   - 本缓存只作为运行期优化，不落数据库，不改变持久化模型。
8. 总播放时长：
   - 改为统计过滤后实际参与榜单计算的电影 + episode 总时长；
   - 不再使用当前“全量播放记录总时长”口径。
9. 正式榜单入库和通知继续复用现有流程，只是输入结果改为 allowlist 过滤后的数据。

#### 4.3 前端展示

1. 管理员在排行榜页看到 allowlist 配置区块。
2. 普通用户不显示该区块。
3. 榜单列表本身不需要额外改协议；只显示后端已过滤的结果。
4. 配置区块文案明确说明：
   - 该配置同时影响排行榜页面、历史榜、预览生成和 Telegram 推送。
   - 未选择时默认统计全部媒体库。
5. 排行榜页首屏只读取榜单结果；媒体库配置区块采用懒加载或同等级的解耦方式，避免配置查询拖慢榜单首屏。

### 5. 失败路径与边界条件

- Emby 未配置或媒体库列表读取失败：管理员配置区块返回错误；旧榜单读取能力不应被该配置接口失败拖垮。
- allowlist 中存在已被 Emby 删除的媒体库：
  - 读取配置时应把这些 ID 识别为“失效配置”；
  - 前端应提示管理员重新保存；
  - 计算链路默认忽略不存在的库，并在日志中记录失效库 ID，避免整批生成失败。
- allowlist 非空但所有库都无可用条目：
  - 预览与正式生成都应返回空榜单；
  - Telegram 通知沿用现有空榜语义，不伪造“全库总时长”。
- 归属解析失败：
  - 单个 item / series 的详情或祖先链读取失败时，标记为 `unknown` 并继续计算其余候选。
  - 只有在候选窗口整体无法得到有效结果时，才返回空榜或错误。
- 候选扩窗后仍不足 `10` 条：
  - 允许返回不足 `10` 条的榜单；
  - 必须在日志中记录当前候选窗口、命中条数和扩窗终止原因。
- 剧集榜过滤后仅剩部分 episode：
  - 先按候选 episode 聚并 `SeriesId`，再按 series 归属过滤；
  - 不允许把未入选 allowlist 的 episode 时长算进同一部剧。
- 兼容性约束：
  - 不能破坏当前 `GET /api/v1/rankings/latest` / `history` 响应格式。
  - 不能让普通用户额外感知到“媒体库权限配置”操作入口。
  - 不能因为第一版缺少本地持久化索引就改变当前排行榜入库和通知的幂等语义。

## 影响范围

涉及的子系统：

- API：有
  - 排行榜生成服务
  - 排行榜管理员配置读取 / 保存接口
  - Emby 顶层媒体库 / item 详情 / ancestors 读取复用
  - 排行榜运行期归属缓存
- Web：有
  - `RankingsView.vue` 增加管理员 allowlist 配置区块
  - 新增排行榜 allowlist 类型与 API 封装
  - 前端实现必须遵守 Ember 风格；设计与交互基线以 [docs/reference/web-design-guide.md](../../reference/web-design-guide.md) 为准
- Bot：无独立接口改动
  - 但排行榜通知 payload 的数据口径会随榜单生成结果一起变化
- 配置/部署：有
  - 新增 `settings` 中的业务配置 key：`playback_ranking_library_allowlist`
  - 不新增环境变量，不新增 migration
- 文档：有
  - 落地后需同步 `docs/system-architecture.md`
  - 如排行榜页面职责发生稳定变化，需同步 `docs/reference/web-information-architecture.md`

## 验证方式

### 编译/测试

- `cd services/api && go test ./...`
- `cd services/api && go build ./...`
- `cd services/web && npm run test`
- `cd services/web && npm run build`

### 手工验证

- 管理员打开排行榜页，能看到当前 Emby 媒体库列表，并保存 allowlist。
- allowlist 为空时，预览榜单与现有全库行为一致。
- allowlist 只选择单个电影库时，电影榜不再混入未选库内容，剧集榜为空或仅包含选中库结果。
- allowlist 只选择单个剧集库时，剧集榜只展示选中库内剧集，电影榜为空或仅包含选中库结果。
- 修改 allowlist 后执行预览生成，结果立即切换到新口径。
- 定时或手动生成正式榜单后，`GET /api/v1/rankings/latest` 与 Telegram 通知内容与预览口径一致。
- Emby 删除某个已配置媒体库后，管理员重新进入页面能看到失效提示，且排行榜生成不会整批报错。
- allowlist 非空时，不再出现“为构造过滤集合而先扫描整个媒体库”的同步慢调用。
- 剧集榜只对唯一 `seriesId` 做媒体库归属判断，不对每条 episode 重复做祖先解析。
- 候选扩窗、归属解析命中率和缓存命中率可以通过日志复现实证，不靠主观判断。

## 落地后文档处理

已同步处理：

- “排行榜支持媒体库 allowlist，空配置视为全库统计”的稳定结论已提炼到 `docs/system-architecture.md`
- 排行榜页的管理员配置区块职责已同步到 `docs/reference/web-information-architecture.md`
- 第一版当前保留“不新增本地持久化归库表、改用候选扩窗 + 运行期归属缓存”的现状结论；若后续性能不足，再单独起新方案
- 本稿已迁入 `docs/archive/plan/media-subscription/`
