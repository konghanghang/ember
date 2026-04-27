package handlers

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/konghang/ember/backend/internal/common/httpx"
	mediapkg "github.com/konghang/ember/backend/internal/services/media"
)

type MediaQualityHandler struct {
	service *mediapkg.MediaQualityService
}

func NewMediaQualityHandler() *MediaQualityHandler {
	return &MediaQualityHandler{
		service: mediapkg.NewMediaQualityService(),
	}
}

// GetLibraries 获取媒体库列表（管理员）
// GET /api/v1/admin/media-quality/libraries
func (h *MediaQualityHandler) GetLibraries(c *gin.Context) {
	libraries, err := h.service.GetLibraries()
	if err != nil {
		httpx.InternalError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": libraries})
}

// GetLibraryQuality 获取媒体库质量报告（管理员）
// GET /api/v1/admin/media-quality/libraries/:libraryId?force=false
func (h *MediaQualityHandler) GetLibraryQuality(c *gin.Context) {
	libraryID := strings.TrimSpace(c.Param("libraryId"))
	force := strings.EqualFold(strings.TrimSpace(c.DefaultQuery("force", "false")), "true")
	page := parsePositiveInt(c.DefaultQuery("page", "1"), 1)
	pageSize := parsePositiveInt(c.DefaultQuery("pageSize", "20"), 20)
	if pageSize > 100 {
		pageSize = 100
	}

	report, err := h.service.GetLibraryQuality(c.Request.Context(), libraryID, force)
	if err != nil {
		switch {
		case errors.Is(err, mediapkg.ErrMediaQualityLibraryIDRequired):
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		case errors.Is(err, mediapkg.ErrMediaQualityScanFailed):
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": err.Error()})
		default:
			httpx.InternalError(c, err)
		}
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": paginateMediaQualityReport(report, page, pageSize)})
}

// ScanLibraryQuality 触发媒体库质量扫描（管理员）
// POST /api/v1/admin/media-quality/libraries/:libraryId/scan
func (h *MediaQualityHandler) ScanLibraryQuality(c *gin.Context) {
	libraryID := strings.TrimSpace(c.Param("libraryId"))

	report, err := h.service.ScanLibraryQuality(c.Request.Context(), libraryID)
	if err != nil {
		switch {
		case errors.Is(err, mediapkg.ErrMediaQualityLibraryIDRequired):
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		case errors.Is(err, mediapkg.ErrMediaQualityScanFailed):
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": err.Error()})
		default:
			httpx.InternalError(c, err)
		}
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": paginateMediaQualityReport(report, 1, 20)})
}

// GetLibraryQualityGroupDetails 获取低画质汇总项的下钻明细（管理员）
// GET /api/v1/admin/media-quality/libraries/:libraryId/groups/:groupId/details
func (h *MediaQualityHandler) GetLibraryQualityGroupDetails(c *gin.Context) {
	libraryID := strings.TrimSpace(c.Param("libraryId"))
	groupID := strings.TrimSpace(c.Param("groupId"))
	force := strings.EqualFold(strings.TrimSpace(c.DefaultQuery("force", "false")), "true")
	page := parsePositiveInt(c.DefaultQuery("page", "1"), 1)
	pageSize := parsePositiveInt(c.DefaultQuery("pageSize", "20"), 20)
	if pageSize > 100 {
		pageSize = 100
	}

	details, err := h.service.GetGroupLowQualityDetails(c.Request.Context(), libraryID, groupID, force)
	if err != nil {
		switch {
		case errors.Is(err, mediapkg.ErrMediaQualityLibraryIDRequired),
			errors.Is(err, mediapkg.ErrMediaQualityGroupIDRequired):
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		case errors.Is(err, mediapkg.ErrMediaQualityScanFailed):
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": err.Error()})
		default:
			httpx.InternalError(c, err)
		}
		return
	}

	total := len(details)
	offset := (page - 1) * pageSize
	if offset > total {
		offset = total
	}
	end := offset + pageSize
	if end > total {
		end = total
	}

	c.JSON(http.StatusOK, gin.H{
		"data":     details[offset:end],
		"total":    total,
		"page":     page,
		"pageSize": pageSize,
	})
}

// GetPoster 获取媒体条目封面（管理员代理）
// GET /api/v1/admin/media-quality/posters/:itemId
func (h *MediaQualityHandler) GetPoster(c *gin.Context) {
	itemID := strings.TrimSpace(c.Param("itemId"))
	if itemID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "itemId 不能为空"})
		return
	}

	maxHeight := parsePositiveInt(c.DefaultQuery("maxHeight", "180"), 180)
	quality := parsePositiveInt(c.DefaultQuery("quality", "90"), 90)

	content, contentType, err := h.service.GetPoster(c.Request.Context(), itemID, maxHeight, quality)
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "获取封面失败"})
		return
	}

	c.Data(http.StatusOK, contentType, content)
}

func parsePositiveInt(raw string, fallback int) int {
	v, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || v <= 0 {
		return fallback
	}
	return v
}

func paginateMediaQualityReport(report *mediapkg.QualityReport, page, pageSize int) *mediapkg.QualityReport {
	if report == nil {
		return nil
	}

	total := len(report.LowQualityItems)
	offset := (page - 1) * pageSize
	if offset > total {
		offset = total
	}
	end := offset + pageSize
	if end > total {
		end = total
	}

	cloned := *report
	cloned.LowQualityTotal = total
	cloned.Page = page
	cloned.PageSize = pageSize
	cloned.LowQualityItems = report.LowQualityItems[offset:end]
	cloned.LowQualityDetails = nil
	return &cloned
}
