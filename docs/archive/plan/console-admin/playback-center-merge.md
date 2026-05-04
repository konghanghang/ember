# 「播放分析」菜单合并方案

> 状态：已归档（已完成；归档于 2026-05-04）
> 负责人：Ember
> 更新时间：2026-05-04

## 背景

当前管理控制台在 admin 菜单组下并列存在两个独立条目：

- **用户画像**（`/console/user-profiles`）：按用户聚合的活跃度总览，列出累计时长、播放次数、活跃天数、最近播放、峰值时段、标签摘要。
- **播放历史**（`/console/playback-history`）：按播放记录展开的流水明细，列出时间、用户、片名、设备、客户端、时长。

这两块在产品语义上已经是同一域的"上下钻取"关系，而不是平行能力：

- 用户画像总览表头里就有"播放历史"按钮，会跳到播放历史页面并自动带上 `username` 和日期范围；
- 播放历史表头里也有"用户画像"按钮，跳到对应单用户画像详情页；
- 单用户画像详情页 `/console/user-profiles/:id` 也已经能跳回播放历史。

按现状，管理员要查"某用户最近一周看了什么"，需要在两个菜单之间反复切换；而 admin 菜单本身条目偏多（10 项），同域却平行展示带来认知负担。

如果不做这件事：菜单会持续臃肿，跨页跳转会持续靠 query 透传，新增能力（如"会话维度"分析）时无处归口。

## 目标

1. 在 admin 菜单组下用一个「播放分析」一级条目替代现有两个独立条目，子视图通过分段 Tab 切换。
2. 沿用项目已有的"兑换中心 / 支付中心"合并样板（容器壳 + `?tab=` + `EmberSegmentTabs`），不发明新模式。
3. 跨 Tab 跳转改为同页切换并透传上下文（`username` / 日期范围），保持现有"从用户视角下钻到流水"和"从流水回查画像"的双向链路。
4. 旧路径全部以 redirect 兼容，不破坏既有书签、Bot 链接、文档引用。
5. 单用户画像详情页继续保持独立路由，仅迁移到新的命名空间 `/console/playback/users/:id`。

## 非目标

- 本次不动后端 API、不动 GORM 模型、不动 SQL Schema。
- 本次不重新设计两个子视图各自的筛选项和列结构。
- 本次不合并"会话管理"`/console/sessions`（设备会话视角不同，独立保留）。
- 本次不引入二级折叠菜单或新的菜单容器机制。
- 本次不改普通用户侧的「我的画像」`/console/profile-analytics`（属于 user 角色）。

## 当前事实

以当前代码和现行文档为准：

- 相关文档：
  - `docs/system-architecture.md`（第 1325–1346 行描述了管理端播放历史与用户画像）
  - `docs/reference/web-design-guide.md`（前端设计基线）
  - `docs/reference/plan-directory-governance.md`（计划目录归类）
- 相关页面：
  - `services/web/src/views/admin/UserPlaybackProfilesView.vue`：用户画像总览（约 467 行）
  - `services/web/src/views/admin/UserPlaybackProfileView.vue`：单用户画像详情（约 260 行）
  - `services/web/src/views/admin/PlaybackHistoryView.vue`：播放历史（约 229 行）
- 相关路由（`services/web/src/router/index.ts`）：
  - `console-user-profiles` → `/console/user-profiles`
  - `console-user-profile` → `/console/user-profiles/:id`
  - `console-playback-history` → `/console/playback-history`
  - 兼容重定向：`/console/users/:id/profile` → `/console/user-profiles/:id`
- 相关菜单：`services/web/src/components/console/Sidebar.vue` admin 组里的"用户画像""播放历史"两条独立项。
- 相关面包屑：`services/web/src/components/console/TopBar.vue` 中 `console-user-profile` / `console-user-profiles` / `console-playback-history` 的标题映射。
- 已有样板：
  - `services/web/src/views/admin/RedemptionCenterView.vue`（约 66 行）：`?tab=codes|history` + `EmberSegmentTabs` + 子视图 `embedded` + `#tabs` 插槽。
  - `services/web/src/views/admin/PaymentCenterView.vue`（约 76 行）：同模式。
- 现有限制：跨菜单跳转依赖 `router.push({ name: ... })` + query 透传，状态在两个独立页面之间无法保留筛选上下文。

## 方案设计

### 1. 用户可见行为

新增能力：

- admin 菜单组新增一级条目「播放分析」，路径 `/console/playback`。
- 进入后默认展示「用户画像」Tab；通过分段控件可切到「播放历史」Tab，URL 同步为 `?tab=history`。
- 跨 Tab 操作（如"查看播放历史""查看用户画像"按钮）改为同页切 Tab，并自动透传 `username` 与日期范围；用户在另一 Tab 内不需要重新筛选。

修改的现有行为：

- 旧的"用户画像"和"播放历史"菜单条目从 Sidebar 移除。
- 旧路径 `/console/user-profiles`、`/console/playback-history`、`/console/user-profiles/:id` 通过 redirect 进入新路径，原书签可继续工作。
- 单用户画像详情页路径迁移至 `/console/playback/users/:id`，原 `/console/user-profiles/:id` redirect 至新路径，已存在的 `/console/users/:id/profile` 兼容路由保持不变（同样最终落到新路径）。

必须保持不变的行为：

- 两个子视图的筛选项、列、分页、排序、统计卡完全保留，不裁剪能力。
- 单用户画像详情页内容、字段、跳转保持不变。
- 后端 API 路由与字段不变。
- Bot / 邮件 / 通知中如有指向旧路径的链接，仍可通过 redirect 工作（无需 Bot 侧改动）。

前端设计约束：

- 前端实现必须遵守 Ember 风格。
- 设计与交互基线以 `docs/reference/web-design-guide.md` 为准。
- 容器壳不引入额外说明性文案；分段 Tab 标签即为视图标题，避免重复"指导性"描述。
- 不偏离规范的特例。

### 2. 数据与模型

> 本次不涉及数据模型变更。

### 3. 接口与边界

后端：

- 不新增、不修改任何 API。`/api/v1/admin/playback-history` 与 `/api/v1/admin/playback-profiles` 行为保持不变。

前端 query 契约（容器层与子视图共同遵守）：

| 参数 | 含义 | 作用域 | 默认值 |
|---|---|---|---|
| `tab` | `profiles` / `history` | 容器层路由层 | `profiles` |
| `username` | 跨 Tab 透传的用户筛选 | 两个子 Tab 都识别 | 空 |
| `userId` | 跨 Tab 透传的用户 ID（与 `username` 互斥） | 两个子 Tab 都识别 | 空 |
| `startDate` / `endDate` | 自定义日期范围 | 两个子 Tab 都识别 | 由各自子视图的预设决定 |
| `range` | `today` / `7d` / `30d` / `90d` / `all` / `custom` | 仅画像总览子视图识别 | `today` |
| `keyword` | 各 Tab 内含义不同（画像=用户名子串，历史=片名/设备/客户端） | 子视图各自维护，不跨 Tab 透传 | 空 |

容器层职责边界：

- 解析 `tab` 并选择子视图组件；非法值回退到 `profiles` 并 `router.replace`。
- 透传 `#tabs` 插槽给子视图（仿兑换中心模式），不接管子视图内部筛选。
- 不读 `keyword`、不读 `range` 等子视图内部字段，避免容器与子视图的字段耦合。

子视图职责边界：

- `embedded` prop 控制渲染时的 padding/边框，与兑换中心子视图同语义。
- `onMounted` 阶段从 `route.query` 同步 `username` / `userId` / 日期范围；`tab` 切换由容器维护。
- 跨 Tab 跳转方法改为 `router.replace({ query: { tab: 'xxx', username, startDate, endDate } })`，**不再使用 `router.push({ name: ... })`**。
- 子视图内部的"片名/设备搜索"`keyword` 不在切 Tab 时透传（语义不同）。

### 4. 关键流程

管理员从概览到流水的下钻：

1. 管理员进入 `/console/playback` → 容器读 `tab=profiles`（默认）→ 渲染用户画像总览。
2. 在画像总览中筛选日期、搜索某用户名 → 表行点击"播放历史" → 容器路由切到 `?tab=history&username=xxx&startDate=...&endDate=...`。
3. 容器渲染播放历史子视图 → 子视图 onMount 读 query 自动应用筛选 → 调用 `/api/v1/admin/playback-history`。

管理员从流水回查画像：

1. 管理员进入 `/console/playback?tab=history`（或从概览切过来）。
2. 表行点击"用户画像" → 跳到 `/console/playback/users/:id`（独立详情页，不在 Tab 内）。
3. 详情页内"返回"动作改为返回 `/console/playback?tab=profiles`。

旧路径访问：

1. 用户访问 `/console/user-profiles` → router 拦截 → redirect 到 `/console/playback?tab=profiles`。
2. 用户访问 `/console/playback-history?username=xxx` → redirect 到 `/console/playback?tab=history&username=xxx`，query 完整透传。
3. 用户访问 `/console/user-profiles/:id` → redirect 到 `/console/playback/users/:id`。
4. 用户访问 `/console/users/:id/profile`（既有兼容路径）→ redirect 到 `/console/playback/users/:id`。

### 5. 失败路径与边界条件

- 非法 `tab` 值（不属于 `profiles` / `history`）：容器层 `router.replace` 回退到 `tab=profiles`，与兑换中心样板一致。
- 旧 URL 带 query 的情况：redirect 必须保留原 query（参考 `router/index.ts` 现有 `redemption-codes` redirect 写法）。
- 详情页直接访问（无 referer，例如管理员粘贴 URL）：返回按钮改回到 `/console/playback?tab=profiles`，避免出现"返回画像列表"指向不存在的旧路由。
- 子视图首次挂载时 query 与 Tab 不一致（例如手动改 URL 同时改了 `tab` 和 `username`）：以 query 为准，容器先切 Tab，子视图 onMount 再读筛选参数；测试时需要覆盖"先选 history 再回 profiles"的来回切换。
- 兼容性约束：
  - 不破坏既有的 `/console/users/:id/profile` 兼容链路。
  - 不破坏 admin 角色守卫（`adminRouteMeta`），新路由必须继承同一守卫。
  - 不破坏面包屑显示（`TopBar.vue` 名称映射要新增 `console-playback`，旧的三条保留为兜底，等 redirect 链路完全无人访问时再清理）。

## 影响范围

- API：无变更。
- Web：
  - 新增 `services/web/src/views/admin/PlaybackCenterView.vue`（容器壳，约 60 行）。
  - 改造 `UserPlaybackProfilesView.vue` 与 `PlaybackHistoryView.vue` 接受 `embedded` prop 与 `#tabs` 插槽、跨 Tab 跳转改用 query。
  - 改造 `UserPlaybackProfileView.vue` 中的"返回"目标。
  - `services/web/src/router/index.ts` 增 `console-playback`、`console-playback-user-profile`，旧路由全部改 redirect。
  - `services/web/src/components/console/Sidebar.vue` admin 组合并菜单项为「播放分析」。
  - `services/web/src/components/console/TopBar.vue` 增加 `console-playback` 标题映射。
- Bot：无变更（Bot 不直接持有这些前端路径；如有静态文案稍后另行处理）。
- 配置/部署：无变更。
- 文档：
  - `docs/system-architecture.md`：更新第 1325–1346 行附近的"管理端播放历史 / 管理端用户画像"段落，统一收口到「播放分析」描述。
  - `docs/reference/web-design-guide.md`：本次未引入新控件或新基线，原则上无需更新；如发现 Tab 容器在 admin 域首次出现可考虑补充一条引用，但以"无新增基线"为前提，默认不动。
  - `docs/plan/README.md`：在"推进中"列表中加入本计划。
  - 落地后按"落地后文档处理"小节决定归档动作。

## 验证方式

### 编译/测试

- `cd services/web && npm run build`
- `cd services/web && npm run test:unit`（如有相关单测覆盖到 RedemptionCenter 模式可顺带跑一遍）

后端无变更，无需 `go test` / `go build`。

### 手工验证

- 进入 `/console/playback`，默认显示用户画像 Tab；切到「播放历史」Tab，URL 同步为 `?tab=history`，刷新后保持当前 Tab。
- 在用户画像 Tab 选择"近 7 天" + 搜索某用户名，点击表行"播放历史"，校验：URL 切到 `?tab=history` 并带 `username` + `startDate` + `endDate`；播放历史 Tab 自动应用筛选。
- 在播放历史 Tab 内点击表行"用户画像"，跳到 `/console/playback/users/:id`，详情页能展示数据；点"返回"回到 `/console/playback?tab=profiles`。
- 直接访问旧路径 `/console/user-profiles`、`/console/playback-history?username=foo`、`/console/user-profiles/:id`、`/console/users/:id/profile`，确认全部 redirect 正确（带 query 不丢失）。
- admin 菜单组里只剩「播放分析」一条，旧两条不再展示。
- 顶栏面包屑：在「播放分析」三个状态（profiles / history / 详情页）下标题与描述均能正确显示。
- 普通用户登录后无法访问 `/console/playback`（admin 守卫继续生效），跳到 dashboard 并提示无权限。

## 落地记录

> 实现 commit：`85cc73d feat(playback): 合并用户画像与播放历史为「播放分析」`
> 归档时间：2026-05-04

| 计划目标 | 落地证据 |
|---|---|
| 1. 「播放分析」一级菜单替代两条独立条目 | `services/web/src/components/console/Sidebar.vue`、`services/web/src/views/admin/PlaybackCenterView.vue` |
| 2. 沿用兑换中心 / 支付中心样板 | 容器壳（`?tab=` + `EmberSegmentTabs`）+ 子视图 `embedded` + `#tabs` 插槽 |
| 3. 跨 Tab 透传 `username` / 日期范围 | `router.replace` 带 query；子视图 `onMounted` 同步 |
| 4. 旧路径 redirect 兼容 | `services/web/src/router/index.ts` 中 4 条 redirect：`/console/user-profiles`、`/console/user-profiles/:id`、`/console/playback-history`、`/console/users/:id/profile` |
| 5. 单用户画像迁移到 `/console/playback/users/:id` | `console-user-profile` 路由（`router/index.ts:104`） |

稳定结论已收口到：

- `docs/system-architecture.md` 第 1327 行起「管理端播放分析」段落（含兼容路径清单）
- `services/web/src/router/index.ts` 中 4 条 redirect 保留为长期兼容入口

原计划要求"稳定 30 天后归档"，因 85cc73d 之后无相关 fix 提交、关键路径已落入架构文档，提前归档；如后续出现需要回滚的回归，从归档目录恢复即可。
