package playbackgateway

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"mime"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"
)

// imageCacheRequestKey accepts only exact, already gated item images. Hashing
// all representation headers conservatively isolates Vary and credential state.
func imageCacheRequestKey(request *http.Request) (string, bool, bool) {
	if request == nil || request.URL == nil || request.URL.EscapedPath() != request.URL.Path ||
		!isEmbyItemImagePath(request.URL.Path) || (request.Method != http.MethodGet && request.Method != http.MethodHead) ||
		request.ContentLength != 0 || request.Body != nil && request.Body != http.NoBody || request.Header.Get("Upgrade") != "" ||
		request.URL.User != nil || request.URL.Opaque != "" || request.URL.Fragment != "" {
		return "", false, false
	}
	for _, key := range []string{"Range", "If-Range", "If-None-Match", "If-Modified-Since", "If-Match", "If-Unmodified-Since"} {
		if len(request.Header.Values(key)) != 0 {
			return "", false, false
		}
	}
	force, allowed := imageRequestCachePolicy(request.Header)
	if !allowed {
		return "", false, false
	}
	route, ok := request.Context().Value(requestRouteContextKey{}).(requestRouteContext)
	if !ok {
		return "", false, false
	}
	digest := sha256.New()
	write := func(value string) { _, _ = fmt.Fprintf(digest, "%d:%s", len(value), value) }
	if p := route.principal; p != nil {
		if p.ServerID == "" || p.MappingID == "" || p.User.ID == "" {
			return "", false, false
		}
		for _, field := range []string{"protected", p.ServerID, p.User.ID, p.MappingID, p.DeviceID, p.ClientName} {
			write(field)
		}
	} else {
		if route.kind != routeItemImage || !isAnonymousItemImageRequest(request) {
			return "", false, false
		}
		write("anonymous")
	}
	for _, field := range []string{request.URL.Scheme, request.URL.Host, request.Host, request.URL.Path, request.URL.RawQuery} {
		write(field)
	}
	keys := make([]string, 0, len(request.Header))
	for key := range request.Header {
		switch http.CanonicalHeaderKey(key) {
		case "Connection", "Proxy-Connection", "Keep-Alive", "Proxy-Authenticate", "Proxy-Authorization", "Te", "Trailer", "Transfer-Encoding", "Upgrade", "Cache-Control", "Pragma":
			continue
		}
		keys = append(keys, key)
	}
	sort.Strings(keys)
	write(strconv.Itoa(len(keys)))
	for _, key := range keys {
		write(key)
		write(strconv.Itoa(len(request.Header[key])))
		for _, value := range request.Header[key] {
			write(value)
		}
	}
	return hex.EncodeToString(digest.Sum(nil)), force, true
}

// imageRequestCachePolicy honors the common forced-validation directives;
// unfamiliar request restrictions bypass caching rather than being ignored.
func imageRequestCachePolicy(header http.Header) (bool, bool) {
	force := false
	for _, raw := range header.Values("Cache-Control") {
		for _, directive := range strings.Split(raw, ",") {
			name, value, hasValue := strings.Cut(strings.TrimSpace(directive), "=")
			switch strings.ToLower(name) {
			case "no-cache":
				if hasValue {
					return false, false
				}
				force = true
			case "max-age":
				if !hasValue || strings.TrimSpace(value) != "0" {
					return false, false
				}
				force = true
			default:
				return false, false
			}
		}
	}
	for _, pragma := range header.Values("Pragma") {
		if !strings.EqualFold(strings.TrimSpace(pragma), "no-cache") {
			return false, false
		}
		force = true
	}
	return force, true
}

// imageResponseEligible rejects responses that cannot be safely retained or
// shared, before any body buffering or upstream cleanup changes its reader.
func imageResponseEligible(response *http.Response) bool {
	if response == nil || response.StatusCode != http.StatusOK || response.Header.Get("Content-Range") != "" ||
		len(response.Header.Values("Set-Cookie")) != 0 || len(response.Trailer) != 0 || response.Header.Get("Trailer") != "" {
		return false
	}
	contentType, _, err := mime.ParseMediaType(response.Header.Get("Content-Type"))
	if err != nil || !strings.HasPrefix(contentType, "image/") {
		return false
	}
	for _, raw := range response.Header.Values("Vary") {
		for _, field := range strings.Split(raw, ",") {
			field = strings.TrimSpace(field)
			if field == "" || field == "*" || !validImageVaryField(field) {
				return false
			}
			switch http.CanonicalHeaderKey(field) {
			case "Cache-Control", "Pragma", "Connection", "Proxy-Connection", "Keep-Alive", "Proxy-Authenticate", "Proxy-Authorization", "Te", "Trailer", "Transfer-Encoding", "Upgrade":
				return false
			}
		}
	}
	headerBytes := 0
	for key, values := range response.Header {
		headerBytes += len(key)
		for _, value := range values {
			headerBytes += len(value)
		}
	}
	if headerBytes > 16<<10 {
		return false
	}
	_, _, ok := imageResponseCachePolicy(response.Header)
	return ok
}

// validImageVaryField accepts HTTP token field names only; every named value
// is already part of the conservative request-header digest.
func validImageVaryField(value string) bool {
	for _, char := range value {
		if char >= 'a' && char <= 'z' || char >= 'A' && char <= 'Z' || char >= '0' && char <= '9' || strings.ContainsRune("!#$%&'*+-.^_`|~", char) {
			continue
		}
		return false
	}
	return value != ""
}

// imageResponseCachePolicy supports explicit HTTP freshness without guessing
// an expiry; unknown/qualified directives conservatively disable this cache.
func imageResponseCachePolicy(header http.Header) (time.Duration, bool, bool) {
	var maxAge, sharedAge time.Duration
	var hasAge, hasShared, noCache bool
	seen := make(map[string]bool)
	for _, raw := range header.Values("Cache-Control") {
		for _, directive := range strings.Split(raw, ",") {
			name, value, hasValue := strings.Cut(strings.TrimSpace(directive), "=")
			name = strings.ToLower(name)
			if seen[name] {
				return 0, false, false
			}
			seen[name] = true
			switch name {
			case "public", "must-revalidate", "proxy-revalidate", "immutable", "no-transform":
				if hasValue {
					return 0, false, false
				}
			case "no-cache":
				if hasValue {
					return 0, false, false
				}
				noCache = true
			case "max-age", "s-maxage":
				value = strings.TrimSpace(value)
				if len(value) >= 2 && value[0] == '"' && value[len(value)-1] == '"' {
					value = value[1 : len(value)-1]
				}
				seconds, ok := imageDeltaSeconds(value)
				if !hasValue || !ok {
					return 0, false, false
				}
				if name == "max-age" {
					maxAge, hasAge = seconds, true
				} else {
					sharedAge, hasShared = seconds, true
				}
			default:
				return 0, false, false
			}
		}
	}
	if hasShared {
		maxAge, hasAge = sharedAge, true
	}
	if noCache {
		return 0, true, true
	}
	return maxAge, hasAge, true
}

// imageDeltaSeconds parses bounded nonnegative HTTP seconds without allowing
// arithmetic overflow to turn a very old response into a fresh entry.
func imageDeltaSeconds(value string) (time.Duration, bool) {
	if value == "" {
		return 0, false
	}
	for _, char := range value {
		if char < '0' || char > '9' {
			return 0, false
		}
	}
	seconds, err := strconv.ParseUint(value, 10, 63)
	if err != nil || seconds > uint64((1<<63-1)/int64(time.Second)) {
		return 0, false
	}
	return time.Duration(seconds) * time.Second, true
}

// imageFromResponse builds an immutable entry with corrected age. An image
// without explicit freshness is retained only when it can be revalidated.
func imageFromResponse(response *http.Response, body []byte, now, started time.Time) (*cachedImage, bool) {
	if !imageResponseEligible(response) {
		return nil, false
	}
	lifetime, explicit, _ := imageResponseCachePolicy(response.Header)
	age := time.Duration(0)
	if values := response.Header.Values("Age"); len(values) != 0 {
		parsed, ok := imageDeltaSeconds(response.Header.Get("Age"))
		if len(values) != 1 || !ok {
			return nil, false
		}
		age = parsed
	}
	if delay := now.Sub(started); delay > 0 {
		if delay > time.Duration(1<<63-1)-age {
			age = time.Duration(1<<63 - 1)
		} else {
			age += delay
		}
	}
	date := now
	if raw := response.Header.Get("Date"); raw != "" {
		var err error
		date, err = http.ParseTime(raw)
		if err != nil {
			return nil, false
		}
		if apparent := now.Sub(date); apparent > age {
			age = apparent
		}
	}
	if !explicit && response.Header.Get("Expires") != "" {
		expires, err := http.ParseTime(response.Header.Get("Expires"))
		if err != nil {
			return nil, false
		}
		lifetime, explicit = expires.Sub(date), true
	}
	validator := response.Header.Get("ETag") != "" || response.Header.Get("Last-Modified") != ""
	freshFor := time.Duration(0)
	if explicit && lifetime > age {
		freshFor = lifetime - age
	}
	if freshFor == 0 && !validator {
		return nil, false
	}
	if freshFor > imageCacheFreshLimit {
		freshFor = imageCacheFreshLimit
	}
	return &cachedImage{header: response.Header.Clone(), body: body, storedAt: now,
		freshUntil: now.Add(freshFor), retainUntil: now.Add(imageCacheRetention), age: age}, true
}
