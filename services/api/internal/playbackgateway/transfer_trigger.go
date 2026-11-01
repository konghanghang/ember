package playbackgateway

import (
	"context"
	"net/http"
	"time"

	"github.com/konghang/ember/backend/internal/config"
	"github.com/konghang/ember/backend/internal/services/directplay"
	"github.com/konghang/ember/backend/internal/services/embytoken"
)

const transferPolicyReadTimeout = 500 * time.Millisecond

// TransferTriggerPolicy supplies the administrator's database-backed mode and
// weak-GET sublimits. Unavailable policy never grants unbounded permission.
type TransferTriggerPolicy interface {
	P115TransferTriggerMode(context.Context) (string, error)
	P115CompatibilityTransferLimits(context.Context) (config.CompatibilityTransferLimits, error)
}

// authorizeVideoTransfer keeps the decision source and its sublimits in one
// locked-admission snapshot. Missing limits cannot silently uncap weak GETs.
func (gateway *Gateway) authorizeVideoTransfer(ctx context.Context, method string, proof PlaybackProof, principal embytoken.Principal) (directplay.TransferAuthorization, string) {
	if ctx == nil {
		return directplay.TransferAuthorization{}, "policy_denied"
	}
	readContext, cancel := context.WithTimeout(ctx, transferPolicyReadTimeout)
	defer cancel()
	allowed, trigger := gateway.canCreateVideoTransfer(readContext, method, proof, principal)
	if !allowed || trigger != "compatible_request" {
		return directplay.TransferAuthorization{Allowed: allowed}, trigger
	}
	limits, err := gateway.transferTriggerPolicy.P115CompatibilityTransferLimits(readContext)
	if err != nil || readContext.Err() != nil || !gateway.proofs.mediaProofIsCurrent(proof, principal) {
		gateway.debugf("[PlaybackGateway] level=debug code=transfer_policy_unavailable requestId=%s reasonCode=config_unavailable errorType=%T", diagnosticRequestID(ctx), err)
		return directplay.TransferAuthorization{}, "policy_denied"
	}
	return directplay.TransferAuthorization{Allowed: true, CompatibilityLimits: &directplay.CompatibilityTransferLimits{
		UserHourly: limits.UserHourly, UserDaily: limits.UserDaily, AccountHourly: limits.AccountHourly, AccountDaily: limits.AccountDaily,
	}}, trigger
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
	if gateway.proofs.hasConfirmedPlayback(proof, principal) {
		return true, "session_playback"
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
