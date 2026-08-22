package emby

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	serverIdentityPath             = "/emby/System/Info"
	serverIdentityResponseMaxBytes = int64(256 * 1024)
	serverIdentityAPIKeyMaxBytes   = 16 * 1024
	serverIdentityIDMaxBytes       = 64
	serverIdentityVersionMaxBytes  = 32
	serverIdentityNameMaxBytes     = 256
	serverIdentityTimeout          = 10 * time.Second
)

var (
	ErrServerIdentityConfig      = errors.New("emby server identity configuration invalid")
	ErrServerIdentityUnavailable = errors.New("emby server identity unavailable")
	ErrServerIdentityHTTP        = errors.New("emby server identity http failure")
	ErrServerIdentityProtocol    = errors.New("emby server identity protocol invalid")
)

// ServerIdentity is the fixed subset of Emby 4.9.3.0 SystemInfo used to bind
// the gateway to one upstream before it starts listening.
type ServerIdentity struct {
	ID         string `json:"Id"`
	Version    string `json:"Version"`
	ServerName string `json:"ServerName"`
}

// ServerIdentityVerifier performs the one read-only version and server ID
// check required before the playback gateway can accept client traffic.
type ServerIdentityVerifier struct {
	requestURL string
	apiKey     string
	client     *http.Client
}

// NewServerIdentityVerifier validates the upstream boundary without making a
// request. A nil transport uses http.DefaultTransport with redirect following
// disabled by the verifier-owned client.
func NewServerIdentityVerifier(rawURL, apiKey string, transport http.RoundTripper) (*ServerIdentityVerifier, error) {
	requestURL, err := serverIdentityRequestURL(rawURL)
	if err != nil || !validServerIdentityAPIKey(apiKey) {
		return nil, ErrServerIdentityConfig
	}
	if transport == nil {
		transport = http.DefaultTransport
	}
	return &ServerIdentityVerifier{
		requestURL: requestURL,
		apiKey:     apiKey,
		client: &http.Client{
			Transport: transport,
			Timeout:   serverIdentityTimeout,
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
	}, nil
}

// Verify requests the fixed SystemInfo endpoint and returns only validated,
// bounded identity fields. Every error is detached from upstream response data.
func (verifier *ServerIdentityVerifier) Verify(ctx context.Context) (ServerIdentity, error) {
	if verifier == nil || verifier.client == nil || ctx == nil {
		return ServerIdentity{}, ErrServerIdentityConfig
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, verifier.requestURL, nil)
	if err != nil {
		return ServerIdentity{}, ErrServerIdentityConfig
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("X-Emby-Token", verifier.apiKey)
	response, err := verifier.client.Do(request)
	if err != nil {
		return ServerIdentity{}, ErrServerIdentityUnavailable
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return ServerIdentity{}, fmt.Errorf("%w: status=%d", ErrServerIdentityHTTP, response.StatusCode)
	}
	mediaType, _, err := mime.ParseMediaType(response.Header.Get("Content-Type"))
	if err != nil || mediaType != "application/json" {
		return ServerIdentity{}, ErrServerIdentityProtocol
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, serverIdentityResponseMaxBytes+1))
	if err != nil || int64(len(body)) > serverIdentityResponseMaxBytes {
		return ServerIdentity{}, ErrServerIdentityProtocol
	}
	var identity ServerIdentity
	if err := json.Unmarshal(body, &identity); err != nil || !validServerIdentity(identity) {
		return ServerIdentity{}, ErrServerIdentityProtocol
	}
	return identity, nil
}

// serverIdentityRequestURL accepts only a credential-free absolute HTTP(S)
// base URL and appends the versioned Emby route without path normalization.
func serverIdentityRequestURL(rawURL string) (string, error) {
	if rawURL == "" || strings.TrimSpace(rawURL) != rawURL {
		return "", ErrServerIdentityConfig
	}
	parsed, err := url.Parse(rawURL)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" ||
		parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" || parsed.Opaque != "" || parsed.RawPath != "" {
		return "", ErrServerIdentityConfig
	}
	parsed.Path = strings.TrimRight(parsed.Path, "/") + serverIdentityPath
	parsed.ForceQuery = false
	return parsed.String(), nil
}

// validServerIdentityAPIKey preserves the exact opaque key while rejecting
// whitespace normalization and header injection.
func validServerIdentityAPIKey(apiKey string) bool {
	return apiKey != "" && len(apiKey) <= serverIdentityAPIKeyMaxBytes && strings.TrimSpace(apiKey) == apiKey &&
		!strings.ContainsAny(apiKey, "\r\n")
}

// validServerIdentity enforces the same ServerId bound consumed by
// EmbyTokenService and rejects normalized or control-character variants.
func validServerIdentity(identity ServerIdentity) bool {
	return validServerIdentityValue(identity.ID, serverIdentityIDMaxBytes, false) &&
		validServerIdentityValue(identity.Version, serverIdentityVersionMaxBytes, false) &&
		validServerIdentityValue(identity.ServerName, serverIdentityNameMaxBytes, true)
}

// validServerIdentityValue requires exact UTF-8 text so ServerId and version
// comparisons cannot be changed by trimming or line normalization.
func validServerIdentityValue(value string, maxBytes int, allowEmpty bool) bool {
	if value == "" {
		return allowEmpty
	}
	return utf8.ValidString(value) && len(value) <= maxBytes && strings.TrimSpace(value) == value &&
		!strings.ContainsAny(value, "\r\n")
}
