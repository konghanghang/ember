package config

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
	"regexp"
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

const smtpTimeout = 10 * time.Second

type ConfigValueType string
type ConfigEmptyValueMode string
type ConfigRiskLevel string

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
	ConfigEmptyValueNotAllowed ConfigEmptyValueMode = "not_allowed"
	ConfigEmptyValueDisable    ConfigEmptyValueMode = "disable"
	ConfigEmptyValueFallback   ConfigEmptyValueMode = "fallback"
	ConfigEmptyValueInherit    ConfigEmptyValueMode = "inherit"
)

const (
	ConfigRiskNone     ConfigRiskLevel = "none"
	ConfigRiskInfo     ConfigRiskLevel = "info"
	ConfigRiskWarning  ConfigRiskLevel = "warning"
	ConfigRiskCritical ConfigRiskLevel = "critical"
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

const defaultTelegramWelcomeMessageTemplate = `👋 欢迎 <b>{names}</b> 加入！

📢 入库通知群组：{notifyGroupLink}
⏳ 本消息将在 30 秒后自动删除`

var telegramWelcomeMessagePlaceholderPattern = regexp.MustCompile(`\{[a-zA-Z][a-zA-Z0-9]*\}`)

type ConfigOption struct {
	Label string `json:"label"`
	Value string `json:"value"`
}

type ConfigDefinition struct {
	Key                string
	Group              string
	GroupLabel         string
	Label              string
	Description        string
	Type               ConfigValueType
	DefaultValue       string
	Placeholder        string
	Multiline          bool
	Editable           bool
	Sensitive          bool
	RestartRequired    bool
	AllowEmpty         bool
	EmptyValueMode     ConfigEmptyValueMode
	EmptyValueHint     string
	ReadOnlyHint       string
	MissingValueHint   string
	MissingValueLevel  ConfigRiskLevel
	EnvKey             string
	DisableEnvFallback bool
	MinValue           *int
	MaxValue           *int
	Options            []ConfigOption
	Validate           func(string) error
	Normalize          func(string) (string, error)
}

type ConfigItem struct {
	Key               string               `json:"key"`
	Group             string               `json:"group"`
	GroupLabel        string               `json:"groupLabel"`
	Label             string               `json:"label"`
	Description       string               `json:"description"`
	Type              ConfigValueType      `json:"type"`
	Placeholder       string               `json:"placeholder,omitempty"`
	Multiline         bool                 `json:"multiline"`
	Editable          bool                 `json:"editable"`
	Sensitive         bool                 `json:"sensitive"`
	RestartRequired   bool                 `json:"restartRequired"`
	AllowEmpty        bool                 `json:"allowEmpty"`
	EmptyValueMode    ConfigEmptyValueMode `json:"emptyValueMode"`
	EmptyValueHint    string               `json:"emptyValueHint,omitempty"`
	ReadOnlyHint      string               `json:"readOnlyHint,omitempty"`
	MissingValueHint  string               `json:"missingValueHint,omitempty"`
	MissingValueLevel ConfigRiskLevel      `json:"missingValueLevel"`
	FallbackHint      string               `json:"fallbackHint,omitempty"`
	MinValue          *int                 `json:"minValue,omitempty"`
	MaxValue          *int                 `json:"maxValue,omitempty"`
	Options           []ConfigOption       `json:"options,omitempty"`
	Source            string               `json:"source"`
	HasValue          bool                 `json:"hasValue"`
	Value             *string              `json:"value,omitempty"`
	Error             string               `json:"error,omitempty"`
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

	value, source, hasValue, err := s.resolveRawValue(def, nil)
	if err != nil {
		return "", source, err
	}
	if !hasValue {
		return "", source, nil
	}
	return value, source, nil
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

	return s.importEnvDefinitions(definitions, settingsMap, updatedByUserID, os.Getenv, s.Update), nil
}

func (s *ConfigService) importEnvDefinitions(
	definitions []ConfigDefinition,
	settingsMap map[string]models.Setting,
	updatedByUserID string,
	getenv func(string) string,
	updateFn func(string, UpdateConfigRequest, string) (*ConfigItem, error),
) *ImportEnvResult {
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

		envValue := strings.TrimSpace(getenv(def.EnvKey))
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

		if _, updateErr := updateFn(def.Key, UpdateConfigRequest{Value: &value}, updatedByUserID); updateErr != nil {
			result.Failed[def.Key] = updateErr.Error()
			continue
		}

		result.Imported = append(result.Imported, def.Key)
	}

	return result
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

	if err := s.testEmbyConnection(); err != nil {
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

	if strings.TrimSpace(s.GetString("MOVIEPILOT_URL")) != "" {
		if err := s.testMoviePilotConnection(); err != nil {
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

func (s *ConfigService) testEmbyConnection() error {
	baseURL := strings.TrimRight(strings.TrimSpace(s.GetString("EMBY_URL")), "/")
	apiKey := strings.TrimSpace(s.GetString("EMBY_API_KEY"))
	if baseURL == "" || apiKey == "" {
		return errors.New("Emby 配置未设置（EMBY_URL 或 EMBY_API_KEY）")
	}

	req, err := http.NewRequest(http.MethodGet, baseURL+"/emby/Users?api_key="+neturl.QueryEscape(apiKey), nil)
	if err != nil {
		return err
	}

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("无法连接到 Emby 服务器：%v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("Emby API 返回异常状态码 %d", resp.StatusCode)
	}

	return nil
}

func (s *ConfigService) testMoviePilotConnection() error {
	baseURL := strings.TrimRight(strings.TrimSpace(s.GetString("MOVIEPILOT_URL")), "/")
	username := strings.TrimSpace(s.GetString("MOVIEPILOT_USERNAME"))
	password := s.GetString("MOVIEPILOT_PASSWORD")
	if baseURL == "" || username == "" || password == "" {
		return errors.New("MoviePilot 未配置")
	}

	formData := neturl.Values{}
	formData.Set("username", username)
	formData.Set("password", password)

	req, err := http.NewRequest(http.MethodPost, baseURL+"/api/v1/login/access-token", strings.NewReader(formData.Encode()))
	if err != nil {
		return fmt.Errorf("创建请求失败: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("登录请求失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("MoviePilot 登录失败: %d %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	return nil
}

func (s *ConfigService) testEmailGroup() *ConfigGroupTestResult {
	result := &ConfigGroupTestResult{
		Details: make([]ConfigGroupTestDetail, 0, 1),
	}

	host := strings.TrimSpace(s.GetString("SMTP_HOST"))
	port := strings.TrimSpace(s.GetString("SMTP_PORT"))
	if port == "" {
		port = "587"
	}

	if err := TestSMTPDial(host, port); err != nil {
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
		Key:               def.Key,
		Group:             def.Group,
		GroupLabel:        def.GroupLabel,
		Label:             def.Label,
		Description:       def.Description,
		Type:              def.Type,
		Placeholder:       def.Placeholder,
		Multiline:         def.Multiline,
		Editable:          def.Editable,
		Sensitive:         def.Sensitive,
		RestartRequired:   def.RestartRequired,
		AllowEmpty:        def.AllowEmpty,
		EmptyValueMode:    normalizeEmptyValueMode(def),
		EmptyValueHint:    def.EmptyValueHint,
		ReadOnlyHint:      def.ReadOnlyHint,
		MissingValueHint:  def.MissingValueHint,
		MissingValueLevel: normalizeRiskLevel(def.MissingValueLevel),
		FallbackHint:      buildFallbackHint(def),
		MinValue:          def.MinValue,
		MaxValue:          def.MaxValue,
		Options:           def.Options,
		Source:            ConfigSourceUnset,
	}

	value, source, hasValue, err := s.resolveRawValue(def, settingsMap)
	if err != nil {
		item.Source = source
		item.Error = err.Error()
		return item, nil
	}
	if source == ConfigSourceUnset {
		item.Source = source
		item.HasValue = false
		return item, nil
	}
	item.HasValue = hasValue
	return s.applyResolvedValue(item, def, value, source), nil
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

func (s *ConfigService) resolveRawValue(def ConfigDefinition, settingsMap map[string]models.Setting) (string, string, bool, error) {
	providedSettingsMap := settingsMap != nil
	if settingsMap == nil {
		loadedSettings, err := s.loadSettings([]ConfigDefinition{def})
		if err != nil {
			return "", ConfigSourceUnset, false, err
		}
		settingsMap = loadedSettings
	}

	if stored, ok := settingsMap[def.Key]; ok && s.shouldUseDatabaseValue(def, stored) {
		resolved, err := s.decodeStoredValue(def, stored)
		if err != nil {
			return "", ConfigSourceDatabase, false, err
		}
		return resolved, ConfigSourceDatabase, resolved != "", nil
	}

	allowEnvFallback := !def.DisableEnvFallback || (!providedSettingsMap && db.DB == nil)
	if def.EnvKey != "" && allowEnvFallback {
		if envValue, ok := s.resolveEnvValue(def); ok {
			return envValue, ConfigSourceEnv, envValue != "", nil
		}
	}

	if def.DefaultValue != "" || def.AllowEmpty {
		return def.DefaultValue, ConfigSourceDefault, def.DefaultValue != "", nil
	}

	return "", ConfigSourceUnset, false, nil
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

func normalizeEmptyValueMode(def ConfigDefinition) ConfigEmptyValueMode {
	if !def.AllowEmpty {
		return ConfigEmptyValueNotAllowed
	}
	if def.EmptyValueMode == "" {
		return ConfigEmptyValueFallback
	}
	return def.EmptyValueMode
}

func normalizeRiskLevel(level ConfigRiskLevel) ConfigRiskLevel {
	if level == "" {
		return ConfigRiskNone
	}
	return level
}

func buildFallbackHint(def ConfigDefinition) string {
	sources := make([]string, 0, 2)
	if def.EnvKey != "" && !def.DisableEnvFallback {
		sources = append(sources, "环境变量")
	}
	if def.DefaultValue != "" {
		sources = append(sources, "默认值")
	} else if def.AllowEmpty {
		sources = append(sources, "系统默认空值")
	}

	if len(sources) == 0 {
		return "移除数据库覆盖值后将回到未设置状态。"
	}

	if len(sources) == 1 {
		switch sources[0] {
		case "环境变量":
			return "移除数据库覆盖值后将回退到环境变量；若环境变量缺失则回到未设置状态。"
		case "默认值":
			return "移除数据库覆盖值后将回退到默认值。"
		case "系统默认空值":
			return "移除数据库覆盖值后将回退到系统默认空值。"
		}
	}

	return fmt.Sprintf("移除数据库覆盖值后将按顺序回退到%s。", strings.Join(sources, "、"))
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

func intPtr(v int) *int {
	return &v
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
			MinValue:     intPtr(0),
			MaxValue:     intPtr(3650),
			Validate:     validateIntRange(0, 3650),
		},
		{
			Key:            "notify_group_link",
			Group:          ConfigGroupBusiness,
			GroupLabel:     "基础业务",
			Label:          "通知群组链接",
			Description:    "Telegram 欢迎消息中展示的群组链接",
			Type:           ConfigValueURL,
			DefaultValue:   "",
			Placeholder:    "https://t.me/your_notify_group",
			Editable:       true,
			AllowEmpty:     true,
			EmptyValueMode: ConfigEmptyValueDisable,
			EmptyValueHint: "保存为空值后将关闭欢迎消息中的群组链接展示。",
			Validate: func(value string) error {
				if strings.TrimSpace(value) == "" {
					return nil
				}
				return validateURL(value)
			},
			Normalize: normalizeTrimmedURLAllowEmpty,
		},
		{
			Key:            "telegram_welcome_message_template",
			Group:          ConfigGroupBusiness,
			GroupLabel:     "基础业务",
			Label:          "Telegram 欢迎语模板",
			Description:    "Telegram 入群欢迎语模板，支持 {names} 和 {notifyGroupLink} 占位符",
			Type:           ConfigValueString,
			DefaultValue:   defaultTelegramWelcomeMessageTemplate,
			Editable:       true,
			Multiline:      true,
			AllowEmpty:     true,
			EmptyValueMode: ConfigEmptyValueDisable,
			EmptyValueHint: "保存为空值后将关闭入群欢迎消息发送。",
			Validate:       validateTelegramWelcomeMessageTemplate,
			Normalize:      normalizeTelegramWelcomeMessageTemplate,
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
			Key:            "stripe_allowed_payment_methods",
			Group:          ConfigGroupBusiness,
			GroupLabel:     "基础业务",
			Label:          "Stripe 支付方式",
			Description:    "非空时仅允许选中的支付方式",
			Type:           ConfigValueJSONList,
			DefaultValue:   "",
			Editable:       true,
			AllowEmpty:     true,
			EmptyValueMode: ConfigEmptyValueInherit,
			EmptyValueHint: "保存为空值后将跟随 Stripe Dashboard 中启用的支付方式。",
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
			Key:                "EMBY_URL",
			EnvKey:             "EMBY_URL",
			DisableEnvFallback: true,
			Group:              ConfigGroupMedia,
			GroupLabel:         "媒体集成",
			Label:              "Emby 服务地址",
			Description:        "后端访问 Emby API 的基础地址",
			Type:               ConfigValueURL,
			Editable:           true,
			Placeholder:        "https://your-emby-server.com",
			Validate:           validateURL,
			Normalize:          normalizeTrimmedURL,
		},
		{
			Key:                "EMBY_API_KEY",
			EnvKey:             "EMBY_API_KEY",
			DisableEnvFallback: true,
			Group:              ConfigGroupMedia,
			GroupLabel:         "媒体集成",
			Label:              "Emby API Key",
			Description:        "用于访问 Emby API 的鉴权密钥",
			Type:               ConfigValueSecret,
			Editable:           true,
			Sensitive:          true,
			Validate:           validateNonEmpty("Emby API Key 不能为空"),
		},
		{
			Key:                "NEXT_PUBLIC_EMBY_URL",
			EnvKey:             "NEXT_PUBLIC_EMBY_URL",
			DisableEnvFallback: true,
			Group:              ConfigGroupMedia,
			GroupLabel:         "媒体集成",
			Label:              "前端 Emby 地址",
			Description:        "前端跳转到 Emby 播放页时使用的公网地址",
			Type:               ConfigValueURL,
			Editable:           true,
			Placeholder:        "https://your-public-emby.com",
			AllowEmpty:         true,
			EmptyValueMode:     ConfigEmptyValueFallback,
			EmptyValueHint:     "保存为空值后将回退到 Emby 服务地址。",
			Validate: func(value string) error {
				if strings.TrimSpace(value) == "" {
					return nil
				}
				return validateURL(value)
			},
			Normalize: normalizeTrimmedURLAllowEmpty,
		},
		{
			Key:                "TMDB_API_KEY",
			EnvKey:             "TMDB_API_KEY",
			DisableEnvFallback: true,
			Group:              ConfigGroupMedia,
			GroupLabel:         "媒体集成",
			Label:              "TMDB API Key",
			Description:        "用于 TMDB 搜索和追剧日历的接口密钥",
			Type:               ConfigValueSecret,
			Editable:           true,
			Sensitive:          true,
			Validate:           validateNonEmpty("TMDB API Key 不能为空"),
		},
		{
			Key:                "MOVIEPILOT_URL",
			EnvKey:             "MOVIEPILOT_URL",
			DisableEnvFallback: true,
			Group:              ConfigGroupMedia,
			GroupLabel:         "媒体集成",
			Label:              "MoviePilot 地址",
			Description:        "管理员审批订阅后调用的 MoviePilot API 地址",
			Type:               ConfigValueURL,
			Editable:           true,
			Placeholder:        "http://your-moviepilot-server:3001",
			Validate:           validateURL,
			Normalize:          normalizeTrimmedURL,
		},
		{
			Key:                "MOVIEPILOT_USERNAME",
			EnvKey:             "MOVIEPILOT_USERNAME",
			DisableEnvFallback: true,
			Group:              ConfigGroupMedia,
			GroupLabel:         "媒体集成",
			Label:              "MoviePilot 用户名",
			Description:        "用于调用 MoviePilot API 的登录用户名",
			Type:               ConfigValueString,
			Editable:           true,
			Sensitive:          true,
			Validate:           validateNonEmpty("MoviePilot 用户名不能为空"),
			Normalize:          normalizeTrimmedString,
		},
		{
			Key:                "MOVIEPILOT_PASSWORD",
			EnvKey:             "MOVIEPILOT_PASSWORD",
			DisableEnvFallback: true,
			Group:              ConfigGroupMedia,
			GroupLabel:         "媒体集成",
			Label:              "MoviePilot 密码",
			Description:        "用于调用 MoviePilot API 的登录密码",
			Type:               ConfigValueSecret,
			Editable:           true,
			Sensitive:          true,
			Validate:           validateNonEmpty("MoviePilot 密码不能为空"),
		},
		{
			Key:                "SMTP_HOST",
			EnvKey:             "SMTP_HOST",
			DisableEnvFallback: true,
			Group:              ConfigGroupEmail,
			GroupLabel:         "邮件服务",
			Label:              "SMTP 主机",
			Description:        "邮件服务器主机地址",
			Type:               ConfigValueString,
			Editable:           true,
			Placeholder:        "smtp.example.com",
			Validate:           validateNonEmpty("SMTP 主机不能为空"),
			Normalize:          normalizeTrimmedString,
		},
		{
			Key:                "SMTP_PORT",
			EnvKey:             "SMTP_PORT",
			DisableEnvFallback: true,
			Group:              ConfigGroupEmail,
			GroupLabel:         "邮件服务",
			Label:              "SMTP 端口",
			Description:        "邮件服务器端口，默认 587",
			Type:               ConfigValueInteger,
			DefaultValue:       "587",
			Editable:           true,
			MinValue:           intPtr(1),
			MaxValue:           intPtr(65535),
			Validate:           validateIntRange(1, 65535),
		},
		{
			Key:                "SMTP_USERNAME",
			EnvKey:             "SMTP_USERNAME",
			DisableEnvFallback: true,
			Group:              ConfigGroupEmail,
			GroupLabel:         "邮件服务",
			Label:              "SMTP 用户名",
			Description:        "邮件服务器登录用户名",
			Type:               ConfigValueString,
			Editable:           true,
			Sensitive:          true,
			Validate:           validateNonEmpty("SMTP 用户名不能为空"),
			Normalize:          normalizeTrimmedString,
		},
		{
			Key:                "SMTP_PASSWORD",
			EnvKey:             "SMTP_PASSWORD",
			DisableEnvFallback: true,
			Group:              ConfigGroupEmail,
			GroupLabel:         "邮件服务",
			Label:              "SMTP 密码",
			Description:        "邮件服务器登录密码",
			Type:               ConfigValueSecret,
			Editable:           true,
			Sensitive:          true,
			Validate:           validateNonEmpty("SMTP 密码不能为空"),
		},
		{
			Key:                "SMTP_FROM",
			EnvKey:             "SMTP_FROM",
			DisableEnvFallback: true,
			Group:              ConfigGroupEmail,
			GroupLabel:         "邮件服务",
			Label:              "发件人",
			Description:        "支持显示名的发件人地址",
			Type:               ConfigValueString,
			Editable:           true,
			Placeholder:        "Ember <no-reply@example.com>",
			AllowEmpty:         true,
			EmptyValueMode:     ConfigEmptyValueFallback,
			EmptyValueHint:     "保存为空值后将回退到 SMTP 用户名。",
			Validate:           validateMailAddressAllowEmpty,
			Normalize:          normalizeTrimmedString,
		},
		{
			Key:                "EMAIL_CODE_EXPIRY_MINUTES",
			EnvKey:             "EMAIL_CODE_EXPIRY_MINUTES",
			DisableEnvFallback: true,
			Group:              ConfigGroupEmail,
			GroupLabel:         "邮件服务",
			Label:              "验证码有效期",
			Description:        "邮箱验证码有效时间，单位分钟",
			Type:               ConfigValueInteger,
			DefaultValue:       "10",
			Editable:           true,
			MinValue:           intPtr(1),
			MaxValue:           intPtr(1440),
			Validate:           validateIntRange(1, 1440),
		},
		{
			Key:                "EMAIL_CODE_DAILY_LIMIT",
			EnvKey:             "EMAIL_CODE_DAILY_LIMIT",
			DisableEnvFallback: true,
			Group:              ConfigGroupEmail,
			GroupLabel:         "邮件服务",
			Label:              "单邮箱日发送上限",
			Description:        "同一邮箱 24 小时内最多发送验证码次数",
			Type:               ConfigValueInteger,
			DefaultValue:       "5",
			Editable:           true,
			MinValue:           intPtr(1),
			MaxValue:           intPtr(1000),
			Validate:           validateIntRange(1, 1000),
		},
		{
			Key:                "EMAIL_CODE_IP_DAILY_LIMIT",
			EnvKey:             "EMAIL_CODE_IP_DAILY_LIMIT",
			DisableEnvFallback: true,
			Group:              ConfigGroupEmail,
			GroupLabel:         "邮件服务",
			Label:              "单 IP 日发送上限",
			Description:        "同一 IP 24 小时内最多发送验证码次数",
			Type:               ConfigValueInteger,
			DefaultValue:       "15",
			Editable:           true,
			MinValue:           intPtr(1),
			MaxValue:           intPtr(5000),
			Validate:           validateIntRange(1, 5000),
		},
		{
			Key:                "BOT_NOTIFY_URL",
			EnvKey:             "BOT_NOTIFY_URL",
			DisableEnvFallback: true,
			Group:              ConfigGroupNotification,
			GroupLabel:         "通知与 Bot",
			Label:              "Bot 通知地址",
			Description:        "API fire-and-forget 推送到 Bot 的地址",
			Type:               ConfigValueURL,
			Editable:           true,
			AllowEmpty:         true,
			EmptyValueMode:     ConfigEmptyValueDisable,
			EmptyValueHint:     "保存为空值后将关闭 API 到 Bot 的 fire-and-forget 推送。",
			Placeholder:        "http://localhost:8000",
			Validate: func(value string) error {
				if strings.TrimSpace(value) == "" {
					return nil
				}
				return validateURL(value)
			},
			Normalize: normalizeTrimmedURLAllowEmpty,
		},
		{
			Key:                "CRON_ENABLED",
			EnvKey:             "CRON_ENABLED",
			DisableEnvFallback: true,
			Group:              ConfigGroupSchedule,
			GroupLabel:         "任务调度",
			Label:              "Cron 总开关",
			Description:        "控制 API 内置 cron 是否启用",
			Type:               ConfigValueBoolean,
			Editable:           true,
			RestartRequired:    true,
			DefaultValue:       "true",
			Validate:           validateBoolean,
			Normalize:          normalizeBoolean,
		},
		{
			Key:                "CRON_SCHEDULE",
			EnvKey:             "CRON_SCHEDULE",
			DisableEnvFallback: true,
			Group:              ConfigGroupSchedule,
			GroupLabel:         "任务调度",
			Label:              "过期检查计划",
			Description:        "用户过期检查的 cron 表达式",
			Type:               ConfigValueString,
			Editable:           true,
			RestartRequired:    true,
			DefaultValue:       "0 2 * * *",
			Validate:           validateCronExpression,
			Normalize:          normalizeTrimmedString,
		},
		{
			Key:                "CRON_TIMEZONE",
			EnvKey:             "CRON_TIMEZONE",
			DisableEnvFallback: true,
			Group:              ConfigGroupSchedule,
			GroupLabel:         "任务调度",
			Label:              "Cron 时区",
			Description:        "cron 执行使用的时区名称",
			Type:               ConfigValueString,
			Editable:           true,
			RestartRequired:    true,
			DefaultValue:       defaultCronTimezone,
			Validate:           validateTimezone,
			Normalize:          normalizeTrimmedString,
		},
		{
			Key:                "RANKING_CRON_ENABLED",
			EnvKey:             "RANKING_CRON_ENABLED",
			DisableEnvFallback: true,
			Group:              ConfigGroupSchedule,
			GroupLabel:         "任务调度",
			Label:              "排行榜 cron 开关",
			Description:        "控制播放排行榜定时生成是否启用",
			Type:               ConfigValueBoolean,
			Editable:           true,
			RestartRequired:    true,
			DefaultValue:       "false",
			Validate:           validateBoolean,
			Normalize:          normalizeBoolean,
		},
		{
			Key:                "RANKING_DAILY_SCHEDULE",
			EnvKey:             "RANKING_DAILY_SCHEDULE",
			DisableEnvFallback: true,
			Group:              ConfigGroupSchedule,
			GroupLabel:         "任务调度",
			Label:              "日榜计划",
			Description:        "播放日榜定时生成的 cron 表达式",
			Type:               ConfigValueString,
			Editable:           true,
			RestartRequired:    true,
			DefaultValue:       "0 20 * * *",
			Validate:           validateCronExpression,
			Normalize:          normalizeTrimmedString,
		},
		{
			Key:                "RANKING_WEEKLY_SCHEDULE",
			EnvKey:             "RANKING_WEEKLY_SCHEDULE",
			DisableEnvFallback: true,
			Group:              ConfigGroupSchedule,
			GroupLabel:         "任务调度",
			Label:              "周榜计划",
			Description:        "播放周榜定时生成的 cron 表达式",
			Type:               ConfigValueString,
			Editable:           true,
			RestartRequired:    true,
			DefaultValue:       "30 20 * * 0",
			Validate:           validateCronExpression,
			Normalize:          normalizeTrimmedString,
		},
		{
			Key:                "TV_CALENDAR_SYNC_SCHEDULE",
			EnvKey:             "TV_CALENDAR_SYNC_SCHEDULE",
			DisableEnvFallback: true,
			Group:              ConfigGroupSchedule,
			GroupLabel:         "任务调度",
			Label:              "追剧日历同步计划",
			Description:        "追剧日历自动同步的 cron 表达式",
			Type:               ConfigValueString,
			Editable:           true,
			RestartRequired:    true,
			DefaultValue:       "0 */12 * * *",
			Validate:           validateCronExpression,
			Normalize:          normalizeTrimmedString,
		},
		{
			Key:                "STRIPE_SECRET_KEY",
			EnvKey:             "STRIPE_SECRET_KEY",
			DisableEnvFallback: true,
			Group:              ConfigGroupPayment,
			GroupLabel:         "支付",
			Label:              "Stripe Secret Key",
			Description:        "Stripe 服务端密钥",
			Type:               ConfigValueSecret,
			Editable:           true,
			Sensitive:          true,
			Validate:           validateNonEmpty("Stripe Secret Key 不能为空"),
		},
		{
			Key:               "STRIPE_WEBHOOK_SECRET",
			EnvKey:            "STRIPE_WEBHOOK_SECRET",
			Group:             ConfigGroupPayment,
			GroupLabel:        "支付",
			Label:             "Stripe Webhook Secret",
			Description:       "Stripe Webhook 签名密钥",
			Type:              ConfigValueSecret,
			Editable:          false,
			Sensitive:         true,
			RestartRequired:   true,
			ReadOnlyHint:      "该密钥用于校验 Stripe 回调签名，只能通过部署环境注入，避免后台误改后导致回调验签失效。",
			MissingValueHint:  "未设置时 Stripe Webhook 回调无法完成签名校验，支付状态同步会失败。",
			MissingValueLevel: ConfigRiskCritical,
		},
		{
			Key:                "STRIPE_SUCCESS_URL",
			EnvKey:             "STRIPE_SUCCESS_URL",
			DisableEnvFallback: true,
			Group:              ConfigGroupPayment,
			GroupLabel:         "支付",
			Label:              "支付成功跳转地址",
			Description:        "Stripe Checkout 支付成功后的跳转地址",
			Type:               ConfigValueURL,
			Editable:           true,
			Validate:           validateURL,
			Normalize:          normalizeTrimmedURL,
		},
		{
			Key:                "STRIPE_CANCEL_URL",
			EnvKey:             "STRIPE_CANCEL_URL",
			DisableEnvFallback: true,
			Group:              ConfigGroupPayment,
			GroupLabel:         "支付",
			Label:              "支付取消跳转地址",
			Description:        "Stripe Checkout 支付取消后的跳转地址",
			Type:               ConfigValueURL,
			Editable:           true,
			Validate:           validateURL,
			Normalize:          normalizeTrimmedURL,
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
			Key:               "TELEGRAM_BOT_TOKEN",
			EnvKey:            "TELEGRAM_BOT_TOKEN",
			Group:             ConfigGroupDeployment,
			GroupLabel:        "部署与密钥",
			Label:             "Telegram Bot Token",
			Description:       "Telegram Bot 令牌",
			Type:              ConfigValueSecret,
			Editable:          false,
			Sensitive:         true,
			RestartRequired:   true,
			ReadOnlyHint:      "Bot 启动时必须从部署环境读取该令牌；修改后需要重启 Bot，不应在后台在线编辑。",
			MissingValueHint:  "未设置时 Telegram Bot 无法启动，也无法接收或发送通知。",
			MissingValueLevel: ConfigRiskCritical,
		},
		{
			Key:                "TELEGRAM_ADMIN_CHAT_ID",
			EnvKey:             "TELEGRAM_ADMIN_CHAT_ID",
			DisableEnvFallback: true,
			Group:              ConfigGroupNotification,
			GroupLabel:         "通知与 Bot",
			Label:              "Telegram 管理员 Chat ID",
			Description:        "Bot 发送管理员通知时使用的对象",
			Type:               ConfigValueString,
			Editable:           true,
			RestartRequired:    false,
			Validate:           validateTelegramPositiveChatID,
			Normalize:          normalizeTrimmedString,
		},
		{
			Key:                "TELEGRAM_GROUP_CHAT_ID",
			EnvKey:             "TELEGRAM_GROUP_CHAT_ID",
			DisableEnvFallback: true,
			Group:              ConfigGroupNotification,
			GroupLabel:         "通知与 Bot",
			Label:              "Telegram 群组 Chat ID",
			Description:        "排行榜等群推送使用的目标群组",
			Type:               ConfigValueString,
			Editable:           true,
			RestartRequired:    false,
			AllowEmpty:         true,
			EmptyValueMode:     ConfigEmptyValueFallback,
			EmptyValueHint:     "保存为空值后排行榜通知将回退到管理员 Chat ID。",
			Placeholder:        "-1001234567890",
			Validate:           validateTelegramSignedChatIDAllowEmpty,
			Normalize:          normalizeTrimmedString,
		},
		{
			Key:               "TELEGRAM_WEBHOOK_SECRET",
			EnvKey:            "TELEGRAM_WEBHOOK_SECRET",
			Group:             ConfigGroupDeployment,
			GroupLabel:        "部署与密钥",
			Label:             "Telegram Webhook Secret",
			Description:       "Telegram Webhook 校验密钥",
			Type:              ConfigValueSecret,
			Editable:          false,
			Sensitive:         true,
			RestartRequired:   true,
			ReadOnlyHint:      "该密钥必须与 Telegram 注册的 Webhook 配置保持一致，只能通过部署环境管理。",
			MissingValueHint:  "未设置时 Telegram Webhook 请求无法做额外校验，Bot Webhook 安全边界不完整。",
			MissingValueLevel: ConfigRiskCritical,
		},
		{
			Key:               "WEBHOOK_URL",
			EnvKey:            "WEBHOOK_URL",
			Group:             ConfigGroupDeployment,
			GroupLabel:        "部署与密钥",
			Label:             "Webhook URL",
			Description:       "Telegram Webhook 公网地址",
			Type:              ConfigValueSecret,
			Editable:          false,
			Sensitive:         true,
			RestartRequired:   true,
			ReadOnlyHint:      "Webhook 公网地址取决于部署拓扑和反向代理，只能在部署环境维护，修改后需要重新注册 Telegram Webhook。",
			MissingValueHint:  "未设置时 Telegram Webhook 模式无法正常接入公网请求。",
			MissingValueLevel: ConfigRiskCritical,
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
			MinValue:        intPtr(1),
			MaxValue:        intPtr(65535),
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

func normalizeTrimmedMultilineString(value string) string {
	normalized := strings.ReplaceAll(value, "\r\n", "\n")
	normalized = strings.ReplaceAll(normalized, "\r", "\n")
	return strings.TrimSpace(normalized)
}

func normalizeTelegramWelcomeMessageTemplate(value string) (string, error) {
	return normalizeTrimmedMultilineString(value), nil
}

func validateTelegramWelcomeMessageTemplate(value string) error {
	trimmed := normalizeTrimmedMultilineString(value)
	if trimmed == "" {
		return nil
	}
	if !strings.Contains(trimmed, "{names}") {
		return errors.New("欢迎语模板必须包含 {names} 占位符")
	}

	placeholders := telegramWelcomeMessagePlaceholderPattern.FindAllString(trimmed, -1)
	for _, placeholder := range placeholders {
		if placeholder == "{names}" || placeholder == "{notifyGroupLink}" {
			continue
		}
		return fmt.Errorf("欢迎语模板包含不支持的占位符 %s，仅支持 {names} 和 {notifyGroupLink}", placeholder)
	}

	return nil
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
		return errors.New("邮件服务未配置")
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
