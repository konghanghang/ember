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

截至 2026-10-25，`docs/plan/` 共 10 份现行实施稿：3 份继续推进实现或验收、6 份待开发、1 份处于观察期。实施归档和外部验收分开记录，未验证事项由主计划或现行 runbook/reference 跟踪。

- [自助注册人数上限](./access-auth/registration-user-capacity.md)（待开发）
- [分组观看保号](./billing-redemption/watch-activity-renewal.md)（实现、专用 PostgreSQL 与管理员页面验收完成；10 月 18 日起点修复已落地，10 月 23 日有包含该修复的历史部署记录；用户端页面、真实考核与权限同步仍待验收）
- [Emby–115 DirectPlay 总计划](./architecture/emby-115-direct-play-gateway.md)（播放数据面主体完成，单 Gateway 运维功能与部分真实取证待完成）
- [Gateway 客户端兼容与转存策略](./architecture/gateway-client-compatibility.md)（协议、诊断、preID 与持续自动化本地完成；承接起播许可和 SenPlayer 分项验收，远端 CI、真实客户端与性能收益待验证；默认严格，兼容模式由管理员开启）
- [运行期配置缓存演进](./architecture/runtime-settings-cache-evolution.md)（观察期，尚未排期；没有实证不替换）
- [Telegram 通知静音规则](./bot-telegram/notification-mute-rules.md)（待开发；已有结果通知不等于静音治理）
- [群签到与双途径保号](./bot-telegram/group-checkin-retention.md)（产品规则已确认，整体待实施；正文已同步 10 月 18 日观看保号起点边界，首次日期持久化、签到、迁移与双条件考核尚未实现）
- [设备风控自动化](./console-admin/device-risk-automation.md)（待开发）
- [站内通知中心](./console-admin/in-app-notification-center.md)（待开发）
- [媒体去重与质量治理](./media-subscription/media-dedupe-and-quality-governance.md)（待开发）

最近已完成归档的实施稿包括：

- [套餐与分组权益改版](../archive/plan/billing-redemption/plan-group-entitlements.md)（2026-10-25：实施与本地/沙盒验收收尾；剩余业务验收和种子用户迁组状态由[升级手册](../runbooks/entitlements-upgrade.md)维护）
- [115 个人账号与 Redis 配额](../archive/plan/architecture/p115-personal-account-routing-and-redis-quotas.md)（2026-10-25：四阶段实施与数据库专项完成；真实个人 Cookie/Redis/配额验收留在[端到端参考](../reference/p115-playback-end-to-end-flow.md)）
- [115 新增转存起播许可](../archive/plan/architecture/p115-transfer-playback-intent.md)（2026-10-25：实施完成，剩余取证由客户端兼容主计划接替，未标为真实验收通过）
- [SenPlayer 原文件播放兼容](../archive/plan/architecture/senplayer-original-stream-compatibility.md)（2026-10-25：适配已实现，剩余 302/CDN/字幕等分项由客户端兼容主计划接替）
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
