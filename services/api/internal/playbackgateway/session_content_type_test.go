package playbackgateway

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestSessionJSONSidecarContentTypeCompatibility verifies known JSON DTOs
// independently of mislabeled headers, without modifying the upstream request.
func TestSessionJSONSidecarContentTypeCompatibility(t *testing.T) {
	for _, contentType := range []string{"", "application/json", "text/plain", "application/octet-stream", "application/x-www-form-urlencoded", "application/json; broken"} {
		for _, status := range []int{204, 400} {
			t.Run(contentType+http.StatusText(status), func(t *testing.T) {
				const body = `{ "ItemId":"item-1", "MediaSourceId":"source-1", "PlaySessionId":"session-1", "PositionTicks":42 }`
				sessions := &fakePlaybackSessionService{}
				gateway := newVideoTestGateway(t, "http://emby.invalid", &fakeTokenService{principal: fixturePrincipal()}, &fakeDirectPlayService{}, roundTripFunc(func(r *http.Request) (*http.Response, error) {
					data, _ := io.ReadAll(r.Body)
					if string(data) != body || r.Header.Get("Content-Type") != contentType {
						t.Fatal("sidecar modified original request")
					}
					return transferIntentHTTPResponse(r, status, ""), nil
				}), &bytes.Buffer{})
				gateway.playbackSessionService = sessions
				r := httptest.NewRequest("POST", "/Sessions/Playing/Progress", strings.NewReader(body))
				r.Header.Set(accessTokenHeader, fixtureAccessToken)
				if contentType != "" {
					r.Header.Set("Content-Type", contentType)
				}
				response := httptest.NewRecorder()
				gateway.ServeHTTP(response, r)
				want := 0
				if status == 204 {
					want = 1
				}
				if response.Code != status || len(sessions.events) != want {
					t.Fatalf("status=%d events=%d want=%d", response.Code, len(sessions.events), want)
				}
			})
		}
	}
}

// TestSessionSidecarRejectsUnknownAndAmbiguousFormats keeps malformed data
// transparent without turning a successful upstream response into local proof.
func TestSessionSidecarRejectsUnknownAndAmbiguousFormats(t *testing.T) {
	for _, body := range []string{
		`ItemId=item-1&MediaSourceId=source-1&PlaySessionId=session-1`,
		`<PlaybackProgressInfo><ItemId>item-1</ItemId></PlaybackProgressInfo>`,
		`{"ItemId":"item-1","itemid":"item-1","MediaSourceId":"source-1","PlaySessionId":"session-1"}`,
		`{"ItemId":"item-1","MediaSourceId":"source-1","PlaySessionId":"session-1"} {}`,
	} {
		sessions := &fakePlaybackSessionService{}
		gateway := newVideoTestGateway(t, "http://emby.invalid", &fakeTokenService{principal: fixturePrincipal()}, &fakeDirectPlayService{}, roundTripFunc(func(r *http.Request) (*http.Response, error) {
			data, _ := io.ReadAll(r.Body)
			if string(data) != body {
				t.Fatal("invalid body was rewritten")
			}
			return transferIntentHTTPResponse(r, 204, ""), nil
		}), &bytes.Buffer{})
		gateway.playbackSessionService = sessions
		r := httptest.NewRequest("POST", "/Sessions/Playing", strings.NewReader(body))
		r.Header.Set(accessTokenHeader, fixtureAccessToken)
		r.Header.Set("Content-Type", "text/plain")
		response := httptest.NewRecorder()
		gateway.ServeHTTP(response, r)
		if response.Code != 204 || len(sessions.events) != 0 {
			t.Fatal("unsupported body became local activity")
		}
	}
}
