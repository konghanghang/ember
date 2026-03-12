package services

import (
	"encoding/json"
	"errors"
	"slices"
	"strconv"
	"strings"

	"github.com/konghang/ember/backend/internal/db"
	"github.com/konghang/ember/backend/internal/models"
	"gorm.io/gorm/clause"
)

const (
	settingRegistrationMode           = "registration_mode"
	settingDefaultTrialDays           = "default_trial_days"
	settingNotifyGroupLink            = "notify_group_link"
	settingEmailVerification          = "email_verification"
	settingStripeAllowedPaymentMethod = "stripe_allowed_payment_methods"
)

var allowedStripePaymentMethods = []string{"card", "alipay", "wechat_pay"}

// SettingService 系统配置服务
type SettingService struct{}

// GetSetting 获取配置值
func (s *SettingService) GetSetting(key string) string {
	var setting models.Setting
	result := db.DB.Where("key = ?", key).First(&setting)
	if result.Error != nil {
		return ""
	}
	return setting.Value
}

func (s *SettingService) GetSettingModel(key string) (*models.Setting, error) {
	var setting models.Setting
	if err := db.DB.Where("key = ?", key).First(&setting).Error; err != nil {
		return nil, err
	}
	return &setting, nil
}

// SetSetting 设置配置值（带校验）
func (s *SettingService) SetSetting(key, value string) error {
	switch key {
	case settingRegistrationMode, settingDefaultTrialDays, settingNotifyGroupLink, settingEmailVerification, settingStripeAllowedPaymentMethod:
	default:
		return ErrSettingNotFound
	}

	switch key {
	case settingRegistrationMode:
		if value != "open" && value != "invite" {
			return errors.New("无效的注册模式，必须为 open 或 invite")
		}
	case settingDefaultTrialDays:
		days, err := strconv.Atoi(value)
		if err != nil || days < 0 {
			return errors.New("无效的试用天数")
		}
	case settingEmailVerification:
		if value != "true" && value != "false" {
			return errors.New("无效的值，必须为 true 或 false")
		}
	case settingStripeAllowedPaymentMethod:
		if _, err := NormalizeStripeAllowedPaymentMethods(value); err != nil {
			return err
		}
	}

	setting := models.Setting{
		Key:         key,
		Value:       value,
		IsEncrypted: false,
	}

	return db.DB.Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "key"}},
		DoUpdates: clause.Assignments(map[string]interface{}{
			"value":       value,
			"isEncrypted": false,
			"updatedAt":   clause.Expr{SQL: "CURRENT_TIMESTAMP"},
		}),
	}).Create(&setting).Error
}

// GetAllSettings 获取所有配置
func (s *SettingService) GetAllSettings() ([]models.Setting, error) {
	var settings []models.Setting
	result := db.DB.Find(&settings)
	if result.Error != nil {
		return nil, result.Error
	}
	return settings, nil
}

// GetDefaultTrialDays 获取默认试用天数
func (s *SettingService) GetDefaultTrialDays() int {
	value := s.GetSetting(settingDefaultTrialDays)
	if value == "" {
		return 7 // 默认 7 天
	}
	days, err := strconv.Atoi(value)
	if err != nil || days < 0 {
		return 7
	}
	return days
}

// GetRegistrationMode 获取注册模式
func (s *SettingService) GetRegistrationMode() string {
	value := s.GetSetting(settingRegistrationMode)
	if value == "" {
		return "open" // 默认开放注册
	}
	return value
}

// IsEmailVerificationEnabled 检查邮箱验证是否启用
func (s *SettingService) IsEmailVerificationEnabled() bool {
	return s.GetSetting(settingEmailVerification) == "true"
}

func (s *SettingService) GetStripeAllowedPaymentMethods() ([]string, error) {
	methods, err := NormalizeStripeAllowedPaymentMethods(s.GetSetting(settingStripeAllowedPaymentMethod))
	if err != nil {
		return nil, err
	}
	return methods, nil
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
