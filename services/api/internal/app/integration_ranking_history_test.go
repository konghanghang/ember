package app

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/konghang/ember/backend/internal/handlers"
	"github.com/konghang/ember/backend/internal/models"
)

// TestIntegrationRankingHistorySelectsCompletePeriod verifies actual PostgreSQL
// ordering and date boundaries through the authenticated history endpoint.
func TestIntegrationRankingHistorySelectsCompletePeriod(t *testing.T) {
	h := newIntegrationHarness(t)
	t.Setenv("CRON_TIMEZONE", "Asia/Singapore")
	loc, err := time.LoadLocation("Asia/Singapore")
	if err != nil {
		t.Fatal(err)
	}
	for _, period := range []models.RankingPeriod{models.RankingDaily, models.RankingWeekly} {
		for _, legacy := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/legacy=%t", period, legacy), func(t *testing.T) {
				if err := h.database.Exec("DELETE FROM playback_rankings").Error; err != nil {
					t.Fatal(err)
				}
				start := time.Date(2026, 9, 14, 0, 0, 0, 0, loc)
				days := 1
				if period == models.RankingWeekly {
					days = 7
				}
				end := start.AddDate(0, 0, days)
				batch := func(key string) string {
					if legacy {
						return ""
					}
					return key
				}
				snapshot := func(id string, from, to time.Time) models.PlaybackRanking {
					return models.PlaybackRanking{ID: id, BatchID: batch(id), Period: period, Category: models.RankingMediaMovie, Rank: 1, ItemKey: id, ItemSourceType: "movie_item", ItemName: id, PlayCount: 1, Duration: 120, SnapshotAt: to, PeriodStart: from, PeriodEnd: to}
				}
				rows := []models.PlaybackRanking{
					snapshot("partial", start, start.Add(12*time.Hour)),
					snapshot("complete", start, end),
					snapshot("overrun", start, end.Add(time.Second)),
					snapshot("next_period", end, end.AddDate(0, 0, days)),
				}
				if err := h.database.Create(&rows).Error; err != nil {
					t.Fatal(err)
				}
				check := func(want string) {
					t.Helper()
					response := h.performAdminRequest(http.MethodGet, "/api/v1/rankings/history?period="+string(period)+"&date=2026-09-14", nil)
					if response.Code != http.StatusOK {
						t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
					}
					var payload handlers.RankingResponse
					if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
						t.Fatal(err)
					}
					if len(payload.Movies) != 1 || payload.Movies[0].ItemName != want {
						t.Fatalf("unexpected movies %+v, want %s", payload.Movies, want)
					}
				}
				check("complete")
				if err := h.database.Where("id = ?", "complete").Delete(&models.PlaybackRanking{}).Error; err != nil {
					t.Fatal(err)
				}
				check("partial")
			})
		}
	}
}
