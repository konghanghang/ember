package models

import (
	"time"
)

// Setting 系统配置
type Setting struct {
	Key             string    `json:"key" gorm:"column:key;type:varchar(100);primaryKey"`
	Value           string    `json:"value" gorm:"column:value;type:text;not null"`
	IsEncrypted     bool      `json:"isEncrypted" gorm:"column:is_encrypted;default:false;not null"`
	UpdatedByUserID *string   `json:"updatedByUserId,omitempty" gorm:"column:updated_by_user_id;size:25"`
	UpdatedAt       time.Time `json:"updatedAt" gorm:"column:updated_at;autoUpdateTime"`
}

func (Setting) TableName() string {
	return "settings"
}
