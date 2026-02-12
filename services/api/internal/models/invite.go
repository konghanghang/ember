package models

import (
	"time"

	"gorm.io/gorm"
)

// Invite 邀请码模型
// 对应 Prisma schema 的 Invite 模型
type Invite struct {
	ID          string     `json:"id" gorm:"column:id;type:varchar(25);primaryKey"` // cuid
	Code        string     `json:"code" gorm:"column:code;uniqueIndex;size:20;not null"`
	MaxUses     int        `json:"maxUses" gorm:"column:maxUses;not null;default:1"`
	UsedCount   int        `json:"usedCount" gorm:"column:usedCount;not null;default:0"`
	ExpiresAt   *time.Time `json:"expiresAt,omitempty" gorm:"column:expiresAt"` // 可选的过期时间
	DefaultDays int        `json:"defaultDays" gorm:"column:defaultDays;not null;default:30"`
	CreatedAt   time.Time  `json:"createdAt" gorm:"column:createdAt;autoCreateTime"`

	// 关联：一个邀请码可以创建多个用户
	Users []User `json:"-" gorm:"foreignKey:InviteCode;references:Code"`
}

func (Invite) TableName() string {
	return "invites"
}

// BeforeCreate 钩子
func (i *Invite) BeforeCreate(tx *gorm.DB) error {
	if i.ID == "" {
		i.ID = generateCUID()
	}
	return nil
}

// IsValid 检查邀请码是否有效
func (i *Invite) IsValid() bool {
	// 检查使用次数
	if i.UsedCount >= i.MaxUses {
		return false
	}
	// 检查过期时间
	if i.ExpiresAt != nil && i.ExpiresAt.Before(time.Now()) {
		return false
	}
	return true
}
