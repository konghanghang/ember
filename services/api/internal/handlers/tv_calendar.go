package handlers

import (
	"crypto/subtle"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/konghang/ember/backend/internal/services"
)

// TVCalendarHandler 追剧日历处理器
type TVCalendarHandler struct {
	service *services.TVCalendarService
}

func NewTVCalendarHandler() *TVCalendarHandler {
	return &TVCalendarHandler{service: services.NewTVCalendarService()}
}

func (h *TVCalendarHandler) GetGlobalWeeklyCalendar(c *gin.Context) {
	status := strings.TrimSpace(c.Query("status"))
	weekStart, err := services.ParseTVCalendarWeekDate(c.Query("weekDate"), c.Query("weekOffset"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	data, err := h.service.GetGlobalWeeklyCalendar(c.Request.Context(), weekStart, status)
	if err != nil {
		switch {
		case errors.Is(err, services.ErrTVCalendarInvalidStatus), errors.Is(err, services.ErrTVCalendarInvalidWeekOffset), errors.Is(err, services.ErrTVCalendarInvalidDate):
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		case errors.Is(err, services.ErrTVCalendarNotConfigured):
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": err.Error()})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		}
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": data})
}

func (h *TVCalendarHandler) GetFollowingWeeklyCalendar(c *gin.Context) {
	userID, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "未登录"})
		return
	}

	status := strings.TrimSpace(c.Query("status"))
	weekStart, err := services.ParseTVCalendarWeekDate(c.Query("weekDate"), c.Query("weekOffset"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	data, err := h.service.GetFollowingWeeklyCalendar(c.Request.Context(), userID.(string), weekStart, status)
	if err != nil {
		switch {
		case errors.Is(err, services.ErrTVCalendarInvalidStatus), errors.Is(err, services.ErrTVCalendarInvalidWeekOffset), errors.Is(err, services.ErrTVCalendarInvalidDate):
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		case errors.Is(err, services.ErrTVCalendarNotConfigured):
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": err.Error()})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		}
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": data})
}

func (h *TVCalendarHandler) GetCalendar(c *gin.Context) {
	userID, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "未登录"})
		return
	}

	status := strings.TrimSpace(c.Query("status"))
	startDate, endDate := services.DefaultTVCalendarDateRange()

	if raw := strings.TrimSpace(c.Query("startDate")); raw != "" {
		parsed, err := services.ParseTVCalendarDate(raw)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		startDate = parsed
	}
	if raw := strings.TrimSpace(c.Query("endDate")); raw != "" {
		parsed, err := services.ParseTVCalendarDate(raw)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		endDate = parsed
	}
	if endDate.Before(startDate) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "endDate 不能早于 startDate"})
		return
	}

	items, err := h.service.FetchCalendar(c.Request.Context(), userID.(string), startDate, endDate, status)
	if err != nil {
		switch {
		case errors.Is(err, services.ErrTVCalendarInvalidStatus), errors.Is(err, services.ErrTVCalendarInvalidDate):
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		case errors.Is(err, services.ErrTVCalendarNotConfigured):
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": err.Error()})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		}
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": items})
}

func (h *TVCalendarHandler) Subscribe(c *gin.Context) {
	userID, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "未登录"})
		return
	}

	var req services.CreateTVCalendarSubscriptionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数错误"})
		return
	}

	if err := h.service.Subscribe(userID.(string), req); err != nil {
		switch {
		case errors.Is(err, services.ErrTVCalendarTMDBIDRequired), errors.Is(err, services.ErrTVCalendarShowNameNeeded):
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		}
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true})
}

func (h *TVCalendarHandler) GetSubscriptions(c *gin.Context) {
	userID, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "未登录"})
		return
	}

	subscriptions, err := h.service.GetSubscriptions(userID.(string))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": subscriptions})
}

func (h *TVCalendarHandler) Unsubscribe(c *gin.Context) {
	userID, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "未登录"})
		return
	}

	tmdbID := strings.TrimSpace(c.Param("tmdbId"))
	if err := h.service.Unsubscribe(userID.(string), tmdbID); err != nil {
		if errors.Is(err, services.ErrTVCalendarTMDBIDRequired) {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true})
}

func (h *TVCalendarHandler) Sync(c *gin.Context) {
	var req struct {
		TmdbID      *string `json:"tmdbId"`
		Force       bool    `json:"force"`
		WeekOffsets []int   `json:"weekOffsets"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		// 空 body 视为“同步全部默认周视图”，保持请求兼容性。
		if !errors.Is(err, io.EOF) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数错误"})
			return
		}
	}

	count, err := h.service.SyncCalendar(c.Request.Context(), req.WeekOffsets, req.TmdbID, req.Force)
	if err != nil {
		switch {
		case errors.Is(err, services.ErrTVCalendarInvalidWeekOffset):
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		case errors.Is(err, services.ErrTVCalendarNotConfigured):
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": err.Error()})
			return
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "count": count})
}

func (h *TVCalendarHandler) Refresh(c *gin.Context) {
	h.Sync(c)
}

func (h *TVCalendarHandler) HandleEmbyWebhook(c *gin.Context) {
	configuredToken := resolveEmbyWebhookToken()
	if configuredToken == "" {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Webhook token 未配置"})
		return
	}

	reqToken := strings.TrimSpace(c.Query("token"))
	if subtle.ConstantTimeCompare([]byte(reqToken), []byte(configuredToken)) != 1 {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Webhook token 无效"})
		return
	}

	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "读取请求体失败"})
		return
	}

	var payload map[string]interface{}
	if err := json.Unmarshal(body, &payload); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求体格式错误"})
		return
	}

	eventName := strings.ToLower(extractString(payload, "Event", "event", "NotificationType", "notificationType"))
	if eventName != "library.new" && eventName != "item.added" {
		c.JSON(http.StatusOK, gin.H{"success": true, "ignored": true})
		return
	}

	item := extractMap(payload, "Item", "item")
	if len(item) == 0 {
		c.JSON(http.StatusOK, gin.H{"success": true, "ignored": true})
		return
	}

	itemType := strings.ToLower(extractString(item, "Type", "type"))
	if itemType != "episode" {
		c.JSON(http.StatusOK, gin.H{"success": true, "ignored": true})
		return
	}

	if !hasPhysicalMedia(item) {
		c.JSON(http.StatusOK, gin.H{"success": true, "ignored": true})
		return
	}

	season := extractInt(item, "ParentIndexNumber", "parentIndexNumber", "SeasonNumber", "seasonNumber")
	episode := extractInt(item, "IndexNumber", "indexNumber", "EpisodeNumber", "episodeNumber")
	if season <= 0 || episode <= 0 {
		c.JSON(http.StatusOK, gin.H{"success": true, "ignored": true})
		return
	}

	tmdbID := extractTMDBID(item)
	seriesID := extractString(item, "SeriesId", "seriesId", "SeriesID", "ParentId", "parentId")
	embyItemID := extractString(item, "Id", "id")

	updatedCount, err := h.service.MarkEpisodeReadyByWebhook(c.Request.Context(), tmdbID, seriesID, season, episode, embyItemID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"updated": updatedCount > 0,
		"count":   updatedCount,
	})
}

func resolveEmbyWebhookToken() string {
	return strings.TrimSpace(os.Getenv("EMBY_WEBHOOK_TOKEN"))
}

func extractString(data map[string]interface{}, keys ...string) string {
	for _, key := range keys {
		v, ok := data[key]
		if !ok || v == nil {
			continue
		}
		s, ok := v.(string)
		if ok {
			return strings.TrimSpace(s)
		}
	}
	return ""
}

func extractMap(data map[string]interface{}, keys ...string) map[string]interface{} {
	for _, key := range keys {
		v, ok := data[key]
		if !ok || v == nil {
			continue
		}
		obj, ok := v.(map[string]interface{})
		if ok {
			return obj
		}
	}
	return nil
}

func extractInt(data map[string]interface{}, keys ...string) int {
	for _, key := range keys {
		v, ok := data[key]
		if !ok || v == nil {
			continue
		}
		switch typed := v.(type) {
		case float64:
			return int(typed)
		case int:
			return typed
		case int32:
			return int(typed)
		case int64:
			return int(typed)
		case json.Number:
			i64, err := typed.Int64()
			if err == nil {
				return int(i64)
			}
		case string:
			i, err := strconv.Atoi(strings.TrimSpace(typed))
			if err == nil {
				return i
			}
		}
	}
	return 0
}

func extractBool(data map[string]interface{}, keys ...string) bool {
	for _, key := range keys {
		v, ok := data[key]
		if !ok || v == nil {
			continue
		}
		switch typed := v.(type) {
		case bool:
			return typed
		case float64:
			return typed != 0
		case string:
			lower := strings.ToLower(strings.TrimSpace(typed))
			return lower == "1" || lower == "true" || lower == "yes"
		}
	}
	return false
}

func extractTMDBID(item map[string]interface{}) string {
	if value := extractString(item, "Provider_tmdb", "provider_tmdb", "TmdbId", "tmdbId", "TMDB", "tmdb"); value != "" {
		return value
	}

	providerIDs := extractMap(item, "ProviderIds", "providerIds")
	if len(providerIDs) == 0 {
		return ""
	}
	return extractString(providerIDs, "Tmdb", "TMDB", "tmdb")
}

func hasPhysicalMedia(item map[string]interface{}) bool {
	if extractBool(item, "IsMissing", "isMissing") {
		return false
	}

	locationType := strings.ToLower(extractString(item, "LocationType", "locationType"))
	if locationType == "virtual" {
		return false
	}

	path := extractString(item, "Path", "path")
	if path != "" {
		return true
	}

	for _, key := range []string{"MediaSources", "mediaSources"} {
		value, ok := item[key]
		if !ok || value == nil {
			continue
		}
		sources, ok := value.([]interface{})
		if ok && len(sources) > 0 {
			return true
		}
	}

	return false
}
