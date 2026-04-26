package models

import (
	"time"

	"gorm.io/gorm"
)

// FailedEmbyAsyncOpOrigin 记录补偿来源，便于按业务定位重试失败链路。
type FailedEmbyAsyncOpOrigin string

const (
	FailedEmbyOriginPaymentUnban    FailedEmbyAsyncOpOrigin = "payment_unban"
	FailedEmbyOriginRedemptionUnban FailedEmbyAsyncOpOrigin = "redemption_unban"
	FailedEmbyOriginRegisterCleanup FailedEmbyAsyncOpOrigin = "register_cleanup"
)

// FailedEmbyAsyncOpAction 记录待执行动作。
type FailedEmbyAsyncOpAction string

const (
	FailedEmbyActionUnban  FailedEmbyAsyncOpAction = "unban"
	FailedEmbyActionDelete FailedEmbyAsyncOpAction = "delete"
)

// FailedEmbyAsyncOp 记录支付履约 / 兑换码 / 注册回滚等链路在事务外调 Emby 失败时
// 需要补偿重试的操作。cron 按 nextAttemptAt 拉取，成功后删除该行；失败时指数退避并保留行。
type FailedEmbyAsyncOp struct {
	ID            string                  `json:"id" gorm:"column:id;type:varchar(25);primaryKey"`
	Origin        FailedEmbyAsyncOpOrigin `json:"origin" gorm:"column:origin;size:32;not null"`
	OriginRefID   string                  `json:"originRefId" gorm:"column:originRefId;size:64;not null"`
	EmbyUserID    string                  `json:"embyUserId" gorm:"column:embyUserId;size:64;not null"`
	Action        FailedEmbyAsyncOpAction `json:"action" gorm:"column:action;size:20;not null"`
	Payload       *string                 `json:"payload,omitempty" gorm:"column:payload;type:text"`
	Retries       int                     `json:"retries" gorm:"column:retries;not null;default:0"`
	NextAttemptAt time.Time               `json:"nextAttemptAt" gorm:"column:nextAttemptAt;not null"`
	LastError     *string                 `json:"lastError,omitempty" gorm:"column:lastError;size:500"`
	CreatedAt     time.Time               `json:"createdAt" gorm:"column:createdAt;autoCreateTime"`
	UpdatedAt     time.Time               `json:"updatedAt" gorm:"column:updatedAt;autoUpdateTime"`
}

func (FailedEmbyAsyncOp) TableName() string {
	return "failed_emby_async_ops"
}

func (f *FailedEmbyAsyncOp) BeforeCreate(tx *gorm.DB) error {
	if f.ID == "" {
		f.ID = generateCUID()
	}
	if f.NextAttemptAt.IsZero() {
		f.NextAttemptAt = time.Now().UTC()
	}
	return nil
}
