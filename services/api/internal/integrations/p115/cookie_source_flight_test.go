package p115

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// TestSourceDirectoryConcurrentFilesShareSnapshot protects directory-level
// sharing without changing per-file uniqueness or retaining completed results.
func TestSourceDirectoryConcurrentFilesShareSnapshot(t *testing.T) {
	var paths, lists atomic.Int32
	started, release := make(chan struct{}, 8), make(chan struct{})
	adapter := newDeleteTestAdapter(t, httpDoerFunc(func(request *http.Request) (*http.Response, error) {
		if request.URL.Path == "/files/get_path_id" {
			paths.Add(1)
			started <- struct{}{}
			select {
			case <-release:
			case <-request.Context().Done():
				return nil, request.Context().Err()
			}
			return sourceFlightResponse(`{"state":true,"id":300}`), nil
		}
		lists.Add(1)
		return sourceFlightResponse(fmt.Sprintf(`{"state":true,"cid":300,"count":2,"offset":0,"data":[
			{"fid":789,"cid":300,"n":"01.mkv","pc":"%s","sha":"%s","s":1024},
			{"fid":790,"cid":300,"n":"02.mkv","pc":"%s","sha":"%s","s":2048}]}`,
			fixtureDownloadPickCode, fixtureSHA1, fixtureDownloadPickCode, fixtureSHA1)), nil
	}))
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	results := make(chan error, 2)
	resolve := func(name string) {
		file, err := adapter.ResolveFileByPath(ctx, fixtureCredential(), FilePathQuery{RootID: "100", RelativePath: "Season/" + name})
		if err == nil && (file == nil || file.Name != name) {
			err = fmt.Errorf("wrong file: %+v", file)
		}
		results <- err
	}
	go resolve("01.mkv")
	select {
	case <-started:
	case <-ctx.Done():
		t.Fatal("first request did not start")
	}
	go resolve("02.mkv")
	waitSourceFlightWaiters(t, &adapter.sourceFlights, 2)
	close(release)
	for range 2 {
		if err := <-results; err != nil {
			t.Fatal(err)
		}
	}
	if paths.Load() != 1 || lists.Load() != 1 {
		t.Fatalf("concurrent snapshot: path calls=%d list calls=%d, want 1 each", paths.Load(), lists.Load())
	}
	resolve("01.mkv")
	if err := <-results; err != nil {
		t.Fatal(err)
	}
	if paths.Load() != 2 || lists.Load() != 2 {
		t.Fatal("completed snapshot was cached")
	}
}

// TestSourceDirectorySharedPaginationKeepsFileUniqueness ensures sharing a
// root snapshot preserves independent success, missing and ambiguous results.
func TestSourceDirectorySharedPaginationKeepsFileUniqueness(t *testing.T) {
	var calls atomic.Int32
	var observedTiming atomic.Pointer[sourceReadTiming]
	release := make(chan struct{})
	adapter := newDeleteTestAdapter(t, httpDoerFunc(func(request *http.Request) (*http.Response, error) {
		calls.Add(1)
		timing, _ := request.Context().Value(sourceReadTimingKey{}).(*sourceReadTiming)
		observedTiming.Store(timing)
		if request.URL.Path != "/files" || request.URL.Query().Get("cid") != "100" {
			return nil, fmt.Errorf("unexpected root request")
		}
		select {
		case <-release:
		case <-request.Context().Done():
			return nil, request.Context().Err()
		}
		offset := request.URL.Query().Get("offset")
		name, id := "duplicate.mkv", "789"
		switch offset {
		case "0":
			name = "unique.mkv"
		case "1":
			id = "790"
		case "2":
			id = "791"
		default:
			return nil, fmt.Errorf("unexpected page")
		}
		return sourceFlightResponse(fmt.Sprintf(`{"state":true,"cid":100,"count":3,"offset":%s,"data":[
			{"fid":%s,"cid":100,"n":%q,"pc":"%s","sha":"%s","s":1024}]}`,
			offset, id, name, fixtureDownloadPickCode, fixtureSHA1)), nil
	}))
	adapter.sourceResolvePageSize = 1
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	results := make(chan error, 3)
	for name, wantErr := range map[string]error{"unique.mkv": nil, "duplicate.mkv": ErrSourceFileAmbiguous, "missing.mkv": ErrSourceFileNotFound} {
		go func() {
			file, err := adapter.ResolveFileByPath(ctx, fixtureCredential(), FilePathQuery{RootID: "100", RelativePath: name})
			if !errors.Is(err, wantErr) || (err == nil && (file == nil || file.Name != name)) {
				results <- fmt.Errorf("%s: file=%+v err=%v want=%v", name, file, err, wantErr)
				return
			}
			results <- nil
		}()
	}
	waitSourceFlightWaiters(t, &adapter.sourceFlights, 3)
	close(release)
	for range 3 {
		if err := <-results; err != nil {
			t.Fatal(err)
		}
	}
	if calls.Load() != 3 {
		t.Fatalf("shared root pagination calls=%d, want 3", calls.Load())
	}
	if timing := observedTiming.Load(); timing == nil || timing.pages != 3 || timing.parent != 0 || timing.listing <= 0 {
		t.Fatalf("root pagination timing=%+v", timing)
	}
}

// sourceFlightResponse creates a local protocol fixture without a listener or network.
func sourceFlightResponse(body string) *http.Response {
	return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}
}

// waitSourceFlightWaiters observes registration under the same mutex as the
// resolver, avoiding timing assumptions about when goroutines reach the group.
func waitSourceFlightWaiters(t *testing.T, group *sourceDirectoryFlights, want int) {
	t.Helper()
	deadline := time.NewTimer(3 * time.Second)
	defer deadline.Stop()
	ticker := time.NewTicker(time.Millisecond)
	defer ticker.Stop()
	for {
		group.mu.Lock()
		got := 0
		for _, call := range group.calls {
			got += call.waiters
		}
		group.mu.Unlock()
		if got == want {
			return
		}
		select {
		case <-deadline.C:
			t.Fatalf("flight waiters=%d, want %d", got, want)
		case <-ticker.C:
		}
	}
}

// TestSourceDirectoryWaitersCancelIndependently covers both the initiating
// request and followers, including upstream cancellation when nobody remains.
func TestSourceDirectoryWaitersCancelIndependently(t *testing.T) {
	for _, cancelLeader := range []bool{true, false} {
		t.Run(fmt.Sprintf("leader=%t", cancelLeader), func(t *testing.T) {
			var group sourceDirectoryFlights
			key := sourceDirectoryKey(fixtureCredential(), "100", "Season")
			started, release, stopped := make(chan struct{}), make(chan struct{}), make(chan struct{})
			resolver := func(ctx context.Context) ([]File, error) {
				close(started)
				defer close(stopped)
				select {
				case <-release:
					return []File{{ID: "789"}}, nil
				case <-ctx.Done():
					return nil, ctx.Err()
				}
			}
			leader, cancelFirst := context.WithCancel(context.Background())
			follower, cancelSecond := context.WithCancel(context.Background())
			defer cancelFirst()
			defer cancelSecond()
			first, second := make(chan error, 1), make(chan error, 1)
			go func() { _, err := group.do(leader, key, resolver); first <- err }()
			waitTestSignal(t, started, "resolver did not start")
			go func() { _, err := group.do(follower, key, resolver); second <- err }()
			waitSourceFlightWaiters(t, &group, 2)
			canceled, remaining := first, second
			if cancelLeader {
				cancelFirst()
			} else {
				cancelSecond()
				canceled, remaining = second, first
			}
			if err := <-canceled; !errors.Is(err, context.Canceled) {
				t.Fatalf("canceled waiter: %v", err)
			}
			select {
			case <-stopped:
				t.Fatal("one cancellation stopped shared read")
			default:
			}
			close(release)
			if err := <-remaining; err != nil {
				t.Fatal(err)
			}
			waitSourceFlightWaiters(t, &group, 0)
		})
	}
}

// TestSourceDirectoryAllCanceledAllowsFreshFlight protects removal before a
// canceled transport finishes, and prevents that old transport deleting a retry.
func TestSourceDirectoryAllCanceledAllowsFreshFlight(t *testing.T) {
	var group sourceDirectoryFlights
	key := sourceDirectoryKey(fixtureCredential(), "100", "Season")
	oldStopped, finishOld := make(chan struct{}), make(chan struct{})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	oldResult := make(chan error, 1)
	go func() {
		_, err := group.do(ctx, key, func(ctx context.Context) ([]File, error) {
			<-ctx.Done()
			close(oldStopped)
			<-finishOld
			return nil, ctx.Err()
		})
		oldResult <- err
	}()
	waitSourceFlightWaiters(t, &group, 1)
	group.mu.Lock()
	old := group.calls[key]
	group.mu.Unlock()
	cancel()
	if err := <-oldResult; !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	waitTestSignal(t, oldStopped, "last waiter did not cancel transport")
	freshRelease, freshResult := make(chan struct{}), make(chan error, 1)
	go func() {
		_, err := group.do(context.Background(), key, func(ctx context.Context) ([]File, error) {
			select {
			case <-freshRelease:
				return []File{{ID: "790"}}, nil
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		})
		freshResult <- err
	}()
	waitSourceFlightWaiters(t, &group, 1)
	close(finishOld)
	waitTestSignal(t, old.done, "old flight did not finish")
	group.mu.Lock()
	fresh := group.calls[key]
	group.mu.Unlock()
	if fresh == nil || fresh == old {
		t.Fatal("old completion removed replacement flight")
	}
	close(freshRelease)
	if err := <-freshResult; err != nil {
		t.Fatal(err)
	}
}

// TestSourceDirectoryFailureAndDeadlineAreNotCached checks error fan-out,
// discarded partial results, pre-cancellation and the independent total budget.
func TestSourceDirectoryFailureAndDeadlineAreNotCached(t *testing.T) {
	group := sourceDirectoryFlights{timeout: time.Second}
	key := sourceDirectoryKey(fixtureCredential(), "100", "Season")
	release := make(chan struct{})
	var calls atomic.Int32
	results := make(chan error, 2)
	resolve := func(context.Context) ([]File, error) {
		calls.Add(1)
		<-release
		return []File{{ID: "partial"}}, ErrProviderProtocol
	}
	for range 2 {
		go func() {
			files, err := group.do(context.Background(), key, resolve)
			if files != nil {
				err = fmt.Errorf("partial snapshot leaked")
			}
			results <- err
		}()
	}
	waitSourceFlightWaiters(t, &group, 2)
	close(release)
	for range 2 {
		if err := <-results; !errors.Is(err, ErrProviderProtocol) {
			t.Fatal(err)
		}
	}
	if calls.Load() != 1 {
		t.Fatal("failure was not shared")
	}
	_, _ = group.do(context.Background(), key, resolve)
	if calls.Load() != 2 {
		t.Fatal("failure was cached")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := group.do(ctx, key, resolve); !errors.Is(err, context.Canceled) || calls.Load() != 2 {
		t.Fatalf("pre-canceled request invoked resolver: %v", err)
	}
	group.timeout = 10 * time.Millisecond
	_, err := group.do(context.Background(), key, func(ctx context.Context) ([]File, error) {
		<-ctx.Done()
		return nil, ErrProviderUnavailable
	})
	if !errors.Is(err, ErrProviderUnavailable) || errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("shared budget was confused with a client deadline: %v", err)
	}
	waitSourceFlightWaiters(t, &group, 0)
}

// TestSourceDirectoryKeyIsolation ensures credential rotation, accounts and
// directory mappings never join the same in-flight operation accidentally.
func TestSourceDirectoryKeyIsolation(t *testing.T) {
	base := fixtureCredential()
	key := sourceDirectoryKey(base, "100", "Season")
	for _, field := range []string{"account", "cookie", "app", "ua", "root", "path"} {
		credential, root, path := base, "100", "Season"
		switch field {
		case "account":
			credential.AccountID += "-other"
		case "cookie":
			credential.Cookie += "; SEID=rotated-fixture"
		case "app":
			credential.AppType += "-other"
		case "ua":
			credential.UserAgent += "-other"
		case "root":
			root = "101"
		case "path":
			path = "OtherSeason"
		}
		if sourceDirectoryKey(credential, root, path) == key {
			t.Fatalf("%s did not isolate flight", field)
		}
	}
	if sourceDirectoryKey(base, "1", "23") == sourceDirectoryKey(base, "12", "3") {
		t.Fatal("key fields are not length-delimited")
	}
}
