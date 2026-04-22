# Ember Web

Vue 3 + TypeScript 前端，承载首页、登录注册、统一控制台和管理后台。

## 本地验证

```bash
cd services/web
npm ci
npm run build
```

如果需要本地调试：

```bash
npm run dev
```

## 目录骨架

```text
services/web/src/
├── api/                       # Axios 封装
├── components/                # 公共组件
├── components/console/        # 控制台布局组件
├── router/                    # 路由与守卫
├── store/                     # Pinia 状态
├── types/                     # TypeScript 类型
├── views/admin/               # 管理端页面
├── views/console/             # 统一控制台页面
└── views/user/                # 注册等用户入口
```

## 当前页面结构

- 公开页：`HomeView`、`LoginView`、`RegisterView`、`ForgotPasswordView`
- 控制台：`/console/*`
  - `DashboardView`
  - `SubscriptionsView`
  - `TVCalendarView`
  - `RankingsView`
  - `RenewalCenterView`
  - `RecentLibrarySection`（概览页内最近入库摘要）
  - 旧 `/console/library` 已重定向到 `/console/dashboard`
- 管理端功能页面：
  - `UsersView`
  - `RedemptionCodesView`
  - `RedemptionHistoryView`
  - `SettingsView`
  - `PlansView`
  - `PaymentsView`
  - `SessionsView`
  - `PlaybackHistoryView`
  - `MediaQualityView`
  - `DevicesView`

## 技术约束

- 路由由 `src/router/index.ts` 统一管理
- API 类型定义以 `src/types/api.ts` 为准
- 统一认证接口优先走 `src/api/console.ts`
- 管理端接口走 `src/api/admin.ts`

设计和交互约束见：

- [Web 设计规范](/Users/konghang/data/me/github/ember/docs/reference/web-design-guide.md)
- [系统架构](/Users/konghang/data/me/github/ember/docs/system-architecture.md)

## 测试

可用脚本：

- `npm run build`
- `npm run test`
- `npm run test:unit`
- `npm run test:component`

手工回归范围见 [手工测试清单](/Users/konghang/data/me/github/ember/docs/runbooks/manual-testing-checklist.md)。

## 部署

- 生产镜像由 [`Dockerfile`](./Dockerfile) 构建
- nginx 配置见 [`nginx.conf`](./nginx.conf)
- 部署入口见 [部署指南](/Users/konghang/data/me/github/ember/docs/runbooks/deployment.md)
