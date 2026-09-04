package handlers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/konghang/ember/backend/internal/middleware"
	"github.com/konghang/ember/backend/internal/models"
	p115accountpkg "github.com/konghang/ember/backend/internal/services/p115account"
)

type stubP115UserAccountService struct {
	getFn         func(context.Context, string) (*p115accountpkg.PersonalAccountSummary, error)
	createFn      func(context.Context, string, string) (*p115accountpkg.PersonalAccountSummary, error)
	replaceFn     func(context.Context, string, string) (*p115accountpkg.PersonalAccountSummary, error)
	validateFn    func(context.Context, string) (*p115accountpkg.PersonalValidationResult, error)
	revokeFn      func(context.Context, string) error
	directoryFn   func(context.Context, string, string) (*p115accountpkg.PersonalAccountSummary, error)
	concurrencyFn func(context.Context, string, int) (*p115accountpkg.PersonalAccountSummary, error)
	enabledFn     func(context.Context, string, bool) (*p115accountpkg.PersonalAccountSummary, error)
	usageFn       func(context.Context, string) (*p115accountpkg.PersonalUsageSummary, error)
}

func (s *stubP115UserAccountService) GetPersonalAccount(ctx context.Context, userID string) (*p115accountpkg.PersonalAccountSummary, error) {
	return s.getFn(ctx, userID)
}

func (s *stubP115UserAccountService) CreatePersonalAccount(ctx context.Context, userID, cookie string) (*p115accountpkg.PersonalAccountSummary, error) {
	return s.createFn(ctx, userID, cookie)
}

func (s *stubP115UserAccountService) ReplacePersonalCookie(ctx context.Context, userID, cookie string) (*p115accountpkg.PersonalAccountSummary, error) {
	return s.replaceFn(ctx, userID, cookie)
}

func (s *stubP115UserAccountService) ValidatePersonalAccount(ctx context.Context, userID string) (*p115accountpkg.PersonalValidationResult, error) {
	return s.validateFn(ctx, userID)
}

func (s *stubP115UserAccountService) RevokePersonalAccount(ctx context.Context, userID string) error {
	return s.revokeFn(ctx, userID)
}

func (s *stubP115UserAccountService) UpdatePersonalDirectory(ctx context.Context, userID, path string) (*p115accountpkg.PersonalAccountSummary, error) {
	return s.directoryFn(ctx, userID, path)
}

func (s *stubP115UserAccountService) UpdatePersonalConcurrency(ctx context.Context, userID string, max int) (*p115accountpkg.PersonalAccountSummary, error) {
	return s.concurrencyFn(ctx, userID, max)
}

func (s *stubP115UserAccountService) SetPersonalEnabled(ctx context.Context, userID string, enabled bool) (*p115accountpkg.PersonalAccountSummary, error) {
	return s.enabledFn(ctx, userID, enabled)
}

func (s *stubP115UserAccountService) GetPersonalUsage(ctx context.Context, userID string) (*p115accountpkg.PersonalUsageSummary, error) {
	return s.usageFn(ctx, userID)
}

func TestP115UserAccountHandlerCreatesFromExactCookieDTO(t *testing.T) {
	service := &stubP115UserAccountService{
		createFn: func(_ context.Context, userID, cookie string) (*p115accountpkg.PersonalAccountSummary, error) {
			if userID != "user-1" || cookie != "UID=100_F1_1700000000" {
				t.Fatalf("create input userID=%q cookie=%q", userID, cookie)
			}
			return &p115accountpkg.PersonalAccountSummary{ID: "personal", AppType: "android", Status: models.P115AccountStatusPending}, nil
		},
	}
	handler := &P115UserAccountHandler{service: service}
	ctx, recorder := newP115UserContext(http.MethodPost, "/api/v1/user/p115-account", []byte(`{"cookie":"UID=100_F1_1700000000"}`))

	handler.Create(ctx)

	if recorder.Code != http.StatusCreated || strings.Contains(recorder.Body.String(), "UID=100") ||
		strings.Contains(recorder.Body.String(), "userAgent") || strings.Contains(recorder.Body.String(), `"p115PlaybackMode":""`) {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestP115UserAccountHandlerRejectsPrivilegedCreateFields(t *testing.T) {
	called := false
	service := &stubP115UserAccountService{
		createFn: func(context.Context, string, string) (*p115accountpkg.PersonalAccountSummary, error) {
			called = true
			return nil, nil
		},
	}
	handler := &P115UserAccountHandler{service: service}
	ctx, recorder := newP115UserContext(http.MethodPost, "/api/v1/user/p115-account", []byte(`{"cookie":"UID=100_F1_1700000000","role":"source"}`))

	handler.Create(ctx)

	if recorder.Code != http.StatusBadRequest || called {
		t.Fatalf("status=%d called=%t body=%s", recorder.Code, called, recorder.Body.String())
	}
}

func TestP115UserAccountHandlerLifecycleDelegatesCurrentUser(t *testing.T) {
	var calls []string
	service := &stubP115UserAccountService{
		getFn: func(_ context.Context, userID string) (*p115accountpkg.PersonalAccountSummary, error) {
			calls = append(calls, "get:"+userID)
			return &p115accountpkg.PersonalAccountSummary{ID: "personal"}, nil
		},
		replaceFn: func(_ context.Context, userID, cookie string) (*p115accountpkg.PersonalAccountSummary, error) {
			calls = append(calls, "replace:"+userID+":"+cookie)
			return &p115accountpkg.PersonalAccountSummary{ID: "personal"}, nil
		},
		validateFn: func(_ context.Context, userID string) (*p115accountpkg.PersonalValidationResult, error) {
			calls = append(calls, "validate:"+userID)
			return &p115accountpkg.PersonalValidationResult{Valid: true, Account: &p115accountpkg.PersonalAccountSummary{ID: "personal"}}, nil
		},
		revokeFn: func(_ context.Context, userID string) error {
			calls = append(calls, "revoke:"+userID)
			return nil
		},
		usageFn: func(_ context.Context, userID string) (*p115accountpkg.PersonalUsageSummary, error) {
			calls = append(calls, "usage:"+userID)
			return &p115accountpkg.PersonalUsageSummary{UsageAvailable: true}, nil
		},
	}
	handler := &P115UserAccountHandler{service: service}

	getCtx, getRecorder := newP115UserContext(http.MethodGet, "/api/v1/user/p115-account", nil)
	handler.Get(getCtx)
	usageCtx, usageRecorder := newP115UserContext(http.MethodGet, "/api/v1/user/p115-usage", nil)
	handler.GetUsage(usageCtx)
	replaceCtx, replaceRecorder := newP115UserContext(http.MethodPut, "/api/v1/user/p115-account/cookie", []byte(`{"cookie":"UID=200_F1_1700000000"}`))
	handler.ReplaceCookie(replaceCtx)
	validateCtx, validateRecorder := newP115UserContext(http.MethodPost, "/api/v1/user/p115-account/validate", nil)
	handler.Validate(validateCtx)
	revokeCtx, revokeRecorder := newP115UserContext(http.MethodDelete, "/api/v1/user/p115-account", nil)
	handler.Revoke(revokeCtx)

	for _, recorder := range []int{getRecorder.Code, usageRecorder.Code, replaceRecorder.Code, validateRecorder.Code, revokeRecorder.Code} {
		if recorder != http.StatusOK {
			t.Fatalf("unexpected status sequence get=%d replace=%d validate=%d revoke=%d", getRecorder.Code, replaceRecorder.Code, validateRecorder.Code, revokeRecorder.Code)
		}
	}
	want := "get:user-1,usage:user-1,replace:user-1:UID=200_F1_1700000000,validate:user-1,revoke:user-1"
	if strings.Join(calls, ",") != want {
		t.Fatalf("calls=%q want=%q", strings.Join(calls, ","), want)
	}
}

func TestP115UserAccountHandlerConfiguresDirectoryConcurrencyAndEnabled(t *testing.T) {
	var calls []string
	service := &stubP115UserAccountService{
		directoryFn: func(_ context.Context, userID, path string) (*p115accountpkg.PersonalAccountSummary, error) {
			calls = append(calls, "directory:"+userID+":"+path)
			return &p115accountpkg.PersonalAccountSummary{ID: "personal"}, nil
		},
		concurrencyFn: func(_ context.Context, userID string, max int) (*p115accountpkg.PersonalAccountSummary, error) {
			calls = append(calls, "concurrency:"+userID+":"+strconv.Itoa(max))
			return &p115accountpkg.PersonalAccountSummary{ID: "personal"}, nil
		},
		enabledFn: func(_ context.Context, userID string, enabled bool) (*p115accountpkg.PersonalAccountSummary, error) {
			calls = append(calls, "enabled:"+userID+":"+strconv.FormatBool(enabled))
			return &p115accountpkg.PersonalAccountSummary{ID: "personal", Enabled: enabled}, nil
		},
	}
	handler := &P115UserAccountHandler{service: service}

	directoryCtx, directoryRecorder := newP115UserContext(http.MethodPut, "/api/v1/user/p115-account/directory", []byte(`{"targetParentPath":"/Playback"}`))
	handler.UpdateDirectory(directoryCtx)
	concurrencyCtx, concurrencyRecorder := newP115UserContext(http.MethodPut, "/api/v1/user/p115-account/concurrency", []byte(`{"maxConcurrentStreams":3}`))
	handler.UpdateConcurrency(concurrencyCtx)
	enabledCtx, enabledRecorder := newP115UserContext(http.MethodPut, "/api/v1/user/p115-account/enabled", []byte(`{"enabled":false}`))
	handler.SetEnabled(enabledCtx)

	if directoryRecorder.Code != http.StatusOK || concurrencyRecorder.Code != http.StatusOK || enabledRecorder.Code != http.StatusOK {
		t.Fatalf("statuses directory=%d concurrency=%d enabled=%d", directoryRecorder.Code, concurrencyRecorder.Code, enabledRecorder.Code)
	}
	want := "directory:user-1:/Playback,concurrency:user-1:3,enabled:user-1:false"
	if strings.Join(calls, ",") != want {
		t.Fatalf("calls=%q want=%q", strings.Join(calls, ","), want)
	}
}

func TestP115UserAccountHandlerRejectsPartialConfigurationDTOs(t *testing.T) {
	service := &stubP115UserAccountService{
		directoryFn: func(context.Context, string, string) (*p115accountpkg.PersonalAccountSummary, error) {
			t.Fatal("directory service called")
			return nil, nil
		},
		concurrencyFn: func(context.Context, string, int) (*p115accountpkg.PersonalAccountSummary, error) {
			t.Fatal("concurrency service called")
			return nil, nil
		},
		enabledFn: func(context.Context, string, bool) (*p115accountpkg.PersonalAccountSummary, error) {
			t.Fatal("enabled service called")
			return nil, nil
		},
	}
	handler := &P115UserAccountHandler{service: service}

	for _, call := range []struct {
		name string
		path string
		body string
		run  func(*gin.Context)
	}{
		{name: "directory", path: "/directory", body: `{}`, run: handler.UpdateDirectory},
		{name: "concurrency", path: "/concurrency", body: `{}`, run: handler.UpdateConcurrency},
		{name: "enabled", path: "/enabled", body: `{}`, run: handler.SetEnabled},
	} {
		t.Run(call.name, func(t *testing.T) {
			ctx, recorder := newP115UserContext(http.MethodPut, "/api/v1/user/p115-account"+call.path, []byte(call.body))
			call.run(ctx)
			if recorder.Code != http.StatusBadRequest {
				t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
			}
		})
	}
}

func newP115UserContext(method, target string, body []byte) (*gin.Context, *httptest.ResponseRecorder) {
	ctx, recorder := newTestConfigContext(method, target, body)
	ctx.Set("principal", middleware.AuthPrincipal{UserID: "user-1", Role: "user", IsActive: true})
	return ctx, recorder
}
