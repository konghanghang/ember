package payment

import (
	"errors"
	"github.com/konghang/ember/backend/internal/models"
	"time"
)

var ErrWatchRetentionInvalid = errors.New("观看保号周期须为 1..3650 天，开启时最低观看分钟数须为 1..5256000")

// applyWatchRetentionPolicy validates the complete configuration and resets grace only when enabled rules change.
func applyWatchRetentionPolicy(group *models.PlanGroup, enabled *bool, days, minutes *int, at time.Time) error {
	next := *group
	if next.WatchRetentionDays == 0 {
		next.WatchRetentionDays = 30
	}
	if enabled != nil {
		next.WatchRetentionEnabled = *enabled
	}
	if days != nil {
		next.WatchRetentionDays = *days
	}
	if minutes != nil {
		next.WatchRetentionMinMinutes = *minutes
	}
	if next.WatchRetentionDays < 1 || next.WatchRetentionDays > 3650 || next.WatchRetentionMinMinutes < 0 || next.WatchRetentionMinMinutes > 5256000 || next.WatchRetentionEnabled && next.WatchRetentionMinMinutes == 0 {
		return ErrWatchRetentionInvalid
	}
	if next.WatchRetentionEnabled && (!group.WatchRetentionEnabled || next.WatchRetentionDays != group.WatchRetentionDays || next.WatchRetentionMinMinutes != group.WatchRetentionMinMinutes) {
		next.WatchRetentionResetAt = &at
	}
	*group = next
	return nil
}
