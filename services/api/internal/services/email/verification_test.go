package email

import (
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
