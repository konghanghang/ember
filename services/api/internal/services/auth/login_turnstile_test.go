package auth

import (
	"context"
	"errors"
	"testing"
)

type testContextKey string

type stubAuthConfigReader struct {
	values map[string]string
}

func (s *stubAuthConfigReader) GetString(key string) string {
	if s == nil || s.values == nil {
		return ""
	}
	return s.values[key]
}

type stubAuthTurnstileVerifier struct {
	called      bool
	lastPayload TurnstileVerifyPayload
	lastCtx     context.Context
	verifyErr   error
}

func (s *stubAuthTurnstileVerifier) VerifyLogin(ctx context.Context, payload TurnstileVerifyPayload) error {
	s.called = true
	s.lastCtx = ctx
	s.lastPayload = payload
	return s.verifyErr
}

func TestVerifyTurnstileForLogin(t *testing.T) {
	t.Run("disabled skips verification", func(t *testing.T) {
		verifier := &stubAuthTurnstileVerifier{}
		service := &AuthService{
			configReader: &stubAuthConfigReader{
				values: map[string]string{"turnstile_login_enabled": "false"},
			},
			turnstileVerifier: verifier,
		}

		err := service.verifyTurnstileForLogin(&LoginRequest{})
		if err != nil {
			t.Fatalf("expected no error when disabled, got %v", err)
		}
		if verifier.called {
			t.Fatalf("expected verifier not to be called when disabled")
		}
	})

	t.Run("enabled requires site key", func(t *testing.T) {
		verifier := &stubAuthTurnstileVerifier{}
		service := &AuthService{
			configReader: &stubAuthConfigReader{
				values: map[string]string{"turnstile_login_enabled": "true"},
			},
			turnstileVerifier: verifier,
		}

		err := service.verifyTurnstileForLogin(&LoginRequest{TurnstileToken: "token"})
		if !errors.Is(err, ErrTurnstileValidationFailed) {
			t.Fatalf("expected ErrTurnstileValidationFailed, got %v", err)
		}
		if verifier.called {
			t.Fatalf("expected verifier not to be called when site key is missing")
		}
	})

	t.Run("enabled verifier failure returns unified error", func(t *testing.T) {
		verifier := &stubAuthTurnstileVerifier{
			verifyErr: &TurnstileVerifyError{Reason: "siteverify_rejected"},
		}
		service := &AuthService{
			configReader: &stubAuthConfigReader{
				values: map[string]string{
					"turnstile_login_enabled":     "true",
					"turnstile_site_key":          "site_key",
					"turnstile_expected_hostname": "ember.example.com",
				},
			},
			turnstileVerifier: verifier,
		}

		err := service.verifyTurnstileForLogin(&LoginRequest{
			TurnstileToken: "token",
			ClientIP:       "10.0.0.1",
		})
		if !errors.Is(err, ErrTurnstileValidationFailed) {
			t.Fatalf("expected ErrTurnstileValidationFailed, got %v", err)
		}
		if !verifier.called {
			t.Fatalf("expected verifier to be called")
		}
	})

	t.Run("enabled success passes expected payload", func(t *testing.T) {
		verifier := &stubAuthTurnstileVerifier{}
		service := &AuthService{
			configReader: &stubAuthConfigReader{
				values: map[string]string{
					"turnstile_login_enabled":     "true",
					"turnstile_site_key":          "site_key",
					"turnstile_expected_hostname": "ember.example.com",
				},
			},
			turnstileVerifier: verifier,
		}
		traceKey := testContextKey("trace-id")
		ctx := context.WithValue(context.Background(), traceKey, "trace-123")

		err := service.verifyTurnstileForLogin(&LoginRequest{
			TurnstileToken: "turnstile_token",
			ClientIP:       "127.0.0.1",
			RequestContext: ctx,
		})
		if err != nil {
			t.Fatalf("expected verify success, got %v", err)
		}
		if !verifier.called {
			t.Fatalf("expected verifier to be called")
		}
		if verifier.lastPayload.Token != "turnstile_token" || verifier.lastPayload.RemoteIP != "127.0.0.1" {
			t.Fatalf("unexpected payload token/remoteip: %+v", verifier.lastPayload)
		}
		if verifier.lastPayload.ExpectedAction != "login" || verifier.lastPayload.ExpectedHostname != "ember.example.com" {
			t.Fatalf("unexpected payload action/hostname: %+v", verifier.lastPayload)
		}
		if got := verifier.lastCtx.Value(traceKey); got != "trace-123" {
			t.Fatalf("expected request context to be forwarded, got %v", got)
		}
	})
}
