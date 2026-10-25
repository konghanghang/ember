package directplay

import (
	"container/list"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	p115 "github.com/konghang/ember/backend/internal/integrations/p115"
	"github.com/konghang/ember/backend/internal/services/p115account"
)

const (
	downloadCacheCapacity        = 1024
	mediaResolutionFreshnessTTL  = 10 * time.Minute
	downloadCacheMaxSafetyWindow = 5 * time.Minute
	downloadCacheSafetyRatio     = 10
	maxCachedDownloadURLBytes    = 16 * 1024
)

// downloadCacheScope deliberately excludes PlaySessionId: stopping playback
// releases its lease, but the same authenticated device may replay shortly after.
type downloadCacheScope struct {
	serverID, userID, mappingID, deviceID string
}

type cachedDownload struct {
	key                  string
	result               p115.DownloadURLResult
	createdAt, expiresAt time.Time
}

// downloadURLCache retains only bounded, process-local signed URLs and opaque
// keys. It never owns playback leases, task provenance or account health.
type downloadURLCache struct {
	mu        sync.Mutex
	entries   map[string]*list.Element
	lru       *list.List
	capacity  int
	flights   requestGate
	evictions uint64
}

// newDownloadURLCache creates a bounded LRU without background workers.
func newDownloadURLCache(capacity int) *downloadURLCache {
	return &downloadURLCache{entries: make(map[string]*list.Element), lru: list.New(), capacity: capacity}
}

// downloadKey isolates device/login, account generation and exact target/UA.
// Hashing length-prefixed fields avoids ambiguity and retaining raw credentials.
func (scope *downloadCacheScope) downloadKey(account p115account.ActiveAccountCredential, target p115.File, ua string) string {
	if scope == nil || account.DownloadCacheVersion <= 0 {
		return ""
	}
	fields := []string{scope.serverID, scope.userID, scope.mappingID, scope.deviceID,
		account.Credential.AccountID, strconv.FormatInt(account.DownloadCacheVersion, 10),
		account.Credential.Cookie, account.Credential.AppType, account.Credential.UserAgent,
		account.TargetParentID, target.ID, target.PickCode, strings.ToUpper(target.SHA1), strconv.FormatInt(target.Size, 10), ua}
	h := sha256.New()
	for _, field := range fields {
		if field == "" {
			return ""
		}
		fmt.Fprintf(h, "%d:%s", len(field), field)
	}
	return hex.EncodeToString(h.Sum(nil))
}

// get expires entries at the fixed deadline; hits change LRU order, not TTL.
func (cache *downloadURLCache) get(key string, now time.Time) (p115.DownloadURLResult, bool) {
	entry, ok := cache.getEntry(key, now)
	return entry.result, ok
}

// getEntry exposes the original deadline to dependent media-cache entries;
// a lookup must never extend either layer's lifetime.
func (cache *downloadURLCache) getEntry(key string, now time.Time) (cachedDownload, bool) {
	entry, reason := cache.lookupEntry(key, now)
	return entry, reason == ""
}

// lookupEntry reports only the rejection observed at this lookup, preserving
// deadline checks, invalid-entry removal and access LRU. Empty reason is a hit.
func (cache *downloadURLCache) lookupEntry(key string, now time.Time) (cachedDownload, string) {
	cache.mu.Lock()
	defer cache.mu.Unlock()
	element := cache.entries[key]
	if element == nil {
		return cachedDownload{}, "entry_missing"
	}
	entry := element.Value.(cachedDownload)
	var reason string
	switch {
	case now.Before(entry.createdAt):
		reason = "clock_rollback"
	case !now.Before(entry.expiresAt):
		reason = "expired"
	case !allowedCachedDownloadHost(entry.result.URL):
		reason = "host_rejected"
	}
	if reason != "" {
		cache.lru.Remove(element)
		delete(cache.entries, key)
		return cachedDownload{}, reason
	}
	cache.lru.MoveToFront(element)
	return entry, ""
}

// evictionCount samples only capacity removals under the existing cache mutex.
func (cache *downloadURLCache) evictionCount() uint64 {
	cache.mu.Lock()
	defer cache.mu.Unlock()
	return cache.evictions
}

// put retains a URL until its provider expiry minus the safety window, never
// extending that deadline on access. Capacity bounds memory independently.
func (cache *downloadURLCache) put(key string, result p115.DownloadURLResult, now time.Time) {
	deadline, ok := downloadCacheDeadline(result.ExpiresAt, now)
	if cache.capacity <= 0 || len(result.URL) > maxCachedDownloadURLBytes || !ok {
		return
	}
	parsed, err := url.Parse(result.URL)
	if err != nil || parsed.Scheme != "https" || parsed.Hostname() == "" || parsed.User != nil || parsed.Fragment != "" ||
		!p115.IsAllowedDownloadHost(strings.ToLower(parsed.Hostname())) {
		return
	}
	cache.mu.Lock()
	defer cache.mu.Unlock()
	entry := cachedDownload{key: key, result: result, createdAt: now, expiresAt: deadline}
	if element := cache.entries[key]; element != nil {
		element.Value = entry
		cache.lru.MoveToFront(element)
		return
	}
	if cache.lru.Len() >= cache.capacity {
		oldest := cache.lru.Back()
		delete(cache.entries, oldest.Value.(cachedDownload).key)
		cache.lru.Remove(oldest)
		cache.evictions++
	}
	cache.entries[key] = cache.lru.PushFront(entry)
}

// downloadCacheDeadline keeps a minute-scale safety window without disabling
// short-lived URLs. Safety is min(5m, 10% of remaining life) so a 10s link
// still caches for 9s instead of being rejected by a fixed 10s cut.
func downloadCacheDeadline(expiresAt, now time.Time) (time.Time, bool) {
	remaining := expiresAt.Sub(now)
	if remaining <= 0 {
		return time.Time{}, false
	}
	safety := remaining / downloadCacheSafetyRatio
	if safety > downloadCacheMaxSafetyWindow {
		safety = downloadCacheMaxSafetyWindow
	}
	deadline := expiresAt.Add(-safety)
	return deadline, deadline.After(now)
}

// allowedCachedDownloadHost re-checks the existing 115 hostname allowlist on
// cache hits. It does not widen the Provider boundary.
func allowedCachedDownloadHost(rawURL string) bool {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return false
	}
	return p115.IsAllowedDownloadHost(strings.ToLower(parsed.Hostname()))
}

// remove discards a previously signed URL when fresh lookup required a new
// transfer. Even an upstream that recycles file identifiers must get a new URL.
func (cache *downloadURLCache) remove(key string) {
	cache.mu.Lock()
	defer cache.mu.Unlock()
	if element := cache.entries[key]; element != nil {
		cache.lru.Remove(element)
		delete(cache.entries, key)
	}
}

// downloadCandidate reuses only the final URL after fresh account admission and
// file verification. Each caller retains its own lease and transfer metadata.
func (service *Service) downloadCandidate(ctx context.Context, account p115account.ActiveAccountCredential,
	target p115.File, ua, taskID string, preexisting bool, scope *downloadCacheScope,
) (RedirectCandidate, error) {
	key := scope.downloadKey(account, target, ua)
	if key == "" {
		recordDownloadCache(ctx, "bypass", "key_unavailable")
		return service.fetchDownloadCandidate(ctx, account, target, ua, taskID, preexisting)
	}
	if service.downloadCache == nil {
		recordDownloadCache(ctx, "bypass", "cache_unavailable")
		return service.fetchDownloadCandidate(ctx, account, target, ua, taskID, preexisting)
	}
	finishWait := observeStep(ctx, "downloadWait")
	release, err := service.downloadCache.flights.acquire(ctx, key)
	finishWait()
	if err != nil {
		return RedirectCandidate{}, err
	}
	defer release()
	recordCacheUse(ctx, nil, service.downloadCache)
	if !preexisting {
		service.downloadCache.remove(key)
	}
	entry, reason := service.downloadCache.lookupEntry(key, service.now())
	if reason == "" {
		recordDownloadCache(ctx, "hit", "")
		result := entry.result
		return RedirectCandidate{URL: result.URL, ExpiresAt: result.ExpiresAt, HeaderMode: result.HeaderMode,
			ConcurrentOpenLimit: result.ConcurrentOpenLimit, TaskID: taskID, Preexisting: preexisting, downloadCacheHit: true, downloadCacheKey: key}, nil
	}
	if !preexisting {
		reason = "forced_refresh"
	}
	recordDownloadCache(ctx, "miss", reason)
	candidate, err := service.fetchDownloadCandidate(ctx, account, target, ua, taskID, preexisting)
	if err != nil {
		return candidate, err
	}
	if err := ctx.Err(); err != nil {
		return RedirectCandidate{}, err
	}
	service.downloadCache.put(key, p115.DownloadURLResult{URL: candidate.URL, ExpiresAt: candidate.ExpiresAt,
		HeaderMode: candidate.HeaderMode, ConcurrentOpenLimit: candidate.ConcurrentOpenLimit}, service.now())
	candidate.downloadCacheKey = key
	return candidate, nil
}
