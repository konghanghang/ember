package playbackgateway

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/konghang/ember/backend/internal/services/directplay"
	"github.com/konghang/ember/backend/internal/services/embytoken"
)

const fixtureRedirectURL = "https://cdn.example.invalid/video.mkv?t=1787414400"

func TestGatewayVideoRedirectUsesPlaybackProofAndNeverCallsEmby(t *testing.T) {
	var upstreamCalls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		upstreamCalls.Add(1)
		writer.WriteHeader(http.StatusInternalServerError)
	}))
	defer upstream.Close()

	directPlay := &fakeDirectPlayService{result: directplay.RedirectCandidate{
		URL: fixtureRedirectURL, ExpiresAt: time.Now().Add(time.Minute),
		ConcurrentOpenLimit: 2, TaskID: "task-1", Preexisting: true,
	}}
	var logs bytes.Buffer
	gateway := newVideoTestGateway(t, upstream.URL, &fakeTokenService{principal: fixturePrincipal()}, directPlay, nil, &logs)
	gateway.proofs.Record([]PlaybackProof{fixturePlaybackProof("mapping-1", "item-1", "source-1", "session-1")})

	request := newVideoRequest(http.MethodGet, "/emby/Videos/item-1/stream.mkv?MediaSourceId=source-1&PlaySessionId=session-1&Static=true")
	request.Header.Set("User-Agent", "Infuse-Fixture")
	request.Header.Set("Range", "bytes=0-")
	response := httptest.NewRecorder()
	gateway.ServeHTTP(response, request)

	if response.Code != http.StatusFound || response.Header().Get("Location") != fixtureRedirectURL || response.Body.Len() != 0 {
		t.Fatalf("response = status %d headers=%v body=%q", response.Code, response.Header(), response.Body.String())
	}
	if upstreamCalls.Load() != 0 {
		t.Fatalf("Emby calls = %d, want 0", upstreamCalls.Load())
	}
	requests := directPlay.snapshot()
	if len(requests) != 1 || requests[0].Path != "/mnt/media/fixture.mkv" || requests[0].Size != 1024 || requests[0].ClientUserAgent != "Infuse-Fixture" {
		t.Fatalf("direct play requests = %+v", requests)
	}
	assertSingleDecisionLog(t, logs.String(), "redirect", "direct_play", "direct_play_ready")
	for _, expected := range []string{"statusCode=302", `userId="user-1"`, `itemId="item-1"`, `taskId="task-1"`, "preexisting=true"} {
		if !strings.Contains(logs.String(), expected) {
			t.Fatalf("logs = %q, want %s", logs.String(), expected)
		}
	}
	assertSecretsAbsent(t, logs.String(), fixtureAccessToken, fixtureRedirectURL, "/mnt/media/fixture.mkv")
}

func TestGatewayRootVideoWithoutProofFallsBackToCanonicalEmbyRequest(t *testing.T) {
	const responseBody = "fixture-video-bytes"
	var upstreamCalls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		upstreamCalls.Add(1)
		if request.Method != http.MethodGet || request.URL.RequestURI() != "/emby/Videos/item-1/stream.mkv?MediaSourceId=source-1&PlaySessionId=session-1&Static=true&api=keep" {
			t.Errorf("upstream request = %s %s", request.Method, request.URL.RequestURI())
		}
		for header, expected := range map[string]string{
			accessTokenHeader: fixtureAccessToken,
			"User-Agent":      "Infuse-Fixture",
			"Range":           "bytes=128-",
			"X-Fixture":       "preserved",
		} {
			if request.Header.Get(header) != expected {
				t.Errorf("upstream %s = %q, want %q", header, request.Header.Get(header), expected)
			}
		}
		writer.Header().Set("Content-Type", "video/x-matroska")
		writer.Header().Set("Content-Range", "bytes 128-146/147")
		writer.WriteHeader(http.StatusPartialContent)
		_, _ = io.WriteString(writer, responseBody)
	}))
	defer upstream.Close()

	directPlay := &fakeDirectPlayService{}
	var logs bytes.Buffer
	gateway := newVideoTestGateway(t, upstream.URL, &fakeTokenService{principal: fixturePrincipal()}, directPlay, nil, &logs)
	request := newVideoRequest(http.MethodGet, "/Videos/item-1/stream.mkv?MediaSourceId=source-1&PlaySessionId=session-1&Static=true&api=keep")
	request.Header.Set("User-Agent", "Infuse-Fixture")
	request.Header.Set("Range", "bytes=128-")
	request.Header.Set("X-Fixture", "preserved")
	response := httptest.NewRecorder()
	gateway.ServeHTTP(response, request)

	if response.Code != http.StatusPartialContent || response.Body.String() != responseBody || response.Header().Get("Content-Range") != "bytes 128-146/147" {
		t.Fatalf("response = status %d headers=%v body=%q", response.Code, response.Header(), response.Body.String())
	}
	if upstreamCalls.Load() != 1 || len(directPlay.snapshot()) != 0 {
		t.Fatalf("calls = Emby %d DirectPlay %d", upstreamCalls.Load(), len(directPlay.snapshot()))
	}
	assertSingleDecisionLog(t, logs.String(), "fallback", "proof", "playback_proof_missing")
	for _, expected := range []string{"statusCode=206", "upstreamStatus=206"} {
		if !strings.Contains(logs.String(), expected) {
			t.Fatalf("logs = %q, want %s", logs.String(), expected)
		}
	}
	assertSecretsAbsent(t, logs.String(), fixtureAccessToken, responseBody)
}

func TestGatewayVideoDirectPlayFailureFallsBackToEmby(t *testing.T) {
	var upstreamCalls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		upstreamCalls.Add(1)
		writer.Header().Set("X-Upstream", "fallback")
		writer.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(writer, "emby-video")
	}))
	defer upstream.Close()

	directPlay := &fakeDirectPlayService{err: directplay.ErrProviderUnavailable}
	var logs bytes.Buffer
	gateway := newVideoTestGateway(t, upstream.URL, &fakeTokenService{principal: fixturePrincipal()}, directPlay, nil, &logs)
	gateway.proofs.Record([]PlaybackProof{fixturePlaybackProof("mapping-1", "item-1", "source-1", "session-1")})
	request := newVideoRequest(http.MethodHead, "/emby/Videos/item-1/stream?Container=mkv&MediaSourceId=source-1&PlaySessionId=session-1&Static=true")
	request.Header.Set("User-Agent", "Infuse-Fixture")
	response := httptest.NewRecorder()
	gateway.ServeHTTP(response, request)

	if response.Code != http.StatusOK || response.Header().Get("X-Upstream") != "fallback" {
		t.Fatalf("response = status %d headers=%v", response.Code, response.Header())
	}
	if upstreamCalls.Load() != 1 || len(directPlay.snapshot()) != 1 {
		t.Fatalf("calls = Emby %d DirectPlay %d", upstreamCalls.Load(), len(directPlay.snapshot()))
	}
	assertSingleDecisionLog(t, logs.String(), "fallback", "direct_play", "provider_unavailable")
}

func TestGatewayVideoStreamAndFileNameShapesRedirect(t *testing.T) {
	tests := []struct {
		name   string
		method string
		target string
	}{
		{name: "query container stream", method: http.MethodGet, target: "/emby/Videos/item-1/stream?Container=mkv&MediaSourceId=source-1&PlaySessionId=session-1&Static=true"},
		{name: "original file name", method: http.MethodHead, target: "/emby/Videos/item-1/fixture.mkv?MediaSourceId=source-1&PlaySessionId=session-1&Static=true"},
		{name: "root path stream", method: http.MethodGet, target: "/Videos/item-1/stream.mkv?MediaSourceId=source-1&PlaySessionId=session-1&Static=true"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var upstreamCalls atomic.Int32
			upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
				upstreamCalls.Add(1)
				writer.WriteHeader(http.StatusInternalServerError)
			}))
			defer upstream.Close()

			directPlay := &fakeDirectPlayService{result: validFixtureRedirectCandidate()}
			var logs bytes.Buffer
			gateway := newVideoTestGateway(t, upstream.URL, &fakeTokenService{principal: fixturePrincipal()}, directPlay, nil, &logs)
			gateway.proofs.Record([]PlaybackProof{fixturePlaybackProof("mapping-1", "item-1", "source-1", "session-1")})
			request := newVideoRequest(test.method, test.target)
			request.Header.Set("User-Agent", "Infuse-Fixture")
			response := httptest.NewRecorder()
			gateway.ServeHTTP(response, request)

			if response.Code != http.StatusFound || upstreamCalls.Load() != 0 || len(directPlay.snapshot()) != 1 {
				t.Fatalf("response=%d calls=Emby %d DirectPlay %d", response.Code, upstreamCalls.Load(), len(directPlay.snapshot()))
			}
			assertSingleDecisionLog(t, logs.String(), "redirect", "direct_play", "direct_play_ready")
		})
	}
}

func TestGatewayVideoAccelerationIneligibleOrInvalidFallsBack(t *testing.T) {
	tests := []struct {
		name            string
		target          string
		directPlay      DirectPlayService
		wantDirectCalls int
		wantStage       string
		wantReason      string
	}{
		{
			name: "static flag missing", target: "/emby/Videos/item-1/stream.mkv?MediaSourceId=source-1&PlaySessionId=session-1",
			directPlay: &fakeDirectPlayService{}, wantStage: "route", wantReason: "request_not_eligible",
		},
		{
			name: "direct play disabled", target: "/emby/Videos/item-1/stream.mkv?MediaSourceId=source-1&PlaySessionId=session-1&Static=true",
			wantStage: "eligibility", wantReason: "direct_play_disabled",
		},
		{
			name: "invalid candidate", target: "/emby/Videos/item-1/stream.mkv?MediaSourceId=source-1&PlaySessionId=session-1&Static=true",
			directPlay:      &fakeDirectPlayService{result: directplay.RedirectCandidate{URL: "http://unsafe.invalid/video.mkv", ExpiresAt: time.Now().Add(time.Minute), ConcurrentOpenLimit: 1}},
			wantDirectCalls: 1, wantStage: "direct_play", wantReason: "provider_protocol",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
				writer.WriteHeader(http.StatusOK)
				_, _ = io.WriteString(writer, "emby-video")
			}))
			defer upstream.Close()
			var logs bytes.Buffer
			gateway := newVideoTestGateway(t, upstream.URL, &fakeTokenService{principal: fixturePrincipal()}, test.directPlay, nil, &logs)
			gateway.proofs.Record([]PlaybackProof{fixturePlaybackProof("mapping-1", "item-1", "source-1", "session-1")})
			request := newVideoRequest(http.MethodGet, test.target)
			request.Header.Set("User-Agent", "Infuse-Fixture")
			response := httptest.NewRecorder()
			gateway.ServeHTTP(response, request)
			if response.Code != http.StatusOK || response.Body.String() != "emby-video" {
				t.Fatalf("response = status %d body=%q", response.Code, response.Body.String())
			}
			if fake, ok := test.directPlay.(*fakeDirectPlayService); ok && len(fake.snapshot()) != test.wantDirectCalls {
				t.Fatalf("DirectPlay calls = %d, want %d", len(fake.snapshot()), test.wantDirectCalls)
			}
			assertSingleDecisionLog(t, logs.String(), "fallback", test.wantStage, test.wantReason)
		})
	}
}

func TestGatewayVideoLogsExpiredAndMismatchedProofs(t *testing.T) {
	tests := []struct {
		name       string
		principal  embytoken.Principal
		expire     bool
		wantReason string
	}{
		{name: "expired", principal: fixturePrincipal(), expire: true, wantReason: "playback_proof_expired"},
		{name: "principal mismatch", principal: func() embytoken.Principal { value := fixturePrincipal(); value.User.ID = "other-user"; return value }(), wantReason: "playback_proof_mismatch"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
				writer.WriteHeader(http.StatusOK)
			}))
			defer upstream.Close()
			var logs bytes.Buffer
			gateway := newVideoTestGateway(t, upstream.URL, &fakeTokenService{principal: test.principal}, &fakeDirectPlayService{}, nil, &logs)
			now := time.Date(2026, 8, 22, 12, 0, 0, 0, time.UTC)
			gateway.proofs.now = func() time.Time { return now }
			gateway.proofs.Record([]PlaybackProof{fixturePlaybackProof("mapping-1", "item-1", "source-1", "session-1")})
			if test.expire {
				now = now.Add(defaultPlaybackProofTTL)
			}
			response := httptest.NewRecorder()
			gateway.ServeHTTP(response, newVideoRequest(http.MethodGet, "/emby/Videos/item-1/stream.mkv?MediaSourceId=source-1&PlaySessionId=session-1&Static=true"))
			if response.Code != http.StatusOK {
				t.Fatalf("response status = %d", response.Code)
			}
			assertSingleDecisionLog(t, logs.String(), "fallback", "proof", test.wantReason)
		})
	}
}

func TestGatewayVideoNonStaticManifestFallsBackWithoutDirectPlay(t *testing.T) {
	var upstreamCalls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		upstreamCalls.Add(1)
		writer.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(writer, "#EXTM3U")
	}))
	defer upstream.Close()

	directPlay := &fakeDirectPlayService{}
	var logs bytes.Buffer
	gateway := newVideoTestGateway(t, upstream.URL, &fakeTokenService{principal: fixturePrincipal()}, directPlay, nil, &logs)
	gateway.proofs.Record([]PlaybackProof{fixturePlaybackProof("mapping-1", "item-1", "source-1", "session-1")})
	request := newVideoRequest(http.MethodGet, "/emby/Videos/item-1/master.m3u8?MediaSourceId=source-1&PlaySessionId=session-1&Static=true")
	response := httptest.NewRecorder()
	gateway.ServeHTTP(response, request)

	if response.Code != http.StatusOK || upstreamCalls.Load() != 1 || len(directPlay.snapshot()) != 0 {
		t.Fatalf("response=%d calls=Emby %d DirectPlay %d", response.Code, upstreamCalls.Load(), len(directPlay.snapshot()))
	}
	assertSingleDecisionLog(t, logs.String(), "fallback", "route", "route_not_accelerated")
}

func TestGatewayVideoSecurityFailuresRejectWithoutFallback(t *testing.T) {
	tests := []struct {
		name       string
		token      string
		resolveErr error
		wantStatus int
		wantStage  string
		wantReason string
	}{
		{name: "missing token", wantStatus: http.StatusUnauthorized, wantStage: "identity", wantReason: "token_missing"},
		{name: "revoked token", token: fixtureAccessToken, resolveErr: embytoken.ErrTokenRevoked, wantStatus: http.StatusUnauthorized, wantStage: "identity", wantReason: "token_revoked"},
		{name: "unavailable user", token: fixtureAccessToken, resolveErr: embytoken.ErrUserUnavailable, wantStatus: http.StatusForbidden, wantStage: "user_state", wantReason: "user_unavailable"},
		{name: "expired user", token: fixtureAccessToken, resolveErr: embytoken.ErrUserExpired, wantStatus: http.StatusForbidden, wantStage: "user_state", wantReason: "user_expired"},
		{name: "identity store unavailable", token: fixtureAccessToken, resolveErr: errors.New("store failed with " + fixtureAccessToken), wantStatus: http.StatusServiceUnavailable, wantStage: "identity", wantReason: "identity_store_unavailable"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var upstreamCalls atomic.Int32
			upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
				upstreamCalls.Add(1)
				writer.WriteHeader(http.StatusNoContent)
			}))
			defer upstream.Close()

			directPlay := &fakeDirectPlayService{}
			var logs bytes.Buffer
			gateway := newVideoTestGateway(t, upstream.URL, &fakeTokenService{resolveErr: test.resolveErr}, directPlay, nil, &logs)
			request := httptest.NewRequest(http.MethodGet, "/emby/Videos/item-1/stream.mkv?MediaSourceId=source-1&PlaySessionId=session-1&Static=true", nil)
			if test.token != "" {
				request.Header.Set(accessTokenHeader, test.token)
			}
			response := httptest.NewRecorder()
			gateway.ServeHTTP(response, request)

			if response.Code != test.wantStatus || response.Body.Len() != 0 {
				t.Fatalf("response = status %d body=%q", response.Code, response.Body.String())
			}
			if upstreamCalls.Load() != 0 || len(directPlay.snapshot()) != 0 {
				t.Fatalf("calls = Emby %d DirectPlay %d", upstreamCalls.Load(), len(directPlay.snapshot()))
			}
			assertSingleDecisionLog(t, logs.String(), "reject", test.wantStage, test.wantReason)
			assertSecretsAbsent(t, logs.String(), fixtureAccessToken)
		})
	}
}

func TestGatewayVideoFallbackTransportFailureProducesOneSanitizedDecision(t *testing.T) {
	var logs bytes.Buffer
	transportSecret := "transport-secret"
	gateway := newVideoTestGateway(t, "http://emby.invalid", &fakeTokenService{principal: fixturePrincipal()}, &fakeDirectPlayService{}, roundTripFunc(func(*http.Request) (*http.Response, error) {
		return nil, errors.New("dial failed with " + transportSecret)
	}), &logs)
	request := newVideoRequest(http.MethodGet, "/emby/Videos/item-1/stream.mkv?MediaSourceId=source-1&PlaySessionId=session-1&Static=true")
	response := httptest.NewRecorder()
	gateway.ServeHTTP(response, request)

	if response.Code != http.StatusBadGateway || response.Body.Len() != 0 {
		t.Fatalf("response = status %d body=%q", response.Code, response.Body.String())
	}
	assertSingleDecisionLog(t, logs.String(), "fallback", "proof", "playback_proof_missing")
	if !strings.Contains(logs.String(), "statusCode=502") || !strings.Contains(logs.String(), "proxyErrorCode=upstream_unavailable") || strings.Contains(logs.String(), "code=upstream_unavailable") {
		t.Fatalf("logs = %q", logs.String())
	}
	assertSecretsAbsent(t, logs.String(), fixtureAccessToken, transportSecret, "emby.invalid")
}

func TestDirectPlayReasonCodeIsStable(t *testing.T) {
	tests := []struct {
		err  error
		want string
	}{
		{directplay.ErrInvalidRequest, "invalid_request"},
		{directplay.ErrPathNotMapped, "path_not_mapped"},
		{directplay.ErrAccountUnavailable, "account_unavailable"},
		{directplay.ErrAccountsSame, "accounts_same"},
		{directplay.ErrProviderUnavailable, "provider_unavailable"},
		{directplay.ErrProviderProtocol, "provider_protocol"},
		{directplay.ErrRapidUploadUnavailable, "rapid_upload_unavailable"},
		{directplay.ErrTargetUnavailable, "target_unavailable"},
		{directplay.ErrDownloadIncompatible, "download_incompatible"},
		{directplay.ErrStoreUnavailable, "store_unavailable"},
		{directplay.ErrLockUnavailable, "lock_unavailable"},
		{errors.New("unknown"), "provider_protocol"},
	}
	for _, test := range tests {
		if got := directPlayReasonCode(test.err); got != test.want {
			t.Fatalf("directPlayReasonCode(%v) = %q, want %q", test.err, got, test.want)
		}
	}
}

func TestClassifyRouteRecognizesOnlyExactVideoShapes(t *testing.T) {
	tests := []struct {
		method string
		path   string
		want   routeKind
	}{
		{http.MethodGet, "/emby/Videos/item-1/stream", routeVideo},
		{http.MethodHead, "/emby/Videos/item-1/stream.mkv", routeVideo},
		{http.MethodGet, "/emby/Videos/item-1/fixture.mkv", routeVideo},
		{http.MethodPost, "/emby/Videos/item-1/stream.mkv", routeProtected},
		{http.MethodGet, "/emby/Videos/item-1/stream.mkv/", routeProtected},
		{http.MethodGet, "/emby/Videos/item%2D1/stream.mkv", routeProtected},
		{http.MethodGet, "/emby/Videos/item-1/source-1/Subtitles/0/Stream.srt", routeProtected},
	}
	for _, test := range tests {
		request := httptest.NewRequest(test.method, test.path, nil)
		if got := classifyRoute(request); got != test.want {
			t.Fatalf("classifyRoute(%s %s) = %v, want %v", test.method, test.path, got, test.want)
		}
	}
}

func newVideoRequest(method, target string) *http.Request {
	request := httptest.NewRequest(method, target, nil)
	request.Header.Set(accessTokenHeader, fixtureAccessToken)
	return request
}

func validFixtureRedirectCandidate() directplay.RedirectCandidate {
	return directplay.RedirectCandidate{
		URL: fixtureRedirectURL, ExpiresAt: time.Now().Add(time.Minute), ConcurrentOpenLimit: 2,
	}
}

func newVideoTestGateway(
	t *testing.T,
	upstreamRawURL string,
	tokenService TokenService,
	directPlay DirectPlayService,
	transport http.RoundTripper,
	logs *bytes.Buffer,
) *Gateway {
	t.Helper()
	upstreamURL, err := url.Parse(upstreamRawURL)
	if err != nil {
		t.Fatalf("parse upstream URL: %v", err)
	}
	gateway, err := New(Config{
		Upstream: upstreamURL, TokenService: tokenService, DirectPlayService: directPlay,
		Transport: transport, Logger: log.New(logs, "", 0),
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	return gateway
}

func assertSingleDecisionLog(t *testing.T, logs, decision, stage, reasonCode string) {
	t.Helper()
	if strings.Count(logs, "decision=") != 1 || !strings.Contains(logs, "decision="+decision) ||
		!strings.Contains(logs, "stage="+stage) || !strings.Contains(logs, "reasonCode="+reasonCode) {
		t.Fatalf("decision logs = %q", logs)
	}
}

type fakeDirectPlayService struct {
	mu       sync.Mutex
	requests []directplay.MediaPathResolveRequest
	result   directplay.RedirectCandidate
	err      error
}

func (service *fakeDirectPlayService) ResolveMediaPath(
	_ context.Context,
	request directplay.MediaPathResolveRequest,
) (directplay.RedirectCandidate, error) {
	service.mu.Lock()
	defer service.mu.Unlock()
	service.requests = append(service.requests, request)
	return service.result, service.err
}

func (service *fakeDirectPlayService) snapshot() []directplay.MediaPathResolveRequest {
	service.mu.Lock()
	defer service.mu.Unlock()
	return append([]directplay.MediaPathResolveRequest(nil), service.requests...)
}
