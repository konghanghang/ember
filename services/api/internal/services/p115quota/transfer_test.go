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

func transferRequest(attemptID string, dayStart, dayEnd time.Time) TransferReserveRequest {
	return TransferReserveRequest{
		UserID: "user-1", AttemptID: attemptID,
		HourlyLimit: 5, DailyLimit: 10, DayStart: dayStart, DayEnd: dayEnd,
	}
}

func TestMemoryTransferQuotaStorePreventsConcurrentPendingPenetration(t *testing.T) {
	store := NewMemoryTransferQuotaStore()
	now := time.Date(2026, 9, 4, 12, 0, 0, 0, time.UTC)
	dayStart, dayEnd := DayWindow(now, time.UTC)
	var admitted atomic.Int32
	var wait sync.WaitGroup
	for index := 0; index < 20; index++ {
		wait.Add(1)
		go func(index int) {
			defer wait.Done()
			_, err := store.ReserveTransfer(context.Background(), transferRequest(fmt.Sprintf("attempt-%02d", index), dayStart, dayEnd), now)
			if err == nil {
				admitted.Add(1)
				return
			}
			if !errors.Is(err, ErrTransferQuotaExceeded) {
				t.Errorf("ReserveTransfer() error = %v", err)
			}
		}(index)
	}
	wait.Wait()
	if admitted.Load() != 5 {
		t.Fatalf("admitted = %d, want 5", admitted.Load())
	}
	usage, err := store.TransferUsage(context.Background(), TransferUsageRequest{UserID: "user-1", DayStart: dayStart, DayEnd: dayEnd}, now)
	if err != nil || usage.Pending != 5 || usage.HourlyUsed != 0 || usage.DailyUsed != 0 {
		t.Fatalf("TransferUsage() = %+v, %v", usage, err)
	}
}

func TestMemoryTransferQuotaStoreCommitsIdempotentlyAndDetectsLateSuccess(t *testing.T) {
	store := NewMemoryTransferQuotaStore()
	now := time.Date(2026, 9, 4, 12, 0, 0, 0, time.UTC)
	dayStart, dayEnd := DayWindow(now, time.UTC)
	request := transferRequest("attempt-1", dayStart, dayEnd)
	if _, err := store.ReserveTransfer(context.Background(), request, now); err != nil {
		t.Fatalf("ReserveTransfer() error = %v", err)
	}
	committed, err := store.CommitTransfer(context.Background(), TransferCommitRequest{
		UserID: request.UserID, AttemptID: request.AttemptID, DayStart: dayStart, DayEnd: dayEnd,
	}, now.Add(time.Second))
	if err != nil || !committed.Added || committed.PendingExpiredBeforeCommit || committed.Usage.HourlyUsed != 1 || committed.Usage.DailyUsed != 1 {
		t.Fatalf("CommitTransfer() = %+v, %v", committed, err)
	}
	repeated, err := store.CommitTransfer(context.Background(), TransferCommitRequest{
		UserID: request.UserID, AttemptID: request.AttemptID, DayStart: dayStart, DayEnd: dayEnd,
	}, now.Add(2*time.Second))
	if err != nil || repeated.Added || repeated.PendingExpiredBeforeCommit || repeated.Usage.HourlyUsed != 1 {
		t.Fatalf("CommitTransfer(repeated) = %+v, %v", repeated, err)
	}

	lateRequest := transferRequest("attempt-late", dayStart, dayEnd)
	if _, err := store.ReserveTransfer(context.Background(), lateRequest, now); err != nil {
		t.Fatalf("ReserveTransfer(late) error = %v", err)
	}
	lateAt := now.Add(TransferPendingTTL + time.Second)
	late, err := store.CommitTransfer(context.Background(), TransferCommitRequest{
		UserID: lateRequest.UserID, AttemptID: lateRequest.AttemptID, DayStart: dayStart, DayEnd: dayEnd,
	}, lateAt)
	if err != nil || !late.Added || !late.PendingExpiredBeforeCommit || late.Usage.HourlyUsed != 2 {
		t.Fatalf("CommitTransfer(late) = %+v, %v", late, err)
	}
}

func TestMemoryTransferQuotaStoreReleasesFailuresAndUsesRollingHour(t *testing.T) {
	store := NewMemoryTransferQuotaStore()
	now := time.Date(2026, 9, 4, 12, 0, 0, 0, time.UTC)
	dayStart, dayEnd := DayWindow(now, time.UTC)
	request := transferRequest("attempt-1", dayStart, dayEnd)
	if _, err := store.ReserveTransfer(context.Background(), request, now); err != nil {
		t.Fatal(err)
	}
	if released, err := store.ReleaseTransfer(context.Background(), request.UserID, request.AttemptID, now.Add(time.Second)); err != nil || !released {
		t.Fatalf("ReleaseTransfer() = %t, %v", released, err)
	}
	if _, err := store.ReserveTransfer(context.Background(), request, now.Add(2*time.Second)); err != nil {
		t.Fatalf("ReserveTransfer(after release) error = %v", err)
	}
	if _, err := store.CommitTransfer(context.Background(), TransferCommitRequest{
		UserID: request.UserID, AttemptID: request.AttemptID, DayStart: dayStart, DayEnd: dayEnd,
	}, now.Add(3*time.Second)); err != nil {
		t.Fatal(err)
	}
	usage, err := store.TransferUsage(context.Background(), TransferUsageRequest{UserID: request.UserID, DayStart: dayStart, DayEnd: dayEnd}, now.Add(time.Hour+4*time.Second))
	if err != nil || usage.HourlyUsed != 0 || usage.DailyUsed != 1 {
		t.Fatalf("rolling usage = %+v, %v", usage, err)
	}
}

func TestDayWindowUsesConfiguredTimezoneAcrossDST(t *testing.T) {
	location, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 3, 8, 12, 0, 0, 0, location)
	start, end := DayWindow(now, location)
	if start.Hour() != 0 || end.Hour() != 0 || end.Sub(start) != 23*time.Hour {
		t.Fatalf("window = %s..%s duration=%s", start, end, end.Sub(start))
	}
}
