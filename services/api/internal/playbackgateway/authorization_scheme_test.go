package playbackgateway

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strings"
	"testing"
)

// TestGatewayApplicationAuthorizationSchemesAreCaseInsensitive exercises the
// protected HTTP entry point while preserving opaque Token and request bytes.
func TestGatewayApplicationAuthorizationSchemesAreCaseInsensitive(t *testing.T) {
	const token = "FiXtUrE-ToKeN+/AbC_123=.-~"
	const target = "/emby/Users/emby-user-1/Configuration?fixture=Keep%2BCase&fixture=TwO"
	const requestBody = `{"SubtitleMode":"Default","EnableNextEpisodeAutoPlay":true}`
	const responseBody = `{"fixture":"Upstream"}`
	schemes := []struct {
		value, code string
	}{
		{value: "Emby", code: "emby"},
		{value: "emby", code: "emby"},
		{value: "EMBY", code: "emby"},
		{value: "eMbY", code: "emby"},
		{value: "MediaBrowser", code: "media_browser"},
		{value: "mediabrowser", code: "media_browser"},
		{value: "MEDIABROWSER", code: "media_browser"},
		{value: "mEdIaBrOwSeR", code: "media_browser"},
	}
	for _, headerName := range []string{standardAuthorizationHeader, embyAuthorizationHeader, mediaBrowserAuthorizationHeader} {
		for _, scheme := range schemes {
			t.Run(headerName+"/"+scheme.value, func(t *testing.T) {
				authorization := scheme.value + ` Client="Fixture, MiXeD", DeviceId="Case-Sensitive-Device", Token="` + token + `"`
				request := httptest.NewRequest(http.MethodPost, target, strings.NewReader(requestBody))
				request.Header.Set(headerName, authorization)
				request.Header.Set("Content-Type", "application/json")
				request.Header.Set("User-Agent", "FixtureClient/1.0")
				request.Header.Set("X-Fixture", "KeepCase")
				originalHeader := request.Header.Clone()
				var logs bytes.Buffer
				store := &fakeTokenService{principal: fixturePrincipal()}
				gateway := newTestGateway(t, "http://upstream.invalid", store, &logs)
				calls := 0
				gateway.proxy.Transport = roundTripFunc(func(upstream *http.Request) (*http.Response, error) {
					calls++
					body, err := io.ReadAll(upstream.Body)
					if err != nil || upstream.Method != request.Method || upstream.URL.RequestURI() != target || string(body) != requestBody {
						t.Fatal("scheme parsing changed the upstream method, URL or body")
					}
					for name, values := range originalHeader {
						if !reflect.DeepEqual(upstream.Header.Values(name), values) {
							t.Fatalf("scheme parsing changed header %s", name)
						}
					}
					if applicationAuthorizationHeaderCount(upstream.Header) != 1 || upstream.Header.Get(accessTokenHeader) != "" || upstream.Header.Get(mediaBrowserTokenHeader) != "" {
						t.Fatal("scheme parsing added an authentication carrier")
					}
					return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(responseBody)), Request: upstream}, nil
				})
				response := httptest.NewRecorder()
				gateway.ServeHTTP(response, request)
				if response.Code != http.StatusOK || calls != 1 || response.Body.String() != responseBody {
					t.Fatalf("status=%d upstream calls=%d, want 200 and one unchanged response", response.Code, calls)
				}
				recorded, resolved := store.snapshot()
				if len(recorded) != 0 || !reflect.DeepEqual(resolved, []string{token}) {
					t.Fatal("scheme parsing changed the Token bytes used to resolve identity")
				}
				output := logs.String()
				if !strings.Contains(output, "applicationScheme="+scheme.code+" ") || !strings.Contains(output, "embeddedTokenState=present") || strings.Contains(output, "code=application_header_parse_failed") {
					t.Fatal("scheme diagnostics disagree with the accepted identity grammar")
				}
				assertSecretsAbsent(t, output, token, authorization, "Fixture, MiXeD", "Case-Sensitive-Device", requestBody, responseBody)
			})
		}
	}
}

// TestGatewayApplicationAuthorizationSchemeKeepsTokenCaseSensitive prevents
// scheme normalization from collapsing distinct Header or query identities.
func TestGatewayApplicationAuthorizationSchemeKeepsTokenCaseSensitive(t *testing.T) {
	const token = "FiXtUrE-ToKeN+/AbC_123=.-~"
	for _, scheme := range []string{"eMbY", "mEdIaBrOwSeR"} {
		for _, test := range []struct {
			name, directToken, queryToken string
			wantStatus                    int
		}{
			{name: "matching direct token", directToken: token, wantStatus: http.StatusOK},
			{name: "different direct token case", directToken: strings.ToLower(token), wantStatus: http.StatusUnauthorized},
			{name: "matching query token", queryToken: token, wantStatus: http.StatusOK},
			{name: "different query token case", queryToken: strings.ToLower(token), wantStatus: http.StatusUnauthorized},
		} {
			t.Run(scheme+"/"+test.name, func(t *testing.T) {
				var logs bytes.Buffer
				store := &fakeTokenService{principal: fixturePrincipal()}
				gateway := newTestGateway(t, "http://upstream.invalid", store, &logs)
				calls := 0
				gateway.proxy.Transport = roundTripFunc(func(request *http.Request) (*http.Response, error) {
					calls++
					return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: http.NoBody, Request: request}, nil
				})
				target := "/emby/System/Info"
				if test.queryToken != "" {
					target += "?api_key=" + url.QueryEscape(test.queryToken)
				}
				request := httptest.NewRequest(http.MethodGet, target, nil)
				request.Header.Set(embyAuthorizationHeader, scheme+` Token="`+token+`"`)
				if test.directToken != "" {
					request.Header.Set(accessTokenHeader, test.directToken)
				}
				response := httptest.NewRecorder()
				gateway.ServeHTTP(response, request)
				recorded, resolved := store.snapshot()
				if response.Code != test.wantStatus || len(recorded) != 0 {
					t.Fatalf("status=%d mappings=%d, want status=%d and no mappings", response.Code, len(recorded), test.wantStatus)
				}
				if test.wantStatus == http.StatusOK {
					if calls != 1 || !reflect.DeepEqual(resolved, []string{token}) {
						t.Fatal("matching carriers did not resolve the original Token exactly once")
					}
				} else if calls != 0 || len(resolved) != 0 || !strings.Contains(logs.String(), "reasonCode=token_ambiguous") {
					t.Fatal("different Token case was not rejected before identity lookup and proxying")
				}
				assertSecretsAbsent(t, logs.String(), token, strings.ToLower(token))
			})
		}
	}
}

// TestGatewayApplicationAuthorizationSchemeRejectsInvalidHeaders keeps the
// scheme delimiter, field grammar and missing-identity boundaries fail-closed.
func TestGatewayApplicationAuthorizationSchemeRejectsInvalidHeaders(t *testing.T) {
	const token = "FiXtUrE-ToKeN+/AbC_123=.-~"
	for _, test := range []struct {
		name, header, reason string
		withoutDirectToken   bool
	}{
		{name: "unknown scheme", header: `Bearer ` + token},
		{name: "Emby scheme suffix", header: `embyExtra Token="` + token + `"`},
		{name: "MediaBrowser scheme suffix", header: `mediabrowserExtra Token="` + token + `"`},
		{name: "missing Emby separator", header: `embyToken="` + token + `"`},
		{name: "missing MediaBrowser separator", header: `mediabrowserToken="` + token + `"`},
		{name: "tab separator", header: "eMbY\tToken=\"" + token + "\""},
		{name: "truncated scheme", header: "eM"},
		{name: "bare scheme", header: "mEdIaBrOwSeR"},
		{name: "empty fields", header: "eMbY "},
		{name: "unquoted Token", header: "eMbY Token=" + token},
		{name: "unterminated Token", header: `mEdIaBrOwSeR Token="` + token},
		{name: "invalid escape", header: `eMbY Token="` + token + `\q"`},
		{name: "line break", header: "eMbY Token=\"" + token + "\"\r\n"},
		{name: "duplicate logical Token", header: `eMbY Token="` + token + `", tOkEn="` + token + `"`},
		{name: "missing Token", header: `eMbY Client="FixtureClient"`, reason: "token_missing", withoutDirectToken: true},
		{name: "empty Token", header: `mEdIaBrOwSeR Token=""`, reason: "token_missing", withoutDirectToken: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			var logs bytes.Buffer
			store := &fakeTokenService{principal: fixturePrincipal()}
			gateway := newTestGateway(t, "http://upstream.invalid", store, &logs)
			gateway.proxy.Transport = roundTripFunc(func(*http.Request) (*http.Response, error) {
				t.Fatal("invalid application authorization reached the upstream")
				return nil, nil
			})
			request := httptest.NewRequest(http.MethodGet, "/emby/System/Info", nil)
			request.Header.Set(standardAuthorizationHeader, test.header)
			if !test.withoutDirectToken {
				request.Header.Set(accessTokenHeader, token)
			}
			response := httptest.NewRecorder()
			gateway.ServeHTTP(response, request)
			recorded, resolved := store.snapshot()
			if response.Code != http.StatusUnauthorized || response.Body.Len() != 0 || len(recorded) != 0 || len(resolved) != 0 {
				t.Fatalf("status=%d mappings=%d resolutions=%d, want an empty 401 before identity lookup", response.Code, len(recorded), len(resolved))
			}
			wantReason := test.reason
			if wantReason == "" {
				wantReason = "token_invalid"
			}
			if !strings.Contains(logs.String(), "reasonCode="+wantReason) {
				t.Fatalf("missing rejection reason %s", wantReason)
			}
			assertSecretsAbsent(t, logs.String(), token, "FixtureClient")
		})
	}
}
