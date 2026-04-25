package upstream

import (
	"errors"
	"fmt"
	"net/url"
	"strings"
	"testing"
)

func TestSafeUpstreamErrorStripsURLWithAPIKey(t *testing.T) {
	urlErr := &url.Error{
		Op:  "Get",
		URL: "https://api.themoviedb.org/3/tv/123?api_key=secret-key&language=zh-CN",
		Err: errors.New("EOF"),
	}

	wrapped := fmt.Errorf("请求 TMDB 失败: %w", urlErr)

	safe := SafeUpstreamError(wrapped, "tmdb")
	if safe == nil {
		t.Fatalf("expected non-nil safe error")
	}

	msg := safe.Error()
	if strings.Contains(msg, "api_key") || strings.Contains(msg, "secret-key") {
		t.Fatalf("safe error must not contain api_key or secret, got %q", msg)
	}
	if !strings.Contains(msg, "tmdb") {
		t.Fatalf("safe error should mention system, got %q", msg)
	}
	if !strings.Contains(msg, "op=Get") {
		t.Fatalf("safe error should preserve op, got %q", msg)
	}
}

func TestSafeUpstreamErrorPlainError(t *testing.T) {
	safe := SafeUpstreamError(errors.New("boom"), "moviepilot")
	if safe == nil {
		t.Fatalf("expected non-nil safe error")
	}
	msg := safe.Error()
	if !strings.Contains(msg, "moviepilot") || !strings.Contains(msg, "unavailable") {
		t.Fatalf("expected fallback message, got %q", msg)
	}
}

func TestSafeUpstreamErrorNilPassthrough(t *testing.T) {
	if got := SafeUpstreamError(nil, "tmdb"); got != nil {
		t.Fatalf("expected nil for nil input, got %v", got)
	}
}

func TestSafeUpstreamHTTPError(t *testing.T) {
	err := SafeUpstreamHTTPError("tmdb", 503)
	msg := err.Error()
	if !strings.Contains(msg, "tmdb") || !strings.Contains(msg, "503") {
		t.Fatalf("expected system + status in message, got %q", msg)
	}
	if strings.Contains(msg, "api_key") {
		t.Fatalf("must not contain api_key, got %q", msg)
	}
}
