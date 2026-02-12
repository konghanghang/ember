# Ember 系统架构文档

> 本文档记录 Ember 系统的完整架构、数据模型、服务逻辑和 API 端点。
> 供 AI 协作时快速加载系统上下文，避免重复探索代码。

---

## 1. 系统概览

Ember 是一个 Emby 媒体服务器的用户管理系统，提供：
- 用户注册/登录（可配置开放注册或兑换码注册）
- Emby 账号自动创建与生命周期管理（试用 → 续期 → 过期封禁）
- 兑换码系统（注册门控 + 续期工具，统一模型）
- 求片订阅（TMDB 搜索 → 管理员审批 → MoviePilot 自动下载）
- 定时任务（每日检查过期用户，自动封禁 Emby 账号）

## 2. 技术栈

| 层 | 技术 |
|----|------|
| 后端 | Go 1.23 + Gin + GORM |
| 数据库 | PostgreSQL 15 |
| 前端 | Vue 3 + TypeScript + Element Plus + Tailwind CSS |
| 外部集成 | Emby API, TMDB API, MoviePilot API |
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
│     │  └─ moviepilot.go        # MoviePilot HTTP 客户端
│     ├─ handlers/               # HTTP 处理层（Gin）
│     │  ├─ auth.go, user.go, redemption_code.go, setting.go
│     │  ├─ system.go, media.go, subscription.go, tmdb.go
│     │  └─ （每个 handler 对应一个 service）
│     ├─ middleware/
│     │  └─ jwt.go               # JWTAuth + AdminOnly + UserOnly
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
│  │  │  ├─ user.ts              # profile, password, media, redeem
│  │  │  └─ admin.ts             # users, redemption-codes, settings, subscriptions
│  │  ├─ types/api.ts            # 所有 TypeScript 接口定义
│  │  ├─ store/auth.ts           # Pinia: token + role (localStorage 持久化)
│  │  ├─ router/index.ts         # 路由 + 导航守卫（角色鉴权）
│  │  └─ views/
│  │     ├─ HomeView.vue         # 首页
│  │     ├─ user/                # 用户面板
│  │     │  ├─ RegisterView.vue  # 注册（动态模式：open/invite）
│  │     │  ├─ DashboardView.vue # 面板（双态：活跃/过期降级）
│  │     │  ├─ SubscriptionsView.vue
│  │     │  └─ NewSubscriptionView.vue
│  │     └─ admin/               # 管理后台
│  │        ├─ Layout.vue        # 侧边栏布局
│  │        ├─ UsersView.vue
│  │        ├─ RedemptionCodesView.vue
│  │        ├─ SubscriptionsView.vue
│  │        └─ SettingsView.vue
│  ├─ vite.config.ts             # dev:3000, proxy /api→:8080
│  └─ tailwind.config.js         # 自定义色：ember(橙红), cinema
└─ bot/                          # Telegram Bot（待开发）
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
| MaxUses | int | maxUses | 最大使用次数 |
| UsedCount | int | usedCount | 已使用次数 |
| ExpiresAt | *time.Time | expiresAt | 码本身的过期时间 |
| DefaultDays | int | defaultDays | 每次兑换授予的天数 |
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

### 4.5 Subscription（订阅求片）

**表名**: `subscriptions` | **文件**: `models/subscription.go`

| 字段 | 类型 | 列名 | 说明 |
|------|------|------|------|
| ID | string(25) | id | CUID |
| UserID | string(25) | userId | 用户 ID |
| Type | MediaType | type | `"MOVIE"` 或 `"TV"` |
| Name | string | name | 媒体名称 |
| TmdbID | string | tmdbId | TMDB ID |
| PosterPath | *string | posterPath | 海报 URL |
| Status | SubscriptionStatus | status | `PENDING`/`APPROVED`/`REJECTED` |
| Note | *string | note | 用户备注 |
| MpError | *string | mpError | MoviePilot 同步错误 |

### 4.6 数据关系

```
User (1) ──→ (N) Redemption     （兑换历史）
User (1) ──→ (N) Subscription   （求片记录）
User (1) ──→ (1) Emby User      （外部 Emby 账号，通过 EmbyID 关联）

RedemptionCode ──→ Redemption    （码被使用时生成记录）
Setting                          （全局 KV 配置，无外键）
```

---

## 5. 后端服务层

### 5.1 AuthService (`services/auth.go`)

**登录流程**：
1. 查找用户 → Admin: bcrypt 校验 → 普通用户: 本地密码优先 → 无本地密码时降级 Emby 认证 + 自动补存 hash
2. 过期用户**可以登录**（前端显示过期提示 + 兑换入口）

**注册流程**：
1. 读取 `registration_mode` → `"invite"`: 验证兑换码 → `"open"`: 读取 `default_trial_days`
2. 创建 Emby 用户 → 创建本地用户（含 bcrypt hash）→ 签发 JWT

**关键 struct**：
- `RegisterUserRequest{Username, Password, Email, Code}` — Code 可选
- `LoginResponse{Token, User}`

### 5.2 UserService (`services/user.go`)

- `GetUsers(page, pageSize, search, isActive)` — 分页搜索
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
1. ValidateCode → 计算新 ExpiresAt → 开启事务
2. 更新 User.ExpiresAt → 如果 EmbyDisabled: Emby 解封 → 原子递增 usedCount（`WHERE usedCount < maxUses` 防竞态）→ 创建 Redemption 记录 → 提交

### 5.5 SettingService (`services/setting.go`)

- `GetSetting(key)` / `SetSetting(key, value)` — 带值校验
- `GetRegistrationMode()` — 默认 `"open"`
- `GetDefaultTrialDays()` — 默认 `7`

### 5.6 SystemService (`services/system.go`)

- `GetSystemInfo()` — 统计：用户数、活跃数、兑换码数
- `CheckExpiredUsers()` — **cron 核心**：查询 `expiresAt < NOW() AND embyDisabled = false` → 调用 Emby `SetUserPolicy(IsDisabled: true)` → 设置 `EmbyDisabled = true`

### 5.7 EmbyService (`services/emby.go`, 369 行)

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

- `CreateSubscription(userID, type, name, tmdbId)` — 创建 PENDING 状态
- `ApproveSubscription(id)` — 调用 MoviePilot → 设为 APPROVED（MP 失败不阻塞审批，错误存入 mpError）
- `RejectSubscription(id)` — 设为 REJECTED

### 5.10 MoviePilotClient (`services/moviepilot.go`)

- `IsConfigured()` — 检查三个环境变量是否都设置
- `login()` — `POST /api/v1/login/access-token`（form-urlencoded）
- `CreateSubscription(type, name, tmdbId)` — `POST /api/v1/subscribe/`（type 转中文：movie→电影, tv→电视剧）

---

## 6. API 端点完整列表

### 公开路由（无需认证）

| 方法 | 路径 | 用途 |
|------|------|------|
| POST | `/api/v1/login` | 登录 |
| POST | `/api/v1/user/register` | 注册（code 可选）|
| GET | `/api/v1/register/mode` | 获取注册模式 |
| GET | `/api/v1/register/code/:code/validate` | 验证兑换码（注册前）|
| GET | `/api/v1/tmdb/search?query=&type=` | TMDB 搜索 |

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
| PUT | `/api/v1/admin/users/:id/extend` | 延长有效期 |
| PUT | `/api/v1/admin/users/:id/toggle` | 切换激活状态 |
| PUT | `/api/v1/admin/users/:id/reset-password` | 重置密码 |
| DELETE | `/api/v1/admin/users/:id` | 删除用户 |
| GET | `/api/v1/admin/redemption-codes` | 兑换码列表 |
| POST | `/api/v1/admin/redemption-codes` | 创建兑换码 |
| DELETE | `/api/v1/admin/redemption-codes/:id` | 删除兑换码 |
| GET | `/api/v1/admin/settings` | 获取所有配置 |
| PUT | `/api/v1/admin/settings/:key` | 更新配置 |
| GET | `/api/v1/admin/redemptions` | 全部兑换历史 |
| GET | `/api/v1/admin/subscriptions` | 全部订阅 |
| PUT | `/api/v1/admin/subscriptions/:id/approve` | 审批通过 |
| PUT | `/api/v1/admin/subscriptions/:id/reject` | 审批拒绝 |
| GET | `/api/v1/admin/system/info` | 系统统计 |
| POST | `/api/v1/admin/system/test-emby` | 测试 Emby 连接 |
| POST | `/api/v1/admin/cron/check-expired` | 手动执行过期检查 |

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
- **Context 变量**：`userID`, `username`, `role`, `claims`
- **密码存储**：bcrypt（DefaultCost），所有用户统一存本地 hash
- **存量迁移**：`Password == ""` 时降级 Emby 认证，成功后自动补存本地 hash

---

## 8. 前端架构

### 状态管理（Pinia）

`store/auth.ts`：
- State: `token`, `role`（localStorage 持久化）
- Computed: `isAuthenticated`, `isAdmin`, `isUser`
- Actions: `login`, `register`, `logout`, `setAuth`, `clearAuth`, `restoreAuth`

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

---

## 9. 定时任务

| 环境变量 | 默认值 | 说明 |
|----------|--------|------|
| `CRON_ENABLED` | `"true"` | 是否启用 |
| `CRON_SCHEDULE` | `"0 2 * * *"` | cron 表达式（默认每天凌晨2点）|
| `CRON_TIMEZONE` | `"Asia/Shanghai"` | 时区 |

**执行逻辑**：查询 `expiresAt < NOW() AND embyDisabled = false` → Emby `SetUserPolicy(IsDisabled: true)` → 设置 `EmbyDisabled = true`。不修改 IsActive，不阻止用户登录。

---

## 10. 环境变量完整列表

| 变量 | 必需 | 默认值 | 说明 |
|------|------|--------|------|
| `DATABASE_URL` | ✅ | — | PostgreSQL DSN |
| `JWT_SECRET` | ✅ | — | ≥32 字符 |
| `EMBY_URL` | — | — | Emby 服务器内部 URL |
| `EMBY_API_KEY` | — | — | Emby API 密钥 |
| `NEXT_PUBLIC_EMBY_URL` | — | — | Emby 公开 URL（给前端用）|
| `MOVIEPILOT_URL` | — | — | MoviePilot 地址 |
| `MOVIEPILOT_USERNAME` | — | — | MoviePilot 用户名 |
| `MOVIEPILOT_PASSWORD` | — | — | MoviePilot 密码 |
| `TMDB_API_KEY` | — | — | TMDB API 密钥 |
| `ADMIN_USERNAME` | — | — | 默认管理员用户名（首次启动 seed）|
| `ADMIN_PASSWORD` | — | — | 默认管理员密码 |
| `AUTO_MIGRATE` | — | `"false"` | `"true"` 启用 GORM 自动迁移 |
| `PORT` | — | `8080` | 服务端口 |
| `CRON_ENABLED` | — | `"true"` | 启用定时任务 |
| `CRON_SCHEDULE` | — | `"0 2 * * *"` | cron 表达式 |
| `CRON_TIMEZONE` | — | `"Asia/Shanghai"` | 时区 |

---

## 11. 部署

**Docker Compose**（`infrastructure/docker/docker-compose.yml`）：
- PostgreSQL 16 + Go API + Vue 前端（可选）+ Nginx（可选）
- API 以非 root 用户 `ember:ember`(UID 1000) 运行
- 健康检查：`GET /health`

**数据库连接池**：MaxIdle=10, MaxOpen=100, MaxLifetime=1h, MaxIdleTime=10min

**时间处理**：所有时间戳 UTC 存储（GORM NowFunc 强制 UTC）

---

## 12. 代码模式速查

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
| 前端请求 | Axios 拦截器自动加 Bearer token，401 自动清除登录态 |
