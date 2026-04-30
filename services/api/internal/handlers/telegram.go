package handlers

import (
	"errors"
	"net/http"
	"regexp"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/konghang/ember/backend/internal/common/httpx"
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
		switch {
		case errors.Is(err, telegrampkg.ErrUserAlreadyBoundTelegram):
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		case errors.Is(err, telegrampkg.ErrTelegramUserNotFound):
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		default:
			httpx.InternalError(c, err)
		}
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
		switch {
		case errors.Is(err, telegrampkg.ErrTelegramNotBound):
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		case errors.Is(err, telegrampkg.ErrTelegramUserNotFound):
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		default:
			httpx.InternalError(c, err)
		}
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
		switch {
		case errors.Is(err, telegrampkg.ErrTelegramBindCodeInvalid),
			errors.Is(err, telegrampkg.ErrTelegramAlreadyBound),
			errors.Is(err, telegrampkg.ErrUserAlreadyBoundTelegram):
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		default:
			httpx.InternalError(c, err)
		}
		return
	}

	c.JSON(http.StatusOK, result)
}

// GetAccountInfo Bot 查询账号信息
//
// 反账号枚举：未绑定 Telegram 的请求统一返回与"参数错误"一致的 400 响应，
// 不再向调用方透传 ErrTelegramNotBound 字面值。
func (h *TelegramHandler) GetAccountInfo(c *gin.Context) {
	var req telegrampkg.TelegramIDRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数错误"})
		return
	}

	result, err := h.telegramService.GetAccountInfo(req.TelegramID)
	if err != nil {
		if errors.Is(err, telegrampkg.ErrTelegramNotBound) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数错误"})
			return
		}
		httpx.InternalError(c, err)
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
		if errors.Is(err, telegrampkg.ErrTelegramNotBound) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数错误"})
			return
		}
		switch {
		case errors.Is(err, redemptionpkg.ErrRedemptionCodeNotFound),
			errors.Is(err, redemptionpkg.ErrRedemptionCodeInvalid),
			errors.Is(err, redemptionpkg.ErrRedemptionDuplicate):
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		case errors.Is(err, redemptionpkg.ErrRedeemFailed),
			errors.Is(err, redemptionpkg.ErrEmbyUnbanFailed):
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		default:
			httpx.InternalError(c, err)
		}
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
		if errors.Is(err, telegrampkg.ErrTelegramNotBound) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数错误"})
			return
		}
		httpx.InternalError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "密码重置成功"})
}

var (
	tmdbIDPattern     = regexp.MustCompile(`^\d{1,10}$`)
	posterPathPattern = regexp.MustCompile(`^/[\w./-]+$`)
)

// SubscribeByTelegram Bot 通过 Telegram 创建求片订阅
func (h *TelegramHandler) SubscribeByTelegram(c *gin.Context) {
	var req telegrampkg.TelegramSubscribeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数错误"})
		return
	}

	// 输入校验
	if !tmdbIDPattern.MatchString(req.TmdbID) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "tmdbId 格式无效"})
		return
	}
	if pp := strings.TrimSpace(req.PosterPath); pp != "" && !posterPathPattern.MatchString(pp) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "posterPath 格式无效"})
		return
	}
	if len(req.Name) > 200 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "name 过长（最多 200 字符）"})
		return
	}

	if err := h.telegramService.SubscribeByTelegram(req); err != nil {
		if errors.Is(err, telegrampkg.ErrTelegramNotBound) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数错误"})
			return
		}
		switch {
		case errors.Is(err, subscriptionpkg.ErrSubscriptionDuplicated):
			c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
		case errors.Is(err, subscriptionpkg.ErrSubscriptionInvalidSeason):
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		default:
			httpx.InternalError(c, err)
		}
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "订阅创建成功"})
}

type pendingRejectEnqueueRequest struct {
	ChatID         int64  `json:"chatId" binding:"required"`
	AdminUserID    string `json:"adminUserId" binding:"required"`
	SubscriptionID string `json:"subscriptionId" binding:"required"`
	MessageID      *int64 `json:"messageId"`
	HasPhoto       bool   `json:"hasPhoto"`
	OriginalText   string `json:"originalText"`
}

type pendingRejectPopRequest struct {
	ChatID int64 `json:"chatId" binding:"required"`
}

type botPollingLockRequest struct {
	OwnerID      string `json:"ownerId" binding:"required"`
	LeaseSeconds int    `json:"leaseSeconds"`
}

// EnqueuePendingReject Internal API: 入队一条拒绝待确认记录
func (h *TelegramHandler) EnqueuePendingReject(c *gin.Context) {
	var req pendingRejectEnqueueRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数错误"})
		return
	}

	if err := telegrampkg.EnqueuePendingReject(c.Request.Context(), req.ChatID, req.AdminUserID, req.SubscriptionID, req.MessageID, req.HasPhoto, req.OriginalText); err != nil {
		httpx.InternalError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "入队成功"})
}

// PopPendingReject Internal API: 弹出并删除最新未过期的拒绝待确认记录
func (h *TelegramHandler) PopPendingReject(c *gin.Context) {
	var req pendingRejectPopRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数错误"})
		return
	}

	record, err := telegrampkg.PopPendingReject(c.Request.Context(), req.ChatID)
	if err != nil {
		httpx.InternalError(c, err)
		return
	}
	if record == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "无待处理的拒绝请求"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"id":             record.ID,
		"chatId":         record.ChatID,
		"adminUserId":    record.AdminUserID,
		"subscriptionId": record.SubscriptionID,
		"messageId":      record.MessageID,
		"hasPhoto":       record.HasPhoto,
		"originalText":   record.OriginalText,
		"expiresAt":      record.ExpiresAt,
	})
}

// AcquirePollingLock Internal API: 申请 polling 单实例租约锁
func (h *TelegramHandler) AcquirePollingLock(c *gin.Context) {
	var req botPollingLockRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数错误"})
		return
	}

	if err := telegrampkg.AcquireBotPollingLock(c.Request.Context(), req.OwnerID, req.LeaseSeconds); err != nil {
		if errors.Is(err, telegrampkg.ErrBotPollingLockHeld) {
			c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
			return
		}
		httpx.InternalError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "锁申请成功"})
}

// RenewPollingLock Internal API: 续租 polling 单实例锁
func (h *TelegramHandler) RenewPollingLock(c *gin.Context) {
	var req botPollingLockRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数错误"})
		return
	}

	if err := telegrampkg.RenewBotPollingLock(c.Request.Context(), req.OwnerID, req.LeaseSeconds); err != nil {
		if errors.Is(err, telegrampkg.ErrBotPollingLockLost) {
			c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
			return
		}
		httpx.InternalError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "锁续租成功"})
}

// ReleasePollingLock Internal API: 释放 polling 单实例锁
func (h *TelegramHandler) ReleasePollingLock(c *gin.Context) {
	var req botPollingLockRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数错误"})
		return
	}

	if err := telegrampkg.ReleaseBotPollingLock(c.Request.Context(), req.OwnerID); err != nil {
		httpx.InternalError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "锁释放成功"})
}
