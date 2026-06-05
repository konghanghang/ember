package middleware

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/konghang/ember/backend/internal/common"
	"github.com/konghang/ember/backend/internal/models"
)

func TestAdminCredentialAuthWithAPIKey(t *testing.T) {
	gin.SetMode(gin.TestMode)

	originalValidator := validateAdminAPIKey
	originalLoader := loadPasswordResetUser
	t.Cleanup(func() {
		validateAdminAPIKey = originalValidator
		loadPasswordResetUser = originalLoader
	})

	loadPasswordResetUser = func(userID string) (*models.User, error) {
		t.Fatalf("api key path must not look up users table, got userID=%s", userID)
		return nil, nil
	}

	run := func(valid bool, validateErr error) *httptest.ResponseRecorder {
		validateAdminAPIKey = func(apiKey string) (bool, error) {
			if apiKey != "ember_sk_test_key_with_enough_entropy_for_auth" {
				t.Fatalf("unexpected api key passed to validator")
			}
			return valid, validateErr
		}

		router := gin.New()
		router.GET("/api/v1/admin/system/info", AdminCredentialAuth(), AdminOnly(), func(c *gin.Context) {
			principal, ok := GetValidatedPrincipal(c)
			if !ok {
				t.Fatalf("expected api key principal")
			}
			if principal.UserID != "api_key" || principal.Role != "admin" {
				t.Fatalf("unexpected principal: %+v", principal)
			}
			c.JSON(http.StatusOK, gin.H{"authType": "api_key"})
		})

		req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/system/info", nil)
		req.Header.Set("Authorization", "Bearer ember_sk_test_key_with_enough_entropy_for_auth")
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, req)
		return recorder
	}

	t.Run("allows matching api key", func(t *testing.T) {
		recorder := run(true, nil)
		if recorder.Code != http.StatusOK {
			t.Fatalf("expected api key request to pass, got status=%d body=%s", recorder.Code, recorder.Body.String())
		}
	})

	t.Run("rejects mismatched api key", func(t *testing.T) {
		recorder := run(false, nil)
		if recorder.Code != http.StatusUnauthorized {
			t.Fatalf("expected unauthorized, got status=%d body=%s", recorder.Code, recorder.Body.String())
		}
	})

	t.Run("maps validator error to 500", func(t *testing.T) {
		recorder := run(false, errors.New("store unavailable"))
		if recorder.Code != http.StatusInternalServerError {
			t.Fatalf("expected internal error, got status=%d body=%s", recorder.Code, recorder.Body.String())
		}
	})
}

func TestAdminCredentialAuthWithJWT(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Setenv("JWT_SECRET", "12345678901234567890123456789012")
	if err := common.InitJWT(); err != nil {
		t.Fatalf("InitJWT failed: %v", err)
	}

	originalLoader := loadPasswordResetUser
	t.Cleanup(func() {
		loadPasswordResetUser = originalLoader
	})

	passwordHash := "$2a$10$abcdefghijklmnopqrstuv0123456789012345678901234567890"
	validPwdSig := common.ComputePasswordSignature(passwordHash)

	run := func(seededUser models.User, claimRole string) *httptest.ResponseRecorder {
		loadPasswordResetUser = func(userID string) (*models.User, error) {
			if userID != "user_1" {
				t.Fatalf("unexpected userID lookup: %s", userID)
			}
			user := seededUser
			return &user, nil
		}

		token, err := common.GenerateToken("user_1", "tester", claimRole, validPwdSig)
		if err != nil {
			t.Fatalf("GenerateToken failed: %v", err)
		}

		router := gin.New()
		router.GET("/api/v1/admin/system/info", AdminCredentialAuth(), AdminOnly(), func(c *gin.Context) {
			principal, ok := GetValidatedPrincipal(c)
			if !ok {
				t.Fatalf("expected jwt principal")
			}
			c.JSON(http.StatusOK, gin.H{"role": principal.Role})
		})

		req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/system/info", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, req)
		return recorder
	}

	t.Run("allows jwt admin", func(t *testing.T) {
		recorder := run(models.User{
			ID:       "user_1",
			Role:     "admin",
			IsActive: true,
			Password: passwordHash,
		}, "admin")

		if recorder.Code != http.StatusOK {
			t.Fatalf("expected admin jwt to pass, got status=%d body=%s", recorder.Code, recorder.Body.String())
		}
	})

	t.Run("rejects jwt user", func(t *testing.T) {
		recorder := run(models.User{
			ID:       "user_1",
			Role:     "user",
			IsActive: true,
			Password: passwordHash,
		}, "user")

		if recorder.Code != http.StatusForbidden {
			t.Fatalf("expected user jwt to be forbidden, got status=%d body=%s", recorder.Code, recorder.Body.String())
		}
	})

	t.Run("keeps password reset gate for jwt admin", func(t *testing.T) {
		recorder := run(models.User{
			ID:                    "user_1",
			Role:                  "admin",
			IsActive:              true,
			Password:              passwordHash,
			PasswordResetRequired: true,
		}, "admin")

		var body struct {
			Error string `json:"error"`
		}
		if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}
		if recorder.Code != http.StatusForbidden || body.Error != "当前账号必须先修改密码" {
			t.Fatalf("expected password reset gate, got status=%d body=%+v", recorder.Code, body)
		}
	})
}
