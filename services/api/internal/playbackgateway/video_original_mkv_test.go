package playbackgateway

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/konghang/ember/backend/internal/services/directplay"
)

// TestGatewayOriginalMKVUsesExactProof limits missing-Static compatibility to
// the observed local MKV source while preserving explicit-static behavior.
func TestGatewayOriginalMKVUsesExactProof(t *testing.T) {
	const target = "/emby/videos/item-1/original.mkv?MediaSourceId=source-1&PlaySessionId=session-1&DeviceId=device-1"
	for _, test := range []struct {
		name, method, target, stage, reason string
		change                              func(*PlaybackProof)
		missing, expired, redirect          bool
	}{
		{name: "GET", target: target, redirect: true},
		{name: "HEAD candidate", method: http.MethodHead, target: target, redirect: true},
		{name: "matching token aliases", target: target + "&api_key=" + fixtureAccessToken + "&X-Emby-Token=" + fixtureAccessToken, redirect: true},
		{name: "case variants", target: "/emby/VIDEOS/item-1/ORIGINAL.MKV?mediasourceid=source-1&playsessionid=session-1", change: func(p *PlaybackProof) { p.Container, p.Path = "MKV", "/mnt/media/fixture.MKV" }, redirect: true},
		{name: "another client same contract", target: target, redirect: true},
		{name: "missing proof", target: target, missing: true, stage: "proof", reason: "playback_proof_missing"},
		{name: "expired proof", target: target, expired: true, stage: "proof", reason: "playback_proof_expired"},
		{name: "wrong session", target: strings.Replace(target, "session-1", "session-other", 1), stage: "proof", reason: "playback_proof_missing"},
		{name: "wrong source", target: strings.Replace(target, "source-1", "source-other", 1), stage: "proof", reason: "playback_proof_missing"},
		{name: "wrong proof device", target: target, change: func(p *PlaybackProof) { p.DeviceID = "other" }, stage: "proof", reason: "playback_proof_mismatch"},
		{name: "wrong query device", target: strings.Replace(target, "DeviceId=device-1", "DeviceId=other", 1), stage: "eligibility", reason: "media_not_direct_play"},
		{name: "no direct play", target: target, change: func(p *PlaybackProof) { p.SupportsDirectPlay = false }, stage: "proof", reason: "playback_proof_missing"},
		{name: "no direct stream", target: target, change: func(p *PlaybackProof) { p.SupportsDirectStream = false }, stage: "eligibility", reason: "media_not_direct_play"},
		{name: "remote source", target: target, change: func(p *PlaybackProof) { p.IsRemote = true }, stage: "eligibility", reason: "media_not_direct_play"},
		{name: "unknown container", target: target, change: func(p *PlaybackProof) { p.Container = "" }, stage: "eligibility", reason: "media_not_direct_play"},
		{name: "wrong container", target: target, change: func(p *PlaybackProof) { p.Container = "mp4" }, stage: "eligibility", reason: "media_not_direct_play"},
		{name: "wrong source extension", target: target, change: func(p *PlaybackProof) { p.Path = "/mnt/media/fixture.mp4" }, stage: "eligibility", reason: "media_not_direct_play"},
		{name: "unknown MKV size", target: target, change: func(p *PlaybackProof) { p.Size = 0 }, stage: "eligibility", reason: "media_not_direct_play"},
		{name: "explicit static retains unknown size", target: target + "&Static=true", change: func(p *PlaybackProof) { p.Size = 0 }, redirect: true},
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
						t.Error("fallback changed the original request")
					}
					return &http.Response{StatusCode: http.StatusPartialContent, Request: r, Header: http.Header{}, Body: io.NopCloser(strings.NewReader("fixture"))}, nil
				}), &logs)
			gateway.debugEnabled = func() bool { return true }
			proof := fixturePlaybackProof("mapping-1", "item-1", "source-1", "session-1")
			proof.Path, proof.Container, proof.SupportsDirectStream, proof.Size = "/mnt/media/fixture.mkv", "mkv", true, 1024
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
			if test.name == "another client same contract" {
				request.Header.Set("User-Agent", "OtherPlayer/1.0")
			}
			response := httptest.NewRecorder()
			gateway.ServeHTTP(response, request)
			calls := direct.snapshot()
			if test.redirect {
				if response.Code != http.StatusFound || upstreamCalls != 0 || len(calls) != 1 {
					t.Fatalf("status=%d upstream=%d direct=%d", response.Code, upstreamCalls, len(calls))
				}
				if calls[0].Path != proof.Path || calls[0].Method != method || calls[0].AuthorizeTransfer == nil || calls[0].AuthorizeTransfer().Allowed {
					t.Fatal("candidate changed method/path or granted new transfer intent")
				}
				if !strings.Contains(test.target, "Static=true") && !strings.Contains(logs.String(), "code=video_original_mkv_accepted") {
					t.Fatal("missing MKV compatibility diagnostic")
				}
				assertSingleDecisionLog(t, logs.String(), "redirect", "direct_play", "direct_play_ready")
			} else {
				if response.Code != http.StatusPartialContent || response.Body.String() != "fixture" || upstreamCalls != 1 || len(calls) != 0 {
					t.Fatalf("status=%d upstream=%d direct=%d", response.Code, upstreamCalls, len(calls))
				}
				assertSingleDecisionLog(t, logs.String(), "fallback", test.stage, test.reason)
			}
			assertSecretsAbsent(t, logs.String(), fixtureAccessToken, fixtureRedirectURL)
		})
	}
}

// TestGatewayOriginalMKVRejectsAmbiguousQueries protects the original request
// semantics even when its media proof otherwise qualifies for reuse.
func TestGatewayOriginalMKVRejectsAmbiguousQueries(t *testing.T) {
	const prefix = "/emby/videos/item-1/"
	const identity = "?MediaSourceId=source-1&PlaySessionId=session-1"
	for _, target := range []string{
		prefix + "stream.mkv" + identity,
		prefix + "original.avi" + identity,
		prefix + "original.mkv" + identity + "&Static=false",
		prefix + "original.mkv" + identity + "&Static=",
		prefix + "original.mkv" + identity + "&Static=true&static=false",
		prefix + "original.mkv" + identity + "&MediaSourceId=other",
		prefix + "original.mkv" + identity + "&PlaySessionId=other",
		prefix + "original.mkv" + identity + "&VideoCodec=h264",
		prefix + "original.mkv" + identity + "&AudioStreamIndex=2",
		prefix + "original.mkv" + identity + "&StartTimeTicks=10",
		prefix + "original.mkv" + identity + "&Container=mkv",
		prefix + "original.mkv" + identity + "&unknown=value",
		prefix + "original.mkv" + identity + "&broken=%ZZ",
		prefix + "original.mkv" + identity + "&DeviceId=device-1&deviceid=other",
		prefix + "original.mkv?MediaSourceId=source-1",
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
			proof.Path, proof.Container, proof.SupportsDirectStream, proof.Size = "/mnt/media/fixture.mkv", "mkv", true, 1024
			gateway.proofs.Record([]PlaybackProof{proof})
			response := httptest.NewRecorder()
			gateway.ServeHTTP(response, newVideoRequest(http.MethodGet, target))
			if response.Code != http.StatusOK || len(direct.snapshot()) != 0 {
				t.Fatalf("status=%d direct=%d", response.Code, len(direct.snapshot()))
			}
		})
	}
}

// TestGatewaySenPlayerOriginalMKVRetainsIntentBoundary replays the observed
// false query with a DeviceProfile body through the real proof observation.
func TestGatewaySenPlayerOriginalMKVRetainsIntentBoundary(t *testing.T) {
	const payload = `{"MediaSources":[{"Id":"source-1","ItemId":"item-1","Path":"/mnt/media/fixture.mkv","Container":"mkv","Size":1024,"SupportsDirectPlay":true,"SupportsDirectStream":true}],"PlaySessionId":"session-1"}`
	const target = "/emby/videos/item-1/original.mkv?MediaSourceId=source-1&PlaySessionId=session-1&DeviceId=device-1&X-Emby-Token=" + fixtureAccessToken + "&api_key=" + fixtureAccessToken
	for _, existing := range []bool{true, false} {
		name := "missing target"
		if existing {
			name = "existing target"
		}
		t.Run(name, func(t *testing.T) {
			var logs bytes.Buffer
			directCalls, fallbackCalls := 0, 0
			direct := transferIntentDirectPlayFunc(func(_ context.Context, r directplay.MediaPathResolveRequest) (directplay.RedirectCandidate, error) {
				directCalls++
				if r.Path != "/mnt/media/fixture.mkv" || r.Method != http.MethodGet || r.UserID != "user-1" || r.MappingID != "mapping-1" || r.DeviceID != "device-1" || r.PlaySessionID != "session-1" || r.AuthorizeTransfer == nil || r.AuthorizeTransfer().Allowed {
					t.Error("MKV reuse lost its identity or granted transfer intent")
				}
				if !existing {
					return directplay.RedirectCandidate{}, directplay.ErrPlaybackIntentRequired
				}
				candidate := validFixtureRedirectCandidate()
				candidate.Preexisting = true
				return candidate, nil
			})
			gateway := newVideoTestGateway(t, "http://emby.invalid", &fakeTokenService{principal: fixturePrincipal()}, direct,
				roundTripFunc(func(r *http.Request) (*http.Response, error) {
					if r.Method == http.MethodPost && r.URL.Path == "/emby/Items/item-1/PlaybackInfo" {
						body, _ := io.ReadAll(r.Body)
						if string(body) != `{"DeviceProfile":{}}` || r.URL.Query().Get("IsPlayback") != "false" {
							t.Error("PlaybackInfo body or query was rewritten")
						}
						return transferIntentHTTPResponse(r, http.StatusOK, payload), nil
					}
					fallbackCalls++
					if r.Method != http.MethodGet || r.URL.RequestURI() != target || r.Header.Get("Range") != "bytes=1008-" {
						t.Error("MKV fallback changed path/query/method/Range")
					}
					return &http.Response{StatusCode: http.StatusPartialContent, Request: r, Header: http.Header{}, Body: io.NopCloser(strings.NewReader("fixture-mkv"))}, nil
				}), &logs)
			post := httptest.NewRequest(http.MethodPost, "/emby/Items/item-1/PlaybackInfo?UserId=emby-user-1&IsPlayback=false&MediaSourceId=source-1", strings.NewReader(`{"DeviceProfile":{}}`))
			post.Header.Set(accessTokenHeader, fixtureAccessToken)
			post.Header.Set("Content-Type", "application/json")
			response := httptest.NewRecorder()
			gateway.ServeHTTP(response, post)
			if response.Code != http.StatusOK || response.Body.String() != payload {
				t.Fatal("PlaybackInfo response changed")
			}
			before, ok := gateway.LookupPlaybackProof(fixturePrincipal(), "item-1", "source-1", "session-1")
			if !ok || gateway.proofs.CanCreateTransfer(before, fixturePrincipal()) {
				t.Fatal("false query should record only ordinary proof")
			}
			logs.Reset()
			get := newVideoRequest(http.MethodGet, target)
			get.Header.Set("Range", "bytes=1008-")
			get.Header.Set("User-Agent", "SenPlayer/6.0.4")
			response = httptest.NewRecorder()
			gateway.ServeHTTP(response, get)
			if directCalls != 1 {
				t.Fatalf("directCalls=%d, want one reuse decision", directCalls)
			}
			if existing {
				if response.Code != http.StatusFound || fallbackCalls != 0 {
					t.Fatalf("existing target status=%d fallback=%d", response.Code, fallbackCalls)
				}
				assertSingleDecisionLog(t, logs.String(), "redirect", "direct_play", "direct_play_ready")
			} else {
				if response.Code != http.StatusPartialContent || response.Body.String() != "fixture-mkv" || fallbackCalls != 1 {
					t.Fatalf("missing target status=%d fallback=%d", response.Code, fallbackCalls)
				}
				assertSingleDecisionLog(t, logs.String(), "fallback", "direct_play", "playback_intent_required")
			}
			after, _ := gateway.LookupPlaybackProof(fixturePrincipal(), "item-1", "source-1", "session-1")
			if after != before || gateway.proofs.CanCreateTransfer(after, fixturePrincipal()) {
				t.Fatal("video GET changed proof or transfer intent")
			}
			assertSecretsAbsent(t, logs.String(), fixtureAccessToken, fixtureRedirectURL)
		})
	}
}
