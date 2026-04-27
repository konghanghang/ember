package tvcalendar

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/konghang/ember/backend/internal/models"
)

func TestPickTargetSeasonNumbers(t *testing.T) {
	detail := &tmdbTVDetailResponse{
		NumberOfSeasons: 5,
		Seasons: []struct {
			SeasonNumber int `json:"season_number"`
		}{
			{SeasonNumber: 1},
			{SeasonNumber: 2},
			{SeasonNumber: 3},
			{SeasonNumber: 4},
			{SeasonNumber: 5},
		},
		LastEpisodeToAir: &tmdbEpisodeReference{
			SeasonNumber: 4,
		},
		NextEpisodeToAir: &tmdbEpisodeReference{
			SeasonNumber: 5,
		},
	}

	got := pickTargetSeasonNumbers(detail)
	if len(got) != 2 {
		t.Fatalf("expected 2 seasons, got %d", len(got))
	}
	if got[0] != 4 || got[1] != 5 {
		t.Fatalf("expected seasons [4 5], got %v", got)
	}
}

func TestPickTargetSeasonNumbersIncludesRecentAndCurrentRelatedSeasons(t *testing.T) {
	detail := &tmdbTVDetailResponse{
		NumberOfSeasons: 6,
		Seasons: []struct {
			SeasonNumber int `json:"season_number"`
		}{
			{SeasonNumber: 1},
			{SeasonNumber: 2},
			{SeasonNumber: 3},
			{SeasonNumber: 4},
			{SeasonNumber: 5},
			{SeasonNumber: 6},
		},
		LastEpisodeToAir: &tmdbEpisodeReference{
			SeasonNumber: 2,
		},
		NextEpisodeToAir: &tmdbEpisodeReference{
			SeasonNumber: 3,
		},
	}

	got := pickTargetSeasonNumbers(detail)
	want := []int{2, 3, 5, 6}
	if len(got) != len(want) {
		t.Fatalf("expected %d seasons, got %d (%v)", len(want), len(got), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("expected seasons %v, got %v", want, got)
		}
	}
}

func TestPickTargetSeasonNumbersFallsBackToLastTwoByCount(t *testing.T) {
	detail := &tmdbTVDetailResponse{
		NumberOfSeasons: 5,
	}

	got := pickTargetSeasonNumbers(detail)
	want := []int{4, 5}
	if len(got) != len(want) {
		t.Fatalf("expected %d seasons, got %d (%v)", len(want), len(got), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("expected seasons %v, got %v", want, got)
		}
	}
}

func TestBuildWeeklyCalendarAggregatesConsecutiveEpisodes(t *testing.T) {
	service := NewTVCalendarService()
	start := time.Date(2026, 3, 9, 0, 0, 0, 0, time.UTC)
	end := start.AddDate(0, 0, 6)

	items := []models.TVCalendarItem{
		{
			TmdbID:      "1399",
			SeriesID:    "series_1",
			Season:      3,
			Episode:     1,
			AirDate:     start,
			EpisodeName: "Winter",
			Overview:    "Episode 1",
			Status:      models.TVCalendarStatusMissing,
		},
		{
			TmdbID:      "1399",
			SeriesID:    "series_1",
			Season:      3,
			Episode:     2,
			AirDate:     start,
			EpisodeName: "Spring",
			Overview:    "Episode 2",
			Status:      models.TVCalendarStatusMissing,
		},
	}

	sourceMap := map[string]models.TVCalendarSource{
		"1399": {
			TmdbID:    "1399",
			ShowName:  "Game of Thrones",
			PosterURL: "https://example.com/poster.jpg",
		},
	}

	got := service.buildWeeklyCalendar(items, sourceMap, start, end, time.UTC)
	if len(got.Days) != 7 {
		t.Fatalf("expected 7 days, got %d", len(got.Days))
	}
	if len(got.Days[0].Items) != 1 {
		t.Fatalf("expected 1 aggregated item, got %d", len(got.Days[0].Items))
	}

	episode := got.Days[0].Items[0]
	if episode.Episode != "1-2" {
		t.Fatalf("expected episode range 1-2, got %s", episode.Episode)
	}
	if episode.EpisodeName != "" {
		t.Fatalf("expected aggregated episode name to be blank, got %q", episode.EpisodeName)
	}
}

func TestParseTVCalendarWeekDateUsesWeekStart(t *testing.T) {
	got, err := ParseTVCalendarWeekDate("2026-03-12", "")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	expected := time.Date(2026, 3, 9, 0, 0, 0, 0, time.UTC)
	if !got.Equal(expected) {
		t.Fatalf("expected week start %s, got %s", expected, got)
	}
}

func TestDefaultTVCalendarWeekOffsetsIncludesCurrentAndNextWeek(t *testing.T) {
	got := DefaultTVCalendarWeekOffsets()
	if len(got) != 2 || got[0] != 0 || got[1] != 1 {
		t.Fatalf("expected default week offsets [0 1], got %v", got)
	}
}

func TestIsDateWithinTVCalendarWeek(t *testing.T) {
	weekStart := time.Date(2026, 3, 9, 0, 0, 0, 0, time.UTC)

	tests := []struct {
		name string
		date time.Time
		want bool
	}{
		{
			name: "week start included",
			date: weekStart,
			want: true,
		},
		{
			name: "week end included",
			date: weekStart.AddDate(0, 0, 6),
			want: true,
		},
		{
			name: "previous day excluded",
			date: weekStart.AddDate(0, 0, -1),
			want: false,
		},
		{
			name: "next week excluded",
			date: weekStart.AddDate(0, 0, 7),
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isDateWithinTVCalendarWeek(tt.date, weekStart); got != tt.want {
				t.Fatalf("expected %v, got %v", tt.want, got)
			}
		})
	}
}

func TestParseEmbyDateTimeSupportsRFC3339Nano(t *testing.T) {
	got, ok := parseEmbyDateTime("2026-03-14T09:30:00.0000000Z")
	if !ok {
		t.Fatalf("expected timestamp to parse")
	}

	want := time.Date(2026, 3, 14, 9, 30, 0, 0, time.UTC)
	if !got.Equal(want) {
		t.Fatalf("unexpected parsed time, want=%s got=%s", want.Format(time.RFC3339), got.Format(time.RFC3339))
	}
}

func TestResolveCalendarItemStatus(t *testing.T) {
	now := time.Date(2026, 3, 26, 9, 0, 0, 0, time.UTC)
	loc := time.UTC

	tests := []struct {
		name string
		item models.TVCalendarItem
		want string
	}{
		{
			name: "ready stays ready even when air date is in the future",
			item: models.TVCalendarItem{
				AirDate: now.AddDate(0, 0, 2),
				Status:  models.TVCalendarStatusReady,
			},
			want: models.TVCalendarStatusReady,
		},
		{
			name: "future non-ready becomes upcoming",
			item: models.TVCalendarItem{
				AirDate: now.AddDate(0, 0, 2),
				Status:  models.TVCalendarStatusMissing,
			},
			want: models.TVCalendarStatusUpcoming,
		},
		{
			name: "same day non-ready becomes today",
			item: models.TVCalendarItem{
				AirDate: now,
				Status:  models.TVCalendarStatusUpcoming,
			},
			want: models.TVCalendarStatusToday,
		},
		{
			name: "past non-ready becomes missing",
			item: models.TVCalendarItem{
				AirDate: now.AddDate(0, 0, -1),
				Status:  models.TVCalendarStatusUpcoming,
			},
			want: models.TVCalendarStatusMissing,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := resolveCalendarItemStatus(tt.item, now, loc)
			if got != tt.want {
				t.Fatalf("expected %s, got %s", tt.want, got)
			}
		})
	}
}

func TestDeriveStatusByAirDateUsesConfiguredTimezone(t *testing.T) {
	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		t.Fatalf("load location: %v", err)
	}

	now := time.Date(2026, 3, 26, 16, 30, 0, 0, time.UTC)
	airDate := time.Date(2026, 3, 27, 0, 0, 0, 0, time.UTC)

	if got := deriveStatusByAirDate(airDate, now, loc); got != models.TVCalendarStatusToday {
		t.Fatalf("expected air date to be treated as today in Asia/Shanghai, got %s", got)
	}
}

func TestApplyReadyEpisodeCorrectionsOnlyTouchesCurrentWeekCandidates(t *testing.T) {
	currentWeekStart := time.Date(2026, 3, 23, 0, 0, 0, 0, time.UTC)
	items := []models.TVCalendarItem{
		{
			TmdbID:   "1399",
			SeriesID: "series_1",
			Season:   3,
			Episode:  1,
			AirDate:  currentWeekStart.AddDate(0, 0, 2),
			Status:   models.TVCalendarStatusMissing,
		},
		{
			TmdbID:   "1399",
			SeriesID: "series_1",
			Season:   3,
			Episode:  2,
			AirDate:  currentWeekStart.AddDate(0, 0, 9),
			Status:   models.TVCalendarStatusMissing,
		},
	}

	readyEpisodesBySeries := map[string]map[string]embyEpisodeItem{
		"series_1": {
			buildEpisodeKey(3, 1): {ID: "emby_ep_1", SeriesID: "series_1"},
			buildEpisodeKey(3, 2): {ID: "emby_ep_2", SeriesID: "series_1"},
		},
	}

	corrected, corrections := applyReadyEpisodeCorrections(items, readyEpisodesBySeries, map[string]string{}, currentWeekStart)
	if len(corrections) != 1 {
		t.Fatalf("expected 1 correction, got %d", len(corrections))
	}
	if corrected[0].Status != models.TVCalendarStatusReady {
		t.Fatalf("expected current week item to become ready, got %s", corrected[0].Status)
	}
	if corrected[0].EmbyItemID != "emby_ep_1" {
		t.Fatalf("expected corrected item to carry emby item id, got %q", corrected[0].EmbyItemID)
	}
	if corrected[1].Status != models.TVCalendarStatusMissing {
		t.Fatalf("expected next week item to stay missing, got %s", corrected[1].Status)
	}
}

func TestShouldSyncSourceIncrementally(t *testing.T) {
	cutoff := time.Date(2026, 2, 12, 0, 0, 0, 0, time.UTC)
	recent := cutoff.Add(24 * time.Hour)
	old := cutoff.Add(-24 * time.Hour)
	synced := cutoff.Add(-7 * 24 * time.Hour)

	if !shouldSyncSourceIncrementally(models.TVCalendarSource{LastEpisodeIngestedAt: &recent}, cutoff) {
		t.Fatalf("expected recent active source to be selected")
	}

	if shouldSyncSourceIncrementally(models.TVCalendarSource{LastEpisodeIngestedAt: &old}, cutoff) {
		t.Fatalf("expected stale source to be skipped")
	}

	if !shouldSyncSourceIncrementally(models.TVCalendarSource{}, cutoff) {
		t.Fatalf("expected never-synced source to be selected")
	}

	if shouldSyncSourceIncrementally(models.TVCalendarSource{LastSyncedAt: &synced}, cutoff) {
		t.Fatalf("expected synced source without recent ingest to be skipped")
	}
}

func TestRunTVCalendarSyncOnceDeduplicatesConcurrentCalls(t *testing.T) {
	var executed int32
	start := make(chan struct{})
	var wg sync.WaitGroup
	results := make([]int, 2)

	run := func(index int) {
		defer wg.Done()
		<-start
		count, err := runTVCalendarSyncOnce("test-sync-key", func() (int, error) {
			atomic.AddInt32(&executed, 1)
			time.Sleep(50 * time.Millisecond)
			return 7, nil
		})
		if err != nil {
			t.Errorf("unexpected error: %v", err)
			return
		}
		results[index] = count
	}

	wg.Add(2)
	go run(0)
	go run(1)
	close(start)
	wg.Wait()

	if got := atomic.LoadInt32(&executed); got != 1 {
		t.Fatalf("expected sync function to execute once, got %d", got)
	}
	if results[0] != 7 || results[1] != 7 {
		t.Fatalf("expected both callers to receive shared result 7, got %v", results)
	}
}
