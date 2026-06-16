package system

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/konghang/ember/backend/internal/models"
)

func TestCheckExpiredUsersWithContextReturnsCanceledBeforeDB(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	result, err := NewSystemService().CheckExpiredUsersWithContext(ctx)
	if err != nil {
		t.Fatalf("CheckExpiredUsersWithContext() error = %v", err)
	}
	if result == nil {
		t.Fatalf("expected canceled result, got nil")
	}
	if !result.Canceled {
		t.Fatalf("expected result.Canceled=true, got false")
	}
	if result.Processed != 0 || result.DisabledCount != 0 || result.TotalExpired != 0 {
		t.Fatalf("expected empty canceled result, got %+v", result)
	}
}

func TestCheckExpiredUsersWithContextReturnsEmptyResultWhenNoExpiredUsers(t *testing.T) {
	now := testExpiryNow()
	service := NewSystemService()
	service.now = testExpiryNow
	service.countExpiredUsers = func(_ context.Context, cutoff time.Time) (int64, error) {
		if !cutoff.Equal(now) {
			t.Fatalf("expected cutoff %s, got %s", now, cutoff)
		}
		return 0, nil
	}
	service.findExpiredUsers = func(_ context.Context, cutoff time.Time) ([]models.User, error) {
		if !cutoff.Equal(now) {
			t.Fatalf("expected cutoff %s, got %s", now, cutoff)
		}
		return nil, nil
	}
	service.applyExpiredPolicy = func(userID string) error {
		t.Fatalf("applyExpiredPolicy should not be called, got userID=%s", userID)
		return nil
	}

	result, err := service.CheckExpiredUsersWithContext(context.Background())
	if err != nil {
		t.Fatalf("CheckExpiredUsersWithContext() error = %v", err)
	}
	if result.TotalExpired != 0 || result.Processed != 0 || result.DisabledCount != 0 {
		t.Fatalf("expected empty result, got %+v", result)
	}
	if result.Canceled || result.FailureTruncated {
		t.Fatalf("expected non-canceled non-truncated result, got %+v", result)
	}
}

func TestCheckExpiredUsersWithContextRecordsSuccessAndFailure(t *testing.T) {
	expiresAt := time.Date(2026, 6, 15, 8, 9, 10, 0, time.UTC)
	service := NewSystemService()
	service.now = testExpiryNow
	service.countExpiredUsers = func(context.Context, time.Time) (int64, error) {
		return 2, nil
	}
	service.findExpiredUsers = func(context.Context, time.Time) ([]models.User, error) {
		return []models.User{
			{ID: "user_1", Username: "alice", Email: "alice@example.com", ExpiresAt: &expiresAt},
			{ID: "user_2", Username: "bob", Email: "bob@example.com"},
		}, nil
	}
	service.applyExpiredPolicy = func(userID string) error {
		if userID == "user_2" {
			return errors.New("policy failed")
		}
		return nil
	}

	result, err := service.CheckExpiredUsersWithContext(context.Background())
	if err != nil {
		t.Fatalf("CheckExpiredUsersWithContext() error = %v", err)
	}

	if result.TotalExpired != 2 || result.Processed != 2 || result.DisabledCount != 1 {
		t.Fatalf("unexpected counters: %+v", result)
	}
	if len(result.DisabledUsers) != 1 {
		t.Fatalf("expected one disabled user, got %+v", result.DisabledUsers)
	}
	disabled := result.DisabledUsers[0]
	if disabled.Username != "alice" || disabled.Email != "alice@example.com" {
		t.Fatalf("unexpected disabled user: %+v", disabled)
	}
	if disabled.ExpiresAt == nil || *disabled.ExpiresAt != "2026-06-15 08:09:10" {
		t.Fatalf("unexpected expiresAt: %+v", disabled.ExpiresAt)
	}
	if len(result.Errors) != 1 || !strings.Contains(result.Errors[0], "禁用用户 bob 失败: policy failed") {
		t.Fatalf("unexpected errors: %+v", result.Errors)
	}
	if len(result.FailedUsers) != 1 {
		t.Fatalf("expected one failed user, got %+v", result.FailedUsers)
	}
	if result.FailedUsers[0]["username"] != "bob" || result.FailedUsers[0]["error"] != "policy failed" {
		t.Fatalf("unexpected failed user: %+v", result.FailedUsers[0])
	}
	if result.Canceled || result.FailureTruncated {
		t.Fatalf("expected non-canceled non-truncated result, got %+v", result)
	}
}

func testExpiryNow() time.Time {
	return time.Date(2026, 6, 16, 14, 0, 0, 0, time.UTC)
}
