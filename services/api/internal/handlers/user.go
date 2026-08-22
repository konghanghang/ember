package handlers

import (
	"errors"
	"log"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/konghang/ember/backend/internal/common/httpx"
	configpkg "github.com/konghang/ember/backend/internal/config"
	emailpkg "github.com/konghang/ember/backend/internal/services/email"
	paymentpkg "github.com/konghang/ember/backend/internal/services/payment"
	redemptionpkg "github.com/konghang/ember/backend/internal/services/redemption"
	userpkg "github.com/konghang/ember/backend/internal/services/user"
)

// UserHandler 用户处理器
type UserHandler struct {
	userService           *userpkg.UserService
	emailService          *emailpkg.EmailService
	redemptionService     *redemptionpkg.RedemptionService
	redemptionCodeService *redemptionpkg.RedemptionCodeService
}

// NewUserHandler 创建用户处理器
func NewUserHandler() *UserHandler {
	emailService := emailpkg.NewEmailService()
	return &UserHandler{
		userService:           userpkg.NewUserServiceWithEmailVerifier(emailService),
		emailService:          emailService,
		redemptionService:     &redemptionpkg.RedemptionService{},
		redemptionCodeService: &redemptionpkg.RedemptionCodeService{},
	}
}

// CreateUserByAdmin 管理员创建用户
// @Summary 管理员创建用户
// @Tags 用户管理
// @Accept json
// @Produce json
// @Param body body user.AdminCreateUserRequest true "创建用户请求"
// @Success 200 {object} user.UserView
// @Router /api/v1/admin/users [post]
// @Security BearerAuth
func (h *UserHandler) CreateUserByAdmin(c *gin.Context) {
	var req userpkg.AdminCreateUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "请求参数错误",
		})
		return
	}

	user, err := h.userService.CreateUserByAdmin(&req)
	if err != nil {
		switch {
		case errors.Is(err, userpkg.ErrUsernameLengthInvalid),
			errors.Is(err, userpkg.ErrUsernameCharsetInvalid),
			errors.Is(err, userpkg.ErrEmailRequired),
			errors.Is(err, userpkg.ErrEmailInvalid),
			errors.Is(err, userpkg.ErrPasswordTooShort),
			errors.Is(err, userpkg.ErrNeverExpireConflict),
			errors.Is(err, userpkg.ErrExpiresAtRequired),
			errors.Is(err, userpkg.ErrExpiresAtFormatInvalid),
			errors.Is(err, userpkg.ErrUsernameAlreadyExists),
			errors.Is(err, userpkg.ErrEmailAlreadyExists):
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		case errors.Is(err, userpkg.ErrInvalidPlanGroup), errors.Is(err, paymentpkg.ErrPlanGroupNotFound):
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		default:
			httpx.InternalError(c, err)
		}
		return
	}

	c.JSON(http.StatusOK, user)
}

// GetUsers 获取用户列表
// @Summary 获取用户列表
// @Tags 用户管理
// @Produce json
// @Param page query int false "页码" default(1)
// @Param pageSize query int false "每页数量" default(20)
// @Param search query string false "搜索关键词"
// @Param isActive query bool false "是否启用"
// @Param expiresAfter query string false "到期时间晚于该日期（YYYY-MM-DD）"
// @Param embyStatus query string false "Emby 状态（available/disabled/unlinked）"
// @Success 200 {object} user.GetUsersResponse
// @Router /api/v1/admin/users [get]
// @Security BearerAuth
func (h *UserHandler) GetUsers(c *gin.Context) {
	var req userpkg.GetUsersRequest

	if err := c.ShouldBindQuery(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "请求参数错误",
		})
		return
	}

	resp, err := h.userService.GetUsers(&req)
	if err != nil {
		if errors.Is(err, userpkg.ErrInvalidExpiresAfter) || errors.Is(err, userpkg.ErrInvalidEmbyStatus) || errors.Is(err, userpkg.ErrInvalidPlanGroup) || errors.Is(err, paymentpkg.ErrDefaultPlanGroupNotFound) {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		httpx.InternalError(c, err)
		return
	}

	c.JSON(http.StatusOK, resp)
}

// GetUserByID 获取用户详情
// @Summary 获取用户详情
// @Tags 用户管理
// @Produce json
// @Param id path string true "用户ID"
// @Success 200 {object} models.User
// @Router /api/v1/admin/users/{id} [get]
// @Security BearerAuth
func (h *UserHandler) GetUserByID(c *gin.Context) {
	userID := c.Param("id")

	user, err := h.userService.GetUserByID(userID)
	if err != nil {
		switch {
		case errors.Is(err, userpkg.ErrUserNotFound):
			c.JSON(http.StatusNotFound, gin.H{
				"error": err.Error(),
			})
		case errors.Is(err, paymentpkg.ErrDefaultPlanGroupNotFound):
			c.JSON(http.StatusBadRequest, gin.H{
				"error": err.Error(),
			})
		default:
			httpx.InternalError(c, err)
		}
		return
	}

	c.JSON(http.StatusOK, user)
}

// UpdateUserByAdmin 管理员更新用户信息
// @Summary 管理员更新用户信息
// @Tags 用户管理
// @Accept json
// @Produce json
// @Param id path string true "用户ID"
// @Param body body user.AdminUpdateUserRequest true "可更新字段"
// @Success 200 {object} user.UserView
// @Router /api/v1/admin/users/{id} [put]
// @Security BearerAuth
func (h *UserHandler) UpdateUserByAdmin(c *gin.Context) {
	userID := c.Param("id")

	var req userpkg.AdminUpdateUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "请求参数错误",
		})
		return
	}

	operatorID := ""
	if current := currentUserIDPtr(c); current != nil {
		operatorID = *current
	}
	user, err := h.userService.UpdateUserByAdminWithContext(c.Request.Context(), userID, &req, operatorID)
	if err != nil {
		switch {
		case errors.Is(err, userpkg.ErrUserNotFound):
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		case errors.Is(err, userpkg.ErrUpdateFieldsRequired),
			errors.Is(err, userpkg.ErrClearExpiresAtConflict),
			errors.Is(err, userpkg.ErrEmailRequired),
			errors.Is(err, userpkg.ErrEmailInvalid),
			errors.Is(err, userpkg.ErrExpiresAtFormatInvalid),
			errors.Is(err, userpkg.ErrEmailAlreadyExists):
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		case errors.Is(err, userpkg.ErrInvalidPlanGroup), errors.Is(err, paymentpkg.ErrPlanGroupNotFound), errors.Is(err, paymentpkg.ErrDefaultPlanGroupNotFound):
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		default:
			httpx.InternalError(c, err)
		}
		return
	}

	c.JSON(http.StatusOK, user)
}

// ExtendExpiry 延长用户到期时间
// @Summary 延长用户到期时间
// @Tags 用户管理
// @Accept json
// @Produce json
// @Param id path string true "用户ID"
// @Param body body user.ExtendExpiryRequest true "延长天数"
// @Success 200 {object} user.UserView
// @Router /api/v1/admin/users/{id}/extend [put]
// @Security BearerAuth
func (h *UserHandler) ExtendExpiry(c *gin.Context) {
	userID := c.Param("id")

	var req userpkg.ExtendExpiryRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "请求参数错误",
		})
		return
	}

	user, err := h.userService.ExtendExpiry(userID, req.Days)
	if err != nil {
		if errors.Is(err, userpkg.ErrUserNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
			return
		}
		httpx.InternalError(c, err)
		return
	}

	c.JSON(http.StatusOK, user)
}

// ToggleUserStatus 启用/禁用用户
// @Summary 启用/禁用用户
// @Tags 用户管理
// @Produce json
// @Param id path string true "用户ID"
// @Success 200 {object} user.UserView
// @Router /api/v1/admin/users/{id}/toggle [put]
// @Security BearerAuth
func (h *UserHandler) ToggleUserStatus(c *gin.Context) {
	userID := c.Param("id")
	operatorID := ""
	if current := currentUserIDPtr(c); current != nil {
		operatorID = *current
	}

	user, err := h.userService.ToggleUserStatusWithContext(c.Request.Context(), userID, operatorID)
	if err != nil {
		if errors.Is(err, userpkg.ErrUserNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
			return
		}
		httpx.InternalError(c, err)
		return
	}

	c.JSON(http.StatusOK, user)
}

// DeleteUser 删除用户
// @Summary 删除用户
// @Tags 用户管理
// @Produce json
// @Param id path string true "用户ID"
// @Success 200 {object} SuccessResponse
// @Router /api/v1/admin/users/{id} [delete]
// @Security BearerAuth
func (h *UserHandler) DeleteUser(c *gin.Context) {
	userID := c.Param("id")
	operatorID := ""
	if current := currentUserIDPtr(c); current != nil {
		operatorID = *current
	}

	err := h.userService.DeleteUserWithContext(c.Request.Context(), userID, operatorID)
	if err != nil {
		if errors.Is(err, userpkg.ErrUserNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
			return
		}
		httpx.InternalError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "删除成功",
	})
}

// ResetPassword 重置用户密码
// @Summary 重置用户密码
// @Tags 用户管理
// @Accept json
// @Produce json
// @Param id path string true "用户ID"
// @Param body body user.ResetPasswordRequest true "新密码"
// @Success 200 {object} SuccessResponse
// @Router /api/v1/admin/users/{id}/reset-password [put]
// @Security BearerAuth
func (h *UserHandler) ResetPassword(c *gin.Context) {
	userID := c.Param("id")

	var req userpkg.ResetPasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "请求参数错误",
		})
		return
	}

	err := h.userService.ResetPassword(userID, req.NewPassword)
	if err != nil {
		if errors.Is(err, userpkg.ErrUserNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
			return
		}
		httpx.InternalError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "密码重置成功",
	})
}

// ==================== 用户面板 API ====================

// GetProfile 获取个人信息
// @Summary 获取个人信息
// @Tags 用户面板
// @Produce json
// @Success 200 {object} models.User
// @Router /api/v1/user/profile [get]
// @Security BearerAuth
func (h *UserHandler) GetProfile(c *gin.Context) {
	userID, _ := c.Get("userID")

	user, err := h.userService.GetProfile(userID.(string))
	if err != nil {
		if errors.Is(err, userpkg.ErrUserNotFound) {
			c.JSON(http.StatusNotFound, gin.H{
				"error": err.Error(),
			})
			return
		}
		httpx.InternalError(c, err)
		return
	}

	c.JSON(http.StatusOK, user)
}

// UpdatePassword 修改密码
// @Summary 修改密码
// @Tags 用户面板
// @Accept json
// @Produce json
// @Param body body user.UpdatePasswordRequest true "密码信息"
// @Success 200 {object} SuccessResponse
// @Router /api/v1/user/password [put]
// @Security BearerAuth
func (h *UserHandler) UpdatePassword(c *gin.Context) {
	userID, _ := c.Get("userID")

	var req userpkg.UpdatePasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "请求参数错误",
		})
		return
	}

	err := h.userService.UpdatePassword(userID.(string), &req)
	if err != nil {
		switch {
		case errors.Is(err, userpkg.ErrOldPasswordInvalid),
			errors.Is(err, userpkg.ErrUserEmbyIDRequired):
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		default:
			httpx.InternalError(c, err)
		}
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "密码修改成功",
	})
}

// SendEmailChangeCodeRequest 发送邮箱变更验证码请求
type SendEmailChangeCodeRequest struct {
	NewEmail string `json:"newEmail" binding:"required,email"`
}

// SendEmailChangeCode 发送邮箱变更验证码到新邮箱
// @Summary 发送邮箱变更验证码
// @Tags 用户面板
// @Accept json
// @Produce json
// @Param body body handlers.SendEmailChangeCodeRequest true "新邮箱"
// @Success 200 {object} SuccessResponse
// @Router /api/v1/user/email/send-code [post]
// @Security BearerAuth
func (h *UserHandler) SendEmailChangeCode(c *gin.Context) {
	userIDValue, _ := c.Get("userID")
	userID, _ := userIDValue.(string)

	var req SendEmailChangeCodeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请提供有效的邮箱地址"})
		return
	}

	clientIP := c.ClientIP()
	newEmail := strings.TrimSpace(req.NewEmail)

	currentUser, err := h.userService.GetUserByID(userID)
	if err != nil {
		if errors.Is(err, userpkg.ErrUserNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
			return
		}
		httpx.InternalError(c, err)
		return
	}

	log.Printf("[UserEmailChange] step=send-code userId=%s newHash=%s ip=%s",
		userID, hashEmailForLog(newEmail), clientIP)

	if err := h.emailService.SendEmailChangeCode(currentUser.Email, newEmail, clientIP); err != nil {
		switch {
		case errors.Is(err, emailpkg.ErrEmailUnchanged),
			errors.Is(err, emailpkg.ErrEmailAlreadyBound),
			errors.Is(err, emailpkg.ErrEmailAlreadyRegistered),
			errors.Is(err, emailpkg.ErrEmailDomainNotAllowed):
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		case errors.Is(err, emailpkg.ErrEmailCodeRateLimit),
			errors.Is(err, emailpkg.ErrEmailCodeIPRateLimit):
			c.JSON(http.StatusTooManyRequests, gin.H{"error": err.Error()})
		case errors.Is(err, emailpkg.ErrEmailNotConfigured):
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": err.Error()})
		default:
			log.Printf("[UserEmailChange] send-code failed userId=%s newHash=%s ip=%s err=%v",
				userID, hashEmailForLog(newEmail), clientIP, err)
			httpx.InternalError(c, err)
		}
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "验证码已发送至新邮箱"})
}

// UpdateEmail 修改邮箱
// @Summary 修改邮箱
// @Tags 用户面板
// @Accept json
// @Produce json
// @Param body body user.UpdateEmailRequest true "邮箱信息"
// @Success 200 {object} models.User
// @Router /api/v1/user/email [put]
// @Security BearerAuth
func (h *UserHandler) UpdateEmail(c *gin.Context) {
	userIDValue, _ := c.Get("userID")
	userID, _ := userIDValue.(string)

	var req userpkg.UpdateEmailRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "请求参数错误",
		})
		return
	}

	result, err := h.userService.UpdateEmail(userID, &req)
	if err != nil {
		switch {
		case errors.Is(err, userpkg.ErrEmailAlreadyExists),
			errors.Is(err, emailpkg.ErrEmailUnchanged),
			errors.Is(err, emailpkg.ErrEmailCodeInvalid),
			errors.Is(err, configpkg.ErrRegistrationEmailDomainNotAllowed),
			errors.Is(err, configpkg.ErrRegistrationEmailInvalid):
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		case errors.Is(err, userpkg.ErrUserNotFound):
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		default:
			httpx.InternalError(c, err)
		}
		return
	}

	if result.OldEmail != "" {
		newEmail := result.User.Email
		oldEmail := result.OldEmail
		go func(oldEmail, newEmail string) {
			defer func() {
				if r := recover(); r != nil {
					log.Printf("[UserEmailChange] notify-old-email panic oldHash=%s recovered=%v",
						hashEmailForLog(oldEmail), r)
				}
			}()
			log.Printf("[UserEmailChange] notify-old-email start oldHash=%s newHash=%s",
				hashEmailForLog(oldEmail), hashEmailForLog(newEmail))
			if err := h.emailService.SendEmailChangeNotification(oldEmail, newEmail); err != nil {
				log.Printf("[UserEmailChange] notify-old-email failed oldHash=%s err=%v",
					hashEmailForLog(oldEmail), err)
				return
			}
			log.Printf("[UserEmailChange] notify-old-email sent oldHash=%s newHash=%s",
				hashEmailForLog(oldEmail), hashEmailForLog(newEmail))
		}(oldEmail, newEmail)
	}

	c.JSON(http.StatusOK, result.User)
}

func (h *UserHandler) RedeemCode(c *gin.Context) {
	userID, _ := c.Get("userID")

	var req redemptionpkg.RedeemCodeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数错误"})
		return
	}

	resp, err := h.redemptionService.RedeemCode(userID.(string), &req)
	if err != nil {
		switch {
		case errors.Is(err, redemptionpkg.ErrRedemptionCodeNotFound),
			errors.Is(err, redemptionpkg.ErrRedemptionCodeInvalid),
			errors.Is(err, redemptionpkg.ErrRedemptionDuplicate):
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		default:
			httpx.InternalError(c, err)
		}
		return
	}

	c.JSON(http.StatusOK, resp)
}

func (h *UserHandler) ValidateRedeemCode(c *gin.Context) {
	code := c.Param("code")

	resp, err := h.redemptionCodeService.ValidateRenewalCode(code)
	if err != nil {
		switch {
		case errors.Is(err, redemptionpkg.ErrRedemptionCodeNotFound),
			errors.Is(err, redemptionpkg.ErrRedemptionCodeInvalid):
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		default:
			httpx.InternalError(c, err)
		}
		return
	}

	c.JSON(http.StatusOK, resp)
}

func (h *UserHandler) GetRedemptions(c *gin.Context) {
	userID, _ := c.Get("userID")

	var req redemptionpkg.GetRedemptionsRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数错误"})
		return
	}

	resp, err := h.redemptionService.GetRedemptions(userID.(string), &req)
	if err != nil {
		httpx.InternalError(c, err)
		return
	}

	c.JSON(http.StatusOK, resp)
}

func (h *UserHandler) GetAllRedemptions(c *gin.Context) {
	var req redemptionpkg.GetAllRedemptionsRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数错误"})
		return
	}

	resp, err := h.redemptionService.GetAllRedemptions(&req)
	if err != nil {
		httpx.InternalError(c, err)
		return
	}

	c.JSON(http.StatusOK, resp)
}
