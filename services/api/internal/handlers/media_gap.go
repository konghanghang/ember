package handlers

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	mediagappkg "github.com/konghang/ember/backend/internal/services/mediagap"
)

type MediaGapHandler struct {
	service     *mediagappkg.MediaGapService
	scanManager *mediaGapScanManager
}

type MediaGapSearchCandidate struct {
	ID          string                 `json:"id"`
	Title       string                 `json:"title"`
	Description string                 `json:"description,omitempty"`
	PublishDate string                 `json:"publishDate,omitempty"`
	Site        string                 `json:"site,omitempty"`
	Size        int64                  `json:"size,omitempty"`
	Seeders     int                    `json:"seeders,omitempty"`
	IsPack      bool                   `json:"isPack"`
	MatchMode   string                 `json:"matchMode"`
	Tags        []string               `json:"tags,omitempty"`
	Payload     map[string]interface{} `json:"payload,omitempty"`
}

type MediaGapSearchResponse struct {
	GapID          string                    `json:"gapId"`
	Query          string                    `json:"query"`
	FallbackQuery  string                    `json:"fallbackQuery,omitempty"`
	MatchMode      string                    `json:"matchMode"`
	Candidates     []MediaGapSearchCandidate `json:"candidates"`
	LastSearchedAt string                    `json:"lastSearchedAt,omitempty"`
}

type MediaGapDispatchParams struct {
	CandidateID      string                 `json:"candidateId,omitempty"`
	CandidatePayload map[string]interface{} `json:"candidatePayload,omitempty"`
}

type MediaGapDispatchResponse struct {
	GapID        string `json:"gapId"`
	Status       string `json:"status"`
	RequestedAt  string `json:"requestedAt,omitempty"`
	DispatchMode string `json:"dispatchMode,omitempty"`
	Message      string `json:"message,omitempty"`
}

type mediaGapListQuery struct {
	Keyword     string `form:"keyword"`
	Status      string `form:"status"`
	AirDateFrom string `form:"airDateFrom"`
	AirDateTo   string `form:"airDateTo"`
	Sort        string `form:"sort"`
	Page        int    `form:"page"`
	PageSize    int    `form:"pageSize"`
}

type mediaGapScanRequest struct {
	TmdbID string `json:"tmdbId"`
	Force  bool   `json:"force"`
}

type mediaGapDispatchRequest struct {
	CandidateID      string                 `json:"candidateId"`
	Candidate        map[string]interface{} `json:"candidate"`
	CandidatePayload map[string]interface{} `json:"candidatePayload"`
}

type mediaGapIgnoreRequest struct {
	Reason string `json:"reason"`
}

func NewMediaGapHandler() *MediaGapHandler {
	service := mediagappkg.NewMediaGapService()
	return &MediaGapHandler{
		service: service,
		scanManager: newMediaGapScanManager(func(ctx context.Context, req mediagappkg.ScanRequest) (*mediagappkg.ScanResult, error) {
			return service.Scan(ctx, req)
		}),
	}
}

func (h *MediaGapHandler) ListMediaGaps(c *gin.Context) {
	h.GetMediaGaps(c)
}

func (h *MediaGapHandler) ListGroupedMediaGaps(c *gin.Context) {
	h.GetGroupedMediaGaps(c)
}

func (h *MediaGapHandler) SearchMediaGap(c *gin.Context) {
	h.SearchMediaGapCandidates(c)
}

// GetMediaGaps 获取缺集列表。
// GET /api/v1/admin/media-gaps
func (h *MediaGapHandler) GetMediaGaps(c *gin.Context) {
	var query mediaGapListQuery
	if err := c.ShouldBindQuery(&query); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数错误"})
		return
	}

	page := query.Page
	if page <= 0 {
		page = 1
	}

	pageSize := query.PageSize
	if pageSize <= 0 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}

	resp, err := h.service.ListMediaGaps(c.Request.Context(), mediagappkg.ListQuery{
		Keyword:     strings.TrimSpace(query.Keyword),
		Status:      strings.TrimSpace(query.Status),
		AirDateFrom: strings.TrimSpace(query.AirDateFrom),
		AirDateTo:   strings.TrimSpace(query.AirDateTo),
		Page:        page,
		PageSize:    pageSize,
	})
	if err != nil {
		writeMediaGapError(c, err)
		return
	}

	c.JSON(http.StatusOK, resp)
}

// GetGroupedMediaGaps 获取按剧聚合的缺集列表。
// GET /api/v1/admin/media-gaps/grouped
func (h *MediaGapHandler) GetGroupedMediaGaps(c *gin.Context) {
	var query mediaGapListQuery
	if err := c.ShouldBindQuery(&query); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数错误"})
		return
	}

	page := query.Page
	if page <= 0 {
		page = 1
	}

	pageSize := query.PageSize
	if pageSize <= 0 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}

	resp, err := h.service.ListGroupedMediaGaps(c.Request.Context(), mediagappkg.GroupedListQuery{
		Keyword:     strings.TrimSpace(query.Keyword),
		Status:      strings.TrimSpace(query.Status),
		AirDateFrom: strings.TrimSpace(query.AirDateFrom),
		AirDateTo:   strings.TrimSpace(query.AirDateTo),
		Sort:        strings.TrimSpace(query.Sort),
		Page:        page,
		PageSize:    pageSize,
	})
	if err != nil {
		writeMediaGapError(c, err)
		return
	}

	c.JSON(http.StatusOK, resp)
}

// ScanMediaGaps 触发缺集扫描。
// POST /api/v1/admin/media-gaps/scan
func (h *MediaGapHandler) ScanMediaGaps(c *gin.Context) {
	var req mediaGapScanRequest
	if err := c.ShouldBindJSON(&req); err != nil && !errors.Is(err, io.EOF) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数错误"})
		return
	}

	scanReq := mediagappkg.ScanRequest{
		TMDBID: strings.TrimSpace(req.TmdbID),
		Force:  req.Force,
	}

	if !h.service.IsConfigured() {
		writeMediaGapError(c, mediagappkg.ErrMediaGapNotConfigured)
		return
	}

	status, started := h.scanManager.Start(scanReq)
	httpStatus := http.StatusAccepted
	if !started {
		httpStatus = http.StatusOK
	}

	c.JSON(httpStatus, gin.H{"data": gin.H{
		"accepted": true,
		"async":    true,
		"mode":     "async",
		"started":  started,
		"scanId":   status.ScanID,
		"scope":    status.Scope,
		"status":   status.Status,
		"running":  status.Running,
		"message":  status.Message,
	}})
}

// GetMediaGapScanStatus 获取当前缺集扫描状态。
// GET /api/v1/admin/media-gaps/scan-status
func (h *MediaGapHandler) GetMediaGapScanStatus(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"data": h.scanManager.Status()})
}

// SearchMediaGapCandidates 搜索缺集候选。
// POST /api/v1/admin/media-gaps/:id/search
func (h *MediaGapHandler) SearchMediaGapCandidates(c *gin.Context) {
	gapID := strings.TrimSpace(c.Param("id"))
	if gapID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "缺集 ID 不能为空"})
		return
	}

	resp, err := h.service.SearchGap(c.Request.Context(), gapID)
	if err != nil {
		writeMediaGapError(c, err)
		return
	}

	searchResult := gin.H{
		"mediaGap": resp,
		"candidates": func() []MediaGapSearchCandidate {
			if resp.SearchSnapshot == nil {
				return []MediaGapSearchCandidate{}
			}
			candidates := make([]MediaGapSearchCandidate, 0, len(resp.SearchSnapshot.Candidates))
			for _, candidate := range resp.SearchSnapshot.Candidates {
				candidates = append(candidates, MediaGapSearchCandidate{
					ID:          buildCandidateID(candidate),
					Title:       candidate.Title,
					Description: candidate.Description,
					PublishDate: candidate.PublishDate,
					Site:        candidate.Site,
					Size:        candidate.Size,
					Seeders:     candidate.Seeders,
					IsPack:      candidate.IsPack,
					MatchMode:   resolveCandidateMatchMode(candidate),
					Tags:        candidate.Tags,
					Payload:     candidate.Payload,
				})
			}
			return candidates
		}(),
	}
	if resp.SearchSnapshot != nil {
		searchResult["query"] = resp.SearchSnapshot.Query
		searchResult["fallbackQuery"] = resp.SearchSnapshot.FallbackQuery
		searchResult["matchMode"] = resp.SearchSnapshot.MatchMode
		searchResult["source"] = resp.SearchSnapshot.Source
	}
	if resp.LastSearchedAt != nil {
		searchResult["searchedAt"] = resp.LastSearchedAt
	}

	c.JSON(http.StatusOK, gin.H{"data": searchResult})
}

// DispatchMediaGap 下发缺集候选。
// POST /api/v1/admin/media-gaps/:id/dispatch
func (h *MediaGapHandler) DispatchMediaGap(c *gin.Context) {
	gapID := strings.TrimSpace(c.Param("id"))
	if gapID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "缺集 ID 不能为空"})
		return
	}

	var req mediaGapDispatchRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数错误"})
		return
	}

	candidatePayload := normalizeDispatchPayload(req)
	if strings.TrimSpace(req.CandidateID) == "" && len(candidatePayload) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "candidateId 或 candidatePayload 不能为空"})
		return
	}

	resp, err := h.service.DispatchGap(c.Request.Context(), gapID, mediagappkg.DispatchRequest{
		Candidate: mediagappkg.SearchCandidate{
			Title:   extractMapString(req.Candidate, "title"),
			Site:    extractMapString(req.Candidate, "site", "source"),
			Payload: candidatePayload,
		},
	})
	if err != nil {
		writeMediaGapError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": gin.H{
		"mediaGap": resp,
		"message":  "候选资源已下发",
	}})
}

// IgnoreMediaGap 忽略缺集工单。
// POST /api/v1/admin/media-gaps/:id/ignore
func (h *MediaGapHandler) IgnoreMediaGap(c *gin.Context) {
	gapID := strings.TrimSpace(c.Param("id"))
	if gapID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "缺集 ID 不能为空"})
		return
	}

	var req mediaGapIgnoreRequest
	if err := c.ShouldBindJSON(&req); err != nil && !errors.Is(err, io.EOF) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数错误"})
		return
	}

	resp, err := h.service.IgnoreGap(c.Request.Context(), gapID, strings.TrimSpace(req.Reason))
	if err != nil {
		writeMediaGapError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": gin.H{
		"mediaGap": resp,
		"message":  "缺集工单已忽略",
	}})
}

func normalizeDispatchPayload(req mediaGapDispatchRequest) map[string]interface{} {
	if len(req.CandidatePayload) > 0 {
		return req.CandidatePayload
	}
	if len(req.Candidate) == 0 {
		return nil
	}
	if rawPayload, ok := req.Candidate["payload"].(map[string]interface{}); ok && len(rawPayload) > 0 {
		return rawPayload
	}
	return req.Candidate
}

func writeMediaGapError(c *gin.Context, err error) {
	statusCode := http.StatusInternalServerError
	switch {
	case errors.Is(err, mediagappkg.ErrMediaGapNotFound):
		statusCode = http.StatusNotFound
	case errors.Is(err, mediagappkg.ErrMediaGapInvalidStatus),
		errors.Is(err, mediagappkg.ErrMediaGapSearchState),
		errors.Is(err, mediagappkg.ErrMediaGapDispatchState),
		errors.Is(err, mediagappkg.ErrMediaGapCandidate):
		statusCode = http.StatusBadRequest
	}

	c.JSON(statusCode, gin.H{"error": err.Error()})
}

func buildCandidateID(candidate mediagappkg.SearchCandidate) string {
	if trimmed := strings.TrimSpace(candidate.ID); trimmed != "" {
		return trimmed
	}
	if payloadID := extractMapString(candidate.Payload, "id", "guid", "hash"); payloadID != "" {
		return payloadID
	}
	return candidate.Title
}

func resolveCandidateMatchMode(candidate mediagappkg.SearchCandidate) string {
	if trimmed := strings.TrimSpace(candidate.MatchMode); trimmed != "" {
		return trimmed
	}
	if candidate.IsPack {
		return "season"
	}
	return "episode"
}

func extractMapString(data map[string]interface{}, keys ...string) string {
	for _, key := range keys {
		value, ok := data[key]
		if !ok || value == nil {
			continue
		}
		if text, ok := value.(string); ok {
			trimmed := strings.TrimSpace(text)
			if trimmed != "" {
				return trimmed
			}
			continue
		}
		rendered := strings.TrimSpace(fmt.Sprint(value))
		if rendered != "" {
			return rendered
		}
	}
	return ""
}
