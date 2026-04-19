package mediagap

import (
	"testing"
	"time"

	moviepilotint "github.com/konghang/ember/backend/internal/integrations/moviepilot"
	"github.com/konghang/ember/backend/internal/models"
)

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
