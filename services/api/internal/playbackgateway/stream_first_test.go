package playbackgateway

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/konghang/ember/backend/internal/services/directplay"
)

// TestGatewayStreamFirstContractAndAdmission follows the real Gateway entry
// through exact source selection and live transfer admission. Static sources
// with Size=0 remain usable, including MKV; internal GET never grants intent.
func TestGatewayStreamFirstContractAndAdmission(t *testing.T) {
	for _, test := range []struct {
		name, file, method, mode, extraQuery string
		existing, allowed, clientIntent      bool
		unsupportedClient                    bool
		status                               int
	}{
		{name: "stream mkv strict reuse", file: "stream.mkv", existing: true, status: http.StatusFound},
		{name: "stream mp4 compatible creation", file: "StReAm.MP4", mode: "compatible", extraQuery: "&cOnTaInEr=Mp4", allowed: true, status: http.StatusFound},
		{name: "original mkv strict zero size reuse", file: "original.mkv", existing: true, status: http.StatusFound},
		{name: "original mp4 compatible creation", file: "OrIgInAl.MP4", mode: "compatible", allowed: true, status: http.StatusFound},
		{name: "strict missing target", file: "stream.mkv", status: http.StatusPartialContent},
		{name: "original HEAD forbids creation", file: "original.mkv", method: http.MethodHead, mode: "compatible", status: http.StatusPartialContent},
		{name: "stream HEAD existing target", file: "stream.mp4", method: http.MethodHead, mode: "compatible", existing: true, status: http.StatusFound},
		{name: "inherits explicit client intent", file: "original.mp4", clientIntent: true, unsupportedClient: true, allowed: true, status: http.StatusFound},
		{name: "reuses existing explicit proof", file: "stream.mkv", clientIntent: true, allowed: true, status: http.StatusFound},
	} {
		t.Run(test.name, func(t *testing.T) {
			method := test.method
			if method == "" {
				method = http.MethodGet
			}
			container := strings.ToLower(test.file[strings.LastIndex(test.file, ".")+1:])
			target := "/Videos/item-1/" + test.file + "?mEdIaSoUrCeId=source%2D1&Static=TrUe&DeviceId=device-1" + test.extraQuery
			if strings.HasPrefix(strings.ToLower(test.file), "original.") {
				target += "&Container=" + container
			}
			request := newVideoRequest(method, target+"&aPi_KeY="+fixtureAccessToken)
			request.Header.Set("User-Agent", "Stream-First-Media/1")
			request.Header.Set("Range", "bytes=512-")
			request.Header.Set("If-Range", `"stream-first-etag"`)
			request.Header.Set(embyAuthorizationHeader, mediaBrowserAuthorizationWithToken(fixtureAccessToken))
			internalCalls, fallbackCalls, directCalls := 0, 0, 0
			service := transferIntentDirectPlayFunc(func(_ context.Context, resolved directplay.MediaPathResolveRequest) (directplay.RedirectCandidate, error) {
				directCalls++
				wantSession := "internal-session"
				if test.clientIntent {
					wantSession = "client-session"
				}
				if resolved.Path != "/mnt/media/selected."+container || resolved.Method != method || resolved.ClientUserAgent != request.UserAgent() ||
					resolved.UserID != "user-1" || resolved.MappingID != "mapping-1" || resolved.DeviceID != "device-1" || resolved.PlaySessionID != wantSession {
					t.Errorf("wrong source, method or principal: %+v", resolved)
				}
				allowed := resolved.CanCreateTransfer != nil && resolved.CanCreateTransfer()
				if allowed != test.allowed {
					t.Errorf("transfer callback=%t, want %t", allowed, test.allowed)
				}
				if !test.existing && !allowed {
					return directplay.RedirectCandidate{}, directplay.ErrPlaybackIntentRequired
				}
				candidate := validFixtureRedirectCandidate()
				candidate.Preexisting = test.existing
				return candidate, nil
			})
			var logs bytes.Buffer
			gateway := newVideoTestGateway(t, "http://emby.invalid", &fakeTokenService{principal: fixturePrincipal()}, service,
				roundTripFunc(func(upstream *http.Request) (*http.Response, error) {
					if strings.HasSuffix(upstream.URL.Path, "/PlaybackInfo") {
						if upstream.Method == http.MethodPost {
							body := streamFirstPlaybackInfo(container, "client-session")
							if test.unsupportedClient {
								body = strings.ReplaceAll(body, `"SupportsDirectPlay":true`, `"SupportsDirectPlay":false`)
							}
							return transferIntentHTTPResponse(upstream, http.StatusOK, body), nil
						}
						internalCalls++
						if upstream.Method != http.MethodGet || upstream.URL.RequestURI() != "/emby/Items/item-1/PlaybackInfo?UserId=emby-user-1" ||
							upstream.Header.Get(embyAuthorizationHeader) != request.Header.Get(embyAuthorizationHeader) ||
							upstream.Header.Get("Range") != "" || upstream.Header.Get("If-Range") != "" || upstream.Header.Get("Accept-Encoding") != "gzip, deflate" {
							t.Error("supplement changed the user-token PlaybackInfo contract")
						}
						return transferIntentHTTPResponse(upstream, http.StatusOK, streamFirstPlaybackInfo(container, "internal-session")), nil
					}
					fallbackCalls++
					assertStreamFirstFallbackRequest(t, upstream, request)
					return transferIntentHTTPResponse(upstream, http.StatusPartialContent, "fallback"), nil
				}), &logs)
			gateway.transferTriggerPolicy = transferTriggerPolicyFunc(func(context.Context) (string, error) {
				return test.mode, nil
			})
			if test.clientIntent {
				sendTransferIntentPlaybackInfo(gateway, `{"IsPlayback":true,"MediaSourceId":"source-1"}`)
			}
			response := httptest.NewRecorder()
			gateway.ServeHTTP(response, request)
			wantInternal := 1
			if test.clientIntent && !test.unsupportedClient {
				wantInternal = 0
			}
			wantFallback := 0
			if test.status != http.StatusFound {
				wantFallback = 1
			}
			if response.Code != test.status || internalCalls != wantInternal || directCalls != 1 || fallbackCalls != wantFallback {
				t.Fatalf("status=%d internal=%d direct=%d fallback=%d, want %d/%d/1/%d", response.Code, internalCalls, directCalls, fallbackCalls, test.status, wantInternal, wantFallback)
			}
			session := "internal-session"
			if wantInternal == 0 {
				session = "client-session"
			}
			proof, ok := gateway.LookupPlaybackProof(fixturePrincipal(), "item-1", "source-1", session)
			if !ok || proof.Size != 0 || proof.Path != "/mnt/media/selected."+container || gateway.proofs.CanCreateTransfer(proof, fixturePrincipal()) != test.clientIntent {
				t.Fatal("supplement lost exact source proof or manufactured strict intent")
			}
			assertSecretsAbsent(t, logs.String(), fixtureAccessToken, fixtureRedirectURL)
		})
	}
}

// TestGatewayStreamFirstRejectsAmbiguousCandidates keeps unproved options and
// incomplete identities out of supplementation, even with compatible mode.
func TestGatewayStreamFirstRejectsAmbiguousCandidates(t *testing.T) {
	const base = "MediaSourceId=source-1&Static=true"
	for _, test := range []struct{ name, file, query string }{
		{"original requires container", "original.mkv", base},
		{"original missing static", "original.mkv", "MediaSourceId=source-1&Container=mkv"},
		{"unknown filename", "movie.mkv", base + "&Container=mkv"},
		{"missing source", "stream.mkv", "Static=true"},
		{"empty source", "stream.mkv", "MediaSourceId=&Static=true"},
		{"duplicate source", "stream.mkv", base + "&mediaSourceId=source-1"},
		{"empty static", "stream.mkv", "MediaSourceId=source-1&Static="},
		{"false static", "stream.mkv", "MediaSourceId=source-1&Static=false"},
		{"duplicate static", "stream.mkv", base + "&static=true"},
		{"empty session key", "stream.mkv", base + "&playSessionId="},
		{"cold fixed session", "original.mkv", base + "&Container=mkv&PlaySessionId=client-session"},
		{"container mismatch", "stream.mkv", base + "&Container=mp4"},
		{"original container mismatch", "original.mp4", base + "&Container=mkv"},
		{"empty container", "stream.mkv", base + "&Container="},
		{"duplicate container", "original.mkv", base + "&Container=mkv&container=mkv"},
		{"comma container", "stream.mkv,mp4", base},
		{"invalid path container", "stream.mk_v", base},
		{"hls", "stream.m3u8", base},
		{"dash", "stream.mpd", base},
		{"fragment", "stream.m4s", base},
		{"unknown query", "stream.mkv", base + "&FutureOption=true"},
		{"offset query", "original.mkv", base + "&Container=mkv&StartTimeTicks=0"},
		{"audio selection", "stream.mkv", base + "&AudioStreamIndex=1"},
		{"subtitle selection", "stream.mkv", base + "&SubtitleStreamIndex=-1"},
		{"other device", "original.mkv", base + "&Container=mkv&DeviceId=other"},
		{"empty device", "stream.mkv", base + "&DeviceId="},
		{"duplicate device", "stream.mkv", base + "&DeviceId=device-1&deviceid=device-1"},
		{"oversized device", "stream.mkv", base + "&DeviceId=" + strings.Repeat("d", maxProofDeviceIDBytes+1)},
		{"invalid escape", "stream.mkv", base + "&Container=%zz"},
		{"invalid query separator", "stream.mkv", base + ";Container=mkv"},
	} {
		t.Run(test.name, func(t *testing.T) {
			request := newVideoRequest(http.MethodGet, "/emby/Videos/item-1/"+test.file+"?"+test.query)
			request.Header.Set("Range", "bytes=5-")
			request.Header.Set("If-Range", `"unmodified"`)
			calls := 0
			direct := &fakeDirectPlayService{result: validFixtureRedirectCandidate()}
			gateway := newVideoTestGateway(t, "http://emby.invalid", &fakeTokenService{principal: fixturePrincipal()}, direct,
				roundTripFunc(func(upstream *http.Request) (*http.Response, error) {
					calls++
					assertStreamFirstFallbackRequest(t, upstream, request)
					return transferIntentHTTPResponse(upstream, http.StatusPartialContent, "fallback"), nil
				}), &bytes.Buffer{})
			gateway.transferTriggerPolicy = transferTriggerPolicyFunc(func(context.Context) (string, error) { return "compatible", nil })
			response := httptest.NewRecorder()
			gateway.ServeHTTP(response, request)
			if response.Code != http.StatusPartialContent || calls != 1 || len(direct.snapshot()) != 0 || gateway.proofs.Len() != 0 {
				t.Fatalf("ineligible request acquired playback context: status=%d upstream=%d direct=%d proofs=%d", response.Code, calls, len(direct.snapshot()), gateway.proofs.Len())
			}
		})
	}
}

// TestGatewayStreamFirstFailuresPreserveOriginalRequest protects all new path
// shapes from the plain-stream DirectStreamUrl/session fallback rewrite.
func TestGatewayStreamFirstFailuresPreserveOriginalRequest(t *testing.T) {
	for _, file := range []string{"stream.mkv", "stream.mp4", "original.mkv", "original.mp4"} {
		for _, outcome := range []string{"lookup failure", "proof rejected", "container mismatch", "direct play failure"} {
			t.Run(file+"/"+outcome, func(t *testing.T) {
				container := file[strings.LastIndex(file, ".")+1:]
				target := "/emby/Videos/item-1/" + file + "?Static=true&MediaSourceId=source%2d1&DeviceId=device-1"
				if strings.HasPrefix(file, "original.") {
					target += "&Container=" + container
				}
				method := http.MethodGet
				if outcome == "lookup failure" {
					method = http.MethodHead
				}
				request := newVideoRequest(method, target)
				request.Header.Set("Range", "bytes=1024-2047")
				request.Header.Set("If-Range", `"original-validator"`)
				request.Header.Set("User-Agent", "Stream-First-Fallback/1")
				request.Header.Set(embyAuthorizationHeader, mediaBrowserAuthorizationWithToken(fixtureAccessToken))
				internalCalls, fallbackCalls := 0, 0
				direct := &fakeDirectPlayService{err: directplay.ErrPathNotMapped}
				gateway := newVideoTestGateway(t, "http://emby.invalid", &fakeTokenService{principal: fixturePrincipal()}, direct,
					roundTripFunc(func(upstream *http.Request) (*http.Response, error) {
						if strings.HasSuffix(upstream.URL.Path, "/PlaybackInfo") {
							internalCalls++
							if outcome == "lookup failure" {
								return nil, errors.New("fixture upstream failure")
							}
							body := streamFirstPlaybackInfo(container, "internal-session")
							if outcome == "proof rejected" {
								body = strings.ReplaceAll(body, `"SupportsDirectPlay":true`, `"SupportsDirectPlay":false`)
							}
							if outcome == "container mismatch" {
								body = streamFirstPlaybackInfo("avi", "internal-session")
							}
							return transferIntentHTTPResponse(upstream, http.StatusOK, body), nil
						}
						fallbackCalls++
						assertStreamFirstFallbackRequest(t, upstream, request)
						return transferIntentHTTPResponse(upstream, http.StatusPartialContent, "fallback"), nil
					}), &bytes.Buffer{})
				response := httptest.NewRecorder()
				gateway.ServeHTTP(response, request)
				wantDirect := 0
				if outcome == "direct play failure" {
					wantDirect = 1
				}
				if response.Code != http.StatusPartialContent || internalCalls != 1 || fallbackCalls != 1 || len(direct.snapshot()) != wantDirect {
					t.Fatalf("status=%d internal=%d fallback=%d direct=%d, want 206/1/1/%d", response.Code, internalCalls, fallbackCalls, len(direct.snapshot()), wantDirect)
				}
			})
		}
	}
}

// TestGatewayStreamFirstCanceledWaiterSharesExistingResolver exercises the
// new stream/original entrances sharing one owner while cancellation remains
// request-local. Existing resolver tests cover device/token key separation.
func TestGatewayStreamFirstCanceledWaiterSharesExistingResolver(t *testing.T) {
	started, release := make(chan struct{}), make(chan struct{})
	defer func() {
		select {
		case <-release:
		default:
			close(release)
		}
	}()
	var internalCalls atomic.Int32
	direct := &fakeDirectPlayService{result: validFixtureRedirectCandidate()}
	gateway := newVideoTestGateway(t, "http://emby.invalid", &fakeTokenService{principal: fixturePrincipal()}, direct,
		roundTripFunc(func(request *http.Request) (*http.Response, error) {
			if strings.HasSuffix(request.URL.Path, "/PlaybackInfo") {
				if internalCalls.Add(1) == 1 {
					close(started)
				}
				<-release
				return transferIntentHTTPResponse(request, http.StatusOK, streamFirstPlaybackInfo("mkv", "internal-session")), nil
			}
			return nil, request.Context().Err()
		}), &bytes.Buffer{})
	gateway.logger = log.New(io.Discard, "", 0)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	firstDone := make(chan struct{})
	firstResponse := httptest.NewRecorder()
	go func() {
		defer close(firstDone)
		gateway.ServeHTTP(firstResponse, newVideoRequest(http.MethodGet, "/Videos/item-1/stream.mkv?MediaSourceId=source-1&Static=true").WithContext(ctx))
	}()
	awaitStreamFirstSignal(t, started)
	waiter := &playbackFlightWaitContext{Context: context.Background(), joined: make(chan struct{})}
	secondDone := make(chan struct{})
	secondResponse := httptest.NewRecorder()
	go func() {
		defer close(secondDone)
		gateway.ServeHTTP(secondResponse, newVideoRequest(http.MethodGet, "/Videos/item-1/original.mkv?MediaSourceId=source-1&Static=true&Container=mkv").WithContext(waiter))
	}()
	awaitStreamFirstSignal(t, waiter.joined)
	cancel()
	awaitStreamFirstSignal(t, firstDone)
	if firstResponse.Code != statusClientClosedRequest || activePlaybackPublications(gateway.proofs) != 1 {
		t.Fatal("canceled waiter released the shared publication or lost cancellation status")
	}
	close(release)
	awaitStreamFirstSignal(t, secondDone)
	if secondResponse.Code != http.StatusFound || internalCalls.Load() != 1 || len(direct.snapshot()) != 1 || activePlaybackPublications(gateway.proofs) != 0 {
		t.Fatalf("shared resolver status=%d internal=%d direct=%d", secondResponse.Code, internalCalls.Load(), len(direct.snapshot()))
	}
}

// TestGatewayStreamFirstCannotPublishOverNewClientProof verifies publication
// ordering at the new video entry rather than only calling the resolver.
func TestGatewayStreamFirstCannotPublishOverNewClientProof(t *testing.T) {
	started, release := make(chan struct{}), make(chan struct{})
	defer func() {
		select {
		case <-release:
		default:
			close(release)
		}
	}()
	direct := &fakeDirectPlayService{result: validFixtureRedirectCandidate()}
	request := newVideoRequest(http.MethodGet, "/emby/Videos/item-1/original.mp4?Static=true&Container=mp4&MediaSourceId=source%2d1")
	request.Header.Set("Range", "bytes=0-")
	var fallbackCalls atomic.Int32
	gateway := newVideoTestGateway(t, "http://emby.invalid", &fakeTokenService{principal: fixturePrincipal()}, direct,
		roundTripFunc(func(upstream *http.Request) (*http.Response, error) {
			if strings.HasSuffix(upstream.URL.Path, "/PlaybackInfo") {
				if upstream.Method == http.MethodGet {
					close(started)
					<-release
					return transferIntentHTTPResponse(upstream, http.StatusOK, streamFirstPlaybackInfo("mp4", "internal-session")), nil
				}
				return transferIntentHTTPResponse(upstream, http.StatusOK, streamFirstPlaybackInfo("mp4", "client-session")), nil
			}
			fallbackCalls.Add(1)
			assertStreamFirstFallbackRequest(t, upstream, request)
			return transferIntentHTTPResponse(upstream, http.StatusPartialContent, "fallback"), nil
		}), &bytes.Buffer{})
	gateway.logger = log.New(io.Discard, "", 0)
	done := make(chan struct{})
	response := httptest.NewRecorder()
	go func() {
		defer close(done)
		gateway.ServeHTTP(response, request)
	}()
	awaitStreamFirstSignal(t, started)
	sendTransferIntentPlaybackInfo(gateway, `{"IsPlayback":true,"MediaSourceId":"source-1"}`)
	clientProof, ok := gateway.LookupPlaybackProof(fixturePrincipal(), "item-1", "source-1", "client-session")
	if !ok || !gateway.proofs.CanCreateTransfer(clientProof, fixturePrincipal()) {
		t.Fatal("client fixture failed to establish explicit proof")
	}
	close(release)
	awaitStreamFirstSignal(t, done)
	current, ok := gateway.LookupPlaybackProof(fixturePrincipal(), "item-1", "source-1", "client-session")
	if response.Code != http.StatusPartialContent || fallbackCalls.Load() != 1 || len(direct.snapshot()) != 0 || !ok || current != clientProof ||
		!gateway.proofs.CanCreateTransfer(current, fixturePrincipal()) || activePlaybackPublications(gateway.proofs) != 0 {
		t.Fatal("late stream-first result replaced client proof/intent or bypassed fallback")
	}
	if _, ok := gateway.LookupPlaybackProof(fixturePrincipal(), "item-1", "source-1", "internal-session"); ok {
		t.Fatal("superseded stream-first result retained an internal proof")
	}
}

// streamFirstPlaybackInfo puts the selected source second and provides an
// authoritative URL that new stream-first failures must never adopt.
func streamFirstPlaybackInfo(container, session string) string {
	return fmt.Sprintf(`{"PlaySessionId":%q,"MediaSources":[
		{"Id":"source-other","ItemId":"item-1","Path":"/mnt/media/other.mkv","Container":"mkv","SupportsDirectPlay":true},
		{"Id":"source-1","ItemId":"item-1","Path":%q,"Container":%q,"Size":0,"SupportsDirectPlay":true,"SupportsDirectStream":true,"DirectStreamUrl":%q}
	]}`, session, "/mnt/media/selected."+container, container,
		"/Videos/item-1/stream."+container+"?MediaSourceId=source-1&PlaySessionId="+session+"&Static=true&Container="+container)
}

// assertStreamFirstFallbackRequest compares exact client query bytes and
// end-to-end headers after the existing root-to-/emby path normalization.
func assertStreamFirstFallbackRequest(t *testing.T, upstream, client *http.Request) {
	t.Helper()
	if upstream.Method != client.Method || upstream.URL.Path != client.URL.Path || upstream.URL.RawQuery != client.URL.RawQuery {
		t.Errorf("fallback changed method/path/query: got %s %s, want %s %s", upstream.Method, upstream.URL.RequestURI(), client.Method, client.URL.RequestURI())
	}
	for _, header := range []string{"Range", "If-Range", "User-Agent", accessTokenHeader, embyAuthorizationHeader} {
		if upstream.Header.Get(header) != client.Header.Get(header) {
			t.Errorf("fallback changed header %s", header)
		}
	}
}

// awaitStreamFirstSignal bounds deterministic concurrency checkpoints so a
// routing regression fails locally rather than hanging the Gateway suite.
func awaitStreamFirstSignal(t *testing.T, signal <-chan struct{}) {
	t.Helper()
	select {
	case <-signal:
	case <-time.After(3 * time.Second):
		t.Fatal("stream-first concurrency checkpoint timed out")
	}
}
