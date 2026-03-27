package user

import (
	"errors"
	"testing"

	"github.com/konghang/ember/backend/internal/models"
	emailpkg "github.com/konghang/ember/backend/internal/services/email"
)

type stubUserEmailVerifier struct {
	lastEmail    string
	lastCode     string
	lastCodeType string
	err          error
}

func (s *stubUserEmailVerifier) VerifyCode(email, code, codeType string) error {
	s.lastEmail = email
	s.lastCode = code
	s.lastCodeType = codeType
	return s.err
}

func TestResetPasswordByCode(t *testing.T) {
	t.Run("verification error is returned directly", func(t *testing.T) {
		expectedErr := errors.New("boom")
		verifier := &stubUserEmailVerifier{err: expectedErr}
		service := NewUserServiceWithEmailVerifier(verifier)

		err := service.ResetPasswordByCode(&ResetPasswordByCodeRequest{
			Email:       "ember@example.com",
			Code:        "123456",
			NewPassword: "newpass123",
		})
		if !errors.Is(err, expectedErr) {
			t.Fatalf("expected verifier error, got %v", err)
		}
		if verifier.lastEmail != "ember@example.com" || verifier.lastCode != "123456" || verifier.lastCodeType != models.VerificationTypeReset {
			t.Fatalf("unexpected verifier payload: %+v", verifier)
		}
	})

	t.Run("user not found after verification", func(t *testing.T) {
		verifier := &stubUserEmailVerifier{}
		service := NewUserServiceWithEmailVerifier(verifier)
		service.findUserByEmail = func(email string) (*models.User, error) {
			return nil, errors.New("not found")
		}

		err := service.ResetPasswordByCode(&ResetPasswordByCodeRequest{
			Email:       "ember@example.com",
			Code:        "123456",
			NewPassword: "newpass123",
		})
		if err == nil || err.Error() != "用户不存在" {
			t.Fatalf("expected user not found error, got %v", err)
		}
	})

	t.Run("success updates emby, saves user, and clears reset codes", func(t *testing.T) {
		verifier := &stubUserEmailVerifier{}
		client := &stubUserEmbyClient{}
		saved := false
		clearedEmail := ""
		service := NewUserServiceWithEmailVerifier(verifier)
		service.findUserByEmail = func(email string) (*models.User, error) {
			return &models.User{ID: "user_1", Username: "ember", EmbyID: "emby_1"}, nil
		}
		service.newEmbyClient = func() embyClient { return client }
		service.saveUser = func(user *models.User) error {
			saved = true
			return nil
		}
		service.deleteResetCodes = func(email string) (int64, error) {
			clearedEmail = email
			return 1, nil
		}

		err := service.ResetPasswordByCode(&ResetPasswordByCodeRequest{
			Email:       "ember@example.com",
			Code:        "123456",
			NewPassword: "newpass123",
		})
		if err != nil {
			t.Fatalf("expected success, got %v", err)
		}
		if client.lastUpdateUserID != "emby_1" || client.lastUpdatePwd != "newpass123" {
			t.Fatalf("unexpected emby update payload: user=%q pwd=%q", client.lastUpdateUserID, client.lastUpdatePwd)
		}
		if !saved {
			t.Fatalf("expected saveUser to be called")
		}
		if clearedEmail != "ember@example.com" {
			t.Fatalf("expected reset codes to be cleared for email, got %q", clearedEmail)
		}
	})

	t.Run("zero cleared reset codes returns email code invalid", func(t *testing.T) {
		verifier := &stubUserEmailVerifier{}
		service := NewUserServiceWithEmailVerifier(verifier)
		service.findUserByEmail = func(email string) (*models.User, error) {
			return &models.User{ID: "user_1", Username: "ember"}, nil
		}
		service.saveUser = func(user *models.User) error { return nil }
		service.deleteResetCodes = func(email string) (int64, error) {
			return 0, nil
		}

		err := service.ResetPasswordByCode(&ResetPasswordByCodeRequest{
			Email:       "ember@example.com",
			Code:        "123456",
			NewPassword: "newpass123",
		})
		if !errors.Is(err, emailpkg.ErrEmailCodeInvalid) {
			t.Fatalf("expected ErrEmailCodeInvalid, got %v", err)
		}
	})
}
