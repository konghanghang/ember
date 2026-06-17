package handlers

import (
	"testing"
	"time"

	"github.com/konghang/ember/backend/internal/models"
	playbackpkg "github.com/konghang/ember/backend/internal/services/playback"
)

func TestDateRangeByPeriodBuildsDailyRangeInLocation(t *testing.T) {
	tz := time.FixedZone("UTC+8", 8*60*60)
	date := time.Date(2026, 6, 16, 18, 30, 0, 0, time.UTC)

	start, end, err := dateRangeByPeriod(tz, models.RankingDaily, date)
	if err != nil {
		t.Fatalf("expected daily range, got %v", err)
	}

	wantStart := time.Date(2026, 6, 17, 0, 0, 0, 0, tz)
	wantEnd := time.Date(2026, 6, 18, 0, 0, 0, 0, tz)
	if !start.Equal(wantStart) || !end.Equal(wantEnd) {
		t.Fatalf("expected %s~%s, got %s~%s", wantStart, wantEnd, start, end)
	}
}

func TestDateRangeByPeriodBuildsMondayBasedWeeklyRange(t *testing.T) {
	tz := time.FixedZone("UTC+8", 8*60*60)
	cases := []struct {
		name      string
		date      time.Time
		wantStart time.Time
		wantEnd   time.Time
	}{
		{
			name:      "wednesday",
			date:      time.Date(2026, 6, 17, 12, 0, 0, 0, tz),
			wantStart: time.Date(2026, 6, 15, 0, 0, 0, 0, tz),
			wantEnd:   time.Date(2026, 6, 22, 0, 0, 0, 0, tz),
		},
		{
			name:      "sunday belongs to previous monday",
			date:      time.Date(2026, 6, 21, 12, 0, 0, 0, tz),
			wantStart: time.Date(2026, 6, 15, 0, 0, 0, 0, tz),
			wantEnd:   time.Date(2026, 6, 22, 0, 0, 0, 0, tz),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			start, end, err := dateRangeByPeriod(tz, models.RankingWeekly, tc.date)
			if err != nil {
				t.Fatalf("expected weekly range, got %v", err)
			}
			if !start.Equal(tc.wantStart) || !end.Equal(tc.wantEnd) {
				t.Fatalf("expected %s~%s, got %s~%s", tc.wantStart, tc.wantEnd, start, end)
			}
		})
	}
}

func TestDateRangeByPeriodRejectsUnknownPeriod(t *testing.T) {
	tz := time.UTC

	if _, _, err := dateRangeByPeriod(tz, models.RankingPeriod("monthly"), time.Now()); err == nil {
		t.Fatal("expected unknown ranking period to fail")
	}
}

func TestBuildRankingResponseHandlesNilResult(t *testing.T) {
	resp := buildRankingResponse(models.RankingWeekly, nil, time.UTC)

	if resp.Period != "weekly" {
		t.Fatalf("expected weekly period, got %s", resp.Period)
	}
	if resp.Movies == nil || len(resp.Movies) != 0 {
		t.Fatalf("expected empty non-nil movies, got %+v", resp.Movies)
	}
	if resp.Episodes == nil || len(resp.Episodes) != 0 {
		t.Fatalf("expected empty non-nil episodes, got %+v", resp.Episodes)
	}
	if resp.SnapshotAt != "" || resp.PeriodStart != "" || resp.CutoffAt != "" {
		t.Fatalf("expected blank timestamps for nil result, got %+v", resp)
	}
}

func TestBuildRankingResponseFormatsResultAndItems(t *testing.T) {
	tz := time.FixedZone("UTC+8", 8*60*60)
	result := &playbackpkg.RankingResult{
		Period:      models.RankingDaily,
		BatchID:     "batch_1",
		SnapshotAt:  time.Date(2026, 6, 17, 8, 30, 0, 0, time.UTC),
		PeriodStart: time.Date(2026, 6, 16, 16, 0, 0, 0, time.UTC),
		PeriodEnd:   time.Date(2026, 6, 17, 16, 0, 0, 0, time.UTC),
		Movies: []playbackpkg.RankingResultItem{
			{Rank: 1, ItemKey: "movie_1", ItemName: "Movie", PlayCount: 3, Duration: 7200},
		},
		Episodes: []playbackpkg.RankingResultItem{
			{Rank: 1, ItemKey: "series_1", ItemName: "Show", PlayCount: 5, Duration: 3600},
		},
	}

	resp := buildRankingResponse(models.RankingWeekly, result, tz)
	if resp.Period != "daily" || resp.BatchID != "batch_1" {
		t.Fatalf("unexpected response basics: %+v", resp)
	}
	if resp.SnapshotAt != "2026-06-17T08:30:00Z" {
		t.Fatalf("unexpected snapshot time: %s", resp.SnapshotAt)
	}
	if resp.PeriodStart != "2026-06-17" || resp.PeriodEnd != "2026-06-18" || resp.CutoffAt != "00:00" {
		t.Fatalf("unexpected localized range: %+v", resp)
	}
	if len(resp.Movies) != 1 || resp.Movies[0].ItemKey != "movie_1" || resp.Movies[0].Duration != 7200 {
		t.Fatalf("unexpected movie ranking item: %+v", resp.Movies)
	}
	if len(resp.Episodes) != 1 || resp.Episodes[0].ItemName != "Show" || resp.Episodes[0].PlayCount != 5 {
		t.Fatalf("unexpected episode ranking item: %+v", resp.Episodes)
	}
}

func TestRankingTimeFormatHelpersReturnBlankForZeroValue(t *testing.T) {
	if got := formatRFC3339(time.Time{}); got != "" {
		t.Fatalf("expected zero RFC3339 time to be blank, got %q", got)
	}
	if got := formatDateInLocation(time.Time{}, time.UTC); got != "" {
		t.Fatalf("expected zero date to be blank, got %q", got)
	}
	if got := formatClockInLocation(time.Time{}, time.UTC); got != "" {
		t.Fatalf("expected zero clock to be blank, got %q", got)
	}
}
