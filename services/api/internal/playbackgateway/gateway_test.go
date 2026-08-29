package playbackgateway

import (
	"bytes"
	"compress/flate"
	"compress/gzip"
	"compress/zlib"
	"context"
	"errors"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/konghang/ember/backend/internal/models"
	"github.com/konghang/ember/backend/internal/services/embytoken"
)

const (
	fixtureAccessToken               = "fixture-access-token"
	fixturePassword                  = "fixture-password"
	fixtureApplicationAuthorization  = `Emby UserId="", Client="Infuse", Device="iPhone", DeviceId="device-1", Version="8.0", Token=""`
	fixtureMediaBrowserAuthorization = `MediaBrowser UserId="", Client="Infuse", Device="iPhone", DeviceId="device-1", Version="8.5", Token=""`
)

func TestGatewaySystemInfoPublicIsTransparentWithoutLocalAuthentication(t *testing.T) {
	responseBody := `{"LocalAddresses":[],"RemoteAddresses":[],"ServerName":"Fixture","Version":"4.9.3.0","Id":"server-1"}`
	tests := []struct {
		name              string
		target            string
		pathMode          string
		applicationHeader string
	}{
		{name: "root path", target: "/System/Info/Public?fixture=keep", pathMode: "root"},
		{name: "emby prefixed path", target: "/emby/System/Info/Public?fixture=keep", pathMode: "emby_prefixed"},
		{name: "opaque application header", target: "/System/Info/Public?fixture=keep", pathMode: "root", applicationHeader: `MediaBrowser Client="Infuse"`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var upstreamCalls atomic.Int32
			upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				upstreamCalls.Add(1)
				if request.Method != http.MethodGet || request.URL.Path != publicSystemInfoPath || request.URL.RawQuery != "fixture=keep" {
					t.Errorf("upstream request = %s %s", request.Method, request.URL.RequestURI())
				}
				if request.Header.Get("Authorization") != test.applicationHeader || request.Header.Get("X-Emby-Authorization") != "" || request.Header.Get("X-Fixture") != "preserved" {
					t.Errorf("upstream headers = %#v", request.Header)
				}
				writer.Header().Set("Content-Type", "application/json; charset=utf-8")
				writer.Header().Set("X-Upstream", "preserved")
				_, _ = io.WriteString(writer, responseBody)
			}))
			defer upstream.Close()

			tokenService := &fakeTokenService{}
			var logs bytes.Buffer
			gateway := newTestGateway(t, upstream.URL, tokenService, &logs)
			request := httptest.NewRequest(http.MethodGet, test.target, nil)
			request.Header.Set("X-Fixture", "preserved")
			if test.applicationHeader != "" {
				request.Header.Set("Authorization", test.applicationHeader)
			}
			response := httptest.NewRecorder()
			gateway.ServeHTTP(response, request)

			if response.Code != http.StatusOK || response.Body.String() != responseBody || response.Header().Get("X-Upstream") != "preserved" {
				t.Fatalf("response = status %d headers=%v body=%q", response.Code, response.Header(), response.Body.String())
			}
			if upstreamCalls.Load() != 1 {
				t.Fatalf("upstream calls = %d, want 1", upstreamCalls.Load())
			}
			recorded, resolved := tokenService.snapshot()
			if len(recorded) != 0 || len(resolved) != 0 {
				t.Fatalf("token calls = recorded %d resolved %d, want none", len(recorded), len(resolved))
			}
			for _, expected := range []string{"code=bootstrap_upstream_response", "route=system_info_public", "pathMode=" + test.pathMode, "statusCode=200"} {
				if !strings.Contains(logs.String(), expected) {
					t.Fatalf("logs = %q, want %s", logs.String(), expected)
				}
			}
			assertSecretsAbsent(t, logs.String(), responseBody)
		})
	}
}

func TestGatewaySystemInfoPublicPreservesUpstreamFailure(t *testing.T) {
	for _, status := range []int{http.StatusUnauthorized, http.StatusForbidden, http.StatusInternalServerError} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			responseBody := `{"status":` + strconv.Itoa(status) + `}`
			upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				if request.URL.Path != publicSystemInfoPath {
					t.Errorf("upstream path = %s", request.URL.Path)
				}
				writer.Header().Set("X-Upstream", "preserved")
				writer.WriteHeader(status)
				_, _ = io.WriteString(writer, responseBody)
			}))
			defer upstream.Close()

			var logs bytes.Buffer
			gateway := newTestGateway(t, upstream.URL, &fakeTokenService{}, &logs)
			response := httptest.NewRecorder()
			gateway.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/System/Info/Public", nil))

			if response.Code != status || response.Body.String() != responseBody || response.Header().Get("X-Upstream") != "preserved" {
				t.Fatalf("response = status %d headers=%v body=%q", response.Code, response.Header(), response.Body.String())
			}
			if !strings.Contains(logs.String(), "code=bootstrap_upstream_response") || !strings.Contains(logs.String(), "statusCode="+strconv.Itoa(status)) {
				t.Fatalf("logs = %q", logs.String())
			}
			assertSecretsAbsent(t, logs.String(), responseBody)
		})
	}
}

func TestGatewayRootAuthenticationIsTransparentAndRecordsMapping(t *testing.T) {
	responseBody := `{"User":{"Id":"emby-user-1"},"AccessToken":"` + fixtureAccessToken + `","ServerId":"server-1"}`
	requestBody := `{"Username":"user","Pw":"` + fixturePassword + `"}`
	upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		body, err := io.ReadAll(request.Body)
		if err != nil {
			t.Fatalf("read upstream request body: %v", err)
		}
		if request.Method != http.MethodPost || request.URL.Path != authenticationPath || request.URL.RawQuery != "fixture=keep" {
			t.Errorf("upstream request = %s %s", request.Method, request.URL.RequestURI())
		}
		if string(body) != requestBody {
			t.Errorf("upstream body = %q, want %q", string(body), requestBody)
		}
		if request.Header.Get("X-Emby-Authorization") != fixtureMediaBrowserAuthorization {
			t.Error("MediaBrowser application authorization header changed")
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(writer, responseBody)
	}))
	defer upstream.Close()

	tokenService := &fakeTokenService{}
	var logs bytes.Buffer
	gateway := newTestGateway(t, upstream.URL, tokenService, &logs)
	request := httptest.NewRequest(http.MethodPost, "/Users/AuthenticateByName?fixture=keep", strings.NewReader(requestBody))
	request.Header.Set("X-Emby-Authorization", fixtureMediaBrowserAuthorization)
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	gateway.ServeHTTP(response, request)

	if response.Code != http.StatusOK || response.Body.String() != responseBody {
		t.Fatalf("response = status %d body=%q", response.Code, response.Body.String())
	}
	recorded, resolved := tokenService.snapshot()
	if len(recorded) != 1 || len(resolved) != 0 || recorded[0].AccessToken != fixtureAccessToken ||
		recorded[0].EmbyUserID != "emby-user-1" || recorded[0].ServerID != "server-1" {
		t.Fatalf("token calls = recorded %+v resolved %#v", recorded, resolved)
	}
	assertSecretsAbsent(t, logs.String(), fixtureAccessToken, fixturePassword, requestBody, responseBody)
}

func TestGatewayAuthenticationDeflateResponseIsTransparentAndRecordsMapping(t *testing.T) {
	responseBody := `{"User":{"Id":"emby-user-1"},"AccessToken":"` + fixtureAccessToken + `","ServerId":"server-1","Unknown":true}`
	for _, mode := range []string{"zlib", "raw"} {
		t.Run(mode, func(t *testing.T) {
			compressed := deflateFixture(t, []byte(responseBody), mode == "raw")
			upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				if request.URL.Path != authenticationPath || request.Header.Get("X-Emby-Authorization") != fixtureMediaBrowserAuthorization {
					t.Errorf("upstream request = %s headers=%#v", request.URL.RequestURI(), request.Header)
				}
				writer.Header().Set("Content-Type", "application/json; charset=utf-8")
				writer.Header().Set("Content-Encoding", "deflate")
				writer.Header().Set("X-Upstream", "preserved")
				_, _ = writer.Write(compressed)
			}))
			defer upstream.Close()

			tokenService := &fakeTokenService{}
			var logs bytes.Buffer
			gateway := newTestGateway(t, upstream.URL, tokenService, &logs)
			request := httptest.NewRequest(http.MethodPost, "/Users/AuthenticateByName", strings.NewReader(`{"Username":"user","Pw":"fixture"}`))
			request.Header.Set("X-Emby-Authorization", fixtureMediaBrowserAuthorization)
			response := httptest.NewRecorder()
			gateway.ServeHTTP(response, request)

			if response.Code != http.StatusOK || !bytes.Equal(response.Body.Bytes(), compressed) || response.Header().Get("Content-Encoding") != "deflate" || response.Header().Get("X-Upstream") != "preserved" {
				t.Fatalf("response = status %d headers=%v bodyLength=%d, want original deflate response", response.Code, response.Header(), response.Body.Len())
			}
			recorded, resolved := tokenService.snapshot()
			if len(recorded) != 1 || len(resolved) != 0 || recorded[0].AccessToken != fixtureAccessToken || recorded[0].DeviceID != "device-1" || recorded[0].ClientName != "Infuse" {
				t.Fatalf("token calls = recorded %+v resolved %#v", recorded, resolved)
			}
			assertSecretsAbsent(t, logs.String(), fixtureAccessToken, responseBody)
		})
	}
}

func TestGatewayAuthenticationGzipResponseIsTransparentAndRecordsMapping(t *testing.T) {
	responseBody := `{"User":{"Id":"emby-user-1"},"AccessToken":"` + fixtureAccessToken + `","ServerId":"server-1","Unknown":true}`
	compressed := gzipFixture(t, []byte(responseBody))
	upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != authenticationPath || request.Header.Get("X-Emby-Authorization") != fixtureMediaBrowserAuthorization {
			t.Errorf("upstream request = %s headers=%#v", request.URL.RequestURI(), request.Header)
		}
		writer.Header().Set("Content-Type", "application/json; charset=utf-8")
		writer.Header().Set("Content-Encoding", "gzip")
		writer.Header().Set("X-Upstream", "preserved")
		_, _ = writer.Write(compressed)
	}))
	defer upstream.Close()

	tokenService := &fakeTokenService{}
	var logs bytes.Buffer
	gateway := newTestGateway(t, upstream.URL, tokenService, &logs)
	request := httptest.NewRequest(http.MethodPost, "/Users/AuthenticateByName", strings.NewReader(`{"Username":"user","Pw":"fixture"}`))
	request.Header.Set("X-Emby-Authorization", fixtureMediaBrowserAuthorization)
	request.Header.Set("Accept-Encoding", "gzip")
	response := httptest.NewRecorder()
	gateway.ServeHTTP(response, request)

	if response.Code != http.StatusOK || !bytes.Equal(response.Body.Bytes(), compressed) || response.Header().Get("Content-Encoding") != "gzip" || response.Header().Get("X-Upstream") != "preserved" {
		t.Fatalf("response = status %d headers=%v bodyLength=%d, want original gzip response", response.Code, response.Header(), response.Body.Len())
	}
	recorded, resolved := tokenService.snapshot()
	if len(recorded) != 1 || len(resolved) != 0 || recorded[0].AccessToken != fixtureAccessToken || recorded[0].DeviceID != "device-1" || recorded[0].ClientName != "Infuse" {
		t.Fatalf("token calls = recorded %+v resolved %#v", recorded, resolved)
	}
	assertSecretsAbsent(t, logs.String(), fixtureAccessToken, responseBody)
}

func TestGatewayAuthenticationEncodedSidecarFailuresRemainTransparent(t *testing.T) {
	oversizedJSON := []byte(`{"User":{"Id":"emby-user-1"},"AccessToken":"` + fixtureAccessToken + `","ServerId":"server-1","Padding":"` + strings.Repeat("x", 4096) + `"}`)
	tests := []struct {
		name            string
		contentEncoding string
		body            []byte
		responseMax     int64
		wantEncoding    string
		wantReason      string
	}{
		{
			name: "invalid deflate", contentEncoding: "deflate",
			body: []byte("invalid-deflate-" + fixtureAccessToken), wantEncoding: "deflate", wantReason: "decode_failed",
		},
		{
			name: "invalid gzip", contentEncoding: "gzip",
			body: []byte("invalid-gzip-" + fixtureAccessToken), wantEncoding: "gzip", wantReason: "decode_failed",
		},
		{
			name: "decoded body too large", contentEncoding: "deflate",
			body: deflateFixture(t, oversizedJSON, false), responseMax: 512, wantEncoding: "deflate", wantReason: "decoded_body_too_large",
		},
		{
			name: "gzip decoded body too large", contentEncoding: "gzip",
			body: gzipFixture(t, oversizedJSON), responseMax: 512, wantEncoding: "gzip", wantReason: "decoded_body_too_large",
		},
		{
			name: "unsupported encoding", contentEncoding: "br",
			body: []byte(`{"AccessToken":"` + fixtureAccessToken + `"}`), wantEncoding: "unsupported", wantReason: "encoding_unsupported",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
				writer.Header().Set("Content-Encoding", test.contentEncoding)
				writer.WriteHeader(http.StatusOK)
				_, _ = writer.Write(test.body)
			}))
			defer upstream.Close()

			tokenService := &fakeTokenService{}
			var logs bytes.Buffer
			gateway := newTestGateway(t, upstream.URL, tokenService, &logs)
			if test.responseMax > 0 {
				gateway.maxAuthenticationResponseBytes = test.responseMax
			}
			request := newAuthenticationRequest(`{"Username":"user","Pw":"fixture"}`)
			if test.contentEncoding == "gzip" {
				request.Header.Set("Accept-Encoding", "gzip")
			}
			response := httptest.NewRecorder()
			gateway.ServeHTTP(response, request)

			if response.Code != http.StatusOK || !bytes.Equal(response.Body.Bytes(), test.body) || response.Header().Get("Content-Encoding") != test.contentEncoding {
				t.Fatalf("response = status %d headers=%v bodyLength=%d, want original encoded response", response.Code, response.Header(), response.Body.Len())
			}
			recorded, _ := tokenService.snapshot()
			if len(recorded) != 0 {
				t.Fatalf("recorded mappings = %+v, want none", recorded)
			}
			for _, expected := range []string{"code=authentication_response_decode_failed", "contentEncoding=" + test.wantEncoding, "reasonCode=" + test.wantReason} {
				if !strings.Contains(logs.String(), expected) {
					t.Fatalf("logs = %q, want %s", logs.String(), expected)
				}
			}
			assertSecretsAbsent(t, logs.String(), fixtureAccessToken, string(test.body))
		})
	}
}

func TestGatewayRootProtectedAPIIsCanonicalizedAfterTokenGate(t *testing.T) {
	requestBody := `{"ItemId":"item-1","PositionTicks":123456789}`
	var upstreamCalls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		upstreamCalls.Add(1)
		body, err := io.ReadAll(request.Body)
		if err != nil {
			t.Fatalf("read upstream request body: %v", err)
		}
		if request.Method != http.MethodPost || request.URL.Path != "/emby/Sessions/Playing/Progress" || request.URL.RawQuery != "fixture=keep" {
			t.Errorf("upstream request = %s %s", request.Method, request.URL.RequestURI())
		}
		if request.Header.Get(accessTokenHeader) != fixtureAccessToken || string(body) != requestBody {
			t.Errorf("upstream token/body changed")
		}
		writer.Header().Set("X-Upstream", "preserved")
		writer.WriteHeader(http.StatusNoContent)
	}))
	defer upstream.Close()

	tokenService := &fakeTokenService{principal: fixturePrincipal()}
	var logs bytes.Buffer
	gateway := newTestGateway(t, upstream.URL, tokenService, &logs)
	request := httptest.NewRequest(http.MethodPost, "/Sessions/Playing/Progress?fixture=keep", strings.NewReader(requestBody))
	request.Header.Set(accessTokenHeader, fixtureAccessToken)
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	gateway.ServeHTTP(response, request)

	if response.Code != http.StatusNoContent || response.Header().Get("X-Upstream") != "preserved" || upstreamCalls.Load() != 1 {
		t.Fatalf("response=%d headers=%v upstreamCalls=%d", response.Code, response.Header(), upstreamCalls.Load())
	}
	_, resolved := tokenService.snapshot()
	if len(resolved) != 1 || resolved[0] != fixtureAccessToken {
		t.Fatalf("resolved tokens = %#v", resolved)
	}
	assertSecretsAbsent(t, logs.String(), fixtureAccessToken, requestBody)
}

func TestGatewayRejectsDuplicateEmbyPrefixBeforeProxy(t *testing.T) {
	var upstreamCalls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		upstreamCalls.Add(1)
		writer.WriteHeader(http.StatusNoContent)
	}))
	defer upstream.Close()

	tokenService := &fakeTokenService{principal: fixturePrincipal()}
	var logs bytes.Buffer
	gateway := newTestGateway(t, upstream.URL, tokenService, &logs)
	request := httptest.NewRequest(http.MethodGet, "/emby/emby/System/Info", nil)
	request.Header.Set(accessTokenHeader, fixtureAccessToken)
	response := httptest.NewRecorder()
	gateway.ServeHTTP(response, request)

	if response.Code != http.StatusBadRequest || upstreamCalls.Load() != 0 {
		t.Fatalf("response=%d upstreamCalls=%d, want 400 and no upstream", response.Code, upstreamCalls.Load())
	}
	if !strings.Contains(logs.String(), "code=request_path_invalid") {
		t.Fatalf("logs = %q, want request_path_invalid", logs.String())
	}
	assertSecretsAbsent(t, logs.String(), fixtureAccessToken)
}

func TestGatewayAuthenticationResponseIsTransparentAndRecordsMapping(t *testing.T) {
	responseBody := "{\n  \"Unknown\": {\"keep\": true},\n  \"User\": {\"Id\": \"emby-user-1\", \"Name\": \"user\"},\n  \"AccessToken\": \"" + fixtureAccessToken + "\",\n  \"ServerId\": \"server-1\"\n}\n"
	requestBody := "{\"Username\":\"user\",\"Pw\":\"" + fixturePassword + "\",\"Unknown\":true}"

	var upstreamRequestBody string
	upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		body, err := io.ReadAll(request.Body)
		if err != nil {
			t.Fatalf("read upstream request body: %v", err)
		}
		upstreamRequestBody = string(body)
		if request.Method != http.MethodPost || request.URL.Path != authenticationPath || request.URL.RawQuery != "fixture=keep" {
			t.Fatalf("upstream request = %s %s?%s", request.Method, request.URL.Path, request.URL.RawQuery)
		}
		if request.Header.Get("X-Fixture") != "preserved" {
			t.Fatalf("upstream fixture header = %q", request.Header.Get("X-Fixture"))
		}
		if request.Header.Get("Authorization") != fixtureApplicationAuthorization {
			t.Fatal("application authorization header changed")
		}
		writer.Header().Set("Content-Type", "application/json; charset=utf-8")
		writer.Header().Set("X-Upstream", "preserved")
		writer.Header().Add("Set-Cookie", "first=value; HttpOnly")
		writer.Header().Add("Set-Cookie", "second=value; Secure")
		writer.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(writer, responseBody)
	}))
	defer upstream.Close()

	tokenService := &fakeTokenService{}
	var logs bytes.Buffer
	gateway := newTestGateway(t, upstream.URL, tokenService, &logs)

	request := httptest.NewRequest(http.MethodPost, authenticationPath+"?fixture=keep", strings.NewReader(requestBody))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-Fixture", "preserved")
	request.Header.Set("Authorization", fixtureApplicationAuthorization)
	response := httptest.NewRecorder()
	gateway.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusOK)
	}
	if response.Body.String() != responseBody {
		t.Fatalf("body changed:\n got %q\nwant %q", response.Body.String(), responseBody)
	}
	if response.Header().Get("Content-Type") != "application/json; charset=utf-8" || response.Header().Get("X-Upstream") != "preserved" {
		t.Fatalf("response headers = %#v", response.Header())
	}
	if got := response.Header().Values("Set-Cookie"); len(got) != 2 || got[0] != "first=value; HttpOnly" || got[1] != "second=value; Secure" {
		t.Fatalf("Set-Cookie = %#v", got)
	}
	if upstreamRequestBody != requestBody {
		t.Fatalf("request body changed: got %q want %q", upstreamRequestBody, requestBody)
	}

	records, _ := tokenService.snapshot()
	if len(records) != 1 {
		t.Fatalf("record count = %d, want 1", len(records))
	}
	if got := records[0]; got.ServerID != "server-1" || got.EmbyUserID != "emby-user-1" ||
		got.AccessToken != fixtureAccessToken || got.DeviceID != "device-1" || got.ClientName != "Infuse" {
		t.Fatalf("record input = %+v", got)
	}
	assertSecretsAbsent(t, logs.String(), fixtureAccessToken, fixturePassword, requestBody, responseBody)
}

func TestGatewayDoesNotRecordUnsuccessfulAuthentication(t *testing.T) {
	for _, status := range []int{http.StatusUnauthorized, http.StatusForbidden, http.StatusInternalServerError} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			body := "{\"status\":" + http.StatusText(status) + ",\"echo\":\"" + fixturePassword + "\"}"
			upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
				writer.Header().Set("X-Upstream-Status", http.StatusText(status))
				writer.WriteHeader(status)
				_, _ = io.WriteString(writer, body)
			}))
			defer upstream.Close()

			tokenService := &fakeTokenService{}
			var logs bytes.Buffer
			gateway := newTestGateway(t, upstream.URL, tokenService, &logs)
			response := httptest.NewRecorder()
			gateway.ServeHTTP(response, newAuthenticationRequest("{}"))

			if response.Code != status || response.Body.String() != body || response.Header().Get("X-Upstream-Status") != http.StatusText(status) {
				t.Fatalf("response = status %d headers=%v body=%q", response.Code, response.Header(), response.Body.String())
			}
			records, _ := tokenService.snapshot()
			if len(records) != 0 {
				t.Fatalf("record count = %d, want 0", len(records))
			}
			assertSecretsAbsent(t, logs.String(), fixturePassword, body)
		})
	}
}

func TestGatewayAuthenticationSidecarFailuresNeverChangeSuccessResponse(t *testing.T) {
	validBody := "{\"User\":{\"Id\":\"emby-user-1\"},\"AccessToken\":\"" + fixtureAccessToken + "\",\"ServerId\":\"server-1\",\"Unknown\":1}"
	tests := []struct {
		name           string
		body           string
		recordErr      error
		limit          int64
		wantRecordCall bool
		wantLogCode    string
	}{
		{
			name:           "mapping write fails",
			body:           validBody,
			recordErr:      errors.New("store failed with " + fixtureAccessToken + " and " + fixturePassword),
			wantRecordCall: true,
			wantLogCode:    "authentication_mapping_failed",
		},
		{
			name:        "success body is not json",
			body:        "not-json " + fixtureAccessToken,
			wantLogCode: "authentication_response_invalid",
		},
		{
			name:        "success body exceeds inspection limit",
			body:        validBody + strings.Repeat("x", 128),
			limit:       32,
			wantLogCode: "authentication_response_too_large",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
				writer.Header().Set("Content-Type", "application/json")
				writer.WriteHeader(http.StatusOK)
				_, _ = io.WriteString(writer, test.body)
			}))
			defer upstream.Close()

			tokenService := &fakeTokenService{recordErr: test.recordErr}
			var logs bytes.Buffer
			gateway := newTestGateway(t, upstream.URL, tokenService, &logs)
			if test.limit > 0 {
				gateway.maxAuthenticationResponseBytes = test.limit
			}
			response := httptest.NewRecorder()
			gateway.ServeHTTP(response, newAuthenticationRequest("{}"))

			if response.Code != http.StatusOK || response.Body.String() != test.body {
				t.Fatalf("response changed: status=%d body=%q", response.Code, response.Body.String())
			}
			records, _ := tokenService.snapshot()
			if (len(records) == 1) != test.wantRecordCall {
				t.Fatalf("record count = %d, wantRecordCall=%t", len(records), test.wantRecordCall)
			}
			if !strings.Contains(logs.String(), "code="+test.wantLogCode) {
				t.Fatalf("logs = %q, want code=%s", logs.String(), test.wantLogCode)
			}
			assertSecretsAbsent(t, logs.String(), fixtureAccessToken, fixturePassword, test.body)
		})
	}
}

func TestGatewayGetPlaybackInfoIsTransparentAndRecordsProofs(t *testing.T) {
	responseBody := "{\n  \"MediaSources\": [\n" +
		"    {\"Id\":\"source-1\",\"ItemId\":\"item-1\",\"Path\":\"/private/media/one.mkv\",\"Size\":1024,\"Container\":\"mkv\",\"SupportsDirectPlay\":true,\"SupportsDirectStream\":true},\n" +
		"    {\"Id\":\"source-2\",\"Path\":\"/private/media/two.mp4\",\"Size\":2048,\"Container\":\"mp4\",\"SupportsDirectPlay\":true,\"SupportsTranscoding\":true}\n" +
		"  ],\n  \"PlaySessionId\": \"session-1\",\n  \"Unknown\": true\n}\n"
	var upstreamCalls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		upstreamCalls.Add(1)
		if request.Method != http.MethodGet || request.URL.Path != "/emby/Items/item-1/PlaybackInfo" || request.URL.Query().Get("UserId") != "emby-user-1" {
			t.Errorf("upstream request = %s %s", request.Method, request.URL.RequestURI())
		}
		writer.Header().Set("Content-Type", "application/json; charset=utf-8")
		writer.Header().Set("X-Upstream", "preserved")
		_, _ = io.WriteString(writer, responseBody)
	}))
	defer upstream.Close()

	tokenService := &fakeTokenService{principal: fixturePrincipal()}
	var logs bytes.Buffer
	gateway := newTestGateway(t, upstream.URL, tokenService, &logs)
	request := httptest.NewRequest(http.MethodGet, "/emby/Items/item-1/PlaybackInfo?UserId=emby-user-1", nil)
	request.Header.Set(accessTokenHeader, fixtureAccessToken)
	response := httptest.NewRecorder()
	gateway.ServeHTTP(response, request)

	if response.Code != http.StatusOK || response.Body.String() != responseBody || response.Header().Get("X-Upstream") != "preserved" {
		t.Fatalf("response = status %d headers=%v body=%q", response.Code, response.Header(), response.Body.String())
	}
	if upstreamCalls.Load() != 1 {
		t.Fatalf("upstream calls = %d, want 1", upstreamCalls.Load())
	}
	for _, expected := range []struct {
		id   string
		path string
		size int64
	}{
		{id: "source-1", path: "/private/media/one.mkv", size: 1024},
		{id: "source-2", path: "/private/media/two.mp4", size: 2048},
	} {
		proof, ok := gateway.LookupPlaybackProof(fixturePrincipal(), "item-1", expected.id, "session-1")
		if !ok || proof.Path != expected.path || proof.Size != expected.size || proof.UserID != "user-1" || proof.EmbyUserID != "emby-user-1" {
			t.Fatalf("proof %s = (%+v, %t)", expected.id, proof, ok)
		}
	}
	wrongPrincipal := fixturePrincipal()
	wrongPrincipal.User.ID = "other-user"
	if _, ok := gateway.LookupPlaybackProof(wrongPrincipal, "item-1", "source-1", "session-1"); ok {
		t.Fatal("proof was reusable by a different principal")
	}
	for _, expected := range []string{
		`code=playback_info_media_source_observed mappingId="mapping-1" itemId="item-1" mediaSourceId="source-1" mediaPath="/private/media/one.mkv"`,
		`code=playback_info_media_source_observed mappingId="mapping-1" itemId="item-1" mediaSourceId="source-2" mediaPath="/private/media/two.mp4"`,
	} {
		if !strings.Contains(logs.String(), expected) {
			t.Fatalf("logs=%q, want %s", logs.String(), expected)
		}
	}
	assertSecretsAbsent(t, logs.String(), fixtureAccessToken, responseBody)
}

func TestPlaybackInfoMediaPathLogQuotesControlCharacters(t *testing.T) {
	var logs bytes.Buffer
	gateway := &Gateway{logger: log.New(&logs, "", 0)}
	gateway.logPlaybackInfoMediaSourceObservations("mapping-1", "item-1", []playbackInfoMediaSourceObservation{{
		MediaSourceID: "source-1", MediaPath: "/mnt/media/line\nbreak.mkv", PathPresent: true,
		ProofAccepted: true, ProofRejectReason: "none",
	}})

	if strings.Count(logs.String(), "\n") != 1 || !strings.Contains(logs.String(), `mediaPath="/mnt/media/line\nbreak.mkv"`) {
		t.Fatalf("logs=%q, want one quoted log line", logs.String())
	}
}

func TestGatewayPostPlaybackInfoPreservesRequestAndRecordsProof(t *testing.T) {
	requestBody := `{"UserId":"emby-user-1","MediaSourceId":"source-1","Unknown":true}`
	responseBody := `{"MediaSources":[{"Id":"source-1","ItemId":"item-1","Path":"/private/media/one.mkv","Size":1024,"Container":"mkv","SupportsDirectPlay":true}],"PlaySessionId":"session-1"}`
	var upstreamBody string
	upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		body, _ := io.ReadAll(request.Body)
		upstreamBody = string(body)
		writer.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(writer, responseBody)
	}))
	defer upstream.Close()
	tokenService := &fakeTokenService{principal: fixturePrincipal()}
	var logs bytes.Buffer
	gateway := newTestGateway(t, upstream.URL, tokenService, &logs)
	request := httptest.NewRequest(http.MethodPost, "/emby/Items/item-1/PlaybackInfo", strings.NewReader(requestBody))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set(accessTokenHeader, fixtureAccessToken)
	response := httptest.NewRecorder()
	gateway.ServeHTTP(response, request)
	if response.Code != http.StatusOK || response.Body.String() != responseBody || upstreamBody != requestBody {
		t.Fatalf("response=%d body=%q upstreamBody=%q", response.Code, response.Body.String(), upstreamBody)
	}
	if _, ok := gateway.LookupPlaybackProof(fixturePrincipal(), "item-1", "source-1", "session-1"); !ok {
		t.Fatal("POST PlaybackInfo did not record proof")
	}
}

func TestGatewayRootPlaybackInfoReusesProofObserver(t *testing.T) {
	responseBody := `{"MediaSources":[{"Id":"source-1","ItemId":"item-1","Path":"/private/media/one.mkv","Size":1024,"Container":"mkv","SupportsDirectPlay":true}],"PlaySessionId":"session-1"}`
	upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet || request.URL.Path != "/emby/Items/item-1/PlaybackInfo" || request.URL.Query().Get("UserId") != "emby-user-1" {
			t.Errorf("upstream request = %s %s", request.Method, request.URL.RequestURI())
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(writer, responseBody)
	}))
	defer upstream.Close()

	tokenService := &fakeTokenService{principal: fixturePrincipal()}
	var logs bytes.Buffer
	gateway := newTestGateway(t, upstream.URL, tokenService, &logs)
	request := httptest.NewRequest(http.MethodGet, "/Items/item-1/PlaybackInfo?UserId=emby-user-1", nil)
	request.Header.Set(accessTokenHeader, fixtureAccessToken)
	response := httptest.NewRecorder()
	gateway.ServeHTTP(response, request)

	if response.Code != http.StatusOK || response.Body.String() != responseBody {
		t.Fatalf("response = status %d body=%q", response.Code, response.Body.String())
	}
	if _, ok := gateway.LookupPlaybackProof(fixturePrincipal(), "item-1", "source-1", "session-1"); !ok {
		t.Fatal("root PlaybackInfo did not record proof")
	}
	if !strings.Contains(logs.String(), `code=playback_info_media_source_observed mappingId="mapping-1" itemId="item-1" mediaSourceId="source-1" mediaPath="/private/media/one.mkv"`) {
		t.Fatalf("logs=%q, want complete media path", logs.String())
	}
	assertSecretsAbsent(t, logs.String(), fixtureAccessToken, responseBody)
}

func TestGatewayLowercasePlaybackInfoQueryPreservesYambyEmptyMediaStreams(t *testing.T) {
	responseBody := `{"MediaSources":[{"Id":"source-1","ItemId":"item-1","Path":"/private/media/one.mkv","Size":1024,"Container":"mkv","SupportsDirectPlay":true,"MediaStreams":[]}],"PlaySessionId":"session-1"}`
	upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet || request.URL.Path != "/emby/items/item-1/playbackinfo" ||
			request.URL.Query().Get("userid") != "emby-user-1" || request.URL.Query().Get("api_KEY") != fixtureAccessToken {
			t.Fatalf("upstream request = %s %s", request.Method, request.URL.RequestURI())
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(writer, responseBody)
	}))
	defer upstream.Close()

	tokenService := &fakeTokenService{principal: fixturePrincipal()}
	var logs bytes.Buffer
	gateway := newTestGateway(t, upstream.URL, tokenService, &logs)
	request := httptest.NewRequest(
		http.MethodGet,
		"/items/item-1/playbackinfo?userid=emby-user-1&api_KEY="+fixtureAccessToken,
		nil,
	)
	request.Header.Set("User-Agent", "Yamby/1.2.0")
	response := httptest.NewRecorder()
	gateway.ServeHTTP(response, request)

	if response.Code != http.StatusOK || response.Body.String() != responseBody {
		t.Fatalf("response=%d body=%q", response.Code, response.Body.String())
	}
	if _, ok := gateway.LookupPlaybackProof(fixturePrincipal(), "item-1", "source-1", "session-1"); !ok {
		t.Fatal("lowercase PlaybackInfo did not record proof")
	}
	for _, expected := range []string{"route=playback_info", "queryTokenState=present", "userAgentFamily=yamby"} {
		if !strings.Contains(logs.String(), expected) {
			t.Fatalf("logs=%q, want %s", logs.String(), expected)
		}
	}
	if !strings.Contains(logs.String(), `mediaPath="/private/media/one.mkv"`) {
		t.Fatalf("logs=%q, want complete media path", logs.String())
	}
	assertSecretsAbsent(t, logs.String(), fixtureAccessToken)
}

func TestGatewayEncodedPlaybackInfoPreservesBytesAndRecordsProof(t *testing.T) {
	responseBody := []byte(`{"MediaSources":[{"Id":"source-1","ItemId":"item-1","Path":"/private/media/one.mkv","Size":1024,"Container":"mkv","SupportsDirectPlay":true,"MediaStreams":[]}],"PlaySessionId":"session-1"}`)
	tests := []struct {
		name     string
		encoding string
		encode   func(*testing.T, []byte) []byte
	}{
		{name: "gzip", encoding: "gzip", encode: gzipFixture},
		{name: "deflate", encoding: "deflate", encode: func(t *testing.T, body []byte) []byte { return deflateFixture(t, body, false) }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			encodedBody := test.encode(t, responseBody)
			upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
				writer.Header().Set("Content-Type", "application/json")
				writer.Header().Set("Content-Encoding", test.encoding)
				_, _ = writer.Write(encodedBody)
			}))
			defer upstream.Close()

			tokenService := &fakeTokenService{principal: fixturePrincipal()}
			var logs bytes.Buffer
			gateway := newTestGateway(t, upstream.URL, tokenService, &logs)
			request := httptest.NewRequest(http.MethodGet, "/Items/item-1/PlaybackInfo?UserId=emby-user-1", nil)
			request.Header.Set(accessTokenHeader, fixtureAccessToken)
			request.Header.Set("Accept-Encoding", test.encoding)
			response := httptest.NewRecorder()
			gateway.ServeHTTP(response, request)

			if response.Code != http.StatusOK || !bytes.Equal(response.Body.Bytes(), encodedBody) || response.Header().Get("Content-Encoding") != test.encoding {
				t.Fatalf("response=%d encoding=%q bodyLength=%d", response.Code, response.Header().Get("Content-Encoding"), response.Body.Len())
			}
			if _, ok := gateway.LookupPlaybackProof(fixturePrincipal(), "item-1", "source-1", "session-1"); !ok {
				t.Fatal("encoded PlaybackInfo did not record proof")
			}
			if !strings.Contains(logs.String(), `mediaPath="/private/media/one.mkv"`) {
				t.Fatalf("logs=%q, want complete media path", logs.String())
			}
			assertSecretsAbsent(t, logs.String(), fixtureAccessToken, string(responseBody))
		})
	}
}

func TestGatewayPlaybackInfoIneligibleRequestRemainsTransparentWithoutProof(t *testing.T) {
	responseBody := `{"MediaSources":[{"Id":"source-1","ItemId":"item-1","Path":"/private/media/one.mkv","Size":1024,"SupportsDirectPlay":true}],"PlaySessionId":"session-1"}`
	tests := []struct {
		name        string
		method      string
		target      string
		body        string
		contentType string
		requestMax  int64
	}{
		{name: "GET missing UserId", method: http.MethodGet, target: "/emby/Items/item-1/PlaybackInfo"},
		{name: "GET mismatched UserId", method: http.MethodGet, target: "/emby/Items/item-1/PlaybackInfo?UserId=other"},
		{name: "POST mismatched UserId", method: http.MethodPost, target: "/emby/Items/item-1/PlaybackInfo", body: `{"UserId":"other"}`, contentType: "application/json"},
		{name: "POST invalid JSON", method: http.MethodPost, target: "/emby/Items/item-1/PlaybackInfo", body: `{`, contentType: "application/json"},
		{name: "POST oversized", method: http.MethodPost, target: "/emby/Items/item-1/PlaybackInfo", body: `{"UserId":"emby-user-1"}`, contentType: "application/json", requestMax: 8},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
				writer.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(writer, responseBody)
			}))
			defer upstream.Close()
			tokenService := &fakeTokenService{principal: fixturePrincipal()}
			var logs bytes.Buffer
			gateway := newTestGateway(t, upstream.URL, tokenService, &logs)
			if test.requestMax > 0 {
				gateway.maxPlaybackInfoRequestBytes = test.requestMax
			}
			request := httptest.NewRequest(test.method, test.target, strings.NewReader(test.body))
			request.Header.Set(accessTokenHeader, fixtureAccessToken)
			if test.contentType != "" {
				request.Header.Set("Content-Type", test.contentType)
			}
			response := httptest.NewRecorder()
			gateway.ServeHTTP(response, request)
			if response.Code != http.StatusOK || response.Body.String() != responseBody {
				t.Fatalf("response = status %d body=%q", response.Code, response.Body.String())
			}
			if _, ok := gateway.LookupPlaybackProof(fixturePrincipal(), "item-1", "source-1", "session-1"); ok {
				t.Fatal("ineligible request recorded proof")
			}
		})
	}
}

func TestGatewayPlaybackInfoInvalidResponseDoesNotRecordProof(t *testing.T) {
	validSource := `{"Id":"source-1","ItemId":"item-1","Path":"/private/media/one.mkv","Size":1024,"SupportsDirectPlay":true}`
	tests := []struct {
		name            string
		status          int
		body            string
		responseMax     int64
		wantObservation bool
	}{
		{name: "upstream rejected", status: http.StatusForbidden, body: `{"ErrorCode":"NotAllowed"}`},
		{name: "error code", status: http.StatusOK, body: `{"MediaSources":[` + validSource + `],"PlaySessionId":"session-1","ErrorCode":"NotAllowed"}`},
		{name: "missing play session", status: http.StatusOK, body: `{"MediaSources":[` + validSource + `]}`},
		{name: "duplicate media source", status: http.StatusOK, body: `{"MediaSources":[` + validSource + `,` + validSource + `],"PlaySessionId":"session-1"}`},
		{name: "not direct play", status: http.StatusOK, body: `{"MediaSources":[{"Id":"source-1","Path":"/private/media/one.mkv","Size":1024}],"PlaySessionId":"session-1"}`, wantObservation: true},
		{name: "oversized response", status: http.StatusOK, body: `{"MediaSources":[` + validSource + `],"PlaySessionId":"session-1","Padding":"` + strings.Repeat("x", 128) + `"}`, responseMax: 32},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
				writer.Header().Set("Content-Type", "application/json")
				writer.WriteHeader(test.status)
				_, _ = io.WriteString(writer, test.body)
			}))
			defer upstream.Close()
			tokenService := &fakeTokenService{principal: fixturePrincipal()}
			var logs bytes.Buffer
			gateway := newTestGateway(t, upstream.URL, tokenService, &logs)
			gateway.proofs.Record([]PlaybackProof{fixturePlaybackProof("mapping-1", "item-1", "source-1", "session-1")})
			if test.responseMax > 0 {
				gateway.maxPlaybackInfoResponseBytes = test.responseMax
			}
			request := httptest.NewRequest(http.MethodGet, "/emby/Items/item-1/PlaybackInfo?UserId=emby-user-1", nil)
			request.Header.Set(accessTokenHeader, fixtureAccessToken)
			response := httptest.NewRecorder()
			gateway.ServeHTTP(response, request)
			if response.Code != test.status || response.Body.String() != test.body {
				t.Fatalf("response = status %d body=%q", response.Code, response.Body.String())
			}
			if _, ok := gateway.LookupPlaybackProof(fixturePrincipal(), "item-1", "source-1", "session-1"); ok {
				t.Fatal("invalid response recorded proof")
			}
			if test.wantObservation {
				for _, expected := range []string{
					`code=playback_info_media_source_observed mappingId="mapping-1" itemId="item-1" mediaSourceId="source-1" mediaPath="/private/media/one.mkv"`,
					"proofAccepted=false", "proofRejectReason=direct_play_unsupported",
				} {
					if !strings.Contains(logs.String(), expected) {
						t.Fatalf("logs=%q, want %s", logs.String(), expected)
					}
				}
				assertSecretsAbsent(t, logs.String(), test.body)
			} else {
				assertSecretsAbsent(t, logs.String(), test.body, "/private/media/one.mkv")
			}
		})
	}
}

func TestGatewayProtectedRequestRequiresMappedTokenBeforeProxy(t *testing.T) {
	var upstreamCalls int
	upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		upstreamCalls++
		if request.Header.Get(accessTokenHeader) != fixtureAccessToken {
			t.Fatalf("upstream token header changed")
		}
		writer.Header().Set("X-Upstream", "called")
		writer.WriteHeader(http.StatusNoContent)
	}))
	defer upstream.Close()

	tokenService := &fakeTokenService{}
	var logs bytes.Buffer
	gateway := newTestGateway(t, upstream.URL, tokenService, &logs)
	request := httptest.NewRequest(http.MethodGet, "/emby/Items/fixture", nil)
	request.Header.Set(accessTokenHeader, fixtureAccessToken)
	response := httptest.NewRecorder()
	gateway.ServeHTTP(response, request)

	if response.Code != http.StatusNoContent || response.Header().Get("X-Upstream") != "called" || upstreamCalls != 1 {
		t.Fatalf("response=%d headers=%v upstreamCalls=%d", response.Code, response.Header(), upstreamCalls)
	}
	_, resolved := tokenService.snapshot()
	if len(resolved) != 1 || resolved[0] != fixtureAccessToken {
		t.Fatalf("resolved tokens = %#v", resolved)
	}
	assertSecretsAbsent(t, logs.String(), fixtureAccessToken)
}

func TestGatewayProtectedRequestAcceptsInfuseEmbeddedToken(t *testing.T) {
	var upstreamCalls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		upstreamCalls.Add(1)
		if request.Method != http.MethodGet || request.URL.RequestURI() != "/emby/Users/emby-user-1/Views?fixture=keep" {
			t.Fatalf("upstream request = %s %s", request.Method, request.URL.RequestURI())
		}
		if request.Header.Get("X-Emby-Authorization") != mediaBrowserAuthorizationWithToken(fixtureAccessToken) || request.Header.Get(accessTokenHeader) != "" {
			t.Fatal("upstream authentication headers changed")
		}
		writer.WriteHeader(http.StatusNoContent)
	}))
	defer upstream.Close()

	tokenService := &fakeTokenService{principal: fixturePrincipal()}
	var logs bytes.Buffer
	gateway := newTestGateway(t, upstream.URL, tokenService, &logs)
	request := httptest.NewRequest(http.MethodGet, "/Users/emby-user-1/Views?fixture=keep", nil)
	request.Header.Set("X-Emby-Authorization", mediaBrowserAuthorizationWithToken(fixtureAccessToken))
	response := httptest.NewRecorder()
	gateway.ServeHTTP(response, request)

	if response.Code != http.StatusNoContent || upstreamCalls.Load() != 1 {
		t.Fatalf("response=%d upstreamCalls=%d, want proxied 204", response.Code, upstreamCalls.Load())
	}
	_, resolved := tokenService.snapshot()
	if len(resolved) != 1 || resolved[0] != fixtureAccessToken {
		t.Fatalf("resolved tokens = %#v", resolved)
	}
	assertSecretsAbsent(t, logs.String(), fixtureAccessToken)
}

func TestExtractProtectedAccessTokenUsesOneConsistentVersionedSource(t *testing.T) {
	embyEmbeddedToken := `Emby UserId="emby-user-1", Client="Infuse", Device="iPhone", DeviceId="device-1", Version="8.5", Token="` + fixtureAccessToken + `"`
	tests := []struct {
		name       string
		header     http.Header
		wantToken  string
		wantReason string
		wantAccept bool
	}{
		{
			name: "X-Emby-Token only", header: http.Header{accessTokenHeader: {fixtureAccessToken}},
			wantToken: fixtureAccessToken, wantAccept: true,
		},
		{
			name: "Infuse MediaBrowser embedded token", header: http.Header{embyAuthorizationHeader: {mediaBrowserAuthorizationWithToken(fixtureAccessToken)}},
			wantToken: fixtureAccessToken, wantAccept: true,
		},
		{
			name: "X-Emby-Authorization Emby embedded token", header: http.Header{embyAuthorizationHeader: {embyEmbeddedToken}},
			wantToken: fixtureAccessToken, wantAccept: true,
		},
		{
			name: "Authorization Emby embedded token", header: http.Header{standardAuthorizationHeader: {embyEmbeddedToken}},
			wantToken: fixtureAccessToken, wantAccept: true,
		},
		{
			name: "both sources match", header: http.Header{
				accessTokenHeader:       {fixtureAccessToken},
				embyAuthorizationHeader: {mediaBrowserAuthorizationWithToken(fixtureAccessToken)},
			},
			wantToken: fixtureAccessToken, wantAccept: true,
		},
		{
			name: "X-Emby-Token with empty application token", header: http.Header{
				accessTokenHeader:       {fixtureAccessToken},
				embyAuthorizationHeader: {fixtureMediaBrowserAuthorization},
			},
			wantToken: fixtureAccessToken, wantAccept: true,
		},
		{
			name: "both sources conflict", header: http.Header{
				accessTokenHeader:       {fixtureAccessToken},
				embyAuthorizationHeader: {mediaBrowserAuthorizationWithToken("conflicting-token")},
			},
			wantReason: "token_ambiguous",
		},
		{
			name: "duplicate X-Emby-Token", header: http.Header{accessTokenHeader: {fixtureAccessToken, fixtureAccessToken}},
			wantReason: "token_ambiguous",
		},
		{
			name: "conflicting application header names", header: http.Header{
				embyAuthorizationHeader:     {mediaBrowserAuthorizationWithToken(fixtureAccessToken)},
				standardAuthorizationHeader: {embyEmbeddedToken},
			},
			wantReason: "token_ambiguous",
		},
		{
			name: "empty X-Emby-Token", header: http.Header{accessTokenHeader: {""}},
			wantReason: "token_invalid",
		},
		{
			name: "embedded token missing", header: http.Header{embyAuthorizationHeader: {fixtureMediaBrowserAuthorization}},
			wantReason: "token_missing",
		},
		{
			name:       "embedded token has incomplete metadata",
			header:     http.Header{embyAuthorizationHeader: {`MediaBrowser Client="Infuse", Token="` + fixtureAccessToken + `"`}},
			wantReason: "token_invalid",
		},
		{
			name: "unsupported scheme", header: http.Header{standardAuthorizationHeader: {`Bearer ` + fixtureAccessToken}},
			wantReason: "token_invalid",
		},
		{
			name: "invalid application header does not hide behind X-Emby-Token", header: http.Header{
				accessTokenHeader:           {fixtureAccessToken},
				standardAuthorizationHeader: {`Bearer ` + fixtureAccessToken},
			},
			wantReason: "token_invalid",
		},
		{name: "no token source", header: http.Header{}, wantReason: "token_missing"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			token, reasonCode, accepted := extractProtectedAccessToken(test.header)
			if token != test.wantToken || reasonCode != test.wantReason || accepted != test.wantAccept {
				t.Fatalf("result=(%q,%q,%t), want (%q,%q,%t)", token, reasonCode, accepted, test.wantToken, test.wantReason, test.wantAccept)
			}
		})
	}
}

func TestGatewayProtectedRequestAcceptsMappedAPIKeyQuery(t *testing.T) {
	var upstreamCalls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		upstreamCalls.Add(1)
		writer.WriteHeader(http.StatusNoContent)
	}))
	defer upstream.Close()

	tokenService := &fakeTokenService{principal: fixturePrincipal()}
	var logs bytes.Buffer
	gateway := newTestGateway(t, upstream.URL, tokenService, &logs)
	request := httptest.NewRequest(http.MethodGet, "/Users/emby-user-1/Views?api_key="+fixtureAccessToken, nil)
	response := httptest.NewRecorder()
	gateway.ServeHTTP(response, request)

	if response.Code != http.StatusNoContent || upstreamCalls.Load() != 1 {
		t.Fatalf("response=%d upstreamCalls=%d, want proxied 204", response.Code, upstreamCalls.Load())
	}
	_, resolved := tokenService.snapshot()
	if len(resolved) != 1 || resolved[0] != fixtureAccessToken {
		t.Fatalf("resolved tokens = %#v", resolved)
	}
	for _, expected := range []string{"statusCode=204", "outcome=success", "apiKeyQueryPresent=true", "queryTokenSourceCount=1", "queryTokenState=present"} {
		if !strings.Contains(logs.String(), expected) {
			t.Fatalf("logs = %q, want %s", logs.String(), expected)
		}
	}
	assertSecretsAbsent(t, logs.String(), fixtureAccessToken)
}

func TestExtractProtectedRequestAccessTokenSupportsCompatibleCarriers(t *testing.T) {
	tests := []struct {
		name       string
		target     string
		headers    http.Header
		wantToken  string
		wantReason string
		wantAccept bool
	}{
		{name: "api_key query", target: "/Items/fixture?api_key=" + fixtureAccessToken, wantToken: fixtureAccessToken, wantAccept: true},
		{name: "mixed case api key query", target: "/Items/fixture?API_KEY=" + fixtureAccessToken, wantToken: fixtureAccessToken, wantAccept: true},
		{name: "X-Emby-Token query", target: "/Items/fixture?x-emby-token=" + fixtureAccessToken, wantToken: fixtureAccessToken, wantAccept: true},
		{name: "X-MediaBrowser-Token query", target: "/Items/fixture?X-MediaBrowser-Token=" + fixtureAccessToken, wantToken: fixtureAccessToken, wantAccept: true},
		{name: "AccessToken query", target: "/Items/fixture?AccessToken=" + fixtureAccessToken, wantToken: fixtureAccessToken, wantAccept: true},
		{
			name: "X-MediaBrowser-Token header", target: "/Items/fixture",
			headers: http.Header{http.CanonicalHeaderKey(mediaBrowserTokenHeader): {fixtureAccessToken}}, wantToken: fixtureAccessToken, wantAccept: true,
		},
		{
			name: "X-MediaBrowser-Authorization embedded token", target: "/Items/fixture",
			headers:   http.Header{http.CanonicalHeaderKey(mediaBrowserAuthorizationHeader): {mediaBrowserAuthorizationWithToken(fixtureAccessToken)}},
			wantToken: fixtureAccessToken, wantAccept: true,
		},
		{
			name: "header and query match", target: "/Items/fixture?api_key=" + fixtureAccessToken,
			headers: http.Header{accessTokenHeader: {fixtureAccessToken}}, wantToken: fixtureAccessToken, wantAccept: true,
		},
		{
			name: "header and query conflict", target: "/Items/fixture?api_key=conflicting-token",
			headers: http.Header{accessTokenHeader: {fixtureAccessToken}}, wantReason: "token_ambiguous",
		},
		{name: "duplicate query value", target: "/Items/fixture?api_key=a&api_key=a", wantReason: "token_ambiguous"},
		{name: "duplicate query alias case", target: "/Items/fixture?api_key=a&API_KEY=a", wantReason: "token_ambiguous"},
		{name: "empty query token", target: "/Items/fixture?api_key=", wantReason: "token_invalid"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, test.target, nil)
			request.Header = test.headers.Clone()
			token, reasonCode, accepted := extractProtectedRequestAccessToken(request)
			if token != test.wantToken || reasonCode != test.wantReason || accepted != test.wantAccept {
				t.Fatalf("result=(%q,%q,%t), want (%q,%q,%t)", token, reasonCode, accepted, test.wantToken, test.wantReason, test.wantAccept)
			}
		})
	}
}

func TestGatewayRequestCompletionLogsSanitizedAuthenticationShape(t *testing.T) {
	var upstreamCalls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		upstreamCalls.Add(1)
		writer.WriteHeader(http.StatusNoContent)
	}))
	defer upstream.Close()

	tokenService := &fakeTokenService{principal: fixturePrincipal()}
	var logs bytes.Buffer
	gateway := newTestGateway(t, upstream.URL, tokenService, &logs)
	request := httptest.NewRequest(
		http.MethodGet,
		"/Items/fixture?IncludeExternalContent=true&UserId=emby-user-1",
		nil,
	)
	request.Header.Set(
		"X-Emby-Authorization",
		`MediaBrowser UserId="emby-user-1", Client="Infuse", Device="iPhone", DeviceId="device-1", Version="8.5", Token="`+fixtureAccessToken+`"`,
	)
	request.Header.Set("User-Agent", "Infuse-Direct/8.5")
	response := httptest.NewRecorder()
	gateway.ServeHTTP(response, request)

	if response.Code != http.StatusNoContent || upstreamCalls.Load() != 1 {
		t.Fatalf("response=%d upstreamCalls=%d, want proxied 204", response.Code, upstreamCalls.Load())
	}
	for _, expected := range []string{
		"code=request_completed",
		"method=GET",
		`host="example.com"`,
		`path="/Items/fixture"`,
		`queryKeys="IncludeExternalContent,UserId"`,
		"queryKeyCount=2",
		"route=protected",
		"pathMode=root",
		"statusCode=204",
		"outcome=success",
		"xEmbyTokenCount=0",
		"xEmbyTokenState=missing",
		"xEmbyAuthorizationCount=1",
		"authorizationCount=0",
		"applicationScheme=media_browser",
		"embeddedTokenState=present",
		"apiKeyQueryPresent=false",
		"userAgentFamily=infuse_direct",
		`userAgentVersion="8.5"`,
	} {
		if !strings.Contains(logs.String(), expected) {
			t.Fatalf("logs = %q, want %s", logs.String(), expected)
		}
	}
	assertSecretsAbsent(t, logs.String(), fixtureAccessToken, "emby-user-1")
}

func TestGatewayRequestCompletionLogsSuccessfulUpstreamStatus(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.WriteHeader(http.StatusNoContent)
	}))
	defer upstream.Close()

	tokenService := &fakeTokenService{principal: fixturePrincipal()}
	var logs bytes.Buffer
	gateway := newTestGateway(t, upstream.URL, tokenService, &logs)
	request := httptest.NewRequest(http.MethodGet, "/emby/Items/fixture", nil)
	request.Header.Set(accessTokenHeader, fixtureAccessToken)
	response := httptest.NewRecorder()
	gateway.ServeHTTP(response, request)

	if response.Code != http.StatusNoContent {
		t.Fatalf("response=%d, want 204", response.Code)
	}
	for _, expected := range []string{
		"code=request_completed",
		"method=GET",
		`path="/emby/Items/fixture"`,
		"route=protected",
		"pathMode=emby_prefixed",
		"statusCode=204",
		"outcome=success",
		"xEmbyTokenCount=1",
		"xEmbyTokenState=present",
		"embeddedTokenState=missing",
	} {
		if !strings.Contains(logs.String(), expected) {
			t.Fatalf("logs = %q, want %s", logs.String(), expected)
		}
	}
	assertSecretsAbsent(t, logs.String(), fixtureAccessToken)
}

func TestGatewayInfoLevelSuppressesDetailedSuccessfulRequestLog(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.WriteHeader(http.StatusNoContent)
	}))
	defer upstream.Close()

	upstreamURL, err := url.Parse(upstream.URL)
	if err != nil {
		t.Fatalf("parse upstream URL: %v", err)
	}
	var logs bytes.Buffer
	gateway, err := New(Config{
		Upstream:     upstreamURL,
		TokenService: &fakeTokenService{principal: fixturePrincipal()},
		Logger:       log.New(&logs, "", 0),
		Debug:        false,
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	request := httptest.NewRequest(http.MethodGet, "/Items/fixture", nil)
	request.Header.Set(accessTokenHeader, fixtureAccessToken)
	gateway.ServeHTTP(httptest.NewRecorder(), request)
	if strings.Contains(logs.String(), "code=request_completed") {
		t.Fatalf("info logs=%q, want no detailed request completion", logs.String())
	}
}

func TestApplicationAuthorizationDiagnosticsHandlesEitherHeaderWithoutSecrets(t *testing.T) {
	tests := []struct {
		name           string
		xEmbyValues    []string
		standardValues []string
		wantScheme     string
		wantTokenState string
	}{
		{name: "missing", wantScheme: "missing", wantTokenState: "missing"},
		{
			name: "standard Emby without token", standardValues: []string{fixtureApplicationAuthorization},
			wantScheme: "emby", wantTokenState: "empty",
		},
		{
			name:        "Infuse MediaBrowser with token",
			xEmbyValues: []string{`MediaBrowser UserId="emby-user-1", Client="Infuse", Device="iPhone", DeviceId="device-1", Version="8.5", Token="` + fixtureAccessToken + `"`},
			wantScheme:  "media_browser", wantTokenState: "present",
		},
		{
			name: "conflicting header names", xEmbyValues: []string{fixtureApplicationAuthorization}, standardValues: []string{fixtureApplicationAuthorization},
			wantScheme: "ambiguous", wantTokenState: "ambiguous",
		},
		{
			name: "unsupported scheme", standardValues: []string{`Bearer ` + fixtureAccessToken},
			wantScheme: "other", wantTokenState: "unparseable",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			header := make(http.Header)
			for _, value := range test.xEmbyValues {
				header.Add(embyAuthorizationHeader, value)
			}
			for _, value := range test.standardValues {
				header.Add(standardAuthorizationHeader, value)
			}
			scheme, tokenState := applicationAuthorizationDiagnostics(header)
			if scheme != test.wantScheme || tokenState != test.wantTokenState {
				t.Fatalf("diagnostics=(%s,%s), want (%s,%s)", scheme, tokenState, test.wantScheme, test.wantTokenState)
			}
		})
	}
}

func TestRequestLogInfrastructurePreservesWriterAndBoundsInput(t *testing.T) {
	response := httptest.NewRecorder()
	statusWriter := &requestStatusWriter{ResponseWriter: response}
	if _, err := statusWriter.Write([]byte("ok")); err != nil {
		t.Fatalf("write response: %v", err)
	}
	if statusWriter.statusCode != http.StatusOK || response.Code != http.StatusOK {
		t.Fatalf("status writer=%d response=%d, want 200", statusWriter.statusCode, response.Code)
	}
	if err := http.NewResponseController(statusWriter).Flush(); err != nil {
		t.Fatalf("flush through wrapped writer: %v", err)
	}

	request := &http.Request{
		Method: http.MethodGet,
		Header: http.Header{
			accessTokenHeader: {fixtureAccessToken, "second-token"},
			"User-Agent":      {"Infuse-Direct/8.5 with-untrusted-suffix"},
		},
	}
	request.Header.Set(mediaBrowserTokenHeader, fixtureAccessToken)
	request.Header.Set(mediaBrowserAuthorizationHeader, mediaBrowserAuthorizationWithToken(fixtureAccessToken))
	snapshot := captureRequestLogSnapshot(request)
	if snapshot.path != "" || snapshot.apiKeyQueryPresent || snapshot.xEmbyTokenState != "ambiguous" ||
		snapshot.xMediaBrowserTokenCount != 1 || snapshot.xMediaBrowserTokenState != "present" ||
		snapshot.xMediaAuthorizationCount != 1 || snapshot.applicationScheme != "media_browser" || snapshot.embeddedTokenState != "present" ||
		snapshot.userAgentFamily != "infuse_direct" || snapshot.userAgentVersion != "8.5" {
		t.Fatalf("snapshot = %+v", snapshot)
	}
}

func TestUserAgentDiagnosticsRecognizesClientFamiliesWithoutRawSuffix(t *testing.T) {
	tests := []struct {
		userAgent   string
		wantFamily  string
		wantVersion string
	}{
		{userAgent: "Infuse-Library/8.5", wantFamily: "infuse_library", wantVersion: "8.5"},
		{userAgent: "SenPlayer/3.1", wantFamily: "senplayer", wantVersion: "3.1"},
		{userAgent: "Yamby/1.2.0", wantFamily: "yamby", wantVersion: "1.2.0"},
		{userAgent: "VidHub/1.7.6", wantFamily: "vidhub", wantVersion: "1.7.6"},
		{userAgent: "Fileball/238 CFNetwork/1410.0.3 Darwin/22.6.0", wantFamily: "fileball", wantVersion: "238"},
		{userAgent: "Conflux/1.2.0 (app.svn.Conflux; build:1200)", wantFamily: "conflux", wantVersion: "1.2.0"},
		{userAgent: "Emby/3.2.32-17.41 (Linux;Android 14)", wantFamily: "emby", wantVersion: "3.2.32-17.41"},
		{userAgent: "Yamby/fixture-access-token", wantFamily: "yamby", wantVersion: "invalid"},
		{userAgent: "Mozilla/5.0 token-like-private-value", wantFamily: "other"},
	}
	for _, test := range tests {
		t.Run(test.wantFamily, func(t *testing.T) {
			family, version := userAgentDiagnostics(test.userAgent)
			if family != test.wantFamily || version != test.wantVersion {
				t.Fatalf("diagnostics=(%q,%q), want (%q,%q)", family, version, test.wantFamily, test.wantVersion)
			}
		})
	}
}

func TestGatewayPlaybackSessionEventsAreTransparent(t *testing.T) {
	tests := []struct {
		name        string
		path        string
		body        string
		wantLogCode string
		wantMessage string
	}{
		{
			name:        "playing",
			path:        "/emby/Sessions/Playing",
			body:        `{"ItemId":"item-1","MediaSourceId":"source-1","PlaySessionId":"session-1","PositionTicks":0,"PlayMethod":"DirectPlay","CanSeek":true}`,
			wantLogCode: "code=playback_session_started",
			wantMessage: `message="播放开始上报成功"`,
		},
		{
			name:        "progress",
			path:        "/emby/Sessions/Playing/Progress",
			body:        `{"ItemId":"item-1","MediaSourceId":"source-1","PlaySessionId":"session-1","PositionTicks":123456789,"PlayMethod":"DirectPlay","IsPaused":false}`,
			wantLogCode: "code=playback_progress_reported",
			wantMessage: `message="播放进度上报成功"`,
		},
		{
			name:        "stopped",
			path:        "/emby/Sessions/Playing/Stopped",
			body:        `{"ItemId":"item-1","MediaSourceId":"source-1","PlaySessionId":"session-1","PositionTicks":223456789,"Failed":false,"IsAutomated":false}`,
			wantLogCode: "code=playback_session_stopped",
			wantMessage: `message="播放停止上报成功"`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var upstreamCalls int
			upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				upstreamCalls++
				body, err := io.ReadAll(request.Body)
				if err != nil {
					t.Fatalf("read upstream request body: %v", err)
				}
				if request.Method != http.MethodPost || request.URL.Path != test.path || request.URL.RawQuery != "fixture=keep" {
					t.Fatalf("upstream request = %s %s?%s", request.Method, request.URL.Path, request.URL.RawQuery)
				}
				if request.Header.Get(accessTokenHeader) != fixtureAccessToken ||
					request.Header.Get("Content-Type") != "application/json" || request.Header.Get("X-Fixture") != "preserved" {
					t.Fatalf("upstream headers = %#v", request.Header)
				}
				if string(body) != test.body {
					t.Fatalf("upstream body = %q, want %q", string(body), test.body)
				}
				writer.Header().Set("X-Upstream", "preserved")
				writer.WriteHeader(http.StatusNoContent)
			}))
			defer upstream.Close()

			tokenService := &fakeTokenService{principal: fixturePrincipal()}
			var logs bytes.Buffer
			gateway := newTestGateway(t, upstream.URL, tokenService, &logs)
			request := httptest.NewRequest(http.MethodPost, test.path+"?fixture=keep", strings.NewReader(test.body))
			request.Header.Set(accessTokenHeader, fixtureAccessToken)
			request.Header.Set("Content-Type", "application/json")
			request.Header.Set("X-Fixture", "preserved")
			response := httptest.NewRecorder()
			gateway.ServeHTTP(response, request)

			if response.Code != http.StatusNoContent || response.Header().Get("X-Upstream") != "preserved" || upstreamCalls != 1 {
				t.Fatalf("response=%d headers=%v upstreamCalls=%d", response.Code, response.Header(), upstreamCalls)
			}

			unauthorizedRequest := httptest.NewRequest(http.MethodPost, test.path, strings.NewReader(test.body))
			unauthorizedRequest.Header.Set("Content-Type", "application/json")
			unauthorizedResponse := httptest.NewRecorder()
			gateway.ServeHTTP(unauthorizedResponse, unauthorizedRequest)
			if unauthorizedResponse.Code != http.StatusUnauthorized || unauthorizedResponse.Body.Len() != 0 || upstreamCalls != 1 {
				t.Fatalf("unauthorized response=%d body=%q upstreamCalls=%d", unauthorizedResponse.Code, unauthorizedResponse.Body.String(), upstreamCalls)
			}

			_, resolved := tokenService.snapshot()
			if len(resolved) != 1 || resolved[0] != fixtureAccessToken {
				t.Fatalf("resolved tokens = %#v", resolved)
			}
			for _, expected := range []string{
				test.wantLogCode,
				test.wantMessage,
				`itemId="item-1"`,
				`playSessionId="session-1"`,
				"snapshotState=recorded",
				"statusCode=204",
			} {
				if !strings.Contains(logs.String(), expected) {
					t.Fatalf("logs=%q, want %s", logs.String(), expected)
				}
			}
			assertSecretsAbsent(t, logs.String(), fixtureAccessToken, test.body)
		})
	}
}

func TestGatewayPlaybackProgressFailureAndRecoveryAreObvious(t *testing.T) {
	var upstreamCalls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		upstreamCalls.Add(1)
		writer.WriteHeader(http.StatusNoContent)
	}))
	defer upstream.Close()

	tokenService := &fakeTokenService{principal: fixturePrincipal(), resolveErr: embytoken.ErrStoreUnavailable}
	var logs bytes.Buffer
	gateway := newTestGateway(t, upstream.URL, tokenService, &logs)
	progressBody := `{"ItemId":"item-1","MediaSourceId":"source-1","PlaySessionId":"session-1","PositionTicks":123456789,"PlayMethod":"DirectPlay","IsPaused":false}`

	failedRequest := httptest.NewRequest(http.MethodPost, "/Sessions/Playing/Progress", strings.NewReader(progressBody))
	failedRequest.Header.Set(accessTokenHeader, fixtureAccessToken)
	failedRequest.Header.Set("Content-Type", "application/json")
	failedResponse := httptest.NewRecorder()
	gateway.ServeHTTP(failedResponse, failedRequest)

	if failedResponse.Code != http.StatusServiceUnavailable || upstreamCalls.Load() != 0 {
		t.Fatalf("failed response=%d upstreamCalls=%d", failedResponse.Code, upstreamCalls.Load())
	}
	for _, expected := range []string{
		"code=playback_progress_failed",
		`message="播放进度上报失败"`,
		"result=failure",
		"forwardState=not_attempted",
		"statusCode=503",
		`itemId=""`,
		`playSessionId=""`,
		"snapshotState=not_inspected",
	} {
		if !strings.Contains(logs.String(), expected) {
			t.Fatalf("failure logs=%q, want %s", logs.String(), expected)
		}
	}

	tokenService.mu.Lock()
	tokenService.resolveErr = nil
	tokenService.mu.Unlock()
	playingBody := `{"ItemId":"item-1","MediaSourceId":"source-1","PlaySessionId":"session-1","PositionTicks":123456790,"PlayMethod":"DirectPlay"}`
	recoveredRequest := httptest.NewRequest(http.MethodPost, "/Sessions/Playing", strings.NewReader(playingBody))
	recoveredRequest.Header.Set(accessTokenHeader, fixtureAccessToken)
	recoveredRequest.Header.Set("Content-Type", "application/json")
	recoveredResponse := httptest.NewRecorder()
	gateway.ServeHTTP(recoveredResponse, recoveredRequest)

	if recoveredResponse.Code != http.StatusNoContent || upstreamCalls.Load() != 1 {
		t.Fatalf("recovered response=%d upstreamCalls=%d", recoveredResponse.Code, upstreamCalls.Load())
	}
	for _, expected := range []string{
		"code=playback_progress_recovered",
		`message="播放进度上报已恢复"`,
		"result=success",
		"recoveryEvent=start",
		`playSessionId="session-1"`,
	} {
		if !strings.Contains(logs.String(), expected) {
			t.Fatalf("recovery logs=%q, want %s", logs.String(), expected)
		}
	}

	secondProgressRequest := httptest.NewRequest(http.MethodPost, "/Sessions/Playing/Progress", strings.NewReader(progressBody))
	secondProgressRequest.Header.Set(accessTokenHeader, fixtureAccessToken)
	secondProgressRequest.Header.Set("Content-Type", "application/json")
	gateway.ServeHTTP(httptest.NewRecorder(), secondProgressRequest)
	if count := strings.Count(logs.String(), "code=playback_progress_recovered"); count != 1 {
		t.Fatalf("recovery log count=%d, want one; logs=%q", count, logs.String())
	}
	assertSecretsAbsent(t, logs.String(), fixtureAccessToken, progressBody, playingBody)
}

func TestGatewayProtectedRequestFailsClosedWithoutCallingUpstream(t *testing.T) {
	tests := []struct {
		name       string
		token      string
		duplicate  bool
		resolveErr error
		wantStatus int
	}{
		{name: "missing token", wantStatus: http.StatusUnauthorized},
		{name: "duplicate token header", token: fixtureAccessToken, duplicate: true, wantStatus: http.StatusUnauthorized},
		{name: "unknown token", token: fixtureAccessToken, resolveErr: embytoken.ErrTokenNotFound, wantStatus: http.StatusUnauthorized},
		{name: "revoked token", token: fixtureAccessToken, resolveErr: embytoken.ErrTokenRevoked, wantStatus: http.StatusUnauthorized},
		{name: "invalid token", token: fixtureAccessToken, resolveErr: embytoken.ErrInvalidInput, wantStatus: http.StatusUnauthorized},
		{name: "identity mismatch", token: fixtureAccessToken, resolveErr: embytoken.ErrIdentityMismatch, wantStatus: http.StatusUnauthorized},
		{name: "disabled user", token: fixtureAccessToken, resolveErr: embytoken.ErrUserUnavailable, wantStatus: http.StatusForbidden},
		{name: "expired user", token: fixtureAccessToken, resolveErr: embytoken.ErrUserExpired, wantStatus: http.StatusForbidden},
		{name: "store failure", token: fixtureAccessToken, resolveErr: errors.New("database failed with " + fixtureAccessToken), wantStatus: http.StatusServiceUnavailable},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			upstreamCalls := 0
			upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
				upstreamCalls++
				writer.WriteHeader(http.StatusNoContent)
			}))
			defer upstream.Close()

			tokenService := &fakeTokenService{resolveErr: test.resolveErr}
			var logs bytes.Buffer
			gateway := newTestGateway(t, upstream.URL, tokenService, &logs)
			request := httptest.NewRequest(http.MethodGet, "/emby/Items/fixture", nil)
			if test.token != "" {
				request.Header.Add(accessTokenHeader, test.token)
			}
			if test.duplicate {
				request.Header.Add(accessTokenHeader, "second-token")
			}
			response := httptest.NewRecorder()
			gateway.ServeHTTP(response, request)

			if response.Code != test.wantStatus {
				t.Fatalf("status = %d, want %d", response.Code, test.wantStatus)
			}
			if response.Body.Len() != 0 {
				t.Fatalf("body = %q, want empty", response.Body.String())
			}
			if upstreamCalls != 0 {
				t.Fatalf("upstream calls = %d, want 0", upstreamCalls)
			}
			assertSecretsAbsent(t, logs.String(), fixtureAccessToken, "second-token")
		})
	}
}

func TestGatewayDistinguishesCanceledIdentityLookupFromStoreOutage(t *testing.T) {
	tests := []struct {
		name       string
		err        error
		wantStatus int
		wantCode   string
	}{
		{name: "request canceled", err: context.Canceled, wantStatus: 499, wantCode: "code=request_completed"},
		{name: "deadline exceeded", err: context.DeadlineExceeded, wantStatus: http.StatusGatewayTimeout, wantCode: "token_request_deadline_exceeded"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var upstreamCalls atomic.Int32
			upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
				upstreamCalls.Add(1)
				writer.WriteHeader(http.StatusNoContent)
			}))
			defer upstream.Close()

			var logs bytes.Buffer
			gateway := newTestGateway(t, upstream.URL, &fakeTokenService{resolveErr: test.err}, &logs)
			request := httptest.NewRequest(http.MethodGet, "/Items/fixture", nil)
			request.Header.Set(accessTokenHeader, fixtureAccessToken)
			response := httptest.NewRecorder()
			gateway.ServeHTTP(response, request)

			if response.Code != test.wantStatus || upstreamCalls.Load() != 0 {
				t.Fatalf("response=%d upstreamCalls=%d, want local %d", response.Code, upstreamCalls.Load(), test.wantStatus)
			}
			if !strings.Contains(logs.String(), test.wantCode) || strings.Contains(logs.String(), "code=token_store_unavailable") {
				t.Fatalf("logs=%q", logs.String())
			}
			if test.err == context.Canceled && strings.Count(logs.String(), "code=") != 1 {
				t.Fatalf("canceled request logs=%q, want one completion event", logs.String())
			}
			assertSecretsAbsent(t, logs.String(), fixtureAccessToken)
		})
	}
}

func TestGatewayUpstreamFailureIsSanitized(t *testing.T) {
	tokenService := &fakeTokenService{}
	var logs bytes.Buffer
	upstreamURL, err := url.Parse("http://upstream.invalid")
	if err != nil {
		t.Fatalf("parse upstream URL: %v", err)
	}
	gateway, err := New(Config{
		Upstream:     upstreamURL,
		TokenService: tokenService,
		Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
			return nil, errors.New("dial http://user:" + fixturePassword + "@upstream.invalid/?token=" + fixtureAccessToken)
		}),
		Logger: log.New(&logs, "", 0),
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	request := httptest.NewRequest(http.MethodGet, "/emby/Items/fixture", nil)
	request.Header.Set(accessTokenHeader, fixtureAccessToken)
	response := httptest.NewRecorder()
	gateway.ServeHTTP(response, request)

	if response.Code != http.StatusBadGateway || response.Body.Len() != 0 {
		t.Fatalf("response = status %d body %q", response.Code, response.Body.String())
	}
	if !strings.Contains(logs.String(), "code=upstream_unavailable") {
		t.Fatalf("logs = %q", logs.String())
	}
	assertSecretsAbsent(t, logs.String(), fixtureAccessToken, fixturePassword, "upstream.invalid")
}

func TestGatewayPublicBootstrapUsesApplicationHeaderWithoutTokenMapping(t *testing.T) {
	tests := []struct {
		name   string
		method string
		path   string
		body   string
	}{
		{name: "public users", method: http.MethodGet, path: "/emby/Users/Public", body: `[{"Id":"public-user"}]`},
		{name: "public user image", method: http.MethodGet, path: "/emby/Users/public-user/Images/Primary?Tag=fixture", body: "fixture-image"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				if request.Method != test.method || request.URL.RequestURI() != test.path {
					t.Errorf("upstream request = %s %s", request.Method, request.URL.RequestURI())
				}
				if request.Header.Get("X-Emby-Authorization") != fixtureApplicationAuthorization {
					t.Error("application authorization header changed")
				}
				writer.Header().Set("X-Upstream", "called")
				writer.WriteHeader(http.StatusOK)
				_, _ = io.WriteString(writer, test.body)
			}))
			defer upstream.Close()

			tokenService := &fakeTokenService{}
			var logs bytes.Buffer
			gateway := newTestGateway(t, upstream.URL, tokenService, &logs)
			request := httptest.NewRequest(test.method, test.path, nil)
			request.Header.Set("X-Emby-Authorization", fixtureApplicationAuthorization)
			response := httptest.NewRecorder()
			gateway.ServeHTTP(response, request)

			if response.Code != http.StatusOK || response.Body.String() != test.body || response.Header().Get("X-Upstream") != "called" {
				t.Fatalf("response = status %d headers=%v body=%q", response.Code, response.Header(), response.Body.String())
			}
			recorded, resolved := tokenService.snapshot()
			if len(recorded) != 0 || len(resolved) != 0 {
				t.Fatalf("token service calls: recorded=%d resolved=%d", len(recorded), len(resolved))
			}
			assertSecretsAbsent(t, logs.String(), fixtureApplicationAuthorization)
		})
	}
}

func TestGatewayPublicBootstrapAcceptsMappedTokenCarrier(t *testing.T) {
	var upstreamCalls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		upstreamCalls.Add(1)
		if request.URL.RequestURI() != "/emby/users/public?API_KEY="+fixtureAccessToken {
			t.Fatalf("upstream request=%s", request.URL.RequestURI())
		}
		writer.WriteHeader(http.StatusNoContent)
	}))
	defer upstream.Close()

	tokenService := &fakeTokenService{principal: fixturePrincipal()}
	var logs bytes.Buffer
	gateway := newTestGateway(t, upstream.URL, tokenService, &logs)
	request := httptest.NewRequest(http.MethodGet, "/users/public?API_KEY="+fixtureAccessToken, nil)
	request.Header.Set(embyAuthorizationHeader, fixtureApplicationAuthorization)
	response := httptest.NewRecorder()
	gateway.ServeHTTP(response, request)

	if response.Code != http.StatusNoContent || upstreamCalls.Load() != 1 {
		t.Fatalf("response=%d upstreamCalls=%d", response.Code, upstreamCalls.Load())
	}
	_, resolved := tokenService.snapshot()
	if len(resolved) != 1 || resolved[0] != fixtureAccessToken {
		t.Fatalf("resolved tokens=%#v", resolved)
	}
	assertSecretsAbsent(t, logs.String(), fixtureAccessToken)
}

func TestGatewayAuthenticationRejectsExternalTokenCarriers(t *testing.T) {
	tests := []struct {
		name   string
		target string
		header string
	}{
		{name: "direct token header", target: authenticationPath, header: fixtureAccessToken},
		{name: "query token", target: authenticationPath + "?api_key=" + fixtureAccessToken},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var upstreamCalls atomic.Int32
			upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
				upstreamCalls.Add(1)
				writer.WriteHeader(http.StatusNoContent)
			}))
			defer upstream.Close()

			var logs bytes.Buffer
			gateway := newTestGateway(t, upstream.URL, &fakeTokenService{}, &logs)
			request := httptest.NewRequest(http.MethodPost, test.target, strings.NewReader(`{"Username":"user"}`))
			request.Header.Set(embyAuthorizationHeader, fixtureApplicationAuthorization)
			if test.header != "" {
				request.Header.Set(accessTokenHeader, test.header)
			}
			response := httptest.NewRecorder()
			gateway.ServeHTTP(response, request)

			if response.Code != http.StatusUnauthorized || upstreamCalls.Load() != 0 {
				t.Fatalf("response=%d upstreamCalls=%d", response.Code, upstreamCalls.Load())
			}
			if !strings.Contains(logs.String(), "code=authentication_token_invalid") {
				t.Fatalf("logs=%q", logs.String())
			}
			assertSecretsAbsent(t, logs.String(), fixtureAccessToken)
		})
	}
}

func TestGatewayAuthenticationAndBootstrapRejectInvalidApplicationHeader(t *testing.T) {
	tests := []struct {
		name    string
		method  string
		path    string
		headers http.Header
	}{
		{name: "authentication missing header", method: http.MethodPost, path: authenticationPath},
		{
			name: "authentication wrong scheme", method: http.MethodPost, path: authenticationPath,
			headers: http.Header{"Authorization": {`Bearer Client="Infuse", Device="iPhone", DeviceId="device-1", Version="8.0"`}},
		},
		{
			name: "MediaBrowser on standard authorization is unsupported", method: http.MethodPost, path: authenticationPath,
			headers: http.Header{"Authorization": {fixtureMediaBrowserAuthorization}},
		},
		{
			name: "bootstrap has both header names", method: http.MethodGet, path: "/emby/Users/Public",
			headers: http.Header{
				"Authorization":        {fixtureApplicationAuthorization},
				"X-Emby-Authorization": {fixtureApplicationAuthorization},
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var upstreamCalls atomic.Int32
			upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
				upstreamCalls.Add(1)
				writer.WriteHeader(http.StatusNoContent)
			}))
			defer upstream.Close()

			tokenService := &fakeTokenService{}
			var logs bytes.Buffer
			gateway := newTestGateway(t, upstream.URL, tokenService, &logs)
			request := httptest.NewRequest(test.method, test.path, strings.NewReader("{}"))
			request.Header = test.headers.Clone()
			response := httptest.NewRecorder()
			gateway.ServeHTTP(response, request)

			if response.Code != http.StatusUnauthorized || response.Body.Len() != 0 {
				t.Fatalf("response = status %d body=%q", response.Code, response.Body.String())
			}
			if upstreamCalls.Load() != 0 {
				t.Fatalf("upstream calls = %d, want 0", upstreamCalls.Load())
			}
			if !strings.Contains(logs.String(), "code=application_header_invalid") {
				t.Fatalf("logs = %q", logs.String())
			}
			assertSecretsAbsent(t, logs.String(), fixtureApplicationAuthorization)
		})
	}
}

func TestExtractApplicationMetadataUsesStrictVersionedHeader(t *testing.T) {
	tests := []struct {
		name       string
		headers    http.Header
		want       AuthenticationMetadata
		wantAccept bool
	}{
		{
			name: "authorization header", headers: http.Header{"Authorization": {fixtureApplicationAuthorization}},
			want: AuthenticationMetadata{DeviceID: "device-1", ClientName: "Infuse"}, wantAccept: true,
		},
		{
			name: "x emby authorization header with quoted comma", headers: http.Header{
				"X-Emby-Authorization": {`Emby Client="Infuse, Pro", Device="Living Room, TV", DeviceId="device-2", Version="8.1"`},
			},
			want: AuthenticationMetadata{DeviceID: "device-2", ClientName: "Infuse, Pro"}, wantAccept: true,
		},
		{
			name: "Infuse MediaBrowser scheme", headers: http.Header{
				"X-Emby-Authorization": {fixtureMediaBrowserAuthorization},
			},
			want: AuthenticationMetadata{DeviceID: "device-1", ClientName: "Infuse"}, wantAccept: true,
		},
		{
			name: "MediaBrowser compatibility header", headers: func() http.Header {
				header := make(http.Header)
				header.Set(mediaBrowserAuthorizationHeader, fixtureMediaBrowserAuthorization)
				return header
			}(),
			want: AuthenticationMetadata{DeviceID: "device-1", ClientName: "Infuse"}, wantAccept: true,
		},
		{name: "missing required field", headers: http.Header{"Authorization": {`Emby Client="Infuse", Device="iPhone", Version="8.0"`}}},
		{name: "duplicate field", headers: http.Header{"Authorization": {`Emby Client="Infuse", Client="Other", Device="iPhone", DeviceId="device-1", Version="8.0"`}}},
		{name: "unknown field", headers: http.Header{"Authorization": {`Emby Client="Infuse", Device="iPhone", DeviceId="device-1", Version="8.0", Language="zh"`}}},
		{name: "nonempty embedded token", headers: http.Header{"Authorization": {`Emby Client="Infuse", Device="iPhone", DeviceId="device-1", Version="8.0", Token="old-token"`}}},
		{name: "invalid escape", headers: http.Header{"Authorization": {`Emby Client="Infuse\n", Device="iPhone", DeviceId="device-1", Version="8.0"`}}},
		{name: "duplicate header value", headers: http.Header{"Authorization": {fixtureApplicationAuthorization, fixtureApplicationAuthorization}}},
		{name: "oversized device id", headers: http.Header{"Authorization": {`Emby Client="Infuse", Device="iPhone", DeviceId="` + strings.Repeat("x", 257) + `", Version="8.0"`}}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			metadata, ok := extractApplicationMetadata(test.headers)
			if ok != test.wantAccept || metadata != test.want {
				t.Fatalf("extractApplicationMetadata() = (%+v, %t), want (%+v, %t)", metadata, ok, test.want, test.wantAccept)
			}
		})
	}
}

func TestNewRejectsUnsafeOrIncompleteConfiguration(t *testing.T) {
	tokenService := &fakeTokenService{}
	tests := []struct {
		name         string
		upstream     string
		nilUpstream  bool
		tokenService TokenService
	}{
		{name: "missing upstream", nilUpstream: true, tokenService: tokenService},
		{name: "non http scheme", upstream: "ftp://emby.internal", tokenService: tokenService},
		{name: "upstream credentials", upstream: "http://user:password@emby.internal", tokenService: tokenService},
		{name: "upstream query", upstream: "http://emby.internal?api_key=secret", tokenService: tokenService},
		{name: "upstream fragment", upstream: "http://emby.internal#fragment", tokenService: tokenService},
		{name: "missing token service", upstream: "http://emby.internal"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var upstream *url.URL
			if !test.nilUpstream {
				parsed, err := url.Parse(test.upstream)
				if err != nil {
					t.Fatalf("parse test upstream: %v", err)
				}
				upstream = parsed
			}
			if _, err := New(Config{Upstream: upstream, TokenService: test.tokenService}); err == nil {
				t.Fatal("New() error = nil, want invalid configuration error")
			}
		})
	}
}

func TestClassifyRouteFailsClosedOutsideExactAuthenticationContract(t *testing.T) {
	tests := []struct {
		method string
		path   string
		want   routeKind
	}{
		{method: http.MethodPost, path: authenticationPath, want: routeAuthentication},
		{method: http.MethodGet, path: authenticationPath, want: routeProtected},
		{method: http.MethodPost, path: authenticationPath + "/", want: routeProtected},
		{method: http.MethodPost, path: "/emby/users/authenticatebyname", want: routeAuthentication},
		{method: http.MethodPost, path: "/emby%2FUsers%2FAuthenticateByName", want: routeProtected},
		{method: http.MethodGet, path: "/emby/Users/Public", want: routePublicBootstrap},
		{method: http.MethodGet, path: "/emby/Users/public-user/Images/Primary", want: routePublicBootstrap},
		{method: http.MethodHead, path: "/emby/Users/public-user/Images/Primary", want: routePublicBootstrap},
		{method: http.MethodPost, path: "/emby/Users/public-user/Images/Primary", want: routeProtected},
		{method: http.MethodGet, path: "/emby/Users/public-user/Images/Primary/0", want: routeProtected},
		{method: http.MethodGet, path: "/emby/Users/public-user/Images/Primary/Delete", want: routeProtected},
		{method: http.MethodGet, path: "/emby/Users/public%2Duser/Images/Primary", want: routeProtected},
		{method: http.MethodGet, path: "/emby/System/Info/Public", want: routeSystemInfoPublic},
		{method: http.MethodHead, path: "/emby/System/Info/Public", want: routeProtected},
		{method: http.MethodPost, path: "/emby/System/Info/Public", want: routeProtected},
		{method: http.MethodGet, path: "/emby/System/Info/Public/", want: routeProtected},
		{method: http.MethodGet, path: "/emby/system/info/public", want: routeSystemInfoPublic},
		{method: http.MethodGet, path: "/emby/System%2FInfo%2FPublic", want: routeProtected},
		{method: http.MethodGet, path: "/emby/Items/item-1/PlaybackInfo", want: routePlaybackInfo},
		{method: http.MethodPost, path: "/emby/Items/item-1/PlaybackInfo", want: routePlaybackInfo},
		{method: http.MethodHead, path: "/emby/Items/item-1/PlaybackInfo", want: routeProtected},
		{method: http.MethodGet, path: "/emby/Items/item-1/PlaybackInfo/", want: routeProtected},
		{method: http.MethodGet, path: "/emby/Items/item%2D1/PlaybackInfo", want: routeProtected},
		{method: http.MethodGet, path: "/emby/Users/user-1/Items/item-1", want: routeItemDetail},
		{method: http.MethodPost, path: "/emby/Users/user-1/Items/item-1", want: routeProtected},
		{method: http.MethodGet, path: "/emby/Users/user-1/Items/item-1/LocalTrailers", want: routeProtected},
	}
	for _, test := range tests {
		request := httptest.NewRequest(test.method, test.path, nil)
		if got := classifyRoute(request); got != test.want {
			t.Fatalf("classifyRoute(%s %s) = %v, want %v", test.method, test.path, got, test.want)
		}
	}
}

func newAuthenticationRequest(body string) *http.Request {
	request := httptest.NewRequest(http.MethodPost, authenticationPath, strings.NewReader(body))
	request.Header.Set("Authorization", fixtureApplicationAuthorization)
	return request
}

func mediaBrowserAuthorizationWithToken(token string) string {
	return `MediaBrowser UserId="emby-user-1", Client="Infuse", Device="iPhone", DeviceId="device-1", Version="8.5", Token="` + token + `"`
}

func deflateFixture(t *testing.T, body []byte, raw bool) []byte {
	t.Helper()
	var buffer bytes.Buffer
	var writer io.WriteCloser
	if raw {
		flateWriter, err := flate.NewWriter(&buffer, flate.DefaultCompression)
		if err != nil {
			t.Fatalf("new raw deflate writer: %v", err)
		}
		writer = flateWriter
	} else {
		writer = zlib.NewWriter(&buffer)
	}
	if _, err := writer.Write(body); err != nil {
		t.Fatalf("write deflate fixture: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close deflate fixture: %v", err)
	}
	return buffer.Bytes()
}

func gzipFixture(t *testing.T, body []byte) []byte {
	t.Helper()
	var buffer bytes.Buffer
	writer := gzip.NewWriter(&buffer)
	if _, err := writer.Write(body); err != nil {
		t.Fatalf("write gzip fixture: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close gzip fixture: %v", err)
	}
	return buffer.Bytes()
}

func newTestGateway(
	t *testing.T,
	upstreamRawURL string,
	tokenService *fakeTokenService,
	logs *bytes.Buffer,
) *Gateway {
	t.Helper()
	upstreamURL, err := url.Parse(upstreamRawURL)
	if err != nil {
		t.Fatalf("parse upstream URL: %v", err)
	}
	gateway, err := New(Config{
		Upstream:     upstreamURL,
		TokenService: tokenService,
		Logger:       log.New(logs, "", 0),
		Debug:        true,
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	return gateway
}

func assertSecretsAbsent(t *testing.T, value string, secrets ...string) {
	t.Helper()
	for _, secret := range secrets {
		if secret != "" && strings.Contains(value, secret) {
			t.Fatalf("secret %q leaked in %q", secret, value)
		}
	}
}

type fakeTokenService struct {
	mu         sync.Mutex
	records    []embytoken.AuthenticationResultInput
	resolved   []string
	principal  embytoken.Principal
	recordErr  error
	resolveErr error
}

func (service *fakeTokenService) RecordAuthenticationResult(
	_ context.Context,
	input embytoken.AuthenticationResultInput,
) (embytoken.AuthenticationMapping, error) {
	service.mu.Lock()
	defer service.mu.Unlock()
	service.records = append(service.records, input)
	return embytoken.AuthenticationMapping{}, service.recordErr
}

func (service *fakeTokenService) ResolvePrincipal(_ context.Context, accessToken string) (embytoken.Principal, error) {
	service.mu.Lock()
	defer service.mu.Unlock()
	service.resolved = append(service.resolved, accessToken)
	return service.principal, service.resolveErr
}

func (service *fakeTokenService) snapshot() ([]embytoken.AuthenticationResultInput, []string) {
	service.mu.Lock()
	defer service.mu.Unlock()
	return append([]embytoken.AuthenticationResultInput(nil), service.records...), append([]string(nil), service.resolved...)
}

func fixturePrincipal() embytoken.Principal {
	return embytoken.Principal{
		MappingID: "mapping-1", ServerID: "server-1", DeviceID: "device-1", ClientName: "Infuse",
		User: models.User{ID: "user-1", EmbyID: "emby-user-1", IsActive: true},
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (function roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return function(request)
}
