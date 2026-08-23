package playbackgateway

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNormalizeEmbyAPIPathUsesVersionedRootFamilies(t *testing.T) {
	tests := []struct {
		name     string
		target   string
		wantPath string
		wantMode requestPathMode
		wantOK   bool
	}{
		{name: "system root", target: "/System/Info/Public?fixture=keep", wantPath: "/emby/System/Info/Public", wantMode: requestPathModeRoot, wantOK: true},
		{name: "authentication root", target: "/Users/AuthenticateByName", wantPath: authenticationPath, wantMode: requestPathModeRoot, wantOK: true},
		{name: "playback root", target: "/Items/item-1/PlaybackInfo", wantPath: "/emby/Items/item-1/PlaybackInfo", wantMode: requestPathModeRoot, wantOK: true},
		{name: "session root", target: "/Sessions/Playing/Progress", wantPath: "/emby/Sessions/Playing/Progress", wantMode: requestPathModeRoot, wantOK: true},
		{name: "openapi root", target: "/openapi.json", wantPath: "/emby/openapi.json", wantMode: requestPathModeRoot, wantOK: true},
		{name: "existing prefix", target: "/emby/Items/item-1", wantPath: "/emby/Items/item-1", wantMode: requestPathModeEmbyPrefixed, wantOK: true},
		{name: "web surface remains separate", target: "/web/index.html", wantPath: "/web/index.html", wantMode: requestPathModePassthrough, wantOK: true},
		{name: "unknown surface remains separate", target: "/favicon.ico", wantPath: "/favicon.ico", wantMode: requestPathModePassthrough, wantOK: true},
		{name: "escaped special path is not normalized", target: "/System%2FInfo%2FPublic", wantPath: "/System/Info/Public", wantMode: requestPathModePassthrough, wantOK: true},
		{name: "duplicate prefix", target: "/emby/emby/System/Info", wantPath: "/emby/emby/System/Info", wantMode: requestPathModeEmbyPrefixed},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, test.target, nil)
			originalQuery := request.URL.RawQuery
			mode, ok := normalizeEmbyAPIPath(request)
			if ok != test.wantOK || mode != test.wantMode || request.URL.Path != test.wantPath {
				t.Fatalf("normalizeEmbyAPIPath() = path %q mode %q ok %t, want %q %q %t", request.URL.Path, mode, ok, test.wantPath, test.wantMode, test.wantOK)
			}
			if request.URL.RawQuery != originalQuery {
				t.Fatalf("query changed from %q to %q", originalQuery, request.URL.RawQuery)
			}
		})
	}
}

func TestNormalizeEmbyAPIPathFeedsExistingSpecialRouteClassifiers(t *testing.T) {
	tests := []struct {
		method string
		path   string
		want   routeKind
	}{
		{method: http.MethodGet, path: "/System/Info/Public", want: routeSystemInfoPublic},
		{method: http.MethodPost, path: "/Users/AuthenticateByName", want: routeAuthentication},
		{method: http.MethodGet, path: "/Items/item-1/PlaybackInfo", want: routePlaybackInfo},
		{method: http.MethodGet, path: "/Videos/item-1/stream.mkv", want: routeVideo},
		{method: http.MethodPost, path: "/Sessions/Playing/Progress", want: routeProtected},
	}
	for _, test := range tests {
		request := httptest.NewRequest(test.method, test.path, nil)
		if _, ok := normalizeEmbyAPIPath(request); !ok {
			t.Fatalf("normalizeEmbyAPIPath(%s %s) failed", test.method, test.path)
		}
		if got := classifyRoute(request); got != test.want {
			t.Fatalf("classifyRoute(%s %s) = %v, want %v", test.method, test.path, got, test.want)
		}
	}
}
