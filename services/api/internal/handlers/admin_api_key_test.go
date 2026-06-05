package handlers

import (
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/konghang/ember/backend/internal/services/accessauth"
)

type stubAdminAPIKeyService struct {
	statusFn   func() (*accessauth.AdminAPIKeyStatus, error)
	generateFn func(updatedByUserID string) (*accessauth.GeneratedAdminAPIKey, error)
	disableFn  func(updatedByUserID string) (*accessauth.AdminAPIKeyStatus, error)
}

func (s *stubAdminAPIKeyService) Status() (*accessauth.AdminAPIKeyStatus, error) {
	return s.statusFn()
}

func (s *stubAdminAPIKeyService) Generate(updatedByUserID string) (*accessauth.GeneratedAdminAPIKey, error) {
	return s.generateFn(updatedByUserID)
}

func (s *stubAdminAPIKeyService) Disable(updatedByUserID string) (*accessauth.AdminAPIKeyStatus, error) {
	return s.disableFn(updatedByUserID)
}

func TestAdminAPIKeyHandler(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("returns status", func(t *testing.T) {
		handler := &AdminAPIKeyHandler{
			service: &stubAdminAPIKeyService{
				statusFn: func() (*accessauth.AdminAPIKeyStatus, error) {
					return &accessauth.AdminAPIKeyStatus{Configured: true}, nil
				},
			},
		}

		ctx, recorder := newTestConfigContext(http.MethodGet, "/api/v1/admin/external-api-key", nil)
		handler.GetStatus(ctx)

		if recorder.Code != http.StatusOK || !jsonContains(recorder.Body.String(), `"configured":true`) {
			t.Fatalf("expected configured status response, got status=%d body=%s", recorder.Code, recorder.Body.String())
		}
	})

	t.Run("generates key with jwt admin user id", func(t *testing.T) {
		handler := &AdminAPIKeyHandler{
			service: &stubAdminAPIKeyService{
				generateFn: func(updatedByUserID string) (*accessauth.GeneratedAdminAPIKey, error) {
					if updatedByUserID != "admin_1" {
						t.Fatalf("unexpected updatedByUserID: %s", updatedByUserID)
					}
					return &accessauth.GeneratedAdminAPIKey{Configured: true, APIKey: "ember_sk_plain_once"}, nil
				},
			},
		}

		ctx, recorder := newTestConfigContext(http.MethodPost, "/api/v1/admin/external-api-key", nil)
		ctx.Set("userID", "admin_1")
		handler.Generate(ctx)

		if recorder.Code != http.StatusOK || !jsonContains(recorder.Body.String(), `"apiKey":"ember_sk_plain_once"`) {
			t.Fatalf("expected one-time plain api key response, got status=%d body=%s", recorder.Code, recorder.Body.String())
		}
	})

	t.Run("disables key", func(t *testing.T) {
		handler := &AdminAPIKeyHandler{
			service: &stubAdminAPIKeyService{
				disableFn: func(updatedByUserID string) (*accessauth.AdminAPIKeyStatus, error) {
					if updatedByUserID != "admin_2" {
						t.Fatalf("unexpected updatedByUserID: %s", updatedByUserID)
					}
					return &accessauth.AdminAPIKeyStatus{Configured: false}, nil
				},
			},
		}

		ctx, recorder := newTestConfigContext(http.MethodDelete, "/api/v1/admin/external-api-key", nil)
		ctx.Set("userID", "admin_2")
		handler.Disable(ctx)

		if recorder.Code != http.StatusOK || !jsonContains(recorder.Body.String(), `"configured":false`) {
			t.Fatalf("expected disabled response, got status=%d body=%s", recorder.Code, recorder.Body.String())
		}
	})

	t.Run("rejects api key managing itself", func(t *testing.T) {
		handler := &AdminAPIKeyHandler{
			service: &stubAdminAPIKeyService{
				statusFn: func() (*accessauth.AdminAPIKeyStatus, error) {
					t.Fatalf("service should not be called for api key management")
					return nil, nil
				},
			},
		}

		ctx, recorder := newTestConfigContext(http.MethodGet, "/api/v1/admin/external-api-key", nil)
		ctx.Set("authType", "api_key")
		handler.GetStatus(ctx)

		if recorder.Code != http.StatusForbidden {
			t.Fatalf("expected api key self-management to be forbidden, got status=%d body=%s", recorder.Code, recorder.Body.String())
		}
	})

	t.Run("maps service errors to internal error", func(t *testing.T) {
		handler := &AdminAPIKeyHandler{
			service: &stubAdminAPIKeyService{
				statusFn: func() (*accessauth.AdminAPIKeyStatus, error) {
					return nil, errors.New("store unavailable")
				},
			},
		}

		ctx, recorder := newTestConfigContext(http.MethodGet, "/api/v1/admin/external-api-key", nil)
		handler.GetStatus(ctx)

		if recorder.Code != http.StatusInternalServerError {
			t.Fatalf("expected internal error, got status=%d body=%s", recorder.Code, recorder.Body.String())
		}
	})
}

func jsonContains(raw string, needle string) bool {
	return strings.Contains(strings.ReplaceAll(raw, " ", ""), needle)
}
