package user

import (
	"github.com/konghang/ember/backend/internal/models"
	"testing"
	"time"
)

// TestWatchRetentionDisplay uses server state for disabled, waiting, first-check and invalidated copy.
func TestWatchRetentionDisplay(t *testing.T) {
	loc, _ := time.LoadLocation("Asia/Shanghai")
	start := time.Date(2026, 10, 8, 15, 0, 0, 0, loc)
	for _, tc := range []struct {
		name                      string
		enabled, current, invalid bool
		want                      string
	}{
		{"disabled", false, true, false, ""}, {"waiting", true, false, false, "waiting"}, {"grace", true, true, false, "grace"}, {"invalidated", true, false, true, "invalidated"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			row := EntitlementView{UserEntitlement: models.UserEntitlement{ValidityType: "permanent", WatchRetentionStartedAt: &start}, RetentionEnabled: tc.enabled, RetentionDays: 30, RetentionMinutes: 60, Current: tc.current}
			if tc.invalid {
				row.WatchRetentionInvalidatedAt = &start
			}
			if err := attachWatchRetentionView(&row, start, "0 2 * * *", loc); err != nil {
				t.Fatal(err)
			}
			if tc.want == "" {
				if row.WatchRetention != nil {
					t.Fatal("disabled display")
				}
				return
			}
			if row.WatchRetention == nil || row.WatchRetention.State != tc.want {
				t.Fatalf("%+v", row.WatchRetention)
			}
			if tc.want == "grace" && !row.WatchRetention.FirstCheckAt.Equal(time.Date(2026, 11, 8, 2, 0, 0, 0, loc)) {
				t.Fatal("wrong first check")
			}
		})
	}
}
