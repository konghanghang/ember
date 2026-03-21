package handlers

import (
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	configpkg "github.com/konghang/ember/backend/internal/config"
	"github.com/konghang/ember/backend/internal/models"
	"github.com/konghang/ember/backend/internal/services"
)

type RankingHandler struct {
	service *services.PlaybackRankingService
}

type RankingItemResponse struct {
	Rank      int    `json:"rank"`
	ItemKey   string `json:"itemKey"`
	ItemName  string `json:"itemName"`
	PlayCount int    `json:"playCount"`
	Duration  int64  `json:"duration"`
}

type RankingResponse struct {
	Period      string                `json:"period"`
	BatchID     string                `json:"batchId,omitempty"`
	SnapshotAt  string                `json:"snapshotAt"`
	PeriodStart string                `json:"periodStart"`
	PeriodEnd   string                `json:"periodEnd"`
	CutoffAt    string                `json:"cutoffAt"`
	Movies      []RankingItemResponse `json:"movies"`
	Episodes    []RankingItemResponse `json:"episodes"`
}

func NewRankingHandler() *RankingHandler {
	return &RankingHandler{
		service: services.NewPlaybackRankingService(),
	}
}

// GetLatestRanking 获取最新排行
// GET /api/v1/rankings/latest?period=daily
func (h *RankingHandler) GetLatestRanking(c *gin.Context) {
	period := models.RankingPeriod(c.DefaultQuery("period", string(models.RankingDaily)))

	if period != models.RankingDaily && period != models.RankingWeekly {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的 period 参数"})
		return
	}

	result, err := h.service.GetLatestRanking(period)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, buildRankingResponse(period, result, loadTimezone()))
}

func loadTimezone() *time.Location {
	return configpkg.LoadConfiguredTimezone()
}

// GenerateRanking 手动触发排行榜生成
// POST /api/v1/admin/cron/generate-ranking?type=daily
// POST /api/v1/admin/cron/generate-ranking?type=daily&start=2026-02-10&end=2026-02-13
func (h *RankingHandler) GenerateRanking(c *gin.Context) {
	rankType := c.DefaultQuery("type", "daily")

	var period models.RankingPeriod
	switch rankType {
	case "daily":
		period = models.RankingDaily
	case "weekly":
		period = models.RankingWeekly
	default:
		c.JSON(http.StatusBadRequest, gin.H{"error": "type 参数无效，请用 daily 或 weekly"})
		return
	}

	startStr := c.Query("start")
	endStr := c.Query("end")

	var startPtr, endPtr *time.Time
	if startStr != "" || endStr != "" {
		if startStr == "" || endStr == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "start/end 必须同时传入"})
			return
		}

		tz := loadTimezone()
		startDate, err := time.ParseInLocation("2006-01-02", startStr, tz)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "start 格式无效，请用 YYYY-MM-DD"})
			return
		}
		endDate, err := time.ParseInLocation("2006-01-02", endStr, tz)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "end 格式无效，请用 YYYY-MM-DD"})
			return
		}

		startTime := time.Date(startDate.Year(), startDate.Month(), startDate.Day(), 0, 0, 0, 0, tz)
		endTime := time.Date(endDate.Year(), endDate.Month(), endDate.Day(), 23, 59, 59, 0, tz)
		startPtr = &startTime
		endPtr = &endTime
	}

	if err := h.service.GenerateRanking(period, startPtr, endPtr); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": fmt.Sprintf("%s 排行榜生成完成", rankType),
	})
}

// PreviewRanking 预览生成排行榜（不入库、不推送）
// POST /api/v1/admin/rankings/preview?type=daily
func (h *RankingHandler) PreviewRanking(c *gin.Context) {
	rankType := c.DefaultQuery("type", "daily")

	var period models.RankingPeriod
	switch rankType {
	case "daily":
		period = models.RankingDaily
	case "weekly":
		period = models.RankingWeekly
	default:
		c.JSON(http.StatusBadRequest, gin.H{"error": "type 参数无效，请用 daily 或 weekly"})
		return
	}

	result, err := h.service.PreviewRanking(period)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, buildRankingResponse(period, result, loadTimezone()))
}

func dateRangeByPeriod(tz *time.Location, period models.RankingPeriod, date time.Time) (time.Time, time.Time, error) {
	d := date.In(tz)

	switch period {
	case models.RankingDaily:
		start := time.Date(d.Year(), d.Month(), d.Day(), 0, 0, 0, 0, tz)
		end := start.AddDate(0, 0, 1)
		return start, end, nil
	case models.RankingWeekly:
		weekday := int(d.Weekday()) // Sunday=0
		daysSinceMonday := weekday - 1
		if weekday == 0 {
			daysSinceMonday = 6
		}
		monday := d.AddDate(0, 0, -daysSinceMonday)
		start := time.Date(monday.Year(), monday.Month(), monday.Day(), 0, 0, 0, 0, tz)
		end := start.AddDate(0, 0, 7)
		return start, end, nil
	default:
		return time.Time{}, time.Time{}, fmt.Errorf("无效的 period: %s", period)
	}
}

// GetHistoryRanking 按日期查询某一天/某一周的排行快照
// GET /api/v1/rankings/history?period=daily&date=2026-02-15
func (h *RankingHandler) GetHistoryRanking(c *gin.Context) {
	period := models.RankingPeriod(c.DefaultQuery("period", string(models.RankingDaily)))
	dateStr := c.Query("date")
	if dateStr == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "缺少 date 参数"})
		return
	}

	if period != models.RankingDaily && period != models.RankingWeekly {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的 period 参数"})
		return
	}

	tz := loadTimezone()
	date, err := time.ParseInLocation("2006-01-02", dateStr, tz)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "date 格式无效，请用 YYYY-MM-DD"})
		return
	}

	rangeStart, rangeEnd, err := dateRangeByPeriod(tz, period, date)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	result, err := h.service.GetHistoryRanking(period, rangeStart, rangeEnd)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	response := buildRankingResponse(period, result, tz)
	if result == nil {
		response.PeriodStart = rangeStart.Format("2006-01-02")
		if period == models.RankingWeekly {
			response.PeriodEnd = rangeStart.AddDate(0, 0, 6).Format("2006-01-02")
		} else {
			response.PeriodEnd = rangeStart.Format("2006-01-02")
		}
	}

	c.JSON(http.StatusOK, response)
}

func buildRankingResponse(period models.RankingPeriod, result *services.RankingResult, tz *time.Location) RankingResponse {
	if result == nil {
		return RankingResponse{
			Period:   string(period),
			Movies:   []RankingItemResponse{},
			Episodes: []RankingItemResponse{},
		}
	}

	return RankingResponse{
		Period:      string(result.Period),
		BatchID:     result.BatchID,
		SnapshotAt:  formatRFC3339(result.SnapshotAt),
		PeriodStart: formatDateInLocation(result.PeriodStart, tz),
		PeriodEnd:   formatDateInLocation(result.PeriodEnd, tz),
		CutoffAt:    formatClockInLocation(result.PeriodEnd, tz),
		Movies:      buildRankingItemsResponse(result.Movies),
		Episodes:    buildRankingItemsResponse(result.Episodes),
	}
}

func buildRankingItemsResponse(items []services.RankingResultItem) []RankingItemResponse {
	out := make([]RankingItemResponse, 0, len(items))
	for _, item := range items {
		out = append(out, RankingItemResponse{
			Rank:      item.Rank,
			ItemKey:   item.ItemKey,
			ItemName:  item.ItemName,
			PlayCount: item.PlayCount,
			Duration:  item.Duration,
		})
	}
	return out
}

func formatRFC3339(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.Format(time.RFC3339)
}

func formatDateInLocation(value time.Time, tz *time.Location) string {
	if value.IsZero() {
		return ""
	}
	return value.In(tz).Format("2006-01-02")
}

func formatClockInLocation(value time.Time, tz *time.Location) string {
	if value.IsZero() {
		return ""
	}
	return value.In(tz).Format("15:04")
}
