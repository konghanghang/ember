package directplay

import (
	"context"
	"errors"
	"time"

	"github.com/konghang/ember/backend/internal/services/p115quota"
)

const (
	playbackResolveTimeout         = 2 * time.Minute
	playbackLeaseHeartbeatInterval = 10 * time.Second
)

// maintainPlaybackLease renews only live GET reservations while candidate work
// runs. Losing Redis or the lease cancels that work; finish joins the goroutine
// before cleanup and never promotes playback to active.
func (service *Service) maintainPlaybackLease(ctx context.Context, request p115quota.ConfirmRequest) (context.Context, func() error) {
	workCtx, cancelWork := context.WithCancel(ctx)
	heartbeatCtx, cancelHeartbeat := context.WithCancel(ctx)
	done := make(chan error, 1)
	interval := service.leaseHeartbeatInterval
	if interval <= 0 {
		interval = playbackLeaseHeartbeatInterval
	}
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-heartbeatCtx.Done():
				done <- nil
				return
			case <-ticker.C:
				result, err := service.leases.Confirm(heartbeatCtx, request, service.now().UTC())
				if heartbeatCtx.Err() != nil {
					done <- nil
					return
				}
				if err == nil && result.Found {
					continue
				}
				failure := ErrPlaybackLeaseLost
				if err != nil {
					failure = mapConfirmationError(err)
				}
				cancelWork()
				done <- failure
				return
			}
		}
	}()
	return workCtx, func() error { cancelHeartbeat(); err := <-done; cancelWork(); return err }
}

// mapConfirmationError keeps lease identity changes separate from Redis outages.
func mapConfirmationError(err error) error {
	if errors.Is(err, p115quota.ErrLeaseIdentityInvalid) {
		return ErrPlaybackRouteChanged
	}
	return mapLeaseError(err)
}
