package p115

import (
	"bytes"
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
	"unicode/utf8"

	"github.com/konghang/ember/backend/internal/common/upstream"
	"github.com/konghang/ember/backend/internal/integrations/p115/p115cipher"
)

const (
	cookieUploadInfoURL          = "https://proapi.115.com/app/uploadinfo"
	cookieSHASearchURL           = "https://webapi.115.com/files/shasearch"
	cookieUploadInitURL          = "https://uplb.115.com/4.0/initupload.php"
	cookieUploadAppVersion       = "36.2.28"
	maxCookieResponseBody        = 256 * 1024
	maxUploadUserKeyLength       = 4 * 1024
	targetSearchLimit            = 100
	targetVisibilityPollInterval = 500 * time.Millisecond
	targetVisibilityTimeout      = 10 * time.Second
)

// CookieHTTPAdapter implements Cookie/Web API operations that have fixed
// request and response contracts. Remaining Provider methods are added only
// after their own contract tests exist.
type CookieHTTPAdapter struct {
	client                  httpDoer
	uploadInfoEndpoint      *url.URL
	shaSearchEndpoint       *url.URL
	targetSearchEndpoint    *url.URL
	uploadInitEndpoint      *url.URL
	now                     func() time.Time
	wait                    func(context.Context, time.Duration) error
	targetPollInterval      time.Duration
	targetVisibilityTimeout time.Duration
}

// NewCookieHTTPAdapter builds the production adapter without performing a network call.
func NewCookieHTTPAdapter() *CookieHTTPAdapter {
	adapter, err := newCookieHTTPAdapter(newCookieHTTPClient(), cookieUploadInfoURL, cookieSHASearchURL, cookieUploadInitURL)
	if err != nil {
		panic("invalid fixed 115 Cookie HTTP endpoint: " + err.Error())
	}
	return adapter
}

// newCookieHTTPAdapter injects test-owned endpoints while enforcing the same absolute-URL boundary as production.
func newCookieHTTPAdapter(client httpDoer, uploadInfoEndpoint, shaSearchEndpoint, uploadInitEndpoint string) (*CookieHTTPAdapter, error) {
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
	uploadInitURL, err := parseCookieHTTPEndpoint(uploadInitEndpoint)
	if err != nil {
		return nil, err
	}
	targetSearchURL := *shaSearchURL
	targetSearchURL.Path = "/files/search"
	targetSearchURL.RawPath = ""
	return &CookieHTTPAdapter{
		client:                  client,
		uploadInfoEndpoint:      uploadInfoURL,
		shaSearchEndpoint:       shaSearchURL,
		targetSearchEndpoint:    &targetSearchURL,
		uploadInitEndpoint:      uploadInitURL,
		now:                     time.Now,
		wait:                    waitForCookieTargetPoll,
		targetPollInterval:      targetVisibilityPollInterval,
		targetVisibilityTimeout: targetVisibilityTimeout,
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

// FindTargetFile polls the playback account's target directory until exactly
// one file matches SHA1, size, non-directory type, and parent ID.
func (a *CookieHTTPAdapter) FindTargetFile(ctx context.Context, credential Credential, query FileQuery) (*File, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	expectedSHA1, err := normalizeSHA1(query.SHA1)
	if err != nil || query.Size < 0 {
		return nil, ErrInvalidRequest
	}
	expectedParentID, err := normalizeOptionalProviderID(query.ParentID)
	if err != nil || expectedParentID == "" {
		return nil, ErrInvalidRequest
	}
	if _, err := validateCookieHTTPCredential(credential); err != nil {
		return nil, err
	}

	deadline := a.now().Add(a.targetVisibilityTimeout)
	for {
		file, err := a.findTargetFileOnce(ctx, credential, FileQuery{
			SHA1:     expectedSHA1,
			Size:     query.Size,
			ParentID: expectedParentID,
		})
		if err != nil || file != nil {
			return file, err
		}
		remaining := deadline.Sub(a.now())
		if remaining <= 0 {
			return nil, ErrTargetFileNotVisible
		}
		waitDuration := a.targetPollInterval
		if remaining < waitDuration {
			waitDuration = remaining
		}
		if err := a.wait(ctx, waitDuration); err != nil {
			return nil, err
		}
	}
}

// findTargetFileOnce performs one first-page target search and fails closed on multiple exact matches.
func (a *CookieHTTPAdapter) findTargetFileOnce(ctx context.Context, credential Credential, query FileQuery) (*File, error) {
	params := url.Values{
		"aid":          {"1"},
		"cid":          {query.ParentID},
		"fc":           {"2"},
		"limit":        {strconv.Itoa(targetSearchLimit)},
		"offset":       {"0"},
		"search_value": {query.SHA1},
		"show_dir":     {"0"},
		"type":         {"99"},
	}
	var response struct {
		State *bool           `json:"state"`
		Data  json.RawMessage `json:"data"`
	}
	if err := a.getJSON(ctx, a.targetSearchEndpoint, params, credential, &response); err != nil {
		return nil, err
	}
	if response.State == nil {
		return nil, protocolError("target search state missing")
	}
	if !*response.State {
		return nil, ErrProviderRejected
	}
	if len(response.Data) == 0 || string(response.Data) == "null" {
		return nil, protocolError("target search data missing")
	}
	var items []json.RawMessage
	if err := json.Unmarshal(response.Data, &items); err != nil {
		return nil, protocolError("target search data invalid")
	}

	var match *File
	for _, item := range items {
		file, err := decodeTargetSearchFile(item)
		if err != nil {
			return nil, err
		}
		if file.IsDirectory || file.SHA1 != query.SHA1 || file.Size != query.Size || file.ParentID != query.ParentID {
			continue
		}
		if match != nil {
			return nil, ErrTargetFileAmbiguous
		}
		matched := file
		match = &matched
	}
	return match, nil
}

// InitRapidUpload builds the encrypted Cookie upload request and maps the
// provider's direct status response into Ember-owned state semantics.
func (a *CookieHTTPAdapter) InitRapidUpload(ctx context.Context, credential Credential, request RapidUploadRequest) (RapidUploadResult, error) {
	normalized, err := normalizeRapidUploadRequest(request)
	if err != nil {
		return RapidUploadResult{}, err
	}
	uploadInfo, err := a.GetUploadInfo(ctx, credential)
	if err != nil {
		return RapidUploadResult{}, err
	}
	timestamp := a.now().UTC().Unix()
	encrypted, err := p115cipher.BuildUploadRequest(p115cipher.UploadPayload{
		UserKey:    uploadInfo.UserKey,
		UserID:     uploadInfo.UserID,
		FileID:     normalized.SHA1,
		FileName:   normalized.FileName,
		Target:     "U_1_" + normalized.TargetParentID,
		FileSize:   normalized.Size,
		PreID:      normalized.PreID,
		SignKey:    normalized.SignKey,
		SignValue:  normalized.SignValue,
		TopUpload:  "true",
		AppVersion: cookieUploadAppVersion,
	}, timestamp)
	if err != nil {
		return RapidUploadResult{}, protocolError("upload payload build failed")
	}

	requestURL := *a.uploadInitEndpoint
	requestURL.RawQuery = url.Values{"k_ec": {encrypted.KEc}}.Encode()
	httpRequest, err := http.NewRequestWithContext(ctx, http.MethodPost, requestURL.String(), bytes.NewReader(encrypted.Data))
	if err != nil {
		return RapidUploadResult{}, protocolError("upload request build failed")
	}
	httpRequest.Header.Set("Accept", "*/*")
	httpRequest.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	httpRequest.Header.Set("Cookie", credential.Cookie)
	httpRequest.Header.Set("User-Agent", cookieUploadUserAgent(cookieUploadAppVersion))

	response, err := a.client.Do(httpRequest)
	if err != nil {
		return RapidUploadResult{}, fmt.Errorf("%w: %v", ErrProviderUnavailable, upstream.SafeUpstreamError(err, "p115"))
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return RapidUploadResult{}, fmt.Errorf("%w: %v", ErrProviderUnavailable, upstream.SafeUpstreamHTTPError("p115", response.StatusCode))
	}
	ciphertext, err := io.ReadAll(io.LimitReader(response.Body, maxCookieResponseBody+1))
	if err != nil {
		return RapidUploadResult{}, protocolError("upload response read failed")
	}
	if len(ciphertext) > maxCookieResponseBody {
		return RapidUploadResult{}, protocolError("upload response too large")
	}
	plaintext, err := p115cipher.DecryptResponse(ciphertext)
	if err != nil {
		return RapidUploadResult{}, protocolError("upload response decrypt failed")
	}
	return mapRapidUploadResponse(plaintext, normalized.Size)
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

// decodeTargetSearchFile maps the pinned web search short fields into a complete file identity.
func decodeTargetSearchFile(data json.RawMessage) (File, error) {
	var payload struct {
		FileID   jsonUint64 `json:"fid"`
		ParentID jsonUint64 `json:"cid"`
		Name     *string    `json:"n"`
		PickCode *string    `json:"pc"`
		SHA1     *string    `json:"sha"`
		Size     jsonUint64 `json:"s"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		return File{}, protocolError("target search item invalid")
	}
	if !payload.FileID.set || payload.FileID.value == 0 || !payload.ParentID.set ||
		payload.Name == nil || payload.PickCode == nil || payload.SHA1 == nil || !payload.Size.set ||
		payload.Size.value > math.MaxInt64 {
		return File{}, protocolError("target search fields missing")
	}
	name := *payload.Name
	pickCode := strings.TrimSpace(*payload.PickCode)
	if strings.TrimSpace(name) == "" || pickCode == "" || strings.ContainsAny(name+pickCode, "\r\n") {
		return File{}, protocolError("target search fields invalid")
	}
	rawSHA1 := strings.TrimSpace(*payload.SHA1)
	isDirectory := rawSHA1 == ""
	normalizedSHA1 := ""
	if !isDirectory {
		var err error
		normalizedSHA1, err = normalizeSHA1(rawSHA1)
		if err != nil {
			return File{}, protocolError("target search hash invalid")
		}
	}
	return File{
		ID:          strconv.FormatUint(payload.FileID.value, 10),
		PickCode:    pickCode,
		ParentID:    strconv.FormatUint(payload.ParentID.value, 10),
		Name:        name,
		SHA1:        normalizedSHA1,
		Size:        int64(payload.Size.value),
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

type normalizedRapidUploadRequest struct {
	FileName       string
	SHA1           string
	Size           int64
	TargetParentID string
	PreID          string
	SignKey        string
	SignValue      string
}

// normalizeRapidUploadRequest validates content identity and challenge fields before upload-info lookup.
func normalizeRapidUploadRequest(request RapidUploadRequest) (normalizedRapidUploadRequest, error) {
	fileName := request.FileName
	if strings.TrimSpace(fileName) == "" || !utf8.ValidString(fileName) || len(fileName) > 1024 || strings.ContainsAny(fileName, "\r\n") {
		return normalizedRapidUploadRequest{}, ErrInvalidRequest
	}
	sha1Value, err := normalizeSHA1(request.SHA1)
	if err != nil || request.Size <= 0 {
		return normalizedRapidUploadRequest{}, ErrInvalidRequest
	}
	targetParentID, err := normalizeOptionalProviderID(request.TargetParentID)
	if err != nil || targetParentID == "" {
		return normalizedRapidUploadRequest{}, ErrInvalidRequest
	}
	preID := ""
	if strings.TrimSpace(request.PreID) != "" {
		preID, err = normalizeSHA1(request.PreID)
		if err != nil {
			return normalizedRapidUploadRequest{}, ErrInvalidRequest
		}
	}
	signKey := request.SignKey
	signValue := strings.TrimSpace(request.SignValue)
	if (signKey == "") != (signValue == "") || len(signKey) > 1024 ||
		signKey != strings.TrimSpace(signKey) || strings.ContainsAny(signKey, "\r\n") || !isASCIIProtocolValue(signKey) {
		return normalizedRapidUploadRequest{}, ErrInvalidRequest
	}
	if signValue != "" {
		signValue, err = normalizeSHA1(signValue)
		if err != nil {
			return normalizedRapidUploadRequest{}, ErrInvalidRequest
		}
	}
	return normalizedRapidUploadRequest{
		FileName:       fileName,
		SHA1:           sha1Value,
		Size:           request.Size,
		TargetParentID: targetParentID,
		PreID:          preID,
		SignKey:        signKey,
		SignValue:      signValue,
	}, nil
}

// mapRapidUploadResponse parses decrypted JSON without retaining provider messages or response bodies.
func mapRapidUploadResponse(plaintext []byte, fileSize int64) (RapidUploadResult, error) {
	var response struct {
		State      *bool            `json:"state"`
		Status     jsonUint64       `json:"status"`
		StatusCode jsonProviderCode `json:"statuscode"`
		Errno      jsonProviderCode `json:"errno"`
		SignKey    *string          `json:"sign_key"`
		SignCheck  *string          `json:"sign_check"`
	}
	if err := json.Unmarshal(plaintext, &response); err != nil {
		return RapidUploadResult{}, protocolError("upload response JSON invalid")
	}
	providerCode := response.StatusCode.value
	if providerCode == "" {
		providerCode = response.Errno.value
	}
	if response.State != nil && !*response.State {
		return RapidUploadResult{Status: RapidUploadProviderRejected, ProviderCode: providerCode}, nil
	}
	if !response.Status.set {
		return RapidUploadResult{}, protocolError("upload status missing")
	}

	result := RapidUploadResult{ProviderCode: providerCode}
	switch response.Status.value {
	case 1:
		result.Status = RapidUploadOrdinaryUploadRequired
	case 2:
		result.Status = RapidUploadReused
	case 7:
		if response.SignKey == nil || response.SignCheck == nil {
			return RapidUploadResult{}, protocolError("upload challenge fields missing")
		}
		signKey := *response.SignKey
		if signKey == "" || signKey != strings.TrimSpace(signKey) || len(signKey) > 1024 ||
			strings.ContainsAny(signKey, "\r\n") || !isASCIIProtocolValue(signKey) {
			return RapidUploadResult{}, protocolError("upload challenge key invalid")
		}
		byteRange, err := parseRapidUploadRange(*response.SignCheck, fileSize)
		if err != nil {
			return RapidUploadResult{}, err
		}
		result.Status = RapidUploadRangeChallenge
		result.Challenge = &RapidUploadChallenge{Range: byteRange, SignKey: signKey}
	default:
		result.Status = RapidUploadProviderRejected
	}
	return result, nil
}

// parseRapidUploadRange validates the inclusive sign_check range against the source file size.
func parseRapidUploadRange(value string, fileSize int64) (ByteRange, error) {
	value = strings.TrimSpace(value)
	startValue, endValue, found := strings.Cut(value, "-")
	if !found || strings.Contains(endValue, "-") || startValue == "" || endValue == "" {
		return ByteRange{}, protocolError("upload challenge range invalid")
	}
	start, err := strconv.ParseInt(startValue, 10, 64)
	if err != nil {
		return ByteRange{}, protocolError("upload challenge range invalid")
	}
	end, err := strconv.ParseInt(endValue, 10, 64)
	if err != nil || start < 0 || end < start || end >= fileSize {
		return ByteRange{}, protocolError("upload challenge range invalid")
	}
	return ByteRange{Start: start, End: end}, nil
}

// cookieUploadUserAgent reproduces the pinned upload endpoint's version-bound client identity.
func cookieUploadUserAgent(appVersion string) string {
	return fmt.Sprintf("Mozilla/5.0 115disk/%s 115Browser/%s 115wangpan_android/%s", appVersion, appVersion, appVersion)
}

func isASCIIProtocolValue(value string) bool {
	for index := 0; index < len(value); index++ {
		if value[index] > 0x7f {
			return false
		}
	}
	return true
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

// waitForCookieTargetPoll waits without leaking a timer and returns context cancellation unchanged.
func waitForCookieTargetPoll(ctx context.Context, duration time.Duration) error {
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
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

type jsonProviderCode struct {
	value string
}

// UnmarshalJSON accepts a short number-or-string provider code and rejects message-shaped values.
func (code *jsonProviderCode) UnmarshalJSON(data []byte) error {
	raw := strings.TrimSpace(string(data))
	if raw == "" || raw == "null" {
		return nil
	}
	if raw[0] == '"' {
		if err := json.Unmarshal(data, &raw); err != nil {
			return err
		}
		raw = strings.TrimSpace(raw)
	} else {
		if _, err := strconv.ParseInt(raw, 10, 64); err != nil {
			return err
		}
	}
	if raw == "" || len(raw) > 64 || !isSafeProviderCode(raw) {
		return fmt.Errorf("provider code invalid")
	}
	code.value = raw
	return nil
}

func isSafeProviderCode(value string) bool {
	for index := 0; index < len(value); index++ {
		character := value[index]
		if (character >= 'a' && character <= 'z') || (character >= 'A' && character <= 'Z') ||
			(character >= '0' && character <= '9') || character == '_' || character == '-' || character == '.' {
			continue
		}
		return false
	}
	return true
}
