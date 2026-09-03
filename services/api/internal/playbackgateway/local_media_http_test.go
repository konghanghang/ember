package playbackgateway

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestLocalMediaRequestEligibility(t *testing.T) {
	tests := []struct {
		name      string
		configure func(*http.Request)
		want      bool
	}{
		{name: "plain get", want: true},
		{name: "plain head", configure: func(request *http.Request) { request.Method = http.MethodHead }, want: true},
		{name: "single range", configure: func(request *http.Request) { request.Header.Set("Range", "bytes=0-9") }, want: true},
		{name: "invalid single range stays local", configure: func(request *http.Request) { request.Header.Set("Range", "invalid") }, want: true},
		{name: "multiple range values", configure: func(request *http.Request) {
			request.Header.Add("Range", "bytes=0-9")
			request.Header.Add("Range", "bytes=10-19")
		}},
		{name: "multipart range", configure: func(request *http.Request) { request.Header.Set("Range", "bytes=0-9,20-29") }},
		{name: "conditional if range", configure: func(request *http.Request) { request.Header.Set("If-Range", "fixture") }},
		{name: "conditional if match", configure: func(request *http.Request) { request.Header.Set("If-Match", "fixture") }},
		{name: "conditional if none match", configure: func(request *http.Request) { request.Header.Set("If-None-Match", "fixture") }},
		{name: "conditional if modified since", configure: func(request *http.Request) { request.Header.Set("If-Modified-Since", "fixture") }},
		{name: "conditional if unmodified since", configure: func(request *http.Request) { request.Header.Set("If-Unmodified-Since", "fixture") }},
		{name: "post", configure: func(request *http.Request) { request.Method = http.MethodPost }},
		{name: "gzip preference permits identity", configure: func(request *http.Request) { request.Header.Set("Accept-Encoding", "gzip, br") }, want: true},
		{name: "explicit identity", configure: func(request *http.Request) { request.Header.Set("Accept-Encoding", "identity;q=0.5, gzip") }, want: true},
		{name: "identity rejected", configure: func(request *http.Request) { request.Header.Set("Accept-Encoding", "gzip, identity;q=0") }},
		{name: "wildcard rejects identity", configure: func(request *http.Request) { request.Header.Set("Accept-Encoding", "gzip, *;q=0") }},
		{name: "invalid q value", configure: func(request *http.Request) { request.Header.Set("Accept-Encoding", "gzip;q=1.5") }},
		{name: "conflicting duplicate", configure: func(request *http.Request) { request.Header.Set("Accept-Encoding", "identity;q=1, identity;q=0") }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, "/fixture", nil)
			if test.configure != nil {
				test.configure(request)
			}
			if got := localMediaRequestEligible(request); got != test.want {
				t.Fatalf("localMediaRequestEligible() = %t, want %t", got, test.want)
			}
		})
	}
}

func TestServeLocalMediaContentContracts(t *testing.T) {
	content := []byte("0123456789")
	tests := []struct {
		name        string
		method      string
		rangeHeader *string
		wantStatus  int
		wantBody    string
		wantRange   string
		wantLength  string
	}{
		{name: "full get", method: http.MethodGet, wantStatus: http.StatusOK, wantBody: string(content), wantLength: "10"},
		{name: "head", method: http.MethodHead, wantStatus: http.StatusOK, wantLength: "10"},
		{name: "fixed range", method: http.MethodGet, rangeHeader: stringPointer("bytes=2-5"), wantStatus: http.StatusPartialContent, wantBody: "2345", wantRange: "bytes 2-5/10", wantLength: "4"},
		{name: "open range", method: http.MethodGet, rangeHeader: stringPointer("bytes=7-"), wantStatus: http.StatusPartialContent, wantBody: "789", wantRange: "bytes 7-9/10", wantLength: "3"},
		{name: "suffix range", method: http.MethodGet, rangeHeader: stringPointer("bytes=-3"), wantStatus: http.StatusPartialContent, wantBody: "789", wantRange: "bytes 7-9/10", wantLength: "3"},
		{name: "invalid range", method: http.MethodGet, rangeHeader: stringPointer("invalid"), wantStatus: http.StatusRequestedRangeNotSatisfiable, wantRange: "bytes */10"},
		{name: "empty range", method: http.MethodGet, rangeHeader: stringPointer(""), wantStatus: http.StatusRequestedRangeNotSatisfiable, wantRange: "bytes */10"},
		{name: "unsatisfied range", method: http.MethodGet, rangeHeader: stringPointer("bytes=20-30"), wantStatus: http.StatusRequestedRangeNotSatisfiable, wantRange: "bytes */10"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(test.method, "/fixture.mkv", nil)
			if test.rangeHeader != nil {
				request.Header["Range"] = []string{*test.rangeHeader}
			}
			response := httptest.NewRecorder()
			result := serveLocalMediaContent(response, request, "fixture.mkv", bytes.NewReader(content))
			if response.Code != test.wantStatus || result.StatusCode != test.wantStatus || response.Body.String() != test.wantBody {
				t.Fatalf("response=%d result=%+v body=%q", response.Code, result, response.Body.String())
			}
			for header, want := range map[string]string{
				"Cache-Control":  "private, no-store",
				"Accept-Ranges":  "bytes",
				"Content-Range":  test.wantRange,
				"Content-Length": test.wantLength,
			} {
				if got := response.Header().Get(header); got != want {
					t.Fatalf("%s=%q, want %q; headers=%v", header, got, want, response.Header())
				}
			}
			if response.Header().Get("Content-Type") != "video/x-matroska" ||
				response.Header().Get("Content-Encoding") != "" || response.Header().Get("ETag") != "" ||
				response.Header().Get("Last-Modified") != "" {
				t.Fatalf("unexpected representation headers: %v", response.Header())
			}
		})
	}
}

func TestServeLocalMediaContentReturns416ForRangeOnEmptyFile(t *testing.T) {
	for _, method := range []string{http.MethodGet, http.MethodHead} {
		t.Run(method, func(t *testing.T) {
			request := httptest.NewRequest(method, "/empty.mkv", nil)
			request.Header.Set("Range", "bytes=0-0")
			response := httptest.NewRecorder()

			result := serveLocalMediaContent(response, request, "empty.mkv", bytes.NewReader(nil))
			if result.StatusCode != http.StatusRequestedRangeNotSatisfiable ||
				response.Code != http.StatusRequestedRangeNotSatisfiable || response.Body.Len() != 0 ||
				response.Header().Get("Content-Range") != "bytes */0" ||
				response.Header().Get("Cache-Control") != "private, no-store" {
				t.Fatalf("result=%+v response=%d headers=%v body=%q", result, response.Code, response.Header(), response.Body.String())
			}
		})
	}
}

func TestServeLocalMediaFileUsesOpenedDescriptorAfterPathReplacement(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "fixture.mp4")
	if err := os.WriteFile(path, []byte("original"), 0o600); err != nil {
		t.Fatal(err)
	}
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	if err := os.Rename(path, path+".old"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("replacement"), 0o600); err != nil {
		t.Fatal(err)
	}

	response := httptest.NewRecorder()
	result := serveLocalMediaFile(response, httptest.NewRequest(http.MethodGet, "/fixture.mp4", nil), file)
	if result.Interrupted || response.Code != http.StatusOK || response.Body.String() != "original" {
		t.Fatalf("result=%+v response=%d body=%q", result, response.Code, response.Body.String())
	}
}

func TestServeLocalMediaContentObservesCanceledReadAfterResponseStarts(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	reader := &cancelingReadSeeker{reader: bytes.NewReader([]byte("fixture-video")), cancel: cancel}
	request := httptest.NewRequest(http.MethodGet, "/fixture.mkv", nil).WithContext(ctx)
	response := httptest.NewRecorder()
	result := serveLocalMediaContent(response, request, "fixture.mkv", reader)
	if !result.Interrupted || !errors.Is(result.ReadError, context.Canceled) || response.Code != http.StatusOK {
		t.Fatalf("serveLocalMediaContent() = %+v status=%d", result, response.Code)
	}
}

func TestServeLocalMediaContentObservesShortSourceAndWriterFailure(t *testing.T) {
	t.Run("source ends before advertised length", func(t *testing.T) {
		request := httptest.NewRequest(http.MethodGet, "/fixture.mkv", nil)
		response := httptest.NewRecorder()
		reader := &advertisedSizeReadSeeker{content: []byte("short"), size: 10}

		result := serveLocalMediaContent(response, request, "fixture.mkv", reader)
		if !result.Interrupted || response.Code != http.StatusOK || response.Body.String() != "short" ||
			response.Header().Get("Content-Length") != "10" {
			t.Fatalf("result=%+v response=%d headers=%v body=%q", result, response.Code, response.Header(), response.Body.String())
		}
	})

	t.Run("client writer fails", func(t *testing.T) {
		writeErr := errors.New("fixture client disconnected")
		writer := &failingLocalResponseWriter{writeErr: writeErr}
		request := httptest.NewRequest(http.MethodGet, "/fixture.mkv", nil)

		result := serveLocalMediaContent(writer, request, "fixture.mkv", bytes.NewReader([]byte("fixture")))
		if !result.Interrupted || !errors.Is(result.WriteError, writeErr) || writer.statusCode != http.StatusOK {
			t.Fatalf("result=%+v writer=%+v", result, writer)
		}
	})
}

type cancelingReadSeeker struct {
	reader *bytes.Reader
	cancel context.CancelFunc
	done   bool
}

type advertisedSizeReadSeeker struct {
	content  []byte
	size     int64
	position int64
}

func (reader *advertisedSizeReadSeeker) Read(buffer []byte) (int, error) {
	if reader.position >= int64(len(reader.content)) {
		return 0, io.EOF
	}
	count := copy(buffer, reader.content[reader.position:])
	reader.position += int64(count)
	return count, nil
}

func (reader *advertisedSizeReadSeeker) Seek(offset int64, whence int) (int64, error) {
	var position int64
	switch whence {
	case io.SeekStart:
		position = offset
	case io.SeekCurrent:
		position = reader.position + offset
	case io.SeekEnd:
		position = reader.size + offset
	default:
		return 0, errors.New("invalid whence")
	}
	if position < 0 {
		return 0, errors.New("negative position")
	}
	reader.position = position
	return position, nil
}

type failingLocalResponseWriter struct {
	header     http.Header
	statusCode int
	writeErr   error
}

func (writer *failingLocalResponseWriter) Header() http.Header {
	if writer.header == nil {
		writer.header = make(http.Header)
	}
	return writer.header
}

func (writer *failingLocalResponseWriter) WriteHeader(statusCode int) {
	if writer.statusCode == 0 {
		writer.statusCode = statusCode
	}
}

func (writer *failingLocalResponseWriter) Write([]byte) (int, error) {
	if writer.statusCode == 0 {
		writer.WriteHeader(http.StatusOK)
	}
	return 0, writer.writeErr
}

func (reader *cancelingReadSeeker) Read(buffer []byte) (int, error) {
	if !reader.done {
		reader.done = true
		reader.cancel()
		return 0, nil
	}
	return reader.reader.Read(buffer)
}

func (reader *cancelingReadSeeker) Seek(offset int64, whence int) (int64, error) {
	return reader.reader.Seek(offset, whence)
}

func stringPointer(value string) *string {
	return &value
}

var _ io.ReadSeeker = (*cancelingReadSeeker)(nil)
var _ io.ReadSeeker = (*advertisedSizeReadSeeker)(nil)
var _ http.ResponseWriter = (*failingLocalResponseWriter)(nil)
