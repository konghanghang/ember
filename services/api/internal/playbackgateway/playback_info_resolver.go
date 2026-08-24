package playbackgateway

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/konghang/ember/backend/internal/services/embytoken"
)

const (
	onDemandPlaybackInfoTimeout     = 10 * time.Second
	maxEmbyDirectStreamURLBytes     = 16 * 1024
	onDemandFallbackSourceClient    = "client_request"
	onDemandFallbackSourceContainer = "container_recovered"
	onDemandFallbackSourceDirectURL = "playback_info_direct_stream"
	onDemandFallbackSourceExtension = "playback_info_extension_stream"
	onDemandFallbackSourceAugmented = "playback_info_augmented_stream"
)

type onDemandPlaybackInfo struct {
	PlaySessionID string
	Container     string
	ProofCount    int
	FallbackURL   *url.URL
}

type onDemandPlaybackInfoResolveCall struct {
	done       chan struct{}
	result     onDemandPlaybackInfo
	reasonCode string
}

type onDemandPlaybackInfoFlightGroup struct {
	mu    sync.Mutex
	calls map[string]*onDemandPlaybackInfoResolveCall
}

var errOnDemandPlaybackInfoInvalid = errors.New("on-demand playback info invalid")

// resolvePlaybackInfoOnDemand coalesces concurrent resolution of one mapped
// item/source while allowing each waiting client request to cancel independently.
func (gateway *Gateway) resolvePlaybackInfoOnDemand(
	request *http.Request,
	principal embytoken.Principal,
	accessToken string,
	itemID string,
	mediaSourceID string,
) (onDemandPlaybackInfo, string) {
	if gateway == nil || gateway.playbackInfoFlights == nil || request == nil {
		return onDemandPlaybackInfo{}, "invalid_request"
	}
	if proof, ok := gateway.proofs.LookupLatestMediaSource(principal.MappingID, itemID, mediaSourceID); ok &&
		playbackProofMatchesPrincipal(proof, principal) {
		gateway.debugf("[PlaybackGateway] level=debug code=playback_info_reused_on_demand mappingId=%s itemId=%s", principal.MappingID, itemID)
		return onDemandPlaybackInfo{PlaySessionID: proof.PlaySessionID, Container: proof.Container}, ""
	}
	key := principal.MappingID + "\x00" + itemID + "\x00" + mediaSourceID
	return gateway.playbackInfoFlights.Do(request.Context(), key, func() (onDemandPlaybackInfo, string) {
		detachedRequest := request.Clone(context.WithoutCancel(request.Context()))
		return gateway.resolvePlaybackInfoOnce(detachedRequest, principal, accessToken, itemID, mediaSourceID)
	})
}

// resolvePlaybackInfoOnce asks the same Emby upstream to evaluate one
// incomplete static stream using the mapped user's Token, never the admin key.
func (gateway *Gateway) resolvePlaybackInfoOnce(
	request *http.Request,
	principal embytoken.Principal,
	accessToken string,
	itemID string,
	mediaSourceID string,
) (onDemandPlaybackInfo, string) {
	if gateway == nil || gateway.upstream == nil || gateway.transport == nil || request == nil ||
		!validProofValue(principal.MappingID, maxProofMappingIDBytes, false) ||
		!validProofValue(principal.User.EmbyID, maxProofEmbyUserIDBytes, false) ||
		!validProofValue(itemID, maxProofItemIDBytes, false) || itemID == "." || itemID == ".." ||
		!validProofValue(mediaSourceID, maxProofMediaSourceIDBytes, false) || accessToken == "" {
		return onDemandPlaybackInfo{}, "invalid_request"
	}
	requestURL, err := gateway.onDemandPlaybackInfoURL(itemID, principal.User.EmbyID)
	if err != nil {
		return onDemandPlaybackInfo{}, "invalid_request"
	}
	ctx, cancel := context.WithTimeout(request.Context(), onDemandPlaybackInfoTimeout)
	defer cancel()
	upstreamRequest, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL.String(), nil)
	if err != nil {
		return onDemandPlaybackInfo{}, "invalid_request"
	}
	copyOnDemandPlaybackInfoHeaders(upstreamRequest.Header, request.Header)
	embeddedToken, embeddedPresent, _ := protectedApplicationAccessToken(request.Header)
	if !embeddedPresent || embeddedToken != accessToken {
		upstreamRequest.Header.Set(accessTokenHeader, accessToken)
	}
	upstreamRequest.Header.Set("Accept", "application/json")
	upstreamRequest.Header.Set("Accept-Encoding", "gzip, deflate")

	response, err := gateway.transport.RoundTrip(upstreamRequest)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return onDemandPlaybackInfo{}, "request_canceled"
		}
		if errors.Is(err, context.DeadlineExceeded) {
			return onDemandPlaybackInfo{}, "deadline_exceeded"
		}
		return onDemandPlaybackInfo{}, "upstream_unavailable"
	}
	if response == nil {
		return onDemandPlaybackInfo{}, "upstream_unavailable"
	}
	if response.Body != nil {
		defer response.Body.Close()
	}
	if response.StatusCode != http.StatusOK {
		gateway.logger.Printf(
			"[PlaybackGateway] code=playback_info_resolve_failed reasonCode=upstream_status mappingId=%s itemId=%s statusCode=%d",
			principal.MappingID,
			itemID,
			response.StatusCode,
		)
		return onDemandPlaybackInfo{}, "upstream_status"
	}
	mediaType, _, err := mime.ParseMediaType(response.Header.Get("Content-Type"))
	if err != nil || mediaType != "application/json" || response.Body == nil {
		return onDemandPlaybackInfo{}, "response_invalid"
	}
	encoded, err := io.ReadAll(io.LimitReader(response.Body, gateway.maxPlaybackInfoResponseBytes+1))
	if err != nil || int64(len(encoded)) > gateway.maxPlaybackInfoResponseBytes {
		return onDemandPlaybackInfo{}, "response_too_large"
	}
	decoded, err := decodeResponseSidecar(encoded, response.Header.Get("Content-Encoding"), gateway.maxPlaybackInfoResponseBytes)
	if err != nil {
		return onDemandPlaybackInfo{}, "response_decode_failed"
	}
	resolved, proofs, err := parseOnDemandPlaybackInfo(decoded, principal, itemID, mediaSourceID)
	if err != nil {
		return onDemandPlaybackInfo{}, "response_unusable"
	}
	gateway.proofs.InvalidateItem(principal.MappingID, itemID)
	if len(proofs) > 0 {
		resolved.ProofCount = gateway.proofs.Record(proofs)
	}
	gateway.logger.Printf("[PlaybackGateway] code=playback_info_resolved_on_demand mappingId=%s itemId=%s proofCount=%d",
		principal.MappingID, itemID, resolved.ProofCount)
	return resolved, ""
}

// Do runs at most one bounded resolver for a key. The resolver goroutine uses
// its own timeout; canceled waiters do not poison other concurrent requests.
func (group *onDemandPlaybackInfoFlightGroup) Do(
	ctx context.Context,
	key string,
	resolve func() (onDemandPlaybackInfo, string),
) (onDemandPlaybackInfo, string) {
	if group == nil || ctx == nil || key == "" || resolve == nil {
		return onDemandPlaybackInfo{}, "invalid_request"
	}
	if err := ctx.Err(); err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return onDemandPlaybackInfo{}, "deadline_exceeded"
		}
		return onDemandPlaybackInfo{}, "request_canceled"
	}
	group.mu.Lock()
	if group.calls == nil {
		group.calls = make(map[string]*onDemandPlaybackInfoResolveCall)
	}
	call, exists := group.calls[key]
	if !exists {
		call = &onDemandPlaybackInfoResolveCall{done: make(chan struct{})}
		group.calls[key] = call
		go func() {
			defer func() {
				if recover() != nil {
					call.result = onDemandPlaybackInfo{}
					call.reasonCode = "internal_failure"
				}
				close(call.done)
				group.mu.Lock()
				delete(group.calls, key)
				group.mu.Unlock()
			}()
			call.result, call.reasonCode = resolve()
		}()
	}
	group.mu.Unlock()

	select {
	case <-call.done:
		return call.result, call.reasonCode
	case <-ctx.Done():
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return onDemandPlaybackInfo{}, "deadline_exceeded"
		}
		return onDemandPlaybackInfo{}, "request_canceled"
	}
}

// onDemandPlaybackInfoURL builds one credential-free upstream URL with only
// the UserId query fixed by the Emby 4.9.3 GET PlaybackInfo contract.
func (gateway *Gateway) onDemandPlaybackInfoURL(itemID, embyUserID string) (*url.URL, error) {
	if gateway == nil || gateway.upstream == nil {
		return nil, errOnDemandPlaybackInfoInvalid
	}
	target := *gateway.upstream
	target.Path = strings.TrimRight(target.Path, "/") + "/emby/Items/" + itemID + "/PlaybackInfo"
	target.RawPath = ""
	target.RawQuery = url.Values{"UserId": {embyUserID}}.Encode()
	if _, err := url.ParseRequestURI(target.RequestURI()); err != nil {
		return nil, fmt.Errorf("%w: request URI", errOnDemandPlaybackInfoInvalid)
	}
	return &target, nil
}

// copyOnDemandPlaybackInfoHeaders preserves client/device metadata but drops
// bodies, ranges, hop-by-hop fields and any direct Token header before setting
// the already normalized AccessToken explicitly.
func copyOnDemandPlaybackInfoHeaders(destination, source http.Header) {
	for _, name := range []string{
		standardAuthorizationHeader,
		embyAuthorizationHeader,
		mediaBrowserAuthorizationHeader,
		"User-Agent",
		"X-Emby-Client",
		"X-Emby-Device-Id",
		"X-Emby-Device-Name",
	} {
		for _, value := range source.Values(name) {
			destination.Add(name, value)
		}
	}
	destination.Del(accessTokenHeader)
	destination.Del(mediaBrowserTokenHeader)
}

// parseOnDemandPlaybackInfo validates the selected MediaSource for request
// completion and separately produces strict PlaybackProof entries for 115.
func parseOnDemandPlaybackInfo(
	body []byte,
	principal embytoken.Principal,
	itemID string,
	mediaSourceID string,
) (onDemandPlaybackInfo, []PlaybackProof, error) {
	var payload playbackInfoResponsePayload
	if err := json.Unmarshal(body, &payload); err != nil || payload.ErrorCode != "" ||
		!validProofValue(payload.PlaySessionID, maxProofPlaySessionIDBytes, false) ||
		len(payload.MediaSources) == 0 || len(payload.MediaSources) > maxPlaybackInfoMediaSources {
		return onDemandPlaybackInfo{}, nil, errOnDemandPlaybackInfoInvalid
	}
	var selected *playbackInfoMediaSource
	seen := make(map[string]struct{}, len(payload.MediaSources))
	for index := range payload.MediaSources {
		source := &payload.MediaSources[index]
		if !validProofValue(source.ID, maxProofMediaSourceIDBytes, false) {
			continue
		}
		if _, duplicate := seen[source.ID]; duplicate {
			return onDemandPlaybackInfo{}, nil, errOnDemandPlaybackInfoInvalid
		}
		seen[source.ID] = struct{}{}
		if source.ID == mediaSourceID {
			selected = source
		}
	}
	if selected == nil || selected.ItemID != "" && selected.ItemID != itemID {
		return onDemandPlaybackInfo{}, nil, errOnDemandPlaybackInfoInvalid
	}
	container, ok := normalizedContainer(selected.Container)
	if !ok {
		return onDemandPlaybackInfo{}, nil, errOnDemandPlaybackInfoInvalid
	}
	var fallbackURL *url.URL
	if selected.SupportsDirectStream {
		fallbackURL, _ = parseEmbyDirectStreamURL(
			selected.DirectStreamURL,
			itemID,
			mediaSourceID,
			payload.PlaySessionID,
			container,
		)
	}
	routeContext := requestRouteContext{
		kind: routePlaybackInfo, principal: &principal,
		playbackInfoItemID: itemID, playbackInfoEligible: true,
	}
	proofs, _ := buildPlaybackProofs(body, routeContext)
	return onDemandPlaybackInfo{
		PlaySessionID: payload.PlaySessionID,
		Container:     container,
		FallbackURL:   fallbackURL,
	}, proofs, nil
}

// augmentVideoRequestWithPlaybackInfo clones and appends only missing query
// keys, preserving every client-supplied raw query byte and value.
func augmentVideoRequestWithPlaybackInfo(request *http.Request, resolved onDemandPlaybackInfo) (*http.Request, bool) {
	if request == nil || request.URL == nil || !validProofValue(resolved.PlaySessionID, maxProofPlaySessionIDBytes, false) {
		return request, false
	}
	clone := request.Clone(request.Context())
	urlCopy := *request.URL
	if !queryKeyExistsFold(urlCopy.Query(), "Container") {
		urlCopy.RawQuery = appendRawQueryValue(urlCopy.RawQuery, "Container", resolved.Container)
	}
	if !queryKeyExistsFold(urlCopy.Query(), "PlaySessionId") {
		urlCopy.RawQuery = appendRawQueryValue(urlCopy.RawQuery, "PlaySessionId", resolved.PlaySessionID)
	}
	clone.URL = &urlCopy
	return clone, true
}

// onDemandPlaybackInfoCandidate recognizes only a plain static stream with one
// MediaSourceId and no client-supplied PlaySessionId key.
func onDemandPlaybackInfoCandidate(request *http.Request) (string, string, bool) {
	if request == nil || request.URL == nil {
		return "", "", false
	}
	pathInfo := videoPath(request.URL)
	if pathInfo.ItemID == "" || !strings.EqualFold(pathInfo.StreamFileName, "stream") ||
		queryKeyExistsFold(request.URL.Query(), "PlaySessionId") {
		return "", "", false
	}
	mediaSourceID, mediaSourceOK := singleBoundedQueryValue(request.URL.Query(), "MediaSourceId", maxProofMediaSourceIDBytes)
	staticValue, staticOK := singleBoundedQueryValue(request.URL.Query(), "Static", 5)
	if !mediaSourceOK || !staticOK || !strings.EqualFold(staticValue, "true") {
		return "", "", false
	}
	return pathInfo.ItemID, mediaSourceID, true
}

// appendRawQueryValue adds one trusted key/value pair without parsing or
// re-encoding any client-supplied query bytes.
func appendRawQueryValue(rawQuery, key, value string) string {
	separator := ""
	if rawQuery != "" && !strings.HasSuffix(rawQuery, "&") {
		separator = "&"
	}
	return rawQuery + separator + url.QueryEscape(key) + "=" + url.QueryEscape(value)
}

// parseEmbyDirectStreamURL accepts only the same relative video/item route
// returned by the trusted Emby upstream and removes every URL Token carrier.
func parseEmbyDirectStreamURL(
	rawURL string,
	itemID string,
	mediaSourceID string,
	playSessionID string,
	container string,
) (*url.URL, bool) {
	if !validProofValue(rawURL, maxEmbyDirectStreamURLBytes, false) {
		return nil, false
	}
	target, err := url.Parse(rawURL)
	if err != nil || target.IsAbs() || target.Host != "" || target.User != nil || target.Opaque != "" || target.Fragment != "" {
		return nil, false
	}
	if !strings.HasPrefix(target.Path, "/") {
		target.Path = "/" + target.Path
	}
	normalizeRequest := &http.Request{URL: target}
	pathMode, pathOK := normalizeEmbyAPIPath(normalizeRequest)
	if !pathOK || pathMode == requestPathModePassthrough {
		return nil, false
	}
	pathInfo := videoPath(target)
	if pathInfo.ItemID != itemID {
		return nil, false
	}
	query, err := url.ParseQuery(target.RawQuery)
	if err != nil {
		return nil, false
	}
	for key := range query {
		if _, tokenKey := canonicalTokenQueryKey(key); tokenKey {
			delete(query, key)
		}
	}
	if queryKeyExistsFold(query, "MediaSourceId") {
		value, valid := singleBoundedQueryValue(query, "MediaSourceId", maxProofMediaSourceIDBytes)
		if !valid || value != mediaSourceID {
			return nil, false
		}
	}
	if queryKeyExistsFold(query, "PlaySessionId") {
		value, valid := singleBoundedQueryValue(query, "PlaySessionId", maxProofPlaySessionIDBytes)
		if !valid || value != playSessionID {
			return nil, false
		}
	}
	if queryKeyExistsFold(query, "Static") {
		value, valid := singleBoundedQueryValue(query, "Static", 5)
		if !valid || !strings.EqualFold(value, "true") {
			return nil, false
		}
	}
	if queryKeyExistsFold(query, "Container") {
		value, valid := singleBoundedQueryValue(query, "Container", maxProofContainerBytes)
		if !valid || !equivalentEmbyStreamContainer(container, value) {
			return nil, false
		}
	}
	if strings.EqualFold(pathInfo.StreamFileName, "stream") && !queryKeyExistsFold(query, "Container") {
		query.Set("Container", container)
	}
	directContainer, valid := acceleratedStreamContainer(pathInfo.StreamFileName, query)
	if !valid || !equivalentEmbyStreamContainer(container, directContainer) {
		return nil, false
	}
	target.RawPath = ""
	target.RawQuery = query.Encode()
	target.ForceQuery = false
	return target, true
}

// buildOnDemandEmbyFallbackRequest keeps the DirectPlay decision request
// separate from the Emby byte-stream fallback selected by PlaybackInfo.
func buildOnDemandEmbyFallbackRequest(
	clientRequest *http.Request,
	decisionRequest *http.Request,
	resolved onDemandPlaybackInfo,
	accessToken string,
) (*http.Request, string) {
	if clientRequest == nil || clientRequest.URL == nil || decisionRequest == nil {
		return decisionRequest, onDemandFallbackSourceAugmented
	}
	if resolved.FallbackURL != nil {
		clone := clientRequest.Clone(clientRequest.Context())
		urlCopy := *resolved.FallbackURL
		authoritativeQuery := urlCopy.Query()
		for key, values := range clientRequest.URL.Query() {
			if managedOnDemandFallbackQueryKey(key) {
				continue
			}
			if _, tokenKey := canonicalTokenQueryKey(key); tokenKey || queryKeyExistsFold(authoritativeQuery, key) {
				continue
			}
			for _, value := range values {
				authoritativeQuery.Add(key, value)
			}
		}
		urlCopy.RawQuery = authoritativeQuery.Encode()
		clone.URL = &urlCopy
		ensureFallbackAccessTokenHeader(clone, accessToken)
		return clone, onDemandFallbackSourceDirectURL
	}
	if clone, ok := buildExtensionStreamFallbackRequest(clientRequest, resolved.Container); ok {
		return clone, onDemandFallbackSourceExtension
	}
	return decisionRequest, onDemandFallbackSourceAugmented
}

// buildExtensionStreamFallbackRequest follows the official Emby Web fallback
// shape when PlaybackInfo omits DirectStreamUrl.
func buildExtensionStreamFallbackRequest(request *http.Request, container string) (*http.Request, bool) {
	if request == nil || request.URL == nil {
		return request, false
	}
	pathInfo := videoPath(request.URL)
	if pathInfo.ItemID == "" || !strings.EqualFold(pathInfo.StreamFileName, "stream") {
		return request, false
	}
	pathContainer, ok := embyExtensionStreamContainer(container)
	if !ok {
		return request, false
	}
	segments := strings.Split(request.URL.Path, "/")
	if len(segments) != 5 {
		return request, false
	}
	segments[4] = "stream." + pathContainer
	clone := request.Clone(request.Context())
	urlCopy := *request.URL
	urlCopy.Path = strings.Join(segments, "/")
	urlCopy.RawPath = ""
	clone.URL = &urlCopy
	return clone, true
}

// managedOnDemandFallbackQueryKey keeps Emby's authoritative stream identity
// and shape from being overwritten by the incomplete client URL.
func managedOnDemandFallbackQueryKey(key string) bool {
	return strings.EqualFold(key, "MediaSourceId") || strings.EqualFold(key, "PlaySessionId") ||
		strings.EqualFold(key, "Container") || strings.EqualFold(key, "Static")
}

// ensureFallbackAccessTokenHeader replaces a stripped URL credential with the
// already mapped current-user Token only when no equivalent Header exists.
func ensureFallbackAccessTokenHeader(request *http.Request, accessToken string) {
	if request == nil || accessToken == "" {
		return
	}
	if current, _, ok := extractProtectedAccessToken(request.Header); ok && current == accessToken {
		return
	}
	if request.Header == nil {
		request.Header = make(http.Header)
	}
	request.Header.Set(accessTokenHeader, accessToken)
}

// equivalentEmbyStreamContainer includes Emby Web's documented m4v-to-mp4
// direct-stream normalization while rejecting unrelated containers.
func equivalentEmbyStreamContainer(expected, actual string) bool {
	if strings.EqualFold(expected, actual) {
		return true
	}
	return strings.EqualFold(expected, "m4v") && strings.EqualFold(actual, "mp4")
}

// embyExtensionStreamContainer returns one safe path segment for the official
// /stream.{Container} fallback and rejects multi-container query syntax.
func embyExtensionStreamContainer(container string) (string, bool) {
	value, ok := normalizedContainer(container)
	if !ok || strings.Contains(value, ",") {
		return "", false
	}
	if value == "m4v" {
		value = "mp4"
	}
	return value, true
}
