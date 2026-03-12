package services

import "testing"

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
