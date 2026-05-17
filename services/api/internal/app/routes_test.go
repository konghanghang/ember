package app

import (
	"net/http"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/konghang/ember/backend/internal/apiroutes"
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
