package directplay

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/konghang/ember/backend/internal/services/p115account"
)

// cachedMediaCandidate reuses only validated file identity and a still-live
// download entry. It deliberately omits prior task, quota and health outcomes.
func (service *Service) cachedMediaCandidate(ctx context.Context, key string, now time.Time, activeSession string) (RedirectCandidate, bool) {
	if key == "" {
		recordMediaCache(ctx, "bypass", "key_unavailable")
		return RedirectCandidate{}, false
	}
	if service.mediaCache == nil || service.downloadCache == nil {
		recordMediaCache(ctx, "bypass", "cache_unavailable")
		return RedirectCandidate{}, false
	}
	recordCacheUse(ctx, service.mediaCache, nil)
	media, reason := service.mediaCache.lookupForSession(key, now, activeSession)
	if reason != "" {
		recordMediaCache(ctx, "miss", reason)
		return RedirectCandidate{}, false
	}
	recordCacheUse(ctx, nil, service.downloadCache)
	entry, reason := service.downloadCache.lookupEntry(media.downloadKey, now)
	recordMediaCache(ctx, "miss", dependentURLCacheReason(reason))
	if reason != "" {
		return RedirectCandidate{}, false
	}
	finishPreparation(ctx)
	recordMediaCache(ctx, "hit", "")
	recordDownloadCache(ctx, "hit", "")
	result := entry.result
	return RedirectCandidate{URL: result.URL, ExpiresAt: result.ExpiresAt, HeaderMode: result.HeaderMode,
		ConcurrentOpenLimit: result.ConcurrentOpenLimit, Preexisting: true, downloadCacheHit: true,
		downloadCacheKey: media.downloadKey, resolvedSHA1: media.sha1, resolvedSize: media.size}, true
}

// dependentURLCacheReason names only the failure seen in this dependency
// lookup; an absent URL cannot prove eviction or configuration changes.
func dependentURLCacheReason(reason string) string {
	switch reason {
	case "entry_missing":
		return "dependent_url_missing"
	case "expired":
		return "dependent_url_expired"
	case "clock_rollback":
		return "dependent_url_clock_rollback"
	case "host_rejected":
		return "dependent_url_host_rejected"
	default:
		return ""
	}
}

// mediaResolution stores content identity and its originating session. Fixed
// freshness governs other sessions; retainedUntil bounds live-session reuse.
type mediaResolution struct {
	downloadKey, sha1    string
	size                 int64
	createdAt, expiresAt time.Time
	retainedUntil        time.Time
	sessionID            string
}

// mediaResolutionCache bounds path reuse without background work. Neither
// cache hits nor playback progress extend stored freshness or URL expiration.
type mediaResolutionCache struct {
	mu        sync.Mutex
	entries   map[string]mediaResolution
	capacity  int
	flights   requestGate
	evictions uint64
}

// newMediaResolutionCache creates a process-local, bounded metadata cache.
func newMediaResolutionCache(capacity int) *mediaResolutionCache {
	return &mediaResolutionCache{entries: make(map[string]mediaResolution), capacity: capacity}
}

// mediaRequestKey isolates users, devices, media paths and actual UA without
// PlaySessionId. It is only an in-flight gate key, not an authorization result.
func mediaRequestKey(serverID string, r MediaPathResolveRequest) string {
	return mediaDigest(serverID, r.UserID, r.MappingID, r.DeviceID, r.Path, r.ClientUserAgent)
}

// mediaResolutionKey also binds both accounts' clean configuration snapshots.
// Unknown versions and recovery probes must perform real Provider operations.
func mediaResolutionKey(requestKey string, source, playback p115account.ActiveAccountCredential) string {
	if requestKey == "" || source.DownloadCacheVersion <= 0 || playback.DownloadCacheVersion <= 0 {
		return ""
	}
	fields := []string{requestKey, source.EmbyPathPrefix, source.SourceRootID, playback.TargetParentID}
	for _, account := range []p115account.ActiveAccountCredential{source, playback} {
		fields = append(fields, account.Credential.AccountID, account.ProviderUserID,
			strconv.FormatInt(account.DownloadCacheVersion, 10), account.Credential.Cookie,
			account.Credential.AppType, account.Credential.UserAgent)
	}
	return mediaDigest(fields...)
}

// mediaDigest uses length-prefixed fields and retains only an opaque digest.
func mediaDigest(fields ...string) string {
	h := sha256.New()
	for _, field := range fields {
		if field == "" {
			return ""
		}
		fmt.Fprintf(h, "%d:%s", len(field), field)
	}
	return hex.EncodeToString(h.Sum(nil))
}

// get rejects expired entries and clock rollback. URL-cache expiry/eviction is
// checked separately before a candidate can be returned.
func (cache *mediaResolutionCache) get(key string, now time.Time) (mediaResolution, bool) {
	return cache.getForSession(key, now, "")
}

// getForSession preserves the fixed cross-session freshness window, permitting
// longer reuse only for the original, independently confirmed active lease.
func (cache *mediaResolutionCache) getForSession(key string, now time.Time, activeSession string) (mediaResolution, bool) {
	entry, reason := cache.lookupForSession(key, now, activeSession)
	return entry, reason == ""
}

// lookupForSession reports the current rejection while preserving deletion,
// freshness and original active-session reuse. Empty reason means a hit.
func (cache *mediaResolutionCache) lookupForSession(key string, now time.Time, activeSession string) (mediaResolution, string) {
	cache.mu.Lock()
	defer cache.mu.Unlock()
	entry, ok := cache.entries[key]
	if !ok {
		return mediaResolution{}, "entry_missing"
	}
	if now.Before(entry.createdAt) {
		delete(cache.entries, key)
		return mediaResolution{}, "clock_rollback"
	}
	if !now.Before(entry.retainedUntil) {
		delete(cache.entries, key)
		return mediaResolution{}, "expired"
	}
	if !now.Before(entry.expiresAt) && (activeSession == "" || activeSession != entry.sessionID) {
		return mediaResolution{}, "freshness_expired"
	}
	return entry, ""
}

// evictionCount samples only capacity removals under the cache's own mutex.
func (cache *mediaResolutionCache) evictionCount() uint64 {
	cache.mu.Lock()
	defer cache.mu.Unlock()
	return cache.evictions
}

// put starts freshness at file resolution, capped by the original URL-cache
// deadline. Only a fully admitted successful request may publish metadata.
func (cache *mediaResolutionCache) put(key string, candidate RedirectCandidate, startedAt, now, urlDeadline time.Time, sessionID string) {
	deadline := startedAt.Add(mediaResolutionFreshnessTTL)
	if urlDeadline.Before(deadline) {
		deadline = urlDeadline
	}
	if key == "" || cache.capacity <= 0 || candidate.downloadCacheKey == "" || candidate.resolvedSHA1 == "" ||
		candidate.resolvedSize <= 0 || now.Before(startedAt) || !deadline.After(now) {
		return
	}
	cache.mu.Lock()
	defer cache.mu.Unlock()
	if _, found := cache.entries[key]; !found && len(cache.entries) >= cache.capacity {
		var oldestKey string
		var oldest time.Time
		for k, entry := range cache.entries {
			if oldestKey == "" || entry.expiresAt.Before(oldest) {
				oldestKey, oldest = k, entry.expiresAt
			}
		}
		delete(cache.entries, oldestKey)
		cache.evictions++
	}
	cache.entries[key] = mediaResolution{downloadKey: candidate.downloadCacheKey,
		sha1: candidate.resolvedSHA1, size: candidate.resolvedSize, createdAt: startedAt, expiresAt: deadline, retainedUntil: urlDeadline, sessionID: sessionID}
}
