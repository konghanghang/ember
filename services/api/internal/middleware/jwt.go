package middleware

import (
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/konghang/ember/backend/internal/common"
)

// JWTAuth JWT 认证中间件
func JWTAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		tokenString, err := bearerTokenFromRequest(c)
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
			c.Abort()
			return
		}

		// 2. 解析并验证 Token
		claims, err := common.ParseToken(tokenString)
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{
				"error": "Token 无效或已过期",
			})
			c.Abort()
			return
		}

		setJWTClaims(c, claims)

		c.Next()
	}
}

func bearerTokenFromRequest(c *gin.Context) (string, error) {
	authHeader := c.GetHeader("Authorization")
	if authHeader == "" {
		return "", errors.New("缺少 Authorization Header")
	}

	parts := strings.SplitN(authHeader, " ", 2)
	if len(parts) != 2 || parts[0] != "Bearer" {
		return "", errors.New("Authorization Header 格式错误，应为: Bearer {token}")
	}

	token := strings.TrimSpace(parts[1])
	if token == "" {
		return "", errors.New("Authorization Header 格式错误，应为: Bearer {token}")
	}
	return token, nil
}

func setJWTClaims(c *gin.Context, claims *common.Claims) {
	c.Set("userID", claims.UserID)
	c.Set("username", claims.Username)
	c.Set("role", claims.Role)
	c.Set("pwdSig", claims.PwdSig)
	c.Set("claims", claims)
	c.Set("authType", "jwt")
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
