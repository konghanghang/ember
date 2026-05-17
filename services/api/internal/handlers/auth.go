package handlers

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/konghang/ember/backend/internal/common/httpx"
	configpkg "github.com/konghang/ember/backend/internal/config"
	"github.com/konghang/ember/backend/internal/models"
	authpkg "github.com/konghang/ember/backend/internal/services/auth"
	emailpkg "github.com/konghang/ember/backend/internal/services/email"
	redemptionpkg "github.com/konghang/ember/backend/internal/services/redemption"
	userpkg "github.com/konghang/ember/backend/internal/services/user"
)

// AuthHandler 认证处理器
type AuthHandler struct {
	authService  *authpkg.AuthService
	emailService *emailpkg.EmailService
	userService  *userpkg.UserService
}

// NewAuthHandler 创建认证处理器
func NewAuthHandler() *AuthHandler {
	emailService := emailpkg.NewEmailService()
	return &AuthHandler{
		authService:  authpkg.NewAuthService(),
		emailService: emailService,
		userService:  userpkg.NewUserServiceWithEmailVerifier(emailService),
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
// @Param body body auth.LoginRequest true "登录信息"
// @Success 200 {object} auth.LoginResponse
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Router /api/v1/login [post]
func (h *AuthHandler) Login(c *gin.Context) {
	var req authpkg.LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "请求参数错误",
		})
		return
	}

	req.ClientIP = c.ClientIP()
	req.RequestContext = c.Request.Context()

	resp, err := h.authService.Login(&req)
	if err != nil {
		switch {
		case errors.Is(err, authpkg.ErrTurnstileValidationFailed):
			c.JSON(http.StatusBadRequest, gin.H{
				"error": err.Error(),
			})
		case isAuthLoginUnauthorizedError(err):
			c.JSON(http.StatusUnauthorized, gin.H{
				"error": err.Error(),
			})
		default:
			httpx.InternalError(c, err)
		}
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

	user, err := h.userService.GetProfile(userID.(string))
	if err != nil {
		if errors.Is(err, userpkg.ErrUserNotFound) {
			c.JSON(http.StatusUnauthorized, gin.H{
				"error": err.Error(),
			})
			return
		}
		httpx.InternalError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"user": user,
	})
}

// ListAdminEmbyUsers 获取管理员可绑定的 Emby 用户候选列表
// @Summary 获取 Emby 用户候选列表
// @Tags 认证
// @Produce json
// @Success 200 {object} auth.ListAdminEmbyUsersResponse
// @Failure 502 {object} ErrorResponse
// @Router /api/v1/admin/emby-users [get]
// @Security BearerAuth
func (h *AuthHandler) ListAdminEmbyUsers(c *gin.Context) {
	userID, _ := c.Get("userID")

	resp, err := h.authService.ListAdminEmbyUsers(userID.(string), authpkg.ListAdminEmbyUsersRequest{
		Query: c.Query("query"),
		Limit: parsePositiveIntQuery(c, "limit"),
	})
	if err != nil {
		if errors.Is(err, authpkg.ErrEmbyUserSearchQueryRequired) {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		if errors.Is(err, authpkg.ErrEmbyServiceUnavailable) {
			c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
			return
		}
		httpx.InternalError(c, err)
		return
	}

	c.JSON(http.StatusOK, resp)
}

func parsePositiveIntQuery(c *gin.Context, key string) int {
	value := strings.TrimSpace(c.Query(key))
	if value == "" {
		return 0
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed <= 0 {
		return 0
	}
	return parsed
}

// BindEmbyAccount 绑定 Emby 账号到当前管理员
// @Summary 管理员关联 Emby 账号
// @Tags 认证
// @Accept json
// @Produce json
// @Param body body auth.BindEmbyAccountRequest true "Emby 用户 ID"
// @Success 200 {object} auth.BindEmbyAccountResponse
// @Failure 400 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 409 {object} ErrorResponse
// @Failure 502 {object} ErrorResponse
// @Router /api/v1/admin/current/emby-binding [put]
// @Security BearerAuth
func (h *AuthHandler) BindEmbyAccount(c *gin.Context) {
	userID, _ := c.Get("userID")

	var req authpkg.BindEmbyAccountRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数错误"})
		return
	}

	resp, err := h.authService.BindEmbyAccount(userID.(string), &req)
	if err != nil {
		switch {
		case errors.Is(err, authpkg.ErrEmbyBindingTargetRequired):
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		case errors.Is(err, authpkg.ErrEmbyBindingUserNotFound):
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		case errors.Is(err, authpkg.ErrEmbyAlreadyBound):
			c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
		case authpkg.IsEmbyUserOccupied(err):
			c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
		case errors.Is(err, authpkg.ErrEmbyServiceUnavailable):
			c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		default:
			httpx.InternalError(c, err)
		}
		return
	}

	c.JSON(http.StatusOK, resp)
}

// UnbindEmbyAccount 解除当前管理员的 Emby 关联
// @Summary 管理员解除 Emby 关联
// @Tags 认证
// @Produce json
// @Success 200 {object} SuccessResponse
// @Router /api/v1/admin/current/emby-binding [delete]
// @Security BearerAuth
func (h *AuthHandler) UnbindEmbyAccount(c *gin.Context) {
	userID, _ := c.Get("userID")

	if err := h.authService.UnbindEmbyAccount(userID.(string)); err != nil {
		httpx.InternalError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "解除绑定成功"})
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
// @Param body body auth.RegisterUserRequest true "注册信息"
// @Success 200 {object} auth.RegisterUserResponse
// @Failure 400 {object} ErrorResponse
// @Router /api/v1/user/register [post]
func (h *AuthHandler) RegisterUser(c *gin.Context) {
	var req authpkg.RegisterUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "请求参数错误",
		})
		return
	}

	resp, err := h.authService.RegisterUser(&req)
	if err != nil {
		if isAuthRegisterBadRequest(err) {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": err.Error(),
			})
			return
		}
		httpx.InternalError(c, err)
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
		switch {
		case errors.Is(err, emailpkg.ErrEmailAlreadyRegistered),
			errors.Is(err, emailpkg.ErrEmailDomainNotAllowed):
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		case errors.Is(err, emailpkg.ErrEmailCodeRateLimit), errors.Is(err, emailpkg.ErrEmailCodeIPRateLimit):
			c.JSON(http.StatusTooManyRequests, gin.H{"error": err.Error()})
		default:
			httpx.InternalError(c, err)
		}
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "验证码已发送"})
}

// SendResetCode 发送密码重置验证码
//
// 反账号枚举：除请求格式错误（400）和 SMTP 未配置（503，运维侧问题）外，所有内部错误
// （未注册邮箱 / 限流 / 发送失败）一律折叠为 200 + 与已注册一致的统一文案。
// 攻击者无法通过状态码、文案或限流差异（200 vs 429）枚举站内邮箱。
// 内部仍记录 emailHash 前 8 位 + IP + 错误细节用于排障。
func (h *AuthHandler) SendResetCode(c *gin.Context) {
	var req SendEmailCodeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请提供有效的邮箱地址"})
		return
	}

	clientIP := c.ClientIP()
	const uniformMessage = "如果该邮箱已注册，验证码已发送"

	err := h.emailService.SendVerificationCode(req.Email, clientIP, models.VerificationTypeReset)

	if shouldExposeResetCodeSendError(err) {
		// SMTP 未配置是运维侧问题，向调用方明确返回，便于配置中心 / 测试链路定位
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": err.Error()})
		return
	}

	if err != nil {
		log.Printf("[Auth] 重置码请求内部异常 emailHash=%s ip=%s err=%v",
			hashEmailForLog(req.Email), clientIP, err)
	}

	c.JSON(http.StatusOK, gin.H{"message": uniformMessage})
}

// hashEmailForLog 对 email 做不可逆哈希，仅取前 8 位 hex 用于日志关联，避免明文落盘。
// 输入先 lower+trim 归一化，与 DB lower(email) 查询及 service 层 hashEmail 口径对齐，
// 保证同一邮箱在 send-code、UpdateEmail commit、notify-old-email 三处日志中 hash 一致。
func hashEmailForLog(email string) string {
	normalized := strings.ToLower(strings.TrimSpace(email))
	sum := sha256.Sum256([]byte(normalized))
	return hex.EncodeToString(sum[:])[:8]
}

func shouldExposeResetCodeSendError(err error) bool {
	return errors.Is(err, emailpkg.ErrEmailNotConfigured)
}

func isAuthLoginUnauthorizedError(err error) bool {
	return errors.Is(err, authpkg.ErrAuthInvalidCredentials)
}

func isAuthRegisterBadRequest(err error) bool {
	if err == nil {
		return false
	}

	switch {
	case errors.Is(err, emailpkg.ErrEmailCodeInvalid),
		errors.Is(err, authpkg.ErrRegisterEmailCodeRequired),
		errors.Is(err, authpkg.ErrRegisterInviteCodeRequired),
		errors.Is(err, redemptionpkg.ErrRedemptionCodeNotFound),
		errors.Is(err, redemptionpkg.ErrRedemptionCodeInvalid),
		errors.Is(err, userpkg.ErrUsernameLengthInvalid),
		errors.Is(err, userpkg.ErrUsernameCharsetInvalid),
		errors.Is(err, userpkg.ErrUsernameAlreadyExists),
		errors.Is(err, emailpkg.ErrEmailAlreadyRegistered),
		errors.Is(err, configpkg.ErrRegistrationEmailDomainNotAllowed),
		errors.Is(err, configpkg.ErrRegistrationEmailInvalid),
		errors.Is(err, redemptionpkg.ErrRegistrationPlanGroupNotFound):
		return true
	}
	return false
}

// ResetPasswordByCode 通过验证码重置密码
func (h *AuthHandler) ResetPasswordByCode(c *gin.Context) {
	var req userpkg.ResetPasswordByCodeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数错误"})
		return
	}

	if err := h.userService.ResetPasswordByCode(&req); err != nil {
		if errors.Is(err, emailpkg.ErrEmailCodeInvalid) {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		httpx.InternalError(c, err)
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
