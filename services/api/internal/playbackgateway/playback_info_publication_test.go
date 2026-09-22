package playbackgateway

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/konghang/ember/backend/internal/services/directplay"
)

// TestLateInternalPlaybackInfoCannotReplaceClientIntent reproduces a detail
// probe finishing after an explicit playback has entered DirectPlay.
func TestLateInternalPlaybackInfoCannotReplaceClientIntent(t *testing.T) {
	getStarted, releaseGET := make(chan struct{}), make(chan struct{})
	videoStarted, releaseVideo := make(chan struct{}), make(chan struct{})
	intentAfterGET := make(chan bool, 1)
	service := transferIntentDirectPlayFunc(func(_ context.Context, request directplay.MediaPathResolveRequest) (directplay.RedirectCandidate, error) {
		if request.CanCreateTransfer == nil || !request.CanCreateTransfer() {
			t.Error("explicit playback did not enter DirectPlay with intent")
		}
		close(videoStarted)
		<-releaseVideo
		allowed := request.CanCreateTransfer != nil && request.CanCreateTransfer()
		intentAfterGET <- allowed
		if !allowed {
			return directplay.RedirectCandidate{}, directplay.ErrPlaybackIntentRequired
		}
		return validFixtureRedirectCandidate(), nil
	})
	gateway := newVideoTestGateway(t, "http://emby.invalid", &fakeTokenService{principal: fixturePrincipal()}, service,
		roundTripFunc(func(request *http.Request) (*http.Response, error) {
			if strings.Contains(request.URL.Path, "/PlaybackInfo") {
				if request.Method == http.MethodGet {
					close(getStarted)
					<-releaseGET
				}
				return transferIntentHTTPResponse(request, http.StatusOK, transferIntentResponse), nil
			}
			return transferIntentHTTPResponse(request, http.StatusNotFound, "fallback"), nil
		}), &bytes.Buffer{})
	gateway.logger = log.New(io.Discard, "", 0)
	now := time.Now()
	gateway.proofs.now = func() time.Time { return now }
	getDone := make(chan string, 1)
	go func() {
		request := newVideoRequest(http.MethodGet, "/Videos/item-1/stream?MediaSourceId=source-1&Static=true")
		_, reason := gateway.resolvePlaybackInfoOnDemand(request, fixturePrincipal(), fixtureAccessToken, "item-1", "source-1")
		getDone <- reason
	}()
	<-getStarted
	sendTransferIntentPlaybackInfo(gateway, `{"IsPlayback":true}`)
	clientProof, ok := gateway.LookupPlaybackProof(fixturePrincipal(), "item-1", "source-1", "session-1")
	if !ok || !gateway.proofs.CanCreateTransfer(clientProof, fixturePrincipal()) {
		t.Fatal("client response did not establish intent")
	}
	videoDone := make(chan *httptest.ResponseRecorder, 1)
	go func() {
		response := httptest.NewRecorder()
		gateway.ServeHTTP(response, newVideoRequest(http.MethodGet, "/Videos/item-1/stream.mkv?MediaSourceId=source-1&PlaySessionId=session-1&Static=true"))
		videoDone <- response
	}()
	<-videoStarted
	close(releaseGET)
	reason := <-getDone
	close(releaseVideo)
	response := <-videoDone
	if !<-intentAfterGET || response.Code != http.StatusFound {
		t.Fatalf("late internal GET revoked explicit playback: status=%d", response.Code)
	}
	if reason != "playback_info_superseded" {
		t.Fatalf("late probe must discard its response, reason=%q", reason)
	}
	current, _ := gateway.LookupPlaybackProof(fixturePrincipal(), "item-1", "source-1", "session-1")
	if current != clientProof {
		t.Fatal("late internal GET changed client proof generation, TTL or intent")
	}
}

// TestClientPlaybackInfoCompletionInvalidatesResolversStartedDuringBodyRead
// protects the gap between entry invalidation and final response publication.
func TestClientPlaybackInfoCompletionInvalidatesResolversStartedDuringBodyRead(t *testing.T) {
	for _, clientBody := range []string{transferIntentResponse, "{", `{"MediaSources":[],"PlaySessionId":"session-1"}`} {
		for _, publishDuringRead := range []bool{false, true} {
			t.Run(clientBody+map[bool]string{false: "/pending", true: "/published"}[publishDuringRead], func(t *testing.T) {
				gateway := newTransferIntentGateway(t, nil, http.StatusOK, transferIntentResponse)
				principal := fixturePrincipal()
				var guard *playbackProofPublishGuard
				response := transferIntentHTTPResponse(nil, http.StatusOK, clientBody)
				response.Body = &playbackInfoReadHook{reader: strings.NewReader(clientBody), beforeRead: func() {
					_, guard = gateway.proofs.BeginOnDemand(principal, "item-1", "source-1")
					if guard == nil {
						t.Fatal("resolver was not registered during client body read")
					}
					if publishDuringRead {
						proof := fixturePlaybackProof("mapping-1", "item-1", "source-1", "internal-session")
						if _, ok := gateway.proofs.PublishOnDemand(guard, []PlaybackProof{proof}); !ok {
							t.Fatal("intermediate internal publication failed")
						}
					}
				}}
				gateway.observePlaybackInfoResponse(response, requestRouteContext{
					principal: &principal, playbackInfoItemID: "item-1", playbackInfoEligible: true,
					playbackInfoIntent: playbackTransferIntent{requested: true},
				})
				proof := fixturePlaybackProof("mapping-1", "item-1", "source-1", "internal-session")
				if _, ok := gateway.proofs.PublishOnDemand(guard, []PlaybackProof{proof}); ok {
					t.Fatal("client completion did not invalidate pending internal response")
				}
				gateway.proofs.ReleasePublication(guard)
				if _, ok := gateway.proofs.Lookup("mapping-1", "item-1", "source-1", "internal-session"); ok {
					t.Fatal("intermediate internal snapshot survived client completion")
				}
				current, exists := gateway.LookupPlaybackProof(principal, "item-1", "source-1", "session-1")
				if clientBody == transferIntentResponse {
					if !exists || !gateway.proofs.CanCreateTransfer(current, principal) {
						t.Fatal("valid client completion lost intent")
					}
				} else if gateway.proofs.Len() != 0 {
					t.Fatal("rejected client completion retained an internal snapshot")
				}
			})
		}
	}
}

// TestOlderClientPlaybackInfoCompletionCannotReplaceNewerClientSnapshot
// prevents old body parsing, whether successful or failed, reversing a newer
// explicit client response that completed while the old body was being read.
func TestOlderClientPlaybackInfoCompletionCannotReplaceNewerClientSnapshot(t *testing.T) {
	for _, oldBody := range []string{"{", `{"MediaSources":[],"PlaySessionId":"session-1"}`, transferIntentResponse} {
		t.Run(oldBody, func(t *testing.T) {
			gateway := newTransferIntentGateway(t, nil, http.StatusOK, transferIntentResponse)
			principal := fixturePrincipal()
			var newer PlaybackProof
			response := transferIntentHTTPResponse(nil, http.StatusOK, oldBody)
			response.Body = &playbackInfoReadHook{reader: strings.NewReader(oldBody), beforeRead: func() {
				sendTransferIntentPlaybackInfo(gateway, `{"IsPlayback":true}`)
				newer, _ = gateway.LookupPlaybackProof(principal, "item-1", "source-1", "session-1")
				if !gateway.proofs.CanCreateTransfer(newer, principal) {
					t.Fatal("newer response did not establish intent")
				}
			}}
			gateway.observePlaybackInfoResponse(response, requestRouteContext{
				principal: &principal, playbackInfoItemID: "item-1", playbackInfoEligible: true,
			})
			current, ok := gateway.LookupPlaybackProof(principal, "item-1", "source-1", "session-1")
			if !ok || current != newer || !gateway.proofs.CanCreateTransfer(newer, principal) {
				t.Fatal("older client completion replaced the newer playback intent")
			}
			if count := activePlaybackPublications(gateway.proofs); count != 0 {
				t.Fatalf("client guard leak: %d", count)
			}
		})
	}
}

// TestClientPublicationCapacityFailureRemainsTransparentAndFailsClosed ensures
// bounded response observation cannot restore old proofs without a guard.
func TestClientPublicationCapacityFailureRemainsTransparentAndFailsClosed(t *testing.T) {
	gateway := newTransferIntentGateway(t, nil, http.StatusOK, transferIntentResponse)
	gateway.proofs = newPlaybackProofCache(1, time.Minute)
	old := fixturePlaybackProof("mapping-1", "item-1", "source-1", "session-old")
	old.transferIntent = true
	gateway.proofs.Record([]PlaybackProof{old})
	_, guard := gateway.proofs.BeginOnDemand(fixturePrincipal(), "item-1", "source-2")
	defer gateway.proofs.ReleasePublication(guard)
	request := httptest.NewRequest(http.MethodPost, "/Items/item-1/PlaybackInfo", strings.NewReader(`{"IsPlayback":true}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set(accessTokenHeader, fixtureAccessToken)
	response := httptest.NewRecorder()
	gateway.ServeHTTP(response, request)
	if response.Code != http.StatusOK || response.Body.String() != transferIntentResponse {
		t.Fatal("capacity rejection changed transparent response")
	}
	if gateway.proofs.Len() != 0 || activePlaybackPublications(gateway.proofs) != 1 {
		t.Fatal("capacity rejection kept old proof or created an unbounded guard")
	}
	if _, published := gateway.proofs.PublishOnDemand(guard, []PlaybackProof{old}); published {
		t.Fatal("unguarded client observation allowed older internal response to restore cache")
	}
}

// TestOnDemandPublicationGuardIsBoundedAndScoped ensures pending invalidated
// requests still occupy capacity until their exact owner releases them.
func TestOnDemandPublicationGuardIsBoundedAndScoped(t *testing.T) {
	cache := newPlaybackProofCache(2, time.Minute)
	principal := fixturePrincipal()
	_, first := cache.BeginOnDemand(principal, "item-1", "source-1")
	_, other := cache.BeginOnDemand(principal, "item-2", "source-1")
	if first == nil || other == nil {
		t.Fatal("initial guard missing")
	}
	cache.InvalidateItem("mapping-1", "item-1")
	if _, third := cache.BeginOnDemand(principal, "item-3", "source-1"); third != nil {
		t.Fatal("invalidated in-flight guard released capacity early")
	}
	if _, ok := cache.PublishOnDemand(first, nil); ok {
		t.Fatal("invalidated guard published")
	}
	proof := fixturePlaybackProof("mapping-1", "item-2", "source-1", "session-1")
	if count, ok := cache.PublishOnDemand(other, []PlaybackProof{proof}); !ok || count != 1 {
		t.Fatal("other item guard was invalidated")
	}
	cache.ReleasePublication(first)
	_, replacement := cache.BeginOnDemand(principal, "item-3", "source-1")
	if replacement == nil {
		t.Fatal("released capacity was not reusable")
	}
	cache.ReleasePublication(first)
	proof.ItemID = "item-3"
	if _, ok := cache.PublishOnDemand(replacement, []PlaybackProof{proof}); !ok {
		t.Fatal("double release removed a newer guard")
	}
	cache.ReleasePublication(other)
	cache.ReleasePublication(replacement)
	if count := activePlaybackPublications(cache); count != 0 {
		t.Fatalf("guards=%d", count)
	}
}

// TestOnDemandPublicationRejectsCapacityBeforeHTTP prevents rejected requests
// creating untracked external work when all request-lifetime slots are in use.
func TestOnDemandPublicationRejectsCapacityBeforeHTTP(t *testing.T) {
	gateway := newVideoTestGateway(t, "http://emby.invalid", &fakeTokenService{principal: fixturePrincipal()}, nil,
		roundTripFunc(func(*http.Request) (*http.Response, error) {
			t.Error("capacity rejection sent HTTP")
			return nil, errors.New("unexpected HTTP")
		}), &bytes.Buffer{})
	gateway.proofs = newPlaybackProofCache(1, time.Minute)
	_, guard := gateway.proofs.BeginOnDemand(fixturePrincipal(), "item-other", "source-1")
	defer gateway.proofs.ReleasePublication(guard)
	request := newVideoRequest(http.MethodGet, "/Videos/item-1/stream?MediaSourceId=source-1&Static=true")
	_, reason := gateway.resolvePlaybackInfoOnce(request, fixturePrincipal(), fixtureAccessToken, "item-1", "source-1")
	if reason != "playback_info_busy" || activePlaybackPublications(gateway.proofs) != 1 {
		t.Fatalf("reason=%q active=%d", reason, activePlaybackPublications(gateway.proofs))
	}
}

// TestProofRecordInvalidatesOnlyMatchingInternalPublisher protects callers
// using Record directly as well as the guarded client publication path.
func TestProofRecordInvalidatesOnlyMatchingInternalPublisher(t *testing.T) {
	cache := newPlaybackProofCache(4, time.Minute)
	_, guard := cache.BeginOnDemand(fixturePrincipal(), "item-1", "source-1")
	_, other := cache.BeginOnDemand(fixturePrincipal(), "item-2", "source-1")
	defer cache.ReleasePublication(guard)
	defer cache.ReleasePublication(other)
	proof := fixturePlaybackProof("mapping-1", "item-1", "source-1", "session-1")
	proof.transferIntent = true
	cache.Record([]PlaybackProof{proof})
	current, _ := cache.Lookup("mapping-1", "item-1", "source-1", "session-1")
	if _, ok := cache.PublishOnDemand(guard, nil); ok || !cache.CanCreateTransfer(current, fixturePrincipal()) {
		t.Fatal("direct Record did not protect its new client proof")
	}
	if _, ok := cache.PublishOnDemand(other, nil); !ok {
		t.Fatal("direct Record invalidated another item")
	}
}

// TestConcurrentInternalSourcesPublishOneItemSnapshot retains the existing
// response-wide media contract while preventing later flights reversing it.
func TestConcurrentInternalSourcesPublishOneItemSnapshot(t *testing.T) {
	cache := newPlaybackProofCache(4, time.Minute)
	principal := fixturePrincipal()
	_, first := cache.BeginOnDemand(principal, "item-1", "source-1")
	_, second := cache.BeginOnDemand(principal, "item-1", "source-2")
	otherPrincipal := principal
	otherPrincipal.MappingID = "mapping-2"
	_, other := cache.BeginOnDemand(otherPrincipal, "item-1", "source-1")
	defer cache.ReleasePublication(first)
	defer cache.ReleasePublication(second)
	defer cache.ReleasePublication(other)
	proofs := []PlaybackProof{
		fixturePlaybackProof("mapping-1", "item-1", "source-1", "session-current"),
		fixturePlaybackProof("mapping-1", "item-1", "source-2", "session-current"),
	}
	if count, ok := cache.PublishOnDemand(first, proofs); !ok || count != 2 {
		t.Fatalf("publication=%d ok=%t", count, ok)
	}
	current, _ := cache.Lookup("mapping-1", "item-1", "source-2", "session-current")
	stale := fixturePlaybackProof("mapping-1", "item-1", "source-2", "session-stale")
	if _, ok := cache.PublishOnDemand(second, []PlaybackProof{stale}); ok {
		t.Fatal("competing source overwrote current item snapshot")
	}
	after, _ := cache.Lookup("mapping-1", "item-1", "source-2", "session-current")
	if current != after {
		t.Fatal("stale publication changed generation or TTL")
	}
	otherProof := fixturePlaybackProof("mapping-2", "item-1", "source-1", "session-1")
	if _, ok := cache.PublishOnDemand(other, []PlaybackProof{otherProof}); !ok {
		t.Fatal("another mapping was invalidated")
	}
}

// TestOnDemandOwnerRechecksProofBeforeHTTP covers a client response arriving
// after an outer lookup miss but before the singleflight owner starts its GET.
func TestOnDemandOwnerRechecksProofBeforeHTTP(t *testing.T) {
	gateway := newVideoTestGateway(t, "http://emby.invalid", &fakeTokenService{principal: fixturePrincipal()}, nil,
		roundTripFunc(func(*http.Request) (*http.Response, error) {
			t.Error("owner sent HTTP despite current proof")
			return nil, errors.New("unexpected HTTP")
		}), &bytes.Buffer{})
	proof := fixturePlaybackProof("mapping-1", "item-1", "source-1", "session-1")
	proof.transferIntent = true
	gateway.proofs.Record([]PlaybackProof{proof})
	before, _ := gateway.LookupPlaybackProof(fixturePrincipal(), "item-1", "source-1", "session-1")
	request := newVideoRequest(http.MethodGet, "/Videos/item-1/stream?MediaSourceId=source-1&Static=true")
	resolved, reason := gateway.resolvePlaybackInfoOnce(request, fixturePrincipal(), fixtureAccessToken, "item-1", "source-1")
	after, _ := gateway.LookupPlaybackProof(fixturePrincipal(), "item-1", "source-1", "session-1")
	if reason != "" || resolved.PlaySessionID != "session-1" || before != after || activePlaybackPublications(gateway.proofs) != 0 {
		t.Fatal("atomic owner recheck changed proof or registered a guard")
	}
}

// TestOnDemandPublicationGuardsReleaseOnEveryResolverOutcome checks cleanup of
// the actual owner on HTTP, decoding, protocol, cancellation and panic paths.
func TestOnDemandPublicationGuardsReleaseOnEveryResolverOutcome(t *testing.T) {
	for _, outcome := range []string{"success", "no direct proof", "canceled", "deadline", "transport error", "nil response", "bad status", "bad content type", "nil body", "read failure", "too large", "decode failure", "invalid JSON", "panic"} {
		t.Run(outcome, func(t *testing.T) {
			gateway := newVideoTestGateway(t, "http://emby.invalid", &fakeTokenService{principal: fixturePrincipal()}, nil,
				roundTripFunc(func(request *http.Request) (*http.Response, error) {
					response := transferIntentHTTPResponse(request, http.StatusOK, transferIntentResponse)
					switch outcome {
					case "no direct proof":
						response.Body = io.NopCloser(strings.NewReader(strings.Replace(transferIntentResponse, `"SupportsDirectPlay":true`, `"SupportsDirectPlay":false`, 1)))
					case "canceled":
						return nil, context.Canceled
					case "deadline":
						return nil, context.DeadlineExceeded
					case "transport error":
						return nil, errors.New("fake transport failure")
					case "nil response":
						return nil, nil
					case "bad status":
						response.StatusCode = http.StatusBadGateway
					case "bad content type":
						response.Header.Set("Content-Type", "text/plain")
					case "nil body":
						response.Body = nil
					case "read failure":
						response.Body = &playbackInfoReadHook{readErr: errors.New("fake read failure")}
					case "too large":
						response.Body = io.NopCloser(strings.NewReader(strings.Repeat("x", 513)))
					case "decode failure":
						response.Header.Set("Content-Encoding", "gzip")
					case "invalid JSON":
						response.Body = io.NopCloser(strings.NewReader("{"))
					case "panic":
						panic("fake transport panic")
					}
					return response, nil
				}), &bytes.Buffer{})
			gateway.maxPlaybackInfoResponseBytes = 512
			request := newVideoRequest(http.MethodGet, "/Videos/item-1/stream?MediaSourceId=source-1&Static=true")
			_, reason := gateway.resolvePlaybackInfoOnDemand(request, fixturePrincipal(), fixtureAccessToken, "item-1", "source-1")
			if outcome != "success" && outcome != "no direct proof" && reason == "" {
				t.Fatal("failure scenario did not fail")
			}
			if count := activePlaybackPublications(gateway.proofs); count != 0 {
				t.Fatalf("guard leak after %s: %d", outcome, count)
			}
		})
	}
}

// TestCanceledWaiterDoesNotReleaseRunningOwnerGuard preserves the existing
// detached resolver lifetime and its bounded slot until the HTTP owner exits.
func TestCanceledWaiterDoesNotReleaseRunningOwnerGuard(t *testing.T) {
	started, release := make(chan struct{}), make(chan struct{})
	gateway := newVideoTestGateway(t, "http://emby.invalid", &fakeTokenService{principal: fixturePrincipal()}, nil,
		roundTripFunc(func(request *http.Request) (*http.Response, error) {
			close(started)
			<-release
			return transferIntentHTTPResponse(request, http.StatusOK, transferIntentResponse), nil
		}), &bytes.Buffer{})
	gateway.logger = log.New(io.Discard, "", 0)
	ctx, cancel := context.WithCancel(context.Background())
	request := newVideoRequest(http.MethodGet, "/Videos/item-1/stream?MediaSourceId=source-1&Static=true").WithContext(ctx)
	done := make(chan string, 1)
	go func() {
		_, reason := gateway.resolvePlaybackInfoOnDemand(request, fixturePrincipal(), fixtureAccessToken, "item-1", "source-1")
		done <- reason
	}()
	<-started
	gateway.playbackInfoFlights.mu.Lock()
	call := gateway.playbackInfoFlights.calls["mapping-1\x00item-1\x00source-1"]
	gateway.playbackInfoFlights.mu.Unlock()
	cancel()
	if reason := <-done; reason != "request_canceled" {
		t.Fatalf("reason=%s", reason)
	}
	if count := activePlaybackPublications(gateway.proofs); count != 1 {
		t.Fatalf("owner guard released by waiter: %d", count)
	}
	close(release)
	<-call.done
	if count := activePlaybackPublications(gateway.proofs); count != 0 {
		t.Fatalf("owner guard leaked after completion: %d", count)
	}
}

// activePlaybackPublications reads the bounded owner count under its lock.
func activePlaybackPublications(cache *playbackProofCache) int {
	cache.mu.Lock()
	defer cache.mu.Unlock()
	return len(cache.publications)
}

type playbackInfoReadHook struct {
	reader     io.Reader
	beforeRead func()
	once       sync.Once
	readErr    error
}

// Read executes a deterministic response-body interleaving without sleeps.
func (body *playbackInfoReadHook) Read(buffer []byte) (int, error) {
	if body.beforeRead != nil {
		body.once.Do(body.beforeRead)
	}
	if body.readErr != nil {
		return 0, body.readErr
	}
	return body.reader.Read(buffer)
}

// Close satisfies the fake response body contract without external resources.
func (*playbackInfoReadHook) Close() error { return nil }

// TestLateInternalPlaybackInfoCannotRestoreFailedClientSnapshot ensures even
// empty or unusable client responses invalidate outstanding internal results.
func TestLateInternalPlaybackInfoCannotRestoreFailedClientSnapshot(t *testing.T) {
	for _, test := range []struct {
		name   string
		status int
		body   string
	}{
		{"failure", http.StatusBadGateway, ""},
		{"invalid JSON", http.StatusOK, "{"},
		{"empty media", http.StatusOK, `{"MediaSources":[],"PlaySessionId":"session-1"}`},
	} {
		t.Run(test.name, func(t *testing.T) {
			var gateway *Gateway
			gateway = newVideoTestGateway(t, "http://emby.invalid", &fakeTokenService{principal: fixturePrincipal()}, nil,
				roundTripFunc(func(request *http.Request) (*http.Response, error) {
					if request.Method == http.MethodGet {
						sendTransferIntentPlaybackInfo(gateway, `{"IsPlayback":true}`)
						return transferIntentHTTPResponse(request, http.StatusOK, transferIntentResponse), nil
					}
					return transferIntentHTTPResponse(request, test.status, test.body), nil
				}), &bytes.Buffer{})
			request := newVideoRequest(http.MethodGet, "/Videos/item-1/stream?MediaSourceId=source-1&Static=true")
			resolved, reason := gateway.resolvePlaybackInfoOnce(request, fixturePrincipal(), fixtureAccessToken, "item-1", "source-1")
			if gateway.proofs.Len() != 0 || reason != "playback_info_superseded" || resolved.PlaySessionID != "" {
				t.Fatalf("old response restored rejected snapshot: proofs=%d reason=%q session=%q", gateway.proofs.Len(), reason, resolved.PlaySessionID)
			}
		})
	}
}
