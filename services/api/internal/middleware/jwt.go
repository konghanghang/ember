package middleware

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/konghang/ember/backend/internal/common"
)

// JWTAuth JWT 认证中间件
func JWTAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		// 1. 从 Header 提取 Token
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			c.JSON(http.StatusUnauthorized, gin.H{
				"error": "缺少 Authorization Header",
			})
			c.Abort()
			return
		}

		// Bearer Token 格式
		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) != 2 || parts[0] != "Bearer" {
			c.JSON(http.StatusUnauthorized, gin.H{
				"error": "Authorization Header 格式错误，应为: Bearer {token}",
			})
			c.Abort()
			return
		}

		tokenString := parts[1]

		// 2. 解析并验证 Token
		claims, err := common.ParseToken(tokenString)
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{
				"error": "Token 无效或已过期",
			})
			c.Abort()
			return
		}

		// 3. 将用户信息存入 Context
		c.Set("userID", claims.UserID)
		c.Set("username", claims.Username)
		c.Set("role", claims.Role)
		c.Set("pwdSig", claims.PwdSig)
		c.Set("claims", claims)

		c.Next()
	}
}

// AuthPrincipal 表示已由 PasswordResetRequired 回查 DB 并校验过的真实会话主体。
type AuthPrincipal struct {
	UserID   string
	Role     string
	IsActive bool
}

func (p AuthPrincipal) IsAdmin() bool {
	return p.Role == "admin"
}

func GetValidatedPrincipal(c *gin.Context) (AuthPrincipal, bool) {
	value, exists := c.Get("principal")
	if !exists {
		return AuthPrincipal{}, false
	}
	principal, ok := value.(AuthPrincipal)
	if !ok || strings.TrimSpace(principal.UserID) == "" || strings.TrimSpace(principal.Role) == "" {
		return AuthPrincipal{}, false
	}
	return principal, true
}

func currentRole(c *gin.Context) (string, bool) {
	if principal, ok := GetValidatedPrincipal(c); ok {
		return principal.Role, true
	}

	role, exists := c.Get("role")
	if !exists {
		return "", false
	}
	roleStr, ok := role.(string)
	if !ok || strings.TrimSpace(roleStr) == "" {
		return "", false
	}
	return roleStr, true
}

// AdminOnly 仅管理员可访问的中间件
// 必须在 JWTAuth 之后使用
func AdminOnly() gin.HandlerFunc {
	return func(c *gin.Context) {
		role, exists := currentRole(c)
		if !exists {
			c.JSON(http.StatusUnauthorized, gin.H{
				"error": "未认证",
			})
			c.Abort()
			return
		}

		if role != "admin" {
			c.JSON(http.StatusForbidden, gin.H{
				"error": "需要管理员权限",
			})
			c.Abort()
			return
		}

		c.Next()
	}
}

// UserOnly 仅用户可访问的中间件
// 必须在 JWTAuth 之后使用
func UserOnly() gin.HandlerFunc {
	return func(c *gin.Context) {
		role, exists := currentRole(c)
		if !exists {
			c.JSON(http.StatusUnauthorized, gin.H{
				"error": "未认证",
			})
			c.Abort()
			return
		}

		if role != "user" {
			c.JSON(http.StatusForbidden, gin.H{
				"error": "需要用户权限",
			})
			c.Abort()
			return
		}

		c.Next()
	}
}
