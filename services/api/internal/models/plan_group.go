package models

import "time"

// P115PlaybackMode selects the playback account source for a plan group.
type P115PlaybackMode string

const (
	P115PlaybackModePersonal P115PlaybackMode = "personal"
	P115PlaybackModeSystem   P115PlaybackMode = "system"
)

// PlanGroup 套餐分组
type PlanGroup struct {
	Key                               string           `json:"key" gorm:"column:key;type:varchar(50);primaryKey"`
	Name                              string           `json:"name" gorm:"column:name;size:100;not null"`
	Description                       string           `json:"description,omitempty" gorm:"column:description;size:500"`
	IsDefault                         bool             `json:"isDefault" gorm:"column:is_default;default:false;not null"`
	SortOrder                         int              `json:"sortOrder" gorm:"column:sort_order;default:0;not null"`
	SubscriptionAutoApproveDailyLimit int              `json:"subscriptionAutoApproveDailyLimit" gorm:"column:subscription_auto_approve_daily_limit;default:0;not null"`
	P115PlaybackMode                  P115PlaybackMode `json:"p115PlaybackMode" gorm:"column:p115_playback_mode;type:varchar(20);default:'personal';not null"`
	P115TransferHourlyLimit           int              `json:"p115TransferHourlyLimit" gorm:"column:p115_transfer_hourly_limit;default:5;not null"`
	P115TransferDailyLimit            int              `json:"p115TransferDailyLimit" gorm:"column:p115_transfer_daily_limit;default:10;not null"`
	MediaLibraryTemplateVersion       int64            `json:"-" gorm:"column:media_library_template_version;not null;default:1"`
	CreatedAt                         time.Time        `json:"createdAt" gorm:"column:created_at;autoCreateTime"`
	UpdatedAt                         time.Time        `json:"updatedAt" gorm:"column:updated_at;autoUpdateTime"`
}

func (PlanGroup) TableName() string {
	return "plan_groups"
}
