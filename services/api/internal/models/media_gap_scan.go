package models

import (
	"time"

	"gorm.io/gorm"
)

// MediaGapScanStatus 缺集扫描状态枚举。
type MediaGapScanStatus string

const (
	MediaGapScanStatusRunning MediaGapScanStatus = "running"
	MediaGapScanStatusSuccess MediaGapScanStatus = "success"
	MediaGapScanStatusFailed  MediaGapScanStatus = "failed"
)

// MediaGapScan 缺集扫描的持久化执行记录。
//
// 进程内的扫描状态可能因 crash / 重启丢失；本表配合 PostgreSQL advisory lock
// 提供"是否有节点正在扫描"的跨副本视图，以及扫描历史审计。
type MediaGapScan struct {
	ID           string             `json:"id" gorm:"column:id;type:varchar(25);primaryKey"`
	Status       MediaGapScanStatus `json:"status" gorm:"column:status;size:20;not null"`
	NodeID       string             `json:"nodeId" gorm:"column:nodeId;size:64;not null"`
	StartedAt    time.Time          `json:"startedAt" gorm:"column:startedAt;not null;autoCreateTime"`
	FinishedAt   *time.Time         `json:"finishedAt,omitempty" gorm:"column:finishedAt"`
	ErrorMessage *string            `json:"errorMessage,omitempty" gorm:"column:errorMessage;size:500"`
}

func (MediaGapScan) TableName() string {
	return "media_gap_scans"
}

func (m *MediaGapScan) BeforeCreate(tx *gorm.DB) error {
	if m.ID == "" {
		m.ID = generateCUID()
	}
	return nil
}
