package httpx

import (
	"bytes"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestInternalErrorReturnsGenericResponseAndLogsContextRequestID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	logs := captureInternalErrorLogs(t)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/v1/test", nil)
	ctx.Set("requestId", " req_ctx_1 ")

	InternalError(ctx, errors.New("upstream token=secret response body"))

	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("expected status 500, got %d", recorder.Code)
	}
	var response map[string]string
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response["error"] != "上游服务暂不可用" {
		t.Fatalf("expected generic error, got %+v", response)
	}
	if strings.Contains(recorder.Body.String(), "secret") {
		t.Fatalf("response leaked internal error: %s", recorder.Body.String())
	}
	if !strings.Contains(logs.String(), "requestId=req_ctx_1") {
		t.Fatalf("expected context requestId in logs, got %q", logs.String())
	}
}

func TestInternalErrorFallsBackToHeaderRequestID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	logs := captureInternalErrorLogs(t)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/v1/test", nil)
	ctx.Request.Header.Set("X-Request-Id", " req_header_1 ")

	InternalError(ctx, errors.New("boom"))

	if !strings.Contains(logs.String(), "requestId=req_header_1") {
		t.Fatalf("expected header requestId in logs, got %q", logs.String())
	}
}

func TestInternalErrorAcceptsNilContext(t *testing.T) {
	logs := captureInternalErrorLogs(t)

	InternalError(nil, errors.New("boom"))

	if !strings.Contains(logs.String(), "[Internal] err=boom") {
		t.Fatalf("expected error log without requestId, got %q", logs.String())
	}
}

func captureInternalErrorLogs(t *testing.T) *bytes.Buffer {
	t.Helper()

	var logs bytes.Buffer
	previousOutput := log.Writer()
	previousFlags := log.Flags()
	log.SetOutput(&logs)
	log.SetFlags(0)
	t.Cleanup(func() {
		log.SetOutput(previousOutput)
		log.SetFlags(previousFlags)
	})

	return &logs
}
