package playbackgateway

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestOnDemandPlaybackInfoFlightGroupCoalescesConcurrentRequests(t *testing.T) {
	group := &onDemandPlaybackInfoFlightGroup{}
	started := make(chan struct{})
	release := make(chan struct{})
	var calls atomic.Int32
	resolve := func() (onDemandPlaybackInfo, string) {
		if calls.Add(1) == 1 {
			close(started)
		}
		<-release
		return onDemandPlaybackInfo{PlaySessionID: "session-1", Container: "mkv"}, ""
	}

	var workers sync.WaitGroup
	results := make(chan onDemandPlaybackInfo, 2)
	for index := 0; index < 2; index++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			result, reasonCode := group.Do(context.Background(), "mapping\x00item\x00source", resolve)
			if reasonCode != "" {
				t.Errorf("reasonCode=%q", reasonCode)
			}
			results <- result
		}()
	}
	<-started
	time.Sleep(10 * time.Millisecond)
	close(release)
	workers.Wait()
	close(results)
	if calls.Load() != 1 {
		t.Fatalf("resolver calls=%d, want 1", calls.Load())
	}
	for result := range results {
		if result.PlaySessionID != "session-1" || result.Container != "mkv" {
			t.Fatalf("result=%+v", result)
		}
	}
}

func TestOnDemandPlaybackInfoFlightGroupLetsWaiterCancel(t *testing.T) {
	group := &onDemandPlaybackInfoFlightGroup{}
	release := make(chan struct{})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, reasonCode := group.Do(ctx, "mapping\x00item\x00source", func() (onDemandPlaybackInfo, string) {
		<-release
		return onDemandPlaybackInfo{}, ""
	})
	group.mu.Lock()
	inflight := len(group.calls)
	group.mu.Unlock()
	close(release)
	if reasonCode != "request_canceled" {
		t.Fatalf("reasonCode=%q", reasonCode)
	}
	if inflight != 0 {
		t.Fatalf("inflight calls=%d, want no resolver for pre-canceled waiter", inflight)
	}
}

func TestOnDemandPlaybackInfoFlightGroupRejectsExpiredDeadlineBeforeResolve(t *testing.T) {
	group := &onDemandPlaybackInfoFlightGroup{}
	ctx, cancel := context.WithDeadline(context.Background(), time.Unix(0, 0))
	defer cancel()
	var calls atomic.Int32

	_, reasonCode := group.Do(ctx, "mapping\x00item\x00source", func() (onDemandPlaybackInfo, string) {
		calls.Add(1)
		return onDemandPlaybackInfo{}, ""
	})

	if reasonCode != "deadline_exceeded" || calls.Load() != 0 {
		t.Fatalf("reasonCode=%q resolverCalls=%d", reasonCode, calls.Load())
	}
}

func TestOnDemandPlaybackInfoFlightGroupConvertsResolverPanic(t *testing.T) {
	group := &onDemandPlaybackInfoFlightGroup{}
	_, reasonCode := group.Do(context.Background(), "mapping\x00item\x00panic", func() (onDemandPlaybackInfo, string) {
		panic("secret resolver detail")
	})
	if reasonCode != "internal_failure" {
		t.Fatalf("reasonCode=%q", reasonCode)
	}
}

func TestResolvePlaybackInfoOnDemandReusesLatestProof(t *testing.T) {
	var transportCalls atomic.Int32
	var logs bytes.Buffer
	gateway := newVideoTestGateway(
		t,
		"http://emby.invalid",
		&fakeTokenService{principal: fixturePrincipal()},
		nil,
		roundTripFunc(func(*http.Request) (*http.Response, error) {
			transportCalls.Add(1)
			return nil, context.Canceled
		}),
		&logs,
	)
	gateway.proofs.Record([]PlaybackProof{fixturePlaybackProof("mapping-1", "item-1", "source-1", "session-1")})
	request := httptest.NewRequest(http.MethodGet, "/Videos/item-1/stream?MediaSourceId=source-1&Static=true", nil)
	resolved, reasonCode := gateway.resolvePlaybackInfoOnDemand(request, fixturePrincipal(), fixtureAccessToken, "item-1", "source-1")
	if reasonCode != "" || resolved.PlaySessionID != "session-1" || resolved.Container != "mkv" || transportCalls.Load() != 0 {
		t.Fatalf("resolved=%+v reasonCode=%q transportCalls=%d", resolved, reasonCode, transportCalls.Load())
	}
}

func TestResolvePlaybackInfoOnDemandDoesNotReuseProofFromChangedPrincipal(t *testing.T) {
	var transportCalls atomic.Int32
	var logs bytes.Buffer
	gateway := newVideoTestGateway(
		t,
		"http://emby.invalid",
		&fakeTokenService{principal: fixturePrincipal()},
		nil,
		roundTripFunc(func(*http.Request) (*http.Response, error) {
			transportCalls.Add(1)
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": {"application/json"}},
				Body: io.NopCloser(strings.NewReader(
					`{"MediaSources":[{"Id":"source-1","ItemId":"item-1","Container":"mkv"}],"PlaySessionId":"session-current"}`,
				)),
			}, nil
		}),
		&logs,
	)
	gateway.proofs.Record([]PlaybackProof{fixturePlaybackProof("mapping-1", "item-1", "source-1", "session-stale")})
	principal := fixturePrincipal()
	principal.DeviceID = "device-current"
	request := httptest.NewRequest(http.MethodGet, "/Videos/item-1/stream?MediaSourceId=source-1&Static=true", nil)

	resolved, reasonCode := gateway.resolvePlaybackInfoOnDemand(request, principal, fixtureAccessToken, "item-1", "source-1")

	if reasonCode != "" || resolved.PlaySessionID != "session-current" || transportCalls.Load() != 1 {
		t.Fatalf("resolved=%+v reasonCode=%q transportCalls=%d", resolved, reasonCode, transportCalls.Load())
	}
}

func TestResolvePlaybackInfoOnDemandInvalidatesStaleItemProofsWithoutNewDirectPlayProof(t *testing.T) {
	var logs bytes.Buffer
	gateway := newVideoTestGateway(
		t,
		"http://emby.invalid",
		&fakeTokenService{principal: fixturePrincipal()},
		nil,
		roundTripFunc(func(*http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": {"application/json"}},
				Body: io.NopCloser(strings.NewReader(
					`{"MediaSources":[{"Id":"source-current","ItemId":"item-1","Container":"mkv","SupportsDirectPlay":false}],"PlaySessionId":"session-current"}`,
				)),
			}, nil
		}),
		&logs,
	)
	gateway.proofs.Record([]PlaybackProof{fixturePlaybackProof("mapping-1", "item-1", "source-stale", "session-stale")})
	request := httptest.NewRequest(http.MethodGet, "/Videos/item-1/stream?MediaSourceId=source-current&Static=true", nil)

	resolved, reasonCode := gateway.resolvePlaybackInfoOnDemand(request, fixturePrincipal(), fixtureAccessToken, "item-1", "source-current")

	if reasonCode != "" || resolved.PlaySessionID != "session-current" || resolved.ProofCount != 0 {
		t.Fatalf("resolved=%+v reasonCode=%q", resolved, reasonCode)
	}
	for _, expected := range []string{
		`code=playback_info_media_source_observed mappingId="mapping-1" itemId="item-1" mediaSourceId="source-current" mediaPath=""`,
		"pathPresent=false", "sizePresent=false", "supportsDirectPlay=false",
		"proofAccepted=false", "proofRejectReason=path_missing",
	} {
		if !strings.Contains(logs.String(), expected) {
			t.Fatalf("logs=%q, want %s", logs.String(), expected)
		}
	}
	if _, ok := gateway.proofs.LookupLatestMediaSource("mapping-1", "item-1", "source-stale"); ok {
		t.Fatal("stale proof survived a newer authoritative PlaybackInfo response")
	}
}

func TestResolvePlaybackInfoOnDemandConvertsQueryTokenToInternalHeader(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/emby/Items/item-1/PlaybackInfo" || request.URL.Query().Get("UserId") != "emby-user-1" ||
			request.URL.Query().Has("api_key") || request.Header.Get(accessTokenHeader) != fixtureAccessToken ||
			request.Header.Get("Accept-Encoding") != "gzip, deflate" {
			t.Fatalf("request=%s headers=%v", request.URL.RequestURI(), request.Header)
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(writer, `{"MediaSources":[{"Id":"source-1","ItemId":"item-1","Path":"/private/media/one.mkv","Size":1024,"Container":"mkv","SupportsDirectPlay":true}],"PlaySessionId":"session-1"}`)
	}))
	defer upstream.Close()

	var logs bytes.Buffer
	gateway := newVideoTestGateway(t, upstream.URL, &fakeTokenService{principal: fixturePrincipal()}, nil, nil, &logs)
	request := httptest.NewRequest(http.MethodGet, "/Videos/item-1/stream?MediaSourceId=source-1&Static=true&api_key="+fixtureAccessToken, nil)
	resolved, reasonCode := gateway.resolvePlaybackInfoOnDemand(request, fixturePrincipal(), fixtureAccessToken, "item-1", "source-1")
	if reasonCode != "" || resolved.PlaySessionID != "session-1" || resolved.Container != "mkv" || resolved.ProofCount != 1 {
		t.Fatalf("resolved=%+v reasonCode=%q", resolved, reasonCode)
	}
	if !strings.Contains(logs.String(), `code=playback_info_media_source_observed mappingId="mapping-1" itemId="item-1" mediaSourceId="source-1" mediaPath="/private/media/one.mkv"`) {
		t.Fatalf("logs=%q, want complete media path", logs.String())
	}
	assertSecretsAbsent(t, logs.String(), fixtureAccessToken)
}

func TestResolvePlaybackInfoOnceClassifiesTransportTermination(t *testing.T) {
	tests := []struct {
		name       string
		err        error
		wantReason string
	}{
		{name: "canceled", err: context.Canceled, wantReason: "request_canceled"},
		{name: "deadline", err: context.DeadlineExceeded, wantReason: "deadline_exceeded"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var logs bytes.Buffer
			gateway := newVideoTestGateway(
				t,
				"http://emby.invalid",
				&fakeTokenService{principal: fixturePrincipal()},
				nil,
				roundTripFunc(func(*http.Request) (*http.Response, error) { return nil, test.err }),
				&logs,
			)
			request := httptest.NewRequest(http.MethodGet, "/Videos/item-1/stream?MediaSourceId=source-1&Static=true", nil)

			_, reasonCode := gateway.resolvePlaybackInfoOnce(request, fixturePrincipal(), fixtureAccessToken, "item-1", "source-1")

			if reasonCode != test.wantReason {
				t.Fatalf("reasonCode=%q, want %q", reasonCode, test.wantReason)
			}
		})
	}
}

func TestParseOnDemandPlaybackInfoRejectsIdentityAmbiguity(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{name: "media source mismatch", body: `{"MediaSources":[{"Id":"other-source","ItemId":"item-1","Container":"mkv"}],"PlaySessionId":"session-1"}`},
		{name: "item mismatch", body: `{"MediaSources":[{"Id":"source-1","ItemId":"other-item","Container":"mkv"}],"PlaySessionId":"session-1"}`},
		{name: "duplicate source", body: `{"MediaSources":[{"Id":"source-1","Container":"mkv"},{"Id":"source-1","Container":"mkv"}],"PlaySessionId":"session-1"}`},
		{name: "missing play session", body: `{"MediaSources":[{"Id":"source-1","Container":"mkv"}]}`},
		{name: "unsafe container", body: `{"MediaSources":[{"Id":"source-1","Container":"mkv&api_key=value"}],"PlaySessionId":"session-1"}`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, _, err := parseOnDemandPlaybackInfo([]byte(test.body), fixturePrincipal(), "item-1", "source-1"); err == nil {
				t.Fatal("parseOnDemandPlaybackInfo error=nil")
			}
		})
	}
}

func TestParseEmbyDirectStreamURLNormalizesAndRemovesURLToken(t *testing.T) {
	target, ok := parseEmbyDirectStreamURL(
		"/Videos/item-1/stream.mkv?Static=true&MediaSourceId=source-1&api_key=fixture-url-token",
		"item-1",
		"source-1",
		"session-1",
		"mkv",
	)
	if !ok || target.Path != "/emby/Videos/item-1/stream.mkv" || target.Query().Get("MediaSourceId") != "source-1" ||
		target.Query().Get("Static") != "true" || target.Query().Has("api_key") {
		t.Fatalf("target=%v ok=%t", target, ok)
	}
}

func TestParseEmbyDirectStreamURLRejectsUnsafeOrMismatchedTargets(t *testing.T) {
	tests := []struct {
		name   string
		target string
	}{
		{name: "absolute URL", target: "https://media.example.invalid/Videos/item-1/stream.mkv"},
		{name: "wrong item", target: "/Videos/item-2/stream.mkv"},
		{name: "wrong source", target: "/Videos/item-1/stream.mkv?MediaSourceId=source-2"},
		{name: "wrong session", target: "/Videos/item-1/stream.mkv?PlaySessionId=session-2"},
		{name: "non static", target: "/Videos/item-1/stream.mkv?Static=false"},
		{name: "wrong container", target: "/Videos/item-1/stream.mp4"},
		{name: "conflicting container query", target: "/Videos/item-1/stream.mkv?Container=mp4"},
		{name: "encoded path", target: "/Videos/item-1/%73tream.mkv"},
		{name: "duplicate source", target: "/Videos/item-1/stream.mkv?MediaSourceId=source-1&mediasourceid=source-1"},
		{name: "manifest", target: "/Videos/item-1/stream.m3u8"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if target, ok := parseEmbyDirectStreamURL(test.target, "item-1", "source-1", "session-1", "mkv"); ok || target != nil {
				t.Fatalf("target=%v ok=%t", target, ok)
			}
		})
	}
}

func TestBuildExtensionStreamFallbackRequestPreservesClientQuery(t *testing.T) {
	request := httptest.NewRequest(
		http.MethodGet,
		"/emby/Videos/item-1/stream?MediaSourceId=source-1&Static=true&opaque=%2f%2F",
		nil,
	)

	fallback, ok := buildExtensionStreamFallbackRequest(request, "m4v")

	if !ok || fallback == request || fallback.URL.Path != "/emby/Videos/item-1/stream.mp4" ||
		fallback.URL.RawQuery != request.URL.RawQuery {
		t.Fatalf("fallback=%v original=%v ok=%t", fallback.URL, request.URL, ok)
	}
}

func TestAugmentVideoRequestWithPlaybackInfoPreservesRawQuery(t *testing.T) {
	const originalQuery = "MediaSourceId=source-1&Static=true&opaque=%2f%2F&&container=mkv"
	request := httptest.NewRequest(http.MethodGet, "/Videos/item-1/stream?"+originalQuery, nil)

	augmented, ok := augmentVideoRequestWithPlaybackInfo(request, onDemandPlaybackInfo{
		PlaySessionID: "session+/1",
		Container:     "mp4",
	})

	if !ok || augmented == request {
		t.Fatalf("augmented=%p original=%p ok=%t", augmented, request, ok)
	}
	if request.URL.RawQuery != originalQuery {
		t.Fatalf("original query changed to %q", request.URL.RawQuery)
	}
	want := originalQuery + "&PlaySessionId=session%2B%2F1"
	if augmented.URL.RawQuery != want {
		t.Fatalf("augmented query=%q, want %q", augmented.URL.RawQuery, want)
	}
	if len(augmented.URL.Query()["container"]) != 1 || augmented.URL.Query().Get("PlaySessionId") != "session+/1" {
		t.Fatalf("augmented values=%v", augmented.URL.Query())
	}
}
