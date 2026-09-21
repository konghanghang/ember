package playbackgateway

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/konghang/ember/backend/internal/services/embytoken"
)

// TestGatewayLoginDefersRequestValidationToEmby locks the upstream authority
// boundary without claiming that Emby accepts every client request shape.
func TestGatewayLoginDefersRequestValidationToEmby(t *testing.T) {
	shapes := []struct {
		name, query, authorization string
	}{
		{name: "missing metadata"},
		{name: "standard MediaBrowser", authorization: fixtureMediaBrowserAuthorization},
		{name: "partial metadata", authorization: `Emby Client="fixture-client"`},
		{name: "unknown metadata", authorization: fixtureApplicationAuthorization + `, Extension="fixture-extension"`},
		{name: "opaque grammar", authorization: `other fixture-opaque-header`},
		{name: "header and query", authorization: fixtureApplicationAuthorization, query: "?X-Emby-Client=fixture-client"},
		{name: "old token", authorization: mediaBrowserAuthorizationWithToken("fixture-old-token"), query: "?api_key=fixture-old-query-token"},
	}
	for _, shape := range shapes {
		for _, status := range []int{200, 400, 401, 403, 500} {
			t.Run(shape.name+"/"+http.StatusText(status), func(t *testing.T) {
				requestBody := `{"Username":"fixture-user","Pw":"fixture-password"}`
				responseBody := `{"User":{"Id":"emby-user-1"},"ServerId":"server-1","AccessToken":"` + fixtureAccessToken + `","SessionInfo":{"DeviceId":"server-device","Client":"server-client"},"Unknown":[]}`
				calls := 0
				upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					calls++
					body, _ := io.ReadAll(r.Body)
					if r.RequestURI != authenticationPath+shape.query || string(body) != requestBody || r.Header.Get("Authorization") != shape.authorization {
						t.Error("login request was changed")
					}
					w.Header().Set("Content-Type", "application/json")
					w.Header().Set("X-Upstream", "preserved")
					w.WriteHeader(status)
					_, _ = io.WriteString(w, responseBody)
				}))
				defer upstream.Close()
				store := &fakeTokenService{}
				var logs bytes.Buffer
				gateway := newTestGateway(t, upstream.URL, store, &logs)
				r := httptest.NewRequest(http.MethodPost, authenticationPath+shape.query, strings.NewReader(requestBody))
				if shape.authorization != "" {
					r.Header.Set("Authorization", shape.authorization)
				}
				w := httptest.NewRecorder()
				gateway.ServeHTTP(w, r)
				if calls != 1 || w.Code != status || w.Body.String() != responseBody || w.Header().Get("X-Upstream") != "preserved" {
					t.Fatalf("upstream calls=%d status=%d, want %d with unchanged response", calls, w.Code, status)
				}
				records, resolved := store.snapshot()
				if len(resolved) != 0 {
					t.Fatal("login used a request token for local authentication")
				}
				if status == 200 {
					if len(records) != 1 || records[0].AccessToken != fixtureAccessToken || records[0].DeviceID != "server-device" || records[0].ClientName != "server-client" {
						t.Fatal("mapping did not use successful response identity and session metadata")
					}
				} else if len(records) != 0 {
					t.Fatal("failed login created a mapping")
				}
				assertSecretsAbsent(t, logs.String(), fixtureAccessToken, "fixture-password", "fixture-old-token", "fixture-old-query-token", "fixture-opaque-header")
			})
		}
	}
}

// TestGatewayXMLLoginPreservesBytesAndMapsToken covers official response media
// negotiation, compressed sidecars and the subsequent protected request.
func TestGatewayXMLLoginPreservesBytesAndMapsToken(t *testing.T) {
	xmlBody := []byte(`<?xml version="1.0"?><AuthenticationResult xmlns="http://schemas.datacontract.org/2004/07/MediaBrowser.Controller.Authentication"><User><Id>emby-user-1</Id></User><AccessToken>` + fixtureAccessToken + `</AccessToken><ServerId>server-1</ServerId><SessionInfo><DeviceId>xml-device</DeviceId><Client>xml-client</Client></SessionInfo><Unknown /></AuthenticationResult>`)
	for _, encoding := range []string{"identity", "gzip", "deflate", "br"} {
		t.Run(encoding, func(t *testing.T) {
			body := xmlBody
			switch encoding {
			case "gzip":
				body = gzipFixture(t, body)
			case "deflate":
				body = deflateFixture(t, body, false)
			case "br":
				body = brotliFixture(t, body)
			}
			calls := 0
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				if r.URL.Path != authenticationPath {
					w.WriteHeader(http.StatusNoContent)
					return
				}
				if r.Header.Get("Accept") != "application/xml" {
					t.Error("Accept was changed")
				}
				w.Header().Set("Content-Type", "application/xml; charset=utf-8")
				w.Header().Set("Content-Encoding", encoding)
				_, _ = w.Write(body)
			}))
			defer upstream.Close()
			store := &fakeTokenService{principal: fixturePrincipal()}
			var logs bytes.Buffer
			gateway := newTestGateway(t, upstream.URL, store, &logs)
			r := newAuthenticationRequest(`<AuthenticateUserByName><Username>fixture-user</Username><Pw>fixture-password</Pw></AuthenticateUserByName>`)
			r.Header.Set("Content-Type", "application/xml")
			r.Header.Set("Accept", "application/xml")
			r.Header.Set("Accept-Encoding", encoding)
			w := httptest.NewRecorder()
			gateway.ServeHTTP(w, r)
			if w.Code != 200 || !bytes.Equal(w.Body.Bytes(), body) || w.Header().Get("Content-Encoding") != encoding {
				t.Fatal("XML response changed")
			}
			records, _ := store.snapshot()
			if len(records) != 1 || records[0].AccessToken != fixtureAccessToken || records[0].DeviceID != "xml-device" || records[0].ClientName != "xml-client" {
				t.Fatal("XML mapping missing or incorrect")
			}
			r = httptest.NewRequest(http.MethodGet, "/emby/Users/Me", nil)
			r.Header.Set(accessTokenHeader, records[0].AccessToken)
			r.Header.Set("Authorization", `Emby Client="xml-client", Extension="optional"`)
			w = httptest.NewRecorder()
			gateway.ServeHTTP(w, r)
			_, resolved := store.snapshot()
			if w.Code != 204 || calls != 2 || !reflect.DeepEqual(resolved, []string{fixtureAccessToken}) {
				t.Fatal("protected request failed after XML mapping")
			}
			assertSecretsAbsent(t, logs.String(), fixtureAccessToken, "fixture-password")
		})
	}
}

// TestProtectedTokenIgnoresNonIdentityMetadata keeps token conflict checks
// while decoupling audit metadata completeness from authentication.
func TestProtectedTokenIgnoresNonIdentityMetadata(t *testing.T) {
	for _, value := range []string{
		`Emby Client="fixture"`,
		`Emby Client="", Device="` + strings.Repeat("x", 200) + `", Extension="optional"`,
		`MediaBrowser Token="` + fixtureAccessToken + `"`,
		`Emby Extension="comma, Token=not-an-identity", Token="` + fixtureAccessToken + `"`,
	} {
		r := httptest.NewRequest(http.MethodGet, "/emby/Users/Me", nil)
		r.Header.Set(accessTokenHeader, fixtureAccessToken)
		r.Header.Set("Authorization", value)
		token, reason, ok := extractProtectedRequestAccessToken(r)
		if !ok || reason != "" || token != fixtureAccessToken {
			t.Fatalf("metadata rejected: reason=%s", reason)
		}
	}
}

// TestGatewayPublicBootstrapWithoutMetadata delegates the official pre-login
// endpoints to Emby while preserving mapped-token checks when a token exists.
func TestGatewayPublicBootstrapWithoutMetadata(t *testing.T) {
	for _, target := range []string{"/emby/Users/Public", "/emby/Users/Public?X-Emby-Client=partial", "/emby/Users/user-1/Images/Primary"} {
		t.Run(target, func(t *testing.T) {
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.RequestURI != target {
					t.Error("bootstrap request changed")
				}
				w.WriteHeader(403)
				_, _ = io.WriteString(w, "upstream-denied")
			}))
			defer upstream.Close()
			var logs bytes.Buffer
			store := &fakeTokenService{}
			gateway := newTestGateway(t, upstream.URL, store, &logs)
			w := httptest.NewRecorder()
			gateway.ServeHTTP(w, httptest.NewRequest(http.MethodGet, target, nil))
			if w.Code != 403 || w.Body.String() != "upstream-denied" {
				t.Fatal("bootstrap decision did not come from Emby")
			}
			records, resolved := store.snapshot()
			if len(records) != 0 || len(resolved) != 0 {
				t.Fatal("anonymous bootstrap used token store")
			}
		})
	}
}

// TestAuthenticationSidecarRejectsInvalidXMLWithoutChangingResponse verifies
// malformed and oversized XML cannot create a mapping or leak response data.
func TestAuthenticationSidecarRejectsInvalidXMLWithoutChangingResponse(t *testing.T) {
	valid := `<AuthenticationResult><User><Id>emby-user-1</Id></User><AccessToken>` + fixtureAccessToken + `</AccessToken><ServerId>server-1</ServerId></AuthenticationResult>`
	for _, test := range []struct {
		name, body string
		limit      int64
	}{
		{name: "malformed", body: `<AuthenticationResult><User>`},
		{name: "wrong root", body: strings.ReplaceAll(valid, "AuthenticationResult", "Other")},
		{name: "trailing root", body: valid + valid},
		{name: "trailing data", body: valid + "invalid"},
		{name: "missing identity", body: `<AuthenticationResult><ServerId>server-1</ServerId></AuthenticationResult>`},
		{name: "external entity", body: `<!DOCTYPE AuthenticationResult [<!ENTITY external SYSTEM "https://must-not-be-requested.invalid/">]><AuthenticationResult><AccessToken>&external;</AccessToken></AuthenticationResult>`},
		{name: "bounded sidecar", body: valid, limit: 32},
	} {
		t.Run(test.name, func(t *testing.T) {
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/xml")
				_, _ = io.WriteString(w, test.body)
			}))
			defer upstream.Close()
			var logs bytes.Buffer
			store := &fakeTokenService{}
			gateway := newTestGateway(t, upstream.URL, store, &logs)
			if test.limit > 0 {
				gateway.maxAuthenticationResponseBytes = test.limit
			}
			w := httptest.NewRecorder()
			gateway.ServeHTTP(w, newAuthenticationRequest("{}"))
			records, _ := store.snapshot()
			if w.Code != 200 || w.Body.String() != test.body || len(records) != 0 {
				t.Fatal("invalid XML changed upstream response or created a mapping")
			}
			assertSecretsAbsent(t, logs.String(), fixtureAccessToken, test.body)
		})
	}
}

// TestAuthenticationMetadataCannotInvalidateIdentity exercises optional session
// fields independently of the response identity and bounded request fallback.
func TestAuthenticationMetadataCannotInvalidateIdentity(t *testing.T) {
	for _, test := range []struct {
		name, device, client string
		want                 AuthenticationMetadata
	}{
		{name: "absent session", want: AuthenticationMetadata{DeviceID: "fallback-device", ClientName: "fallback-client"}},
		{name: "server session", device: "server-device", client: "server-client", want: AuthenticationMetadata{DeviceID: "server-device", ClientName: "server-client"}},
		{name: "invalid device", device: strings.Repeat("x", maxApplicationDeviceIDSize+1), client: "server-client", want: AuthenticationMetadata{DeviceID: "fallback-device", ClientName: "server-client"}},
		{name: "invalid client", device: "server-device", client: "line\nbreak", want: AuthenticationMetadata{DeviceID: "server-device", ClientName: "fallback-client"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			var result authenticationResult
			result.SessionInfo.DeviceID, result.SessionInfo.Client = test.device, test.client
			got := authenticationResultMetadata(result, AuthenticationMetadata{DeviceID: "fallback-device", ClientName: "fallback-client"})
			if got != test.want {
				t.Fatal("unexpected bounded metadata selection")
			}
		})
	}
}

// TestProtectedIdentityChecksSurviveMetadataRelaxation ensures the same
// rejected mapping remains rejected for both ordinary and public API routes.
func TestProtectedIdentityChecksSurviveMetadataRelaxation(t *testing.T) {
	for _, target := range []string{"/emby/Users/Me", "/emby/Users/Public", "/emby/Users/user-1/Images/Primary"} {
		t.Run(target, func(t *testing.T) {
			calls := 0
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { calls++; w.WriteHeader(200) }))
			defer upstream.Close()
			store := &fakeTokenService{resolveErr: embytoken.ErrTokenNotFound}
			var logs bytes.Buffer
			gateway := newTestGateway(t, upstream.URL, store, &logs)
			r := httptest.NewRequest(http.MethodGet, target, nil)
			r.Header.Set(accessTokenHeader, fixtureAccessToken)
			r.Header.Set("Authorization", `Emby Extension="fixture"`)
			w := httptest.NewRecorder()
			gateway.ServeHTTP(w, r)
			if w.Code != 401 || calls != 0 {
				t.Fatal("unmapped identity bypassed local gate")
			}
			assertSecretsAbsent(t, logs.String(), fixtureAccessToken)
		})
	}
}

// TestProtectedTokenConflictCannotHideInOptionalMetadata checks mixed-case
// duplicate fields, escaped metadata and conflicts with a direct Token.
func TestProtectedTokenConflictCannotHideInOptionalMetadata(t *testing.T) {
	for _, value := range []string{
		`Emby Token="other", Extension="optional"`,
		`Emby Token="` + fixtureAccessToken + `", token="other"`,
		`Emby Token="` + fixtureAccessToken + `", Token="` + fixtureAccessToken + `"`,
		`Emby Client="unterminated`,
		`Emby Client="fixture", Token="other",`,
	} {
		r := httptest.NewRequest(http.MethodGet, "/emby/Users/Me", nil)
		r.Header.Set(accessTokenHeader, fixtureAccessToken)
		r.Header.Set("Authorization", value)
		if _, _, ok := extractProtectedRequestAccessToken(r); ok {
			t.Fatal("ambiguous or invalid identity accepted")
		}
	}
}

// TestAuthenticationJSONSidecarKeepsLegacyContentTypes prevents XML support
// from narrowing the previous JSON sidecar behavior on mislabeled responses.
func TestAuthenticationJSONSidecarKeepsLegacyContentTypes(t *testing.T) {
	body := []byte(`{"User":{"Id":"emby-user-1"},"AccessToken":"` + fixtureAccessToken + `","ServerId":"server-1"}`)
	for _, contentType := range []string{"", "application/json", "text/plain", "application/json; broken"} {
		result, err := decodeAuthenticationResult(body, contentType)
		if err != nil || !validAuthenticationResult(result) {
			t.Fatalf("legacy JSON rejected for content type %q", contentType)
		}
	}
}

// TestTokenDiagnosticsMatchRelaxedMetadataGrammar ensures safe diagnostics do
// not call an accepted Token unparseable merely because audit fields are absent.
func TestTokenDiagnosticsMatchRelaxedMetadataGrammar(t *testing.T) {
	header := make(http.Header)
	header.Set("Authorization", `MediaBrowser Extension="fixture", Token="`+fixtureAccessToken+`"`)
	scheme, state := applicationAuthorizationDiagnostics(header)
	if scheme != "media_browser" || state != "present" {
		t.Fatalf("scheme=%s state=%s", scheme, state)
	}
}
