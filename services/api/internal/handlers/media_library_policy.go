package handlers

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/konghang/ember/backend/internal/common/httpx"
	embyint "github.com/konghang/ember/backend/internal/integrations/emby"
	policypkg "github.com/konghang/ember/backend/internal/services/policy"
	userpkg "github.com/konghang/ember/backend/internal/services/user"
	"gorm.io/gorm"
)

type updatePlanGroupLibrariesRequest struct {
	LibraryIDs           []string `json:"libraryIds"`
	ApplyToExistingUsers *bool    `json:"applyToExistingUsers,omitempty"`
}

type updateUserLibrariesRequest struct {
	EnabledLibraryIDs []string `json:"enabledLibraryIds"`
}

type telegramMediaLibrariesRequest struct {
	TelegramID int64 `json:"telegramId" binding:"required"`
}

type updateUserEmbyAccessRequest struct {
	Disabled bool `json:"disabled"`
}

func newPolicyService() *policypkg.Service {
	return policypkg.NewService(embyint.GetSharedService())
}

func currentUserIDPtr(c *gin.Context) *string {
	if value, ok := c.Get("userID"); ok {
		if userID, ok := value.(string); ok && userID != "" {
			return &userID
		}
	}
	return nil
}

func handlePolicyError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, policypkg.ErrPlanGroupInvalid),
		errors.Is(err, policypkg.ErrLibraryIDInvalid),
		errors.Is(err, policypkg.ErrLibraryOutsideTemplate),
		errors.Is(err, policypkg.ErrPolicyTemplateInvalid),
		errors.Is(err, policypkg.ErrUserEmbyNotBound):
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
	case errors.Is(err, policypkg.ErrActivePolicySyncTask):
		c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
	case errors.Is(err, policypkg.ErrPlanGroupNotFound),
		errors.Is(err, policypkg.ErrDefaultPlanGroupNotFound),
		errors.Is(err, userpkg.ErrUserNotFound),
		errors.Is(err, gorm.ErrRecordNotFound),
		errors.Is(err, policypkg.ErrBatchNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
	case errors.Is(err, policypkg.ErrTelegramNotBound):
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数错误"})
	default:
		httpx.InternalError(c, err)
	}
}

// GetAdminMediaLibraries 返回当前 Emby 媒体库列表，供分组模板配置使用。
func (h *PaymentHandler) GetAdminMediaLibraries(c *gin.Context) {
	libraries, err := newPolicyService().GetAdminMediaLibraries()
	if err != nil {
		handlePolicyError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": libraries})
}

// GetPlanGroupMediaLibraries 返回某个分组已保存的媒体库模板。
func (h *PaymentHandler) GetPlanGroupMediaLibraries(c *gin.Context) {
	resp, err := newPolicyService().GetPlanGroupMediaLibraries(c.Param("key"))
	if err != nil {
		handlePolicyError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": resp})
}

// UpdatePlanGroupMediaLibraries 保存分组媒体库模板，并按请求决定是否立刻同步现有用户。
func (h *PaymentHandler) UpdatePlanGroupMediaLibraries(c *gin.Context) {
	var req updatePlanGroupLibrariesRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数错误"})
		return
	}
	applyToExistingUsers := true
	if req.ApplyToExistingUsers != nil {
		applyToExistingUsers = *req.ApplyToExistingUsers
	}
	resp, err := newPolicyService().UpdatePlanGroupMediaLibraries(c.Param("key"), req.LibraryIDs, applyToExistingUsers, currentUserIDPtr(c))
	if err != nil {
		handlePolicyError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": resp})
}

func (h *PaymentHandler) GetPlanGroupEmbyPolicyTemplate(c *gin.Context) {
	resp, err := newPolicyService().GetPlanGroupEmbyPolicyTemplate(c.Param("key"))
	if err != nil {
		handlePolicyError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": resp})
}

func (h *PaymentHandler) UpdatePlanGroupEmbyPolicyTemplate(c *gin.Context) {
	var req policypkg.PlanGroupEmbyPolicyTemplateUpdateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数错误"})
		return
	}
	resp, err := newPolicyService().UpdatePlanGroupEmbyPolicyTemplate(c.Param("key"), req, currentUserIDPtr(c))
	if err != nil {
		handlePolicyError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": resp})
}

func (h *PaymentHandler) GetEmbyPolicySyncBatch(c *gin.Context) {
	resp, err := newPolicyService().GetEmbyPolicySyncBatch(c.Param("id"))
	if err != nil {
		handlePolicyError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": resp})
}

func (h *PaymentHandler) RetryFailedEmbyPolicySyncBatch(c *gin.Context) {
	resp, err := newPolicyService().RetryFailedEmbyPolicySyncBatch(c.Param("id"))
	if err != nil {
		handlePolicyError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": resp})
}

func (h *PaymentHandler) PreviewPlanGroupMediaLibrarySync(c *gin.Context) {
	resp, err := newPolicyService().BuildPlanGroupMediaLibrarySyncPreview(c.Param("key"))
	if err != nil {
		handlePolicyError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": resp})
}

func (h *PaymentHandler) ApplyPlanGroupMediaLibrarySync(c *gin.Context) {
	var req policypkg.MediaLibrarySyncApplyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数错误"})
		return
	}
	resp, err := newPolicyService().ApplyPlanGroupMediaLibrarySync(c.Param("key"), req, currentUserIDPtr(c))
	if err != nil {
		handlePolicyError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": resp})
}

func (h *UserHandler) GetUserMediaLibraries(c *gin.Context) {
	userID, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "未登录"})
		return
	}
	resp, err := newPolicyService().GetUserMediaLibrarySettings(userID.(string))
	if err != nil {
		handlePolicyError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": resp})
}

func (h *UserHandler) UpdateUserMediaLibraries(c *gin.Context) {
	userID, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "未登录"})
		return
	}
	var req updateUserLibrariesRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数错误"})
		return
	}
	resp, err := newPolicyService().SaveUserMediaLibraryPreferences(userID.(string), req.EnabledLibraryIDs)
	if err != nil {
		handlePolicyError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": resp})
}

func (h *UserHandler) ResetUserMediaLibraryPreferences(c *gin.Context) {
	userID, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "未登录"})
		return
	}
	resp, err := newPolicyService().ResetUserMediaLibraryPreferences(userID.(string))
	if err != nil {
		handlePolicyError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": resp})
}

// ApplyCurrentUserMediaLibraryPolicy 立即把当前登录用户的有效媒体库设置同步到 Emby。
func (h *UserHandler) ApplyCurrentUserMediaLibraryPolicy(c *gin.Context) {
	userID, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "未登录"})
		return
	}
	resp, err := newPolicyService().ApplyUserCurrentPolicy(userID.(string), "user_media_library_apply_current")
	if err != nil {
		handlePolicyError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": resp})
}

func (h *UserHandler) ClearAdminUserMediaLibraryPreferences(c *gin.Context) {
	resp, err := newPolicyService().ResetUserMediaLibraryPreferences(c.Param("id"))
	if err != nil {
		handlePolicyError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": resp})
}

func (h *UserHandler) SyncAdminUserMediaLibraryPreferences(c *gin.Context) {
	resp, err := newPolicyService().SyncUserMediaLibraryPreferencesFromEmby(c.Param("id"))
	if err != nil {
		handlePolicyError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": resp})
}

// ApplyAdminUserCurrentPolicy 立即把目标用户当前有效的媒体库设置同步到 Emby。
func (h *UserHandler) ApplyAdminUserCurrentPolicy(c *gin.Context) {
	if _, err := newPolicyService().ApplyUserCurrentPolicy(c.Param("id"), "admin_user_policy_apply_current"); err != nil {
		handlePolicyError(c, err)
		return
	}
	user, err := h.userService.GetUserByID(c.Param("id"))
	if err != nil {
		handlePolicyError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": user})
}

// RetryAdminUserPolicySync 由管理员手动重试单个用户当前有效 Emby Policy 同步。
func (h *UserHandler) RetryAdminUserPolicySync(c *gin.Context) {
	if err := newPolicyService().RetryUserPolicySyncFailure(c.Param("id")); err != nil {
		handlePolicyError(c, err)
		return
	}
	user, err := h.userService.GetUserByID(c.Param("id"))
	if err != nil {
		handlePolicyError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": user})
}

func (h *UserHandler) UpdateAdminUserEmbyAccess(c *gin.Context) {
	var req updateUserEmbyAccessRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数错误"})
		return
	}
	if err := newPolicyService().UpdateUserEmbyAccess(c.Param("id"), req.Disabled); err != nil {
		handlePolicyError(c, err)
		return
	}
	user, err := h.userService.GetUserByID(c.Param("id"))
	if err != nil {
		handlePolicyError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": user})
}

func (h *TelegramHandler) GetMediaLibraries(c *gin.Context) {
	var req telegramMediaLibrariesRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数错误"})
		return
	}
	resp, err := newPolicyService().GetTelegramMediaLibrarySettings(req.TelegramID)
	if err != nil {
		handlePolicyError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": resp})
}

func (h *TelegramHandler) ToggleMediaLibrary(c *gin.Context) {
	var req telegramMediaLibrariesRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数错误"})
		return
	}
	resp, err := newPolicyService().ToggleTelegramMediaLibrary(req.TelegramID, c.Param("libraryId"))
	if err != nil {
		handlePolicyError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": resp})
}

func (h *TelegramHandler) ResetMediaLibraryPreferences(c *gin.Context) {
	var req telegramMediaLibrariesRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数错误"})
		return
	}
	resp, err := newPolicyService().ResetTelegramMediaLibraryPreferences(req.TelegramID)
	if err != nil {
		handlePolicyError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": resp})
}
