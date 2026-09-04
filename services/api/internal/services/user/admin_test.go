package user

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/konghang/ember/backend/internal/models"
	embytokenpkg "github.com/konghang/ember/backend/internal/services/embytoken"
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

func TestNormalizeUserLookupError(t *testing.T) {
	cause := errors.New("database unavailable")
	testCases := []struct {
		name string
		err  error
		want error
	}{
		{name: "nil", err: nil, want: nil},
		{name: "domain not found", err: ErrUserNotFound, want: ErrUserNotFound},
		{name: "gorm not found", err: gorm.ErrRecordNotFound, want: ErrUserNotFound},
		{name: "wrapped gorm not found", err: errors.Join(errors.New("query failed"), gorm.ErrRecordNotFound), want: ErrUserNotFound},
		{name: "other error", err: cause, want: cause},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got := normalizeUserLookupError(tc.err)
			if !errors.Is(got, tc.want) {
				t.Fatalf("expected %v, got %v", tc.want, got)
			}
		})
	}
}

func TestCalculateExtendedExpiryStartsFromNowWithoutActiveExpiry(t *testing.T) {
	now := time.Date(2026, 6, 17, 10, 0, 0, 0, time.UTC)
	expiredAt := now.Add(-time.Minute)

	tests := []struct {
		name          string
		currentExpiry *time.Time
	}{
		{name: "nil expiry", currentExpiry: nil},
		{name: "expired expiry", currentExpiry: &expiredAt},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := calculateExtendedExpiry(now, tc.currentExpiry, 14)
			want := now.AddDate(0, 0, 14)

			if !got.Equal(want) {
				t.Fatalf("expected expiry from now %s, got %s", want, got)
			}
		})
	}
}

func TestCalculateExtendedExpiryExtendsActiveExpiry(t *testing.T) {
	now := time.Date(2026, 6, 17, 10, 0, 0, 0, time.UTC)
	currentExpiry := now.AddDate(0, 0, 7)

	got := calculateExtendedExpiry(now, &currentExpiry, 30)
	want := currentExpiry.AddDate(0, 0, 30)

	if !got.Equal(want) {
		t.Fatalf("expected extension from current expiry %s, got %s", want, got)
	}
}

func TestDeleteUserDeletesEmbyBeforeLocalRecord(t *testing.T) {
	client := &stubUserEmbyClient{}
	var deletedUserID string
	service := &UserService{
		findUserByID: func(userID string) (*models.User, error) {
			return &models.User{ID: userID, EmbyID: "emby_1"}, nil
		},
		newEmbyClient: func() embyClient { return client },
		deleteUserRecord: func(user *models.User) error {
			deletedUserID = user.ID
			return nil
		},
		revokeUserTokens:          noopUserTokenRevoker,
		revokePersonalP115Account: noopPersonalP115Revoker,
	}

	err := service.DeleteUser("user_1")

	if err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	if client.lastDeleteUserID != "emby_1" {
		t.Fatalf("expected injected emby client to delete emby_1, got %q", client.lastDeleteUserID)
	}
	if deletedUserID != "user_1" {
		t.Fatalf("expected local user record to be deleted, got %q", deletedUserID)
	}
}

func TestDeleteUserSkipsLocalDeleteWhenEmbyDeleteFails(t *testing.T) {
	client := &stubUserEmbyClient{deleteUserErr: errors.New("emby unavailable")}
	service := &UserService{
		findUserByID: func(userID string) (*models.User, error) {
			return &models.User{ID: userID, EmbyID: "emby_1"}, nil
		},
		newEmbyClient: func() embyClient { return client },
		deleteUserRecord: func(user *models.User) error {
			t.Fatalf("local delete must not run when emby delete fails")
			return nil
		},
		revokeUserTokens:          noopUserTokenRevoker,
		revokePersonalP115Account: noopPersonalP115Revoker,
	}

	err := service.DeleteUser("user_1")

	if err == nil || err.Error() != "删除用户失败：emby unavailable" {
		t.Fatalf("expected emby delete failure, got %v", err)
	}
	if client.lastDeleteUserID != "emby_1" {
		t.Fatalf("expected emby delete attempt, got %q", client.lastDeleteUserID)
	}
}

func TestDeleteUserWithoutEmbyIDDeletesLocalOnly(t *testing.T) {
	var deletedUserID string
	service := &UserService{
		findUserByID: func(userID string) (*models.User, error) {
			return &models.User{ID: userID}, nil
		},
		newEmbyClient: func() embyClient {
			t.Fatalf("emby client must not be created for users without emby id")
			return nil
		},
		deleteUserRecord: func(user *models.User) error {
			deletedUserID = user.ID
			return nil
		},
		revokeUserTokens:          noopUserTokenRevoker,
		revokePersonalP115Account: noopPersonalP115Revoker,
	}

	err := service.DeleteUser("user_1")

	if err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	if deletedUserID != "user_1" {
		t.Fatalf("expected local user record to be deleted, got %q", deletedUserID)
	}
}

func TestToggleUserStatusPersistsFlippedState(t *testing.T) {
	var persistedUserID string
	var persistedActive bool
	service := &UserService{
		findUserByID: func(userID string) (*models.User, error) {
			if userID != "user_1" {
				t.Fatalf("unexpected user id: %s", userID)
			}
			return &models.User{ID: userID, IsActive: false}, nil
		},
		updateUserActive: func(userID string, isActive bool) error {
			persistedUserID = userID
			persistedActive = isActive
			return nil
		},
		getUserViewByID: func(userID string) (*UserView, error) {
			if userID != "user_1" {
				t.Fatalf("unexpected refreshed user id: %s", userID)
			}
			return &UserView{User: models.User{ID: userID, IsActive: persistedActive}}, nil
		},
		revokeUserTokens: noopUserTokenRevoker,
	}

	view, err := service.ToggleUserStatus("user_1")

	if err != nil {
		t.Fatalf("expected toggle success, got %v", err)
	}
	if persistedUserID != "user_1" || !persistedActive {
		t.Fatalf("expected active status to be persisted for user_1, got userID=%q active=%t", persistedUserID, persistedActive)
	}
	if view == nil || !view.IsActive {
		t.Fatalf("expected refreshed active user view, got %+v", view)
	}
}

func TestToggleUserStatusMapsLookupFailureBeforeMutation(t *testing.T) {
	service := &UserService{
		findUserByID: func(userID string) (*models.User, error) {
			return nil, gorm.ErrRecordNotFound
		},
		updateUserActive: func(userID string, isActive bool) error {
			t.Fatalf("updateUserActive must not run after lookup failure")
			return nil
		},
		getUserViewByID: func(userID string) (*UserView, error) {
			t.Fatalf("getUserViewByID must not run after lookup failure")
			return nil, nil
		},
		revokeUserTokens: noopUserTokenRevoker,
	}

	view, err := service.ToggleUserStatus("missing_user")

	if !errors.Is(err, ErrUserNotFound) {
		t.Fatalf("expected ErrUserNotFound, got %v", err)
	}
	if view != nil {
		t.Fatalf("expected nil view on failure, got %+v", view)
	}
}

func TestToggleUserStatusReturnsPersistFailureBeforeRefresh(t *testing.T) {
	persistErr := errors.New("database unavailable")
	service := &UserService{
		findUserByID: func(userID string) (*models.User, error) {
			return &models.User{ID: userID, IsActive: true}, nil
		},
		updateUserActive: func(userID string, isActive bool) error {
			if isActive {
				t.Fatalf("expected active status to be toggled off")
			}
			return persistErr
		},
		getUserViewByID: func(userID string) (*UserView, error) {
			t.Fatalf("getUserViewByID must not run after persist failure")
			return nil, nil
		},
		revokeUserTokens: noopUserTokenRevoker,
	}

	view, err := service.ToggleUserStatus("user_1")

	if !errors.Is(err, persistErr) {
		t.Fatalf("expected persist error, got %v", err)
	}
	if view != nil {
		t.Fatalf("expected nil view on failure, got %+v", view)
	}
}

func TestToggleUserStatusRevokesBeforeEveryHardStateTransition(t *testing.T) {
	tests := []struct {
		name       string
		isActive   bool
		wantActive bool
		wantReason embytokenpkg.RevokeReason
	}{
		{name: "disable", isActive: true, wantActive: false, wantReason: embytokenpkg.RevokeReasonUserDisabled},
		{name: "restore still invalidates old tokens", isActive: false, wantActive: true, wantReason: embytokenpkg.RevokeReasonSecurityRevoke},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var order []string
			service := &UserService{
				findUserByID: func(userID string) (*models.User, error) {
					return &models.User{ID: userID, IsActive: test.isActive}, nil
				},
				revokeUserTokens: func(_ context.Context, userID string, reason embytokenpkg.RevokeReason, actor string) (int64, error) {
					order = append(order, "revoke")
					if userID != "user_1" || reason != test.wantReason || actor != "admin_1" {
						t.Fatalf("revoke input user=%s reason=%s actor=%s", userID, reason, actor)
					}
					return 1, nil
				},
				updateUserActive: func(_ string, active bool) error {
					order = append(order, "update")
					if active != test.wantActive {
						t.Fatalf("active = %t, want %t", active, test.wantActive)
					}
					return nil
				},
				getUserViewByID: func(userID string) (*UserView, error) {
					return &UserView{User: models.User{ID: userID, IsActive: test.wantActive}}, nil
				},
			}
			if _, err := service.ToggleUserStatusWithContext(context.Background(), "user_1", "admin_1"); err != nil {
				t.Fatalf("ToggleUserStatusWithContext() error = %v", err)
			}
			if len(order) != 2 || order[0] != "revoke" || order[1] != "update" {
				t.Fatalf("operation order = %#v", order)
			}
		})
	}
}

func TestToggleUserStatusRevocationFailurePreventsMutation(t *testing.T) {
	service := &UserService{
		findUserByID: func(userID string) (*models.User, error) {
			return &models.User{ID: userID, IsActive: true}, nil
		},
		revokeUserTokens: func(context.Context, string, embytokenpkg.RevokeReason, string) (int64, error) {
			return 0, errors.New("revoke failed")
		},
		updateUserActive: func(string, bool) error {
			t.Fatal("updateUserActive must not run after revoke failure")
			return nil
		},
		getUserViewByID: func(string) (*UserView, error) {
			t.Fatal("getUserViewByID must not run after revoke failure")
			return nil, nil
		},
	}
	if _, err := service.ToggleUserStatusWithContext(context.Background(), "user_1", "admin_1"); !errors.Is(err, ErrUserTokenRevocation) {
		t.Fatalf("ToggleUserStatusWithContext() error = %v, want %v", err, ErrUserTokenRevocation)
	}
}

func TestDeleteUserRevokesBeforeRemoteAndLocalDeletion(t *testing.T) {
	var order []string
	client := &stubUserEmbyClient{}
	service := &UserService{
		findUserByID: func(userID string) (*models.User, error) {
			return &models.User{ID: userID, EmbyID: "emby_1"}, nil
		},
		revokeUserTokens: func(_ context.Context, userID string, reason embytokenpkg.RevokeReason, actor string) (int64, error) {
			order = append(order, "revoke")
			if userID != "user_1" || reason != embytokenpkg.RevokeReasonUserDeleted || actor != "admin_1" {
				t.Fatalf("revoke input user=%s reason=%s actor=%s", userID, reason, actor)
			}
			return 1, nil
		},
		revokePersonalP115Account: func(_ context.Context, userID string) error {
			order = append(order, "p115_tombstone")
			if userID != "user_1" {
				t.Fatalf("p115 tombstone user=%s", userID)
			}
			return nil
		},
		newEmbyClient: func() embyClient {
			return &orderedDeleteUserClient{stubUserEmbyClient: client, order: &order}
		},
		deleteUserRecord: func(*models.User) error {
			order = append(order, "local_delete")
			return nil
		},
	}
	if err := service.DeleteUserWithContext(context.Background(), "user_1", "admin_1"); err != nil {
		t.Fatalf("DeleteUserWithContext() error = %v", err)
	}
	if strings.Join(order, ",") != "revoke,p115_tombstone,remote_delete,local_delete" {
		t.Fatalf("operation order = %#v", order)
	}
}

func TestDeleteUserStopsWhenPersonalP115TombstoneFails(t *testing.T) {
	service := &UserService{
		findUserByID: func(userID string) (*models.User, error) {
			return &models.User{ID: userID, EmbyID: "emby_1"}, nil
		},
		revokeUserTokens: func(context.Context, string, embytokenpkg.RevokeReason, string) (int64, error) {
			return 1, nil
		},
		revokePersonalP115Account: func(context.Context, string) error {
			return errors.New("tombstone failed")
		},
		newEmbyClient: func() embyClient {
			t.Fatal("Emby delete must not run after P115 tombstone failure")
			return nil
		},
		deleteUserRecord: func(*models.User) error {
			t.Fatal("local delete must not run after P115 tombstone failure")
			return nil
		},
	}

	err := service.DeleteUserWithContext(context.Background(), "user_1", "admin_1")
	if !errors.Is(err, ErrUserP115AccountRevocation) {
		t.Fatalf("DeleteUserWithContext() error = %v, want ErrUserP115AccountRevocation", err)
	}
}

type orderedDeleteUserClient struct {
	*stubUserEmbyClient
	order *[]string
}

func noopUserTokenRevoker(context.Context, string, embytokenpkg.RevokeReason, string) (int64, error) {
	return 0, nil
}

func noopPersonalP115Revoker(context.Context, string) error {
	return nil
}

func (client *orderedDeleteUserClient) DeleteUser(embyUserID string) error {
	*client.order = append(*client.order, "remote_delete")
	return client.stubUserEmbyClient.DeleteUser(embyUserID)
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
	assertSQLContains(t, sql, `users."applied_media_library_template_version" < COALESCE(explicit_pg.media_library_template_version, default_pg.media_library_template_version)`)
	assertSQLContains(t, sql, `THEN 'out_of_sync' ELSE 'synced' END AS "policySyncStatus"`)
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
