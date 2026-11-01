package playbackgateway

import (
	"time"

	"github.com/konghang/ember/backend/internal/services/embytoken"
	"github.com/konghang/ember/backend/internal/services/p115quota"
)

// observeFallbackPlayback records only successfully accepted events matched to
// an independently authorized media proof. It neither creates Redis leases nor
// revives missing 115 leases. Strict transfer intent is never manufactured.
func (cache *playbackProofCache) observeFallbackPlayback(principal embytoken.Principal, event playbackSessionEventSnapshot) bool {
	if cache == nil || cache.now == nil || event.snapshotState != "recorded" || event.kind == playbackSessionEventStop {
		return false
	}
	cache.mu.Lock()
	defer cache.mu.Unlock()
	now := cache.now()
	cache.pruneExpiredLocked(now)
	ttl := p115quota.ActiveTTL
	if event.isPausedPresent && event.isPaused {
		ttl = p115quota.PausedTTL
	}
	// Existing fallback activity may outlive its media proof. Previously
	// admitted 115 activity remains exclusively governed by Redis transitions.
	var activeKey playbackProofKey
	activeCount := 0
	for key, binding := range cache.sessions {
		if binding.leaseBacked && activityEventMatches(binding.source, principal, event) {
			return false
		}
		if !binding.leaseBacked && activityEventMatches(binding.source, principal, event) && !now.Before(binding.created) {
			activeKey = key
			activeCount++
		}
	}
	if activeCount > 1 {
		return false
	}
	if activeCount == 1 {
		binding := cache.sessions[activeKey]
		binding.until = now.Add(ttl)
		cache.sessions[activeKey] = binding
		return true
	}
	var selected PlaybackProof
	count := 0
	for key, proof := range cache.entries {
		state := cache.lifecycle[key]
		if (state.generation == proof.generation && (state.admitted || state.stopped)) || now.Before(proof.AuthorizedAt) || !activityEventMatches(proof, principal, event) {
			continue
		}
		selected = proof
		count++
	}
	if count != 1 {
		return false
	}
	selected.transferIntent, selected.transferIntentUntil = false, time.Time{}
	key := playbackProofKey{principal.MappingID, selected.ItemID, selected.MediaSourceID, event.playSessionID}
	makePlaybackCacheRoomLocked(cache.sessions, key, cache.maxEntries, func(b playbackSessionBinding) (time.Time, uint64) { return b.created, b.sequence })
	cache.generation++
	cache.sessions[key] = playbackSessionBinding{source: selected, created: now, until: now.Add(ttl), sequence: cache.generation, confirmed: true}
	return true
}

// activityEventMatches requires the current authenticated device and exact
// item/session; an omitted media source is accepted only after unique matching.
func activityEventMatches(proof PlaybackProof, principal embytoken.Principal, event playbackSessionEventSnapshot) bool {
	return playbackProofMatchesPrincipal(proof, principal) && proof.ClientName == principal.ClientName && proof.ItemID == event.itemID &&
		(proof.PlaySessionID == event.playSessionID || proof.intentOriginSession == event.playSessionID) &&
		(event.mediaSourceID == "" || proof.MediaSourceID == event.mediaSourceID)
}

// hasConfirmedPlayback distinguishes live matched activity from a reservation
// or raw video traffic; it does not alter the strict JSON intent contract.
func (cache *playbackProofCache) hasConfirmedPlayback(proof PlaybackProof, principal embytoken.Principal) bool {
	if cache == nil || cache.now == nil {
		return false
	}
	cache.mu.Lock()
	defer cache.mu.Unlock()
	now := cache.now()
	current, ok := cache.entries[playbackProofKey{proof.MappingID, proof.ItemID, proof.MediaSourceID, proof.PlaySessionID}]
	if !ok || current.generation != proof.generation || !playbackProofMatchesPrincipal(current, principal) {
		return false
	}
	state := cache.lifecycle[playbackProofKey{proof.MappingID, proof.ItemID, proof.MediaSourceID, proof.PlaySessionID}]
	if state.generation == current.generation && state.stopped {
		return false
	}
	origin := current.intentOriginSession
	if origin == "" {
		origin = current.PlaySessionID
	}
	for key, binding := range cache.sessions {
		if key.playSessionID == origin && binding.confirmed && !now.Before(binding.created) && binding.until.After(now) && matchingIntentMedia(binding.source, current) {
			return true
		}
	}
	return false
}
