package handlers

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/konghang/ember/backend/internal/services"
)

type ConfigHandler struct {
	service *services.ConfigService
}

func NewConfigHandler() *ConfigHandler {
	return &ConfigHandler{
		service: services.NewConfigService(),
	}
}

func (h *ConfigHandler) GetConfigs(c *gin.Context) {
	items, err := h.service.List()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "获取配置失败"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": items})
}

func (h *ConfigHandler) UpdateConfig(c *gin.Context) {
	key := c.Param("key")

	var req services.UpdateConfigRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数错误"})
		return
	}

	userID, _ := c.Get("userID")
	updatedByUserID, _ := userID.(string)

	item, err := h.service.Update(key, req, updatedByUserID)
	if err != nil {
		switch {
		case errors.Is(err, services.ErrConfigNotFound):
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		case errors.Is(err, services.ErrConfigNotEditable), errors.Is(err, services.ErrConfigValueRequired), errors.Is(err, services.ErrConfigEncryptionKeyMissing):
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		default:
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
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
		case errors.Is(err, services.ErrConfigGroupUnsupported):
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		default:
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		}
		return
	}

	c.JSON(http.StatusOK, result)
}

func (h *ConfigHandler) ImportEnv(c *gin.Context) {
	userID, _ := c.Get("userID")
	updatedByUserID, _ := userID.(string)

	result, err := h.service.ImportEnv(updatedByUserID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, result)
}
