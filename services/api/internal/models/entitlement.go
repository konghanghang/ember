package models

import "time"

// PlanBenefit is a grant instruction for immutable order snapshots and administrator compensation.
type PlanBenefit struct {
	PlanGroupName string `json:"planGroupName,omitempty" gorm:"-"`
	PlanGroup     string `json:"planGroup" gorm:"column:plan_group"`
	ValidityType  string `json:"validityType" gorm:"column:validity_type"`
	DurationDays  int    `json:"durationDays,omitempty" gorm:"column:duration_days"`
}

// UserEntitlement stores one independently expiring authorization per user and group.
type UserEntitlement struct {
	UserID       string     `json:"userId" gorm:"column:user_id;type:varchar(25);primaryKey"`
	PlanGroup    string     `json:"planGroup" gorm:"column:plan_group;type:varchar(50);primaryKey"`
	ValidityType string     `json:"validityType" gorm:"column:validity_type;type:varchar(20);not null"`
	ExpiresAt    *time.Time `json:"expiresAt" gorm:"column:expires_at"`
	CreatedAt    time.Time  `json:"createdAt" gorm:"column:created_at;autoCreateTime"`
	UpdatedAt    time.Time  `json:"updatedAt" gorm:"column:updated_at;autoUpdateTime"`
}

// TableName fixes the entitlement SQL contract.
func (UserEntitlement) TableName() string { return "user_entitlements" }

// EntitlementEvent records the idempotency key and before/after state of a grant or adjustment.
type EntitlementEvent struct {
	SourceKey   string    `json:"sourceKey" gorm:"column:source_key;type:varchar(200);primaryKey"`
	UserID      string    `json:"userId" gorm:"column:user_id;type:varchar(25);not null;index"`
	Actor       string    `json:"actor" gorm:"column:actor;type:varchar(100);not null"`
	Reason      string    `json:"reason" gorm:"column:reason;type:varchar(100);not null"`
	BeforeState string    `json:"beforeState" gorm:"column:before_state;type:jsonb;not null"`
	AfterState  string    `json:"afterState" gorm:"column:after_state;type:jsonb;not null"`
	CreatedAt   time.Time `json:"createdAt" gorm:"column:created_at;autoCreateTime"`
}

// TableName fixes the audit SQL contract.
func (EntitlementEvent) TableName() string { return "entitlement_events" }
