package models

import (
	"testing"
	"time"
)

// TestAccessExpiryUsesReconciledState preserves scheduled expiry while distinguishing an empty grant set.
func TestAccessExpiryUsesReconciledState(t *testing.T) {
	past := time.Now().Add(-time.Hour)
	for _, test := range []struct {
		name string
		user User
		want bool
	}{
		{"waiting_for_cron", User{Role: "user", ResourceAccessGranted: true, ExpiresAt: &past}, false},
		{"no_grants_is_not_permanent", User{Role: "user"}, true},
		{"permanent", User{Role: "user", ResourceAccessGranted: true}, false},
		{"admin_preserves_expiry_display", User{Role: "admin", ExpiresAt: &past}, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := test.user.IsAccessExpired(); got != test.want {
				t.Fatalf("expired=%t want=%t", got, test.want)
			}
		})
	}
}

// TestLegacyRedemptionCodeCannotBeUsed prevents migration-invalidated codes from reopening registration or renewal.
func TestLegacyRedemptionCodeCannotBeUsed(t *testing.T) {
	code := RedemptionCode{MaxUses: 10, DefaultDays: 30, LegacyInvalidated: true}
	if code.IsValid() {
		t.Fatal("legacy code remained valid")
	}
	code.LegacyInvalidated = false
	if !code.IsValid() {
		t.Fatal("new code was invalidated")
	}
}
