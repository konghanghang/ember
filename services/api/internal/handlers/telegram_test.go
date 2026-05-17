package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/konghang/ember/backend/internal/models"
	subscriptionpkg "github.com/konghang/ember/backend/internal/services/subscription"
	telegrampkg "github.com/konghang/ember/backend/internal/services/telegram"
)

type stubTelegramService struct {
	generateBindCodeFn    func(userID string) (string, time.Time, error)
	unbindFn              func(userID string) error
	verifyBindFn          func(telegramID int64, code string) (*telegrampkg.BindResult, error)
	subscribeByTelegramFn func(req telegrampkg.TelegramSubscribeRequest) error
}

func (s *stubTelegramService) GenerateBindCode(userID string) (string, time.Time, error) {
	if s.generateBindCodeFn == nil {
		return "", time.Time{}, nil
	}
	return s.generateBindCodeFn(userID)
}

func (s *stubTelegramService) Unbind(userID string) error {
	if s.unbindFn == nil {
		return nil
	}
	return s.unbindFn(userID)
}

func (s *stubTelegramService) VerifyBind(telegramID int64, code string) (*telegrampkg.BindResult, error) {
	if s.verifyBindFn == nil {
		return nil, nil
	}
	return s.verifyBindFn(telegramID, code)
}

func (s *stubTelegramService) GetAccountInfo(telegramID int64) (*telegrampkg.AccountInfoResponse, error) {
	return nil, nil
}

func (s *stubTelegramService) RedeemByTelegram(telegramID int64, code string) (*telegrampkg.TelegramRedeemResponse, error) {
	return nil, nil
}

func (s *stubTelegramService) ResetPassword(telegramID int64, newPassword string) error {
	return nil
}

func (s *stubTelegramService) SubscribeByTelegram(req telegrampkg.TelegramSubscribeRequest) error {
	if s.subscribeByTelegramFn == nil {
		return nil
	}
	return s.subscribeByTelegramFn(req)
}

func newTestTelegramContext(method, target string, body []byte) (*gin.Context, *httptest.ResponseRecorder) {
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(method, target, bytes.NewReader(body))
	if body != nil {
		ctx.Request.Header.Set("Content-Type", "application/json")
	}
	return ctx, recorder
}

func TestTelegramHandlerGenerateBindCodeMapsErrors(t *testing.T) {
	gin.SetMode(gin.TestMode)

	testCases := []struct {
		name       string
		err        error
		statusCode int
		wantError  string
	}{
		{name: "already bound", err: telegrampkg.ErrUserAlreadyBoundTelegram, statusCode: http.StatusBadRequest, wantError: telegrampkg.ErrUserAlreadyBoundTelegram.Error()},
		{name: "user not found", err: telegrampkg.ErrTelegramUserNotFound, statusCode: http.StatusNotFound, wantError: telegrampkg.ErrTelegramUserNotFound.Error()},
		{name: "internal", err: errors.New("boom"), statusCode: http.StatusInternalServerError, wantError: "上游服务暂不可用"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			handler := &TelegramHandler{
				telegramService: &stubTelegramService{
					generateBindCodeFn: func(userID string) (string, time.Time, error) {
						if userID != "user_1" {
							t.Fatalf("unexpected userID: %s", userID)
						}
						return "", time.Time{}, tc.err
					},
				},
			}

			ctx, recorder := newTestTelegramContext(http.MethodPost, "/api/v1/user/telegram/bindcode", nil)
			ctx.Set("userID", "user_1")

			handler.GenerateBindCode(ctx)

			if recorder.Code != tc.statusCode {
				t.Fatalf("expected status %d, got %d", tc.statusCode, recorder.Code)
			}

			var resp struct {
				Error string `json:"error"`
			}
			if err := json.Unmarshal(recorder.Body.Bytes(), &resp); err != nil {
				t.Fatalf("failed to decode response: %v", err)
			}
			if resp.Error != tc.wantError {
				t.Fatalf("expected error %q, got %q", tc.wantError, resp.Error)
			}
		})
	}
}

func TestTelegramHandlerUnbindMapsErrors(t *testing.T) {
	gin.SetMode(gin.TestMode)

	testCases := []struct {
		name       string
		err        error
		statusCode int
		wantError  string
	}{
		{name: "not bound", err: telegrampkg.ErrTelegramNotBound, statusCode: http.StatusBadRequest, wantError: telegrampkg.ErrTelegramNotBound.Error()},
		{name: "user not found", err: telegrampkg.ErrTelegramUserNotFound, statusCode: http.StatusNotFound, wantError: telegrampkg.ErrTelegramUserNotFound.Error()},
		{name: "internal", err: errors.New("boom"), statusCode: http.StatusInternalServerError, wantError: "上游服务暂不可用"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			handler := &TelegramHandler{
				telegramService: &stubTelegramService{
					unbindFn: func(userID string) error {
						if userID != "user_1" {
							t.Fatalf("unexpected userID: %s", userID)
						}
						return tc.err
					},
				},
			}

			ctx, recorder := newTestTelegramContext(http.MethodDelete, "/api/v1/user/telegram/bind", nil)
			ctx.Set("userID", "user_1")

			handler.Unbind(ctx)

			if recorder.Code != tc.statusCode {
				t.Fatalf("expected status %d, got %d", tc.statusCode, recorder.Code)
			}

			var resp struct {
				Error string `json:"error"`
			}
			if err := json.Unmarshal(recorder.Body.Bytes(), &resp); err != nil {
				t.Fatalf("failed to decode response: %v", err)
			}
			if resp.Error != tc.wantError {
				t.Fatalf("expected error %q, got %q", tc.wantError, resp.Error)
			}
		})
	}
}

func TestTelegramHandlerVerifyBindMapsErrors(t *testing.T) {
	gin.SetMode(gin.TestMode)

	testCases := []struct {
		name       string
		err        error
		statusCode int
		wantError  string
	}{
		{name: "invalid code", err: telegrampkg.ErrTelegramBindCodeInvalid, statusCode: http.StatusBadRequest, wantError: telegramBindGenericError},
		{name: "telegram already bound", err: telegrampkg.ErrTelegramAlreadyBound, statusCode: http.StatusBadRequest, wantError: telegramBindGenericError},
		{name: "user already bound", err: telegrampkg.ErrUserAlreadyBoundTelegram, statusCode: http.StatusBadRequest, wantError: telegramBindGenericError},
		{name: "internal", err: errors.New("boom"), statusCode: http.StatusInternalServerError, wantError: "上游服务暂不可用"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			handler := &TelegramHandler{
				telegramService: &stubTelegramService{
					verifyBindFn: func(telegramID int64, code string) (*telegrampkg.BindResult, error) {
						if telegramID != 123456 || code != "123456" {
							t.Fatalf("unexpected request: telegramID=%d code=%s", telegramID, code)
						}
						return nil, tc.err
					},
				},
			}

			body := []byte(`{"telegramId":123456,"code":"123456"}`)
			ctx, recorder := newTestTelegramContext(http.MethodPost, "/api/v1/internal/telegram/bind", body)

			handler.VerifyBind(ctx)

			if recorder.Code != tc.statusCode {
				t.Fatalf("expected status %d, got %d", tc.statusCode, recorder.Code)
			}

			var resp struct {
				Error string `json:"error"`
			}
			if err := json.Unmarshal(recorder.Body.Bytes(), &resp); err != nil {
				t.Fatalf("failed to decode response: %v", err)
			}
			if resp.Error != tc.wantError {
				t.Fatalf("expected error %q, got %q", tc.wantError, resp.Error)
			}
		})
	}
}

func TestTelegramHandlerSubscribeByTelegramMapsErrors(t *testing.T) {
	gin.SetMode(gin.TestMode)

	testCases := []struct {
		name       string
		err        error
		statusCode int
		wantError  string
	}{
		{name: "not bound", err: telegrampkg.ErrTelegramNotBound, statusCode: http.StatusBadRequest, wantError: "请求参数错误"},
		{name: "duplicated", err: subscriptionpkg.ErrSubscriptionDuplicated, statusCode: http.StatusConflict, wantError: subscriptionpkg.ErrSubscriptionDuplicated.Error()},
		{name: "invalid season", err: subscriptionpkg.ErrSubscriptionInvalidSeason, statusCode: http.StatusBadRequest, wantError: subscriptionpkg.ErrSubscriptionInvalidSeason.Error()},
		{name: "internal", err: errors.New("boom"), statusCode: http.StatusInternalServerError, wantError: "上游服务暂不可用"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			handler := &TelegramHandler{
				telegramService: &stubTelegramService{
					subscribeByTelegramFn: func(req telegrampkg.TelegramSubscribeRequest) error {
						if req.TelegramID != 123456 || req.TmdbID != "100" || req.Name != "Test Title" {
							t.Fatalf("unexpected request: %+v", req)
						}
						return tc.err
					},
				},
			}

			body := []byte(`{"telegramId":123456,"type":"MOVIE","name":"Test Title","tmdbId":"100","season":0,"posterPath":"/poster.jpg"}`)
			ctx, recorder := newTestTelegramContext(http.MethodPost, "/api/v1/internal/telegram/subscribe", body)

			handler.SubscribeByTelegram(ctx)

			if recorder.Code != tc.statusCode {
				t.Fatalf("expected status %d, got %d", tc.statusCode, recorder.Code)
			}

			var resp struct {
				Error string `json:"error"`
			}
			if err := json.Unmarshal(recorder.Body.Bytes(), &resp); err != nil {
				t.Fatalf("failed to decode response: %v", err)
			}
			if resp.Error != tc.wantError {
				t.Fatalf("expected error %q, got %q", tc.wantError, resp.Error)
			}
		})
	}
}

func TestTelegramHandlerPopPendingRejectPassesAdminUserID(t *testing.T) {
	gin.SetMode(gin.TestMode)

	originalPopPendingReject := popPendingReject
	t.Cleanup(func() {
		popPendingReject = originalPopPendingReject
	})

	popPendingReject = func(ctx context.Context, chatID int64, adminUserID string) (*models.BotPendingRejectRequest, error) {
		_ = ctx
		if chatID != 2002 || adminUserID != "1001" {
			t.Fatalf("unexpected pop args: chatID=%d adminUserID=%s", chatID, adminUserID)
		}
		messageID := int64(77)
		return &models.BotPendingRejectRequest{
			ID:             "pending_1",
			ChatID:         chatID,
			AdminUserID:    adminUserID,
			SubscriptionID: "sub_123",
			MessageID:      &messageID,
			HasPhoto:       true,
			OriginalText:   "<b>原始审批消息</b>",
			ExpiresAt:      time.Date(2026, 4, 29, 12, 0, 0, 0, time.UTC),
		}, nil
	}

	body := []byte(`{"chatId":2002,"adminUserId":"1001"}`)
	ctx, recorder := newTestTelegramContext(http.MethodPost, "/api/v1/internal/telegram/reject-request/pop", body)

	handler := &TelegramHandler{}
	handler.PopPendingReject(ctx)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", recorder.Code)
	}

	var resp struct {
		AdminUserID    string `json:"adminUserId"`
		SubscriptionID string `json:"subscriptionId"`
		MessageID      *int64 `json:"messageId"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp.AdminUserID != "1001" || resp.SubscriptionID != "sub_123" || resp.MessageID == nil || *resp.MessageID != 77 {
		t.Fatalf("unexpected response: %+v", resp)
	}
}

func TestTelegramHandlerPopPendingRejectRequiresAdminUserID(t *testing.T) {
	gin.SetMode(gin.TestMode)

	originalPopPendingReject := popPendingReject
	t.Cleanup(func() {
		popPendingReject = originalPopPendingReject
	})

	popPendingReject = func(ctx context.Context, chatID int64, adminUserID string) (*models.BotPendingRejectRequest, error) {
		_ = ctx
		t.Fatalf("pop should not be called when adminUserId is missing: chatID=%d adminUserID=%s", chatID, adminUserID)
		return nil, nil
	}

	body := []byte(`{"chatId":2002}`)
	ctx, recorder := newTestTelegramContext(http.MethodPost, "/api/v1/internal/telegram/reject-request/pop", body)

	handler := &TelegramHandler{}
	handler.PopPendingReject(ctx)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", recorder.Code)
	}
}
