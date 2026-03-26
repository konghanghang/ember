package handlers

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	configpkg "github.com/konghang/ember/backend/internal/config"
	emailpkg "github.com/konghang/ember/backend/internal/services/email"
)

type SettingHandler struct {
	configService *configpkg.ConfigService
}

func NewSettingHandler() *SettingHandler {
	return &SettingHandler{
		configService: configpkg.NewConfigService(),
	}
}

func (h *SettingHandler) GetRegistrationMode(c *gin.Context) {
	mode := h.configService.GetRegistrationMode()
	resp := gin.H{"mode": mode}
	if mode == "open" {
		resp["defaultTrialDays"] = h.configService.GetDefaultTrialDays()
	}
	emailService := emailpkg.NewEmailService()
	resp["emailVerification"] = emailService.IsEnabled()
	c.JSON(http.StatusOK, resp)
}

func (h *SettingHandler) GetConsoleAccountLinks(c *gin.Context) {
	links, err := h.configService.GetConsoleAccountLinks()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "获取控制台账号资源入口失败"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": links})
}

// GetSettingByKey 获取单个配置值（内部服务调用）
// GET /api/v1/internal/settings/:key
func (h *SettingHandler) GetSettingByKey(c *gin.Context) {
	key := c.Param("key")

	item, err := h.configService.Get(key)
	if err != nil {
		if errors.Is(err, configpkg.ErrConfigNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "获取配置失败"})
		return
	}

	if item.Sensitive {
		c.JSON(http.StatusForbidden, gin.H{"error": configpkg.ErrConfigSensitiveReadForbidden.Error()})
		return
	}

	value := ""
	if item.Value != nil {
		value = *item.Value
	}
	c.JSON(http.StatusOK, gin.H{"key": key, "value": value, "source": item.Source, "hasValue": item.HasValue})
}
