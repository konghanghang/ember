package middleware

import (
	"net/http"
	"os"

	"github.com/gin-gonic/gin"
)

// InternalAuth 内部服务认证中间件（Bot/API 服务间调用）
func InternalAuth() gin.HandlerFunc {
	secret := os.Getenv("INTERNAL_API_SECRET")

	return func(c *gin.Context) {
		if secret == "" {
			c.JSON(http.StatusServiceUnavailable, gin.H{
				"error": "内部认证未配置",
			})
			c.Abort()
			return
		}

		if c.GetHeader("X-Internal-Secret") != secret {
			c.JSON(http.StatusUnauthorized, gin.H{
				"error": "无效的内部认证",
			})
			c.Abort()
			return
		}

		c.Next()
	}
}
