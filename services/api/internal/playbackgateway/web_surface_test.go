package playbackgateway

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// assertWebSurfaceDisabledResponse locks the user-visible GET page and the
// equivalent bodyless HEAD contract shared by every disabled Web Surface.
func assertWebSurfaceDisabledResponse(t *testing.T, response *httptest.ResponseRecorder, method string) {
	t.Helper()
	if response.Code != http.StatusNotFound {
		t.Fatalf("disabled response=%d body=%q", response.Code, response.Body.String())
	}
	if contentType := response.Header().Get("Content-Type"); contentType != "text/html; charset=utf-8" {
		t.Fatalf("disabled Content-Type=%q", contentType)
	}
	if cacheControl := response.Header().Get("Cache-Control"); cacheControl != "no-store" {
		t.Fatalf("disabled Cache-Control=%q", cacheControl)
	}
	if contentLength := response.Header().Get("Content-Length"); contentLength != fmt.Sprint(len(webSurfaceDisabledPage)) {
		t.Fatalf("disabled Content-Length=%q", contentLength)
	}
	if method == http.MethodHead {
		if response.Body.Len() != 0 {
			t.Fatalf("disabled HEAD body=%q", response.Body.String())
		}
		return
	}
	for _, expected := range []string{"<html lang=\"zh-CN\">", "Emby 网页访问已关闭", "请使用受支持的 Emby 客户端，或联系管理员"} {
		if !strings.Contains(response.Body.String(), expected) {
			t.Fatalf("disabled body=%q, want %q", response.Body.String(), expected)
		}
	}
}

func TestGatewayWebSurfaceFollowsPolicyAtRequestBoundary(t *testing.T) {
	var upstreamMu sync.Mutex
	upstreamRequests := make([]string, 0, 4)
	upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		upstreamMu.Lock()
		upstreamRequests = append(upstreamRequests, request.Method+" "+request.URL.RequestURI())
		upstreamMu.Unlock()
		if request.URL.Path == "/" {
			writer.Header().Set("Location", "/web/index.html")
			writer.WriteHeader(http.StatusFound)
			return
		}
		writer.Header().Set("X-Upstream", "preserved")
		writer.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(writer, "fixture-web")
	}))
	defer upstream.Close()

	policy := &fakeWebSurfacePolicy{enabled: true}
	var logs bytes.Buffer
	gateway := newWebSurfaceTestGateway(t, upstream.URL, policy, &logs)

	root := httptest.NewRecorder()
	gateway.ServeHTTP(root, httptest.NewRequest(http.MethodGet, "/?client=browser", nil))
	if root.Code != http.StatusFound || root.Header().Get("Location") != "/web/index.html" {
		t.Fatalf("root response=%d headers=%v", root.Code, root.Header())
	}

	asset := httptest.NewRecorder()
	gateway.ServeHTTP(asset, httptest.NewRequest(http.MethodGet, "/web/index.html?build=fixture", nil))
	if asset.Code != http.StatusOK || asset.Body.String() != "fixture-web" || asset.Header().Get("X-Upstream") != "preserved" {
		t.Fatalf("asset response=%d headers=%v body=%q", asset.Code, asset.Header(), asset.Body.String())
	}

	policy.set(false, nil)
	disabled := httptest.NewRecorder()
	gateway.ServeHTTP(disabled, httptest.NewRequest(http.MethodGet, "/web/app.js", nil))
	assertWebSurfaceDisabledResponse(t, disabled, http.MethodGet)

	disabledHead := httptest.NewRecorder()
	gateway.ServeHTTP(disabledHead, httptest.NewRequest(http.MethodHead, "/web/app.js", nil))
	assertWebSurfaceDisabledResponse(t, disabledHead, http.MethodHead)

	policy.set(true, nil)
	reenabled := httptest.NewRecorder()
	gateway.ServeHTTP(reenabled, httptest.NewRequest(http.MethodHead, "/favicon.ico", nil))
	if reenabled.Code != http.StatusOK {
		t.Fatalf("re-enabled response=%d", reenabled.Code)
	}

	upstreamMu.Lock()
	requests := append([]string(nil), upstreamRequests...)
	upstreamMu.Unlock()
	wantRequests := []string{"GET /?client=browser", "GET /web/index.html?build=fixture", "HEAD /favicon.ico"}
	if len(requests) != len(wantRequests) {
		t.Fatalf("upstream requests=%#v, want %#v", requests, wantRequests)
	}
	for index := range wantRequests {
		if requests[index] != wantRequests[index] {
			t.Fatalf("upstream requests=%#v, want %#v", requests, wantRequests)
		}
	}
	if calls := policy.callCount(); calls != 5 {
		t.Fatalf("policy calls=%d, want one evaluation per Web request", calls)
	}
	for _, expected := range []string{"code=web_surface_disabled", `message="Emby网页访问已关闭"`, "route=emby_web"} {
		if !strings.Contains(logs.String(), expected) {
			t.Fatalf("logs=%q, want %s", logs.String(), expected)
		}
	}
}

func TestGatewayWebSurfaceProxiesExactLoginAssetsWithoutToken(t *testing.T) {
	tests := []struct {
		name   string
		target string
	}{
		{name: "locale resource", target: "/web/strings/zh-CN.json?v=fixture"},
		{name: "branding css", target: "/emby/Branding/Css.css?v=fixture"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var upstreamCalls atomic.Int32
			upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				upstreamCalls.Add(1)
				if request.Method != http.MethodGet || request.URL.RequestURI() != test.target {
					t.Fatalf("upstream request=%s %s, want GET %s", request.Method, request.URL.RequestURI(), test.target)
				}
				writer.WriteHeader(http.StatusOK)
				_, _ = io.WriteString(writer, "fixture-login-asset")
			}))
			defer upstream.Close()

			policy := &fakeWebSurfacePolicy{enabled: true}
			var logs bytes.Buffer
			gateway := newWebSurfaceTestGateway(t, upstream.URL, policy, &logs)
			response := httptest.NewRecorder()
			gateway.ServeHTTP(response, httptest.NewRequest(http.MethodGet, test.target, nil))

			if response.Code != http.StatusOK || response.Body.String() != "fixture-login-asset" || upstreamCalls.Load() != 1 {
				t.Fatalf("response=%d body=%q upstreamCalls=%d", response.Code, response.Body.String(), upstreamCalls.Load())
			}
			if calls := policy.callCount(); calls != 1 {
				t.Fatalf("Web policy calls=%d, want 1", calls)
			}
			if !strings.Contains(logs.String(), "route=emby_web") || strings.Contains(logs.String(), "code=token_header_invalid") {
				t.Fatalf("logs=%q", logs.String())
			}
		})
	}
}

func TestGatewayWebSurfaceProxiesBrandingConfigurationWithWebQueryMetadata(t *testing.T) {
	query := validWebApplicationQuery()
	target := "/emby/Branding/Configuration?" + query.Encode()
	var upstreamCalls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		upstreamCalls.Add(1)
		if request.Method != http.MethodGet || request.URL.RequestURI() != target {
			t.Fatalf("upstream request=%s %s, want GET %s", request.Method, request.URL.RequestURI(), target)
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(writer, `{"CustomCss":"fixture","LoginDisclaimer":"fixture"}`)
	}))
	defer upstream.Close()

	policy := &fakeWebSurfacePolicy{enabled: true}
	var logs bytes.Buffer
	gateway := newWebSurfaceTestGateway(t, upstream.URL, policy, &logs)
	response := httptest.NewRecorder()
	gateway.ServeHTTP(response, httptest.NewRequest(http.MethodGet, target, nil))

	if response.Code != http.StatusOK || upstreamCalls.Load() != 1 || policy.callCount() != 1 {
		t.Fatalf("response=%d upstreamCalls=%d policyCalls=%d", response.Code, upstreamCalls.Load(), policy.callCount())
	}
	if !strings.Contains(logs.String(), "route=emby_web") {
		t.Fatalf("logs=%q", logs.String())
	}
	assertSecretsAbsent(t, logs.String(), "Emby Web", "web-device-1", "Google Chrome macOS", "zh-cn")
}

func TestGatewayBrandingConfigurationRequiresExactWebQueryMetadataAndPolicy(t *testing.T) {
	tests := []struct {
		name       string
		method     string
		path       string
		query      url.Values
		header     string
		enabled    bool
		wantStatus int
		wantPolicy int
		wantPage   bool
	}{
		{name: "disabled", method: http.MethodGet, path: "/emby/Branding/Configuration", query: validWebApplicationQuery(), wantStatus: http.StatusNotFound, wantPolicy: 1, wantPage: true},
		{name: "missing query", method: http.MethodGet, path: "/emby/Branding/Configuration", wantStatus: http.StatusUnauthorized},
		{name: "incomplete query", method: http.MethodGet, path: "/emby/Branding/Configuration", query: func() url.Values {
			values := validWebApplicationQuery()
			values.Del("X-Emby-Device-Name")
			return values
		}(), wantStatus: http.StatusUnauthorized},
		{name: "header and query", method: http.MethodGet, path: "/emby/Branding/Configuration", query: validWebApplicationQuery(), header: fixtureApplicationAuthorization, enabled: true, wantStatus: http.StatusUnauthorized},
		{name: "head", method: http.MethodHead, path: "/emby/Branding/Configuration", query: validWebApplicationQuery(), enabled: true, wantStatus: http.StatusUnauthorized},
		{name: "root path", method: http.MethodGet, path: "/Branding/Configuration", query: validWebApplicationQuery(), enabled: true, wantStatus: http.StatusUnauthorized},
		{name: "trailing slash", method: http.MethodGet, path: "/emby/Branding/Configuration/", query: validWebApplicationQuery(), enabled: true, wantStatus: http.StatusUnauthorized},
		{name: "encoded path", method: http.MethodGet, path: "/emby/Br%61nding/Configuration", query: validWebApplicationQuery(), enabled: true, wantStatus: http.StatusUnauthorized},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var upstreamCalls atomic.Int32
			upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
				upstreamCalls.Add(1)
				writer.WriteHeader(http.StatusNoContent)
			}))
			defer upstream.Close()

			policy := &fakeWebSurfacePolicy{enabled: test.enabled}
			var logs bytes.Buffer
			gateway := newWebSurfaceTestGateway(t, upstream.URL, policy, &logs)
			target := test.path
			if len(test.query) > 0 {
				target += "?" + test.query.Encode()
			}
			request := httptest.NewRequest(test.method, target, nil)
			if test.header != "" {
				request.Header.Set(embyAuthorizationHeader, test.header)
			}
			response := httptest.NewRecorder()
			gateway.ServeHTTP(response, request)

			if response.Code != test.wantStatus || upstreamCalls.Load() != 0 || policy.callCount() != test.wantPolicy {
				t.Fatalf("response=%d body=%q upstreamCalls=%d policyCalls=%d", response.Code, response.Body.String(), upstreamCalls.Load(), policy.callCount())
			}
			if test.wantPage {
				assertWebSurfaceDisabledResponse(t, response, test.method)
			} else if response.Body.Len() != 0 {
				t.Fatalf("response body=%q, want empty", response.Body.String())
			}
			assertSecretsAbsent(t, logs.String(), fixtureApplicationAuthorization, "Emby Web", "web-device-1")
		})
	}
}

func TestGatewayBrandingConfigurationWithMappedTokenUsesProtectedRoute(t *testing.T) {
	query := validWebApplicationQuery()
	query.Set("api_key", fixtureAccessToken)
	target := "/emby/Branding/Configuration?" + query.Encode()
	var upstreamCalls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		upstreamCalls.Add(1)
		if request.URL.RequestURI() != target {
			t.Fatalf("upstream request=%s, want %s", request.URL.RequestURI(), target)
		}
		writer.WriteHeader(http.StatusNoContent)
	}))
	defer upstream.Close()

	policy := &fakeWebSurfacePolicy{enabled: false}
	var logs bytes.Buffer
	gateway := newWebSurfaceTestGateway(t, upstream.URL, policy, &logs)
	response := httptest.NewRecorder()
	gateway.ServeHTTP(response, httptest.NewRequest(http.MethodGet, target, nil))

	if response.Code != http.StatusNoContent || upstreamCalls.Load() != 1 || policy.callCount() != 0 {
		t.Fatalf("response=%d upstreamCalls=%d policyCalls=%d", response.Code, upstreamCalls.Load(), policy.callCount())
	}
	if !strings.Contains(logs.String(), "route=protected") || strings.Contains(logs.String(), "route=emby_web") {
		t.Fatalf("logs=%q", logs.String())
	}
	assertSecretsAbsent(t, logs.String(), fixtureAccessToken, "Emby Web", "web-device-1")
}

func TestGatewayWebSurfaceProxiesExactItemImageWithoutToken(t *testing.T) {
	tests := []struct {
		name   string
		method string
		target string
	}{
		{
			name: "get primary image", method: http.MethodGet,
			target: "/emby/Items/32019/Images/Primary?maxHeight=380&maxWidth=676&tag=fixture-tag&quality=90",
		},
		{
			name: "get indexed backdrop image", method: http.MethodGet,
			target: "/emby/Items/72567/Images/Backdrop/0?tag=fixture-tag&maxWidth=3840&quality=70",
		},
		{name: "head backdrop image", method: http.MethodHead, target: "/emby/Items/item-guid/Images/Backdrop?tag=fixture-tag"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var upstreamCalls atomic.Int32
			upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				upstreamCalls.Add(1)
				if request.Method != test.method || request.URL.RequestURI() != test.target {
					t.Fatalf("upstream request=%s %s, want %s %s", request.Method, request.URL.RequestURI(), test.method, test.target)
				}
				writer.Header().Set("Content-Type", "image/jpeg")
				writer.WriteHeader(http.StatusOK)
				if request.Method != http.MethodHead {
					_, _ = io.WriteString(writer, "fixture-image")
				}
			}))
			defer upstream.Close()

			policy := &fakeWebSurfacePolicy{enabled: true}
			var logs bytes.Buffer
			gateway := newWebSurfaceTestGateway(t, upstream.URL, policy, &logs)
			response := httptest.NewRecorder()
			gateway.ServeHTTP(response, httptest.NewRequest(test.method, test.target, nil))

			if response.Code != http.StatusOK || response.Header().Get("Content-Type") != "image/jpeg" ||
				upstreamCalls.Load() != 1 || policy.callCount() != 1 {
				t.Fatalf("response=%d headers=%v upstreamCalls=%d policyCalls=%d", response.Code, response.Header(), upstreamCalls.Load(), policy.callCount())
			}
			if test.method == http.MethodGet && response.Body.String() != "fixture-image" {
				t.Fatalf("body=%q", response.Body.String())
			}
			if !strings.Contains(logs.String(), "route=emby_web") || strings.Contains(logs.String(), "code=token_header_invalid") {
				t.Fatalf("logs=%q", logs.String())
			}
			assertSecretsAbsent(t, logs.String(), "fixture-tag")
		})
	}
}

func TestGatewayItemImageRequiresExactPathMethodAndWebPolicy(t *testing.T) {
	tests := []struct {
		name       string
		method     string
		target     string
		enabled    bool
		wantStatus int
		wantPolicy int
		wantPage   bool
	}{
		{name: "disabled indexed image", method: http.MethodGet, target: "/emby/Items/72567/Images/Backdrop/1?tag=fixture", wantStatus: http.StatusNotFound, wantPolicy: 1, wantPage: true},
		{name: "root path", method: http.MethodGet, target: "/Items/32019/Images/Primary?tag=fixture", enabled: true, wantStatus: http.StatusUnauthorized},
		{name: "trailing slash", method: http.MethodGet, target: "/emby/Items/32019/Images/Primary/?tag=fixture", enabled: true, wantStatus: http.StatusUnauthorized},
		{name: "encoded item id", method: http.MethodGet, target: "/emby/Items/item%2Did/Images/Primary?tag=fixture", enabled: true, wantStatus: http.StatusUnauthorized},
		{name: "empty item id", method: http.MethodGet, target: "/emby/Items//Images/Primary?tag=fixture", enabled: true, wantStatus: http.StatusUnauthorized},
		{name: "oversized image type", method: http.MethodGet, target: "/emby/Items/32019/Images/" + strings.Repeat("a", 129), enabled: true, wantStatus: http.StatusUnauthorized},
		{name: "index leading zero", method: http.MethodGet, target: "/emby/Items/32019/Images/Backdrop/00", enabled: true, wantStatus: http.StatusUnauthorized},
		{name: "negative index", method: http.MethodGet, target: "/emby/Items/32019/Images/Backdrop/-1", enabled: true, wantStatus: http.StatusUnauthorized},
		{name: "signed index", method: http.MethodGet, target: "/emby/Items/32019/Images/Backdrop/+1", enabled: true, wantStatus: http.StatusUnauthorized},
		{name: "non numeric index", method: http.MethodGet, target: "/emby/Items/32019/Images/Backdrop/first", enabled: true, wantStatus: http.StatusUnauthorized},
		{name: "int32 overflow index", method: http.MethodGet, target: "/emby/Items/32019/Images/Backdrop/2147483648", enabled: true, wantStatus: http.StatusUnauthorized},
		{name: "encoded index", method: http.MethodGet, target: "/emby/Items/32019/Images/Backdrop/%30", enabled: true, wantStatus: http.StatusUnauthorized},
		{name: "index extra path", method: http.MethodGet, target: "/emby/Items/32019/Images/Backdrop/0/extra", enabled: true, wantStatus: http.StatusUnauthorized},
		{name: "post", method: http.MethodPost, target: "/emby/Items/32019/Images/Primary", enabled: true, wantStatus: http.StatusUnauthorized},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var upstreamCalls atomic.Int32
			upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
				upstreamCalls.Add(1)
				writer.WriteHeader(http.StatusNoContent)
			}))
			defer upstream.Close()

			policy := &fakeWebSurfacePolicy{enabled: test.enabled}
			var logs bytes.Buffer
			gateway := newWebSurfaceTestGateway(t, upstream.URL, policy, &logs)
			response := httptest.NewRecorder()
			gateway.ServeHTTP(response, httptest.NewRequest(test.method, test.target, nil))

			if response.Code != test.wantStatus || upstreamCalls.Load() != 0 || policy.callCount() != test.wantPolicy {
				t.Fatalf("response=%d body=%q upstreamCalls=%d policyCalls=%d", response.Code, response.Body.String(), upstreamCalls.Load(), policy.callCount())
			}
			if test.wantPage {
				assertWebSurfaceDisabledResponse(t, response, test.method)
			} else if response.Body.Len() != 0 {
				t.Fatalf("response body=%q, want empty", response.Body.String())
			}
		})
	}
}

func TestGatewayItemImageWithMappedTokenUsesProtectedRoute(t *testing.T) {
	target := "/emby/Items/72567/Images/Backdrop/0?tag=fixture&api_key=" + fixtureAccessToken
	var upstreamCalls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		upstreamCalls.Add(1)
		if request.URL.RequestURI() != target {
			t.Fatalf("upstream request=%s, want %s", request.URL.RequestURI(), target)
		}
		writer.WriteHeader(http.StatusNoContent)
	}))
	defer upstream.Close()

	policy := &fakeWebSurfacePolicy{enabled: false}
	var logs bytes.Buffer
	gateway := newWebSurfaceTestGateway(t, upstream.URL, policy, &logs)
	response := httptest.NewRecorder()
	gateway.ServeHTTP(response, httptest.NewRequest(http.MethodGet, target, nil))

	if response.Code != http.StatusNoContent || upstreamCalls.Load() != 1 || policy.callCount() != 0 {
		t.Fatalf("response=%d upstreamCalls=%d policyCalls=%d", response.Code, upstreamCalls.Load(), policy.callCount())
	}
	if !strings.Contains(logs.String(), "route=protected") || strings.Contains(logs.String(), "route=emby_web") {
		t.Fatalf("logs=%q", logs.String())
	}
	assertSecretsAbsent(t, logs.String(), fixtureAccessToken)
}

func TestGatewayWebSurfaceConfigFailureFailsClosed(t *testing.T) {
	var upstreamCalls int
	upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		upstreamCalls++
		writer.WriteHeader(http.StatusOK)
	}))
	defer upstream.Close()

	policy := &fakeWebSurfacePolicy{err: errors.New("database unavailable with private-value")}
	var logs bytes.Buffer
	gateway := newWebSurfaceTestGateway(t, upstream.URL, policy, &logs)
	response := httptest.NewRecorder()
	gateway.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/web/index.html", nil))

	if response.Code != http.StatusServiceUnavailable || response.Body.Len() != 0 || upstreamCalls != 0 {
		t.Fatalf("response=%d body=%q upstreamCalls=%d", response.Code, response.Body.String(), upstreamCalls)
	}
	for _, expected := range []string{"code=web_surface_config_unavailable", `message="Emby网页配置读取失败"`, "errorType=*errors.errorString"} {
		if !strings.Contains(logs.String(), expected) {
			t.Fatalf("logs=%q, want %s", logs.String(), expected)
		}
	}
	assertSecretsAbsent(t, logs.String(), "private-value")
}

func TestGatewayWebSurfaceDoesNotBypassProtectedWebAPIOrRootWebSocket(t *testing.T) {
	var upstreamMu sync.Mutex
	upstreamPaths := make([]string, 0, 5)
	upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		upstreamMu.Lock()
		upstreamPaths = append(upstreamPaths, request.URL.RequestURI())
		upstreamMu.Unlock()
		if request.URL.Path == "/" && request.Header.Get("Upgrade") != "websocket" {
			t.Errorf("root request missing websocket upgrade: %#v", request.Header)
		}
		writer.WriteHeader(http.StatusOK)
	}))
	defer upstream.Close()

	policy := &fakeWebSurfacePolicy{enabled: false}
	var logs bytes.Buffer
	gateway := newWebSurfaceTestGateway(t, upstream.URL, policy, &logs)
	protectedPaths := []string{
		"/web/ConfigurationPage",
		"/web/ConfigurationPages",
		"/web/strings",
		"/web/stringset",
	}
	for _, path := range protectedPaths {
		unauthorized := httptest.NewRecorder()
		gateway.ServeHTTP(unauthorized, httptest.NewRequest(http.MethodGet, path, nil))
		if unauthorized.Code != http.StatusUnauthorized {
			t.Fatalf("unauthorized %s status=%d", path, unauthorized.Code)
		}

		request := httptest.NewRequest(http.MethodGet, path+"?fixture=keep", nil)
		request.Header.Set(accessTokenHeader, fixtureAccessToken)
		authorized := httptest.NewRecorder()
		gateway.ServeHTTP(authorized, request)
		if authorized.Code != http.StatusOK {
			t.Fatalf("authorized %s status=%d", path, authorized.Code)
		}
	}

	websocketRequest := httptest.NewRequest(http.MethodGet, "/?api_key="+fixtureAccessToken+"&deviceId=device-1", nil)
	websocketRequest.Header.Set("Connection", "Upgrade")
	websocketRequest.Header.Set("Upgrade", "websocket")
	websocket := httptest.NewRecorder()
	gateway.ServeHTTP(websocket, websocketRequest)
	if websocket.Code != http.StatusOK {
		t.Fatalf("websocket probe status=%d", websocket.Code)
	}

	if calls := policy.callCount(); calls != 0 {
		t.Fatalf("Web policy calls=%d, want protected API/WebSocket to bypass Web UI switch", calls)
	}
	upstreamMu.Lock()
	paths := append([]string(nil), upstreamPaths...)
	upstreamMu.Unlock()
	if len(paths) != 5 {
		t.Fatalf("upstream paths=%#v, want 5 authorized requests", paths)
	}
	for index, path := range protectedPaths {
		want := "/emby" + path + "?fixture=keep"
		if paths[index] != want {
			t.Fatalf("upstream path[%d]=%q, want %q", index, paths[index], want)
		}
	}
	if paths[4] != "/?api_key="+fixtureAccessToken+"&deviceId=device-1" {
		t.Fatalf("websocket path=%q", paths[4])
	}
	assertSecretsAbsent(t, logs.String(), fixtureAccessToken)
}

func TestGatewayRootWebSocketUpgradeRemainsTokenGatedAndProxied(t *testing.T) {
	upstreamReached := make(chan struct{}, 1)
	upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/" || request.URL.Query().Get("api_key") != fixtureAccessToken ||
			request.URL.Query().Get("deviceId") != "device-1" || !isWebSocketUpgrade(request) {
			t.Errorf("unexpected upstream WebSocket request: %s headers=%v", request.URL.RequestURI(), request.Header)
		}
		upstreamReached <- struct{}{}
		connection, readWriter, err := writer.(http.Hijacker).Hijack()
		if err != nil {
			t.Errorf("hijack upstream: %v", err)
			return
		}
		defer connection.Close()
		_, _ = readWriter.WriteString("HTTP/1.1 101 Switching Protocols\r\nConnection: Upgrade\r\nUpgrade: websocket\r\n\r\n")
		_ = readWriter.Flush()
	}))
	defer upstream.Close()

	policy := &fakeWebSurfacePolicy{enabled: false}
	var logs bytes.Buffer
	gateway := newWebSurfaceTestGateway(t, upstream.URL, policy, &logs)
	gatewayDone := make(chan struct{})
	gatewayServer := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		gateway.ServeHTTP(writer, request)
		close(gatewayDone)
	}))
	defer gatewayServer.Close()

	connection, err := net.Dial("tcp", gatewayServer.Listener.Addr().String())
	if err != nil {
		t.Fatalf("dial fake Gateway: %v", err)
	}
	_, err = fmt.Fprintf(connection,
		"GET /?api_key=%s&deviceId=device-1 HTTP/1.1\r\nHost: gateway.test\r\nConnection: Upgrade\r\nUpgrade: websocket\r\n\r\n",
		fixtureAccessToken,
	)
	if err != nil {
		t.Fatalf("write WebSocket request: %v", err)
	}
	response, err := http.ReadResponse(bufio.NewReader(connection), &http.Request{Method: http.MethodGet})
	if err != nil {
		t.Fatalf("read WebSocket response: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusSwitchingProtocols {
		t.Fatalf("WebSocket status=%d", response.StatusCode)
	}
	select {
	case <-upstreamReached:
	default:
		t.Fatal("upstream WebSocket was not reached")
	}
	if calls := policy.callCount(); calls != 0 {
		t.Fatalf("Web policy calls=%d, want WebSocket bypass", calls)
	}
	_ = connection.Close()
	select {
	case <-gatewayDone:
	case <-time.After(time.Second):
		t.Fatal("Gateway WebSocket handler did not finish")
	}
	assertSecretsAbsent(t, logs.String(), fixtureAccessToken)
}

func TestClassifyEmbyWebSurfaceIsDepthAndMethodExact(t *testing.T) {
	tests := []struct {
		name   string
		method string
		target string
		header http.Header
		want   bool
	}{
		{name: "root", method: http.MethodGet, target: "/", want: true},
		{name: "favicon", method: http.MethodHead, target: "/favicon.ico", want: true},
		{name: "web root", method: http.MethodGet, target: "/web", want: true},
		{name: "web asset", method: http.MethodGet, target: "/web/app.js", want: true},
		{name: "locale json asset", method: http.MethodGet, target: "/web/strings/zh-CN.json?v=fixture", want: true},
		{name: "branding css", method: http.MethodHead, target: "/emby/Branding/Css.css?v=fixture", want: true},
		{name: "branding configuration", method: http.MethodGet, target: "/emby/Branding/Configuration?" + validWebApplicationQuery().Encode(), want: true},
		{name: "branding configuration missing query", method: http.MethodGet, target: "/emby/Branding/Configuration"},
		{name: "branding configuration head", method: http.MethodHead, target: "/emby/Branding/Configuration?" + validWebApplicationQuery().Encode()},
		{name: "item image get", method: http.MethodGet, target: "/emby/Items/32019/Images/Primary?tag=fixture", want: true},
		{name: "item image head", method: http.MethodHead, target: "/emby/Items/item-guid/Images/Backdrop", want: true},
		{name: "root item image remains protected", method: http.MethodGet, target: "/Items/32019/Images/Primary"},
		{name: "indexed item image", method: http.MethodGet, target: "/emby/Items/72567/Images/Backdrop/0", want: true},
		{name: "indexed item image int32 max", method: http.MethodHead, target: "/emby/Items/72567/Images/Backdrop/2147483647", want: true},
		{name: "indexed item image leading zero", method: http.MethodGet, target: "/emby/Items/72567/Images/Backdrop/00"},
		{name: "indexed item image negative", method: http.MethodGet, target: "/emby/Items/72567/Images/Backdrop/-1"},
		{name: "indexed item image overflow", method: http.MethodGet, target: "/emby/Items/72567/Images/Backdrop/2147483648"},
		{name: "indexed item image extra path", method: http.MethodGet, target: "/emby/Items/72567/Images/Backdrop/0/extra"},
		{name: "encoded item image remains protected", method: http.MethodGet, target: "/emby/Items/item%2Did/Images/Primary"},
		{name: "item image post", method: http.MethodPost, target: "/emby/Items/32019/Images/Primary"},
		{name: "item image token query", method: http.MethodGet, target: "/emby/Items/32019/Images/Primary?api_key=fixture"},
		{name: "root branding remains protected", method: http.MethodGet, target: "/Branding/Css.css"},
		{name: "protected web api", method: http.MethodGet, target: "/web/ConfigurationPage"},
		{name: "protected exact strings api", method: http.MethodGet, target: "/web/strings"},
		{name: "protected strings trailing slash", method: http.MethodGet, target: "/web/strings/"},
		{name: "protected web api trailing slash", method: http.MethodGet, target: "/web/ConfigurationPage/"},
		{name: "protected web api deeper variant", method: http.MethodGet, target: "/web/strings/fixture"},
		{name: "locale resource wrong extension", method: http.MethodGet, target: "/web/strings/zh-CN.js"},
		{name: "locale resource deeper path", method: http.MethodGet, target: "/web/strings/zh-CN.json/extra"},
		{name: "locale resource unsafe name", method: http.MethodGet, target: "/web/strings/zh.CN.json"},
		{name: "locale resource oversized name", method: http.MethodGet, target: "/web/strings/" + strings.Repeat("a", maxWebLocaleNameLength+1) + ".json"},
		{name: "locale resource encoded name", method: http.MethodGet, target: "/web/strings/zh%2DCN.json"},
		{name: "branding without css extension", method: http.MethodGet, target: "/emby/Branding/Css"},
		{name: "branding deeper path", method: http.MethodGet, target: "/emby/Branding/Css.css/extra"},
		{name: "branding encoded path", method: http.MethodGet, target: "/emby/Br%61nding/Css.css"},
		{name: "branding post", method: http.MethodPost, target: "/emby/Branding/Css.css"},
		{name: "post asset", method: http.MethodPost, target: "/web/app.js"},
		{name: "web lookalike", method: http.MethodGet, target: "/webish/app.js"},
		{name: "encoded web", method: http.MethodGet, target: "/web%2Fapp.js"},
		{name: "root websocket", method: http.MethodGet, target: "/", header: http.Header{"Connection": {"Upgrade"}, "Upgrade": {"websocket"}}},
		{name: "root token query", method: http.MethodGet, target: "/?api_key=fixture"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(test.method, test.target, nil)
			request.Header = test.header
			if got := isEmbyWebSurfaceRequest(request); got != test.want {
				t.Fatalf("isEmbyWebSurfaceRequest(%s %s)=%t, want %t", test.method, test.target, got, test.want)
			}
		})
	}
}

func newWebSurfaceTestGateway(t *testing.T, upstreamRawURL string, policy WebSurfacePolicy, logs *bytes.Buffer) *Gateway {
	t.Helper()
	upstreamURL, err := url.Parse(upstreamRawURL)
	if err != nil {
		t.Fatalf("parse upstream URL: %v", err)
	}
	gateway, err := New(Config{
		Upstream:         upstreamURL,
		TokenService:     &fakeTokenService{principal: fixturePrincipal()},
		WebSurfacePolicy: policy,
		Logger:           log.New(logs, "", 0),
		Debug:            true,
	})
	if err != nil {
		t.Fatalf("New() error=%v", err)
	}
	return gateway
}

type fakeWebSurfacePolicy struct {
	mu      sync.Mutex
	enabled bool
	err     error
	calls   int
}

func (policy *fakeWebSurfacePolicy) PlaybackGatewayWebEnabled(context.Context) (bool, error) {
	policy.mu.Lock()
	defer policy.mu.Unlock()
	policy.calls++
	return policy.enabled, policy.err
}

func (policy *fakeWebSurfacePolicy) set(enabled bool, err error) {
	policy.mu.Lock()
	defer policy.mu.Unlock()
	policy.enabled = enabled
	policy.err = err
}

func (policy *fakeWebSurfacePolicy) callCount() int {
	policy.mu.Lock()
	defer policy.mu.Unlock()
	return policy.calls
}
