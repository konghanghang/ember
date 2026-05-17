package middleware

import (
	"crypto/hmac"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/konghang/ember/backend/internal/apiroutes"
	"github.com/konghang/ember/backend/internal/common"
	"github.com/konghang/ember/backend/internal/db"
	"github.com/konghang/ember/backend/internal/models"
)

var passwordResetAllowedPaths = newPasswordResetAllowedPathSet()

func newPasswordResetAllowedPathSet() map[string]struct{} {
	paths := apiroutes.PasswordResetClosedLoopPaths()
	allowed := make(map[string]struct{}, len(paths))
	for _, path := range paths {
		allowed[path] = struct{}{}
	}
	return allowed
}

var loadPasswordResetUser = func(userID string) (*models.User, error) {
	var user models.User
	if err := db.DB.Select("id", "is_active", "role", "password", "password_reset_required").Where("id = ?", userID).First(&user).Error; err != nil {
		return nil, err
	}
	return &user, nil
}

// PasswordResetRequired 是 JWTAuth 之后的会话状态收口中间件：
//   - 每请求回查数据库实际状态，拦截“旧 token 仍存活”造成的越权
//   - 账号被管理员停用（is_active=false）→ 401，强制下线
//   - JWT 内角色与数据库实际角色不一致（被升/降级）→ 401，强制重新登录
//   - 被标记 password_reset_required 的账号只能访问改密闭环相关接口
//
// 仅校验 Ember 账号状态 is_active；不校验 emby_disabled / 过期，
// 过期或 Emby 侧被停用的用户仍可登录控制台续费/兑换。
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

		user, err := loadPasswordResetUser(userID)
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "未认证"})
			c.Abort()
			return
		}

		if !user.IsActive {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "账号已被停用"})
			c.Abort()
			return
		}

		// JWT 内角色与数据库实际角色不一致（管理员被降级 / 普通用户被提权），
		// 旧 token 的 role claim 已不可信，强制重新登录换取新 token。
		roleClaim, _ := c.Get("role")
		if roleStr, ok := roleClaim.(string); !ok || roleStr != user.Role {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "登录状态已失效，请重新登录"})
			c.Abort()
			return
		}

		pwdSigValue, _ := c.Get("pwdSig")
		pwdSig, ok := pwdSigValue.(string)
		if !ok || pwdSig == "" || !hmac.Equal([]byte(pwdSig), []byte(common.ComputePasswordSignature(user.Password))) {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "登录状态已失效，请重新登录"})
			c.Abort()
			return
		}

		principal := AuthPrincipal{
			UserID:   user.ID,
			Role:     user.Role,
			IsActive: user.IsActive,
		}
		c.Set("principal", principal)
		c.Set("role", user.Role)

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
