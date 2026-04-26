package handlers

import (
	"errors"
	"testing"

	emailpkg "github.com/konghang/ember/backend/internal/services/email"
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
