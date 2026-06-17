package email

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/konghang/ember/backend/internal/models"
)

func TestEvaluateVerificationCode(t *testing.T) {
	now := time.Now().UTC()

	tests := []struct {
		name         string
		verification *models.EmailVerification
		code         string
		wantErr      error
	}{
		{
			name:         "nil verification rejected",
			verification: nil,
			code:         "123456",
			wantErr:      ErrEmailCodeInvalid,
		},
		{
			name: "expired code rejected",
			verification: &models.EmailVerification{
				Code:      "123456",
				ExpiresAt: now.Add(-time.Minute),
			},
			code:    "123456",
			wantErr: ErrEmailCodeInvalid,
		},
		{
			name: "code mismatch rejected",
			verification: &models.EmailVerification{
				Code:      "654321",
				ExpiresAt: now.Add(time.Minute),
			},
			code:    "123456",
			wantErr: ErrEmailCodeInvalid,
		},
		{
			name: "valid code passes",
			verification: &models.EmailVerification{
				Code:      "123456",
				ExpiresAt: now.Add(time.Minute),
			},
			code: "123456",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := evaluateVerificationCode(tc.verification, tc.code)
			if tc.wantErr == nil && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tc.wantErr != nil && err != tc.wantErr {
				t.Fatalf("expected %v, got %v", tc.wantErr, err)
			}
		})
	}
}

func TestSendEmailChangeNotificationSkipsBlankOldEmail(t *testing.T) {
	service := NewEmailServiceWithConfig(newConfiguredStubReader(nil))
	called := false
	service.sendEmailFunc = func(to, subject, body string) error {
		called = true
		return nil
	}

	if err := service.SendEmailChangeNotification("  ", "new@example.com"); err != nil {
		t.Fatalf("expected blank old email to be ignored, got %v", err)
	}
	if called {
		t.Fatal("sendEmailFunc must not be called for blank old email")
	}
}

func TestSendEmailChangeNotificationSendsOldAddressWarning(t *testing.T) {
	service := NewEmailServiceWithConfig(newConfiguredStubReader(nil))
	var sentTo string
	var sentSubject string
	var sentBody string
	service.sendEmailFunc = func(to, subject, body string) error {
		sentTo = to
		sentSubject = subject
		sentBody = body
		return nil
	}

	err := service.SendEmailChangeNotification(" old@example.com ", "new@example.com")
	if err != nil {
		t.Fatalf("expected notification to be sent, got %v", err)
	}
	if sentTo != "old@example.com" {
		t.Fatalf("expected notification to old email, got %q", sentTo)
	}
	if sentSubject != "Ember 联系邮箱已变更" {
		t.Fatalf("unexpected subject: %q", sentSubject)
	}
	if !strings.Contains(sentBody, "old@example.com") || !strings.Contains(sentBody, "new@example.com") {
		t.Fatalf("expected body to mention old and new emails, got %q", sentBody)
	}
}

func TestSendEmailChangeNotificationReturnsSendError(t *testing.T) {
	service := NewEmailServiceWithConfig(newConfiguredStubReader(nil))
	expectedErr := errors.New("smtp unavailable")
	service.sendEmailFunc = func(to, subject, body string) error {
		return expectedErr
	}

	err := service.SendEmailChangeNotification("old@example.com", "new@example.com")
	if !errors.Is(err, expectedErr) {
		t.Fatalf("expected send error, got %v", err)
	}
}
