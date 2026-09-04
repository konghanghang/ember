package models

import (
	"time"

	"gorm.io/gorm"
)

type P115AccountRole string

const (
	P115AccountRoleSource   P115AccountRole = "source"
	P115AccountRolePlayback P115AccountRole = "playback"
)

type P115AuthMode string

const (
	P115AuthModeLegacyCookie P115AuthMode = "legacy_cookie"
)

type P115AccountStatus string

const (
	P115AccountStatusPending     P115AccountStatus = "pending"
	P115AccountStatusActive      P115AccountStatus = "active"
	P115AccountStatusExpired     P115AccountStatus = "expired"
	P115AccountStatusError       P115AccountStatus = "error"
	P115AccountStatusCoolingDown P115AccountStatus = "cooling_down"
	P115AccountStatusRevoked     P115AccountStatus = "revoked"
)

// P115Account stores an encrypted administrator or user-owned 115 account credential.
type P115Account struct {
	ID                   string            `json:"id" gorm:"column:id;type:varchar(25);primaryKey"`
	Role                 P115AccountRole   `json:"role" gorm:"column:role;type:varchar(20);not null"`
	Alias                string            `json:"alias" gorm:"column:alias;type:varchar(100);not null"`
	AuthMode             P115AuthMode      `json:"authMode" gorm:"column:auth_mode;type:varchar(20);not null;default:'legacy_cookie'"`
	OwnerUserID          *string           `json:"ownerUserId,omitempty" gorm:"column:owner_user_id;type:varchar(25)"`
	ProviderUserID       *string           `json:"providerUserId,omitempty" gorm:"column:provider_user_id;type:varchar(64)"`
	CookieCiphertext     *string           `json:"-" gorm:"column:cookie_ciphertext;type:text"`
	AppType              *string           `json:"appType,omitempty" gorm:"column:app_type;type:varchar(32)"`
	UserAgent            *string           `json:"userAgent,omitempty" gorm:"column:user_agent;type:varchar(512)"`
	EmbyPathPrefix       *string           `json:"embyPathPrefix,omitempty" gorm:"column:emby_path_prefix;type:varchar(4096)"`
	SourceRootID         *string           `json:"sourceRootId,omitempty" gorm:"column:source_root_id;type:varchar(64)"`
	TargetParentID       *string           `json:"targetParentId,omitempty" gorm:"column:target_parent_id;type:varchar(64)"`
	TargetParentPath     *string           `json:"targetParentPath,omitempty" gorm:"column:target_parent_path;type:varchar(4096)"`
	MaxConcurrentStreams *int              `json:"maxConcurrentStreams,omitempty" gorm:"column:max_concurrent_streams"`
	Status               P115AccountStatus `json:"status" gorm:"column:status;type:varchar(20);not null;default:'pending'"`
	Enabled              bool              `json:"enabled" gorm:"column:enabled;not null;default:false"`
	LastValidatedAt      *time.Time        `json:"lastValidatedAt,omitempty" gorm:"column:last_validated_at"`
	LastSucceededAt      *time.Time        `json:"lastSucceededAt,omitempty" gorm:"column:last_succeeded_at"`
	CooldownUntil        *time.Time        `json:"cooldownUntil,omitempty" gorm:"column:cooldown_until"`
	LastErrorCode        *string           `json:"lastErrorCode,omitempty" gorm:"column:last_error_code;type:varchar(100)"`
	LastErrorMessage     *string           `json:"lastErrorMessage,omitempty" gorm:"column:last_error_message;type:varchar(500)"`
	CreatedAt            time.Time         `json:"createdAt" gorm:"column:created_at;autoCreateTime"`
	UpdatedAt            time.Time         `json:"updatedAt" gorm:"column:updated_at;autoUpdateTime"`
}

// TableName returns the SQL table managed by the p115 account migration.
func (P115Account) TableName() string {
	return "p115_accounts"
}

// BeforeCreate generates the identifier and applies safe credential defaults.
func (a *P115Account) BeforeCreate(tx *gorm.DB) error {
	_ = tx
	if a.ID == "" {
		a.ID = generateCUID()
	}
	if a.AuthMode == "" {
		a.AuthMode = P115AuthModeLegacyCookie
	}
	if a.Status == "" {
		a.Status = P115AccountStatusPending
	}
	return nil
}
