package playbackgateway

import (
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

const (
	defaultPlaybackProofTTL        = 5 * time.Minute
	defaultPlaybackProofMaxEntries = 4096
	maxProofMappingIDBytes         = 25
	maxProofServerIDBytes          = 64
	maxProofUserIDBytes            = 25
	maxProofEmbyUserIDBytes        = 50
	maxProofDeviceIDBytes          = 256
	maxProofClientNameBytes        = 128
	maxProofItemIDBytes            = 128
	maxProofMediaSourceIDBytes     = 128
	maxProofPlaySessionIDBytes     = 128
	maxProofPathBytes              = 8192
	maxProofContainerBytes         = 64
)

// PlaybackProof is one short-lived, in-process authorization observation from
// a successful Emby PlaybackInfo response. It never contains the raw Token.
type PlaybackProof struct {
	MappingID            string
	ServerID             string
	UserID               string
	EmbyUserID           string
	DeviceID             string
	ClientName           string
	ItemID               string
	MediaSourceID        string
	PlaySessionID        string
	Path                 string
	Size                 int64
	Container            string
	IsRemote             bool
	SupportsDirectPlay   bool
	SupportsDirectStream bool
	SupportsTranscoding  bool
	AuthorizedAt         time.Time
	ExpiresAt            time.Time
}

type playbackProofKey struct {
	mappingID     string
	itemID        string
	mediaSourceID string
	playSessionID string
}

type playbackProofCache struct {
	mu         sync.Mutex
	entries    map[playbackProofKey]PlaybackProof
	maxEntries int
	ttl        time.Duration
	now        func() time.Time
}

// newPlaybackProofCache creates a bounded cache with no background goroutine;
// expiration and capacity cleanup happen only on Record, Lookup and Len.
func newPlaybackProofCache(maxEntries int, ttl time.Duration) *playbackProofCache {
	return &playbackProofCache{
		entries: make(map[playbackProofKey]PlaybackProof), maxEntries: maxEntries,
		ttl: ttl, now: time.Now,
	}
}

// Record validates and stores proofs with one shared authorization timestamp.
// It returns the number of valid entries written.
func (cache *playbackProofCache) Record(proofs []PlaybackProof) int {
	if cache == nil || cache.maxEntries <= 0 || cache.ttl <= 0 || cache.now == nil {
		return 0
	}
	now := cache.now().UTC()
	cache.mu.Lock()
	defer cache.mu.Unlock()
	cache.pruneExpiredLocked(now)
	written := 0
	for _, proof := range proofs {
		if !validPlaybackProof(proof) {
			continue
		}
		key := playbackProofKey{
			mappingID: proof.MappingID, itemID: proof.ItemID,
			mediaSourceID: proof.MediaSourceID, playSessionID: proof.PlaySessionID,
		}
		if _, exists := cache.entries[key]; !exists && len(cache.entries) >= cache.maxEntries {
			cache.evictEarliestLocked()
		}
		proof.AuthorizedAt = now
		proof.ExpiresAt = now.Add(cache.ttl)
		cache.entries[key] = proof
		written++
	}
	return written
}

// Lookup requires the exact composite key and lazily removes an expired entry.
func (cache *playbackProofCache) Lookup(mappingID, itemID, mediaSourceID, playSessionID string) (PlaybackProof, bool) {
	if cache == nil || cache.now == nil {
		return PlaybackProof{}, false
	}
	key := playbackProofKey{mappingID: mappingID, itemID: itemID, mediaSourceID: mediaSourceID, playSessionID: playSessionID}
	now := cache.now().UTC()
	cache.mu.Lock()
	defer cache.mu.Unlock()
	proof, ok := cache.entries[key]
	if !ok {
		return PlaybackProof{}, false
	}
	if !proof.ExpiresAt.After(now) {
		delete(cache.entries, key)
		return PlaybackProof{}, false
	}
	return proof, true
}

// InvalidateItem removes every proof for one mapping/item before a newer
// eligible PlaybackInfo response is evaluated, preventing stale success reuse.
func (cache *playbackProofCache) InvalidateItem(mappingID, itemID string) {
	if cache == nil {
		return
	}
	cache.mu.Lock()
	defer cache.mu.Unlock()
	for key := range cache.entries {
		if key.mappingID == mappingID && key.itemID == itemID {
			delete(cache.entries, key)
		}
	}
}

// Len returns the current non-expired entry count for bounded diagnostics and
// tests; it does not expose proof contents.
func (cache *playbackProofCache) Len() int {
	if cache == nil || cache.now == nil {
		return 0
	}
	now := cache.now().UTC()
	cache.mu.Lock()
	defer cache.mu.Unlock()
	cache.pruneExpiredLocked(now)
	return len(cache.entries)
}

// pruneExpiredLocked removes expired entries while the caller holds cache.mu.
func (cache *playbackProofCache) pruneExpiredLocked(now time.Time) {
	for key, proof := range cache.entries {
		if !proof.ExpiresAt.After(now) {
			delete(cache.entries, key)
		}
	}
}

// evictEarliestLocked removes one entry with the nearest expiry while the
// caller holds cache.mu.
func (cache *playbackProofCache) evictEarliestLocked() {
	var earliestKey playbackProofKey
	var earliest time.Time
	found := false
	for key, proof := range cache.entries {
		if !found || proof.ExpiresAt.Before(earliest) {
			earliestKey = key
			earliest = proof.ExpiresAt
			found = true
		}
	}
	if found {
		delete(cache.entries, earliestKey)
	}
}

// validPlaybackProof bounds all cache-resident identity and media fields.
func validPlaybackProof(proof PlaybackProof) bool {
	return validProofValue(proof.MappingID, maxProofMappingIDBytes, false) &&
		validProofValue(proof.ServerID, maxProofServerIDBytes, false) &&
		validProofValue(proof.UserID, maxProofUserIDBytes, false) &&
		validProofValue(proof.EmbyUserID, maxProofEmbyUserIDBytes, false) &&
		validProofValue(proof.DeviceID, maxProofDeviceIDBytes, true) &&
		validProofValue(proof.ClientName, maxProofClientNameBytes, true) &&
		validProofValue(proof.ItemID, maxProofItemIDBytes, false) &&
		validProofValue(proof.MediaSourceID, maxProofMediaSourceIDBytes, false) &&
		validProofValue(proof.PlaySessionID, maxProofPlaySessionIDBytes, false) &&
		validProofValue(proof.Path, maxProofPathBytes, false) &&
		validProofValue(proof.Container, maxProofContainerBytes, true) &&
		proof.Size > 0 && proof.SupportsDirectPlay
}

// validProofValue rejects normalized/control-character variants before a value
// can become part of a cache key or cached media path.
func validProofValue(value string, maxBytes int, allowEmpty bool) bool {
	if value == "" {
		return allowEmpty
	}
	return utf8.ValidString(value) && len(value) <= maxBytes && strings.TrimSpace(value) == value &&
		!strings.ContainsAny(value, "\x00\r\n")
}
