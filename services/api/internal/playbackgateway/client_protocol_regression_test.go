package playbackgateway

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/konghang/ember/backend/internal/services/directplay"
	"github.com/konghang/ember/backend/internal/services/embytoken"
)

// These synthetic fixtures use the Emby 4.9.3.0 OpenAPI contract pinned at
// Emby.SDK 6ee0155063bc85578196489926359a8f37419502. No test opens a listener,
// contacts an Emby/115 instance, or establishes real client compatibility.

// TestGatewayClientProtocolPlaybackInfoKeepsBytesAndSourceIdentity exercises
// response observation and video selection together: array order and the
// control-request UA must not replace the requested source or video UA.
func TestGatewayClientProtocolPlaybackInfoKeepsBytesAndSourceIdentity(t *testing.T) {
	const requestBody = `{
  "UserId": "emby-user-1", "IsPlayback": true,
  "MediaSourceId": "source-selected", "AudioStreamIndex": 2, "SubtitleStreamIndex": 3,
  "DeviceProfile": {"CodecProfiles": [], "SubtitleProfiles": []},
  "FutureRequestField": {"opaque": [1, null, "keep"]}
}`
	for _, test := range []struct {
		name    string
		sources []string
	}{
		{name: "selected source is second", sources: []string{clientProtocolOtherSource, clientProtocolSelectedSource}},
		{name: "source order reversed", sources: []string{clientProtocolSelectedSource, clientProtocolOtherSource}},
	} {
		t.Run(test.name, func(t *testing.T) {
			responseBody := "{\n  \"PlaySessionId\": \"session-1\",\n  \"MediaSources\": [\n" + strings.Join(test.sources, ",\n") + "\n  ],\n  \"FutureResponseField\": {\"empty\": [], \"escaped\": \"\\u4e2d\\u6587\", \"nullable\": null}\n}\n"
			calls := 0
			directPlay := &fakeDirectPlayService{result: validFixtureRedirectCandidate()}
			tokens := &fakeTokenService{principal: fixturePrincipal()}
			gateway := newVideoTestGateway(t, "http://emby.invalid", tokens, directPlay,
				roundTripFunc(func(request *http.Request) (*http.Response, error) {
					calls++
					if request.Method != http.MethodPost || request.URL.RequestURI() != "/emby/Items/item-1/PlaybackInfo?fixture=one%2Ftwo&fixture=three" {
						t.Fatalf("unexpected upstream request: %s %s", request.Method, request.URL.RequestURI())
					}
					body, err := io.ReadAll(request.Body)
					if err != nil || string(body) != requestBody {
						t.Fatalf("PlaybackInfo request changed: body=%q err=%v", body, err)
					}
					for header, want := range map[string]string{
						accessTokenHeader: fixtureAccessToken,
						"User-Agent":      "Protocol-Control/A",
						"Accept-Encoding": "gzip, deflate, br",
						"Content-Type":    "application/json",
					} {
						if got := request.Header.Get(header); got != want {
							t.Fatalf("upstream %s=%q, want %q", header, got, want)
						}
					}
					response := transferIntentHTTPResponse(request, http.StatusOK, responseBody)
					response.Header.Set("Content-Type", "application/json; charset=utf-8")
					response.Header.Set("ETag", `"playback-info-fixture"`)
					response.Header.Set("Content-Length", strconv.Itoa(len(responseBody)))
					return response, nil
				}), &bytes.Buffer{})
			request := httptest.NewRequest(http.MethodPost, "/Items/item-1/PlaybackInfo?fixture=one%2Ftwo&fixture=three", strings.NewReader(requestBody))
			request.Header.Set(accessTokenHeader, fixtureAccessToken)
			request.Header.Set("User-Agent", "Protocol-Control/A")
			request.Header.Set("Accept-Encoding", "gzip, deflate, br")
			request.Header.Set("Content-Type", "application/json")
			response := httptest.NewRecorder()
			gateway.ServeHTTP(response, request)
			if response.Code != http.StatusOK || response.Body.String() != responseBody ||
				response.Header().Get("Content-Type") != "application/json; charset=utf-8" ||
				response.Header().Get("ETag") != `"playback-info-fixture"` ||
				response.Header().Get("Content-Length") != strconv.Itoa(len(responseBody)) {
				t.Fatalf("PlaybackInfo response changed: status=%d headers=%v body=%q", response.Code, response.Header(), response.Body.String())
			}
			if len(directPlay.snapshot()) != 0 {
				t.Fatal("PlaybackInfo observation called DirectPlay")
			}

			for index, userAgent := range []string{"Protocol-Media/B", "Protocol-Media/C"} {
				video := newVideoRequest(http.MethodGet, "/Videos/item-1/stream.mkv?MediaSourceId=source-selected&PlaySessionId=session-1&Static=true")
				video.Header.Set("User-Agent", userAgent)
				video.Header.Set("Range", "bytes=0-")
				videoResponse := httptest.NewRecorder()
				gateway.ServeHTTP(videoResponse, video)
				if videoResponse.Code != http.StatusFound || videoResponse.Header().Get("Location") != fixtureRedirectURL || videoResponse.Body.Len() != 0 {
					t.Fatalf("video UA %q: status=%d headers=%v", userAgent, videoResponse.Code, videoResponse.Header())
				}
				resolved := directPlay.snapshot()
				if len(resolved) != index+1 {
					t.Fatalf("DirectPlay calls=%d, want %d", len(resolved), index+1)
				}
				got := resolved[index]
				if got.Path != "/mnt/media/fixture-selected.mkv" || got.ClientUserAgent != userAgent || got.Method != http.MethodGet ||
					got.MappingID != "mapping-1" || got.UserID != "user-1" || got.DeviceID != "device-1" || got.PlaySessionID != "session-1" {
					t.Fatalf("video resolved the wrong source, UA or identity: %+v", got)
				}
			}
			recorded, resolved := tokens.snapshot()
			if calls != 1 || len(recorded) != 0 || !reflect.DeepEqual(resolved, []string{fixtureAccessToken, fixtureAccessToken, fixtureAccessToken}) {
				t.Fatalf("upstream=%d authentication records=%d principal resolutions=%d", calls, len(recorded), len(resolved))
			}
		})
	}
}

// TestGatewayClientProtocolVideoFallbackPreservesConditionalRanges uses the
// standard HTTP content handler as an in-memory upstream. HEAD transfer/lease
// side effects remain covered by transfer_trigger_test.go and DirectPlay tests.
func TestGatewayClientProtocolVideoFallbackPreservesConditionalRanges(t *testing.T) {
	const videoBytes = "\x00\x01client-protocol-video\xfe\xff"
	const entityTag = `"synthetic-video-v1"`
	// HTTP Last-Modified is a GMT wire value, independent of business timezone.
	modifiedAt := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)
	for _, test := range []struct {
		name, method, byteRange, ifRange string
		status                           int
		contentRange                     string
	}{
		{name: "matching validator returns partial bytes", method: http.MethodGet, byteRange: "bytes=2-7", ifRange: entityTag, status: http.StatusPartialContent, contentRange: "bytes 2-7/25"},
		{name: "stale validator returns full representation", method: http.MethodGet, byteRange: "bytes=2-7", ifRange: `"synthetic-video-v0"`, status: http.StatusOK},
		{name: "unsatisfiable range stays 416", method: http.MethodGet, byteRange: "bytes=999-", ifRange: entityTag, status: http.StatusRequestedRangeNotSatisfiable, contentRange: "bytes */25"},
		{name: "HEAD has metadata without a body", method: http.MethodHead, status: http.StatusOK},
	} {
		t.Run(test.name, func(t *testing.T) {
			const target = "/Videos/item-1/stream.mkv?MediaSourceId=source-1&PlaySessionId=session-1&Static=true&fixture=one%2Ftwo"
			calls := 0
			var upstream *httptest.ResponseRecorder
			var upstreamHeaders http.Header
			directPlay := &fakeDirectPlayService{err: directplay.ErrPathNotMapped}
			gateway := newVideoTestGateway(t, "http://emby.invalid", &fakeTokenService{principal: fixturePrincipal()}, directPlay,
				roundTripFunc(func(request *http.Request) (*http.Response, error) {
					calls++
					if request.Method != test.method || request.URL.RequestURI() != "/emby"+target ||
						request.Header.Get("Range") != test.byteRange || request.Header.Get("If-Range") != test.ifRange ||
						request.Header.Get("User-Agent") != "Protocol-Media/B" || request.Header.Get(accessTokenHeader) != fixtureAccessToken {
						t.Fatal("video fallback changed the method, URL, range, validator or identity headers")
					}
					upstream = httptest.NewRecorder()
					upstream.Header().Set("ETag", entityTag)
					upstream.Header().Set("Content-Type", "video/x-matroska")
					http.ServeContent(upstream, request, "fixture.mkv", modifiedAt, strings.NewReader(videoBytes))
					response := upstream.Result()
					response.Request = request
					upstreamHeaders = response.Header.Clone()
					return response, nil
				}), &bytes.Buffer{})
			gateway.proofs.Record([]PlaybackProof{fixturePlaybackProof("mapping-1", "item-1", "source-1", "session-1")})
			request := newVideoRequest(test.method, target)
			request.Header.Set("User-Agent", "Protocol-Media/B")
			if test.byteRange != "" {
				request.Header.Set("Range", test.byteRange)
				request.Header.Set("If-Range", test.ifRange)
			}
			response := httptest.NewRecorder()
			gateway.ServeHTTP(response, request)
			if calls != 1 || len(directPlay.snapshot()) != 1 {
				t.Fatalf("fallback calls: Emby=%d DirectPlay=%d", calls, len(directPlay.snapshot()))
			}
			if response.Code != test.status || response.Body.String() != upstream.Body.String() || response.Header().Get("Content-Range") != test.contentRange {
				t.Fatalf("conditional response changed: status=%d headers=%v body=%q", response.Code, response.Header(), response.Body.String())
			}
			for _, header := range []string{"ETag", "Last-Modified", "Accept-Ranges", "Content-Range", "Content-Type", "Content-Length"} {
				if !reflect.DeepEqual(response.Header().Values(header), upstreamHeaders.Values(header)) {
					t.Fatalf("response %s=%v, want %v", header, response.Header().Values(header), upstreamHeaders.Values(header))
				}
			}
			if test.method == http.MethodHead && response.Body.Len() != 0 {
				t.Fatal("HEAD forwarded a response body")
			}
		})
	}
}

// TestGatewayClientProtocolOtherMediaStaysProtectedProxy locks transport only.
// Audio/Sync are generic protected proxies; Download/Sync denial passthrough
// does not establish package download rights, sync eligibility or Emby policy.
func TestGatewayClientProtocolOtherMediaStaysProtectedProxy(t *testing.T) {
	for _, test := range []struct {
		name, method, target, requestBody, contentType, body string
		status                                               int
	}{
		{name: "external SRT", method: http.MethodGet, target: "/emby/Items/item-1/source-1/Subtitles/3/Stream.srt?StartPositionTicks=0&CopyTimestamps=false", status: http.StatusOK, contentType: "application/x-subrip", body: "1\r\n00:00:00,000 --> 00:00:01,000\r\n合成字幕\r\n"},
		{name: "VTT with start position", method: http.MethodGet, target: "/emby/Videos/item-1/source-1/Subtitles/3/0/Stream.vtt?CopyTimestamps=true", status: http.StatusOK, contentType: "text/vtt", body: "WEBVTT\n\n00:00.000 --> 00:01.000\nFixture subtitle\n"},
		{name: "embedded ASS", method: http.MethodGet, target: "/emby/Videos/item-1/source-1/Subtitles/4/Stream.ass?StartPositionTicks=0", status: http.StatusOK, contentType: "text/x-ssa", body: "[Script Info]\nScriptType: v4.00+\n[Events]\nFormat: Layer, Start, End, Style, Name, MarginL, MarginR, MarginV, Effect, Text\nDialogue: 0,0:00:00.00,0:00:01.00,Default,,0,0,0,,Fixture subtitle\n"},
		{name: "subtitle HEAD", method: http.MethodHead, target: "/emby/Items/item-1/source-1/Subtitles/3/0/Stream.vtt", status: http.StatusOK, contentType: "text/vtt"},
		{name: "HLS manifest", method: http.MethodGet, target: "/emby/Videos/item-1/master.m3u8?MediaSourceId=source-1&PlaySessionId=session-1&Static=true", status: http.StatusOK, contentType: "application/vnd.apple.mpegurl", body: "#EXTM3U\n#EXT-X-STREAM-INF:BANDWIDTH=800000\nmain.m3u8?MediaSourceId=source-1\n"},
		{name: "HLS segment", method: http.MethodGet, target: "/emby/Videos/item-1/hls1/playlist-1/0.ts?MediaSourceId=source-1&PlaySessionId=session-1&Static=true", status: http.StatusOK, contentType: "video/mp2t", body: "\x47\x40\x00\x10synthetic-segment"},
		{name: "audio stream", method: http.MethodGet, target: "/emby/Audio/item-1/stream.mp3?MediaSourceId=source-1&PlaySessionId=session-1&Static=true", status: http.StatusOK, contentType: "audio/mpeg", body: "ID3-synthetic-audio"},
		{name: "download upstream denial", method: http.MethodGet, target: "/emby/Items/item-1/Download", status: http.StatusForbidden, contentType: "application/json", body: `{"ErrorCode":"fixture_download_denied"}`},
		{name: "sync upstream denial", method: http.MethodPost, target: "/emby/Sync/Jobs", requestBody: `{ "ItemIds": ["item-1"], "TargetId": "fixture-target", "Name": "Fixture" }`, status: http.StatusForbidden, contentType: "application/json", body: `{"ErrorCode":"fixture_sync_denied"}`},
	} {
		t.Run(test.name, func(t *testing.T) {
			calls := 0
			directPlay := &fakeDirectPlayService{result: validFixtureRedirectCandidate()}
			tokens := &fakeTokenService{principal: fixturePrincipal()}
			gateway := newVideoTestGateway(t, "http://emby.invalid", tokens, directPlay,
				roundTripFunc(func(request *http.Request) (*http.Response, error) {
					calls++
					var body []byte
					var err error
					if request.Body != nil {
						body, err = io.ReadAll(request.Body)
					}
					if request.Method != test.method || request.URL.RequestURI() != test.target || err != nil || string(body) != test.requestBody ||
						request.Header.Get(accessTokenHeader) != fixtureAccessToken || request.Header.Get("User-Agent") != "Protocol-Media/B" ||
						request.Header.Get("X-Client-Fixture") != "preserved" {
						t.Fatal("protected proxy changed the method, URL, body or request headers")
					}
					return &http.Response{
						StatusCode: test.status, Request: request,
						Header: http.Header{"Content-Type": {test.contentType}, "Etag": {`"synthetic-media-v1"`}, "X-Upstream-Fixture": {"preserved"}},
						Body:   io.NopCloser(strings.NewReader(test.body)),
					}, nil
				}), &bytes.Buffer{})
			gateway.proofs.Record([]PlaybackProof{fixturePlaybackProof("mapping-1", "item-1", "source-1", "session-1")})
			request := httptest.NewRequest(test.method, test.target, strings.NewReader(test.requestBody))
			request.Header.Set(accessTokenHeader, fixtureAccessToken)
			request.Header.Set("User-Agent", "Protocol-Media/B")
			request.Header.Set("X-Client-Fixture", "preserved")
			if test.requestBody != "" {
				request.Header.Set("Content-Type", "application/json")
			}
			response := httptest.NewRecorder()
			gateway.ServeHTTP(response, request)
			if response.Code != test.status || response.Body.String() != test.body || response.Header().Get("Content-Type") != test.contentType ||
				response.Header().Get("ETag") != `"synthetic-media-v1"` || response.Header().Get("X-Upstream-Fixture") != "preserved" {
				t.Fatalf("proxy response changed: status=%d headers=%v body=%q", response.Code, response.Header(), response.Body.String())
			}
			recorded, resolved := tokens.snapshot()
			if calls != 1 || len(directPlay.snapshot()) != 0 || len(recorded) != 0 || !reflect.DeepEqual(resolved, []string{fixtureAccessToken}) {
				t.Fatalf("calls: upstream=%d DirectPlay=%d authentication=%d principal=%d", calls, len(directPlay.snapshot()), len(recorded), len(resolved))
			}
		})
	}
}

// TestGatewayClientProtocolMediaDoesNotBypassTokenGate prevents transport
// coverage from treating subtitles, audio, downloads or sync as anonymous APIs.
func TestGatewayClientProtocolMediaDoesNotBypassTokenGate(t *testing.T) {
	for _, test := range []struct {
		name, target string
		revoked      bool
	}{
		{name: "subtitle missing token", target: "/emby/Items/item-1/source-1/Subtitles/3/Stream.srt"},
		{name: "audio revoked token", target: "/emby/Audio/item-1/stream.mp3", revoked: true},
		{name: "download missing token", target: "/emby/Items/item-1/Download"},
		{name: "sync revoked token", target: "/emby/Sync/Jobs", revoked: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			calls := 0
			directPlay := &fakeDirectPlayService{result: validFixtureRedirectCandidate()}
			tokens := &fakeTokenService{principal: fixturePrincipal(), resolveErr: embytoken.ErrTokenRevoked}
			gateway := newVideoTestGateway(t, "http://emby.invalid", tokens, directPlay,
				roundTripFunc(func(request *http.Request) (*http.Response, error) {
					calls++
					return transferIntentHTTPResponse(request, http.StatusOK, "unexpected"), nil
				}), &bytes.Buffer{})
			request := httptest.NewRequest(http.MethodGet, test.target, nil)
			if test.revoked {
				request.Header.Set(accessTokenHeader, fixtureAccessToken)
			}
			response := httptest.NewRecorder()
			gateway.ServeHTTP(response, request)
			if response.Code != http.StatusUnauthorized || calls != 0 || len(directPlay.snapshot()) != 0 {
				t.Fatalf("identity denial bypassed: status=%d upstream=%d DirectPlay=%d", response.Code, calls, len(directPlay.snapshot()))
			}
		})
	}
}

const clientProtocolOtherSource = `    {
      "Id": "source-other", "ItemId": "item-1", "Protocol": "File",
      "Path": "/mnt/media/fixture-other.mkv", "Container": "mkv", "Size": 4096,
      "IsRemote": false, "SupportsDirectPlay": true, "SupportsDirectStream": true,
      "SupportsTranscoding": false, "DefaultAudioStreamIndex": null, "DefaultSubtitleStreamIndex": -1,
      "MediaStreams": [], "Formats": [], "RequiredHttpHeaders": {},
      "FutureSourceField": {"preserve": true}
    }`

const clientProtocolSelectedSource = `    {
      "Id": "source-selected", "ItemId": "item-1", "Protocol": "File",
      "Path": "/mnt/media/fixture-selected.mkv", "Container": "mkv", "Size": 8192,
      "IsRemote": false, "SupportsDirectPlay": true, "SupportsDirectStream": true,
      "SupportsTranscoding": true, "DefaultAudioStreamIndex": 2, "DefaultSubtitleStreamIndex": 3,
      "Formats": ["matroska"], "RequiredHttpHeaders": {},
      "MediaStreams": [
        {"Type": "Video", "Index": 0, "Codec": "h264", "FutureStreamField": {"preserve": []}},
        {"Type": "Audio", "Index": 1, "Codec": "aac", "Language": "eng", "IsDefault": false, "DisplayTitle": "English"},
        {"Type": "Audio", "Index": 2, "Codec": "ac3", "Language": "chi", "IsDefault": true, "DisplayTitle": "中文"},
        {"Type": "Subtitle", "Index": 3, "Codec": "subrip", "Language": "chi", "IsDefault": true, "IsForced": false,
         "IsExternal": true, "IsTextSubtitleStream": true, "DeliveryMethod": "External",
         "DeliveryUrl": "/emby/Items/item-1/source-selected/Subtitles/3/Stream.srt", "Path": "/mnt/media/fixture-selected.zh.srt"},
        {"Type": "Subtitle", "Index": 4, "Codec": "ass", "Language": "eng", "IsDefault": false, "IsForced": true,
         "IsExternal": false, "IsTextSubtitleStream": true, "DeliveryMethod": "Embed"}
      ],
      "FutureSourceField": {"preserve": "second-source", "empty": []}
    }`
