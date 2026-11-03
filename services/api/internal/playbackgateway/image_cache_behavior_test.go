package playbackgateway

import (
	"errors"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// TestImageCacheConditionalRevalidation proves that stale entries are checked
// with Emby while an internal 304 becomes the original representation's 200.
func TestImageCacheConditionalRevalidation(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	var calls int
	cache := newImageCachingTransport(roundTripFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		if calls == 1 {
			response := imageCacheTestResponse("original")
			response.Header.Set("Cache-Control", "max-age=1")
			response.Header.Set("Date", now.Format(http.TimeFormat))
			return response, nil
		}
		if r.Header.Get("If-None-Match") != "\"image-v1\"" {
			t.Errorf("revalidation missing ETag: %v", r.Header)
		}
		return &http.Response{StatusCode: http.StatusNotModified, Header: http.Header{"Cache-Control": {"max-age=20"}, "Date": {now.Format(http.TimeFormat)}}, Body: http.NoBody}, nil
	}), nil).(*imageCachingTransport)
	cache.now = func() time.Time { return now }
	request := func() *http.Request {
		return imageCacheTestRequest(http.MethodGet, "http://emby.invalid/emby/Items/123/Images/Primary", true)
	}
	response, err := cache.RoundTrip(request())
	imageCacheRead(t, response, err)
	now = now.Add(2 * time.Second)
	for i := 0; i < 2; i++ {
		response, err = cache.RoundTrip(request())
		if response.StatusCode != http.StatusOK || imageCacheRead(t, response, err) != "original" {
			t.Fatal("conditional validation did not return original 200 image")
		}
	}
	if calls != 2 {
		t.Fatalf("calls=%d, want first fetch plus one validation", calls)
	}
}

// TestImageCacheResponsePolicyBypass covers independent reasons that an image
// response must remain unshared and unretained, preserving its original bytes.
func TestImageCacheResponsePolicyBypass(t *testing.T) {
	for _, kind := range []string{"cookie", "no-store", "private", "vary-star", "vary-cache-control", "unknown-policy", "malformed-policy", "non-image", "partial", "error", "large-known", "large-stream"} {
		t.Run(kind, func(t *testing.T) {
			var calls atomic.Int32
			cache := newImageCachingTransport(roundTripFunc(func(*http.Request) (*http.Response, error) {
				calls.Add(1)
				response := imageCacheTestResponse("abcdefgh")
				switch kind {
				case "cookie":
					response.Header.Set("Set-Cookie", "fixture=value")
				case "no-store", "private":
					response.Header.Set("Cache-Control", kind)
				case "vary-star":
					response.Header.Set("Vary", "*")
				case "vary-cache-control":
					response.Header.Set("Vary", "Cache-Control")
				case "unknown-policy":
					response.Header.Set("Cache-Control", "max-age=60, unknown-directive")
				case "malformed-policy":
					response.Header.Set("Cache-Control", "max-age=\"60")
				case "non-image":
					response.Header.Set("Content-Type", "text/html")
				case "partial":
					response.StatusCode = http.StatusPartialContent
					response.Header.Set("Content-Range", "bytes 0-7/16")
				case "error":
					response.StatusCode = http.StatusNotFound
				case "large-stream":
					response.ContentLength = -1
				}
				return response, nil
			}), nil).(*imageCachingTransport)
			if strings.HasPrefix(kind, "large-") {
				cache.maxBody = 4
			}
			for i := 0; i < 2; i++ {
				response, err := cache.RoundTrip(imageCacheTestRequest(http.MethodGet, "http://emby.invalid/emby/Items/123/Images/Primary", true))
				if imageCacheRead(t, response, err) != "abcdefgh" {
					t.Fatal("bypass changed body")
				}
			}
			if calls.Load() != 2 || len(cache.entries) != 0 {
				t.Fatalf("uncacheable response retained: calls=%d entries=%d", calls.Load(), len(cache.entries))
			}
		})
	}
}

// TestImageCacheAgeAndForcedValidation prevents Age, Date and no-cache from
// incorrectly extending the origin's explicitly permitted freshness.
func TestImageCacheAgeAndForcedValidation(t *testing.T) {
	for _, kind := range []string{"aged", "date", "response-no-cache", "request-no-cache", "validator-only"} {
		t.Run(kind, func(t *testing.T) {
			now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
			calls := 0
			cache := newImageCachingTransport(roundTripFunc(func(r *http.Request) (*http.Response, error) {
				calls++
				if calls > 1 {
					if kind == "request-no-cache" {
						if r.Header.Get("Cache-Control") != "no-cache" || r.Header.Get("If-None-Match") != "" {
							t.Error("request no-cache did not remain a transparent upstream refresh")
						}
					} else if r.Header.Get("If-None-Match") != "\"image-v1\"" {
						t.Error("expected conditional validation")
					}
				}
				response := imageCacheTestResponse("image")
				switch kind {
				case "aged":
					response.Header.Set("Age", "60")
				case "date":
					response.Header.Set("Date", now.Add(-time.Minute).Format(http.TimeFormat))
				case "response-no-cache":
					response.Header.Set("Cache-Control", "no-cache")
				case "validator-only":
					response.Header.Del("Cache-Control")
				}
				return response, nil
			}), nil).(*imageCachingTransport)
			cache.now = func() time.Time { return now }
			for i := 0; i < 2; i++ {
				request := imageCacheTestRequest(http.MethodGet, "http://emby.invalid/emby/Items/123/Images/Primary", true)
				if i == 1 && kind == "request-no-cache" {
					request.Header.Set("Cache-Control", "no-cache")
				}
				response, err := cache.RoundTrip(request)
				imageCacheRead(t, response, err)
			}
			if calls != 2 {
				t.Fatalf("calls=%d, want validation", calls)
			}
		})
	}
}

// TestImageCacheVaryAndBounds verifies conservative header isolation and both
// byte/count LRU limits, then checks rollback discards a formerly fresh image.
func TestImageCacheVaryAndBounds(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	calls := 0
	cache := newImageCachingTransport(roundTripFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		response := imageCacheTestResponse("abc")
		response.Header.Set("Vary", "Accept-Language")
		return response, nil
	}), nil).(*imageCachingTransport)
	cache.now = func() time.Time { return now }
	cache.maxEntries, cache.maxBytes = 2, 6
	for _, language := range []string{"zh", "en", "ja", "zh"} {
		request := imageCacheTestRequest(http.MethodGet, "http://emby.invalid/emby/Items/123/Images/Primary", true)
		request.Header.Set("Accept-Language", language)
		response, err := cache.RoundTrip(request)
		imageCacheRead(t, response, err)
	}
	if calls != 4 || cache.lru.Len() != 2 || cache.bytes != 6 {
		t.Fatalf("bound/isolation failed: calls=%d entries=%d bytes=%d", calls, cache.lru.Len(), cache.bytes)
	}
	now = now.Add(-time.Second)
	request := imageCacheTestRequest(http.MethodGet, "http://emby.invalid/emby/Items/123/Images/Primary", true)
	request.Header.Set("Accept-Language", "zh")
	response, err := cache.RoundTrip(request)
	imageCacheRead(t, response, err)
	if calls != 5 {
		t.Fatal("backwards clock kept cached image")
	}
}

type imageCacheBrokenBody struct{ read bool }

// Read returns the original prefix and one fixture failure for replay checks.
func (body *imageCacheBrokenBody) Read(p []byte) (int, error) {
	if body.read {
		return 0, io.EOF
	}
	body.read = true
	return copy(p, "prefix"), errors.New("fixture image read failure")
}

// Close has no external resource; it fulfils the fake upstream body contract.
func (*imageCacheBrokenBody) Close() error { return nil }

// TestImageCacheReadFailurePreservesBytesAndError ensures cache inspection
// cannot swallow a body failure or silently return a truncated successful image.
func TestImageCacheReadFailurePreservesBytesAndError(t *testing.T) {
	calls := 0
	cache := newImageCachingTransport(roundTripFunc(func(*http.Request) (*http.Response, error) {
		calls++
		response := imageCacheTestResponse("")
		response.Body, response.ContentLength = &imageCacheBrokenBody{}, -1
		return response, nil
	}), nil).(*imageCachingTransport)
	for i := 0; i < 2; i++ {
		response, err := cache.RoundTrip(imageCacheTestRequest(http.MethodGet, "http://emby.invalid/emby/Items/123/Images/Primary", true))
		if err != nil {
			t.Fatal(err)
		}
		body, readErr := io.ReadAll(response.Body)
		response.Body.Close()
		if string(body) != "prefix" || readErr == nil || !strings.Contains(readErr.Error(), "fixture image read failure") {
			t.Fatalf("body=%q err=%v", body, readErr)
		}
	}
	if calls != 2 || len(cache.entries) != 0 {
		t.Fatal("failed body was cached")
	}
}

// TestImageCacheNoStaleOnError keeps an upstream failure visible even when a
// stale successful image is available, rather than changing failure semantics.
func TestImageCacheNoStaleOnError(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	calls := 0
	cache := newImageCachingTransport(roundTripFunc(func(*http.Request) (*http.Response, error) {
		calls++
		response := imageCacheTestResponse("image")
		response.Header.Set("Cache-Control", "max-age=1")
		if calls > 1 {
			response.StatusCode = http.StatusServiceUnavailable
		}
		return response, nil
	}), nil).(*imageCachingTransport)
	cache.now = func() time.Time { return now }
	response, err := cache.RoundTrip(imageCacheTestRequest(http.MethodGet, "http://emby.invalid/emby/Items/123/Images/Primary", true))
	imageCacheRead(t, response, err)
	now = now.Add(2 * time.Second)
	response, err = cache.RoundTrip(imageCacheTestRequest(http.MethodGet, "http://emby.invalid/emby/Items/123/Images/Primary", true))
	if response.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("stale response replaced upstream status: %d", response.StatusCode)
	}
	imageCacheRead(t, response, err)
	if len(cache.entries) != 0 {
		t.Fatal("upstream failure left a stale entry available")
	}
}

// TestImageCacheValidationCannotReplaceRepresentationHeaders prevents a 304
// response from describing cached bytes with a different encoding or length.
func TestImageCacheValidationCannotReplaceRepresentationHeaders(t *testing.T) {
	for _, kind := range []string{"length", "encoding"} {
		t.Run(kind, func(t *testing.T) {
			now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
			calls := 0
			cache := newImageCachingTransport(roundTripFunc(func(r *http.Request) (*http.Response, error) {
				calls++
				if calls == 2 {
					header := http.Header{"Cache-Control": {"max-age=60"}}
					if kind == "length" {
						header.Set("Content-Length", "0")
					} else {
						header.Set("Content-Encoding", "gzip")
					}
					return &http.Response{StatusCode: http.StatusNotModified, Header: header, Body: http.NoBody}, nil
				}
				response := imageCacheTestResponse("image-bytes")
				response.Header.Set("Cache-Control", "max-age=1")
				return response, nil
			}), nil).(*imageCachingTransport)
			cache.now = func() time.Time { return now }
			request := func() *http.Request {
				return imageCacheTestRequest(http.MethodGet, "http://emby.invalid/emby/Items/123/Images/Primary", true)
			}
			response, err := cache.RoundTrip(request())
			imageCacheRead(t, response, err)
			now = now.Add(2 * time.Second)
			response, err = cache.RoundTrip(request())
			if response.Header.Get("Content-Encoding") != "" || response.Header.Get("Content-Length") == "0" {
				t.Fatalf("representation headers corrupted by 304: %v", response.Header)
			}
			imageCacheRead(t, response, err)
			if kind == "encoding" && calls != 3 {
				t.Fatalf("encoding change did not fetch full image: calls=%d", calls)
			}
		})
	}
}

// TestImageCacheResponseDelayConsumesFreshness includes upstream wait/body
// time in corrected Age even when the response carries no Date header.
func TestImageCacheResponseDelayConsumesFreshness(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	calls := 0
	cache := newImageCachingTransport(roundTripFunc(func(*http.Request) (*http.Response, error) {
		calls++
		now = now.Add(30 * time.Second)
		response := imageCacheTestResponse("image")
		response.Header.Set("Age", "30")
		return response, nil
	}), nil).(*imageCachingTransport)
	cache.now = func() time.Time { return now }
	for i := 0; i < 2; i++ {
		response, err := cache.RoundTrip(imageCacheTestRequest(http.MethodGet, "http://emby.invalid/emby/Items/123/Images/Primary", true))
		imageCacheRead(t, response, err)
	}
	if calls != 2 {
		t.Fatalf("response delay extended freshness: calls=%d", calls)
	}
}

// TestImageCacheForcedRefreshDoesNotPopulate verifies that a transparent
// refresh cannot become the cache result for a following ordinary request.
func TestImageCacheForcedRefreshDoesNotPopulate(t *testing.T) {
	for _, directive := range []struct{ header, value string }{
		{"Cache-Control", "no-cache"},
		{"Cache-Control", "max-age=0"},
		{"Pragma", "no-cache"},
	} {
		t.Run(directive.header+"="+directive.value, func(t *testing.T) {
			calls := 0
			cache := newImageCachingTransport(roundTripFunc(func(r *http.Request) (*http.Response, error) {
				calls++
				body := "ordinary"
				if r.Header.Get(directive.header) == directive.value {
					body = "refresh"
				}
				return imageCacheTestResponse(body), nil
			}), nil).(*imageCachingTransport)
			request := imageCacheTestRequest(http.MethodGet, "http://emby.invalid/emby/Items/123/Images/Primary", true)
			request.Header.Set(directive.header, directive.value)
			response, err := cache.RoundTrip(request)
			if imageCacheRead(t, response, err) != "refresh" || len(cache.entries) != 0 {
				t.Fatal("forced refresh was not a transparent, uncached response")
			}
			for i := 0; i < 2; i++ {
				response, err = cache.RoundTrip(imageCacheTestRequest(http.MethodGet, request.URL.String(), true))
				if imageCacheRead(t, response, err) != "ordinary" {
					t.Fatal("ordinary request inherited refresh result")
				}
			}
			if calls != 2 {
				t.Fatalf("calls=%d, want refresh and one ordinary fetch", calls)
			}
		})
	}
}

// TestImageCacheUncacheable304PreservesValidatedResponse keeps origin metadata
// from a valid 304 instead of discarding it through an unnecessary second GET.
func TestImageCacheUncacheable304PreservesValidatedResponse(t *testing.T) {
	for _, policy := range []string{"cookie", "private", "no-store"} {
		t.Run(policy, func(t *testing.T) {
			now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
			calls := 0
			cache := newImageCachingTransport(roundTripFunc(func(r *http.Request) (*http.Response, error) {
				calls++
				if calls == 2 {
					if r.Header.Get("If-None-Match") != "\"image-v1\"" {
						t.Error("expected internal conditional validation")
					}
					header := make(http.Header)
					if policy == "cookie" {
						header.Set("Set-Cookie", "fixture-session=rotated")
					} else {
						header.Set("Cache-Control", policy)
					}
					return &http.Response{StatusCode: http.StatusNotModified, Header: header, Body: http.NoBody}, nil
				}
				body := "validated-original"
				if calls > 2 {
					body = "unnecessary-refetch-without-cookie"
				}
				response := imageCacheTestResponse(body)
				response.Header.Set("Cache-Control", "max-age=1")
				return response, nil
			}), nil).(*imageCachingTransport)
			cache.now = func() time.Time { return now }
			request := func() *http.Request {
				return imageCacheTestRequest(http.MethodGet, "http://emby.invalid/emby/Items/123/Images/Primary", true)
			}
			response, err := cache.RoundTrip(request())
			imageCacheRead(t, response, err)
			now = now.Add(2 * time.Second)
			response, err = cache.RoundTrip(request())
			body := imageCacheRead(t, response, err)
			if response.StatusCode != http.StatusOK || body != "validated-original" || calls != 2 {
				t.Errorf("validated response was replaced: status=%d body=%q calls=%d", response.StatusCode, body, calls)
			}
			if policy == "cookie" && response.Header.Get("Set-Cookie") != "fixture-session=rotated" {
				t.Error("304 cookie rotation was discarded")
			}
			if policy != "cookie" && response.Header.Get("Cache-Control") != policy {
				t.Errorf("304 policy was discarded: %v", response.Header)
			}
			if len(cache.entries) != 0 {
				t.Error("uncacheable validation left an old representation cached")
			}
		})
	}
}

// TestImageCacheHeaderArrayBoundariesCannotCollide keeps multi-value headers
// distinct from multiple header names when an origin varies on the latter.
func TestImageCacheHeaderArrayBoundariesCannotCollide(t *testing.T) {
	calls := 0
	cache := newImageCachingTransport(roundTripFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		body := "missing-b"
		if r.Header.Get("X-B") == "bar" {
			body = "separate-b"
		}
		response := imageCacheTestResponse(body)
		response.Header.Set("Vary", "X-B")
		return response, nil
	}), nil).(*imageCachingTransport)
	first := imageCacheTestRequest(http.MethodGet, "http://emby.invalid/emby/Items/123/Images/Primary", true)
	first.Header["X-A"] = []string{"foo", "X-B", "bar"}
	second := imageCacheTestRequest(http.MethodGet, first.URL.String(), true)
	second.Header["X-A"] = []string{"foo"}
	second.Header["X-B"] = []string{"bar"}
	for i, request := range []*http.Request{first, second} {
		response, err := cache.RoundTrip(request)
		got := imageCacheRead(t, response, err)
		want := "missing-b"
		if i == 1 {
			want = "separate-b"
		}
		if got != want {
			t.Errorf("request %d representation=%q, want %q", i, got, want)
		}
	}
	if calls != 2 {
		t.Fatalf("header structure collision reused one representation: calls=%d", calls)
	}
}
