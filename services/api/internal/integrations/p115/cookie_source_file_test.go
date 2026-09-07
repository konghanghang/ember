package p115

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
)

func TestCookieHTTPAdapterResolveFileByPathResolvesParentPathOnce(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet {
			t.Fatalf("unexpected source resolve method: %s", request.Method)
		}
		assertCookieAdapterHeaders(t, request)
		calls.Add(1)
		switch request.URL.Path {
		case "/files/get_path_id":
			query := request.URL.Query()
			if len(query) != 3 || query.Get("parent_id") != "100" || query.Get("path") != "Movies/科幻 (2026) {tmdb-1}" ||
				query.Get("is_create") != "0" {
				t.Fatalf("unexpected source path query: %v", query)
			}
			_, _ = w.Write([]byte(`{"state":true,"data":{"file_id":"300"}}`))
		case "/files":
			assertSourceListQuery(t, request, sourceResolvePageSize)
			if request.URL.Query().Get("cid") != "300" {
				t.Fatalf("unexpected source directory cid=%s", request.URL.Query().Get("cid"))
			}
			_, _ = w.Write([]byte(`{"state":true,"cid":300,"count":1,"offset":0,"data":[` +
				`{"fid":"789","cid":"300","n":"fixture-video.mkv","pc":"` + fixtureDownloadPickCode + `",` +
				`"sha":"` + fixtureSHA1 + `","s":"1024"}]}`))
		default:
			t.Fatalf("unexpected source resolve path: %s", request.URL.Path)
		}
	}))
	defer server.Close()

	adapter := newTestCookieHTTPAdapter(t, server)
	file, err := adapter.ResolveFileByPath(context.Background(), fixtureCredential(), FilePathQuery{
		RootID:       "100",
		RelativePath: "Movies/科幻 (2026) {tmdb-1}/fixture-video.mkv",
	})
	if err != nil {
		t.Fatalf("ResolveFileByPath() error = %v", err)
	}
	if file == nil || file.ID != "789" || file.ParentID != "300" || file.Name != "fixture-video.mkv" ||
		file.PickCode != fixtureDownloadPickCode || file.SHA1 != fixtureSHA1 || file.Size != 1024 || file.IsDirectory {
		t.Fatalf("ResolveFileByPath() file = %+v", file)
	}
	if calls.Load() != 2 {
		t.Fatalf("ResolveFileByPath() calls = %d, want 2", calls.Load())
	}
}

func TestCookieHTTPAdapterResolveFileByPathFailsClosedOnParentPathResponse(t *testing.T) {
	tests := []struct {
		name    string
		body    string
		wantErr error
	}{
		{name: "state missing", body: `{"data":{"file_id":"300"}}`, wantErr: ErrProviderProtocol},
		{name: "provider rejected", body: `{"state":false,"error":"cookie-secret"}`, wantErr: ErrProviderRejected},
		{name: "file id missing", body: `{"state":true,"data":{}}`, wantErr: ErrProviderProtocol},
		{name: "file id invalid", body: `{"state":true,"data":{"file_id":"bad"}}`, wantErr: ErrProviderProtocol},
		{name: "directory not found", body: `{"state":true,"data":{"file_id":"0"}}`, wantErr: ErrSourceFileNotFound},
		{name: "conflicting ids", body: `{"state":true,"id":"300","data":{"file_id":"301"}}`, wantErr: ErrProviderProtocol},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
				calls.Add(1)
				if request.URL.Path != "/files/get_path_id" {
					t.Fatalf("unexpected source resolve path: %s", request.URL.Path)
				}
				_, _ = w.Write([]byte(test.body))
			}))
			defer server.Close()

			adapter := newTestCookieHTTPAdapter(t, server)
			_, err := adapter.ResolveFileByPath(context.Background(), fixtureCredential(), FilePathQuery{
				RootID: "100", RelativePath: "Movies/fixture-video.mkv",
			})
			if !errors.Is(err, test.wantErr) {
				t.Fatalf("ResolveFileByPath() error = %v, want %v", err, test.wantErr)
			}
			if calls.Load() != 1 {
				t.Fatalf("ResolveFileByPath() calls = %d, want 1", calls.Load())
			}
			if strings.Contains(fmt.Sprint(err), "cookie-secret") {
				t.Fatalf("ResolveFileByPath() exposed provider response: %v", err)
			}
		})
	}
}

func TestCookieHTTPAdapterResolveFileByPathAcceptsTopLevelParentID(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		calls.Add(1)
		switch request.URL.Path {
		case "/files/get_path_id":
			_, _ = w.Write([]byte(`{"state":true,"id":300}`))
		case "/files":
			if request.URL.Query().Get("cid") != "300" {
				t.Fatalf("unexpected source directory cid=%s", request.URL.Query().Get("cid"))
			}
			_, _ = w.Write([]byte(`{"state":true,"cid":300,"count":1,"offset":0,"data":[` +
				`{"fid":789,"cid":300,"n":"fixture-video.mkv","pc":"` + fixtureDownloadPickCode + `",` +
				`"sha":"` + fixtureSHA1 + `","s":1024}]}`))
		default:
			t.Fatalf("unexpected source resolve path: %s", request.URL.Path)
		}
	}))
	defer server.Close()

	adapter := newTestCookieHTTPAdapter(t, server)
	file, err := adapter.ResolveFileByPath(context.Background(), fixtureCredential(), FilePathQuery{
		RootID: "100", RelativePath: "Movies/fixture-video.mkv",
	})
	if err != nil || file == nil || file.ID != "789" {
		t.Fatalf("ResolveFileByPath() file=%+v error=%v", file, err)
	}
	if calls.Load() != 2 {
		t.Fatalf("ResolveFileByPath() calls = %d, want 2", calls.Load())
	}
}

func TestCookieHTTPAdapterResolveFileByPathUsesProviderSizeAfterUniqueNameMatch(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		calls.Add(1)
		if request.URL.Path != "/files" || request.URL.Query().Get("cid") != "100" {
			t.Fatalf("unexpected root file request: %s %v", request.URL.Path, request.URL.Query())
		}
		_, _ = w.Write([]byte(`{"state":true,"cid":100,"count":1,"offset":0,"data":[` +
			`{"fid":789,"cid":100,"n":"fixture-video.mkv","pc":"` + fixtureDownloadPickCode + `",` +
			`"sha":"` + fixtureSHA1 + `","s":2048}]}`))
	}))
	defer server.Close()

	adapter := newTestCookieHTTPAdapter(t, server)
	file, err := adapter.ResolveFileByPath(context.Background(), fixtureCredential(), FilePathQuery{
		RootID: "100", RelativePath: "fixture-video.mkv",
	})
	if err != nil || file == nil || file.Size != 2048 || calls.Load() != 1 {
		t.Fatalf("ResolveFileByPath() file=%+v error=%v", file, err)
	}
}

func TestCookieHTTPAdapterResolveFileByPathPaginatesBeforeAcceptingMatch(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		assertSourceListQuery(t, request, 1)
		switch request.URL.Query().Get("offset") {
		case "0":
			_, _ = w.Write([]byte(`{"state":true,"cid":100,"count":2,"offset":0,"data":[` +
				`{"fid":901,"cid":100,"n":"other.mkv","pc":"unrelated000001","sha":"` + fixtureSHA1 + `","s":1024}]}`))
		case "1":
			_, _ = w.Write([]byte(`{"state":true,"cid":100,"count":2,"offset":1,"data":[` +
				`{"fid":789,"cid":100,"n":"fixture-video.mkv","pc":"` + fixtureDownloadPickCode + `",` +
				`"sha":"` + fixtureSHA1 + `","s":1024}]}`))
		default:
			t.Fatalf("unexpected offset %s", request.URL.Query().Get("offset"))
		}
	}))
	defer server.Close()

	adapter := newTestCookieHTTPAdapter(t, server)
	adapter.sourceResolvePageSize = 1
	file, err := adapter.ResolveFileByPath(context.Background(), fixtureCredential(), FilePathQuery{
		RootID: "100", RelativePath: "fixture-video.mkv",
	})
	if err != nil || file == nil || file.ID != "789" {
		t.Fatalf("ResolveFileByPath() file=%+v error=%v", file, err)
	}
}

func TestCookieHTTPAdapterResolveFileByPathFailsClosedOnMissingAndAmbiguousMatches(t *testing.T) {
	tests := []struct {
		name         string
		body         string
		relativePath string
		wantErr      error
	}{
		{
			name: "ambiguous same-name files with different sizes",
			body: `{"state":true,"cid":100,"count":2,"offset":0,"data":[` +
				`{"fid":789,"cid":100,"n":"fixture-video.mkv","pc":"` + fixtureDownloadPickCode + `","sha":"` + fixtureSHA1 + `","s":1024},` +
				`{"fid":790,"cid":100,"n":"fixture-video.mkv","pc":"a1b2c3d4e5f6g7h9","sha":"` + fixtureSHA1 + `","s":2048}]}`,
			wantErr: ErrSourceFileAmbiguous,
		},
		{
			name:    "directory exceeds bounded snapshot",
			body:    `{"state":true,"cid":100,"count":10001,"offset":0,"data":[]}`,
			wantErr: ErrSourceDirectoryTooLarge,
		},
		{
			name:    "provider rejected",
			body:    `{"state":false,"error":"cookie-secret"}`,
			wantErr: ErrProviderRejected,
		},
		{
			name:    "provider fell back to root",
			body:    `{"state":true,"cid":0,"count":0,"offset":0,"data":[]}`,
			wantErr: ErrSourceFileNotFound,
		},
		{
			name:    "malformed item",
			body:    `{"state":true,"cid":100,"count":1,"offset":0,"data":[{"fid":789,"cid":100}]}`,
			wantErr: ErrProviderProtocol,
		},
		{
			name: "item parent mismatch",
			body: `{"state":true,"cid":100,"count":1,"offset":0,"data":[` +
				`{"fid":789,"cid":101,"n":"fixture-video.mkv","pc":"` + fixtureDownloadPickCode + `",` +
				`"sha":"` + fixtureSHA1 + `","s":1024}]}`,
			wantErr: ErrProviderProtocol,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				_, _ = w.Write([]byte(test.body))
			}))
			defer server.Close()

			adapter := newTestCookieHTTPAdapter(t, server)
			relativePath := test.relativePath
			if relativePath == "" {
				relativePath = "fixture-video.mkv"
			}
			_, err := adapter.ResolveFileByPath(context.Background(), fixtureCredential(), FilePathQuery{
				RootID: "100", RelativePath: relativePath,
			})
			if !errors.Is(err, test.wantErr) {
				t.Fatalf("ResolveFileByPath() error = %v, want %v", err, test.wantErr)
			}
			if strings.Contains(fmt.Sprint(err), "cookie-secret") {
				t.Fatalf("ResolveFileByPath() exposed provider response: %v", err)
			}
		})
	}
}

func TestCookieHTTPAdapterResolveFileByPathRejectsInvalidQueryBeforeHTTP(t *testing.T) {
	var calls atomic.Int32
	adapter, err := newCookieHTTPAdapter(httpDoerFunc(func(*http.Request) (*http.Response, error) {
		calls.Add(1)
		return nil, errors.New("must not be called")
	}), "https://example.invalid/app/uploadinfo", "https://example.invalid/files/shasearch", "https://example.invalid/4.0/initupload.php")
	if err != nil {
		t.Fatalf("newCookieHTTPAdapter() error = %v", err)
	}

	queries := []FilePathQuery{
		{RootID: "", RelativePath: "fixture-video.mkv"},
		{RootID: "bad", RelativePath: "fixture-video.mkv"},
		{RootID: "100", RelativePath: ""},
		{RootID: "100", RelativePath: "/absolute/video.mkv"},
		{RootID: "100", RelativePath: "Movies/../video.mkv"},
		{RootID: "100", RelativePath: "Movies//video.mkv"},
		{RootID: "100", RelativePath: "Movies\\video.mkv"},
	}
	for _, query := range queries {
		if _, err := adapter.ResolveFileByPath(context.Background(), fixtureCredential(), query); !errors.Is(err, ErrInvalidRequest) {
			t.Fatalf("ResolveFileByPath(%+v) error = %v, want ErrInvalidRequest", query, err)
		}
	}
	if calls.Load() != 0 {
		t.Fatalf("invalid source query reached HTTP: calls=%d", calls.Load())
	}
}

func assertSourceListQuery(t *testing.T, request *http.Request, pageSize int) {
	t.Helper()
	query := request.URL.Query()
	want := map[string]string{
		"aid": "1", "asc": "1", "count_folders": "1", "cur": "1", "fc_mix": "1",
		"limit": strconv.Itoa(pageSize), "o": "file_name", "show_dir": "1",
	}
	for key, value := range want {
		if query.Get(key) != value {
			t.Fatalf("source query %s=%q, want %q; raw=%s", key, query.Get(key), value, request.URL.RawQuery)
		}
	}
	if query.Get("cid") == "" || query.Get("offset") == "" || len(query) != len(want)+2 {
		t.Fatalf("unexpected source query: %s", request.URL.RawQuery)
	}
}
