package playbackgateway

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"errors"
	"io"
	"mime"
	"strings"
)

var errAuthenticationResponseInvalid = errors.New("invalid authentication response")

// decodeAuthenticationResult reads the official JSON/XML success DTO from an
// already bounded sidecar. XML must contain exactly one AuthenticationResult;
// no request Accept/header or original response bytes are changed.
func decodeAuthenticationResult(body []byte, contentType string) (authenticationResult, error) {
	var result authenticationResult
	mediaType, _, _ := mime.ParseMediaType(contentType)
	var err error
	switch mediaType {
	case "application/xml", "text/xml":
		payload := struct {
			XMLName xml.Name `xml:"AuthenticationResult"`
			authenticationResult
		}{}
		decoder := xml.NewDecoder(bytes.NewReader(body))
		if err = decoder.Decode(&payload); err == nil {
			err = authenticationXMLEnd(decoder)
		}
		result = payload.authenticationResult
	default:
		// Preserve the existing JSON sidecar behavior even when an upstream
		// omits or mislabels Content-Type; XML negotiation is additive.
		err = json.Unmarshal(body, &result)
	}
	return result, err
}

// authenticationXMLEnd rejects additional roots or trailing data instead of
// mapping an identity from just the first fragment of an invalid XML document.
func authenticationXMLEnd(decoder *xml.Decoder) error {
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		switch value := token.(type) {
		case xml.CharData:
			if len(bytes.TrimSpace(value)) != 0 {
				return errAuthenticationResponseInvalid
			}
		case xml.Comment, xml.ProcInst:
		default:
			return errAuthenticationResponseInvalid
		}
	}
}

// authenticationResultMetadata prefers Emby's session metadata, with a bounded
// request-side fallback for older responses. Invalid optional metadata never
// invalidates a successful identity or reaches persistence/logs unbounded.
func authenticationResultMetadata(result authenticationResult, fallback AuthenticationMetadata) AuthenticationMetadata {
	deviceID := strings.TrimSpace(result.SessionInfo.DeviceID)
	if validApplicationQueryValue(deviceID, maxApplicationDeviceIDSize) {
		fallback.DeviceID = deviceID
	}
	client := strings.TrimSpace(result.SessionInfo.Client)
	if validApplicationQueryValue(client, maxApplicationClientSize) {
		fallback.ClientName = client
	}
	return fallback
}
