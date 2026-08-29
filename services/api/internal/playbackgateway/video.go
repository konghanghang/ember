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
	Decision           string
	Stage              string
	ReasonCode         string
	FallbackSource     string
	Method             string
	UserID             string
	MappingID          string
	DeviceID           string
	ClientName         string
	ItemID             string
	MediaSourceID      string
	PlaySessionID      string
	TaskID             string
	Preexisting        bool
	StatusCode         int
	UpstreamStatus     int
	ProxyErrorCode     string
	MediaPath          string
	EmbyPathPrefix     string
	SourceRootID       string
	MappedRelativePath string
	StartedAt          time.Time
}

// serveVideo chooses exactly one of reject, redirect or transparent Emby
// fallback after ServeHTTP has already resolved a fresh local Principal.
func (gateway *Gateway) serveVideo(
	writer http.ResponseWriter,
	request *http.Request,
	principal embytoken.Principal,
	accessToken string,
	startedAt time.Time,
) {
	fallbackRequest := request
	fallbackSource := onDemandFallbackSourceClient
	playbackInfoResolved := false
	resolvedMediaPath := ""
	if itemID, mediaSourceID, eligible := onDemandPlaybackInfoCandidate(request); eligible {
		clientRequest := request
		resolved, reasonCode := gateway.resolvePlaybackInfoOnDemand(request, principal, accessToken, itemID, mediaSourceID)
		if reasonCode == "" {
			resolvedMediaPath = resolved.MediaSource.MediaPath
			request, playbackInfoResolved = augmentVideoRequestWithPlaybackInfo(request, resolved)
			if playbackInfoResolved {
				fallbackRequest, fallbackSource = buildOnDemandEmbyFallbackRequest(
					clientRequest,
					request,
					resolved,
					accessToken,
				)
			}
		} else if reasonCode != "upstream_status" {
			gateway.logger.Printf(
				"[PlaybackGateway] code=playback_info_resolve_failed reasonCode=%s mappingId=%s itemId=%s",
				reasonCode,
				principal.MappingID,
				itemID,
			)
		}
	}
	var containerRecovered bool
	if !playbackInfoResolved {
		request, containerRecovered = gateway.recoverMissingStreamContainer(request, principal)
		fallbackRequest = request
		if containerRecovered {
			fallbackSource = onDemandFallbackSourceContainer
		}
	}
	info := inspectVideoRequest(request)
	if containerRecovered {
		info.Accelerated = false
		info.FallbackStage = "route"
		info.FallbackReason = "container_recovered"
	}
	decision := newVideoDecision(request, info, &principal, startedAt)
	decision.MediaPath = resolvedMediaPath
	if !info.Accelerated {
		decision.Stage = info.FallbackStage
		decision.ReasonCode = info.FallbackReason
		gateway.proxyVideoFallback(writer, fallbackRequest, decision, principal, fallbackSource)
		return
	}

	proof, proofReason := gateway.lookupPlaybackProof(principal, info.ItemID, info.MediaSourceID, info.PlaySessionID)
	if proofReason != "" {
		decision.Stage = "proof"
		decision.ReasonCode = proofReason
		gateway.proxyVideoFallback(writer, fallbackRequest, decision, principal, fallbackSource)
		return
	}
	if proof.Container != "" && info.Container != "" && !strings.EqualFold(proof.Container, info.Container) {
		decision.Stage = "eligibility"
		decision.ReasonCode = "media_not_direct_play"
		gateway.proxyVideoFallback(writer, fallbackRequest, decision, principal, fallbackSource)
		return
	}
	if gateway.directPlayService == nil {
		decision.Stage = "eligibility"
		decision.ReasonCode = "direct_play_disabled"
		gateway.proxyVideoFallback(writer, fallbackRequest, decision, principal, fallbackSource)
		return
	}

	decision.MediaPath = proof.Path
	candidate, err := gateway.directPlayService.ResolveMediaPath(request.Context(), directplay.MediaPathResolveRequest{
		Path: proof.Path, Size: proof.Size, ClientUserAgent: request.UserAgent(),
	})
	if candidate.PathMapping.OriginalPath != "" {
		decision.MediaPath = candidate.PathMapping.OriginalPath
	}
	decision.EmbyPathPrefix = candidate.PathMapping.EmbyPathPrefix
	decision.SourceRootID = candidate.PathMapping.SourceRootID
	decision.MappedRelativePath = candidate.PathMapping.RelativePath
	if err != nil {
		decision.Stage = "direct_play"
		decision.ReasonCode = directPlayReasonCode(err)
		gateway.proxyVideoFallback(writer, fallbackRequest, decision, principal, fallbackSource)
		return
	}
	if !validRedirectCandidate(candidate) {
		decision.Stage = "direct_play"
		decision.ReasonCode = "provider_protocol"
		gateway.proxyVideoFallback(writer, fallbackRequest, decision, principal, fallbackSource)
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

// proxyVideoFallback delegates the selected client or PlaybackInfo-authorized
// Emby request to the existing proxy; response/error hooks own the final log.
func (gateway *Gateway) proxyVideoFallback(
	writer http.ResponseWriter,
	request *http.Request,
	decision videoDecision,
	principal embytoken.Principal,
	fallbackSource string,
) {
	decision.Decision = "fallback"
	decision.FallbackSource = fallbackSource
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
	if !mediaSourceOK || !playSessionOK || !staticOK || !strings.EqualFold(staticValue, "true") {
		return info
	}
	info.Accelerated = true
	info.FallbackStage = ""
	info.FallbackReason = ""
	return info
}

// videoPath recognizes the three fixed video shapes case-insensitively at one
// exact depth. Escaped, trailing-slash and subtitle variants remain ordinary.
func videoPath(requestURL *url.URL) videoPathInfo {
	if requestURL == nil || requestURL.EscapedPath() != requestURL.Path {
		return videoPathInfo{}
	}
	segments := strings.Split(requestURL.Path, "/")
	if len(segments) != 5 || segments[0] != "" || !strings.EqualFold(segments[1], "emby") || !strings.EqualFold(segments[2], "Videos") ||
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
	lowerFileName := strings.ToLower(streamFileName)
	switch {
	case lowerFileName == "stream":
		value, ok := singleBoundedQueryValue(query, "Container", maxProofContainerBytes)
		if !ok {
			return "", false
		}
		container = value
	case strings.HasPrefix(lowerFileName, "stream."):
		container = strings.TrimPrefix(lowerFileName, "stream.")
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

// singleBoundedQueryValue accepts one case-insensitive logical key while
// rejecting duplicate casing variants before values enter proof or log keys.
func singleBoundedQueryValue(values url.Values, key string, maxBytes int) (string, bool) {
	var items []string
	found := false
	for candidateKey, candidateItems := range values {
		if !strings.EqualFold(candidateKey, key) {
			continue
		}
		if found {
			return "", false
		}
		found = true
		items = candidateItems
	}
	if !found || len(items) != 1 || !validProofValue(items[0], maxBytes, false) {
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

// logVideoDecision writes the single request-level decision record with fixed
// enums and the explicitly authorized Emby/source path mapping provenance.
// Tokens, Cookies, signed URLs, response bodies and raw errors never enter it.
func (gateway *Gateway) logVideoDecision(decision videoDecision) {
	duration := time.Since(decision.StartedAt).Milliseconds()
	if duration < 0 {
		duration = 0
	}
	gateway.logger.Printf(
		"[PlaybackGateway] decision=%s stage=%s reasonCode=%s fallbackSource=%s method=%s userId=%s mappingId=%s deviceId=%s clientName=%s itemId=%s mediaSourceId=%s playSessionId=%s taskId=%s preexisting=%t statusCode=%d upstreamStatus=%d proxyErrorCode=%s mediaPath=%s embyPathPrefix=%s sourceRootId=%s mappedRelativePath=%s durationMs=%d",
		decision.Decision, decision.Stage, decision.ReasonCode, decision.FallbackSource, decision.Method,
		strconv.Quote(decision.UserID), strconv.Quote(decision.MappingID), strconv.Quote(decision.DeviceID),
		strconv.Quote(decision.ClientName), strconv.Quote(decision.ItemID), strconv.Quote(decision.MediaSourceID),
		strconv.Quote(decision.PlaySessionID), strconv.Quote(decision.TaskID), decision.Preexisting,
		decision.StatusCode, decision.UpstreamStatus, decision.ProxyErrorCode,
		strconv.Quote(decision.MediaPath), strconv.Quote(decision.EmbyPathPrefix),
		strconv.Quote(decision.SourceRootID), strconv.Quote(decision.MappedRelativePath), duration,
	)
}
