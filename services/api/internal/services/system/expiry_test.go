package system

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/konghang/ember/backend/internal/models"
	embytokenpkg "github.com/konghang/ember/backend/internal/services/embytoken"
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
	service := newTestSystemService()
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
	service := newTestSystemService()
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

func TestCheckExpiredUsersWithContextStopsAfterCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	service := newTestSystemService()
	service.now = testExpiryNow
	service.countExpiredUsers = func(context.Context, time.Time) (int64, error) {
		return 3, nil
	}
	service.findExpiredUsers = func(context.Context, time.Time) ([]models.User, error) {
		return []models.User{
			{ID: "user_1", Username: "alice", Email: "alice@example.com"},
			{ID: "user_2", Username: "bob", Email: "bob@example.com"},
			{ID: "user_3", Username: "charlie", Email: "charlie@example.com"},
		}, nil
	}
	applied := []string{}
	service.applyExpiredPolicy = func(userID string) error {
		applied = append(applied, userID)
		cancel()
		return nil
	}

	result, err := service.CheckExpiredUsersWithContext(ctx)
	if err != nil {
		t.Fatalf("CheckExpiredUsersWithContext() error = %v", err)
	}
	if !result.Canceled {
		t.Fatalf("expected canceled result, got %+v", result)
	}
	if result.TotalExpired != 3 || result.Processed != 1 || result.DisabledCount != 1 {
		t.Fatalf("unexpected counters after cancellation: %+v", result)
	}
	if len(applied) != 1 || applied[0] != "user_1" {
		t.Fatalf("expected only first user to be processed, got %+v", applied)
	}
	if len(result.DisabledUsers) != 1 || result.DisabledUsers[0].Username != "alice" {
		t.Fatalf("expected first user in disabled list, got %+v", result.DisabledUsers)
	}
}

func TestCheckExpiredUsersWithContextTruncatesFailureDetails(t *testing.T) {
	service := newTestSystemService()
	service.now = testExpiryNow
	service.countExpiredUsers = func(context.Context, time.Time) (int64, error) {
		return 25, nil
	}
	service.findExpiredUsers = func(context.Context, time.Time) ([]models.User, error) {
		users := make([]models.User, 0, 25)
		for i := 0; i < 25; i++ {
			username := fmt.Sprintf("user_%c", 'a'+i)
			users = append(users, models.User{
				ID:       username,
				Username: username,
				Email:    "user@example.com",
			})
		}
		return users, nil
	}
	service.applyExpiredPolicy = func(userID string) error {
		return errors.New("policy failed")
	}

	result, err := service.CheckExpiredUsersWithContext(context.Background())
	if err != nil {
		t.Fatalf("CheckExpiredUsersWithContext() error = %v", err)
	}
	if result.TotalExpired != 25 || result.Processed != 25 || result.DisabledCount != 0 {
		t.Fatalf("unexpected counters: %+v", result)
	}
	if !result.FailureTruncated {
		t.Fatalf("expected failure details to be truncated")
	}
	if len(result.Errors) != maxCheckExpiredUsersErrors {
		t.Fatalf("expected %d error messages, got %d", maxCheckExpiredUsersErrors, len(result.Errors))
	}
	if len(result.FailedUsers) != maxCheckExpiredUsersFailedUsers {
		t.Fatalf("expected %d failed users, got %d", maxCheckExpiredUsersFailedUsers, len(result.FailedUsers))
	}
	lastError := result.Errors[len(result.Errors)-1]
	if !strings.Contains(lastError, "禁用用户 user_t 失败: policy failed") {
		t.Fatalf("expected last retained error to be for user_t, got %q", lastError)
	}
	if result.FailedUsers[len(result.FailedUsers)-1]["username"] != "user_t" {
		t.Fatalf("expected last retained failed user to be user_t, got %+v", result.FailedUsers[len(result.FailedUsers)-1])
	}
}

func TestCheckExpiredUsersRevokesBeforeApplyingExpiredPolicy(t *testing.T) {
	var order []string
	service := newTestSystemService()
	service.countExpiredUsers = func(context.Context, time.Time) (int64, error) { return 1, nil }
	service.findExpiredUsers = func(context.Context, time.Time) ([]models.User, error) {
		return []models.User{{ID: "user_1", Username: "alice"}}, nil
	}
	service.revokeUserTokens = func(_ context.Context, userID string, reason embytokenpkg.RevokeReason, actor string) (int64, error) {
		order = append(order, "revoke")
		if userID != "user_1" || reason != embytokenpkg.RevokeReasonEmbyDisabled || actor != expiryRevocationActor {
			t.Fatalf("revoke input user=%s reason=%s actor=%s", userID, reason, actor)
		}
		return 1, nil
	}
	service.applyExpiredPolicy = func(string) error {
		order = append(order, "policy")
		return nil
	}
	result, err := service.CheckExpiredUsersWithContext(context.Background())
	if err != nil || result.DisabledCount != 1 {
		t.Fatalf("CheckExpiredUsersWithContext() result=%+v error=%v", result, err)
	}
	if strings.Join(order, ",") != "revoke,policy" {
		t.Fatalf("operation order = %#v", order)
	}
}

func TestCheckExpiredUsersRevocationFailureSkipsPolicy(t *testing.T) {
	service := newTestSystemService()
	service.countExpiredUsers = func(context.Context, time.Time) (int64, error) { return 1, nil }
	service.findExpiredUsers = func(context.Context, time.Time) ([]models.User, error) {
		return []models.User{{ID: "user_1", Username: "alice"}}, nil
	}
	service.revokeUserTokens = func(context.Context, string, embytokenpkg.RevokeReason, string) (int64, error) {
		return 0, errors.New("revoke failed")
	}
	service.applyExpiredPolicy = func(string) error {
		t.Fatal("policy must not run after revoke failure")
		return nil
	}
	result, err := service.CheckExpiredUsersWithContext(context.Background())
	if err != nil {
		t.Fatalf("CheckExpiredUsersWithContext() error = %v", err)
	}
	if result.DisabledCount != 0 || len(result.FailedUsers) != 1 || result.FailedUsers[0]["error"] != ErrExpiredUserTokenRevocation.Error() {
		t.Fatalf("result = %+v", result)
	}
}

func newTestSystemService() *SystemService {
	service := NewSystemService()
	service.revokeUserTokens = func(context.Context, string, embytokenpkg.RevokeReason, string) (int64, error) {
		return 0, nil
	}
	return service
}

func testExpiryNow() time.Time {
	return time.Date(2026, 6, 16, 14, 0, 0, 0, time.UTC)
}
