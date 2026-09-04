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
	if gateway == nil || gateway.playbackSessionService == nil || event.snapshotState != "recorded" {
		return
	}
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
			"[PlaybackGateway] level=warn code=playback_lease_update_failed event=%s reasonCode=%s errorType=%T",
			event.kind, reasonCode, err,
		)
		return
	}
	if !result.Found {
		gateway.debugf("[PlaybackGateway] level=debug code=playback_lease_not_found event=%s", event.kind)
	}
}
