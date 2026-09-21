package playbackgateway

import (
	"bytes"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/konghang/ember/backend/internal/services/embytoken"
)

// diagnosticItemRef correlates client PlaybackInfo, internal lookups and video
// requests without requiring a session or exposing device/login identifiers.
func diagnosticItemRef(p embytoken.Principal, itemID string) string {
	if itemID == "" {
		return "unavailable"
	}
	return diagnosticSessionRef(p, "item:"+itemID)
}

// logPlaybackInfoRequest observes a bounded sidecar only at Debug level. Enum
// states are diagnostic evidence, never authorization or playback intent gates.
func (g *Gateway) logPlaybackInfoRequest(r *http.Request, p embytoken.Principal, itemID, source, state string, body []byte) {
	if !g.isDebugEnabled() {
		return
	}
	fields := map[string]json.RawMessage{}
	if state == "json" {
		if json.Unmarshal(body, &fields) != nil || fields == nil {
			state = "invalid_json"
			fields = nil
		}
	}
	g.debugf("[PlaybackGateway] level=debug code=playback_info_request_observed requestId=%s itemRef=%s itemId=%q source=%s method=%s bodyState=%s isPlayback=%s enableDirectPlay=%s enableDirectStream=%s enableTranscoding=%s autoOpenLiveStream=%s startTimeTicks=%s currentPlaySessionId=%s mediaSourceId=%s deviceProfile=%s",
		diagnosticRequestID(r.Context()), diagnosticItemRef(p, itemID), itemID, source, r.Method, state,
		playbackIntentField(fields, "IsPlayback", "bool"), playbackIntentField(fields, "EnableDirectPlay", "bool"),
		playbackIntentField(fields, "EnableDirectStream", "bool"), playbackIntentField(fields, "EnableTranscoding", "bool"),
		playbackIntentField(fields, "AutoOpenLiveStream", "bool"), playbackIntentField(fields, "StartTimeTicks", "ticks"),
		playbackIntentField(fields, "CurrentPlaySessionId", "string"), playbackIntentField(fields, "MediaSourceId", "string"),
		playbackIntentField(fields, "DeviceProfile", "object"))
}

// playbackIntentField emits fixed states only; string/object contents and
// malformed values never enter logs. Case-ambiguous fields are invalid.
func playbackIntentField(fields map[string]json.RawMessage, name, kind string) string {
	var raw json.RawMessage
	for key, value := range fields {
		if strings.EqualFold(key, name) {
			if raw != nil {
				return "invalid"
			}
			raw = value
		}
	}
	if raw == nil {
		return "missing"
	}
	value := strings.TrimSpace(string(raw))
	if value == "null" {
		return "null"
	}
	switch kind {
	case "bool":
		if value == "true" || value == "false" {
			return value
		}
	case "ticks":
		if n, err := strconv.ParseInt(value, 10, 64); err == nil && n >= 0 {
			if n == 0 {
				return "zero"
			}
			return "positive"
		}
	case "string":
		var text string
		if json.Unmarshal(raw, &text) == nil {
			if text == "" {
				return "empty"
			}
			return "present"
		}
	case "object":
		if bytes.HasPrefix(bytes.TrimSpace(raw), []byte("{")) {
			return "present"
		}
	}
	return "invalid"
}
