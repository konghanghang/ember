# `docs/plan` 盘点清单

> 更新时间：2026-07-09

本清单只回答三件事：

1. 这份方案文档在代码里有没有落地证据
2. 它现在更适合继续留在 `docs/plan/`，还是进入归档队列
3. 如果要归档，前面还缺不缺稳定结论的提炼

盘点基准：

- 以当前代码结构和 [系统架构文档](../system-architecture.md) 为准
- “文档里写了已完成”不算证据，必须能在代码或架构文档里找到落点
- 本清单反映 2026-05-01 的盘点结果；若后续目录已调整，以仓库当前文件位置为准

## 0. 本轮 8 份主计划进度表

这里的“8 份主计划”指本轮批次 0→5 主干路线图实际使用的 8 份实施稿，不包含 `notification-mute-rules.md`、`registration-email-domain-allowlist.md`、`database-migration-baseline-and-archive.md`、`in-app-notification-center.md` 等旁支文档。

当前总览：

- 8 份主计划里，`8` 份已经有代码落地证据
- 其中 `8` 份已在本轮完成归档迁移：计划 1、计划 2、计划 3、计划 4、计划 5、计划 6、计划 7、计划 8
- 当前这 8 份主计划都已退出 `docs/plan/`，只保留历史追溯价值

| 编号 | 文档 | 状态标签 | 已落地证据 | 主要剩余项 | 建议动作 |
|------|------|----------|------------|------------|----------|
| 1 | `archive/plan/access-auth/auth-and-account-integrity-hardening.md` | 已归档 | 验证码发送限流并发收口、注册/重置密码验证码事务路径、注册回滚补偿、统一错误响应、`CheckExpiredUsers` cancel/失败上限、ConfigService `maskedValue` 语义、显式依赖构造入口均已落地 | 历史追溯 | 已迁入 `docs/archive/plan/access-auth/` |
| 2 | `archive/plan/billing-redemption/payment-redemption-integrity-hardening.md` | 已归档 | pending 支付幂等、Stripe webhook 去重、事务外 Emby 补偿、多币种口径、PlanGroup DTO 拆分均已落地 | 历史追溯 | 已迁入 `docs/archive/plan/billing-redemption/` |
| 3 | `archive/plan/media-subscription/subscription-state-machine-hardening.md` | 已归档 | 原子状态转移、`ingestProgress`、IGNORED 不复活、`redispatch`、`DISPATCH_FAILED`、`ignoreReasonCode`、前端闭环均已完成；稳定结论与退场条件已补齐 | 历史追溯 | 已迁入 `docs/archive/plan/media-subscription/` |
| 4 | `archive/plan/media-subscription/tv-calendar-and-tmdb-key-protection.md` | 已归档 | TMDB / MoviePilot / Stripe / SMTP 上游错误脱敏、`httpx.InternalError`、webhook `tmdbId` 命中精度、`tmdb_cache` GC、`resolveSeriesTMDBIDBySeriesID` 5 分钟缓存、同 key TMDB in-flight 去重、当前周纠偏落库已完成；稳定结论与退场条件已补齐 | 历史追溯 | 已迁入 `docs/archive/plan/media-subscription/` |
| 5 | `archive/plan/console-admin/playback-and-device-observation-hardening.md` | 已归档 | 排行榜幂等、single-flight、`LATEST_CACHE_PER_USER`、设备审计、结构化注销返回、ULID batchId、history / overview 回退链路收口均已完成；稳定结论与入口文档已同步 | 历史追溯 | 已迁入 `docs/archive/plan/console-admin/` |
| 6 | `archive/plan/console-admin/web-frontend-auth-and-design-baseline-fix.md` | 已归档 | 前端鉴权红线、Dashboard 真相收口、用户侧海报代理、关键请求竞态与双轨状态清理均已完成 | 历史追溯 | 已迁入 `docs/archive/plan/console-admin/` |
| 7 | `archive/plan/bot-telegram/bot-notification-and-info-leak-hardening.md` | 已归档 | `internal/async.SafeGo` fire-and-forget 收口、VerifyBind 反 DoS、错误模糊化、通知脱敏、runtime settings 保留旧值、pending reject 消息上下文持久化、Polling 单实例租约锁、BotNotifier 配置缓存、webhook 健康状态暴露均已完成；稳定结论已同步到架构文档 | 历史追溯 | 已迁入 `docs/archive/plan/bot-telegram/` |
| 8 | `archive/plan/architecture/schema-deployment-and-baseline-cleanup.md` | 已归档 | 启动路径移除 `AutoMigrate`、`VerifySchema` fail-fast、initdb 隔离、schema 对齐、airDate、连接池、容器非 root、固定部署镜像、空库初始化入口收口均已完成；归档前入口与交叉引用已同步 | 历史追溯 | 已迁入 `docs/archive/plan/architecture/` |

## A. 已落地，已完成归档

这些文档已经有明确代码落点，并已从 `docs/plan/` 退出。

| 文档 | 盘点结论 | 主要证据 | 建议动作 |
|------|----------|----------|----------|
| `active-sessions.md` | 已落地 | `SessionHandler`、`/api/v1/admin/sessions`、`views/admin/SessionsView.vue`、架构文档已收录 | 已提炼后归档到 `docs/archive/plan/console-admin/` |
| `email-verification.md` | 已落地 | `email_verifications` 模型、`SendEmailCode`、注册验证码路由、设置中心配置项 | 已提炼后归档到 `docs/archive/plan/access-auth/` |
| `forgot-password.md` | 已落地 | `SendResetCode`、`ResetPasswordByCode`、`ForgotPasswordView.vue`、Bot `/resetpw` | 已提炼后归档到 `docs/archive/plan/access-auth/` |
| `latest-media.md` | 已落地 | `/api/v1/media/latest`、`MediaService.GetLatestItems`、`RecentLibrarySection.vue` | 已提炼后归档到 `docs/archive/plan/media-subscription/` |
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
| `media-subscription/gap-management-and-precision-download.md` | 已落地 | `media_gaps` 模型与 migration、`MediaGapService`、`/api/v1/admin/media-gaps/grouped`、`MediaGapsView.vue`、MoviePilot 候选搜索与真实下发链路、架构文档已收录 | 已提炼后归档到 `docs/archive/plan/media-subscription/` |
| `media-subscription/subscription-status-and-notification.md` | 已落地 | `Subscription` 已支持 `INGESTED / rejectReason / reviewedAt / ingestedAt`，订阅服务已实现结果通知与 webhook 入库回写，用户端订阅页已展示状态与拒绝原因 | 已提炼后归档到 `docs/archive/plan/media-subscription/` |
| `media-subscription/library-entry-consolidation.md` | 已落地 | `DashboardView` 已接入 `RecentLibrarySection`，`/console/library` 已路由级重定向到 `/console/dashboard`，最近入库摘要由 `RecentLibrarySection` 承载（原 `LibraryView.vue` 兼容壳已删除） | 已提炼后归档到 `docs/archive/plan/media-subscription/` |
| `embypulse-features/p2-user-avatar.md` | 已落地 | `DefaultAvatar.vue`、Dashboard / Account Center / TopBar / Sidebar / UsersView 已统一接入默认头像组件 | 已归档到 `docs/archive/plan/console-admin/` |
| `console-admin/admin-create-user-with-plan-group-expiry.md` | 已落地 | `POST /api/v1/admin/users`、`CreateUserByAdmin`、`UsersView.vue` 新建用户弹窗、架构文档已收录后台创建用户接口 | 已提炼后归档到 `docs/archive/plan/console-admin/` |
| `console-admin/ember-web-component-foundation.md` | 已落地 | `services/web/src/components/ember/` 基础组件层、后台/控制台页头与 tabs 收口、表单基线统一、empty state 组件化、前端残留清理均已落地 | 已提炼后归档到 `docs/archive/plan/console-admin/` |
| `console-admin/console-admin-ui-consistency-optimization.md` | 已落地 | `SettingsView` 字段区已切回统一表单基线，筛选基础组件通用外观已收口到 `src/assets/base.css`，关键页面手工验收已通过 | 已提炼后归档到 `docs/archive/plan/console-admin/` |
| `console-admin/playback-center-merge.md` | 已落地 | 「播放分析」容器壳 `PlaybackCenterView.vue`、Sidebar 菜单合并为单条目、`/console/playback?tab=` 路由 + 4 条旧路径 redirect、单用户画像迁至 `/console/playback/users/:id`，架构文档「管理端播放分析」段落已收录 | 已归档到 `docs/archive/plan/console-admin/` |
| `archive/plan/access-auth/registration-email-domain-allowlist.md` | 已落地 | `registration_allowed_email_domains` 运行期配置已落地；`ConfigService.IsRegistrationEmailAllowed` / `GetRegistrationAllowedEmailDomains` 已暴露；`SendVerificationCode(register)` + `RegisterUser` 注册链路双重门控已生效；`SendEmailChangeCode` + `UpdateEmail` 账号中心换邮箱双重门控已生效；`GET /api/v1/register/mode` 已返回 `allowedEmailDomains`；注册页与账号中心已加提示与失焦预校验 | 已提炼后归档到 `docs/archive/plan/access-auth/` |
| `archive/plan/architecture/database-migration-auto-apply.md` | 已落地 | `services/api/internal/db/migrate.go` 启动期自动迁移、`schema_migrations` 记账、forward-only / backfill / checksum 测试、部署与数据库文档均已同步；相关提交已进入 `v1.5.0` / `v1.5.1`，日志已有 `[Migrate]` backfill / forward-only 成功记录 | 已提炼后归档到 `docs/archive/plan/architecture/` |
| `archive/plan/architecture/oss-deployment-experience.md` | 已落地 | Phase 1 + Phase 2 已进入 `v1.5.x`；README quickstart、Docker Compose 默认部署、Bot profile、GHCR 多架构构建、升级流程与 PG `initdb.d` 退役均已落地并完成验证 | 已提炼后归档到 `docs/archive/plan/architecture/` |
| `archive/plan/console-admin/admin-emby-binding.md` | 已落地 | `GET /api/v1/admin/emby-users`、`PUT/DELETE /api/v1/admin/current/emby-binding`、账号中心 Emby 用户选择器、API 端点目录和系统架构文档均已落地；提交 `362ff23` 已进入 `v1.5.1`，真实环境日志已有 list / bind / unbind 成功记录 | 已提炼后归档到 `docs/archive/plan/console-admin/` |
| `archive/plan/media-subscription/user-media-library-management.md` | 已落地 | `plan_group_media_libraries`、`plan_group_emby_policy_templates`、`user_media_library_preferences`、`emby_policy_sync_batches`、`emby_policy_sync_tasks` 与 `users.emby_access_disabled` 已落地；用户 Web / Telegram 媒体库偏好、分组模板、Emby Policy 同步 worker、API 目录、数据模型参考、系统架构和前端信息架构均已同步 | 已提炼后归档到 `docs/archive/plan/media-subscription/` |
| `archive/plan/console-admin/console-overview-account-layout-redesign.md` | 已落地 | `DashboardView.vue` 已按服务状态、Emby 入口、片库数字和最近入库重排；`AccountCenterView.vue` 已收口连接与绑定、媒体库偏好和账号设置布局；本次无 API / schema 变化 | 已归档到 `docs/archive/plan/console-admin/` |

## B. 当前仍留在 `docs/plan/` 的未完成文档

这些文档当前还没有退出 `docs/plan/`，并且按现有代码与稳定文档判断，仍未满足归档条件。

| 文档 | 盘点结论 | 主要原因 | 建议动作 |
|------|----------|----------|----------|
| `bot-telegram/notification-mute-rules.md` | 继续保留 | 本轮未见它已完成归档所需的稳定结论提炼与退场动作 | 继续按实施方案维护 |
| `access-auth/registration-user-capacity.md` | 继续保留 | 当前仍在 `docs/plan/`，本轮未见它已完成落地并收口到稳定文档 | 继续按实施方案维护 |
| `console-admin/device-risk-automation.md` | 继续保留 | 本轮未见它已完成归档所需的稳定结论提炼与退场动作 | 继续按实施方案维护 |
| `console-admin/in-app-notification-center.md` | 继续保留 | 本轮未见它已完成归档所需的稳定结论提炼与退场动作 | 继续按实施方案维护 |
| `media-subscription/media-dedupe-and-quality-governance.md` | 继续保留 | 本轮未见它已完成归档所需的稳定结论提炼与退场动作 | 继续按实施方案维护 |
| `media-subscription/subscription-manual-moviepilot-dispatch.md` | 继续保留 | 当前仍在 `docs/plan/`，本轮未见它已完成归档所需的稳定结论提炼与退场动作 | 继续按实施方案维护 |

## C. 本轮从 `docs/plan/` 迁入归档的文档

这些文档已经有明确代码或稳定文档落点，已按职责边界迁入对应 `docs/archive/plan/` 分类目录。

| 文档 | 盘点结论 | 主要证据 | 建议动作 |
|------|----------|----------|----------|
| `archive/plan/architecture/project-wide-log-level.md` | 已归档 | API/Gateway/Bot 共用 `LOG_LEVEL=info|debug`，Gin/GORM/Gateway/Bot 高噪声入口已分级，测试、race、vet/build、Bot pytest/py_compile 与 Compose 静态校验通过，稳定事实已同步 | 历史追溯 |
| `archive/plan/architecture/oss-deployment-experience.md` | 已归档 | README quickstart、Docker Compose 默认部署、Bot profile、GHCR 双架构构建、升级流程、`initdb/` 退役和部署 runbook 均已完成并验证 | 历史追溯 |
| `archive/plan/architecture/database-migration-auto-apply.md` | 已归档 | API 启动期自动迁移、SQL 镜像内置、五分支判断、checksum 防改写、部署升级文档与系统架构同步均已落地；相关提交已进入 `v1.5.0` / `v1.5.1` | 历史追溯 |
| `archive/plan/console-admin/admin-emby-binding.md` | 已归档 | 管理员 Emby 用户候选查询、按 `embyId` 绑定 / 解绑、前端选择器、测试、API 目录和系统架构同步均已落地；真实环境回归日志已覆盖成功路径 | 历史追溯 |
| `archive/plan/architecture/database-migration-baseline-and-archive.md` | 已归档 | 文档自身已标 `已完成`；现行迁移规则已由 `20260422_00_schema_baseline.sql`、`infrastructure/database/README.md`、`docs/system-architecture.md` 和 schema 部署基线收口方案接管 | 历史追溯 |
| `archive/plan/media-subscription/subscription-resubmission-after-rejection.md` | 已归档 | `subscriptions.retryFromId`、`20260424_01_subscription_resubmission_after_rejection.sql`、`uq_subscriptions_active_media`、`POST /subscriptions/:id/resubmit`、`ResubmitSubscriptionWithResult`、用户侧“再次提交”入口和架构文档均已落地 | 历史追溯 |
| `archive/plan/bot-telegram/subscription-admin-message-sync.md` | 已归档 | `subscription_admin_notifications` 模型与 migration、`telegram_approval_admin_ids`、Bot 多审批人员投递返回、API 投递记录持久化、Web / Telegram 审批后批量同步管理员消息均已落地，系统架构与配置/数据模型参考文档已同步 | 历史追溯 |
| `archive/plan/media-subscription/user-media-library-management.md` | 已归档 | 用户媒体库模板、用户偏好、Emby Policy 同步、Bot `/libraries` 和后台分组权益入口均已落地；稳定事实已同步到系统架构、API 目录、数据模型和前端信息架构 | 历史追溯 |
| `archive/plan/console-admin/console-overview-account-layout-redesign.md` | 已归档 | 控制台概览和账号中心布局已落地；本次仅为页面局部布局改造，无需新增稳定架构规则 | 历史追溯 |
| `archive/plan/access-auth/admin-api-key.md` | 已归档 | `AdminCredentialAuth()`、Admin API Key 生成 / 状态 / 禁用接口、设置中心管理区与认证边界均已落地；稳定结论已同步到系统架构、配置参考和 API 目录 | 历史追溯 |
| `archive/plan/architecture/settings-key-cache.md` | 已归档 | `settings` 按 key 的 TTL 缓存、negative cache、并发合并、写后失效与失效竞态修复均已落地；稳定结论已同步到系统架构 | 历史追溯 |
| `archive/plan/console-admin/plan-group-media-library-deferred-sync.md` | 已归档 | 分组模板 deferred / batch 保存、`out_of_sync` 状态语义、单用户“同步到 Emby”入口和 no-op 脏写收口均已落地；稳定结论已同步到系统架构 | 历史追溯 |
| `archive/plan/media-subscription/playback-ranking-library-allowlist.md` | 已归档 | 排行榜媒体库 allowlist 配置、候选扩窗 + 运行期归属缓存、latest/history/preview/通知统一口径与缓存污染修复均已落地 | 历史追溯 |
| `archive/plan/media-subscription/subscription-plan-group-auto-approval.md` | 已归档 | `PlanGroup` 自动通过额度、`review_source`、账号可提交校验、自动通过只读通知和后台来源展示均已落地；稳定结论已同步到架构与参考文档 | 历史追溯 |

## D. 本轮新增归档记录

本轮已补充归档：

- `architecture/project-wide-log-level.md`
- `architecture/oss-deployment-experience.md`
- `architecture/database-migration-auto-apply.md`
- `console-admin/admin-emby-binding.md`
- `access-auth/registration-email-domain-allowlist.md`
- `bot-polling-mode.md`
- `access-auth/auth-and-account-integrity-hardening.md`
- `billing-redemption/payment-redemption-integrity-hardening.md`
- `docs/archive/plan/billing-redemption/redemption-code-registration-plan-group.md`
- `billing-redemption/user-plan-grouping.md`
- `media-subscription/subscription-state-machine-hardening.md`
- `media-subscription/tv-calendar-and-tmdb-key-protection.md`
- `console-admin/web-frontend-auth-and-design-baseline-fix.md`
- `console-admin/admin-create-user-with-plan-group-expiry.md`
- `console-admin/console-admin-ui-consistency-optimization.md`
- `media-subscription/moviepilot-api-key-direct-integration.md`
- `media-subscription/tv-calendar-status-correction.md`
- `user-profile-analytics.md`
- `user-profile-overview.md`
- `dashboard-renewal-redesign.md`
- `playback-ranking-rework.md`
- `media-subscription/gap-management-and-precision-download.md`
- `media-subscription/library-entry-consolidation.md`
- `media-subscription/subscription-status-and-notification.md`
- `architecture/database-migration-baseline-and-archive.md`
- `media-subscription/subscription-resubmission-after-rejection.md`
- `embypulse-features/p1-user-profile.md`
- `embypulse-features/p2-user-avatar.md`
- `console-admin/ember-web-component-foundation.md`
- `archive/plan/architecture/schema-deployment-and-baseline-cleanup.md`
- `bot-telegram/subscription-admin-message-sync.md`
- `media-subscription/user-media-library-management.md`
- `console-admin/console-overview-account-layout-redesign.md`
- `access-auth/admin-api-key.md`
- `architecture/settings-key-cache.md`
- `console-admin/plan-group-media-library-deferred-sync.md`
- `media-subscription/playback-ranking-library-allowlist.md`
- `media-subscription/subscription-plan-group-auto-approval.md`
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
