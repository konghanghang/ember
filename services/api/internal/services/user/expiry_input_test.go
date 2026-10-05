package user

import (
	"testing"
	"time"
)

// TestParseAdminExpiryInput locks global-timezone wall time and legacy RFC3339 semantics.
func TestParseAdminExpiryInput(t *testing.T) {
	location, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		t.Fatal(err)
	}
	for _, input := range []string{"2099-02-01 12:00:00", "2099-02-01T04:00:00Z", "2099-02-01T12:00:00+08:00"} {
		got, err := parseAdminExpiryInput(input, location)
		if err != nil || !got.Equal(time.Date(2099, 2, 1, 4, 0, 0, 0, time.UTC)) {
			t.Fatalf("input=%s got=%v err=%v", input, got, err)
		}
	}
	if _, err := parseAdminExpiryInput("not-a-date", location); err == nil {
		t.Fatal("invalid expiry accepted")
	}
}
