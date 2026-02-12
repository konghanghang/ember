package models

import (
	"time"
)

// Setting 系统配置
type Setting struct {
	Key       string    `json:"key" gorm:"column:key;type:varchar(50);primaryKey"`
	Value     string    `json:"value" gorm:"column:value;size:500;not null"`
	UpdatedAt time.Time `json:"updatedAt" gorm:"column:updatedAt;autoUpdateTime"`
}

func (Setting) TableName() string {
	return "settings"
}
