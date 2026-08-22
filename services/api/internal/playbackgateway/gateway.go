// Package playbackgateway provides the HTTP transport core that sits between
// Emby clients and one version-pinned Emby upstream.
package playbackgateway

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"time"

	"github.com/konghang/ember/backend/internal/services/embytoken"
)

const (
	authenticationPath                   = "/emby/Users/AuthenticateByName"
	accessTokenHeader                    = "X-Emby-Token"
	defaultAuthenticationResponseMaxSize = int64(1 << 20)
)

// TokenService is the narrow identity boundary required by the HTTP gateway.
// Implementations must never expose a raw token in returned errors or logs.
type TokenService interface {
	RecordAuthenticationResult(context.Context, embytoken.AuthenticationResultInput) (embytoken.AuthenticationMapping, error)
	ResolvePrincipal(context.Context, string) (embytoken.Principal, error)
}

// AuthenticationMetadata contains non-authoritative device information that
// may be persisted for audit and device-scoped local revocation.
type AuthenticationMetadata struct {
	DeviceID   string
	ClientName string
}

// Config contains only dependencies needed by the transport core. Process
// configuration, database construction and HTTP server startup belong to the
// later cmd/playback-gateway composition layer.
type Config struct {
	Upstream          *url.URL
	TokenService      TokenService
	DirectPlayService DirectPlayService
	Transport         http.RoundTripper
	Logger            *log.Logger
}

// Gateway validates mapped tokens before proxying protected requests and
// observes successful Emby 4.9.3.0 username/password authentication responses
// without changing the upstream response.
type Gateway struct {
	proxy                          *httputil.ReverseProxy
	tokenService                   TokenService
	directPlayService              DirectPlayService
	logger                         *log.Logger
	proofs                         *playbackProofCache
	maxAuthenticationResponseBytes int64
	maxPlaybackInfoRequestBytes    int64
	maxPlaybackInfoResponseBytes   int64
}

type routeKind uint8

const (
	routeProtected routeKind = iota
	routeAuthentication
	routePublicBootstrap
	routePlaybackInfo
	routeVideo
)

type requestRouteContext struct {
	kind                 routeKind
	metadata             AuthenticationMetadata
	principal            *embytoken.Principal
	playbackInfoItemID   string
	playbackInfoEligible bool
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

	gateway := &Gateway{
		tokenService:                   config.TokenService,
		directPlayService:              config.DirectPlayService,
		logger:                         logger,
		proofs:                         newPlaybackProofCache(defaultPlaybackProofMaxEntries, defaultPlaybackProofTTL),
		maxAuthenticationResponseBytes: defaultAuthenticationResponseMaxSize,
		maxPlaybackInfoRequestBytes:    defaultPlaybackInfoRequestMaxSize,
		maxPlaybackInfoResponseBytes:   defaultPlaybackInfoResponseMaxSize,
	}
	proxy := httputil.NewSingleHostReverseProxy(upstream)
	proxy.Transport = transport
	proxy.ErrorLog = log.New(io.Discard, "", 0)
	proxy.ErrorHandler = gateway.handleUpstreamError
	proxy.ModifyResponse = gateway.observeResponse
	gateway.proxy = proxy
	return gateway, nil
}

// ServeHTTP applies the exact-route authentication exception and fails closed
// for every other request before allowing the reverse proxy to reach Emby.
func (gateway *Gateway) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	startedAt := time.Now().UTC()
	kind := classifyRoute(request)
	routeContext := requestRouteContext{kind: kind}
	switch kind {
	case routeAuthentication, routePublicBootstrap:
		metadata, ok := extractApplicationMetadata(request.Header)
		if !ok {
			gateway.logger.Printf("[PlaybackGateway] code=application_header_invalid")
			writer.WriteHeader(http.StatusUnauthorized)
			return
		}
		routeContext.metadata = metadata
	default:
		accessToken, ok := singleAccessToken(request.Header)
		if !ok {
			if kind == routeVideo {
				status, reasonCode := videoTokenHeaderRejection(request.Header)
				gateway.rejectVideo(writer, request, status, "identity", reasonCode, startedAt)
				return
			}
			gateway.logger.Printf("[PlaybackGateway] code=token_header_invalid")
			writer.WriteHeader(http.StatusUnauthorized)
			return
		}
		principal, err := gateway.tokenService.ResolvePrincipal(request.Context(), accessToken)
		if err != nil {
			if kind == routeVideo {
				status, stage, reasonCode := videoPrincipalRejection(err)
				gateway.rejectVideo(writer, request, status, stage, reasonCode, startedAt)
				return
			}
			status, code := tokenRejection(err)
			gateway.logger.Printf("[PlaybackGateway] code=%s errorType=%T", code, err)
			writer.WriteHeader(status)
			return
		}
		routeContext.principal = &principal
		if kind == routePlaybackInfo {
			routeContext.playbackInfoItemID, routeContext.playbackInfoEligible = gateway.preparePlaybackInfoRequest(request, principal)
		}
	}
	if kind == routeVideo {
		gateway.serveVideo(writer, request, *routeContext.principal, startedAt)
		return
	}

	ctx := context.WithValue(request.Context(), requestRouteContextKey{}, routeContext)
	gateway.proxy.ServeHTTP(writer, request.WithContext(ctx))
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
	case routePlaybackInfo:
		return gateway.observePlaybackInfoResponse(response, routeContext)
	case routeVideo:
		gateway.observeVideoFallbackResponse(response, routeContext)
		return nil
	default:
		return nil
	}
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

	var result authenticationResult
	if err := json.Unmarshal(prefix, &result); err != nil || !validAuthenticationResult(result) {
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
	if proof.ServerID != principal.ServerID || proof.UserID != principal.User.ID ||
		proof.EmbyUserID != principal.User.EmbyID || proof.DeviceID != principal.DeviceID {
		return PlaybackProof{}, "playback_proof_mismatch"
	}
	return proof, ""
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

// Exact method and path matching prevents case, suffix or trailing-slash
// variants from bypassing token validation.
func classifyRoute(request *http.Request) routeKind {
	if request == nil || request.URL == nil {
		return routeProtected
	}
	if request.Method == http.MethodPost && exactRequestPath(request.URL, authenticationPath) {
		return routeAuthentication
	}
	if request.Method == http.MethodGet && exactRequestPath(request.URL, publicUsersPath) {
		return routePublicBootstrap
	}
	if (request.Method == http.MethodGet || request.Method == http.MethodHead) && isPublicUserImagePath(request.URL) {
		return routePublicBootstrap
	}
	if (request.Method == http.MethodGet || request.Method == http.MethodPost) && playbackInfoItemID(request.URL) != "" {
		return routePlaybackInfo
	}
	if (request.Method == http.MethodGet || request.Method == http.MethodHead) && videoPath(request.URL).ItemID != "" {
		return routeVideo
	}
	return routeProtected
}

// exactRequestPath rejects alternate escaping even when net/url decodes it to
// the same Path, preventing an encoded route from inheriting a public class.
func exactRequestPath(requestURL *url.URL, expected string) bool {
	return requestURL != nil && requestURL.Path == expected && requestURL.EscapedPath() == expected
}

// isPublicUserImagePath accepts only the no-index GET/HEAD shape referenced by
// the version-pinned login documentation. Mutation and deeper image paths stay
// protected.
func isPublicUserImagePath(requestURL *url.URL) bool {
	if requestURL == nil || requestURL.EscapedPath() != requestURL.Path {
		return false
	}
	segments := strings.Split(requestURL.Path, "/")
	if len(segments) != 6 || segments[0] != "" || segments[1] != "emby" || segments[2] != "Users" ||
		segments[4] != "Images" {
		return false
	}
	return validPublicPathSegment(segments[3]) && validPublicPathSegment(segments[5])
}

// validPublicPathSegment bounds the two dynamic route segments and rejects dot
// semantics instead of relying on proxy or upstream path normalization.
func validPublicPathSegment(value string) bool {
	return value != "" && value != "." && value != ".." && len(value) <= 128
}

// singleAccessToken accepts exactly one header value and preserves the opaque
// token bytes for the purpose-separated hasher to validate.
func singleAccessToken(header http.Header) (string, bool) {
	values := header.Values(accessTokenHeader)
	returnValue := ""
	if len(values) == 1 {
		returnValue = values[0]
	}
	return returnValue, len(values) == 1 && returnValue != ""
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

// validAuthenticationResult checks only fields fixed by the 4.9.3.0 contract;
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
