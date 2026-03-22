package playback

import (
	"context"
	"fmt"
	"log"
	"sort"
	"strings"
	"time"

	"github.com/konghang/ember/backend/internal/db"
	"github.com/konghang/ember/backend/internal/models"
)

const (
	defaultPlaybackProfileListPage       = 1
	defaultPlaybackProfileListPageSize   = 20
	maxPlaybackProfileListPageSize       = 100
	defaultPlaybackProfileListSortBy     = "totalDuration"
	defaultPlaybackProfileListSortOrder  = "desc"
	playbackProfileListBadgePreviewLimit = 2
)

type PlaybackProfileListQuery struct {
	Range     string `form:"range" json:"range"`
	Keyword   string `form:"keyword" json:"keyword"`
	SortBy    string `form:"sortBy" json:"sortBy"`
	SortOrder string `form:"sortOrder" json:"sortOrder"`
	Page      int    `form:"page" json:"page"`
	PageSize  int    `form:"pageSize" json:"pageSize"`
}

type PlaybackProfileListItem struct {
	UserID                     string                 `json:"userId"`
	Username                   string                 `json:"username"`
	Range                      string                 `json:"range"`
	TotalPlayCount             int64                  `json:"totalPlayCount"`
	TotalPlayDuration          int64                  `json:"totalPlayDuration"`
	TotalPlayDurationFormatted string                 `json:"totalPlayDurationFormatted"`
	ActiveDays                 int                    `json:"activeDays"`
	LastPlayedAt               *time.Time             `json:"lastPlayedAt"`
	PeakHour                   *int                   `json:"peakHour"`
	PeakHourLabel              string                 `json:"peakHourLabel"`
	Badges                     []PlaybackProfileBadge `json:"badges"`
}

type PlaybackProfileListSummary struct {
	UserCount                  int64  `json:"userCount"`
	TotalPlayCount             int64  `json:"totalPlayCount"`
	TotalPlayDuration          int64  `json:"totalPlayDuration"`
	TotalPlayDurationFormatted string `json:"totalPlayDurationFormatted"`
}

type PlaybackProfileListResponse struct {
	Data     []PlaybackProfileListItem  `json:"data"`
	Total    int64                      `json:"total"`
	Page     int                        `json:"page"`
	PageSize int                        `json:"pageSize"`
	Summary  PlaybackProfileListSummary `json:"summary"`
}

type normalizedPlaybackProfileListQuery struct {
	rangeValue      string
	startAt         *time.Time
	keyword         string
	sortBy          string
	sortOrder       string
	page            int
	pageSize        int
	keywordMiss     bool
	playbackUserIDs []string
}

func (s *UserPlaybackProfileService) GetUserProfilesOverview(ctx context.Context, req PlaybackProfileListQuery) (*PlaybackProfileListResponse, error) {
	startedAt := time.Now()
	query, err := s.normalizePlaybackProfileListQuery(ctx, req)
	if err != nil {
		return nil, err
	}
	log.Printf(
		"[PlaybackProfileOverview] start range=%s keyword=%q sortBy=%s sortOrder=%s page=%d pageSize=%d filteredPlaybackUsers=%d keywordMiss=%t",
		query.rangeValue,
		query.keyword,
		query.sortBy,
		query.sortOrder,
		query.page,
		query.pageSize,
		len(query.playbackUserIDs),
		query.keywordMiss,
	)
	if query.keywordMiss {
		log.Printf("[PlaybackProfileOverview] keyword miss range=%s keyword=%q cost=%s", query.rangeValue, query.keyword, time.Since(startedAt))
		return &PlaybackProfileListResponse{
			Data:     []PlaybackProfileListItem{},
			Total:    0,
			Page:     query.page,
			PageSize: query.pageSize,
			Summary:  PlaybackProfileListSummary{},
		}, nil
	}

	rows, err := s.loadPlaybackProfileRowsForOverview(query.playbackUserIDs, query.startAt)
	if err != nil {
		log.Printf("[PlaybackProfileOverview] playback query failed range=%s keyword=%q err=%v", query.rangeValue, query.keyword, err)
		return nil, fmt.Errorf("%w: %v", ErrPlaybackHistoryQueryFailed, err)
	}

	userMap, err := s.loadLocalUsersForOverview(ctx, rows)
	if err != nil {
		return nil, err
	}

	items := s.buildPlaybackProfileListItems(query.rangeValue, rows, userMap)
	sortPlaybackProfileListItems(items, query.sortBy, query.sortOrder)

	total := int64(len(items))
	summary := buildPlaybackProfileListSummary(items)

	offset := (query.page - 1) * query.pageSize
	if offset >= len(items) {
		log.Printf(
			"[PlaybackProfileOverview] done range=%s keyword=%q rows=%d mappedUsers=%d overviewUsers=%d page=%d pageSize=%d returned=0 cost=%s",
			query.rangeValue,
			query.keyword,
			len(rows),
			len(userMap),
			len(items),
			query.page,
			query.pageSize,
			time.Since(startedAt),
		)
		return &PlaybackProfileListResponse{
			Data:     []PlaybackProfileListItem{},
			Total:    total,
			Page:     query.page,
			PageSize: query.pageSize,
			Summary:  summary,
		}, nil
	}

	end := offset + query.pageSize
	if end > len(items) {
		end = len(items)
	}
	resp := &PlaybackProfileListResponse{
		Data:     items[offset:end],
		Total:    total,
		Page:     query.page,
		PageSize: query.pageSize,
		Summary:  summary,
	}
	log.Printf(
		"[PlaybackProfileOverview] done range=%s keyword=%q rows=%d mappedUsers=%d overviewUsers=%d page=%d pageSize=%d returned=%d cost=%s",
		query.rangeValue,
		query.keyword,
		len(rows),
		len(userMap),
		len(items),
		query.page,
		query.pageSize,
		end-offset,
		time.Since(startedAt),
	)
	return resp, nil
}

func (s *UserPlaybackProfileService) normalizePlaybackProfileListQuery(ctx context.Context, req PlaybackProfileListQuery) (*normalizedPlaybackProfileListQuery, error) {
	rangeValue, startAt, err := normalizePlaybackProfileRange(req.Range)
	if err != nil {
		return nil, err
	}

	keyword := strings.TrimSpace(req.Keyword)
	if keyword != "" {
		if len([]rune(keyword)) > 100 || !playbackKeywordPattern.MatchString(keyword) {
			return nil, ErrPlaybackHistoryInvalidKeyword
		}
	}

	sortBy := strings.TrimSpace(req.SortBy)
	switch sortBy {
	case "", "totalDuration", "playCount", "activeDays", "lastPlayedAt":
		if sortBy == "" {
			sortBy = defaultPlaybackProfileListSortBy
		}
	default:
		sortBy = defaultPlaybackProfileListSortBy
	}

	sortOrder := strings.ToLower(strings.TrimSpace(req.SortOrder))
	switch sortOrder {
	case "", "desc", "asc":
		if sortOrder == "" {
			sortOrder = defaultPlaybackProfileListSortOrder
		}
	default:
		sortOrder = defaultPlaybackProfileListSortOrder
	}

	page := req.Page
	if page <= 0 {
		page = defaultPlaybackProfileListPage
	}

	pageSize := req.PageSize
	if pageSize <= 0 {
		pageSize = defaultPlaybackProfileListPageSize
	}
	if pageSize > maxPlaybackProfileListPageSize {
		pageSize = maxPlaybackProfileListPageSize
	}

	var playbackUserIDs []string
	if keyword != "" {
		users, err := s.findUsersByKeyword(ctx, keyword)
		if err != nil {
			return nil, err
		}
		if len(users) == 0 {
			playbackUserIDs = []string{}
		} else {
			playbackUserIDs = make([]string, 0, len(users))
			seen := make(map[string]struct{}, len(users))
			for _, user := range users {
				playbackUserID := strings.TrimSpace(user.EmbyID)
				if playbackUserID == "" {
					playbackUserID = strings.TrimSpace(user.ID)
				}
				if playbackUserID == "" {
					continue
				}
				if _, exists := seen[playbackUserID]; exists {
					continue
				}
				seen[playbackUserID] = struct{}{}
				playbackUserIDs = append(playbackUserIDs, playbackUserID)
			}
		}
	}

	return &normalizedPlaybackProfileListQuery{
		rangeValue:      rangeValue,
		startAt:         startAt,
		keyword:         keyword,
		sortBy:          sortBy,
		sortOrder:       sortOrder,
		page:            page,
		pageSize:        pageSize,
		keywordMiss:     keyword != "" && len(playbackUserIDs) == 0,
		playbackUserIDs: playbackUserIDs,
	}, nil
}

func (s *UserPlaybackProfileService) findUsersByKeyword(ctx context.Context, keyword string) ([]models.User, error) {
	var users []models.User
	if err := db.DB.WithContext(ctx).
		Select("id", "username", "\"embyId\"").
		Where("username ILIKE ?", "%"+keyword+"%").
		Order("username ASC").
		Find(&users).Error; err != nil {
		return nil, fmt.Errorf("查询用户失败: %w", err)
	}
	log.Printf("[PlaybackProfileOverview] keyword matched keyword=%q users=%d", keyword, len(users))
	return users, nil
}

func (s *UserPlaybackProfileService) loadPlaybackProfileRowsForOverview(playbackUserIDs []string, startAt *time.Time) ([]playbackActivityRow, error) {
	whereClause := buildPlaybackProfileOverviewWhereClause(playbackUserIDs, startAt)
	querySQL := fmt.Sprintf(`
SELECT *
FROM PlaybackActivity
WHERE %s
ORDER BY DateCreated DESC
`, whereClause)

	resp, err := s.embyService.QueryPlaybackStats(querySQL)
	if err != nil {
		fallbackSQL := fmt.Sprintf(`
SELECT *
FROM PlaybackActivity
WHERE %s
`, whereClause)
		resp, err = s.embyService.QueryPlaybackStats(fallbackSQL)
		if err != nil {
			return nil, err
		}
	}

	rows, err := parsePlaybackRows(resp)
	if err != nil {
		return nil, fmt.Errorf("解析用户画像总览播放记录失败: %w", err)
	}
	log.Printf("[PlaybackProfileOverview] rows loaded rangeStart=%s filterUsers=%d rows=%d", formatOptionalTime(startAt), len(playbackUserIDs), len(rows))

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

func buildPlaybackProfileOverviewWhereClause(playbackUserIDs []string, startAt *time.Time) string {
	conditions := []string{"1=1"}
	if len(playbackUserIDs) > 0 {
		quotedUserIDs := make([]string, 0, len(playbackUserIDs))
		for _, playbackUserID := range playbackUserIDs {
			quotedUserIDs = append(quotedUserIDs, fmt.Sprintf("'%s'", escapeSQLLiteral(playbackUserID)))
		}
		conditions = append(conditions, fmt.Sprintf("UserId IN (%s)", strings.Join(quotedUserIDs, ", ")))
	}
	if startAt != nil {
		conditions = append(conditions, fmt.Sprintf("DateCreated >= '%s'", startAt.UTC().Format("2006-01-02 15:04:05")))
	}
	return strings.Join(conditions, " AND ")
}

func (s *UserPlaybackProfileService) loadLocalUsersForOverview(ctx context.Context, rows []playbackActivityRow) (map[string]models.User, error) {
	playbackUserIDSet := make(map[string]struct{}, len(rows))
	for _, row := range rows {
		if strings.TrimSpace(row.embyUserID) == "" {
			continue
		}
		playbackUserIDSet[row.embyUserID] = struct{}{}
	}
	if len(playbackUserIDSet) == 0 {
		return map[string]models.User{}, nil
	}

	identifiers := make([]string, 0, len(playbackUserIDSet))
	for id := range playbackUserIDSet {
		identifiers = append(identifiers, id)
	}

	var users []models.User
	if err := db.DB.WithContext(ctx).
		Select("id", "username", "\"embyId\"").
		Where("\"embyId\" IN ? OR id IN ?", identifiers, identifiers).
		Find(&users).Error; err != nil {
		return nil, fmt.Errorf("查询本地用户映射失败: %w", err)
	}

	userMap := make(map[string]models.User, len(users)*2)
	for _, user := range users {
		if trimmedEmbyID := strings.TrimSpace(user.EmbyID); trimmedEmbyID != "" {
			userMap[trimmedEmbyID] = user
		}
		if trimmedID := strings.TrimSpace(user.ID); trimmedID != "" {
			userMap[trimmedID] = user
		}
	}
	log.Printf("[PlaybackProfileOverview] mapped local users identifiers=%d users=%d mapKeys=%d", len(identifiers), len(users), len(userMap))

	return userMap, nil
}

func (s *UserPlaybackProfileService) buildPlaybackProfileListItems(rangeValue string, rows []playbackActivityRow, userMap map[string]models.User) []PlaybackProfileListItem {
	groupedRows := make(map[string][]playbackActivityRow)
	groupedUsers := make(map[string]models.User)

	for _, row := range rows {
		localUser, ok := userMap[row.embyUserID]
		if !ok {
			continue
		}
		groupedRows[localUser.ID] = append(groupedRows[localUser.ID], row)
		groupedUsers[localUser.ID] = localUser
	}

	items := make([]PlaybackProfileListItem, 0, len(groupedRows))
	for userID, userRows := range groupedRows {
		user := groupedUsers[userID]
		aggregate := buildPlaybackProfileAggregate(user, userRows)
		peakHour, peakHourLabel := resolvePeakHour(aggregate.hourly)
		items = append(items, PlaybackProfileListItem{
			UserID:                     user.ID,
			Username:                   user.Username,
			Range:                      rangeValue,
			TotalPlayCount:             aggregate.totalPlayCount,
			TotalPlayDuration:          aggregate.totalPlayDuration,
			TotalPlayDurationFormatted: formatPlayDuration(aggregate.totalPlayDuration),
			ActiveDays:                 aggregate.activeDays,
			LastPlayedAt:               aggregate.lastPlayedAt,
			PeakHour:                   peakHour,
			PeakHourLabel:              peakHourLabel,
			Badges:                     previewPlaybackProfileBadges(aggregate.badges),
		})
	}

	return items
}

func previewPlaybackProfileBadges(badges []PlaybackProfileBadge) []PlaybackProfileBadge {
	if len(badges) <= playbackProfileListBadgePreviewLimit {
		return badges
	}
	return badges[:playbackProfileListBadgePreviewLimit]
}

func resolvePeakHour(hourly []PlaybackProfileHourlyBucket) (*int, string) {
	var peak *PlaybackProfileHourlyBucket
	for idx := range hourly {
		item := hourly[idx]
		if item.Count <= 0 {
			continue
		}
		if peak == nil || item.Count > peak.Count || (item.Count == peak.Count && item.Hour < peak.Hour) {
			itemCopy := item
			peak = &itemCopy
		}
	}
	if peak == nil {
		return nil, ""
	}
	hour := peak.Hour
	return &hour, fmt.Sprintf("%02d:00 - %02d:00", hour, (hour+1)%24)
}

func sortPlaybackProfileListItems(items []PlaybackProfileListItem, sortBy string, sortOrder string) {
	desc := sortOrder != "asc"
	sort.SliceStable(items, func(i, j int) bool {
		left := items[i]
		right := items[j]

		compareInt64 := func(a, b int64) bool {
			if a == b {
				return left.Username < right.Username
			}
			if desc {
				return a > b
			}
			return a < b
		}

		switch sortBy {
		case "playCount":
			return compareInt64(left.TotalPlayCount, right.TotalPlayCount)
		case "activeDays":
			return compareInt64(int64(left.ActiveDays), int64(right.ActiveDays))
		case "lastPlayedAt":
			leftTime := left.LastPlayedAt
			rightTime := right.LastPlayedAt
			if leftTime == nil && rightTime == nil {
				return left.Username < right.Username
			}
			if leftTime == nil {
				return !desc
			}
			if rightTime == nil {
				return desc
			}
			if leftTime.Equal(*rightTime) {
				return left.Username < right.Username
			}
			if desc {
				return leftTime.After(*rightTime)
			}
			return leftTime.Before(*rightTime)
		default:
			return compareInt64(left.TotalPlayDuration, right.TotalPlayDuration)
		}
	})
}

func buildPlaybackProfileListSummary(items []PlaybackProfileListItem) PlaybackProfileListSummary {
	var totalPlayCount int64
	var totalPlayDuration int64

	for _, item := range items {
		totalPlayCount += item.TotalPlayCount
		totalPlayDuration += item.TotalPlayDuration
	}

	return PlaybackProfileListSummary{
		UserCount:                  int64(len(items)),
		TotalPlayCount:             totalPlayCount,
		TotalPlayDuration:          totalPlayDuration,
		TotalPlayDurationFormatted: formatPlayDuration(totalPlayDuration),
	}
}
