package models

import "time"

// StripeWebhookEventStatus webhook 事件去重状态。
type StripeWebhookEventStatus string

const (
	StripeWebhookEventReceived  StripeWebhookEventStatus = "received"
	StripeWebhookEventProcessed StripeWebhookEventStatus = "processed"
	StripeWebhookEventSkipped   StripeWebhookEventStatus = "skipped"
	StripeWebhookEventFailed    StripeWebhookEventStatus = "failed"
)

// StripeWebhookEvent 用 event.id 主键的去重表，保证 Stripe Dashboard "Resend"
// 或网络抖动重投的同一事件只履约一次。
type StripeWebhookEvent struct {
	EventID     string                   `json:"eventId" gorm:"column:eventId;type:varchar(64);primaryKey"`
	EventType   string                   `json:"eventType" gorm:"column:eventType;size:64;not null"`
	Livemode    bool                     `json:"livemode" gorm:"column:livemode;not null;default:false"`
	ReceivedAt  time.Time                `json:"receivedAt" gorm:"column:receivedAt;not null;autoCreateTime"`
	ProcessedAt *time.Time               `json:"processedAt,omitempty" gorm:"column:processedAt"`
	Status      StripeWebhookEventStatus `json:"status" gorm:"column:status;size:20;not null;default:received"`
	Error       *string                  `json:"errorMessage,omitempty" gorm:"column:errorMessage;size:500"`
}

func (StripeWebhookEvent) TableName() string {
	return "stripe_webhook_events"
}
