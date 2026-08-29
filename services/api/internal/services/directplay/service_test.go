package directplay

import (
	"context"
	"errors"
	"reflect"
	"sync"
	"testing"
	"time"

	p115integration "github.com/konghang/ember/backend/internal/integrations/p115"
	"github.com/konghang/ember/backend/internal/models"
	"github.com/konghang/ember/backend/internal/services/p115account"
)

const (
	directPlaySourceSHA1    = "0123456789ABCDEF0123456789ABCDEF01234567"
	directPlayPreID         = "89ABCDEF0123456789ABCDEF0123456789ABCDEF"
	directPlayChallengeSHA1 = "FEDCBA9876543210FEDCBA9876543210FEDCBA98"
)

func TestNewServiceRejectsMissingProductionDependencies(t *testing.T) {
	if _, err := NewService(nil, nil, nil); !errors.Is(err, ErrStoreUnavailable) {
		t.Fatalf("NewService() error = %v, want ErrStoreUnavailable", err)
	}
}

func TestServiceResolveMediaPathUsesSourceAccountLocation(t *testing.T) {
	provider := newFakeProvider()
	provider.searchResults = [][]p115integration.File{{provider.targetFile}}
	service := newServiceWithDependencies(fakeAccountLoader{}, provider, &fakeTaskStore{}, &fakeTaskLocker{})

	result, err := service.ResolveMediaPath(context.Background(), MediaPathResolveRequest{
		Path:            "/mnt/cloudNAS/115lifetime/Media/fixture.mkv",
		Size:            1024 * 1024,
		ClientUserAgent: "Infuse-Fixture",
	})
	if err != nil {
		t.Fatalf("ResolveMediaPath() error = %v", err)
	}
	if !result.Preexisting || provider.resolvedQuery.RootID != "0" || provider.resolvedQuery.RelativePath != "Media/fixture.mkv" {
		t.Fatalf("ResolveMediaPath() result=%+v query=%+v", result, provider.resolvedQuery)
	}
	if result.PathMapping.OriginalPath != "/mnt/cloudNAS/115lifetime/Media/fixture.mkv" ||
		result.PathMapping.EmbyPathPrefix != "/mnt/cloudNAS/115lifetime" ||
		result.PathMapping.SourceRootID != "0" || result.PathMapping.RelativePath != "Media/fixture.mkv" {
		t.Fatalf("ResolveMediaPath() mapping=%+v", result.PathMapping)
	}
}

func TestServiceResolveMediaPathRejectsPrefixBoundaryAndTraversal(t *testing.T) {
	tests := []string{
		"/mnt/cloudNAS/115lifetime2/Media/fixture.mkv",
		"/mnt/cloudNAS/115lifetime/Media/../fixture.mkv",
		"/mnt/cloudNAS/115lifetime",
	}
	for _, mediaPath := range tests {
		t.Run(mediaPath, func(t *testing.T) {
			provider := newFakeProvider()
			service := newServiceWithDependencies(fakeAccountLoader{}, provider, &fakeTaskStore{}, &fakeTaskLocker{})
			_, err := service.ResolveMediaPath(context.Background(), MediaPathResolveRequest{
				Path: mediaPath, Size: 1024 * 1024, ClientUserAgent: "Infuse-Fixture",
			})
			if !errors.Is(err, ErrPathNotMapped) {
				t.Fatalf("ResolveMediaPath(%q) error = %v, want ErrPathNotMapped", mediaPath, err)
			}
			if len(provider.calls) != 0 {
				t.Fatalf("invalid media path reached provider: %v", provider.calls)
			}
		})
	}
}

func TestServiceResolveMediaPathReturnsKnownMappingOnPrefixMismatch(t *testing.T) {
	service := newServiceWithDependencies(fakeAccountLoader{}, newFakeProvider(), &fakeTaskStore{}, &fakeTaskLocker{})
	result, err := service.ResolveMediaPath(context.Background(), MediaPathResolveRequest{
		Path:            "/mnt/other/Media/fixture.mkv",
		Size:            1024 * 1024,
		ClientUserAgent: "Infuse-Fixture",
	})
	if !errors.Is(err, ErrPathNotMapped) {
		t.Fatalf("ResolveMediaPath() error=%v, want ErrPathNotMapped", err)
	}
	if result.PathMapping.OriginalPath != "/mnt/other/Media/fixture.mkv" ||
		result.PathMapping.EmbyPathPrefix != "/mnt/cloudNAS/115lifetime" ||
		result.PathMapping.SourceRootID != "0" || result.PathMapping.RelativePath != "" {
		t.Fatalf("ResolveMediaPath() mapping=%+v", result.PathMapping)
	}
}

func TestServiceResolveReusesPreexistingTargetWithoutLockOrUpload(t *testing.T) {
	provider := newFakeProvider()
	provider.searchResults = [][]p115integration.File{{provider.targetFile}}
	store := &fakeTaskStore{}
	locker := &fakeTaskLocker{}
	service := newServiceWithDependencies(fakeAccountLoader{}, provider, store, locker)

	result, err := service.Resolve(context.Background(), fixtureResolveRequest())
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if !result.Preexisting || result.TaskID != "" || result.URL != provider.download.URL {
		t.Fatalf("Resolve() result = %+v", result)
	}
	if locker.acquireCount != 0 || store.beginCount != 0 || store.touchCount != 1 {
		t.Fatalf("preexisting side effects: locker=%d begin=%d touch=%d", locker.acquireCount, store.beginCount, store.touchCount)
	}
	if containsCall(provider.calls, "init_upload") || containsCall(provider.calls, "hash_range") || containsCall(provider.calls, "find_target") {
		t.Fatalf("preexisting flow called upload path: %v", provider.calls)
	}
}

func TestServiceResolveRunsOneChallengeAndPersistsSucceededTask(t *testing.T) {
	provider := newFakeProvider()
	provider.searchResults = [][]p115integration.File{{}, {}}
	provider.uploadResults = []p115integration.RapidUploadResult{
		{
			Status: p115integration.RapidUploadRangeChallenge,
			Challenge: &p115integration.RapidUploadChallenge{
				Range:   p115integration.ByteRange{Start: 10, End: 19},
				SignKey: "fixture-sign-key",
			},
		},
		{Status: p115integration.RapidUploadReused},
	}
	store := &fakeTaskStore{}
	locker := &fakeTaskLocker{}
	service := newServiceWithDependencies(fakeAccountLoader{}, provider, store, locker)
	now := time.Date(2026, 8, 22, 6, 0, 0, 0, time.UTC)
	service.now = func() time.Time { return now }

	result, err := service.Resolve(context.Background(), fixtureResolveRequest())
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if result.Preexisting || result.TaskID != "task_1" || result.URL != provider.download.URL {
		t.Fatalf("Resolve() result = %+v", result)
	}
	if locker.acquireCount != 1 || locker.releaseCount != 1 || store.beginCount != 1 ||
		store.incrementCount != 1 || store.succeededTaskID != "task_1" {
		t.Fatalf("transfer persistence: locker=%d/%d begin=%d increments=%d succeeded=%q",
			locker.acquireCount, locker.releaseCount, store.beginCount, store.incrementCount, store.succeededTaskID)
	}
	wantStatuses := []models.PlaybackTransferTaskStatus{
		models.PlaybackTransferTaskStatusInitializing,
		models.PlaybackTransferTaskStatusChallenging,
		models.PlaybackTransferTaskStatusInitializing,
		models.PlaybackTransferTaskStatusVerifying,
	}
	if !reflect.DeepEqual(store.statuses, wantStatuses) {
		t.Fatalf("task statuses = %v, want %v", store.statuses, wantStatuses)
	}
	if len(provider.rangeRequests) != 2 || provider.rangeRequests[0].Range != (p115integration.ByteRange{Start: 0, End: 131071}) ||
		provider.rangeRequests[1].Range != (p115integration.ByteRange{Start: 10, End: 19}) {
		t.Fatalf("range requests = %+v", provider.rangeRequests)
	}
	if len(provider.uploadRequests) != 2 || provider.uploadRequests[0].PreID != directPlayPreID ||
		provider.uploadRequests[1].SignKey != "fixture-sign-key" || provider.uploadRequests[1].SignValue != directPlayChallengeSHA1 {
		t.Fatalf("upload requests = %+v", provider.uploadRequests)
	}
}

func TestServiceResolveMarksOrdinaryUploadAsFailed(t *testing.T) {
	provider := newFakeProvider()
	provider.searchResults = [][]p115integration.File{{}, {}}
	provider.uploadResults = []p115integration.RapidUploadResult{{Status: p115integration.RapidUploadOrdinaryUploadRequired}}
	store := &fakeTaskStore{}
	locker := &fakeTaskLocker{}
	service := newServiceWithDependencies(fakeAccountLoader{}, provider, store, locker)

	_, err := service.Resolve(context.Background(), fixtureResolveRequest())
	if !errors.Is(err, ErrRapidUploadUnavailable) {
		t.Fatalf("Resolve() error = %v, want ErrRapidUploadUnavailable", err)
	}
	if store.failedTaskID != "task_1" || store.failedCode != "ordinary_upload_required" || locker.releaseCount != 1 {
		t.Fatalf("failed task: id=%q code=%q releases=%d", store.failedTaskID, store.failedCode, locker.releaseCount)
	}
}

func TestServiceResolveRejectsOutOfBoundsChallenge(t *testing.T) {
	provider := newFakeProvider()
	provider.searchResults = [][]p115integration.File{{}, {}}
	provider.uploadResults = []p115integration.RapidUploadResult{{
		Status: p115integration.RapidUploadRangeChallenge,
		Challenge: &p115integration.RapidUploadChallenge{
			Range:   p115integration.ByteRange{Start: 0, End: provider.sourceFile.Size},
			SignKey: "fixture-sign-key",
		},
	}}
	store := &fakeTaskStore{}
	service := newServiceWithDependencies(fakeAccountLoader{}, provider, store, &fakeTaskLocker{})

	_, err := service.Resolve(context.Background(), fixtureResolveRequest())
	if !errors.Is(err, ErrProviderProtocol) {
		t.Fatalf("Resolve() error = %v, want ErrProviderProtocol", err)
	}
	if store.failedCode != "challenge_invalid" || len(provider.rangeRequests) != 1 {
		t.Fatalf("invalid challenge: failedCode=%q ranges=%+v", store.failedCode, provider.rangeRequests)
	}
}

func TestServiceResolveRejectsSameProviderAccountAndCookieDownloadMode(t *testing.T) {
	t.Run("same provider account", func(t *testing.T) {
		loader := fakeAccountLoader{sameProviderUser: true}
		provider := newFakeProvider()
		service := newServiceWithDependencies(loader, provider, &fakeTaskStore{}, &fakeTaskLocker{})
		if _, err := service.Resolve(context.Background(), fixtureResolveRequest()); !errors.Is(err, ErrAccountsSame) {
			t.Fatalf("Resolve() error = %v, want ErrAccountsSame", err)
		}
		if len(provider.calls) != 0 {
			t.Fatalf("same-account request reached provider: %v", provider.calls)
		}
	})

	t.Run("download requires cookie", func(t *testing.T) {
		provider := newFakeProvider()
		provider.searchResults = [][]p115integration.File{{provider.targetFile}}
		provider.download.HeaderMode = p115integration.DownloadHeadersSameUserAgentAndCookie
		store := &fakeTaskStore{}
		service := newServiceWithDependencies(fakeAccountLoader{}, provider, store, &fakeTaskLocker{})
		if _, err := service.Resolve(context.Background(), fixtureResolveRequest()); !errors.Is(err, ErrDownloadIncompatible) {
			t.Fatalf("Resolve() error = %v, want ErrDownloadIncompatible", err)
		}
		if store.touchCount != 0 {
			t.Fatalf("failed download refreshed lastAccessedAt: touches=%d", store.touchCount)
		}
	})
}

type fakeAccountLoader struct {
	sameProviderUser bool
}

func (loader fakeAccountLoader) LoadActiveCredentialByRole(_ context.Context, role models.P115AccountRole) (p115account.ActiveAccountCredential, error) {
	providerUserID := "provider-source"
	accountID := "source_account"
	targetParentID := ""
	if role == models.P115AccountRolePlayback {
		providerUserID = "provider-playback"
		accountID = "playback_account"
		targetParentID = "200000002"
	}
	if loader.sameProviderUser {
		providerUserID = "provider-same"
	}
	return p115account.ActiveAccountCredential{
		Role:           role,
		ProviderUserID: providerUserID,
		TargetParentID: targetParentID,
		EmbyPathPrefix: "/mnt/cloudNAS/115lifetime",
		SourceRootID:   "0",
		Credential: p115integration.Credential{
			AccountID: accountID,
			Cookie:    "fixture-cookie",
			AppType:   "ios",
			UserAgent: "fixture-agent",
		},
	}, nil
}

type fakeProvider struct {
	mu             sync.Mutex
	calls          []string
	searchResults  [][]p115integration.File
	uploadResults  []p115integration.RapidUploadResult
	uploadRequests []p115integration.RapidUploadRequest
	rangeRequests  []p115integration.FileRangeRequest
	resolvedQuery  p115integration.FilePathQuery
	sourceFile     p115integration.File
	targetFile     p115integration.File
	download       p115integration.DownloadURLResult
}

func newFakeProvider() *fakeProvider {
	return &fakeProvider{
		sourceFile: p115integration.File{
			ID: "source-file", PickCode: "source-pick", ParentID: "100", Name: "fixture.mkv",
			SHA1: directPlaySourceSHA1, Size: 1024 * 1024, IsDirectory: false,
		},
		targetFile: p115integration.File{
			ID: "target-file", PickCode: "target-pick", ParentID: "200000002", Name: "fixture.mkv",
			SHA1: directPlaySourceSHA1, Size: 1024 * 1024, IsDirectory: false,
		},
		download: p115integration.DownloadURLResult{
			URL: "https://cdn.115.com/fixture", ExpiresAt: time.Now().UTC().Add(time.Hour),
			HeaderMode: p115integration.DownloadHeadersSameUserAgent, ConcurrentOpenLimit: 2,
		},
	}
}

func (provider *fakeProvider) ResolveFileByPath(_ context.Context, _ p115integration.Credential, query p115integration.FilePathQuery) (*p115integration.File, error) {
	provider.mu.Lock()
	defer provider.mu.Unlock()
	provider.calls = append(provider.calls, "resolve_source")
	provider.resolvedQuery = query
	file := provider.sourceFile
	return &file, nil
}

func (provider *fakeProvider) SearchBySHA1(_ context.Context, _ p115integration.Credential, _ p115integration.FileQuery) ([]p115integration.File, error) {
	provider.mu.Lock()
	defer provider.mu.Unlock()
	provider.calls = append(provider.calls, "search_target")
	if len(provider.searchResults) == 0 {
		return []p115integration.File{}, nil
	}
	result := provider.searchResults[0]
	provider.searchResults = provider.searchResults[1:]
	return result, nil
}

func (provider *fakeProvider) HashFileRange(_ context.Context, _ p115integration.Credential, request p115integration.FileRangeRequest) (p115integration.FileRangeHash, error) {
	provider.mu.Lock()
	defer provider.mu.Unlock()
	provider.calls = append(provider.calls, "hash_range")
	provider.rangeRequests = append(provider.rangeRequests, request)
	sha1Value := directPlayPreID
	if len(provider.rangeRequests) > 1 {
		sha1Value = directPlayChallengeSHA1
	}
	return p115integration.FileRangeHash{SHA1: sha1Value, BytesRead: request.Range.End - request.Range.Start + 1}, nil
}

func (provider *fakeProvider) InitRapidUpload(_ context.Context, _ p115integration.Credential, request p115integration.RapidUploadRequest) (p115integration.RapidUploadResult, error) {
	provider.mu.Lock()
	defer provider.mu.Unlock()
	provider.calls = append(provider.calls, "init_upload")
	provider.uploadRequests = append(provider.uploadRequests, request)
	if len(provider.uploadResults) == 0 {
		return p115integration.RapidUploadResult{Status: p115integration.RapidUploadReused}, nil
	}
	result := provider.uploadResults[0]
	provider.uploadResults = provider.uploadResults[1:]
	return result, nil
}

func (provider *fakeProvider) FindTargetFile(_ context.Context, _ p115integration.Credential, _ p115integration.FileQuery) (*p115integration.File, error) {
	provider.record("find_target")
	file := provider.targetFile
	return &file, nil
}

func (provider *fakeProvider) GetDownloadURL(_ context.Context, _ p115integration.Credential, _ p115integration.DownloadURLRequest) (p115integration.DownloadURLResult, error) {
	provider.record("download")
	return provider.download, nil
}

func (provider *fakeProvider) record(call string) {
	provider.mu.Lock()
	defer provider.mu.Unlock()
	provider.calls = append(provider.calls, call)
}

type fakeTaskStore struct {
	beginCount      int
	incrementCount  int
	touchCount      int
	statuses        []models.PlaybackTransferTaskStatus
	succeededTaskID string
	failedTaskID    string
	failedCode      string
}

func (store *fakeTaskStore) BeginAttempt(_ context.Context, input beginAttemptInput) (*models.PlaybackTransferTask, error) {
	store.beginCount++
	return &models.PlaybackTransferTask{
		ID: "task_1", SourceAccountID: input.SourceAccountID, PlaybackAccountID: input.PlaybackAccountID,
		SHA1: input.SHA1, Size: input.Size, FileName: input.FileName, TargetParentID: input.TargetParentID,
		Status: models.PlaybackTransferTaskStatusPending, AttemptCount: 1, StartedAt: input.StartedAt,
	}, nil
}

func (store *fakeTaskStore) MarkStatus(_ context.Context, _ string, status models.PlaybackTransferTaskStatus, _ time.Time) error {
	store.statuses = append(store.statuses, status)
	return nil
}

func (store *fakeTaskStore) IncrementAttempt(_ context.Context, _ string, _ time.Time) error {
	store.incrementCount++
	return nil
}

func (store *fakeTaskStore) MarkSucceeded(_ context.Context, taskID string, _ p115integration.File, _ time.Time) error {
	store.succeededTaskID = taskID
	return nil
}

func (store *fakeTaskStore) MarkFailed(_ context.Context, taskID, code, _ string, _ time.Time) error {
	store.failedTaskID = taskID
	store.failedCode = code
	return nil
}

func (store *fakeTaskStore) TouchSucceeded(_ context.Context, _ string, _ string, _ int64, _ time.Time) error {
	store.touchCount++
	return nil
}

type fakeTaskLocker struct {
	acquireCount int
	releaseCount int
}

func (locker *fakeTaskLocker) Acquire(_ context.Context, _ string, _ string, _ int64) (taskLock, error) {
	locker.acquireCount++
	return fakeTaskLock{release: func() { locker.releaseCount++ }}, nil
}

type fakeTaskLock struct {
	release func()
}

func (lock fakeTaskLock) Release() error {
	lock.release()
	return nil
}

func fixtureResolveRequest() ResolveRequest {
	return ResolveRequest{
		SourceFile: p115integration.FilePathQuery{
			RootID: "0", RelativePath: "Media/fixture.mkv", Size: 1024 * 1024,
		},
		ClientUserAgent: "Infuse-Fixture",
	}
}

func fixtureMediaPathResolveRequest() MediaPathResolveRequest {
	return MediaPathResolveRequest{
		Path:            "/mnt/cloudNAS/115lifetime/Media/fixture.mkv",
		Size:            1024 * 1024,
		ClientUserAgent: "Infuse-Fixture",
	}
}

func containsCall(calls []string, want string) bool {
	for _, call := range calls {
		if call == want {
			return true
		}
	}
	return false
}
