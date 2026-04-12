package playback

import (
	"context"
	"testing"
)

func TestNormalizePlaybackProfileListQueryDefaultsToToday(t *testing.T) {
	t.Setenv("CRON_TIMEZONE", "Asia/Shanghai")

	service := &UserPlaybackProfileService{}
	query, err := service.normalizePlaybackProfileListQuery(context.Background(), PlaybackProfileListQuery{
		Page:     1,
		PageSize: 20,
	})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if query.rangeValue != "today" {
		t.Fatalf("expected today range, got %s", query.rangeValue)
	}
	if query.startAt == nil || query.endAt == nil {
		t.Fatal("expected non-nil start/end")
	}
	if query.startAt.Hour() != 0 || query.startAt.Minute() != 0 || query.startAt.Second() != 0 {
		t.Fatalf("expected startAt at beginning of day, got %s", query.startAt.Format(playbackDateTimeFormat))
	}
}

func TestNormalizePlaybackProfileListQuerySupportsCustomDates(t *testing.T) {
	t.Setenv("CRON_TIMEZONE", "Asia/Shanghai")

	service := &UserPlaybackProfileService{}
	query, err := service.normalizePlaybackProfileListQuery(context.Background(), PlaybackProfileListQuery{
		StartDate: "2026-01-01 08:30:00",
		EndDate:   "2026-01-15 20:45:00",
		Page:      1,
		PageSize:  20,
	})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if query.rangeValue != "custom" {
		t.Fatalf("expected custom range, got %s", query.rangeValue)
	}
	if query.startAt == nil || query.endAt == nil {
		t.Fatal("expected non-nil start/end")
	}
	if query.startAt.Format(playbackDateTimeFormat) != "2026-01-01 08:30:00" {
		t.Fatalf("unexpected startAt: %s", query.startAt.Format(playbackDateTimeFormat))
	}
	if query.endAt.Format(playbackDateTimeFormat) != "2026-01-15 20:45:00" {
		t.Fatalf("unexpected endAt: %s", query.endAt.Format(playbackDateTimeFormat))
	}
}
