package playback

import (
	"crypto/rand"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	configpkg "github.com/konghang/ember/backend/internal/config"
	"github.com/konghang/ember/backend/internal/db"
	embyint "github.com/konghang/ember/backend/internal/integrations/emby"
	notifierint "github.com/konghang/ember/backend/internal/integrations/notifier"
	"github.com/konghang/ember/backend/internal/models"
	"gorm.io/gorm"
)

const rankingLimit = 10

type PlaybackRankingService struct {
	embyService *embyint.EmbyService
	notifier    *notifierint.BotNotifier
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
	Period     models.RankingPeriod
	BatchID    string
	Start      time.Time
	End        time.Time
	ComputedAt time.Time
	Movies     []models.PlaybackRanking
	Episodes   []models.PlaybackRanking
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
		embyService: embyint.NewEmbyService(),
		notifier:    notifierint.NewBotNotifier(),
	}
}

func (s *PlaybackRankingService) fetchMovieRanking(
	columns playbackActivityColumns,
	start, end time.Time,
	limit int,
) ([]models.PlaybackRanking, error) {
	rows, err := s.queryPlaybackAggregates("Movie", columns.itemID, columns.itemName, "movie_item", start, end, limit)
	if err != nil {
		return nil, err
	}
	return convertAggregateRows(models.RankingMediaMovie, rows), nil
}

func (s *PlaybackRankingService) fetchEpisodeRanking(
	columns playbackActivityColumns,
	start, end time.Time,
) ([]models.PlaybackRanking, error) {
	rows, err := s.queryPlaybackAggregates("Episode", columns.itemID, columns.itemName, "episode_item", start, end, 0)
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return []models.PlaybackRanking{}, nil
	}

	itemIDs := make([]string, 0, len(rows))
	for _, row := range rows {
		if row.duration <= 0 {
			continue
		}
		itemIDs = append(itemIDs, row.itemKey)
	}
	if len(itemIDs) == 0 {
		return []models.PlaybackRanking{}, nil
	}

	items, err := s.embyService.GetItemsByIDs(itemIDs)
	if err != nil {
		return nil, err
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
	for _, row := range rows {
		if row.duration <= 0 {
			continue
		}

		itemDetail, ok := itemDetails[row.itemKey]
		if !ok {
			continue
		}

		seriesID := strings.TrimSpace(itemDetail.SeriesID)
		seriesName := strings.TrimSpace(itemDetail.SeriesName)
		if seriesID == "" || seriesName == "" {
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

	aggregated := make([]models.PlaybackRanking, 0, len(seriesRows))
	for _, row := range seriesRows {
		if row.duration <= 0 {
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
		return aggregated[i].ItemName < aggregated[j].ItemName
	})

	if len(aggregated) > rankingLimit {
		aggregated = aggregated[:rankingLimit]
	}
	for i := range aggregated {
		aggregated[i].Rank = i + 1
	}

	return aggregated, nil
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

func convertAggregateRows(category models.RankingCategory, rows []playbackAggregateRow) []models.PlaybackRanking {
	rankings := make([]models.PlaybackRanking, 0, len(rows))
	for _, row := range rows {
		if row.duration <= 0 {
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
	return rankings
}

func loadCronTimezone() *time.Location {
	return configpkg.LoadConfiguredTimezone()
}

func dayRange(t time.Time) (time.Time, time.Time) {
	start := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
	end := t
	return start, end
}

func weekRange(t time.Time) (time.Time, time.Time) {
	weekday := int(t.Weekday()) // Sunday=0
	daysSinceMonday := weekday - 1
	if weekday == 0 {
		daysSinceMonday = 6
	}

	monday := t.AddDate(0, 0, -daysSinceMonday)
	start := time.Date(monday.Year(), monday.Month(), monday.Day(), 0, 0, 0, 0, t.Location())
	return start, t
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

	movies, err := s.fetchMovieRanking(columns, rangeStart, rangeEnd, rankingLimit)
	if err != nil {
		return nil, err
	}
	episodes, err := s.fetchEpisodeRanking(columns, rangeStart, rangeEnd)
	if err != nil {
		return nil, err
	}

	return &RankingComputeResult{
		Period:     period,
		Start:      rangeStart,
		End:        rangeEnd,
		ComputedAt: now,
		Movies:     movies,
		Episodes:   episodes,
	}, nil
}

func (s *PlaybackRankingService) GenerateRanking(period models.RankingPeriod, start, end *time.Time) error {
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
		if err := db.DB.Create(&rankings).Error; err != nil {
			return err
		}
	}

	go s.notifier.NotifyRanking(notifierint.RankingNotification{
		Period:      string(period),
		PeriodStart: res.Start.Format("2006-01-02"),
		PeriodEnd:   res.End.Format("2006-01-02"),
		CutoffAt:    res.End.Format("15:04"),
		Movies:      toNotifyItems(res.Movies),
		Episodes:    toNotifyItems(res.Episodes),
	})

	return nil
}

// PreviewRanking 预览生成排行榜（仅计算，不入库、不推送）
func (s *PlaybackRankingService) PreviewRanking(period models.RankingPeriod) (*RankingResult, error) {
	res, err := s.computeRanking(period, nil, nil)
	if err != nil {
		return nil, err
	}
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
	randomBytes := make([]byte, 8)
	_, _ = rand.Read(randomBytes)
	return fmt.Sprintf("rb%x%x", time.Now().UnixNano(), randomBytes)[:25]
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
