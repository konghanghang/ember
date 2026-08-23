package playbackgateway

import (
	"net/http"
	"strings"
	"unicode/utf8"
)

const (
	publicUsersPath                 = "/emby/Users/Public"
	embyAuthorizationHeader         = "X-Emby-Authorization"
	standardAuthorizationHeader     = "Authorization"
	embyAuthorizationScheme         = "Emby "
	mediaBrowserAuthorizationScheme = "MediaBrowser "
	maxApplicationAuthorizationSize = 8 * 1024
	maxApplicationUserIDSize        = 50
	maxApplicationClientSize        = 128
	maxApplicationDeviceSize        = 128
	maxApplicationDeviceIDSize      = 256
	maxApplicationVersionSize       = 64
)

type applicationAuthorizationHeader uint8

const (
	applicationHeaderStandard applicationAuthorizationHeader = iota + 1
	applicationHeaderEmby
)

// extractApplicationMetadata accepts exactly one of the two header names
// fixed by the Emby 4.9 contract and returns only non-authoritative audit metadata.
func extractApplicationMetadata(header http.Header) (AuthenticationMetadata, bool) {
	rawValue, headerKind, ok := singleApplicationAuthorization(header)
	if !ok {
		return AuthenticationMetadata{}, false
	}
	fields, ok := parseApplicationAuthorization(rawValue, headerKind)
	if !ok || fields["Client"] == "" || fields["Device"] == "" || fields["DeviceId"] == "" || fields["Version"] == "" {
		return AuthenticationMetadata{}, false
	}
	return AuthenticationMetadata{DeviceID: fields["DeviceId"], ClientName: fields["Client"]}, true
}

// singleApplicationAuthorization prevents duplicate or conflicting header
// names from creating parser differences between the gateway and Emby.
func singleApplicationAuthorization(header http.Header) (string, applicationAuthorizationHeader, bool) {
	standardValues := header.Values(standardAuthorizationHeader)
	embyValues := header.Values(embyAuthorizationHeader)
	if len(standardValues)+len(embyValues) != 1 {
		return "", 0, false
	}
	if len(standardValues) == 1 {
		return standardValues[0], applicationHeaderStandard, standardValues[0] != ""
	}
	return embyValues[0], applicationHeaderEmby, embyValues[0] != ""
}

// parseApplicationAuthorization parses the fixed Emby quoted field grammar.
// It supports commas inside quoted values but rejects unknown/duplicate fields,
// control characters and all escapes except quoted quote/backslash.
func parseApplicationAuthorization(value string, headerKind applicationAuthorizationHeader) (map[string]string, bool) {
	if len(value) > maxApplicationAuthorizationSize || !utf8.ValidString(value) ||
		strings.ContainsAny(value, "\r\n") {
		return nil, false
	}
	position, ok := applicationAuthorizationFieldsStart(value, headerKind)
	if !ok {
		return nil, false
	}
	fields := make(map[string]string, 6)
	for {
		position = skipOptionalWhitespace(value, position)
		if position >= len(value) {
			return nil, false
		}
		keyStart := position
		for position < len(value) && isApplicationKeyCharacter(value[position]) {
			position++
		}
		key := value[keyStart:position]
		limit, allowEmpty, known := applicationFieldLimit(key)
		if key == "" || !known {
			return nil, false
		}
		if _, duplicate := fields[key]; duplicate {
			return nil, false
		}
		position = skipOptionalWhitespace(value, position)
		if position >= len(value) || value[position] != '=' {
			return nil, false
		}
		position++
		position = skipOptionalWhitespace(value, position)
		fieldValue, nextPosition, ok := parseApplicationQuotedValue(value, position)
		if !ok || !validApplicationFieldValue(fieldValue, limit, allowEmpty) {
			return nil, false
		}
		fields[key] = fieldValue
		position = skipOptionalWhitespace(value, nextPosition)
		if position == len(value) {
			break
		}
		if value[position] != ',' {
			return nil, false
		}
		position++
	}
	return fields, true
}

// applicationAuthorizationFieldsStart accepts Emby on either documented
// header and MediaBrowser only on the X-Emby-Authorization shape observed from
// Infuse 8.5, without case folding or arbitrary scheme expansion.
func applicationAuthorizationFieldsStart(value string, headerKind applicationAuthorizationHeader) (int, bool) {
	switch {
	case strings.HasPrefix(value, embyAuthorizationScheme):
		return len(embyAuthorizationScheme), true
	case headerKind == applicationHeaderEmby && strings.HasPrefix(value, mediaBrowserAuthorizationScheme):
		return len(mediaBrowserAuthorizationScheme), true
	default:
		return 0, false
	}
}

// parseApplicationQuotedValue returns one decoded quoted-string and the first
// byte after its closing quote.
func parseApplicationQuotedValue(value string, position int) (string, int, bool) {
	if position >= len(value) || value[position] != '"' {
		return "", position, false
	}
	position++
	var builder strings.Builder
	for position < len(value) {
		character := value[position]
		switch {
		case character == '"':
			return builder.String(), position + 1, true
		case character == '\\':
			position++
			if position >= len(value) || (value[position] != '\\' && value[position] != '"') {
				return "", position, false
			}
			builder.WriteByte(value[position])
		case character < 0x20 || character == 0x7f:
			return "", position, false
		default:
			builder.WriteByte(character)
		}
		position++
	}
	return "", position, false
}

// validApplicationFieldValue applies field-specific byte bounds and rejects
// surrounding whitespace that would create a second normalized identity.
func validApplicationFieldValue(value string, limit int, allowEmpty bool) bool {
	if value == "" {
		return allowEmpty
	}
	return limit > 0 && len(value) <= limit && strings.TrimSpace(value) == value
}

// applicationFieldLimit keeps the accepted schema immutable and makes empty
// value semantics explicit for UserId and the pre-login Token placeholder.
func applicationFieldLimit(key string) (limit int, allowEmpty, known bool) {
	switch key {
	case "UserId":
		return maxApplicationUserIDSize, true, true
	case "Client":
		return maxApplicationClientSize, false, true
	case "Device":
		return maxApplicationDeviceSize, false, true
	case "DeviceId":
		return maxApplicationDeviceIDSize, false, true
	case "Version":
		return maxApplicationVersionSize, false, true
	case "Token":
		return 0, true, true
	default:
		return 0, false, false
	}
}

// skipOptionalWhitespace accepts only HTTP space and tab between grammar
// tokens; whitespace inside quoted values remains subject to field validation.
func skipOptionalWhitespace(value string, position int) int {
	for position < len(value) && (value[position] == ' ' || value[position] == '\t') {
		position++
	}
	return position
}

// isApplicationKeyCharacter limits field names to the ASCII token subset used
// by the fixed Emby authorization schema.
func isApplicationKeyCharacter(character byte) bool {
	return character >= 'A' && character <= 'Z' || character >= 'a' && character <= 'z' || character >= '0' && character <= '9'
}
