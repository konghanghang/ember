# Ember 系统架构文档

> 本文档记录 Ember 系统的高层架构、服务边界、关键流转和参考文档入口。
> 供 AI 协作时快速加载系统上下文，避免重复探索代码。

## 文档职责说明

本文件是 Ember 的**系统入口文档**，优先承载以下内容：

- 系统边界、服务职责与核心目录结构
- 核心数据关系、关键状态流转与外部集成关系
- 进入更细粒度参考文档的导航入口

为避免继续膨胀，以下内容不再把“完整清单”长期保留在本文件中：

- 全量配置与环境变量字典：看 [docs/reference/configuration-reference.md](./reference/configuration-reference.md)
- 详细部署与排障步骤：看 [docs/runbooks/deployment.md](./runbooks/deployment.md)、[docs/runbooks/deployment-environment.md](./runbooks/deployment-environment.md)、[docs/runbooks/deployment-troubleshooting.md](./runbooks/deployment-troubleshooting.md)

当前主文档中的大块枚举型内容已迁移到 `docs/reference/`；后续以目录入口收尾、引用校正和归档准备为主。

---

## 1. 系统概览

Ember 是一个 Emby 媒体服务器的用户管理系统，提供：
- 用户注册/登录（可配置开放注册或兑换码注册，支持邮箱验证码）
- Emby 账号自动创建与生命周期管理（试用 → 续期 → 过期封禁）
- 兑换码系统（注册门控 + 续期工具，统一模型；同一用户同一码仅可兑换一次）
- 付费方案与 Stripe 一次性支付
- 求片订阅（TMDB 搜索 → 管理员审批 → MoviePilot 自动下载）
- 播放排行榜（日榜 / 周榜，从 Emby PlaybackActivity 生成）
- Telegram Bot（订阅审批、新用户通知、排行榜推送、欢迎消息、账号绑定/查询/续期、媒体库统计、媒体库显示偏好）
- 定时任务（过期检查、验证码清理、排行榜生成、Emby Policy 同步）

## 2. 技术栈

| 层 | 技术 |
|----|------|
| 后端 | Go 1.23 + Gin + GORM |
| 数据库 | PostgreSQL 15 |
| 前端 | Vue 3 + TypeScript + Element Plus + Tailwind CSS |
| Bot | Python 3.11 + python-telegram-bot + FastAPI |
| 支付 | Stripe（一次性支付，Checkout Session 模式）|
| 外部集成 | Emby API, TMDB API, MoviePilot API, Stripe API |
| 定时任务 | robfig/cron/v3 |
| 部署 | Docker + Docker Compose |

数据库 schema 约定补充：
- 表名、列名、索引名统一使用 `snake_case`
- Go / GORM 字段继续使用 `CamelCase`，通过显式 `gorm:"column:..."` 映射到数据库列
- API / 前端 JSON 字段继续使用 `camelCase`

## 3. 目录结构

```
services/
├─ api/                          # Go 后端
│  ├─ cmd/server/main.go         # 入口：路由注册 + cron 初始化
│  ├─ cmd/p115-contract-check/   # 显式授权后运行的一次性真实 115 只读合同检查器
│  └─ internal/
│     ├─ config/                 # 运行期配置定义 / 解析 / 校验
│     │  ├─ config.go            # ConfigService（配置定义/解析/测试/导入）
│     │  ├─ setting.go           # Stripe 支付方式规范化
│     │  └─ errors.go            # 配置层错误定义
│     ├─ models/                 # 数据模型（GORM）
│     │  ├─ user.go              # User（核心用户模型）
│     │  ├─ redemption_code.go   # RedemptionCode（统一兑换码）
│     │  ├─ redemption.go        # Redemption（兑换历史）
│     │  ├─ setting.go           # Setting（系统配置 KV）
│     │  ├─ subscription.go      # Subscription（求片订阅）
│     │  ├─ email_verification.go # EmailVerification（邮箱验证码）
│     │  ├─ telegram_bind_code.go # TelegramBindCode（Telegram 绑定验证码）
│     │  ├─ plan.go              # Plan（付费方案）
│     │  ├─ plan_group.go        # PlanGroup（套餐分组）
│     │  ├─ payment.go           # Payment（支付记录）
│     │  ├─ playback_ranking.go  # PlaybackRanking（播放排行快照）
│     │  ├─ media_quality_cache.go # MediaQualityCache（媒体质量缓存）
│     │  ├─ client_blacklist.go  # ClientBlacklist（客户端黑名单）
│     │  ├─ device_action.go     # DeviceAction（设备操作日志）
│     │  ├─ media_gap.go         # MediaGap（缺集工单）
│     │  ├─ p115_account.go      # P115Account（管理员 115 源账号 / 播放账号加密凭证）
│     │  ├─ tv_calendar.go       # TVCalendar（追剧日历 + 订阅 + TMDB 缓存）
│     │  └─ utils.go             # generateCUID()
│     ├─ integrations/           # 外部系统集成
│     │  ├─ emby/
│     │  │  ├─ emby.go           # Emby HTTP 客户端
│     │  │  └─ library.go        # Emby 媒体库列表/条目查询
│     │  ├─ moviepilot/
│     │  │  └─ client.go         # MoviePilot HTTP 客户端
│     │  ├─ p115/
│     │  │  ├─ provider.go       # 115 Cookie / OpenAPI 共用 Provider 业务合同
│     │  │  ├─ cookie_provider.go # 完整 Cookie Provider 组合与编译期接口保护
│     │  │  ├─ cookie_validator.go # Cookie 登录状态只读验证
│     │  │  ├─ cookie_http_adapter.go # 源解析、查重、秒传、Range、直链与删除 Adapter
│     │  │  └─ p115cipher/       # Cookie 上传 AES/LZ4 与下载 RSA 固定向量协议层
│     │  └─ notifier/
│     │     └─ notifier.go       # BotNotifier（火忘式推送通知给 Bot）
│     ├─ services/               # 业务逻辑
│     │  ├─ accessauth/
│     │  │  └─ admin_api_key.go  # 全局 Admin API Key 生成、hash、禁用与校验
│     │  ├─ auth/
│     │  │  ├─ service.go        # AuthService（共享装配 / 模板权限应用）
│     │  │  ├─ login.go          # AuthService（登录链路编排）
│     │  │  ├─ register.go       # AuthService（注册链路编排）
│     │  │  ├─ register_persist.go # AuthService（注册落库事务）
│     │  │  ├─ register_notify.go # AuthService（注册通知副作用）
│     │  │  └─ emby_binding.go   # 管理员 Emby 账号自助绑定 / 解绑链路
│     │  ├─ user/
│     │  │  ├─ service.go        # UserService（共享依赖 / Emby 同步）
│     │  │  ├─ admin.go          # 用户管理 / 后台创建 / 续期 / 启停 / 删除
│     │  │  ├─ profile.go        # 用户资料 / 邮箱更新
│     │  │  ├─ password.go       # 用户密码修改 / 管理员重置密码
│     │  │  └─ password_reset.go # 用户邮箱验证码重置密码
│     │  ├─ system/
│     │  │  ├─ service.go        # SystemService（系统信息 / Emby 探活）
│     │  │  └─ expiry.go         # SystemService（过期用户检查）
│     │  ├─ media.go             # 媒体统计（带 5min 缓存）
│     │  ├─ media_quality.go     # MediaQualityService（媒体质量盘点）
│     │  ├─ mediagap/
│     │  │  ├─ service.go        # MediaGapService（缺集扫描 / 搜索候选 / 下发 / webhook 核销）
│     │  │  ├─ types.go          # 缺集领域请求/响应与快照结构
│     │  │  └─ errors.go         # 缺集领域错误
│     │  ├─ subscription.go      # 订阅工作流
│     │  ├─ email/
│     │  │  ├─ service.go        # EmailService（配置读取 / 开关判断）
│     │  │  ├─ verification.go   # EmailService（邮箱验证码发送 / 校验 / 清理）
│     │  │  └─ sender.go         # EmailService（SMTP 发送 / 连接测试）
│     │  ├─ telegram/
│     │  │  ├─ service.go        # TelegramService（绑定/查询/续期）
│     │  │  ├─ wiring.go         # Telegram 默认依赖装配（redemption/subscription/emby）
│     │  │  └─ errors.go         # Telegram 领域错误
│     │  ├─ redemption/
│     │  │  ├─ service.go        # RedemptionService（兑换核心逻辑 + 历史）
│     │  │  ├─ code_service.go   # RedemptionCodeService（兑换码 CRUD）
│     │  │  ├─ errors.go         # 兑换领域错误
│     │  │  └─ types.go          # 兑换领域请求/响应结构
│     │  ├─ playback/
│     │  │  ├─ history.go        # PlaybackHistoryService（播放历史查询）
│     │  │  ├─ profile.go        # UserPlaybackProfileService（用户播放画像）
│     │  │  ├─ profile_list.go   # 用户画像总览聚合
│     │  │  └─ ranking.go        # PlaybackRankingService（播放排行生成）
│     │  ├─ payment/
│     │  │  └─ service.go        # PaymentService（Stripe 支付流程）
│     │  ├─ p115account/
│     │  │  ├─ service.go        # 115 管理员账号创建、凭证读取与 Cookie 轮换
│     │  │  └─ store.go          # p115_accounts GORM 持久化
│     │  ├─ policy/
│     │  │  ├─ effective_policy.go # 普通用户 Emby Policy 统一重算入口
│     │  │  └─ media_library_settings.go # 分组媒体库模板、用户偏好和同步批次
│     │  ├─ device.go            # DeviceService（设备管理）
│     │  ├─ tvcalendar/
│     │  │  └─ service.go        # TVCalendarService（追剧日历）
│     │  └─ *_errors.go          # 领域错误定义（按业务拆分）
│     ├─ handlers/               # HTTP 处理层（Gin）
│     │  ├─ auth.go              # 登录 / 注册
│     │  ├─ admin_api_key.go     # Admin API Key 状态 / 生成 / 禁用接口
│     │  ├─ user.go              # 用户管理（列表 / 后台创建 / 编辑 / 删除）
│     │  ├─ redemption_code.go   # 兑换码管理
│     │  ├─ config.go            # 设置中心配置接口
│     │  ├─ setting.go           # 系统配置
│     │  ├─ system.go            # 系统信息
│     │  ├─ media.go             # 媒体信息
│     │  ├─ media_quality.go     # 媒体质量盘点
│     │  ├─ media_gap.go         # 缺集管理
│     │  ├─ subscription.go      # 订阅管理
│     │  ├─ tmdb.go              # TMDB 搜索 / 剧集季列表
│     │  ├─ ranking.go           # 播放排行
│     │  ├─ session.go           # 活跃会话
│     │  ├─ playback_history.go  # 播放历史
│     │  ├─ user_playback_profile.go # 用户播放画像
│     │  ├─ device.go            # 设备管理
│     │  ├─ telegram.go          # Telegram 绑定与 Bot Internal API
│     │  ├─ payment.go           # 支付与方案
│     │  ├─ media_library_policy.go # 媒体库模板 / 用户偏好 / Emby Policy 同步接口
│     │  └─ tv_calendar.go       # 追剧日历
│     ├─ middleware/
│     │  ├─ admin_credential_auth.go # 管理员 JWT / Admin API Key 组合认证
│     │  ├─ jwt.go               # JWTAuth + AdminOnly + UserOnly
│     │  └─ internal_auth.go     # InternalAuth（Bot 内部通信认证）
│     ├─ common/
│     │  ├─ jwt.go               # Token 生成/解析（HS256, 7天有效）
│     │  └─ utils.go             # CalculateExpiryDate
│     ├─ security/secretbox/
│     │  └─ secretbox.go         # CONFIG_ENCRYPTION_KEY 共享 AES-GCM 格式与用途隔离派生
│     └─ db/
│        ├─ db.go                # DB 初始化 + VerifySchema + Bootstrap（启动期不再调用 AutoMigrate）
│        └─ migrate.go           # 启动期自动迁移：advisory lock + schema_migrations 记账 + 五分支判断
├─ web/                          # Vue 3 前端
│  ├─ src/
│  │  ├─ api/                    # Axios 请求层
│  │  │  ├─ request.ts           # 基础配置：baseURL=/api/v1, 401拦截
│  │  │  ├─ auth.ts              # login, register, getRegistrationMode
│  │  │  ├─ user.ts              # redeem, redemptions, tmdb
│  │  │  ├─ admin.ts             # 管理后台全部接口
│  │  │  └─ console.ts           # 统一认证路由接口（profile, account-links, subscriptions, payments, rankings, media-libraries 等）
│  │  ├─ plugins/
│  │  │  └─ element-plus.ts      # Element Plus 按需组件/指令/样式注册入口
│  │  ├─ components/
│  │  │  ├─ console/             # 控制台导航 / 顶栏等布局组件
│  │  │  ├─ common/
│  │  │  │  └─ DefaultAvatar.vue # 默认头像（首字母 + 稳定配色）
│  │  │  ├─ ember/               # Ember Web 基础组件层（后台/控制台高频骨架）
│  │  │  │  ├─ data-display/
│  │  │  │  │  ├─ EmberMetricCard.vue # 统计卡基线
│  │  │  │  │  └─ EmberTableCard.vue  # 表格容器 + 分页区基线
│  │  │  │  ├─ filters/
│  │  │  │  │  ├─ EmberDateField.vue      # 单日期筛选字段
│  │  │  │  │  ├─ EmberDateRangeField.vue # 日期范围筛选字段
│  │  │  │  │  ├─ EmberSearchInput.vue    # 搜索输入框
│  │  │  │  │  └─ EmberSelectField.vue    # 下拉筛选字段
│  │  │  │  ├─ forms/
│  │  │  │  │  └─ EmberFormDialog.vue # 通用弹窗表单容器
│  │  │  │  ├─ feedback/
│  │  │  │  │  └─ EmberEmptyStateCard.vue # 空状态容器基线
│  │  │  │  └─ layout/
│  │  │  │     ├─ EmberFilterPanel.vue    # 筛选区容器
│  │  │  │     ├─ EmberPageHeaderCard.vue # 页头卡片
│  │  │  │     └─ EmberSegmentTabs.vue    # 页内分段 tabs
│  │  │  └─ profile/
│  │  │     └─ PlaybackProfileContent.vue # 用户画像共享主体（user/admin 共用）
│  │  ├─ types/api.ts            # 所有 TypeScript 接口定义
│  │  ├─ store/
│  │  │  ├─ auth.ts              # Pinia: token 持久化；role / passwordResetRequired 仅来自 /profile 内存态
│  │  │  ├─ console.ts           # 控制台共享状态（账号资源入口等）
│  │  │  ├─ user.ts              # 用户状态
│  │  │  └─ admin.ts             # 管理员状态
│  │  ├─ router/index.ts         # 路由 + 导航守卫（角色鉴权）
│  │  └─ views/
│  │     ├─ HomeView.vue         # 首页
│  │     ├─ LoginView.vue        # 登录
│  │     ├─ NotFoundView.vue     # 404
│  │     ├─ user/
│  │     │  └─ RegisterView.vue  # 注册（动态模式：open/invite，支持邮箱验证码）
│  │     ├─ console/             # 统一控制台（admin + user 共享布局）
│  │     │  ├─ Layout.vue        # 控制台布局
│  │     │  ├─ DashboardView.vue # 概览页（会员状态 / 媒体统计 / 服务器入口 / 最近入库摘要 / Emby 桥接）
│  │     │  ├─ AccountCenterView.vue # 账号中心（邮箱 / 密码 / Telegram 绑定 / 媒体库偏好 / 帮助资源）
│  │     │  ├─ ProfileAnalyticsView.vue # 我的画像
│  │     │  ├─ SubscriptionsView.vue  # 求片订阅
│  │     │  ├─ NewSubscriptionView.vue # 新建订阅
│  │     │  ├─ TVCalendarView.vue # 追剧日历
│  │     │  ├─ RankingsView.vue  # 播放排行
│  │     │  └─ RenewalCenterView.vue # 续费中心（支付 + 兑换码）
│  │     └─ admin/               # 管理后台
│  │        ├─ UsersView.vue     # 用户管理（筛选 / 后台创建 / 编辑）
│  │        ├─ PlaybackCenterView.vue # 播放分析（用户画像 + 播放历史）
│  │        ├─ UserPlaybackProfilesView.vue # 用户画像总览（嵌入播放分析容器）
│  │        ├─ UserPlaybackProfileView.vue # 单用户画像（详情）
│  │        ├─ RedemptionCenterView.vue # 兑换中心（兑换码池 + 兑换记录）
│  │        ├─ RedemptionCodesView.vue # 兑换码管理
│  │        ├─ RedemptionHistoryView.vue # 兑换历史
│  │        ├─ PaymentCenterView.vue # 支付中心（付费方案 + 支付记录）
│  │        ├─ PlanGroupsView.vue # 用户分组 / 权益模板管理
│  │        ├─ SettingsView.vue  # 设置中心
│  │        ├─ P115AccountsView.vue # 115 源账号 / 播放账号控制面
│  │        ├─ PlansView.vue     # 方案管理
│  │        ├─ PaymentsView.vue  # 支付记录审计
│  │        ├─ SessionsView.vue  # 活跃会话
│  │        ├─ PlaybackHistoryView.vue # 播放历史（嵌入播放分析容器）
│  │        ├─ MediaQualityView.vue # 媒体质量盘点
│  │        └─ DevicesView.vue   # 设备管理
│  ├─ vite.config.ts             # dev:3000, proxy /api→:8080, build manualChunks 分包
│  └─ tailwind.config.js         # 自定义色：ember(橙红), cinema
└─ bot/                          # Python Telegram Bot
   ├─ main.py                    # 入口
   ├─ requirements.txt           # Python 依赖
   ├─ Dockerfile                 # 容器构建
   └─ app/
      ├─ config.py               # 启动期环境变量加载
      ├─ runtime_settings.py     # Bot 运行期设置读取（API + TTL 缓存）
      ├─ server.py               # FastAPI + Telegram Application（Webhook 模式，lifespan 负责 HTTP client 生命周期与 notify 入口）
      ├─ handlers/
      │  ├─ telegram_handler.py  # 消息/回调处理（审批、欢迎消息、菜单清理）
      │  └─ search_cache.py      # 搜索会话缓存
      ├─ formatters/
      │  └─ message_formatter.py # Telegram 消息格式化
      └─ clients/
         └─ api_client.py        # Ember API 内部客户端（共享 AsyncClient 连接池）
```

---

### 3.1 Web 信息架构入口

Web 共享组件层、状态管理、路由守卫、关键页面职责与兼容路由已迁移到 [docs/reference/web-information-architecture.md](./reference/web-information-architecture.md)。

主文档只保留高层边界：

- Web 端继续采用“共享布局 / 页面 view / API 层 / Store”分层
- 共享组件层只承载稳定 UI 契约，不侵入业务编排
- 页面职责、中心页 Tab 结构、兼容路由与页面级数据源统一维护在 [docs/reference/web-information-architecture.md](./reference/web-information-architecture.md)

---

## 4. 数据模型

完整字段字典、模型设计要点和关系说明已迁移到 [docs/reference/data-model-reference.md](./reference/data-model-reference.md)。

本节只保留系统入口级摘要。

### 4.1 核心模型分组

- 账号与认证：`users`、`email_verifications`、`telegram_bind_codes`、`p115_accounts`
- 兑换与支付：`redemption_codes`、`redemptions`、`plans`、`plan_groups`、`payments`、`stripe_webhook_events`
- 内容与行为：`subscriptions`、`subscription_admin_notifications`、`playback_rankings`、`client_blacklists`、`device_actions`
- 追剧与媒体：`tv_calendar_sources`、`tv_calendar_items`、`tv_calendar_subscriptions`、`tmdb_cache`
- 系统运行期：`settings`、`failed_emby_async_ops`、`media_gap_scans`、`bot_runtime_locks`

### 4.2 关键关系

- `User` 是核心主体，向外关联 `Redemption`、`Subscription`、`Payment`、`TelegramBindCode` 和追剧订阅
- `PlanGroup → Plan → Payment` 构成套餐与支付主链路；`User.planGroup`、`RedemptionCode.registrationPlanGroup` 参与用户可见套餐边界
- `Subscription` 承载媒体订阅状态流转，`APPROVED → INGESTED` 与 Emby 入库事件联动；`SubscriptionAdminNotification` 记录每条 Telegram 管理员审批消息的 `chatId/messageId`，用于 Web / Telegram 任一端审批后的消息同步
- `TVCalendarSource / Item / Subscription` 构成追剧日历缓存和用户关注关系
- `Setting` 作为运行期配置 KV 存储层，不通过外键耦合业务表；全局 Admin API Key 仅在该表保存 `external_api_key_hash`，不保存明文
- `P115Account` 是管理员维护的独立外部账号，不归普通用户所有；数据库只保存 Cookie 密文，每个角色至多一条启用记录

### 4.3 维护约束

- 数据库 schema 真相以 `infrastructure/database/` 顶层 migration 为准
- 改模型字段、索引、表结构、约束时，必须同时补 SQL migration
- 完整字段表、枚举和表关系图统一维护在 [docs/reference/data-model-reference.md](./reference/data-model-reference.md)

---

## 5. 后端服务层

### 5.1 AuthService (`services/auth/service.go`, `services/auth/login.go`, `services/auth/register.go`, `services/auth/register_persist.go`, `services/auth/register_notify.go`, `services/auth/emby_binding.go`)

**登录流程**：
1. 通过 `ConfigService` 读取登录保护公开配置；若 `turnstile_login_enabled=true`，则先校验 `turnstileToken`
2. Turnstile 校验通过后再查找用户（`lower(username) = ?` 大小写不敏感比较，按 `createdAt ASC` 取最早记录）→ Admin: bcrypt 校验 → 普通用户: 优先以 Emby 为权威认证，Emby 通过且 `embyUser.ID == user.EmbyID` 时同步本地 hash；Emby 失败时降级本地 bcrypt 校验
3. **不再反向覆盖 Emby 密码**：本地通过 + Emby 失败的路径不再调用 `UpdateUserPassword`，登录链路不修改 Emby 端密码
4. **EmbyID 错配显式拒登**：Emby 返回 `embyUser.ID != user.EmbyID` 时记 ERROR 日志（`username / localEmbyID / remoteEmbyID`）+ 返回与"用户名或密码错误"统一文案，不再静默走兜底
5. 过期用户**可以登录**（前端显示过期提示 + 兑换入口）

**注册流程**：
1. 通过 `ConfigService` 读取 `registration_mode` → `"invite"`: 调用注册场景兑换码校验（会额外校验 `registrationPlanGroup` 仍存在）→ `"open"`: 读取 `default_trial_days`
2. 调用 `ConfigService.IsRegistrationEmailAllowed(email)` 做注册邮箱域名白名单门控；非空白名单且邮箱域名不在白名单内时直接返回 400，不消耗邀请码、不调用 Emby、不写库；空白名单视为关闭限制（详见 §5.5 与 §5.13；reset / change_email / 后台创建用户不走该门控）
3. 如果 `ConfigService` 解析的 `email_verification` 开启，且 SMTP 已配置：校验邮箱验证码（VerifyCode 在事务中"校验即消费"，详见 §5.13）
4. 创建 Emby 用户：`default_trial_days <= 0` 时，在设置 Emby 密码前先写入 `IsDisabled=true` 初始策略；随后设置 Emby 密码并创建本地用户（含 bcrypt hash）；invite 模式使用兑换码必填的 `registrationPlanGroup` 写入 `users.planGroup`；open 模式显式写入当前默认分组，避免新增用户继续依赖 `users.plan_group IS NULL` 的隐式跟随语义
5. 本地事务提交后调用 `ApplyEffectiveUserPolicy(user_registered)` 全量写入当前有效 Emby Policy；若外部写入失败，注册仍按成功返回，响应带 `policySyncStatus=pending`，并记录 `emby_policy_sync_tasks(status=pending, reason=user_registered)` 交给 Policy worker 重试
6. 签发 JWT
7. 火忘式通知 Bot（新用户注册）

**唯一性校验**：`ensureRegisterUserUnique` 用 `lower(username) = ?` / `lower(email) = ?` 比较，与登录链路保持一致；schema 层由 `20260426_01_users_lower_unique_indexes.sql` 创建的函数唯一索引（`uq_users_username_lower` / `uq_users_email_lower`）兜底，避免并发或多入口写入造成大小写逻辑重复账号。

**关键 struct**：
- `RegisterUserRequest{Username, Password, Email, Code, EmailCode}` — Code/EmailCode 可选
- `LoginRequest{Username, Password, TurnstileToken}`
- `LoginResponse{Token, User, IsExpired}`

**Admin API Key**（`services/accessauth/admin_api_key.go`）：
- 设置中心提供全局 Admin API Key 的状态、生成 / 轮换和禁用能力；生成的明文以 `ember_sk_` 开头，只在 `POST /api/v1/admin/external-api-key` 响应中返回一次
- 数据库只保存 `settings.external_api_key_hash`（SHA-256 hex）；空值表示未启用，重新生成会覆盖旧 hash，禁用会清空 hash
- 校验路径由 `AdminCredentialAuth()` 识别 `Authorization: Bearer ember_sk_xxx`，使用 constant-time compare 对比 hash；成功后注入 `authType=api_key`、`role=admin`、`userID=api_key`
- API Key 没有真实当前用户语义，不进入 `/api/v1/user/*`、统一认证用户侧接口或 `/api/v1/internal/*`；管理 key 本身的接口禁止 API Key 自管理

**管理员 Emby 账号自助绑定**（`emby_binding.go`）：
- `ListAdminEmbyUsers(userID, {query, limit})` — 要求 `query` 至少 2 个字符；通过 `EmbyService.GetUsers` 拉取 Emby 用户列表后在服务端按用户名 / ID 过滤并截断到 `limit`，再合并本地 `users.emby_id` 占用状态；返回 `data[]`，每项包含 `embyId / name / hasPassword / boundUsername / boundToCurrent / available`，供前端选择绑定目标；前端弹窗不自动加载全量 Emby 用户
- `BindEmbyAccount(userID, {embyId})` — 复用 `EmbyService.GetUserByID` 校验目标 Emby 用户仍存在 → 应用层先查冲突：当前用户已绑同一 ID 走幂等成功；当前用户绑了其他 ID 返回 409 `ErrEmbyAlreadyBound`；目标 ID 已被其他本地账号占用返回 409 `ErrEmbyUserOccupied`（错误消息含冲突方 username）→ UPDATE `users.emby_id`；DB 层由偏唯一索引 `uniq_users_emby_id` 兜底并发，`23505` 唯一约束冲突翻译为 `ErrEmbyUserOccupied`
- `UnbindEmbyAccount(userID)` — 直接清空 `emby_id`，幂等；不删除 Emby 真实用户、不修改 Emby 任何属性
- 不影响登录链路：管理员仍走本地密码；该接口仅用于让管理员获得普通用户级别的 Emby 相关读权限（媒体 latest、个人播放档案、自助兑换等）
- 绑定流不接收 Emby 明文密码；外部 Emby 用户不存在返回 404，Emby 配置缺失或不可达返回 502，401 只保留给 Ember 登录态失效
- 不影响启动期 `seedDefaultAdmin`：seed 仍纯本地，不调用 Emby

### 5.2 UserService (`services/user/service.go`, `services/user/admin.go`, `services/user/profile.go`, `services/user/password.go`, `services/user/password_reset.go`)

- `GetUsers(page, pageSize, search, isActive, expiresAfter, embyStatus, planGroup)` — 分页搜索（`expiresAfter` 格式 `YYYY-MM-DD`，筛选 `expiresAt > expiresAfter`；`embyStatus` 支持 `available/disabled/unlinked`；`planGroup` 按“有效套餐分组”过滤：用户显式分组优先，否则回退系统默认分组）
- `UpdateUserByAdmin(userID, req)` — 管理员更新用户邮箱/状态/套餐组/到期时间；`planGroup` 不传表示不改，传合法 key 表示绑定目标分组，传空字符串会被拒绝；有效分组变化后会同步把该用户关联的 `pending` 支付标记为 `expired`
- `ExtendExpiry(userID, days)` — 已过期从 now 起算，未过期从 ExpiresAt 叠加
- `GetProfile(userID)` — 获取用户个人资料
- `UpdateEmail(userID, req)` — 邮箱变更落库；`UpdateEmailRequest{NewEmail string \`binding:"required,email"\`, Code string \`binding:"required,len=6"\`}`，先做 `unchangedEmailCheck` → 调用 `EmailService.IsRegistrationEmailAllowed(newEmail)` 做注册邮箱域名白名单门控（命中拒绝在事务开启前直接返回，不消费验证码、不写库；与 `SendEmailChangeCode` 共用同一份语义防御 send-code 通过后管理员收紧白名单的窗口）→ 事务内 `EmailService.ConsumeCodeTx(tx, newEmail, code, change_email)`（校验即消费）→ `UPDATE users.email`；返回 `*UpdateEmailResult{OldEmail, User}` 由 handler 用于 fire-and-forget 通知旧邮箱
- `UpdatePassword(userID, old, new)` — Emby + 本地 hash 同步
- `ResetPassword(userID, new)` — 管理员重置，Emby + 本地 hash 同步
- `ResetPasswordByCode(email, code, newPassword)` — 通过注入的验证码校验能力完成邮箱验证码重置，不再在方法内部自行构造 EmailService
- `ToggleUserStatus(userID)` — 翻转 IsActive

### 5.3 RedemptionCodeService (`services/redemption/code_service.go`)

- `CreateRedemptionCode(maxUses, defaultDays, expiresAt, registrationPlanGroup, notes)` — 生成 16 字符 hex 码；`registrationPlanGroup` 必填，创建时校验分组存在
- `CreateRedemptionCodesBatch(count, maxUses, defaultDays, expiresAt, registrationPlanGroup, notes)` — 批量生成兑换码，`registrationPlanGroup` 必填，单次最多 100 个，整批事务提交
- `GetRedemptionCodes(page, pageSize, showAll, code, status, registrationPlanGroup)` — 支持按兑换码关键字、状态（`active|expired|exhausted`）和注册套餐分组过滤，并返回 `notes`、`registrationPlanGroupName`；未指定 `status` 且 `showAll=false` 时仅返回当前仍可兑换的码
- `ValidateRegistrationCode(code)` — 注册场景兑换码校验；查找 + IsValid()，并强校验 `registrationPlanGroup` 仍存在
- `ValidateRenewalCode(code)` — 续期场景兑换码校验；只查找 + IsValid()，忽略 `registrationPlanGroup`
- `UseCode(code)` — 原子递增 usedCount

### 5.4 RedemptionService (`services/redemption/service.go`)

**核心方法 `RedeemCode(userID, code)`**：
1. 开启事务后查询兑换码并校验 `IsValid()`
2. 在事务中检查 `redemptions(userId, code)` 是否已存在，存在则返回 `ErrRedemptionDuplicate`
3. 查询用户并计算新 ExpiresAt，仅更新 `expiresAt/embyDisabled`（**Emby 调权移到 commit 后异步执行**）
4. 先插入 Redemption 记录（依赖 `redemptions(userId, code)` 唯一约束兜底并发重复兑换）
5. 原子递增 usedCount（`WHERE usedCount < maxUses AND (expiresAt IS NULL OR expiresAt > now)`）→ 提交
6. commit 后异步调用 `ApplyEffectiveUserPolicyOrRecordFailure(userID, "redemption_renewal")`：成功后刷新 Emby 禁用缓存；失败写入 `emby_policy_sync_tasks` 的单用户 `failed` 处理记录，由管理员在用户管理中手动重试

### 5.5 ConfigService (`config/config.go`)

- `List()` — 返回配置定义 + 当前解析结果（来源、是否有值、是否敏感、是否需重启）
- `Update(key, req, userID)` — 更新单项配置，支持敏感值加密存储
- `ResolveString(key)` / `GetString(key)` — 统一配置读取入口
- `GetRegistrationMode()` / `GetDefaultTrialDays()` / `IsEmailVerificationEnabled()` / `GetStripeAllowedPaymentMethods()` / `GetRegistrationAllowedEmailDomains()` — 业务配置便捷读取
- `IsRegistrationEmailAllowed(email)` — 注册邮箱域名白名单门控；空白名单视为关闭，非空时按 `mail.ParseAddress` 解析后的 host 与白名单做精确小写比较（不做后缀匹配），由 `auth.RegisterUser` 与 `email.SendVerificationCode(register)` 共用同一份语义；保存时由 `validateRegistrationEmailDomains` + `normalizeRegistrationEmailDomains` 拒绝非主机名格式（协议 / 端口 / 路径 / 通配符 / `@`），并落库为小写、去重、按字典序排序的稳定形态
- `TestGroup(group)` — 分组配置连通性测试（v1: `media`、`email`）

**关键职责**：
- 配置定义注册表（标签、分组、类型、校验、默认值）
- 读取策略由配置定义控制：已托管的运行期集成配置优先数据库并可禁用 env 回退；部署边界配置仍保留 env / default 解析
- 敏感值加密：`CONFIG_ENCRYPTION_KEY`
- 运行期配置中心 API 的后端基础设施
- `external_api_key_hash` 属于设置中心可见的只读敏感项，由 Admin API Key 专用接口写入或清空，不通过通用 `UpdateConfig` 手填

### 5.6 SystemService (`services/system/service.go`, `services/system/expiry.go`)

- `GetSystemInfo()` — 统计：用户数、活跃数、兑换码数
- `CheckExpiredUsersWithContext(ctx)` — **cron 核心**：查询 `expiresAt < NOW() AND embyDisabled = false` → 调用 Emby `SetUserPolicy(IsDisabled: true)` → 设置 `EmbyDisabled = true`；支持 `context cancel`，并对错误样本 / 失败用户返回做上限保护，避免长时间任务在中断或大量失败时继续无界膨胀

### 5.7 EmbyService (`integrations/emby/emby.go`)

Emby 媒体服务器 HTTP 客户端，10 秒超时。

| 方法 | Emby 端点 | 用途 |
|------|-----------|------|
| `AuthenticateUser` | `POST /emby/Users/AuthenticateByName` | 用户认证 |
| `CreateEmbyUser` / `CreateEmbyUserWithInitialDisabled` | `POST /emby/Users/New` | 创建账号；0 天试用注册会在设置密码前追加初始禁用 Policy |
| `UpdateUserPassword` | `POST /emby/Users/{id}/Password` | 修改密码 |
| `SetUserPolicy` | `POST /emby/Users/{id}/Policy` | 封禁/解封（IsDisabled） |
| `GetMediaStats` | `GET /emby/Items/Counts` | 媒体库统计 |
| `GetLibraries` | `GET /emby/Library/VirtualFolders/Query` / `GET /emby/Library/VirtualFolders` | 媒体库列表；过滤 Emby 系统生成的 `boxsets` 合集入口 |
| `GetAdminLibraryContext` | `GET /emby/Users` + `GET /emby/Users/{adminUserId}/Views` | 返回确定的管理员 ID 和同一用户上下文下的媒体库视图；找不到管理员直接失败 |
| `GetUserLibraryItemsByIDs` | `GET /emby/Users/{adminUserId}/Items?ParentId=...&Ids=...&Recursive=true` | 在同一管理员上下文中查询候选条目与所选媒体库的交集，避免混用用户 Views 与全局 Items |
| `GetUsers` | `GET /emby/Users` | 连接测试 |
| `GetDevices` | `GET /emby/Devices` | 设备列表 |
| `GetAllSessions` | `GET /emby/Sessions` | 全量会话（含非播放） |
| `LogoutDevice` | `DELETE /emby/Devices/{id}` | 强制设备下线 |

**认证方式**：`X-Emby-Token` 头

**配置读取**：
- `refreshConfig()` 在调用前通过 `ConfigService` 重新解析 `EMBY_URL` / `EMBY_API_KEY`
- Emby 相关配置已切换为设置中心托管，运行期不再隐式回退到 Docker env
- 设置中心改完 Emby 配置后，无需重启 API 即可对新请求生效

### 5.8 MediaService (`services/media.go`)

- `GetEmbyConfig()` — 优先返回设置中心里的前端 Emby 地址（配置键沿用 `NEXT_PUBLIC_EMBY_URL`），为空时回退 `EMBY_URL`，供控制台拼接 Emby 图片与地址展示
- `IsEmbyConfigured()` — 代理 `EmbyService.IsConfigured()`，供 dashboard 媒体接口在调用上游前判断"系统是否已配置 Emby"，避免把"未配置"当成上游错误返回
- `GetMediaStats()` — 5 分钟 RWMutex 缓存层
- `GetLatestItems(embyUserID, itemType, limit)` — 通过 Emby `/Users/{userId}/Items/Latest` 获取最近入库媒体，并做短 TTL 去重缓存

> Dashboard 三个接口（`/emby/config`、`/media/stats`、`/media/latest`）在 emby 未配置 / 用户未绑定 Emby 时返回 `200 + configured/bound` 业务标志位，仅在依赖已配置但调用真实失败时才走 `500 + "上游服务暂不可用"`；详见 [docs/reference/api-response-standard.md](reference/api-response-standard.md) 的《未初始化态 vs 上游错误》章节。前端首屏探测请求统一带 `silent: true`，由组件自行渲染空态，避免 fresh-install 场景下叠加触发全局 toast。

### 5.9 SubscriptionService (`services/subscription.go`)

- `CreateSubscription(userID, type, name, tmdbId, season)` — 提交前强校验用户 `embyId` 非空，且按业务真相 `!IsExpired() && !EmbyAccessDisabled` 判定其 Emby 仍可用，而不是依赖 `embyDisabled` 缓存；随后在事务内同时串行化“同资源活跃唯一”和“用户当天自动通过额度”两类约束。命中所属 `PlanGroup.subscriptionAutoApproveDailyLimit` 时直接写 `APPROVED + reviewedAt + reviewSource='AUTO_QUOTA'` 并异步下发 MoviePilot，只向管理员发送只读通知；超额时继续写 `PENDING` 并走现有待审批通知。命中已有活跃订阅返回 `AlreadyExists=true` 幂等成功（不再 409）
- `ResubmitSubscription(userID, rejectedSubscriptionID, note)` — 同上资格校验、幂等保护和自动通过额度判断；新记录写入 `retryFromId`，原记录保持 `REJECTED`
- `ApproveSubscription(id)` — **批次 2 改原子状态转移**：`UPDATE WHERE status='PENDING'`，`RowsAffected=0` 返回 `ErrSubscriptionStateConflict` → handler 映射 409。MoviePilot 调用从同步路径剥离到 commit 后 `async.SafeGo("subscription.dispatchMoviePilot", ...)`，失败仅写 `mpError`，状态保持 APPROVED；状态更新成功后异步同步所有已落库的 Telegram 管理员审批消息并移除按钮
- `RejectSubscription(id, reason)` — 同样原子状态转移，`RowsAffected=0` 返回 `ErrSubscriptionStateConflict`；状态更新成功后异步同步所有已落库的 Telegram 管理员审批消息并移除按钮
- `RedispatchSubscription(id)` — **批次 2 新增**：管理员手动重试 MoviePilot 调用，仅在 `status='APPROVED'` 且 `mpError != nil` 时允许；状态保持 APPROVED；mpError 由本次结果覆盖。路由 `PUT /api/v1/admin/subscriptions/:id/redispatch`
- `ManualSearchSubscription(id, season?)` — 管理员手动补偿搜索，仅允许 `APPROVED` 订阅；电影按 `tmdbId + movie` 搜索，单季剧按订阅季号搜索，整剧订阅 (`season=0`) 必须显式提交季号。路由 `POST /api/v1/admin/subscriptions/:id/manual-search`
- `ManualDispatchSubscription(id, candidate, season?)` — 管理员将选定候选直接下发 MoviePilot 下载入口；请求体带 `torrent_in` 与 `tmdbid`，TV 订阅同时带 `season`，整剧订阅 (`season=0`) 必须提交搜索时使用的季号；订阅状态保持 `APPROVED`。成功清空 `mpError`（清除旧的自动订阅失败痕迹）；**失败不写 `mpError`**（避免与 `RedispatchSubscription` 的"重试订阅创建"链路混用，redispatch 可见条件依赖 `mpError` 非空），仅同步返回错误。`POST /api/v1/download/add` 业务失败（HTTP 200 + 顶层 `success=false`）由 `DispatchDownloadCandidate` 显式识别，不再被误判为 `Accepted:true`；该路径返回 sentinel `ErrMoviePilotBusinessRejected`，handler 层 `errors.Is` 命中后映射为 **409 Conflict**（业务拒绝是状态冲突，如重复添加），业务原因（脱敏后的 message）透传给管理员；基础设施错误（网络/HTTP 5xx）仍走 `SafeUpstreamError` 脱敏 + 500
- `MarkSubscriptionIngestedAsAdmin(id)` — 管理员校验 Emby 后手动收口为 INGESTED；批次 2 起触发与 webhook 一致的 `notifyIngested` 通知
- `MarkSubscriptionsIngestedByWebhook(payload)` — **批次 2 拆分整剧 / 单季命中策略**：
  - 单季订阅 (`season=N`)：webhook 命中即 INGEST
  - 整剧订阅 (`season=0`)：用 TMDB 已播出剧集清单计算总量 `X`，再结合 Emby 当前实际库存与 `IGNORED` 集排除项计算已完成集数 `Y`，写 `ingestProgress="Y/X"`；`Y >= X` 时自动收口为 INGESTED 并触发用户通知。TMDB / Emby 任一侧缺失时不做错误自动收口，fallback 记最近一集 `S01E10`，由管理员通过 `MarkSubscriptionIngestedAsAdmin` 显式确认
  - 剧集 webhook 缺主条目 `tmdbId` 时，会调用 `resolveSeriesTMDBIDBySeriesID(seriesId)`；该解析结果已加 5 分钟进程内 TTL 缓存，避免同批 webhook 反复查询 Emby

### 5.10 DeviceService (`services/device.go`)

- `GetDevices(req)` — 设备列表（支持 userId/clientName/isBlacklisted 过滤 + 分页）
- `GetBlacklist()` / `AddClientToBlacklist()` / `RemoveClientFromBlacklist()` — 客户端黑名单管理
- `LogoutDevice(deviceID)` — 强制注销单设备
- `LogoutBlacklistedDevices()` — 批量注销黑名单设备
- `GetStats()` — 客户端分布、设备分布、黑名单数量、活跃会话数
- `GetDeviceActions(limit)` — 最近设备操作日志

### 5.11 MediaGapService (`services/mediagap/service.go`)

- `ScanMediaGaps(tmdbId?)` — 扫描 Emby 连载剧的已激活季，创建/更新/核销缺集工单；后台管理入口已改为异步触发。**批次 2 新增跨副本互斥**：`mediaGapScanManager.Start` 通过 `pg_try_advisory_lock` 拿到独占锁后再写 `media_gap_scans (status='running')` 并启动 goroutine（`async.SafeGo` 包裹），结束时在 `defer` 内释放锁并写终态；锁被其他副本占有时返回 409
- `ListGroupedMediaGaps(query)` — 按剧聚合缺集工单，后端完成分组、排序、分页与摘要统计
- `SearchGap(id)` — 调用 MoviePilot 搜索当前缺集候选；写入 `searchSnapshot` 与 `lastSearchedAt`
- `DispatchGap(id, candidate)` — 调用 MoviePilot 下载入口下发已选候选资源，请求体带缺集 `tmdbId`；成功推进为 `REQUESTED` 并清空 `lastDispatchError`；**失败时写入 `lastDispatchError` 并切换为 `DISPATCH_FAILED`**：MoviePilot 业务拒绝保留已脱敏的 message，基础设施错误经 `upstream.SafeUpstreamError` 脱敏；前端可通过同一接口重试
- `IgnoreGap(id, reason)` — 将单条缺集工单标记为 `IGNORED`；显式忽略写 `ignoreReasonCode='manual'`
- `MarkIngestedByWebhook(payload)` — Emby webhook 命中缺集工单后按状态分支处理：
  - `MISSING` / `SEARCHED` / `REQUESTED` / `DISPATCH_FAILED` → 收口为 `INGESTED`
  - `INGESTED` → noop
  - `IGNORED` → **仅 INFO 日志，不再清空 `ignoredAt / ignoreReason / ignoreReasonCode`**（管理员的忽略决定不能被自动撤销）

### 5.12 MoviePilotClient (`integrations/moviepilot/client.go`)

- `IsConfigured()` — 检查 `MOVIEPILOT_URL` 与 `MOVIEPILOT_API_KEY` 是否齐全
- `TestConnection()` — `GET /api/v1/site/`，请求头使用 `X-API-KEY`
- `CreateSubscription(type, name, tmdbId, season)` — `POST /api/v1/subscribe/`，请求头使用 `X-API-KEY`（`type` 转中文：movie→电影, tv→电视剧；`season>0` 时透传季号）
- `SearchMediaCandidates(tmdbId, mediaType, season)` — `GET /api/v1/search/media/tmdb:<tmdbId>`，Ember 内部 `mediaType=movie|tv` 会在 MoviePilot client 适配为 `mtype=电影|电视剧`，电视剧传 `season=N`，用于订阅手动补偿
- `SearchTitleCandidates(keyword)` — `GET /api/v1/search/title`，通用标题搜索入口；缺集搜索包装会调用它
- `SearchGapCandidates(seriesName, season, episode)` — `GET /api/v1/search/title`，优先搜索 `SxxExx` 单集，空结果时回退 `Sxx` 整季包；候选按做种数、体积排序
- `DispatchDownloadCandidate(candidatePayload, tmdbId?, season?)` — `POST /api/v1/download/add`，将选中的资源快照放入 `torrent_in`，有 TMDB ID 时同时传 `tmdbid`，TV 手动下发传 `season`；解析响应顶层 `success` 字段：`success=false` 时返回业务错误（MoviePilot v2 在业务失败时 HTTP 仍返回 200，仅靠 `success` 区分）
- `DispatchGapCandidate(candidatePayload, tmdbId)` — 缺集业务包装，内部调用 `DispatchDownloadCandidate`

### 5.13 EmailService (`services/email/service.go`, `services/email/verification.go`, `services/email/sender.go`)

邮箱验证码发送、校验和清理服务，基于 SMTP。

- `SendVerificationCode(email, ip, codeType)` — 先对 email 做 `strings.ToLower(strings.TrimSpace(...))` 规范化 → `codeType=register` 时调用 `ConfigService.IsRegistrationEmailAllowed` 做域名白名单门控（命中拒绝直接返回 `ErrEmailDomainNotAllowed`，handler 映射 400；不消耗限流配额、不调用 SMTP；reset / change_email 不走该门控以保留反账号枚举语义）→ 生成 6 位随机验证码 → 按类型频率限制（每邮箱/每 IP 每日上限）→ SMTP 发送
- `SendEmailChangeCode(currentEmail, newEmail, ip)` — 账号中心邮箱变更专用，`codeType` 固定为 `change_email`，验证码发往新邮箱；业务前置校验"新邮箱与当前邮箱相同"返回 `ErrEmailUnchanged`、命中注册邮箱域名白名单门控返回 `ErrEmailDomainNotAllowed`（与 `register` 路径共用 `ConfigService.IsRegistrationEmailAllowed` 同一份语义；只校验 `newEmail`，不看 `currentEmail`，避免老用户被锁）、"新邮箱被其他用户占用"返回 `ErrEmailAlreadyBound`，三者均不消耗限流配额；其余流程复用 advisory lock + 24h 双维度限流骨架，与 `register` / `reset` 共用同一份限流入口但配额按 `(email, type)` / `(ip, type)` 隔离
- `IsRegistrationEmailAllowed(email)` — `ConfigService.IsRegistrationEmailAllowed` 的 thin delegate，让 `UserService.UpdateEmail` 等上层不直接依赖 `ConfigService`，把"邮箱业务策略"在 EmailService 层统一收口
- `SendEmailChangeNotification(oldEmail, newEmail)` — 邮箱变更成功后给旧邮箱发送通知邮件；不写 `email_verifications` 表、不消耗限流配额；SMTP 未配置直接返回 `ErrEmailNotConfigured`，其余失败仅记日志
- `VerifyCode(email, code, codeType)` — **"校验即消费"契约**：先按同一 email 规范化规则查询；在事务中 `Clauses(clause.Locking{Strength: "UPDATE"})` 锁行 → 校验有效期与 code 匹配 → 立即 DELETE 该行；同一码不可重放用于注册或重置，并发拿到同一码的第二个请求统一返回 `ErrEmailCodeInvalid`
- `CleanupExpired()` — 删除过期验证码（cron 调用）
- `IsEnabled()` — 通过 `ConfigService` 解析 `email_verification`，并叠加 SMTP 完整性判断
- `TestConnection()` — 基于当前 SMTP 配置做连通性探活

**频率限制**：
- 每邮箱每日：`EMAIL_CODE_DAILY_LIMIT`（默认 5，按规范化后的 email + `codeType` 隔离计数）
- 每 IP 每日：`EMAIL_CODE_IP_DAILY_LIMIT`（默认 15，**按 `codeType` 隔离计数**：`register` / `reset` / `change_email` 三类配额互不串号）
- 验证码有效期：`EMAIL_CODE_EXPIRY_MINUTES`（默认 10 分钟）

**忘记密码反枚举**：`SendResetCode` handler 除请求格式错误（400）和 SMTP 未配置（503）外，所有内部错误（未注册邮箱 / 限流 / 发送失败 / 其他）一律折叠为 200 + 与已注册一致的统一文案；service 层在 reset 路径上无论邮箱是否注册都先消耗 IP / email 限流配额（未注册邮箱仍写入限流计数行但不发送邮件），攻击者无法借响应状态、文案或限流差异（200 vs 429）枚举站内邮箱。内部记录 emailHash 前 8 位 + clientIP + 错误细节用于排障。

**账号中心邮箱变更状态码映射**：
- `POST /api/v1/email/send-code` / `POST /api/v1/user/email/send-code`
  - 200：`{"message":"验证码已发送至新邮箱"}`
  - 400：请求体格式错误、`ErrEmailUnchanged`（新邮箱与当前邮箱相同）、`ErrEmailDomainNotAllowed`（新邮箱不在注册邮箱域名白名单内）、`ErrEmailAlreadyBound`（新邮箱被他人占用）、`ErrEmailAlreadyRegistered`
  - 429：`ErrEmailCodeRateLimit` / `ErrEmailCodeIPRateLimit`
  - 503：`ErrEmailNotConfigured`（SMTP 未配置）
  - 500：其余内部错误统一走 `httpx.InternalError`
- `PUT /api/v1/email` / `PUT /api/v1/user/email`
  - 200：返回更新后的 `User` 对象，handler 紧接着 `go func()` 调 `SendEmailChangeNotification` 通知旧邮箱（带 `recover`，仅当 `oldEmail != ""` 时触发）
  - 400：请求体格式错误（缺 `newEmail` / `code` 不是 6 位）、`ErrEmailUnchanged`、`ErrEmailCodeInvalid`、`ErrEmailAlreadyExists`、`config.ErrRegistrationEmailDomainNotAllowed`、`config.ErrRegistrationEmailInvalid`
  - 404：`ErrUserNotFound`
  - 500：其余内部错误统一走 `httpx.InternalError`

### 5.14 BotNotifier (`integrations/notifier/notifier.go`)

火忘式 HTTP 推送通知服务，将事件推送给 Telegram Bot。

- 进程内通过 `GetSharedBotNotifier()` 复用单例，避免每次通知都重建客户端与配置解析
- `BOT_NOTIFY_URL` 读取带 30 秒刷新节流；`Reload()` 可强制刷新配置缓存
- 发送日志统一包含 `endpoint / event / payloadSize / requestId / latency`，便于串联 API → Bot 通知失败链路

**通知类型**：
| 方法 | Bot 端点 | 触发时机 |
|------|----------|----------|
| `NotifyNewSubscriptionWithDeliveries` | `POST /notify/subscription` | 用户创建求片订阅；Bot 返回管理员消息投递引用供 API 落库 |
| `SyncSubscriptionAdminMessages` | `POST /notify/subscription-admin-sync` | 订阅审批成功后批量同步管理员审批消息状态 |
| `NotifySubscriptionApproved` / `NotifySubscriptionRejected` / `NotifySubscriptionIngested` | `POST /notify/subscription-result` | 用户订阅审核结果 / 入库结果 |
| `NotifyNewRegistration` | `POST /notify/registration` | 新用户注册 |
| `NotifyPaymentSuccess` | `POST /notify/payment` | Stripe 支付履约成功 |
| `NotifyRanking` | `POST /notify/ranking` | 排行榜生成完成 |

**认证方式**：`X-Internal-Secret` 头（值 = `INTERNAL_API_SECRET`）

**通知载荷脱敏（PaymentSuccessNotification）**：admin 通知不再包含 `Email` / `StripeSessionID` / `StripePaymentIntentID`，避免长期落 Telegram 服务器。运维侧追溯单笔支付走后台审计页，通过 `paymentId` / `userId` 查询。

### 5.15 PlaybackRankingService (`services/playback/ranking.go`)

从 Emby PlaybackActivity 数据库生成播放排行。

- `GenerateRanking(period)` — 无数据读取地校验 PlaybackActivity 六个必需字段 → 读取排行榜媒体库 allowlist 与管理员上下文 → 电影候选按 `ItemId` 扩窗；episode 候选回查详情后按 `SeriesId` 归并；再由同一管理员的 Items 接口按 `ParentId + Ids` 筛选候选与所选媒体库的交集 → 存入数据库 → 通知 Bot
- `GetLatestRanking(period)` — 获取指定周期最近一批正式排行榜（按 `periodEnd` 排序，不按 `snapshotAt` 猜）
- `GetHistoryRanking(period, rangeStart, rangeEnd)` — 按统计周期查询历史排行；新格式按 `batchId` 读取，旧格式按 `snapshotAt` 兼容
- `NotifyRanking` 推送 payload 额外包含整期 `totalDuration`，用于 Telegram 展示当天/当周总播放时长
- `PreviewRanking(period)` — 即时预览当前周期排行（不持久化、不推送）
- `GetRankingLibraryAllowlist()` / `UpdateRankingLibraryAllowlist()` — 管理员读取或保存排行榜参与统计的媒体库 allowlist；空配置视为全部媒体库参与统计

**支持周期**：`daily`（日榜）、`weekly`（周榜）

**实现约束**：

- 正式榜单按 `batchId` 组织，同一期电影榜和剧集榜共享同一批次
- 最新榜不再按 `category` 分开读取，统一返回整期榜单
- 聚合键不再使用 `ItemName`
- 电影榜直接依赖 PlaybackActivity 的 `ItemId`
- 当前 PlaybackActivity 不返回 `SeriesId` / `SeriesName`，剧集榜需额外回查 Emby 媒体详情后按 `SeriesId` 归并
- 排行榜媒体库范围使用全站统一 allowlist，而不是按用户可见媒体库拆分
- allowlist 为空时默认统计全部媒体库；非空时把管理员 View ID 和电影 `ItemId` / 剧集 `SeriesId` 候选交给 `/Users/{adminUserId}/Items` 做范围查询，不在本地直接比较 Views、`ParentId`、Ancestors 的 ID

### 5.16 PaymentService (`services/payment/service.go`)

Stripe 一次性支付流程管理。

- `GetPlanGroups()` / `CreatePlanGroup()` / `UpdatePlanGroup()` / `DeletePlanGroup()` — 后台套餐分组管理；默认分组全局唯一；分组除名称/排序外还承载 `subscriptionAutoApproveDailyLimit` 这类审核权益配置。分组存在性、引用检查（`plans` / `users` / `redemption_codes.registrationPlanGroup`）和默认分组切换收口都在应用层完成，切换默认分组时会同步收口跟随默认用户的 `pending` 支付
- `CreateCheckoutSession(userID, planID)` — **批次 2 改造为占位幂等模式**：先在事务里 `INSERT payments (status='pending', stripeSessionId='') ON CONFLICT (uq_payments_pending_user_plan) DO NOTHING`，命中冲突回查现有 pending 复用；事务外调 Stripe 时携带 `Idempotency-Key=checkout:<paymentId>`，并发的两个请求拿到同一 paymentId → Stripe 返回同一 Session；最后 `UPDATE payments SET stripeSessionId, checkoutUrl WHERE id=?` 回填
- `GetPlansForUser(userID)` — 登录态可购方案列表，仅返回当前用户有效分组下的启用套餐
- `HandleWebhook(payload, signature)` — 签名验证后按 `event.id` 在 `stripe_webhook_events` 做去重 + 失败重试状态机：
  - 首次 `INSERT ON CONFLICT DO NOTHING` 成功 → 进入业务分发
  - 命中冲突时回查 status：`processed / skipped` → 真正幂等 200 不再分发；`received / failed` → 视为上次未完成（崩溃中断 / 业务返回 5xx），允许 Stripe 自动重试驱动履约，同时把 `receivedAt` 刷新为本次重投时间
  - 分发完成后 UPDATE 写终态；`checkout.session.expired` → `MarkPaymentExpired(sessionID)` 把本地 pending 收口为 expired
- `fulfillPayment(sessionID, paymentIntentID, eventCreated, metadata)` — 事务内只做 Payment / User 状态更新和 `expiresAt` 延长（**Emby 调权移到 commit 后异步执行**）；引入 `event.created < payment.updatedAt` 乱序保护；commit 后异步调用 `ApplyEffectiveUserPolicyOrRecordFailure(userID, "payment_fulfillment")`，Emby 写入失败不回滚支付履约，但会写入单用户 `failed` 处理记录供管理员手动重试
- `markPaymentFailed(sessionID, eventCreated)` — 同样接受 `eventCreated`，做乱序保护
- `MarkPaymentExpired(sessionID)` — `UPDATE payments SET status='expired' WHERE stripeSessionId=? AND status='pending'`，`RowsAffected=0` 视为已收口（noop）
- 邀请码模板用户 Policy 复制链路已废弃；注册权益只来自 `registrationPlanGroup` 对应的分组媒体库模板和 Emby 权益模板

### 5.17 错误定义（按业务拆分）

统一的业务错误定义已按领域拆分，例如：

- `services/redemption/errors.go`
- `services/email/errors.go`
- `services/payment/errors.go`
- `services/subscription/errors.go`
- `services/telegram/errors.go`
- `services/device/errors.go`
- `services/tvcalendar/errors.go`
- `services/playback/errors.go`
- `services/media_quality_errors.go`

handler 继续通过 `errors.Is()` 做错误映射。

### 5.18 TelegramService (`services/telegram/service.go`)

Telegram 账号绑定与 Bot 自助能力服务。

- `GenerateBindCode(userID)` — 生成 6 位绑定验证码（5 分钟有效）；改用 `Clauses(clause.OnConflict{Columns: userId, DoUpdates: code/expiresAt/createdAt}).Create` **原地刷新**，与 `telegram_bind_codes(userId)` 上的唯一索引（`uq_telegram_bind_codes_user`，由 `20260426_01` migration 引入）配合实现真正的原子互斥，不再依赖事务里的 DELETE+INSERT
- `VerifyBind(telegramID, code)` — 校验验证码并绑定 Telegram ID（事务 + 行锁）；命中同 code 多条记录时记 ERROR 日志（`code / count`）+ 改返回 `ErrTelegramBindCodeInvalid`，**反 DoS**：避免攻击者借"绑定码碰撞"造成全量用户绑不上
- `Unbind(userID)` — 解绑 Telegram ID
- `GetAccountInfo(telegramID)` — 查询绑定用户账号状态
- `RedeemByTelegram(telegramID, code)` — 复用 `RedemptionService` 完成续期兑换
- `ResetPassword(telegramID, newPassword)` — 通过 Telegram 身份重置 Ember/Emby 密码
- `/libraries` — Bot 私聊媒体库偏好入口；通过 Internal API 查询、切换单个媒体库或恢复分组默认，最终仍由 API 统一重算 Emby Policy
- `/search` — Bot 私聊 TMDB 搜索入口；Bot 通过 `InternalAuth` 保护的内部 TMDB 代理访问搜索与剧集季列表，不重新开放匿名 TMDB 代理
- `SubscribeByTelegram(req)` — Bot 求片订阅入口；电影直接确认，电视剧先选季再提交，并透传 `season`；为保持既有体验，Bot 提交默认视为已确认库内已存在提示，不走 Web 二次确认弹窗
- `CleanupExpiredBindCodes()` — 删除过期绑定码（cron 调用）

**反账号枚举（handler 层）**：`VerifyBind` 命中绑定码无效、Telegram 已绑定或 Ember 用户已绑定时统一返回 400 + 中性绑定失败文案；`GetAccountInfo` / `RedeemByTelegram` / `ResetPassword` / `SubscribeByTelegram` 命中 `ErrTelegramNotBound` 时统一返回 400 + `请求参数错误`，不再透传 sentinel 字面值；攻击者无法借 `/bind`、`/redeem`、`/resetpw` 等命令枚举 Telegram↔Ember 的绑定关系。具体非枚举业务错误（兑换码无效、密码长度不够等）继续按各自 sentinel 返回。

**订阅管理员消息同步**：订阅审批通知接收人来自设置项 `telegram_approval_admin_ids`，语义是显式 Telegram 审批人员 user_id 列表；为空时回退 `TELEGRAM_ADMIN_CHAT_ID`，不会从 Telegram 群管理员或 Ember 后台 `role=admin` 推导。Bot 对每个审批人员私聊发送待审批消息，返回 `adminTelegramId/chatId/messageId/hasPhoto/deliveryStatus`，API 写入 `subscription_admin_notifications`。Web 后台或 Telegram 任一端审批成功后，API 调 `POST /notify/subscription-admin-sync`，Bot 逐条编辑消息为最终结果并移除按钮；编辑失败只写回 `edit_failed/deleted`，不回滚订阅审批状态。

**审批拒绝上下文持久化**：Bot 管理员拒绝订阅时，待输入的 `adminUserId / subscriptionId / messageId / hasPhoto / originalText / expiresAt` 已落到 `bot_pending_reject_requests`，避免 Bot 重启或滚动发布导致 5 分钟内的待输入状态丢失；第二步提交拒绝原因时，Bot 调用 `reject-request/pop` 必须同时提交 `chatId + adminUserId`，API 只弹出同一操作者创建的待确认记录；搜索交互 `message_id` 仍保留为 10 分钟 TTL 的私聊会话态边界，只用于校验用户是否在操作最新一条搜索消息。

### 5.19 TVCalendarService (`services/tvcalendar/service.go`)

追剧日历聚合服务，主链路改为“Emby 全库发现 + 周历同步 + Webhook 点亮 + 读时纠偏”，TMDB 仍使用三层缓存（内存 + PostgreSQL + TMDB）。

- `DiscoverContinuingSeries(ctx)` — 从 Emby 自动发现所有 `Continuing` 且带 `Tmdb` Provider ID 的剧集
- `SyncWeeklyCalendar(ctx, weekOffset, tmdbId, force)` — 按指定周偏移同步周历缓存；默认同步当前周与下周，并优先同步最近 30 天活跃剧，单剧源 TMDB/Emby 异常会记录日志并跳过，不再中断整批同步
- `GetGlobalWeeklyCalendar(ctx, weekOffset, status)` — 查询全局周历视图（只读当前缓存/数据库，不触发即时同步）
- `GetFollowingWeeklyCalendar(ctx, userID, weekOffset, status)` — 查询当前用户的关注周历视图（只读当前缓存/数据库，不触发即时同步）
- `FetchCalendar(userID, startDate, endDate, status)` — 兼容旧平铺接口，底层仍复用新的全局缓存数据
- `Subscribe(userID, tmdbId, showName, posterUrl)` — 创建或更新用户关注
- `GetSubscriptions(userID)` — 获取用户关注列表
- `Unsubscribe(userID, tmdbId)` — 取消关注
- `SyncCalendar(weekOffsets, tmdbId, force)` — 管理员手动同步（单剧 / 全部 / 指定周）；默认优先同步最近 30 天活跃剧，`force=true` 时回退全量
- `MarkEpisodeReadyByWebhook(...)` — Emby Webhook 将剧集状态点亮为 `ready`
- 周历查询阶段会按 `CRON_TIMEZONE` 实时归一非 `ready` 状态，避免 `upcoming/today/missing` 因 UTC 边界或缓存未刷新而长期滞后
- 当前周可见范围内，读链路会按 `seriesId` 对非 `ready` 条目做轻量物理校验；若 Emby 已存在对应季集，会即时纠正为 `ready` 并回写数据库
- 同步和 Webhook 都会刷新 `lastEpisodeIngestedAt`，保证仍在更新的剧源不会被增量同步窗口错误跳过
- Webhook 只信任显式 `SeriesId` 字段，`ParentId` 不参与 `seriesId` 持久化，避免季节点误写污染追剧源

### 5.20 PlaybackHistoryService (`services/playback/history.go`)

管理员播放历史查询服务，复用 Emby Playback Reporting 插件能力，支持分页和条件筛选。

- `GetPlaybackHistory(req)` — 支持 `username` / `keyword` / `startDate` / `endDate` / `page` / `pageSize`，兼容旧 `userId`
- 对 `keyword` 做白名单校验并转义，避免 SQL 注入
- 统一输出播放时长格式（`Xm` / `Xh Ym`）
- 插件不可用时返回统一错误：`Playback Reporting 查询失败`
- 与旧兼容逻辑相比，当前不再按列位置猜字段，也不再把查询退化成“全量拉回后本地分页”；当 Playback Reporting 返回缺列或不受支持 schema 时，接口显式返回兼容错误

### 5.21 UserPlaybackProfileService (`services/playback/profile.go`)

管理员/用户播放画像聚合服务，基于单个用户的 `PlaybackActivity` 记录输出摘要、分布和勋章结果。

- `GetUserProfile(ctx, userID, query)` — 支持 `range=today|7d|30d|90d|all`，也支持 `startDate/endDate` 自定义日期时间范围
- 先读取本地 `users` 表映射 `embyId`，未绑定时回退使用本地 `userId`
- 输出指标：`totalPlayCount` / `totalPlayDuration` / `activeDays` / `averagePlayDuration` / `lastPlayedAt`
- 输出分布：`hourlyDistribution` / `deviceDistribution` / `clientDistribution`
- 输出最近记录预览：`recentRecords`（最多 10 条）
- 画像标签包含行为标签和高阈值勋章，例如：`evening_viewer` / `steady_viewer` / `night_owl` / `weekend_warrior` / `hardcore_viewer`
- 自定义日期时间范围最大跨度限制为 `92` 天
- 关键日志包含 `userID` / `embyUserID` / `range` / 结果统计 / 耗时，便于排障

### 5.22 UserPlaybackProfileOverview (`services/playback/profile_list.go`)

管理员侧用户画像总览聚合能力，按用户维度汇总指定时间窗口内的播放活跃度，并输出分页列表。

- `GetUserProfilesOverview(ctx, query)` — 支持 `range`，也支持 `startDate/endDate` 自定义日期时间范围，并支持 `keyword` / `sortBy` / `sortOrder` / `page` / `pageSize`
- 聚合结果字段：`totalPlayDuration` / `totalPlayCount` / `activeDays` / `lastPlayedAt` / `peakHourLabel`
- 总览页标签只返回精简预览（默认前 2 个），避免列表噪音
- 默认按累计播放时长倒序，可切换按播放次数、活跃天数、最近播放排序
- 总览摘要包含：`userCount` / `totalPlayCount` / `totalPlayDuration`
- 自定义日期时间范围最大跨度限制为 `92` 天，和单用户画像保持一致
- 查询策略已收敛为“全量聚合 + 当前页明细补充”：先按 `UserId` 聚合出总览摘要并排序分页，再只为当前页用户补拉明细计算 `peakHourLabel` 与 badge 预览；不再先拉全量明细再分页

### 5.23 MediaQualityService (`services/media_quality.go`)

管理员媒体库质量盘点服务，按媒体库维度（支持 `libraryId=all`）聚合分辨率、编码、HDR 分布，并输出低画质汇总清单。

- `GetLibraryQuality(ctx, libraryID, force)` — 缓存命中优先（`force=false`），否则触发扫描
- `ScanLibraryQuality(ctx, libraryID)` — 拉取媒体库条目并生成质量报告
- `GetGroupLowQualityDetails(ctx, libraryID, groupID, force)` — 按汇总分组下钻低画质明细
- 缓存模型：`media_quality_caches`（PostgreSQL 持久化，按 `libraryId` 唯一）
- force 扫描使用 `inflightUntil` 做进程间互斥，残留 inflight 由 cron 清理；清理失败会记日志，避免静默残留
- `libraryId=all` 路径单库失败时保留已成功媒体库结果，并在报告中返回 `failedLibraries`
- 低画质清单按“影片/剧集”汇总，避免电视剧按单集展开造成噪音
- 低画质汇总项包含 `groupId`，前端使用 `groupId` 请求下钻接口
- 报告字段：`resolutionDistribution` / `codecDistribution` / `hdrDistribution` / `lowQualityItems` / `lowQualityTotal` / `page` / `pageSize` / `scanAt`

### 5.24 P115AccountService 与 Cookie HTTP 适配器 (`services/p115account/`, `integrations/p115/`)

当前已落地 115 Cookie 模式的账号控制面，以及凭证验证、上传信息、源路径解析、旧 SHA1 查重、秒传初始化、目标目录复核、下载 URL、受限 Range Hash 和串行删除的完整 Provider 合同适配；尚未实现播放网关或调用真实 115 账号：

- 管理 API：`/api/v1/admin/p115-accounts` 提供列表、详情、创建、Cookie 替换、显式验证和启停；全部只允许管理员 JWT，Admin API Key 返回 `403`
- 管理 Web：`/console/p115-accounts` 提供安全摘要、创建、Cookie 替换、显式验证和启停；Cookie 输入不会从查询结果回填，提交成功或关闭弹窗后立即从页面状态清空
- `Create(ctx, input)`：校验 `source` / `playback` 角色字段，使用 `CONFIG_ENCRYPTION_KEY` 加密 Cookie，账号以 `pending + disabled` 创建
- `ReplaceCookie(ctx, accountID, cookie)`：覆盖密文并清空 Provider 用户、验证时间、冷却和错误状态，重新回到 `pending + disabled`
- `Validate(ctx, accountID)`：调用固定的登录状态端点；成功进入 `active` 但不自动启用，凭证失效进入 `expired + disabled`，网络或协议失败进入 `error`；回写按 Cookie 密文做乐观并发检查
- `SetEnabled(ctx, accountID, enabled)`：事务行锁内要求 `active + providerUserId + lastValidatedAt`，并由 partial unique index 保证每个角色至多一个启用账号、源账号与播放账号不能是同一 Provider 用户
- `LoadCredentialForValidation(ctx, accountID)`：仅供显式账号验证读取待验证凭证
- `LoadActiveCredential(ctx, accountID)`：只允许读取 `enabled + active` 账号，防止播放链路误用未验证 Cookie
- `integrations/p115.CookieCredentialValidator`：固定请求 `GET https://my.115.com/?ct=guide&ac=status`，严格解析布尔 `state` 并从 Cookie `UID` 规范化 Provider 用户 ID；测试使用 fake HTTP server，不访问真实 115
- `integrations/p115.CookieProvider`：组合 `CookieCredentialValidator` 与 `CookieHTTPAdapter`，通过编译期断言完整实现 Provider-neutral 接口；生产账号控制面注入该对象的验证边界，后续 direct play Service 可复用同一具体 Provider
- `integrations/p115.CookieHTTPAdapter.GetUploadInfo`：固定请求上传信息端点，严格映射顶层 `user_id` / `userkey`，并要求响应用户与 Cookie UID 一致
- `integrations/p115.CookieHTTPAdapter.ResolveFileByPath`：在显式 root 下逐级分页列举 `/files`，精确匹配相对路径目录和最终文件名/size，返回完整源文件身份；无效 cid 回退、分页漂移、重名和单级超过 10,000 项全部失败关闭
- `integrations/p115.CookieHTTPAdapter.SearchBySHA1`：只向旧接口发送规范化 SHA1，在本地复核 size、非目录和可选父目录；固定未命中映射为空列表
- `integrations/p115.CookieHTTPAdapter.InitRapidUpload`：校验完整内容身份后获取账号上传信息，调用 `p115cipher.BuildUploadRequest` 生成 `k_ec` 与加密 body，并把 `status=1/2/7` 映射为普通上传拒绝、复用和有界 Range challenge
- `integrations/p115.CookieHTTPAdapter.FindTargetFile`：`status=2` 后立即查询目标目录，只接受 SHA1、size、非目录和 parent 全部一致的唯一候选；每 500ms 轮询、10s 截止并执行最终查询，超时和多候选使用独立错误
- `integrations/p115.CookieHTTPAdapter.GetDownloadURL`：使用真实播放客户端 UA 请求 Chrome downurl，RSA 解密单文件响应，并严格映射 HTTPS 115 域名 allowlist、UTC 过期时间、并发限制和 `f` HeaderMode；allowlist 包含 `115.com` 完整域边界及 2026-08-22 真实验证后精确加入的 `cdnfhnfile.115cdn.net`，URL 不通过 JSON 或错误回显
- 下载 URL 策略拒绝使用类型化安全证据，仅允许一次性检查器读取固定 reason、scheme 和受限 hostname；普通日志/API 仍只接收通用 sentinel，永不包含 URL path、query、签名、端口值、userinfo 或 IP literal
- `integrations/p115.CookieHTTPAdapter.HashFileRange`：使用源账号配置 UA 获取签名 URL，按 HeaderMode 在 Provider 内发起最大 1 MiB 的 Range GET，只接受精确 `206/Content-Range/Content-Length` 并只返回大写 SHA1 与读取字节数
- `integrations/p115.CookieHTTPAdapter.DeleteFile`：只发送单个规范化 file ID，并按 Cookie Provider UID 使用进程内共享锁串行删除；不同 UID 可并行，等待支持 context 取消
- Cookie HTTP 公共边界：10 秒超时、禁止跟随重定向、响应体限额、网络/HTTP/业务/协议错误分层；错误不包含 Cookie、URL、响应正文或上游原始文本
- `integrations/p115/p115cipher`：基于固定提交黑盒输出独立实现上传 `k_ec`/AES-CBC/LZ4、完整签名表单和下载 RSA request/response 变换；固定向量不含真实账号信息
- `integrations/p115.Provider`：账号验证、上传信息、源路径解析、SHA1 搜索、秒传初始化、目标复核、下载地址、受限 Range Hash 和串行删除的 Cookie 实现均已有 fake HTTP 合同；2026-08-22 本地真实只读检查已通过账号、上传信息、源解析、playback 查重、source downurl 和 128 KiB Range，playback 最终直链与写入型链路仍未验证
- `cmd/p115-contract-check`：只组合 Provider 的六个只读方法；凭证仅从当前进程环境读取，CI 或缺少确认值时拒绝运行，报告不包含可复用凭证、完整文件身份或签名 URL
- `security/secretbox`：复用 ConfigService 历史 AES-GCM 密文格式，已有 settings 密文保持兼容；115 Cookie 通过 `p115-cookie` purpose 派生独立密钥，禁止与 settings 密文跨用途替换
- `p115_accounts` 存储使用局部静默 GORM session，避免 PostgreSQL 失败行详情携带 Cookie 密文；错误日志只保留操作名、SQLSTATE 和约束名
- 数据库约束：`source` / `playback` 每个角色至多一条启用记录；同一 Provider 用户不能同时成为两个启用角色；播放账号必须有目标目录

---

## 6. API 端点总览

完整路由目录、分组和用途已迁移到 [docs/reference/api-endpoint-catalog.md](./reference/api-endpoint-catalog.md)。

本节只保留系统入口级摘要。

### 6.1 路由分组

- 公开路由：登录、注册、验证码发送、Webhook
- 统一认证路由：当前登录用户可访问的个人信息、订阅、TMDB 搜索代理、兑换、支付、追剧日历、排行等能力
- 用户路由：保留 `/user/*` 兼容别名
- 管理员路由：用户管理、单用户 Emby Policy 同步失败重试、配置中心、Admin API Key 管理、支付与兑换后台、媒体质量、设备、追剧日历同步、cron 手动触发
- 内部服务路由：Bot 通过 `InternalAuth` 访问的审批、配置、媒体统计、TMDB 代理和 Telegram 内部能力

### 6.2 关键约束

- 列表接口统一使用 `data` 字段
- 字段命名统一使用 camelCase
- 返回格式约定以 [docs/reference/api-response-standard.md](./reference/api-response-standard.md) 为准
- 完整路径清单和分组用途统一维护在 [docs/reference/api-endpoint-catalog.md](./reference/api-endpoint-catalog.md)

---

## 7. 认证与授权

- **JWT**：HS256，7 天有效期，Claims = {userID, username, role, pwdSig}
- **Token 传递**：`Authorization: Bearer {token}`
- **用户 / 统一认证路由中间件链**：`JWTAuth()` → `PasswordResetRequired()` → `UserOnly()`（如需用户角色）
- **管理员路由中间件链**：`AdminCredentialAuth()` → `AdminOnly()`；其中 JWT 分支仍执行用户状态、角色、密码签名和密码重置闭环校验，Admin API Key 分支只校验 `external_api_key_hash`
- **会话状态收口**：`PasswordResetRequired()`（`middleware/password_reset_required.go`）是 `JWTAuth` 之后每请求必经的数据库回查点，承担会话失效语义：
  - 账号被管理员停用（`is_active=false`）→ 401，旧 token 立即失效
  - JWT 内 `role` 与数据库实际 `role` 不一致（被升/降级）→ 401，强制重新登录换新 token；因此该中间件之后下游可信任 context `role`
  - JWT 内 `pwdSig` 与数据库当前密码哈希重新计算出的签名不一致（用户改密 / 管理员重置 / Telegram 重置 / 邮箱找回重置后）→ 401，旧 token 立即失效
  - 被标记 `password_reset_required` 的账号只能访问改密闭环白名单接口
  - 仅校验 Ember 账号状态 `is_active`；**不校验 `emby_disabled` / 过期**，过期或 Emby 侧被停用的用户仍可登录控制台续费/兑换
- **登录态校验**：`AuthService.authenticateLoginUser` 在凭据校验前先拒绝 `is_active=false` 账号（返回与凭据错误一致文案），阻止停用账号重新登录换取新 JWT
- **Admin API Key**：`Authorization: Bearer ember_sk_xxx` 只允许进入 `/api/v1/admin/*`；未配置、格式错误或 hash 不匹配返回 401，配置读取失败返回 500；日志只记录 `authType=api_key`、路径、方法、客户端 IP 和失败原因，不输出明文或 hash
- **InternalAuth**：`middleware/internal_auth.go` — 校验 `X-Internal-Secret` header，用于 Bot ↔ API 内部通信；`INTERNAL_API_SECRET` 在 API 与 Bot 启动期均要求非空、长度至少 32 字符，并拒绝示例占位值
- **Context 变量**：`userID`, `username`, `role`, `pwdSig`, `claims`, `principal`, `authType`
- **密码存储**：bcrypt（DefaultCost），所有用户统一存本地 hash
- **存量迁移**：`Password == ""` 时降级 Emby 认证，成功后自动补存本地 hash

---

## 8. 前端架构总览

完整的共享组件层、状态管理、路由守卫、页面职责与兼容路由已迁移到 [docs/reference/web-information-architecture.md](./reference/web-information-architecture.md)。

本节只保留系统入口级摘要。

### 8.1 前端分层

- Store：基于 Pinia 维护认证态、用户态、管理员态
- API：`request.ts` 负责 token 注入和 401 收口，各业务模块按职责拆分
- Router：通过 `requiresAuth / role` 守卫做鉴权和 redirect 收口；刷新后先用 token 拉 `/profile`，再以服务端返回的 `role / passwordResetRequired` 判断 UI 权限
- View：页面继续保留接口调用、路由状态、筛选参数和弹窗编排
- Shared Components：`components/ember/` 承载稳定 UI 契约，不侵入业务
- Build Metadata：`components/common/ProjectSourceLink.vue` 读取 Vite 构建期注入的 GitHub 仓库与 commit SHA，在首页导航和控制台侧边栏展示源码入口；控制台保留低干扰当前构建短 hash

### 8.2 高层页面边界

- 用户侧重点页面：Dashboard、Renewal、Subscriptions、TV Calendar、Rankings、Profile Analytics
- 管理侧重点页面：Users、Playback Center、Payment Center、Redemption Center、Media Quality、Devices、Settings
- 页面级职责、Tab 结构、兼容路由和关键数据源统一维护在 [docs/reference/web-information-architecture.md](./reference/web-information-architecture.md)

---

## 9. Telegram Bot 架构总览

完整的运行模式、端点、命令处理器与环境变量说明已迁移到 [docs/reference/bot-architecture-reference.md](./reference/bot-architecture-reference.md)。

本节只保留系统入口级摘要。

### 9.1 运行边界

- Bot 使用 Python 3.11 + python-telegram-bot + FastAPI，支持 `webhook` / `polling` 双模式
- 与 Go API 通过 `X-Internal-Secret` 做双向内部通信
- API → Bot 通过 `BotNotifier` 火忘式推送通知；Telegram 用户交互则通过 Bot 再调用 Go Internal API

### 9.2 关键约束

- `polling` 模式启动前会通过 Internal API 申请 `bot_runtime_locks(name='telegram_polling')` 租约锁，避免多副本重复消费
- `webhook` 模式注册失败达到最大重试次数后，`GET /health` 返回 `degraded`
- 详细端点、命令、群菜单策略和环境变量语义统一维护在 [docs/reference/bot-architecture-reference.md](./reference/bot-architecture-reference.md)

---

## 10. 定时任务

| 任务 | 调度 | 控制变量 | 说明 |
|------|------|----------|------|
| 过期用户检查 | `CRON_SCHEDULE`（默认 `0 2 * * *`）| `CRON_ENABLED` | 封禁过期 Emby 账号 |
| 验证码清理 | `0 3 * * *` | `CRON_ENABLED` | 删除过期 EmailVerification + TelegramBindCode |
| 日榜生成 | `RANKING_DAILY_SCHEDULE`（默认 `0 20 * * *`）| `RANKING_CRON_ENABLED` | 从 Emby 生成日播放排行 |
| 周榜生成 | `RANKING_WEEKLY_SCHEDULE`（默认 `30 20 * * 0`）| `RANKING_CRON_ENABLED` | 从 Emby 生成周播放排行 |
| 追剧日历同步 | `TV_CALENDAR_SYNC_SCHEDULE`（默认 `0 */12 * * *`） | `CRON_ENABLED` | 同步 TMDB/Emby 追剧日历缓存 |
| Emby Policy 同步 | `@every 1m` | `CRON_ENABLED` | 领取 pending Policy 同步任务，回收超时 processing 任务 |

补充说明：
- API 启动后默认会在 `15s` 后额外执行一次追剧日历补偿同步，用于预热周历缓存。
- API 启动后默认会在 `15s` 后额外执行一次 Emby Policy 同步补偿，用于回收上次进程中断遗留的 processing 任务。
- 单用户 Emby Policy 同步失败以 `failed` 终态保留给管理员处理；覆盖后台 Emby 启停、用户分组变更、过期封禁、支付履约和兑换续期等账号状态变更；管理员可在用户管理中手动重试，成功后旧失败任务会被收口为 `synced`。
- 追剧日历启动补偿由 `TV_CALENDAR_STARTUP_SYNC_ENABLED` 控制，默认 `"true"`；关闭后不影响 `TV_CALENDAR_SYNC_SCHEDULE` 对应的定时同步。
- `CRON_TIMEZONE` 是 Ember 唯一的全局业务时区，统一作为调度、日期边界、排行榜、播放记录、追剧日历状态和用户可见时间的判定基线。

**通用配置**：
这些项由 `ConfigService` 统一解析，优先级为“数据库覆盖值 > 环境变量 > 默认值”；管理员可在设置中心修改，但属于启动期配置，保存后需重启 API 才会生效。

| 配置项 | 默认值 | 说明 |
|----------|--------|------|
| `CRON_ENABLED` | `"true"` | 是否启用（过期检查 + 验证码清理 + 追剧日历同步 + Emby Policy 同步）|
| `CRON_SCHEDULE` | `"0 2 * * *"` | 过期检查 cron 表达式 |
| `CRON_TIMEZONE` | `"Asia/Shanghai"` | Ember 全局业务时区；统一用于调度、日期边界、排行榜、播放记录、追剧日历状态和用户可见时间 |
| `RANKING_CRON_ENABLED` | `"false"` | 是否启用排行榜生成 |
| `RANKING_DAILY_SCHEDULE` | `"0 20 * * *"` | 日榜 cron 表达式 |
| `RANKING_WEEKLY_SCHEDULE` | `"30 20 * * 0"` | 周榜 cron 表达式 |
| `TV_CALENDAR_STARTUP_SYNC_ENABLED` | `"true"` | 是否启用 API 启动后的追剧日历补偿同步 |
| `TV_CALENDAR_SYNC_SCHEDULE` | `"0 */12 * * *"` | 追剧日历自动同步表达式 |

**过期检查逻辑**：查询 `expiresAt < NOW() AND embyDisabled = false` → Emby `SetUserPolicy(IsDisabled: true)` → 设置 `EmbyDisabled = true`。不修改 IsActive，不阻止用户登录。

---

## 11. 配置与环境变量边界

本节只保留配置分层和系统边界。完整变量字典、敏感性、优先级和生效方式统一以 [docs/reference/configuration-reference.md](./reference/configuration-reference.md) 为准。

### 配置分层

| 层级 | 典型项 | 真相源 | 说明 |
|------|--------|--------|------|
| API 运行期数据库配置 | `registration_mode`、`EMBY_URL`、`SMTP_*`、`CRON_*`、`external_api_key_hash` | [configuration-reference](./reference/configuration-reference.md) | 由设置中心统一解析；大多数可运行期生效，调度相关配置改后需重启 API |
| API 部署期环境变量 | `DATABASE_URL`、`JWT_SECRET`、`CONFIG_ENCRYPTION_KEY`、`INTERNAL_API_SECRET`、`EMBY_WEBHOOK_TOKEN` | [configuration-reference](./reference/configuration-reference.md) | 作为启动边界或信任根，不放进设置中心 |
| Bot 启动环境变量 | `TELEGRAM_BOT_TOKEN`、`TELEGRAM_UPDATE_MODE`、`WEBHOOK_URL` | [configuration-reference](./reference/configuration-reference.md) | Bot 进程启动直接读取；部分 Chat ID 支持通过 API 设置中心回读并以 env 兜底 |
| Web 构建期变量 | `VITE_GIT_COMMIT_SHA`、`VITE_GITHUB_REPOSITORY`、`VITE_GITHUB_REPOSITORY_URL` | [configuration-reference](./reference/configuration-reference.md) | 仅用于静态前端构建元信息展示，不属于运行期配置 |

### 关键约束

- 配置解析优先级固定为：数据库覆盖值 > 环境变量 > 代码默认值
- `JWT_SECRET`、`INTERNAL_API_SECRET`、`STRIPE_WEBHOOK_SECRET`、`CONFIG_ENCRYPTION_KEY` 属于边界密钥，只允许通过环境变量提供
- `.env` 默认填写方式与 compose 入口以 `infrastructure/docker/.env.example` 和部署 runbook 为准，不再在本文件重复维护全量变量表
- Stripe Dashboard 仍是支付方式能力的真实来源；系统设置中的 `stripe_allowed_payment_methods` 仅用于进一步限制 Checkout 可展示的支付方式

---

## 12. 外部集成汇总

| 服务 | 用途 | 配置变量 |
|------|------|----------|
| **Emby API** | 用户创建/认证/封禁/解封、媒体统计、播放活动 | `EMBY_URL`, `EMBY_API_KEY` |
| **TMDB API** | 电影/电视剧搜索（求片功能）| `TMDB_API_KEY` |
| **MoviePilot API** | 自动订阅下载（审批后触发）| `MOVIEPILOT_URL`, `MOVIEPILOT_API_KEY` |
| **Stripe API** | 一次性支付（Checkout Session + Webhook）| `STRIPE_SECRET_KEY`, `STRIPE_WEBHOOK_SECRET` |
| **SMTP** | 邮箱验证码发送 | `SMTP_HOST/PORT/USERNAME/PASSWORD` |
| **Telegram Bot API** | 通知推送、订阅审批、账号绑定/查询/续期 | `TELEGRAM_BOT_TOKEN` 等（见 Bot 章节）|
| **115 Cookie/Web API** | 直连播放 Cookie Provider；账号控制面和完整离线协议 Adapter 已实现，播放网关与真实账号验证尚未完成 | Cookie 密文在 `p115_accounts`，根密钥为 `CONFIG_ENCRYPTION_KEY` |

---

## 13. 部署拓扑与关键约束

本节只保留部署拓扑和关键边界。具体部署步骤、环境变量填写和排障动作统一以 runbook 为准：

- [docs/runbooks/deployment.md](./runbooks/deployment.md)
- [docs/runbooks/deployment-environment.md](./runbooks/deployment-environment.md)
- [docs/runbooks/deployment-troubleshooting.md](./runbooks/deployment-troubleshooting.md)

### 部署拓扑

- 默认部署拓扑为：PostgreSQL 16 + Go API + Vue Web + Telegram Bot（`profiles: ["bot"]` 控制，默认不启动）
- Compose 主入口为 `infrastructure/docker/docker-compose.yml`
- API 与 Web 使用独立镜像；Bot 按 profile 显式启用
- PostgreSQL 默认仅监听 `127.0.0.1:5432`；远程访问应通过 SSH tunnel 或受控反代

### 关键约束

- `POSTGRES_USER`、`POSTGRES_PASSWORD`、`JWT_SECRET`、`CONFIG_ENCRYPTION_KEY`、`INTERNAL_API_SECRET` 是 compose 启动硬依赖；启用 Bot 时还要求 `TELEGRAM_BOT_TOKEN`
- `DATABASE_URL` 缺省时由 compose 按 `POSTGRES_USER/PASSWORD/DB` 自动拼接到内置 postgres，外部覆盖路径保留
- API 容器以非 root 用户 `ember:ember` 运行，健康检查使用 `GET /health`
- 数据库 schema 初始化与升级全部由 API 启动期 `Migrate` 阶段接管；启动序列固定为 `InitDB → Migrate → VerifySchema → Bootstrap → Start`
- 启动期迁移依赖 `schema_migrations` 记账、`pg_advisory_lock` 串行和 checksum 防改写；支持窗口内升级路径已收口为 `docker compose pull && up -d`
- 当前直接升级支持起点是 `2026-06-05` / v1.6.0 截点；`archive/` 不参与运行时链路，旧于该截点且未执行过已归档增量的数据库不承诺直接跳升
- 数据库迁移资产、baseline 和归档边界以 [`infrastructure/database/README.md`](../infrastructure/database/README.md) 为准
- 数据库连接池基线：MaxIdle=15、MaxOpen=30、MaxLifetime=1h、MaxIdleTime=10min
- 时间处理约束：所有时间戳 UTC 存储（GORM `NowFunc` 强制 UTC）

---

## 14. 代码模式入口

常见实现约定不再单独维护碎片速查页，而是按边界收口到现行参考文档：

- handler / service 分工、火忘通知、上游错误脱敏、ID/密码生成约定：看 [docs/reference/api-development-conventions.md](./reference/api-development-conventions.md)
- 列表响应、错误响应、操作结果格式：看 [docs/reference/api-response-standard.md](./reference/api-response-standard.md)
- Internal API / Bot / 配置来源边界：看 [docs/reference/configuration-reference.md](./reference/configuration-reference.md) 与 [docs/reference/bot-architecture-reference.md](./reference/bot-architecture-reference.md)
