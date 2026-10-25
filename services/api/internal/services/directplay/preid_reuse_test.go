package directplay

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	p115 "github.com/konghang/ember/backend/internal/integrations/p115"
	"github.com/konghang/ember/backend/internal/models"
	"github.com/konghang/ember/backend/internal/services/p115account"
	"github.com/konghang/ember/backend/internal/services/p115quota"
)

// TestPreIDReuseEqualWork compares two complete transfers into different personal
// accounts. Only source prefix reads may decrease; quotas and targets stay separate.
func TestPreIDReuseEqualWork(t *testing.T) {
	for _, overlap := range []bool{false, true} {
		for _, reuse := range []bool{false, true} {
			t.Run(fmt.Sprintf("overlap=%t/reuse=%t", overlap, reuse), func(t *testing.T) {
				fixture := newPreIDFixture(t)
				if !reuse {
					fixture.service.preIDCache = nil
				}
				first, second := preIDRequest("user-1"), preIDRequest("user-2")
				var results [2]preIDResolveResult
				if overlap {
					entered, release := make(chan struct{}), make(chan struct{})
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
					firstDone := fixture.resolveAsync(context.Background(), first)
					awaitPreIDSignal(t, entered)
					secondDone := fixture.resolveAsync(context.Background(), second)
					// Both tasks exist before the first source read is released.
					awaitPreIDSignal(t, fixture.store.initializing)
					awaitPreIDSignal(t, fixture.store.initializing)
					if reuse {
						awaitPreIDGateRefs(t, fixture.service.preIDCache, 2)
					}
					close(release)
					results[0], results[1] = awaitPreIDResult(t, firstDone), awaitPreIDResult(t, secondDone)
				} else {
					results[0].candidate, results[0].err = fixture.service.ResolveMediaPath(context.Background(), first)
					results[1].candidate, results[1].err = fixture.service.ResolveMediaPath(context.Background(), second)
				}
				for i, result := range results {
					if result.err != nil || result.candidate.URL == "" || result.candidate.Preexisting {
						t.Fatalf("request %d: created=%t candidate=%t err=%v", i, !result.candidate.Preexisting, result.candidate.URL != "", result.err)
					}
				}
				fixture.assertEqualWork(t, results)
				counts := fixture.provider.snapshot()
				t.Logf("equal work: source=%d searches=%d prefixReads=%d uploads=%d verifies=%d downloads=%d tasks=2 quotas=user1:1,user2:1", counts.resolves, counts.searches, counts.hashes, counts.uploads, counts.verifies, counts.downloads)
				wantReads := 2
				if reuse {
					wantReads = 1
				}
				if counts.hashes != wantReads {
					t.Fatalf("preID HashFileRange calls=%d, want %d with unchanged target work", counts.hashes, wantReads)
				}
				if reuse {
					if results[0].candidate.Timing.preIDCache != "miss" || results[0].candidate.Timing.stages["preID"].calls != 1 ||
						results[1].candidate.Timing.preIDCache != "hit" || results[1].candidate.Timing.stages["preID"].calls != 0 {
						t.Fatal("cache hit duplicated the owner's Provider timing")
					}
					if overlap && results[1].candidate.Timing.stages["preIDWait"].calls != 1 {
						t.Fatal("overlapping hit omitted its independent wait timing")
					}
				}
			})
		}
	}
}

type preIDResolveResult struct {
	candidate RedirectCandidate
	err       error
}

type preIDFixture struct {
	service  *Service
	accounts *preIDAccounts
	provider *preIDProvider
	store    *preIDTaskStore
	quotas   *p115quota.MemoryLeaseStore
}

// newPreIDFixture gives overlapping requests race-safe accounts, unique task IDs
// and independent playback targets, with the real in-memory quota implementation.
func newPreIDFixture(t *testing.T) *preIDFixture {
	t.Helper()
	source, _ := (fakeAccountLoader{}).LoadActiveCredentialByRole(context.Background(), models.P115AccountRoleSource)
	accounts := &preIDAccounts{source: source}
	provider := &preIDProvider{sourceFile: newFakeProvider().sourceFile}
	store := &preIDTaskStore{tasks: make(map[string]*preIDTask), initializing: make(chan struct{}, 16)}
	quotas := p115quota.NewMemoryLeaseStore()
	deriver, err := p115quota.NewKeyDeriver("preid-fixture-root-key")
	if err != nil {
		t.Fatal(err)
	}
	location, err := time.LoadLocation("Asia/Singapore")
	if err != nil {
		t.Fatal(err)
	}
	service, err := newRoutedServiceWithDependencies(accounts, accounts, provider, store, &preIDLocker{}, quotas, deriver, "preid-server", location)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 10, 25, 12, 0, 0, 0, location)
	service.now = func() time.Time { return now }
	provider.expiresAt = now.Add(time.Hour)
	return &preIDFixture{service: service, accounts: accounts, provider: provider, store: store, quotas: quotas}
}

// preIDRequest varies caller and playback identities without changing the source.
func preIDRequest(user string) MediaPathResolveRequest {
	request := routedMediaPathRequest("GET", "preid-session-"+user)
	request.UserID, request.MappingID, request.DeviceID = user, "mapping-"+user, "device-"+user
	request.ClientUserAgent = "fixture-client-" + user
	return request
}

// resolveAsync keeps all mutable fixture state in the synchronized collaborators.
func (fixture *preIDFixture) resolveAsync(ctx context.Context, request MediaPathResolveRequest) <-chan preIDResolveResult {
	done := make(chan preIDResolveResult, 1)
	go func() {
		candidate, err := fixture.service.ResolveMediaPath(ctx, request)
		done <- preIDResolveResult{candidate: candidate, err: err}
	}()
	return done
}

// awaitPreIDSignal bounds blocked test coordination without timing-based assertions.
func awaitPreIDSignal(t *testing.T, signal <-chan struct{}) {
	t.Helper()
	select {
	case <-signal:
	case <-time.After(3 * time.Second):
		t.Fatal("preID fixture did not reach the expected barrier")
	}
}

// awaitPreIDResult fails promptly when a flight or transfer deadlocks.
func awaitPreIDResult(t *testing.T, result <-chan preIDResolveResult) preIDResolveResult {
	t.Helper()
	select {
	case resolved := <-result:
		return resolved
	case <-time.After(3 * time.Second):
		t.Fatal("preID resolution did not finish")
		return preIDResolveResult{}
	}
}

// assertEqualWork verifies actual task/target/quota results rather than inferring
// throughput from a faster fake or from successful redirect candidates alone.
func (fixture *preIDFixture) assertEqualWork(t *testing.T, results [2]preIDResolveResult) {
	t.Helper()
	counts := fixture.provider.snapshot()
	if counts.resolves != 2 || counts.searches != 4 || counts.uploads != 2 || counts.verifies != 2 || counts.downloads != 2 {
		t.Fatalf("target work changed: %+v", counts)
	}
	if results[0].candidate.TaskID == "" || results[0].candidate.TaskID == results[1].candidate.TaskID {
		t.Fatal("two personal targets must have two independent task IDs")
	}
	fixture.store.mu.Lock()
	defer fixture.store.mu.Unlock()
	if len(fixture.store.tasks) != 2 {
		t.Fatalf("tasks=%d, want 2", len(fixture.store.tasks))
	}
	for i, user := range []string{"user-1", "user-2"} {
		task := fixture.store.tasks[results[i].candidate.TaskID]
		if task == nil || task.input.PlaybackAccountID != "personal-"+user || !task.succeeded || task.failed {
			t.Fatalf("independent task for %s did not succeed", user)
		}
		start, end := p115quota.DayWindow(fixture.service.now(), fixture.service.businessTimezone)
		usage, err := fixture.quotas.TransferUsage(context.Background(), p115quota.TransferUsageRequest{UserID: user, DayStart: start, DayEnd: end}, fixture.service.now())
		if err != nil || usage.HourlyUsed != 1 || usage.DailyUsed != 1 || usage.Pending != 0 {
			t.Fatalf("quota %s=%+v err=%v", user, usage, err)
		}
	}
}

type preIDAccounts struct {
	mu     sync.Mutex
	source p115account.ActiveAccountCredential
	health []runtimeHealthEvent
}

// LoadEnabledSourceLocation returns only the current source mapping snapshot.
func (accounts *preIDAccounts) LoadEnabledSourceLocation(context.Context) (p115account.SourceLocation, error) {
	accounts.mu.Lock()
	defer accounts.mu.Unlock()
	return p115account.SourceLocation{AccountID: accounts.source.Credential.AccountID, EmbyPathPrefix: accounts.source.EmbyPathPrefix, SourceRootID: accounts.source.SourceRootID}, nil
}

// LoadActiveCredentialByRole returns a copied source credential for this request.
func (accounts *preIDAccounts) LoadActiveCredentialByRole(context.Context, models.P115AccountRole) (p115account.ActiveAccountCredential, error) {
	accounts.mu.Lock()
	defer accounts.mu.Unlock()
	return accounts.source, nil
}

// ResolvePlaybackRoute assigns a different personal account to every fixture user.
func (*preIDAccounts) ResolvePlaybackRoute(_ context.Context, user string, _ time.Time) (p115account.PlaybackRoute, error) {
	route := routedPlaybackFixture()
	route.AccountID, route.OwnerUserID, route.ProviderUserID = "personal-"+user, user, "100"+strings.TrimPrefix(user, "user-")
	return route, nil
}

// AcquirePlaybackRoute preserves the selected account instead of sharing a target.
func (*preIDAccounts) AcquirePlaybackRoute(_ context.Context, route p115account.PlaybackRoute) (p115account.ActiveAccountCredential, error) {
	return p115account.ActiveAccountCredential{Role: models.P115AccountRolePlayback, ProviderUserID: route.ProviderUserID,
		TargetParentID: route.TargetParentID, DownloadCacheVersion: route.ConfigVersion,
		Credential: p115.Credential{AccountID: route.AccountID, Cookie: "fixture-" + route.AccountID, AppType: "web", UserAgent: "fixture-agent"}}, nil
}

// ReportRuntimeHealth records independently completed source/playback observations.
func (accounts *preIDAccounts) ReportRuntimeHealth(_ context.Context, account p115account.ActiveAccountCredential, outcome p115account.RuntimeHealthOutcome) error {
	accounts.mu.Lock()
	defer accounts.mu.Unlock()
	accounts.health = append(accounts.health, runtimeHealthEvent{accountID: account.Credential.AccountID, role: account.Role, outcome: outcome})
	return nil
}

type preIDCounts struct{ resolves, searches, hashes, uploads, verifies, downloads int }

type preIDProvider struct {
	mu         sync.Mutex
	counts     preIDCounts
	sourceFile p115.File
	expiresAt  time.Time
	hash       func(context.Context, int, p115.FileRangeRequest) (p115.FileRangeHash, error)
	upload     func(context.Context, p115.Credential, p115.RapidUploadRequest) (p115.RapidUploadResult, error)
}

// snapshot returns synchronized operation counts, with no credentials or hashes.
func (provider *preIDProvider) snapshot() preIDCounts {
	provider.mu.Lock()
	defer provider.mu.Unlock()
	return provider.counts
}

// ResolveFileByPath re-resolves the source for every caller, including cache hits.
func (provider *preIDProvider) ResolveFileByPath(context.Context, p115.Credential, p115.FilePathQuery) (*p115.File, error) {
	provider.mu.Lock()
	defer provider.mu.Unlock()
	provider.counts.resolves++
	file := provider.sourceFile
	return &file, nil
}

// SearchBySHA1 models a missing file in every independently selected target account.
func (provider *preIDProvider) SearchBySHA1(context.Context, p115.Credential, p115.FileQuery) ([]p115.File, error) {
	provider.mu.Lock()
	defer provider.mu.Unlock()
	provider.counts.searches++
	return nil, nil
}

// HashFileRange releases the mutex before a controllable read, so tests can prove
// concurrent cancellation and shared work without serializing all fake operations.
func (provider *preIDProvider) HashFileRange(ctx context.Context, _ p115.Credential, request p115.FileRangeRequest) (p115.FileRangeHash, error) {
	provider.mu.Lock()
	provider.counts.hashes++
	call, hash := provider.counts.hashes, provider.hash
	provider.mu.Unlock()
	if hash != nil {
		return hash(ctx, call, request)
	}
	return preIDHash(request), nil
}

// preIDHash is deterministic for the exact requested byte count.
func preIDHash(request p115.FileRangeRequest) p115.FileRangeHash {
	return p115.FileRangeHash{SHA1: directPlayPreID, BytesRead: request.Range.End - request.Range.Start + 1}
}

// InitRapidUpload counts each target's own write and can independently fail it.
func (provider *preIDProvider) InitRapidUpload(ctx context.Context, credential p115.Credential, request p115.RapidUploadRequest) (p115.RapidUploadResult, error) {
	provider.mu.Lock()
	provider.counts.uploads++
	upload := provider.upload
	provider.mu.Unlock()
	if upload != nil {
		return upload(ctx, credential, request)
	}
	return p115.RapidUploadResult{Status: p115.RapidUploadReused}, nil
}

// FindTargetFile constructs a separately verified target for the routed account.
func (provider *preIDProvider) FindTargetFile(_ context.Context, credential p115.Credential, query p115.FileQuery) (*p115.File, error) {
	provider.mu.Lock()
	defer provider.mu.Unlock()
	provider.counts.verifies++
	return &p115.File{ID: "target-" + credential.AccountID, PickCode: "pick-" + credential.AccountID,
		ParentID: query.ParentID, SHA1: query.SHA1, Size: query.Size}, nil
}

// GetDownloadURL models a final playback URL without making any network request.
func (provider *preIDProvider) GetDownloadURL(context.Context, p115.Credential, p115.DownloadURLRequest) (p115.DownloadURLResult, error) {
	provider.mu.Lock()
	defer provider.mu.Unlock()
	provider.counts.downloads++
	return p115.DownloadURLResult{URL: "https://cdn.115.com/preid-fixture", ExpiresAt: provider.expiresAt,
		HeaderMode: p115.DownloadHeadersSameUserAgent, ConcurrentOpenLimit: 2}, nil
}

type preIDTask struct {
	input             beginAttemptInput
	statuses          []models.PlaybackTransferTaskStatus
	succeeded, failed bool
}

type preIDTaskStore struct {
	mu           sync.Mutex
	tasks        map[string]*preIDTask
	initializing chan struct{}
	beginErr     error
	statusErr    error
}

// BeginAttempt returns a unique task identity and records its target provenance.
func (store *preIDTaskStore) BeginAttempt(_ context.Context, input beginAttemptInput) (*models.PlaybackTransferTask, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	if store.beginErr != nil {
		return nil, store.beginErr
	}
	id := fmt.Sprintf("preid-task-%d", len(store.tasks)+1)
	store.tasks[id] = &preIDTask{input: input}
	return &models.PlaybackTransferTask{ID: id}, nil
}

// MarkStatus exposes an Initializing barrier after the task has been persisted.
func (store *preIDTaskStore) MarkStatus(_ context.Context, id string, status models.PlaybackTransferTaskStatus, _ time.Time) error {
	store.mu.Lock()
	defer store.mu.Unlock()
	if store.statusErr != nil {
		return store.statusErr
	}
	store.tasks[id].statuses = append(store.tasks[id].statuses, status)
	if status == models.PlaybackTransferTaskStatusInitializing {
		store.initializing <- struct{}{}
	}
	return nil
}

// IncrementAttempt leaves task identity unchanged when challenge retry is required.
func (*preIDTaskStore) IncrementAttempt(context.Context, string, time.Time) error { return nil }

// MarkSucceeded records the terminal result on exactly the supplied task.
func (store *preIDTaskStore) MarkSucceeded(_ context.Context, id string, _ p115.File, _ time.Time) error {
	store.mu.Lock()
	defer store.mu.Unlock()
	store.tasks[id].succeeded = true
	return nil
}

// MarkFailed records failures separately from another request's successful task.
func (store *preIDTaskStore) MarkFailed(_ context.Context, id, _, _ string, _ time.Time) error {
	store.mu.Lock()
	defer store.mu.Unlock()
	store.tasks[id].failed = true
	return nil
}

// TouchSucceeded implements the existing-file path without adding a new task.
func (*preIDTaskStore) TouchSucceeded(context.Context, string, string, int64, time.Time) error {
	return nil
}

type preIDLocker struct{ gate requestGate }
type preIDLock struct{ release func() }

// Acquire keeps the actual per-playback/content lock boundary in concurrent tests.
func (locker *preIDLocker) Acquire(ctx context.Context, account, sha1 string, size int64) (taskLock, error) {
	release, err := locker.gate.acquire(ctx, fmt.Sprintf("%s:%s:%d", account, sha1, size))
	if err != nil {
		return nil, err
	}
	return preIDLock{release: release}, nil
}

// Release relinquishes only this target's content lock.
func (lock preIDLock) Release() error { lock.release(); return nil }
