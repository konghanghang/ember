package playbackgateway

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/konghang/ember/backend/internal/services/p115quota"
	"github.com/konghang/ember/backend/internal/testutil/faketime"
)

// TestFixedSessionPlaybackRevalidatesAfterProofExpiry follows real memory-lease
// transitions while fake Emby returns a different session for the same media.
func TestFixedSessionPlaybackRevalidatesAfterProofExpiry(t *testing.T) {
	for _, file := range []string{"original.mp4", "original.mkv", "stream.mkv"} {
		for _, method := range []string{http.MethodGet, http.MethodHead} {
			t.Run(file+"/"+method, func(t *testing.T) {
				keys, err := p115quota.NewKeyDeriver("fixture-root-key")
				if err != nil {
					t.Fatal(err)
				}
				account, _ := keys.PlaybackAccountKey("100")
				h := &leaseIdentityHarness{keys: keys, account: account, store: p115quota.NewMemoryLeaseStore(), now: time.Now()}
				container := "mkv"
				if strings.HasSuffix(file, ".mp4") {
					container = "mp4"
				}
				var logs bytes.Buffer
				internalCalls, videoFallbacks := 0, 0
				gateway := newVideoTestGateway(t, "http://emby.invalid", &fakeTokenService{principal: fixturePrincipal()}, h,
					roundTripFunc(func(request *http.Request) (*http.Response, error) {
						switch {
						case strings.Contains(request.URL.Path, "/Sessions/"):
							return transferIntentHTTPResponse(request, http.StatusNoContent, ""), nil
						case strings.Contains(request.URL.Path, "/PlaybackInfo"):
							internalCalls++
							if request.Method != http.MethodGet || request.Header.Get(accessTokenHeader) != fixtureAccessToken ||
								request.URL.Query().Get("UserId") != "emby-user-1" || request.URL.Query().Get("api_key") != "" || request.Header.Get("Range") != "" {
								t.Error("revalidation changed the versioned user-token query contract")
							}
							return transferIntentHTTPResponse(request, http.StatusOK, revalidationResponse(container, "new-internal-session")), nil
						default:
							videoFallbacks++
							return transferIntentHTTPResponse(request, http.StatusPartialContent, "fallback"), nil
						}
					}), &logs)
				gateway.playbackSessionService = h
				gateway.proofs.now = func() time.Time { return h.now }
				proof := revalidationProof(container, "session-1")
				gateway.proofs.Record([]PlaybackProof{proof})
				target := "/Videos/item-1/" + file + "?MediaSourceId=source-1&PlaySessionId=session-1&DeviceId=device-1"
				if file == "stream.mkv" {
					target += "&Static=true"
				}
				first := httptest.NewRecorder()
				gateway.ServeHTTP(first, newVideoRequest(http.MethodGet, target))
				if first.Code != http.StatusFound {
					t.Fatalf("initial status=%d", first.Code)
				}
				for i := 0; i < 7; i++ {
					if i > 0 {
						h.now = h.now.Add(time.Minute)
					}
					sendRevalidationProgress(t, gateway, "session-1", false)
					if !h.result.Found {
						t.Fatal("progress lost the admitted client lease")
					}
					gateway.proofs.Len()
				}
				request := newVideoRequest(method, target)
				request.Header.Set("Range", "bytes=512-")
				response := httptest.NewRecorder()
				gateway.ServeHTTP(response, request)
				if response.Code != http.StatusFound || internalCalls != 1 || videoFallbacks != 0 {
					t.Fatalf("renewal status=%d internal=%d fallback=%d", response.Code, internalCalls, videoFallbacks)
				}
				if len(h.sessions) != 2 || h.sessions[1] != "session-1" {
					t.Fatal("renewal changed the original lease identity")
				}
				usage, _ := h.store.AccountUsage(context.Background(), account, h.now)
				if usage.OccupiedStreams != 1 {
					t.Fatal("renewal allocated another playback slot")
				}
				current, ok := gateway.proofs.Lookup("mapping-1", "item-1", "source-1", "session-1")
				if !ok || current.Path != proof.Path || gateway.proofs.CanCreateTransfer(current, fixturePrincipal()) {
					t.Fatal("renewal lost the fixed-session proof or granted strict transfer intent")
				}
				sendRevalidationProgress(t, gateway, "session-1", true)
				h.now = h.now.Add(7 * time.Minute)
				paused := httptest.NewRecorder()
				gateway.ServeHTTP(paused, newVideoRequest(method, target))
				if paused.Code != http.StatusFound || internalCalls != 2 || h.sessions[len(h.sessions)-1] != "session-1" {
					t.Fatal("paused live session did not survive proof expiry")
				}
				assertSecretsAbsent(t, logs.String(), fixtureAccessToken, "new-internal-session", fixtureRedirectURL)
			})
		}
	}
}

// revalidationProof supplies the existing local original-file contract without
// implying that the client sent an explicit transfer intent.
func revalidationProof(container, session string) PlaybackProof {
	proof := fixturePlaybackProof("mapping-1", "item-1", "source-1", session)
	proof.Path, proof.Container, proof.Size, proof.SupportsDirectStream = "/mnt/media/fixture."+container, container, 1024, true
	return proof
}

// revalidationResponse renders a fake successful Emby response for one source.
func revalidationResponse(container, session string) string {
	data := map[string]any{"PlaySessionId": session, "MediaSources": []any{map[string]any{
		"Id": "source-1", "ItemId": "item-1", "Path": "/mnt/media/fixture." + container,
		"Container": container, "Size": 1024, "SupportsDirectPlay": true, "SupportsDirectStream": true,
	}}}
	encoded, _ := json.Marshal(data)
	return string(encoded)
}

// sendRevalidationProgress advances only the already admitted fake lease.
func sendRevalidationProgress(t *testing.T, gateway *Gateway, session string, paused bool) {
	t.Helper()
	body := fmt.Sprintf(`{"ItemId":"item-1","MediaSourceId":"source-1","PlaySessionId":%q,"IsPaused":%t,"PositionTicks":42}`, session, paused)
	request := httptest.NewRequest(http.MethodPost, "/Sessions/Playing/Progress", strings.NewReader(body))
	request.Header.Set(accessTokenHeader, fixtureAccessToken)
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	gateway.ServeHTTP(response, request)
	if response.Code != http.StatusNoContent {
		t.Fatalf("progress status=%d", response.Code)
	}
}

// boundRevalidationGateway retains an admitted binding after its media proof
// expires, without needing real Redis, account storage or a running service.
func boundRevalidationGateway(t *testing.T, transport http.RoundTripper) (*Gateway, *faketime.Clock, *fakeDirectPlayService) {
	t.Helper()
	clock := faketime.New(time.Now())
	direct := &fakeDirectPlayService{result: validFixtureRedirectCandidate()}
	gateway := newVideoTestGateway(t, "http://emby.invalid", &fakeTokenService{principal: fixturePrincipal()}, direct, transport, &bytes.Buffer{})
	gateway.proofs.now, gateway.proofs.ttl = clock.Now, time.Minute
	proof := revalidationProof("mkv", "session-1")
	gateway.proofs.Record([]PlaybackProof{proof})
	proof, _ = gateway.proofs.Lookup(proof.MappingID, proof.ItemID, proof.MediaSourceID, proof.PlaySessionID)
	gateway.proofs.rememberPlaybackSession(proof, fixturePrincipal())
	gateway.proofs.updatePlaybackBinding(fixturePrincipal(), proof.ItemID, proof.MediaSourceID, proof.PlaySessionID, p115quota.LeaseStateActive, true)
	clock.Advance(time.Minute + time.Second)
	gateway.proofs.Len()
	return gateway, clock, direct
}

// TestFixedSessionRevalidationFailsClosed covers media/protocol failures and
// ineligible requests while keeping the exact video fallback unchanged.
func TestFixedSessionRevalidationFailsClosed(t *testing.T) {
	for _, kind := range []string{"path", "container", "size", "source", "remote", "capabilities", "direct_stream", "zero_mkv_size", "json", "status", "transport", "expired_binding", "stopped", "unknown_query", "transform", "duplicate_static", "wrong_device", "wrong_session", "wrong_principal"} {
		t.Run(kind, func(t *testing.T) {
			target := "/emby/Videos/item-1/original.mkv?MediaSourceId=source-1&PlaySessionId=session-1&DeviceId=device-1"
			responseBody := revalidationResponse("mkv", "new-internal")
			wantCalls := 1
			switch kind {
			case "path":
				responseBody = strings.Replace(responseBody, "fixture.mkv", "changed.mkv", 1)
			case "container":
				responseBody = strings.Replace(responseBody, `"Container":"mkv"`, `"Container":"mp4"`, 1)
			case "size":
				responseBody = strings.Replace(responseBody, `"Size":1024`, `"Size":2048`, 1)
			case "source":
				responseBody = strings.Replace(responseBody, "source-1", "other-source", 1)
			case "remote":
				responseBody = strings.Replace(responseBody, `"Container":`, `"IsRemote":true,"Container":`, 1)
			case "capabilities":
				responseBody = strings.Replace(responseBody, `"SupportsDirectPlay":true`, `"SupportsDirectPlay":false`, 1)
			case "direct_stream":
				responseBody = strings.Replace(responseBody, `"SupportsDirectStream":true`, `"SupportsDirectStream":false`, 1)
			case "zero_mkv_size":
				responseBody = strings.Replace(responseBody, `"Size":1024`, `"Size":0`, 1)
			case "json":
				responseBody = "invalid-json"
			case "unknown_query":
				target += "&unknown=value"
				wantCalls = 0
			case "transform":
				target = strings.Replace(target, "original.mkv", "stream.mkv", 1) + "&Static=true&AudioStreamIndex=1"
				wantCalls = 0
			case "duplicate_static":
				target += "&Static=true&static=true"
				wantCalls = 0
			case "wrong_device":
				target = strings.Replace(target, "device-1", "other-device", 1)
				wantCalls = 0
			case "wrong_session":
				target = strings.Replace(target, "session-1", "other-session", 1)
				wantCalls = 0
			case "expired_binding", "stopped", "wrong_principal":
				wantCalls = 0
			}
			calls, fallbacks := 0, 0
			gateway, clock, direct := boundRevalidationGateway(t, roundTripFunc(func(request *http.Request) (*http.Response, error) {
				if strings.Contains(request.URL.Path, "/PlaybackInfo") {
					calls++
					if kind == "transport" {
						return nil, errors.New("fixture-transport-error")
					}
					status := http.StatusOK
					if kind == "status" {
						status = http.StatusForbidden
					}
					return transferIntentHTTPResponse(request, status, responseBody), nil
				}
				fallbacks++
				if request.Method != http.MethodGet || request.URL.RequestURI() != target || request.Header.Get("Range") != "bytes=512-" || request.Header.Get("User-Agent") != "FixturePlayer/1" {
					t.Error("revalidation failure changed the original video request")
				}
				return &http.Response{StatusCode: http.StatusPartialContent, Request: request, Header: http.Header{}, Body: io.NopCloser(strings.NewReader("source-range"))}, nil
			}))
			switch kind {
			case "expired_binding":
				clock.Advance(2 * time.Minute)
			case "stopped":
				gateway.proofs.stopPlaybackBinding(fixturePrincipal(), "item-1", "source-1", "session-1")
			case "wrong_principal":
				gateway.tokenService.(*fakeTokenService).principal.DeviceID = "other-device"
			}
			request := newVideoRequest(http.MethodGet, target)
			request.Header.Set("Range", "bytes=512-")
			request.Header.Set("User-Agent", "FixturePlayer/1")
			response := httptest.NewRecorder()
			gateway.ServeHTTP(response, request)
			if response.Code != http.StatusPartialContent || response.Body.String() != "source-range" || calls != wantCalls || fallbacks != 1 || len(direct.snapshot()) != 0 {
				t.Fatalf("status=%d calls=%d want=%d fallbacks=%d direct=%d", response.Code, calls, wantCalls, fallbacks, len(direct.snapshot()))
			}
		})
	}
}

// TestFixedSessionRevalidationSupersededDuringRead proves successful stale
// responses cannot revive a stopped session or overwrite a new client result.
func TestFixedSessionRevalidationSupersededDuringRead(t *testing.T) {
	for _, action := range []string{"stop", "client_begin", "record", "missing_lease", "binding_expired", "other_device_stop"} {
		t.Run(action, func(t *testing.T) {
			started, release := make(chan struct{}), make(chan struct{})
			gateway, clock, _ := boundRevalidationGateway(t, roundTripFunc(func(request *http.Request) (*http.Response, error) {
				close(started)
				<-release
				return transferIntentHTTPResponse(request, http.StatusOK, revalidationResponse("mkv", "new-internal")), nil
			}))
			request := newVideoRequest(http.MethodGet, "/emby/Videos/item-1/original.mkv?MediaSourceId=source-1&PlaySessionId=session-1")
			type result struct {
				proof     PlaybackProof
				reason    string
				attempted bool
			}
			done := make(chan result, 1)
			go func() {
				proof, reason, attempted := gateway.revalidatePlaybackProof(request, fixturePrincipal(), fixtureAccessToken, inspectVideoRequest(request))
				done <- result{proof, reason, attempted}
			}()
			<-started
			switch action {
			case "stop":
				gateway.proofs.stopPlaybackBinding(fixturePrincipal(), "item-1", "", "session-1")
			case "client_begin":
				guard := gateway.proofs.BeginClient("mapping-1", "item-1")
				defer gateway.proofs.ReleasePublication(guard)
			case "record":
				gateway.proofs.Record([]PlaybackProof{revalidationProof("mkv", "session-1")})
			case "missing_lease":
				gateway.proofs.updatePlaybackBinding(fixturePrincipal(), "item-1", "", "session-1", p115quota.LeaseStateActive, false)
			case "binding_expired":
				clock.Advance(2 * time.Minute)
			case "other_device_stop":
				principal := fixturePrincipal()
				principal.DeviceID = "another-device"
				gateway.proofs.stopPlaybackBinding(principal, "item-1", "", "session-1")
			}
			close(release)
			got := <-done
			if !got.attempted || (got.reason == "") != (action == "other_device_stop") {
				t.Fatalf("attempted=%t reason=%s", got.attempted, got.reason)
			}
			gateway.proofs.mu.Lock()
			defer gateway.proofs.mu.Unlock()
			if len(gateway.proofs.revalidations) != 0 {
				t.Fatal("completed refresh retained its flight")
			}
			for guard := range gateway.proofs.publications {
				if guard.revalidation != nil {
					t.Fatal("completed refresh retained its publication guard")
				}
			}
		})
	}
}

// TestFixedSessionRevalidationCoalescesAndKeepsOtherWaiter exercises one
// canceled waiter alongside another authorized waiter without scheduling sleeps.
func TestFixedSessionRevalidationCoalescesAndKeepsOtherWaiter(t *testing.T) {
	started, release := make(chan struct{}), make(chan struct{})
	var calls atomic.Int32
	gateway, _, _ := boundRevalidationGateway(t, roundTripFunc(func(request *http.Request) (*http.Response, error) {
		if calls.Add(1) == 1 {
			close(started)
		}
		<-release
		return transferIntentHTTPResponse(request, http.StatusOK, revalidationResponse("mkv", "new-internal")), nil
	}))
	base, cancel := context.WithCancel(context.Background())
	defer cancel()
	contexts := []*playbackFlightWaitContext{
		{Context: base, joined: make(chan struct{})},
		{Context: context.Background(), joined: make(chan struct{})},
	}
	reasons := make(chan string, 2)
	var workers sync.WaitGroup
	for _, ctx := range contexts {
		workers.Add(1)
		go func(ctx context.Context) {
			defer workers.Done()
			request := newVideoRequest(http.MethodGet, "/emby/Videos/item-1/original.mkv?MediaSourceId=source-1&PlaySessionId=session-1").WithContext(ctx)
			_, reason, _ := gateway.revalidatePlaybackProof(request, fixturePrincipal(), fixtureAccessToken, inspectVideoRequest(request))
			reasons <- reason
		}(ctx)
	}
	<-started
	for _, ctx := range contexts {
		<-ctx.joined
	}
	cancel()
	if reason := <-reasons; reason != "request_canceled" {
		t.Fatalf("canceled reason=%s", reason)
	}
	close(release)
	if reason := <-reasons; reason != "" {
		t.Fatalf("active reason=%s", reason)
	}
	workers.Wait()
	if calls.Load() != 1 {
		t.Fatalf("upstream calls=%d", calls.Load())
	}
}

// TestFixedSessionRevalidationAllCanceledCannotPublishOrDeleteRetry holds an
// obsolete transport response past cancellation and a new same-key owner.
func TestFixedSessionRevalidationAllCanceledCannotPublishOrDeleteRetry(t *testing.T) {
	firstStarted, secondStarted := make(chan struct{}), make(chan struct{})
	firstRelease, secondRelease := make(chan struct{}), make(chan struct{})
	var calls atomic.Int32
	gateway, _, _ := boundRevalidationGateway(t, roundTripFunc(func(request *http.Request) (*http.Response, error) {
		if calls.Add(1) == 1 {
			close(firstStarted)
			<-firstRelease // Deliberately model transport returning after cancellation.
		} else {
			close(secondStarted)
			<-secondRelease
		}
		return transferIntentHTTPResponse(request, http.StatusOK, revalidationResponse("mkv", "new-internal")), nil
	}))
	request := newVideoRequest(http.MethodGet, "/emby/Videos/item-1/original.mkv?MediaSourceId=source-1&PlaySessionId=session-1")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	firstResult := make(chan string, 1)
	go func() {
		_, reason, _ := gateway.revalidatePlaybackProof(request.WithContext(ctx), fixturePrincipal(), fixtureAccessToken, inspectVideoRequest(request))
		firstResult <- reason
	}()
	<-firstStarted
	gateway.proofs.mu.Lock()
	var obsolete *playbackRevalidationCall
	for _, call := range gateway.proofs.revalidations {
		obsolete = call
	}
	gateway.proofs.mu.Unlock()
	cancel()
	if reason := <-firstResult; reason != "request_canceled" {
		t.Fatalf("first reason=%s", reason)
	}
	if obsolete == nil || obsolete.ctx.Err() == nil {
		t.Fatal("last waiter did not cancel shared work")
	}
	secondResult := make(chan string, 1)
	go func() {
		_, reason, _ := gateway.revalidatePlaybackProof(request, fixturePrincipal(), fixtureAccessToken, inspectVideoRequest(request))
		secondResult <- reason
	}()
	<-secondStarted
	close(firstRelease)
	<-obsolete.done
	if _, found := gateway.proofs.Lookup("mapping-1", "item-1", "source-1", "session-1"); found {
		t.Fatal("canceled response published a proof")
	}
	gateway.proofs.mu.Lock()
	if len(gateway.proofs.revalidations) != 1 || gateway.proofs.revalidations[obsolete.key] == obsolete {
		gateway.proofs.mu.Unlock()
		t.Fatal("old completion removed the retry owner")
	}
	gateway.proofs.mu.Unlock()
	close(secondRelease)
	if reason := <-secondResult; reason != "" {
		t.Fatalf("retry reason=%s", reason)
	}
	if calls.Load() != 2 {
		t.Fatalf("calls=%d", calls.Load())
	}
	gateway.proofs.mu.Lock()
	defer gateway.proofs.mu.Unlock()
	if len(gateway.proofs.revalidations) != 0 || len(gateway.proofs.publications) != 0 {
		t.Fatal("completed retry leaked owner state")
	}
}

// TestFixedSessionRevalidationSeparatesSessions prevents two live sessions of
// the same file from sharing one owner's original-session authorization alias.
func TestFixedSessionRevalidationSeparatesSessions(t *testing.T) {
	started, release := make(chan struct{}, 2), make(chan struct{})
	var calls atomic.Int32
	gateway, clock, _ := boundRevalidationGateway(t, roundTripFunc(func(request *http.Request) (*http.Response, error) {
		index := calls.Add(1)
		started <- struct{}{}
		<-release
		return transferIntentHTTPResponse(request, http.StatusOK, revalidationResponse("mkv", fmt.Sprintf("internal-%d", index))), nil
	}))
	gateway.proofs.updatePlaybackBinding(fixturePrincipal(), "item-1", "source-1", "session-1", p115quota.LeaseStateActive, true)
	second := revalidationProof("mkv", "session-2")
	gateway.proofs.Record([]PlaybackProof{second})
	second, _ = gateway.proofs.Lookup("mapping-1", "item-1", "source-1", "session-2")
	gateway.proofs.rememberPlaybackSession(second, fixturePrincipal())
	gateway.proofs.updatePlaybackBinding(fixturePrincipal(), "item-1", "source-1", "session-2", p115quota.LeaseStateActive, true)
	clock.Advance(time.Minute + time.Second)
	gateway.proofs.Len()
	type result struct {
		expected string
		proof    PlaybackProof
		reason   string
	}
	results := make(chan result, 2)
	for _, session := range []string{"session-1", "session-2"} {
		go func(session string) {
			request := newVideoRequest(http.MethodGet, "/emby/Videos/item-1/original.mkv?MediaSourceId=source-1&PlaySessionId="+session)
			proof, reason, _ := gateway.revalidatePlaybackProof(request, fixturePrincipal(), fixtureAccessToken, inspectVideoRequest(request))
			results <- result{session, proof, reason}
		}(session)
	}
	<-started
	<-started
	close(release)
	for i := 0; i < 2; i++ {
		got := <-results
		if got.reason != "" || got.proof.PlaySessionID != got.expected || gateway.proofs.CanCreateTransfer(got.proof, fixturePrincipal()) {
			t.Fatalf("session=%s alias=%s reason=%s", got.expected, got.proof.PlaySessionID, got.reason)
		}
	}
	if calls.Load() != 2 {
		t.Fatalf("calls=%d", calls.Load())
	}
}

// TestFixedSessionRevalidationSupersedesOlderPlainPublication prevents a late
// whole-item response from deleting the newer exact-session alias. Another
// device's independent query retains its own publication authority.
func TestFixedSessionRevalidationSupersedesOlderPlainPublication(t *testing.T) {
	for _, otherDevice := range []bool{false, true} {
		t.Run(fmt.Sprintf("otherDevice=%t", otherDevice), func(t *testing.T) {
			plainStarted, plainRelease := make(chan struct{}), make(chan struct{})
			var calls atomic.Int32
			gateway, _, _ := boundRevalidationGateway(t, roundTripFunc(func(request *http.Request) (*http.Response, error) {
				if calls.Add(1) == 1 {
					close(plainStarted)
					<-plainRelease
					return transferIntentHTTPResponse(request, http.StatusOK, revalidationResponse("mkv", "late-plain")), nil
				}
				return transferIntentHTTPResponse(request, http.StatusOK, revalidationResponse("mkv", "fresh-fixed")), nil
			}))
			plainPrincipal := fixturePrincipal()
			if otherDevice {
				plainPrincipal.DeviceID = "other-device"
			}
			plainResult := make(chan string, 1)
			go func() {
				request := newVideoRequest(http.MethodGet, "/emby/Videos/item-1/stream?MediaSourceId=source-1&Static=true")
				_, reason := gateway.resolvePlaybackInfoOnDemand(request, plainPrincipal, fixtureAccessToken, "item-1", "source-1")
				plainResult <- reason
			}()
			<-plainStarted
			fixedRequest := newVideoRequest(http.MethodGet, "/emby/Videos/item-1/original.mkv?MediaSourceId=source-1&PlaySessionId=session-1")
			fresh, reason, attempted := gateway.revalidatePlaybackProof(fixedRequest, fixturePrincipal(), fixtureAccessToken, inspectVideoRequest(fixedRequest))
			close(plainRelease)
			if !attempted || reason != "" {
				t.Fatalf("fixed attempted=%t reason=%s", attempted, reason)
			}
			plainReason := <-plainResult
			wantReason := "playback_info_superseded"
			if otherDevice {
				wantReason = ""
			}
			if plainReason != wantReason {
				t.Fatalf("plain reason=%s", plainReason)
			}
			current, found := gateway.proofs.Lookup("mapping-1", "item-1", "source-1", "session-1")
			if !found || current.generation != fresh.generation || gateway.proofs.CanCreateTransfer(current, fixturePrincipal()) {
				t.Fatal("late whole-item publication removed the fixed-session alias")
			}
		})
	}
}
