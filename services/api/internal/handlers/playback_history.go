package handlers

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	playbackpkg "github.com/konghang/ember/backend/internal/services/playback"
)

type PlaybackHistoryHandler struct {
	service *playbackpkg.PlaybackHistoryService
}

func NewPlaybackHistoryHandler() *PlaybackHistoryHandler {
	return &PlaybackHistoryHandler{
		service: playbackpkg.NewPlaybackHistoryService(),
	}
}

// GetPlaybackHistory 分页查询播放历史（管理员）
// GET /api/v1/admin/playback-history
func (h *PlaybackHistoryHandler) GetPlaybackHistory(c *gin.Context) {
	var req playbackpkg.PlaybackHistoryRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数错误"})
		return
	}

	resp, err := h.service.GetPlaybackHistory(c.Request.Context(), req)
	if err != nil {
		switch {
		case errors.Is(err, playbackpkg.ErrPlaybackHistoryInvalidDate),
			errors.Is(err, playbackpkg.ErrPlaybackHistoryInvalidKeyword),
			errors.Is(err, playbackpkg.ErrPlaybackHistoryInvalidUserID),
			errors.Is(err, playbackpkg.ErrPlaybackHistoryUserNotFound):
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		case errors.Is(err, playbackpkg.ErrPlaybackHistoryQueryFailed):
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": playbackpkg.ErrPlaybackHistoryQueryFailed.Error()})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"error": "内部服务错误"})
		}
		return
	}

	c.JSON(http.StatusOK, resp)
}
