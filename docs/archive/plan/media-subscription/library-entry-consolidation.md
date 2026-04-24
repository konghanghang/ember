# 用户侧媒体库入口收口方案

> 状态：已归档
> 负责人：Ember
> 更新时间：2026-04-24
> 归档说明：稳定结论已提炼到 `docs/system-architecture.md`，本方案仅保留实现收口过程的追溯价值。

## 背景

这个问题为什么现在要解决：

- 当前用户侧 `媒体库` 页面只有“最近入库 + 电影/剧集切换”，功能过薄，不足以支撑一级导航入口。
- Ember 的主职责是账号、续费、订阅、状态流转和外围桥接，不是替代 Emby 承担内容搜索与消费前台。
- 如果继续围绕 `媒体库` 页面追加“探索”或“搜索”能力，实际是在重复 Emby 已经具备的能力，并持续拉歪系统边界。

## 目标

本方案要实现：

1. 收口用户侧独立 `媒体库` 导航与页面职责，避免保留低价值入口。
2. 将现有“最近入库”能力并入 `概览` 页，保留必要的内容感知能力。
3. 明确把内容搜索与消费入口交还给 Emby，在控制台内提供清晰的跳转桥接。

## 非目标

本次明确不做：

- 不新增独立 `媒体探索` 页面。
- 不新增站内媒体搜索、媒体详情页或推荐系统。
- 不替代 Emby 做完整片库浏览、筛选和搜索体验。
- 不调整订阅、续费、账号中心等现有页面职责。

## 当前事实

以当前代码和现行文档为准，先写清现状：

- 相关文档：
  - `docs/system-architecture.md`
  - `docs/reference/web-design-guide.md`
- 相关服务/页面/模型：
  - `services/web/src/router/index.ts`
  - `services/web/src/components/console/Sidebar.vue`
  - `services/web/src/components/console/TopBar.vue`
  - `services/web/src/views/console/DashboardView.vue`
  - `services/web/src/views/console/LibraryView.vue`
  - `services/web/src/api/console.ts`
  - `services/api/internal/handlers/media.go`
  - `services/api/internal/services/media/service.go`
- 当前行为：
  - `/console/library` 已在前端路由层兼容重定向到 `/console/dashboard`，不再作为用户侧独立一级入口。
  - `DashboardView` 继续承担会员状态、服务器入口、媒体统计和快捷操作等控制台首页职责，并已接入最近入库摘要与 Emby 桥接入口。
  - `LibraryView` 已退化为兼容壳页面，不再承载独立业务内容。
  - 当前用户侧媒体相关接口主要只有 `GET /api/v1/emby/config`、`GET /api/v1/media/stats`、`GET /api/v1/media/latest`。
- 已完成项：
  - 已完成产品方向确认：不继续推进“媒体探索”，收口弱 `媒体库` 页面。
  - 已完成现状评估：原独立 `媒体库` 页面不具备独立存在的功能价值。
  - 已完成导航与路由收口：用户侧不再展示独立 `媒体库` 一级入口，旧路径走兼容重定向。
  - 已完成首页摘要并入：最近入库能力已通过 `RecentLibrarySection` 收口到 `DashboardView`。
  - 已完成 Emby 桥接入口补齐：用户在概览页即可直接打开 Emby。
  - 已完成前端状态源收口：最近入库摘要与首页桥接统一复用用户态 `embyUrl`。
- 归档原因：
  - 方案目标已达成，相关现行事实已同步到 `docs/system-architecture.md`。
  - 当前正文只剩“为什么这样收口”和“当时的边界约束”这类追溯价值，不再指导现行实现。
- 现有限制：
  - 现有后端接口足够支持“概览 + 最近入库摘要”，但不足以支撑一个完整的探索页。
  - 当前 `LibraryView` 仅提供内容展示，不解决“找片”和“开始播放”的主任务。

## 方案设计

### 1. 用户可见行为

- 新增能力：
  - `概览` 页新增轻量 `最近入库` 模块，展示少量最新电影/剧集。
  - `概览` 页增加清晰的 `打开 Emby` 入口，引导用户进入原生内容消费场景。
- 修改现有行为：
  - 用户侧不再展示独立 `媒体库` 一级导航。
  - 旧 `/console/library` 链接不再进入独立页面，而是兼容重定向回 `/console/dashboard`。
  - 最近入库内容从独立页面收口为首页摘要区块，不再承担独立浏览页职责。
- 哪些现有行为必须保持不变：
  - `概览` 页原有会员状态、到期提示、服务器连接和快捷操作行为保持不变。
  - 现有 `GET /api/v1/media/latest`、`GET /api/v1/media/stats`、`GET /api/v1/emby/config` 接口契约保持不变。
  - 用户仍可通过 Emby 原生入口完成搜索、浏览和播放，不在 Ember 内引入新的内容消费分支。
- 前端约束：
  - 前端实现必须遵守 Ember 风格。
  - 设计与交互基线以 `docs/reference/web-design-guide.md` 为准。
  - `最近入库` 模块应作为 `概览` 页的摘要区块存在，不得扩张为新的整页海报墙。
  - `最近入库` 首版保留最近 `20` 条内容，但通过横向滑动查看，避免重新回到整页探索布局。
  - 如果补充按钮与卡片动作，主次操作语义仍需保持清晰，避免首页堆砌次级入口。

### 2. 数据与模型

> 本次不涉及数据模型变更。

### 3. 接口与边界

- 新增或修改哪些 API / Internal API / webhook / 命令：
  - 首版不新增 API，不修改现有接口结构。
  - 前端继续复用：
    - `GET /api/v1/emby/config`
    - `GET /api/v1/media/stats`
    - `GET /api/v1/media/latest`
- 请求参数与响应字段怎么变：
  - 不变。
- 哪些调用方会受影响：
  - `DashboardView`
  - 新增控制台业务组件（建议命名为 `RecentLibrarySection`）
  - 控制台侧边栏和顶栏路由元信息
- 边界约束：
  - Ember 控制台只保留内容摘要与桥接入口，不再承担通用媒体探索职责。
  - Emby 继续作为内容搜索、条目浏览和播放的主入口。
  - 如后续确需补“库内已存在”提示，应放在订阅或缺集等桥接链路，而不是重启独立探索页。
  - `EMBY_URL` 在控制台内必须收口为单一状态源，首页与最近入库摘要都复用同一份用户态配置，不再各自维护本地副本。

### 3.1 前端落地拆分

首版按下面的切片实施，避免把整页逻辑直接堆进 `DashboardView`：

1. 新增控制台业务组件 `RecentLibrarySection`
   - 负责 `GET /api/v1/media/latest` 的加载、电影/剧集切换、刷新、横向滑动卡片展示。
   - 组件内部复用当前海报占位逻辑，但失败时只做区块内空态降级，不弹全局错误消息。
2. `DashboardView` 只负责页面编排
   - 接入 `RecentLibrarySection` 摘要区块。
   - 在 `服务器连接` 卡片内补 `打开 Emby` 主入口。
   - 复用用户态已有的 Emby 地址状态，不再新增第二套本地 `embyUrl` 状态。
3. 路由与导航兼容收口
   - `/console/library` 直接路由级重定向到 `/console/dashboard`。
   - 删除侧边栏和顶栏中的 `媒体库` 独立入口与文案。
   - 不保留带实际业务内容的兼容页壳。

### 4. 关键流程

按顺序写清主链路，不需要贴代码：

1. 用户登录进入控制台，默认仍落在 `概览` 页。
2. `概览` 页在现有会员状态、服务器入口和快捷操作之外，额外加载少量最近入库摘要。
3. 用户若只需感知站内更新，可直接在首页完成浏览。
4. 用户若需要进一步搜索或消费内容，点击 `打开 Emby`，进入 Emby 原生界面继续操作。
5. 旧 `/console/library` 访问请求被兼容重定向到 `/console/dashboard`，不产生死链。

### 5. 失败路径与边界条件

- `GET /api/v1/media/latest` 请求失败：`概览` 页只降级 `最近入库` 区块，不影响会员状态与服务器入口主信息。
- 用户账号过期：继续遵守现有 `概览` 页过期态与服务器锁定逻辑，不能因增加最近入库摘要而破坏既有约束。
- Emby 地址未配置：`打开 Emby` 入口应明确禁用或提供清晰提示，不能出现空跳转。
- 旧书签命中 `/console/library`：必须通过前端路由兼容回首页，避免 404 或空白页。
- 最近入库摘要请求失败：不允许在首页首次加载时弹全局错误 toast，避免把摘要失败放大成整页错误体验。
- 兼容性约束：
  - 不得改变现有 `概览` 页路由归属与默认落点。
  - 不得把 `最近入库` 模块重新膨胀为新的伪探索页。
  - 不得引入新的用户侧媒体接口依赖作为本次收口的阻塞项。

## 影响范围

涉及的子系统：

- API：无
  - 继续复用现有媒体接口
- Web：有
  - `services/web/src/router/index.ts`
  - `services/web/src/components/console/Sidebar.vue`
  - `services/web/src/components/console/TopBar.vue`
  - `services/web/src/components/console/RecentLibrarySection.vue`
  - `services/web/src/views/console/DashboardView.vue`
  - `services/web/src/views/console/LibraryView.vue`（仅兼容退场，如仍保留）
- Bot：无
- 配置/部署：无
- 文档：需要更新
  - `docs/system-architecture.md`

## 验证方式

### 编译/测试

- `cd services/web && npm run build`

如实现过程中触达后端或类型定义，再补充：

- `cd services/api && go build ./...`

### 手工验证

- 登录后侧边栏不再出现独立 `媒体库` 入口。
- 访问 `/console/library` 会被正确重定向到 `/console/dashboard`。
- `概览` 页仍正常展示会员状态、服务器连接和快捷操作。
- `概览` 页能正常加载最近入库摘要，并支持电影/剧集切换与刷新。
- `打开 Emby` 入口在已配置 Emby 地址时可用，在未配置时展示清晰提示。
- 用户过期场景下，原有锁定逻辑继续成立，未被最近入库模块破坏。

## 落地后文档处理

本次归档前已完成同步：

- 稳定结论已同步到 `docs/system-architecture.md`
  - `LibraryView` 已改写为兼容壳说明
  - `DashboardView` 已明确包含最近入库摘要与 Emby 桥接入口
- 归档条件已满足：
  - 用户侧独立 `媒体库` 导航与路由收口完成
  - `最近入库` 模块已稳定并入 `概览` 页
  - 相关兼容路径、构建验证和手工验收已完成
- 正文已移入 `docs/archive/plan/media-subscription/`
