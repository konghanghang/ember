# Stripe 一次性支付集成设计

## Context

当前 Ember 系统的订阅延期完全依赖管理员手动发放兑换码，没有自助付费能力。用户到期后只能等管理员分发新的兑换码，缺少自助续费渠道。

**目标**：集成 Stripe Checkout Sessions 实现一次性付费购买订阅天数。管理员在后台自由创建/编辑/删除多档付费方案（如月度、季度、年度），用户在控制台选择购买。Stripe 完成支付后通过 Webhook 自动延长用户有效期。

**核心设计选择**：
- **Stripe Checkout Sessions**（重定向模式）— 不需要前端集成 Stripe.js，用户跳转到 Stripe 托管页面付款，零 PCI 合规负担
- **动态 `price_data`** — 价格从我们的数据库读取，传入 Checkout Session，不需要在 Stripe Dashboard 预创建 Price 对象。管理员改价即生效
- **一次性付费** — 与现有兑换码模型一致（买 N 天），不是 Stripe Subscription 周期订阅
- **USD 美元** — 固定币种

---

## 一、数据模型

### 1.1 Plan（付费方案）

**新文件**：`services/api/internal/models/plan.go`

```go
package models

import (
	"time"

	"gorm.io/gorm"
)

// Plan 付费方案
type Plan struct {
	ID          string    `json:"id" gorm:"column:id;type:varchar(25);primaryKey"`
	Name        string    `json:"name" gorm:"column:name;size:100;not null"`
	Description string    `json:"description" gorm:"column:description;size:500"`
	Days        int       `json:"days" gorm:"column:days;not null"`
	Price       int64     `json:"price" gorm:"column:price;not null"`
	Currency    string    `json:"currency" gorm:"column:currency;size:3;not null;default:usd"`
	IsActive    bool      `json:"isActive" gorm:"column:isActive;default:true;not null"`
	SortOrder   int       `json:"sortOrder" gorm:"column:sortOrder;default:0;not null"`
	CreatedAt   time.Time `json:"createdAt" gorm:"column:createdAt;autoCreateTime"`
	UpdatedAt   time.Time `json:"updatedAt" gorm:"column:updatedAt;autoUpdateTime"`
}

func (Plan) TableName() string {
	return "plans"
}

func (p *Plan) BeforeCreate(tx *gorm.DB) error {
	if p.ID == "" {
		p.ID = generateCUID()
	}
	return nil
}
```

**字段说明**：
- `Price` 单位为**美分**（Stripe 标准），如 $9.99 存储为 `999`
- `SortOrder` 控制前端展示顺序（升序）
- `IsActive` 管理员可随时下架方案
- 遵循项目模式：CUID、BeforeCreate、显式 column 标签

### 1.2 Payment（支付记录）

**新文件**：`services/api/internal/models/payment.go`

```go
package models

import (
	"time"

	"gorm.io/gorm"
)

type PaymentStatus string

const (
	PaymentPending   PaymentStatus = "pending"
	PaymentCompleted PaymentStatus = "completed"
	PaymentFailed    PaymentStatus = "failed"
)

// Payment 支付记录
type Payment struct {
	ID                    string        `json:"id" gorm:"column:id;type:varchar(25);primaryKey"`
	UserID                string        `json:"userId" gorm:"column:userId;size:25;index;not null"`
	PlanID                string        `json:"planId" gorm:"column:planId;size:25;index;not null"`
	StripeSessionID       string        `json:"stripeSessionId" gorm:"column:stripeSessionId;size:255;uniqueIndex;not null"`
	StripePaymentIntentID string        `json:"stripePaymentIntentId,omitempty" gorm:"column:stripePaymentIntentId;size:255"`
	Amount                int64         `json:"amount" gorm:"column:amount;not null"`
	Currency              string        `json:"currency" gorm:"column:currency;size:3;not null;default:usd"`
	Days                  int           `json:"days" gorm:"column:days;not null"`
	Status                PaymentStatus `json:"status" gorm:"column:status;size:20;not null;default:pending"`
	CreatedAt             time.Time     `json:"createdAt" gorm:"column:createdAt;autoCreateTime"`
	UpdatedAt             time.Time     `json:"updatedAt" gorm:"column:updatedAt;autoUpdateTime"`
}

func (Payment) TableName() string {
	return "payments"
}

func (p *Payment) BeforeCreate(tx *gorm.DB) error {
	if p.ID == "" {
		p.ID = generateCUID()
	}
	return nil
}
```

**关键设计**：
- `StripeSessionID` 加 uniqueIndex，保证幂等（同一 session 不会重复处理）
- `Status` 三态：`pending`（创建 session 时）→ `completed`（webhook 确认后）/ `failed`
- 记录 `Amount`、`Days` 快照，即使后续方案价格变动，历史记录不受影响

**数据关系**：
```
User (1) ──→ (N) Payment
Plan (1) ──→ (N) Payment
```

---

## 二、Stripe 配置

通过环境变量配置（与 `EMBY_API_KEY`、`JWT_SECRET` 同级）：

| 变量 | 必需 | 说明 |
|------|------|------|
| `STRIPE_SECRET_KEY` | 是 | Stripe 密钥（`sk_test_...` 或 `sk_live_...`） |
| `STRIPE_WEBHOOK_SECRET` | 是 | Webhook 签名密钥（`whsec_...`） |
| `STRIPE_SUCCESS_URL` | 是 | 支付成功后跳转 URL（如 `https://your-domain.com/console/pricing?success=true`） |
| `STRIPE_CANCEL_URL` | 是 | 支付取消后跳转 URL（如 `https://your-domain.com/console/pricing?canceled=true`） |

未配置时支付功能不可用，但不影响系统其他功能。

**Go 依赖**：`github.com/stripe/stripe-go/v81`

---

## 三、API 端点设计

| 方法 | 路径 | 认证 | 说明 |
|------|------|------|------|
| GET | `/api/v1/plans` | 公开 | 获取所有启用方案（用户页面展示） |
| POST | `/api/v1/webhooks/stripe` | 无（Stripe 签名验证） | Stripe Webhook 回调 |
| POST | `/api/v1/payments/checkout` | JWT | 创建 Checkout Session，返回跳转 URL |
| GET | `/api/v1/payments` | JWT | 用户自己的支付记录 |
| GET | `/api/v1/admin/plans` | Admin | 所有方案（含已下架） |
| POST | `/api/v1/admin/plans` | Admin | 创建方案 |
| PUT | `/api/v1/admin/plans/:id` | Admin | 更新方案 |
| DELETE | `/api/v1/admin/plans/:id` | Admin | 删除方案 |
| GET | `/api/v1/admin/payments` | Admin | 所有支付记录（审计） |

---

## 四、PaymentService

### 新文件：`services/api/internal/services/payment.go`

核心方法：

```go
type PaymentService struct{}

func NewPaymentService() *PaymentService {
	stripe.Key = os.Getenv("STRIPE_SECRET_KEY")
	return &PaymentService{}
}
```

#### 4.1 Plan CRUD

```go
type CreatePlanRequest struct {
	Name        string `json:"name" binding:"required"`
	Description string `json:"description"`
	Days        int    `json:"days" binding:"required,min=1"`
	Price       int64  `json:"price" binding:"required,min=1"` // 美分
	SortOrder   int    `json:"sortOrder"`
}

type UpdatePlanRequest struct {
	Name        *string `json:"name"`
	Description *string `json:"description"`
	Days        *int    `json:"days" binding:"omitempty,min=1"`
	Price       *int64  `json:"price" binding:"omitempty,min=1"`
	IsActive    *bool   `json:"isActive"`
	SortOrder   *int    `json:"sortOrder"`
}
```

- `CreatePlan(req)` — 创建方案
- `UpdatePlan(id, req)` — 更新方案（指针字段实现 partial update）
- `DeletePlan(id)` — 删除方案
- `GetPlans(req)` — 分页列表（admin，支持 showAll 含已下架）
- `GetActivePlans()` — 所有启用方案（无分页，用于用户页面）

#### 4.2 Checkout Session 创建

```go
type CreateCheckoutRequest struct {
	PlanID string `json:"planId" binding:"required"`
}

func (s *PaymentService) CreateCheckoutSession(userID string, req *CreateCheckoutRequest) (*CreateCheckoutResponse, error) {
	// 1. 从 DB 查询方案（必须 isActive）
	// 2. 创建 Stripe Checkout Session：
	//    - mode: "payment"（一次性）
	//    - line_items 使用 price_data（动态价格，从 DB 读取）
	//    - metadata 存入 user_id、plan_id、days（用于 webhook 关联）
	//    - success_url / cancel_url 从环境变量读取
	// 3. 创建 Payment 记录（status = pending）
	// 4. 返回 session.URL（前端跳转到 Stripe 页面）

	params := &stripe.CheckoutSessionParams{
		Mode: stripe.String(string(stripe.CheckoutSessionModePayment)),
		LineItems: []*stripe.CheckoutSessionLineItemParams{
			{
				PriceData: &stripe.CheckoutSessionLineItemPriceDataParams{
					Currency: stripe.String(plan.Currency),
					ProductData: &stripe.CheckoutSessionLineItemPriceDataProductDataParams{
						Name:        stripe.String(plan.Name),
						Description: stripe.String(plan.Description),
					},
					UnitAmount: stripe.Int64(plan.Price),
				},
				Quantity: stripe.Int64(1),
			},
		},
		SuccessURL: stripe.String(os.Getenv("STRIPE_SUCCESS_URL")),
		CancelURL:  stripe.String(os.Getenv("STRIPE_CANCEL_URL")),
		Metadata: map[string]string{
			"user_id": userID,
			"plan_id": plan.ID,
			"days":    fmt.Sprintf("%d", plan.Days),
		},
	}

	sess, err := session.New(params)
	// ... 创建 pending Payment 记录 ...
	return &CreateCheckoutResponse{URL: sess.URL}, nil
}
```

#### 4.3 Webhook 处理

```go
func (s *PaymentService) HandleWebhook(r *http.Request) error {
	// 1. 读取原始请求体（Stripe 签名验证需要）
	// 2. 验证 Stripe 签名
	// 3. 仅处理 "checkout.session.completed" 事件
	// 4. 调用 fulfillPayment 完成履约
}

func (s *PaymentService) fulfillPayment(sessionID, paymentIntentID string, metadata map[string]string) error {
	// 1. 通过 stripeSessionId 查找 Payment 记录
	// 2. 幂等检查：如果 status 已经是 completed，直接返回（防重复处理）
	// 3. 开启事务：
	//    a. 更新 Payment.Status = completed
	//    b. 延长用户 ExpiresAt（复用 RedeemCode 的逻辑）：
	//       - 如果 ExpiresAt 为 nil 或已过期 → newExpiry = NOW + days
	//       - 否则 → newExpiry = ExpiresAt + days
	//    c. 自动解封（复用 RedeemCode 的逻辑）：
	//       - 如果 EmbyDisabled && IsActive → 解除 Emby 禁用
	//    d. 保存用户
	// 4. 提交事务
}
```

**关键设计**：
- **幂等**：通过 `StripeSessionID` uniqueIndex + status 检查，同一 webhook 多次触发不会重复延期
- **数据信任**：fulfillPayment 从我们自己的 Payment 记录读取 userID 和 days，不信任 Stripe metadata（metadata 仅作为创建 Payment 时的关联桥梁）
- **事务**：Payment 状态更新 + 用户延期 + Emby 解封在同一个事务中
- **复用逻辑**：expiry 延长和 auto-unban 的算法与 `services/redemption.go:60-131` 中的 `RedeemCode` 完全一致

#### 4.4 支付记录查询

```go
// GetAllPayments — 管理员查看所有支付记录（带 JOIN 显示用户名和方案名）
// GetUserPayments — 用户查看自己的支付记录
```

分页模式与现有 `GetRedemptionCodes` 一致：`{ data, total, page, pageSize, totalPages }`

---

## 五、PaymentHandler

### 新文件：`services/api/internal/handlers/payment.go`

```go
type PaymentHandler struct {
	service *services.PaymentService
}

func NewPaymentHandler() *PaymentHandler {
	return &PaymentHandler{service: services.NewPaymentService()}
}
```

方法清单：
- `GetActivePlans(c)` — 公开，返回 `{ data: []Plan }`
- `CreateCheckout(c)` — 认证，从 JWT 取 userID，返回 `{ url: string }`
- `GetMyPayments(c)` — 认证，分页
- `HandleStripeWebhook(c)` — 公开，读 `c.Request` 原始 body 做签名验证
- `GetPlans(c)` — Admin，分页 + showAll
- `CreatePlan(c)` — Admin
- `UpdatePlan(c)` — Admin
- `DeletePlan(c)` — Admin
- `GetAllPayments(c)` — Admin，分页

**Webhook body 读取**：Stripe webhook 需要原始 body 做签名验证。由于该路由没有 JWT 中间件，也没有 `ShouldBindJSON` 调用，`c.Request.Body` 是未消费的，直接 `io.ReadAll` 即可。

**Rate limiting 提示**：Webhook 端点不需要限流，Stripe 发送频率可控，有签名保护。

---

## 六、错误定义

### 修改文件：`services/api/internal/services/errors.go`

追加：
```go
ErrPlanNotFound  = errors.New("方案不存在")
ErrPaymentFailed = errors.New("支付处理失败")
```

---

## 七、数据库迁移

### 修改文件：`services/api/internal/db/db.go`

AutoMigrate 新增：
```go
&models.Plan{},
&models.Payment{},
```

---

## 八、路由注册

### 修改文件：`services/api/cmd/server/main.go`

#### 8.1 创建 Handler（约第 49 行后）

```go
paymentHandler := handlers.NewPaymentHandler()
```

#### 8.2 公开路由区（约第 65 行后）

```go
// 付费方案（公开）
api.GET("/plans", paymentHandler.GetActivePlans)
// Stripe Webhook（公开，Stripe 签名验证）
api.POST("/webhooks/stripe", paymentHandler.HandleStripeWebhook)
```

#### 8.3 统一认证路由（`authenticated` 组内）

```go
// 支付
authenticated.POST("/payments/checkout", paymentHandler.CreateCheckout)
authenticated.GET("/payments", paymentHandler.GetMyPayments)
```

#### 8.4 管理员路由（`admin` 组内）

```go
// 方案管理
admin.GET("/plans", paymentHandler.GetPlans)
admin.POST("/plans", paymentHandler.CreatePlan)
admin.PUT("/plans/:id", paymentHandler.UpdatePlan)
admin.DELETE("/plans/:id", paymentHandler.DeletePlan)
// 支付记录
admin.GET("/payments", paymentHandler.GetAllPayments)
```

---

## 九、前端改造

### 9.1 类型定义

**修改文件**：`services/web/src/types/api.ts`

```typescript
// ==================== 付费方案 ====================
export interface Plan {
  id: string
  name: string
  description: string
  days: number
  price: number         // 美分
  currency: string
  isActive: boolean
  sortOrder: number
  createdAt: string
  updatedAt: string
}

export interface CreatePlanRequest {
  name: string
  description?: string
  days: number
  price: number         // 美分
  sortOrder?: number
}

export interface UpdatePlanRequest {
  name?: string
  description?: string
  days?: number
  price?: number
  isActive?: boolean
  sortOrder?: number
}

export type PaymentStatus = 'pending' | 'completed' | 'failed'

export interface Payment {
  id: string
  userId: string
  planId: string
  stripeSessionId: string
  stripePaymentIntentId?: string
  amount: number
  currency: string
  days: number
  status: PaymentStatus
  createdAt: string
  username?: string    // JOIN 字段
  planName?: string    // JOIN 字段
}

export interface CheckoutResponse {
  url: string
}
```

### 9.2 API 函数

**修改文件**：`services/web/src/api/console.ts`

```typescript
// 获取启用方案
export function getActivePlans(): Promise<{ data: Plan[] }> {
  return request({ url: '/plans', method: 'get' })
}

// 创建 Checkout Session
export function createCheckout(planId: string): Promise<CheckoutResponse> {
  return request({ url: '/payments/checkout', method: 'post', data: { planId } })
}

// 用户支付记录
export function getMyPayments(params?: { page?: number; pageSize?: number }): Promise<PaymentListResponse> {
  return request({ url: '/payments', method: 'get', params })
}
```

**修改文件**：`services/web/src/api/admin.ts`

```typescript
// 方案 CRUD
export function getPlans(params?: { page?: number; pageSize?: number; showAll?: boolean }): Promise<PlanListResponse> {
  return request({ url: '/admin/plans', method: 'get', params })
}
export function createPlan(data: CreatePlanRequest): Promise<Plan> {
  return request({ url: '/admin/plans', method: 'post', data })
}
export function updatePlan(id: string, data: UpdatePlanRequest): Promise<Plan> {
  return request({ url: `/admin/plans/${id}`, method: 'put', data })
}
export function deletePlan(id: string) {
  return request({ url: `/admin/plans/${id}`, method: 'delete' })
}

// 支付记录
export function getAllPayments(params?: { page?: number; pageSize?: number; userId?: string }): Promise<PaymentListResponse> {
  return request({ url: '/admin/payments', method: 'get', params })
}
```

### 9.3 Admin 方案管理页

**新文件**：`services/web/src/views/admin/PlansView.vue`

参照 `RedemptionCodesView.vue` 的表格/对话框/分页模式：
- 表格列：名称、天数、价格（`$X.XX` 从美分格式化）、状态（active/inactive tag）、排序、操作
- 创建对话框：名称、描述、天数、价格（输入美元，提交时 ×100 转美分）、排序
- 编辑对话框：同上 + isActive 开关
- 顶部：统计 badge、showAll 切换、刷新、创建按钮

### 9.4 用户购买页

**新文件**：`services/web/src/views/console/PricingView.vue`

- 调用 `getActivePlans()` 获取方案列表
- 响应式卡片网格（mobile 1列，desktop 2-3列）
- 每个卡片：方案名称、描述、价格 `$X.XX`、天数、"购买" 按钮
- 点击"购买"→ `createCheckout(planId)` → `window.location.href = resp.url`（跳转 Stripe）
- URL query 参数检测：`?success=true` 显示成功提示，`?canceled=true` 显示取消提示
- 下方展示用户支付历史（`getMyPayments()`），状态列用 tag 着色

### 9.5 路由注册

**修改文件**：`services/web/src/router/index.ts`

在 `/console` children 中新增（约第 57 行后，`subscriptions/new` 之后）：

```typescript
{
  path: 'pricing',
  name: 'console-pricing',
  component: () => import('../views/console/PricingView.vue'),
},
```

在 admin 路由区新增（约第 81 行后，`sessions` 之后）：

```typescript
{
  path: 'plans',
  name: 'console-plans',
  meta: { role: 'admin' },
  component: () => import('../views/admin/PlansView.vue'),
},
```

### 9.6 侧边栏导航

**修改文件**：`services/web/src/components/console/Sidebar.vue`

import 新增：
```typescript
import { ShoppingCart, Goods } from '@element-plus/icons-vue'
```

用户菜单区新增（在"媒体库"后，约第 45 行后）：
```typescript
{
  title: '购买订阅',
  path: '/console/pricing',
  icon: ShoppingCart,
  role: 'user'
},
```

管理员 children 新增（在"兑换码管理"后，约第 62 行后）：
```typescript
{
  title: '付费方案',
  path: '/console/plans',
  icon: Goods,
  role: 'admin'
},
```

---

## 支付流程图

```
用户                    Ember 后端              Stripe
 │                         │                      │
 │ 查看方案                │                      │
 │ GET /plans ────────────→│                      │
 │ ←──────── plans[] ──────│                      │
 │                         │                      │
 │ 点击"购买"              │                      │
 │ POST /payments/checkout │                      │
 │ { planId } ────────────→│                      │
 │                         │ Create Session ──────→│
 │                         │ ←── session.URL ──────│
 │                         │ 创建 Payment(pending) │
 │ ←── { url } ───────────│                      │
 │                         │                      │
 │ window.location = url   │                      │
 │ ─────────────────────────────────────────────→│
 │                    Stripe Checkout 页面         │
 │ 输入信用卡信息并支付                             │
 │ ←─── 跳转 success_url ─────────────────────────│
 │                         │                      │
 │                         │ webhook: session.completed
 │                         │ ←────────────────────│
 │                         │ 验证签名              │
 │                         │ 幂等检查              │
 │                         │ Payment → completed   │
 │                         │ User.ExpiresAt += days│
 │                         │ Emby 解封（如需）      │
 │                         │ 返回 200 ────────────→│
```

---

## 文件变更清单（按执行顺序）

### 新文件（6 个）

| # | 文件 | 说明 |
|---|------|------|
| 1 | `services/api/internal/models/plan.go` | Plan 模型 |
| 2 | `services/api/internal/models/payment.go` | Payment 模型 |
| 3 | `services/api/internal/services/payment.go` | PaymentService（方案 CRUD + Checkout + Webhook + 记录查询） |
| 4 | `services/api/internal/handlers/payment.go` | PaymentHandler |
| 5 | `services/web/src/views/admin/PlansView.vue` | 管理员方案管理页 |
| 6 | `services/web/src/views/console/PricingView.vue` | 用户购买页 |

### 修改文件（9 个）

| # | 文件 | 说明 |
|---|------|------|
| 7 | `services/api/go.mod` + `go.sum` | 添加 `stripe-go/v81` 依赖 |
| 8 | `services/api/internal/services/errors.go` | 新增 2 个 error 常量 |
| 9 | `services/api/internal/db/db.go` | AutoMigrate 新增 Plan、Payment |
| 10 | `services/api/cmd/server/main.go` | 创建 handler + 注册所有新路由 |
| 11 | `services/web/src/types/api.ts` | 新增 Plan、Payment、Checkout 相关类型 |
| 12 | `services/web/src/api/console.ts` | 新增 getActivePlans、createCheckout、getMyPayments |
| 13 | `services/web/src/api/admin.ts` | 新增 getPlans、createPlan、updatePlan、deletePlan、getAllPayments |
| 14 | `services/web/src/router/index.ts` | 新增 pricing 和 plans 路由 |
| 15 | `services/web/src/components/console/Sidebar.vue` | 新增"购买订阅"和"付费方案"导航项 |

---

## 执行阶段

**Phase 1 — 后端基础设施**（步骤 1-3）
- 安装 stripe-go 依赖
- 创建 Plan、Payment 模型
- 实现 PaymentService

**Phase 2 — 后端集成**（步骤 4-10）
- 创建 PaymentHandler
- 新增 error 常量
- 数据库迁移
- 路由注册

**Phase 3 — 前端**（步骤 11-15）
- 类型定义 + API 函数
- 管理员方案管理页
- 用户购买页
- 路由 + 侧边栏

---

## 验证方式

1. **编译验证**：
   - `cd services/api && go build ./...`
   - `cd services/web && npm run build`

2. **功能验证**（手动）：
   - 管理员创建/编辑/删除方案
   - 用户页面展示启用的方案
   - 用户点击购买 → 跳转 Stripe Checkout
   - Stripe 测试卡 `4242424242424242` 完成支付
   - 本地 webhook 测试：`stripe listen --forward-to localhost:8080/api/v1/webhooks/stripe`
   - 支付成功后 User.ExpiresAt 延长
   - 过期用户支付后自动解封 Emby
   - 同一 webhook 重复触发不会重复延期（幂等）
   - 管理员可查看所有支付记录
