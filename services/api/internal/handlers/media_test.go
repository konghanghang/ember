package handlers

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/gin-gonic/gin"
)

// 关键约定：emby 未配置时三个 dashboard 接口必须返回 200 + configured:false，而不是 500。
// 这是首启场景下避免前端 toast 风暴的协议契约（参见 docs/reference/api-response-standard.md）。

func TestMediaHandlerGetEmbyConfigReturnsUnconfiguredStateWhenEmbyMissing(t *testing.T) {
	gin.SetMode(gin.TestMode)

	handler := &MediaHandler{
		isEmbyConfigured: func() bool { return false },
	}

	ctx, recorder := newTestConfigContext(http.MethodGet, "/api/v1/emby/config", nil)
	handler.GetEmbyConfig(ctx)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", recorder.Code)
	}

	var resp struct {
		Success    bool   `json:"success"`
		Configured bool   `json:"configured"`
		URL        string `json:"url"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if !resp.Success || resp.Configured || resp.URL != "" {
		t.Fatalf("expected unconfigured state, got %+v", resp)
	}
}

func TestMediaHandlerGetMediaStatsReturnsUnconfiguredStateWhenEmbyMissing(t *testing.T) {
	gin.SetMode(gin.TestMode)

	handler := &MediaHandler{
		isEmbyConfigured: func() bool { return false },
	}

	ctx, recorder := newTestConfigContext(http.MethodGet, "/api/v1/media/stats", nil)
	handler.GetMediaStats(ctx)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", recorder.Code)
	}

	var resp struct {
		Success    bool        `json:"success"`
		Configured bool        `json:"configured"`
		Data       interface{} `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if !resp.Success || resp.Configured || resp.Data != nil {
		t.Fatalf("expected unconfigured state with nil data, got %+v", resp)
	}
}

func TestMediaHandlerGetLatestItemsReturnsUnconfiguredStateWhenEmbyMissing(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// isEmbyConfigured 返回 false 时 handler 必须在触达 db 前提早返回，
	// 这样测试就不需要真实数据库；同时也确认了 dashboard 首屏调用不会因 db 缺失而走 5xx。
	handler := &MediaHandler{
		isEmbyConfigured: func() bool { return false },
	}

	ctx, recorder := newTestConfigContext(http.MethodGet, "/api/v1/media/latest?type=Movie&limit=20", nil)
	handler.GetLatestItems(ctx)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", recorder.Code)
	}

	var resp struct {
		Success    bool              `json:"success"`
		Configured bool              `json:"configured"`
		Bound      bool              `json:"bound"`
		Data       []LatestMediaItem `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if !resp.Success || resp.Configured || resp.Bound {
		t.Fatalf("expected configured=false bound=false, got %+v", resp)
	}
	if resp.Data == nil {
		t.Fatalf("expected non-nil empty slice in data, got nil")
	}
	if len(resp.Data) != 0 {
		t.Fatalf("expected empty data, got %d items", len(resp.Data))
	}
}

func TestGetCurrentUserSoftRejectsNonStringUserID(t *testing.T) {
	gin.SetMode(gin.TestMode)

	ctx, recorder := newTestConfigContext(http.MethodGet, "/api/v1/media/latest", nil)
	ctx.Set("userID", 123)

	user, ok := getCurrentUserSoft(ctx)
	if ok || user != nil {
		t.Fatalf("expected non-string userID to be rejected, got ok=%v user=%+v", ok, user)
	}
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("expected status 401, got %d", recorder.Code)
	}

	var resp struct {
		Success bool   `json:"success"`
		Error   string `json:"error"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp.Success || resp.Error != "未认证" {
		t.Fatalf("expected unauthenticated response, got %+v", resp)
	}
}
