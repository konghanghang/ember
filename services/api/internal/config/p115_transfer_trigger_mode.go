package config

import (
	"context"
	"errors"
	"strings"
)

const (
	// P115TransferTriggerModeKey is the database-only administrator policy key.
	P115TransferTriggerModeKey = "P115_TRANSFER_TRIGGER_MODE"
	// TransferTriggerModeStrict requires the existing explicit playback permit.
	TransferTriggerModeStrict = "strict"
	// TransferTriggerModeCompatible allows separately validated video requests.
	TransferTriggerModeCompatible = "compatible"
)

// P115TransferTriggerMode reads the database-only policy through the short
// Gateway runtime cache. Missing records use strict mode; invalid records and
// refresh failures return no mode so callers cannot reuse stale compatibility.
func (s *ConfigService) P115TransferTriggerMode(ctx context.Context) (string, error) {
	settings, err := s.loadGatewayRuntimeSetting(ctx, P115TransferTriggerModeKey)
	if err != nil {
		return "", err
	}
	value := TransferTriggerModeStrict
	if setting, found := settings[P115TransferTriggerModeKey]; found {
		// An explicitly stored empty value is invalid, not a missing policy.
		value = setting.Value
	}
	mode, err := normalizeP115TransferTriggerMode(value)
	if err != nil {
		return "", wrapConfigValidationError(err)
	}
	return mode, nil
}

// normalizeP115TransferTriggerMode canonicalizes the two supported policies
// without including untrusted persisted or submitted values in errors.
func normalizeP115TransferTriggerMode(value string) (string, error) {
	normalized := strings.ToLower(strings.TrimSpace(value))
	switch normalized {
	case TransferTriggerModeStrict, TransferTriggerModeCompatible:
		return normalized, nil
	default:
		return "", errors.New("首次转存策略只支持 strict 或 compatible")
	}
}
