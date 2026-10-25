# `docs/plan` 盘点清单

> 更新时间：2026-10-25（四份计划完成归档与待办交接，现行实施稿 10 份）

本清单只回答三件事：

1. 这份方案文档在代码里有没有落地证据
2. 它现在更适合继续留在 `docs/plan/`，还是进入归档队列
3. 如果要归档，前面还缺不缺稳定结论的提炼

盘点基准：

- 以当前代码结构和 [系统架构文档](../system-architecture.md) 为准
- “文档里写了已完成”不算证据，必须能在代码或架构文档里找到落点
- 历史各批次盘点日期保留在对应段落；后续每次新增或归档计划同步增补，不将历史验证扩大为本轮实证

## 0. 历史主干批次 8 份计划进度表

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

2026-09-17 归档：[支付与媒体状态问题修复计划](../archive/plan/media-subscription/open-issue-followup.md)，#21–#25 代码、回归、文档和逐项本地提交完成；API/Web 检查通过，真实 PostgreSQL 与外部验收未执行，尚未推送。

2026-09-22 增补：[115 新增转存起播许可](../archive/plan/architecture/p115-transfer-playback-intent.md) 代码与自动化验证已完成。仅对新建转存增加客户端明确起播许可，复用现有 proof、内容锁、租约清理和 Emby fallback；不新增 SQL，已有文件及十分钟跨会话缓存保持复用。全量非集成 Go、关键包 race、vet/build 通过；真实客户端验收尚未执行，继续保留该边界。

## A. 已落地，已完成归档

这些文档已经有明确代码落点，并已从 `docs/plan/` 退出。

2026-09-29 归档：[每日播放总结分步修复](../archive/plan/media-subscription/playback-summary-improvements.md)。五步本地实施与验证完成：前四步修复存储、发送反馈、统计口径和日期边界，第五步将已匹配候选从后续媒体库回查中移除。300 个候选、4 库的有匹配 fake 场景从 12 次降为 3～7 次，全部未匹配仍为 12 次；SQL 合并在日 / 周 / 月合成窗口收益不稳定，暂不实施。Go 非集成测试、关键 race、vet、build 和 SQLite 成本脚本通过；第一步 PostgreSQL、第二步 Bot、第四步 Web 验收只覆盖对应批次，真实 Emby / Telegram、浏览器与部署未执行。

2026-09-16 增补：[GitHub 开放问题零迁移修复计划](../archive/plan/architecture/github-issue-remediation.md) 已归档。#8/#10–#19 共 11 项完成本地代码修复或已有修复验收；用户明确接受 fake 验证，非集成 Go、关键包 race、Web 和 Bot 检查通过，未新增 migration。真实 PostgreSQL 不可达且未完成验证，推送/发布和旧 Bot pop 兼容清理保留独立后续边界。

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

## B. 当前 `docs/plan/` 状态复核

以下为分批历史记录，数量及未验收边界只描述当时版本；本轮 10 份现行实施稿以本节末表格为准。

2026-09-05 继续核对代码、测试和稳定文档；115 用户自有账号、套餐来源、Redis 播放租约与转存配额已完成阶段 0–3 代码、fake/race/全量验证和稳定文档同步。新增 PostgreSQL migration 已在专用 `EMBER_INTEGRATION_DATABASE_URL` 环境实际执行并通过；本机也已使用占位配置完成包含 `gateway`、`bot` profile 的 `docker compose config --quiet`，但这不替代目标部署环境的实际 `.env` 验证。未获真实外部验证授权，因此个人 Cookie/Redis/客户端链路仍未验收。Gateway 本地媒体直出已按产品边界撤销并归档，不再列为现行计划。

2026-09-17 文档复核：上述通过记录只覆盖对应历史批次；当时 9 月 12 日新增账号配置版本与成功记录优化的 PostgreSQL 用例尚未执行。2026-09-21 已补跑 DirectPlay PostgreSQL 专项并全部通过，详见 [网关总计划的验证记录](../plan/architecture/emby-115-direct-play-gateway.md)；真实 Redis/115/客户端验收仍未完成。2026-09-22 增补起播许可、2026-09-29 完成每日播放总结计划归档、2026-10-04 新增套餐与分组权益改版、分组观看保号计划后，当时 `docs/plan/` 保留 11 份。

2026-10-14 增补：计入 2026-10-08 的群签到计划和本次 [SenPlayer 原文件播放兼容](../archive/plan/architecture/senplayer-original-stream-compatibility.md)，当时共 13 份实施稿。SenPlayer 的版本化协议、MKV 抓包及受限适配实现已完成，用户选择保留预读不新增转存；Go 非集成全量、关键三包 race、vet/build 和抓包工具 17 项测试通过。实现与本地验证在 master 完成，尚未部署，不能把本轮 Emby 回退样本写成 115 加速验收。

2026-10-17 二轮修复 A–C（`080be97`、`da4d64a`、`d3f6fed`）已补齐文件/账号失败分类、停止释放、设备与缓存容量隔离、源阶段预算、取消观测及 tombstone 后限流桶清理。对应 Go 全量 vet/test/build、定向 race、fake/SQL mock 证据已同步到三份相关计划与稳定合同；本批没有新增真实 PostgreSQL/Redis/115/Emby/客户端验收，不改变计划数量、剩余验收项或归档条件。

2026-10-25 增补：[Gateway 客户端兼容与转存策略](../plan/architecture/gateway-client-compatibility.md) 的首轮三批次已在 `30fb9a4d` 完成代码、本地验证和独立审查，当时为 14 份实施稿。用户确认默认严格、管理员可开启全局兼容模式，并接受开启后预读可能转存；图片与网页开关解耦、会话路径大小写、活跃固定会话续播重新核验和兼容模式均已落地。首轮 Go 全量非集成测试、关键五包 race、vet/build、Web 10 项组件测试与构建、采集工具 19 项离线测试通过；随后获授权的认证 scheme、协议请求矩阵与无会话首请求补查增量也已完成实现、定向回归、全量非数据库 Go 测试、Gateway/DirectPlay/EmbyToken race、vet/build 和独立代码审查，结果见主计划第 7 节。尚未部署或执行真实客户端验收，不能把本地自动化结果扩大为线上播放结论；本次不归档。

同日后续优化以 `8799688` 为基线，完成采集工具的固定认证/路由/UA 过滤和离线 `diagnosticSummary`、媒体/直链固定 miss 原因及累计容量快照，以及独立 preID 成功哈希复用。两个用户和两个个人目标的顺序/重叠 fake，在任务、写入与配额工作量相同的条件下确认实际前缀读取从 2 次减至 1 次；全量非数据库 Go 测试、五包 race、vet/build、独立审查、原目录/独立目录 39 项 Python 离线测试、10 个源文件编译/AST 和 Go 日志→Python 摘要串接复验均完成，结果见主计划第 8 节。未部署或请求真实 Emby/115/数据库，真实缓存命中率、准备耗时、首帧和客户端收益未测；不增加 Web UI、配置或 migration，计划数不变且不归档。

同日以 `3e1cd42` 为基线补齐持续自动化：采集 Python 3.9/3.11 矩阵、五包 race、Go→Python 实际日志合同分别接入 CI，统一测试入口和失败/跳过状态汇总同步完成。三个 runner 本地通过：双 Python 各 39 项采集加 20 项入口/校验器测试、五包 race、31 个字段和 4 类 Gateway 日志合同，以及相关 vet/配置检查与独立审查，详见主计划第 9 节。用户明确按单 Gateway 维护；远端 CI 尚未运行，真实客户端及性能验收仍待完成，计划数和归档条件不变。

2026-10-25 本轮收尾：四份计划已转入 C 节归档记录，现行实施稿从 14 份收敛为 10 份。个人 115/Redis 按正文既定条件以“实施完成、未授权真实 E2E 明确保留未验证”归档；索引不再额外要求所有外部验收先通过。起播许可与 SenPlayer 的剩余取证由新主计划逐项接替，套餐业务/运维项由升级手册承接；归档不代表豁免或通过这些验收。

| 文档 | 盘点结论 | 主要证据或剩余项 | 建议动作 |
|------|----------|------------------|----------|
| [自助注册人数上限](../plan/access-auth/registration-user-capacity.md) | 待开发 | 当前未发现容量配置、人数统计、满员门控和公开容量字段 | 继续保留实施计划 |
| [分组观看保号](../plan/billing-redemption/watch-activity-renewal.md) | 实现完成，待分项验收 | 专用 PostgreSQL 与管理员页面已有记录；10/18 保留起点修复已落地，10/23 历史镜像包含该修复；用户端页面、真实考核与权限同步未验收 | 保留验收计划，不重复要求已完成的管理员页面验收 |
| [Emby–115 DirectPlay 总计划](../plan/architecture/emby-115-direct-play-gateway.md) | 播放数据面主体完成，运维待实施 | 任务查询/重试、主动健康告警、文件容量清理和部分 CDN 取证未完成；目录与 Redis 已落地 | 按单 Gateway 保留当前剩余项，不把多实例列为归档门槛 |
| [运行期配置缓存演进](../plan/architecture/runtime-settings-cache-evolution.md) | 观察期 | 未取得替换启动条件实证，未提升最低 Go 语言版本；5s 专用策略已有修补不等于通用刷新演进完成 | 无实证不替换，继续观察 |
| [Telegram 通知静音规则](../plan/bot-telegram/notification-mute-rules.md) | 待开发 | 已有订阅结果通知，但没有事件规则、用户偏好和统一静音入口 | 继续保留实施计划 |
| [群签到与双途径保号](../plan/bot-telegram/group-checkin-retention.md) | 规则已确认，待实施 | 已同步10/18起点边界；签到、首次日期持久化、迁移及双条件考核仍未实现 | 保留实施计划，不能把前置修复算成签到完成 |
| [Gateway 客户端兼容与转存策略](../plan/architecture/gateway-client-compatibility.md) | 本地实施完成，待远端 CI 与真实验收 | 30fb9a4/8799688/3e1cd42/33eb11f 已提交并完成各自本地验证；新增镜像部署、客户端、命中率与准备耗时待验证 | 承接旧专项取证，按单 Gateway 完成授权范围内验收 |
| [设备风控自动化](../plan/console-admin/device-risk-automation.md) | 待开发 | 已有人工设备处置，未发现风险记录、自动判定和告警闭环 | 继续保留实施计划 |
| [站内通知中心](../plan/console-admin/in-app-notification-center.md) | 待开发 | 已有 Telegram 消息引用表不等于站内通知，未读/已读 API 与页面未实现 | 继续保留实施计划 |
| [媒体去重与质量治理](../plan/media-subscription/media-dedupe-and-quality-governance.md) | 待开发 | 已有质量报告，未发现重复版本组、评分、保留建议和忽略能力 | 继续保留实施计划 |

历史快照（2026-10-04）：当时新增分组观看保号后为 11 份实施稿，观看保号当时仅完成规则记录；当前实现与验收状态以本节表格为准，不沿用该历史判断。

## C. 已完成的归档记录

这些文档已经有明确代码或稳定文档落点，已按职责边界迁入对应 `docs/archive/plan/` 分类目录。

| 文档 | 盘点结论 | 主要证据 | 建议动作 |
|------|----------|----------|----------|
| [套餐与分组权益改版](../archive/plan/billing-redemption/plan-group-entitlements.md) | 2026-10-25 实施收尾归档 | c20080cc/2c320f36/065f1b3a 已提交，历史数据库/页面/Stripe 沙盒验证有记录；10/23 镜像包含这些改动 | 业务验收、种子用户迁组状态和外部边界由[升级手册](../runbooks/entitlements-upgrade.md)承接 |
| [115 个人账号与 Redis 配额](../archive/plan/architecture/p115-personal-account-routing-and-redis-quotas.md) | 2026-10-25 实施收尾归档 | 四阶段代码、基础及9/21后续 PostgreSQL 专项已完成；真实个人 Cookie/Redis/配额尚未验收 | 现行实现及逐项真实验收清单由[端到端参考](../reference/p115-playback-end-to-end-flow.md)承接 |
| [115 新增转存起播许可](../archive/plan/architecture/p115-transfer-playback-intent.md) | 2026-10-25 被新主计划接替 | 严格许可已实现；兼容模式不伪造许可；首次/停止/重播取证和 thumb 假设仍未完成 | 由[客户端兼容主计划](../plan/architecture/gateway-client-compatibility.md)接替，不写成实机验收通过 |
| [SenPlayer 原文件播放兼容](../archive/plan/architecture/senplayer-original-stream-compatibility.md) | 2026-10-25 被新主计划接替 | 4c973080 已实现并包含于10/23历史部署镜像；真实302/CDN Range/字幕与播放分项仍缺证据 | 由[客户端兼容主计划](../plan/architecture/gateway-client-compatibility.md)逐项跟踪 |
| `archive/plan/architecture/gateway-media-path-diagnostics.md` | 已归档 | `5189ff3` 已进入 v2.0.3，新中文决策、完整 `mediaPath`、首次/复用 302、映射字段与 `path_not_mapped` 均有部署日志；生产未故障注入 Provider/账号失败，保留 fake 证据边界；`Items/Latest` 误分类转由 [Issue #8](https://github.com/konghanghang/ember/issues/8) 跟踪 | 历史追溯；稳定日志合同由系统架构、Emby 代理合同和 115 端到端参考接管 |
| `archive/plan/architecture/ember-gateway-transparent-proxy-and-web-access.md` | 已归档 | v2.0.3 已完成 root/`/emby` bootstrap、Infuse 登录/资源/字幕、本地 fallback `206`、首次/复用 `302`、Playing/Progress/Stopped，以及 Web 开启/关闭、完整资源、query 登录、Primary/Backdrop 和 WebSocket 验收；115 Web 播放按用户无使用场景排除，原始 Emby 隔离按部署管理员确认收口 | 历史追溯；稳定合同由系统架构、Emby 代理合同、客户端矩阵与部署 runbook 接管 |
| `archive/plan/architecture/web-frontend-quality-improvement.md` | 已归档 | MediaStats camelCase、TMDB `{data,total}`、Web/Bot 调用方、注册跨 tab 会话替换和设备页全量刷新均有自动化测试；API/Web/Bot 全量验证与 Web build 通过；真实浏览器验收未执行，按用户决定不再阻塞归档 | 历史追溯，不表述为 E2E 或人工验收通过 |
| `archive/plan/architecture/p115-path-resolution-without-emby-size.md` | 已归档 | `5189ff3` 已落地并进入 v2.0.3；Size=0 条目已有 proof、路径映射、Provider Size、首次与复用 302 部署证据，稳定合同已提炼 | 历史追溯 |
| `archive/plan/console-admin/web-layout-design-improvement.md` | 已归档 | `9234559` 完成四批代码收口，`0082121` 同步设计规范和计划；计划内验证结果与组件测试已记录 | 历史追溯 |
| `archive/plan/media-subscription/subscription-manual-moviepilot-dispatch.md` | 已归档 | `5df0244` 已落地 Service/Handler/API/Web，稳定事实已进入系统架构和 API 目录；2026-08-30 当前目标 Go 测试与 8 个 Web 用例通过 | 历史追溯 |
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

## D. 历史归档记录

历史批次已补充归档：

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
- `architecture/p115-path-resolution-without-emby-size.md`
- `console-admin/web-layout-design-improvement.md`
- `media-subscription/subscription-manual-moviepilot-dispatch.md`
- `embypulse-features/README.md`（索引目录退出，归档方案已按职责边界重组）

历史批次已迁移为治理提案：

- `design-system-governance.md` → `docs/proposals/design-system-governance.md`

归档后，`docs/plan/` 当前剩余重点为：

- 仍在推进中的功能实施稿
- 功能方案模板：`plan-template.md`

2026-08-30 新一轮复核与迁移已完成：3 份计划退出 `docs/plan/`，对应状态、索引和直接引用均已同步。

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

截至 2026-10-25，`docs/plan/` 共 10 份现行实施稿：3 份推进实现或验收、6 份待开发、1 份观察期；逐项状态见 B 节，四份本轮归档见 C 节。实施归档不删除未完成验收，所有待办已有现行承接位置。

10 月 23 日历史部署记录确认 hz 四个应用容器的镜像 revision 为 `b1f524f5a81b5507d505b272b96355c82ce7da38`；本轮本地 Git 祖先核对确认其包含套餐迁组、观看保号起点修复与 SenPlayer MKV 适配。这是历史部署事实，不是本轮线上版本或业务验收；10 月 25 日新增 Gateway 兼容、优化与 CI 仍只有本地证据。

当前剩余事项：

1. Gateway 兼容主计划承接旧起播/SenPlayer取证，新增 CI 的远端执行、最新部署和真实客户端/性能证据分别记录，不能用 fake、Tag 或 302 代替完整播放。
2. 个人 Cookie、单 Gateway Redis、配额及故障回退按端到端参考清单等待授权验证；套餐业务验收与种子用户操作状态按升级手册跟踪。
3. 分组观看保号保留用户端页面、真实考核与远端权限同步验收；群签到及另五份功能计划仍待开发，通用配置缓存维持观察期。
4. DirectPlay 总计划只保留当前单 Gateway 的运维与合同取证欠账；多实例能力不作为本次推进或归档门槛。
5. 后续状态变动同时同步计划、归档、提案索引和直接引用；历史测试、数据库与部署结果始终限定到对应日期和版本。
