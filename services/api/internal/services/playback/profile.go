package playback

import (
	"context"
	"fmt"
	"log"
	"sort"
	"strings"
	"time"

	"github.com/konghang/ember/backend/internal/db"
	embyint "github.com/konghang/ember/backend/internal/integrations/emby"
	"github.com/konghang/ember/backend/internal/models"
	"gorm.io/gorm"
)

const (
	defaultPlaybackProfileRange = "today"
	playbackProfileRecentLimit  = 10
	playbackProfileBadgeLimit   = 4
	playbackProfileMaxRangeDays = 92
	playbackDateTimeFormat      = "2006-01-02 15:04:05"
)

type UserPlaybackProfileService struct {
	embyService *embyint.EmbyService
}

type PlaybackProfileQuery struct {
	Range     string `form:"range" json:"range"`
	StartDate string `form:"startDate" json:"startDate"`
	EndDate   string `form:"endDate" json:"endDate"`
}

type PlaybackProfileHourlyBucket struct {
	Hour  int   `json:"hour"`
	Count int64 `json:"count"`
}

type PlaybackProfileDeviceBucket struct {
	DeviceName        string `json:"deviceName"`
	Count             int64  `json:"count"`
	Duration          int64  `json:"duration"`
	DurationFormatted string `json:"durationFormatted"`
}

type PlaybackProfileClientBucket struct {
	ClientName        string `json:"clientName"`
	Count             int64  `json:"count"`
	Duration          int64  `json:"duration"`
	DurationFormatted string `json:"durationFormatted"`
}

type PlaybackProfileBadge struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

type UserPlaybackProfile struct {
	UserID                       string                        `json:"userId"`
	Username                     string                        `json:"username"`
	Range                        string                        `json:"range"`
	TotalPlayCount               int64                         `json:"totalPlayCount"`
	TotalPlayDuration            int64                         `json:"totalPlayDuration"`
	TotalPlayDurationFormatted   string                        `json:"totalPlayDurationFormatted"`
	ActiveDays                   int                           `json:"activeDays"`
	AveragePlayDuration          int64                         `json:"averagePlayDuration"`
	AveragePlayDurationFormatted string                        `json:"averagePlayDurationFormatted"`
	LastPlayedAt                 *time.Time                    `json:"lastPlayedAt"`
	HourlyDistribution           []PlaybackProfileHourlyBucket `json:"hourlyDistribution"`
	DeviceDistribution           []PlaybackProfileDeviceBucket `json:"deviceDistribution"`
	ClientDistribution           []PlaybackProfileClientBucket `json:"clientDistribution"`
	Badges                       []PlaybackProfileBadge        `json:"badges"`
	RecentRecords                []PlaybackHistoryItem         `json:"recentRecords"`
}

type UserPlaybackProfileResponse struct {
	Data UserPlaybackProfile `json:"data"`
}

type playbackProfileTarget struct {
	user           models.User
	playbackUserID string
}

type playbackProfileAggregate struct {
	totalPlayCount    int64
	totalPlayDuration int64
	activeDays        int
	averageDuration   int64
	lastPlayedAt      *time.Time
	hourly            []PlaybackProfileHourlyBucket
	devices           []PlaybackProfileDeviceBucket
	clients           []PlaybackProfileClientBucket
	badges            []PlaybackProfileBadge
	recentRecords     []PlaybackHistoryItem
}

type playbackDistributionAggregate struct {
	name     string
	count    int64
	duration int64
}

func NewUserPlaybackProfileService() *UserPlaybackProfileService {
	return &UserPlaybackProfileService{
		embyService: embyint.GetSharedService(),
	}
}

func (s *UserPlaybackProfileService) GetUserProfile(ctx context.Context, userID string, query PlaybackProfileQuery) (*UserPlaybackProfileResponse, error) {
	startedAt := time.Now()
	normalizedRange, startAt, endAt, err := normalizePlaybackProfileRange(query)
	if err != nil {
		return nil, err
	}

	target, err := s.loadPlaybackProfileTarget(ctx, userID)
	if err != nil {
		return nil, err
	}

	log.Printf("[PlaybackProfile] start userID=%s embyUserID=%s range=%s", target.user.ID, target.playbackUserID, normalizedRange)

	rows, err := s.loadPlaybackProfileRows(target.playbackUserID, startAt, endAt)
	if err != nil {
		log.Printf("[PlaybackProfile] query failed userID=%s embyUserID=%s range=%s err=%v", target.user.ID, target.playbackUserID, normalizedRange, err)
		return nil, fmt.Errorf("%w: %v", ErrPlaybackHistoryQueryFailed, err)
	}

	aggregate := buildPlaybackProfileAggregate(target.user, rows)
	log.Printf(
		"[PlaybackProfile] done userID=%s embyUserID=%s range=%s rows=%d totalPlayCount=%d totalPlayDuration=%d activeDays=%d badges=%d cost=%s",
		target.user.ID,
		target.playbackUserID,
		normalizedRange,
		len(rows),
		aggregate.totalPlayCount,
		aggregate.totalPlayDuration,
		aggregate.activeDays,
		len(aggregate.badges),
		time.Since(startedAt),
	)

	return &UserPlaybackProfileResponse{
		Data: UserPlaybackProfile{
			UserID:                       target.user.ID,
			Username:                     target.user.Username,
			Range:                        normalizedRange,
			TotalPlayCount:               aggregate.totalPlayCount,
			TotalPlayDuration:            aggregate.totalPlayDuration,
			TotalPlayDurationFormatted:   formatPlayDuration(aggregate.totalPlayDuration),
			ActiveDays:                   aggregate.activeDays,
			AveragePlayDuration:          aggregate.averageDuration,
			AveragePlayDurationFormatted: formatPlayDuration(aggregate.averageDuration),
			LastPlayedAt:                 aggregate.lastPlayedAt,
			HourlyDistribution:           aggregate.hourly,
			DeviceDistribution:           aggregate.devices,
			ClientDistribution:           aggregate.clients,
			Badges:                       aggregate.badges,
			RecentRecords:                aggregate.recentRecords,
		},
	}, nil
}

func normalizePlaybackProfileRange(query PlaybackProfileQuery) (string, *time.Time, *time.Time, error) {
	startDateRaw := strings.TrimSpace(query.StartDate)
	endDateRaw := strings.TrimSpace(query.EndDate)
	tz := loadPlaybackTimezone()

	if startDateRaw != "" || endDateRaw != "" {
		if startDateRaw == "" || endDateRaw == "" {
			return "", nil, nil, ErrPlaybackProfileInvalidDate
		}

		startDate, hasStartTime, err := parsePlaybackProfileTime(startDateRaw, tz)
		if err != nil {
			return "", nil, nil, ErrPlaybackProfileInvalidDate
		}
		endDate, hasEndTime, err := parsePlaybackProfileTime(endDateRaw, tz)
		if err != nil {
			return "", nil, nil, ErrPlaybackProfileInvalidDate
		}
		if endDate.Before(startDate) {
			return "", nil, nil, ErrPlaybackProfileInvalidDate
		}

		startDay := time.Date(startDate.Year(), startDate.Month(), startDate.Day(), 0, 0, 0, 0, tz)
		endDay := time.Date(endDate.Year(), endDate.Month(), endDate.Day(), 0, 0, 0, 0, tz)
		if endDay.Sub(startDay) > time.Duration(playbackProfileMaxRangeDays-1)*24*time.Hour {
			return "", nil, nil, ErrPlaybackProfileRangeTooLarge
		}

		startAt := startDate
		if !hasStartTime {
			startAt = time.Date(startDate.Year(), startDate.Month(), startDate.Day(), 0, 0, 0, 0, tz)
		}
		endAt := endDate
		if !hasEndTime {
			endAt = time.Date(endDate.Year(), endDate.Month(), endDate.Day(), 23, 59, 59, 0, tz)
		}
		return "custom", &startAt, &endAt, nil
	}

	value := strings.TrimSpace(strings.ToLower(query.Range))
	if value == "" {
		value = defaultPlaybackProfileRange
	}

	now := time.Now().In(tz)
	switch value {
	case "today":
		start := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, tz)
		return value, &start, &now, nil
	case "7d":
		start := now.AddDate(0, 0, -7)
		return value, &start, &now, nil
	case "30d":
		start := now.AddDate(0, 0, -30)
		return value, &start, &now, nil
	case "90d":
		start := now.AddDate(0, 0, -90)
		return value, &start, &now, nil
	case "all":
		return value, nil, nil, nil
	default:
		return "", nil, nil, ErrPlaybackProfileInvalidRange
	}
}

func parsePlaybackProfileTime(raw string, tz *time.Location) (time.Time, bool, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return time.Time{}, false, ErrPlaybackProfileInvalidDate
	}

	if t, err := time.ParseInLocation(playbackDateTimeFormat, trimmed, tz); err == nil {
		return t, true, nil
	}

	if t, err := time.ParseInLocation(playbackDateFormat, trimmed, tz); err == nil {
		return t, false, nil
	}

	return time.Time{}, false, ErrPlaybackProfileInvalidDate
}

func (s *UserPlaybackProfileService) loadPlaybackProfileTarget(ctx context.Context, userID string) (*playbackProfileTarget, error) {
	userID = strings.TrimSpace(userID)
	if !playbackUserIDPattern.MatchString(userID) {
		return nil, ErrPlaybackHistoryInvalidUserID
	}

	var user models.User
	err := db.DB.WithContext(ctx).
		Select("id", "username", "\"embyId\"").
		Where("id = ?", userID).
		First(&user).
		Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, ErrPlaybackHistoryUserNotFound
		}
		return nil, fmt.Errorf("查询用户失败: %w", err)
	}

	playbackUserID := strings.TrimSpace(user.EmbyID)
	if playbackUserID == "" {
		playbackUserID = user.ID
	}
	log.Printf("[PlaybackProfile] resolved target userID=%s username=%s playbackUserID=%s hasEmbyID=%t", user.ID, user.Username, playbackUserID, strings.TrimSpace(user.EmbyID) != "")

	return &playbackProfileTarget{
		user:           user,
		playbackUserID: playbackUserID,
	}, nil
}

func (s *UserPlaybackProfileService) loadPlaybackProfileRows(playbackUserID string, startAt *time.Time, endAt *time.Time) ([]playbackActivityRow, error) {
	whereClause := buildPlaybackProfileWhereClause(playbackUserID, startAt, endAt)

	queryWithOrder := fmt.Sprintf(`
SELECT *
FROM PlaybackActivity
WHERE %s
ORDER BY DateCreated DESC
`, whereClause)

	resp, err := s.embyService.QueryPlaybackStats(queryWithOrder)
	if err != nil {
		queryWithoutOrder := fmt.Sprintf(`
SELECT *
FROM PlaybackActivity
WHERE %s
`, whereClause)
		resp, err = s.embyService.QueryPlaybackStats(queryWithoutOrder)
		if err != nil {
			return nil, err
		}
	}

	rows, err := parsePlaybackRows(resp)
	if err != nil {
		return nil, fmt.Errorf("解析用户画像播放记录失败: %w", err)
	}
	log.Printf("[PlaybackProfile] rows loaded playbackUserID=%s rangeStart=%s rangeEnd=%s rows=%d", playbackUserID, formatOptionalTime(startAt), formatOptionalTime(endAt), len(rows))

	sort.Slice(rows, func(i, j int) bool {
		ti := parsePlaybackTime(rows[i].playedAtRaw)
		tj := parsePlaybackTime(rows[j].playedAtRaw)
		if !ti.Equal(tj) {
			return ti.After(tj)
		}
		return rows[i].itemName < rows[j].itemName
	})

	return rows, nil
}

func formatOptionalTime(value *time.Time) string {
	if value == nil {
		return "all"
	}
	return value.Format(time.RFC3339)
}

func buildPlaybackProfileWhereClause(playbackUserID string, startAt *time.Time, endAt *time.Time) string {
	conditions := []string{
		fmt.Sprintf("UserId = '%s'", escapeSQLLiteral(playbackUserID)),
	}
	if startAt != nil {
		conditions = append(conditions, fmt.Sprintf("DateCreated >= '%s'", startAt.UTC().Format("2006-01-02 15:04:05")))
	}
	if endAt != nil {
		conditions = append(conditions, fmt.Sprintf("DateCreated <= '%s'", endAt.UTC().Format("2006-01-02 15:04:05")))
	}
	return strings.Join(conditions, " AND ")
}

func buildPlaybackProfileAggregate(user models.User, rows []playbackActivityRow) playbackProfileAggregate {
	hourCounts := make([]int64, 24)
	activeDays := make(map[string]struct{})
	deviceMap := make(map[string]*playbackDistributionAggregate)
	clientMap := make(map[string]*playbackDistributionAggregate)
	recentRecords := make([]PlaybackHistoryItem, 0, minPlaybackProfileRecentLimit(len(rows)))

	var totalPlayCount int64
	var totalPlayDuration int64
	var lastPlayedAt *time.Time
	var nightOwlCount int64
	var weekendCount int64
	var weekdayCount int64
	var lunchBreakCount int64
	var eveningCount int64

	playsByDay := make(map[string]int64)
	devicePlayCount := make(map[string]int64)

	for index, row := range rows {
		duration := row.playDuration
		if duration < 0 {
			duration = 0
		}

		totalPlayCount++
		totalPlayDuration += duration

		playedAt := parsePlaybackTime(row.playedAtRaw)
		if !playedAt.IsZero() {
			if lastPlayedAt == nil || playedAt.After(*lastPlayedAt) {
				playedAtCopy := playedAt
				lastPlayedAt = &playedAtCopy
			}

			dayKey := playedAt.Format(playbackDateFormat)
			activeDays[dayKey] = struct{}{}
			playsByDay[dayKey]++
			hourCounts[playedAt.Hour()]++

			if playedAt.Hour() >= 2 && playedAt.Hour() < 6 {
				nightOwlCount++
			}
			if playedAt.Hour() >= 11 && playedAt.Hour() < 14 {
				lunchBreakCount++
			}
			if playedAt.Hour() >= 19 && playedAt.Hour() < 24 {
				eveningCount++
			}
			if playedAt.Weekday() == time.Saturday || playedAt.Weekday() == time.Sunday {
				weekendCount++
			} else {
				weekdayCount++
			}
		}

		deviceName := strings.TrimSpace(row.deviceName)
		if deviceName == "" {
			deviceName = "未知设备"
		}
		clientName := strings.TrimSpace(row.clientName)
		if clientName == "" {
			clientName = "未知客户端"
		}

		deviceBucket := deviceMap[deviceName]
		if deviceBucket == nil {
			deviceBucket = &playbackDistributionAggregate{name: deviceName}
			deviceMap[deviceName] = deviceBucket
		}
		deviceBucket.count++
		deviceBucket.duration += duration
		devicePlayCount[deviceName]++

		clientBucket := clientMap[clientName]
		if clientBucket == nil {
			clientBucket = &playbackDistributionAggregate{name: clientName}
			clientMap[clientName] = clientBucket
		}
		clientBucket.count++
		clientBucket.duration += duration

		if index < playbackProfileRecentLimit {
			recentRecords = append(recentRecords, PlaybackHistoryItem{
				UserID:                user.ID,
				Username:              user.Username,
				ItemName:              row.itemName,
				ItemType:              row.itemType,
				PlayedAt:              playedAt,
				DeviceName:            deviceName,
				ClientName:            clientName,
				PlayDuration:          duration,
				PlayDurationFormatted: formatPlayDuration(duration),
			})
		}
	}

	hourly := make([]PlaybackProfileHourlyBucket, 24)
	for hour := 0; hour < 24; hour++ {
		hourly[hour] = PlaybackProfileHourlyBucket{
			Hour:  hour,
			Count: hourCounts[hour],
		}
	}

	var averageDuration int64
	if totalPlayCount > 0 {
		averageDuration = totalPlayDuration / totalPlayCount
	}

	return playbackProfileAggregate{
		totalPlayCount:    totalPlayCount,
		totalPlayDuration: totalPlayDuration,
		activeDays:        len(activeDays),
		averageDuration:   averageDuration,
		lastPlayedAt:      lastPlayedAt,
		hourly:            hourly,
		devices:           buildDeviceBuckets(deviceMap),
		clients:           buildClientBuckets(clientMap),
		badges: buildPlaybackProfileBadges(playbackProfileBadgeInput{
			totalPlayCount:    totalPlayCount,
			totalPlayDuration: totalPlayDuration,
			activeDays:        len(activeDays),
			averageDuration:   averageDuration,
			nightOwlCount:     nightOwlCount,
			lunchBreakCount:   lunchBreakCount,
			eveningCount:      eveningCount,
			weekdayCount:      weekdayCount,
			weekendCount:      weekendCount,
			deviceCount:       len(deviceMap),
			maxDailyPlayCount: maxDailyPlayCount(playsByDay),
			topDeviceShare:    topDeviceShare(devicePlayCount, totalPlayCount),
		}),
		recentRecords: recentRecords,
	}
}

func buildDeviceBuckets(source map[string]*playbackDistributionAggregate) []PlaybackProfileDeviceBucket {
	items := make([]PlaybackProfileDeviceBucket, 0, len(source))
	for _, item := range source {
		items = append(items, PlaybackProfileDeviceBucket{
			DeviceName:        item.name,
			Count:             item.count,
			Duration:          item.duration,
			DurationFormatted: formatPlayDuration(item.duration),
		})
	}

	sort.Slice(items, func(i, j int) bool {
		if items[i].Duration != items[j].Duration {
			return items[i].Duration > items[j].Duration
		}
		if items[i].Count != items[j].Count {
			return items[i].Count > items[j].Count
		}
		return items[i].DeviceName < items[j].DeviceName
	})

	return items
}

func buildClientBuckets(source map[string]*playbackDistributionAggregate) []PlaybackProfileClientBucket {
	items := make([]PlaybackProfileClientBucket, 0, len(source))
	for _, item := range source {
		items = append(items, PlaybackProfileClientBucket{
			ClientName:        item.name,
			Count:             item.count,
			Duration:          item.duration,
			DurationFormatted: formatPlayDuration(item.duration),
		})
	}

	sort.Slice(items, func(i, j int) bool {
		if items[i].Duration != items[j].Duration {
			return items[i].Duration > items[j].Duration
		}
		if items[i].Count != items[j].Count {
			return items[i].Count > items[j].Count
		}
		return items[i].ClientName < items[j].ClientName
	})

	return items
}

type playbackProfileBadgeInput struct {
	totalPlayCount    int64
	totalPlayDuration int64
	activeDays        int
	averageDuration   int64
	nightOwlCount     int64
	lunchBreakCount   int64
	eveningCount      int64
	weekdayCount      int64
	weekendCount      int64
	deviceCount       int
	maxDailyPlayCount int64
	topDeviceShare    float64
}

func buildPlaybackProfileBadges(input playbackProfileBadgeInput) []PlaybackProfileBadge {
	badges := make([]PlaybackProfileBadge, 0, 9)

	if input.eveningCount >= 4 && input.totalPlayCount > 0 && float64(input.eveningCount)/float64(input.totalPlayCount) >= 0.4 {
		badges = append(badges, PlaybackProfileBadge{
			ID:          "evening_viewer",
			Name:        "晚间档",
			Description: "19-24 点播放占比较高，偏好晚间看片",
		})
	}

	if input.lunchBreakCount >= 3 {
		badges = append(badges, PlaybackProfileBadge{
			ID:          "lunch_break_viewer",
			Name:        "午休党",
			Description: "11-14 点播放次数达到 3 次以上",
		})
	}

	if input.weekdayCount >= 5 && input.weekdayCount > input.weekendCount {
		badges = append(badges, PlaybackProfileBadge{
			ID:          "weekday_regular",
			Name:        "工作日常驻",
			Description: "工作日播放明显多于周末，看片节奏更稳定",
		})
	}

	if input.deviceCount >= 3 {
		badges = append(badges, PlaybackProfileBadge{
			ID:          "multi_device_user",
			Name:        "多端切换",
			Description: "使用设备达到 3 台以上，经常跨端观看",
		})
	} else if input.topDeviceShare >= 0.8 && input.totalPlayCount >= 5 {
		badges = append(badges, PlaybackProfileBadge{
			ID:          "single_device_loyalist",
			Name:        "专机用户",
			Description: "超过 80% 的播放集中在同一设备",
		})
	}

	if input.maxDailyPlayCount >= 4 {
		badges = append(badges, PlaybackProfileBadge{
			ID:          "binge_watcher",
			Name:        "连刷党",
			Description: "单日播放次数达到 4 次以上",
		})
	}

	if input.activeDays >= 7 {
		badges = append(badges, PlaybackProfileBadge{
			ID:          "steady_viewer",
			Name:        "稳定追更",
			Description: "当前时间窗口内活跃天数达到 7 天以上",
		})
	}

	if input.averageDuration >= 40*60 {
		badges = append(badges, PlaybackProfileBadge{
			ID:          "heavy_session_user",
			Name:        "长时观看",
			Description: "平均单次播放时长达到 40 分钟以上",
		})
	}

	if input.nightOwlCount >= 3 {
		badges = append(badges, PlaybackProfileBadge{
			ID:          "night_owl",
			Name:        "修仙党",
			Description: "凌晨 2-5 点播放次数达到 3 次以上",
		})
	}

	if input.weekendCount >= 6 {
		badges = append(badges, PlaybackProfileBadge{
			ID:          "weekend_warrior",
			Name:        "周末狂欢",
			Description: "周末播放次数达到 6 次以上",
		})
	}

	if input.totalPlayDuration >= 20*3600 {
		badges = append(badges, PlaybackProfileBadge{
			ID:          "hardcore_viewer",
			Name:        "Emby 肝帝",
			Description: "累计播放时长达到 20 小时以上",
		})
	}

	sort.SliceStable(badges, func(i, j int) bool {
		pi := playbackProfileBadgePriority(badges[i].ID)
		pj := playbackProfileBadgePriority(badges[j].ID)
		if pi != pj {
			return pi > pj
		}
		return badges[i].Name < badges[j].Name
	})

	if len(badges) > playbackProfileBadgeLimit {
		return badges[:playbackProfileBadgeLimit]
	}

	return badges
}

func playbackProfileBadgePriority(id string) int {
	switch id {
	case "evening_viewer":
		return 100
	case "hardcore_viewer":
		return 95
	case "night_owl":
		return 90
	case "weekend_warrior":
		return 85
	case "steady_viewer":
		return 80
	case "binge_watcher":
		return 75
	case "lunch_break_viewer":
		return 70
	case "weekday_regular":
		return 65
	case "heavy_session_user":
		return 60
	case "multi_device_user":
		return 55
	case "single_device_loyalist":
		return 50
	default:
		return 0
	}
}

func maxDailyPlayCount(playsByDay map[string]int64) int64 {
	var maxCount int64
	for _, count := range playsByDay {
		if count > maxCount {
			maxCount = count
		}
	}
	return maxCount
}

func topDeviceShare(devicePlayCount map[string]int64, totalPlayCount int64) float64 {
	if totalPlayCount <= 0 {
		return 0
	}

	var topCount int64
	for _, count := range devicePlayCount {
		if count > topCount {
			topCount = count
		}
	}

	return float64(topCount) / float64(totalPlayCount)
}

func minPlaybackProfileRecentLimit(size int) int {
	if size < playbackProfileRecentLimit {
		return size
	}
	return playbackProfileRecentLimit
}
