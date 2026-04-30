package handlers

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/konghang/ember/backend/internal/models"
	subscriptionpkg "github.com/konghang/ember/backend/internal/services/subscription"
)

type stubSubscriptionService struct {
	deleteFn func(subscriptionID, userID string) error
}

func (s *stubSubscriptionService) CreateSubscriptionWithResult(userID string, req subscriptionpkg.CreateSubscriptionRequest) (*subscriptionpkg.CreateSubscriptionResult, error) {
	return nil, nil
}

func (s *stubSubscriptionService) ResubmitSubscriptionWithResult(userID, subscriptionID string, req subscriptionpkg.ResubmitSubscriptionRequest) (*subscriptionpkg.CreateSubscriptionResult, error) {
	return nil, nil
}

func (s *stubSubscriptionService) CheckExisting(req subscriptionpkg.CheckExistingRequest) (*subscriptionpkg.CheckExistingResponse, error) {
	return nil, nil
}

func (s *stubSubscriptionService) GetUserSubscriptions(userID string) ([]models.Subscription, error) {
	return nil, nil
}

func (s *stubSubscriptionService) GetUserSubscriptionsPaginated(userID string, status *models.SubscriptionStatus, page, pageSize int) (*subscriptionpkg.GetAllSubscriptionsResponse, error) {
	return nil, nil
}

func (s *stubSubscriptionService) DeleteSubscription(subscriptionID, userID string) error {
	if s.deleteFn == nil {
		return nil
	}
	return s.deleteFn(subscriptionID, userID)
}

func (s *stubSubscriptionService) GetAllSubscriptions(status *models.SubscriptionStatus, page, pageSize int) (*subscriptionpkg.GetAllSubscriptionsResponse, error) {
	return nil, nil
}

func (s *stubSubscriptionService) ApproveSubscription(subscriptionID string) error {
	return nil
}

func (s *stubSubscriptionService) RejectSubscription(subscriptionID, reason string) error {
	return nil
}

func (s *stubSubscriptionService) MarkSubscriptionIngestedAsAdmin(subscriptionID string) error {
	return nil
}

func (s *stubSubscriptionService) RedispatchSubscription(subscriptionID string) error {
	return nil
}

func (s *stubSubscriptionService) DeleteSubscriptionAsAdmin(subscriptionID string) error {
	return nil
}

func newTestSubscriptionContext(method, target string, body []byte) (*gin.Context, *httptest.ResponseRecorder) {
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(method, target, bytes.NewReader(body))
	if body != nil {
		ctx.Request.Header.Set("Content-Type", "application/json")
	}
	return ctx, recorder
}

func TestSubscriptionHandlerDeleteSubscriptionMapsErrors(t *testing.T) {
	gin.SetMode(gin.TestMode)

	testCases := []struct {
		name       string
		err        error
		statusCode int
		wantError  string
	}{
		{name: "not found", err: subscriptionpkg.ErrSubscriptionNotFound, statusCode: http.StatusNotFound, wantError: subscriptionpkg.ErrSubscriptionNotFound.Error()},
		{name: "delete forbidden", err: subscriptionpkg.ErrSubscriptionDeleteForbidden, statusCode: http.StatusBadRequest, wantError: subscriptionpkg.ErrSubscriptionDeleteForbidden.Error()},
		{name: "delete state", err: subscriptionpkg.ErrSubscriptionDeleteState, statusCode: http.StatusBadRequest, wantError: subscriptionpkg.ErrSubscriptionDeleteState.Error()},
		{name: "internal", err: errors.New("db failed"), statusCode: http.StatusInternalServerError, wantError: "上游服务暂不可用"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			handler := &SubscriptionHandler{
				service: &stubSubscriptionService{
					deleteFn: func(subscriptionID, userID string) error {
						if subscriptionID != "sub_1" || userID != "user_1" {
							t.Fatalf("unexpected args: subscriptionID=%s userID=%s", subscriptionID, userID)
						}
						return tc.err
					},
				},
			}

			ctx, recorder := newTestSubscriptionContext(http.MethodDelete, "/api/v1/user/subscriptions/sub_1", nil)
			ctx.Params = gin.Params{{Key: "id", Value: "sub_1"}}
			ctx.Set("userID", "user_1")

			handler.DeleteSubscription(ctx)

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
