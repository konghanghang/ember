package playbackgateway

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// TestGatewayApplicationHeaderParseDiagnostics pins safe failure reasons at
// the HTTP boundary without relaxing token checks or contacting an upstream.
func TestGatewayApplicationHeaderParseDiagnostics(t *testing.T) {
	tests := []struct {
		name, header, reason, field, character string
		offset                                 int
	}{
		{name: "empty header", header: "", reason: "empty_header", field: "none", character: "end", offset: 0},
		{name: "unquoted token", header: `Emby Token=private-client-value`, reason: "expected_quote", field: "token", character: "other", offset: 11},
		{name: "single quoted token", header: `Emby Token='private-client-value'`, reason: "expected_quote", field: "token", character: "single_quote", offset: 11},
		{name: "percent encoded token quote", header: `Emby Token=%22private-client-value%22`, reason: "expected_quote", field: "token", character: "percent", offset: 11},
		{name: "invalid unquoted metadata", header: `Emby Client=/private-client-value`, reason: "expected_value", field: "client", character: "other", offset: 12},
		{name: "unclosed quote", header: `Emby Client="private-client-value`, reason: "unterminated_quote", field: "client", character: "end", offset: 33},
		{name: "invalid escape", header: `Emby Client="private\q"`, reason: "invalid_escape", field: "client", character: "other", offset: 21},
		{name: "control in value", header: "Emby Client=\"private\tvalue\"", reason: "control_character", field: "client", character: "tab", offset: 20},
		{name: "missing equals", header: `Emby Client "private"`, reason: "expected_equals", field: "client", character: "double_quote", offset: 12},
		{name: "wrong separator", header: `Emby Client="private"; Token="secret"`, reason: "expected_comma", field: "client", character: "semicolon", offset: 21},
		{name: "trailing comma", header: `Emby Client="private",`, reason: "expected_field", field: "none", character: "end", offset: 22},
		{name: "empty field", header: `Emby ="private"`, reason: "invalid_field", field: "none", character: "equals", offset: 5},
		{name: "duplicate token", header: `Emby Token="first", token="second"`, reason: "duplicate_field", field: "token", character: "other", offset: 20},
		{name: "token whitespace", header: `Emby Token=" secret "`, reason: "invalid_value", field: "token", character: "double_quote", offset: 11},
		{name: "private extension name", header: `Emby private-attribute="hidden`, reason: "unterminated_quote", field: "other", character: "end", offset: 30},
		{name: "unsupported scheme", header: `Other private-client-value`, reason: "unsupported_scheme", field: "none", character: "other", offset: 0},
		{name: "oversized header", header: strings.Repeat("x", maxApplicationAuthorizationSize+1), reason: "header_too_long", field: "none", character: "unavailable", offset: -1},
		{name: "invalid UTF8", header: "Emby Client=\"\xff\"", reason: "invalid_utf8", field: "none", character: "unavailable", offset: -1},
		{name: "line break", header: "Emby Client=\"a\nvalue\"", reason: "header_line_break", field: "none", character: "control", offset: 14},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var logs bytes.Buffer
			store := &fakeTokenService{principal: fixturePrincipal()}
			gateway := newTestGateway(t, "http://upstream.invalid", store, &logs)
			gateway.proxy.Transport = roundTripFunc(func(*http.Request) (*http.Response, error) {
				t.Fatal("malformed protected request reached upstream")
				return nil, nil
			})
			request := httptest.NewRequest(http.MethodGet, "/emby/System/Info", nil)
			request.Header.Set(accessTokenHeader, fixtureAccessToken)
			request.Header.Set(embyAuthorizationHeader, test.header)
			response := httptest.NewRecorder()
			gateway.ServeHTTP(response, request)
			_, resolved := store.snapshot()
			if response.Code != http.StatusUnauthorized || len(resolved) != 0 {
				t.Fatal("diagnostics changed the existing local rejection")
			}
			output := logs.String()
			for _, want := range []string{
				"code=application_header_parse_failed", "route=protected", "statusCode=401",
				"reasonCode=" + test.reason, "field=" + test.field,
				"offset=" + strconv.Itoa(test.offset), "characterClass=" + test.character,
			} {
				if !strings.Contains(output, want) {
					t.Fatalf("missing diagnostic %s", want)
				}
			}
			ids := regexp.MustCompile(`requestId=([a-f0-9]+-[a-f0-9]+)`).FindAllStringSubmatch(output, -1)
			if len(ids) != 2 || ids[0][1] != ids[1][1] || strings.Count(output, "code=application_header_parse_failed") != 1 {
				t.Fatal("parse diagnostic must correlate with exactly one request completion")
			}
			assertSecretsAbsent(t, output, fixtureAccessToken, test.header, "private-client-value", "private-attribute", "hidden", "first", "second", "secret")
		})
	}
}

// TestGatewayApplicationHeaderDiagnosticsPreserveLoginAndHonorLogLevel checks
// that diagnostic parsing neither changes login traffic nor logs at Info.
func TestGatewayApplicationHeaderDiagnosticsPreserveLoginAndHonorLogLevel(t *testing.T) {
	for _, debug := range []bool{false, true} {
		t.Run(strconv.FormatBool(debug), func(t *testing.T) {
			const header = `Emby Client="private-client-value`
			const requestBody = `{"Username":"fixture-user","Pw":"fixture-password"}`
			responseBody := `{"User":{"Id":"emby-user-1"},"ServerId":"server-1","AccessToken":"` + fixtureAccessToken + `"}`
			var logs bytes.Buffer
			store := &fakeTokenService{}
			gateway := newTestGateway(t, "http://upstream.invalid", store, &logs)
			gateway.debugEnabled = func() bool { return debug }
			calls := 0
			gateway.proxy.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
				calls++
				body, err := io.ReadAll(r.Body)
				if err != nil || string(body) != requestBody || r.Header.Get(embyAuthorizationHeader) != header {
					t.Fatal("diagnostics changed the login request")
				}
				return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(responseBody)), Request: r}, nil
			})
			request := httptest.NewRequest(http.MethodPost, authenticationPath, strings.NewReader(requestBody))
			request.Header.Set(embyAuthorizationHeader, header)
			response := httptest.NewRecorder()
			gateway.ServeHTTP(response, request)
			records, _ := store.snapshot()
			if calls != 1 || response.Code != http.StatusOK || response.Body.String() != responseBody || len(records) != 1 {
				t.Fatal("diagnostics changed upstream login or mapping behavior")
			}
			output := logs.String()
			if strings.Contains(output, "code=application_header_parse_failed") != debug {
				t.Fatal("parse diagnostics did not respect Debug level")
			}
			if debug && (!strings.Contains(output, "route=authentication") || !strings.Contains(output, "statusCode=200")) {
				t.Fatal("audit parse failure was not distinguished from a failed login")
			}
			assertSecretsAbsent(t, output, header, fixtureAccessToken, "fixture-password", requestBody, responseBody)
		})
	}
}
