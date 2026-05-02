package user

import (
	"errors"
	"testing"

	"github.com/konghang/ember/backend/internal/models"
	"gorm.io/gorm"
)

type stubUserEmailVerifier struct {
	lastEmail            string
	lastCode             string
	lastCodeType         string
	err                  error
	domainCheckCalled    bool
	domainCheckLastEmail string
	domainErr            error
}

func (s *stubUserEmailVerifier) VerifyCode(email, code, codeType string) error {
	s.lastEmail = email
	s.lastCode = code
	s.lastCodeType = codeType
	return s.err
}

func (s *stubUserEmailVerifier) CheckCode(email, code, codeType string) error {
	return s.VerifyCode(email, code, codeType)
}

func (s *stubUserEmailVerifier) ConsumeCodeTx(_ *gorm.DB, email, code, codeType string) error {
	return s.VerifyCode(email, code, codeType)
}

func (s *stubUserEmailVerifier) IsRegistrationEmailAllowed(email string) error {
	s.domainCheckCalled = true
	s.domainCheckLastEmail = email
	return s.domainErr
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

	t.Run("success updates emby and saves user", func(t *testing.T) {
		verifier := &stubUserEmailVerifier{}
		client := &stubUserEmbyClient{}
		saved := false
		service := NewUserServiceWithEmailVerifier(verifier)
		service.findUserByEmail = func(email string) (*models.User, error) {
			return &models.User{ID: "user_1", Username: "ember", EmbyID: "emby_1"}, nil
		}
		service.newEmbyClient = func() embyClient { return client }
		service.saveUser = func(user *models.User) error {
			saved = true
			return nil
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
	})
}
