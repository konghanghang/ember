package payment

import (
	"github.com/konghang/ember/backend/internal/models"
	"testing"
	"time"
)

// TestWatchRetentionConfiguration resets grace only for meaningful enabled policy changes.
func TestWatchRetentionConfiguration(t *testing.T) {
	now := time.Now()
	enabled := true
	days := 14
	minutes := 60
	group := models.PlanGroup{WatchRetentionDays: 30}
	if err := applyWatchRetentionPolicy(&group, &enabled, &days, &minutes, now); err != nil {
		t.Fatal(err)
	}
	if group.WatchRetentionResetAt == nil || !group.WatchRetentionResetAt.Equal(now) {
		t.Fatal("no reset")
	}
	later := now.Add(time.Hour)
	if err := applyWatchRetentionPolicy(&group, &enabled, &days, &minutes, later); err != nil || !group.WatchRetentionResetAt.Equal(now) {
		t.Fatal("unchanged config reset")
	}
	days = 15
	if err := applyWatchRetentionPolicy(&group, nil, &days, nil, later); err != nil || !group.WatchRetentionResetAt.Equal(later) {
		t.Fatal("change did not reset")
	}
	minutes = 0
	if err := applyWatchRetentionPolicy(&group, nil, nil, &minutes, later); err == nil {
		t.Fatal("accepted enabled zero")
	}
}
