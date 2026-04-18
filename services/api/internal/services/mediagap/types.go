package mediagap

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/konghang/ember/backend/internal/models"
)

type ListQuery struct {
	Keyword     string
	Status      string
	AirDateFrom string
	AirDateTo   string
	Page        int
	PageSize    int
}

type ListRequest struct {
	Keyword     string
	Status      string
	AirDateFrom *time.Time
	AirDateTo   *time.Time
	Page        int
	PageSize    int
}

type ListResponse struct {
	Data  []models.MediaGap `json:"data"`
	Total int64             `json:"total"`
}

type ScanRequest struct {
	TMDBID string
	Force  bool
}

type ScanResult struct {
	ScannedAt        time.Time `json:"scannedAt"`
	ScannedSeries    int       `json:"scannedSeries"`
	SkippedSeries    int       `json:"skippedSeries"`
	ExaminedEpisodes int       `json:"examinedEpisodes"`
	Created          int       `json:"created"`
	Updated          int       `json:"updated"`
	Ingested         int       `json:"ingested"`
}

type WebhookIngestPayload struct {
	TmdbID     string
	SeriesID   string
	Season     int
	Episode    int
	EmbyItemID string
	ItemName   string
}

type SearchCandidate struct {
	Title   string                 `json:"title"`
	Site    string                 `json:"site,omitempty"`
	Size    int64                  `json:"size,omitempty"`
	Seeders int                    `json:"seeders,omitempty"`
	IsPack  bool                   `json:"isPack"`
	Tags    []string               `json:"tags,omitempty"`
	Payload map[string]interface{} `json:"payload,omitempty"`
}

type SearchSnapshot struct {
	Keyword    string            `json:"keyword"`
	SearchedAt time.Time         `json:"searchedAt"`
	Candidates []SearchCandidate `json:"candidates"`
}

type DispatchSnapshot struct {
	RequestedAt time.Time       `json:"requestedAt"`
	Candidate   SearchCandidate `json:"candidate"`
}

type DispatchRequest struct {
	Candidate SearchCandidate `json:"candidate"`
}

type MediaGapDTO struct {
	ID               string                `json:"id"`
	TmdbID           string                `json:"tmdbId"`
	EmbySeriesID     string                `json:"embySeriesId,omitempty"`
	SeriesName       string                `json:"seriesName"`
	Season           int                   `json:"season"`
	Episode          int                   `json:"episode"`
	AirDate          time.Time             `json:"airDate"`
	Status           models.MediaGapStatus `json:"status"`
	LastScannedAt    *time.Time            `json:"lastScannedAt,omitempty"`
	LastSearchedAt   *time.Time            `json:"lastSearchedAt,omitempty"`
	RequestedAt      *time.Time            `json:"requestedAt,omitempty"`
	IngestedAt       *time.Time            `json:"ingestedAt,omitempty"`
	IgnoredAt        *time.Time            `json:"ignoredAt,omitempty"`
	IgnoreReason     string                `json:"ignoreReason,omitempty"`
	SearchSnapshot   *SearchSnapshot       `json:"searchSnapshot,omitempty"`
	DispatchSnapshot *DispatchSnapshot     `json:"dispatchSnapshot,omitempty"`
	CreatedAt        time.Time             `json:"createdAt"`
	UpdatedAt        time.Time             `json:"updatedAt"`
}

func toDTO(gap models.MediaGap) *MediaGapDTO {
	dto := &MediaGapDTO{
		ID:             gap.ID,
		TmdbID:         gap.TmdbID,
		EmbySeriesID:   gap.EmbySeriesID,
		SeriesName:     gap.SeriesName,
		Season:         gap.Season,
		Episode:        gap.Episode,
		AirDate:        gap.AirDate,
		Status:         gap.Status,
		LastScannedAt:  gap.LastScannedAt,
		LastSearchedAt: gap.LastSearchedAt,
		RequestedAt:    gap.RequestedAt,
		IngestedAt:     gap.IngestedAt,
		IgnoredAt:      gap.IgnoredAt,
		IgnoreReason:   gap.IgnoreReason,
		CreatedAt:      gap.CreatedAt,
		UpdatedAt:      gap.UpdatedAt,
	}

	if gap.SearchSnapshot != "" {
		var snapshot SearchSnapshot
		if err := json.Unmarshal([]byte(gap.SearchSnapshot), &snapshot); err == nil {
			dto.SearchSnapshot = &snapshot
		}
	}
	if gap.DispatchSnapshot != "" {
		var snapshot DispatchSnapshot
		if err := json.Unmarshal([]byte(gap.DispatchSnapshot), &snapshot); err == nil {
			dto.DispatchSnapshot = &snapshot
		}
	}
	return dto
}

func buildDefaultSearchKeyword(gap models.MediaGap) string {
	return fmt.Sprintf("%s S%02dE%02d", gap.SeriesName, gap.Season, gap.Episode)
}
