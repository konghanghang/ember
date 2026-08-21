package p115

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

const (
	fixtureDownloadPickCode = "a1b2c3d4e5f6g7h8"
	fixtureClientUserAgent  = "Infuse/8.1 Ember-Contract"
	fixtureDownloadExpiry   = int64(1700003600)
)

func TestCookieHTTPAdapterGetDownloadURLUsesRealClientUserAgentAndMapsURL(t *testing.T) {
	directURL := fixtureDirectDownloadURL("0", "1", fixtureDownloadExpiry)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost || request.URL.Path != "/app/chrome/downurl" || request.URL.RawQuery != "" {
			t.Fatalf("unexpected download request: %s %s?%s", request.Method, request.URL.Path, request.URL.RawQuery)
		}
		if request.Header.Get("Cookie") != fixtureCookie || request.Header.Get("Accept") != "*/*" {
			t.Fatalf("unexpected download credential headers")
		}
		if request.Header.Get("User-Agent") != fixtureClientUserAgent {
			t.Fatalf("download User-Agent = %q, want real client UA", request.Header.Get("User-Agent"))
		}
		if request.Header.Get("Content-Type") != "application/x-www-form-urlencoded" || request.Header.Get("Authorization") != "" {
			t.Fatalf("unexpected download request headers")
		}
		body, err := io.ReadAll(request.Body)
		if err != nil {
			t.Fatalf("read download request: %v", err)
		}
		values, err := url.ParseQuery(string(body))
		if err != nil || len(values) != 1 || values.Get("data") != "encrypted-request" {
			t.Fatalf("unexpected download form: %q error=%v", body, err)
		}
		_, _ = w.Write([]byte(`{"state":true,"data":"encrypted-response"}`))
	}))
	defer server.Close()

	adapter := newTestDownloadURLAdapter(t, server, directURL)
	result, err := adapter.GetDownloadURL(context.Background(), fixtureCredential(), DownloadURLRequest{
		PickCode:  fixtureDownloadPickCode,
		UserAgent: fixtureClientUserAgent,
	})
	if err != nil {
		t.Fatalf("GetDownloadURL() error = %v", err)
	}
	if result.URL != directURL || result.HeaderMode != DownloadHeadersSameUserAgent ||
		result.ConcurrentOpenLimit != 0 || !result.ExpiresAt.Equal(time.Unix(fixtureDownloadExpiry, 0).UTC()) {
		t.Fatalf("GetDownloadURL() = %+v", result)
	}
}

func TestCookieHTTPAdapterGetDownloadURLMapsHeaderModesAndConcurrency(t *testing.T) {
	tests := []struct {
		name      string
		flag      string
		limit     string
		wantMode  DownloadHeaderMode
		wantLimit int64
	}{
		{name: "empty flag", flag: "", limit: "0", wantMode: DownloadHeadersNone, wantLimit: 0},
		{name: "zero flag", flag: "0", limit: "3", wantMode: DownloadHeadersNone, wantLimit: 3},
		{name: "same user agent", flag: "1", limit: "1", wantMode: DownloadHeadersSameUserAgent, wantLimit: 1},
		{name: "user agent and cookie", flag: "3", limit: "2", wantMode: DownloadHeadersSameUserAgentAndCookie, wantLimit: 2},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := newDownloadURLServer(t)
			defer server.Close()
			adapter := newTestDownloadURLAdapter(t, server, fixtureDirectDownloadURL(test.limit, test.flag, fixtureDownloadExpiry))
			result, err := adapter.GetDownloadURL(context.Background(), fixtureCredential(), DownloadURLRequest{
				PickCode: fixtureDownloadPickCode, UserAgent: fixtureClientUserAgent,
			})
			if err != nil {
				t.Fatalf("GetDownloadURL() error = %v", err)
			}
			if result.HeaderMode != test.wantMode || result.ConcurrentOpenLimit != test.wantLimit {
				t.Fatalf("GetDownloadURL() = %+v", result)
			}
		})
	}
}

func TestCookieHTTPAdapterGetDownloadURLRejectsUnsafeURLs(t *testing.T) {
	tests := []struct {
		name    string
		url     string
		wantErr error
	}{
		{name: "http scheme", url: "http://cdnfhnfile.115.com/video.mkv?t=1700003600&c=0&f=1", wantErr: ErrDownloadURLNotAllowed},
		{name: "foreign host", url: "https://evil.example/video.mkv?t=1700003600&c=0&f=1", wantErr: ErrDownloadURLNotAllowed},
		{name: "userinfo", url: "https://user@cdnfhnfile.115.com/video.mkv?t=1700003600&c=0&f=1", wantErr: ErrDownloadURLNotAllowed},
		{name: "port", url: "https://cdnfhnfile.115.com:8443/video.mkv?t=1700003600&c=0&f=1", wantErr: ErrDownloadURLNotAllowed},
		{name: "fragment", url: "https://cdnfhnfile.115.com/video.mkv?t=1700003600&c=0&f=1#fragment", wantErr: ErrDownloadURLNotAllowed},
		{name: "missing expiry", url: "https://cdnfhnfile.115.com/video.mkv?c=0&f=1", wantErr: ErrProviderProtocol},
		{name: "duplicate expiry", url: "https://cdnfhnfile.115.com/video.mkv?t=1700003600&t=1700007200&c=0&f=1", wantErr: ErrProviderProtocol},
		{name: "expired", url: "https://cdnfhnfile.115.com/video.mkv?t=1700000000&c=0&f=1", wantErr: ErrDownloadURLExpired},
		{name: "missing concurrency", url: "https://cdnfhnfile.115.com/video.mkv?t=1700003600&f=1", wantErr: ErrProviderProtocol},
		{name: "invalid concurrency", url: "https://cdnfhnfile.115.com/video.mkv?t=1700003600&c=-1&f=1", wantErr: ErrProviderProtocol},
		{name: "missing header flag", url: "https://cdnfhnfile.115.com/video.mkv?t=1700003600&c=0", wantErr: ErrProviderProtocol},
		{name: "unknown header flag", url: "https://cdnfhnfile.115.com/video.mkv?t=1700003600&c=0&f=2", wantErr: ErrDownloadURLIncompatible},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := newDownloadURLServer(t)
			defer server.Close()
			adapter := newTestDownloadURLAdapter(t, server, test.url)
			_, err := adapter.GetDownloadURL(context.Background(), fixtureCredential(), DownloadURLRequest{
				PickCode: fixtureDownloadPickCode, UserAgent: fixtureClientUserAgent,
			})
			if !errors.Is(err, test.wantErr) {
				t.Fatalf("GetDownloadURL() error = %v, want %v", err, test.wantErr)
			}
			if strings.Contains(fmt.Sprint(err), test.url) {
				t.Fatalf("download error exposed URL: %v", err)
			}
		})
	}
}

func TestCookieHTTPAdapterGetDownloadURLRejectsInvalidRequestBeforeHTTP(t *testing.T) {
	var calls atomic.Int32
	adapter, err := newCookieHTTPAdapter(httpDoerFunc(func(*http.Request) (*http.Response, error) {
		calls.Add(1)
		return nil, errors.New("must not be called")
	}), "https://example.invalid/app/uploadinfo", "https://example.invalid/files/shasearch", "https://example.invalid/4.0/initupload.php")
	if err != nil {
		t.Fatalf("newCookieHTTPAdapter() error = %v", err)
	}
	for _, request := range []DownloadURLRequest{
		{PickCode: "bad-pick-code", UserAgent: fixtureClientUserAgent},
		{PickCode: fixtureDownloadPickCode, UserAgent: ""},
		{PickCode: fixtureDownloadPickCode, UserAgent: "Infuse\r\nCookie: secret"},
	} {
		if _, err := adapter.GetDownloadURL(context.Background(), fixtureCredential(), request); !errors.Is(err, ErrInvalidRequest) {
			t.Fatalf("GetDownloadURL(%+v) error = %v, want ErrInvalidRequest", request, err)
		}
	}
	if calls.Load() != 0 {
		t.Fatalf("invalid download request reached HTTP: calls=%d", calls.Load())
	}
}

func TestCookieHTTPAdapterGetDownloadURLMapsSafeProviderFailures(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		body       string
		decrypt    func(string) ([]byte, error)
		wantErr    error
	}{
		{name: "http status", statusCode: http.StatusServiceUnavailable, body: "cookie-secret", wantErr: ErrProviderUnavailable},
		{name: "invalid outer json", statusCode: http.StatusOK, body: `{"state":`, wantErr: ErrProviderProtocol},
		{name: "provider rejected", statusCode: http.StatusOK, body: `{"state":false,"error":"cookie-secret"}`, wantErr: ErrProviderRejected},
		{name: "missing data", statusCode: http.StatusOK, body: `{"state":true}`, wantErr: ErrProviderProtocol},
		{name: "decrypt failure", statusCode: http.StatusOK, body: `{"state":true,"data":"encrypted-response"}`, decrypt: func(string) ([]byte, error) { return nil, errors.New("cookie-secret") }, wantErr: ErrProviderProtocol},
		{name: "invalid decrypted json", statusCode: http.StatusOK, body: `{"state":true,"data":"encrypted-response"}`, decrypt: func(string) ([]byte, error) { return []byte(`{"url":"cookie-secret"`), nil }, wantErr: ErrProviderProtocol},
		{name: "multiple files", statusCode: http.StatusOK, body: `{"state":true,"data":"encrypted-response"}`, decrypt: func(string) ([]byte, error) {
			return []byte(`{"1":{"pick_code":"` + fixtureDownloadPickCode + `"},"2":{"pick_code":"other"}}`), nil
		}, wantErr: ErrProviderProtocol},
		{name: "pickcode mismatch", statusCode: http.StatusOK, body: `{"state":true,"data":"encrypted-response"}`, decrypt: func(string) ([]byte, error) {
			return downloadResponseJSON("otherpickcode00000", fixtureDirectDownloadURL("0", "1", fixtureDownloadExpiry)), nil
		}, wantErr: ErrProviderProtocol},
		{name: "directory", statusCode: http.StatusOK, body: `{"state":true,"data":"encrypted-response"}`, decrypt: func(string) ([]byte, error) {
			return []byte(`{"789":{"pick_code":"` + fixtureDownloadPickCode + `","url":null}}`), nil
		}, wantErr: ErrProviderRejected},
		{name: "response too large", statusCode: http.StatusOK, body: strings.Repeat("x", maxCookieResponseBody+1), wantErr: ErrProviderProtocol},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(test.statusCode)
				_, _ = w.Write([]byte(test.body))
			}))
			defer server.Close()
			adapter := newTestDownloadURLAdapter(t, server, fixtureDirectDownloadURL("0", "1", fixtureDownloadExpiry))
			if test.decrypt != nil {
				adapter.downloadDecrypt = test.decrypt
			}
			_, err := adapter.GetDownloadURL(context.Background(), fixtureCredential(), DownloadURLRequest{
				PickCode: fixtureDownloadPickCode, UserAgent: fixtureClientUserAgent,
			})
			if !errors.Is(err, test.wantErr) {
				t.Fatalf("GetDownloadURL() error = %v, want %v", err, test.wantErr)
			}
			if strings.Contains(fmt.Sprint(err), "cookie-secret") {
				t.Fatalf("download error exposed provider details: %v", err)
			}
		})
	}

	t.Run("encrypt failure", func(t *testing.T) {
		var calls atomic.Int32
		adapter, err := newCookieHTTPAdapter(httpDoerFunc(func(*http.Request) (*http.Response, error) {
			calls.Add(1)
			return nil, errors.New("must not be called")
		}), "https://example.invalid/app/uploadinfo", "https://example.invalid/files/shasearch", "https://example.invalid/4.0/initupload.php")
		if err != nil {
			t.Fatalf("newCookieHTTPAdapter() error = %v", err)
		}
		adapter.downloadEncrypt = func([]byte) (string, error) { return "", errors.New("cookie-secret") }
		_, err = adapter.GetDownloadURL(context.Background(), fixtureCredential(), DownloadURLRequest{
			PickCode: fixtureDownloadPickCode, UserAgent: fixtureClientUserAgent,
		})
		if !errors.Is(err, ErrProviderProtocol) || calls.Load() != 0 || strings.Contains(fmt.Sprint(err), "cookie-secret") {
			t.Fatalf("download encrypt error=%v calls=%d", err, calls.Load())
		}
	})

	t.Run("transport", func(t *testing.T) {
		adapter, err := newCookieHTTPAdapter(httpDoerFunc(func(*http.Request) (*http.Response, error) {
			return nil, &url.Error{Op: "Post", URL: "https://proapi.115.com/app/chrome/downurl?secret=cookie-secret", Err: errors.New("dial failed")}
		}), "https://example.invalid/app/uploadinfo", "https://example.invalid/files/shasearch", "https://example.invalid/4.0/initupload.php")
		if err != nil {
			t.Fatalf("newCookieHTTPAdapter() error = %v", err)
		}
		adapter.downloadEncrypt = func([]byte) (string, error) { return "encrypted-request", nil }
		_, err = adapter.GetDownloadURL(context.Background(), fixtureCredential(), DownloadURLRequest{
			PickCode: fixtureDownloadPickCode, UserAgent: fixtureClientUserAgent,
		})
		if !errors.Is(err, ErrProviderUnavailable) || strings.Contains(fmt.Sprint(err), "cookie-secret") || strings.Contains(fmt.Sprint(err), "proapi.115.com") {
			t.Fatalf("download transport error = %v", err)
		}
	})
}

func newTestDownloadURLAdapter(t *testing.T, server *httptest.Server, directURL string) *CookieHTTPAdapter {
	t.Helper()
	adapter := newTestRapidUploadAdapter(t, server)
	adapter.now = func() time.Time { return time.Unix(fixtureUploadTimestamp, 0) }
	adapter.downloadEncrypt = func(plaintext []byte) (string, error) {
		want := `{"pickcode":"` + fixtureDownloadPickCode + `","user_id":12345}`
		if string(plaintext) != want {
			t.Fatalf("download plaintext = %s, want %s", plaintext, want)
		}
		return "encrypted-request", nil
	}
	adapter.downloadDecrypt = func(ciphertext string) ([]byte, error) {
		if ciphertext != "encrypted-response" {
			t.Fatalf("download ciphertext = %q", ciphertext)
		}
		return downloadResponseJSON(fixtureDownloadPickCode, directURL), nil
	}
	return adapter
}

func newDownloadURLServer(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"state":true,"data":"encrypted-response"}`))
	}))
}

func downloadResponseJSON(pickCode, directURL string) []byte {
	return []byte(`{"789":{"file_name":"fixture-video.mkv","file_size":"1024","pick_code":"` +
		pickCode + `","sha1":"` + fixtureSHA1 + `","url":{"url":"` + directURL + `"}}}`)
}

func fixtureDirectDownloadURL(concurrency, flag string, expiry int64) string {
	values := url.Values{
		"t": {fmt.Sprint(expiry)},
		"u": {"12345"},
		"c": {concurrency},
		"f": {flag},
		"k": {"fixture-signature"},
	}
	return "https://cdnfhnfile.115.com/path/fixture-video.mkv?" + values.Encode()
}
