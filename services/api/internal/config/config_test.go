package config

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	logpkg "github.com/konghang/ember/backend/internal/logging"
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
	if err := validateTimezone(" "); err == nil {
		t.Fatalf("expected blank timezone to fail")
	}
	if err := validateTimezone("Invalid/Timezone"); err == nil {
		t.Fatalf("expected invalid timezone to fail")
	}
}

func TestConfigValidationErrorWrapping(t *testing.T) {
	if wrapConfigValidationError(nil) != nil {
		t.Fatal("expected nil validation error to stay nil")
	}

	cause := errors.New("具体配置错误")
	err := wrapConfigValidationError(cause)
	if !errors.Is(err, ErrConfigValidation) {
		t.Fatalf("expected wrapped error to match ErrConfigValidation, got %v", err)
	}
	if !errors.Is(err, cause) {
		t.Fatalf("expected wrapped error to preserve cause, got %v", err)
	}
	if err.Error() != "具体配置错误" {
		t.Fatalf("expected wrapped error message from cause, got %q", err.Error())
	}

	existing := wrapConfigValidationError(ErrConfigValidation)
	if existing != ErrConfigValidation {
		t.Fatalf("expected existing validation sentinel to be reused, got %v", existing)
	}

	var nilValidation *configValidationError
	if nilValidation.Error() != ErrConfigValidation.Error() {
		t.Fatalf("expected nil validation error receiver to use sentinel message")
	}
	if nilValidation.Unwrap() != nil {
		t.Fatal("expected nil validation error receiver to unwrap nil")
	}
}

func TestBasicConfigValidators(t *testing.T) {
	validateMode := validateEnum("open", "invite")
	if err := validateMode("open"); err != nil {
		t.Fatalf("expected enum value to pass, got %v", err)
	}
	if err := validateMode("closed"); err == nil {
		t.Fatalf("expected invalid enum value to fail")
	}

	value, err := normalizeBoolean(" TRUE ")
	if err != nil {
		t.Fatalf("expected boolean normalize to pass, got %v", err)
	}
	if value != "true" {
		t.Fatalf("expected normalized boolean true, got %q", value)
	}
	if _, err := normalizeBoolean("yes"); err == nil {
		t.Fatalf("expected invalid boolean to fail")
	}

	validateRange := validateIntRange(1, 10)
	if err := validateRange(" 7 "); err != nil {
		t.Fatalf("expected integer range to pass, got %v", err)
	}
	if err := validateRange("0"); err == nil || !strings.Contains(err.Error(), "1 到 10") {
		t.Fatalf("expected range error, got %v", err)
	}
	if err := validateRange("abc"); err == nil || !strings.Contains(err.Error(), "整数") {
		t.Fatalf("expected integer parse error, got %v", err)
	}
}

func TestValidateMailAddressAllowEmpty(t *testing.T) {
	if err := validateMailAddressAllowEmpty(" "); err != nil {
		t.Fatalf("expected blank mail address to be allowed, got %v", err)
	}
	if err := validateMailAddressAllowEmpty(" Ember <ember@example.com> "); err != nil {
		t.Fatalf("expected formatted mail address to pass, got %v", err)
	}
	if err := validateMailAddressAllowEmpty("not an address"); err == nil {
		t.Fatal("expected invalid mail address to fail")
	}
}

func TestConsoleAccountLinksParsingAndNormalization(t *testing.T) {
	raw := `[
		{
			"key": " wiki ",
			"title": " Wiki ",
			"description": " Docs ",
			"url": " https://example.com/wiki ",
			"icon": " wiki ",
			"sortOrder": 20
		},
		{
			"key": "notify",
			"title": "Notify",
			"description": "Channel",
			"url": "https://example.com/notify",
			"icon": "notify",
			"sortOrder": 0
		}
	]`

	links, err := parseConsoleAccountLinks(raw)
	if err != nil {
		t.Fatalf("expected console links to parse, got %v", err)
	}
	if len(links) != 2 {
		t.Fatalf("expected two links, got %+v", links)
	}
	if links[0].Key != "notify" || links[0].SortOrder != 20 {
		t.Fatalf("expected missing sort order to default and sort by key, got %+v", links[0])
	}
	if links[1].Key != "wiki" || links[1].Title != "Wiki" || links[1].Description != "Docs" || links[1].URL != "https://example.com/wiki" {
		t.Fatalf("expected fields to be trimmed, got %+v", links[1])
	}

	normalized, err := normalizeConsoleAccountLinks(raw)
	if err != nil {
		t.Fatalf("expected console links normalization to pass, got %v", err)
	}
	roundTrip, err := parseConsoleAccountLinks(normalized)
	if err != nil {
		t.Fatalf("expected normalized console links to parse, got %v", err)
	}
	if len(roundTrip) != 2 || roundTrip[0].Key != "notify" || roundTrip[1].Key != "wiki" {
		t.Fatalf("unexpected normalized round trip: %+v", roundTrip)
	}

	empty, err := normalizeConsoleAccountLinks(" ")
	if err != nil {
		t.Fatalf("expected blank console links to normalize, got %v", err)
	}
	if empty != "" {
		t.Fatalf("expected blank console links to stay blank, got %q", empty)
	}
	if err := validateConsoleAccountLinks(raw); err != nil {
		t.Fatalf("expected console links validation to pass, got %v", err)
	}
	if err := validateConsoleAccountLinks(" "); err != nil {
		t.Fatalf("expected blank console links validation to pass, got %v", err)
	}
}

func TestConsoleAccountLinksRejectInvalidInputs(t *testing.T) {
	invalidCases := []struct {
		name string
		raw  string
	}{
		{name: "not json", raw: `not-json`},
		{name: "missing key", raw: `[{"title":"T","description":"D","url":"https://example.com","icon":"notify"}]`},
		{name: "duplicate key", raw: `[
			{"key":"a","title":"T","description":"D","url":"https://example.com/1","icon":"notify"},
			{"key":"a","title":"T","description":"D","url":"https://example.com/2","icon":"notify"}
		]`},
		{name: "missing title", raw: `[{"key":"a","description":"D","url":"https://example.com","icon":"notify"}]`},
		{name: "missing description", raw: `[{"key":"a","title":"T","url":"https://example.com","icon":"notify"}]`},
		{name: "missing url", raw: `[{"key":"a","title":"T","description":"D","icon":"notify"}]`},
		{name: "invalid url", raw: `[{"key":"a","title":"T","description":"D","url":"not-url","icon":"notify"}]`},
		{name: "invalid icon", raw: `[{"key":"a","title":"T","description":"D","url":"https://example.com","icon":"bad"}]`},
	}

	for _, tc := range invalidCases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := parseConsoleAccountLinks(tc.raw); err == nil {
				t.Fatalf("expected %s to fail", tc.name)
			}
			if _, err := normalizeConsoleAccountLinks(tc.raw); err == nil {
				t.Fatalf("expected normalize %s to fail", tc.name)
			}
			if err := validateConsoleAccountLinks(tc.raw); err == nil {
				t.Fatalf("expected validate %s to fail", tc.name)
			}
		})
	}
}

func TestStripeAllowedPaymentMethodsNormalization(t *testing.T) {
	methods, err := NormalizeStripeAllowedPaymentMethods(`[" card ","alipay","card","wechat_pay"]`)
	if err != nil {
		t.Fatalf("expected payment methods to normalize, got %v", err)
	}
	if !slices.Equal(methods, []string{"card", "alipay", "wechat_pay"}) {
		t.Fatalf("unexpected normalized methods: %+v", methods)
	}

	empty, err := NormalizeStripeAllowedPaymentMethods(" ")
	if err != nil {
		t.Fatalf("expected empty payment methods to be allowed, got %v", err)
	}
	if empty != nil {
		t.Fatalf("expected empty payment methods to return nil, got %+v", empty)
	}

	invalidInputs := []string{
		`not-json`,
		`[]`,
		`["paypal"]`,
		`[" "]`,
	}
	for _, input := range invalidInputs {
		if _, err := NormalizeStripeAllowedPaymentMethods(input); !errors.Is(err, ErrPaymentMethodSettingInvalid) {
			t.Fatalf("expected invalid payment method setting for %q, got %v", input, err)
		}
	}
}

func TestNormalizeStripePaymentMethodsReturnsCanonicalJSON(t *testing.T) {
	normalized, err := normalizeStripePaymentMethods(`["wechat_pay"," card ","wechat_pay"]`)
	if err != nil {
		t.Fatalf("expected Stripe payment method normalization to pass, got %v", err)
	}
	if normalized != `["wechat_pay","card"]` {
		t.Fatalf("unexpected normalized Stripe payment methods: %s", normalized)
	}

	empty, err := normalizeStripePaymentMethods(" ")
	if err != nil {
		t.Fatalf("expected empty Stripe payment method setting to pass, got %v", err)
	}
	if empty != "" {
		t.Fatalf("expected empty Stripe payment method setting to stay empty, got %q", empty)
	}

	if _, err := normalizeStripePaymentMethods(`["paypal"]`); !errors.Is(err, ErrPaymentMethodSettingInvalid) {
		t.Fatalf("expected unsupported payment method error, got %v", err)
	}
}

func TestTelegramApprovalAdminIDsNormalizationAndValidation(t *testing.T) {
	normalized, err := normalizeTelegramApprovalAdminIDs(" 1001, 1002,1001,, 1003 ")
	if err != nil {
		t.Fatalf("expected approval admin IDs to normalize, got %v", err)
	}
	if normalized != "1001,1002,1003" {
		t.Fatalf("unexpected normalized approval admin IDs: %q", normalized)
	}

	empty, err := normalizeTelegramApprovalAdminIDs(" , ")
	if err != nil {
		t.Fatalf("expected blank approval admin IDs to normalize, got %v", err)
	}
	if empty != "" {
		t.Fatalf("expected blank approval admin IDs to stay empty, got %q", empty)
	}

	if err := validateTelegramApprovalAdminIDs("1001,1002"); err != nil {
		t.Fatalf("expected approval admin IDs to validate, got %v", err)
	}
	for _, value := range []string{"0", "-1", "abc", "1001,abc"} {
		if err := validateTelegramApprovalAdminIDs(value); err == nil {
			t.Fatalf("expected invalid approval admin IDs %q to fail", value)
		}
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

func TestPlaybackGatewayWebConfigIsDatabaseManagedAndImmediate(t *testing.T) {
	definition, ok := getConfigDefinitionMap()[PlaybackGatewayWebEnabledKey]
	if !ok {
		t.Fatalf("expected %s definition", PlaybackGatewayWebEnabledKey)
	}
	if definition.Group != ConfigGroupMedia || definition.Type != ConfigValueBoolean ||
		definition.DefaultValue != "true" || !definition.Editable || definition.RestartRequired ||
		definition.EnvKey != "" || !definition.DisableEnvFallback {
		t.Fatalf("unexpected playback Gateway Web definition: %+v", definition)
	}
}

func TestLogLevelConfigIsDatabaseManagedAndImmediate(t *testing.T) {
	definition, ok := getConfigDefinitionMap()[LogLevelKey]
	if !ok {
		t.Fatalf("expected %s definition", LogLevelKey)
	}
	if definition.Group != ConfigGroupDeployment || definition.Type != ConfigValueEnum ||
		definition.DefaultValue != "info" || !definition.Editable || definition.RestartRequired ||
		definition.EnvKey != "" || !definition.DisableEnvFallback {
		t.Fatalf("unexpected log level definition: %+v", definition)
	}
	wantOptions := []ConfigOption{
		{Label: "信息", Value: "info"},
		{Label: "调试", Value: "debug"},
	}
	if !slices.Equal(definition.Options, wantOptions) {
		t.Fatalf("log level options=%+v, want %+v", definition.Options, wantOptions)
	}
	if definition.Normalize == nil || definition.Validate == nil {
		t.Fatal("expected log level normalization and validation")
	}
	if got, err := definition.Normalize(" DeBuG "); err != nil || got != "debug" {
		t.Fatalf("Normalize(debug)=(%q,%v), want debug", got, err)
	}
	if _, err := definition.Normalize("trace"); err == nil {
		t.Fatal("Normalize(trace) error=nil")
	}
}

func TestApplyGoRuntimeConfigSwitchesOnlyLogLevel(t *testing.T) {
	if NewConfigService().applyRuntimeConfig == nil {
		t.Fatal("NewConfigService must wire the Go runtime config applier")
	}
	t.Cleanup(func() { _ = logpkg.ApplyLevel("info") })
	if err := logpkg.ApplyLevel("info"); err != nil {
		t.Fatalf("ApplyLevel(info) error=%v", err)
	}
	debug := "debug"
	applyGoRuntimeConfig(ConfigItem{Key: LogLevelKey, Value: &debug})
	if !logpkg.DebugEnabled() {
		t.Fatal("LOG_LEVEL runtime update did not enable debug")
	}
	info := "info"
	applyGoRuntimeConfig(ConfigItem{Key: "unrelated", Value: &info})
	if !logpkg.DebugEnabled() {
		t.Fatal("unrelated config changed runtime log level")
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
		"external_api_key_hash",
		LogLevelKey,
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
		{
			key:               "external_api_key_hash",
			readOnlyHint:      "专用生成和禁用接口管理",
			missingValueLevel: ConfigRiskNone,
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
