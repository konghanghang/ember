package emby

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// WatchRetentionSeconds reads one complete aggregate by the confirmed plugin record-start contract.
// SQL is generated internally; malformed evidence is an error, never a zero-watch result.
func (s *EmbyService) WatchRetentionSeconds(ctx context.Context, userID string, start, end time.Time, loc *time.Location) (int64, error) {
	if userID == "" || loc == nil || !start.Before(end) {
		return 0, errors.New("保号统计参数无效")
	}
	id := strings.ReplaceAll(userID, "'", "''")
	query := fmt.Sprintf(`SELECT COALESCE(SUM(PlayDuration - PauseDuration),0) AS watched_seconds,
 COALESCE(SUM(CASE WHEN typeof(PlayDuration) != 'integer' OR typeof(PauseDuration) != 'integer' OR PlayDuration < 0 OR PauseDuration < 0 OR PauseDuration > PlayDuration THEN 1 ELSE 0 END),0) AS invalid_rows
 FROM PlaybackActivity WHERE UserId = '%s' AND ItemType IN ('Movie','Episode') AND DateCreated >= '%s' AND DateCreated < '%s'`, id, start.In(loc).Format("2006-01-02 15:04:05"), end.In(loc).Format("2006-01-02 15:04:05"))
	response, err := s.QueryPlaybackStatsContext(ctx, query)
	if err != nil {
		return 0, err
	}
	if response == nil || len(response.Colums) != 2 || response.Colums[0] != "watched_seconds" || response.Colums[1] != "invalid_rows" || len(response.Results) != 1 || len(response.Results[0]) != 2 {
		return 0, errors.New("保号统计响应不完整")
	}
	values := make([]string, 2)
	for i, v := range response.Results[0] {
		text, ok := v.(string)
		if !ok {
			return 0, errors.New("保号统计数值类型无效")
		}
		values[i] = text
	}
	return parseRetentionAggregate(values)
}

// parseRetentionAggregate accepts exact non-negative integer totals and rejects corrupt source rows.
func parseRetentionAggregate(values []string) (int64, error) {
	if len(values) != 2 {
		return 0, errors.New("保号统计列数无效")
	}
	seconds, err := strconv.ParseInt(values[0], 10, 64)
	if err != nil || seconds < 0 {
		return 0, errors.New("保号统计时长无效")
	}
	invalid, err := strconv.ParseInt(values[1], 10, 64)
	if err != nil || invalid != 0 {
		return 0, errors.New("保号统计存在无效播放记录")
	}
	return seconds, nil
}
