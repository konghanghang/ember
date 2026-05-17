package auth

import (
	"errors"
	"testing"

	embyint "github.com/konghang/ember/backend/internal/integrations/emby"
	"github.com/konghang/ember/backend/internal/models"
)

type stubAuthEmbyClient struct {
	authUserResp      *embyint.EmbyUser
	authUserErr       error
	updatePasswordErr error
	lastAuthUsername  string
	lastAuthPassword  string
	lastUpdateUserID  string
	lastUpdatePwd     string
}

func (s *stubAuthEmbyClient) AuthenticateUser(username, password string) (*embyint.EmbyUser, error) {
	s.lastAuthUsername = username
	s.lastAuthPassword = password
	return s.authUserResp, s.authUserErr
}

func (s *stubAuthEmbyClient) UpdateUserPassword(embyUserID, newPassword string) error {
	s.lastUpdateUserID = embyUserID
	s.lastUpdatePwd = newPassword
	return s.updatePasswordErr
}

func (s *stubAuthEmbyClient) CreateEmbyUser(username, password string) (*embyint.EmbyUser, error) {
	return nil, errors.New("unexpected CreateEmbyUser call")
}

func (s *stubAuthEmbyClient) DeleteUser(embyUserID string) error {
	return errors.New("unexpected DeleteUser call")
}

func (s *stubAuthEmbyClient) GetUserPolicyRaw(embyUserID string) (map[string]any, error) {
	return nil, errors.New("unexpected GetUserPolicyRaw call")
}

func (s *stubAuthEmbyClient) PatchUserPolicyFields(targetUserID string, sourcePolicy map[string]any, fields []string) error {
	return errors.New("unexpected PatchUserPolicyFields call")
}

func TestAuthenticateLoginUserAdmin(t *testing.T) {
	service := NewAuthService()
	admin := &models.User{
		ID:       "admin_1",
		Username: "admin",
		Role:     "admin",
		IsActive: true,
	}
	if err := admin.SetPassword("secret123"); err != nil {
		t.Fatalf("failed to set password: %v", err)
	}

	if err := service.authenticateLoginUser(admin, "secret123"); err != nil {
		t.Fatalf("expected admin login to succeed, got %v", err)
	}

	if err := service.authenticateLoginUser(admin, "wrong"); err == nil || err.Error() != "用户名或密码错误" {
		t.Fatalf("expected invalid credential error, got %v", err)
	}
}

func TestAuthenticateLoginUserEmbySuccessSyncsLocalHash(t *testing.T) {
	embyClient := &stubAuthEmbyClient{
		authUserResp: &embyint.EmbyUser{ID: "emby_1"},
	}
	saved := false
	service := &AuthService{
		newEmbyClient: func() authEmbyClient { return embyClient },
		saveUser: func(user *models.User) error {
			saved = true
			return nil
		},
	}

	user := &models.User{
		ID:       "user_1",
		Username: "ember",
		Role:     "user",
		EmbyID:   "emby_1",
		IsActive: true,
	}

	if err := service.authenticateLoginUser(user, "pass1234"); err != nil {
		t.Fatalf("expected emby login success, got %v", err)
	}
	if embyClient.lastAuthUsername != "ember" || embyClient.lastAuthPassword != "pass1234" {
		t.Fatalf("unexpected emby auth payload: %q %q", embyClient.lastAuthUsername, embyClient.lastAuthPassword)
	}
	if !saved {
		t.Fatalf("expected local hash save to be triggered")
	}
	if !user.CheckPassword("pass1234") {
		t.Fatalf("expected local password hash to be updated")
	}
}

func TestAuthenticateLoginUserFallsBackToLocalPassword(t *testing.T) {
	embyClient := &stubAuthEmbyClient{
		authUserErr: errors.New("boom"),
	}
	service := &AuthService{
		newEmbyClient: func() authEmbyClient { return embyClient },
		saveUser: func(user *models.User) error {
			t.Fatalf("saveUser should not be called on local fallback path")
			return nil
		},
	}

	user := &models.User{
		ID:       "user_2",
		Username: "ember",
		Role:     "user",
		EmbyID:   "emby_2",
		IsActive: true,
	}
	if err := user.SetPassword("pass1234"); err != nil {
		t.Fatalf("failed to set password: %v", err)
	}

	if err := service.authenticateLoginUser(user, "pass1234"); err != nil {
		t.Fatalf("expected local fallback login to succeed, got %v", err)
	}
	if embyClient.lastUpdateUserID != "" || embyClient.lastUpdatePwd != "" {
		t.Fatalf("expected no UpdateUserPassword call on fallback path, got user=%q pwd=%q", embyClient.lastUpdateUserID, embyClient.lastUpdatePwd)
	}

	if err := service.authenticateLoginUser(user, "wrong"); err == nil || err.Error() != "用户名或密码错误" {
		t.Fatalf("expected invalid credential error on wrong fallback password, got %v", err)
	}
}

func TestAuthenticateLoginUserRejectsEmbyIDMismatch(t *testing.T) {
	embyClient := &stubAuthEmbyClient{
		authUserResp: &embyint.EmbyUser{ID: "emby_remote_drift"},
	}
	service := &AuthService{
		newEmbyClient: func() authEmbyClient { return embyClient },
		saveUser: func(user *models.User) error {
			t.Fatalf("saveUser should not be called when EmbyID mismatches")
			return nil
		},
	}

	user := &models.User{
		ID:       "user_3",
		Username: "ember",
		Role:     "user",
		EmbyID:   "emby_local_original",
		IsActive: true,
	}
	if err := user.SetPassword("pass1234"); err != nil {
		t.Fatalf("failed to set password: %v", err)
	}

	err := service.authenticateLoginUser(user, "pass1234")
	if err == nil || err.Error() != "用户名或密码错误" {
		t.Fatalf("expected EmbyID mismatch to be rejected, got %v", err)
	}
	if embyClient.lastUpdateUserID != "" || embyClient.lastUpdatePwd != "" {
		t.Fatalf("expected no UpdateUserPassword call on mismatch, got user=%q pwd=%q", embyClient.lastUpdateUserID, embyClient.lastUpdatePwd)
	}
}

func TestAuthenticateLoginUserRejectsInactiveUser(t *testing.T) {
	service := NewAuthService()
	user := &models.User{
		ID:       "user_4",
		Username: "ember",
		Role:     "user",
		IsActive: false,
	}

	err := service.authenticateLoginUser(user, "pass1234")
	if err == nil || err.Error() != "用户名或密码错误" {
		t.Fatalf("expected inactive user to be rejected, got %v", err)
	}
}
