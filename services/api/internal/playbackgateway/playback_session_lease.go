package playbackgateway

import (
	"context"
	"errors"

	"github.com/konghang/ember/backend/internal/services/directplay"
	"github.com/konghang/ember/backend/internal/services/embytoken"
)

// updatePlaybackSessionLease mirrors only successfully forwarded Emby events
// into an existing 115 reverse session. Failures never alter the Emby response.
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
		gateway.debugf("[PlaybackGateway] level=debug code=playback_transfer_intent_revoked message=\"播放停止，撤销新增转存许可\" requestId=%s sessionRef=%s reasonCode=playback_stopped count=%d", requestID, sessionRef, revoked)
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
	result, err := gateway.playbackSessionService.HandlePlaybackSessionEvent(ctx, directplay.PlaybackSessionEvent{
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
		gateway.debugf("[PlaybackGateway] level=debug code=playback_lease_not_found event=%s requestId=%s sessionRef=%s itemId=%q", event.kind, requestID, sessionRef, event.itemID)
		return
	}
	gateway.debugf("[PlaybackGateway] level=debug code=playback_lease_updated requestId=%s sessionRef=%s event=%s found=true state=%s accountReservedStreams=%d accountActiveStreams=%d accountOccupiedStreams=%d userReservedStreams=%d userActiveStreams=%d userOccupiedStreams=%d",
		requestID, sessionRef, event.kind, result.State, result.Account.ReservedStreams, result.Account.ActiveStreams, result.Account.OccupiedStreams,
		result.User.ReservedStreams, result.User.ActiveStreams, result.User.OccupiedStreams)
}
