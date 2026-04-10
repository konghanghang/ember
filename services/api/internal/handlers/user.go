package handlers

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	paymentpkg "github.com/konghang/ember/backend/internal/services/payment"
	redemptionpkg "github.com/konghang/ember/backend/internal/services/redemption"
	userpkg "github.com/konghang/ember/backend/internal/services/user"
)

// UserHandler 用户处理器
type UserHandler struct {
	userService           *userpkg.UserService
	redemptionService     *redemptionpkg.RedemptionService
	redemptionCodeService *redemptionpkg.RedemptionCodeService
}

// NewUserHandler 创建用户处理器
func NewUserHandler() *UserHandler {
	return &UserHandler{
		userService:           userpkg.NewUserService(),
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
		statusCode := http.StatusInternalServerError
		switch err.Error() {
		case "用户名长度必须为 3-50 位", "用户名只能包含字母和数字", "邮箱不能为空", "邮箱格式错误",
			"密码长度不能小于 6 位", "neverExpire=true 时不能再传 expiresAt", "neverExpire=false 时必须传 expiresAt",
			"expiresAt 必须是 RFC3339 格式", "用户名已存在", "邮箱已存在":
			statusCode = http.StatusBadRequest
		default:
			if errors.Is(err, userpkg.ErrInvalidPlanGroup) || errors.Is(err, paymentpkg.ErrPlanGroupNotFound) {
				statusCode = http.StatusBadRequest
			}
		}

		c.JSON(statusCode, gin.H{
			"error": err.Error(),
		})
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
		statusCode := http.StatusInternalServerError
		if errors.Is(err, userpkg.ErrInvalidExpiresAfter) || errors.Is(err, userpkg.ErrInvalidEmbyStatus) || errors.Is(err, userpkg.ErrInvalidPlanGroup) || errors.Is(err, paymentpkg.ErrDefaultPlanGroupNotFound) {
			statusCode = http.StatusBadRequest
		}

		c.JSON(statusCode, gin.H{
			"error": err.Error(),
		})
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
		statusCode := http.StatusNotFound
		if errors.Is(err, paymentpkg.ErrDefaultPlanGroupNotFound) {
			statusCode = http.StatusBadRequest
		}
		c.JSON(statusCode, gin.H{
			"error": err.Error(),
		})
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

	user, err := h.userService.UpdateUserByAdmin(userID, &req)
	if err != nil {
		statusCode := http.StatusInternalServerError
		switch err.Error() {
		case "用户不存在":
			statusCode = http.StatusNotFound
		case "至少提供一个可更新字段", "clearExpiresAt 和 expiresAt 不能同时设置", "邮箱不能为空", "邮箱格式错误", "expiresAt 必须是 RFC3339 格式", "邮箱已存在":
			statusCode = http.StatusBadRequest
		default:
			if errors.Is(err, userpkg.ErrInvalidPlanGroup) || errors.Is(err, paymentpkg.ErrPlanGroupNotFound) || errors.Is(err, paymentpkg.ErrDefaultPlanGroupNotFound) {
				statusCode = http.StatusBadRequest
			}
		}

		c.JSON(statusCode, gin.H{
			"error": err.Error(),
		})
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
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": err.Error(),
		})
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

	user, err := h.userService.ToggleUserStatus(userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": err.Error(),
		})
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

	err := h.userService.DeleteUser(userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": err.Error(),
		})
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
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": err.Error(),
		})
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
		c.JSON(http.StatusNotFound, gin.H{
			"error": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, user)
}

// UpdateProfile 更新个人信息
// @Summary 更新个人信息
// @Tags 用户面板
// @Accept json
// @Produce json
// @Param body body user.UpdateProfileRequest true "个人信息"
// @Success 200 {object} models.User
// @Router /api/v1/user/profile [put]
// @Security BearerAuth
func (h *UserHandler) UpdateProfile(c *gin.Context) {
	userID, _ := c.Get("userID")

	var req userpkg.UpdateProfileRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "请求参数错误",
		})
		return
	}

	user, err := h.userService.UpdateProfile(userID.(string), &req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": err.Error(),
		})
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
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "密码修改成功",
	})
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
	userID, _ := c.Get("userID")

	var req userpkg.UpdateEmailRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "请求参数错误",
		})
		return
	}

	user, err := h.userService.UpdateEmail(userID.(string), &req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, user)
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
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		}
		return
	}

	c.JSON(http.StatusOK, resp)
}

func (h *UserHandler) ValidateRedeemCode(c *gin.Context) {
	code := c.Param("code")

	resp, err := h.redemptionCodeService.ValidateCode(code)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
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
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
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
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, resp)
}
