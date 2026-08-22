# Ember Web 信息架构参考

> 本文档承接 Ember Web 端当前的信息架构、共享组件层边界、路由归属与关键页面职责。
> 视觉与交互规范以 [Web 设计规范](./web-design-guide.md) 为准；本文件只记录当前实现边界与页面职责。

## 1. Web 共享组件层

`services/web/src/components/ember/` 是当前 Web 端的 Ember 基础组件层，职责是把后台与控制台高频重复的 UI 骨架收口为稳定契约，而不是把业务逻辑搬进组件。

- `layout/`
  - `EmberPageHeaderCard`：统一页面标题、说明、统计 badge、右侧 actions/tabs slot
  - `EmberFilterPanel`：统一筛选区容器、字段区布局、按钮区对齐
  - `EmberSegmentTabs`：统一页内单选分段切换，当前以 `radiogroup / radio` 语义提供共享键盘交互约定，但不默认承诺 `tabpanel` 语义
- `filters/`
  - `EmberSearchInput`、`EmberSelectField`、`EmberDateField`、`EmberDateRangeField`
  - 负责筛选字段的 Ember 风格外观与交互基线，消费共享 field token，不承载页面查询逻辑
- `data-display/`
  - `EmberTableCard`：统一表格容器、可选标题头、表头样式和分页区
  - `EmberMetricCard`：统一简单统计卡基线
- `forms/`
  - `EmberFormDialog`：统一弹窗表单容器和 footer 区域
- `feedback/`
  - `EmberEmptyStateCard`：统一中性 / 风险态空状态容器和可选动作区
- `components/common/`
  - `ProjectSourceLink`：展示 GitHub 仓库入口与可选当前前端构建短 commit hash；首页 Navbar 只展示源码入口，控制台 Sidebar 展示低干扰 build 信息，不承载业务状态

当前已接入这套基础组件的后台页面包括：

- 列表/表单类页面：
  - `services/web/src/views/admin/UsersView.vue`
  - `services/web/src/views/admin/PaymentsView.vue`
  - `services/web/src/views/admin/PlaybackHistoryView.vue`
  - `services/web/src/views/admin/RedemptionCodesView.vue`
  - `services/web/src/views/admin/RedemptionHistoryView.vue`
  - `services/web/src/views/admin/UserPlaybackProfilesView.vue`
  - `services/web/src/views/admin/PlansView.vue`
  - `services/web/src/views/admin/PlanGroupsView.vue`（用户分组 / 权益模板主入口）
- 容器型中心页：
  - `services/web/src/views/admin/PaymentCenterView.vue`
  - `services/web/src/views/admin/PlaybackCenterView.vue`
  - `services/web/src/views/admin/RedemptionCenterView.vue`
  - `services/web/src/views/admin/SettingsView.vue`
  - `services/web/src/views/admin/SessionsView.vue`
  - `services/web/src/views/admin/MediaQualityView.vue`
  - `services/web/src/views/admin/P115AccountsView.vue`

当前已接入这套基础组件的控制台页面包括：

- `services/web/src/views/console/SubscriptionsView.vue`
- `services/web/src/views/console/NewSubscriptionView.vue`
- `services/web/src/views/console/RenewalCenterView.vue`
- `services/web/src/views/console/DashboardView.vue`
- `services/web/src/views/console/RankingsView.vue`

边界约束：

- 页面 view 继续保留接口调用、路由状态、筛选参数、弹窗状态和数据编排
- Ember 基础组件只承载稳定 UI 契约，不侵入 store 和 API 请求
- 强业务、强视觉特例页面仍允许保留页面内实现，例如 `services/web/src/views/console/TVCalendarView.vue`

## 2. 前端架构

### 2.1 状态管理（Pinia）

- `store/auth.ts`：Token 持久化；`role / passwordResetRequired` 只接受登录响应或 `/profile` 返回的服务端事实，不再从 localStorage 恢复
  - State: `token`, `role`, `passwordResetRequired`, `protectionConfig`, `crossTabSyncEnabled`
  - Computed: `isAuthenticated`, `isAdmin`, `isUser`
  - Actions: `login`, `register`, `logout`, `setAuth`, `setSessionFromProfile`, `clearAuth`, `restoreAuth`, `initCrossTabSync`, `loadProtectionConfig`
- `store/user.ts`：用户状态管理
- `store/admin.ts`：管理员状态管理

### 2.2 API 层

- `api/request.ts` — 基础配置：baseURL=/api/v1, Bearer token 自动注入；普通接口 401 单例化收口为“清本地登录态 + 跳 `/login?redirect=`”；`/login` 和 `/logout` 走专门分支，不混入“登录过期”逻辑
- `api/auth.ts` — login, getLoginProtectionConfig, register, getRegistrationMode, sendEmailCode, sendResetCode, resetPasswordByCode
- `api/user.ts` — redeem, redemptions, tmdb
- `api/admin.ts` — 管理后台全部接口（users, codes, settings, subscriptions, plans, payments, sessions, devices, rankings, p115-accounts）
- `api/console.ts` — 统一认证路由（profile, subscriptions, payments, rankings, media, emby, telegram, media-libraries；账号中心邮箱变更走 `sendEmailChangeCode(newEmail)` + `updateEmail(newEmail, code)` 两步流，旧 `updateProfile` 已下线）

### 2.3 路由守卫

- 未认证 → 重定向 `/login`（带 redirect 参数）
- `redirect` 仅接受站内已解析路由：必须以 `/` 开头、不能以 `//` 开头、不能落到 `not-found`
- 已认证但本地尚无 profile → 先调用 `/profile` 同步服务端 `role / passwordResetRequired`，再做角色守卫与强制改密跳转
- 角色不匹配 → 重定向 `/console/dashboard` 并提示“当前账号无权访问该页面”
- 守卫遍历 `to.matched` 收集 `requiresAuth / role`，不再只看最后一层 `meta`
- 多标签页登录态通过 `storage` 事件同步：其他窗口登出后，当前窗口会清空本地状态并跳回登录页
- meta: `{requiresAuth: boolean, role: 'admin' | 'user'}`

### 2.4 设计系统

- **CSS 类**：`panel-clean`（卡片）, `input-ember`（输入框）, `btn-ember`（按钮）
- **颜色**：ember 色系（橙红 `#ea580c`）
- **布局**：Tailwind 响应式 grid + Element Plus 组件
- **图标**：`@element-plus/icons-vue`
- **构建元信息**：`utils/buildInfo.ts` 统一消费 `VITE_GIT_COMMIT_SHA`、`VITE_GITHUB_REPOSITORY`、`VITE_GITHUB_REPOSITORY_URL`，缺失 hash 时降级展示 `dev`

## 3. 页面与路由职责

### 3.1 Dashboard 双态设计

用户面板根据 `isExpired` computed 做渐进式降级：
- **活跃态**：绿色 banner + 媒体统计 + 续费中心入口
- **过期态**：橙色警告 banner + 明显的“立即续费”入口 + 媒体统计灰化
- **定位**：Dashboard 只负责账户概览，不再承载兑换码输入和续费历史

### 3.2 续费中心

- 路由：`/console/renewal`（user）
- 兼容路由：`/console/pricing` → 重定向到 `/console/renewal`
- 视图：`views/console/RenewalCenterView.vue`
- 页面结构：
  - 当前会员状态
  - 续费方式 Tab
    - 在线购买（Stripe Checkout）
    - 兑换码续期
  - 支付记录 + 兑换记录
- 目标：把“在线支付”和“兑换码续期”统一到同一续费心智下，而不是分散在 Dashboard 和独立价格页中

### 3.3 管理端兑换中心

- 路由：`/console/redemptions`（admin）
- 兼容路由：
  - `/console/redemption-codes` → `?tab=codes`
  - `/console/redemption-history` → `?tab=history`
- 视图：`views/admin/RedemptionCenterView.vue`
- Tab 结构：
  - `codes`：`views/admin/RedemptionCodesView.vue`
  - `history`：`views/admin/RedemptionHistoryView.vue`
- 数据源：
  - `GET /api/v1/admin/redemption-codes`（支持兑换码、状态、注册套餐分组筛选；返回 `notes`、`registrationPlanGroup`、`registrationPlanGroupName`）
  - `POST /api/v1/admin/redemption-codes`（必须提交 `registrationPlanGroup`，支持可选备注 `notes`）
  - `POST /api/v1/admin/redemption-codes/batch`（必须提交 `registrationPlanGroup`，支持可选备注 `notes`）
  - `PUT /api/v1/admin/redemption-codes/:id`（必须提交 `registrationPlanGroup`，支持更新备注 `notes`）
  - `DELETE /api/v1/admin/redemption-codes/:id`
  - `GET /api/v1/admin/redemptions`（支持按用户名、用户 ID、兑换码筛选）

### 3.4 管理端支付中心

- 路由：`/console/billing`（admin）
- 兼容路由：
  - `/console/billing?tab=groups` → `/console/plan-groups`
  - `/console/plans` → `?tab=plans`
  - `/console/payments` → `?tab=payments`
- 视图：`views/admin/PaymentCenterView.vue`
- Tab 结构：
  - `plans`：`views/admin/PlansView.vue`
  - `payments`：`views/admin/PaymentsView.vue`
- 分组维护职责已迁移到“用户分组 / 权益模板”，支付中心不再嵌入第二套分组编辑 UI。
- 数据源：
  - `GET /api/v1/admin/plans`
  - `POST /api/v1/admin/plans`
  - `PUT /api/v1/admin/plans/:id`
  - `DELETE /api/v1/admin/plans/:id`
  - `GET /api/v1/admin/payments`

### 3.4.1 管理端用户分组 / 权益模板

- 路由：`/console/plan-groups`（admin）
- 兼容路由：
  - `/admin/plan-groups` → `/console/plan-groups`
- 视图：`views/admin/PlanGroupsView.vue`
- 页面职责：
  - 用户业务分组 CRUD
  - 分组媒体库模板配置
  - 分组 Emby 权益模板配置
  - 展示模板保存后创建的 Emby Policy 同步批次入口结果
- 数据源：
  - `GET /api/v1/admin/plan-groups`
  - `POST /api/v1/admin/plan-groups`
  - `PUT /api/v1/admin/plan-groups/:key`
  - `DELETE /api/v1/admin/plan-groups/:key`
  - `GET /api/v1/admin/media-libraries`
  - `GET /api/v1/admin/plan-groups/:key/media-libraries`
  - `PUT /api/v1/admin/plan-groups/:key/media-libraries`
  - `GET /api/v1/admin/plan-groups/:key/emby-policy-template`
  - `PUT /api/v1/admin/plan-groups/:key/emby-policy-template`

### 3.5 管理端设备管理

- 新增路由：`/console/devices`（admin）
- 新增视图：`views/admin/DevicesView.vue`
- 数据源：
  - `GET /api/v1/admin/devices`
  - `GET /api/v1/admin/devices/stats`
  - `GET /api/v1/admin/devices/blacklist`
  - `POST /api/v1/admin/devices/logout/:deviceId`

### 3.5.1 管理端 115 账号

- 路由：`/console/p115-accounts`（admin）
- 兼容路由：`/admin/p115-accounts` → `/console/p115-accounts`
- 视图：`views/admin/P115AccountsView.vue`
- 页面职责：
  - 展示管理员维护的源账号和播放账号安全摘要、验证状态、启用状态及脱敏错误
  - 创建账号、配置 source 的 Emby 挂载目录/115 源目录 ID、替换 Cookie、显式验证和启停
  - `pending / expired / error / cooling_down` 账号不提供启用操作；停用不受验证状态限制
  - Cookie 只存在于创建或替换表单，提交成功或关闭弹窗后立即清空，任何查询结果都不回填
- 数据源：
  - `GET /api/v1/admin/p115-accounts`
  - `POST /api/v1/admin/p115-accounts`
  - `PUT /api/v1/admin/p115-accounts/:id/cookie`
  - `PUT /api/v1/admin/p115-accounts/:id/source-location`
  - `POST /api/v1/admin/p115-accounts/:id/validate`
  - `PUT /api/v1/admin/p115-accounts/:id/enabled`
- 权限边界：路由只允许管理员角色进入，后端账号接口只接受管理员 JWT，Admin API Key 返回 `403`

### 3.6 管理端播放分析

- 主路由：`/console/playback`（admin），分段 Tab 切换
  - `?tab=profiles`（默认）：用户画像总览
  - `?tab=history`：播放历史
- 容器视图：`views/admin/PlaybackCenterView.vue`
- 子视图：
  - `views/admin/UserPlaybackProfilesView.vue`（用户画像总览）
  - `views/admin/PlaybackHistoryView.vue`（播放历史）
- 数据源：
  - `GET /api/v1/admin/playback-profiles`（用户画像总览，支持 `range / startDate / endDate / keyword / sortBy / sortOrder / page / pageSize`）
  - `GET /api/v1/admin/playback-history`（播放历史，支持 username / keyword / 日期范围 / 分页筛选，兼容旧 `userId`）
- 跨 Tab 上下文：切 Tab 时透传 `username / userId / startDate / endDate`；`keyword` 与 `range` 各 Tab 内部自治，不跨 Tab 透传
- 联动：
  - 用户画像总览支持跳转到「播放历史」Tab 或单用户画像详情
  - 播放历史支持跳转到单用户画像详情
- 兼容路由（全部以 redirect 形式保留）：
  - `/console/user-profiles` → `/console/playback?tab=profiles`
  - `/console/playback-history` → `/console/playback?tab=history`
  - `/admin/users` 等历史 admin 路径不涉及

### 3.7 管理端单用户画像

- 主路由：`/console/playback/users/:id`（admin）
- 兼容路由（redirect）：
  - `/console/user-profiles/:id` → `/console/playback/users/:id`
  - `/console/users/:id/profile` → `/console/playback/users/:id`
- 视图：`views/admin/UserPlaybackProfileView.vue`
- 主入口：`views/admin/UserPlaybackProfilesView.vue`（嵌入「播放分析」容器中）
- 辅助入口：`views/admin/PlaybackHistoryView.vue`（嵌入「播放分析」容器中）
- 兼容入口：`views/admin/UsersView.vue`
- 页面主体：复用 `components/profile/PlaybackProfileContent.vue`，仅在外层补管理员操作
- 数据源：`GET /api/v1/admin/users/:id/profile?range=today|7d|30d|90d|all` 或 `startDate/endDate`
- 页面模块：
  - 摘要卡：累计播放时长 / 播放次数 / 活跃天数 / 最近播放
  - 时间范围：推荐快捷范围 + 自定义日期时间范围（最大 92 天）
  - 分布：24 小时活跃时段、设备分布、客户端分布
  - 勋章：基于固定阈值的解释型画像标签
  - 最近记录：最近 10 条播放记录预览，并支持跳回播放历史

### 3.8 用户端我的画像

- 新增路由：`/console/profile-analytics`（user）
- 新增视图：`views/console/ProfileAnalyticsView.vue`
- 页面主体：复用 `components/profile/PlaybackProfileContent.vue`
- 数据源：`GET /api/v1/profile/analytics?range=today|7d|30d|90d|all` 或 `startDate/endDate`
- 页面模块：
  - 摘要卡：累计播放时长 / 播放次数 / 活跃天数 / 最近播放
  - 时间范围：推荐快捷范围 + 自定义日期时间范围（最大 92 天）
  - 活跃时段：24 小时分布 + 峰值时段摘要
  - 画像标签：展示当前时间窗口内最有代表性的少量标签
  - 偏好分布：设备偏好 / 客户端偏好
  - 最近播放记录：最近 10 条个人播放记录预览

### 3.9 管理端媒体质量盘点

- 新增路由：`/console/media-quality`（admin）
- 新增视图：`views/admin/MediaQualityView.vue`
- 数据源：
  - `GET /api/v1/admin/media-quality/libraries`
  - `GET /api/v1/admin/media-quality/libraries/:libraryId?force=true|false&page=1&pageSize=20`
  - `POST /api/v1/admin/media-quality/libraries/:libraryId/scan`
  - `GET /api/v1/admin/media-quality/posters/:itemId`
- 支持 `libraryId=all` 进行全媒体库汇总分析
- 低画质结果按“影片/剧集”汇总后分页展示

### 3.10 播放排行榜

- 主入口：`/console/rankings`
- 视图：`views/console/RankingsView.vue`
- 数据源：
  - `GET /api/v1/rankings/latest?period=daily|weekly`
  - `GET /api/v1/rankings/history?period=daily|weekly&date=YYYY-MM-DD`
  - 管理员额外使用：
    - `POST /api/v1/admin/rankings/preview?type=daily|weekly`
    - `GET /api/v1/admin/rankings/library-allowlist`
    - `PUT /api/v1/admin/rankings/library-allowlist`
- 页面行为：
  - 普通用户只查看日榜 / 周榜与历史榜
  - 管理员可在同页预览即时排行，并配置“参与统计的媒体库” allowlist
  - allowlist 为全站统一配置，同时影响日榜、周榜、预览生成、正式入库结果与 Telegram 推送
  - allowlist 为空时默认统计全部媒体库

### 3.11 最近入库

- 主入口：`/console/dashboard`（user）
- 展示位置：`views/console/DashboardView.vue` + `components/console/RecentLibrarySection.vue`
- 兼容路径：`/console/library` 路由级重定向到 `/console/dashboard`
- 数据源：`GET /api/v1/media/latest?type=Movie|Series&limit=20`
- 封面：前端通过 `GET /api/v1/media/posters/:itemId?type=Movie|Series` 拉取 blob，再转 object URL；不再直接拼 Emby 公网图床
- 权限边界：封面代理只允许访问“当前用户最近入库列表里已经出现的条目”，避免把管理员 API key 图床直接暴露给浏览器
- 行为：在概览页展示当前用户视角的最近入库摘要，支持电影/剧集切换、横向滑动与手动刷新，不做搜索和分页

### 3.12 Dashboard Emby 入口

- 主入口：`/console/dashboard`
- 数据源：`GET /api/v1/emby/config`
- 行为：概览页只展示后端返回的单条 Emby 地址，不再由前端伪造“备用线路 A/B”；用户侧操作仅保留复制地址与新窗口打开
