package playbackgateway

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"runtime"
	"sync/atomic"
	"testing"
	"time"
)

type imageCacheOutcome struct {
	body   string
	header http.Header
	err    error
}

// imageCacheAsync performs one complete fake response read without invoking
// testing.Fatal in a worker goroutine, keeping failures on the test thread.
func imageCacheAsync(cache *imageCachingTransport, ctx context.Context) <-chan imageCacheOutcome {
	result := make(chan imageCacheOutcome, 1)
	go func() {
		request := imageCacheTestRequest(http.MethodGet, "http://emby.invalid/emby/Items/123/Images/Primary", true).WithContext(ctx)
		original := imageCacheTestRequest(http.MethodGet, request.URL.String(), true)
		request = request.WithContext(context.WithValue(request.Context(), requestRouteContextKey{}, original.Context().Value(requestRouteContextKey{})))
		response, err := cache.RoundTrip(request)
		if err != nil {
			result <- imageCacheOutcome{err: err}
			return
		}
		body, err := io.ReadAll(response.Body)
		response.Body.Close()
		result <- imageCacheOutcome{body: string(body), header: response.Header, err: err}
	}()
	return result
}

// imageCacheWaitFor waits for a local synchronization condition rather than
// relying on sleeps to guess when competing cache callers have arrived.
func imageCacheWaitFor(t *testing.T, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for !condition() {
		if time.Now().After(deadline) {
			t.Fatal("image cache synchronization deadline exceeded")
		}
		runtime.Gosched()
	}
}

// imageCacheWaiters reads the in-flight waiter count under its production lock.
func imageCacheWaiters(cache *imageCachingTransport) int {
	cache.mu.Lock()
	defer cache.mu.Unlock()
	count := 0
	for _, flight := range cache.flights {
		count += len(flight.waiters)
	}
	return count
}

// TestImageCacheConcurrentMissCoalesces measures actual fake upstream calls,
// ensuring concurrent identical GETs share one body fetch rather than six.
func TestImageCacheConcurrentMissCoalesces(t *testing.T) {
	var calls atomic.Int32
	release := make(chan struct{})
	cache := newImageCachingTransport(roundTripFunc(func(r *http.Request) (*http.Response, error) {
		calls.Add(1)
		select {
		case <-release:
			return imageCacheTestResponse("shared-image"), nil
		case <-r.Context().Done():
			return nil, r.Context().Err()
		}
	}), nil).(*imageCachingTransport)
	results := make([]<-chan imageCacheOutcome, 6)
	for i := range results {
		results[i] = imageCacheAsync(cache, context.Background())
	}
	imageCacheWaitFor(t, func() bool { return imageCacheWaiters(cache) == len(results) })
	close(release)
	for _, result := range results {
		got := <-result
		if got.err != nil || got.body != "shared-image" {
			t.Fatalf("shared result=%+v", got)
		}
	}
	if calls.Load() != 1 {
		t.Fatalf("upstream calls=%d, want 1", calls.Load())
	}
}

// TestImageCacheWaiterCancellationIndependent ensures canceling one caller
// cannot cancel the shared image retrieval needed by another active caller.
func TestImageCacheWaiterCancellationIndependent(t *testing.T) {
	release := make(chan struct{})
	sharedCanceled := make(chan struct{}, 1)
	cache := newImageCachingTransport(roundTripFunc(func(r *http.Request) (*http.Response, error) {
		select {
		case <-release:
			return imageCacheTestResponse("image"), nil
		case <-r.Context().Done():
			sharedCanceled <- struct{}{}
			return nil, r.Context().Err()
		}
	}), nil).(*imageCachingTransport)
	ctx, cancel := context.WithCancel(context.Background())
	first := imageCacheAsync(cache, ctx)
	second := imageCacheAsync(cache, context.Background())
	imageCacheWaitFor(t, func() bool { return imageCacheWaiters(cache) == 2 })
	cancel()
	if result := <-first; !errors.Is(result.err, context.Canceled) {
		t.Fatalf("canceled caller=%+v", result)
	}
	select {
	case <-sharedCanceled:
		t.Fatal("one canceled waiter stopped the other's upstream work")
	default:
	}
	close(release)
	if result := <-second; result.err != nil || result.body != "image" {
		t.Fatalf("healthy waiter=%+v", result)
	}
}

// TestImageCacheLastWaiterCancelsWithoutPoisoningReplacement verifies an old
// canceled fetch cannot erase the next healthy fetch's cache publication.
func TestImageCacheLastWaiterCancelsWithoutPoisoningReplacement(t *testing.T) {
	var calls atomic.Int32
	stopped, finishOld := make(chan struct{}), make(chan struct{})
	cache := newImageCachingTransport(roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if calls.Add(1) == 1 {
			<-r.Context().Done()
			close(stopped)
			<-finishOld
			return nil, r.Context().Err()
		}
		return imageCacheTestResponse("replacement"), nil
	}), nil).(*imageCachingTransport)
	ctx, cancel := context.WithCancel(context.Background())
	first := imageCacheAsync(cache, ctx)
	imageCacheWaitFor(t, func() bool { return imageCacheWaiters(cache) == 1 })
	cache.mu.Lock()
	var old *imageCacheFlight
	for _, flight := range cache.flights {
		old = flight
	}
	cache.mu.Unlock()
	cancel()
	if result := <-first; !errors.Is(result.err, context.Canceled) {
		t.Fatal(result.err)
	}
	<-stopped
	if result := <-imageCacheAsync(cache, context.Background()); result.err != nil || result.body != "replacement" {
		t.Fatalf("replacement poisoned=%+v", result)
	}
	close(finishOld)
	imageCacheWaitFor(t, func() bool {
		cache.mu.Lock()
		defer cache.mu.Unlock()
		return old.finished
	})
	if result := <-imageCacheAsync(cache, context.Background()); result.err != nil || result.body != "replacement" || calls.Load() != 2 {
		t.Fatalf("old completion erased replacement: result=%+v calls=%d", result, calls.Load())
	}
}

// TestImageCacheNoncacheableResponsesNeverShared gives every waiting caller
// an independent Set-Cookie response and independent upstream body.
func TestImageCacheNoncacheableResponsesNeverShared(t *testing.T) {
	var calls atomic.Int32
	release := make(chan struct{})
	cache := newImageCachingTransport(roundTripFunc(func(r *http.Request) (*http.Response, error) {
		call := calls.Add(1)
		if call == 1 {
			<-release
		}
		response := imageCacheTestResponse(fmt.Sprint(call))
		response.Header.Set("Set-Cookie", fmt.Sprintf("fixture=%d", call))
		return response, nil
	}), nil).(*imageCachingTransport)
	results := make([]<-chan imageCacheOutcome, 6)
	for i := range results {
		results[i] = imageCacheAsync(cache, context.Background())
	}
	imageCacheWaitFor(t, func() bool { return imageCacheWaiters(cache) == 6 })
	close(release)
	seen := make(map[string]bool)
	for _, result := range results {
		got := <-result
		if got.err != nil || seen[got.header.Get("Set-Cookie")] {
			t.Fatalf("noncacheable result shared: %+v", got)
		}
		seen[got.header.Get("Set-Cookie")] = true
	}
	if calls.Load() != 6 || len(cache.entries) != 0 {
		t.Fatalf("calls=%d entries=%d", calls.Load(), len(cache.entries))
	}
}

// TestImageCacheFlightLimitsBypass verifies a saturated miss registry does not
// block a different image or allocate an additional tracked shared worker.
func TestImageCacheFlightLimitsBypass(t *testing.T) {
	release := make(chan struct{})
	var calls atomic.Int32
	cache := newImageCachingTransport(roundTripFunc(func(r *http.Request) (*http.Response, error) {
		calls.Add(1)
		if r.URL.Query().Get("tag") == "" {
			<-release
		}
		return imageCacheTestResponse("image"), nil
	}), nil).(*imageCachingTransport)
	cache.maxFlights = 1
	first := imageCacheAsync(cache, context.Background())
	imageCacheWaitFor(t, func() bool { return imageCacheWaiters(cache) == 1 })
	request := imageCacheTestRequest(http.MethodGet, "http://emby.invalid/emby/Items/123/Images/Primary?tag=other", true)
	response, err := cache.RoundTrip(request)
	imageCacheRead(t, response, err)
	if imageCacheWaiters(cache) != 1 || calls.Load() != 2 {
		t.Fatal("full flight registry did not use transparent bypass")
	}
	close(release)
	if result := <-first; result.err != nil {
		t.Fatal(result.err)
	}
}

// TestImageCacheWorkerPanicReleasesWaiters prevents a custom RoundTripper panic
// from crashing a background worker or leaving the shared key stuck forever.
func TestImageCacheWorkerPanicReleasesWaiters(t *testing.T) {
	var calls atomic.Int32
	cache := newImageCachingTransport(roundTripFunc(func(*http.Request) (*http.Response, error) {
		if calls.Add(1) == 1 {
			panic("fixture transport panic")
		}
		return imageCacheTestResponse("recovered"), nil
	}), nil).(*imageCachingTransport)
	if result := <-imageCacheAsync(cache, context.Background()); result.err == nil {
		t.Fatal("worker panic was treated as successful image")
	}
	if result := <-imageCacheAsync(cache, context.Background()); result.err != nil || result.body != "recovered" {
		t.Fatalf("failed worker blocked replacement: %+v", result)
	}
}

// TestImageCacheForcedRefreshCannotJoinOrdinaryFlight keeps origin refresh
// directives intact even while an ordinary fetch for the same image is busy.
func TestImageCacheForcedRefreshCannotJoinOrdinaryFlight(t *testing.T) {
	for _, directive := range []struct{ header, value string }{
		{"Cache-Control", "no-cache"},
		{"Cache-Control", "max-age=0"},
		{"Pragma", "no-cache"},
	} {
		t.Run(directive.header+"="+directive.value, func(t *testing.T) {
			normalStarted, releaseNormal := make(chan struct{}), make(chan struct{})
			var forcedCalled atomic.Bool
			cache := newImageCachingTransport(roundTripFunc(func(r *http.Request) (*http.Response, error) {
				if r.Header.Get(directive.header) == directive.value {
					forcedCalled.Store(true)
					return imageCacheTestResponse("refreshed-image"), nil
				}
				close(normalStarted)
				<-releaseNormal
				return imageCacheTestResponse("ordinary-image"), nil
			}), nil).(*imageCachingTransport)
			normal := imageCacheAsync(cache, context.Background())
			<-normalStarted
			forced := make(chan imageCacheOutcome, 1)
			go func() {
				r := imageCacheTestRequest(http.MethodGet, "http://emby.invalid/emby/Items/123/Images/Primary", true)
				r.Header.Set(directive.header, directive.value)
				response, err := cache.RoundTrip(r)
				if err != nil {
					forced <- imageCacheOutcome{err: err}
					return
				}
				body, readErr := io.ReadAll(response.Body)
				response.Body.Close()
				forced <- imageCacheOutcome{body: string(body), err: readErr}
			}()
			imageCacheWaitFor(t, func() bool { return forcedCalled.Load() || imageCacheWaiters(cache) == 2 })
			if !forcedCalled.Load() {
				t.Error("forced refresh joined ordinary flight instead of reaching upstream with its directive")
			}
			close(releaseNormal)
			if got := <-forced; got.err != nil || got.body != "refreshed-image" {
				t.Errorf("forced refresh result=%+v", got)
			}
			if got := <-normal; got.err != nil || got.body != "ordinary-image" {
				t.Errorf("ordinary result=%+v", got)
			}
		})
	}
}

// TestImageCacheUncacheable304NeverShared allows one caller to receive a
// validated private/cookie-bearing representation while all others refetch.
func TestImageCacheUncacheable304NeverShared(t *testing.T) {
	for _, policy := range []string{"cookie", "private", "no-store"} {
		t.Run(policy, func(t *testing.T) {
			now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
			var calls atomic.Int32
			release := make(chan struct{})
			cache := newImageCachingTransport(roundTripFunc(func(r *http.Request) (*http.Response, error) {
				call := calls.Add(1)
				if call == 2 {
					<-release
					header := make(http.Header)
					if policy == "cookie" {
						header.Set("Set-Cookie", "fixture-session=rotated")
					} else {
						header.Set("Cache-Control", policy)
					}
					return &http.Response{StatusCode: http.StatusNotModified, Header: header, Body: http.NoBody}, nil
				}
				body := "validated-original"
				if call > 2 {
					body = "independent-refetch"
				}
				response := imageCacheTestResponse(body)
				response.Header.Set("Cache-Control", "max-age=1")
				return response, nil
			}), nil).(*imageCachingTransport)
			cache.now = func() time.Time { return now }
			if got := <-imageCacheAsync(cache, context.Background()); got.err != nil {
				t.Fatal(got.err)
			}
			now = now.Add(2 * time.Second)
			results := make([]<-chan imageCacheOutcome, 6)
			for i := range results {
				results[i] = imageCacheAsync(cache, context.Background())
			}
			imageCacheWaitFor(t, func() bool { return imageCacheWaiters(cache) == 6 })
			close(release)
			validated, rotated := 0, 0
			for _, result := range results {
				got := <-result
				if got.err != nil {
					t.Fatal(got.err)
				}
				if got.body == "validated-original" {
					validated++
				}
				if got.header.Get("Set-Cookie") == "fixture-session=rotated" {
					rotated++
				}
			}
			if validated != 1 || calls.Load() != 7 || len(cache.entries) != 0 {
				t.Errorf("uncacheable 304 shared/refetched incorrectly: validated=%d calls=%d entries=%d", validated, calls.Load(), len(cache.entries))
			}
			if policy == "cookie" && rotated != 1 {
				t.Errorf("rotated cookie delivered to %d callers, want exactly one", rotated)
			}
		})
	}
}
