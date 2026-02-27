package handlers

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/konghang/ember/backend/internal/models"
	"github.com/konghang/ember/backend/internal/services"
)

// AuthHandler 认证处理器
type AuthHandler struct {
	authService  *services.AuthService
	emailService *services.EmailService
}

// NewAuthHandler 创建认证处理器
func NewAuthHandler() *AuthHandler {
	return &AuthHandler{
		authService:  services.NewAuthService(),
		emailService: services.NewEmailService(),
	}
}

// SendEmailCodeRequest 发送邮箱验证码请求
type SendEmailCodeRequest struct {
	Email string `json:"email" binding:"required,email"`
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

// SendEmailCode 发送邮箱验证码
// @Summary 发送邮箱验证码
// @Tags 认证
// @Accept json
// @Produce json
// @Param body body SendEmailCodeRequest true "邮箱信息"
// @Success 200 {object} SuccessResponse
// @Failure 400 {object} ErrorResponse
// @Failure 429 {object} ErrorResponse
// @Router /api/v1/register/send-code [post]
func (h *AuthHandler) SendEmailCode(c *gin.Context) {
	var req SendEmailCodeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请提供有效的邮箱地址"})
		return
	}

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

// ErrorResponse 错误响应结构
type ErrorResponse struct {
	Error string `json:"error"`
}

// SuccessResponse 成功响应结构
type SuccessResponse struct {
	Message string `json:"message"`
}
