package handlers

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/konghang/ember/backend/internal/common/httpx"
	paymentpkg "github.com/konghang/ember/backend/internal/services/payment"
)

type PaymentHandler struct {
	service *paymentpkg.PaymentService
}

func NewPaymentHandler() *PaymentHandler {
	return &PaymentHandler{
		service: paymentpkg.NewPaymentService(),
	}
}

func (h *PaymentHandler) GetUserPlans(c *gin.Context) {
	userID, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "未登录"})
		return
	}

	plans, err := h.service.GetPlansForUser(userID.(string))
	if err != nil {
		switch {
		case errors.Is(err, paymentpkg.ErrPlanGroupInvalid), errors.Is(err, paymentpkg.ErrDefaultPlanGroupNotFound):
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		default:
			httpx.InternalError(c, err)
		}
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": plans})
}

func (h *PaymentHandler) CreateCheckout(c *gin.Context) {
	userID, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "未登录"})
		return
	}

	var req paymentpkg.CreateCheckoutRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数错误"})
		return
	}

	resp, err := h.service.CreateCheckoutSession(userID.(string), &req)
	if err != nil {
		switch {
		case errors.Is(err, paymentpkg.ErrPlanNotFound):
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		case errors.Is(err, paymentpkg.ErrPlanGroupInvalid), errors.Is(err, paymentpkg.ErrPlanGroupNotFound), errors.Is(err, paymentpkg.ErrDefaultPlanGroupNotFound):
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		case errors.Is(err, paymentpkg.ErrStripeNotConfigured):
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": err.Error()})
		default:
			httpx.InternalError(c, err)
		}
		return
	}

	c.JSON(http.StatusOK, resp)
}

func (h *PaymentHandler) GetMyPayments(c *gin.Context) {
	userID, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "未登录"})
		return
	}

	var req paymentpkg.GetPaymentsRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数错误"})
		return
	}

	resp, err := h.service.GetUserPayments(userID.(string), &req)
	if err != nil {
		httpx.InternalError(c, err)
		return
	}

	c.JSON(http.StatusOK, resp)
}

func (h *PaymentHandler) HandleStripeWebhook(c *gin.Context) {
	if err := h.service.HandleWebhook(c.Request); err != nil {
		switch {
		case errors.Is(err, paymentpkg.ErrStripeNotConfigured):
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": err.Error()})
		case errors.Is(err, paymentpkg.ErrStripeWebhookInvalid), errors.Is(err, paymentpkg.ErrStripeWebhookParseFailed):
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		default:
			httpx.InternalError(c, err)
		}
		return
	}

	c.JSON(http.StatusOK, gin.H{"received": true})
}

func (h *PaymentHandler) GetPlans(c *gin.Context) {
	var req paymentpkg.GetPlansRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数错误"})
		return
	}

	resp, err := h.service.GetPlans(&req)
	if err != nil {
		switch {
		case errors.Is(err, paymentpkg.ErrPlanGroupInvalid):
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		default:
			httpx.InternalError(c, err)
		}
		return
	}

	c.JSON(http.StatusOK, resp)
}

func (h *PaymentHandler) GetPlanGroups(c *gin.Context) {
	resp, err := h.service.GetPlanGroups()
	if err != nil {
		httpx.InternalError(c, err)
		return
	}
	c.JSON(http.StatusOK, resp)
}

func (h *PaymentHandler) CreatePlanGroup(c *gin.Context) {
	var req paymentpkg.CreatePlanGroupRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数错误"})
		return
	}

	group, err := h.service.CreatePlanGroup(&req)
	if err != nil {
		switch {
		case errors.Is(err, paymentpkg.ErrPlanGroupInvalid):
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		default:
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		}
		return
	}
	c.JSON(http.StatusOK, group)
}

func (h *PaymentHandler) UpdatePlanGroup(c *gin.Context) {
	key := c.Param("key")
	var req paymentpkg.UpdatePlanGroupRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数错误"})
		return
	}

	group, err := h.service.UpdatePlanGroup(key, &req)
	if err != nil {
		switch {
		case errors.Is(err, paymentpkg.ErrPlanGroupInvalid):
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		case errors.Is(err, paymentpkg.ErrPlanGroupNotFound):
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		default:
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		}
		return
	}
	c.JSON(http.StatusOK, group)
}

func (h *PaymentHandler) DeletePlanGroup(c *gin.Context) {
	key := c.Param("key")
	if err := h.service.DeletePlanGroup(key); err != nil {
		switch {
		case errors.Is(err, paymentpkg.ErrPlanGroupInvalid):
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		case errors.Is(err, paymentpkg.ErrPlanGroupNotFound):
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		case errors.Is(err, paymentpkg.ErrDefaultPlanGroupDelete), errors.Is(err, paymentpkg.ErrPlanGroupDeleteBlocked):
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		default:
			httpx.InternalError(c, err)
		}
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "删除成功"})
}

func (h *PaymentHandler) CreatePlan(c *gin.Context) {
	var req paymentpkg.CreatePlanRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数错误"})
		return
	}

	plan, err := h.service.CreatePlan(&req)
	if err != nil {
		switch {
		case errors.Is(err, paymentpkg.ErrPlanGroupInvalid), errors.Is(err, paymentpkg.ErrPlanGroupNotFound):
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		default:
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		}
		return
	}

	c.JSON(http.StatusOK, plan)
}

func (h *PaymentHandler) UpdatePlan(c *gin.Context) {
	id := c.Param("id")

	var req paymentpkg.UpdatePlanRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数错误"})
		return
	}

	plan, err := h.service.UpdatePlan(id, &req)
	if err != nil {
		switch {
		case errors.Is(err, paymentpkg.ErrPlanNotFound):
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		default:
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		}
		return
	}

	c.JSON(http.StatusOK, plan)
}

func (h *PaymentHandler) DeletePlan(c *gin.Context) {
	id := c.Param("id")
	if err := h.service.DeletePlan(id); err != nil {
		if errors.Is(err, paymentpkg.ErrPlanNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
			return
		}
		httpx.InternalError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "下架成功"})
}

func (h *PaymentHandler) GetAllPayments(c *gin.Context) {
	var req paymentpkg.GetPaymentsRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数错误"})
		return
	}

	resp, err := h.service.GetAllPayments(&req)
	if err != nil {
		httpx.InternalError(c, err)
		return
	}

	c.JSON(http.StatusOK, resp)
}
