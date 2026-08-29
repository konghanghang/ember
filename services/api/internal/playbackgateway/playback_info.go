package playbackgateway

import (
	"bytes"
	"encoding/json"
	"io"
	"mime"
	"net/http"
	"net/url"
	"strings"

	"github.com/konghang/ember/backend/internal/services/embytoken"
)

const (
	defaultPlaybackInfoRequestMaxSize  = int64(1 << 20)
	defaultPlaybackInfoResponseMaxSize = int64(2 << 20)
	maxPlaybackInfoMediaSources        = 64
)

type playbackInfoRequestPayload struct {
	UserID string `json:"UserId"`
}

type playbackInfoResponsePayload struct {
	MediaSources  []playbackInfoMediaSource `json:"MediaSources"`
	PlaySessionID string                    `json:"PlaySessionId"`
	ErrorCode     string                    `json:"ErrorCode"`
}

type playbackInfoMediaSource struct {
	ID                   string `json:"Id"`
	ItemID               string `json:"ItemId"`
	Path                 string `json:"Path"`
	Container            string `json:"Container"`
	DirectStreamURL      string `json:"DirectStreamUrl"`
	AddAPIKeyToDirectURL bool   `json:"AddApiKeyToDirectStreamUrl"`
	Size                 *int64 `json:"Size"`
	IsRemote             bool   `json:"IsRemote"`
	SupportsDirectPlay   bool   `json:"SupportsDirectPlay"`
	SupportsDirectStream bool   `json:"SupportsDirectStream"`
	SupportsTranscoding  bool   `json:"SupportsTranscoding"`
}

type playbackInfoMediaSourceObservation struct {
	MediaSourceID        string
	MediaPath            string
	PathPresent          bool
	PathTruncated        bool
	Size                 int64
	SizePresent          bool
	SupportsDirectPlay   bool
	SupportsDirectStream bool
	ProofAccepted        bool
	ProofRejectReason    string
}

// preparePlaybackInfoRequest records only bounded request metadata in context.
// Invalid or mismatched requests remain transparent but become proof-ineligible.
func (gateway *Gateway) preparePlaybackInfoRequest(request *http.Request, principal embytoken.Principal) (string, bool) {
	itemID := playbackInfoItemID(request.URL)
	if itemID == "" || principal.MappingID == "" || principal.User.ID == "" || principal.User.EmbyID == "" {
		return itemID, false
	}
	switch request.Method {
	case http.MethodGet:
		userID, ok := singleBoundedQueryValue(request.URL.Query(), "UserId", maxApplicationUserIDSize)
		return itemID, ok && userID == principal.User.EmbyID
	case http.MethodPost:
		return itemID, gateway.inspectPlaybackInfoPostRequest(request, principal.User.EmbyID)
	default:
		return itemID, false
	}
}

// inspectPlaybackInfoPostRequest restores the exact body after reading a
// bounded JSON copy and validates only the optional UserId field.
func (gateway *Gateway) inspectPlaybackInfoPostRequest(request *http.Request, embyUserID string) bool {
	if request.Body == nil {
		return false
	}
	mediaType, _, err := mime.ParseMediaType(request.Header.Get("Content-Type"))
	if err != nil || mediaType != "application/json" {
		return false
	}
	originalBody := request.Body
	prefix, readErr := io.ReadAll(io.LimitReader(originalBody, gateway.maxPlaybackInfoRequestBytes+1))
	request.Body = &replayedBody{Reader: io.MultiReader(bytes.NewReader(prefix), originalBody), closer: originalBody}
	if readErr != nil || int64(len(prefix)) > gateway.maxPlaybackInfoRequestBytes {
		return false
	}
	var payload playbackInfoRequestPayload
	if err := json.Unmarshal(prefix, &payload); err != nil {
		return false
	}
	return payload.UserID == "" || payload.UserID == embyUserID
}

// observePlaybackInfoResponse records proofs from an exact successful response
// while always restoring and returning the original upstream bytes.
func (gateway *Gateway) observePlaybackInfoResponse(response *http.Response, routeContext requestRouteContext) error {
	if !routeContext.playbackInfoEligible || routeContext.principal == nil {
		return nil
	}
	gateway.proofs.InvalidateItem(routeContext.principal.MappingID, routeContext.playbackInfoItemID)
	if response.StatusCode != http.StatusOK {
		return nil
	}
	mediaType, _, err := mime.ParseMediaType(response.Header.Get("Content-Type"))
	if err != nil || mediaType != "application/json" || response.Body == nil {
		gateway.logger.Printf("[PlaybackGateway] code=playback_info_response_invalid")
		return nil
	}
	originalBody := response.Body
	prefix, readErr := io.ReadAll(io.LimitReader(originalBody, gateway.maxPlaybackInfoResponseBytes+1))
	response.Body = &replayedBody{Reader: io.MultiReader(bytes.NewReader(prefix), originalBody), closer: originalBody}
	if readErr != nil {
		gateway.logger.Printf("[PlaybackGateway] code=playback_info_response_read_failed errorType=%T", readErr)
		return nil
	}
	if int64(len(prefix)) > gateway.maxPlaybackInfoResponseBytes {
		gateway.logger.Printf("[PlaybackGateway] code=playback_info_response_too_large")
		return nil
	}
	decodedPrefix, decodeErr := decodeResponseSidecar(prefix, response.Header.Get("Content-Encoding"), gateway.maxPlaybackInfoResponseBytes)
	if decodeErr != nil {
		gateway.logger.Printf(
			"[PlaybackGateway] code=playback_info_response_decode_failed contentEncoding=%s reasonCode=%s errorType=%T",
			responseSidecarEncodingCode(response.Header.Get("Content-Encoding")),
			responseSidecarDecodeReasonCode(decodeErr),
			decodeErr,
		)
		return nil
	}
	proofs, observations, ok := buildPlaybackProofs(decodedPrefix, routeContext)
	gateway.logPlaybackInfoMediaSourceObservations(
		routeContext.principal.MappingID,
		routeContext.playbackInfoItemID,
		observations,
	)
	if !ok {
		gateway.logger.Printf("[PlaybackGateway] code=playback_info_response_unusable")
		return nil
	}
	written := gateway.proofs.Record(proofs)
	if written == 0 {
		gateway.logger.Printf("[PlaybackGateway] code=playback_info_proof_rejected")
		return nil
	}
	gateway.logger.Printf("[PlaybackGateway] code=playback_info_proof_recorded mappingId=%s itemId=%s count=%d",
		routeContext.principal.MappingID, routeContext.playbackInfoItemID, written)
	return nil
}

// logPlaybackInfoMediaSourceObservations records each observed Emby path and
// the exact proof acceptance boundary before any cache write.
func (gateway *Gateway) logPlaybackInfoMediaSourceObservations(
	mappingID string,
	itemID string,
	observations []playbackInfoMediaSourceObservation,
) {
	if gateway == nil || gateway.logger == nil {
		return
	}
	for _, observation := range observations {
		gateway.logger.Printf(
			"[PlaybackGateway] code=playback_info_media_source_observed mappingId=%q itemId=%q mediaSourceId=%q mediaPath=%q pathPresent=%t pathTruncated=%t sizePresent=%t size=%d supportsDirectPlay=%t supportsDirectStream=%t proofAccepted=%t proofRejectReason=%s",
			mappingID,
			itemID,
			observation.MediaSourceID,
			observation.MediaPath,
			observation.PathPresent,
			observation.PathTruncated,
			observation.SizePresent,
			observation.Size,
			observation.SupportsDirectPlay,
			observation.SupportsDirectStream,
			observation.ProofAccepted,
			observation.ProofRejectReason,
		)
	}
}

// buildPlaybackProofs validates the response-level identity once and produces
// one observation per unique MediaSource and one proof per accepted source.
func buildPlaybackProofs(
	body []byte,
	routeContext requestRouteContext,
) ([]PlaybackProof, []playbackInfoMediaSourceObservation, bool) {
	var payload playbackInfoResponsePayload
	if err := json.Unmarshal(body, &payload); err != nil || payload.ErrorCode != "" ||
		!validProofValue(payload.PlaySessionID, maxProofPlaySessionIDBytes, false) ||
		len(payload.MediaSources) == 0 || len(payload.MediaSources) > maxPlaybackInfoMediaSources ||
		routeContext.principal == nil {
		return nil, nil, false
	}
	seen := make(map[string]struct{}, len(payload.MediaSources))
	proofs := make([]PlaybackProof, 0, len(payload.MediaSources))
	observations := make([]playbackInfoMediaSourceObservation, 0, len(payload.MediaSources))
	principal := routeContext.principal
	for _, source := range payload.MediaSources {
		if !validProofValue(source.ID, maxProofMediaSourceIDBytes, false) {
			continue
		}
		if _, duplicate := seen[source.ID]; duplicate {
			return nil, nil, false
		}
		seen[source.ID] = struct{}{}
		mediaPath, pathTruncated := boundedRequestLogValue(source.Path, maxProofPathBytes)
		observation := playbackInfoMediaSourceObservation{
			MediaSourceID:        source.ID,
			MediaPath:            mediaPath,
			PathPresent:          source.Path != "",
			PathTruncated:        pathTruncated,
			SizePresent:          source.Size != nil,
			SupportsDirectPlay:   source.SupportsDirectPlay,
			SupportsDirectStream: source.SupportsDirectStream,
		}
		if source.Size != nil {
			observation.Size = *source.Size
		}
		if source.ItemID != "" && source.ItemID != routeContext.playbackInfoItemID {
			observation.ProofRejectReason = "item_mismatch"
			observations = append(observations, observation)
			continue
		}
		proof := PlaybackProof{
			MappingID: principal.MappingID, ServerID: principal.ServerID,
			UserID: principal.User.ID, EmbyUserID: principal.User.EmbyID,
			DeviceID: principal.DeviceID, ClientName: principal.ClientName,
			ItemID: routeContext.playbackInfoItemID, MediaSourceID: source.ID,
			PlaySessionID: payload.PlaySessionID, Path: source.Path, Size: observation.Size,
			Container: source.Container, IsRemote: source.IsRemote,
			SupportsDirectPlay: source.SupportsDirectPlay, SupportsDirectStream: source.SupportsDirectStream,
			SupportsTranscoding: source.SupportsTranscoding,
		}
		observation.ProofRejectReason = playbackProofRejectionReason(proof)
		if observation.ProofRejectReason == "" {
			observation.ProofAccepted = true
			observation.ProofRejectReason = "none"
			proofs = append(proofs, proof)
		}
		observations = append(observations, observation)
	}
	return proofs, observations, len(proofs) > 0
}

// playbackInfoItemID matches the fixed case-insensitive PlaybackInfo segments
// with one unescaped, bounded item segment.
func playbackInfoItemID(requestURL *url.URL) string {
	if requestURL == nil || requestURL.EscapedPath() != requestURL.Path {
		return ""
	}
	segments := strings.Split(requestURL.Path, "/")
	if len(segments) != 5 || segments[0] != "" || !strings.EqualFold(segments[1], "emby") || !strings.EqualFold(segments[2], "Items") ||
		!strings.EqualFold(segments[4], "PlaybackInfo") || !validProofValue(segments[3], maxProofItemIDBytes, false) {
		return ""
	}
	return segments[3]
}
