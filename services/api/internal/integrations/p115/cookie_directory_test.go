package p115

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

func TestCookieHTTPAdapterResolveDirectoryByPathTraversesUniqueDirectories(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		assertSourceListQuery(t, request, sourceResolvePageSize)
		switch request.URL.Query().Get("cid") {
		case "0":
			_, _ = w.Write([]byte(`{"state":true,"cid":0,"count":2,"offset":0,"data":[` +
				`{"fid":901,"cid":0,"n":"EmberPlayback","pc":"unrelated000001","sha":"` + fixtureSHA1 + `","s":1},` +
				`{"cid":200,"pid":0,"n":"EmberPlayback"}]}`))
		case "200":
			_, _ = w.Write([]byte(`{"state":true,"cid":200,"count":1,"offset":0,"data":[` +
				`{"cid":300,"pid":200,"n":"Cache"}]}`))
		default:
			t.Fatalf("unexpected cid=%s", request.URL.Query().Get("cid"))
		}
	}))
	defer server.Close()

	adapter := newTestCookieHTTPAdapter(t, server)
	directory, err := adapter.ResolveDirectoryByPath(context.Background(), fixtureCredential(), DirectoryPathQuery{
		RootID: "0", RelativePath: "/EmberPlayback/Cache",
	})
	if err != nil {
		t.Fatalf("ResolveDirectoryByPath() error = %v", err)
	}
	if directory == nil || directory.ID != "300" || directory.ParentID != "200" ||
		directory.Name != "Cache" || directory.Path != "/EmberPlayback/Cache" {
		t.Fatalf("ResolveDirectoryByPath() = %+v", directory)
	}
}

func TestCookieHTTPAdapterResolveDirectoryByPathPaginatesAndKeepsDirectoryType(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		assertSourceListQuery(t, request, 1)
		switch request.URL.Query().Get("offset") {
		case "0":
			_, _ = w.Write([]byte(`{"state":true,"cid":0,"count":2,"offset":0,"data":[` +
				`{"fid":901,"cid":0,"n":"EmberPlayback","pc":"unrelated000001","sha":"` + fixtureSHA1 + `","s":1}]}`))
		case "1":
			_, _ = w.Write([]byte(`{"state":true,"cid":0,"count":2,"offset":1,"data":[` +
				`{"cid":200,"pid":0,"n":"EmberPlayback"}]}`))
		default:
			t.Fatalf("unexpected offset=%s", request.URL.Query().Get("offset"))
		}
	}))
	defer server.Close()

	adapter := newTestCookieHTTPAdapter(t, server)
	adapter.sourceResolvePageSize = 1
	directory, err := adapter.ResolveDirectoryByPath(context.Background(), fixtureCredential(), DirectoryPathQuery{
		RootID: "0", RelativePath: "EmberPlayback",
	})
	if err != nil || directory == nil || directory.ID != "200" {
		t.Fatalf("ResolveDirectoryByPath() directory=%+v error=%v", directory, err)
	}
}

func TestCookieHTTPAdapterResolveDirectoryByPathFailsClosed(t *testing.T) {
	tests := []struct {
		name    string
		body    string
		wantErr error
	}{
		{name: "file only", body: `{"state":true,"cid":0,"count":1,"offset":0,"data":[` +
			`{"fid":901,"cid":0,"n":"EmberPlayback","pc":"unrelated000001","sha":"` + fixtureSHA1 + `","s":1}]}`, wantErr: ErrDirectoryNotFound},
		{name: "ambiguous directories", body: `{"state":true,"cid":0,"count":2,"offset":0,"data":[` +
			`{"cid":200,"pid":0,"n":"EmberPlayback"},{"cid":201,"pid":0,"n":"EmberPlayback"}]}`, wantErr: ErrDirectoryAmbiguous},
		{name: "cid fallback", body: `{"state":true,"cid":999,"count":0,"offset":0,"data":[]}`, wantErr: ErrDirectoryNotFound},
		{name: "provider rejected", body: `{"state":false,"error":"cookie-secret"}`, wantErr: ErrProviderRejected},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				_, _ = w.Write([]byte(test.body))
			}))
			defer server.Close()
			adapter := newTestCookieHTTPAdapter(t, server)

			_, err := adapter.ResolveDirectoryByPath(context.Background(), fixtureCredential(), DirectoryPathQuery{
				RootID: "0", RelativePath: "/EmberPlayback",
			})
			if !errors.Is(err, test.wantErr) {
				t.Fatalf("ResolveDirectoryByPath() error=%v, want %v", err, test.wantErr)
			}
		})
	}
}

func TestCookieHTTPAdapterResolveDirectoryByPathRejectsInvalidInputBeforeHTTP(t *testing.T) {
	var calls atomic.Int32
	adapter, err := newCookieHTTPAdapter(httpDoerFunc(func(*http.Request) (*http.Response, error) {
		calls.Add(1)
		return nil, errors.New("must not be called")
	}), "https://example.invalid/app/uploadinfo", "https://example.invalid/files/shasearch", "https://example.invalid/4.0/initupload.php")
	if err != nil {
		t.Fatalf("newCookieHTTPAdapter() error = %v", err)
	}

	queries := []DirectoryPathQuery{
		{RootID: "", RelativePath: "/EmberPlayback"},
		{RootID: "bad", RelativePath: "/EmberPlayback"},
		{RootID: "0", RelativePath: ""},
		{RootID: "0", RelativePath: "/"},
		{RootID: "0", RelativePath: "/EmberPlayback/"},
		{RootID: "0", RelativePath: "EmberPlayback//Cache"},
		{RootID: "0", RelativePath: "EmberPlayback/../Cache"},
		{RootID: "0", RelativePath: "EmberPlayback\\Cache"},
	}
	for _, query := range queries {
		if _, err := adapter.ResolveDirectoryByPath(context.Background(), fixtureCredential(), query); !errors.Is(err, ErrInvalidRequest) {
			t.Fatalf("ResolveDirectoryByPath(%+v) error=%v, want ErrInvalidRequest", query, err)
		}
	}
	if calls.Load() != 0 {
		t.Fatalf("invalid directory query reached HTTP: calls=%d", calls.Load())
	}
}
