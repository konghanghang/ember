package playbackgateway

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/konghang/ember/backend/internal/services/directplay"
	"github.com/konghang/ember/backend/internal/services/embytoken"
	"github.com/konghang/ember/backend/internal/services/p115quota"
)

// TestGatewayAnonymousItemImagesIgnoreWebPolicy keeps native client images
// independent from both a disabled Web UI and unavailable Web configuration.
func TestGatewayAnonymousItemImagesIgnoreWebPolicy(t *testing.T) {
	for _, test := range []struct {
		name, method, target string
		policy               *fakeWebSurfacePolicy
		status               int
	}{
		{name: "enabled primary", method: http.MethodGet, target: "/emby/Items/fixture/Images/Primary?tag=private-tag", policy: &fakeWebSurfacePolicy{enabled: true}, status: http.StatusOK},
		{name: "disabled indexed backdrop", method: http.MethodGet, target: "/emby/Items/fixture/Images/Backdrop/1?tag=private-tag", policy: &fakeWebSurfacePolicy{}, status: http.StatusOK},
		{name: "failed policy HEAD", method: http.MethodHead, target: "/emby/Items/fixture/Images/Backdrop/0", policy: &fakeWebSurfacePolicy{err: errors.New("private-config-error")}, status: http.StatusOK},
		{name: "missing policy upstream denial", method: http.MethodGet, target: "/eMbY/iTeMs/fixture/iMaGeS/Primary", status: http.StatusForbidden},
	} {
		t.Run(test.name, func(t *testing.T) {
			var logs bytes.Buffer
			calls := 0
			gateway := newVideoTestGateway(t, "http://emby.invalid", &fakeTokenService{principal: fixturePrincipal()}, nil,
				roundTripFunc(func(request *http.Request) (*http.Response, error) {
					calls++
					if request.Method != test.method || request.URL.RequestURI() != test.target || request.Header.Get("If-None-Match") != "fixture-etag" {
						t.Fatal("image proxy changed the original request")
					}
					if request.Header.Get(accessTokenHeader) != "" || request.Header.Get("Authorization") != "" {
						t.Fatal("image proxy injected credentials")
					}
					body := "fixture-image"
					if test.method == http.MethodHead {
						body = ""
					}
					return &http.Response{StatusCode: test.status, Header: http.Header{"Content-Type": {"image/jpeg"}, "Etag": {"fixture-etag"}}, Body: io.NopCloser(strings.NewReader(body)), Request: request}, nil
				}), &logs)
			if test.policy != nil {
				gateway.webSurfacePolicy = test.policy
			}
			gateway.debugEnabled = func() bool { return true }
			request := httptest.NewRequest(test.method, test.target, nil)
			request.Header.Set("If-None-Match", "fixture-etag")
			response := httptest.NewRecorder()
			gateway.ServeHTTP(response, request)
			if response.Code != test.status || calls != 1 || response.Header().Get("Content-Type") != "image/jpeg" || response.Header().Get("ETag") != "fixture-etag" {
				t.Fatalf("status=%d upstream=%d headers=%v", response.Code, calls, response.Header())
			}
			if test.method == http.MethodHead && response.Body.Len() != 0 || test.method == http.MethodGet && response.Body.String() != "fixture-image" {
				t.Fatal("image proxy changed the original response body")
			}
			if test.policy != nil && test.policy.callCount() != 0 {
				t.Fatal("image request consulted Web policy")
			}
			if !strings.Contains(logs.String(), "route=item_image") || strings.Contains(logs.String(), "route=emby_web") {
				t.Fatal("image route was not diagnosed independently from Web UI")
			}
			assertSecretsAbsent(t, logs.String(), "private-tag", "private-config-error")
		})
	}
}

// TestGatewayItemImagesDoNotDowngradeInvalidTokens prevents the anonymous
// image exception from bypassing an explicitly supplied invalid identity.
func TestGatewayItemImagesDoNotDowngradeInvalidTokens(t *testing.T) {
	for _, test := range []struct {
		name, query string
		header      http.Header
		resolveErr  error
		status      int
	}{
		{name: "empty query token", query: "?api_key="},
		{name: "duplicate query token", query: "?api_key=" + fixtureAccessToken + "&api_key=" + fixtureAccessToken},
		{name: "empty header token", header: http.Header{accessTokenHeader: {""}}},
		{name: "conflicting token carriers", query: "?api_key=other-token", header: http.Header{accessTokenHeader: {fixtureAccessToken}}},
		{name: "malformed application token", header: http.Header{"Authorization": {`MediaBrowser Token=unquoted-token`}}},
		{name: "unmapped token", query: "?api_key=" + fixtureAccessToken, resolveErr: embytoken.ErrTokenNotFound},
		{name: "revoked token", query: "?api_key=" + fixtureAccessToken, resolveErr: embytoken.ErrTokenRevoked},
		{name: "expired user", query: "?api_key=" + fixtureAccessToken, resolveErr: embytoken.ErrUserExpired, status: http.StatusForbidden},
	} {
		t.Run(test.name, func(t *testing.T) {
			calls := 0
			var logs bytes.Buffer
			gateway := newVideoTestGateway(t, "http://emby.invalid", &fakeTokenService{principal: fixturePrincipal(), resolveErr: test.resolveErr}, nil,
				roundTripFunc(func(request *http.Request) (*http.Response, error) {
					calls++
					return transferIntentHTTPResponse(request, http.StatusOK, "unexpected"), nil
				}), &logs)
			request := httptest.NewRequest(http.MethodGet, "/emby/Items/fixture/Images/Primary"+test.query, nil)
			request.Header = test.header
			response := httptest.NewRecorder()
			gateway.ServeHTTP(response, request)
			wantStatus := test.status
			if wantStatus == 0 {
				wantStatus = http.StatusUnauthorized
			}
			if response.Code != wantStatus || calls != 0 {
				t.Fatalf("invalid identity became anonymous: status=%d upstream=%d", response.Code, calls)
			}
			assertSecretsAbsent(t, logs.String(), fixtureAccessToken, "other-token", "unquoted-token")
		})
	}
}

// TestGatewaySessionCaseVariantsKeepLeaseLifecycle exercises real in-memory
// lease transitions and strict-intent revocation through each client spelling.
func TestGatewaySessionCaseVariantsKeepLeaseLifecycle(t *testing.T) {
	for _, spelling := range []struct {
		name, prefix, playing, progress, stopped string
	}{
		{name: "standard", prefix: "/Sessions/", playing: "Playing", progress: "Playing/Progress", stopped: "Playing/Stopped"},
		{name: "lowercase", prefix: "/sessions/", playing: "playing", progress: "playing/progress", stopped: "playing/stopped"},
		{name: "mixed", prefix: "/eMbY/sEsSiOnS/", playing: "pLaYiNg", progress: "pLaYiNg/pRoGrEsS", stopped: "pLaYiNg/sToPpEd"},
	} {
		t.Run(spelling.name, func(t *testing.T) {
			keys, err := p115quota.NewKeyDeriver("fixture-root-key")
			if err != nil {
				t.Fatal(err)
			}
			account, err := keys.PlaybackAccountKey("100")
			if err != nil {
				t.Fatal(err)
			}
			h := &leaseIdentityHarness{keys: keys, account: account, store: p115quota.NewMemoryLeaseStore(), now: time.Now()}
			principal := fixturePrincipal()
			if _, err := h.ResolveMediaPath(context.Background(), directplay.MediaPathResolveRequest{UserID: principal.User.ID, MappingID: principal.MappingID, DeviceID: principal.DeviceID, PlaySessionID: "session-1"}); err != nil {
				t.Fatal(err)
			}
			status := http.StatusNoContent
			var forwarded string
			gateway := newVideoTestGateway(t, "http://emby.invalid", &fakeTokenService{principal: principal}, nil,
				roundTripFunc(func(request *http.Request) (*http.Response, error) {
					if strings.HasSuffix(request.URL.Path, "/PlaybackInfo") {
						return transferIntentHTTPResponse(request, http.StatusOK, transferIntentResponse), nil
					}
					body, err := io.ReadAll(request.Body)
					if err != nil {
						t.Fatal(err)
					}
					forwarded = string(body)
					return transferIntentHTTPResponse(request, status, ""), nil
				}), &bytes.Buffer{})
			gateway.playbackSessionService = h
			gateway.proofs.now = func() time.Time { return h.now }
			send := func(event string, paused bool) {
				t.Helper()
				body := fmt.Sprintf(`{"ItemId":"item-1","MediaSourceId":"source-1","PlaySessionId":"session-1","PositionTicks":42,"IsPaused":%t}`, paused)
				request := httptest.NewRequest(http.MethodPost, spelling.prefix+event, strings.NewReader(body))
				request.Header.Set(accessTokenHeader, fixtureAccessToken)
				request.Header.Set("Content-Type", "application/json")
				response := httptest.NewRecorder()
				gateway.ServeHTTP(response, request)
				if response.Code != status || forwarded != body {
					t.Fatal("session request or upstream status changed")
				}
			}
			h.now = h.now.Add(5 * time.Second)
			send(spelling.playing, false)
			if !h.result.Found || h.result.State != p115quota.LeaseStateActive {
				t.Fatal("Playing did not advance the existing reservation")
			}
			h.now = h.now.Add(110 * time.Second)
			send(spelling.progress, false)
			if !h.result.Found || h.result.State != p115quota.LeaseStateActive {
				t.Fatal("Progress did not renew the existing active lease")
			}
			h.now = h.now.Add(110 * time.Second)
			send(spelling.progress, true)
			if !h.result.Found || h.result.State != p115quota.LeaseStatePaused {
				t.Fatal("paused Progress lost the renewed lease")
			}
			sendTransferIntentPlaybackInfo(gateway, `{"IsPlayback":true}`)
			proof, ok := gateway.LookupPlaybackProof(principal, "item-1", "source-1", "session-1")
			if !ok || !gateway.proofs.CanCreateTransfer(proof, principal) {
				t.Fatal("fixture explicit intent missing")
			}
			status = http.StatusBadGateway
			send(spelling.stopped, false)
			usage, _ := h.store.AccountUsage(context.Background(), account, h.now)
			if usage.OccupiedStreams != 1 || !gateway.proofs.CanCreateTransfer(proof, principal) {
				t.Fatal("failed upstream stop changed lease or transfer intent")
			}
			status = http.StatusNoContent
			send(spelling.stopped, false)
			usage, _ = h.store.AccountUsage(context.Background(), account, h.now)
			if !h.result.Found || usage.OccupiedStreams != 0 || gateway.proofs.CanCreateTransfer(proof, principal) {
				t.Fatal("successful stop did not release lease and revoke intent")
			}
			send(spelling.progress, false)
			usage, _ = h.store.AccountUsage(context.Background(), account, h.now)
			if h.result.Found || usage.OccupiedStreams != 0 {
				t.Fatal("late progress rebuilt the stopped lease")
			}
		})
	}
}

// TestPlaybackSessionCaseMatchingKeepsExactRouteBoundary prevents a casing
// compatibility change from accepting other methods or alternate path shapes.
func TestPlaybackSessionCaseMatchingKeepsExactRouteBoundary(t *testing.T) {
	for _, test := range []struct{ method, target string }{
		{http.MethodGet, "/emby/sessions/playing"},
		{http.MethodPost, "/emby/sessions/playing/"},
		{http.MethodPost, "/emby/sessions/playing/progress/extra"},
		{http.MethodPost, "/emby/sessions/playing/stopped/"},
		{http.MethodPost, "/emby/Sessions/Playing/%50rogress"},
	} {
		request := httptest.NewRequest(test.method, test.target, nil)
		if got := playbackSessionEventKindForRequest(request); got != playbackSessionEventNone {
			t.Errorf("%s %s accepted as %s", test.method, test.target, got)
		}
	}
}
