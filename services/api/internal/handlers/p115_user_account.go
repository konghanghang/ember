package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/konghang/ember/backend/internal/middleware"
	p115accountpkg "github.com/konghang/ember/backend/internal/services/p115account"
)

const maxP115UserRequestBody = 20 * 1024

type p115UserAccountService interface {
	GetPersonalAccount(context.Context, string) (*p115accountpkg.PersonalAccountSummary, error)
	GetPersonalUsage(context.Context, string) (*p115accountpkg.PersonalUsageSummary, error)
	CreatePersonalAccount(context.Context, string, string) (*p115accountpkg.PersonalAccountSummary, error)
	ReplacePersonalCookie(context.Context, string, string) (*p115accountpkg.PersonalAccountSummary, error)
	ValidatePersonalAccount(context.Context, string) (*p115accountpkg.PersonalValidationResult, error)
	UpdatePersonalDirectory(context.Context, string, string) (*p115accountpkg.PersonalAccountSummary, error)
	UpdatePersonalConcurrency(context.Context, string, int) (*p115accountpkg.PersonalAccountSummary, error)
	SetPersonalEnabled(context.Context, string, bool) (*p115accountpkg.PersonalAccountSummary, error)
	RevokePersonalAccount(context.Context, string) error
}

// GetUsage returns current-user playback attribution and transfer quota usage.
func (h *P115UserAccountHandler) GetUsage(c *gin.Context) {
	userID, ok := p115CurrentUserID(c)
	if !ok {
		return
	}
	usage, err := h.service.GetPersonalUsage(c.Request.Context(), userID)
	if err != nil {
		handleP115AccountError(c, err)
		return
	}
	c.JSON(http.StatusOK, usage)
}

// P115UserAccountHandler exposes the current user's write-only personal 115 account lifecycle.
type P115UserAccountHandler struct {
	service p115UserAccountService
}

type p115UserCookieRequest struct {
	Cookie string `json:"cookie"`
}

type p115UserDirectoryRequest struct {
	TargetParentPath *string `json:"targetParentPath"`
}

type p115UserConcurrencyRequest struct {
	MaxConcurrentStreams *int `json:"maxConcurrentStreams"`
}

type p115UserEnabledRequest struct {
	Enabled *bool `json:"enabled"`
}

// NewP115UserAccountHandler builds the user-owned account handler.
func NewP115UserAccountHandler(service *p115accountpkg.Service) *P115UserAccountHandler {
	return &P115UserAccountHandler{service: service}
}

// Get returns the current user's safe personal account summary.
func (h *P115UserAccountHandler) Get(c *gin.Context) {
	userID, ok := p115CurrentUserID(c)
	if !ok {
		return
	}
	account, err := h.service.GetPersonalAccount(c.Request.Context(), userID)
	if err != nil {
		handleP115AccountError(c, err)
		return
	}
	c.JSON(http.StatusOK, account)
}

// Create accepts exactly one write-only Cookie and creates a disabled pending account.
func (h *P115UserAccountHandler) Create(c *gin.Context) {
	userID, ok := p115CurrentUserID(c)
	if !ok {
		return
	}
	var req p115UserCookieRequest
	if err := decodeStrictP115JSON(c, &req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数错误"})
		return
	}
	account, err := h.service.CreatePersonalAccount(c.Request.Context(), userID, req.Cookie)
	if err != nil {
		handleP115AccountError(c, err)
		return
	}
	c.JSON(http.StatusCreated, account)
}

// ReplaceCookie resets Provider-derived state using exactly one write-only Cookie.
func (h *P115UserAccountHandler) ReplaceCookie(c *gin.Context) {
	userID, ok := p115CurrentUserID(c)
	if !ok {
		return
	}
	var req p115UserCookieRequest
	if err := decodeStrictP115JSON(c, &req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数错误"})
		return
	}
	account, err := h.service.ReplacePersonalCookie(c.Request.Context(), userID, req.Cookie)
	if err != nil {
		handleP115AccountError(c, err)
		return
	}
	c.JSON(http.StatusOK, account)
}

// Validate explicitly checks the saved personal Cookie without enabling the account.
func (h *P115UserAccountHandler) Validate(c *gin.Context) {
	userID, ok := p115CurrentUserID(c)
	if !ok {
		return
	}
	result, err := h.service.ValidatePersonalAccount(c.Request.Context(), userID)
	if err != nil {
		handleP115AccountError(c, err)
		return
	}
	c.JSON(http.StatusOK, result)
}

// UpdateDirectory resolves and stores one existing target directory.
func (h *P115UserAccountHandler) UpdateDirectory(c *gin.Context) {
	userID, ok := p115CurrentUserID(c)
	if !ok {
		return
	}
	var req p115UserDirectoryRequest
	if err := decodeStrictP115JSON(c, &req); err != nil || req.TargetParentPath == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数错误"})
		return
	}
	account, err := h.service.UpdatePersonalDirectory(c.Request.Context(), userID, *req.TargetParentPath)
	if err != nil {
		handleP115AccountError(c, err)
		return
	}
	c.JSON(http.StatusOK, account)
}

// UpdateConcurrency stores a plan-bounded personal account stream limit.
func (h *P115UserAccountHandler) UpdateConcurrency(c *gin.Context) {
	userID, ok := p115CurrentUserID(c)
	if !ok {
		return
	}
	var req p115UserConcurrencyRequest
	if err := decodeStrictP115JSON(c, &req); err != nil || req.MaxConcurrentStreams == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数错误"})
		return
	}
	account, err := h.service.UpdatePersonalConcurrency(c.Request.Context(), userID, *req.MaxConcurrentStreams)
	if err != nil {
		handleP115AccountError(c, err)
		return
	}
	c.JSON(http.StatusOK, account)
}

// SetEnabled rechecks complete account and plan state before enabling.
func (h *P115UserAccountHandler) SetEnabled(c *gin.Context) {
	userID, ok := p115CurrentUserID(c)
	if !ok {
		return
	}
	var req p115UserEnabledRequest
	if err := decodeStrictP115JSON(c, &req); err != nil || req.Enabled == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数错误"})
		return
	}
	account, err := h.service.SetPersonalEnabled(c.Request.Context(), userID, *req.Enabled)
	if err != nil {
		handleP115AccountError(c, err)
		return
	}
	c.JSON(http.StatusOK, account)
}

// Revoke irreversibly erases the current personal credential and is idempotent.
func (h *P115UserAccountHandler) Revoke(c *gin.Context) {
	userID, ok := p115CurrentUserID(c)
	if !ok {
		return
	}
	if err := h.service.RevokePersonalAccount(c.Request.Context(), userID); err != nil {
		handleP115AccountError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "115 账号已解绑"})
}

func p115CurrentUserID(c *gin.Context) (string, bool) {
	principal, ok := middleware.GetValidatedPrincipal(c)
	if !ok || principal.Role != "user" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "未认证"})
		return "", false
	}
	return principal.UserID, true
}

func decodeStrictP115JSON(c *gin.Context, target interface{}) error {
	decoder := json.NewDecoder(io.LimitReader(c.Request.Body, maxP115UserRequestBody+1))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return errors.New("request body must contain exactly one JSON object")
	}
	return nil
}
