package playbackgateway

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/konghang/ember/backend/internal/services/embytoken"
)

// TestGatewayImageCacheStillChecksCurrentIdentity protects local revocation
// and expiration even when the requested image is already fresh in memory.
func TestGatewayImageCacheStillChecksCurrentIdentity(t *testing.T) {
	for _, test := range []struct {
		name   string
		err    error
		status int
	}{
		{name: "revoked token", err: embytoken.ErrTokenRevoked, status: http.StatusUnauthorized},
		{name: "expired user", err: embytoken.ErrUserExpired, status: http.StatusForbidden},
	} {
		t.Run(test.name, func(t *testing.T) {
			var calls atomic.Int32
			var logs bytes.Buffer
			tokens := &fakeTokenService{principal: fixturePrincipal()}
			gateway := newVideoTestGateway(t, "http://emby.invalid", tokens, nil,
				roundTripFunc(func(request *http.Request) (*http.Response, error) {
					calls.Add(1)
					return freshImageProxyResponse(request, "private-image"), nil
				}), &logs)
			for i := 0; i < 2; i++ {
				request := httptest.NewRequest(http.MethodGet, "/emby/Items/fixture/Images/Primary?tag=fixture-tag", nil)
				request.Header.Set(accessTokenHeader, fixtureAccessToken)
				response := httptest.NewRecorder()
				gateway.ServeHTTP(response, request)
				if response.Code != http.StatusOK || response.Body.String() != "private-image" {
					t.Fatalf("warm image status=%d body=%q", response.Code, response.Body.String())
				}
			}
			if calls.Load() != 1 {
				t.Fatalf("fresh image contacted upstream %d times, want 1", calls.Load())
			}
			tokens.mu.Lock()
			tokens.resolveErr = test.err
			tokens.mu.Unlock()
			request := httptest.NewRequest(http.MethodGet, "/emby/Items/fixture/Images/Primary?tag=fixture-tag", nil)
			request.Header.Set(accessTokenHeader, fixtureAccessToken)
			response := httptest.NewRecorder()
			gateway.ServeHTTP(response, request)
			if response.Code != test.status || response.Body.Len() != 0 || calls.Load() != 1 {
				t.Fatalf("changed identity status=%d bytes=%d upstream=%d", response.Code, response.Body.Len(), calls.Load())
			}
			_, resolved := tokens.snapshot()
			if len(resolved) != 3 {
				t.Fatalf("identity checks=%d, want one per request", len(resolved))
			}
			assertSecretsAbsent(t, logs.String(), fixtureAccessToken, "fixture-tag", "private-image")
		})
	}
}

// TestGatewayImageCacheSeparatesAnonymousAndProtectedResponses prevents a
// cached image from changing the existing exact anonymous exception boundary.
func TestGatewayImageCacheSeparatesAnonymousAndProtectedResponses(t *testing.T) {
	var calls atomic.Int32
	var logs bytes.Buffer
	policy := &fakeWebSurfacePolicy{}
	gateway := newVideoTestGateway(t, "http://emby.invalid", &fakeTokenService{principal: fixturePrincipal()}, nil,
		roundTripFunc(func(request *http.Request) (*http.Response, error) {
			calls.Add(1)
			body := "anonymous-image"
			if request.Header.Get(accessTokenHeader) != "" {
				body = "protected-image"
			}
			return freshImageProxyResponse(request, body), nil
		}), &logs)
	gateway.webSurfacePolicy = policy
	for _, protected := range []bool{false, true, false, true} {
		request := httptest.NewRequest(http.MethodGet, "/emby/Items/fixture/Images/Primary", nil)
		expected := "anonymous-image"
		if protected {
			request.Header.Set(accessTokenHeader, fixtureAccessToken)
			expected = "protected-image"
		}
		response := httptest.NewRecorder()
		gateway.ServeHTTP(response, request)
		if response.Code != http.StatusOK || response.Body.String() != expected {
			t.Fatalf("image protected=%t status=%d body=%q", protected, response.Code, response.Body.String())
		}
	}
	if calls.Load() != 2 || policy.callCount() != 0 {
		t.Fatalf("upstream=%d web-policy=%d, want 2 and 0", calls.Load(), policy.callCount())
	}
	response := httptest.NewRecorder()
	gateway.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/Items/fixture/Images/Primary", nil))
	if response.Code != http.StatusUnauthorized || calls.Load() != 2 {
		t.Fatalf("root anonymous image widened scope: status=%d calls=%d", response.Code, calls.Load())
	}
}

// freshImageProxyResponse provides a cacheable fake upstream response without
// starting a listener or contacting an external image server.
func freshImageProxyResponse(request *http.Request, body string) *http.Response {
	return &http.Response{
		StatusCode: http.StatusOK,
		Header: http.Header{
			"Content-Type":  {"image/png"},
			"Cache-Control": {"public, max-age=60"},
			"Etag":          {`"fixture-image-v1"`},
		},
		Body: io.NopCloser(strings.NewReader(body)), ContentLength: int64(len(body)), Request: request,
	}
}
