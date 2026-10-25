package directplay

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	p115 "github.com/konghang/ember/backend/internal/integrations/p115"
	"github.com/konghang/ember/backend/internal/models"
	"github.com/konghang/ember/backend/internal/services/p115account"
	"github.com/konghang/ember/backend/internal/services/p115quota"
)

// TestPreIDWarmCacheCannotBypassAdmission keeps all transfer side effects behind
// intent, HEAD, quota and durable task initialization even when the hash is warm.
func TestPreIDWarmCacheCannotBypassAdmission(t *testing.T) {
	for _, stage := range []string{"missing intent", "denied intent", "HEAD", "quota", "task begin", "task status"} {
		t.Run(stage, func(t *testing.T) {
			fixture := newPreIDFixture(t)
			if _, err := fixture.service.ResolveMediaPath(context.Background(), preIDRequest("user-1")); err != nil {
				t.Fatal(err)
			}
			request := preIDRequest("user-2")
			wantErr := ErrPlaybackIntentRequired
			switch stage {
			case "missing intent":
				request.CanCreateTransfer = nil
			case "denied intent":
				request.CanCreateTransfer = func() bool { return false }
			case "HEAD":
				fixture.reserveLease(t, request)
				request.Method = "HEAD"
			case "quota":
				fixture.service.transferQuotas = preIDDeniedQuota{TransferQuotaStore: fixture.quotas}
				wantErr = ErrTransferQuotaExceeded
			case "task begin":
				fixture.store.beginErr = errors.New("fixture store unavailable")
				wantErr = ErrStoreUnavailable
			case "task status":
				fixture.store.statusErr = errors.New("fixture status unavailable")
				wantErr = ErrStoreUnavailable
			}
			candidate, err := fixture.service.ResolveMediaPath(context.Background(), request)
			if !errors.Is(err, wantErr) || candidate.URL != "" {
				t.Fatalf("blocked request returned candidate=%t err=%v", candidate.URL != "", err)
			}
			counts := fixture.provider.snapshot()
			if counts.hashes != 1 || counts.uploads != 1 || counts.verifies != 1 || counts.searches != 4 {
				t.Fatalf("warm hash bypassed transfer admission: %+v", counts)
			}
			if strings.Contains(strings.Join(candidate.Timing.LogFields(), " "), "preID") {
				t.Fatal("blocked request consulted the preID cache")
			}
			fixture.assertQuota(t, "user-2", 0)
		})
	}
}

type preIDDeniedQuota struct{ p115quota.TransferQuotaStore }

// ReserveTransfer rejects admission before any cached or real source read.
func (preIDDeniedQuota) ReserveTransfer(context.Context, p115quota.TransferReserveRequest, time.Time) (p115quota.TransferReserveResult, error) {
	return p115quota.TransferReserveResult{}, p115quota.ErrTransferQuotaExceeded
}

// reserveLease gives HEAD a real existing reservation, so it reaches the transfer
// intent check rather than passing only the earlier missing-lease error path.
func (fixture *preIDFixture) reserveLease(t *testing.T, request MediaPathResolveRequest) {
	t.Helper()
	route, _ := fixture.accounts.ResolvePlaybackRoute(context.Background(), request.UserID, fixture.service.now())
	accountKey, _ := fixture.service.keyDeriver.PlaybackAccountKey(route.ProviderUserID)
	fingerprint, _ := fixture.service.keyDeriver.SessionFingerprint(p115quota.SessionIdentity{ServerID: fixture.service.serverID,
		UserID: request.UserID, MappingID: request.MappingID, DeviceID: request.DeviceID, PlaySessionID: request.PlaySessionID})
	if _, err := fixture.quotas.Reserve(context.Background(), p115quota.ReserveRequest{PlaybackAccountKey: accountKey,
		UserID: request.UserID, SessionFingerprint: fingerprint, MaxConcurrentStreams: 2}, fixture.service.now()); err != nil {
		t.Fatal(err)
	}
}

// assertQuota checks released pending reservations as well as completed work.
func (fixture *preIDFixture) assertQuota(t *testing.T, user string, used int) {
	t.Helper()
	start, end := p115quota.DayWindow(fixture.service.now(), fixture.service.businessTimezone)
	usage, err := fixture.quotas.TransferUsage(context.Background(), p115quota.TransferUsageRequest{UserID: user, DayStart: start, DayEnd: end}, fixture.service.now())
	if err != nil || usage.Pending != 0 || usage.HourlyUsed != used || usage.DailyUsed != used {
		t.Fatalf("quota %s=%+v expected used=%d err=%v", user, usage, used, err)
	}
}

// TestPreIDShortFilesAndDiagnostics ensures range size and canonical SHA1 survive
// reuse, while the hit caller receives no owner read timing or Provider call.
func TestPreIDShortFilesAndDiagnostics(t *testing.T) {
	for _, size := range []int64{1, 17, directPlayPreIDBytes - 1, directPlayPreIDBytes, directPlayPreIDBytes + 1} {
		t.Run(fmt.Sprint(size), func(t *testing.T) {
			fixture := newPreIDFixture(t)
			fixture.provider.sourceFile.Size = size
			fixture.provider.hash = func(_ context.Context, _ int, request p115.FileRangeRequest) (p115.FileRangeHash, error) {
				if request.Range != boundedRange(size) {
					return p115.FileRangeHash{}, errors.New("unexpected prefix range")
				}
				result := preIDHash(request)
				result.SHA1 = strings.ToLower(result.SHA1)
				return result, nil
			}
			fixture.provider.upload = func(_ context.Context, _ p115.Credential, request p115.RapidUploadRequest) (p115.RapidUploadResult, error) {
				if request.PreID != directPlayPreID {
					return p115.RapidUploadResult{}, errors.New("noncanonical preID")
				}
				return p115.RapidUploadResult{Status: p115.RapidUploadReused}, nil
			}
			first, err := fixture.service.ResolveMediaPath(context.Background(), preIDRequest("user-1"))
			if err != nil {
				t.Fatal(err)
			}
			var events []string
			ctx := WithStepObserver(context.Background(), func(step, phase string, _ int64) { events = append(events, step+":"+phase) })
			second, err := fixture.service.ResolveMediaPath(ctx, preIDRequest("user-2"))
			if err != nil {
				t.Fatal(err)
			}
			fixture.assertEqualWork(t, [2]preIDResolveResult{{candidate: first}, {candidate: second}})
			fields := strings.Join(second.Timing.LogFields(), " ")
			if fixture.provider.snapshot().hashes != 1 || !strings.Contains(fields, "preIDCache=hit") || strings.Contains(fields, "preIDCalls=") || strings.Contains(fields, "preIDMs=") {
				t.Fatalf("cached prefix reported Provider work: %s", fields)
			}
			joined := strings.Join(events, " ")
			if !strings.Contains(joined, "preIDCache:hit") || strings.Contains(joined, "preID:") || strings.Contains(joined, "preIDWait:") {
				t.Fatalf("warm hit copied a Provider/wait event: %s", joined)
			}
			fixture.accounts.mu.Lock()
			defer fixture.accounts.mu.Unlock()
			if len(fixture.accounts.health) != 4 {
				t.Fatalf("source path and playback health observations=%d, want exactly 4", len(fixture.accounts.health))
			}
		})
	}
}

// TestPreIDFailureNeverPopulates verifies task failure and quota release before
// another personal target retries the same source with a new real read.
func TestPreIDFailureNeverPopulates(t *testing.T) {
	for _, failure := range []string{"provider", "hash", "short bytes", "long bytes", "cancel"} {
		t.Run(failure, func(t *testing.T) {
			fixture := newPreIDFixture(t)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			fixture.provider.hash = func(_ context.Context, call int, request p115.FileRangeRequest) (p115.FileRangeHash, error) {
				result := preIDHash(request)
				if call > 1 {
					return result, nil
				}
				switch failure {
				case "provider":
					return p115.FileRangeHash{}, p115.ErrProviderUnavailable
				case "hash":
					result.SHA1 = "invalid"
				case "short bytes":
					result.BytesRead--
				case "long bytes":
					result.BytesRead++
				case "cancel":
					cancel()
				}
				return result, nil
			}
			if _, err := fixture.service.ResolveMediaPath(ctx, preIDRequest("user-1")); err == nil {
				t.Fatal("invalid prefix did not fail its task")
			}
			if len(fixture.service.preIDCache.entries) != 0 || fixture.provider.snapshot().uploads != 0 || !fixture.store.tasks["preid-task-1"].failed {
				t.Fatal("failed read was published or continued uploading")
			}
			fixture.assertQuota(t, "user-1", 0)
			if _, err := fixture.service.ResolveMediaPath(context.Background(), preIDRequest("user-2")); err != nil {
				t.Fatal(err)
			}
			if fixture.provider.snapshot().hashes != 2 || len(fixture.service.preIDCache.entries) != 1 {
				t.Fatal("failed read was reused instead of retrying")
			}
			fixture.assertQuota(t, "user-2", 1)
		})
	}
}

// TestPreIDOwnerCancellationLetsWaiterRetry proves there is no detached shared
// owner: its context cancels the read and the waiting request becomes a new owner.
func TestPreIDOwnerCancellationLetsWaiterRetry(t *testing.T) {
	fixture := newPreIDFixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	entered := make(chan struct{})
	fixture.provider.hash = func(ctx context.Context, call int, request p115.FileRangeRequest) (p115.FileRangeHash, error) {
		if call == 1 {
			close(entered)
			<-ctx.Done()
			// Even a Provider that returns a valid result after cancellation must
			// not publish it or share its completion with the waiting caller.
		}
		return preIDHash(request), nil
	}
	owner := fixture.resolveAsync(ctx, preIDRequest("user-1"))
	awaitPreIDSignal(t, entered)
	waiter := fixture.resolveAsync(context.Background(), preIDRequest("user-2"))
	awaitPreIDGateRefs(t, fixture.service.preIDCache, 2)
	cancel()
	if result := awaitPreIDResult(t, owner); !errors.Is(result.err, context.Canceled) {
		t.Fatalf("owner error=%v", result.err)
	}
	result := awaitPreIDResult(t, waiter)
	if result.err != nil || fixture.provider.snapshot().hashes != 2 {
		t.Fatalf("waiter failed to retry: error=%v calls=%d", result.err, fixture.provider.snapshot().hashes)
	}
	fields := strings.Join(result.candidate.Timing.LogFields(), " ")
	if !strings.Contains(fields, "preIDWaitCalls=1") || !strings.Contains(fields, "preIDCalls=1") || !strings.Contains(fields, "preIDCache=miss") {
		t.Fatalf("waiter retry diagnostics=%s", fields)
	}
	fixture.assertQuota(t, "user-1", 0)
	fixture.assertQuota(t, "user-2", 1)
	awaitPreIDGateRefs(t, fixture.service.preIDCache, 0)
}

// TestPreIDWaiterCancelDoesNotCancelOwner preserves independent request lifetime,
// and a third caller can reuse the owner's success after the canceled waiter exits.
func TestPreIDWaiterCancelDoesNotCancelOwner(t *testing.T) {
	fixture := newPreIDFixture(t)
	entered, release := make(chan struct{}), make(chan struct{})
	var releaseOnce sync.Once
	defer releaseOnce.Do(func() { close(release) })
	fixture.provider.hash = func(ctx context.Context, call int, request p115.FileRangeRequest) (p115.FileRangeHash, error) {
		if call == 1 {
			close(entered)
			select {
			case <-release:
			case <-ctx.Done():
				return p115.FileRangeHash{}, ctx.Err()
			}
		}
		return preIDHash(request), nil
	}
	owner := fixture.resolveAsync(context.Background(), preIDRequest("user-1"))
	awaitPreIDSignal(t, entered)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	waiter := fixture.resolveAsync(ctx, preIDRequest("user-2"))
	awaitPreIDGateRefs(t, fixture.service.preIDCache, 2)
	cancel()
	if result := awaitPreIDResult(t, waiter); !errors.Is(result.err, context.Canceled) {
		t.Fatalf("waiter error=%v", result.err)
	}
	if fixture.provider.snapshot().hashes != 1 {
		t.Fatal("canceled waiter performed its own read")
	}
	awaitPreIDGateRefs(t, fixture.service.preIDCache, 1)
	releaseOnce.Do(func() { close(release) })
	if result := awaitPreIDResult(t, owner); result.err != nil {
		t.Fatal(result.err)
	}
	third, err := fixture.service.ResolveMediaPath(context.Background(), preIDRequest("user-3"))
	if err != nil || fixture.provider.snapshot().hashes != 1 || !strings.Contains(strings.Join(third.Timing.LogFields(), " "), "preIDCache=hit") {
		t.Fatalf("owner success lost after waiter cancellation: %v", err)
	}
	fixture.assertQuota(t, "user-1", 1)
	fixture.assertQuota(t, "user-2", 0)
	fixture.assertQuota(t, "user-3", 1)
	awaitPreIDGateRefs(t, fixture.service.preIDCache, 0)
}

// awaitPreIDGateRefs observes actual gate membership, avoiding sleep-based races
// where cancellation would accidentally happen before the waiter joined.
func awaitPreIDGateRefs(t *testing.T, cache *preIDCache, want int) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for {
		cache.flights.mu.Lock()
		refs := 0
		for _, entry := range cache.flights.entries {
			refs += entry.refs
		}
		cache.flights.mu.Unlock()
		if refs == want {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("preID gate refs=%d, want %d", refs, want)
		}
		runtime.Gosched()
	}
}

// TestPreIDRecoveryBypassesWarmCacheAndFlights keeps version-zero source recovery
// on real independent reads even when an earlier healthy generation is cached.
func TestPreIDRecoveryBypassesWarmCacheAndFlights(t *testing.T) {
	fixture := newPreIDFixture(t)
	if _, err := fixture.service.ResolveMediaPath(context.Background(), preIDRequest("user-1")); err != nil {
		t.Fatal(err)
	}
	fixture.accounts.source.DownloadCacheVersion = 0
	entered, release := make(chan struct{}, 2), make(chan struct{})
	var releaseOnce sync.Once
	defer releaseOnce.Do(func() { close(release) })
	fixture.provider.hash = func(ctx context.Context, _ int, request p115.FileRangeRequest) (p115.FileRangeHash, error) {
		entered <- struct{}{}
		select {
		case <-release:
		case <-ctx.Done():
			return p115.FileRangeHash{}, ctx.Err()
		}
		return preIDHash(request), nil
	}
	first := fixture.resolveAsync(context.Background(), preIDRequest("user-2"))
	second := fixture.resolveAsync(context.Background(), preIDRequest("user-3"))
	awaitPreIDSignal(t, entered)
	awaitPreIDSignal(t, entered)
	awaitPreIDGateRefs(t, fixture.service.preIDCache, 0)
	releaseOnce.Do(func() { close(release) })
	for _, done := range []<-chan preIDResolveResult{first, second} {
		result := awaitPreIDResult(t, done)
		fields := strings.Join(result.candidate.Timing.LogFields(), " ")
		if result.err != nil || !strings.Contains(fields, "preIDCache=bypass") || !strings.Contains(fields, "preIDCalls=1") || strings.Contains(fields, "preIDWait") {
			t.Fatalf("recovery error=%v fields=%s", result.err, fields)
		}
	}
	if fixture.provider.snapshot().hashes != 3 || len(fixture.service.preIDCache.entries) != 1 {
		t.Fatal("recovery read was reused, merged or published")
	}
	fixture.accounts.mu.Lock()
	defer fixture.accounts.mu.Unlock()
	sourceSuccess := 0
	for _, event := range fixture.accounts.health {
		if event.role == models.P115AccountRoleSource && event.outcome == p115account.RuntimeHealthSucceeded {
			sourceSuccess++
		}
	}
	if sourceSuccess != 3 {
		t.Fatalf("real source path recovery successes=%d, want 3", sourceSuccess)
	}
}

// TestPreIDChallengeAlwaysReadsSource preserves real challenge IO even when its
// requested range exactly equals a previously validated preID prefix.
func TestPreIDChallengeAlwaysReadsSource(t *testing.T) {
	fixture := newPreIDFixture(t)
	fixture.provider.upload = func(_ context.Context, _ p115.Credential, request p115.RapidUploadRequest) (p115.RapidUploadResult, error) {
		if request.SignKey == "" {
			return p115.RapidUploadResult{Status: p115.RapidUploadRangeChallenge,
				Challenge: &p115.RapidUploadChallenge{SignKey: "fixture-sign", Range: boundedRange(request.Size)}}, nil
		}
		return p115.RapidUploadResult{Status: p115.RapidUploadReused}, nil
	}
	for _, user := range []string{"user-1", "user-2"} {
		candidate, err := fixture.service.ResolveMediaPath(context.Background(), preIDRequest(user))
		if err != nil || !strings.Contains(strings.Join(candidate.Timing.LogFields(), " "), "challengeCalls=1") {
			t.Fatalf("challenge %s failed: %v", user, err)
		}
		fixture.assertQuota(t, user, 1)
	}
	counts := fixture.provider.snapshot()
	if counts.hashes != 3 || counts.uploads != 4 || counts.verifies != 2 || len(fixture.store.tasks) != 2 {
		t.Fatalf("challenge work was reused: %+v", counts)
	}
}

// TestPreIDPublishesBeforeTargetCompletion retains a proven source hash when an
// unrelated target later fails, without making another target wait for that write.
func TestPreIDPublishesBeforeTargetCompletion(t *testing.T) {
	fixture := newPreIDFixture(t)
	entered, release := make(chan struct{}), make(chan struct{})
	var releaseOnce sync.Once
	defer releaseOnce.Do(func() { close(release) })
	fixture.provider.upload = func(ctx context.Context, credential p115.Credential, _ p115.RapidUploadRequest) (p115.RapidUploadResult, error) {
		if credential.AccountID == "personal-user-1" {
			close(entered)
			select {
			case <-release:
			case <-ctx.Done():
				return p115.RapidUploadResult{}, ctx.Err()
			}
			return p115.RapidUploadResult{}, p115.ErrProviderUnavailable
		}
		return p115.RapidUploadResult{Status: p115.RapidUploadReused}, nil
	}
	owner := fixture.resolveAsync(context.Background(), preIDRequest("user-1"))
	awaitPreIDSignal(t, entered)
	second := awaitPreIDResult(t, fixture.resolveAsync(context.Background(), preIDRequest("user-2")))
	if second.err != nil || fixture.provider.snapshot().hashes != 1 {
		t.Fatalf("second target waited for upload or repeated read: %v", second.err)
	}
	releaseOnce.Do(func() { close(release) })
	if result := awaitPreIDResult(t, owner); !errors.Is(result.err, ErrProviderUnavailable) {
		t.Fatalf("owner upload error=%v", result.err)
	}
	if _, err := fixture.service.ResolveMediaPath(context.Background(), preIDRequest("user-3")); err != nil || fixture.provider.snapshot().hashes != 1 {
		t.Fatalf("target failure invalidated successful source read: %v", err)
	}
	fixture.assertQuota(t, "user-1", 0)
	fixture.assertQuota(t, "user-2", 1)
	fixture.assertQuota(t, "user-3", 1)
}

// TestPreIDDiagnosticsRejectUnknownValues protects the fixed observer/log surface
// from accidentally accepting a cache key, hash or raw Provider detail as outcome.
func TestPreIDDiagnosticsRejectUnknownValues(t *testing.T) {
	var events []string
	ctx := WithStepObserver(context.Background(), func(step, phase string, _ int64) { events = append(events, step+":"+phase) })
	ctx, timing := withTiming(ctx)
	for _, outcome := range []string{"hit", "miss", "bypass"} {
		recordPreIDCache(ctx, outcome)
	}
	recordPreIDCache(ctx, "fixture-secret\nforged=true")
	if len(events) != 3 || strings.Contains(strings.Join(timing.finish().LogFields(), " "), "fixture-secret") {
		t.Fatal("unknown preID diagnostic value escaped the allowlist")
	}
}

// TestPreIDSourceChangesRequireFreshRead exercises source snapshot changes through
// the public media entrypoint, not only through the key builder in isolation.
func TestPreIDSourceChangesRequireFreshRead(t *testing.T) {
	for _, change := range []string{"account", "UID", "version", "cookie", "app", "source UA", "file ID", "pick code", "full hash", "size"} {
		t.Run(change, func(t *testing.T) {
			fixture := newPreIDFixture(t)
			if _, err := fixture.service.ResolveMediaPath(context.Background(), preIDRequest("user-1")); err != nil {
				t.Fatal(err)
			}
			// The first request has joined its heartbeat before returning; all
			// snapshots below change between requests, never during fake reads.
			switch change {
			case "account":
				fixture.accounts.source.Credential.AccountID += "2"
			case "UID":
				fixture.accounts.source.ProviderUserID += "2"
			case "version":
				fixture.accounts.source.DownloadCacheVersion++
			case "cookie":
				fixture.accounts.source.Credential.Cookie += " "
			case "app":
				fixture.accounts.source.Credential.AppType = "web"
			case "source UA":
				fixture.accounts.source.Credential.UserAgent += "2"
			case "file ID":
				fixture.provider.sourceFile.ID += "2"
			case "pick code":
				fixture.provider.sourceFile.PickCode += "2"
			case "full hash":
				fixture.provider.sourceFile.SHA1 = strings.Repeat("B", 40)
			case "size":
				fixture.provider.sourceFile.Size++
			}
			candidate, err := fixture.service.ResolveMediaPath(context.Background(), preIDRequest("user-2"))
			if err != nil || fixture.provider.snapshot().hashes != 2 || candidate.Timing.preIDCache != "miss" {
				t.Fatalf("changed source reused prior prefix: reads=%d outcome=%s err=%v", fixture.provider.snapshot().hashes, candidate.Timing.preIDCache, err)
			}
			fixture.assertQuota(t, "user-1", 1)
			fixture.assertQuota(t, "user-2", 1)
		})
	}
}
