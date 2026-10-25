package playbackgateway

import (
	"bytes"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/konghang/ember/backend/internal/services/directplay"
)

// TestGatewayDiagnosticContractLogProducer records the real Gateway logger
// through in-memory HTTP exchanges, without a listener, Python or external IO.
func TestGatewayDiagnosticContractLogProducer(t *testing.T) {
	type logCase struct {
		Name  string   `json:"name"`
		Lines []string `json:"lines"`
	}
	packet := struct {
		Kind  string    `json:"kind"`
		Cases []logCase `json:"cases"`
	}{Kind: "gateway"}
	for _, name := range []string{"authentication", "header_parse", "fallback", "bound"} {
		var output bytes.Buffer
		upstreamCalls := 0
		transport := roundTripFunc(func(request *http.Request) (*http.Response, error) {
			upstreamCalls++
			status := http.StatusPartialContent
			if name == "authentication" {
				if _, err := io.Copy(io.Discard, request.Body); err != nil {
					t.Fatal(err)
				}
				status = http.StatusUnauthorized
			} else if name != "fallback" {
				t.Fatalf("unexpected upstream call in %s", name)
			}
			return &http.Response{StatusCode: status, Header: http.Header{"Content-Type": {"application/json"}},
				Body: io.NopCloser(strings.NewReader("{}")), Request: request}, nil
		})
		direct := &fakeDirectPlayService{result: validFixtureRedirectCandidate()}
		if name == "fallback" {
			direct.err = directplay.ErrSourceReadTimeout
		}
		upstream, err := url.Parse("http://contract.invalid/emby")
		if err != nil {
			t.Fatal(err)
		}
		gateway, err := New(Config{Upstream: upstream, TokenService: &fakeTokenService{principal: fixturePrincipal()},
			DirectPlayService: direct, Transport: transport, Logger: log.New(&output, "", 0), Debug: true})
		if err != nil {
			t.Fatal(err)
		}
		request := newVideoRequest(http.MethodGet, "/emby/Videos/item-1/stream.mkv?MediaSourceId=source-1&PlaySessionId=session-1&Static=true")
		wantStatus, wantUpstreamCalls := http.StatusPartialContent, 1
		switch name {
		case "authentication":
			request = httptest.NewRequest(http.MethodPost, authenticationPath,
				strings.NewReader(`{"Username":"contract-private-login","Pw":"contract-private-password"}`))
			request.Header.Set("Content-Type", "application/json")
			wantStatus = http.StatusUnauthorized
		case "header_parse":
			request = newVideoRequest(http.MethodGet, "/emby/System/Info")
			request.Header.Set(embyAuthorizationHeader, `Emby Client="contract-private-client`)
			wantStatus, wantUpstreamCalls = http.StatusUnauthorized, 0
		case "fallback":
			gateway.proofs.Record([]PlaybackProof{fixturePlaybackProof("mapping-1", "item-1", "source-1", "session-1")})
		case "bound":
			// Reuse the production publication path to inherit an active
			// client identity; the emitted binding must explain both refs.
			proofs, _, original := bindingFixture(t)
			fresh := original
			fresh.PlaySessionID, fresh.transferIntent = "session-1", false
			_, guard := proofs.BeginOnDemand(fixturePrincipal(), fresh.ItemID, fresh.MediaSourceID)
			if guard == nil {
				t.Fatal("missing fixture publication guard")
			}
			_, published := proofs.PublishOnDemand(guard, []PlaybackProof{fresh})
			proofs.ReleasePublication(guard)
			if !published {
				t.Fatal("fixture proof publication failed")
			}
			gateway.proofs = proofs
			wantStatus, wantUpstreamCalls = http.StatusFound, 0
		}
		request.Header.Set("User-Agent", "SenPlayer/6.0.4")
		request.Header.Set("Cookie", "contract-cookie=contract-private-cookie")
		response := httptest.NewRecorder()
		gateway.ServeHTTP(response, request)
		if response.Code != wantStatus || upstreamCalls != wantUpstreamCalls {
			t.Fatalf("%s status=%d upstreamCalls=%d", name, response.Code, upstreamCalls)
		}
		if name == "bound" && (len(direct.snapshot()) != 1 || direct.snapshot()[0].PlaySessionID != "client") {
			t.Fatal("fixture did not bind the real client session")
		}
		assertSecretsAbsent(t, output.String(), fixtureAccessToken, fixtureRedirectURL,
			"contract-private-login", "contract-private-password", "contract-private-cookie", "contract-private-client")
		packet.Cases = append(packet.Cases, logCase{Name: name, Lines: strings.Split(strings.TrimSpace(output.String()), "\n")})
	}
	encoded, err := json.Marshal(packet)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("GATEWAY_DIAGNOSTIC_CONTRACT %s", encoded)
}
