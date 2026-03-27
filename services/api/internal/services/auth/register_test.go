package auth

import (
	"errors"
	"testing"
	"time"

	notifierint "github.com/konghang/ember/backend/internal/integrations/notifier"
	"github.com/konghang/ember/backend/internal/models"
)

type stubAuthEmailVerifier struct {
	enabled       bool
	lastEmail     string
	lastCode      string
	lastCodeType  string
	verifyCalled  bool
	verifyCodeErr error
}

func (s *stubAuthEmailVerifier) IsEnabled() bool {
	return s.enabled
}

func (s *stubAuthEmailVerifier) VerifyCode(email, code, codeType string) error {
	s.verifyCalled = true
	s.lastEmail = email
	s.lastCode = code
	s.lastCodeType = codeType
	return s.verifyCodeErr
}

type stubAuthRegistrationNotifier struct {
	configured bool
	calledCh   chan notifierint.RegistrationNotification
}

func (s *stubAuthRegistrationNotifier) IsConfigured() bool {
	return s.configured
}

func (s *stubAuthRegistrationNotifier) NotifyNewRegistration(data notifierint.RegistrationNotification) {
	if s.calledCh != nil {
		s.calledCh <- data
	}
}

func TestValidateRegisterRequest(t *testing.T) {
	service := &AuthService{}

	tests := []struct {
		name     string
		req      RegisterUserRequest
		wantErr  string
		wantUser string
	}{
		{
			name:     "trimmed username valid",
			req:      RegisterUserRequest{Username: "  ember123  "},
			wantUser: "ember123",
		},
		{
			name:    "username too short",
			req:     RegisterUserRequest{Username: "ab"},
			wantErr: "用户名长度必须为 3-50 位",
		},
		{
			name:    "username invalid chars",
			req:     RegisterUserRequest{Username: "ember-123"},
			wantErr: "用户名只能包含字母和数字",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := tc.req
			err := service.validateRegisterRequest(&req)
			if tc.wantErr != "" {
				if err == nil || err.Error() != tc.wantErr {
					t.Fatalf("expected error %q, got %v", tc.wantErr, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if req.Username != tc.wantUser {
				t.Fatalf("expected trimmed username %q, got %q", tc.wantUser, req.Username)
			}
		})
	}
}

func TestVerifyRegisterEmailCode(t *testing.T) {
	t.Run("disabled email verification skips validation", func(t *testing.T) {
		emailVerifier := &stubAuthEmailVerifier{enabled: false}
		service := &AuthService{emailService: emailVerifier}

		err := service.verifyRegisterEmailCode(&RegisterUserRequest{
			Email:     "ember@example.com",
			EmailCode: "",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if emailVerifier.verifyCalled {
			t.Fatalf("expected verify not to be called when email verification is disabled")
		}
	})

	t.Run("enabled email verification requires code", func(t *testing.T) {
		emailVerifier := &stubAuthEmailVerifier{enabled: true}
		service := &AuthService{emailService: emailVerifier}

		err := service.verifyRegisterEmailCode(&RegisterUserRequest{
			Email: "ember@example.com",
		})
		if err == nil || err.Error() != "请先获取邮箱验证码" {
			t.Fatalf("expected missing code error, got %v", err)
		}
		if emailVerifier.verifyCalled {
			t.Fatalf("expected verify not to be called when email code is missing")
		}
	})

	t.Run("enabled email verification delegates to verifier", func(t *testing.T) {
		emailVerifier := &stubAuthEmailVerifier{enabled: true}
		service := &AuthService{emailService: emailVerifier}

		err := service.verifyRegisterEmailCode(&RegisterUserRequest{
			Email:     "ember@example.com",
			EmailCode: "123456",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !emailVerifier.verifyCalled {
			t.Fatalf("expected verify to be called")
		}
		if emailVerifier.lastEmail != "ember@example.com" || emailVerifier.lastCode != "123456" || emailVerifier.lastCodeType != models.VerificationTypeRegister {
			t.Fatalf("unexpected verify payload: email=%q code=%q type=%q", emailVerifier.lastEmail, emailVerifier.lastCode, emailVerifier.lastCodeType)
		}
	})

	t.Run("enabled email verification returns verifier error", func(t *testing.T) {
		expectedErr := errors.New("boom")
		emailVerifier := &stubAuthEmailVerifier{enabled: true, verifyCodeErr: expectedErr}
		service := &AuthService{emailService: emailVerifier}

		err := service.verifyRegisterEmailCode(&RegisterUserRequest{
			Email:     "ember@example.com",
			EmailCode: "123456",
		})
		if !errors.Is(err, expectedErr) {
			t.Fatalf("expected delegated error, got %v", err)
		}
	})
}

func TestNotifyNewRegistration(t *testing.T) {
	t.Run("skips when notifier is not configured", func(t *testing.T) {
		notifier := &stubAuthRegistrationNotifier{
			configured: false,
			calledCh:   make(chan notifierint.RegistrationNotification, 1),
		}
		service := &AuthService{notifier: notifier}
		service.notifyNewRegistration(models.User{Username: "ember"}, "open")

		select {
		case payload := <-notifier.calledCh:
			t.Fatalf("expected notifier not to be called, got payload %+v", payload)
		case <-time.After(50 * time.Millisecond):
		}
	})

	t.Run("configured notifier receives expected payload", func(t *testing.T) {
		notifier := &stubAuthRegistrationNotifier{
			configured: true,
			calledCh:   make(chan notifierint.RegistrationNotification, 1),
		}
		service := &AuthService{notifier: notifier}

		expiresAtTime := time.Date(2026, 3, 27, 10, 30, 0, 0, time.UTC)
		service.notifyNewRegistration(models.User{
			ID:        "user_1",
			Username:  "ember",
			Email:     "ember@example.com",
			EmbyID:    "emby_1",
			ExpiresAt: &expiresAtTime,
		}, "invite")

		select {
		case payload := <-notifier.calledCh:
			if payload.ID != "user_1" || payload.UserName != "ember" || payload.Email != "ember@example.com" || payload.EmbyID != "emby_1" || payload.RegistrationMode != "invite" {
				t.Fatalf("unexpected payload: %+v", payload)
			}
			if payload.ExpiresAt == nil || *payload.ExpiresAt != "2026-03-27 10:30:00 UTC" {
				t.Fatalf("unexpected expiresAt payload: %+v", payload.ExpiresAt)
			}
		case <-time.After(200 * time.Millisecond):
			t.Fatal("expected notifier to be called")
		}
	})
}
