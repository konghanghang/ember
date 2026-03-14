package config

import (
	"encoding/json"
	"slices"
	"strings"
)

var allowedStripePaymentMethods = []string{"card", "alipay", "wechat_pay"}

func NormalizeStripeAllowedPaymentMethods(raw string) ([]string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}

	var methods []string
	if err := json.Unmarshal([]byte(raw), &methods); err != nil {
		return nil, ErrPaymentMethodSettingInvalid
	}

	normalized := make([]string, 0, len(methods))
	seen := make(map[string]struct{}, len(methods))
	for _, method := range methods {
		method = strings.TrimSpace(method)
		if !slices.Contains(allowedStripePaymentMethods, method) {
			return nil, ErrPaymentMethodSettingInvalid
		}
		if _, exists := seen[method]; exists {
			continue
		}
		seen[method] = struct{}{}
		normalized = append(normalized, method)
	}

	if len(normalized) == 0 {
		return nil, ErrPaymentMethodSettingInvalid
	}

	return normalized, nil
}
