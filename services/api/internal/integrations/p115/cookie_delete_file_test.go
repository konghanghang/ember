package p115

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestCookieHTTPAdapterDeleteFileFixesRequestContract(t *testing.T) {
	var calls atomic.Int32
	adapter := newDeleteTestAdapter(t, httpDoerFunc(func(request *http.Request) (*http.Response, error) {
		calls.Add(1)
		if request.Method != http.MethodPost || request.URL.Path != "/rb/delete" || request.URL.RawQuery != "" {
			t.Fatalf("unexpected delete request: %s %s?%s", request.Method, request.URL.Path, request.URL.RawQuery)
		}
		if request.Header.Get("Cookie") != fixtureCookie || request.Header.Get("User-Agent") != fixtureUserAgent ||
			request.Header.Get("Accept") != "*/*" || request.Header.Get("Content-Type") != "application/x-www-form-urlencoded" ||
			request.Header.Get("Authorization") != "" {
			t.Fatalf("unexpected delete headers")
		}
		body, err := io.ReadAll(request.Body)
		if err != nil {
			t.Fatalf("read delete body: %v", err)
		}
		values, err := url.ParseQuery(string(body))
		if err != nil || len(values) != 1 || values.Get("fid") != "789" {
			t.Fatalf("delete body = %q values=%v error=%v", body, values, err)
		}
		return jsonHTTPResponse(http.StatusOK, `{"state":true}`), nil
	}))

	if err := adapter.DeleteFile(context.Background(), fixtureCredential(), "000789"); err != nil {
		t.Fatalf("DeleteFile() error = %v", err)
	}
	if calls.Load() != 1 {
		t.Fatalf("DeleteFile() calls = %d", calls.Load())
	}
}

func TestCookieHTTPAdapterDeleteFileSerializesSameProviderAccount(t *testing.T) {
	firstEntered := make(chan struct{})
	secondEntered := make(chan struct{})
	releaseFirst := make(chan struct{})
	var calls atomic.Int32
	adapter := newDeleteTestAdapter(t, httpDoerFunc(func(*http.Request) (*http.Response, error) {
		switch calls.Add(1) {
		case 1:
			close(firstEntered)
			<-releaseFirst
		case 2:
			close(secondEntered)
		}
		return jsonHTTPResponse(http.StatusOK, `{"state":true}`), nil
	}))

	firstDone := make(chan error, 1)
	go func() { firstDone <- adapter.DeleteFile(context.Background(), fixtureCredential(), "789") }()
	waitTestSignal(t, firstEntered, "first delete did not enter HTTP")

	secondStarted := make(chan struct{})
	secondDone := make(chan error, 1)
	go func() {
		close(secondStarted)
		secondDone <- adapter.DeleteFile(context.Background(), fixtureCredential(), "790")
	}()
	waitTestSignal(t, secondStarted, "second delete did not start")
	select {
	case <-secondEntered:
		t.Fatal("same provider account entered concurrent delete")
	case <-time.After(50 * time.Millisecond):
	}
	close(releaseFirst)
	waitTestSignal(t, secondEntered, "second delete did not enter after first release")
	if err := <-firstDone; err != nil {
		t.Fatalf("first DeleteFile() error = %v", err)
	}
	if err := <-secondDone; err != nil {
		t.Fatalf("second DeleteFile() error = %v", err)
	}
}

func TestCookieHTTPAdapterDeleteFileAllowsDifferentProviderAccountsInParallel(t *testing.T) {
	entered := make(chan struct{}, 2)
	release := make(chan struct{})
	adapter := newDeleteTestAdapter(t, httpDoerFunc(func(*http.Request) (*http.Response, error) {
		entered <- struct{}{}
		<-release
		return jsonHTTPResponse(http.StatusOK, `{"state":true}`), nil
	}))
	credentialA := fixtureCredential()
	credentialB := fixtureCredential()
	credentialB.AccountID = "account_fixture_b"
	credentialB.Cookie = "UID=0054321_A1; CID=fake-b; SEID=fake-b"

	done := make(chan error, 2)
	go func() { done <- adapter.DeleteFile(context.Background(), credentialA, "789") }()
	go func() { done <- adapter.DeleteFile(context.Background(), credentialB, "790") }()
	waitTestSignal(t, entered, "first provider delete did not enter")
	waitTestSignal(t, entered, "different provider account was incorrectly serialized")
	close(release)
	for range 2 {
		if err := <-done; err != nil {
			t.Fatalf("parallel DeleteFile() error = %v", err)
		}
	}
}

func TestCookieHTTPAdapterDeleteFileCancelsWhileWaitingForAccountLock(t *testing.T) {
	firstEntered := make(chan struct{})
	releaseFirst := make(chan struct{})
	var calls atomic.Int32
	adapter := newDeleteTestAdapter(t, httpDoerFunc(func(*http.Request) (*http.Response, error) {
		calls.Add(1)
		close(firstEntered)
		<-releaseFirst
		return jsonHTTPResponse(http.StatusOK, `{"state":true}`), nil
	}))

	firstDone := make(chan error, 1)
	go func() { firstDone <- adapter.DeleteFile(context.Background(), fixtureCredential(), "789") }()
	waitTestSignal(t, firstEntered, "first delete did not acquire account lock")

	ctx, cancel := context.WithCancel(context.Background())
	secondStarted := make(chan struct{})
	secondDone := make(chan error, 1)
	go func() {
		close(secondStarted)
		secondDone <- adapter.DeleteFile(ctx, fixtureCredential(), "790")
	}()
	waitTestSignal(t, secondStarted, "second delete did not start")
	cancel()
	if err := <-secondDone; !errors.Is(err, context.Canceled) {
		t.Fatalf("waiting DeleteFile() error = %v, want context.Canceled", err)
	}
	if calls.Load() != 1 {
		t.Fatalf("canceled waiter reached HTTP: calls=%d", calls.Load())
	}
	close(releaseFirst)
	if err := <-firstDone; err != nil {
		t.Fatalf("first DeleteFile() error = %v", err)
	}
}

func TestCookieHTTPAdapterDeleteFileRejectsInvalidIDBeforeHTTP(t *testing.T) {
	var calls atomic.Int32
	adapter := newDeleteTestAdapter(t, httpDoerFunc(func(*http.Request) (*http.Response, error) {
		calls.Add(1)
		return nil, errors.New("must not be called")
	}))
	for _, fileID := range []string{"", "0", "file-1", "1,2", "-1"} {
		if err := adapter.DeleteFile(context.Background(), fixtureCredential(), fileID); !errors.Is(err, ErrInvalidRequest) {
			t.Fatalf("DeleteFile(%q) error = %v, want ErrInvalidRequest", fileID, err)
		}
	}
	if calls.Load() != 0 {
		t.Fatalf("invalid file ID reached HTTP: calls=%d", calls.Load())
	}
}

func TestCookieHTTPAdapterDeleteFileMapsSafeProviderFailures(t *testing.T) {
	for _, test := range []struct {
		name       string
		statusCode int
		body       string
		wantErr    error
	}{
		{name: "http status", statusCode: http.StatusServiceUnavailable, body: "cookie-secret", wantErr: ErrProviderUnavailable},
		{name: "invalid json", statusCode: http.StatusOK, body: `{"state":`, wantErr: ErrProviderProtocol},
		{name: "missing state", statusCode: http.StatusOK, body: `{}`, wantErr: ErrProviderProtocol},
		{name: "provider rejected", statusCode: http.StatusOK, body: `{"state":false,"error":"cookie-secret"}`, wantErr: ErrProviderRejected},
		{name: "response too large", statusCode: http.StatusOK, body: strings.Repeat("x", maxCookieResponseBody+1), wantErr: ErrProviderProtocol},
	} {
		t.Run(test.name, func(t *testing.T) {
			adapter := newDeleteTestAdapter(t, httpDoerFunc(func(*http.Request) (*http.Response, error) {
				return jsonHTTPResponse(test.statusCode, test.body), nil
			}))
			err := adapter.DeleteFile(context.Background(), fixtureCredential(), "789")
			if !errors.Is(err, test.wantErr) {
				t.Fatalf("DeleteFile() error = %v, want %v", err, test.wantErr)
			}
			if strings.Contains(fmt.Sprint(err), "cookie-secret") {
				t.Fatalf("delete error exposed provider details: %v", err)
			}
		})
	}

	t.Run("transport", func(t *testing.T) {
		adapter := newDeleteTestAdapter(t, httpDoerFunc(func(*http.Request) (*http.Response, error) {
			return nil, &url.Error{Op: "Post", URL: "https://webapi.115.com/rb/delete?secret=cookie-secret", Err: errors.New("dial failed")}
		}))
		err := adapter.DeleteFile(context.Background(), fixtureCredential(), "789")
		if !errors.Is(err, ErrProviderUnavailable) || strings.Contains(fmt.Sprint(err), "cookie-secret") || strings.Contains(fmt.Sprint(err), "webapi.115.com") {
			t.Fatalf("delete transport error = %v", err)
		}
	})
}

func newDeleteTestAdapter(t *testing.T, client httpDoer) *CookieHTTPAdapter {
	t.Helper()
	adapter, err := newCookieHTTPAdapter(
		client,
		"https://example.invalid/app/uploadinfo",
		"https://example.invalid/files/shasearch",
		"https://example.invalid/4.0/initupload.php",
	)
	if err != nil {
		t.Fatalf("newCookieHTTPAdapter() error = %v", err)
	}
	return adapter
}

func waitTestSignal(t *testing.T, signal <-chan struct{}, failure string) {
	t.Helper()
	select {
	case <-signal:
	case <-time.After(time.Second):
		t.Fatal(failure)
	}
}
