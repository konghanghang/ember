package p115quota

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

// TestLeaseConfirmationContract runs the same expiry, identity and renewal
// contract against the in-memory implementation and the real Lua adapter.
func TestLeaseConfirmationContract(t *testing.T) {
	factories := map[string]func(*testing.T) LeaseStore{
		"memory": func(t *testing.T) LeaseStore { return NewMemoryLeaseStore() },
		"redis":  func(t *testing.T) LeaseStore { store, _, _ := newTestRedisLeaseStore(t); return store },
	}
	for name, factory := range factories {
		t.Run(name, func(t *testing.T) {
			for _, mode := range []string{"renew", "read only", "expired", "stopped", "identity", "active", "paused", "canceled", "reused reservation"} {
				t.Run(mode, func(t *testing.T) {
					store := factory(t)
					now := time.Date(2026, 9, 12, 0, 0, 0, 0, time.UTC)
					reserve := ReserveRequest{PlaybackAccountKey: strings.Repeat("a", 64), UserID: "user-1", SessionFingerprint: strings.Repeat("b", 64), MaxConcurrentStreams: 1}
					if _, err := store.Reserve(context.Background(), reserve, now); err != nil {
						t.Fatal(err)
					}
					request := ConfirmRequest{PlaybackAccountKey: reserve.PlaybackAccountKey, UserID: reserve.UserID, SessionFingerprint: reserve.SessionFingerprint, RenewReservation: true}
					at := now.Add(20 * time.Second)
					wantExpiry := at.Add(ReservationTTL)
					wantFound := true
					var wantErr error
					ctx := context.Background()
					switch mode {
					case "reused reservation":
						if result, err := store.Reserve(ctx, reserve, at); err != nil || result.Created {
							t.Fatalf("reuse=%+v error=%v", result, err)
						}
						request.RenewReservation = false
					case "read only":
						request.RenewReservation = false
						wantExpiry = now.Add(ReservationTTL)
					case "expired":
						at = now.Add(ReservationTTL)
						wantFound = false
					case "stopped":
						if _, err := store.Stop(ctx, reserve.SessionFingerprint, now.Add(time.Second)); err != nil {
							t.Fatal(err)
						}
						wantFound = false
					case "identity":
						request.UserID = "user-2"
						wantErr = ErrLeaseIdentityInvalid
					case "active", "paused":
						state, ttl := LeaseStateActive, ActiveTTL
						if mode == "paused" {
							state, ttl = LeaseStatePaused, PausedTTL
						}
						if _, err := store.Advance(ctx, reserve.SessionFingerprint, state, now.Add(time.Second)); err != nil {
							t.Fatal(err)
						}
						wantExpiry = now.Add(time.Second).Add(ttl)
					case "canceled":
						canceled, cancel := context.WithCancel(ctx)
						cancel()
						ctx = canceled
						wantErr = context.Canceled
					}
					result, err := store.Confirm(ctx, request, at)
					if !errors.Is(err, wantErr) {
						t.Fatalf("Confirm error=%v want=%v", err, wantErr)
					}
					if wantErr != nil {
						return
					}
					if result.Found != wantFound {
						t.Fatalf("Confirm found=%t want=%t", result.Found, wantFound)
					}
					if !wantFound {
						if _, found, err := store.Session(ctx, request.SessionFingerprint, at); err != nil || found {
							t.Fatalf("missing lease recreated: found=%t err=%v", found, err)
						}
						return
					}
					if !result.Session.ExpiresAt.Equal(wantExpiry) || result.Account.OccupiedStreams != 1 || result.User.OccupiedStreams != 1 {
						t.Fatalf("confirmation=%+v expiry=%s", result, wantExpiry)
					}
					if mode == "renew" {
						if _, found, err := store.Session(ctx, request.SessionFingerprint, now.Add(35*time.Second)); err != nil || !found {
							t.Fatalf("renewal not persisted: found=%t err=%v", found, err)
						}
					}
				})
			}
		})
	}
}
