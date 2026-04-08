package models

import "time"

// PlanGroup 套餐分组
type PlanGroup struct {
	Key                string    `json:"key" gorm:"column:key;type:varchar(50);primaryKey"`
	Name               string    `json:"name" gorm:"column:name;size:100;not null"`
	Description        string    `json:"description,omitempty" gorm:"column:description;size:500"`
	IsDefault          bool      `json:"isDefault" gorm:"column:isDefault;default:false;not null"`
	SortOrder          int       `json:"sortOrder" gorm:"column:sortOrder;default:0;not null"`
	CreatedAt          time.Time `json:"createdAt" gorm:"column:createdAt;autoCreateTime"`
	UpdatedAt          time.Time `json:"updatedAt" gorm:"column:updatedAt;autoUpdateTime"`
	PlanCount          int64     `json:"planCount,omitempty" gorm:"-"`
	UserCount          int64     `json:"userCount,omitempty" gorm:"-"`
	FollowingUserCount int64     `json:"followingUserCount,omitempty" gorm:"-"`
}

func (PlanGroup) TableName() string {
	return "plan_groups"
}
