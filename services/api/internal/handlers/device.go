package handlers

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/konghang/ember/backend/internal/services"
)

type DeviceHandler struct {
	service *services.DeviceService
}

func NewDeviceHandler() *DeviceHandler {
	return &DeviceHandler{
		service: services.NewDeviceService(),
	}
}

type AddClientBlacklistRequest struct {
	ClientName string `json:"clientName" binding:"required"`
	Reason     string `json:"reason"`
}

func (h *DeviceHandler) GetDevices(c *gin.Context) {
	var req services.GetDevicesRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数错误"})
		return
	}

	resp, err := h.service.GetDevices(req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, resp)
}

func (h *DeviceHandler) GetBlacklist(c *gin.Context) {
	blacklists, err := h.service.GetBlacklist()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": blacklists})
}

func (h *DeviceHandler) AddToBlacklist(c *gin.Context) {
	var req AddClientBlacklistRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数错误"})
		return
	}

	if err := h.service.AddClientToBlacklist(req.ClientName, req.Reason); err != nil {
		if errors.Is(err, services.ErrDeviceClientNameRequired) {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "添加黑名单成功"})
}

func (h *DeviceHandler) RemoveFromBlacklist(c *gin.Context) {
	clientName := c.Param("clientName")
	if err := h.service.RemoveClientFromBlacklist(clientName); err != nil {
		switch {
		case errors.Is(err, services.ErrDeviceClientNameRequired):
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		case errors.Is(err, services.ErrClientBlacklistNotFound):
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		}
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "移除黑名单成功"})
}

func (h *DeviceHandler) LogoutDevice(c *gin.Context) {
	deviceID := c.Param("deviceId")
	if err := h.service.LogoutDevice(deviceID); err != nil {
		switch {
		case errors.Is(err, services.ErrDeviceIDRequired):
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		}
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "设备已强制注销"})
}

func (h *DeviceHandler) LogoutBlacklistedDevices(c *gin.Context) {
	count, err := h.service.LogoutBlacklistedDevices()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": err.Error(),
			"count": count,
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "黑名单设备已批量注销",
		"count":   count,
	})
}

func (h *DeviceHandler) GetStats(c *gin.Context) {
	stats, err := h.service.GetStats()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": stats})
}

func (h *DeviceHandler) GetActions(c *gin.Context) {
	var req struct {
		Limit int `form:"limit" binding:"omitempty,min=1"`
	}
	if err := c.ShouldBindQuery(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数错误"})
		return
	}

	actions, err := h.service.GetDeviceActions(req.Limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, actions)
}
