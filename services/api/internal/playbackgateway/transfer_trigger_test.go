package playbackgateway

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/konghang/ember/backend/internal/config"
	"github.com/konghang/ember/backend/internal/services/directplay"
	"gorm.io/gorm"
)

type transferTriggerPolicyFunc func(context.Context) (string, error)

// P115TransferTriggerMode exposes a test-owned configuration read boundary.
func (policy transferTriggerPolicyFunc) P115TransferTriggerMode(ctx context.Context) (string, error) {
	return policy(ctx)
}

// TestTransferTriggerAdmissionModes keeps compatibility separate from explicit
// intent and preserves strict defaults, HEAD protection and safe diagnostics.
func TestTransferTriggerAdmissionModes(t *testing.T) {
	for _, test := range []struct {
		name, method, mode, trigger string
		intent, missing, allowed    bool
		err                         error
	}{
		{name: "default strict", missing: true, trigger: "policy_denied"},
		{name: "strict denies prefetch", mode: "strict", trigger: "policy_denied"},
		{name: "compatible admits video", mode: "compatible", allowed: true, trigger: "compatible_request"},
		{name: "explicit strict", mode: "strict", intent: true, allowed: true, trigger: "explicit_playback"},
		{name: "explicit compatible", mode: "compatible", intent: true, allowed: true, trigger: "explicit_playback"},
		{name: "explicit survives policy failure", intent: true, err: errors.New("fixture-policy-secret"), allowed: true, trigger: "explicit_playback"},
		{name: "policy failure", err: errors.New("fixture-policy-secret"), trigger: "policy_denied"},
		{name: "invalid mode", mode: "fixture-policy-secret", trigger: "policy_denied"},
		{name: "HEAD compatible", method: http.MethodHead, mode: "compatible", intent: true, trigger: "policy_denied"},
		{name: "POST compatible", method: http.MethodPost, mode: "compatible", trigger: "policy_denied"},
	} {
		t.Run(test.name, func(t *testing.T) {
			method := test.method
			if method == "" {
				method = http.MethodGet
			}
			reads := 0
			policy := transferTriggerPolicyFunc(func(ctx context.Context) (string, error) {
				reads++
				deadline, bounded := ctx.Deadline()
				if !bounded || time.Until(deadline) > 500*time.Millisecond {
					t.Error("configuration read has no bounded deadline")
				}
				return test.mode, test.err
			})
			if test.missing {
				policy = nil
			}
			gateway, proof, logs := newTransferTriggerFixture(t, policy, test.intent)
			allowed, trigger := gateway.canCreateVideoTransfer(context.Background(), method, proof, fixturePrincipal())
			if allowed != test.allowed || trigger != test.trigger {
				t.Fatalf("allowed=%t trigger=%s, want %t/%s", allowed, trigger, test.allowed, test.trigger)
			}
			if (test.intent || method != http.MethodGet) && reads != 0 {
				t.Fatal("strict permit or non-GET request unnecessarily loaded policy")
			}
			if !test.intent && gateway.proofs.CanCreateTransfer(proof, fixturePrincipal()) {
				t.Fatal("compatibility manufactured explicit playback intent")
			}
			assertSecretsAbsent(t, logs.String(), "fixture-policy-secret")
		})
	}
}

// TestTransferTriggerRechecksQueuedRequest rejects a proof replaced or stopped
// while a configuration read is in flight, and respects caller cancellation.
func TestTransferTriggerRechecksQueuedRequest(t *testing.T) {
	for _, change := range []string{"replace", "stop", "expire", "cancel", "identity"} {
		t.Run(change, func(t *testing.T) {
			gateway, proof, _ := newTransferTriggerFixture(t, nil, false)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			principal := fixturePrincipal()
			gateway.transferTriggerPolicy = transferTriggerPolicyFunc(func(context.Context) (string, error) {
				switch change {
				case "replace":
					gateway.proofs.Record([]PlaybackProof{proof})
				case "stop":
					gateway.proofs.stopPlaybackBinding(principal, proof.ItemID, proof.MediaSourceID, proof.PlaySessionID)
				case "expire":
					gateway.proofs.now = func() time.Time { return proof.ExpiresAt.Add(time.Second) }
				case "cancel":
					cancel()
				}
				return config.TransferTriggerModeCompatible, nil
			})
			if change == "identity" {
				principal.DeviceID = "another-device"
			}
			if allowed, _ := gateway.canCreateVideoTransfer(ctx, http.MethodGet, proof, principal); allowed {
				t.Fatal("stale, canceled or mismatched video obtained compatibility admission")
			}
		})
	}
}

// newTransferTriggerFixture uses production Gateway construction and a stored
// proof, while keeping every identity/configuration dependency in process.
func newTransferTriggerFixture(t *testing.T, policy transferTriggerPolicyFunc, intent bool) (*Gateway, PlaybackProof, *bytes.Buffer) {
	t.Helper()
	upstream, _ := url.Parse("http://emby.invalid")
	logs := &bytes.Buffer{}
	configuration := Config{Upstream: upstream, TokenService: &fakeTokenService{principal: fixturePrincipal()}, Logger: log.New(logs, "", 0), Debug: true}
	if policy != nil {
		configuration.TransferTriggerPolicy = policy
	}
	gateway, err := New(configuration)
	if err != nil {
		t.Fatal(err)
	}
	proof := fixturePlaybackProof("mapping-1", "item-1", "source-1", "session-1")
	proof.transferIntent = intent
	gateway.proofs.Record([]PlaybackProof{proof})
	proof, ok := gateway.proofs.Lookup(proof.MappingID, proof.ItemID, proof.MediaSourceID, proof.PlaySessionID)
	if !ok {
		t.Fatal("fixture proof was not recorded")
	}
	return gateway, proof, logs
}

// P115TransferTriggerMode gives runtime construction tests the same new
// settings contract without introducing a database or environment fallback.
func (settings fakeRuntimeSettings) P115TransferTriggerMode(ctx context.Context) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if mode := settings[config.P115TransferTriggerModeKey]; mode != "" {
		return mode, nil
	}
	return config.TransferTriggerModeStrict, nil
}

// TestRuntimeBindsTransferTriggerPolicy proves production wiring consumes the
// administrator policy rather than silently using the strict nil dependency.
func TestRuntimeBindsTransferTriggerPolicy(t *testing.T) {
	settings := validRuntimeSettings()
	settings[config.P115TransferTriggerModeKey] = config.TransferTriggerModeCompatible
	runtime, err := NewProductionRuntime(context.Background(), runtimeEnvironment(nil), ProductionDependencies{
		Database: &gorm.DB{}, Settings: settings, DirectPlayService: &fakeDirectPlayService{}, Logger: log.New(&bytes.Buffer{}, "", 0),
		Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
			return transferIntentHTTPResponse(request, http.StatusOK, `{"Id":"server-1","Version":"4.9.3.0","ServerName":"Fixture"}`), nil
		}),
	})
	if err != nil {
		t.Fatal(err)
	}
	mux := runtime.Handler().(*http.ServeMux)
	handler, _ := mux.Handler(httptest.NewRequest(http.MethodGet, "/Videos/item-1/stream.mkv", nil))
	gateway := handler.(*Gateway)
	proof := fixturePlaybackProof("mapping-1", "item-1", "source-1", "session-1")
	gateway.proofs.Record([]PlaybackProof{proof})
	proof, _ = gateway.proofs.Lookup(proof.MappingID, proof.ItemID, proof.MediaSourceID, proof.PlaySessionID)
	if allowed, _ := gateway.canCreateVideoTransfer(context.Background(), http.MethodGet, proof, fixturePrincipal()); !allowed {
		t.Fatal("production runtime did not wire compatibility policy")
	}
}

// TestGatewayCompatibleTransferPolicyUsesNormalVideoFlow replays SenPlayer's
// query-only false intent through real response observation and video routing.
func TestGatewayCompatibleTransferPolicyUsesNormalVideoFlow(t *testing.T) {
	for _, test := range []struct {
		name, mode, changeMode, method, trigger string
		existing                                bool
		status                                  int
	}{
		{name: "strict keeps fallback", mode: "strict", trigger: "policy_denied", status: http.StatusPartialContent},
		{name: "compatible admits first transfer", mode: "compatible", trigger: "compatible_request", status: http.StatusFound},
		{name: "disabled while queued", mode: "compatible", changeMode: "strict", trigger: "policy_denied", status: http.StatusPartialContent},
		{name: "enabled while queued", mode: "strict", changeMode: "compatible", trigger: "compatible_request", status: http.StatusFound},
		{name: "HEAD never creates", mode: "compatible", method: http.MethodHead, trigger: "policy_denied", status: http.StatusPartialContent},
		{name: "existing file does not read mode", mode: "compatible", existing: true, trigger: "existing_target", status: http.StatusFound},
	} {
		t.Run(test.name, func(t *testing.T) {
			mode, reads := test.mode, 0
			var logs bytes.Buffer
			service := transferIntentDirectPlayFunc(func(_ context.Context, request directplay.MediaPathResolveRequest) (directplay.RedirectCandidate, error) {
				if test.changeMode != "" {
					mode = test.changeMode
				}
				if !test.existing && (request.CanCreateTransfer == nil || !request.CanCreateTransfer()) {
					return directplay.RedirectCandidate{}, directplay.ErrPlaybackIntentRequired
				}
				candidate := validFixtureRedirectCandidate()
				candidate.Preexisting = test.existing
				return candidate, nil
			})
			const videoTarget = "/emby/videos/item-1/original.mkv?MediaSourceId=source-1&PlaySessionId=session-1&DeviceId=device-1"
			gateway := newVideoTestGateway(t, "http://emby.invalid", &fakeTokenService{principal: fixturePrincipal()}, service,
				roundTripFunc(func(request *http.Request) (*http.Response, error) {
					if strings.Contains(request.URL.Path, "/PlaybackInfo") {
						body, _ := io.ReadAll(request.Body)
						if string(body) != `{"DeviceProfile":{}}` || request.URL.Query().Get("IsPlayback") != "false" {
							t.Error("compatibility rewrote client playback intent")
						}
						return transferIntentHTTPResponse(request, http.StatusOK, `{"MediaSources":[{"Id":"source-1","ItemId":"item-1","Path":"/mnt/media/fixture.mkv","Container":"mkv","Size":1024,"SupportsDirectPlay":true,"SupportsDirectStream":true}],"PlaySessionId":"session-1"}`), nil
					}
					if request.URL.RequestURI() != videoTarget || request.Header.Get("Range") != "bytes=0-" {
						t.Error("policy fallback changed the original video request")
					}
					return transferIntentHTTPResponse(request, http.StatusPartialContent, "fixture-video"), nil
				}), &logs)
			gateway.transferTriggerPolicy = transferTriggerPolicyFunc(func(context.Context) (string, error) {
				reads++
				return mode, nil
			})
			post := httptest.NewRequest(http.MethodPost, "/emby/Items/item-1/PlaybackInfo?IsPlayback=false&MediaSourceId=source-1", strings.NewReader(`{"DeviceProfile":{}}`))
			post.Header.Set(accessTokenHeader, fixtureAccessToken)
			post.Header.Set("Content-Type", "application/json")
			gateway.ServeHTTP(httptest.NewRecorder(), post)
			method := test.method
			if method == "" {
				method = http.MethodGet
			}
			video := newVideoRequest(method, videoTarget)
			video.Header.Set("User-Agent", "SenPlayer/6.0.4")
			video.Header.Set("Range", "bytes=0-")
			response := httptest.NewRecorder()
			gateway.ServeHTTP(response, video)
			if response.Code != test.status || !strings.Contains(logs.String(), "transferTrigger="+test.trigger) {
				t.Fatalf("status=%d expected=%d; missing trigger=%s in logs=%s", response.Code, test.status, test.trigger, logs.String())
			}
			if test.existing && reads != 0 {
				t.Fatal("target reuse depended on configuration availability")
			}
			proof, ok := gateway.proofs.Lookup("mapping-1", "item-1", "source-1", "session-1")
			if !ok || gateway.proofs.CanCreateTransfer(proof, fixturePrincipal()) {
				t.Fatal("compatibility manufactured strict transfer permission")
			}
		})
	}
}
