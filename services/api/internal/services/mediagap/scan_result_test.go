package mediagap

import (
	"context"
	"errors"
	"github.com/DATA-DOG/go-sqlmock"
	"github.com/konghang/ember/backend/internal/common/tmdbcache"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

// TestScanSeriesBatchResults 锁定完整、部分失败和取消的计数及安全错误输出。
func TestScanSeriesBatchResults(t *testing.T) {
	items := []embySeriesItem{{ID: "ok", Name: "成功剧", ProviderIDs: map[string]string{"Tmdb": "1"}}, {ID: "bad", Name: "失败剧", ProviderIDs: map[string]string{"Tmdb": "2"}}}
	result := scanSeriesBatch(context.Background(), items, func(item embySeriesItem) (*scanSeriesStats, error) {
		if item.ID == "bad" {
			return nil, errors.New("private upstream response")
		}
		return &scanSeriesStats{Created: 2, Examined: 5}, nil
	})
	if result.ScannedSeries != 1 || result.SkippedSeries != 1 || result.Created != 2 || len(result.Failures) != 1 || result.Failures[0].TmdbID != "2" || result.Failures[0].Reason != "剧集扫描失败，请重试" {
		t.Fatalf("result=%+v", result)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	result = scanSeriesBatch(ctx, items, func(embySeriesItem) (*scanSeriesStats, error) {
		t.Fatal("canceled scan invoked dependency")
		return nil, nil
	})
	if result.ScannedSeries != 0 || result.SkippedSeries != 2 {
		t.Fatalf("result=%+v", result)
	}
	result = scanSeriesBatch(context.Background(), items[:1], func(embySeriesItem) (*scanSeriesStats, error) {
		return nil, &ScanFailure{TmdbID: "1", Season: 2, Reason: "季元数据获取失败"}
	})
	if result.SkippedSeries != 1 || result.Failures[0].Season != 2 {
		t.Fatalf("result=%+v", result)
	}
}

// fixtureGapEmby 提供已激活季的最小库存，禁止访问真实 Emby。
type fixtureGapEmby struct{}

// IsConfigured 返回 fixture 已就绪。
func (fixtureGapEmby) IsConfigured() bool { return true }

// GetWithAPIKey 返回一个真实文件条目的固定库存。
func (fixtureGapEmby) GetWithAPIKey(string, map[string]string) ([]byte, error) {
	return []byte(`{"Items":[{"Id":"ep1","ParentIndexNumber":1,"IndexNumber":1,"Path":"/fixture/ep1.mkv"}],"TotalRecordCount":1}`), nil
}

type gapRoundTripper func(*http.Request) (*http.Response, error)

// RoundTrip 只使用调用方提供的内存响应。
func (f gapRoundTripper) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

// TestSeasonFetchFailureStopsSeries 验证实际单剧扫描不会把季元数据失败吞成成功，也不据此修改工单。
func TestSeasonFetchFailureStopsSeries(t *testing.T) {
	swapGlobalDB(t, nil)
	svc := &Service{embyService: fixtureGapEmby{}, tmdbCache: tmdbcache.NewStore(), httpClient: &http.Client{Transport: gapRoundTripper(func(r *http.Request) (*http.Response, error) {
		if strings.Contains(r.URL.Path, "/season/") {
			return nil, errors.New("fake season unavailable")
		}
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"id":1,"name":"Demo","seasons":[{"season_number":1}]}`))}, nil
	})}}
	if _, err := svc.fetchTVDetail(context.Background(), "1", true); err != nil {
		t.Fatal(err)
	}
	mock := newGapSQLMock(t)
	mock.ExpectQuery(`SELECT .* FROM "media_gaps"`).WillReturnRows(sqlmock.NewRows([]string{"id"}))
	mock.ExpectQuery(`SELECT .* FROM "tmdb_cache"`).WillReturnRows(sqlmock.NewRows([]string{"cache_key"}))
	_, err := svc.scanSingleSeries(context.Background(), embySeriesItem{ID: "series", Name: "Demo", ProviderIDs: map[string]string{"Tmdb": "1"}}, time.Now(), time.Now(), false)
	var failure *ScanFailure
	if !errors.As(err, &failure) || failure.Season != 1 {
		t.Fatalf("expected season failure, got %v", err)
	}
}
