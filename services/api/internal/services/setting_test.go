package services

import (
	"testing"

	"github.com/konghang/ember/backend/internal/db"
)

func TestSettingServiceGetAllSettingsReturnsLegacyCompatibilityKeys(t *testing.T) {
	originalDB := db.DB
	db.DB = nil
	defer func() {
		db.DB = originalDB
	}()

	service := &SettingService{}
	settings, err := service.GetAllSettings()
	if err != nil {
		t.Fatalf("GetAllSettings returned error: %v", err)
	}

	if len(settings) != len(legacySettingKeys) {
		t.Fatalf("expected %d legacy settings, got %d", len(legacySettingKeys), len(settings))
	}

	expectedValues := map[string]string{
		settingRegistrationMode:           "open",
		settingDefaultTrialDays:           "7",
		settingNotifyGroupLink:            "",
		settingEmailVerification:          "false",
		settingStripeAllowedPaymentMethod: "",
	}

	for _, setting := range settings {
		expectedValue, ok := expectedValues[setting.Key]
		if !ok {
			t.Fatalf("unexpected legacy setting key: %s", setting.Key)
		}
		if setting.Value != expectedValue {
			t.Fatalf("expected %s value %q, got %q", setting.Key, expectedValue, setting.Value)
		}
	}
}

func TestSettingServiceGetSettingModelRejectsNonLegacyKey(t *testing.T) {
	service := &SettingService{}
	if _, err := service.GetSettingModel("EMBY_URL"); err != ErrSettingNotFound {
		t.Fatalf("expected ErrSettingNotFound, got %v", err)
	}
}
