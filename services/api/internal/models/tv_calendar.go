package models

import (
	"time"

	"gorm.io/gorm"
)

const (
	TVCalendarStatusReady    = "ready"
	TVCalendarStatusMissing  = "missing"
	TVCalendarStatusUpcoming = "upcoming"
	TVCalendarStatusToday    = "today"
)

// TVCalendarSource 全局追剧源
type TVCalendarSource struct {
	ID                    string     `json:"id" gorm:"column:id;type:varchar(25);primaryKey"`
	TmdbID                string     `json:"tmdbId" gorm:"column:tmdb_id;size:50;not null;uniqueIndex"`
	SeriesID              string     `json:"seriesId,omitempty" gorm:"column:series_id;size:50;not null;default:'';index"`
	ShowName              string     `json:"showName" gorm:"column:show_name;size:255;not null;default:''"`
	PosterURL             string     `json:"posterUrl,omitempty" gorm:"column:poster_url;size:500;not null;default:''"`
	Overview              string     `json:"overview,omitempty" gorm:"column:overview;type:text;not null;default:''"`
	EmbyStatus            string     `json:"embyStatus" gorm:"column:emby_status;size:20;not null;default:'continuing'"`
	LastEpisodeIngestedAt *time.Time `json:"lastEpisodeIngestedAt,omitempty" gorm:"column:last_episode_ingested_at;index"`
	LastSyncedAt          *time.Time `json:"lastSyncedAt,omitempty" gorm:"column:last_synced_at"`
	LastFullSyncAt        *time.Time `json:"lastFullSyncAt,omitempty" gorm:"column:last_full_sync_at"`
	LastCorrectionAt      *time.Time `json:"lastCorrectionAt,omitempty" gorm:"column:last_correction_at"`
	CreatedAt             time.Time  `json:"createdAt" gorm:"column:created_at;autoCreateTime"`
	UpdatedAt             time.Time  `json:"updatedAt" gorm:"column:updated_at;autoUpdateTime"`
}

func (TVCalendarSource) TableName() string {
	return "tv_calendar_sources"
}

func (s *TVCalendarSource) BeforeCreate(tx *gorm.DB) error {
	if s.ID == "" {
		s.ID = generateCUID()
	}
	return nil
}

// TVCalendarItem 追剧日历条目
type TVCalendarItem struct {
	ID          string    `json:"id" gorm:"column:id;type:varchar(25);primaryKey"`
	TmdbID      string    `json:"tmdbId" gorm:"column:tmdb_id;size:50;not null;index;uniqueIndex:uk_tv_calendar_episode,priority:1"`
	SeriesID    string    `json:"seriesId,omitempty" gorm:"column:series_id;size:50;index"`
	Season      int       `json:"season" gorm:"column:season;not null;uniqueIndex:uk_tv_calendar_episode,priority:2"`
	Episode     int       `json:"episode" gorm:"column:episode;not null;uniqueIndex:uk_tv_calendar_episode,priority:3"`
	AirDate     time.Time `json:"airDate" gorm:"column:air_date;not null;index"` // date 列，不含时区
	EpisodeName string    `json:"episodeName" gorm:"column:episode_name;size:255"`
	Overview    string    `json:"overview,omitempty" gorm:"column:overview;type:text;not null;default:''"`
	Status      string    `json:"status" gorm:"column:status;size:20;not null;default:'upcoming'"`
	EmbyItemID  string    `json:"embyItemId,omitempty" gorm:"column:emby_item_id;size:50"`
	LastChecked time.Time `json:"lastChecked" gorm:"column:last_checked"`
	CreatedAt   time.Time `json:"createdAt" gorm:"column:created_at;autoCreateTime"`
	UpdatedAt   time.Time `json:"updatedAt" gorm:"column:updated_at;autoUpdateTime"`
}

func (TVCalendarItem) TableName() string {
	return "tv_calendar_items"
}

func (i *TVCalendarItem) BeforeCreate(tx *gorm.DB) error {
	if i.ID == "" {
		i.ID = generateCUID()
	}
	return nil
}

// TVCalendarSubscription 用户追剧订阅
type TVCalendarSubscription struct {
	ID        string    `json:"id" gorm:"column:id;type:varchar(25);primaryKey"`
	UserID    string    `json:"userId" gorm:"column:user_id;type:varchar(25);not null;index;uniqueIndex:uk_tv_calendar_subscription,priority:1"`
	TmdbID    string    `json:"tmdbId" gorm:"column:tmdb_id;size:50;not null;index;uniqueIndex:uk_tv_calendar_subscription,priority:2"`
	ShowName  string    `json:"showName" gorm:"column:show_name;size:255;not null"`
	PosterURL string    `json:"posterUrl" gorm:"column:poster_url;size:500"`
	CreatedAt time.Time `json:"createdAt" gorm:"column:created_at;autoCreateTime"`
}

func (TVCalendarSubscription) TableName() string {
	return "tv_calendar_subscriptions"
}

func (s *TVCalendarSubscription) BeforeCreate(tx *gorm.DB) error {
	if s.ID == "" {
		s.ID = generateCUID()
	}
	return nil
}

// TMDBCache TMDB 响应持久化缓存
type TMDBCache struct {
	ID         string    `json:"id" gorm:"column:id;type:varchar(25);primaryKey"`
	CacheKey   string    `json:"cacheKey" gorm:"column:cache_key;size:255;not null;uniqueIndex"`
	CacheValue string    `json:"cacheValue" gorm:"column:cache_value;type:text;not null"`
	ExpiresAt  time.Time `json:"expiresAt" gorm:"column:expires_at;index"`
	CreatedAt  time.Time `json:"createdAt" gorm:"column:created_at;autoCreateTime"`
}

func (TMDBCache) TableName() string {
	return "tmdb_cache"
}

func (c *TMDBCache) BeforeCreate(tx *gorm.DB) error {
	if c.ID == "" {
		c.ID = generateCUID()
	}
	return nil
}
