# Ember 系统架构文档

> 本文档记录 Ember 系统的完整架构、数据模型、服务逻辑和 API 端点。
> 供 AI 协作时快速加载系统上下文，避免重复探索代码。

---

## 1. 系统概览

Ember 是一个 Emby 媒体服务器的用户管理系统，提供：
- 用户注册/登录（可配置开放注册或兑换码注册，支持邮箱验证码）
- Emby 账号自动创建与生命周期管理（试用 → 续期 → 过期封禁）
- 兑换码系统（注册门控 + 续期工具，统一模型；同一用户同一码仅可兑换一次）
- 付费方案与 Stripe 一次性支付
- 求片订阅（TMDB 搜索 → 管理员审批 → MoviePilot 自动下载）
- 播放排行榜（日榜 / 周榜，从 Emby PlaybackActivity 生成）
- Telegram Bot（订阅审批、新用户通知、排行榜推送、欢迎消息、账号绑定/查询/续期）
- 定时任务（过期检查、验证码清理、排行榜生成）

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

## 3. 目录结构

```
services/
├─ api/                          # Go 后端
│  ├─ cmd/server/main.go         # 入口：路由注册 + cron 初始化
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
│     │  ├─ payment.go           # Payment（支付记录）
│     │  ├─ playback_ranking.go  # PlaybackRanking（播放排行快照）
│     │  ├─ media_quality_cache.go # MediaQualityCache（媒体质量缓存）
│     │  ├─ client_blacklist.go  # ClientBlacklist（客户端黑名单）
│     │  ├─ device_action.go     # DeviceAction（设备操作日志）
│     │  ├─ tv_calendar.go       # TVCalendar（追剧日历 + 订阅 + TMDB 缓存）
│     │  └─ utils.go             # generateCUID()
│     ├─ integrations/           # 外部系统集成
│     │  ├─ emby/
│     │  │  ├─ emby.go           # Emby HTTP 客户端
│     │  │  └─ library.go        # Emby 媒体库列表/条目查询
│     │  ├─ moviepilot/
│     │  │  └─ client.go         # MoviePilot HTTP 客户端
│     │  └─ notifier/
│     │     └─ notifier.go       # BotNotifier（火忘式推送通知给 Bot）
│     ├─ services/               # 业务逻辑
│     │  ├─ auth.go              # 登录 / 注册
│     │  ├─ user.go              # 用户 CRUD + 密码管理
│     │  ├─ redemption_code.go   # 兑换码 CRUD
│     │  ├─ redemption.go        # 兑换核心逻辑 + 历史
│     │  ├─ system.go            # 系统信息 + 过期检查
│     │  ├─ media.go             # 媒体统计（带 5min 缓存）
│     │  ├─ media_quality.go     # MediaQualityService（媒体质量盘点）
│     │  ├─ subscription.go      # 订阅工作流
│     │  ├─ email.go             # EmailService（邮箱验证码发送/校验/清理）
│     │  ├─ telegram.go          # TelegramService（绑定/查询/续期）
│     │  ├─ playback/
│     │  │  ├─ history.go        # PlaybackHistoryService（播放历史查询）
│     │  │  └─ ranking.go        # PlaybackRankingService（播放排行生成）
│     │  ├─ payment/
│     │  │  └─ service.go        # PaymentService（Stripe 支付流程）
│     │  ├─ device.go            # DeviceService（设备管理）
│     │  ├─ tvcalendar/
│     │  │  └─ service.go        # TVCalendarService（追剧日历）
│     │  ├─ *_errors.go          # 领域错误定义（按业务拆分）
│     │  ├─ playback_compat.go   # playback 子目录兼容导出
│     │  └─ tvcalendar_compat.go # tvcalendar 子目录兼容导出
│     ├─ handlers/               # HTTP 处理层（Gin）
│     │  ├─ auth.go              # 登录 / 注册
│     │  ├─ user.go              # 用户管理
│     │  ├─ redemption_code.go   # 兑换码管理
│     │  ├─ config.go            # 设置中心配置接口
│     │  ├─ setting.go           # 系统配置
│     │  ├─ system.go            # 系统信息
│     │  ├─ media.go             # 媒体信息
│     │  ├─ media_quality.go     # 媒体质量盘点
│     │  ├─ subscription.go      # 订阅管理
│     │  ├─ tmdb.go              # TMDB 搜索
│     │  ├─ ranking.go           # 播放排行
│     │  ├─ session.go           # 活跃会话
│     │  ├─ playback_history.go  # 播放历史
│     │  ├─ device.go            # 设备管理
│     │  ├─ telegram.go          # Telegram 绑定与 Bot Internal API
│     │  ├─ payment.go           # 支付与方案
│     │  └─ tv_calendar.go       # 追剧日历
│     ├─ middleware/
│     │  ├─ jwt.go               # JWTAuth + AdminOnly + UserOnly
│     │  └─ internal_auth.go     # InternalAuth（Bot 内部通信认证）
│     ├─ common/
│     │  ├─ jwt.go               # Token 生成/解析（HS256, 7天有效）
│     │  └─ utils.go             # CalculateExpiryDate
│     └─ db/
│        └─ db.go                # DB 初始化 + AutoMigrate + Seed
├─ web/                          # Vue 3 前端
│  ├─ src/
│  │  ├─ api/                    # Axios 请求层
│  │  │  ├─ request.ts           # 基础配置：baseURL=/api/v1, 401拦截
│  │  │  ├─ auth.ts              # login, register, getRegistrationMode
│  │  │  ├─ user.ts              # redeem, redemptions, tmdb
│  │  │  ├─ admin.ts             # 管理后台全部接口
│  │  │  └─ console.ts           # 统一认证路由接口（profile, subscriptions, payments, rankings 等）
│  │  ├─ types/api.ts            # 所有 TypeScript 接口定义
│  │  ├─ store/
│  │  │  ├─ auth.ts              # Pinia: token + role (localStorage 持久化)
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
│  │     │  ├─ DashboardView.vue # 面板（双态：活跃/过期降级）
│  │     │  ├─ SubscriptionsView.vue  # 求片订阅
│  │     │  ├─ NewSubscriptionView.vue # 新建订阅
│  │     │  ├─ TVCalendarView.vue # 追剧日历
│  │     │  ├─ LibraryView.vue   # 媒体库
│  │     │  ├─ RankingsView.vue  # 播放排行
│  │     │  └─ PricingView.vue   # 付费方案
│  │     └─ admin/               # 管理后台
│  │        ├─ UsersView.vue     # 用户管理
│  │        ├─ RedemptionCodesView.vue # 兑换码管理
│  │        ├─ RedemptionHistoryView.vue # 兑换历史
│  │        ├─ SettingsView.vue  # 设置中心
│  │        ├─ PlansView.vue     # 方案管理
│  │        ├─ SessionsView.vue  # 活跃会话
│  │        ├─ PlaybackHistoryView.vue # 播放历史
│  │        ├─ MediaQualityView.vue # 媒体质量盘点
│  │        └─ DevicesView.vue   # 设备管理
│  ├─ vite.config.ts             # dev:3000, proxy /api→:8080
│  └─ tailwind.config.js         # 自定义色：ember(橙红), cinema
└─ bot/                          # Python Telegram Bot
   ├─ main.py                    # 入口
   ├─ requirements.txt           # Python 依赖
   ├─ Dockerfile                 # 容器构建
   └─ app/
      ├─ config.py               # 启动期环境变量加载
      ├─ runtime_settings.py     # Bot 运行期设置读取（API + TTL 缓存）
      ├─ server.py               # FastAPI + Telegram Application（Webhook 模式）
      ├─ handlers/
      │  ├─ telegram_handler.py  # 消息/回调处理（审批、欢迎消息）
      │  └─ search_cache.py      # 搜索会话缓存
      ├─ formatters/
      │  └─ message_formatter.py # Telegram 消息格式化
      └─ clients/
         └─ api_client.py        # Ember API 内部客户端
```

---

## 4. 数据模型

### 4.1 User

**表名**: `users` | **文件**: `models/user.go`

| 字段 | 类型 | 列名 | 说明 |
|------|------|------|------|
| ID | string(25) | id | CUID 主键 |
| Username | string(50) | username | 唯一索引 |
| Role | string(10) | role | `"admin"` 或 `"user"` |
| Password | string | password | bcrypt hash（JSON 隐藏） |
| Email | string(255) | email | 唯一索引 |
| EmbyID | string(50) | embyId | Emby 用户 ID |
| EmbyDisabled | bool | embyDisabled | cron 封禁标记 |
| TelegramID | *int64 | telegramId | Telegram 绑定 ID（唯一，可空） |
| ExpiresAt | *time.Time | expiresAt | 到期时间（nil=永不过期）|
| IsActive | bool | isActive | 管理员手动开关 |
| CreatedAt | time.Time | createdAt | 自动 |
| UpdatedAt | time.Time | updatedAt | 自动 |

**方法**：
- `SetPassword(pwd)` / `CheckPassword(pwd)` — bcrypt
- `IsExpired()` — `ExpiresAt != nil && ExpiresAt < now`
- `IsAdmin()` — `Role == "admin"`

**设计要点**：
- `IsActive` 是管理员手动控制的"人工开关"
- `EmbyDisabled` 是 cron 自动管理的"过期封禁状态"
- 两者正交，互不干扰

### 4.2 RedemptionCode

**表名**: `redemption_codes` | **文件**: `models/redemption_code.go`

| 字段 | 类型 | 列名 | 说明 |
|------|------|------|------|
| ID | string(25) | id | CUID |
| Code | string(20) | code | 唯一，16 字符 hex |
| MaxUses | int | maxUses | 最大使用次数（默认 1）|
| UsedCount | int | usedCount | 已使用次数（默认 0）|
| ExpiresAt | *time.Time | expiresAt | 码本身的过期时间 |
| DefaultDays | int | defaultDays | 每次兑换授予的天数（默认 30）|
| TemplateUserID | *string(25) | templateUserId | 模板用户 ID（可空，仅邀请码注册时生效）|
| CreatedAt | time.Time | createdAt | 自动 |

**方法**：`IsValid()` — `UsedCount < MaxUses && (ExpiresAt == nil || ExpiresAt > now)`

**双重角色**：
- `registration_mode = "invite"` 时：注册门控（必须提供码才能注册）
- 已注册用户：续期工具（兑换码延长有效期）

### 4.3 Redemption（兑换历史）

**表名**: `redemptions` | **文件**: `models/redemption.go`

| 字段 | 类型 | 列名 | 说明 |
|------|------|------|------|
| ID | string(25) | id | CUID |
| UserID | string(25) | userId | 用户 ID（有索引）|
| Code | string(20) | code | 使用的兑换码 |
| Days | int | days | 兑换天数 |
| CreatedAt | time.Time | createdAt | 自动 |

### 4.4 Setting（系统配置）

**表名**: `settings` | **文件**: `models/setting.go`

| 字段 | 类型 | 列名 | 说明 |
|------|------|------|------|
| Key | string(100) | key | 主键 |
| Value | text | value | 值或密文 |
| IsEncrypted | bool | isEncrypted | 是否为加密存储 |
| UpdatedByUserID | *string(25) | updatedByUserId | 最后修改人（可空） |
| UpdatedAt | time.Time | updatedAt | 自动 |

**设计要点**：
- `settings` 已扩展为设置中心的运行期存储层
- 配置解析优先级固定为：数据库覆盖值 > 环境变量 > 代码默认值
- 敏感配置可加密落库，不通过 API 明文回显
- 配置定义显式声明“空值语义”：关闭功能 / 回退到上游配置 / 跟随外部服务默认行为，不再靠模糊文案猜
- 只读部署边界项同时声明“只读原因”和“缺失影响”，前端直接展示，不再让管理员自己猜为什么不能改

**当前已托管或接入统一解析的配置项**：
- 业务配置：`registration_mode`、`default_trial_days`、`notify_group_link`、`email_verification`、`stripe_allowed_payment_methods`
- 媒体集成：`EMBY_URL`、`EMBY_API_KEY`、`NEXT_PUBLIC_EMBY_URL`、`TMDB_API_KEY`、`MOVIEPILOT_URL`、`MOVIEPILOT_USERNAME`、`MOVIEPILOT_PASSWORD`
- 邮件服务：`SMTP_HOST`、`SMTP_PORT`、`SMTP_USERNAME`、`SMTP_PASSWORD`、`SMTP_FROM`、`EMAIL_CODE_EXPIRY_MINUTES`、`EMAIL_CODE_DAILY_LIMIT`、`EMAIL_CODE_IP_DAILY_LIMIT`
- 通知：`BOT_NOTIFY_URL`
- 只读展示：`DATABASE_URL`、`JWT_SECRET`、`INTERNAL_API_SECRET`、`ADMIN_USERNAME`、`ADMIN_PASSWORD`、`TELEGRAM_BOT_TOKEN`、`TELEGRAM_WEBHOOK_SECRET`、`WEBHOOK_URL`、`PORT`、`AUTO_MIGRATE` 等

### 4.5 Subscription（订阅求片）

**表名**: `subscriptions` | **文件**: `models/subscription.go`

| 字段 | 类型 | 列名 | 说明 |
|------|------|------|------|
| ID | string(25) | id | CUID |
| UserID | string(25) | userId | 用户 ID |
| Type | MediaType | type | `"MOVIE"` 或 `"TV"` |
| Name | string(255) | name | 媒体名称 |
| TmdbID | string | tmdbId | TMDB ID |
| PosterPath | *string(500) | posterPath | 海报 URL |
| Status | SubscriptionStatus | status | `PENDING`/`APPROVED`/`REJECTED` |
| Note | *string | note | 用户备注 |
| MpError | *string(500) | mpError | MoviePilot 同步错误 |
| CreatedAt | time.Time | createdAt | 自动 |
| UpdatedAt | time.Time | updatedAt | 自动 |

### 4.6 EmailVerification（邮箱验证码）

**表名**: `email_verifications` | **文件**: `models/email_verification.go`

| 字段 | 类型 | 列名 | 说明 |
|------|------|------|------|
| ID | string(25) | id | CUID |
| Email | string(255) | email | 索引 |
| Code | string(6) | code | 6 位验证码（JSON 隐藏）|
| Type | string(20) | type | 验证码类型：`register`/`reset`（索引）|
| IP | string(45) | ip | 请求 IP（索引，JSON 隐藏）|
| ExpiresAt | time.Time | expiresAt | 过期时间 |
| CreatedAt | time.Time | createdAt | 自动 |

### 4.7 Plan（付费方案）

**表名**: `plans` | **文件**: `models/plan.go`

| 字段 | 类型 | 列名 | 说明 |
|------|------|------|------|
| ID | string(25) | id | CUID |
| Name | string(100) | name | 方案名称 |
| Description | string(500) | description | 描述 |
| Days | int | days | 天数 |
| Price | int64 | price | 价格（分）|
| Currency | string(3) | currency | 币种（默认 `"usd"`）|
| IsActive | bool | isActive | 是否启用（默认 true，DELETE 接口仅置为 false 作为软删除）|
| SortOrder | int | sortOrder | 排序（默认 0）|
| CreatedAt | time.Time | createdAt | 自动 |
| UpdatedAt | time.Time | updatedAt | 自动 |

### 4.8 Payment（支付记录）

**表名**: `payments` | **文件**: `models/payment.go`

| 字段 | 类型 | 列名 | 说明 |
|------|------|------|------|
| ID | string(25) | id | CUID |
| UserID | string(25) | userId | 用户 ID（索引）|
| PlanID | string(25) | planId | 方案 ID（索引）|
| StripeSessionID | string | stripeSessionId | Stripe 会话（唯一）|
| StripePaymentIntentID | string | stripePaymentIntentId | Stripe 支付意向 |
| Amount | int64 | amount | 金额（分）|
| Currency | string | currency | 币种（默认 `"usd"`）|
| Days | int | days | 购买天数 |
| Status | PaymentStatus | status | `pending`/`completed`/`failed` |
| CreatedAt | time.Time | createdAt | 自动 |
| UpdatedAt | time.Time | updatedAt | 自动 |

### 4.9 PlaybackRanking（播放排行快照）

**表名**: `playback_rankings` | **文件**: `models/playback_ranking.go`

| 字段 | 类型 | 列名 | 说明 |
|------|------|------|------|
| ID | string(25) | id | CUID |
| Period | RankingPeriod | period | `"daily"` 或 `"weekly"` |
| Category | RankingCategory | category | `"media_movie"` 或 `"media_episode"` |
| Rank | int | rank | 排名 |
| ItemName | string(500) | itemName | 媒体名称 |
| PlayCount | int | playCount | 播放次数 |
| Duration | int64 | duration | 总时长（秒）|
| SnapshotAt | time.Time | snapshotAt | 快照时间 |
| PeriodStart | time.Time | periodStart | 周期开始 |
| PeriodEnd | time.Time | periodEnd | 周期结束 |
| CreatedAt | time.Time | createdAt | 自动 |

### 4.10 ClientBlacklist（客户端黑名单）

**表名**: `client_blacklists` | **文件**: `models/client_blacklist.go`

| 字段 | 类型 | 列名 | 说明 |
|------|------|------|------|
| ID | string(25) | id | CUID |
| ClientName | string(100) | clientName | 客户端名称 |
| NormalizedClientName | string(100) | normalizedClientName | 归一化名称（唯一索引） |
| Reason | string(255) | reason | 黑名单原因 |
| CreatedAt | time.Time | createdAt | 自动 |

### 4.11 DeviceAction（设备操作日志）

**表名**: `device_actions` | **文件**: `models/device_action.go`

| 字段 | 类型 | 列名 | 说明 |
|------|------|------|------|
| ID | string(25) | id | CUID |
| DeviceID | string(100) | deviceId | 设备 ID（索引） |
| UserID | string(25) | userId | 用户 ID（索引） |
| ClientName | string(100) | clientName | 客户端名 |
| Action | string(50) | action | 操作类型（blacklist/unblacklist/logout） |
| Note | string(255) | note | 备注 |
| CreatedAt | time.Time | createdAt | 自动 |

### 4.12 TelegramBindCode（Telegram 绑定验证码）

**表名**: `telegram_bind_codes` | **文件**: `models/telegram_bind_code.go`

| 字段 | 类型 | 列名 | 说明 |
|------|------|------|------|
| ID | string(25) | id | CUID |
| UserID | string(25) | userId | 用户 ID（索引） |
| Code | string(6) | code | 6 位绑定验证码 |
| ExpiresAt | time.Time | expiresAt | 过期时间（默认 5 分钟） |
| CreatedAt | time.Time | createdAt | 自动 |

### 4.13 TVCalendar（追剧日历）

**表名**: `tv_calendar_sources` | **文件**: `models/tv_calendar.go`

| 字段 | 类型 | 列名 | 说明 |
|------|------|------|------|
| ID | string(25) | id | CUID |
| TmdbID | string(50) | tmdbId | TMDB 剧集 ID（唯一） |
| SeriesID | string(50) | seriesId | Emby SeriesId |
| ShowName | string(255) | showName | 剧名 |
| PosterURL | string(500) | posterUrl | 海报地址 |
| Overview | text | overview | 剧集简介 |
| EmbyStatus | string(20) | embyStatus | Emby 识别状态，当前主要使用 `continuing` |
| LastSyncedAt | *time.Time | lastSyncedAt | 最近周历同步时间 |
| CreatedAt | time.Time | createdAt | 自动 |
| UpdatedAt | time.Time | updatedAt | 自动 |

**表名**: `tv_calendar_items` | **文件**: `models/tv_calendar.go`

| 字段 | 类型 | 列名 | 说明 |
|------|------|------|------|
| ID | string(25) | id | CUID |
| TmdbID | string(50) | tmdbId | TMDB 剧集 ID（联合唯一索引） |
| SeriesID | string(50) | seriesId | Emby SeriesId（可空） |
| Season | int | season | 季号（联合唯一索引） |
| Episode | int | episode | 集号（联合唯一索引） |
| AirDate | time.Time | airDate | 播出日期（UTC 00:00:00） |
| EpisodeName | string(255) | episodeName | 集标题 |
| Overview | text | overview | 单集简介 |
| Status | string(20) | status | `ready/missing/upcoming/today` |
| EmbyItemID | string(50) | embyItemId | Emby 集条目 ID（可空） |
| LastChecked | time.Time | lastChecked | 最近同步时间 |
| CreatedAt | time.Time | createdAt | 自动 |
| UpdatedAt | time.Time | updatedAt | 自动 |

**表名**: `tv_calendar_subscriptions` | **文件**: `models/tv_calendar.go`

| 字段 | 类型 | 列名 | 说明 |
|------|------|------|------|
| ID | string(25) | id | CUID |
| UserID | string(25) | userId | 用户 ID（联合唯一索引） |
| TmdbID | string(50) | tmdbId | TMDB 剧集 ID（联合唯一索引） |
| ShowName | string(255) | showName | 剧名 |
| PosterURL | string(500) | posterUrl | 海报地址 |
| CreatedAt | time.Time | createdAt | 自动 |

**表名**: `tmdb_cache` | **文件**: `models/tv_calendar.go`

| 字段 | 类型 | 列名 | 说明 |
|------|------|------|------|
| ID | string(25) | id | CUID |
| CacheKey | string(255) | cacheKey | 缓存键（唯一） |
| CacheValue | text | cacheValue | TMDB JSON 响应 |
| ExpiresAt | time.Time | expiresAt | 过期时间 |
| CreatedAt | time.Time | createdAt | 自动 |

### 4.14 数据关系

```
User (1) ──→ (N) Redemption     （兑换历史）
User (1) ──→ (N) Subscription   （求片记录）
User (1) ──→ (N) Payment        （支付记录）
User (1) ──→ (N) TelegramBindCode（临时绑定验证码）
User (1) ──→ (N) RedemptionCode （模板用户引用，可空）
User (1) ──→ (1) Emby User      （外部 Emby 账号，通过 EmbyID 关联）

Plan (1) ──→ (N) Payment        （方案关联）
RedemptionCode ──→ Redemption   （码被使用时生成记录）
Setting                         （全局 KV 配置，无外键）
EmailVerification               （独立验证码，无外键）
PlaybackRanking                 （独立排行快照，无外键）
ClientBlacklist ──→ DeviceAction（按 clientName 审计）
User (1) ──→ (N) TVCalendarSubscription（用户追剧订阅）
TVCalendarSource (1) ──→ (N) TVCalendarItem（按 tmdbId 关联）
TVCalendarSubscription (N) ──→ (1) TVCalendarSource（按 tmdbId 关联）
TMDBCache（独立缓存表）
```

---

## 5. 后端服务层

### 5.1 AuthService (`services/auth.go`)

**登录流程**：
1. 查找用户 → Admin: bcrypt 校验 → 普通用户: 本地密码优先 → 无本地密码时降级 Emby 认证 + 自动补存 hash
2. 过期用户**可以登录**（前端显示过期提示 + 兑换入口）

**注册流程**：
1. 通过 `ConfigService` 读取 `registration_mode` → `"invite"`: 验证兑换码 → `"open"`: 读取 `default_trial_days`
2. 如果 `ConfigService` 解析的 `email_verification` 开启，且 SMTP 已配置：校验邮箱验证码
3. 创建 Emby 用户 → 创建本地用户（含 bcrypt hash）
4. invite 模式且兑换码绑定 `templateUserId` 时：按白名单字段复制模板用户 Emby Policy
5. 签发 JWT
6. 火忘式通知 Bot（新用户注册）

**关键 struct**：
- `RegisterUserRequest{Username, Password, Email, Code, EmailCode}` — Code/EmailCode 可选
- `LoginResponse{Token, User}`

### 5.2 UserService (`services/user.go`)

- `GetUsers(page, pageSize, search, isActive, expiresAfter, embyStatus)` — 分页搜索（`expiresAfter` 格式 `YYYY-MM-DD`，筛选 `expiresAt > expiresAfter`；`embyStatus` 支持 `available/disabled/unlinked`）
- `ExtendExpiry(userID, days)` — 已过期从 now 起算，未过期从 ExpiresAt 叠加
- `UpdatePassword(userID, old, new)` — Emby + 本地 hash 同步
- `ResetPassword(userID, new)` — 管理员重置，Emby + 本地 hash 同步
- `ToggleUserStatus(userID)` — 翻转 IsActive

### 5.3 RedemptionCodeService (`services/redemption_code.go`)

- `CreateRedemptionCode(maxUses, defaultDays, expiresAt, templateUserId)` — 生成 16 字符 hex 码
- `GetRedemptionCodes(page, pageSize, showAll)` — showAll=false 过滤已失效
- `GetUserTemplates()` — 获取可选模板用户列表（启用且未过期）
- `ValidateCode(code)` — 查找 + IsValid()
- `UseCode(code)` — 原子递增 usedCount

### 5.4 RedemptionService (`services/redemption.go`)

**核心方法 `RedeemCode(userID, code)`**：
1. 开启事务后查询兑换码并校验 `IsValid()`
2. 在事务中检查 `redemptions(userId, code)` 是否已存在，存在则返回 `ErrRedemptionDuplicate`
3. 查询用户并计算新 ExpiresAt，按需调用 Emby 解封，仅更新 `expiresAt/embyDisabled`
4. 先插入 Redemption 记录（依赖 `redemptions(userId, code)` 唯一约束兜底并发重复兑换）
5. 原子递增 usedCount（`WHERE usedCount < maxUses AND (expiresAt IS NULL OR expiresAt > now)`）→ 提交

### 5.5 ConfigService (`config/config.go`)

- `List()` — 返回配置定义 + 当前解析结果（来源、是否有值、是否敏感、是否需重启）
- `Update(key, req, userID)` — 更新单项配置，支持敏感值加密存储
- `ResolveString(key)` / `GetString(key)` — 统一配置读取入口
- `GetRegistrationMode()` / `GetDefaultTrialDays()` / `IsEmailVerificationEnabled()` / `GetStripeAllowedPaymentMethods()` — 业务配置便捷读取
- `TestGroup(group)` — 分组配置连通性测试（v1: `media`、`email`）
- `ImportEnv(userID)` — 把允许托管的环境变量导入数据库

**关键职责**：
- 配置定义注册表（标签、分组、类型、校验、默认值）
- 读取策略由配置定义控制：已托管的运行期集成配置优先数据库并可禁用 env 回退；部署边界配置仍保留 env / default 解析
- 敏感值加密：`CONFIG_ENCRYPTION_KEY`
- 运行期配置中心 API 的后端基础设施

### 5.6 SystemService (`services/system.go`)

- `GetSystemInfo()` — 统计：用户数、活跃数、兑换码数
- `CheckExpiredUsers()` — **cron 核心**：查询 `expiresAt < NOW() AND embyDisabled = false` → 调用 Emby `SetUserPolicy(IsDisabled: true)` → 设置 `EmbyDisabled = true`

### 5.7 EmbyService (`integrations/emby/emby.go`)

Emby 媒体服务器 HTTP 客户端，10 秒超时。

| 方法 | Emby 端点 | 用途 |
|------|-----------|------|
| `AuthenticateUser` | `POST /emby/Users/AuthenticateByName` | 用户认证 |
| `CreateEmbyUser` | `POST /emby/Users/New` | 创建账号 |
| `UpdateUserPassword` | `POST /emby/Users/{id}/Password` | 修改密码 |
| `SetUserPolicy` | `POST /emby/Users/{id}/Policy` | 封禁/解封（IsDisabled） |
| `GetMediaStats` | `GET /emby/Items/Counts` | 媒体库统计 |
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

- `GetEmbyConfig()` — 返回公开的 Emby URL（`NEXT_PUBLIC_EMBY_URL` 优先；该项允许显式置空以强制回退 `EMBY_URL`）
- `GetMediaStats()` — 5 分钟 RWMutex 缓存层

### 5.9 SubscriptionService (`services/subscription.go`)

- `CreateSubscription(userID, type, name, tmdbId)` — 创建 PENDING 状态 + 火忘式通知 Bot
- `ApproveSubscription(id)` — 调用 MoviePilot → 设为 APPROVED（MP 失败不阻塞审批，错误存入 mpError）
- `RejectSubscription(id)` — 设为 REJECTED

### 5.10 DeviceService (`services/device.go`)

- `GetDevices(req)` — 设备列表（支持 userId/clientName/isBlacklisted 过滤 + 分页）
- `GetBlacklist()` / `AddClientToBlacklist()` / `RemoveClientFromBlacklist()` — 客户端黑名单管理
- `LogoutDevice(deviceID)` — 强制注销单设备
- `LogoutBlacklistedDevices()` — 批量注销黑名单设备
- `GetStats()` — 客户端分布、设备分布、黑名单数量、活跃会话数
- `GetDeviceActions(limit)` — 最近设备操作日志

### 5.11 MoviePilotClient (`integrations/moviepilot/client.go`)

- `IsConfigured()` — 检查三个环境变量是否都设置
- `login()` — `POST /api/v1/login/access-token`（form-urlencoded）
- `CreateSubscription(type, name, tmdbId)` — `POST /api/v1/subscribe/`（type 转中文：movie→电影, tv→电视剧）

### 5.12 EmailService (`services/email.go`)

邮箱验证码发送、校验和清理服务，基于 SMTP。

- `SendVerificationCode(email, ip, codeType)` — 生成 6 位随机验证码 → 按类型频率限制（每邮箱/每 IP 每日上限）→ SMTP 发送
- `VerifyCode(email, code, codeType)` — 按类型校验验证码是否有效且未过期
- `CleanupExpired()` — 删除过期验证码（cron 调用）
- `IsEnabled()` — 通过 `ConfigService` 解析 `email_verification`，并叠加 SMTP 完整性判断

**频率限制**：
- 每邮箱每日：`EMAIL_CODE_DAILY_LIMIT`（默认 5，按 `codeType` 隔离计数）
- 每 IP 每日：`EMAIL_CODE_IP_DAILY_LIMIT`（默认 15）
- 验证码有效期：`EMAIL_CODE_EXPIRY_MINUTES`（默认 10 分钟）

### 5.13 BotNotifier (`integrations/notifier/notifier.go`)

火忘式 HTTP 推送通知服务，将事件推送给 Telegram Bot。

**通知类型**：
| 方法 | Bot 端点 | 触发时机 |
|------|----------|----------|
| `NotifyNewSubscription` | `POST /notify/subscription` | 用户创建求片订阅 |
| `NotifyNewRegistration` | `POST /notify/registration` | 新用户注册 |
| `NotifyRanking` | `POST /notify/ranking` | 排行榜生成完成 |

**认证方式**：`X-Internal-Secret` 头（值 = `INTERNAL_API_SECRET`）

### 5.14 PlaybackRankingService (`services/playback/ranking.go`)

从 Emby PlaybackActivity 数据库生成播放排行。

- `GenerateRanking(period)` — 查询 Emby 活动日志 → 按播放次数/时长排名 → 存入数据库 → 通知 Bot
- `GetLatestRanking()` — 获取最新一期排行
- `GetRankingHistory(page, pageSize)` — 分页查询历史排行
- `PreviewRanking(period)` — 预览排行（不持久化）

**支持周期**：`daily`（日榜）、`weekly`（周榜）

### 5.15 PaymentService (`services/payment/service.go`)

Stripe 一次性支付流程管理。

- `CreateCheckoutSession(userID, planID)` — 创建 Stripe Checkout Session → 通过 `ConfigService` 读取 `stripe_allowed_payment_methods` 决定是否显式限制支付方式 → 存储 Payment 记录（pending）
- `HandleWebhook(payload, signature)` — 处理 Stripe Webhook → 更新 Payment 状态 → 成功时自动延长用户有效期
- Plan CRUD — `GetPlans`, `CreatePlan`, `UpdatePlan`, `DeletePlan`（软删除：仅下架 `isActive=false`）
- `GetPayments(page, pageSize)` — 支付记录查询

### 5.16 错误定义（按业务拆分）

统一的业务错误定义已按领域拆分，例如：

- `services/redemption_errors.go`
- `services/subscription_errors.go`
- `services/email_errors.go`
- `services/payment_errors.go`
- `services/telegram_errors.go`
- `services/device_errors.go`
- `services/media_quality_errors.go`

handler 继续通过 `errors.Is()` 做错误映射。

### 5.17 TelegramService (`services/telegram.go`)

Telegram 账号绑定与 Bot 自助能力服务。

- `GenerateBindCode(userID)` — 生成 6 位绑定验证码（5 分钟有效），并清理该用户旧验证码
- `VerifyBind(telegramID, code)` — 校验验证码并绑定 Telegram ID（事务 + 行锁）
- `Unbind(userID)` — 解绑 Telegram ID
- `GetAccountInfo(telegramID)` — 查询绑定用户账号状态
- `RedeemByTelegram(telegramID, code)` — 复用 `RedemptionService` 完成续期兑换
- `ResetPassword(telegramID, newPassword)` — 通过 Telegram 身份重置 Ember/Emby 密码
- `CleanupExpiredBindCodes()` — 删除过期绑定码（cron 调用）

### 5.18 TVCalendarService (`services/tvcalendar/service.go`)

追剧日历聚合服务，主链路改为“Emby 全库发现 + 周历同步 + Webhook 点亮”，TMDB 仍使用三层缓存（内存 + PostgreSQL + TMDB）。

- `DiscoverContinuingSeries(ctx)` — 从 Emby 自动发现所有 `Continuing` 且带 `Tmdb` Provider ID 的剧集
- `SyncWeeklyCalendar(ctx, weekOffset, tmdbId, force)` — 同步上周 / 本周 / 下周的全局周历缓存
- `GetGlobalWeeklyCalendar(ctx, weekOffset, status)` — 查询全局周历视图
- `GetFollowingWeeklyCalendar(ctx, userID, weekOffset, status)` — 查询当前用户的关注周历视图
- `FetchCalendar(userID, startDate, endDate, status)` — 兼容旧平铺接口，底层仍复用新的全局缓存数据
- `Subscribe(userID, tmdbId, showName, posterUrl)` — 创建或更新用户关注
- `GetSubscriptions(userID)` — 获取用户关注列表
- `Unsubscribe(userID, tmdbId)` — 取消关注
- `SyncCalendar(weekOffsets, tmdbId, force)` — 管理员手动同步（单剧 / 全部 / 指定周）
- `MarkEpisodeReadyByWebhook(...)` — Emby Webhook 将剧集状态点亮为 `ready`

### 5.19 PlaybackHistoryService (`services/playback/history.go`)

管理员播放历史查询服务，复用 Emby Playback Reporting 插件能力，支持分页和条件筛选。

- `GetPlaybackHistory(req)` — 支持 `userId` / `keyword` / `startDate` / `endDate` / `page` / `pageSize`
- 对 `keyword` 做白名单校验并转义，避免 SQL 注入
- 统一输出播放时长格式（`Xm` / `Xh Ym`）
- 插件不可用时返回统一错误：`Playback Reporting 查询失败`

### 5.20 MediaQualityService (`services/media_quality.go`)

管理员媒体库质量盘点服务，按媒体库维度（支持 `libraryId=all`）聚合分辨率、编码、HDR 分布，并输出低画质汇总清单。

- `GetLibraryQuality(ctx, libraryID, force)` — 缓存命中优先（`force=false`），否则触发扫描
- `ScanLibraryQuality(ctx, libraryID)` — 拉取媒体库条目并生成质量报告
- `GetGroupLowQualityDetails(ctx, libraryID, groupID, force)` — 按汇总分组下钻低画质明细
- 缓存模型：`media_quality_caches`（PostgreSQL 持久化，按 `libraryId` 唯一）
- 低画质清单按“影片/剧集”汇总，避免电视剧按单集展开造成噪音
- 低画质汇总项包含 `groupId`，前端使用 `groupId` 请求下钻接口
- 报告字段：`resolutionDistribution` / `codecDistribution` / `hdrDistribution` / `lowQualityItems` / `lowQualityTotal` / `page` / `pageSize` / `scanAt`

---

## 6. API 端点完整列表

### 公开路由（无需认证）

| 方法 | 路径 | 用途 |
|------|------|------|
| POST | `/api/v1/login` | 登录 |
| POST | `/api/v1/user/register` | 注册（code/emailCode 可选）|
| POST | `/api/v1/register/send-code` | 发送邮箱验证码 |
| POST | `/api/v1/forgot-password/send-code` | 发送密码重置验证码 |
| POST | `/api/v1/forgot-password/reset` | 通过验证码重置密码 |
| GET | `/api/v1/register/mode` | 获取注册模式 |
| GET | `/api/v1/register/code/:code/validate` | 验证兑换码（注册前）|
| GET | `/api/v1/plans` | 公开方案列表（仅 isActive=true）|
| POST | `/api/v1/webhooks/stripe` | Stripe Webhook 回调 |
| POST | `/api/v1/webhooks/emby?token=` | Emby 入库 Webhook（追剧日历） |
| GET | `/api/v1/tmdb/search?query=&type=` | TMDB 搜索 |

### 统一认证路由（admin + user 共享，需 JWT）

| 方法 | 路径 | 用途 |
|------|------|------|
| GET | `/api/v1/subscriptions` | 我的订阅 |
| POST | `/api/v1/subscriptions` | 创建订阅 |
| DELETE | `/api/v1/subscriptions/:id` | 删除订阅 |
| GET | `/api/v1/profile` | 个人信息 |
| PUT | `/api/v1/profile` | 更新资料 |
| PUT | `/api/v1/password` | 修改密码 |
| PUT | `/api/v1/email` | 修改邮箱 |
| POST | `/api/v1/telegram/bindcode` | 生成 Telegram 绑定验证码 |
| DELETE | `/api/v1/telegram/unbind` | 解除 Telegram 绑定 |
| GET | `/api/v1/emby/config` | Emby 配置 |
| GET | `/api/v1/media/stats` | 媒体统计 |
| GET | `/api/v1/media/latest` | 最新入库 |
| GET | `/api/v1/rankings/latest` | 最新排行 |
| GET | `/api/v1/rankings/history` | 排行历史 |
| POST | `/api/v1/payments/checkout` | Stripe 结账 |
| GET | `/api/v1/payments` | 我的支付记录 |
| GET | `/api/v1/tv-calendar/global` | 全局追剧周历 |
| GET | `/api/v1/tv-calendar/following` | 我的关注周历 |
| GET | `/api/v1/tv-calendar` | 追剧日历 |
| GET | `/api/v1/tv-calendar/subscriptions` | 我的关注列表 |
| POST | `/api/v1/tv-calendar/subscriptions` | 关注剧集 |
| DELETE | `/api/v1/tv-calendar/subscriptions/:tmdbId` | 取消关注剧集 |

### 用户路由（需认证 + role=user）

| 方法 | 路径 | 用途 |
|------|------|------|
| GET | `/api/v1/user/profile` | 个人信息 |
| PUT | `/api/v1/user/profile` | 更新资料 |
| PUT | `/api/v1/user/password` | 修改密码 |
| PUT | `/api/v1/user/email` | 修改邮箱 |
| POST | `/api/v1/user/redeem` | 兑换续期 |
| GET | `/api/v1/user/redeem/:code/validate` | 兑换码预验证 |
| GET | `/api/v1/user/redemptions` | 我的兑换历史 |
| GET | `/api/v1/user/emby/config` | Emby 服务器地址 |
| GET | `/api/v1/user/media/stats` | 媒体库统计 |
| GET | `/api/v1/user/subscriptions` | 我的订阅 |
| POST | `/api/v1/user/subscriptions` | 创建订阅 |
| DELETE | `/api/v1/user/subscriptions/:id` | 删除订阅 |

### 管理员路由（需认证 + role=admin）

| 方法 | 路径 | 用途 |
|------|------|------|
| GET | `/api/v1/admin/current` | 当前管理员信息 |
| GET | `/api/v1/admin/users` | 用户列表 |
| GET | `/api/v1/admin/users/:id` | 用户详情 |
| PUT | `/api/v1/admin/users/:id` | 更新用户 |
| PUT | `/api/v1/admin/users/:id/extend` | 延长有效期 |
| PUT | `/api/v1/admin/users/:id/toggle` | 切换激活状态 |
| PUT | `/api/v1/admin/users/:id/reset-password` | 重置密码 |
| DELETE | `/api/v1/admin/users/:id` | 删除用户 |
| GET | `/api/v1/admin/redemption-codes` | 兑换码列表 |
| POST | `/api/v1/admin/redemption-codes` | 创建兑换码 |
| PUT | `/api/v1/admin/redemption-codes/:id` | 更新兑换码 |
| DELETE | `/api/v1/admin/redemption-codes/:id` | 删除兑换码 |
| GET | `/api/v1/admin/user-templates` | 模板用户列表 |
| GET | `/api/v1/admin/configs` | 获取设置中心全部配置（定义 + 当前值 + 来源） |
| PATCH | `/api/v1/admin/configs/:key` | 更新单项配置 |
| POST | `/api/v1/admin/configs/:group/test` | 测试指定配置组 |
| POST | `/api/v1/admin/configs/import-env` | 导入当前环境变量为数据库覆盖值 |
| GET | `/api/v1/admin/redemptions` | 全部兑换历史 |
| GET | `/api/v1/admin/subscriptions` | 全部订阅 |
| PUT | `/api/v1/admin/subscriptions/:id/approve` | 审批通过 |
| PUT | `/api/v1/admin/subscriptions/:id/reject` | 审批拒绝 |
| DELETE | `/api/v1/admin/subscriptions/:id` | 删除订阅 |
| GET | `/api/v1/admin/sessions` | 活跃会话 |
| GET | `/api/v1/admin/playback-history` | 播放历史查询 |
| GET | `/api/v1/admin/media-quality/libraries` | 媒体库列表（质量盘点） |
| GET | `/api/v1/admin/media-quality/libraries/:libraryId` | 媒体库质量报告（支持 `force/page/pageSize`） |
| POST | `/api/v1/admin/media-quality/libraries/:libraryId/scan` | 触发媒体库质量扫描 |
| GET | `/api/v1/admin/media-quality/libraries/:libraryId/groups/:groupId/details` | 低画质汇总项下钻明细（支持 `force/page/pageSize`） |
| GET | `/api/v1/admin/media-quality/posters/:itemId` | 媒体质量封面代理 |
| GET | `/api/v1/admin/devices` | 设备列表 |
| GET | `/api/v1/admin/devices/stats` | 设备统计 |
| GET | `/api/v1/admin/devices/actions` | 设备操作日志 |
| GET | `/api/v1/admin/devices/blacklist` | 黑名单列表 |
| POST | `/api/v1/admin/devices/blacklist` | 添加黑名单 |
| DELETE | `/api/v1/admin/devices/blacklist/:clientName` | 移除黑名单 |
| POST | `/api/v1/admin/devices/logout/:deviceId` | 强制注销设备 |
| POST | `/api/v1/admin/devices/blacklist/logout-all` | 批量注销黑名单设备 |
| GET | `/api/v1/admin/plans` | 方案列表 |
| POST | `/api/v1/admin/plans` | 创建方案 |
| PUT | `/api/v1/admin/plans/:id` | 更新方案 |
| DELETE | `/api/v1/admin/plans/:id` | 下架方案（软删除） |
| GET | `/api/v1/admin/payments` | 全部支付记录 |
| GET | `/api/v1/admin/system/info` | 系统统计 |
| POST | `/api/v1/admin/system/test-emby` | 测试 Emby 连接 |
| POST | `/api/v1/admin/tv-calendar/sync` | 手动同步追剧日历 |
| POST | `/api/v1/admin/tv-calendar/refresh` | 手动刷新追剧日历 |
| POST | `/api/v1/admin/cron/check-expired` | 手动执行过期检查 |
| POST | `/api/v1/admin/cron/generate-ranking` | 手动生成排行 |
| POST | `/api/v1/admin/rankings/preview` | 排行预览 |

追剧日历同步接口说明：

- `POST /api/v1/admin/tv-calendar/sync`：请求体可选，默认同步 `[-1, 0, 1]`
- `tmdbId` 可选，传入时只同步单剧
- `weekOffsets` 可选，仅支持 `-1/0/1`
- `POST /api/v1/admin/tv-calendar/refresh` 仍保留，内部复用同步逻辑，作为兼容入口

### 内部服务路由（InternalAuth 中间件，Bot 调用）

| 方法 | 路径 | 用途 |
|------|------|------|
| PUT | `/api/v1/internal/subscriptions/:id/approve` | 审批通过 |
| PUT | `/api/v1/internal/subscriptions/:id/reject` | 审批拒绝 |
| GET | `/api/v1/internal/settings/:key` | 读取内部配置（仅允许访问统一配置层中已注册的非敏感 key；未知 key 返回 404） |
| POST | `/api/v1/internal/telegram/bind` | Bot 校验并绑定账号 |
| POST | `/api/v1/internal/telegram/info` | Bot 查询账号信息 |
| POST | `/api/v1/internal/telegram/redeem` | Bot 兑换续期码 |
| POST | `/api/v1/internal/telegram/reset-password` | Bot 重置账号密码 |
| POST | `/api/v1/internal/telegram/subscribe` | Bot 创建求片订阅 |

### API 响应格式约定

- **列表**：`{data: [], total, page, pageSize, totalPages}`
- **单个对象**：直接返回对象或 `{user: object}`
- **成功操作**：`{message: "xxx"}`
- **错误**：`{error: "xxx"}`（400/401/404/500）
- **字段命名**：camelCase

---

## 7. 认证与授权

- **JWT**：HS256，7 天有效期，Claims = {userID, username, role}
- **Token 传递**：`Authorization: Bearer {token}`
- **中间件链**：`JWTAuth()` → `AdminOnly()` / `UserOnly()`
- **InternalAuth**：`middleware/internal_auth.go` — 校验 `X-Internal-Secret` header，用于 Bot ↔ API 内部通信
- **Context 变量**：`userID`, `username`, `role`, `claims`
- **密码存储**：bcrypt（DefaultCost），所有用户统一存本地 hash
- **存量迁移**：`Password == ""` 时降级 Emby 认证，成功后自动补存本地 hash

---

## 8. 前端架构

### 状态管理（Pinia）

- `store/auth.ts`：Token + Role（localStorage 持久化）
  - State: `token`, `role`
  - Computed: `isAuthenticated`, `isAdmin`, `isUser`
  - Actions: `login`, `register`, `logout`, `setAuth`, `clearAuth`, `restoreAuth`
- `store/user.ts`：用户状态管理
- `store/admin.ts`：管理员状态管理

### API 层

- `api/request.ts` — 基础配置：baseURL=/api/v1, 401 拦截
- `api/auth.ts` — login, register, getRegistrationMode, sendEmailCode, sendResetCode, resetPasswordByCode
- `api/user.ts` — redeem, redemptions, tmdb
- `api/admin.ts` — 管理后台全部接口（users, codes, settings, subscriptions, plans, payments, sessions, devices, rankings）
- `api/console.ts` — 统一认证路由（profile, subscriptions, payments, rankings, media, emby, telegram）

### 路由守卫

- 未认证 → 重定向 `/login`（带 redirect 参数）
- 角色不匹配 → 重定向 `/`
- meta: `{requiresAuth: boolean, role: 'admin' | 'user'}`

### 设计系统

- **CSS 类**：`panel-clean`（卡片）, `input-ember`（输入框）, `btn-ember`（按钮）
- **颜色**：ember 色系（橙红 `#ea580c`）
- **布局**：Tailwind 响应式 grid + Element Plus 组件
- **图标**：`@element-plus/icons-vue`

### Dashboard 双态设计

用户面板根据 `isExpired` computed 做渐进式降级：
- **活跃态**：绿色 banner + 媒体统计 + 兑换折叠面板
- **过期态**：橙色警告 banner + 兑换码输入醒目展示 + 媒体统计灰化
- **兑换历史**：普通用户在 Dashboard 查看个人兑换记录（分页 + 手动刷新）

### 管理端兑换历史

- 新增路由：`/console/redemption-history`（admin）
- 新增视图：`views/admin/RedemptionHistoryView.vue`
- 数据源：`GET /api/v1/admin/redemptions`（支持 userId 分页筛选）

### 管理端设备管理

- 新增路由：`/console/devices`（admin）
- 新增视图：`views/admin/DevicesView.vue`
- 数据源：
  - `GET /api/v1/admin/devices`
  - `GET /api/v1/admin/devices/stats`
  - `GET /api/v1/admin/devices/blacklist`
  - `POST /api/v1/admin/devices/logout/:deviceId`

### 管理端播放历史

- 新增路由：`/console/playback-history`（admin）
- 新增视图：`views/admin/PlaybackHistoryView.vue`
- 数据源：`GET /api/v1/admin/playback-history`（支持 userId / keyword / 日期范围 / 分页筛选）

### 管理端媒体质量盘点

- 新增路由：`/console/media-quality`（admin）
- 新增视图：`views/admin/MediaQualityView.vue`
- 数据源：
  - `GET /api/v1/admin/media-quality/libraries`
  - `GET /api/v1/admin/media-quality/libraries/:libraryId?force=true|false&page=1&pageSize=20`
  - `POST /api/v1/admin/media-quality/libraries/:libraryId/scan`
  - `GET /api/v1/admin/media-quality/posters/:itemId`
- 支持 `libraryId=all` 进行全媒体库汇总分析
- 低画质结果按“影片/剧集”汇总后分页展示

---

## 9. Telegram Bot 架构

### 技术栈

- Python 3.11 + python-telegram-bot（Webhook 模式，非 Polling）
- FastAPI 作为 HTTP 服务器（接收 Telegram Webhook + API 通知）
- 与 Go API 通过 `X-Internal-Secret` 双向通信

### 通信模式

```
用户操作 → Go API → BotNotifier（火忘式 POST）→ Bot FastAPI → Telegram Bot → 发送消息
Telegram 用户操作 → Telegram → Bot Webhook → Bot 处理 → 调用 Go Internal API → 返回结果
```

### Bot 端点

| 端点 | 用途 |
|------|------|
| `GET /health` | 健康检查 |
| `POST /telegram/webhook` | Telegram Webhook 入口 |
| `POST /notify/subscription` | 接收新订阅通知 |
| `POST /notify/registration` | 接收新注册通知 |
| `POST /notify/ranking` | 接收排行榜通知 |

### 命令与处理器

- **CallbackQuery**：订阅审批按钮（approve/reject → 调用 Internal API）
- **NewChatMembers**：群组欢迎消息（读取 `notify_group_link` 配置）
- **Commands**：`/search`（搜索影视并订阅）、`/cancel`（取消备注输入并回到详情页）、`/bind`（绑定账号）、`/info`（查看账号信息）、`/redeem`（兑换续期码）、`/resetpw`（重置密码）
- **通知格式化**：`message_formatter.py` 统一格式化 Telegram 消息（HTML 模式）

### 环境变量

| 变量 | 必需 | 默认值 | 说明 |
|------|------|--------|------|
| `TELEGRAM_BOT_TOKEN` | ✅ | — | Bot Token（@BotFather 获取）|
| `TELEGRAM_ADMIN_CHAT_ID` | — | — | 管理员 Chat ID；可被设置中心数据库值覆盖，env 仅作兜底 |
| `TELEGRAM_GROUP_CHAT_ID` | — | — | 群组 Chat ID（排行榜推送）；可被设置中心数据库值覆盖，env 仅作兜底 |
| `TELEGRAM_WEBHOOK_SECRET` | ✅ | — | Webhook 签名校验 |
| `INTERNAL_API_SECRET` | ✅ | — | 与 Go API 共享密钥 |
| `WEBHOOK_URL` | ✅ | — | 公开 HTTPS Webhook URL |
| `API_URL` | — | `http://localhost:8080` | Ember API 地址 |
| `BOT_PORT` | — | `8000` | Bot 服务端口 |

说明：Bot 在运行期通过 Internal API 读取 `TELEGRAM_ADMIN_CHAT_ID`、`TELEGRAM_GROUP_CHAT_ID` 和 `notify_group_link`，并做短 TTL 缓存；当 API 未返回值时，Chat ID 回退到本地 env。

---

## 10. 定时任务

| 任务 | 调度 | 控制变量 | 说明 |
|------|------|----------|------|
| 过期用户检查 | `CRON_SCHEDULE`（默认 `0 2 * * *`）| `CRON_ENABLED` | 封禁过期 Emby 账号 |
| 验证码清理 | `0 3 * * *` | `CRON_ENABLED` | 删除过期 EmailVerification + TelegramBindCode |
| 日榜生成 | `RANKING_DAILY_SCHEDULE`（默认 `0 20 * * *`）| `RANKING_CRON_ENABLED` | 从 Emby 生成日播放排行 |
| 周榜生成 | `RANKING_WEEKLY_SCHEDULE`（默认 `30 20 * * 0`）| `RANKING_CRON_ENABLED` | 从 Emby 生成周播放排行 |
| 追剧日历同步 | `TV_CALENDAR_SYNC_SCHEDULE`（默认 `0 */12 * * *`） | `CRON_ENABLED` | 同步 TMDB/Emby 追剧日历缓存 |

**通用配置**：
这些项由 `ConfigService` 统一解析，优先级为“数据库覆盖值 > 环境变量 > 默认值”；管理员可在设置中心修改，但属于启动期配置，保存后需重启 API 才会生效。

| 配置项 | 默认值 | 说明 |
|----------|--------|------|
| `CRON_ENABLED` | `"true"` | 是否启用（过期检查 + 验证码清理 + 追剧日历同步）|
| `CRON_SCHEDULE` | `"0 2 * * *"` | 过期检查 cron 表达式 |
| `CRON_TIMEZONE` | `"Asia/Shanghai"` | cron 与排行榜计算使用的时区 |
| `RANKING_CRON_ENABLED` | `"false"` | 是否启用排行榜生成 |
| `RANKING_DAILY_SCHEDULE` | `"0 20 * * *"` | 日榜 cron 表达式 |
| `RANKING_WEEKLY_SCHEDULE` | `"30 20 * * 0"` | 周榜 cron 表达式 |
| `TV_CALENDAR_SYNC_SCHEDULE` | `"0 */12 * * *"` | 追剧日历自动同步表达式 |

**过期检查逻辑**：查询 `expiresAt < NOW() AND embyDisabled = false` → Emby `SetUserPolicy(IsDisabled: true)` → 设置 `EmbyDisabled = true`。不修改 IsActive，不阻止用户登录。

---

## 11. 环境变量完整列表

### 核心配置

| 变量 | 必需 | 默认值 | 说明 |
|------|------|--------|------|
| `DATABASE_URL` | ✅ | — | PostgreSQL DSN |
| `JWT_SECRET` | ✅ | — | ≥32 字符 |
| `PORT` | — | `8080` | 服务端口 |
| `AUTO_MIGRATE` | — | `"false"` | `"true"` 启用 GORM 自动迁移 |
| `ADMIN_USERNAME` | — | — | 默认管理员用户名（首次启动 seed）|
| `ADMIN_PASSWORD` | — | — | 默认管理员密码 |

### Emby 集成

| 变量 | 必需 | 默认值 | 说明 |
|------|------|--------|------|
| `EMBY_URL` | — | — | Emby 服务器内部 URL |
| `EMBY_API_KEY` | — | — | Emby API 密钥 |
| `NEXT_PUBLIC_EMBY_URL` | — | — | Emby 公开 URL（给前端用）；允许显式置空后回退 `EMBY_URL` |
| `EMBY_WEBHOOK_TOKEN` | — | — | Emby Webhook token（`/api/v1/webhooks/emby?token=`）|

### TMDB / MoviePilot

| 变量 | 必需 | 默认值 | 说明 |
|------|------|--------|------|
| `TMDB_API_KEY` | — | — | TMDB API 密钥 |
| `MOVIEPILOT_URL` | — | — | MoviePilot 地址 |
| `MOVIEPILOT_USERNAME` | — | — | MoviePilot 用户名 |
| `MOVIEPILOT_PASSWORD` | — | — | MoviePilot 密码 |

### 邮件服务

| 变量 | 必需 | 默认值 | 说明 |
|------|------|--------|------|
| `SMTP_HOST` | — | — | SMTP 服务器地址 |
| `SMTP_PORT` | — | `587` | SMTP 端口 |
| `SMTP_USERNAME` | — | — | SMTP 用户名 |
| `SMTP_PASSWORD` | — | — | SMTP 密码 |
| `SMTP_FROM` | — | — | 发件人；允许显式置空后回退 `SMTP_USERNAME` |
| `EMAIL_CODE_EXPIRY_MINUTES` | — | `10` | 验证码有效期（分钟）|
| `EMAIL_CODE_DAILY_LIMIT` | — | `5` | 每邮箱每日发送上限 |
| `EMAIL_CODE_IP_DAILY_LIMIT` | — | `15` | 每 IP 每日发送上限 |

### Bot 通信

| 变量 | 必需 | 默认值 | 说明 |
|------|------|--------|------|
| `BOT_NOTIFY_URL` | — | — | Bot 通知 Webhook 地址；允许显式置空后关闭推送 |
| `INTERNAL_API_SECRET` | — | — | 内部通信共享密钥 |

### Stripe 支付

| 变量 | 必需 | 默认值 | 说明 |
|------|------|--------|------|
| `STRIPE_SECRET_KEY` | — | — | Stripe API 密钥；支持被设置中心数据库值覆盖 |
| `STRIPE_WEBHOOK_SECRET` | — | — | Stripe Webhook 签名密钥 |
| `STRIPE_SUCCESS_URL` | — | — | 支付成功跳转 URL；支持被设置中心数据库值覆盖 |
| `STRIPE_CANCEL_URL` | — | — | 支付取消跳转 URL；支持被设置中心数据库值覆盖 |

说明：Stripe Dashboard 仍是支付方式能力的真实来源；系统设置中的 `stripe_allowed_payment_methods` 仅用于进一步限制 Checkout 可展示的支付方式。

### 定时任务

| 变量 | 必需 | 默认值 | 说明 |
|------|------|--------|------|
| `CRON_ENABLED` | — | `"true"` | 启用定时任务；由设置中心数据库托管，修改后需重启 API |
| `CRON_SCHEDULE` | — | `"0 2 * * *"` | 过期检查表达式；由设置中心数据库托管，修改后需重启 API |
| `CRON_TIMEZONE` | — | `"Asia/Shanghai"` | 时区；由设置中心数据库托管，修改后需重启 API |
| `RANKING_CRON_ENABLED` | — | `"false"` | 启用排行榜生成；由设置中心数据库托管，修改后需重启 API |
| `RANKING_DAILY_SCHEDULE` | — | `"0 20 * * *"` | 日榜表达式；由设置中心数据库托管，修改后需重启 API |
| `RANKING_WEEKLY_SCHEDULE` | — | `"30 20 * * 0"` | 周榜表达式；由设置中心数据库托管，修改后需重启 API |
| `TV_CALENDAR_SYNC_SCHEDULE` | — | `"0 */12 * * *"` | 追剧日历同步表达式；由设置中心数据库托管，修改后需重启 API |

---

## 12. 外部集成汇总

| 服务 | 用途 | 配置变量 |
|------|------|----------|
| **Emby API** | 用户创建/认证/封禁/解封、媒体统计、播放活动 | `EMBY_URL`, `EMBY_API_KEY` |
| **TMDB API** | 电影/电视剧搜索（求片功能）| `TMDB_API_KEY` |
| **MoviePilot API** | 自动订阅下载（审批后触发）| `MOVIEPILOT_URL/USERNAME/PASSWORD` |
| **Stripe API** | 一次性支付（Checkout Session + Webhook）| `STRIPE_SECRET_KEY`, `STRIPE_WEBHOOK_SECRET` |
| **SMTP** | 邮箱验证码发送 | `SMTP_HOST/PORT/USERNAME/PASSWORD` |
| **Telegram Bot API** | 通知推送、订阅审批、账号绑定/查询/续期 | `TELEGRAM_BOT_TOKEN` 等（见 Bot 章节）|

---

## 13. 部署

**Docker Compose**（`infrastructure/docker/docker-compose.yml`）：
- PostgreSQL 16 + Go API + Vue 前端（可选）+ Telegram Bot + Nginx（可选）
- API 以非 root 用户 `ember:ember`(UID 1000) 运行
- 健康检查：`GET /health`

**数据库连接池**：MaxIdle=10, MaxOpen=100, MaxLifetime=1h, MaxIdleTime=10min

**时间处理**：所有时间戳 UTC 存储（GORM NowFunc 强制 UTC）

---

## 14. 代码模式速查

| 模式 | 说明 |
|------|------|
| ID 生成 | CUID 格式：`cl` + timestamp(hex) + random(hex)，25 字符 |
| 分页响应 | `{data:[], total, page, pageSize, totalPages}` |
| 错误响应 | `{error: "中文错误消息"}`，400/401/404/500 |
| Handler 模式 | `ShouldBindJSON/ShouldBindQuery` → 调用 Service → 返回 JSON |
| Service 模式 | 接收 Request struct → 业务逻辑 → 返回 Response/error |
| 码生成 | `crypto/rand.Read(bytes)` → `hex.EncodeToString` → 16 字符 |
| 密码哈希 | `bcrypt.GenerateFromPassword(DefaultCost)` |
| Emby 认证 | `X-Emby-Token: {apiKey}` 头 |
| 内部通信 | `X-Internal-Secret: {secret}` 头（Bot ↔ API）|
| 前端请求 | Axios 拦截器自动加 Bearer token，401 自动清除登录态 |
| 火忘通知 | `go func() { http.Post(...) }()` 不阻塞主流程 |
