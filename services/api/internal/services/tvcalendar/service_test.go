package tvcalendar

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/konghang/ember/backend/internal/common/tmdbcache"
	embyint "github.com/konghang/ember/backend/internal/integrations/emby"
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

func TestFetchTMDBJSONDeduplicatesInflightRequests(t *testing.T) {
	var requestCount atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount.Add(1)
		time.Sleep(50 * time.Millisecond)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":200}`))
	}))
	defer server.Close()

	service := &TVCalendarService{
		httpClient:        server.Client(),
		tmdbCache:         tmdbcache.NewStore(),
		readyEpisodeCache: make(map[string]readyEpisodeCacheEntry),
	}

	var wg sync.WaitGroup
	start := make(chan struct{})
	run := func() {
		defer wg.Done()
		<-start
		var out map[string]any
		if err := service.fetchTMDBJSON(context.Background(), "detail:200", server.URL, time.Minute, true, &out); err != nil {
			t.Errorf("fetchTMDBJSON returned error: %v", err)
			return
		}
		if got := int(out["id"].(float64)); got != 200 {
			t.Errorf("expected id 200, got %d", got)
		}
	}

	wg.Add(2)
	go run()
	go run()
	close(start)
	wg.Wait()

	if got := requestCount.Load(); got != 1 {
		t.Fatalf("expected exactly 1 upstream request, got %d", got)
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

func TestTVCalendarDateParsingHelpers(t *testing.T) {
	date, err := ParseTVCalendarDate(" 2026-03-14 ")
	if err != nil {
		t.Fatalf("expected valid date, got %v", err)
	}
	want := time.Date(2026, 3, 14, 0, 0, 0, 0, time.UTC)
	if !date.Equal(want) {
		t.Fatalf("expected normalized date %s, got %s", want, date)
	}

	if _, err := ParseTVCalendarDate(""); !errors.Is(err, ErrTVCalendarInvalidDate) {
		t.Fatalf("expected ErrTVCalendarInvalidDate for empty date, got %v", err)
	}
	if _, err := ParseTVCalendarDate("2026/03/14"); !errors.Is(err, ErrTVCalendarInvalidDate) {
		t.Fatalf("expected ErrTVCalendarInvalidDate for malformed date, got %v", err)
	}

	parsed, err := parseDateOnly("2026-03-15")
	if err != nil {
		t.Fatalf("expected date-only parse to succeed, got %v", err)
	}
	if !parsed.Equal(time.Date(2026, 3, 15, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("unexpected date-only parse result: %s", parsed)
	}
	if _, err := parseDateOnly("2026-3-15"); err == nil {
		t.Fatalf("expected date-only parser to reject non-ISO date")
	}
}

func TestTVCalendarWeekOffsetHelpers(t *testing.T) {
	offset, err := ParseTVCalendarWeekOffset(" ")
	if err != nil {
		t.Fatalf("expected default week offset, got %v", err)
	}
	if offset != tvCalendarDefaultOffset {
		t.Fatalf("expected default offset %d, got %d", tvCalendarDefaultOffset, offset)
	}

	offset, err = ParseTVCalendarWeekOffset(" -1 ")
	if err != nil {
		t.Fatalf("expected valid offset, got %v", err)
	}
	if offset != -1 {
		t.Fatalf("expected offset -1, got %d", offset)
	}
	if _, err := ParseTVCalendarWeekOffset("2"); !errors.Is(err, ErrTVCalendarInvalidWeekOffset) {
		t.Fatalf("expected invalid week offset error, got %v", err)
	}
	if _, err := ParseTVCalendarWeekOffset("next"); !errors.Is(err, ErrTVCalendarInvalidWeekOffset) {
		t.Fatalf("expected invalid week offset error for non-number, got %v", err)
	}

	defaults, err := normalizeWeekOffsets(nil)
	if err != nil {
		t.Fatalf("expected default offsets, got %v", err)
	}
	assertIntSlice(t, defaults, []int{0, 1})

	normalized, err := normalizeWeekOffsets([]int{1, -1, 1, 0})
	if err != nil {
		t.Fatalf("expected normalized offsets, got %v", err)
	}
	assertIntSlice(t, normalized, []int{-1, 0, 1})

	if _, err := normalizeWeekOffsets([]int{-2}); !errors.Is(err, ErrTVCalendarInvalidWeekOffset) {
		t.Fatalf("expected invalid week offset error, got %v", err)
	}
}

func TestTVCalendarWeekStartHelpersUseLocation(t *testing.T) {
	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		t.Fatalf("load location: %v", err)
	}

	now := time.Date(2026, 3, 15, 16, 30, 0, 0, time.UTC)
	currentDate := currentCalendarDateInLocation(now, loc)
	if !currentDate.Equal(time.Date(2026, 3, 16, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("expected Shanghai calendar date 2026-03-16, got %s", currentDate)
	}

	weekStart := normalizeTVCalendarWeekStartInLocation(currentDate, loc)
	if !weekStart.Equal(time.Date(2026, 3, 16, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("expected local Monday week start, got %s", weekStart)
	}

	starts := weekStartsFromOffsets([]int{-1, 0, 1}, currentDate, loc)
	assertTimeSlice(t, starts, []time.Time{
		time.Date(2026, 3, 9, 0, 0, 0, 0, time.UTC),
		time.Date(2026, 3, 16, 0, 0, 0, 0, time.UTC),
		time.Date(2026, 3, 23, 0, 0, 0, 0, time.UTC),
	})

	cutoff := activeSourceCutoff(time.Date(2026, 4, 1, 12, 0, 0, 0, time.FixedZone("UTC+8", 8*60*60)))
	wantCutoff := time.Date(2026, 4, 1, 4, 0, 0, 0, time.UTC).AddDate(0, 0, -tvCalendarActiveSourceWindowDays)
	if !cutoff.Equal(wantCutoff) {
		t.Fatalf("expected UTC active source cutoff %s, got %s", wantCutoff, cutoff)
	}
}

func TestTVCalendarLocalMediaHelpers(t *testing.T) {
	if !isValidTVCalendarStatus(models.TVCalendarStatusToday) || isValidTVCalendarStatus("done") {
		t.Fatalf("unexpected status validation result")
	}
	if !isValidTVCalendarWeekOffset(-1) || isValidTVCalendarWeekOffset(2) {
		t.Fatalf("unexpected week offset validation result")
	}

	if got := tvCalendarStatusSortWeight(models.TVCalendarStatusReady); got != 0 {
		t.Fatalf("expected ready sort weight 0, got %d", got)
	}
	if got := tvCalendarStatusSortWeight(models.TVCalendarStatusUpcoming); got != 3 {
		t.Fatalf("expected upcoming/default sort weight 3, got %d", got)
	}

	if got := buildTMDBPosterURL(" /poster.jpg "); got != tmdbImageBaseURL+"/poster.jpg" {
		t.Fatalf("unexpected poster url: %s", got)
	}
	if got := buildTMDBPosterURL(" "); got != "" {
		t.Fatalf("expected empty poster path to stay empty, got %q", got)
	}

	providerID := extractProviderID(map[string]string{" TmDb ": " 1399 "}, "tmdb")
	if providerID != "1399" {
		t.Fatalf("expected trimmed provider id 1399, got %q", providerID)
	}
	if extractProviderID(nil, "tmdb") != "" {
		t.Fatalf("expected missing provider ids to return empty string")
	}

	if !hasPhysicalEpisodeMedia(embyEpisodeItem{Path: " /media/s01e01.mkv "}) {
		t.Fatalf("expected item with path to be physical")
	}
	if !hasPhysicalEpisodeMedia(embyEpisodeItem{MediaSources: []embyint.EmbyMediaSource{{}}}) {
		t.Fatalf("expected item with media sources to be physical")
	}
	if hasPhysicalEpisodeMedia(embyEpisodeItem{LocationType: " virtual ", Path: "/media/s01e01.mkv"}) {
		t.Fatalf("expected virtual item to be ignored")
	}
	if hasPhysicalEpisodeMedia(embyEpisodeItem{IsMissing: true, Path: "/media/s01e01.mkv"}) {
		t.Fatalf("expected missing item to be ignored")
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

func assertIntSlice(t *testing.T, got, want []int) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("expected %v, got %v", want, got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("expected %v, got %v", want, got)
		}
	}
}

func assertTimeSlice(t *testing.T, got, want []time.Time) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("expected %v, got %v", want, got)
	}
	for i := range want {
		if !got[i].Equal(want[i]) {
			t.Fatalf("expected %v, got %v", want, got)
		}
	}
}
