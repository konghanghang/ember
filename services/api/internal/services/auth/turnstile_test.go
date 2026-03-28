package auth

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

type stubTurnstileHTTPClient struct {
	do func(req *http.Request) (*http.Response, error)
}

func (s *stubTurnstileHTTPClient) Do(req *http.Request) (*http.Response, error) {
	return s.do(req)
}

func readVerifyErrorReason(t *testing.T, err error) string {
	t.Helper()
	var verifyErr *TurnstileVerifyError
	if !errors.As(err, &verifyErr) {
		t.Fatalf("expected TurnstileVerifyError, got %T (%v)", err, err)
	}
	return verifyErr.Reason
}

func TestCloudflareTurnstileVerifierSuccess(t *testing.T) {
	var capturedSecret string
	var capturedToken string
	var capturedIP string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("expected POST, got %s", r.Method)
		}
		if err := r.ParseForm(); err != nil {
			t.Fatalf("parse form failed: %v", err)
		}

		capturedSecret = r.FormValue("secret")
		capturedToken = r.FormValue("response")
		capturedIP = r.FormValue("remoteip")

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"success":true,"action":"login","hostname":"ember.example.com"}`))
	}))
	defer server.Close()

	verifier := NewCloudflareTurnstileVerifierWithClient("test_secret", server.Client(), server.URL)
	err := verifier.VerifyLogin(context.Background(), TurnstileVerifyPayload{
		Token:            "turnstile_token",
		RemoteIP:         "127.0.0.1",
		ExpectedAction:   "login",
		ExpectedHostname: "ember.example.com",
	})
	if err != nil {
		t.Fatalf("expected verify success, got %v", err)
	}

	if capturedSecret != "test_secret" || capturedToken != "turnstile_token" || capturedIP != "127.0.0.1" {
		t.Fatalf("unexpected request payload: secret=%q token=%q remoteip=%q", capturedSecret, capturedToken, capturedIP)
	}
}

func TestCloudflareTurnstileVerifierFailurePaths(t *testing.T) {
	tests := []struct {
		name       string
		body       string
		payload    TurnstileVerifyPayload
		wantReason string
	}{
		{
			name:       "success false",
			body:       `{"success":false,"error-codes":["invalid-input-response"]}`,
			payload:    TurnstileVerifyPayload{Token: "token", ExpectedAction: "login"},
			wantReason: "siteverify_rejected_invalid-input-response",
		},
		{
			name:       "action mismatch",
			body:       `{"success":true,"action":"register","hostname":"ember.example.com"}`,
			payload:    TurnstileVerifyPayload{Token: "token", ExpectedAction: "login"},
			wantReason: "action_mismatch",
		},
		{
			name:       "hostname mismatch",
			body:       `{"success":true,"action":"login","hostname":"other.example.com"}`,
			payload:    TurnstileVerifyPayload{Token: "token", ExpectedAction: "login", ExpectedHostname: "ember.example.com"},
			wantReason: "hostname_mismatch",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(tc.body))
			}))
			defer server.Close()

			verifier := NewCloudflareTurnstileVerifierWithClient("test_secret", server.Client(), server.URL)
			err := verifier.VerifyLogin(context.Background(), tc.payload)
			if err == nil {
				t.Fatalf("expected verify failure")
			}
			if reason := readVerifyErrorReason(t, err); reason != tc.wantReason {
				t.Fatalf("expected reason %q, got %q", tc.wantReason, reason)
			}
		})
	}
}

func TestCloudflareTurnstileVerifierNetworkAndConfigErrors(t *testing.T) {
	t.Run("missing secret key", func(t *testing.T) {
		verifier := NewCloudflareTurnstileVerifierWithClient("", &stubTurnstileHTTPClient{
			do: func(req *http.Request) (*http.Response, error) {
				t.Fatal("http client should not be called when secret key is missing")
				return nil, nil
			},
		}, "https://example.com")
		err := verifier.VerifyLogin(context.Background(), TurnstileVerifyPayload{Token: "token"})
		if err == nil {
			t.Fatalf("expected verify failure")
		}
		if reason := readVerifyErrorReason(t, err); reason != "missing_secret_key" {
			t.Fatalf("expected reason missing_secret_key, got %q", reason)
		}
	})

	t.Run("request failed", func(t *testing.T) {
		verifier := NewCloudflareTurnstileVerifierWithClient("test_secret", &stubTurnstileHTTPClient{
			do: func(req *http.Request) (*http.Response, error) {
				return nil, &url.Error{Op: "Post", URL: req.URL.String(), Err: errors.New("dial failed")}
			},
		}, "https://example.com")
		err := verifier.VerifyLogin(context.Background(), TurnstileVerifyPayload{Token: "token"})
		if err == nil {
			t.Fatalf("expected verify failure")
		}
		if reason := readVerifyErrorReason(t, err); reason != "request_failed" {
			t.Fatalf("expected reason request_failed, got %q", reason)
		}
	})

	t.Run("missing token", func(t *testing.T) {
		verifier := NewCloudflareTurnstileVerifierWithClient("test_secret", &stubTurnstileHTTPClient{
			do: func(req *http.Request) (*http.Response, error) {
				t.Fatal("http client should not be called when token is missing")
				return nil, nil
			},
		}, "https://example.com")
		err := verifier.VerifyLogin(context.Background(), TurnstileVerifyPayload{})
		if err == nil {
			t.Fatalf("expected verify failure")
		}
		if reason := readVerifyErrorReason(t, err); reason != "missing_token" {
			t.Fatalf("expected reason missing_token, got %q", reason)
		}
	})

	t.Run("unexpected status code", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusBadGateway)
			_, _ = w.Write([]byte(strings.Repeat("x", 4096)))
		}))
		defer server.Close()

		verifier := NewCloudflareTurnstileVerifierWithClient("test_secret", server.Client(), server.URL)
		err := verifier.VerifyLogin(context.Background(), TurnstileVerifyPayload{Token: "token"})
		if err == nil {
			t.Fatalf("expected verify failure")
		}
		if reason := readVerifyErrorReason(t, err); reason != "unexpected_status_502" {
			t.Fatalf("expected reason unexpected_status_502, got %q", reason)
		}
	})
}
