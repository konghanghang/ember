package directplay

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	p115 "github.com/konghang/ember/backend/internal/integrations/p115"
)

// TestSourceFailureBackoffSkipsRepeatedTransferWork keeps deterministic source
// rejection from creating another task or pending quota for the same caller.
func TestSourceFailureBackoffSkipsRepeatedTransferWork(t *testing.T) {
	fixture := newPreIDFixture(t)
	fixture.provider.hash = func(context.Context, int, p115.FileRangeRequest) (p115.FileRangeHash, error) {
		return p115.FileRangeHash{}, &p115.DownloadURLPolicyError{
			Reason: p115.DownloadURLPolicyHostNotAllowed, Scheme: "https", Host: "untrusted.example",
		}
	}
	request := preIDRequest("user-1")
	for i := 0; i < 2; i++ {
		candidate, err := fixture.service.ResolveMediaPath(context.Background(), request)
		if !errors.Is(err, ErrProviderProtocol) || candidate.URL != "" ||
			InspectFailure(err).ProviderOperation != failureOperationHashSourcePreID {
			t.Fatalf("attempt %d: candidate=%t err=%v detail=%+v", i, candidate.URL != "", err, InspectFailure(err))
		}
		fixture.assertQuota(t, request.UserID, 0)
	}
	if got := fixture.provider.snapshot(); got.hashes != 1 || got.uploads != 0 || got.verifies != 0 {
		t.Fatalf("repeated failure performed more Provider work: %+v", got)
	}
	fixture.store.mu.Lock()
	defer fixture.store.mu.Unlock()
	if len(fixture.store.tasks) != 1 {
		t.Fatalf("stable repeated failure created %d tasks, want 1", len(fixture.store.tasks))
	}
	for _, task := range fixture.store.tasks {
		if !task.failed || task.succeeded {
			t.Fatal("original failed task was not preserved")
		}
	}
}

// TestSourceFailureBackoffYieldsToSuccessfulPreID lets a newer successful read
// from another eligible caller supersede the earlier caller's local refusal.
func TestSourceFailureBackoffYieldsToSuccessfulPreID(t *testing.T) {
	fixture := newPreIDFixture(t)
	fixture.provider.hash = func(ctx context.Context, call int, request p115.FileRangeRequest) (p115.FileRangeHash, error) {
		if call == 1 {
			return stableSourceHostRefusal(ctx, call, request)
		}
		return preIDHash(request), nil
	}
	if _, err := fixture.service.ResolveMediaPath(context.Background(), preIDRequest("user-1")); !errors.Is(err, ErrProviderProtocol) {
		t.Fatal(err)
	}
	for _, user := range []string{"user-2", "user-1"} {
		if candidate, err := fixture.service.ResolveMediaPath(context.Background(), preIDRequest(user)); err != nil || candidate.URL == "" {
			t.Fatalf("new successful source result was hidden for %s: %v", user, err)
		}
		fixture.assertQuota(t, user, 1)
	}
	if got := fixture.provider.snapshot().hashes; got != 2 {
		t.Fatalf("successful preID was not reused: reads=%d", got)
	}
}

// TestSourceFailureBackoffExpiryAndCallerIsolation checks that hits neither
// extend the fixed window nor leak one caller's refusal into another identity.
func TestSourceFailureBackoffExpiryAndCallerIsolation(t *testing.T) {
	fixture := newPreIDFixture(t)
	fixture.provider.hash = stableSourceHostRefusal
	now := fixture.service.now()
	fixture.service.now = func() time.Time { return now }
	request := preIDRequest("user-1")
	for _, offset := range []time.Duration{0, 9 * time.Second, 10 * time.Second} {
		now = fixture.provider.expiresAt.Add(-time.Hour).Add(offset)
		if _, err := fixture.service.ResolveMediaPath(context.Background(), request); !errors.Is(err, ErrProviderProtocol) {
			t.Fatal(err)
		}
	}
	if got := fixture.provider.snapshot().hashes; got != 2 {
		t.Fatalf("fixed window performed %d reads, want 2", got)
	}
	if _, err := fixture.service.ResolveMediaPath(context.Background(), preIDRequest("user-2")); !errors.Is(err, ErrProviderProtocol) {
		t.Fatal(err)
	}
	if got := fixture.provider.snapshot().hashes; got != 3 {
		t.Fatalf("another caller inherited a refusal: reads=%d", got)
	}
	fixture.assertQuota(t, "user-1", 0)
	fixture.assertQuota(t, "user-2", 0)
}

// TestSourceFailureBackoffConcurrentRequests shares no failure task between
// overlapping sessions, while allowing only the first one to repeat the read.
func TestSourceFailureBackoffConcurrentRequests(t *testing.T) {
	fixture := newPreIDFixture(t)
	entered, release, waiting := make(chan struct{}), make(chan struct{}), make(chan struct{})
	fixture.provider.hash = func(ctx context.Context, call int, request p115.FileRangeRequest) (p115.FileRangeHash, error) {
		if call == 1 {
			close(entered)
			select {
			case <-release:
			case <-ctx.Done():
				return p115.FileRangeHash{}, ctx.Err()
			}
		}
		return stableSourceHostRefusal(ctx, call, request)
	}
	first := fixture.resolveAsync(context.Background(), preIDRequest("user-1"))
	awaitPreIDSignal(t, entered)
	request := preIDRequest("user-1")
	request.PlaySessionID += "-next"
	ctx := WithStepObserver(context.Background(), func(step, phase string, _ int64) {
		if step == "mediaWait" && phase == "started" {
			close(waiting)
		}
	})
	second := fixture.resolveAsync(ctx, request)
	awaitPreIDSignal(t, waiting)
	close(release)
	for _, done := range []<-chan preIDResolveResult{first, second} {
		if result := awaitPreIDResult(t, done); !errors.Is(result.err, ErrProviderProtocol) {
			t.Fatal(result.err)
		}
	}
	if got := fixture.provider.snapshot(); got.hashes != 1 || got.uploads != 0 {
		t.Fatalf("waiting request repeated stable failure: %+v", got)
	}
	fixture.store.mu.Lock()
	defer fixture.store.mu.Unlock()
	if len(fixture.store.tasks) != 1 {
		t.Fatalf("waiting request created another failure task: %d", len(fixture.store.tasks))
	}
}

type sourceFailureTargetProvider struct {
	*preIDProvider
	found atomic.Bool
}

// SearchBySHA1 can expose a valid target after a source refusal; cached failure
// must never override the real target lookup that precedes new transfer work.
func (provider *sourceFailureTargetProvider) SearchBySHA1(ctx context.Context, credential p115.Credential, query p115.FileQuery) ([]p115.File, error) {
	if !provider.found.Load() {
		return provider.preIDProvider.SearchBySHA1(ctx, credential, query)
	}
	return []p115.File{{ID: "existing-file", PickCode: "existing-pick", ParentID: query.ParentID,
		SHA1: query.SHA1, Size: query.Size}}, nil
}

// TestSourceFailureBackoffDoesNotBlockReuseOrAdmission ensures later target
// availability, strict intent and HEAD preserve their original priority.
func TestSourceFailureBackoffDoesNotBlockReuseOrAdmission(t *testing.T) {
	for _, mode := range []string{"target appears", "missing intent", "HEAD"} {
		t.Run(mode, func(t *testing.T) {
			fixture := newPreIDFixture(t)
			fixture.provider.hash = stableSourceHostRefusal
			provider := &sourceFailureTargetProvider{preIDProvider: fixture.provider}
			fixture.service.provider = provider
			request := preIDRequest("user-1")
			if _, err := fixture.service.ResolveMediaPath(context.Background(), request); !errors.Is(err, ErrProviderProtocol) {
				t.Fatal(err)
			}
			switch mode {
			case "target appears":
				provider.found.Store(true)
			case "missing intent":
				request.AuthorizeTransfer = nil
			case "HEAD":
				fixture.reserveLease(t, request)
				request.Method = "HEAD"
			}
			candidate, err := fixture.service.ResolveMediaPath(context.Background(), request)
			if mode == "target appears" {
				if err != nil || candidate.URL == "" || !candidate.Preexisting {
					t.Fatalf("target reuse blocked: candidate=%t reused=%t err=%v", candidate.URL != "", candidate.Preexisting, err)
				}
			} else if !errors.Is(err, ErrPlaybackIntentRequired) {
				t.Fatalf("backoff bypassed admission: %v", err)
			}
			if got := fixture.provider.snapshot(); got.hashes != 1 || got.uploads != 0 {
				t.Fatalf("admission/reuse repeated the source read: %+v", got)
			}
			fixture.assertQuota(t, "user-1", 0)
		})
	}
}

type sourceFailurePersistStore struct{ *preIDTaskStore }

// MarkFailed simulates an unavailable terminal write without changing the
// source refusal, so storage failure cannot accidentally establish a backoff.
func (*sourceFailurePersistStore) MarkFailed(context.Context, string, string, string, time.Time) error {
	return errors.New("fixture terminal storage unavailable")
}

// TestSourceFailureBackoffRejectsCanceledAndUnpersistedFailures leaves a real
// retry available after a caller disconnect or failed terminal storage write.
func TestSourceFailureBackoffRejectsCanceledAndUnpersistedFailures(t *testing.T) {
	for _, mode := range []string{"cancel", "storage"} {
		t.Run(mode, func(t *testing.T) {
			fixture := newPreIDFixture(t)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			fixture.provider.hash = func(ctx context.Context, call int, request p115.FileRangeRequest) (p115.FileRangeHash, error) {
				if mode == "cancel" && call == 1 {
					cancel()
				}
				return stableSourceHostRefusal(ctx, call, request)
			}
			want := context.Canceled
			if mode == "storage" {
				fixture.service.store = &sourceFailurePersistStore{preIDTaskStore: fixture.store}
				want = ErrStoreUnavailable
			}
			request := preIDRequest("user-1")
			if _, err := fixture.service.ResolveMediaPath(ctx, request); !errors.Is(err, want) {
				t.Fatalf("first error=%v, want %v", err, want)
			}
			fixture.service.store = fixture.store
			if _, err := fixture.service.ResolveMediaPath(context.Background(), request); !errors.Is(err, ErrProviderProtocol) {
				t.Fatal(err)
			}
			if got := fixture.provider.snapshot().hashes; got != 2 {
				t.Fatalf("%s suppressed retry: reads=%d", mode, got)
			}
			fixture.assertQuota(t, "user-1", 0)
		})
	}
}

// stableSourceHostRefusal produces a typed deterministic failure with a host
// that must never be retained in the process-local failure cache or its logs.
func stableSourceHostRefusal(context.Context, int, p115.FileRangeRequest) (p115.FileRangeHash, error) {
	return p115.FileRangeHash{}, &p115.DownloadURLPolicyError{
		Reason: p115.DownloadURLPolicyHostNotAllowed, Scheme: "https", Host: "untrusted.example",
	}
}

// TestSourceFailureBackoffClassifiesAndBounds verifies the narrow admission
// list, immutable deadlines, no retained host and bounded LRU independently.
func TestSourceFailureBackoffClassifiesAndBounds(t *testing.T) {
	_, stable := stableSourceHostRefusal(context.Background(), 0, p115.FileRangeRequest{})
	now := time.Date(2026, 11, 3, 12, 0, 0, 0, time.FixedZone("fixture", 8*60*60))
	for _, err := range []error{nil, p115.ErrProviderProtocol, p115.ErrDownloadURLNotAllowed, p115.ErrProviderUnavailable,
		p115.ErrCredentialRejected, p115.ErrProviderRateLimited, p115.ErrDataPlaneQueueLimited, p115.ErrSourceRangeTimeout,
		context.Canceled, context.DeadlineExceeded, errors.Join(stable, context.Canceled),
		&p115.DownloadURLPolicyError{Reason: p115.DownloadURLPolicySchemeNotHTTPS}} {
		cache := newSourceFailureBackoff(2)
		if cache.put(context.Background(), "key", err, now) {
			t.Fatalf("nonstable error cached: %T", err)
		}
	}
	cache := newSourceFailureBackoff(2)
	if !cache.put(context.Background(), "first", fmt.Errorf("fixture wrapper: %w", stable), now) {
		t.Fatal("typed stable refusal not cached")
	}
	entry, ok := cache.get("first", now.Add(9*time.Second))
	if !ok || strings.Contains(fmt.Sprintf("%+v", entry), "untrusted.example") {
		t.Fatal("safe entry missing or retained host")
	}
	if cache.put(context.Background(), "first", stable, now.Add(9*time.Second)) {
		t.Fatal("duplicate refusal extended a live window")
	}
	if _, found := cache.get("first", now.Add(sourceFailureBackoffTTL)); found {
		t.Fatal("fixed refusal lifetime was extended")
	}
	cache.put(context.Background(), "first", stable, now)
	cache.put(context.Background(), "second", stable, now)
	cache.get("first", now)
	cache.put(context.Background(), "third", stable, now)
	if _, found := cache.get("second", now); found || len(cache.entries) != 2 {
		t.Fatal("LRU capacity was not enforced")
	}
	if _, found := cache.get("first", now.Add(-time.Nanosecond)); found {
		t.Fatal("clock rollback reused a refusal")
	}
}

// TestSourceFailureBackoffKeyIsolation rejects unknown/recovery generations and
// separates the caller, source snapshot and both account configuration versions.
func TestSourceFailureBackoffKeyIsolation(t *testing.T) {
	fixture := newPreIDFixture(t)
	source := fixture.accounts.source
	route, _ := fixture.accounts.ResolvePlaybackRoute(context.Background(), "user-1", fixture.service.now())
	playback, _ := fixture.accounts.AcquirePlaybackRoute(context.Background(), route)
	request := ResolveRequest{SourceFile: p115.FilePathQuery{RootID: "100", RelativePath: "fixture.mkv"},
		ClientUserAgent: "fixture-client", downloadScope: &downloadCacheScope{
			serverID: "server", userID: "user", mappingID: "mapping", deviceID: "device"}}
	file := fixture.provider.sourceFile
	key := sourceFailureBackoffKey(source, playback, file, request)
	if len(key) != 64 {
		t.Fatal("complete identity did not produce an opaque digest")
	}
	for _, name := range []string{"source version", "source cookie", "playback version", "playback cookie", "caller", "mapping", "device", "UA", "file", "path"} {
		t.Run(name, func(t *testing.T) {
			s, p, f, r := source, playback, file, request
			scope := *r.downloadScope
			r.downloadScope = &scope
			switch name {
			case "source version":
				s.DownloadCacheVersion++
			case "source cookie":
				s.Credential.Cookie += "changed"
			case "playback version":
				p.DownloadCacheVersion++
			case "playback cookie":
				p.Credential.Cookie += "changed"
			case "caller":
				scope.userID += "changed"
			case "mapping":
				scope.mappingID += "changed"
			case "device":
				scope.deviceID += "changed"
			case "UA":
				r.ClientUserAgent += "changed"
			case "file":
				f.PickCode += "changed"
			case "path":
				r.SourceFile.RelativePath += "changed"
			}
			if other := sourceFailureBackoffKey(s, p, f, r); other == "" || other == key {
				t.Fatal("changed identity reused failure")
			}
		})
	}
	for _, mode := range []string{"source probe", "playback probe", "scope missing", "file invalid"} {
		s, p, f, r := source, playback, file, request
		switch mode {
		case "source probe":
			s.DownloadCacheVersion = 0
		case "playback probe":
			p.DownloadCacheVersion = 0
		case "scope missing":
			r.downloadScope = nil
		case "file invalid":
			f.IsDirectory = true
		}
		if sourceFailureBackoffKey(s, p, f, r) != "" {
			t.Fatalf("%s reused a failure", mode)
		}
	}
}
