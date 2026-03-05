package models

import (
	"strings"
	"time"

	"gorm.io/gorm"
)

// ClientBlacklist 客户端黑名单
type ClientBlacklist struct {
	ID                   string    `json:"id" gorm:"column:id;type:varchar(25);primaryKey"`
	ClientName           string    `json:"clientName" gorm:"column:clientName;size:100;not null"`
	NormalizedClientName string    `json:"-" gorm:"column:normalizedClientName;size:100;uniqueIndex;not null"`
	Reason               string    `json:"reason,omitempty" gorm:"column:reason;size:255"`
	CreatedAt            time.Time `json:"createdAt" gorm:"column:createdAt;autoCreateTime"`
}

func (ClientBlacklist) TableName() string {
	return "client_blacklists"
}

func (c *ClientBlacklist) BeforeCreate(tx *gorm.DB) error {
	if c.ID == "" {
		c.ID = generateCUID()
	}
	if c.NormalizedClientName == "" {
		c.NormalizedClientName = strings.ToLower(strings.TrimSpace(c.ClientName))
	}
	return nil
}
