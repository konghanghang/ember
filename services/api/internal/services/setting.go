package services

import (
	"encoding/json"
	"slices"
	"strings"

	"github.com/konghang/ember/backend/internal/db"
	"github.com/konghang/ember/backend/internal/models"
)

const (
	settingRegistrationMode           = "registration_mode"
	settingDefaultTrialDays           = "default_trial_days"
	settingNotifyGroupLink            = "notify_group_link"
	settingEmailVerification          = "email_verification"
	settingStripeAllowedPaymentMethod = "stripe_allowed_payment_methods"
)

var allowedStripePaymentMethods = []string{"card", "alipay", "wechat_pay"}

var legacySettingKeys = []string{
	settingRegistrationMode,
	settingDefaultTrialDays,
	settingNotifyGroupLink,
	settingEmailVerification,
	settingStripeAllowedPaymentMethod,
}

func isLegacySettingKey(key string) bool {
	return slices.Contains(legacySettingKeys, key)
}

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

func (s *SettingService) GetSettingModel(key string) (*models.Setting, error) {
	if !isLegacySettingKey(key) {
		return nil, ErrSettingNotFound
	}

	configItem, err := NewConfigService().Get(key)
	if err != nil {
		return nil, err
	}

	value := ""
	if configItem.Value != nil {
		value = *configItem.Value
	}

	setting := &models.Setting{
		Key:         key,
		Value:       value,
		IsEncrypted: false,
	}

	if db.DB == nil {
		return setting, nil
	}

	var stored models.Setting
	if err := db.DB.Where("key = ?", key).First(&stored).Error; err == nil {
		setting.UpdatedAt = stored.UpdatedAt
		setting.UpdatedByUserID = stored.UpdatedByUserID
	}

	return setting, nil
}

// SetSetting 设置配置值（带校验）
func (s *SettingService) SetSetting(key, value string) error {
	if !isLegacySettingKey(key) {
		return ErrSettingNotFound
	}

	_, err := NewConfigService().Update(key, UpdateConfigRequest{Value: &value}, "")
	return err
}

// GetAllSettings 获取所有配置
func (s *SettingService) GetAllSettings() ([]models.Setting, error) {
	settings := make([]models.Setting, 0, len(legacySettingKeys))

	for _, key := range legacySettingKeys {
		setting, err := s.GetSettingModel(key)
		if err != nil {
			return nil, err
		}
		if setting == nil {
			continue
		}
		settings = append(settings, *setting)
	}

	return settings, nil
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
