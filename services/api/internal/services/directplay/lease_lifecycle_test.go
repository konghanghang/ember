package directplay

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	p115 "github.com/konghang/ember/backend/internal/integrations/p115"
	"github.com/konghang/ember/backend/internal/services/p115quota"
)

type lifecycleProvider struct {
	*fakeProvider
	resolve      func(context.Context) error
	downloadStep func()
}

// ResolveFileByPath injects request-owned delays without external I/O.
func (p *lifecycleProvider) ResolveFileByPath(ctx context.Context, credential p115.Credential, query p115.FilePathQuery) (*p115.File, error) {
	if p.resolve != nil {
		if err := p.resolve(ctx); err != nil {
			return nil, err
		}
	}
	return p.fakeProvider.ResolveFileByPath(ctx, credential, query)
}

// GetDownloadURL advances the test's clock just before candidate completion.
func (p *lifecycleProvider) GetDownloadURL(ctx context.Context, credential p115.Credential, request p115.DownloadURLRequest) (p115.DownloadURLResult, error) {
	if p.downloadStep != nil {
		p.downloadStep()
	}
	return p.fakeProvider.GetDownloadURL(ctx, credential, request)
}

type observedLeaseStore struct {
	*p115quota.MemoryLeaseStore
	confirmed  chan time.Time
	confirmErr error
}

// Confirm exposes completed heartbeats to the deterministic slow-path test.
func (s *observedLeaseStore) Confirm(ctx context.Context, request p115quota.ConfirmRequest, now time.Time) (p115quota.ConfirmResult, error) {
	if s.confirmErr != nil {
		return p115quota.ConfirmResult{}, s.confirmErr
	}
	result, err := s.MemoryLeaseStore.Confirm(ctx, request, now)
	if request.RenewReservation && result.Found {
		select {
		case s.confirmed <- result.Session.ExpiresAt:
		default:
		}
	}
	return result, err
}

// TestCandidateRequiresLiveLease prevents expired and stopped sessions from
// escaping as usable candidates even when the Provider succeeded.
func TestCandidateRequiresLiveLease(t *testing.T) {
	for _, stop := range []bool{false, true} {
		t.Run(map[bool]string{false: "expired", true: "stopped"}[stop], func(t *testing.T) {
			base := newFakeProvider()
			base.searchResults = [][]p115.File{{base.targetFile}}
			leases := p115quota.NewMemoryLeaseStore()
			service := newRoutedDirectPlayForTest(t, &fakeRoutedAccountRuntime{}, base, leases)
			service.leaseHeartbeatInterval = time.Hour
			now := service.now()
			service.now = func() time.Time { return now }
			request := routedMediaPathRequest("GET", "lease-lifecycle")
			service.provider = &lifecycleProvider{fakeProvider: base, downloadStep: func() {
				if !stop {
					now = now.Add(p115quota.ReservationTTL + time.Second)
					return
				}
				_, err := service.HandlePlaybackSessionEvent(context.Background(), PlaybackSessionEvent{UserID: request.UserID, MappingID: request.MappingID, DeviceID: request.DeviceID, PlaySessionID: request.PlaySessionID, Stopped: true})
				if err != nil {
					t.Fatal(err)
				}
			}}
			candidate, err := service.ResolveMediaPath(context.Background(), request)
			if !errors.Is(err, ErrPlaybackLeaseLost) || candidate.URL != "" {
				t.Fatalf("candidate=%t error=%v", candidate.URL != "", err)
			}
		})
	}
}

// TestSlowCandidateRenewsReservation advances forty business seconds while
// synchronizing with real heartbeat completions instead of sleeping for TTLs.
func TestSlowCandidateRenewsReservation(t *testing.T) {
	base := newFakeProvider()
	base.searchResults = [][]p115.File{{base.targetFile}}
	leases := &observedLeaseStore{MemoryLeaseStore: p115quota.NewMemoryLeaseStore(), confirmed: make(chan time.Time, 1)}
	service := newRoutedDirectPlayForTest(t, &fakeRoutedAccountRuntime{}, base, leases)
	service.leaseHeartbeatInterval = time.Millisecond
	var clock atomic.Int64
	clock.Store(service.now().UnixMilli())
	service.now = func() time.Time { return time.UnixMilli(clock.Load()) }
	service.provider = &lifecycleProvider{fakeProvider: base, downloadStep: func() {
		for range 2 {
			clock.Add(20_000)
			deadline := time.After(time.Second)
			for {
				select {
				case expiry := <-leases.confirmed:
					if expiry.Before(service.now().Add(p115quota.ReservationTTL)) {
						continue
					}
				case <-deadline:
					t.Fatal("reservation heartbeat missing")
				}
				break
			}
		}
	}}
	request := routedMediaPathRequest("GET", "slow-session")
	candidate, err := service.ResolveMediaPath(context.Background(), request)
	if err != nil || candidate.URL == "" {
		t.Fatalf("candidate=%t error=%v", candidate.URL != "", err)
	}
	event, err := service.HandlePlaybackSessionEvent(context.Background(), PlaybackSessionEvent{UserID: request.UserID, MappingID: request.MappingID, DeviceID: request.DeviceID, PlaySessionID: request.PlaySessionID})
	if err != nil || !event.Found || event.Account.ActiveStreams != 1 {
		t.Fatalf("Playing result=%+v error=%v", event, err)
	}
}

// TestSameSessionCancellationCannotReleaseNextRequest runs the cancellation
// ordering that previously removed another successful request's reservation.
func TestSameSessionCancellationCannotReleaseNextRequest(t *testing.T) {
	base := newFakeProvider()
	base.searchResults = [][]p115.File{{base.targetFile}}
	leases := p115quota.NewMemoryLeaseStore()
	service := newRoutedDirectPlayForTest(t, &fakeRoutedAccountRuntime{}, base, leases)
	entered := make(chan struct{})
	var calls atomic.Int32
	service.provider = &lifecycleProvider{fakeProvider: base, resolve: func(ctx context.Context) error {
		if calls.Add(1) == 1 {
			close(entered)
			<-ctx.Done()
			return ctx.Err()
		}
		return nil
	}}
	request := routedMediaPathRequest("GET", "same-session")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	first := make(chan error, 1)
	go func() { _, err := service.ResolveMediaPath(ctx, request); first <- err }()
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("first request missing")
	}
	secondCtx, cancelSecond := context.WithTimeout(context.Background(), time.Second)
	defer cancelSecond()
	second := make(chan error, 1)
	go func() {
		candidate, err := service.ResolveMediaPath(secondCtx, request)
		if err == nil && candidate.URL == "" {
			err = errors.New("missing candidate")
		}
		second <- err
	}()
	deadline := time.Now().Add(time.Second)
	for {
		service.sessionRequests.mu.Lock()
		refs := 0
		for _, entry := range service.sessionRequests.entries {
			refs += entry.refs
		}
		service.sessionRequests.mu.Unlock()
		if refs == 2 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("second request did not queue")
		}
		time.Sleep(time.Millisecond)
	}
	cancel()
	if err := <-first; !errors.Is(err, context.Canceled) {
		t.Fatalf("first=%v", err)
	}
	if err := <-second; err != nil {
		t.Fatalf("second=%v", err)
	}
	key, _ := service.keyDeriver.PlaybackAccountKey("100")
	usage, err := leases.AccountUsage(context.Background(), key, service.now())
	if err != nil || usage.OccupiedStreams != 1 {
		t.Fatalf("usage=%+v error=%v", usage, err)
	}
}

// TestLeaseGuardFailuresDoNotPoisonAccountHealth covers the internal budget
// and heartbeat outage without treating either as a Provider account failure.
func TestLeaseGuardFailuresDoNotPoisonAccountHealth(t *testing.T) {
	for _, outage := range []bool{false, true} {
		t.Run(map[bool]string{false: "budget", true: "redis"}[outage], func(t *testing.T) {
			base := newFakeProvider()
			health := &fakeAccountHealthReporter{}
			leases := &observedLeaseStore{MemoryLeaseStore: p115quota.NewMemoryLeaseStore()}
			if outage {
				leases.confirmErr = p115quota.ErrRedisUnavailable
			}
			service := newRoutedDirectPlayForTest(t, &fakeRoutedAccountRuntime{fakeAccountLoader: fakeAccountLoader{health: health}}, base, leases)
			service.resolveTimeout = 20 * time.Millisecond
			if outage {
				service.resolveTimeout = time.Second
			}
			service.leaseHeartbeatInterval = time.Millisecond
			service.provider = &lifecycleProvider{fakeProvider: base, resolve: func(ctx context.Context) error { <-ctx.Done(); return p115.ErrProviderUnavailable }}
			candidate, err := service.ResolveMediaPath(context.Background(), routedMediaPathRequest("GET", "guard-session"))
			want := ErrPlaybackResolveTimeout
			if outage {
				want = ErrRedisUnavailable
			}
			if !errors.Is(err, want) || candidate.URL != "" {
				t.Fatalf("candidate=%t error=%v want=%v", candidate.URL != "", err, want)
			}
			if len(health.events) != 0 {
				t.Fatalf("guard failure changed health: %+v", health.events)
			}
		})
	}
}
