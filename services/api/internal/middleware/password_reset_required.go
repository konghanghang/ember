package middleware

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/konghang/ember/backend/internal/db"
	"github.com/konghang/ember/backend/internal/models"
)

var passwordResetAllowedPaths = map[string]struct{}{
	"/api/v1/profile":       {},
	"/api/v1/password":      {},
	"/api/v1/account-links": {},
	"/api/v1/admin/current": {},
	"/api/v1/user/profile":  {},
	"/api/v1/user/password": {},
}

// PasswordResetRequired 限制被标记为 passwordResetRequired 的账号只能访问改密闭环相关接口。
func PasswordResetRequired() gin.HandlerFunc {
	return func(c *gin.Context) {
		userIDValue, exists := c.Get("userID")
		if !exists {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "未认证"})
			c.Abort()
			return
		}

		userID, ok := userIDValue.(string)
		if !ok || strings.TrimSpace(userID) == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "未认证"})
			c.Abort()
			return
		}

		var user models.User
		if err := db.DB.Select("id", "password_reset_required").Where("id = ?", userID).First(&user).Error; err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "未认证"})
			c.Abort()
			return
		}

		if !user.PasswordResetRequired {
			c.Next()
			return
		}

		if _, ok := passwordResetAllowedPaths[c.FullPath()]; ok {
			c.Next()
			return
		}

		c.JSON(http.StatusForbidden, gin.H{"error": "当前账号必须先修改密码"})
		c.Abort()
	}
}
