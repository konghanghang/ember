package playbackgateway

import "strings"

// applicationAuthorizationFailure retains only fixed diagnostic labels and
// a zero-based byte offset; -1 means the header-level failure has no position.
type applicationAuthorizationFailure struct {
	reasonCode     string
	field          string
	offset         int
	characterClass string
}

// newApplicationAuthorizationFailure removes all caller-controlled text from
// parser failures, including unknown field names that could contain secrets.
func newApplicationAuthorizationFailure(reasonCode, key, value string, offset int) *applicationAuthorizationFailure {
	field := "other"
	switch strings.ToLower(key) {
	case "":
		field = "none"
	case "client":
		field = "client"
	case "device":
		field = "device"
	case "deviceid":
		field = "device_id"
	case "version":
		field = "version"
	case "userid":
		field = "user_id"
	case "token":
		field = "token"
	}
	return &applicationAuthorizationFailure{
		reasonCode: reasonCode, field: field, offset: offset,
		characterClass: applicationAuthorizationCharacterClass(value, offset),
	}
}

// applicationAuthorizationCharacterClass identifies syntax punctuation without
// revealing any letter, digit, Unicode character or raw escaped value.
func applicationAuthorizationCharacterClass(value string, offset int) string {
	if offset < 0 {
		return "unavailable"
	}
	if offset >= len(value) {
		return "end"
	}
	switch value[offset] {
	case '"':
		return "double_quote"
	case '\'':
		return "single_quote"
	case '\\':
		return "backslash"
	case ',':
		return "comma"
	case ';':
		return "semicolon"
	case '=':
		return "equals"
	case '%':
		return "percent"
	case ' ':
		return "space"
	case '\t':
		return "tab"
	}
	if value[offset] < 0x20 || value[offset] == 0x7f {
		return "control"
	}
	return "other"
}
