package account

import (
	"context"
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
