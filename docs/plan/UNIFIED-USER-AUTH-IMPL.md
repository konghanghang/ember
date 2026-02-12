# 用户/管理员合并实施方案

## Context

当前 Ember 项目存在 `admins` 和 `users` 两张独立的表，分别对应两套完全不同的认证逻辑（admin 本地 bcrypt / user Emby API）。这导致了大量代码重复：两套 model、两套 login handler、两套前端 login view、两套 API 函数。

**目标**：合并为单表 `users` + `role` 字段，统一 `/login` 入口，删除所有 admin 专用的重复代码。新项目，无需数据迁移。

**核心设计决策**：统一 `/login` 端点内部按 `role` 分发认证方式：
- `role=admin` → 本地 bcrypt 验证（password 字段）
- `role=user` → Emby API 远程验证（无本地密码）

---

## 不需要修改的文件（已有设计正确）

- `services/api/internal/common/jwt.go` — Claims 已有 Role 字段
- `services/api/internal/middleware/jwt.go` — AdminOnly/UserOnly 已正确实现

---

## 实施步骤

### 第 1 步：修改 User 模型

**文件**：`services/api/internal/models/user.go`

改后完整 struct：

```go
package models

import (
	"time"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

// User 统一用户模型（admin + user）
// role 字段区分角色：admin 使用本地密码，user 通过 Emby 认证
type User struct {
	ID         string     `json:"id" gorm:"column:id;type:varchar(25);primaryKey"`
	Username   string     `json:"username" gorm:"column:username;uniqueIndex;size:50;not null"`
	Role       string     `json:"role" gorm:"column:role;size:10;not null;default:user"`
	Password   string     `json:"-" gorm:"column:password"`                                   // admin 专用，bcrypt hash，user 为空
	Email      string     `json:"email,omitempty" gorm:"column:email;size:255"`                // user 专用
	EmbyID     string     `json:"embyId,omitempty" gorm:"column:embyId;size:50;index"`         // user 专用
	InviteCode string     `json:"inviteCode,omitempty" gorm:"column:inviteCode;size:20;index"` // user 专用
	ExpiresAt  *time.Time `json:"expiresAt,omitempty" gorm:"column:expiresAt"`
	IsActive   bool       `json:"isActive" gorm:"column:isActive;default:true;not null"`
	CreatedAt  time.Time  `json:"createdAt" gorm:"column:createdAt;autoCreateTime"`
	UpdatedAt  time.Time  `json:"updatedAt" gorm:"column:updatedAt;autoUpdateTime"`

	Invite        *Invite        `json:"-" gorm:"foreignKey:InviteCode;references:Code"`
	Subscriptions []Subscription `json:"-" gorm:"foreignKey:UserID"`
}

func (User) TableName() string {
	return "users"
}

func (u *User) BeforeCreate(tx *gorm.DB) error {
	if u.ID == "" {
		u.ID = generateCUID()
	}
	return nil
}

// IsExpired 检查账号是否过期
func (u *User) IsExpired() bool {
	if u.ExpiresAt == nil {
		return false
	}
	return u.ExpiresAt.Before(time.Now())
}

// IsAdmin 是否为管理员
func (u *User) IsAdmin() bool {
	return u.Role == "admin"
}

// SetPassword 设置加密密码（admin 专用）
func (u *User) SetPassword(password string) error {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	u.Password = string(hash)
	return nil
}

// CheckPassword 验证密码（admin 专用）
func (u *User) CheckPassword(password string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(u.Password), []byte(password))
	return err == nil
}
```

关键变更点：
1. 新增 `Role` 字段 — `not null;default:user`
2. 新增 `Password` 字段 — nullable，`json:"-"` 永不序列化
3. `Email` 去掉 `not null`（admin 不需要邮箱）
4. `EmbyID` 去掉 `uniqueIndex` 和 `not null`，改为普通 `index`（admin 无 EmbyID）
5. `InviteCode` 去掉 `not null`（admin 不需要邀请码）
6. 从 `admin.go` 迁入 `SetPassword()`、`CheckPassword()` 方法
7. 新增 `IsAdmin()` 方法
8. import 新增 `golang.org/x/crypto/bcrypt`

---

### 第 2 步：删除 Admin 模型

**删除文件**：`services/api/internal/models/admin.go`

---

### 第 3 步：修改数据库初始化

**文件**：`services/api/internal/db/db.go`

变更：

1. `AutoMigrate()` 中移除 `&models.Admin{}`：
```go
func AutoMigrate() error {
	return DB.AutoMigrate(
		&models.Invite{},
		&models.User{},
		&models.Subscription{},
	)
}
```

2. 新增 `seedDefaultAdmin()` 函数：
```go
// seedDefaultAdmin 初始化默认管理员账号
func seedDefaultAdmin() {
	var count int64
	DB.Model(&models.User{}).Where("role = ?", "admin").Count(&count)
	if count > 0 {
		return
	}

	username := os.Getenv("ADMIN_USERNAME")
	password := os.Getenv("ADMIN_PASSWORD")
	if username == "" || password == "" {
		log.Println("⚠️  跳过 admin 初始化：ADMIN_USERNAME 或 ADMIN_PASSWORD 未设置")
		return
	}

	admin := models.User{
		Username: username,
		Role:     "admin",
		IsActive: true,
	}
	if err := admin.SetPassword(password); err != nil {
		log.Printf("❌ 创建默认管理员失败：%v", err)
		return
	}
	if err := DB.Create(&admin).Error; err != nil {
		log.Printf("❌ 创建默认管理员失败：%v", err)
		return
	}
	log.Printf("✅ 默认管理员已创建：%s", admin.Username)
}
```

3. 在 `InitDB()` 末尾（`fmt.Println("✅ 数据库连接成功")` 之前）调用：
```go
	// 自动迁移表结构
	if err := AutoMigrate(); err != nil {
		log.Fatalf("❌ 数据库迁移失败：%v", err)
	}

	// 初始化默认管理员
	seedDefaultAdmin()

	fmt.Println("✅ 数据库连接成功")
```

同时删除原有的注释块（第 98-101 行的"不执行 AutoMigrate"注释）。

> **改进：AutoMigrate 环境开关**
>
> 在 `InitDB()` 中通过环境变量控制是否执行 AutoMigrate：
> ```go
> // 按需自动迁移表结构
> if os.Getenv("AUTO_MIGRATE") == "true" {
> 	if err := AutoMigrate(); err != nil {
> 		log.Fatalf("❌ 数据库迁移失败：%v", err)
> 	}
> 	log.Println("✅ 数据库迁移完成")
> } else {
> 	log.Println("ℹ️  AUTO_MIGRATE 未启用，跳过数据库迁移")
> }
> ```
> `.env` 新增：`AUTO_MIGRATE=true`（开发环境默认开启，生产环境按需关闭）

---

### 第 4 步：重写 auth service

**文件**：`services/api/internal/services/auth.go`

改后完整内容：

```go
package services

import (
	"errors"
	"log"

	"github.com/konghang/ember/backend/internal/common"
	"github.com/konghang/ember/backend/internal/db"
	"github.com/konghang/ember/backend/internal/models"
)

// AuthService 认证服务
type AuthService struct{}

// LoginRequest 统一登录请求
type LoginRequest struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
}

// LoginResponse 统一登录响应
type LoginResponse struct {
	Token string       `json:"token"`
	User  *models.User `json:"user"`
}

// Login 统一登录（admin: bcrypt 本地验证 / user: Emby API 远程验证）
func (s *AuthService) Login(req *LoginRequest) (*LoginResponse, error) {
	// 1. 查询用户
	var user models.User
	result := db.DB.Where("username = ?", req.Username).First(&user)
	if result.Error != nil {
		return nil, errors.New("用户名或密码错误")
	}

	// 2. 根据角色选择认证方式
	if user.IsAdmin() {
		// admin: 本地 bcrypt 验证
		if !user.CheckPassword(req.Password) {
			return nil, errors.New("用户名或密码错误")
		}
	} else {
		// user: Emby API 远程验证
		if !user.IsActive {
			return nil, errors.New("账号已被禁用")
		}
		if user.IsExpired() {
			return nil, errors.New("账号已过期")
		}
		embyService := NewEmbyService()
		embyUser, err := embyService.AuthenticateUser(user.Username, req.Password)
		if err != nil {
			return nil, errors.New("用户名或密码错误")
		}
		if embyUser.ID != user.EmbyID {
			return nil, errors.New("用户信息不匹配")
		}
	}

	// 3. 生成 JWT Token
	token, err := common.GenerateToken(user.ID, user.Username, user.Role)
	if err != nil {
		return nil, errors.New("生成 Token 失败")
	}

	return &LoginResponse{
		Token: token,
		User:  &user,
	}, nil
}

// RegisterUserRequest 用户注册请求
type RegisterUserRequest struct {
	Username   string `json:"username" binding:"required,min=3,max=50"`
	Password   string `json:"password" binding:"required,min=6"`
	Email      string `json:"email" binding:"required,email"`
	InviteCode string `json:"inviteCode" binding:"required"`
}

// RegisterUserResponse 用户注册响应
type RegisterUserResponse struct {
	Token string       `json:"token"`
	User  *models.User `json:"user"`
}

// RegisterUser 用户注册
func (s *AuthService) RegisterUser(req *RegisterUserRequest) (*RegisterUserResponse, error) {
	// 1. 验证邀请码
	inviteService := &InviteService{}
	invite, err := inviteService.ValidateInvite(req.InviteCode)
	if err != nil {
		return nil, err
	}

	// 2. 检查用户名是否已存在
	var existingUser models.User
	result := db.DB.Where("username = ?", req.Username).First(&existingUser)
	if result.Error == nil {
		return nil, errors.New("用户名已存在")
	}

	// 3. 创建 Emby 用户
	embyService := NewEmbyService()
	embyUser, err := embyService.CreateEmbyUser(req.Username, req.Password)
	if err != nil {
		return nil, errors.New("创建 Emby 用户失败：" + err.Error())
	}

	// 4. 计算到期时间
	expiresAt := common.CalculateExpiryDate(invite.DefaultDays)

	// 5. 创建数据库用户记录
	user := models.User{
		Username:   req.Username,
		Role:       "user", // 显式设置角色
		Email:      req.Email,
		EmbyID:     embyUser.ID,
		InviteCode: req.InviteCode,
		ExpiresAt:  &expiresAt,
		IsActive:   true,
	}

	if err := db.DB.Create(&user).Error; err != nil {
		return nil, errors.New("创建用户失败")
	}

	// 6. 使用邀请码
	if err := inviteService.UseInvite(req.InviteCode); err != nil {
		log.Printf("⚠️  邀请码使用次数更新失败（不影响注册）：code=%s, err=%v", req.InviteCode, err)
	}

	// 7. 生成 JWT Token
	token, err := common.GenerateToken(user.ID, user.Username, "user")
	if err != nil {
		return nil, errors.New("生成 Token 失败")
	}

	return &RegisterUserResponse{
		Token: token,
		User:  &user,
	}, nil
}

// GetCurrentUser 获取当前用户信息（统一查 users 表）
func (s *AuthService) GetCurrentUser(userID string) (*models.User, error) {
	var user models.User
	result := db.DB.Where("id = ?", userID).First(&user)
	if result.Error != nil {
		return nil, errors.New("用户不存在")
	}
	return &user, nil
}
```

关键变更点：
1. 删除 `AdminLoginRequest`、`AdminLoginResponse`、`AdminLogin()`
2. 删除 `UserLoginRequest`、`UserLoginResponse`、`UserLogin()`
3. 新增统一的 `LoginRequest`、`LoginResponse`、`Login()`
4. `GetCurrentUser()` 签名简化：去掉 `role` 参数，不再区分 admin/user 查不同表
5. `RegisterUser()` 中创建用户时显式设置 `Role: "user"`

---

### 第 5 步：重写 auth handler

**文件**：`services/api/internal/handlers/auth.go`

改后完整内容：

```go
package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/konghang/ember/backend/internal/services"
)

// AuthHandler 认证处理器
type AuthHandler struct {
	authService *services.AuthService
}

// NewAuthHandler 创建认证处理器
func NewAuthHandler() *AuthHandler {
	return &AuthHandler{
		authService: &services.AuthService{},
	}
}

// Login 统一登录
// @Summary 统一登录（admin + user）
// @Tags 认证
// @Accept json
// @Produce json
// @Param body body services.LoginRequest true "登录信息"
// @Success 200 {object} services.LoginResponse
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Router /api/v1/login [post]
func (h *AuthHandler) Login(c *gin.Context) {
	var req services.LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "请求参数错误",
		})
		return
	}

	resp, err := h.authService.Login(&req)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{
			"error": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, resp)
}

// GetCurrentUser 获取当前用户信息
// @Summary 获取当前用户信息
// @Tags 认证
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Failure 401 {object} ErrorResponse
// @Router /api/v1/admin/current [get]
// @Security BearerAuth
func (h *AuthHandler) GetCurrentUser(c *gin.Context) {
	userID, _ := c.Get("userID")

	user, err := h.authService.GetCurrentUser(userID.(string))
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{
			"error": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"user": user,
	})
}

// Logout 统一登出（JWT 无状态，仅返回成功）
// @Summary 统一登出
// @Tags 认证
// @Produce json
// @Success 200 {object} SuccessResponse
// @Router /api/v1/logout [post]
// @Security BearerAuth
func (h *AuthHandler) Logout(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"message": "登出成功",
	})
}

// RegisterUser 用户注册
// @Summary 用户注册
// @Tags 认证
// @Accept json
// @Produce json
// @Param body body services.RegisterUserRequest true "注册信息"
// @Success 200 {object} services.RegisterUserResponse
// @Failure 400 {object} ErrorResponse
// @Router /api/v1/user/register [post]
func (h *AuthHandler) RegisterUser(c *gin.Context) {
	var req services.RegisterUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "请求参数错误",
		})
		return
	}

	resp, err := h.authService.RegisterUser(&req)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, resp)
}

// ErrorResponse 错误响应结构
type ErrorResponse struct {
	Error string `json:"error"`
}

// SuccessResponse 成功响应结构
type SuccessResponse struct {
	Message string `json:"message"`
}
```

关键变更点：
1. 删除 `AdminLogin()`、`UserLogin()` → 统一为 `Login()`
2. 删除 `AdminLogout()`、`UserLogout()` → 统一为 `Logout()`
3. `GetCurrentUser()` 不再传 role 参数
4. 补回 Swagger 注释（`@Summary`、`@Tags`、`@Router` 等）

---

### 第 6 步：修改路由注册

**文件**：`services/api/cmd/server/main.go`

变更 3 处：

**公开路由**（约第 47-54 行）：
```go
// 改前
api.POST("/admin/login", authHandler.AdminLogin)
api.POST("/user/login", authHandler.UserLogin)
api.POST("/user/register", authHandler.RegisterUser)

// 改后
api.POST("/login", authHandler.Login)
api.POST("/logout", middleware.JWTAuth(), authHandler.Logout) // 统一登出，仅需 JWT 认证
api.POST("/user/register", authHandler.RegisterUser)
```

**Admin 路由组内**（约第 67-68 行）：
```go
// 改前
admin.GET("/current", authHandler.GetCurrentUser)
admin.POST("/logout", authHandler.AdminLogout)

// 改后（删除 logout，移至公开路由）
admin.GET("/current", authHandler.GetCurrentUser)
```

**User 路由组内**（约第 101 行）：
```go
// 改前
user.POST("/logout", authHandler.UserLogout)

// 改后（删除 logout，移至公开路由）
// （已移除）
```

> **改进：统一 logout 路由**
>
> JWT 无状态，logout 不关心角色。将 `/admin/logout` 和 `/user/logout` 合并为
> `POST /api/v1/logout`（仅需 JWTAuth，不检查 role）。
> 前端只需调用固定 URL，无需按 role 分发。

---

### 第 7 步：前端类型定义

**文件**：`services/web/src/types/api.ts`

变更：

```typescript
// ===== 删除以下接口 =====
// export interface AdminInfo { ... }
// export interface AdminLoginResponse { ... }
// export interface UserLoginResponse { ... }

// ===== 修改 UserInfo，新增 role，部分字段改为可选 =====
export interface UserInfo {
  id: string
  username: string
  role: UserRole           // 新增
  email?: string           // 改为可选（admin 无邮箱）
  embyId?: string          // 改为可选（admin 无 EmbyID）
  expiresAt?: string       // 保持可选
  isActive: boolean
  createdAt: string
}

// ===== 新增统一登录响应 =====
export interface LoginResponse {
  token: string
  user: UserInfo
}

// ===== RegisterResponse 改为与 LoginResponse 结构一致 =====
export interface RegisterResponse {
  token: string
  user: UserInfo
}
```

---

### 第 8 步：前端 API 调用

**文件**：`services/web/src/api/auth.ts`

改后完整内容：

```typescript
import request from './request'
import type {
  LoginCredentials,
  LoginResponse,
  RegisterRequest,
  RegisterResponse,
  ValidateInviteResponse
} from '@/types/api'

// 统一登录
export function login(data: LoginCredentials): Promise<LoginResponse> {
  return request({
    url: '/login',
    method: 'post',
    data
  })
}

// 统一登出（不区分角色）
export function logout() {
  return request({ url: '/logout', method: 'post' })
}

// 用户注册
export function register(data: RegisterRequest): Promise<RegisterResponse> {
  return request({
    url: '/user/register',
    method: 'post',
    data
  })
}

// 验证邀请码
export function validateInviteCode(code: string): Promise<ValidateInviteResponse> {
  return request({
    url: `/invites/${code}/validate`,
    method: 'get'
  })
}
```

关键变更点：
1. `login()` URL 从 `/admin/login` 改为 `/login`，返回类型改为 `LoginResponse`
2. 删除 `userLogin()`、`getCurrentAdmin()`、`adminLogout()`、`userLogout()`
3. `logout()` 简化为固定 URL `/logout`（不再按 role 分发）
4. 清理 import：删除 `AdminInfo`、`AdminLoginResponse`、`UserLoginResponse`

---

### 第 9 步：前端 Auth Store

**文件**：`services/web/src/store/auth.ts`

改后完整内容：

```typescript
import { defineStore } from 'pinia'
import { ref, computed } from 'vue'
import * as authApi from '@/api/auth'
import type { LoginCredentials, RegisterRequest, LoginResponse, RegisterResponse } from '@/types/api'

export const useAuthStore = defineStore('auth', () => {
  // 状态
  const token = ref<string | null>(localStorage.getItem('token'))
  const role = ref<string | null>(localStorage.getItem('role'))

  // 计算属性
  const isAuthenticated = computed(() => !!token.value)
  const isAdmin = computed(() => role.value === 'admin')
  const isUser = computed(() => role.value === 'user')

  // 统一登录
  const login = async (credentials: LoginCredentials) => {
    const res: LoginResponse = await authApi.login(credentials)
    setAuth(res.token, res.user.role as 'admin' | 'user')
    return res
  }

  // 用户注册
  const register = async (data: RegisterRequest) => {
    const res: RegisterResponse = await authApi.register(data)
    setAuth(res.token, 'user')
    return res
  }

  // 登出
  const logout = async () => {
    try {
      await authApi.logout()
    } finally {
      clearAuth()
    }
  }

  const setAuth = (newToken: string, newRole: 'admin' | 'user') => {
    token.value = newToken
    role.value = newRole
    localStorage.setItem('token', newToken)
    localStorage.setItem('role', newRole)
  }

  const clearAuth = () => {
    token.value = null
    role.value = null
    localStorage.removeItem('token')
    localStorage.removeItem('role')
  }

  const restoreAuth = () => {
    const savedToken = localStorage.getItem('token')
    const savedRole = localStorage.getItem('role')
    if (savedToken && savedRole) {
      token.value = savedToken
      role.value = savedRole
    }
  }

  const validateInvite = async (code: string) => {
    return await authApi.validateInviteCode(code)
  }

  return {
    token,
    role,
    isAuthenticated,
    isAdmin,
    isUser,
    login,
    register,
    logout,
    setAuth,
    clearAuth,
    restoreAuth,
    validateInvite
  }
})
```

关键变更点：
1. 删除 `adminLogin()` 和 `userLogin()`
2. 新增统一的 `login()` — 从响应 `res.user.role` 提取角色
3. `logout()` 简化，不再传 role 参数
4. return 中导出 `login` 替代 `adminLogin, userLogin`

---

### 第 10 步：统一登录页面

**删除**：`services/web/src/views/admin/LoginView.vue`
**删除**：`services/web/src/views/user/LoginView.vue`
**新建**：`services/web/src/views/LoginView.vue`

以 `user/LoginView.vue` 为基础，改后完整内容：

```vue
<script setup lang="ts">
import { ref } from 'vue'
import { useRouter } from 'vue-router'
import { ElMessage } from 'element-plus'
import { useAuthStore } from '@/store/auth'
import { User, Lock, ArrowLeft } from '@element-plus/icons-vue'

const router = useRouter()
const authStore = useAuthStore()
const form = ref({
  username: '',
  password: ''
})
const loading = ref(false)

const handleLogin = async () => {
  if (!form.value.username || !form.value.password) {
    ElMessage.warning('请输入用户名和密码')
    return
  }

  loading.value = true
  try {
    const res = await authStore.login(form.value)
    ElMessage.success('登录成功')
    // 根据角色跳转
    if (res.user.role === 'admin') {
      router.push('/admin/users')
    } else {
      router.push('/user/dashboard')
    }
  } catch {
    ElMessage.error('登录失败，请检查用户名或密码')
  } finally {
    loading.value = false
  }
}
</script>

<template>
  <div class="min-h-screen bg-cinema-bg flex items-center justify-center p-4 relative overflow-hidden">
    <div class="absolute -top-[30%] -right-[10%] w-[70%] h-[70%] bg-ember/5 opacity-60 blur-[100px] rounded-full pointer-events-none"></div>
    <div class="absolute bottom-[10%] -left-[10%] w-[40%] h-[40%] bg-gray-50 opacity-60 blur-[80px] rounded-full pointer-events-none"></div>

    <div class="w-full max-w-md relative z-10 animate-fade-in">

      <router-link to="/" class="inline-flex items-center text-text-secondary hover:text-ember mb-8 transition-colors text-sm group">
        <el-icon class="mr-1 transition-transform group-hover:-translate-x-1"><ArrowLeft /></el-icon>
        返回首页
      </router-link>

      <div class="panel-clean rounded-2xl p-8 md:p-10">

        <div class="text-center mb-10">
          <div class="inline-flex items-center justify-center w-12 h-12 rounded-xl bg-ember/10 text-ember mb-4">
            <el-icon class="text-2xl"><User /></el-icon>
          </div>
          <h1 class="text-2xl font-bold text-text-primary tracking-tight mb-2">欢迎回来</h1>
          <p class="text-text-secondary text-sm">登录您的 Ember 账号</p>
        </div>

        <el-form :model="form" @submit.prevent="handleLogin" size="large" class="space-y-6">
          <div class="space-y-4">
            <div class="space-y-1">
              <label class="text-xs font-semibold text-text-secondary uppercase tracking-wider ml-1">用户名</label>
              <el-input
                v-model="form.username"
                placeholder="请输入用户名"
                class="input-ember"
                :prefix-icon="User"
              />
            </div>

            <div class="space-y-1">
              <label class="text-xs font-semibold text-text-secondary uppercase tracking-wider ml-1">密码</label>
              <el-input
                v-model="form.password"
                type="password"
                placeholder="••••••••"
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
            登 录
          </el-button>
        </el-form>

        <div class="mt-8 pt-6 border-t border-gray-100 text-center text-sm">
          <router-link to="/register" class="text-text-secondary hover:text-ember transition-colors font-medium">
            注册新账号
          </router-link>
        </div>

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

关键变更点：
1. `authStore.userLogin()` → `authStore.login()`
2. 登录成功后按 `res.user.role` 分发跳转
3. 描述文案从"私人账号"改为"Ember 账号"
4. 删除 footer 中"管理员登录"链接

---

### 第 11 步：前端路由

**文件**：`services/web/src/router/index.ts`

**路由定义**（约第 12-21 行）：
```typescript
// 改前
{
  path: '/login',
  name: 'admin-login',
  component: () => import('../views/admin/LoginView.vue'),
},
{
  path: '/user/login',
  name: 'user-login',
  component: () => import('../views/user/LoginView.vue'),
},

// 改后
{
  path: '/login',
  name: 'login',
  component: () => import('../views/LoginView.vue'),
},
```

**Navigation Guard**（约第 87-109 行）：
```typescript
// 改前
router.beforeEach((to, _from, next) => {
  const authStore = useAuthStore()
  authStore.restoreAuth()
  if (to.meta.requiresAuth) {
    if (!authStore.isAuthenticated) {
      if (to.meta.role === 'admin') {
        next({ name: 'admin-login', query: { redirect: to.fullPath } })
      } else {
        next({ name: 'user-login', query: { redirect: to.fullPath } })
      }
      return
    }
    if (to.meta.role && to.meta.role !== authStore.role) {
      next({ name: 'home' })
      return
    }
  }
  next()
})

// 改后
router.beforeEach((to, _from, next) => {
  const authStore = useAuthStore()
  authStore.restoreAuth()
  if (to.meta.requiresAuth) {
    if (!authStore.isAuthenticated) {
      next({ name: 'login', query: { redirect: to.fullPath } })
      return
    }
    if (to.meta.role && to.meta.role !== authStore.role) {
      next({ name: 'home' })
      return
    }
  }
  next()
})
```

---

### 第 12 步：前端请求拦截器

**文件**：`services/web/src/api/request.ts`

401 处理（约第 37-41 行）：
```typescript
// 改前
if (window.location.pathname.startsWith('/admin')) {
  router.push('/login')
} else {
  router.push('/user/login')
}

// 改后
router.push('/login')
```

---

### 第 13 步：首页导航链接

**文件**：`services/web/src/components/home/Navbar.vue`

第 44 行（desktop nav）：
```html
<!-- 改前 -->
<router-link to="/user/login" class="...">登录</router-link>

<!-- 改后 -->
<router-link to="/login" class="...">登录</router-link>
```

第 67 行（mobile menu）：
```html
<!-- 改前 -->
<router-link to="/user/login" class="..." @click="mobileMenuOpen = false">登录</router-link>

<!-- 改后 -->
<router-link to="/login" class="..." @click="mobileMenuOpen = false">登录</router-link>
```

**文件**：`services/web/src/views/HomeView.vue`

第 51 行按钮（已指向 `/login`，只需改文案）：
```html
<!-- 改前 -->
<button @click="router.push('/login')" class="...">用户登录</button>

<!-- 改后 -->
<button @click="router.push('/login')" class="...">登录</button>
```

---

## 附加清理：彻底删除 Prisma + 移除所有外键

### 背景

当前项目遗留了 Prisma 备份文件和文档引用，且 GORM model 中定义了外键关系。需要：
1. 彻底删除所有 Prisma 相关文件和引用
2. 移除所有 GORM model 中的外键定义（保留简单关联查询，删除约束）

### 步骤 A：删除 Prisma 残留

#### A1. 删除备份文件

```bash
# 删除 Prisma schema 备份（已无用）
rm -rf /Users/konghang/data/me/github/ember/backups/backup_20260210_173715/
```

#### A2. 清理 .gitignore

**文件**：`.gitignore`

移除第 37-43 行的 Prisma 相关配置：

```bash
# 改前（第 37-43 行）
# Prisma
*.db
*.db-journal
/app/generated/prisma

# prisma/migrations/ 应该被提交，作为数据库 schema 的版本控制

# 改后（删除整段）
```

#### A3. 更新文档引用

**文件 1**：`CLAUDE.md`（第 177 行）

```markdown
<!-- 改前 -->
数据库：PostgreSQL 15（使用 Prisma 管理 Schema）

<!-- 改后 -->
数据库：PostgreSQL 15（使用 GORM 管理 Schema）
```

**文件 2**：`README.md`（第 215 行）

```markdown
<!-- 改前 -->
数据库连接层（与 Prisma 兼容）

<!-- 改后 -->
数据库连接层（GORM ORM）
```

**文件 3**：`CHANGELOG.md`（第 39、83 行）

```markdown
<!-- 改前 -->
**数据库**: PostgreSQL 16 + Prisma ORM

<!-- 改后 -->
**数据库**: PostgreSQL 16 + GORM

<!-- 改前 -->
✅ SQL 注入防护（Prisma ORM）

<!-- 改后 -->
✅ SQL 注入防护（GORM 参数化查询）
```

**文件 4-7**：归档文档（可选删除或标注过期）

- `docs/MIGRATION-GUIDE.md` - 保留作为历史记录，或删除
- `docs/specs/archive/*.md` - 归档文件，建议保留不改

#### A4. 清理 CI/CD

**文件**：`.github/workflows/ci.yml`

检查是否有 Prisma 相关步骤（如 `prisma validate`、`prisma generate`），删除：

```yaml
# 改前（示例）
- name: Validate Prisma Schema
  run: npx prisma validate

# 改后（删除整个 step）
```

---

### 步骤 B：删除所有外键定义

#### 当前外键关系

```
invites.code ←─ users.inviteCode  (GORM foreignKey)
users.id     ←─ subscriptions.userId  (GORM foreignKey)
```

#### B1. User Model 移除外键

**文件**：`services/api/internal/models/user.go`

```go
// 改前（第 28-31 行）
	// 关联：注册时使用的邀请码（可选关系，邀请码可能被删除）
	Invite *Invite `json:"-" gorm:"foreignKey:InviteCode;references:Code"`

	// 关联：用户的订阅记录
	Subscriptions []Subscription `json:"-" gorm:"foreignKey:UserID"`

// 改后（完全删除）
	// （删除 Invite 关联）
	// （删除 Subscriptions 关联）
```

**说明**：
- 删除 `Invite` 字段 → 不再支持 `db.Preload("Invite")` 关联查询
- 删除 `Subscriptions` 字段 → 不再支持 `db.Preload("Subscriptions")` 关联查询
- 如需查询关联数据，改为手动 JOIN 或多次查询

#### B2. Subscription Model 移除外键

**文件**：`services/api/internal/models/subscription.go`

```go
// 改前（第 42 行）
	User *User `json:"-" gorm:"foreignKey:UserID"`

// 改后（删除）
	// （删除 User 关联）
```

#### B3. Invite Model 移除外键

**文件**：`services/api/internal/models/invite.go`

```go
// 改前（第 21 行）
	Users []User `json:"-" gorm:"foreignKey:InviteCode;references:Code"`

// 改后（删除）
	// （删除 Users 关联）
```

#### B4. 检查代码中的关联查询

搜索 `Preload` 使用：

```bash
grep -r "Preload" services/api/internal --include="*.go"
```

如果发现类似代码：

```go
// 改前（需要修改）
db.Preload("Invite").Find(&users)
db.Preload("Subscriptions").First(&user, userID)

// 改后（手动查询或 JOIN）
// 方案 1：手动查询
db.Find(&users)
var invite Invite
db.Where("code = ?", user.InviteCode).First(&invite)

// 方案 2：使用 JOIN（如果需要）
db.Table("users").
  Select("users.*, invites.code").
  Joins("LEFT JOIN invites ON users.invite_code = invites.code").
  Find(&users)
```

#### B5. 数据库迁移 SQL

如果数据库已存在外键约束（由 GORM AutoMigrate 或手动创建），需删除：

```sql
-- 删除 users 表的外键约束
ALTER TABLE users DROP CONSTRAINT IF EXISTS fk_invites_users;
ALTER TABLE users DROP CONSTRAINT IF EXISTS users_invite_code_fkey;

-- 删除 subscriptions 表的外键约束
ALTER TABLE subscriptions DROP CONSTRAINT IF EXISTS fk_users_subscriptions;
ALTER TABLE subscriptions DROP CONSTRAINT IF EXISTS subscriptions_user_id_fkey;

-- 验证外键是否全部删除
SELECT conname, conrelid::regclass, confrelid::regclass
FROM pg_constraint
WHERE contype = 'f';
-- 预期：无结果或只剩其他表的外键
```

---

### 步骤 C：验证清理结果

#### C1. 检查 Prisma 残留

```bash
# 搜索 "prisma" 关键词（忽略大小写）
grep -ri "prisma" . \
  --exclude-dir=node_modules \
  --exclude-dir=.git \
  --exclude-dir=backups \
  --exclude="*.md" \
  | grep -v "Binary file"

# 预期：无结果或仅在注释/归档文档中
```

#### C2. 检查外键定义

```bash
# 搜索 GORM foreignKey 标签
grep -r "foreignKey" services/api/internal/models --include="*.go"

# 预期：无结果
```

#### C3. 编译验证

```bash
cd services/api
go build ./...

# 预期：编译成功，无错误
```

#### C4. 数据库验证

```bash
# 连接数据库检查外键
psql $DATABASE_URL -c "
SELECT conname AS constraint_name,
       conrelid::regclass AS table_name,
       confrelid::regclass AS referenced_table
FROM pg_constraint
WHERE contype = 'f';
"

# 预期：无结果（所有外键已删除）
```

---

### 为什么删除外键？

**Linus 的观点**：数据库的职责是存储数据，不是强制业务规则。

**问题**：
1. `inviteCode` 是历史快照（记录用哪个码注册的），不是活跃关系
2. admin 用户没有 `inviteCode`，外键阻止创建
3. 文档自己说"邀请码删除不影响用户"，但外键会阻止删除邀请码
4. 外键增加数据库复杂度，应用层已有验证（注册时检查邀请码有效性）

**解决**：
- 删除外键约束
- 保留字段（`inviteCode`、`userId`）用于应用层关联
- 应用层负责数据一致性验证

---

## 变更总览

| 操作 | 文件 | 说明 |
|------|------|------|
| **统一认证** |||
| 删除 | `services/api/internal/models/admin.go` | 删除 Admin 模型 |
| 删除 | `services/web/src/views/admin/LoginView.vue` | 删除 admin 登录页 |
| 删除 | `services/web/src/views/user/LoginView.vue` | 删除 user 登录页 |
| 新建 | `services/web/src/views/LoginView.vue` | 统一登录页 |
| 修改 | `services/api/internal/models/user.go` | 新增 role/password，移除外键 |
| 修改 | `services/api/internal/db/db.go` | AutoMigrate 开关，seedDefaultAdmin |
| 修改 | `services/api/internal/services/auth.go` | 统一 Login，移除 AdminLogin/UserLogin |
| 修改 | `services/api/internal/handlers/auth.go` | 统一 Login/Logout handler |
| 修改 | `services/api/cmd/server/main.go` | 统一 /login 路由 |
| 修改 | `services/web/src/types/api.ts` | 删除 AdminInfo，UserInfo 新增 role |
| 修改 | `services/web/src/api/auth.ts` | 统一 login()，删除 userLogin() |
| 修改 | `services/web/src/store/auth.ts` | 统一 login action |
| 修改 | `services/web/src/router/index.ts` | 统一 /login 路由 |
| 修改 | `services/web/src/api/request.ts` | 简化 401 处理 |
| 修改 | `services/web/src/components/home/Navbar.vue` | 登录链接指向 /login |
| 修改 | `services/web/src/views/HomeView.vue` | 登录按钮文案 |
| **清理 Prisma** |||
| 删除 | `backups/backup_20260210_173715/` | 删除 Prisma schema 备份 |
| 修改 | `.gitignore` | 移除 Prisma 相关配置 |
| 修改 | `CLAUDE.md` | 更新数据库描述 |
| 修改 | `README.md` | 更新数据库描述 |
| 修改 | `CHANGELOG.md` | 更新 ORM 引用 |
| 修改 | `.github/workflows/ci.yml` | 删除 Prisma 验证步骤（如有） |
| **移除外键** |||
| 修改 | `services/api/internal/models/user.go` | 删除 Invite/Subscriptions 关联 |
| 修改 | `services/api/internal/models/subscription.go` | 删除 User 关联 |
| 修改 | `services/api/internal/models/invite.go` | 删除 Users 关联 |
| 执行 | SQL 迁移 | DROP CONSTRAINT 删除数据库外键 |

**净效果**：
- **统一认证**：删 3 文件，新建 1 文件，修改 12 文件
- **清理 Prisma**：删 1 目录，修改 5 文件
- **移除外键**：修改 3 models，执行 SQL
- **总计**：删除 4 项，新建 1 文件，修改 20 文件，代码总量净减少

---

## 验证步骤

### 1. 代码验证

```bash
# 后端编译
cd services/api && go build ./...

# 前端构建
cd services/web && npm run build

# 检查 Prisma 残留
grep -ri "prisma" . \
  --exclude-dir=node_modules \
  --exclude-dir=.git \
  --exclude-dir=backups \
  --exclude="*.md" | grep -v "Binary"
# 预期：无结果

# 检查 GORM 外键定义
grep -r "foreignKey" services/api/internal/models --include="*.go"
# 预期：无结果
```

### 2. 数据库验证

```bash
# 检查外键约束
psql $DATABASE_URL -c "
SELECT conname, conrelid::regclass, confrelid::regclass
FROM pg_constraint
WHERE contype = 'f';
"
# 预期：无结果或只剩无关外键

# 验证表结构
psql $DATABASE_URL -c "\d users"
# 预期：有 role、password 字段，inviteCode/email/embyId 可为 NULL
```

### 3. 功能测试

**统一认证**：
- `POST /api/v1/login`（admin 凭据）→ 返回 `role: "admin"`
- `POST /api/v1/login`（user 凭据）→ 返回 `role: "user"`
- `POST /api/v1/logout`（任意有效 token）→ 200
- `POST /api/v1/logout`（无 token）→ 401
- admin token 访问 `/api/v1/admin/*` → 200
- user token 访问 `/api/v1/admin/*` → 403

**前端**：
- `/login` 页面正常渲染
- admin 登录后跳转 `/admin/users`
- user 登录后跳转 `/user/dashboard`

**管理员初始化**：
- 首次启动输出 `✅ 默认管理员已创建：admin`
- 再次启动静默跳过（幂等）

**外键删除验证**：
- 删除邀请码后，相关用户记录不受影响
- 创建 admin 用户成功（`inviteCode` 为空）

---

## 上线指南：管理员初始化

### 生产环境部署流程

统一认证后，管理员账号通过 **环境变量自动初始化**，无需手动执行 SQL 脚本。

#### 1. 配置环境变量

在 `.env` 文件（或部署环境的环境变量配置）中添加：

```bash
# ===========================================
# 管理员初始化配置（首次部署必须配置）
# ===========================================

ADMIN_USERNAME=admin                      # 管理员用户名
ADMIN_PASSWORD=你的超强密码123!@#         # 管理员密码（必须修改）

# ===========================================
# 数据库迁移开关（首次部署或更新时开启）
# ===========================================

AUTO_MIGRATE=true                         # 启用自动迁移（新项目开启）

# ===========================================
# 其他必要配置
# ===========================================

DATABASE_URL=postgresql://...             # 数据库连接
JWT_SECRET=$(openssl rand -base64 32)     # JWT 密钥（32+ 字符）
```

**密码要求**：
- ✅ 最少 8 个字符（推荐 16+ 字符）
- ✅ 包含大小写字母、数字、特殊字符
- ❌ 禁止弱密码：`admin`、`123456`、`password`

**生成强密码命令**：
```bash
# 方法 1: 使用 openssl（16 字符）
openssl rand -base64 16

# 方法 2: 使用 pwgen（20 字符，包含特殊字符）
pwgen -s 20 1

# 方法 3: 使用 /dev/urandom（32 字符）
head /dev/urandom | tr -dc A-Za-z0-9 | head -c 32
```

#### 2. 启动应用

```bash
# Docker Compose 部署
docker compose up -d

# 查看日志确认初始化成功
docker compose logs api | grep -E "(管理员|admin)"

# 预期输出（成功）：
# ✅ 默认管理员已创建：admin

# 预期输出（已存在，跳过）：
# （无日志输出，静默跳过）

# 预期输出（未配置环境变量）：
# ⚠️  跳过 admin 初始化：ADMIN_USERNAME 或 ADMIN_PASSWORD 未设置
```

#### 3. 验证登录

```bash
# 测试管理员登录
curl -X POST http://localhost:8080/api/v1/login \
  -H "Content-Type: application/json" \
  -d '{
    "username": "admin",
    "password": "你配置的密码"
  }'

# 预期响应：
{
  "token": "eyJhbGciOiJIUzI1Ni...",
  "user": {
    "id": "clxxxxx",
    "username": "admin",
    "role": "admin",
    "isActive": true,
    "createdAt": "2026-02-12T..."
  }
}
```

#### 4. 前端登录测试

访问 `http://localhost:3000/login`（或生产域名），使用管理员账号登录：

- **用户名**：`admin`（或 `.env` 中配置的 `ADMIN_USERNAME`）
- **密码**：`.env` 中配置的 `ADMIN_PASSWORD`

登录成功后应跳转到 `/admin/users`。

---

### 工作原理

**自动初始化逻辑**（`services/api/internal/db/db.go`）：

```go
func seedDefaultAdmin() {
    // 1. 检查是否已存在 admin 用户（幂等）
    var count int64
    DB.Model(&models.User{}).Where("role = ?", "admin").Count(&count)
    if count > 0 {
        return  // 已存在，跳过
    }

    // 2. 读取环境变量
    username := os.Getenv("ADMIN_USERNAME")
    password := os.Getenv("ADMIN_PASSWORD")
    if username == "" || password == "" {
        log.Println("⚠️  跳过 admin 初始化：ADMIN_USERNAME 或 ADMIN_PASSWORD 未设置")
        return
    }

    // 3. 创建管理员用户
    admin := models.User{
        Username: username,
        Role:     "admin",
        IsActive: true,
    }
    admin.SetPassword(password)  // bcrypt 加密
    DB.Create(&admin)

    log.Printf("✅ 默认管理员已创建：%s", admin.Username)
}
```

**执行时机**：
- `InitDB()` → `AutoMigrate()` → `seedDefaultAdmin()` → 应用启动完成

**幂等保证**：
- 启动 N 次只会创建 1 次
- 已存在 admin 时，静默跳过
- 多实例部署也安全（数据库层 `uniqueIndex` 保护）

---

### 安全最佳实践

#### 部署前

```bash
# ✅ 使用强密码生成器
ADMIN_PASSWORD=$(openssl rand -base64 20)

# ✅ 将密码保存到安全的密码管理器
# 如：1Password、Bitwarden、LastPass

# ✅ 确保 .env 在 .gitignore 中
echo ".env" >> .gitignore
```

#### 部署后（可选）

```bash
# 移除 .env 中的 ADMIN_PASSWORD 行（防止泄露）
# 注意：移除后无法通过环境变量自动创建新 admin，但已有 admin 不受影响
sed -i '/ADMIN_PASSWORD/d' .env

# 或者设为空（效果相同）
sed -i 's/ADMIN_PASSWORD=.*/ADMIN_PASSWORD=/' .env
```

#### 定期审计

```bash
# 检查数据库中的 admin 用户
psql $DATABASE_URL -c "SELECT id, username, role, \"createdAt\" FROM users WHERE role='admin';"

# 预期输出：应该只有 1 个 admin 用户
```

---

### 常见问题

#### Q1: 首次启动未创建管理员怎么办？

**原因**：未配置 `ADMIN_USERNAME` 或 `ADMIN_PASSWORD`

**解决**：
```bash
# 1. 补充 .env 配置
echo "ADMIN_USERNAME=admin" >> .env
echo "ADMIN_PASSWORD=你的强密码" >> .env

# 2. 重启应用
docker compose restart api
```

#### Q2: 多次重启会重复创建吗？

**答**：不会。`seedDefaultAdmin()` 有幂等检查：

```go
if count > 0 {
    return  // 已存在，跳过
}
```

即使忘记移除 `ADMIN_PASSWORD` 也不会重复创建。

#### Q3: 如何创建第二个管理员？

**方式 1**：通过管理后台（推荐，待开发）

未来可在管理后台添加"创建管理员"功能。

**方式 2**：手动执行 SQL

```sql
INSERT INTO users (id, username, role, password, "isActive", "createdAt", "updatedAt")
VALUES (
  generate_cuid(),  -- 使用 CUID 生成函数
  'admin2',
  'admin',
  '$2a$10$...',  -- bcrypt 密码哈希（见下方生成方法）
  true,
  NOW(),
  NOW()
);
```

**生成密码哈希**：
```bash
# 使用 Go（推荐，与应用一致）
cd services/api
go run -e "package main; import (\"fmt\"; \"golang.org/x/crypto/bcrypt\"); func main() { h, _ := bcrypt.GenerateFromPassword([]byte(\"新密码\"), 10); fmt.Println(string(h)) }"

# 使用 Node.js（需要安装 bcryptjs）
node -e "console.log(require('bcryptjs').hashSync('新密码', 10))"

# 使用 Python（需要安装 bcrypt）
python3 -c "import bcrypt; print(bcrypt.hashpw(b'新密码', bcrypt.gensalt(10)).decode())"
```

#### Q4: 忘记管理员密码怎么办？

**解决方法**：通过数据库重置密码

```bash
# 1. 生成新密码的 bcrypt 哈希（使用上面的方法之一）
NEW_HASH=$(node -e "console.log(require('bcryptjs').hashSync('新密码123', 10))")

# 2. 更新数据库
psql $DATABASE_URL -c "UPDATE users SET password='$NEW_HASH' WHERE role='admin' AND username='admin'"

# 3. 使用新密码登录
```

#### Q5: 如何禁用自动迁移（生产环境）？

**原因**：生产环境应由 CI/CD 独立执行迁移，避免应用启动时的竞态条件。

**配置**：
```bash
# .env 文件
AUTO_MIGRATE=false  # 或完全不设置（默认 false）
```

应用日志输出：
```
ℹ️  AUTO_MIGRATE 未启用，跳过数据库迁移
```

---

### 生产环境检查清单

部署前确认：

- [ ] `ADMIN_USERNAME` 已配置（推荐保持 `admin`）
- [ ] `ADMIN_PASSWORD` 已设为强密码（16+ 字符）
- [ ] `AUTO_MIGRATE=true`（首次部署时）
- [ ] `JWT_SECRET` 已配置（32+ 字符）
- [ ] `DATABASE_URL` 已配置并测试连通性
- [ ] `.env` 文件在 `.gitignore` 中
- [ ] 密码已保存到安全的密码管理器

部署后验证：

- [ ] 查看日志确认 admin 创建成功
- [ ] 使用配置的密码成功登录
- [ ] 登录后跳转到 `/admin/users`
- [ ] 测试 admin token 可访问 `/api/v1/admin/*` 路由
- [ ] （可选）从 `.env` 移除 `ADMIN_PASSWORD` 行

定期检查：

- [ ] 每季度审计 admin 用户数量（应为 1 个）
- [ ] 每半年更换 admin 密码
- [ ] 监控异常登录尝试（待开发日志功能）

---

## 附：环境变量完整清单

统一认证后的必要环境变量：

```bash
# =================== 数据库 ===================
DATABASE_URL=postgresql://user:pass@host:port/db

# =================== JWT ===================
JWT_SECRET=至少32个字符的随机字符串

# =============== 管理员初始化 ===============
ADMIN_USERNAME=admin                    # 首次部署时配置
ADMIN_PASSWORD=你的超强密码             # 首次部署时配置

# =============== 数据库迁移开关 ===============
AUTO_MIGRATE=true                       # 开发/首次部署=true，生产=false

# =================== Go API ===================
PORT=8080                               # API 端口（可选，默认 8080）

# ================= 其他配置 =================
# Emby、TMDB、MoviePilot 等配置保持不变
```

---

## Docker 环境变量传递方式

### 兼容性说明

当前代码使用 `os.Getenv()` 读取所有配置，**完全兼容**以下方式：

1. ✅ Docker Compose `environment` 字段
2. ✅ Docker Compose `env_file` 引用
3. ✅ `docker run -e` 命令行参数
4. ✅ Kubernetes ConfigMap/Secret
5. ✅ Docker Swarm Secrets
6. ✅ 本地 .env 文件（开发环境回退）

**优先级**：Docker 传入的环境变量 > .env 文件

### 方式 1：docker-compose.yml + env_file（推荐）

```yaml
# infrastructure/docker/docker-compose.yml
services:
  api:
    build:
      context: ../../services/api
    env_file:
      - .env  # 自动读取 .env 文件中的所有变量
    environment:
      # 也可以覆盖或补充特定变量
      - PORT=8080
      - DATABASE_URL=postgresql://postgres:password@postgres:5432/ember
```

```bash
# .env 文件（同目录）
ADMIN_USERNAME=admin
ADMIN_PASSWORD=你的超强密码
AUTO_MIGRATE=true
JWT_SECRET=超长随机字符串
```

### 方式 2：docker-compose.yml 直接配置

```yaml
services:
  api:
    environment:
      - PORT=8080
      - DATABASE_URL=postgresql://postgres:password@postgres:5432/ember
      - JWT_SECRET=${JWT_SECRET}                      # 从宿主机环境变量读取
      - ADMIN_USERNAME=${ADMIN_USERNAME:-admin}       # 默认值 admin
      - ADMIN_PASSWORD=${ADMIN_PASSWORD}              # 从宿主机环境变量读取
      - AUTO_MIGRATE=${AUTO_MIGRATE:-false}           # 默认值 false
      - EMBY_URL=${EMBY_URL}
      - EMBY_API_KEY=${EMBY_API_KEY}
```

### 方式 3：docker run 命令

```bash
docker run -d \
  --name ember-api \
  -p 8080:8080 \
  -e PORT=8080 \
  -e DATABASE_URL="postgresql://..." \
  -e JWT_SECRET="超长随机字符串" \
  -e ADMIN_USERNAME="admin" \
  -e ADMIN_PASSWORD="你的超强密码" \
  -e AUTO_MIGRATE="true" \
  -e EMBY_URL="https://emby.example.com" \
  -e EMBY_API_KEY="your-api-key" \
  ghcr.io/your-org/ember-api:latest
```

### 方式 4：Kubernetes（生产推荐）

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: ember-secrets
type: Opaque
stringData:
  admin-password: "你的超强密码"
  jwt-secret: "超长随机字符串"
  database-url: "postgresql://..."

---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: ember-api
spec:
  template:
    spec:
      containers:
      - name: api
        image: ghcr.io/your-org/ember-api:latest
        env:
        - name: ADMIN_USERNAME
          value: "admin"
        - name: ADMIN_PASSWORD
          valueFrom:
            secretKeyRef:
              name: ember-secrets
              key: admin-password
        - name: AUTO_MIGRATE
          value: "false"
        - name: JWT_SECRET
          valueFrom:
            secretKeyRef:
              name: ember-secrets
              key: jwt-secret
        - name: DATABASE_URL
          valueFrom:
            secretKeyRef:
              name: ember-secrets
              key: database-url
```

### godotenv 工作原理

```go
// db.go 中的加载逻辑
if err := godotenv.Load(".env"); err == nil {
    // .env 文件存在，但只加载「未设置」的变量
    // Docker 已传入的变量不会被覆盖
}

// 读取环境变量（优先使用 Docker 传入的值）
dsn := os.Getenv("DATABASE_URL")
```

**实际效果示例**：

```bash
# Docker 传入 + .env 文件同时存在
docker run -e ADMIN_PASSWORD=docker密码 ...

# .env 文件中
ADMIN_PASSWORD=env文件密码
JWT_SECRET=env文件密钥

# 结果
os.Getenv("ADMIN_PASSWORD") → "docker密码"     （Docker 优先）
os.Getenv("JWT_SECRET")     → "env文件密钥"    （Docker 未传，使用 .env）
```

### 建议的 docker-compose.yml 配置

更新 `infrastructure/docker/docker-compose.yml`：

```yaml
services:
  api:
    build:
      context: ../../services/api
      dockerfile: Dockerfile
    container_name: ember-api
    env_file:
      - .env  # 从 .env 文件读取所有变量（推荐）
    environment:
      # 可选：覆盖或补充特定变量
      - PORT=8080
      - DATABASE_URL=postgresql://postgres:password@postgres:5432/ember?search_path=public
    ports:
      - "8080:8080"
    depends_on:
      postgres:
        condition: service_healthy
    networks:
      - ember-network
    restart: unless-stopped
```

配套 `.env` 文件：

```bash
# ========== 管理员初始化 ==========
ADMIN_USERNAME=admin
ADMIN_PASSWORD=$(openssl rand -base64 20)

# ========== 数据库迁移 ==========
AUTO_MIGRATE=true

# ========== JWT ==========
JWT_SECRET=$(openssl rand -base64 32)

# ========== Emby ==========
EMBY_URL=https://your-emby-server.com
EMBY_API_KEY=your-emby-api-key

# ========== TMDB（可选）==========
TMDB_API_KEY=your-tmdb-api-key

# ========== MoviePilot（可选）==========
MOVIEPILOT_URL=http://moviepilot:3001
MOVIEPILOT_USERNAME=admin
MOVIEPILOT_PASSWORD=your-mp-password
```
