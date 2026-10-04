package models

import (
	"time"

	"gorm.io/gorm"
)

type PaymentStatus string

const (
	PaymentManualReview PaymentStatus = "paid_review"
	PaymentResolved     PaymentStatus = "resolved"
	PaymentPending      PaymentStatus = "pending"
	PaymentCompleted    PaymentStatus = "completed"
	PaymentExpired      PaymentStatus = "expired"
	PaymentFailed       PaymentStatus = "failed"
)

// Payment 支付记录
type Payment struct {
	Benefits              []PlanBenefit `json:"benefits" gorm:"column:benefits;type:jsonb;serializer:json"`
	PaidAt                *time.Time    `json:"paidAt" gorm:"column:paid_at"`
	ManualReviewReason    string        `json:"manualReviewReason,omitempty" gorm:"column:manual_review_reason;type:varchar(100);not null;default:''"`
	Resolution            string        `json:"resolution,omitempty" gorm:"column:resolution;type:varchar(30);not null;default:''"`
	ResolutionNote        string        `json:"resolutionNote,omitempty" gorm:"column:resolution_note;type:varchar(500);not null;default:''"`
	ResolvedBy            string        `json:"resolvedBy,omitempty" gorm:"column:resolved_by;type:varchar(100);not null;default:''"`
	ResolvedAt            *time.Time    `json:"resolvedAt" gorm:"column:resolved_at"`
	ID                    string        `json:"id" gorm:"column:id;type:varchar(25);primaryKey"`
	UserID                string        `json:"userId" gorm:"column:user_id;size:25;index;not null"`
	PlanID                string        `json:"planId" gorm:"column:plan_id;size:25;index;not null"`
	StripeSessionID       string        `json:"stripeSessionId" gorm:"column:stripe_session_id;size:255;uniqueIndex;not null"`
	StripePaymentIntentID string        `json:"stripePaymentIntentId,omitempty" gorm:"column:stripe_payment_intent_id;size:255"`
	CheckoutURL           string        `json:"checkoutUrl,omitempty" gorm:"column:checkout_url;size:2048;not null;default:''"`
	Amount                int64         `json:"amount" gorm:"column:amount;not null"`
	Currency              string        `json:"currency" gorm:"column:currency;size:3;not null;default:usd"`
	Days                  int           `json:"days" gorm:"column:days;not null"`
	Status                PaymentStatus `json:"status" gorm:"column:status;size:20;not null;default:pending"`
	ExpiresAt             *time.Time    `json:"expiresAt,omitempty" gorm:"column:expires_at;index"`
	CreatedAt             time.Time     `json:"createdAt" gorm:"column:created_at;autoCreateTime"`
	UpdatedAt             time.Time     `json:"updatedAt" gorm:"column:updated_at;autoUpdateTime"`
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
