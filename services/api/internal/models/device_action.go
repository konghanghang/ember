package models

import (
	"time"

	"gorm.io/gorm"
)

// DeviceAction 设备操作日志
type DeviceAction struct {
	ID         string    `json:"id" gorm:"column:id;type:varchar(25);primaryKey"`
	DeviceID   string    `json:"deviceId,omitempty" gorm:"column:deviceId;size:100;index"`
	UserID     string    `json:"userId,omitempty" gorm:"column:userId;size:25;index"`
	ClientName string    `json:"clientName,omitempty" gorm:"column:clientName;size:100"`
	Action     string    `json:"action" gorm:"column:action;size:50;not null"`
	Note       string    `json:"note,omitempty" gorm:"column:note;size:255"`
	OperatorID *string   `json:"operatorId,omitempty" gorm:"column:operatorId"`
	CreatedAt  time.Time `json:"createdAt" gorm:"column:createdAt;autoCreateTime"`
}

func (DeviceAction) TableName() string {
	return "device_actions"
}

func (d *DeviceAction) BeforeCreate(tx *gorm.DB) error {
	if d.ID == "" {
		d.ID = generateCUID()
	}
	return nil
}
