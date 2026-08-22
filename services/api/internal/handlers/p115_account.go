package handlers

import (
	"context"
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/konghang/ember/backend/internal/common/httpx"
	p115integration "github.com/konghang/ember/backend/internal/integrations/p115"
	"github.com/konghang/ember/backend/internal/models"
	p115accountpkg "github.com/konghang/ember/backend/internal/services/p115account"
)

type p115AccountService interface {
	List(context.Context) ([]p115accountpkg.AccountSummary, error)
	Get(context.Context, string) (*p115accountpkg.AccountSummary, error)
	Create(context.Context, p115accountpkg.CreateAccountInput) (*p115accountpkg.AccountSummary, error)
	ReplaceCookie(context.Context, string, string) (*p115accountpkg.AccountSummary, error)
	Validate(context.Context, string) (*p115accountpkg.ValidationResult, error)
	SetEnabled(context.Context, string, bool) (*p115accountpkg.AccountSummary, error)
	UpdateSourceLocation(context.Context, string, p115accountpkg.SourceLocationInput) (*p115accountpkg.AccountSummary, error)
}

// P115AccountHandler exposes JWT-admin-only account management without returning credentials.
type P115AccountHandler struct {
	service p115AccountService
}

type createP115AccountRequest struct {
	Role           models.P115AccountRole `json:"role"`
	Alias          string                 `json:"alias"`
	Cookie         string                 `json:"cookie"`
	AppType        string                 `json:"appType"`
	UserAgent      string                 `json:"userAgent"`
	EmbyPathPrefix string                 `json:"embyPathPrefix"`
	SourceRootID   string                 `json:"sourceRootId"`
	TargetParentID string                 `json:"targetParentId"`
}

type updateP115SourceLocationRequest struct {
	EmbyPathPrefix string `json:"embyPathPrefix"`
	SourceRootID   string `json:"sourceRootId"`
}

type replaceP115CookieRequest struct {
	Cookie string `json:"cookie"`
}

type setP115AccountEnabledRequest struct {
	Enabled *bool `json:"enabled" binding:"required"`
}

// NewP115AccountHandler builds the administrator account handler.
func NewP115AccountHandler(service *p115accountpkg.Service) *P115AccountHandler {
	return &P115AccountHandler{service: service}
}

// List returns all safe 115 account summaries.
func (h *P115AccountHandler) List(c *gin.Context) {
	if !requireJWTAdminForP115AccountManagement(c) {
		return
	}
	items, err := h.service.List(c.Request.Context())
	if err != nil {
		handleP115AccountError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": items})
}

// Get returns one safe 115 account summary.
func (h *P115AccountHandler) Get(c *gin.Context) {
	if !requireJWTAdminForP115AccountManagement(c) {
		return
	}
	account, err := h.service.Get(c.Request.Context(), c.Param("id"))
	if err != nil {
		handleP115AccountError(c, err)
		return
	}
	c.JSON(http.StatusOK, account)
}

// Create accepts a write-only Cookie and creates a disabled pending account.
func (h *P115AccountHandler) Create(c *gin.Context) {
	if !requireJWTAdminForP115AccountManagement(c) {
		return
	}
	var req createP115AccountRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数错误"})
		return
	}
	account, err := h.service.Create(c.Request.Context(), p115accountpkg.CreateAccountInput{
		Role:           req.Role,
		Alias:          req.Alias,
		Cookie:         req.Cookie,
		AppType:        req.AppType,
		UserAgent:      req.UserAgent,
		EmbyPathPrefix: req.EmbyPathPrefix,
		SourceRootID:   req.SourceRootID,
		TargetParentID: req.TargetParentID,
	})
	if err != nil {
		handleP115AccountError(c, err)
		return
	}
	c.JSON(http.StatusCreated, account)
}

// UpdateSourceLocation changes the source account's Emby prefix and 115 root.
func (h *P115AccountHandler) UpdateSourceLocation(c *gin.Context) {
	if !requireJWTAdminForP115AccountManagement(c) {
		return
	}
	var req updateP115SourceLocationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数错误"})
		return
	}
	account, err := h.service.UpdateSourceLocation(c.Request.Context(), c.Param("id"), p115accountpkg.SourceLocationInput{
		EmbyPathPrefix: req.EmbyPathPrefix,
		SourceRootID:   req.SourceRootID,
	})
	if err != nil {
		handleP115AccountError(c, err)
		return
	}
	c.JSON(http.StatusOK, account)
}

// ReplaceCookie overwrites the credential and resets the account to pending and disabled.
func (h *P115AccountHandler) ReplaceCookie(c *gin.Context) {
	if !requireJWTAdminForP115AccountManagement(c) {
		return
	}
	var req replaceP115CookieRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数错误"})
		return
	}
	account, err := h.service.ReplaceCookie(c.Request.Context(), c.Param("id"), req.Cookie)
	if err != nil {
		handleP115AccountError(c, err)
		return
	}
	c.JSON(http.StatusOK, account)
}

// Validate performs the explicit read-only Cookie check and persists its state transition.
func (h *P115AccountHandler) Validate(c *gin.Context) {
	if !requireJWTAdminForP115AccountManagement(c) {
		return
	}
	result, err := h.service.Validate(c.Request.Context(), c.Param("id"))
	if err != nil {
		handleP115AccountError(c, err)
		return
	}
	c.JSON(http.StatusOK, result)
}

// SetEnabled enables a validated account or disables an existing account.
func (h *P115AccountHandler) SetEnabled(c *gin.Context) {
	if !requireJWTAdminForP115AccountManagement(c) {
		return
	}
	var req setP115AccountEnabledRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.Enabled == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数错误"})
		return
	}
	account, err := h.service.SetEnabled(c.Request.Context(), c.Param("id"), *req.Enabled)
	if err != nil {
		handleP115AccountError(c, err)
		return
	}
	c.JSON(http.StatusOK, account)
}

func requireJWTAdminForP115AccountManagement(c *gin.Context) bool {
	authType, _ := c.Get("authType")
	if authType == "api_key" {
		c.JSON(http.StatusForbidden, gin.H{"error": "API Key 不能管理 115 账号"})
		c.Abort()
		return false
	}
	return true
}

func handleP115AccountError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, p115accountpkg.ErrAccountNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
	case isP115AccountInputError(err):
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
	case errors.Is(err, p115accountpkg.ErrAccountUnavailable),
		errors.Is(err, p115accountpkg.ErrCredentialChanged),
		errors.Is(err, p115accountpkg.ErrRoleAlreadyEnabled),
		errors.Is(err, p115accountpkg.ErrProviderUserAlreadyEnabled):
		c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
	case errors.Is(err, p115integration.ErrProviderUnavailable),
		errors.Is(err, p115integration.ErrProviderProtocol):
		c.JSON(http.StatusBadGateway, gin.H{"error": "115 服务暂不可用"})
	default:
		httpx.InternalError(c, err)
	}
}

func isP115AccountInputError(err error) bool {
	return errors.Is(err, p115accountpkg.ErrAccountIDRequired) ||
		errors.Is(err, p115accountpkg.ErrInvalidRole) ||
		errors.Is(err, p115accountpkg.ErrAliasRequired) ||
		errors.Is(err, p115accountpkg.ErrAliasInvalid) ||
		errors.Is(err, p115accountpkg.ErrCookieRequired) ||
		errors.Is(err, p115accountpkg.ErrCookieInvalid) ||
		errors.Is(err, p115accountpkg.ErrAppTypeRequired) ||
		errors.Is(err, p115accountpkg.ErrAppTypeInvalid) ||
		errors.Is(err, p115accountpkg.ErrUserAgentRequired) ||
		errors.Is(err, p115accountpkg.ErrUserAgentInvalid) ||
		errors.Is(err, p115accountpkg.ErrTargetParentInvalid) ||
		errors.Is(err, p115accountpkg.ErrPlaybackTargetParentRequired) ||
		errors.Is(err, p115accountpkg.ErrSourceTargetParentUnexpected) ||
		errors.Is(err, p115accountpkg.ErrEmbyPathPrefixRequired) ||
		errors.Is(err, p115accountpkg.ErrEmbyPathPrefixInvalid) ||
		errors.Is(err, p115accountpkg.ErrSourceRootIDRequired) ||
		errors.Is(err, p115accountpkg.ErrSourceRootIDInvalid) ||
		errors.Is(err, p115accountpkg.ErrPlaybackSourceLocationUnexpected) ||
		errors.Is(err, p115accountpkg.ErrSourceLocationOnly)
}
