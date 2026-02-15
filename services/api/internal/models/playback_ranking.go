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
	ID          string          `json:"id" gorm:"column:id;type:varchar(25);primaryKey"`
	Period      RankingPeriod   `json:"period" gorm:"column:period;type:varchar(10);not null;index:idx_ranking_lookup,priority:1"`
	Category    RankingCategory `json:"category" gorm:"column:category;type:varchar(20);not null;index:idx_ranking_lookup,priority:2"`
	Rank        int             `json:"rank" gorm:"column:rank;not null"`
	ItemName    string          `json:"itemName" gorm:"column:item_name;size:500;not null"`
	PlayCount   int             `json:"playCount" gorm:"column:play_count;not null"`
	Duration    int64           `json:"duration" gorm:"column:duration;not null"` // 总时长（秒）
	SnapshotAt  time.Time       `json:"snapshotAt" gorm:"column:snapshot_at;not null;index:idx_ranking_lookup,priority:3"`
	PeriodStart time.Time       `json:"periodStart" gorm:"column:period_start;not null"`
	PeriodEnd   time.Time       `json:"periodEnd" gorm:"column:period_end;not null"`
	CreatedAt   time.Time       `json:"createdAt" gorm:"column:created_at;autoCreateTime"`
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
