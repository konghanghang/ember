package playbackgateway

import (
	"bytes"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/konghang/ember/backend/internal/services/embytoken"
)

func TestGatewayPlaybackSessionSidecarKeepsUnusableBodiesTransparent(t *testing.T) {
	tests := []struct {
		name         string
		body         string
		maxBytes     int64
		wantState    string
		extraHeaders map[string]string
	}{
		{
			name:      "malformed json",
			body:      `{"ItemId":"item-1","Padding":"private fixture`,
			maxBytes:  1024,
			wantState: "invalid",
		},
		{
			name:      "oversized json",
			body:      `{"ItemId":"item-1","Padding":"` + strings.Repeat("private", 32) + `"}`,
			maxBytes:  32,
			wantState: "too_large",
		},
		{
			name:         "encoded body",
			body:         "private encoded fixture",
			maxBytes:     1024,
			wantState:    "content_encoding_unsupported",
			extraHeaders: map[string]string{"Content-Encoding": "gzip"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var upstreamBody []byte
			upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				var err error
				upstreamBody, err = io.ReadAll(request.Body)
				if err != nil {
					t.Fatalf("read upstream body: %v", err)
				}
				writer.WriteHeader(http.StatusNoContent)
			}))
			defer upstream.Close()

			var logs bytes.Buffer
			gateway := newTestGateway(t, upstream.URL, &fakeTokenService{principal: fixturePrincipal()}, &logs)
			gateway.maxPlaybackSessionRequestBytes = test.maxBytes
			request := httptest.NewRequest(http.MethodPost, "/Sessions/Playing/Progress", strings.NewReader(test.body))
			request.Header.Set(accessTokenHeader, fixtureAccessToken)
			request.Header.Set("Content-Type", "application/json")
			for key, value := range test.extraHeaders {
				request.Header.Set(key, value)
			}
			response := httptest.NewRecorder()
			gateway.ServeHTTP(response, request)

			if response.Code != http.StatusNoContent || string(upstreamBody) != test.body {
				t.Fatalf("response=%d upstreamBody=%q, want exact original", response.Code, upstreamBody)
			}
			if !strings.Contains(logs.String(), "snapshotState="+test.wantState) {
				t.Fatalf("logs=%q, want snapshotState=%s", logs.String(), test.wantState)
			}
			assertSecretsAbsent(t, logs.String(), test.body, "private")
		})
	}
}

func TestGatewayDoesNotReadPlaybackSessionBodyBeforeIdentitySucceeds(t *testing.T) {
	tests := []struct {
		name       string
		token      string
		resolveErr error
		wantStatus int
	}{
		{name: "missing token", wantStatus: http.StatusUnauthorized},
		{name: "store unavailable", token: fixtureAccessToken, resolveErr: embytoken.ErrStoreUnavailable, wantStatus: http.StatusServiceUnavailable},
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
			gateway := newTestGateway(t, upstream.URL, &fakeTokenService{resolveErr: test.resolveErr}, &logs)
			body := &countingPlaybackSessionBody{reader: strings.NewReader(`{"ItemId":"private-item"}`)}
			request := httptest.NewRequest(http.MethodPost, "/Sessions/Playing/Progress", body)
			request.Header.Set("Content-Type", "application/json")
			if test.token != "" {
				request.Header.Set(accessTokenHeader, test.token)
			}
			response := httptest.NewRecorder()
			gateway.ServeHTTP(response, request)

			if response.Code != test.wantStatus || body.readCalls != 0 || upstreamCalls.Load() != 0 {
				t.Fatalf("response=%d readCalls=%d upstreamCalls=%d, want status=%d and no body/upstream read",
					response.Code, body.readCalls, upstreamCalls.Load(), test.wantStatus)
			}
			if !strings.Contains(logs.String(), "snapshotState=not_inspected") {
				t.Fatalf("logs=%q, want not_inspected", logs.String())
			}
			assertSecretsAbsent(t, logs.String(), "private-item", fixtureAccessToken)
		})
	}
}

func TestGatewayPlaybackProgressSuccessStaysDebugOnly(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.WriteHeader(http.StatusNoContent)
	}))
	defer upstream.Close()
	upstreamURL, err := url.Parse(upstream.URL)
	if err != nil {
		t.Fatalf("parse upstream URL: %v", err)
	}

	var logs bytes.Buffer
	tokenService := &fakeTokenService{principal: fixturePrincipal()}
	gateway, err := New(Config{
		Upstream:     upstreamURL,
		TokenService: tokenService,
		Logger:       log.New(&logs, "", 0),
		Debug:        false,
	})
	if err != nil {
		t.Fatalf("New() error=%v", err)
	}
	progressBody := `{"ItemId":"item-1","MediaSourceId":"source-1","PlaySessionId":"session-1","PositionTicks":1,"IsPaused":false}`
	request := httptest.NewRequest(http.MethodPost, "/Sessions/Playing/Progress", strings.NewReader(progressBody))
	request.Header.Set(accessTokenHeader, fixtureAccessToken)
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	gateway.ServeHTTP(response, request)

	if response.Code != http.StatusNoContent {
		t.Fatalf("response=%d", response.Code)
	}
	if strings.Contains(logs.String(), "code=playback_progress_reported") || strings.Contains(logs.String(), "code=request_completed") {
		t.Fatalf("info logs=%q, want no successful progress heartbeat", logs.String())
	}

	tokenService.mu.Lock()
	tokenService.resolveErr = embytoken.ErrStoreUnavailable
	tokenService.mu.Unlock()
	failedRequest := httptest.NewRequest(http.MethodPost, "/Sessions/Playing/Progress", strings.NewReader(progressBody))
	failedRequest.Header.Set(accessTokenHeader, fixtureAccessToken)
	failedRequest.Header.Set("Content-Type", "application/json")
	gateway.ServeHTTP(httptest.NewRecorder(), failedRequest)
	if !strings.Contains(logs.String(), "code=playback_progress_failed") {
		t.Fatalf("info logs=%q, want progress failure", logs.String())
	}

	tokenService.mu.Lock()
	tokenService.resolveErr = nil
	tokenService.mu.Unlock()
	startBody := `{"ItemId":"item-1","MediaSourceId":"source-1","PlaySessionId":"session-1","PositionTicks":2}`
	startRequest := httptest.NewRequest(http.MethodPost, "/Sessions/Playing", strings.NewReader(startBody))
	startRequest.Header.Set(accessTokenHeader, fixtureAccessToken)
	startRequest.Header.Set("Content-Type", "application/json")
	gateway.ServeHTTP(httptest.NewRecorder(), startRequest)
	for _, expected := range []string{"code=playback_progress_recovered", "code=playback_session_started"} {
		if !strings.Contains(logs.String(), expected) {
			t.Fatalf("info logs=%q, want %s", logs.String(), expected)
		}
	}
}

func TestPlaybackSessionFailureTrackerBoundsAndExpiresEntries(t *testing.T) {
	tracker := newPlaybackSessionFailureTracker(1, time.Minute)
	startedAt := time.Date(2026, 8, 29, 18, 43, 54, 0, time.UTC)
	eventOne := playbackSessionEventSnapshot{playSessionID: "session-1", snapshotState: "recorded"}
	eventTwo := playbackSessionEventSnapshot{playSessionID: "session-2", snapshotState: "recorded"}

	tracker.Record(eventOne, startedAt)
	tracker.Record(eventTwo, startedAt.Add(time.Second))
	if _, recovered := tracker.Recover(eventOne, startedAt.Add(2*time.Second)); recovered {
		t.Fatal("evicted session unexpectedly recovered")
	}
	if interruptionMs, recovered := tracker.Recover(eventTwo, startedAt.Add(3*time.Second)); !recovered || interruptionMs != 2000 {
		t.Fatalf("second session recovery=(%d,%t), want (2000,true)", interruptionMs, recovered)
	}

	tracker.Record(eventOne, startedAt)
	if _, recovered := tracker.Recover(eventOne, startedAt.Add(time.Minute)); recovered {
		t.Fatal("expired session unexpectedly recovered")
	}
}

type countingPlaybackSessionBody struct {
	reader    io.Reader
	readCalls int
}

func (body *countingPlaybackSessionBody) Read(buffer []byte) (int, error) {
	body.readCalls++
	return body.reader.Read(buffer)
}

func (body *countingPlaybackSessionBody) Close() error {
	return nil
}
