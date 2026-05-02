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

type MediaGapIgnoreReasonCode string

const (
	MediaGapIgnoreReasonManual             MediaGapIgnoreReasonCode = "manual"
	MediaGapIgnoreReasonSeasonNotActivated MediaGapIgnoreReasonCode = "season_not_activated"
)

// MediaGap 缺集工单模型。
type MediaGap struct {
	ID                string         `json:"id" gorm:"column:id;type:varchar(25);primaryKey"`
	TmdbID            string         `json:"tmdbId" gorm:"column:tmdb_id;size:50;not null;uniqueIndex:uk_media_gap_episode,priority:1"`
	EmbySeriesID      string         `json:"embySeriesId,omitempty" gorm:"column:emby_series_id;size:50;not null;default:'';index"`
	SeriesName        string         `json:"seriesName" gorm:"column:series_name;size:255;not null;default:''"`
	Season            int            `json:"season" gorm:"column:season;not null;uniqueIndex:uk_media_gap_episode,priority:2"`
	Episode           int            `json:"episode" gorm:"column:episode;not null;uniqueIndex:uk_media_gap_episode,priority:3"`
	AirDate           time.Time      `json:"airDate" gorm:"column:air_date;not null;index:idx_media_gaps_status_air_date,priority:2"` // date 列，不含时区
	Status            MediaGapStatus `json:"status" gorm:"column:status;type:varchar(20);not null;default:'MISSING';index:idx_media_gaps_status_air_date,priority:1"`
	SearchSnapshot    string         `json:"searchSnapshot,omitempty" gorm:"column:search_snapshot;type:text;not null;default:''"`
	DispatchSnapshot  string         `json:"dispatchSnapshot,omitempty" gorm:"column:dispatch_snapshot;type:text;not null;default:''"`
	LastDispatchError *string        `json:"lastDispatchError,omitempty" gorm:"column:last_dispatch_error;size:500"`
	LastScannedAt     *time.Time     `json:"lastScannedAt,omitempty" gorm:"column:last_scanned_at"`
	LastSearchedAt    *time.Time     `json:"lastSearchedAt,omitempty" gorm:"column:last_searched_at"`
	RequestedAt       *time.Time     `json:"requestedAt,omitempty" gorm:"column:requested_at"`
	IngestedAt        *time.Time     `json:"ingestedAt,omitempty" gorm:"column:ingested_at"`
	IgnoredAt         *time.Time     `json:"ignoredAt,omitempty" gorm:"column:ignored_at"`
	IgnoreReasonCode  *string        `json:"ignoreReasonCode,omitempty" gorm:"column:ignore_reason_code;size:50"`
	IgnoreReason      string         `json:"ignoreReason,omitempty" gorm:"column:ignore_reason;type:text;not null;default:''"`
	CreatedAt         time.Time      `json:"createdAt" gorm:"column:created_at;autoCreateTime"`
	UpdatedAt         time.Time      `json:"updatedAt" gorm:"column:updated_at;autoUpdateTime"`
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
