package models

import (
	"time"

	"gorm.io/gorm"
)

// User 用户模型
// 对应 Prisma schema 的 User 模型
//
// 设计决策：保留 InviteCode string（业务外键）
// 理由：注册是"历史事件"，保存的是"我用哪个码注册的"（历史快照）
// 而不是"我属于哪个邀请码"（当前关系）
// 详见：prisma/schema.prisma 第24-35行注释
type User struct {
	ID         string     `json:"id" gorm:"column:id;type:varchar(25);primaryKey"` // cuid
	Username   string     `json:"username" gorm:"column:username;uniqueIndex;size:50;not null"`
	Email      string     `json:"email" gorm:"column:email;size:255;not null"`
	EmbyID     string     `json:"embyId" gorm:"column:embyId;uniqueIndex;size:50;not null"` // Emby用户ID
	InviteCode string     `json:"inviteCode" gorm:"column:inviteCode;size:20;not null;index"`   // ✅ 保留字符串外键
	ExpiresAt  *time.Time `json:"expiresAt,omitempty" gorm:"column:expiresAt"`                        // 账号到期时间（null=永久有效）
	IsActive   bool       `json:"isActive" gorm:"column:isActive;default:true;not null"`
	CreatedAt  time.Time  `json:"createdAt" gorm:"column:createdAt;autoCreateTime"`
	UpdatedAt  time.Time  `json:"updatedAt" gorm:"column:updatedAt;autoUpdateTime"`

	// 关联：注册时使用的邀请码（可选关系，邀请码可能被删除）
	Invite *Invite `json:"-" gorm:"foreignKey:InviteCode;references:Code"`

	// 关联：用户的订阅记录
	Subscriptions []Subscription `json:"-" gorm:"foreignKey:UserID"`
}

func (User) TableName() string {
	return "users"
}

// BeforeCreate 钩子
func (u *User) BeforeCreate(tx *gorm.DB) error {
	if u.ID == "" {
		u.ID = generateCUID()
	}
	return nil
}

// IsExpired 检查账号是否过期
func (u *User) IsExpired() bool {
	if u.ExpiresAt == nil {
		return false // 永久有效
	}
	return u.ExpiresAt.Before(time.Now())
}
