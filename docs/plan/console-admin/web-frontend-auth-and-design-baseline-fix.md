# 前端鉴权与设计基线收口方案

> 状态：可进入归档准备（P0 + P1 已落地）
> 负责人：Ember
> 更新时间：2026-04-29

## 落地进度

批次 4 已按 4 个 PR 落地完成：

- ✅ `5fd236a` `fix(web): 收口前端鉴权红线`
  - `request.ts` 401 单例化
  - `/login` 与 `/logout` 的 401 行为分流
  - `useAuthStore` 跨标签登录态同步
  - 路由守卫改为遍历 `to.matched`
  - `LoginView` `redirect` 白名单
- ✅ `75735d1` `fix(console): 收口概览页入口与海报代理`
  - Dashboard 删除伪造“备用线路 A/B”
  - 用户侧 `GET /api/v1/media/posters/:itemId`
  - 最近入库改为 blob/object URL 海报代理
- ✅ `fc10a61` `fix(console): 收口续费页竞态与订阅确认渲染`
  - 续费页支付 / 兑换记录分页请求 token 收口
  - `redirectToCheckout` 失败提示 + loading 修正
  - `SubscriptionsView` / `NewSubscriptionView` 去掉 `dangerouslyUseHTMLString`
- ✅ `5c7b970` `refactor(web): 统一时间格式与 tone token`
  - 新增 `components/ember/tokens.ts`
  - `utils/date.ts` 统一时间格式
  - `EmberEmptyStateCard` / `EmberMetricCard` / `TVCalendarView` 接入统一 tone 语义
  - `docs/reference/web-design-guide.md` / `docs/system-architecture.md` 同步当前事实

当前剩余项：

- 更广范围的请求竞态 / 交互一致性 sweep 仍可继续做，但当前高价值链路已基本收口
- 构建产物 chunk 体积告警、icon/样式统一化仍属于下一轮“前端一致性治理”，不阻塞本批功能闭环
- `vite proxy / prod baseURL` 若后续需要更细的部署步骤，应补到 runbook，不再继续堆在本计划实施稿

## 归档判断

- 当前可以进入归档准备，但暂不直接归档。
- 原因：主链路事实已经稳定，剩余项主要是全站 sweep 与提炼工作；下一步更适合补稳定结论与交叉引用，而不是继续把它当核心实施稿使用。

## 稳定结论

以下结论已经稳定，可视为当前事实，而不是临时实施策略：

- 前端统一认证入口固定为 `/api/v1`，401 收口为“清本地登录态 + 跳 `/login?redirect=`”，且 `/login`、`/logout` 不混入“登录过期”逻辑。
- 路由守卫必须遍历 `to.matched`，`redirect` 仅允许站内已解析路径，普通用户命中 admin 路由必须给出提示而不是静默吞掉。
- 最近入库封面必须走 `GET /api/v1/media/posters/:itemId` 代理，不再直拼 Emby 图床。
- tone token 与时间格式已经收口为基础组件契约，不再允许页面各自造词或直接在 view 中拼 `toLocaleString()`。

## 交叉引用

- 当前系统事实：
  - [docs/system-architecture.md](</Users/konghang/data/github/ember/docs/system-architecture.md>) §8 已收录前端鉴权链路、401 单例化、跨标签同步、redirect 白名单
  - [docs/system-architecture.md](</Users/konghang/data/github/ember/docs/system-architecture.md>) “最近入库”段已收录用户侧海报代理边界
- 当前设计规范：
  - [docs/reference/web-design-guide.md](</Users/konghang/data/github/ember/docs/reference/web-design-guide.md>) §3.5 已收录 tone token 与时间格式基线
- 当前盘点入口：
  - [docs/proposals/plan-inventory.md](</Users/konghang/data/github/ember/docs/proposals/plan-inventory.md>) 已把本方案标为“可进入归档准备”

## 退场说明

- 本文档后续不再承担“当前事实说明”的职责；现行事实应以 `docs/system-architecture.md` 和 `docs/reference/web-design-guide.md` 为准。
- 在以下条件同时满足后，可移入 `docs/archive/plan/console-admin/`：
  - 本文顶部“当前剩余项”只剩历史追溯价值，不再指导新的实现决策
  - `docs/plan/README`、`docs/proposals/README`、`docs/proposals/plan-inventory.md` 已同步把本方案从现行实施稿入口移除
  - 文中仍指向旧实施过程的表述已清理，不再与稳定文档重复承担规则说明

## 批次定位

本方案是这轮 8 份主计划里的 **计划 6**，对应总路线图中的 **批次 4｜前端鉴权 + 设计基线**。它不是一份孤立的前端美化文档，而是承接批次 1-3 后端契约后的用户侧收口。

本轮主干 8 份计划如下：

| 编号 | 计划 | 现状 | 与批次 4 的关系 |
|---|---|---|---|
| 1 | `docs/plan/access-auth/auth-and-account-integrity-hardening.md` | 批次 1/2 关键项已落地 | 提供登录 / 注册 / 忘记密码的安全边界，前端消费登录态与文案约束 |
| 2 | `docs/plan/billing-redemption/payment-redemption-integrity-hardening.md` | 批次 2 已落地 | 前端需要消费 `payments.status='expired'`、结账幂等后的交互行为 |
| 3 | `docs/plan/media-subscription/subscription-state-machine-hardening.md` | 批次 2 已落地 | 前端需要消费 `ingestProgress`、`redispatch`、状态机收口后的展示 |
| 4 | `docs/plan/media-subscription/tv-calendar-and-tmdb-key-protection.md` | 批次 1 / 3A 关键项已落地 | 前端需要消费追剧日历纠偏后的稳定返回与统一错误语义 |
| 5 | `docs/plan/console-admin/playback-and-device-observation-hardening.md` | 批次 3-A 已落地大部分 | 前端已可消费 `scan_in_flight`、`LATEST_CACHE_PER_USER` 等契约；但用户侧海报代理仍需本批补齐 |
| 6 | `docs/plan/console-admin/web-frontend-auth-and-design-baseline-fix.md` | 本计划 | 批次 4 主体 |
| 7 | `docs/plan/bot-telegram/bot-notification-and-info-leak-hardening.md` | 批次 1 / 3A 已落地 | 本批仅消费其稳定结果，不直接改 Bot |
| 8 | `docs/plan/architecture/schema-deployment-and-baseline-cleanup.md` | 批次 0 / 3-B 已落地 | 提供部署 / 运行基线，本批只补前端部署文档结论 |

不属于这轮主干的文档，例如 `registration-email-domain-allowlist.md`、`notification-mute-rules.md`、`database-migration-baseline-and-archive.md`、`in-app-notification-center.md` 等，不纳入本批次依赖排序。

## 背景

2026-04-25 系统性 review 在前端 Web（`services/web`）发现一处 P0 级数据造假与一组 P1 级跨标签同步 / 鉴权 / open redirect 问题，整体品味评分 🟡：

- `views/console/DashboardView.vue` 把同一个 `embyUrl` 复制 3 次伪造"主线路 / 备用线路 A/B"，对外撒谎；后端没有"多线路"概念。
- `LoginView` 接收 `?redirect=` 直接 `router.push` 到任意字符串，存在 open redirect / 协议穿透。
- `request.ts` 的 401 拦截会触发 N 个并发请求 N 次 `ElMessage.error` + N 次 `router.push('/login')`。
- 跨标签登录态不同步：localStorage 写入但没有 `storage` 事件监听，`restoreAuth` 只在 `beforeEach` 触发。
- `auth.logout()` 调用 `/logout` 时如果 token 已过期，自身会触发 401 自动登出，逻辑闭环模糊。
- admin 子路由没有在 `beforeEach` 校验父级 `requiresAuth+role`，依赖子 meta 但 `requiresAuth` 不继承；新增 admin 路由忘记加 meta 即暴露。
- 普通用户访问 admin 路由静默重定向，无任何提示。
- `RecentLibrarySection` 直接拼 `embyUrl/emby/Items/.../Images/Primary` 不带认证，依赖 Emby 公开图床。
- `DashboardView.fetchOverview` 直接修改 `userStore.embyUrl = ''`，绕过 store action。
- `EmberSegmentTabs` 用 `radiogroup/radio` 角色 + 强制 focus 行为，键盘从外部 Tab 进入会"吞焦点"。
- `RenewalCenterView` 支付 / 兑换记录分页未做并发去抖。
- `redirectToCheckout` 失败无错误反馈，按钮立刻可点。
- `SubscriptionsView` 与 `NewSubscriptionView` 用 `dangerouslyUseHTMLString` 渲染拼接字符串。
- `formatDate` 与 `Date#toLocaleString()` 在多个页面各写一遍，结果时区与展示风格不一致。
- TVCalendar `summaryCards` 用 `tone:'ink/ready/today/warning'`，与 `EmberEmptyStateCard.tone` 的 `'neutral/danger'` 命名分裂。
- `useUserStore.subscriptions` 与 `views/console/SubscriptionsView` 各自维护订阅列表，状态双轨。

如果不收口，会出现"对外撒谎多线路"、"open redirect 钓鱼"、"401 错误消息洪水"、"跨标签退出后旧 token 还能继续操作"等真实可见的体验 / 安全问题。

## 目标

本方案要实现：

1. 删除 Dashboard 假"备用线路 A/B"显示；要么走真实多线路接口，要么只显示后端返回的单条 URL
2. LoginView `redirect` 参数白名单：必须以 `/` 开头且不以 `//` 开头，且 `router.resolve` 解析存在
3. `request.ts` 401 拦截改单例化（模块级 flag），并把当前 path 写入 `redirect` 参数
4. `auth.logout()` 与 401 拦截路径解耦：拦截器跳过 `url === '/logout'` 的 401
5. 跨标签登录态同步：`useAuthStore` 初始化时挂 `storage` 事件，监听 token / role 变化
6. 路由守卫遍历 `to.matched`，任何一层有 `meta.role` 强校验；admin 页面统一复用共享 `meta.role='admin'` 定义
7. 普通用户访问 admin 路由：重定向后 `ElMessage.warning` 提示
8. `RecentLibrarySection` 海报改走后端代理（参考 `media-quality/posters/:itemId` 的代理模式）
9. Dashboard 不再直接写 store 字段，统一通过 store action
10. `EmberSegmentTabs` 仅在箭头键时 focus；明确"分段控制"语义，去掉与 tabs 面板的混淆
11. 续费记录翻页加 token 比对去抖
12. `redirectToCheckout` try/catch 包住失败提示；按钮 loading 持续到跳转或失败
13. 用 `ElMessageBox.h(...)` JSX 渲染替代 `dangerouslyUseHTMLString`，或对消息做 escape
14. 时间格式化收口到 `utils/date.ts`，提供 `formatDateTimeShort/Long/Relative` 等明确语义函数
15. tone token 统一为 `neutral / info / success / warning / danger`，所有基础组件强制使用
16. `useUserStore.subscriptions` 与页面状态收口为单一来源
17. `vite proxy` 与 prod baseURL 的反代要求写入 `docs/system-architecture.md` 部署章节

## 非目标

本次明确不做：

- 不重写 Pinia 状态管理结构
- 不新建 i18n 体系（仅收口时间格式）
- 不替换 Element Plus 组件库
- 不调整路由 lazy 分包策略
- 不引入 SSR
- 不重写 `views/console/TVCalendarView.vue` 的强业务实现，仅做 tone 收口
- 不动 admin 后台业务功能

## 当前事实

以当前代码和现行文档为准，现状如下：

- 相关文档：`docs/system-architecture.md` §3.1 / §8、`docs/reference/web-design-guide.md`
- 相关文件：
  - `services/web/src/api/request.ts`、`api/auth.ts`、`api/console.ts`
  - `services/web/src/store/auth.ts`、`store/console.ts`、`store/user.ts`、`store/admin.ts`
  - `services/web/src/router/index.ts`
  - `services/web/src/views/LoginView.vue`、`HomeView.vue`、`NotFoundView.vue`
  - `services/web/src/views/console/*.vue`、`views/admin/*.vue`
  - `services/web/src/components/console/*.vue`、`components/ember/**/*.vue`
  - `services/web/src/utils/date.ts`
  - `services/web/vite.config.ts`、`tailwind.config.js`
- 当前行为：
  - Dashboard `backupLines` 由前端硬编码复制
  - `request.ts` 401 直接 `auth.clearAuth()` + `ElMessage.error` + `router.push('/login')`
  - `useAuthStore` 没有 `storage` 事件监听
  - 路由守卫只看当前 `to.meta.role`
  - tone 命名分散
- 现有限制：
  - 前端必须遵守 Ember 风格（`docs/reference/web-design-guide.md`）
  - 后端 `/api/v1/emby/config` 仅返回单条 URL
  - 必须保持现有用户可见路由不变，例如 `/console/users`、`/console/billing`、`/console/subscriptions`；本批禁止为了守卫收口而改路由路径语义
  - 批次 2 / 3 已提供的后端契约已就位：`redispatchSubscription`、`Subscription.ingestProgress`、`PaymentStatus='expired'`、媒体质量 `scan_in_flight` 等均已可消费
  - 当前唯一未就位的直接协作项是用户侧海报代理：现有只有 `/api/v1/admin/media-quality/posters/:itemId`，普通用户不能复用

## 方案边界

本批以 `services/web` 为主，但包含 1 个必要的窄后端协作项：新增用户侧海报代理 `GET /api/v1/media/posters/:itemId`。原因很简单：`RecentLibrarySection` 现在仍在直拼 Emby 图床，继续拖着只会把“公开图床依赖”永久固化。

## 方案设计

### 1. 用户可见行为

- Dashboard：只显示后端返回的真实 Emby URL；不再展示"备用线路 A/B"
- 登录：恶意 `?redirect=https://evil.com` 被拒绝，回退到 `/console/dashboard`
- 401 弹窗只出现 1 次；浏览器 URL 只跳转 1 次
- 在 A 标签页登出后，B 标签页 1s 内自动清登录态并提示"已在其他窗口登出"
- 普通用户手输 admin 路由：弹 `ElMessage.warning("当前账号无权访问该页面")` 后重定向
- 最近入库海报：通过后端代理稳定显示，不依赖 Emby 公开图床
- 续费中心翻页：连点不会出现"先到的响应覆盖后到的"
- Stripe 结账失败：弹错误提示，按钮保持 loading 一段时间防连点
- 订阅 / 缺集等弹窗消息：不再出现 HTML 注入
- 时间格式：所有页面统一展示风格（如 `2026-04-25 14:30`）
- TVCalendar / EmptyState 等卡片 tone 配色统一

### 2. 数据与模型

> 本次不涉及后端数据模型变更。

前端约定的 tone token 与 store 字段：

#### tone token（写入 `services/web/src/components/ember/tokens.ts`）

- `neutral` — 默认
- `info` — 信息
- `success` — 成功 / ready
- `warning` — 提醒 / today / upcoming
- `danger` — 错误 / missing / 风险

所有基础组件（`EmberMetricCard`、`EmberEmptyStateCard`、`EmberSegmentTabs` 等）只接受这 5 个值。

#### store 字段

- `useAuthStore`：新增 `crossTabSyncEnabled` 标志，初始化时挂 `storage` 监听
- `useUserStore.subscriptions` / 相关 actions：标记 `@deprecated`，由 view 自管或迁到 `useSubscriptionStore`

### 3. 接口与边界

#### 后端接口

本批默认以前端改动为主，但依赖以下后端契约与 1 个必要协作接口：

- `/api/v1/emby/config` 保持单条 `url`；如未来需要多线路，由后端在响应中返回 `accessUrls: string[]` 并约定语义
- `/api/v1/admin/media-quality/posters/:itemId` 已存在的代理模式，只作为实现参考，**不能**直接给普通用户复用
- 本批新增 `GET /api/v1/media/posters/:itemId`：按当前登录用户的 `embyUserID` 鉴权后代理 Emby 图床，供 `RecentLibrarySection` 使用
- `/api/v1/logout` 后端实现保持 stub 语义不变；前端文档明确"仅清本端"

#### 前端契约

- `request.ts` 拦截器：
  - 401 → 单例化 redirect；跳过 `url === '/logout'`
  - 5xx → 透传业务错误码；禁止裸渲染 `error.message` 为 HTML
- `useAuthStore`:
  - `setAuth(payload)` / `clearAuth()` 调用时通过 `localStorage.setItem` 同步；新增 `onStorage(handler)` 注册函数
  - SSR 安全：`typeof window !== 'undefined'` 判断
- 路由守卫：
  - 遍历 `to.matched`，任意一层 `meta.role` 不匹配则重定向到 `/console/dashboard` + warning 提示
  - 任意一层 `meta.requiresAuth` 为 true 则强校验 token
  - 未登录 → `/login?redirect=<encoded path>`
- `LoginView`:
  - `redirect = route.query.redirect`
  - 校验：`typeof redirect === 'string' && redirect.startsWith('/') && !redirect.startsWith('//') && router.resolve(redirect).matched.length > 0 && router.resolve(redirect).name !== 'not-found'`
  - 不通过：回退到 `/console/dashboard`
- 时间格式（`utils/date.ts`）：
  - `formatDateTime(value, { style: 'short' | 'long' | 'date' | 'relative' })`
  - 所有页面统一调用，禁止 `Date#toLocaleString()` 直接写在 view

### 4. 关键流程

#### 4.1 401 单例化

1. `request.ts` 顶层声明 `let redirecting = false`
2. response 拦截器命中 401（且 `config.url !== '/logout'`）：
   - 已 redirecting → 直接 reject
   - 否则 → 设 redirecting=true → `auth.clearAuth()` → `ElMessage.error('登录已过期')` → `router.push({ path: '/login', query: { redirect: router.currentRoute.value.fullPath } })` → 设 redirecting=false（在 router 守卫完成回调里重置）

#### 4.2 跨标签同步

1. `useAuthStore.restoreAuth()` 在 `App.vue` 挂载时调用
2. 同时注册 `window.addEventListener('storage', handler)`：
   - 监听 `token` / `role` 键变化
   - 当前 store 与 storage 不一致 → 同步并清理相关 store（`useConsoleStore.clear()` / `useUserStore.clear()`）
   - 必要时 `router.push({ path: '/login' })` + `ElMessage.warning("已在其他窗口登出")`

#### 4.3 路由守卫强化

1. `beforeEach((to, from, next))`:
   - 遍历 `to.matched`：收集所有 `meta.requiresAuth` / `meta.role`
   - 若 `requiresAuth=true` 但无 token → push `/login?redirect=<to.fullPath>`
   - 若 `meta.role` 与当前角色不匹配 → push `/console/dashboard` + ElMessage.warning
   - 否则 next()
2. 保持现有 `/console/users`、`/console/billing`、`/console/settings` 等路径不变；仅收口 `meta` 声明与守卫逻辑，不改用户已知 URL
3. 对 admin 页面使用共享 `adminRouteMeta` 常量或等价方式，避免新增页面时漏写 `meta.role='admin'`

#### 4.4 LoginView redirect 校验

1. 取 `query.redirect`
2. 校验合法性（见 §3）
3. 不通过：回退到 `/console/dashboard`
4. 通过：`router.replace(redirect)`（避免 history 增长）

#### 4.5 Dashboard 删假数据

1. 删除 `backupLines` 数组
2. 模板中"主线路 / 备用线路 A/B"区块改为单条 URL + 复制按钮
3. 后续若后端引入 `accessUrls: string[]`，按数组渲染（0/1/N 项）

#### 4.6 RecentLibrarySection 海报代理

1. 后端新增 `GET /api/v1/media/posters/:itemId`（按 `embyUserID` 鉴权后代理 Emby 图床）
2. 前端用该接口替换 `embyUrl/emby/Items/.../Images/Primary` 的拼接
3. 失败 fallback 占位图

#### 4.7 EmberSegmentTabs 焦点收敛

1. ArrowLeft / ArrowRight：切换 + focus
2. 鼠标点击 / 程序切换：仅切换，不抢 focus
3. role 改为 `tablist / tab`（如有 `aria-controls` 关联面板）或显式声明"分段控制"，注释说明非 tabs 语义

#### 4.8 dangerouslyUseHTMLString 替换

1. `formatExistingSummary` 等返回纯文本
2. ElMessageBox 调用改为 `message: () => h('div', { class: 'whitespace-pre-line' }, text)`
3. 不再 `replace(/\n/g, '<br/>')`

#### 4.9 时间格式收口

1. `utils/date.ts` 提供：
   - `formatDateTime(value, 'short' | 'long' | 'date' | 'relative')`
   - `formatPlaybackDuration(seconds)`
2. sweep 所有 `toLocaleString` / `toLocaleDateString` / 自拼时间格式调用，统一替换

#### 4.10 tone token 收口

1. 新建 `components/ember/tokens.ts` 导出 `Tone = 'neutral'|'info'|'success'|'warning'|'danger'`
2. 所有基础组件 props 改用 `Tone` 类型
3. TVCalendar `summaryCards` 把 `ink/ready/today/warning` 映射到新 token
4. `EmberEmptyStateCard.tone` 接受新 token；旧 `neutral/danger` 兼容一段时间后移除

### 5. 失败路径与边界条件

- **后端 `/emby/config` 返回空字符串**：UI 显示"未配置 Emby 地址"，不显示线路区块
- **localStorage 被禁用（隐私模式）**：跨标签同步降级为不同步，`useAuthStore` 仅在当前 tab 有效
- **admin 页面漏写 `meta.role`**：不改路径；开发期通过共享 `adminRouteMeta`、路由测试与 sweep 清单兜底，避免漏网
- **海报代理 5xx**：fallback 占位图，前端不报错
- **EmberSegmentTabs 在低版本浏览器无键盘事件**：行为退化为仅鼠标点击
- **`router.resolve(redirect)` 命中通配 NotFound 路由**：视为不通过，回退 dashboard
- **storage 事件被同源 iframe 触发**：用 newValue / oldValue 判断真实变化，避免误同步
- **dangerouslyUseHTMLString 替换后用户可见行为**：换行展示靠 CSS `whitespace-pre-line`

## 影响范围

- API：
  - 新增（必做）：`GET /api/v1/media/posters/:itemId` 海报代理（实现参考 `admin/media-quality/posters/:itemId`）
- Web：
  - 修改：`api/request.ts`、`store/auth.ts`、`router/index.ts`、`views/LoginView.vue`、`views/console/DashboardView.vue`、`views/console/RenewalCenterView.vue`、`views/console/SubscriptionsView.vue`、`views/console/NewSubscriptionView.vue`、`views/console/TVCalendarView.vue`、`components/ember/feedback/EmberEmptyStateCard.vue`、`components/ember/data-display/EmberMetricCard.vue`、`components/ember/layout/EmberSegmentTabs.vue`、`components/console/RecentLibrarySection.vue`、`utils/date.ts`、`store/user.ts`
  - 新增：`components/ember/tokens.ts`
- Bot：不变
- 配置 / 部署：
  - 无 SQL migration
  - `vite.config.ts` 与反代要求写入文档
- 文档：
  - `docs/reference/web-design-guide.md` 增补 tone token 与时间格式规范
  - `docs/system-architecture.md` §8 改写"401 单例化 / 跨标签同步 / redirect 白名单"
  - 部署章节增补"前端 baseURL 反代要求"

## 实施顺序

按依赖关系与风险等级，批次 4 建议拆成 4 个 PR，顺序如下：

### PR-1：前端鉴权红线

范围：

- `api/request.ts`：401 单例化、跳过 `/logout`、保留 redirect
- `store/auth.ts`：跨标签同步、清理 console/user store
- `router/index.ts`：遍历 `to.matched`、普通用户访问 admin 给 warning
- `views/LoginView.vue`：`redirect` 白名单

理由：

- 这是本批唯一的纯安全 / 鉴权红线，独立、风险高、最该先收口
- 不依赖海报代理、时间格式、tone token 等后续工作

### PR-2：Dashboard 真相收口 + 用户侧海报代理

范围：

- `views/console/DashboardView.vue`：删除假“备用线路 A/B”，改 store action
- `components/console/RecentLibrarySection.vue`：切换到 `/api/v1/media/posters/:itemId`
- `services/api`：新增用户侧 poster proxy handler / route / service 复用

理由：

- 这是本批唯一仍未补齐的后端协作项
- “对外撒谎多线路”和“依赖公开图床”都属于用户直接可见问题，优先级高于纯风格 sweep

### PR-3：前端竞态与注入面收口

范围：

- `views/console/RenewalCenterView.vue`：翻页 token 去抖、`redirectToCheckout` 错误反馈与 loading
- `views/console/SubscriptionsView.vue`、`views/console/NewSubscriptionView.vue`：替换 `dangerouslyUseHTMLString`
- `store/user.ts` / 页面状态：收口订阅列表双轨

理由：

- 依赖批次 2 的 `expired`、批次 3 的 `redispatch` / `ingestProgress` 契约，当前这些契约已稳定
- 改动集中在交互一致性与注入面，适合单独回归

### PR-4：时间格式、tone token 与文档提炼

范围：

- `utils/date.ts` 与全站调用点 sweep
- `components/ember/tokens.ts` 与基础组件 tone 收口
- `TVCalendarView.vue` tone 映射统一
- `docs/reference/web-design-guide.md`、`docs/system-architecture.md` 文档同步

理由：

- 这是典型“链路收尾”工作，价值高，但不该阻塞前 3 个更硬的 PR
- 适合在行为已经稳定后一次性 sweep，避免多轮反复改 view

## 验证方式

### 编译 / 测试

- `cd services/web && npm run build`
- `cd services/web && npm run type-check`（如有）
- `cd services/web && npm run lint`（如有）

### 手工验证

#### Dashboard 多线路
- 进入 `/console/dashboard`：只显示一条 Emby URL，无"备用线路 A/B"

#### LoginView redirect
- 访问 `/login?redirect=https://evil.com`：登录后跳到 `/console/dashboard`
- 访问 `/login?redirect=//evil.com`：同上
- 访问 `/login?redirect=/console/users`（admin 路由，普通用户登录）：跳到 `/console/dashboard` 并提示

#### 401 单例化
- 让 token 过期，同时触发 5 个并发请求：只弹 1 次错误，URL 只跳转 1 次

#### logout 与 401 解耦
- token 过期后点登出：不会触发额外的 401 弹窗

#### 跨标签同步
- A 标签登录，B 标签同时打开 → A 登出 → B 在 1 秒内自动跳到登录页 + 提示

#### 路由守卫
- 普通用户访问 `/console/users`：跳转 + warning
- 新增 admin 页面若漏写 `meta.role`：路由测试 / sweep 清单能发现，避免无提示暴露

#### RecentLibrarySection 海报
- 反代 Emby 不允许匿名图床 → 海报通过后端代理稳定显示

#### EmberSegmentTabs 焦点
- 鼠标点击切换：焦点不被抢
- 键盘 ArrowLeft/Right 切换：焦点跟随

#### 续费翻页去抖
- 连点页码 5 次：最终 UI 与最后一次响应一致

#### redirectToCheckout
- mock Stripe 失败：toast 显示 + 按钮 loading 1s 后释放

#### dangerouslyUseHTMLString
- 后端文案含 `<script>`：渲染为纯文本，不执行

#### 时间格式
- Dashboard / Subscriptions / RenewalCenter / Users / Account 各页面时间显示一致

#### tone 一致性
- TVCalendar `summaryCards` 配色与 `EmberEmptyStateCard` 一致；`tokens.ts` 单一来源

### 修复后验证清单

- [ ] `npm run build` 通过且 bundle 体积无显著回归
- [ ] `npm run type-check` 通过
- [ ] Dashboard 假数据已删除
- [ ] LoginView open redirect 测试用例通过
- [ ] 401 拦截单例化在测试环境 100 并发请求下只弹 1 次
- [ ] 跨标签同步在 Chrome / Safari 验证通过
- [ ] 路由守卫遍历 matched 在 admin / user / 公共三类路由验证通过
- [ ] `/api/v1/media/posters/:itemId` 在普通用户权限下可用，且不能越权拉取其他用户图床
- [ ] tone token 文档同步到 `docs/reference/web-design-guide.md`
- [ ] 时间格式收口完成，`grep -r "toLocaleString" services/web/src` 不返回 view 层

### 二次暴露检查清单

- [ ] sweep 所有 `dangerouslyUseHTMLString` / `v-html` 用法
- [ ] sweep 所有 `Date#toLocaleString` / `toLocaleDateString` 在 view 层的用法
- [ ] sweep 所有 store 字段被 view 直接写入的位置（绕过 action）
- [ ] sweep 所有 `router.push(string)` 字符串形式，确认不接收用户输入
- [ ] sweep 所有基础组件的 `tone` / `variant` props 命名是否统一
- [ ] sweep 所有 `RowsAffected` / `success+info` 等不符合统一响应规范的接口接入处
- [ ] 复核 `useUserStore.subscriptions` 与新 `useSubscriptionStore` 的边界
- [ ] 复核所有"按钮重复点击"风险（结账 / 兑换 / 订阅 / 重置密码）
- [ ] 复核所有 admin 路由都显式复用统一 `meta.role='admin'`，而不是靠路径猜测权限

## 落地后文档处理

- 已提炼：
  - `docs/system-architecture.md` §8：前端鉴权链路、401 单例化、跨标签登录态同步、redirect 白名单
  - `docs/reference/web-design-guide.md` §3.5：tone token 与时间格式基线
- 归档前仍需补的收尾：
  - `docs/plan/README` / `docs/proposals/README` / `docs/proposals/plan-inventory.md` 入口状态保持一致
  - 若后续补更细的 `vite proxy / prod baseURL` 运维步骤，落到 `docs/runbooks/`，不再回流到本实施稿
- 本方案完成归档准备后，移入 `docs/archive/plan/console-admin/`
- P2 / P3 中未顺手收口的项转交下一轮“前端一致性治理”，不阻塞本文退场

## 附录：问题清单与本方案条目映射

| review 编号 | 问题 | 本方案条目 |
|---|---|---|
| P0-1 (前端) | Dashboard 多线路造假 | §4.5 |
| P1-1 (前端) | LoginView redirect open redirect | §4.4 |
| P1-2 (前端) | 401 拦截无单例化 | §4.1 |
| P1-3 (前端) | logout 与 401 拦截耦合 | §4.1 |
| P1-4 (前端) | 跨标签登录态不同步 | §4.2 |
| P1-5 (前端) | admin 子路由守卫继承缺失 | §4.3 |
| P1-6 (前端) | 普通用户访问 admin 静默重定向 | §4.3 |
| P2-1 (前端) | RecentLibrarySection 拼公开图床 | §4.6 |
| P2-2 (前端) | Dashboard 直接写 store | §4 + store action |
| P2-3 (前端) | EmberSegmentTabs 焦点抢占 | §4.7 |
| P2-4 (前端) | 续费翻页未去抖 | §4 + token 比对 |
| P2-5 (前端) | redirectToCheckout 失败无提示 | §4 |
| P2-6 (前端) | dangerouslyUseHTMLString 注入 | §4.8 |
| P2-7 (前端) | 时间格式散落 | §4.9 |
| P2-8 (前端) | tone 命名分裂 | §4.10 |
| P2-9 (前端) | useUserStore.subscriptions 双轨 | §2 + §3 |
| P2-10 (前端) | vite proxy / baseURL 文档缺失 | 文档同步 |
| P3-1~P3-7 (前端) | 风格 / icon / chunks | 二次暴露清单 |
