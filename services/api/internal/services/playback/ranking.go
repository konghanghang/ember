package playback

import (
	"errors"
	"fmt"
	"time"

	configpkg "github.com/konghang/ember/backend/internal/config"
	"github.com/konghang/ember/backend/internal/db"
	embyint "github.com/konghang/ember/backend/internal/integrations/emby"
	notifierint "github.com/konghang/ember/backend/internal/integrations/notifier"
	"github.com/konghang/ember/backend/internal/models"
)

type PlaybackRankingService struct {
	embyService *embyint.EmbyService
	notifier    *notifierint.BotNotifier
}

// RankingComputeResult 排行榜计算结果（可用于预览，不涉及入库/推送）
type RankingComputeResult struct {
	Period     models.RankingPeriod
	Start      time.Time
	End        time.Time
	ComputedAt time.Time
	Movies     []models.PlaybackRanking
	Episodes   []models.PlaybackRanking
}

func NewPlaybackRankingService() *PlaybackRankingService {
	return &PlaybackRankingService{
		embyService: embyint.NewEmbyService(),
		notifier:    notifierint.NewBotNotifier(),
	}
}

func (s *PlaybackRankingService) fetchMediaRanking(
	category models.RankingCategory,
	start, end time.Time,
	limit int,
) ([]models.PlaybackRanking, error) {
	itemType := ""
	switch category {
	case models.RankingMediaMovie:
		itemType = "Movie"
	case models.RankingMediaEpisode:
		itemType = "Episode"
	default:
		return nil, fmt.Errorf("未知的 category: %s", category)
	}

	startStr := start.Format("2006-01-02 15:04:05")
	endStr := end.Format("2006-01-02 15:04:05")

	sql := fmt.Sprintf(`
SELECT ItemName, COUNT(1) AS play_count,
       COALESCE(SUM(COALESCE(PlayDuration, 0) - COALESCE(PauseDuration, 0)), 0) AS total_duration
FROM PlaybackActivity
WHERE ItemType = '%s'
  AND DateCreated >= '%s'
  AND DateCreated <= '%s'
GROUP BY ItemName
ORDER BY total_duration DESC
LIMIT %d
`, itemType, startStr, endStr, limit)

	resp, err := s.embyService.QueryPlaybackStats(sql)
	if err != nil {
		return nil, err
	}

	rankings := make([]models.PlaybackRanking, 0, len(resp.Results))
	for i, row := range resp.Results {
		if len(row) < 3 {
			continue
		}

		name := asString(row[0])
		playCount, err := asInt(row[1])
		if err != nil {
			return nil, err
		}
		duration, err := asInt64(row[2])
		if err != nil {
			return nil, err
		}

		rankings = append(rankings, models.PlaybackRanking{
			Category:  category,
			Rank:      i + 1,
			ItemName:  name,
			PlayCount: playCount,
			Duration:  duration,
		})
	}

	return rankings, nil
}

func loadCronTimezone() *time.Location {
	return configpkg.LoadConfiguredTimezone()
}

func dayRange(t time.Time) (time.Time, time.Time) {
	start := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
	// 阶段榜：截止时间以触发时刻为准，而不是“当天结束”
	end := t
	return start, end
}

func weekRange(t time.Time) (time.Time, time.Time) {
	// 约定：周一为一周起点，周日为一周终点
	weekday := int(t.Weekday()) // Sunday=0
	daysSinceMonday := weekday - 1
	if weekday == 0 {
		daysSinceMonday = 6
	}

	monday := t.AddDate(0, 0, -daysSinceMonday)
	start := time.Date(monday.Year(), monday.Month(), monday.Day(), 0, 0, 0, 0, t.Location())
	// 阶段榜：截止时间以触发时刻为准
	return start, t
}

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

	movies, err := s.fetchMediaRanking(models.RankingMediaMovie, rangeStart, rangeEnd, 10)
	if err != nil {
		return nil, err
	}
	episodes, err := s.fetchMediaRanking(models.RankingMediaEpisode, rangeStart, rangeEnd, 10)
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

	for i := range rankings {
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

	// 推送是辅助功能，不应该让“生成排行”失败（防止 bot 挂了导致 cron 链路整体掉线）
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
func (s *PlaybackRankingService) PreviewRanking(period models.RankingPeriod) (*RankingComputeResult, error) {
	return s.computeRanking(period, nil, nil)
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

func (s *PlaybackRankingService) GetLatestRanking(
	period models.RankingPeriod,
	category models.RankingCategory,
) ([]models.PlaybackRanking, error) {
	var rankings []models.PlaybackRanking

	subQuery := db.DB.Model(&models.PlaybackRanking{}).
		Select("MAX(snapshot_at)").
		Where("period = ? AND category = ?", period, category)

	err := db.DB.
		Where("period = ? AND category = ? AND snapshot_at = (?)", period, category, subQuery).
		Order("rank ASC").
		Find(&rankings).Error

	return rankings, err
}

func (s *PlaybackRankingService) GetRankingBySnapshot(
	period models.RankingPeriod,
	snapshotAt time.Time,
) ([]models.PlaybackRanking, []models.PlaybackRanking, error) {
	var movies []models.PlaybackRanking
	if err := db.DB.
		Where("period = ? AND category = ? AND snapshot_at = ?", period, models.RankingMediaMovie, snapshotAt).
		Order("rank ASC").
		Find(&movies).Error; err != nil {
		return nil, nil, err
	}

	var episodes []models.PlaybackRanking
	if err := db.DB.
		Where("period = ? AND category = ? AND snapshot_at = ?", period, models.RankingMediaEpisode, snapshotAt).
		Order("rank ASC").
		Find(&episodes).Error; err != nil {
		return nil, nil, err
	}

	return movies, episodes, nil
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
