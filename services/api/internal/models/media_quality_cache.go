package models

import (
	"time"

	"gorm.io/gorm"
)

// MediaQualityCache 媒体库质量报告缓存
type MediaQualityCache struct {
	ID            string     `json:"id" gorm:"column:id;type:varchar(25);primaryKey"`
	LibraryID     string     `json:"libraryId" gorm:"column:library_id;size:100;uniqueIndex;not null"`
	Statistics    string     `json:"statistics" gorm:"column:statistics;type:text;not null"`
	SchemaVersion int        `json:"schemaVersion" gorm:"column:schema_version;not null;default:1"`
	InflightUntil *time.Time `json:"inflightUntil,omitempty" gorm:"column:inflight_until"`
	ExpiresAt     time.Time  `json:"expiresAt" gorm:"column:expires_at;index;not null"`
	CreatedAt     time.Time  `json:"createdAt" gorm:"column:created_at;autoCreateTime"`
	UpdatedAt     time.Time  `json:"updatedAt" gorm:"column:updated_at;autoUpdateTime"`
}

func (MediaQualityCache) TableName() string {
	return "media_quality_caches"
}

func (m *MediaQualityCache) BeforeCreate(tx *gorm.DB) error {
	_ = tx
	if m.ID == "" {
		m.ID = generateCUID()
	}
	return nil
}
