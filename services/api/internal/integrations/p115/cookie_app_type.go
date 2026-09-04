package p115

import (
	"fmt"
	"net/http"
	"strings"
)

var cookieAppTypeBySSOEnt = map[string]string{
	"A1": "web",
	"D1": "ios",
	"D2": "bios",
	"D3": "115ios",
	"F1": "android",
	"F2": "bandroid",
	"F3": "115android",
	"H1": "ipad",
	"H2": "bipad",
	"H3": "115ipad",
	"I1": "tv",
	"I2": "apple_tv",
	"M1": "qandroid",
	"N1": "qios",
	"O1": "qipad",
	"P1": "os_windows",
	"P2": "os_mac",
	"P3": "os_linux",
	"R1": "wechatmini",
	"R2": "alipaymini",
	"S1": "harmony",
}

// DetectCookieAppType maps the UID ssoent segment to a stable client name
// without sending the credential to 115. Unknown codes remain unresolved so
// callers can require an explicit compatibility fallback.
func DetectCookieAppType(cookieHeader string) (string, bool) {
	uid, ok := singleCookieUID(cookieHeader)
	if !ok {
		return "", false
	}
	parts := strings.Split(uid, "_")
	if len(parts) < 2 {
		return "", false
	}
	ssoent := strings.ToUpper(strings.TrimSpace(parts[1]))
	if ssoent == "" {
		return "", false
	}
	appType, ok := cookieAppTypeBySSOEnt[ssoent]
	return appType, ok
}

// DetectPersonalCookieAppType validates the single Cookie UID locally and
// returns a stable diagnostic client type without calling 115.
func DetectPersonalCookieAppType(cookieHeader string) (string, error) {
	if _, err := parseCookieProviderUserID(cookieHeader); err != nil {
		return "", err
	}
	uid, ok := singleCookieUID(cookieHeader)
	if !ok {
		return "", ErrCredentialRejected
	}
	parts := strings.Split(uid, "_")
	if len(parts) < 2 || strings.TrimSpace(parts[1]) == "" {
		return "", fmt.Errorf("%w: cookie UID ssoent is missing", ErrCredentialRejected)
	}
	if appType, ok := cookieAppTypeBySSOEnt[strings.ToUpper(strings.TrimSpace(parts[1]))]; ok {
		return appType, nil
	}
	return "unknown", nil
}

func singleCookieUID(cookieHeader string) (string, bool) {
	request := &http.Request{Header: make(http.Header)}
	request.Header.Set("Cookie", strings.TrimSpace(cookieHeader))
	var uidValues []string
	for _, cookie := range request.Cookies() {
		if cookie.Name == "UID" {
			uidValues = append(uidValues, strings.TrimSpace(cookie.Value))
		}
	}
	if len(uidValues) != 1 || uidValues[0] == "" {
		return "", false
	}
	return uidValues[0], true
}
