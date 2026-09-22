package playbackgateway

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/konghang/ember/backend/internal/services/directplay"
)

// TestGatewayVideoIntentSkipKeepsUpstreamOutcome separates policy skips from
// Provider failures while preserving the real fallback response and severity.
func TestGatewayVideoIntentSkipKeepsUpstreamOutcome(t *testing.T) {
	for _, test := range []struct {
		name        string
		status      int
		err         error
		wantDirect  string
		wantResult  string
		wantLevel   string
		wantMessage string
	}{
		{"intent_full_response", http.StatusOK, directplay.ErrPlaybackIntentRequired, "skipped", "success", "info", "无起播许可，跳过新增转存；Emby回退成功"},
		{"intent_range_response", http.StatusPartialContent, directplay.ErrPlaybackIntentRequired, "skipped", "success", "info", "无起播许可，跳过新增转存；Emby回退成功"},
		{"intent_not_found", http.StatusNotFound, directplay.ErrPlaybackIntentRequired, "skipped", "failure", "warn", "无起播许可，跳过新增转存；Emby回退失败"},
		{"intent_upstream_failure", http.StatusBadGateway, directplay.ErrPlaybackIntentRequired, "skipped", "failure", "warn", "无起播许可，跳过新增转存；Emby回退失败"},
		{"provider_not_found", http.StatusNotFound, directplay.ErrProviderUnavailable, "failure", "failure", "warn", "115直链失败，Emby回退失败"},
	} {
		t.Run(test.name, func(t *testing.T) {
			var logs bytes.Buffer
			const responseBody = "fixture upstream video response"
			transport := roundTripFunc(func(request *http.Request) (*http.Response, error) {
				if request.Method != http.MethodGet || request.URL.Path != "/emby/Videos/item-1/stream.mkv" || request.Header.Get("Range") != "bytes=0-" {
					t.Errorf("fallback changed method, path or Range")
				}
				return &http.Response{
					StatusCode: test.status, Request: request,
					Header: http.Header{"Content-Type": []string{"application/octet-stream"}},
					Body:   io.NopCloser(strings.NewReader(responseBody)),
				}, nil
			})
			gateway := newVideoTestGateway(t, "http://emby.invalid", &fakeTokenService{principal: fixturePrincipal()}, &fakeDirectPlayService{err: test.err}, transport, &logs)
			gateway.proofs.Record([]PlaybackProof{fixturePlaybackProof("mapping-1", "item-1", "source-1", "session-1")})
			request := newVideoRequest(http.MethodGet, "/emby/Videos/item-1/stream.mkv?MediaSourceId=source-1&PlaySessionId=session-1&Static=true")
			request.Header.Set("Range", "bytes=0-")
			response := httptest.NewRecorder()
			gateway.ServeHTTP(response, request)
			if response.Code != test.status || response.Body.String() != responseBody {
				t.Fatalf("fallback response changed: status=%d", response.Code)
			}
			assertSingleDecisionLog(t, logs.String(), "fallback", "direct_play", directPlayReasonCode(test.err))
			for _, want := range []string{
				"code=direct_play_fallback", "level=" + test.wantLevel,
				"directPlayResult=" + test.wantDirect, "fallbackResult=" + test.wantResult,
				"statusCode=" + strconv.Itoa(test.status), "upstreamStatus=" + strconv.Itoa(test.status),
				"message=" + strconv.Quote(test.wantMessage),
			} {
				if !strings.Contains(logs.String(), want) {
					t.Errorf("missing %s in log %s", want, logs.String())
				}
			}
			assertSecretsAbsent(t, logs.String(), fixtureAccessToken, responseBody, "emby.invalid")
		})
	}
}
