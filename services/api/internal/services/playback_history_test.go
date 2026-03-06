package services

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestBuildPlaybackWhereClauseEscapesLikeKeyword(t *testing.T) {
	query := &playbackHistoryQuery{
		PlaybackHistoryRequest: PlaybackHistoryRequest{
			Keyword: "a_b%",
		},
	}

	where := buildPlaybackWhereClause(query, "")
	expected := "(ItemName LIKE '%a\\_b\\%%' ESCAPE '\\' OR UserName LIKE '%a\\_b\\%%' ESCAPE '\\' OR DeviceName LIKE '%a\\_b\\%%' ESCAPE '\\' OR ClientName LIKE '%a\\_b\\%%' ESCAPE '\\')"
	if where != "1=1 AND "+expected {
		t.Fatalf("where clause mismatch:\nwant: %s\ngot:  %s", "1=1 AND "+expected, where)
	}
}

func TestParsePlaybackRowsKeepNilAsEmptyString(t *testing.T) {
	resp := &CustomQueryResponse{
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

func TestParsePlaybackTimeWithoutTimezoneUsesLocalTime(t *testing.T) {
	originalLocal := time.Local
	defer func() { time.Local = originalLocal }()

	loc := time.FixedZone("UTC+8", 8*3600)
	time.Local = loc

	got := parsePlaybackTime("2026-03-06 10:00:00")
	want := time.Date(2026, 3, 6, 2, 0, 0, 0, time.UTC)
	if !got.Equal(want) {
		t.Fatalf("unexpected parsed time, want=%s got=%s", want.Format(time.RFC3339), got.Format(time.RFC3339))
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
