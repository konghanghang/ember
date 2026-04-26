package mediagap

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"time"

	moviepilotint "github.com/konghang/ember/backend/internal/integrations/moviepilot"
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

type GroupedSortMode string

const (
	GroupedSortMissing   GroupedSortMode = "missing"
	GroupedSortUpdated   GroupedSortMode = "updated"
	GroupedSortRequested GroupedSortMode = "requested"
	GroupedSortName      GroupedSortMode = "name"
)

type GroupedListQuery struct {
	Keyword     string
	Status      string
	AirDateFrom string
	AirDateTo   string
	Sort        string
	Page        int
	PageSize    int
}

type GroupedListRequest struct {
	Keyword     string
	Status      string
	AirDateFrom *time.Time
	AirDateTo   *time.Time
	Sort        string
	Page        int
	PageSize    int
}

type GroupedSeason struct {
	Season int               `json:"season"`
	Gaps   []models.MediaGap `json:"gaps"`
}

type GroupedSeries struct {
	Key             string            `json:"key"`
	SeriesName      string            `json:"seriesName"`
	TmdbID          string            `json:"tmdbId,omitempty"`
	EmbySeriesID        string            `json:"embySeriesId,omitempty"`
	Gaps                []models.MediaGap `json:"gaps"`
	Seasons             []GroupedSeason   `json:"seasons"`
	TotalGaps           int               `json:"totalGaps"`
	MissingCount        int               `json:"missingCount"`
	SearchedCount       int               `json:"searchedCount"`
	RequestedCount      int               `json:"requestedCount"`
	DispatchFailedCount int               `json:"dispatchFailedCount"`
	IngestedCount       int               `json:"ingestedCount"`
	IgnoredCount        int               `json:"ignoredCount"`
	LatestUpdatedAt     *time.Time        `json:"latestUpdatedAt,omitempty"`
}

type GroupedSummary struct {
	MissingCount        int `json:"missingCount"`
	SearchedCount       int `json:"searchedCount"`
	RequestedCount      int `json:"requestedCount"`
	DispatchFailedCount int `json:"dispatchFailedCount"`
	IngestedCount       int `json:"ingestedCount"`
	IgnoredCount        int `json:"ignoredCount"`
}

type GroupedListResponse struct {
	Data      []GroupedSeries `json:"data"`
	Total     int64           `json:"total"`
	ItemTotal int64           `json:"itemTotal"`
	Page      int             `json:"page"`
	PageSize  int             `json:"pageSize"`
	Summary   GroupedSummary  `json:"summary"`
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
	ID          string                 `json:"id,omitempty"`
	Title       string                 `json:"title"`
	Description string                 `json:"description,omitempty"`
	PublishDate string                 `json:"publishDate,omitempty"`
	Site        string                 `json:"site,omitempty"`
	Size        int64                  `json:"size,omitempty"`
	Seeders     int                    `json:"seeders,omitempty"`
	IsPack      bool                   `json:"isPack"`
	MatchMode   string                 `json:"matchMode,omitempty"`
	Tags        []string               `json:"tags,omitempty"`
	Payload     map[string]interface{} `json:"payload,omitempty"`
}

type SearchSnapshot struct {
	Keyword       string            `json:"keyword"`
	Query         string            `json:"query,omitempty"`
	FallbackQuery string            `json:"fallbackQuery,omitempty"`
	MatchMode     string            `json:"matchMode,omitempty"`
	Source        string            `json:"source,omitempty"`
	SearchedAt    time.Time         `json:"searchedAt"`
	Candidates    []SearchCandidate `json:"candidates"`
}

type DispatchSnapshot struct {
	RequestedAt time.Time       `json:"requestedAt"`
	Candidate   SearchCandidate `json:"candidate"`
}

type DispatchRequest struct {
	Candidate SearchCandidate `json:"candidate"`
}

type MediaGapDTO struct {
	ID                string                `json:"id"`
	TmdbID            string                `json:"tmdbId"`
	EmbySeriesID      string                `json:"embySeriesId,omitempty"`
	SeriesName        string                `json:"seriesName"`
	Season            int                   `json:"season"`
	Episode           int                   `json:"episode"`
	AirDate           time.Time             `json:"airDate"`
	Status            models.MediaGapStatus `json:"status"`
	LastScannedAt     *time.Time            `json:"lastScannedAt,omitempty"`
	LastSearchedAt    *time.Time            `json:"lastSearchedAt,omitempty"`
	RequestedAt       *time.Time            `json:"requestedAt,omitempty"`
	IngestedAt        *time.Time            `json:"ingestedAt,omitempty"`
	IgnoredAt         *time.Time            `json:"ignoredAt,omitempty"`
	IgnoreReason      string                `json:"ignoreReason,omitempty"`
	LastDispatchError *string               `json:"lastDispatchError,omitempty"`
	SearchSnapshot    *SearchSnapshot       `json:"searchSnapshot,omitempty"`
	DispatchSnapshot  *DispatchSnapshot     `json:"dispatchSnapshot,omitempty"`
	CreatedAt         time.Time             `json:"createdAt"`
	UpdatedAt         time.Time             `json:"updatedAt"`
}

func toDTO(gap models.MediaGap) *MediaGapDTO {
	dto := &MediaGapDTO{
		ID:                gap.ID,
		TmdbID:            gap.TmdbID,
		EmbySeriesID:      gap.EmbySeriesID,
		SeriesName:        gap.SeriesName,
		Season:            gap.Season,
		Episode:           gap.Episode,
		AirDate:           gap.AirDate,
		Status:            gap.Status,
		LastScannedAt:     gap.LastScannedAt,
		LastSearchedAt:    gap.LastSearchedAt,
		RequestedAt:       gap.RequestedAt,
		IngestedAt:        gap.IngestedAt,
		LastDispatchError: gap.LastDispatchError,
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

func isSearchableMediaGapStatus(status models.MediaGapStatus) bool {
	switch status {
	case models.MediaGapStatusMissing, models.MediaGapStatusSearched, models.MediaGapStatusRequested, models.MediaGapStatusDispatchFailed:
		return true
	default:
		return false
	}
}

func isDispatchableMediaGapStatus(status models.MediaGapStatus) bool {
	switch status {
	case models.MediaGapStatusMissing, models.MediaGapStatusSearched, models.MediaGapStatusRequested, models.MediaGapStatusDispatchFailed:
		return true
	default:
		return false
	}
}

func buildSearchSnapshot(gap models.MediaGap, searchedAt time.Time, resp *moviepilotint.GapSearchResponse) SearchSnapshot {
	snapshot := SearchSnapshot{
		Keyword:    buildDefaultSearchKeyword(gap),
		Source:     "MoviePilot",
		SearchedAt: searchedAt,
		Candidates: []SearchCandidate{},
	}
	if resp == nil {
		return snapshot
	}

	snapshot.Query = resp.Query
	snapshot.FallbackQuery = resp.FallbackQuery
	snapshot.MatchMode = resp.MatchMode
	snapshot.Candidates = normalizeSearchCandidates(resp.Candidates)
	return snapshot
}

func normalizeSearchCandidates(candidates []moviepilotint.GapSearchCandidate) []SearchCandidate {
	if len(candidates) == 0 {
		return []SearchCandidate{}
	}

	normalized := make([]SearchCandidate, 0, len(candidates))
	for _, candidate := range candidates {
		normalized = append(normalized, SearchCandidate{
			ID:          candidate.ID,
			Title:       candidate.Title,
			Description: candidate.Description,
			PublishDate: candidate.PublishDate,
			Site:        candidate.Site,
			Size:        candidate.Size,
			Seeders:     candidate.Seeders,
			IsPack:      candidate.IsPack,
			MatchMode:   candidate.MatchMode,
			Tags:        candidate.Tags,
			Payload:     candidate.Payload,
		})
	}

	return normalized
}

func normalizeGroupedSortMode(sort string) GroupedSortMode {
	switch GroupedSortMode(strings.TrimSpace(sort)) {
	case GroupedSortUpdated:
		return GroupedSortUpdated
	case GroupedSortRequested:
		return GroupedSortRequested
	case GroupedSortName:
		return GroupedSortName
	default:
		return GroupedSortMissing
	}
}

func buildGroupedSeries(items []models.MediaGap, sort string) ([]GroupedSeries, GroupedSummary) {
	groupMap := make(map[string]*GroupedSeries)
	summary := GroupedSummary{}

	for _, gap := range items {
		switch gap.Status {
		case models.MediaGapStatusMissing:
			summary.MissingCount++
		case models.MediaGapStatusSearched:
			summary.SearchedCount++
		case models.MediaGapStatusRequested:
			summary.RequestedCount++
		case models.MediaGapStatusDispatchFailed:
			summary.DispatchFailedCount++
		case models.MediaGapStatusIngested:
			summary.IngestedCount++
		case models.MediaGapStatusIgnored:
			summary.IgnoredCount++
		}

		key := buildGroupedSeriesKey(gap)
		existing := groupMap[key]
		if existing == nil {
			existing = &GroupedSeries{
				Key:          key,
				SeriesName:   gap.SeriesName,
				TmdbID:       gap.TmdbID,
				EmbySeriesID: gap.EmbySeriesID,
				Gaps:         make([]models.MediaGap, 0, 1),
				Seasons:      make([]GroupedSeason, 0),
			}
			groupMap[key] = existing
		}

		existing.Gaps = append(existing.Gaps, gap)
		existing.TotalGaps++
		switch gap.Status {
		case models.MediaGapStatusMissing:
			existing.MissingCount++
		case models.MediaGapStatusSearched:
			existing.SearchedCount++
		case models.MediaGapStatusRequested:
			existing.RequestedCount++
		case models.MediaGapStatusDispatchFailed:
			existing.DispatchFailedCount++
		case models.MediaGapStatusIngested:
			existing.IngestedCount++
		case models.MediaGapStatusIgnored:
			existing.IgnoredCount++
		}

		if existing.LatestUpdatedAt == nil || gap.UpdatedAt.After(*existing.LatestUpdatedAt) {
			updatedAt := gap.UpdatedAt
			existing.LatestUpdatedAt = &updatedAt
		}
	}

	grouped := make([]GroupedSeries, 0, len(groupMap))
	for _, group := range groupMap {
		seasonMap := make(map[int][]models.MediaGap)
		for _, gap := range group.Gaps {
			seasonMap[gap.Season] = append(seasonMap[gap.Season], gap)
		}

		group.Seasons = make([]GroupedSeason, 0, len(seasonMap))
		for season, gaps := range seasonMap {
			sortMediaGapItems(gaps)
			group.Seasons = append(group.Seasons, GroupedSeason{
				Season: season,
				Gaps:   gaps,
			})
		}
		sortGroupedSeasons(group.Seasons)
		sortMediaGapItems(group.Gaps)
		grouped = append(grouped, *group)
	}

	sortGroupedSeries(grouped, normalizeGroupedSortMode(sort))
	return grouped, summary
}

func buildGroupedSeriesKey(gap models.MediaGap) string {
	switch {
	case strings.TrimSpace(gap.TmdbID) != "":
		return gap.TmdbID
	case strings.TrimSpace(gap.EmbySeriesID) != "":
		return gap.EmbySeriesID
	case strings.TrimSpace(gap.SeriesName) != "":
		return gap.SeriesName
	default:
		return gap.ID
	}
}

func mediaGapStatusOrder(status models.MediaGapStatus) int {
	switch status {
	case models.MediaGapStatusMissing:
		return 0
	case models.MediaGapStatusSearched:
		return 1
	case models.MediaGapStatusRequested:
		return 2
	case models.MediaGapStatusDispatchFailed:
		return 3
	case models.MediaGapStatusIngested:
		return 4
	case models.MediaGapStatusIgnored:
		return 5
	default:
		return 9
	}
}

func sortMediaGapItems(items []models.MediaGap) {
	slices.SortStableFunc(items, func(left, right models.MediaGap) int {
		if diff := mediaGapStatusOrder(left.Status) - mediaGapStatusOrder(right.Status); diff != 0 {
			return diff
		}
		if left.Season != right.Season {
			return left.Season - right.Season
		}
		return left.Episode - right.Episode
	})
}

func sortGroupedSeasons(seasons []GroupedSeason) {
	slices.SortStableFunc(seasons, func(left, right GroupedSeason) int {
		return left.Season - right.Season
	})
}

func sortGroupedSeries(groups []GroupedSeries, sortMode GroupedSortMode) {
	slices.SortStableFunc(groups, func(left, right GroupedSeries) int {
		switch sortMode {
		case GroupedSortUpdated:
			leftTime := ""
			rightTime := ""
			if left.LatestUpdatedAt != nil {
				leftTime = left.LatestUpdatedAt.Format(time.RFC3339Nano)
			}
			if right.LatestUpdatedAt != nil {
				rightTime = right.LatestUpdatedAt.Format(time.RFC3339Nano)
			}
			if leftTime != rightTime {
				return strings.Compare(rightTime, leftTime)
			}
			if left.MissingCount != right.MissingCount {
				return right.MissingCount - left.MissingCount
			}
			return strings.Compare(left.SeriesName, right.SeriesName)
		case GroupedSortRequested:
			if left.RequestedCount != right.RequestedCount {
				return right.RequestedCount - left.RequestedCount
			}
			if left.MissingCount != right.MissingCount {
				return right.MissingCount - left.MissingCount
			}
			return strings.Compare(left.SeriesName, right.SeriesName)
		case GroupedSortName:
			return strings.Compare(left.SeriesName, right.SeriesName)
		case GroupedSortMissing:
			fallthrough
		default:
			if left.MissingCount != right.MissingCount {
				return right.MissingCount - left.MissingCount
			}
			if left.RequestedCount != right.RequestedCount {
				return right.RequestedCount - left.RequestedCount
			}
			return strings.Compare(left.SeriesName, right.SeriesName)
		}
	})
}
