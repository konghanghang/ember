package playbackgateway

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// TestPlaybackInfoIntentDiagnostic preserves proxy bytes and eligibility even
// when optional diagnostic fields are malformed or contain secrets.
func TestPlaybackInfoIntentDiagnostic(t *testing.T) {
	for _, tc := range []struct{ name, body, want string }{
		{"true", `{"IsPlayback":true,"EnableDirectPlay":false,"StartTimeTicks":0,"CurrentPlaySessionId":"session-secret","DeviceProfile":{"Token":"profile-secret"}}`, "isPlayback=true"},
		{"false", `{"IsPlayback":false}`, "isPlayback=false"},
		{"missing", `{}`, "isPlayback=missing"},
		{"null", `{"IsPlayback":null}`, "isPlayback=null"},
		{"invalid", `{"IsPlayback":"secret-value"}`, "isPlayback=invalid"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var logs bytes.Buffer
			g := newVideoTestGateway(t, "http://emby.invalid", &fakeTokenService{principal: fixturePrincipal()}, nil, nil, &logs)
			g.debugEnabled = func() bool { return true }
			r := withDiagnosticRequest(httptest.NewRequest(http.MethodPost, "/emby/Items/item-1/PlaybackInfo", strings.NewReader(tc.body)))
			r.Header.Set("Content-Type", "application/json")
			_, eligible, _ := g.preparePlaybackInfoRequest(r, fixturePrincipal())
			if !eligible {
				t.Fatal("diagnostics changed proof eligibility")
			}
			body, err := io.ReadAll(r.Body)
			if err != nil || string(body) != tc.body {
				t.Fatal("request bytes changed")
			}
			for _, want := range []string{"code=playback_info_request_observed", "source=client", "itemRef=", "requestId=", tc.want} {
				if !strings.Contains(logs.String(), want) {
					t.Errorf("missing %s: %s", want, logs.String())
				}
			}
			assertSecretsAbsent(t, logs.String(), "session-secret", "profile-secret", "secret-value")
		})
	}
}

// TestPlaybackIntentDiagnosticCorrelation covers the client-response and
// gateway-lookup paths through the real proxy, followed by proof-cache reuse.
func TestPlaybackIntentDiagnosticCorrelation(t *testing.T) {
	for _, clientFirst := range []bool{true, false} {
		t.Run(strconv.FormatBool(clientFirst), func(t *testing.T) {
			var logs bytes.Buffer
			calls := 0
			payload := `{"MediaSources":[{"Id":"source-1","ItemId":"item-1","Path":"/fixture/one.mkv","Container":"mkv","Size":1024,"SupportsDirectPlay":true}],"PlaySessionId":"session-secret"}`
			g := newVideoTestGateway(t, "http://emby.invalid", &fakeTokenService{principal: fixturePrincipal()},
				&fakeDirectPlayService{result: validFixtureRedirectCandidate()}, roundTripFunc(func(r *http.Request) (*http.Response, error) {
					calls++
					if !strings.HasSuffix(r.URL.Path, "/PlaybackInfo") {
						t.Fatalf("unexpected upstream path: %s", r.URL.Path)
					}
					return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(payload)), Request: r}, nil
				}), &logs)
			g.debugEnabled = func() bool { return true }
			if clientFirst {
				r := newVideoRequest("POST", "/Items/item-1/PlaybackInfo")
				r.Body = io.NopCloser(strings.NewReader(`{"IsPlayback":true}`))
				r.Header.Set("Content-Type", "application/json")
				w := httptest.NewRecorder()
				g.ServeHTTP(w, r)
				if w.Code != 200 || w.Body.String() != payload {
					t.Fatal("client PlaybackInfo response changed")
				}
				if !strings.Contains(logs.String(), "code=playback_info_response_observed") || !strings.Contains(logs.String(), "source=client") {
					t.Fatal("missing client response correlation")
				}
			}
			for i := 0; i < 2; i++ {
				w := httptest.NewRecorder()
				g.ServeHTTP(w, newVideoRequest("GET", "/Videos/item-1/stream?MediaSourceId=source-1&Static=true"))
				if w.Code != 302 {
					t.Fatalf("video behavior changed: %d logs=%s", w.Code, logs.String())
				}
			}
			if calls != 1 || !strings.Contains(logs.String(), "source=proof_cache") {
				t.Fatal("proof reuse or diagnostic source changed")
			}
			if !clientFirst && !strings.Contains(logs.String(), "source=gateway method=GET bodyState=not_applicable") {
				t.Fatal("missing actual internal lookup diagnostic")
			}
			refs := regexp.MustCompile(`itemRef=(\S+)`).FindAllStringSubmatch(logs.String(), -1)
			if len(refs) < 4 {
				t.Fatal("missing item correlation")
			}
			for _, ref := range refs {
				if ref[1] != diagnosticItemRef(fixturePrincipal(), "item-1") {
					t.Fatal("item correlation differs across requests")
				}
			}
			assertSecretsAbsent(t, logs.String(), "session-secret", fixtureAccessToken)
		})
	}
}

// TestPlaybackIntentFieldStates checks the entire logged whitelist's value
// categories, including invalid types, overflow and case ambiguity.
func TestPlaybackIntentFieldStates(t *testing.T) {
	for _, tc := range []struct{ kind, raw, want string }{
		{"ticks", "0", "zero"}, {"ticks", "42", "positive"}, {"ticks", "-1", "invalid"},
		{"ticks", "9223372036854775808", "invalid"}, {"ticks", "1.5", "invalid"},
		{"string", `""`, "empty"}, {"string", `"secret"`, "present"}, {"string", "1", "invalid"},
		{"object", `{"Token":"secret"}`, "present"}, {"object", "[]", "invalid"},
		{"bool", "null", "null"}, {"bool", "true", "true"}, {"bool", "false", "false"},
	} {
		if got := playbackIntentField(map[string]json.RawMessage{"Field": json.RawMessage(tc.raw)}, "Field", tc.kind); got != tc.want {
			t.Errorf("%s %s: got %s want %s", tc.kind, tc.raw, got, tc.want)
		}
	}
	if playbackIntentField(map[string]json.RawMessage{"IsPlayback": []byte("true"), "isPlayback": []byte("false")}, "IsPlayback", "bool") != "invalid" {
		t.Fatal("case ambiguity accepted")
	}
}

// TestPlaybackInfoDiagnosticBoundaries ensures absent, malformed, oversized and
// unsupported bodies retain existing eligibility and exact forwarding bytes.
func TestPlaybackInfoDiagnosticBoundaries(t *testing.T) {
	for _, tc := range []struct {
		name, body, contentType, state string
		eligible                       bool
	}{
		{"invalid", `{"IsPlayback":`, "application/json", "invalid_json", false},
		{"large", strings.Repeat("x", 65), "application/json", "read_error_or_too_large", false},
		{"xml", "<secret/>", "application/xml", "unsupported_content_type", false},
		{"null", "null", "application/json", "invalid_json", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var logs bytes.Buffer
			g := newVideoTestGateway(t, "http://emby.invalid", &fakeTokenService{principal: fixturePrincipal()}, nil, nil, &logs)
			g.debugEnabled = func() bool { return true }
			g.maxPlaybackInfoRequestBytes = 64
			r := withDiagnosticRequest(httptest.NewRequest("POST", "/emby/Items/item-1/PlaybackInfo", strings.NewReader(tc.body)))
			r.Header.Set("Content-Type", tc.contentType)
			_, eligible, _ := g.preparePlaybackInfoRequest(r, fixturePrincipal())
			body, err := io.ReadAll(r.Body)
			if eligible != tc.eligible || err != nil || string(body) != tc.body || !strings.Contains(logs.String(), "bodyState="+tc.state) {
				t.Fatalf("boundary changed: eligible=%t err=%v logs=%s", eligible, err, logs.String())
			}
		})
	}
	var logs bytes.Buffer
	g := newVideoTestGateway(t, "http://emby.invalid", &fakeTokenService{principal: fixturePrincipal()}, nil, nil, &logs)
	g.debugEnabled = func() bool { return false }
	r := httptest.NewRequest("POST", "/emby/Items/item-1/PlaybackInfo", strings.NewReader(`{"IsPlayback":true}`))
	r.Header.Set("Content-Type", "application/json")
	g.preparePlaybackInfoRequest(r, fixturePrincipal())
	if logs.Len() != 0 {
		t.Fatal("intent diagnostics leaked outside Debug")
	}
}

// TestDiagnosticItemRefScope preserves correlation across playback sessions
// while isolating media, login mappings and devices without raw identifiers.
func TestDiagnosticItemRefScope(t *testing.T) {
	p := fixturePrincipal()
	ref := diagnosticItemRef(p, "item-1")
	if ref == "unavailable" || ref != diagnosticItemRef(p, "item-1") || ref == diagnosticItemRef(p, "item-2") {
		t.Fatal("invalid media correlation")
	}
	p.DeviceID = "other-device"
	if ref == diagnosticItemRef(p, "item-1") {
		t.Fatal("device isolation lost")
	}
	p = fixturePrincipal()
	p.MappingID = "other-mapping"
	if ref == diagnosticItemRef(p, "item-1") || diagnosticItemRef(p, "") != "unavailable" {
		t.Fatal("login isolation or missing item handling lost")
	}
}
