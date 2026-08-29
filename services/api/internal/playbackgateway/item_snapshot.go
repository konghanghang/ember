package playbackgateway

import (
	"bytes"
	"encoding/json"
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
	defaultItemContainerSnapshotTTL        = 5 * time.Minute
	defaultItemContainerSnapshotMaxEntries = 4096
	defaultItemDetailResponseMaxSize       = int64(2 << 20)
	maxItemDetailMediaSources              = 64
)

type itemContainerSource struct {
	MediaSourceID string `json:"Id"`
	Container     string `json:"Container"`
}

type itemContainerSnapshotKey struct {
	mappingID     string
	itemID        string
	mediaSourceID string
}

type itemContainerSnapshot struct {
	Container string
	ExpiresAt time.Time
}

type itemContainerSnapshotCache struct {
	mu         sync.Mutex
	entries    map[itemContainerSnapshotKey]itemContainerSnapshot
	maxEntries int
	ttl        time.Duration
	now        func() time.Time
}

type userItemDetailPayload struct {
	ID           string                `json:"Id"`
	Container    string                `json:"Container"`
	MediaSources []itemContainerSource `json:"MediaSources"`
}

// newItemContainerSnapshotCache creates a bounded, in-process compatibility
// cache. It contains no Token, media Path, size or playback authorization.
func newItemContainerSnapshotCache(maxEntries int, ttl time.Duration) *itemContainerSnapshotCache {
	return &itemContainerSnapshotCache{
		entries:    make(map[itemContainerSnapshotKey]itemContainerSnapshot),
		maxEntries: maxEntries,
		ttl:        ttl,
		now:        time.Now,
	}
}

// Record stores an item-level Container and exact MediaSource Containers from
// one successful, user-authorized BaseItemDto response.
func (cache *itemContainerSnapshotCache) Record(mappingID, itemID, itemContainer string, sources []itemContainerSource) int {
	if cache == nil || cache.maxEntries <= 0 || cache.ttl <= 0 || cache.now == nil ||
		!validProofValue(mappingID, maxProofMappingIDBytes, false) || !validProofValue(itemID, maxProofItemIDBytes, false) {
		return 0
	}
	candidates, ok := itemContainerCandidates(itemContainer, sources)
	if !ok || len(candidates) == 0 {
		return 0
	}
	now := cache.now().UTC()
	cache.mu.Lock()
	defer cache.mu.Unlock()
	cache.pruneExpiredLocked(now)
	written := 0
	for mediaSourceID, container := range candidates {
		key := itemContainerSnapshotKey{mappingID: mappingID, itemID: itemID, mediaSourceID: mediaSourceID}
		if _, exists := cache.entries[key]; !exists && len(cache.entries) >= cache.maxEntries {
			cache.evictEarliestLocked()
		}
		cache.entries[key] = itemContainerSnapshot{Container: container, ExpiresAt: now.Add(cache.ttl)}
		written++
	}
	return written
}

// Lookup prefers an exact MediaSource match and then the BaseItemDto top-level
// Container. A fresh Principal is still required before this method is used.
func (cache *itemContainerSnapshotCache) Lookup(mappingID, itemID, mediaSourceID string) (string, bool) {
	if cache == nil || cache.now == nil {
		return "", false
	}
	now := cache.now().UTC()
	cache.mu.Lock()
	defer cache.mu.Unlock()
	for _, key := range []itemContainerSnapshotKey{
		{mappingID: mappingID, itemID: itemID, mediaSourceID: mediaSourceID},
		{mappingID: mappingID, itemID: itemID},
	} {
		snapshot, ok := cache.entries[key]
		if !ok {
			continue
		}
		if !snapshot.ExpiresAt.After(now) {
			delete(cache.entries, key)
			continue
		}
		return snapshot.Container, true
	}
	return "", false
}

// itemContainerCandidates validates the complete source set before caching so
// duplicate MediaSource IDs cannot create order-dependent fallback behavior.
func itemContainerCandidates(itemContainer string, sources []itemContainerSource) (map[string]string, bool) {
	if len(sources) > maxItemDetailMediaSources {
		return nil, false
	}
	candidates := make(map[string]string, len(sources)+1)
	for _, source := range sources {
		if !validProofValue(source.MediaSourceID, maxProofMediaSourceIDBytes, false) {
			continue
		}
		container, ok := normalizedContainer(source.Container)
		if !ok {
			continue
		}
		if _, duplicate := candidates[source.MediaSourceID]; duplicate {
			return nil, false
		}
		candidates[source.MediaSourceID] = container
	}
	if len(candidates) == 0 {
		if normalized, ok := normalizedContainer(itemContainer); ok {
			candidates[""] = normalized
		}
	}
	return candidates, true
}

// userItemDetailPath matches /emby/Users/{UserId}/Items/{ItemId} at one exact
// depth; the caller owns method checks so list and child routes stay ordinary.
func userItemDetailPath(requestURL *url.URL) (string, string, bool) {
	if requestURL == nil || requestURL.EscapedPath() != requestURL.Path {
		return "", "", false
	}
	segments := strings.Split(requestURL.Path, "/")
	if len(segments) != 6 || segments[0] != "" || !strings.EqualFold(segments[1], "emby") ||
		!strings.EqualFold(segments[2], "Users") || !strings.EqualFold(segments[4], "Items") ||
		!validProofValue(segments[3], maxProofEmbyUserIDBytes, false) || !validProofValue(segments[5], maxProofItemIDBytes, false) {
		return "", "", false
	}
	return segments[3], segments[5], true
}

// normalizedContainer keeps only the bounded token needed by Emby's required
// stream query and never accepts whitespace or query delimiters.
func normalizedContainer(value string) (string, bool) {
	value = strings.ToLower(value)
	if !validProofValue(value, maxProofContainerBytes, false) {
		return "", false
	}
	for _, character := range value {
		if character >= 'a' && character <= 'z' || character >= '0' && character <= '9' || character == ',' || character == '-' {
			continue
		}
		return "", false
	}
	return value, true
}

// observeUserItemDetailResponse records only Container metadata while restoring
// and returning the exact upstream BaseItemDto response bytes.
func (gateway *Gateway) observeUserItemDetailResponse(response *http.Response, routeContext requestRouteContext) error {
	if response == nil || routeContext.principal == nil || !routeContext.itemDetailEligible ||
		response.StatusCode != http.StatusOK || response.Body == nil {
		return nil
	}
	mediaType, _, err := mime.ParseMediaType(response.Header.Get("Content-Type"))
	if err != nil || mediaType != "application/json" {
		return nil
	}
	originalBody := response.Body
	prefix, readErr := io.ReadAll(io.LimitReader(originalBody, gateway.maxItemDetailResponseBytes+1))
	response.Body = &replayedBody{Reader: io.MultiReader(bytes.NewReader(prefix), originalBody), closer: originalBody}
	if readErr != nil || int64(len(prefix)) > gateway.maxItemDetailResponseBytes {
		gateway.logger.Printf("[PlaybackGateway] code=item_container_snapshot_unusable reasonCode=response_read_failed")
		return nil
	}
	decoded, decodeErr := decodeResponseSidecar(prefix, response.Header.Get("Content-Encoding"), gateway.maxItemDetailResponseBytes)
	if decodeErr != nil {
		gateway.logger.Printf("[PlaybackGateway] code=item_container_snapshot_unusable reasonCode=response_decode_failed contentEncoding=%s",
			responseSidecarEncodingCode(response.Header.Get("Content-Encoding")))
		return nil
	}
	var payload userItemDetailPayload
	if err := json.Unmarshal(decoded, &payload); err != nil {
		gateway.logger.Printf(
			"[PlaybackGateway] code=item_container_snapshot_unusable reasonCode=response_json_invalid mappingId=%q itemId=%q contentEncoding=%s bodyBytes=%d errorType=%T",
			routeContext.principal.MappingID,
			routeContext.itemDetailItemID,
			responseSidecarEncodingCode(response.Header.Get("Content-Encoding")),
			len(decoded),
			err,
		)
		return nil
	}
	if payload.ID == "" {
		gateway.logger.Printf(
			"[PlaybackGateway] code=item_container_snapshot_unusable reasonCode=response_item_id_missing mappingId=%q itemId=%q contentEncoding=%s bodyBytes=%d",
			routeContext.principal.MappingID,
			routeContext.itemDetailItemID,
			responseSidecarEncodingCode(response.Header.Get("Content-Encoding")),
			len(decoded),
		)
		return nil
	}
	if payload.ID != routeContext.itemDetailItemID {
		responseItemID, responseItemIDTruncated := boundedRequestLogValue(payload.ID, maxProofItemIDBytes)
		gateway.logger.Printf(
			"[PlaybackGateway] code=item_container_snapshot_unusable reasonCode=response_item_id_mismatch mappingId=%q itemId=%q responseItemId=%q responseItemIdTruncated=%t contentEncoding=%s bodyBytes=%d",
			routeContext.principal.MappingID,
			routeContext.itemDetailItemID,
			responseItemID,
			responseItemIDTruncated,
			responseSidecarEncodingCode(response.Header.Get("Content-Encoding")),
			len(decoded),
		)
		return nil
	}
	written := gateway.itemContainers.Record(
		routeContext.principal.MappingID,
		routeContext.itemDetailItemID,
		payload.Container,
		payload.MediaSources,
	)
	if written > 0 {
		gateway.debugf("[PlaybackGateway] level=debug code=item_container_snapshot_recorded mappingId=%s itemId=%s count=%d",
			routeContext.principal.MappingID, routeContext.itemDetailItemID, written)
	}
	return nil
}

// recoverMissingStreamContainer clones a request only when the plain stream
// endpoint lacks every Container key and a recent authorized item snapshot can
// supply the exact MediaSource or item-level value.
func (gateway *Gateway) recoverMissingStreamContainer(request *http.Request, principal embytoken.Principal) (*http.Request, bool) {
	if gateway == nil || gateway.itemContainers == nil || request == nil || request.URL == nil {
		return request, false
	}
	pathInfo := videoPath(request.URL)
	if pathInfo.ItemID == "" || !strings.EqualFold(pathInfo.StreamFileName, "stream") || queryKeyExistsFold(request.URL.Query(), "Container") {
		return request, false
	}
	mediaSourceID, ok := singleBoundedQueryValue(request.URL.Query(), "MediaSourceId", maxProofMediaSourceIDBytes)
	if !ok {
		return request, false
	}
	container, ok := gateway.itemContainers.Lookup(principal.MappingID, pathInfo.ItemID, mediaSourceID)
	if !ok {
		return request, false
	}
	clone := request.Clone(request.Context())
	urlCopy := *request.URL
	separator := ""
	if urlCopy.RawQuery != "" {
		separator = "&"
	}
	urlCopy.RawQuery += separator + "Container=" + url.QueryEscape(container)
	clone.URL = &urlCopy
	return clone, true
}

// queryKeyExistsFold detects any casing of a logical query key without
// normalizing or mutating the forwarded query.
func queryKeyExistsFold(values url.Values, key string) bool {
	for candidateKey := range values {
		if strings.EqualFold(candidateKey, key) {
			return true
		}
	}
	return false
}

// pruneExpiredLocked removes expired entries while the caller holds cache.mu.
func (cache *itemContainerSnapshotCache) pruneExpiredLocked(now time.Time) {
	for key, snapshot := range cache.entries {
		if !snapshot.ExpiresAt.After(now) {
			delete(cache.entries, key)
		}
	}
}

// evictEarliestLocked removes one nearest-expiry entry while cache.mu is held.
func (cache *itemContainerSnapshotCache) evictEarliestLocked() {
	var earliestKey itemContainerSnapshotKey
	var earliest time.Time
	found := false
	for key, snapshot := range cache.entries {
		if !found || snapshot.ExpiresAt.Before(earliest) {
			earliestKey = key
			earliest = snapshot.ExpiresAt
			found = true
		}
	}
	if found {
		delete(cache.entries, earliestKey)
	}
}
