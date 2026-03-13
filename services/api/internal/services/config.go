package services

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/mail"
	neturl "net/url"
	"os"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/konghang/ember/backend/internal/db"
	"github.com/konghang/ember/backend/internal/models"
	"github.com/robfig/cron/v3"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type ConfigValueType string

const (
	ConfigValueString   ConfigValueType = "string"
	ConfigValueSecret   ConfigValueType = "secret"
	ConfigValueBoolean  ConfigValueType = "boolean"
	ConfigValueInteger  ConfigValueType = "integer"
	ConfigValueURL      ConfigValueType = "url"
	ConfigValueEnum     ConfigValueType = "enum"
	ConfigValueJSONList ConfigValueType = "json_list"
)

const (
	ConfigGroupBusiness     = "business"
	ConfigGroupMedia        = "media"
	ConfigGroupEmail        = "email"
	ConfigGroupPayment      = "payment"
	ConfigGroupNotification = "notification"
	ConfigGroupSchedule     = "schedule"
	ConfigGroupDeployment   = "deployment"
)

const (
	ConfigSourceDatabase = "database"
	ConfigSourceEnv      = "env"
	ConfigSourceDefault  = "default"
	ConfigSourceUnset    = "unset"
)

const defaultCronTimezone = "Asia/Shanghai"

type ConfigOption struct {
	Label string `json:"label"`
	Value string `json:"value"`
}

type ConfigDefinition struct {
	Key             string
	Group           string
	GroupLabel      string
	Label           string
	Description     string
	Type            ConfigValueType
	DefaultValue    string
	Placeholder     string
	Editable        bool
	Sensitive       bool
	RestartRequired bool
	AllowEmpty      bool
	EnvKey          string
	Options         []ConfigOption
	Validate        func(string) error
	Normalize       func(string) (string, error)
}

type ConfigItem struct {
	Key             string          `json:"key"`
	Group           string          `json:"group"`
	GroupLabel      string          `json:"groupLabel"`
	Label           string          `json:"label"`
	Description     string          `json:"description"`
	Type            ConfigValueType `json:"type"`
	Placeholder     string          `json:"placeholder,omitempty"`
	Editable        bool            `json:"editable"`
	Sensitive       bool            `json:"sensitive"`
	RestartRequired bool            `json:"restartRequired"`
	Options         []ConfigOption  `json:"options,omitempty"`
	Source          string          `json:"source"`
	HasValue        bool            `json:"hasValue"`
	Value           *string         `json:"value,omitempty"`
	Error           string          `json:"error,omitempty"`
}

type UpdateConfigRequest struct {
	Value *string `json:"value"`
	Clear bool    `json:"clear"`
}

type ConfigGroupTestDetail struct {
	Target  string `json:"target"`
	Success bool   `json:"success"`
	Message string `json:"message"`
}

type ConfigGroupTestResult struct {
	Success bool                    `json:"success"`
	Message string                  `json:"message"`
	Details []ConfigGroupTestDetail `json:"details"`
}

type ImportEnvResult struct {
	Imported []string          `json:"imported"`
	Skipped  map[string]string `json:"skipped"`
	Failed   map[string]string `json:"failed"`
}

type ConfigService struct {
	encryptionKey string
}

func NewConfigService() *ConfigService {
	return &ConfigService{
		encryptionKey: strings.TrimSpace(os.Getenv("CONFIG_ENCRYPTION_KEY")),
	}
}

func (s *ConfigService) GetString(key string) string {
	value, _, err := s.ResolveString(key)
	if err != nil {
		return ""
	}
	return value
}

func (s *ConfigService) GetRegistrationMode() string {
	value := s.GetString("registration_mode")
	if value == "" {
		return "open"
	}
	return value
}

func (s *ConfigService) GetDefaultTrialDays() int {
	value := s.GetString("default_trial_days")
	if value == "" {
		return 7
	}
	days, err := strconv.Atoi(value)
	if err != nil || days < 0 {
		return 7
	}
	return days
}

func (s *ConfigService) IsEmailVerificationEnabled() bool {
	return s.GetString("email_verification") == "true"
}

func (s *ConfigService) GetStripeAllowedPaymentMethods() ([]string, error) {
	return NormalizeStripeAllowedPaymentMethods(s.GetString("stripe_allowed_payment_methods"))
}

func (s *ConfigService) ResolveString(key string) (string, string, error) {
	def, ok := getConfigDefinitionMap()[key]
	if !ok {
		return "", ConfigSourceUnset, ErrConfigNotFound
	}

	item, err := s.resolveDefinition(def, nil)
	if err != nil {
		return "", ConfigSourceUnset, err
	}
	if item.Error != "" {
		return "", item.Source, errors.New(item.Error)
	}
	if item.Value == nil {
		return "", item.Source, nil
	}
	return *item.Value, item.Source, nil
}

func (s *ConfigService) List() ([]ConfigItem, error) {
	definitions := getConfigDefinitions()
	settingsMap, err := s.loadSettings(definitions)
	if err != nil {
		return nil, err
	}

	items := make([]ConfigItem, 0, len(definitions))
	for _, def := range definitions {
		item, resolveErr := s.resolveDefinition(def, settingsMap)
		if resolveErr != nil {
			return nil, resolveErr
		}
		items = append(items, item)
	}

	return items, nil
}

func (s *ConfigService) Get(key string) (*ConfigItem, error) {
	def, ok := getConfigDefinitionMap()[key]
	if !ok {
		return nil, ErrConfigNotFound
	}

	item, err := s.resolveDefinition(def, nil)
	if err != nil {
		return nil, err
	}
	return &item, nil
}

func (s *ConfigService) Update(key string, req UpdateConfigRequest, updatedByUserID string) (*ConfigItem, error) {
	def, ok := getConfigDefinitionMap()[key]
	if !ok {
		return nil, ErrConfigNotFound
	}
	if !def.Editable {
		return nil, ErrConfigNotEditable
	}

	if req.Clear {
		if err := db.DB.Where("key = ?", key).Delete(&models.Setting{}).Error; err != nil {
			return nil, err
		}
		return s.Get(key)
	}

	if req.Value == nil {
		return nil, ErrConfigValueRequired
	}

	value := *req.Value
	if def.Normalize != nil {
		normalized, err := def.Normalize(value)
		if err != nil {
			return nil, err
		}
		value = normalized
	}

	if def.Validate != nil {
		if err := def.Validate(value); err != nil {
			return nil, err
		}
	}

	setting := models.Setting{
		Key:             key,
		Value:           value,
		IsEncrypted:     false,
		UpdatedByUserID: nil,
	}

	if updatedByUserID != "" {
		setting.UpdatedByUserID = &updatedByUserID
	}

	if def.Sensitive {
		if s.encryptionKey == "" {
			return nil, ErrConfigEncryptionKeyMissing
		}
		encrypted, err := s.encrypt(value)
		if err != nil {
			return nil, err
		}
		setting.Value = encrypted
		setting.IsEncrypted = true
	}

	if err := db.DB.Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "key"}},
		DoUpdates: clause.Assignments(map[string]interface{}{
			"value":           setting.Value,
			"isEncrypted":     setting.IsEncrypted,
			"updatedByUserId": setting.UpdatedByUserID,
			"updatedAt":       gorm.Expr("CURRENT_TIMESTAMP"),
		}),
	}).Create(&setting).Error; err != nil {
		return nil, err
	}

	return s.Get(key)
}

func (s *ConfigService) ImportEnv(updatedByUserID string) (*ImportEnvResult, error) {
	definitions := getConfigDefinitions()
	settingsMap, err := s.loadSettings(definitions)
	if err != nil {
		return nil, err
	}

	result := &ImportEnvResult{
		Imported: make([]string, 0),
		Skipped:  make(map[string]string),
		Failed:   make(map[string]string),
	}

	for _, def := range definitions {
		if !def.Editable || def.EnvKey == "" {
			continue
		}
		if existing, ok := settingsMap[def.Key]; ok && s.shouldUseDatabaseValue(def, existing) {
			result.Skipped[def.Key] = "数据库已存在覆盖值"
			continue
		}

		envValue := strings.TrimSpace(os.Getenv(def.EnvKey))
		if envValue == "" {
			result.Skipped[def.Key] = "环境变量未设置"
			continue
		}

		value := envValue
		if def.Normalize != nil {
			normalized, normalizeErr := def.Normalize(value)
			if normalizeErr != nil {
				result.Failed[def.Key] = normalizeErr.Error()
				continue
			}
			value = normalized
		}
		if def.Validate != nil {
			if validateErr := def.Validate(value); validateErr != nil {
				result.Failed[def.Key] = validateErr.Error()
				continue
			}
		}

		if _, updateErr := s.Update(def.Key, UpdateConfigRequest{Value: &value}, updatedByUserID); updateErr != nil {
			result.Failed[def.Key] = updateErr.Error()
			continue
		}

		result.Imported = append(result.Imported, def.Key)
	}

	return result, nil
}

func (s *ConfigService) TestGroup(group string) (*ConfigGroupTestResult, error) {
	switch group {
	case ConfigGroupMedia:
		return s.testMediaGroup(), nil
	case ConfigGroupEmail:
		return s.testEmailGroup(), nil
	default:
		return nil, ErrConfigGroupUnsupported
	}
}

func (s *ConfigService) testMediaGroup() *ConfigGroupTestResult {
	result := &ConfigGroupTestResult{
		Details: make([]ConfigGroupTestDetail, 0, 2),
	}

	embyService := NewEmbyService()
	if err := embyService.TestConnection(); err != nil {
		result.Details = append(result.Details, ConfigGroupTestDetail{
			Target:  "emby",
			Success: false,
			Message: err.Error(),
		})
	} else {
		result.Details = append(result.Details, ConfigGroupTestDetail{
			Target:  "emby",
			Success: true,
			Message: "连接成功",
		})
	}

	moviePilot := NewMoviePilotClient()
	if moviePilot.IsConfigured() {
		if err := moviePilot.TestConnection(); err != nil {
			result.Details = append(result.Details, ConfigGroupTestDetail{
				Target:  "moviepilot",
				Success: false,
				Message: err.Error(),
			})
		} else {
			result.Details = append(result.Details, ConfigGroupTestDetail{
				Target:  "moviepilot",
				Success: true,
				Message: "连接成功",
			})
		}
	} else {
		result.Details = append(result.Details, ConfigGroupTestDetail{
			Target:  "moviepilot",
			Success: true,
			Message: "未配置，已跳过",
		})
	}

	result.Success = true
	result.Message = "媒体配置检查通过"
	for _, detail := range result.Details {
		if !detail.Success && detail.Target != "moviepilot" {
			result.Success = false
			result.Message = "媒体配置检查失败"
			break
		}
	}

	return result
}

func (s *ConfigService) testEmailGroup() *ConfigGroupTestResult {
	result := &ConfigGroupTestResult{
		Details: make([]ConfigGroupTestDetail, 0, 1),
	}

	emailService := NewEmailService()
	if err := emailService.TestConnection(); err != nil {
		result.Success = false
		result.Message = "邮件配置检查失败"
		result.Details = append(result.Details, ConfigGroupTestDetail{
			Target:  "smtp",
			Success: false,
			Message: err.Error(),
		})
		return result
	}

	result.Success = true
	result.Message = "邮件配置检查通过"
	result.Details = append(result.Details, ConfigGroupTestDetail{
		Target:  "smtp",
		Success: true,
		Message: "连接成功",
	})

	return result
}

func (s *ConfigService) resolveDefinition(def ConfigDefinition, settingsMap map[string]models.Setting) (ConfigItem, error) {
	item := ConfigItem{
		Key:             def.Key,
		Group:           def.Group,
		GroupLabel:      def.GroupLabel,
		Label:           def.Label,
		Description:     def.Description,
		Type:            def.Type,
		Placeholder:     def.Placeholder,
		Editable:        def.Editable,
		Sensitive:       def.Sensitive,
		RestartRequired: def.RestartRequired,
		Options:         def.Options,
		Source:          ConfigSourceUnset,
	}

	if settingsMap == nil {
		loadedSettings, err := s.loadSettings([]ConfigDefinition{def})
		if err != nil {
			return item, err
		}
		settingsMap = loadedSettings
	}

	if stored, ok := settingsMap[def.Key]; ok && s.shouldUseDatabaseValue(def, stored) {
		resolved, err := s.decodeStoredValue(def, stored)
		item.Source = ConfigSourceDatabase
		if err != nil {
			item.Error = err.Error()
			return item, nil
		}
		return s.applyResolvedValue(item, def, resolved, ConfigSourceDatabase), nil
	}

	if def.EnvKey != "" {
		if envValue, ok := s.resolveEnvValue(def); ok {
			return s.applyResolvedValue(item, def, envValue, ConfigSourceEnv), nil
		}
	}

	if def.DefaultValue != "" || def.AllowEmpty {
		return s.applyResolvedValue(item, def, def.DefaultValue, ConfigSourceDefault), nil
	}

	item.Source = ConfigSourceUnset
	item.HasValue = false
	return item, nil
}

func (s *ConfigService) applyResolvedValue(item ConfigItem, def ConfigDefinition, raw string, source string) ConfigItem {
	item.Source = source
	item.HasValue = raw != ""
	if def.Sensitive {
		return item
	}
	value := raw
	item.Value = &value
	return item
}

func (s *ConfigService) decodeStoredValue(def ConfigDefinition, setting models.Setting) (string, error) {
	if !def.Sensitive {
		return setting.Value, nil
	}
	if !setting.IsEncrypted {
		return setting.Value, nil
	}
	if s.encryptionKey == "" {
		return "", ErrConfigEncryptionKeyMissing
	}
	return s.decrypt(setting.Value)
}

func (s *ConfigService) shouldUseDatabaseValue(def ConfigDefinition, setting models.Setting) bool {
	if def.AllowEmpty {
		return true
	}
	if def.Sensitive {
		return strings.TrimSpace(setting.Value) != ""
	}
	return strings.TrimSpace(setting.Value) != ""
}

func (s *ConfigService) resolveEnvValue(def ConfigDefinition) (string, bool) {
	raw := os.Getenv(def.EnvKey)
	if raw == "" {
		return "", false
	}
	if def.Sensitive {
		return raw, true
	}
	if def.Normalize == nil {
		return raw, true
	}
	normalized, err := def.Normalize(raw)
	if err != nil {
		return raw, true
	}
	return normalized, true
}

func (s *ConfigService) loadSettings(definitions []ConfigDefinition) (map[string]models.Setting, error) {
	if db.DB == nil {
		return map[string]models.Setting{}, nil
	}

	keys := make([]string, 0, len(definitions))
	for _, def := range definitions {
		keys = append(keys, def.Key)
	}

	var settings []models.Setting
	if err := db.DB.Where("key IN ?", keys).Find(&settings).Error; err != nil {
		return nil, err
	}

	result := make(map[string]models.Setting, len(settings))
	for _, setting := range settings {
		result[setting.Key] = setting
	}
	return result, nil
}

func (s *ConfigService) encrypt(plain string) (string, error) {
	block, err := aes.NewCipher(hashEncryptionKey(s.encryptionKey))
	if err != nil {
		return "", err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}

	ciphertext := gcm.Seal(nil, nonce, []byte(plain), nil)
	return base64.StdEncoding.EncodeToString(append(nonce, ciphertext...)), nil
}

func (s *ConfigService) decrypt(encoded string) (string, error) {
	payload, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return "", errors.New("配置解密失败")
	}

	block, err := aes.NewCipher(hashEncryptionKey(s.encryptionKey))
	if err != nil {
		return "", errors.New("配置解密失败")
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", errors.New("配置解密失败")
	}

	nonceSize := gcm.NonceSize()
	if len(payload) < nonceSize {
		return "", errors.New("配置解密失败")
	}

	nonce := payload[:nonceSize]
	ciphertext := payload[nonceSize:]
	plain, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", errors.New("配置解密失败")
	}

	return string(plain), nil
}

func hashEncryptionKey(raw string) []byte {
	sum := sha256.Sum256([]byte(raw))
	return sum[:]
}

func getConfigDefinitions() []ConfigDefinition {
	return []ConfigDefinition{
		{
			Key:          "registration_mode",
			Group:        ConfigGroupBusiness,
			GroupLabel:   "基础业务",
			Label:        "注册模式",
			Description:  "控制新用户是开放注册还是仅限邀请码注册",
			Type:         ConfigValueEnum,
			DefaultValue: "open",
			Editable:     true,
			Options: []ConfigOption{
				{Label: "开放注册", Value: "open"},
				{Label: "邀请码注册", Value: "invite"},
			},
			Validate: validateEnum("open", "invite"),
		},
		{
			Key:          "default_trial_days",
			Group:        ConfigGroupBusiness,
			GroupLabel:   "基础业务",
			Label:        "默认试用天数",
			Description:  "开放注册时，新用户默认获得的试用天数；0 表示无试用",
			Type:         ConfigValueInteger,
			DefaultValue: "7",
			Editable:     true,
			Validate:     validateIntRange(0, 3650),
		},
		{
			Key:          "notify_group_link",
			Group:        ConfigGroupBusiness,
			GroupLabel:   "基础业务",
			Label:        "通知群组链接",
			Description:  "Telegram 欢迎消息中展示的群组链接，留空表示关闭",
			Type:         ConfigValueURL,
			DefaultValue: "",
			Placeholder:  "https://t.me/your_notify_group",
			Editable:     true,
			AllowEmpty:   true,
			Validate: func(value string) error {
				if strings.TrimSpace(value) == "" {
					return nil
				}
				return validateURL(value)
			},
			Normalize: normalizeTrimmedURLAllowEmpty,
		},
		{
			Key:          "email_verification",
			Group:        ConfigGroupBusiness,
			GroupLabel:   "基础业务",
			Label:        "邮箱验证",
			Description:  "控制注册时是否要求邮箱验证码；实际生效还依赖 SMTP 配置完整",
			Type:         ConfigValueBoolean,
			DefaultValue: "false",
			Editable:     true,
			Validate:     validateBoolean,
			Normalize:    normalizeBoolean,
		},
		{
			Key:          "stripe_allowed_payment_methods",
			Group:        ConfigGroupBusiness,
			GroupLabel:   "基础业务",
			Label:        "Stripe 支付方式",
			Description:  "空值表示跟随 Stripe Dashboard，非空时仅允许选中的方式",
			Type:         ConfigValueJSONList,
			DefaultValue: "",
			Editable:     true,
			AllowEmpty:   true,
			Options: []ConfigOption{
				{Label: "信用卡", Value: "card"},
				{Label: "支付宝", Value: "alipay"},
				{Label: "微信支付", Value: "wechat_pay"},
			},
			Validate: func(value string) error {
				_, err := NormalizeStripeAllowedPaymentMethods(value)
				return err
			},
			Normalize: normalizeStripePaymentMethods,
		},
		{
			Key:         "EMBY_URL",
			EnvKey:      "EMBY_URL",
			Group:       ConfigGroupMedia,
			GroupLabel:  "媒体集成",
			Label:       "Emby 服务地址",
			Description: "后端访问 Emby API 的基础地址",
			Type:        ConfigValueURL,
			Editable:    true,
			Placeholder: "https://your-emby-server.com",
			Validate:    validateURL,
			Normalize:   normalizeTrimmedURL,
		},
		{
			Key:         "EMBY_API_KEY",
			EnvKey:      "EMBY_API_KEY",
			Group:       ConfigGroupMedia,
			GroupLabel:  "媒体集成",
			Label:       "Emby API Key",
			Description: "用于访问 Emby API 的鉴权密钥",
			Type:        ConfigValueSecret,
			Editable:    true,
			Sensitive:   true,
			Validate:    validateNonEmpty("Emby API Key 不能为空"),
		},
		{
			Key:         "NEXT_PUBLIC_EMBY_URL",
			EnvKey:      "NEXT_PUBLIC_EMBY_URL",
			Group:       ConfigGroupMedia,
			GroupLabel:  "媒体集成",
			Label:       "前端 Emby 地址",
			Description: "前端跳转到 Emby 播放页时使用的公网地址，不设置时回退到 Emby 服务地址",
			Type:        ConfigValueURL,
			Editable:    true,
			Placeholder: "https://your-public-emby.com",
			AllowEmpty:  false,
			Validate:    validateURL,
			Normalize:   normalizeTrimmedURL,
		},
		{
			Key:         "TMDB_API_KEY",
			EnvKey:      "TMDB_API_KEY",
			Group:       ConfigGroupMedia,
			GroupLabel:  "媒体集成",
			Label:       "TMDB API Key",
			Description: "用于 TMDB 搜索和追剧日历的接口密钥",
			Type:        ConfigValueSecret,
			Editable:    true,
			Sensitive:   true,
			Validate:    validateNonEmpty("TMDB API Key 不能为空"),
		},
		{
			Key:         "MOVIEPILOT_URL",
			EnvKey:      "MOVIEPILOT_URL",
			Group:       ConfigGroupMedia,
			GroupLabel:  "媒体集成",
			Label:       "MoviePilot 地址",
			Description: "管理员审批订阅后调用的 MoviePilot API 地址",
			Type:        ConfigValueURL,
			Editable:    true,
			Placeholder: "http://your-moviepilot-server:3001",
			Validate:    validateURL,
			Normalize:   normalizeTrimmedURL,
		},
		{
			Key:         "MOVIEPILOT_USERNAME",
			EnvKey:      "MOVIEPILOT_USERNAME",
			Group:       ConfigGroupMedia,
			GroupLabel:  "媒体集成",
			Label:       "MoviePilot 用户名",
			Description: "用于调用 MoviePilot API 的登录用户名",
			Type:        ConfigValueString,
			Editable:    true,
			Sensitive:   true,
			Validate:    validateNonEmpty("MoviePilot 用户名不能为空"),
			Normalize:   normalizeTrimmedString,
		},
		{
			Key:         "MOVIEPILOT_PASSWORD",
			EnvKey:      "MOVIEPILOT_PASSWORD",
			Group:       ConfigGroupMedia,
			GroupLabel:  "媒体集成",
			Label:       "MoviePilot 密码",
			Description: "用于调用 MoviePilot API 的登录密码",
			Type:        ConfigValueSecret,
			Editable:    true,
			Sensitive:   true,
			Validate:    validateNonEmpty("MoviePilot 密码不能为空"),
		},
		{
			Key:         "SMTP_HOST",
			EnvKey:      "SMTP_HOST",
			Group:       ConfigGroupEmail,
			GroupLabel:  "邮件服务",
			Label:       "SMTP 主机",
			Description: "邮件服务器主机地址",
			Type:        ConfigValueString,
			Editable:    true,
			Placeholder: "smtp.example.com",
			Validate:    validateNonEmpty("SMTP 主机不能为空"),
			Normalize:   normalizeTrimmedString,
		},
		{
			Key:          "SMTP_PORT",
			EnvKey:       "SMTP_PORT",
			Group:        ConfigGroupEmail,
			GroupLabel:   "邮件服务",
			Label:        "SMTP 端口",
			Description:  "邮件服务器端口，默认 587",
			Type:         ConfigValueInteger,
			DefaultValue: "587",
			Editable:     true,
			Validate:     validateIntRange(1, 65535),
		},
		{
			Key:         "SMTP_USERNAME",
			EnvKey:      "SMTP_USERNAME",
			Group:       ConfigGroupEmail,
			GroupLabel:  "邮件服务",
			Label:       "SMTP 用户名",
			Description: "邮件服务器登录用户名",
			Type:        ConfigValueString,
			Editable:    true,
			Sensitive:   true,
			Validate:    validateNonEmpty("SMTP 用户名不能为空"),
			Normalize:   normalizeTrimmedString,
		},
		{
			Key:         "SMTP_PASSWORD",
			EnvKey:      "SMTP_PASSWORD",
			Group:       ConfigGroupEmail,
			GroupLabel:  "邮件服务",
			Label:       "SMTP 密码",
			Description: "邮件服务器登录密码",
			Type:        ConfigValueSecret,
			Editable:    true,
			Sensitive:   true,
			Validate:    validateNonEmpty("SMTP 密码不能为空"),
		},
		{
			Key:         "SMTP_FROM",
			EnvKey:      "SMTP_FROM",
			Group:       ConfigGroupEmail,
			GroupLabel:  "邮件服务",
			Label:       "发件人",
			Description: "支持显示名，未设置时回退为 SMTP 用户名",
			Type:        ConfigValueString,
			Editable:    true,
			Placeholder: "Ember <no-reply@example.com>",
			AllowEmpty:  false,
			Validate:    validateMailAddressAllowEmpty,
			Normalize:   normalizeTrimmedString,
		},
		{
			Key:          "EMAIL_CODE_EXPIRY_MINUTES",
			EnvKey:       "EMAIL_CODE_EXPIRY_MINUTES",
			Group:        ConfigGroupEmail,
			GroupLabel:   "邮件服务",
			Label:        "验证码有效期",
			Description:  "邮箱验证码有效时间，单位分钟",
			Type:         ConfigValueInteger,
			DefaultValue: "10",
			Editable:     true,
			Validate:     validateIntRange(1, 1440),
		},
		{
			Key:          "EMAIL_CODE_DAILY_LIMIT",
			EnvKey:       "EMAIL_CODE_DAILY_LIMIT",
			Group:        ConfigGroupEmail,
			GroupLabel:   "邮件服务",
			Label:        "单邮箱日发送上限",
			Description:  "同一邮箱 24 小时内最多发送验证码次数",
			Type:         ConfigValueInteger,
			DefaultValue: "5",
			Editable:     true,
			Validate:     validateIntRange(1, 1000),
		},
		{
			Key:          "EMAIL_CODE_IP_DAILY_LIMIT",
			EnvKey:       "EMAIL_CODE_IP_DAILY_LIMIT",
			Group:        ConfigGroupEmail,
			GroupLabel:   "邮件服务",
			Label:        "单 IP 日发送上限",
			Description:  "同一 IP 24 小时内最多发送验证码次数",
			Type:         ConfigValueInteger,
			DefaultValue: "15",
			Editable:     true,
			Validate:     validateIntRange(1, 5000),
		},
		{
			Key:         "BOT_NOTIFY_URL",
			EnvKey:      "BOT_NOTIFY_URL",
			Group:       ConfigGroupNotification,
			GroupLabel:  "通知与 Bot",
			Label:       "Bot 通知地址",
			Description: "API fire-and-forget 推送到 Bot 的地址，留空表示关闭",
			Type:        ConfigValueURL,
			Editable:    true,
			AllowEmpty:  false,
			Placeholder: "http://localhost:8000",
			Validate:    validateURL,
			Normalize:   normalizeTrimmedURL,
		},
		{
			Key:             "CRON_ENABLED",
			EnvKey:          "CRON_ENABLED",
			Group:           ConfigGroupSchedule,
			GroupLabel:      "任务调度",
			Label:           "Cron 总开关",
			Description:     "控制 API 内置 cron 是否启用",
			Type:            ConfigValueBoolean,
			Editable:        true,
			RestartRequired: true,
			DefaultValue:    "true",
			Validate:        validateBoolean,
			Normalize:       normalizeBoolean,
		},
		{
			Key:             "CRON_SCHEDULE",
			EnvKey:          "CRON_SCHEDULE",
			Group:           ConfigGroupSchedule,
			GroupLabel:      "任务调度",
			Label:           "过期检查计划",
			Description:     "用户过期检查的 cron 表达式",
			Type:            ConfigValueString,
			Editable:        true,
			RestartRequired: true,
			DefaultValue:    "0 2 * * *",
			Validate:        validateCronExpression,
			Normalize:       normalizeTrimmedString,
		},
		{
			Key:             "CRON_TIMEZONE",
			EnvKey:          "CRON_TIMEZONE",
			Group:           ConfigGroupSchedule,
			GroupLabel:      "任务调度",
			Label:           "Cron 时区",
			Description:     "cron 执行使用的时区名称",
			Type:            ConfigValueString,
			Editable:        true,
			RestartRequired: true,
			DefaultValue:    defaultCronTimezone,
			Validate:        validateTimezone,
			Normalize:       normalizeTrimmedString,
		},
		{
			Key:             "RANKING_CRON_ENABLED",
			EnvKey:          "RANKING_CRON_ENABLED",
			Group:           ConfigGroupSchedule,
			GroupLabel:      "任务调度",
			Label:           "排行榜 cron 开关",
			Description:     "控制播放排行榜定时生成是否启用",
			Type:            ConfigValueBoolean,
			Editable:        true,
			RestartRequired: true,
			DefaultValue:    "false",
			Validate:        validateBoolean,
			Normalize:       normalizeBoolean,
		},
		{
			Key:             "RANKING_DAILY_SCHEDULE",
			EnvKey:          "RANKING_DAILY_SCHEDULE",
			Group:           ConfigGroupSchedule,
			GroupLabel:      "任务调度",
			Label:           "日榜计划",
			Description:     "播放日榜定时生成的 cron 表达式",
			Type:            ConfigValueString,
			Editable:        true,
			RestartRequired: true,
			DefaultValue:    "0 20 * * *",
			Validate:        validateCronExpression,
			Normalize:       normalizeTrimmedString,
		},
		{
			Key:             "RANKING_WEEKLY_SCHEDULE",
			EnvKey:          "RANKING_WEEKLY_SCHEDULE",
			Group:           ConfigGroupSchedule,
			GroupLabel:      "任务调度",
			Label:           "周榜计划",
			Description:     "播放周榜定时生成的 cron 表达式",
			Type:            ConfigValueString,
			Editable:        true,
			RestartRequired: true,
			DefaultValue:    "30 20 * * 0",
			Validate:        validateCronExpression,
			Normalize:       normalizeTrimmedString,
		},
		{
			Key:             "TV_CALENDAR_SYNC_SCHEDULE",
			EnvKey:          "TV_CALENDAR_SYNC_SCHEDULE",
			Group:           ConfigGroupSchedule,
			GroupLabel:      "任务调度",
			Label:           "追剧日历同步计划",
			Description:     "追剧日历自动同步的 cron 表达式",
			Type:            ConfigValueString,
			Editable:        true,
			RestartRequired: true,
			DefaultValue:    "0 */12 * * *",
			Validate:        validateCronExpression,
			Normalize:       normalizeTrimmedString,
		},
		{
			Key:         "STRIPE_SECRET_KEY",
			EnvKey:      "STRIPE_SECRET_KEY",
			Group:       ConfigGroupPayment,
			GroupLabel:  "支付",
			Label:       "Stripe Secret Key",
			Description: "Stripe 服务端密钥",
			Type:        ConfigValueSecret,
			Editable:    true,
			Sensitive:   true,
			Validate:    validateNonEmpty("Stripe Secret Key 不能为空"),
		},
		{
			Key:             "STRIPE_WEBHOOK_SECRET",
			EnvKey:          "STRIPE_WEBHOOK_SECRET",
			Group:           ConfigGroupPayment,
			GroupLabel:      "支付",
			Label:           "Stripe Webhook Secret",
			Description:     "Stripe Webhook 签名密钥",
			Type:            ConfigValueSecret,
			Editable:        false,
			Sensitive:       true,
			RestartRequired: true,
		},
		{
			Key:         "STRIPE_SUCCESS_URL",
			EnvKey:      "STRIPE_SUCCESS_URL",
			Group:       ConfigGroupPayment,
			GroupLabel:  "支付",
			Label:       "支付成功跳转地址",
			Description: "Stripe Checkout 支付成功后的跳转地址",
			Type:        ConfigValueURL,
			Editable:    true,
			Validate:    validateURL,
			Normalize:   normalizeTrimmedURL,
		},
		{
			Key:         "STRIPE_CANCEL_URL",
			EnvKey:      "STRIPE_CANCEL_URL",
			Group:       ConfigGroupPayment,
			GroupLabel:  "支付",
			Label:       "支付取消跳转地址",
			Description: "Stripe Checkout 支付取消后的跳转地址",
			Type:        ConfigValueURL,
			Editable:    true,
			Validate:    validateURL,
			Normalize:   normalizeTrimmedURL,
		},
		{
			Key:             "DATABASE_URL",
			EnvKey:          "DATABASE_URL",
			Group:           ConfigGroupDeployment,
			GroupLabel:      "部署与密钥",
			Label:           "数据库连接串",
			Description:     "部署期数据库连接配置",
			Type:            ConfigValueSecret,
			Editable:        false,
			Sensitive:       true,
			RestartRequired: true,
		},
		{
			Key:             "JWT_SECRET",
			EnvKey:          "JWT_SECRET",
			Group:           ConfigGroupDeployment,
			GroupLabel:      "部署与密钥",
			Label:           "JWT 密钥",
			Description:     "JWT 签名密钥",
			Type:            ConfigValueSecret,
			Editable:        false,
			Sensitive:       true,
			RestartRequired: true,
		},
		{
			Key:             "INTERNAL_API_SECRET",
			EnvKey:          "INTERNAL_API_SECRET",
			Group:           ConfigGroupDeployment,
			GroupLabel:      "部署与密钥",
			Label:           "内部服务密钥",
			Description:     "API 与 Bot 内部通信鉴权密钥",
			Type:            ConfigValueSecret,
			Editable:        false,
			Sensitive:       true,
			RestartRequired: true,
		},
		{
			Key:             "ADMIN_USERNAME",
			EnvKey:          "ADMIN_USERNAME",
			Group:           ConfigGroupDeployment,
			GroupLabel:      "部署与密钥",
			Label:           "默认管理员用户名",
			Description:     "仅首次部署时用于初始化管理员账号",
			Type:            ConfigValueString,
			Editable:        false,
			RestartRequired: true,
		},
		{
			Key:             "ADMIN_PASSWORD",
			EnvKey:          "ADMIN_PASSWORD",
			Group:           ConfigGroupDeployment,
			GroupLabel:      "部署与密钥",
			Label:           "默认管理员密码",
			Description:     "仅首次部署时用于初始化管理员账号",
			Type:            ConfigValueSecret,
			Editable:        false,
			Sensitive:       true,
			RestartRequired: true,
		},
		{
			Key:             "TELEGRAM_BOT_TOKEN",
			EnvKey:          "TELEGRAM_BOT_TOKEN",
			Group:           ConfigGroupDeployment,
			GroupLabel:      "部署与密钥",
			Label:           "Telegram Bot Token",
			Description:     "Telegram Bot 令牌",
			Type:            ConfigValueSecret,
			Editable:        false,
			Sensitive:       true,
			RestartRequired: true,
		},
		{
			Key:             "TELEGRAM_ADMIN_CHAT_ID",
			EnvKey:          "TELEGRAM_ADMIN_CHAT_ID",
			Group:           ConfigGroupNotification,
			GroupLabel:      "通知与 Bot",
			Label:           "Telegram 管理员 Chat ID",
			Description:     "Bot 发送管理员通知时使用的对象",
			Type:            ConfigValueString,
			Editable:        true,
			RestartRequired: false,
			Validate:        validateTelegramPositiveChatID,
			Normalize:       normalizeTrimmedString,
		},
		{
			Key:             "TELEGRAM_GROUP_CHAT_ID",
			EnvKey:          "TELEGRAM_GROUP_CHAT_ID",
			Group:           ConfigGroupNotification,
			GroupLabel:      "通知与 Bot",
			Label:           "Telegram 群组 Chat ID",
			Description:     "排行榜等群推送使用的目标群组",
			Type:            ConfigValueString,
			Editable:        true,
			RestartRequired: false,
			AllowEmpty:      true,
			Placeholder:     "-1001234567890",
			Validate:        validateTelegramSignedChatIDAllowEmpty,
			Normalize:       normalizeTrimmedString,
		},
		{
			Key:             "TELEGRAM_WEBHOOK_SECRET",
			EnvKey:          "TELEGRAM_WEBHOOK_SECRET",
			Group:           ConfigGroupDeployment,
			GroupLabel:      "部署与密钥",
			Label:           "Telegram Webhook Secret",
			Description:     "Telegram Webhook 校验密钥",
			Type:            ConfigValueSecret,
			Editable:        false,
			Sensitive:       true,
			RestartRequired: true,
		},
		{
			Key:             "WEBHOOK_URL",
			EnvKey:          "WEBHOOK_URL",
			Group:           ConfigGroupDeployment,
			GroupLabel:      "部署与密钥",
			Label:           "Webhook URL",
			Description:     "Telegram Webhook 公网地址",
			Type:            ConfigValueSecret,
			Editable:        false,
			Sensitive:       true,
			RestartRequired: true,
		},
		{
			Key:             "PORT",
			EnvKey:          "PORT",
			Group:           ConfigGroupDeployment,
			GroupLabel:      "部署与密钥",
			Label:           "API 端口",
			Description:     "API 服务监听端口",
			Type:            ConfigValueInteger,
			Editable:        false,
			RestartRequired: true,
			DefaultValue:    "8080",
		},
		{
			Key:             "AUTO_MIGRATE",
			EnvKey:          "AUTO_MIGRATE",
			Group:           ConfigGroupDeployment,
			GroupLabel:      "部署与密钥",
			Label:           "自动迁移",
			Description:     "控制启动时是否自动执行数据库迁移",
			Type:            ConfigValueBoolean,
			Editable:        false,
			RestartRequired: true,
			DefaultValue:    "false",
		},
	}
}

var configDefinitionMap = buildConfigDefinitionMap()

func buildConfigDefinitionMap() map[string]ConfigDefinition {
	definitions := getConfigDefinitions()
	result := make(map[string]ConfigDefinition, len(definitions))
	for _, def := range definitions {
		result[def.Key] = def
	}
	return result
}

func getConfigDefinitionMap() map[string]ConfigDefinition {
	return configDefinitionMap
}

func validateEnum(allowed ...string) func(string) error {
	return func(value string) error {
		if !slices.Contains(allowed, value) {
			return fmt.Errorf("无效的值，必须为 %s", strings.Join(allowed, " / "))
		}
		return nil
	}
}

func validateBoolean(value string) error {
	if value != "true" && value != "false" {
		return errors.New("无效的布尔值，必须为 true 或 false")
	}
	return nil
}

func normalizeBoolean(value string) (string, error) {
	normalized := strings.ToLower(strings.TrimSpace(value))
	if err := validateBoolean(normalized); err != nil {
		return "", err
	}
	return normalized, nil
}

func validateIntRange(min int, max int) func(string) error {
	return func(value string) error {
		n, err := strconv.Atoi(strings.TrimSpace(value))
		if err != nil {
			return errors.New("请输入有效的整数")
		}
		if n < min || n > max {
			return fmt.Errorf("数值必须在 %d 到 %d 之间", min, max)
		}
		return nil
	}
}

func validateNonEmpty(message string) func(string) error {
	return func(value string) error {
		if strings.TrimSpace(value) == "" {
			return errors.New(message)
		}
		return nil
	}
}

func validateURL(value string) error {
	parsed, err := neturl.ParseRequestURI(strings.TrimSpace(value))
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return errors.New("请输入有效的 URL")
	}
	return nil
}

func normalizeTrimmedURL(value string) (string, error) {
	trimmed := strings.TrimSpace(value)
	trimmed = strings.TrimRight(trimmed, "/")
	if err := validateURL(trimmed); err != nil {
		return "", err
	}
	return trimmed, nil
}

func normalizeTrimmedURLAllowEmpty(value string) (string, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return "", nil
	}
	return normalizeTrimmedURL(trimmed)
}

func normalizeTrimmedString(value string) (string, error) {
	return strings.TrimSpace(value), nil
}

func validateTelegramPositiveChatID(value string) error {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return errors.New("Telegram 管理员 Chat ID 不能为空")
	}
	chatID, err := strconv.ParseInt(trimmed, 10, 64)
	if err != nil || chatID <= 0 {
		return errors.New("Telegram 管理员 Chat ID 无效")
	}
	return nil
}

func validateTelegramSignedChatIDAllowEmpty(value string) error {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil
	}
	chatID, err := strconv.ParseInt(trimmed, 10, 64)
	if err != nil || chatID == 0 {
		return errors.New("Telegram 群组 Chat ID 无效")
	}
	return nil
}

func validateCronExpression(value string) error {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return errors.New("cron 表达式不能为空")
	}
	if _, err := cron.ParseStandard(trimmed); err != nil {
		return errors.New("cron 表达式无效")
	}
	return nil
}

func validateTimezone(value string) error {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return errors.New("时区不能为空")
	}
	if _, err := time.LoadLocation(trimmed); err != nil {
		return errors.New("时区无效")
	}
	return nil
}

func LoadConfiguredTimezone() *time.Location {
	tzName := NewConfigService().GetString("CRON_TIMEZONE")
	if strings.TrimSpace(tzName) == "" {
		tzName = defaultCronTimezone
	}

	tz, err := time.LoadLocation(tzName)
	if err != nil {
		return time.UTC
	}
	return tz
}

func validateMailAddressAllowEmpty(value string) error {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	if _, err := mail.ParseAddress(strings.TrimSpace(value)); err != nil {
		return errors.New("发件人格式无效")
	}
	return nil
}

func normalizeStripePaymentMethods(value string) (string, error) {
	raw := strings.TrimSpace(value)
	if raw == "" {
		return "", nil
	}

	var values []string
	if err := json.Unmarshal([]byte(raw), &values); err != nil {
		return "", ErrPaymentMethodSettingInvalid
	}

	normalized, err := NormalizeStripeAllowedPaymentMethods(raw)
	if err != nil {
		return "", err
	}
	if len(normalized) == 0 {
		return "", nil
	}

	data, err := json.Marshal(normalized)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func TestSMTPDial(host string, port string) error {
	if strings.TrimSpace(host) == "" || strings.TrimSpace(port) == "" {
		return ErrEmailNotConfigured
	}

	conn, err := net.DialTimeout("tcp", net.JoinHostPort(host, port), smtpTimeout)
	if err != nil {
		return fmt.Errorf("SMTP 连接失败: %w", err)
	}
	defer conn.Close()

	_ = conn.SetDeadline(time.Now().Add(smtpTimeout))
	return nil
}

func TestHTTPReachable(rawURL string) error {
	if strings.TrimSpace(rawURL) == "" {
		return errors.New("URL 未配置")
	}
	req, err := http.NewRequest(http.MethodGet, rawURL, nil)
	if err != nil {
		return err
	}

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 500 {
		return fmt.Errorf("远端返回状态码 %d", resp.StatusCode)
	}

	return nil
}
