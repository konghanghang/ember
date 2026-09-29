package playbackgateway

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/konghang/ember/backend/internal/services/directplay"
)

// TestGatewayOriginalMP4UsesExactProof locks the observed missing-Static shape
// to one local MP4 source without changing the existing transfer-intent gate.
func TestGatewayOriginalMP4UsesExactProof(t *testing.T) {
	const target = "/emby/videos/item-1/original.mp4?MediaSourceId=source-1&PlaySessionId=session-1&DeviceId=device-1"
	for _, test := range []struct {
		name, method, target, stage, reason string
		change                              func(*PlaybackProof)
		missing, expired, redirect          bool
	}{
		{name: "GET", method: http.MethodGet, target: target, redirect: true},
		{name: "HEAD", method: http.MethodHead, target: target, redirect: true},
		{name: "matching token aliases", target: target + "&X-Emby-Token=" + fixtureAccessToken + "&api_key=" + fixtureAccessToken, redirect: true},
		{name: "explicit static unchanged", target: target + "&Static=true", redirect: true},
		{name: "case variants", target: "/emby/VIDEOS/item-1/ORIGINAL.MP4?mediasourceid=source-1&playsessionid=session-1", redirect: true},
		{name: "missing proof", target: target, missing: true, stage: "proof", reason: "playback_proof_missing"},
		{name: "expired proof", target: target, expired: true, stage: "proof", reason: "playback_proof_expired"},
		{name: "wrong session", target: strings.Replace(target, "session-1", "session-other", 1), stage: "proof", reason: "playback_proof_missing"},
		{name: "wrong source", target: strings.Replace(target, "source-1", "source-other", 1), stage: "proof", reason: "playback_proof_missing"},
		{name: "wrong device proof", target: target, change: func(p *PlaybackProof) { p.DeviceID = "other" }, stage: "proof", reason: "playback_proof_mismatch"},
		{name: "wrong device query", target: strings.Replace(target, "DeviceId=device-1", "DeviceId=other", 1), stage: "eligibility", reason: "media_not_direct_play"},
		{name: "unsupported direct play", target: target, change: func(p *PlaybackProof) { p.SupportsDirectPlay = false }, stage: "proof", reason: "playback_proof_missing"},
		{name: "remote media", target: target, change: func(p *PlaybackProof) { p.IsRemote = true }, stage: "eligibility", reason: "media_not_direct_play"},
		{name: "no direct stream", target: target, change: func(p *PlaybackProof) { p.SupportsDirectStream = false }, stage: "eligibility", reason: "media_not_direct_play"},
		{name: "unknown container", target: target, change: func(p *PlaybackProof) { p.Container = "" }, stage: "eligibility", reason: "media_not_direct_play"},
		{name: "container mismatch", target: target, change: func(p *PlaybackProof) { p.Container = "mkv" }, stage: "eligibility", reason: "media_not_direct_play"},
		{name: "source extension mismatch", target: target, change: func(p *PlaybackProof) { p.Path = "/mnt/media/fixture.mkv" }, stage: "eligibility", reason: "media_not_direct_play"},
	} {
		t.Run(test.name, func(t *testing.T) {
			method := test.method
			if method == "" {
				method = http.MethodGet
			}
			var logs bytes.Buffer
			upstreamCalls := 0
			direct := &fakeDirectPlayService{result: validFixtureRedirectCandidate()}
			gateway := newVideoTestGateway(t, "http://emby.invalid", &fakeTokenService{principal: fixturePrincipal()}, direct,
				roundTripFunc(func(r *http.Request) (*http.Response, error) {
					upstreamCalls++
					if r.URL.RequestURI() != test.target || r.Method != method || r.Header.Get("Range") != "bytes=10-" {
						t.Error("fallback changed the original path, query, method or Range")
					}
					return &http.Response{StatusCode: http.StatusPartialContent, Request: r, Header: http.Header{}, Body: io.NopCloser(strings.NewReader("fixture"))}, nil
				}), &logs)
			proof := fixturePlaybackProof("mapping-1", "item-1", "source-1", "session-1")
			proof.Path, proof.Container, proof.SupportsDirectStream = "/mnt/media/fixture.mp4", "mp4", true
			if test.change != nil {
				test.change(&proof)
			}
			if !test.missing {
				gateway.proofs.Record([]PlaybackProof{proof})
			}
			if test.expired {
				gateway.proofs.now = func() time.Time { return time.Now().Add(defaultPlaybackProofTTL + time.Second) }
			}
			request := newVideoRequest(method, test.target)
			request.Header.Set("Range", "bytes=10-")
			request.Header.Set("User-Agent", "SenPlayer/6.0.4")
			response := httptest.NewRecorder()
			gateway.ServeHTTP(response, request)
			calls := direct.snapshot()
			if test.redirect {
				if response.Code != http.StatusFound || upstreamCalls != 0 || len(calls) != 1 {
					t.Fatalf("status=%d upstream=%d direct=%d", response.Code, upstreamCalls, len(calls))
				}
				if calls[0].Path != proof.Path || calls[0].CanCreateTransfer == nil || calls[0].CanCreateTransfer() {
					t.Fatal("original stream must use the exact proof without granting transfer intent")
				}
				assertSingleDecisionLog(t, logs.String(), "redirect", "direct_play", "direct_play_ready")
			} else {
				if response.Code != http.StatusPartialContent || upstreamCalls != 1 || len(calls) != 0 {
					t.Fatalf("status=%d upstream=%d direct=%d", response.Code, upstreamCalls, len(calls))
				}
				assertSingleDecisionLog(t, logs.String(), "fallback", test.stage, test.reason)
			}
			assertSecretsAbsent(t, logs.String(), fixtureAccessToken, fixtureRedirectURL)
		})
	}
}

// TestGatewayOriginalMP4RejectsAmbiguousQueries preserves explicit Static
// semantics and rejects transformation parameters even with a valid proof.
func TestGatewayOriginalMP4RejectsAmbiguousQueries(t *testing.T) {
	const prefix = "/emby/videos/item-1/"
	const identity = "?MediaSourceId=source-1&PlaySessionId=session-1"
	for _, target := range []string{
		prefix + "stream.mp4" + identity,
		prefix + "original.mkv" + identity,
		prefix + "original.mp4" + identity + "&Static=false",
		prefix + "original.mp4" + identity + "&Static=",
		prefix + "original.mp4" + identity + "&Static=true&static=false",
		prefix + "original.mp4" + identity + "&MediaSourceId=other",
		prefix + "original.mp4" + identity + "&PlaySessionId=other",
		prefix + "original.mp4" + identity + "&VideoCodec=h264",
		prefix + "original.mp4" + identity + "&AudioStreamIndex=2",
		prefix + "original.mp4" + identity + "&StartTimeTicks=10",
		prefix + "original.mp4" + identity + "&Container=mkv",
		prefix + "original.mp4" + identity + "&unknown=value",
		prefix + "original.mp4" + identity + "&broken=%ZZ",
		prefix + "original.mp4" + identity + "&DeviceId=device-1&deviceid=other",
		prefix + "original.mp4?MediaSourceId=source-1",
	} {
		t.Run(target, func(t *testing.T) {
			var logs bytes.Buffer
			direct := &fakeDirectPlayService{result: validFixtureRedirectCandidate()}
			gateway := newVideoTestGateway(t, "http://emby.invalid", &fakeTokenService{principal: fixturePrincipal()}, direct,
				roundTripFunc(func(r *http.Request) (*http.Response, error) {
					if r.URL.RequestURI() != target {
						t.Error("fallback changed the original query")
					}
					return &http.Response{StatusCode: http.StatusOK, Request: r, Header: http.Header{}, Body: io.NopCloser(strings.NewReader("fixture"))}, nil
				}), &logs)
			proof := fixturePlaybackProof("mapping-1", "item-1", "source-1", "session-1")
			proof.Path, proof.Container, proof.SupportsDirectStream = "/mnt/media/fixture.mp4", "mp4", true
			gateway.proofs.Record([]PlaybackProof{proof})
			response := httptest.NewRecorder()
			gateway.ServeHTTP(response, newVideoRequest(http.MethodGet, target))
			if response.Code != http.StatusOK || len(direct.snapshot()) != 0 {
				t.Fatalf("status=%d direct=%d", response.Code, len(direct.snapshot()))
			}
		})
	}
}

// TestGatewayOriginalMP4DirectPlayFailureKeepsOriginalRequest prevents an
// acceleration failure from injecting Static or stripping a client's Range.
func TestGatewayOriginalMP4DirectPlayFailureKeepsOriginalRequest(t *testing.T) {
	const target = "/emby/videos/item-1/original.mp4?MediaSourceId=source-1&PlaySessionId=session-1"
	var logs bytes.Buffer
	direct := &fakeDirectPlayService{err: directplay.ErrPlaybackIntentRequired}
	gateway := newVideoTestGateway(t, "http://emby.invalid", &fakeTokenService{principal: fixturePrincipal()}, direct,
		roundTripFunc(func(r *http.Request) (*http.Response, error) {
			if r.URL.RequestURI() != target || r.Header.Get("Range") != "bytes=10-" {
				t.Error("fallback changed the original request")
			}
			return &http.Response{StatusCode: http.StatusPartialContent, Request: r, Header: http.Header{}, Body: io.NopCloser(strings.NewReader("fixture"))}, nil
		}), &logs)
	proof := fixturePlaybackProof("mapping-1", "item-1", "source-1", "session-1")
	proof.Path, proof.Container, proof.SupportsDirectStream = "/mnt/media/fixture.mp4", "mp4", true
	gateway.proofs.Record([]PlaybackProof{proof})
	request := newVideoRequest(http.MethodGet, target)
	request.Header.Set("Range", "bytes=10-")
	response := httptest.NewRecorder()
	gateway.ServeHTTP(response, request)
	if response.Code != http.StatusPartialContent || len(direct.snapshot()) != 1 {
		t.Fatalf("status=%d direct=%d", response.Code, len(direct.snapshot()))
	}
	assertSingleDecisionLog(t, logs.String(), "fallback", "direct_play", "playback_intent_required")
}

// TestGatewaySenPlayerOriginalMP4RetainsQueryIntentBoundary exercises a real
// response observation before the video request; query intent cannot create files.
func TestGatewaySenPlayerOriginalMP4RetainsQueryIntentBoundary(t *testing.T) {
	const payload = `{"MediaSources":[{"Id":"source-1","ItemId":"item-1","Path":"/mnt/media/fixture.mp4","Container":"mp4","Size":1024,"SupportsDirectPlay":true,"SupportsDirectStream":true}],"PlaySessionId":"session-1"}`
	var logs bytes.Buffer
	upstreamCalls := 0
	direct := &fakeDirectPlayService{result: validFixtureRedirectCandidate()}
	gateway := newVideoTestGateway(t, "http://emby.invalid", &fakeTokenService{principal: fixturePrincipal()}, direct,
		roundTripFunc(func(r *http.Request) (*http.Response, error) {
			upstreamCalls++
			if r.Method != http.MethodPost || r.URL.Path != "/emby/Items/item-1/PlaybackInfo" {
				t.Error("unexpected upstream request")
			}
			return transferIntentHTTPResponse(r, http.StatusOK, payload), nil
		}), &logs)
	gateway.debugEnabled = func() bool { return true }
	request := httptest.NewRequest(http.MethodPost, "/emby/Items/item-1/PlaybackInfo?UserId=emby-user-1&IsPlayback=true&MediaSourceId=source-1", strings.NewReader(`{}`))
	request.Header.Set(accessTokenHeader, fixtureAccessToken)
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	gateway.ServeHTTP(response, request)
	if response.Code != http.StatusOK || response.Body.String() != payload {
		t.Fatal("PlaybackInfo response changed")
	}
	response = httptest.NewRecorder()
	gateway.ServeHTTP(response, newVideoRequest(http.MethodGet, "/emby/videos/item-1/original.mp4?MediaSourceId=source-1&PlaySessionId=session-1&DeviceId=device-1&X-Emby-Token="+fixtureAccessToken+"&api_key="+fixtureAccessToken))
	calls := direct.snapshot()
	if response.Code != http.StatusFound || len(calls) != 1 || upstreamCalls != 1 {
		t.Fatalf("status=%d direct=%d upstream=%d", response.Code, len(calls), upstreamCalls)
	}
	if calls[0].CanCreateTransfer() {
		t.Fatal("query-only IsPlayback granted transfer intent")
	}
	if !strings.Contains(logs.String(), "code=video_original_mp4_accepted") {
		t.Fatal("missing compatibility diagnostic")
	}
	assertSecretsAbsent(t, logs.String(), fixtureAccessToken, fixtureRedirectURL)
}
