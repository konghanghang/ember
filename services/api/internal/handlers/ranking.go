package handlers

import (
	"database/sql"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	configpkg "github.com/konghang/ember/backend/internal/config"
	"github.com/konghang/ember/backend/internal/db"
	"github.com/konghang/ember/backend/internal/models"
	"github.com/konghang/ember/backend/internal/services"
)

type RankingHandler struct {
	service *services.PlaybackRankingService
}

type RankingPreviewItem struct {
	Rank      int    `json:"rank"`
	ItemName  string `json:"itemName"`
	PlayCount int    `json:"playCount"`
	Duration  int64  `json:"duration"`
}

func NewRankingHandler() *RankingHandler {
	return &RankingHandler{
		service: services.NewPlaybackRankingService(),
	}
}

// GetLatestRanking 获取最新排行
// GET /api/v1/rankings/latest?period=daily&category=media_movie
func (h *RankingHandler) GetLatestRanking(c *gin.Context) {
	period := models.RankingPeriod(c.DefaultQuery("period", string(models.RankingDaily)))
	category := models.RankingCategory(c.DefaultQuery("category", string(models.RankingMediaMovie)))

	if period != models.RankingDaily && period != models.RankingWeekly {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的 period 参数"})
		return
	}
	if category != models.RankingMediaMovie && category != models.RankingMediaEpisode {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的 category 参数"})
		return
	}

	rankings, err := h.service.GetLatestRanking(period, category)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": rankings})
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

	res, err := h.service.PreviewRanking(period)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	movies := make([]RankingPreviewItem, 0, len(res.Movies))
	for _, r := range res.Movies {
		movies = append(movies, RankingPreviewItem{
			Rank:      r.Rank,
			ItemName:  r.ItemName,
			PlayCount: r.PlayCount,
			Duration:  r.Duration,
		})
	}
	episodes := make([]RankingPreviewItem, 0, len(res.Episodes))
	for _, r := range res.Episodes {
		episodes = append(episodes, RankingPreviewItem{
			Rank:      r.Rank,
			ItemName:  r.ItemName,
			PlayCount: r.PlayCount,
			Duration:  r.Duration,
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"period":      string(period),
		"snapshotAt":  res.ComputedAt.Format(time.RFC3339),
		"periodStart": res.Start.Format("2006-01-02"),
		"periodEnd":   res.End.Format("2006-01-02"),
		"cutoffAt":    res.End.Format("15:04"),
		"movies":      movies,
		"episodes":    episodes,
	})
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

// GetHistoryRanking 按日期查询某一天/某一周的排行快照（取该范围内 snapshot_at 最新的一次）
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

	var maxSnapshot sql.NullTime
	if err := db.DB.Model(&models.PlaybackRanking{}).
		Select("MAX(snapshot_at)").
		Where("period = ? AND snapshot_at >= ? AND snapshot_at < ?", period, rangeStart, rangeEnd).
		Scan(&maxSnapshot).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if !maxSnapshot.Valid {
		// 没有数据：返回空列表，但仍返回用户选中的范围，方便前端展示
		periodStart := rangeStart.Format("2006-01-02")
		periodEnd := rangeStart.Format("2006-01-02")
		if period == models.RankingWeekly {
			periodEnd = rangeStart.AddDate(0, 0, 6).Format("2006-01-02")
		}

		c.JSON(http.StatusOK, gin.H{
			"period":      string(period),
			"snapshotAt":  "",
			"periodStart": periodStart,
			"periodEnd":   periodEnd,
			"cutoffAt":    "",
			"movies":      []RankingPreviewItem{},
			"episodes":    []RankingPreviewItem{},
		})
		return
	}

	snapshotAt := maxSnapshot.Time
	movieRows, episodeRows, err := h.service.GetRankingBySnapshot(period, snapshotAt)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	meta := (*models.PlaybackRanking)(nil)
	if len(movieRows) > 0 {
		meta = &movieRows[0]
	} else if len(episodeRows) > 0 {
		meta = &episodeRows[0]
	}

	periodStart := rangeStart.Format("2006-01-02")
	periodEnd := rangeStart.Format("2006-01-02")
	cutoffAt := ""
	if period == models.RankingWeekly {
		periodEnd = rangeStart.AddDate(0, 0, 6).Format("2006-01-02")
	}
	if meta != nil {
		periodStart = meta.PeriodStart.In(tz).Format("2006-01-02")
		periodEnd = meta.PeriodEnd.In(tz).Format("2006-01-02")
		cutoffAt = meta.PeriodEnd.In(tz).Format("15:04")
	}

	movies := make([]RankingPreviewItem, 0, len(movieRows))
	for _, r := range movieRows {
		movies = append(movies, RankingPreviewItem{
			Rank:      r.Rank,
			ItemName:  r.ItemName,
			PlayCount: r.PlayCount,
			Duration:  r.Duration,
		})
	}
	episodes := make([]RankingPreviewItem, 0, len(episodeRows))
	for _, r := range episodeRows {
		episodes = append(episodes, RankingPreviewItem{
			Rank:      r.Rank,
			ItemName:  r.ItemName,
			PlayCount: r.PlayCount,
			Duration:  r.Duration,
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"period":      string(period),
		"snapshotAt":  snapshotAt.Format(time.RFC3339),
		"periodStart": periodStart,
		"periodEnd":   periodEnd,
		"cutoffAt":    cutoffAt,
		"movies":      movies,
		"episodes":    episodes,
	})
}
