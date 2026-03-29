package handlers

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	configpkg "github.com/konghang/ember/backend/internal/config"
)

type stubConfigService struct {
	listFn      func() ([]configpkg.ConfigItem, error)
	updateFn    func(key string, req configpkg.UpdateConfigRequest, updatedByUserID string) (*configpkg.ConfigItem, error)
	testGroupFn func(group string) (*configpkg.ConfigGroupTestResult, error)
}

func (s *stubConfigService) List() ([]configpkg.ConfigItem, error) {
	if s.listFn == nil {
		return nil, nil
	}
	return s.listFn()
}

func (s *stubConfigService) Update(key string, req configpkg.UpdateConfigRequest, updatedByUserID string) (*configpkg.ConfigItem, error) {
	if s.updateFn == nil {
		return nil, nil
	}
	return s.updateFn(key, req, updatedByUserID)
}

func (s *stubConfigService) TestGroup(group string) (*configpkg.ConfigGroupTestResult, error) {
	if s.testGroupFn == nil {
		return nil, nil
	}
	return s.testGroupFn(group)
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
			listFn: func() ([]configpkg.ConfigItem, error) {
				value := "open"
				return []configpkg.ConfigItem{
					{
						Key:      "registration_mode",
						Group:    "business",
						Label:    "注册模式",
						Source:   configpkg.ConfigSourceDefault,
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
		Data []configpkg.ConfigItem `json:"data"`
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
			listFn: func() ([]configpkg.ConfigItem, error) {
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
			updateFn: func(key string, req configpkg.UpdateConfigRequest, updatedByUserID string) (*configpkg.ConfigItem, error) {
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
				return &configpkg.ConfigItem{
					Key:      key,
					Group:    "media",
					Label:    "Emby 服务地址",
					Source:   configpkg.ConfigSourceDatabase,
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
		{name: "not found", err: configpkg.ErrConfigNotFound, statusCode: http.StatusNotFound},
		{name: "not editable", err: configpkg.ErrConfigNotEditable, statusCode: http.StatusBadRequest},
		{name: "value required", err: configpkg.ErrConfigValueRequired, statusCode: http.StatusBadRequest},
		{name: "encryption key missing", err: configpkg.ErrConfigEncryptionKeyMissing, statusCode: http.StatusBadRequest},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			handler := &ConfigHandler{
				service: &stubConfigService{
					updateFn: func(key string, req configpkg.UpdateConfigRequest, updatedByUserID string) (*configpkg.ConfigItem, error) {
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
			testGroupFn: func(group string) (*configpkg.ConfigGroupTestResult, error) {
				if group != configpkg.ConfigGroupEmail {
					t.Fatalf("unexpected group: %s", group)
				}
				return &configpkg.ConfigGroupTestResult{
					Success: true,
					Message: "邮件配置检查通过",
					Details: []configpkg.ConfigGroupTestDetail{
						{Target: "smtp", Success: true, Message: "连接成功"},
					},
				}, nil
			},
		},
	}

	ctx, recorder := newTestConfigContext(http.MethodPost, "/api/v1/admin/configs/email/test", nil)
	ctx.Params = gin.Params{{Key: "group", Value: configpkg.ConfigGroupEmail}}

	handler.TestConfigGroup(ctx)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", recorder.Code)
	}
}

func TestConfigHandlerTestConfigGroupUnsupported(t *testing.T) {
	gin.SetMode(gin.TestMode)

	handler := &ConfigHandler{
		service: &stubConfigService{
			testGroupFn: func(group string) (*configpkg.ConfigGroupTestResult, error) {
				return nil, configpkg.ErrConfigGroupUnsupported
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
