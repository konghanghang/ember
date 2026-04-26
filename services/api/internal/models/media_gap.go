package models

import (
	"time"

	"gorm.io/gorm"
)

// MediaGapStatus 缺集工单状态。
type MediaGapStatus string

const (
	MediaGapStatusMissing        MediaGapStatus = "MISSING"
	MediaGapStatusSearched       MediaGapStatus = "SEARCHED"
	MediaGapStatusRequested      MediaGapStatus = "REQUESTED"
	MediaGapStatusIngested       MediaGapStatus = "INGESTED"
	MediaGapStatusIgnored        MediaGapStatus = "IGNORED"
	MediaGapStatusDispatchFailed MediaGapStatus = "DISPATCH_FAILED"
)

// MediaGap 缺集工单模型。
type MediaGap struct {
	ID                string         `json:"id" gorm:"column:id;type:varchar(25);primaryKey"`
	TmdbID            string         `json:"tmdbId" gorm:"column:tmdbId;size:50;not null;uniqueIndex:uk_media_gap_episode,priority:1;index"`
	EmbySeriesID      string         `json:"embySeriesId,omitempty" gorm:"column:embySeriesId;size:50;not null;default:'';index"`
	SeriesName        string         `json:"seriesName" gorm:"column:seriesName;size:255;not null;default:''"`
	Season            int            `json:"season" gorm:"column:season;not null;uniqueIndex:uk_media_gap_episode,priority:2"`
	Episode           int            `json:"episode" gorm:"column:episode;not null;uniqueIndex:uk_media_gap_episode,priority:3"`
	AirDate           time.Time      `json:"airDate" gorm:"column:airDate;not null;index:idx_media_gaps_status_air_date,priority:2"`
	Status            MediaGapStatus `json:"status" gorm:"column:status;type:varchar(20);not null;default:'MISSING';index:idx_media_gaps_status_air_date,priority:1"`
	SearchSnapshot    string         `json:"searchSnapshot,omitempty" gorm:"column:searchSnapshot;type:text;not null;default:''"`
	DispatchSnapshot  string         `json:"dispatchSnapshot,omitempty" gorm:"column:dispatchSnapshot;type:text;not null;default:''"`
	LastDispatchError *string        `json:"lastDispatchError,omitempty" gorm:"column:lastDispatchError;size:500"`
	LastScannedAt     *time.Time     `json:"lastScannedAt,omitempty" gorm:"column:lastScannedAt"`
	LastSearchedAt    *time.Time     `json:"lastSearchedAt,omitempty" gorm:"column:lastSearchedAt"`
	RequestedAt       *time.Time     `json:"requestedAt,omitempty" gorm:"column:requestedAt"`
	IngestedAt        *time.Time     `json:"ingestedAt,omitempty" gorm:"column:ingestedAt"`
	IgnoredAt         *time.Time     `json:"ignoredAt,omitempty" gorm:"column:ignoredAt"`
	IgnoreReason      string         `json:"ignoreReason,omitempty" gorm:"column:ignoreReason;type:text;not null;default:''"`
	CreatedAt         time.Time      `json:"createdAt" gorm:"column:createdAt;autoCreateTime"`
	UpdatedAt         time.Time      `json:"updatedAt" gorm:"column:updatedAt;autoUpdateTime"`
}

func (MediaGap) TableName() string {
	return "media_gaps"
}

func (g *MediaGap) BeforeCreate(tx *gorm.DB) error {
	if g.ID == "" {
		g.ID = generateCUID()
	}
	return nil
}
