package common

import (
	"errors"
	"os"
	"strings"
)

const minInternalAPISecretLength = 32

var (
	internalAPISecret             string
	internalAPISecretPlaceholders = map[string]struct{}{
		"your-internal-api-secret": {},
	}
)

// InitInternalAPISecret validates and caches the API/Bot internal trust root.
func InitInternalAPISecret() error {
	secret := strings.TrimSpace(os.Getenv("INTERNAL_API_SECRET"))
	if err := ValidateInternalAPISecret(secret); err != nil {
		return err
	}
	internalAPISecret = secret
	return nil
}

func InternalAPISecret() string {
	return internalAPISecret
}

func ValidateInternalAPISecret(secret string) error {
	secret = strings.TrimSpace(secret)
	if secret == "" {
		return errors.New("INTERNAL_API_SECRET 环境变量未设置")
	}
	if _, ok := internalAPISecretPlaceholders[strings.ToLower(secret)]; ok {
		return errors.New("INTERNAL_API_SECRET 不能使用 .env.example 中的占位值")
	}
	if len(secret) < minInternalAPISecretLength {
		return errors.New("INTERNAL_API_SECRET 长度必须至少 32 个字符")
	}
	return nil
}
