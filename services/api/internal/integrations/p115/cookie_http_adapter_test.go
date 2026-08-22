package p115

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
)

const (
	fixtureCookie    = "UID=0012345_A1; CID=fake; SEID=fake"
	fixtureUserAgent = "ember-p115-adapter-test"
	fixtureSHA1      = "0123456789ABCDEF0123456789ABCDEF01234567"
)

func TestCookieHTTPAdapterGetUploadInfoFixesRequestContract(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/app/uploadinfo" || r.URL.RawQuery != "" {
			t.Fatalf("unexpected upload-info request: %s %s?%s", r.Method, r.URL.Path, r.URL.RawQuery)
		}
		assertCookieAdapterHeaders(t, r)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"state":true,"user_id":12345,"userkey":"fixture-user-key"}`))
	}))
	defer server.Close()

	adapter := newTestCookieHTTPAdapter(t, server)
	info, err := adapter.GetUploadInfo(context.Background(), fixtureCredential())
	if err != nil {
		t.Fatalf("GetUploadInfo() error = %v", err)
	}
	if info.UserID != "12345" || info.UserKey != "fixture-user-key" {
		t.Fatalf("GetUploadInfo() = %+v", info)
	}
}

func TestCookieHTTPAdapterGetUploadInfoRejectsUnsafeResponses(t *testing.T) {
	tests := []struct {
		name    string
		body    string
		wantErr error
	}{
		{name: "provider rejected", body: `{"state":false,"error":"cookie-secret"}`, wantErr: ErrProviderRejected},
		{name: "missing state", body: `{"user_id":12345,"userkey":"fixture-user-key"}`, wantErr: ErrProviderProtocol},
		{name: "missing user key", body: `{"state":true,"user_id":12345}`, wantErr: ErrProviderProtocol},
		{name: "invalid user id", body: `{"state":true,"user_id":"not-a-number","userkey":"fixture-user-key"}`, wantErr: ErrProviderProtocol},
		{name: "account mismatch", body: `{"state":true,"user_id":99999,"userkey":"fixture-user-key"}`, wantErr: ErrCredentialRejected},
		{name: "invalid json", body: `{"state":true,"userkey":"cookie-secret"`, wantErr: ErrProviderProtocol},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				_, _ = w.Write([]byte(test.body))
			}))
			defer server.Close()

			adapter := newTestCookieHTTPAdapter(t, server)
			_, err := adapter.GetUploadInfo(context.Background(), fixtureCredential())
			if !errors.Is(err, test.wantErr) {
				t.Fatalf("GetUploadInfo() error = %v, want %v", err, test.wantErr)
			}
			if strings.Contains(fmt.Sprint(err), "cookie-secret") {
				t.Fatalf("GetUploadInfo() exposed provider response: %v", err)
			}
		})
	}
}

func TestCookieHTTPAdapterRejectsInvalidCredentialBeforeHTTP(t *testing.T) {
	var calls atomic.Int32
	adapter, err := newCookieHTTPAdapter(httpDoerFunc(func(*http.Request) (*http.Response, error) {
		calls.Add(1)
		return nil, errors.New("must not be called")
	}), "https://example.invalid/app/uploadinfo", "https://example.invalid/files/shasearch", "https://example.invalid/4.0/initupload.php")
	if err != nil {
		t.Fatalf("newCookieHTTPAdapter() error = %v", err)
	}
	credential := fixtureCredential()
	credential.Cookie = "CID=fake"
	if _, err := adapter.GetUploadInfo(context.Background(), credential); !errors.Is(err, ErrCredentialRejected) {
		t.Fatalf("GetUploadInfo() error = %v, want ErrCredentialRejected", err)
	}
	if _, err := adapter.SearchBySHA1(context.Background(), credential, FileQuery{SHA1: fixtureSHA1, Size: 1024}); !errors.Is(err, ErrCredentialRejected) {
		t.Fatalf("SearchBySHA1() error = %v, want ErrCredentialRejected", err)
	}
	if _, err := adapter.ResolveFileByPath(context.Background(), credential, FilePathQuery{RootID: "100", RelativePath: "fixture.mkv", Size: 1024}); !errors.Is(err, ErrCredentialRejected) {
		t.Fatalf("ResolveFileByPath() error = %v, want ErrCredentialRejected", err)
	}
	if _, err := adapter.InitRapidUpload(context.Background(), credential, fixtureRapidUploadRequest()); !errors.Is(err, ErrCredentialRejected) {
		t.Fatalf("InitRapidUpload() error = %v, want ErrCredentialRejected", err)
	}
	if _, err := adapter.GetDownloadURL(context.Background(), credential, DownloadURLRequest{PickCode: fixtureDownloadPickCode, UserAgent: fixtureClientUserAgent}); !errors.Is(err, ErrCredentialRejected) {
		t.Fatalf("GetDownloadURL() error = %v, want ErrCredentialRejected", err)
	}
	if _, err := adapter.HashFileRange(context.Background(), credential, FileRangeRequest{
		File: File{PickCode: fixtureDownloadPickCode, Size: 1024}, Range: ByteRange{Start: 0, End: 127},
	}); !errors.Is(err, ErrCredentialRejected) {
		t.Fatalf("HashFileRange() error = %v, want ErrCredentialRejected", err)
	}
	if calls.Load() != 0 {
		t.Fatalf("invalid credential reached HTTP client: calls=%d", calls.Load())
	}
}

func TestCookieHTTPAdapterSearchBySHA1FixesQueryAndMapsCandidate(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/files/shasearch" {
			t.Fatalf("unexpected SHA1 search request: %s %s", r.Method, r.URL.Path)
		}
		query := r.URL.Query()
		if len(query) != 1 || query.Get("sha1") != fixtureSHA1 {
			t.Fatalf("unexpected SHA1 search query: %s", r.URL.RawQuery)
		}
		assertCookieAdapterHeaders(t, r)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"state":true,
			"data":{
				"file_id":789,
				"parent_id":"456",
				"file_name":"fixture.mkv",
				"pick_code":"fixture-pick-code",
				"sha1":"` + fixtureSHA1 + `",
				"file_size":"1024"
			}
		}`))
	}))
	defer server.Close()

	adapter := newTestCookieHTTPAdapter(t, server)
	files, err := adapter.SearchBySHA1(context.Background(), fixtureCredential(), FileQuery{
		SHA1: strings.ToLower(fixtureSHA1),
		Size: 1024,
	})
	if err != nil {
		t.Fatalf("SearchBySHA1() error = %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("SearchBySHA1() returned %d files, want 1", len(files))
	}
	file := files[0]
	if file.ID != "789" || file.ParentID != "456" || file.Name != "fixture.mkv" ||
		file.PickCode != "fixture-pick-code" || file.SHA1 != fixtureSHA1 || file.Size != 1024 || file.IsDirectory {
		t.Fatalf("SearchBySHA1() candidate = %+v", file)
	}
}

func TestCookieHTTPAdapterSearchBySHA1UsesScopedTargetSearchWhenParentProvided(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet || request.URL.Path != "/files/search" {
			t.Fatalf("unexpected scoped SHA1 search request: %s %s", request.Method, request.URL.Path)
		}
		expected := url.Values{
			"aid":          {"1"},
			"cid":          {"456"},
			"fc":           {"2"},
			"limit":        {strconv.Itoa(targetSearchLimit)},
			"offset":       {"0"},
			"search_value": {fixtureSHA1},
			"show_dir":     {"0"},
			"type":         {"99"},
		}
		if request.URL.Query().Encode() != expected.Encode() {
			t.Fatalf("scoped SHA1 search query = %s, want %s", request.URL.RawQuery, expected.Encode())
		}
		_, _ = w.Write([]byte(`{"state":true,"data":[{
			"fid":789,
			"cid":456,
			"n":"fixture.mkv",
			"pc":"fixture-pick-code",
			"sha":"` + fixtureSHA1 + `",
			"s":1024
		}]}`))
	}))
	defer server.Close()

	adapter := newTestCookieHTTPAdapter(t, server)
	files, err := adapter.SearchBySHA1(context.Background(), fixtureCredential(), FileQuery{
		SHA1:     fixtureSHA1,
		Size:     1024,
		ParentID: "456",
	})
	if err != nil {
		t.Fatalf("SearchBySHA1() scoped search error = %v", err)
	}
	if len(files) != 1 || files[0].ID != "789" || files[0].ParentID != "456" || files[0].SHA1 != fixtureSHA1 {
		t.Fatalf("SearchBySHA1() scoped result = %+v", files)
	}
}

func TestCookieHTTPAdapterSearchBySHA1MapsPinnedLegacyFieldAliases(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{
			"state":true,
			"data":{
				"file_id":"789",
				"category_id":456,
				"file_name":"fixture.mkv",
				"pick_code":"fixture-pick-code",
				"file_sha1":"` + fixtureSHA1 + `",
				"file_size":1024
			}
		}`))
	}))
	defer server.Close()

	adapter := newTestCookieHTTPAdapter(t, server)
	files, err := adapter.SearchBySHA1(context.Background(), fixtureCredential(), FileQuery{
		SHA1: fixtureSHA1,
		Size: 1024,
	})
	if err != nil {
		t.Fatalf("SearchBySHA1() legacy alias error = %v", err)
	}
	if len(files) != 1 || files[0].ParentID != "456" || files[0].SHA1 != fixtureSHA1 {
		t.Fatalf("SearchBySHA1() legacy alias result = %+v", files)
	}
}

func TestCookieHTTPAdapterSearchBySHA1MapsPinnedWebShortFields(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{
			"state":true,
			"data":{
				"fid":"789",
				"cid":"456",
				"n":"fixture.mkv",
				"pc":"fixture-pick-code",
				"sha":"` + fixtureSHA1 + `",
				"s":"1024"
			}
		}`))
	}))
	defer server.Close()

	adapter := newTestCookieHTTPAdapter(t, server)
	files, err := adapter.SearchBySHA1(context.Background(), fixtureCredential(), FileQuery{
		SHA1: fixtureSHA1,
		Size: 1024,
	})
	if err != nil {
		t.Fatalf("SearchBySHA1() web short fields error = %v", err)
	}
	if len(files) != 1 || files[0].ID != "789" || files[0].ParentID != "456" ||
		files[0].Name != "fixture.mkv" || files[0].PickCode != "fixture-pick-code" ||
		files[0].SHA1 != fixtureSHA1 || files[0].Size != 1024 || files[0].IsDirectory {
		t.Fatalf("SearchBySHA1() web short fields result = %+v", files)
	}
}

func TestCookieHTTPAdapterSearchBySHA1MapsPinnedWebFallbackFields(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{
			"state":true,
			"data":{
				"fid":789,
				"cid":456,
				"fn":"fixture.mkv",
				"pc":"fixture-pick-code",
				"sha1":"` + fixtureSHA1 + `",
				"fs":1024
			}
		}`))
	}))
	defer server.Close()

	adapter := newTestCookieHTTPAdapter(t, server)
	files, err := adapter.SearchBySHA1(context.Background(), fixtureCredential(), FileQuery{
		SHA1: fixtureSHA1,
		Size: 1024,
	})
	if err != nil {
		t.Fatalf("SearchBySHA1() web fallback fields error = %v", err)
	}
	if len(files) != 1 || files[0].ID != "789" || files[0].ParentID != "456" ||
		files[0].Name != "fixture.mkv" || files[0].PickCode != "fixture-pick-code" ||
		files[0].SHA1 != fixtureSHA1 || files[0].Size != 1024 || files[0].IsDirectory {
		t.Fatalf("SearchBySHA1() web fallback fields result = %+v", files)
	}
}

func TestCookieHTTPAdapterSearchBySHA1MapsLegacyMissToEmptyResult(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"state":false,"error":"文件错误"}`))
	}))
	defer server.Close()

	adapter := newTestCookieHTTPAdapter(t, server)
	files, err := adapter.SearchBySHA1(context.Background(), fixtureCredential(), FileQuery{SHA1: fixtureSHA1, Size: 1024})
	if err != nil {
		t.Fatalf("SearchBySHA1() miss error = %v", err)
	}
	if len(files) != 0 {
		t.Fatalf("SearchBySHA1() miss returned %+v", files)
	}
}

func TestCookieHTTPAdapterSearchBySHA1RejectsMismatchedCandidates(t *testing.T) {
	tests := []struct {
		name     string
		query    FileQuery
		response string
	}{
		{
			name:  "sha1 mismatch",
			query: FileQuery{SHA1: fixtureSHA1, Size: 1024},
			response: `{"state":true,"data":{"file_id":789,"parent_id":456,"file_name":"fixture.mkv",` +
				`"pick_code":"fixture-pick-code","sha1":"1123456789ABCDEF0123456789ABCDEF01234567","file_size":1024}}`,
		},
		{
			name:  "size mismatch",
			query: FileQuery{SHA1: fixtureSHA1, Size: 1024},
			response: `{"state":true,"data":{"file_id":789,"parent_id":456,"file_name":"fixture.mkv",` +
				`"pick_code":"fixture-pick-code","sha1":"` + fixtureSHA1 + `","file_size":2048}}`,
		},
		{
			name:  "directory mismatch",
			query: FileQuery{SHA1: fixtureSHA1, Size: 1024},
			response: `{"state":true,"data":{"file_id":789,"parent_id":456,"file_name":"fixture",` +
				`"pick_code":"fixture-pick-code","sha1":"","file_size":1024}}`,
		},
		{
			name:  "parent mismatch",
			query: FileQuery{SHA1: fixtureSHA1, Size: 1024, ParentID: "999"},
			response: `{"state":true,"data":[{"fid":789,"cid":456,"n":"fixture.mkv",` +
				`"pc":"fixture-pick-code","sha":"` + fixtureSHA1 + `","s":1024}]}`,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				_, _ = w.Write([]byte(test.response))
			}))
			defer server.Close()

			adapter := newTestCookieHTTPAdapter(t, server)
			files, err := adapter.SearchBySHA1(context.Background(), fixtureCredential(), test.query)
			if err != nil {
				t.Fatalf("SearchBySHA1() mismatch error = %v", err)
			}
			if len(files) != 0 {
				t.Fatalf("SearchBySHA1() accepted mismatched candidates: %+v", files)
			}
		})
	}
}

func TestCookieHTTPAdapterSearchBySHA1ValidatesInputBeforeHTTP(t *testing.T) {
	var calls atomic.Int32
	adapter, err := newCookieHTTPAdapter(httpDoerFunc(func(*http.Request) (*http.Response, error) {
		calls.Add(1)
		return nil, errors.New("must not be called")
	}), "https://example.invalid/app/uploadinfo", "https://example.invalid/files/shasearch", "https://example.invalid/4.0/initupload.php")
	if err != nil {
		t.Fatalf("newCookieHTTPAdapter() error = %v", err)
	}

	for _, query := range []FileQuery{
		{SHA1: "not-a-sha1", Size: 1},
		{SHA1: fixtureSHA1, Size: -1},
	} {
		if _, err := adapter.SearchBySHA1(context.Background(), fixtureCredential(), query); !errors.Is(err, ErrInvalidRequest) {
			t.Fatalf("SearchBySHA1(%+v) error = %v, want ErrInvalidRequest", query, err)
		}
	}
	if calls.Load() != 0 {
		t.Fatalf("invalid query reached HTTP client: calls=%d", calls.Load())
	}
}

func TestCookieHTTPAdapterMapsSafeProviderErrors(t *testing.T) {
	t.Run("transport", func(t *testing.T) {
		adapter, err := newCookieHTTPAdapter(httpDoerFunc(func(*http.Request) (*http.Response, error) {
			return nil, &url.Error{Op: "Get", URL: "https://webapi.115.com/files/shasearch?secret=cookie-secret", Err: errors.New("dial failed")}
		}), "https://example.invalid/app/uploadinfo", "https://example.invalid/files/shasearch", "https://example.invalid/4.0/initupload.php")
		if err != nil {
			t.Fatalf("newCookieHTTPAdapter() error = %v", err)
		}
		_, err = adapter.SearchBySHA1(context.Background(), fixtureCredential(), FileQuery{SHA1: fixtureSHA1, Size: 1024})
		if !errors.Is(err, ErrProviderUnavailable) {
			t.Fatalf("SearchBySHA1() error = %v, want ErrProviderUnavailable", err)
		}
		if strings.Contains(fmt.Sprint(err), "cookie-secret") || strings.Contains(fmt.Sprint(err), "webapi.115.com") {
			t.Fatalf("transport error exposed request details: %v", err)
		}
	})

	for _, test := range []struct {
		name       string
		statusCode int
		body       string
		wantErr    error
	}{
		{name: "http status", statusCode: http.StatusServiceUnavailable, body: "cookie-secret", wantErr: ErrProviderUnavailable},
		{name: "invalid json", statusCode: http.StatusOK, body: `{"state":true,"data":"cookie-secret"`, wantErr: ErrProviderProtocol},
		{name: "provider rejected", statusCode: http.StatusOK, body: `{"state":false,"error":"cookie-secret"}`, wantErr: ErrProviderRejected},
		{name: "missing file fields", statusCode: http.StatusOK, body: `{"state":true,"data":{"file_name":"cookie-secret"}}`, wantErr: ErrProviderProtocol},
		{name: "conflicting aliases", statusCode: http.StatusOK, body: `{"state":true,"data":{"file_id":789,"parent_id":456,"category_id":999,"file_name":"fixture.mkv","pick_code":"fixture-pick-code","sha1":"` + fixtureSHA1 + `","file_size":1024}}`, wantErr: ErrProviderProtocol},
		{name: "response too large", statusCode: http.StatusOK, body: strings.Repeat("x", maxCookieResponseBody+1), wantErr: ErrProviderProtocol},
	} {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(test.statusCode)
				_, _ = w.Write([]byte(test.body))
			}))
			defer server.Close()

			adapter := newTestCookieHTTPAdapter(t, server)
			_, err := adapter.SearchBySHA1(context.Background(), fixtureCredential(), FileQuery{SHA1: fixtureSHA1, Size: 1024})
			if !errors.Is(err, test.wantErr) {
				t.Fatalf("SearchBySHA1() error = %v, want %v", err, test.wantErr)
			}
			if strings.Contains(fmt.Sprint(err), "cookie-secret") {
				t.Fatalf("provider error exposed response details: %v", err)
			}
		})
	}
}

func fixtureCredential() Credential {
	return Credential{
		AccountID: "account_fixture",
		Cookie:    fixtureCookie,
		AppType:   "web",
		UserAgent: fixtureUserAgent,
	}
}

func assertCookieAdapterHeaders(t *testing.T, request *http.Request) {
	t.Helper()
	if request.Header.Get("Cookie") != fixtureCookie {
		t.Fatalf("unexpected Cookie header: %q", request.Header.Get("Cookie"))
	}
	if request.Header.Get("User-Agent") != fixtureUserAgent {
		t.Fatalf("unexpected User-Agent: %q", request.Header.Get("User-Agent"))
	}
	if request.Header.Get("Accept") != "*/*" {
		t.Fatalf("unexpected Accept header: %q", request.Header.Get("Accept"))
	}
	if request.Header.Get("Authorization") != "" {
		t.Fatalf("unexpected Authorization header: %q", request.Header.Get("Authorization"))
	}
}

func newTestCookieHTTPAdapter(t *testing.T, server *httptest.Server) *CookieHTTPAdapter {
	t.Helper()
	adapter, err := newCookieHTTPAdapter(
		server.Client(),
		server.URL+"/app/uploadinfo",
		server.URL+"/files/shasearch",
		server.URL+"/4.0/initupload.php",
	)
	if err != nil {
		t.Fatalf("newCookieHTTPAdapter() error = %v", err)
	}
	return adapter
}
