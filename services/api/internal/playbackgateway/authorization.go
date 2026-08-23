package playbackgateway

import (
	"net/http"
	"net/url"
	"strings"
	"unicode/utf8"
)

const (
	publicUsersPath                 = "/emby/Users/Public"
	embyAuthorizationHeader         = "X-Emby-Authorization"
	mediaBrowserAuthorizationHeader = "X-MediaBrowser-Authorization"
	mediaBrowserTokenHeader         = "X-MediaBrowser-Token"
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
	applicationHeaderMediaBrowser
)

type accessTokenCandidates struct {
	value   string
	present bool
}

// extractApplicationMetadata accepts exactly one supported application Header
// and returns only non-authoritative audit metadata.
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

// extractProtectedRequestAccessToken resolves every supported Header and query
// carrier to one opaque Token. Multiple carriers are valid only when all
// values are identical, preventing Gateway/Emby identity selection drift.
func extractProtectedRequestAccessToken(request *http.Request) (string, string, bool) {
	if request == nil {
		return "", "token_invalid", false
	}
	candidates, reasonCode := protectedHeaderAccessTokenCandidates(request.Header)
	if reasonCode != "" {
		return "", reasonCode, false
	}
	if request.URL != nil {
		queryCandidates, queryReasonCode := protectedQueryAccessTokenCandidates(request.URL.Query())
		if queryReasonCode != "" {
			return "", queryReasonCode, false
		}
		if queryCandidates.present && !candidates.add(queryCandidates.value) {
			return "", "token_ambiguous", false
		}
	}
	return candidates.result()
}

// extractProtectedAccessToken applies the Header-only subset used by focused
// unit tests and by callers that have no URL query.
func extractProtectedAccessToken(header http.Header) (string, string, bool) {
	candidates, reasonCode := protectedHeaderAccessTokenCandidates(header)
	if reasonCode != "" {
		return "", reasonCode, false
	}
	return candidates.result()
}

// protectedHeaderAccessTokenCandidates collects the two direct Token headers
// and one strict application authorization header without source precedence.
func protectedHeaderAccessTokenCandidates(header http.Header) (accessTokenCandidates, string) {
	var candidates accessTokenCandidates
	for _, headerName := range []string{accessTokenHeader, mediaBrowserTokenHeader} {
		values := header.Values(headerName)
		switch {
		case len(values) > 1:
			return accessTokenCandidates{}, "token_ambiguous"
		case len(values) == 1 && values[0] == "":
			return accessTokenCandidates{}, "token_invalid"
		case len(values) == 1 && !candidates.add(values[0]):
			return accessTokenCandidates{}, "token_ambiguous"
		}
	}
	embeddedToken, embeddedPresent, reasonCode := protectedApplicationAccessToken(header)
	if reasonCode != "" {
		return accessTokenCandidates{}, reasonCode
	}
	if embeddedPresent && !candidates.add(embeddedToken) {
		return accessTokenCandidates{}, "token_ambiguous"
	}
	return candidates, ""
}

// protectedQueryAccessTokenCandidates accepts the exact compatibility aliases
// case-insensitively while treating repeated logical sources as ambiguous.
func protectedQueryAccessTokenCandidates(values url.Values) (accessTokenCandidates, string) {
	var candidates accessTokenCandidates
	seen := make(map[string]struct{}, 4)
	for key, items := range values {
		canonicalKey, supported := canonicalTokenQueryKey(key)
		if !supported {
			continue
		}
		if _, duplicate := seen[canonicalKey]; duplicate {
			return accessTokenCandidates{}, "token_ambiguous"
		}
		seen[canonicalKey] = struct{}{}
		if len(items) != 1 {
			return accessTokenCandidates{}, "token_ambiguous"
		}
		if items[0] == "" {
			return accessTokenCandidates{}, "token_invalid"
		}
		if !candidates.add(items[0]) {
			return accessTokenCandidates{}, "token_ambiguous"
		}
	}
	return candidates, ""
}

// canonicalTokenQueryKey returns only fixed labels and never a Token value.
func canonicalTokenQueryKey(key string) (string, bool) {
	switch strings.ToLower(key) {
	case "api_key":
		return "api_key", true
	case "x-emby-token":
		return "x-emby-token", true
	case "x-mediabrowser-token":
		return "x-mediabrowser-token", true
	case "accesstoken":
		return "accesstoken", true
	default:
		return "", false
	}
}

// add merges a non-empty candidate without allowing conflicting identities.
func (candidates *accessTokenCandidates) add(value string) bool {
	if value == "" {
		return false
	}
	if !candidates.present {
		candidates.value = value
		candidates.present = true
		return true
	}
	return candidates.value == value
}

// result maps an empty candidate set to the fixed missing-token rejection.
func (candidates accessTokenCandidates) result() (string, string, bool) {
	if !candidates.present {
		return "", "token_missing", false
	}
	return candidates.value, "", true
}

// protectedApplicationAccessToken validates an optional application Header
// and returns only its non-empty Token candidate. Empty or missing Token fields
// do not compete with a valid X-Emby-Token source.
func protectedApplicationAccessToken(header http.Header) (string, bool, string) {
	standardValues := header.Values(standardAuthorizationHeader)
	embyValues := header.Values(embyAuthorizationHeader)
	mediaBrowserValues := header.Values(mediaBrowserAuthorizationHeader)
	if len(standardValues)+len(embyValues)+len(mediaBrowserValues) == 0 {
		return "", false, ""
	}
	if len(standardValues)+len(embyValues)+len(mediaBrowserValues) != 1 {
		return "", false, "token_ambiguous"
	}
	rawValue, headerKind, ok := singleApplicationAuthorization(header)
	if !ok {
		return "", false, "token_invalid"
	}
	fields, ok := parseApplicationAuthorizationWithAccessToken(rawValue, headerKind)
	if !ok || fields["Client"] == "" || fields["Device"] == "" || fields["DeviceId"] == "" || fields["Version"] == "" {
		return "", false, "token_invalid"
	}
	token := fields["Token"]
	return token, token != "", ""
}

// singleApplicationAuthorization prevents duplicate or conflicting header
// names from creating parser differences between the gateway and Emby.
func singleApplicationAuthorization(header http.Header) (string, applicationAuthorizationHeader, bool) {
	standardValues := header.Values(standardAuthorizationHeader)
	embyValues := header.Values(embyAuthorizationHeader)
	mediaBrowserValues := header.Values(mediaBrowserAuthorizationHeader)
	if len(standardValues)+len(embyValues)+len(mediaBrowserValues) != 1 {
		return "", 0, false
	}
	if len(standardValues) == 1 {
		return standardValues[0], applicationHeaderStandard, standardValues[0] != ""
	}
	if len(embyValues) == 1 {
		return embyValues[0], applicationHeaderEmby, embyValues[0] != ""
	}
	return mediaBrowserValues[0], applicationHeaderMediaBrowser, mediaBrowserValues[0] != ""
}

// parseApplicationAuthorization parses the fixed Emby quoted field grammar.
// It supports commas inside quoted values but rejects unknown/duplicate fields,
// control characters and all escapes except quoted quote/backslash.
func parseApplicationAuthorization(value string, headerKind applicationAuthorizationHeader) (map[string]string, bool) {
	return parseApplicationAuthorizationWithTokenPolicy(value, headerKind, false)
}

// parseApplicationAuthorizationWithAccessToken accepts a bounded non-empty
// Token for protected authentication and diagnostics. Callers must never log
// the returned map or any value from it.
func parseApplicationAuthorizationWithAccessToken(value string, headerKind applicationAuthorizationHeader) (map[string]string, bool) {
	return parseApplicationAuthorizationWithTokenPolicy(value, headerKind, true)
}

// parseApplicationAuthorizationWithTokenPolicy shares the strict grammar while
// keeping login validation and request diagnostics on separate Token policies.
func parseApplicationAuthorizationWithTokenPolicy(value string, headerKind applicationAuthorizationHeader, allowNonEmptyToken bool) (map[string]string, bool) {
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
		limit, allowEmpty, known := applicationFieldLimit(key, allowNonEmptyToken)
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

// applicationAuthorizationFieldsStart accepts Emby on its documented headers
// and MediaBrowser on the X-Emby shape observed from Infuse or its explicit
// X-MediaBrowser compatibility alias, without arbitrary scheme expansion.
func applicationAuthorizationFieldsStart(value string, headerKind applicationAuthorizationHeader) (int, bool) {
	switch {
	case (headerKind == applicationHeaderStandard || headerKind == applicationHeaderEmby) && strings.HasPrefix(value, embyAuthorizationScheme):
		return len(embyAuthorizationScheme), true
	case (headerKind == applicationHeaderEmby || headerKind == applicationHeaderMediaBrowser) && strings.HasPrefix(value, mediaBrowserAuthorizationScheme):
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
func applicationFieldLimit(key string, allowNonEmptyToken bool) (limit int, allowEmpty, known bool) {
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
		if allowNonEmptyToken {
			return maxApplicationAuthorizationSize, true, true
		}
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
