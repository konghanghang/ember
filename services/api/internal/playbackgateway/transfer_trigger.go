package playbackgateway

import (
	"context"
	"net/http"
	"time"

	"github.com/konghang/ember/backend/internal/config"
	"github.com/konghang/ember/backend/internal/services/embytoken"
)

const transferPolicyReadTimeout = 500 * time.Millisecond

// TransferTriggerPolicy supplies only the administrator's database-backed
// transfer mode. Missing or unavailable policy never grants extra permission.
type TransferTriggerPolicy interface {
	P115TransferTriggerMode(context.Context) (string, error)
}

// canCreateVideoTransfer runs at the existing locked transfer admission point.
// Compatibility authorizes one current GET, never creates explicit playback
// intent, and rechecks proof replacement/cancellation after configuration I/O.
func (gateway *Gateway) canCreateVideoTransfer(ctx context.Context, method string, proof PlaybackProof, principal embytoken.Principal) (bool, string) {
	const denied = "policy_denied"
	if gateway == nil || ctx == nil || ctx.Err() != nil || method != http.MethodGet {
		return false, denied
	}
	if gateway.proofs.CanCreateTransfer(proof, principal) {
		return true, "explicit_playback"
	}
	if gateway.transferTriggerPolicy == nil || !gateway.proofs.mediaProofIsCurrent(proof, principal) {
		return false, denied
	}
	readContext, cancel := context.WithTimeout(ctx, transferPolicyReadTimeout)
	defer cancel()
	mode, err := gateway.transferTriggerPolicy.P115TransferTriggerMode(readContext)
	if err != nil || readContext.Err() != nil {
		gateway.debugf("[PlaybackGateway] level=debug code=transfer_policy_unavailable requestId=%s reasonCode=config_unavailable errorType=%T", diagnosticRequestID(ctx), err)
		return false, denied
	}
	if mode != config.TransferTriggerModeCompatible {
		return false, denied
	}
	if ctx.Err() != nil || !gateway.proofs.mediaProofIsCurrent(proof, principal) {
		return false, denied
	}
	return true, "compatible_request"
}

// mediaProofIsCurrent validates the exact generation without requiring an
// explicit transfer permit; it is used only after normal video eligibility.
func (cache *playbackProofCache) mediaProofIsCurrent(snapshot PlaybackProof, principal embytoken.Principal) bool {
	if cache == nil || cache.now == nil || snapshot.generation == 0 || !playbackProofMatchesPrincipal(snapshot, principal) {
		return false
	}
	cache.mu.Lock()
	defer cache.mu.Unlock()
	current, ok := cache.entries[playbackProofKey{snapshot.MappingID, snapshot.ItemID, snapshot.MediaSourceID, snapshot.PlaySessionID}]
	now := cache.now()
	return ok && current.generation == snapshot.generation && matchingIntentMedia(snapshot, current) &&
		playbackProofMatchesPrincipal(current, principal) && !now.Before(current.AuthorizedAt) && current.ExpiresAt.After(now)
}
