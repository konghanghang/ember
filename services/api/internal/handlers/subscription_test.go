package handlers

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/konghang/ember/backend/internal/middleware"
	"github.com/konghang/ember/backend/internal/models"
	subscriptionpkg "github.com/konghang/ember/backend/internal/services/subscription"
)

type stubSubscriptionService struct {
	deleteFn           func(subscriptionID, userID string) error
	getAllFn           func(status *models.SubscriptionStatus, page, pageSize int) (*subscriptionpkg.GetAllSubscriptionsResponse, error)
	getUserPaginatedFn func(userID string, status *models.SubscriptionStatus, page, pageSize int) (*subscriptionpkg.GetAllSubscriptionsResponse, error)
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
	if s.getUserPaginatedFn == nil {
		return nil, nil
	}
	return s.getUserPaginatedFn(userID, status, page, pageSize)
}

func (s *stubSubscriptionService) DeleteSubscription(subscriptionID, userID string) error {
	if s.deleteFn == nil {
		return nil
	}
	return s.deleteFn(subscriptionID, userID)
}

func (s *stubSubscriptionService) GetAllSubscriptions(status *models.SubscriptionStatus, page, pageSize int) (*subscriptionpkg.GetAllSubscriptionsResponse, error) {
	if s.getAllFn == nil {
		return nil, nil
	}
	return s.getAllFn(status, page, pageSize)
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

func TestSubscriptionHandlerGetSubscriptionsUsesValidatedPrincipal(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("admin branch reads all subscriptions", func(t *testing.T) {
		handler := &SubscriptionHandler{
			service: &stubSubscriptionService{
				getAllFn: func(status *models.SubscriptionStatus, page, pageSize int) (*subscriptionpkg.GetAllSubscriptionsResponse, error) {
					if status == nil || *status != models.SubscriptionPending || page != 2 || pageSize != 20 {
						t.Fatalf("unexpected admin query args: status=%v page=%d pageSize=%d", status, page, pageSize)
					}
					return &subscriptionpkg.GetAllSubscriptionsResponse{}, nil
				},
				getUserPaginatedFn: func(userID string, status *models.SubscriptionStatus, page, pageSize int) (*subscriptionpkg.GetAllSubscriptionsResponse, error) {
					t.Fatalf("user branch should not be called for admin principal")
					return nil, nil
				},
			},
		}

		ctx, recorder := newTestSubscriptionContext(http.MethodGet, "/api/v1/subscriptions?status=PENDING&page=2&pageSize=20", nil)
		ctx.Set("userID", "admin_1")
		ctx.Set("principal", middleware.AuthPrincipal{UserID: "admin_1", Role: "admin", IsActive: true})

		handler.GetSubscriptions(ctx)

		if recorder.Code != http.StatusOK {
			t.Fatalf("expected status %d, got %d", http.StatusOK, recorder.Code)
		}
	})

	t.Run("user branch keeps user scope", func(t *testing.T) {
		handler := &SubscriptionHandler{
			service: &stubSubscriptionService{
				getAllFn: func(status *models.SubscriptionStatus, page, pageSize int) (*subscriptionpkg.GetAllSubscriptionsResponse, error) {
					t.Fatalf("admin branch should not be called for user principal")
					return nil, nil
				},
				getUserPaginatedFn: func(userID string, status *models.SubscriptionStatus, page, pageSize int) (*subscriptionpkg.GetAllSubscriptionsResponse, error) {
					if userID != "user_1" || page != 1 || pageSize != 12 {
						t.Fatalf("unexpected user query args: userID=%s page=%d pageSize=%d", userID, page, pageSize)
					}
					if status != nil {
						t.Fatalf("expected nil status for invalid filter, got %v", *status)
					}
					return &subscriptionpkg.GetAllSubscriptionsResponse{}, nil
				},
			},
		}

		ctx, recorder := newTestSubscriptionContext(http.MethodGet, "/api/v1/subscriptions?status=INVALID", nil)
		ctx.Set("userID", "user_1")
		ctx.Set("principal", middleware.AuthPrincipal{UserID: "user_1", Role: "user", IsActive: true})

		handler.GetSubscriptions(ctx)

		if recorder.Code != http.StatusOK {
			t.Fatalf("expected status %d, got %d", http.StatusOK, recorder.Code)
		}
	})
}
