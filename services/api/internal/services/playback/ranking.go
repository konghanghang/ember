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
	"gorm.io/gorm/clause"
)

const (
	rankingLimit                    = 10
	minRankingDurationSeconds int64 = 60
	rankingUnknownLibraryID         = "__unknown__"
)

var (
	rankingMovieCandidateWindows   = []int{100, 300, 1000, 3000}
	rankingEpisodeCandidateWindows = []int{300, 1000, 3000, 10000}
)

type PlaybackRankingService struct {
	embyService          *embyint.EmbyService
	notifier             rankingNotifier
	loadLibraryAllowlist func() ([]string, error)
	saveLibraryAllowlist func([]string, *string) error
	asyncGo              func(string, func())
	persistRankings      func([]models.PlaybackRanking) (int64, error)
	rankingLibrariesCache *sfCache[string, []RankingLibraryOption]
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

func NewPlaybackRankingService() *PlaybackRankingService {
	return &PlaybackRankingService{
		embyService:          embyint.GetSharedService(),
		notifier:             notifierint.GetSharedBotNotifier(),
		loadLibraryAllowlist: loadPlaybackRankingLibraryAllowlist,
		saveLibraryAllowlist: savePlaybackRankingLibraryAllowlist,
		asyncGo:              async.SafeGo,
		persistRankings:      persistPlaybackRankings,
		rankingLibrariesCache: newSFCache[string, []RankingLibraryOption](10 * time.Minute),
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

func (s *PlaybackRankingService) fetchMovieRankingWithFilter(
	columns playbackActivityColumns,
	start, end time.Time,
	limit int,
	filter rankingLibraryFilter,
) ([]models.PlaybackRanking, int64, error) {
	if filter.allowAll {
		rows, err := s.queryPlaybackAggregates("Movie", columns.itemID, columns.itemName, "movie_item", start, end, limit)
		if err != nil {
			return nil, 0, err
		}
		rankings := convertAggregateRows(models.RankingMediaMovie, rows)
		totalDuration := sumRankingDuration(rankings)
		if limit > 0 && len(rankings) > limit {
			rankings = rankings[:limit]
		}
		log.Printf("[PlaybackRanking] movie aggregates rows=%d rankings=%d range=%s~%s", len(rows), len(rankings), start.Format(time.RFC3339), end.Format(time.RFC3339))
		return rankings, totalDuration, nil
	}

	var lastRows []playbackAggregateRow
	var filteredRows []playbackAggregateRow
	for _, window := range rankingMovieCandidateWindows {
		rows, err := s.queryPlaybackAggregates("Movie", columns.itemID, columns.itemName, "movie_item", start, end, window)
		if err != nil {
			return nil, 0, err
		}
		lastRows = rows

		filteredRows, err = s.filterMovieRowsByLibraries(rows, filter.allowedLibraryIDs)
		if err != nil {
			return nil, 0, err
		}

		rankings := convertAggregateRows(models.RankingMediaMovie, filteredRows)
		if len(rankings) >= limit || len(rows) < window || window == rankingMovieCandidateWindows[len(rankingMovieCandidateWindows)-1] {
			totalDuration := sumRankingDuration(rankings)
			if limit > 0 && len(rankings) > limit {
				rankings = rankings[:limit]
			}
			log.Printf("[PlaybackRanking] movie aggregates rows=%d filtered=%d rankings=%d range=%s~%s", len(lastRows), len(filteredRows), len(rankings), start.Format(time.RFC3339), end.Format(time.RFC3339))
			return rankings, totalDuration, nil
		}
	}

	rankings := convertAggregateRows(models.RankingMediaMovie, filteredRows)
	totalDuration := sumRankingDuration(rankings)
	if limit > 0 && len(rankings) > limit {
		rankings = rankings[:limit]
	}
	log.Printf("[PlaybackRanking] movie aggregates rows=%d filtered=%d rankings=%d range=%s~%s", len(lastRows), len(filteredRows), len(rankings), start.Format(time.RFC3339), end.Format(time.RFC3339))
	return rankings, totalDuration, nil
}

func (s *PlaybackRankingService) fetchEpisodeRanking(
	columns playbackActivityColumns,
	start, end time.Time,
) ([]models.PlaybackRanking, int64, error) {
	return s.fetchEpisodeRankingWithFilter(columns, start, end, rankingLibraryFilter{allowAll: true})
}

func (s *PlaybackRankingService) fetchEpisodeRankingWithFilter(
	columns playbackActivityColumns,
	start, end time.Time,
	filter rankingLibraryFilter,
) ([]models.PlaybackRanking, int64, error) {
	if filter.allowAll {
		rows, err := s.queryPlaybackAggregates("Episode", columns.itemID, columns.itemName, "episode_item", start, end, 0)
		if err != nil {
			return nil, 0, err
		}
		return s.aggregateEpisodeRows(rows, start, end, nil)
	}

	var rows []playbackAggregateRow
	var rankings []models.PlaybackRanking
	var totalDuration int64
	for _, window := range rankingEpisodeCandidateWindows {
		var err error
		rows, err = s.queryPlaybackAggregates("Episode", columns.itemID, columns.itemName, "episode_item", start, end, window)
		if err != nil {
			return nil, 0, err
		}

		rankings, totalDuration, err = s.aggregateEpisodeRows(rows, start, end, filter.allowedLibraryIDs)
		if err != nil {
			return nil, 0, err
		}
		if len(rankings) >= rankingLimit || len(rows) < window || window == rankingEpisodeCandidateWindows[len(rankingEpisodeCandidateWindows)-1] {
			return rankings, totalDuration, nil
		}
	}

	return rankings, totalDuration, nil
}

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

	startStr := start.Format("2006-01-02 15:04:05")
	endStr := end.Format("2006-01-02 15:04:05")
	startStr = start.UTC().Format("2006-01-02 15:04:05")
	endStr = end.UTC().Format("2006-01-02 15:04:05")

	sql := fmt.Sprintf(`
SELECT %s AS item_key,
       %s AS item_name,
       '%s' AS item_source_type,
       COUNT(1) AS play_count,
       COALESCE(SUM(COALESCE(PlayDuration, 0) - COALESCE(PauseDuration, 0)), 0) AS total_duration
FROM PlaybackActivity
WHERE ItemType = '%s'
  AND DateCreated >= '%s'
  AND DateCreated <= '%s'
  AND %s IS NOT NULL
GROUP BY %s, %s
ORDER BY total_duration DESC, play_count DESC, item_name ASC
`, keyExpr, nameExpr, sourceType, itemType, startStr, endStr, keyExpr, keyExpr, nameExpr)
	if limit > 0 {
		sql = fmt.Sprintf("%sLIMIT %d\n", sql, limit)
	}

	resp, err := s.embyService.QueryPlaybackStats(sql)
	if err != nil {
		return nil, err
	}

	rows := make([]playbackAggregateRow, 0, len(resp.Results))
	for _, row := range resp.Results {
		if len(row) < 5 {
			continue
		}

		itemKey := strings.TrimSpace(asString(row[0]))
		itemName := strings.TrimSpace(asString(row[1]))
		if itemKey == "" || itemName == "" {
			continue
		}

		playCount, err := asInt(row[3])
		if err != nil {
			return nil, err
		}
		duration, err := asInt64(row[4])
		if err != nil {
			return nil, err
		}

		rows = append(rows, playbackAggregateRow{
			itemKey:        itemKey,
			itemName:       itemName,
			itemSourceType: strings.TrimSpace(asString(row[2])),
			playCount:      playCount,
			duration:       duration,
		})
	}

	return rows, nil
}

func (s *PlaybackRankingService) queryTotalPlaybackDuration(start, end time.Time) (int64, error) {
	startStr := start.UTC().Format("2006-01-02 15:04:05")
	endStr := end.UTC().Format("2006-01-02 15:04:05")

	sql := fmt.Sprintf(`
SELECT COALESCE(SUM(COALESCE(PlayDuration, 0) - COALESCE(PauseDuration, 0)), 0) AS total_duration
FROM PlaybackActivity
WHERE DateCreated >= '%s'
  AND DateCreated <= '%s'
`, startStr, endStr)

	resp, err := s.embyService.QueryPlaybackStats(sql)
	if err != nil {
		return 0, err
	}
	if resp == nil || len(resp.Results) == 0 || len(resp.Results[0]) == 0 {
		return 0, nil
	}
	return asInt64(resp.Results[0][0])
}

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
	end := start.Add(24 * time.Hour)
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

func (s *PlaybackRankingService) computeRanking(period models.RankingPeriod, start, end *time.Time) (*RankingComputeResult, error) {
	if period != models.RankingDaily && period != models.RankingWeekly {
		return nil, fmt.Errorf("无效的 period: %s", period)
	}
	if (start == nil) != (end == nil) {
		return nil, errors.New("start/end 必须同时传入，或同时为空")
	}

	columns, err := s.loadPlaybackActivityColumns()
	if err != nil {
		return nil, err
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

	movies, movieDuration, err := s.fetchMovieRankingWithFilter(columns, rangeStart, rangeEnd, rankingLimit, filter)
	if err != nil {
		return nil, err
	}
	episodes, episodeDuration, err := s.fetchEpisodeRankingWithFilter(columns, rangeStart, rangeEnd, filter)
	if err != nil {
		return nil, err
	}

	totalDuration := movieDuration + episodeDuration
	if filter.allowAll {
		totalDuration, err = s.queryTotalPlaybackDuration(rangeStart, rangeEnd)
		if err != nil {
			return nil, err
		}
	}

	return &RankingComputeResult{
		Period:        period,
		Start:         rangeStart,
		End:           rangeEnd,
		ComputedAt:    now,
		TotalDuration: totalDuration,
		Movies:        movies,
		Episodes:      episodes,
	}, nil
}

func (s *PlaybackRankingService) filterMovieRowsByLibraries(rows []playbackAggregateRow, allowedLibraryIDs map[string]struct{}) ([]playbackAggregateRow, error) {
	if len(rows) == 0 || len(allowedLibraryIDs) == 0 {
		return []playbackAggregateRow{}, nil
	}

	itemIDs := uniqueAggregateItemKeys(rows)
	libraryByItemID, err := s.resolveEntityLibraries("movie", itemIDs, allowedLibraryIDs)
	if err != nil {
		return nil, err
	}

	filtered := make([]playbackAggregateRow, 0, len(rows))
	for _, row := range rows {
		if libraryByItemID[row.itemKey] == rankingUnknownLibraryID {
			continue
		}
		filtered = append(filtered, row)
	}
	return filtered, nil
}

func (s *PlaybackRankingService) aggregateEpisodeRows(
	rows []playbackAggregateRow,
	start, end time.Time,
	allowedLibraryIDs map[string]struct{},
) ([]models.PlaybackRanking, int64, error) {
	totalDuration := sumPlaybackAggregateDuration(rows)
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
		if len(items) == 0 {
			log.Printf(
				"[PlaybackRanking] episode item lookup failed all batches itemIDs=%d err=%v; degrade to empty episode ranking",
				len(itemIDs),
				err,
			)
			return []models.PlaybackRanking{}, totalDuration, nil
		}
		if !embyint.IsGetItemsByIDsPartialFailure(err) {
			log.Printf(
				"[PlaybackRanking] episode item lookup failed without partial marker itemIDs=%d resolvedItems=%d err=%v; continue with partial results",
				len(itemIDs),
				len(items),
				err,
			)
		} else {
			log.Printf(
				"[PlaybackRanking] episode item lookup partially failed itemIDs=%d resolvedItems=%d err=%v",
				len(itemIDs),
				len(items),
				err,
			)
		}
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
			continue
		}

		seriesID := strings.TrimSpace(itemDetail.SeriesID)
		seriesName := strings.TrimSpace(itemDetail.SeriesName)
		if seriesID == "" || seriesName == "" {
			missingSeriesInfoCount++
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
		libraryBySeriesID, err := s.resolveEntityLibraries("series", seriesIDs, allowedLibraryIDs)
		if err != nil {
			return nil, 0, err
		}
		for seriesID := range seriesRows {
			if libraryBySeriesID[seriesID] != rankingUnknownLibraryID {
				continue
			}
			delete(seriesRows, seriesID)
		}
	}

	aggregated := make([]models.PlaybackRanking, 0, len(seriesRows))
	shortSeriesCount := 0
	filteredDuration := int64(0)
	for _, row := range seriesRows {
		if row.duration < minRankingDurationSeconds {
			shortSeriesCount++
			log.Printf(
				"[PlaybackRanking] 剧集未进入排行榜：seriesId=%s seriesName=%s totalDuration=%ds reason=当前播放总时长不足 60 秒，未进入排行榜",
				row.itemKey,
				row.itemName,
				row.duration,
			)
			continue
		}
		filteredDuration += row.duration
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
		return aggregated[i].ItemName < aggregated[j].ItemName
	})

	if len(aggregated) > rankingLimit {
		aggregated = aggregated[:rankingLimit]
	}
	for i := range aggregated {
		aggregated[i].Rank = i + 1
	}

	if len(allowedLibraryIDs) > 0 {
		totalDuration = filteredDuration
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

func (s *PlaybackRankingService) resolveEntityLibraries(kind string, ids []string, allowedLibraryIDs map[string]struct{}) (map[string]string, error) {
	results := make(map[string]string, len(ids))
	unresolved := make([]string, 0, len(ids))
	for _, id := range ids {
		cacheKey := rankingEntityLibraryCacheKey(kind, id)
		if libraryID, ok := cacheLookupFreshString(s.entityLibraryCache, cacheKey); ok {
			if libraryID == rankingUnknownLibraryID {
				unresolved = append(unresolved, id)
				continue
			}
			results[id] = libraryID
			log.Printf(
				"[PlaybackRanking] %s library cache hit entityId=%s libraryId=%s allowed=%v",
				kind,
				id,
				libraryID,
				sortedLibraryIDKeys(allowedLibraryIDs),
			)
			continue
		}
		unresolved = append(unresolved, id)
	}

	if len(unresolved) == 0 {
		return results, nil
	}

	items, err := s.embyService.GetItemsByIDs(unresolved)
	if err != nil && len(items) > 0 {
		log.Printf("[PlaybackRanking] %s detail lookup partially failed ids=%d resolved=%d err=%v", kind, len(unresolved), len(items), err)
	} else if err != nil {
		log.Printf("[PlaybackRanking] %s detail lookup failed ids=%d err=%v", kind, len(unresolved), err)
	}

	itemByID := make(map[string]embyint.EmbyLibraryItem, len(items))
	for _, item := range items {
		itemByID[strings.TrimSpace(item.ID)] = item
	}

	for _, id := range unresolved {
		if item, ok := itemByID[id]; ok {
			parentID := strings.TrimSpace(item.ParentID)
			if _, allowed := allowedLibraryIDs[parentID]; allowed {
				results[id] = parentID
				cacheStoreString(s.entityLibraryCache, rankingEntityLibraryCacheKey(kind, id), parentID)
				log.Printf(
					"[PlaybackRanking] %s library resolved by parent entityId=%s parentId=%s allowed=%v",
					kind,
					id,
					parentID,
					sortedLibraryIDKeys(allowedLibraryIDs),
				)
				continue
			}
			log.Printf(
				"[PlaybackRanking] %s parent not allowed entityId=%s parentId=%s allowed=%v",
				kind,
				id,
				parentID,
				sortedLibraryIDKeys(allowedLibraryIDs),
			)
		}

		libraryID, err := s.resolveEntityLibraryByAncestors(kind, id, allowedLibraryIDs)
		if err != nil {
			log.Printf("[PlaybackRanking] %s library resolve failed id=%s err=%v", kind, id, err)
			results[id] = rankingUnknownLibraryID
			continue
		}
		results[id] = libraryID
		log.Printf(
			"[PlaybackRanking] %s library resolved by ancestors entityId=%s libraryId=%s allowed=%v",
			kind,
			id,
			libraryID,
			sortedLibraryIDKeys(allowedLibraryIDs),
		)
	}

	log.Printf(
		"[PlaybackRanking] %s library resolve summary ids=%v resolved=%v",
		kind,
		ids,
		results,
	)

	return results, nil
}

func (s *PlaybackRankingService) resolveEntityLibraryByAncestors(kind string, id string, allowedLibraryIDs map[string]struct{}) (string, error) {
	cacheKey := rankingEntityLibraryCacheKey(kind, id)
	if libraryID, ok := cacheLookupFreshString(s.entityLibraryCache, cacheKey); ok && libraryID != rankingUnknownLibraryID {
		return libraryID, nil
	}

	libraryID, err := s.resolveEntityLibraryByAncestorsUncached(id, allowedLibraryIDs)
	if err != nil {
		return rankingUnknownLibraryID, err
	}
	if libraryID != rankingUnknownLibraryID {
		cacheStoreString(s.entityLibraryCache, cacheKey, libraryID)
	}
	return libraryID, nil
}

func (s *PlaybackRankingService) resolveEntityLibraryByAncestorsUncached(id string, allowedLibraryIDs map[string]struct{}) (string, error) {
	ancestors, err := s.embyService.GetItemAncestors(id)
	if err != nil {
		return rankingUnknownLibraryID, err
	}
	ancestorIDs := make([]string, 0, len(ancestors))
	for _, ancestor := range ancestors {
		ancestorID := strings.TrimSpace(ancestor.ID)
		if ancestorID != "" {
			ancestorIDs = append(ancestorIDs, fmt.Sprintf("%s(%s)", ancestorID, ancestor.Name))
		}
		if _, ok := allowedLibraryIDs[ancestorID]; ok {
			log.Printf(
				"[PlaybackRanking] ancestors matched entityId=%s matched=%s ancestors=%v",
				id,
				ancestorID,
				ancestorIDs,
			)
			return ancestorID, nil
		}
	}
	log.Printf(
		"[PlaybackRanking] ancestors unmatched entityId=%s ancestors=%v allowed=%v",
		id,
		ancestorIDs,
		sortedLibraryIDKeys(allowedLibraryIDs),
	)
	return rankingUnknownLibraryID, nil
}

func rankingEntityLibraryCacheKey(kind, id string) string {
	return kind + ":" + strings.TrimSpace(id)
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

func sumRankingDuration(rows []models.PlaybackRanking) int64 {
	var total int64
	for _, row := range rows {
		if row.Duration <= 0 {
			continue
		}
		total += row.Duration
	}
	return total
}

func (s *PlaybackRankingService) GenerateRanking(period models.RankingPeriod, start, end *time.Time) error {
	if s.persistRankings == nil {
		s.persistRankings = persistPlaybackRankings
	}
	if s.asyncGo == nil {
		s.asyncGo = async.SafeGo
	}

	res, err := s.computeRanking(period, start, end)
	if err != nil {
		return err
	}

	rankings := make([]models.PlaybackRanking, 0, len(res.Movies)+len(res.Episodes))
	rankings = append(rankings, res.Movies...)
	rankings = append(rankings, res.Episodes...)

	batchID := ""
	if len(rankings) > 0 {
		batchID = generateRankingBatchID()
	}
	res.BatchID = batchID

	for i := range rankings {
		rankings[i].BatchID = batchID
		rankings[i].Period = res.Period
		rankings[i].SnapshotAt = res.ComputedAt
		rankings[i].PeriodStart = res.Start
		rankings[i].PeriodEnd = res.End
	}

	if len(rankings) > 0 {
		rowsAffected, err := s.persistRankings(rankings)
		if err != nil {
			return err
		}
		if rowsAffected == 0 {
			log.Printf("[Ranking] %s 榜 %s~%s 已存在，跳过", period, res.Start.Format("2006-01-02"), res.End.Format("2006-01-02"))
			return nil
		}
	}

	log.Printf("[PlaybackRanking] generate done period=%s batchId=%s movies=%d episodes=%d start=%s end=%s snapshot=%s", period, batchID, len(res.Movies), len(res.Episodes), res.Start.Format(time.RFC3339), res.End.Format(time.RFC3339), res.ComputedAt.Format(time.RFC3339))

	rankingPayload := buildRankingNotificationPayload(res)
	if s.notifier != nil {
		s.asyncGo("playback.notifyRanking", func() { s.notifier.NotifyRanking(rankingPayload) })
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

func (s *PlaybackRankingService) GetLatestRanking(period models.RankingPeriod) (*RankingResult, error) {
	if period != models.RankingDaily && period != models.RankingWeekly {
		return nil, fmt.Errorf("无效的 period: %s", period)
	}

	now := time.Now().In(loadCronTimezone())

	var latest models.PlaybackRanking
	err := db.DB.
		Where("period = ? AND batch_id <> '' AND period_end <= ?", period, now).
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

	log.Printf("[PlaybackRanking] latest period=%s batchId=%s periodStart=%s periodEnd=%s snapshot=%s", period, latest.BatchID, latest.PeriodStart.Format(time.RFC3339), latest.PeriodEnd.Format(time.RFC3339), latest.SnapshotAt.Format(time.RFC3339))
	return s.loadRankingByBatchID(period, latest.BatchID)
}

func (s *PlaybackRankingService) GetHistoryRanking(
	period models.RankingPeriod,
	rangeStart time.Time,
	rangeEnd time.Time,
) (*RankingResult, error) {
	if period != models.RankingDaily && period != models.RankingWeekly {
		return nil, fmt.Errorf("无效的 period: %s", period)
	}

	var latestBatch models.PlaybackRanking
	err := db.DB.
		Where(
			"period = ? AND batch_id <> '' AND period_start = ? AND period_end >= ? AND period_end < ?",
			period,
			rangeStart,
			rangeStart,
			rangeEnd,
		).
		Order("period_end DESC").
		Order("snapshot_at DESC").
		Order("created_at DESC").
		First(&latestBatch).Error
	if err == nil {
		return s.loadRankingByBatchID(period, latestBatch.BatchID)
	}
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}

	var legacy models.PlaybackRanking
	err = db.DB.
		Where(
			"period = ? AND (batch_id = '' OR batch_id IS NULL) AND period_start = ? AND period_end >= ? AND period_end < ?",
			period,
			rangeStart,
			rangeStart,
			rangeEnd,
		).
		Order("period_end DESC").
		Order("snapshot_at DESC").
		Order("created_at DESC").
		First(&legacy).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	return s.loadLegacyRanking(period, legacy.SnapshotAt)
}

func (s *PlaybackRankingService) loadRankingByBatchID(period models.RankingPeriod, batchID string) (*RankingResult, error) {
	var rows []models.PlaybackRanking
	if err := db.DB.
		Where("period = ? AND batch_id = ?", period, batchID).
		Order("category ASC").
		Order("rank ASC").
		Find(&rows).Error; err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, nil
	}

	return buildRankingResultFromRows(rows), nil
}

func (s *PlaybackRankingService) loadLegacyRanking(period models.RankingPeriod, snapshotAt time.Time) (*RankingResult, error) {
	var rows []models.PlaybackRanking
	if err := db.DB.
		Where("period = ? AND snapshot_at = ? AND (batch_id = '' OR batch_id IS NULL)", period, snapshotAt).
		Order("category ASC").
		Order("rank ASC").
		Find(&rows).Error; err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, nil
	}

	return buildRankingResultFromRows(rows), nil
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

func buildRankingNotificationPayload(res *RankingComputeResult) notifierint.RankingNotification {
	if res == nil {
		return notifierint.RankingNotification{}
	}
	return notifierint.RankingNotification{
		Period:        string(res.Period),
		PeriodStart:   res.Start.Format("2006-01-02"),
		PeriodEnd:     res.End.Format("2006-01-02"),
		CutoffAt:      res.End.Format("15:04"),
		TotalDuration: res.TotalDuration,
		Movies:        toNotifyItems(res.Movies),
		Episodes:      toNotifyItems(res.Episodes),
	}
}

func persistPlaybackRankings(rankings []models.PlaybackRanking) (int64, error) {
	result := db.DB.Clauses(clause.OnConflict{DoNothing: true}).Create(&rankings)
	if result.Error != nil {
		return 0, result.Error
	}
	return result.RowsAffected, nil
}

func (s *PlaybackRankingService) loadPlaybackActivityColumns() (playbackActivityColumns, error) {
	resp, err := s.embyService.QueryPlaybackStats("SELECT * FROM PlaybackActivity LIMIT 1")
	if err != nil {
		return playbackActivityColumns{}, err
	}

	columns := queryColumns(resp)
	if len(columns) == 0 {
		return playbackActivityColumns{}, errors.New("无法识别 PlaybackActivity 字段")
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
	if len(resp.Columns) > 0 {
		return resp.Columns
	}
	return resp.Colums
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
