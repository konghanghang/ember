package playbackgateway

import (
	"context"
	"errors"
	"time"

	"github.com/konghang/ember/backend/internal/services/directplay"
	"github.com/konghang/ember/backend/internal/services/embytoken"
)

// updatePlaybackSessionLease mirrors only successfully forwarded Emby events
// into an existing 115 reverse session. Persistence has its own two-second
// budget so a client disconnect cannot prevent a successful Stopped release.
// Failures never alter the Emby response.
func (gateway *Gateway) updatePlaybackSessionLease(
	ctx context.Context,
	principal embytoken.Principal,
	event playbackSessionEventSnapshot,
) {
	if gateway == nil {
		return
	}
	requestID := diagnosticRequestID(ctx)
	sessionRef := diagnosticSessionRef(principal, event.playSessionID)
	if event.kind == playbackSessionEventStop && event.snapshotState == "recorded" && event.intentRevocationEligible {
		revoked := gateway.proofs.RevokeTransferIntent(principal, event.itemID, event.mediaSourceID, event.playSessionID)
		gateway.proofs.stopPlaybackBinding(principal, event.itemID, event.mediaSourceID, event.playSessionID)
		gateway.debugf("[PlaybackGateway] level=debug code=playback_transfer_intent_revoked message=\"播放停止，撤销新增转存许可\" requestId=%s sessionRef=%s reasonCode=playback_stopped count=%d", requestID, sessionRef, revoked)
	}
	if event.snapshotState == "recorded" && event.kind != playbackSessionEventStop {
		if gateway.proofs.observeFallbackPlayback(principal, event) {
			gateway.debugf("[PlaybackGateway] level=debug code=playback_activity_observed requestId=%s sessionRef=%s event=%s itemId=%q", requestID, sessionRef, event.kind, event.itemID)
		}
	}
	if gateway.playbackSessionService == nil || event.snapshotState != "recorded" {
		reason := "snapshot_unavailable"
		if gateway.playbackSessionService == nil {
			reason = "service_unavailable"
		}
		gateway.debugf("[PlaybackGateway] level=debug code=playback_lease_skipped requestId=%s sessionRef=%s event=%s reasonCode=%s snapshotState=%s", requestID, sessionRef, event.kind, reason, event.snapshotState)
		return
	}
	gateway.debugf("[PlaybackGateway] level=debug code=playback_lease_update_started requestId=%s sessionRef=%s event=%s itemId=%q", requestID, sessionRef, event.kind, event.itemID)
	updateContext, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
	defer cancel()
	result, err := gateway.playbackSessionService.HandlePlaybackSessionEvent(updateContext, directplay.PlaybackSessionEvent{
		UserID: principal.User.ID, MappingID: principal.MappingID, DeviceID: principal.DeviceID,
		PlaySessionID: event.playSessionID,
		IsProgress:    event.kind == playbackSessionEventProgress,
		IsPaused:      event.kind == playbackSessionEventProgress && event.isPausedPresent && event.isPaused,
		Stopped:       event.kind == playbackSessionEventStop,
	})
	if err != nil {
		reasonCode := "lease_update_failed"
		if errors.Is(err, directplay.ErrRedisUnavailable) {
			reasonCode = "redis_unavailable"
		} else if errors.Is(err, directplay.ErrInvalidRequest) {
			reasonCode = "session_identity_invalid"
		}
		gateway.logger.Printf(
			"[PlaybackGateway] level=warn code=playback_lease_update_failed event=%s reasonCode=%s errorType=%T requestId=%s sessionRef=%s itemId=%q",
			event.kind, reasonCode, err, requestID, sessionRef, event.itemID,
		)
		return
	}
	if !result.Found {
		gateway.proofs.updatePlaybackBinding(principal, event.itemID, event.mediaSourceID, event.playSessionID, result.State, false)
		gateway.debugf("[PlaybackGateway] level=debug code=playback_lease_not_found event=%s requestId=%s sessionRef=%s itemId=%q", event.kind, requestID, sessionRef, event.itemID)
		return
	}
	if event.kind != playbackSessionEventStop {
		gateway.proofs.updatePlaybackBinding(principal, event.itemID, event.mediaSourceID, event.playSessionID, result.State, true)
	}
	gateway.debugf("[PlaybackGateway] level=debug code=playback_lease_updated requestId=%s sessionRef=%s event=%s found=true state=%s accountReservedStreams=%d accountActiveStreams=%d accountOccupiedStreams=%d userReservedStreams=%d userActiveStreams=%d userOccupiedStreams=%d",
		requestID, sessionRef, event.kind, result.State, result.Account.ReservedStreams, result.Account.ActiveStreams, result.Account.OccupiedStreams,
		result.User.ReservedStreams, result.User.ActiveStreams, result.User.OccupiedStreams)
}
