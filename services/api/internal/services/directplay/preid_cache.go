package directplay

import (
	"container/list"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	p115 "github.com/konghang/ember/backend/internal/integrations/p115"
	"github.com/konghang/ember/backend/internal/services/p115account"
)

const (
	preIDCacheCapacity = 1024
	preIDCacheTTL      = 10 * time.Minute
)

type cachedPreID struct {
	key                  string
	result               p115.FileRangeHash
	createdAt, expiresAt time.Time
}

// preIDCache retains only validated source-prefix hashes under opaque identity
// digests. It owns no URL, source bytes, credential, target, task or health state.
type preIDCache struct {
	mu       sync.Mutex
	entries  map[string]*list.Element
	lru      *list.List
	capacity int
	flights  requestGate
}

// newPreIDCache creates an independent bounded cache without background work.
func newPreIDCache(capacity int) *preIDCache {
	return &preIDCache{entries: make(map[string]*list.Element), lru: list.New(), capacity: capacity}
}

// preIDKey binds a clean source snapshot and the exact Provider inputs. Length
// encoding prevents boundary collisions; caller/playback identities are absent.
func preIDKey(source p115account.ActiveAccountCredential, file p115.File, byteRange p115.ByteRange) string {
	sha1Value, err := validateSourceFile(&file)
	if source.DownloadCacheVersion <= 0 || err != nil || byteRange.Start < 0 || byteRange.End < byteRange.Start ||
		byteRange.End >= file.Size || byteRange.End-byteRange.Start+1 > directPlayPreIDBytes {
		return ""
	}
	fields := []string{source.Credential.AccountID, source.ProviderUserID,
		strconv.FormatInt(source.DownloadCacheVersion, 10), source.Credential.Cookie,
		source.Credential.AppType, source.Credential.UserAgent, file.ID, file.PickCode,
		sha1Value, strconv.FormatInt(file.Size, 10), strconv.FormatInt(byteRange.Start, 10), strconv.FormatInt(byteRange.End, 10)}
	digest := sha256.New()
	for _, field := range fields {
		if strings.TrimSpace(field) == "" {
			return ""
		}
		fmt.Fprintf(digest, "%d:%s", len(field), field)
	}
	return hex.EncodeToString(digest.Sum(nil))
}

// get expires a result at its fixed deadline and rejects a backwards clock.
// Access changes capacity eviction order, never the successful read's lifetime.
func (cache *preIDCache) get(key string, now time.Time) (p115.FileRangeHash, bool) {
	cache.mu.Lock()
	defer cache.mu.Unlock()
	element := cache.entries[key]
	if element == nil {
		return p115.FileRangeHash{}, false
	}
	entry := element.Value.(cachedPreID)
	if now.Before(entry.createdAt) || !now.Before(entry.expiresAt) {
		delete(cache.entries, key)
		cache.lru.Remove(element)
		return p115.FileRangeHash{}, false
	}
	cache.lru.MoveToFront(element)
	return entry.result, true
}

// put publishes a successful hash independently of later target upload results.
// Validation and the cancellation check guard every publication, including tests.
func (cache *preIDCache) put(ctx context.Context, key string, result p115.FileRangeHash, byteRange p115.ByteRange, now time.Time) {
	sha1Value, err := validateRangeHash(result, byteRange)
	if cache.capacity <= 0 || key == "" || err != nil || result.BytesRead <= 0 || result.BytesRead > directPlayPreIDBytes {
		return
	}
	cache.mu.Lock()
	defer cache.mu.Unlock()
	if ctx.Err() != nil {
		return
	}
	result.SHA1 = sha1Value
	entry := cachedPreID{key: key, result: result, createdAt: now, expiresAt: now.Add(preIDCacheTTL)}
	if element := cache.entries[key]; element != nil {
		element.Value = entry
		cache.lru.MoveToFront(element)
		return
	}
	if cache.lru.Len() >= cache.capacity {
		oldest := cache.lru.Back()
		delete(cache.entries, oldest.Value.(cachedPreID).key)
		cache.lru.Remove(oldest)
	}
	cache.entries[key] = cache.lru.PushFront(entry)
}

// sourcePreID runs only after target double-check, transfer permission, quota and
// task initialization. A canceled owner releases the gate without a shared worker;
// the next waiter may read again using its own request context.
func (service *Service) sourcePreID(ctx context.Context, source p115account.ActiveAccountCredential, file p115.File) (p115.FileRangeHash, error) {
	if err := ctx.Err(); err != nil {
		return p115.FileRangeHash{}, err
	}
	byteRange := boundedRange(file.Size)
	key := preIDKey(source, file, byteRange)
	cache := service.preIDCache
	if cache == nil || key == "" {
		recordPreIDCache(ctx, "bypass")
		return service.readSourcePreID(ctx, source, file, byteRange)
	}
	if result, found := cache.get(key, service.now()); found {
		recordPreIDCache(ctx, "hit")
		return result, ctx.Err()
	}
	finishWait := measureStage(ctx, "preIDWait")
	release, err := cache.flights.acquire(ctx, key)
	finishWait()
	if err != nil {
		recordPreIDCache(ctx, "miss")
		return p115.FileRangeHash{}, err
	}
	defer release()
	if result, found := cache.get(key, service.now()); found {
		recordPreIDCache(ctx, "hit")
		return result, ctx.Err()
	}
	recordPreIDCache(ctx, "miss")
	result, err := service.readSourcePreID(ctx, source, file, byteRange)
	if err == nil {
		cache.put(ctx, key, result, byteRange, service.now())
	}
	return result, err
}

// readSourcePreID measures only this request's actual Provider call. Validation
// failures retain their existing task error mapping in resolveUnderLock.
func (service *Service) readSourcePreID(ctx context.Context, source p115account.ActiveAccountCredential, file p115.File, byteRange p115.ByteRange) (p115.FileRangeHash, error) {
	finish := measureStage(ctx, "preID")
	defer finish()
	result, err := service.provider.HashFileRange(ctx, source.Credential, p115.FileRangeRequest{File: file, Range: byteRange})
	return result, sourceReadResult(ctx, ctx, err)
}
