package models

import (
	"time"

	"gorm.io/gorm"
)

// RedemptionCode 兑换码模型（统一用于注册门控和续期）
type RedemptionCode struct {
	ID                        string     `json:"id" gorm:"column:id;type:varchar(25);primaryKey"`
	Code                      string     `json:"code" gorm:"column:code;uniqueIndex;size:20;not null"`
	MaxUses                   int        `json:"maxUses" gorm:"column:max_uses;not null;default:1"`
	UsedCount                 int        `json:"usedCount" gorm:"column:used_count;not null;default:0"`
	ExpiresAt                 *time.Time `json:"expiresAt,omitempty" gorm:"column:expires_at"`
	DefaultDays               int        `json:"defaultDays" gorm:"column:default_days;not null;default:30"`
	TemplateUserID            *string    `json:"templateUserId,omitempty" gorm:"column:template_user_id;type:varchar(25);index"`
	TemplateUserName          *string    `json:"templateUserName,omitempty" gorm:"-"`
	RegistrationPlanGroup     *string    `json:"registrationPlanGroup,omitempty" gorm:"column:registration_plan_group;size:50;index"`
	RegistrationPlanGroupName *string    `json:"registrationPlanGroupName,omitempty" gorm:"-"`
	Notes                     string     `json:"notes,omitempty" gorm:"column:notes;size:500"`
	CreatedAt                 time.Time  `json:"createdAt" gorm:"column:created_at;autoCreateTime"`
}

func (RedemptionCode) TableName() string {
	return "redemption_codes"
}

func (r *RedemptionCode) BeforeCreate(tx *gorm.DB) error {
	if r.ID == "" {
		r.ID = generateCUID()
	}
	return nil
}

// IsValid 检查兑换码是否有效
func (r *RedemptionCode) IsValid() bool {
	if r.UsedCount >= r.MaxUses {
		return false
	}
	if r.ExpiresAt != nil && r.ExpiresAt.Before(time.Now().UTC()) {
		return false
	}
	return true
}
