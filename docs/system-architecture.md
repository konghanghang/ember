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
- Telegram Bot（订阅审批、新用户通知、排行榜推送、欢迎消息、账号绑定/查询/续期、媒体库统计）
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
│     │  ├─ plan_group.go        # PlanGroup（套餐分组）
│     │  ├─ payment.go           # Payment（支付记录）
│     │  ├─ playback_ranking.go  # PlaybackRanking（播放排行快照）
│     │  ├─ media_quality_cache.go # MediaQualityCache（媒体质量缓存）
│     │  ├─ client_blacklist.go  # ClientBlacklist（客户端黑名单）
│     │  ├─ device_action.go     # DeviceAction（设备操作日志）
│     │  ├─ media_gap.go         # MediaGap（缺集工单）
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
│     │  ├─ auth/
│     │  │  ├─ service.go        # AuthService（共享装配 / 模板权限应用）
│     │  │  ├─ login.go          # AuthService（登录链路编排）
│     │  │  ├─ register.go       # AuthService（注册链路编排）
│     │  │  ├─ register_persist.go # AuthService（注册落库事务）
│     │  │  └─ register_notify.go # AuthService（注册通知副作用）
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
│     │  ├─ device.go            # DeviceService（设备管理）
│     │  ├─ tvcalendar/
│     │  │  └─ service.go        # TVCalendarService（追剧日历）
│     │  └─ *_errors.go          # 领域错误定义（按业务拆分）
│     ├─ handlers/               # HTTP 处理层（Gin）
│     │  ├─ auth.go              # 登录 / 注册
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
│     │  └─ tv_calendar.go       # 追剧日历
│     ├─ middleware/
│     │  ├─ jwt.go               # JWTAuth + AdminOnly + UserOnly
│     │  └─ internal_auth.go     # InternalAuth（Bot 内部通信认证）
│     ├─ common/
│     │  ├─ jwt.go               # Token 生成/解析（HS256, 7天有效）
│     │  └─ utils.go             # CalculateExpiryDate
│     └─ db/
│        └─ db.go                # DB 初始化 + VerifySchema + Bootstrap（启动期不再调用 AutoMigrate）
├─ web/                          # Vue 3 前端
│  ├─ src/
│  │  ├─ api/                    # Axios 请求层
│  │  │  ├─ request.ts           # 基础配置：baseURL=/api/v1, 401拦截
│  │  │  ├─ auth.ts              # login, register, getRegistrationMode
│  │  │  ├─ user.ts              # redeem, redemptions, tmdb
│  │  │  ├─ admin.ts             # 管理后台全部接口
│  │  │  └─ console.ts           # 统一认证路由接口（profile, account-links, subscriptions, payments, rankings 等）
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
│  │  │  ├─ auth.ts              # Pinia: token + role (localStorage 持久化)
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
│  │     │  ├─ AccountCenterView.vue # 账号中心（邮箱 / 密码 / Telegram 绑定 / 帮助资源）
│  │     │  ├─ ProfileAnalyticsView.vue # 我的画像
│  │     │  ├─ SubscriptionsView.vue  # 求片订阅
│  │     │  ├─ NewSubscriptionView.vue # 新建订阅
│  │     │  ├─ TVCalendarView.vue # 追剧日历
│  │     │  ├─ LibraryView.vue   # 兼容壳（`/console/library` 路由已重定向到概览页）
│  │     │  ├─ RankingsView.vue  # 播放排行
│  │     │  └─ RenewalCenterView.vue # 续费中心（支付 + 兑换码）
│  │     └─ admin/               # 管理后台
│  │        ├─ UsersView.vue     # 用户管理（筛选 / 后台创建 / 编辑）
│  │        ├─ UserPlaybackProfilesView.vue # 用户画像总览
│  │        ├─ UserPlaybackProfileView.vue # 用户画像
│  │        ├─ RedemptionCenterView.vue # 兑换中心（兑换码池 + 兑换记录）
│  │        ├─ RedemptionCodesView.vue # 兑换码管理
│  │        ├─ RedemptionHistoryView.vue # 兑换历史
│  │        ├─ PaymentCenterView.vue # 支付中心（付费方案 + 支付记录）
│  │        ├─ PlanGroupsView.vue # 套餐分组管理
│  │        ├─ SettingsView.vue  # 设置中心
│  │        ├─ PlansView.vue     # 方案管理
│  │        ├─ PaymentsView.vue  # 支付记录审计
│  │        ├─ SessionsView.vue  # 活跃会话
│  │        ├─ PlaybackHistoryView.vue # 播放历史
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
      ├─ server.py               # FastAPI + Telegram Application（Webhook 模式，lifespan 负责 HTTP client 生命周期）
      ├─ handlers/
      │  ├─ telegram_handler.py  # 消息/回调处理（审批、欢迎消息、菜单清理）
      │  └─ search_cache.py      # 搜索会话缓存
      ├─ formatters/
      │  └─ message_formatter.py # Telegram 消息格式化
      └─ clients/
         └─ api_client.py        # Ember API 内部客户端（共享 AsyncClient 连接池）
```

---

### 3.1 Web 共享组件层

`services/web/src/components/ember/` 是当前 Web 端的 Ember 基础组件层，职责是把后台与控制台高频重复的 UI 骨架收口为稳定契约，而不是把业务逻辑搬进组件。

- `layout/`
  - `EmberPageHeaderCard`：统一页面标题、说明、统计 badge、右侧 actions/tabs slot
  - `EmberFilterPanel`：统一筛选区容器、字段区布局、按钮区对齐
  - `EmberSegmentTabs`：统一页内单选分段切换，当前以 `radiogroup / radio` 语义提供共享键盘交互约定，但不默认承诺 `tabpanel` 语义
- `filters/`
  - `EmberSearchInput`、`EmberSelectField`、`EmberDateField`、`EmberDateRangeField`
  - 负责筛选字段的 Ember 风格外观与交互基线，消费共享 field token，不承载页面查询逻辑
- `data-display/`
  - `EmberTableCard`：统一表格容器、可选标题头、表头样式和分页区
  - `EmberMetricCard`：统一简单统计卡基线
- `forms/`
  - `EmberFormDialog`：统一弹窗表单容器和 footer 区域
- `feedback/`
  - `EmberEmptyStateCard`：统一中性 / 风险态空状态容器和可选动作区

当前已接入这套基础组件的后台页面包括：

- 列表/表单类页面：
  - `services/web/src/views/admin/UsersView.vue`
  - `services/web/src/views/admin/PaymentsView.vue`
  - `services/web/src/views/admin/PlaybackHistoryView.vue`
  - `services/web/src/views/admin/RedemptionCodesView.vue`
  - `services/web/src/views/admin/RedemptionHistoryView.vue`
  - `services/web/src/views/admin/UserPlaybackProfilesView.vue`
  - `services/web/src/views/admin/PlansView.vue`
  - `services/web/src/views/admin/PlanGroupsView.vue`
- 容器型中心页：
  - `services/web/src/views/admin/PaymentCenterView.vue`
  - `services/web/src/views/admin/RedemptionCenterView.vue`
  - `services/web/src/views/admin/SettingsView.vue`
  - `services/web/src/views/admin/SessionsView.vue`
  - `services/web/src/views/admin/MediaQualityView.vue`

当前已接入这套基础组件的控制台页面包括：

- `services/web/src/views/console/SubscriptionsView.vue`
- `services/web/src/views/console/NewSubscriptionView.vue`
- `services/web/src/views/console/RenewalCenterView.vue`
- `services/web/src/views/console/DashboardView.vue`
- `services/web/src/views/console/RankingsView.vue`

边界约束：

- 页面 view 继续保留接口调用、路由状态、筛选参数、弹窗状态和数据编排。
- Ember 基础组件只承载稳定 UI 契约，不侵入 store 和 API 请求。
- 强业务、强视觉特例页面仍允许保留页面内实现，例如 `services/web/src/views/console/TVCalendarView.vue`。

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
| PlanGroup | *string | planGroup | 用户显式绑定的套餐分组 key；为空时按系统默认分组计算可见/可购套餐 |
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
- `User` 只承载 `users` 表真实列；`planGroupName`、`effectivePlanGroup` 等展示态字段只存在于 `services/user` 查询 DTO，不再混入持久化模型

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
| RegistrationPlanGroup | *string(50) | registrationPlanGroup | 注册场景专用套餐分组 key（可空；仅注册时生效，续期忽略） |
| Notes | string(500) | notes | 备注（可选，用于记录用途或来源） |
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
- 业务配置：`registration_mode`、`default_trial_days`、`notify_group_link`、`telegram_welcome_message_template`、`email_verification`、`stripe_allowed_payment_methods`
- 媒体集成：`EMBY_URL`、`EMBY_API_KEY`、`NEXT_PUBLIC_EMBY_URL`（历史键名，数据库配置项）、`TMDB_API_KEY`、`MOVIEPILOT_URL`、`MOVIEPILOT_API_KEY`
- 邮件服务：`SMTP_HOST`、`SMTP_PORT`、`SMTP_USERNAME`、`SMTP_PASSWORD`、`SMTP_FROM`、`EMAIL_CODE_EXPIRY_MINUTES`、`EMAIL_CODE_DAILY_LIMIT`、`EMAIL_CODE_IP_DAILY_LIMIT`
- 通知：`BOT_NOTIFY_URL`
- 只读展示：`DATABASE_URL`、`JWT_SECRET`、`INTERNAL_API_SECRET`、`ADMIN_USERNAME`、`ADMIN_PASSWORD`、`TELEGRAM_BOT_TOKEN`、`TELEGRAM_WEBHOOK_SECRET`、`WEBHOOK_URL`、`PORT` 等

### 4.5 Subscription（订阅求片）

**表名**: `subscriptions` | **文件**: `models/subscription.go`

| 字段 | 类型 | 列名 | 说明 |
|------|------|------|------|
| ID | string(25) | id | CUID |
| UserID | string(25) | userId | 用户 ID |
| Type | MediaType | type | `"MOVIE"` 或 `"TV"` |
| Name | string(255) | name | 媒体名称 |
| TmdbID | string | tmdbId | TMDB ID |
| Season | int | season | 季号，`0` 表示整剧 |
| PosterPath | *string(500) | posterPath | 海报 URL |
| Status | SubscriptionStatus | status | `PENDING`/`APPROVED`/`REJECTED`/`INGESTED` |
| Note | *string | note | 用户备注 |
| RetryFromID | *string(25) | retryFromId | 拒绝后重新发起时指向上一条 `REJECTED` 订阅，可空 |
| MpError | *string(500) | mpError | MoviePilot 同步错误 |
| RejectReason | *string | rejectReason | 管理员拒绝原因 |
| ReviewedAt | *time.Time | reviewedAt | 审核时间（通过/拒绝） |
| IngestedAt | *time.Time | ingestedAt | 真实入库时间（由 Emby webhook 或管理员校验命中后收口写入） |
| CreatedAt | time.Time | createdAt | 自动 |
| UpdatedAt | time.Time | updatedAt | 自动 |

**状态流转**：
- 用户创建后进入 `PENDING`
- 管理员审核通过后转 `APPROVED`，并记录 `reviewedAt`
- 管理员拒绝后转 `REJECTED`，必须写入 `rejectReason` 与 `reviewedAt`
- 用户可基于自己的 `REJECTED` 记录重新发起一条新的 `PENDING` 订阅，新记录写入 `retryFromId`，原拒绝记录保持历史不改写
- Emby 真实入库事件命中已通过订阅后转 `INGESTED`，并写入 `ingestedAt`
- 对历史漏回写记录，管理员可主动触发 Emby 校验；只有命中真实资源时，`APPROVED` 才能收口为 `INGESTED`

**唯一约束**：
- `subscriptions` 不再使用全局唯一索引 `uk_subscription_media`
- 活跃状态唯一索引 `uq_subscriptions_active_media` 只约束 `status IN ('PENDING','APPROVED','INGESTED')` 的 `(type, tmdbId, season)`
- `REJECTED` 历史记录不占用唯一位，允许同一作品在被拒绝后重新提交，但任意时刻同一作品仍只能存在一条活跃订阅

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
| Currency | string(3) | currency | 币种（当前支持 `"usd"` / `"hkd"` / `"cny"`）|
| PlanGroup | string(50) | planGroup | 套餐所属分组 key（由应用层校验其存在性与删除约束）|
| IsActive | bool | isActive | 是否启用（默认 true，DELETE 接口仅置为 false 作为软删除）|
| SortOrder | int | sortOrder | 排序（默认 0）|
| CreatedAt | time.Time | createdAt | 自动 |
| UpdatedAt | time.Time | updatedAt | 自动 |

**设计要点**：
- `Plan` 只承载 `plans` 表真实列；`planGroupName` 这类 join 后的展示字段由 `services/payment` 查询 DTO 承载，避免普通 `First/Find` 误查不存在列

### 4.7.1 PlanGroup

**表名**: `plan_groups` | **文件**: `models/plan_group.go`

| 字段 | 类型 | 列名 | 说明 |
|------|------|------|------|
| Key | string(50) | key | 分组稳定标识，主键 |
| Name | string(100) | name | 分组展示名称 |
| Description | string(500) | description | 分组说明 |
| IsDefault | bool | isDefault | 是否默认分组（全局唯一） |
| SortOrder | int | sortOrder | 排序 |
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
| CheckoutURL | string | checkoutUrl | 待支付订单复用的 Stripe Checkout 链接 |
| Amount | int64 | amount | 金额（分）|
| Currency | string | currency | 支付币种快照 |
| Days | int | days | 购买天数 |
| Status | PaymentStatus | status | `pending`/`completed`/`expired`/`failed` |
| ExpiresAt | *time.Time | expiresAt | 本地待支付订单过期时间（默认 30 分钟） |
| CreatedAt | time.Time | createdAt | 自动 |
| UpdatedAt | time.Time | updatedAt | 自动 |

### 4.9 PlaybackRanking（播放排行快照）

**表名**: `playback_rankings` | **文件**: `models/playback_ranking.go`

| 字段 | 类型 | 列名 | 说明 |
|------|------|------|------|
| ID | string(25) | id | CUID |
| BatchID | string(25) | batchId | 同一次生成的排行榜批次 ID |
| Period | RankingPeriod | period | `"daily"` 或 `"weekly"` |
| Category | RankingCategory | category | `"media_movie"` 或 `"media_episode"` |
| Rank | int | rank | 排名 |
| ItemKey | string(128) | itemKey | 稳定聚合键（电影使用 `ItemId`；剧集使用回查 Emby 条目详情得到的 `SeriesId`） |
| ItemSourceType | string(32) | itemSourceType | 聚合键来源（如 `movie_item` / `series` / `episode_item`） |
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
| LastEpisodeIngestedAt | *time.Time | lastEpisodeIngestedAt | 最近一次新剧集入库时间（用于轻量同步活跃剧） |
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

### 5.1 AuthService (`services/auth/service.go`, `services/auth/login.go`, `services/auth/register.go`, `services/auth/register_persist.go`, `services/auth/register_notify.go`)

**登录流程**：
1. 通过 `ConfigService` 读取登录保护公开配置；若 `turnstile_login_enabled=true`，则先校验 `turnstileToken`
2. Turnstile 校验通过后再查找用户 → Admin: bcrypt 校验 → 普通用户: 本地密码优先 → 无本地密码时降级 Emby 认证 + 自动补存 hash
3. 过期用户**可以登录**（前端显示过期提示 + 兑换入口）

**注册流程**：
1. 通过 `ConfigService` 读取 `registration_mode` → `"invite"`: 调用注册场景兑换码校验（会额外校验 `registrationPlanGroup` 仍存在）→ `"open"`: 读取 `default_trial_days`
2. 如果 `ConfigService` 解析的 `email_verification` 开启，且 SMTP 已配置：校验邮箱验证码
3. 创建 Emby 用户 → 创建本地用户（含 bcrypt hash）；invite 模式且兑换码绑定 `registrationPlanGroup` 时，把该 key 写入 `users.planGroup`，未绑定时继续跟随系统默认分组
4. invite 模式且兑换码绑定 `templateUserId` 时：按白名单字段复制模板用户 Emby Policy
5. 签发 JWT
6. 火忘式通知 Bot（新用户注册）

**关键 struct**：
- `RegisterUserRequest{Username, Password, Email, Code, EmailCode}` — Code/EmailCode 可选
- `LoginRequest{Username, Password, TurnstileToken}`
- `LoginResponse{Token, User, IsExpired}`

### 5.2 UserService (`services/user/service.go`, `services/user/admin.go`, `services/user/profile.go`, `services/user/password.go`, `services/user/password_reset.go`)

- `GetUsers(page, pageSize, search, isActive, expiresAfter, embyStatus, planGroup)` — 分页搜索（`expiresAfter` 格式 `YYYY-MM-DD`，筛选 `expiresAt > expiresAfter`；`embyStatus` 支持 `available/disabled/unlinked`；`planGroup` 按“有效套餐分组”过滤：用户显式分组优先，否则回退系统默认分组）
- `UpdateUserByAdmin(userID, req)` — 管理员更新用户邮箱/状态/套餐组/到期时间；`planGroup` 不传表示不改，传合法 key 表示显式绑定，传空字符串表示清空显式绑定并改为跟随系统默认分组；有效分组变化后会同步把该用户关联的 `pending` 支付标记为 `expired`
- `ExtendExpiry(userID, days)` — 已过期从 now 起算，未过期从 ExpiresAt 叠加
- `GetProfile(userID)` — 获取用户个人资料
- `UpdateProfile(userID, email)` — 更新用户个人资料
- `UpdateEmail(userID, newEmail)` — 更新用户邮箱
- `UpdatePassword(userID, old, new)` — Emby + 本地 hash 同步
- `ResetPassword(userID, new)` — 管理员重置，Emby + 本地 hash 同步
- `ResetPasswordByCode(email, code, newPassword)` — 通过注入的验证码校验能力完成邮箱验证码重置，不再在方法内部自行构造 EmailService
- `ToggleUserStatus(userID)` — 翻转 IsActive

### 5.3 RedemptionCodeService (`services/redemption/code_service.go`)

- `CreateRedemptionCode(maxUses, defaultDays, expiresAt, templateUserId, registrationPlanGroup, notes)` — 生成 16 字符 hex 码；若传 `registrationPlanGroup`，创建时校验分组存在
- `CreateRedemptionCodesBatch(count, maxUses, defaultDays, expiresAt, templateUserId, registrationPlanGroup, notes)` — 批量生成兑换码，单次最多 100 个，整批事务提交
- `GetRedemptionCodes(page, pageSize, showAll, code, status, templateUserId, registrationPlanGroup)` — 支持按兑换码关键字、状态（`active|expired|exhausted`）、模板用户和注册套餐分组过滤，并返回 `notes`、`registrationPlanGroupName`；未指定 `status` 且 `showAll=false` 时仅返回当前仍可兑换的码
- `GetUserTemplates()` — 获取可选模板用户列表（启用且未过期）
- `ValidateRegistrationCode(code)` — 注册场景兑换码校验；查找 + IsValid()，且当 `registrationPlanGroup` 非空时强校验分组仍存在
- `ValidateRenewalCode(code)` — 续期场景兑换码校验；只查找 + IsValid()，忽略 `registrationPlanGroup`
- `UseCode(code)` — 原子递增 usedCount

### 5.4 RedemptionService (`services/redemption/service.go`)

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

**关键职责**：
- 配置定义注册表（标签、分组、类型、校验、默认值）
- 读取策略由配置定义控制：已托管的运行期集成配置优先数据库并可禁用 env 回退；部署边界配置仍保留 env / default 解析
- 敏感值加密：`CONFIG_ENCRYPTION_KEY`
- 运行期配置中心 API 的后端基础设施

### 5.6 SystemService (`services/system/service.go`, `services/system/expiry.go`)

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

- `GetEmbyConfig()` — 优先返回设置中心里的前端 Emby 地址（配置键沿用 `NEXT_PUBLIC_EMBY_URL`），为空时回退 `EMBY_URL`，供控制台拼接 Emby 图片与地址展示
- `GetMediaStats()` — 5 分钟 RWMutex 缓存层
- `GetLatestItems(embyUserID, itemType, limit)` — 通过 Emby `/Users/{userId}/Items/Latest` 获取最近入库媒体，并做短 TTL 去重缓存

### 5.9 SubscriptionService (`services/subscription.go`)

- `CreateSubscription(userID, type, name, tmdbId, season)` — 创建 PENDING 状态 + 火忘式通知 Bot；按活跃状态的 `type + tmdbId + season` 去重，历史 `REJECTED` 不阻止新建
- `ResubmitSubscription(userID, rejectedSubscriptionID, note)` — 基于当前用户自己的 `REJECTED` 记录重新发起一条新的 `PENDING` 记录，复用原媒体信息并写入 `retryFromId`；原记录保持 `REJECTED`
- `ApproveSubscription(id)` — 调用 MoviePilot → 设为 APPROVED（MP 失败不阻塞审批，错误存入 mpError；`season>0` 时透传季号，`season=0` 不传季参数）
- `RejectSubscription(id)` — 设为 REJECTED

### 5.10 DeviceService (`services/device.go`)

- `GetDevices(req)` — 设备列表（支持 userId/clientName/isBlacklisted 过滤 + 分页）
- `GetBlacklist()` / `AddClientToBlacklist()` / `RemoveClientFromBlacklist()` — 客户端黑名单管理
- `LogoutDevice(deviceID)` — 强制注销单设备
- `LogoutBlacklistedDevices()` — 批量注销黑名单设备
- `GetStats()` — 客户端分布、设备分布、黑名单数量、活跃会话数
- `GetDeviceActions(limit)` — 最近设备操作日志

### 5.11 MediaGapService (`services/mediagap/service.go`)

- `ScanMediaGaps(tmdbId?)` — 扫描 Emby 连载剧的已激活季，创建/更新/核销缺集工单；后台管理入口已改为异步触发，扫描本体在后台独立上下文中执行
- `ListGroupedMediaGaps(query)` — 按剧聚合缺集工单，后端完成分组、排序、分页与摘要统计，供聚合视图直接消费
- `SearchGap(id)` — 调用 MoviePilot 搜索当前缺集候选，优先按单集查询，未命中时回退整季查询；写入 `searchSnapshot` 与 `lastSearchedAt`，候选摘要保留发布时间等展示字段
- `DispatchGap(id, candidate)` — 调用 MoviePilot 下载入口下发已选候选资源；仅在下发成功后写入 `dispatchSnapshot`、`requestedAt` 并推进为 `REQUESTED`
- `IgnoreGap(id, reason)` — 将单条缺集工单标记为 `IGNORED`
- `MarkIngestedByWebhook(payload)` — Emby webhook 命中缺集工单后核销为 `INGESTED`

### 5.12 MoviePilotClient (`integrations/moviepilot/client.go`)

- `IsConfigured()` — 检查 `MOVIEPILOT_URL` 与 `MOVIEPILOT_API_KEY` 是否齐全
- `TestConnection()` — `GET /api/v1/site/`，请求头使用 `X-API-KEY`
- `CreateSubscription(type, name, tmdbId, season)` — `POST /api/v1/subscribe/`，请求头使用 `X-API-KEY`（`type` 转中文：movie→电影, tv→电视剧；`season>0` 时透传季号）
- `SearchGapCandidates(seriesName, season, episode)` — `GET /api/v1/search/title`，优先搜索 `SxxExx` 单集，空结果时回退 `Sxx` 整季包；候选按做种数、体积排序
- `DispatchGapCandidate(candidatePayload)` — `POST /api/v1/download/add`，将选中的资源快照透传给 MoviePilot 下载入口

### 5.13 EmailService (`services/email/service.go`, `services/email/verification.go`, `services/email/sender.go`)

邮箱验证码发送、校验和清理服务，基于 SMTP。

- `SendVerificationCode(email, ip, codeType)` — 生成 6 位随机验证码 → 按类型频率限制（每邮箱/每 IP 每日上限）→ SMTP 发送
- `VerifyCode(email, code, codeType)` — 按类型校验验证码是否有效且未过期
- `CleanupExpired()` — 删除过期验证码（cron 调用）
- `IsEnabled()` — 通过 `ConfigService` 解析 `email_verification`，并叠加 SMTP 完整性判断
- `TestConnection()` — 基于当前 SMTP 配置做连通性探活

**频率限制**：
- 每邮箱每日：`EMAIL_CODE_DAILY_LIMIT`（默认 5，按 `codeType` 隔离计数）
- 每 IP 每日：`EMAIL_CODE_IP_DAILY_LIMIT`（默认 15）
- 验证码有效期：`EMAIL_CODE_EXPIRY_MINUTES`（默认 10 分钟）

### 5.14 BotNotifier (`integrations/notifier/notifier.go`)

火忘式 HTTP 推送通知服务，将事件推送给 Telegram Bot。

**通知类型**：
| 方法 | Bot 端点 | 触发时机 |
|------|----------|----------|
| `NotifyNewSubscription` | `POST /notify/subscription` | 用户创建求片订阅 |
| `NotifySubscriptionApproved` / `NotifySubscriptionRejected` / `NotifySubscriptionIngested` | `POST /notify/subscription-result` | 用户订阅审核结果 / 入库结果 |
| `NotifyNewRegistration` | `POST /notify/registration` | 新用户注册 |
| `NotifyPaymentSuccess` | `POST /notify/payment` | Stripe 支付履约成功 |
| `NotifyRanking` | `POST /notify/ranking` | 排行榜生成完成 |

**认证方式**：`X-Internal-Secret` 头（值 = `INTERNAL_API_SECRET`）

### 5.15 PlaybackRankingService (`services/playback/ranking.go`)

从 Emby PlaybackActivity 数据库生成播放排行。

- `GenerateRanking(period)` — 校验 PlaybackActivity 基础字段 → 电影榜按 `ItemId` 聚合；剧集榜先按 episode `ItemId` 聚合，再回查 Emby 条目详情按 `SeriesId` 归并 → 存入数据库 → 通知 Bot
- `GetLatestRanking(period)` — 获取指定周期最近一批正式排行榜（按 `periodEnd` 排序，不按 `snapshotAt` 猜）
- `GetHistoryRanking(period, rangeStart, rangeEnd)` — 按统计周期查询历史排行；新格式按 `batchId` 读取，旧格式按 `snapshotAt` 兼容
- `NotifyRanking` 推送 payload 额外包含整期 `totalDuration`，用于 Telegram 展示当天/当周总播放时长
- `PreviewRanking(period)` — 即时预览当前周期排行（不持久化、不推送）

**支持周期**：`daily`（日榜）、`weekly`（周榜）

**实现约束**：

- 正式榜单按 `batchId` 组织，同一期电影榜和剧集榜共享同一批次
- 最新榜不再按 `category` 分开读取，统一返回整期榜单
- 聚合键不再使用 `ItemName`
- 电影榜直接依赖 PlaybackActivity 的 `ItemId`
- 当前 PlaybackActivity 不返回 `SeriesId` / `SeriesName`，剧集榜需额外回查 Emby 媒体详情后按 `SeriesId` 归并

### 5.15 PaymentService (`services/payment/service.go`)

Stripe 一次性支付流程管理。

- `GetPlanGroups()` / `CreatePlanGroup()` / `UpdatePlanGroup()` / `DeletePlanGroup()` — 后台套餐分组管理；默认分组全局唯一；分组存在性、引用检查（`plans` / `users` / `redemption_codes.registrationPlanGroup`）和默认分组切换收口都在应用层完成，切换默认分组时会同步收口跟随默认用户的 `pending` 支付
- `CreateCheckoutSession(userID, planID)` — 优先复用同方案 30 分钟内未过期的待支付订单；否则创建新的 Stripe Checkout Session → 通过 `ConfigService` 读取 `stripe_allowed_payment_methods` 决定是否显式限制支付方式 → 存储 Payment 记录（pending）；创建前会先解析用户“有效套餐分组”（显式分组优先，否则回退系统默认分组），再按该分组强校验方案归属
- `GetPlansForUser(userID)` — 登录态可购方案列表，仅返回当前用户有效分组下的启用套餐；`/api/v1/plans` 与 `/api/v1/payments/plans` 都要求认证并复用这条过滤结果
- `HandleWebhook(payload, signature)` — 处理 Stripe Webhook → 更新 Payment 状态 → 成功时自动延长用户有效期
- `fulfillPayment(sessionID, paymentIntentID, metadata)` — 事务更新 Payment/User；履约前会复核当前用户有效分组与套餐当前分组，不再允许旧 Checkout Session 跨组续期；提交成功后火忘式通知 Bot 推送管理员支付成功消息
- Plan CRUD — `GetPlans`, `CreatePlan`, `UpdatePlan`, `DeletePlan`（软删除：仅下架 `isActive=false`；后台支持按 `planGroup` 过滤；套餐必须绑定已存在的分组，分组变更会同步失效该套餐关联的 `pending` 支付）
- `GetPayments(page, pageSize)` — 支付记录查询

### 5.16 错误定义（按业务拆分）

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

### 5.17 TelegramService (`services/telegram/service.go`)

Telegram 账号绑定与 Bot 自助能力服务。

- `GenerateBindCode(userID)` — 生成 6 位绑定验证码（5 分钟有效），并清理该用户旧验证码
- `VerifyBind(telegramID, code)` — 校验验证码并绑定 Telegram ID（事务 + 行锁）
- `Unbind(userID)` — 解绑 Telegram ID
- `GetAccountInfo(telegramID)` — 查询绑定用户账号状态
- `RedeemByTelegram(telegramID, code)` — 复用 `RedemptionService` 完成续期兑换
- `ResetPassword(telegramID, newPassword)` — 通过 Telegram 身份重置 Ember/Emby 密码
- `SubscribeByTelegram(req)` — Bot 求片订阅入口；电影直接确认，电视剧先选季再提交，并透传 `season`；为保持既有体验，Bot 提交默认视为已确认库内已存在提示，不走 Web 二次确认弹窗
- `CleanupExpiredBindCodes()` — 删除过期绑定码（cron 调用）

### 5.18 TVCalendarService (`services/tvcalendar/service.go`)

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

### 5.19 PlaybackHistoryService (`services/playback/history.go`)

管理员播放历史查询服务，复用 Emby Playback Reporting 插件能力，支持分页和条件筛选。

- `GetPlaybackHistory(req)` — 支持 `username` / `keyword` / `startDate` / `endDate` / `page` / `pageSize`，兼容旧 `userId`
- 对 `keyword` 做白名单校验并转义，避免 SQL 注入
- 统一输出播放时长格式（`Xm` / `Xh Ym`）
- 插件不可用时返回统一错误：`Playback Reporting 查询失败`

### 5.20 UserPlaybackProfileService (`services/playback/profile.go`)

管理员/用户播放画像聚合服务，基于单个用户的 `PlaybackActivity` 记录输出摘要、分布和勋章结果。

- `GetUserProfile(ctx, userID, query)` — 支持 `range=today|7d|30d|90d|all`，也支持 `startDate/endDate` 自定义日期时间范围
- 先读取本地 `users` 表映射 `embyId`，未绑定时回退使用本地 `userId`
- 输出指标：`totalPlayCount` / `totalPlayDuration` / `activeDays` / `averagePlayDuration` / `lastPlayedAt`
- 输出分布：`hourlyDistribution` / `deviceDistribution` / `clientDistribution`
- 输出最近记录预览：`recentRecords`（最多 10 条）
- 画像标签包含行为标签和高阈值勋章，例如：`evening_viewer` / `steady_viewer` / `night_owl` / `weekend_warrior` / `hardcore_viewer`
- 自定义日期时间范围最大跨度限制为 `92` 天
- 关键日志包含 `userID` / `embyUserID` / `range` / 结果统计 / 耗时，便于排障

### 5.21 UserPlaybackProfileOverview (`services/playback/profile_list.go`)

管理员侧用户画像总览聚合能力，按用户维度汇总指定时间窗口内的播放活跃度，并输出分页列表。

- `GetUserProfilesOverview(ctx, query)` — 支持 `range`，也支持 `startDate/endDate` 自定义日期时间范围，并支持 `keyword` / `sortBy` / `sortOrder` / `page` / `pageSize`
- 聚合结果字段：`totalPlayDuration` / `totalPlayCount` / `activeDays` / `lastPlayedAt` / `peakHourLabel`
- 总览页标签只返回精简预览（默认前 2 个），避免列表噪音
- 默认按累计播放时长倒序，可切换按播放次数、活跃天数、最近播放排序
- 总览摘要包含：`userCount` / `totalPlayCount` / `totalPlayDuration`
- 自定义日期时间范围最大跨度限制为 `92` 天，和单用户画像保持一致

### 5.22 MediaQualityService (`services/media_quality.go`)

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
| GET | `/api/v1/login/protection-config` | 登录页公开保护配置（Turnstile 开关 / Site Key / Hostname） |
| POST | `/api/v1/user/register` | 注册（code/emailCode 可选）|
| POST | `/api/v1/register/send-code` | 发送邮箱验证码 |
| POST | `/api/v1/forgot-password/send-code` | 发送密码重置验证码 |
| POST | `/api/v1/forgot-password/reset` | 通过验证码重置密码 |
| GET | `/api/v1/register/mode` | 获取注册模式 |
| GET | `/api/v1/register/code/:code/validate` | 验证注册场景兑换码（会校验绑定的 `registrationPlanGroup` 仍存在） |
| POST | `/api/v1/webhooks/stripe` | Stripe Webhook 回调 |
| POST | `/api/v1/webhooks/emby?token=` | Emby 入库 Webhook（追剧日历） |
| GET | `/api/v1/tmdb/search?query=&type=` | TMDB 搜索 |
| GET | `/api/v1/tmdb/tv/:id/seasons` | TMDB 剧集季列表 |

### 统一认证路由（admin + user 共享，需 JWT）

| 方法 | 路径 | 用途 |
|------|------|------|
| GET | `/api/v1/subscriptions` | 我的订阅 |
| POST | `/api/v1/subscriptions/check-existing` | 创建前检测库内是否已存在资源 |
| POST | `/api/v1/subscriptions` | 创建订阅（支持可选 `season`，`0` 表示整剧） |
| POST | `/api/v1/subscriptions/:id/resubmit` | 基于自己的 `REJECTED` 订阅重新发起，必须提交本次 `note` |
| DELETE | `/api/v1/subscriptions/:id` | 删除订阅 |
| GET | `/api/v1/profile` | 个人信息 |
| GET | `/api/v1/profile/analytics` | 当前登录用户画像（支持 `range` 或 `startDate/endDate`） |
| PUT | `/api/v1/profile` | 更新资料 |
| PUT | `/api/v1/password` | 修改密码 |
| PUT | `/api/v1/email` | 修改邮箱 |
| POST | `/api/v1/redeem` | 通用兑换续期 |
| GET | `/api/v1/redeem/:code/validate` | 续期兑换码预验证（忽略 `registrationPlanGroup`） |
| GET | `/api/v1/redemptions` | 当前登录账号的兑换历史 |
| POST | `/api/v1/telegram/bindcode` | 生成 Telegram 绑定验证码 |
| DELETE | `/api/v1/telegram/unbind` | 解除 Telegram 绑定 |
| GET | `/api/v1/emby/config` | Emby 配置 |
| GET | `/api/v1/media/stats` | 媒体统计 |
| GET | `/api/v1/media/latest` | 最新入库 |
| GET | `/api/v1/rankings/latest` | 最新整期排行（`period`） |
| GET | `/api/v1/rankings/history` | 按日期查询整期历史排行（`period` + `date`） |
| GET | `/api/v1/plans` | 当前登录用户可购方案列表（认证兼容别名，按用户有效套餐分组过滤） |
| GET | `/api/v1/payments/plans` | 当前登录用户可购方案列表（按用户有效套餐分组过滤） |
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
| GET | `/api/v1/user/redeem/:code/validate` | 续期兑换码预验证（忽略 `registrationPlanGroup`） |
| GET | `/api/v1/user/redemptions` | 我的兑换历史 |
| GET | `/api/v1/user/emby/config` | Emby 服务器地址 |
| GET | `/api/v1/user/media/stats` | 媒体库统计 |
| GET | `/api/v1/user/subscriptions` | 我的订阅 |
| POST | `/api/v1/user/subscriptions` | 创建订阅 |
| POST | `/api/v1/user/subscriptions/:id/resubmit` | 基于自己的 `REJECTED` 订阅重新发起，必须提交本次 `note` |
| DELETE | `/api/v1/user/subscriptions/:id` | 删除订阅 |

### 管理员路由（需认证 + role=admin）

| 方法 | 路径 | 用途 |
|------|------|------|
| GET | `/api/v1/admin/current` | 当前管理员信息 |
| GET | `/api/v1/admin/users` | 用户列表（支持按有效 `planGroup` 过滤；显式分组为空时自动归入默认分组） |
| POST | `/api/v1/admin/users` | 后台创建普通用户（显式指定 `planGroup` 与 `expiresAt` / `neverExpire`） |
| GET | `/api/v1/admin/users/:id` | 用户详情 |
| GET | `/api/v1/admin/users/:id/profile` | 用户画像（支持 `range` 或 `startDate/endDate`） |
| PUT | `/api/v1/admin/users/:id` | 更新用户 |
| PUT | `/api/v1/admin/users/:id/extend` | 延长有效期 |
| PUT | `/api/v1/admin/users/:id/toggle` | 切换激活状态 |
| PUT | `/api/v1/admin/users/:id/reset-password` | 重置密码 |
| DELETE | `/api/v1/admin/users/:id` | 删除用户 |
| GET | `/api/v1/admin/redemption-codes` | 兑换码列表（支持 `code` / `status` / `templateUserId` / `registrationPlanGroup` / `showAll` 过滤） |
| POST | `/api/v1/admin/redemption-codes` | 创建兑换码（支持可选 `registrationPlanGroup`） |
| POST | `/api/v1/admin/redemption-codes/batch` | 批量创建兑换码（支持可选 `registrationPlanGroup`） |
| PUT | `/api/v1/admin/redemption-codes/:id` | 更新兑换码（支持可选 `registrationPlanGroup`） |
| DELETE | `/api/v1/admin/redemption-codes/:id` | 删除兑换码 |
| GET | `/api/v1/admin/user-templates` | 模板用户列表 |
| GET | `/api/v1/admin/configs` | 获取设置中心全部配置（定义 + 当前值 + 来源） |
| PATCH | `/api/v1/admin/configs/:key` | 更新单项配置 |
| POST | `/api/v1/admin/configs/:group/test` | 测试指定配置组 |
| GET | `/api/v1/admin/redemptions` | 全部兑换历史（支持 `username` / `userId` / `code` 过滤） |
| GET | `/api/v1/admin/subscriptions` | 全部订阅 |
| PUT | `/api/v1/admin/subscriptions/:id/approve` | 审批通过 |
| PUT | `/api/v1/admin/subscriptions/:id/reject` | 审批拒绝（请求体必须携带 `reason`） |
| PUT | `/api/v1/admin/subscriptions/:id/ingest` | 校验 Emby 已入库后收口（仅 `APPROVED` 可用） |
| DELETE | `/api/v1/admin/subscriptions/:id` | 删除订阅 |
| GET | `/api/v1/admin/sessions` | 活跃会话 |
| GET | `/api/v1/admin/playback-history` | 播放历史查询 |
| GET | `/api/v1/admin/playback-profiles` | 用户画像总览（支持 `range` 或 `startDate/endDate`，以及 `keyword/sortBy/sortOrder/page/pageSize`） |
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
| GET | `/api/v1/admin/plan-groups` | 套餐分组列表 |
| POST | `/api/v1/admin/plan-groups` | 创建套餐分组 |
| PUT | `/api/v1/admin/plan-groups/:key` | 更新套餐分组 / 切换默认分组 |
| DELETE | `/api/v1/admin/plan-groups/:key` | 删除套餐分组 |
| GET | `/api/v1/admin/plans` | 方案列表（支持 `planGroup` 过滤） |
| POST | `/api/v1/admin/plans` | 创建方案 |
| PUT | `/api/v1/admin/plans/:id` | 更新方案 |
| DELETE | `/api/v1/admin/plans/:id` | 下架方案（软删除） |
| GET | `/api/v1/admin/payments` | 全部支付记录 |
| GET | `/api/v1/admin/system/info` | 系统统计 |
| POST | `/api/v1/admin/system/test-emby` | 测试 Emby 连接 |
| GET | `/api/v1/admin/media-gaps/scan-status` | 查询缺集扫描后台任务状态 |
| POST | `/api/v1/admin/media-gaps/scan` | 异步触发缺集扫描 |
| POST | `/api/v1/admin/tv-calendar/sync` | 手动同步追剧日历 |
| POST | `/api/v1/admin/tv-calendar/refresh` | 手动刷新追剧日历 |
| POST | `/api/v1/admin/cron/check-expired` | 手动执行过期检查 |
| POST | `/api/v1/admin/cron/generate-ranking` | 手动生成排行 |
| POST | `/api/v1/admin/rankings/preview` | 排行预览 |

追剧日历同步接口说明：

- `POST /api/v1/admin/tv-calendar/sync`：请求体可选，默认同步 `[0,1]`（当前周 + 下周）
- `tmdbId` 可选，传入时只同步单剧
- `weekOffsets` 可选，仅支持 `-1/0/1`
- `force=true` 时跳过轻量活跃剧筛选，并强制刷新 TMDB 缓存
- `POST /api/v1/admin/tv-calendar/refresh` 仍保留，内部复用同步逻辑，作为兼容入口
- Emby 入库 webhook 在保留 TV Calendar 点亮逻辑的同时，额外回写 `subscriptions`：电影按 `tmdbId` 命中 `APPROVED` 电影订阅；剧集优先按 webhook 自带 `tmdbId + season` 命中指定季订阅，若 webhook 未携带剧集主 TMDB ID，则回退用 `seriesId` 向 Emby 查询主剧 `ProviderIds`，优先走 `Items?Ids=`，未命中时再尝试 `/Items/{id}`，同时允许 `season=0` 的整剧订阅在任意季首个真实剧集入库时转为 `INGESTED`

### 内部服务路由（InternalAuth 中间件，Bot 调用）

| 方法 | 路径 | 用途 |
|------|------|------|
| PUT | `/api/v1/internal/subscriptions/:id/approve` | 审批通过 |
| PUT | `/api/v1/internal/subscriptions/:id/reject` | 审批拒绝（请求体必须携带 `reason`） |
| GET | `/api/v1/internal/settings/:key` | 读取内部配置（仅允许访问统一配置层中已注册的非敏感 key；未知 key 返回 404） |
| GET | `/api/v1/internal/media/stats` | 读取内部媒体统计（Bot 复用） |
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
  - State: `token`, `role`, `protectionConfig`
  - Computed: `isAuthenticated`, `isAdmin`, `isUser`
  - Actions: `login`, `register`, `logout`, `setAuth`, `clearAuth`, `restoreAuth`, `loadProtectionConfig`
- `store/user.ts`：用户状态管理
- `store/admin.ts`：管理员状态管理

### API 层

- `api/request.ts` — 基础配置：baseURL=/api/v1, 401 拦截
- `api/auth.ts` — login, getLoginProtectionConfig, register, getRegistrationMode, sendEmailCode, sendResetCode, resetPasswordByCode
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
- **活跃态**：绿色 banner + 媒体统计 + 续费中心入口
- **过期态**：橙色警告 banner + 明显的“立即续费”入口 + 媒体统计灰化
- **定位**：Dashboard 只负责账户概览，不再承载兑换码输入和续费历史

### 续费中心

- 路由：`/console/renewal`（user）
- 兼容路由：`/console/pricing` → 重定向到 `/console/renewal`
- 视图：`views/console/RenewalCenterView.vue`
- 页面结构：
  - 当前会员状态
  - 续费方式 Tab
    - 在线购买（Stripe Checkout）
    - 兑换码续期
  - 支付记录 + 兑换记录
- 目标：把“在线支付”和“兑换码续期”统一到同一续费心智下，而不是分散在 Dashboard 和独立价格页中

### 管理端兑换中心

- 路由：`/console/redemptions`（admin）
- 兼容路由：
  - `/console/redemption-codes` → `?tab=codes`
  - `/console/redemption-history` → `?tab=history`
- 视图：`views/admin/RedemptionCenterView.vue`
- Tab 结构：
  - `codes`：`views/admin/RedemptionCodesView.vue`
  - `history`：`views/admin/RedemptionHistoryView.vue`
- 数据源：
  - `GET /api/v1/admin/redemption-codes`（支持兑换码、状态、模板用户、注册套餐分组筛选；返回 `notes`、`registrationPlanGroup`、`registrationPlanGroupName`）
  - `POST /api/v1/admin/redemption-codes`（支持可选备注 `notes` 与 `registrationPlanGroup`）
  - `POST /api/v1/admin/redemption-codes/batch`（支持可选备注 `notes` 与 `registrationPlanGroup`）
  - `PUT /api/v1/admin/redemption-codes/:id`（支持更新备注 `notes` 与 `registrationPlanGroup`）
  - `DELETE /api/v1/admin/redemption-codes/:id`
  - `GET /api/v1/admin/redemptions`（支持按用户名、用户 ID、兑换码筛选）

### 管理端支付中心

- 路由：`/console/billing`（admin）
- 兼容路由：
  - `/console/billing?tab=groups`
  - `/console/plans` → `?tab=plans`
  - `/console/payments` → `?tab=payments`
- 视图：`views/admin/PaymentCenterView.vue`
- Tab 结构：
  - `groups`：`views/admin/PlanGroupsView.vue`
  - `plans`：`views/admin/PlansView.vue`
  - `payments`：`views/admin/PaymentsView.vue`
- 数据源：
  - `GET /api/v1/admin/plan-groups`
  - `POST /api/v1/admin/plan-groups`
  - `PUT /api/v1/admin/plan-groups/:key`
  - `DELETE /api/v1/admin/plan-groups/:key`
  - `GET /api/v1/admin/plans`
  - `POST /api/v1/admin/plans`
  - `PUT /api/v1/admin/plans/:id`
  - `DELETE /api/v1/admin/plans/:id`
  - `GET /api/v1/admin/payments`

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
- 数据源：`GET /api/v1/admin/playback-history`（支持 username / keyword / 日期范围 / 分页筛选，兼容旧 `userId`）
- 联动：支持跳转到 `/console/users/:id/profile`

### 管理端用户画像

- 新增路由：`/console/user-profiles`（admin）
- 新增视图：`views/admin/UserPlaybackProfilesView.vue`
- 数据源：`GET /api/v1/admin/playback-profiles`
- 支持：
  - 时间窗口：`today / 7d / 30d / 90d / all`
  - 自定义日期时间范围（最大 92 天）
  - 用户名搜索
  - 排序字段切换
  - 查看单用户画像 / 播放历史

### 管理端单用户画像

- 新增路由：`/console/user-profiles/:id`（admin）
- 兼容路由：`/console/users/:id/profile` → `/console/user-profiles/:id`
- 新增视图：`views/admin/UserPlaybackProfileView.vue`
- 主入口：`views/admin/UserPlaybackProfilesView.vue`
- 辅助入口：`views/admin/PlaybackHistoryView.vue`
- 兼容入口：`views/admin/UsersView.vue`
- 页面主体：复用 `components/profile/PlaybackProfileContent.vue`，仅在外层补管理员操作
- 数据源：`GET /api/v1/admin/users/:id/profile?range=today|7d|30d|90d|all` 或 `startDate/endDate`
- 页面模块：
  - 摘要卡：累计播放时长 / 播放次数 / 活跃天数 / 最近播放
  - 时间范围：推荐快捷范围 + 自定义日期时间范围（最大 92 天）
  - 分布：24 小时活跃时段、设备分布、客户端分布
  - 勋章：基于固定阈值的解释型画像标签
  - 最近记录：最近 10 条播放记录预览，并支持跳回播放历史

### 用户端我的画像

- 新增路由：`/console/profile-analytics`（user）
- 新增视图：`views/console/ProfileAnalyticsView.vue`
- 页面主体：复用 `components/profile/PlaybackProfileContent.vue`
- 数据源：`GET /api/v1/profile/analytics?range=today|7d|30d|90d|all` 或 `startDate/endDate`
- 页面模块：
  - 摘要卡：累计播放时长 / 播放次数 / 活跃天数 / 最近播放
  - 时间范围：推荐快捷范围 + 自定义日期时间范围（最大 92 天）
  - 活跃时段：24 小时分布 + 峰值时段摘要
  - 画像标签：展示当前时间窗口内最有代表性的少量标签
  - 偏好分布：设备偏好 / 客户端偏好
  - 最近播放记录：最近 10 条个人播放记录预览

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

### 最近入库

- 主入口：`/console/dashboard`（user）
- 展示位置：`views/console/DashboardView.vue` + `components/console/RecentLibrarySection.vue`
- 兼容路径：`/console/library` 路由级重定向到 `/console/dashboard`
- 数据源：`GET /api/v1/media/latest?type=Movie|Series&limit=20`
- 行为：在概览页展示当前用户视角的最近入库摘要，支持电影/剧集切换、横向滑动与手动刷新，不做搜索和分页

---

## 9. Telegram Bot 架构

### 技术栈

- Python 3.11 + python-telegram-bot（支持 `webhook` / `polling` 双模式，默认 `webhook`）
- FastAPI 作为 HTTP 服务器（接收 API 通知；`webhook` 模式下同时接收 Telegram Webhook）
- 与 Go API 通过 `X-Internal-Secret` 双向通信

### 通信模式

```
用户操作 → Go API → BotNotifier（火忘式 POST）→ Bot FastAPI → Telegram Bot → 发送消息
Telegram 用户操作 → Telegram → Bot Webhook → Bot 处理 → 调用 Go Internal API → 返回结果
```

`polling` 模式下第二条链路改为：

```
Telegram 用户操作 → Telegram → Bot Polling → Bot 处理 → 调用 Go Internal API → 返回结果
```

### Bot 端点

| 端点 | 用途 |
|------|------|
| `GET /health` | 健康检查 |
| `POST /telegram/webhook` | Telegram Webhook 入口 |
| `POST /notify/subscription` | 接收新订阅通知 |
| `POST /notify/registration` | 接收新注册通知 |
| `POST /notify/payment` | 接收支付成功通知 |
| `POST /notify/ranking` | 接收排行榜通知 |

### 命令与处理器

- **CallbackQuery**：订阅审批按钮（approve/reject → 调用 Internal API）
- **NewChatMembers**：群组欢迎消息（读取 `notify_group_link` 与 `telegram_welcome_message_template` 配置）
- **Commands**：`/search`（搜索影视并订阅；电影直接确认，电视剧先选季再确认）、`/bind`（绑定账号）、`/info`（查看账号信息）、`/redeem`（兑换续期码）、`/resetpw`（重置密码）、`/refresh_menu`（管理员强制刷新当前群菜单）
- **群菜单策略**：仅私聊作用域写入命令菜单；default/group scope 保持为空，群聊默认不展示命令菜单，首次收到群消息时按群清理旧作用域菜单，并在当前 Bot 进程内缓存已同步群；`/refresh_menu` 强刷会额外重试清理 default / all-group 作用域
- **通知格式化**：`message_formatter.py` 统一格式化 Telegram 消息（HTML 模式）

### 环境变量

| 变量 | 必需 | 默认值 | 说明 |
|------|------|--------|------|
| `TELEGRAM_BOT_TOKEN` | ✅ | — | Bot Token（@BotFather 获取）|
| `TELEGRAM_UPDATE_MODE` | — | `webhook` | Telegram 更新接入模式：`webhook` 或 `polling` |
| `TELEGRAM_ADMIN_CHAT_ID` | — | — | 管理员 Chat ID；可被设置中心数据库值覆盖，env 仅作兜底 |
| `TELEGRAM_GROUP_CHAT_ID` | — | — | 群组 Chat ID（排行榜推送）；可被设置中心数据库值覆盖，env 仅作兜底 |
| `TELEGRAM_WEBHOOK_SECRET` | 条件必需 | — | `webhook` 模式下用于 Webhook 签名校验 |
| `INTERNAL_API_SECRET` | ✅ | — | 与 Go API 共享密钥 |
| `WEBHOOK_URL` | 条件必需 | — | `webhook` 模式下的公开 HTTPS Webhook URL |
| `API_URL` | — | `http://localhost:8080` | Ember API 地址 |
| `BOT_PORT` | — | `8000` | Bot 服务端口 |

说明：Bot 在运行期通过 Internal API 读取 `TELEGRAM_ADMIN_CHAT_ID`、`TELEGRAM_GROUP_CHAT_ID`、`notify_group_link` 和 `telegram_welcome_message_template`，并做短 TTL 缓存；当 API 未返回值时，Chat ID 回退到本地 env。`polling` 模式下可移除 Telegram 使用的公网域名和 HTTPS 回调入口，但 Bot 仍需保留内网 HTTP 地址供 API 访问 `/notify/*`，且只支持单实例部署。

---

## 10. 定时任务

| 任务 | 调度 | 控制变量 | 说明 |
|------|------|----------|------|
| 过期用户检查 | `CRON_SCHEDULE`（默认 `0 2 * * *`）| `CRON_ENABLED` | 封禁过期 Emby 账号 |
| 验证码清理 | `0 3 * * *` | `CRON_ENABLED` | 删除过期 EmailVerification + TelegramBindCode |
| 日榜生成 | `RANKING_DAILY_SCHEDULE`（默认 `0 20 * * *`）| `RANKING_CRON_ENABLED` | 从 Emby 生成日播放排行 |
| 周榜生成 | `RANKING_WEEKLY_SCHEDULE`（默认 `30 20 * * 0`）| `RANKING_CRON_ENABLED` | 从 Emby 生成周播放排行 |
| 追剧日历同步 | `TV_CALENDAR_SYNC_SCHEDULE`（默认 `0 */12 * * *`） | `CRON_ENABLED` | 同步 TMDB/Emby 追剧日历缓存 |

补充说明：
- API 启动后默认会在 `15s` 后额外执行一次追剧日历补偿同步，用于预热周历缓存。
- 该补偿同步由 `TV_CALENDAR_STARTUP_SYNC_ENABLED` 控制，默认 `"true"`；关闭后不影响 `TV_CALENDAR_SYNC_SCHEDULE` 对应的定时同步。
- `CRON_TIMEZONE` 不只影响 cron 调度本身，也会作为追剧日历 `today / upcoming / missing` 的用户可见状态判定基线。

**通用配置**：
这些项由 `ConfigService` 统一解析，优先级为“数据库覆盖值 > 环境变量 > 默认值”；管理员可在设置中心修改，但属于启动期配置，保存后需重启 API 才会生效。

| 配置项 | 默认值 | 说明 |
|----------|--------|------|
| `CRON_ENABLED` | `"true"` | 是否启用（过期检查 + 验证码清理 + 追剧日历同步）|
| `CRON_SCHEDULE` | `"0 2 * * *"` | 过期检查 cron 表达式 |
| `CRON_TIMEZONE` | `"Asia/Shanghai"` | cron、排行榜计算与追剧日历状态判定使用的时区 |
| `RANKING_CRON_ENABLED` | `"false"` | 是否启用排行榜生成 |
| `RANKING_DAILY_SCHEDULE` | `"0 20 * * *"` | 日榜 cron 表达式 |
| `RANKING_WEEKLY_SCHEDULE` | `"30 20 * * 0"` | 周榜 cron 表达式 |
| `TV_CALENDAR_STARTUP_SYNC_ENABLED` | `"true"` | 是否启用 API 启动后的追剧日历补偿同步 |
| `TV_CALENDAR_SYNC_SCHEDULE` | `"0 */12 * * *"` | 追剧日历自动同步表达式 |

**过期检查逻辑**：查询 `expiresAt < NOW() AND embyDisabled = false` → Emby `SetUserPolicy(IsDisabled: true)` → 设置 `EmbyDisabled = true`。不修改 IsActive，不阻止用户登录。

---

## 11. 环境变量完整列表

### 核心配置

| 变量 | 必需 | 默认值 | 说明 |
|------|------|--------|------|
| `DATABASE_URL` | ✅ | — | PostgreSQL DSN |
| `JWT_SECRET` | ✅ | — | ≥32 字符 |
| `CONFIG_ENCRYPTION_KEY` | ✅ | — | 设置中心敏感配置加密主密钥（≥32 字符） |
| `INTERNAL_API_SECRET` | ✅ | — | API ↔ Bot 共享密钥 |
| `PORT` | — | `8080` | 服务端口 |
| `ADMIN_USERNAME` | — | — | 默认管理员用户名（首次启动 seed）|
| `ADMIN_PASSWORD` | — | — | 默认管理员密码（首次启动 seed，落地后请立即在控制台改密）|

### Emby 集成

| 变量 | 必需 | 默认值 | 说明 |
|------|------|--------|------|
| `EMBY_URL` | — | — | Emby 服务器内部 URL |
| `EMBY_API_KEY` | — | — | Emby API 密钥 |
| `EMBY_WEBHOOK_TOKEN` | — | — | Emby Webhook token（`/api/v1/webhooks/emby?token=`）|

### TMDB / MoviePilot

| 变量 | 必需 | 默认值 | 说明 |
|------|------|--------|------|
| `TMDB_API_KEY` | — | — | TMDB API 密钥 |
| `MOVIEPILOT_URL` | — | — | MoviePilot 地址 |
| `MOVIEPILOT_API_KEY` | — | — | MoviePilot API Key（X-API-KEY） |

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
| `TV_CALENDAR_STARTUP_SYNC_ENABLED` | — | `"true"` | 控制 API 启动后是否执行一次追剧日历补偿同步；由设置中心数据库托管，修改后需重启 API |
| `TV_CALENDAR_SYNC_SCHEDULE` | — | `"0 */12 * * *"` | 追剧日历同步表达式；由设置中心数据库托管，修改后需重启 API |

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

---

## 13. 部署

**Docker Compose（`infrastructure/docker/docker-compose.yml`）**：
- PostgreSQL 16 + Go API + Vue 前端（可选）+ Telegram Bot + Nginx（可选）
- 强制 env：`POSTGRES_USER` / `POSTGRES_PASSWORD` / `DATABASE_URL` / `JWT_SECRET` / `CONFIG_ENCRYPTION_KEY` / `INTERNAL_API_SECRET`（`docker compose up` 缺失任一立即拒绝启动）；Bot 启用时还要求 `TELEGRAM_BOT_TOKEN`
- API 容器仅保留启动期/边界环境变量（`DATABASE_URL`、`JWT_SECRET`、`CONFIG_ENCRYPTION_KEY`、`INTERNAL_API_SECRET`、`ADMIN_USERNAME`、`ADMIN_PASSWORD`、`EMBY_WEBHOOK_TOKEN`、`TELEGRAM_BOT_TOKEN`、`TELEGRAM_WEBHOOK_SECRET`、`WEBHOOK_URL`）
- API 以非 root 用户 `ember:ember`(UID 1000) 运行
- 健康检查：`GET /health`；Bot 通过 `depends_on.condition: service_healthy` 等 API 健康后再启动
- PostgreSQL 端口默认仅监听 `127.0.0.1:5432`；远程访问请走 SSH tunnel 或反代授权
- 首次初始化目录：`infrastructure/docker/initdb/`（compose 仅挂载该子目录到 `/docker-entrypoint-initdb.d/`，README/archive 不参与首启）；新增顶层 SQL 必须同步复制到 `initdb/`
- 数据库迁移资产当前收口为 `infrastructure/database/20260415_00_schema_baseline.sql` + baseline 之后的顶层增量 migration；`pre-20260415` 历史 SQL 已归档到 `infrastructure/database/archive/pre-20260415/`
- 启动期不再调用 `AutoMigrate`：本地空库可执行 `cd services/api && go run ./cmd/migrate`，工具会按字典序应用 `infrastructure/database/` 顶层与生产同源的 SQL，再跑 `VerifySchema` 自检；生产 schema 必须通过 `infrastructure/database/` 下的 SQL migration 升级

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
