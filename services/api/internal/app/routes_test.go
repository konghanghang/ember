package app

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/konghang/ember/backend/internal/apiroutes"
	"github.com/konghang/ember/backend/internal/common"
	apihandlers "github.com/konghang/ember/backend/internal/handlers"
)

func TestPasswordResetClosedLoopRoutesAreRegistered(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router := gin.New()
	registerRoutes(router, &appHandlers{})

	registered := make(map[string]struct{})
	for _, route := range router.Routes() {
		registered[route.Method+" "+route.Path] = struct{}{}
	}

	expected := map[string]string{
		apiroutes.FullProfilePath:      http.MethodGet,
		apiroutes.FullPasswordPath:     http.MethodPut,
		apiroutes.FullAccountLinksPath: http.MethodGet,
		apiroutes.FullAdminCurrentPath: http.MethodGet,
		apiroutes.FullUserProfilePath:  http.MethodGet,
		apiroutes.FullUserPasswordPath: http.MethodPut,
	}

	for _, path := range apiroutes.PasswordResetClosedLoopPaths() {
		method, ok := expected[path]
		if !ok {
			t.Fatalf("closed loop path %s is missing expected method coverage", path)
		}
		if _, ok := registered[method+" "+path]; !ok {
			t.Fatalf("closed loop route is not registered: %s %s", method, path)
		}
	}
}

func TestTMDBProxyRoutesRemainSeparatedByAuthBoundary(t *testing.T) {
	gin.SetMode(gin.TestMode)
	t.Setenv("INTERNAL_API_SECRET", "0123456789abcdef0123456789abcdef")
	t.Setenv("TMDB_API_KEY", "")
	if err := common.InitInternalAPISecret(); err != nil {
		t.Fatalf("InitInternalAPISecret() error = %v", err)
	}

	router := gin.New()
	registerRoutes(router, &appHandlers{
		tmdb: apihandlers.NewTMDBHandler(),
	})

	authenticatedRoutes := []string{
		"/api/v1/tmdb/search?query=inception&type=movie",
		"/api/v1/tmdb/tv/1399/seasons",
	}
	internalRoutes := []string{
		"/api/v1/internal/tmdb/search?query=inception&type=movie",
		"/api/v1/internal/tmdb/tv/1399/seasons",
	}

	for _, target := range authenticatedRoutes {
		req := httptest.NewRequest(http.MethodGet, target, nil)
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, req)
		if recorder.Code != http.StatusUnauthorized {
			t.Fatalf("expected authenticated TMDB proxy to reject missing bearer token: target=%s status=%d body=%s", target, recorder.Code, recorder.Body.String())
		}
	}

	for _, target := range internalRoutes {
		req := httptest.NewRequest(http.MethodGet, target, nil)
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, req)
		if recorder.Code != http.StatusUnauthorized {
			t.Fatalf("expected internal TMDB proxy to reject missing internal secret: target=%s status=%d body=%s", target, recorder.Code, recorder.Body.String())
		}

		reqWithSecret := httptest.NewRequest(http.MethodGet, target, nil)
		reqWithSecret.Header.Set("X-Internal-Secret", "0123456789abcdef0123456789abcdef")
		recorderWithSecret := httptest.NewRecorder()
		router.ServeHTTP(recorderWithSecret, reqWithSecret)
		if recorderWithSecret.Code == http.StatusUnauthorized {
			t.Fatalf("expected internal TMDB proxy with secret to bypass JWT auth: target=%s status=%d body=%s", target, recorderWithSecret.Code, recorderWithSecret.Body.String())
		}
	}
}

func TestP115AccountAdminRoutesAreRegistered(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	registerRoutes(router, &appHandlers{})

	registered := make(map[string]struct{})
	for _, route := range router.Routes() {
		registered[route.Method+" "+route.Path] = struct{}{}
	}

	expected := map[string]string{
		"/api/v1/admin/p115-accounts":              http.MethodGet,
		"/api/v1/admin/p115-accounts/:id":          http.MethodGet,
		"/api/v1/admin/p115-accounts/:id/cookie":   http.MethodPut,
		"/api/v1/admin/p115-accounts/:id/validate": http.MethodPost,
		"/api/v1/admin/p115-accounts/:id/enabled":  http.MethodPut,
	}
	for path, method := range expected {
		if _, ok := registered[method+" "+path]; !ok {
			t.Fatalf("p115 account route is not registered: %s %s", method, path)
		}
	}
	if _, ok := registered[http.MethodPost+" /api/v1/admin/p115-accounts"]; !ok {
		t.Fatal("p115 account create route is not registered")
	}
}
