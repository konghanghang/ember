package playbackgateway

import (
	"time"

	"github.com/konghang/ember/backend/internal/services/embytoken"
	"github.com/konghang/ember/backend/internal/services/p115quota"
)

// playbackSessionBinding outlives media proofs only while real playback events
// confirm the original lease. It carries no transfer permission or credentials.
type playbackSessionBinding struct {
	source         PlaybackProof
	created, until time.Time
}

// rememberPlaybackSession records an admitted redirect's verified client
// identity. A stopped/replaced in-flight proof cannot publish a late binding.
func (cache *playbackProofCache) rememberPlaybackSession(proof PlaybackProof, principal embytoken.Principal) {
	if cache == nil || cache.now == nil || (proof.intentOriginSession == "" && !proof.transferIntent) {
		return
	}
	cache.mu.Lock()
	defer cache.mu.Unlock()
	now := cache.now()
	cache.pruneExpiredLocked(now)
	current, ok := cache.entries[playbackProofKey{proof.MappingID, proof.ItemID, proof.MediaSourceID, proof.PlaySessionID}]
	if !ok || current.generation != proof.generation || !playbackProofMatchesPrincipal(current, principal) {
		return
	}
	origin := proof.intentOriginSession
	if origin == "" {
		origin = proof.PlaySessionID
	}
	// Retain media/session identity only, never the old write permission.
	proof.transferIntent = false
	proof.transferIntentUntil = time.Time{}
	proof.generation = 0
	key := playbackProofKey{proof.MappingID, proof.ItemID, proof.MediaSourceID, origin}
	if binding, found := cache.sessions[key]; found {
		if matchingIntentMedia(binding.source, proof) {
			binding.source = proof
			cache.sessions[key] = binding
		}
		return // Video traffic cannot extend confirmed playback activity.
	}
	if len(cache.sessions) >= cache.maxEntries {
		return
	}
	cache.sessions[key] = playbackSessionBinding{source: proof, created: now, until: now.Add(p115quota.ReservationTTL)}
}

// sessionOriginLocked links a fresh independently authorized proof to one live
// matching media session. Ambiguity and clock rollback fail closed.
func (cache *playbackProofCache) sessionOriginLocked(proof PlaybackProof, now time.Time) string {
	origin := ""
	for key, binding := range cache.sessions {
		if now.Before(binding.created) || !binding.until.After(now) || !matchingIntentMedia(binding.source, proof) {
			continue
		}
		if origin != "" && origin != key.playSessionID {
			return ""
		}
		origin = key.playSessionID
	}
	return origin
}

// updatePlaybackBinding follows successful lease transitions only. It never
// creates a binding from a client event or grants new-transfer permission.
func (cache *playbackProofCache) updatePlaybackBinding(principal embytoken.Principal, item, source, session string, state p115quota.LeaseState, found bool) {
	if cache == nil || cache.now == nil {
		return
	}
	cache.mu.Lock()
	defer cache.mu.Unlock()
	now := cache.now()
	cache.pruneExpiredLocked(now)
	for key, binding := range cache.sessions {
		if key.itemID != item || key.playSessionID != session || (source != "" && key.mediaSourceID != source) || !playbackProofMatchesPrincipal(binding.source, principal) {
			continue
		}
		if now.Before(binding.created) {
			delete(cache.sessions, key)
			continue
		}
		if !found {
			delete(cache.sessions, key)
			continue
		}
		ttl := time.Duration(0)
		switch state {
		case p115quota.LeaseStateActive:
			ttl = p115quota.ActiveTTL
		case p115quota.LeaseStatePaused:
			ttl = p115quota.PausedTTL
		default:
			continue
		}
		binding.until = now.Add(ttl)
		cache.sessions[key] = binding
	}
}

// stopPlaybackBinding also invalidates pending redirect snapshots. It is
// independent of whether Redis still contains the stopped session's lease.
func (cache *playbackProofCache) stopPlaybackBinding(principal embytoken.Principal, item, source, session string) {
	if cache == nil {
		return
	}
	cache.mu.Lock()
	defer cache.mu.Unlock()
	for key, binding := range cache.sessions {
		if key.itemID == item && (source == "" || key.mediaSourceID == source) &&
			(key.playSessionID == session || binding.source.PlaySessionID == session) && playbackProofMatchesPrincipal(binding.source, principal) {
			delete(cache.sessions, key)
		}
	}
	for key, proof := range cache.entries {
		if key.itemID != item || (source != "" && key.mediaSourceID != source) || !playbackProofMatchesPrincipal(proof, principal) {
			continue
		}
		if proof.PlaySessionID != session && proof.intentOriginSession != session {
			continue
		}
		cache.generation++
		proof.generation = cache.generation
		proof.intentOriginSession = ""
		cache.entries[key] = proof
	}
}
