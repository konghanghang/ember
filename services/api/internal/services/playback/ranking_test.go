package playback

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	embyint "github.com/konghang/ember/backend/internal/integrations/emby"
	"github.com/konghang/ember/backend/internal/models"
	"github.com/oklog/ulid/v2"
)

func TestGenerateRankingBatchIDUsesULID(t *testing.T) {
	got := generateRankingBatchID()

	if len(got) != 26 {
		t.Fatalf("expected ULID length 26, got %d (%q)", len(got), got)
	}
	if _, err := ulid.ParseStrict(got); err != nil {
		t.Fatalf("expected valid ULID, got %q err=%v", got, err)
	}
}

func TestPreviewRankingGroupsEpisodesBySeriesID(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/emby/user_usage_stats/submit_custom_query":
			handlePlaybackQueryTestRequest(t, w, r)
		case "/emby/Items":
			handlePlaybackItemsTestRequest(t, w, r)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	t.Setenv("EMBY_URL", server.URL)
	t.Setenv("EMBY_API_KEY", "test-key")
	t.Setenv("CRON_TIMEZONE", "UTC")

	svc := &PlaybackRankingService{
		embyService: embyint.NewEmbyService(),
	}
	result, err := svc.PreviewRanking(models.RankingWeekly)
	if err != nil {
		t.Fatalf("preview ranking failed: %v", err)
	}

	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if len(result.Movies) != 1 {
		t.Fatalf("expected 1 movie ranking, got %d", len(result.Movies))
	}
	if len(result.Episodes) != 2 {
		t.Fatalf("expected 2 episode rankings, got %d", len(result.Episodes))
	}

	if result.Movies[0].ItemKey != "movie_1" {
		t.Fatalf("expected movie_1 to remain after filtering, got %q", result.Movies[0].ItemKey)
	}
	if result.Movies[0].Duration != 5400 {
		t.Fatalf("expected movie_1 duration 5400, got %d", result.Movies[0].Duration)
	}

	topEpisode := result.Episodes[0]
	if topEpisode.ItemKey != "series_a" {
		t.Fatalf("expected merged series_a, got %q", topEpisode.ItemKey)
	}
	if topEpisode.ItemName != "斗罗大陆II绝世唐门" {
		t.Fatalf("expected merged series name, got %q", topEpisode.ItemName)
	}
	if topEpisode.PlayCount != 3 {
		t.Fatalf("expected merged playCount 3, got %d", topEpisode.PlayCount)
	}
	if topEpisode.Duration != 2100 {
		t.Fatalf("expected merged duration 2100, got %d", topEpisode.Duration)
	}

	secondEpisode := result.Episodes[1]
	if secondEpisode.ItemKey != "series_b" {
		t.Fatalf("expected series_b as second ranking, got %q", secondEpisode.ItemKey)
	}
	if secondEpisode.Duration != 1200 {
		t.Fatalf("expected second series duration 1200, got %d", secondEpisode.Duration)
	}
}

func TestPreviewRankingKeepsSuccessfulEpisodeBatchesWhenSomeLookupsFail(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/emby/user_usage_stats/submit_custom_query":
			handlePlaybackPartialBatchQueryTestRequest(t, w, r)
		case "/emby/Items":
			handlePlaybackPartialBatchItemsTestRequest(t, w, r)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	t.Setenv("EMBY_URL", server.URL)
	t.Setenv("EMBY_API_KEY", "test-key")
	t.Setenv("CRON_TIMEZONE", "UTC")

	svc := &PlaybackRankingService{
		embyService: embyint.NewEmbyService(),
	}
	result, err := svc.PreviewRanking(models.RankingWeekly)
	if err != nil {
		t.Fatalf("preview ranking failed: %v", err)
	}

	if len(result.Episodes) != 1 {
		t.Fatalf("expected 1 episode ranking after partial batch failure, got %d", len(result.Episodes))
	}
	if result.Episodes[0].ItemKey != "series_a" {
		t.Fatalf("expected surviving series_a ranking, got %q", result.Episodes[0].ItemKey)
	}
	if result.Episodes[0].PlayCount != 100 {
		t.Fatalf("expected aggregated playCount 100, got %d", result.Episodes[0].PlayCount)
	}
}

func TestPreviewRankingReturnsMoviesWhenAllEpisodeLookupBatchesFail(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/emby/user_usage_stats/submit_custom_query":
			handlePlaybackAllBatchFailQueryTestRequest(t, w, r)
		case "/emby/Items":
			http.Error(w, "upstream timeout", http.StatusGatewayTimeout)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	t.Setenv("EMBY_URL", server.URL)
	t.Setenv("EMBY_API_KEY", "test-key")
	t.Setenv("CRON_TIMEZONE", "UTC")

	svc := &PlaybackRankingService{
		embyService: embyint.NewEmbyService(),
	}
	result, err := svc.PreviewRanking(models.RankingWeekly)
	if err != nil {
		t.Fatalf("expected preview ranking to degrade gracefully, got error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if len(result.Movies) != 0 {
		t.Fatalf("expected no movie rankings, got %d", len(result.Movies))
	}
	if len(result.Episodes) != 0 {
		t.Fatalf("expected no episode rankings when all lookups fail, got %d", len(result.Episodes))
	}
}

func handlePlaybackQueryTestRequest(t *testing.T, w http.ResponseWriter, r *http.Request) {
	t.Helper()

	body, err := io.ReadAll(r.Body)
	if err != nil {
		t.Fatalf("read body failed: %v", err)
	}

	var req map[string]string
	if err := json.Unmarshal(body, &req); err != nil {
		t.Fatalf("unmarshal body failed: %v", err)
	}

	sql := req["CustomQueryString"]
	switch {
	case strings.Contains(sql, "SELECT * FROM PlaybackActivity LIMIT 1"):
		_ = json.NewEncoder(w).Encode(map[string]any{
			"columns": []string{"DateCreated", "UserId", "ItemId", "ItemType", "ItemName", "PlayDuration", "PauseDuration"},
			"results": []any{},
			"message": "",
		})
	case strings.Contains(sql, "ItemType = 'Movie'"):
		_ = json.NewEncoder(w).Encode(map[string]any{
			"columns": []string{"item_key", "item_name", "item_source_type", "play_count", "total_duration"},
			"results": [][]any{
				{"movie_1", "星际穿越", "movie_item", 2, 5400},
				{"movie_short", "短片预告", "movie_item", 1, 59},
			},
			"message": "",
		})
	case strings.Contains(sql, "ItemType = 'Episode'"):
		_ = json.NewEncoder(w).Encode(map[string]any{
			"columns": []string{"item_key", "item_name", "item_source_type", "play_count", "total_duration"},
			"results": [][]any{
				{"ep_145", "斗罗大陆II绝世唐门 - s02e145", "episode_item", 2, 1800},
				{"ep_144", "斗罗大陆II绝世唐门 - s02e144", "episode_item", 1, 300},
				{"ep_b1", "吞噬星空 - s07e215", "episode_item", 2, 1200},
				{"ep_zero", "无效剧集 - s01e01", "episode_item", 1, 0},
			},
			"message": "",
		})
	case strings.Contains(sql, "SUM(COALESCE(PlayDuration, 0) - COALESCE(PauseDuration, 0))"):
		_ = json.NewEncoder(w).Encode(map[string]any{
			"columns": []string{"total_duration"},
			"results": [][]any{
				{6600},
			},
			"message": "",
		})
	default:
		t.Fatalf("unexpected playback sql: %s", sql)
	}
}

func handlePlaybackItemsTestRequest(t *testing.T, w http.ResponseWriter, r *http.Request) {
	t.Helper()

	rawIDs := strings.TrimSpace(r.URL.Query().Get("Ids"))
	ids := strings.Split(rawIDs, ",")
	items := make([]map[string]any, 0, len(ids))

	for _, id := range ids {
		id = strings.TrimSpace(id)
		switch id {
		case "ep_145", "ep_144":
			items = append(items, map[string]any{
				"Id":         id,
				"SeriesId":   "series_a",
				"SeriesName": "斗罗大陆II绝世唐门",
			})
		case "ep_b1":
			items = append(items, map[string]any{
				"Id":         id,
				"SeriesId":   "series_b",
				"SeriesName": "吞噬星空",
			})
		case "ep_zero":
			items = append(items, map[string]any{
				"Id":         id,
				"SeriesId":   "series_zero",
				"SeriesName": "无效剧集",
			})
		default:
			t.Fatalf("unexpected item id lookup: %s", id)
		}
	}

	_ = json.NewEncoder(w).Encode(map[string]any{
		"Items":            items,
		"TotalRecordCount": len(items),
	})
}

func handlePlaybackPartialBatchQueryTestRequest(t *testing.T, w http.ResponseWriter, r *http.Request) {
	t.Helper()

	body, err := io.ReadAll(r.Body)
	if err != nil {
		t.Fatalf("read body failed: %v", err)
	}

	var req map[string]string
	if err := json.Unmarshal(body, &req); err != nil {
		t.Fatalf("unmarshal body failed: %v", err)
	}

	sql := req["CustomQueryString"]
	switch {
	case strings.Contains(sql, "SELECT * FROM PlaybackActivity LIMIT 1"):
		_ = json.NewEncoder(w).Encode(map[string]any{
			"columns": []string{"DateCreated", "UserId", "ItemId", "ItemType", "ItemName", "PlayDuration", "PauseDuration"},
			"results": []any{},
			"message": "",
		})
	case strings.Contains(sql, "ItemType = 'Movie'"):
		_ = json.NewEncoder(w).Encode(map[string]any{
			"columns": []string{"item_key", "item_name", "item_source_type", "play_count", "total_duration"},
			"results": [][]any{
				{"movie_1", "Movie", "movie_item", 1, 300},
			},
			"message": "",
		})
	case strings.Contains(sql, "ItemType = 'Episode'"):
		results := make([][]any, 0, 101)
		for i := 1; i <= 100; i++ {
			results = append(results, []any{"ep_a_" + strconv.Itoa(i), "Series A", "episode_item", 1, 120})
		}
		results = append(results, []any{"ep_fail_101", "Series Fail", "episode_item", 1, 120})
		_ = json.NewEncoder(w).Encode(map[string]any{
			"columns": []string{"item_key", "item_name", "item_source_type", "play_count", "total_duration"},
			"results": results,
			"message": "",
		})
	case strings.Contains(sql, "SUM(COALESCE(PlayDuration, 0) - COALESCE(PauseDuration, 0))"):
		_ = json.NewEncoder(w).Encode(map[string]any{
			"columns": []string{"total_duration"},
			"results": [][]any{
				{12300},
			},
			"message": "",
		})
	default:
		t.Fatalf("unexpected playback sql: %s", sql)
	}
}

func handlePlaybackPartialBatchItemsTestRequest(t *testing.T, w http.ResponseWriter, r *http.Request) {
	t.Helper()

	rawIDs := strings.TrimSpace(r.URL.Query().Get("Ids"))
	if strings.Contains(rawIDs, "ep_fail_101") {
		http.Error(w, "upstream busy", http.StatusBadGateway)
		return
	}

	ids := strings.Split(rawIDs, ",")
	items := make([]map[string]any, 0, len(ids))
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		items = append(items, map[string]any{
			"Id":         id,
			"SeriesId":   "series_a",
			"SeriesName": "Series A",
		})
	}

	_ = json.NewEncoder(w).Encode(map[string]any{
		"Items":            items,
		"TotalRecordCount": len(items),
	})
}

func handlePlaybackAllBatchFailQueryTestRequest(t *testing.T, w http.ResponseWriter, r *http.Request) {
	t.Helper()

	body, err := io.ReadAll(r.Body)
	if err != nil {
		t.Fatalf("read body failed: %v", err)
	}

	var req map[string]string
	if err := json.Unmarshal(body, &req); err != nil {
		t.Fatalf("unmarshal body failed: %v", err)
	}

	sql := req["CustomQueryString"]
	switch {
	case strings.Contains(sql, "SELECT * FROM PlaybackActivity LIMIT 1"):
		_ = json.NewEncoder(w).Encode(map[string]any{
			"columns": []string{"DateCreated", "UserId", "ItemId", "ItemType", "ItemName", "PlayDuration", "PauseDuration"},
			"results": []any{},
			"message": "",
		})
	case strings.Contains(sql, "ItemType = 'Movie'"):
		_ = json.NewEncoder(w).Encode(map[string]any{
			"columns": []string{"item_key", "item_name", "item_source_type", "play_count", "total_duration"},
			"results": [][]any{},
			"message": "",
		})
	case strings.Contains(sql, "ItemType = 'Episode'"):
		_ = json.NewEncoder(w).Encode(map[string]any{
			"columns": []string{"item_key", "item_name", "item_source_type", "play_count", "total_duration"},
			"results": [][]any{
				{"ep_fail_1", "Series Fail 1", "episode_item", 1, 300},
				{"ep_fail_2", "Series Fail 2", "episode_item", 1, 300},
			},
			"message": "",
		})
	case strings.Contains(sql, "SUM(COALESCE(PlayDuration, 0) - COALESCE(PauseDuration, 0))"):
		_ = json.NewEncoder(w).Encode(map[string]any{
			"columns": []string{"total_duration"},
			"results": [][]any{
				{600},
			},
			"message": "",
		})
	default:
		t.Fatalf("unexpected playback sql: %s", sql)
	}
}
