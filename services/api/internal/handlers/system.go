package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
	systempkg "github.com/konghang/ember/backend/internal/services/system"
)

// SystemHandler 系统处理器
type SystemHandler struct {
	service *systempkg.SystemService
}

// NewSystemHandler 创建系统处理器
func NewSystemHandler() *SystemHandler {
	return &SystemHandler{
		service: systempkg.NewSystemService(),
	}
}

// GetSystemInfo 获取系统信息
// GET /api/v1/system/info
func (h *SystemHandler) GetSystemInfo(c *gin.Context) {
	info, err := h.service.GetSystemInfo()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"info":    info,
	})
}

// TestEmbyConnection 测试 Emby 连接
// POST /api/v1/system/test-emby
func (h *SystemHandler) TestEmbyConnection(c *gin.Context) {
	err := h.service.TestEmbyConnection()
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Emby 服务器连接正常",
	})
}

// CheckExpiredUsers 检查并禁用过期用户
// POST /api/v1/cron/check-expired
func (h *SystemHandler) CheckExpiredUsers(c *gin.Context) {
	result, err := h.service.CheckExpiredUsers()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success":       true,
		"disabledCount": result.DisabledCount,
		"totalExpired":  result.TotalExpired,
		"processed":     result.Processed,
		"errors":        result.Errors,
	})
}
