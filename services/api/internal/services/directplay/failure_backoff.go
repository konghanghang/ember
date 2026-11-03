package directplay

import (
	"container/list"
	"context"
	"errors"
	"sync"
	"time"

	p115 "github.com/konghang/ember/backend/internal/integrations/p115"
	"github.com/konghang/ember/backend/internal/logging"
	"github.com/konghang/ember/backend/internal/services/p115account"
)

const (
	sourceFailureBackoffCapacity = 1024
	sourceFailureBackoffTTL      = 10 * time.Second
)

type cachedSourceFailure struct {
	key                  string
	reason               p115.DownloadURLPolicyReason
	detail               p115.FailureDetail
	createdAt, expiresAt time.Time
}

// sourceFailureBackoff bounds deterministic source refusals without retaining
// raw errors, credentials, URLs or hostnames. It never grants playback access.
type sourceFailureBackoff struct {
	mu       sync.Mutex
	entries  map[string]*list.Element
	lru      *list.List
	capacity int
}

// newSourceFailureBackoff owns a small process-local LRU with no background work.
func newSourceFailureBackoff(capacity int) *sourceFailureBackoff {
	return &sourceFailureBackoff{entries: make(map[string]*list.Element), lru: list.New(), capacity: capacity}
}

// sourceFailureBackoffKey binds the validated source and both clean account
// generations to one caller/device/UA. Recovery probes and missing scope bypass.
func sourceFailureBackoffKey(source, playback p115account.ActiveAccountCredential, file p115.File, request ResolveRequest) string {
	sourceKey := preIDKey(source, file, boundedRange(file.Size))
	callerKey := request.downloadScope.downloadKey(playback, file, request.ClientUserAgent)
	return mediaDigest(sourceKey, callerKey, playback.ProviderUserID, source.EmbyPathPrefix,
		source.SourceRootID, request.SourceFile.RootID, request.SourceFile.RelativePath, file.ParentID, file.Name)
}

// get rejects expired entries and clock rollback; an access changes LRU order
// but never prolongs a failure window or revives an expired refusal.
func (cache *sourceFailureBackoff) get(key string, now time.Time) (cachedSourceFailure, bool) {
	if cache == nil || key == "" {
		return cachedSourceFailure{}, false
	}
	cache.mu.Lock()
	defer cache.mu.Unlock()
	element := cache.entries[key]
	if element == nil {
		return cachedSourceFailure{}, false
	}
	entry := element.Value.(cachedSourceFailure)
	if now.Before(entry.createdAt) || !now.Before(entry.expiresAt) {
		delete(cache.entries, key)
		cache.lru.Remove(element)
		return cachedSourceFailure{}, false
	}
	cache.lru.MoveToFront(element)
	return entry, true
}

// put admits only typed host-policy refusals after a live caller's failed task
// was persisted. Generic protocol, transport and cancellation errors stay fresh.
func (cache *sourceFailureBackoff) put(ctx context.Context, key string, err error, now time.Time) bool {
	if cache == nil || cache.capacity <= 0 || key == "" || ctx == nil || ctx.Err() != nil ||
		errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	var policy *p115.DownloadURLPolicyError
	if !errors.As(err, &policy) || policy.Reason != p115.DownloadURLPolicyHostNotAllowed {
		return false
	}
	cache.mu.Lock()
	defer cache.mu.Unlock()
	if ctx.Err() != nil {
		return false
	}
	if element := cache.entries[key]; element != nil {
		entry := element.Value.(cachedSourceFailure)
		if !now.Before(entry.createdAt) && now.Before(entry.expiresAt) {
			return false
		}
		delete(cache.entries, key)
		cache.lru.Remove(element)
	}
	if cache.lru.Len() >= cache.capacity {
		oldest := cache.lru.Back()
		delete(cache.entries, oldest.Value.(cachedSourceFailure).key)
		cache.lru.Remove(oldest)
	}
	entry := cachedSourceFailure{key: key, reason: policy.Reason, detail: p115.InspectFailureDetail(err),
		createdAt: now, expiresAt: now.Add(sourceFailureBackoffTTL)}
	cache.entries[key] = cache.lru.PushFront(entry)
	return true
}

// cachedSourceFailure preserves the original public fallback classification and
// safe Provider details without replaying any account health or task side effect.
func (service *Service) cachedSourceFailure(ctx context.Context, key string, source p115account.ActiveAccountCredential, file p115.File) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	entry, found := service.sourceFailures.get(key, service.now())
	if !found {
		return nil
	}
	// A newer independently successful source read is stronger evidence than
	// this caller's short refusal, while retaining the normal task/quota path.
	if service.preIDCache != nil {
		if _, valid := service.preIDCache.get(preIDKey(source, file, boundedRange(file.Size)), service.now()); valid {
			return nil
		}
	}
	logging.Debugf("[DirectPlay] code=source_failure_backoff outcome=hit reasonCode=download_host_not_allowed sourceAccountId=%q sourceFileId=%q", source.Credential.AccountID, file.ID)
	cause := mapProviderFailure(failureOperationHashSourcePreID, &p115.DownloadURLPolicyError{Reason: entry.reason})
	return withFailureContext(cause, FailureContext{ProviderOperation: failureOperationHashSourcePreID, ProviderDetail: entry.detail})
}

// rememberSourceFailure publishes only after task failure persistence succeeds;
// a storage failure or a canceled request cannot suppress another real attempt.
func (service *Service) rememberSourceFailure(ctx context.Context, key string, providerErr, finalErr error, source p115account.ActiveAccountCredential, file p115.File) {
	if errors.Is(finalErr, ErrStoreUnavailable) || !errors.Is(finalErr, ErrProviderProtocol) {
		return
	}
	if service.sourceFailures.put(ctx, key, providerErr, service.now()) {
		logging.Debugf("[DirectPlay] code=source_failure_backoff outcome=stored reasonCode=download_host_not_allowed sourceAccountId=%q sourceFileId=%q", source.Credential.AccountID, file.ID)
	}
}
