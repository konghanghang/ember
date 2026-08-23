package playbackgateway

import (
	"net/http"
	"strings"
)

type requestPathMode string

const (
	requestPathModeRoot         requestPathMode = "root"
	requestPathModeEmbyPrefixed requestPathMode = "emby_prefixed"
	requestPathModePassthrough  requestPathMode = "passthrough"
)

// normalizeEmbyAPIPath maps the root API paths emitted by Emby clients onto
// the upstream /emby base while leaving Web and unknown root surfaces alone.
// It mutates only URL.Path; method, query, headers and body remain untouched.
func normalizeEmbyAPIPath(request *http.Request) (requestPathMode, bool) {
	if request == nil || request.URL == nil {
		return requestPathModePassthrough, false
	}
	requestPath := request.URL.Path
	if request.URL.EscapedPath() != requestPath {
		return requestPathModePassthrough, true
	}
	if requestPath == "/emby/emby" || strings.HasPrefix(requestPath, "/emby/emby/") {
		return requestPathModeEmbyPrefixed, false
	}
	if requestPath == "/emby" || strings.HasPrefix(requestPath, "/emby/") {
		return requestPathModeEmbyPrefixed, true
	}
	if !isEmbyRootAPIPath(requestPath) {
		return requestPathModePassthrough, true
	}
	request.URL.Path = "/emby" + requestPath
	request.URL.RawPath = ""
	return requestPathModeRoot, true
}

// isEmbyRootAPIPath recognizes the union of top-level API families from the
// supported stable Emby 4.9 OpenAPI tags. The ambiguous root /web surface is
// excluded until its static-resource and WebSocket contract is implemented.
func isEmbyRootAPIPath(requestPath string) bool {
	if len(requestPath) < 2 || requestPath[0] != '/' {
		return false
	}
	segment := strings.TrimPrefix(requestPath, "/")
	if separator := strings.IndexByte(segment, '/'); separator >= 0 {
		segment = segment[:separator]
	}
	switch segment {
	case "Albums", "Artists", "Audio", "AudioBooks", "AudioCodecs", "AudioLayouts",
		"Auth", "BackupRestore", "Branding", "Channels", "Collections", "Connect",
		"Containers", "Devices", "DisplayPreferences", "Dlna", "Encoding", "Environment",
		"ExtendedVideoTypes", "Features", "GameGenres", "Games", "Genres", "Images",
		"ItemTypes", "Items", "Libraries", "Library", "LiveStreams", "LiveTv", "Localization",
		"Movies", "MusicGenres", "Notifications", "OfficialRatings", "Packages", "Parties",
		"Persons", "Playback", "Playlists", "Plugins", "Providers", "ScheduledTasks", "Sessions",
		"Shows", "Songs", "StreamLanguages", "Studios", "SubtitleCodecs", "Sync", "System",
		"Tags", "Trailers", "UI", "UserSettings", "Users", "VideoCodecs", "Videos", "Years",
		"openapi", "openapi.json", "swagger", "swagger.json":
		return true
	default:
		return false
	}
}

// routeKindCode returns a fixed diagnostic label and never includes the
// request path, query or any client-controlled identifier.
func routeKindCode(kind routeKind) string {
	switch kind {
	case routeAuthentication:
		return "authentication"
	case routePublicBootstrap:
		return "public_bootstrap"
	case routePlaybackInfo:
		return "playback_info"
	case routeVideo:
		return "video"
	default:
		return "protected"
	}
}
