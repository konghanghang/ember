package mediagap

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	configpkg "github.com/konghang/ember/backend/internal/config"
	"github.com/konghang/ember/backend/internal/db"
	embyint "github.com/konghang/ember/backend/internal/integrations/emby"
	"github.com/konghang/ember/backend/internal/models"
	subscriptionpkg "github.com/konghang/ember/backend/internal/services/subscription"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	tmdbBaseURL        = "https://api.themoviedb.org/3"
	tmdbDetailCacheTTL = 24 * time.Hour
	tmdbSeasonCacheTTL = 24 * time.Hour
	scanPageSize       = 200
)

type tmdbMemoryCacheEntry struct {
	Payload   []byte
	ExpiresAt time.Time
}

type tmdbTVDetailResponse struct {
	ID      int    `json:"id"`
	Name    string `json:"name"`
	Seasons []struct {
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

type embySeriesItemsResponse struct {
	Items            []embySeriesItem `json:"Items"`
	TotalRecordCount int              `json:"TotalRecordCount"`
}

type embySeriesItem struct {
	ID           string            `json:"Id"`
	Name         string            `json:"Name"`
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
	IndexNumberEnd    int                       `json:"IndexNumberEnd"`
	LocationType      string                    `json:"LocationType"`
	Path              string                    `json:"Path"`
	IsMissing         bool                      `json:"IsMissing"`
	MediaSources      []embyint.EmbyMediaSource `json:"MediaSources"`
}

type scanCandidate struct {
	SeriesName string
	SeriesID   string
	Season     int
	Episode    int
	AirDate    time.Time
}

type scanSeriesStats struct {
	Examined int
	Created  int
	Updated  int
	Ingested int
}

type episodeInventory map[int]map[int]struct{}
type seasonSet map[int]struct{}

type Service struct {
	embyService *embyint.EmbyService
	httpClient  *http.Client
	tmdbAPIKey  string

	cacheMu     sync.RWMutex
	memoryCache map[string]tmdbMemoryCacheEntry
}

type MediaGapService = Service

func NewService() *Service {
	svc := &Service{
		embyService: embyint.NewEmbyService(),
		httpClient: &http.Client{
			Timeout: 15 * time.Second,
		},
		memoryCache: make(map[string]tmdbMemoryCacheEntry),
	}
	svc.refreshConfig()
	return svc
}

func NewMediaGapService() *Service {
	return NewService()
}

func (s *Service) refreshConfig() {
	configService := configpkg.NewConfigService()
	s.tmdbAPIKey = strings.TrimSpace(configService.GetString("TMDB_API_KEY"))
}

func (s *Service) IsConfigured() bool {
	s.refreshConfig()
	return s.tmdbAPIKey != "" && s.embyService.IsConfigured()
}

func (s *Service) List(ctx context.Context, req ListRequest) (*ListResponse, error) {
	page := req.Page
	if page <= 0 {
		page = 1
	}
	pageSize := req.PageSize
	if pageSize <= 0 {
		pageSize = 20
	}
	if pageSize > 200 {
		pageSize = 200
	}

	query := db.DB.WithContext(ctx).Model(&models.MediaGap{})

	keyword := strings.TrimSpace(req.Keyword)
	if keyword != "" {
		like := "%" + keyword + "%"
		query = query.Where("\"seriesName\" ILIKE ? OR \"tmdbId\" ILIKE ?", like, like)
	}

	status := normalizeMediaGapStatus(req.Status)
	if status != "" {
		if !isValidMediaGapStatus(status) {
			return nil, ErrMediaGapInvalidStatus
		}
		query = query.Where("status = ?", status)
	}

	if req.AirDateFrom != nil {
		query = query.Where("\"airDate\" >= ?", normalizeDateUTC(*req.AirDateFrom))
	}
	if req.AirDateTo != nil {
		query = query.Where("\"airDate\" <= ?", normalizeDateUTC(*req.AirDateTo))
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, fmt.Errorf("查询缺集总数失败: %w", err)
	}

	var items []models.MediaGap
	if err := query.
		Order("\"airDate\" ASC").
		Order("\"seriesName\" ASC").
		Order("season ASC").
		Order("episode ASC").
		Offset((page - 1) * pageSize).
		Limit(pageSize).
		Find(&items).Error; err != nil {
		return nil, fmt.Errorf("查询缺集列表失败: %w", err)
	}

	log.Printf("[MediaGap] 列表查询完成 keyword=%q status=%q page=%d pageSize=%d total=%d returned=%d", keyword, status, page, pageSize, total, len(items))
	return &ListResponse{
		Data:  items,
		Total: total,
	}, nil
}

func (s *Service) ListMediaGaps(ctx context.Context, query ListQuery) (*ListResponse, error) {
	req := ListRequest{
		Keyword:  strings.TrimSpace(query.Keyword),
		Status:   strings.TrimSpace(query.Status),
		Page:     query.Page,
		PageSize: query.PageSize,
	}

	if trimmed := strings.TrimSpace(query.AirDateFrom); trimmed != "" {
		if parsed, err := time.Parse("2006-01-02", trimmed); err == nil {
			req.AirDateFrom = cloneTimePointer(parsed)
		}
	}
	if trimmed := strings.TrimSpace(query.AirDateTo); trimmed != "" {
		if parsed, err := time.Parse("2006-01-02", trimmed); err == nil {
			req.AirDateTo = cloneTimePointer(parsed)
		}
	}

	return s.List(ctx, req)
}

func (s *Service) Scan(ctx context.Context, req ScanRequest) (*ScanResult, error) {
	s.refreshConfig()
	if !s.IsConfigured() {
		return nil, ErrMediaGapNotConfigured
	}

	scannedAt := time.Now().UTC()
	today := currentCalendarDateInLocation(scannedAt, configpkg.LoadConfiguredTimezone())

	seriesItems, err := s.loadSeriesForScan(req.TMDBID)
	if err != nil {
		return nil, err
	}

	result := &ScanResult{
		ScannedAt: scannedAt,
	}
	if len(seriesItems) == 0 {
		log.Printf("[MediaGap] 扫描结束：没有待扫描剧集 tmdbId=%s", strings.TrimSpace(req.TMDBID))
		return result, nil
	}

	for _, series := range seriesItems {
		stats, err := s.scanSingleSeries(ctx, series, scannedAt, today, req.Force)
		if err != nil {
			result.SkippedSeries++
			log.Printf("[MediaGap] 跳过剧集扫描 seriesId=%s tmdbId=%s name=%q err=%v", strings.TrimSpace(series.ID), extractProviderID(series.ProviderIDs, "Tmdb"), strings.TrimSpace(series.Name), err)
			continue
		}

		result.ScannedSeries++
		result.ExaminedEpisodes += stats.Examined
		result.Created += stats.Created
		result.Updated += stats.Updated
		result.Ingested += stats.Ingested
	}

	log.Printf("[MediaGap] 扫描完成 tmdbId=%s scannedSeries=%d skippedSeries=%d examined=%d created=%d updated=%d ingested=%d",
		strings.TrimSpace(req.TMDBID), result.ScannedSeries, result.SkippedSeries, result.ExaminedEpisodes, result.Created, result.Updated, result.Ingested)
	return result, nil
}

func (s *Service) ScanMediaGaps(ctx context.Context, tmdbID *string) (*ScanResult, error) {
	request := ScanRequest{}
	if tmdbID != nil {
		request.TMDBID = strings.TrimSpace(*tmdbID)
	}
	return s.Scan(ctx, request)
}

func (s *Service) Ignore(ctx context.Context, id, reason string) (*models.MediaGap, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return nil, ErrMediaGapInvalidID
	}

	var gap models.MediaGap
	if err := db.DB.WithContext(ctx).Where("id = ?", id).First(&gap).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, ErrMediaGapNotFound
		}
		return nil, fmt.Errorf("查询缺集工单失败: %w", err)
	}

	now := time.Now().UTC()
	gap.Status = models.MediaGapStatusIgnored
	gap.IgnoredAt = &now
	gap.IgnoreReason = strings.TrimSpace(reason)
	if err := db.DB.WithContext(ctx).Save(&gap).Error; err != nil {
		return nil, fmt.Errorf("忽略缺集工单失败: %w", err)
	}

	log.Printf("[MediaGap] 工单已忽略 id=%s tmdbId=%s seriesId=%s season=%d episode=%d reason=%q",
		gap.ID, gap.TmdbID, gap.EmbySeriesID, gap.Season, gap.Episode, gap.IgnoreReason)
	return &gap, nil
}

func (s *Service) IgnoreGap(ctx context.Context, id, reason string) (*models.MediaGap, error) {
	return s.Ignore(ctx, id, reason)
}

func (s *Service) SearchGap(ctx context.Context, id string) (*MediaGapDTO, error) {
	gap, err := s.loadGapByID(ctx, id)
	if err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	snapshot := SearchSnapshot{
		Keyword:    buildDefaultSearchKeyword(gap),
		SearchedAt: now,
		Candidates: []SearchCandidate{},
	}
	payload, err := json.Marshal(snapshot)
	if err != nil {
		return nil, fmt.Errorf("序列化搜索快照失败: %w", err)
	}

	if gap.Status == models.MediaGapStatusMissing {
		gap.Status = models.MediaGapStatusSearched
	}
	gap.SearchSnapshot = string(payload)
	gap.LastSearchedAt = &now
	if err := db.DB.WithContext(ctx).Save(&gap).Error; err != nil {
		return nil, fmt.Errorf("更新搜索快照失败: %w", err)
	}

	dto := toDTO(gap)
	dto.SearchSnapshot = &snapshot
	log.Printf("[MediaGap] 搜索占位结果已写入 id=%s tmdbId=%s season=%d episode=%d candidates=%d", gap.ID, gap.TmdbID, gap.Season, gap.Episode, len(snapshot.Candidates))
	return dto, nil
}

func (s *Service) DispatchGap(ctx context.Context, id string, req DispatchRequest) (*MediaGapDTO, error) {
	gap, err := s.loadGapByID(ctx, id)
	if err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	snapshot := DispatchSnapshot{
		RequestedAt: now,
		Candidate:   req.Candidate,
	}
	payload, err := json.Marshal(snapshot)
	if err != nil {
		return nil, fmt.Errorf("序列化下发快照失败: %w", err)
	}

	gap.Status = models.MediaGapStatusRequested
	gap.DispatchSnapshot = string(payload)
	gap.RequestedAt = &now
	if err := db.DB.WithContext(ctx).Save(&gap).Error; err != nil {
		return nil, fmt.Errorf("更新下发状态失败: %w", err)
	}

	dto := toDTO(gap)
	dto.DispatchSnapshot = &snapshot
	log.Printf("[MediaGap] 下发占位结果已写入 id=%s tmdbId=%s season=%d episode=%d payloadKeys=%d", gap.ID, gap.TmdbID, gap.Season, gap.Episode, len(req.Candidate.Payload))
	return dto, nil
}

func (s *Service) MarkIngestedByWebhook(ctx context.Context, payload subscriptionpkg.SubscriptionIngestWebhookPayload) (int64, error) {
	if strings.ToLower(strings.TrimSpace(payload.ItemType)) != "episode" {
		return 0, nil
	}
	if payload.Season <= 0 || payload.Episode <= 0 {
		log.Printf("[MediaGap] Webhook 跳过核销：季集号无效 tmdbId=%s seriesId=%s season=%d episode=%d itemName=%q",
			strings.TrimSpace(payload.TmdbID), strings.TrimSpace(payload.SeriesID), payload.Season, payload.Episode, strings.TrimSpace(payload.ItemName))
		return 0, nil
	}

	var matches []models.MediaGap
	matchBy := ""
	trimmedTMDBID := strings.TrimSpace(payload.TmdbID)
	trimmedSeriesID := strings.TrimSpace(payload.SeriesID)

	if trimmedTMDBID != "" {
		if err := db.DB.WithContext(ctx).
			Where("\"tmdbId\" = ? AND season = ? AND episode = ?", trimmedTMDBID, payload.Season, payload.Episode).
			Find(&matches).Error; err != nil {
			return 0, fmt.Errorf("按 tmdbId 查询缺集工单失败: %w", err)
		}
		if len(matches) > 0 {
			matchBy = "tmdbId"
		}
	}

	if len(matches) == 0 && trimmedSeriesID != "" {
		if err := db.DB.WithContext(ctx).
			Where("\"embySeriesId\" = ? AND season = ? AND episode = ?", trimmedSeriesID, payload.Season, payload.Episode).
			Find(&matches).Error; err != nil {
			return 0, fmt.Errorf("按 seriesId 查询缺集工单失败: %w", err)
		}
		if len(matches) > 0 {
			matchBy = "seriesId"
		}
	}

	if len(matches) == 0 {
		log.Printf("[MediaGap] Webhook 未命中缺集工单 tmdbId=%s seriesId=%s season=%d episode=%d itemName=%q",
			trimmedTMDBID, trimmedSeriesID, payload.Season, payload.Episode, strings.TrimSpace(payload.ItemName))
		return 0, nil
	}

	now := time.Now().UTC()
	var updatedCount int64
	for _, gap := range matches {
		changed := false
		if gap.Status != models.MediaGapStatusIngested {
			gap.Status = models.MediaGapStatusIngested
			changed = true
		}
		if gap.IngestedAt == nil {
			gap.IngestedAt = &now
			changed = true
		}
		if trimmedSeriesID != "" && gap.EmbySeriesID != trimmedSeriesID {
			gap.EmbySeriesID = trimmedSeriesID
			changed = true
		}
		if gap.IgnoredAt != nil {
			gap.IgnoredAt = nil
			changed = true
		}
		if gap.IgnoreReason != "" {
			gap.IgnoreReason = ""
			changed = true
		}
		if !changed {
			continue
		}
		if err := db.DB.WithContext(ctx).Save(&gap).Error; err != nil {
			return updatedCount, fmt.Errorf("回写缺集工单入库状态失败: %w", err)
		}
		updatedCount++
	}

	log.Printf("[MediaGap] Webhook 核销完成 matchBy=%s tmdbId=%s seriesId=%s season=%d episode=%d updated=%d itemName=%q",
		matchBy, trimmedTMDBID, trimmedSeriesID, payload.Season, payload.Episode, updatedCount, strings.TrimSpace(payload.ItemName))
	return updatedCount, nil
}

func (s *Service) loadGapByID(ctx context.Context, id string) (models.MediaGap, error) {
	var gap models.MediaGap
	if err := db.DB.WithContext(ctx).Where("id = ?", strings.TrimSpace(id)).First(&gap).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return gap, ErrMediaGapNotFound
		}
		return gap, fmt.Errorf("查询缺集工单失败: %w", err)
	}
	return gap, nil
}

func (s *Service) loadSeriesForScan(tmdbID string) ([]embySeriesItem, error) {
	trimmedTMDBID := strings.TrimSpace(tmdbID)
	if trimmedTMDBID != "" {
		return s.findSeriesByTMDBID(trimmedTMDBID)
	}

	startIndex := 0
	items := make([]embySeriesItem, 0)
	for {
		body, err := s.embyService.GetWithAPIKey("/emby/Items", map[string]string{
			"Recursive":        "true",
			"IncludeItemTypes": "Series",
			"Fields":           "ProviderIds,Path,LocationType,Status,SeriesStatus",
			"Status":           "Continuing",
			"StartIndex":       strconv.Itoa(startIndex),
			"Limit":            strconv.Itoa(scanPageSize),
		})
		if err != nil {
			return nil, fmt.Errorf("拉取 Emby 连载剧失败: %w", err)
		}

		var resp embySeriesItemsResponse
		if err := json.Unmarshal(body, &resp); err != nil {
			return nil, fmt.Errorf("解析 Emby 连载剧失败: %w", err)
		}
		if len(resp.Items) == 0 {
			break
		}

		for _, item := range resp.Items {
			if !isScannableSeries(item, true) {
				continue
			}
			items = append(items, item)
		}

		startIndex += len(resp.Items)
		if resp.TotalRecordCount > 0 && startIndex >= resp.TotalRecordCount {
			break
		}
		if len(resp.Items) < scanPageSize {
			break
		}
	}

	return items, nil
}

func (s *Service) findSeriesByTMDBID(tmdbID string) ([]embySeriesItem, error) {
	body, err := s.embyService.GetWithAPIKey("/emby/Items", map[string]string{
		"Recursive":           "true",
		"IncludeItemTypes":    "Series",
		"Fields":              "ProviderIds,Path,LocationType,Status,SeriesStatus",
		"AnyProviderIdEquals": "Tmdb." + tmdbID,
		"Limit":               "20",
	})
	if err != nil {
		return nil, fmt.Errorf("按 tmdbId 查询 Emby 剧集失败: %w", err)
	}

	var resp embySeriesItemsResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("解析 Emby 剧集查询结果失败: %w", err)
	}

	items := make([]embySeriesItem, 0, len(resp.Items))
	for _, item := range resp.Items {
		if extractProviderID(item.ProviderIDs, "Tmdb") != tmdbID {
			continue
		}
		if !isScannableSeries(item, false) {
			continue
		}
		items = append(items, item)
	}

	if len(items) == 0 {
		log.Printf("[MediaGap] 指定剧集未命中 Emby tmdbId=%s", tmdbID)
	}
	return items, nil
}

func (s *Service) scanSingleSeries(ctx context.Context, series embySeriesItem, scannedAt, today time.Time, force bool) (*scanSeriesStats, error) {
	tmdbID := extractProviderID(series.ProviderIDs, "Tmdb")
	seriesID := strings.TrimSpace(series.ID)
	seriesName := strings.TrimSpace(series.Name)
	if tmdbID == "" || seriesID == "" {
		return nil, fmt.Errorf("缺少扫描所需的 tmdbId 或 seriesId")
	}

	detail, err := s.fetchTVDetail(ctx, tmdbID, force)
	if err != nil {
		return nil, err
	}
	if seriesName == "" {
		seriesName = strings.TrimSpace(detail.Name)
	}

	inventory, err := s.fetchEpisodeInventory(seriesID)
	if err != nil {
		return nil, err
	}
	activeSeasons := inventory.activeSeasons()

	existingByKey, err := s.loadExistingGapMap(ctx, tmdbID)
	if err != nil {
		return nil, err
	}

	stats := &scanSeriesStats{}
	if len(activeSeasons) == 0 {
		cleaned, err := s.cleanupInactiveSeasonGaps(ctx, seriesID, seriesName, scannedAt, existingByKey, nil)
		if err != nil {
			return nil, err
		}
		stats.Updated += cleaned
		log.Printf("[MediaGap] 跳过整剧缺集生成：未发现已激活季 seriesId=%s tmdbId=%s seriesName=%q cleaned=%d", seriesID, tmdbID, seriesName, cleaned)
		return stats, nil
	}

	missing := make(map[string]scanCandidate)
	for _, seasonMeta := range detail.Seasons {
		seasonNumber := seasonMeta.SeasonNumber
		if seasonNumber <= 0 {
			continue
		}
		if !activeSeasons.contains(seasonNumber) {
			continue
		}

		seasonDetail, err := s.fetchSeasonDetail(ctx, tmdbID, seasonNumber, force)
		if err != nil {
			log.Printf("[MediaGap] 跳过季扫描 tmdbId=%s seriesId=%s season=%d err=%v", tmdbID, seriesID, seasonNumber, err)
			continue
		}

		for _, episodeMeta := range seasonDetail.Episodes {
			if episodeMeta.EpisodeNumber <= 0 {
				continue
			}
			airDate, ok := parseTMDBAirDate(episodeMeta.AirDate)
			if !ok || !airDate.Before(today) {
				continue
			}

			stats.Examined++
			if inventory.has(seasonNumber, episodeMeta.EpisodeNumber) {
				continue
			}

			key := buildEpisodeKey(seasonNumber, episodeMeta.EpisodeNumber)
			missing[key] = scanCandidate{
				SeriesName: seriesName,
				SeriesID:   seriesID,
				Season:     seasonNumber,
				Episode:    episodeMeta.EpisodeNumber,
				AirDate:    airDate,
			}
		}
	}

	cleaned, err := s.cleanupInactiveSeasonGaps(ctx, seriesID, seriesName, scannedAt, existingByKey, activeSeasons)
	if err != nil {
		return nil, err
	}
	stats.Updated += cleaned

	for key, candidate := range missing {
		existing := existingByKey[key]
		if existing == nil {
			gap := models.MediaGap{
				TmdbID:        tmdbID,
				EmbySeriesID:  candidate.SeriesID,
				SeriesName:    candidate.SeriesName,
				Season:        candidate.Season,
				Episode:       candidate.Episode,
				AirDate:       candidate.AirDate,
				Status:        models.MediaGapStatusMissing,
				LastScannedAt: cloneTimePointer(scannedAt),
			}
			if err := db.DB.WithContext(ctx).Create(&gap).Error; err != nil {
				return nil, fmt.Errorf("创建缺集工单失败: %w", err)
			}
			stats.Created++
			continue
		}

		changed := false
		if existing.EmbySeriesID != candidate.SeriesID && candidate.SeriesID != "" {
			existing.EmbySeriesID = candidate.SeriesID
			changed = true
		}
		if existing.SeriesName != candidate.SeriesName && candidate.SeriesName != "" {
			existing.SeriesName = candidate.SeriesName
			changed = true
		}
		if !existing.AirDate.Equal(candidate.AirDate) {
			existing.AirDate = candidate.AirDate
			changed = true
		}
		if !timePointerEqual(existing.LastScannedAt, &scannedAt) {
			existing.LastScannedAt = cloneTimePointer(scannedAt)
			changed = true
		}
		if existing.Status == "" {
			existing.Status = models.MediaGapStatusMissing
			changed = true
		}
		if !changed {
			continue
		}
		if err := db.DB.WithContext(ctx).Save(existing).Error; err != nil {
			return nil, fmt.Errorf("更新缺集工单失败: %w", err)
		}
		stats.Updated++
	}

	for key, existing := range existingByKey {
		if existing == nil {
			continue
		}
		if _, stillMissing := missing[key]; stillMissing {
			continue
		}
		if !inventory.has(existing.Season, existing.Episode) {
			continue
		}

		changed := false
		if existing.Status != models.MediaGapStatusIngested {
			existing.Status = models.MediaGapStatusIngested
			changed = true
		}
		if existing.IngestedAt == nil {
			existing.IngestedAt = cloneTimePointer(scannedAt)
			changed = true
		}
		if existing.EmbySeriesID != seriesID && seriesID != "" {
			existing.EmbySeriesID = seriesID
			changed = true
		}
		if existing.IgnoredAt != nil {
			existing.IgnoredAt = nil
			changed = true
		}
		if existing.IgnoreReason != "" {
			existing.IgnoreReason = ""
			changed = true
		}
		if !timePointerEqual(existing.LastScannedAt, &scannedAt) {
			existing.LastScannedAt = cloneTimePointer(scannedAt)
			changed = true
		}
		if !changed {
			continue
		}
		if err := db.DB.WithContext(ctx).Save(existing).Error; err != nil {
			return nil, fmt.Errorf("回写已入库缺集工单失败: %w", err)
		}
		stats.Ingested++
	}

	log.Printf("[MediaGap] 单剧扫描完成 seriesId=%s tmdbId=%s seriesName=%q examined=%d created=%d updated=%d ingested=%d",
		seriesID, tmdbID, seriesName, stats.Examined, stats.Created, stats.Updated, stats.Ingested)
	return stats, nil
}

func (s *Service) cleanupInactiveSeasonGaps(ctx context.Context, seriesID, seriesName string, scannedAt time.Time, existingByKey map[string]*models.MediaGap, activeSeasons seasonSet) (int, error) {
	cleaned := 0
	for _, existing := range existingByKey {
		if existing == nil {
			continue
		}
		if activeSeasons != nil && activeSeasons.contains(existing.Season) {
			continue
		}

		switch existing.Status {
		case models.MediaGapStatusMissing, models.MediaGapStatusSearched:
		default:
			continue
		}

		changed := false
		if existing.Status != models.MediaGapStatusIgnored {
			existing.Status = models.MediaGapStatusIgnored
			changed = true
		}
		if existing.IgnoredAt == nil {
			existing.IgnoredAt = cloneTimePointer(scannedAt)
			changed = true
		}

		reason := "season not activated in library"
		if existing.IgnoreReason != reason {
			existing.IgnoreReason = reason
			changed = true
		}
		if existing.EmbySeriesID != seriesID && seriesID != "" {
			existing.EmbySeriesID = seriesID
			changed = true
		}
		if existing.SeriesName != seriesName && seriesName != "" {
			existing.SeriesName = seriesName
			changed = true
		}
		if !timePointerEqual(existing.LastScannedAt, &scannedAt) {
			existing.LastScannedAt = cloneTimePointer(scannedAt)
			changed = true
		}
		if !changed {
			continue
		}
		if err := db.DB.WithContext(ctx).Save(existing).Error; err != nil {
			return cleaned, fmt.Errorf("收口未激活季缺集工单失败: %w", err)
		}
		cleaned++
	}
	if cleaned > 0 {
		log.Printf("[MediaGap] 已收口未激活季缺集工单 seriesId=%s seriesName=%q cleaned=%d", seriesID, seriesName, cleaned)
	}
	return cleaned, nil
}

func (s *Service) loadExistingGapMap(ctx context.Context, tmdbID string) (map[string]*models.MediaGap, error) {
	var items []models.MediaGap
	if err := db.DB.WithContext(ctx).Where("\"tmdbId\" = ?", tmdbID).Find(&items).Error; err != nil {
		return nil, fmt.Errorf("查询历史缺集工单失败: %w", err)
	}

	result := make(map[string]*models.MediaGap, len(items))
	for i := range items {
		result[buildEpisodeKey(items[i].Season, items[i].Episode)] = &items[i]
	}
	return result, nil
}

func (s *Service) fetchEpisodeInventory(seriesID string) (episodeInventory, error) {
	inventory := make(episodeInventory)
	startIndex := 0
	for {
		body, err := s.embyService.GetWithAPIKey("/emby/Items", map[string]string{
			"ParentId":         seriesID,
			"Recursive":        "true",
			"IncludeItemTypes": "Episode",
			"Fields":           "SeriesId,ParentIndexNumber,IndexNumber,IndexNumberEnd,LocationType,Path,MediaSources,IsMissing",
			"StartIndex":       strconv.Itoa(startIndex),
			"Limit":            strconv.Itoa(scanPageSize),
		})
		if err != nil {
			return nil, fmt.Errorf("拉取 Emby 剧集库存失败: %w", err)
		}

		var resp embyEpisodeItemsResponse
		if err := json.Unmarshal(body, &resp); err != nil {
			return nil, fmt.Errorf("解析 Emby 剧集库存失败: %w", err)
		}
		if len(resp.Items) == 0 {
			break
		}

		for _, item := range resp.Items {
			if !hasPhysicalEpisodeMedia(item) {
				continue
			}
			if item.ParentIndexNumber <= 0 || item.IndexNumber <= 0 {
				continue
			}
			if _, ok := inventory[item.ParentIndexNumber]; !ok {
				inventory[item.ParentIndexNumber] = make(map[int]struct{})
			}
			end := item.IndexNumberEnd
			if end <= 0 || end < item.IndexNumber {
				end = item.IndexNumber
			}
			for episode := item.IndexNumber; episode <= end; episode++ {
				inventory[item.ParentIndexNumber][episode] = struct{}{}
			}
		}

		startIndex += len(resp.Items)
		if resp.TotalRecordCount > 0 && startIndex >= resp.TotalRecordCount {
			break
		}
		if len(resp.Items) < scanPageSize {
			break
		}
	}

	return inventory, nil
}

func (s *Service) fetchTVDetail(ctx context.Context, tmdbID string, force bool) (*tmdbTVDetailResponse, error) {
	endpoint := fmt.Sprintf("%s/tv/%s?api_key=%s&language=zh-CN", tmdbBaseURL, url.PathEscape(tmdbID), url.QueryEscape(s.tmdbAPIKey))
	cacheKey := fmt.Sprintf("mediagap:tv-detail:%s", tmdbID)
	var resp tmdbTVDetailResponse
	if err := s.fetchTMDBJSON(ctx, cacheKey, endpoint, tmdbDetailCacheTTL, force, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

func (s *Service) fetchSeasonDetail(ctx context.Context, tmdbID string, season int, force bool) (*tmdbSeasonResponse, error) {
	endpoint := fmt.Sprintf("%s/tv/%s/season/%d?api_key=%s&language=zh-CN", tmdbBaseURL, url.PathEscape(tmdbID), season, url.QueryEscape(s.tmdbAPIKey))
	cacheKey := fmt.Sprintf("mediagap:tv-season:%s:%d", tmdbID, season)
	var resp tmdbSeasonResponse
	if err := s.fetchTMDBJSON(ctx, cacheKey, endpoint, tmdbSeasonCacheTTL, force, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

func (s *Service) fetchTMDBJSON(ctx context.Context, cacheKey, requestURL string, ttl time.Duration, force bool, out interface{}) error {
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

func (s *Service) getTMDBMemoryCache(cacheKey string, now time.Time) ([]byte, bool) {
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

func (s *Service) setTMDBMemoryCache(cacheKey string, payload []byte, expiresAt time.Time) {
	copyPayload := make([]byte, len(payload))
	copy(copyPayload, payload)
	s.cacheMu.Lock()
	s.memoryCache[cacheKey] = tmdbMemoryCacheEntry{
		Payload:   copyPayload,
		ExpiresAt: expiresAt,
	}
	s.cacheMu.Unlock()
}

func isScannableSeries(item embySeriesItem, requireContinuing bool) bool {
	if strings.EqualFold(strings.TrimSpace(item.LocationType), "Virtual") {
		return false
	}
	if extractProviderID(item.ProviderIDs, "Tmdb") == "" {
		return false
	}
	if !requireContinuing {
		return true
	}

	status := strings.TrimSpace(item.Status)
	if status == "" {
		status = strings.TrimSpace(item.SeriesStatus)
	}
	if status == "" {
		return true
	}
	return strings.EqualFold(status, "Continuing")
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

func parseTMDBAirDate(value string) (time.Time, bool) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return time.Time{}, false
	}
	parsed, err := time.Parse("2006-01-02", trimmed)
	if err != nil {
		return time.Time{}, false
	}
	return normalizeDateUTC(parsed), true
}

func normalizeDateUTC(t time.Time) time.Time {
	y, m, d := t.UTC().Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

func currentCalendarDateInLocation(now time.Time, loc *time.Location) time.Time {
	if loc == nil {
		loc = time.UTC
	}
	y, m, d := now.In(loc).Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

func buildEpisodeKey(season, episode int) string {
	return fmt.Sprintf("%d:%d", season, episode)
}

func cloneTimePointer(value time.Time) *time.Time {
	cloned := value.UTC()
	return &cloned
}

func timePointerEqual(left *time.Time, right *time.Time) bool {
	if left == nil && right == nil {
		return true
	}
	if left == nil || right == nil {
		return false
	}
	return left.UTC().Equal(right.UTC())
}

func isValidMediaGapStatus(status string) bool {
	switch normalizeMediaGapStatus(status) {
	case "",
		string(models.MediaGapStatusMissing),
		string(models.MediaGapStatusSearched),
		string(models.MediaGapStatusRequested),
		string(models.MediaGapStatusIngested),
		string(models.MediaGapStatusIgnored):
		return true
	default:
		return false
	}
}

func normalizeMediaGapStatus(status string) string {
	return strings.ToUpper(strings.TrimSpace(status))
}

func (i episodeInventory) has(season, episode int) bool {
	if season <= 0 || episode <= 0 {
		return false
	}
	seasonInventory, ok := i[season]
	if !ok {
		return false
	}
	_, ok = seasonInventory[episode]
	return ok
}

func (i episodeInventory) activeSeasons() seasonSet {
	result := make(seasonSet)
	for season, episodes := range i {
		if season <= 0 || len(episodes) == 0 {
			continue
		}
		result[season] = struct{}{}
	}
	return result
}

func (s seasonSet) contains(season int) bool {
	if len(s) == 0 {
		return false
	}
	_, ok := s[season]
	return ok
}
