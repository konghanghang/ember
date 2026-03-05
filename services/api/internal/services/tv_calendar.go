package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/konghang/ember/backend/internal/db"
	"github.com/konghang/ember/backend/internal/models"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	tmdbBaseURL              = "https://api.themoviedb.org/3"
	tmdbDetailCacheTTL       = 24 * time.Hour
	tmdbSeasonCacheTTL       = 24 * time.Hour
	tvCalendarRefreshMinTTL  = 6 * time.Hour
	tvCalendarFetchTimeout   = 15 * time.Second
	tvCalendarMaxSeasonCount = 200
)

type tmdbMemoryCacheEntry struct {
	Payload   []byte
	ExpiresAt time.Time
}

// TVCalendarService 追剧日历服务
type TVCalendarService struct {
	embyService *EmbyService
	tmdbAPIKey  string
	httpClient  *http.Client

	memoryCache map[string]tmdbMemoryCacheEntry
	cacheMu     sync.RWMutex
}

func NewTVCalendarService() *TVCalendarService {
	return &TVCalendarService{
		embyService: NewEmbyService(),
		tmdbAPIKey:  strings.TrimSpace(os.Getenv("TMDB_API_KEY")),
		httpClient: &http.Client{
			Timeout: tvCalendarFetchTimeout,
		},
		memoryCache: make(map[string]tmdbMemoryCacheEntry),
	}
}

type TVCalendarDTO struct {
	ID          string    `json:"id"`
	TmdbID      string    `json:"tmdbId"`
	Season      int       `json:"season"`
	Episode     int       `json:"episode"`
	AirDate     time.Time `json:"airDate"`
	EpisodeName string    `json:"episodeName"`
	Status      string    `json:"status"`
	EmbyItemID  string    `json:"embyItemId,omitempty"`
	ShowName    string    `json:"showName"`
	PosterURL   string    `json:"posterUrl,omitempty"`
}

type CreateTVCalendarSubscriptionRequest struct {
	TmdbID    string `json:"tmdbId" binding:"required"`
	ShowName  string `json:"showName" binding:"required"`
	PosterURL string `json:"posterUrl"`
}

type tmdbTVDetailResponse struct {
	ID              int    `json:"id"`
	Name            string `json:"name"`
	PosterPath      string `json:"poster_path"`
	NumberOfSeasons int    `json:"number_of_seasons"`
	Seasons         []struct {
		SeasonNumber int `json:"season_number"`
	} `json:"seasons"`
}

type tmdbSeasonResponse struct {
	Episodes []struct {
		EpisodeNumber int    `json:"episode_number"`
		Name          string `json:"name"`
		AirDate       string `json:"air_date"`
	} `json:"episodes"`
}

func isValidTVCalendarStatus(status string) bool {
	switch status {
	case "", models.TVCalendarStatusReady, models.TVCalendarStatusMissing, models.TVCalendarStatusUpcoming, models.TVCalendarStatusToday:
		return true
	default:
		return false
	}
}

func normalizeDateUTC(t time.Time) time.Time {
	y, m, d := t.UTC().Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

func deriveStatusByAirDate(airDate time.Time, now time.Time) string {
	today := normalizeDateUTC(now)
	date := normalizeDateUTC(airDate)
	if date.After(today) {
		return models.TVCalendarStatusUpcoming
	}
	if date.Equal(today) {
		return models.TVCalendarStatusToday
	}
	return models.TVCalendarStatusMissing
}

func parseDateOnly(date string) (time.Time, error) {
	t, err := time.Parse("2006-01-02", date)
	if err != nil {
		return time.Time{}, err
	}
	return normalizeDateUTC(t), nil
}

// FetchCalendar 获取用户追剧日历
func (s *TVCalendarService) FetchCalendar(ctx context.Context, userID string, startDate, endDate time.Time, status string) ([]TVCalendarDTO, error) {
	if !isValidTVCalendarStatus(status) {
		return nil, ErrTVCalendarInvalidStatus
	}

	var subscriptions []models.TVCalendarSubscription
	if err := db.DB.Where("\"userId\" = ?", userID).Find(&subscriptions).Error; err != nil {
		return nil, fmt.Errorf("查询追剧订阅失败: %w", err)
	}
	if len(subscriptions) == 0 {
		return []TVCalendarDTO{}, nil
	}

	tmdbIDs := make([]string, 0, len(subscriptions))
	subByTmdbID := make(map[string]models.TVCalendarSubscription, len(subscriptions))
	for _, sub := range subscriptions {
		tmdbIDs = append(tmdbIDs, sub.TmdbID)
		subByTmdbID[sub.TmdbID] = sub
	}

	if _, err := s.refreshByTMDBIDs(ctx, tmdbIDs, false); err != nil {
		return nil, err
	}

	start := normalizeDateUTC(startDate)
	end := normalizeDateUTC(endDate)

	query := db.DB.Model(&models.TVCalendarItem{}).
		Where("\"tmdbId\" IN ?", tmdbIDs).
		Where("\"airDate\" >= ? AND \"airDate\" <= ?", start, end)
	if status != "" {
		query = query.Where("status = ?", status)
	}

	var items []models.TVCalendarItem
	if err := query.Order("\"airDate\" ASC").Order("season ASC").Order("episode ASC").Find(&items).Error; err != nil {
		return nil, fmt.Errorf("查询追剧日历失败: %w", err)
	}

	result := make([]TVCalendarDTO, 0, len(items))
	for _, item := range items {
		sub := subByTmdbID[item.TmdbID]
		result = append(result, TVCalendarDTO{
			ID:          item.ID,
			TmdbID:      item.TmdbID,
			Season:      item.Season,
			Episode:     item.Episode,
			AirDate:     item.AirDate,
			EpisodeName: item.EpisodeName,
			Status:      item.Status,
			EmbyItemID:  item.EmbyItemID,
			ShowName:    sub.ShowName,
			PosterURL:   sub.PosterURL,
		})
	}

	return result, nil
}

// Subscribe 创建/更新用户追剧订阅
func (s *TVCalendarService) Subscribe(userID string, req CreateTVCalendarSubscriptionRequest) error {
	tmdbID := strings.TrimSpace(req.TmdbID)
	if tmdbID == "" {
		return ErrTVCalendarTMDBIDRequired
	}

	showName := strings.TrimSpace(req.ShowName)
	if showName == "" {
		return ErrTVCalendarShowNameNeeded
	}

	posterURL := strings.TrimSpace(req.PosterURL)

	var existing models.TVCalendarSubscription
	err := db.DB.Where("\"userId\" = ? AND \"tmdbId\" = ?", userID, tmdbID).First(&existing).Error
	if err == nil {
		existing.ShowName = showName
		existing.PosterURL = posterURL
		if saveErr := db.DB.Save(&existing).Error; saveErr != nil {
			return fmt.Errorf("更新追剧订阅失败: %w", saveErr)
		}
		return nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return fmt.Errorf("创建追剧订阅失败: %w", err)
	}

	subscription := models.TVCalendarSubscription{
		UserID:    userID,
		TmdbID:    tmdbID,
		ShowName:  showName,
		PosterURL: posterURL,
	}

	if err := db.DB.Create(&subscription).Error; err != nil {
		return fmt.Errorf("创建追剧订阅失败: %w", err)
	}

	return nil
}

// GetSubscriptions 获取用户追剧订阅
func (s *TVCalendarService) GetSubscriptions(userID string) ([]models.TVCalendarSubscription, error) {
	var subscriptions []models.TVCalendarSubscription
	if err := db.DB.Where("\"userId\" = ?", userID).Order("\"createdAt\" DESC").Find(&subscriptions).Error; err != nil {
		return nil, fmt.Errorf("查询追剧订阅失败: %w", err)
	}
	return subscriptions, nil
}

// Unsubscribe 取消订阅
func (s *TVCalendarService) Unsubscribe(userID, tmdbID string) error {
	tmdbID = strings.TrimSpace(tmdbID)
	if tmdbID == "" {
		return ErrTVCalendarTMDBIDRequired
	}

	result := db.DB.Where("\"userId\" = ? AND \"tmdbId\" = ?", userID, tmdbID).Delete(&models.TVCalendarSubscription{})
	if result.Error != nil {
		return fmt.Errorf("取消订阅失败: %w", result.Error)
	}

	return nil
}

// RefreshCalendar 管理员触发刷新
func (s *TVCalendarService) RefreshCalendar(ctx context.Context, tmdbID *string, force bool) (int, error) {
	if s.tmdbAPIKey == "" {
		return 0, ErrTVCalendarNotConfigured
	}

	if tmdbID != nil && strings.TrimSpace(*tmdbID) != "" {
		count, err := s.refreshByTMDBIDs(ctx, []string{strings.TrimSpace(*tmdbID)}, force)
		return count, err
	}

	var tmdbIDs []string
	if err := db.DB.Model(&models.TVCalendarSubscription{}).Distinct("\"tmdbId\"").Pluck("\"tmdbId\"", &tmdbIDs).Error; err != nil {
		return 0, fmt.Errorf("查询订阅剧集失败: %w", err)
	}
	if len(tmdbIDs) == 0 {
		return 0, nil
	}

	return s.refreshByTMDBIDs(ctx, tmdbIDs, force)
}

func (s *TVCalendarService) refreshByTMDBIDs(ctx context.Context, tmdbIDs []string, force bool) (int, error) {
	if s.tmdbAPIKey == "" {
		return 0, ErrTVCalendarNotConfigured
	}

	total := 0
	for _, raw := range tmdbIDs {
		tmdbID := strings.TrimSpace(raw)
		if tmdbID == "" {
			continue
		}
		count, err := s.refreshSingleTMDB(ctx, tmdbID, force)
		if err != nil {
			return total, err
		}
		total += count
	}
	return total, nil
}

func (s *TVCalendarService) refreshSingleTMDB(ctx context.Context, tmdbID string, force bool) (int, error) {
	now := time.Now().UTC()
	if !force {
		checkedAt, err := s.lastCheckedAt(tmdbID)
		if err != nil {
			return 0, err
		}
		if checkedAt != nil && now.Sub(*checkedAt) < tvCalendarRefreshMinTTL {
			return 0, nil
		}
	}

	detail, err := s.fetchTVDetail(ctx, tmdbID, force)
	if err != nil {
		return 0, err
	}

	seasonNumbers := make([]int, 0, len(detail.Seasons))
	for _, season := range detail.Seasons {
		if season.SeasonNumber > 0 {
			seasonNumbers = append(seasonNumbers, season.SeasonNumber)
		}
	}
	if len(seasonNumbers) == 0 {
		maxSeason := detail.NumberOfSeasons
		if maxSeason > tvCalendarMaxSeasonCount {
			maxSeason = tvCalendarMaxSeasonCount
		}
		for i := 1; i <= maxSeason; i++ {
			seasonNumbers = append(seasonNumbers, i)
		}
	}

	if len(seasonNumbers) == 0 {
		return 0, nil
	}

	affected := 0
	for _, seasonNumber := range seasonNumbers {
		season, err := s.fetchSeasonDetail(ctx, tmdbID, seasonNumber, force)
		if err != nil {
			return affected, err
		}
		for _, ep := range season.Episodes {
			if ep.EpisodeNumber <= 0 || strings.TrimSpace(ep.AirDate) == "" {
				continue
			}

			airDate, err := parseDateOnly(ep.AirDate)
			if err != nil {
				continue
			}

			item := models.TVCalendarItem{
				TmdbID:      tmdbID,
				Season:      seasonNumber,
				Episode:     ep.EpisodeNumber,
				AirDate:     airDate,
				EpisodeName: strings.TrimSpace(ep.Name),
				Status:      deriveStatusByAirDate(airDate, now),
				LastChecked: now,
			}

			result := db.DB.Clauses(clause.OnConflict{
				Columns: []clause.Column{{Name: "tmdbId"}, {Name: "season"}, {Name: "episode"}},
				DoUpdates: clause.Assignments(map[string]interface{}{
					"airDate":     item.AirDate,
					"episodeName": item.EpisodeName,
					"lastChecked": item.LastChecked,
					"status": gorm.Expr(
						`CASE WHEN status = ? THEN status ELSE EXCLUDED."status" END`,
						models.TVCalendarStatusReady,
					),
				}),
			}).Create(&item)
			if result.Error != nil {
				return affected, fmt.Errorf("写入追剧日历失败: %w", result.Error)
			}
			affected += int(result.RowsAffected)
		}
	}

	return affected, nil
}

func (s *TVCalendarService) lastCheckedAt(tmdbID string) (*time.Time, error) {
	var row struct {
		LastChecked *time.Time `gorm:"column:lastChecked"`
	}
	if err := db.DB.Model(&models.TVCalendarItem{}).
		Select("MAX(\"lastChecked\") AS \"lastChecked\"").
		Where("\"tmdbId\" = ?", tmdbID).
		Scan(&row).Error; err != nil {
		return nil, fmt.Errorf("查询日历缓存状态失败: %w", err)
	}
	return row.LastChecked, nil
}

func (s *TVCalendarService) fetchTVDetail(ctx context.Context, tmdbID string, force bool) (*tmdbTVDetailResponse, error) {
	endpoint := fmt.Sprintf("%s/tv/%s?api_key=%s&language=zh-CN", tmdbBaseURL, url.PathEscape(tmdbID), url.QueryEscape(s.tmdbAPIKey))
	cacheKey := fmt.Sprintf("tv-detail:%s", tmdbID)
	var resp tmdbTVDetailResponse
	if err := s.fetchTMDBJSON(ctx, cacheKey, endpoint, tmdbDetailCacheTTL, force, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

func (s *TVCalendarService) fetchSeasonDetail(ctx context.Context, tmdbID string, season int, force bool) (*tmdbSeasonResponse, error) {
	endpoint := fmt.Sprintf("%s/tv/%s/season/%d?api_key=%s&language=zh-CN", tmdbBaseURL, url.PathEscape(tmdbID), season, url.QueryEscape(s.tmdbAPIKey))
	cacheKey := fmt.Sprintf("tv-season:%s:%d", tmdbID, season)
	var resp tmdbSeasonResponse
	if err := s.fetchTMDBJSON(ctx, cacheKey, endpoint, tmdbSeasonCacheTTL, force, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

func (s *TVCalendarService) fetchTMDBJSON(ctx context.Context, cacheKey, requestURL string, ttl time.Duration, force bool, out interface{}) error {
	now := time.Now().UTC()
	if !force {
		if payload, ok := s.getTMDBMemoryCache(cacheKey, now); ok {
			if err := json.Unmarshal(payload, out); err == nil {
				return nil
			}
		}

		var cached models.TMDBCache
		err := db.DB.Where("\"cacheKey\" = ? AND \"expiresAt\" > ?", cacheKey, now).First(&cached).Error
		if err == nil {
			payload := []byte(cached.CacheValue)
			if decodeErr := json.Unmarshal(payload, out); decodeErr == nil {
				s.setTMDBMemoryCache(cacheKey, payload, cached.ExpiresAt)
				return nil
			}
		}
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL, nil)
	if err != nil {
		return fmt.Errorf("构建 TMDB 请求失败: %w", err)
	}

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("请求 TMDB 失败: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("读取 TMDB 响应失败: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("TMDB API 返回错误：HTTP %d", resp.StatusCode)
	}

	if err := json.Unmarshal(body, out); err != nil {
		return fmt.Errorf("解析 TMDB 响应失败: %w", err)
	}

	expiresAt := now.Add(ttl)
	s.setTMDBMemoryCache(cacheKey, body, expiresAt)

	cacheRow := models.TMDBCache{
		CacheKey:   cacheKey,
		CacheValue: string(body),
		ExpiresAt:  expiresAt,
	}
	if err := db.DB.Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "cacheKey"}},
		DoUpdates: clause.Assignments(map[string]interface{}{
			"cacheValue": cacheRow.CacheValue,
			"expiresAt":  cacheRow.ExpiresAt,
		}),
	}).Create(&cacheRow).Error; err != nil {
		return fmt.Errorf("写入 TMDB 缓存失败: %w", err)
	}

	return nil
}

func (s *TVCalendarService) getTMDBMemoryCache(cacheKey string, now time.Time) ([]byte, bool) {
	s.cacheMu.RLock()
	entry, ok := s.memoryCache[cacheKey]
	s.cacheMu.RUnlock()
	if !ok {
		return nil, false
	}
	if now.After(entry.ExpiresAt) {
		s.cacheMu.Lock()
		if current, exists := s.memoryCache[cacheKey]; exists && now.After(current.ExpiresAt) {
			delete(s.memoryCache, cacheKey)
		}
		s.cacheMu.Unlock()
		return nil, false
	}
	return entry.Payload, true
}

func (s *TVCalendarService) setTMDBMemoryCache(cacheKey string, payload []byte, expiresAt time.Time) {
	copyPayload := make([]byte, len(payload))
	copy(copyPayload, payload)
	s.cacheMu.Lock()
	s.memoryCache[cacheKey] = tmdbMemoryCacheEntry{Payload: copyPayload, ExpiresAt: expiresAt}
	s.cacheMu.Unlock()
}

// MarkEpisodeReadyByWebhook Webhook 点亮剧集状态
func (s *TVCalendarService) MarkEpisodeReadyByWebhook(ctx context.Context, tmdbID, seriesID string, season, episode int, embyItemID string) (int64, error) {
	if season <= 0 || episode <= 0 {
		return 0, nil
	}

	updates := map[string]interface{}{
		"status":      models.TVCalendarStatusReady,
		"lastChecked": time.Now().UTC(),
	}
	if strings.TrimSpace(embyItemID) != "" {
		updates["embyItemId"] = strings.TrimSpace(embyItemID)
	}
	if strings.TrimSpace(seriesID) != "" {
		updates["seriesId"] = strings.TrimSpace(seriesID)
	}

	var result *gorm.DB
	trimmedTmdbID := strings.TrimSpace(tmdbID)
	trimmedSeriesID := strings.TrimSpace(seriesID)
	if trimmedTmdbID != "" {
		result = db.DB.WithContext(ctx).Model(&models.TVCalendarItem{}).
			Where("\"tmdbId\" = ? AND season = ? AND episode = ?", trimmedTmdbID, season, episode).
			Updates(updates)
		if result.Error != nil {
			return 0, fmt.Errorf("更新剧集状态失败: %w", result.Error)
		}
		if result.RowsAffected > 0 || trimmedSeriesID == "" {
			return result.RowsAffected, nil
		}
	}

	if trimmedSeriesID == "" {
		return 0, nil
	}

	result = db.DB.WithContext(ctx).Model(&models.TVCalendarItem{}).
		Where("\"seriesId\" = ? AND season = ? AND episode = ?", trimmedSeriesID, season, episode).
		Updates(updates)
	if result.Error != nil {
		return 0, fmt.Errorf("更新剧集状态失败: %w", result.Error)
	}

	return result.RowsAffected, nil
}

func ParseTVCalendarDate(value string) (time.Time, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}, ErrTVCalendarInvalidDate
	}
	t, err := time.Parse("2006-01-02", value)
	if err != nil {
		return time.Time{}, ErrTVCalendarInvalidDate
	}
	return normalizeDateUTC(t), nil
}

func DefaultTVCalendarDateRange() (time.Time, time.Time) {
	now := normalizeDateUTC(time.Now().UTC())
	return now.AddDate(0, 0, -7), now.AddDate(0, 0, 30)
}
