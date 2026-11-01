package config

import (
	"context"
	"strconv"
	"strings"

	"github.com/konghang/ember/backend/internal/models"
)

var compatibilityLimitKeys = []string{
	"P115_COMPAT_USER_HOURLY_LIMIT", "P115_COMPAT_USER_DAILY_LIMIT",
	"P115_COMPAT_ACCOUNT_HOURLY_LIMIT", "P115_COMPAT_ACCOUNT_DAILY_LIMIT",
}

// CompatibilityTransferLimits bounds GET-only transfers without explicit or
// successfully matched session evidence. Total plan quotas still apply.
type CompatibilityTransferLimits struct {
	UserHourly, UserDaily, AccountHourly, AccountDaily int
}

// P115CompatibilityTransferLimits loads database-only limits through the same
// five-second cache as the mode. Invalid or unavailable records fail closed.
func (s *ConfigService) P115CompatibilityTransferLimits(ctx context.Context) (CompatibilityTransferLimits, error) {
	settings, err := globalSettingsCacheStore.loadManyWithTTL(ctx, compatibilityLimitKeys, gatewayRuntimeSettingCacheTTL,
		func(keys []string) (map[string]models.Setting, error) {
			return s.loadSettingsFromStoreContext(ctx, keys)
		})
	if err != nil {
		return CompatibilityTransferLimits{}, err
	}
	values := make([]int, len(compatibilityLimitKeys))
	for index, key := range compatibilityLimitKeys {
		definition := getConfigDefinitionMap()[key]
		raw := definition.DefaultValue
		if setting, exists := settings[key]; exists {
			raw = setting.Value
		}
		if err := definition.Validate(raw); err != nil {
			return CompatibilityTransferLimits{}, wrapConfigValidationError(err)
		}
		values[index], err = strconv.Atoi(strings.TrimSpace(raw))
		if err != nil {
			return CompatibilityTransferLimits{}, wrapConfigValidationError(err)
		}
	}
	return CompatibilityTransferLimits{UserHourly: values[0], UserDaily: values[1], AccountHourly: values[2], AccountDaily: values[3]}, nil
}
