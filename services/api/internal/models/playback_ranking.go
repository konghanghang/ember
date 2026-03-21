package models

import (
	"time"

	"gorm.io/gorm"
)

// RankingPeriod 排行周期
type RankingPeriod string

const (
	RankingDaily  RankingPeriod = "daily"
	RankingWeekly RankingPeriod = "weekly"
)

// RankingCategory 排行类别
type RankingCategory string

const (
	RankingMediaMovie   RankingCategory = "media_movie"   // 电影播放榜
	RankingMediaEpisode RankingCategory = "media_episode" // 剧集播放榜
)

// PlaybackRanking 播放排行快照
type PlaybackRanking struct {
	ID             string          `json:"id" gorm:"column:id;type:varchar(25);primaryKey"`
	BatchID        string          `json:"batchId" gorm:"column:batch_id;type:varchar(25);not null;default:'';index:idx_ranking_batch,priority:1"`
	Period         RankingPeriod   `json:"period" gorm:"column:period;type:varchar(10);not null;index:idx_ranking_period_window,priority:1;index:idx_ranking_item,priority:1"`
	Category       RankingCategory `json:"category" gorm:"column:category;type:varchar(20);not null;index:idx_ranking_batch,priority:2;index:idx_ranking_item,priority:2"`
	Rank           int             `json:"rank" gorm:"column:rank;not null;index:idx_ranking_batch,priority:3"`
	ItemKey        string          `json:"itemKey" gorm:"column:item_key;type:varchar(128);not null;default:'';index:idx_ranking_item,priority:3"`
	ItemSourceType string          `json:"itemSourceType" gorm:"column:item_source_type;type:varchar(32);not null;default:''"`
	ItemName       string          `json:"itemName" gorm:"column:item_name;size:500;not null"`
	PlayCount      int             `json:"playCount" gorm:"column:play_count;not null"`
	Duration       int64           `json:"duration" gorm:"column:duration;not null"` // 总时长（秒）
	SnapshotAt     time.Time       `json:"snapshotAt" gorm:"column:snapshot_at;not null;index:idx_ranking_period_window,priority:4"`
	PeriodStart    time.Time       `json:"periodStart" gorm:"column:period_start;not null;index:idx_ranking_period_window,priority:2;index:idx_ranking_item,priority:4"`
	PeriodEnd      time.Time       `json:"periodEnd" gorm:"column:period_end;not null;index:idx_ranking_period_window,priority:3;index:idx_ranking_item,priority:5"`
	CreatedAt      time.Time       `json:"createdAt" gorm:"column:created_at;autoCreateTime"`
}

func (PlaybackRanking) TableName() string {
	return "playback_rankings"
}

func (r *PlaybackRanking) BeforeCreate(tx *gorm.DB) error {
	_ = tx
	if r.ID == "" {
		r.ID = generateCUID()
	}
	return nil
}
