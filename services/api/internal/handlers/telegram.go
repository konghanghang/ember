package handlers

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/konghang/ember/backend/internal/services"
)

type TelegramHandler struct {
	telegramService *services.TelegramService
}

func NewTelegramHandler() *TelegramHandler {
	return &TelegramHandler{
		telegramService: services.NewTelegramService(),
	}
}

// GenerateBindCode 生成 Telegram 绑定码
func (h *TelegramHandler) GenerateBindCode(c *gin.Context) {
	userID, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "未登录"})
		return
	}

	code, expiresAt, err := h.telegramService.GenerateBindCode(userID.(string))
	if err != nil {
		statusCode := http.StatusInternalServerError
		switch {
		case errors.Is(err, services.ErrUserAlreadyBoundTelegram):
			statusCode = http.StatusBadRequest
		case err.Error() == "用户不存在":
			statusCode = http.StatusNotFound
		}
		c.JSON(statusCode, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":      code,
		"expiresAt": expiresAt,
	})
}

// Unbind 解绑 Telegram
func (h *TelegramHandler) Unbind(c *gin.Context) {
	userID, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "未登录"})
		return
	}

	if err := h.telegramService.Unbind(userID.(string)); err != nil {
		statusCode := http.StatusInternalServerError
		switch {
		case errors.Is(err, services.ErrTelegramNotBound):
			statusCode = http.StatusBadRequest
		case err.Error() == "用户不存在":
			statusCode = http.StatusNotFound
		}
		c.JSON(statusCode, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "已解除 Telegram 绑定"})
}

// VerifyBind Bot 验证绑定码
func (h *TelegramHandler) VerifyBind(c *gin.Context) {
	var req services.TelegramBindRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数错误"})
		return
	}

	result, err := h.telegramService.VerifyBind(req.TelegramID, req.Code)
	if err != nil {
		statusCode := http.StatusInternalServerError
		switch {
		case errors.Is(err, services.ErrTelegramBindCodeInvalid),
			errors.Is(err, services.ErrTelegramAlreadyBound),
			errors.Is(err, services.ErrUserAlreadyBoundTelegram):
			statusCode = http.StatusBadRequest
		}
		c.JSON(statusCode, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, result)
}

// GetAccountInfo Bot 查询账号信息
func (h *TelegramHandler) GetAccountInfo(c *gin.Context) {
	var req services.TelegramIDRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数错误"})
		return
	}

	result, err := h.telegramService.GetAccountInfo(req.TelegramID)
	if err != nil {
		statusCode := http.StatusInternalServerError
		if errors.Is(err, services.ErrTelegramNotBound) {
			statusCode = http.StatusBadRequest
		}
		c.JSON(statusCode, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, result)
}

// RedeemByTelegram Bot 通过 Telegram 兑换续期码
func (h *TelegramHandler) RedeemByTelegram(c *gin.Context) {
	var req services.TelegramRedeemRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数错误"})
		return
	}

	result, err := h.telegramService.RedeemByTelegram(req.TelegramID, req.Code)
	if err != nil {
		statusCode := http.StatusInternalServerError
		switch {
		case errors.Is(err, services.ErrTelegramNotBound),
			errors.Is(err, services.ErrRedemptionCodeNotFound),
			errors.Is(err, services.ErrRedemptionCodeInvalid),
			errors.Is(err, services.ErrRedemptionDuplicate):
			statusCode = http.StatusBadRequest
		case errors.Is(err, services.ErrRedeemFailed),
			errors.Is(err, services.ErrEmbyUnbanFailed):
			statusCode = http.StatusInternalServerError
		}
		c.JSON(statusCode, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, result)
}

// ResetPassword Bot 通过 Telegram 重置密码
func (h *TelegramHandler) ResetPassword(c *gin.Context) {
	var req services.TelegramResetPasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数错误"})
		return
	}

	if err := h.telegramService.ResetPassword(req.TelegramID, req.NewPassword); err != nil {
		statusCode := http.StatusInternalServerError
		if errors.Is(err, services.ErrTelegramNotBound) {
			statusCode = http.StatusBadRequest
		}
		c.JSON(statusCode, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "密码重置成功"})
}

// SubscribeByTelegram Bot 通过 Telegram 创建求片订阅
func (h *TelegramHandler) SubscribeByTelegram(c *gin.Context) {
	var req services.TelegramSubscribeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数错误"})
		return
	}

	if err := h.telegramService.SubscribeByTelegram(req); err != nil {
		statusCode := http.StatusInternalServerError
		switch {
		case errors.Is(err, services.ErrTelegramNotBound):
			statusCode = http.StatusBadRequest
		case errors.Is(err, services.ErrSubscriptionDuplicated):
			statusCode = http.StatusConflict
		}
		c.JSON(statusCode, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "订阅创建成功"})
}
