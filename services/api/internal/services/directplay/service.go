package directplay

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	p115integration "github.com/konghang/ember/backend/internal/integrations/p115"
	"github.com/konghang/ember/backend/internal/models"
	"github.com/konghang/ember/backend/internal/services/p115account"
	"github.com/konghang/ember/backend/internal/services/p115quota"
	"gorm.io/gorm"
)

const (
	directPlayPreIDBytes         = int64(128 * 1024)
	maxDirectPlayClientUserAgent = 1024
	maxDirectPlayMediaPath       = 16 * 1024
	maxMappedRelativePath        = 4 * 1024
	maxMappedPathSegment         = 1024
	taskTerminalWriteTimeout     = 5 * time.Second
	accountHealthWriteTimeout    = 2 * time.Second
	reservationReleaseTimeout    = time.Second
	transferCommitTimeout        = 2 * time.Second
	transferCommitRetryInterval  = 50 * time.Millisecond
)

// ResolveRequest contains the already mapped source path and the actual
// playback client's User-Agent. It never carries a signed URL or Cookie.
type ResolveRequest struct {
	SourceFile      p115integration.FilePathQuery
	ClientUserAgent string
}

// MediaPathResolveRequest is the future gateway input before source-account
// path mapping has produced a Provider FilePathQuery.
type MediaPathResolveRequest struct {
	Path            string
	ClientUserAgent string
	Method          string
	UserID          string
	MappingID       string
	DeviceID        string
	PlaySessionID   string
}

// MediaPathMapping keeps the exact Emby path and the source-account mapping
// selected for one Gateway request. It is runtime diagnostic provenance only:
// callers may log it, but it is never serialized or persisted by DirectPlay.
type MediaPathMapping struct {
	OriginalPath   string
	EmbyPathPrefix string
	SourceRootID   string
	RelativePath   string
}

// RedirectCandidate is the internal handoff to the future playback gateway.
// URL is deliberately excluded from JSON and persistent task state.
type RedirectCandidate struct {
	URL                 string                             `json:"-"`
	ExpiresAt           time.Time                          `json:"expiresAt"`
	HeaderMode          p115integration.DownloadHeaderMode `json:"headerMode"`
	ConcurrentOpenLimit int64                              `json:"concurrentOpenLimit"`
	TaskID              string                             `json:"taskId,omitempty"`
	Preexisting         bool                               `json:"preexisting"`
	PathMapping         MediaPathMapping                   `json:"-"`
	Routing             RoutingDiagnostics                 `json:"-"`
}

// RoutingDiagnostics contains only fixed modes and aggregate counts used by
// the Gateway's single final decision log.
type RoutingDiagnostics struct {
	Routed                         bool
	PlaybackMode                   models.P115PlaybackMode
	PlaybackAccountOwner           string
	AccountLimitsAvailable         bool
	ConfiguredMaxConcurrentStreams int
	EffectiveMaxConcurrentStreams  int
	SimultaneousStreamLimit        int
	LeaseUsageAvailable            bool
	AccountUsage                   p115quota.LeaseUsage
	UserUsage                      p115quota.LeaseUsage
	TransferChecked                bool
	TransferUsageAvailable         bool
	TransferUsage                  p115quota.TransferUsage
	TransferHourlyLimit            int
	TransferDailyLimit             int
}

type activeAccountLoader interface {
	LoadActiveCredentialByRole(ctx context.Context, role models.P115AccountRole) (p115account.ActiveAccountCredential, error)
}

type accountHealthReporter interface {
	ReportRuntimeHealth(ctx context.Context, account p115account.ActiveAccountCredential, outcome p115account.RuntimeHealthOutcome) error
}

type accountRuntime interface {
	activeAccountLoader
	accountHealthReporter
	LoadEnabledSourceLocation(ctx context.Context) (p115account.SourceLocation, error)
}

type playbackAccountRouter interface {
	ResolvePlaybackRoute(context.Context, string, time.Time) (p115account.PlaybackRoute, error)
	AcquirePlaybackRoute(context.Context, p115account.PlaybackRoute) (p115account.ActiveAccountCredential, error)
}

type leaseKeyDeriver interface {
	PlaybackAccountKey(string) (string, error)
	SessionFingerprint(p115quota.SessionIdentity) (string, error)
}

// TransferProvider intentionally omits DeleteFile so the phase-one service
// cannot delete retained playback files.
type TransferProvider interface {
	ResolveFileByPath(ctx context.Context, credential p115integration.Credential, query p115integration.FilePathQuery) (*p115integration.File, error)
	SearchBySHA1(ctx context.Context, credential p115integration.Credential, query p115integration.FileQuery) ([]p115integration.File, error)
	HashFileRange(ctx context.Context, credential p115integration.Credential, request p115integration.FileRangeRequest) (p115integration.FileRangeHash, error)
	InitRapidUpload(ctx context.Context, credential p115integration.Credential, request p115integration.RapidUploadRequest) (p115integration.RapidUploadResult, error)
	FindTargetFile(ctx context.Context, credential p115integration.Credential, query p115integration.FileQuery) (*p115integration.File, error)
	GetDownloadURL(ctx context.Context, credential p115integration.Credential, request p115integration.DownloadURLRequest) (p115integration.DownloadURLResult, error)
}

type beginAttemptInput struct {
	SourceAccountID   string
	PlaybackAccountID string
	SHA1              string
	Size              int64
	FileName          string
	TargetParentID    string
	StartedAt         time.Time
}

type taskStore interface {
	BeginAttempt(ctx context.Context, input beginAttemptInput) (*models.PlaybackTransferTask, error)
	MarkStatus(ctx context.Context, taskID string, status models.PlaybackTransferTaskStatus, at time.Time) error
	IncrementAttempt(ctx context.Context, taskID string, at time.Time) error
	MarkSucceeded(ctx context.Context, taskID string, target p115integration.File, at time.Time) error
	MarkFailed(ctx context.Context, taskID, code, message string, at time.Time) error
	TouchSucceeded(ctx context.Context, playbackAccountID, sha1 string, size int64, at time.Time) error
}

type taskLocker interface {
	Acquire(ctx context.Context, playbackAccountID, sha1 string, size int64) (taskLock, error)
}

type taskLock interface {
	Release() error
}

type transferQuotaContext struct {
	UserID         string
	HourlyLimit    int
	DailyLimit     int
	Checked        bool
	UsageAvailable bool
	Usage          p115quota.TransferUsage
}

// Service serializes retained playback transfers and returns a validated 115
// redirect candidate without exposing an HTTP endpoint.
type Service struct {
	accounts              accountRuntime
	playbackRouter        playbackAccountRouter
	provider              TransferProvider
	store                 taskStore
	locker                taskLocker
	leases                p115quota.LeaseStore
	transferQuotas        p115quota.TransferQuotaStore
	keyDeriver            leaseKeyDeriver
	serverID              string
	businessTimezone      *time.Location
	transferCommitBudget  time.Duration
	transferRetryInterval time.Duration
	now                   func() time.Time
}

// NewService builds the production transfer service using PostgreSQL task
// persistence and session-scoped advisory locks.
func NewService(database *gorm.DB, accounts *p115account.Service, provider TransferProvider) (*Service, error) {
	if database == nil || accounts == nil || provider == nil {
		return nil, ErrStoreUnavailable
	}
	locker, err := newPostgresTaskLocker(database)
	if err != nil {
		return nil, err
	}
	return newServiceWithDependencies(accounts, provider, &gormTaskStore{db: database}, locker), nil
}

// newServiceWithDependencies keeps unit tests on fake accounts, Provider,
// task persistence, and locks without weakening the production constructor.
func newServiceWithDependencies(accounts accountRuntime, provider TransferProvider, store taskStore, locker taskLocker) *Service {
	return &Service{accounts: accounts, provider: provider, store: store, locker: locker, now: time.Now}
}

// NewRoutedService adds personal/system account routing and Redis admission to
// the production media-path entrypoint while preserving the lower transfer state machine.
func NewRoutedService(
	database *gorm.DB,
	accounts *p115account.Service,
	provider TransferProvider,
	leases p115quota.LeaseStore,
	keyDeriver *p115quota.KeyDeriver,
	serverID string,
	businessTimezone *time.Location,
) (*Service, error) {
	if database == nil || accounts == nil || provider == nil || leases == nil || keyDeriver == nil || strings.TrimSpace(serverID) == "" || businessTimezone == nil {
		return nil, ErrStoreUnavailable
	}
	locker, err := newPostgresTaskLocker(database)
	if err != nil {
		return nil, err
	}
	return newRoutedServiceWithDependencies(
		accounts, accounts, provider, &gormTaskStore{db: database}, locker, leases, keyDeriver, serverID, businessTimezone,
	)
}

func newRoutedServiceWithDependencies(
	accounts accountRuntime,
	playbackRouter playbackAccountRouter,
	provider TransferProvider,
	store taskStore,
	locker taskLocker,
	leases p115quota.LeaseStore,
	keyDeriver leaseKeyDeriver,
	serverID string,
	businessTimezone *time.Location,
) (*Service, error) {
	transferQuotas, ok := leases.(p115quota.TransferQuotaStore)
	if accounts == nil || playbackRouter == nil || provider == nil || store == nil || locker == nil || leases == nil ||
		!ok || keyDeriver == nil || strings.TrimSpace(serverID) == "" || businessTimezone == nil {
		return nil, ErrStoreUnavailable
	}
	return &Service{
		accounts: accounts, playbackRouter: playbackRouter, provider: provider, store: store, locker: locker,
		leases: leases, transferQuotas: transferQuotas, keyDeriver: keyDeriver, serverID: serverID,
		businessTimezone: businessTimezone, transferCommitBudget: transferCommitTimeout,
		transferRetryInterval: transferCommitRetryInterval, now: time.Now,
	}, nil
}

// Resolve returns a fresh playback-account download URL, creating and
// retaining the target file exactly once when it is absent.
func (service *Service) Resolve(ctx context.Context, request ResolveRequest) (RedirectCandidate, error) {
	if err := validateResolveRequest(request); err != nil {
		return RedirectCandidate{}, err
	}
	source, playback, err := service.loadAccounts(ctx)
	if err != nil {
		return RedirectCandidate{}, err
	}
	return service.resolveWithAccounts(ctx, source, playback, request, nil)
}

// ResolveMediaPath maps an Emby media path through the active source account
// before entering the already tested transfer orchestration.
func (service *Service) ResolveMediaPath(ctx context.Context, request MediaPathResolveRequest) (RedirectCandidate, error) {
	mapping := MediaPathMapping{OriginalPath: request.Path}
	if !validAbsoluteMediaPath(request.Path, maxDirectPlayMediaPath) {
		return RedirectCandidate{PathMapping: mapping}, ErrPathNotMapped
	}
	location, err := service.accounts.LoadEnabledSourceLocation(ctx)
	if err != nil {
		return RedirectCandidate{PathMapping: mapping}, withFailureContext(
			ErrAccountUnavailable,
			FailureContext{AccountRole: string(models.P115AccountRoleSource)},
		)
	}
	mapping.EmbyPathPrefix = location.EmbyPathPrefix
	mapping.SourceRootID = location.SourceRootID
	fileQuery, err := mapMediaPath(location.EmbyPathPrefix, location.SourceRootID, request.Path)
	if err != nil {
		return RedirectCandidate{PathMapping: mapping}, err
	}
	mapping.RelativePath = fileQuery.RelativePath
	if !validClientUserAgent(request.ClientUserAgent) {
		return RedirectCandidate{PathMapping: mapping}, ErrInvalidRequest
	}
	if service.playbackRouter != nil {
		candidate, err := service.resolveRoutedMediaPath(ctx, request, fileQuery, location)
		candidate.PathMapping = mapping
		return candidate, err
	}
	source, playback, err := service.loadAccounts(ctx)
	if err != nil {
		return RedirectCandidate{PathMapping: mapping}, err
	}
	if source.Credential.AccountID != location.AccountID || source.EmbyPathPrefix != location.EmbyPathPrefix ||
		source.SourceRootID != location.SourceRootID {
		return RedirectCandidate{PathMapping: mapping}, withFailureContext(
			ErrAccountUnavailable,
			FailureContext{AccountRole: string(models.P115AccountRoleSource)},
		)
	}
	candidate, err := service.resolveWithAccounts(ctx, source, playback, ResolveRequest{
		SourceFile: fileQuery, ClientUserAgent: request.ClientUserAgent,
	}, nil)
	candidate.PathMapping = mapping
	return candidate, err
}

// PlaybackSessionEvent is the authenticated, bounded event identity forwarded
// by Gateway only after Emby accepted Playing/Progress/Stopped.
type PlaybackSessionEvent struct {
	UserID        string
	MappingID     string
	DeviceID      string
	PlaySessionID string
	IsProgress    bool
	IsPaused      bool
	Stopped       bool
}

// PlaybackSessionEventResult exposes only bounded lease observations needed by
// Gateway diagnostics; it contains no raw Provider or session identifiers.
type PlaybackSessionEventResult struct {
	Found   bool
	State   p115quota.LeaseState
	Account p115quota.LeaseUsage
	User    p115quota.LeaseUsage
}

// HandlePlaybackSessionEvent advances or releases only an existing reverse
// session; ordinary Emby/local playback events cannot create a 115 lease.
func (service *Service) HandlePlaybackSessionEvent(ctx context.Context, event PlaybackSessionEvent) (PlaybackSessionEventResult, error) {
	if service.leases == nil || service.keyDeriver == nil || strings.TrimSpace(service.serverID) == "" {
		return PlaybackSessionEventResult{}, ErrRedisUnavailable
	}
	fingerprint, err := service.keyDeriver.SessionFingerprint(p115quota.SessionIdentity{
		ServerID: service.serverID, UserID: event.UserID, MappingID: event.MappingID,
		DeviceID: event.DeviceID, PlaySessionID: event.PlaySessionID,
	})
	if err != nil {
		return PlaybackSessionEventResult{}, ErrInvalidRequest
	}
	now := service.now().UTC()
	var result p115quota.TransitionResult
	if event.Stopped {
		result, err = service.leases.Stop(ctx, fingerprint, now)
	} else {
		state := p115quota.LeaseStateActive
		if event.IsProgress && event.IsPaused {
			state = p115quota.LeaseStatePaused
		}
		result, err = service.leases.Advance(ctx, fingerprint, state, now)
	}
	if err != nil {
		return PlaybackSessionEventResult{}, mapLeaseError(err)
	}
	return PlaybackSessionEventResult{Found: result.Found, State: result.State, Account: result.Account, User: result.User}, nil
}

func (service *Service) resolveRoutedMediaPath(
	ctx context.Context,
	request MediaPathResolveRequest,
	fileQuery p115integration.FilePathQuery,
	location p115account.SourceLocation,
) (RedirectCandidate, error) {
	if request.Method != http.MethodGet && request.Method != http.MethodHead {
		return RedirectCandidate{}, ErrInvalidRequest
	}
	now := service.now().UTC()
	fingerprint, err := service.keyDeriver.SessionFingerprint(p115quota.SessionIdentity{
		ServerID: service.serverID, UserID: request.UserID, MappingID: request.MappingID,
		DeviceID: request.DeviceID, PlaySessionID: request.PlaySessionID,
	})
	if err != nil {
		return RedirectCandidate{}, ErrInvalidRequest
	}
	route, err := service.playbackRouter.ResolvePlaybackRoute(ctx, request.UserID, now)
	diagnostics := routingDiagnosticsFromRoute(route)
	if err != nil {
		return RedirectCandidate{Routing: diagnostics}, mapPlaybackRouteError(err)
	}
	accountKey, err := service.keyDeriver.PlaybackAccountKey(route.ProviderUserID)
	if err != nil {
		return RedirectCandidate{Routing: diagnostics}, withFailureContext(ErrAccountUnavailable, FailureContext{AccountRole: string(models.P115AccountRolePlayback)})
	}

	createdReservation := false
	if request.Method == http.MethodHead {
		session, found, leaseErr := service.leases.Session(ctx, fingerprint, now)
		if leaseErr != nil {
			return RedirectCandidate{Routing: diagnostics}, mapLeaseError(leaseErr)
		}
		if !found {
			return RedirectCandidate{Routing: diagnostics}, ErrHeadLeaseMissing
		}
		if session.PlaybackAccountKey != accountKey || session.UserID != request.UserID {
			return RedirectCandidate{Routing: diagnostics}, ErrPlaybackRouteChanged
		}
		accountUsage, leaseErr := service.leases.AccountUsage(ctx, accountKey, now)
		if leaseErr != nil {
			return RedirectCandidate{Routing: diagnostics}, mapLeaseError(leaseErr)
		}
		userUsage, leaseErr := service.leases.UserUsage(ctx, request.UserID, now)
		if leaseErr != nil {
			return RedirectCandidate{Routing: diagnostics}, mapLeaseError(leaseErr)
		}
		diagnostics.LeaseUsageAvailable = true
		diagnostics.AccountUsage = accountUsage
		diagnostics.UserUsage = userUsage
	} else {
		admission, leaseErr := service.leases.Reserve(ctx, p115quota.ReserveRequest{
			PlaybackAccountKey: accountKey, UserID: request.UserID, SessionFingerprint: fingerprint,
			MaxConcurrentStreams: route.EffectiveMaxConcurrentStreams,
		}, now)
		if leaseErr != nil {
			if errors.Is(leaseErr, p115quota.ErrAccountConcurrencyExceeded) {
				diagnostics.LeaseUsageAvailable = true
				diagnostics.AccountUsage = admission.Account
				diagnostics.UserUsage = admission.User
			}
			return RedirectCandidate{Routing: diagnostics}, mapLeaseError(leaseErr)
		}
		if admission.PlaybackAccountKey != accountKey || admission.UserID != request.UserID {
			return RedirectCandidate{Routing: diagnostics}, ErrPlaybackRouteChanged
		}
		createdReservation = admission.Created
		diagnostics.LeaseUsageAvailable = true
		diagnostics.AccountUsage = admission.Account
		diagnostics.UserUsage = admission.User
	}

	source, playback, err := service.loadRoutedAccounts(ctx, route, location)
	if err != nil {
		service.releaseNewReservation(ctx, fingerprint, createdReservation)
		return RedirectCandidate{Routing: diagnostics}, err
	}
	quotaContext := &transferQuotaContext{UserID: request.UserID, HourlyLimit: route.TransferHourlyLimit, DailyLimit: route.TransferDailyLimit}
	candidate, err := service.resolveWithAccounts(
		ctx, source, playback,
		ResolveRequest{SourceFile: fileQuery, ClientUserAgent: request.ClientUserAgent},
		quotaContext,
	)
	diagnostics.TransferChecked = quotaContext.Checked
	diagnostics.TransferUsageAvailable = quotaContext.UsageAvailable
	diagnostics.TransferUsage = quotaContext.Usage
	candidate.Routing = diagnostics
	if err != nil {
		service.releaseNewReservation(ctx, fingerprint, createdReservation)
	}
	return candidate, err
}

// routingDiagnosticsFromRoute converts only policy labels, aggregate limits
// and availability markers; account identity and Provider UID stay private.
func routingDiagnosticsFromRoute(route p115account.PlaybackRoute) RoutingDiagnostics {
	diagnostics := RoutingDiagnostics{
		PlaybackMode: route.PlaybackMode, SimultaneousStreamLimit: route.SimultaneousStreamLimit,
		TransferHourlyLimit: route.TransferHourlyLimit, TransferDailyLimit: route.TransferDailyLimit,
	}
	switch route.PlaybackMode {
	case models.P115PlaybackModePersonal:
		diagnostics.Routed = true
		diagnostics.PlaybackAccountOwner = "current_user"
	case models.P115PlaybackModeSystem:
		diagnostics.Routed = true
		diagnostics.PlaybackAccountOwner = "shared"
	}
	if route.AccountID != "" {
		diagnostics.AccountLimitsAvailable = true
		diagnostics.ConfiguredMaxConcurrentStreams = route.ConfiguredMaxConcurrentStreams
		diagnostics.EffectiveMaxConcurrentStreams = route.EffectiveMaxConcurrentStreams
	}
	return diagnostics
}

func (service *Service) loadRoutedAccounts(
	ctx context.Context,
	route p115account.PlaybackRoute,
	location p115account.SourceLocation,
) (p115account.ActiveAccountCredential, p115account.ActiveAccountCredential, error) {
	source, err := service.accounts.LoadActiveCredentialByRole(ctx, models.P115AccountRoleSource)
	if err != nil {
		return p115account.ActiveAccountCredential{}, p115account.ActiveAccountCredential{}, withFailureContext(
			ErrAccountUnavailable, FailureContext{AccountRole: string(models.P115AccountRoleSource)},
		)
	}
	playback, err := service.playbackRouter.AcquirePlaybackRoute(ctx, route)
	if err != nil {
		return p115account.ActiveAccountCredential{}, p115account.ActiveAccountCredential{}, withFailureContext(
			ErrAccountUnavailable, FailureContext{AccountRole: string(models.P115AccountRolePlayback)},
		)
	}
	if source.Credential.AccountID != location.AccountID || source.EmbyPathPrefix != location.EmbyPathPrefix ||
		source.SourceRootID != location.SourceRootID {
		return p115account.ActiveAccountCredential{}, p115account.ActiveAccountCredential{}, withFailureContext(
			ErrAccountUnavailable, FailureContext{AccountRole: string(models.P115AccountRoleSource)},
		)
	}
	if source.ProviderUserID == playback.ProviderUserID || source.Credential.AccountID == playback.Credential.AccountID {
		return p115account.ActiveAccountCredential{}, p115account.ActiveAccountCredential{}, ErrAccountsSame
	}
	return source, playback, nil
}

func (service *Service) releaseNewReservation(ctx context.Context, fingerprint string, created bool) {
	if !created {
		return
	}
	releaseContext, cancel := context.WithTimeout(context.WithoutCancel(ctx), reservationReleaseTimeout)
	defer cancel()
	_, _ = service.leases.ReleaseReservation(releaseContext, fingerprint, service.now().UTC())
}

func mapPlaybackRouteError(err error) error {
	if errors.Is(err, p115account.ErrPersonalAccountMissing) {
		return ErrPersonalAccountMissing
	}
	return withFailureContext(ErrAccountUnavailable, FailureContext{AccountRole: string(models.P115AccountRolePlayback)})
}

func mapLeaseError(err error) error {
	if errors.Is(err, p115quota.ErrAccountConcurrencyExceeded) {
		return ErrAccountConcurrencyExceeded
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	return ErrRedisUnavailable
}

// resolveWithAccounts shares the transfer state machine between callers that
// already have a FilePathQuery and the gateway-facing media-path entrypoint.
func (service *Service) resolveWithAccounts(
	ctx context.Context,
	source, playback p115account.ActiveAccountCredential,
	request ResolveRequest,
	quota *transferQuotaContext,
) (RedirectCandidate, error) {
	sourceFile, err := service.provider.ResolveFileByPath(ctx, source.Credential, request.SourceFile)
	if err != nil {
		return RedirectCandidate{}, service.reportProviderFailure(source, failureOperationResolveSourcePath, err)
	}
	sha1Value, err := validateSourceFile(sourceFile)
	if err != nil {
		service.reportRuntimeHealth(source, p115account.RuntimeHealthProviderProtocol)
		return RedirectCandidate{}, err
	}
	query := p115integration.FileQuery{SHA1: sha1Value, Size: sourceFile.Size, ParentID: playback.TargetParentID}

	target, found, err := service.searchTarget(ctx, playback, query)
	if err != nil {
		return RedirectCandidate{}, err
	}
	if found {
		candidate, err := service.downloadCandidate(ctx, playback, *target, request.ClientUserAgent, "", true)
		if err != nil {
			return RedirectCandidate{}, err
		}
		if err := service.touchSucceeded(ctx, playback.Credential.AccountID, sha1Value, sourceFile.Size); err != nil {
			return RedirectCandidate{}, err
		}
		service.reportRuntimeSuccess(source, playback)
		return candidate, nil
	}

	lock, err := service.locker.Acquire(ctx, playback.Credential.AccountID, sha1Value, sourceFile.Size)
	if err != nil {
		return RedirectCandidate{}, fmt.Errorf("%w: acquire", ErrLockUnavailable)
	}
	released := false
	defer func() {
		if !released {
			if releaseErr := lock.Release(); releaseErr != nil {
				log.Printf("[DirectPlay] 释放 transfer lock 失败 playbackAccountId=%s errorType=%T", playback.Credential.AccountID, releaseErr)
			}
		}
	}()

	lockedTarget, taskID, preexisting, err := service.resolveUnderLock(ctx, source, playback, *sourceFile, query, quota)
	if err != nil {
		return RedirectCandidate{}, err
	}
	if err := lock.Release(); err != nil {
		return RedirectCandidate{}, fmt.Errorf("%w: release", ErrLockUnavailable)
	}
	released = true
	candidate, err := service.downloadCandidate(ctx, playback, lockedTarget, request.ClientUserAgent, taskID, preexisting)
	if err != nil {
		return RedirectCandidate{}, err
	}
	if preexisting {
		if err := service.touchSucceeded(ctx, playback.Credential.AccountID, sha1Value, sourceFile.Size); err != nil {
			return RedirectCandidate{}, err
		}
	}
	service.reportRuntimeSuccess(source, playback)
	return candidate, nil
}

// loadAccounts resolves the unique active source/playback pair and rejects an
// accidental same-account configuration before any Provider data call.
func (service *Service) loadAccounts(ctx context.Context) (p115account.ActiveAccountCredential, p115account.ActiveAccountCredential, error) {
	source, err := service.accounts.LoadActiveCredentialByRole(ctx, models.P115AccountRoleSource)
	if err != nil {
		return p115account.ActiveAccountCredential{}, p115account.ActiveAccountCredential{}, withFailureContext(
			ErrAccountUnavailable,
			FailureContext{AccountRole: string(models.P115AccountRoleSource)},
		)
	}
	playback, err := service.accounts.LoadActiveCredentialByRole(ctx, models.P115AccountRolePlayback)
	if err != nil {
		return p115account.ActiveAccountCredential{}, p115account.ActiveAccountCredential{}, withFailureContext(
			ErrAccountUnavailable,
			FailureContext{AccountRole: string(models.P115AccountRolePlayback)},
		)
	}
	if source.ProviderUserID == playback.ProviderUserID || source.Credential.AccountID == playback.Credential.AccountID {
		return p115account.ActiveAccountCredential{}, p115account.ActiveAccountCredential{}, ErrAccountsSame
	}
	if strings.TrimSpace(playback.TargetParentID) == "" {
		return p115account.ActiveAccountCredential{}, p115account.ActiveAccountCredential{}, withFailureContext(
			ErrAccountUnavailable,
			FailureContext{AccountRole: string(models.P115AccountRolePlayback)},
		)
	}
	return source, playback, nil
}

// resolveUnderLock repeats target lookup under the content lock and owns the
// complete task lifecycle only when the target is still absent.
func (service *Service) resolveUnderLock(
	ctx context.Context,
	source, playback p115account.ActiveAccountCredential,
	sourceFile p115integration.File,
	query p115integration.FileQuery,
	quota *transferQuotaContext,
) (p115integration.File, string, bool, error) {
	target, found, err := service.searchTarget(ctx, playback, query)
	if err != nil {
		return p115integration.File{}, "", false, err
	}
	if found {
		return *target, "", true, nil
	}

	now := service.now().UTC()
	transferAttemptID := ""
	releasePending := false
	if quota != nil {
		var err error
		transferAttemptID, err = newTransferAttemptID()
		if err != nil {
			return p115integration.File{}, "", false, ErrStoreUnavailable
		}
		dayStart, dayEnd := p115quota.DayWindow(now, service.businessTimezone)
		reservation, err := service.transferQuotas.ReserveTransfer(ctx, p115quota.TransferReserveRequest{
			UserID: quota.UserID, AttemptID: transferAttemptID,
			HourlyLimit: quota.HourlyLimit, DailyLimit: quota.DailyLimit, DayStart: dayStart, DayEnd: dayEnd,
		}, now)
		quota.Checked = true
		quota.Usage = reservation.Usage
		quota.UsageAvailable = err == nil || errors.Is(err, p115quota.ErrTransferQuotaExceeded)
		if err != nil {
			return p115integration.File{}, "", false, mapTransferQuotaError(err)
		}
		if !reservation.Created {
			return p115integration.File{}, "", false, ErrRedisUnavailable
		}
		releasePending = true
		defer func() {
			if releasePending {
				service.releaseTransferReservation(quota.UserID, transferAttemptID)
			}
		}()
	}
	task, err := service.store.BeginAttempt(ctx, beginAttemptInput{
		SourceAccountID: source.Credential.AccountID, PlaybackAccountID: playback.Credential.AccountID,
		SHA1: query.SHA1, Size: query.Size, FileName: sourceFile.Name,
		TargetParentID: playback.TargetParentID, StartedAt: now,
	})
	if err != nil {
		return p115integration.File{}, "", false, fmt.Errorf("%w: begin", ErrStoreUnavailable)
	}
	log.Printf("[DirectPlay] transfer task 开始 taskId=%s sourceAccountId=%s playbackAccountId=%s size=%d",
		task.ID, source.Credential.AccountID, playback.Credential.AccountID, sourceFile.Size)

	if err := service.markStatus(ctx, task.ID, models.PlaybackTransferTaskStatusInitializing); err != nil {
		return p115integration.File{}, "", false, err
	}
	preIDRange := boundedRange(sourceFile.Size)
	preIDHash, err := service.provider.HashFileRange(ctx, source.Credential, p115integration.FileRangeRequest{
		File: sourceFile, Range: preIDRange,
	})
	if err != nil {
		return service.failTask(ctx, task.ID, "preid_failed", "source preID range failed", service.reportProviderFailure(source, failureOperationHashSourcePreID, err))
	}
	preID, err := validateRangeHash(preIDHash, preIDRange)
	if err != nil {
		service.reportRuntimeHealth(source, p115account.RuntimeHealthProviderProtocol)
		return service.failTask(ctx, task.ID, "preid_invalid", "source preID range invalid", err)
	}

	uploadRequest := p115integration.RapidUploadRequest{
		FileName: sourceFile.Name, SHA1: query.SHA1, Size: query.Size,
		TargetParentID: playback.TargetParentID, PreID: preID,
	}
	result, err := service.provider.InitRapidUpload(ctx, playback.Credential, uploadRequest)
	if err != nil {
		return service.failTask(ctx, task.ID, providerFailureCode(err), "rapid upload initialization failed", service.reportProviderFailure(playback, failureOperationRapidUpload, err))
	}
	if result.Status == p115integration.RapidUploadRangeChallenge {
		if !validChallenge(result.Challenge, sourceFile.Size) {
			service.reportRuntimeHealth(playback, p115account.RuntimeHealthProviderProtocol)
			return service.failTask(ctx, task.ID, "challenge_invalid", "rapid upload challenge invalid", ErrProviderProtocol)
		}
		if err := service.markStatus(ctx, task.ID, models.PlaybackTransferTaskStatusChallenging); err != nil {
			return p115integration.File{}, "", false, err
		}
		challengeHash, hashErr := service.provider.HashFileRange(ctx, source.Credential, p115integration.FileRangeRequest{
			File: sourceFile, Range: result.Challenge.Range,
		})
		if hashErr != nil {
			return service.failTask(ctx, task.ID, "challenge_failed", "rapid upload challenge range failed", service.reportProviderFailure(source, failureOperationHashSourceChallenge, hashErr))
		}
		signValue, hashErr := validateRangeHash(challengeHash, result.Challenge.Range)
		if hashErr != nil {
			service.reportRuntimeHealth(source, p115account.RuntimeHealthProviderProtocol)
			return service.failTask(ctx, task.ID, "challenge_invalid", "rapid upload challenge range invalid", hashErr)
		}
		uploadRequest.SignKey = result.Challenge.SignKey
		uploadRequest.SignValue = signValue
		if err := service.store.IncrementAttempt(ctx, task.ID, service.now().UTC()); err != nil {
			return p115integration.File{}, "", false, fmt.Errorf("%w: increment_attempt", ErrStoreUnavailable)
		}
		if err := service.markStatus(ctx, task.ID, models.PlaybackTransferTaskStatusInitializing); err != nil {
			return p115integration.File{}, "", false, err
		}
		result, err = service.provider.InitRapidUpload(ctx, playback.Credential, uploadRequest)
		if err != nil {
			return service.failTask(ctx, task.ID, providerFailureCode(err), "rapid upload retry failed", service.reportProviderFailure(playback, failureOperationRapidUploadRetry, err))
		}
		if result.Status == p115integration.RapidUploadRangeChallenge {
			service.reportRuntimeHealth(playback, p115account.RuntimeHealthProviderProtocol)
			return service.failTask(ctx, task.ID, "repeated_challenge", "rapid upload repeated challenge", ErrProviderProtocol)
		}
	}

	switch result.Status {
	case p115integration.RapidUploadReused:
	case p115integration.RapidUploadOrdinaryUploadRequired:
		return service.failTask(ctx, task.ID, "ordinary_upload_required", "ordinary upload is disabled", ErrRapidUploadUnavailable)
	default:
		service.reportRuntimeHealth(playback, p115account.RuntimeHealthProviderProtocol)
		return service.failTask(ctx, task.ID, "provider_rejected", "rapid upload rejected", ErrRapidUploadUnavailable)
	}
	if err := service.markStatus(ctx, task.ID, models.PlaybackTransferTaskStatusVerifying); err != nil {
		return p115integration.File{}, "", false, err
	}
	target, err = service.provider.FindTargetFile(ctx, playback.Credential, query)
	if err != nil {
		return service.failTask(ctx, task.ID, "target_verify_failed", "target verification failed", service.reportProviderFailure(playback, failureOperationVerifyPlaybackTarget, err))
	}
	if !validTargetFile(target, query) {
		service.reportRuntimeHealth(playback, p115account.RuntimeHealthProviderProtocol)
		return service.failTask(ctx, task.ID, "target_invalid", "target verification invalid", ErrTargetUnavailable)
	}
	if quota != nil {
		// After Provider success, preserve pending on bookkeeping failure so its
		// original five-minute TTL remains the only automatic release path.
		releasePending = false
		commit, commitErr := service.commitTransferWithRetry(quota.UserID, transferAttemptID)
		if commitErr != nil {
			return service.failTask(ctx, task.ID, "transfer_quota_commit_failed", "transfer quota success bookkeeping failed", ErrTransferQuotaCommitFailed)
		}
		if commit.PendingExpiredBeforeCommit {
			log.Printf("[DirectPlay] code=transfer_pending_expired_before_commit userId=%s", quota.UserID)
		}
		quota.Usage = commit.Usage
	}
	completedAt := service.now().UTC()
	persistCtx, cancelPersist := context.WithTimeout(context.Background(), taskTerminalWriteTimeout)
	defer cancelPersist()
	if err := service.store.MarkSucceeded(persistCtx, task.ID, *target, completedAt); err != nil {
		return p115integration.File{}, "", false, fmt.Errorf("%w: mark_succeeded", ErrStoreUnavailable)
	}
	log.Printf("[DirectPlay] transfer task 成功 taskId=%s playbackAccountId=%s size=%d", task.ID, playback.Credential.AccountID, sourceFile.Size)
	return *target, task.ID, false, nil
}

func newTransferAttemptID() (string, error) {
	random := make([]byte, 16)
	if _, err := rand.Read(random); err != nil {
		return "", err
	}
	return hex.EncodeToString(random), nil
}

func (service *Service) releaseTransferReservation(userID, attemptID string) {
	ctx, cancel := context.WithTimeout(context.Background(), reservationReleaseTimeout)
	defer cancel()
	_, _ = service.transferQuotas.ReleaseTransfer(ctx, userID, attemptID, service.now().UTC())
}

func (service *Service) commitTransferWithRetry(userID, attemptID string) (p115quota.TransferCommitResult, error) {
	budget := service.transferCommitBudget
	if budget <= 0 {
		budget = transferCommitTimeout
	}
	retryInterval := service.transferRetryInterval
	if retryInterval <= 0 {
		retryInterval = transferCommitRetryInterval
	}
	ctx, cancel := context.WithTimeout(context.Background(), budget)
	defer cancel()
	for {
		now := service.now().UTC()
		dayStart, dayEnd := p115quota.DayWindow(now, service.businessTimezone)
		result, err := service.transferQuotas.CommitTransfer(ctx, p115quota.TransferCommitRequest{
			UserID: userID, AttemptID: attemptID, DayStart: dayStart, DayEnd: dayEnd,
		}, now)
		if err == nil {
			return result, nil
		}
		if ctx.Err() != nil {
			return p115quota.TransferCommitResult{}, ErrTransferQuotaCommitFailed
		}
		timer := time.NewTimer(retryInterval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return p115quota.TransferCommitResult{}, ErrTransferQuotaCommitFailed
		case <-timer.C:
		}
	}
}

func mapTransferQuotaError(err error) error {
	if errors.Is(err, p115quota.ErrTransferQuotaExceeded) {
		return ErrTransferQuotaExceeded
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	return ErrRedisUnavailable
}

// searchTarget accepts only one exact file in the configured playback parent.
func (service *Service) searchTarget(ctx context.Context, account p115account.ActiveAccountCredential, query p115integration.FileQuery) (*p115integration.File, bool, error) {
	files, err := service.provider.SearchBySHA1(ctx, account.Credential, query)
	if err != nil {
		return nil, false, service.reportProviderFailure(account, failureOperationSearchPlaybackTarget, err)
	}
	if len(files) > 1 {
		service.reportRuntimeHealth(account, p115account.RuntimeHealthProviderProtocol)
		return nil, false, ErrProviderProtocol
	}
	if len(files) == 0 {
		return nil, false, nil
	}
	if !validTargetFile(&files[0], query) {
		service.reportRuntimeHealth(account, p115account.RuntimeHealthProviderProtocol)
		return nil, false, ErrProviderProtocol
	}
	return &files[0], true, nil
}

// downloadCandidate validates the client-visible HeaderMode before returning
// the short-lived URL; it never persists or logs that URL.
func (service *Service) downloadCandidate(
	ctx context.Context,
	account p115account.ActiveAccountCredential,
	target p115integration.File,
	userAgent, taskID string,
	preexisting bool,
) (RedirectCandidate, error) {
	download, err := service.provider.GetDownloadURL(ctx, account.Credential, p115integration.DownloadURLRequest{
		PickCode: target.PickCode, UserAgent: userAgent,
	})
	if err != nil {
		return RedirectCandidate{}, service.reportProviderFailure(account, failureOperationGetDownloadURL, err)
	}
	if download.URL == "" || !download.ExpiresAt.After(service.now().UTC()) || download.ConcurrentOpenLimit <= 0 {
		service.reportRuntimeHealth(account, p115account.RuntimeHealthProviderProtocol)
		return RedirectCandidate{}, ErrProviderProtocol
	}
	if download.HeaderMode != p115integration.DownloadHeadersNone &&
		download.HeaderMode != p115integration.DownloadHeadersSameUserAgent {
		return RedirectCandidate{}, ErrDownloadIncompatible
	}
	return RedirectCandidate{
		URL: download.URL, ExpiresAt: download.ExpiresAt, HeaderMode: download.HeaderMode,
		ConcurrentOpenLimit: download.ConcurrentOpenLimit, TaskID: taskID, Preexisting: preexisting,
	}, nil
}

// reportProviderFailure preserves the existing DirectPlay error while sending
// only account-wide Provider outcomes to the health state machine.
func (service *Service) reportProviderFailure(
	account p115account.ActiveAccountCredential,
	operation string,
	providerErr error,
) error {
	mapped := mapProviderFailure(operation, providerErr)
	if outcome, ok := runtimeHealthOutcome(providerErr); ok {
		service.reportRuntimeHealth(account, outcome)
	}
	return mapped
}

// reportRuntimeSuccess marks both accounts healthy only after a usable download
// candidate and required transfer persistence have completed.
func (service *Service) reportRuntimeSuccess(source, playback p115account.ActiveAccountCredential) {
	persistCtx, cancelPersist := context.WithTimeout(context.Background(), accountHealthWriteTimeout)
	defer cancelPersist()
	service.reportRuntimeHealthWithContext(persistCtx, source, p115account.RuntimeHealthSucceeded)
	service.reportRuntimeHealthWithContext(persistCtx, playback, p115account.RuntimeHealthSucceeded)
}

// reportRuntimeHealth persists a bounded account outcome without changing the
// redirect/fallback result when the health side effect is stale or unavailable.
func (service *Service) reportRuntimeHealth(account p115account.ActiveAccountCredential, outcome p115account.RuntimeHealthOutcome) {
	persistCtx, cancelPersist := context.WithTimeout(context.Background(), accountHealthWriteTimeout)
	defer cancelPersist()
	service.reportRuntimeHealthWithContext(persistCtx, account, outcome)
}

// reportRuntimeHealthWithContext shares one bounded write budget across all
// account outcomes emitted by the same DirectPlay result.
func (service *Service) reportRuntimeHealthWithContext(
	persistCtx context.Context,
	account p115account.ActiveAccountCredential,
	outcome p115account.RuntimeHealthOutcome,
) {
	if err := service.accounts.ReportRuntimeHealth(persistCtx, account, outcome); err != nil {
		if errors.Is(err, p115account.ErrRuntimeStateChanged) {
			log.Printf("[DirectPlay] 账号运行期健康结果已过期 accountId=%s role=%s outcome=%s",
				account.Credential.AccountID, account.Role, outcome)
			return
		}
		log.Printf("[DirectPlay] 账号运行期健康回写失败 accountId=%s role=%s outcome=%s errorType=%T",
			account.Credential.AccountID, account.Role, outcome, err)
	}
}

// runtimeHealthOutcome excludes request cancellation and file-specific errors
// from account-wide state changes.
func runtimeHealthOutcome(err error) (p115account.RuntimeHealthOutcome, bool) {
	switch {
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		return "", false
	case errors.Is(err, p115integration.ErrCredentialRejected):
		return p115account.RuntimeHealthCredentialRejected, true
	case errors.Is(err, p115integration.ErrProviderUnavailable):
		return p115account.RuntimeHealthProviderUnavailable, true
	case errors.Is(err, p115integration.ErrProviderRejected), errors.Is(err, p115integration.ErrProviderProtocol):
		return p115account.RuntimeHealthProviderProtocol, true
	default:
		return "", false
	}
}

// markStatus persists one non-terminal task transition using the request context.
func (service *Service) markStatus(ctx context.Context, taskID string, status models.PlaybackTransferTaskStatus) error {
	if err := service.store.MarkStatus(ctx, taskID, status, service.now().UTC()); err != nil {
		return fmt.Errorf("%w: mark_status", ErrStoreUnavailable)
	}
	return nil
}

// touchSucceeded records access only after a compatible download URL was issued.
func (service *Service) touchSucceeded(ctx context.Context, playbackAccountID, sha1Value string, size int64) error {
	if err := service.store.TouchSucceeded(ctx, playbackAccountID, sha1Value, size, service.now().UTC()); err != nil {
		return fmt.Errorf("%w: touch", ErrStoreUnavailable)
	}
	return nil
}

// failTask uses an independent bounded context so request cancellation does
// not leave an avoidable active task behind.
func (service *Service) failTask(
	ctx context.Context,
	taskID, code, message string,
	cause error,
) (p115integration.File, string, bool, error) {
	persistCtx, cancelPersist := context.WithTimeout(context.Background(), taskTerminalWriteTimeout)
	defer cancelPersist()
	if err := service.store.MarkFailed(persistCtx, taskID, code, message, service.now().UTC()); err != nil {
		return p115integration.File{}, "", false, fmt.Errorf("%w: mark_failed", ErrStoreUnavailable)
	}
	log.Printf("[DirectPlay] transfer task 失败 taskId=%s code=%s", taskID, code)
	return p115integration.File{}, "", false, cause
}

func validateResolveRequest(request ResolveRequest) error {
	if strings.TrimSpace(request.SourceFile.RootID) == "" ||
		strings.TrimSpace(request.SourceFile.RelativePath) == "" || !validClientUserAgent(request.ClientUserAgent) {
		return ErrInvalidRequest
	}
	return nil
}

func validClientUserAgent(value string) bool {
	trimmed := strings.TrimSpace(value)
	return trimmed != "" && trimmed == value && utf8.ValidString(value) &&
		len(value) <= maxDirectPlayClientUserAgent && !strings.ContainsAny(value, "\r\n")
}

func mapMediaPath(embyPathPrefix, sourceRootID, mediaPath string) (p115integration.FilePathQuery, error) {
	if !validAbsoluteMediaPath(embyPathPrefix, maxDirectPlayMediaPath) ||
		!validAbsoluteMediaPath(mediaPath, maxDirectPlayMediaPath) ||
		!validSourceRootID(sourceRootID) || !strings.HasPrefix(mediaPath, embyPathPrefix+"/") {
		return p115integration.FilePathQuery{}, ErrPathNotMapped
	}
	relativePath := strings.TrimPrefix(mediaPath, embyPathPrefix+"/")
	if !validRelativeMediaPath(relativePath) {
		return p115integration.FilePathQuery{}, ErrPathNotMapped
	}
	return p115integration.FilePathQuery{RootID: sourceRootID, RelativePath: relativePath}, nil
}

func validRelativeMediaPath(value string) bool {
	if value == "" || len(value) > maxMappedRelativePath || strings.HasPrefix(value, "/") {
		return false
	}
	for _, segment := range strings.Split(value, "/") {
		if segment == "" || segment == "." || segment == ".." || len(segment) > maxMappedPathSegment {
			return false
		}
	}
	return true
}

func validSourceRootID(value string) bool {
	parsed, err := strconv.ParseUint(value, 10, 64)
	return err == nil && strconv.FormatUint(parsed, 10) == value && len(value) <= 64
}

func validAbsoluteMediaPath(value string, maxLength int) bool {
	if !utf8.ValidString(value) || value == "/" || len(value) > maxLength ||
		!strings.HasPrefix(value, "/") || strings.HasSuffix(value, "/") ||
		strings.ContainsAny(value, "\\\x00\r\n") {
		return false
	}
	for _, segment := range strings.Split(strings.TrimPrefix(value, "/"), "/") {
		if segment == "" || segment == "." || segment == ".." {
			return false
		}
	}
	return true
}

func validateSourceFile(file *p115integration.File) (string, error) {
	if file == nil || file.IsDirectory || file.Size <= 0 ||
		strings.TrimSpace(file.ID) == "" || strings.TrimSpace(file.PickCode) == "" ||
		strings.TrimSpace(file.ParentID) == "" || strings.TrimSpace(file.Name) == "" {
		return "", ErrProviderProtocol
	}
	sha1Value, err := normalizeSHA1(file.SHA1)
	if err != nil {
		return "", ErrProviderProtocol
	}
	return sha1Value, nil
}

func validTargetFile(file *p115integration.File, query p115integration.FileQuery) bool {
	if file == nil || file.IsDirectory || file.Size != query.Size || file.ParentID != query.ParentID ||
		strings.TrimSpace(file.ID) == "" || strings.TrimSpace(file.PickCode) == "" {
		return false
	}
	sha1Value, err := normalizeSHA1(file.SHA1)
	return err == nil && sha1Value == query.SHA1
}

func validChallenge(challenge *p115integration.RapidUploadChallenge, fileSize int64) bool {
	if challenge == nil || challenge.Range.Start < 0 || challenge.Range.End < challenge.Range.Start ||
		challenge.Range.End >= fileSize {
		return false
	}
	signKey := strings.TrimSpace(challenge.SignKey)
	return signKey != "" && len(signKey) <= 4096 && !strings.ContainsAny(signKey, "\r\n")
}

func boundedRange(size int64) p115integration.ByteRange {
	end := size - 1
	if end >= directPlayPreIDBytes {
		end = directPlayPreIDBytes - 1
	}
	return p115integration.ByteRange{Start: 0, End: end}
}

func validateRangeHash(result p115integration.FileRangeHash, byteRange p115integration.ByteRange) (string, error) {
	expectedBytes := byteRange.End - byteRange.Start + 1
	sha1Value, err := normalizeSHA1(result.SHA1)
	if err != nil || result.BytesRead != expectedBytes {
		return "", ErrProviderProtocol
	}
	return sha1Value, nil
}

func normalizeSHA1(value string) (string, error) {
	value = strings.ToUpper(strings.TrimSpace(value))
	if len(value) != 40 {
		return "", ErrProviderProtocol
	}
	if _, err := hex.DecodeString(value); err != nil {
		return "", ErrProviderProtocol
	}
	return value, nil
}

func mapProviderFailure(operation string, err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	switch {
	case errors.Is(err, p115integration.ErrCredentialRejected):
		return withFailureContext(ErrAccountUnavailable, FailureContext{ProviderOperation: operation})
	case errors.Is(err, p115integration.ErrProviderUnavailable):
		return withFailureContext(ErrProviderUnavailable, FailureContext{ProviderOperation: operation})
	case errors.Is(err, p115integration.ErrDownloadURLIncompatible):
		return withFailureContext(ErrDownloadIncompatible, FailureContext{ProviderOperation: operation})
	case errors.Is(err, p115integration.ErrTargetFileNotVisible), errors.Is(err, p115integration.ErrTargetFileAmbiguous):
		return withFailureContext(ErrTargetUnavailable, FailureContext{ProviderOperation: operation})
	default:
		return withFailureContext(ErrProviderProtocol, FailureContext{ProviderOperation: operation})
	}
}

func providerFailureCode(err error) string {
	switch {
	case errors.Is(err, p115integration.ErrCredentialRejected):
		return "credential_rejected"
	case errors.Is(err, p115integration.ErrProviderUnavailable):
		return "provider_unavailable"
	default:
		return "provider_protocol"
	}
}
