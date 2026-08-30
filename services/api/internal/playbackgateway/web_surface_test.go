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
	"testing"
	"time"
)

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
	if disabled.Code != http.StatusNotFound || disabled.Body.Len() != 0 {
		t.Fatalf("disabled response=%d body=%q", disabled.Code, disabled.Body.String())
	}

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
	if calls := policy.callCount(); calls != 4 {
		t.Fatalf("policy calls=%d, want one evaluation per Web request", calls)
	}
	for _, expected := range []string{"code=web_surface_disabled", `message="Emby网页访问已关闭"`, "route=emby_web"} {
		if !strings.Contains(logs.String(), expected) {
			t.Fatalf("logs=%q, want %s", logs.String(), expected)
		}
	}
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
		{name: "protected web api", method: http.MethodGet, target: "/web/ConfigurationPage"},
		{name: "protected web api trailing slash", method: http.MethodGet, target: "/web/ConfigurationPage/"},
		{name: "protected web api deeper variant", method: http.MethodGet, target: "/web/strings/fixture"},
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
