package playback

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	embyint "github.com/konghang/ember/backend/internal/integrations/emby"
	"github.com/konghang/ember/backend/internal/models"
)

// TestMovieRankingTotalsIncludeCandidateTail 验证超过旧窗口上限、未入 Top 10 和不足 60 秒的数据仍计总量。
func TestMovieRankingTotalsIncludeCandidateTail(t *testing.T) {
	for _, allowAll := range []bool{false, true} {
		t.Run(fmt.Sprintf("allowAll=%t", allowAll), func(t *testing.T) {
			rows := make([][]any, 0, 3022)
			for i := 0; i < 10; i++ {
				rows = append(rows, []any{fmt.Sprintf("allowed_top_%d", i), "Movie", "movie_item", 1, 200})
			}
			for i := 0; i < 3010; i++ {
				rows = append(rows, []any{fmt.Sprintf("excluded_%d", i), "Other Movie", "movie_item", 1, 180})
			}
			rows = append(rows, []any{"allowed_tail", "Tail", "movie_item", 1, 100}, []any{"allowed_short", "Short", "movie_item", 1, 40})
			var queryCalls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/emby/user_usage_stats/submit_custom_query":
					queryCalls.Add(1)
					var request struct {
						SQL string `json:"CustomQueryString"`
					}
					if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
						t.Error(err)
						return
					}
					limit := len(rows)
					// fake 必须执行 LIMIT，否则旧截断路径也会误通过。
					if match := regexp.MustCompile(`LIMIT (\d+)`).FindStringSubmatch(request.SQL); len(match) > 0 {
						n, _ := strconv.Atoi(match[1])
						if n < limit {
							limit = n
						}
					}
					writeRankingAggregateFixture(w, rows[:limit])
				case "/emby/Users/admin/Items":
					ids := strings.Split(r.URL.Query().Get("Ids"), ",")
					if r.URL.Query().Get("ParentId") != "selected" || len(ids) > 100 {
						t.Error("invalid or unbatched library query")
					}
					items := []map[string]string{}
					for _, id := range ids {
						if strings.HasPrefix(id, "allowed_") {
							items = append(items, map[string]string{"Id": id, "Type": "Movie"})
						}
					}
					_ = json.NewEncoder(w).Encode(map[string]any{"Items": items, "TotalRecordCount": len(items)})
				default:
					t.Errorf("unexpected scan or request: %s", r.URL.Path)
					http.NotFound(w, r)
				}
			}))
			defer server.Close()
			t.Setenv("EMBY_URL", server.URL)
			t.Setenv("EMBY_API_KEY", "fixture-key")
			service := &PlaybackRankingService{embyService: embyint.NewEmbyService()}
			filter := rankingLibraryFilter{allowAll: allowAll, adminUserID: "admin", allowedLibraryIDs: map[string]struct{}{"selected": {}}}
			start := time.Date(2026, 9, 29, 0, 0, 0, 0, loadCronTimezone())
			items, total, err := service.fetchMovieRankingWithFilter(playbackActivityColumns{itemID: "ItemId", itemName: "ItemName"}, start, start.AddDate(0, 0, 1), 10, filter)
			want := int64(2140)
			if allowAll {
				want += 3010 * 180
			}
			if err != nil || total != want || len(items) != 10 {
				t.Fatalf("items=%d total=%d want=%d err=%v", len(items), total, want, err)
			}
			if queryCalls.Load() != 1 {
				t.Fatalf("expected one complete movie query, got %d", queryCalls.Load())
			}
		})
	}
}

// writeRankingAggregateFixture 固定插件拼写与聚合列合同，不调用真实 Playback Reporting。
func writeRankingAggregateFixture(w http.ResponseWriter, rows [][]any) {
	_ = json.NewEncoder(w).Encode(map[string]any{"colums": []string{"item_key", "item_name", "item_source_type", "play_count", "total_duration"}, "results": rows, "message": ""})
}

// TestRankingAggregateQueryUsesIdentityContract 锁定 SQL 仅按稳定 ID 分组，名称只是确定的展示候选。
func TestRankingAggregateQueryUsesIdentityContract(t *testing.T) {
	queries := make(chan string, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			SQL string `json:"CustomQueryString"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Error(err)
			return
		}
		queries <- request.SQL
		writeRankingAggregateFixture(w, [][]any{{"movie_1", "Title", "movie_item", 2, 150}, {"movie_2", "Title", "movie_item", 1, 100}})
	}))
	defer server.Close()
	t.Setenv("EMBY_URL", server.URL)
	t.Setenv("EMBY_API_KEY", "fixture-key")
	service := &PlaybackRankingService{embyService: embyint.NewEmbyService()}
	start := time.Date(2026, 9, 29, 0, 0, 0, 0, loadCronTimezone())
	rows, err := service.queryPlaybackAggregates("Movie", "ItemId", "ItemName", "movie_item", start, start.AddDate(0, 0, 1), 0)
	if err != nil {
		t.Fatal(err)
	}
	sql := <-queries
	if !strings.Contains(sql, "GROUP BY NULLIF(TRIM(COALESCE(ItemId, '')), '')\n") || !strings.Contains(sql, "MAX(NULLIF(TRIM(COALESCE(ItemName, '')), '')) AS item_name") {
		t.Fatalf("name must not split an identity: %s", sql)
	}
	if len(rows) != 2 || rows[0].itemKey == rows[1].itemKey || rows[0].duration != 150 {
		t.Fatalf("unexpected independent identities: %+v", rows)
	}
}

// TestEpisodeTotalsSkipMissingMetadata 验证成功响应缺失条目时跳过并记日志，短剧集仍计总量。
func TestEpisodeTotalsSkipMissingMetadata(t *testing.T) {
	for _, filtered := range []bool{false, true} {
		t.Run(fmt.Sprintf("filtered=%t", filtered), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/emby/Items" {
					_ = json.NewEncoder(w).Encode(map[string]any{"Items": []map[string]string{
						{"Id": "episode_good", "SeriesId": "series_good", "SeriesName": "Good"},
						{"Id": "episode_short", "SeriesId": "series_short", "SeriesName": "Short"},
						{"Id": "episode_no_series"},
					}})
					return
				}
				if r.URL.Path == "/emby/Users/admin/Items" {
					_ = json.NewEncoder(w).Encode(map[string]any{"Items": []map[string]string{{"Id": "series_good"}, {"Id": "series_short"}}})
					return
				}
				t.Errorf("unexpected path %s", r.URL.Path)
				http.NotFound(w, r)
			}))
			defer server.Close()
			t.Setenv("EMBY_URL", server.URL)
			t.Setenv("EMBY_API_KEY", "fixture-key")
			service := &PlaybackRankingService{embyService: embyint.NewEmbyService()}
			var logs bytes.Buffer
			previous := log.Writer()
			log.SetOutput(&logs)
			t.Cleanup(func() { log.SetOutput(previous) })
			var libraries map[string]struct{}
			if filtered {
				libraries = map[string]struct{}{"selected": {}}
			}
			rows := []playbackAggregateRow{{itemKey: "episode_good", duration: 100, playCount: 1}, {itemKey: "episode_short", duration: 40, playCount: 1}, {itemKey: "episode_missing", duration: 99, playCount: 1}, {itemKey: "episode_no_series", duration: 88, playCount: 1}}
			items, total, err := service.aggregateEpisodeRows(rows, time.Now(), time.Now(), "admin", libraries)
			if err != nil || len(items) != 1 || items[0].ItemKey != "series_good" || total != 140 {
				t.Fatalf("items=%+v total=%d err=%v", items, total, err)
			}
			for _, want := range []string{"episode_missing", "episode_no_series", "missingDetail=1", "missingSeries=1"} {
				if !strings.Contains(logs.String(), want) {
					t.Fatalf("missing diagnostic %q: %s", want, logs.String())
				}
			}
		})
	}
}

// TestRankingObsoleteAllowlistPublishesEmptyBatch 失效配置只读保留，全失效仍保存并通知空榜，不触达全库统计。
func TestRankingObsoleteAllowlistPublishesEmptyBatch(t *testing.T) {
	for _, period := range []models.RankingPeriod{models.RankingDaily, models.RankingWeekly} {
		t.Run(string(period), func(t *testing.T) {
			var aggregateCalls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/emby/Users":
					handleRankingUsersRequest(t, w, r)
				case "/emby/Users/admin_1/Views":
					handleRankingViewsRequest(t, w, r)
				case "/emby/user_usage_stats/submit_custom_query":
					aggregateCalls.Add(1)
					handlePlaybackQueryTestRequest(t, w, r)
				case "/emby/Items":
					handlePlaybackItemsTestRequest(t, w, r)
				default:
					t.Errorf("unexpected request: %s", r.URL.Path)
					http.NotFound(w, r)
				}
			}))
			defer server.Close()
			t.Setenv("EMBY_URL", server.URL)
			t.Setenv("EMBY_API_KEY", "fixture-key")
			notifications := &captureRankingNotifier{}
			writes, committed := 0, false
			service := &PlaybackRankingService{embyService: embyint.NewEmbyService(), notifier: notifications,
				loadLibraryAllowlist: func() ([]string, error) { return []string{"missing_library"}, nil },
				saveLibraryAllowlist: func([]string, *string) error { writes++; return nil },
				asyncGo:              func(_ string, task func()) { task() },
				persistBatch: func(result *RankingComputeResult) (bool, error) {
					if result.TotalDuration != 0 || len(result.Movies) != 0 || len(result.Episodes) != 0 {
						t.Errorf("obsolete allowlist leaked full-library data: %+v", result)
					}
					created := !committed
					committed = true
					return created, nil
				},
			}
			settings, err := service.GetRankingLibraryAllowlist()
			if err != nil || settings.AllowAll || len(settings.InvalidLibraryIDs) != 1 {
				t.Errorf("settings=%+v err=%v", settings, err)
			}
			for i := 0; i < 2; i++ {
				if err := service.GenerateRanking(period, nil, nil); err != nil {
					t.Fatal(err)
				}
			}
			if writes != 0 || aggregateCalls.Load() != 0 || !committed || len(notifications.payloads) != 1 {
				t.Fatalf("writes=%d queries=%d committed=%t notifications=%d", writes, aggregateCalls.Load(), committed, len(notifications.payloads))
			}
		})
	}
}

// TestRankingAggregateResponseValidation 区分损坏响应与明确缺失条目，不能把截断数据当空榜。
func TestRankingAggregateResponseValidation(t *testing.T) {
	columns := []string{"item_key", "item_name", "item_source_type", "play_count", "total_duration"}
	for _, tt := range []struct {
		name      string
		columns   []string
		rows      [][]any
		wantError bool
		wantRows  int
	}{
		{name: "short_row", columns: columns, rows: [][]any{{"movie_1"}}, wantError: true},
		{name: "missing_column", columns: columns[:4], rows: [][]any{{"movie_1", "Title", "movie_item", 1, 100}}, wantError: true},
		{name: "invalid_number", columns: columns, rows: [][]any{{"movie_1", "Title", "movie_item", 1, "bad-duration"}}, wantError: true},
		{name: "empty_response", columns: []string{}, rows: [][]any{}, wantRows: 0},
		{name: "missing_name", columns: columns, rows: [][]any{{"movie_missing", nil, "movie_item", 1, 40}, {"movie_1", "Title", "movie_item", 1, 100}}, wantRows: 1},
		{name: "invalid_duration", columns: columns, rows: [][]any{{"movie_bad", "Invalid", "movie_item", 1, -10}, {"movie_1", "Title", "movie_item", 1, 100}}, wantRows: 1},
		{name: "reordered_columns", columns: []string{"item_name", "item_source_type", "play_count", "total_duration", "item_key"}, rows: [][]any{{"Title", "movie_item", 1, 100, "movie_1"}}, wantRows: 1},
	} {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_ = json.NewEncoder(w).Encode(map[string]any{"colums": tt.columns, "results": tt.rows, "message": ""})
			}))
			defer server.Close()
			t.Setenv("EMBY_URL", server.URL)
			t.Setenv("EMBY_API_KEY", "fixture-key")
			service := &PlaybackRankingService{embyService: embyint.NewEmbyService()}
			rows, err := service.queryPlaybackAggregates("Movie", "ItemId", "ItemName", "movie_item", time.Now(), time.Now(), 0)
			if (err != nil) != tt.wantError || (!tt.wantError && len(rows) != tt.wantRows) {
				t.Fatalf("rows=%+v err=%v", rows, err)
			}
			if !tt.wantError && len(rows) == 1 && (rows[0].itemKey != "movie_1" || rows[0].duration != 100) {
				t.Fatalf("incorrect field mapping: %+v", rows[0])
			}
		})
	}
}

// TestRankingAllowlistValidatesBeforeNormalizingAll 防止同数量的失效选择绕过校验，意外保存为全库。
func TestRankingAllowlistValidatesBeforeNormalizingAll(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/emby/Users":
			handleRankingUsersRequest(t, w, r)
		case "/emby/Users/admin_1/Views":
			handleRankingViewsRequest(t, w, r)
		default:
			t.Errorf("unexpected request %s", r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	t.Setenv("EMBY_URL", server.URL)
	t.Setenv("EMBY_API_KEY", "fixture-key")
	writes := 0
	service := &PlaybackRankingService{embyService: embyint.NewEmbyService(), saveLibraryAllowlist: func([]string, *string) error { writes++; return nil }}
	if _, err := service.UpdateRankingLibraryAllowlist([]string{"lib_movie_only", "missing_library"}, nil); err != ErrRankingLibraryIDInvalid || writes != 0 {
		t.Fatalf("err=%v writes=%d", err, writes)
	}
}

// TestRankingUpstreamFailureDoesNotPersistOrNotify 错误响应不能触发空榜保存或发送，旧快照不受影响。
func TestRankingUpstreamFailureDoesNotPersistOrNotify(t *testing.T) {
	for _, failure := range []string{"stats", "views", "episode_details"} {
		t.Run(failure, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/emby/user_usage_stats/submit_custom_query":
					if failure == "stats" {
						http.Error(w, "fixture failure", http.StatusBadGateway)
						return
					}
					handlePlaybackQueryTestRequest(t, w, r)
				case "/emby/Users":
					handleRankingUsersRequest(t, w, r)
				case "/emby/Users/admin_1/Views":
					http.Error(w, "fixture failure", http.StatusForbidden)
				case "/emby/Items":
					http.Error(w, "fixture failure", http.StatusForbidden)
				default:
					t.Errorf("unexpected request %s", r.URL.Path)
					http.NotFound(w, r)
				}
			}))
			defer server.Close()
			t.Setenv("EMBY_URL", server.URL)
			t.Setenv("EMBY_API_KEY", "fixture-key")
			writes := 0
			notifications := &captureRankingNotifier{}
			service := &PlaybackRankingService{embyService: embyint.NewEmbyService(), notifier: notifications,
				loadLibraryAllowlist: func() ([]string, error) {
					if failure == "views" {
						return []string{"missing_library"}, nil
					}
					return nil, nil
				},
				persistBatch: func(*RankingComputeResult) (bool, error) { writes++; return true, nil },
				asyncGo:      func(_ string, work func()) { work() },
			}
			if err := service.GenerateRanking(models.RankingDaily, nil, nil); err == nil || writes != 0 || len(notifications.payloads) != 0 {
				t.Fatalf("err=%v writes=%d notifications=%d", err, writes, len(notifications.payloads))
			}
		})
	}
}
