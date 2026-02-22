package services

import (
	"errors"
	"strconv"

	"github.com/konghang/ember/backend/internal/db"
	"github.com/konghang/ember/backend/internal/models"
)

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
	if key != "registration_mode" && key != "default_trial_days" && key != "notify_group_link" && key != "email_verification" {
		return ErrSettingNotFound
	}

	switch key {
	case "registration_mode":
		if value != "open" && value != "invite" {
			return errors.New("无效的注册模式，必须为 open 或 invite")
		}
	case "default_trial_days":
		days, err := strconv.Atoi(value)
		if err != nil || days <= 0 {
			return errors.New("无效的试用天数")
		}
	case "email_verification":
		if value != "true" && value != "false" {
			return errors.New("无效的值，必须为 true 或 false")
		}
	}

	var setting models.Setting
	if err := db.DB.Where("key = ?", key).First(&setting).Error; err != nil {
		return ErrSettingNotFound
	}

	setting.Value = value
	return db.DB.Save(&setting).Error
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
	value := s.GetSetting("default_trial_days")
	if value == "" {
		return 7 // 默认 7 天
	}
	days, err := strconv.Atoi(value)
	if err != nil || days <= 0 {
		return 7
	}
	return days
}

// GetRegistrationMode 获取注册模式
func (s *SettingService) GetRegistrationMode() string {
	value := s.GetSetting("registration_mode")
	if value == "" {
		return "open" // 默认开放注册
	}
	return value
}

// IsEmailVerificationEnabled 检查邮箱验证是否启用
func (s *SettingService) IsEmailVerificationEnabled() bool {
	return s.GetSetting("email_verification") == "true"
}
