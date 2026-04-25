package tvcalendar

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/konghang/ember/backend/internal/common/upstream"
	configpkg "github.com/konghang/ember/backend/internal/config"
	"github.com/konghang/ember/backend/internal/db"
	embyint "github.com/konghang/ember/backend/internal/integrations/emby"
	"github.com/konghang/ember/backend/internal/models"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	tmdbBaseURL                      = "https://api.themoviedb.org/3"
	tmdbImageBaseURL                 = "https://image.tmdb.org/t/p/w500"
	tmdbDetailCacheTTL               = 24 * time.Hour
	tmdbSeasonCacheTTL               = 24 * time.Hour
	tvCalendarReadyEpisodesCacheTTL  = 2 * time.Minute
	tvCalendarFetchTimeout           = 15 * time.Second
	tvCalendarSyncPageSize           = 200
	tvCalendarActiveSourceWindowDays = 30
	tvCalendarDefaultOffset          = 0
	tvCalendarDefaultTimezone        = "Asia/Shanghai"
	tvCalendarReadyFetchConcurrency  = 4
)

var defaultTVCalendarWeekOffsets = []int{0, 1}

type tvCalendarSyncCall struct {
	done  chan struct{}
	count int
	err   error
}

var tvCalendarSyncCoordinator = struct {
	mu    sync.Mutex
	calls map[string]*tvCalendarSyncCall
}{
	calls: make(map[string]*tvCalendarSyncCall),
}

func tvCalendarWeekSyncLockKey(weekStart time.Time, tmdbID *string, force bool) string {
	scope := "all"
	if tmdbID != nil && strings.TrimSpace(*tmdbID) != "" {
		scope = "tmdb:" + strings.TrimSpace(*tmdbID)
	}
	mode := "cached"
	if force {
		mode = "force"
	}
	return fmt.Sprintf(
		"tv-calendar:sync:%s:%s:%s",
		normalizeTVCalendarWeekStart(weekStart).Format("2006-01-02"),
		scope,
		mode,
	)
}

func tvCalendarDiscoverLockKey(force bool) string {
	if force {
		return "tv-calendar:discover:force"
	}
	return "tv-calendar:discover"
}

func runTVCalendarSyncOnce(key string, fn func() (int, error)) (int, error) {
	tvCalendarSyncCoordinator.mu.Lock()
	if existing, ok := tvCalendarSyncCoordinator.calls[key]; ok {
		tvCalendarSyncCoordinator.mu.Unlock()
		<-existing.done
		return existing.count, existing.err
	}

	call := &tvCalendarSyncCall{
		done: make(chan struct{}),
	}
	tvCalendarSyncCoordinator.calls[key] = call
	tvCalendarSyncCoordinator.mu.Unlock()

	call.count, call.err = fn()
	close(call.done)

	tvCalendarSyncCoordinator.mu.Lock()
	delete(tvCalendarSyncCoordinator.calls, key)
	tvCalendarSyncCoordinator.mu.Unlock()

	return call.count, call.err
}

type tmdbMemoryCacheEntry struct {
	Payload   []byte
	ExpiresAt time.Time
}

type readyEpisodeCacheEntry struct {
	Episodes        map[string]embyEpisodeItem
	LatestCreatedAt *time.Time
	Validated       bool
	ExpiresAt       time.Time
}

// TVCalendarService 追剧日历服务
type TVCalendarService struct {
	embyService *embyint.EmbyService
	tmdbAPIKey  string
	httpClient  *http.Client

	memoryCache       map[string]tmdbMemoryCacheEntry
	readyEpisodeCache map[string]readyEpisodeCacheEntry
	cacheMu           sync.RWMutex
	readyCacheMu      sync.RWMutex
}

func NewTVCalendarService() *TVCalendarService {
	service := &TVCalendarService{
		embyService: embyint.NewEmbyService(),
		httpClient: &http.Client{
			Timeout: tvCalendarFetchTimeout,
		},
		memoryCache:       make(map[string]tmdbMemoryCacheEntry),
		readyEpisodeCache: make(map[string]readyEpisodeCacheEntry),
	}
	service.refreshConfig()
	return service
}

func (s *TVCalendarService) refreshConfig() {
	configService := configpkg.NewConfigService()
	s.tmdbAPIKey = strings.TrimSpace(configService.GetString("TMDB_API_KEY"))
}

func (s *TVCalendarService) SyncAvailable() bool {
	s.refreshConfig()
	return strings.TrimSpace(s.tmdbAPIKey) != "" &&
		s.embyService.IsConfigured()
}

func DefaultTVCalendarWeekOffsets() []int {
	offsets := make([]int, len(defaultTVCalendarWeekOffsets))
	copy(offsets, defaultTVCalendarWeekOffsets)
	return offsets
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

type TVCalendarWeeklyDTO struct {
	DateRange string             `json:"dateRange"`
	Days      []TVCalendarDayDTO `json:"days"`
}

type TVCalendarDayDTO struct {
	Date      string                 `json:"date"`
	WeekdayCn string                 `json:"weekdayCn"`
	IsToday   bool                   `json:"isToday"`
	Items     []TVCalendarWeeklyItem `json:"items"`
}

type TVCalendarWeeklyItem struct {
	TmdbID      string `json:"tmdbId"`
	SeriesID    string `json:"seriesId,omitempty"`
	ShowName    string `json:"showName"`
	PosterURL   string `json:"posterUrl,omitempty"`
	Season      int    `json:"season"`
	Episode     string `json:"episode"`
	AirDate     string `json:"airDate"`
	Status      string `json:"status"`
	EpisodeName string `json:"episodeName,omitempty"`
	Overview    string `json:"overview,omitempty"`
}

type CreateTVCalendarSubscriptionRequest struct {
	TmdbID    string `json:"tmdbId" binding:"required"`
	ShowName  string `json:"showName" binding:"required"`
	PosterURL string `json:"posterUrl"`
}

type tmdbEpisodeReference struct {
	SeasonNumber int `json:"season_number"`
}

type tmdbTVDetailResponse struct {
	ID              int    `json:"id"`
	Name            string `json:"name"`
	Overview        string `json:"overview"`
	PosterPath      string `json:"poster_path"`
	NumberOfSeasons int    `json:"number_of_seasons"`
	Seasons         []struct {
		SeasonNumber int `json:"season_number"`
	} `json:"seasons"`
	LastEpisodeToAir *tmdbEpisodeReference `json:"last_episode_to_air"`
	NextEpisodeToAir *tmdbEpisodeReference `json:"next_episode_to_air"`
}

type tmdbSeasonResponse struct {
	Episodes []struct {
		EpisodeNumber int    `json:"episode_number"`
		Name          string `json:"name"`
		AirDate       string `json:"air_date"`
		Overview      string `json:"overview"`
	} `json:"episodes"`
}

type embyPagedItemsResponse struct {
	Items            []embySeriesItem `json:"Items"`
	TotalRecordCount int              `json:"TotalRecordCount"`
}

type embySeriesItem struct {
	ID           string            `json:"Id"`
	Name         string            `json:"Name"`
	Overview     string            `json:"Overview"`
	Status       string            `json:"Status"`
	SeriesStatus string            `json:"SeriesStatus"`
	LocationType string            `json:"LocationType"`
	Path         string            `json:"Path"`
	ProviderIDs  map[string]string `json:"ProviderIds"`
}

type embyEpisodeItemsResponse struct {
	Items            []embyEpisodeItem `json:"Items"`
	TotalRecordCount int               `json:"TotalRecordCount"`
}

type embyEpisodeItem struct {
	ID                string                    `json:"Id"`
	SeriesID          string                    `json:"SeriesId"`
	ParentIndexNumber int                       `json:"ParentIndexNumber"`
	IndexNumber       int                       `json:"IndexNumber"`
	DateCreated       string                    `json:"DateCreated"`
	Path              string                    `json:"Path"`
	LocationType      string                    `json:"LocationType"`
	IsMissing         bool                      `json:"IsMissing"`
	MediaSources      []embyint.EmbyMediaSource `json:"MediaSources"`
}

type weeklyAggregate struct {
	TmdbID       string
	SeriesID     string
	ShowName     string
	PosterURL    string
	Season       int
	StartEpisode int
	EndEpisode   int
	AirDate      string
	Status       string
	EpisodeName  string
	Overview     string
}

type weeklySourceMeta struct {
	ShowName  string
	PosterURL string
}

func isValidTVCalendarStatus(status string) bool {
	switch status {
	case "", models.TVCalendarStatusReady, models.TVCalendarStatusMissing, models.TVCalendarStatusUpcoming, models.TVCalendarStatusToday:
		return true
	default:
		return false
	}
}

func isValidTVCalendarWeekOffset(offset int) bool {
	return offset >= -1 && offset <= 1
}

func normalizeDateUTC(t time.Time) time.Time {
	y, m, d := t.UTC().Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

func loadTVCalendarLocation() *time.Location {
	configService := configpkg.NewConfigService()
	tzName := strings.TrimSpace(configService.GetString("CRON_TIMEZONE"))
	if tzName == "" {
		tzName = tvCalendarDefaultTimezone
	}

	loc, err := time.LoadLocation(tzName)
	if err == nil {
		return loc
	}

	log.Printf("[TV Calendar] 时区 %q 无效，回退到 %s: %v", tzName, tvCalendarDefaultTimezone, err)
	defaultLocation, defaultErr := time.LoadLocation(tvCalendarDefaultTimezone)
	if defaultErr == nil {
		return defaultLocation
	}

	log.Printf("[TV Calendar] 默认时区 %q 加载失败，回退 UTC: %v", tvCalendarDefaultTimezone, defaultErr)
	return time.UTC
}

func currentCalendarDateInLocation(now time.Time, loc *time.Location) time.Time {
	if loc == nil {
		loc = time.UTC
	}
	y, m, d := now.In(loc).Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

func normalizeTVCalendarWeekStartInLocation(t time.Time, loc *time.Location) time.Time {
	if loc == nil {
		loc = time.UTC
	}
	current := normalizeDateUTC(t)
	y, m, d := current.UTC().Date()
	currentLocal := time.Date(y, m, d, 0, 0, 0, 0, loc)
	weekday := int(currentLocal.Weekday())
	if weekday == 0 {
		weekday = 7
	}
	weekStartLocal := currentLocal.AddDate(0, 0, -(weekday - 1))
	startYear, startMonth, startDay := weekStartLocal.Date()
	return time.Date(startYear, startMonth, startDay, 0, 0, 0, 0, time.UTC)
}

func normalizeTVCalendarWeekStart(t time.Time) time.Time {
	return normalizeTVCalendarWeekStartInLocation(t, loadTVCalendarLocation())
}

func tvCalendarWeekEnd(weekStart time.Time) time.Time {
	return normalizeDateUTC(weekStart).AddDate(0, 0, 6)
}

func isDateWithinTVCalendarWeek(date, weekStart time.Time) bool {
	normalizedDate := normalizeDateUTC(date)
	normalizedWeekStart := normalizeDateUTC(weekStart)
	weekEnd := tvCalendarWeekEnd(normalizedWeekStart)
	return !normalizedDate.Before(normalizedWeekStart) && !normalizedDate.After(weekEnd)
}

func deriveStatusByAirDate(airDate time.Time, now time.Time, loc *time.Location) string {
	today := currentCalendarDateInLocation(now, loc)
	date := normalizeDateUTC(airDate)
	if date.After(today) {
		return models.TVCalendarStatusUpcoming
	}
	if date.Equal(today) {
		return models.TVCalendarStatusToday
	}
	return models.TVCalendarStatusMissing
}

func resolveCalendarItemStatus(item models.TVCalendarItem, now time.Time, loc *time.Location) string {
	if item.Status == models.TVCalendarStatusReady {
		return models.TVCalendarStatusReady
	}
	return deriveStatusByAirDate(item.AirDate, now, loc)
}

func parseDateOnly(date string) (time.Time, error) {
	t, err := time.Parse("2006-01-02", date)
	if err != nil {
		return time.Time{}, err
	}
	return normalizeDateUTC(t), nil
}

func parseEmbyDateTime(value string) (time.Time, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}, false
	}

	for _, layout := range []string{time.RFC3339Nano, time.RFC3339} {
		if parsed, err := time.Parse(layout, value); err == nil {
			return parsed.UTC(), true
		}
	}

	return time.Time{}, false
}

func activeSourceCutoff(now time.Time) time.Time {
	return now.UTC().AddDate(0, 0, -tvCalendarActiveSourceWindowDays)
}

func shouldSyncSourceIncrementally(source models.TVCalendarSource, cutoff time.Time) bool {
	if source.LastEpisodeIngestedAt != nil {
		return !source.LastEpisodeIngestedAt.Before(cutoff)
	}
	return source.LastSyncedAt == nil
}

func ParseTVCalendarWeekOffset(value string) (int, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return tvCalendarDefaultOffset, nil
	}
	offset, err := strconv.Atoi(value)
	if err != nil || !isValidTVCalendarWeekOffset(offset) {
		return 0, ErrTVCalendarInvalidWeekOffset
	}
	return offset, nil
}

func ParseTVCalendarWeekDate(weekDateValue, weekOffsetValue string) (time.Time, error) {
	loc := loadTVCalendarLocation()
	if raw := strings.TrimSpace(weekDateValue); raw != "" {
		date, err := ParseTVCalendarDate(raw)
		if err != nil {
			return time.Time{}, err
		}
		return normalizeTVCalendarWeekStartInLocation(date, loc), nil
	}

	offset, err := ParseTVCalendarWeekOffset(weekOffsetValue)
	if err != nil {
		return time.Time{}, err
	}
	return tvCalendarWeekStartFromOffset(offset, time.Now().UTC(), loc), nil
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
	now := currentCalendarDateInLocation(time.Now().UTC(), loadTVCalendarLocation())
	return now.AddDate(0, 0, -7), now.AddDate(0, 0, 30)
}

func tvCalendarWeekStartFromOffset(offset int, now time.Time, loc *time.Location) time.Time {
	return normalizeTVCalendarWeekStartInLocation(now, loc).AddDate(0, 0, offset*7)
}

func tvCalendarWeekRange(weekStart time.Time) (time.Time, time.Time) {
	start := normalizeTVCalendarWeekStart(weekStart)
	end := start.AddDate(0, 0, 6)
	return start, end
}

func formatTVCalendarDateRange(start, end time.Time) string {
	return fmt.Sprintf("%02d/%02d - %02d/%02d", start.Month(), start.Day(), end.Month(), end.Day())
}

func tvCalendarWeekdayCn(t time.Time) string {
	switch t.Weekday() {
	case time.Monday:
		return "周一"
	case time.Tuesday:
		return "周二"
	case time.Wednesday:
		return "周三"
	case time.Thursday:
		return "周四"
	case time.Friday:
		return "周五"
	case time.Saturday:
		return "周六"
	default:
		return "周日"
	}
}

func tvCalendarStatusSortWeight(status string) int {
	switch status {
	case models.TVCalendarStatusReady:
		return 0
	case models.TVCalendarStatusToday:
		return 1
	case models.TVCalendarStatusMissing:
		return 2
	default:
		return 3
	}
}

func buildTMDBPosterURL(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	return tmdbImageBaseURL + path
}

func buildEpisodeKey(season, episode int) string {
	return fmt.Sprintf("%d:%d", season, episode)
}

func extractProviderID(providerIDs map[string]string, key string) string {
	if len(providerIDs) == 0 {
		return ""
	}
	for providerKey, providerValue := range providerIDs {
		if strings.EqualFold(strings.TrimSpace(providerKey), key) {
			return strings.TrimSpace(providerValue)
		}
	}
	return ""
}

func hasPhysicalEpisodeMedia(item embyEpisodeItem) bool {
	if strings.EqualFold(strings.TrimSpace(item.LocationType), "Virtual") {
		return false
	}
	if item.IsMissing {
		return false
	}
	if strings.TrimSpace(item.Path) != "" {
		return true
	}
	return len(item.MediaSources) > 0
}

func pickTargetSeasonNumbers(detail *tmdbTVDetailResponse) []int {
	if detail == nil {
		return nil
	}

	seen := make(map[int]struct{})
	seasonNumbers := make([]int, 0, 3)
	appendSeason := func(number int) {
		if number <= 0 {
			return
		}
		if _, exists := seen[number]; exists {
			return
		}
		seen[number] = struct{}{}
		seasonNumbers = append(seasonNumbers, number)
	}

	if detail.LastEpisodeToAir != nil {
		appendSeason(detail.LastEpisodeToAir.SeasonNumber)
	}
	if detail.NextEpisodeToAir != nil {
		appendSeason(detail.NextEpisodeToAir.SeasonNumber)
	}
	if len(seasonNumbers) == 0 {
		for i := len(detail.Seasons) - 1; i >= 0; i-- {
			appendSeason(detail.Seasons[i].SeasonNumber)
			if len(seasonNumbers) > 0 {
				break
			}
		}
	}
	if len(seasonNumbers) == 0 {
		appendSeason(detail.NumberOfSeasons)
	}

	sort.Ints(seasonNumbers)
	return seasonNumbers
}

func normalizeWeekOffsets(offsets []int) ([]int, error) {
	if len(offsets) == 0 {
		return DefaultTVCalendarWeekOffsets(), nil
	}

	seen := make(map[int]struct{}, len(offsets))
	result := make([]int, 0, len(offsets))
	for _, offset := range offsets {
		if !isValidTVCalendarWeekOffset(offset) {
			return nil, ErrTVCalendarInvalidWeekOffset
		}
		if _, exists := seen[offset]; exists {
			continue
		}
		seen[offset] = struct{}{}
		result = append(result, offset)
	}

	sort.Ints(result)
	return result, nil
}

func weekStartsFromOffsets(offsets []int, now time.Time, loc *time.Location) []time.Time {
	result := make([]time.Time, 0, len(offsets))
	for _, offset := range offsets {
		result = append(result, tvCalendarWeekStartFromOffset(offset, now, loc))
	}
	return result
}

func (s *TVCalendarService) buildEmptyWeeklyDTO(start, end time.Time, loc *time.Location) *TVCalendarWeeklyDTO {
	today := currentCalendarDateInLocation(time.Now().UTC(), loc)
	days := make([]TVCalendarDayDTO, 0, 7)
	for cursor := start; !cursor.After(end); cursor = cursor.AddDate(0, 0, 1) {
		days = append(days, TVCalendarDayDTO{
			Date:      cursor.Format("2006-01-02"),
			WeekdayCn: tvCalendarWeekdayCn(cursor),
			IsToday:   cursor.Equal(today),
			Items:     []TVCalendarWeeklyItem{},
		})
	}
	return &TVCalendarWeeklyDTO{
		DateRange: formatTVCalendarDateRange(start, end),
		Days:      days,
	}
}

type readyCorrection struct {
	TmdbID     string
	SeriesID   string
	Season     int
	Episode    int
	EmbyItemID string
}

func applyReadyEpisodeCorrections(
	items []models.TVCalendarItem,
	readyEpisodesBySeries map[string]map[string]embyEpisodeItem,
	seriesIDByTmdb map[string]string,
	currentWeekStart time.Time,
) ([]models.TVCalendarItem, []readyCorrection) {
	if len(items) == 0 {
		return items, nil
	}

	correctedItems := make([]models.TVCalendarItem, len(items))
	copy(correctedItems, items)
	corrections := make([]readyCorrection, 0)

	for idx, item := range correctedItems {
		if item.Status == models.TVCalendarStatusReady || !isDateWithinTVCalendarWeek(item.AirDate, currentWeekStart) {
			continue
		}

		seriesID := strings.TrimSpace(item.SeriesID)
		if seriesID == "" {
			seriesID = strings.TrimSpace(seriesIDByTmdb[item.TmdbID])
		}
		if seriesID == "" {
			continue
		}

		readyEpisodes, ok := readyEpisodesBySeries[seriesID]
		if !ok {
			continue
		}
		readyEpisode, exists := readyEpisodes[buildEpisodeKey(item.Season, item.Episode)]
		if !exists {
			continue
		}

		correctedItems[idx].Status = models.TVCalendarStatusReady
		correctedItems[idx].EmbyItemID = strings.TrimSpace(readyEpisode.ID)
		if strings.TrimSpace(correctedItems[idx].SeriesID) == "" {
			correctedItems[idx].SeriesID = strings.TrimSpace(readyEpisode.SeriesID)
		}
		corrections = append(corrections, readyCorrection{
			TmdbID:     item.TmdbID,
			SeriesID:   correctedItems[idx].SeriesID,
			Season:     item.Season,
			Episode:    item.Episode,
			EmbyItemID: correctedItems[idx].EmbyItemID,
		})
	}

	return correctedItems, corrections
}

func (s *TVCalendarService) correctCurrentWeekReadyItems(
	ctx context.Context,
	items []models.TVCalendarItem,
	start time.Time,
	end time.Time,
	loc *time.Location,
) []models.TVCalendarItem {
	if len(items) == 0 {
		return items
	}

	currentWeekStart := tvCalendarWeekStartFromOffset(0, time.Now().UTC(), loc)
	currentWeekEnd := tvCalendarWeekEnd(currentWeekStart)
	if end.Before(currentWeekStart) || start.After(currentWeekEnd) {
		return items
	}

	candidateTmdbIDs := make([]string, 0)
	seenTmdbIDs := make(map[string]struct{})
	candidateSeriesIDs := make(map[string]struct{})
	for _, item := range items {
		if item.Status == models.TVCalendarStatusReady || !isDateWithinTVCalendarWeek(item.AirDate, currentWeekStart) {
			continue
		}
		if tmdbID := strings.TrimSpace(item.TmdbID); tmdbID != "" {
			if _, exists := seenTmdbIDs[tmdbID]; !exists {
				seenTmdbIDs[tmdbID] = struct{}{}
				candidateTmdbIDs = append(candidateTmdbIDs, tmdbID)
			}
		}
		if seriesID := strings.TrimSpace(item.SeriesID); seriesID != "" {
			candidateSeriesIDs[seriesID] = struct{}{}
		}
	}
	if len(candidateTmdbIDs) == 0 {
		return items
	}

	sourceMap, err := s.loadSourceMap(candidateTmdbIDs)
	if err != nil {
		log.Printf("[TV Calendar] 当前周 ready 校正跳过：加载追剧源失败: %v", err)
		return items
	}

	seriesIDByTmdb := make(map[string]string, len(sourceMap))
	for tmdbID, source := range sourceMap {
		seriesID := strings.TrimSpace(source.SeriesID)
		if seriesID == "" {
			continue
		}
		seriesIDByTmdb[tmdbID] = seriesID
		candidateSeriesIDs[seriesID] = struct{}{}
	}
	if len(candidateSeriesIDs) == 0 {
		return items
	}

	readyEpisodesBySeries := s.loadReadyEpisodesBySeries(ctx, candidateSeriesIDs)
	if len(readyEpisodesBySeries) == 0 {
		return items
	}

	correctedItems, corrections := applyReadyEpisodeCorrections(items, readyEpisodesBySeries, seriesIDByTmdb, currentWeekStart)
	if len(corrections) == 0 {
		return items
	}

	checkedAt := time.Now().UTC()
	persisted := 0
	for _, correction := range corrections {
		updates := map[string]interface{}{
			"status":      models.TVCalendarStatusReady,
			"lastChecked": checkedAt,
		}
		if correction.EmbyItemID != "" {
			updates["embyItemId"] = correction.EmbyItemID
		}
		if correction.SeriesID != "" {
			updates["seriesId"] = correction.SeriesID
		}

		result := db.DB.WithContext(ctx).Model(&models.TVCalendarItem{}).
			Where("\"tmdbId\" = ? AND season = ? AND episode = ?", correction.TmdbID, correction.Season, correction.Episode).
			Updates(updates)
		if result.Error != nil {
			log.Printf("[TV Calendar] 当前周 ready 校正写回失败: tmdbId=%s season=%d episode=%d err=%v", correction.TmdbID, correction.Season, correction.Episode, result.Error)
			continue
		}
		persisted++
	}

	log.Printf("[TV Calendar] 当前周 ready 校正完成: visibleRange=%s~%s corrected=%d persisted=%d", start.Format("2006-01-02"), end.Format("2006-01-02"), len(corrections), persisted)
	return correctedItems
}

func (s *TVCalendarService) loadReadyEpisodesBySeries(ctx context.Context, candidateSeriesIDs map[string]struct{}) map[string]map[string]embyEpisodeItem {
	if len(candidateSeriesIDs) == 0 {
		return map[string]map[string]embyEpisodeItem{}
	}

	seriesIDs := make([]string, 0, len(candidateSeriesIDs))
	for seriesID := range candidateSeriesIDs {
		trimmed := strings.TrimSpace(seriesID)
		if trimmed == "" {
			continue
		}
		seriesIDs = append(seriesIDs, trimmed)
	}
	sort.Strings(seriesIDs)

	readyEpisodesBySeries := make(map[string]map[string]embyEpisodeItem, len(seriesIDs))
	var resultMu sync.Mutex
	var wg sync.WaitGroup
	semaphore := make(chan struct{}, tvCalendarReadyFetchConcurrency)

	for _, seriesID := range seriesIDs {
		seriesID := seriesID
		wg.Add(1)
		go func() {
			defer wg.Done()
			semaphore <- struct{}{}
			defer func() { <-semaphore }()

			readyEpisodes, _, _, fetchErr := s.fetchReadyEpisodesForSeries(ctx, seriesID)
			if fetchErr != nil {
				log.Printf("[TV Calendar] 当前周 ready 校正跳过：seriesId=%s err=%v", seriesID, fetchErr)
				return
			}

			resultMu.Lock()
			readyEpisodesBySeries[seriesID] = readyEpisodes
			resultMu.Unlock()
		}()
	}
	wg.Wait()

	return readyEpisodesBySeries
}

func (s *TVCalendarService) queryCalendarItems(ctx context.Context, tmdbIDs []string, start, end time.Time, status string, loc *time.Location) ([]models.TVCalendarItem, error) {
	query := db.DB.Model(&models.TVCalendarItem{}).
		Where("\"airDate\" >= ? AND \"airDate\" <= ?", start, end)
	if len(tmdbIDs) > 0 {
		query = query.Where("\"tmdbId\" IN ?", tmdbIDs)
	}

	var items []models.TVCalendarItem
	if err := query.Order("\"airDate\" ASC").Order("\"tmdbId\" ASC").Order("season ASC").Order("episode ASC").Find(&items).Error; err != nil {
		return nil, fmt.Errorf("查询追剧日历失败: %w", err)
	}

	items = s.correctCurrentWeekReadyItems(ctx, items, start, end, loc)

	now := time.Now().UTC()
	filtered := make([]models.TVCalendarItem, 0, len(items))
	for _, item := range items {
		item.Status = resolveCalendarItemStatus(item, now, loc)
		if status != "" && item.Status != status {
			continue
		}
		filtered = append(filtered, item)
	}
	return filtered, nil
}

func (s *TVCalendarService) loadSourceMap(tmdbIDs []string) (map[string]models.TVCalendarSource, error) {
	result := make(map[string]models.TVCalendarSource, len(tmdbIDs))
	if len(tmdbIDs) == 0 {
		return result, nil
	}

	var sources []models.TVCalendarSource
	if err := db.DB.Where("\"tmdbId\" IN ?", tmdbIDs).Find(&sources).Error; err != nil {
		return nil, fmt.Errorf("查询追剧源失败: %w", err)
	}
	for _, source := range sources {
		result[source.TmdbID] = source
	}
	return result, nil
}

func (s *TVCalendarService) loadSourcesForSync(tmdbID *string, force bool) ([]models.TVCalendarSource, error) {
	if tmdbID != nil && strings.TrimSpace(*tmdbID) != "" {
		trimmed := strings.TrimSpace(*tmdbID)
		var source models.TVCalendarSource
		err := db.DB.Where("\"tmdbId\" = ?", trimmed).First(&source).Error
		if err == nil {
			return []models.TVCalendarSource{source}, nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("查询追剧源失败: %w", err)
		}
		return []models.TVCalendarSource{{TmdbID: trimmed, EmbyStatus: "continuing"}}, nil
	}

	var sources []models.TVCalendarSource
	if err := db.DB.Where("\"embyStatus\" = ? OR \"embyStatus\" = ''", "continuing").Order("\"showName\" ASC").Find(&sources).Error; err != nil {
		return nil, fmt.Errorf("查询追剧源失败: %w", err)
	}
	if force {
		return sources, nil
	}

	cutoff := activeSourceCutoff(time.Now().UTC())
	prioritized := make([]models.TVCalendarSource, 0, len(sources))
	hasActivityMarker := false
	for _, source := range sources {
		if source.LastEpisodeIngestedAt != nil {
			hasActivityMarker = true
		}
		if shouldSyncSourceIncrementally(source, cutoff) {
			prioritized = append(prioritized, source)
		}
	}
	if len(prioritized) > 0 {
		return prioritized, nil
	}
	if hasActivityMarker {
		return []models.TVCalendarSource{}, nil
	}

	return sources, nil
}

func (s *TVCalendarService) upsertSource(source models.TVCalendarSource, touchSynced bool) error {
	assignments := map[string]interface{}{
		"seriesId": gorm.Expr(
			`CASE WHEN EXCLUDED."seriesId" <> '' THEN EXCLUDED."seriesId" ELSE tv_calendar_sources."seriesId" END`,
		),
		"showName": gorm.Expr(
			`CASE WHEN EXCLUDED."showName" <> '' THEN EXCLUDED."showName" ELSE tv_calendar_sources."showName" END`,
		),
		"posterUrl": gorm.Expr(
			`CASE WHEN EXCLUDED."posterUrl" <> '' THEN EXCLUDED."posterUrl" ELSE tv_calendar_sources."posterUrl" END`,
		),
		"overview": gorm.Expr(
			`CASE WHEN EXCLUDED.overview <> '' THEN EXCLUDED.overview ELSE tv_calendar_sources.overview END`,
		),
		"embyStatus": gorm.Expr(
			`CASE WHEN EXCLUDED."embyStatus" <> '' THEN EXCLUDED."embyStatus" ELSE tv_calendar_sources."embyStatus" END`,
		),
		"updatedAt": time.Now().UTC(),
	}
	if source.LastEpisodeIngestedAt != nil {
		assignments["lastEpisodeIngestedAt"] = gorm.Expr(
			`CASE
				WHEN EXCLUDED."lastEpisodeIngestedAt" IS NULL THEN tv_calendar_sources."lastEpisodeIngestedAt"
				WHEN tv_calendar_sources."lastEpisodeIngestedAt" IS NULL THEN EXCLUDED."lastEpisodeIngestedAt"
				WHEN EXCLUDED."lastEpisodeIngestedAt" > tv_calendar_sources."lastEpisodeIngestedAt" THEN EXCLUDED."lastEpisodeIngestedAt"
				ELSE tv_calendar_sources."lastEpisodeIngestedAt"
			END`,
		)
	}
	if touchSynced {
		assignments["lastSyncedAt"] = source.LastSyncedAt
	}

	if err := db.DB.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "tmdbId"}},
		DoUpdates: clause.Assignments(assignments),
	}).Create(&source).Error; err != nil {
		return fmt.Errorf("写入追剧源失败: %w", err)
	}
	return nil
}

func (s *TVCalendarService) upsertCalendarItem(item models.TVCalendarItem, force bool, readyValidated bool) error {
	statusAssignment := interface{}(item.Status)
	embyItemAssignment := interface{}(item.EmbyItemID)
	if item.Status != models.TVCalendarStatusReady && !(force || readyValidated) {
		statusAssignment = gorm.Expr(
			`CASE WHEN tv_calendar_items.status = ? THEN tv_calendar_items.status ELSE EXCLUDED.status END`,
			models.TVCalendarStatusReady,
		)
		embyItemAssignment = gorm.Expr(
			`CASE WHEN tv_calendar_items.status = ? AND COALESCE(tv_calendar_items."embyItemId", '') <> '' THEN tv_calendar_items."embyItemId" ELSE EXCLUDED."embyItemId" END`,
			models.TVCalendarStatusReady,
		)
	}

	if err := db.DB.Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "tmdbId"}, {Name: "season"}, {Name: "episode"}},
		DoUpdates: clause.Assignments(map[string]interface{}{
			"seriesId": gorm.Expr(
				`CASE WHEN EXCLUDED."seriesId" <> '' THEN EXCLUDED."seriesId" ELSE tv_calendar_items."seriesId" END`,
			),
			"airDate": item.AirDate,
			"episodeName": gorm.Expr(
				`CASE WHEN EXCLUDED."episodeName" <> '' THEN EXCLUDED."episodeName" ELSE tv_calendar_items."episodeName" END`,
			),
			"overview": gorm.Expr(
				`CASE WHEN EXCLUDED.overview <> '' THEN EXCLUDED.overview ELSE tv_calendar_items.overview END`,
			),
			"status":      statusAssignment,
			"embyItemId":  embyItemAssignment,
			"lastChecked": item.LastChecked,
			"updatedAt":   item.UpdatedAt,
		}),
	}).Create(&item).Error; err != nil {
		return fmt.Errorf("写入追剧日历失败: %w", err)
	}
	return nil
}

func (s *TVCalendarService) DiscoverContinuingSeries(ctx context.Context) (int, error) {
	s.refreshConfig()
	if !s.embyService.IsConfigured() {
		return 0, fmt.Errorf("Emby 配置未设置")
	}

	startIndex := 0
	total := 0
	for {
		params := map[string]string{
			"Recursive":        "true",
			"IncludeItemTypes": "Series",
			"Fields":           "ProviderIds,Overview,Path,Status,SeriesStatus,LocationType",
			"Status":           "Continuing",
			"StartIndex":       strconv.Itoa(startIndex),
			"Limit":            strconv.Itoa(tvCalendarSyncPageSize),
		}

		body, err := s.embyService.GetWithAPIKey("/emby/Items", params)
		if err != nil {
			return total, fmt.Errorf("拉取 Emby 连载剧失败: %w", err)
		}

		var resp embyPagedItemsResponse
		if err := json.Unmarshal(body, &resp); err != nil {
			return total, fmt.Errorf("解析 Emby 连载剧失败: %w", err)
		}
		if len(resp.Items) == 0 {
			break
		}

		for _, item := range resp.Items {
			if strings.EqualFold(strings.TrimSpace(item.LocationType), "Virtual") {
				continue
			}
			status := strings.TrimSpace(item.Status)
			if status == "" {
				status = strings.TrimSpace(item.SeriesStatus)
			}
			if status != "" && !strings.EqualFold(status, "Continuing") {
				continue
			}

			tmdbID := extractProviderID(item.ProviderIDs, "Tmdb")
			if tmdbID == "" {
				continue
			}

			source := models.TVCalendarSource{
				TmdbID:     tmdbID,
				SeriesID:   strings.TrimSpace(item.ID),
				ShowName:   strings.TrimSpace(item.Name),
				Overview:   strings.TrimSpace(item.Overview),
				EmbyStatus: "continuing",
			}
			if err := s.upsertSource(source, false); err != nil {
				return total, err
			}
			total++
		}

		startIndex += len(resp.Items)
		if resp.TotalRecordCount > 0 && startIndex >= resp.TotalRecordCount {
			break
		}
		if len(resp.Items) < tvCalendarSyncPageSize {
			break
		}
	}

	return total, nil
}

func (s *TVCalendarService) fetchReadyEpisodesForSeries(ctx context.Context, seriesID string) (map[string]embyEpisodeItem, *time.Time, bool, error) {
	seriesID = strings.TrimSpace(seriesID)
	if seriesID == "" {
		return map[string]embyEpisodeItem{}, nil, false, nil
	}
	if !s.embyService.IsConfigured() {
		return map[string]embyEpisodeItem{}, nil, false, nil
	}
	if episodes, latestCreatedAt, validated, ok := s.getReadyEpisodesCache(seriesID, time.Now()); ok {
		return episodes, latestCreatedAt, validated, nil
	}

	episodes := make(map[string]embyEpisodeItem)
	var latestCreatedAt *time.Time
	startIndex := 0
	for {
		body, err := s.embyService.GetWithAPIKey("/emby/Items", map[string]string{
			"ParentId":         seriesID,
			"Recursive":        "true",
			"IncludeItemTypes": "Episode",
			"Fields":           "SeriesId,ParentIndexNumber,IndexNumber,DateCreated,LocationType,Path,MediaSources,IsMissing",
			"StartIndex":       strconv.Itoa(startIndex),
			"Limit":            strconv.Itoa(tvCalendarSyncPageSize),
		})
		if err != nil {
			return nil, nil, false, fmt.Errorf("拉取 Emby 剧集失败: %w", err)
		}

		var resp embyEpisodeItemsResponse
		if err := json.Unmarshal(body, &resp); err != nil {
			return nil, nil, false, fmt.Errorf("解析 Emby 剧集失败: %w", err)
		}
		if len(resp.Items) == 0 {
			break
		}

		for _, item := range resp.Items {
			if item.ParentIndexNumber <= 0 || item.IndexNumber <= 0 {
				continue
			}
			if !hasPhysicalEpisodeMedia(item) {
				continue
			}
			episodes[buildEpisodeKey(item.ParentIndexNumber, item.IndexNumber)] = item
			if createdAt, ok := parseEmbyDateTime(item.DateCreated); ok {
				if latestCreatedAt == nil || createdAt.After(*latestCreatedAt) {
					createdAtCopy := createdAt
					latestCreatedAt = &createdAtCopy
				}
			}
		}

		startIndex += len(resp.Items)
		if resp.TotalRecordCount > 0 && startIndex >= resp.TotalRecordCount {
			break
		}
		if len(resp.Items) < tvCalendarSyncPageSize {
			break
		}
	}

	s.setReadyEpisodesCache(seriesID, episodes, latestCreatedAt, true, time.Now().Add(tvCalendarReadyEpisodesCacheTTL))
	return episodes, latestCreatedAt, true, nil
}

func copyReadyEpisodesMap(source map[string]embyEpisodeItem) map[string]embyEpisodeItem {
	if len(source) == 0 {
		return map[string]embyEpisodeItem{}
	}
	cloned := make(map[string]embyEpisodeItem, len(source))
	for key, item := range source {
		cloned[key] = item
	}
	return cloned
}

func cloneOptionalTime(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

func (s *TVCalendarService) getReadyEpisodesCache(seriesID string, now time.Time) (map[string]embyEpisodeItem, *time.Time, bool, bool) {
	s.readyCacheMu.RLock()
	entry, ok := s.readyEpisodeCache[seriesID]
	s.readyCacheMu.RUnlock()
	if !ok {
		return nil, nil, false, false
	}
	if now.After(entry.ExpiresAt) {
		s.readyCacheMu.Lock()
		if current, exists := s.readyEpisodeCache[seriesID]; exists && now.After(current.ExpiresAt) {
			delete(s.readyEpisodeCache, seriesID)
		}
		s.readyCacheMu.Unlock()
		return nil, nil, false, false
	}
	return copyReadyEpisodesMap(entry.Episodes), cloneOptionalTime(entry.LatestCreatedAt), entry.Validated, true
}

func (s *TVCalendarService) setReadyEpisodesCache(seriesID string, episodes map[string]embyEpisodeItem, latestCreatedAt *time.Time, validated bool, expiresAt time.Time) {
	s.readyCacheMu.Lock()
	s.readyEpisodeCache[seriesID] = readyEpisodeCacheEntry{
		Episodes:        copyReadyEpisodesMap(episodes),
		LatestCreatedAt: cloneOptionalTime(latestCreatedAt),
		Validated:       validated,
		ExpiresAt:       expiresAt,
	}
	s.readyCacheMu.Unlock()
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
		return upstream.SafeUpstreamError(err, "tmdb")
	}

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return upstream.SafeUpstreamError(err, "tmdb")
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return upstream.SafeUpstreamError(err, "tmdb")
	}

	if resp.StatusCode != http.StatusOK {
		return upstream.SafeUpstreamHTTPError("tmdb", resp.StatusCode)
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

func (s *TVCalendarService) SyncWeek(ctx context.Context, weekStart time.Time, tmdbID *string, force bool) (int, error) {
	s.refreshConfig()
	loc := loadTVCalendarLocation()
	normalizedWeekStart := normalizeTVCalendarWeekStartInLocation(weekStart, loc)
	normalizedWeekEnd := tvCalendarWeekEnd(normalizedWeekStart)
	return runTVCalendarSyncOnce(tvCalendarWeekSyncLockKey(normalizedWeekStart, tmdbID, force), func() (int, error) {
		if strings.TrimSpace(s.tmdbAPIKey) == "" {
			return 0, ErrTVCalendarNotConfigured
		}

		sources, err := s.loadSourcesForSync(tmdbID, force)
		if err != nil {
			return 0, err
		}
		if len(sources) == 0 {
			return 0, nil
		}

		now := time.Now().UTC()
		total := 0

		for _, source := range sources {
			tmdbIDValue := strings.TrimSpace(source.TmdbID)
			if tmdbIDValue == "" {
				continue
			}

			detail, err := s.fetchTVDetail(ctx, tmdbIDValue, force)
			if err != nil {
				log.Printf("[TV Calendar] 跳过剧集同步：show=%q tmdbId=%s err=%v", strings.TrimSpace(source.ShowName), tmdbIDValue, err)
				continue
			}

			if strings.TrimSpace(source.ShowName) == "" {
				source.ShowName = strings.TrimSpace(detail.Name)
			}
			if strings.TrimSpace(source.Overview) == "" {
				source.Overview = strings.TrimSpace(detail.Overview)
			}
			if strings.TrimSpace(source.PosterURL) == "" {
				source.PosterURL = buildTMDBPosterURL(detail.PosterPath)
			}
			syncTime := now
			source.LastSyncedAt = &syncTime
			if err := s.upsertSource(source, true); err != nil {
				return total, err
			}

			readyEpisodes, lastEpisodeIngestedAt, readyValidated, err := s.fetchReadyEpisodesForSeries(ctx, source.SeriesID)
			if err != nil {
				log.Printf("[TV Calendar] 跳过剧集同步：show=%q tmdbId=%s seriesId=%s err=%v", strings.TrimSpace(source.ShowName), tmdbIDValue, strings.TrimSpace(source.SeriesID), err)
				continue
			}
			if lastEpisodeIngestedAt != nil {
				source.LastEpisodeIngestedAt = lastEpisodeIngestedAt
				if err := s.upsertSource(source, true); err != nil {
					return total, err
				}
			}

			seasonNumbers := pickTargetSeasonNumbers(detail)
			for _, seasonNumber := range seasonNumbers {
				season, err := s.fetchSeasonDetail(ctx, tmdbIDValue, seasonNumber, force)
				if err != nil {
					log.Printf("[TV Calendar] 跳过剧集季同步：show=%q tmdbId=%s season=%d err=%v", strings.TrimSpace(source.ShowName), tmdbIDValue, seasonNumber, err)
					continue
				}

				for _, ep := range season.Episodes {
					if ep.EpisodeNumber <= 0 || strings.TrimSpace(ep.AirDate) == "" {
						continue
					}

					airDate, err := parseDateOnly(ep.AirDate)
					if err != nil {
						continue
					}
					if airDate.Before(normalizedWeekStart) || airDate.After(normalizedWeekEnd) {
						continue
					}

					item := models.TVCalendarItem{
						TmdbID:      tmdbIDValue,
						SeriesID:    strings.TrimSpace(source.SeriesID),
						Season:      seasonNumber,
						Episode:     ep.EpisodeNumber,
						AirDate:     airDate,
						EpisodeName: strings.TrimSpace(ep.Name),
						Overview:    strings.TrimSpace(ep.Overview),
						Status:      deriveStatusByAirDate(airDate, now, loc),
						LastChecked: now,
						UpdatedAt:   now,
					}

					if ready, exists := readyEpisodes[buildEpisodeKey(seasonNumber, ep.EpisodeNumber)]; exists {
						item.Status = models.TVCalendarStatusReady
						item.EmbyItemID = strings.TrimSpace(ready.ID)
						if strings.TrimSpace(item.SeriesID) == "" {
							item.SeriesID = strings.TrimSpace(ready.SeriesID)
						}
					}

					if err := s.upsertCalendarItem(item, force, readyValidated); err != nil {
						return total, err
					}
					total++
				}
			}
		}

		return total, nil
	})
}

func (s *TVCalendarService) SyncWeeklyCalendar(ctx context.Context, weekOffset int, tmdbID *string, force bool) (int, error) {
	if !isValidTVCalendarWeekOffset(weekOffset) {
		return 0, ErrTVCalendarInvalidWeekOffset
	}
	return s.SyncWeek(ctx, tvCalendarWeekStartFromOffset(weekOffset, time.Now().UTC(), loadTVCalendarLocation()), tmdbID, force)
}

func (s *TVCalendarService) SyncCalendar(ctx context.Context, weekOffsets []int, tmdbID *string, force bool) (int, error) {
	s.refreshConfig()
	offsets, err := normalizeWeekOffsets(weekOffsets)
	if err != nil {
		return 0, err
	}

	if tmdbID == nil || strings.TrimSpace(*tmdbID) == "" {
		if _, err := runTVCalendarSyncOnce(tvCalendarDiscoverLockKey(force), func() (int, error) {
			return s.DiscoverContinuingSeries(ctx)
		}); err != nil {
			return 0, err
		}
	}

	total := 0
	loc := loadTVCalendarLocation()
	for _, weekStart := range weekStartsFromOffsets(offsets, time.Now().UTC(), loc) {
		count, err := s.SyncWeek(ctx, weekStart, tmdbID, force)
		total += count
		if err != nil {
			return total, err
		}
	}
	return total, nil
}

func (s *TVCalendarService) buildWeeklyCalendar(items []models.TVCalendarItem, sourceMap map[string]models.TVCalendarSource, start, end time.Time, loc *time.Location) *TVCalendarWeeklyDTO {
	dto := s.buildEmptyWeeklyDTO(start, end, loc)
	if len(items) == 0 {
		return dto
	}

	dayIndex := make(map[string]int, len(dto.Days))
	for idx, day := range dto.Days {
		dayIndex[day.Date] = idx
	}

	grouped := make(map[string][]models.TVCalendarItem)
	for _, item := range items {
		dateKey := item.AirDate.Format("2006-01-02")
		grouped[dateKey] = append(grouped[dateKey], item)
	}

	for dateKey, dayItems := range grouped {
		idx, ok := dayIndex[dateKey]
		if !ok {
			continue
		}

		sort.Slice(dayItems, func(i, j int) bool {
			left := dayItems[i]
			right := dayItems[j]
			leftSource := sourceMap[left.TmdbID]
			rightSource := sourceMap[right.TmdbID]
			leftWeight := tvCalendarStatusSortWeight(left.Status)
			rightWeight := tvCalendarStatusSortWeight(right.Status)
			if leftWeight != rightWeight {
				return leftWeight < rightWeight
			}
			if leftSource.ShowName != rightSource.ShowName {
				return leftSource.ShowName < rightSource.ShowName
			}
			if left.Season != right.Season {
				return left.Season < right.Season
			}
			return left.Episode < right.Episode
		})

		aggregates := make([]weeklyAggregate, 0, len(dayItems))
		for _, item := range dayItems {
			source := sourceMap[item.TmdbID]
			meta := weeklySourceMeta{
				ShowName:  strings.TrimSpace(source.ShowName),
				PosterURL: strings.TrimSpace(source.PosterURL),
			}
			if meta.ShowName == "" {
				meta.ShowName = item.TmdbID
			}

			if len(aggregates) > 0 {
				lastIdx := len(aggregates) - 1
				last := &aggregates[lastIdx]
				if last.TmdbID == item.TmdbID &&
					last.Season == item.Season &&
					last.Status == item.Status &&
					last.EndEpisode+1 == item.Episode {
					last.EndEpisode = item.Episode
					if strings.TrimSpace(last.EpisodeName) != strings.TrimSpace(item.EpisodeName) {
						last.EpisodeName = ""
					}
					if strings.TrimSpace(last.Overview) != strings.TrimSpace(item.Overview) {
						last.Overview = ""
					}
					continue
				}
			}

			aggregates = append(aggregates, weeklyAggregate{
				TmdbID:       item.TmdbID,
				SeriesID:     strings.TrimSpace(item.SeriesID),
				ShowName:     meta.ShowName,
				PosterURL:    meta.PosterURL,
				Season:       item.Season,
				StartEpisode: item.Episode,
				EndEpisode:   item.Episode,
				AirDate:      dateKey,
				Status:       item.Status,
				EpisodeName:  strings.TrimSpace(item.EpisodeName),
				Overview:     strings.TrimSpace(item.Overview),
			})
		}

		dayDTO := &dto.Days[idx]
		for _, aggregate := range aggregates {
			episodeText := strconv.Itoa(aggregate.StartEpisode)
			if aggregate.EndEpisode > aggregate.StartEpisode {
				episodeText = fmt.Sprintf("%d-%d", aggregate.StartEpisode, aggregate.EndEpisode)
			}
			dayDTO.Items = append(dayDTO.Items, TVCalendarWeeklyItem{
				TmdbID:      aggregate.TmdbID,
				SeriesID:    aggregate.SeriesID,
				ShowName:    aggregate.ShowName,
				PosterURL:   aggregate.PosterURL,
				Season:      aggregate.Season,
				Episode:     episodeText,
				AirDate:     aggregate.AirDate,
				Status:      aggregate.Status,
				EpisodeName: aggregate.EpisodeName,
				Overview:    aggregate.Overview,
			})
		}
	}

	return dto
}

func (s *TVCalendarService) GetGlobalWeeklyCalendar(ctx context.Context, weekStart time.Time, status string) (*TVCalendarWeeklyDTO, error) {
	s.refreshConfig()
	if !isValidTVCalendarStatus(status) {
		return nil, ErrTVCalendarInvalidStatus
	}
	loc := loadTVCalendarLocation()
	start, end := tvCalendarWeekRange(weekStart)
	items, err := s.queryCalendarItems(ctx, nil, start, end, status, loc)
	if err != nil {
		return nil, err
	}

	tmdbIDs := make([]string, 0, len(items))
	seen := make(map[string]struct{}, len(items))
	for _, item := range items {
		if _, exists := seen[item.TmdbID]; exists {
			continue
		}
		seen[item.TmdbID] = struct{}{}
		tmdbIDs = append(tmdbIDs, item.TmdbID)
	}
	sourceMap, err := s.loadSourceMap(tmdbIDs)
	if err != nil {
		return nil, err
	}

	return s.buildWeeklyCalendar(items, sourceMap, start, end, loc), nil
}

func (s *TVCalendarService) GetFollowingWeeklyCalendar(ctx context.Context, userID string, weekStart time.Time, status string) (*TVCalendarWeeklyDTO, error) {
	s.refreshConfig()
	if !isValidTVCalendarStatus(status) {
		return nil, ErrTVCalendarInvalidStatus
	}
	loc := loadTVCalendarLocation()
	start, end := tvCalendarWeekRange(weekStart)
	subscriptions, err := s.GetSubscriptions(userID)
	if err != nil {
		return nil, err
	}
	if len(subscriptions) == 0 {
		return s.buildEmptyWeeklyDTO(start, end, loc), nil
	}

	tmdbIDs := make([]string, 0, len(subscriptions))
	for _, subscription := range subscriptions {
		tmdbIDs = append(tmdbIDs, subscription.TmdbID)
	}

	items, err := s.queryCalendarItems(ctx, tmdbIDs, start, end, status, loc)
	if err != nil {
		return nil, err
	}

	sourceMap, err := s.loadSourceMap(tmdbIDs)
	if err != nil {
		return nil, err
	}
	for _, subscription := range subscriptions {
		source := sourceMap[subscription.TmdbID]
		if strings.TrimSpace(source.ShowName) == "" {
			source.ShowName = strings.TrimSpace(subscription.ShowName)
		}
		if strings.TrimSpace(source.PosterURL) == "" {
			source.PosterURL = strings.TrimSpace(subscription.PosterURL)
		}
		sourceMap[subscription.TmdbID] = source
	}

	return s.buildWeeklyCalendar(items, sourceMap, start, end, loc), nil
}

func (s *TVCalendarService) FetchCalendar(ctx context.Context, userID string, startDate, endDate time.Time, status string) ([]TVCalendarDTO, error) {
	s.refreshConfig()
	if !isValidTVCalendarStatus(status) {
		return nil, ErrTVCalendarInvalidStatus
	}
	loc := loadTVCalendarLocation()

	subscriptions, err := s.GetSubscriptions(userID)
	if err != nil {
		return nil, err
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

	items, err := s.queryCalendarItems(ctx, tmdbIDs, normalizeDateUTC(startDate), normalizeDateUTC(endDate), status, loc)
	if err != nil {
		return nil, err
	}

	result := make([]TVCalendarDTO, 0, len(items))
	for _, item := range items {
		subscription := subByTmdbID[item.TmdbID]
		result = append(result, TVCalendarDTO{
			ID:          item.ID,
			TmdbID:      item.TmdbID,
			Season:      item.Season,
			Episode:     item.Episode,
			AirDate:     item.AirDate,
			EpisodeName: item.EpisodeName,
			Status:      item.Status,
			EmbyItemID:  item.EmbyItemID,
			ShowName:    subscription.ShowName,
			PosterURL:   subscription.PosterURL,
		})
	}
	return result, nil
}

func (s *TVCalendarService) Subscribe(userID string, req CreateTVCalendarSubscriptionRequest) error {
	s.refreshConfig()
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

func (s *TVCalendarService) GetSubscriptions(userID string) ([]models.TVCalendarSubscription, error) {
	s.refreshConfig()
	var subscriptions []models.TVCalendarSubscription
	if err := db.DB.Where("\"userId\" = ?", userID).Order("\"createdAt\" DESC").Find(&subscriptions).Error; err != nil {
		return nil, fmt.Errorf("查询追剧订阅失败: %w", err)
	}
	return subscriptions, nil
}

func (s *TVCalendarService) Unsubscribe(userID, tmdbID string) error {
	s.refreshConfig()
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

func (s *TVCalendarService) touchSourceLastEpisodeIngestedAt(ctx context.Context, tmdbID, seriesID string, ingestedAt time.Time) error {
	trimmedTmdbID := strings.TrimSpace(tmdbID)
	trimmedSeriesID := strings.TrimSpace(seriesID)
	ingestedAt = ingestedAt.UTC()

	updates := map[string]interface{}{
		"lastEpisodeIngestedAt": ingestedAt,
	}
	if trimmedSeriesID != "" {
		updates["seriesId"] = trimmedSeriesID
	}

	if trimmedTmdbID != "" {
		result := db.DB.WithContext(ctx).Model(&models.TVCalendarSource{}).
			Where("\"tmdbId\" = ?", trimmedTmdbID).
			Updates(updates)
		if result.Error != nil {
			return fmt.Errorf("更新追剧源活跃时间失败: %w", result.Error)
		}
		if result.RowsAffected > 0 || trimmedSeriesID == "" {
			return nil
		}
	}

	if trimmedSeriesID == "" {
		return nil
	}

	result := db.DB.WithContext(ctx).Model(&models.TVCalendarSource{}).
		Where("\"seriesId\" = ?", trimmedSeriesID).
		Updates(map[string]interface{}{
			"lastEpisodeIngestedAt": ingestedAt,
		})
	if result.Error != nil {
		return fmt.Errorf("更新追剧源活跃时间失败: %w", result.Error)
	}

	return nil
}

// MarkEpisodeReadyByWebhook Webhook 点亮剧集状态
func (s *TVCalendarService) MarkEpisodeReadyByWebhook(ctx context.Context, tmdbID, seriesID string, season, episode int, embyItemID string) (int64, error) {
	s.refreshConfig()
	if season <= 0 || episode <= 0 {
		log.Printf("[TV Calendar] Webhook 跳过状态更新：季集号无效 tmdbId=%s seriesId=%s season=%d episode=%d embyItemId=%s", strings.TrimSpace(tmdbID), strings.TrimSpace(seriesID), season, episode, strings.TrimSpace(embyItemID))
		return 0, nil
	}

	now := time.Now().UTC()
	updates := map[string]interface{}{
		"status":      models.TVCalendarStatusReady,
		"lastChecked": now,
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
	log.Printf("[TV Calendar] Webhook 开始更新剧集状态 tmdbId=%s seriesId=%s season=%d episode=%d embyItemId=%s", trimmedTmdbID, trimmedSeriesID, season, episode, strings.TrimSpace(embyItemID))
	if trimmedTmdbID != "" {
		result = db.DB.WithContext(ctx).Model(&models.TVCalendarItem{}).
			Where("\"tmdbId\" = ? AND season = ? AND episode = ?", trimmedTmdbID, season, episode).
			Updates(updates)
		if result.Error != nil {
			log.Printf("[TV Calendar] Webhook 按 tmdbId 更新失败 tmdbId=%s seriesId=%s season=%d episode=%d err=%v", trimmedTmdbID, trimmedSeriesID, season, episode, result.Error)
			return 0, fmt.Errorf("更新剧集状态失败: %w", result.Error)
		}
		if result.RowsAffected > 0 {
			if err := s.touchSourceLastEpisodeIngestedAt(ctx, trimmedTmdbID, trimmedSeriesID, now); err != nil {
				log.Printf("[TV Calendar] Webhook 更新剧集状态后刷新追剧源失败 tmdbId=%s seriesId=%s season=%d episode=%d err=%v", trimmedTmdbID, trimmedSeriesID, season, episode, err)
				return result.RowsAffected, err
			}
			log.Printf("[TV Calendar] Webhook 标记剧集入库成功 matchBy=tmdbId tmdbId=%s seriesId=%s season=%d episode=%d embyItemId=%s updated=%d", trimmedTmdbID, trimmedSeriesID, season, episode, strings.TrimSpace(embyItemID), result.RowsAffected)
			return result.RowsAffected, nil
		}
		log.Printf("[TV Calendar] Webhook 按 tmdbId 未命中任何条目 tmdbId=%s seriesId=%s season=%d episode=%d", trimmedTmdbID, trimmedSeriesID, season, episode)
		if trimmedSeriesID == "" {
			log.Printf("[TV Calendar] Webhook 无法继续回退到 seriesId：seriesId 为空 tmdbId=%s season=%d episode=%d", trimmedTmdbID, season, episode)
			return result.RowsAffected, nil
		}
	}

	if trimmedSeriesID == "" {
		log.Printf("[TV Calendar] Webhook 跳过 seriesId 匹配：seriesId 为空 tmdbId=%s season=%d episode=%d", trimmedTmdbID, season, episode)
		return 0, nil
	}

	result = db.DB.WithContext(ctx).Model(&models.TVCalendarItem{}).
		Where("\"seriesId\" = ? AND season = ? AND episode = ?", trimmedSeriesID, season, episode).
		Updates(updates)
	if result.Error != nil {
		log.Printf("[TV Calendar] Webhook 按 seriesId 更新失败 tmdbId=%s seriesId=%s season=%d episode=%d err=%v", trimmedTmdbID, trimmedSeriesID, season, episode, result.Error)
		return 0, fmt.Errorf("更新剧集状态失败: %w", result.Error)
	}

	if err := s.touchSourceLastEpisodeIngestedAt(ctx, trimmedTmdbID, trimmedSeriesID, now); err != nil {
		log.Printf("[TV Calendar] Webhook 按 seriesId 更新后刷新追剧源失败 tmdbId=%s seriesId=%s season=%d episode=%d err=%v", trimmedTmdbID, trimmedSeriesID, season, episode, err)
		return result.RowsAffected, err
	}
	if result.RowsAffected > 0 {
		log.Printf("[TV Calendar] Webhook 标记剧集入库成功 matchBy=seriesId tmdbId=%s seriesId=%s season=%d episode=%d embyItemId=%s updated=%d", trimmedTmdbID, trimmedSeriesID, season, episode, strings.TrimSpace(embyItemID), result.RowsAffected)
		return result.RowsAffected, nil
	}

	log.Printf("[TV Calendar] Webhook 未命中任何追剧日历条目 tmdbId=%s seriesId=%s season=%d episode=%d embyItemId=%s", trimmedTmdbID, trimmedSeriesID, season, episode, strings.TrimSpace(embyItemID))
	return result.RowsAffected, nil
}
