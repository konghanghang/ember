package mediagap

import (
	"encoding/json"
	"testing"
	"time"

	moviepilotint "github.com/konghang/ember/backend/internal/integrations/moviepilot"
	"github.com/konghang/ember/backend/internal/models"
)

func TestToDTOParsesSnapshotsAndCopiesModelFields(t *testing.T) {
	airDate := time.Date(2026, 4, 19, 0, 0, 0, 0, time.UTC)
	lastScannedAt := time.Date(2026, 4, 19, 1, 0, 0, 0, time.UTC)
	lastSearchedAt := time.Date(2026, 4, 19, 2, 0, 0, 0, time.UTC)
	requestedAt := time.Date(2026, 4, 19, 3, 0, 0, 0, time.UTC)
	ingestedAt := time.Date(2026, 4, 19, 4, 0, 0, 0, time.UTC)
	ignoredAt := time.Date(2026, 4, 19, 5, 0, 0, 0, time.UTC)
	createdAt := time.Date(2026, 4, 18, 10, 0, 0, 0, time.UTC)
	updatedAt := time.Date(2026, 4, 19, 10, 0, 0, 0, time.UTC)
	ignoreReasonCode := string(models.MediaGapIgnoreReasonManual)
	dispatchError := "重复添加"
	searchSnapshot := SearchSnapshot{
		Keyword:    "Show S01E02",
		Source:     "MoviePilot",
		SearchedAt: lastSearchedAt,
		Candidates: []SearchCandidate{{ID: "cand-1", Title: "Show S01E02"}},
	}
	dispatchSnapshot := DispatchSnapshot{
		RequestedAt: requestedAt,
		Candidate: SearchCandidate{
			ID:      "cand-2",
			Title:   "Show Pack",
			Payload: map[string]interface{}{"id": "cand-2"},
		},
	}
	searchPayload, err := json.Marshal(searchSnapshot)
	if err != nil {
		t.Fatalf("marshal search snapshot: %v", err)
	}
	dispatchPayload, err := json.Marshal(dispatchSnapshot)
	if err != nil {
		t.Fatalf("marshal dispatch snapshot: %v", err)
	}

	dto := toDTO(models.MediaGap{
		ID:                "gap-1",
		TmdbID:            "1399",
		EmbySeriesID:      "series-1",
		SeriesName:        "Show",
		Season:            1,
		Episode:           2,
		AirDate:           airDate,
		Status:            models.MediaGapStatusRequested,
		SearchSnapshot:    string(searchPayload),
		DispatchSnapshot:  string(dispatchPayload),
		LastDispatchError: &dispatchError,
		LastScannedAt:     &lastScannedAt,
		LastSearchedAt:    &lastSearchedAt,
		RequestedAt:       &requestedAt,
		IngestedAt:        &ingestedAt,
		IgnoredAt:         &ignoredAt,
		IgnoreReasonCode:  &ignoreReasonCode,
		IgnoreReason:      "人工忽略",
		CreatedAt:         createdAt,
		UpdatedAt:         updatedAt,
	})

	if dto.ID != "gap-1" || dto.TmdbID != "1399" || dto.EmbySeriesID != "series-1" {
		t.Fatalf("unexpected dto identifiers: %+v", dto)
	}
	if dto.AirDate != "2026-04-19" {
		t.Fatalf("expected UTC air date 2026-04-19, got %s", dto.AirDate)
	}
	if dto.SearchSnapshot == nil || len(dto.SearchSnapshot.Candidates) != 1 || dto.SearchSnapshot.Candidates[0].ID != "cand-1" {
		t.Fatalf("expected parsed search snapshot, got %+v", dto.SearchSnapshot)
	}
	if dto.DispatchSnapshot == nil || dto.DispatchSnapshot.Candidate.ID != "cand-2" {
		t.Fatalf("expected parsed dispatch snapshot, got %+v", dto.DispatchSnapshot)
	}
	if dto.LastDispatchError == nil || *dto.LastDispatchError != dispatchError {
		t.Fatalf("expected dispatch error %q, got %+v", dispatchError, dto.LastDispatchError)
	}
	if dto.IgnoreReasonCode == nil || *dto.IgnoreReasonCode != ignoreReasonCode {
		t.Fatalf("expected ignore reason code %q, got %+v", ignoreReasonCode, dto.IgnoreReasonCode)
	}
	if !dto.CreatedAt.Equal(createdAt) || !dto.UpdatedAt.Equal(updatedAt) {
		t.Fatalf("unexpected timestamps created=%s updated=%s", dto.CreatedAt, dto.UpdatedAt)
	}
}

func TestToDTOIgnoresInvalidSnapshotJSON(t *testing.T) {
	dto := toDTO(models.MediaGap{
		ID:               "gap-1",
		AirDate:          time.Date(2026, 4, 19, 0, 0, 0, 0, time.UTC),
		SearchSnapshot:   "{invalid",
		DispatchSnapshot: "{invalid",
	})

	if dto.SearchSnapshot != nil {
		t.Fatalf("expected invalid search snapshot to be ignored, got %+v", dto.SearchSnapshot)
	}
	if dto.DispatchSnapshot != nil {
		t.Fatalf("expected invalid dispatch snapshot to be ignored, got %+v", dto.DispatchSnapshot)
	}
}

func TestBuildDefaultSearchKeywordPadsSeasonAndEpisode(t *testing.T) {
	gap := models.MediaGap{
		SeriesName: "Show",
		Season:     3,
		Episode:    7,
	}

	if got := buildDefaultSearchKeyword(gap); got != "Show S03E07" {
		t.Fatalf("unexpected keyword: %s", got)
	}
}

func TestIsSearchableMediaGapStatus(t *testing.T) {
	cases := []struct {
		name   string
		status models.MediaGapStatus
		want   bool
	}{
		{name: "missing", status: models.MediaGapStatusMissing, want: true},
		{name: "searched", status: models.MediaGapStatusSearched, want: true},
		{name: "requested", status: models.MediaGapStatusRequested, want: true},
		{name: "ingested", status: models.MediaGapStatusIngested, want: false},
		{name: "ignored", status: models.MediaGapStatusIgnored, want: false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := isSearchableMediaGapStatus(tc.status); got != tc.want {
				t.Fatalf("expected %v, got %v", tc.want, got)
			}
		})
	}
}

func TestIsDispatchableMediaGapStatus(t *testing.T) {
	cases := []struct {
		name   string
		status models.MediaGapStatus
		want   bool
	}{
		{name: "missing", status: models.MediaGapStatusMissing, want: true},
		{name: "searched", status: models.MediaGapStatusSearched, want: true},
		{name: "requested", status: models.MediaGapStatusRequested, want: true},
		{name: "ingested", status: models.MediaGapStatusIngested, want: false},
		{name: "ignored", status: models.MediaGapStatusIgnored, want: false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := isDispatchableMediaGapStatus(tc.status); got != tc.want {
				t.Fatalf("expected %v, got %v", tc.want, got)
			}
		})
	}
}

func TestNormalizeSearchCandidatesMapsMoviePilotFields(t *testing.T) {
	normalized := normalizeSearchCandidates([]moviepilotint.GapSearchCandidate{
		{
			ID:          "cand-1",
			Title:       "Show S01E02",
			Description: "first",
			PublishDate: "2026-04-19 20:00:00",
			Site:        "MTeam",
			Size:        2048,
			Seeders:     23,
			IsPack:      true,
			MatchMode:   "episode",
			Tags:        []string{"4K", "HDR"},
			Payload: map[string]interface{}{
				"id":    "cand-1",
				"title": "Show S01E02",
			},
		},
	})

	if len(normalized) != 1 {
		t.Fatalf("expected 1 normalized candidate, got %d", len(normalized))
	}
	got := normalized[0]
	if got.ID != "cand-1" || got.Title != "Show S01E02" || got.Description != "first" {
		t.Fatalf("unexpected normalized candidate basics: %+v", got)
	}
	if got.PublishDate != "2026-04-19 20:00:00" || got.Site != "MTeam" {
		t.Fatalf("unexpected normalized source fields: %+v", got)
	}
	if got.Size != 2048 || got.Seeders != 23 || !got.IsPack || got.MatchMode != "episode" {
		t.Fatalf("unexpected normalized metrics: %+v", got)
	}
	if len(got.Tags) != 2 || got.Tags[0] != "4K" || got.Payload["id"] != "cand-1" {
		t.Fatalf("unexpected normalized tags or payload: %+v", got)
	}
}

func TestNormalizeSearchCandidatesReturnsEmptyNonNilSlice(t *testing.T) {
	normalized := normalizeSearchCandidates(nil)

	if normalized == nil {
		t.Fatal("expected empty non-nil candidates slice")
	}
	if len(normalized) != 0 {
		t.Fatalf("expected empty candidates, got %d", len(normalized))
	}
}

func TestBuildSearchSnapshotIncludesMoviePilotResponse(t *testing.T) {
	searchedAt := time.Date(2026, 4, 18, 12, 30, 0, 0, time.UTC)
	gap := models.MediaGap{
		SeriesName: "斗破苍穹",
		Season:     5,
		Episode:    7,
	}
	resp := &moviepilotint.GapSearchResponse{
		Query:         "斗破苍穹 S05E07",
		FallbackQuery: "斗破苍穹 S05",
		MatchMode:     "season",
		Candidates: []moviepilotint.GapSearchCandidate{
			{
				ID:          "cand-1",
				Title:       "斗破苍穹 S05 4K",
				Description: "首条候选",
				PublishDate: "2026-04-18 20:00:00",
				Site:        "MTeam",
				Size:        1024,
				Seeders:     12,
				IsPack:      true,
				MatchMode:   "season",
				Tags:        []string{"4K", "HDR"},
				Payload: map[string]interface{}{
					"id": "cand-1",
				},
			},
		},
	}

	snapshot := buildSearchSnapshot(gap, searchedAt, resp)
	if snapshot.Keyword != "斗破苍穹 S05E07" {
		t.Fatalf("unexpected keyword: %s", snapshot.Keyword)
	}
	if snapshot.Query != resp.Query {
		t.Fatalf("expected query %s, got %s", resp.Query, snapshot.Query)
	}
	if snapshot.FallbackQuery != resp.FallbackQuery {
		t.Fatalf("expected fallback query %s, got %s", resp.FallbackQuery, snapshot.FallbackQuery)
	}
	if snapshot.MatchMode != "season" {
		t.Fatalf("expected season match mode, got %s", snapshot.MatchMode)
	}
	if snapshot.Source != "MoviePilot" {
		t.Fatalf("expected source MoviePilot, got %s", snapshot.Source)
	}
	if len(snapshot.Candidates) != 1 {
		t.Fatalf("expected 1 candidate, got %d", len(snapshot.Candidates))
	}
	if snapshot.Candidates[0].ID != "cand-1" {
		t.Fatalf("expected candidate id cand-1, got %s", snapshot.Candidates[0].ID)
	}
	if snapshot.Candidates[0].Description != "首条候选" {
		t.Fatalf("unexpected description: %s", snapshot.Candidates[0].Description)
	}
	if snapshot.Candidates[0].PublishDate != "2026-04-18 20:00:00" {
		t.Fatalf("unexpected publish date: %s", snapshot.Candidates[0].PublishDate)
	}
}

func TestBuildSearchSnapshotHandlesNilResponse(t *testing.T) {
	gap := models.MediaGap{
		SeriesName: "完美世界",
		Season:     1,
		Episode:    3,
	}
	searchedAt := time.Date(2026, 4, 18, 10, 0, 0, 0, time.UTC)

	snapshot := buildSearchSnapshot(gap, searchedAt, nil)
	if snapshot.Keyword != "完美世界 S01E03" {
		t.Fatalf("unexpected keyword: %s", snapshot.Keyword)
	}
	if snapshot.Source != "MoviePilot" {
		t.Fatalf("expected source MoviePilot, got %s", snapshot.Source)
	}
	if len(snapshot.Candidates) != 0 {
		t.Fatalf("expected empty candidates, got %d", len(snapshot.Candidates))
	}
}

func TestNormalizeGroupedSortMode(t *testing.T) {
	cases := []struct {
		name string
		sort string
		want GroupedSortMode
	}{
		{name: "updated", sort: " updated ", want: GroupedSortUpdated},
		{name: "requested", sort: "requested", want: GroupedSortRequested},
		{name: "name", sort: "name", want: GroupedSortName},
		{name: "missing", sort: "missing", want: GroupedSortMissing},
		{name: "blank", sort: "  ", want: GroupedSortMissing},
		{name: "unknown", sort: "latest", want: GroupedSortMissing},
		{name: "case sensitive fallback", sort: "UPDATED", want: GroupedSortMissing},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := normalizeGroupedSortMode(tc.sort); got != tc.want {
				t.Fatalf("expected %s, got %s", tc.want, got)
			}
		})
	}
}

func TestBuildGroupedSeriesKeyPrefersTmdbIDThenEmbySeriesIDThenName(t *testing.T) {
	cases := []struct {
		name string
		gap  models.MediaGap
		want string
	}{
		{
			name: "tmdb id",
			gap:  models.MediaGap{ID: "gap-1", TmdbID: " 1399 ", EmbySeriesID: "series-1", SeriesName: "Show"},
			want: " 1399 ",
		},
		{
			name: "emby series id",
			gap:  models.MediaGap{ID: "gap-1", EmbySeriesID: " series-1 ", SeriesName: "Show"},
			want: " series-1 ",
		},
		{
			name: "series name",
			gap:  models.MediaGap{ID: "gap-1", SeriesName: " Show "},
			want: " Show ",
		},
		{
			name: "gap id",
			gap:  models.MediaGap{ID: "gap-1"},
			want: "gap-1",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := buildGroupedSeriesKey(tc.gap); got != tc.want {
				t.Fatalf("expected key %q, got %q", tc.want, got)
			}
		})
	}
}

func TestSortMediaGapItemsOrdersByStatusSeasonAndEpisode(t *testing.T) {
	items := []models.MediaGap{
		{ID: "ignored", Status: models.MediaGapStatusIgnored, Season: 1, Episode: 1},
		{ID: "requested", Status: models.MediaGapStatusRequested, Season: 1, Episode: 2},
		{ID: "missing-late-season", Status: models.MediaGapStatusMissing, Season: 2, Episode: 1},
		{ID: "missing-late-episode", Status: models.MediaGapStatusMissing, Season: 1, Episode: 3},
		{ID: "missing-first", Status: models.MediaGapStatusMissing, Season: 1, Episode: 1},
		{ID: "unknown", Status: models.MediaGapStatus("UNKNOWN"), Season: 1, Episode: 1},
	}

	sortMediaGapItems(items)

	wantOrder := []string{
		"missing-first",
		"missing-late-episode",
		"missing-late-season",
		"requested",
		"ignored",
		"unknown",
	}
	for i, want := range wantOrder {
		if items[i].ID != want {
			t.Fatalf("expected item %d to be %s, got %s; full order=%+v", i, want, items[i].ID, items)
		}
	}
}

func TestBuildGroupedSeriesAggregatesAndSortsByMissing(t *testing.T) {
	now := time.Date(2026, 4, 19, 12, 0, 0, 0, time.UTC)
	items := []models.MediaGap{
		{
			ID:         "gap-1",
			TmdbID:     "100",
			SeriesName: "B Show",
			Season:     1,
			Episode:    2,
			Status:     models.MediaGapStatusMissing,
			UpdatedAt:  now.Add(-time.Hour),
		},
		{
			ID:         "gap-2",
			TmdbID:     "100",
			SeriesName: "B Show",
			Season:     1,
			Episode:    1,
			Status:     models.MediaGapStatusRequested,
			UpdatedAt:  now,
		},
		{
			ID:         "gap-3",
			TmdbID:     "200",
			SeriesName: "A Show",
			Season:     2,
			Episode:    1,
			Status:     models.MediaGapStatusMissing,
			UpdatedAt:  now.Add(-2 * time.Hour),
		},
		{
			ID:         "gap-4",
			TmdbID:     "200",
			SeriesName: "A Show",
			Season:     2,
			Episode:    2,
			Status:     models.MediaGapStatusMissing,
			UpdatedAt:  now.Add(-90 * time.Minute),
		},
		{
			ID:         "gap-5",
			TmdbID:     "200",
			SeriesName: "A Show",
			Season:     1,
			Episode:    3,
			Status:     models.MediaGapStatusIgnored,
			UpdatedAt:  now.Add(-30 * time.Minute),
		},
	}

	grouped, summary := buildGroupedSeries(items, "missing")
	if len(grouped) != 2 {
		t.Fatalf("expected 2 grouped series, got %d", len(grouped))
	}
	if grouped[0].SeriesName != "A Show" {
		t.Fatalf("expected A Show first, got %s", grouped[0].SeriesName)
	}
	if grouped[0].MissingCount != 2 {
		t.Fatalf("expected missing count 2, got %d", grouped[0].MissingCount)
	}
	if len(grouped[0].Seasons) != 2 {
		t.Fatalf("expected 2 seasons, got %d", len(grouped[0].Seasons))
	}
	if grouped[1].RequestedCount != 1 {
		t.Fatalf("expected requested count 1, got %d", grouped[1].RequestedCount)
	}
	if summary.MissingCount != 3 || summary.RequestedCount != 1 || summary.IgnoredCount != 1 {
		t.Fatalf("unexpected summary: %+v", summary)
	}
}
