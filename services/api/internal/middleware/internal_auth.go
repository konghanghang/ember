package middleware

import (
	"crypto/subtle"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	configpkg "github.com/konghang/ember/backend/internal/config"
)

// InternalAuth 内部服务认证中间件（Bot/API 服务间调用）
func InternalAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		secret := strings.TrimSpace(configpkg.NewConfigService().GetString("INTERNAL_API_SECRET"))
		if secret == "" {
			c.JSON(http.StatusServiceUnavailable, gin.H{
				"error": "内部认证不可用",
			})
			c.Abort()
			return
		}

		reqSecret := strings.TrimSpace(c.GetHeader("X-Internal-Secret"))
		if subtle.ConstantTimeCompare([]byte(reqSecret), []byte(secret)) != 1 {
			c.JSON(http.StatusUnauthorized, gin.H{
				"error": "无效的内部认证",
			})
			c.Abort()
			return
		}

		c.Next()
	}
}
