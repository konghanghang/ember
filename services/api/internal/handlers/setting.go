package handlers

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/konghang/ember/backend/internal/common/httpx"
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

	// 注册邮箱域名白名单：始终返回数组（空白名单 → []），前端按数组消费，
	// 不要 omitempty 让它在“无限制”时变成 null。
	domains := h.configService.GetRegistrationAllowedEmailDomains()
	if domains == nil {
		domains = []string{}
	}
	resp["allowedEmailDomains"] = domains

	c.JSON(http.StatusOK, resp)
}

func (h *SettingHandler) GetConsoleAccountLinks(c *gin.Context) {
	links, err := h.configService.GetConsoleAccountLinks()
	if err != nil {
		httpx.InternalError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": links})
}

// GetLoginProtectionConfig 获取登录页公开配置
// GET /api/v1/login/protection-config
func (h *SettingHandler) GetLoginProtectionConfig(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"turnstileLoginEnabled":     h.configService.IsTurnstileLoginEnabled(),
		"turnstileSiteKey":          h.configService.GetTurnstileSiteKey(),
		"turnstileExpectedHostname": h.configService.GetTurnstileExpectedHostname(),
	})
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
		httpx.InternalError(c, err)
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
