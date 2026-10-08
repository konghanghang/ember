# 功能方案

这里放具体功能或模块的实现方案。默认要求是：写到足够让另一个工程师直接开做，但不要把文档写成源码替代品。

## 什么时候放这里

- 新功能设计
- 重要功能重构方案
- 需要明确接口、数据结构、行为边界的实施稿

## 目录规则

`docs/plan/` 根目录只保留入口和模板：

- `README.md`
- `plan-template.md`

新增计划文档默认按职责边界落位，不再平铺在根目录。

当前标准目录为：

- `docs/plan/access-auth/`
- `docs/plan/billing-redemption/`
- `docs/plan/media-subscription/`
- `docs/plan/bot-telegram/`
- `docs/plan/console-admin/`
- `docs/plan/architecture/`

具体归类规则见 [docs/reference/plan-directory-governance.md](../reference/plan-directory-governance.md)。

## 不该放这里的内容

- 文档治理、盘点、重构策略：放 `docs/proposals/`
- 已完成或废弃的旧方案：移到 `docs/archive/`
- 稳定规则或现行事实：提炼到 `docs/reference/` 或 `docs/system-architecture.md`

当前 `docs/plan/` 中待实施、推进中或待验收的实施稿共 12 份：

- `access-auth/registration-user-capacity.md`
- [分组观看保号](./billing-redemption/watch-activity-renewal.md)（2026-10-08：已确认分组三项配置（开关、周期天数、分钟门槛），观察期与统计窗口共用周期（默认 30 天），实际接替后开始完整观察期，付费组替代期间暂停考核，再次接替重新起算；代码与本地自动化已落地，专用 PostgreSQL 验收通过，真实播放/浏览器验收待完成）
- [套餐与分组权益改版](./billing-redemption/plan-group-entitlements.md)（2026-10-04：首轮实现与页面/Stripe 沙盒验收完成；单分组套餐收敛、测试库转换与 Stripe 沙盒复验完成，待提交收尾）
- `architecture/emby-115-direct-play-gateway.md`
- `architecture/p115-personal-account-routing-and-redis-quotas.md`（阶段 0–3 已落地，基础 PostgreSQL 集成于 2026-09-05 通过；后续 DirectPlay 数据库专项于 2026-09-21 补跑通过，受控真实外部验收仍待完成）
- [115 新增转存起播许可](./architecture/p115-transfer-playback-intent.md)（代码与自动化已完成，保留已有文件和跨会话缓存复用；待受控客户端验收）
- `architecture/runtime-settings-cache-evolution.md`
- `bot-telegram/notification-mute-rules.md`
- [群签到与双途径保号](./bot-telegram/group-checkin-retention.md)（2026-10-08：产品规则已确认，待实施；单开关、共用 N 天，观看或签到任一达标即可保号；签到默认 5 天，PostgreSQL 明细保留 3 个日历月并自动清理；权益保存首次检查日期，修改规则不重置起点；技术检查与迁移验证待执行）
- `console-admin/device-risk-automation.md`
- `console-admin/in-app-notification-center.md`
- `media-subscription/media-dedupe-and-quality-governance.md`

最近已完成归档的实施稿包括：

- [每日播放总结分步修复](../archive/plan/media-subscription/playback-summary-improvements.md)（2026-09-29：五步本地实施完成；回查去重已落地，SQL 合并经成本评估暂缓，真实 Emby / Telegram 与部署未执行）
- [支付与媒体状态问题修复计划](../archive/plan/media-subscription/open-issue-followup.md)（2026-09-17：#21–#25 已修复并逐项本地提交；真实 PostgreSQL/外部验证未执行）
- [GitHub 开放问题零迁移修复计划](../archive/plan/architecture/github-issue-remediation.md)（2026-09-16：9 项修复、2 项已有修复验收；用户确认 fake 验收，真实 PostgreSQL 未执行）
- `architecture/gateway-media-path-diagnostics.md` → `docs/archive/plan/architecture/gateway-media-path-diagnostics.md`
- `architecture/ember-gateway-transparent-proxy-and-web-access.md` → `docs/archive/plan/architecture/ember-gateway-transparent-proxy-and-web-access.md`
- `architecture/web-frontend-quality-improvement.md` → `docs/archive/plan/architecture/web-frontend-quality-improvement.md`
- `architecture/p115-path-resolution-without-emby-size.md` → `docs/archive/plan/architecture/p115-path-resolution-without-emby-size.md`
- `console-admin/web-layout-design-improvement.md` → `docs/archive/plan/console-admin/web-layout-design-improvement.md`
- `media-subscription/subscription-manual-moviepilot-dispatch.md` → `docs/archive/plan/media-subscription/subscription-manual-moviepilot-dispatch.md`
- `architecture/project-wide-log-level.md` → `docs/archive/plan/architecture/project-wide-log-level.md`
- `access-auth/admin-api-key.md` → `docs/archive/plan/access-auth/admin-api-key.md`
- `architecture/settings-key-cache.md` → `docs/archive/plan/architecture/settings-key-cache.md`
- `console-admin/plan-group-media-library-deferred-sync.md` → `docs/archive/plan/console-admin/plan-group-media-library-deferred-sync.md`
- `media-subscription/playback-ranking-library-allowlist.md` → `docs/archive/plan/media-subscription/playback-ranking-library-allowlist.md`
- `media-subscription/subscription-plan-group-auto-approval.md` → `docs/archive/plan/media-subscription/subscription-plan-group-auto-approval.md`
- `media-subscription/user-media-library-management.md` → `docs/archive/plan/media-subscription/user-media-library-management.md`
- `console-admin/console-overview-account-layout-redesign.md` → `docs/archive/plan/console-admin/console-overview-account-layout-redesign.md`
- `bot-telegram/subscription-admin-message-sync.md` → `docs/archive/plan/bot-telegram/subscription-admin-message-sync.md`
- `architecture/oss-deployment-experience.md` → `docs/archive/plan/architecture/oss-deployment-experience.md`
- `architecture/database-migration-auto-apply.md` → `docs/archive/plan/architecture/database-migration-auto-apply.md`
- `console-admin/admin-emby-binding.md` → `docs/archive/plan/console-admin/admin-emby-binding.md`
- `architecture/baseline-fresh-install-rewrite.md` → `docs/archive/plan/architecture/baseline-fresh-install-rewrite.md`
- `architecture/system-architecture-document-split.md` → `docs/archive/plan/architecture/system-architecture-document-split.md`

## 模板

- [功能方案模板](./plan-template.md)

## 编写标准

- 讲清目标、非目标、影响面和验收方式
- 写清接口、数据结构、用户可见行为
- 不贴大段实现代码
- 不写“先这样后面再说”的空话
- 新文档创建前，先判断主链路属于哪个职责目录；不要默认直接放在 `docs/plan/` 根目录
