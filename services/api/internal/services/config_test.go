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
