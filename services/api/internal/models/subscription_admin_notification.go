package models

import (
	"time"

	"gorm.io/gorm"
)

// SubscriptionAdminNotification 记录每条订阅管理员 Telegram 审批消息的投递引用。
type SubscriptionAdminNotification struct {
	ID              string    `json:"id" gorm:"column:id;type:varchar(25);primaryKey"`
	SubscriptionID  string    `json:"subscriptionId" gorm:"column:subscription_id;type:varchar(25);not null;index"`
	AdminTelegramID int64     `json:"adminTelegramId" gorm:"column:admin_telegram_id;not null;index"`
	ChatID          int64     `json:"chatId" gorm:"column:chat_id;not null"`
	MessageID       *int64    `json:"messageId,omitempty" gorm:"column:message_id"`
	HasPhoto        bool      `json:"hasPhoto" gorm:"column:has_photo;not null;default:false"`
	DeliveryStatus  string    `json:"deliveryStatus" gorm:"column:delivery_status;type:varchar(20);not null;default:'sent'"`
	FailureReason   *string   `json:"failureReason,omitempty" gorm:"column:failure_reason;size:500"`
	CreatedAt       time.Time `json:"createdAt" gorm:"column:created_at;autoCreateTime"`
	UpdatedAt       time.Time `json:"updatedAt" gorm:"column:updated_at;autoUpdateTime"`
}

func (SubscriptionAdminNotification) TableName() string {
	return "subscription_admin_notifications"
}

func (s *SubscriptionAdminNotification) BeforeCreate(tx *gorm.DB) error {
	_ = tx
	if s.ID == "" {
		s.ID = generateCUID()
	}
	return nil
}
