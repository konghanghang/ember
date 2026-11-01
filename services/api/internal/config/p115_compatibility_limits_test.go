package config

import (
	"context"
	"errors"
	"testing"

	"github.com/konghang/ember/backend/internal/models"
)

// TestCompatibilityLimitsRuntime checks database-only defaults, fresh changes
// and fail-closed validation rather than relying on UI constraints.
func TestCompatibilityLimitsRuntime(t *testing.T) {
	resetSettingsCacheForTest()
	t.Cleanup(resetSettingsCacheForTest)
	records := map[string]models.Setting{}
	service := &ConfigService{loadSettingRecords: func(keys []string) (map[string]models.Setting, error) { return records, nil }}
	limits, err := service.P115CompatibilityTransferLimits(context.Background())
	if err != nil || limits.UserHourly != 1 || limits.UserDaily != 3 || limits.AccountHourly != 10 || limits.AccountDaily != 30 {
		t.Fatalf("defaults=%+v, %v", limits, err)
	}
	key := "P115_COMPAT_USER_HOURLY_LIMIT"
	t.Setenv(key, "99")
	records[key] = models.Setting{Key: key, Value: "2"}
	InvalidateCachedSetting(key)
	limits, err = service.P115CompatibilityTransferLimits(context.Background())
	if err != nil || limits.UserHourly != 2 {
		t.Fatalf("stored=%+v, %v", limits, err)
	}
	records[key] = models.Setting{Key: key, Value: "0"}
	InvalidateCachedSetting(key)
	if _, err = service.P115CompatibilityTransferLimits(context.Background()); !errors.Is(err, ErrConfigValidation) {
		t.Fatalf("invalid limit error=%v", err)
	}
	for _, key := range compatibilityLimitKeys {
		d, ok := getConfigDefinitionMap()[key]
		if !ok || !d.DisableEnvFallback || d.EnvKey != "" || d.RestartRequired || d.Type != ConfigValueInteger {
			t.Fatalf("invalid definition for %s", key)
		}
	}
}
