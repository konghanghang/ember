package handlers

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/konghang/ember/backend/internal/common/httpx"
	"github.com/konghang/ember/backend/internal/models"
	redemptionpkg "github.com/konghang/ember/backend/internal/services/redemption"
	userpkg "github.com/konghang/ember/backend/internal/services/user"
)

func newTestUserHandlerContext(method, target string, body []byte) (*gin.Context, *httptest.ResponseRecorder) {
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(method, target, bytes.NewReader(body))
	if body != nil {
		ctx.Request.Header.Set("Content-Type", "application/json")
	}
	return ctx, recorder
}

func newTestUserHandler(service *userpkg.UserService) *UserHandler {
	return &UserHandler{
		userService:           service,
		redemptionService:     &redemptionpkg.RedemptionService{},
		redemptionCodeService: &redemptionpkg.RedemptionCodeService{},
	}
}

func assertResponseError(t *testing.T, recorder *httptest.ResponseRecorder, want string) {
	t.Helper()
	var resp struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp.Error != want {
		t.Fatalf("expected error %q, got %q", want, resp.Error)
	}
}

func TestUserHandlerAdminMutationsReturn404WhenUserMissing(t *testing.T) {
	gin.SetMode(gin.TestMode)

	service := userpkg.NewUserServiceWithDeps(userpkg.UserServiceDeps{
		FindUserByID: func(userID string) (*models.User, error) {
			return nil, userpkg.ErrUserNotFound
		},
	})
	handler := newTestUserHandler(service)

	testCases := []struct {
		name   string
		method string
		target string
		body   []byte
		run    func(*UserHandler, *gin.Context)
	}{
		{
			name:   "extend expiry",
			method: http.MethodPut,
			target: "/api/v1/admin/users/user_404/extend",
			body:   []byte(`{"days":7}`),
			run:    func(h *UserHandler, c *gin.Context) { h.ExtendExpiry(c) },
		},
		{
			name:   "toggle user status",
			method: http.MethodPut,
			target: "/api/v1/admin/users/user_404/toggle",
			run:    func(h *UserHandler, c *gin.Context) { h.ToggleUserStatus(c) },
		},
		{
			name:   "delete user",
			method: http.MethodDelete,
			target: "/api/v1/admin/users/user_404",
			run:    func(h *UserHandler, c *gin.Context) { h.DeleteUser(c) },
		},
		{
			name:   "reset password",
			method: http.MethodPut,
			target: "/api/v1/admin/users/user_404/reset-password",
			body:   []byte(`{"newPassword":"newpass123"}`),
			run:    func(h *UserHandler, c *gin.Context) { h.ResetPassword(c) },
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctx, recorder := newTestUserHandlerContext(tc.method, tc.target, tc.body)
			ctx.Params = gin.Params{{Key: "id", Value: "user_404"}}

			tc.run(handler, ctx)

			if recorder.Code != http.StatusNotFound {
				t.Fatalf("expected status 404, got %d", recorder.Code)
			}
			assertResponseError(t, recorder, userpkg.ErrUserNotFound.Error())
		})
	}
}

func TestUserHandlerAdminMutationsKeepInternalErrorsAs500(t *testing.T) {
	gin.SetMode(gin.TestMode)

	service := userpkg.NewUserServiceWithDeps(userpkg.UserServiceDeps{
		FindUserByID: func(userID string) (*models.User, error) {
			return nil, errors.New("db unavailable")
		},
	})
	handler := newTestUserHandler(service)

	ctx, recorder := newTestUserHandlerContext(http.MethodPut, "/api/v1/admin/users/user_500/reset-password", []byte(`{"newPassword":"newpass123"}`))
	ctx.Params = gin.Params{{Key: "id", Value: "user_500"}}

	handler.ResetPassword(ctx)

	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("expected status 500, got %d", recorder.Code)
	}
	assertResponseError(t, recorder, "上游服务暂不可用")
}

func TestInternalErrorHelperStillUsesGenericMessage(t *testing.T) {
	gin.SetMode(gin.TestMode)

	ctx, recorder := newTestUserHandlerContext(http.MethodGet, "/internal-error", nil)
	httpx.InternalError(ctx, errors.New("boom"))

	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("expected status 500, got %d", recorder.Code)
	}
	assertResponseError(t, recorder, "上游服务暂不可用")
}
