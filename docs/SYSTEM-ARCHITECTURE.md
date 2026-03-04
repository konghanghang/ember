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
│     │  └─ utils.go             # generateCUID()
│     ├─ services/               # 业务逻辑
│     │  ├─ auth.go              # 登录 / 注册
│     │  ├─ user.go              # 用户 CRUD + 密码管理
│     │  ├─ redemption_code.go   # 兑换码 CRUD
│     │  ├─ redemption.go        # 兑换核心逻辑 + 历史
│     │  ├─ setting.go           # 系统配置
│     │  ├─ system.go            # 系统信息 + 过期检查
│     │  ├─ emby.go              # Emby HTTP 客户端
│     │  ├─ media.go             # 媒体统计（带 5min 缓存）
│     │  ├─ subscription.go      # 订阅工作流
│     │  ├─ moviepilot.go        # MoviePilot HTTP 客户端
│     │  ├─ email.go             # EmailService（邮箱验证码发送/校验/清理）
│     │  ├─ telegram.go          # TelegramService（绑定/查询/续期）
│     │  ├─ notifier.go          # BotNotifier（火忘式推送通知给 Bot）
│     │  ├─ playback_ranking.go  # PlaybackRankingService（播放排行生成）
│     │  ├─ payment.go           # PaymentService（Stripe 支付流程）
│     │  └─ errors.go            # 统一错误定义
│     ├─ handlers/               # HTTP 处理层（Gin）
│     │  ├─ auth.go              # 登录 / 注册
│     │  ├─ user.go              # 用户管理
│     │  ├─ redemption_code.go   # 兑换码管理
│     │  ├─ setting.go           # 系统配置
│     │  ├─ system.go            # 系统信息
│     │  ├─ media.go             # 媒体信息
│     │  ├─ subscription.go      # 订阅管理
│     │  ├─ tmdb.go              # TMDB 搜索
│     │  ├─ ranking.go           # 播放排行
│     │  ├─ session.go           # 活跃会话
│     │  ├─ telegram.go          # Telegram 绑定与 Bot Internal API
│     │  └─ payment.go           # 支付与方案
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
│  │     │  ├─ LibraryView.vue   # 媒体库
│  │     │  ├─ RankingsView.vue  # 播放排行
│  │     │  └─ PricingView.vue   # 付费方案
│  │     └─ admin/               # 管理后台
│  │        ├─ UsersView.vue     # 用户管理
│  │        ├─ RedemptionCodesView.vue # 兑换码管理
│  │        ├─ RedemptionHistoryView.vue # 兑换历史
│  │        ├─ SettingsView.vue  # 系统设置
│  │        ├─ PlansView.vue     # 方案管理
│  │        └─ SessionsView.vue  # 活跃会话
│  ├─ vite.config.ts             # dev:3000, proxy /api→:8080
│  └─ tailwind.config.js         # 自定义色：ember(橙红), cinema
└─ bot/                          # Python Telegram Bot
   ├─ main.py                    # 入口
   ├─ requirements.txt           # Python 依赖
   ├─ Dockerfile                 # 容器构建
   └─ app/
      ├─ config.py               # 环境变量加载
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
| Key | string(50) | key | 主键 |
| Value | string(500) | value | 值 |
| UpdatedAt | time.Time | updatedAt | 自动 |

**当前配置项**：
- `registration_mode` — `"open"` 或 `"invite"`（默认 `"open"`）
- `default_trial_days` — 开放注册时的试用天数（默认 `"7"`）
- `notify_group_link` — Telegram 通知群链接（默认空）
- `email_verification` — 是否开启邮箱验证（默认 `"false"`）

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
| IsActive | bool | isActive | 是否启用（默认 true）|
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

### 4.10 TelegramBindCode（Telegram 绑定验证码）

**表名**: `telegram_bind_codes` | **文件**: `models/telegram_bind_code.go`

| 字段 | 类型 | 列名 | 说明 |
|------|------|------|------|
| ID | string(25) | id | CUID |
| UserID | string(25) | userId | 用户 ID（索引） |
| Code | string(6) | code | 6 位绑定验证码 |
| ExpiresAt | time.Time | expiresAt | 过期时间（默认 5 分钟） |
| CreatedAt | time.Time | createdAt | 自动 |

### 4.11 数据关系

```
User (1) ──→ (N) Redemption     （兑换历史）
User (1) ──→ (N) Subscription   （求片记录）
User (1) ──→ (N) Payment        （支付记录）
User (1) ──→ (N) TelegramBindCode（临时绑定验证码）
User (1) ──→ (1) Emby User      （外部 Emby 账号，通过 EmbyID 关联）

Plan (1) ──→ (N) Payment        （方案关联）
RedemptionCode ──→ Redemption   （码被使用时生成记录）
Setting                         （全局 KV 配置，无外键）
EmailVerification               （独立验证码，无外键）
PlaybackRanking                 （独立排行快照，无外键）
```

---

## 5. 后端服务层

### 5.1 AuthService (`services/auth.go`)

**登录流程**：
1. 查找用户 → Admin: bcrypt 校验 → 普通用户: 本地密码优先 → 无本地密码时降级 Emby 认证 + 自动补存 hash
2. 过期用户**可以登录**（前端显示过期提示 + 兑换入口）

**注册流程**：
1. 读取 `registration_mode` → `"invite"`: 验证兑换码 → `"open"`: 读取 `default_trial_days`
2. 如果开启 `email_verification`：校验邮箱验证码
3. 创建 Emby 用户 → 创建本地用户（含 bcrypt hash）→ 签发 JWT
4. 火忘式通知 Bot（新用户注册）

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

- `CreateRedemptionCode(maxUses, defaultDays, expiresAt)` — 生成 16 字符 hex 码
- `GetRedemptionCodes(page, pageSize, showAll)` — showAll=false 过滤已失效
- `ValidateCode(code)` — 查找 + IsValid()
- `UseCode(code)` — 原子递增 usedCount

### 5.4 RedemptionService (`services/redemption.go`)

**核心方法 `RedeemCode(userID, code)`**：
1. 开启事务后查询兑换码并校验 `IsValid()`
2. 在事务中检查 `redemptions(userId, code)` 是否已存在，存在则返回 `ErrRedemptionDuplicate`
3. 查询用户并计算新 ExpiresAt，按需调用 Emby 解封，仅更新 `expiresAt/embyDisabled`
4. 先插入 Redemption 记录（依赖 `redemptions(userId, code)` 唯一约束兜底并发重复兑换）
5. 原子递增 usedCount（`WHERE usedCount < maxUses AND (expiresAt IS NULL OR expiresAt > now)`）→ 提交

### 5.5 SettingService (`services/setting.go`)

- `GetSetting(key)` / `SetSetting(key, value)` — 带值校验
- `GetRegistrationMode()` — 默认 `"open"`
- `GetDefaultTrialDays()` — 默认 `7`

### 5.6 SystemService (`services/system.go`)

- `GetSystemInfo()` — 统计：用户数、活跃数、兑换码数
- `CheckExpiredUsers()` — **cron 核心**：查询 `expiresAt < NOW() AND embyDisabled = false` → 调用 Emby `SetUserPolicy(IsDisabled: true)` → 设置 `EmbyDisabled = true`

### 5.7 EmbyService (`services/emby.go`)

Emby 媒体服务器 HTTP 客户端，10 秒超时。

| 方法 | Emby 端点 | 用途 |
|------|-----------|------|
| `AuthenticateUser` | `POST /emby/Users/AuthenticateByName` | 用户认证 |
| `CreateEmbyUser` | `POST /emby/Users/New` | 创建账号 |
| `UpdateUserPassword` | `POST /emby/Users/{id}/Password` | 修改密码 |
| `SetUserPolicy` | `POST /emby/Users/{id}/Policy` | 封禁/解封（IsDisabled） |
| `GetMediaStats` | `GET /emby/Items/Counts` | 媒体库统计 |
| `GetUsers` | `GET /emby/Users` | 连接测试 |

**认证方式**：`X-Emby-Token` 头

### 5.8 MediaService (`services/media.go`)

- `GetEmbyConfig()` — 返回公开的 Emby URL（`NEXT_PUBLIC_EMBY_URL` 优先，回退 `EMBY_URL`）
- `GetMediaStats()` — 5 分钟 RWMutex 缓存层

### 5.9 SubscriptionService (`services/subscription.go`)

- `CreateSubscription(userID, type, name, tmdbId)` — 创建 PENDING 状态 + 火忘式通知 Bot
- `ApproveSubscription(id)` — 调用 MoviePilot → 设为 APPROVED（MP 失败不阻塞审批，错误存入 mpError）
- `RejectSubscription(id)` — 设为 REJECTED

### 5.10 MoviePilotClient (`services/moviepilot.go`)

- `IsConfigured()` — 检查三个环境变量是否都设置
- `login()` — `POST /api/v1/login/access-token`（form-urlencoded）
- `CreateSubscription(type, name, tmdbId)` — `POST /api/v1/subscribe/`（type 转中文：movie→电影, tv→电视剧）

### 5.11 EmailService (`services/email.go`)

邮箱验证码发送、校验和清理服务，基于 SMTP。

- `SendVerificationCode(email, ip, codeType)` — 生成 6 位随机验证码 → 按类型频率限制（每邮箱/每 IP 每日上限）→ SMTP 发送
- `VerifyCode(email, code, codeType)` — 按类型校验验证码是否有效且未过期
- `CleanupExpired()` — 删除过期验证码（cron 调用）

**频率限制**：
- 每邮箱每日：`EMAIL_CODE_DAILY_LIMIT`（默认 5，按 `codeType` 隔离计数）
- 每 IP 每日：`EMAIL_CODE_IP_DAILY_LIMIT`（默认 15）
- 验证码有效期：`EMAIL_CODE_EXPIRY_MINUTES`（默认 10 分钟）

### 5.12 BotNotifier (`services/notifier.go`)

火忘式 HTTP 推送通知服务，将事件推送给 Telegram Bot。

**通知类型**：
| 方法 | Bot 端点 | 触发时机 |
|------|----------|----------|
| `NotifyNewSubscription` | `POST /notify/subscription` | 用户创建求片订阅 |
| `NotifyNewRegistration` | `POST /notify/registration` | 新用户注册 |
| `NotifyRanking` | `POST /notify/ranking` | 排行榜生成完成 |

**认证方式**：`X-Internal-Secret` 头（值 = `INTERNAL_API_SECRET`）

### 5.13 PlaybackRankingService (`services/playback_ranking.go`)

从 Emby PlaybackActivity 数据库生成播放排行。

- `GenerateRanking(period)` — 查询 Emby 活动日志 → 按播放次数/时长排名 → 存入数据库 → 通知 Bot
- `GetLatestRanking()` — 获取最新一期排行
- `GetRankingHistory(page, pageSize)` — 分页查询历史排行
- `PreviewRanking(period)` — 预览排行（不持久化）

**支持周期**：`daily`（日榜）、`weekly`（周榜）

### 5.14 PaymentService (`services/payment.go`)

Stripe 一次性支付流程管理。

- `CreateCheckoutSession(userID, planID)` — 创建 Stripe Checkout Session → 存储 Payment 记录（pending）
- `HandleWebhook(payload, signature)` — 处理 Stripe Webhook → 更新 Payment 状态 → 成功时自动延长用户有效期
- Plan CRUD — `GetPlans`, `CreatePlan`, `UpdatePlan`, `DeletePlan`
- `GetPayments(page, pageSize)` — 支付记录查询

### 5.15 错误定义 (`services/errors.go`)

统一的业务错误定义，用于 Service → Handler 的错误传递。

### 5.16 TelegramService (`services/telegram.go`)

Telegram 账号绑定与 Bot 自助能力服务。

- `GenerateBindCode(userID)` — 生成 6 位绑定验证码（5 分钟有效），并清理该用户旧验证码
- `VerifyBind(telegramID, code)` — 校验验证码并绑定 Telegram ID（事务 + 行锁）
- `Unbind(userID)` — 解绑 Telegram ID
- `GetAccountInfo(telegramID)` — 查询绑定用户账号状态
- `RedeemByTelegram(telegramID, code)` — 复用 `RedemptionService` 完成续期兑换
- `ResetPassword(telegramID, newPassword)` — 通过 Telegram 身份重置 Ember/Emby 密码
- `CleanupExpiredBindCodes()` — 删除过期绑定码（cron 调用）

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
| GET | `/api/v1/admin/settings` | 获取所有配置 |
| PUT | `/api/v1/admin/settings/:key` | 更新配置 |
| GET | `/api/v1/admin/redemptions` | 全部兑换历史 |
| GET | `/api/v1/admin/subscriptions` | 全部订阅 |
| PUT | `/api/v1/admin/subscriptions/:id/approve` | 审批通过 |
| PUT | `/api/v1/admin/subscriptions/:id/reject` | 审批拒绝 |
| DELETE | `/api/v1/admin/subscriptions/:id` | 删除订阅 |
| GET | `/api/v1/admin/sessions` | 活跃会话 |
| GET | `/api/v1/admin/plans` | 方案列表 |
| POST | `/api/v1/admin/plans` | 创建方案 |
| PUT | `/api/v1/admin/plans/:id` | 更新方案 |
| DELETE | `/api/v1/admin/plans/:id` | 删除方案 |
| GET | `/api/v1/admin/payments` | 全部支付记录 |
| GET | `/api/v1/admin/system/info` | 系统统计 |
| POST | `/api/v1/admin/system/test-emby` | 测试 Emby 连接 |
| POST | `/api/v1/cron/check-expired` | 手动执行过期检查 |
| POST | `/api/v1/cron/generate-ranking` | 手动生成排行 |
| POST | `/api/v1/rankings/preview` | 排行预览 |

### 内部服务路由（InternalAuth 中间件，Bot 调用）

| 方法 | 路径 | 用途 |
|------|------|------|
| PUT | `/api/v1/internal/subscriptions/:id/approve` | 审批通过 |
| PUT | `/api/v1/internal/subscriptions/:id/reject` | 审批拒绝 |
| GET | `/api/v1/internal/settings/:key` | 读取配置 |
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
- `api/admin.ts` — 管理后台全部接口（users, codes, settings, subscriptions, plans, payments, sessions, rankings）
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
| `TELEGRAM_ADMIN_CHAT_ID` | ✅ | — | 管理员 Chat ID |
| `TELEGRAM_GROUP_CHAT_ID` | — | — | 群组 Chat ID（排行榜推送）|
| `TELEGRAM_WEBHOOK_SECRET` | ✅ | — | Webhook 签名校验 |
| `INTERNAL_API_SECRET` | ✅ | — | 与 Go API 共享密钥 |
| `WEBHOOK_URL` | ✅ | — | 公开 HTTPS Webhook URL |
| `API_URL` | — | `http://localhost:8080` | Ember API 地址 |
| `BOT_PORT` | — | `8000` | Bot 服务端口 |

---

## 10. 定时任务

| 任务 | 调度 | 控制变量 | 说明 |
|------|------|----------|------|
| 过期用户检查 | `CRON_SCHEDULE`（默认 `0 2 * * *`）| `CRON_ENABLED` | 封禁过期 Emby 账号 |
| 验证码清理 | `0 3 * * *` | `CRON_ENABLED` | 删除过期 EmailVerification + TelegramBindCode |
| 日榜生成 | `RANKING_DAILY_SCHEDULE`（默认 `0 20 * * *`）| `RANKING_CRON_ENABLED` | 从 Emby 生成日播放排行 |
| 周榜生成 | `RANKING_WEEKLY_SCHEDULE`（默认 `30 20 * * 0`）| `RANKING_CRON_ENABLED` | 从 Emby 生成周播放排行 |

**通用配置**：
| 环境变量 | 默认值 | 说明 |
|----------|--------|------|
| `CRON_ENABLED` | `"true"` | 是否启用（过期检查 + 验证码清理）|
| `CRON_SCHEDULE` | `"0 2 * * *"` | 过期检查 cron 表达式 |
| `CRON_TIMEZONE` | `"Asia/Shanghai"` | 时区 |
| `RANKING_CRON_ENABLED` | `"false"` | 是否启用排行榜生成 |
| `RANKING_DAILY_SCHEDULE` | `"0 20 * * *"` | 日榜 cron 表达式 |
| `RANKING_WEEKLY_SCHEDULE` | `"30 20 * * 0"` | 周榜 cron 表达式 |

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
| `NEXT_PUBLIC_EMBY_URL` | — | — | Emby 公开 URL（给前端用）|

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
| `SMTP_FROM` | — | — | 发件人（回退 `SMTP_USERNAME`）|
| `EMAIL_CODE_EXPIRY_MINUTES` | — | `10` | 验证码有效期（分钟）|
| `EMAIL_CODE_DAILY_LIMIT` | — | `5` | 每邮箱每日发送上限 |
| `EMAIL_CODE_IP_DAILY_LIMIT` | — | `15` | 每 IP 每日发送上限 |

### Bot 通信

| 变量 | 必需 | 默认值 | 说明 |
|------|------|--------|------|
| `BOT_NOTIFY_URL` | — | — | Bot 通知 Webhook 地址 |
| `INTERNAL_API_SECRET` | — | — | 内部通信共享密钥 |

### Stripe 支付

| 变量 | 必需 | 默认值 | 说明 |
|------|------|--------|------|
| `STRIPE_SECRET_KEY` | — | — | Stripe API 密钥 |
| `STRIPE_WEBHOOK_SECRET` | — | — | Stripe Webhook 签名密钥 |
| `STRIPE_SUCCESS_URL` | — | — | 支付成功跳转 URL |
| `STRIPE_CANCEL_URL` | — | — | 支付取消跳转 URL |

### 定时任务

| 变量 | 必需 | 默认值 | 说明 |
|------|------|--------|------|
| `CRON_ENABLED` | — | `"true"` | 启用定时任务 |
| `CRON_SCHEDULE` | — | `"0 2 * * *"` | 过期检查表达式 |
| `CRON_TIMEZONE` | — | `"Asia/Shanghai"` | 时区 |
| `RANKING_CRON_ENABLED` | — | `"false"` | 启用排行榜生成 |
| `RANKING_DAILY_SCHEDULE` | — | `"0 20 * * *"` | 日榜表达式 |
| `RANKING_WEEKLY_SCHEDULE` | — | `"30 20 * * 0"` | 周榜表达式 |

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
