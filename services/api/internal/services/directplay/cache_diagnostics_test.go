package directplay

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	p115 "github.com/konghang/ember/backend/internal/integrations/p115"
	"github.com/konghang/ember/backend/internal/services/p115quota"
)

// cacheDiagnosticFields rejects duplicate fields while inspecting the same
// safe timing output consumed by the single Gateway decision log.
func cacheDiagnosticFields(t *testing.T, timing TimingDiagnostics) map[string]string {
	t.Helper()
	fields := make(map[string]string)
	for _, field := range timing.LogFields() {
		key, value, ok := strings.Cut(field, "=")
		if _, exists := fields[key]; !ok || exists {
			t.Fatalf("invalid or duplicate diagnostic field %q", field)
		}
		fields[key] = value
	}
	return fields
}

// TestCacheDiagnosticsDownloadReasons protects direct evidence for misses,
// bypasses and forced refresh without changing Provider work or cache policy.
func TestCacheDiagnosticsDownloadReasons(t *testing.T) {
	for _, name := range []string{"hit", "entry_missing", "config_changed", "expired", "clock_rollback", "host_rejected", "key_unavailable", "cache_unavailable", "forced_refresh"} {
		t.Run(name, func(t *testing.T) {
			s, p, account, scope := cacheDownloadFixture(t)
			now := s.now()
			s.now = func() time.Time { return now }
			if _, err := s.downloadCandidate(context.Background(), account, p.targetFile, "ua", "old-task", true, scope); err != nil {
				t.Fatal(err)
			}
			key := scope.downloadKey(account, p.targetFile, "ua")
			entry, _ := s.downloadCache.getEntry(key, now)
			preexisting, outcome, reason := true, "miss", name
			switch name {
			case "hit":
				outcome, reason = "hit", ""
			case "entry_missing":
				s.downloadCache.remove(key)
			case "config_changed":
				account.DownloadCacheVersion++
				reason = "entry_missing"
			case "expired":
				now = entry.expiresAt
			case "clock_rollback":
				now = now.Add(-time.Nanosecond)
			case "host_rejected":
				entry.result.URL = "https://rejected.invalid/fixture-secret"
				s.downloadCache.entries[key].Value = entry
			case "key_unavailable":
				account.DownloadCacheVersion = 0
				outcome = "bypass"
			case "cache_unavailable":
				s.downloadCache = nil
				outcome = "bypass"
			case "forced_refresh":
				preexisting = false
				p.download.URL = "https://cdn.115.com/refreshed-fixture"
			}
			ctx, recorder := withTiming(context.Background())
			candidate, err := s.downloadCandidate(ctx, account, p.targetFile, "ua", "current-task", preexisting, scope)
			if err != nil || candidate.downloadCacheHit != (outcome == "hit") || candidate.TaskID != "current-task" || candidate.Preexisting != preexisting {
				t.Fatalf("candidate hit=%t task=%q preexisting=%t err=%v", candidate.downloadCacheHit, candidate.TaskID, candidate.Preexisting, err)
			}
			fields := cacheDiagnosticFields(t, recorder.finish())
			if fields["downloadURLCache"] != outcome || fields["downloadURLCacheReason"] != reason {
				t.Fatalf("diagnostics=%v want=%s/%s", fields, outcome, reason)
			}
			_, counted := fields["downloadCacheEvictions"]
			if counted != (outcome != "bypass") || (counted && fields["downloadCacheEvictions"] != "0") {
				t.Fatalf("unexpected eviction snapshot: %v", fields)
			}
			calls := 2
			if outcome == "hit" {
				calls = 1
			}
			if countDownloadCalls(p) != calls {
				t.Fatalf("Provider calls=%d want=%d", countDownloadCalls(p), calls)
			}
			if name == "forced_refresh" && candidate.URL != p.download.URL {
				t.Fatal("fresh transfer reused the old signed URL")
			}
		})
	}
}

// TestCacheDiagnosticsMediaReasons separates metadata freshness from its URL
// dependency, retaining the existing observer outcomes and active-session rule.
func TestCacheDiagnosticsMediaReasons(t *testing.T) {
	for _, name := range []string{"hit", "active_session", "entry_missing", "expired", "clock_rollback", "freshness_expired", "different_session", "dependent_url_missing", "dependent_url_expired", "dependent_url_clock_rollback", "dependent_url_host_rejected", "key_unavailable", "cache_unavailable", "download_cache_unavailable"} {
		t.Run(name, func(t *testing.T) {
			s, p, _, _ := cacheDownloadFixture(t)
			now := s.now()
			key, downloadKey := "media-fixture", "download-fixture"
			s.downloadCache.put(downloadKey, p.download, now)
			entry, _ := s.downloadCache.getEntry(downloadKey, now)
			candidate := RedirectCandidate{downloadCacheKey: downloadKey, resolvedSHA1: directPlaySourceSHA1, resolvedSize: 1}
			s.mediaCache.put(key, candidate, now, now, entry.expiresAt, "original-session")
			activeSession, outcome, reason := "", "miss", name
			switch name {
			case "hit":
				outcome, reason = "hit", ""
			case "active_session":
				now = now.Add(mediaResolutionFreshnessTTL)
				activeSession, outcome, reason = "original-session", "hit", ""
			case "entry_missing":
				key = "different-media-fixture"
			case "expired":
				now = entry.expiresAt
			case "clock_rollback":
				now = now.Add(-time.Nanosecond)
			case "freshness_expired", "different_session":
				now = now.Add(mediaResolutionFreshnessTTL)
				reason = "freshness_expired"
				if name == "different_session" {
					activeSession = "other-session"
				}
			case "dependent_url_missing":
				s.downloadCache.remove(downloadKey)
			case "dependent_url_expired":
				shortURL := p.download
				shortURL.ExpiresAt = now.Add(10 * time.Second)
				s.downloadCache.put(downloadKey, shortURL, now)
				now = now.Add(9 * time.Second)
			case "dependent_url_clock_rollback":
				s.downloadCache.put(downloadKey, p.download, now.Add(time.Second))
			case "dependent_url_host_rejected":
				entry.result.URL = "https://rejected.invalid/fixture-secret"
				s.downloadCache.entries[downloadKey].Value = entry
			case "key_unavailable":
				key, outcome = "", "bypass"
			case "cache_unavailable":
				s.mediaCache, outcome = nil, "bypass"
			case "download_cache_unavailable":
				s.downloadCache, outcome, reason = nil, "bypass", "cache_unavailable"
			}
			var observed []string
			ctx := WithStepObserver(context.Background(), func(step, phase string, _ int64) {
				observed = append(observed, step+"="+phase)
			})
			ctx, recorder := withTiming(ctx)
			got, hit := s.cachedMediaCandidate(ctx, key, now, activeSession)
			if hit != (outcome == "hit") || (hit && (got.URL != p.download.URL || !got.Preexisting || got.TaskID != "")) {
				t.Fatalf("unexpected media hit=%t metadata=%+v", hit, got)
			}
			fields := cacheDiagnosticFields(t, recorder.finish())
			if fields["mediaResolutionCache"] != outcome || fields["mediaResolutionCacheReason"] != reason {
				t.Fatalf("diagnostics=%v want=%s/%s", fields, outcome, reason)
			}
			_, mediaCounted := fields["mediaCacheEvictions"]
			_, downloadCounted := fields["downloadCacheEvictions"]
			if mediaCounted != (outcome != "bypass") || downloadCounted != (hit || strings.HasPrefix(name, "dependent_url_")) {
				t.Fatalf("unexpected observed cache snapshots: %v", fields)
			}
			if (mediaCounted && fields["mediaCacheEvictions"] != "0") || (downloadCounted && fields["downloadCacheEvictions"] != "0") {
				t.Fatalf("non-capacity removal changed eviction counts: %v", fields)
			}
			wantObserved := "mediaResolutionCache=" + outcome
			if hit {
				wantObserved = "mediaResolutionCache=miss,mediaResolutionCache=hit,downloadURLCache=hit"
			}
			if strings.Join(observed, ",") != wantObserved {
				t.Fatalf("observer=%v want=%s", observed, wantObserved)
			}
			if countDownloadCalls(p) != 0 || fields["downloadURLCalls"] != "" || fields["sourceResolveCalls"] != "" {
				t.Fatal("cache diagnostics invented Provider work")
			}
			if (name == "clock_rollback" || name == "expired") && len(s.mediaCache.entries) != 0 {
				t.Fatal("invalid media entry was retained")
			}
			if reason == "freshness_expired" && len(s.mediaCache.entries) != 1 {
				t.Fatal("cross-session freshness rejection discarded retained original-session metadata")
			}
		})
	}
}

// TestCacheDiagnosticsFailedForcedRefresh removes stale URLs even when the
// new transfer's download fails; removal is not a capacity eviction.
func TestCacheDiagnosticsFailedForcedRefresh(t *testing.T) {
	s, p, account, scope := cacheDownloadFixture(t)
	if _, err := s.downloadCandidate(context.Background(), account, p.targetFile, "ua", "", true, scope); err != nil {
		t.Fatal(err)
	}
	p.downloadErr = p115.ErrProviderUnavailable
	ctx, recorder := withTiming(context.Background())
	_, err := s.downloadCandidate(ctx, account, p.targetFile, "ua", "new-task", false, scope)
	fields := cacheDiagnosticFields(t, recorder.finish())
	if !errors.Is(err, ErrProviderUnavailable) || fields["downloadURLCacheReason"] != "forced_refresh" || fields["downloadCacheEvictions"] != "0" || len(s.downloadCache.entries) != 0 {
		t.Fatalf("failed refresh err=%v fields=%v retained=%d", err, fields, len(s.downloadCache.entries))
	}
}

// TestCacheDiagnosticsResolveKeepsProviderWork verifies reason propagation on
// the public resolver and omission of Provider stage counts on a media hit.
func TestCacheDiagnosticsResolveKeepsProviderWork(t *testing.T) {
	p := newFakeProvider()
	p.searchResults = [][]p115.File{{p.targetFile}}
	s := newRoutedDirectPlayForTest(t, &fakeRoutedAccountRuntime{}, p, p115quota.NewMemoryLeaseStore())
	first, err := s.ResolveMediaPath(context.Background(), routedMediaPathRequest("GET", "first"))
	if err != nil {
		t.Fatal(err)
	}
	fields := cacheDiagnosticFields(t, first.Timing)
	for _, key := range []string{"mediaResolutionCacheReason", "downloadURLCacheReason"} {
		if fields[key] != "entry_missing" {
			t.Fatalf("cold resolver diagnostics=%v", fields)
		}
	}
	second, err := s.ResolveMediaPath(context.Background(), routedMediaPathRequest("GET", "second"))
	if err != nil {
		t.Fatal(err)
	}
	fields = cacheDiagnosticFields(t, second.Timing)
	if fields["mediaResolutionCache"] != "hit" || fields["downloadURLCache"] != "hit" || countDownloadCalls(p) != 1 {
		t.Fatalf("warm resolver diagnostics=%v downloads=%d", fields, countDownloadCalls(p))
	}
	for _, key := range []string{"mediaResolutionCacheReason", "downloadURLCacheReason", "sourceResolveCalls", "targetSearchCalls", "downloadURLCalls"} {
		if _, found := fields[key]; found {
			t.Fatalf("media hit emitted %s", key)
		}
	}
}

// TestCacheDiagnosticsCapacitySnapshots counts only capacity pressure, keeps
// the original expiry/LRU choices and exposes a cumulative, not per-call value.
func TestCacheDiagnosticsCapacitySnapshots(t *testing.T) {
	s, p, account, scope := cacheDownloadFixture(t)
	s.mediaCache, s.downloadCache = newMediaResolutionCache(2), newDownloadURLCache(2)
	now := s.now()
	s.now = func() time.Time { return now }
	media := RedirectCandidate{downloadCacheKey: "url", resolvedSHA1: directPlaySourceSHA1, resolvedSize: 1}
	s.mediaCache.put("later", media, now, now, now.Add(time.Minute), "")
	s.mediaCache.put("earlier", media, now, now, now.Add(30*time.Second), "")
	if _, found := s.mediaCache.get("earlier", now); !found {
		t.Fatal("fixture media missing")
	}
	ctx, recorder := withTiming(context.Background())
	s.cachedMediaCandidate(ctx, "missing", now, "")
	s.mediaCache.put("new", media, now, now, now.Add(2*time.Minute), "")
	if _, found := s.mediaCache.get("earlier", now); found {
		t.Fatal("media access changed earliest-expiry eviction")
	}
	get := func(ua string) {
		t.Helper()
		if _, err := s.downloadCandidate(ctx, account, p.targetFile, ua, "", true, scope); err != nil {
			t.Fatal(err)
		}
	}
	get("a")
	get("b")
	get("a")
	get("c")
	if _, found := s.downloadCache.get(scope.downloadKey(account, p.targetFile, "b"), now); found {
		t.Fatal("download hits stopped updating LRU order")
	}
	fields := cacheDiagnosticFields(t, recorder.finish())
	if fields["mediaCacheEvictions"] != "1" || fields["downloadCacheEvictions"] != "1" {
		t.Fatalf("snapshot omitted later capacity pressure: %v", fields)
	}
	// Same-key replacement, expiry deletion and explicit removal must not
	// inflate counters; repeated observations retain the cumulative value.
	s.mediaCache.put("new", media, now, now, now.Add(2*time.Minute), "")
	s.mediaCache.get("later", now.Add(time.Minute))
	s.downloadCache.remove(scope.downloadKey(account, p.targetFile, "c"))
	s.downloadCache.put(scope.downloadKey(account, p.targetFile, "a"), p.download, now)
	for range 2 {
		snapshotCtx, snapshot := withTiming(context.Background())
		s.cachedMediaCandidate(snapshotCtx, "earlier", now, "")
		_, err := s.downloadCandidate(snapshotCtx, account, p.targetFile, "a", "", true, scope)
		fields = cacheDiagnosticFields(t, snapshot.finish())
		if err != nil || fields["mediaResolutionCacheReason"] != "entry_missing" || fields["mediaCacheEvictions"] != "1" || fields["downloadCacheEvictions"] != "1" {
			t.Fatalf("cumulative snapshots err=%v fields=%v", err, fields)
		}
	}
}

// TestCacheDiagnosticsLogFieldsFailClosed rejects arbitrary secrets, unknown
// enums, cross-cache reasons and reasons attached to hits or invalid outcomes.
func TestCacheDiagnosticsLogFieldsFailClosed(t *testing.T) {
	for _, secret := range []string{"fixture-cookie-secret", "https://cdn.115.com/path?token=fixture-secret", "private-path\nforged=true", directPlaySourceSHA1} {
		ctx, recorder := withTiming(context.Background())
		recordMediaCache(ctx, "miss", secret)
		recordDownloadCache(ctx, "bypass", secret)
		recorder.stages[secret] = stageTiming{calls: 1}
		timing := recorder.finish()
		if strings.Contains(strings.Join(timing.LogFields(), " "), secret) {
			t.Fatal("unrecognized diagnostic string entered LogFields")
		}
		for _, outcome := range []string{"hit", secret} {
			timing.mediaCache, timing.downloadCache = outcome, outcome
			timing.mediaCacheReason, timing.downloadCacheReason = "expired", "expired"
			fields := cacheDiagnosticFields(t, timing)
			if _, found := fields["mediaResolutionCacheReason"]; found {
				t.Fatal("hit or unknown outcome retained a media reason")
			}
			if _, found := fields["downloadURLCacheReason"]; found {
				t.Fatal("hit or unknown outcome retained a download reason")
			}
			if strings.Contains(strings.Join(timing.LogFields(), " "), secret) {
				t.Fatal("unknown cache outcome escaped filtering")
			}
		}
		timing.mediaCache, timing.downloadCache = "miss", "miss"
		timing.mediaCacheReason, timing.downloadCacheReason = "forced_refresh", "freshness_expired"
		fields := cacheDiagnosticFields(t, timing)
		if fields["mediaResolutionCacheReason"] != "" || fields["downloadURLCacheReason"] != "" {
			t.Fatal("reason from a different cache escaped filtering")
		}
		if dependentURLCacheReason(secret) != "" {
			t.Fatal("unknown dependency reason escaped filtering")
		}
	}
}

// TestCacheDiagnosticsConcurrentCapacitySnapshots exercises counter reads
// against concurrent pressure using only cache operations and local recorders.
func TestCacheDiagnosticsConcurrentCapacitySnapshots(t *testing.T) {
	s, p, _, _ := cacheDownloadFixture(t)
	s.mediaCache, s.downloadCache = newMediaResolutionCache(1), newDownloadURLCache(1)
	now := s.now()
	media := RedirectCandidate{downloadCacheKey: "url", resolvedSHA1: directPlaySourceSHA1, resolvedSize: 1}
	s.mediaCache.put("initial", media, now, now, now.Add(time.Minute), "")
	s.downloadCache.put("url", p.download, now)
	var workers sync.WaitGroup
	for worker := range 4 {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for iteration := range 25 {
				key := strconv.Itoa(worker) + ":" + strconv.Itoa(iteration)
				s.mediaCache.put(key, media, now, now, now.Add(time.Minute), "")
				s.downloadCache.put(key, p.download, now)
				ctx, recorder := withTiming(context.Background())
				s.cachedMediaCandidate(ctx, key, now, "")
				recorder.finish().LogFields()
			}
		}()
	}
	workers.Wait()
	// Publish one known pair for a deterministic final observation. Both
	// keys are new, so each cache has exactly 101 capacity evictions.
	s.downloadCache.put("final-url", p.download, now)
	media.downloadCacheKey = "final-url"
	s.mediaCache.put("final-media", media, now, now, now.Add(time.Minute), "")
	ctx, recorder := withTiming(context.Background())
	if _, hit := s.cachedMediaCandidate(ctx, "final-media", now, ""); !hit {
		t.Fatal("final cache pair missing")
	}
	fields := cacheDiagnosticFields(t, recorder.finish())
	if fields["mediaCacheEvictions"] != "101" || fields["downloadCacheEvictions"] != "101" {
		t.Fatalf("concurrent capacity snapshots=%v", fields)
	}
}
