package playbackgateway

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"
)

var errWebSurfacePolicyUnavailable = errors.New("playback gateway Web Surface policy unavailable")

const (
	brandingCSSPath           = "/emby/Branding/Css.css"
	brandingConfigurationPath = "/emby/Branding/Configuration"
	maxWebLocaleNameLength    = 64
)

// serveWebSurface applies the current short-cached database switch before
// transparently proxying an Emby Web page, static asset or exact metadata-gated
// login bootstrap without local user Token gating.
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

// isEmbyWebSurfaceRequest recognizes browser entry/static GET and HEAD requests,
// the exact query-gated Branding bootstrap and no-Token item images. Protected
// WebAppService APIs and root WebSocket upgrades stay on the Token-gated path.
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
	if exactRequestPathFold(request.URL, brandingCSSPath) {
		return true
	}
	if request.Method == http.MethodGet && exactRequestPathFold(request.URL, brandingConfigurationPath) {
		_, carrier, metadataOK := extractAuthenticationApplicationMetadata(request)
		return carrier == applicationMetadataQuery && metadataOK
	}
	if isEmbyWebItemImagePath(path) {
		return true
	}
	return strings.HasPrefix(path, "/web/") && !hasRootWebAppAPIReservedPrefix(path)
}

// isEmbyWebItemImagePath accepts the exact /emby item image shape with an
// optional canonical non-negative int32 Index emitted by the target Web
// client. Mutation, root, empty and deeper segments remain protected.
func isEmbyWebItemImagePath(path string) bool {
	segments := strings.Split(path, "/")
	if (len(segments) != 6 && len(segments) != 7) || segments[0] != "" || !strings.EqualFold(segments[1], "emby") ||
		!strings.EqualFold(segments[2], "Items") || !strings.EqualFold(segments[4], "Images") {
		return false
	}
	if !validPublicPathSegment(segments[3]) || !validPublicPathSegment(segments[5]) {
		return false
	}
	return len(segments) == 6 || validEmbyWebImageIndex(segments[6])
}

// validEmbyWebImageIndex accepts the canonical non-negative int32 path value
// from the fixed ImageService contract and rejects alternate numeric spellings.
func validEmbyWebImageIndex(value string) bool {
	if value == "0" {
		return true
	}
	if value == "" || value[0] == '0' {
		return false
	}
	index, err := strconv.ParseUint(value, 10, 31)
	return err == nil && strconv.FormatUint(index, 10) == value
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
	case "configurationpage", "configurationpages", "stringset":
		return true
	case "strings":
		return !isWebLocaleResourcePath(path)
	default:
		return false
	}
}

// isWebLocaleResourcePath accepts only the one-segment locale JSON shape
// emitted by the target Emby 4.9.3.0 Web client. The exact /web/strings API and
// every other deeper variant remain protected.
func isWebLocaleResourcePath(path string) bool {
	segments := strings.Split(path, "/")
	if len(segments) != 4 || segments[0] != "" || !strings.EqualFold(segments[1], "web") ||
		!strings.EqualFold(segments[2], "strings") || !strings.HasSuffix(segments[3], ".json") {
		return false
	}
	locale := strings.TrimSuffix(segments[3], ".json")
	if len(locale) == 0 || len(locale) > maxWebLocaleNameLength {
		return false
	}
	for index := range len(locale) {
		character := locale[index]
		if character >= 'a' && character <= 'z' || character >= 'A' && character <= 'Z' ||
			character >= '0' && character <= '9' || character == '-' || character == '_' {
			continue
		}
		return false
	}
	return true
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
