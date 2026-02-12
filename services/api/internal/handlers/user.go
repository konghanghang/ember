package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/konghang/ember/backend/internal/services"
)

// UserHandler 用户处理器
type UserHandler struct {
	userService           *services.UserService
	redemptionService     *services.RedemptionService
	redemptionCodeService *services.RedemptionCodeService
}

// NewUserHandler 创建用户处理器
func NewUserHandler() *UserHandler {
	return &UserHandler{
		userService:           &services.UserService{},
		redemptionService:     &services.RedemptionService{},
		redemptionCodeService: &services.RedemptionCodeService{},
	}
}

// GetUsers 获取用户列表
// @Summary 获取用户列表
// @Tags 用户管理
// @Produce json
// @Param page query int false "页码" default(1)
// @Param pageSize query int false "每页数量" default(20)
// @Param search query string false "搜索关键词"
// @Param isActive query bool false "是否启用"
// @Success 200 {object} services.GetUsersResponse
// @Router /api/v1/admin/users [get]
// @Security BearerAuth
func (h *UserHandler) GetUsers(c *gin.Context) {
	var req services.GetUsersRequest

	if err := c.ShouldBindQuery(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "请求参数错误",
		})
		return
	}

	resp, err := h.userService.GetUsers(&req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
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
		c.JSON(http.StatusNotFound, gin.H{
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
// @Param body body services.ExtendExpiryRequest true "延长天数"
// @Success 200 {object} models.User
// @Router /api/v1/admin/users/{id}/extend [put]
// @Security BearerAuth
func (h *UserHandler) ExtendExpiry(c *gin.Context) {
	userID := c.Param("id")

	var req services.ExtendExpiryRequest
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
// @Success 200 {object} models.User
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
// @Param body body services.ResetPasswordRequest true "新密码"
// @Success 200 {object} SuccessResponse
// @Router /api/v1/admin/users/{id}/reset-password [put]
// @Security BearerAuth
func (h *UserHandler) ResetPassword(c *gin.Context) {
	userID := c.Param("id")

	var req services.ResetPasswordRequest
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
// @Param body body services.UpdateProfileRequest true "个人信息"
// @Success 200 {object} models.User
// @Router /api/v1/user/profile [put]
// @Security BearerAuth
func (h *UserHandler) UpdateProfile(c *gin.Context) {
	userID, _ := c.Get("userID")

	var req services.UpdateProfileRequest
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
// @Param body body services.UpdatePasswordRequest true "密码信息"
// @Success 200 {object} SuccessResponse
// @Router /api/v1/user/password [put]
// @Security BearerAuth
func (h *UserHandler) UpdatePassword(c *gin.Context) {
	userID, _ := c.Get("userID")

	var req services.UpdatePasswordRequest
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
// @Param body body services.UpdateEmailRequest true "邮箱信息"
// @Success 200 {object} models.User
// @Router /api/v1/user/email [put]
// @Security BearerAuth
func (h *UserHandler) UpdateEmail(c *gin.Context) {
	userID, _ := c.Get("userID")

	var req services.UpdateEmailRequest
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

	var req services.RedeemCodeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数错误"})
		return
	}

	resp, err := h.redemptionService.RedeemCode(userID.(string), &req)
	if err != nil {
		switch err.Error() {
		case "兑换码不存在", "兑换码已失效":
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		case "Emby 解封失败，请稍后重试", "兑换失败，请稍后重试":
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
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

	var req services.GetRedemptionsRequest
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
	var req services.GetAllRedemptionsRequest
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
