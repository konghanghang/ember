# 统一控制台合并方案

## 背景与目标

当前 Ember 前端存在 admin（`/admin/*`）和 user（`/user/*`）两套独立面板。经代码审查发现：

- **两个 Layout.vue 结构完全相同**：相同的 `el-container` 骨架、相同的 `handleLogout` 逻辑、相同的 CSS，仅标题文字和菜单项不同
- **订阅页面操作的是同一数据模型**（`Subscription`），但 admin 用表格、user 用卡片，UI 形态不同
- **后端 User 模型已统一**：通过 `role` 字段区分 admin/user，不存在独立的 Admin 表

**目标**：合并为统一控制台 `/console/*`，通过角色控制菜单和操作可见性。订阅页统一为 Netflix 风格海报卡片。

---

## 一、当前架构分析

### 1.1 前端路由结构

```
/login                  → LoginView.vue（公开）
/register               → user/RegisterView.vue（公开）
/admin                  → admin/Layout.vue（requiresAuth + role: admin）
  /admin/users          → admin/UsersView.vue
  /admin/redemption-codes → admin/RedemptionCodesView.vue
  /admin/subscriptions  → admin/SubscriptionsView.vue
  /admin/settings       → admin/SettingsView.vue
/user                   → user/Layout.vue（requiresAuth + role: user）
  /user/dashboard       → user/DashboardView.vue
  /user/subscriptions   → user/SubscriptionsView.vue
  /user/subscriptions/new → user/NewSubscriptionView.vue
```

### 1.2 后端路由结构

```
公开路由（无认证）：
  POST /api/v1/login
  POST /api/v1/user/register
  GET  /api/v1/register/mode
  GET  /api/v1/register/code/:code/validate
  GET  /api/v1/tmdb/search

Admin 路由（JWTAuth + AdminOnly）：
  GET  /api/v1/admin/current
  GET  /api/v1/admin/users
  GET  /api/v1/admin/users/:id
  PUT  /api/v1/admin/users/:id/extend
  PUT  /api/v1/admin/users/:id/toggle
  PUT  /api/v1/admin/users/:id/reset-password
  DELETE /api/v1/admin/users/:id
  GET  /api/v1/admin/redemption-codes
  POST /api/v1/admin/redemption-codes
  DELETE /api/v1/admin/redemption-codes/:id
  GET  /api/v1/admin/settings
  PUT  /api/v1/admin/settings/:key
  GET  /api/v1/admin/redemptions
  GET  /api/v1/admin/subscriptions
  PUT  /api/v1/admin/subscriptions/:id/approve
  PUT  /api/v1/admin/subscriptions/:id/reject
  GET  /api/v1/admin/system/info
  POST /api/v1/admin/system/test-emby
  POST /api/v1/admin/cron/check-expired

User 路由（JWTAuth + UserOnly）：
  GET  /api/v1/user/profile
  PUT  /api/v1/user/profile
  PUT  /api/v1/user/password
  PUT  /api/v1/user/email
  POST /api/v1/user/redeem
  GET  /api/v1/user/redeem/:code/validate
  GET  /api/v1/user/redemptions
  GET  /api/v1/user/subscriptions
  POST /api/v1/user/subscriptions
  DELETE /api/v1/user/subscriptions/:id
  GET  /api/v1/user/emby/config
  GET  /api/v1/user/media/stats
```

### 1.3 Admin 与 User 功能重合度分析

| 功能 | Admin | User | 重合度 |
|------|-------|------|--------|
| 订阅查看 | 查看所有人（表格） | 查看自己（卡片） | 🟡 数据模型相同，UI 和范围不同 |
| 订阅操作 | 批准/拒绝 | 新建/删除 | 🔴 操作完全不同 |
| 个人信息 | 无 | Dashboard（改密/兑换/Emby） | ⚫ 零重合 |
| 用户管理 | CRUD + 延长/禁用 | 无 | ⚫ 零重合 |
| 兑换码管理 | 创建/删除/列表 | 无（兑换在 Dashboard） | ⚫ 零重合 |
| 系统设置 | 注册模式/Emby 测试 | 无 | ⚫ 零重合 |

**结论**：只有订阅页有合并价值，其余页面按角色控制菜单显隐即可。Layout 是纯粹的重复，必须合并。

---

## 二、目标架构

### 2.1 前端路由结构（合并后）

```
/                              → HomeView（公开）
/login                         → LoginView（公开）
/register                      → RegisterView（公开）
/console                       → console/Layout.vue（requiresAuth: true，不限角色）
  /console/dashboard           → console/DashboardView（所有角色）
  /console/subscriptions       → console/SubscriptionsView（所有角色，角色感知）
  /console/subscriptions/new   → console/NewSubscriptionView（所有角色）
  /console/users               → admin/UsersView（meta.role: 'admin'）
  /console/redemption-codes    → admin/RedemptionCodesView（meta.role: 'admin'）
  /console/settings            → admin/SettingsView（meta.role: 'admin'）
/admin/:pathMatch(.*)          → redirect 到 /console/...（兼容旧链接）
/user/:pathMatch(.*)           → redirect 到 /console/...（兼容旧链接）
/:pathMatch(.*)                → NotFoundView
```

### 2.2 后端路由结构（新增统一组）

```
新增 — 统一认证��由（JWTAuth，不限角色）：
  GET    /api/v1/subscriptions      → GetSubscriptions（角色感知：admin 查全部，user 查自己）
  POST   /api/v1/subscriptions      → CreateSubscription（复用现有 handler）
  DELETE /api/v1/subscriptions/:id  → DeleteSubscription（复用现有 handler）
  GET    /api/v1/profile            → GetProfile（复用 userHandler）
  PUT    /api/v1/profile            → UpdateProfile
  PUT    /api/v1/password           → UpdatePassword
  PUT    /api/v1/email              → UpdateEmail
  GET    /api/v1/emby/config        → GetEmbyConfig（复用 mediaHandler）
  GET    /api/v1/media/stats        → GetMediaStats

保留不变 — Admin 路由：
  PUT  /api/v1/admin/subscriptions/:id/approve  （审批权限保持 AdminOnly）
  PUT  /api/v1/admin/subscriptions/:id/reject   （审批权限保持 AdminOnly）
  ...其余 admin 端点全部保留...

保留不变 — User 路由（向后兼容，后续迭代清理）：
  ...所有现有 user 端点保留...
```

---

## 三、详细文件变更清单

### 3.1 新建文件（4 个）

| 文件路径 | 用途 |
|---------|------|
| `services/web/src/views/console/Layout.vue` | 统一控制台布局 |
| `services/web/src/views/console/SubscriptionsView.vue` | 统一订阅页（Netflix 卡片） |
| `services/web/src/views/console/DashboardView.vue` | 个人面板（从 user/ 迁移） |
| `services/web/src/views/console/NewSubscriptionView.vue` | 新建订阅（从 user/ 迁移） |
| `services/web/src/api/console.ts` | 统一控制台 API 层 |

### 3.2 修改文件（8 个）

| 文件路径 | 改动内容 |
|---------|----------|
| `services/web/src/router/index.ts` | 重写路由树 + 导航守卫 |
| `services/web/src/views/LoginView.vue` | 登录跳转改为 `/console/dashboard` |
| `services/web/src/views/HomeView.vue` | 已登录跳转改为 `/console/dashboard` |
| `services/web/src/views/user/RegisterView.vue` | 注册成功跳转改为 `/console/dashboard` |
| `services/web/src/components/home/HeroSection.vue` | 修复死链接 `/user/login` → `/login` |
| `services/api/cmd/server/main.go` | 新增 authenticated 路由组 |
| `services/api/internal/handlers/subscription.go` | 新增 `GetSubscriptions` 统一 handler |
| `services/api/internal/services/subscription.go` | 新增 `GetUserSubscriptionsPaginated` 方法 |

### 3.3 删除文件（6 个）

| 文件路径 | 原因 |
|---------|------|
| `services/web/src/views/admin/Layout.vue` | 被 console/Layout.vue 替代 |
| `services/web/src/views/admin/SubscriptionsView.vue` | 被 console/SubscriptionsView.vue 替代 |
| `services/web/src/views/user/Layout.vue` | 被 console/Layout.vue 替代 |
| `services/web/src/views/user/SubscriptionsView.vue` | 被 console/SubscriptionsView.vue 替代 |
| `services/web/src/views/user/DashboardView.vue` | 已迁移到 console/ |
| `services/web/src/views/user/NewSubscriptionView.vue` | 已迁移到 console/ |

### 3.4 保留不动的文件

| 文件路径 | 说明 |
|---------|------|
| `services/web/src/views/admin/UsersView.vue` | 仅路由挂载点变化，代码不改 |
| `services/web/src/views/admin/RedemptionCodesView.vue` | 同上 |
| `services/web/src/views/admin/SettingsView.vue` | 同上 |
| `services/web/src/views/user/RegisterView.vue` | 公开路由，不属于控制台 |
| `services/web/src/api/admin.ts` | 保留 approve/reject + 用户/兑换码/设置 API |
| `services/web/src/api/user.ts` | 保留 redeem 相关 API（后续清理 subscription/profile） |
| `services/web/src/store/auth.ts` | 不变，isAdmin/isUser 仍可用 |
| `services/web/src/store/user.ts` | 保留 |
| `services/web/src/store/admin.ts` | 保留 |

---

## 四、核心组件设计细节

### 4.1 统一 Layout.vue

**设计要点**：
- 标题：`Ember 控制台`
- Admin 角色标签：`<el-tag type="danger" size="small">管理员</el-tag>`
- 侧边栏菜单：

```
所有角色可见：
  - 我的账号      → /console/dashboard     （图标：Odometer）
  - 订阅管理      → /console/subscriptions  （图标：VideoPlay）

管理员可见（v-if="authStore.isAdmin"，el-menu-item-group 分组 title="管理"）：
  - 用户管理      → /console/users          （图标：User）
  - 兑换码管理    → /console/redemption-codes（图标：Ticket）
  - 系统设置      → /console/settings       （图标：Setting）
```

- 脚本逻辑：与现有 Layout 完全相同（`useAuthStore` + `handleLogout`）
- 样式：与现有 Layout 完全相同（复制任意一个即可）

### 4.2 统一 SubscriptionsView.vue — Netflix 风格卡片

**头部区域**：
```
[订阅管理 (h2)]                [全部|待审核|已批准|已拒绝]  [+ 提交新订阅]
```

**卡片网格**：响应式布局
```css
grid-template-columns: repeat(auto-fill, minmax(200px, 1fr))
/* 或 Tailwind: grid-cols-2 md:grid-cols-3 lg:grid-cols-4 xl:grid-cols-6 */
```

**单张卡片结构**：
```
┌──────────────────┐
│                  │
│   TMDB 海报封面   │  ← 300px 高度，object-fit: cover
│                  │
│  ┌─ 悬浮操作层 ─┐ │  ← 鼠标悬停时显示（opacity 0→1 过渡）
│  │  [批准] [拒绝]│ │  ← v-if="isAdmin && status==='PENDING'"
│  │    [删除]     │ │  ← v-if="!isAdmin && status==='PENDING'"
│  └──────────────┘ │
├──────────────────┤
│ 流浪地球 2        │  ← 标题，单行省略
│ [电影] [待审核]   │  ← 类型标签 + 状态标签（颜色编码）
│ 申请人: zhang     │  ← v-if="isAdmin && sub.user?.username"
└──────────────────┘
```

**状态标签颜色**：
- `PENDING` → `warning`（橙色）
- `APPROVED` → `success`（绿色）
- `REJECTED` → `info`（灰色）

**分页**：
- 默认 `pageSize = 12`（6 列 x 2 行）
- 可选 `12 / 24 / 48`
- `el-pagination` layout: `total, sizes, prev, pager, next`

**API 调用逻辑**：
```typescript
import { getSubscriptions, deleteSubscription } from '@/api/console'
import { approveSubscription, rejectSubscription } from '@/api/admin'
```
- `getSubscriptions(params)` — 后端根据 JWT role 自动返回对应数据
- `approveSubscription(id)` / `rejectSubscription(id)` — 仍走 admin 端点
- `deleteSubscription(id)` — 走统一端点

**状态筛选交互**：切换状态时重置 `page = 1` 并重新加载。

### 4.3 DashboardView 迁移调整

从 `user/DashboardView.vue` 复制到 `console/DashboardView.vue`，改动：

1. **API 导入改为 `@/api/console`**：`getProfile`, `updateEmail`, `updatePassword`, `getEmbyConfig`, `getMediaStats`
2. **兑换码区域隐藏 admin**：`v-if="!authStore.isAdmin"` — admin 无需兑换
3. **兑换 API 调用保留从 `@/api/user` 导入**：`redeemCode`, `validateRedeemCode`, `getRedemptions`（这些仍在 user 路由组下，admin 不会触发）

### 4.4 NewSubscriptionView 迁移调整

从 `user/NewSubscriptionView.vue` 复制到 `console/NewSubscriptionView.vue`，改动：

1. **API 导入改为 `@/api/console`**：`createSubscription`
2. **提交成功跳转路径**：`router.push('/user/subscriptions')` → `router.push('/console/subscriptions')`

---

## 五、后端设计细节

### 5.1 新增 `GetSubscriptions` 统一 Handler

**文件**：`services/api/internal/handlers/subscription.go`

```go
// GetSubscriptions 统一订阅列表（角色感知）
// admin → 返回所有用户的订阅（含用户信息），支持分页和状态筛选
// user  → 返回当前用户自己的订阅，支持分页和状态筛选
func (h *SubscriptionHandler) GetSubscriptions(c *gin.Context) {
    userID, _ := c.Get("userID")
    role, _ := c.Get("role")

    // 解析查询参数
    statusStr := c.Query("status")
    page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
    pageSize, _ := strconv.Atoi(c.DefaultQuery("pageSize", "12"))

    if page < 1 { page = 1 }
    if pageSize < 1 || pageSize > 100 { pageSize = 12 }

    // 解析状态筛选
    var status *models.SubscriptionStatus
    if statusStr != "" {
        s := models.SubscriptionStatus(statusStr)
        status = &s
    }

    if role.(string) == "admin" {
        // 管理员：查看所有订阅（含用户信息）
        result, err := h.service.GetAllSubscriptions(status, page, pageSize)
        if err != nil {
            c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
            return
        }
        c.JSON(http.StatusOK, result)
    } else {
        // 普通用户：查看自己的订阅
        result, err := h.service.GetUserSubscriptionsPaginated(userID.(string), status, page, pageSize)
        if err != nil {
            c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
            return
        }
        c.JSON(http.StatusOK, result)
    }
}
```

**设计决策**：
- Handler 层负责角色分流（策略），Service 层负责具体查询（机制）
- 两个角色的响应格式统一为 `{ data: [...], total: N }`
- admin 查询复用现有 `GetAllSubscriptions`（已包含用户信息的 JOIN 查询）
- user 查询使用新增 `GetUserSubscriptionsPaginated`（无需 JOIN）

### 5.2 新增 `GetUserSubscriptionsPaginated` Service 方法

**文件**：`services/api/internal/services/subscription.go`

```go
// GetUserSubscriptionsPaginated 用户订阅分页查询
// 与 GetAllSubscriptions 返回相同的 Response 格式，但仅查询当前用户
func (s *SubscriptionService) GetUserSubscriptionsPaginated(
    userID string, status *models.SubscriptionStatus, page, pageSize int,
) (*GetAllSubscriptionsResponse, error) {
    offset := (page - 1) * pageSize

    query := db.DB.Model(&models.Subscription{}).Where("\"userId\" = ?", userID)
    if status != nil {
        query = query.Where("status = ?", *status)
    }

    var total int64
    query.Count(&total)

    var subscriptions []models.Subscription
    query.Order("\"createdAt\" DESC").Offset(offset).Limit(pageSize).Find(&subscriptions)

    // 构建响应（用户查询不需要 user 关联信息）
    result := make([]SubscriptionWithUser, len(subscriptions))
    for i, sub := range subscriptions {
        result[i] = SubscriptionWithUser{Subscription: sub}
    }

    return &GetAllSubscriptionsResponse{
        Data:  result,
        Total: total,
    }, nil
}
```

**为什么不复用 `GetUserSubscriptions`**：现有方法返回 `[]models.Subscription`（无分页、无状态筛选），而统一接口需要 `{ data, total }` 分页格式 + 状态筛选。新增方法保持旧方法不变。

### 5.3 路由注册

**文件**：`services/api/cmd/server/main.go`

在现有 admin 和 user 路由组之间，新增：

```go
// ==================== 统一认证路由（admin + user 共享） ====================
authenticated := api.Group("")
authenticated.Use(middleware.JWTAuth())
{
    // 订阅管理（统一）
    authenticated.GET("/subscriptions", subscriptionHandler.GetSubscriptions)
    authenticated.POST("/subscriptions", subscriptionHandler.CreateSubscription)
    authenticated.DELETE("/subscriptions/:id", subscriptionHandler.DeleteSubscription)

    // 个人信息（复用现有 handler，admin 也可访问）
    authenticated.GET("/profile", userHandler.GetProfile)
    authenticated.PUT("/profile", userHandler.UpdateProfile)
    authenticated.PUT("/password", userHandler.UpdatePassword)
    authenticated.PUT("/email", userHandler.UpdateEmail)

    // 媒体信息
    authenticated.GET("/emby/config", mediaHandler.GetEmbyConfig)
    authenticated.GET("/media/stats", mediaHandler.GetMediaStats)
}
```

**关键点**：
- 所有新端点仅需 `JWTAuth()` 中间件，不限制角色
- Handler 复用现有的 `userHandler`、`mediaHandler`，无需修改 handler 代码
- `CreateSubscription` 和 `DeleteSubscription` 内部通过 `c.Get("userID")` 获取用户 ID，不依赖角色，admin 调用也安全
- **旧端点保留不删**，零风险向后兼容

---

## 六、前端 API 层设计

### 6.1 新建 `api/console.ts`

```typescript
import request from './request'
import type {
  Subscription,
  SubscriptionListQuery,
  CreateSubscriptionRequest,
  EmbyConfigResponse,
  MediaStatsResponse
} from '@/types/api'

// ==================== 订阅管理 ====================

// 统一订阅列表（后端根据 JWT role 自动过滤）
export function getSubscriptions(params: SubscriptionListQuery) {
  return request<{ data: Subscription[]; total: number }>({
    url: '/subscriptions',
    method: 'get',
    params
  })
}

// 创建订阅
export function createSubscription(data: CreateSubscriptionRequest) {
  return request<{ success: boolean }>({
    url: '/subscriptions',
    method: 'post',
    data
  })
}

// 删除订阅
export function deleteSubscription(id: string) {
  return request<{ success: boolean }>({
    url: `/subscriptions/${id}`,
    method: 'delete'
  })
}

// ==================== 个人信息 ====================

export function getProfile() {
  return request({ url: '/profile', method: 'get' })
}

export function updateProfile(data: { email?: string }) {
  return request({ url: '/profile', method: 'put', data })
}

export function updatePassword(data: { oldPassword: string; newPassword: string }) {
  return request({ url: '/password', method: 'put', data })
}

export function updateEmail(data: { email: string }) {
  return request({ url: '/email', method: 'put', data })
}

// ==================== 媒体信息 ====================

export function getEmbyConfig(): Promise<EmbyConfigResponse> {
  return request({ url: '/emby/config', method: 'get' })
}

export function getMediaStats(): Promise<MediaStatsResponse> {
  return request({ url: '/media/stats', method: 'get' })
}
```

### 6.2 保留的 API 调用

- `api/admin.ts`：保留 `approveSubscription` 和 `rejectSubscription`（SubscriptionsView 需要），以及所有用户管理/兑换码/设置相关 API
- `api/user.ts`：保留 `redeemCode`、`validateRedeemCode`、`getRedemptions`（DashboardView 兑换功能需要），后续迭代清理 subscription/profile 相关函数

---

## 七、路由与导航守卫设计

### 7.1 路由配置

```typescript
const router = createRouter({
  history: createWebHistory(import.meta.env.BASE_URL),
  routes: [
    { path: '/', name: 'home', component: () => import('../views/HomeView.vue') },
    { path: '/login', name: 'login', component: () => import('../views/LoginView.vue') },
    { path: '/register', name: 'register', component: () => import('../views/user/RegisterView.vue') },

    // 统一控制台
    {
      path: '/console',
      component: () => import('../views/console/Layout.vue'),
      meta: { requiresAuth: true },
      children: [
        { path: 'dashboard', name: 'console-dashboard',
          component: () => import('../views/console/DashboardView.vue') },
        { path: 'subscriptions', name: 'console-subscriptions',
          component: () => import('../views/console/SubscriptionsView.vue') },
        { path: 'subscriptions/new', name: 'console-subscriptions-new',
          component: () => import('../views/console/NewSubscriptionView.vue') },
        { path: 'users', name: 'console-users', meta: { role: 'admin' },
          component: () => import('../views/admin/UsersView.vue') },
        { path: 'redemption-codes', name: 'console-redemption-codes', meta: { role: 'admin' },
          component: () => import('../views/admin/RedemptionCodesView.vue') },
        { path: 'settings', name: 'console-settings', meta: { role: 'admin' },
          component: () => import('../views/admin/SettingsView.vue') },
      ],
    },

    // 旧路由兼容重定向
    { path: '/admin/users', redirect: '/console/users' },
    { path: '/admin/redemption-codes', redirect: '/console/redemption-codes' },
    { path: '/admin/subscriptions', redirect: '/console/subscriptions' },
    { path: '/admin/settings', redirect: '/console/settings' },
    { path: '/user/dashboard', redirect: '/console/dashboard' },
    { path: '/user/subscriptions/new', redirect: '/console/subscriptions/new' },
    { path: '/user/subscriptions', redirect: '/console/subscriptions' },

    { path: '/:pathMatch(.*)*', name: 'not-found',
      component: () => import('../views/NotFoundView.vue') },
  ],
})
```

### 7.2 导航守卫

```typescript
router.beforeEach((to, _from, next) => {
  const authStore = useAuthStore()
  authStore.restoreAuth()

  // 已登录用户访问 /login → 直接跳转控制台（不再区分角色）
  if (to.name === 'login' && authStore.isAuthenticated) {
    next({ name: 'console-dashboard' })
    return
  }

  // 需要认证的路由
  if (to.meta.requiresAuth) {
    if (!authStore.isAuthenticated) {
      next({ name: 'login', query: { redirect: to.fullPath } })
      return
    }
    // 子路由级别角色检查（仅 admin 专属页面）
    if (to.meta.role && to.meta.role !== authStore.role) {
      next({ name: 'console-dashboard' })
      return
    }
  }

  next()
})
```

**关键变化**：
- 根级 `/console` 只要求认证，不限角色
- admin 专属页面通过子路由 `meta.role: 'admin'` 控制
- 角色不匹配时跳 dashboard（用户已登录，踢到首页没意义）

### 7.3 需要更新的路由跳转引用

| 文件 | 行号 | 旧值 | 新值 |
|------|------|------|------|
| `LoginView.vue` | 26-30 | `if admin → /admin/users else → /user/dashboard` | `→ route.query.redirect \|\| '/console/dashboard'` |
| `HomeView.vue` | 14-18 | `if admin → /admin/users else → /user/dashboard` | `→ /console/dashboard` |
| `RegisterView.vue` | 60 | `→ /user/dashboard` | `→ /console/dashboard` |
| `HeroSection.vue` | 30, 38 | `/user/login` | `/login` |

---

## 八、实施步骤

### Step 1：后端改动（不破坏现有前端）

1. `services/subscription.go` — 新增 `GetUserSubscriptionsPaginated` 方法
2. `handlers/subscription.go` — 新增 `GetSubscriptions` 统一 handler
3. `cmd/server/main.go` — 新增 `authenticated` 路由组，注册统一端点
4. 编译验证：`cd services/api && go build ./...`

### Step 2：前端新建文件（不改路由，不影响现有功能）

1. 新建 `views/console/Layout.vue`
2. 新建 `views/console/SubscriptionsView.vue`（Netflix 卡片）
3. 新建 `views/console/DashboardView.vue`（从 user/ 复制，改 API 导入 + 隐藏 admin 兑换区域）
4. 新建 `views/console/NewSubscriptionView.vue`（从 user/ 复制，改跳转路径）
5. 新建 `api/console.ts`

### Step 3：路由切换（一次性切换）

1. 重写 `router/index.ts`
2. 修改 `LoginView.vue` — 登录跳转
3. 修改 `HomeView.vue` — 已登录跳转
4. 修改 `RegisterView.vue` — 注册跳转
5. 修复 `HeroSection.vue` — 死链接

### Step 4：清理删除

1. 删除 6 个旧文件（两个 Layout + 两个 Subscriptions + DashboardView + NewSubscriptionView）
2. 清理 `api/user.ts` 中不再调用的 subscription/profile 函数
3. 清理 `api/admin.ts` 中不再调用的 `getAllSubscriptions`

### Step 5：编译验证

```bash
cd services/api && go build ./...
cd services/web && npx vite build
```

---

## 九、关键决策记录

| 决策 | 选择 | 理由 |
|------|------|------|
| admin 能否看到 Dashboard | ✅ 可以 | admin 也是 User 模型，可改密码、看基本信息 |
| Dashboard 兑换码区域 | admin 隐藏 | admin 无需兑换，`v-if="!authStore.isAdmin"` |
| admin 能否创建订阅 | ✅ 可以 | handler 用 JWT userID，角色无关 |
| 后端旧端点是否删除 | 保留 | 零成本零风险，后续迭代清理 |
| Store 是否合并 | 不合并 | 各视图自管状态，无跨组件共享需求 |
| admin 专属视图是否移动目录 | 不移动 | 保持在 views/admin/，仅改路由挂载点 |
| 订阅页 UI 形态 | Netflix 卡片 | 媒体内容用海报封面更直观，两种角色统一体验 |
| 分页大小默认值 | 12 | 适配 6 列 x 2 行网格 |

---

## 十、风险与应对

| 风险 | 应对 |
|------|------|
| admin 访问 Dashboard 时 profile API 403 | 新增统一 `/profile` 端点（JWTAuth only） |
| admin 的 CreateSubscription userID 问题 | handler 从 JWT 取 userID，不依赖角色，已验证安全 |
| 旧书签 `/admin/users` 无法访问 | redirect 规则处理 |
| 小屏幕卡片布局响应式 | Tailwind grid-cols-2 兜底 |
| api/console.ts 与 api/user.ts 函数重复 | 允许短期共存，Step 4 清理 |
