package p115

import (
	"bytes"
	"context"
	"encoding/binary"
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

	"github.com/konghang/ember/backend/internal/integrations/p115/p115cipher"
)

const (
	fixtureUploadTimestamp = int64(1700000000)
	fixtureUploadFileName  = "fixture-video.mkv"
	fixtureUploadParentID  = "200000002"
	fixtureUploadPreID     = "89ABCDEF0123456789ABCDEF0123456789ABCDEF"
	fixtureUploadSignValue = "FEDCBA9876543210FEDCBA9876543210FEDCBA98"
)

func TestCookieHTTPAdapterInitRapidUploadSendsPinnedEncryptedRequest(t *testing.T) {
	var calls atomic.Int32
	rapidRequest := fixtureRapidUploadRequest()
	rapidRequest.SignKey = ""
	rapidRequest.SignValue = ""
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/app/uploadinfo":
			if calls.Add(1) != 1 {
				t.Fatalf("upload info request was not first")
			}
			_, _ = w.Write([]byte(`{"state":true,"user_id":12345,"userkey":"fixture-user-key"}`))
		case "/4.0/initupload.php":
			if calls.Add(1) != 2 {
				t.Fatalf("upload init request was not second")
			}
			assertRapidUploadHTTPRequest(t, request)
			body, err := io.ReadAll(request.Body)
			if err != nil {
				t.Fatalf("read upload request: %v", err)
			}
			expected, err := p115cipher.BuildUploadRequest(p115cipher.UploadPayload{
				UserKey:    "fixture-user-key",
				UserID:     "12345",
				FileID:     fixtureSHA1,
				FileName:   fixtureUploadFileName,
				Target:     "U_1_" + fixtureUploadParentID,
				FileSize:   1024,
				PreID:      fixtureUploadPreID,
				SignKey:    rapidRequest.SignKey,
				SignValue:  rapidRequest.SignValue,
				TopUpload:  "true",
				AppVersion: cookieUploadAppVersion,
			}, fixtureUploadTimestamp)
			if err != nil {
				t.Fatalf("build expected upload request: %v", err)
			}
			if request.URL.Query().Get("k_ec") != expected.KEc || len(request.URL.Query()) != 1 {
				t.Fatalf("unexpected upload query: %s", request.URL.RawQuery)
			}
			if !bytes.Equal(body, expected.Data) {
				t.Fatalf("upload body does not match pinned encrypted payload")
			}
			writeEncryptedUploadResponse(t, w, `{"status":2,"statuscode":0,"pickcode":"fixture-pick-code"}`)
		default:
			t.Fatalf("unexpected request path: %s", request.URL.Path)
		}
	}))
	defer server.Close()

	adapter := newTestRapidUploadAdapter(t, server)
	result, err := adapter.InitRapidUpload(context.Background(), fixtureCredential(), rapidRequest)
	if err != nil {
		t.Fatalf("InitRapidUpload() error = %v", err)
	}
	if result.Status != RapidUploadReused || result.ProviderCode != "0" || result.File != nil || result.Challenge != nil {
		t.Fatalf("InitRapidUpload() result = %+v", result)
	}
	if calls.Load() != 2 {
		t.Fatalf("InitRapidUpload() calls = %d, want 2", calls.Load())
	}
}

func TestCookieHTTPAdapterInitRapidUploadMapsStatuses(t *testing.T) {
	tests := []struct {
		name          string
		response      string
		wantStatus    RapidUploadStatus
		wantCode      string
		wantChallenge *RapidUploadChallenge
	}{
		{name: "ordinary upload", response: `{"status":1,"statuscode":100}`, wantStatus: RapidUploadOrdinaryUploadRequired, wantCode: "100"},
		{name: "reused", response: `{"status":"2","statuscode":"0"}`, wantStatus: RapidUploadReused, wantCode: "0"},
		{
			name:       "range challenge",
			response:   `{"status":7,"statuscode":701,"sign_key":"fixture-sign-key","sign_check":"10-19"}`,
			wantStatus: RapidUploadRangeChallenge,
			wantCode:   "701",
			wantChallenge: &RapidUploadChallenge{
				Range:   ByteRange{Start: 10, End: 19},
				SignKey: "fixture-sign-key",
			},
		},
		{name: "unknown status", response: `{"status":99,"statuscode":"fixture_code"}`, wantStatus: RapidUploadProviderRejected, wantCode: "fixture_code"},
		{name: "state rejected", response: `{"state":false,"errno":990009,"error":"cookie-secret"}`, wantStatus: RapidUploadProviderRejected, wantCode: "990009"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := newRapidUploadServer(t, test.response)
			defer server.Close()

			adapter := newTestRapidUploadAdapter(t, server)
			result, err := adapter.InitRapidUpload(context.Background(), fixtureCredential(), fixtureRapidUploadRequest())
			if err != nil {
				t.Fatalf("InitRapidUpload() error = %v", err)
			}
			if result.Status != test.wantStatus || result.ProviderCode != test.wantCode {
				t.Fatalf("InitRapidUpload() result = %+v", result)
			}
			if test.wantChallenge == nil {
				if result.Challenge != nil {
					t.Fatalf("unexpected challenge: %+v", result.Challenge)
				}
			} else if result.Challenge == nil || *result.Challenge != *test.wantChallenge {
				t.Fatalf("challenge = %+v, want %+v", result.Challenge, test.wantChallenge)
			}
		})
	}
}

func TestCookieHTTPAdapterInitRapidUploadRejectsInvalidRequestBeforeHTTP(t *testing.T) {
	var calls atomic.Int32
	adapter, err := newCookieHTTPAdapter(httpDoerFunc(func(*http.Request) (*http.Response, error) {
		calls.Add(1)
		return nil, errors.New("must not be called")
	}), "https://example.invalid/app/uploadinfo", "https://example.invalid/files/shasearch", "https://example.invalid/4.0/initupload.php")
	if err != nil {
		t.Fatalf("newCookieHTTPAdapter() error = %v", err)
	}

	tests := []struct {
		name   string
		mutate func(*RapidUploadRequest)
	}{
		{name: "missing filename", mutate: func(request *RapidUploadRequest) { request.FileName = "" }},
		{name: "invalid filename encoding", mutate: func(request *RapidUploadRequest) { request.FileName = string([]byte{0xff}) }},
		{name: "invalid sha1", mutate: func(request *RapidUploadRequest) { request.SHA1 = "bad" }},
		{name: "non positive size", mutate: func(request *RapidUploadRequest) { request.Size = 0 }},
		{name: "invalid parent", mutate: func(request *RapidUploadRequest) { request.TargetParentID = "target" }},
		{name: "invalid preid", mutate: func(request *RapidUploadRequest) { request.PreID = "bad" }},
		{name: "missing sign value", mutate: func(request *RapidUploadRequest) { request.SignValue = "" }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := fixtureRapidUploadRequest()
			test.mutate(&request)
			if _, err := adapter.InitRapidUpload(context.Background(), fixtureCredential(), request); !errors.Is(err, ErrInvalidRequest) {
				t.Fatalf("InitRapidUpload() error = %v, want ErrInvalidRequest", err)
			}
		})
	}
	if calls.Load() != 0 {
		t.Fatalf("invalid rapid-upload request reached HTTP client: calls=%d", calls.Load())
	}
}

func TestCookieHTTPAdapterInitRapidUploadMapsSafeFailures(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		body       []byte
		wantErr    error
	}{
		{name: "http status", statusCode: http.StatusServiceUnavailable, body: []byte("cookie-secret"), wantErr: ErrProviderUnavailable},
		{name: "invalid encrypted response", statusCode: http.StatusOK, body: []byte("cookie-secret"), wantErr: ErrProviderProtocol},
		{name: "invalid status", statusCode: http.StatusOK, body: encryptedUploadResponse(t, `{"status":"bad","error":"cookie-secret"}`), wantErr: ErrProviderProtocol},
		{name: "missing challenge", statusCode: http.StatusOK, body: encryptedUploadResponse(t, `{"status":7,"statuscode":701}`), wantErr: ErrProviderProtocol},
		{name: "invalid challenge key", statusCode: http.StatusOK, body: encryptedUploadResponse(t, `{"status":7,"statuscode":701,"sign_key":"密钥","sign_check":"10-19"}`), wantErr: ErrProviderProtocol},
		{name: "invalid challenge", statusCode: http.StatusOK, body: encryptedUploadResponse(t, `{"status":7,"statuscode":701,"sign_key":"cookie-secret","sign_check":"10-2048"}`), wantErr: ErrProviderProtocol},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
				if request.URL.Path == "/app/uploadinfo" {
					_, _ = w.Write([]byte(`{"state":true,"user_id":12345,"userkey":"fixture-user-key"}`))
					return
				}
				w.WriteHeader(test.statusCode)
				_, _ = w.Write(test.body)
			}))
			defer server.Close()

			adapter := newTestRapidUploadAdapter(t, server)
			_, err := adapter.InitRapidUpload(context.Background(), fixtureCredential(), fixtureRapidUploadRequest())
			if !errors.Is(err, test.wantErr) {
				t.Fatalf("InitRapidUpload() error = %v, want %v", err, test.wantErr)
			}
			if strings.Contains(fmt.Sprint(err), "cookie-secret") {
				t.Fatalf("InitRapidUpload() exposed provider details: %v", err)
			}
		})
	}

	t.Run("transport", func(t *testing.T) {
		calls := atomic.Int32{}
		adapter, err := newCookieHTTPAdapter(httpDoerFunc(func(request *http.Request) (*http.Response, error) {
			if calls.Add(1) == 1 {
				return jsonHTTPResponse(http.StatusOK, `{"state":true,"user_id":12345,"userkey":"fixture-user-key"}`), nil
			}
			return nil, &url.Error{Op: "Post", URL: "https://uplb.115.com/4.0/initupload.php?secret=cookie-secret", Err: errors.New("dial failed")}
		}), "https://example.invalid/app/uploadinfo", "https://example.invalid/files/shasearch", "https://example.invalid/4.0/initupload.php")
		if err != nil {
			t.Fatalf("newCookieHTTPAdapter() error = %v", err)
		}
		adapter.now = func() time.Time { return time.Unix(fixtureUploadTimestamp, 0) }
		_, err = adapter.InitRapidUpload(context.Background(), fixtureCredential(), fixtureRapidUploadRequest())
		if !errors.Is(err, ErrProviderUnavailable) {
			t.Fatalf("InitRapidUpload() error = %v, want ErrProviderUnavailable", err)
		}
		if strings.Contains(fmt.Sprint(err), "cookie-secret") || strings.Contains(fmt.Sprint(err), "uplb.115.com") {
			t.Fatalf("InitRapidUpload() transport error exposed details: %v", err)
		}
	})
}

func fixtureRapidUploadRequest() RapidUploadRequest {
	return RapidUploadRequest{
		FileName:       fixtureUploadFileName,
		SHA1:           fixtureSHA1,
		Size:           1024,
		TargetParentID: fixtureUploadParentID,
		PreID:          fixtureUploadPreID,
		SignKey:        "fixture-sign-key",
		SignValue:      fixtureUploadSignValue,
	}
}

func newTestRapidUploadAdapter(t *testing.T, server *httptest.Server) *CookieHTTPAdapter {
	t.Helper()
	adapter, err := newCookieHTTPAdapter(
		server.Client(),
		server.URL+"/app/uploadinfo",
		server.URL+"/files/shasearch",
		server.URL+"/4.0/initupload.php",
	)
	if err != nil {
		t.Fatalf("newCookieHTTPAdapter() error = %v", err)
	}
	adapter.now = func() time.Time { return time.Unix(fixtureUploadTimestamp, 0) }
	return adapter
}

func newRapidUploadServer(t *testing.T, response string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/app/uploadinfo":
			_, _ = w.Write([]byte(`{"state":true,"user_id":12345,"userkey":"fixture-user-key"}`))
		case "/4.0/initupload.php":
			writeEncryptedUploadResponse(t, w, response)
		default:
			t.Fatalf("unexpected request path: %s", request.URL.Path)
		}
	}))
}

func assertRapidUploadHTTPRequest(t *testing.T, request *http.Request) {
	t.Helper()
	if request.Method != http.MethodPost {
		t.Fatalf("upload method = %s, want POST", request.Method)
	}
	if request.Header.Get("Cookie") != fixtureCookie || request.Header.Get("Accept") != "*/*" {
		t.Fatalf("unexpected upload credential headers")
	}
	if request.Header.Get("Content-Type") != "application/x-www-form-urlencoded" {
		t.Fatalf("upload content type = %q", request.Header.Get("Content-Type"))
	}
	if request.Header.Get("User-Agent") != cookieUploadUserAgent(cookieUploadAppVersion) {
		t.Fatalf("upload User-Agent = %q", request.Header.Get("User-Agent"))
	}
	if request.Header.Get("Authorization") != "" {
		t.Fatalf("unexpected Authorization header")
	}
	decoded, err := p115cipher.DecodeToken(request.URL.Query().Get("k_ec"))
	if err != nil || decoded.Timestamp != fixtureUploadTimestamp {
		t.Fatalf("upload k_ec timestamp = %+v, err=%v", decoded, err)
	}
}

func writeEncryptedUploadResponse(t *testing.T, writer http.ResponseWriter, payload string) {
	t.Helper()
	_, _ = writer.Write(encryptedUploadResponse(t, payload))
}

func encryptedUploadResponse(t *testing.T, payload string) []byte {
	t.Helper()
	block := literalLZ4Block([]byte(payload))
	if len(block) > int(^uint16(0)) {
		t.Fatal("fixture LZ4 block is too large")
	}
	framed := make([]byte, 2, len(block)+2)
	binary.LittleEndian.PutUint16(framed, uint16(len(block)))
	framed = append(framed, block...)
	ciphertext, err := p115cipher.EncryptRequest(framed)
	if err != nil {
		t.Fatalf("encrypt upload response fixture: %v", err)
	}
	return ciphertext
}

func literalLZ4Block(payload []byte) []byte {
	block := make([]byte, 0, len(payload)+8)
	if len(payload) < 15 {
		block = append(block, byte(len(payload)<<4))
	} else {
		block = append(block, 0xf0)
		remaining := len(payload) - 15
		for remaining >= 255 {
			block = append(block, 255)
			remaining -= 255
		}
		block = append(block, byte(remaining))
	}
	return append(block, payload...)
}

func jsonHTTPResponse(statusCode int, body string) *http.Response {
	return &http.Response{
		StatusCode: statusCode,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}
