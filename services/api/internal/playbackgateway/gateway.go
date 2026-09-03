// Package playbackgateway provides the HTTP transport core that sits between
// Emby clients and one version-pinned Emby upstream.
package playbackgateway

import (
	"bytes"
	"compress/flate"
	"compress/gzip"
	"compress/zlib"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"sync/atomic"
	"time"

	"github.com/andybalholm/brotli"
	logpkg "github.com/konghang/ember/backend/internal/logging"
	"github.com/konghang/ember/backend/internal/services/embytoken"
)

const (
	authenticationPath                   = "/emby/Users/AuthenticateByName"
	publicSystemInfoPath                 = "/emby/System/Info/Public"
	accessTokenHeader                    = "X-Emby-Token"
	statusClientClosedRequest            = 499
	defaultAuthenticationResponseMaxSize = int64(1 << 20)
	defaultLogLevelRefreshTimeout        = 500 * time.Millisecond
)

var (
	errSidecarResponseEncodingUnsupported = errors.New("sidecar response encoding unsupported")
	errSidecarResponseDecodeFailed        = errors.New("sidecar response decode failed")
	errSidecarResponseDecodedTooLarge     = errors.New("sidecar response decoded body too large")
)

// TokenService is the narrow identity boundary required by the HTTP gateway.
// Implementations must never expose a raw token in returned errors or logs.
type TokenService interface {
	RecordAuthenticationResult(context.Context, embytoken.AuthenticationResultInput) (embytoken.AuthenticationMapping, error)
	ResolvePrincipal(context.Context, string) (embytoken.Principal, error)
}

// WebSurfacePolicy resolves the short-cached database-backed Web UI switch at
// request boundaries so an admin update is visible without a restart.
type WebSurfacePolicy interface {
	PlaybackGatewayWebEnabled(context.Context) (bool, error)
}

// LogLevelPolicy resolves the short-cached database-only API/Gateway log level.
// Gateway calls it at request boundaries so a setting saved by the API process
// becomes visible without restarting either process.
type LogLevelPolicy interface {
	LogLevel(context.Context) (string, error)
}

// AuthenticationMetadata contains non-authoritative device information that
// may be persisted for audit and device-scoped local revocation.
type AuthenticationMetadata struct {
	DeviceID   string
	ClientName string
}

// Config contains only dependencies needed by the transport core. Process
// configuration, database construction and HTTP server startup belong to the
// gateway runtime selected by the unified ember entrypoint.
type Config struct {
	Upstream           *url.URL
	TokenService       TokenService
	DirectPlayService  DirectPlayService
	LocalMediaResolver LocalMediaResolver
	WebSurfacePolicy   WebSurfacePolicy
	LogLevelPolicy     LogLevelPolicy
	Transport          http.RoundTripper
	Logger             *log.Logger
	Debug              bool
	ApplyLogLevel      func(string) error
	DebugEnabled       func() bool
}

// Gateway validates mapped tokens before proxying protected requests and
// observes successful Emby 4.9 username/password authentication responses
// without changing the upstream response.
type Gateway struct {
	proxy                          *httputil.ReverseProxy
	upstream                       *url.URL
	transport                      http.RoundTripper
	tokenService                   TokenService
	directPlayService              DirectPlayService
	localMediaResolver             LocalMediaResolver
	webSurfacePolicy               WebSurfacePolicy
	logLevelPolicy                 LogLevelPolicy
	applyLogLevel                  func(string) error
	debugEnabled                   func() bool
	logLevelRefreshFailed          atomic.Bool
	logger                         *log.Logger
	proofs                         *playbackProofCache
	itemContainers                 *itemContainerSnapshotCache
	playbackInfoFlights            *onDemandPlaybackInfoFlightGroup
	playbackSessionFailures        *playbackSessionFailureTracker
	maxAuthenticationResponseBytes int64
	maxPlaybackInfoRequestBytes    int64
	maxPlaybackInfoResponseBytes   int64
	maxItemDetailResponseBytes     int64
	maxPlaybackSessionRequestBytes int64
}

type routeKind uint8

const (
	routeProtected routeKind = iota
	routeAuthentication
	routeSystemInfoPublic
	routePublicBootstrap
	routePlaybackInfo
	routeItemDetail
	routeVideo
	routeWebSurface
)

type requestRouteContext struct {
	kind                 routeKind
	pathMode             requestPathMode
	metadata             AuthenticationMetadata
	principal            *embytoken.Principal
	playbackInfoItemID   string
	playbackInfoEligible bool
	itemDetailItemID     string
	itemDetailEligible   bool
	videoDecision        *videoDecision
}

type requestRouteContextKey struct{}

type authenticationResult struct {
	User struct {
		ID string `json:"Id"`
	} `json:"User"`
	AccessToken string `json:"AccessToken"`
	ServerID    string `json:"ServerId"`
}

// New builds a gateway handler without starting a listener or making an
// upstream request. The upstream must be an absolute HTTP(S) base URL without
// credentials, query parameters or fragments.
func New(config Config) (*Gateway, error) {
	upstream, err := validatedUpstream(config.Upstream)
	if err != nil {
		return nil, err
	}
	if config.TokenService == nil {
		return nil, errors.New("playback gateway token service is required")
	}
	logger := config.Logger
	if logger == nil {
		logger = log.Default()
	}
	transport := config.Transport
	if transport == nil {
		transport = http.DefaultTransport
	}
	applyLogLevel := config.ApplyLogLevel
	if applyLogLevel == nil {
		applyLogLevel = logpkg.ApplyLevel
	}
	debugEnabled := config.DebugEnabled
	if debugEnabled == nil {
		fixedDebug := config.Debug
		debugEnabled = func() bool { return fixedDebug }
	}

	gateway := &Gateway{
		upstream:                       upstream,
		transport:                      transport,
		tokenService:                   config.TokenService,
		directPlayService:              config.DirectPlayService,
		localMediaResolver:             config.LocalMediaResolver,
		webSurfacePolicy:               config.WebSurfacePolicy,
		logLevelPolicy:                 config.LogLevelPolicy,
		applyLogLevel:                  applyLogLevel,
		debugEnabled:                   debugEnabled,
		logger:                         logger,
		proofs:                         newPlaybackProofCache(defaultPlaybackProofMaxEntries, defaultPlaybackProofTTL),
		itemContainers:                 newItemContainerSnapshotCache(defaultItemContainerSnapshotMaxEntries, defaultItemContainerSnapshotTTL),
		playbackInfoFlights:            &onDemandPlaybackInfoFlightGroup{},
		playbackSessionFailures:        newPlaybackSessionFailureTracker(defaultPlaybackSessionFailureMaxEntries, defaultPlaybackSessionFailureTTL),
		maxAuthenticationResponseBytes: defaultAuthenticationResponseMaxSize,
		maxPlaybackInfoRequestBytes:    defaultPlaybackInfoRequestMaxSize,
		maxPlaybackInfoResponseBytes:   defaultPlaybackInfoResponseMaxSize,
		maxItemDetailResponseBytes:     defaultItemDetailResponseMaxSize,
		maxPlaybackSessionRequestBytes: defaultPlaybackSessionRequestMaxSize,
	}
	proxy := httputil.NewSingleHostReverseProxy(upstream)
	proxy.Transport = transport
	proxy.ErrorLog = log.New(io.Discard, "", 0)
	proxy.ErrorHandler = gateway.handleUpstreamError
	proxy.ModifyResponse = gateway.observeResponse
	gateway.proxy = proxy
	return gateway, nil
}

// ServeHTTP normalizes versioned API paths, applies depth-exact bootstrap and
// authentication exceptions, then fails closed before any protected proxy.
func (gateway *Gateway) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	startedAt := time.Now().UTC()
	logLevelContext := context.Background()
	if request != nil {
		logLevelContext = request.Context()
	}
	gateway.refreshLogLevel(logLevelContext)
	requestLog := captureRequestLogSnapshot(request)
	statusWriter := &requestStatusWriter{ResponseWriter: writer}
	routeCode := "unclassified"
	pathMode := requestPathModePassthrough
	playbackSessionEvent := playbackSessionEventSnapshot{}
	playbackSessionForwardAttempted := false
	defer func() {
		gateway.logPlaybackSessionEvent(playbackSessionEvent, statusWriter.statusCode, playbackSessionForwardAttempted, startedAt)
		gateway.logRequestCompletion(requestLog, routeCode, pathMode, statusWriter.statusCode, startedAt)
	}()
	if isEmbyWebSurfaceRequest(request) {
		routeCode = routeKindCode(routeWebSurface)
		gateway.serveWebSurface(statusWriter, request)
		return
	}

	var pathOK bool
	pathMode, pathOK = normalizeEmbyAPIPath(request)
	if !pathOK {
		gateway.logger.Printf("[PlaybackGateway] code=request_path_invalid pathMode=%s", pathMode)
		statusWriter.WriteHeader(http.StatusBadRequest)
		return
	}
	kind := classifyRoute(request)
	routeCode = routeKindCode(kind)
	playbackSessionEvent = newPlaybackSessionEventSnapshot(request)
	routeContext := requestRouteContext{kind: kind, pathMode: pathMode}
	requestAccessToken := ""
	switch kind {
	case routeSystemInfoPublic:
		// PublicSystemInfo is the exact pre-login discovery endpoint. Emby is
		// authoritative for its response and no local identity exists yet.
	case routeAuthentication:
		metadata, metadataCarrier, ok := extractAuthenticationApplicationMetadata(request)
		if !ok {
			gateway.logger.Printf("[PlaybackGateway] code=%s route=%s pathMode=%s", invalidApplicationMetadataLogCode(metadataCarrier), routeKindCode(kind), pathMode)
			statusWriter.WriteHeader(http.StatusUnauthorized)
			return
		}
		if _, reasonCode, tokenPresent := extractProtectedRequestAccessToken(request); tokenPresent || reasonCode != "token_missing" {
			if tokenPresent {
				reasonCode = "token_present"
			}
			gateway.logger.Printf("[PlaybackGateway] code=authentication_token_invalid reasonCode=%s", reasonCode)
			statusWriter.WriteHeader(http.StatusUnauthorized)
			return
		}
		routeContext.metadata = metadata
	case routePublicBootstrap:
		metadata, metadataCarrier, metadataOK := extractPublicBootstrapApplicationMetadata(request)
		// The target Web query bundle is proven only for the exact public user
		// list. Public images keep the existing application Header contract.
		if metadataCarrier == applicationMetadataQuery && !exactRequestPathFold(request.URL, publicUsersPath) {
			metadataOK = false
		}
		if metadataCarrier != applicationMetadataNone && !metadataOK {
			gateway.logger.Printf("[PlaybackGateway] code=%s route=%s pathMode=%s", invalidApplicationMetadataLogCode(metadataCarrier), routeKindCode(kind), pathMode)
			statusWriter.WriteHeader(http.StatusUnauthorized)
			return
		}
		accessToken, reasonCode, tokenOK := extractProtectedRequestAccessToken(request)
		if tokenOK {
			principal, resolved := gateway.resolveRequestPrincipal(statusWriter, request, kind, startedAt, accessToken)
			if !resolved {
				return
			}
			routeContext.principal = &principal
			if metadataOK {
				routeContext.metadata = metadata
			}
			break
		}
		if reasonCode != "token_missing" {
			gateway.logger.Printf("[PlaybackGateway] code=token_header_invalid route=%s pathMode=%s reasonCode=%s", routeKindCode(kind), pathMode, reasonCode)
			statusWriter.WriteHeader(http.StatusUnauthorized)
			return
		}
		if metadataOK {
			routeContext.metadata = metadata
			break
		}
		fallthrough
	default:
		accessToken, reasonCode, ok := extractProtectedRequestAccessToken(request)
		if !ok {
			if kind == routeVideo {
				gateway.rejectVideo(statusWriter, request, http.StatusUnauthorized, "identity", reasonCode, startedAt)
				return
			}
			gateway.logger.Printf("[PlaybackGateway] code=token_header_invalid route=%s pathMode=%s reasonCode=%s", routeKindCode(kind), pathMode, reasonCode)
			statusWriter.WriteHeader(http.StatusUnauthorized)
			return
		}
		if playbackSessionEvent.kind != playbackSessionEventNone {
			playbackSessionEvent.correlationKey, playbackSessionEvent.correlationPresent =
				gateway.playbackSessionFailures.CorrelationKey(accessToken)
		}
		principal, resolved := gateway.resolveRequestPrincipal(statusWriter, request, kind, startedAt, accessToken)
		if !resolved {
			return
		}
		routeContext.principal = &principal
		requestAccessToken = accessToken
		if playbackSessionEvent.kind != playbackSessionEventNone {
			observed := gateway.inspectPlaybackSessionRequest(request)
			observed.correlationKey = playbackSessionEvent.correlationKey
			observed.correlationPresent = playbackSessionEvent.correlationPresent
			playbackSessionEvent = observed
		} else if kind == routePlaybackInfo {
			routeContext.playbackInfoItemID, routeContext.playbackInfoEligible = gateway.preparePlaybackInfoRequest(request, principal)
		} else if kind == routeItemDetail {
			userID, itemID, pathOK := userItemDetailPath(request.URL)
			routeContext.itemDetailItemID = itemID
			routeContext.itemDetailEligible = pathOK && userID == principal.User.EmbyID && principal.MappingID != ""
		}
	}
	if kind == routeVideo {
		gateway.serveVideo(statusWriter, request, *routeContext.principal, requestAccessToken, startedAt)
		return
	}

	ctx := context.WithValue(request.Context(), requestRouteContextKey{}, routeContext)
	playbackSessionForwardAttempted = playbackSessionEvent.kind != playbackSessionEventNone
	gateway.proxy.ServeHTTP(statusWriter, request.WithContext(ctx))
}

// refreshLogLevel applies the current runtime policy value without making
// logging availability part of the HTTP availability contract. The production
// policy bounds database refreshes with a short cache; consecutive failures
// emit one transition log and retain the last valid runtime level.
func (gateway *Gateway) refreshLogLevel(ctx context.Context) {
	if gateway == nil || gateway.logLevelPolicy == nil || gateway.applyLogLevel == nil {
		return
	}
	refreshContext, cancel := context.WithTimeout(ctx, defaultLogLevelRefreshTimeout)
	defer cancel()
	level, err := gateway.logLevelPolicy.LogLevel(refreshContext)
	if err == nil {
		err = gateway.applyLogLevel(level)
	}
	if err != nil {
		if !gateway.logLevelRefreshFailed.Swap(true) {
			gateway.logger.Printf("[Logging] level=info code=log_level_refresh_failed processRole=gateway errorType=%T", err)
		}
		return
	}
	if gateway.logLevelRefreshFailed.Swap(false) {
		gateway.logger.Printf("[Logging] level=info code=log_level_refresh_recovered processRole=gateway")
	}
}

// resolveRequestPrincipal applies one shared cancellation, video rejection and
// ordinary API error mapping after a Token carrier has been normalized.
func (gateway *Gateway) resolveRequestPrincipal(
	writer http.ResponseWriter,
	request *http.Request,
	kind routeKind,
	startedAt time.Time,
	accessToken string,
) (embytoken.Principal, bool) {
	principal, err := gateway.tokenService.ResolvePrincipal(request.Context(), accessToken)
	if err == nil {
		return principal, true
	}
	if status, code, terminated := tokenRequestTermination(err); terminated {
		if kind == routeVideo {
			gateway.rejectVideo(writer, request, status, "identity", strings.TrimPrefix(code, "token_"), startedAt)
			return embytoken.Principal{}, false
		}
		if code != "token_request_canceled" {
			gateway.logger.Printf("[PlaybackGateway] code=%s errorType=%T", code, err)
		}
		writer.WriteHeader(status)
		return embytoken.Principal{}, false
	}
	if kind == routeVideo {
		status, stage, reasonCode := videoPrincipalRejection(err)
		gateway.rejectVideo(writer, request, status, stage, reasonCode, startedAt)
		return embytoken.Principal{}, false
	}
	status, code := tokenRejection(err)
	gateway.logger.Printf("[PlaybackGateway] code=%s errorType=%T", code, err)
	writer.WriteHeader(status)
	return embytoken.Principal{}, false
}

// observeResponse dispatches sidecar observation by the already classified
// request route while preserving ReverseProxy's original response semantics.
func (gateway *Gateway) observeResponse(response *http.Response) error {
	routeContext, ok := routeContextFromResponse(response)
	if !ok {
		return nil
	}
	switch routeContext.kind {
	case routeAuthentication:
		return gateway.observeAuthenticationResponse(response)
	case routeSystemInfoPublic:
		gateway.logSystemInfoPublicResponse(response, routeContext)
		return nil
	case routePlaybackInfo:
		return gateway.observePlaybackInfoResponse(response, routeContext)
	case routeItemDetail:
		return gateway.observeUserItemDetailResponse(response, routeContext)
	case routeVideo:
		gateway.observeVideoFallbackResponse(response, routeContext)
		return nil
	default:
		return nil
	}
}

// logSystemInfoPublicResponse records only the upstream status and fixed route
// metadata so production can distinguish local discovery failures from Emby.
func (gateway *Gateway) logSystemInfoPublicResponse(response *http.Response, routeContext requestRouteContext) {
	if response == nil {
		return
	}
	gateway.logger.Printf(
		"[PlaybackGateway] code=bootstrap_upstream_response route=system_info_public pathMode=%s statusCode=%d",
		routeContext.pathMode,
		response.StatusCode,
	)
}

// observeAuthenticationResponse buffers only the bounded successful login
// response needed for token mapping, then restores the exact bytes before the
// reverse proxy writes them to the client. Every sidecar failure returns nil so
// Emby's original successful response remains authoritative.
func (gateway *Gateway) observeAuthenticationResponse(response *http.Response) error {
	routeContext, ok := routeContextFromResponse(response)
	if !ok || routeContext.kind != routeAuthentication || response.StatusCode != http.StatusOK {
		return nil
	}
	if response.Body == nil {
		gateway.logger.Printf("[PlaybackGateway] code=authentication_response_invalid errorType=nil_body")
		return nil
	}

	originalBody := response.Body
	prefix, readErr := io.ReadAll(io.LimitReader(originalBody, gateway.maxAuthenticationResponseBytes+1))
	response.Body = &replayedBody{
		Reader: io.MultiReader(bytes.NewReader(prefix), originalBody),
		closer: originalBody,
	}
	if readErr != nil {
		gateway.logger.Printf("[PlaybackGateway] code=authentication_response_read_failed errorType=%T", readErr)
		return nil
	}
	if int64(len(prefix)) > gateway.maxAuthenticationResponseBytes {
		gateway.logger.Printf("[PlaybackGateway] code=authentication_response_too_large")
		return nil
	}

	decodedPrefix, decodeErr := decodeResponseSidecar(prefix, response.Header.Get("Content-Encoding"), gateway.maxAuthenticationResponseBytes)
	if decodeErr != nil {
		gateway.logger.Printf(
			"[PlaybackGateway] code=authentication_response_decode_failed contentEncoding=%s reasonCode=%s errorType=%T",
			responseSidecarEncodingCode(response.Header.Get("Content-Encoding")),
			responseSidecarDecodeReasonCode(decodeErr),
			decodeErr,
		)
		return nil
	}

	var result authenticationResult
	if err := json.Unmarshal(decodedPrefix, &result); err != nil || !validAuthenticationResult(result) {
		gateway.logger.Printf("[PlaybackGateway] code=authentication_response_invalid errorType=%T", err)
		return nil
	}
	_, err := gateway.tokenService.RecordAuthenticationResult(response.Request.Context(), embytoken.AuthenticationResultInput{
		ServerID:    result.ServerID,
		EmbyUserID:  result.User.ID,
		AccessToken: result.AccessToken,
		DeviceID:    routeContext.metadata.DeviceID,
		ClientName:  routeContext.metadata.ClientName,
	})
	if err != nil {
		gateway.logger.Printf("[PlaybackGateway] code=authentication_mapping_failed errorType=%T", err)
	}
	return nil
}

// decodeResponseSidecar decodes only a bounded observation copy. The original
// response body and Content-Encoding stay intact for the client.
func decodeResponseSidecar(body []byte, contentEncoding string, maxBytes int64) ([]byte, error) {
	switch strings.ToLower(strings.TrimSpace(contentEncoding)) {
	case "", "identity":
		return body, nil
	case "gzip":
		return decodeGzipResponseSidecar(body, maxBytes)
	case "deflate":
		return decodeDeflateResponseSidecar(body, maxBytes)
	case "br":
		return decodeBrotliResponseSidecar(body, maxBytes)
	default:
		return nil, errSidecarResponseEncodingUnsupported
	}
}

// decodeGzipResponseSidecar decodes a gzip sidecar while keeping the
// compressed response bytes untouched for the downstream client.
func decodeGzipResponseSidecar(body []byte, maxBytes int64) ([]byte, error) {
	gzipReader, err := gzip.NewReader(bytes.NewReader(body))
	if err != nil {
		return nil, errSidecarResponseDecodeFailed
	}
	defer gzipReader.Close()
	return readBoundedResponseSidecar(gzipReader, maxBytes)
}

// decodeDeflateResponseSidecar accepts the zlib-wrapped HTTP deflate
// form and the legacy raw DEFLATE form while applying the same decoded limit.
func decodeDeflateResponseSidecar(body []byte, maxBytes int64) ([]byte, error) {
	zlibReader, err := zlib.NewReader(bytes.NewReader(body))
	if err == nil {
		defer zlibReader.Close()
		return readBoundedResponseSidecar(zlibReader, maxBytes)
	}

	rawReader := flate.NewReader(bytes.NewReader(body))
	defer rawReader.Close()
	return readBoundedResponseSidecar(rawReader, maxBytes)
}

// decodeBrotliResponseSidecar reads only the bounded observation copy used by
// browser authentication and other JSON sidecars. The downstream still
// receives the original Brotli bytes and Content-Encoding header.
func decodeBrotliResponseSidecar(body []byte, maxBytes int64) ([]byte, error) {
	return readBoundedResponseSidecar(brotli.NewReader(bytes.NewReader(body)), maxBytes)
}

// readBoundedResponseSidecar prevents compressed responses from
// expanding beyond the sidecar inspection limit.
func readBoundedResponseSidecar(reader io.Reader, maxBytes int64) ([]byte, error) {
	if reader == nil || maxBytes <= 0 {
		return nil, errSidecarResponseDecodeFailed
	}
	decoded, err := io.ReadAll(io.LimitReader(reader, maxBytes+1))
	if err != nil {
		return nil, errSidecarResponseDecodeFailed
	}
	if int64(len(decoded)) > maxBytes {
		return nil, errSidecarResponseDecodedTooLarge
	}
	return decoded, nil
}

// responseSidecarEncodingCode limits logs to fixed encoding labels.
func responseSidecarEncodingCode(contentEncoding string) string {
	switch strings.ToLower(strings.TrimSpace(contentEncoding)) {
	case "", "identity":
		return "identity"
	case "gzip":
		return "gzip"
	case "deflate":
		return "deflate"
	case "br":
		return "br"
	default:
		return "unsupported"
	}
}

// responseSidecarDecodeReasonCode maps decoder failures without
// exposing compressed bytes or upstream-controlled Header text.
func responseSidecarDecodeReasonCode(err error) string {
	switch {
	case errors.Is(err, errSidecarResponseEncodingUnsupported):
		return "encoding_unsupported"
	case errors.Is(err, errSidecarResponseDecodedTooLarge):
		return "decoded_body_too_large"
	default:
		return "decode_failed"
	}
}

// LookupPlaybackProof returns one exact, non-expired in-process proof and
// requires the fresh Principal returned by ResolvePrincipal rather than a raw
// MappingID string.
func (gateway *Gateway) LookupPlaybackProof(principal embytoken.Principal, itemID, mediaSourceID, playSessionID string) (PlaybackProof, bool) {
	proof, reasonCode := gateway.lookupPlaybackProof(principal, itemID, mediaSourceID, playSessionID)
	return proof, reasonCode == ""
}

// lookupPlaybackProof retains the public bool lookup while giving the video
// decision path a stable missing, expired or identity-mismatch reason.
func (gateway *Gateway) lookupPlaybackProof(principal embytoken.Principal, itemID, mediaSourceID, playSessionID string) (PlaybackProof, string) {
	if gateway == nil || gateway.proofs == nil {
		return PlaybackProof{}, "playback_proof_missing"
	}
	proof, status := gateway.proofs.lookup(principal.MappingID, itemID, mediaSourceID, playSessionID)
	if status == playbackProofExpired {
		return PlaybackProof{}, "playback_proof_expired"
	}
	if status != playbackProofFound {
		return PlaybackProof{}, "playback_proof_missing"
	}
	if !playbackProofMatchesPrincipal(proof, principal) {
		return PlaybackProof{}, "playback_proof_mismatch"
	}
	return proof, ""
}

// playbackProofMatchesPrincipal prevents a cached session from crossing the
// current Emby server, Ember user, Emby user or device identity boundary.
func playbackProofMatchesPrincipal(proof PlaybackProof, principal embytoken.Principal) bool {
	return proof.MappingID == principal.MappingID && proof.ServerID == principal.ServerID &&
		proof.UserID == principal.User.ID && proof.EmbyUserID == principal.User.EmbyID &&
		proof.DeviceID == principal.DeviceID
}

// handleUpstreamError returns a fixed transport status and logs only the Go
// error type. Upstream URLs, credentials and response bodies are never logged.
func (gateway *Gateway) handleUpstreamError(writer http.ResponseWriter, request *http.Request, err error) {
	if routeContext, ok := routeContextFromRequest(request); ok && routeContext.kind == routeVideo && routeContext.videoDecision != nil {
		decision := *routeContext.videoDecision
		decision.StatusCode = http.StatusBadGateway
		decision.ProxyErrorCode = "upstream_unavailable"
		gateway.logVideoDecision(decision)
		writer.WriteHeader(http.StatusBadGateway)
		return
	}
	gateway.logger.Printf("[PlaybackGateway] code=upstream_unavailable errorType=%T", err)
	writer.WriteHeader(http.StatusBadGateway)
}

// Exact method/depth matching with case-insensitive semantic segments supports
// client casing differences without allowing suffix or trailing-slash bypass.
func classifyRoute(request *http.Request) routeKind {
	if request == nil || request.URL == nil {
		return routeProtected
	}
	if request.Method == http.MethodPost && exactRequestPathFold(request.URL, authenticationPath) {
		return routeAuthentication
	}
	if request.Method == http.MethodGet && exactRequestPathFold(request.URL, publicSystemInfoPath) {
		return routeSystemInfoPublic
	}
	if request.Method == http.MethodGet && exactRequestPathFold(request.URL, publicUsersPath) {
		return routePublicBootstrap
	}
	if (request.Method == http.MethodGet || request.Method == http.MethodHead) && isPublicUserImagePath(request.URL) {
		return routePublicBootstrap
	}
	if (request.Method == http.MethodGet || request.Method == http.MethodPost) && playbackInfoItemID(request.URL) != "" {
		return routePlaybackInfo
	}
	if request.Method == http.MethodGet {
		if _, _, ok := userItemDetailPath(request.URL); ok {
			return routeItemDetail
		}
	}
	if (request.Method == http.MethodGet || request.Method == http.MethodHead) && videoPath(request.URL).ItemID != "" {
		return routeVideo
	}
	return routeProtected
}

// exactRequestPathFold accepts client casing differences but rejects alternate
// escaping, suffixes and depth changes before granting a special route class.
func exactRequestPathFold(requestURL *url.URL, expected string) bool {
	return requestURL != nil && requestURL.EscapedPath() == requestURL.Path && strings.EqualFold(requestURL.Path, expected)
}

// isPublicUserImagePath accepts only the no-index GET/HEAD shape referenced by
// the version-pinned login documentation. Mutation and deeper image paths stay
// protected.
func isPublicUserImagePath(requestURL *url.URL) bool {
	if requestURL == nil || requestURL.EscapedPath() != requestURL.Path {
		return false
	}
	segments := strings.Split(requestURL.Path, "/")
	if len(segments) != 6 || segments[0] != "" || !strings.EqualFold(segments[1], "emby") || !strings.EqualFold(segments[2], "Users") ||
		!strings.EqualFold(segments[4], "Images") {
		return false
	}
	return validPublicPathSegment(segments[3]) && validPublicPathSegment(segments[5])
}

// validPublicPathSegment bounds the two dynamic route segments and rejects dot
// semantics instead of relying on proxy or upstream path normalization.
func validPublicPathSegment(value string) bool {
	return value != "" && value != "." && value != ".." && len(value) <= 128
}

// tokenRequestTermination distinguishes an abandoned client request from an
// actual identity-store outage before generic token rejection mapping.
func tokenRequestTermination(err error) (int, string, bool) {
	switch {
	case errors.Is(err, context.Canceled):
		return statusClientClosedRequest, "token_request_canceled", true
	case errors.Is(err, context.DeadlineExceeded):
		return http.StatusGatewayTimeout, "token_request_deadline_exceeded", true
	default:
		return 0, "", false
	}
}

// tokenRejection separates invalid login state, dynamic user policy and
// infrastructure failures without exposing the underlying reason text.
func tokenRejection(err error) (int, string) {
	switch {
	case errors.Is(err, embytoken.ErrInvalidInput),
		errors.Is(err, embytoken.ErrTokenNotFound),
		errors.Is(err, embytoken.ErrTokenRevoked),
		errors.Is(err, embytoken.ErrIdentityMismatch):
		return http.StatusUnauthorized, "token_rejected"
	case errors.Is(err, embytoken.ErrUserUnavailable), errors.Is(err, embytoken.ErrUserExpired):
		return http.StatusForbidden, "user_rejected"
	default:
		return http.StatusServiceUnavailable, "token_store_unavailable"
	}
}

// routeContextFromResponse recovers classification metadata carried by the
// exact outbound request that produced this upstream response.
func routeContextFromResponse(response *http.Response) (requestRouteContext, bool) {
	if response == nil || response.Request == nil {
		return requestRouteContext{}, false
	}
	value, ok := response.Request.Context().Value(requestRouteContextKey{}).(requestRouteContext)
	return value, ok
}

// routeContextFromRequest recovers the immutable routing decision used by the
// reverse proxy error path when no upstream response exists.
func routeContextFromRequest(request *http.Request) (requestRouteContext, bool) {
	if request == nil {
		return requestRouteContext{}, false
	}
	value, ok := request.Context().Value(requestRouteContextKey{}).(requestRouteContext)
	return value, ok
}

// validAuthenticationResult checks only fields fixed by the Emby 4.9 contract;
// detailed bounds and identity rules remain owned by EmbyTokenService.
func validAuthenticationResult(result authenticationResult) bool {
	return strings.TrimSpace(result.User.ID) != "" && result.AccessToken != "" && strings.TrimSpace(result.ServerID) != ""
}

// validatedUpstream copies and validates the proxy target so later caller
// mutations and URL-embedded credentials cannot alter the gateway boundary.
func validatedUpstream(upstream *url.URL) (*url.URL, error) {
	if upstream == nil {
		return nil, errors.New("playback gateway upstream is required")
	}
	copy := *upstream
	if (copy.Scheme != "http" && copy.Scheme != "https") || copy.Host == "" || copy.User != nil ||
		copy.RawQuery != "" || copy.Fragment != "" {
		return nil, errors.New("playback gateway upstream is invalid")
	}
	return &copy, nil
}

type replayedBody struct {
	io.Reader
	closer io.Closer
}

// Close releases the original upstream response body after replay completes.
func (body *replayedBody) Close() error {
	return body.closer.Close()
}
