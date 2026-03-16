package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/konghang/ember/backend/internal/models"
	"github.com/konghang/ember/backend/internal/services"
)

type stubRedemptionCodeService struct {
	createBatchFn func(req *services.CreateRedemptionCodesBatchRequest) (*services.CreateRedemptionCodesBatchResponse, error)
}

func (s *stubRedemptionCodeService) CreateRedemptionCode(req *services.CreateRedemptionCodeRequest) (*models.RedemptionCode, error) {
	return nil, nil
}

func (s *stubRedemptionCodeService) CreateRedemptionCodesBatch(req *services.CreateRedemptionCodesBatchRequest) (*services.CreateRedemptionCodesBatchResponse, error) {
	if s.createBatchFn == nil {
		return nil, nil
	}
	return s.createBatchFn(req)
}

func (s *stubRedemptionCodeService) GetRedemptionCodes(req *services.GetRedemptionCodesRequest) (*services.GetRedemptionCodesResponse, error) {
	return nil, nil
}

func (s *stubRedemptionCodeService) DeleteRedemptionCode(id string) error {
	return nil
}

func (s *stubRedemptionCodeService) UpdateRedemptionCode(id string, req *services.UpdateRedemptionCodeRequest) (*models.RedemptionCode, error) {
	return nil, nil
}

func (s *stubRedemptionCodeService) ValidateCode(code string) (*models.RedemptionCode, error) {
	return nil, nil
}

func (s *stubRedemptionCodeService) GetUserTemplates() (*services.GetUserTemplatesResponse, error) {
	return nil, nil
}

func newTestRedemptionCodeContext(method, target string, body []byte) (*gin.Context, *httptest.ResponseRecorder) {
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(method, target, bytes.NewReader(body))
	if body != nil {
		ctx.Request.Header.Set("Content-Type", "application/json")
	}
	return ctx, recorder
}

func TestRedemptionCodeHandlerCreateBatch(t *testing.T) {
	gin.SetMode(gin.TestMode)

	handler := &RedemptionCodeHandler{
		service: &stubRedemptionCodeService{
			createBatchFn: func(req *services.CreateRedemptionCodesBatchRequest) (*services.CreateRedemptionCodesBatchResponse, error) {
				if req.Count != 2 {
					t.Fatalf("unexpected count: %d", req.Count)
				}
				if req.MaxUses != 3 || req.DefaultDays != 30 {
					t.Fatalf("unexpected request payload: %+v", req)
				}
				expiresAt := time.Date(2026, 3, 20, 10, 0, 0, 0, time.UTC)
				return &services.CreateRedemptionCodesBatchResponse{
					Data: []models.RedemptionCode{
						{ID: "code-1", Code: "abcd1234abcd1234", MaxUses: 3, DefaultDays: 30, ExpiresAt: &expiresAt},
						{ID: "code-2", Code: "dcba4321dcba4321", MaxUses: 3, DefaultDays: 30, ExpiresAt: &expiresAt},
					},
					Count: 2,
				}, nil
			},
		},
	}

	body := []byte(`{"count":2,"maxUses":3,"defaultDays":30,"expiresAt":"2026-03-20T10:00:00Z"}`)
	ctx, recorder := newTestRedemptionCodeContext(http.MethodPost, "/api/v1/admin/redemption-codes/batch", body)
	handler.CreateRedemptionCodesBatch(ctx)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", recorder.Code)
	}

	var resp services.CreateRedemptionCodesBatchResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp.Count != 2 || len(resp.Data) != 2 {
		t.Fatalf("unexpected response: %+v", resp)
	}
}

func TestRedemptionCodeHandlerCreateBatchBadRequest(t *testing.T) {
	gin.SetMode(gin.TestMode)

	handler := &RedemptionCodeHandler{service: &stubRedemptionCodeService{}}
	ctx, recorder := newTestRedemptionCodeContext(http.MethodPost, "/api/v1/admin/redemption-codes/batch", []byte(`{"count":0,`))
	handler.CreateRedemptionCodesBatch(ctx)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", recorder.Code)
	}
}

func TestRedemptionCodeHandlerCreateBatchMapsRequestErrors(t *testing.T) {
	gin.SetMode(gin.TestMode)

	testCases := []struct {
		name string
		err  error
	}{
		{name: "batch count invalid", err: services.ErrRedemptionCodeBatchCountInvalid},
		{name: "template user missing", err: services.ErrTemplateUserNotFound},
		{name: "template user role invalid", err: services.ErrTemplateUserMustBeUser},
		{name: "template user emby missing", err: services.ErrTemplateUserEmbyRequired},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			handler := &RedemptionCodeHandler{
				service: &stubRedemptionCodeService{
					createBatchFn: func(req *services.CreateRedemptionCodesBatchRequest) (*services.CreateRedemptionCodesBatchResponse, error) {
						return nil, tc.err
					},
				},
			}

			body := []byte(`{"count":2,"maxUses":3,"defaultDays":30}`)
			ctx, recorder := newTestRedemptionCodeContext(http.MethodPost, "/api/v1/admin/redemption-codes/batch", body)
			handler.CreateRedemptionCodesBatch(ctx)

			if recorder.Code != http.StatusBadRequest {
				t.Fatalf("expected status 400, got %d", recorder.Code)
			}
		})
	}
}
