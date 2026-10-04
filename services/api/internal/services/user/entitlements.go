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
type EntitlementView struct {
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
	err := db.DB.WithContext(ctx).Table("user_entitlements AS e").Select("e.*, g.name AS plan_group_name").Joins("JOIN plan_groups g ON g.key = e.plan_group").Where("e.user_id = ?", userID).Order("g.entitlement_rank DESC NULLS LAST, e.plan_group").Scan(&rows).Error
	return rows, err
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
		parsed, err := time.ParseInLocation("2006-01-02 15:04:05", *req.ExpiresAt, location)
		if err != nil {
			parsed, err = time.Parse(time.RFC3339, *req.ExpiresAt)
		}
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
