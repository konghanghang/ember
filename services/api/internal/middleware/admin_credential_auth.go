package middleware

import (
	"log"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/konghang/ember/backend/internal/common"
	"github.com/konghang/ember/backend/internal/services/accessauth"
)

var validateAdminAPIKey = func(apiKey string) (bool, error) {
	return accessauth.NewAdminAPIKeyService().Validate(apiKey)
}

// AdminCredentialAuth 认证管理员路由的 Bearer 凭证。
// JWT 路径保持原有用户状态、角色和密码重置闭环校验；Admin API Key 路径只校验配置表 hash。
func AdminCredentialAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		tokenString, err := bearerTokenFromRequest(c)
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
			c.Abort()
			return
		}

		if accessauth.LooksLikeAdminAPIKey(tokenString) {
			authenticateAdminAPIKey(c, tokenString)
			return
		}

		claims, err := common.ParseToken(tokenString)
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Token 无效或已过期"})
			c.Abort()
			return
		}
		setJWTClaims(c, claims)

		if !validateJWTSessionState(c, true) {
			return
		}

		c.Next()
	}
}

func authenticateAdminAPIKey(c *gin.Context, apiKey string) {
	ok, err := validateAdminAPIKey(apiKey)
	if err != nil {
		log.Printf(
			"[AdminAPIKeyAuth] 配置读取或校验失败: authType=api_key method=%s path=%s clientIP=%s err=%v",
			c.Request.Method,
			c.FullPath(),
			c.ClientIP(),
			err,
		)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "API Key 认证不可用"})
		c.Abort()
		return
	}

	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "无效的管理员凭证"})
		c.Abort()
		return
	}

	principal := AuthPrincipal{
		UserID:   "api_key",
		Role:     "admin",
		IsActive: true,
	}
	c.Set("principal", principal)
	c.Set("userID", principal.UserID)
	c.Set("username", "Admin API Key")
	c.Set("role", principal.Role)
	c.Set("authType", "api_key")

	c.Next()
}
