package p115

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sync/atomic"
	"testing"
	"time"
)

func TestCookieHTTPAdapterFindTargetFilePollsUntilExactCandidateIsVisible(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet || request.URL.Path != "/files/search" {
			t.Fatalf("unexpected target search request: %s %s", request.Method, request.URL.Path)
		}
		assertTargetSearchQuery(t, request)
		assertCookieAdapterHeaders(t, request)
		if calls.Add(1) == 1 {
			_, _ = w.Write([]byte(`{"state":true,"count":0,"data":[]}`))
			return
		}
		_, _ = w.Write([]byte(`{
			"state":true,
			"count":4,
			"data":[
				{"fid":"786","cid":"200000002","n":"directory","pc":"directory-pc","sha":"","s":"1024"},
				{"fid":"787","cid":"200000002","n":"wrong-size.mkv","pc":"wrong-size-pc","sha":"` + fixtureSHA1 + `","s":"2048"},
				{"fid":"788","cid":"999","n":"wrong-parent.mkv","pc":"wrong-parent-pc","sha":"` + fixtureSHA1 + `","s":"1024"},
				{"fid":"789","cid":"200000002","n":"fixture-video.mkv","pc":"fixture-target-pc","sha":"` + fixtureSHA1 + `","s":"1024"}
			]
		}`))
	}))
	defer server.Close()

	adapter := newTestTargetFileAdapter(t, server)
	clock := newTargetFakeClock()
	clock.bind(adapter)
	file, err := adapter.FindTargetFile(context.Background(), fixtureCredential(), fixtureTargetFileQuery())
	if err != nil {
		t.Fatalf("FindTargetFile() error = %v", err)
	}
	if file == nil || file.ID != "789" || file.ParentID != fixtureUploadParentID ||
		file.Name != fixtureUploadFileName || file.PickCode != "fixture-target-pc" ||
		file.SHA1 != fixtureSHA1 || file.Size != 1024 || file.IsDirectory {
		t.Fatalf("FindTargetFile() = %+v", file)
	}
	if calls.Load() != 2 {
		t.Fatalf("target search calls = %d, want 2", calls.Load())
	}
	if !reflect.DeepEqual(clock.waits, []time.Duration{targetVisibilityPollInterval}) {
		t.Fatalf("target waits = %v", clock.waits)
	}
}

func TestCookieHTTPAdapterFindTargetFileLocksFinalPollAndTimeout(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		_, _ = w.Write([]byte(`{"state":true,"count":0,"data":[]}`))
	}))
	defer server.Close()

	adapter := newTestTargetFileAdapter(t, server)
	clock := newTargetFakeClock()
	clock.bind(adapter)
	adapter.targetPollInterval = 400 * time.Millisecond
	adapter.targetVisibilityTimeout = time.Second
	file, err := adapter.FindTargetFile(context.Background(), fixtureCredential(), fixtureTargetFileQuery())
	if file != nil || !errors.Is(err, ErrTargetFileNotVisible) {
		t.Fatalf("FindTargetFile() file=%+v error=%v, want ErrTargetFileNotVisible", file, err)
	}
	if calls.Load() != 4 {
		t.Fatalf("target timeout calls = %d, want initial, 400ms, 800ms, and final 1000ms polls", calls.Load())
	}
	wantWaits := []time.Duration{400 * time.Millisecond, 400 * time.Millisecond, 200 * time.Millisecond}
	if !reflect.DeepEqual(clock.waits, wantWaits) {
		t.Fatalf("target timeout waits = %v, want %v", clock.waits, wantWaits)
	}
}

func TestCookieHTTPAdapterFindTargetFileRejectsAmbiguousExactMatches(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{
			"state":true,
			"data":[
				{"fid":"789","cid":"200000002","n":"a.mkv","pc":"pc-a","sha":"` + fixtureSHA1 + `","s":"1024"},
				{"fid":"790","cid":"200000002","n":"b.mkv","pc":"pc-b","sha":"` + fixtureSHA1 + `","s":"1024"}
			]
		}`))
	}))
	defer server.Close()

	adapter := newTestTargetFileAdapter(t, server)
	clock := newTargetFakeClock()
	clock.bind(adapter)
	file, err := adapter.FindTargetFile(context.Background(), fixtureCredential(), fixtureTargetFileQuery())
	if file != nil || !errors.Is(err, ErrTargetFileAmbiguous) {
		t.Fatalf("FindTargetFile() file=%+v error=%v, want ErrTargetFileAmbiguous", file, err)
	}
	if len(clock.waits) != 0 {
		t.Fatalf("ambiguous target unexpectedly retried: %v", clock.waits)
	}
}

func TestCookieHTTPAdapterFindTargetFileStopsOnProviderError(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		_, _ = w.Write([]byte(`{"state":false,"error":"cookie-secret"}`))
	}))
	defer server.Close()

	adapter := newTestTargetFileAdapter(t, server)
	clock := newTargetFakeClock()
	clock.bind(adapter)
	_, err := adapter.FindTargetFile(context.Background(), fixtureCredential(), fixtureTargetFileQuery())
	if !errors.Is(err, ErrProviderRejected) {
		t.Fatalf("FindTargetFile() error = %v, want ErrProviderRejected", err)
	}
	if calls.Load() != 1 || len(clock.waits) != 0 {
		t.Fatalf("provider error was retried: calls=%d waits=%v", calls.Load(), clock.waits)
	}
}

func TestCookieHTTPAdapterFindTargetFileStopsOnProtocolError(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		_, _ = w.Write([]byte(`{"state":true,"data":[{"fid":"bad","cid":"200000002"}]}`))
	}))
	defer server.Close()

	adapter := newTestTargetFileAdapter(t, server)
	clock := newTargetFakeClock()
	clock.bind(adapter)
	_, err := adapter.FindTargetFile(context.Background(), fixtureCredential(), fixtureTargetFileQuery())
	if !errors.Is(err, ErrProviderProtocol) {
		t.Fatalf("FindTargetFile() error = %v, want ErrProviderProtocol", err)
	}
	if calls.Load() != 1 || len(clock.waits) != 0 {
		t.Fatalf("protocol error was retried: calls=%d waits=%v", calls.Load(), clock.waits)
	}
}

func TestCookieHTTPAdapterFindTargetFileHonorsContextBeforeHTTP(t *testing.T) {
	var calls atomic.Int32
	adapter, err := newCookieHTTPAdapter(httpDoerFunc(func(*http.Request) (*http.Response, error) {
		calls.Add(1)
		return nil, errors.New("must not be called")
	}), "https://example.invalid/app/uploadinfo", "https://example.invalid/files/shasearch", "https://example.invalid/4.0/initupload.php")
	if err != nil {
		t.Fatalf("newCookieHTTPAdapter() error = %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := adapter.FindTargetFile(ctx, fixtureCredential(), fixtureTargetFileQuery()); !errors.Is(err, context.Canceled) {
		t.Fatalf("FindTargetFile() error = %v, want context.Canceled", err)
	}
	if calls.Load() != 0 {
		t.Fatalf("canceled target lookup reached HTTP: calls=%d", calls.Load())
	}
}

func TestCookieHTTPAdapterFindTargetFileHonorsContextDuringWait(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		_, _ = w.Write([]byte(`{"state":true,"count":0,"data":[]}`))
	}))
	defer server.Close()

	adapter := newTestTargetFileAdapter(t, server)
	ctx, cancel := context.WithCancel(context.Background())
	adapter.wait = func(ctx context.Context, _ time.Duration) error {
		cancel()
		return ctx.Err()
	}
	if _, err := adapter.FindTargetFile(ctx, fixtureCredential(), fixtureTargetFileQuery()); !errors.Is(err, context.Canceled) {
		t.Fatalf("FindTargetFile() error = %v, want context.Canceled", err)
	}
	if calls.Load() != 1 {
		t.Fatalf("context cancellation during wait calls = %d, want 1", calls.Load())
	}
}

func TestCookieHTTPAdapterFindTargetFileRejectsInvalidQueryBeforeHTTP(t *testing.T) {
	var calls atomic.Int32
	adapter, err := newCookieHTTPAdapter(httpDoerFunc(func(*http.Request) (*http.Response, error) {
		calls.Add(1)
		return nil, errors.New("must not be called")
	}), "https://example.invalid/app/uploadinfo", "https://example.invalid/files/shasearch", "https://example.invalid/4.0/initupload.php")
	if err != nil {
		t.Fatalf("newCookieHTTPAdapter() error = %v", err)
	}
	for _, query := range []FileQuery{
		{SHA1: "bad", Size: 1024, ParentID: fixtureUploadParentID},
		{SHA1: fixtureSHA1, Size: -1, ParentID: fixtureUploadParentID},
		{SHA1: fixtureSHA1, Size: 1024},
		{SHA1: fixtureSHA1, Size: 1024, ParentID: "target"},
	} {
		if _, err := adapter.FindTargetFile(context.Background(), fixtureCredential(), query); !errors.Is(err, ErrInvalidRequest) {
			t.Fatalf("FindTargetFile(%+v) error = %v, want ErrInvalidRequest", query, err)
		}
	}
	if calls.Load() != 0 {
		t.Fatalf("invalid target query reached HTTP: calls=%d", calls.Load())
	}
}

func fixtureTargetFileQuery() FileQuery {
	return FileQuery{SHA1: fixtureSHA1, Size: 1024, ParentID: fixtureUploadParentID}
}

func newTestTargetFileAdapter(t *testing.T, server *httptest.Server) *CookieHTTPAdapter {
	t.Helper()
	adapter := newTestRapidUploadAdapter(t, server)
	return adapter
}

func assertTargetSearchQuery(t *testing.T, request *http.Request) {
	t.Helper()
	query := request.URL.Query()
	want := map[string]string{
		"aid":          "1",
		"cid":          fixtureUploadParentID,
		"fc":           "2",
		"limit":        "100",
		"offset":       "0",
		"search_value": fixtureSHA1,
		"show_dir":     "0",
		"type":         "99",
	}
	if len(query) != len(want) {
		t.Fatalf("target query = %s", request.URL.RawQuery)
	}
	for key, value := range want {
		if query.Get(key) != value {
			t.Fatalf("target query %s=%q, want %q", key, query.Get(key), value)
		}
	}
}

type targetFakeClock struct {
	current time.Time
	waits   []time.Duration
}

func newTargetFakeClock() *targetFakeClock {
	return &targetFakeClock{current: time.Unix(1700000000, 0)}
}

func (clock *targetFakeClock) bind(adapter *CookieHTTPAdapter) {
	adapter.now = func() time.Time { return clock.current }
	adapter.wait = func(ctx context.Context, duration time.Duration) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		clock.waits = append(clock.waits, duration)
		clock.current = clock.current.Add(duration)
		return nil
	}
}
