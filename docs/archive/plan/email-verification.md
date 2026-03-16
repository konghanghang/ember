# 邮箱验证码注册功能设计

## Context

当前 Ember 系统的注册流程不验证邮箱真实性——用户可以填写任意邮箱完成注册。这导致：
1. 管理员无法确认邮箱的有效性，无法联系用户
2. 无法保证邮箱唯一性的真实约束力
3. 没有任何注册门槛，容易被滥用

**目标**：在注册流程中加入邮箱验证码环节，用户必须通过邮箱验证才能完成注册；同时限制发送频率以防滥用；提供管理后台开关，可随时关闭验证码功能而无需重启服务。

---

## 开关控制设计

邮箱验证是否生效由**两层条件**共同决定：

| 层级 | 控制方式 | 作用 |
|------|---------|------|
| 基础设施层 | `SMTP_HOST` 环境变量 | SMTP 未配置则物理上无法发邮件，验证自动关闭 |
| 业务层 | `email_verification` 数据库设置 | 管理员随时可在后台开关，无需重启 |

**判定逻辑**（伪代码）：
```
验证码功能生效 = SMTP 已配置 AND email_verification == "true"
```

- SMTP 未配置 → 无论开关如何，都跳过验证（物理限制）
- SMTP 已配置 + 开关关闭 → 跳过验证（业务控制）
- SMTP 已配置 + 开关开启 → 强制验证

`GET /api/v1/register/mode` 响应增加 `emailVerification` 布尔字段，前端据此决定是否展示验证码 UI。

---

## 一、数据模型

### 新文件：`services/api/internal/models/email_verification.go`

```go
package models

import (
	"time"

	"gorm.io/gorm"
)

// EmailVerification 邮箱验证码
type EmailVerification struct {
	ID        string    `json:"id" gorm:"column:id;type:varchar(25);primaryKey"`
	Email     string    `json:"email" gorm:"column:email;size:255;not null;index"`
	Code      string    `json:"-" gorm:"column:code;size:6;not null"`
	IP        string    `json:"-" gorm:"column:ip;size:45;not null;index"`
	ExpiresAt time.Time `json:"expiresAt" gorm:"column:expiresAt;not null"`
	CreatedAt time.Time `json:"createdAt" gorm:"column:createdAt;autoCreateTime"`
}

func (EmailVerification) TableName() string {
	return "email_verifications"
}

func (e *EmailVerification) BeforeCreate(tx *gorm.DB) error {
	if e.ID == "" {
		e.ID = generateCUID()
	}
	return nil
}

// IsExpired 检查验证码是否过期
func (e *EmailVerification) IsExpired() bool {
	return e.ExpiresAt.Before(time.Now().UTC())
}
```

**设计要点**：
- 没有 `is_used`/`is_valid` 状态字段。验证时取 `ORDER BY created_at DESC LIMIT 1`，天然只有最新一条被比对
- **双维度限频**：同一邮箱每天 5 次 + 同一 IP 每天 15 次，均从 `email_verifications` 表 `COUNT(*)` 派生，零额外数据结构
- `IP` 字段 `size:45` 兼容 IPv6 最大长度（如 `::ffff:192.168.1.1`），加 index 加速按 IP 查询
- 遵循项目模式：CUID 主键、`BeforeCreate` 钩子、显式 `gorm:"column:xxx"` 标签、`TableName()` 方法

---

## 二、SMTP 配置

通过环境变量配置（与 `EMBY_URL`、`MOVIEPILOT_URL`、`JWT_SECRET` 同级，属于基础设施级凭据）：

| 变量 | 必需 | 默认值 | 说明 |
|------|------|--------|------|
| `SMTP_HOST` | 是 | — | SMTP 服务器地址（如 `smtp.gmail.com`） |
| `SMTP_PORT` | 否 | `587` | SMTP 端口 |
| `SMTP_USERNAME` | 是 | — | SMTP 登录用户名 |
| `SMTP_PASSWORD` | 是 | — | SMTP 登录密码 |
| `SMTP_FROM` | 否 | 同 USERNAME | 发件人（支持显示名），初始化时会解析出信封发件地址；解析失败视为未配置 |
| `EMAIL_CODE_EXPIRY_MINUTES` | 否 | `10` | 验证码有效期（分钟） |
| `EMAIL_CODE_DAILY_LIMIT` | 否 | `5` | 每邮箱每天发送上限 |
| `EMAIL_CODE_IP_DAILY_LIMIT` | 否 | `15` | 每 IP 每天发送上限 |

---

## 三、数据库设置（业务开关）

### 修改文件：`services/api/internal/db/db.go`

`seedDefaultSettings()` 新增：
```go
{Key: "email_verification", Value: "false"},
```

默认关闭（`"false"`），管理员在后台手动启用。

### 修改文件：`services/api/internal/services/setting.go`

1. `SetSetting` 白名单增加 `"email_verification"`：
```go
// 修改 SetSetting 中的白名单判断
if key != "registration_mode" && key != "default_trial_days" && key != "notify_group_link" && key != "email_verification" {
    return ErrSettingNotFound
}
```

2. 新增验证：
```go
case "email_verification":
    if value != "true" && value != "false" {
        return errors.New("无效的值，必须为 true 或 false")
    }
```

3. 新增辅助方法：
```go
// IsEmailVerificationEnabled 检查邮箱验证是否启用
func (s *SettingService) IsEmailVerificationEnabled() bool {
    return s.GetSetting("email_verification") == "true"
}
```

---

## 四、EmailService

### 新文件：`services/api/internal/services/email.go`

当前实现已从 `smtp.SendMail` 升级为“`gomail` 生成 MIME + `net/smtp` 手动会话发送”：

- 使用 `gomail.NewMessage()` 处理 Header/Body 编码，避免手拼 MIME。
- 初始化阶段解析 `SMTP_FROM`，缓存 `fromAddress`（信封发件地址）。
- `IsConfigured()` 除了 host/username/password，还要求 `fromAddress` 有效。
- 发送链路使用 `DialTimeout + SetDeadline`，避免 SMTP 操作阶段无限挂起。
- 邮件主体写入并 `DATA` 成功后即视为发送成功，`QUIT` 失败仅记录日志，不影响业务返回。

关键结构（精简）：

```go
type EmailService struct {
	host          string
	port          string
	username      string
	password      string
	from          string
	fromAddress   string
	expiryMinutes int
	dailyLimit    int
	ipDailyLimit  int
}

func NewEmailService() *EmailService {
	from := os.Getenv("SMTP_FROM")
	if from == "" {
		from = os.Getenv("SMTP_USERNAME")
	}
	fromAddress := ""
	if from != "" {
		if addr, err := mail.ParseAddress(from); err == nil {
			fromAddress = addr.Address
		}
	}
	// ... 其他配置读取保持不变 ...
}

func (s *EmailService) IsConfigured() bool {
	return s.host != "" && s.username != "" && s.password != "" && s.fromAddress != ""
}
```

验证码业务接口（精简）：

```go
func (s *EmailService) SendVerificationCode(email, ip, codeType string) error {
	// 1) 按 codeType 检查用户存在性（注册必须不存在，重置必须已存在）
	// 2) 按邮箱+类型、按 IP 做 24h 限频
	// 3) 生成验证码并开启事务：写入记录 -> 发送邮件 -> 提交
	// 4) 发送失败或提交失败返回 ErrEmailSendFailed
}

func (s *EmailService) VerifyCode(email, code, codeType string) error {
	// 按 email + type 取最新验证码并校验
}
```

SMTP 发送关键实现（精简）：

```go
func (s *EmailService) sendEmail(to, subject, body string) error {
	m := gomail.NewMessage()
	m.SetHeader("From", s.from)
	m.SetHeader("To", to)
	m.SetHeader("Subject", subject)
	m.SetBody("text/plain", body)

	conn, err := net.DialTimeout("tcp", net.JoinHostPort(s.host, s.port), smtpTimeout)
	if err != nil {
		return err
	}
	defer conn.Close()

	_ = conn.SetDeadline(time.Now().Add(smtpTimeout))

	// STARTTLS/AUTH/MAIL/RCPT/DATA
	// MAIL FROM 使用 s.fromAddress（ASCII 信封地址）
	// DATA 通过 m.WriteTo(w) 输出 MIME 正文
	// QUIT 失败仅日志，不返回业务错误
	return nil
}
```

---

## 五、错误定义

### 修改文件：`services/api/internal/services/errors.go`

新增 6 个常量（追加到现有 `var` 块）：

```go
ErrEmailNotConfigured     = errors.New("邮件服务未配置")
ErrEmailAlreadyRegistered = errors.New("邮箱已被注册")
ErrEmailCodeRateLimit     = errors.New("该邮箱今日发送次数已达上限")
ErrEmailCodeIPRateLimit   = errors.New("请求过于频繁，请稍后再试")
ErrEmailCodeInvalid       = errors.New("邮箱验证码无效或已过期")
ErrEmailSendFailed        = errors.New("验证码发送失败，请稍后重试")
```

---

## 六、Auth Service 改造

### 修改文件：`services/api/internal/services/auth.go`

#### 6.1 AuthService struct 增加 emailService

```go
type AuthService struct {
	notifier     *BotNotifier
	emailService *EmailService  // 新增
}

func NewAuthService() *AuthService {
	return &AuthService{
		notifier:     NewBotNotifier(),
		emailService: NewEmailService(),  // 新增
	}
}
```

#### 6.2 RegisterUserRequest 增加 EmailCode 字段

```go
type RegisterUserRequest struct {
	Username  string `json:"username" binding:"required,min=3,max=50"`
	Password  string `json:"password" binding:"required,min=6"`
	Email     string `json:"email" binding:"required,email"`
	Code      string `json:"code"`      // 兑换码（invite 模式必填，open 模式忽略）
	EmailCode string `json:"emailCode"` // 邮箱验证码（启用时必填）
}
```

#### 6.3 RegisterUser 方法中插入验证逻辑

在现有用户名校验之后、注册模式检查之前（约第 118 行后）插入：

```go
// 邮箱验证码校验（仅在功能启用时生效）
if s.emailService.IsEnabled() {
    if req.EmailCode == "" {
        return nil, errors.New("请先获取邮箱验证码")
    }
    if err := s.emailService.VerifyCode(req.Email, req.EmailCode, models.VerificationTypeRegister); err != nil {
        return nil, err
    }
}
```

**完整的插入位置**（在 `auth.go` 的 `RegisterUser` 方法中）：
```go
func (s *AuthService) RegisterUser(req *RegisterUserRequest) (*RegisterUserResponse, error) {
	// ... 用户名校验（现有，不变）...

	// === 新增：邮箱验证码校验 ===
	if s.emailService.IsEnabled() {
		if req.EmailCode == "" {
			return nil, errors.New("请先获取邮箱验证码")
		}
		if err := s.emailService.VerifyCode(req.Email, req.EmailCode, models.VerificationTypeRegister); err != nil {
			return nil, err
		}
	}

	// ... 注册模式检查（现有，不变）...
	// ... 后续逻辑不变 ...
}
```

---

## 七、Auth Handler 改造

### 修改文件：`services/api/internal/handlers/auth.go`

#### 7.1 AuthHandler struct 增加 emailService

```go
type AuthHandler struct {
	authService  *services.AuthService
	emailService *services.EmailService  // 新增
}

func NewAuthHandler() *AuthHandler {
	return &AuthHandler{
		authService:  services.NewAuthService(),
		emailService: services.NewEmailService(),  // 新增
	}
}
```

#### 7.2 新增 SendEmailCode handler

```go
// SendEmailCodeRequest 发送邮箱验证码请求
type SendEmailCodeRequest struct {
	Email string `json:"email" binding:"required,email"`
}

// SendEmailCode 发送邮箱验证码
func (h *AuthHandler) SendEmailCode(c *gin.Context) {
	var req SendEmailCodeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请提供有效的邮箱地址"})
		return
	}

	// c.ClientIP() 自动处理 X-Forwarded-For / X-Real-IP
	if err := h.emailService.SendVerificationCode(req.Email, c.ClientIP(), models.VerificationTypeRegister); err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, services.ErrEmailCodeRateLimit) || errors.Is(err, services.ErrEmailCodeIPRateLimit) {
			status = http.StatusTooManyRequests
		}
		c.JSON(status, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "验证码已发送"})
}
```

---

## 八、Setting Handler 改造

### 修改文件：`services/api/internal/handlers/setting.go`

`GetRegistrationMode` 方法增加 `emailVerification` 字段：

```go
func (h *SettingHandler) GetRegistrationMode(c *gin.Context) {
	mode := h.service.GetRegistrationMode()
	resp := gin.H{"mode": mode}
	if mode == "open" {
		resp["defaultTrialDays"] = h.service.GetDefaultTrialDays()
	}

	// 新增：邮箱验证状态
	emailService := services.NewEmailService()
	resp["emailVerification"] = emailService.IsEnabled()

	c.JSON(http.StatusOK, resp)
}
```

---

## 九、数据库迁移

### 修改文件：`services/api/internal/db/db.go`

#### 9.1 AutoMigrate 增加新模型

```go
func AutoMigrate() error {
	if err := DB.AutoMigrate(
		&models.RedemptionCode{},
		&models.Redemption{},
		&models.Setting{},
		&models.User{},
		&models.Subscription{},
		&models.PlaybackRanking{},
		&models.EmailVerification{},  // 新增
	); err != nil {
		return err
	}
	// ... 其余不变
}
```

#### 9.2 seedDefaultSettings 增加新设置

```go
defaultSettings := []models.Setting{
	{Key: "default_trial_days", Value: "7"},
	{Key: "registration_mode", Value: "open"},
	{Key: "notify_group_link", Value: ""},
	{Key: "email_verification", Value: "false"},  // 新增，默认关闭
}
```

#### 9.3 `AUTO_MIGRATE=false` 手动迁移 SQL（完整可执行）

**新增文件**：`infrastructure/database/20260222_01_add_email_verification.sql`

```sql
BEGIN;

CREATE TABLE IF NOT EXISTS email_verifications (
  id          varchar(25)  PRIMARY KEY,
  email       varchar(255) NOT NULL,
  code        varchar(6)   NOT NULL,
  ip          varchar(45)  NOT NULL,
  "expiresAt" timestamptz  NOT NULL,
  "createdAt" timestamptz  NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_email_verifications_email
  ON email_verifications (email);

CREATE INDEX IF NOT EXISTS idx_email_verifications_ip
  ON email_verifications (ip);

INSERT INTO settings ("key", "value", "updatedAt")
VALUES ('email_verification', 'false', now())
ON CONFLICT ("key") DO NOTHING;

COMMIT;
```

执行命令：

```bash
psql "$DATABASE_URL" -f infrastructure/database/20260222_01_add_email_verification.sql
```

---

## 十、路由注册 + Cron

### 修改文件：`services/api/cmd/server/main.go`

#### 10.1 新增路由

在公开路由区（约第 62 行附近），紧跟 `api.POST("/user/register", ...)` 之后：

```go
api.POST("/register/send-code", authHandler.SendEmailCode)
api.POST("/forgot-password/send-code", authHandler.SendResetCode)
api.POST("/forgot-password/reset", authHandler.ResetPasswordByCode)
```

#### 10.2 新增 Cron 清理任务

在现有 cron 任务注册区块中（约第 218-230 行的过期检查后面），新增：

```go
// 清理过期验证码（每天凌晨 3 点）
emailService := services.NewEmailService()
if _, err := c.AddFunc("0 3 * * *", func() {
    count, err := emailService.CleanupExpired()
    if err != nil {
        log.Printf("[Cron] 清理过期验证码失败：%v", err)
        return
    }
    if count > 0 {
        log.Printf("[Cron] 已清理 %d 条过期验证码", count)
    }
}); err != nil {
    log.Printf("定时任务注册失败（验证码清理）：%v", err)
} else {
    taskRegistered = true
}
```

---

## 十一、前端改造

### 11.1 类型定义

**修改文件**：`services/web/src/types/api.ts`

```typescript
// RegisterRequest 新增 emailCode
export interface RegisterRequest {
  username: string
  password: string
  email: string
  emailCode?: string    // 新增
  code?: string
}

// RegistrationModeResponse 新增 emailVerification
export interface RegistrationModeResponse {
  mode: 'open' | 'invite'
  defaultTrialDays?: number
  emailVerification?: boolean  // 新增
}
```

### 11.2 API 函数

**修改文件**：`services/web/src/api/auth.ts`

新增 `sendEmailCode` 函数：

```typescript
// 发送邮箱验证码
export function sendEmailCode(email: string): Promise<{ message: string }> {
  return request({
    url: '/register/send-code',
    method: 'post',
    data: { email }
  })
}
```

### 11.3 注册页面

**修改文件**：`services/web/src/views/user/RegisterView.vue`

#### script setup 变更

新增响应式变量和逻辑：

```typescript
import { sendEmailCode } from '@/api/auth'

// 新增
const emailVerification = ref(false)
const emailCodeSent = ref(false)
const emailCodeCountdown = ref(0)
const sendingCode = ref(false)
let countdownTimer: ReturnType<typeof setInterval> | null = null

// 修改 form，新增 emailCode 字段
const form = ref({
  username: '',
  password: '',
  email: '',
  emailCode: '',  // 新增
  code: ''
})

// 修改 fetchRegistrationMode，读取 emailVerification
const fetchRegistrationMode = async () => {
  loadingMode.value = true
  try {
    const res = await getRegistrationMode()
    mode.value = res.mode
    emailVerification.value = res.emailVerification ?? false  // 新增
  } finally {
    loadingMode.value = false
  }
}

// 新增：发送验证码
const handleSendCode = async () => {
  if (!form.value.email) {
    ElMessage.warning('请先输入邮箱')
    return
  }
  sendingCode.value = true
  try {
    await sendEmailCode(form.value.email)
    ElMessage.success('验证码已发送，请查收邮件')
    emailCodeSent.value = true
    // 60 秒倒计时
    emailCodeCountdown.value = 60
    countdownTimer = setInterval(() => {
      emailCodeCountdown.value--
      if (emailCodeCountdown.value <= 0) {
        clearInterval(countdownTimer!)
        countdownTimer = null
      }
    }, 1000)
  } finally {
    sendingCode.value = false
  }
}

// 修改 handleRegister：提交时包含 emailCode
const handleRegister = async () => {
  // ... 现有校验 ...
  if (emailVerification.value && !form.value.emailCode) {
    ElMessage.warning('请输入邮箱验证码')
    return
  }

  loading.value = true
  try {
    await authStore.register({
      username: form.value.username,
      password: form.value.password,
      email: form.value.email,
      emailCode: emailVerification.value ? form.value.emailCode : undefined,  // 新增
      code: form.value.code || undefined
    })
    ElMessage.success('注册成功')
    router.push('/console/dashboard')
  } finally {
    loading.value = false
  }
}
```

#### template 变更

在"邮箱"表单项后增加验证码交互：

```html
<!-- 邮箱输入框改造：增加发送验证码按钮 -->
<el-form-item label="邮箱" required>
  <div class="flex gap-2 w-full" v-if="emailVerification">
    <el-input v-model="form.email" placeholder="请输入邮箱" class="input-ember" :prefix-icon="Message" />
    <el-button
      :loading="sendingCode"
      :disabled="emailCodeCountdown > 0"
      @click="handleSendCode"
    >
      {{ emailCodeCountdown > 0 ? `${emailCodeCountdown}s` : '发送验证码' }}
    </el-button>
  </div>
  <el-input v-else v-model="form.email" placeholder="请输入邮箱" class="input-ember" :prefix-icon="Message" />
</el-form-item>

<!-- 新增：验证码输入框（仅在 emailVerification 启用时显示） -->
<el-form-item v-if="emailVerification" label="邮箱验证码" required>
  <el-input
    v-model="form.emailCode"
    placeholder="请输入 6 位验证码"
    maxlength="6"
    class="input-ember"
  />
</el-form-item>
```

### 11.4 管理后台设置页

**修改文件**：`services/web/src/views/admin/SettingsView.vue`

#### form 新增字段

```typescript
const form = ref({
  registration_mode: 'open',
  default_trial_days: 7,
  notify_group_link: '',
  email_verification: false  // 新增
})
```

#### fetchSettings 读取新设置

```typescript
const emailVerify = list.find(item => item.key === 'email_verification')
if (emailVerify?.value !== undefined) form.value.email_verification = emailVerify.value === 'true'
```

#### handleSaveSettings 保存新设置

```typescript
await updateSetting('email_verification', { value: String(form.value.email_verification) })
```

#### template 新增开关

在"注册与试用配置"区域的 grid 中新增一个表单项：

```html
<el-form-item label="邮箱验证">
  <div class="bg-gray-50 p-1 rounded-xl inline-flex w-full">
    <button
      type="button"
      @click="form.email_verification = true"
      class="flex-1 py-2 px-4 rounded-lg text-sm font-bold transition-all"
      :class="form.email_verification ? 'bg-white text-green-600 shadow-sm' : 'text-gray-500 hover:text-gray-900'"
    >
      开启
    </button>
    <button
      type="button"
      @click="form.email_verification = false"
      class="flex-1 py-2 px-4 rounded-lg text-sm font-bold transition-all"
      :class="!form.email_verification ? 'bg-white text-red-600 shadow-sm' : 'text-gray-500 hover:text-gray-900'"
    >
      关闭
    </button>
  </div>
  <p class="text-xs text-gray-400 mt-2">
    {{ form.email_verification ? '注册时需要邮箱验证码（需配置 SMTP）。' : '注册时不需要邮箱验证。' }}
  </p>
</el-form-item>
```

---

## 文件变更清单（按执行顺序）

| # | 文件 | 操作 | 说明 |
|---|------|------|------|
| 1 | `services/api/internal/models/email_verification.go` | **新建** | EmailVerification 模型 |
| 2 | `services/api/internal/services/errors.go` | 修改 | 新增 6 个 error 常量 |
| 3 | `services/api/internal/services/email.go` | **新建** | EmailService 完整实现 |
| 4 | `services/api/internal/services/setting.go` | 修改 | 白名单 + 验证 + IsEmailVerificationEnabled |
| 5 | `services/api/internal/services/auth.go` | 修改 | AuthService 增加 emailService 字段；RegisterUserRequest 增加 EmailCode；RegisterUser 增加验证逻辑 |
| 6 | `services/api/internal/handlers/auth.go` | 修改 | AuthHandler 增加 emailService + SendEmailCode handler |
| 7 | `services/api/internal/handlers/setting.go` | 修改 | GetRegistrationMode 增加 emailVerification 字段 |
| 8 | `services/api/internal/db/db.go` | 修改 | AutoMigrate + seedDefaultSettings |
| 9 | `services/api/cmd/server/main.go` | 修改 | 路由注册 + cron 清理任务 |
| 10 | `services/web/src/types/api.ts` | 修改 | RegisterRequest + RegistrationModeResponse |
| 11 | `services/web/src/api/auth.ts` | 修改 | 新增 sendEmailCode |
| 12 | `services/web/src/views/user/RegisterView.vue` | 修改 | 两步注册交互 |
| 13 | `services/web/src/views/admin/SettingsView.vue` | 修改 | 邮箱验证开关 |

---

## 执行阶段

**Phase 1 — 后端基础设施**（步骤 1-3）
- 创建 EmailVerification 模型
- 新增 error 常量
- 实现 EmailService（SMTP + 验证码生成/校验/清理）

**Phase 2 — 后端集成**（步骤 4-9）
- 修改 Setting Service（白名单 + 验证 + 开关查询）
- 修改 Auth Service（注册逻辑加入验证码校验）
- 修改 Auth Handler（新增 SendEmailCode）
- 修改 Setting Handler（返回 emailVerification 状态）
- 修改 db.go（迁移 + seed）
- 修改 main.go（路由 + cron）

**Phase 3 — 前端**（步骤 10-13）
- 类型定义
- API 函数
- 注册页面改造
- 管理后台设置页增加开关

---

## 验证方式

1. **编译验证**：
   - `cd services/api && go build ./...`
   - `cd services/web && npm run build`

2. **功能验证**（手动）：
   - SMTP 未配置 + 开关关闭 → 注册行为与当前完全一致
   - SMTP 已配置 + 开关关闭 → 注册行为与当前一致（开关优先）
   - SMTP 已配置 + 开关开启 → 注册时要求验证码
   - 同一邮箱连续发送超过 5 次 → 返回 429 限制错误
   - 同一 IP 连续发送超过 15 次 → 返回 429 限制错误
   - 输入错误验证码 → 注册失败
   - 验证码超过 10 分钟 → 注册失败
   - 管理后台可实时切换开关
