package directplay

import (
	"context"
	"errors"
	"net/http"
	"sync/atomic"
	"testing"
	"time"

	p115 "github.com/konghang/ember/backend/internal/integrations/p115"
	"github.com/konghang/ember/backend/internal/services/p115quota"
)

// TestPlaybackIntentDenialHasNoTransferSideEffects protects the boundary before
// transfer quota, task persistence, preID reads and retained Provider writes.
func TestPlaybackIntentDenialHasNoTransferSideEffects(t *testing.T) {
	for _, absent := range []bool{true, false} {
		name := "denied"
		if absent {
			name = "missing"
		}
		t.Run(name, func(t *testing.T) {
			health := &fakeAccountHealthReporter{}
			accounts := &fakeRoutedAccountRuntime{fakeAccountLoader: fakeAccountLoader{health: health}}
			provider := newFakeProvider()
			leases := p115quota.NewMemoryLeaseStore()
			service := newRoutedDirectPlayForTest(t, accounts, provider, leases)
			request := routedMediaPathRequest(http.MethodGet, "probe")
			checks := 0
			request.CanCreateTransfer = func() bool { checks++; return false }
			if absent {
				request.CanCreateTransfer = nil
			}

			candidate, err := service.ResolveMediaPath(context.Background(), request)
			if !errors.Is(err, ErrPlaybackIntentRequired) || candidate.URL != "" {
				t.Fatalf("probe candidate=%t error=%v", candidate.URL != "", err)
			}
			if candidate.Routing.TransferChecked || len(health.events) != 0 {
				t.Fatalf("probe checked quota=%t health events=%d", candidate.Routing.TransferChecked, len(health.events))
			}
			if !absent && checks != 1 {
				t.Fatalf("intent checks=%d, want one current check", checks)
			}
			assertNoIntentTransfer(t, service, provider)
			accountKey, _ := service.keyDeriver.PlaybackAccountKey("100")
			usage, err := leases.AccountUsage(context.Background(), accountKey, service.now())
			if err != nil || usage.OccupiedStreams != 0 {
				t.Fatalf("denial retained new account reservation: %+v, %v", usage, err)
			}
			userUsage, err := leases.UserUsage(context.Background(), request.UserID, service.now())
			if err != nil || userUsage.OccupiedStreams != 0 {
				t.Fatalf("denial retained new user reservation: %+v, %v", userUsage, err)
			}
		})
	}
}

// TestPlaybackIntentDirectEntryDefaultsToDeny prevents callers of the mapped
// source-path entry from bypassing the same new-transfer permission.
func TestPlaybackIntentDirectEntryDefaultsToDeny(t *testing.T) {
	for _, mapped := range []bool{true, false} {
		t.Run(map[bool]string{true: "source path", false: "media path"}[mapped], func(t *testing.T) {
			provider := newFakeProvider()
			store := &fakeTaskStore{}
			service := newServiceWithDependencies(fakeAccountLoader{}, provider, store, &fakeTaskLocker{})
			resolve := func(intent func() bool) (RedirectCandidate, error) {
				if mapped {
					request := fixtureResolveRequest()
					request.CanCreateTransfer = intent
					return service.Resolve(context.Background(), request)
				}
				request := fixtureMediaPathResolveRequest()
				request.CanCreateTransfer = intent
				return service.ResolveMediaPath(context.Background(), request)
			}
			if _, err := resolve(nil); !errors.Is(err, ErrPlaybackIntentRequired) {
				t.Fatalf("Resolve without intent error=%v", err)
			}
			if store.beginCount != 0 || len(provider.rangeRequests) != 0 || len(provider.uploadRequests) != 0 {
				t.Fatal("direct entry created a task or called the transfer Provider")
			}
			if candidate, err := resolve(func() bool { return true }); err != nil || candidate.Preexisting || store.beginCount != 1 {
				t.Fatalf("authorized Resolve error=%v preexisting=%t tasks=%d", err, candidate.Preexisting, store.beginCount)
			}
		})
	}
}

// TestPlaybackIntentAllowsReuseBeforeAndInsideContentLock keeps normal probes
// compatible when a file exists or a concurrent transfer has just created it.
func TestPlaybackIntentAllowsReuseBeforeAndInsideContentLock(t *testing.T) {
	for _, underLock := range []bool{false, true} {
		t.Run(map[bool]string{false: "existing", true: "concurrent creation"}[underLock], func(t *testing.T) {
			provider := newFakeProvider()
			provider.searchResults = [][]p115.File{{provider.targetFile}}
			if underLock {
				provider.searchResults = [][]p115.File{{}, {provider.targetFile}}
			}
			service := newRoutedDirectPlayForTest(t, &fakeRoutedAccountRuntime{}, provider, p115quota.NewMemoryLeaseStore())
			request := routedMediaPathRequest(http.MethodGet, "reuse")
			request.CanCreateTransfer = func() bool { t.Error("existing target checked transfer intent"); return false }
			candidate, err := service.ResolveMediaPath(context.Background(), request)
			if err != nil || candidate.URL == "" || !candidate.Preexisting {
				t.Fatalf("reuse candidate=%t preexisting=%t error=%v", candidate.URL != "", candidate.Preexisting, err)
			}
			assertNoIntentTransfer(t, service, provider)
		})
	}
}

// TestPlaybackIntentAllowsFirstTransferAndCrossSessionCacheReuse separates the
// one-time write permission from the existing ten-minute metadata cache.
func TestPlaybackIntentAllowsFirstTransferAndCrossSessionCacheReuse(t *testing.T) {
	provider := newFakeProvider()
	service := newRoutedDirectPlayForTest(t, &fakeRoutedAccountRuntime{}, provider, p115quota.NewMemoryLeaseStore())
	request := routedMediaPathRequest(http.MethodGet, "play")
	checks := 0
	request.CanCreateTransfer = func() bool { checks++; return true }
	candidate, err := service.ResolveMediaPath(context.Background(), request)
	if err != nil || candidate.URL == "" || candidate.Preexisting || checks != 1 {
		t.Fatalf("first transfer error=%v preexisting=%t checks=%d", err, candidate.Preexisting, checks)
	}
	beforeCalls := len(provider.calls)
	request.PlaySessionID = "later-probe"
	request.CanCreateTransfer = nil
	candidate, err = service.ResolveMediaPath(context.Background(), request)
	if err != nil || !candidate.Preexisting || candidate.Routing.TransferChecked || len(provider.calls) != beforeCalls {
		t.Fatalf("cache reuse error=%v preexisting=%t quota=%t calls=%v", err, candidate.Preexisting, candidate.Routing.TransferChecked, provider.calls)
	}
	if service.store.(*fakeTaskStore).beginCount != 1 || len(provider.uploadRequests) != 1 {
		t.Fatal("cache reuse duplicated retained transfer")
	}
}

// TestPlaybackIntentConcurrentProbeReusesAuthorizedTransfer proves a waiting
// probe can share a newly created file without spending a second transfer.
func TestPlaybackIntentConcurrentProbeReusesAuthorizedTransfer(t *testing.T) {
	provider := newFakeProvider()
	service := newRoutedDirectPlayForTest(t, &fakeRoutedAccountRuntime{}, provider, p115quota.NewMemoryLeaseStore())
	entered, release := make(chan struct{}), make(chan struct{})
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	service.provider = &lifecycleProvider{fakeProvider: provider, downloadStep: func() {
		close(entered)
		select {
		case <-release:
		case <-ctx.Done():
		}
	}}
	type outcome struct {
		candidate RedirectCandidate
		err       error
	}
	owner, waiter := make(chan outcome, 1), make(chan outcome, 1)
	request := routedMediaPathRequest(http.MethodGet, "authorized-owner")
	go func() { c, err := service.ResolveMediaPath(ctx, request); owner <- outcome{c, err} }()
	select {
	case <-entered:
	case <-ctx.Done():
		t.Fatal("authorized transfer did not reach download")
	}
	probe := routedMediaPathRequest(http.MethodGet, "concurrent-probe")
	probe.CanCreateTransfer = nil
	go func() { c, err := service.ResolveMediaPath(ctx, probe); waiter <- outcome{c, err} }()
	key := mediaRequestKey(service.serverID, request)
	for {
		service.mediaCache.flights.mu.Lock()
		entry := service.mediaCache.flights.entries[key]
		queued := entry != nil && entry.refs == 2
		service.mediaCache.flights.mu.Unlock()
		if queued {
			break
		}
		if ctx.Err() != nil {
			close(release)
			<-owner
			<-waiter
			t.Fatal("probe did not queue behind authorized transfer")
		}
		time.Sleep(time.Millisecond)
	}
	close(release)
	first, second := <-owner, <-waiter
	if first.err != nil || first.candidate.Preexisting || second.err != nil || !second.candidate.Preexisting {
		t.Fatalf("owner preexisting=%t error=%v; probe preexisting=%t error=%v", first.candidate.Preexisting, first.err, second.candidate.Preexisting, second.err)
	}
	if service.store.(*fakeTaskStore).beginCount != 1 || len(provider.uploadRequests) != 1 || countDownloadCalls(provider) != 1 {
		t.Fatalf("concurrent probe duplicated transfer: calls=%v", provider.calls)
	}
	if second.candidate.Routing.TransferChecked || second.candidate.Routing.AccountUsage.OccupiedStreams != 2 {
		t.Fatalf("probe routing=%+v", second.candidate.Routing)
	}
}

type intentWaitingLocker struct {
	fakeTaskLocker
	waiting chan struct{}
	resume  chan struct{}
}

// Acquire lets the test revoke intent while an actual content-lock waiter is
// blocked, then delegates release accounting to the ordinary fake locker.
func (locker *intentWaitingLocker) Acquire(ctx context.Context, accountID, sha1 string, size int64) (taskLock, error) {
	close(locker.waiting)
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-locker.resume:
		return locker.fakeTaskLocker.Acquire(ctx, accountID, sha1, size)
	}
}

// TestPlaybackIntentRevokedDuringContentLockWait ensures permission is checked
// after the lock wait and second lookup, rather than captured at request start.
func TestPlaybackIntentRevokedDuringContentLockWait(t *testing.T) {
	provider := newFakeProvider()
	service := newRoutedDirectPlayForTest(t, &fakeRoutedAccountRuntime{}, provider, p115quota.NewMemoryLeaseStore())
	locker := &intentWaitingLocker{waiting: make(chan struct{}), resume: make(chan struct{})}
	service.locker = locker
	var allowed atomic.Bool
	allowed.Store(true)
	var checks atomic.Int32
	request := routedMediaPathRequest(http.MethodGet, "expired-while-waiting")
	request.CanCreateTransfer = func() bool { checks.Add(1); return allowed.Load() }
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	done := make(chan error, 1)
	go func() { _, err := service.ResolveMediaPath(ctx, request); done <- err }()
	select {
	case <-locker.waiting:
	case <-ctx.Done():
		t.Fatal("request did not reach content lock")
	}
	allowed.Store(false)
	close(locker.resume)
	if err := <-done; !errors.Is(err, ErrPlaybackIntentRequired) {
		t.Fatalf("revoked waiter error=%v", err)
	}
	if checks.Load() != 1 || locker.releaseCount != 1 {
		t.Fatalf("checks=%d content lock releases=%d", checks.Load(), locker.releaseCount)
	}
	assertNoIntentTransfer(t, service, provider)
}

// TestPlaybackIntentDenialPreservesActiveLease also prevents HEAD with an old
// active lease and explicit intent from recreating a deleted target.
func TestPlaybackIntentDenialPreservesActiveLease(t *testing.T) {
	for _, method := range []string{http.MethodGet, http.MethodHead} {
		t.Run(method, func(t *testing.T) {
			provider := newFakeProvider()
			provider.searchResults = [][]p115.File{{provider.targetFile}}
			leases := p115quota.NewMemoryLeaseStore()
			service := newRoutedDirectPlayForTest(t, &fakeRoutedAccountRuntime{}, provider, leases)
			service.mediaCache = nil
			request := routedMediaPathRequest(http.MethodGet, "active-session")
			if _, err := service.ResolveMediaPath(context.Background(), request); err != nil {
				t.Fatal(err)
			}
			event := PlaybackSessionEvent{UserID: request.UserID, MappingID: request.MappingID, DeviceID: request.DeviceID, PlaySessionID: request.PlaySessionID}
			if _, err := service.HandlePlaybackSessionEvent(context.Background(), event); err != nil {
				t.Fatal(err)
			}
			request.Method = method
			request.CanCreateTransfer = func() bool { return method == http.MethodHead }
			if _, err := service.ResolveMediaPath(context.Background(), request); !errors.Is(err, ErrPlaybackIntentRequired) {
				t.Fatalf("missing target error=%v", err)
			}
			assertNoIntentTransfer(t, service, provider)
			accountKey, _ := service.keyDeriver.PlaybackAccountKey("100")
			usage, err := leases.AccountUsage(context.Background(), accountKey, service.now())
			if err != nil || usage.ActiveStreams != 1 || usage.OccupiedStreams != 1 {
				t.Fatalf("denial changed active lease: %+v, %v", usage, err)
			}
		})
	}
}

// assertNoIntentTransfer checks both persistent task creation and all transfer
// Provider work, plus the pending/succeeded user quota state.
func assertNoIntentTransfer(t *testing.T, service *Service, provider *fakeProvider) {
	t.Helper()
	if service.store.(*fakeTaskStore).beginCount != 0 || len(provider.rangeRequests) != 0 || len(provider.uploadRequests) != 0 {
		t.Fatalf("unexpected transfer side effects: tasks=%d calls=%v", service.store.(*fakeTaskStore).beginCount, provider.calls)
	}
	dayStart, dayEnd := p115quota.DayWindow(service.now(), service.businessTimezone)
	usage, err := service.transferQuotas.TransferUsage(context.Background(), p115quota.TransferUsageRequest{
		UserID: "user-1", DayStart: dayStart, DayEnd: dayEnd,
	}, service.now())
	if err != nil || usage != (p115quota.TransferUsage{}) {
		t.Fatalf("unexpected transfer quota: %+v, %v", usage, err)
	}
}
