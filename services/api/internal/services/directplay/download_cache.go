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
	downloadCacheCapacity     = 1024
	downloadCacheTTL          = 30 * time.Second
	downloadCacheSafetyWindow = 10 * time.Second
	maxCachedDownloadURLBytes = 16 * 1024
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
	mu       sync.Mutex
	entries  map[string]*list.Element
	lru      *list.List
	capacity int
	flights  requestGate
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
	cache.mu.Lock()
	defer cache.mu.Unlock()
	element := cache.entries[key]
	if element == nil {
		return cachedDownload{}, false
	}
	entry := element.Value.(cachedDownload)
	if now.Before(entry.createdAt) || !now.Before(entry.expiresAt) {
		cache.lru.Remove(element)
		delete(cache.entries, key)
		return cachedDownload{}, false
	}
	cache.lru.MoveToFront(element)
	return entry, true
}

// put bounds both lifetime and retained URL bytes. Provider already validates
// CDN policy; Gateway still validates every returned candidate on cache hits.
func (cache *downloadURLCache) put(key string, result p115.DownloadURLResult, now time.Time) {
	deadline := now.Add(downloadCacheTTL)
	if safeExpiry := result.ExpiresAt.Add(-downloadCacheSafetyWindow); safeExpiry.Before(deadline) {
		deadline = safeExpiry
	}
	if cache.capacity <= 0 || len(result.URL) > maxCachedDownloadURLBytes || !deadline.After(now) {
		return
	}
	parsed, err := url.Parse(result.URL)
	if err != nil || parsed.Scheme != "https" || parsed.Hostname() == "" || parsed.User != nil || parsed.Fragment != "" {
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
	}
	cache.entries[key] = cache.lru.PushFront(entry)
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
	if key == "" || service.downloadCache == nil {
		recordDownloadCache(ctx, "bypass")
		return service.fetchDownloadCandidate(ctx, account, target, ua, taskID, preexisting)
	}
	release, err := service.downloadCache.flights.acquire(ctx, key)
	if err != nil {
		return RedirectCandidate{}, err
	}
	defer release()
	if !preexisting {
		service.downloadCache.remove(key)
	}
	if result, ok := service.downloadCache.get(key, service.now()); ok {
		recordDownloadCache(ctx, "hit")
		return RedirectCandidate{URL: result.URL, ExpiresAt: result.ExpiresAt, HeaderMode: result.HeaderMode,
			ConcurrentOpenLimit: result.ConcurrentOpenLimit, TaskID: taskID, Preexisting: preexisting, downloadCacheHit: true, downloadCacheKey: key}, nil
	}
	recordDownloadCache(ctx, "miss")
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
