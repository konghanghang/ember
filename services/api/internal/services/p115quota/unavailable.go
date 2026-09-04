package p115quota

import (
	"context"
	"time"
)

// UnavailableLeaseStore keeps Gateway proxying available when REDIS_URL is
// absent or unusable while failing every 115 admission closed.
type UnavailableLeaseStore struct{}

func (UnavailableLeaseStore) Reserve(context.Context, ReserveRequest, time.Time) (ReserveResult, error) {
	return ReserveResult{}, ErrRedisUnavailable
}

func (UnavailableLeaseStore) Session(context.Context, string, time.Time) (LeaseSession, bool, error) {
	return LeaseSession{}, false, ErrRedisUnavailable
}

func (UnavailableLeaseStore) Advance(context.Context, string, LeaseState, time.Time) (TransitionResult, error) {
	return TransitionResult{}, ErrRedisUnavailable
}

func (UnavailableLeaseStore) ReleaseReservation(context.Context, string, time.Time) (bool, error) {
	return false, ErrRedisUnavailable
}

func (UnavailableLeaseStore) Stop(context.Context, string, time.Time) (TransitionResult, error) {
	return TransitionResult{}, ErrRedisUnavailable
}

func (UnavailableLeaseStore) AccountUsage(context.Context, string, time.Time) (LeaseUsage, error) {
	return LeaseUsage{}, ErrRedisUnavailable
}

func (UnavailableLeaseStore) UserUsage(context.Context, string, time.Time) (LeaseUsage, error) {
	return LeaseUsage{}, ErrRedisUnavailable
}

func (UnavailableLeaseStore) ReserveTransfer(context.Context, TransferReserveRequest, time.Time) (TransferReserveResult, error) {
	return TransferReserveResult{}, ErrRedisUnavailable
}

func (UnavailableLeaseStore) ReleaseTransfer(context.Context, string, string, time.Time) (bool, error) {
	return false, ErrRedisUnavailable
}

func (UnavailableLeaseStore) CommitTransfer(context.Context, TransferCommitRequest, time.Time) (TransferCommitResult, error) {
	return TransferCommitResult{}, ErrRedisUnavailable
}

func (UnavailableLeaseStore) TransferUsage(context.Context, TransferUsageRequest, time.Time) (TransferUsage, error) {
	return TransferUsage{}, ErrRedisUnavailable
}
