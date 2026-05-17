package common

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// Claims JWT 声明
type Claims struct {
	UserID   string `json:"userId"`
	Username string `json:"username"`
	Role     string `json:"role"` // "admin" 或 "user"
	PwdSig   string `json:"pwdSig"`
	jwt.RegisteredClaims
}

var jwtSecret []byte

// InitJWT 初始化 JWT 密钥
func InitJWT() error {
	secret := os.Getenv("JWT_SECRET")
	if secret == "" {
		return errors.New("JWT_SECRET 环境变量未设置")
	}
	if len(secret) < 32 {
		return errors.New("JWT_SECRET 长度必须至少 32 个字符")
	}
	jwtSecret = []byte(secret)
	return nil
}

// GenerateToken 生成 JWT Token
// expiresIn: Token 有效期（秒），默认 7 天
func GenerateToken(userID, username, role, pwdSig string, expiresIn ...int64) (string, error) {
	// 默认 7 天过期
	expireTime := int64(7 * 24 * 60 * 60)
	if len(expiresIn) > 0 && expiresIn[0] > 0 {
		expireTime = expiresIn[0]
	}

	claims := Claims{
		UserID:   userID,
		Username: username,
		Role:     role,
		PwdSig:   pwdSig,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Duration(expireTime) * time.Second)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			NotBefore: jwt.NewNumericDate(time.Now()),
			Issuer:    "ember-api",
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(jwtSecret)
}

// ComputePasswordSignature 基于当前密码哈希生成 JWT 可回查签名。
// JWT 中只存服务端 HMAC 派生值，不暴露原始 bcrypt hash。
func ComputePasswordSignature(passwordHash string) string {
	mac := hmac.New(sha256.New, jwtSecret)
	_, _ = mac.Write([]byte(passwordHash))
	sum := mac.Sum(nil)
	return hex.EncodeToString(sum[:12])
}

// ParseToken 解析 JWT Token
func ParseToken(tokenString string) (*Claims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &Claims{}, func(token *jwt.Token) (interface{}, error) {
		return jwtSecret, nil
	})

	if err != nil {
		return nil, err
	}

	if claims, ok := token.Claims.(*Claims); ok && token.Valid {
		return claims, nil
	}

	return nil, errors.New("无效的 Token")
}

// ValidateToken 验证 Token 是否有效（不解析内容）
func ValidateToken(tokenString string) error {
	_, err := ParseToken(tokenString)
	return err
}
