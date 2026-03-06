package handlers

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/konghang/ember/backend/internal/services"
)

type PlaybackHistoryHandler struct {
	service *services.PlaybackHistoryService
}

func NewPlaybackHistoryHandler() *PlaybackHistoryHandler {
	return &PlaybackHistoryHandler{
		service: services.NewPlaybackHistoryService(),
	}
}

// GetPlaybackHistory 分页查询播放历史（管理员）
// GET /api/v1/admin/playback-history
func (h *PlaybackHistoryHandler) GetPlaybackHistory(c *gin.Context) {
	var req services.PlaybackHistoryRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数错误"})
		return
	}

	resp, err := h.service.GetPlaybackHistory(c.Request.Context(), req)
	if err != nil {
		switch {
		case errors.Is(err, services.ErrPlaybackHistoryInvalidDate),
			errors.Is(err, services.ErrPlaybackHistoryInvalidKeyword),
			errors.Is(err, services.ErrPlaybackHistoryInvalidUserID),
			errors.Is(err, services.ErrPlaybackHistoryUserNotFound):
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		case errors.Is(err, services.ErrPlaybackHistoryQueryFailed):
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": services.ErrPlaybackHistoryQueryFailed.Error()})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"error": "内部服务错误"})
		}
		return
	}

	c.JSON(http.StatusOK, resp)
}
