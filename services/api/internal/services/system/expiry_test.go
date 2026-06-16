package system

import (
	"context"
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

func testExpiryNow() time.Time {
	return time.Date(2026, 6, 16, 14, 0, 0, 0, time.UTC)
}
