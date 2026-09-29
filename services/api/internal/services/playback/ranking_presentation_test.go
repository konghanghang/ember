package playback

import (
	"testing"
	"time"
)

// TestRankingDisplayEndCoversDates 验证展示日期不受零点排他边界或夏令时天长影响。
func TestRankingDisplayEndCoversDates(t *testing.T) {
	loc, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Fatal(err)
	}
	start := time.Date(2026, 3, 8, 0, 0, 0, 0, loc)
	for _, tc := range []struct {
		name string
		end  time.Time
		want string
	}{
		{"daily_dst", start.AddDate(0, 0, 1), "2026-03-08"},
		{"weekly", start.AddDate(0, 0, 7), "2026-03-14"},
		{"partial", start.Add(12 * time.Hour), "2026-03-08"},
		{"same_boundary", start, "2026-03-08"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := RankingDisplayEnd(start, tc.end, loc).Format("2006-01-02"); got != tc.want {
				t.Fatalf("got %s, want %s", got, tc.want)
			}
		})
	}
	if !RankingDisplayEnd(start, time.Time{}, loc).IsZero() {
		t.Fatal("zero range end should stay zero")
	}
}
