# `docs/plan` 盘点清单

> 更新时间：2026-03-29

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
| `active-sessions.md` | 已落地 | `SessionHandler`、`/api/v1/admin/sessions`、`views/admin/SessionsView.vue`、架构文档已收录 | 已提炼后归档到 `docs/archive/plan/` |
| `email-verification.md` | 已落地 | `email_verifications` 模型、`SendEmailCode`、注册验证码路由、设置中心配置项 | 已提炼后归档到 `docs/archive/plan/` |
| `forgot-password.md` | 已落地 | `SendResetCode`、`ResetPasswordByCode`、`ForgotPasswordView.vue`、Bot `/resetpw` | 已提炼后归档到 `docs/archive/plan/` |
| `latest-media.md` | 已落地 | `/api/v1/media/latest`、`MediaService.GetLatestItems`、`LibraryView.vue` | 已提炼后归档到 `docs/archive/plan/` |
| `playback-ranking.md` | 已落地 | `RankingHandler`、`PlaybackRankingService`、`RankingsView.vue`、排行 cron | 已提炼后归档到 `docs/archive/plan/` |
| `redemption-code-batch-create.md` | 已落地 | `CreateRedemptionCodesBatch`、批量接口、管理端批量创建 UI | 已归档到 `docs/archive/plan/` |
| `redemption-code-one-per-user.md` | 已落地 | 一人一码约束、兑换历史接口、用户端/管理端兑换历史 UI | 已归档到 `docs/archive/plan/` |
| `settings-center.md` | 已落地 | 文档自身已写“已完成”，代码中有 `config/`、`handlers/config.go`、`SettingsView.vue` | 已提炼后归档到 `docs/archive/plan/` |
| `stripe-payment.md` | 已落地 | `PaymentService`、`Plan`/`Payment` 模型、`PaymentsView.vue`、`PlansView.vue` | 已提炼后归档到 `docs/archive/plan/` |
| `telegram-binding.md` | 已落地 | `GenerateBindCode`、`VerifyBind`、Bot `/bind` `/info` `/redeem`、Dashboard 绑定入口 | 已提炼后归档到 `docs/archive/plan/` |
| `telegram-bot-menu.md` | 已落地 | `menu_sync.py`、`/refresh_menu`、Bot 启动菜单同步 | 已归档到 `docs/archive/plan/` |
| `bot-polling-mode.md` | 已落地 | `TELEGRAM_UPDATE_MODE`、Bot `webhook/polling` 双模式启动、`docker-compose.yml` 透传、配置中心条件风险提示 | 已提炼后归档到 `docs/archive/plan/` |
| `telegram-search-subscribe.md` | 已落地 | `SubscribeByTelegram`、Bot `/search`、搜索会话缓存、内部订阅接口 | 已提炼后归档到 `docs/archive/plan/` |
| `telegram-search-multi-type.md` | 已落地 | TMDB `type=multi`、Bot `_do_search(..., "multi")` | 已归档到 `docs/archive/plan/` |
| `telegram-subscription-notification.md` | 已落地 | `BotNotifier`、`/notify/subscription`、Bot 审批回调、InternalAuth | 已提炼后归档到 `docs/archive/plan/` |
| `unified-console.md` | 已落地 | `/console/*` 路由、旧 `/admin/*` `/user/*` 重定向、统一 `Layout.vue` | 已归档到 `docs/archive/plan/` |
| `welcome-message.md` | 已落地 | `handle_new_member`、`notify_group_link`、运行期设置读取 | 已归档到 `docs/archive/plan/` |
| `embypulse-features/p0-device-management.md` | 已落地 | 设备接口、`DeviceService`、`DevicesView.vue` | 已归档到 `docs/archive/plan/embypulse-features/` |
| `embypulse-features/p0-permission-template.md` | 已落地 | `templateUserId` 字段、模板用户列表、兑换码管理 UI | 已归档到 `docs/archive/plan/embypulse-features/` |
| `embypulse-features/p0-tv-calendar.md` | 已落地 | `TVCalendarService`、全局/关注周历、同步与 webhook 就绪标记、`TVCalendarView.vue` | 已提炼后归档到 `docs/archive/plan/embypulse-features/` |
| `embypulse-features/p1-media-quality.md` | 已落地 | 媒体库列表、质量报告、明细接口、`MediaQualityView.vue` | 已归档到 `docs/archive/plan/embypulse-features/` |
| `embypulse-features/p1-playback-history.md` | 已落地 | `PlaybackHistoryService`、管理端播放历史路由与页面 | 已归档到 `docs/archive/plan/embypulse-features/` |

## B. 已有明显产出，但更像持续治理，不急着归档

| 文档 | 盘点结论 | 原因 | 建议动作 |
|------|----------|------|----------|
| `embypulse-features/README.md` | 索引文档 | 它是该子目录的规划入口，不是单一功能方案 | 保留，并随子目录状态及时同步 |

## C. 未见完整落地证据，继续保留在 `docs/plan/`

这些文档的目标在当前代码里还没有形成完整落点，继续保留为计划文档是合理的。

| 文档 | 盘点结论 | 主要原因 | 建议动作 |
|------|----------|----------|----------|
| `embypulse-features/p2-subscription-season.md` | 未落地 | `Subscription` 模型当前没有 `season` 字段 | 继续保留 |
| `embypulse-features/p2-user-avatar.md` | 未落地 | 未见头像上传/同步接口与页面 | 继续保留 |

## D. 本轮新增归档记录

本轮已补充归档：

- `bot-polling-mode.md`
- `user-profile-analytics.md`
- `user-profile-overview.md`
- `dashboard-renewal-redesign.md`
- `playback-ranking-rework.md`
- `embypulse-features/p1-user-profile.md`

本轮已迁移为治理提案：

- `design-system-governance.md` → `docs/proposals/design-system-governance.md`

归档后，`docs/plan/` 当前剩余重点为：

- 规划索引：`embypulse-features/README.md`
- 尚未落地的 P2 条目：`p2-subscription-season.md`、`p2-user-avatar.md`

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
2. `embypulse-features/README.md` 这类索引文档必须跟随子项状态同步，不能再次过时
3. 剩余 P2 计划应继续保留为未落地方案，不要过早归档
