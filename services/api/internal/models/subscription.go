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
	SubscriptionIngested SubscriptionStatus = "INGESTED" // 已入库
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
	ID           string             `json:"id" gorm:"column:id;type:varchar(25);primaryKey"` // cuid
	UserID       string             `json:"userId" gorm:"column:userId;type:varchar(25);not null;index"`
	Type         MediaType          `json:"type" gorm:"column:type;type:varchar(10);not null"`
	Name         string             `json:"name" gorm:"column:name;size:255;not null"`
	TmdbID       string             `json:"tmdbId" gorm:"column:tmdbId;size:50;not null"`
	Season       int                `json:"season" gorm:"column:season;not null;default:0"`
	PosterPath   *string            `json:"posterPath,omitempty" gorm:"column:posterPath;size:500"`
	Status       SubscriptionStatus `json:"status" gorm:"column:status;type:varchar(20);not null;default:'PENDING'"`
	Note         *string            `json:"note,omitempty" gorm:"column:note;type:text"`
	MpError      *string            `json:"mpError,omitempty" gorm:"column:mpError;size:500"` // MoviePilot 同步错误
	RejectReason *string            `json:"rejectReason,omitempty" gorm:"column:rejectReason;type:text"`
	RetryFromID  *string            `json:"retryFromId,omitempty" gorm:"column:retryFromId;type:varchar(25);index"`
	ReviewedAt   *time.Time         `json:"reviewedAt,omitempty" gorm:"column:reviewedAt"`
	IngestedAt   *time.Time         `json:"ingestedAt,omitempty" gorm:"column:ingestedAt"`
	CreatedAt    time.Time          `json:"createdAt" gorm:"column:createdAt;autoCreateTime"`
	UpdatedAt    time.Time          `json:"updatedAt" gorm:"column:updatedAt;autoUpdateTime"`
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
