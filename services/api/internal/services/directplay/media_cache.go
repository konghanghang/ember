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
	if key == "" || service.mediaCache == nil || service.downloadCache == nil {
		recordMediaCache(ctx, "bypass")
		return RedirectCandidate{}, false
	}
	recordMediaCache(ctx, "miss")
	media, ok := service.mediaCache.getForSession(key, now, activeSession)
	if !ok {
		return RedirectCandidate{}, false
	}
	result, ok := service.downloadCache.get(media.downloadKey, now)
	if !ok {
		return RedirectCandidate{}, false
	}
	finishPreparation(ctx)
	recordMediaCache(ctx, "hit")
	recordDownloadCache(ctx, "hit")
	return RedirectCandidate{URL: result.URL, ExpiresAt: result.ExpiresAt, HeaderMode: result.HeaderMode,
		ConcurrentOpenLimit: result.ConcurrentOpenLimit, Preexisting: true, downloadCacheHit: true,
		downloadCacheKey: media.downloadKey, resolvedSHA1: media.sha1, resolvedSize: media.size}, true
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
	mu       sync.Mutex
	entries  map[string]mediaResolution
	capacity int
	flights  requestGate
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
	cache.mu.Lock()
	defer cache.mu.Unlock()
	entry, ok := cache.entries[key]
	if !ok {
		return mediaResolution{}, false
	}
	if now.Before(entry.createdAt) || !now.Before(entry.retainedUntil) {
		delete(cache.entries, key)
		return mediaResolution{}, false
	}
	if !now.Before(entry.expiresAt) && (activeSession == "" || activeSession != entry.sessionID) {
		return mediaResolution{}, false
	}
	return entry, true
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
	}
	cache.entries[key] = mediaResolution{downloadKey: candidate.downloadCacheKey,
		sha1: candidate.resolvedSHA1, size: candidate.resolvedSize, createdAt: startedAt, expiresAt: deadline, retainedUntil: urlDeadline, sessionID: sessionID}
}
