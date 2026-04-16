package handlers

import (
	"crypto/subtle"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	subscriptionpkg "github.com/konghang/ember/backend/internal/services/subscription"
	tvcalendarpkg "github.com/konghang/ember/backend/internal/services/tvcalendar"
)

// TVCalendarHandler 追剧日历处理器
type TVCalendarHandler struct {
	service             *tvcalendarpkg.TVCalendarService
	subscriptionService *subscriptionpkg.SubscriptionService
}

func NewTVCalendarHandler() *TVCalendarHandler {
	return &TVCalendarHandler{
		service:             tvcalendarpkg.NewTVCalendarService(),
		subscriptionService: subscriptionpkg.NewSubscriptionService(),
	}
}

func (h *TVCalendarHandler) GetGlobalWeeklyCalendar(c *gin.Context) {
	status := strings.TrimSpace(c.Query("status"))
	weekStart, err := tvcalendarpkg.ParseTVCalendarWeekDate(c.Query("weekDate"), c.Query("weekOffset"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	data, err := h.service.GetGlobalWeeklyCalendar(c.Request.Context(), weekStart, status)
	if err != nil {
		switch {
		case errors.Is(err, tvcalendarpkg.ErrTVCalendarInvalidStatus), errors.Is(err, tvcalendarpkg.ErrTVCalendarInvalidWeekOffset), errors.Is(err, tvcalendarpkg.ErrTVCalendarInvalidDate):
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		case errors.Is(err, tvcalendarpkg.ErrTVCalendarNotConfigured):
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
	weekStart, err := tvcalendarpkg.ParseTVCalendarWeekDate(c.Query("weekDate"), c.Query("weekOffset"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	data, err := h.service.GetFollowingWeeklyCalendar(c.Request.Context(), userID.(string), weekStart, status)
	if err != nil {
		switch {
		case errors.Is(err, tvcalendarpkg.ErrTVCalendarInvalidStatus), errors.Is(err, tvcalendarpkg.ErrTVCalendarInvalidWeekOffset), errors.Is(err, tvcalendarpkg.ErrTVCalendarInvalidDate):
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		case errors.Is(err, tvcalendarpkg.ErrTVCalendarNotConfigured):
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
	startDate, endDate := tvcalendarpkg.DefaultTVCalendarDateRange()

	if raw := strings.TrimSpace(c.Query("startDate")); raw != "" {
		parsed, err := tvcalendarpkg.ParseTVCalendarDate(raw)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		startDate = parsed
	}
	if raw := strings.TrimSpace(c.Query("endDate")); raw != "" {
		parsed, err := tvcalendarpkg.ParseTVCalendarDate(raw)
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
		case errors.Is(err, tvcalendarpkg.ErrTVCalendarInvalidStatus), errors.Is(err, tvcalendarpkg.ErrTVCalendarInvalidDate):
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		case errors.Is(err, tvcalendarpkg.ErrTVCalendarNotConfigured):
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

	var req tvcalendarpkg.CreateTVCalendarSubscriptionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数错误"})
		return
	}

	if err := h.service.Subscribe(userID.(string), req); err != nil {
		switch {
		case errors.Is(err, tvcalendarpkg.ErrTVCalendarTMDBIDRequired), errors.Is(err, tvcalendarpkg.ErrTVCalendarShowNameNeeded):
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
		if errors.Is(err, tvcalendarpkg.ErrTVCalendarTMDBIDRequired) {
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
		case errors.Is(err, tvcalendarpkg.ErrTVCalendarInvalidWeekOffset):
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		case errors.Is(err, tvcalendarpkg.ErrTVCalendarNotConfigured):
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
	requestID := strings.TrimSpace(c.GetHeader("X-Request-Id"))
	remoteIP := strings.TrimSpace(c.ClientIP())
	configuredToken := resolveEmbyWebhookToken()
	if configuredToken == "" {
		log.Printf("[TV Calendar] Emby webhook 拒绝：token 未配置 requestId=%s remoteIP=%s", requestID, remoteIP)
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Webhook token 未配置"})
		return
	}

	reqToken := strings.TrimSpace(c.Query("token"))
	if subtle.ConstantTimeCompare([]byte(reqToken), []byte(configuredToken)) != 1 {
		log.Printf("[TV Calendar] Emby webhook 拒绝：token 无效 requestId=%s remoteIP=%s tokenProvided=%t tokenLength=%d", requestID, remoteIP, reqToken != "", len(reqToken))
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Webhook token 无效"})
		return
	}

	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		log.Printf("[TV Calendar] Emby webhook 读取请求体失败 requestId=%s remoteIP=%s err=%v", requestID, remoteIP, err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "读取请求体失败"})
		return
	}

	var payload map[string]interface{}
	if err := json.Unmarshal(body, &payload); err != nil {
		log.Printf("[TV Calendar] Emby webhook JSON 解析失败 requestId=%s remoteIP=%s bodyBytes=%d err=%v", requestID, remoteIP, len(body), err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求体格式错误"})
		return
	}

	eventName := strings.ToLower(extractString(payload, "Event", "event", "NotificationType", "notificationType"))
	log.Printf("[TV Calendar] Emby webhook 收到通知 requestId=%s remoteIP=%s event=%q bodyBytes=%d", requestID, remoteIP, eventName, len(body))
	if eventName != "library.new" && eventName != "item.added" {
		log.Printf("[TV Calendar] Emby webhook 忽略：非入库事件 requestId=%s remoteIP=%s event=%q", requestID, remoteIP, eventName)
		c.JSON(http.StatusOK, gin.H{"success": true, "ignored": true})
		return
	}

	item := extractMap(payload, "Item", "item")
	if len(item) == 0 {
		log.Printf("[TV Calendar] Emby webhook 忽略：缺少 Item 节点 requestId=%s remoteIP=%s event=%q", requestID, remoteIP, eventName)
		c.JSON(http.StatusOK, gin.H{"success": true, "ignored": true})
		return
	}

	itemType := strings.ToLower(extractString(item, "Type", "type"))
	itemName := extractString(item, "Name", "name")
	embyItemID := extractString(item, "Id", "id")
	tmdbID := extractTMDBID(item)
	seriesID := extractSeriesID(item)
	season := extractInt(item, "ParentIndexNumber", "parentIndexNumber", "SeasonNumber", "seasonNumber")
	episode := extractInt(item, "IndexNumber", "indexNumber", "EpisodeNumber", "episodeNumber")
	locationType := extractString(item, "LocationType", "locationType")
	itemPath := extractString(item, "Path", "path")
	isMissing := extractBool(item, "IsMissing", "isMissing")
	physical := hasPhysicalMedia(item)

	log.Printf(
		"[TV Calendar] Emby webhook 条目详情 requestId=%s event=%q itemId=%q itemType=%q itemName=%q tmdbId=%q seriesId=%q season=%d episode=%d locationType=%q isMissing=%t hasPhysical=%t path=%q",
		requestID,
		eventName,
		embyItemID,
		itemType,
		itemName,
		tmdbID,
		seriesID,
		season,
		episode,
		locationType,
		isMissing,
		physical,
		itemPath,
	)
	if !physical {
		log.Printf("[TV Calendar] Emby webhook 忽略：条目没有物理媒体 requestId=%s itemId=%q itemName=%q locationType=%q isMissing=%t path=%q", requestID, embyItemID, itemName, locationType, isMissing, itemPath)
		c.JSON(http.StatusOK, gin.H{"success": true, "ignored": true})
		return
	}

	var subscriptionUpdatedCount int64
	switch itemType {
	case "movie":
		subscriptionCount, err := h.subscriptionService.MarkSubscriptionsIngestedByWebhook(c.Request.Context(), subscriptionpkg.SubscriptionIngestWebhookPayload{
			ItemType:   itemType,
			TmdbID:     tmdbID,
			EmbyItemID: embyItemID,
			ItemName:   itemName,
		})
		if err != nil {
			log.Printf("[Subscription] Emby webhook 电影入库回写失败 requestId=%s itemId=%q itemName=%q tmdbId=%q err=%v", requestID, embyItemID, itemName, tmdbID, err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		log.Printf("[Subscription] Emby webhook 电影处理完成 requestId=%s itemId=%q itemName=%q tmdbId=%q updated=%t updatedCount=%d", requestID, embyItemID, itemName, tmdbID, subscriptionCount > 0, subscriptionCount)
		c.JSON(http.StatusOK, gin.H{
			"success":             true,
			"subscriptionUpdated": subscriptionCount > 0,
			"subscriptionCount":   subscriptionCount,
		})
		return
	case "episode":
		subscriptionCount, err := h.subscriptionService.MarkSubscriptionsIngestedByWebhook(c.Request.Context(), subscriptionpkg.SubscriptionIngestWebhookPayload{
			ItemType:   itemType,
			TmdbID:     tmdbID,
			Season:     season,
			Episode:    episode,
			EmbyItemID: embyItemID,
			ItemName:   itemName,
		})
		if err != nil {
			log.Printf("[Subscription] Emby webhook 剧集入库回写失败 requestId=%s itemId=%q itemName=%q tmdbId=%q season=%d episode=%d err=%v", requestID, embyItemID, itemName, tmdbID, season, episode, err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		subscriptionUpdatedCount = subscriptionCount
	default:
		log.Printf("[TV Calendar] Emby webhook 忽略：条目类型不支持 requestId=%s itemId=%q itemType=%q itemName=%q", requestID, embyItemID, itemType, itemName)
		c.JSON(http.StatusOK, gin.H{"success": true, "ignored": true})
		return
	}

	if season <= 0 || episode <= 0 {
		log.Printf("[TV Calendar] Emby webhook 忽略：季集号无效 requestId=%s itemId=%q itemName=%q season=%d episode=%d", requestID, embyItemID, itemName, season, episode)
		c.JSON(http.StatusOK, gin.H{
			"success":             true,
			"ignored":             true,
			"subscriptionUpdated": subscriptionUpdatedCount > 0,
			"subscriptionCount":   subscriptionUpdatedCount,
		})
		return
	}

	updatedCount, err := h.service.MarkEpisodeReadyByWebhook(c.Request.Context(), tmdbID, seriesID, season, episode, embyItemID)
	if err != nil {
		log.Printf("[TV Calendar] Emby webhook 更新失败 requestId=%s event=%q itemId=%q itemName=%q tmdbId=%q seriesId=%q season=%d episode=%d err=%v", requestID, eventName, embyItemID, itemName, tmdbID, seriesID, season, episode, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	log.Printf("[TV Calendar] Emby webhook 更新完成 requestId=%s event=%q itemId=%q itemName=%q tmdbId=%q seriesId=%q season=%d episode=%d updated=%t updatedCount=%d subscriptionUpdated=%t subscriptionCount=%d", requestID, eventName, embyItemID, itemName, tmdbID, seriesID, season, episode, updatedCount > 0, updatedCount, subscriptionUpdatedCount > 0, subscriptionUpdatedCount)

	c.JSON(http.StatusOK, gin.H{
		"success":             true,
		"updated":             updatedCount > 0,
		"count":               updatedCount,
		"subscriptionUpdated": subscriptionUpdatedCount > 0,
		"subscriptionCount":   subscriptionUpdatedCount,
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

func extractSeriesID(item map[string]interface{}) string {
	return extractString(item, "SeriesId", "seriesId", "SeriesID")
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
