package playbackgateway

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/konghang/ember/backend/internal/services/directplay"
	"github.com/konghang/ember/backend/internal/services/p115quota"
	"github.com/konghang/ember/backend/internal/testutil/faketime"
)

// TestFallbackPlaybackActivityRevalidates uses successful versioned session
// events to recover a previously authorized media proof without a 115 lease.
func TestFallbackPlaybackActivityRevalidates(t *testing.T) {
	for _, kind := range []string{"matching", "paused", "expired", "early_stop", "wrong_source", "wrong_device", "failed_event", "stopped", "duplicate"} {
		t.Run(kind, func(t *testing.T) {
			clock := faketime.New(time.Now())
			internal := 0
			direct := &fakeDirectPlayService{err: directplay.ErrPlaybackIntentRequired, result: validFixtureRedirectCandidate()}
			gateway := newVideoTestGateway(t, "http://emby.invalid", &fakeTokenService{principal: fixturePrincipal()}, direct, roundTripFunc(func(r *http.Request) (*http.Response, error) {
				if strings.Contains(r.URL.Path, "PlaybackInfo") {
					internal++
					return transferIntentHTTPResponse(r, 200, revalidationResponse("mkv", "new-internal")), nil
				}
				if strings.Contains(r.URL.Path, "Sessions/") {
					status := 204
					if kind == "failed_event" {
						status = 400
					}
					return transferIntentHTTPResponse(r, status, ""), nil
				}
				return transferIntentHTTPResponse(r, 206, "fallback"), nil
			}), &bytes.Buffer{})
			gateway.proofs.now = clock.Now
			gateway.playbackSessionService = &fakePlaybackSessionService{}
			gateway.proofs.Record([]PlaybackProof{revalidationProof("mkv", "session-1")})
			target := "/Videos/item-1/original.mkv?MediaSourceId=source-1&PlaySessionId=session-1&DeviceId=device-1"
			first := httptest.NewRecorder()
			gateway.ServeHTTP(first, newVideoRequest("GET", target))
			if first.Code != 206 {
				t.Fatalf("initial fallback=%d", first.Code)
			}
			send := func(event string) {
				body := `{"ItemId":"item-1","MediaSourceId":"source-1","PlaySessionId":"session-1","PositionTicks":42,"IsPaused":false}`
				if kind == "paused" {
					body = strings.Replace(body, `"IsPaused":false`, `"IsPaused":true`, 1)
				}
				if kind == "wrong_source" {
					body = strings.Replace(body, "source-1", "other-source", 1)
				}
				if kind == "duplicate" {
					body = strings.Replace(body, `"ItemId":`, `"itemid":"other-item","ItemId":`, 1)
				}
				if kind == "wrong_device" {
					gateway.tokenService.(*fakeTokenService).principal.DeviceID = "other-device"
				}
				r := httptest.NewRequest("POST", "/Sessions/Playing"+event, strings.NewReader(body))
				r.Header.Set(accessTokenHeader, fixtureAccessToken)
				r.Header.Set("Content-Type", "application/json")
				gateway.ServeHTTP(httptest.NewRecorder(), r)
				gateway.tokenService.(*fakeTokenService).principal = fixturePrincipal()
			}
			for i := 0; i < 7; i++ {
				send("/Progress")
				if kind == "early_stop" && i == 0 {
					send("/Stopped")
				}
				clock.Advance(time.Minute)
			}
			if kind == "paused" {
				clock.Advance(7 * time.Minute)
			}
			if kind == "expired" {
				clock.Advance(3 * time.Minute)
			}
			if kind == "stopped" {
				send("/Stopped")
				send("/Progress")
			}
			direct.err = nil
			response := httptest.NewRecorder()
			gateway.ServeHTTP(response, newVideoRequest("GET", target))
			want := 206
			if kind == "matching" || kind == "paused" {
				want = 302
			}
			wantInternal := 0
			if want == 302 {
				wantInternal = 1
			}
			if response.Code != want || internal != wantInternal {
				t.Fatalf("kind=%s status=%d queries=%d", kind, response.Code, internal)
			}
			if kind == "matching" {
				proof, ok := gateway.LookupPlaybackProof(fixturePrincipal(), "item-1", "source-1", "session-1")
				if !ok || gateway.proofs.CanCreateTransfer(proof, fixturePrincipal()) {
					t.Fatal("activity manufactured strict transfer intent")
				}
				gateway.transferTriggerPolicy = transferTriggerPolicyFunc(func(context.Context) (string, error) { return "compatible", nil })
				authorization, trigger := gateway.authorizeVideoTransfer(context.Background(), "GET", proof, fixturePrincipal())
				if !authorization.Allowed || authorization.CompatibilityLimits != nil || trigger != "session_playback" {
					t.Fatalf("confirmed activity authorization=%+v trigger=%s", authorization, trigger)
				}
				gateway.transferTriggerPolicy = transferTriggerPolicyFunc(func(context.Context) (string, error) { return "strict", nil })
				if authorization, _ := gateway.authorizeVideoTransfer(context.Background(), "GET", proof, fixturePrincipal()); authorization.Allowed {
					t.Fatal("session evidence bypassed strict mode")
				}
				other := proof
				other.PlaySessionID = "other-session"
				gateway.proofs.Record([]PlaybackProof{other})
				other, _ = gateway.proofs.Lookup(other.MappingID, other.ItemID, other.MediaSourceID, other.PlaySessionID)
				if gateway.proofs.hasConfirmedPlayback(other, fixturePrincipal()) {
					t.Fatal("another session borrowed activity")
				}
			}
		})
	}
}

// TestFallbackActivityCannotReclassifyLost115Lease prevents a refreshed
// internal proof from converting an admitted session into lease-free activity.
func TestFallbackActivityCannotReclassifyLost115Lease(t *testing.T) {
	cache := newPlaybackProofCache(8, time.Minute)
	proof := revalidationProof("mkv", "client-session")
	cache.Record([]PlaybackProof{proof})
	proof, _ = cache.Lookup(proof.MappingID, proof.ItemID, proof.MediaSourceID, proof.PlaySessionID)
	cache.rememberPlaybackSession(proof, fixturePrincipal())
	cache.updatePlaybackBinding(fixturePrincipal(), proof.ItemID, proof.MediaSourceID, proof.PlaySessionID, p115quota.LeaseStateActive, true)
	_, guard := cache.BeginOnDemand(fixturePrincipal(), proof.ItemID, proof.MediaSourceID)
	// Publish a distinct internal alias with the verified original identity.
	if guard == nil {
		guard = &playbackProofPublishGuard{mappingID: proof.MappingID, itemID: proof.ItemID, deviceID: proof.DeviceID}
		cache.publications[guard] = struct{}{}
	}
	fresh := proof
	fresh.PlaySessionID = "internal-session"
	cache.PublishOnDemand(guard, []PlaybackProof{fresh})
	cache.ReleasePublication(guard)
	event := playbackSessionEventSnapshot{kind: playbackSessionEventProgress, itemID: proof.ItemID, mediaSourceID: proof.MediaSourceID, playSessionID: proof.PlaySessionID, snapshotState: "recorded"}
	if cache.observeFallbackPlayback(fixturePrincipal(), event) {
		t.Fatal("admitted session converted to fallback activity")
	}
	cache.updatePlaybackBinding(fixturePrincipal(), proof.ItemID, proof.MediaSourceID, proof.PlaySessionID, p115quota.LeaseStateActive, false)
	if cache.observeFallbackPlayback(fixturePrincipal(), event) || len(cache.sessions) != 0 {
		t.Fatal("missing Redis lease was revived through fallback activity")
	}
}
