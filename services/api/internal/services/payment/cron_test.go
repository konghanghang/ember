package payment

import (
	"context"
	"errors"
	"testing"
)

func TestExpirePendingPaymentsUsesInjectedStore(t *testing.T) {
	original := expirePendingPaymentsStore
	t.Cleanup(func() { expirePendingPaymentsStore = original })

	type contextKey string
	ctx := context.WithValue(context.Background(), contextKey("trace"), "test")
	var captured context.Context
	expirePendingPaymentsStore = func(ctx context.Context) (int64, error) {
		captured = ctx
		return 3, nil
	}

	rows, err := ExpirePendingPayments(ctx)

	if err != nil {
		t.Fatalf("expected expire success, got %v", err)
	}
	if rows != 3 {
		t.Fatalf("expected three expired rows, got %d", rows)
	}
	if captured != ctx {
		t.Fatalf("expected context to be passed through")
	}
}

func TestExpirePendingPaymentsReturnsStoreFailure(t *testing.T) {
	original := expirePendingPaymentsStore
	t.Cleanup(func() { expirePendingPaymentsStore = original })

	storeErr := errors.New("database unavailable")
	expirePendingPaymentsStore = func(ctx context.Context) (int64, error) {
		return 0, storeErr
	}

	rows, err := ExpirePendingPayments(context.Background())

	if !errors.Is(err, storeErr) {
		t.Fatalf("expected store error, got %v", err)
	}
	if rows != 0 {
		t.Fatalf("expected zero expired rows on failure, got %d", rows)
	}
}
