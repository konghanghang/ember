package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sync/atomic"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/konghang/ember/backend/internal/common/tmdbcache"
)

func TestBuildTMDBTVSeasonOptionsUsesExplicitSeasonList(t *testing.T) {
	options := buildTMDBTVSeasonOptions(TMDBTVDetailResponse{
		ID:              1399,
		Name:            "Game of Thrones",
		NumberOfSeasons: 8,
		Seasons: []TMDBTVSeason{
			{SeasonNumber: 0},
			{SeasonNumber: 1},
			{SeasonNumber: 2},
			{SeasonNumber: 2},
			{SeasonNumber: 8},
		},
	})

	expected := []int{1, 2, 8}
	if !reflect.DeepEqual(options.Seasons, expected) {
		t.Fatalf("expected seasons %v, got %v", expected, options.Seasons)
	}
	if options.NumberOfSeasons != 8 {
		t.Fatalf("expected numberOfSeasons to be 8, got %d", options.NumberOfSeasons)
	}
}

func TestBuildTMDBTVSeasonOptionsFallsBackToSeasonCount(t *testing.T) {
	options := buildTMDBTVSeasonOptions(TMDBTVDetailResponse{
		ID:              66732,
		Name:            "Stranger Things",
		NumberOfSeasons: 4,
	})

	expected := []int{1, 2, 3, 4}
	if !reflect.DeepEqual(options.Seasons, expected) {
		t.Fatalf("expected seasons %v, got %v", expected, options.Seasons)
	}
}

func TestTMDBHandlerSearchReturnsGenericInternalErrorWhenAPIKeyMissing(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Setenv("TMDB_API_KEY", "")

	handler := NewTMDBHandler()
	ctx, recorder := newTestConfigContext(http.MethodGet, "/api/v1/tmdb/search?query=inception&type=movie", nil)
	handler.Search(ctx)

	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("expected status 500, got %d", recorder.Code)
	}

	var resp struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp.Error != "上游服务暂不可用" {
		t.Fatalf("expected generic internal error message, got %q", resp.Error)
	}
}

func TestTMDBHandlerSearchUsesTMDBCache(t *testing.T) {
	gin.SetMode(gin.TestMode)
	t.Setenv("TMDB_API_KEY", "test-key")

	var requestCount atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount.Add(1)
		if got := r.URL.Query().Get("api_key"); got != "test-key" {
			t.Errorf("api_key = %q, want test-key", got)
			http.Error(w, "bad api_key", http.StatusInternalServerError)
			return
		}
		if got := r.URL.Query().Get("language"); got != "zh-CN" {
			t.Errorf("language = %q, want zh-CN", got)
			http.Error(w, "bad language", http.StatusInternalServerError)
			return
		}
		if got := r.URL.Query().Get("query"); got != "inception" {
			t.Errorf("query = %q, want inception", got)
			http.Error(w, "bad query", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"results":[{"id":27205,"title":"Inception","overview":""}],"total_results":1}`))
	}))
	defer server.Close()

	handler := NewTMDBHandler()
	handler.baseURL = server.URL
	handler.httpClient = server.Client()
	handler.cacheStore = tmdbcache.NewStore()

	for i := 0; i < 2; i++ {
		ctx, recorder := newTestConfigContext(http.MethodGet, "/api/v1/tmdb/search?query=inception&type=movie", nil)
		handler.Search(ctx)
		if recorder.Code != http.StatusOK {
			t.Fatalf("request %d expected status 200, got %d body=%s", i+1, recorder.Code, recorder.Body.String())
		}
	}

	if got := requestCount.Load(); got != 1 {
		t.Fatalf("expected one upstream request after cache reuse, got %d", got)
	}
}
