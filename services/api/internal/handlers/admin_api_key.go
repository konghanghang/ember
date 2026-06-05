package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/konghang/ember/backend/internal/common/httpx"
	"github.com/konghang/ember/backend/internal/services/accessauth"
)

type adminAPIKeyService interface {
	Status() (*accessauth.AdminAPIKeyStatus, error)
	Generate(updatedByUserID string) (*accessauth.GeneratedAdminAPIKey, error)
	Disable(updatedByUserID string) (*accessauth.AdminAPIKeyStatus, error)
}

type AdminAPIKeyHandler struct {
	service adminAPIKeyService
}

func NewAdminAPIKeyHandler() *AdminAPIKeyHandler {
	return &AdminAPIKeyHandler{
		service: accessauth.NewAdminAPIKeyService(),
	}
}

// GetStatus 返回全局 Admin API Key 的启用状态，不返回明文或 hash。
func (h *AdminAPIKeyHandler) GetStatus(c *gin.Context) {
	if !requireJWTAdminForAPIKeyManagement(c) {
		return
	}

	status, err := h.service.Status()
	if err != nil {
		httpx.InternalError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": status})
}

// Generate 轮换全局 Admin API Key，并只在本次响应中返回明文。
func (h *AdminAPIKeyHandler) Generate(c *gin.Context) {
	if !requireJWTAdminForAPIKeyManagement(c) {
		return
	}

	result, err := h.service.Generate(currentUserID(c))
	if err != nil {
		httpx.InternalError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": result})
}

// Disable 清空全局 Admin API Key hash，使现有 key 立即失效。
func (h *AdminAPIKeyHandler) Disable(c *gin.Context) {
	if !requireJWTAdminForAPIKeyManagement(c) {
		return
	}

	status, err := h.service.Disable(currentUserID(c))
	if err != nil {
		httpx.InternalError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": status})
}

func requireJWTAdminForAPIKeyManagement(c *gin.Context) bool {
	authType, _ := c.Get("authType")
	if authType == "api_key" {
		c.JSON(http.StatusForbidden, gin.H{"error": "API Key 不能管理自身"})
		c.Abort()
		return false
	}
	return true
}

func currentUserID(c *gin.Context) string {
	userID, _ := c.Get("userID")
	value, _ := userID.(string)
	return value
}
