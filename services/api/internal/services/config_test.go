package services

import (
	"strings"
	"testing"

	"github.com/konghang/ember/backend/internal/models"
)

func TestScheduleConfigDefinitionsAreEditable(t *testing.T) {
	keys := []string{
		"CRON_ENABLED",
		"CRON_SCHEDULE",
		"CRON_TIMEZONE",
		"RANKING_CRON_ENABLED",
		"RANKING_DAILY_SCHEDULE",
		"RANKING_WEEKLY_SCHEDULE",
		"TV_CALENDAR_SYNC_SCHEDULE",
	}

	definitions := getConfigDefinitionMap()
	for _, key := range keys {
		def, ok := definitions[key]
		if !ok {
			t.Fatalf("expected config definition %s to exist", key)
		}
		if !def.Editable {
			t.Fatalf("expected %s to be editable", key)
		}
		if !def.RestartRequired {
			t.Fatalf("expected %s to require restart", key)
		}
		if def.Validate == nil {
			t.Fatalf("expected %s to have validation", key)
		}
		if def.Normalize == nil {
			t.Fatalf("expected %s to have normalization", key)
		}
	}
}

func TestValidateCronExpression(t *testing.T) {
	validCases := []string{
		"0 2 * * *",
		"30 20 * * 0",
		"0 */12 * * *",
	}

	for _, value := range validCases {
		if err := validateCronExpression(value); err != nil {
			t.Fatalf("expected %q to be valid, got %v", value, err)
		}
	}

	invalidCases := []string{
		"",
		"not-a-cron",
		"61 2 * * *",
	}

	for _, value := range invalidCases {
		if err := validateCronExpression(value); err == nil {
			t.Fatalf("expected %q to be invalid", value)
		}
	}
}

func TestValidateTimezone(t *testing.T) {
	if err := validateTimezone("Asia/Shanghai"); err != nil {
		t.Fatalf("expected valid timezone, got %v", err)
	}
	if err := validateTimezone("Invalid/Timezone"); err == nil {
		t.Fatalf("expected invalid timezone to fail")
	}
}

func TestPaymentAndTelegramConfigDefinitionsAreEditable(t *testing.T) {
	testCases := []struct {
		key             string
		restartRequired bool
	}{
		{key: "STRIPE_SECRET_KEY", restartRequired: false},
		{key: "STRIPE_SUCCESS_URL", restartRequired: false},
		{key: "STRIPE_CANCEL_URL", restartRequired: false},
		{key: "TELEGRAM_ADMIN_CHAT_ID", restartRequired: false},
		{key: "TELEGRAM_GROUP_CHAT_ID", restartRequired: false},
	}

	definitions := getConfigDefinitionMap()
	for _, tc := range testCases {
		def, ok := definitions[tc.key]
		if !ok {
			t.Fatalf("expected config definition %s to exist", tc.key)
		}
		if !def.Editable {
			t.Fatalf("expected %s to be editable", tc.key)
		}
		if def.RestartRequired != tc.restartRequired {
			t.Fatalf("expected %s restartRequired=%v, got %v", tc.key, tc.restartRequired, def.RestartRequired)
		}
	}
}

func TestValidateTelegramChatID(t *testing.T) {
	if err := validateTelegramPositiveChatID("123456"); err != nil {
		t.Fatalf("expected valid admin chat id, got %v", err)
	}
	if err := validateTelegramPositiveChatID("-100123"); err == nil {
		t.Fatalf("expected negative admin chat id to fail")
	}
	if err := validateTelegramSignedChatIDAllowEmpty(""); err != nil {
		t.Fatalf("expected empty group chat id to be allowed, got %v", err)
	}
	if err := validateTelegramSignedChatIDAllowEmpty("-1001234567890"); err != nil {
		t.Fatalf("expected valid group chat id, got %v", err)
	}
	if err := validateTelegramSignedChatIDAllowEmpty("0"); err == nil {
		t.Fatalf("expected zero group chat id to fail")
	}
}

func TestFallbackAndDisableConfigDefinitionsAllowExplicitEmpty(t *testing.T) {
	testCases := []struct {
		key      string
		mode     ConfigEmptyValueMode
		hintPart string
	}{
		{key: "notify_group_link", mode: ConfigEmptyValueDisable, hintPart: "关闭欢迎消息"},
		{key: "NEXT_PUBLIC_EMBY_URL", mode: ConfigEmptyValueFallback, hintPart: "回退到 Emby 服务地址"},
		{key: "SMTP_FROM", mode: ConfigEmptyValueFallback, hintPart: "回退到 SMTP 用户名"},
		{key: "BOT_NOTIFY_URL", mode: ConfigEmptyValueDisable, hintPart: "关闭 API 到 Bot"},
		{key: "TELEGRAM_GROUP_CHAT_ID", mode: ConfigEmptyValueFallback, hintPart: "回退到管理员 Chat ID"},
		{key: "stripe_allowed_payment_methods", mode: ConfigEmptyValueInherit, hintPart: "Stripe Dashboard"},
	}

	definitions := getConfigDefinitionMap()
	for _, tc := range testCases {
		def, ok := definitions[tc.key]
		if !ok {
			t.Fatalf("expected config definition %s to exist", tc.key)
		}
		if !def.AllowEmpty {
			t.Fatalf("expected %s to allow explicit empty value", tc.key)
		}
		if def.EmptyValueMode != tc.mode {
			t.Fatalf("expected %s emptyValueMode=%s, got %s", tc.key, tc.mode, def.EmptyValueMode)
		}
		if def.EmptyValueHint == "" || !strings.Contains(def.EmptyValueHint, tc.hintPart) {
			t.Fatalf("expected %s emptyValueHint to contain %q, got %q", tc.key, tc.hintPart, def.EmptyValueHint)
		}
		if def.Validate == nil {
			t.Fatalf("expected %s to have validation", tc.key)
		}
		if err := def.Validate(""); err != nil {
			t.Fatalf("expected %s to accept empty value, got %v", tc.key, err)
		}
		if def.Normalize == nil {
			t.Fatalf("expected %s to have normalization", tc.key)
		}
		value, err := def.Normalize("   ")
		if err != nil {
			t.Fatalf("expected %s to normalize empty value, got %v", tc.key, err)
		}
		if value != "" {
			t.Fatalf("expected %s normalized empty value to be blank, got %q", tc.key, value)
		}
	}
}

func TestReadOnlyBoundaryConfigDefinitionsExposeHints(t *testing.T) {
	testCases := []struct {
		key              string
		readOnlyHint     string
		missingValueHint string
	}{
		{
			key:              "STRIPE_WEBHOOK_SECRET",
			readOnlyHint:     "部署环境注入",
			missingValueHint: "支付状态同步会失败",
		},
		{
			key:              "TELEGRAM_BOT_TOKEN",
			readOnlyHint:     "部署环境读取",
			missingValueHint: "Telegram Bot 无法启动",
		},
		{
			key:              "TELEGRAM_WEBHOOK_SECRET",
			readOnlyHint:     "Webhook 配置保持一致",
			missingValueHint: "Webhook 安全边界不完整",
		},
		{
			key:              "WEBHOOK_URL",
			readOnlyHint:     "部署拓扑",
			missingValueHint: "Webhook 模式无法正常接入公网请求",
		},
	}

	definitions := getConfigDefinitionMap()
	for _, tc := range testCases {
		def, ok := definitions[tc.key]
		if !ok {
			t.Fatalf("expected config definition %s to exist", tc.key)
		}
		if def.Editable {
			t.Fatalf("expected %s to be read-only", tc.key)
		}
		if !strings.Contains(def.ReadOnlyHint, tc.readOnlyHint) {
			t.Fatalf("expected %s ReadOnlyHint to contain %q, got %q", tc.key, tc.readOnlyHint, def.ReadOnlyHint)
		}
		if !strings.Contains(def.MissingValueHint, tc.missingValueHint) {
			t.Fatalf("expected %s MissingValueHint to contain %q, got %q", tc.key, tc.missingValueHint, def.MissingValueHint)
		}
		if def.MissingValueLevel != ConfigRiskCritical {
			t.Fatalf("expected %s MissingValueLevel=%s, got %s", tc.key, ConfigRiskCritical, def.MissingValueLevel)
		}
	}
}

func TestImportEnvDefinitionsImportsNormalizedValues(t *testing.T) {
	definitions := getConfigDefinitionMap()
	service := &ConfigService{}
	settingsMap := map[string]models.Setting{}
	envValues := map[string]string{
		"EMBY_URL":               " https://emby.example.com/ ",
		"TELEGRAM_GROUP_CHAT_ID": "-1001234567890",
	}
	updated := map[string]string{}

	result := service.importEnvDefinitions(
		[]ConfigDefinition{
			definitions["EMBY_URL"],
			definitions["TELEGRAM_GROUP_CHAT_ID"],
		},
		settingsMap,
		"admin-user",
		func(key string) string {
			return envValues[key]
		},
		func(key string, req UpdateConfigRequest, updatedByUserID string) (*ConfigItem, error) {
			if updatedByUserID != "admin-user" {
				t.Fatalf("expected updatedByUserID=admin-user, got %s", updatedByUserID)
			}
			if req.Value == nil {
				t.Fatalf("expected %s to provide update value", key)
			}
			updated[key] = *req.Value
			return &ConfigItem{Key: key}, nil
		},
	)

	if len(result.Failed) != 0 {
		t.Fatalf("expected no failed items, got %+v", result.Failed)
	}
	if len(result.Skipped) != 0 {
		t.Fatalf("expected no skipped items, got %+v", result.Skipped)
	}
	if len(result.Imported) != 2 {
		t.Fatalf("expected 2 imported items, got %+v", result.Imported)
	}
	if updated["EMBY_URL"] != "https://emby.example.com" {
		t.Fatalf("expected EMBY_URL normalized, got %q", updated["EMBY_URL"])
	}
	if updated["TELEGRAM_GROUP_CHAT_ID"] != "-1001234567890" {
		t.Fatalf("unexpected TELEGRAM_GROUP_CHAT_ID value: %q", updated["TELEGRAM_GROUP_CHAT_ID"])
	}
}

func TestImportEnvDefinitionsSkipsDatabaseOverrideAndUnsetEnv(t *testing.T) {
	definitions := getConfigDefinitionMap()
	service := &ConfigService{}
	settingsMap := map[string]models.Setting{
		"NEXT_PUBLIC_EMBY_URL": {
			Key:   "NEXT_PUBLIC_EMBY_URL",
			Value: "",
		},
	}

	result := service.importEnvDefinitions(
		[]ConfigDefinition{
			definitions["NEXT_PUBLIC_EMBY_URL"],
			definitions["EMBY_URL"],
		},
		settingsMap,
		"admin-user",
		func(key string) string {
			if key == "NEXT_PUBLIC_EMBY_URL" {
				return "https://public.example.com"
			}
			return ""
		},
		func(key string, req UpdateConfigRequest, updatedByUserID string) (*ConfigItem, error) {
			t.Fatalf("unexpected update call for %s", key)
			return nil, nil
		},
	)

	if got := result.Skipped["NEXT_PUBLIC_EMBY_URL"]; got != "数据库已存在覆盖值" {
		t.Fatalf("expected NEXT_PUBLIC_EMBY_URL skip reason, got %q", got)
	}
	if got := result.Skipped["EMBY_URL"]; got != "环境变量未设置" {
		t.Fatalf("expected EMBY_URL unset skip reason, got %q", got)
	}
}

func TestImportEnvDefinitionsCollectsValidationAndUpdateFailures(t *testing.T) {
	definitions := getConfigDefinitionMap()
	service := &ConfigService{}

	result := service.importEnvDefinitions(
		[]ConfigDefinition{
			definitions["EMBY_URL"],
			definitions["TMDB_API_KEY"],
		},
		map[string]models.Setting{},
		"admin-user",
		func(key string) string {
			switch key {
			case "EMBY_URL":
				return "not-a-url"
			case "TMDB_API_KEY":
				return "tmdb-secret"
			default:
				return ""
			}
		},
		func(key string, req UpdateConfigRequest, updatedByUserID string) (*ConfigItem, error) {
			if key != "TMDB_API_KEY" {
				t.Fatalf("unexpected update call for %s", key)
			}
			return nil, ErrConfigEncryptionKeyMissing
		},
	)

	if got := result.Failed["EMBY_URL"]; got == "" {
		t.Fatal("expected EMBY_URL validation failure")
	}
	if got := result.Failed["TMDB_API_KEY"]; got != ErrConfigEncryptionKeyMissing.Error() {
		t.Fatalf("expected TMDB_API_KEY update failure, got %q", got)
	}
	if len(result.Imported) != 0 {
		t.Fatalf("expected no imported items, got %+v", result.Imported)
	}
}
