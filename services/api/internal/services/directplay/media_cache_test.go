package directplay

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	p115 "github.com/konghang/ember/backend/internal/integrations/p115"
	"github.com/konghang/ember/backend/internal/models"
	"github.com/konghang/ember/backend/internal/services/p115account"
	"github.com/konghang/ember/backend/internal/services/p115quota"
)

type mediaAccountRuntime struct {
	*fakeRoutedAccountRuntime
	change func(*p115account.ActiveAccountCredential)
}

// LoadActiveCredentialByRole changes only the source snapshot for isolation tests.
func (a *mediaAccountRuntime) LoadActiveCredentialByRole(ctx context.Context, role models.P115AccountRole) (p115account.ActiveAccountCredential, error) {
	account, err := a.fakeRoutedAccountRuntime.LoadActiveCredentialByRole(ctx, role)
	if err == nil && a.change != nil {
		a.change(&account)
	}
	return account, err
}

// TestMediaCacheSourceIsolation protects both account credentials and recovery
// state, even when the playback URL cache can still reuse the final address.
func TestMediaCacheSourceIsolation(t *testing.T) {
	for _, kind := range []string{"cookie", "version", "probe", "provider-ua", "provider-app"} {
		t.Run(kind, func(t *testing.T) {
			p := newFakeProvider()
			p.searchResults = [][]p115.File{{p.targetFile}, {p.targetFile}}
			a := &fakeRoutedAccountRuntime{}
			s := newRoutedDirectPlayForTest(t, a, p, p115quota.NewMemoryLeaseStore())
			accounts := &mediaAccountRuntime{fakeRoutedAccountRuntime: a}
			s.accounts = accounts
			if _, err := s.ResolveMediaPath(context.Background(), routedMediaPathRequest("GET", "first")); err != nil {
				t.Fatal(err)
			}
			accounts.change = func(a *p115account.ActiveAccountCredential) {
				switch kind {
				case "cookie":
					a.Credential.Cookie = "replacement-fixture"
				case "version":
					a.DownloadCacheVersion++
				case "probe":
					a.DownloadCacheVersion = 0
				case "provider-ua":
					a.Credential.UserAgent = "new-provider-agent"
				case "provider-app":
					a.Credential.AppType = "android"
				}
			}
			result, err := s.ResolveMediaPath(context.Background(), routedMediaPathRequest("GET", "second"))
			if err != nil {
				t.Fatal(err)
			}
			fields := strings.Join(result.Timing.LogFields(), " ")
			if !strings.Contains(fields, "sourceResolveCalls=1") || strings.Contains(fields, "mediaResolutionCache=hit") {
				t.Fatalf("diagnostics=%s", fields)
			}
		})
	}
}

// TestMediaCacheFixedDeadline makes external deletion visible at expiry and
// proves hits neither slide the media window nor exceed URL-cache lifetime.
func TestMediaCacheFixedDeadline(t *testing.T) {
	for _, lifetime := range []time.Duration{time.Hour, 25 * time.Second} {
		t.Run(lifetime.String(), func(t *testing.T) {
			p := newFakeProvider()
			p.searchResults = [][]p115.File{{p.targetFile}}
			s := newRoutedDirectPlayForTest(t, &fakeRoutedAccountRuntime{}, p, p115quota.NewMemoryLeaseStore())
			now := s.now()
			s.now = func() time.Time { return now }
			p.download.ExpiresAt = now.Add(lifetime)
			r := routedMediaPathRequest("GET", "first")
			if _, err := s.ResolveMediaPath(context.Background(), r); err != nil {
				t.Fatal(err)
			}
			p.resolveErr = p115.ErrSourceFileNotFound
			window := downloadCacheTTL
			if lifetime-downloadCacheSafetyWindow < window {
				window = lifetime - downloadCacheSafetyWindow
			}
			now = now.Add(window - time.Nanosecond)
			r.PlaySessionID = "second"
			result, err := s.ResolveMediaPath(context.Background(), r)
			if err != nil || !strings.Contains(strings.Join(result.Timing.LogFields(), " "), "mediaResolutionCache=hit") {
				t.Fatalf("before expiry: %v", err)
			}
			if result.TaskID != "" || result.Routing.TransferChecked || result.Routing.AccountUsage.OccupiedStreams != 2 {
				t.Fatal("cache copied request metadata or bypassed new session admission")
			}
			now = now.Add(time.Nanosecond)
			result, err = s.ResolveMediaPath(context.Background(), r)
			if !errors.Is(err, ErrProviderProtocol) || result.URL != "" {
				t.Fatalf("expiry error=%v", err)
			}
		})
	}
}

// TestMediaCacheKeepsOriginalURLDeadline covers refilling metadata after source
// configuration changes while the older download entry is still reusable.
func TestMediaCacheKeepsOriginalURLDeadline(t *testing.T) {
	p := newFakeProvider()
	p.searchResults = [][]p115.File{{p.targetFile}, {p.targetFile}, {p.targetFile}}
	a := &fakeRoutedAccountRuntime{}
	s := newRoutedDirectPlayForTest(t, a, p, p115quota.NewMemoryLeaseStore())
	now := s.now()
	s.now = func() time.Time { return now }
	r := routedMediaPathRequest("GET", "session")
	if _, err := s.ResolveMediaPath(context.Background(), r); err != nil {
		t.Fatal(err)
	}
	s.accounts = &mediaAccountRuntime{fakeRoutedAccountRuntime: a, change: func(a *p115account.ActiveAccountCredential) { a.DownloadCacheVersion++ }}
	now = now.Add(5 * time.Second)
	if _, err := s.ResolveMediaPath(context.Background(), r); err != nil {
		t.Fatal(err)
	}
	now = now.Add(25 * time.Second)
	result, err := s.ResolveMediaPath(context.Background(), r)
	if err != nil || strings.Contains(strings.Join(result.Timing.LogFields(), " "), "mediaResolutionCache=hit") || countDownloadCalls(p) != 2 {
		t.Fatalf("original deadline lost: err=%v downloads=%d", err, countDownloadCalls(p))
	}
}

// TestMediaCacheFailureDoesNotPublish ensures failed final admission and
// canceled Provider work cannot populate the cross-session metadata cache.
func TestMediaCacheFailureDoesNotPublish(t *testing.T) {
	for _, kind := range []string{"final-confirm", "cancel", "provider"} {
		t.Run(kind, func(t *testing.T) {
			p := newFakeProvider()
			p.searchResults = [][]p115.File{{p.targetFile}, {p.targetFile}}
			leases := p115quota.NewMemoryLeaseStore()
			s := newRoutedDirectPlayForTest(t, &fakeRoutedAccountRuntime{}, p, leases)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			switch kind {
			case "final-confirm":
				s.leases = &cacheFailingLeaseStore{MemoryLeaseStore: leases}
			case "cancel":
				s.provider = &lifecycleProvider{fakeProvider: p, downloadStep: cancel}
			case "provider":
				p.downloadErr = p115.ErrProviderUnavailable
			}
			if _, err := s.ResolveMediaPath(ctx, routedMediaPathRequest("GET", "first")); err == nil {
				t.Fatal("expected failure")
			}
			if len(s.mediaCache.entries) != 0 {
				t.Fatal("failed request published media cache")
			}
			s.leases, s.provider, p.downloadErr = leases, p, nil
			result, err := s.ResolveMediaPath(context.Background(), routedMediaPathRequest("GET", "second"))
			if err != nil || !strings.Contains(strings.Join(result.Timing.LogFields(), " "), "sourceResolveCalls=1") {
				t.Fatalf("retry=%v", err)
			}
		})
	}
}

// TestMediaCacheMissingURLAndDifferentPath performs real lookups when either
// the referenced URL was evicted or a different media path is requested.
func TestMediaCacheMissingURLAndDifferentPath(t *testing.T) {
	for _, kind := range []string{"evicted-url", "other-path"} {
		t.Run(kind, func(t *testing.T) {
			p := newFakeProvider()
			p.searchResults = [][]p115.File{{p.targetFile}, {p.targetFile}}
			s := newRoutedDirectPlayForTest(t, &fakeRoutedAccountRuntime{}, p, p115quota.NewMemoryLeaseStore())
			r := routedMediaPathRequest("GET", "first")
			if _, err := s.ResolveMediaPath(context.Background(), r); err != nil {
				t.Fatal(err)
			}
			r.PlaySessionID = "second"
			if kind == "evicted-url" {
				s.downloadCache = newDownloadURLCache(downloadCacheCapacity)
			} else {
				r.Path = "/mnt/cloudNAS/115lifetime/Media/other.mkv"
			}
			result, err := s.ResolveMediaPath(context.Background(), r)
			if err != nil {
				t.Fatal(err)
			}
			fields := strings.Join(result.Timing.LogFields(), " ")
			if !strings.Contains(fields, "mediaResolutionCache=miss") || !strings.Contains(fields, "sourceResolveCalls=1") || !strings.Contains(fields, "targetSearchCalls=1") {
				t.Fatalf("lookups missing: %s", fields)
			}
		})
	}
}

// TestMediaCacheBoundAndClockRollback locks down bounded retention and rejects
// entries written in the future after a backward clock adjustment.
func TestMediaCacheBoundAndClockRollback(t *testing.T) {
	cache := newMediaResolutionCache(2)
	now := time.Now()
	c := RedirectCandidate{downloadCacheKey: "opaque", resolvedSHA1: directPlaySourceSHA1, resolvedSize: 1}
	cache.put("a", c, now, now, now.Add(time.Minute))
	cache.put("b", c, now, now, now.Add(10*time.Second))
	cache.put("c", c, now, now, now.Add(time.Minute))
	if _, found := cache.get("b", now); found || len(cache.entries) != 2 {
		t.Fatal("earliest-expiry eviction failed")
	}
	if _, found := cache.get("a", now.Add(-time.Nanosecond)); found {
		t.Fatal("clock rollback accepted entry")
	}
}

// TestMediaCacheConcurrentSessionsShareLookups keeps separate reservations
// while a canceled waiter cannot cancel the owner's Provider operation.
func TestMediaCacheConcurrentSessionsShareLookups(t *testing.T) {
	p := newFakeProvider()
	p.searchResults = [][]p115.File{{p.targetFile}}
	a := &fakeRoutedAccountRuntime{route: routedPlaybackFixture()}
	a.route.EffectiveMaxConcurrentStreams = 3
	s := newRoutedDirectPlayForTest(t, a, p, p115quota.NewMemoryLeaseStore())
	entered, release := make(chan struct{}), make(chan struct{})
	s.provider = &lifecycleProvider{fakeProvider: p, downloadStep: func() { close(entered); <-release }}
	owner := make(chan error, 1)
	go func() {
		_, err := s.ResolveMediaPath(context.Background(), routedMediaPathRequest("GET", "owner"))
		owner <- err
	}()
	<-entered
	ctx, cancel := context.WithCancel(context.Background())
	waiter := make(chan error, 1)
	go func() { _, err := s.ResolveMediaPath(ctx, routedMediaPathRequest("GET", "waiter")); waiter <- err }()
	key := mediaRequestKey(s.serverID, routedMediaPathRequest("GET", "owner"))
	deadline := time.Now().Add(time.Second)
	for {
		s.mediaCache.flights.mu.Lock()
		e := s.mediaCache.flights.entries[key]
		queued := e != nil && e.refs == 2
		s.mediaCache.flights.mu.Unlock()
		if queued {
			break
		}
		if time.Now().After(deadline) {
			cancel()
			close(release)
			<-owner
			t.Fatal("waiter not queued")
		}
		time.Sleep(time.Millisecond)
	}
	cancel()
	if err := <-waiter; !errors.Is(err, context.Canceled) {
		close(release)
		<-owner
		t.Fatalf("waiter=%v", err)
	}
	next := make(chan RedirectCandidate, 1)
	nextErr := make(chan error, 1)
	go func() {
		c, err := s.ResolveMediaPath(context.Background(), routedMediaPathRequest("GET", "next"))
		next <- c
		nextErr <- err
	}()
	close(release)
	if err := <-owner; err != nil {
		t.Fatal(err)
	}
	c := <-next
	if err := <-nextErr; err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(strings.Join(c.Timing.LogFields(), " "), "mediaResolutionCache=hit") || c.Routing.AccountUsage.OccupiedStreams != 2 {
		t.Fatal("waiter failed to reuse media with independent admission")
	}
	if countDownloadCalls(p) != 1 || len(s.mediaCache.flights.entries) != 0 {
		t.Fatal("duplicate download or leaked gate")
	}
}
