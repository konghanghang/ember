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

## 变更总览

| 操作 | 文件 |
|------|------|
| 删除 | `services/api/internal/models/admin.go` |
| 删除 | `services/web/src/views/admin/LoginView.vue` |
| 删除 | `services/web/src/views/user/LoginView.vue` |
| 新建 | `services/web/src/views/LoginView.vue` |
| 修改 | `services/api/internal/models/user.go` |
| 修改 | `services/api/internal/db/db.go` |
| 修改 | `services/api/internal/services/auth.go` |
| 修改 | `services/api/internal/handlers/auth.go` |
| 修改 | `services/api/cmd/server/main.go` |
| 修改 | `services/web/src/types/api.ts` |
| 修改 | `services/web/src/api/auth.ts` |
| 修改 | `services/web/src/store/auth.ts` |
| 修改 | `services/web/src/router/index.ts` |
| 修改 | `services/web/src/api/request.ts` |
| 修改 | `services/web/src/components/home/Navbar.vue` |
| 修改 | `services/web/src/views/HomeView.vue` |

**净效果**：删 3 文件，新建 1 文件，修改 12 文件。代码总量净减少。

---

## 验证步骤

1. 后端编译：`cd services/api && go build ./...`
2. 前端构建：`cd services/web && npm run build`
3. 手动测试：
   - `POST /api/v1/login`（admin 凭据）→ 返回 `role: "admin"`
   - `POST /api/v1/login`（user 凭据）→ 返回 `role: "user"`
   - `POST /api/v1/logout`（任意有效 token）→ 200
   - `POST /api/v1/logout`（无 token）→ 401
   - admin token 访问 `/api/v1/admin/*` → 200
   - user token 访问 `/api/v1/admin/*` → 403
   - 前端 `/login` 页面正常渲染
   - admin 登录后跳转 `/admin/users`
   - user 登录后跳转 `/user/dashboard`
