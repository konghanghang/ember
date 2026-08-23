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

// normalizeEmbyAPIPath maps case-insensitive root API families emitted by Emby
// clients onto the upstream /emby base while leaving Web and unknown surfaces.
// It mutates only URL.Path; method, query, headers and body remain untouched.
func normalizeEmbyAPIPath(request *http.Request) (requestPathMode, bool) {
	if request == nil || request.URL == nil {
		return requestPathModePassthrough, false
	}
	requestPath := request.URL.Path
	if request.URL.EscapedPath() != requestPath {
		return requestPathModePassthrough, true
	}
	segments := strings.Split(requestPath, "/")
	if len(segments) >= 2 && strings.EqualFold(segments[1], "emby") {
		if len(segments) >= 3 && strings.EqualFold(segments[2], "emby") {
			return requestPathModeEmbyPrefixed, false
		}
		if segments[1] != "emby" {
			segments[1] = "emby"
			request.URL.Path = strings.Join(segments, "/")
			request.URL.RawPath = ""
		}
		return requestPathModeEmbyPrefixed, true
	}
	if !isEmbyRootAPIPath(requestPath) {
		return requestPathModePassthrough, true
	}
	request.URL.Path = "/emby" + requestPath
	request.URL.RawPath = ""
	return requestPathModeRoot, true
}

// isEmbyRootAPIPath recognizes the case-insensitive union of top-level API
// families from supported stable Emby 4.9 tags. Root /web stays separate.
func isEmbyRootAPIPath(requestPath string) bool {
	if len(requestPath) < 2 || requestPath[0] != '/' {
		return false
	}
	segment := strings.TrimPrefix(requestPath, "/")
	if separator := strings.IndexByte(segment, '/'); separator >= 0 {
		segment = segment[:separator]
	}
	switch strings.ToLower(segment) {
	case "albums", "artists", "audio", "audiobooks", "audiocodecs", "audiolayouts",
		"auth", "backuprestore", "branding", "channels", "collections", "connect",
		"containers", "devices", "displaypreferences", "dlna", "encoding", "environment",
		"extendedvideotypes", "features", "gamegenres", "games", "genres", "images",
		"itemtypes", "items", "libraries", "library", "livestreams", "livetv", "localization",
		"movies", "musicgenres", "notifications", "officialratings", "packages", "parties",
		"persons", "playback", "playlists", "plugins", "providers", "scheduledtasks", "sessions",
		"shows", "songs", "streamlanguages", "studios", "subtitlecodecs", "sync", "system",
		"tags", "trailers", "ui", "usersettings", "users", "videocodecs", "videos", "years",
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
	case routeSystemInfoPublic:
		return "system_info_public"
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
