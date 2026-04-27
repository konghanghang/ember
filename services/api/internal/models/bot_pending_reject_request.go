package models

import (
	"time"

	"gorm.io/gorm"
)

// BotPendingRejectRequest 持久化的 Bot 拒绝订阅待确认记录
type BotPendingRejectRequest struct {
	ID             string    `json:"id" gorm:"column:id;type:varchar(25);primaryKey"`
	ChatID         int64     `json:"chatId" gorm:"column:chatId;not null"`
	AdminUserID    string    `json:"adminUserId" gorm:"column:adminUserId;type:varchar(25);not null"`
	SubscriptionID string    `json:"subscriptionId" gorm:"column:subscriptionId;type:varchar(25);not null"`
	MessageID      *int64    `json:"messageId,omitempty" gorm:"column:messageId"`
	HasPhoto       bool      `json:"hasPhoto" gorm:"column:hasPhoto;not null;default:false"`
	OriginalText   string    `json:"originalText,omitempty" gorm:"column:originalText;type:text;not null;default:''"`
	CreatedAt      time.Time `json:"createdAt" gorm:"column:createdAt;autoCreateTime"`
	ExpiresAt      time.Time `json:"expiresAt" gorm:"column:expiresAt;not null"`
}

func (BotPendingRejectRequest) TableName() string {
	return "bot_pending_reject_requests"
}

func (b *BotPendingRejectRequest) BeforeCreate(tx *gorm.DB) error {
	_ = tx
	if b.ID == "" {
		b.ID = generateCUID()
	}
	return nil
}
