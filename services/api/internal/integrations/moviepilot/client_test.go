package moviepilot

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestBuildSubscribeRequestBodyIncludesSeasonForTVSeasonSubscription(t *testing.T) {
	body, err := buildSubscribeRequestBody(SubscribeRequest{
		Type:   "tv",
		Name:   "Game of Thrones",
		TmdbID: "1399",
		Season: 3,
	})
	if err != nil {
		t.Fatalf("buildSubscribeRequestBody returned error: %v", err)
	}

	if got, ok := body["type"].(string); !ok || got != "电视剧" {
		t.Fatalf("expected type to be 电视剧, got %#v", body["type"])
	}
	if got, ok := body["name"].(string); !ok || got != "Game of Thrones" {
		t.Fatalf("expected name to be preserved, got %#v", body["name"])
	}
	if got, ok := body["tmdbid"].(int); !ok || got != 1399 {
		t.Fatalf("expected tmdbid to be 1399, got %#v", body["tmdbid"])
	}
	if got, ok := body["season"].(int); !ok || got != 3 {
		t.Fatalf("expected season to be 3, got %#v", body["season"])
	}
}

func TestBuildSubscribeRequestBodyOmitsSeasonForWholeSeries(t *testing.T) {
	body, err := buildSubscribeRequestBody(SubscribeRequest{
		Type:   "tv",
		Name:   "Game of Thrones",
		TmdbID: "1399",
		Season: 0,
	})
	if err != nil {
		t.Fatalf("buildSubscribeRequestBody returned error: %v", err)
	}

	if _, exists := body["season"]; exists {
		t.Fatalf("expected season to be omitted for whole-series subscription, got %#v", body["season"])
	}
}

func TestBuildSubscribeRequestBodyRejectsInvalidTMDBID(t *testing.T) {
	_, err := buildSubscribeRequestBody(SubscribeRequest{
		Type:   "movie",
		Name:   "Inception",
		TmdbID: "bad-id",
	})
	if err == nil {
		t.Fatal("expected invalid tmdb id to return error")
	}
}

func TestToMoviePilotMediaTypeUsesMoviePilotEnumValues(t *testing.T) {
	tests := []struct {
		name      string
		mediaType string
		want      string
	}{
		{name: "movie", mediaType: "movie", want: "电影"},
		{name: "tv", mediaType: "tv", want: "电视剧"},
		{name: "trim and normalize", mediaType: " TV ", want: "电视剧"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := toMoviePilotMediaType(tt.mediaType)
			if err != nil {
				t.Fatalf("toMoviePilotMediaType returned error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("expected %s, got %s", tt.want, got)
			}
		})
	}
}

func TestToMoviePilotMediaTypeRejectsUnknownType(t *testing.T) {
	if _, err := toMoviePilotMediaType("collection"); err == nil {
		t.Fatal("expected unknown media type to return error")
	}
}

func TestCreateSubscriptionUsesXAPIKeyHeader(t *testing.T) {
	t.Setenv("MOVIEPILOT_URL", "http://moviepilot.test")
	t.Setenv("MOVIEPILOT_API_KEY", "test-key")

	client := NewMoviePilotClient()
	client.client = &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.Method != http.MethodPost {
			t.Fatalf("expected POST method, got %s", req.Method)
		}
		if req.URL.String() != "http://moviepilot.test/api/v1/subscribe/" {
			t.Fatalf("unexpected request url: %s", req.URL.String())
		}
		if req.Header.Get("X-API-KEY") != "test-key" {
			t.Fatalf("expected X-API-KEY header to be set")
		}
		if got := req.Header.Get("Authorization"); got != "" {
			t.Fatalf("expected Authorization header to be empty, got %s", got)
		}

		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(`{"ok":true}`)),
			Header:     make(http.Header),
		}, nil
	})}

	err := client.CreateSubscription(SubscribeRequest{
		Type:   "movie",
		Name:   "Inception",
		TmdbID: "27205",
	})
	if err != nil {
		t.Fatalf("expected create subscription to succeed, got %v", err)
	}
}

func TestSearchMediaCandidatesUsesTMDBMediaEndpoint(t *testing.T) {
	t.Setenv("MOVIEPILOT_URL", "http://moviepilot.test")
	t.Setenv("MOVIEPILOT_API_KEY", "test-key")

	client := NewMoviePilotClient()
	client.client = &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.Method != http.MethodGet {
			t.Fatalf("expected GET method, got %s", req.Method)
		}
		if req.URL.Path != "/api/v1/search/media/tmdb:1399" {
			t.Fatalf("unexpected request path: %s", req.URL.Path)
		}
		if req.URL.Query().Get("mtype") != "电视剧" || req.URL.Query().Get("season") != "2" {
			t.Fatalf("unexpected query: %s", req.URL.RawQuery)
		}
		if req.Header.Get("X-API-KEY") != "test-key" {
			t.Fatalf("expected X-API-KEY header to be set")
		}

		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(`[{"title":"Show S02","site":"MTeam"}]`)),
			Header:     make(http.Header),
		}, nil
	})}

	resp, err := client.SearchMediaCandidates(SearchMediaRequest{
		TmdbID:    "1399",
		MediaType: "tv",
		Season:    2,
	})
	if err != nil {
		t.Fatalf("expected search to succeed, got %v", err)
	}
	if len(resp.Candidates) != 1 || resp.Candidates[0].Title != "Show S02" {
		t.Fatalf("unexpected candidates: %#v", resp.Candidates)
	}
}

func TestDispatchDownloadCandidateIncludesTMDBID(t *testing.T) {
	t.Setenv("MOVIEPILOT_URL", "http://moviepilot.test")
	t.Setenv("MOVIEPILOT_API_KEY", "test-key")

	client := NewMoviePilotClient()
	client.client = &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.Method != http.MethodPost {
			t.Fatalf("expected POST method, got %s", req.Method)
		}
		if req.URL.Path != "/api/v1/download/add" {
			t.Fatalf("unexpected request path: %s", req.URL.Path)
		}

		var body map[string]interface{}
		if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		if got := body["tmdbid"]; got != float64(27205) {
			t.Fatalf("expected tmdbid 27205, got %#v", got)
		}
		if got := body["season"]; got != float64(2) {
			t.Fatalf("expected season 2, got %#v", got)
		}
		torrent, ok := body["torrent_in"].(map[string]interface{})
		if !ok || torrent["title"] != "Inception" {
			t.Fatalf("unexpected torrent_in: %#v", body["torrent_in"])
		}

		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(`{"message":"ok"}`)),
			Header:     make(http.Header),
		}, nil
	})}

	resp, err := client.DispatchDownloadCandidate(DownloadCandidateRequest{
		TmdbID: "27205",
		Season: 2,
		CandidatePayload: map[string]interface{}{
			"title": "Inception",
		},
	})
	if err != nil {
		t.Fatalf("expected dispatch to succeed, got %v", err)
	}
	if resp.Message != "ok" {
		t.Fatalf("expected response message ok, got %q", resp.Message)
	}
}

func TestNormalizeGapCandidatesPreservesPublishDate(t *testing.T) {
	results := []map[string]interface{}{
		{
			"title":        "完美世界 S01E10",
			"publish_date": "2026-04-18 20:30:00",
			"site":         "MTeam",
			"size":         2048,
			"seeders":      8,
		},
	}

	candidates := normalizeGapCandidates(results, "episode")
	if len(candidates) != 1 {
		t.Fatalf("expected 1 candidate, got %d", len(candidates))
	}
	if candidates[0].PublishDate != "2026-04-18 20:30:00" {
		t.Fatalf("unexpected publish date: %s", candidates[0].PublishDate)
	}
}

// TestDispatchDownloadCandidateRejectsBusinessFailure 覆盖 P1-3：
// MoviePilot 在业务失败时 HTTP 仍返回 200，仅在响应顶层标记 success=false。
// 客户端必须解析该字段并返回错误，不能误判成 Accepted:true。
func TestDispatchDownloadCandidateRejectsBusinessFailure(t *testing.T) {
	t.Setenv("MOVIEPILOT_URL", "http://moviepilot.test")
	t.Setenv("MOVIEPILOT_API_KEY", "test-key")

	client := NewMoviePilotClient()
	client.client = &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.URL.Path != "/api/v1/download/add" {
			t.Fatalf("unexpected request path: %s", req.URL.Path)
		}
		// 模拟 MoviePilot 重复添加场景：HTTP 200 + success=false + message。
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(`{"success":false,"message":"重复添加","data":null}`)),
			Header:     make(http.Header),
		}, nil
	})}

	resp, err := client.DispatchDownloadCandidate(DownloadCandidateRequest{
		TmdbID: "27205",
		CandidatePayload: map[string]interface{}{
			"title": "Inception",
		},
	})
	if err == nil {
		t.Fatal("expected business failure to return error, got nil")
	}
	if resp == nil {
		t.Fatal("expected response payload even on business failure")
	}
	if resp.Accepted {
		t.Fatalf("expected Accepted=false on business failure, got %+v", resp)
	}
	// 新-2：业务拒绝必须以 sentinel ErrMoviePilotBusinessRejected 暴露给上层，
	// 让 service / handler 通过 errors.Is 区分业务拒绝与基础设施故障。
	if !errors.Is(err, ErrMoviePilotBusinessRejected) {
		t.Fatalf("expected error to wrap ErrMoviePilotBusinessRejected, got %v", err)
	}
	if !strings.Contains(err.Error(), "重复添加") {
		// 业务错误信息应来自 message 字段，便于管理员看到上游拒绝原因。
		t.Fatalf("expected error to expose business message, got %q", err.Error())
	}
	if strings.Contains(err.Error(), "27205") || strings.Contains(err.Error(), "moviepilot.test") {
		// 错误信息不应透传 TMDB ID 或上游 URL（仍属脱敏范畴）。
		t.Fatalf("error must not leak tmdbId or upstream url, got %q", err.Error())
	}
}

// TestDispatchDownloadCandidateFallsBackToDefaultMessageWhenMissing 覆盖：
// success=false 但 message 缺失时，应使用固定文案兜底，而不是返回空错误信息。
func TestDispatchDownloadCandidateFallsBackToDefaultMessageWhenMissing(t *testing.T) {
	t.Setenv("MOVIEPILOT_URL", "http://moviepilot.test")
	t.Setenv("MOVIEPILOT_API_KEY", "test-key")

	client := NewMoviePilotClient()
	client.client = &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(`{"success":false}`)),
			Header:     make(http.Header),
		}, nil
	})}

	resp, err := client.DispatchDownloadCandidate(DownloadCandidateRequest{
		TmdbID: "27205",
		CandidatePayload: map[string]interface{}{
			"title": "Inception",
		},
	})
	if err == nil {
		t.Fatal("expected business failure to return error, got nil")
	}
	if resp.Accepted {
		t.Fatalf("expected Accepted=false on business failure, got %+v", resp)
	}
	// 缺省 message 也应使用 sentinel 包装，便于上层 errors.Is 判定。
	if !errors.Is(err, ErrMoviePilotBusinessRejected) {
		t.Fatalf("expected fallback error to wrap ErrMoviePilotBusinessRejected, got %v", err)
	}
	if !strings.Contains(err.Error(), "MoviePilot 拒绝下载请求") {
		t.Fatalf("expected default fallback message, got %q", err.Error())
	}
}

// TestDispatchDownloadCandidateAcceptsSuccessResponse 覆盖：
// success=true 时维持原成功路径，Accepted=true 且不返回错误。
func TestDispatchDownloadCandidateAcceptsSuccessResponse(t *testing.T) {
	t.Setenv("MOVIEPILOT_URL", "http://moviepilot.test")
	t.Setenv("MOVIEPILOT_API_KEY", "test-key")

	client := NewMoviePilotClient()
	client.client = &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(`{"success":true,"data":{"download_id":"abc123"}}`)),
			Header:     make(http.Header),
		}, nil
	})}

	resp, err := client.DispatchDownloadCandidate(DownloadCandidateRequest{
		TmdbID: "27205",
		CandidatePayload: map[string]interface{}{
			"title": "Inception",
		},
	})
	if err != nil {
		t.Fatalf("expected success response to be accepted, got %v", err)
	}
	if !resp.Accepted {
		t.Fatalf("expected Accepted=true on success response, got %+v", resp)
	}
	if downloadID, _ := resp.Response["data"].(map[string]interface{})["download_id"].(string); downloadID != "abc123" {
		t.Fatalf("expected data.download_id to be preserved in response, got %+v", resp.Response)
	}
}

// TestDispatchDownloadCandidateKeepsLegacyBehaviourWhenSuccessMissing 覆盖：
// 响应体不含 success 字段（保守处理缺省），维持原 Accepted:true 行为，
// 避免 MoviePilot 非标准响应导致误判。
func TestDispatchDownloadCandidateKeepsLegacyBehaviourWhenSuccessMissing(t *testing.T) {
	t.Setenv("MOVIEPILOT_URL", "http://moviepilot.test")
	t.Setenv("MOVIEPILOT_API_KEY", "test-key")

	client := NewMoviePilotClient()
	client.client = &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(`{"message":"ok"}`)),
			Header:     make(http.Header),
		}, nil
	})}

	resp, err := client.DispatchDownloadCandidate(DownloadCandidateRequest{
		TmdbID: "27205",
		CandidatePayload: map[string]interface{}{
			"title": "Inception",
		},
	})
	if err != nil {
		t.Fatalf("expected legacy response without success to be accepted, got %v", err)
	}
	if !resp.Accepted {
		t.Fatalf("expected Accepted=true when success missing, got %+v", resp)
	}
	if resp.Message != "ok" {
		t.Fatalf("expected message preserved, got %q", resp.Message)
	}
}

func TestLookupResponseSuccess(t *testing.T) {
	cases := []struct {
		name      string
		parsed    map[string]interface{}
		wantHave  bool
		wantValue bool
	}{
		{name: "nil map", parsed: nil, wantHave: false},
		{name: "empty map", parsed: map[string]interface{}{}, wantHave: false},
		{name: "success missing", parsed: map[string]interface{}{"data": 1}, wantHave: false},
		{name: "success true bool", parsed: map[string]interface{}{"success": true}, wantHave: true, wantValue: true},
		{name: "success false bool", parsed: map[string]interface{}{"success": false}, wantHave: true, wantValue: false},
		{name: "success string true", parsed: map[string]interface{}{"success": "True"}, wantHave: true, wantValue: true},
		{name: "success string false", parsed: map[string]interface{}{"success": "FALSE"}, wantHave: true, wantValue: false},
		{name: "success other type ignored", parsed: map[string]interface{}{"success": 1}, wantHave: false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			have, value := lookupResponseSuccess(tc.parsed)
			if have != tc.wantHave || value != tc.wantValue {
				t.Fatalf("expected (have=%v value=%v), got (have=%v value=%v)", tc.wantHave, tc.wantValue, have, value)
			}
		})
	}
}
