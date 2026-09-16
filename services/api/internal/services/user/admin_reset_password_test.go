package user

import (
	"errors"
	"strings"
	"testing"

	"github.com/konghang/ember/backend/internal/common"
	embyint "github.com/konghang/ember/backend/internal/integrations/emby"
	"github.com/konghang/ember/backend/internal/models"
)

func TestResetPasswordAdminLocalOnly(t *testing.T) {
	t.Run("admin without emby config resets local password", func(t *testing.T) {
		admin := &models.User{
			ID:                    "admin_1",
			Username:              "admin",
			Role:                  "admin",
			PasswordResetRequired: true,
		}
		if err := admin.SetPassword("oldpass"); err != nil {
			t.Fatalf("failed to seed admin password: %v", err)
		}
		oldHash := admin.Password
		oldPwdSig := common.ComputePasswordSignature(oldHash)
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

		err := service.ResetPassword("admin_1", "newpass123")
		if err != nil {
			t.Fatalf("expected admin reset success, got %v", err)
		}
		if !saved {
			t.Fatalf("expected local admin password to be saved")
		}
		if admin.PasswordResetRequired {
			t.Fatalf("expected password reset flag to be cleared")
		}
		if !admin.CheckPassword("newpass123") {
			t.Fatalf("expected admin password hash to be updated")
		}
		if admin.CheckPassword("oldpass") {
			t.Fatalf("expected old admin password to stop matching")
		}
		if oldPwdSig == common.ComputePasswordSignature(admin.Password) {
			t.Fatalf("expected old JWT pwdSig to become invalid after hash update")
		}
	})

	t.Run("bound admin still resets local password only", func(t *testing.T) {
		admin := &models.User{
			ID:                    "admin_2",
			Username:              "admin2",
			Role:                  "admin",
			EmbyID:                "emby_admin",
			PasswordResetRequired: true,
		}
		if err := admin.SetPassword("oldpass"); err != nil {
			t.Fatalf("failed to seed admin password: %v", err)
		}
		client := &stubUserEmbyClient{updatePasswordErr: errors.New("emby should not be called")}
		service := &UserService{
			findUserByID: func(userID string) (*models.User, error) {
				return admin, nil
			},
			newEmbyClient: func() embyClient { return client },
			saveUser: func(user *models.User) error {
				return nil
			},
		}

		err := service.ResetPassword("admin_2", "newpass123")
		if err != nil {
			t.Fatalf("expected bound admin reset success, got %v", err)
		}
		if client.lastUpdateUserID != "" {
			t.Fatalf("expected emby password update to be skipped, got user %q", client.lastUpdateUserID)
		}
		if admin.PasswordResetRequired {
			t.Fatalf("expected password reset flag to be cleared")
		}
		if !admin.CheckPassword("newpass123") {
			t.Fatalf("expected admin password hash to be updated")
		}
	})

	t.Run("admin local save failure is reported", func(t *testing.T) {
		admin := &models.User{
			ID:                    "admin_3",
			Username:              "admin3",
			Role:                  "admin",
			PasswordResetRequired: true,
		}
		if err := admin.SetPassword("oldpass"); err != nil {
			t.Fatalf("failed to seed admin password: %v", err)
		}
		service := &UserService{
			findUserByID: func(userID string) (*models.User, error) {
				return admin, nil
			},
			saveUser: func(user *models.User) error {
				return errors.New("db down")
			},
		}

		err := service.ResetPassword("admin_3", "newpass123")
		if err == nil || err.Error() != "重置密码失败：本地密码保存失败" {
			t.Fatalf("expected local save failure, got %v", err)
		}
		if admin.PasswordResetRequired {
			t.Fatalf("expected password reset flag to be cleared before save attempt")
		}
		if !admin.CheckPassword("newpass123") {
			t.Fatalf("expected admin password hash to be updated before save attempt")
		}
	})
}

func TestResetPasswordMemberSync(t *testing.T) {
	t.Run("missing emby id returns explicit error", func(t *testing.T) {
		member := &models.User{ID: "user_1", Username: "ember", Role: "user"}
		service := &UserService{
			findUserByID: func(userID string) (*models.User, error) {
				return member, nil
			},
		}

		err := service.ResetPassword("user_1", "newpass123")
		if err == nil || err.Error() != "重置密码失败：用户缺少 Emby ID" {
			t.Fatalf("expected missing emby id error, got %v", err)
		}
	})

	t.Run("success syncs emby and local password", func(t *testing.T) {
		member := &models.User{
			ID:                    "user_2",
			Username:              "ember",
			Role:                  "user",
			EmbyID:                "emby_1",
			PasswordResetRequired: true,
		}
		client := &stubUserEmbyClient{}
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

		err := service.ResetPassword("user_2", "newpass123")
		if err != nil {
			t.Fatalf("expected member reset success, got %v", err)
		}
		if client.lastUpdateUserID != "emby_1" || client.lastUpdatePwd != "newpass123" {
			t.Fatalf("unexpected emby update payload: user=%q pwd=%q", client.lastUpdateUserID, client.lastUpdatePwd)
		}
		if !saved || !member.CheckPassword("newpass123") {
			t.Fatalf("expected local password update and save")
		}
		if member.PasswordResetRequired {
			t.Fatalf("expected password reset flag to be cleared")
		}
	})

	t.Run("remote failure stops local save", func(t *testing.T) {
		member := &models.User{ID: "user_3", Username: "ember", Role: "user", EmbyID: "emby_1"}
		client := &stubUserEmbyClient{updatePasswordErr: errors.New("remote unavailable")}
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

		err := service.ResetPassword("user_3", "newpass123")
		if err == nil || !strings.Contains(err.Error(), "remote unavailable") {
			t.Fatalf("expected remote failure, got %v", err)
		}
		if saved {
			t.Fatalf("expected local save to be skipped after remote failure")
		}
		if member.Password != "" {
			t.Fatalf("expected local password hash to remain unchanged")
		}
	})

	t.Run("local save failure is reported after remote update", func(t *testing.T) {
		member := &models.User{ID: "user_4", Username: "ember", Role: "user", EmbyID: "emby_1"}
		client := &stubUserEmbyClient{}
		service := &UserService{
			findUserByID: func(userID string) (*models.User, error) {
				return member, nil
			},
			newEmbyClient: func() embyClient { return client },
			saveUser: func(user *models.User) error {
				return errors.New("db down")
			},
		}

		err := service.ResetPassword("user_4", "newpass123")
		if err == nil || err.Error() != "重置密码失败：本地密码保存失败" {
			t.Fatalf("expected local save failure, got %v", err)
		}
		if client.lastUpdateUserID != "emby_1" {
			t.Fatalf("expected remote password update before save failure")
		}
	})

	t.Run("hash update invalidates old password signature", func(t *testing.T) {
		member := &models.User{ID: "user_5", Username: "ember", Role: "user", EmbyID: "emby_1"}
		if err := member.SetPassword("oldpass"); err != nil {
			t.Fatalf("failed to seed member password: %v", err)
		}
		oldPwdSig := common.ComputePasswordSignature(member.Password)
		client := &stubUserEmbyClient{authUserResp: &embyint.EmbyUser{ID: "emby_1"}}
		service := &UserService{
			findUserByID: func(userID string) (*models.User, error) {
				return member, nil
			},
			newEmbyClient: func() embyClient { return client },
			saveUser: func(user *models.User) error {
				return nil
			},
		}

		err := service.ResetPassword("user_5", "newpass123")
		if err != nil {
			t.Fatalf("expected member reset success, got %v", err)
		}
		if oldPwdSig == common.ComputePasswordSignature(member.Password) {
			t.Fatalf("expected old JWT pwdSig to become invalid after hash update")
		}
	})
}
