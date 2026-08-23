package embytoken

import (
	"context"
	"database/sql/driver"
	"errors"
	"fmt"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
)

func TestExecuteMappingStoreReadRetriesOnlySafeConnectionFailure(t *testing.T) {
	tests := []struct {
		name      string
		firstErr  error
		secondErr error
		wantCalls int
		wantErr   error
	}{
		{name: "bad connection then success", firstErr: driver.ErrBadConn, wantCalls: 2},
		{name: "wrapped bad connection", firstErr: fmt.Errorf("wrapped: %w", driver.ErrBadConn), wantCalls: 2},
		{name: "retry exhausted", firstErr: driver.ErrBadConn, secondErr: driver.ErrBadConn, wantCalls: 2, wantErr: driver.ErrBadConn},
		{name: "request canceled", firstErr: context.Canceled, wantCalls: 1, wantErr: context.Canceled},
		{name: "deadline exceeded", firstErr: context.DeadlineExceeded, wantCalls: 1, wantErr: context.DeadlineExceeded},
		{name: "postgres response", firstErr: &pgconn.PgError{Code: "08006"}, wantCalls: 1},
		{name: "unknown error", firstErr: errors.New("opaque failure"), wantCalls: 1},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			calls := 0
			err := executeMappingStoreRead(context.Background(), func() error {
				calls++
				if calls == 1 {
					return test.firstErr
				}
				return test.secondErr
			})
			if calls != test.wantCalls {
				t.Fatalf("calls=%d, want %d", calls, test.wantCalls)
			}
			if test.wantErr != nil && !errors.Is(err, test.wantErr) {
				t.Fatalf("error=%v, want %v", err, test.wantErr)
			}
			if test.wantErr == nil && test.secondErr == nil && test.wantCalls == 2 && err != nil {
				t.Fatalf("error=%v, want nil after retry", err)
			}
		})
	}
}

func TestSafeMappingStoreErrorPreservesRequestTermination(t *testing.T) {
	for _, expected := range []error{context.Canceled, context.DeadlineExceeded} {
		if err := safeMappingStoreError("find_mapping", expected, nil); !errors.Is(err, expected) {
			t.Fatalf("safeMappingStoreError(%v)=%v", expected, err)
		}
	}
}

func TestMappingStoreErrorReasonUsesFixedLabels(t *testing.T) {
	tests := []struct {
		err  error
		want string
	}{
		{context.Canceled, "context_canceled"},
		{context.DeadlineExceeded, "deadline_exceeded"},
		{driver.ErrBadConn, "bad_connection"},
		{&pgconn.PgError{Code: "08006"}, "postgres"},
		{errors.New("contains secret fixture-access-token"), "unknown"},
	}
	for _, test := range tests {
		if got := mappingStoreErrorReason(test.err); got != test.want {
			t.Fatalf("mappingStoreErrorReason(%T)=%q, want %q", test.err, got, test.want)
		}
	}
}
