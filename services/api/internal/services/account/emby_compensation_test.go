package account

import (
	"context"
	"strings"
	"testing"

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
