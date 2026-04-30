package handlers

import (
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/konghang/ember/backend/internal/common/httpx"
	configpkg "github.com/konghang/ember/backend/internal/config"
)

type configService interface {
	List() ([]configpkg.ConfigItem, error)
	Update(key string, req configpkg.UpdateConfigRequest, updatedByUserID string) (*configpkg.ConfigItem, error)
	TestGroup(group string) (*configpkg.ConfigGroupTestResult, error)
}

type ConfigHandler struct {
	service configService
}

func NewConfigHandler() *ConfigHandler {
	return &ConfigHandler{
		service: configpkg.NewConfigService(),
	}
}

func (h *ConfigHandler) GetConfigs(c *gin.Context) {
	items, err := h.service.List()
	if err != nil {
		httpx.InternalError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": items})
}

func (h *ConfigHandler) UpdateConfig(c *gin.Context) {
	key := c.Param("key")

	var req configpkg.UpdateConfigRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数错误"})
		return
	}

	userID, _ := c.Get("userID")
	updatedByUserID, _ := userID.(string)

	item, err := h.service.Update(key, req, updatedByUserID)
	if err != nil {
		switch {
		case errors.Is(err, configpkg.ErrConfigNotFound):
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		case isConfigUpdateBadRequest(err):
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		default:
			httpx.InternalError(c, err)
		}
		return
	}

	c.JSON(http.StatusOK, item)
}

func (h *ConfigHandler) TestConfigGroup(c *gin.Context) {
	group := c.Param("group")

	result, err := h.service.TestGroup(group)
	if err != nil {
		switch {
		case errors.Is(err, configpkg.ErrConfigGroupUnsupported):
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		default:
			httpx.InternalError(c, err)
		}
		return
	}

	c.JSON(http.StatusOK, result)
}

func isConfigUpdateBadRequest(err error) bool {
	if err == nil {
		return false
	}

	switch {
	case errors.Is(err, configpkg.ErrConfigNotEditable),
		errors.Is(err, configpkg.ErrConfigValueRequired),
		errors.Is(err, configpkg.ErrConfigEncryptionKeyMissing),
		errors.Is(err, configpkg.ErrPaymentMethodSettingInvalid):
		return true
	}

	message := strings.TrimSpace(err.Error())
	if message == "" {
		return false
	}

	if strings.HasPrefix(message, "无效的值，必须为 ") ||
		strings.HasPrefix(message, "数值必须在 ") ||
		strings.HasPrefix(message, "请输入有效的") ||
		strings.HasPrefix(message, "账号资源入口 ") ||
		strings.HasPrefix(message, "第 ") {
		return true
	}

	switch message {
	case "无效的布尔值，必须为 true 或 false",
		"欢迎语模板必须包含 {names} 占位符",
		"Telegram 管理员 Chat ID 不能为空",
		"Telegram 管理员 Chat ID 无效",
		"Telegram 群组 Chat ID 无效",
		"cron 表达式不能为空",
		"cron 表达式无效",
		"时区不能为空",
		"时区无效",
		"发件人格式无效",
		"控制台账号资源入口必须是 JSON 数组",
		"Turnstile Site Key 不能包含空白字符",
		"请输入有效的主机名",
		"主机名不支持端口":
		return true
	default:
		return false
	}
}
