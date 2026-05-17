package middleware

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/konghang/ember/backend/internal/apiroutes"
	"github.com/konghang/ember/backend/internal/common"
	"github.com/konghang/ember/backend/internal/models"
)

func TestPasswordResetRequired(t *testing.T) {
	gin.SetMode(gin.TestMode)

	originalLoader := loadPasswordResetUser
	t.Cleanup(func() {
		loadPasswordResetUser = originalLoader
	})

	type responseBody struct {
		Error string `json:"error"`
		Role  string `json:"role"`
	}

	run := func(path string, seededUser models.User, claimRole, claimPwdSig string) (*httptest.ResponseRecorder, responseBody) {
		loadPasswordResetUser = func(userID string) (*models.User, error) {
			if userID != "user_1" {
				t.Fatalf("unexpected userID lookup: %s", userID)
			}
			user := seededUser
			return &user, nil
		}

		recorder := httptest.NewRecorder()
		router := gin.New()
		router.Use(func(c *gin.Context) {
			c.Set("userID", "user_1")
			c.Set("role", claimRole)
			c.Set("pwdSig", claimPwdSig)
			c.Next()
		})
		router.GET(path, PasswordResetRequired(), func(c *gin.Context) {
			principal, ok := GetValidatedPrincipal(c)
			if !ok {
				t.Fatalf("expected validated principal to be present")
			}
			role, _ := c.Get("role")
			if principal.UserID != "user_1" {
				t.Fatalf("unexpected principal user id: %s", principal.UserID)
			}
			c.JSON(http.StatusOK, gin.H{"role": role})
		})

		req := httptest.NewRequest(http.MethodGet, path, nil)
		router.ServeHTTP(recorder, req)

		var body responseBody
		if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}
		return recorder, body
	}

	passwordHash := "$2a$10$abcdefghijklmnopqrstuv0123456789012345678901234567890"
	validPwdSig := common.ComputePasswordSignature(passwordHash)

	t.Run("rejects inactive user", func(t *testing.T) {
		recorder, body := run("/api/v1/profile", models.User{
			ID:       "user_1",
			Role:     "user",
			IsActive: false,
			Password: passwordHash,
		}, "user", validPwdSig)

		if recorder.Code != http.StatusUnauthorized || body.Error != "账号已被停用" {
			t.Fatalf("expected inactive rejection, got status=%d body=%+v", recorder.Code, body)
		}
	})

	t.Run("rejects role mismatch", func(t *testing.T) {
		recorder, body := run("/api/v1/profile", models.User{
			ID:       "user_1",
			Role:     "admin",
			IsActive: true,
			Password: passwordHash,
		}, "user", validPwdSig)

		if recorder.Code != http.StatusUnauthorized || body.Error != "登录状态已失效，请重新登录" {
			t.Fatalf("expected role mismatch rejection, got status=%d body=%+v", recorder.Code, body)
		}
	})

	t.Run("rejects password signature mismatch", func(t *testing.T) {
		recorder, body := run("/api/v1/profile", models.User{
			ID:       "user_1",
			Role:     "user",
			IsActive: true,
			Password: passwordHash,
		}, "user", common.ComputePasswordSignature("different_hash"))

		if recorder.Code != http.StatusUnauthorized || body.Error != "登录状态已失效，请重新登录" {
			t.Fatalf("expected pwdSig mismatch rejection, got status=%d body=%+v", recorder.Code, body)
		}
	})

	t.Run("allows password reset whitelist path and sets validated principal", func(t *testing.T) {
		recorder, body := run(apiroutes.FullPasswordPath, models.User{
			ID:                    "user_1",
			Role:                  "user",
			IsActive:              true,
			Password:              passwordHash,
			PasswordResetRequired: true,
		}, "user", validPwdSig)

		if recorder.Code != http.StatusOK {
			t.Fatalf("expected whitelist path to pass, got status=%d body=%+v", recorder.Code, body)
		}
		if body.Role != "user" {
			t.Fatalf("expected validated principal payload, got %+v", body)
		}
	})

	t.Run("allows all password reset closed loop paths", func(t *testing.T) {
		for _, path := range apiroutes.PasswordResetClosedLoopPaths() {
			t.Run(path, func(t *testing.T) {
				recorder, body := run(path, models.User{
					ID:                    "user_1",
					Role:                  "user",
					IsActive:              true,
					Password:              passwordHash,
					PasswordResetRequired: true,
				}, "user", validPwdSig)

				if recorder.Code != http.StatusOK {
					t.Fatalf("expected closed loop path to pass, got status=%d body=%+v", recorder.Code, body)
				}
			})
		}
	})

	t.Run("blocks non whitelist path while password reset required", func(t *testing.T) {
		recorder, body := run("/api/v1/redemptions", models.User{
			ID:                    "user_1",
			Role:                  "user",
			IsActive:              true,
			Password:              passwordHash,
			PasswordResetRequired: true,
		}, "user", validPwdSig)

		if recorder.Code != http.StatusForbidden || body.Error != "当前账号必须先修改密码" {
			t.Fatalf("expected password reset enforcement, got status=%d body=%+v", recorder.Code, body)
		}
	})

	t.Run("rejects lookup failure", func(t *testing.T) {
		loadPasswordResetUser = func(userID string) (*models.User, error) {
			return nil, errors.New("db unavailable")
		}

		recorder := httptest.NewRecorder()
		router := gin.New()
		router.Use(func(c *gin.Context) {
			c.Set("userID", "user_1")
			c.Set("role", "user")
			c.Set("pwdSig", validPwdSig)
			c.Next()
		})
		router.GET("/api/v1/profile", PasswordResetRequired(), func(c *gin.Context) {
			t.Fatalf("handler should not be reached when lookup fails")
		})

		req := httptest.NewRequest(http.MethodGet, "/api/v1/profile", nil)
		router.ServeHTTP(recorder, req)

		if recorder.Code != http.StatusUnauthorized {
			t.Fatalf("expected unauthorized on lookup failure, got %d", recorder.Code)
		}
	})
}
