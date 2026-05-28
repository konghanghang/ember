package policy

import (
	"testing"
	"time"
)

func TestResolveBatchStatusKeepsPendingWhenTasksAreWaiting(t *testing.T) {
	now := time.Date(2026, 5, 28, 12, 0, 0, 0, time.UTC)

	status, finishedAt := resolveBatchStatus(3, 2, 0, 1, 0, nil, now)

	if status != SyncStatusPending {
		t.Fatalf("expected pending status, got %s", status)
	}
	if finishedAt != nil {
		t.Fatalf("expected unfinished batch, got %v", finishedAt)
	}
}

func TestResolveBatchStatusMarksProcessingWhenAnyTaskIsProcessing(t *testing.T) {
	now := time.Date(2026, 5, 28, 12, 0, 0, 0, time.UTC)

	status, finishedAt := resolveBatchStatus(3, 1, 1, 1, 0, nil, now)

	if status != SyncStatusProcessing {
		t.Fatalf("expected processing status, got %s", status)
	}
	if finishedAt != nil {
		t.Fatalf("expected unfinished batch, got %v", finishedAt)
	}
}

func TestResolveBatchStatusClosesTerminalStates(t *testing.T) {
	now := time.Date(2026, 5, 28, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		name   string
		total  int
		synced int
		failed int
		want   string
	}{
		{name: "empty", total: 0, want: SyncStatusSynced},
		{name: "all synced", total: 2, synced: 2, want: SyncStatusSynced},
		{name: "all failed", total: 2, failed: 2, want: SyncStatusFailed},
		{name: "partial failed", total: 3, synced: 2, failed: 1, want: SyncStatusPartialFailed},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status, finishedAt := resolveBatchStatus(tt.total, 0, 0, tt.synced, tt.failed, nil, now)
			if status != tt.want {
				t.Fatalf("expected %s status, got %s", tt.want, status)
			}
			if finishedAt == nil || !finishedAt.Equal(now) {
				t.Fatalf("expected finishedAt %v, got %v", now, finishedAt)
			}
		})
	}
}

func TestResolveBatchStatusPreservesExistingFinishedAt(t *testing.T) {
	now := time.Date(2026, 5, 28, 12, 0, 0, 0, time.UTC)
	existing := now.Add(-time.Hour)

	_, finishedAt := resolveBatchStatus(1, 0, 0, 1, 0, &existing, now)

	if finishedAt == nil || !finishedAt.Equal(existing) {
		t.Fatalf("expected existing finishedAt %v, got %v", existing, finishedAt)
	}
}
