package playbackgateway

import (
	"context"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

var localMediaConditionalHeaders = [...]string{
	"If-Range",
	"If-Match",
	"If-None-Match",
	"If-Modified-Since",
	"If-Unmodified-Since",
}

// localMediaServeResult records only the transport outcome needed by the final
// Gateway decision log. ReadError and WriteError are never written directly to
// a log or response.
type localMediaServeResult struct {
	StatusCode  int
	Interrupted bool
	ReadError   error
	WriteError  error
}

// localMediaRequestEligible rejects request shapes whose conditional,
// multipart, or representation semantics must remain authoritative at Emby.
// Invalid single ranges stay eligible so the selected local file returns 416.
func localMediaRequestEligible(request *http.Request) bool {
	if request == nil || (request.Method != http.MethodGet && request.Method != http.MethodHead) {
		return false
	}
	for _, name := range localMediaConditionalHeaders {
		if _, present := headerValuesFold(request.Header, name); present {
			return false
		}
	}
	ranges, rangePresent := headerValuesFold(request.Header, "Range")
	if rangePresent && (len(ranges) != 1 || strings.Contains(ranges[0], ",")) {
		return false
	}
	return identityEncodingAccepted(request.Header)
}

// serveLocalMediaFile streams from the descriptor returned by the secure local
// resolver. Renaming or replacing the pathname after Open cannot change bytes.
func serveLocalMediaFile(writer http.ResponseWriter, request *http.Request, file *os.File) localMediaServeResult {
	if file == nil {
		return localMediaServeResult{}
	}
	return serveLocalMediaContent(writer, request, file.Name(), file)
}

// serveLocalMediaContent delegates standard single-range parsing to net/http
// while pinning Ember's no-store, identity-only representation contract on
// every local status, including 416.
func serveLocalMediaContent(
	writer http.ResponseWriter,
	request *http.Request,
	name string,
	content io.ReadSeeker,
) localMediaServeResult {
	if writer == nil || request == nil || content == nil {
		return localMediaServeResult{}
	}
	size, err := content.Seek(0, io.SeekEnd)
	if err != nil || size < 0 {
		return localMediaServeResult{ReadError: ErrLocalMediaUnavailable}
	}
	if _, err := content.Seek(0, io.SeekStart); err != nil {
		return localMediaServeResult{ReadError: ErrLocalMediaUnavailable}
	}
	response := &localMediaResponseWriter{
		ResponseWriter: writer, contentType: localMediaContentType(name), method: request.Method, size: size,
		expectedBodyBytes: -1,
	}
	if _, rangePresent := headerValuesFold(request.Header, "Range"); size == 0 && rangePresent {
		response.WriteHeader(http.StatusRequestedRangeNotSatisfiable)
		return localMediaServeResult{StatusCode: response.statusCode}
	}
	serveRequest := request
	if values, present := headerValuesFold(request.Header, "Range"); present && len(values) == 1 && values[0] == "" {
		serveRequest = request.Clone(request.Context())
		serveRequest.Header = request.Header.Clone()
		serveRequest.Header.Set("Range", "invalid")
	}
	observer := &localMediaReadSeeker{ctx: request.Context(), delegate: content}
	http.ServeContent(response, serveRequest, name, time.Time{}, observer)
	if response.statusCode == 0 {
		response.statusCode = http.StatusOK
	}
	shortBody := response.expectedBodyBytes >= 0 && response.bodyBytes != response.expectedBodyBytes
	return localMediaServeResult{
		StatusCode: response.statusCode, Interrupted: observer.readErr != nil || response.writeErr != nil || shortBody,
		ReadError: observer.readErr, WriteError: response.writeErr,
	}
}

type localMediaReadSeeker struct {
	ctx      context.Context
	delegate io.ReadSeeker
	readErr  error
}

func (reader *localMediaReadSeeker) Read(buffer []byte) (int, error) {
	if err := reader.ctx.Err(); err != nil {
		reader.readErr = err
		return 0, err
	}
	count, err := reader.delegate.Read(buffer)
	if err != nil && err != io.EOF {
		reader.readErr = err
	}
	return count, err
}

func (reader *localMediaReadSeeker) Seek(offset int64, whence int) (int64, error) {
	return reader.delegate.Seek(offset, whence)
}

type localMediaResponseWriter struct {
	http.ResponseWriter
	contentType       string
	method            string
	size              int64
	statusCode        int
	bodyBytes         int64
	expectedBodyBytes int64
	writeErr          error
}

// WriteHeader pins the local representation headers and captures the expected
// body length before the underlying writer makes the response immutable.
func (writer *localMediaResponseWriter) WriteHeader(statusCode int) {
	if writer.statusCode != 0 {
		return
	}
	writer.statusCode = statusCode
	header := writer.Header()
	header.Set("Cache-Control", "private, no-store")
	header.Set("Accept-Ranges", "bytes")
	header.Set("Content-Type", writer.contentType)
	header.Del("Content-Encoding")
	header.Del("ETag")
	header.Del("Last-Modified")
	if statusCode == http.StatusRequestedRangeNotSatisfiable {
		header.Set("Content-Range", "bytes */"+strconv.FormatInt(writer.size, 10))
		header.Del("Content-Length")
		header.Del("X-Content-Type-Options")
	}
	if writer.method != http.MethodHead && (statusCode == http.StatusOK || statusCode == http.StatusPartialContent) {
		if contentLength, err := strconv.ParseInt(header.Get("Content-Length"), 10, 64); err == nil && contentLength >= 0 {
			writer.expectedBodyBytes = contentLength
		}
	}
	writer.ResponseWriter.WriteHeader(statusCode)
}

// Write counts bytes accepted by the client-facing writer and retains only the
// error object in request memory so ServeContent's discarded copy result cannot
// turn a partial local response into a successful final decision log.
func (writer *localMediaResponseWriter) Write(body []byte) (int, error) {
	if writer.statusCode == 0 {
		writer.WriteHeader(http.StatusOK)
	}
	if writer.statusCode == http.StatusRequestedRangeNotSatisfiable {
		return len(body), nil
	}
	count, err := writer.ResponseWriter.Write(body)
	writer.bodyBytes += int64(count)
	if err == nil && count != len(body) {
		err = io.ErrShortWrite
	}
	if err != nil && writer.writeErr == nil {
		writer.writeErr = err
	}
	return count, err
}

func headerValuesFold(header http.Header, name string) ([]string, bool) {
	var values []string
	present := false
	for key, candidates := range header {
		if strings.EqualFold(key, name) {
			if present {
				values = append(values, candidates...)
				continue
			}
			present = true
			values = append(values, candidates...)
		}
	}
	return values, present
}

func identityEncodingAccepted(header http.Header) bool {
	values, present := headerValuesFold(header, "Accept-Encoding")
	if !present || (len(values) == 1 && strings.TrimSpace(values[0]) == "") {
		return true
	}
	qualityByToken := make(map[string]int)
	for _, value := range values {
		for _, item := range strings.Split(value, ",") {
			token, quality, ok := parseAcceptEncodingItem(item)
			if !ok {
				return false
			}
			if previous, exists := qualityByToken[token]; exists && previous != quality {
				return false
			}
			qualityByToken[token] = quality
		}
	}
	if quality, exists := qualityByToken["identity"]; exists {
		return quality > 0
	}
	if quality, exists := qualityByToken["*"]; exists && quality == 0 {
		return false
	}
	return true
}

func parseAcceptEncodingItem(item string) (string, int, bool) {
	parts := strings.Split(item, ";")
	if len(parts) == 0 || len(parts) > 2 {
		return "", 0, false
	}
	token := strings.ToLower(strings.TrimSpace(parts[0]))
	if !validHTTPToken(token) {
		return "", 0, false
	}
	quality := 1000
	if len(parts) == 2 {
		parameter := strings.SplitN(strings.TrimSpace(parts[1]), "=", 2)
		if len(parameter) != 2 || !strings.EqualFold(strings.TrimSpace(parameter[0]), "q") {
			return "", 0, false
		}
		var ok bool
		quality, ok = parseQualityValue(strings.TrimSpace(parameter[1]))
		if !ok {
			return "", 0, false
		}
	}
	return token, quality, true
}

func parseQualityValue(value string) (int, bool) {
	if value == "0" || value == "0." {
		return 0, true
	}
	if value == "1" || value == "1." {
		return 1000, true
	}
	whole, fraction, found := strings.Cut(value, ".")
	if !found || len(fraction) == 0 || len(fraction) > 3 || (whole != "0" && whole != "1") {
		return 0, false
	}
	for _, digit := range fraction {
		if digit < '0' || digit > '9' || (whole == "1" && digit != '0') {
			return 0, false
		}
	}
	for len(fraction) < 3 {
		fraction += "0"
	}
	parsed, err := strconv.Atoi(fraction)
	return parsed, err == nil
}

func validHTTPToken(value string) bool {
	if value == "" {
		return false
	}
	for _, character := range value {
		if character > 127 || !(character >= 'a' && character <= 'z') && !(character >= '0' && character <= '9') &&
			!strings.ContainsRune("!#$%&'*+-.^_`|~", character) {
			return false
		}
	}
	return true
}

func localMediaContentType(name string) string {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".mkv":
		return "video/x-matroska"
	case ".mp4", ".m4v":
		return "video/mp4"
	case ".mov":
		return "video/quicktime"
	case ".avi":
		return "video/x-msvideo"
	case ".ts", ".m2ts":
		return "video/mp2t"
	case ".webm":
		return "video/webm"
	case ".mpeg", ".mpg":
		return "video/mpeg"
	}
	if contentType := mime.TypeByExtension(filepath.Ext(name)); contentType != "" {
		return contentType
	}
	return "application/octet-stream"
}
