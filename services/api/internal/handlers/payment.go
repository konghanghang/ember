package handlers

import (
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/konghang/ember/backend/internal/services"
)

type PaymentHandler struct {
	service *services.PaymentService
}

func NewPaymentHandler() *PaymentHandler {
	return &PaymentHandler{
		service: services.NewPaymentService(),
	}
}

func (h *PaymentHandler) GetActivePlans(c *gin.Context) {
	plans, err := h.service.GetActivePlans()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
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

	var req services.CreateCheckoutRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数错误"})
		return
	}

	resp, err := h.service.CreateCheckoutSession(userID.(string), &req)
	if err != nil {
		switch {
		case errors.Is(err, services.ErrPlanNotFound):
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		case errors.Is(err, services.ErrStripeNotConfigured):
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": err.Error()})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
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

	var req services.GetPaymentsRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数错误"})
		return
	}

	resp, err := h.service.GetUserPayments(userID.(string), &req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, resp)
}

func (h *PaymentHandler) HandleStripeWebhook(c *gin.Context) {
	if err := h.service.HandleWebhook(c.Request); err != nil {
		switch {
		case errors.Is(err, services.ErrStripeNotConfigured):
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": err.Error()})
		case strings.Contains(err.Error(), "签名"), strings.Contains(err.Error(), "解析"):
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		case errors.Is(err, services.ErrPaymentFailed):
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		}
		return
	}

	c.JSON(http.StatusOK, gin.H{"received": true})
}

func (h *PaymentHandler) GetPlans(c *gin.Context) {
	var req services.GetPlansRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数错误"})
		return
	}

	resp, err := h.service.GetPlans(&req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, resp)
}

func (h *PaymentHandler) CreatePlan(c *gin.Context) {
	var req services.CreatePlanRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数错误"})
		return
	}

	plan, err := h.service.CreatePlan(&req)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, plan)
}

func (h *PaymentHandler) UpdatePlan(c *gin.Context) {
	id := c.Param("id")

	var req services.UpdatePlanRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数错误"})
		return
	}

	plan, err := h.service.UpdatePlan(id, &req)
	if err != nil {
		switch {
		case errors.Is(err, services.ErrPlanNotFound):
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
		if errors.Is(err, services.ErrPlanNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "下架成功"})
}

func (h *PaymentHandler) GetAllPayments(c *gin.Context) {
	var req services.GetPaymentsRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数错误"})
		return
	}

	resp, err := h.service.GetAllPayments(&req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, resp)
}
