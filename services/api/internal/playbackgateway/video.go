package playbackgateway

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/konghang/ember/backend/internal/services/directplay"
	"github.com/konghang/ember/backend/internal/services/embytoken"
)

const maxVideoStreamFileNameBytes = 1024

// DirectPlayService is the narrow acceleration boundary used by the Gateway.
// Any returned error affects only optional 115 acceleration and must not decide
// whether a valid Principal may use normal Emby playback.
type DirectPlayService interface {
	ResolveMediaPath(context.Context, directplay.MediaPathResolveRequest) (directplay.RedirectCandidate, error)
}

type videoPathInfo struct {
	ItemID         string
	StreamFileName string
}

type videoRequestInfo struct {
	ItemID         string
	MediaSourceID  string
	PlaySessionID  string
	Container      string
	Accelerated    bool
	FallbackStage  string
	FallbackReason string
}

type videoDecision struct {
	Decision       string
	Stage          string
	ReasonCode     string
	Method         string
	UserID         string
	MappingID      string
	DeviceID       string
	ClientName     string
	ItemID         string
	MediaSourceID  string
	PlaySessionID  string
	TaskID         string
	Preexisting    bool
	StatusCode     int
	UpstreamStatus int
	ProxyErrorCode string
	StartedAt      time.Time
}

// serveVideo chooses exactly one of reject, redirect or transparent Emby
// fallback after ServeHTTP has already resolved a fresh local Principal.
func (gateway *Gateway) serveVideo(writer http.ResponseWriter, request *http.Request, principal embytoken.Principal, startedAt time.Time) {
	info := inspectVideoRequest(request)
	decision := newVideoDecision(request, info, &principal, startedAt)
	if !info.Accelerated {
		decision.Stage = info.FallbackStage
		decision.ReasonCode = info.FallbackReason
		gateway.proxyVideoFallback(writer, request, decision, principal)
		return
	}

	proof, proofReason := gateway.lookupPlaybackProof(principal, info.ItemID, info.MediaSourceID, info.PlaySessionID)
	if proofReason != "" {
		decision.Stage = "proof"
		decision.ReasonCode = proofReason
		gateway.proxyVideoFallback(writer, request, decision, principal)
		return
	}
	if proof.Container != "" && info.Container != "" && !strings.EqualFold(proof.Container, info.Container) {
		decision.Stage = "eligibility"
		decision.ReasonCode = "media_not_direct_play"
		gateway.proxyVideoFallback(writer, request, decision, principal)
		return
	}
	if gateway.directPlayService == nil {
		decision.Stage = "eligibility"
		decision.ReasonCode = "direct_play_disabled"
		gateway.proxyVideoFallback(writer, request, decision, principal)
		return
	}

	candidate, err := gateway.directPlayService.ResolveMediaPath(request.Context(), directplay.MediaPathResolveRequest{
		Path: proof.Path, Size: proof.Size, ClientUserAgent: request.UserAgent(),
	})
	if err != nil {
		decision.Stage = "direct_play"
		decision.ReasonCode = directPlayReasonCode(err)
		gateway.proxyVideoFallback(writer, request, decision, principal)
		return
	}
	if !validRedirectCandidate(candidate) {
		decision.Stage = "direct_play"
		decision.ReasonCode = "provider_protocol"
		gateway.proxyVideoFallback(writer, request, decision, principal)
		return
	}

	decision.Decision = "redirect"
	decision.Stage = "direct_play"
	decision.ReasonCode = "direct_play_ready"
	decision.TaskID = candidate.TaskID
	decision.Preexisting = candidate.Preexisting
	decision.StatusCode = http.StatusFound
	writer.Header().Set("Location", candidate.URL)
	writer.Header().Set("Cache-Control", "no-store")
	writer.WriteHeader(http.StatusFound)
	gateway.logVideoDecision(decision)
}

// proxyVideoFallback preserves the original request object and delegates it to
// the existing reverse proxy. The response/error hooks own the single final log.
func (gateway *Gateway) proxyVideoFallback(
	writer http.ResponseWriter,
	request *http.Request,
	decision videoDecision,
	principal embytoken.Principal,
) {
	decision.Decision = "fallback"
	routeContext := requestRouteContext{kind: routeVideo, principal: &principal, videoDecision: &decision}
	ctx := context.WithValue(request.Context(), requestRouteContextKey{}, routeContext)
	gateway.proxy.ServeHTTP(writer, request.WithContext(ctx))
}

// observeVideoFallbackResponse records the upstream status without treating a
// chosen fallback path as proof that the client completed playback.
func (gateway *Gateway) observeVideoFallbackResponse(response *http.Response, routeContext requestRouteContext) {
	if routeContext.videoDecision == nil || response == nil {
		return
	}
	decision := *routeContext.videoDecision
	decision.StatusCode = response.StatusCode
	decision.UpstreamStatus = response.StatusCode
	gateway.logVideoDecision(decision)
}

// rejectVideo emits one fixed local security response and never calls either
// DirectPlay or Emby, because fallback would bypass Ember's identity gate.
func (gateway *Gateway) rejectVideo(writer http.ResponseWriter, request *http.Request, status int, stage, reasonCode string, startedAt time.Time) {
	decision := newVideoDecision(request, inspectVideoRequest(request), nil, startedAt)
	decision.Decision = "reject"
	decision.Stage = stage
	decision.ReasonCode = reasonCode
	decision.StatusCode = status
	writer.WriteHeader(status)
	gateway.logVideoDecision(decision)
}

// inspectVideoRequest accepts only a conservative static direct-play shape.
// Anything ambiguous stays playable through Emby but never reaches 115.
func inspectVideoRequest(request *http.Request) videoRequestInfo {
	info := videoRequestInfo{FallbackStage: "route", FallbackReason: "request_not_eligible"}
	if request == nil || request.URL == nil {
		return info
	}
	videoPath := videoPath(request.URL)
	info.ItemID = videoPath.ItemID
	if videoPath.ItemID == "" {
		return info
	}
	container, routeAccelerated := acceleratedStreamContainer(videoPath.StreamFileName, request.URL.Query())
	if !routeAccelerated {
		info.FallbackReason = "route_not_accelerated"
		return info
	}
	mediaSourceID, mediaSourceOK := singleBoundedQueryValue(request.URL.Query(), "MediaSourceId", maxProofMediaSourceIDBytes)
	playSessionID, playSessionOK := singleBoundedQueryValue(request.URL.Query(), "PlaySessionId", maxProofPlaySessionIDBytes)
	staticValue, staticOK := singleBoundedQueryValue(request.URL.Query(), "Static", 5)
	info.MediaSourceID = mediaSourceID
	info.PlaySessionID = playSessionID
	info.Container = container
	if !mediaSourceOK || !playSessionOK || !staticOK || staticValue != "true" {
		return info
	}
	info.Accelerated = true
	info.FallbackStage = ""
	info.FallbackReason = ""
	return info
}

// videoPath recognizes only the three fixed GET/HEAD path shapes at one exact
// segment depth. Escaped, trailing-slash and subtitle variants remain protected
// ordinary routes.
func videoPath(requestURL *url.URL) videoPathInfo {
	if requestURL == nil || requestURL.EscapedPath() != requestURL.Path {
		return videoPathInfo{}
	}
	segments := strings.Split(requestURL.Path, "/")
	if len(segments) != 5 || segments[0] != "" || segments[1] != "emby" || segments[2] != "Videos" ||
		!validProofValue(segments[3], maxProofItemIDBytes, false) ||
		!validProofValue(segments[4], maxVideoStreamFileNameBytes, false) {
		return videoPathInfo{}
	}
	return videoPathInfo{ItemID: segments[3], StreamFileName: segments[4]}
}

// acceleratedStreamContainer rejects manifest containers while extracting the
// original media container from the fixed stream path variants.
func acceleratedStreamContainer(streamFileName string, query url.Values) (string, bool) {
	var container string
	switch {
	case streamFileName == "stream":
		value, ok := singleBoundedQueryValue(query, "Container", maxProofContainerBytes)
		if !ok {
			return "", false
		}
		container = value
	case strings.HasPrefix(streamFileName, "stream."):
		container = strings.TrimPrefix(streamFileName, "stream.")
	default:
		container = strings.TrimPrefix(path.Ext(streamFileName), ".")
	}
	container = strings.ToLower(container)
	if !validProofValue(container, maxProofContainerBytes, false) {
		return "", false
	}
	switch container {
	case "m3u8", "mpd", "m4s":
		return container, false
	default:
		return container, true
	}
}

// singleBoundedQueryValue requires one exact query value before it can enter a
// proof lookup key or decision log.
func singleBoundedQueryValue(values url.Values, key string, maxBytes int) (string, bool) {
	items, exists := values[key]
	if !exists || len(items) != 1 || !validProofValue(items[0], maxBytes, false) {
		return "", false
	}
	return items[0], true
}

// validRedirectCandidate applies a final transport-level sanity check to the
// already policy-validated internal DirectPlay result.
func validRedirectCandidate(candidate directplay.RedirectCandidate) bool {
	if !candidate.ExpiresAt.After(time.Now().UTC()) || candidate.ConcurrentOpenLimit <= 0 ||
		candidate.URL == "" || strings.ContainsAny(candidate.URL, "\r\n") {
		return false
	}
	location, err := url.Parse(candidate.URL)
	return err == nil && location.Scheme == "https" && location.Host != "" && location.User == nil && location.Fragment == ""
}

// directPlayReasonCode maps every public DirectPlay error class to the fixed
// fallback log contract without exposing wrapped Provider error text.
func directPlayReasonCode(err error) string {
	switch {
	case errors.Is(err, directplay.ErrInvalidRequest):
		return "invalid_request"
	case errors.Is(err, directplay.ErrPathNotMapped):
		return "path_not_mapped"
	case errors.Is(err, directplay.ErrAccountUnavailable):
		return "account_unavailable"
	case errors.Is(err, directplay.ErrAccountsSame):
		return "accounts_same"
	case errors.Is(err, directplay.ErrProviderUnavailable):
		return "provider_unavailable"
	case errors.Is(err, directplay.ErrRapidUploadUnavailable):
		return "rapid_upload_unavailable"
	case errors.Is(err, directplay.ErrTargetUnavailable):
		return "target_unavailable"
	case errors.Is(err, directplay.ErrDownloadIncompatible):
		return "download_incompatible"
	case errors.Is(err, directplay.ErrStoreUnavailable):
		return "store_unavailable"
	case errors.Is(err, directplay.ErrLockUnavailable):
		return "lock_unavailable"
	default:
		return "provider_protocol"
	}
}

// videoPrincipalRejection maps local identity and user-state failures to fixed
// reject status, stage and reason values.
func videoPrincipalRejection(err error) (int, string, string) {
	switch {
	case errors.Is(err, embytoken.ErrTokenRevoked):
		return http.StatusUnauthorized, "identity", "token_revoked"
	case errors.Is(err, embytoken.ErrIdentityMismatch):
		return http.StatusUnauthorized, "identity", "identity_mismatch"
	case errors.Is(err, embytoken.ErrInvalidInput), errors.Is(err, embytoken.ErrTokenNotFound):
		return http.StatusUnauthorized, "identity", "token_unmapped"
	case errors.Is(err, embytoken.ErrUserExpired):
		return http.StatusForbidden, "user_state", "user_expired"
	case errors.Is(err, embytoken.ErrUserUnavailable):
		return http.StatusForbidden, "user_state", "user_unavailable"
	default:
		return http.StatusServiceUnavailable, "identity", "identity_store_unavailable"
	}
}

// newVideoDecision copies only bounded identifiers into one request-scoped log
// record and carries the original request start for end-to-end decision timing.
func newVideoDecision(request *http.Request, info videoRequestInfo, principal *embytoken.Principal, startedAt time.Time) videoDecision {
	if startedAt.IsZero() {
		startedAt = time.Now().UTC()
	}
	decision := videoDecision{
		ItemID: info.ItemID, MediaSourceID: info.MediaSourceID, PlaySessionID: info.PlaySessionID,
		StartedAt: startedAt,
	}
	if request != nil {
		decision.Method = request.Method
	}
	if principal != nil {
		decision.UserID = principal.User.ID
		decision.MappingID = principal.MappingID
		decision.DeviceID = principal.DeviceID
		decision.ClientName = principal.ClientName
	}
	return decision
}

// logVideoDecision writes the single request-level decision record using only
// bounded identifiers and fixed enums; paths, Tokens, URLs and raw errors never
// enter the message.
func (gateway *Gateway) logVideoDecision(decision videoDecision) {
	duration := time.Since(decision.StartedAt).Milliseconds()
	if duration < 0 {
		duration = 0
	}
	gateway.logger.Printf(
		"[PlaybackGateway] decision=%s stage=%s reasonCode=%s method=%s userId=%s mappingId=%s deviceId=%s clientName=%s itemId=%s mediaSourceId=%s playSessionId=%s taskId=%s preexisting=%t statusCode=%d upstreamStatus=%d proxyErrorCode=%s durationMs=%d",
		decision.Decision, decision.Stage, decision.ReasonCode, decision.Method,
		strconv.Quote(decision.UserID), strconv.Quote(decision.MappingID), strconv.Quote(decision.DeviceID),
		strconv.Quote(decision.ClientName), strconv.Quote(decision.ItemID), strconv.Quote(decision.MediaSourceID),
		strconv.Quote(decision.PlaySessionID), strconv.Quote(decision.TaskID), decision.Preexisting,
		decision.StatusCode, decision.UpstreamStatus, decision.ProxyErrorCode, duration,
	)
}
