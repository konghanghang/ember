# 前端鉴权与设计基线收口方案

> 状态：已归档（主链路稳定，转为历史追溯）
> 负责人：Ember
> 更新时间：2026-04-30

本文档已退出现行实施稿目录。当前前端鉴权与设计基线事实以 `docs/system-architecture.md`、`docs/reference/web-design-guide.md` 为准；本文件仅保留历史实施过程与决策追溯价值。

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

## 稳定结论

以下结论已经提炼为当前事实：

- 前端统一认证入口固定为 `/api/v1`，401 收口为“清本地登录态 + 跳 `/login?redirect=`”，且 `/login`、`/logout` 不混入“登录过期”逻辑。
- 路由守卫必须遍历 `to.matched`，`redirect` 仅允许站内已解析路径，普通用户命中 admin 路由必须给出提示而不是静默吞掉。
- 最近入库封面必须走 `GET /api/v1/media/posters/:itemId` 代理，不再直拼 Emby 图床。
- tone token 与时间格式已经收口为基础组件契约，不再允许页面各自造词或直接在 view 中拼 `toLocaleString()`。

## 交叉引用

- 当前系统事实：
  - [docs/system-architecture.md](</Users/konghang/data/me/github/ember/docs/system-architecture.md>) §8 已收录前端鉴权链路、401 单例化、跨标签同步、redirect 白名单
  - [docs/system-architecture.md](</Users/konghang/data/me/github/ember/docs/system-architecture.md>) “最近入库”段已收录用户侧海报代理边界
- 当前设计规范：
  - [docs/reference/web-design-guide.md](</Users/konghang/data/me/github/ember/docs/reference/web-design-guide.md>) §3.5 已收录 tone token 与时间格式基线
- 当前盘点入口：
  - [docs/proposals/plan-inventory.md](</Users/konghang/data/me/github/ember/docs/proposals/plan-inventory.md>) 已把本方案标为已归档

## 退场说明

- 本文档不再承担当前事实说明职责；现行事实以 `docs/system-architecture.md` 和 `docs/reference/web-design-guide.md` 为准。
- 顶部状态、交叉引用与入口文档已完成归档收口，因此本文件只保留历史追溯价值。

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
