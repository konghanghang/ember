package config

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestScheduleConfigDefinitionsAreEditable(t *testing.T) {
	keys := []string{
		"CRON_ENABLED",
		"CRON_SCHEDULE",
		"CRON_TIMEZONE",
		"RANKING_CRON_ENABLED",
		"RANKING_DAILY_SCHEDULE",
		"RANKING_WEEKLY_SCHEDULE",
		"TV_CALENDAR_STARTUP_SYNC_ENABLED",
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

func TestIntegerConfigDefinitionsExposeBounds(t *testing.T) {
	testCases := []struct {
		key string
		min int
		max int
	}{
		{key: "default_trial_days", min: 0, max: 3650},
		{key: "SMTP_PORT", min: 1, max: 65535},
		{key: "EMAIL_CODE_EXPIRY_MINUTES", min: 1, max: 1440},
		{key: "EMAIL_CODE_DAILY_LIMIT", min: 1, max: 1000},
		{key: "EMAIL_CODE_IP_DAILY_LIMIT", min: 1, max: 5000},
	}

	definitions := getConfigDefinitionMap()
	for _, tc := range testCases {
		def, ok := definitions[tc.key]
		if !ok {
			t.Fatalf("expected config definition %s to exist", tc.key)
		}
		if def.MinValue == nil || *def.MinValue != tc.min {
			t.Fatalf("expected %s minValue=%d, got %+v", tc.key, tc.min, def.MinValue)
		}
		if def.MaxValue == nil || *def.MaxValue != tc.max {
			t.Fatalf("expected %s maxValue=%d, got %+v", tc.key, tc.max, def.MaxValue)
		}
	}
}

func TestRuntimeManagedConfigDefinitionsDisableEnvFallback(t *testing.T) {
	keys := []string{
		"EMBY_URL",
		"EMBY_API_KEY",
		"TMDB_API_KEY",
		"MOVIEPILOT_URL",
		"MOVIEPILOT_API_KEY",
		"SMTP_HOST",
		"SMTP_PORT",
		"SMTP_USERNAME",
		"SMTP_PASSWORD",
		"SMTP_FROM",
		"EMAIL_CODE_EXPIRY_MINUTES",
		"EMAIL_CODE_DAILY_LIMIT",
		"EMAIL_CODE_IP_DAILY_LIMIT",
		"BOT_NOTIFY_URL",
		"STRIPE_SECRET_KEY",
		"STRIPE_SUCCESS_URL",
		"STRIPE_CANCEL_URL",
		"CRON_ENABLED",
		"CRON_SCHEDULE",
		"CRON_TIMEZONE",
		"RANKING_CRON_ENABLED",
		"RANKING_DAILY_SCHEDULE",
		"RANKING_WEEKLY_SCHEDULE",
		"TV_CALENDAR_STARTUP_SYNC_ENABLED",
		"TV_CALENDAR_SYNC_SCHEDULE",
		"TELEGRAM_ADMIN_CHAT_ID",
		"TELEGRAM_GROUP_CHAT_ID",
		"turnstile_login_enabled",
		"turnstile_site_key",
		"turnstile_expected_hostname",
	}

	definitions := getConfigDefinitionMap()
	for _, key := range keys {
		def, ok := definitions[key]
		if !ok {
			t.Fatalf("expected config definition %s to exist", key)
		}
		if !def.DisableEnvFallback {
			t.Fatalf("expected %s to disable env fallback", key)
		}
	}
}

func TestMoviePilotNeedsAPIKeyMigrationWithLegacyEnv(t *testing.T) {
	t.Setenv("MOVIEPILOT_URL", "http://moviepilot.test")
	t.Setenv("MOVIEPILOT_USERNAME", "admin")
	t.Setenv("MOVIEPILOT_PASSWORD", "secret")
	t.Setenv("MOVIEPILOT_API_KEY", "")

	service := NewConfigService()
	if !service.MoviePilotNeedsAPIKeyMigration() {
		t.Fatal("expected legacy username/password config to require API key migration")
	}
}

func TestMoviePilotNeedsAPIKeyMigrationDisabledWhenAPIKeyExists(t *testing.T) {
	t.Setenv("MOVIEPILOT_URL", "http://moviepilot.test")
	t.Setenv("MOVIEPILOT_USERNAME", "admin")
	t.Setenv("MOVIEPILOT_PASSWORD", "secret")
	t.Setenv("MOVIEPILOT_API_KEY", "key")

	service := NewConfigService()
	if service.MoviePilotNeedsAPIKeyMigration() {
		t.Fatal("expected migration warning to be disabled when API key already exists")
	}
}

func TestMoviePilotConnectionUsesXAPIKey(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Fatalf("expected GET request, got %s", r.Method)
		}
		if r.URL.Path != "/api/v1/site/" {
			t.Fatalf("unexpected request path: %s", r.URL.Path)
		}
		if got := r.Header.Get("X-API-KEY"); got != "test-key" {
			t.Fatalf("expected X-API-KEY header, got %q", got)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	t.Setenv("MOVIEPILOT_URL", server.URL)
	t.Setenv("MOVIEPILOT_API_KEY", "test-key")

	service := NewConfigService()
	if err := service.testMoviePilotConnection(); err != nil {
		t.Fatalf("expected moviepilot connection test to succeed, got %v", err)
	}
}

func TestMoviePilotConnectionReturnsMigrationErrorForLegacyConfig(t *testing.T) {
	t.Setenv("MOVIEPILOT_URL", "http://moviepilot.test")
	t.Setenv("MOVIEPILOT_USERNAME", "admin")
	t.Setenv("MOVIEPILOT_PASSWORD", "secret")
	t.Setenv("MOVIEPILOT_API_KEY", "")

	service := NewConfigService()
	err := service.testMoviePilotConnection()
	if err == nil {
		t.Fatal("expected migration error for legacy moviepilot credentials")
	}
	if !strings.Contains(err.Error(), "MOVIEPILOT_API_KEY") {
		t.Fatalf("expected migration error to mention MOVIEPILOT_API_KEY, got %v", err)
	}
}

func TestBuildFallbackHintMatchesDefinitionBehavior(t *testing.T) {
	testCases := []struct {
		name     string
		def      ConfigDefinition
		expected string
	}{
		{
			name: "env then default",
			def: ConfigDefinition{
				EnvKey:       "TEST_ENV",
				DefaultValue: "fallback",
			},
			expected: "移除数据库覆盖值后将按顺序回退到环境变量、默认值。",
		},
		{
			name: "default only",
			def: ConfigDefinition{
				DefaultValue: "fallback",
			},
			expected: "移除数据库覆盖值后将回退到默认值。",
		},
		{
			name: "default empty only",
			def: ConfigDefinition{
				AllowEmpty: true,
			},
			expected: "移除数据库覆盖值后将回退到系统默认空值。",
		},
		{
			name:     "unset",
			def:      ConfigDefinition{},
			expected: "移除数据库覆盖值后将回到未设置状态。",
		},
	}

	for _, tc := range testCases {
		if got := buildFallbackHint(tc.def); got != tc.expected {
			t.Fatalf("%s: expected %q, got %q", tc.name, tc.expected, got)
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

func TestTurnstileConfigDefinitions(t *testing.T) {
	definitions := getConfigDefinitionMap()
	testCases := []struct {
		key string
	}{
		{key: "turnstile_login_enabled"},
		{key: "turnstile_site_key"},
		{key: "turnstile_expected_hostname"},
	}

	for _, tc := range testCases {
		def, ok := definitions[tc.key]
		if !ok {
			t.Fatalf("expected config definition %s to exist", tc.key)
		}
		if def.Group != ConfigGroupBusiness {
			t.Fatalf("expected %s group=%s, got %s", tc.key, ConfigGroupBusiness, def.Group)
		}
		if !def.Editable {
			t.Fatalf("expected %s to be editable", tc.key)
		}
		if !def.DisableEnvFallback {
			t.Fatalf("expected %s to disable env fallback", tc.key)
		}
		if def.Normalize == nil {
			t.Fatalf("expected %s to have normalize", tc.key)
		}
	}

	if err := definitions["turnstile_login_enabled"].Validate("true"); err != nil {
		t.Fatalf("expected turnstile_login_enabled true to be valid, got %v", err)
	}
	if err := definitions["turnstile_login_enabled"].Validate("invalid"); err == nil {
		t.Fatal("expected turnstile_login_enabled invalid value to fail")
	}

	if err := definitions["turnstile_site_key"].Validate(""); err != nil {
		t.Fatalf("expected empty turnstile_site_key to be allowed, got %v", err)
	}
	if err := definitions["turnstile_site_key"].Validate("0x4AAAA-abc123"); err != nil {
		t.Fatalf("expected valid turnstile_site_key to pass, got %v", err)
	}
	if err := definitions["turnstile_site_key"].Validate("bad key"); err == nil {
		t.Fatal("expected turnstile_site_key with whitespace to fail")
	}

	if err := definitions["turnstile_expected_hostname"].Validate(""); err != nil {
		t.Fatalf("expected empty turnstile_expected_hostname to be allowed, got %v", err)
	}
	if err := definitions["turnstile_expected_hostname"].Validate("ember.example.com"); err != nil {
		t.Fatalf("expected valid turnstile_expected_hostname to pass, got %v", err)
	}
	if err := definitions["turnstile_expected_hostname"].Validate("https://ember.example.com"); err == nil {
		t.Fatal("expected turnstile_expected_hostname containing scheme to fail")
	}
}

func TestFallbackAndDisableConfigDefinitionsAllowExplicitEmpty(t *testing.T) {
	testCases := []struct {
		key      string
		mode     ConfigEmptyValueMode
		hintPart string
	}{
		{key: "notify_group_link", mode: ConfigEmptyValueDisable, hintPart: "关闭欢迎消息"},
		{key: "console_account_links", mode: ConfigEmptyValueDisable, hintPart: "隐藏账号面板"},
		{key: "telegram_welcome_message_template", mode: ConfigEmptyValueDisable, hintPart: "关闭入群欢迎消息"},
		{key: "SMTP_FROM", mode: ConfigEmptyValueFallback, hintPart: "回退到 SMTP 用户名"},
		{key: "BOT_NOTIFY_URL", mode: ConfigEmptyValueDisable, hintPart: "关闭 API 到 Bot"},
		{key: "TELEGRAM_GROUP_CHAT_ID", mode: ConfigEmptyValueFallback, hintPart: "回退到管理员 Chat ID"},
		{key: "stripe_allowed_payment_methods", mode: ConfigEmptyValueInherit, hintPart: "Stripe Dashboard"},
		{key: "turnstile_site_key", mode: ConfigEmptyValueDisable, hintPart: "无法渲染 Turnstile"},
		{key: "turnstile_expected_hostname", mode: ConfigEmptyValueDisable, hintPart: "不校验 hostname"},
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

func TestConsoleAccountLinksValidation(t *testing.T) {
	definitions := getConfigDefinitionMap()
	def, ok := definitions["console_account_links"]
	if !ok {
		t.Fatal("expected console_account_links definition to exist")
	}
	if !def.Multiline {
		t.Fatal("expected console_account_links to be multiline")
	}
	if def.DefaultValue == "" || !strings.Contains(def.DefaultValue, "\"notify-channel\"") {
		t.Fatal("expected default account links to contain notify-channel")
	}

	normalized, err := def.Normalize(`[
  {
    "key": "wiki",
    "title": "使用 Wiki",
    "description": "查看说明",
    "url": "https://example.com/wiki",
    "icon": "wiki"
  }
]`)
	if err != nil {
		t.Fatalf("expected normalize success, got %v", err)
	}
	if !strings.Contains(normalized, "\"sortOrder\": 10") {
		t.Fatalf("expected normalize to fill sortOrder, got %q", normalized)
	}
	if err := def.Validate(normalized); err != nil {
		t.Fatalf("expected valid account links, got %v", err)
	}
	if err := def.Validate(`[{"key":"broken","title":"","description":"x","url":"https://example.com","icon":"notify"}]`); err == nil {
		t.Fatal("expected empty title to fail")
	}
	if err := def.Validate(`[{"key":"broken","title":"x","description":"x","url":"https://example.com","icon":"unknown"}]`); err == nil {
		t.Fatal("expected unsupported icon to fail")
	}
	if err := def.Validate(""); err != nil {
		t.Fatalf("expected empty account links to be allowed, got %v", err)
	}
}

func TestTelegramWelcomeMessageTemplateValidation(t *testing.T) {
	definitions := getConfigDefinitionMap()
	def, ok := definitions["telegram_welcome_message_template"]
	if !ok {
		t.Fatal("expected telegram_welcome_message_template definition to exist")
	}
	if !def.Multiline {
		t.Fatal("expected telegram_welcome_message_template to be multiline")
	}
	if def.DefaultValue == "" || !strings.Contains(def.DefaultValue, "{names}") {
		t.Fatal("expected default welcome template to contain {names}")
	}

	validValue, err := def.Normalize("  👋 欢迎 <b>{names}</b> 加入！\r\n\r\n📢 入库通知群组：{notifyGroupLink}\r\n  ")
	if err != nil {
		t.Fatalf("expected normalize success, got %v", err)
	}
	if strings.Contains(validValue, "\r") {
		t.Fatalf("expected normalized template to remove carriage returns, got %q", validValue)
	}
	if err := def.Validate(validValue); err != nil {
		t.Fatalf("expected valid welcome template, got %v", err)
	}
	if err := def.Validate("欢迎加入"); err == nil {
		t.Fatal("expected template without {names} to fail")
	}
	if err := def.Validate("欢迎 {names}\n链接 {groupLink}"); err == nil {
		t.Fatal("expected template with unsupported placeholder to fail")
	}
	if err := def.Validate(""); err != nil {
		t.Fatalf("expected empty template to be allowed, got %v", err)
	}
}

func TestReadOnlyBoundaryConfigDefinitionsExposeHints(t *testing.T) {
	testCases := []struct {
		key               string
		readOnlyHint      string
		missingValueHint  string
		missingValueLevel ConfigRiskLevel
	}{
		{
			key:               "STRIPE_WEBHOOK_SECRET",
			readOnlyHint:      "部署环境注入",
			missingValueHint:  "支付状态同步会失败",
			missingValueLevel: ConfigRiskCritical,
		},
		{
			key:               "TELEGRAM_BOT_TOKEN",
			readOnlyHint:      "部署环境读取",
			missingValueHint:  "Telegram Bot 无法启动",
			missingValueLevel: ConfigRiskCritical,
		},
		{
			key:               "TELEGRAM_UPDATE_MODE",
			readOnlyHint:      "Telegram 接入方式",
			missingValueLevel: ConfigRiskNone,
		},
		{
			key:               "TELEGRAM_WEBHOOK_SECRET",
			readOnlyHint:      "Webhook 配置保持一致",
			missingValueHint:  "Webhook 安全边界不完整",
			missingValueLevel: ConfigRiskCritical,
		},
		{
			key:               "WEBHOOK_URL",
			readOnlyHint:      "部署拓扑",
			missingValueHint:  "Webhook 模式无法正常接入公网请求",
			missingValueLevel: ConfigRiskCritical,
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
		if tc.missingValueHint != "" && !strings.Contains(def.MissingValueHint, tc.missingValueHint) {
			t.Fatalf("expected %s MissingValueHint to contain %q, got %q", tc.key, tc.missingValueHint, def.MissingValueHint)
		}
		if def.MissingValueLevel != tc.missingValueLevel {
			t.Fatalf("expected %s MissingValueLevel=%s, got %s", tc.key, tc.missingValueLevel, def.MissingValueLevel)
		}
	}
}
