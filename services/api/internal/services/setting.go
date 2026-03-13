package services

import (
	"encoding/json"
	"slices"
	"strings"

	"github.com/konghang/ember/backend/internal/db"
	"github.com/konghang/ember/backend/internal/models"
)

var allowedStripePaymentMethods = []string{"card", "alipay", "wechat_pay"}

// SettingService 系统配置服务
type SettingService struct{}

// GetSetting 获取配置值
func (s *SettingService) GetSetting(key string) string {
	if db.DB == nil {
		return ""
	}
	var setting models.Setting
	result := db.DB.Where("key = ?", key).First(&setting)
	if result.Error != nil {
		return ""
	}
	return setting.Value
}

func NormalizeStripeAllowedPaymentMethods(raw string) ([]string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}

	var methods []string
	if err := json.Unmarshal([]byte(raw), &methods); err != nil {
		return nil, ErrPaymentMethodSettingInvalid
	}

	normalized := make([]string, 0, len(methods))
	seen := make(map[string]struct{}, len(methods))
	for _, method := range methods {
		method = strings.TrimSpace(method)
		if !slices.Contains(allowedStripePaymentMethods, method) {
			return nil, ErrPaymentMethodSettingInvalid
		}
		if _, exists := seen[method]; exists {
			continue
		}
		seen[method] = struct{}{}
		normalized = append(normalized, method)
	}

	if len(normalized) == 0 {
		return nil, ErrPaymentMethodSettingInvalid
	}

	return normalized, nil
}
