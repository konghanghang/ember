package p115

import (
	"bytes"
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"mime"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/konghang/ember/backend/internal/common/upstream"
	"github.com/konghang/ember/backend/internal/integrations/p115/p115cipher"
	"github.com/konghang/ember/backend/internal/logging"
)

const (
	cookieUploadInfoURL          = "https://proapi.115.com/app/uploadinfo"
	cookieSHASearchURL           = "https://webapi.115.com/files/shasearch"
	cookieUploadInitURL          = "https://uplb.115.com/4.0/initupload.php"
	cookieUploadAppVersion       = "36.2.28"
	maxCookieResponseBody        = 256 * 1024
	maxUploadUserKeyLength       = 4 * 1024
	maxDownloadURLLength         = 16 * 1024
	maxDownloadUserAgentLength   = 1024
	targetSearchLimit            = 100
	targetVisibilityPollInterval = 500 * time.Millisecond
	targetVisibilityTimeout      = 10 * time.Second
	sourceResolvePageSize        = 200
	sourceResolveMaxEntries      = 10_000
	maxSourcePathLength          = 4 * 1024
	maxSourcePathSegmentLength   = 1024
	maxSourceRangeBytes          = 1024 * 1024
)

// CookieHTTPAdapter implements the Cookie/Web data-plane operations whose
// request and response contracts are fixed by fake HTTP tests.
type CookieHTTPAdapter struct {
	client                  httpDoer
	uploadInfoEndpoint      *url.URL
	shaSearchEndpoint       *url.URL
	sourcePathIDEndpoint    *url.URL
	sourceListEndpoint      *url.URL
	targetSearchEndpoint    *url.URL
	uploadInitEndpoint      *url.URL
	downloadEndpoint        *url.URL
	deleteEndpoint          *url.URL
	deleteLocks             *keyedSerialLocks
	downloadEncrypt         func([]byte) (string, error)
	downloadDecrypt         func(string) ([]byte, error)
	downloadHostAllowed     func(string) bool
	now                     func() time.Time
	wait                    func(context.Context, time.Duration) error
	targetPollInterval      time.Duration
	targetVisibilityTimeout time.Duration
	sourceResolvePageSize   int
	sourceResolveMaxEntries int
	sourceFlights           sourceDirectoryFlights
	sourceReadTimeout       func(context.Context, time.Duration) (context.Context, context.CancelFunc)
	limiter                 *accountRateLimiter
}

// NewCookieHTTPAdapter builds the production adapter without performing a network call.
func NewCookieHTTPAdapter() *CookieHTTPAdapter {
	adapter, err := newCookieHTTPAdapter(newCookieHTTPClient(), cookieUploadInfoURL, cookieSHASearchURL, cookieUploadInitURL)
	if err != nil {
		panic("invalid fixed 115 Cookie HTTP endpoint: " + err.Error())
	}
	attachDataPlaneLimiter(adapter)
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
	sourceListURL := *shaSearchURL
	sourceListURL.Path = "/files"
	sourceListURL.RawPath = ""
	sourcePathIDURL := *shaSearchURL
	sourcePathIDURL.Path = "/files/get_path_id"
	sourcePathIDURL.RawPath = ""
	downloadURL := *uploadInfoURL
	downloadURL.Path = "/app/chrome/downurl"
	downloadURL.RawPath = ""
	deleteURL := *shaSearchURL
	deleteURL.Path = "/rb/delete"
	deleteURL.RawPath = ""
	return &CookieHTTPAdapter{
		client:                  client,
		uploadInfoEndpoint:      uploadInfoURL,
		shaSearchEndpoint:       shaSearchURL,
		sourcePathIDEndpoint:    &sourcePathIDURL,
		sourceListEndpoint:      &sourceListURL,
		targetSearchEndpoint:    &targetSearchURL,
		uploadInitEndpoint:      uploadInitURL,
		downloadEndpoint:        &downloadURL,
		deleteEndpoint:          &deleteURL,
		deleteLocks:             sharedCookieDeleteLocks,
		downloadEncrypt:         p115cipher.RSAEncrypt,
		downloadDecrypt:         p115cipher.RSADecrypt,
		downloadHostAllowed:     isAllowed115DownloadHost,
		now:                     time.Now,
		wait:                    waitForCookieTargetPoll,
		targetPollInterval:      targetVisibilityPollInterval,
		targetVisibilityTimeout: targetVisibilityTimeout,
		sourceResolvePageSize:   sourceResolvePageSize,
		sourceResolveMaxEntries: sourceResolveMaxEntries,
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
		providerErrorCodes
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
		return UploadInfo{}, response.rejection()
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

// SearchBySHA1 uses a directory-scoped Web search when ParentID is present;
// otherwise it queries the legacy global single-result endpoint. Both paths
// return only a candidate that matches size and non-directory type.
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
	if expectedParentID != "" {
		file, err := a.findTargetFileOnce(ctx, credential, FileQuery{
			SHA1:     expectedSHA1,
			Size:     query.Size,
			ParentID: expectedParentID,
		})
		if err != nil {
			return nil, err
		}
		if file == nil {
			return []File{}, nil
		}
		return []File{*file}, nil
	}

	var response struct {
		providerErrorCodes
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
		return nil, response.rejection()
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

// ResolveFileByPath resolves the parent directory below one explicit root in a
// single Provider call, sharing in-flight directory reads with sibling files.
// Each caller independently requires a unique file in the completed snapshot.
func (a *CookieHTTPAdapter) ResolveFileByPath(
	ctx context.Context,
	credential Credential,
	query FilePathQuery,
) (*File, error) {
	if _, err := validateCookieHTTPCredential(credential); err != nil {
		return nil, err
	}
	normalized, err := normalizeFilePathQuery(query)
	if err != nil {
		return nil, err
	}

	parentPath := strings.Join(normalized.Segments[:len(normalized.Segments)-1], "/")
	entries, err := a.sourceFlights.do(ctx, sourceDirectoryKey(credential, normalized.RootID, parentPath), func(sharedCtx context.Context) ([]sourceListEntry, error) {
		parentID := normalized.RootID
		if parentPath != "" {
			var resolveErr error
			parentID, resolveErr = a.resolveSourceParentDirectory(sharedCtx, credential, normalized.RootID, parentPath)
			if resolveErr != nil {
				return nil, resolveErr
			}
		}
		return a.listSourceDirectory(sharedCtx, credential, parentID)
	})
	if err != nil {
		return nil, err
	}
	fileName := normalized.Segments[len(normalized.Segments)-1]
	matches := make([]sourceListEntry, 0, 1)
	for _, entry := range entries {
		if entry.Name == fileName && !entry.IsDirectory {
			matches = append(matches, entry)
		}
	}
	if len(matches) == 0 {
		return nil, ErrSourceFileNotFound
	}
	if len(matches) != 1 {
		return nil, ErrSourceFileAmbiguous
	}
	if matches[0].invalid != nil {
		return nil, matches[0].invalid
	}
	file := matches[0].File
	return &file, nil
}

// resolveSourceParentDirectory maps one validated relative directory path to a
// Provider directory ID without allowing playback to create missing folders.
func (a *CookieHTTPAdapter) resolveSourceParentDirectory(
	ctx context.Context,
	credential Credential,
	rootID, relativePath string,
) (string, error) {
	started := time.Now()
	defer func() {
		if timing, ok := ctx.Value(sourceReadTimingKey{}).(*sourceReadTiming); ok {
			timing.parent += time.Since(started)
		}
	}()
	params := url.Values{
		"is_create": {"0"},
		"parent_id": {rootID},
		"path":      {relativePath},
	}
	var response struct {
		providerErrorCodes
		State *bool      `json:"state"`
		ID    jsonUint64 `json:"id"`
		Data  *struct {
			FileID jsonUint64 `json:"file_id"`
		} `json:"data"`
	}
	if err := a.getJSON(ctx, a.sourcePathIDEndpoint, params, credential, &response); err != nil {
		return "", err
	}
	if response.State == nil {
		return "", protocolError("source path response state missing")
	}
	if !*response.State {
		return "", response.rejection()
	}
	nestedID := jsonUint64{}
	if response.Data != nil {
		nestedID = response.Data.FileID
	}
	if response.ID.set && nestedID.set && response.ID.value != nestedID.value {
		return "", protocolError("source path response id conflict")
	}
	directoryID := response.ID
	if !directoryID.set {
		directoryID = nestedID
	}
	if !directoryID.set {
		return "", protocolError("source path response id missing")
	}
	if directoryID.value == 0 {
		return "", ErrSourceFileNotFound
	}
	return strconv.FormatUint(directoryID.value, 10), nil
}

// ResolveDirectoryByPath traverses a root-relative path and accepts exactly
// one directory at every segment, returning its stable ID and canonical path.
func (a *CookieHTTPAdapter) ResolveDirectoryByPath(
	ctx context.Context,
	credential Credential,
	query DirectoryPathQuery,
) (*Directory, error) {
	if _, err := validateCookieHTTPCredential(credential); err != nil {
		return nil, err
	}
	normalized, err := normalizeDirectoryPathQuery(query)
	if err != nil {
		return nil, err
	}

	parentID := normalized.RootID
	for index, segment := range normalized.Segments {
		entries, err := a.listSourceDirectory(ctx, credential, parentID)
		if err != nil {
			if errors.Is(err, ErrSourceFileNotFound) {
				return nil, ErrDirectoryNotFound
			}
			return nil, err
		}
		matches := make([]sourceListEntry, 0, 1)
		for _, entry := range entries {
			if entry.IsDirectory && entry.Name == segment {
				matches = append(matches, entry)
			}
		}
		if len(matches) == 0 {
			return nil, ErrDirectoryNotFound
		}
		if len(matches) != 1 {
			return nil, ErrDirectoryAmbiguous
		}
		match := matches[0]
		if index == len(normalized.Segments)-1 {
			return &Directory{
				ID: match.ID, ParentID: match.ParentID, Name: match.Name, Path: normalized.Path,
			}, nil
		}
		parentID = match.ID
	}
	return nil, ErrDirectoryNotFound
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
		providerErrorCodes
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
		return nil, response.rejection()
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
func (a *CookieHTTPAdapter) InitRapidUpload(ctx context.Context, credential Credential, request RapidUploadRequest) (uploadResult RapidUploadResult, resultErr error) {
	normalized, err := normalizeRapidUploadRequest(request)
	if err != nil {
		return RapidUploadResult{}, err
	}
	uploadInfo, err := a.GetUploadInfo(ctx, credential)
	if err != nil {
		return RapidUploadResult{}, err
	}
	// Upload-info owns its own trace; only the actual upload request and
	// response belong to this observation, avoiding nested HTTP timings.
	ctx, observation := observeOperation(ctx, "rapid_upload")
	defer func() { resultErr = observation.finish(resultErr) }()
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
		return RapidUploadResult{}, rapidUploadProtocolError(RapidUploadPhasePayloadBuild, nil, nil)
	}

	requestURL := *a.uploadInitEndpoint
	requestURL.RawQuery = url.Values{"k_ec": {encrypted.KEc}}.Encode()
	httpRequest, err := http.NewRequestWithContext(ctx, http.MethodPost, requestURL.String(), bytes.NewReader(encrypted.Data))
	if err != nil {
		return RapidUploadResult{}, rapidUploadProtocolError(RapidUploadPhaseRequestBuild, nil, nil)
	}
	httpRequest.Header.Set("Accept", "*/*")
	httpRequest.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	httpRequest.Header.Set("Cookie", credential.Cookie)
	httpRequest.Header.Set("User-Agent", cookieUploadUserAgent(cookieUploadAppVersion))

	if err := a.waitDataPlane(ctx, credential); err != nil {
		return RapidUploadResult{}, err
	}
	response, err := observation.do(a.client, httpRequest)
	if err != nil {
		return RapidUploadResult{}, err
	}
	defer closeResponseBody(response.Body)
	if err := mapCookieAPIStatus(response.StatusCode); err != nil {
		return RapidUploadResult{}, err
	}
	ciphertext, err := io.ReadAll(io.LimitReader(response.Body, maxCookieResponseBody+1))
	if err != nil {
		if ctx.Err() != nil {
			return RapidUploadResult{}, ctx.Err()
		}
		return RapidUploadResult{}, rapidUploadProtocolError(RapidUploadPhaseResponseRead, response, nil)
	}
	if len(ciphertext) > maxCookieResponseBody {
		return RapidUploadResult{}, rapidUploadProtocolError(RapidUploadPhaseResponseTooLarge, response, ciphertext)
	}
	plaintext, err := p115cipher.DecryptResponse(ciphertext)
	if err != nil {
		evidence := rapidUploadProtocolError(RapidUploadPhaseResponseDecrypt, response, ciphertext)
		var decryptErr *p115cipher.ResponseDecryptError
		if errors.As(err, &decryptErr) {
			switch decryptErr.Phase {
			case p115cipher.ResponseDecryptPhaseAES:
				evidence.DecryptPhase = RapidUploadDecryptPhaseAES
			case p115cipher.ResponseDecryptPhaseLZ4:
				evidence.DecryptPhase = RapidUploadDecryptPhaseLZ4
			}
		}
		return RapidUploadResult{}, evidence
	}
	result, err := mapRapidUploadResponse(plaintext, normalized.Size)
	if err != nil {
		if errors.Is(err, ErrProviderProtocol) {
			return RapidUploadResult{}, rapidUploadProtocolError(RapidUploadPhaseResponseMap, response, nil)
		}
		return RapidUploadResult{}, err
	}
	return result, nil
}

// GetDownloadURL signs one file URL with the actual playback client User-Agent
// and returns only an HTTPS 115-owned URL with complete t/c/f semantics.
func (a *CookieHTTPAdapter) GetDownloadURL(ctx context.Context, credential Credential, request DownloadURLRequest) (result DownloadURLResult, resultErr error) {
	ctx, observation := observeOperation(ctx, "download_url")
	defer func() {
		if resultErr == nil && ctx.Err() != nil {
			result, resultErr = DownloadURLResult{}, ctx.Err()
		}
		resultErr = observation.finish(resultErr)
	}()
	if err := ctx.Err(); err != nil {
		return DownloadURLResult{}, err
	}
	providerUserID, err := validateCookieHTTPCredential(credential)
	if err != nil {
		return DownloadURLResult{}, err
	}
	pickCode := strings.TrimSpace(request.PickCode)
	userAgent := request.UserAgent
	if !isValidDownloadPickCode(pickCode) || userAgent == "" || userAgent != strings.TrimSpace(userAgent) ||
		len(userAgent) > maxDownloadUserAgentLength || strings.ContainsAny(userAgent, "\r\n") {
		return DownloadURLResult{}, ErrInvalidRequest
	}
	if err := a.waitDataPlane(ctx, credential); err != nil {
		return DownloadURLResult{}, err
	}
	userID, err := strconv.ParseUint(providerUserID, 10, 64)
	if err != nil || userID == 0 {
		return DownloadURLResult{}, ErrCredentialRejected
	}
	payload, err := json.Marshal(struct {
		PickCode string `json:"pickcode"`
		UserID   uint64 `json:"user_id"`
	}{PickCode: pickCode, UserID: userID})
	if err != nil {
		return DownloadURLResult{}, protocolError("download payload build failed")
	}
	encrypted, err := a.downloadEncrypt(payload)
	if err != nil {
		return DownloadURLResult{}, protocolError("download payload encrypt failed")
	}

	form := url.Values{"data": {encrypted}}.Encode()
	httpRequest, err := http.NewRequestWithContext(ctx, http.MethodPost, a.downloadEndpoint.String(), strings.NewReader(form))
	if err != nil {
		return DownloadURLResult{}, protocolError("download request build failed")
	}
	httpRequest.Header.Set("Accept", "*/*")
	httpRequest.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	httpRequest.Header.Set("Cookie", credential.Cookie)
	httpRequest.Header.Set("User-Agent", userAgent)

	response, err := observation.do(a.client, httpRequest)
	if err != nil {
		return DownloadURLResult{}, err
	}
	defer closeResponseBody(response.Body)
	if err := mapCookieAPIStatus(response.StatusCode); err != nil {
		return DownloadURLResult{}, err
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, maxCookieResponseBody+1))
	if err != nil {
		if ctx.Err() != nil {
			return DownloadURLResult{}, ctx.Err()
		}
		return DownloadURLResult{}, &operationFailure{cause: ErrProviderUnavailable, stage: observation.stage, reason: "response_read_failed", status: response.StatusCode}
	}
	if len(body) > maxCookieResponseBody {
		return DownloadURLResult{}, protocolError("download response too large")
	}
	var outer struct {
		State *bool           `json:"state"`
		Data  json.RawMessage `json:"data"`
		Errno json.RawMessage `json:"errno"`
	}
	if err := json.Unmarshal(body, &outer); err != nil {
		return DownloadURLResult{}, protocolError("download response JSON invalid")
	}
	if outer.State == nil {
		return DownloadURLResult{}, protocolError("download response state missing")
	}
	if !*outer.State {
		return DownloadURLResult{}, downloadRejection(outer.Errno)
	}
	var encryptedData *string
	if json.Unmarshal(outer.Data, &encryptedData) != nil || encryptedData == nil || strings.TrimSpace(*encryptedData) == "" {
		return DownloadURLResult{}, protocolError("download response data missing")
	}
	plaintext, err := a.downloadDecrypt(*encryptedData)
	if err != nil {
		return DownloadURLResult{}, protocolError("download response decrypt failed")
	}
	rawURL, err := decodeChromeDownloadURL(plaintext, pickCode)
	if err != nil {
		return DownloadURLResult{}, err
	}
	return parseDownloadURL(rawURL, a.now().UTC(), a.downloadHostAllowed)
}

// HashFileRange gives signed-URL preparation and the bounded CDN read separate
// deadlines; only SHA1 and byte count leave the provider boundary.
func (a *CookieHTTPAdapter) HashFileRange(
	ctx context.Context,
	credential Credential,
	request FileRangeRequest,
) (result FileRangeHash, resultErr error) {
	if err := ctx.Err(); err != nil {
		return FileRangeHash{}, err
	}
	if _, err := validateCookieHTTPCredential(credential); err != nil {
		return FileRangeHash{}, err
	}
	expectedBytes, err := validateFileRangeRequest(request)
	if err != nil {
		return FileRangeHash{}, err
	}
	rangeUserAgent := credential.UserAgent
	if request.UserAgent != "" {
		rangeUserAgent = request.UserAgent
	}
	started := time.Now()
	var downloadDuration, rangeDuration time.Duration
	defer func() {
		logging.Debugf("[P115] code=source_range_completed sourceDownloadMs=%d rangeReadMs=%d durationMs=%d success=%t",
			downloadDuration.Milliseconds(), rangeDuration.Milliseconds(), time.Since(started).Milliseconds(), resultErr == nil)
	}()
	phaseLimit := sourceRangePhaseTimeout
	downloadCtx, cancelDownload := a.sourcePhaseContext(ctx, phaseLimit)
	downloadStarted := time.Now()
	download, err := a.GetDownloadURL(downloadCtx, credential, DownloadURLRequest{
		PickCode:  request.File.PickCode,
		UserAgent: rangeUserAgent,
	})
	downloadDuration = time.Since(downloadStarted)
	err = sourceRangeStageResult(ctx, downloadCtx, err)
	cancelDownload()
	if err != nil {
		return FileRangeHash{}, err
	}
	rangeCtx, cancelRange := a.sourcePhaseContext(ctx, phaseLimit)
	defer cancelRange()
	rangeStarted := time.Now()
	result, resultErr = a.readFileRange(rangeCtx, credential, request, download, rangeUserAgent, expectedBytes)
	rangeDuration = time.Since(rangeStarted)
	resultErr = sourceRangeStageResult(ctx, rangeCtx, resultErr)
	if resultErr != nil {
		return FileRangeHash{}, resultErr
	}
	return result, nil
}

// readFileRange observes only the CDN request and validates exact status,
// encoding, range and length before hashing, including cancellation during body reads.
func (a *CookieHTTPAdapter) readFileRange(ctx context.Context, credential Credential, request FileRangeRequest, download DownloadURLResult, userAgent string, expectedBytes int64) (result FileRangeHash, resultErr error) {
	ctx, observation := observeOperation(ctx, "source_range")
	defer func() { resultErr = observation.finish(resultErr) }()
	httpRequest, err := http.NewRequestWithContext(ctx, http.MethodGet, download.URL, nil)
	if err != nil {
		return FileRangeHash{}, protocolError("range request build failed")
	}
	httpRequest.Header.Set("Accept", "*/*")
	httpRequest.Header.Set("Accept-Encoding", "identity")
	httpRequest.Header.Set("Range", fmt.Sprintf("bytes=%d-%d", request.Range.Start, request.Range.End))
	httpRequest.Header.Set("User-Agent", userAgent)
	switch download.HeaderMode {
	case DownloadHeadersNone, DownloadHeadersSameUserAgent:
	case DownloadHeadersSameUserAgentAndCookie:
		httpRequest.Header.Set("Cookie", credential.Cookie)
	default:
		return FileRangeHash{}, ErrDownloadURLIncompatible
	}

	response, err := observation.do(a.client, httpRequest)
	if err != nil {
		if ctx.Err() != nil {
			return FileRangeHash{}, ctx.Err()
		}
		return FileRangeHash{}, err
	}
	defer closeResponseBody(response.Body)
	if response.StatusCode != http.StatusPartialContent {
		if response.StatusCode >= http.StatusOK && response.StatusCode < http.StatusMultipleChoices {
			return FileRangeHash{}, protocolError("range response status invalid")
		}
		return FileRangeHash{}, fmt.Errorf("%w: %v", ErrProviderUnavailable, upstream.SafeUpstreamHTTPError("p115", response.StatusCode))
	}
	if encoding := strings.TrimSpace(response.Header.Get("Content-Encoding")); encoding != "" && !strings.EqualFold(encoding, "identity") {
		return FileRangeHash{}, protocolError("range response encoding invalid")
	}
	if err := validateRangeResponseHeaders(response, request.Range, request.File.Size, expectedBytes); err != nil {
		return FileRangeHash{}, err
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, expectedBytes+1))
	if err != nil {
		if ctx.Err() != nil {
			return FileRangeHash{}, ctx.Err()
		}
		return FileRangeHash{}, protocolError("range response read failed")
	}
	if int64(len(body)) != expectedBytes {
		return FileRangeHash{}, protocolError("range response length invalid")
	}
	hash := sha1.Sum(body)
	return FileRangeHash{SHA1: strings.ToUpper(hex.EncodeToString(hash[:])), BytesRead: expectedBytes}, nil
}

// DeleteFile serializes destructive requests by normalized Provider UID and
// deletes exactly one caller-verified file ID.
func (a *CookieHTTPAdapter) DeleteFile(ctx context.Context, credential Credential, fileID string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	numericFileID, err := strconv.ParseUint(strings.TrimSpace(fileID), 10, 64)
	if err != nil || numericFileID == 0 {
		return ErrInvalidRequest
	}
	providerUserID, err := validateCookieHTTPCredential(credential)
	if err != nil {
		return err
	}
	release, err := a.deleteLocks.acquire(ctx, providerUserID)
	if err != nil {
		return err
	}
	defer release()

	form := url.Values{"fid": {strconv.FormatUint(numericFileID, 10)}}.Encode()
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, a.deleteEndpoint.String(), strings.NewReader(form))
	if err != nil {
		return protocolError("delete request build failed")
	}
	request.Header.Set("Accept", "*/*")
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Cookie", credential.Cookie)
	request.Header.Set("User-Agent", credential.UserAgent)

	if err := a.waitDataPlane(ctx, credential); err != nil {
		return err
	}
	response, err := a.client.Do(request)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrProviderUnavailable, upstream.SafeUpstreamError(err, "p115"))
	}
	defer closeResponseBody(response.Body)
	if err := mapCookieAPIStatus(response.StatusCode); err != nil {
		return err
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, maxCookieResponseBody+1))
	if err != nil {
		return protocolError("delete response read failed")
	}
	if len(body) > maxCookieResponseBody {
		return protocolError("delete response too large")
	}
	var result struct {
		State *bool `json:"state"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return protocolError("delete response JSON invalid")
	}
	if result.State == nil {
		return protocolError("delete response state missing")
	}
	if !*result.State {
		return ErrProviderRejected
	}
	return nil
}

// getJSON sends a credential-bound GET and maps transport, HTTP, size, and JSON failures without response leakage.
func (a *CookieHTTPAdapter) getJSON(
	ctx context.Context,
	endpoint *url.URL,
	params url.Values,
	credential Credential,
	target any,
) (resultErr error) {
	ctx, observation := observeOperation(ctx, cookieGETStage(endpoint.Path))
	defer func() { resultErr = observation.finish(resultErr) }()
	if err := a.waitDataPlane(ctx, credential); err != nil {
		return err
	}
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

	response, err := observation.do(a.client, request)
	if err != nil {
		return err
	}
	defer closeResponseBody(response.Body)
	if err := mapCookieAPIStatus(response.StatusCode); err != nil {
		return err
	}

	body, err := io.ReadAll(io.LimitReader(response.Body, maxCookieResponseBody+1))
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return &operationFailure{cause: ErrProviderUnavailable, stage: observation.stage, reason: "response_read_failed", status: response.StatusCode}
	}
	if len(body) > maxCookieResponseBody {
		return protocolError("response too large")
	}
	if err := json.Unmarshal(body, target); err != nil {
		return protocolError("response JSON invalid")
	}
	return nil
}

// listSourceDirectory reads a stable, bounded snapshot of one directory so
// path resolution can detect duplicate names instead of accepting a first hit.
func (a *CookieHTTPAdapter) listSourceDirectory(
	ctx context.Context,
	credential Credential,
	parentID string,
) ([]sourceListEntry, error) {
	started := time.Now()
	timing, _ := ctx.Value(sourceReadTimingKey{}).(*sourceReadTiming)
	defer func() {
		if timing != nil {
			timing.listing += time.Since(started)
		}
	}()
	pageSize := a.sourceResolvePageSize
	if pageSize <= 0 || pageSize > sourceResolvePageSize {
		pageSize = sourceResolvePageSize
	}
	maxEntries := a.sourceResolveMaxEntries
	if maxEntries <= 0 || maxEntries > sourceResolveMaxEntries {
		maxEntries = sourceResolveMaxEntries
	}

	entries := make([]sourceListEntry, 0)
	expectedCount := -1
	for offset := 0; ; {
		var response struct {
			providerErrorCodes
			State  *bool             `json:"state"`
			CID    jsonUint64        `json:"cid"`
			Count  jsonUint64        `json:"count"`
			Offset jsonUint64        `json:"offset"`
			Data   []json.RawMessage `json:"data"`
		}
		params := url.Values{
			"aid":           {"1"},
			"asc":           {"1"},
			"cid":           {parentID},
			"count_folders": {"1"},
			"cur":           {"1"},
			"fc_mix":        {"1"},
			"limit":         {strconv.Itoa(pageSize)},
			"o":             {"file_name"},
			"offset":        {strconv.Itoa(offset)},
			"show_dir":      {"1"},
		}
		if timing != nil {
			timing.pages++
		}
		if err := a.getJSON(ctx, a.sourceListEndpoint, params, credential, &response); err != nil {
			return nil, err
		}
		if response.State == nil {
			return nil, protocolError("source list response state missing")
		}
		if !*response.State {
			return nil, response.rejection()
		}
		if !response.CID.set || !response.Count.set || !response.Offset.set || response.Data == nil ||
			response.Count.value > math.MaxInt || response.Offset.value > math.MaxInt {
			return nil, protocolError("source list response fields missing")
		}
		if strconv.FormatUint(response.CID.value, 10) != parentID {
			return nil, ErrSourceFileNotFound
		}
		count := int(response.Count.value)
		if count > maxEntries {
			return nil, ErrSourceDirectoryTooLarge
		}
		if expectedCount < 0 {
			expectedCount = count
		} else if expectedCount != count {
			return nil, protocolError("source list count changed")
		}
		if int(response.Offset.value) != offset || offset > count || offset+len(response.Data) > count {
			return nil, protocolError("source list pagination invalid")
		}
		if len(response.Data) == 0 && offset < count {
			return nil, protocolError("source list page empty")
		}
		for _, raw := range response.Data {
			entry, err := decodeSourceListEntry(raw)
			if err != nil {
				return nil, err
			}
			if entry.ParentID != parentID {
				return nil, protocolError("source list parent mismatch")
			}
			entries = append(entries, entry)
		}
		offset += len(response.Data)
		if offset >= count {
			return entries, nil
		}
	}
}

// sourceListEntry is one listed child. invalid is set only for a file whose
// identity fields failed validation: the listing stays usable for siblings,
// and only selecting that exact file fails closed.
type sourceListEntry struct {
	File
	invalid error
}

// decodeSourceListEntry maps the pinned web /files short fields without
// accepting aliases from unrelated Provider endpoints. Structural failures
// (JSON, name, directory shape) still reject the whole listing.
func decodeSourceListEntry(data json.RawMessage) (sourceListEntry, error) {
	var payload struct {
		FileID     jsonUint64 `json:"fid"`
		CategoryID jsonUint64 `json:"cid"`
		ParentID   jsonUint64 `json:"pid"`
		Name       *string    `json:"n"`
		PickCode   *string    `json:"pc"`
		SHA1       *string    `json:"sha"`
		Size       jsonUint64 `json:"s"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		return sourceListEntry{}, protocolError("source list item invalid")
	}
	if payload.Name == nil || !payload.CategoryID.set {
		return sourceListEntry{}, protocolError("source list fields missing")
	}
	name := *payload.Name
	if strings.TrimSpace(name) == "" || !utf8.ValidString(name) || strings.ContainsAny(name, "\x00\r\n") {
		return sourceListEntry{}, protocolError("source list name invalid")
	}
	if !payload.FileID.set {
		if payload.CategoryID.value == 0 || !payload.ParentID.set {
			return sourceListEntry{}, protocolError("source directory fields missing")
		}
		return sourceListEntry{File: File{
			ID:          strconv.FormatUint(payload.CategoryID.value, 10),
			ParentID:    strconv.FormatUint(payload.ParentID.value, 10),
			Name:        name,
			IsDirectory: true,
		}}, nil
	}
	// Keep name and parent so path matching and the listing parent check still
	// apply; identity failures surface only if this file is selected.
	invalidFile := func(reason string) (sourceListEntry, error) {
		return sourceListEntry{
			File:    File{ParentID: strconv.FormatUint(payload.CategoryID.value, 10), Name: name},
			invalid: protocolError(reason),
		}, nil
	}
	if payload.FileID.value == 0 || payload.PickCode == nil || payload.SHA1 == nil || !payload.Size.set ||
		payload.Size.value > math.MaxInt64 {
		return invalidFile("source file fields missing")
	}
	pickCode := strings.TrimSpace(*payload.PickCode)
	if !isValidDownloadPickCode(pickCode) {
		return invalidFile("source file pickcode invalid")
	}
	normalizedSHA1, err := normalizeSHA1(*payload.SHA1)
	if err != nil {
		return invalidFile("source file hash invalid")
	}
	return sourceListEntry{File: File{
		ID:          strconv.FormatUint(payload.FileID.value, 10),
		PickCode:    pickCode,
		ParentID:    strconv.FormatUint(payload.CategoryID.value, 10),
		Name:        name,
		SHA1:        normalizedSHA1,
		Size:        int64(payload.Size.value),
		IsDirectory: false,
	}}, nil
}

type normalizedFilePathQuery struct {
	RootID   string
	Segments []string
}

type normalizedDirectoryPathQuery struct {
	RootID   string
	Segments []string
	Path     string
}

// normalizeFilePathQuery fixes the first path format to slash-separated,
// relative segments so a mapping cannot escape its configured Provider root.
func normalizeFilePathQuery(query FilePathQuery) (normalizedFilePathQuery, error) {
	rootID, err := normalizeOptionalProviderID(query.RootID)
	if err != nil || rootID == "" {
		return normalizedFilePathQuery{}, ErrInvalidRequest
	}
	pathValue := query.RelativePath
	if pathValue == "" || pathValue != strings.TrimSpace(pathValue) || !utf8.ValidString(pathValue) ||
		len(pathValue) > maxSourcePathLength || strings.HasPrefix(pathValue, "/") || strings.HasSuffix(pathValue, "/") ||
		strings.ContainsAny(pathValue, "\\\x00\r\n") {
		return normalizedFilePathQuery{}, ErrInvalidRequest
	}
	segments := strings.Split(pathValue, "/")
	for _, segment := range segments {
		if segment == "" || segment == "." || segment == ".." || strings.TrimSpace(segment) == "" ||
			len(segment) > maxSourcePathSegmentLength {
			return normalizedFilePathQuery{}, ErrInvalidRequest
		}
	}
	return normalizedFilePathQuery{RootID: rootID, Segments: segments}, nil
}

// normalizeDirectoryPathQuery accepts one optional leading slash for admin UX
// while rejecting root, empty, escaping, or ambiguous path forms.
func normalizeDirectoryPathQuery(query DirectoryPathQuery) (normalizedDirectoryPathQuery, error) {
	rootID, err := normalizeOptionalProviderID(query.RootID)
	if err != nil || rootID == "" {
		return normalizedDirectoryPathQuery{}, ErrInvalidRequest
	}
	pathValue := query.RelativePath
	if pathValue == "" || pathValue != strings.TrimSpace(pathValue) || !utf8.ValidString(pathValue) ||
		len(pathValue) > maxSourcePathLength || strings.HasSuffix(pathValue, "/") ||
		strings.ContainsAny(pathValue, "\\\x00\r\n") {
		return normalizedDirectoryPathQuery{}, ErrInvalidRequest
	}
	pathValue = strings.TrimPrefix(pathValue, "/")
	if pathValue == "" || strings.HasPrefix(pathValue, "/") {
		return normalizedDirectoryPathQuery{}, ErrInvalidRequest
	}
	segments := strings.Split(pathValue, "/")
	for _, segment := range segments {
		if segment == "" || segment == "." || segment == ".." || strings.TrimSpace(segment) == "" ||
			len(segment) > maxSourcePathSegmentLength {
			return normalizedDirectoryPathQuery{}, ErrInvalidRequest
		}
	}
	return normalizedDirectoryPathQuery{
		RootID: rootID, Segments: segments, Path: "/" + strings.Join(segments, "/"),
	}, nil
}

// validateFileRangeRequest accepts only a non-directory file and one inclusive
// range inside both the file size and Ember's fixed one-megabyte safety cap.
func validateFileRangeRequest(request FileRangeRequest) (int64, error) {
	if request.File.IsDirectory || !isValidDownloadPickCode(strings.TrimSpace(request.File.PickCode)) || request.File.Size <= 0 ||
		request.Range.Start < 0 || request.Range.End < request.Range.Start || request.Range.End >= request.File.Size ||
		(request.UserAgent != "" && (request.UserAgent != strings.TrimSpace(request.UserAgent) ||
			len(request.UserAgent) > maxDownloadUserAgentLength || strings.ContainsAny(request.UserAgent, "\r\n"))) {
		return 0, ErrInvalidRequest
	}
	length := request.Range.End - request.Range.Start + 1
	if length <= 0 || length > maxSourceRangeBytes {
		return 0, ErrInvalidRequest
	}
	return length, nil
}

// validateRangeResponseHeaders prevents a Provider or intermediary from
// ignoring the request and streaming more source bytes than the challenge.
func validateRangeResponseHeaders(response *http.Response, byteRange ByteRange, fileSize, expectedBytes int64) error {
	contentRanges := response.Header.Values("Content-Range")
	if len(contentRanges) != 1 {
		return protocolError("range response content-range missing")
	}
	value := strings.TrimSpace(contentRanges[0])
	if !strings.HasPrefix(value, "bytes ") {
		return protocolError("range response content-range invalid")
	}
	rangeAndSize := strings.TrimPrefix(value, "bytes ")
	rangeValue, sizeValue, found := strings.Cut(rangeAndSize, "/")
	if !found || strings.Contains(sizeValue, "/") {
		return protocolError("range response content-range invalid")
	}
	startValue, endValue, found := strings.Cut(rangeValue, "-")
	if !found || strings.Contains(endValue, "-") {
		return protocolError("range response content-range invalid")
	}
	start, startErr := strconv.ParseInt(startValue, 10, 64)
	end, endErr := strconv.ParseInt(endValue, 10, 64)
	total, totalErr := strconv.ParseInt(sizeValue, 10, 64)
	if startErr != nil || endErr != nil || totalErr != nil || start != byteRange.Start || end != byteRange.End || total != fileSize {
		return protocolError("range response content-range mismatch")
	}
	if response.ContentLength != expectedBytes {
		return protocolError("range response content-length mismatch")
	}
	return nil
}

// decodeSHASearchFile converts the legacy single-result object into Provider-neutral file identity.
func decodeSHASearchFile(data json.RawMessage) (File, error) {
	if len(data) == 0 || string(data) == "null" {
		return File{}, protocolError("SHA1 search data missing")
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return File{}, protocolError("SHA1 search data invalid")
	}
	_, hasFileID := fields["file_id"]
	_, hasCategoryID := fields["category_id"]
	if !hasFileID && !hasCategoryID {
		return decodeWebSHASearchFile(data)
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

// decodeWebSHASearchFile maps the short Web fields used by the pinned
// fs_shasearch normalizer into a complete Provider-neutral file identity.
func decodeWebSHASearchFile(data json.RawMessage) (File, error) {
	var payload struct {
		FileID       jsonUint64 `json:"fid"`
		ParentID     jsonUint64 `json:"cid"`
		Name         *string    `json:"n"`
		FallbackName *string    `json:"fn"`
		LongName     *string    `json:"file_name"`
		PickCode     *string    `json:"pc"`
		SHA1         *string    `json:"sha"`
		FallbackSHA1 *string    `json:"sha1"`
		Size         jsonUint64 `json:"s"`
		FallbackSize jsonUint64 `json:"fs"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		return File{}, protocolError("SHA1 search data invalid")
	}
	name := firstProviderString(payload.Name, payload.FallbackName, payload.LongName)
	sha1Value := firstProviderString(payload.SHA1, payload.FallbackSHA1)
	size := firstProviderUint64(payload.Size, payload.FallbackSize)
	if !payload.FileID.set || payload.FileID.value == 0 || !payload.ParentID.set ||
		name == nil || payload.PickCode == nil || sha1Value == nil || !size.set || size.value > math.MaxInt64 {
		return File{}, protocolError("SHA1 search fields missing")
	}
	fileName := *name
	pickCode := strings.TrimSpace(*payload.PickCode)
	if strings.TrimSpace(fileName) == "" || pickCode == "" || strings.ContainsAny(fileName+pickCode, "\r\n") {
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
		ParentID:    strconv.FormatUint(payload.ParentID.value, 10),
		Name:        fileName,
		SHA1:        normalizedSHA1,
		Size:        int64(size.value),
		IsDirectory: isDirectory,
	}, nil
}

// firstProviderString mirrors the pinned Web normalizer's first non-empty
// alias selection while preserving an explicit empty value for validation.
func firstProviderString(values ...*string) *string {
	for _, value := range values {
		if value != nil && *value != "" {
			return value
		}
	}
	for _, value := range values {
		if value != nil {
			return value
		}
	}
	return nil
}

// firstProviderUint64 mirrors the pinned Web normalizer's first non-zero
// numeric alias selection while preserving an explicit zero for validation.
func firstProviderUint64(values ...jsonUint64) jsonUint64 {
	for _, value := range values {
		if value.set && value.value != 0 {
			return value
		}
	}
	for _, value := range values {
		if value.set {
			return value
		}
	}
	return jsonUint64{}
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

// downloadRejection classifies only the documented downurl errno allowlist.
// Unknown or malformed codes keep the existing rejection semantics; raw text
// never supplies credential or account-health evidence.
func downloadRejection(errno json.RawMessage) error {
	switch providerCodeValue(errno) {
	case "70005", "231011", "50003", "50015":
		return ErrFileUnavailable
	default:
		return stateFalseRejection(errno)
	}
}

// providerErrorCodes captures the two numeric code fields 115 Web APIs use on
// state=false. Community clients (115driver BasicResp) read errno first and
// errNo only when errno is absent or zero.
type providerErrorCodes struct {
	Errno json.RawMessage `json:"errno"`
	ErrNo json.RawMessage `json:"errNo"`
}

// rejection maps a state=false response to the shared credential allowlist.
func (codes providerErrorCodes) rejection() error {
	return stateFalseRejection(codes.Errno, codes.ErrNo)
}

// stateFalseRejection uses the first non-zero code. Only the not-logged-in
// codes are endpoint independent; file-scoped codes stay with downurl, and
// unknown codes keep ErrProviderRejected. A present but malformed code fails
// closed instead of falling through, so it can never disable an account.
func stateFalseRejection(codes ...json.RawMessage) error {
	for _, raw := range codes {
		if len(raw) == 0 || string(raw) == "null" {
			continue
		}
		value := providerCodeValue(raw)
		if value == "" {
			return ErrProviderRejected
		}
		if value == "0" {
			continue
		}
		if value == "99" || value == "990001" {
			return ErrCredentialRejected
		}
		return ErrProviderRejected
	}
	return ErrProviderRejected
}

// providerCodeValue returns a validated code or empty for missing/malformed input.
func providerCodeValue(raw json.RawMessage) string {
	var code jsonProviderCode
	if len(raw) == 0 || json.Unmarshal(raw, &code) != nil {
		return ""
	}
	return code.value
}

// decodeChromeDownloadURL requires one pickcode-bound entry before treating
// false/null/empty URLs as file failures instead of account protocol failures.
func decodeChromeDownloadURL(plaintext []byte, expectedPickCode string) (string, error) {
	var entries map[string]struct {
		PickCode *string         `json:"pick_code"`
		URL      json.RawMessage `json:"url"`
	}
	if err := json.Unmarshal(plaintext, &entries); err != nil {
		return "", protocolError("download data JSON invalid")
	}
	if len(entries) != 1 {
		return "", protocolError("download data entry count invalid")
	}
	for _, entry := range entries {
		if entry.PickCode == nil || *entry.PickCode != expectedPickCode {
			return "", protocolError("download pickcode mismatch")
		}
		raw := bytes.TrimSpace(entry.URL)
		if len(raw) == 0 || bytes.Equal(raw, []byte("false")) || bytes.Equal(raw, []byte("null")) {
			return "", ErrFileUnavailable
		}
		var download struct {
			Value *string `json:"url"`
		}
		if err := json.Unmarshal(raw, &download); err != nil {
			return "", protocolError("download URL object invalid")
		}
		if download.Value == nil || strings.TrimSpace(*download.Value) == "" {
			return "", ErrFileUnavailable
		}
		return *download.Value, nil
	}
	return "", protocolError("download data missing")
}

// parseDownloadURL validates the redirect target and maps provider t/c/f query semantics.
func parseDownloadURL(rawURL string, now time.Time, hostAllowed func(string) bool) (DownloadURLResult, error) {
	if rawURL == "" || rawURL != strings.TrimSpace(rawURL) || len(rawURL) > maxDownloadURLLength {
		return DownloadURLResult{}, protocolError("download URL invalid")
	}
	parsed, err := url.Parse(rawURL)
	if err != nil || !parsed.IsAbs() || parsed.Opaque != "" || parsed.Host == "" || parsed.Path == "" {
		return DownloadURLResult{}, protocolError("download URL invalid")
	}
	hostname := strings.ToLower(parsed.Hostname())
	safeScheme := safeDownloadURLScheme(parsed.Scheme)
	safeHostname := safeDownloadURLHostname(hostname)
	if parsed.Scheme != "https" {
		return DownloadURLResult{}, downloadURLPolicyError(DownloadURLPolicySchemeNotHTTPS, safeScheme, safeHostname)
	}
	if parsed.User != nil {
		return DownloadURLResult{}, downloadURLPolicyError(DownloadURLPolicyUserinfo, safeScheme, safeHostname)
	}
	if parsed.Port() != "" {
		return DownloadURLResult{}, downloadURLPolicyError(DownloadURLPolicyExplicitPort, safeScheme, safeHostname)
	}
	if parsed.Fragment != "" {
		return DownloadURLResult{}, downloadURLPolicyError(DownloadURLPolicyFragment, safeScheme, safeHostname)
	}
	if net.ParseIP(hostname) != nil {
		return DownloadURLResult{}, downloadURLPolicyError(DownloadURLPolicyIPLiteral, safeScheme, "")
	}
	if !hostAllowed(hostname) {
		return DownloadURLResult{}, downloadURLPolicyError(DownloadURLPolicyHostNotAllowed, safeScheme, safeHostname)
	}
	query, err := url.ParseQuery(parsed.RawQuery)
	if err != nil {
		return DownloadURLResult{}, protocolError("download URL query invalid")
	}
	expiryValue, err := singleDownloadQueryValue(query, "t")
	if err != nil {
		return DownloadURLResult{}, err
	}
	expiryUnix, err := strconv.ParseInt(expiryValue, 10, 64)
	if err != nil || expiryUnix <= 0 {
		return DownloadURLResult{}, protocolError("download URL expiry invalid")
	}
	expiresAt := time.Unix(expiryUnix, 0).UTC()
	if !expiresAt.After(now) {
		return DownloadURLResult{}, ErrDownloadURLExpired
	}
	concurrencyValue, err := singleDownloadQueryValue(query, "c")
	if err != nil {
		return DownloadURLResult{}, err
	}
	concurrentOpenLimit, err := strconv.ParseInt(concurrencyValue, 10, 64)
	if err != nil {
		return DownloadURLResult{}, protocolError("download URL concurrency invalid")
	}
	if concurrentOpenLimit < 0 {
		return DownloadURLResult{}, ErrFileUnavailable
	}
	flag, err := singleDownloadQueryValue(query, "f")
	if err != nil {
		return DownloadURLResult{}, err
	}
	var headerMode DownloadHeaderMode
	switch flag {
	case "", "0":
		headerMode = DownloadHeadersNone
	case "1":
		headerMode = DownloadHeadersSameUserAgent
	case "3":
		headerMode = DownloadHeadersSameUserAgentAndCookie
	default:
		return DownloadURLResult{}, ErrDownloadURLIncompatible
	}
	return DownloadURLResult{
		URL:                 rawURL,
		ExpiresAt:           expiresAt,
		HeaderMode:          headerMode,
		ConcurrentOpenLimit: concurrentOpenLimit,
	}, nil
}

func downloadURLPolicyError(reason DownloadURLPolicyReason, scheme, host string) error {
	return &DownloadURLPolicyError{Reason: reason, Scheme: scheme, Host: host}
}

func safeDownloadURLScheme(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "http" || value == "https" {
		return value
	}
	return "other"
}

func safeDownloadURLHostname(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" || len(value) > 253 {
		return ""
	}
	for index := 0; index < len(value); index++ {
		character := value[index]
		if (character >= 'a' && character <= 'z') || (character >= '0' && character <= '9') ||
			character == '.' || character == '-' {
			continue
		}
		return ""
	}
	return value
}

func singleDownloadQueryValue(query url.Values, key string) (string, error) {
	values, exists := query[key]
	if !exists || len(values) != 1 {
		return "", protocolError("download URL query field invalid")
	}
	return values[0], nil
}

// IsAllowedDownloadHost keeps redirects inside the initial 115-owned hostname
// boundary. Gateway and caches reuse this exact allowlist; it is not widened here.
func IsAllowedDownloadHost(hostname string) bool {
	return isAllowed115DownloadHost(hostname)
}

// isAllowed115DownloadHost keeps redirects inside the initial 115-owned hostname boundary.
func isAllowed115DownloadHost(hostname string) bool {
	return hostname == "115.com" || strings.HasSuffix(hostname, ".115.com") || hostname == "cdnfhnfile.115cdn.net"
}

// isValidDownloadPickCode accepts one lowercase file pickcode and rejects directory-like f prefixes.
func isValidDownloadPickCode(value string) bool {
	if len(value) < 6 || len(value) > 64 || strings.HasPrefix(value, "f") {
		return false
	}
	for index := 0; index < len(value); index++ {
		character := value[index]
		if (character < '0' || character > '9') && (character < 'a' || character > 'z') {
			return false
		}
	}
	return true
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

// responseDrainLimit bounds how much of an unread body is discarded so the
// connection can return to the pool; larger leftovers (for example a 200 full
// file answering a Range request) are closed without reuse.
const responseDrainLimit = 64 << 10

// closeResponseBody drains a bounded remainder before closing, letting error
// and early-return paths keep their keep-alive connection.
func closeResponseBody(body io.ReadCloser) {
	_, _ = io.Copy(io.Discard, io.LimitReader(body, responseDrainLimit))
	_ = body.Close()
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
	return &protocolFailure{reason: reason}
}

func rapidUploadProtocolError(
	phase RapidUploadProtocolPhase,
	response *http.Response,
	body []byte,
) *RapidUploadProtocolError {
	evidence := &RapidUploadProtocolError{Phase: phase}
	if response != nil {
		evidence.ContentType = safeRapidUploadContentType(response.Header.Get("Content-Type"))
	}
	if body != nil {
		evidence.BodyBytes = len(body)
		evidence.BodyShape = safeRapidUploadBodyShape(body)
	}
	return evidence
}

func safeRapidUploadContentType(value string) string {
	mediaType, _, err := mime.ParseMediaType(strings.TrimSpace(value))
	if err != nil {
		return "unknown"
	}
	switch strings.ToLower(mediaType) {
	case "application/json", "application/octet-stream", "text/plain":
		return strings.ToLower(mediaType)
	default:
		return "other"
	}
}

func safeRapidUploadBodyShape(body []byte) string {
	trimmed := bytes.TrimSpace(body)
	if len(trimmed) == 0 {
		return "empty"
	}
	switch trimmed[0] {
	case '{':
		return "json_object"
	case '[':
		return "json_array"
	default:
		return "binary"
	}
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
