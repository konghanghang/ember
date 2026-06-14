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
	resubmitFn         func(userID, subscriptionID string, req subscriptionpkg.ResubmitSubscriptionRequest) (*subscriptionpkg.CreateSubscriptionResult, error)
	deleteFn           func(subscriptionID, userID string) error
	getAllFn           func(status *models.SubscriptionStatus, page, pageSize int) (*subscriptionpkg.GetAllSubscriptionsResponse, error)
	getUserPaginatedFn func(userID string, status *models.SubscriptionStatus, page, pageSize int) (*subscriptionpkg.GetAllSubscriptionsResponse, error)
	manualSearchFn     func(subscriptionID string, req subscriptionpkg.ManualSearchRequest) (*subscriptionpkg.ManualSearchResult, error)
	manualDispatchFn   func(subscriptionID string, req subscriptionpkg.ManualDispatchRequest) (*subscriptionpkg.ManualDispatchResult, error)
}

func (s *stubSubscriptionService) CreateSubscriptionWithResult(userID string, req subscriptionpkg.CreateSubscriptionRequest) (*subscriptionpkg.CreateSubscriptionResult, error) {
	return nil, nil
}

func (s *stubSubscriptionService) ResubmitSubscriptionWithResult(userID, subscriptionID string, req subscriptionpkg.ResubmitSubscriptionRequest) (*subscriptionpkg.CreateSubscriptionResult, error) {
	if s.resubmitFn == nil {
		return nil, nil
	}
	return s.resubmitFn(userID, subscriptionID, req)
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

func (s *stubSubscriptionService) ManualSearchSubscription(subscriptionID string, req subscriptionpkg.ManualSearchRequest) (*subscriptionpkg.ManualSearchResult, error) {
	if s.manualSearchFn == nil {
		return nil, nil
	}
	return s.manualSearchFn(subscriptionID, req)
}

func (s *stubSubscriptionService) ManualDispatchSubscription(subscriptionID string, req subscriptionpkg.ManualDispatchRequest) (*subscriptionpkg.ManualDispatchResult, error) {
	if s.manualDispatchFn == nil {
		return nil, nil
	}
	return s.manualDispatchFn(subscriptionID, req)
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
		{name: "delete forbidden", err: subscriptionpkg.ErrSubscriptionDeleteForbidden, statusCode: http.StatusNotFound, wantError: subscriptionpkg.ErrSubscriptionNotFound.Error()},
		{name: "delete state", err: subscriptionpkg.ErrSubscriptionDeleteState, statusCode: http.StatusNotFound, wantError: subscriptionpkg.ErrSubscriptionNotFound.Error()},
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

func TestSubscriptionHandlerResubmitSubscriptionMapsEnumerationErrorsToNotFound(t *testing.T) {
	gin.SetMode(gin.TestMode)

	testCases := []struct {
		name string
		err  error
	}{
		{name: "not found", err: subscriptionpkg.ErrSubscriptionNotFound},
		{name: "forbidden", err: subscriptionpkg.ErrSubscriptionForbidden},
		{name: "not rejected", err: subscriptionpkg.ErrSubscriptionNotRejected},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			handler := &SubscriptionHandler{
				service: &stubSubscriptionService{
					resubmitFn: func(userID, subscriptionID string, req subscriptionpkg.ResubmitSubscriptionRequest) (*subscriptionpkg.CreateSubscriptionResult, error) {
						if subscriptionID != "sub_1" || userID != "user_1" {
							t.Fatalf("unexpected args: subscriptionID=%s userID=%s", subscriptionID, userID)
						}
						return nil, tc.err
					},
				},
			}

			body := []byte(`{"note":"补充说明"}`)
			ctx, recorder := newTestSubscriptionContext(http.MethodPost, "/api/v1/subscriptions/sub_1/resubmit", body)
			ctx.Params = gin.Params{{Key: "id", Value: "sub_1"}}
			ctx.Set("userID", "user_1")

			handler.ResubmitSubscription(ctx)

			if recorder.Code != http.StatusNotFound {
				t.Fatalf("expected status 404, got %d", recorder.Code)
			}

			var resp struct {
				Error string `json:"error"`
			}
			if err := json.Unmarshal(recorder.Body.Bytes(), &resp); err != nil {
				t.Fatalf("failed to decode response: %v", err)
			}
			if resp.Error != subscriptionpkg.ErrSubscriptionNotFound.Error() {
				t.Fatalf("expected error %q, got %q", subscriptionpkg.ErrSubscriptionNotFound.Error(), resp.Error)
			}
		})
	}
}

func TestSubscriptionHandlerManualSearchMapsDomainErrors(t *testing.T) {
	gin.SetMode(gin.TestMode)

	testCases := []struct {
		name       string
		err        error
		statusCode int
	}{
		{name: "not found", err: subscriptionpkg.ErrSubscriptionNotFound, statusCode: http.StatusNotFound},
		{name: "not approved", err: subscriptionpkg.ErrSubscriptionNotApproved, statusCode: http.StatusConflict},
		{name: "season required", err: subscriptionpkg.ErrSubscriptionManualSeason, statusCode: http.StatusBadRequest},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			handler := &SubscriptionHandler{
				service: &stubSubscriptionService{
					manualSearchFn: func(subscriptionID string, req subscriptionpkg.ManualSearchRequest) (*subscriptionpkg.ManualSearchResult, error) {
						if subscriptionID != "sub_1" {
							t.Fatalf("unexpected subscriptionID=%s", subscriptionID)
						}
						if req.Season == nil || *req.Season != 2 {
							t.Fatalf("unexpected season=%v", req.Season)
						}
						return nil, tc.err
					},
				},
			}

			ctx, recorder := newTestSubscriptionContext(http.MethodPost, "/api/v1/admin/subscriptions/sub_1/manual-search", []byte(`{"season":2}`))
			ctx.Params = gin.Params{{Key: "id", Value: "sub_1"}}

			handler.ManualSearchSubscription(ctx)

			if recorder.Code != tc.statusCode {
				t.Fatalf("expected status %d, got %d", tc.statusCode, recorder.Code)
			}
		})
	}
}

func TestSubscriptionHandlerManualDispatchMapsBadRequestErrors(t *testing.T) {
	gin.SetMode(gin.TestMode)

	testCases := []struct {
		name string
		err  error
	}{
		{name: "candidate missing", err: subscriptionpkg.ErrSubscriptionManualCandidate},
		{name: "season missing", err: subscriptionpkg.ErrSubscriptionManualSeason},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			handler := &SubscriptionHandler{
				service: &stubSubscriptionService{
					manualDispatchFn: func(subscriptionID string, req subscriptionpkg.ManualDispatchRequest) (*subscriptionpkg.ManualDispatchResult, error) {
						if subscriptionID != "sub_1" {
							t.Fatalf("unexpected subscriptionID=%s", subscriptionID)
						}
						return nil, tc.err
					},
				},
			}

			ctx, recorder := newTestSubscriptionContext(http.MethodPost, "/api/v1/admin/subscriptions/sub_1/manual-dispatch", []byte(`{"candidateId":"cand_1"}`))
			ctx.Params = gin.Params{{Key: "id", Value: "sub_1"}}

			handler.ManualDispatchSubscription(ctx)

			if recorder.Code != http.StatusBadRequest {
				t.Fatalf("expected status %d, got %d", http.StatusBadRequest, recorder.Code)
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
		ctx.Set("principal", middleware.AuthPrincipal{UserID: "user_1", Role: "user", IsActive: true})

		handler.GetSubscriptions(ctx)

		if recorder.Code != http.StatusOK {
			t.Fatalf("expected status %d, got %d", http.StatusOK, recorder.Code)
		}
	})
}
