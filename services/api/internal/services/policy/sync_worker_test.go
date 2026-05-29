package policy

import (
	"errors"
	"testing"
	"time"

	"github.com/konghang/ember/backend/internal/models"
)

type stubPolicyClient struct {
	raw map[string]any
	err error
}

func (s *stubPolicyClient) GetUserPolicyRaw(string) (map[string]any, error) {
	return s.raw, s.err
}

func (s *stubPolicyClient) PatchUserPolicyFields(string, map[string]any, []string) error {
	return nil
}

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

func TestBuildUserPolicySyncRetryTaskUsesPendingRetryState(t *testing.T) {
	now := time.Date(2026, 5, 28, 12, 0, 0, 0, time.UTC)
	planGroup := "VIP_A"

	task, err := buildUserPolicySyncRetryTask(&models.User{
		ID:        "user_1",
		EmbyID:    "emby_1",
		PlanGroup: &planGroup,
	}, " user_registered ", errors.New("policy write failed"), now)
	if err != nil {
		t.Fatalf("expected task build success, got %v", err)
	}
	if task == nil {
		t.Fatalf("expected retry task")
	}
	if task.UserID != "user_1" || task.EmbyID != "emby_1" || task.PlanGroupKey != "VIP_A" {
		t.Fatalf("unexpected task identity: %+v", task)
	}
	if task.Reason != "user_registered" {
		t.Fatalf("expected trimmed reason, got %q", task.Reason)
	}
	if task.Status != SyncStatusPending || task.Attempts != 1 {
		t.Fatalf("expected pending retry with one recorded attempt, got status=%s attempts=%d", task.Status, task.Attempts)
	}
	if task.LastError == nil || *task.LastError != "policy write failed" {
		t.Fatalf("expected last error to be preserved, got %+v", task.LastError)
	}
	if task.NextRetryAt == nil || !task.NextRetryAt.Equal(now) {
		t.Fatalf("expected nextRetryAt %v, got %v", now, task.NextRetryAt)
	}
}

func TestBuildUserPolicySyncRetryTaskSkipsUnboundEmbyUser(t *testing.T) {
	task, err := buildUserPolicySyncRetryTask(&models.User{ID: "user_1"}, "user_registered", errors.New("boom"), time.Now())
	if err != nil {
		t.Fatalf("expected unbound Emby user to be skipped without error, got %v", err)
	}
	if task != nil {
		t.Fatalf("expected no task for unbound Emby user, got %+v", task)
	}
}

func TestBuildUserPolicySyncFailureTaskUsesManualFailureState(t *testing.T) {
	task := buildUserPolicySyncFailureTask(&models.User{
		ID:     "user_1",
		EmbyID: "emby_1",
	}, "VIP_A", " admin_retry ", errors.New("policy write failed"))

	if task.UserID != "user_1" || task.EmbyID != "emby_1" || task.PlanGroupKey != "VIP_A" {
		t.Fatalf("unexpected task identity: %+v", task)
	}
	if task.Reason != "admin_retry" {
		t.Fatalf("expected trimmed reason, got %q", task.Reason)
	}
	if task.Status != SyncStatusFailed || task.Attempts != 1 {
		t.Fatalf("expected manual failed task, got status=%s attempts=%d", task.Status, task.Attempts)
	}
	if task.LastError == nil || *task.LastError != "policy write failed" {
		t.Fatalf("expected last error to be preserved, got %+v", task.LastError)
	}
}

func TestReadCurrentUserPolicyLibraryIDsUsesAllFolders(t *testing.T) {
	service := &Service{embyClient: &stubPolicyClient{raw: map[string]any{
		"EnableAllFolders": true,
	}}}

	got, err := service.readCurrentUserPolicyLibraryIDs("emby_1", []string{"lib_b", "lib_a"})
	if err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	if len(got) != 2 || got[0] != "lib_a" || got[1] != "lib_b" {
		t.Fatalf("expected sorted all library ids, got %+v", got)
	}
}

func TestReadCurrentUserPolicyLibraryIDsUsesEnabledFolders(t *testing.T) {
	service := &Service{embyClient: &stubPolicyClient{raw: map[string]any{
		"EnableAllFolders": false,
		"EnabledFolders":   []any{"lib_b", "lib_a", "lib_a", ""},
	}}}

	got, err := service.readCurrentUserPolicyLibraryIDs("emby_1", []string{"ignored"})
	if err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	if len(got) != 2 || got[0] != "lib_a" || got[1] != "lib_b" {
		t.Fatalf("expected sorted enabled folders, got %+v", got)
	}
}
