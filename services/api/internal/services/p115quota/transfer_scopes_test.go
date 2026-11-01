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

// TestTransferScopesAtomicAdmission checks the production Lua and memory fake
// against the same total/user/account limits, including concurrent users.
func TestTransferScopesAtomicAdmission(t *testing.T) {
	for _, backend := range []string{"memory", "redis"} {
		t.Run(backend, func(t *testing.T) {
			var store TransferQuotaStore = NewMemoryTransferQuotaStore()
			if backend == "redis" {
				redisStore, _, _ := newTestRedisLeaseStore(t)
				store = redisStore
			}
			now := time.Date(2026, 11, 1, 4, 0, 0, 0, time.FixedZone("business", 8*3600))
			start, end := DayWindow(now, now.Location())
			var admitted atomic.Int32
			var wg sync.WaitGroup
			for i := 0; i < 12; i++ {
				wg.Add(1)
				go func(i int) {
					defer wg.Done()
					request := TransferReserveRequest{UserID: fmt.Sprintf("user-%d", i), AttemptID: fmt.Sprintf("attempt-%d", i), HourlyLimit: 5, DailyLimit: 10, DayStart: start, DayEnd: end,
						AdditionalScopes: []TransferQuotaScope{{ID: "compat-account-fixture", HourlyLimit: 2, DailyLimit: 3}}}
					_, err := store.ReserveTransfer(context.Background(), request, now)
					if err == nil {
						admitted.Add(1)
					} else if !errors.Is(err, ErrCompatibilityQuotaExceeded) {
						t.Errorf("reserve: %v", err)
					}
				}(i)
			}
			wg.Wait()
			if admitted.Load() != 2 {
				t.Fatalf("admitted=%d, want account sublimit 2", admitted.Load())
			}
		})
	}
}

// TestTransferScopesCommitAndRefund ensures sublimits count the same opaque
// attempt exactly once, survive late success, and do not consume denied totals.
func TestTransferScopesCommitAndRefund(t *testing.T) {
	for _, backend := range []string{"memory", "redis"} {
		t.Run(backend, func(t *testing.T) {
			var store TransferQuotaStore = NewMemoryTransferQuotaStore()
			if backend == "redis" {
				r, _, _ := newTestRedisLeaseStore(t)
				store = r
			}
			now := time.Date(2026, 11, 1, 12, 0, 0, 0, time.FixedZone("business", 8*3600))
			start, end := DayWindow(now, now.Location())
			scopes := []TransferQuotaScope{{ID: "compat-user-fixture", HourlyLimit: 1, DailyLimit: 2}, {ID: "compat-account-fixture", HourlyLimit: 2, DailyLimit: 3}}
			request := transferRequest("scope-attempt", start, end)
			request.AdditionalScopes = scopes
			if _, err := store.ReserveTransfer(context.Background(), request, now); err != nil {
				t.Fatal(err)
			}
			denied := request
			denied.AttemptID = "scope-denied"
			if _, err := store.ReserveTransfer(context.Background(), denied, now); !errors.Is(err, ErrCompatibilityQuotaExceeded) {
				t.Fatalf("sublimit error=%v", err)
			}
			usage, _ := store.TransferUsage(context.Background(), TransferUsageRequest{UserID: request.UserID, DayStart: start, DayEnd: end}, now)
			if usage.Pending != 1 {
				t.Fatalf("denied request consumed total: %+v", usage)
			}
			commit := TransferCommitRequest{UserID: request.UserID, AttemptID: request.AttemptID, DayStart: start, DayEnd: end, AdditionalScopes: scopes}
			for i := 0; i < 2; i++ {
				if _, err := store.CommitTransfer(context.Background(), commit, now.Add(TransferPendingTTL+time.Second)); err != nil {
					t.Fatal(err)
				}
			}
			for _, id := range []string{request.UserID, scopes[0].ID, scopes[1].ID} {
				usage, err := store.TransferUsage(context.Background(), TransferUsageRequest{UserID: id, DayStart: start, DayEnd: end}, now.Add(TransferPendingTTL+time.Second))
				if err != nil || usage.Pending != 0 || usage.HourlyUsed != 1 || usage.DailyUsed != 1 {
					t.Fatalf("scope %s: %+v, %v", id, usage, err)
				}
			}
			// Explicit playback still has the remaining total allowance.
			explicit := transferRequest("explicit-attempt", start, end)
			if _, err := store.ReserveTransfer(context.Background(), explicit, now.Add(TransferPendingTTL+time.Second)); err != nil {
				t.Fatalf("explicit blocked by sublimit: %v", err)
			}
		})
	}
}

// TestTransferScopesValidateAndResetAtBusinessDay covers malformed internal
// scopes and independent rolling-hour/natural-day expiry in both stores.
func TestTransferScopesValidateAndResetAtBusinessDay(t *testing.T) {
	for _, backend := range []string{"memory", "redis"} {
		t.Run(backend, func(t *testing.T) {
			var store TransferQuotaStore = NewMemoryTransferQuotaStore()
			if backend == "redis" {
				r, _, _ := newTestRedisLeaseStore(t)
				store = r
			}
			now := time.Date(2026, 11, 1, 21, 0, 0, 0, time.FixedZone("business", 8*3600))
			start, end := DayWindow(now, now.Location())
			base := transferRequest("scope-validate", start, end)
			for _, scopes := range [][]TransferQuotaScope{
				{{ID: base.UserID, HourlyLimit: 1, DailyLimit: 1}},
				{{ID: "bad-scope", HourlyLimit: 0, DailyLimit: 1}},
				{{ID: "duplicate", HourlyLimit: 1, DailyLimit: 1}, {ID: "duplicate", HourlyLimit: 1, DailyLimit: 1}},
			} {
				base.AdditionalScopes = scopes
				if _, err := store.ReserveTransfer(context.Background(), base, now); err == nil {
					t.Fatal("invalid scope accepted")
				}
			}
			base.AdditionalScopes = []TransferQuotaScope{{ID: "compat-day", HourlyLimit: 1, DailyLimit: 1}}
			if _, err := store.ReserveTransfer(context.Background(), base, now); err != nil {
				t.Fatal(err)
			}
			commit := TransferCommitRequest{UserID: base.UserID, AttemptID: base.AttemptID, DayStart: start, DayEnd: end, AdditionalScopes: base.AdditionalScopes}
			if _, err := store.CommitTransfer(context.Background(), commit, now); err != nil {
				t.Fatal(err)
			}
			base.AttemptID = "later-day"
			if _, err := store.ReserveTransfer(context.Background(), base, now.Add(2*time.Hour)); !errors.Is(err, ErrCompatibilityQuotaExceeded) {
				t.Fatalf("daily limit lost after hour: %v", err)
			}
			next := now.Add(4 * time.Hour)
			base.DayStart, base.DayEnd = DayWindow(next, now.Location())
			if _, err := store.ReserveTransfer(context.Background(), base, next); err != nil {
				t.Fatalf("business day did not reset: %v", err)
			}
		})
	}
}
