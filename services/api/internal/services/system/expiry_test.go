package system

import (
	"context"
	"testing"
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
