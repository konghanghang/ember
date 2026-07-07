package models

import "time"

// PlanGroup 套餐分组
type PlanGroup struct {
	Key                               string    `json:"key" gorm:"column:key;type:varchar(50);primaryKey"`
	Name                              string    `json:"name" gorm:"column:name;size:100;not null"`
	Description                       string    `json:"description,omitempty" gorm:"column:description;size:500"`
	IsDefault                         bool      `json:"isDefault" gorm:"column:is_default;default:false;not null"`
	SortOrder                         int       `json:"sortOrder" gorm:"column:sort_order;default:0;not null"`
	SubscriptionAutoApproveDailyLimit int       `json:"subscriptionAutoApproveDailyLimit" gorm:"column:subscription_auto_approve_daily_limit;default:0;not null"`
	MediaLibraryTemplateVersion       int64     `json:"-" gorm:"column:media_library_template_version;not null;default:1"`
	CreatedAt                         time.Time `json:"createdAt" gorm:"column:created_at;autoCreateTime"`
	UpdatedAt                         time.Time `json:"updatedAt" gorm:"column:updated_at;autoUpdateTime"`
}

func (PlanGroup) TableName() string {
	return "plan_groups"
}
