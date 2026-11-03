package playbackgateway

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

// imageCacheTestRequest supplies the already gated route context consumed by
// the transport, without contacting an Emby server or bypassing Gateway auth.
func imageCacheTestRequest(method, target string, authenticated bool) *http.Request {
	r := httptest.NewRequest(method, target, nil)
	if authenticated {
		p := fixturePrincipal()
		r.Header.Set("X-Emby-Token", "fixture-image-token")
		r = r.WithContext(context.WithValue(r.Context(), requestRouteContextKey{}, requestRouteContext{kind: routeProtected, principal: &p}))
	} else {
		r = r.WithContext(context.WithValue(r.Context(), requestRouteContextKey{}, requestRouteContext{kind: routeItemImage}))
	}
	return r
}

// imageCacheTestResponse returns a small cacheable image fixture; its body is
// opaque bytes because the proxy must preserve rather than decode images.
func imageCacheTestResponse(body string) *http.Response {
	return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"image/png"}, "Cache-Control": {"public, max-age=60"}, "Etag": {"\"image-v1\""}}, Body: io.NopCloser(strings.NewReader(body)), ContentLength: int64(len(body))}
}

// imageCacheRead closes each response so shared upstream work cannot leak
// bodies or remain alive after a test has consumed the bytes.
func imageCacheRead(t *testing.T, response *http.Response, err error) string {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	body, readErr := io.ReadAll(response.Body)
	if readErr != nil {
		t.Fatal(readErr)
	}
	return string(body)
}

// TestImageCacheFreshGETAndHEAD verifies that repeated GET and a later HEAD
// reuse one successful image without replacing HEAD with a body.
func TestImageCacheFreshGETAndHEAD(t *testing.T) {
	var calls atomic.Int32
	transport := newImageCachingTransport(roundTripFunc(func(*http.Request) (*http.Response, error) {
		calls.Add(1)
		return imageCacheTestResponse("image-bytes"), nil
	}), nil)
	for _, method := range []string{http.MethodGet, http.MethodGet, http.MethodHead} {
		response, err := transport.RoundTrip(imageCacheTestRequest(method, "http://emby.invalid/emby/Items/123/Images/Primary?Width=300&tag=one", true))
		got := imageCacheRead(t, response, err)
		want := "image-bytes"
		if method == http.MethodHead {
			want = ""
		}
		if got != want {
			t.Fatalf("%s body=%q, want %q", method, got, want)
		}
	}
	if calls.Load() != 1 {
		t.Fatalf("upstream calls=%d, want 1", calls.Load())
	}
}

// TestImageCacheIdentityAndVariantIsolation keeps authenticated, anonymous,
// credential and image-size/tag variants in distinct cache entries.
func TestImageCacheIdentityAndVariantIsolation(t *testing.T) {
	var calls atomic.Int32
	transport := newImageCachingTransport(roundTripFunc(func(*http.Request) (*http.Response, error) {
		calls.Add(1)
		return imageCacheTestResponse("image"), nil
	}), nil)
	base := "http://emby.invalid/emby/Items/123/Images/Primary?Width=300&tag=one"
	requests := []*http.Request{
		imageCacheTestRequest(http.MethodGet, base, true),
		imageCacheTestRequest(http.MethodGet, base, false),
		imageCacheTestRequest(http.MethodGet, base+"&Quality=80", true),
		imageCacheTestRequest(http.MethodGet, strings.Replace(base, "tag=one", "tag=two", 1), true),
		imageCacheTestRequest(http.MethodGet, base, true),
	}
	requests[4].Header.Set("X-Emby-Token", "different-fixture-token")
	for _, request := range requests {
		response, err := transport.RoundTrip(request)
		imageCacheRead(t, response, err)
	}
	if calls.Load() != int32(len(requests)) {
		t.Fatalf("upstream calls=%d, want %d", calls.Load(), len(requests))
	}
}

// TestImageCacheBypassesUnsafeRequests confirms unsupported methods, Range,
// client preconditions and ungated paths remain transparent upstream requests.
func TestImageCacheBypassesUnsafeRequests(t *testing.T) {
	for _, kind := range []string{"range", "condition", "no-store", "no-context", "other-path", "get-body", "upgrade"} {
		t.Run(kind, func(t *testing.T) {
			var calls atomic.Int32
			transport := newImageCachingTransport(roundTripFunc(func(*http.Request) (*http.Response, error) {
				calls.Add(1)
				return imageCacheTestResponse("image"), nil
			}), nil)
			for i := 0; i < 2; i++ {
				request := imageCacheTestRequest(http.MethodGet, "http://emby.invalid/emby/Items/123/Images/Primary", true)
				switch kind {
				case "range":
					request.Header.Set("Range", "bytes=0-2")
				case "condition":
					request.Header.Set("If-None-Match", "\"image-v1\"")
				case "no-store":
					request.Header.Set("Cache-Control", "no-store")
				case "no-context":
					request = request.WithContext(context.Background())
				case "other-path":
					request.URL.Path = "/emby/Videos/123/stream"
				case "get-body":
					request.Body, request.ContentLength = io.NopCloser(strings.NewReader("fixture-body")), 12
				case "upgrade":
					request.Header.Set("Connection", "Upgrade")
					request.Header.Set("Upgrade", "websocket")
				}
				response, err := transport.RoundTrip(request)
				imageCacheRead(t, response, err)
			}
			if calls.Load() != 2 {
				t.Fatalf("upstream calls=%d, want 2", calls.Load())
			}
		})
	}
}
