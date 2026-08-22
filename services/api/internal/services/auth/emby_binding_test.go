package auth

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
	embyint "github.com/konghang/ember/backend/internal/integrations/emby"
	"github.com/konghang/ember/backend/internal/models"
	embytokenpkg "github.com/konghang/ember/backend/internal/services/embytoken"
)

// fakeBindingDB 表示 BindEmbyAccount 测试场景中的本地数据视图。
// findUserByID / findOccupyingUser / findUsersByEmbyIDs / updateUserEmbyID 的行为都来自这里。
type fakeBindingDB struct {
	currentUser     *models.User
	occupyingUser   *models.User
	boundUsers      []models.User
	updateErr       error
	updateCalled    bool
	lastWrittenID   string
	lastWrittenEmby string
}

func newAuthServiceForBinding(t *testing.T, embyClient *stubAuthEmbyClient, fake *fakeBindingDB) *AuthService {
	t.Helper()
	service := &AuthService{
		newEmbyClient: func() authEmbyClient { return embyClient },
		findUserByIDForBindingFn: func(userID string) (*models.User, error) {
			if fake.currentUser == nil {
				return nil, errors.New("current user not configured")
			}
			if fake.currentUser.ID != userID {
				t.Fatalf("unexpected userID lookup: want=%s got=%s", fake.currentUser.ID, userID)
			}
			// 返回副本，避免被 service 内部直接改写影响后续断言
			copyUser := *fake.currentUser
			return &copyUser, nil
		},
		findOccupyingUserFn: func(embyID, excludeUserID string) (*models.User, error) {
			if fake.occupyingUser == nil {
				return nil, nil
			}
			if fake.occupyingUser.ID == excludeUserID {
				return nil, nil
			}
			if fake.occupyingUser.EmbyID != embyID {
				return nil, nil
			}
			copyUser := *fake.occupyingUser
			return &copyUser, nil
		},
		findUsersByEmbyIDsFn: func(embyIDs []string) ([]models.User, error) {
			allowed := make(map[string]struct{}, len(embyIDs))
			for _, embyID := range embyIDs {
				allowed[embyID] = struct{}{}
			}
			users := make([]models.User, 0, len(fake.boundUsers))
			for _, user := range fake.boundUsers {
				if _, ok := allowed[user.EmbyID]; ok {
					users = append(users, user)
				}
			}
			return users, nil
		},
		updateUserEmbyIDFn: func(userID, embyID string) error {
			fake.updateCalled = true
			fake.lastWrittenID = userID
			fake.lastWrittenEmby = embyID
			return fake.updateErr
		},
		revokeUserTokensFn: func(context.Context, string, embytokenpkg.RevokeReason, string) (int64, error) {
			return 0, nil
		},
	}
	return service
}

func TestListAdminEmbyUsersMergesLocalBindingState(t *testing.T) {
	embyClient := &stubAuthEmbyClient{
		getUsersResp: []embyint.EmbyUser{
			{ID: "emby_1", Name: "admin_remote", HasPassword: true},
			{ID: "emby_2", Name: "occupied_remote"},
			{ID: "emby_3", Name: "free_remote"},
		},
	}
	fake := &fakeBindingDB{
		currentUser: &models.User{ID: "admin_1", Username: "admin", Role: "admin", EmbyID: "emby_1"},
		boundUsers: []models.User{
			{ID: "admin_1", Username: "admin", EmbyID: "emby_1"},
			{ID: "user_2", Username: "member", EmbyID: "emby_2"},
		},
	}
	service := newAuthServiceForBinding(t, embyClient, fake)

	resp, err := service.ListAdminEmbyUsers("admin_1", ListAdminEmbyUsersRequest{Query: "remote"})
	if err != nil {
		t.Fatalf("expected list success, got %v", err)
	}
	if len(resp.Data) != 3 {
		t.Fatalf("expected 3 options, got %+v", resp.Data)
	}
	if !resp.Data[0].BoundToCurrent || !resp.Data[0].Available || resp.Data[0].BoundUsername != "admin" {
		t.Fatalf("unexpected current binding option: %+v", resp.Data[0])
	}
	if resp.Data[1].Available || resp.Data[1].BoundToCurrent || resp.Data[1].BoundUsername != "member" {
		t.Fatalf("unexpected occupied option: %+v", resp.Data[1])
	}
	if !resp.Data[2].Available || resp.Data[2].BoundUsername != "" {
		t.Fatalf("unexpected free option: %+v", resp.Data[2])
	}
}

func TestListAdminEmbyUsersUnavailable(t *testing.T) {
	embyClient := &stubAuthEmbyClient{getUsersErr: errors.New("无法连接到 Emby 服务器")}
	fake := &fakeBindingDB{
		currentUser: &models.User{ID: "admin_1", Username: "admin", Role: "admin"},
	}
	service := newAuthServiceForBinding(t, embyClient, fake)

	_, err := service.ListAdminEmbyUsers("admin_1", ListAdminEmbyUsersRequest{Query: "admin"})
	if !errors.Is(err, ErrEmbyServiceUnavailable) {
		t.Fatalf("expected emby unavailable error, got %v", err)
	}
}

func TestListAdminEmbyUsersRequiresSearchQuery(t *testing.T) {
	service := &AuthService{}

	_, err := service.ListAdminEmbyUsers("admin_1", ListAdminEmbyUsersRequest{Query: "a"})
	if !errors.Is(err, ErrEmbyUserSearchQueryRequired) {
		t.Fatalf("expected search query required error, got %v", err)
	}
}

func TestBindEmbyAccountValidation(t *testing.T) {
	service := &AuthService{}

	cases := []struct {
		name string
		req  BindEmbyAccountRequest
	}{
		{"empty emby id", BindEmbyAccountRequest{EmbyID: "  "}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			req := c.req
			_, err := service.BindEmbyAccount("admin_1", &req)
			if !errors.Is(err, ErrEmbyBindingTargetRequired) {
				t.Fatalf("expected target required error, got %v", err)
			}
		})
	}
}

func TestBindEmbyAccountEmbyUnavailable(t *testing.T) {
	embyClient := &stubAuthEmbyClient{getUserByIDErr: errors.New("无法连接到 Emby 服务器")}
	fake := &fakeBindingDB{
		currentUser: &models.User{ID: "admin_1", Username: "admin", Role: "admin"},
	}
	service := newAuthServiceForBinding(t, embyClient, fake)

	_, err := service.BindEmbyAccount("admin_1", &BindEmbyAccountRequest{
		EmbyID: "emby_1",
	})
	if !errors.Is(err, ErrEmbyServiceUnavailable) {
		t.Fatalf("expected emby unavailable error, got %v", err)
	}
	if fake.updateCalled {
		t.Fatalf("expected no DB write when Emby is unavailable")
	}
}

func TestBindEmbyAccountTargetNotFound(t *testing.T) {
	embyClient := &stubAuthEmbyClient{getUserByIDErr: embyint.ErrEmbyUserNotFound}
	fake := &fakeBindingDB{
		currentUser: &models.User{ID: "admin_1", Username: "admin", Role: "admin"},
	}
	service := newAuthServiceForBinding(t, embyClient, fake)

	_, err := service.BindEmbyAccount("admin_1", &BindEmbyAccountRequest{
		EmbyID: "emby_missing",
	})
	if !errors.Is(err, ErrEmbyBindingUserNotFound) {
		t.Fatalf("expected user not found error, got %v", err)
	}
	if fake.updateCalled {
		t.Fatalf("expected no DB write when target Emby user is missing")
	}
}

func TestBindEmbyAccountGetUserSuccessButEmptyEmbyID(t *testing.T) {
	embyClient := &stubAuthEmbyClient{getUserByIDResp: &embyint.EmbyUser{ID: "  "}}
	fake := &fakeBindingDB{
		currentUser: &models.User{ID: "admin_1", Username: "admin", Role: "admin"},
	}
	service := newAuthServiceForBinding(t, embyClient, fake)

	_, err := service.BindEmbyAccount("admin_1", &BindEmbyAccountRequest{
		EmbyID: "emby_1",
	})
	if !errors.Is(err, ErrEmbyBindingUserNotFound) {
		t.Fatalf("expected user not found error on empty emby user, got %v", err)
	}
	if fake.updateCalled {
		t.Fatalf("expected no DB write when Emby returns empty user ID")
	}
}

func TestBindEmbyAccountIdempotentWhenAlreadyBoundToSameTarget(t *testing.T) {
	embyClient := &stubAuthEmbyClient{getUserByIDResp: &embyint.EmbyUser{ID: "emby_1", Name: "ember_remote"}}
	fake := &fakeBindingDB{
		currentUser: &models.User{ID: "admin_1", Username: "admin", Role: "admin", EmbyID: "emby_1"},
	}
	service := newAuthServiceForBinding(t, embyClient, fake)

	resp, err := service.BindEmbyAccount("admin_1", &BindEmbyAccountRequest{
		EmbyID: "emby_1",
	})
	if err != nil {
		t.Fatalf("expected idempotent success, got %v", err)
	}
	if resp == nil || resp.EmbyID != "emby_1" || resp.EmbyUsername != "ember_remote" {
		t.Fatalf("unexpected response: %+v", resp)
	}
	if fake.updateCalled {
		t.Fatalf("idempotent path must not write to DB")
	}
}

func TestBindEmbyAccountAlreadyBoundToOtherTarget(t *testing.T) {
	embyClient := &stubAuthEmbyClient{getUserByIDResp: &embyint.EmbyUser{ID: "emby_new"}}
	fake := &fakeBindingDB{
		currentUser: &models.User{ID: "admin_1", Username: "admin", Role: "admin", EmbyID: "emby_old"},
	}
	service := newAuthServiceForBinding(t, embyClient, fake)

	_, err := service.BindEmbyAccount("admin_1", &BindEmbyAccountRequest{
		EmbyID: "emby_new",
	})
	if !errors.Is(err, ErrEmbyAlreadyBound) {
		t.Fatalf("expected already bound error, got %v", err)
	}
	if fake.updateCalled {
		t.Fatalf("must not write when current account already bound elsewhere")
	}
}

func TestBindEmbyAccountTargetOccupiedByOtherLocalUser(t *testing.T) {
	embyClient := &stubAuthEmbyClient{getUserByIDResp: &embyint.EmbyUser{ID: "emby_1", Name: "ember_remote"}}
	fake := &fakeBindingDB{
		currentUser:   &models.User{ID: "admin_1", Username: "admin", Role: "admin"},
		occupyingUser: &models.User{ID: "user_42", Username: "ember_user", EmbyID: "emby_1"},
	}
	service := newAuthServiceForBinding(t, embyClient, fake)

	_, err := service.BindEmbyAccount("admin_1", &BindEmbyAccountRequest{
		EmbyID: "emby_1",
	})
	if !IsEmbyUserOccupied(err) {
		t.Fatalf("expected occupied error, got %v", err)
	}
	occupied := &ErrEmbyUserOccupied{}
	if !errors.As(err, &occupied) || occupied.ConflictUsername != "ember_user" {
		t.Fatalf("expected conflict username ember_user, got %+v", err)
	}
	if fake.updateCalled {
		t.Fatalf("must not write when target occupied by other user")
	}
}

func TestBindEmbyAccountSuccess(t *testing.T) {
	embyClient := &stubAuthEmbyClient{getUserByIDResp: &embyint.EmbyUser{ID: "emby_1", Name: "ember_remote"}}
	fake := &fakeBindingDB{
		currentUser: &models.User{ID: "admin_1", Username: "admin", Role: "admin"},
	}
	service := newAuthServiceForBinding(t, embyClient, fake)

	resp, err := service.BindEmbyAccount("admin_1", &BindEmbyAccountRequest{
		EmbyID: "emby_1",
	})
	if err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	if resp == nil || resp.EmbyID != "emby_1" || resp.EmbyUsername != "ember_remote" {
		t.Fatalf("unexpected response: %+v", resp)
	}
	if !fake.updateCalled {
		t.Fatalf("expected DB write on success")
	}
	if fake.lastWrittenID != "admin_1" || fake.lastWrittenEmby != "emby_1" {
		t.Fatalf("unexpected DB write: id=%s emby=%s", fake.lastWrittenID, fake.lastWrittenEmby)
	}
}

func TestBindEmbyAccountConcurrentUniqueViolationMapsToOccupied(t *testing.T) {
	embyClient := &stubAuthEmbyClient{getUserByIDResp: &embyint.EmbyUser{ID: "emby_1", Name: "ember_remote"}}

	// 模拟"应用层 occupy 检查通过，但写库阶段被 DB 唯一索引兜底"。
	// 写后 fake 把 occupyingUser 设为冲突方，模拟另一管理员抢先写入。
	fake := &fakeBindingDB{
		currentUser: &models.User{ID: "admin_1", Username: "admin", Role: "admin"},
		updateErr: &pgconn.PgError{
			Code:           "23505",
			ConstraintName: "uniq_users_emby_id",
			Detail:         "Key (emby_id)=(emby_1) already exists.",
		},
	}
	service := newAuthServiceForBinding(t, embyClient, fake)

	// 在更新失败后，service 会再次调 findOccupyingUser；用一个 wrap 在闭包里切换状态
	originalFind := service.findOccupyingUserFn
	callCount := 0
	service.findOccupyingUserFn = func(embyID, excludeUserID string) (*models.User, error) {
		callCount++
		if callCount == 1 {
			return originalFind(embyID, excludeUserID)
		}
		return &models.User{ID: "admin_2", Username: "admin_other", EmbyID: embyID}, nil
	}

	_, err := service.BindEmbyAccount("admin_1", &BindEmbyAccountRequest{
		EmbyID: "emby_1",
	})
	if !IsEmbyUserOccupied(err) {
		t.Fatalf("expected occupied error from concurrent unique violation, got %v", err)
	}
	occupied := &ErrEmbyUserOccupied{}
	if !errors.As(err, &occupied) || occupied.ConflictUsername != "admin_other" {
		t.Fatalf("expected conflict username admin_other, got %+v", err)
	}
}

func TestUnbindEmbyAccountSuccess(t *testing.T) {
	fake := &fakeBindingDB{
		currentUser: &models.User{ID: "admin_1", Username: "admin", Role: "admin", EmbyID: "emby_1"},
	}
	service := &AuthService{
		findUserByIDForBindingFn: func(userID string) (*models.User, error) {
			copyUser := *fake.currentUser
			return &copyUser, nil
		},
		updateUserEmbyIDFn: func(userID, embyID string) error {
			fake.updateCalled = true
			fake.lastWrittenID = userID
			fake.lastWrittenEmby = embyID
			return nil
		},
		revokeUserTokensFn: func(_ context.Context, userID string, reason embytokenpkg.RevokeReason, actor string) (int64, error) {
			if userID != "admin_1" || reason != embytokenpkg.RevokeReasonEmbyUnbound || actor != "admin_1" {
				t.Fatalf("revoke input user=%s reason=%s actor=%s", userID, reason, actor)
			}
			if fake.updateCalled {
				t.Fatal("revoke must run before emby_id update")
			}
			return 1, nil
		},
	}

	if err := service.UnbindEmbyAccountWithContext(context.Background(), "admin_1", "admin_1"); err != nil {
		t.Fatalf("expected unbind success, got %v", err)
	}
	if !fake.updateCalled {
		t.Fatalf("expected DB write on unbind")
	}
	if fake.lastWrittenEmby != "" {
		t.Fatalf("expected emby_id cleared, got %q", fake.lastWrittenEmby)
	}
}

func TestUnbindEmbyAccountIdempotentWhenAlreadyEmpty(t *testing.T) {
	updateCalled := false
	revokeCalled := false
	service := &AuthService{
		findUserByIDForBindingFn: func(userID string) (*models.User, error) {
			return &models.User{ID: "admin_1", Username: "admin", Role: "admin"}, nil
		},
		updateUserEmbyIDFn: func(userID, embyID string) error {
			updateCalled = true
			return nil
		},
		revokeUserTokensFn: func(_ context.Context, userID string, reason embytokenpkg.RevokeReason, actor string) (int64, error) {
			revokeCalled = true
			return 0, nil
		},
	}

	if err := service.UnbindEmbyAccountWithContext(context.Background(), "admin_1", "admin_1"); err != nil {
		t.Fatalf("expected idempotent success, got %v", err)
	}
	if updateCalled {
		t.Fatalf("idempotent unbind must not write to DB")
	}
	if !revokeCalled {
		t.Fatal("idempotent unbind must revoke legacy active mappings")
	}
}

func TestUnbindEmbyAccountRevocationFailurePreventsUpdate(t *testing.T) {
	updateCalled := false
	service := &AuthService{
		findUserByIDForBindingFn: func(userID string) (*models.User, error) {
			return &models.User{ID: userID, EmbyID: "emby_1"}, nil
		},
		updateUserEmbyIDFn: func(string, string) error {
			updateCalled = true
			return nil
		},
		revokeUserTokensFn: func(context.Context, string, embytokenpkg.RevokeReason, string) (int64, error) {
			return 0, errors.New("revoke failed")
		},
	}
	if err := service.UnbindEmbyAccountWithContext(context.Background(), "admin_1", "admin_1"); !errors.Is(err, ErrEmbyTokenRevocation) {
		t.Fatalf("UnbindEmbyAccountWithContext() error = %v, want %v", err, ErrEmbyTokenRevocation)
	}
	if updateCalled {
		t.Fatal("emby_id update ran after revoke failure")
	}
}

func TestRebindAfterUnbindSuccess(t *testing.T) {
	embyClient := &stubAuthEmbyClient{getUserByIDResp: &embyint.EmbyUser{ID: "emby_2", Name: "ember_new"}}
	fake := &fakeBindingDB{
		currentUser: &models.User{ID: "admin_1", Username: "admin", Role: "admin"},
	}
	service := newAuthServiceForBinding(t, embyClient, fake)

	resp, err := service.BindEmbyAccount("admin_1", &BindEmbyAccountRequest{
		EmbyID: "emby_2",
	})
	if err != nil {
		t.Fatalf("expected rebind success, got %v", err)
	}
	if resp.EmbyID != "emby_2" {
		t.Fatalf("expected new emby id, got %q", resp.EmbyID)
	}
}

func TestIsEmbyIDUniqueViolation(t *testing.T) {
	if isEmbyIDUniqueViolation(nil) {
		t.Fatalf("nil error should not be unique violation")
	}
	if isEmbyIDUniqueViolation(errors.New("boom")) {
		t.Fatalf("non-pg error should not be unique violation")
	}
	if !isEmbyIDUniqueViolation(&pgconn.PgError{Code: "23505", ConstraintName: "uniq_users_emby_id"}) {
		t.Fatalf("constraint name match should be detected")
	}
	if !isEmbyIDUniqueViolation(&pgconn.PgError{Code: "23505", Detail: "Key (emby_id)=(x) already exists."}) {
		t.Fatalf("detail match should be detected")
	}
	if isEmbyIDUniqueViolation(&pgconn.PgError{Code: "23505", ConstraintName: "uq_users_email_lower"}) {
		t.Fatalf("unrelated constraint must not match")
	}
	if isEmbyIDUniqueViolation(&pgconn.PgError{Code: "23502"}) {
		t.Fatalf("non-23505 must not match")
	}
}
