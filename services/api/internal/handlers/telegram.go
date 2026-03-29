package handlers

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	redemptionpkg "github.com/konghang/ember/backend/internal/services/redemption"
	subscriptionpkg "github.com/konghang/ember/backend/internal/services/subscription"
	telegrampkg "github.com/konghang/ember/backend/internal/services/telegram"
)

type TelegramHandler struct {
	telegramService *telegrampkg.TelegramService
}

func NewTelegramHandler() *TelegramHandler {
	return &TelegramHandler{
		telegramService: telegrampkg.NewDefaultService(),
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
		case errors.Is(err, telegrampkg.ErrUserAlreadyBoundTelegram):
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
		case errors.Is(err, telegrampkg.ErrTelegramNotBound):
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
	var req telegrampkg.TelegramBindRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数错误"})
		return
	}

	result, err := h.telegramService.VerifyBind(req.TelegramID, req.Code)
	if err != nil {
		statusCode := http.StatusInternalServerError
		switch {
		case errors.Is(err, telegrampkg.ErrTelegramBindCodeInvalid),
			errors.Is(err, telegrampkg.ErrTelegramAlreadyBound),
			errors.Is(err, telegrampkg.ErrUserAlreadyBoundTelegram):
			statusCode = http.StatusBadRequest
		}
		c.JSON(statusCode, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, result)
}

// GetAccountInfo Bot 查询账号信息
func (h *TelegramHandler) GetAccountInfo(c *gin.Context) {
	var req telegrampkg.TelegramIDRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数错误"})
		return
	}

	result, err := h.telegramService.GetAccountInfo(req.TelegramID)
	if err != nil {
		statusCode := http.StatusInternalServerError
		if errors.Is(err, telegrampkg.ErrTelegramNotBound) {
			statusCode = http.StatusBadRequest
		}
		c.JSON(statusCode, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, result)
}

// RedeemByTelegram Bot 通过 Telegram 兑换续期码
func (h *TelegramHandler) RedeemByTelegram(c *gin.Context) {
	var req telegrampkg.TelegramRedeemRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数错误"})
		return
	}

	result, err := h.telegramService.RedeemByTelegram(req.TelegramID, req.Code)
	if err != nil {
		statusCode := http.StatusInternalServerError
		switch {
		case errors.Is(err, telegrampkg.ErrTelegramNotBound),
			errors.Is(err, redemptionpkg.ErrRedemptionCodeNotFound),
			errors.Is(err, redemptionpkg.ErrRedemptionCodeInvalid),
			errors.Is(err, redemptionpkg.ErrRedemptionDuplicate):
			statusCode = http.StatusBadRequest
		case errors.Is(err, redemptionpkg.ErrRedeemFailed),
			errors.Is(err, redemptionpkg.ErrEmbyUnbanFailed):
			statusCode = http.StatusInternalServerError
		}
		c.JSON(statusCode, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, result)
}

// ResetPassword Bot 通过 Telegram 重置密码
func (h *TelegramHandler) ResetPassword(c *gin.Context) {
	var req telegrampkg.TelegramResetPasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数错误"})
		return
	}

	if err := h.telegramService.ResetPassword(req.TelegramID, req.NewPassword); err != nil {
		statusCode := http.StatusInternalServerError
		if errors.Is(err, telegrampkg.ErrTelegramNotBound) {
			statusCode = http.StatusBadRequest
		}
		c.JSON(statusCode, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "密码重置成功"})
}

// SubscribeByTelegram Bot 通过 Telegram 创建求片订阅
func (h *TelegramHandler) SubscribeByTelegram(c *gin.Context) {
	var req telegrampkg.TelegramSubscribeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数错误"})
		return
	}

	if err := h.telegramService.SubscribeByTelegram(req); err != nil {
		statusCode := http.StatusInternalServerError
		switch {
		case errors.Is(err, telegrampkg.ErrTelegramNotBound):
			statusCode = http.StatusBadRequest
		case errors.Is(err, subscriptionpkg.ErrSubscriptionDuplicated):
			statusCode = http.StatusConflict
		case errors.Is(err, subscriptionpkg.ErrSubscriptionInvalidSeason):
			statusCode = http.StatusBadRequest
		}
		c.JSON(statusCode, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "订阅创建成功"})
}
