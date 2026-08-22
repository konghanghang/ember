package emby

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

const fixtureServerIdentityAPIKey = "fixture-server-identity-api-key"

func TestServerIdentityVerifierReadsFixedSystemInfoContract(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet || request.URL.Path != "/emby/System/Info" || request.URL.RawQuery != "" {
			t.Errorf("request = %s %s", request.Method, request.URL.RequestURI())
		}
		if request.Header.Get("X-Emby-Token") != fixtureServerIdentityAPIKey {
			t.Error("API key header changed")
		}
		if request.Header.Get("Accept") != "application/json" {
			t.Errorf("Accept = %q", request.Header.Get("Accept"))
		}
		writer.Header().Set("Content-Type", "application/json; charset=utf-8")
		_, _ = io.WriteString(writer, `{"Id":"server-1","Version":"4.9.3.0","ServerName":"Fixture","Unknown":true}`)
	}))
	defer upstream.Close()

	verifier, err := NewServerIdentityVerifier(upstream.URL, fixtureServerIdentityAPIKey, nil)
	if err != nil {
		t.Fatalf("NewServerIdentityVerifier() error = %v", err)
	}
	identity, err := verifier.Verify(context.Background())
	if err != nil {
		t.Fatalf("Verify() error = %v", err)
	}
	if identity.ID != "server-1" || identity.Version != "4.9.3.0" || identity.ServerName != "Fixture" {
		t.Fatalf("Verify() = %+v", identity)
	}
}

func TestServerIdentityVerifierRejectsUnsafeConfiguration(t *testing.T) {
	tests := []struct {
		name   string
		url    string
		apiKey string
	}{
		{name: "missing URL", apiKey: fixtureServerIdentityAPIKey},
		{name: "unsupported scheme", url: "ftp://emby.internal", apiKey: fixtureServerIdentityAPIKey},
		{name: "URL credentials", url: "http://user:password@emby.internal", apiKey: fixtureServerIdentityAPIKey},
		{name: "URL query", url: "http://emby.internal?api_key=secret", apiKey: fixtureServerIdentityAPIKey},
		{name: "missing API key", url: "http://emby.internal"},
		{name: "padded API key", url: "http://emby.internal", apiKey: " padded "},
		{name: "oversized API key", url: "http://emby.internal", apiKey: strings.Repeat("x", 16*1024+1)},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := NewServerIdentityVerifier(test.url, test.apiKey, nil); !errors.Is(err, ErrServerIdentityConfig) {
				t.Fatalf("NewServerIdentityVerifier() error = %v, want %v", err, ErrServerIdentityConfig)
			}
		})
	}
	verifier, err := NewServerIdentityVerifier("http://emby.internal", fixtureServerIdentityAPIKey, nil)
	if err != nil {
		t.Fatalf("NewServerIdentityVerifier(valid) error = %v", err)
	}
	if _, err := verifier.Verify(nil); !errors.Is(err, ErrServerIdentityConfig) {
		t.Fatalf("Verify(nil) error = %v, want %v", err, ErrServerIdentityConfig)
	}
}

func TestServerIdentityVerifierFailsClosedOnHTTPAndProtocolErrors(t *testing.T) {
	tests := []struct {
		name        string
		status      int
		contentType string
		body        string
		wantErr     error
	}{
		{name: "unauthorized", status: http.StatusUnauthorized, contentType: "application/json", body: `{"error":"` + fixtureServerIdentityAPIKey + `"}`, wantErr: ErrServerIdentityHTTP},
		{name: "server failure", status: http.StatusInternalServerError, contentType: "application/json", body: `{"error":"failed"}`, wantErr: ErrServerIdentityHTTP},
		{name: "non json", status: http.StatusOK, contentType: "text/plain", body: `{"Id":"server-1","Version":"4.9.3.0"}`, wantErr: ErrServerIdentityProtocol},
		{name: "invalid json", status: http.StatusOK, contentType: "application/json", body: `{`, wantErr: ErrServerIdentityProtocol},
		{name: "missing id", status: http.StatusOK, contentType: "application/json", body: `{"Version":"4.9.3.0"}`, wantErr: ErrServerIdentityProtocol},
		{name: "missing version", status: http.StatusOK, contentType: "application/json", body: `{"Id":"server-1"}`, wantErr: ErrServerIdentityProtocol},
		{name: "padded id", status: http.StatusOK, contentType: "application/json", body: `{"Id":" server-1 ","Version":"4.9.3.0"}`, wantErr: ErrServerIdentityProtocol},
		{name: "oversized body", status: http.StatusOK, contentType: "application/json", body: `{"Id":"server-1","Version":"4.9.3.0","Padding":"` + strings.Repeat("x", 256*1024) + `"}`, wantErr: ErrServerIdentityProtocol},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
				writer.Header().Set("Content-Type", test.contentType)
				writer.WriteHeader(test.status)
				_, _ = io.WriteString(writer, test.body)
			}))
			defer upstream.Close()

			verifier, err := NewServerIdentityVerifier(upstream.URL, fixtureServerIdentityAPIKey, nil)
			if err != nil {
				t.Fatalf("NewServerIdentityVerifier() error = %v", err)
			}
			_, err = verifier.Verify(context.Background())
			if !errors.Is(err, test.wantErr) {
				t.Fatalf("Verify() error = %v, want %v", err, test.wantErr)
			}
			assertIdentitySecretsAbsent(t, err.Error(), fixtureServerIdentityAPIKey, test.body, upstream.URL)
		})
	}
}

func TestServerIdentityVerifierDoesNotFollowRedirects(t *testing.T) {
	var redirectedCalls atomic.Int32
	redirected := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		redirectedCalls.Add(1)
		_, _ = io.WriteString(writer, `{"Id":"wrong","Version":"4.9.3.0"}`)
	}))
	defer redirected.Close()
	upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		http.Redirect(writer, &http.Request{}, redirected.URL, http.StatusFound)
	}))
	defer upstream.Close()

	verifier, err := NewServerIdentityVerifier(upstream.URL, fixtureServerIdentityAPIKey, nil)
	if err != nil {
		t.Fatalf("NewServerIdentityVerifier() error = %v", err)
	}
	if _, err := verifier.Verify(context.Background()); !errors.Is(err, ErrServerIdentityHTTP) {
		t.Fatalf("Verify() error = %v, want %v", err, ErrServerIdentityHTTP)
	}
	if redirectedCalls.Load() != 0 {
		t.Fatalf("redirected calls = %d, want 0", redirectedCalls.Load())
	}
}

func TestServerIdentityVerifierHonorsCancellation(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		close(started)
		select {
		case <-request.Context().Done():
		case <-release:
		}
		writer.WriteHeader(http.StatusGatewayTimeout)
	}))
	defer func() {
		close(release)
		upstream.Close()
	}()

	verifier, err := NewServerIdentityVerifier(upstream.URL, fixtureServerIdentityAPIKey, nil)
	if err != nil {
		t.Fatalf("NewServerIdentityVerifier() error = %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, verifyErr := verifier.Verify(ctx)
		done <- verifyErr
	}()
	<-started
	cancel()
	if err := <-done; !errors.Is(err, ErrServerIdentityUnavailable) {
		t.Fatalf("Verify() error = %v, want %v", err, ErrServerIdentityUnavailable)
	}
}

func assertIdentitySecretsAbsent(t *testing.T, value string, secrets ...string) {
	t.Helper()
	for _, secret := range secrets {
		if secret != "" && strings.Contains(value, secret) {
			t.Fatalf("secret %q leaked in %q", secret, value)
		}
	}
}
