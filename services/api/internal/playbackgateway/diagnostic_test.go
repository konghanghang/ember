package playbackgateway

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"github.com/konghang/ember/backend/internal/services/directplay"
	"github.com/konghang/ember/backend/internal/services/p115quota"
)

// TestVideoDiagnosticPrecedesUpstream protects early visibility, bounded
// request shape, fallback timing and the single final Info decision.
func TestVideoDiagnosticPrecedesUpstream(t *testing.T) {
	var logs bytes.Buffer
	g := newVideoTestGateway(t, "http://emby.invalid", &fakeTokenService{principal: fixturePrincipal()}, nil,
		roundTripFunc(func(r *http.Request) (*http.Response, error) {
			if !strings.Contains(logs.String(), "code=video_request_started") || !strings.Contains(logs.String(), "code=video_fallback_started") {
				t.Error("upstream called before start diagnostics")
			}
			return &http.Response{StatusCode: 206, Header: make(http.Header), Body: io.NopCloser(strings.NewReader("fixture")), Request: r}, nil
		}), &logs)
	g.debugEnabled = func() bool { return true }
	r := newVideoRequest("GET", "/Videos/item-1/stream.mkv?MediaSourceId=source-1&PlaySessionId=session-secret&Static=true")
	r.Header.Set("Range", "bytes=0-1023")
	r.Header.Set("Purpose", "untrusted-secret")
	g.ServeHTTP(httptest.NewRecorder(), r)
	for _, want := range []string{"requestId=", "rangeKind=bounded", "rangeStart=0", "rangeEnd=1023", "purpose=other", "code=video_fallback_headers", "code=video_fallback_completed"} {
		if !strings.Contains(logs.String(), want) {
			t.Errorf("missing %s", want)
		}
	}
	assertSingleDecisionLog(t, logs.String(), "fallback", "proof", "playback_proof_missing")
	assertSecretsAbsent(t, logs.String(), fixtureAccessToken, "session-secret", "untrusted-secret")
	ids := regexp.MustCompile(`requestId=([^\s]+)`).FindAllStringSubmatch(logs.String(), -1)
	if len(ids) < 5 {
		t.Fatal("missing correlation IDs")
	}
	for _, id := range ids {
		if id[1] != ids[0][1] {
			t.Fatal("request ID changed within chain")
		}
	}
}

// TestDiagnosticRangeNeverEchoesInvalidHeaders covers all accepted numeric
// forms, duplicate/multiple ranges and malformed or oversized inputs.
func TestDiagnosticRangeNeverEchoesInvalidHeaders(t *testing.T) {
	for _, tc := range []struct{ value, want string }{
		{"bytes=1-2", "rangeKind=bounded rangeStart=1 rangeEnd=2"},
		{"bytes=5-", "rangeKind=open rangeStart=5"}, {"bytes=-5", "rangeKind=suffix rangeLength=5"},
		{"bytes=0-1,5-6", "rangeKind=multiple"}, {"bytes=2-1", "rangeKind=invalid"},
		{"bytes=-0", "rangeKind=invalid"}, {"bytes=secret-", "rangeKind=invalid"},
		{"bytes=9223372036854775808-", "rangeKind=invalid"}, {strings.Repeat("s", 200), "rangeKind=invalid"},
	} {
		h := http.Header{"Range": []string{tc.value}}
		if got := rangeDiagnostics(h); got != tc.want {
			t.Errorf("got=%s want=%s", got, tc.want)
		}
	}
	if rangeDiagnostics(http.Header{}) != "rangeKind=none" || rangeDiagnostics(http.Header{"Range": []string{"bytes=0-1", "bytes=2-3"}}) != "rangeKind=invalid" {
		t.Fatal("missing/duplicate range handling")
	}
	if purposeDiagnostic(http.Header{"Purpose": []string{"prefetch"}}, "Purpose") != "prefetch" {
		t.Fatal("prefetch classification")
	}
}

// TestDiagnosticSessionRefIsScopedAndNotCredentialDerived checks that video
// and session events can share a reference without exposing source identities.
func TestDiagnosticSessionRefIsScopedAndNotCredentialDerived(t *testing.T) {
	p := fixturePrincipal()
	ref := diagnosticSessionRef(p, "session-secret")
	if ref == "unavailable" || ref != diagnosticSessionRef(p, "session-secret") {
		t.Fatal("unstable reference")
	}
	p.DeviceID = "other"
	if ref == diagnosticSessionRef(p, "session-secret") {
		t.Fatal("device isolation lost")
	}
	if diagnosticSessionRef(p, "") != "unavailable" {
		t.Fatal("empty session accepted")
	}
}

// TestLeaseMissingAndFailureDiagnostics retain session correlation for errors
// and missing reverse mappings without changing any lease behavior.
func TestLeaseMissingAndFailureDiagnostics(t *testing.T) {
	for _, fail := range []bool{false, true} {
		var logs bytes.Buffer
		g := newVideoTestGateway(t, "http://emby.invalid", &fakeTokenService{principal: fixturePrincipal()}, nil, nil, &logs)
		g.debugEnabled = func() bool { return true }
		fake := &fakePlaybackSessionService{}
		want := "playback_lease_not_found"
		if fail {
			fake.err = errors.New("secret-error-body")
			want = "playback_lease_update_failed"
		}
		g.playbackSessionService = fake
		r := withDiagnosticRequest(httptest.NewRequest("POST", "/Sessions/Playing", nil))
		g.updatePlaybackSessionLease(r.Context(), fixturePrincipal(), playbackSessionEventSnapshot{kind: playbackSessionEventStart, snapshotState: "recorded", playSessionID: "session-secret"})
		if !strings.Contains(logs.String(), want) || !strings.Contains(logs.String(), "sessionRef=") {
			t.Fatal("missing error correlation")
		}
		assertSecretsAbsent(t, logs.String(), "secret-error-body", "session-secret")
	}
}

// TestFallbackFailureAndInfoLevel preserve the single decision on transport
// cancellation/failure and keep all new process diagnostics behind Debug.
func TestFallbackFailureAndInfoLevel(t *testing.T) {
	for _, debug := range []bool{false, true} {
		var logs bytes.Buffer
		g := newVideoTestGateway(t, "http://emby.invalid", &fakeTokenService{principal: fixturePrincipal()}, nil,
			roundTripFunc(func(*http.Request) (*http.Response, error) { return nil, context.Canceled }), &logs)
		g.debugEnabled = func() bool { return debug }
		g.ServeHTTP(httptest.NewRecorder(), newVideoRequest("GET", "/Videos/item-1/stream.mkv?MediaSourceId=source-1&PlaySessionId=session-secret&Static=true"))
		if strings.Count(logs.String(), "decision=") != 1 {
			t.Fatal("duplicated final decision")
		}
		if strings.Contains(logs.String(), "code=video_request_started") != debug || strings.Contains(logs.String(), "code=video_fallback_completed") != debug {
			t.Fatal("debug gating failed")
		}
		if strings.Contains(logs.String(), "code=video_fallback_headers") {
			t.Fatal("invented upstream headers")
		}
	}
}

// TestPlaybackLeaseDiagnostics preserves the upstream response while exposing
// whether accepted playback events updated an existing lease.
func TestPlaybackLeaseDiagnostics(t *testing.T) {
	for _, debug := range []bool{false, true} {
		var logs bytes.Buffer
		g := newVideoTestGateway(t, "http://emby.invalid", &fakeTokenService{principal: fixturePrincipal()}, nil,
			roundTripFunc(func(r *http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: 204, Header: make(http.Header), Body: http.NoBody, Request: r}, nil
			}), &logs)
		g.debugEnabled = func() bool { return debug }
		g.playbackSessionService = &fakePlaybackSessionService{result: directplay.PlaybackSessionEventResult{
			Found: true, State: p115quota.LeaseStateActive, Account: p115quota.LeaseUsage{ActiveStreams: 1, OccupiedStreams: 1},
		}}
		r := newVideoRequest("POST", "/Sessions/Playing")
		r.Body = io.NopCloser(strings.NewReader(`{"ItemId":"item-1","MediaSourceId":"source-1","PlaySessionId":"session-secret"}`))
		r.Header.Set("Content-Type", "application/json")
		response := httptest.NewRecorder()
		g.ServeHTTP(response, r)
		if response.Code != 204 {
			t.Fatal(response.Code)
		}
		if strings.Contains(logs.String(), "code=playback_lease_updated") != debug {
			t.Fatalf("debug=%t logs=%s", debug, logs.String())
		}
		if debug && (!strings.Contains(logs.String(), "state=active") || !strings.Contains(logs.String(), "sessionRef=")) {
			t.Fatal("missing lease correlation")
		}
		assertSecretsAbsent(t, logs.String(), "session-secret", fixtureAccessToken)
	}
}
