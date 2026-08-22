package p115

import (
	"context"
	"crypto/sha1"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestCookieHTTPAdapterHashFileRangeUsesSignedHeadersAndReturnsSHA1(t *testing.T) {
	content := []byte("0123456789")
	tests := []struct {
		name       string
		flag       string
		wantCookie bool
	}{
		{name: "no required headers", flag: "0"},
		{name: "same user agent", flag: "1"},
		{name: "same user agent and cookie", flag: "3", wantCookie: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			adapter := newTestRangeAdapter(t, test.flag, func(request *http.Request) (*http.Response, error) {
				if request.Method != http.MethodGet || request.URL.Host != "range.test" || request.URL.Path != "/fixture-video.mkv" {
					t.Fatalf("unexpected range request: %s %s", request.Method, request.URL.String())
				}
				if request.Header.Get("Range") != "bytes=10-19" || request.Header.Get("Accept-Encoding") != "identity" {
					t.Fatalf("unexpected range headers: %+v", request.Header)
				}
				if request.Header.Get("User-Agent") != fixtureUserAgent {
					t.Fatalf("range User-Agent = %q", request.Header.Get("User-Agent"))
				}
				if got := request.Header.Get("Cookie") != ""; got != test.wantCookie {
					t.Fatalf("range Cookie present=%t, want %t", got, test.wantCookie)
				}
				return rangeHTTPResponse(http.StatusPartialContent, "bytes 10-19/1024", content), nil
			})

			result, err := adapter.HashFileRange(context.Background(), fixtureCredential(), FileRangeRequest{
				File:  File{ID: "789", PickCode: fixtureDownloadPickCode, Size: 1024},
				Range: ByteRange{Start: 10, End: 19},
			})
			if err != nil {
				t.Fatalf("HashFileRange() error = %v", err)
			}
			wantHash := fmt.Sprintf("%X", sha1.Sum(content))
			if result.SHA1 != wantHash || result.BytesRead != int64(len(content)) {
				t.Fatalf("HashFileRange() = %+v, want SHA1=%s bytes=%d", result, wantHash, len(content))
			}
		})
	}
}

func TestCookieHTTPAdapterHashFileRangeRejectsInvalidRequestBeforeHTTP(t *testing.T) {
	var calls atomic.Int32
	adapter, err := newCookieHTTPAdapter(httpDoerFunc(func(*http.Request) (*http.Response, error) {
		calls.Add(1)
		return nil, errors.New("must not be called")
	}), "https://example.invalid/app/uploadinfo", "https://example.invalid/files/shasearch", "https://example.invalid/4.0/initupload.php")
	if err != nil {
		t.Fatalf("newCookieHTTPAdapter() error = %v", err)
	}

	valid := FileRangeRequest{
		File:  File{ID: "789", PickCode: fixtureDownloadPickCode, Size: 1024},
		Range: ByteRange{Start: 10, End: 19},
	}
	tests := []FileRangeRequest{
		{File: File{PickCode: fixtureDownloadPickCode, Size: 1024, IsDirectory: true}, Range: valid.Range},
		{File: File{PickCode: "bad-pick-code", Size: 1024}, Range: valid.Range},
		{File: File{PickCode: fixtureDownloadPickCode, Size: 0}, Range: valid.Range},
		{File: valid.File, Range: ByteRange{Start: -1, End: 19}},
		{File: valid.File, Range: ByteRange{Start: 20, End: 19}},
		{File: valid.File, Range: ByteRange{Start: 10, End: 1024}},
		{File: File{PickCode: fixtureDownloadPickCode, Size: maxSourceRangeBytes + 1}, Range: ByteRange{Start: 0, End: maxSourceRangeBytes}},
	}
	for _, request := range tests {
		if _, err := adapter.HashFileRange(context.Background(), fixtureCredential(), request); !errors.Is(err, ErrInvalidRequest) {
			t.Fatalf("HashFileRange(%+v) error = %v, want ErrInvalidRequest", request, err)
		}
	}
	if calls.Load() != 0 {
		t.Fatalf("invalid range request reached HTTP: calls=%d", calls.Load())
	}
}

func TestCookieHTTPAdapterHashFileRangeRejectsUnsafeResponses(t *testing.T) {
	tests := []struct {
		name    string
		respond func(*http.Request) (*http.Response, error)
		wantErr error
	}{
		{name: "transport", respond: func(*http.Request) (*http.Response, error) {
			return nil, errors.New("https://range.test/cookie-secret")
		}, wantErr: ErrProviderUnavailable},
		{name: "upstream status", respond: func(*http.Request) (*http.Response, error) {
			return rangeHTTPResponse(http.StatusServiceUnavailable, "", []byte("cookie-secret")), nil
		}, wantErr: ErrProviderUnavailable},
		{name: "ignored range", respond: func(*http.Request) (*http.Response, error) {
			return rangeHTTPResponse(http.StatusOK, "", []byte("0123456789")), nil
		}, wantErr: ErrProviderProtocol},
		{name: "wrong content range", respond: func(*http.Request) (*http.Response, error) {
			return rangeHTTPResponse(http.StatusPartialContent, "bytes 11-20/1024", []byte("0123456789")), nil
		}, wantErr: ErrProviderProtocol},
		{name: "compressed response", respond: func(*http.Request) (*http.Response, error) {
			response := rangeHTTPResponse(http.StatusPartialContent, "bytes 10-19/1024", []byte("0123456789"))
			response.Header.Set("Content-Encoding", "gzip")
			return response, nil
		}, wantErr: ErrProviderProtocol},
		{name: "unknown content length", respond: func(*http.Request) (*http.Response, error) {
			response := rangeHTTPResponse(http.StatusPartialContent, "bytes 10-19/1024", []byte("0123456789"))
			response.ContentLength = -1
			return response, nil
		}, wantErr: ErrProviderProtocol},
		{name: "short body", respond: func(*http.Request) (*http.Response, error) {
			return rangeHTTPResponse(http.StatusPartialContent, "bytes 10-19/1024", []byte("short")), nil
		}, wantErr: ErrProviderProtocol},
		{name: "long body", respond: func(*http.Request) (*http.Response, error) {
			return rangeHTTPResponse(http.StatusPartialContent, "bytes 10-19/1024", []byte("01234567890")), nil
		}, wantErr: ErrProviderProtocol},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			adapter := newTestRangeAdapter(t, "1", test.respond)
			_, err := adapter.HashFileRange(context.Background(), fixtureCredential(), FileRangeRequest{
				File:  File{ID: "789", PickCode: fixtureDownloadPickCode, Size: 1024},
				Range: ByteRange{Start: 10, End: 19},
			})
			if !errors.Is(err, test.wantErr) {
				t.Fatalf("HashFileRange() error = %v, want %v", err, test.wantErr)
			}
			if strings.Contains(fmt.Sprint(err), "range.test") || strings.Contains(fmt.Sprint(err), "cookie-secret") {
				t.Fatalf("HashFileRange() exposed sensitive upstream data: %v", err)
			}
		})
	}
}

func newTestRangeAdapter(
	t *testing.T,
	headerFlag string,
	rangeResponder func(*http.Request) (*http.Response, error),
) *CookieHTTPAdapter {
	t.Helper()
	directURL := "https://range.test/fixture-video.mkv?t=1700003600&c=1&f=" + headerFlag
	adapter, err := newCookieHTTPAdapter(httpDoerFunc(func(request *http.Request) (*http.Response, error) {
		if request.URL.Host == "range.test" {
			return rangeResponder(request)
		}
		if request.URL.Path != "/app/chrome/downurl" {
			t.Fatalf("unexpected adapter request: %s", request.URL.String())
		}
		return jsonHTTPResponse(http.StatusOK, `{"state":true,"data":"encrypted-response"}`), nil
	}), "https://provider.test/app/uploadinfo", "https://provider.test/files/shasearch", "https://provider.test/4.0/initupload.php")
	if err != nil {
		t.Fatalf("newCookieHTTPAdapter() error = %v", err)
	}
	adapter.now = func() time.Time { return time.Unix(fixtureUploadTimestamp, 0) }
	adapter.downloadEncrypt = func([]byte) (string, error) { return "encrypted-request", nil }
	adapter.downloadDecrypt = func(string) ([]byte, error) {
		return downloadResponseJSON(fixtureDownloadPickCode, directURL), nil
	}
	adapter.downloadHostAllowed = func(host string) bool { return host == "range.test" }
	return adapter
}

func rangeHTTPResponse(statusCode int, contentRange string, body []byte) *http.Response {
	response := &http.Response{
		StatusCode:    statusCode,
		Header:        make(http.Header),
		Body:          io.NopCloser(strings.NewReader(string(body))),
		ContentLength: int64(len(body)),
	}
	if contentRange != "" {
		response.Header.Set("Content-Range", contentRange)
	}
	response.Header.Set("Content-Length", fmt.Sprint(len(body)))
	return response
}
