package handlers

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/konghang/ember/backend/internal/common/httpx"
	devicepkg "github.com/konghang/ember/backend/internal/services/device"
)

type DeviceHandler struct {
	service *devicepkg.DeviceService
}

func NewDeviceHandler() *DeviceHandler {
	return &DeviceHandler{
		service: devicepkg.NewDeviceService(),
	}
}

type AddClientBlacklistRequest struct {
	ClientName string `json:"clientName" binding:"required"`
	Reason     string `json:"reason"`
}

func (h *DeviceHandler) GetDevices(c *gin.Context) {
	var req devicepkg.GetDevicesRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数错误"})
		return
	}

	resp, err := h.service.GetDevices(req)
	if err != nil {
		httpx.InternalError(c, err)
		return
	}

	c.JSON(http.StatusOK, resp)
}

func (h *DeviceHandler) GetBlacklist(c *gin.Context) {
	blacklists, err := h.service.GetBlacklist()
	if err != nil {
		httpx.InternalError(c, err)
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

	operatorID, _ := c.Get("userID")
	if err := h.service.AddClientToBlacklist(req.ClientName, req.Reason, operatorID.(string)); err != nil {
		if errors.Is(err, devicepkg.ErrDeviceClientNameRequired) {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		httpx.InternalError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "添加黑名单成功"})
}

func (h *DeviceHandler) RemoveFromBlacklist(c *gin.Context) {
	clientName := c.Param("clientName")
	operatorID, _ := c.Get("userID")
	if err := h.service.RemoveClientFromBlacklist(clientName, operatorID.(string)); err != nil {
		switch {
		case errors.Is(err, devicepkg.ErrDeviceClientNameRequired):
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		case errors.Is(err, devicepkg.ErrClientBlacklistNotFound):
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		default:
			httpx.InternalError(c, err)
		}
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "移除黑名单成功"})
}

func (h *DeviceHandler) LogoutDevice(c *gin.Context) {
	deviceID := c.Param("deviceId")
	operatorID, _ := c.Get("userID")
	if err := h.service.LogoutDevice(deviceID, operatorID.(string)); err != nil {
		switch {
		case errors.Is(err, devicepkg.ErrDeviceIDRequired):
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		default:
			httpx.InternalError(c, err)
		}
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "设备已强制注销"})
}

func (h *DeviceHandler) LogoutBlacklistedDevices(c *gin.Context) {
	operatorID, _ := c.Get("userID")
	result, err := h.service.LogoutBlacklistedDevices(operatorID.(string))
	if err != nil {
		httpx.InternalError(c, err)
		return
	}

	if len(result.SuccessDeviceIDs) == 0 && len(result.FailedDeviceIDs) > 0 {
		c.JSON(http.StatusBadGateway, gin.H{
			"successDeviceIds": result.SuccessDeviceIDs,
			"failedDeviceIds":  result.FailedDeviceIDs,
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"successDeviceIds": result.SuccessDeviceIDs,
		"failedDeviceIds":  result.FailedDeviceIDs,
	})
}

func (h *DeviceHandler) GetStats(c *gin.Context) {
	stats, err := h.service.GetStats()
	if err != nil {
		httpx.InternalError(c, err)
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
		httpx.InternalError(c, err)
		return
	}

	c.JSON(http.StatusOK, actions)
}
