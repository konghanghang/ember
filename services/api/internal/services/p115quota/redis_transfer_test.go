package p115quota

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestRedisTransferQuotaStoreEnforcesPendingAndSuccessfulUsage(t *testing.T) {
	store, server, _ := newTestRedisLeaseStore(t)
	now := time.Date(2026, 9, 4, 12, 0, 0, 0, time.UTC)
	dayStart, dayEnd := DayWindow(now, time.UTC)
	for index := 0; index < 5; index++ {
		request := transferRequest("attempt-"+string(rune('a'+index)), dayStart, dayEnd)
		result, err := store.ReserveTransfer(context.Background(), request, now)
		if err != nil || !result.Created || result.Usage.Pending != index+1 {
			t.Fatalf("ReserveTransfer(%d) = %+v, %v", index, result, err)
		}
	}
	if _, err := store.ReserveTransfer(context.Background(), transferRequest("attempt-over", dayStart, dayEnd), now); !errors.Is(err, ErrTransferQuotaExceeded) {
		t.Fatalf("ReserveTransfer(over) error = %v", err)
	}
	if ttl := server.TTL(transferPendingKey("user-1")); ttl != TransferPendingKeyTTL {
		t.Fatalf("pending key TTL = %v", ttl)
	}
}

func TestRedisTransferQuotaStoreCommitIsIdempotentAndSurvivesScriptFlush(t *testing.T) {
	store, server, client := newTestRedisLeaseStore(t)
	now := time.Date(2026, 9, 4, 12, 0, 0, 0, time.UTC)
	dayStart, dayEnd := DayWindow(now, time.UTC)
	reserve := transferRequest("attempt-1", dayStart, dayEnd)
	if _, err := store.ReserveTransfer(context.Background(), reserve, now); err != nil {
		t.Fatal(err)
	}
	if err := client.ScriptFlush(context.Background()).Err(); err != nil {
		t.Fatal(err)
	}
	commitRequest := TransferCommitRequest{UserID: reserve.UserID, AttemptID: reserve.AttemptID, DayStart: dayStart, DayEnd: dayEnd}
	result, err := store.CommitTransfer(context.Background(), commitRequest, now.Add(time.Second))
	if err != nil || !result.Added || result.PendingExpiredBeforeCommit || result.Usage.HourlyUsed != 1 || result.Usage.DailyUsed != 1 {
		t.Fatalf("CommitTransfer() = %+v, %v", result, err)
	}
	repeated, err := store.CommitTransfer(context.Background(), commitRequest, now.Add(2*time.Second))
	if err != nil || repeated.Added || repeated.PendingExpiredBeforeCommit || repeated.Usage.HourlyUsed != 1 {
		t.Fatalf("CommitTransfer(repeated) = %+v, %v", repeated, err)
	}
	wantTTL := succeededKeyTTL(now.Add(time.Second), dayEnd)
	if ttl := server.TTL(transferSucceededKey("user-1")); ttl != wantTTL {
		t.Fatalf("succeeded key TTL = %v, want %v", ttl, wantTTL)
	}
}

func TestRedisTransferQuotaStoreRecordsLateSuccessAndReleasesFailure(t *testing.T) {
	store, _, _ := newTestRedisLeaseStore(t)
	now := time.Date(2026, 9, 4, 12, 0, 0, 0, time.UTC)
	dayStart, dayEnd := DayWindow(now, time.UTC)
	late := transferRequest("attempt-late", dayStart, dayEnd)
	if _, err := store.ReserveTransfer(context.Background(), late, now); err != nil {
		t.Fatal(err)
	}
	result, err := store.CommitTransfer(context.Background(), TransferCommitRequest{
		UserID: late.UserID, AttemptID: late.AttemptID, DayStart: dayStart, DayEnd: dayEnd,
	}, now.Add(TransferPendingTTL+time.Second))
	if err != nil || !result.Added || !result.PendingExpiredBeforeCommit || result.Usage.HourlyUsed != 1 {
		t.Fatalf("CommitTransfer(late) = %+v, %v", result, err)
	}

	released := transferRequest("attempt-release", dayStart, dayEnd)
	if _, err := store.ReserveTransfer(context.Background(), released, now.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	removed, err := store.ReleaseTransfer(context.Background(), released.UserID, released.AttemptID, now.Add(2*time.Second))
	if err != nil || !removed {
		t.Fatalf("ReleaseTransfer() = %t, %v", removed, err)
	}
}
