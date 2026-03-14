package models

import (
	"time"

	"gorm.io/gorm"
)

type PaymentStatus string

const (
	PaymentPending   PaymentStatus = "pending"
	PaymentCompleted PaymentStatus = "completed"
	PaymentFailed    PaymentStatus = "failed"
)

// Payment 支付记录
type Payment struct {
	ID                    string        `json:"id" gorm:"column:id;type:varchar(25);primaryKey"`
	UserID                string        `json:"userId" gorm:"column:userId;size:25;index;not null"`
	PlanID                string        `json:"planId" gorm:"column:planId;size:25;index;not null"`
	StripeSessionID       string        `json:"stripeSessionId" gorm:"column:stripeSessionId;size:255;uniqueIndex;not null"`
	StripePaymentIntentID string        `json:"stripePaymentIntentId,omitempty" gorm:"column:stripePaymentIntentId;size:255"`
	CheckoutURL           string        `json:"checkoutUrl,omitempty" gorm:"column:checkoutUrl;size:2048;not null;default:''"`
	Amount                int64         `json:"amount" gorm:"column:amount;not null"`
	Currency              string        `json:"currency" gorm:"column:currency;size:3;not null;default:usd"`
	Days                  int           `json:"days" gorm:"column:days;not null"`
	Status                PaymentStatus `json:"status" gorm:"column:status;size:20;not null;default:pending"`
	CreatedAt             time.Time     `json:"createdAt" gorm:"column:createdAt;autoCreateTime"`
	UpdatedAt             time.Time     `json:"updatedAt" gorm:"column:updatedAt;autoUpdateTime"`
}

func (Payment) TableName() string {
	return "payments"
}

func (p *Payment) BeforeCreate(tx *gorm.DB) error {
	if p.ID == "" {
		p.ID = generateCUID()
	}
	return nil
}
