# 找回密码功能设计

## Context

早期注册用户因密码同步 bug 导致 Emby 端密码不正确，无法登录。数据库中密码使用 bcrypt 单向哈希存储，无法解密还原明文。目前只有管理员通过后台接口 `PUT /api/v1/admin/users/:id/reset-password` 重置密码这一条路径，用户自助能力缺失。

**目标**：提供两条自助重置密码的路径，覆盖所有用户场景：

1. **邮箱验证码重置**（Web 前端）— 用户通过已注册邮箱接收验证码来重置密码
2. **Telegram `/resetpw` 命令**（Bot）— 已绑定 Telegram 的用户直接通过 Bot 重置密码

两条路径共享同一个底层逻辑：更新本地 bcrypt 哈希 + 同步 Emby 密码。

---

## Part 1：邮箱验证码重置（Web）

### 一、数据模型改造

#### 修改文件：`services/api/internal/models/email_verification.go`

给 `EmailVerification` 模型新增 `Type` 字段，区分"注册验证码"和"密码重置验证码"：

```go
type EmailVerification struct {
	ID        string    `json:"id" gorm:"column:id;type:varchar(25);primaryKey"`
	Email     string    `json:"email" gorm:"column:email;size:255;not null;index"`
	Code      string    `json:"-" gorm:"column:code;size:6;not null"`
	Type      string    `json:"-" gorm:"column:type;size:20;not null;default:register;index"` // 新增
	IP        string    `json:"-" gorm:"column:ip;size:45;not null;index"`
	ExpiresAt time.Time `json:"expiresAt" gorm:"column:expiresAt;not null"`
	CreatedAt time.Time `json:"createdAt" gorm:"column:createdAt;autoCreateTime"`
}
```

新增常量（同文件底部）：

```go
const (
	VerificationTypeRegister = "register"
	VerificationTypeReset    = "reset"
)
```

**设计要点**：
- `default:register` 确保 GORM AutoMigrate 时 `ALTER TABLE ADD COLUMN` 自动为已有行填充默认值，零破坏
- 加 `index` 加速按类型筛选的频率限制查询
- 注册验证码和重置验证码共享同一张表、同一套清理 cron，无额外维护成本

---

### 二、错误定义

#### 修改文件：`services/api/internal/services/errors.go`

在现有 `var` 块中新增：

```go
ErrEmailNotRegistered = errors.New("该邮箱未注册")
```

---

### 三、EmailService 改造

#### 修改文件：`services/api/internal/services/email.go`

#### 3.1 `SendVerificationCode` 加 `codeType` 参数

签名从 `(email, ip string)` 改为 `(email, ip, codeType string)`。

完整修改后的方法：

```go
// SendVerificationCode 发送验证码
// ip 参数由 handler 层通过 c.ClientIP() 传入
// codeType 取值：models.VerificationTypeRegister 或 models.VerificationTypeReset
func (s *EmailService) SendVerificationCode(email, ip, codeType string) error {
	if !s.IsConfigured() {
		return ErrEmailNotConfigured
	}

	// 业务校验：注册要求邮箱未注册，重置要求邮箱已注册
	var userCount int64
	db.DB.Model(&models.User{}).Where("email = ?", email).Count(&userCount)
	if codeType == models.VerificationTypeRegister && userCount > 0 {
		return ErrEmailAlreadyRegistered
	}
	if codeType == models.VerificationTypeReset && userCount == 0 {
		return ErrEmailNotRegistered
	}

	since := time.Now().UTC().Add(-24 * time.Hour)

	// 频率限制：按 type 隔离计数，注册和重置互不干扰
	var emailCount int64
	db.DB.Model(&models.EmailVerification{}).
		Where("email = ? AND type = ? AND \"createdAt\" > ?", email, codeType, since).
		Count(&emailCount)
	if emailCount >= int64(s.dailyLimit) {
		return ErrEmailCodeRateLimit
	}

	var ipCount int64
	db.DB.Model(&models.EmailVerification{}).
		Where("ip = ? AND \"createdAt\" > ?", ip, since).
		Count(&ipCount)
	if ipCount >= int64(s.ipDailyLimit) {
		return ErrEmailCodeIPRateLimit
	}

	code := generateVerificationCode()

	verification := models.EmailVerification{
		Email:     email,
		Code:      code,
		Type:      codeType, // 新增：设置类型
		IP:        ip,
		ExpiresAt: time.Now().UTC().Add(time.Duration(s.expiryMinutes) * time.Minute),
	}

	tx := db.DB.Begin()
	if tx.Error != nil {
		log.Printf("发送验证码开启事务失败 [%s]: %v", email, tx.Error)
		return ErrEmailSendFailed
	}
	if err := tx.Create(&verification).Error; err != nil {
		tx.Rollback()
		log.Printf("发送验证码保存记录失败 [%s]: %v", email, err)
		return ErrEmailSendFailed
	}

	// 邮件内容根据类型区分
	subject := "Ember 注册验证码"
	action := "注册"
	if codeType == models.VerificationTypeReset {
		subject = "Ember 密码重置验证码"
		action = "密码重置"
	}
	body := fmt.Sprintf("你的 Ember %s验证码是：%s\n有效期 %d 分钟，请勿泄露给他人。", action, code, s.expiryMinutes)

	if err := s.sendEmail(email, subject, body); err != nil {
		tx.Rollback()
		log.Printf("发送验证码邮件失败 [%s]: %v", email, err)
		return ErrEmailSendFailed
	}
	if err := tx.Commit().Error; err != nil {
		log.Printf("发送验证码提交事务失败 [%s]: %v", email, err)
		return ErrEmailSendFailed
	}

	return nil
}
```

**关键改动点**：
- 业务校验根据 `codeType` 分支：`register` 检查邮箱未注册；`reset` 检查邮箱已注册
- 频率限制查询加 `AND type = ?` 条件，按类型隔离计数（IP 限制仍然全局共享，防止同一 IP 滥用）
- 创建记录时设置 `Type: codeType`
- 邮件主题/正文根据类型区分

#### 3.2 `VerifyCode` 加 `codeType` 参数

签名从 `(email, code string)` 改为 `(email, code, codeType string)`。

完整修改后的方法：

```go
// VerifyCode 校验验证码
func (s *EmailService) VerifyCode(email, code, codeType string) error {
	var verification models.EmailVerification
	result := db.DB.Where("email = ? AND type = ?", email, codeType).
		Order("\"createdAt\" DESC").
		First(&verification)
	if result.Error != nil {
		return ErrEmailCodeInvalid
	}

	if verification.IsExpired() {
		return ErrEmailCodeInvalid
	}

	if verification.Code != code {
		return ErrEmailCodeInvalid
	}

	return nil
}
```

**关键改动**：查询条件加 `AND type = ?`，确保注册验证码无法被拿来重置密码，反之亦然。

---

### 四、更新现有调用方

两处现有代码需要补上第三个参数：

#### 4.1 修改文件：`services/api/internal/services/auth.go`

第 127 行，注册流程的验证码校验：

```go
// 修改前
if err := s.emailService.VerifyCode(req.Email, req.EmailCode); err != nil {

// 修改后
if err := s.emailService.VerifyCode(req.Email, req.EmailCode, models.VerificationTypeRegister); err != nil {
```

#### 4.2 修改文件：`services/api/internal/handlers/auth.go`

第 143 行，发送注册验证码：

```go
// 修改前
if err := h.emailService.SendVerificationCode(req.Email, c.ClientIP()); err != nil {

// 修改后
if err := h.emailService.SendVerificationCode(req.Email, c.ClientIP(), models.VerificationTypeRegister); err != nil {
```

需要在 handlers/auth.go 的 import 中新增 `"github.com/konghang/ember/backend/internal/models"`。

---

### 五、UserService 新增密码重置方法

#### 修改文件：`services/api/internal/services/user.go`

新增请求体定义和方法：

```go
// ResetPasswordByCodeRequest 通过邮箱验证码重置密码
type ResetPasswordByCodeRequest struct {
	Email       string `json:"email" binding:"required,email"`
	Code        string `json:"code" binding:"required,len=6"`
	NewPassword string `json:"newPassword" binding:"required,min=6"`
}

// ResetPasswordByCode 通过邮箱验证码重置密码
func (s *UserService) ResetPasswordByCode(req *ResetPasswordByCodeRequest) error {
	// 1. 校验验证码
	emailService := NewEmailService()
	if err := emailService.VerifyCode(req.Email, req.Code, models.VerificationTypeReset); err != nil {
		return err
	}

	// 2. 查找用户
	var user models.User
	result := db.DB.Where("email = ?", req.Email).First(&user)
	if result.Error != nil {
		return errors.New("用户不存在")
	}

	// 3. 更新 Emby 密码（非管理员且有 EmbyID 时同步）
	if user.EmbyID != "" {
		embyService := NewEmbyService()
		if err := embyService.UpdateUserPassword(user.EmbyID, req.NewPassword); err != nil {
			return errors.New("密码重置失败：" + err.Error())
		}
	}

	// 4. 更新本地 bcrypt 哈希
	if err := user.SetPassword(req.NewPassword); err != nil {
		return errors.New("密码重置失败：本地密码更新失败")
	}
	if err := db.DB.Save(&user).Error; err != nil {
		return errors.New("密码重置失败：本地密码保存失败")
	}

	// 5. 验证码已使用，清除该邮箱所有 reset 类型验证码（防重放）
	db.DB.Where("email = ? AND type = ?", req.Email, models.VerificationTypeReset).
		Delete(&models.EmailVerification{})

	return nil
}
```

**设计要点**：
- 复用 `ResetPassword()` 的逻辑模式（先 Emby 再本地），但通过 email 而非 userID 查找用户
- 验证码使用后立即清除所有 reset 类型验证码，防止重放攻击
- admin 用户（没有 EmbyID 的情况）也能通过这个流程重置密码

---

### 六、AuthHandler 新增两个方法

#### 修改文件：`services/api/internal/handlers/auth.go`

```go
// SendResetCode 发送密码重置验证码
func (h *AuthHandler) SendResetCode(c *gin.Context) {
	var req SendEmailCodeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请提供有效的邮箱地址"})
		return
	}

	if err := h.emailService.SendVerificationCode(req.Email, c.ClientIP(), models.VerificationTypeReset); err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, services.ErrEmailCodeRateLimit) || errors.Is(err, services.ErrEmailCodeIPRateLimit) {
			status = http.StatusTooManyRequests
		}
		c.JSON(status, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "验证码已发送"})
}

// ResetPasswordByCode 通过验证码重置密码
func (h *AuthHandler) ResetPasswordByCode(c *gin.Context) {
	var req services.ResetPasswordByCodeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数错误"})
		return
	}

	userService := &services.UserService{}
	if err := userService.ResetPasswordByCode(&req); err != nil {
		if errors.Is(err, services.ErrEmailCodeInvalid) {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "密码重置成功"})
}
```

**设计要点**：
- `SendResetCode` 复用现有 `SendEmailCodeRequest` 结构体（只有 email 字段）
- 两个 handler 都复用现有的错误处理模式（`errors.Is` 判断 + HTTP status 映射）
- `AuthHandler` 已持有 `emailService`，无需修改 struct

---

### 七、公开路由注册

#### 修改文件：`services/api/cmd/server/main.go`

在公开路由区域（第 67 行 `api.POST("/register/send-code", ...)` 之后）新增：

```go
api.POST("/forgot-password/send-code", authHandler.SendResetCode)
api.POST("/forgot-password/reset", authHandler.ResetPasswordByCode)
```

---

### 八、前端 API 函数

#### 修改文件：`services/web/src/api/auth.ts`

新增两个函数：

```ts
// 发送密码重置验证码
export function sendResetCode(email: string): Promise<{ message: string }> {
  return request({
    url: '/forgot-password/send-code',
    method: 'post',
    data: { email }
  })
}

// 通过验证码重置密码
export function resetPasswordByCode(data: {
  email: string
  code: string
  newPassword: string
}): Promise<{ message: string }> {
  return request({
    url: '/forgot-password/reset',
    method: 'post',
    data
  })
}
```

---

### 九、新建忘记密码页面

#### 新建文件：`services/web/src/views/ForgotPasswordView.vue`

两步表单，复用 LoginView 的 `panel-clean` + `input-ember` + `btn-ember` 样式，参照 RegisterView 的验证码倒计时模式：

```vue
<script setup lang="ts">
import { onBeforeUnmount, ref } from 'vue'
import { useRouter } from 'vue-router'
import { ElMessage } from 'element-plus'
import { ArrowLeft, Lock, Message } from '@element-plus/icons-vue'
import { sendResetCode, resetPasswordByCode } from '@/api/auth'

const router = useRouter()
const step = ref(1) // 1 = 输入邮箱, 2 = 输入验证码+新密码
const loading = ref(false)
const sendingCode = ref(false)
const emailCodeCountdown = ref(0)
let countdownTimer: ReturnType<typeof setInterval> | null = null

const form = ref({
  email: '',
  code: '',
  newPassword: '',
  confirmPassword: ''
})

// 发送验证码
const handleSendCode = async () => {
  if (!form.value.email) {
    ElMessage.warning('请输入邮箱')
    return
  }

  sendingCode.value = true
  try {
    await sendResetCode(form.value.email)
    ElMessage.success('验证码已发送，请查收邮件')
    step.value = 2

    // 60s 倒计时
    emailCodeCountdown.value = 60
    countdownTimer = setInterval(() => {
      emailCodeCountdown.value -= 1
      if (emailCodeCountdown.value <= 0) {
        emailCodeCountdown.value = 0
        if (countdownTimer) {
          clearInterval(countdownTimer)
          countdownTimer = null
        }
      }
    }, 1000)
  } finally {
    sendingCode.value = false
  }
}

// 提交重置
const handleReset = async () => {
  if (!form.value.code || !form.value.newPassword) {
    ElMessage.warning('请填写完整信息')
    return
  }
  if (form.value.newPassword.length < 6) {
    ElMessage.warning('密码至少 6 位')
    return
  }
  if (form.value.newPassword !== form.value.confirmPassword) {
    ElMessage.warning('两次密码不一致')
    return
  }

  loading.value = true
  try {
    await resetPasswordByCode({
      email: form.value.email,
      code: form.value.code,
      newPassword: form.value.newPassword
    })
    ElMessage.success('密码重置成功，请重新登录')
    router.push('/login')
  } finally {
    loading.value = false
  }
}

onBeforeUnmount(() => {
  if (countdownTimer) {
    clearInterval(countdownTimer)
    countdownTimer = null
  }
})
</script>

<template>
  <div class="min-h-screen bg-cinema-bg flex items-center justify-center p-4 relative overflow-hidden">
    <div class="absolute -top-[30%] -right-[10%] w-[70%] h-[70%] bg-ember/5 opacity-60 blur-[100px] rounded-full pointer-events-none"></div>
    <div class="absolute bottom-[10%] -left-[10%] w-[40%] h-[40%] bg-gray-50 opacity-60 blur-[80px] rounded-full pointer-events-none"></div>

    <div class="w-full max-w-md relative z-10 animate-fade-in">

      <router-link to="/login" class="inline-flex items-center text-text-secondary hover:text-ember mb-8 transition-colors text-sm group">
        <el-icon class="mr-1 transition-transform group-hover:-translate-x-1"><ArrowLeft /></el-icon>
        返回登录
      </router-link>

      <div class="panel-clean rounded-2xl p-8 md:p-10">

        <div class="text-center mb-10">
          <div class="inline-flex items-center justify-center w-12 h-12 rounded-xl bg-ember/10 text-ember mb-4">
            <el-icon class="text-2xl"><Lock /></el-icon>
          </div>
          <h1 class="text-2xl font-bold text-text-primary tracking-tight mb-2">重置密码</h1>
          <p class="text-text-secondary text-sm">
            {{ step === 1 ? '输入注册时的邮箱地址' : '输入验证码和新密码' }}
          </p>
        </div>

        <!-- Step 1: 输入邮箱 -->
        <el-form v-if="step === 1" @submit.prevent="handleSendCode" size="large" class="space-y-6">
          <div class="space-y-1">
            <label class="text-xs font-semibold text-text-secondary uppercase tracking-wider ml-1">邮箱</label>
            <el-input
              v-model="form.email"
              placeholder="请输入注册邮箱"
              class="input-ember"
              :prefix-icon="Message"
            />
          </div>

          <el-button
            native-type="submit"
            :loading="sendingCode"
            class="btn-ember w-full !h-12 !text-base !rounded-xl !font-semibold mt-2 shadow-lg"
          >
            发送验证码
          </el-button>
        </el-form>

        <!-- Step 2: 输入验证码 + 新密码 -->
        <el-form v-else @submit.prevent="handleReset" size="large" class="space-y-6">
          <div class="space-y-4">
            <div class="space-y-1">
              <label class="text-xs font-semibold text-text-secondary uppercase tracking-wider ml-1">邮箱</label>
              <el-input :model-value="form.email" disabled class="input-ember" :prefix-icon="Message" />
            </div>

            <div class="space-y-1">
              <div class="flex items-center justify-between ml-1 mr-1">
                <label class="text-xs font-semibold text-text-secondary uppercase tracking-wider">验证码</label>
                <button
                  type="button"
                  class="text-xs text-ember hover:text-ember/80 transition-colors disabled:text-text-muted disabled:cursor-not-allowed"
                  :disabled="emailCodeCountdown > 0 || sendingCode"
                  @click="handleSendCode"
                >
                  {{ emailCodeCountdown > 0 ? `${emailCodeCountdown}s 后重发` : '重新发送' }}
                </button>
              </div>
              <el-input
                v-model="form.code"
                placeholder="请输入 6 位验证码"
                maxlength="6"
                class="input-ember"
              />
            </div>

            <div class="space-y-1">
              <label class="text-xs font-semibold text-text-secondary uppercase tracking-wider ml-1">新密码</label>
              <el-input
                v-model="form.newPassword"
                type="password"
                placeholder="至少 6 位"
                class="input-ember"
                :prefix-icon="Lock"
                show-password
              />
            </div>

            <div class="space-y-1">
              <label class="text-xs font-semibold text-text-secondary uppercase tracking-wider ml-1">确认密码</label>
              <el-input
                v-model="form.confirmPassword"
                type="password"
                placeholder="再次输入新密码"
                class="input-ember"
                :prefix-icon="Lock"
                show-password
              />
            </div>
          </div>

          <el-button
            native-type="submit"
            :loading="loading"
            class="btn-ember w-full !h-12 !text-base !rounded-xl !font-semibold mt-2 shadow-lg"
          >
            重置密码
          </el-button>
        </el-form>

      </div>

      <p class="text-center text-text-muted text-xs mt-8">
        &copy; 2026 Ember Project
      </p>

    </div>
  </div>
</template>

<style scoped>
.animate-fade-in {
  animation: fadeIn 0.6s ease-out forwards;
}
@keyframes fadeIn {
  from { opacity: 0; transform: translateY(10px); }
  to { opacity: 1; transform: translateY(0); }
}
</style>
```

---

### 十、前端路由注册

#### 修改文件：`services/web/src/router/index.ts`

在 `register` 路由（第 20-21 行）之后新增：

```ts
{
  path: '/forgot-password',
  name: 'forgot-password',
  component: () => import('../views/ForgotPasswordView.vue'),
},
```

---

### 十一、登录页加入口链接

#### 修改文件：`services/web/src/views/LoginView.vue`

在密码输入框 `</div>` 闭合标签（第 92 行）之后、`<el-button>` 登录按钮之前插入：

```html
<div class="text-right -mt-2 mb-2">
  <router-link to="/forgot-password" class="text-xs text-text-secondary hover:text-ember transition-colors">
    忘记密码？
  </router-link>
</div>
```

---

## Part 2：Telegram `/resetpw` 命令（Bot）

Telegram 身份本身就是认证——控制已绑定的 Telegram 账号即证明身份，无需额外验证码。完全复用 `/redeem` 命令的架构模式：Bot 调内部 API → 后端按 `telegramId` 查用户 → 执行操作。

### 十二、TelegramService 新增重置方法

#### 修改文件：`services/api/internal/services/telegram.go`

新增请求体和方法：

```go
// TelegramResetPasswordRequest Bot 调 Internal API 重置密码
type TelegramResetPasswordRequest struct {
	TelegramID  int64  `json:"telegramId" binding:"required"`
	NewPassword string `json:"newPassword" binding:"required,min=6"`
}

// ResetPassword 通过 Telegram 身份重置密码
func (s *TelegramService) ResetPassword(telegramID int64, newPassword string) error {
	// 1. 按 telegramId 查找用户
	var user models.User
	if err := db.DB.Where("\"telegramId\" = ?", telegramID).First(&user).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrTelegramNotBound
		}
		return errors.New("密码重置失败，请稍后重试")
	}

	// 2. 更新 Emby 密码
	if user.EmbyID != "" {
		embyService := NewEmbyService()
		if err := embyService.UpdateUserPassword(user.EmbyID, newPassword); err != nil {
			return errors.New("密码重置失败：" + err.Error())
		}
	}

	// 3. 更新本地 bcrypt 哈希
	if err := user.SetPassword(newPassword); err != nil {
		return errors.New("密码重置失败：本地密码更新失败")
	}
	if err := db.DB.Save(&user).Error; err != nil {
		return errors.New("密码重置失败：本地密码保存失败")
	}

	return nil
}
```

**设计要点**：
- 按 `telegramId` 查找用户的模式与 `GetAccountInfo`、`RedeemByTelegram` 完全一致
- 密码更新逻辑与 `UserService.ResetPassword` 一致（先 Emby 再本地）
- 未绑定用户返回 `ErrTelegramNotBound`

---

### 十三、TelegramHandler 新增方法

#### 修改文件：`services/api/internal/handlers/telegram.go`

复用 `RedeemByTelegram` handler 的模式：

```go
// ResetPassword Bot 通过 Telegram 重置密码
func (h *TelegramHandler) ResetPassword(c *gin.Context) {
	var req services.TelegramResetPasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数错误"})
		return
	}

	if err := h.telegramService.ResetPassword(req.TelegramID, req.NewPassword); err != nil {
		statusCode := http.StatusInternalServerError
		if errors.Is(err, services.ErrTelegramNotBound) {
			statusCode = http.StatusBadRequest
		}
		c.JSON(statusCode, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "密码重置成功"})
}
```

---

### 十四、内部路由注册

#### 修改文件：`services/api/cmd/server/main.go`

在内部路由组（第 135 行 `internal.POST("/telegram/redeem", ...)` 之后）新增：

```go
internal.POST("/telegram/reset-password", telegramHandler.ResetPassword)
```

---

### 十五、Bot API Client 新增方法

#### 修改文件：`services/bot/app/clients/api_client.py`

复用 `redeem_by_telegram` 的模式：

```python
async def reset_password_by_telegram(telegram_id: int, new_password: str) -> Optional[dict]:
    """通过 Telegram 身份重置密码"""
    url = f"{API_URL}/api/v1/internal/telegram/reset-password"
    headers = {"X-Internal-Secret": INTERNAL_API_SECRET}

    try:
        async with httpx.AsyncClient(timeout=10.0) as client:
            resp = await client.post(
                url,
                headers=headers,
                json={
                    "telegramId": telegram_id,
                    "newPassword": new_password,
                },
            )
        if resp.status_code == 200:
            return resp.json()
        return {"error": resp.json().get("error", "密码重置失败")}
    except Exception:
        return None
```

---

### 十六、Bot 命令处理器

#### 修改文件：`services/bot/app/handlers/telegram_handler.py`

复用 `handle_redeem` 的模式：

```python
async def handle_resetpw(update: Update, context: ContextTypes.DEFAULT_TYPE) -> None:
    message = update.message
    if message is None or message.from_user is None:
        return

    if message.chat.type != "private":
        await message.reply_text("⚠️ 请在私聊中使用此命令")
        return

    args = context.args or []
    if len(args) != 1 or len(args[0]) < 6:
        await message.reply_text(
            "📝 <b>使用方式</b>\n\n"
            "/resetpw <code>新密码</code>\n\n"
            "密码至少 6 位，将同时更新 Ember 和 Emby 登录密码。",
            parse_mode="HTML",
        )
        return

    result = await api_client.reset_password_by_telegram(message.from_user.id, args[0])
    if result is None:
        await message.reply_text("❌ 服务暂不可用，请稍后重试")
        return
    if "error" in result:
        await message.reply_text(f"❌ {escape(str(result['error']))}", parse_mode="HTML")
        return

    await message.reply_text(
        "✅ <b>密码重置成功</b>\n\n"
        "新密码已同步到 Ember 和 Emby，请使用新密码登录。",
        parse_mode="HTML",
    )
```

**设计要点**：
- 密码长度校验在 Bot 端先做一次（`len(args[0]) < 6`），后端 binding 也会校验
- 私聊强制检查（与所有现有命令一致）
- 参数只接受 1 个（新密码），使用方式与 `/redeem <code>` 一致
- 成功消息明确告知"已同步到 Ember 和 Emby"

---

### 十七、Bot 注册命令

#### 修改文件：`services/bot/app/server.py`

第 47 行 `tg_app.add_handler(CommandHandler("redeem", handle_redeem))` 之后新增：

```python
tg_app.add_handler(CommandHandler("resetpw", handle_resetpw))
```

同时更新第 25-33 行的 import：

```python
from app.handlers.telegram_handler import (
    handle_bind,
    handle_callback,
    handle_info,
    handle_new_member,
    handle_redeem,
    handle_resetpw,  # 新增
    send_registration_notification,
    send_ranking_notification,
    send_subscription_notification,
)
```

---

## API 端点汇总

| 方法 | 路径 | 认证 | 请求体 | 响应 |
|------|------|------|--------|------|
| POST | `/api/v1/forgot-password/send-code` | 无（公开） | `{ email }` | `{ message }` |
| POST | `/api/v1/forgot-password/reset` | 无（公开） | `{ email, code, newPassword }` | `{ message }` |
| POST | `/api/v1/internal/telegram/reset-password` | X-Internal-Secret | `{ telegramId, newPassword }` | `{ message }` |

---

## 文件变更清单

| # | 文件 | 操作 | 说明 |
|---|------|------|------|
| 1 | `services/api/internal/models/email_verification.go` | 修改 | 加 `Type` 字段 + 常量 |
| 2 | `services/api/internal/services/errors.go` | 修改 | 加 `ErrEmailNotRegistered` |
| 3 | `services/api/internal/services/email.go` | 修改 | `SendVerificationCode` 和 `VerifyCode` 加 `codeType` 参数 |
| 4 | `services/api/internal/services/auth.go` | 修改 | `VerifyCode` 调用加第三参数 |
| 5 | `services/api/internal/handlers/auth.go` | 修改 | `SendVerificationCode` 调用加第三参数 + 新增 2 个 handler + 新增 models import |
| 6 | `services/api/internal/services/user.go` | 修改 | 新增 `ResetPasswordByCode` |
| 7 | `services/api/internal/services/telegram.go` | 修改 | 新增 `ResetPassword` + 请求体 |
| 8 | `services/api/internal/handlers/telegram.go` | 修改 | 新增 `ResetPassword` handler |
| 9 | `services/api/cmd/server/main.go` | 修改 | 注册公开路由 + 内部路由 |
| 10 | `services/bot/app/clients/api_client.py` | 修改 | 新增 `reset_password_by_telegram` |
| 11 | `services/bot/app/handlers/telegram_handler.py` | 修改 | 新增 `handle_resetpw` |
| 12 | `services/bot/app/server.py` | 修改 | 注册命令 + 更新 import |
| 13 | `services/web/src/api/auth.ts` | 修改 | 新增 2 个 API 函数 |
| 14 | `services/web/src/views/ForgotPasswordView.vue` | **新建** | 找回密码页面 |
| 15 | `services/web/src/router/index.ts` | 修改 | 注册路由 |
| 16 | `services/web/src/views/LoginView.vue` | 修改 | 加"忘记密码？"链接 |
| 17 | `docs/SYSTEM-ARCHITECTURE.md` | 修改 | 更新架构文档 |

---

## 执行顺序

**Phase 1 — 后端：邮箱重置**（步骤 1-6）
1. `internal/models/email_verification.go` — 加 `Type` 字段 + 常量
2. `internal/services/errors.go` — 加 `ErrEmailNotRegistered`
3. `internal/services/email.go` — `SendVerificationCode` 和 `VerifyCode` 加 `codeType` 参数
4. `internal/services/auth.go` — 更新 `VerifyCode` 调用
5. `internal/handlers/auth.go` — 更新 `SendVerificationCode` 调用 + 新增 2 个 handler
6. `internal/services/user.go` — 新增 `ResetPasswordByCode`

**Phase 2 — 后端：Telegram 重置**（步骤 7-9）
7. `internal/services/telegram.go` — 新增 `ResetPassword` 方法
8. `internal/handlers/telegram.go` — 新增 `ResetPassword` handler
9. `cmd/server/main.go` — 注册公开路由 + 内部路由

**Phase 3 — 编译验证**
10. `cd services/api && go build ./...`

**Phase 4 — Bot**（步骤 11-13）
11. `services/bot/app/clients/api_client.py` — 新增 API 方法
12. `services/bot/app/handlers/telegram_handler.py` — 新增 `handle_resetpw`
13. `services/bot/app/server.py` — 注册命令

**Phase 5 — 前端**（步骤 14-17）
14. `services/web/src/api/auth.ts` — 新增 API 函数
15. `services/web/src/views/ForgotPasswordView.vue` — 新建页面
16. `services/web/src/router/index.ts` — 注册路由
17. `services/web/src/views/LoginView.vue` — 加"忘记密码"链接

**Phase 6 — 前端编译验证**
18. `cd services/web && npm run build`

**Phase 7 — 收尾**
19. 更新 `docs/SYSTEM-ARCHITECTURE.md`

---

## 验证方式

编译验证后，由用户手动测试：

**邮箱重置：**
- 已注册邮箱发送重置码 → 收到邮件
- 未注册邮箱发送 → 返回"该邮箱未注册"
- 正确验证码 + 新密码提交 → 成功
- 用新密码登录 → 成功；旧密码 → 失败
- 重复使用同一验证码 → "验证码无效或已过期"
- 连续发送 6 次 → 触发频率限制

**Telegram 重置：**
- 已绑定用户发 `/resetpw newpass123` → 成功，新密码可登录 Ember 和 Emby
- 未绑定用户发 `/resetpw xxx` → "尚未绑定 Telegram 账号"
- 不带参数 → 提示使用方式
- 密码少于 6 位 → 提示使用方式

**回归测试：**
- 注册流程不受影响（`SendVerificationCode` 加了 type 参数但默认行为不变）
- 现有 Bot 命令（`/bind`、`/info`、`/redeem`）不受影响
