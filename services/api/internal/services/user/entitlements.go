package user

import (
	"context"
	"strings"
	"time"

	configpkg "github.com/konghang/ember/backend/internal/config"
	"github.com/konghang/ember/backend/internal/db"
	"github.com/konghang/ember/backend/internal/models"
	entitlementpkg "github.com/konghang/ember/backend/internal/services/entitlement"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// EntitlementView exposes owned groups without recomputing the currently applied group on reads.
type WatchRetentionView struct {
	Days         int        `json:"days"`
	MinMinutes   int        `json:"minMinutes"`
	State        string     `json:"state"`
	FirstCheckAt *time.Time `json:"firstCheckAt,omitempty"`
}

// EntitlementView includes the effective policy facts needed for user-facing retention copy.
type EntitlementView struct {
	WatchRetention   *WatchRetentionView `json:"watchRetention,omitempty" gorm:"-"`
	RetentionEnabled bool                `json:"-" gorm:"column:retention_enabled"`
	RetentionDays    int                 `json:"-" gorm:"column:retention_days"`
	RetentionMinutes int                 `json:"-" gorm:"column:retention_minutes"`
	RetentionResetAt *time.Time          `json:"-" gorm:"column:retention_reset_at"`
	Current          bool                `json:"isCurrent" gorm:"column:is_current"`
	models.UserEntitlement
	PlanGroupName string `json:"planGroupName" gorm:"column:plan_group_name"`
}

// AdjustEntitlementRequest describes one explicit, replay-safe administrator action.
type AdjustEntitlementRequest struct {
	OperationID  string  `json:"operationId" binding:"required,max=64"`
	PlanGroup    string  `json:"planGroup" binding:"required"`
	Action       string  `json:"action" binding:"required,oneof=set extend revoke"`
	ValidityType string  `json:"validityType"`
	ExpiresAt    *string `json:"expiresAt"`
	Days         int     `json:"days"`
}

// GetEntitlements returns all grants, including expired ones, for account and administrator displays.
func (s *UserService) GetEntitlements(ctx context.Context, userID string) ([]EntitlementView, error) {
	rows := []EntitlementView{}
	err := db.DB.WithContext(ctx).Table("user_entitlements AS e").Select("e.*, g.name AS plan_group_name, g.watch_retention_enabled AS retention_enabled, g.watch_retention_days AS retention_days, g.watch_retention_min_minutes AS retention_minutes, g.watch_retention_reset_at AS retention_reset_at, (u.resource_access_granted AND u.plan_group = e.plan_group) AS is_current").Joins("JOIN plan_groups g ON g.key = e.plan_group").Joins("JOIN users u ON u.id = e.user_id").Where("e.user_id = ?", userID).Order("g.entitlement_rank DESC NULLS LAST, e.plan_group").Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	cfg := configpkg.NewConfigService()
	schedule := cfg.GetString("WATCH_RETENTION_SCHEDULE")
	if schedule == "" {
		schedule = "0 2 * * *"
	}
	loc := configpkg.LoadConfiguredTimezone()
	enabled := cfg.GetString("CRON_ENABLED") != "false"
	if runtime := entitlementpkg.RegisteredRetentionSchedule(); runtime != nil {
		schedule = runtime.Expression
		loc = runtime.Location
		enabled = runtime.Enabled
	}
	for i := range rows {
		if err := attachWatchRetentionView(&rows[i], time.Now(), schedule, loc); err != nil {
			return nil, err
		}
		if !enabled && rows[i].WatchRetention != nil && rows[i].Current && rows[i].WatchRetentionInvalidatedAt == nil {
			rows[i].WatchRetention.State = "suspended"
			rows[i].WatchRetention.FirstCheckAt = nil
		}
	}
	return rows, nil
}

// attachWatchRetentionView derives display stages from server facts without mutating entitlement state.
func attachWatchRetentionView(row *EntitlementView, now time.Time, schedule string, loc *time.Location) error {
	if !row.RetentionEnabled {
		return nil
	}
	view := &WatchRetentionView{Days: row.RetentionDays, MinMinutes: row.RetentionMinutes, State: "waiting"}
	row.WatchRetention = view
	if row.WatchRetentionInvalidatedAt != nil {
		view.State = "invalidated"
		return nil
	}
	if row.ExpiresAt != nil && !row.ExpiresAt.After(now) && !row.Current {
		view.State = "expired"
		return nil
	}
	if !row.Current {
		return nil
	}
	start := entitlementpkg.RetentionStart(row.UserEntitlement, models.PlanGroup{WatchRetentionResetAt: row.RetentionResetAt})
	if start == nil {
		return nil
	}
	first, err := entitlementpkg.FirstWatchRetentionCheck(*start, row.RetentionDays, schedule, loc)
	if err != nil {
		return err
	}
	view.FirstCheckAt = &first
	view.State = "checking"
	if now.Before(first) {
		view.State = "grace"
	}
	return nil
}

// AdjustEntitlement keeps other groups intact and synchronizes the selected full template after commit.
func (s *UserService) AdjustEntitlement(ctx context.Context, userID, actor string, req *AdjustEntitlementRequest) error {
	if req == nil || strings.TrimSpace(req.OperationID) == "" || len(req.OperationID) > 64 {
		return ErrRequestInvalid
	}
	if actor == "" {
		actor = "admin:api-key"
	}
	location := configpkg.LoadConfiguredTimezone()
	var expiresAt *time.Time
	if req.Action == "set" && req.ValidityType == entitlementpkg.Duration {
		if req.ExpiresAt == nil {
			return ErrRequestInvalid
		}
		parsed, err := parseAdminExpiryInput(*req.ExpiresAt, location)
		if err != nil {
			return ErrExpiresAtFormatInvalid
		}
		expiresAt = &parsed
	}
	var user models.User
	err := db.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", userID).First(&user).Error; err != nil {
			return err
		}
		if user.IsAdmin() {
			return ErrRequestInvalid
		}
		key := "admin:" + userID + ":" + req.OperationID
		switch req.Action {
		case "extend":
			return entitlementpkg.GrantLocked(tx, &user, []entitlementpkg.Benefit{{PlanGroup: req.PlanGroup, ValidityType: entitlementpkg.Duration, DurationDays: req.Days}}, key, actor, time.Now(), location)
		case "set", "revoke":
			return entitlementpkg.AdjustLocked(tx, &user, entitlementpkg.Holding{PlanGroup: req.PlanGroup, ValidityType: req.ValidityType, ExpiresAt: expiresAt}, req.Action == "revoke", key, actor, time.Now())
		default:
			return ErrRequestInvalid
		}
	})
	if err != nil {
		return err
	}
	return s.syncEmbyPolicy(&user, "admin_entitlement_update")
}

// parseAdminExpiryInput accepts explicit-offset API timestamps or wall time in the global business timezone.
func parseAdminExpiryInput(value string, location *time.Location) (time.Time, error) {
	if parsed, err := time.Parse(time.RFC3339, value); err == nil {
		return parsed, nil
	}
	return time.ParseInLocation("2006-01-02 15:04:05", value, location)
}
