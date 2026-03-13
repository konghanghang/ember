package handlers

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/konghang/ember/backend/internal/services"
)

type stubConfigService struct {
	listFn      func() ([]services.ConfigItem, error)
	updateFn    func(key string, req services.UpdateConfigRequest, updatedByUserID string) (*services.ConfigItem, error)
	testGroupFn func(group string) (*services.ConfigGroupTestResult, error)
	importEnvFn func(updatedByUserID string) (*services.ImportEnvResult, error)
}

func (s *stubConfigService) List() ([]services.ConfigItem, error) {
	if s.listFn == nil {
		return nil, nil
	}
	return s.listFn()
}

func (s *stubConfigService) Update(key string, req services.UpdateConfigRequest, updatedByUserID string) (*services.ConfigItem, error) {
	if s.updateFn == nil {
		return nil, nil
	}
	return s.updateFn(key, req, updatedByUserID)
}

func (s *stubConfigService) TestGroup(group string) (*services.ConfigGroupTestResult, error) {
	if s.testGroupFn == nil {
		return nil, nil
	}
	return s.testGroupFn(group)
}

func (s *stubConfigService) ImportEnv(updatedByUserID string) (*services.ImportEnvResult, error) {
	if s.importEnvFn == nil {
		return nil, nil
	}
	return s.importEnvFn(updatedByUserID)
}

func newTestConfigContext(method, target string, body []byte) (*gin.Context, *httptest.ResponseRecorder) {
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(method, target, bytes.NewReader(body))
	if body != nil {
		ctx.Request.Header.Set("Content-Type", "application/json")
	}
	return ctx, recorder
}

func TestConfigHandlerGetConfigs(t *testing.T) {
	gin.SetMode(gin.TestMode)

	handler := &ConfigHandler{
		service: &stubConfigService{
			listFn: func() ([]services.ConfigItem, error) {
				value := "open"
				return []services.ConfigItem{
					{
						Key:      "registration_mode",
						Group:    "business",
						Label:    "注册模式",
						Source:   services.ConfigSourceDefault,
						HasValue: true,
						Value:    &value,
					},
				}, nil
			},
		},
	}

	ctx, recorder := newTestConfigContext(http.MethodGet, "/api/v1/admin/configs", nil)
	handler.GetConfigs(ctx)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", recorder.Code)
	}

	var resp struct {
		Data []services.ConfigItem `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if len(resp.Data) != 1 || resp.Data[0].Key != "registration_mode" {
		t.Fatalf("unexpected response data: %+v", resp.Data)
	}
}

func TestConfigHandlerGetConfigsInternalError(t *testing.T) {
	gin.SetMode(gin.TestMode)

	handler := &ConfigHandler{
		service: &stubConfigService{
			listFn: func() ([]services.ConfigItem, error) {
				return nil, errors.New("boom")
			},
		},
	}

	ctx, recorder := newTestConfigContext(http.MethodGet, "/api/v1/admin/configs", nil)
	handler.GetConfigs(ctx)

	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("expected status 500, got %d", recorder.Code)
	}
}

func TestConfigHandlerUpdateConfig(t *testing.T) {
	gin.SetMode(gin.TestMode)

	handler := &ConfigHandler{
		service: &stubConfigService{
			updateFn: func(key string, req services.UpdateConfigRequest, updatedByUserID string) (*services.ConfigItem, error) {
				if key != "EMBY_URL" {
					t.Fatalf("unexpected key: %s", key)
				}
				if updatedByUserID != "admin-user" {
					t.Fatalf("unexpected updatedByUserID: %s", updatedByUserID)
				}
				if req.Value == nil || *req.Value != "https://emby.example.com" {
					t.Fatalf("unexpected request payload: %+v", req)
				}
				value := "https://emby.example.com"
				return &services.ConfigItem{
					Key:      key,
					Group:    "media",
					Label:    "Emby 服务地址",
					Source:   services.ConfigSourceDatabase,
					HasValue: true,
					Value:    &value,
				}, nil
			},
		},
	}

	body := []byte(`{"value":"https://emby.example.com"}`)
	ctx, recorder := newTestConfigContext(http.MethodPatch, "/api/v1/admin/configs/EMBY_URL", body)
	ctx.Params = gin.Params{{Key: "key", Value: "EMBY_URL"}}
	ctx.Set("userID", "admin-user")

	handler.UpdateConfig(ctx)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", recorder.Code)
	}
}

func TestConfigHandlerUpdateConfigBadRequest(t *testing.T) {
	gin.SetMode(gin.TestMode)

	handler := &ConfigHandler{service: &stubConfigService{}}
	ctx, recorder := newTestConfigContext(http.MethodPatch, "/api/v1/admin/configs/EMBY_URL", []byte(`{"value":`))
	ctx.Params = gin.Params{{Key: "key", Value: "EMBY_URL"}}

	handler.UpdateConfig(ctx)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", recorder.Code)
	}
}

func TestConfigHandlerUpdateConfigMapsErrors(t *testing.T) {
	gin.SetMode(gin.TestMode)

	testCases := []struct {
		name       string
		err        error
		statusCode int
	}{
		{name: "not found", err: services.ErrConfigNotFound, statusCode: http.StatusNotFound},
		{name: "not editable", err: services.ErrConfigNotEditable, statusCode: http.StatusBadRequest},
		{name: "value required", err: services.ErrConfigValueRequired, statusCode: http.StatusBadRequest},
		{name: "encryption key missing", err: services.ErrConfigEncryptionKeyMissing, statusCode: http.StatusBadRequest},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			handler := &ConfigHandler{
				service: &stubConfigService{
					updateFn: func(key string, req services.UpdateConfigRequest, updatedByUserID string) (*services.ConfigItem, error) {
						return nil, tc.err
					},
				},
			}

			body := []byte(`{"value":"test"}`)
			ctx, recorder := newTestConfigContext(http.MethodPatch, "/api/v1/admin/configs/TEST_KEY", body)
			ctx.Params = gin.Params{{Key: "key", Value: "TEST_KEY"}}

			handler.UpdateConfig(ctx)

			if recorder.Code != tc.statusCode {
				t.Fatalf("expected status %d, got %d", tc.statusCode, recorder.Code)
			}
		})
	}
}

func TestConfigHandlerTestConfigGroup(t *testing.T) {
	gin.SetMode(gin.TestMode)

	handler := &ConfigHandler{
		service: &stubConfigService{
			testGroupFn: func(group string) (*services.ConfigGroupTestResult, error) {
				if group != services.ConfigGroupEmail {
					t.Fatalf("unexpected group: %s", group)
				}
				return &services.ConfigGroupTestResult{
					Success: true,
					Message: "邮件配置检查通过",
					Details: []services.ConfigGroupTestDetail{
						{Target: "smtp", Success: true, Message: "连接成功"},
					},
				}, nil
			},
		},
	}

	ctx, recorder := newTestConfigContext(http.MethodPost, "/api/v1/admin/configs/email/test", nil)
	ctx.Params = gin.Params{{Key: "group", Value: services.ConfigGroupEmail}}

	handler.TestConfigGroup(ctx)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", recorder.Code)
	}
}

func TestConfigHandlerTestConfigGroupUnsupported(t *testing.T) {
	gin.SetMode(gin.TestMode)

	handler := &ConfigHandler{
		service: &stubConfigService{
			testGroupFn: func(group string) (*services.ConfigGroupTestResult, error) {
				return nil, services.ErrConfigGroupUnsupported
			},
		},
	}

	ctx, recorder := newTestConfigContext(http.MethodPost, "/api/v1/admin/configs/unknown/test", nil)
	ctx.Params = gin.Params{{Key: "group", Value: "unknown"}}

	handler.TestConfigGroup(ctx)

	if recorder.Code != http.StatusNotFound {
		t.Fatalf("expected status 404, got %d", recorder.Code)
	}
}

func TestConfigHandlerImportEnv(t *testing.T) {
	gin.SetMode(gin.TestMode)

	handler := &ConfigHandler{
		service: &stubConfigService{
			importEnvFn: func(updatedByUserID string) (*services.ImportEnvResult, error) {
				if updatedByUserID != "admin-user" {
					t.Fatalf("unexpected updatedByUserID: %s", updatedByUserID)
				}
				return &services.ImportEnvResult{
					Imported: []string{"EMBY_URL"},
					Skipped:  map[string]string{"SMTP_HOST": "环境变量未设置"},
					Failed:   map[string]string{},
				}, nil
			},
		},
	}

	ctx, recorder := newTestConfigContext(http.MethodPost, "/api/v1/admin/configs/import-env", nil)
	ctx.Set("userID", "admin-user")

	handler.ImportEnv(ctx)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", recorder.Code)
	}
}

func TestConfigHandlerImportEnvInternalError(t *testing.T) {
	gin.SetMode(gin.TestMode)

	handler := &ConfigHandler{
		service: &stubConfigService{
			importEnvFn: func(updatedByUserID string) (*services.ImportEnvResult, error) {
				return nil, errors.New("boom")
			},
		},
	}

	ctx, recorder := newTestConfigContext(http.MethodPost, "/api/v1/admin/configs/import-env", nil)
	handler.ImportEnv(ctx)

	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("expected status 500, got %d", recorder.Code)
	}
}
