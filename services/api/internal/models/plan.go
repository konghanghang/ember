package models

import (
	"time"

	"gorm.io/gorm"
)

// Plan 付费方案
type Plan struct {
	ID            string    `json:"id" gorm:"column:id;type:varchar(25);primaryKey"`
	Name          string    `json:"name" gorm:"column:name;size:100;not null"`
	Description   string    `json:"description" gorm:"column:description;size:500"`
	Days          int       `json:"days" gorm:"column:days;not null"`
	Price         int64     `json:"price" gorm:"column:price;not null"`
	Currency      string    `json:"currency" gorm:"column:currency;size:3;not null;default:usd"`
	PlanGroup     string    `json:"planGroup" gorm:"column:planGroup;size:50;not null;index"`
	PlanGroupName string    `json:"planGroupName,omitempty" gorm:"->;column:planGroupName"`
	IsActive      bool      `json:"isActive" gorm:"column:isActive;default:true;not null"`
	SortOrder     int       `json:"sortOrder" gorm:"column:sortOrder;default:0;not null"`
	CreatedAt     time.Time `json:"createdAt" gorm:"column:createdAt;autoCreateTime"`
	UpdatedAt     time.Time `json:"updatedAt" gorm:"column:updatedAt;autoUpdateTime"`
}

func (Plan) TableName() string {
	return "plans"
}

func (p *Plan) BeforeCreate(tx *gorm.DB) error {
	if p.ID == "" {
		p.ID = generateCUID()
	}
	return nil
}
