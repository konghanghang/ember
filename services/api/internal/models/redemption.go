package models

import (
	"time"

	"gorm.io/gorm"
)

// Redemption 兑换历史记录
type Redemption struct {
	ID        string    `json:"id" gorm:"column:id;type:varchar(25);primaryKey"`
	UserID    string    `json:"userId" gorm:"column:userId;size:25;index;not null"`
	Code      string    `json:"code" gorm:"column:code;size:20;not null"`
	Days      int       `json:"days" gorm:"column:days;not null"`
	CreatedAt time.Time `json:"createdAt" gorm:"column:createdAt;autoCreateTime"`
}

func (Redemption) TableName() string {
	return "redemptions"
}

func (r *Redemption) BeforeCreate(tx *gorm.DB) error {
	if r.ID == "" {
		r.ID = generateCUID()
	}
	return nil
}
