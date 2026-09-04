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

// PlaybackSessionService updates only Redis leases after Emby accepted a
// previously authenticated Playing/Progress/Stopped event.
type PlaybackSessionService interface {
	HandlePlaybackSessionEvent(context.Context, directplay.PlaybackSessionEvent) (directplay.PlaybackSessionEventResult, error)
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
	FallbackTarget     string
	LocalLookup        string
	LocalReasonCode    string
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
	ProviderOperation  string
	AccountRole        string
	MediaPath          string
	EmbyPathPrefix     string
	SourceRootID       string
	MappedRelativePath string
	Routing            directplay.RoutingDiagnostics
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
		Path: proof.Path, ClientUserAgent: request.UserAgent(), Method: request.Method,
		UserID: principal.User.ID, MappingID: principal.MappingID, DeviceID: principal.DeviceID, PlaySessionID: info.PlaySessionID,
	})
	decision.Routing = candidate.Routing
	if candidate.PathMapping.OriginalPath != "" {
		decision.MediaPath = candidate.PathMapping.OriginalPath
	}
	decision.EmbyPathPrefix = candidate.PathMapping.EmbyPathPrefix
	decision.SourceRootID = candidate.PathMapping.SourceRootID
	decision.MappedRelativePath = candidate.PathMapping.RelativePath
	if status, reasonCode, terminated := directPlayRequestTermination(request, err); terminated {
		decision.Decision = "reject"
		decision.Stage = "direct_play"
		decision.ReasonCode = reasonCode
		decision.StatusCode = status
		writer.WriteHeader(status)
		gateway.logVideoDecision(decision)
		return
	}
	if err != nil {
		failureContext := directplay.InspectFailure(err)
		decision.ProviderOperation = failureContext.ProviderOperation
		decision.AccountRole = failureContext.AccountRole
		decision.Stage = "direct_play"
		decision.ReasonCode = directPlayReasonCode(err)
		gateway.serveMappedVideoFallback(writer, request, fallbackRequest, decision, principal, fallbackSource)
		return
	}
	if !validRedirectCandidate(candidate) {
		decision.Stage = "direct_play"
		decision.ReasonCode = "provider_protocol"
		gateway.serveMappedVideoFallback(writer, request, fallbackRequest, decision, principal, fallbackSource)
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

// directPlayRequestTermination prevents an already canceled or expired request
// from opening local media or starting an Emby fallback after DirectPlay exits.
func directPlayRequestTermination(request *http.Request, directPlayErr error) (int, string, bool) {
	requestErr := error(nil)
	if request != nil {
		requestErr = request.Context().Err()
	}
	switch {
	case errors.Is(requestErr, context.DeadlineExceeded), errors.Is(directPlayErr, context.DeadlineExceeded):
		return http.StatusGatewayTimeout, "request_deadline_exceeded", true
	case errors.Is(requestErr, context.Canceled), errors.Is(directPlayErr, context.Canceled):
		return statusClientClosedRequest, "request_canceled", true
	default:
		return 0, "", false
	}
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
	decision.FallbackTarget = "emby"
	decision.FallbackSource = fallbackSource
	routeContext := requestRouteContext{kind: routeVideo, principal: &principal, videoDecision: &decision}
	ctx := context.WithValue(request.Context(), requestRouteContextKey{}, routeContext)
	gateway.proxy.ServeHTTP(writer, request.WithContext(ctx))
}

// serveMappedVideoFallback tries the optional exact local file only after the
// authenticated DirectPlay path has produced a trusted relative mapping. Any
// pre-response local miss or incompatibility preserves the authoritative Emby
// fallback request without changing its method, headers, query, or body.
func (gateway *Gateway) serveMappedVideoFallback(
	writer http.ResponseWriter,
	request *http.Request,
	fallbackRequest *http.Request,
	decision videoDecision,
	principal embytoken.Principal,
	fallbackSource string,
) {
	if decision.MappedRelativePath == "" {
		gateway.proxyVideoFallback(writer, fallbackRequest, decision, principal, fallbackSource)
		return
	}
	if gateway.localMediaResolver == nil {
		decision.LocalLookup = "disabled"
		decision.LocalReasonCode = "local_media_disabled"
		gateway.proxyVideoFallback(writer, fallbackRequest, decision, principal, fallbackSource)
		return
	}
	if !localMediaRequestEligible(request) {
		decision.LocalLookup = "unsupported"
		decision.LocalReasonCode = "local_request_unsupported"
		gateway.proxyVideoFallback(writer, fallbackRequest, decision, principal, fallbackSource)
		return
	}

	file, err := gateway.localMediaResolver.Open(decision.MappedRelativePath)
	if err != nil {
		decision.LocalLookup, decision.LocalReasonCode = localMediaOpenFailure(err)
		gateway.proxyVideoFallback(writer, fallbackRequest, decision, principal, fallbackSource)
		return
	}
	result := serveLocalMediaFile(writer, request, file)
	_ = file.Close()
	if result.StatusCode == 0 {
		decision.LocalLookup = "unavailable"
		decision.LocalReasonCode = "local_media_open_failed"
		gateway.proxyVideoFallback(writer, fallbackRequest, decision, principal, fallbackSource)
		return
	}

	decision.Decision = "fallback"
	decision.FallbackTarget = "local"
	decision.FallbackSource = ""
	decision.LocalLookup = "hit"
	decision.LocalReasonCode = "local_media_ready"
	decision.StatusCode = result.StatusCode
	if result.Interrupted {
		decision.LocalReasonCode = "local_stream_interrupted"
	}
	gateway.logVideoDecision(decision)
}

func localMediaOpenFailure(err error) (string, string) {
	switch {
	case errors.Is(err, ErrLocalMediaNotFound):
		return "miss", "local_media_not_found"
	case errors.Is(err, ErrLocalMediaUnsafe), errors.Is(err, ErrLocalMediaRootUnsafe):
		return "unsafe", "local_media_unsafe"
	default:
		return "unavailable", "local_media_open_failed"
	}
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
	case errors.Is(err, directplay.ErrPersonalAccountMissing):
		return "personal_account_missing"
	case errors.Is(err, directplay.ErrAccountConcurrencyExceeded):
		return "account_concurrency_exceeded"
	case errors.Is(err, directplay.ErrRedisUnavailable):
		return "redis_unavailable"
	case errors.Is(err, directplay.ErrHeadLeaseMissing):
		return "head_lease_missing"
	case errors.Is(err, directplay.ErrPlaybackRouteChanged):
		return "playback_route_changed"
	case errors.Is(err, directplay.ErrTransferQuotaExceeded):
		return "transfer_quota_exceeded"
	case errors.Is(err, directplay.ErrTransferQuotaCommitFailed):
		return "transfer_quota_commit_failed"
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

// logVideoDecision writes one outcome-specific request record whose leading
// fields state the human-readable result before the detailed diagnostics.
// Tokens, Cookies, signed URLs, response bodies and raw errors never enter it.
func (gateway *Gateway) logVideoDecision(decision videoDecision) {
	duration := time.Since(decision.StartedAt).Milliseconds()
	if duration < 0 {
		duration = 0
	}
	fields := videoDecisionHeadline(decision)
	fields = appendVideoDecisionContext(fields, decision, duration)
	gateway.logger.Print(strings.Join(fields, " "))
}

// videoDecisionHeadline puts the human and machine-readable outcome before
// request identifiers so operators can classify a line without scanning it.
func videoDecisionHeadline(decision videoDecision) []string {
	fields := []string{"[PlaybackGateway]"}
	switch decision.Decision {
	case "redirect":
		targetState := "created"
		if decision.Preexisting {
			targetState = "reused"
		}
		fields = append(fields,
			"level=info", "code=direct_play_redirect", "message="+strconv.Quote("115直链成功"),
			"result=success", "statusCode="+strconv.Itoa(decision.StatusCode), "target=p115",
			"targetState="+targetState,
		)
	case "fallback":
		fallbackResult := requestOutcome(decision.StatusCode)
		fallbackName := "Emby"
		if decision.FallbackTarget == "local" {
			fallbackName = "本地"
		}
		message := fallbackName + "回退成功"
		level := "info"
		if fallbackResult != "success" || decision.LocalReasonCode == "local_stream_interrupted" {
			fallbackResult = "failure"
			message = fallbackName + "回退失败"
			level = "warn"
		}
		code := "playback_fallback"
		if decision.Stage == "direct_play" {
			code = "direct_play_fallback"
			message = "115直链失败，" + message
		}
		fields = append(fields, "level="+level, "code="+code, "message="+strconv.Quote(message))
		if decision.Stage == "direct_play" {
			fields = append(fields, "directPlayResult=failure")
		}
		fields = append(fields,
			"fallbackResult="+fallbackResult,
			"statusCode="+strconv.Itoa(decision.StatusCode),
		)
	case "reject":
		fields = append(fields,
			"level=warn", "code=playback_rejected", "message="+strconv.Quote("播放请求已拒绝"),
			"result=rejected", "statusCode="+strconv.Itoa(decision.StatusCode),
		)
	default:
		fields = append(fields,
			"level=warn", "code=playback_decision_unknown", "message="+strconv.Quote("播放决策异常"),
			"result=unknown", "statusCode="+strconv.Itoa(decision.StatusCode),
		)
	}
	return fields
}

// appendVideoDecisionContext adds only relevant diagnostics for the selected
// outcome; empty fallback/task/mapping fields are intentionally omitted.
func appendVideoDecisionContext(fields []string, decision videoDecision, duration int64) []string {
	fields = append(fields,
		"decision="+decision.Decision,
		"stage="+decision.Stage,
		"reasonCode="+decision.ReasonCode,
	)
	fields = appendOptionalLogField(fields, "fallbackSource", decision.FallbackSource, false)
	fields = appendOptionalLogField(fields, "fallbackTarget", decision.FallbackTarget, false)
	fields = appendOptionalLogField(fields, "localLookup", decision.LocalLookup, false)
	fields = appendOptionalLogField(fields, "localReasonCode", decision.LocalReasonCode, false)
	fields = appendOptionalLogField(fields, "providerOperation", decision.ProviderOperation, false)
	fields = appendOptionalLogField(fields, "accountRole", decision.AccountRole, false)
	fields = appendOptionalLogField(fields, "method", decision.Method, false)
	fields = appendOptionalLogField(fields, "userId", decision.UserID, true)
	fields = appendOptionalLogField(fields, "mappingId", decision.MappingID, true)
	fields = appendOptionalLogField(fields, "deviceId", decision.DeviceID, true)
	fields = appendOptionalLogField(fields, "clientName", decision.ClientName, true)
	fields = appendOptionalLogField(fields, "itemId", decision.ItemID, true)
	fields = appendOptionalLogField(fields, "mediaSourceId", decision.MediaSourceID, true)
	fields = appendOptionalLogField(fields, "taskId", decision.TaskID, true)
	if decision.Decision == "redirect" {
		fields = append(fields, "preexisting="+strconv.FormatBool(decision.Preexisting))
	}
	if decision.UpstreamStatus > 0 {
		fields = append(fields, "upstreamStatus="+strconv.Itoa(decision.UpstreamStatus))
	}
	fields = appendOptionalLogField(fields, "proxyErrorCode", decision.ProxyErrorCode, false)
	fields = appendOptionalLogField(fields, "mediaPath", decision.MediaPath, true)
	fields = appendOptionalLogField(fields, "embyPathPrefix", decision.EmbyPathPrefix, true)
	fields = appendOptionalLogField(fields, "sourceRootId", decision.SourceRootID, true)
	fields = appendOptionalLogField(fields, "mappedRelativePath", decision.MappedRelativePath, true)
	fields = appendRoutingDecisionContext(fields, decision.Routing)
	fields = append(fields, "durationMs="+strconv.FormatInt(duration, 10))
	return fields
}

// appendRoutingDecisionContext exposes only fixed routing labels and aggregate
// counters. Provider identities, session fingerprints and Redis keys never
// cross the DirectPlay-to-Gateway diagnostic boundary.
func appendRoutingDecisionContext(fields []string, routing directplay.RoutingDiagnostics) []string {
	if !routing.Routed {
		return fields
	}
	playbackMode := string(routing.PlaybackMode)
	if playbackMode == "personal" || playbackMode == "system" {
		fields = append(fields, "playbackMode="+playbackMode)
	}
	if routing.PlaybackAccountOwner == "shared" || routing.PlaybackAccountOwner == "current_user" {
		fields = append(fields, "playbackAccountOwner="+routing.PlaybackAccountOwner)
	}
	if routing.AccountLimitsAvailable {
		fields = append(fields,
			"accountConfiguredStreamLimit="+strconv.Itoa(routing.ConfiguredMaxConcurrentStreams),
			"accountEffectiveStreamLimit="+strconv.Itoa(routing.EffectiveMaxConcurrentStreams),
		)
	}
	if playbackMode == "personal" {
		fields = append(fields, "simultaneousStreamLimit="+strconv.Itoa(routing.SimultaneousStreamLimit))
	}
	if routing.LeaseUsageAvailable {
		fields = append(fields,
			"accountReservedStreams="+strconv.Itoa(routing.AccountUsage.ReservedStreams),
			"accountActiveStreams="+strconv.Itoa(routing.AccountUsage.ActiveStreams),
			"accountOccupiedStreams="+strconv.Itoa(routing.AccountUsage.OccupiedStreams),
			"userReservedStreams="+strconv.Itoa(routing.UserUsage.ReservedStreams),
			"userActiveStreams="+strconv.Itoa(routing.UserUsage.ActiveStreams),
			"userOccupiedStreams="+strconv.Itoa(routing.UserUsage.OccupiedStreams),
		)
	}
	if routing.TransferChecked && routing.TransferUsageAvailable {
		fields = append(fields,
			"transferHourlyUsed="+strconv.Itoa(routing.TransferUsage.HourlyUsed),
			"transferHourlyLimit="+strconv.Itoa(routing.TransferHourlyLimit),
			"transferDailyUsed="+strconv.Itoa(routing.TransferUsage.DailyUsed),
			"transferDailyLimit="+strconv.Itoa(routing.TransferDailyLimit),
		)
	}
	return fields
}

// appendOptionalLogField omits meaningless empty values while preserving
// quoted output for identifiers and the explicitly authorized media paths.
func appendOptionalLogField(fields []string, key, value string, quoted bool) []string {
	if value == "" {
		return fields
	}
	if quoted {
		value = strconv.Quote(value)
	}
	return append(fields, key+"="+value)
}
