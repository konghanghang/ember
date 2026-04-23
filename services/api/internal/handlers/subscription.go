package handlers

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/konghang/ember/backend/internal/models"
	subscriptionpkg "github.com/konghang/ember/backend/internal/services/subscription"
)

// SubscriptionHandler 订阅处理器
type SubscriptionHandler struct {
	service *subscriptionpkg.SubscriptionService
}

// NewSubscriptionHandler 创建订阅处理器
func NewSubscriptionHandler() *SubscriptionHandler {
	return &SubscriptionHandler{
		service: subscriptionpkg.NewSubscriptionService(),
	}
}

func parseSubscriptionStatus(statusStr string) *models.SubscriptionStatus {
	if statusStr == "" {
		return nil
	}
	s := models.SubscriptionStatus(statusStr)
	if s == models.SubscriptionPending || s == models.SubscriptionApproved || s == models.SubscriptionRejected || s == models.SubscriptionIngested {
		return &s
	}
	return nil
}

// CreateSubscription 用户创建订阅
// POST /api/v1/user/subscriptions
func (h *SubscriptionHandler) CreateSubscription(c *gin.Context) {
	// 获取当前用户 ID
	userID, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "未登录"})
		return
	}

	// 解析请求
	var req subscriptionpkg.CreateSubscriptionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数错误"})
		return
	}

	// 创建订阅
	result, err := h.service.CreateSubscriptionWithResult(userID.(string), req)
	if err != nil {
		if errors.Is(err, subscriptionpkg.ErrSubscriptionDuplicated) {
			c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
			return
		}
		if errors.Is(err, subscriptionpkg.ErrSubscriptionInvalidSeason) {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if result != nil && result.ConfirmationRequired {
		c.JSON(http.StatusConflict, gin.H{
			"error":                "库内已存在相关资源，请确认后继续提交",
			"confirmationRequired": true,
			"detectionFailed":      result.DetectionFailed,
			"existingSummary":      result.ExistingSummary,
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true})
}

// CheckExisting 提交前库内检测
// POST /api/v1/subscriptions/check-existing
func (h *SubscriptionHandler) CheckExisting(c *gin.Context) {
	var req subscriptionpkg.CheckExistingRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数错误"})
		return
	}

	result, err := h.service.CheckExisting(req)
	if err != nil {
		if errors.Is(err, subscriptionpkg.ErrSubscriptionInvalidSeason) {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, result)
}

// GetMySubscriptions 获取我的订阅列表
// GET /api/v1/user/subscriptions
func (h *SubscriptionHandler) GetMySubscriptions(c *gin.Context) {
	// 获取当前用户 ID
	userID, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "未登录"})
		return
	}

	// 查询订阅列表
	subscriptions, err := h.service.GetUserSubscriptions(userID.(string))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, subscriptions)
}

// GetSubscriptions 统一订阅列表（角色感知）
// GET /api/v1/subscriptions
func (h *SubscriptionHandler) GetSubscriptions(c *gin.Context) {
	userID, hasUserID := c.Get("userID")
	role, hasRole := c.Get("role")
	if !hasUserID || !hasRole {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "未登录"})
		return
	}

	statusStr := c.Query("status")
	pageStr := c.DefaultQuery("page", "1")
	pageSizeStr := c.DefaultQuery("pageSize", "12")

	page, err := strconv.Atoi(pageStr)
	if err != nil || page < 1 {
		page = 1
	}
	pageSize, err := strconv.Atoi(pageSizeStr)
	if err != nil || pageSize < 1 || pageSize > 100 {
		pageSize = 12
	}

	status := parseSubscriptionStatus(statusStr)

	if role.(string) == "admin" {
		result, err := h.service.GetAllSubscriptions(status, page, pageSize)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, result)
		return
	}

	result, err := h.service.GetUserSubscriptionsPaginated(userID.(string), status, page, pageSize)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, result)
}

// DeleteSubscription 删除订阅（仅 PENDING 状态）
// DELETE /api/v1/user/subscriptions/:id
func (h *SubscriptionHandler) DeleteSubscription(c *gin.Context) {
	// 获取当前用户 ID
	userID, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "未登录"})
		return
	}

	// 获取订阅 ID
	subscriptionID := c.Param("id")
	if subscriptionID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "订阅 ID 不能为空"})
		return
	}

	// 删除订阅
	if err := h.service.DeleteSubscription(subscriptionID, userID.(string)); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true})
}

// GetAllSubscriptions 管理员查看所有订阅（分页）
// GET /api/v1/admin/subscriptions
// Query: status, page, pageSize
func (h *SubscriptionHandler) GetAllSubscriptions(c *gin.Context) {
	// 解析查询参数
	statusStr := c.Query("status")
	pageStr := c.DefaultQuery("page", "1")
	pageSizeStr := c.DefaultQuery("pageSize", "10")

	page, err := strconv.Atoi(pageStr)
	if err != nil || page < 1 {
		page = 1
	}

	pageSize, err := strconv.Atoi(pageSizeStr)
	if err != nil || pageSize < 1 || pageSize > 100 {
		pageSize = 10
	}

	status := parseSubscriptionStatus(statusStr)

	// 查询订阅列表
	result, err := h.service.GetAllSubscriptions(status, page, pageSize)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, result)
}

// ApproveSubscription 批准订阅
// PUT /api/v1/admin/subscriptions/:id/approve
func (h *SubscriptionHandler) ApproveSubscription(c *gin.Context) {
	// 获取订阅 ID
	subscriptionID := c.Param("id")
	if subscriptionID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "订阅 ID 不能为空"})
		return
	}

	// 批准订阅
	if err := h.service.ApproveSubscription(subscriptionID); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true})
}

// RejectSubscription 拒绝订阅
// PUT /api/v1/admin/subscriptions/:id/reject
func (h *SubscriptionHandler) RejectSubscription(c *gin.Context) {
	// 获取订阅 ID
	subscriptionID := c.Param("id")
	if subscriptionID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "订阅 ID 不能为空"})
		return
	}

	var req subscriptionpkg.RejectSubscriptionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数错误"})
		return
	}

	// 拒绝订阅
	if err := h.service.RejectSubscription(subscriptionID, req.Reason); err != nil {
		if errors.Is(err, subscriptionpkg.ErrSubscriptionRejectReason) {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true})
}

// MarkSubscriptionIngested 校验库内存在后标记订阅已入库
// PUT /api/v1/admin/subscriptions/:id/ingest
func (h *SubscriptionHandler) MarkSubscriptionIngested(c *gin.Context) {
	subscriptionID := c.Param("id")
	if subscriptionID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "订阅 ID 不能为空"})
		return
	}

	if err := h.service.MarkSubscriptionIngestedAsAdmin(subscriptionID); err != nil {
		if errors.Is(err, subscriptionpkg.ErrSubscriptionNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
			return
		}
		if errors.Is(err, subscriptionpkg.ErrSubscriptionNotApproved) {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		if errors.Is(err, subscriptionpkg.ErrSubscriptionNotInLibrary) {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true})
}

// AdminDeleteSubscription 管理员删除订阅
// DELETE /api/v1/admin/subscriptions/:id
func (h *SubscriptionHandler) AdminDeleteSubscription(c *gin.Context) {
	subscriptionID := c.Param("id")
	if subscriptionID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "订阅 ID 不能为空"})
		return
	}

	if err := h.service.DeleteSubscriptionAsAdmin(subscriptionID); err != nil {
		if errors.Is(err, subscriptionpkg.ErrSubscriptionNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true})
}
