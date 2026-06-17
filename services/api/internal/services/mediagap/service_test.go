package mediagap

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/konghang/ember/backend/internal/common/tmdbcache"
	"github.com/konghang/ember/backend/internal/db"
	embyint "github.com/konghang/ember/backend/internal/integrations/emby"
	moviepilotint "github.com/konghang/ember/backend/internal/integrations/moviepilot"
	"github.com/konghang/ember/backend/internal/models"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestIsScannableSeriesRequiresPhysicalSeriesAndTmdbID(t *testing.T) {
	cases := []struct {
		name              string
		item              embySeriesItem
		requireContinuing bool
		want              bool
	}{
		{
			name: "virtual series skipped",
			item: embySeriesItem{
				LocationType: " Virtual ",
				ProviderIDs:  map[string]string{"Tmdb": "1399"},
			},
			want: false,
		},
		{
			name: "missing tmdb id skipped",
			item: embySeriesItem{
				ProviderIDs: map[string]string{"Imdb": "tt0944947"},
			},
			want: false,
		},
		{
			name: "status ignored when continuing not required",
			item: embySeriesItem{
				Status:      "Ended",
				ProviderIDs: map[string]string{" tmdb ": " 1399 "},
			},
			want: true,
		},
		{
			name: "blank status accepted when continuing required",
			item: embySeriesItem{
				ProviderIDs: map[string]string{"Tmdb": "1399"},
			},
			requireContinuing: true,
			want:              true,
		},
		{
			name: "series status continuing accepted",
			item: embySeriesItem{
				SeriesStatus: "Continuing",
				ProviderIDs:  map[string]string{"Tmdb": "1399"},
			},
			requireContinuing: true,
			want:              true,
		},
		{
			name: "ended status rejected when continuing required",
			item: embySeriesItem{
				Status:      "Ended",
				ProviderIDs: map[string]string{"Tmdb": "1399"},
			},
			requireContinuing: true,
			want:              false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := isScannableSeries(tc.item, tc.requireContinuing); got != tc.want {
				t.Fatalf("expected %v, got %v", tc.want, got)
			}
		})
	}
}

func TestHasPhysicalEpisodeMedia(t *testing.T) {
	cases := []struct {
		name string
		item embyEpisodeItem
		want bool
	}{
		{
			name: "virtual episode skipped",
			item: embyEpisodeItem{LocationType: " Virtual ", Path: "/media/show.mkv"},
			want: false,
		},
		{
			name: "missing episode skipped",
			item: embyEpisodeItem{IsMissing: true, Path: "/media/show.mkv"},
			want: false,
		},
		{
			name: "path marks physical media",
			item: embyEpisodeItem{Path: " /media/show.mkv "},
			want: true,
		},
		{
			name: "media source marks physical media",
			item: embyEpisodeItem{MediaSources: []embyint.EmbyMediaSource{{}}},
			want: true,
		},
		{
			name: "empty media skipped",
			item: embyEpisodeItem{},
			want: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := hasPhysicalEpisodeMedia(tc.item); got != tc.want {
				t.Fatalf("expected %v, got %v", tc.want, got)
			}
		})
	}
}

func TestParseTMDBAirDate(t *testing.T) {
	parsed, ok := parseTMDBAirDate(" 2026-04-19 ")
	if !ok {
		t.Fatal("expected valid TMDB air date")
	}
	want := time.Date(2026, 4, 19, 0, 0, 0, 0, time.UTC)
	if !parsed.Equal(want) {
		t.Fatalf("expected %s, got %s", want, parsed)
	}

	for _, value := range []string{"", "2026/04/19", "not-a-date"} {
		t.Run(value, func(t *testing.T) {
			if got, ok := parseTMDBAirDate(value); ok || !got.IsZero() {
				t.Fatalf("expected invalid date to return zero false, got %s %v", got, ok)
			}
		})
	}
}

func TestEpisodeInventoryAndSeasonSet(t *testing.T) {
	inventory := episodeInventory{
		1: {1: {}, 2: {}},
		2: {},
		0: {1: {}},
	}

	if !inventory.has(1, 2) {
		t.Fatal("expected inventory to contain season 1 episode 2")
	}
	if inventory.has(0, 1) || inventory.has(1, 0) || inventory.has(2, 1) || inventory.has(3, 1) {
		t.Fatal("expected invalid or absent inventory lookups to be false")
	}

	active := inventory.activeSeasons()
	if !active.contains(1) {
		t.Fatal("expected season 1 to be active")
	}
	if active.contains(0) || active.contains(2) || active.contains(3) {
		t.Fatalf("unexpected active seasons: %+v", active)
	}
	if (seasonSet{}).contains(1) {
		t.Fatal("empty season set must not contain any season")
	}
}

func TestFetchTMDBJSONDeduplicatesInflightRequests(t *testing.T) {
	var requestCount atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount.Add(1)
		time.Sleep(50 * time.Millisecond)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":100}`))
	}))
	defer server.Close()

	service := &Service{
		httpClient: server.Client(),
		tmdbCache:  tmdbcache.NewStore(),
	}

	var wg sync.WaitGroup
	start := make(chan struct{})
	run := func() {
		defer wg.Done()
		<-start
		var out map[string]any
		if err := service.fetchTMDBJSON(context.Background(), "detail:100", server.URL, time.Minute, true, &out); err != nil {
			t.Errorf("fetchTMDBJSON returned error: %v", err)
			return
		}
		if got := int(out["id"].(float64)); got != 100 {
			t.Errorf("expected id 100, got %d", got)
		}
	}

	wg.Add(2)
	go run()
	go run()
	close(start)
	wg.Wait()

	if got := requestCount.Load(); got != 1 {
		t.Fatalf("expected exactly 1 upstream request, got %d", got)
	}
}

// stubGapMoviePilotClient 实现 gapMoviePilotClient 接口，用于 DispatchGap 单测注入。
// 通过 dispatchFn 控制 DispatchGapCandidate 的返回，模拟 MoviePilot 业务拒绝 / 基础设施故障等场景。
type stubGapMoviePilotClient struct {
	dispatchFn func(moviepilotint.GapDispatchRequest) (*moviepilotint.GapDispatchResponse, error)
	searchFn   func(moviepilotint.GapSearchRequest) (*moviepilotint.GapSearchResponse, error)
}

func (s *stubGapMoviePilotClient) SearchGapCandidates(req moviepilotint.GapSearchRequest) (*moviepilotint.GapSearchResponse, error) {
	if s.searchFn == nil {
		return nil, nil
	}
	return s.searchFn(req)
}

func (s *stubGapMoviePilotClient) DispatchGapCandidate(req moviepilotint.GapDispatchRequest) (*moviepilotint.GapDispatchResponse, error) {
	if s.dispatchFn == nil {
		return nil, nil
	}
	return s.dispatchFn(req)
}

// newDispatchGapTestService 构造一个注入了 mock MoviePilot client 的 Service，
// 并通过 swapLoadGapByIDFunc 让 DispatchGap 内的 loadGapByID 不再依赖真实 DB。
//
// DryRun DB 用于拦截 DispatchGap 失败分支里的 db.DB.WithContext().Updates，
// 让 SQL 只生成不执行（避免连真实 PostgreSQL）。本测试聚焦"返回的 error 是否透传业务原因"
// 与"DISPATCH_FAILED 写入是否触发"，真实 DB 状态机字段断言由集成测试覆盖。
func newDispatchGapTestService(t *testing.T, dispatchFn func(moviepilotint.GapDispatchRequest) (*moviepilotint.GapDispatchResponse, error)) (*Service, *gorm.DB) {
	t.Helper()
	database, err := gorm.Open(postgres.New(postgres.Config{
		DSN:                  "host=127.0.0.1 user=test dbname=test sslmode=disable",
		PreferSimpleProtocol: true,
	}), &gorm.Config{
		DryRun:               true,
		DisableAutomaticPing: true,
		Logger:               logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("open dry-run database: %v", err)
	}
	return &Service{
		moviepilot: &stubGapMoviePilotClient{dispatchFn: dispatchFn},
		tmdbCache:  tmdbcache.NewStore(),
	}, database
}

// swapLoadGapByIDFunc 临时替换 loadGapByIDFunc 包级变量，让 DispatchGap 内的 loadGapByID
// 不再查询真实 DB。回传恢复函数，由调用方 defer 执行。
func swapLoadGapByIDFunc(gap models.MediaGap) func() {
	original := loadGapByIDFunc
	loadGapByIDFunc = func(ctx context.Context, id string) (models.MediaGap, error) {
		if gap.ID != "" && gap.ID != id {
			return models.MediaGap{}, ErrMediaGapNotFound
		}
		return gap, nil
	}
	return func() { loadGapByIDFunc = original }
}

// swapGlobalDB 临时把 db.DB 替换为 DryRun 数据库，避免 DispatchGap 失败分支里的
// db.DB.WithContext().Updates 调用真实 PostgreSQL。测试结束自动恢复。
func swapGlobalDB(t *testing.T, database *gorm.DB) {
	t.Helper()
	original := db.DB
	db.DB = database
	t.Cleanup(func() { db.DB = original })
}

// TestDispatchGapBusinessRejectionTransparentlyPropagatesReason 覆盖新-2 / 新-3：
// MoviePilot 返回业务拒绝（重复添加等）时，mediagap DispatchGap 必须把业务原因
// 透传给调用方，而不是被 SafeUpstreamError 截断成 "upstream moviepilot unavailable"。
//
// DryRun DB 只能拦截 SQL 生成；本用例聚焦"返回 error 是否携带业务原因"这一关键断言，
// 状态机字段的 DB 写入由集成测试覆盖。
func TestDispatchGapBusinessRejectionTransparentlyPropagatesReason(t *testing.T) {
	restoreGap := swapLoadGapByIDFunc(dispatchGapGapFixture())
	defer restoreGap()

	service, database := newDispatchGapTestService(t, func(req moviepilotint.GapDispatchRequest) (*moviepilotint.GapDispatchResponse, error) {
		return &moviepilotint.GapDispatchResponse{Accepted: false, StatusCode: 200, Message: "重复添加"},
			fmt.Errorf("%w: %s", moviepilotint.ErrMoviePilotBusinessRejected, "重复添加")
	})
	swapGlobalDB(t, database)

	ctx := context.Background()
	_, err := service.DispatchGap(ctx, "gap_test", DispatchRequest{
		Candidate: SearchCandidate{Payload: map[string]interface{}{"title": "Show S01"}},
	})
	if err == nil {
		t.Fatal("expected business rejection to return error, got nil")
	}
	if !errors.Is(err, moviepilotint.ErrMoviePilotBusinessRejected) {
		t.Fatalf("expected error to wrap ErrMoviePilotBusinessRejected, got %v", err)
	}
	// 业务原因（重复添加）必须透传，不能被替换成基础设施 unavailable 文案。
	if !strings.Contains(err.Error(), "重复添加") {
		t.Fatalf("expected error to expose business reason, got %q", err.Error())
	}
	if strings.Contains(err.Error(), "unavailable") {
		t.Fatalf("business rejection must not be masked as infrastructure unavailable, got %q", err.Error())
	}
}

// TestDispatchGapInfrastructureErrorFallsBackToSafeUpstream 覆盖新-2 对称分支：
// 非 sentinel（基础设施错误）仍走 SafeUpstreamError 脱敏，避免回写 URL/凭证。
func TestDispatchGapInfrastructureErrorFallsBackToSafeUpstream(t *testing.T) {
	restoreGap := swapLoadGapByIDFunc(dispatchGapGapFixture())
	defer restoreGap()

	service, database := newDispatchGapTestService(t, func(req moviepilotint.GapDispatchRequest) (*moviepilotint.GapDispatchResponse, error) {
		return nil, errors.New("connection refused")
	})
	swapGlobalDB(t, database)

	ctx := context.Background()
	_, err := service.DispatchGap(ctx, "gap_test", DispatchRequest{
		Candidate: SearchCandidate{Payload: map[string]interface{}{"title": "Show S01"}},
	})
	if err == nil {
		t.Fatal("expected infrastructure error to return error, got nil")
	}
	if errors.Is(err, moviepilotint.ErrMoviePilotBusinessRejected) {
		t.Fatalf("plain infra error must not be tagged as business rejection, got %v", err)
	}
	if !strings.Contains(err.Error(), "unavailable") {
		t.Fatalf("expected SafeUpstreamError fallback message, got %q", err.Error())
	}
	// 脱敏：原始基础设施错误细节不应泄漏。
	if strings.Contains(err.Error(), "connection refused") {
		t.Fatalf("infrastructure error detail must be sanitized, got %q", err.Error())
	}
}

func TestApplyMediaGapWebhookIngestTransitionUpdatesMissingGap(t *testing.T) {
	gap := models.MediaGap{
		ID:           "gap_1",
		TmdbID:       "1399",
		EmbySeriesID: "old_series",
		Season:       1,
		Episode:      2,
		Status:       models.MediaGapStatusMissing,
	}
	now := time.Date(2026, 6, 17, 12, 0, 0, 0, time.UTC)

	updated, changed := applyWebhookIngestTransition(&gap, "new_series", now)

	if !changed {
		t.Fatal("expected missing gap to be marked changed")
	}
	if updated.Status != models.MediaGapStatusIngested {
		t.Fatalf("expected status INGESTED, got %s", updated.Status)
	}
	if updated.IngestedAt == nil || !updated.IngestedAt.Equal(now) {
		t.Fatalf("expected ingestedAt %s, got %+v", now, updated.IngestedAt)
	}
	if updated.EmbySeriesID != "new_series" {
		t.Fatalf("expected series id to be updated, got %q", updated.EmbySeriesID)
	}
}

func TestApplyMediaGapWebhookIngestTransitionKeepsIgnoredGap(t *testing.T) {
	ignoredAt := time.Date(2026, 6, 16, 12, 0, 0, 0, time.UTC)
	reasonCode := string(models.MediaGapIgnoreReasonManual)
	gap := models.MediaGap{
		ID:               "gap_1",
		TmdbID:           "1399",
		EmbySeriesID:     "old_series",
		Season:           1,
		Episode:          2,
		Status:           models.MediaGapStatusIgnored,
		IgnoredAt:        &ignoredAt,
		IgnoreReasonCode: &reasonCode,
		IgnoreReason:     "管理员确认无资源",
	}

	updated, changed := applyWebhookIngestTransition(&gap, "new_series", time.Now().UTC())

	if changed {
		t.Fatal("expected ignored gap to remain unchanged")
	}
	if updated.Status != models.MediaGapStatusIgnored || updated.EmbySeriesID != "old_series" {
		t.Fatalf("ignored gap was modified: %+v", updated)
	}
	if updated.IgnoredAt != &ignoredAt || updated.IgnoreReasonCode != &reasonCode || updated.IgnoreReason != "管理员确认无资源" {
		t.Fatalf("ignored metadata must be preserved, got %+v", updated)
	}
}

func TestApplyMediaGapWebhookIngestTransitionIsIdempotent(t *testing.T) {
	ingestedAt := time.Date(2026, 6, 16, 12, 0, 0, 0, time.UTC)
	gap := models.MediaGap{
		ID:           "gap_1",
		EmbySeriesID: "series_1",
		Status:       models.MediaGapStatusIngested,
		IngestedAt:   &ingestedAt,
	}

	updated, changed := applyWebhookIngestTransition(&gap, "series_1", time.Date(2026, 6, 17, 12, 0, 0, 0, time.UTC))

	if changed {
		t.Fatal("expected already ingested gap with same series id to be unchanged")
	}
	if updated.IngestedAt != &ingestedAt {
		t.Fatalf("expected original ingestedAt pointer to be preserved, got %+v", updated.IngestedAt)
	}
}

func TestApplyMediaGapWebhookIngestTransitionFillsMissingIngestedAt(t *testing.T) {
	now := time.Date(2026, 6, 17, 12, 0, 0, 0, time.UTC)
	gap := models.MediaGap{
		ID:           "gap_1",
		EmbySeriesID: "series_1",
		Status:       models.MediaGapStatusIngested,
	}

	updated, changed := applyWebhookIngestTransition(&gap, "", now)

	if !changed {
		t.Fatal("expected missing ingestedAt to be filled")
	}
	if updated.Status != models.MediaGapStatusIngested {
		t.Fatalf("expected status to stay INGESTED, got %s", updated.Status)
	}
	if updated.IngestedAt == nil || !updated.IngestedAt.Equal(now) {
		t.Fatalf("expected ingestedAt %s, got %+v", now, updated.IngestedAt)
	}
	if updated.EmbySeriesID != "series_1" {
		t.Fatalf("blank webhook series id must not clear existing series id, got %q", updated.EmbySeriesID)
	}
}

// dispatchGapGapFixture 用于断言状态机写入所需的最小 gap 行结构。
// loadGapByID 的 stub 直接返回这份 fixture，让 DispatchGap 越过状态前置校验，
// 进入 moviepilot 调用与错误处理分支。
func dispatchGapGapFixture() models.MediaGap {
	return models.MediaGap{
		ID:         "gap_test",
		TmdbID:     "1399",
		Season:     1,
		Episode:    1,
		Status:     models.MediaGapStatusMissing,
		SeriesName: "Show",
	}
}
