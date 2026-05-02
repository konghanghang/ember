package models

import "time"

// BotRuntimeLock Bot 运行时租约锁，用于 polling 单实例保护。
type BotRuntimeLock struct {
	Name      string    `json:"name" gorm:"column:name;type:varchar(100);primaryKey"`
	OwnerID   string    `json:"ownerId" gorm:"column:owner_id;type:varchar(200);not null"`
	ExpiresAt time.Time `json:"expiresAt" gorm:"column:expires_at;not null;index"`
	CreatedAt time.Time `json:"createdAt" gorm:"column:created_at;autoCreateTime"`
	UpdatedAt time.Time `json:"updatedAt" gorm:"column:updated_at;autoUpdateTime"`
}

func (BotRuntimeLock) TableName() string {
	return "bot_runtime_locks"
}
