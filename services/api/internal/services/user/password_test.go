package user

import (
	"errors"
	"testing"

	embyint "github.com/konghang/ember/backend/internal/integrations/emby"
	"github.com/konghang/ember/backend/internal/models"
)

type stubUserEmbyClient struct {
	authUserResp      *embyint.EmbyUser
	authUserErr       error
	updatePasswordErr error
	lastAuthUsername  string
	lastAuthPassword  string
	lastUpdateUserID  string
	lastUpdatePwd     string
}

func (s *stubUserEmbyClient) AuthenticateUser(username, password string) (*embyint.EmbyUser, error) {
	s.lastAuthUsername = username
	s.lastAuthPassword = password
	return s.authUserResp, s.authUserErr
}

func (s *stubUserEmbyClient) UpdateUserPassword(embyUserID, newPassword string) error {
	s.lastUpdateUserID = embyUserID
	s.lastUpdatePwd = newPassword
	return s.updatePasswordErr
}

func (s *stubUserEmbyClient) SetUserPolicy(embyUserID string, policy embyint.EmbyUserPolicy) error {
	return errors.New("unexpected SetUserPolicy call")
}

func (s *stubUserEmbyClient) DeleteUser(embyUserID string) error {
	return errors.New("unexpected DeleteUser call")
}

func TestResetPassword(t *testing.T) {
	t.Run("user not found", func(t *testing.T) {
		service := &UserService{
			findUserByID: func(userID string) (*models.User, error) {
				return nil, errors.New("not found")
			},
		}

		err := service.ResetPassword("user_1", "newpass123")
		if err == nil || err.Error() != "用户不存在" {
			t.Fatalf("expected user not found error, got %v", err)
		}
	})

	t.Run("emby password update failure", func(t *testing.T) {
		client := &stubUserEmbyClient{updatePasswordErr: errors.New("boom")}
		service := &UserService{
			findUserByID: func(userID string) (*models.User, error) {
				return &models.User{ID: userID, EmbyID: "emby_1"}, nil
			},
			newEmbyClient: func() embyClient { return client },
		}

		err := service.ResetPassword("user_1", "newpass123")
		if err == nil || err.Error() != "重置密码失败：boom" {
			t.Fatalf("expected emby update failure, got %v", err)
		}
	})

	t.Run("success updates local password and saves user", func(t *testing.T) {
		client := &stubUserEmbyClient{}
		saved := false
		service := &UserService{
			findUserByID: func(userID string) (*models.User, error) {
				return &models.User{ID: userID, EmbyID: "emby_1"}, nil
			},
			newEmbyClient: func() embyClient { return client },
			saveUser: func(user *models.User) error {
				saved = true
				return nil
			},
		}

		err := service.ResetPassword("user_1", "newpass123")
		if err != nil {
			t.Fatalf("expected success, got %v", err)
		}
		if client.lastUpdateUserID != "emby_1" || client.lastUpdatePwd != "newpass123" {
			t.Fatalf("unexpected emby update payload: user=%q pwd=%q", client.lastUpdateUserID, client.lastUpdatePwd)
		}
		if !saved {
			t.Fatalf("expected saveUser to be called")
		}
	})
}

func TestUpdatePassword(t *testing.T) {
	t.Run("admin wrong old password", func(t *testing.T) {
		admin := &models.User{ID: "admin_1", Username: "admin", Role: "admin"}
		if err := admin.SetPassword("oldpass"); err != nil {
			t.Fatalf("failed to set admin password: %v", err)
		}
		service := &UserService{
			findUserByID: func(userID string) (*models.User, error) {
				return admin, nil
			},
		}

		err := service.UpdatePassword("admin_1", &UpdatePasswordRequest{
			OldPassword: "wrong",
			NewPassword: "newpass123",
		})
		if err == nil || err.Error() != "旧密码错误" {
			t.Fatalf("expected wrong old password error, got %v", err)
		}
	})

	t.Run("admin success updates local password", func(t *testing.T) {
		admin := &models.User{ID: "admin_1", Username: "admin", Role: "admin"}
		if err := admin.SetPassword("oldpass"); err != nil {
			t.Fatalf("failed to set admin password: %v", err)
		}
		saved := false
		service := &UserService{
			findUserByID: func(userID string) (*models.User, error) {
				return admin, nil
			},
			saveUser: func(user *models.User) error {
				saved = true
				return nil
			},
		}

		err := service.UpdatePassword("admin_1", &UpdatePasswordRequest{
			OldPassword: "oldpass",
			NewPassword: "newpass123",
		})
		if err != nil {
			t.Fatalf("expected success, got %v", err)
		}
		if !saved {
			t.Fatalf("expected saveUser to be called")
		}
		if !admin.CheckPassword("newpass123") {
			t.Fatalf("expected admin password hash to be updated")
		}
	})

	t.Run("member emby auth success updates emby and local password", func(t *testing.T) {
		member := &models.User{ID: "user_1", Username: "ember", Role: "user", EmbyID: "emby_1"}
		client := &stubUserEmbyClient{
			authUserResp: &embyint.EmbyUser{ID: "emby_1"},
		}
		saved := false
		service := &UserService{
			findUserByID: func(userID string) (*models.User, error) {
				return member, nil
			},
			newEmbyClient: func() embyClient { return client },
			saveUser: func(user *models.User) error {
				saved = true
				return nil
			},
		}

		err := service.UpdatePassword("user_1", &UpdatePasswordRequest{
			OldPassword: "oldpass",
			NewPassword: "newpass123",
		})
		if err != nil {
			t.Fatalf("expected success, got %v", err)
		}
		if client.lastAuthUsername != "ember" || client.lastAuthPassword != "oldpass" {
			t.Fatalf("unexpected emby auth payload: %q %q", client.lastAuthUsername, client.lastAuthPassword)
		}
		if client.lastUpdateUserID != "emby_1" || client.lastUpdatePwd != "newpass123" {
			t.Fatalf("unexpected emby update payload: user=%q pwd=%q", client.lastUpdateUserID, client.lastUpdatePwd)
		}
		if !saved || !member.CheckPassword("newpass123") {
			t.Fatalf("expected local password update and save")
		}
	})

	t.Run("member local fallback without emby id fails before update", func(t *testing.T) {
		member := &models.User{ID: "user_2", Username: "ember", Role: "user"}
		if err := member.SetPassword("oldpass"); err != nil {
			t.Fatalf("failed to set member password: %v", err)
		}
		client := &stubUserEmbyClient{authUserErr: errors.New("boom")}
		service := &UserService{
			findUserByID: func(userID string) (*models.User, error) {
				return member, nil
			},
			newEmbyClient: func() embyClient { return client },
		}

		err := service.UpdatePassword("user_2", &UpdatePasswordRequest{
			OldPassword: "oldpass",
			NewPassword: "newpass123",
		})
		if err == nil || err.Error() != "密码更新失败：用户缺少 Emby ID" {
			t.Fatalf("expected missing emby id error, got %v", err)
		}
	})
}
