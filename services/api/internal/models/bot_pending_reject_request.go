package models

import (
	"time"

	"gorm.io/gorm"
)

// BotPendingRejectRequest 持久化的 Bot 拒绝订阅待确认记录
type BotPendingRejectRequest struct {
	ID             string    `json:"id" gorm:"column:id;type:varchar(25);primaryKey"`
	ChatID         int64     `json:"chatId" gorm:"column:chat_id;not null"`
	AdminUserID    string    `json:"adminUserId" gorm:"column:admin_user_id;type:varchar(25);not null"`
	SubscriptionID string    `json:"subscriptionId" gorm:"column:subscription_id;type:varchar(25);not null"`
	MessageID      *int64    `json:"messageId,omitempty" gorm:"column:message_id"`
	HasPhoto       bool      `json:"hasPhoto" gorm:"column:has_photo;not null;default:false"`
	OriginalText   string    `json:"originalText,omitempty" gorm:"column:original_text;type:text;not null;default:''"`
	CreatedAt      time.Time `json:"createdAt" gorm:"column:created_at;autoCreateTime"`
	ExpiresAt      time.Time `json:"expiresAt" gorm:"column:expires_at;not null"`
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
