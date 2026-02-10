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

// AdminLogin 管理员登录
// @Summary 管理员登录
// @Tags 认证
// @Accept json
// @Produce json
// @Param body body services.AdminLoginRequest true "登录信息"
// @Success 200 {object} services.AdminLoginResponse
// @Failure 400 {object} ErrorResponse
// @Router /api/v1/admin/login [post]
func (h *AuthHandler) AdminLogin(c *gin.Context) {
	var req services.AdminLoginRequest

	// 绑定并验证请求参数
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "请求参数错误",
		})
		return
	}

	// 调用服务层
	resp, err := h.authService.AdminLogin(&req)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{
			"error": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, resp)
}

// UserLogin 用户登录
// @Summary 用户登录
// @Tags 认证
// @Accept json
// @Produce json
// @Param body body services.UserLoginRequest true "登录信息"
// @Success 200 {object} services.UserLoginResponse
// @Failure 400 {object} ErrorResponse
// @Router /api/v1/user/login [post]
func (h *AuthHandler) UserLogin(c *gin.Context) {
	var req services.UserLoginRequest

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "请求参数错误",
		})
		return
	}

	resp, err := h.authService.UserLogin(&req)
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
	// 从中间件获取用户信息
	userID, _ := c.Get("userID")
	role, _ := c.Get("role")

	user, err := h.authService.GetCurrentUser(userID.(string), role.(string))
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

// AdminLogout 管理员登出
// @Summary 管理员登出
// @Tags 认证
// @Produce json
// @Success 200 {object} SuccessResponse
// @Router /api/v1/admin/logout [post]
// @Security BearerAuth
func (h *AuthHandler) AdminLogout(c *gin.Context) {
	// JWT 是无状态的，登出在前端删除 Token 即可
	// 这里只返回成功响应
	c.JSON(http.StatusOK, gin.H{
		"message": "登出成功",
	})
}

// UserLogout 用户登出
// @Summary 用户登出
// @Tags 认证
// @Produce json
// @Success 200 {object} SuccessResponse
// @Router /api/v1/user/logout [post]
// @Security BearerAuth
func (h *AuthHandler) UserLogout(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"message": "登出成功",
	})
}

// ErrorResponse 错误响应结构
type ErrorResponse struct {
	Error string `json:"error"`
}

// SuccessResponse 成功响应结构
type SuccessResponse struct {
	Message string `json:"message"`
}
