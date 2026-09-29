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
)

// TestUnsupportedClientProofRetainsBoundedIntent reproduces Infuse's explicit
// POST followed by a supported internal GET with a different session id.
func TestUnsupportedClientProofRetainsBoundedIntent(t *testing.T) {
	body := strings.Replace(transferIntentResponse, `"SupportsDirectPlay":true`, `"SupportsDirectPlay":false`, 1)
	gateway := newTransferIntentGateway(t, nil, http.StatusOK, body)
	now := time.Now()
	gateway.proofs.now = func() time.Time { return now }
	request := httptest.NewRequest(http.MethodPost, "/Items/item-1/PlaybackInfo", strings.NewReader(`{"IsPlayback":true,"MediaSourceId":"source-1"}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set(accessTokenHeader, fixtureAccessToken)
	gateway.ServeHTTP(httptest.NewRecorder(), request)
	if gateway.proofs.Len() != 0 {
		t.Fatal("unsupported response became a media proof")
	}
	_, guard := gateway.proofs.BeginOnDemand(fixturePrincipal(), "item-1", "source-1")
	defer gateway.proofs.ReleasePublication(guard)
	proof := fixturePlaybackProof("mapping-1", "item-1", "source-1", "session-1")
	proof.PlaySessionID = "internal-session"
	now = now.Add(10 * time.Second)
	if _, ok := gateway.proofs.PublishOnDemand(guard, []PlaybackProof{proof}); !ok {
		t.Fatal("publication failed")
	}
	current, _ := gateway.proofs.Lookup(proof.MappingID, proof.ItemID, proof.MediaSourceID, proof.PlaySessionID)
	if !gateway.proofs.CanCreateTransfer(current, fixturePrincipal()) {
		t.Fatal("explicit client intent lost during internal lookup")
	}
	now = now.Add(20 * time.Second)
	if gateway.proofs.CanCreateTransfer(current, fixturePrincipal()) {
		t.Fatal("internal lookup renewed client intent")
	}
}

// bridgeFixture records only a successful explicit client response; it never
// supplies a media capability to the video route by itself.
func bridgeFixture(t *testing.T, requestBody, responseBody string) *Gateway {
	t.Helper()
	gateway := newTransferIntentGateway(t, nil, http.StatusOK, responseBody)
	sendTransferIntentPlaybackInfo(gateway, requestBody)
	return gateway
}

// publishBridgeProof exercises the guarded internal publication used by the
// real resolver, then retrieves its generation-bearing snapshot.
func publishBridgeProof(t *testing.T, gateway *Gateway, proof PlaybackProof) PlaybackProof {
	t.Helper()
	_, guard := gateway.proofs.BeginOnDemand(fixturePrincipal(), proof.ItemID, proof.MediaSourceID)
	defer gateway.proofs.ReleasePublication(guard)
	if _, ok := gateway.proofs.PublishOnDemand(guard, []PlaybackProof{proof}); !ok {
		t.Fatal("internal publication failed")
	}
	current, _ := gateway.proofs.Lookup(proof.MappingID, proof.ItemID, proof.MediaSourceID, proof.PlaySessionID)
	return current
}

// TestIntentBridgeBoundaries excludes ambiguous/no intent, unsafe responses,
// identity drift, later browsing and stopped sessions from transfer admission.
func TestIntentBridgeBoundaries(t *testing.T) {
	unsupported := strings.Replace(transferIntentResponse, `"SupportsDirectPlay":true`, `"SupportsDirectPlay":false`, 1)
	for _, test := range []struct {
		name, request, response string
		mutate                  func(*PlaybackProof)
		before                  func(*Gateway)
		allowed                 bool
	}{
		{name: "matching", allowed: true},
		{name: "missing intent", request: `{}`},
		{name: "false intent", request: `{"IsPlayback":false}`},
		{name: "duplicate intent", request: `{"IsPlayback":true,"isPlayback":true}`},
		{name: "wrong selected source", request: `{"IsPlayback":true,"MediaSourceId":"other"}`},
		{name: "upstream error", response: strings.Replace(unsupported, `"PlaySessionId"`, `"ErrorCode":"NotAllowed","PlaySessionId"`, 1)},
		{name: "wrong response item", response: strings.Replace(unsupported, `"ItemId":"item-1"`, `"ItemId":"other"`, 1)},
		{name: "empty session", response: strings.Replace(unsupported, `"session-1"`, `""`, 1)},
		{name: "remote source", response: strings.Replace(unsupported, `"Container"`, `"IsRemote":true,"Container"`, 1)},
		{name: "path mismatch", mutate: func(p *PlaybackProof) { p.Path = "/mnt/media/other.mkv" }},
		{name: "source mismatch", mutate: func(p *PlaybackProof) { p.MediaSourceID = "other" }},
		{name: "device mismatch", mutate: func(p *PlaybackProof) { p.DeviceID = "other" }},
		{name: "server mismatch", mutate: func(p *PlaybackProof) { p.ServerID = "other" }},
		{name: "user mismatch", mutate: func(p *PlaybackProof) { p.UserID = "other" }},
		{name: "container mismatch", mutate: func(p *PlaybackProof) { p.Container = "mp4" }},
		{name: "stop before supplement", before: func(g *Gateway) { g.proofs.RevokeTransferIntent(fixturePrincipal(), "item-1", "source-1", "session-1") }},
		{name: "unrelated stop", before: func(g *Gateway) { g.proofs.RevokeTransferIntent(fixturePrincipal(), "item-1", "source-1", "unrelated") }, allowed: true},
		{name: "later browse", before: func(g *Gateway) { sendTransferIntentPlaybackInfo(g, `{}`) }},
	} {
		t.Run(test.name, func(t *testing.T) {
			if test.request == "" {
				test.request = `{"IsPlayback":true}`
			}
			if test.response == "" {
				test.response = unsupported
			}
			g := bridgeFixture(t, test.request, test.response)
			if test.before != nil {
				test.before(g)
			}
			p := fixturePlaybackProof("mapping-1", "item-1", "source-1", "internal")
			if test.mutate != nil {
				test.mutate(&p)
			}
			current := publishBridgeProof(t, g, p)
			if got := g.proofs.CanCreateTransfer(current, fixturePrincipal()); got != test.allowed {
				t.Fatalf("intent=%t want=%t", got, test.allowed)
			}
		})
	}
}

// TestIntentBridgeStopAndPublicationRace ensures the original client's stop
// revokes its descendant and an in-flight internal response cannot restore it.
func TestIntentBridgeStopAndPublicationRace(t *testing.T) {
	unsupported := strings.Replace(transferIntentResponse, `"SupportsDirectPlay":true`, `"SupportsDirectPlay":false`, 1)
	for _, stopSession := range []string{"session-1", "internal"} {
		g := bridgeFixture(t, `{"IsPlayback":true}`, unsupported)
		p := fixturePlaybackProof("mapping-1", "item-1", "source-1", "internal")
		current := publishBridgeProof(t, g, p)
		if !g.proofs.CanCreateTransfer(current, fixturePrincipal()) {
			t.Fatal("missing initial grant")
		}
		g.proofs.RevokeTransferIntent(fixturePrincipal(), "item-1", "", stopSession)
		if g.proofs.CanCreateTransfer(current, fixturePrincipal()) {
			t.Fatal("stopped descendant retained grant")
		}
	}
	g := bridgeFixture(t, `{"IsPlayback":true}`, unsupported)
	_, guard := g.proofs.BeginOnDemand(fixturePrincipal(), "item-1", "source-1")
	defer g.proofs.ReleasePublication(guard)
	g.proofs.RevokeTransferIntent(fixturePrincipal(), "item-1", "source-1", "session-1")
	p := fixturePlaybackProof("mapping-1", "item-1", "source-1", "internal")
	g.proofs.PublishOnDemand(guard, []PlaybackProof{p})
	current, _ := g.proofs.Lookup(p.MappingID, p.ItemID, p.MediaSourceID, p.PlaySessionID)
	if g.proofs.CanCreateTransfer(current, fixturePrincipal()) {
		t.Fatal("late supplement resurrected stopped intent")
	}
}

// TestGatewayInfuseExplicitIntentSurvivesUnsupportedResponse covers the full
// POST -> internal GET -> video -> DirectPlay callback with all HTTP mocked.
func TestGatewayInfuseExplicitIntentSurvivesUnsupportedResponse(t *testing.T) {
	called := false
	service := transferIntentDirectPlayFunc(func(_ context.Context, request directplay.MediaPathResolveRequest) (directplay.RedirectCandidate, error) {
		called = true
		if request.CanCreateTransfer == nil || !request.CanCreateTransfer() {
			t.Fatal("explicit intent did not reach transfer admission")
		}
		return validFixtureRedirectCandidate(), nil
	})
	g := newVideoTestGateway(t, "http://emby.invalid", &fakeTokenService{principal: fixturePrincipal()}, service, roundTripFunc(func(request *http.Request) (*http.Response, error) {
		body := transferIntentResponse
		if request.Method == http.MethodPost {
			body = strings.Replace(body, `"SupportsDirectPlay":true`, `"SupportsDirectPlay":false`, 1)
		} else {
			body = strings.Replace(body, `"session-1"`, `"internal"`, 1)
		}
		return transferIntentHTTPResponse(request, http.StatusOK, body), nil
	}), &bytes.Buffer{})
	sendTransferIntentPlaybackInfo(g, `{"IsPlayback":true,"MediaSourceId":"source-1"}`)
	recorder := httptest.NewRecorder()
	g.ServeHTTP(recorder, newVideoRequest(http.MethodGet, "/Videos/item-1/stream?MediaSourceId=source-1&Static=true"))
	if !called || recorder.Code != http.StatusFound {
		t.Fatalf("called=%t status=%d", called, recorder.Code)
	}
}

// TestIntentBridgeCannotCrossSessionOrSupersedeClient bounds inheritance to
// one internal session and applies the existing stale-publication guards.
func TestIntentBridgeCannotCrossSessionOrSupersedeClient(t *testing.T) {
	unsupported := strings.Replace(transferIntentResponse, `"SupportsDirectPlay":true`, `"SupportsDirectPlay":false`, 1)
	g := bridgeFixture(t, `{"IsPlayback":true}`, unsupported)
	p := fixturePlaybackProof("mapping-1", "item-1", "source-1", "internal")
	current := publishBridgeProof(t, g, p)
	// Register through another source to simulate a second in-flight resolution
	// replacing the whole item snapshot with a different internal session.
	_, guard := g.proofs.BeginOnDemand(fixturePrincipal(), "item-1", "other-source")
	defer g.proofs.ReleasePublication(guard)
	p.PlaySessionID = "different-internal"
	g.proofs.PublishOnDemand(guard, []PlaybackProof{p})
	later, _ := g.proofs.Lookup(p.MappingID, p.ItemID, p.MediaSourceID, p.PlaySessionID)
	if g.proofs.CanCreateTransfer(later, fixturePrincipal()) || g.proofs.CanCreateTransfer(current, fixturePrincipal()) {
		t.Fatal("grant lent to another session or stale proof generation")
	}
	old := g.proofs.BeginClient("mapping-1", "item-1")
	defer g.proofs.ReleasePublication(old)
	newer := g.proofs.BeginClient("mapping-1", "item-1")
	defer g.proofs.ReleasePublication(newer)
	g.proofs.PublishClient(newer, nil)
	intent := fixturePlaybackProof("mapping-1", "item-1", "source-1", "session-1")
	intent.SupportsDirectPlay = false
	intent.transferIntent = true
	if _, _, published := g.proofs.publishClientEvidence(old, nil, []PlaybackProof{intent}); published {
		t.Fatal("old client response restored intent after newer response")
	}
	later = publishBridgeProof(t, g, p)
	if g.proofs.CanCreateTransfer(later, fixturePrincipal()) {
		t.Fatal("stale grant resurrected")
	}
}

// TestIntentBridgeStorageBoundedAndExpired keeps unsupported client traffic
// from growing a second unbounded store; expired grants free capacity lazily.
func TestIntentBridgeStorageBoundedAndExpired(t *testing.T) {
	cache := newPlaybackProofCache(1, time.Minute)
	now := time.Now()
	cache.now = func() time.Time { return now }
	for _, item := range []string{"first", "second"} {
		guard := cache.BeginClient("mapping-1", item)
		p := fixturePlaybackProof("mapping-1", item, "source-1", "session-1")
		cache.publishClientEvidence(guard, nil, []PlaybackProof{p})
		cache.ReleasePublication(guard)
	}
	if len(cache.intents) != 1 {
		t.Fatal("intent storage exceeded capacity")
	}
	now = now.Add(playbackTransferIntentTTL)
	cache.Len()
	if len(cache.intents) != 0 {
		t.Fatal("expired grants retained capacity")
	}
}
