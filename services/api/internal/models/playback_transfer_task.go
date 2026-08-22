package models

import (
	"time"

	"gorm.io/gorm"
)

// PlaybackTransferTaskStatus is the persisted lifecycle of one retained 115
// playback transfer attempt.
type PlaybackTransferTaskStatus string

const (
	PlaybackTransferTaskStatusPending      PlaybackTransferTaskStatus = "pending"
	PlaybackTransferTaskStatusInitializing PlaybackTransferTaskStatus = "initializing"
	PlaybackTransferTaskStatusChallenging  PlaybackTransferTaskStatus = "challenging"
	PlaybackTransferTaskStatusVerifying    PlaybackTransferTaskStatus = "verifying"
	PlaybackTransferTaskStatusSucceeded    PlaybackTransferTaskStatus = "succeeded"
	PlaybackTransferTaskStatusFailed       PlaybackTransferTaskStatus = "failed"
)

// PlaybackTransferTask records one attempt to retain source content in the
// configured playback account. Signed URLs and Cookies are never persisted.
type PlaybackTransferTask struct {
	ID                string                     `json:"id" gorm:"column:id;type:varchar(25);primaryKey"`
	SourceAccountID   string                     `json:"sourceAccountId" gorm:"column:source_account_id;type:varchar(25);not null"`
	PlaybackAccountID string                     `json:"playbackAccountId" gorm:"column:playback_account_id;type:varchar(25);not null"`
	SHA1              string                     `json:"-" gorm:"column:sha1;type:char(40);not null"`
	Size              int64                      `json:"size" gorm:"column:size;not null"`
	FileName          string                     `json:"fileName" gorm:"column:file_name;type:varchar(1024);not null"`
	TargetParentID    string                     `json:"targetParentId" gorm:"column:target_parent_id;type:varchar(64);not null"`
	Status            PlaybackTransferTaskStatus `json:"status" gorm:"column:status;type:varchar(24);not null;default:'pending'"`
	TargetFileID      *string                    `json:"-" gorm:"column:target_file_id;type:varchar(64)"`
	TargetPickCode    *string                    `json:"-" gorm:"column:target_pick_code;type:varchar(128)"`
	AttemptCount      int                        `json:"attemptCount" gorm:"column:attempt_count;not null;default:1"`
	LastErrorCode     *string                    `json:"lastErrorCode,omitempty" gorm:"column:last_error_code;type:varchar(100)"`
	LastErrorMessage  *string                    `json:"lastErrorMessage,omitempty" gorm:"column:last_error_message;type:varchar(500)"`
	StartedAt         time.Time                  `json:"startedAt" gorm:"column:started_at;not null"`
	CompletedAt       *time.Time                 `json:"completedAt,omitempty" gorm:"column:completed_at"`
	LastAccessedAt    *time.Time                 `json:"lastAccessedAt,omitempty" gorm:"column:last_accessed_at"`
	CreatedAt         time.Time                  `json:"createdAt" gorm:"column:created_at;autoCreateTime"`
	UpdatedAt         time.Time                  `json:"updatedAt" gorm:"column:updated_at;autoUpdateTime"`
}

// TableName returns the SQL table managed by the playback transfer migration.
func (PlaybackTransferTask) TableName() string {
	return "playback_transfer_tasks"
}

// BeforeCreate generates the identifier and applies safe attempt defaults.
func (task *PlaybackTransferTask) BeforeCreate(tx *gorm.DB) error {
	_ = tx
	if task.ID == "" {
		task.ID = generateCUID()
	}
	if task.Status == "" {
		task.Status = PlaybackTransferTaskStatusPending
	}
	if task.AttemptCount <= 0 {
		task.AttemptCount = 1
	}
	if task.StartedAt.IsZero() {
		task.StartedAt = time.Now().UTC()
	}
	return nil
}
