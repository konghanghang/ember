package models

import (
	"time"

	"gorm.io/gorm"
)

// EmbyAccessToken stores only a purpose-separated digest and its Ember user
// binding. The original Emby AccessToken is never persisted or serialized.
type EmbyAccessToken struct {
	ID            string     `json:"id" gorm:"column:id;type:varchar(25);primaryKey"`
	ServerID      string     `json:"serverId" gorm:"column:server_id;type:varchar(64);not null"`
	TokenHash     []byte     `json:"-" gorm:"column:token_hash;type:bytea;not null"`
	EmbyUserID    string     `json:"embyUserId" gorm:"column:emby_user_id;type:varchar(50);not null"`
	UserID        *string    `json:"userId,omitempty" gorm:"column:user_id;type:varchar(25)"`
	DeviceID      string     `json:"deviceId,omitempty" gorm:"column:device_id;type:varchar(256);not null;default:''"`
	ClientName    string     `json:"clientName,omitempty" gorm:"column:client_name;type:varchar(128);not null;default:''"`
	LastSeenAt    time.Time  `json:"lastSeenAt" gorm:"column:last_seen_at;not null"`
	RevokedAt     *time.Time `json:"revokedAt,omitempty" gorm:"column:revoked_at"`
	RevokedReason *string    `json:"revokedReason,omitempty" gorm:"column:revoked_reason;type:varchar(100)"`
	RevokedBy     *string    `json:"revokedBy,omitempty" gorm:"column:revoked_by;type:varchar(64)"`
	CreatedAt     time.Time  `json:"createdAt" gorm:"column:created_at;autoCreateTime"`
	UpdatedAt     time.Time  `json:"updatedAt" gorm:"column:updated_at;autoUpdateTime"`
}

// TableName returns the SQL table managed by the Emby token mapping migration.
func (EmbyAccessToken) TableName() string {
	return "emby_access_tokens"
}

// BeforeCreate generates the identifier and initializes the first seen time.
func (mapping *EmbyAccessToken) BeforeCreate(tx *gorm.DB) error {
	_ = tx
	if mapping.ID == "" {
		mapping.ID = generateCUID()
	}
	if mapping.LastSeenAt.IsZero() {
		mapping.LastSeenAt = time.Now().UTC()
	}
	return nil
}
