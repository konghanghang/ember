package redemption

import (
	"testing"
	"time"
)

func TestCalculateRedeemedExpiryStartsFromNowWithoutActiveExpiry(t *testing.T) {
	now := time.Date(2026, 6, 17, 10, 0, 0, 0, time.UTC)
	expiredAt := now.Add(-2 * time.Hour)

	tests := []struct {
		name          string
		currentExpiry *time.Time
	}{
		{name: "nil expiry", currentExpiry: nil},
		{name: "expired expiry", currentExpiry: &expiredAt},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := calculateRedeemedExpiry(now, tt.currentExpiry, 30)
			want := now.AddDate(0, 0, 30)

			if !got.Equal(want) {
				t.Fatalf("expected expiry from now %s, got %s", want, got)
			}
		})
	}
}

func TestCalculateRedeemedExpiryExtendsActiveExpiry(t *testing.T) {
	now := time.Date(2026, 6, 17, 10, 0, 0, 0, time.UTC)
	currentExpiry := now.AddDate(0, 0, 12)

	got := calculateRedeemedExpiry(now, &currentExpiry, 45)
	want := currentExpiry.AddDate(0, 0, 45)

	if !got.Equal(want) {
		t.Fatalf("expected active expiry extension %s, got %s", want, got)
	}
}
