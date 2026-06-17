package common

import (
	"testing"
	"time"
)

func TestCalculateExpiryDateAddsDaysFromCurrentUTC(t *testing.T) {
	before := time.Now().UTC().AddDate(0, 0, 30)
	got := CalculateExpiryDate(30)
	after := time.Now().UTC().AddDate(0, 0, 30)

	if got.Before(before) || got.After(after) {
		t.Fatalf("expected expiry between %s and %s, got %s", before, after, got)
	}
	if got.Location() != time.UTC {
		t.Fatalf("expected UTC expiry, got location %s", got.Location())
	}
}

func TestCalculateExpiryDateAllowsZeroAndNegativeDays(t *testing.T) {
	beforeNow := time.Now().UTC()
	gotNow := CalculateExpiryDate(0)
	afterNow := time.Now().UTC()
	if gotNow.Before(beforeNow) || gotNow.After(afterNow) {
		t.Fatalf("expected zero-day expiry between %s and %s, got %s", beforeNow, afterNow, gotNow)
	}

	beforePast := time.Now().UTC().AddDate(0, 0, -7)
	gotPast := CalculateExpiryDate(-7)
	afterPast := time.Now().UTC().AddDate(0, 0, -7)
	if gotPast.Before(beforePast) || gotPast.After(afterPast) {
		t.Fatalf("expected negative-day expiry between %s and %s, got %s", beforePast, afterPast, gotPast)
	}
}
