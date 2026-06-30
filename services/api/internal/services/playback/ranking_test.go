package playback

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	embyint "github.com/konghang/ember/backend/internal/integrations/emby"
	notifierint "github.com/konghang/ember/backend/internal/integrations/notifier"
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

func TestPlaybackRankingColumnHelpers(t *testing.T) {
	resp := &embyint.CustomQueryResponse{
		Columns: []string{" ItemId ", "ItemName"},
		Colums:  []string{"legacy"},
	}
	if got := queryColumns(resp); len(got) != 2 || got[0] != " ItemId " {
		t.Fatalf("expected columns to take precedence, got %+v", got)
	}

	resp = &embyint.CustomQueryResponse{Colums: []string{"legacy_item"}}
	if got := queryColumns(resp); len(got) != 1 || got[0] != "legacy_item" {
		t.Fatalf("expected legacy colums fallback, got %+v", got)
	}

	columns := []string{" DateCreated ", "itemid", "ITEMNAME"}
	if got := matchColumn(columns, "ItemId"); got != "itemid" {
		t.Fatalf("expected case-insensitive match, got %q", got)
	}
	if got := matchColumn(columns, "Missing"); got != "" {
		t.Fatalf("expected missing column to return blank, got %q", got)
	}
	if got := nullableTrimExpr(" ItemName "); got != "NULLIF(TRIM(COALESCE( ItemName , '')), '')" {
		t.Fatalf("unexpected nullable trim expression: %s", got)
	}
	if got := nullableTrimExpr(" "); got != "NULL" {
		t.Fatalf("expected blank column to become NULL, got %s", got)
	}
}

func TestPlaybackRankingLibraryAllowlistHelpers(t *testing.T) {
	got, err := parsePlaybackRankingLibraryAllowlist(`[" lib_b ","lib_a","","lib_b"]`)
	if err != nil {
		t.Fatalf("parse allowlist: %v", err)
	}
	want := []string{"lib_a", "lib_b"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("expected %+v, got %+v", want, got)
	}

	if _, err := parsePlaybackRankingLibraryAllowlist(`{"bad":true}`); err == nil {
		t.Fatal("expected invalid allowlist json to fail")
	}

	valid, invalid := partitionRankingLibraryIDs(
		[]string{"lib_b", "lib_missing", "lib_a"},
		map[string]RankingLibraryOption{
			"lib_a": {ID: "lib_a"},
			"lib_b": {ID: "lib_b"},
		},
	)
	if !reflect.DeepEqual(valid, []string{"lib_a", "lib_b"}) {
		t.Fatalf("unexpected valid ids: %+v", valid)
	}
	if !reflect.DeepEqual(invalid, []string{"lib_missing"}) {
		t.Fatalf("unexpected invalid ids: %+v", invalid)
	}
}

func TestFilterPlaybackAggregateRowsAndSumDuration(t *testing.T) {
	rows := []playbackAggregateRow{
		{itemKey: "movie_1", duration: 100},
		{itemKey: "movie_2", duration: 50},
		{itemKey: "movie_3", duration: -1},
	}

	filtered := filterPlaybackAggregateRows(rows, map[string]struct{}{
		"movie_2": {},
		"movie_3": {},
	})
	if len(filtered) != 2 || filtered[0].itemKey != "movie_2" || filtered[1].itemKey != "movie_3" {
		t.Fatalf("unexpected filtered rows: %+v", filtered)
	}
	if got := sumPlaybackAggregateDuration(filtered); got != 50 {
		t.Fatalf("expected summed duration 50, got %d", got)
	}
}

func TestConvertAggregateRowsFiltersShortRowsAndRanksRemaining(t *testing.T) {
	rankings := convertAggregateRows(models.RankingMediaMovie, []playbackAggregateRow{
		{itemKey: "short", itemName: "Too Short", playCount: 1, duration: minRankingDurationSeconds - 1},
		{itemKey: "movie_1", itemSourceType: "movie", itemName: "Movie 1", playCount: 3, duration: 7200},
		{itemKey: "movie_2", itemSourceType: "movie", itemName: "Movie 2", playCount: 2, duration: minRankingDurationSeconds},
	})

	if len(rankings) != 2 {
		t.Fatalf("expected 2 rankings after short-duration filter, got %d", len(rankings))
	}
	if rankings[0].Rank != 1 || rankings[0].ItemKey != "movie_1" || rankings[0].Category != models.RankingMediaMovie {
		t.Fatalf("unexpected first ranking: %+v", rankings[0])
	}
	if rankings[1].Rank != 2 || rankings[1].ItemKey != "movie_2" {
		t.Fatalf("unexpected second ranking: %+v", rankings[1])
	}
}

func TestBuildRankingResultFromRowsSplitsCategoriesAndCopiesMetadata(t *testing.T) {
	snapshotAt := time.Date(2026, 6, 17, 8, 0, 0, 0, time.UTC)
	start := time.Date(2026, 6, 16, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 6, 17, 0, 0, 0, 0, time.UTC)
	rows := []models.PlaybackRanking{
		{
			Period:      models.RankingDaily,
			BatchID:     "batch_1",
			Category:    models.RankingMediaEpisode,
			Rank:        1,
			ItemKey:     "series_1",
			ItemName:    "Show",
			PlayCount:   4,
			Duration:    3600,
			SnapshotAt:  snapshotAt,
			PeriodStart: start,
			PeriodEnd:   end,
		},
		{
			Period:         models.RankingDaily,
			BatchID:        "batch_1",
			Category:       models.RankingMediaMovie,
			Rank:           1,
			ItemKey:        "movie_1",
			ItemSourceType: "movie",
			ItemName:       "Movie",
			PlayCount:      2,
			Duration:       7200,
			SnapshotAt:     snapshotAt,
			PeriodStart:    start,
			PeriodEnd:      end,
		},
		{Category: models.RankingCategory("ignored"), ItemKey: "ignored"},
	}

	result := buildRankingResultFromRows(rows)
	if result == nil {
		t.Fatal("expected ranking result")
	}
	if result.Period != models.RankingDaily || result.BatchID != "batch_1" || !result.SnapshotAt.Equal(snapshotAt) {
		t.Fatalf("unexpected result metadata: %+v", result)
	}
	if len(result.Movies) != 1 || result.Movies[0].ItemKey != "movie_1" || result.Movies[0].ItemSourceType != "movie" {
		t.Fatalf("unexpected movie items: %+v", result.Movies)
	}
	if len(result.Episodes) != 1 || result.Episodes[0].ItemKey != "series_1" || result.Episodes[0].PlayCount != 4 {
		t.Fatalf("unexpected episode items: %+v", result.Episodes)
	}
	if buildRankingResultFromRows(nil) != nil {
		t.Fatal("expected empty rows to return nil result")
	}
}

func TestPlaybackRankingTypeConverters(t *testing.T) {
	if got := asString([]byte("movie")); got != "movie" {
		t.Fatalf("expected bytes to string, got %q", got)
	}
	if got := asString(123); got != "123" {
		t.Fatalf("expected fmt string conversion, got %q", got)
	}

	intCases := []struct {
		name string
		in   interface{}
		want int64
	}{
		{name: "nil", in: nil, want: 0},
		{name: "float64", in: float64(12.8), want: 12},
		{name: "float32", in: float32(7.9), want: 7},
		{name: "int", in: int(3), want: 3},
		{name: "int64", in: int64(4), want: 4},
		{name: "int32", in: int32(5), want: 5},
		{name: "uint64", in: uint64(6), want: 6},
		{name: "uint32", in: uint32(7), want: 7},
		{name: "string", in: " 8 ", want: 8},
	}
	for _, tc := range intCases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := asInt64(tc.in)
			if err != nil {
				t.Fatalf("expected conversion success, got %v", err)
			}
			if got != tc.want {
				t.Fatalf("expected %d, got %d", tc.want, got)
			}
		})
	}

	gotInt, err := asInt("9")
	if err != nil || gotInt != 9 {
		t.Fatalf("expected asInt string conversion to 9, got %d err=%v", gotInt, err)
	}
	if _, err := asInt64("not-a-number"); err == nil {
		t.Fatal("expected invalid numeric string to fail")
	}
	if _, err := asInt64(struct{}{}); err == nil {
		t.Fatal("expected unsupported type to fail")
	}
	if _, err := asInt("not-a-number"); err == nil {
		t.Fatal("expected asInt to propagate conversion error")
	}
}

func TestPlaybackRankingRangeHelpersUseConfiguredTimezone(t *testing.T) {
	t.Setenv("CRON_TIMEZONE", "Asia/Shanghai")
	input := time.Date(2026, 6, 16, 18, 30, 0, 0, time.UTC)

	dayStart, dayEnd := dayRange(input)
	if dayStart.Format(time.RFC3339) != "2026-06-17T00:00:00+08:00" ||
		dayEnd.Format(time.RFC3339) != "2026-06-18T00:00:00+08:00" {
		t.Fatalf("unexpected day range: %s~%s", dayStart, dayEnd)
	}

	weekStart, weekEnd := weekRange(time.Date(2026, 6, 21, 12, 0, 0, 0, time.FixedZone("UTC+8", 8*60*60)))
	if weekStart.Format("2006-01-02") != "2026-06-15" ||
		weekEnd.Format("2006-01-02") != "2026-06-22" {
		t.Fatalf("unexpected week range: %s~%s", weekStart, weekEnd)
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

func TestPreviewRankingFiltersByLibraryAllowlist(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/emby/user_usage_stats/submit_custom_query":
			handlePlaybackQueryTestRequest(t, w, r)
		case "/emby/Items":
			if r.Method == http.MethodGet && strings.TrimSpace(r.URL.Query().Get("Ids")) != "" {
				handlePlaybackItemsTestRequest(t, w, r)
				return
			}
			handleRankingLibraryItemsRequest(t, w, r)
		case "/emby/Library/VirtualFolders/Query":
			handleRankingLibrariesRequest(t, w, r)
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
		loadLibraryAllowlist: func() ([]string, error) {
			return []string{"lib_movie_only"}, nil
		},
	}

	result, err := svc.PreviewRanking(models.RankingWeekly)
	if err != nil {
		t.Fatalf("preview ranking failed: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if len(result.Movies) != 1 || result.Movies[0].ItemKey != "movie_1" {
		t.Fatalf("expected only allowed movie ranking, got %+v", result.Movies)
	}
	if len(result.Episodes) != 0 {
		t.Fatalf("expected no episode rankings after allowlist filter, got %+v", result.Episodes)
	}
}

func TestComputeRankingFiltersTotalDurationByLibraryAllowlist(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/emby/user_usage_stats/submit_custom_query":
			handlePlaybackQueryTestRequest(t, w, r)
		case "/emby/Items":
			if r.Method == http.MethodGet && strings.TrimSpace(r.URL.Query().Get("Ids")) != "" {
				handlePlaybackItemsTestRequest(t, w, r)
				return
			}
			handleRankingLibraryItemsRequest(t, w, r)
		case "/emby/Library/VirtualFolders/Query":
			handleRankingLibrariesRequest(t, w, r)
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
		loadLibraryAllowlist: func() ([]string, error) {
			return []string{"lib_movie_only"}, nil
		},
	}

	result, err := svc.computeRanking(models.RankingWeekly, nil, nil)
	if err != nil {
		t.Fatalf("compute ranking failed: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.TotalDuration != 5400 {
		t.Fatalf("expected filtered totalDuration 5400, got %d", result.TotalDuration)
	}
}

func TestComputeRankingUsesFullDurationWhenAllowlistIsEmpty(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/emby/user_usage_stats/submit_custom_query":
			handlePlaybackQueryTestRequest(t, w, r)
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
		loadLibraryAllowlist: func() ([]string, error) {
			return []string{}, nil
		},
	}

	result, err := svc.computeRanking(models.RankingWeekly, nil, nil)
	if err != nil {
		t.Fatalf("compute ranking failed: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.TotalDuration != 6600 {
		t.Fatalf("expected full totalDuration 6600, got %d", result.TotalDuration)
	}
}

func TestBuildRankingNotificationPayloadKeepsFilteredTotalDuration(t *testing.T) {
	computeResult := &RankingComputeResult{
		Period:        models.RankingWeekly,
		Start:         time.Date(2026, 6, 30, 0, 0, 0, 0, time.UTC),
		End:           time.Date(2026, 7, 7, 20, 30, 0, 0, time.UTC),
		TotalDuration: 5400,
		Movies: []models.PlaybackRanking{
			{Rank: 1, ItemName: "Movie A", PlayCount: 2, Duration: 5400},
		},
		Episodes: []models.PlaybackRanking{},
	}

	payload := buildRankingNotificationPayload(computeResult)
	if payload.TotalDuration != 5400 {
		t.Fatalf("expected payload totalDuration 5400, got %d", payload.TotalDuration)
	}
	if payload.Period != "weekly" || payload.PeriodStart != "2026-06-30" || payload.PeriodEnd != "2026-07-07" {
		t.Fatalf("unexpected payload range: %+v", payload)
	}
	if len(payload.Movies) != 1 || payload.Movies[0].Name != "Movie A" {
		t.Fatalf("unexpected movie payload: %+v", payload.Movies)
	}
}

type captureRankingNotifier struct {
	payloads []notifierint.RankingNotification
}

func (n *captureRankingNotifier) NotifyRanking(data notifierint.RankingNotification) {
	n.payloads = append(n.payloads, data)
}

func TestGenerateRankingNotifiesFilteredPayload(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/emby/user_usage_stats/submit_custom_query":
			handlePlaybackQueryTestRequest(t, w, r)
		case "/emby/Items":
			if r.Method == http.MethodGet && strings.TrimSpace(r.URL.Query().Get("Ids")) != "" {
				handlePlaybackItemsTestRequest(t, w, r)
				return
			}
			handleRankingLibraryItemsRequest(t, w, r)
		case "/emby/Library/VirtualFolders/Query":
			handleRankingLibrariesRequest(t, w, r)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	t.Setenv("EMBY_URL", server.URL)
	t.Setenv("EMBY_API_KEY", "test-key")
	t.Setenv("CRON_TIMEZONE", "UTC")

	notifier := &captureRankingNotifier{}
	svc := &PlaybackRankingService{
		embyService: embyint.NewEmbyService(),
		loadLibraryAllowlist: func() ([]string, error) {
			return []string{"lib_movie_only"}, nil
		},
		notifier: notifier,
		asyncGo: func(_ string, fn func()) {
			fn()
		},
		persistRankings: func([]models.PlaybackRanking) (int64, error) {
			return 1, nil
		},
	}

	if err := svc.GenerateRanking(models.RankingWeekly, nil, nil); err != nil {
		t.Fatalf("generate ranking failed: %v", err)
	}
	if len(notifier.payloads) != 1 {
		t.Fatalf("expected one ranking payload, got %d", len(notifier.payloads))
	}
	payload := notifier.payloads[0]
	if payload.TotalDuration != 5400 {
		t.Fatalf("expected filtered totalDuration 5400, got %d", payload.TotalDuration)
	}
	if len(payload.Movies) != 1 || payload.Movies[0].Name != "星际穿越" {
		t.Fatalf("unexpected movie payload: %+v", payload.Movies)
	}
	if len(payload.Episodes) != 0 {
		t.Fatalf("expected no episode payloads, got %+v", payload.Episodes)
	}
}

func TestGenerateRankingUsesDefaultAsyncGoWhenNil(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/emby/user_usage_stats/submit_custom_query":
			handlePlaybackQueryTestRequest(t, w, r)
		case "/emby/Items":
			if r.Method == http.MethodGet && strings.TrimSpace(r.URL.Query().Get("Ids")) != "" {
				handlePlaybackItemsTestRequest(t, w, r)
				return
			}
			handleRankingLibraryItemsRequest(t, w, r)
		case "/emby/Library/VirtualFolders/Query":
			handleRankingLibrariesRequest(t, w, r)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	t.Setenv("EMBY_URL", server.URL)
	t.Setenv("EMBY_API_KEY", "test-key")
	t.Setenv("CRON_TIMEZONE", "UTC")

	notifier := &captureRankingNotifier{}
	svc := &PlaybackRankingService{
		embyService: embyint.NewEmbyService(),
		loadLibraryAllowlist: func() ([]string, error) {
			return []string{"lib_movie_only"}, nil
		},
		notifier:        notifier,
		persistRankings: func([]models.PlaybackRanking) (int64, error) { return 1, nil },
	}

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("GenerateRanking should not panic when asyncGo is nil, got %v", r)
		}
	}()

	if err := svc.GenerateRanking(models.RankingWeekly, nil, nil); err != nil {
		t.Fatalf("generate ranking failed: %v", err)
	}
}

func TestGetRankingLibraryAllowlistReportsInvalidIDs(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/emby/Library/VirtualFolders/Query":
			handleRankingLibrariesRequest(t, w, r)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	t.Setenv("EMBY_URL", server.URL)
	t.Setenv("EMBY_API_KEY", "test-key")

	svc := &PlaybackRankingService{
		embyService: embyint.NewEmbyService(),
		loadLibraryAllowlist: func() ([]string, error) {
			return []string{"lib_movie_only", "lib_missing"}, nil
		},
	}

	settings, err := svc.GetRankingLibraryAllowlist()
	if err != nil {
		t.Fatalf("get ranking allowlist: %v", err)
	}
	if settings.AllowAll {
		t.Fatalf("expected allowAll=false, got %+v", settings)
	}
	if !reflect.DeepEqual(settings.LibraryIDs, []string{"lib_movie_only"}) {
		t.Fatalf("unexpected valid library ids: %+v", settings.LibraryIDs)
	}
	if !reflect.DeepEqual(settings.InvalidLibraryIDs, []string{"lib_missing"}) {
		t.Fatalf("unexpected invalid library ids: %+v", settings.InvalidLibraryIDs)
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

func handleRankingLibrariesRequest(t *testing.T, w http.ResponseWriter, r *http.Request) {
	t.Helper()

	_ = json.NewEncoder(w).Encode(map[string]any{
		"Items": []map[string]any{
			{"Guid": "lib_movie_only", "Name": "电影库", "CollectionType": "movies", "ItemCount": 1},
			{"Guid": "lib_series_only", "Name": "剧集库", "CollectionType": "tvshows", "ItemCount": 2},
		},
	})
}

func handleRankingLibraryItemsRequest(t *testing.T, w http.ResponseWriter, r *http.Request) {
	t.Helper()

	switch strings.TrimSpace(r.URL.Query().Get("ParentId")) {
	case "lib_movie_only":
		_ = json.NewEncoder(w).Encode(map[string]any{
			"Items": []map[string]any{
				{"Id": "movie_1", "Name": "星际穿越", "Type": "Movie"},
			},
			"TotalRecordCount": 1,
		})
	case "lib_series_only":
		_ = json.NewEncoder(w).Encode(map[string]any{
			"Items": []map[string]any{
				{"Id": "ep_145", "Name": "斗罗大陆II绝世唐门 - s02e145", "Type": "Episode"},
				{"Id": "ep_144", "Name": "斗罗大陆II绝世唐门 - s02e144", "Type": "Episode"},
				{"Id": "ep_b1", "Name": "吞噬星空 - s07e215", "Type": "Episode"},
			},
			"TotalRecordCount": 3,
		})
	default:
		t.Fatalf("unexpected library parentId: %s", r.URL.Query().Get("ParentId"))
	}
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
