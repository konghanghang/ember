package p115

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"syscall"
	"testing"
)

// TestDownloadFileFailuresStayFileScoped exercises the full downurl boundary,
// including malformed variants which must still fail protocol validation.
func TestDownloadFileFailuresStayFileScoped(t *testing.T) {
	for _, test := range []struct {
		name, outer, entryURL string
		want                  error
	}{
		{name: "url false", entryURL: `false`, want: ErrFileUnavailable},
		{name: "url null", entryURL: `null`, want: ErrFileUnavailable},
		{name: "url absent", want: ErrFileUnavailable},
		{name: "url empty object", entryURL: `{}`, want: ErrFileUnavailable},
		{name: "url empty value", entryURL: `{"url":""}`, want: ErrFileUnavailable},
		{name: "url true", entryURL: `true`, want: ErrProviderProtocol},
		{name: "url number", entryURL: `42`, want: ErrProviderProtocol},
		{name: "url string", entryURL: `"fixture-secret"`, want: ErrProviderProtocol},
		{name: "url invalid value", entryURL: `{"url":false}`, want: ErrProviderProtocol},
		{name: "not logged in", outer: `{"state":false,"errno":99}`, want: ErrCredentialRejected},
		{name: "not logged in string", outer: `{"state":false,"errno":"990001"}`, want: ErrCredentialRejected},
		{name: "not logged in with false data", outer: `{"state":false,"errno":99,"data":false}`, want: ErrCredentialRejected},
		{name: "file deleted", outer: `{"state":false,"errno":70005}`, want: ErrFileUnavailable},
		{name: "file missing string", outer: `{"state":false,"errno":"231011"}`, want: ErrFileUnavailable},
		{name: "file missing with object data", outer: `{"state":false,"errno":70005,"data":{}}`, want: ErrFileUnavailable},
		{name: "pickcode missing", outer: `{"state":false,"errno":50003}`, want: ErrFileUnavailable},
		{name: "pickcode deleted", outer: `{"state":false,"errno":"50015"}`, want: ErrFileUnavailable},
		{name: "unknown errno", outer: `{"state":false,"errno":123456}`, want: ErrProviderRejected},
		{name: "missing errno", outer: `{"state":false}`, want: ErrProviderRejected},
		{name: "malformed errno", outer: `{"state":false,"errno":{"error":"fixture-secret"}}`, want: ErrProviderRejected},
	} {
		t.Run(test.name, func(t *testing.T) {
			body := test.outer
			if body == "" {
				body = `{"state":true,"data":"fixture-ciphertext"}`
			}
			calls := 0
			adapter, err := newCookieHTTPAdapter(httpDoerFunc(func(*http.Request) (*http.Response, error) {
				calls++
				return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body))}, nil
			}), cookieUploadInfoURL, cookieSHASearchURL, cookieUploadInitURL)
			if err != nil {
				t.Fatal(err)
			}
			adapter.downloadEncrypt = func([]byte) (string, error) { return "fixture-request", nil }
			adapter.downloadDecrypt = func(string) ([]byte, error) {
				entry := `{"pick_code":"` + fixtureDownloadPickCode + `"`
				if test.entryURL != "" {
					entry += `,"url":` + test.entryURL
				}
				return []byte(`{"789":` + entry + `}}`), nil
			}
			_, err = adapter.GetDownloadURL(context.Background(), fixtureCredential(), DownloadURLRequest{PickCode: fixtureDownloadPickCode, UserAgent: fixtureClientUserAgent})
			if !errors.Is(err, test.want) || calls != 1 {
				t.Fatalf("error=%v want=%v calls=%d", err, test.want, calls)
			}
			if strings.Contains(err.Error(), "fixture-secret") {
				t.Fatal("failure leaked upstream content")
			}
			if test.want == ErrFileUnavailable {
				stage, reason, _ := FailureDiagnostic(err)
				if stage != "download_url" || reason != "file_unavailable" {
					t.Fatalf("file failure diagnostics=%s/%s", stage, reason)
				}
			}
		})
	}
}

type failingCookieBody struct {
	err    error
	cancel context.CancelFunc
	closed bool
}

// Read simulates failure after headers, optionally cancelling the caller first.
func (body *failingCookieBody) Read([]byte) (int, error) {
	if body.cancel != nil {
		body.cancel()
	}
	return 0, body.err
}

// Close records cleanup even when the response body cannot be consumed.
func (body *failingCookieBody) Close() error { body.closed = true; return nil }

// TestCookieBodyReadFailureClassification distinguishes a client read timeout
// from caller cancellation without sockets, wall-clock waits or raw error leaks.
func TestCookieBodyReadFailureClassification(t *testing.T) {
	for _, download := range []bool{false, true} {
		for _, test := range []struct {
			name    string
			failure error
			cancel  bool
			want    error
		}{
			{"client timeout", &url.Error{Op: "read", URL: "https://fixture.invalid/fixture-secret", Err: context.DeadlineExceeded}, false, ErrProviderUnavailable},
			{"connection reset", syscall.ECONNRESET, false, ErrProviderUnavailable},
			{"truncated body", io.ErrUnexpectedEOF, false, ErrProviderUnavailable},
			{"caller canceled", errors.New("fixture-secret"), true, context.Canceled},
		} {
			t.Run(fmt.Sprintf("download=%t/%s", download, test.name), func(t *testing.T) {
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				body := &failingCookieBody{err: test.failure}
				if test.cancel {
					body.cancel = cancel
				}
				adapter, err := newCookieHTTPAdapter(httpDoerFunc(func(*http.Request) (*http.Response, error) {
					return &http.Response{StatusCode: http.StatusOK, Body: body}, nil
				}), cookieUploadInfoURL, cookieSHASearchURL, cookieUploadInitURL)
				if err != nil {
					t.Fatal(err)
				}
				stage := "cookie_get"
				if download {
					stage = "download_url"
					adapter.downloadEncrypt = func([]byte) (string, error) { return "fixture-request", nil }
					_, err = adapter.GetDownloadURL(ctx, fixtureCredential(), DownloadURLRequest{PickCode: fixtureDownloadPickCode, UserAgent: fixtureClientUserAgent})
				} else {
					_, err = adapter.GetUploadInfo(ctx, fixtureCredential())
				}
				if !errors.Is(err, test.want) || errors.Is(err, ErrProviderProtocol) || !body.closed {
					t.Fatalf("error=%v want=%v bodyClosed=%t", err, test.want, body.closed)
				}
				gotStage, _, status := FailureDiagnostic(err)
				if gotStage != stage || status != http.StatusOK || strings.Contains(err.Error(), "fixture-secret") {
					t.Fatalf("unsafe/missing read failure metadata: stage=%s status=%d error=%v", gotStage, status, err)
				}
			})
		}
	}
}

// TestCookieStateFalseCredentialCodesAcrossEndpoints keeps login-expired
// errno/errNo consistent with downurl on the other Cookie GET endpoints, while
// unknown and file-scoped codes stay ordinary rejections there.
func TestCookieStateFalseCredentialCodesAcrossEndpoints(t *testing.T) {
	for _, test := range []struct {
		name, body string
		want       error
	}{
		{name: "errno 99", body: `{"state":false,"errno":99}`, want: ErrCredentialRejected},
		{name: "errno string 990001", body: `{"state":false,"errno":"990001"}`, want: ErrCredentialRejected},
		{name: "errNo 990001", body: `{"state":false,"errNo":990001}`, want: ErrCredentialRejected},
		{name: "zero errno falls through to errNo", body: `{"state":false,"errno":0,"errNo":99}`, want: ErrCredentialRejected},
		{name: "file code stays rejected", body: `{"state":false,"errno":70005}`, want: ErrProviderRejected},
		{name: "unknown code", body: `{"state":false,"errno":123456}`, want: ErrProviderRejected},
		{name: "missing code", body: `{"state":false}`, want: ErrProviderRejected},
		{name: "malformed code", body: `{"state":false,"errno":{"error":"fixture-secret"}}`, want: ErrProviderRejected},
		{name: "malformed errno does not fall through", body: `{"state":false,"errno":{"error":"x"},"errNo":99}`, want: ErrProviderRejected},
		{name: "null errno falls through", body: `{"state":false,"errno":null,"errNo":99}`, want: ErrCredentialRejected},
	} {
		t.Run(test.name, func(t *testing.T) {
			adapter, err := newCookieHTTPAdapter(httpDoerFunc(func(*http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(test.body))}, nil
			}), cookieUploadInfoURL, cookieSHASearchURL, cookieUploadInitURL)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := adapter.GetUploadInfo(context.Background(), fixtureCredential()); !errors.Is(err, test.want) {
				t.Fatalf("GetUploadInfo() error = %v, want %v", err, test.want)
			}
			if _, err := adapter.SearchBySHA1(context.Background(), fixtureCredential(), FileQuery{SHA1: fixtureSHA1, Size: 1024}); !errors.Is(err, test.want) {
				t.Fatalf("SearchBySHA1() error = %v, want %v", err, test.want)
			}
		})
	}
}
