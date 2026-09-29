package playback

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	configpkg "github.com/konghang/ember/backend/internal/config"
	"github.com/konghang/ember/backend/internal/db"
	embyint "github.com/konghang/ember/backend/internal/integrations/emby"
	"github.com/konghang/ember/backend/internal/models"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// rankingStoreFixture 生成跨两个类别的快照，避免单条 fixture 隐藏整期明细约束错误。
func rankingStoreFixture(count int) *RankingComputeResult {
	start := time.Date(2026, 9, 29, 0, 0, 0, 0, time.FixedZone("fixture", 8*60*60))
	result := &RankingComputeResult{Period: models.RankingDaily, BatchID: "batch_new", Start: start, End: start.AddDate(0, 0, 1), ComputedAt: start.Add(20 * time.Hour), TotalDuration: 3600}
	for i := 0; i < count; i++ {
		item := models.PlaybackRanking{ID: fmt.Sprintf("row_%d", i), Rank: i%10 + 1, ItemKey: fmt.Sprintf("item_%d", i), ItemName: "Fixture", Duration: 120, PlayCount: 1}
		if i < 10 {
			item.Category, item.ItemSourceType = models.RankingMediaMovie, "movie_item"
			result.Movies = append(result.Movies, item)
		} else {
			item.Category, item.ItemSourceType = models.RankingMediaEpisode, "series"
			result.Episodes = append(result.Episodes, item)
		}
	}
	return result
}

// TestPersistRankingBatchIsAtomic 验证完整写入、空榜、周期冲突和明细失败的事务边界。
func TestPersistRankingBatchIsAtomic(t *testing.T) {
	for _, scenario := range []string{"twenty_items", "empty", "duplicate", "item_failure", "short_write", "batch_failure"} {
		t.Run(scenario, func(t *testing.T) {
			mock := newRankingStoreMock(t)
			count := 20
			if scenario == "empty" {
				count = 0
			}
			fixture := rankingStoreFixture(count)
			mock.ExpectBegin()
			insert := mock.ExpectExec(`INSERT INTO "playback_ranking_batches" .*ON CONFLICT \("period","period_start","period_end"\) DO NOTHING`).
				WithArgs(fixture.BatchID, fixture.Period, fixture.Start, fixture.End, fixture.ComputedAt, fixture.TotalDuration, sqlmock.AnyArg())
			switch scenario {
			case "batch_failure":
				insert.WillReturnError(errors.New("batch write failed"))
				mock.ExpectRollback()
			case "duplicate":
				insert.WillReturnResult(sqlmock.NewResult(0, 0))
				mock.ExpectCommit()
			case "empty":
				insert.WillReturnResult(sqlmock.NewResult(0, 1))
				mock.ExpectCommit()
			default:
				insert.WillReturnResult(sqlmock.NewResult(0, 1))
				// 20 条明细、每条 14 列；不允许逐条静默跳过或只写第一条。
				details := mock.ExpectExec(`INSERT INTO "playback_rankings" .*\$280\)$`)
				if scenario == "item_failure" {
					details.WillReturnError(errors.New("detail write failed"))
					mock.ExpectRollback()
				} else if scenario == "short_write" {
					details.WillReturnResult(sqlmock.NewResult(0, 1))
					mock.ExpectRollback()
				} else {
					details.WillReturnResult(sqlmock.NewResult(0, 20))
					mock.ExpectCommit()
				}
			}
			created, err := persistRankingBatch(fixture)
			wantError := scenario == "batch_failure" || scenario == "item_failure" || scenario == "short_write"
			if (err != nil) != wantError {
				t.Fatalf("created=%v err=%v, want error=%v", created, err, wantError)
			}
			if created != (!wantError && scenario != "duplicate") {
				t.Fatalf("unexpected created=%v for %s", created, scenario)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

// TestGenerateRankingPublishesOnlyCreatedBatch 以 fake Emby 验证空榜也先持久化，重复/失败均不通知。
func TestGenerateRankingPublishesOnlyCreatedBatch(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/emby/user_usage_stats/submit_custom_query" {
			t.Errorf("unexpected request path %s", r.URL.Path)
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"colums":  []string{"DateCreated", "ItemId", "ItemType", "ItemName", "PlayDuration", "PauseDuration"},
			"results": []any{}, "message": "",
		})
	}))
	defer server.Close()
	t.Setenv("EMBY_URL", server.URL)
	t.Setenv("EMBY_API_KEY", "fixture-key")
	for _, scenario := range []string{"created", "duplicate", "failed"} {
		t.Run(scenario, func(t *testing.T) {
			notifier := &captureRankingNotifier{}
			stored := false
			batchID := ""
			var notification func()
			service := &PlaybackRankingService{
				embyService: embyint.NewEmbyService(), notifier: notifier,
				loadLibraryAllowlist: func() ([]string, error) { return nil, nil },
				asyncGo: func(_ string, work func()) {
					if !stored {
						t.Fatal("notification must be scheduled after persistence")
					}
					notification = work
				},
				persistBatch: func(result *RankingComputeResult) (bool, error) {
					stored = true
					batchID = result.BatchID
					if result.BatchID == "" || len(result.Movies)+len(result.Episodes) != 0 {
						t.Fatal("expected a stable empty-batch identity before persistence")
					}
					if scenario == "failed" {
						return false, errors.New("fixture persistence failure")
					}
					return scenario == "created", nil
				},
			}
			err := service.GenerateRanking(models.RankingDaily, nil, nil)
			if !stored || (err != nil) != (scenario == "failed") {
				t.Fatalf("stored=%v err=%v", stored, err)
			}
			if len(notifier.payloads) != 0 {
				t.Fatal("generation must finish without waiting for the asynchronous notification")
			}
			if (notification != nil) != (scenario == "created") {
				t.Fatal("only a newly committed batch may schedule a notification")
			}
			if notification != nil {
				notification()
			}
			want := 0
			if scenario == "created" {
				want = 1
			}
			if len(notifier.payloads) != want {
				t.Fatalf("notifications=%d want=%d", len(notifier.payloads), want)
			}
			if want == 1 && notifier.payloads[0].BatchID != batchID {
				t.Fatal("notification must retain the saved batch identity")
			}
		})
	}
}

// newRankingStoreMock 隔离榜单数据库与时区缓存，所有 SQL 均由 sqlmock 接管。
func newRankingStoreMock(t *testing.T) sqlmock.Sqlmock {
	t.Helper()
	conn, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	database, err := gorm.Open(postgres.New(postgres.Config{Conn: conn}), &gorm.Config{DisableAutomaticPing: true})
	if err != nil {
		t.Fatal(err)
	}
	original := db.DB
	db.DB = database
	configpkg.InvalidateCachedSetting("CRON_TIMEZONE")
	t.Cleanup(func() {
		db.DB = original
		configpkg.InvalidateCachedSetting("CRON_TIMEZONE")
		_ = conn.Close()
	})
	return mock
}

// TestGetLatestRankingReadsOpenPeriodBatch 覆盖未到午夜的正式榜以及没有明细的合法空榜。
func TestGetLatestRankingReadsOpenPeriodBatch(t *testing.T) {
	for _, empty := range []bool{false, true} {
		t.Run(map[bool]string{false: "with_items", true: "empty_batch"}[empty], func(t *testing.T) {
			mock := newRankingStoreMock(t)
			now := time.Now()
			start, end, generated := now.Add(-time.Hour), now.Add(time.Hour), now.Add(-time.Minute)
			mock.ExpectQuery(`SELECT .*"settings"`).WillReturnRows(sqlmock.NewRows([]string{"key", "value"}).AddRow("CRON_TIMEZONE", "Asia/Singapore"))
			query := `SELECT * FROM "playback_ranking_batches" WHERE period = $1 AND period_start <= $2 AND snapshot_at <= $3 ORDER BY period_end DESC,snapshot_at DESC,created_at DESC,"playback_ranking_batches"."id" LIMIT $4`
			mock.ExpectQuery(regexp.QuoteMeta(query)).WithArgs(models.RankingDaily, sqlmock.AnyArg(), sqlmock.AnyArg(), 1).
				WillReturnRows(sqlmock.NewRows([]string{"id", "period", "snapshot_at", "period_start", "period_end", "total_duration"}).AddRow("batch_live", "daily", generated, start, end, 120))
			rows := sqlmock.NewRows([]string{"id", "batch_id", "period", "category", "rank", "item_name", "duration"})
			if !empty {
				rows.AddRow("item_1", "batch_live", "daily", "media_movie", 1, "Fixture", 120)
			}
			mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "playback_rankings" WHERE period = $1 AND batch_id = $2 ORDER BY category ASC,rank ASC`)).
				WithArgs(models.RankingDaily, "batch_live").WillReturnRows(rows)
			result, err := (&PlaybackRankingService{}).GetLatestRanking(models.RankingDaily)
			if err != nil {
				t.Fatal(err)
			}
			if result == nil || result.BatchID != "batch_live" || !result.PeriodEnd.Equal(end) || !result.SnapshotAt.Equal(generated) {
				t.Fatalf("expected current-period batch metadata even without items, got %+v", result)
			}
			if empty != (len(result.Movies) == 0) || result.Movies == nil || result.Episodes == nil {
				t.Fatalf("unexpected batch items: %+v", result)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

// TestGetLatestRankingReadFailures 区分无批次与读取失败，避免数据库错误被包装成空榜。
func TestGetLatestRankingReadFailures(t *testing.T) {
	for _, scenario := range []string{"missing", "batch_error", "detail_error"} {
		t.Run(scenario, func(t *testing.T) {
			mock := newRankingStoreMock(t)
			mock.ExpectQuery(`SELECT .*"settings"`).WillReturnRows(sqlmock.NewRows([]string{"key", "value"}).AddRow("CRON_TIMEZONE", "Asia/Singapore"))
			selection := mock.ExpectQuery(`SELECT .*"playback_ranking_batches"`)
			failure := errors.New("fixture database error")
			switch scenario {
			case "missing":
				selection.WillReturnRows(sqlmock.NewRows([]string{"id"}))
			case "batch_error":
				selection.WillReturnError(failure)
			case "detail_error":
				selection.WillReturnRows(sqlmock.NewRows([]string{"id", "period"}).AddRow("batch_read", "daily"))
				mock.ExpectQuery(`SELECT .*"playback_rankings"`).WillReturnError(failure)
			}
			result, err := (&PlaybackRankingService{}).GetLatestRanking(models.RankingDaily)
			if result != nil || (scenario == "missing" && err != nil) || (scenario != "missing" && !errors.Is(err, failure)) {
				t.Fatalf("scenario=%s result=%+v err=%v", scenario, result, err)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

// TestRankingNotificationUsesGenerationTime 避免把自然日排他上界误报为生成时间。
func TestRankingNotificationUsesGenerationTime(t *testing.T) {
	t.Setenv("CRON_TIMEZONE", "Asia/Singapore")
	loc, err := time.LoadLocation("Asia/Singapore")
	if err != nil {
		t.Fatal(err)
	}
	start := time.Date(2026, 9, 29, 0, 0, 0, 0, loc)
	result := &RankingComputeResult{Period: models.RankingDaily, Start: start, End: start.AddDate(0, 0, 1), ComputedAt: start.Add(20 * time.Hour)}
	payload := buildRankingNotificationPayload(result)
	if payload.PeriodStart != "2026-09-29" || payload.PeriodEnd != "2026-09-29" || payload.CutoffAt != "20:00" {
		t.Fatalf("expected same-day display with generation time, got %+v", payload)
	}
}
