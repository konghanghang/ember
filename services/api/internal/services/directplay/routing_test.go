package directplay

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	p115integration "github.com/konghang/ember/backend/internal/integrations/p115"
	"github.com/konghang/ember/backend/internal/models"
	"github.com/konghang/ember/backend/internal/services/p115account"
	"github.com/konghang/ember/backend/internal/services/p115quota"
)

type fakeRoutedAccountRuntime struct {
	fakeAccountLoader
	route        p115account.PlaybackRoute
	routeErr     error
	acquireErr   error
	acquireCalls int
}

func (runtime *fakeRoutedAccountRuntime) ResolvePlaybackRoute(_ context.Context, userID string, _ time.Time) (p115account.PlaybackRoute, error) {
	if runtime.routeErr != nil {
		return runtime.route, runtime.routeErr
	}
	route := runtime.route
	if route.AccountID == "" {
		route = routedPlaybackFixture()
	}
	if route.PlaybackMode == models.P115PlaybackModePersonal {
		route.OwnerUserID = userID
	}
	return route, nil
}

func TestRoutedResolveMediaPathKeepsKnownPolicyDiagnosticsWhenAccountIsMissing(t *testing.T) {
	accounts := &fakeRoutedAccountRuntime{
		route: p115account.PlaybackRoute{
			PlaybackMode: models.P115PlaybackModePersonal, SimultaneousStreamLimit: 3,
			TransferHourlyLimit: 5, TransferDailyLimit: 10,
		},
		routeErr: p115account.ErrPersonalAccountMissing,
	}
	service := newRoutedDirectPlayForTest(t, accounts, newFakeProvider(), p115quota.NewMemoryLeaseStore())

	candidate, err := service.ResolveMediaPath(context.Background(), routedMediaPathRequest("GET", "session-missing"))
	if !errors.Is(err, ErrPersonalAccountMissing) {
		t.Fatalf("ResolveMediaPath() error = %v", err)
	}
	if !candidate.Routing.Routed || candidate.Routing.PlaybackMode != models.P115PlaybackModePersonal ||
		candidate.Routing.PlaybackAccountOwner != "current_user" || candidate.Routing.AccountLimitsAvailable {
		t.Fatalf("missing account routing diagnostics = %+v", candidate.Routing)
	}
}

func (runtime *fakeRoutedAccountRuntime) AcquirePlaybackRoute(_ context.Context, route p115account.PlaybackRoute) (p115account.ActiveAccountCredential, error) {
	runtime.acquireCalls++
	if runtime.acquireErr != nil {
		return p115account.ActiveAccountCredential{}, runtime.acquireErr
	}
	return p115account.ActiveAccountCredential{
		Role: models.P115AccountRolePlayback, ProviderUserID: route.ProviderUserID, TargetParentID: route.TargetParentID,
		Credential: p115integration.Credential{AccountID: route.AccountID, Cookie: "playback-cookie", AppType: "web", UserAgent: "fixture-agent"},
	}, nil
}

func routedPlaybackFixture() p115account.PlaybackRoute {
	return p115account.PlaybackRoute{
		PlaybackMode: models.P115PlaybackModePersonal, AccountID: "personal-account", OwnerUserID: "user-1",
		ProviderUserID: "100", TargetParentID: "200000002", TargetParentPath: "/Playback",
		ConfiguredMaxConcurrentStreams: 2, EffectiveMaxConcurrentStreams: 2, SimultaneousStreamLimit: 3,
		TransferHourlyLimit: 5, TransferDailyLimit: 10, Status: models.P115AccountStatusActive,
		UpdatedAt: time.Date(2026, 9, 4, 0, 0, 0, 0, time.UTC),
	}
}

func routedMediaPathRequest(method, sessionID string) MediaPathResolveRequest {
	return MediaPathResolveRequest{
		Path: "/mnt/cloudNAS/115lifetime/Media/fixture.mkv", ClientUserAgent: "Infuse-Fixture",
		Method: method, UserID: "user-1", MappingID: "mapping-1", DeviceID: "device-1", PlaySessionID: sessionID,
	}
}

func newRoutedDirectPlayForTest(t *testing.T, accounts *fakeRoutedAccountRuntime, provider *fakeProvider, leases p115quota.LeaseStore) *Service {
	t.Helper()
	deriver, err := p115quota.NewKeyDeriver("fixture-root-key")
	if err != nil {
		t.Fatalf("NewKeyDeriver() error = %v", err)
	}
	service, err := newRoutedServiceWithDependencies(accounts, accounts, provider, &fakeTaskStore{}, &fakeTaskLocker{}, leases, deriver, "server-1", time.UTC)
	if err != nil {
		t.Fatalf("newRoutedServiceWithDependencies() error = %v", err)
	}
	service.now = func() time.Time { return time.Date(2026, 9, 4, 0, 0, 0, 0, time.UTC) }
	return service
}

func TestRoutedResolveMediaPathCreatesOneReservationAndReusesIt(t *testing.T) {
	accounts := &fakeRoutedAccountRuntime{}
	provider := newFakeProvider()
	provider.searchResults = [][]p115integration.File{{provider.targetFile}, {provider.targetFile}}
	leases := p115quota.NewMemoryLeaseStore()
	service := newRoutedDirectPlayForTest(t, accounts, provider, leases)
	request := routedMediaPathRequest("GET", "session-1")

	if _, err := service.ResolveMediaPath(context.Background(), request); err != nil {
		t.Fatalf("ResolveMediaPath(first) error = %v", err)
	}
	if _, err := service.ResolveMediaPath(context.Background(), request); err != nil {
		t.Fatalf("ResolveMediaPath(duplicate) error = %v", err)
	}
	accountKey, _ := service.keyDeriver.PlaybackAccountKey("100")
	usage, err := leases.AccountUsage(context.Background(), accountKey, service.now())
	if err != nil || usage.ReservedStreams != 1 || usage.OccupiedStreams != 1 {
		t.Fatalf("usage = %+v, %v", usage, err)
	}
}

func TestRoutedResolveMediaPathEnforcesAccountLimitAndReleasesFailedCandidate(t *testing.T) {
	accounts := &fakeRoutedAccountRuntime{}
	accounts.route = routedPlaybackFixture()
	accounts.route.EffectiveMaxConcurrentStreams = 1
	provider := newFakeProvider()
	provider.searchResults = [][]p115integration.File{{provider.targetFile}}
	leases := p115quota.NewMemoryLeaseStore()
	service := newRoutedDirectPlayForTest(t, accounts, provider, leases)

	if _, err := service.ResolveMediaPath(context.Background(), routedMediaPathRequest("GET", "session-1")); err != nil {
		t.Fatalf("ResolveMediaPath(first) error = %v", err)
	}
	limited, err := service.ResolveMediaPath(context.Background(), routedMediaPathRequest("GET", "session-2"))
	if !errors.Is(err, ErrAccountConcurrencyExceeded) {
		t.Fatalf("ResolveMediaPath(limit) error = %v", err)
	}
	if !limited.Routing.LeaseUsageAvailable || limited.Routing.AccountUsage.OccupiedStreams != 1 ||
		limited.Routing.UserUsage.OccupiedStreams != 1 {
		t.Fatalf("ResolveMediaPath(limit) routing = %+v", limited.Routing)
	}

	failedAccounts := &fakeRoutedAccountRuntime{acquireErr: p115account.ErrRuntimeStateChanged}
	failedLeases := p115quota.NewMemoryLeaseStore()
	failedService := newRoutedDirectPlayForTest(t, failedAccounts, newFakeProvider(), failedLeases)
	if _, err := failedService.ResolveMediaPath(context.Background(), routedMediaPathRequest("GET", "session-3")); !errors.Is(err, ErrAccountUnavailable) {
		t.Fatalf("ResolveMediaPath(stale account) error = %v", err)
	}
	accountKey, _ := failedService.keyDeriver.PlaybackAccountKey("100")
	usage, err := failedLeases.AccountUsage(context.Background(), accountKey, failedService.now())
	if err != nil || usage.OccupiedStreams != 0 {
		t.Fatalf("failed reservation usage = %+v, %v", usage, err)
	}
}

func TestRoutedResolveMediaPathHEADRequiresExistingLease(t *testing.T) {
	accounts := &fakeRoutedAccountRuntime{}
	provider := newFakeProvider()
	service := newRoutedDirectPlayForTest(t, accounts, provider, p115quota.NewMemoryLeaseStore())

	if _, err := service.ResolveMediaPath(context.Background(), routedMediaPathRequest("HEAD", "session-1")); !errors.Is(err, ErrHeadLeaseMissing) {
		t.Fatalf("ResolveMediaPath(HEAD) error = %v", err)
	}
	if accounts.acquireCalls != 0 || len(provider.calls) != 0 {
		t.Fatalf("HEAD without lease acquired=%d providerCalls=%v", accounts.acquireCalls, provider.calls)
	}
}

func TestRoutedPlaybackEventsPromotePauseAndStopExistingLease(t *testing.T) {
	accounts := &fakeRoutedAccountRuntime{}
	provider := newFakeProvider()
	provider.searchResults = [][]p115integration.File{{provider.targetFile}}
	leases := p115quota.NewMemoryLeaseStore()
	service := newRoutedDirectPlayForTest(t, accounts, provider, leases)
	request := routedMediaPathRequest("GET", "session-1")
	if _, err := service.ResolveMediaPath(context.Background(), request); err != nil {
		t.Fatalf("ResolveMediaPath() error = %v", err)
	}

	event := PlaybackSessionEvent{UserID: request.UserID, MappingID: request.MappingID, DeviceID: request.DeviceID, PlaySessionID: request.PlaySessionID}
	if result, err := service.HandlePlaybackSessionEvent(context.Background(), event); err != nil || !result.Found || result.State != p115quota.LeaseStateActive {
		t.Fatalf("HandlePlaybackSessionEvent(active) = %+v, %v", result, err)
	}
	event.IsProgress = true
	event.IsPaused = true
	if result, err := service.HandlePlaybackSessionEvent(context.Background(), event); err != nil || result.State != p115quota.LeaseStatePaused {
		t.Fatalf("HandlePlaybackSessionEvent(paused) = %+v, %v", result, err)
	}
	event.Stopped = true
	if result, err := service.HandlePlaybackSessionEvent(context.Background(), event); err != nil || !result.Found || result.Account.OccupiedStreams != 0 {
		t.Fatalf("HandlePlaybackSessionEvent(stopped) = %+v, %v", result, err)
	}
}

type failingLeaseStore struct {
	p115quota.Store
	err error
}

func (store failingLeaseStore) Reserve(context.Context, p115quota.ReserveRequest, time.Time) (p115quota.ReserveResult, error) {
	return p115quota.ReserveResult{}, store.err
}

func TestRoutedResolveMediaPathRedisFailureDoesNotAcquireProviderAccount(t *testing.T) {
	accounts := &fakeRoutedAccountRuntime{}
	service := newRoutedDirectPlayForTest(t, accounts, newFakeProvider(), failingLeaseStore{
		Store: p115quota.NewMemoryLeaseStore(), err: p115quota.ErrRedisUnavailable,
	})
	_, err := service.ResolveMediaPath(context.Background(), routedMediaPathRequest("GET", "session-1"))
	if !errors.Is(err, ErrRedisUnavailable) || accounts.acquireCalls != 0 {
		t.Fatalf("ResolveMediaPath() error=%v acquireCalls=%d", err, accounts.acquireCalls)
	}
}

func TestRoutedSessionIdentityDoesNotLeakIntoAccountKeys(t *testing.T) {
	deriver, err := p115quota.NewKeyDeriver("fixture-root-key")
	if err != nil {
		t.Fatal(err)
	}
	fingerprint, err := deriver.SessionFingerprint(p115quota.SessionIdentity{
		ServerID: "server-1", UserID: "user-1", MappingID: "mapping-1", DeviceID: "device-1", PlaySessionID: "session-secret",
	})
	if err != nil || strings.Contains(fingerprint, "session-secret") {
		t.Fatalf("fingerprint=%q error=%v", fingerprint, err)
	}
}

func TestRoutedTransferQuotaChargesOnlyVerifiedNewTargets(t *testing.T) {
	accounts := &fakeRoutedAccountRuntime{}
	accounts.route = routedPlaybackFixture()
	accounts.route.TransferHourlyLimit = 1
	accounts.route.TransferDailyLimit = 10
	provider := newFakeProvider()
	provider.searchResults = [][]p115integration.File{{}, {}, {}, {}}
	quotas := p115quota.NewMemoryLeaseStore()
	service := newRoutedDirectPlayForTest(t, accounts, provider, quotas)

	if _, err := service.ResolveMediaPath(context.Background(), routedMediaPathRequest("GET", "session-1")); err != nil {
		t.Fatalf("ResolveMediaPath(first transfer) error = %v", err)
	}
	limited, err := service.ResolveMediaPath(context.Background(), routedMediaPathRequest("GET", "session-2"))
	if !errors.Is(err, ErrTransferQuotaExceeded) {
		t.Fatalf("ResolveMediaPath(quota) error = %v", err)
	}
	if !limited.Routing.TransferChecked || !limited.Routing.TransferUsageAvailable ||
		limited.Routing.TransferUsage.HourlyUsed != 1 || limited.Routing.TransferUsage.DailyUsed != 1 {
		t.Fatalf("ResolveMediaPath(quota) routing = %+v", limited.Routing)
	}
	initCalls := 0
	for _, call := range provider.calls {
		if call == "init_upload" {
			initCalls++
		}
	}
	if initCalls != 1 {
		t.Fatalf("InitRapidUpload calls = %d, want 1", initCalls)
	}
	dayStart, dayEnd := p115quota.DayWindow(service.now(), time.UTC)
	usage, err := quotas.TransferUsage(context.Background(), p115quota.TransferUsageRequest{
		UserID: "user-1", DayStart: dayStart, DayEnd: dayEnd,
	}, service.now())
	if err != nil || usage.HourlyUsed != 1 || usage.DailyUsed != 1 || usage.Pending != 0 {
		t.Fatalf("transfer usage = %+v, %v", usage, err)
	}
}

func TestRoutedTransferFailureReleasesPendingQuota(t *testing.T) {
	accounts := &fakeRoutedAccountRuntime{}
	accounts.route = routedPlaybackFixture()
	accounts.route.TransferHourlyLimit = 1
	accounts.route.TransferDailyLimit = 1
	provider := newFakeProvider()
	provider.searchResults = [][]p115integration.File{{}, {}, {}, {}}
	provider.uploadResults = []p115integration.RapidUploadResult{
		{Status: p115integration.RapidUploadOrdinaryUploadRequired},
		{Status: p115integration.RapidUploadReused},
	}
	service := newRoutedDirectPlayForTest(t, accounts, provider, p115quota.NewMemoryLeaseStore())

	if _, err := service.ResolveMediaPath(context.Background(), routedMediaPathRequest("GET", "session-1")); !errors.Is(err, ErrRapidUploadUnavailable) {
		t.Fatalf("ResolveMediaPath(failed transfer) error = %v", err)
	}
	if _, err := service.ResolveMediaPath(context.Background(), routedMediaPathRequest("GET", "session-2")); err != nil {
		t.Fatalf("ResolveMediaPath(after refund) error = %v", err)
	}
}

type flakyTransferStore struct {
	*p115quota.MemoryLeaseStore
	mu             sync.Mutex
	commitFailures int
	commitCalls    int
}

func (store *flakyTransferStore) CommitTransfer(
	ctx context.Context,
	request p115quota.TransferCommitRequest,
	now time.Time,
) (p115quota.TransferCommitResult, error) {
	store.mu.Lock()
	store.commitCalls++
	if store.commitFailures > 0 {
		store.commitFailures--
		store.mu.Unlock()
		return p115quota.TransferCommitResult{}, p115quota.ErrRedisUnavailable
	}
	store.mu.Unlock()
	return store.MemoryLeaseStore.CommitTransfer(ctx, request, now)
}

func TestRoutedTransferCommitRetriesWithinIndependentBudget(t *testing.T) {
	accounts := &fakeRoutedAccountRuntime{}
	provider := newFakeProvider()
	provider.searchResults = [][]p115integration.File{{}, {}}
	store := &flakyTransferStore{MemoryLeaseStore: p115quota.NewMemoryLeaseStore(), commitFailures: 2}
	service := newRoutedDirectPlayForTest(t, accounts, provider, store)
	service.transferRetryInterval = time.Millisecond

	if _, err := service.ResolveMediaPath(context.Background(), routedMediaPathRequest("GET", "session-1")); err != nil {
		t.Fatalf("ResolveMediaPath() error = %v", err)
	}
	if store.commitCalls != 3 {
		t.Fatalf("commit calls = %d, want 3", store.commitCalls)
	}
}

func TestRoutedTransferCommitFailureFallsBackAndKeepsPending(t *testing.T) {
	accounts := &fakeRoutedAccountRuntime{}
	provider := newFakeProvider()
	provider.searchResults = [][]p115integration.File{{}, {}}
	store := &flakyTransferStore{MemoryLeaseStore: p115quota.NewMemoryLeaseStore(), commitFailures: 1000}
	service := newRoutedDirectPlayForTest(t, accounts, provider, store)
	service.transferCommitBudget = 10 * time.Millisecond
	service.transferRetryInterval = time.Millisecond

	if _, err := service.ResolveMediaPath(context.Background(), routedMediaPathRequest("GET", "session-1")); !errors.Is(err, ErrTransferQuotaCommitFailed) {
		t.Fatalf("ResolveMediaPath() error = %v", err)
	}
	dayStart, dayEnd := p115quota.DayWindow(service.now(), time.UTC)
	usage, err := store.TransferUsage(context.Background(), p115quota.TransferUsageRequest{
		UserID: "user-1", DayStart: dayStart, DayEnd: dayEnd,
	}, service.now())
	if err != nil || usage.Pending != 1 || usage.HourlyUsed != 0 {
		t.Fatalf("failed commit usage = %+v, %v", usage, err)
	}
}
