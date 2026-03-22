package playback

import "testing"

func TestNormalizePlaybackProfileRangeSupportsCustomDates(t *testing.T) {
	t.Setenv("CRON_TIMEZONE", "Asia/Shanghai")

	rangeValue, startAt, endAt, err := normalizePlaybackProfileRange(PlaybackProfileQuery{
		StartDate: "2026-01-01",
		EndDate:   "2026-03-15",
	})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if rangeValue != "custom" {
		t.Fatalf("expected custom range, got %s", rangeValue)
	}
	if startAt == nil || endAt == nil {
		t.Fatal("expected non-nil start/end")
	}
	if startAt.Format("2006-01-02 15:04:05") != "2026-01-01 00:00:00" {
		t.Fatalf("unexpected startAt: %s", startAt.Format("2006-01-02 15:04:05"))
	}
	if endAt.Format("2006-01-02 15:04:05") != "2026-03-15 23:59:59" {
		t.Fatalf("unexpected endAt: %s", endAt.Format("2006-01-02 15:04:05"))
	}
}

func TestNormalizePlaybackProfileRangeSupportsCustomDateTime(t *testing.T) {
	t.Setenv("CRON_TIMEZONE", "Asia/Shanghai")

	rangeValue, startAt, endAt, err := normalizePlaybackProfileRange(PlaybackProfileQuery{
		StartDate: "2026-01-01 08:30:00",
		EndDate:   "2026-01-01 22:15:00",
	})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if rangeValue != "custom" {
		t.Fatalf("expected custom range, got %s", rangeValue)
	}
	if startAt == nil || endAt == nil {
		t.Fatal("expected non-nil start/end")
	}
	if startAt.Format("2006-01-02 15:04:05") != "2026-01-01 08:30:00" {
		t.Fatalf("unexpected startAt: %s", startAt.Format("2006-01-02 15:04:05"))
	}
	if endAt.Format("2006-01-02 15:04:05") != "2026-01-01 22:15:00" {
		t.Fatalf("unexpected endAt: %s", endAt.Format("2006-01-02 15:04:05"))
	}
}

func TestNormalizePlaybackProfileRangeRejectsTooLargeCustomWindow(t *testing.T) {
	t.Setenv("CRON_TIMEZONE", "Asia/Shanghai")

	_, _, _, err := normalizePlaybackProfileRange(PlaybackProfileQuery{
		StartDate: "2026-01-01",
		EndDate:   "2026-04-10",
	})
	if err != ErrPlaybackProfileRangeTooLarge {
		t.Fatalf("expected ErrPlaybackProfileRangeTooLarge, got %v", err)
	}
}
