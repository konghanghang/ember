package account

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/konghang/ember/backend/internal/models"
)

func TestEmbyCompensationEnqueueRejectsInvalidInputBeforeDB(t *testing.T) {
	tests := []struct {
		name    string
		op      models.FailedEmbyAsyncOp
		wantErr string
	}{
		{
			name: "empty origin",
			op: models.FailedEmbyAsyncOp{
				Origin:     " ",
				EmbyUserID: "emby_user_1",
				Action:     models.FailedEmbyActionUnban,
			},
			wantErr: "origin",
		},
		{
			name: "unknown origin",
			op: models.FailedEmbyAsyncOp{
				Origin:     models.FailedEmbyAsyncOpOrigin("manual_unban"),
				EmbyUserID: "emby_user_1",
				Action:     models.FailedEmbyActionUnban,
			},
			wantErr: "origin",
		},
		{
			name: "empty emby user id",
			op: models.FailedEmbyAsyncOp{
				Origin:     models.FailedEmbyOriginPaymentUnban,
				EmbyUserID: " ",
				Action:     models.FailedEmbyActionUnban,
			},
			wantErr: "embyUserId",
		},
		{
			name: "empty action",
			op: models.FailedEmbyAsyncOp{
				Origin:     models.FailedEmbyOriginPaymentUnban,
				EmbyUserID: "emby_user_1",
				Action:     " ",
			},
			wantErr: "action",
		},
		{
			name: "unknown action",
			op: models.FailedEmbyAsyncOp{
				Origin:     models.FailedEmbyOriginPaymentUnban,
				EmbyUserID: "emby_user_1",
				Action:     models.FailedEmbyAsyncOpAction("enable"),
			},
			wantErr: "action",
		},
	}

	compensation := NewEmbyCompensation(nil)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := compensation.Enqueue(context.Background(), tt.op)
			if err == nil {
				t.Fatalf("expected validation error")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("expected error to mention %q, got %v", tt.wantErr, err)
			}
		})
	}
}

func TestEmbyCompensationEnqueueAppliesDefaultBackoff(t *testing.T) {
	now := time.Date(2026, 6, 16, 12, 0, 0, 0, time.UTC)
	var captured models.FailedEmbyAsyncOp

	compensation := NewEmbyCompensation(nil)
	compensation.now = func() time.Time { return now }
	compensation.createOp = func(_ context.Context, op models.FailedEmbyAsyncOp) error {
		captured = op
		return nil
	}

	err := compensation.Enqueue(context.Background(), models.FailedEmbyAsyncOp{
		Origin:      models.FailedEmbyOriginPaymentUnban,
		OriginRefID: "payment_1",
		EmbyUserID:  "emby_user_1",
		Action:      models.FailedEmbyActionUnban,
	})
	if err != nil {
		t.Fatalf("Enqueue() error = %v", err)
	}

	wantNextAttemptAt := now.Add(30 * time.Second)
	if !captured.NextAttemptAt.Equal(wantNextAttemptAt) {
		t.Fatalf("NextAttemptAt mismatch: want %s got %s", wantNextAttemptAt, captured.NextAttemptAt)
	}
}

func TestEmbyCompensationMarkFailureAppliesBackoffAndTruncatesError(t *testing.T) {
	now := time.Date(2026, 6, 16, 13, 0, 0, 0, time.UTC)
	longErr := strings.Repeat("x", maxLastErrorLength+20)
	var capturedID string
	var capturedUpdates map[string]interface{}

	compensation := NewEmbyCompensation(nil)
	compensation.now = func() time.Time { return now }
	compensation.updateOpFailure = func(_ context.Context, id string, updates map[string]interface{}) error {
		capturedID = id
		capturedUpdates = updates
		return nil
	}

	compensation.markFailure(context.Background(), models.FailedEmbyAsyncOp{
		ID:          "op_1",
		Origin:      models.FailedEmbyOriginPaymentUnban,
		OriginRefID: "payment_1",
		EmbyUserID:  "emby_user_1",
		Action:      models.FailedEmbyActionUnban,
		Retries:     1,
	}, errors.New(longErr))

	if capturedID != "op_1" {
		t.Fatalf("expected op id op_1, got %q", capturedID)
	}
	if capturedUpdates["retries"] != 2 {
		t.Fatalf("expected retries=2, got %#v", capturedUpdates["retries"])
	}
	wantNextAttemptAt := now.Add(2 * time.Minute)
	if got, ok := capturedUpdates["next_attempt_at"].(time.Time); !ok || !got.Equal(wantNextAttemptAt) {
		t.Fatalf("next_attempt_at mismatch: want %s got %#v", wantNextAttemptAt, capturedUpdates["next_attempt_at"])
	}
	lastError, ok := capturedUpdates["last_error"].(string)
	if !ok {
		t.Fatalf("expected last_error string, got %#v", capturedUpdates["last_error"])
	}
	if len(lastError) != maxLastErrorLength {
		t.Fatalf("expected last_error length %d, got %d", maxLastErrorLength, len(lastError))
	}
	if got, ok := capturedUpdates["updated_at"].(time.Time); !ok || !got.Equal(now) {
		t.Fatalf("updated_at mismatch: want %s got %#v", now, capturedUpdates["updated_at"])
	}
}
