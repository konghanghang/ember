package models

import "time"

// WatchRetentionCheck stores the complete local decision evidence, never external response bodies.
type WatchRetentionCheck struct {
	UserID         string    `gorm:"column:user_id;primaryKey"`
	PlanGroup      string    `gorm:"column:plan_group;primaryKey"`
	CheckedAt      time.Time `gorm:"column:checked_at;primaryKey"`
	WindowStart    time.Time `gorm:"column:window_start"`
	StartedAt      time.Time `gorm:"column:started_at"`
	PeriodDays     int       `gorm:"column:period_days"`
	MinMinutes     int       `gorm:"column:min_minutes"`
	WatchedSeconds int64     `gorm:"column:watched_seconds"`
	Invalidated    bool      `gorm:"column:invalidated"`
}

// TableName fixes the migration contract for audit evidence.
func (WatchRetentionCheck) TableName() string { return "watch_retention_checks" }
