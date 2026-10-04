package handlers

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/konghang/ember/backend/internal/common/httpx"
	configpkg "github.com/konghang/ember/backend/internal/config"
	entitlementpkg "github.com/konghang/ember/backend/internal/services/entitlement"
	paymentpkg "github.com/konghang/ember/backend/internal/services/payment"
	userpkg "github.com/konghang/ember/backend/internal/services/user"
)

// GetMyEntitlements exposes the caller's grants without accepting a user ID override.
func (h *UserHandler) GetMyEntitlements(c *gin.Context) { h.listEntitlements(c, c.GetString("userID")) }

// GetUserEntitlements serves the administrator-only target user route.
func (h *UserHandler) GetUserEntitlements(c *gin.Context) { h.listEntitlements(c, c.Param("id")) }

// listEntitlements preserves the common list envelope.
func (h *UserHandler) listEntitlements(c *gin.Context, id string) {
	rows, err := h.userService.GetEntitlements(c.Request.Context(), id)
	if err != nil {
		httpx.InternalError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": rows, "businessTimezone": configpkg.LoadConfiguredTimezone().String()})
}

// AdjustUserEntitlement binds one explicit administrator action and maps domain errors.
func (h *UserHandler) AdjustUserEntitlement(c *gin.Context) {
	var req userpkg.AdjustEntitlementRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": "请求参数错误"})
		return
	}
	actor := ""
	if id := currentUserIDPtr(c); id != nil {
		actor = *id
	}
	if err := h.userService.AdjustEntitlement(c.Request.Context(), c.Param("id"), actor, &req); err != nil {
		writeEntitlementError(c, err)
		return
	}
	c.JSON(200, gin.H{"message": "权益已更新"})
}

// SetEntitlementRanks applies an explicit full ranking instead of guessing from display sorting.
func (h *PaymentHandler) SetEntitlementRanks(c *gin.Context) {
	var req paymentpkg.SetEntitlementRanksRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": "请求参数错误"})
		return
	}
	if err := h.service.SetEntitlementRanks(c.Request.Context(), req); err != nil {
		writeEntitlementError(c, err)
		return
	}
	c.JSON(200, gin.H{"message": "权益等级已保存"})
}

// ResolvePayment records an explicit manual resolution; no external refund is triggered.
func (h *PaymentHandler) ResolvePayment(c *gin.Context) {
	var req paymentpkg.ResolvePaymentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": "请求参数错误"})
		return
	}
	actor := "admin:api-key"
	if id := currentUserIDPtr(c); id != nil {
		actor = *id
	}
	if err := h.service.ResolvePayment(c.Request.Context(), c.Param("id"), actor, req); err != nil {
		writeEntitlementError(c, err)
		return
	}
	c.JSON(200, gin.H{"message": "人工处理已记录"})
}

// writeEntitlementError separates actionable input errors from storage failures.
func writeEntitlementError(c *gin.Context, err error) {
	if errors.Is(err, entitlementpkg.ErrInvalidBenefit) || errors.Is(err, entitlementpkg.ErrInvalidHolding) || errors.Is(err, entitlementpkg.ErrAlreadyOwned) || errors.Is(err, entitlementpkg.ErrGroupsNotReady) || errors.Is(err, userpkg.ErrRequestInvalid) || errors.Is(err, userpkg.ErrExpiresAtFormatInvalid) || errors.Is(err, paymentpkg.ErrManualResolutionInvalid) {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	httpx.InternalError(c, err)
}
