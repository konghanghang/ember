package handlers

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/konghang/ember/backend/internal/common/httpx"
	playbackpkg "github.com/konghang/ember/backend/internal/services/playback"
)

type UserPlaybackProfileHandler struct {
	service *playbackpkg.UserPlaybackProfileService
}

func NewUserPlaybackProfileHandler() *UserPlaybackProfileHandler {
	return &UserPlaybackProfileHandler{
		service: playbackpkg.NewUserPlaybackProfileService(),
	}
}

// GetAdminUserProfiles 获取管理员视角用户画像总览
// GET /api/v1/admin/playback-profiles
func (h *UserPlaybackProfileHandler) GetAdminUserProfiles(c *gin.Context) {
	var query playbackpkg.PlaybackProfileListQuery
	if err := c.ShouldBindQuery(&query); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数错误"})
		return
	}

	resp, err := h.service.GetUserProfilesOverview(c.Request.Context(), query)
	if err != nil {
		switch {
		case errors.Is(err, playbackpkg.ErrPlaybackHistoryInvalidKeyword),
			errors.Is(err, playbackpkg.ErrPlaybackProfileInvalidDate),
			errors.Is(err, playbackpkg.ErrPlaybackProfileInvalidRange),
			errors.Is(err, playbackpkg.ErrPlaybackProfileRangeTooLarge),
			errors.Is(err, playbackpkg.ErrPlaybackProfileOverviewTooLarge):
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		case errors.Is(err, playbackpkg.ErrPlaybackHistoryQueryFailed):
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": playbackpkg.ErrPlaybackHistoryQueryFailed.Error()})
		default:
			httpx.InternalError(c, err)
		}
		return
	}

	c.JSON(http.StatusOK, resp)
}

// GetAdminUserProfile 获取管理员视角用户画像
// GET /api/v1/admin/users/:id/profile
func (h *UserPlaybackProfileHandler) GetAdminUserProfile(c *gin.Context) {
	var query playbackpkg.PlaybackProfileQuery
	if err := c.ShouldBindQuery(&query); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数错误"})
		return
	}

	resp, err := h.service.GetUserProfile(c.Request.Context(), c.Param("id"), query)
	if err != nil {
		switch {
		case errors.Is(err, playbackpkg.ErrPlaybackHistoryInvalidUserID),
			errors.Is(err, playbackpkg.ErrPlaybackProfileInvalidDate),
			errors.Is(err, playbackpkg.ErrPlaybackProfileInvalidRange),
			errors.Is(err, playbackpkg.ErrPlaybackProfileRangeTooLarge):
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		case errors.Is(err, playbackpkg.ErrPlaybackHistoryUserNotFound):
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		case errors.Is(err, playbackpkg.ErrPlaybackHistoryQueryFailed):
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": playbackpkg.ErrPlaybackHistoryQueryFailed.Error()})
		default:
			httpx.InternalError(c, err)
		}
		return
	}

	c.JSON(http.StatusOK, resp)
}

// GetCurrentUserProfile 获取当前登录用户画像
// GET /api/v1/profile/analytics
func (h *UserPlaybackProfileHandler) GetCurrentUserProfile(c *gin.Context) {
	var query playbackpkg.PlaybackProfileQuery
	if err := c.ShouldBindQuery(&query); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数错误"})
		return
	}

	userID, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "未登录"})
		return
	}

	resp, err := h.service.GetUserProfile(c.Request.Context(), userID.(string), query)
	if err != nil {
		switch {
		case errors.Is(err, playbackpkg.ErrPlaybackHistoryInvalidUserID),
			errors.Is(err, playbackpkg.ErrPlaybackProfileInvalidDate),
			errors.Is(err, playbackpkg.ErrPlaybackProfileInvalidRange),
			errors.Is(err, playbackpkg.ErrPlaybackProfileRangeTooLarge):
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		case errors.Is(err, playbackpkg.ErrPlaybackHistoryUserNotFound):
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		case errors.Is(err, playbackpkg.ErrPlaybackHistoryQueryFailed):
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": playbackpkg.ErrPlaybackHistoryQueryFailed.Error()})
		default:
			httpx.InternalError(c, err)
		}
		return
	}

	c.JSON(http.StatusOK, resp)
}
