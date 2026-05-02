package playback

import (
	"context"
	"errors"
	"fmt"
	"log"
	"sort"
	"strings"
	"time"

	"github.com/konghang/ember/backend/internal/db"
	embyint "github.com/konghang/ember/backend/internal/integrations/emby"
	"github.com/konghang/ember/backend/internal/models"
)

const (
	defaultPlaybackProfileListPage       = 1
	defaultPlaybackProfileListPageSize   = 20
	maxPlaybackProfileListPageSize       = 100
	defaultPlaybackProfileListSortBy     = "totalDuration"
	defaultPlaybackProfileListSortOrder  = "desc"
	playbackProfileListBadgePreviewLimit = 2
	maxPlaybackProfileOverviewRows       = 20000
)

type PlaybackProfileListQuery struct {
	Range     string `form:"range" json:"range"`
	StartDate string `form:"startDate" json:"startDate"`
	EndDate   string `form:"endDate" json:"endDate"`
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
	endAt           *time.Time
	keyword         string
	sortBy          string
	sortOrder       string
	page            int
	pageSize        int
	keywordMiss     bool
	playbackUserIDs []string
}

type playbackProfileOverviewAggregateRow struct {
	playbackUserID    string
	totalPlayCount    int64
	totalPlayDuration int64
	activeDays        int
	lastPlayedAt      *time.Time
	lastPlayedAtRaw   string
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

	totalRows, err := s.countPlaybackProfileOverviewRows(query.playbackUserIDs, query.startAt, query.endAt)
	if err != nil {
		log.Printf("[PlaybackProfileOverview] playback query failed range=%s keyword=%q err=%v", query.rangeValue, query.keyword, err)
		return nil, fmt.Errorf("%w: %v", ErrPlaybackHistoryQueryFailed, err)
	}
	if totalRows > maxPlaybackProfileOverviewRows {
		return nil, ErrPlaybackProfileOverviewTooLarge
	}

	aggregateRows, err := s.loadPlaybackProfileOverviewAggregateRows(query.playbackUserIDs, query.startAt, query.endAt)
	if err != nil {
		log.Printf("[PlaybackProfileOverview] aggregate query failed range=%s keyword=%q err=%v", query.rangeValue, query.keyword, err)
		if errors.Is(err, ErrPlaybackHistorySchemaUnsupported) {
			return nil, err
		}
		return nil, fmt.Errorf("%w: %v", ErrPlaybackHistoryQueryFailed, err)
	}

	userMap, err := s.loadLocalUsersForOverview(ctx, overviewAggregateIdentifiers(aggregateRows))
	if err != nil {
		return nil, err
	}

	items, localToPlaybackUserID := s.buildPlaybackProfileListItems(query.rangeValue, aggregateRows, userMap)
	sortPlaybackProfileListItems(items, query.sortBy, query.sortOrder)

	total := int64(len(items))
	summary := buildPlaybackProfileListSummary(items)

	offset := (query.page - 1) * query.pageSize
	if offset >= len(items) {
		log.Printf(
			"[PlaybackProfileOverview] done range=%s keyword=%q rows=%d mappedUsers=%d overviewUsers=%d page=%d pageSize=%d returned=0 cost=%s",
			query.rangeValue,
			query.keyword,
			totalRows,
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

	pageItems := append([]PlaybackProfileListItem(nil), items[offset:end]...)
	pagePlaybackUserIDs := make([]string, 0, len(pageItems))
	for _, item := range pageItems {
		if playbackUserID := strings.TrimSpace(localToPlaybackUserID[item.UserID]); playbackUserID != "" {
			pagePlaybackUserIDs = append(pagePlaybackUserIDs, playbackUserID)
		}
	}
	if len(pagePlaybackUserIDs) > 0 {
		pageRows, pageRowsErr := s.loadPlaybackProfileRowsForOverviewPage(pagePlaybackUserIDs, query.startAt, query.endAt)
		if pageRowsErr != nil {
			log.Printf("[PlaybackProfileOverview] page enrichment failed range=%s keyword=%q page=%d pageSize=%d err=%v", query.rangeValue, query.keyword, query.page, query.pageSize, pageRowsErr)
			if errors.Is(pageRowsErr, ErrPlaybackHistorySchemaUnsupported) {
				return nil, pageRowsErr
			}
			return nil, fmt.Errorf("%w: %v", ErrPlaybackHistoryQueryFailed, pageRowsErr)
		}
		pageItems = s.enrichPlaybackProfileListItems(pageItems, pageRows, userMap)
	}

	resp := &PlaybackProfileListResponse{
		Data:     pageItems,
		Total:    total,
		Page:     query.page,
		PageSize: query.pageSize,
		Summary:  summary,
	}
	log.Printf(
		"[PlaybackProfileOverview] done range=%s keyword=%q rows=%d mappedUsers=%d overviewUsers=%d page=%d pageSize=%d returned=%d cost=%s",
		query.rangeValue,
		query.keyword,
		totalRows,
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
	rangeValue, startAt, endAt, err := normalizePlaybackProfileRange(PlaybackProfileQuery{
		Range:     req.Range,
		StartDate: req.StartDate,
		EndDate:   req.EndDate,
	})
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
		endAt:           endAt,
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
		Select("id", "username", "\"emby_id\"").
		Where("username ILIKE ?", "%"+keyword+"%").
		Order("username ASC").
		Find(&users).Error; err != nil {
		return nil, fmt.Errorf("查询用户失败: %w", err)
	}
	log.Printf("[PlaybackProfileOverview] keyword matched keyword=%q users=%d", keyword, len(users))
	return users, nil
}

func (s *UserPlaybackProfileService) countPlaybackProfileOverviewRows(playbackUserIDs []string, startAt *time.Time, endAt *time.Time) (int64, error) {
	whereClause := buildPlaybackProfileOverviewWhereClause(playbackUserIDs, startAt, endAt)
	countSQL := fmt.Sprintf(`
SELECT COUNT(1) AS total
FROM PlaybackActivity
WHERE %s
`, whereClause)
	countResp, err := s.queryPlaybackStats(countSQL)
	if err != nil {
		return 0, err
	}
	totalRows, err := parsePlaybackCount(countResp)
	if err != nil {
		return 0, fmt.Errorf("解析用户画像总览记录总数失败: %w", err)
	}
	return totalRows, nil
}

func (s *UserPlaybackProfileService) loadPlaybackProfileOverviewAggregateRows(playbackUserIDs []string, startAt *time.Time, endAt *time.Time) ([]playbackProfileOverviewAggregateRow, error) {
	whereClause := buildPlaybackProfileOverviewWhereClause(playbackUserIDs, startAt, endAt)
	localDateExpr := playbackSQLiteLocalDateExpr("DateCreated")
	querySQL := fmt.Sprintf(`
SELECT
  UserId,
  COUNT(1) AS TotalPlayCount,
  SUM(COALESCE(PlayDuration, 0) - COALESCE(PauseDuration, 0)) AS TotalPlayDuration,
  COUNT(DISTINCT %s) AS ActiveDays,
  MAX(DateCreated) AS LastPlayedAt
FROM PlaybackActivity
WHERE %s
GROUP BY UserId
`, localDateExpr, whereClause)

	resp, err := s.queryPlaybackStats(querySQL)
	if err != nil {
		return nil, err
	}

	rows, err := parsePlaybackProfileOverviewAggregateRows(resp)
	if err != nil {
		return nil, fmt.Errorf("解析用户画像总览聚合结果失败: %w", err)
	}
	log.Printf("[PlaybackProfileOverview] aggregate rows loaded rangeStart=%s rangeEnd=%s filterUsers=%d aggregateUsers=%d", formatOptionalTime(startAt), formatOptionalTime(endAt), len(playbackUserIDs), len(rows))
	return rows, nil
}

func (s *UserPlaybackProfileService) loadPlaybackProfileRowsForOverviewPage(playbackUserIDs []string, startAt *time.Time, endAt *time.Time) ([]playbackActivityRow, error) {
	whereClause := buildPlaybackProfileOverviewWhereClause(playbackUserIDs, startAt, endAt)

	detailResp, err := s.queryPlaybackStats(fmt.Sprintf(`
SELECT UserId, UserName, ItemName, ItemType, DateCreated, DeviceName, ClientName,
       COALESCE(PlayDuration, 0) - COALESCE(PauseDuration, 0) AS PlayDuration
FROM PlaybackActivity
WHERE %s
ORDER BY DateCreated DESC
`, whereClause))
	if err != nil && shouldFallbackPlaybackDetailError(err) {
		detailResp, err = s.queryPlaybackStats(fmt.Sprintf(`
SELECT UserId, UserName, ItemName, ItemType, DateCreated, DeviceName, ClientName,
       COALESCE(PlayDuration, 0) AS PlayDuration
FROM PlaybackActivity
WHERE %s
ORDER BY DateCreated DESC
`, whereClause))
	}
	if err != nil || shouldFallbackPlaybackDetailQuery(detailResp) {
		detailResp, err = s.queryPlaybackStats(fmt.Sprintf(`
SELECT *
FROM PlaybackActivity
WHERE %s
ORDER BY DateCreated DESC
`, whereClause))
	}
	if err != nil || shouldFallbackPlaybackDetailQuery(detailResp) {
		return nil, ErrPlaybackHistorySchemaUnsupported
	}

	rows, err := parsePlaybackRows(detailResp)
	if err != nil {
		return nil, fmt.Errorf("解析用户画像总览页明细失败: %w", err)
	}
	log.Printf("[PlaybackProfileOverview] page rows loaded rangeStart=%s rangeEnd=%s filterUsers=%d rows=%d", formatOptionalTime(startAt), formatOptionalTime(endAt), len(playbackUserIDs), len(rows))
	return rows, nil
}

func playbackSQLiteLocalDateExpr(column string) string {
	loc := loadPlaybackTimezone()
	_, offsetSeconds := time.Now().In(loc).Zone()
	sign := "+"
	if offsetSeconds < 0 {
		sign = "-"
		offsetSeconds = -offsetSeconds
	}
	hours := offsetSeconds / 3600
	minutes := (offsetSeconds % 3600) / 60
	return fmt.Sprintf("strftime('%%Y-%%m-%%d', datetime(%s, '%s%02d:%02d'))", column, sign, hours, minutes)
}

func parsePlaybackProfileOverviewAggregateRows(resp *embyint.CustomQueryResponse) ([]playbackProfileOverviewAggregateRow, error) {
	if resp == nil || len(resp.Results) == 0 {
		return []playbackProfileOverviewAggregateRow{}, nil
	}

	indexes := map[string]int{}
	columns := resp.Colums
	if len(columns) == 0 {
		columns = resp.Columns
	}
	if len(columns) == 0 {
		return nil, ErrPlaybackHistorySchemaUnsupported
	}
	for idx, col := range columns {
		indexes[strings.ToLower(strings.TrimSpace(col))] = idx
	}
	requiredColumns := []string{"userid", "totalplaycount", "totalplayduration", "activedays", "lastplayedat"}
	for _, column := range requiredColumns {
		if _, ok := indexes[column]; !ok {
			return nil, fmt.Errorf("%w: missing column %s", ErrPlaybackHistorySchemaUnsupported, column)
		}
	}

	get := func(row []interface{}, key string) interface{} {
		idx, ok := indexes[key]
		if !ok || idx < 0 || idx >= len(row) {
			return nil
		}
		return row[idx]
	}

	result := make([]playbackProfileOverviewAggregateRow, 0, len(resp.Results))
	for _, row := range resp.Results {
		totalPlayCount, err := asInt64(get(row, "totalplaycount"))
		if err != nil {
			return nil, err
		}
		totalPlayDuration, err := asInt64(get(row, "totalplayduration"))
		if err != nil {
			return nil, err
		}
		activeDays, err := asInt(get(row, "activedays"))
		if err != nil {
			return nil, err
		}
		lastPlayedAtRaw := safeString(get(row, "lastplayedat"))
		lastPlayedAt := parsePlaybackTime(lastPlayedAtRaw)
		var lastPlayedAtPtr *time.Time
		if !lastPlayedAt.IsZero() {
			lastPlayedAtCopy := lastPlayedAt
			lastPlayedAtPtr = &lastPlayedAtCopy
		}

		result = append(result, playbackProfileOverviewAggregateRow{
			playbackUserID:    safeString(get(row, "userid")),
			totalPlayCount:    totalPlayCount,
			totalPlayDuration: totalPlayDuration,
			activeDays:        activeDays,
			lastPlayedAt:      lastPlayedAtPtr,
			lastPlayedAtRaw:   lastPlayedAtRaw,
		})
	}

	return result, nil
}

func buildPlaybackProfileOverviewWhereClause(playbackUserIDs []string, startAt *time.Time, endAt *time.Time) string {
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
	if endAt != nil {
		conditions = append(conditions, fmt.Sprintf("DateCreated <= '%s'", endAt.UTC().Format("2006-01-02 15:04:05")))
	}
	return strings.Join(conditions, " AND ")
}

func overviewAggregateIdentifiers(rows []playbackProfileOverviewAggregateRow) []string {
	identifiers := make([]string, 0, len(rows))
	for _, row := range rows {
		if trimmed := strings.TrimSpace(row.playbackUserID); trimmed != "" {
			identifiers = append(identifiers, trimmed)
		}
	}
	return identifiers
}

func (s *UserPlaybackProfileService) loadLocalUsersForOverview(ctx context.Context, identifiers []string) (map[string]models.User, error) {
	if len(identifiers) == 0 {
		return map[string]models.User{}, nil
	}

	var users []models.User
	if err := db.DB.WithContext(ctx).
		Select("id", "username", "\"emby_id\"").
		Where("\"emby_id\" IN ? OR id IN ?", identifiers, identifiers).
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

func (s *UserPlaybackProfileService) buildPlaybackProfileListItems(rangeValue string, rows []playbackProfileOverviewAggregateRow, userMap map[string]models.User) ([]PlaybackProfileListItem, map[string]string) {
	items := make([]PlaybackProfileListItem, 0, len(rows))
	localToPlaybackUserID := make(map[string]string, len(rows))
	for _, row := range rows {
		localUser, ok := userMap[row.playbackUserID]
		if !ok {
			continue
		}
		items = append(items, PlaybackProfileListItem{
			UserID:                     localUser.ID,
			Username:                   localUser.Username,
			Range:                      rangeValue,
			TotalPlayCount:             row.totalPlayCount,
			TotalPlayDuration:          row.totalPlayDuration,
			TotalPlayDurationFormatted: formatPlayDuration(row.totalPlayDuration),
			ActiveDays:                 row.activeDays,
			LastPlayedAt:               row.lastPlayedAt,
		})
		localToPlaybackUserID[localUser.ID] = row.playbackUserID
	}

	return items, localToPlaybackUserID
}

func (s *UserPlaybackProfileService) enrichPlaybackProfileListItems(items []PlaybackProfileListItem, rows []playbackActivityRow, userMap map[string]models.User) []PlaybackProfileListItem {
	if len(items) == 0 || len(rows) == 0 {
		return items
	}

	groupedRows := make(map[string][]playbackActivityRow)
	for _, row := range rows {
		localUser, ok := userMap[row.embyUserID]
		if !ok {
			continue
		}
		groupedRows[localUser.ID] = append(groupedRows[localUser.ID], row)
	}

	for index := range items {
		userRows := groupedRows[items[index].UserID]
		if len(userRows) == 0 {
			continue
		}
		localUser := userMap[items[index].UserID]
		aggregate := buildPlaybackProfileAggregate(localUser, userRows)
		peakHour, peakHourLabel := resolvePeakHour(aggregate.hourly)
		items[index].PeakHour = peakHour
		items[index].PeakHourLabel = peakHourLabel
		items[index].Badges = previewPlaybackProfileBadges(aggregate.badges)
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
