package models

import (
	"time"

	"gorm.io/gorm"
)

// SubscriptionStatus 订阅状态枚举
type SubscriptionStatus string

const (
	SubscriptionPending  SubscriptionStatus = "PENDING"  // 待审核
	SubscriptionApproved SubscriptionStatus = "APPROVED" // 已批准
	SubscriptionRejected SubscriptionStatus = "REJECTED" // 已拒绝
)

// MediaType 媒体类型枚举
type MediaType string

const (
	MediaMovie MediaType = "MOVIE" // 电影
	MediaTV    MediaType = "TV"    // 电视剧
)

// Subscription 订阅模型
// 对应 Prisma schema 的 Subscription 模型
type Subscription struct {
	ID         string             `json:"id" gorm:"type:varchar(25);primaryKey"` // cuid
	UserID     string             `json:"userId" gorm:"type:varchar(25);not null;index"`
	Type       MediaType          `json:"type" gorm:"type:varchar(10);not null"`
	Name       string             `json:"name" gorm:"size:255;not null"`
	TmdbID     string             `json:"tmdbId" gorm:"size:50;not null"`
	PosterPath *string            `json:"posterPath,omitempty" gorm:"size:500"`
	Status     SubscriptionStatus `json:"status" gorm:"type:varchar(20);not null;default:'PENDING'"`
	Note       *string            `json:"note,omitempty" gorm:"type:text"`
	MpError    *string            `json:"mpError,omitempty" gorm:"size:500"` // MoviePilot 同步错误
	CreatedAt  time.Time          `json:"createdAt" gorm:"autoCreateTime"`
	UpdatedAt  time.Time          `json:"updatedAt" gorm:"autoUpdateTime"`

	// 关联：属于某个用户
	User *User `json:"-" gorm:"foreignKey:UserID"`
}

func (Subscription) TableName() string {
	return "subscriptions"
}

// BeforeCreate 钩子
func (s *Subscription) BeforeCreate(tx *gorm.DB) error {
	if s.ID == "" {
		s.ID = generateCUID()
	}
	return nil
}
