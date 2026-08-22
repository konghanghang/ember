package handlers

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	p115integration "github.com/konghang/ember/backend/internal/integrations/p115"
	"github.com/konghang/ember/backend/internal/models"
	p115accountpkg "github.com/konghang/ember/backend/internal/services/p115account"
)

type stubP115AccountService struct {
	listFn                 func(context.Context) ([]p115accountpkg.AccountSummary, error)
	getFn                  func(context.Context, string) (*p115accountpkg.AccountSummary, error)
	createFn               func(context.Context, p115accountpkg.CreateAccountInput) (*p115accountpkg.AccountSummary, error)
	replaceFn              func(context.Context, string, string) (*p115accountpkg.AccountSummary, error)
	validateFn             func(context.Context, string) (*p115accountpkg.ValidationResult, error)
	setEnabledFn           func(context.Context, string, bool) (*p115accountpkg.AccountSummary, error)
	updateSourceLocationFn func(context.Context, string, p115accountpkg.SourceLocationInput) (*p115accountpkg.AccountSummary, error)
}

func (s *stubP115AccountService) List(ctx context.Context) ([]p115accountpkg.AccountSummary, error) {
	return s.listFn(ctx)
}

func (s *stubP115AccountService) Get(ctx context.Context, id string) (*p115accountpkg.AccountSummary, error) {
	return s.getFn(ctx, id)
}

func (s *stubP115AccountService) Create(ctx context.Context, input p115accountpkg.CreateAccountInput) (*p115accountpkg.AccountSummary, error) {
	return s.createFn(ctx, input)
}

func (s *stubP115AccountService) ReplaceCookie(ctx context.Context, id, cookie string) (*p115accountpkg.AccountSummary, error) {
	return s.replaceFn(ctx, id, cookie)
}

func (s *stubP115AccountService) Validate(ctx context.Context, id string) (*p115accountpkg.ValidationResult, error) {
	return s.validateFn(ctx, id)
}

func (s *stubP115AccountService) SetEnabled(ctx context.Context, id string, enabled bool) (*p115accountpkg.AccountSummary, error) {
	return s.setEnabledFn(ctx, id, enabled)
}

func (s *stubP115AccountService) UpdateSourceLocation(ctx context.Context, id string, input p115accountpkg.SourceLocationInput) (*p115accountpkg.AccountSummary, error) {
	return s.updateSourceLocationFn(ctx, id, input)
}

func TestP115AccountHandlerListUsesDataField(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler := &P115AccountHandler{service: &stubP115AccountService{
		listFn: func(context.Context) ([]p115accountpkg.AccountSummary, error) {
			return []p115accountpkg.AccountSummary{{ID: "account_1", Role: models.P115AccountRoleSource}}, nil
		},
	}}

	ctx, recorder := newTestConfigContext(http.MethodGet, "/api/v1/admin/p115-accounts", nil)
	handler.List(ctx)
	if recorder.Code != http.StatusOK || !jsonContains(recorder.Body.String(), `"data":[{"id":"account_1"`) {
		t.Fatalf("unexpected response: status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestP115AccountHandlerCreateBindsWriteOnlyCookie(t *testing.T) {
	handler := &P115AccountHandler{service: &stubP115AccountService{
		createFn: func(_ context.Context, input p115accountpkg.CreateAccountInput) (*p115accountpkg.AccountSummary, error) {
			if input.Cookie != "UID=fake" || input.Role != models.P115AccountRolePlayback || input.TargetParentID != "target" {
				t.Fatalf("unexpected create input: %+v", input)
			}
			return &p115accountpkg.AccountSummary{ID: "account_1", Role: input.Role}, nil
		},
	}}
	body := []byte(`{"role":"playback","alias":"playback","cookie":"UID=fake","appType":"android","userAgent":"agent","targetParentId":"target"}`)
	ctx, recorder := newTestConfigContext(http.MethodPost, "/api/v1/admin/p115-accounts", body)
	handler.Create(ctx)
	if recorder.Code != http.StatusCreated || strings.Contains(recorder.Body.String(), "UID=fake") {
		t.Fatalf("unexpected response: status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestP115AccountHandlerCreateBindsSourceLocation(t *testing.T) {
	handler := &P115AccountHandler{service: &stubP115AccountService{
		createFn: func(_ context.Context, input p115accountpkg.CreateAccountInput) (*p115accountpkg.AccountSummary, error) {
			if input.Role != models.P115AccountRoleSource || input.EmbyPathPrefix != "/mnt/cloudNAS/115lifetime" || input.SourceRootID != "0" {
				t.Fatalf("unexpected source create input: %+v", input)
			}
			return &p115accountpkg.AccountSummary{ID: "source_1", Role: input.Role}, nil
		},
	}}
	body := []byte(`{"role":"source","alias":"source","cookie":"UID=fake","appType":"ios","userAgent":"agent","embyPathPrefix":"/mnt/cloudNAS/115lifetime","sourceRootId":"0"}`)
	ctx, recorder := newTestConfigContext(http.MethodPost, "/api/v1/admin/p115-accounts", body)
	handler.Create(ctx)
	if recorder.Code != http.StatusCreated || strings.Contains(recorder.Body.String(), "UID=fake") {
		t.Fatalf("unexpected response: status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestP115AccountHandlerUpdatesSourceLocation(t *testing.T) {
	handler := &P115AccountHandler{service: &stubP115AccountService{
		updateSourceLocationFn: func(_ context.Context, id string, input p115accountpkg.SourceLocationInput) (*p115accountpkg.AccountSummary, error) {
			if id != "source_1" || input.EmbyPathPrefix != "/mnt/cloudNAS/115lifetime" || input.SourceRootID != "0" {
				t.Fatalf("unexpected source location input: id=%q input=%+v", id, input)
			}
			return &p115accountpkg.AccountSummary{ID: id, Role: models.P115AccountRoleSource}, nil
		},
	}}
	ctx, recorder := newTestConfigContext(http.MethodPut, "/api/v1/admin/p115-accounts/source_1/source-location",
		[]byte(`{"embyPathPrefix":"/mnt/cloudNAS/115lifetime","sourceRootId":"0"}`))
	ctx.Params = gin.Params{{Key: "id", Value: "source_1"}}
	handler.UpdateSourceLocation(ctx)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s, want 200", recorder.Code, recorder.Body.String())
	}
}

func TestP115AccountHandlerValidateMapsProviderFailureSafely(t *testing.T) {
	handler := &P115AccountHandler{service: &stubP115AccountService{
		validateFn: func(context.Context, string) (*p115accountpkg.ValidationResult, error) {
			return nil, p115integration.ErrProviderUnavailable
		},
	}}
	ctx, recorder := newTestConfigContext(http.MethodPost, "/api/v1/admin/p115-accounts/account_1/validate", nil)
	ctx.Params = gin.Params{{Key: "id", Value: "account_1"}}
	handler.Validate(ctx)
	if recorder.Code != http.StatusBadGateway || !strings.Contains(recorder.Body.String(), `"error":"115 服务暂不可用"`) {
		t.Fatalf("unexpected response: status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestP115AccountHandlerMapsDomainErrors(t *testing.T) {
	tests := []struct {
		name       string
		err        error
		statusCode int
	}{
		{name: "not found", err: p115accountpkg.ErrAccountNotFound, statusCode: http.StatusNotFound},
		{name: "invalid input", err: p115accountpkg.ErrCookieRequired, statusCode: http.StatusBadRequest},
		{name: "state conflict", err: p115accountpkg.ErrAccountUnavailable, statusCode: http.StatusConflict},
		{name: "credential changed", err: p115accountpkg.ErrCredentialChanged, statusCode: http.StatusConflict},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := &P115AccountHandler{service: &stubP115AccountService{
				getFn: func(context.Context, string) (*p115accountpkg.AccountSummary, error) { return nil, tt.err },
			}}
			ctx, recorder := newTestConfigContext(http.MethodGet, "/api/v1/admin/p115-accounts/account_1", nil)
			ctx.Params = gin.Params{{Key: "id", Value: "account_1"}}
			handler.Get(ctx)
			if recorder.Code != tt.statusCode {
				t.Fatalf("status=%d body=%s, want %d", recorder.Code, recorder.Body.String(), tt.statusCode)
			}
		})
	}
}

func TestP115AccountHandlerRejectsAdminAPIKey(t *testing.T) {
	handler := &P115AccountHandler{service: &stubP115AccountService{
		listFn: func(context.Context) ([]p115accountpkg.AccountSummary, error) {
			t.Fatal("service must not be called")
			return nil, errors.New("unreachable")
		},
	}}
	ctx, recorder := newTestConfigContext(http.MethodGet, "/api/v1/admin/p115-accounts", nil)
	ctx.Set("authType", "api_key")
	handler.List(ctx)
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("status=%d body=%s, want 403", recorder.Code, recorder.Body.String())
	}
}

func TestP115AccountHandlerAllowsExplicitDisable(t *testing.T) {
	handler := &P115AccountHandler{service: &stubP115AccountService{
		setEnabledFn: func(_ context.Context, id string, enabled bool) (*p115accountpkg.AccountSummary, error) {
			if id != "account_1" || enabled {
				t.Fatalf("unexpected SetEnabled input: id=%q enabled=%t", id, enabled)
			}
			return &p115accountpkg.AccountSummary{ID: id, Enabled: false}, nil
		},
	}}
	ctx, recorder := newTestConfigContext(http.MethodPut, "/api/v1/admin/p115-accounts/account_1/enabled", []byte(`{"enabled":false}`))
	ctx.Params = gin.Params{{Key: "id", Value: "account_1"}}
	handler.SetEnabled(ctx)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s, want 200", recorder.Code, recorder.Body.String())
	}
}
