package playbackgateway

import (
	"context"
	"errors"
	"net/http"
	"strings"
)

var errWebSurfacePolicyUnavailable = errors.New("playback gateway Web Surface policy unavailable")

// serveWebSurface applies the current short-cached database switch before
// transparently proxying an Emby Web page or static asset without local user
// Token gating.
func (gateway *Gateway) serveWebSurface(writer http.ResponseWriter, request *http.Request) {
	enabled, err := gateway.playbackGatewayWebEnabled(request.Context())
	if err != nil {
		gateway.logger.Printf("[PlaybackGateway] code=web_surface_config_unavailable message=%q errorType=%T", "Emby网页配置读取失败", err)
		writer.WriteHeader(http.StatusServiceUnavailable)
		return
	}
	if !enabled {
		gateway.logger.Printf("[PlaybackGateway] code=web_surface_disabled message=%q result=rejected statusCode=404", "Emby网页访问已关闭")
		writer.WriteHeader(http.StatusNotFound)
		return
	}

	routeContext := requestRouteContext{kind: routeWebSurface, pathMode: requestPathModePassthrough}
	ctx := context.WithValue(request.Context(), requestRouteContextKey{}, routeContext)
	gateway.proxy.ServeHTTP(writer, request.WithContext(ctx))
}

// playbackGatewayWebEnabled keeps a missing policy fail-closed while allowing
// production and fake tests to inject the same narrow dynamic setting contract.
func (gateway *Gateway) playbackGatewayWebEnabled(ctx context.Context) (bool, error) {
	if gateway == nil || gateway.webSurfacePolicy == nil {
		return false, errWebSurfacePolicyUnavailable
	}
	return gateway.webSurfacePolicy.PlaybackGatewayWebEnabled(ctx)
}

// isEmbyWebSurfaceRequest recognizes only browser entry/static GET and HEAD
// requests. Protected WebAppService APIs and root WebSocket upgrades stay on
// the normal Token-gated API path.
func isEmbyWebSurfaceRequest(request *http.Request) bool {
	if request == nil || request.URL == nil ||
		(request.Method != http.MethodGet && request.Method != http.MethodHead) ||
		request.URL.EscapedPath() != request.URL.Path {
		return false
	}
	if _, reasonCode, tokenPresent := extractProtectedRequestAccessToken(request); tokenPresent || reasonCode != "token_missing" {
		return false
	}
	path := request.URL.Path
	if path == "/" {
		return !isWebSocketUpgrade(request)
	}
	if path == "/favicon.ico" || path == "/web" {
		return true
	}
	return strings.HasPrefix(path, "/web/") && !hasRootWebAppAPIReservedPrefix(path)
}

// isRootWebAppAPIPath protects the four /web API paths fixed by the Emby
// 4.9.3.0 OpenAPI instead of treating them as anonymous static resources.
func isRootWebAppAPIPath(path string) bool {
	segments := strings.Split(path, "/")
	return len(segments) == 3 && hasRootWebAppAPIReservedPrefix(path)
}

// hasRootWebAppAPIReservedPrefix keeps trailing or deeper variants of a
// protected WebAppService route out of the anonymous static Surface.
func hasRootWebAppAPIReservedPrefix(path string) bool {
	segments := strings.Split(path, "/")
	if len(segments) < 3 || !strings.EqualFold(segments[1], "web") {
		return false
	}
	switch strings.ToLower(segments[2]) {
	case "configurationpage", "configurationpages", "strings", "stringset":
		return true
	default:
		return false
	}
}

// isWebSocketUpgrade follows the fixed SDK root WebSocket contract and keeps
// native client sockets independent from the Web UI exposure switch.
func isWebSocketUpgrade(request *http.Request) bool {
	if request == nil || !strings.EqualFold(strings.TrimSpace(request.Header.Get("Upgrade")), "websocket") {
		return false
	}
	for _, value := range request.Header.Values("Connection") {
		for _, token := range strings.Split(value, ",") {
			if strings.EqualFold(strings.TrimSpace(token), "upgrade") {
				return true
			}
		}
	}
	return false
}
