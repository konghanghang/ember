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
	"github.com/konghang/ember/backend/internal/services/p115quota"
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
		PathMapping: directplay.MediaPathMapping{
			OriginalPath: "/mnt/media/fixture.mkv", EmbyPathPrefix: "/mnt/media",
			SourceRootID: "100", RelativePath: "fixture.mkv",
		},
		Routing: directplay.RoutingDiagnostics{
			Routed: true, PlaybackMode: "personal", PlaybackAccountOwner: "current_user",
			AccountLimitsAvailable:         true,
			ConfiguredMaxConcurrentStreams: 4, EffectiveMaxConcurrentStreams: 3, SimultaneousStreamLimit: 3,
			LeaseUsageAvailable: true,
			AccountUsage:        p115quota.LeaseUsage{ReservedStreams: 1, ActiveStreams: 1, OccupiedStreams: 2},
			UserUsage:           p115quota.LeaseUsage{ReservedStreams: 1, ActiveStreams: 1, OccupiedStreams: 2},
			TransferChecked:     true, TransferUsageAvailable: true,
			TransferUsage:       p115quota.TransferUsage{HourlyUsed: 2, DailyUsed: 7},
			TransferHourlyLimit: 5, TransferDailyLimit: 10,
		},
	}}
	var logs bytes.Buffer
	gateway := newVideoTestGateway(t, upstream.URL, &fakeTokenService{principal: fixturePrincipal()}, directPlay, nil, &logs)
	gateway.proofs.Record([]PlaybackProof{fixturePlaybackProof("mapping-1", "item-1", "source-1", "session-1")})

	request := newVideoRequest(http.MethodGet, "/emby/Videos/item-1/stream.mkv?MediaSourceId=source-1&PlaySessionId=session-1&Static=true&transferAttemptId=transfer-attempt-secret&redisKey=%7Bp115%7D%3Aleases%3Aaccount%3Asecret")
	request.Header.Set("User-Agent", "Infuse-Fixture")
	request.Header.Set("Range", "bytes=0-")
	request.Header.Set("Cookie", "UID=provider-uid-secret; token=cookie-secret")
	response := httptest.NewRecorder()
	gateway.ServeHTTP(response, request)

	if response.Code != http.StatusFound || response.Header().Get("Location") != fixtureRedirectURL || response.Body.Len() != 0 {
		t.Fatalf("response = status %d headers=%v body=%q", response.Code, response.Header(), response.Body.String())
	}
	if upstreamCalls.Load() != 0 {
		t.Fatalf("Emby calls = %d, want 0", upstreamCalls.Load())
	}
	requests := directPlay.snapshot()
	if len(requests) != 1 || requests[0].Path != "/mnt/media/fixture.mkv" || requests[0].ClientUserAgent != "Infuse-Fixture" ||
		requests[0].Method != http.MethodGet || requests[0].UserID != "user-1" || requests[0].MappingID != "mapping-1" ||
		requests[0].DeviceID != "device-1" || requests[0].PlaySessionID != "session-1" {
		t.Fatalf("direct play requests = %+v", requests)
	}
	assertSingleDecisionLog(t, logs.String(), "redirect", "direct_play", "direct_play_ready")
	for _, expected := range []string{
		"level=info", "code=direct_play_redirect", `message="115直链成功"`, "result=success",
		"statusCode=302", "target=p115", "targetState=reused", `userId="user-1"`, `itemId="item-1"`,
		`taskId="task-1"`, "preexisting=true",
		`mediaPath="/mnt/media/fixture.mkv"`, `embyPathPrefix="/mnt/media"`,
		`sourceRootId="100"`, `mappedRelativePath="fixture.mkv"`,
		"playbackMode=personal", "playbackAccountOwner=current_user",
		"accountReservedStreams=1", "accountActiveStreams=1", "accountOccupiedStreams=2",
		"accountConfiguredStreamLimit=4", "accountEffectiveStreamLimit=3", "simultaneousStreamLimit=3",
		"userReservedStreams=1", "userActiveStreams=1", "userOccupiedStreams=2",
		"transferHourlyUsed=2", "transferHourlyLimit=5", "transferDailyUsed=7", "transferDailyLimit=10",
	} {
		if !strings.Contains(logs.String(), expected) {
			t.Fatalf("logs = %q, want %s", logs.String(), expected)
		}
	}
	for _, unexpected := range []string{"fallbackSource=", "upstreamStatus=", "proxyErrorCode="} {
		if strings.Contains(logs.String(), unexpected) {
			t.Fatalf("logs = %q, unexpected %s", logs.String(), unexpected)
		}
	}
	assertSecretsAbsent(t, logs.String(), fixtureAccessToken, fixtureRedirectURL, "provider-uid-secret", "cookie-secret", "session-1", "{p115}:leases:account:", "transfer-attempt-secret")
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

	directPlay := &fakeDirectPlayService{
		result: directplay.RedirectCandidate{PathMapping: directplay.MediaPathMapping{
			OriginalPath: "/mnt/media/fixture.mkv", EmbyPathPrefix: "/mnt/media",
			SourceRootID: "100", RelativePath: "fixture.mkv",
		}},
		err: fixtureDirectPlayFailure{
			cause:   directplay.ErrProviderUnavailable,
			context: directplay.FailureContext{ProviderOperation: "get_download_url"},
		},
	}
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
	for _, expected := range []string{
		"level=info", "code=direct_play_fallback", `message="115直链失败，Emby回退成功"`,
		"directPlayResult=failure", "providerOperation=get_download_url", "fallbackResult=success", "statusCode=200",
		`mediaPath="/mnt/media/fixture.mkv"`, `embyPathPrefix="/mnt/media"`,
		`sourceRootId="100"`, `mappedRelativePath="fixture.mkv"`,
	} {
		if !strings.Contains(logs.String(), expected) {
			t.Fatalf("logs=%q, want %s", logs.String(), expected)
		}
	}
}

func TestGatewayVideoDirectPlayFailureAlwaysUsesAuthoritativeEmbyFallback(t *testing.T) {
	var upstreamCalls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		upstreamCalls.Add(1)
		if request.Header.Get("Range") != "bytes=2-5" || request.Header.Get("X-Fixture") != "preserved" {
			t.Fatalf("upstream headers=%v", request.Header)
		}
		writer.Header().Set("Content-Range", "bytes 2-5/10")
		writer.Header().Set("Cache-Control", "public, max-age=60")
		writer.WriteHeader(http.StatusPartialContent)
		_, _ = io.WriteString(writer, "emby")
	}))
	defer upstream.Close()

	directPlay := &fakeDirectPlayService{
		result: directplay.RedirectCandidate{PathMapping: directplay.MediaPathMapping{
			OriginalPath: "/mnt/media/Media/fixture.mkv", EmbyPathPrefix: "/mnt/media",
			SourceRootID: "100", RelativePath: "Media/fixture.mkv",
		}},
		err: directplay.ErrAccountUnavailable,
	}
	var logs bytes.Buffer
	gateway := newVideoTestGateway(t, upstream.URL, &fakeTokenService{principal: fixturePrincipal()}, directPlay, nil, &logs)
	gateway.proofs.Record([]PlaybackProof{fixturePlaybackProof("mapping-1", "item-1", "source-1", "session-1")})
	request := newVideoRequest(http.MethodGet, "/emby/Videos/item-1/stream.mkv?MediaSourceId=source-1&PlaySessionId=session-1&Static=true")
	request.Header.Set("User-Agent", "Infuse-Fixture")
	request.Header.Set("Range", "bytes=2-5")
	request.Header.Set("X-Fixture", "preserved")
	response := httptest.NewRecorder()
	gateway.ServeHTTP(response, request)

	if response.Code != http.StatusPartialContent || response.Body.String() != "emby" || upstreamCalls.Load() != 1 {
		t.Fatalf("response=%d body=%q upstreamCalls=%d", response.Code, response.Body.String(), upstreamCalls.Load())
	}
	if response.Header().Get("Content-Range") != "bytes 2-5/10" || response.Header().Get("Cache-Control") != "public, max-age=60" {
		t.Fatalf("response headers=%v", response.Header())
	}
	assertSingleDecisionLog(t, logs.String(), "fallback", "direct_play", "account_unavailable")
	for _, expected := range []string{`message="115直链失败，Emby回退成功"`, "directPlayResult=failure", "fallbackTarget=emby", "fallbackResult=success", "statusCode=206"} {
		if !strings.Contains(logs.String(), expected) {
			t.Fatalf("logs=%q, want %s", logs.String(), expected)
		}
	}
}

func TestGatewayVideoDirectPlayCancellationTerminatesWithoutFallback(t *testing.T) {
	for _, test := range []struct {
		name       string
		directErr  error
		wantStatus int
		wantReason string
	}{
		{name: "canceled", directErr: context.Canceled, wantStatus: statusClientClosedRequest, wantReason: "request_canceled"},
		{name: "deadline", directErr: context.DeadlineExceeded, wantStatus: http.StatusGatewayTimeout, wantReason: "request_deadline_exceeded"},
	} {
		t.Run(test.name, func(t *testing.T) {
			var upstreamCalls atomic.Int32
			upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
				upstreamCalls.Add(1)
				writer.WriteHeader(http.StatusNoContent)
			}))
			defer upstream.Close()

			directPlay := &fakeDirectPlayService{
				result: directplay.RedirectCandidate{PathMapping: directplay.MediaPathMapping{RelativePath: "Media/fixture.mkv"}},
				err:    test.directErr,
			}
			var logs bytes.Buffer
			gateway := newVideoTestGateway(t, upstream.URL, &fakeTokenService{principal: fixturePrincipal()}, directPlay, nil, &logs)
			gateway.proofs.Record([]PlaybackProof{fixturePlaybackProof("mapping-1", "item-1", "source-1", "session-1")})
			response := httptest.NewRecorder()
			gateway.ServeHTTP(response, newVideoRequest(
				http.MethodGet,
				"/emby/Videos/item-1/stream.mkv?MediaSourceId=source-1&PlaySessionId=session-1&Static=true",
			))

			if response.Code != test.wantStatus || response.Body.Len() != 0 || upstreamCalls.Load() != 0 {
				t.Fatalf("response=%d body=%q upstreamCalls=%d", response.Code, response.Body.String(), upstreamCalls.Load())
			}
			assertSingleDecisionLog(t, logs.String(), "reject", "direct_play", test.wantReason)
			if strings.Contains(logs.String(), "provider_protocol") || strings.Contains(logs.String(), "fallbackTarget=") {
				t.Fatalf("terminal request was logged as fallback: %q", logs.String())
			}
		})
	}
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
		{name: "lowercase path and query", method: http.MethodGet, target: "/videos/item-1/STREAM.MKV?mediasourceid=source-1&playsessionid=session-1&static=TRUE"},
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

func TestGatewayResolvesPlaybackInfoForIncompleteInfuseStreamFallback(t *testing.T) {
	var itemCalls atomic.Int32
	var playbackInfoCalls atomic.Int32
	var videoCalls atomic.Int32
	itemBody := []byte(`{"Id":"item-1","Container":"mkv","MediaSources":[{"Id":"source-1","Container":"mkv"}]}`)
	encodedItemBody := deflateFixture(t, itemBody, false)
	upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/emby/Users/emby-user-1/Items/item-1":
			itemCalls.Add(1)
			writer.Header().Set("Content-Type", "application/json")
			writer.Header().Set("Content-Encoding", "deflate")
			_, _ = writer.Write(encodedItemBody)
		case "/emby/Videos/item-1/stream.mkv":
			videoCalls.Add(1)
			if request.URL.Query().Get("MediaSourceId") != "source-1" || request.URL.Query().Get("Static") != "true" ||
				request.URL.Query().Get("Container") != "" || request.URL.Query().Get("PlaySessionId") != "" {
				t.Fatalf("upstream video query=%q", request.URL.RawQuery)
			}
			writer.WriteHeader(http.StatusOK)
			_, _ = io.WriteString(writer, "fixture-video")
		case "/emby/Items/item-1/PlaybackInfo":
			playbackInfoCalls.Add(1)
			if request.Method != http.MethodGet || request.URL.Query().Get("UserId") != "emby-user-1" || request.Header.Get(accessTokenHeader) != fixtureAccessToken {
				t.Fatalf("PlaybackInfo request=%s %s headers=%v", request.Method, request.URL.RequestURI(), request.Header)
			}
			writer.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(writer, `{"MediaSources":[{"Id":"source-1","ItemId":"item-1","Path":"/private/media/one.mkv","Size":1024,"Container":"mkv","SupportsDirectPlay":true}],"PlaySessionId":"session-1"}`)
		default:
			t.Fatalf("unexpected upstream path=%s", request.URL.Path)
		}
	}))
	defer upstream.Close()

	var logs bytes.Buffer
	gateway := newVideoTestGateway(t, upstream.URL, &fakeTokenService{principal: fixturePrincipal()}, nil, nil, &logs)

	itemRequest := httptest.NewRequest(http.MethodGet, "/Users/emby-user-1/Items/item-1?Fields=MediaSources", nil)
	itemRequest.Header.Set(accessTokenHeader, fixtureAccessToken)
	itemRequest.Header.Set("Accept-Encoding", "deflate")
	itemResponse := httptest.NewRecorder()
	gateway.ServeHTTP(itemResponse, itemRequest)
	if itemResponse.Code != http.StatusOK || !bytes.Equal(itemResponse.Body.Bytes(), encodedItemBody) || itemResponse.Header().Get("Content-Encoding") != "deflate" {
		t.Fatalf("item response=%d encoding=%q bodyLength=%d", itemResponse.Code, itemResponse.Header().Get("Content-Encoding"), itemResponse.Body.Len())
	}

	videoRequest := httptest.NewRequest(http.MethodGet, "/Videos/item-1/stream?MediaSourceId=source-1&Static=true", nil)
	videoRequest.Header.Set(accessTokenHeader, fixtureAccessToken)
	videoResponse := httptest.NewRecorder()
	gateway.ServeHTTP(videoResponse, videoRequest)

	if videoResponse.Code != http.StatusOK || videoResponse.Body.String() != "fixture-video" || itemCalls.Load() != 1 || playbackInfoCalls.Load() != 1 || videoCalls.Load() != 1 {
		t.Fatalf("video response=%d body=%q itemCalls=%d playbackInfoCalls=%d videoCalls=%d", videoResponse.Code, videoResponse.Body.String(), itemCalls.Load(), playbackInfoCalls.Load(), videoCalls.Load())
	}
	assertSingleDecisionLog(t, logs.String(), "fallback", "eligibility", "direct_play_disabled")
	if !strings.Contains(logs.String(), "fallbackSource=playback_info_extension_stream") {
		t.Fatalf("logs=%q", logs.String())
	}
	if !strings.Contains(logs.String(), "code=playback_info_resolved_on_demand") {
		t.Fatalf("logs=%q", logs.String())
	}
	assertSecretsAbsent(t, logs.String(), fixtureAccessToken, "fixture-video")
}

func TestGatewayUsesPlaybackInfoDirectStreamURLWhenDirectPlayUnavailable(t *testing.T) {
	const directStreamToken = "fixture-direct-stream-token"
	var playbackInfoCalls atomic.Int32
	var authoritativeStreamCalls atomic.Int32
	var originalStreamCalls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/emby/Items/item-1/PlaybackInfo":
			playbackInfoCalls.Add(1)
			if request.Header.Get(accessTokenHeader) != fixtureAccessToken || request.URL.Query().Has("api_key") {
				t.Fatalf("PlaybackInfo authentication=%s headers=%v", request.URL.RequestURI(), request.Header)
			}
			writer.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(writer, `{"MediaSources":[{"Id":"source-1","ItemId":"item-1","Path":"/private/media/one.mkv","Size":1024,"Container":"mkv","SupportsDirectPlay":true,"SupportsDirectStream":true,"DirectStreamUrl":"/Videos/item-1/stream.mkv?Static=true&MediaSourceId=source-1&api_key=`+directStreamToken+`"}],"PlaySessionId":"session-1"}`)
		case "/emby/Videos/item-1/stream.mkv":
			authoritativeStreamCalls.Add(1)
			if request.URL.Query().Get("Static") != "true" || request.URL.Query().Get("MediaSourceId") != "source-1" ||
				request.URL.Query().Get("StartTimeTicks") != "123" || request.URL.Query().Has("api_key") ||
				request.Header.Get(accessTokenHeader) != fixtureAccessToken || request.Header.Get("Range") != "bytes=123-" ||
				request.Header.Get("X-Fixture") != "preserved" {
				t.Fatalf("authoritative fallback request=%s headers=%v", request.URL.RequestURI(), request.Header)
			}
			writer.WriteHeader(http.StatusPartialContent)
			_, _ = io.WriteString(writer, "fixture-local-video")
		case "/emby/Videos/item-1/stream":
			originalStreamCalls.Add(1)
			writer.WriteHeader(http.StatusNotFound)
		default:
			t.Fatalf("unexpected upstream path=%s", request.URL.Path)
		}
	}))
	defer upstream.Close()

	directPlay := &fakeDirectPlayService{err: fixtureDirectPlayFailure{
		cause:   directplay.ErrAccountUnavailable,
		context: directplay.FailureContext{AccountRole: "source"},
	}}
	var logs bytes.Buffer
	gateway := newVideoTestGateway(t, upstream.URL, &fakeTokenService{principal: fixturePrincipal()}, directPlay, nil, &logs)
	request := httptest.NewRequest(
		http.MethodGet,
		"/Videos/item-1/stream?MediaSourceId=source-1&Static=true&StartTimeTicks=123&api_key="+fixtureAccessToken,
		nil,
	)
	request.Header.Set("Range", "bytes=123-")
	request.Header.Set("X-Fixture", "preserved")
	response := httptest.NewRecorder()
	gateway.ServeHTTP(response, request)

	if response.Code != http.StatusPartialContent || response.Body.String() != "fixture-local-video" ||
		playbackInfoCalls.Load() != 1 || authoritativeStreamCalls.Load() != 1 || originalStreamCalls.Load() != 0 {
		t.Fatalf(
			"response=%d body=%q playbackInfoCalls=%d authoritativeStreamCalls=%d originalStreamCalls=%d",
			response.Code,
			response.Body.String(),
			playbackInfoCalls.Load(),
			authoritativeStreamCalls.Load(),
			originalStreamCalls.Load(),
		)
	}
	assertSingleDecisionLog(t, logs.String(), "fallback", "direct_play", "account_unavailable")
	for _, expected := range []string{
		`message="115直链失败，Emby回退成功"`, "accountRole=source",
		"fallbackSource=playback_info_direct_stream", "fallbackResult=success", "statusCode=206",
	} {
		if !strings.Contains(logs.String(), expected) {
			t.Fatalf("logs=%q, want %s", logs.String(), expected)
		}
	}
	if !strings.Contains(logs.String(), `mediaPath="/private/media/one.mkv"`) {
		t.Fatalf("logs=%q, want complete media path", logs.String())
	}
	assertSecretsAbsent(t, logs.String(), fixtureAccessToken, directStreamToken)
}

func TestGatewayOnDemandPlaybackInfoWithoutProofLogsPathInFallbackDecision(t *testing.T) {
	const mediaPath = "/mnt/cloudNAS/115lifetime/Media/fixture.mkv"
	var playbackInfoCalls atomic.Int32
	var streamCalls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/emby/Items/item-1/PlaybackInfo":
			playbackInfoCalls.Add(1)
			writer.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(writer, `{"MediaSources":[{"Id":"source-1","ItemId":"item-1","Path":"`+mediaPath+`","Size":1024,"Container":"mkv","SupportsDirectPlay":false,"SupportsDirectStream":true,"DirectStreamUrl":"/Videos/item-1/stream.mkv?Static=true&MediaSourceId=source-1"}],"PlaySessionId":"session-1"}`)
		case "/emby/Videos/item-1/stream.mkv":
			streamCalls.Add(1)
			writer.WriteHeader(http.StatusNotFound)
		default:
			t.Fatalf("unexpected upstream path=%s", request.URL.Path)
		}
	}))
	defer upstream.Close()

	directPlay := &fakeDirectPlayService{}
	var logs bytes.Buffer
	gateway := newVideoTestGateway(t, upstream.URL, &fakeTokenService{principal: fixturePrincipal()}, directPlay, nil, &logs)
	request := httptest.NewRequest(http.MethodGet, "/Videos/item-1/stream?MediaSourceId=source-1&Static=true", nil)
	request.Header.Set(accessTokenHeader, fixtureAccessToken)
	response := httptest.NewRecorder()
	gateway.ServeHTTP(response, request)

	if response.Code != http.StatusNotFound || playbackInfoCalls.Load() != 1 || streamCalls.Load() != 1 || len(directPlay.snapshot()) != 0 {
		t.Fatalf("response=%d playbackInfoCalls=%d streamCalls=%d directPlayCalls=%d", response.Code, playbackInfoCalls.Load(), streamCalls.Load(), len(directPlay.snapshot()))
	}
	assertSingleDecisionLog(t, logs.String(), "fallback", "proof", "playback_proof_missing")
	for _, expected := range []string{
		"code=playback_fallback", `message="Emby回退失败"`, "fallbackResult=failure", "statusCode=404",
		`code=playback_info_media_source_observed mappingId="mapping-1" itemId="item-1" mediaSourceId="source-1" mediaPath="` + mediaPath + `"`,
		"pathPresent=true", "sizePresent=true", "supportsDirectPlay=false", "supportsDirectStream=true",
		"proofAccepted=false", "proofRejectReason=direct_play_unsupported",
		"fallbackSource=playback_info_direct_stream", `mediaPath="` + mediaPath + `"`,
	} {
		if !strings.Contains(logs.String(), expected) {
			t.Fatalf("logs=%q, want %s", logs.String(), expected)
		}
	}
	for _, unexpected := range []string{"embyPathPrefix=", "sourceRootId=", "mappedRelativePath="} {
		if strings.Contains(logs.String(), unexpected) {
			t.Fatalf("logs=%q, unexpected empty mapping field %s", logs.String(), unexpected)
		}
	}
	assertSecretsAbsent(t, logs.String(), fixtureAccessToken)
}

func TestGatewayOnDemandPlaybackInfoCanAuthorizeDirectPlay(t *testing.T) {
	var playbackInfoCalls atomic.Int32
	var videoCalls atomic.Int32
	playbackInfoBody := []byte(`{"MediaSources":[{"Id":"mediasource_item-1","ItemId":"item-1","Path":"/private/media/one.mkv","Size":1024,"Container":"mkv","SupportsDirectPlay":true}],"PlaySessionId":"session-1"}`)
	encodedPlaybackInfoBody := deflateFixture(t, playbackInfoBody, false)
	upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/emby/Items/item-1/PlaybackInfo":
			playbackInfoCalls.Add(1)
			if request.Header.Get(embyAuthorizationHeader) != mediaBrowserAuthorizationWithToken(fixtureAccessToken) || request.Header.Get(accessTokenHeader) != "" {
				t.Fatal("on-demand PlaybackInfo authentication headers changed")
			}
			writer.Header().Set("Content-Type", "application/json")
			writer.Header().Set("Content-Encoding", "deflate")
			_, _ = writer.Write(encodedPlaybackInfoBody)
		case "/emby/Videos/item-1/stream":
			videoCalls.Add(1)
			writer.WriteHeader(http.StatusInternalServerError)
		default:
			t.Fatalf("unexpected upstream path=%s", request.URL.Path)
		}
	}))
	defer upstream.Close()

	directPlay := &fakeDirectPlayService{result: validFixtureRedirectCandidate()}
	var logs bytes.Buffer
	gateway := newVideoTestGateway(t, upstream.URL, &fakeTokenService{principal: fixturePrincipal()}, directPlay, nil, &logs)
	request := httptest.NewRequest(http.MethodGet, "/Videos/item-1/stream?MediaSourceId=mediasource_item-1&Static=true", nil)
	request.Header.Set(embyAuthorizationHeader, mediaBrowserAuthorizationWithToken(fixtureAccessToken))
	request.Header.Set("Accept-Encoding", "deflate")
	response := httptest.NewRecorder()
	gateway.ServeHTTP(response, request)

	if response.Code != http.StatusFound || response.Header().Get("Location") != fixtureRedirectURL || playbackInfoCalls.Load() != 1 || videoCalls.Load() != 0 {
		t.Fatalf("response=%d location=%q playbackInfoCalls=%d videoCalls=%d", response.Code, response.Header().Get("Location"), playbackInfoCalls.Load(), videoCalls.Load())
	}
	requests := directPlay.snapshot()
	if len(requests) != 1 || requests[0].Path != "/private/media/one.mkv" {
		t.Fatalf("DirectPlay requests=%+v", requests)
	}
	assertSingleDecisionLog(t, logs.String(), "redirect", "direct_play", "direct_play_ready")
	if !strings.Contains(logs.String(), `mediaPath="/private/media/one.mkv"`) {
		t.Fatalf("logs=%q, want complete media path", logs.String())
	}
	assertSecretsAbsent(t, logs.String(), fixtureAccessToken)
}

func TestGatewayOnDemandPlaybackInfoWithZeroSizeCanAuthorizeDirectPlay(t *testing.T) {
	const mediaPath = "/mnt/cloudNAS/115lifetime/video/fixture.mkv"
	var playbackInfoCalls atomic.Int32
	var videoCalls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/emby/Items/item-1/PlaybackInfo":
			playbackInfoCalls.Add(1)
			writer.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(writer, `{"MediaSources":[{"Id":"source-1","ItemId":"item-1","Path":"`+mediaPath+`","Size":0,"Container":"mkv","SupportsDirectPlay":true,"SupportsDirectStream":true}],"PlaySessionId":"session-1"}`)
		default:
			videoCalls.Add(1)
			writer.WriteHeader(http.StatusNotFound)
		}
	}))
	defer upstream.Close()

	directPlay := &fakeDirectPlayService{result: validFixtureRedirectCandidate()}
	var logs bytes.Buffer
	gateway := newVideoTestGateway(t, upstream.URL, &fakeTokenService{principal: fixturePrincipal()}, directPlay, nil, &logs)
	request := httptest.NewRequest(http.MethodGet, "/Videos/item-1/stream?MediaSourceId=source-1&Static=true", nil)
	request.Header.Set(accessTokenHeader, fixtureAccessToken)
	request.Header.Set("User-Agent", "Infuse-Fixture")
	response := httptest.NewRecorder()
	gateway.ServeHTTP(response, request)

	if response.Code != http.StatusFound || playbackInfoCalls.Load() != 1 || videoCalls.Load() != 0 {
		t.Fatalf("response=%d playbackInfoCalls=%d videoCalls=%d", response.Code, playbackInfoCalls.Load(), videoCalls.Load())
	}
	requests := directPlay.snapshot()
	if len(requests) != 1 || requests[0].Path != mediaPath {
		t.Fatalf("DirectPlay requests=%+v", requests)
	}
	for _, expected := range []string{
		"sizePresent=true", "size=0", "proofAccepted=true", "proofRejectReason=none",
		"decision=redirect", "statusCode=302", `mediaPath="` + mediaPath + `"`,
	} {
		if !strings.Contains(logs.String(), expected) {
			t.Fatalf("logs=%q, want %s", logs.String(), expected)
		}
	}
	assertSecretsAbsent(t, logs.String(), fixtureAccessToken, fixtureRedirectURL)
}

func TestGatewayOnDemandPlaybackInfoFailureKeepsOriginalFallback(t *testing.T) {
	var playbackInfoCalls atomic.Int32
	var videoCalls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/emby/Items/item-1/PlaybackInfo":
			playbackInfoCalls.Add(1)
			writer.WriteHeader(http.StatusServiceUnavailable)
		case "/emby/Videos/item-1/stream":
			videoCalls.Add(1)
			if request.URL.Query().Get("Container") != "" || request.URL.Query().Get("PlaySessionId") != "" {
				t.Fatalf("failed resolver changed fallback query=%q", request.URL.RawQuery)
			}
			writer.WriteHeader(http.StatusNotFound)
		default:
			t.Fatalf("unexpected upstream path=%s", request.URL.Path)
		}
	}))
	defer upstream.Close()

	directPlay := &fakeDirectPlayService{}
	var logs bytes.Buffer
	gateway := newVideoTestGateway(t, upstream.URL, &fakeTokenService{principal: fixturePrincipal()}, directPlay, nil, &logs)
	request := newVideoRequest(http.MethodGet, "/Videos/item-1/stream?MediaSourceId=source-1&Static=true")
	response := httptest.NewRecorder()
	gateway.ServeHTTP(response, request)

	if response.Code != http.StatusNotFound || playbackInfoCalls.Load() != 1 || videoCalls.Load() != 1 || len(directPlay.snapshot()) != 0 {
		t.Fatalf("response=%d playbackInfoCalls=%d videoCalls=%d directPlayCalls=%d", response.Code, playbackInfoCalls.Load(), videoCalls.Load(), len(directPlay.snapshot()))
	}
	if !strings.Contains(logs.String(), "code=playback_info_resolve_failed reasonCode=upstream_status mappingId=mapping-1 itemId=item-1 statusCode=503") {
		t.Fatalf("logs=%q", logs.String())
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

type fixtureDirectPlayFailure struct {
	cause   error
	context directplay.FailureContext
}

func (failure fixtureDirectPlayFailure) Error() string {
	return failure.cause.Error()
}

func (failure fixtureDirectPlayFailure) Unwrap() error {
	return failure.cause
}

func (failure fixtureDirectPlayFailure) DirectPlayFailureContext() directplay.FailureContext {
	return failure.context
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

func TestAppendRoutingDecisionContextOmitsUnavailableUsageAndSystemPlanLimit(t *testing.T) {
	fields := appendRoutingDecisionContext(nil, directplay.RoutingDiagnostics{
		Routed: true, PlaybackMode: "system", PlaybackAccountOwner: "shared",
		AccountLimitsAvailable: true, ConfiguredMaxConcurrentStreams: 6, EffectiveMaxConcurrentStreams: 6,
		SimultaneousStreamLimit: 2, TransferChecked: true,
	})
	encoded := strings.Join(fields, " ")
	for _, expected := range []string{
		"playbackMode=system", "playbackAccountOwner=shared",
		"accountConfiguredStreamLimit=6", "accountEffectiveStreamLimit=6",
	} {
		if !strings.Contains(encoded, expected) {
			t.Fatalf("routing context = %q, want %s", encoded, expected)
		}
	}
	for _, unexpected := range []string{
		"simultaneousStreamLimit=", "accountReservedStreams=", "userActiveStreams=", "transferHourlyUsed=", "transferDailyLimit=",
	} {
		if strings.Contains(encoded, unexpected) {
			t.Fatalf("routing context = %q, unexpected %s", encoded, unexpected)
		}
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
