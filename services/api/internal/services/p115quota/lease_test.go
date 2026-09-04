package p115quota

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestMemoryLeaseStoreReservationIsAtomicAndIdempotent(t *testing.T) {
	store := NewMemoryLeaseStore()
	now := time.Date(2026, 9, 4, 0, 0, 0, 0, time.UTC)

	var admitted atomic.Int32
	admittedFingerprints := make(chan string, 12)
	var wait sync.WaitGroup
	for index := 0; index < 12; index++ {
		wait.Add(1)
		go func(index int) {
			defer wait.Done()
			fingerprint := fmt.Sprintf("%064x", index+1)
			_, err := store.Reserve(context.Background(), ReserveRequest{
				PlaybackAccountKey:   "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
				UserID:               "user-1",
				SessionFingerprint:   fingerprint,
				MaxConcurrentStreams: 2,
			}, now)
			if err == nil {
				admitted.Add(1)
				admittedFingerprints <- fingerprint
				return
			}
			if !errors.Is(err, ErrAccountConcurrencyExceeded) {
				t.Errorf("Reserve() error = %v", err)
			}
		}(index)
	}
	wait.Wait()
	close(admittedFingerprints)

	if admitted.Load() != 2 {
		t.Fatalf("admitted = %d, want 2", admitted.Load())
	}
	usage, err := store.AccountUsage(context.Background(), "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", now)
	if err != nil {
		t.Fatalf("AccountUsage() error = %v", err)
	}
	if usage.ReservedStreams != 2 || usage.ActiveStreams != 0 || usage.OccupiedStreams != 2 {
		t.Fatalf("usage = %+v", usage)
	}

	admittedFingerprint := <-admittedFingerprints
	duplicate, err := store.Reserve(context.Background(), ReserveRequest{
		PlaybackAccountKey:   "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		UserID:               "user-1",
		SessionFingerprint:   admittedFingerprint,
		MaxConcurrentStreams: 2,
	}, now.Add(time.Second))
	if err != nil || duplicate.Created || duplicate.State != LeaseStateReservation || duplicate.Account.OccupiedStreams != 2 {
		t.Fatalf("duplicate reservation = %+v, %v", duplicate, err)
	}
}

func TestMemoryLeaseStoreLimitResultIncludesAtomicUsage(t *testing.T) {
	store := NewMemoryLeaseStore()
	now := time.Date(2026, 9, 4, 0, 0, 0, 0, time.UTC)
	accountKey := "abababababababababababababababababababababababababababababababab"
	first := ReserveRequest{
		PlaybackAccountKey:   accountKey,
		UserID:               "user-limit",
		SessionFingerprint:   "cdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcd",
		MaxConcurrentStreams: 1,
	}
	if _, err := store.Reserve(context.Background(), first, now); err != nil {
		t.Fatalf("Reserve(first) error = %v", err)
	}

	second := first
	second.SessionFingerprint = "efefefefefefefefefefefefefefefefefefefefefefefefefefefefefefefef"
	result, err := store.Reserve(context.Background(), second, now)
	if !errors.Is(err, ErrAccountConcurrencyExceeded) {
		t.Fatalf("Reserve(limit) error = %v", err)
	}
	if result.Account != (LeaseUsage{ReservedStreams: 1, OccupiedStreams: 1}) ||
		result.User != (LeaseUsage{ReservedStreams: 1, OccupiedStreams: 1}) {
		t.Fatalf("Reserve(limit) result = %+v", result)
	}
}

func TestMemoryLeaseStorePromotesPausesStopsAndExpiresSessions(t *testing.T) {
	store := NewMemoryLeaseStore()
	now := time.Date(2026, 9, 4, 0, 0, 0, 0, time.UTC)
	request := ReserveRequest{
		PlaybackAccountKey:   "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		UserID:               "user-2",
		SessionFingerprint:   "cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc",
		MaxConcurrentStreams: 1,
	}
	if _, err := store.Reserve(context.Background(), request, now); err != nil {
		t.Fatalf("Reserve() error = %v", err)
	}

	active, err := store.Advance(context.Background(), request.SessionFingerprint, LeaseStateActive, now.Add(time.Second))
	if err != nil || !active.Found || active.PreviousState != LeaseStateReservation || active.State != LeaseStateActive {
		t.Fatalf("active advance = %+v, %v", active, err)
	}
	if active.Account.ReservedStreams != 0 || active.Account.ActiveStreams != 1 || active.Account.OccupiedStreams != 1 {
		t.Fatalf("active usage = %+v", active.Account)
	}

	pausedAt := now.Add(2 * time.Second)
	paused, err := store.Advance(context.Background(), request.SessionFingerprint, LeaseStatePaused, pausedAt)
	if err != nil || paused.State != LeaseStatePaused || paused.Account.ActiveStreams != 1 {
		t.Fatalf("paused advance = %+v, %v", paused, err)
	}
	if session, found, err := store.Session(context.Background(), request.SessionFingerprint, pausedAt.Add(PausedTTL-time.Second)); err != nil || !found || session.State != LeaseStatePaused {
		t.Fatalf("paused session = %+v found=%t err=%v", session, found, err)
	}
	if _, found, err := store.Session(context.Background(), request.SessionFingerprint, pausedAt.Add(PausedTTL+time.Millisecond)); err != nil || found {
		t.Fatalf("expired session found=%t err=%v", found, err)
	}

	if _, err := store.Reserve(context.Background(), request, pausedAt.Add(PausedTTL+time.Second)); err != nil {
		t.Fatalf("Reserve(after expiry) error = %v", err)
	}
	stopped, err := store.Stop(context.Background(), request.SessionFingerprint, pausedAt.Add(PausedTTL+2*time.Second))
	if err != nil || !stopped.Found || stopped.Account.OccupiedStreams != 0 || stopped.User.OccupiedStreams != 0 {
		t.Fatalf("Stop() = %+v, %v", stopped, err)
	}
}

func TestMemoryLeaseStoreReleaseReservationDoesNotDeleteActiveSession(t *testing.T) {
	store := NewMemoryLeaseStore()
	now := time.Date(2026, 9, 4, 0, 0, 0, 0, time.UTC)
	request := ReserveRequest{
		PlaybackAccountKey:   "dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd",
		UserID:               "user-3",
		SessionFingerprint:   "eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee",
		MaxConcurrentStreams: 1,
	}
	if _, err := store.Reserve(context.Background(), request, now); err != nil {
		t.Fatalf("Reserve() error = %v", err)
	}
	if _, err := store.Advance(context.Background(), request.SessionFingerprint, LeaseStateActive, now.Add(time.Second)); err != nil {
		t.Fatalf("Advance() error = %v", err)
	}
	released, err := store.ReleaseReservation(context.Background(), request.SessionFingerprint, now.Add(2*time.Second))
	if err != nil || released {
		t.Fatalf("ReleaseReservation(active) = %t, %v", released, err)
	}
	usage, err := store.UserUsage(context.Background(), request.UserID, now.Add(2*time.Second))
	if err != nil || usage.ActiveStreams != 1 || usage.OccupiedStreams != 1 {
		t.Fatalf("UserUsage() = %+v, %v", usage, err)
	}
}

func TestMemoryLeaseStoreRejectsInvalidAndCanceledRequests(t *testing.T) {
	store := NewMemoryLeaseStore()
	now := time.Now().UTC()
	if _, err := store.Reserve(context.Background(), ReserveRequest{}, now); !errors.Is(err, ErrLeaseIdentityInvalid) {
		t.Fatalf("Reserve(invalid) error = %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := store.AccountUsage(ctx, "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", now); !errors.Is(err, context.Canceled) {
		t.Fatalf("AccountUsage(canceled) error = %v", err)
	}
}
