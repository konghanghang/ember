package playbackgateway

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/konghang/ember/backend/internal/services/directplay"
	"github.com/konghang/ember/backend/internal/services/p115quota"
)

// leaseIdentityHarness uses the production key derivation and memory lease
// contract while replacing provider and Redis I/O at the Gateway boundary.
type leaseIdentityHarness struct {
	keys     *p115quota.KeyDeriver
	store    *p115quota.MemoryLeaseStore
	now      time.Time
	account  string
	result   p115quota.TransitionResult
	sessions []string
}

// ResolveMediaPath reserves the exact identity received from the real video route.
func (h *leaseIdentityHarness) ResolveMediaPath(ctx context.Context, r directplay.MediaPathResolveRequest) (directplay.RedirectCandidate, error) {
	h.sessions = append(h.sessions, r.PlaySessionID)
	key, err := h.keys.SessionFingerprint(p115quota.SessionIdentity{ServerID: "server-1", UserID: r.UserID, MappingID: r.MappingID, DeviceID: r.DeviceID, PlaySessionID: r.PlaySessionID})
	if err != nil {
		return directplay.RedirectCandidate{}, err
	}
	_, err = h.store.Reserve(ctx, p115quota.ReserveRequest{PlaybackAccountKey: h.account, UserID: r.UserID, SessionFingerprint: key, MaxConcurrentStreams: 2}, h.now)
	return validFixtureRedirectCandidate(), err
}

// HandlePlaybackSessionEvent mirrors the service's existing exact-key event
// behavior, deliberately without any alias lookup or missing-lease creation.
func (h *leaseIdentityHarness) HandlePlaybackSessionEvent(ctx context.Context, e directplay.PlaybackSessionEvent) (directplay.PlaybackSessionEventResult, error) {
	key, err := h.keys.SessionFingerprint(p115quota.SessionIdentity{ServerID: "server-1", UserID: e.UserID, MappingID: e.MappingID, DeviceID: e.DeviceID, PlaySessionID: e.PlaySessionID})
	if err != nil {
		return directplay.PlaybackSessionEventResult{}, err
	}
	state := p115quota.LeaseStateActive
	if e.IsPaused {
		state = p115quota.LeaseStatePaused
	}
	if e.Stopped {
		h.result, err = h.store.Stop(ctx, key, h.now)
	} else {
		h.result, err = h.store.Advance(ctx, key, state, h.now)
	}
	return directplay.PlaybackSessionEventResult{Found: h.result.Found, State: h.result.State, Account: h.result.Account, User: h.result.User}, err
}

// TestSupplementedVideoLeaseUsesClientLifecycle reproduces the two-session
// Infuse flow, verifies transparent forwarding, and outlives both proof TTLs.
func TestSupplementedVideoLeaseUsesClientLifecycle(t *testing.T) {
	keys, err := p115quota.NewKeyDeriver("fixture-root-key")
	if err != nil {
		t.Fatal(err)
	}
	account, err := keys.PlaybackAccountKey("100")
	if err != nil {
		t.Fatal(err)
	}
	h := &leaseIdentityHarness{keys: keys, account: account, store: p115quota.NewMemoryLeaseStore(), now: time.Now()}
	var forwarded string
	internalCalls := 0
	status := http.StatusNoContent
	g := newVideoTestGateway(t, "http://emby.invalid", &fakeTokenService{principal: fixturePrincipal()}, h, roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if strings.Contains(r.URL.Path, "/Sessions/Playing") {
			body, _ := io.ReadAll(r.Body)
			forwarded = string(body)
			return transferIntentHTTPResponse(r, status, ""), nil
		}
		body := transferIntentResponse
		if r.Method == http.MethodPost {
			body = strings.Replace(body, `"SupportsDirectPlay":true`, `"SupportsDirectPlay":false`, 1)
		} else {
			internalCalls++
			body = strings.Replace(body, `"session-1"`, fmt.Sprintf("\"internal-session-%d\"", internalCalls), 1)
		}
		return transferIntentHTTPResponse(r, http.StatusOK, body), nil
	}), &bytes.Buffer{})
	g.playbackSessionService = h
	g.proofs.now = func() time.Time { return h.now }
	sendTransferIntentPlaybackInfo(g, `{"IsPlayback":true,"MediaSourceId":"source-1"}`)
	recorder := httptest.NewRecorder()
	g.ServeHTTP(recorder, newVideoRequest(http.MethodGet, "/Videos/item-1/stream?MediaSourceId=source-1&Static=true"))
	if recorder.Code != http.StatusFound {
		t.Fatalf("video status=%d", recorder.Code)
	}
	event := func(path, session string, paused bool) {
		t.Helper()
		body := fmt.Sprintf(`{"ItemId":"item-1","MediaSourceId":"source-1","PlaySessionId":%q,"PositionTicks":42,"IsPaused":%t}`, session, paused)
		request := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set(accessTokenHeader, fixtureAccessToken)
		response := httptest.NewRecorder()
		g.ServeHTTP(response, request)
		if response.Code != status || forwarded != body {
			t.Fatal("event body or upstream status changed")
		}
	}
	h.now = h.now.Add(5 * time.Second)
	event("/Sessions/Playing", "session-1", false)
	if !h.result.Found || h.result.State != p115quota.LeaseStateActive {
		t.Fatal("original client start missed video reservation")
	}
	event("/Sessions/Playing/Progress", "other-session", false)
	if h.result.Found {
		t.Fatal("unrelated session borrowed lease")
	}
	for i := 0; i < 7; i++ {
		h.now = h.now.Add(time.Minute)
		g.proofs.Len() // Exercise lazy eviction, including the expired origin grant.
		event("/Sessions/Playing/Progress", "session-1", false)
		if !h.result.Found {
			t.Fatal("progress depended on expired proof or transfer intent")
		}
	}
	h.now = h.now.Add(107 * time.Second)
	event("/Sessions/Playing/Progress", "session-1", false)
	if !h.result.Found {
		t.Fatal("a reporting gap shorter than active TTL lost the lease")
	}
	// A new Range request after proof expiry must keep the active client session.
	g.ServeHTTP(httptest.NewRecorder(), newVideoRequest(http.MethodGet, "/Videos/item-1/stream?MediaSourceId=source-1&Static=true"))
	if h.sessions[len(h.sessions)-1] != "session-1" {
		t.Fatal("proof refresh replaced active playback identity")
	}
	usageAfterRefresh, _ := h.store.AccountUsage(context.Background(), account, h.now)
	if usageAfterRefresh.OccupiedStreams != 1 {
		t.Fatal("proof refresh reserved a second playback slot")
	}
	event("/Sessions/Playing/Progress", "session-1", true)
	if h.result.State != p115quota.LeaseStatePaused {
		t.Fatal("pause lost")
	}
	h.now = h.now.Add(7 * time.Minute)
	g.ServeHTTP(httptest.NewRecorder(), newVideoRequest(http.MethodGet, "/Videos/item-1/stream?MediaSourceId=source-1&Static=true"))
	if h.sessions[len(h.sessions)-1] != "session-1" {
		t.Fatal("paused playback lost its original session after proof expiry")
	}
	event("/Sessions/Playing", "session-1", false)
	if !h.result.Found || h.result.State != p115quota.LeaseStateActive {
		t.Fatal("resume lost")
	}
	status = http.StatusBadGateway
	event("/Sessions/Playing/Stopped", "session-1", false)
	usage, _ := h.store.AccountUsage(context.Background(), account, h.now)
	if usage.OccupiedStreams != 1 {
		t.Fatal("failed Emby stop released lease")
	}
	status = http.StatusNoContent
	event("/Sessions/Playing/Stopped", "session-1", false)
	usage, _ = h.store.AccountUsage(context.Background(), account, h.now)
	if usage.OccupiedStreams != 0 {
		t.Fatal("successful stop retained lease")
	}
	event("/Sessions/Playing/Progress", "session-1", false)
	if h.result.Found {
		t.Fatal("late progress resurrected stopped lease")
	}
	sendTransferIntentPlaybackInfo(g, `{"IsPlayback":true,"MediaSourceId":"source-1"}`)
	g.ServeHTTP(httptest.NewRecorder(), newVideoRequest(http.MethodGet, "/Videos/item-1/stream?MediaSourceId=source-1&Static=true"))
	h.now = h.now.Add(31 * time.Second)
	event("/Sessions/Playing", "session-1", false)
	if h.result.Found {
		t.Fatal("late start resurrected expired reservation")
	}
}
