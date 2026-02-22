package models

import (
	"time"

	"gorm.io/gorm"
)

// EmailVerification 邮箱验证码
type EmailVerification struct {
	ID        string    `json:"id" gorm:"column:id;type:varchar(25);primaryKey"`
	Email     string    `json:"email" gorm:"column:email;size:255;not null;index"`
	Code      string    `json:"-" gorm:"column:code;size:6;not null"`
	IP        string    `json:"-" gorm:"column:ip;size:45;not null;index"`
	ExpiresAt time.Time `json:"expiresAt" gorm:"column:expiresAt;not null"`
	CreatedAt time.Time `json:"createdAt" gorm:"column:createdAt;autoCreateTime"`
}

func (EmailVerification) TableName() string {
	return "email_verifications"
}

func (e *EmailVerification) BeforeCreate(tx *gorm.DB) error {
	if e.ID == "" {
		e.ID = generateCUID()
	}
	return nil
}

// IsExpired 检查验证码是否过期
func (e *EmailVerification) IsExpired() bool {
	return e.ExpiresAt.Before(time.Now().UTC())
}
