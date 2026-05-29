package user

import (
	"errors"
	"strings"
	"testing"

	"github.com/konghang/ember/backend/internal/models"
	paymentpkg "github.com/konghang/ember/backend/internal/services/payment"
)

func TestNormalizePlanGroupStrict(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{name: "uppercase key", input: "vip_a", want: "VIP_A"},
		{name: "blank rejected", input: "", wantErr: true},
		{name: "invalid rejected", input: "vip a", wantErr: true},
	}

	for _, tc := range tests {
		got, err := normalizePlanGroupStrict(tc.input)
		if tc.wantErr {
			if err == nil {
				t.Fatalf("%s: expected error, got nil", tc.name)
			}
			continue
		}
		if err != nil {
			t.Fatalf("%s: unexpected error: %v", tc.name, err)
		}
		if got != tc.want {
			t.Fatalf("%s: want %s, got %s", tc.name, tc.want, got)
		}
	}
}

func TestNormalizePlanGroupUpdateRejectsBlank(t *testing.T) {
	_, err := normalizePlanGroupUpdate(" ")
	if !errors.Is(err, paymentpkg.ErrPlanGroupInvalid) {
		t.Fatalf("expected ErrPlanGroupInvalid, got %v", err)
	}
}

func TestSyncEmbyPolicyRecordsFailure(t *testing.T) {
	cause := errors.New("policy write failed")
	var recordedUserID string
	var recordedReason string
	var recordedCause error
	service := NewUserServiceWithDeps(UserServiceDeps{
		ApplyPolicy: func(userID, reason string) error {
			return cause
		},
		RecordPolicyFailure: func(userID, reason string, err error) error {
			recordedUserID = userID
			recordedReason = reason
			recordedCause = err
			return nil
		},
	})

	err := service.syncEmbyPolicy(&models.User{ID: "user_1", EmbyID: "emby_1"}, "admin_plan_group_update")

	if err == nil || !strings.Contains(err.Error(), "同步 Emby 用户策略失败") {
		t.Fatalf("expected sync error, got %v", err)
	}
	if recordedUserID != "user_1" || recordedReason != "admin_plan_group_update" || recordedCause != cause {
		t.Fatalf("expected failure to be recorded, got userID=%q reason=%q cause=%v", recordedUserID, recordedReason, recordedCause)
	}
}
