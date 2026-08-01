package p115

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
)

type httpDoerFunc func(*http.Request) (*http.Response, error)

func (f httpDoerFunc) Do(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestCookieCredentialValidatorValidatesLoginStatusAndNormalizesUserID(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		if r.URL.Query().Get("ct") != "guide" || r.URL.Query().Get("ac") != "status" {
			t.Fatalf("unexpected query: %s", r.URL.RawQuery)
		}
		if r.Header.Get("Cookie") != "UID=0012345_A1; CID=fake; SEID=fake" {
			t.Fatalf("unexpected Cookie header: %q", r.Header.Get("Cookie"))
		}
		if r.Header.Get("User-Agent") != "ember-test-agent" {
			t.Fatalf("unexpected User-Agent: %q", r.Header.Get("User-Agent"))
		}
		if r.Header.Get("Accept") != "*/*" {
			t.Fatalf("unexpected Accept header: %q", r.Header.Get("Accept"))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"state":true}`))
	}))
	defer server.Close()

	validator, err := newCookieCredentialValidator(server.Client(), server.URL)
	if err != nil {
		t.Fatalf("newCookieCredentialValidator() failed: %v", err)
	}
	identity, err := validator.ValidateCredential(context.Background(), Credential{
		AccountID: "account_1",
		Cookie:    "UID=0012345_A1; CID=fake; SEID=fake",
		UserAgent: "ember-test-agent",
	})
	if err != nil {
		t.Fatalf("ValidateCredential() failed: %v", err)
	}
	if identity.ProviderUserID != "12345" || identity.DisplayName != "" {
		t.Fatalf("unexpected identity: %+v", identity)
	}
}

func TestCookieCredentialValidatorRejectsInactiveAndMalformedCookies(t *testing.T) {
	t.Run("inactive", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte(`{"state":false}`))
		}))
		defer server.Close()

		validator, err := newCookieCredentialValidator(server.Client(), server.URL)
		if err != nil {
			t.Fatalf("newCookieCredentialValidator() failed: %v", err)
		}
		_, err = validator.ValidateCredential(context.Background(), Credential{
			Cookie:    "UID=12345_A1; CID=fake",
			UserAgent: "agent",
		})
		if !errors.Is(err, ErrCredentialRejected) {
			t.Fatalf("ValidateCredential() error = %v, want ErrCredentialRejected", err)
		}
	})

	for _, cookie := range []string{"CID=fake", "UID=not-a-number_A1", "UID=0_A1", "UID=1_A1; UID=2_A1"} {
		t.Run(cookie, func(t *testing.T) {
			var calls atomic.Int32
			validator, err := newCookieCredentialValidator(httpDoerFunc(func(*http.Request) (*http.Response, error) {
				calls.Add(1)
				return nil, errors.New("must not be called")
			}), "https://example.invalid")
			if err != nil {
				t.Fatalf("newCookieCredentialValidator() failed: %v", err)
			}
			_, err = validator.ValidateCredential(context.Background(), Credential{Cookie: cookie, UserAgent: "agent"})
			if !errors.Is(err, ErrCredentialRejected) {
				t.Fatalf("ValidateCredential() error = %v, want ErrCredentialRejected", err)
			}
			if calls.Load() != 0 {
				t.Fatalf("invalid Cookie reached HTTP client: calls=%d", calls.Load())
			}
		})
	}
}

func TestCookieCredentialValidatorMapsSafeProviderErrors(t *testing.T) {
	t.Run("transport", func(t *testing.T) {
		validator, err := newCookieCredentialValidator(httpDoerFunc(func(*http.Request) (*http.Response, error) {
			return nil, &url.Error{Op: "Get", URL: "https://my.115.com/?secret=cookie-secret", Err: errors.New("dial failed")}
		}), "https://example.invalid")
		if err != nil {
			t.Fatalf("newCookieCredentialValidator() failed: %v", err)
		}
		_, err = validator.ValidateCredential(context.Background(), Credential{Cookie: "UID=123_A1", UserAgent: "agent"})
		if !errors.Is(err, ErrProviderUnavailable) {
			t.Fatalf("ValidateCredential() error = %v, want ErrProviderUnavailable", err)
		}
		if strings.Contains(err.Error(), "cookie-secret") || strings.Contains(err.Error(), "my.115.com") {
			t.Fatalf("transport error exposed request details: %v", err)
		}
	})

	tests := []struct {
		name       string
		statusCode int
		body       string
		wantErr    error
	}{
		{name: "http status", statusCode: http.StatusServiceUnavailable, body: `cookie-secret`, wantErr: ErrProviderUnavailable},
		{name: "invalid json", statusCode: http.StatusOK, body: `{"state":`, wantErr: ErrProviderProtocol},
		{name: "missing state", statusCode: http.StatusOK, body: `{}`, wantErr: ErrProviderProtocol},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tt.statusCode)
				_, _ = w.Write([]byte(tt.body))
			}))
			defer server.Close()

			validator, err := newCookieCredentialValidator(server.Client(), server.URL)
			if err != nil {
				t.Fatalf("newCookieCredentialValidator() failed: %v", err)
			}
			_, err = validator.ValidateCredential(context.Background(), Credential{Cookie: "UID=123_A1", UserAgent: "agent"})
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("ValidateCredential() error = %v, want %v", err, tt.wantErr)
			}
			if strings.Contains(fmt.Sprint(err), "cookie-secret") {
				t.Fatalf("provider error exposed response body: %v", err)
			}
		})
	}
}
