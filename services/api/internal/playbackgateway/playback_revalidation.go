package playbackgateway

import (
	"context"
	"crypto/sha256"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/konghang/ember/backend/internal/services/embytoken"
)

// playbackRevalidationSnapshot identifies an admitted, still-live client
// session. Its media fields are comparison evidence, never a reusable grant.
type playbackRevalidationSnapshot struct {
	key      playbackProofKey
	sequence uint64
	source   PlaybackProof
}

type playbackRevalidationWaiter struct{ ctx context.Context }

// playbackRevalidationCall shares only one exact session generation's refresh.
// All mutable state and publication decisions use the proof cache mutex.
type playbackRevalidationCall struct {
	key      string
	done     chan struct{}
	ctx      context.Context
	cancel   context.CancelFunc
	guard    *playbackProofPublishGuard
	waiters  map[*playbackRevalidationWaiter]struct{}
	result   PlaybackProof
	reason   string
	finished bool
}

// revalidatePlaybackProof refreshes only a previously admitted live session.
// The original request remains the fallback, and the new proof has no intent.
func (gateway *Gateway) revalidatePlaybackProof(request *http.Request, principal embytoken.Principal, token string, info videoRequestInfo) (PlaybackProof, string, bool) {
	if gateway == nil || gateway.proofs == nil || token == "" || !revalidationRequestEligible(request, info) {
		return PlaybackProof{}, "", false
	}
	if device, present := singleBoundedQueryValue(request.URL.Query(), "DeviceId", maxProofDeviceIDBytes); present && device != principal.DeviceID {
		return PlaybackProof{}, "", false
	}
	call, waiter, owner := gateway.proofs.beginPlaybackRevalidation(request.Context(), principal, token, info)
	if call == nil {
		return PlaybackProof{}, "", false
	}
	defer gateway.proofs.leavePlaybackRevalidation(call, waiter)
	if owner {
		sharedRequest := request.Clone(call.ctx)
		go gateway.runPlaybackRevalidation(sharedRequest, principal, token, info, call)
	}
	select {
	case <-request.Context().Done():
		return PlaybackProof{}, "request_canceled", true
	case <-call.done:
		if request.Context().Err() != nil {
			return PlaybackProof{}, "request_canceled", true
		}
		return call.result, call.reason, true
	}
}

// revalidationRequestEligible restricts refresh to fixed original/static
// identity-only URLs; it does not widen the existing acceleration eligibility.
func revalidationRequestEligible(request *http.Request, info videoRequestInfo) bool {
	if request == nil || request.URL == nil || !info.Accelerated ||
		(request.Method != http.MethodGet && request.Method != http.MethodHead) {
		return false
	}
	file := strings.ToLower(videoPath(request.URL).StreamFileName)
	if file != "original.mp4" && file != "original.mkv" && file != "stream" && !strings.HasPrefix(file, "stream.") {
		return false
	}
	query, err := url.ParseQuery(request.URL.RawQuery)
	if err != nil {
		return false
	}
	for key := range query {
		if _, tokenKey := canonicalTokenQueryKey(key); tokenKey {
			continue
		}
		switch strings.ToLower(key) {
		case "mediasourceid", "playsessionid", "static":
			// inspectVideoRequest already validated these logical fields.
		case "deviceid":
			if _, ok := singleBoundedQueryValue(query, "DeviceId", maxProofDeviceIDBytes); !ok {
				return false
			}
		case "container":
			value, ok := singleBoundedQueryValue(query, "Container", maxProofContainerBytes)
			if !ok || !strings.EqualFold(value, info.Container) {
				return false
			}
		default:
			return false
		}
	}
	return true
}

// beginPlaybackRevalidation atomically joins a refresh or captures one exact
// live binding before creating a bounded publication owner. Missing evidence
// fails closed; both total owners and owners/waiters per mapping are bounded.
func (cache *playbackProofCache) beginPlaybackRevalidation(ctx context.Context, principal embytoken.Principal, token string, info videoRequestInfo) (*playbackRevalidationCall, *playbackRevalidationWaiter, bool) {
	if cache == nil || cache.now == nil || ctx == nil || ctx.Err() != nil || cache.maxEntries <= 0 || cache.ttl <= 0 {
		return nil, nil, false
	}
	cache.mu.Lock()
	defer cache.mu.Unlock()
	now := cache.now()
	cache.pruneExpiredLocked(now)
	key := playbackProofKey{principal.MappingID, info.ItemID, info.MediaSourceID, info.PlaySessionID}
	binding, exists := cache.sessions[key]
	if !exists || now.Before(binding.created) || !binding.until.After(now) || !playbackProofMatchesPrincipal(binding.source, principal) || binding.source.ClientName != principal.ClientName ||
		!strings.EqualFold(binding.source.Container, info.Container) {
		return nil, nil, false
	}
	if current, exists := cache.entries[key]; exists && playbackProofMatchesPrincipal(current, principal) &&
		!now.Before(current.AuthorizedAt) && current.ExpiresAt.After(now) && matchingIntentMedia(binding.source, current) {
		done := make(chan struct{})
		close(done)
		return &playbackRevalidationCall{done: done, result: current, finished: true}, nil, false
	}
	tokenHash := sha256.Sum256([]byte(token))
	flightKey := fmt.Sprintf("%s\x00%s\x00%s\x00%s\x00%d\x00%x", key.mappingID, key.itemID, key.mediaSourceID, key.playSessionID, binding.sequence, tokenHash)
	waiter := &playbackRevalidationWaiter{ctx: ctx}
	if call := cache.revalidations[flightKey]; call != nil {
		if len(call.waiters) >= maxPlaybackEntriesPerMapping {
			return nil, nil, false
		}
		call.waiters[waiter] = struct{}{}
		return call, waiter, false
	}
	mappingOwners := 0
	for guard := range cache.publications {
		if guard.mappingID == principal.MappingID {
			mappingOwners++
		}
	}
	if len(cache.publications) >= cache.maxEntries || mappingOwners >= maxPlaybackEntriesPerMapping {
		return nil, nil, false
	}
	shared, cancel := context.WithTimeout(context.WithoutCancel(ctx), onDemandPlaybackInfoTimeout)
	guard := &playbackProofPublishGuard{mappingID: principal.MappingID, itemID: info.ItemID, deviceID: principal.DeviceID,
		revalidation: &playbackRevalidationSnapshot{key: key, sequence: binding.sequence, source: binding.source}}
	call := &playbackRevalidationCall{key: flightKey, done: make(chan struct{}), ctx: shared, cancel: cancel, guard: guard,
		waiters: map[*playbackRevalidationWaiter]struct{}{waiter: {}}}
	if cache.revalidations == nil {
		cache.revalidations = make(map[string]*playbackRevalidationCall)
	}
	cache.revalidations[flightKey] = call
	cache.publications[guard] = struct{}{}
	return call, waiter, true
}

// leavePlaybackRevalidation cancels shared work only after its last waiter
// leaves. Old cleanup cannot delete a newer owner that reused the same key.
func (cache *playbackProofCache) leavePlaybackRevalidation(call *playbackRevalidationCall, waiter *playbackRevalidationWaiter) {
	if cache == nil || call == nil || waiter == nil {
		return
	}
	cache.mu.Lock()
	defer cache.mu.Unlock()
	delete(call.waiters, waiter)
	if !call.finished && len(call.waiters) == 0 {
		call.guard.invalidated = true
		call.cancel()
		if cache.revalidations[call.key] == call {
			delete(cache.revalidations, call.key)
		}
	}
}

// runPlaybackRevalidation fetches independently authorized media and always
// releases its owner, including cancellation and panics from injected I/O.
func (gateway *Gateway) runPlaybackRevalidation(request *http.Request, principal embytoken.Principal, token string, info videoRequestInfo, call *playbackRevalidationCall) {
	var selected PlaybackProof
	reason := "internal_failure"
	defer func() {
		if recover() != nil {
			selected, reason = PlaybackProof{}, "internal_failure"
		}
		gateway.proofs.finishPlaybackRevalidation(call, principal, selected, reason)
		if call.reason == "" {
			gateway.debugf("[PlaybackGateway] level=debug code=playback_info_revalidated requestId=%s itemRef=%s sessionRef=%s reasonCode=verified_live_session",
				diagnosticRequestID(request.Context()), diagnosticItemRef(principal, info.ItemID), diagnosticSessionRef(principal, info.PlaySessionID))
		}
	}()
	gateway.debugf("[PlaybackGateway] level=debug code=playback_info_revalidation_started requestId=%s itemRef=%s sessionRef=%s",
		diagnosticRequestID(request.Context()), diagnosticItemRef(principal, info.ItemID), diagnosticSessionRef(principal, info.PlaySessionID))
	_, proofs, fetchReason := gateway.fetchPlaybackInfo(request, principal, token, info.ItemID, info.MediaSourceID)
	reason = fetchReason
	if reason != "" {
		return
	}
	reason = "media_identity_mismatch"
	for _, proof := range proofs {
		if proof.MediaSourceID != info.MediaSourceID || !matchingIntentMedia(call.guard.revalidation.source, proof) ||
			!playbackProofMatchesPrincipal(proof, principal) || !validPlaybackProof(proof) ||
			(info.OriginalStream && !originalStreamProofMatchesRequest(request, proof, info.Container)) {
			continue
		}
		if device, present := singleBoundedQueryValue(request.URL.Query(), "DeviceId", maxProofDeviceIDBytes); present && device != proof.DeviceID {
			continue
		}
		selected, reason = proof, ""
		return
	}
}

// finishPlaybackRevalidation publishes under the same lock as waiter removal,
// stop and client replacement. It creates only the trusted original-session
// alias, never borrows a newer intent or replaces another session's proof.
func (cache *playbackProofCache) finishPlaybackRevalidation(call *playbackRevalidationCall, principal embytoken.Principal, fresh PlaybackProof, reason string) {
	cache.mu.Lock()
	defer cache.mu.Unlock()
	defer call.cancel()
	for waiter := range call.waiters {
		if waiter.ctx.Err() != nil {
			delete(call.waiters, waiter)
		}
	}
	if reason == "" && (len(call.waiters) == 0 || call.ctx.Err() != nil) {
		reason = "request_canceled"
	}
	snapshot := call.guard.revalidation
	if reason == "" {
		now := cache.now()
		binding, exists := cache.sessions[snapshot.key]
		_, registered := cache.publications[call.guard]
		current, replaced := cache.entries[snapshot.key]
		if !registered || call.guard.invalidated || !exists || binding.sequence != snapshot.sequence ||
			now.Before(binding.created) || !binding.until.After(now) || !playbackProofMatchesPrincipal(binding.source, principal) ||
			!matchingIntentMedia(snapshot.source, fresh) || !matchingIntentMedia(binding.source, fresh) ||
			(replaced && current.ExpiresAt.After(now)) {
			reason = "playback_info_superseded"
		} else {
			// Older whole-item responses would remove this exact-session alias.
			// Other fixed sessions publish independently; client observations
			// remain authoritative and already invalidate this owner on entry.
			for guard := range cache.publications {
				if !guard.client && guard.revalidation == nil && guard.mappingID == snapshot.key.mappingID &&
					guard.itemID == snapshot.key.itemID && guard.deviceID == principal.DeviceID {
					guard.invalidated = true
				}
			}
			fresh.PlaySessionID = snapshot.key.playSessionID
			fresh.intentOriginSession = ""
			fresh.transferIntent = false
			fresh.transferIntentUntil = time.Time{}
			cache.recordLocked([]PlaybackProof{fresh}, now)
			call.result = cache.entries[snapshot.key]
		}
	}
	call.reason, call.finished = reason, true
	delete(cache.publications, call.guard)
	if cache.revalidations[call.key] == call {
		delete(cache.revalidations, call.key)
	}
	close(call.done)
}
