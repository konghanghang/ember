package playback

import (
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/konghang/ember/backend/internal/db"
	"github.com/konghang/ember/backend/internal/models"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// TestGetHistoryRankingIncludesPeriodEnd locks the snapshot selection contract for
// complete and partial daily/weekly snapshots, including migrated legacy batch IDs.
func TestGetHistoryRankingIncludesPeriodEnd(t *testing.T) {
	for _, period := range []models.RankingPeriod{models.RankingDaily, models.RankingWeekly} {
		for _, legacy := range []bool{false, true} {
			for _, partial := range []bool{false, true} {
				name := string(period)
				if legacy {
					name += "/legacy"
				} else {
					name += "/batch"
				}
				if partial {
					name += "/partial"
				} else {
					name += "/complete"
				}
				t.Run(name, func(t *testing.T) {
					conn, mock, err := sqlmock.New()
					if err != nil {
						t.Fatal(err)
					}
					defer conn.Close()
					database, err := gorm.Open(postgres.New(postgres.Config{Conn: conn}), &gorm.Config{DisableAutomaticPing: true})
					if err != nil {
						t.Fatal(err)
					}
					original := db.DB
					db.DB = database
					defer func() { db.DB = original }()
					t.Setenv("CRON_TIMEZONE", "Asia/Singapore")
					location, err := time.LoadLocation("Asia/Singapore")
					if err != nil {
						t.Fatal(err)
					}
					start := time.Date(2026, 9, 14, 0, 0, 0, 0, location)
					days := 1
					if period == models.RankingWeekly {
						days = 7
					}
					end := start.AddDate(0, 0, days)
					cutoff := end
					if partial {
						cutoff = start.Add(12 * time.Hour)
					}
					batchID := "batch_history"
					if legacy {
						batchID = "legacy_history"
					}
					rows := func() *sqlmock.Rows {
						return sqlmock.NewRows([]string{"id", "batch_id", "period", "category", "rank", "item_name", "duration", "snapshot_at", "period_start", "period_end"}).
							AddRow("history_1", batchID, period, models.RankingMediaMovie, 1, "Fixture", 120, cutoff, start, cutoff)
					}
					query := `SELECT * FROM "playback_ranking_batches" WHERE period = $1 AND period_start = $2 AND period_end >= $3 AND period_end <= $4 ORDER BY period_end DESC,snapshot_at DESC,"playback_ranking_batches"."id" LIMIT $5`
					mock.ExpectQuery(regexp.QuoteMeta(query)).WithArgs(period, start, start, end, 1).
						WillReturnRows(sqlmock.NewRows([]string{"id", "period", "snapshot_at", "period_start", "period_end"}).AddRow(batchID, period, cutoff, start, cutoff))
					mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "playback_rankings" WHERE period = $1 AND batch_id = $2 ORDER BY category ASC,rank ASC`)).WithArgs(period, batchID).WillReturnRows(rows())
					result, err := (&PlaybackRankingService{}).GetHistoryRanking(period, start, end)
					if err != nil {
						t.Fatal(err)
					}
					if result == nil || !result.PeriodEnd.Equal(cutoff) || len(result.Movies) != 1 || result.Movies[0].Duration != 120 {
						t.Fatalf("unexpected result: %+v", result)
					}
					if err := mock.ExpectationsWereMet(); err != nil {
						t.Fatal(err)
					}
				})
			}
		}
	}
}
