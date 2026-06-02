package user

import (
	"errors"
	"strings"
	"testing"

	"github.com/konghang/ember/backend/internal/models"
	paymentpkg "github.com/konghang/ember/backend/internal/services/payment"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
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

func TestAdminUpdateChangesEmbyPolicyIgnoresLocalActiveFlag(t *testing.T) {
	active := false
	email := "alice@example.com"
	if adminUpdateChangesEmbyPolicy(&AdminUpdateUserRequest{IsActive: &active, Email: &email}) {
		t.Fatalf("expected local isActive and email update to skip Emby Policy sync")
	}

	expiresAt := "2099-01-01T00:00:00Z"
	if !adminUpdateChangesEmbyPolicy(&AdminUpdateUserRequest{ExpiresAt: &expiresAt}) {
		t.Fatalf("expected expiry update to require Emby Policy sync")
	}

	if !adminUpdateChangesEmbyPolicy(&AdminUpdateUserRequest{ClearExpiresAt: true}) {
		t.Fatalf("expected expiry clearing to require Emby Policy sync")
	}
}

func TestSyncEmbyPolicyRecordsFailureWithoutFailingCommittedMutation(t *testing.T) {
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

	if err != nil {
		t.Fatalf("expected recorded policy failure to be downgraded, got %v", err)
	}
	if recordedUserID != "user_1" || recordedReason != "admin_plan_group_update" || recordedCause != cause {
		t.Fatalf("expected failure to be recorded, got userID=%q reason=%q cause=%v", recordedUserID, recordedReason, recordedCause)
	}
}

func TestSyncEmbyPolicyReturnsErrorWhenFailureRecordFails(t *testing.T) {
	cause := errors.New("policy write failed")
	recordErr := errors.New("database unavailable")
	service := NewUserServiceWithDeps(UserServiceDeps{
		ApplyPolicy: func(userID, reason string) error {
			return cause
		},
		RecordPolicyFailure: func(userID, reason string, err error) error {
			return recordErr
		},
	})

	err := service.syncEmbyPolicy(&models.User{ID: "user_1", EmbyID: "emby_1"}, "admin_plan_group_update")

	if err == nil {
		t.Fatalf("expected sync error when failure record fails")
	}
	if !strings.Contains(err.Error(), "记录同步失败任务失败") {
		t.Fatalf("expected failure record error, got %v", err)
	}
}

func TestBuildUsersWithPlanGroupSelectSeparatesBatchFailureStatus(t *testing.T) {
	database, err := gorm.Open(postgres.New(postgres.Config{
		DSN:                  "host=127.0.0.1 user=test dbname=test sslmode=disable",
		PreferSimpleProtocol: true,
	}), &gorm.Config{
		DryRun:               true,
		DisableAutomaticPing: true,
	})
	if err != nil {
		t.Fatalf("open dry-run database: %v", err)
	}

	var users []UserView
	stmt := buildUsersWithPlanGroupSelect(database.Model(&models.User{})).Find(&users).Statement
	sql := strings.Join(strings.Fields(stmt.SQL.String()), " ")

	assertSQLContains(t, sql, `WHEN users.role <> 'user' THEN 'synced'`)
	assertSQLContains(t, sql, `AND tasks.batch_id IS NULL AND COALESCE(tasks.last_error, '') NOT LIKE`)
	assertSQLContains(t, sql, `) THEN 'failed' ELSE 'synced' END AS "policySyncStatus"`)
	assertSQLContains(t, sql, `WHEN users.role <> 'user' THEN ''`)
	assertSQLContains(t, sql, `AND tasks.batch_id IS NOT NULL AND COALESCE(tasks.last_error, '') NOT LIKE`)
	assertSQLContains(t, sql, `) THEN 'failed' ELSE '' END AS "policySyncBatchStatus"`)
	assertSQLContains(t, sql, `ORDER BY tasks.updated_at DESC, tasks.created_at DESC LIMIT 1 ), '') AS "policySyncBatchId"`)
	if len(stmt.Vars) < 3 {
		t.Fatalf("expected admin protection patterns in select vars, got %+v", stmt.Vars)
	}
	for i := 0; i < 3; i++ {
		pattern, ok := stmt.Vars[i].(string)
		if !ok || !strings.Contains(pattern, embyAdminPolicyProtectionText) {
			t.Fatalf("expected admin protection pattern at var %d, got %+v", i, stmt.Vars[i])
		}
	}
}

func assertSQLContains(t *testing.T, sql string, fragment string) {
	t.Helper()
	if !strings.Contains(sql, fragment) {
		t.Fatalf("expected SQL to contain %q, got %s", fragment, sql)
	}
}
