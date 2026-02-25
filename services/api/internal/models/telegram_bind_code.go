package models

import (
	"time"

	"gorm.io/gorm"
)

// TelegramBindCode Telegram 绑定验证码
type TelegramBindCode struct {
	ID        string    `json:"id" gorm:"column:id;type:varchar(25);primaryKey"`
	UserID    string    `json:"userId" gorm:"column:userId;size:25;not null;index"`
	Code      string    `json:"-" gorm:"column:code;size:6;not null;uniqueIndex"`
	ExpiresAt time.Time `json:"expiresAt" gorm:"column:expiresAt;not null"`
	CreatedAt time.Time `json:"createdAt" gorm:"column:createdAt;autoCreateTime"`
}

func (TelegramBindCode) TableName() string {
	return "telegram_bind_codes"
}

func (t *TelegramBindCode) BeforeCreate(tx *gorm.DB) error {
	if t.ID == "" {
		t.ID = generateCUID()
	}
	return nil
}

// IsExpired 检查验证码是否过期
func (t *TelegramBindCode) IsExpired() bool {
	return t.ExpiresAt.Before(time.Now().UTC())
}
