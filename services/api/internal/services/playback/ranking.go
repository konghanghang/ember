package playback

import (
	"crypto/rand"
	"errors"
	"fmt"
	"log"
	"sort"
	"strings"
	"time"

	"github.com/konghang/ember/backend/internal/async"
	configpkg "github.com/konghang/ember/backend/internal/config"
	"github.com/konghang/ember/backend/internal/db"
	embyint "github.com/konghang/ember/backend/internal/integrations/emby"
	notifierint "github.com/konghang/ember/backend/internal/integrations/notifier"
	"github.com/konghang/ember/backend/internal/models"
	"github.com/oklog/ulid/v2"
	"gorm.io/gorm"
)

const (
	rankingLimit                        = 10
	minRankingDurationSeconds     int64 = 60
	rankingUnknownLibraryID             = "__unknown__"
	rankingFilteredLogSampleLimit       = 10
)

type PlaybackRankingService struct {
	embyService           *embyint.EmbyService
	notifier              rankingNotifier
	loadLibraryAllowlist  func() ([]string, error)
	saveLibraryAllowlist  func([]string, *string) error
	asyncGo               func(string, func())
	persistBatch          func(*RankingComputeResult) (bool, error)
	rankingLibrariesCache *sfCache[string, rankingLibraryContext]
	entityLibraryCache    *sfCache[string, string]
}

type rankingNotifier interface {
	NotifyRanking(data notifierint.RankingNotification)
}

type RankingResultItem struct {
	Rank           int
	ItemKey        string
	ItemSourceType string
	ItemName       string
	PlayCount      int
	Duration       int64
}

// RankingComputeResult 排行榜计算结果（可用于预览，不涉及入库/推送）
type RankingComputeResult struct {
	Period        models.RankingPeriod
	BatchID       string
	Start         time.Time
	End           time.Time
	ComputedAt    time.Time
	TotalDuration int64
	Movies        []models.PlaybackRanking
	Episodes      []models.PlaybackRanking
}

type RankingResult struct {
	Period      models.RankingPeriod
	BatchID     string
	SnapshotAt  time.Time
	PeriodStart time.Time
	PeriodEnd   time.Time
	Movies      []RankingResultItem
	Episodes    []RankingResultItem
}

type playbackActivityColumns struct {
	itemID   string
	itemName string
}

type playbackAggregateRow struct {
	itemKey        string
	itemName       string
	itemSourceType string
	playCount      int
	duration       int64
}

// NewPlaybackRankingService 装配排行聚合、批次存储与既有异步通知依赖。
func NewPlaybackRankingService() *PlaybackRankingService {
	return &PlaybackRankingService{
		embyService:           embyint.GetSharedService(),
		notifier:              notifierint.GetSharedBotNotifier(),
		loadLibraryAllowlist:  loadPlaybackRankingLibraryAllowlist,
		saveLibraryAllowlist:  savePlaybackRankingLibraryAllowlist,
		asyncGo:               async.SafeGo,
		persistBatch:          persistRankingBatch,
		rankingLibrariesCache: newSFCache[string, rankingLibraryContext](10 * time.Minute),
		entityLibraryCache:    newSFCache[string, string](time.Hour),
	}
}

func (s *PlaybackRankingService) fetchMovieRanking(
	columns playbackActivityColumns,
	start, end time.Time,
	limit int,
) ([]models.PlaybackRanking, int64, error) {
	return s.fetchMovieRankingWithFilter(columns, start, end, limit, rankingLibraryFilter{allowAll: true})
}

// fetchMovieRankingWithFilter 完整聚合后筛选媒体库，先算总量，再应用上榜门槛与 Top N。
func (s *PlaybackRankingService) fetchMovieRankingWithFilter(
	columns playbackActivityColumns,
	start, end time.Time,
	limit int,
	filter rankingLibraryFilter,
) ([]models.PlaybackRanking, int64, error) {
	if !filter.allowAll && len(filter.allowedLibraryIDs) == 0 {
		return []models.PlaybackRanking{}, 0, nil
	}
	rows, err := s.queryPlaybackAggregates("Movie", columns.itemID, columns.itemName, "movie_item", start, end, 0)
	if err != nil {
		return nil, 0, err
	}
	filteredRows := rows
	if !filter.allowAll {
		filteredRows, err = s.filterMovieRowsByLibraries(rows, filter.adminUserID, filter.allowedLibraryIDs)
		if err != nil {
			return nil, 0, err
		}
	}
	totalDuration := sumPlaybackAggregateDuration(filteredRows)
	rankings := convertAggregateRows(models.RankingMediaMovie, filteredRows)
	if limit > 0 && len(rankings) > limit {
		rankings = rankings[:limit]
	}
	log.Printf("[PlaybackRanking] movie aggregates rows=%d filtered=%d rankings=%d totalDuration=%d range=%s~%s", len(rows), len(filteredRows), len(rankings), totalDuration, start.Format(time.RFC3339), end.Format(time.RFC3339))
	return rankings, totalDuration, nil
}

func (s *PlaybackRankingService) fetchEpisodeRanking(
	columns playbackActivityColumns,
	start, end time.Time,
) ([]models.PlaybackRanking, int64, error) {
	return s.fetchEpisodeRankingWithFilter(columns, start, end, rankingLibraryFilter{allowAll: true})
}

// fetchEpisodeRankingWithFilter 完整读取单集聚合后再按 Series 和媒体库筛选，最后取前十。
// 单集时长排名不能作为剧集总时长排名的候选截断条件；条目详情仍由集成层分批查询。
func (s *PlaybackRankingService) fetchEpisodeRankingWithFilter(
	columns playbackActivityColumns,
	start, end time.Time,
	filter rankingLibraryFilter,
) ([]models.PlaybackRanking, int64, error) {
	if !filter.allowAll && len(filter.allowedLibraryIDs) == 0 {
		return []models.PlaybackRanking{}, 0, nil
	}
	rows, err := s.queryPlaybackAggregates("Episode", columns.itemID, columns.itemName, "episode_item", start, end, 0)
	if err != nil {
		return nil, 0, err
	}
	if filter.allowAll {
		return s.aggregateEpisodeRows(rows, start, end, "", nil)
	}
	return s.aggregateEpisodeRows(rows, start, end, filter.adminUserID, filter.allowedLibraryIDs)
}

// queryPlaybackAggregates 按稳定 ID 汇总；MAX 名称仅用于确定展示值，不代表最新名称。
func (s *PlaybackRankingService) queryPlaybackAggregates(
	itemType string,
	itemIDColumn string,
	itemNameColumn string,
	sourceType string,
	start, end time.Time,
	limit int,
) ([]playbackAggregateRow, error) {
	keyExpr := nullableTrimExpr(itemIDColumn)
	nameExpr := nullableTrimExpr(itemNameColumn)

	startStr := formatPlaybackDatabaseTime(start)
	endStr := formatPlaybackDatabaseTime(end)

	sql := fmt.Sprintf(`
SELECT %s AS item_key,
       MAX(%s) AS item_name,
       '%s' AS item_source_type,
       COUNT(1) AS play_count,
       COALESCE(SUM(COALESCE(PlayDuration, 0) - COALESCE(PauseDuration, 0)), 0) AS total_duration
FROM PlaybackActivity
WHERE ItemType = '%s'
  AND DateCreated >= '%s'
  AND DateCreated < '%s'
  AND %s IS NOT NULL
GROUP BY %s
ORDER BY total_duration DESC, play_count DESC, item_name ASC, item_key ASC
`, keyExpr, nameExpr, sourceType, itemType, startStr, endStr, keyExpr, keyExpr)
	if limit > 0 {
		sql = fmt.Sprintf("%sLIMIT %d\n", sql, limit)
	}

	resp, err := s.embyService.QueryPlaybackStats(sql)
	if err != nil {
		return nil, err
	}
	return parseRankingAggregateResponse(itemType, resp)
}

// parseRankingAggregateResponse 按别名解析聚合合同；损坏行返回错误，明确缺失或非正时长条目跳过并记录。
func parseRankingAggregateResponse(itemType string, resp *embyint.CustomQueryResponse) ([]playbackAggregateRow, error) {
	rows := make([]playbackAggregateRow, 0, len(resp.Results))
	if len(resp.Results) == 0 {
		return rows, nil
	}
	indexes := make(map[string]int)
	for index, column := range queryColumns(resp) {
		indexes[strings.ToLower(strings.TrimSpace(column))] = index
	}
	maxIndex := 0
	for _, column := range []string{"item_key", "item_name", "item_source_type", "play_count", "total_duration"} {
		index, ok := indexes[column]
		if !ok {
			return nil, fmt.Errorf("排行榜聚合响应缺少列 %s", column)
		}
		if index > maxIndex {
			maxIndex = index
		}
	}
	missingIdentity, nonPositive := 0, 0
	for rowIndex, row := range resp.Results {
		if len(row) <= maxIndex {
			return nil, fmt.Errorf("排行榜聚合响应不完整: row=%d columns=%d", rowIndex, len(row))
		}

		itemKey, itemName := "", ""
		if value := row[indexes["item_key"]]; value != nil {
			itemKey = strings.TrimSpace(asString(value))
		}
		if value := row[indexes["item_name"]]; value != nil {
			itemName = strings.TrimSpace(asString(value))
		}
		if itemKey == "" || itemName == "" {
			missingIdentity++
			if missingIdentity <= rankingFilteredLogSampleLimit {
				log.Printf("[PlaybackRanking] skip aggregate itemType=%s itemId=%q row=%d reason=missing_identity_or_name", itemType, itemKey, rowIndex)
			}
			continue
		}

		playCount, err := asInt(row[indexes["play_count"]])
		if err != nil {
			return nil, err
		}
		duration, err := asInt64(row[indexes["total_duration"]])
		if err != nil {
			return nil, err
		}
		if duration <= 0 {
			nonPositive++
			continue
		}

		rows = append(rows, playbackAggregateRow{
			itemKey:        itemKey,
			itemName:       itemName,
			itemSourceType: strings.TrimSpace(asString(row[indexes["item_source_type"]])),
			playCount:      playCount,
			duration:       duration,
		})
	}
	if missingIdentity > 0 || nonPositive > 0 {
		log.Printf("[PlaybackRanking] aggregate skipped itemType=%s missingIdentity=%d nonPositiveDuration=%d", itemType, missingIdentity, nonPositive)
	}

	return rows, nil
}

// convertAggregateRows 仅应用上榜资格并编号，不参与总量计算。
func convertAggregateRows(category models.RankingCategory, rows []playbackAggregateRow) []models.PlaybackRanking {
	rankings := make([]models.PlaybackRanking, 0, len(rows))
	shortDurationCount := 0
	for _, row := range rows {
		if row.duration < minRankingDurationSeconds {
			shortDurationCount++
			continue
		}
		rankings = append(rankings, models.PlaybackRanking{
			Category:       category,
			Rank:           len(rankings) + 1,
			ItemKey:        row.itemKey,
			ItemSourceType: row.itemSourceType,
			ItemName:       row.itemName,
			PlayCount:      row.playCount,
			Duration:       row.duration,
		})
	}
	if shortDurationCount > 0 {
		log.Printf("[PlaybackRanking] skip short %s rows count=%d minDuration=%ds", category, shortDurationCount, minRankingDurationSeconds)
	}
	return rankings
}

func loadCronTimezone() *time.Location {
	return configpkg.LoadConfiguredTimezone()
}

func dayRange(t time.Time) (time.Time, time.Time) {
	loc := loadCronTimezone()
	now := t.In(loc)
	start := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)
	end := start.AddDate(0, 0, 1)
	return start, end
}

func weekRange(t time.Time) (time.Time, time.Time) {
	loc := loadCronTimezone()
	now := t.In(loc)
	weekday := int(now.Weekday()) // Sunday=0
	daysSinceMonday := weekday - 1
	if weekday == 0 {
		daysSinceMonday = 6
	}

	monday := now.AddDate(0, 0, -daysSinceMonday)
	start := time.Date(monday.Year(), monday.Month(), monday.Day(), 0, 0, 0, 0, loc)
	end := start.AddDate(0, 0, 7) // 下周一 00:00
	return start, end
}

// computeRanking 按业务时区计算所选范围的电影 / 剧集总量和榜单；全失效范围产生有效空榜。
// 查询失败返回错误，只有成功响应中明确缺少信息的条目可以跳过。
func (s *PlaybackRankingService) computeRanking(period models.RankingPeriod, start, end *time.Time) (*RankingComputeResult, error) {
	if period != models.RankingDaily && period != models.RankingWeekly {
		return nil, fmt.Errorf("无效的 period: %s", period)
	}
	if (start == nil) != (end == nil) {
		return nil, errors.New("start/end 必须同时传入，或同时为空")
	}

	tz := loadCronTimezone()
	now := time.Now().In(tz)

	var rangeStart, rangeEnd time.Time
	if start == nil && end == nil {
		if period == models.RankingDaily {
			rangeStart, rangeEnd = dayRange(now)
		} else {
			rangeStart, rangeEnd = weekRange(now)
		}
	} else {
		rangeStart = start.In(tz)
		rangeEnd = end.In(tz)
	}

	log.Printf("[PlaybackRanking] compute start period=%s start=%s end=%s", period, rangeStart.Format(time.RFC3339), rangeEnd.Format(time.RFC3339))

	filter, err := s.loadRankingLibraryFilter()
	if err != nil {
		return nil, err
	}
	log.Printf(
		"[PlaybackRanking] compute filter allowAll=%v libraryIds=%v",
		filter.allowAll,
		filter.libraryIDs,
	)
	if !filter.allowAll && len(filter.allowedLibraryIDs) == 0 {
		log.Printf("[PlaybackRanking] empty ranking reason=no_valid_selected_libraries period=%s start=%s end=%s", period, rangeStart.Format(time.RFC3339), rangeEnd.Format(time.RFC3339))
		return &RankingComputeResult{Period: period, Start: rangeStart, End: rangeEnd, ComputedAt: now,
			Movies: []models.PlaybackRanking{}, Episodes: []models.PlaybackRanking{}}, nil
	}
	columns, err := s.loadPlaybackActivityColumns()
	if err != nil {
		return nil, err
	}

	movies, movieDuration, err := s.fetchMovieRankingWithFilter(columns, rangeStart, rangeEnd, rankingLimit, filter)
	if err != nil {
		return nil, err
	}
	episodes, episodeDuration, err := s.fetchEpisodeRankingWithFilter(columns, rangeStart, rangeEnd, filter)
	if err != nil {
		return nil, err
	}

	return &RankingComputeResult{
		Period:        period,
		Start:         rangeStart,
		End:           rangeEnd,
		ComputedAt:    now,
		TotalDuration: movieDuration + episodeDuration,
		Movies:        movies,
		Episodes:      episodes,
	}, nil
}

func (s *PlaybackRankingService) filterMovieRowsByLibraries(rows []playbackAggregateRow, adminUserID string, allowedLibraryIDs map[string]struct{}) ([]playbackAggregateRow, error) {
	if len(rows) == 0 || len(allowedLibraryIDs) == 0 {
		return []playbackAggregateRow{}, nil
	}

	itemIDs := uniqueAggregateItemKeys(rows)
	libraryByItemID, err := s.resolveEntityLibraries(adminUserID, "movie", itemIDs, allowedLibraryIDs)
	if err != nil {
		return nil, err
	}

	filtered := make([]playbackAggregateRow, 0, len(rows))
	filteredOutCount := 0
	for _, row := range rows {
		if libraryByItemID[row.itemKey] == rankingUnknownLibraryID {
			filteredOutCount++
			if filteredOutCount <= rankingFilteredLogSampleLimit {
				log.Printf(
					"[PlaybackRanking] 电影未进入排行榜：itemId=%s itemName=%s reason=未在所选媒体库中匹配",
					row.itemKey,
					row.itemName,
				)
			}
			continue
		}
		filtered = append(filtered, row)
	}
	if filteredOutCount > rankingFilteredLogSampleLimit {
		log.Printf(
			"[PlaybackRanking] movie library filter omitted additional logs count=%d sampleLimit=%d",
			filteredOutCount-rankingFilteredLogSampleLimit,
			rankingFilteredLogSampleLimit,
		)
	}
	return filtered, nil
}

// aggregateEpisodeRows 汇总已解析剧集，缺失信息只跳过并记录；请求失败不等同于条目缺失。
// 总量在上榜门槛和 Top 10 之前计算，包含可归属的短时长剧集。
func (s *PlaybackRankingService) aggregateEpisodeRows(
	rows []playbackAggregateRow,
	start, end time.Time,
	adminUserID string,
	allowedLibraryIDs map[string]struct{},
) ([]models.PlaybackRanking, int64, error) {
	totalDuration := int64(0)
	if len(rows) == 0 {
		log.Printf("[PlaybackRanking] episode aggregates rows=0 range=%s~%s", start.Format(time.RFC3339), end.Format(time.RFC3339))
		return []models.PlaybackRanking{}, totalDuration, nil
	}

	itemIDs := make([]string, 0, len(rows))
	zeroDurationCount := 0
	for _, row := range rows {
		if row.duration <= 0 {
			zeroDurationCount++
			continue
		}
		itemIDs = append(itemIDs, row.itemKey)
	}
	if len(itemIDs) == 0 {
		log.Printf("[PlaybackRanking] episode aggregates skipped all rows because duration<=0 count=%d", zeroDurationCount)
		return []models.PlaybackRanking{}, totalDuration, nil
	}

	items, err := s.embyService.GetItemsByIDs(itemIDs)
	if err != nil {
		log.Printf("[PlaybackRanking] episode item lookup failed itemIDs=%d resolvedItems=%d partial=%t", len(itemIDs), len(items), embyint.IsGetItemsByIDsPartialFailure(err))
		return nil, 0, fmt.Errorf("排行榜剧集信息查询失败: %w", err)
	}

	itemDetails := make(map[string]embyint.EmbyLibraryItem, len(items))
	for _, item := range items {
		id := strings.TrimSpace(item.ID)
		if id == "" {
			continue
		}
		itemDetails[id] = item
	}

	type seriesAggregate struct {
		itemKey   string
		itemName  string
		playCount int
		duration  int64
	}

	seriesRows := make(map[string]*seriesAggregate)
	missingItemDetailCount := 0
	missingSeriesInfoCount := 0
	for _, row := range rows {
		if row.duration <= 0 {
			continue
		}

		itemDetail, ok := itemDetails[row.itemKey]
		if !ok {
			missingItemDetailCount++
			if missingItemDetailCount <= rankingFilteredLogSampleLimit {
				log.Printf("[PlaybackRanking] skip episode itemId=%q reason=missing_item_detail", row.itemKey)
			}
			continue
		}

		seriesID := strings.TrimSpace(itemDetail.SeriesID)
		seriesName := strings.TrimSpace(itemDetail.SeriesName)
		if seriesID == "" || seriesName == "" {
			missingSeriesInfoCount++
			if missingSeriesInfoCount <= rankingFilteredLogSampleLimit {
				log.Printf("[PlaybackRanking] skip episode itemId=%q reason=missing_series_info", row.itemKey)
			}
			continue
		}

		aggregate, exists := seriesRows[seriesID]
		if !exists {
			aggregate = &seriesAggregate{
				itemKey:  seriesID,
				itemName: seriesName,
			}
			seriesRows[seriesID] = aggregate
		}

		aggregate.playCount += row.playCount
		aggregate.duration += row.duration
	}

	if len(allowedLibraryIDs) > 0 && len(seriesRows) > 0 {
		seriesIDs := make([]string, 0, len(seriesRows))
		for seriesID := range seriesRows {
			seriesIDs = append(seriesIDs, seriesID)
		}
		libraryBySeriesID, err := s.resolveEntityLibraries(adminUserID, "series", seriesIDs, allowedLibraryIDs)
		if err != nil {
			return nil, 0, err
		}
		filteredOutCount := 0
		for seriesID, row := range seriesRows {
			if libraryBySeriesID[seriesID] != rankingUnknownLibraryID {
				continue
			}
			filteredOutCount++
			if filteredOutCount <= rankingFilteredLogSampleLimit {
				log.Printf(
					"[PlaybackRanking] 剧集未进入排行榜：seriesId=%s seriesName=%s reason=未在所选媒体库中匹配",
					seriesID,
					row.itemName,
				)
			}
			delete(seriesRows, seriesID)
		}
		if filteredOutCount > rankingFilteredLogSampleLimit {
			log.Printf(
				"[PlaybackRanking] series library filter omitted additional logs count=%d sampleLimit=%d",
				filteredOutCount-rankingFilteredLogSampleLimit,
				rankingFilteredLogSampleLimit,
			)
		}
	}

	aggregated := make([]models.PlaybackRanking, 0, len(seriesRows))
	shortSeriesCount := 0
	for _, row := range seriesRows {
		totalDuration += row.duration
		if row.duration < minRankingDurationSeconds {
			shortSeriesCount++
			continue
		}
		aggregated = append(aggregated, models.PlaybackRanking{
			Category:       models.RankingMediaEpisode,
			ItemKey:        row.itemKey,
			ItemSourceType: "series",
			ItemName:       row.itemName,
			PlayCount:      row.playCount,
			Duration:       row.duration,
		})
	}

	sort.Slice(aggregated, func(i, j int) bool {
		if aggregated[i].Duration != aggregated[j].Duration {
			return aggregated[i].Duration > aggregated[j].Duration
		}
		if aggregated[i].PlayCount != aggregated[j].PlayCount {
			return aggregated[i].PlayCount > aggregated[j].PlayCount
		}
		if aggregated[i].ItemName != aggregated[j].ItemName {
			return aggregated[i].ItemName < aggregated[j].ItemName
		}
		return aggregated[i].ItemKey < aggregated[j].ItemKey
	})

	if len(aggregated) > rankingLimit {
		aggregated = aggregated[:rankingLimit]
	}
	for i := range aggregated {
		aggregated[i].Rank = i + 1
	}

	log.Printf(
		"[PlaybackRanking] episode aggregates rows=%d itemIDs=%d resolvedItems=%d series=%d rankings=%d zeroDuration=%d shortSeries=%d missingDetail=%d missingSeries=%d range=%s~%s",
		len(rows),
		len(itemIDs),
		len(itemDetails),
		len(seriesRows),
		len(aggregated),
		zeroDurationCount,
		shortSeriesCount,
		missingItemDetailCount,
		missingSeriesInfoCount,
		start.Format(time.RFC3339),
		end.Format(time.RFC3339),
	)

	return aggregated, totalDuration, nil
}

func uniqueAggregateItemKeys(rows []playbackAggregateRow) []string {
	seen := make(map[string]struct{}, len(rows))
	keys := make([]string, 0, len(rows))
	for _, row := range rows {
		key := strings.TrimSpace(row.itemKey)
		if key == "" {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		keys = append(keys, key)
	}
	return keys
}

func filterPlaybackAggregateRows(rows []playbackAggregateRow, allowedItemIDs map[string]struct{}) []playbackAggregateRow {
	if len(rows) == 0 || len(allowedItemIDs) == 0 {
		return []playbackAggregateRow{}
	}

	filtered := make([]playbackAggregateRow, 0, len(rows))
	for _, row := range rows {
		if _, ok := allowedItemIDs[row.itemKey]; !ok {
			continue
		}
		filtered = append(filtered, row)
	}
	return filtered
}

// resolveEntityLibraries 沿用管理员 View 下的批量匹配，仅缓存成功归属；未匹配不能扩大统计范围。
func (s *PlaybackRankingService) resolveEntityLibraries(adminUserID string, kind string, ids []string, allowedLibraryIDs map[string]struct{}) (map[string]string, error) {
	adminUserID = strings.TrimSpace(adminUserID)
	if adminUserID == "" {
		return nil, errors.New("排行榜媒体库过滤缺少 Emby 管理员用户上下文")
	}
	allowedIDs := sortedLibraryIDKeys(allowedLibraryIDs)
	results := make(map[string]string, len(ids))
	unresolved := make([]string, 0, len(ids))
	for _, id := range ids {
		cacheKey := rankingEntityLibraryCacheKey(adminUserID, kind, id, allowedIDs)
		if libraryID, ok := cacheLookupFreshString(s.entityLibraryCache, cacheKey); ok {
			if _, allowed := allowedLibraryIDs[libraryID]; allowed {
				results[id] = libraryID
			} else {
				results[id] = rankingUnknownLibraryID
			}
			continue
		}
		unresolved = append(unresolved, id)
	}

	if len(unresolved) == 0 {
		log.Printf("[PlaybackRanking] %s library resolve from cache candidates=%d", kind, len(ids))
		return results, nil
	}

	requested := make(map[string]struct{}, len(unresolved))
	unmatched := 0
	for _, id := range unresolved {
		requested[id] = struct{}{}
	}
	for _, libraryID := range allowedIDs {
		items, err := s.embyService.GetUserLibraryItemsByIDs(adminUserID, libraryID, unresolved)
		if err != nil {
			return nil, err
		}
		for _, item := range items {
			itemID := strings.TrimSpace(item.ID)
			if _, ok := requested[itemID]; !ok {
				continue
			}
			results[itemID] = libraryID
			cacheStoreString(s.entityLibraryCache, rankingEntityLibraryCacheKey(adminUserID, kind, itemID, allowedIDs), libraryID)
		}
		log.Printf(
			"[PlaybackRanking] %s library membership adminUserId=%s libraryId=%s candidates=%d matched=%d",
			kind,
			adminUserID,
			libraryID,
			len(unresolved),
			len(items),
		)
	}

	for _, id := range unresolved {
		if _, ok := results[id]; ok {
			continue
		}
		results[id] = rankingUnknownLibraryID
		unmatched++
	}

	log.Printf(
		"[PlaybackRanking] %s library resolve summary candidates=%d cacheHits=%d unmatched=%d",
		kind,
		len(ids),
		len(ids)-len(unresolved),
		unmatched,
	)

	return results, nil
}

func rankingEntityLibraryCacheKey(adminUserID, kind, id string, allowedLibraryIDs []string) string {
	return strings.Join([]string{
		strings.TrimSpace(adminUserID),
		strings.TrimSpace(kind),
		strings.TrimSpace(id),
		strings.Join(allowedLibraryIDs, ","),
	}, ":")
}

func cacheLookupFreshString(c *sfCache[string, string], key string) (string, bool) {
	if c == nil {
		return "", false
	}
	c.mu.RLock()
	defer c.mu.RUnlock()

	value, ok := c.items[key]
	if !ok {
		return "", false
	}
	if time.Since(c.ts[key]) >= c.ttl {
		return "", false
	}
	return value, true
}

func cacheStoreString(c *sfCache[string, string], key, value string) {
	if c == nil {
		return
	}
	c.mu.Lock()
	c.items[key] = value
	c.ts[key] = time.Now()
	c.mu.Unlock()
}

func sortedLibraryIDKeys(values map[string]struct{}) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func sumPlaybackAggregateDuration(rows []playbackAggregateRow) int64 {
	var total int64
	for _, row := range rows {
		if row.duration <= 0 {
			continue
		}
		total += row.duration
	}
	return total
}

// GenerateRanking 完整提交一个周期批次，仅本次创建成功的批次触发既有通知。
// 同周期竞争者和空榜都由批次唯一约束去重；通知结果仅记日志，不会回滚快照或触发补发。
func (s *PlaybackRankingService) GenerateRanking(period models.RankingPeriod, start, end *time.Time) error {
	persist := s.persistBatch
	if persist == nil {
		persist = persistRankingBatch
	}
	runAsync := s.asyncGo
	if runAsync == nil {
		runAsync = async.SafeGo
	}

	res, err := s.computeRanking(period, start, end)
	if err != nil {
		return err
	}

	res.BatchID = generateRankingBatchID()
	created, err := persist(res)
	if err != nil {
		return err
	}
	if !created {
		log.Printf("[PlaybackRanking] duplicate period=%s start=%s end=%s; skip notification", period, res.Start.Format(time.RFC3339), res.End.Format(time.RFC3339))
		return nil
	}

	log.Printf("[PlaybackRanking] generate done period=%s batchId=%s movies=%d episodes=%d start=%s end=%s snapshot=%s", period, res.BatchID, len(res.Movies), len(res.Episodes), res.Start.Format(time.RFC3339), res.End.Format(time.RFC3339), res.ComputedAt.Format(time.RFC3339))

	rankingPayload := buildRankingNotificationPayload(res)
	if s.notifier != nil {
		runAsync("playback.notifyRanking", func() { s.notifier.NotifyRanking(rankingPayload) })
	}

	return nil
}

// PreviewRanking 预览生成排行榜（仅计算，不入库、不推送）
func (s *PlaybackRankingService) PreviewRanking(period models.RankingPeriod) (*RankingResult, error) {
	res, err := s.computeRanking(period, nil, nil)
	if err != nil {
		return nil, err
	}
	log.Printf("[PlaybackRanking] preview done period=%s movies=%d episodes=%d start=%s end=%s snapshot=%s", period, len(res.Movies), len(res.Episodes), res.Start.Format(time.RFC3339), res.End.Format(time.RFC3339), res.ComputedAt.Format(time.RFC3339))
	return buildRankingResult(res.Period, "", res.ComputedAt, res.Start, res.End, res.Movies, res.Episodes), nil
}

// GetLatestRanking 读取已经生成的最近周期，允许周期尚未结束，并保留空批次。
func (s *PlaybackRankingService) GetLatestRanking(period models.RankingPeriod) (*RankingResult, error) {
	if period != models.RankingDaily && period != models.RankingWeekly {
		return nil, fmt.Errorf("无效的 period: %s", period)
	}

	now := time.Now().In(loadCronTimezone())

	var latest models.PlaybackRankingBatch
	err := db.DB.
		Where("period = ? AND period_start <= ? AND snapshot_at <= ?", period, now, now).
		Order("period_end DESC").
		Order("snapshot_at DESC").
		Order("created_at DESC").
		First(&latest).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	log.Printf("[PlaybackRanking] latest period=%s batchId=%s periodStart=%s periodEnd=%s snapshot=%s", period, latest.ID, latest.PeriodStart.Format(time.RFC3339), latest.PeriodEnd.Format(time.RFC3339), latest.SnapshotAt.Format(time.RFC3339))
	return s.loadRankingBatch(&latest)
}

// GetHistoryRanking 选择指定周期内最新快照，包含恰好覆盖整个周期的 period_end。
// 快照元数据的闭合上界不改变播放明细的 [start, end) 统计边界。
func (s *PlaybackRankingService) GetHistoryRanking(
	period models.RankingPeriod,
	rangeStart time.Time,
	rangeEnd time.Time,
) (*RankingResult, error) {
	if period != models.RankingDaily && period != models.RankingWeekly {
		return nil, fmt.Errorf("无效的 period: %s", period)
	}

	var latestBatch models.PlaybackRankingBatch
	err := db.DB.
		Where(
			"period = ? AND period_start = ? AND period_end >= ? AND period_end <= ?",
			period,
			rangeStart,
			rangeStart,
			rangeEnd,
		).
		Order("period_end DESC").
		Order("snapshot_at DESC").
		First(&latestBatch).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	return s.loadRankingBatch(&latestBatch)
}

// loadRankingBatch 将批次作为元数据真相源，空明细仍返回一份有效的空榜。
func (s *PlaybackRankingService) loadRankingBatch(batch *models.PlaybackRankingBatch) (*RankingResult, error) {
	var rows []models.PlaybackRanking
	if err := db.DB.
		Where("period = ? AND batch_id = ?", batch.Period, batch.ID).
		Order("category ASC").
		Order("rank ASC").
		Find(&rows).Error; err != nil {
		return nil, err
	}
	result := buildRankingResultFromRows(rows)
	if result == nil {
		result = buildRankingResult(batch.Period, batch.ID, batch.SnapshotAt, batch.PeriodStart, batch.PeriodEnd, nil, nil)
	}
	result.Period, result.BatchID = batch.Period, batch.ID
	result.SnapshotAt, result.PeriodStart, result.PeriodEnd = batch.SnapshotAt, batch.PeriodStart, batch.PeriodEnd
	return result, nil
}

func toNotifyItems(rankings []models.PlaybackRanking) []notifierint.RankingItemNotify {
	items := make([]notifierint.RankingItemNotify, 0, len(rankings))
	for _, r := range rankings {
		items = append(items, notifierint.RankingItemNotify{
			Rank:     r.Rank,
			Name:     r.ItemName,
			Duration: r.Duration,
			Count:    r.PlayCount,
		})
	}
	return items
}

// buildRankingNotificationPayload 在业务时区生成展示日期和实际生成时间，不改变查询边界。
func buildRankingNotificationPayload(res *RankingComputeResult) notifierint.RankingNotification {
	if res == nil {
		return notifierint.RankingNotification{}
	}
	tz := loadCronTimezone()
	snapshotAt := ""
	generatedClock := ""
	if !res.ComputedAt.IsZero() {
		snapshotAt = res.ComputedAt.In(tz).Format(time.RFC3339)
		generatedClock = res.ComputedAt.In(tz).Format("15:04")
	}
	return notifierint.RankingNotification{
		BatchID:       res.BatchID,
		Period:        string(res.Period),
		PeriodStart:   res.Start.In(tz).Format("2006-01-02"),
		PeriodEnd:     RankingDisplayEnd(res.Start, res.End, tz).Format("2006-01-02"),
		CutoffAt:      generatedClock,
		SnapshotAt:    snapshotAt,
		TotalDuration: res.TotalDuration,
		Movies:        toNotifyItems(res.Movies),
		Episodes:      toNotifyItems(res.Episodes),
	}
}

func (s *PlaybackRankingService) loadPlaybackActivityColumns() (playbackActivityColumns, error) {
	resp, err := s.embyService.QueryPlaybackStats("SELECT DateCreated, ItemId, ItemType, ItemName, PlayDuration, PauseDuration FROM PlaybackActivity LIMIT 0")
	if err != nil {
		return playbackActivityColumns{}, err
	}

	columns := queryColumns(resp)
	if len(columns) == 0 {
		return playbackActivityColumns{}, errors.New("无法识别 PlaybackActivity 字段")
	}

	requiredColumns := []string{"DateCreated", "ItemId", "ItemType", "ItemName", "PlayDuration", "PauseDuration"}
	for _, requiredColumn := range requiredColumns {
		if matchColumn(columns, requiredColumn) == "" {
			return playbackActivityColumns{}, fmt.Errorf("PlaybackActivity 缺少必需字段：%s", requiredColumn)
		}
	}

	out := playbackActivityColumns{
		itemID:   matchColumn(columns, "ItemId"),
		itemName: matchColumn(columns, "ItemName"),
	}

	switch {
	case out.itemID == "":
		return playbackActivityColumns{}, errors.New("PlaybackActivity 缺少稳定媒体键字段：ItemId")
	case out.itemName == "":
		return playbackActivityColumns{}, errors.New("PlaybackActivity 缺少展示字段：ItemName")
	default:
		return out, nil
	}
}

func queryColumns(resp *embyint.CustomQueryResponse) []string {
	if len(resp.Colums) > 0 {
		return resp.Colums
	}
	return resp.Columns
}

func matchColumn(columns []string, target string) string {
	for _, column := range columns {
		if strings.EqualFold(strings.TrimSpace(column), target) {
			return strings.TrimSpace(column)
		}
	}
	return ""
}

func nullableTrimExpr(column string) string {
	if strings.TrimSpace(column) == "" {
		return "NULL"
	}
	return fmt.Sprintf("NULLIF(TRIM(COALESCE(%s, '')), '')", column)
}

func generateRankingBatchID() string {
	return ulid.MustNew(ulid.Timestamp(time.Now().UTC()), rand.Reader).String()
}

func buildRankingResultFromRows(rows []models.PlaybackRanking) *RankingResult {
	if len(rows) == 0 {
		return nil
	}

	meta := rows[0]
	movies := make([]models.PlaybackRanking, 0, len(rows))
	episodes := make([]models.PlaybackRanking, 0, len(rows))
	for _, row := range rows {
		switch row.Category {
		case models.RankingMediaMovie:
			movies = append(movies, row)
		case models.RankingMediaEpisode:
			episodes = append(episodes, row)
		}
	}

	return buildRankingResult(meta.Period, meta.BatchID, meta.SnapshotAt, meta.PeriodStart, meta.PeriodEnd, movies, episodes)
}

func buildRankingResult(
	period models.RankingPeriod,
	batchID string,
	snapshotAt time.Time,
	periodStart time.Time,
	periodEnd time.Time,
	movies []models.PlaybackRanking,
	episodes []models.PlaybackRanking,
) *RankingResult {
	return &RankingResult{
		Period:      period,
		BatchID:     batchID,
		SnapshotAt:  snapshotAt,
		PeriodStart: periodStart,
		PeriodEnd:   periodEnd,
		Movies:      buildRankingItems(movies),
		Episodes:    buildRankingItems(episodes),
	}
}

func buildRankingItems(rows []models.PlaybackRanking) []RankingResultItem {
	items := make([]RankingResultItem, 0, len(rows))
	for _, row := range rows {
		items = append(items, RankingResultItem{
			Rank:           row.Rank,
			ItemKey:        row.ItemKey,
			ItemSourceType: row.ItemSourceType,
			ItemName:       row.ItemName,
			PlayCount:      row.PlayCount,
			Duration:       row.Duration,
		})
	}
	return items
}

func asString(v interface{}) string {
	switch t := v.(type) {
	case string:
		return t
	case []byte:
		return string(t)
	default:
		return fmt.Sprintf("%v", v)
	}
}

func asInt(v interface{}) (int, error) {
	n, err := asInt64(v)
	if err != nil {
		return 0, err
	}
	return int(n), nil
}

func asInt64(v interface{}) (int64, error) {
	switch t := v.(type) {
	case nil:
		return 0, nil
	case float64:
		return int64(t), nil
	case float32:
		return int64(t), nil
	case int:
		return int64(t), nil
	case int64:
		return t, nil
	case int32:
		return int64(t), nil
	case uint64:
		return int64(t), nil
	case uint32:
		return int64(t), nil
	case string:
		var out int64
		if _, err := fmt.Sscan(t, &out); err != nil {
			return 0, fmt.Errorf("数字解析失败: %v", err)
		}
		return out, nil
	default:
		return 0, fmt.Errorf("无法转换为数字: %T", v)
	}
}
