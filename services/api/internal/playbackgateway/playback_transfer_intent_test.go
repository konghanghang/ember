package playbackgateway

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/konghang/ember/backend/internal/services/directplay"
	"github.com/konghang/ember/backend/internal/services/embytoken"
)

const transferIntentResponse = `{"MediaSources":[{"Id":"source-1","ItemId":"item-1","Path":"/mnt/media/fixture.mkv","Container":"mkv","SupportsDirectPlay":true}],"PlaySessionId":"session-1"}`

// TestGatewayTransferIntentRequiresUnambiguousClientPlayback preserves proof
// eligibility and exact proxy bytes while rejecting ambiguous transfer intent.
func TestGatewayTransferIntentRequiresUnambiguousClientPlayback(t *testing.T) {
	for _, test := range []struct {
		name, method, body string
		wantIntent         bool
	}{
		{"explicit", http.MethodPost, `{"IsPlayback":true}`, true},
		{"selected", http.MethodPost, `{"IsPlayback":true,"MediaSourceId":"source-1"}`, true},
		{"lowercase", http.MethodPost, `{"isPlayback":true,"mediaSourceId":"source-1"}`, true},
		{"empty source", http.MethodPost, `{"IsPlayback":true,"MediaSourceId":""}`, true},
		{"false", http.MethodPost, `{"IsPlayback":false}`, false},
		{"missing", http.MethodPost, `{}`, false},
		{"null", http.MethodPost, `{"IsPlayback":null}`, false},
		{"string", http.MethodPost, `{"IsPlayback":"true"}`, false},
		{"number", http.MethodPost, `{"IsPlayback":1}`, false},
		{"duplicate", http.MethodPost, `{"IsPlayback":false,"IsPlayback":true}`, false},
		{"duplicate same", http.MethodPost, `{"IsPlayback":true,"IsPlayback":true}`, false},
		{"case duplicate", http.MethodPost, `{"IsPlayback":false,"isPlayback":true}`, false},
		{"source null", http.MethodPost, `{"IsPlayback":true,"MediaSourceId":null}`, false},
		{"source number", http.MethodPost, `{"IsPlayback":true,"MediaSourceId":1}`, false},
		{"source duplicate", http.MethodPost, `{"IsPlayback":true,"MediaSourceId":"other","MediaSourceId":"source-1"}`, false},
		{"source case duplicate", http.MethodPost, `{"IsPlayback":true,"MediaSourceId":"source-1","mediaSourceId":"source-1"}`, false},
		{"source mismatch", http.MethodPost, `{"IsPlayback":true,"MediaSourceId":"other"}`, false},
		{"source whitespace", http.MethodPost, `{"IsPlayback":true,"MediaSourceId":" source-1"}`, false},
		{"nested does not count", http.MethodPost, `{"DeviceProfile":{"IsPlayback":true}}`, false},
		{"body null", http.MethodPost, `null`, false},
		{"user duplicate", http.MethodPost, `{"IsPlayback":true,"UserId":"other","UserId":"emby-user-1"}`, false},
		{"client GET", http.MethodGet, `{"IsPlayback":true}`, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			var forwarded string
			gateway := newVideoTestGateway(t, "http://emby.invalid", &fakeTokenService{principal: fixturePrincipal()}, nil,
				roundTripFunc(func(request *http.Request) (*http.Response, error) {
					body, _ := io.ReadAll(request.Body)
					forwarded = string(body)
					return transferIntentHTTPResponse(request, http.StatusOK, transferIntentResponse), nil
				}), &bytes.Buffer{})
			request := httptest.NewRequest(test.method, "/Items/item-1/PlaybackInfo?UserId=emby-user-1", strings.NewReader(test.body))
			request.Header.Set("Content-Type", "application/json")
			request.Header.Set(accessTokenHeader, fixtureAccessToken)
			response := httptest.NewRecorder()
			gateway.ServeHTTP(response, request)
			proof, ok := gateway.LookupPlaybackProof(fixturePrincipal(), "item-1", "source-1", "session-1")
			if !ok || response.Code != http.StatusOK || response.Body.String() != transferIntentResponse || forwarded != test.body {
				t.Fatalf("transparent proof contract changed: proof=%t status=%d body=%q forwarded=%q", ok, response.Code, response.Body.String(), forwarded)
			}
			if got := gateway.proofs.CanCreateTransfer(proof, fixturePrincipal()); got != test.wantIntent {
				t.Fatalf("CanCreateTransfer=%t, want %t", got, test.wantIntent)
			}
		})
	}
}

// TestGatewayTransferIntentRejectsQueryBindingAmbiguity leaves POST proof
// eligibility intact when unsupported query fields could override its body.
func TestGatewayTransferIntentRejectsQueryBindingAmbiguity(t *testing.T) {
	for _, query := range []string{"IsPlayback=false", "IsPlayback=true", "isPlayback=true", "MediaSourceId=source-1", "mediaSourceId=other"} {
		t.Run(query, func(t *testing.T) {
			gateway := newTransferIntentGateway(t, nil, http.StatusOK, transferIntentResponse)
			request := httptest.NewRequest(http.MethodPost, "/Items/item-1/PlaybackInfo?"+query, strings.NewReader(`{"IsPlayback":true}`))
			request.Header.Set("Content-Type", "application/json")
			request.Header.Set(accessTokenHeader, fixtureAccessToken)
			gateway.ServeHTTP(httptest.NewRecorder(), request)
			proof, ok := gateway.LookupPlaybackProof(fixturePrincipal(), "item-1", "source-1", "session-1")
			if !ok || gateway.proofs.CanCreateTransfer(proof, fixturePrincipal()) {
				t.Fatal("ambiguous query changed proof or granted intent")
			}
		})
	}
}

// TestGatewayTransferIntentRequiresSuccessfulMatchingResponse prevents a
// client flag alone, failed upstream or wrong media response from granting.
func TestGatewayTransferIntentRequiresSuccessfulMatchingResponse(t *testing.T) {
	for _, test := range []struct {
		name   string
		status int
		body   string
	}{
		{"upstream failure", http.StatusBadGateway, transferIntentResponse},
		{"wrong item", http.StatusOK, strings.Replace(transferIntentResponse, `"item-1"`, `"other"`, 1)},
		{"error response", http.StatusOK, strings.Replace(transferIntentResponse, `"PlaySessionId"`, `"ErrorCode":"NotAllowed","PlaySessionId"`, 1)},
		{"empty session", http.StatusOK, strings.Replace(transferIntentResponse, `"session-1"`, `""`, 1)},
	} {
		t.Run(test.name, func(t *testing.T) {
			gateway := newTransferIntentGateway(t, nil, test.status, test.body)
			sendTransferIntentPlaybackInfo(gateway, `{"IsPlayback":true}`)
			if gateway.proofs.Len() != 0 {
				t.Fatal("unusable response granted proof or intent")
			}
		})
	}
}

// TestGatewayTransferIntentSelectsOnlyAuthoritativeSource rejects implicit
// selection when Emby reports multiple sources, including invalid alternatives.
func TestGatewayTransferIntentSelectsOnlyAuthoritativeSource(t *testing.T) {
	for _, test := range []struct {
		name, request, extraSource string
		wantFirst, wantSecond      bool
	}{
		{"ambiguous", `{"IsPlayback":true}`, `{"Id":"source-2","Path":"/mnt/media/other.mkv","SupportsDirectPlay":true}`, false, false},
		{"selected", `{"IsPlayback":true,"MediaSourceId":"source-2"}`, `{"Id":"source-2","Path":"/mnt/media/other.mkv","SupportsDirectPlay":true}`, false, true},
		{"invalid alternative", `{"IsPlayback":true}`, `{"Id":"source-2","SupportsDirectPlay":false}`, false, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			body := strings.Replace(transferIntentResponse, `}],"PlaySessionId"`, `},`+test.extraSource+`],"PlaySessionId"`, 1)
			gateway := newTransferIntentGateway(t, nil, http.StatusOK, body)
			sendTransferIntentPlaybackInfo(gateway, test.request)
			for source, want := range map[string]bool{"source-1": test.wantFirst, "source-2": test.wantSecond} {
				proof, _ := gateway.LookupPlaybackProof(fixturePrincipal(), "item-1", source, "session-1")
				if got := gateway.proofs.CanCreateTransfer(proof, fixturePrincipal()); got != want {
					t.Fatalf("source=%s intent=%t want=%t", source, got, want)
				}
			}
		})
	}
}

// TestPlaybackTransferIntentExpiryReplacementAndIdentity models time spent
// waiting for DirectPlay locks and prevents stale requests borrowing new intent.
func TestPlaybackTransferIntentExpiryReplacementAndIdentity(t *testing.T) {
	now := time.Now()
	gateway := newTransferIntentGateway(t, nil, http.StatusOK, transferIntentResponse)
	gateway.proofs.now = func() time.Time { return now }
	sendTransferIntentPlaybackInfo(gateway, `{"IsPlayback":true}`)
	proof, _ := gateway.LookupPlaybackProof(fixturePrincipal(), "item-1", "source-1", "session-1")
	checker := func() bool { return gateway.proofs.CanCreateTransfer(proof, fixturePrincipal()) }
	if !checker() {
		t.Fatal("explicit client intent missing")
	}
	now = now.Add(-time.Second)
	if checker() {
		t.Fatal("clock rollback accepted intent issued in the future")
	}
	now = now.Add(time.Second)
	for _, mutate := range []func(*embytoken.Principal){
		func(p *embytoken.Principal) { p.MappingID = "other" },
		func(p *embytoken.Principal) { p.ServerID = "other" },
		func(p *embytoken.Principal) { p.User.ID = "other" },
		func(p *embytoken.Principal) { p.User.EmbyID = "other" },
		func(p *embytoken.Principal) { p.DeviceID = "other" },
	} {
		principal := fixturePrincipal()
		mutate(&principal)
		if gateway.proofs.CanCreateTransfer(proof, principal) {
			t.Fatal("intent crossed identity")
		}
	}
	now = now.Add(29 * time.Second)
	gateway.LookupPlaybackProof(fixturePrincipal(), "item-1", "source-1", "session-1")
	if !checker() {
		t.Fatal("intent expired early")
	}
	now = now.Add(time.Second)
	if checker() {
		t.Fatal("lookup extended fixed intent TTL")
	}
	if _, ok := gateway.LookupPlaybackProof(fixturePrincipal(), "item-1", "source-1", "session-1"); !ok {
		t.Fatal("intent expiry removed media proof")
	}
	sendTransferIntentPlaybackInfo(gateway, `{"IsPlayback":true}`)
	current, _ := gateway.LookupPlaybackProof(fixturePrincipal(), "item-1", "source-1", "session-1")
	if checker() || !gateway.proofs.CanCreateTransfer(current, fixturePrincipal()) {
		t.Fatal("old request borrowed replacement intent")
	}
	// Same timestamp, key and path still denote a new response generation.
	sendTransferIntentPlaybackInfo(gateway, `{"IsPlayback":true}`)
	if gateway.proofs.CanCreateTransfer(current, fixturePrincipal()) {
		t.Fatal("same-timestamp replacement reused old generation")
	}
}

// TestPlaybackTransferIntentCannotOutliveMediaProof keeps a shorter proof TTL
// authoritative even though the transfer permission normally lasts 30 seconds.
func TestPlaybackTransferIntentCannotOutliveMediaProof(t *testing.T) {
	now := time.Now()
	cache := newPlaybackProofCache(4, 5*time.Second)
	cache.now = func() time.Time { return now }
	proof := fixturePlaybackProof("mapping-1", "item-1", "source-1", "session-1")
	proof.transferIntent = true
	cache.Record([]PlaybackProof{proof})
	current, _ := cache.Lookup("mapping-1", "item-1", "source-1", "session-1")
	if !cache.CanCreateTransfer(current, fixturePrincipal()) {
		t.Fatal("initial intent missing")
	}
	now = now.Add(5 * time.Second)
	if cache.CanCreateTransfer(current, fixturePrincipal()) {
		t.Fatal("intent outlived media proof")
	}
}

// TestGatewayTransferIntentStoppedRevokesWithoutLeaseService keeps intent
// revocation independent from Redis while preserving other proofs and sessions.
func TestGatewayTransferIntentStoppedRevokesWithoutLeaseService(t *testing.T) {
	for _, test := range []struct {
		name, source string
		status       int
		want         bool
	}{
		{"matching", `,"MediaSourceId":"source-1"`, http.StatusNoContent, false},
		{"source omitted", "", http.StatusNoContent, false},
		{"other source", `,"MediaSourceId":"other"`, http.StatusNoContent, true},
		{"null source ambiguous", `,"MediaSourceId":null`, http.StatusNoContent, true},
		{"duplicate source", `,"MediaSourceId":"other","MediaSourceId":"source-1"`, http.StatusNoContent, true},
		{"case duplicate session", `,"playSessionId":"session-1"`, http.StatusNoContent, true},
		{"failed upstream", "", http.StatusBadGateway, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			gateway := newTransferIntentGateway(t, nil, http.StatusOK, transferIntentResponse)
			sendTransferIntentPlaybackInfo(gateway, `{"IsPlayback":true}`)
			proof, _ := gateway.LookupPlaybackProof(fixturePrincipal(), "item-1", "source-1", "session-1")
			gateway.transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
				return transferIntentHTTPResponse(r, test.status, ""), nil
			})
			gateway.proxy.Transport = gateway.transport
			request := httptest.NewRequest(http.MethodPost, "/Sessions/Playing/Stopped", strings.NewReader(`{"ItemId":"item-1","PlaySessionId":"session-1"`+test.source+`}`))
			request.Header.Set(accessTokenHeader, fixtureAccessToken)
			request.Header.Set("Content-Type", "application/json")
			response := httptest.NewRecorder()
			gateway.ServeHTTP(response, request)
			if got := gateway.proofs.CanCreateTransfer(proof, fixturePrincipal()); got != test.want || response.Code != test.status {
				t.Fatalf("intent=%t status=%d", got, response.Code)
			}
			if _, ok := gateway.LookupPlaybackProof(fixturePrincipal(), "item-1", "source-1", "session-1"); !ok {
				t.Fatal("Stopped discarded reusable proof")
			}
		})
	}
}

// TestGatewayVideoUsesLiveTransferIntent verifies the real Gateway-to-service
// handoff, including existing-file reuse and intent expiry while queued.
func TestGatewayVideoUsesLiveTransferIntent(t *testing.T) {
	for _, test := range []struct {
		name                     string
		intent, existing, expire bool
		want                     int
	}{
		{"detail new file", false, false, false, http.StatusPartialContent},
		{"explicit playback", true, false, false, http.StatusFound},
		{"detail existing file", false, true, false, http.StatusFound},
		{"expires while queued", true, false, true, http.StatusPartialContent},
	} {
		t.Run(test.name, func(t *testing.T) {
			now := time.Now()
			called := false
			service := transferIntentDirectPlayFunc(func(_ context.Context, request directplay.MediaPathResolveRequest) (directplay.RedirectCandidate, error) {
				called = true
				if test.expire {
					now = now.Add(30 * time.Second)
				}
				if request.CanCreateTransfer == nil {
					t.Fatal("live checker missing")
				}
				if !test.existing && !request.CanCreateTransfer() {
					return directplay.RedirectCandidate{}, directplay.ErrPlaybackIntentRequired
				}
				return validFixtureRedirectCandidate(), nil
			})
			gateway := newTransferIntentGateway(t, service, http.StatusOK, transferIntentResponse)
			var logs bytes.Buffer
			gateway.logger = log.New(&logs, "", 0)
			gateway.proofs.now = func() time.Time { return now }
			if test.intent {
				sendTransferIntentPlaybackInfo(gateway, `{"IsPlayback":true}`)
			} else {
				sendTransferIntentPlaybackInfo(gateway, `{}`)
			}
			response := httptest.NewRecorder()
			gateway.ServeHTTP(response, newVideoRequest(http.MethodGet, "/Videos/item-1/stream.mkv?MediaSourceId=source-1&PlaySessionId=session-1&Static=true"))
			if !called || response.Code != test.want {
				t.Fatalf("called=%t status=%d want=%d", called, response.Code, test.want)
			}
			if test.want == http.StatusPartialContent && !strings.Contains(logs.String(), "reasonCode=playback_intent_required") {
				t.Fatalf("missing intent reason: %s", logs.String())
			}
		})
	}
}

// TestGatewayInternalPlaybackInfoNeverGrantsTransferIntent covers the actual
// first-detail path: Gateway GET resolves media but cannot create a new file.
func TestGatewayInternalPlaybackInfoNeverGrantsTransferIntent(t *testing.T) {
	called := false
	service := transferIntentDirectPlayFunc(func(_ context.Context, request directplay.MediaPathResolveRequest) (directplay.RedirectCandidate, error) {
		called = true
		if request.CanCreateTransfer == nil || request.CanCreateTransfer() {
			t.Fatal("internal GET granted transfer intent")
		}
		return directplay.RedirectCandidate{}, directplay.ErrPlaybackIntentRequired
	})
	gateway := newTransferIntentGateway(t, service, http.StatusOK, transferIntentResponse)
	response := httptest.NewRecorder()
	gateway.ServeHTTP(response, newVideoRequest(http.MethodGet, "/Videos/item-1/stream?MediaSourceId=source-1&Static=true"))
	if !called || response.Code != http.StatusPartialContent || gateway.proofs.Len() != 1 {
		t.Fatalf("called=%t status=%d proofs=%d", called, response.Code, gateway.proofs.Len())
	}
}

// TestGatewayQueuedVideoCannotReuseStoppedOrReplacedIntent simulates a
// DirectPlay lock wait with a live checker, including a missing-session URL.
func TestGatewayQueuedVideoCannotReuseStoppedOrReplacedIntent(t *testing.T) {
	for _, action := range []string{"stopped", "replaced", "path changed"} {
		t.Run(action, func(t *testing.T) {
			var gateway *Gateway
			service := transferIntentDirectPlayFunc(func(_ context.Context, request directplay.MediaPathResolveRequest) (directplay.RedirectCandidate, error) {
				if request.CanCreateTransfer == nil || !request.CanCreateTransfer() {
					t.Fatal("initial checker rejected active intent")
				}
				switch action {
				case "stopped":
					gateway.updatePlaybackSessionLease(context.Background(), fixturePrincipal(), playbackSessionEventSnapshot{
						kind: playbackSessionEventStop, snapshotState: "recorded", itemID: "item-1", playSessionID: "session-1", intentRevocationEligible: true,
					})
				case "replaced":
					sendTransferIntentPlaybackInfo(gateway, `{"IsPlayback":true}`)
				case "path changed":
					gateway.proofs.mu.Lock()
					for key, proof := range gateway.proofs.entries {
						proof.Path = "/other.mkv"
						gateway.proofs.entries[key] = proof
					}
					gateway.proofs.mu.Unlock()
				}
				if request.CanCreateTransfer() {
					t.Fatal("queued request borrowed expired or replaced grant")
				}
				return directplay.RedirectCandidate{}, directplay.ErrPlaybackIntentRequired
			})
			gateway = newTransferIntentGateway(t, service, http.StatusOK, transferIntentResponse)
			sendTransferIntentPlaybackInfo(gateway, `{"IsPlayback":true}`)
			response := httptest.NewRecorder()
			gateway.ServeHTTP(response, newVideoRequest(http.MethodGet, "/Videos/item-1/stream?MediaSourceId=source-1&Static=true"))
			if response.Code != http.StatusPartialContent {
				t.Fatalf("status=%d", response.Code)
			}
		})
	}
}

// TestPlaybackTransferIntentRevocationIsScopedAndIndependentFromRedis verifies
// session/source isolation and both missing-lease and Redis-failure paths.
func TestPlaybackTransferIntentRevocationIsScopedAndIndependentFromRedis(t *testing.T) {
	for _, leaseService := range []*fakePlaybackSessionService{nil, {}, {err: directplay.ErrRedisUnavailable}} {
		gateway := newTransferIntentGateway(t, nil, http.StatusOK, transferIntentResponse)
		if leaseService != nil {
			gateway.playbackSessionService = leaseService
		}
		proofs := []PlaybackProof{
			fixturePlaybackProof("mapping-1", "item-1", "source-1", "session-1"),
			fixturePlaybackProof("mapping-1", "item-1", "source-2", "session-1"),
			fixturePlaybackProof("mapping-1", "item-1", "source-1", "session-2"),
			fixturePlaybackProof("mapping-1", "item-2", "source-1", "session-1"),
			fixturePlaybackProof("mapping-2", "item-1", "source-1", "session-1"),
		}
		for index := range proofs {
			proofs[index].transferIntent = true
		}
		gateway.proofs.Record(proofs)
		gateway.updatePlaybackSessionLease(context.Background(), fixturePrincipal(), playbackSessionEventSnapshot{
			kind: playbackSessionEventStop, snapshotState: "recorded", itemID: "item-1", playSessionID: "session-1", intentRevocationEligible: true,
		})
		for index, original := range proofs {
			proof, ok := gateway.proofs.Lookup(original.MappingID, original.ItemID, original.MediaSourceID, original.PlaySessionID)
			principal := fixturePrincipal()
			principal.MappingID = proof.MappingID
			if !ok || gateway.proofs.CanCreateTransfer(proof, principal) != (index > 1) {
				t.Fatalf("scope changed at proof %d", index)
			}
		}
	}
	for _, mutate := range []func(*embytoken.Principal){
		func(p *embytoken.Principal) { p.MappingID = "other" },
		func(p *embytoken.Principal) { p.ServerID = "other" },
		func(p *embytoken.Principal) { p.User.ID = "other" },
		func(p *embytoken.Principal) { p.User.EmbyID = "other" },
		func(p *embytoken.Principal) { p.DeviceID = "other" },
	} {
		gateway := newTransferIntentGateway(t, nil, http.StatusOK, transferIntentResponse)
		sendTransferIntentPlaybackInfo(gateway, `{"IsPlayback":true}`)
		principal := fixturePrincipal()
		mutate(&principal)
		if gateway.proofs.RevokeTransferIntent(principal, "item-1", "source-1", "session-1") != 0 {
			t.Fatal("foreign identity revoked intent")
		}
	}
}

// TestPlaybackTransferIntentConcurrentAccess protects shared proof state used
// by concurrent videos, PlaybackInfo responses and Stopped notifications.
func TestPlaybackTransferIntentConcurrentAccess(t *testing.T) {
	cache := newPlaybackProofCache(32, time.Minute)
	var workers sync.WaitGroup
	for index := 0; index < 24; index++ {
		workers.Add(1)
		go func(index int) {
			defer workers.Done()
			item := fmt.Sprintf("item-%d", index)
			proof := fixturePlaybackProof("mapping-1", item, "source-1", "session-1")
			proof.transferIntent = true
			cache.Record([]PlaybackProof{proof})
			current, _ := cache.Lookup("mapping-1", item, "source-1", "session-1")
			if !cache.CanCreateTransfer(current, fixturePrincipal()) {
				t.Error("concurrent grant missing")
			}
			cache.RevokeTransferIntent(fixturePrincipal(), item, "", "session-1")
			if cache.CanCreateTransfer(current, fixturePrincipal()) {
				t.Error("concurrent revocation lost")
			}
		}(index)
	}
	workers.Wait()
}

// newTransferIntentGateway fakes every upstream request and keeps video
// fallback observable without starting a server or contacting external APIs.
func newTransferIntentGateway(t *testing.T, service DirectPlayService, status int, body string) *Gateway {
	t.Helper()
	return newVideoTestGateway(t, "http://emby.invalid", &fakeTokenService{principal: fixturePrincipal()}, service,
		roundTripFunc(func(r *http.Request) (*http.Response, error) {
			if strings.Contains(r.URL.Path, "/Videos/") {
				return transferIntentHTTPResponse(r, http.StatusPartialContent, "fallback"), nil
			}
			return transferIntentHTTPResponse(r, status, body), nil
		}), &bytes.Buffer{})
}

// sendTransferIntentPlaybackInfo exercises request and response sidecars.
func sendTransferIntentPlaybackInfo(gateway *Gateway, body string) {
	request := httptest.NewRequest(http.MethodPost, "/Items/item-1/PlaybackInfo", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set(accessTokenHeader, fixtureAccessToken)
	gateway.ServeHTTP(httptest.NewRecorder(), request)
}

// transferIntentHTTPResponse builds one transparent fake upstream response.
func transferIntentHTTPResponse(request *http.Request, status int, body string) *http.Response {
	return &http.Response{StatusCode: status, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(body)), Request: request}
}

type transferIntentDirectPlayFunc func(context.Context, directplay.MediaPathResolveRequest) (directplay.RedirectCandidate, error)

// ResolveMediaPath allows tests to invalidate intent after Gateway inspection.
func (fn transferIntentDirectPlayFunc) ResolveMediaPath(ctx context.Context, request directplay.MediaPathResolveRequest) (directplay.RedirectCandidate, error) {
	return fn(ctx, request)
}
