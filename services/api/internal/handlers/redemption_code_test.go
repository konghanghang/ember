package handlers

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/konghang/ember/backend/internal/models"
	redemptionpkg "github.com/konghang/ember/backend/internal/services/redemption"
)

type stubRedemptionCodeService struct {
	createFn      func(req *redemptionpkg.CreateRedemptionCodeRequest) (*models.RedemptionCode, error)
	createBatchFn func(req *redemptionpkg.CreateRedemptionCodesBatchRequest) (*redemptionpkg.CreateRedemptionCodesBatchResponse, error)
	getCodesFn    func(req *redemptionpkg.GetRedemptionCodesRequest) (*redemptionpkg.GetRedemptionCodesResponse, error)
}

func (s *stubRedemptionCodeService) CreateRedemptionCode(req *redemptionpkg.CreateRedemptionCodeRequest) (*models.RedemptionCode, error) {
	if s.createFn == nil {
		return nil, nil
	}
	return s.createFn(req)
}

func (s *stubRedemptionCodeService) CreateRedemptionCodesBatch(req *redemptionpkg.CreateRedemptionCodesBatchRequest) (*redemptionpkg.CreateRedemptionCodesBatchResponse, error) {
	if s.createBatchFn == nil {
		return nil, nil
	}
	return s.createBatchFn(req)
}

func (s *stubRedemptionCodeService) GetRedemptionCodes(req *redemptionpkg.GetRedemptionCodesRequest) (*redemptionpkg.GetRedemptionCodesResponse, error) {
	if s.getCodesFn == nil {
		return nil, nil
	}
	return s.getCodesFn(req)
}

func (s *stubRedemptionCodeService) DeleteRedemptionCode(id string) error {
	return nil
}

func (s *stubRedemptionCodeService) UpdateRedemptionCode(id string, req *redemptionpkg.UpdateRedemptionCodeRequest) (*models.RedemptionCode, error) {
	return nil, nil
}

func (s *stubRedemptionCodeService) ValidateRegistrationCode(code string) (*models.RedemptionCode, error) {
	return nil, nil
}

func (s *stubRedemptionCodeService) ValidateRenewalCode(code string) (*models.RedemptionCode, error) {
	return nil, nil
}

func (s *stubRedemptionCodeService) GetUserTemplates() (*redemptionpkg.GetUserTemplatesResponse, error) {
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
			createBatchFn: func(req *redemptionpkg.CreateRedemptionCodesBatchRequest) (*redemptionpkg.CreateRedemptionCodesBatchResponse, error) {
				if req.Count != 2 {
					t.Fatalf("unexpected count: %d", req.Count)
				}
				if req.MaxUses != 3 || req.DefaultDays != 30 {
					t.Fatalf("unexpected request payload: %+v", req)
				}
				expiresAt := time.Date(2026, 3, 20, 10, 0, 0, 0, time.UTC)
				return &redemptionpkg.CreateRedemptionCodesBatchResponse{
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

	var resp redemptionpkg.CreateRedemptionCodesBatchResponse
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
		{name: "batch count invalid", err: redemptionpkg.ErrRedemptionCodeBatchCountInvalid},
		{name: "template user missing", err: redemptionpkg.ErrTemplateUserNotFound},
		{name: "template user role invalid", err: redemptionpkg.ErrTemplateUserMustBeUser},
		{name: "template user emby missing", err: redemptionpkg.ErrTemplateUserEmbyRequired},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			handler := &RedemptionCodeHandler{
				service: &stubRedemptionCodeService{
					createBatchFn: func(req *redemptionpkg.CreateRedemptionCodesBatchRequest) (*redemptionpkg.CreateRedemptionCodesBatchResponse, error) {
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

func TestRedemptionCodeHandlerCreateBatchReturnsGenericInternalError(t *testing.T) {
	gin.SetMode(gin.TestMode)

	handler := &RedemptionCodeHandler{
		service: &stubRedemptionCodeService{
			createBatchFn: func(req *redemptionpkg.CreateRedemptionCodesBatchRequest) (*redemptionpkg.CreateRedemptionCodesBatchResponse, error) {
				return nil, errors.New("db write failed")
			},
		},
	}

	body := []byte(`{"count":2,"maxUses":3,"defaultDays":30}`)
	ctx, recorder := newTestRedemptionCodeContext(http.MethodPost, "/api/v1/admin/redemption-codes/batch", body)
	handler.CreateRedemptionCodesBatch(ctx)

	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("expected status 500, got %d", recorder.Code)
	}

	var resp struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp.Error != "上游服务暂不可用" {
		t.Fatalf("expected generic internal error message, got %q", resp.Error)
	}
}

func TestRedemptionCodeHandlerGetRedemptionCodesBindsFilters(t *testing.T) {
	gin.SetMode(gin.TestMode)

	handler := &RedemptionCodeHandler{
		service: &stubRedemptionCodeService{
			getCodesFn: func(req *redemptionpkg.GetRedemptionCodesRequest) (*redemptionpkg.GetRedemptionCodesResponse, error) {
				if req.Page != 2 || req.PageSize != 20 {
					t.Fatalf("unexpected pagination: %+v", req)
				}
				if req.Code != "ABCD" || req.Status != "expired" || req.TemplateUserID != "user_123" || req.RegistrationPlanGroup != "VIP_A" {
					t.Fatalf("unexpected filters: %+v", req)
				}
				if !req.ShowAll {
					t.Fatalf("expected showAll to be true")
				}
				return &redemptionpkg.GetRedemptionCodesResponse{
					Data:     []models.RedemptionCode{},
					Total:    0,
					Page:     req.Page,
					PageSize: req.PageSize,
				}, nil
			},
		},
	}

	ctx, recorder := newTestRedemptionCodeContext(
		http.MethodGet,
		"/api/v1/admin/redemption-codes?page=2&pageSize=20&showAll=true&code=ABCD&status=expired&templateUserId=user_123&registrationPlanGroup=VIP_A",
		nil,
	)
	handler.GetRedemptionCodes(ctx)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", recorder.Code)
	}
}

func TestRedemptionCodeHandlerGetRedemptionCodesMapsInvalidStatus(t *testing.T) {
	gin.SetMode(gin.TestMode)

	handler := &RedemptionCodeHandler{
		service: &stubRedemptionCodeService{
			getCodesFn: func(req *redemptionpkg.GetRedemptionCodesRequest) (*redemptionpkg.GetRedemptionCodesResponse, error) {
				return nil, redemptionpkg.ErrRedemptionCodeStatusInvalid
			},
		},
	}

	ctx, recorder := newTestRedemptionCodeContext(http.MethodGet, "/api/v1/admin/redemption-codes?status=broken", nil)
	handler.GetRedemptionCodes(ctx)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", recorder.Code)
	}
}
