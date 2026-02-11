package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/konghang/ember/backend/internal/services"
)

// MediaHandler 媒体处理器
type MediaHandler struct {
	service *services.MediaService
}

// NewMediaHandler 创建媒体处理器
func NewMediaHandler() *MediaHandler {
	return &MediaHandler{
		service: services.NewMediaService(),
	}
}

// GetEmbyConfig 获取 Emby 配置
// GET /api/v1/emby/config
func (h *MediaHandler) GetEmbyConfig(c *gin.Context) {
	url, err := h.service.GetEmbyConfig()
	if err != nil || url == "" {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "未配置 Emby 服务器地址",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"url":     url,
	})
}

// GetMediaStats 获取媒体库统计信息（带缓存）
// GET /api/v1/media/stats
func (h *MediaHandler) GetMediaStats(c *gin.Context) {
	stats, err := h.service.GetMediaStats()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    stats,
	})
}
