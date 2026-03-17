package handlers

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/gin-gonic/gin"
	configpkg "github.com/konghang/ember/backend/internal/config"
)

func TestSettingHandlerGetSettingByKeyReturnsRuntimeConfig(t *testing.T) {
	gin.SetMode(gin.TestMode)
	t.Setenv("PORT", "18080")

	handler := &SettingHandler{configService: configpkg.NewConfigService()}
	ctx, recorder := newTestConfigContext(http.MethodGet, "/api/v1/internal/settings/PORT", nil)
	ctx.Params = gin.Params{{Key: "key", Value: "PORT"}}

	handler.GetSettingByKey(ctx)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", recorder.Code)
	}

	var resp struct {
		Key      string `json:"key"`
		Value    string `json:"value"`
		Source   string `json:"source"`
		HasValue bool   `json:"hasValue"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp.Key != "PORT" {
		t.Fatalf("unexpected key: %s", resp.Key)
	}
	if resp.Value != "18080" {
		t.Fatalf("unexpected value: %s", resp.Value)
	}
	if resp.Source != configpkg.ConfigSourceEnv {
		t.Fatalf("expected env source, got %s", resp.Source)
	}
	if !resp.HasValue {
		t.Fatal("expected hasValue=true")
	}
}

func TestSettingHandlerGetConsoleAccountLinksReturnsDefaultLinks(t *testing.T) {
	gin.SetMode(gin.TestMode)

	handler := &SettingHandler{configService: configpkg.NewConfigService()}
	ctx, recorder := newTestConfigContext(http.MethodGet, "/api/v1/account-links", nil)

	handler.GetConsoleAccountLinks(ctx)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", recorder.Code)
	}

	var resp struct {
		Data []configpkg.ConsoleAccountLink `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if len(resp.Data) == 0 {
		t.Fatal("expected account links to be returned")
	}
	if resp.Data[0].Key == "" || resp.Data[0].URL == "" {
		t.Fatalf("expected first account link to contain key and url, got %+v", resp.Data[0])
	}
}

func TestSettingHandlerGetSettingByKeyRejectsSensitiveConfig(t *testing.T) {
	gin.SetMode(gin.TestMode)
	t.Setenv("TMDB_API_KEY", "super-secret")

	handler := &SettingHandler{configService: configpkg.NewConfigService()}
	ctx, recorder := newTestConfigContext(http.MethodGet, "/api/v1/internal/settings/TMDB_API_KEY", nil)
	ctx.Params = gin.Params{{Key: "key", Value: "TMDB_API_KEY"}}

	handler.GetSettingByKey(ctx)

	if recorder.Code != http.StatusForbidden {
		t.Fatalf("expected status 403, got %d", recorder.Code)
	}
}

func TestSettingHandlerGetSettingByKeyReturnsNotFoundForUnknownKey(t *testing.T) {
	gin.SetMode(gin.TestMode)

	handler := &SettingHandler{configService: configpkg.NewConfigService()}
	ctx, recorder := newTestConfigContext(http.MethodGet, "/api/v1/internal/settings/legacy_only", nil)
	ctx.Params = gin.Params{{Key: "key", Value: "legacy_only"}}

	handler.GetSettingByKey(ctx)

	if recorder.Code != http.StatusNotFound {
		t.Fatalf("expected status 404, got %d", recorder.Code)
	}
}
