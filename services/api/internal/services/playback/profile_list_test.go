package playback

import (
	"context"
	"testing"
	"time"

	embyint "github.com/konghang/ember/backend/internal/integrations/emby"
	"github.com/konghang/ember/backend/internal/models"
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

func TestParsePlaybackProfileOverviewAggregateRows(t *testing.T) {
	resp := &embyint.CustomQueryResponse{
		Columns: []string{"UserId", "TotalPlayCount", "TotalPlayDuration", "ActiveDays", "LastPlayedAt"},
		Results: [][]interface{}{
			{"emby_u_1", float64(12), float64(3600), float64(5), "2026-04-30 10:00:00"},
		},
	}

	rows, err := parsePlaybackProfileOverviewAggregateRows(resp)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
	if rows[0].playbackUserID != "emby_u_1" || rows[0].totalPlayCount != 12 || rows[0].totalPlayDuration != 3600 || rows[0].activeDays != 5 {
		t.Fatalf("unexpected row: %+v", rows[0])
	}
	if rows[0].lastPlayedAt == nil {
		t.Fatal("expected non-nil lastPlayedAt")
	}
}

func TestPlaybackSQLiteLocalDateExprUsesStoredLocalTimeDirectly(t *testing.T) {
	t.Setenv("CRON_TIMEZONE", "Asia/Shanghai")
	if got := playbackSQLiteLocalDateExpr("DateCreated"); got != "strftime('%Y-%m-%d', DateCreated)" {
		t.Fatalf("unexpected local date expression: %s", got)
	}
}

func TestBuildPlaybackProfileListItemsUsesAggregateRows(t *testing.T) {
	service := &UserPlaybackProfileService{}
	lastPlayedAt := time.Date(2026, 4, 30, 18, 0, 0, 0, loadPlaybackTimezone())
	items, mapping := service.buildPlaybackProfileListItems(
		"30d",
		[]playbackProfileOverviewAggregateRow{
			{
				playbackUserID:    "emby_u_1",
				totalPlayCount:    8,
				totalPlayDuration: 2400,
				activeDays:        3,
				lastPlayedAt:      &lastPlayedAt,
			},
		},
		map[string]models.User{
			"emby_u_1": {
				ID:       "user_1",
				Username: "alice",
				EmbyID:   "emby_u_1",
			},
		},
	)

	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}
	if items[0].UserID != "user_1" || items[0].Username != "alice" || items[0].TotalPlayCount != 8 || items[0].TotalPlayDuration != 2400 || items[0].ActiveDays != 3 {
		t.Fatalf("unexpected item: %+v", items[0])
	}
	if mapping["user_1"] != "emby_u_1" {
		t.Fatalf("unexpected mapping: %+v", mapping)
	}
}
