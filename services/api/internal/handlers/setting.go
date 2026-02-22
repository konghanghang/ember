package handlers

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/konghang/ember/backend/internal/services"
)

type SettingHandler struct {
	service *services.SettingService
}

type updateSettingRequest struct {
	Value string `json:"value" binding:"required"`
}

func NewSettingHandler() *SettingHandler {
	return &SettingHandler{service: &services.SettingService{}}
}

func (h *SettingHandler) GetSettings(c *gin.Context) {
	settings, err := h.service.GetAllSettings()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "获取配置失败"})
		return
	}

	c.JSON(http.StatusOK, settings)
}

func (h *SettingHandler) UpdateSetting(c *gin.Context) {
	key := c.Param("key")
	var req updateSettingRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数错误"})
		return
	}

	if err := h.service.SetSetting(key, req.Value); err != nil {
		if errors.Is(err, services.ErrSettingNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	setting, err := h.service.GetSettingModel(key)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "获取配置失败"})
		return
	}

	c.JSON(http.StatusOK, setting)
}

func (h *SettingHandler) GetRegistrationMode(c *gin.Context) {
	mode := h.service.GetRegistrationMode()
	resp := gin.H{"mode": mode}
	if mode == "open" {
		resp["defaultTrialDays"] = h.service.GetDefaultTrialDays()
	}
	emailService := services.NewEmailService()
	resp["emailVerification"] = emailService.IsEnabled()
	c.JSON(http.StatusOK, resp)
}

// GetSettingByKey 获取单个配置值（内部服务调用）
// GET /api/v1/internal/settings/:key
func (h *SettingHandler) GetSettingByKey(c *gin.Context) {
	key := c.Param("key")
	value := h.service.GetSetting(key)
	c.JSON(http.StatusOK, gin.H{"key": key, "value": value})
}
