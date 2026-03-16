# `docs/plan` 盘点清单

本清单只回答三件事：

1. 这份方案文档在代码里有没有落地证据
2. 它现在更适合继续留在 `docs/plan/`，还是进入归档队列
3. 如果要归档，前面还缺不缺稳定结论的提炼

盘点基准：

- 以当前代码结构和 [系统架构文档](../system-architecture.md) 为准
- “文档里写了已完成”不算证据，必须能在代码或架构文档里找到落点
- 本轮只盘点，不迁移文件

## A. 已落地，建议进入归档队列

这些文档已经有明确代码落点，继续长期挂在 `docs/plan/` 的价值不高。

| 文档 | 盘点结论 | 主要证据 | 建议动作 |
|------|----------|----------|----------|
| `active-sessions.md` | 已落地 | `SessionHandler`、`/api/v1/admin/sessions`、`views/admin/SessionsView.vue`、架构文档已收录 | 先提炼稳定接口说明，再归档 |
| `email-verification.md` | 已落地 | `email_verifications` 模型、`SendEmailCode`、注册验证码路由、设置中心配置项 | 先补稳定配置/流程说明，再归档 |
| `forgot-password.md` | 已落地 | `SendResetCode`、`ResetPasswordByCode`、`ForgotPasswordView.vue`、Bot `/resetpw` | 先提炼用户可见行为，再归档 |
| `latest-media.md` | 已落地 | `/api/v1/media/latest`、`MediaService.GetLatestItems`、`LibraryView.vue` | 先补稳定页面/API 说明，再归档 |
| `playback-ranking.md` | 已落地 | `RankingHandler`、`PlaybackRankingService`、`RankingsView.vue`、排行 cron | 先提炼规则与依赖，再归档 |
| `redemption-code-batch-create.md` | 已落地 | `CreateRedemptionCodesBatch`、批量接口、管理端批量创建 UI | 已归档到 `docs/archive/plan/` |
| `redemption-code-one-per-user.md` | 已落地 | 一人一码约束、兑换历史接口、用户端/管理端兑换历史 UI | 已归档到 `docs/archive/plan/` |
| `settings-center.md` | 已落地 | 文档自身已写“已完成”，代码中有 `config/`、`handlers/config.go`、`SettingsView.vue` | 先提炼设置中心边界，再归档 |
| `stripe-payment.md` | 已落地 | `PaymentService`、`Plan`/`Payment` 模型、`PaymentsView.vue`、`PlansView.vue` | 先补支付边界与回调说明，再归档 |
| `telegram-binding.md` | 已落地 | `GenerateBindCode`、`VerifyBind`、Bot `/bind` `/info` `/redeem`、Dashboard 绑定入口 | 先补稳定 Bot 能力说明，再归档 |
| `telegram-bot-menu.md` | 已落地 | `menu_sync.py`、`/refresh_menu`、Bot 启动菜单同步 | 已归档到 `docs/archive/plan/` |
| `telegram-search-subscribe.md` | 已落地 | `SubscribeByTelegram`、Bot `/search`、搜索会话缓存、内部订阅接口 | 先提炼 Bot 搜索/订阅行为，再归档 |
| `telegram-search-multi-type.md` | 已落地 | TMDB `type=multi`、Bot `_do_search(..., "multi")` | 可直接归档，作为前者的补充归档项 |
| `telegram-subscription-notification.md` | 已落地 | `BotNotifier`、`/notify/subscription`、Bot 审批回调、InternalAuth | 先补通知链路说明，再归档 |
| `unified-console.md` | 已落地 | `/console/*` 路由、旧 `/admin/*` `/user/*` 重定向、统一 `Layout.vue` | 已归档到 `docs/archive/plan/` |
| `welcome-message.md` | 已落地 | `handle_new_member`、`notify_group_link`、运行期设置读取 | 已归档到 `docs/archive/plan/` |
| `embypulse-features/p0-device-management.md` | 已落地 | 设备接口、`DeviceService`、`DevicesView.vue` | 已归档到 `docs/archive/plan/embypulse-features/` |
| `embypulse-features/p0-permission-template.md` | 已落地 | `templateUserId` 字段、模板用户列表、兑换码管理 UI | 已归档到 `docs/archive/plan/embypulse-features/` |
| `embypulse-features/p0-tv-calendar.md` | 已落地 | `TVCalendarService`、全局/关注周历、同步与 webhook 就绪标记、`TVCalendarView.vue` | 先提炼最终行为，再归档 |
| `embypulse-features/p1-media-quality.md` | 已落地 | 媒体库列表、质量报告、明细接口、`MediaQualityView.vue` | 已归档到 `docs/archive/plan/embypulse-features/` |
| `embypulse-features/p1-playback-history.md` | 已落地 | `PlaybackHistoryService`、管理端播放历史路由与页面 | 已归档到 `docs/archive/plan/embypulse-features/` |

## B. 已有明显产出，但更像持续治理，不急着归档

| 文档 | 盘点结论 | 原因 | 建议动作 |
|------|----------|------|----------|
| `design-system-governance.md` | 持续治理中 | 它对应的是长期的设计系统收口，不是一次性交付功能 | 暂留 `docs/plan/`，等规范真正稳定后转 `reference/` 或归档 |
| `embypulse-features/README.md` | 索引文档 | 它是该子目录的规划入口，不是单一功能方案 | 保留，等子目录清理完再处理 |

## C. 未见落地证据，继续保留在 `docs/plan/`

这些文档的目标在当前代码里还没有形成完整落点，继续保留为计划文档是合理的。

| 文档 | 盘点结论 | 主要原因 | 建议动作 |
|------|----------|----------|----------|
| `embypulse-features/p1-user-profile.md` | 未落地 | 未见对应 `UserProfileService`、路由或页面 | 继续保留 |
| `embypulse-features/p2-code-notes.md` | 未落地 | `redemption_codes` 当前无 `notes` 字段与配套 UI | 继续保留 |
| `embypulse-features/p2-library-list.md` | 未完整落地 | 当前只有媒体质量场景复用的媒体库列表，没有独立通用 `libraries` 能力入口 | 继续保留 |
| `embypulse-features/p2-subscription-season.md` | 未落地 | `Subscription` 模型当前没有 `season` 字段 | 继续保留 |
| `embypulse-features/p2-user-avatar.md` | 未落地 | 未见头像上传/同步接口与页面 | 继续保留 |

## D. 下一轮归档建议顺序

### 第一批：已执行归档

- `redemption-code-batch-create.md`
- `redemption-code-one-per-user.md`
- `telegram-bot-menu.md`
- `unified-console.md`
- `welcome-message.md`
- `embypulse-features/p0-device-management.md`
- `embypulse-features/p0-permission-template.md`
- `embypulse-features/p1-media-quality.md`
- `embypulse-features/p1-playback-history.md`

### 第二批：先提炼稳定结论，再归档

- `active-sessions.md`
- `email-verification.md`
- `forgot-password.md`
- `latest-media.md`
- `playback-ranking.md`
- `settings-center.md`
- `stripe-payment.md`
- `telegram-binding.md`
- `telegram-search-subscribe.md`
- `telegram-search-multi-type.md`
- `telegram-subscription-notification.md`
- `embypulse-features/p0-tv-calendar.md`

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

`docs/plan/` 现在的主要问题不是“文档太多”，而是“已落地方案没有退场”。  
下一步最合理的动作，不是继续写新计划，而是按上面的第一批、第二批顺序分两轮归档。
