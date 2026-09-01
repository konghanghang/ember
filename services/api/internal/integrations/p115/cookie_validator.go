package p115

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/konghang/ember/backend/internal/common/upstream"
)

const (
	cookieLoginStatusURL = "https://my.115.com/?ct=guide&ac=status"
	maxLoginStatusBody   = 64 * 1024
)

type httpDoer interface {
	Do(req *http.Request) (*http.Response, error)
}

// CookieCredentialValidator validates a legacy Cookie against the fixed login-status endpoint.
type CookieCredentialValidator struct {
	client   httpDoer
	endpoint *url.URL
}

// NewCookieCredentialValidator builds the production validator without performing any network call.
func NewCookieCredentialValidator() *CookieCredentialValidator {
	client := newCookieHTTPClient()
	validator, err := newCookieCredentialValidator(client, cookieLoginStatusURL)
	if err != nil {
		panic("invalid fixed 115 login status URL: " + err.Error())
	}
	return validator
}

func newCookieCredentialValidator(client httpDoer, endpoint string) (*CookieCredentialValidator, error) {
	if client == nil {
		return nil, fmt.Errorf("%w: http client is nil", ErrProviderUnavailable)
	}
	parsed, err := url.ParseRequestURI(strings.TrimSpace(endpoint))
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return nil, fmt.Errorf("%w: invalid endpoint", ErrProviderProtocol)
	}
	query := parsed.Query()
	query.Set("ct", "guide")
	query.Set("ac", "status")
	parsed.RawQuery = query.Encode()
	return &CookieCredentialValidator{client: client, endpoint: parsed}, nil
}

// ValidateCredential checks remote login state and returns the normalized UID only after success.
func (v *CookieCredentialValidator) ValidateCredential(ctx context.Context, credential Credential) (AccountIdentity, error) {
	providerUserID, err := parseCookieProviderUserID(credential.Cookie)
	if err != nil {
		return AccountIdentity{}, err
	}
	if strings.TrimSpace(credential.UserAgent) == "" || strings.ContainsAny(credential.Cookie+credential.UserAgent, "\r\n") {
		return AccountIdentity{}, ErrCredentialRejected
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, v.endpoint.String(), nil)
	if err != nil {
		return AccountIdentity{}, fmt.Errorf("%w: request build failed", ErrProviderProtocol)
	}
	req.Header.Set("Accept", "*/*")
	req.Header.Set("Cookie", credential.Cookie)
	req.Header.Set("User-Agent", credential.UserAgent)

	resp, err := v.client.Do(req)
	if err != nil {
		return AccountIdentity{}, fmt.Errorf("%w: %v", ErrProviderUnavailable, upstream.SafeUpstreamError(err, "p115"))
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return AccountIdentity{}, fmt.Errorf("%w: %v", ErrProviderUnavailable, upstream.SafeUpstreamHTTPError("p115", resp.StatusCode))
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxLoginStatusBody+1))
	if err != nil {
		return AccountIdentity{}, fmt.Errorf("%w: response read failed", ErrProviderProtocol)
	}
	if len(body) > maxLoginStatusBody {
		return AccountIdentity{}, fmt.Errorf("%w: response too large", ErrProviderProtocol)
	}
	var payload struct {
		State *bool `json:"state"`
	}
	if err := json.Unmarshal(body, &payload); err != nil || payload.State == nil {
		return AccountIdentity{}, fmt.Errorf("%w: invalid login status response", ErrProviderProtocol)
	}
	if !*payload.State {
		return AccountIdentity{}, ErrCredentialRejected
	}
	return AccountIdentity{ProviderUserID: providerUserID}, nil
}

func parseCookieProviderUserID(cookieHeader string) (string, error) {
	uid, ok := singleCookieUID(cookieHeader)
	if !ok {
		return "", ErrCredentialRejected
	}
	rawUserID, _, _ := strings.Cut(uid, "_")
	userID, err := strconv.ParseUint(rawUserID, 10, 64)
	if err != nil || userID == 0 {
		return "", ErrCredentialRejected
	}
	return strconv.FormatUint(userID, 10), nil
}
