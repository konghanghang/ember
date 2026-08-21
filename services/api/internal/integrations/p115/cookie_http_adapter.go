package p115

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/konghang/ember/backend/internal/common/upstream"
)

const (
	cookieUploadInfoURL    = "https://proapi.115.com/app/uploadinfo"
	cookieSHASearchURL     = "https://webapi.115.com/files/shasearch"
	maxCookieResponseBody  = 256 * 1024
	maxUploadUserKeyLength = 4 * 1024
)

// CookieHTTPAdapter implements the read-only Cookie/Web API operations that
// have fixed request and response contracts. Remaining Provider methods are
// added only after their own contract tests exist.
type CookieHTTPAdapter struct {
	client             httpDoer
	uploadInfoEndpoint *url.URL
	shaSearchEndpoint  *url.URL
}

// NewCookieHTTPAdapter builds the production adapter without performing a network call.
func NewCookieHTTPAdapter() *CookieHTTPAdapter {
	adapter, err := newCookieHTTPAdapter(newCookieHTTPClient(), cookieUploadInfoURL, cookieSHASearchURL)
	if err != nil {
		panic("invalid fixed 115 Cookie HTTP endpoint: " + err.Error())
	}
	return adapter
}

// newCookieHTTPAdapter injects test-owned endpoints while enforcing the same absolute-URL boundary as production.
func newCookieHTTPAdapter(client httpDoer, uploadInfoEndpoint, shaSearchEndpoint string) (*CookieHTTPAdapter, error) {
	if client == nil {
		return nil, fmt.Errorf("%w: http client is nil", ErrProviderUnavailable)
	}
	uploadInfoURL, err := parseCookieHTTPEndpoint(uploadInfoEndpoint)
	if err != nil {
		return nil, err
	}
	shaSearchURL, err := parseCookieHTTPEndpoint(shaSearchEndpoint)
	if err != nil {
		return nil, err
	}
	return &CookieHTTPAdapter{
		client:             client,
		uploadInfoEndpoint: uploadInfoURL,
		shaSearchEndpoint:  shaSearchURL,
	}, nil
}

// GetUploadInfo returns the account-bound upload user key only when the
// response user ID matches the exact UID in the supplied Cookie.
func (a *CookieHTTPAdapter) GetUploadInfo(ctx context.Context, credential Credential) (UploadInfo, error) {
	providerUserID, err := validateCookieHTTPCredential(credential)
	if err != nil {
		return UploadInfo{}, err
	}

	var response struct {
		State   *bool      `json:"state"`
		UserID  jsonUint64 `json:"user_id"`
		UserKey *string    `json:"userkey"`
	}
	if err := a.getJSON(ctx, a.uploadInfoEndpoint, nil, credential, &response); err != nil {
		return UploadInfo{}, err
	}
	if response.State == nil {
		return UploadInfo{}, protocolError("upload info state missing")
	}
	if !*response.State {
		return UploadInfo{}, ErrProviderRejected
	}
	if !response.UserID.set || response.UserID.value == 0 || response.UserKey == nil {
		return UploadInfo{}, protocolError("upload info fields missing")
	}
	userKey := *response.UserKey
	if userKey == "" || userKey != strings.TrimSpace(userKey) || len(userKey) > maxUploadUserKeyLength || strings.ContainsAny(userKey, "\r\n") {
		return UploadInfo{}, protocolError("upload user key invalid")
	}
	responseUserID := strconv.FormatUint(response.UserID.value, 10)
	if responseUserID != providerUserID {
		return UploadInfo{}, ErrCredentialRejected
	}
	return UploadInfo{UserID: responseUserID, UserKey: userKey}, nil
}

// SearchBySHA1 queries the legacy single-result endpoint and returns only a
// candidate that also matches size, non-directory type, and optional parent.
func (a *CookieHTTPAdapter) SearchBySHA1(ctx context.Context, credential Credential, query FileQuery) ([]File, error) {
	if _, err := validateCookieHTTPCredential(credential); err != nil {
		return nil, err
	}
	expectedSHA1, err := normalizeSHA1(query.SHA1)
	if err != nil || query.Size < 0 {
		return nil, ErrInvalidRequest
	}
	expectedParentID, err := normalizeOptionalProviderID(query.ParentID)
	if err != nil {
		return nil, ErrInvalidRequest
	}

	var response struct {
		State *bool           `json:"state"`
		Error string          `json:"error"`
		Data  json.RawMessage `json:"data"`
	}
	params := url.Values{"sha1": {expectedSHA1}}
	if err := a.getJSON(ctx, a.shaSearchEndpoint, params, credential, &response); err != nil {
		return nil, err
	}
	if response.State == nil {
		return nil, protocolError("SHA1 search state missing")
	}
	if !*response.State {
		if strings.TrimSpace(response.Error) == "文件错误" {
			return []File{}, nil
		}
		return nil, ErrProviderRejected
	}

	file, err := decodeSHASearchFile(response.Data)
	if err != nil {
		return nil, err
	}
	if file.IsDirectory || file.SHA1 != expectedSHA1 || file.Size != query.Size ||
		(expectedParentID != "" && file.ParentID != expectedParentID) {
		return []File{}, nil
	}
	return []File{file}, nil
}

// getJSON sends a credential-bound GET and maps transport, HTTP, size, and JSON failures without response leakage.
func (a *CookieHTTPAdapter) getJSON(
	ctx context.Context,
	endpoint *url.URL,
	params url.Values,
	credential Credential,
	target any,
) error {
	requestURL := *endpoint
	if params != nil {
		requestURL.RawQuery = params.Encode()
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL.String(), nil)
	if err != nil {
		return protocolError("request build failed")
	}
	request.Header.Set("Accept", "*/*")
	request.Header.Set("Cookie", credential.Cookie)
	request.Header.Set("User-Agent", credential.UserAgent)

	response, err := a.client.Do(request)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrProviderUnavailable, upstream.SafeUpstreamError(err, "p115"))
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("%w: %v", ErrProviderUnavailable, upstream.SafeUpstreamHTTPError("p115", response.StatusCode))
	}

	body, err := io.ReadAll(io.LimitReader(response.Body, maxCookieResponseBody+1))
	if err != nil {
		return protocolError("response read failed")
	}
	if len(body) > maxCookieResponseBody {
		return protocolError("response too large")
	}
	if err := json.Unmarshal(body, target); err != nil {
		return protocolError("response JSON invalid")
	}
	return nil
}

// decodeSHASearchFile converts the legacy single-result object into Provider-neutral file identity.
func decodeSHASearchFile(data json.RawMessage) (File, error) {
	if len(data) == 0 || string(data) == "null" {
		return File{}, protocolError("SHA1 search data missing")
	}
	var payload struct {
		FileID     jsonUint64 `json:"file_id"`
		ParentID   jsonUint64 `json:"parent_id"`
		CategoryID jsonUint64 `json:"category_id"`
		FileName   *string    `json:"file_name"`
		PickCode   *string    `json:"pick_code"`
		SHA1       *string    `json:"sha1"`
		FileSHA1   *string    `json:"file_sha1"`
		FileSize   jsonUint64 `json:"file_size"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		return File{}, protocolError("SHA1 search data invalid")
	}
	parentID, err := chooseProviderID(payload.ParentID, payload.CategoryID)
	if err != nil {
		return File{}, err
	}
	sha1Value, err := chooseProviderSHA1(payload.SHA1, payload.FileSHA1)
	if err != nil {
		return File{}, err
	}
	if !payload.FileID.set || payload.FileID.value == 0 || !parentID.set ||
		payload.FileName == nil || payload.PickCode == nil || sha1Value == nil || !payload.FileSize.set ||
		payload.FileSize.value > math.MaxInt64 {
		return File{}, protocolError("SHA1 search fields missing")
	}
	name := *payload.FileName
	pickCode := strings.TrimSpace(*payload.PickCode)
	if strings.TrimSpace(name) == "" || pickCode == "" || strings.ContainsAny(name+pickCode, "\r\n") {
		return File{}, protocolError("SHA1 search fields invalid")
	}

	rawSHA1 := strings.TrimSpace(*sha1Value)
	isDirectory := rawSHA1 == ""
	normalizedSHA1 := ""
	if !isDirectory {
		var err error
		normalizedSHA1, err = normalizeSHA1(rawSHA1)
		if err != nil {
			return File{}, protocolError("SHA1 search hash invalid")
		}
	}
	return File{
		ID:          strconv.FormatUint(payload.FileID.value, 10),
		PickCode:    pickCode,
		ParentID:    strconv.FormatUint(parentID.value, 10),
		Name:        name,
		SHA1:        normalizedSHA1,
		Size:        int64(payload.FileSize.value),
		IsDirectory: isDirectory,
	}, nil
}

// chooseProviderID accepts the two parent-ID field names observed in the pinned response normalizer.
func chooseProviderID(parentID, categoryID jsonUint64) (jsonUint64, error) {
	if parentID.set && categoryID.set && parentID.value != categoryID.value {
		return jsonUint64{}, protocolError("SHA1 search parent fields conflict")
	}
	if parentID.set {
		return parentID, nil
	}
	return categoryID, nil
}

// chooseProviderSHA1 accepts the two SHA1 field names observed in the pinned response normalizer.
func chooseProviderSHA1(sha1Value, fileSHA1 *string) (*string, error) {
	if sha1Value != nil && fileSHA1 != nil && !strings.EqualFold(strings.TrimSpace(*sha1Value), strings.TrimSpace(*fileSHA1)) {
		return nil, protocolError("SHA1 search hash fields conflict")
	}
	if sha1Value != nil {
		return sha1Value, nil
	}
	return fileSHA1, nil
}

// validateCookieHTTPCredential rejects malformed Cookies and header injection before any network call.
func validateCookieHTTPCredential(credential Credential) (string, error) {
	providerUserID, err := parseCookieProviderUserID(credential.Cookie)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(credential.UserAgent) == "" || strings.ContainsAny(credential.Cookie+credential.UserAgent, "\r\n") {
		return "", ErrCredentialRejected
	}
	return providerUserID, nil
}

// normalizeSHA1 validates the content identifier and returns the canonical uppercase representation.
func normalizeSHA1(value string) (string, error) {
	value = strings.ToUpper(strings.TrimSpace(value))
	if len(value) != 40 {
		return "", ErrInvalidRequest
	}
	if _, err := hex.DecodeString(value); err != nil {
		return "", ErrInvalidRequest
	}
	return value, nil
}

// normalizeOptionalProviderID canonicalizes a decimal 115 identifier used only for local response filtering.
func normalizeOptionalProviderID(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", nil
	}
	numericID, err := strconv.ParseUint(value, 10, 64)
	if err != nil {
		return "", ErrInvalidRequest
	}
	return strconv.FormatUint(numericID, 10), nil
}

// parseCookieHTTPEndpoint accepts only absolute HTTP(S) endpoints without credentials, fragments, or query state.
func parseCookieHTTPEndpoint(value string) (*url.URL, error) {
	parsed, err := url.ParseRequestURI(strings.TrimSpace(value))
	if err != nil || parsed.Scheme == "" || parsed.Host == "" || parsed.User != nil || parsed.Fragment != "" || parsed.RawQuery != "" {
		return nil, protocolError("endpoint invalid")
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return nil, protocolError("endpoint scheme invalid")
	}
	return parsed, nil
}

// newCookieHTTPClient applies the shared timeout and fail-closed redirect policy for Cookie endpoints.
func newCookieHTTPClient() *http.Client {
	return &http.Client{
		Timeout: 10 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
}

func protocolError(reason string) error {
	return fmt.Errorf("%w: %s", ErrProviderProtocol, reason)
}

type jsonUint64 struct {
	value uint64
	set   bool
}

// UnmarshalJSON accepts the number-or-decimal-string shape used by legacy 115 identifiers and sizes.
func (value *jsonUint64) UnmarshalJSON(data []byte) error {
	raw := strings.TrimSpace(string(data))
	if len(raw) >= 2 && raw[0] == '"' && raw[len(raw)-1] == '"' {
		var decoded string
		if err := json.Unmarshal(data, &decoded); err != nil {
			return err
		}
		raw = strings.TrimSpace(decoded)
	}
	if raw == "" || raw == "null" {
		return fmt.Errorf("integer missing")
	}
	parsed, err := strconv.ParseUint(raw, 10, 64)
	if err != nil {
		return err
	}
	value.value = parsed
	value.set = true
	return nil
}
