package directplay

import (
	"context"
	"errors"
	"github.com/konghang/ember/backend/internal/models"
	"github.com/konghang/ember/backend/internal/services/p115account"
	"strings"
	"sync"
	"testing"
	"time"

	p115 "github.com/konghang/ember/backend/internal/integrations/p115"
	"github.com/konghang/ember/backend/internal/services/p115quota"
)

// TestDownloadCacheReplaysAfterStoppedWithNewSession locks down the requested
// behavior while preserving real file lookup and a separate playback lease.
func TestDownloadCacheReplaysAfterStoppedWithNewSession(t *testing.T) {
	provider := newFakeProvider()
	provider.searchResults = [][]p115.File{{provider.targetFile}, {provider.targetFile}}
	leases := p115quota.NewMemoryLeaseStore()
	service := newRoutedDirectPlayForTest(t, &fakeRoutedAccountRuntime{}, provider, leases)
	first := routedMediaPathRequest("GET", "first-play")
	result, err := service.ResolveMediaPath(context.Background(), first)
	if err != nil {
		t.Fatal(err)
	}
	stopped, err := service.HandlePlaybackSessionEvent(context.Background(), PlaybackSessionEvent{
		UserID: first.UserID, MappingID: first.MappingID, DeviceID: first.DeviceID, PlaySessionID: first.PlaySessionID, Stopped: true,
	})
	if err != nil || !stopped.Found {
		t.Fatalf("stop found=%t error=%v", stopped.Found, err)
	}
	replay, err := service.ResolveMediaPath(context.Background(), routedMediaPathRequest("GET", "second-play"))
	if err != nil {
		t.Fatal(err)
	}
	if result.URL != replay.URL {
		t.Fatal("replay did not reuse URL")
	}

	fields := strings.Join(replay.Timing.LogFields(), " ")
	if !strings.Contains(fields, "downloadURLCache=hit") || strings.Contains(fields, "downloadURLCalls=") || strings.Contains(fields, "http") {
		t.Fatalf("replay diagnostics=%s", fields)
	}
	if !strings.Contains(strings.Join(result.Timing.LogFields(), " "), "downloadURLCache=miss") {
		t.Fatal("first call lost cache diagnostic")
	}
	counts := map[string]int{}
	for _, call := range provider.calls {
		counts[call]++
	}
	if counts["download"] != 1 || counts["resolve_source"] != 2 || counts["search_target"] != 2 {
		t.Fatalf("Provider calls = %v; want one download and two real file lookups", counts)
	}
	if replay.Routing.AccountUsage.ReservedStreams != 1 {
		t.Fatalf("replay lease = %+v", replay.Routing.AccountUsage)
	}
}

// TestDownloadCacheIsolation checks the public routed behavior rather than
// merely comparing keys, including content replacement at the same path.
func TestDownloadCacheIsolation(t *testing.T) {
	tests := []struct {
		name   string
		change func(*MediaPathResolveRequest, *fakeRoutedAccountRuntime, *fakeProvider)
	}{
		{"device", func(r *MediaPathResolveRequest, _ *fakeRoutedAccountRuntime, _ *fakeProvider) { r.DeviceID = "other" }},
		{"login", func(r *MediaPathResolveRequest, _ *fakeRoutedAccountRuntime, _ *fakeProvider) { r.MappingID = "other" }},
		{"user", func(r *MediaPathResolveRequest, _ *fakeRoutedAccountRuntime, _ *fakeProvider) { r.UserID = "other" }},
		{"ua", func(r *MediaPathResolveRequest, _ *fakeRoutedAccountRuntime, _ *fakeProvider) {
			r.ClientUserAgent = "other"
		}},
		{"config", func(_ *MediaPathResolveRequest, a *fakeRoutedAccountRuntime, _ *fakeProvider) {
			a.route.ConfigVersion++
		}},
		{"account", func(_ *MediaPathResolveRequest, a *fakeRoutedAccountRuntime, _ *fakeProvider) {
			a.route.AccountID = "other"
		}},
		{"target", func(_ *MediaPathResolveRequest, _ *fakeRoutedAccountRuntime, p *fakeProvider) {
			p.targetFile.ID = "other"
			p.targetFile.PickCode = "other"
		}},
		{"content", func(_ *MediaPathResolveRequest, _ *fakeRoutedAccountRuntime, p *fakeProvider) {
			p.targetFile.Size++
			p.sourceFile.Size++
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			provider := newFakeProvider()
			provider.searchResults = [][]p115.File{{provider.targetFile}}
			accounts := &fakeRoutedAccountRuntime{route: routedPlaybackFixture()}
			service := newRoutedDirectPlayForTest(t, accounts, provider, p115quota.NewMemoryLeaseStore())
			r := routedMediaPathRequest("GET", "first")
			if _, err := service.ResolveMediaPath(context.Background(), r); err != nil {
				t.Fatal(err)
			}
			r.PlaySessionID = "second"
			test.change(&r, accounts, provider)
			provider.searchResults = [][]p115.File{{provider.targetFile}}
			if _, err := service.ResolveMediaPath(context.Background(), r); err != nil {
				t.Fatal(err)
			}
			if countDownloadCalls(provider) != 2 {
				t.Fatalf("download calls=%d", countDownloadCalls(provider))
			}
		})
	}
}

// TestDownloadCacheNeverBypassesAdmission warms a usable URL before exercising
// each gate so a false positive cannot hide behind an empty cache.
func TestDownloadCacheNeverBypassesAdmission(t *testing.T) {
	for _, kind := range []string{"head", "account", "source", "limit", "redis", "final-confirm", "source-deleted", "target-deleted"} {
		t.Run(kind, func(t *testing.T) {
			p := newFakeProvider()
			p.searchResults = [][]p115.File{{p.targetFile}, {p.targetFile}}
			accounts := &fakeRoutedAccountRuntime{route: routedPlaybackFixture()}
			leases := p115quota.NewMemoryLeaseStore()
			service := newRoutedDirectPlayForTest(t, accounts, p, leases)
			if _, err := service.ResolveMediaPath(context.Background(), routedMediaPathRequest("GET", "first")); err != nil {
				t.Fatal(err)
			}
			request := routedMediaPathRequest("GET", "second")
			want := ErrAccountUnavailable
			switch kind {
			case "head":
				request.Method = "HEAD"
				want = ErrHeadLeaseMissing
			case "account":
				accounts.acquireErr = p115account.ErrAccountUnavailable
			case "source":
				accounts.sourceErr = p115account.ErrAccountUnavailable
			case "limit":
				accounts.route.EffectiveMaxConcurrentStreams = 1
				want = ErrAccountConcurrencyExceeded
			case "redis":
				service.leases = &cacheFailingLeaseStore{MemoryLeaseStore: leases, reserveFails: true}
				want = ErrRedisUnavailable
			case "final-confirm":
				service.leases = &cacheFailingLeaseStore{MemoryLeaseStore: leases}
				want = ErrPlaybackLeaseLost
			case "source-deleted":
				p.resolveErr = p115.ErrSourceFileNotFound
				want = ErrProviderProtocol
			case "target-deleted":
				p.searchResults = [][]p115.File{{}, {}}
				p.downloadErr = p115.ErrProviderUnavailable
				want = ErrProviderUnavailable
			}
			result, err := service.ResolveMediaPath(context.Background(), request)
			if err == nil || result.URL != "" {
				t.Fatalf("gate returned candidate=%t err=%v", result.URL != "", err)
			}
			if !errors.Is(err, want) {
				t.Fatalf("error=%v want=%v", err, want)
			}
			expectedCalls := 1
			if kind == "target-deleted" {
				expectedCalls = 2
			}
			if countDownloadCalls(p) != expectedCalls {
				t.Fatalf("download calls=%d", countDownloadCalls(p))
			}
		})
	}
}

type cacheFailingLeaseStore struct {
	*p115quota.MemoryLeaseStore
	reserveFails bool
}

// Reserve simulates Redis failure after the URL was cached.
func (s *cacheFailingLeaseStore) Reserve(ctx context.Context, r p115quota.ReserveRequest, now time.Time) (p115quota.ReserveResult, error) {
	if s.reserveFails {
		return p115quota.ReserveResult{}, errors.New("fixture Redis failure")
	}
	return s.MemoryLeaseStore.Reserve(ctx, r, now)
}

// Confirm simulates Stopped/expiry during real file lookup on a cache hit.
func (s *cacheFailingLeaseStore) Confirm(context.Context, p115quota.ConfirmRequest, time.Time) (p115quota.ConfirmResult, error) {
	return p115quota.ConfirmResult{}, nil
}

// countDownloadCalls safely observes fake Provider calls after concurrent work.
func countDownloadCalls(p *fakeProvider) int {
	p.mu.Lock()
	defer p.mu.Unlock()
	n := 0
	for _, call := range p.calls {
		if call == "download" {
			n++
		}
	}
	return n
}

// cacheDownloadFixture supplies a clean, versioned account and bounded cache.
func cacheDownloadFixture(t *testing.T) (*Service, *fakeProvider, p115account.ActiveAccountCredential, *downloadCacheScope) {
	t.Helper()
	p := newFakeProvider()
	service := newRoutedDirectPlayForTest(t, &fakeRoutedAccountRuntime{}, p, p115quota.NewMemoryLeaseStore())
	p.download.ExpiresAt = service.now().Add(time.Hour)
	account := p115account.ActiveAccountCredential{DownloadCacheVersion: 1, TargetParentID: p.targetFile.ParentID,
		Credential: p115.Credential{AccountID: "playback", Cookie: "fixture-cookie", AppType: "web", UserAgent: "fixture-provider"}}
	return service, p, account, &downloadCacheScope{serverID: "server", userID: "user", mappingID: "mapping", deviceID: "device"}
}

// TestDownloadCacheExpiryAndNoSlidingTTL protects exact expiry and early expiry
// safety without sleeping or relying on the wall clock.
func TestDownloadCacheExpiryAndNoSlidingTTL(t *testing.T) {
	for _, lifetime := range []time.Duration{time.Hour, 25 * time.Second} {
		t.Run(lifetime.String(), func(t *testing.T) {
			s, p, a, scope := cacheDownloadFixture(t)
			now := s.now()
			s.now = func() time.Time { return now }
			p.download.ExpiresAt = now.Add(lifetime)
			get := func() RedirectCandidate {
				c, e := s.downloadCandidate(context.Background(), a, p.targetFile, "ua", "", true, scope)
				if e != nil {
					t.Fatal(e)
				}
				return c
			}
			get()
			window := downloadCacheTTL
			if lifetime-downloadCacheSafetyWindow < window {
				window = lifetime - downloadCacheSafetyWindow
			}
			now = now.Add(window - time.Nanosecond)
			if !get().downloadCacheHit {
				t.Fatal("missed before deadline")
			}
			now = now.Add(time.Nanosecond)
			if get().downloadCacheHit {
				t.Fatal("hit at deadline")
			}
			if countDownloadCalls(p) != 2 {
				t.Fatalf("calls=%d", countDownloadCalls(p))
			}
		})
	}
}

// TestDownloadCacheDoesNotRetainInvalidOrFailedResults covers recovery after
// failure and bypass for half-open/unknown account snapshots.
func TestDownloadCacheDoesNotRetainInvalidOrFailedResults(t *testing.T) {
	for _, kind := range []string{"failure", "expired", "cookie-required", "zero-concurrency", "short-lived", "oversize", "http", "cancel", "probe"} {
		t.Run(kind, func(t *testing.T) {
			s, p, a, scope := cacheDownloadFixture(t)
			good := p.download
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			switch kind {
			case "failure":
				p.downloadErr = p115.ErrProviderUnavailable
			case "expired":
				p.download.ExpiresAt = s.now()
			case "cookie-required":
				p.download.HeaderMode = p115.DownloadHeadersSameUserAgentAndCookie
			case "zero-concurrency":
				p.download.ConcurrentOpenLimit = 0
			case "short-lived":
				p.download.ExpiresAt = s.now().Add(10 * time.Second)
			case "oversize":
				p.download.URL = "https://cdn.115.com/" + strings.Repeat("x", maxCachedDownloadURLBytes)
			case "http":
				p.download.URL = "http://cdn.115.com/fixture"
			case "cancel":
				s.provider = &lifecycleProvider{fakeProvider: p, downloadStep: cancel}
			case "probe":
				a.DownloadCacheVersion = 0
			}
			_, _ = s.downloadCandidate(ctx, a, p.targetFile, "ua", "", true, scope)
			p.download = good
			p.downloadErr = nil
			s.provider = p
			a.DownloadCacheVersion = 1
			result, err := s.downloadCandidate(context.Background(), a, p.targetFile, "ua", "", true, scope)
			if err != nil || result.downloadCacheHit || countDownloadCalls(p) != 2 {
				t.Fatalf("recovery hit=%t calls=%d err=%v", result.downloadCacheHit, countDownloadCalls(p), err)
			}
		})
	}
}

// TestDownloadCacheLRUEvictionAndMetadata ensures eviction is bounded and a
// reused address cannot copy another request's transfer provenance.
func TestDownloadCacheLRUEvictionAndMetadata(t *testing.T) {
	s, p, a, scope := cacheDownloadFixture(t)
	s.downloadCache = newDownloadURLCache(2)
	get := func(ua, id string, preexisting bool) RedirectCandidate {
		r, e := s.downloadCandidate(context.Background(), a, p.targetFile, ua, id, preexisting, scope)
		if e != nil {
			t.Fatal(e)
		}
		return r
	}
	get("a", "old-task", false)
	get("b", "", true)
	hit := get("a", "new-task", true)
	if !hit.downloadCacheHit || hit.TaskID != "new-task" || !hit.Preexisting {
		t.Fatal("cached task metadata leaked")
	}
	get("c", "", true)
	get("b", "", true)
	if countDownloadCalls(p) != 4 || len(s.downloadCache.entries) != 2 {
		t.Fatalf("calls=%d size=%d", countDownloadCalls(p), len(s.downloadCache.entries))
	}
}

// TestDownloadCacheHitDoesNotReportPlaybackRecovery preserves real source
// observations but never claims the cached download endpoint was probed again.
func TestDownloadCacheHitDoesNotReportPlaybackRecovery(t *testing.T) {
	health := &fakeAccountHealthReporter{}
	accounts := &fakeRoutedAccountRuntime{fakeAccountLoader: fakeAccountLoader{health: health}}
	p := newFakeProvider()
	p.searchResults = [][]p115.File{{p.targetFile}, {p.targetFile}}
	s := newRoutedDirectPlayForTest(t, accounts, p, p115quota.NewMemoryLeaseStore())
	for _, session := range []string{"first", "second"} {
		if _, e := s.ResolveMediaPath(context.Background(), routedMediaPathRequest("GET", session)); e != nil {
			t.Fatal(e)
		}
	}
	source, playback := 0, 0
	for _, event := range health.events {
		if event.role == models.P115AccountRoleSource {
			source++
		} else {
			playback++
		}
	}
	if source != 2 || playback != 1 {
		t.Fatalf("health source=%d playback=%d", source, playback)
	}
}

// TestDownloadCacheConcurrentReuseAndCanceledWaiter verifies per-key waiting
// does not duplicate successful downloads or let a waiter cancel the owner.
func TestDownloadCacheConcurrentReuseAndCanceledWaiter(t *testing.T) {
	s, p, a, scope := cacheDownloadFixture(t)
	entered, release := make(chan struct{}), make(chan struct{})
	s.provider = &lifecycleProvider{fakeProvider: p, downloadStep: func() { close(entered); <-release }}
	owner := make(chan error, 1)
	go func() {
		_, e := s.downloadCandidate(context.Background(), a, p.targetFile, "ua", "", true, scope)
		owner <- e
	}()
	<-entered
	waiterCtx, cancel := context.WithCancel(context.Background())
	waiter := make(chan error, 1)
	go func() { _, e := s.downloadCandidate(waiterCtx, a, p.targetFile, "ua", "", true, scope); waiter <- e }()
	deadline := time.Now().Add(time.Second)
	for {
		s.downloadCache.flights.mu.Lock()
		entry := s.downloadCache.flights.entries[scope.downloadKey(a, p.targetFile, "ua")]
		queued := entry != nil && entry.refs == 2
		s.downloadCache.flights.mu.Unlock()
		if queued {
			break
		}
		if time.Now().After(deadline) {
			cancel()
			close(release)
			t.Fatal("waiter never queued")
		}
		time.Sleep(time.Millisecond)
	}
	cancel()
	if e := <-waiter; !errors.Is(e, context.Canceled) {
		t.Fatalf("waiter=%v", e)
	}
	var group sync.WaitGroup
	for i := 0; i < 8; i++ {
		group.Add(1)
		go func() {
			defer group.Done()
			c, e := s.downloadCandidate(context.Background(), a, p.targetFile, "ua", "", true, scope)
			if e != nil || !c.downloadCacheHit {
				t.Errorf("waiter hit=%t err=%v", c.downloadCacheHit, e)
			}
		}()
	}
	close(release)
	if e := <-owner; e != nil {
		t.Fatal(e)
	}
	group.Wait()
	if countDownloadCalls(p) != 1 || len(s.downloadCache.flights.entries) != 0 {
		t.Fatalf("calls=%d", countDownloadCalls(p))
	}
}

// TestDownloadCacheWarmProbeAndCredentialChange checks bypass against a warm
// cache and protects credential replacement even if a fake reuses the version.
func TestDownloadCacheWarmProbeAndCredentialChange(t *testing.T) {
	for _, kind := range []string{"probe", "cookie", "server"} {
		t.Run(kind, func(t *testing.T) {
			s, p, a, scope := cacheDownloadFixture(t)
			if _, e := s.downloadCandidate(context.Background(), a, p.targetFile, "ua", "", true, scope); e != nil {
				t.Fatal(e)
			}
			switch kind {
			case "probe":
				a.DownloadCacheVersion = 0
			case "cookie":
				a.Credential.Cookie = "replacement-fixture"
			case "server":
				scope.serverID = "other"
			}
			r, e := s.downloadCandidate(context.Background(), a, p.targetFile, "ua", "", true, scope)
			if e != nil || r.downloadCacheHit || countDownloadCalls(p) != 2 {
				t.Fatalf("hit=%t calls=%d err=%v", r.downloadCacheHit, countDownloadCalls(p), e)
			}
		})
	}
}

// TestDownloadCacheHeadReuseDoesNotExtendLease proves HEAD may reuse the URL
// only within the existing session, without changing its reservation deadline.
func TestDownloadCacheHeadReuseDoesNotExtendLease(t *testing.T) {
	p := newFakeProvider()
	p.searchResults = [][]p115.File{{p.targetFile}, {p.targetFile}}
	leases := p115quota.NewMemoryLeaseStore()
	s := newRoutedDirectPlayForTest(t, &fakeRoutedAccountRuntime{}, p, leases)
	now := s.now()
	s.now = func() time.Time { return now }
	request := routedMediaPathRequest("GET", "head-session")
	if _, e := s.ResolveMediaPath(context.Background(), request); e != nil {
		t.Fatal(e)
	}
	fingerprint, _ := s.keyDeriver.SessionFingerprint(p115quota.SessionIdentity{ServerID: s.serverID, UserID: request.UserID, MappingID: request.MappingID, DeviceID: request.DeviceID, PlaySessionID: request.PlaySessionID})
	before, _, _ := leases.Session(context.Background(), fingerprint, now)
	now = now.Add(time.Second)
	request.Method = "HEAD"
	c, e := s.ResolveMediaPath(context.Background(), request)
	if e != nil || !c.downloadCacheHit {
		t.Fatalf("HEAD hit=%t err=%v", c.downloadCacheHit, e)
	}
	after, _, _ := leases.Session(context.Background(), fingerprint, now)
	if !before.ExpiresAt.Equal(after.ExpiresAt) {
		t.Fatal("HEAD extended reservation")
	}
}

// TestDownloadCacheOwnerCancellationDoesNotPoisonRetry covers a canceled owner
// independently of canceled waiters; a later request can fetch and cache anew.
func TestDownloadCacheOwnerCancellationDoesNotPoisonRetry(t *testing.T) {
	s, p, a, scope := cacheDownloadFixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	entered, release := make(chan struct{}), make(chan struct{})
	s.provider = &lifecycleProvider{fakeProvider: p, downloadStep: func() { close(entered); <-release }}
	owner := make(chan error, 1)
	go func() { _, e := s.downloadCandidate(ctx, a, p.targetFile, "ua", "", true, scope); owner <- e }()
	<-entered
	cancel()
	close(release)
	if e := <-owner; !errors.Is(e, context.Canceled) {
		t.Fatalf("owner=%v", e)
	}
	s.provider = p
	c, e := s.downloadCandidate(context.Background(), a, p.targetFile, "ua", "", true, scope)
	if e != nil || c.downloadCacheHit || countDownloadCalls(p) != 2 {
		t.Fatalf("retry hit=%t err=%v", c.downloadCacheHit, e)
	}
}

// TestDownloadCacheAfterFirstTransferDoesNotReuseTaskOrQuota ensures a replay
// reuses only the URL while fresh lookup determines the current file outcome.
func TestDownloadCacheAfterFirstTransferDoesNotReuseTaskOrQuota(t *testing.T) {
	p := newFakeProvider()
	p.searchResults = [][]p115.File{{}, {}, {p.targetFile}}
	leases := p115quota.NewMemoryLeaseStore()
	s := newRoutedDirectPlayForTest(t, &fakeRoutedAccountRuntime{}, p, leases)
	first, e := s.ResolveMediaPath(context.Background(), routedMediaPathRequest("GET", "first"))
	if e != nil || first.Preexisting || first.TaskID == "" {
		t.Fatalf("first preexisting=%t taskPresent=%t err=%v", first.Preexisting, first.TaskID != "", e)
	}
	second, e := s.ResolveMediaPath(context.Background(), routedMediaPathRequest("GET", "second"))
	if e != nil || !second.downloadCacheHit || !second.Preexisting || second.TaskID != "" || second.Routing.TransferChecked {
		t.Fatalf("replay hit=%t preexisting=%t taskPresent=%t checked=%t err=%v", second.downloadCacheHit, second.Preexisting, second.TaskID != "", second.Routing.TransferChecked, e)
	}
	if countDownloadCalls(p) != 1 {
		t.Fatalf("downloads=%d", countDownloadCalls(p))
	}
}

// TestDownloadCacheClockRollbackDropsFutureEntry prevents a backward clock
// adjustment from extending a URL's fixed local reuse window.
func TestDownloadCacheClockRollbackDropsFutureEntry(t *testing.T) {
	s, p, a, scope := cacheDownloadFixture(t)
	now := s.now()
	s.now = func() time.Time { return now }
	if _, e := s.downloadCandidate(context.Background(), a, p.targetFile, "ua", "", true, scope); e != nil {
		t.Fatal(e)
	}
	now = now.Add(-time.Second)
	c, e := s.downloadCandidate(context.Background(), a, p.targetFile, "ua", "", true, scope)
	if e != nil || c.downloadCacheHit || countDownloadCalls(p) != 2 {
		t.Fatalf("rollback hit=%t err=%v", c.downloadCacheHit, e)
	}
}
