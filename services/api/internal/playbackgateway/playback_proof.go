package playbackgateway

import (
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/konghang/ember/backend/internal/services/embytoken"
)

const (
	defaultPlaybackProofTTL        = 5 * time.Minute
	playbackTransferIntentTTL      = 30 * time.Second
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
	transferIntent       bool
	transferIntentUntil  time.Time
	generation           uint64
}

type playbackProofKey struct {
	mappingID     string
	itemID        string
	mediaSourceID string
	playSessionID string
}

// playbackProofPublishGuard belongs to one internal request or client response
// observation. It prevents late completion replacing a newer item snapshot and
// lives only until its owner returns; client observations supersede internals.
type playbackProofPublishGuard struct {
	mappingID   string
	itemID      string
	client      bool
	invalidated bool
}

type playbackProofLookupStatus uint8

const (
	playbackProofMissing playbackProofLookupStatus = iota
	playbackProofFound
	playbackProofExpired
)

type playbackProofCache struct {
	mu           sync.Mutex
	entries      map[playbackProofKey]PlaybackProof
	maxEntries   int
	ttl          time.Duration
	now          func() time.Time
	generation   uint64
	publications map[*playbackProofPublishGuard]struct{}
}

// newPlaybackProofCache creates a bounded cache with no background goroutine;
// expiration and capacity cleanup happen lazily during cache operations.
func newPlaybackProofCache(maxEntries int, ttl time.Duration) *playbackProofCache {
	return &playbackProofCache{
		entries: make(map[playbackProofKey]PlaybackProof), maxEntries: maxEntries,
		publications: make(map[*playbackProofPublishGuard]struct{}),
		ttl:          ttl, now: time.Now,
	}
}

// Record validates and stores proofs with one shared authorization timestamp.
// Each write invalidates pending publishers for the same item. It returns the
// number of valid entries written.
func (cache *playbackProofCache) Record(proofs []PlaybackProof) int {
	if cache == nil || cache.maxEntries <= 0 || cache.ttl <= 0 || cache.now == nil {
		return 0
	}
	now := cache.now().UTC()
	cache.mu.Lock()
	defer cache.mu.Unlock()
	for _, proof := range proofs {
		if validPlaybackProof(proof) {
			cache.invalidatePublicationsLocked(proof.MappingID, proof.ItemID, true)
		}
	}
	return cache.recordLocked(proofs, now)
}

// recordLocked writes validated proofs with one timestamp and new generations;
// callers own the cache lock and the decision to supersede an item snapshot.
func (cache *playbackProofCache) recordLocked(proofs []PlaybackProof, now time.Time) int {
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
		cache.generation++
		proof.generation = cache.generation
		proof.transferIntentUntil = time.Time{}
		if proof.transferIntent {
			proof.transferIntentUntil = now.Add(playbackTransferIntentTTL)
			if proof.ExpiresAt.Before(proof.transferIntentUntil) {
				proof.transferIntentUntil = proof.ExpiresAt
			}
		}
		cache.entries[key] = proof
		written++
	}
	return written
}

// BeginOnDemand atomically rechecks current proof reuse before registering a
// bounded request-lifetime publication guard. A zero proof and nil guard mean
// capacity is exhausted; invalidated guards retain their slot until released.
func (cache *playbackProofCache) BeginOnDemand(principal embytoken.Principal, itemID, mediaSourceID string) (PlaybackProof, *playbackProofPublishGuard) {
	if cache == nil || cache.now == nil || cache.maxEntries <= 0 || cache.ttl <= 0 {
		return PlaybackProof{}, nil
	}
	cache.mu.Lock()
	defer cache.mu.Unlock()
	cache.pruneExpiredLocked(cache.now().UTC())
	if proof, ok := cache.lookupLatestMediaSourceLocked(principal.MappingID, itemID, mediaSourceID); ok && playbackProofMatchesPrincipal(proof, principal) {
		return proof, nil
	}
	if len(cache.publications) >= cache.maxEntries {
		return PlaybackProof{}, nil
	}
	guard := &playbackProofPublishGuard{mappingID: principal.MappingID, itemID: itemID}
	cache.publications[guard] = struct{}{}
	return PlaybackProof{}, guard
}

// BeginClient atomically clears the prior snapshot and supersedes older
// observations before reading a client response body. Capacity rejection still
// fails closed, but never permits an unguarded completion to write later.
func (cache *playbackProofCache) BeginClient(mappingID, itemID string) *playbackProofPublishGuard {
	if cache == nil {
		return nil
	}
	cache.mu.Lock()
	defer cache.mu.Unlock()
	cache.invalidateItemLocked(mappingID, itemID, true)
	if cache.now == nil || cache.maxEntries <= 0 || cache.ttl <= 0 || len(cache.publications) >= cache.maxEntries {
		return nil
	}
	guard := &playbackProofPublishGuard{mappingID: mappingID, itemID: itemID, client: true}
	cache.publications[guard] = struct{}{}
	return guard
}

// ReleasePublication releases an internal or client guard on every completion,
// cancellation, error or panic path without affecting any cached media proof.
func (cache *playbackProofCache) ReleasePublication(guard *playbackProofPublishGuard) {
	if cache == nil || guard == nil {
		return
	}
	cache.mu.Lock()
	defer cache.mu.Unlock()
	delete(cache.publications, guard)
}

// PublishOnDemand replaces the complete item snapshot only while its request
// guard remains current. Publication invalidates competing internal requests,
// including other sources, so late responses cannot reverse the newer state.
func (cache *playbackProofCache) PublishOnDemand(guard *playbackProofPublishGuard, proofs []PlaybackProof) (int, bool) {
	return cache.publishGuarded(guard, proofs, false)
}

// PublishClient conditionally replaces the complete item snapshot, including
// an empty failed response. Superseded client bodies cannot undo newer intent.
func (cache *playbackProofCache) PublishClient(guard *playbackProofPublishGuard, proofs []PlaybackProof) (int, bool) {
	return cache.publishGuarded(guard, proofs, true)
}

// publishGuarded applies one current result under the cache lock. Internal
// results cannot supersede client bodies still being inspected; their final
// successful or empty client snapshot remains authoritative over those GETs.
func (cache *playbackProofCache) publishGuarded(guard *playbackProofPublishGuard, proofs []PlaybackProof, client bool) (int, bool) {
	if cache == nil || guard == nil {
		return 0, false
	}
	cache.mu.Lock()
	defer cache.mu.Unlock()
	if _, active := cache.publications[guard]; !active || guard.invalidated || guard.client != client {
		return 0, false
	}
	cache.invalidateItemLocked(guard.mappingID, guard.itemID, client)
	return cache.recordLocked(proofs, cache.now().UTC()), true
}

// invalidatePublicationsLocked revokes only in-flight results for one item;
// no tombstone survives after the owning requests release their guards.
func (cache *playbackProofCache) invalidatePublicationsLocked(mappingID, itemID string, includeClients bool) {
	for guard := range cache.publications {
		if guard.mappingID == mappingID && guard.itemID == itemID && (includeClients || !guard.client) {
			guard.invalidated = true
		}
	}
}

// CanCreateTransfer rechecks the exact proof under the cache lock at the
// transfer admission point. Reads never extend intent and replaced proofs
// cannot lend a newer intent to an already queued video request.
func (cache *playbackProofCache) CanCreateTransfer(snapshot PlaybackProof, principal embytoken.Principal) bool {
	if cache == nil || cache.now == nil || snapshot.generation == 0 || !playbackProofMatchesPrincipal(snapshot, principal) {
		return false
	}
	cache.mu.Lock()
	defer cache.mu.Unlock()
	now := cache.now()
	current, ok := cache.entries[playbackProofKey{
		mappingID: snapshot.MappingID, itemID: snapshot.ItemID,
		mediaSourceID: snapshot.MediaSourceID, playSessionID: snapshot.PlaySessionID,
	}]
	return ok && current.generation == snapshot.generation && current.Path == snapshot.Path &&
		playbackProofMatchesPrincipal(current, principal) && !now.Before(current.AuthorizedAt) && current.ExpiresAt.After(now) &&
		current.transferIntentUntil.After(now)
}

// RevokeTransferIntent clears only matching session intent after a successful
// Stopped forward. An omitted source covers all sources of that item/session;
// the reusable media proofs and other sessions remain available. This revokes
// recorded grants, not a still-in-flight explicit PlaybackInfo response.
func (cache *playbackProofCache) RevokeTransferIntent(principal embytoken.Principal, itemID, mediaSourceID, playSessionID string) int {
	if cache == nil {
		return 0
	}
	cache.mu.Lock()
	defer cache.mu.Unlock()
	revoked := 0
	for key, proof := range cache.entries {
		if key.itemID != itemID || key.playSessionID != playSessionID ||
			(mediaSourceID != "" && key.mediaSourceID != mediaSourceID) ||
			!playbackProofMatchesPrincipal(proof, principal) || proof.transferIntentUntil.IsZero() {
			continue
		}
		proof.transferIntent = false
		proof.transferIntentUntil = time.Time{}
		cache.entries[key] = proof
		revoked++
	}
	return revoked
}

// Lookup requires the exact composite key and lazily removes an expired entry.
func (cache *playbackProofCache) Lookup(mappingID, itemID, mediaSourceID, playSessionID string) (PlaybackProof, bool) {
	proof, status := cache.lookup(mappingID, itemID, mediaSourceID, playSessionID)
	return proof, status == playbackProofFound
}

// LookupLatestMediaSource returns the freshest non-expired proof for one exact
// mapping/item/source when a client omits PlaySessionId on a later stream URL.
func (cache *playbackProofCache) LookupLatestMediaSource(mappingID, itemID, mediaSourceID string) (PlaybackProof, bool) {
	if cache == nil || cache.now == nil {
		return PlaybackProof{}, false
	}
	now := cache.now().UTC()
	cache.mu.Lock()
	defer cache.mu.Unlock()
	cache.pruneExpiredLocked(now)
	return cache.lookupLatestMediaSourceLocked(mappingID, itemID, mediaSourceID)
}

// lookupLatestMediaSourceLocked selects the newest proof without releasing the
// lock, allowing the on-demand owner to combine reuse and guard registration.
func (cache *playbackProofCache) lookupLatestMediaSourceLocked(mappingID, itemID, mediaSourceID string) (PlaybackProof, bool) {
	var latest PlaybackProof
	found := false
	for key, proof := range cache.entries {
		if key.mappingID != mappingID || key.itemID != itemID || key.mediaSourceID != mediaSourceID {
			continue
		}
		if !found || proof.AuthorizedAt.After(latest.AuthorizedAt) ||
			proof.AuthorizedAt.Equal(latest.AuthorizedAt) && proof.generation > latest.generation {
			latest = proof
			found = true
		}
	}
	return latest, found
}

// lookup distinguishes an exact expired proof from one that never existed so
// the final decision log can explain fallback without exposing cached content.
func (cache *playbackProofCache) lookup(mappingID, itemID, mediaSourceID, playSessionID string) (PlaybackProof, playbackProofLookupStatus) {
	if cache == nil || cache.now == nil {
		return PlaybackProof{}, playbackProofMissing
	}
	key := playbackProofKey{mappingID: mappingID, itemID: itemID, mediaSourceID: mediaSourceID, playSessionID: playSessionID}
	now := cache.now().UTC()
	cache.mu.Lock()
	defer cache.mu.Unlock()
	proof, ok := cache.entries[key]
	if !ok {
		return PlaybackProof{}, playbackProofMissing
	}
	if !proof.ExpiresAt.After(now) {
		delete(cache.entries, key)
		return PlaybackProof{}, playbackProofExpired
	}
	return proof, playbackProofFound
}

// InvalidateItem removes every proof for one mapping/item before a newer
// eligible PlaybackInfo response is evaluated, preventing stale success reuse.
func (cache *playbackProofCache) InvalidateItem(mappingID, itemID string) {
	if cache == nil {
		return
	}
	cache.mu.Lock()
	defer cache.mu.Unlock()
	cache.invalidateItemLocked(mappingID, itemID, true)
}

// invalidateItemLocked clears an item snapshot and invalidates its pending
// internal publishers. Client observations are invalidated only when a newer
// client observation or explicit invalidation supersedes their authority.
func (cache *playbackProofCache) invalidateItemLocked(mappingID, itemID string, includeClients bool) {
	cache.invalidatePublicationsLocked(mappingID, itemID, includeClients)
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
	return playbackProofRejectionReason(proof) == ""
}

// playbackProofRejectionReason returns one stable diagnostic for the first
// contract field that prevents a MediaSource from becoming a 115 proof.
func playbackProofRejectionReason(proof PlaybackProof) string {
	switch {
	case !validProofValue(proof.MappingID, maxProofMappingIDBytes, false),
		!validProofValue(proof.ServerID, maxProofServerIDBytes, false),
		!validProofValue(proof.UserID, maxProofUserIDBytes, false),
		!validProofValue(proof.EmbyUserID, maxProofEmbyUserIDBytes, false),
		!validProofValue(proof.DeviceID, maxProofDeviceIDBytes, true),
		!validProofValue(proof.ClientName, maxProofClientNameBytes, true),
		!validProofValue(proof.ItemID, maxProofItemIDBytes, false):
		return "identity_invalid"
	case !validProofValue(proof.MediaSourceID, maxProofMediaSourceIDBytes, false):
		return "media_source_invalid"
	case !validProofValue(proof.PlaySessionID, maxProofPlaySessionIDBytes, false):
		return "play_session_invalid"
	case proof.Path == "":
		return "path_missing"
	case !validProofValue(proof.Path, maxProofPathBytes, false):
		return "path_invalid"
	case !validProofValue(proof.Container, maxProofContainerBytes, true):
		return "container_invalid"
	case !proof.SupportsDirectPlay:
		return "direct_play_unsupported"
	default:
		return ""
	}
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
