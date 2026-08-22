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

func TestCookieHTTPAdapterResolveFileByPathTraversesExactSegments(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet || request.URL.Path != "/files" {
			t.Fatalf("unexpected source resolve request: %s %s", request.Method, request.URL.Path)
		}
		assertCookieAdapterHeaders(t, request)
		assertSourceListQuery(t, request, sourceResolvePageSize)
		calls.Add(1)
		switch request.URL.Query().Get("cid") {
		case "100":
			_, _ = w.Write([]byte(`{"state":true,"cid":100,"count":2,"offset":0,"data":[` +
				`{"fid":901,"cid":100,"n":"unrelated.txt","pc":"unrelated000001","sha":"` + fixtureSHA1 + `","s":5},` +
				`{"cid":200,"pid":100,"n":"Movies"}]}`))
		case "200":
			_, _ = w.Write([]byte(`{"state":true,"cid":"200","count":"1","offset":"0","data":[` +
				`{"cid":"300","pid":"200","n":"Sci-Fi"}]}`))
		case "300":
			_, _ = w.Write([]byte(`{"state":true,"cid":300,"count":1,"offset":0,"data":[` +
				`{"fid":"789","cid":"300","n":"fixture-video.mkv","pc":"` + fixtureDownloadPickCode + `",` +
				`"sha":"` + fixtureSHA1 + `","s":"1024"}]}`))
		default:
			t.Fatalf("unexpected source directory cid=%s", request.URL.Query().Get("cid"))
		}
	}))
	defer server.Close()

	adapter := newTestCookieHTTPAdapter(t, server)
	file, err := adapter.ResolveFileByPath(context.Background(), fixtureCredential(), FilePathQuery{
		RootID:       "100",
		RelativePath: "Movies/Sci-Fi/fixture-video.mkv",
		Size:         1024,
	})
	if err != nil {
		t.Fatalf("ResolveFileByPath() error = %v", err)
	}
	if file == nil || file.ID != "789" || file.ParentID != "300" || file.Name != "fixture-video.mkv" ||
		file.PickCode != fixtureDownloadPickCode || file.SHA1 != fixtureSHA1 || file.Size != 1024 || file.IsDirectory {
		t.Fatalf("ResolveFileByPath() file = %+v", file)
	}
	if calls.Load() != 3 {
		t.Fatalf("ResolveFileByPath() calls = %d, want 3", calls.Load())
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
		RootID: "100", RelativePath: "fixture-video.mkv", Size: 1024,
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
			name: "missing exact size",
			body: `{"state":true,"cid":100,"count":1,"offset":0,"data":[` +
				`{"fid":789,"cid":100,"n":"fixture-video.mkv","pc":"` + fixtureDownloadPickCode + `",` +
				`"sha":"` + fixtureSHA1 + `","s":2048}]}`,
			wantErr: ErrSourceFileNotFound,
		},
		{
			name: "ambiguous exact files",
			body: `{"state":true,"cid":100,"count":2,"offset":0,"data":[` +
				`{"fid":789,"cid":100,"n":"fixture-video.mkv","pc":"` + fixtureDownloadPickCode + `","sha":"` + fixtureSHA1 + `","s":1024},` +
				`{"fid":790,"cid":100,"n":"fixture-video.mkv","pc":"a1b2c3d4e5f6g7h9","sha":"` + fixtureSHA1 + `","s":1024}]}`,
			wantErr: ErrSourceFileAmbiguous,
		},
		{
			name: "ambiguous intermediate directories",
			body: `{"state":true,"cid":100,"count":2,"offset":0,"data":[` +
				`{"cid":200,"pid":100,"n":"Movies"},{"cid":201,"pid":100,"n":"Movies"}]}`,
			relativePath: "Movies/fixture-video.mkv",
			wantErr:      ErrSourceFileAmbiguous,
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
				RootID: "100", RelativePath: relativePath, Size: 1024,
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
		{RootID: "", RelativePath: "fixture-video.mkv", Size: 1024},
		{RootID: "bad", RelativePath: "fixture-video.mkv", Size: 1024},
		{RootID: "100", RelativePath: "", Size: 1024},
		{RootID: "100", RelativePath: "/absolute/video.mkv", Size: 1024},
		{RootID: "100", RelativePath: "Movies/../video.mkv", Size: 1024},
		{RootID: "100", RelativePath: "Movies//video.mkv", Size: 1024},
		{RootID: "100", RelativePath: "Movies\\video.mkv", Size: 1024},
		{RootID: "100", RelativePath: "fixture-video.mkv", Size: 0},
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
