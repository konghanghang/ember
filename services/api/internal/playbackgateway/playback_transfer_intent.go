package playbackgateway

import (
	"bytes"
	"encoding/json"
	"io"
	"strings"
)

// playbackTransferIntent carries only validated POST metadata; it is neither
// a media proof nor permission until a successful matching response arrives.
type playbackTransferIntent struct {
	requested     bool
	mediaSourceID string
}

// parsePlaybackTransferIntent rejects ambiguous logical fields instead of
// relying on encoding/json's last-key-wins behavior or the diagnostic map.
func parsePlaybackTransferIntent(body []byte) playbackTransferIntent {
	fields, ok := uniquePlaybackJSONFields(body, "IsPlayback", "MediaSourceId", "UserId")
	if !ok || !bytes.Equal(bytes.TrimSpace(fields["isplayback"]), []byte("true")) {
		return playbackTransferIntent{}
	}
	intent := playbackTransferIntent{requested: true}
	if raw, present := fields["mediasourceid"]; present {
		if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) || json.Unmarshal(raw, &intent.mediaSourceID) != nil ||
			!validProofValue(intent.mediaSourceID, maxProofMediaSourceIDBytes, true) {
			return playbackTransferIntent{}
		}
	}
	return intent
}

// validPlaybackIntentRevocation supplements the existing session snapshot
// validation without changing its lease semantics. Null is not an omitted
// source: accepting it would revoke every source of a session ambiguously.
func validPlaybackIntentRevocation(body []byte) bool {
	fields, ok := uniquePlaybackJSONFields(body, "ItemId", "MediaSourceId", "PlaySessionId")
	if !ok {
		return false
	}
	for _, raw := range fields {
		if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
			return false
		}
	}
	return true
}

// uniquePlaybackJSONFields decodes selected top-level fields with a token
// stream so exact duplicates and case variants cannot collapse into a map.
// Unknown fields stay transparent and nested names do not become candidates.
func uniquePlaybackJSONFields(body []byte, names ...string) (map[string]json.RawMessage, bool) {
	decoder := json.NewDecoder(bytes.NewReader(body))
	opening, err := decoder.Token()
	if err != nil || opening != json.Delim('{') {
		return nil, false
	}
	fields := make(map[string]json.RawMessage, len(names))
	for decoder.More() {
		token, err := decoder.Token()
		if err != nil {
			return nil, false
		}
		key, ok := token.(string)
		if !ok {
			return nil, false
		}
		var raw json.RawMessage
		if decoder.Decode(&raw) != nil {
			return nil, false
		}
		for _, name := range names {
			if strings.EqualFold(key, name) {
				canonical := strings.ToLower(name)
				if _, duplicate := fields[canonical]; duplicate {
					return nil, false
				}
				fields[canonical] = raw
				break
			}
		}
	}
	closing, err := decoder.Token()
	if err != nil || closing != json.Delim('}') {
		return nil, false
	}
	if _, err := decoder.Token(); err != io.EOF {
		return nil, false
	}
	return fields, true
}
