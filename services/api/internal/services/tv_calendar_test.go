package services

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

	got := service.buildWeeklyCalendar(items, sourceMap, start, end)
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
