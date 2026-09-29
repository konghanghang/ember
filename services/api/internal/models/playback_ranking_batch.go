package models

import "time"

// PlaybackRankingBatch 是一份已提交的整期排行；空榜同样存在，历史总时长未知时保留 NULL。
type PlaybackRankingBatch struct {
	ID            string        `json:"id" gorm:"column:id;type:varchar(32);primaryKey"`
	Period        RankingPeriod `json:"period" gorm:"column:period;type:varchar(10);not null;uniqueIndex:uq_playback_ranking_batches_period,priority:1"`
	PeriodStart   time.Time     `json:"periodStart" gorm:"column:period_start;not null;uniqueIndex:uq_playback_ranking_batches_period,priority:2"`
	PeriodEnd     time.Time     `json:"periodEnd" gorm:"column:period_end;not null;uniqueIndex:uq_playback_ranking_batches_period,priority:3"`
	SnapshotAt    time.Time     `json:"snapshotAt" gorm:"column:snapshot_at;not null"`
	TotalDuration *int64        `json:"totalDuration" gorm:"column:total_duration"`
	CreatedAt     time.Time     `json:"createdAt" gorm:"column:created_at;autoCreateTime"`
}

// TableName 将整期快照与一行一个排名位置的明细表分离。
func (PlaybackRankingBatch) TableName() string {
	return "playback_ranking_batches"
}
