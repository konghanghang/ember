# `docs/plan` 盘点清单

> 更新时间：2026-04-15

本清单只回答三件事：

1. 这份方案文档在代码里有没有落地证据
2. 它现在更适合继续留在 `docs/plan/`，还是进入归档队列
3. 如果要归档，前面还缺不缺稳定结论的提炼

盘点基准：

- 以当前代码结构和 [系统架构文档](../system-architecture.md) 为准
- “文档里写了已完成”不算证据，必须能在代码或架构文档里找到落点
- 本清单反映 2026-03-29 的盘点结果；若后续目录已调整，以仓库当前文件位置为准

## A. 已落地，已完成归档

这些文档已经有明确代码落点，并已从 `docs/plan/` 退出。

| 文档 | 盘点结论 | 主要证据 | 建议动作 |
|------|----------|----------|----------|
| `active-sessions.md` | 已落地 | `SessionHandler`、`/api/v1/admin/sessions`、`views/admin/SessionsView.vue`、架构文档已收录 | 已提炼后归档到 `docs/archive/plan/console-admin/` |
| `email-verification.md` | 已落地 | `email_verifications` 模型、`SendEmailCode`、注册验证码路由、设置中心配置项 | 已提炼后归档到 `docs/archive/plan/access-auth/` |
| `forgot-password.md` | 已落地 | `SendResetCode`、`ResetPasswordByCode`、`ForgotPasswordView.vue`、Bot `/resetpw` | 已提炼后归档到 `docs/archive/plan/access-auth/` |
| `latest-media.md` | 已落地 | `/api/v1/media/latest`、`MediaService.GetLatestItems`、`LibraryView.vue` | 已提炼后归档到 `docs/archive/plan/media-subscription/` |
| `playback-ranking.md` | 已落地 | `RankingHandler`、`PlaybackRankingService`、`RankingsView.vue`、排行 cron | 已提炼后归档到 `docs/archive/plan/media-subscription/` |
| `redemption-code-batch-create.md` | 已落地 | `CreateRedemptionCodesBatch`、批量接口、管理端批量创建 UI | 已归档到 `docs/archive/plan/billing-redemption/` |
| `redemption-code-one-per-user.md` | 已落地 | 一人一码约束、兑换历史接口、用户端/管理端兑换历史 UI | 已归档到 `docs/archive/plan/billing-redemption/` |
| `settings-center.md` | 已落地 | 文档自身已写“已完成”，代码中有 `config/`、`handlers/config.go`、`SettingsView.vue` | 已提炼后归档到 `docs/archive/plan/architecture/` |
| `stripe-payment.md` | 已落地 | `PaymentService`、`Plan`/`Payment` 模型、`PaymentsView.vue`、`PlansView.vue` | 已提炼后归档到 `docs/archive/plan/billing-redemption/` |
| `billing-redemption/user-plan-grouping.md` | 已落地 | `plan_groups` 模型与 migration、`PaymentService` 分组校验、`PlanGroupsView.vue`、`UsersView.vue`、架构文档已收录有效分组与后台分组管理接口 | 已提炼后归档到 `docs/archive/plan/billing-redemption/` |
| `docs/archive/plan/billing-redemption/redemption-code-registration-plan-group.md` | 已落地 | `redemption_codes.registrationPlanGroup`、注册/续期分离校验、invite 注册写入 `users.planGroup`、后台兑换码管理页和分组删除阻断均已落地 | 已归档到 `docs/archive/plan/billing-redemption/` |
| `telegram-binding.md` | 已落地 | `GenerateBindCode`、`VerifyBind`、Bot `/bind` `/info` `/redeem`、Dashboard 绑定入口 | 已提炼后归档到 `docs/archive/plan/bot-telegram/` |
| `telegram-bot-menu.md` | 已落地 | `menu_sync.py`、`/refresh_menu`、Bot 启动菜单同步 | 已归档到 `docs/archive/plan/bot-telegram/` |
| `bot-polling-mode.md` | 已落地 | `TELEGRAM_UPDATE_MODE`、Bot `webhook/polling` 双模式启动、`docker-compose.yml` 透传、配置中心条件风险提示 | 已提炼后归档到 `docs/archive/plan/bot-telegram/` |
| `telegram-search-subscribe.md` | 已落地 | `SubscribeByTelegram`、Bot `/search`、搜索会话缓存、内部订阅接口 | 已提炼后归档到 `docs/archive/plan/bot-telegram/` |
| `telegram-search-multi-type.md` | 已落地 | TMDB `type=multi`、Bot `_do_search(..., "multi")` | 已归档到 `docs/archive/plan/bot-telegram/` |
| `telegram-subscription-notification.md` | 已落地 | `BotNotifier`、`/notify/subscription`、Bot 审批回调、InternalAuth | 已提炼后归档到 `docs/archive/plan/bot-telegram/` |
| `unified-console.md` | 已落地 | `/console/*` 路由、旧 `/admin/*` `/user/*` 重定向、统一 `Layout.vue` | 已归档到 `docs/archive/plan/console-admin/` |
| `welcome-message.md` | 已落地 | `handle_new_member`、`notify_group_link`、运行期设置读取 | 已归档到 `docs/archive/plan/bot-telegram/` |
| `embypulse-features/p0-device-management.md` | 已落地 | 设备接口、`DeviceService`、`DevicesView.vue` | 已归档到 `docs/archive/plan/console-admin/` |
| `embypulse-features/p0-permission-template.md` | 已落地 | `templateUserId` 字段、模板用户列表、兑换码管理 UI | 已归档到 `docs/archive/plan/console-admin/` |
| `embypulse-features/p0-tv-calendar.md` | 已落地 | `TVCalendarService`、全局/关注周历、同步与 webhook 就绪标记、`TVCalendarView.vue` | 已提炼后归档到 `docs/archive/plan/media-subscription/` |
| `embypulse-features/p1-media-quality.md` | 已落地 | 媒体库列表、质量报告、明细接口、`MediaQualityView.vue` | 已归档到 `docs/archive/plan/media-subscription/` |
| `embypulse-features/p1-playback-history.md` | 已落地 | `PlaybackHistoryService`、管理端播放历史路由与页面 | 已归档到 `docs/archive/plan/media-subscription/` |
| `embypulse-features/p2-subscription-season.md` | 已落地 | `Subscription.season` 字段、Web 分季选择、MoviePilot `season` 透传、Bot 先选季再确认 | 已归档到 `docs/archive/plan/media-subscription/` |
| `media-subscription/moviepilot-api-key-direct-integration.md` | 已落地 | `MoviePilotClient` 已改为 `X-API-KEY` 直连，配置中心改用 `MOVIEPILOT_API_KEY`，架构/配置/部署文档已同步 | 已提炼后归档到 `docs/archive/plan/media-subscription/` |
| `media-subscription/tv-calendar-status-correction.md` | 已落地 | TV Calendar 状态已切到 `CRON_TIMEZONE`，默认同步窗口扩到本周+下周，当前周读时 `ready` 纠偏与 webhook 关键日志已补齐 | 已提炼后归档到 `docs/archive/plan/media-subscription/` |
| `embypulse-features/p2-user-avatar.md` | 已落地 | `DefaultAvatar.vue`、Dashboard / Account Center / TopBar / Sidebar / UsersView 已统一接入默认头像组件 | 已归档到 `docs/archive/plan/console-admin/` |
| `console-admin/admin-create-user-with-plan-group-expiry.md` | 已落地 | `POST /api/v1/admin/users`、`CreateUserByAdmin`、`UsersView.vue` 新建用户弹窗、架构文档已收录后台创建用户接口 | 已提炼后归档到 `docs/archive/plan/console-admin/` |
| `console-admin/ember-web-component-foundation.md` | 已落地 | `services/web/src/components/ember/` 基础组件层、后台/控制台页头与 tabs 收口、表单基线统一、empty state 组件化、前端残留清理均已落地 | 已提炼后归档到 `docs/archive/plan/console-admin/` |

## B. 当前仍留在 `docs/plan/` 的文档

这些文档当前还没有退出 `docs/plan/`。其中一部分尚未完整落地，另一部分已经完成收口、下一步应执行归档动作。

| 文档 | 盘点结论 | 主要原因 | 建议动作 |
|------|----------|----------|----------|
| `bot-telegram/notification-mute-rules.md` | 继续保留 | 本轮未见它已完成归档所需的稳定结论提炼与退场动作 | 继续按实施方案维护 |
| `console-admin/device-risk-automation.md` | 继续保留 | 本轮未见它已完成归档所需的稳定结论提炼与退场动作 | 继续按实施方案维护 |
| `console-admin/in-app-notification-center.md` | 继续保留 | 本轮未见它已完成归档所需的稳定结论提炼与退场动作 | 继续按实施方案维护 |
| `media-subscription/gap-management-and-precision-download.md` | 已落地 | `media_gaps` 模型与 migration、`MediaGapService`、`/api/v1/admin/media-gaps/grouped`、`MediaGapsView.vue`、MoviePilot 候选搜索与真实下发链路、架构文档已收录 | 已提炼后归档到 `docs/archive/plan/media-subscription/` |
| `media-subscription/media-dedupe-and-quality-governance.md` | 继续保留 | 本轮未见它已完成归档所需的稳定结论提炼与退场动作 | 继续按实施方案维护 |
| `media-subscription/subscription-status-and-notification.md` | 继续保留 | 本轮未见它已完成归档所需的稳定结论提炼与退场动作 | 继续按实施方案维护 |

## D. 本轮新增归档记录

本轮已补充归档：

- `bot-polling-mode.md`
- `docs/archive/plan/billing-redemption/redemption-code-registration-plan-group.md`
- `billing-redemption/user-plan-grouping.md`
- `console-admin/admin-create-user-with-plan-group-expiry.md`
- `media-subscription/moviepilot-api-key-direct-integration.md`
- `media-subscription/tv-calendar-status-correction.md`
- `user-profile-analytics.md`
- `user-profile-overview.md`
- `dashboard-renewal-redesign.md`
- `playback-ranking-rework.md`
- `media-subscription/gap-management-and-precision-download.md`
- `embypulse-features/p1-user-profile.md`
- `embypulse-features/p2-user-avatar.md`
- `console-admin/ember-web-component-foundation.md`
- `embypulse-features/README.md`（索引目录退出，归档方案已按职责边界重组）

本轮已迁移为治理提案：

- `design-system-governance.md` → `docs/proposals/design-system-governance.md`

归档后，`docs/plan/` 当前剩余重点为：

- 仍在推进中的功能实施稿
- 功能方案模板：`plan-template.md`

## E. 归档前提炼原则

归档前不要机械移动文件，先做这件事：

1. 把当前仍有效的事实提炼进稳定文档
   - 架构与主流程：`docs/system-architecture.md`
   - 规范与配置边界：`docs/reference/`
   - 操作步骤：`docs/runbooks/`

2. 从方案文档里删掉不值得长期保留的噪音
   - 逐行代码草稿
   - 临时验证步骤
   - 已经过时的实现顺序说明

3. 只把仍有追溯价值的方案正文放进 `docs/archive/`

## 当前结论

`docs/plan/` 已经基本完成一轮收口，当前主要问题不再是“已落地方案没有退场”，而是：

1. 持续治理类文档的边界还不够清楚，后续应决定转入 `docs/proposals/`、`docs/reference/` 还是归档
2. 盘点清单本身也需要随归档动作同步更新，避免“正文已归档、索引仍显示未落地”的信息漂移
