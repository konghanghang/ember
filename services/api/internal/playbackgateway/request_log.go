package playbackgateway

import (
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
)

const (
	maxLoggedMethodBytes      = 32
	maxLoggedHostBytes        = 256
	maxLoggedRequestPathBytes = 1024
	maxLoggedQueryKeys        = 32
	maxLoggedQueryKeyBytes    = 64
	maxLoggedUserAgentVersion = 32
)

// requestLogSnapshot captures only bounded, non-secret request metadata before
// path normalization mutates the request passed to the upstream proxy.
type requestLogSnapshot struct {
	method                     string
	host                       string
	path                       string
	pathTruncated              bool
	queryKeys                  string
	queryKeyCount              int
	queryKeysTruncated         bool
	xEmbyTokenCount            int
	xEmbyTokenState            string
	xMediaBrowserTokenCount    int
	xMediaBrowserTokenState    string
	xEmbyAuthorizationCount    int
	xMediaAuthorizationCount   int
	standardAuthorizationCount int
	applicationScheme          string
	embeddedTokenState         string
	apiKeyQueryPresent         bool
	queryTokenSourceCount      int
	queryTokenState            string
	userAgentFamily            string
	userAgentVersion           string
}

// requestStatusWriter records the first HTTP status without changing the
// wrapped writer. Unwrap preserves optional capabilities used by ReverseProxy.
type requestStatusWriter struct {
	http.ResponseWriter
	statusCode int
}

// WriteHeader records the first status just as net/http ignores later calls.
func (writer *requestStatusWriter) WriteHeader(statusCode int) {
	if writer.statusCode != 0 {
		return
	}
	writer.statusCode = statusCode
	writer.ResponseWriter.WriteHeader(statusCode)
}

// Write records the implicit 200 status before forwarding response bytes.
func (writer *requestStatusWriter) Write(body []byte) (int, error) {
	if writer.statusCode == 0 {
		writer.WriteHeader(http.StatusOK)
	}
	return writer.ResponseWriter.Write(body)
}

// Unwrap lets http.ResponseController reach Flush, Hijack and deadline support
// implemented by the original server writer.
func (writer *requestStatusWriter) Unwrap() http.ResponseWriter {
	return writer.ResponseWriter
}

// captureRequestLogSnapshot records request shape without retaining any Header
// value that could contain a reusable credential.
func captureRequestLogSnapshot(request *http.Request) requestLogSnapshot {
	if request == nil {
		return requestLogSnapshot{
			method:             "unknown",
			xEmbyTokenState:    "missing",
			applicationScheme:  "missing",
			embeddedTokenState: "missing",
			userAgentFamily:    "missing",
		}
	}

	path := ""
	pathTruncated := false
	queryTokenSourceCount := 0
	queryTokenState := "missing"
	apiKeyQueryPresent := false
	if request.URL != nil {
		path, pathTruncated = boundedRequestLogValue(request.URL.EscapedPath(), maxLoggedRequestPathBytes)
		queryTokenSourceCount, queryTokenState, apiKeyQueryPresent = queryTokenDiagnostics(request.URL.Query())
	}
	queryKeys, queryKeyCount, queryKeysTruncated := requestQueryKeySummary(request)
	xEmbyTokenValues := request.Header.Values(accessTokenHeader)
	xMediaBrowserTokenValues := request.Header.Values(mediaBrowserTokenHeader)
	xEmbyAuthorizationValues := request.Header.Values(embyAuthorizationHeader)
	xMediaAuthorizationValues := request.Header.Values(mediaBrowserAuthorizationHeader)
	standardAuthorizationValues := request.Header.Values(standardAuthorizationHeader)
	applicationScheme, embeddedTokenState := applicationAuthorizationDiagnostics(request.Header)
	userAgentFamily, userAgentVersion := userAgentDiagnostics(request.UserAgent())

	return requestLogSnapshot{
		method:                     boundedRequestLogText(request.Method, maxLoggedMethodBytes),
		host:                       boundedRequestLogText(request.Host, maxLoggedHostBytes),
		path:                       path,
		pathTruncated:              pathTruncated,
		queryKeys:                  queryKeys,
		queryKeyCount:              queryKeyCount,
		queryKeysTruncated:         queryKeysTruncated,
		xEmbyTokenCount:            len(xEmbyTokenValues),
		xEmbyTokenState:            tokenValueState(xEmbyTokenValues),
		xMediaBrowserTokenCount:    len(xMediaBrowserTokenValues),
		xMediaBrowserTokenState:    tokenValueState(xMediaBrowserTokenValues),
		xEmbyAuthorizationCount:    len(xEmbyAuthorizationValues),
		xMediaAuthorizationCount:   len(xMediaAuthorizationValues),
		standardAuthorizationCount: len(standardAuthorizationValues),
		applicationScheme:          applicationScheme,
		embeddedTokenState:         embeddedTokenState,
		apiKeyQueryPresent:         apiKeyQueryPresent,
		queryTokenSourceCount:      queryTokenSourceCount,
		queryTokenState:            queryTokenState,
		userAgentFamily:            userAgentFamily,
		userAgentVersion:           userAgentVersion,
	}
}

// logRequestCompletion emits one safe request/result summary. Query values,
// authorization values, Cookies and token bytes never enter this log line.
func (gateway *Gateway) logRequestCompletion(
	snapshot requestLogSnapshot,
	route string,
	pathMode requestPathMode,
	statusCode int,
	startedAt time.Time,
) {
	if !gateway.debug {
		return
	}
	gateway.logger.Printf(
		"[PlaybackGateway] level=debug code=request_completed method=%s host=%q path=%q pathTruncated=%t queryKeys=%q queryKeyCount=%d queryKeysTruncated=%t route=%s pathMode=%s statusCode=%d outcome=%s durationMs=%d xEmbyTokenCount=%d xEmbyTokenState=%s xMediaBrowserTokenCount=%d xMediaBrowserTokenState=%s xEmbyAuthorizationCount=%d xMediaBrowserAuthorizationCount=%d authorizationCount=%d applicationScheme=%s embeddedTokenState=%s apiKeyQueryPresent=%t queryTokenSourceCount=%d queryTokenState=%s userAgentFamily=%s userAgentVersion=%q",
		snapshot.method,
		snapshot.host,
		snapshot.path,
		snapshot.pathTruncated,
		snapshot.queryKeys,
		snapshot.queryKeyCount,
		snapshot.queryKeysTruncated,
		route,
		pathMode,
		statusCode,
		requestOutcome(statusCode),
		time.Since(startedAt).Milliseconds(),
		snapshot.xEmbyTokenCount,
		snapshot.xEmbyTokenState,
		snapshot.xMediaBrowserTokenCount,
		snapshot.xMediaBrowserTokenState,
		snapshot.xEmbyAuthorizationCount,
		snapshot.xMediaAuthorizationCount,
		snapshot.standardAuthorizationCount,
		snapshot.applicationScheme,
		snapshot.embeddedTokenState,
		snapshot.apiKeyQueryPresent,
		snapshot.queryTokenSourceCount,
		snapshot.queryTokenState,
		snapshot.userAgentFamily,
		snapshot.userAgentVersion,
	)
}

// debugf keeps request-shape and cache diagnostics injectable with the
// Gateway's existing logger while honoring the process-level Debug decision.
func (gateway *Gateway) debugf(format string, args ...interface{}) {
	if gateway != nil && gateway.debug {
		gateway.logger.Printf(format, args...)
	}
}

// requestQueryKeySummary logs sorted, bounded key names but never query values.
func requestQueryKeySummary(request *http.Request) (string, int, bool) {
	if request == nil || request.URL == nil || request.URL.RawQuery == "" {
		return "", 0, false
	}
	query := request.URL.Query()
	keys := make([]string, 0, len(query))
	for key := range query {
		boundedKey, _ := boundedRequestLogValue(key, maxLoggedQueryKeyBytes)
		keys = append(keys, boundedKey)
	}
	sort.Strings(keys)
	keyCount := len(keys)
	truncated := keyCount > maxLoggedQueryKeys
	if truncated {
		keys = keys[:maxLoggedQueryKeys]
	}
	return strings.Join(keys, ","), keyCount, truncated
}

// queryTokenDiagnostics reports carrier shape without retaining or logging any
// query Token value.
func queryTokenDiagnostics(values url.Values) (int, string, bool) {
	sourceCount := 0
	apiKeyPresent := false
	for key, items := range values {
		canonicalKey, supported := canonicalTokenQueryKey(key)
		if !supported {
			continue
		}
		if canonicalKey == "api_key" {
			apiKeyPresent = true
		}
		if len(items) == 0 {
			sourceCount++
		} else {
			sourceCount += len(items)
		}
	}
	candidates, reasonCode := protectedQueryAccessTokenCandidates(values)
	if reasonCode != "" {
		return sourceCount, strings.TrimPrefix(reasonCode, "token_"), apiKeyPresent
	}
	if candidates.present {
		return sourceCount, "present", apiKeyPresent
	}
	return sourceCount, "missing", apiKeyPresent
}

// applicationAuthorizationDiagnostics reports only fixed scheme and Token
// presence labels, never the application authorization Header value.
func applicationAuthorizationDiagnostics(header http.Header) (string, string) {
	standardValues := header.Values(standardAuthorizationHeader)
	xEmbyValues := header.Values(embyAuthorizationHeader)
	xMediaValues := header.Values(mediaBrowserAuthorizationHeader)
	if len(xEmbyValues)+len(xMediaValues)+len(standardValues) == 0 {
		return "missing", "missing"
	}
	if len(xEmbyValues)+len(xMediaValues)+len(standardValues) != 1 {
		return "ambiguous", "ambiguous"
	}
	value, headerKind, ok := singleApplicationAuthorization(header)
	if !ok {
		return "other", "unparseable"
	}
	scheme := applicationAuthorizationSchemeCode(value)
	fields, ok := parseApplicationAuthorizationWithAccessToken(value, headerKind)
	if !ok {
		return scheme, "unparseable"
	}
	token, present := fields["Token"]
	if !present {
		return scheme, "missing"
	}
	if token == "" {
		return scheme, "empty"
	}
	return scheme, "present"
}

// applicationAuthorizationSchemeCode returns a fixed label instead of an
// upstream-controlled scheme string.
func applicationAuthorizationSchemeCode(value string) string {
	switch {
	case strings.HasPrefix(value, embyAuthorizationScheme):
		return "emby"
	case strings.HasPrefix(value, mediaBrowserAuthorizationScheme):
		return "media_browser"
	default:
		return "other"
	}
}

// tokenValueState distinguishes missing, empty, present and ambiguous Header
// shapes without exposing a token value.
func tokenValueState(values []string) string {
	switch {
	case len(values) == 0:
		return "missing"
	case len(values) != 1:
		return "ambiguous"
	case values[0] == "":
		return "empty"
	default:
		return "present"
	}
}

// userAgentDiagnostics exposes only a known client family and a strict version
// token; arbitrary User-Agent text is never logged.
func userAgentDiagnostics(value string) (string, string) {
	for _, candidate := range []struct {
		prefix string
		family string
	}{
		{prefix: "Infuse-Direct/", family: "infuse_direct"},
		{prefix: "Infuse-Library/", family: "infuse_library"},
		{prefix: "Infuse/", family: "infuse"},
		{prefix: "SenPlayer/", family: "senplayer"},
		{prefix: "Yamby/", family: "yamby"},
		{prefix: "VidHub/", family: "vidhub"},
		{prefix: "Fileball/", family: "fileball"},
		{prefix: "Conflux/", family: "conflux"},
		{prefix: "Emby/", family: "emby"},
	} {
		if !strings.HasPrefix(value, candidate.prefix) {
			continue
		}
		version := strings.TrimPrefix(value, candidate.prefix)
		if separator := strings.IndexAny(version, " \t"); separator >= 0 {
			version = version[:separator]
		}
		if len(version) == 0 || len(version) > maxLoggedUserAgentVersion || !isSafeUserAgentVersion(version) {
			return candidate.family, "invalid"
		}
		return candidate.family, version
	}
	if value == "" {
		return "missing", ""
	}
	return "other", ""
}

// isSafeUserAgentVersion accepts numeric version syntax only so arbitrary
// opaque User-Agent suffixes cannot be copied into logs as a "version".
func isSafeUserAgentVersion(value string) bool {
	if value == "" || value[0] < '0' || value[0] > '9' {
		return false
	}
	for index := 0; index < len(value); index++ {
		character := value[index]
		if character >= '0' && character <= '9' || character == '.' || character == '-' {
			continue
		}
		return false
	}
	return true
}

// boundedRequestLogValue prevents a single request from producing an
// unbounded log field. The returned text is always printed with %q.
func boundedRequestLogValue(value string, maxBytes int) (string, bool) {
	if maxBytes <= 0 || len(value) <= maxBytes {
		return value, false
	}
	return value[:maxBytes], true
}

// boundedRequestLogText is used when the caller does not need a truncation
// flag but still must prevent unbounded client-controlled log fields.
func boundedRequestLogText(value string, maxBytes int) string {
	bounded, _ := boundedRequestLogValue(value, maxBytes)
	return bounded
}

// requestOutcome maps the captured status to a fixed success/failure label.
func requestOutcome(statusCode int) string {
	switch {
	case statusCode >= 100 && statusCode < http.StatusBadRequest:
		return "success"
	case statusCode >= http.StatusBadRequest:
		return "failure"
	default:
		return "unknown"
	}
}
