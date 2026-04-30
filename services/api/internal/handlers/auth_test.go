package handlers

import (
	"errors"
	"testing"

	authpkg "github.com/konghang/ember/backend/internal/services/auth"
	emailpkg "github.com/konghang/ember/backend/internal/services/email"
	redemptionpkg "github.com/konghang/ember/backend/internal/services/redemption"
	userpkg "github.com/konghang/ember/backend/internal/services/user"
)

func TestShouldExposeResetCodeSendError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "smtp not configured is exposed",
			err:  emailpkg.ErrEmailNotConfigured,
			want: true,
		},
		{
			name: "not registered is folded",
			err:  emailpkg.ErrEmailNotRegistered,
		},
		{
			name: "rate limit is folded",
			err:  emailpkg.ErrEmailCodeRateLimit,
		},
		{
			name: "send failure is folded",
			err:  emailpkg.ErrEmailSendFailed,
		},
		{
			name: "wrapped smtp not configured is exposed",
			err:  errors.Join(errors.New("wrapper"), emailpkg.ErrEmailNotConfigured),
			want: true,
		},
		{
			name: "nil is folded",
			err:  nil,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := shouldExposeResetCodeSendError(tc.err); got != tc.want {
				t.Fatalf("expected %t, got %t", tc.want, got)
			}
		})
	}
}

func TestIsAuthRegisterBadRequest(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{name: "username length", err: userpkg.ErrUsernameLengthInvalid, want: true},
		{name: "username charset", err: userpkg.ErrUsernameCharsetInvalid, want: true},
		{name: "username exists", err: userpkg.ErrUsernameAlreadyExists, want: true},
		{name: "email exists", err: emailpkg.ErrEmailAlreadyRegistered, want: true},
		{name: "email code required", err: authpkg.ErrRegisterEmailCodeRequired, want: true},
		{name: "invite code required", err: authpkg.ErrRegisterInviteCodeRequired, want: true},
		{name: "email code invalid", err: emailpkg.ErrEmailCodeInvalid, want: true},
		{name: "redemption not found", err: redemptionpkg.ErrRedemptionCodeNotFound, want: true},
		{name: "nil", err: nil, want: false},
		{name: "internal", err: errors.New("boom"), want: false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := isAuthRegisterBadRequest(tc.err); got != tc.want {
				t.Fatalf("expected %t, got %t", tc.want, got)
			}
		})
	}
}
