package playback

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	embyint "github.com/konghang/ember/backend/internal/integrations/emby"
)

func TestBuildPlaybackWhereClauseEscapesLikeKeyword(t *testing.T) {
	query := &playbackHistoryQuery{
		PlaybackHistoryRequest: PlaybackHistoryRequest{
			Keyword: "a_b%",
		},
	}

	where := buildPlaybackWhereClause(query, nil)
	expected := "(ItemName LIKE '%a\\_b\\%%' ESCAPE '\\' OR UserName LIKE '%a\\_b\\%%' ESCAPE '\\' OR DeviceName LIKE '%a\\_b\\%%' ESCAPE '\\' OR ClientName LIKE '%a\\_b\\%%' ESCAPE '\\')"
	if where != "1=1 AND "+expected {
		t.Fatalf("where clause mismatch:\nwant: %s\ngot:  %s", "1=1 AND "+expected, where)
	}
}

func TestBuildPlaybackWhereClauseSupportsMultiplePlaybackUsers(t *testing.T) {
	query := &playbackHistoryQuery{}

	where := buildPlaybackWhereClause(query, []string{"emby_1", "emby_2"})
	expected := "1=1 AND UserId IN ('emby_1', 'emby_2')"
	if where != expected {
		t.Fatalf("where clause mismatch:\nwant: %s\ngot:  %s", expected, where)
	}
}

func TestParsePlaybackRowsKeepNilAsEmptyString(t *testing.T) {
	resp := &embyint.CustomQueryResponse{
		Colums: []string{"UserId", "UserName", "ItemName", "ItemType", "DateCreated", "DeviceName", "ClientName", "PlayDuration"},
		Results: [][]interface{}{
			{"emby_u_1", "alice", "movie", "Movie", "2026-03-06 10:00:00", nil, nil, float64(120)},
		},
	}

	rows, err := parsePlaybackRows(resp)
	if err != nil {
		t.Fatalf("parse rows failed: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("unexpected row count: %d", len(rows))
	}
	if rows[0].deviceName != "" || rows[0].clientName != "" {
		t.Fatalf("nil string field should be empty, got device=%q client=%q", rows[0].deviceName, rows[0].clientName)
	}
}

func TestParsePlaybackRowsSupportsColumnsField(t *testing.T) {
	resp := &embyint.CustomQueryResponse{
		Columns: []string{"UserId", "UserName", "ItemName", "ItemType", "DateCreated", "DeviceName", "ClientName", "PlayDuration"},
		Results: [][]interface{}{
			{"emby_u_1", "alice", "movie", "Movie", "2026-03-06 10:00:00", "TV", "Web", float64(120)},
		},
	}

	rows, err := parsePlaybackRows(resp)
	if err != nil {
		t.Fatalf("parse rows failed: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("unexpected row count: %d", len(rows))
	}
	if rows[0].itemName != "movie" || rows[0].deviceName != "TV" || rows[0].clientName != "Web" {
		t.Fatalf("unexpected parsed row: %+v", rows[0])
	}
}

func TestParsePlaybackTimeWithoutTimezoneUsesLocalTime(t *testing.T) {
	t.Setenv("CRON_TIMEZONE", "Asia/Shanghai")

	got := parsePlaybackTime("2026-03-06 10:00:00")
	want := time.Date(2026, 3, 6, 18, 0, 0, 0, loadPlaybackTimezone())
	if !got.Equal(want) {
		t.Fatalf("unexpected parsed time, want=%s got=%s", want.Format(time.RFC3339), got.Format(time.RFC3339))
	}
}

func TestParsePlaybackTimeWithTimezoneConvertsToConfiguredTimezone(t *testing.T) {
	t.Setenv("CRON_TIMEZONE", "Asia/Shanghai")

	got := parsePlaybackTime("2026-03-06T02:00:00Z")
	want := time.Date(2026, 3, 6, 10, 0, 0, 0, loadPlaybackTimezone())
	if !got.Equal(want) {
		t.Fatalf("unexpected parsed time, want=%s got=%s", want.Format(time.RFC3339), got.Format(time.RFC3339))
	}
}

func TestBuildPlaybackWhereClauseConvertsLocalDayToUTCRange(t *testing.T) {
	tz := time.FixedZone("UTC+8", 8*3600)
	start := time.Date(2026, 3, 21, 0, 0, 0, 0, tz)
	end := time.Date(2026, 3, 21, 0, 0, 0, 0, tz)
	query := &playbackHistoryQuery{
		startDate: &start,
		endDate:   &end,
	}

	where := buildPlaybackWhereClause(query, nil)
	expected := "1=1 AND DateCreated >= '2026-03-20 16:00:00' AND DateCreated <= '2026-03-21 15:59:59'"
	if where != expected {
		t.Fatalf("where clause mismatch:\nwant: %s\ngot:  %s", expected, where)
	}
}

func TestGetPlaybackHistoryReturnsReadablePluginError(t *testing.T) {
	t.Setenv("EMBY_URL", "")
	t.Setenv("EMBY_API_KEY", "")

	svc := NewPlaybackHistoryService()
	_, err := svc.GetPlaybackHistory(context.Background(), PlaybackHistoryRequest{})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, ErrPlaybackHistoryQueryFailed) {
		t.Fatalf("expected wrapped ErrPlaybackHistoryQueryFailed, got: %v", err)
	}
}

func TestShouldFallbackPlaybackDetailQuery(t *testing.T) {
	cases := []struct {
		name string
		resp *embyint.CustomQueryResponse
		want bool
	}{
		{
			name: "nil response",
			resp: nil,
			want: true,
		},
		{
			name: "has rows",
			resp: &embyint.CustomQueryResponse{
				Results: [][]interface{}{{"row"}},
			},
			want: false,
		},
		{
			name: "empty rows with pause column error",
			resp: &embyint.CustomQueryResponse{
				Results: [][]interface{}{},
				Message: "SQL error: no such column: PauseDuration",
			},
			want: true,
		},
		{
			name: "empty rows without error message",
			resp: &embyint.CustomQueryResponse{
				Results: [][]interface{}{},
				Message: "",
			},
			want: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := shouldFallbackPlaybackDetailQuery(tc.resp)
			if got != tc.want {
				t.Fatalf("want %v, got %v", tc.want, got)
			}
		})
	}
}

func TestShouldFallbackPlaybackDetailError(t *testing.T) {
	if !shouldFallbackPlaybackDetailError(fmt.Errorf("SQL error: no such column: PauseDuration")) {
		t.Fatal("pause/column sql error should trigger fallback")
	}
	if shouldFallbackPlaybackDetailError(fmt.Errorf("request timeout")) {
		t.Fatal("non-sql compatibility error should not trigger fallback")
	}
	if shouldFallbackPlaybackDetailError(nil) {
		t.Fatal("nil error should not trigger fallback")
	}
}
