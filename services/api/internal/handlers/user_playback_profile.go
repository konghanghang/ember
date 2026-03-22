package handlers

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/konghang/ember/backend/internal/services"
)

type UserPlaybackProfileHandler struct {
	service *services.UserPlaybackProfileService
}

func NewUserPlaybackProfileHandler() *UserPlaybackProfileHandler {
	return &UserPlaybackProfileHandler{
		service: services.NewUserPlaybackProfileService(),
	}
}

// GetAdminUserProfile 获取管理员视角用户画像
// GET /api/v1/admin/users/:id/profile
func (h *UserPlaybackProfileHandler) GetAdminUserProfile(c *gin.Context) {
	var query services.PlaybackProfileQuery
	if err := c.ShouldBindQuery(&query); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数错误"})
		return
	}

	resp, err := h.service.GetUserProfile(c.Request.Context(), c.Param("id"), query)
	if err != nil {
		switch {
		case errors.Is(err, services.ErrPlaybackHistoryInvalidUserID), errors.Is(err, services.ErrPlaybackProfileInvalidRange):
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		case errors.Is(err, services.ErrPlaybackHistoryUserNotFound):
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		case errors.Is(err, services.ErrPlaybackHistoryQueryFailed):
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": services.ErrPlaybackHistoryQueryFailed.Error()})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"error": "内部服务错误"})
		}
		return
	}

	c.JSON(http.StatusOK, resp)
}

// GetCurrentUserProfile 获取当前登录用户画像
// GET /api/v1/profile/analytics
func (h *UserPlaybackProfileHandler) GetCurrentUserProfile(c *gin.Context) {
	var query services.PlaybackProfileQuery
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
		case errors.Is(err, services.ErrPlaybackHistoryInvalidUserID), errors.Is(err, services.ErrPlaybackProfileInvalidRange):
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		case errors.Is(err, services.ErrPlaybackHistoryUserNotFound):
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		case errors.Is(err, services.ErrPlaybackHistoryQueryFailed):
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": services.ErrPlaybackHistoryQueryFailed.Error()})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"error": "内部服务错误"})
		}
		return
	}

	c.JSON(http.StatusOK, resp)
}
