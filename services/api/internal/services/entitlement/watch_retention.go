package entitlement

import (
	"errors"
	"github.com/robfig/cron/v3"
	"strings"
	"sync/atomic"
	"time"
)

// FirstWatchRetentionCheck returns the first configured run at or after a full business-calendar period.
func FirstWatchRetentionCheck(start time.Time, days int, expression string, location *time.Location) (time.Time, error) {
	if location == nil || days < 1 || days > 3650 {
		return time.Time{}, errors.New("观看保号周期须为 1..3650 天")
	}
	if strings.Contains(expression, "TZ=") {
		return time.Time{}, errors.New("观看保号计划必须使用全局业务时区")
	}
	schedule, err := cron.ParseStandard(expression)
	if err != nil {
		return time.Time{}, err
	}
	ready := start.In(location).AddDate(0, 0, days)
	next := schedule.Next(ready.Add(-time.Nanosecond))
	if next.IsZero() {
		return time.Time{}, errors.New("观看保号计划没有下一次执行时间")
	}
	return next, nil
}

// RetentionSchedule is the process's registered schedule, not a pending settings change requiring restart.
type RetentionSchedule struct {
	Expression string
	Location   *time.Location
	Enabled    bool
}

var registeredRetentionSchedule atomic.Pointer[RetentionSchedule]

// RegisterRetentionSchedule publishes the same immutable schedule used by the cron process.
func RegisterRetentionSchedule(expression string, location *time.Location, enabled bool) {
	registeredRetentionSchedule.Store(&RetentionSchedule{Expression: expression, Location: location, Enabled: enabled})
}

// RegisteredRetentionSchedule returns nil before process assembly (for isolated service tests).
func RegisteredRetentionSchedule() *RetentionSchedule { return registeredRetentionSchedule.Load() }
