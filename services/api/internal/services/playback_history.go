package services

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/konghang/ember/backend/internal/db"
	"github.com/konghang/ember/backend/internal/models"
	"gorm.io/gorm"
)

const (
	defaultPlaybackHistoryPage     = 1
	defaultPlaybackHistoryPageSize = 20
	maxPlaybackHistoryPageSize     = 100
	playbackDateFormat             = "2006-01-02"
)

var playbackKeywordPattern = regexp.MustCompile(`^[\p{Han}\p{L}\p{N}._'\- ]+$`)
var playbackUserIDPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,50}$`)

type PlaybackHistoryService struct {
	embyService *EmbyService
}

type PlaybackHistoryRequest struct {
	UserID    string `form:"userId" json:"userId"`
	Keyword   string `form:"keyword" json:"keyword"`
	StartDate string `form:"startDate" json:"startDate"`
	EndDate   string `form:"endDate" json:"endDate"`
	Page      int    `form:"page" json:"page"`
	PageSize  int    `form:"pageSize" json:"pageSize"`
}

type PlaybackHistoryItem struct {
	UserID                string    `json:"userId"`
	Username              string    `json:"username"`
	ItemName              string    `json:"itemName"`
	ItemType              string    `json:"itemType"`
	PlayedAt              time.Time `json:"playedAt"`
	DeviceName            string    `json:"deviceName"`
	ClientName            string    `json:"clientName"`
	PlayDuration          int64     `json:"playDuration"`
	PlayDurationFormatted string    `json:"playDurationFormatted"`
}

type PlaybackHistoryResponse struct {
	Data     []PlaybackHistoryItem `json:"data"`
	Total    int64                 `json:"total"`
	Page     int                   `json:"page"`
	PageSize int                   `json:"pageSize"`
}

type playbackHistoryQuery struct {
	PlaybackHistoryRequest
	startDate *time.Time
	endDate   *time.Time
}

type playbackActivityRow struct {
	embyUserID   string
	username     string
	itemName     string
	itemType     string
	playedAtRaw  string
	deviceName   string
	clientName   string
	playDuration int64
}

func NewPlaybackHistoryService() *PlaybackHistoryService {
	return &PlaybackHistoryService{
		embyService: NewEmbyService(),
	}
}

func (s *PlaybackHistoryService) GetPlaybackHistory(ctx context.Context, req PlaybackHistoryRequest) (*PlaybackHistoryResponse, error) {
	query, embyUserID, err := s.normalizeRequest(req)
	if err != nil {
		return nil, err
	}

	if query.UserID != "" && embyUserID == "" {
		return &PlaybackHistoryResponse{
			Data:     []PlaybackHistoryItem{},
			Total:    0,
			Page:     query.Page,
			PageSize: query.PageSize,
		}, nil
	}

	whereClause := buildPlaybackWhereClause(query, embyUserID)
	countSQL := fmt.Sprintf("SELECT COUNT(1) AS total FROM PlaybackActivity WHERE %s", whereClause)

	countResp, err := s.embyService.QueryPlaybackStats(countSQL)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrPlaybackHistoryQueryFailed, err)
	}

	total, err := parsePlaybackCount(countResp)
	if err != nil {
		return nil, fmt.Errorf("解析播放历史总数失败: %w", err)
	}

	if total == 0 {
		return &PlaybackHistoryResponse{
			Data:     []PlaybackHistoryItem{},
			Total:    0,
			Page:     query.Page,
			PageSize: query.PageSize,
		}, nil
	}

	offset := (query.Page - 1) * query.PageSize
	if int64(offset) >= total {
		return &PlaybackHistoryResponse{
			Data:     []PlaybackHistoryItem{},
			Total:    total,
			Page:     query.Page,
			PageSize: query.PageSize,
		}, nil
	}

	rows, err := s.loadPlaybackRowsWithFallback(whereClause, query.PageSize, offset)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrPlaybackHistoryQueryFailed, err)
	}

	localUsers, err := s.loadUsersByEmbyIDs(ctx, rows)
	if err != nil {
		return nil, err
	}

	items := make([]PlaybackHistoryItem, 0, len(rows))
	for _, row := range rows {
		userID := row.embyUserID
		username := strings.TrimSpace(row.username)
		if localUser, ok := localUsers[row.embyUserID]; ok {
			if strings.TrimSpace(localUser.ID) != "" {
				userID = localUser.ID
			}
			if strings.TrimSpace(localUser.Username) != "" {
				username = localUser.Username
			}
		}

		playDuration := row.playDuration
		if playDuration < 0 {
			playDuration = 0
		}

		items = append(items, PlaybackHistoryItem{
			UserID:                userID,
			Username:              username,
			ItemName:              row.itemName,
			ItemType:              row.itemType,
			PlayedAt:              parsePlaybackTime(row.playedAtRaw),
			DeviceName:            row.deviceName,
			ClientName:            row.clientName,
			PlayDuration:          playDuration,
			PlayDurationFormatted: formatPlayDuration(playDuration),
		})
	}

	return &PlaybackHistoryResponse{
		Data:     items,
		Total:    total,
		Page:     query.Page,
		PageSize: query.PageSize,
	}, nil
}

func (s *PlaybackHistoryService) queryPlaybackDetails(whereClause string, pageSize int, offset int, includePauseDuration bool) (*CustomQueryResponse, error) {
	durationExpr := "COALESCE(PlayDuration, 0) AS PlayDuration"
	if includePauseDuration {
		durationExpr = "COALESCE(PlayDuration, 0) - COALESCE(PauseDuration, 0) AS PlayDuration"
	}

	detailSQL := fmt.Sprintf(`
SELECT UserId, UserName, ItemName, ItemType, DateCreated, DeviceName, ClientName,
       %s
FROM PlaybackActivity
WHERE %s
ORDER BY DateCreated DESC
LIMIT %d OFFSET %d
`, durationExpr, whereClause, pageSize, offset)

	return s.embyService.QueryPlaybackStats(detailSQL)
}

func (s *PlaybackHistoryService) queryPlaybackDetailsWildcard(whereClause string, pageSize int, offset int) (*CustomQueryResponse, error) {
	sqlWithOrder := fmt.Sprintf(`
SELECT *
FROM PlaybackActivity
WHERE %s
ORDER BY DateCreated DESC
LIMIT %d OFFSET %d
`, whereClause, pageSize, offset)

	resp, err := s.embyService.QueryPlaybackStats(sqlWithOrder)
	if err == nil {
		return resp, nil
	}

	sqlNoOrder := fmt.Sprintf(`
SELECT *
FROM PlaybackActivity
WHERE %s
LIMIT %d OFFSET %d
`, whereClause, pageSize, offset)
	return s.embyService.QueryPlaybackStats(sqlNoOrder)
}

func (s *PlaybackHistoryService) queryPlaybackDetailsAll(whereClause string) (*CustomQueryResponse, error) {
	sqlWithOrder := fmt.Sprintf(`
SELECT *
FROM PlaybackActivity
WHERE %s
ORDER BY DateCreated DESC
`, whereClause)

	resp, err := s.embyService.QueryPlaybackStats(sqlWithOrder)
	if err == nil {
		return resp, nil
	}

	sqlNoOrder := fmt.Sprintf(`
SELECT *
FROM PlaybackActivity
WHERE %s
`, whereClause)
	return s.embyService.QueryPlaybackStats(sqlNoOrder)
}

func (s *PlaybackHistoryService) loadPlaybackRowsWithFallback(whereClause string, pageSize int, offset int) ([]playbackActivityRow, error) {
	detailResp, err := s.queryPlaybackDetails(whereClause, pageSize, offset, true)
	if err != nil {
		if shouldFallbackPlaybackDetailError(err) {
			detailResp, err = s.queryPlaybackDetails(whereClause, pageSize, offset, false)
		}
		if err != nil {
			detailResp, err = s.queryPlaybackDetailsWildcard(whereClause, pageSize, offset)
		}
		if err != nil {
			return s.loadPlaybackRowsByLocalPagination(whereClause, pageSize, offset)
		}
	}

	if shouldFallbackPlaybackDetailQuery(detailResp) {
		detailResp, err = s.queryPlaybackDetails(whereClause, pageSize, offset, false)
		if err != nil || shouldFallbackPlaybackDetailQuery(detailResp) {
			detailResp, err = s.queryPlaybackDetailsWildcard(whereClause, pageSize, offset)
		}
		if err != nil || shouldFallbackPlaybackDetailQuery(detailResp) {
			return s.loadPlaybackRowsByLocalPagination(whereClause, pageSize, offset)
		}
	}

	rows, err := parsePlaybackRows(detailResp)
	if err != nil {
		return nil, fmt.Errorf("解析播放历史明细失败: %w", err)
	}
	if len(rows) > 0 {
		return rows, nil
	}

	return s.loadPlaybackRowsByLocalPagination(whereClause, pageSize, offset)
}

func (s *PlaybackHistoryService) loadPlaybackRowsByLocalPagination(whereClause string, pageSize int, offset int) ([]playbackActivityRow, error) {
	allResp, err := s.queryPlaybackDetailsAll(whereClause)
	if err != nil {
		return nil, err
	}
	allRows, err := parsePlaybackRows(allResp)
	if err != nil {
		return nil, fmt.Errorf("解析全量播放历史失败: %w", err)
	}
	if len(allRows) == 0 {
		return []playbackActivityRow{}, nil
	}

	sort.Slice(allRows, func(i, j int) bool {
		ti := parsePlaybackTime(allRows[i].playedAtRaw)
		tj := parsePlaybackTime(allRows[j].playedAtRaw)
		if !ti.Equal(tj) {
			return ti.After(tj)
		}
		return allRows[i].itemName < allRows[j].itemName
	})

	if offset >= len(allRows) {
		return []playbackActivityRow{}, nil
	}
	end := offset + pageSize
	if end > len(allRows) {
		end = len(allRows)
	}
	return allRows[offset:end], nil
}

func shouldFallbackPlaybackDetailQuery(resp *CustomQueryResponse) bool {
	if resp == nil {
		return true
	}
	if len(resp.Results) > 0 {
		return false
	}
	msg := strings.ToLower(strings.TrimSpace(resp.Message))
	if msg == "" {
		return false
	}
	if strings.Contains(msg, "pause") || strings.Contains(msg, "column") || strings.Contains(msg, "sql") {
		return true
	}
	return false
}

func shouldFallbackPlaybackDetailError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	if strings.Contains(msg, "pause") || strings.Contains(msg, "column") || strings.Contains(msg, "sql") {
		return true
	}
	return false
}

func (s *PlaybackHistoryService) normalizeRequest(req PlaybackHistoryRequest) (*playbackHistoryQuery, string, error) {
	query := &playbackHistoryQuery{
		PlaybackHistoryRequest: PlaybackHistoryRequest{
			UserID:    strings.TrimSpace(req.UserID),
			Keyword:   strings.TrimSpace(req.Keyword),
			StartDate: strings.TrimSpace(req.StartDate),
			EndDate:   strings.TrimSpace(req.EndDate),
			Page:      req.Page,
			PageSize:  req.PageSize,
		},
	}

	if query.Page <= 0 {
		query.Page = defaultPlaybackHistoryPage
	}
	if query.PageSize <= 0 {
		query.PageSize = defaultPlaybackHistoryPageSize
	}
	if query.PageSize > maxPlaybackHistoryPageSize {
		query.PageSize = maxPlaybackHistoryPageSize
	}

	if query.Keyword != "" {
		if len([]rune(query.Keyword)) > 100 || !playbackKeywordPattern.MatchString(query.Keyword) {
			return nil, "", ErrPlaybackHistoryInvalidKeyword
		}
	}

	if query.StartDate != "" {
		start, err := time.Parse(playbackDateFormat, query.StartDate)
		if err != nil {
			return nil, "", ErrPlaybackHistoryInvalidDate
		}
		startUTC := start.UTC()
		query.startDate = &startUTC
	}

	if query.EndDate != "" {
		end, err := time.Parse(playbackDateFormat, query.EndDate)
		if err != nil {
			return nil, "", ErrPlaybackHistoryInvalidDate
		}
		endUTC := end.UTC()
		query.endDate = &endUTC
	}

	if query.startDate != nil && query.endDate != nil && query.endDate.Before(*query.startDate) {
		return nil, "", ErrPlaybackHistoryInvalidDate
	}

	if query.UserID == "" {
		return query, "", nil
	}
	if !playbackUserIDPattern.MatchString(query.UserID) {
		return nil, "", ErrPlaybackHistoryInvalidUserID
	}

	var user models.User
	err := db.DB.Select("id", "username", "\"embyId\"").Where("id = ?", query.UserID).First(&user).Error
	if err == nil {
		embyID := strings.TrimSpace(user.EmbyID)
		if embyID != "" {
			return query, embyID, nil
		}
		// 本地用户未绑定 embyId 时，降级按原始 userId 直查 PlaybackActivity.UserId。
		return query, query.UserID, nil
	}
	if err != nil && err != gorm.ErrRecordNotFound {
		return nil, "", fmt.Errorf("查询用户失败: %w", err)
	}

	// 兼容无本地映射用户：按 Emby UserId 直接过滤。
	return query, query.UserID, nil
}

func buildPlaybackWhereClause(query *playbackHistoryQuery, embyUserID string) string {
	conditions := []string{"1=1"}

	if embyUserID != "" {
		conditions = append(conditions, fmt.Sprintf("UserId = '%s'", escapeSQLLiteral(embyUserID)))
	}

	if query.Keyword != "" {
		keyword := escapeLikePattern(query.Keyword)
		conditions = append(conditions, fmt.Sprintf(
			"(ItemName LIKE '%%%s%%' ESCAPE '\\' OR UserName LIKE '%%%s%%' ESCAPE '\\' OR DeviceName LIKE '%%%s%%' ESCAPE '\\' OR ClientName LIKE '%%%s%%' ESCAPE '\\')",
			keyword, keyword, keyword, keyword,
		))
	}

	if query.startDate != nil {
		start := query.startDate.Format("2006-01-02 00:00:00")
		conditions = append(conditions, fmt.Sprintf("DateCreated >= '%s'", start))
	}

	if query.endDate != nil {
		end := query.endDate.Format("2006-01-02 23:59:59")
		conditions = append(conditions, fmt.Sprintf("DateCreated <= '%s'", end))
	}

	return strings.Join(conditions, " AND ")
}

func parsePlaybackCount(resp *CustomQueryResponse) (int64, error) {
	if resp == nil || len(resp.Results) == 0 || len(resp.Results[0]) == 0 {
		return 0, nil
	}
	return asInt64(resp.Results[0][0])
}

func parsePlaybackRows(resp *CustomQueryResponse) ([]playbackActivityRow, error) {
	if resp == nil || len(resp.Results) == 0 {
		return []playbackActivityRow{}, nil
	}

	indexes := map[string]int{}
	columns := resp.Colums
	if len(columns) == 0 {
		columns = resp.Columns
	}
	for idx, col := range columns {
		indexes[strings.ToLower(strings.TrimSpace(col))] = idx
	}
	fallbackIndexes := map[string]int{
		"userid":       0,
		"username":     1,
		"itemname":     2,
		"itemtype":     3,
		"datecreated":  4,
		"devicename":   5,
		"clientname":   6,
		"playduration": 7,
	}

	get := func(row []interface{}, keys ...string) interface{} {
		for _, key := range keys {
			key = strings.ToLower(strings.TrimSpace(key))
			idx, ok := indexes[key]
			if !ok {
				idx, ok = fallbackIndexes[key]
			}
			if ok && idx >= 0 && idx < len(row) {
				return row[idx]
			}
		}
		return nil
	}

	result := make([]playbackActivityRow, 0, len(resp.Results))
	for _, row := range resp.Results {
		playDuration, err := asInt64(get(row, "playduration", "duration"))
		if err != nil {
			return nil, err
		}
		pauseDuration, err := asInt64(get(row, "pauseduration"))
		if err != nil {
			return nil, err
		}
		duration := playDuration - pauseDuration
		if duration < 0 {
			duration = 0
		}

		result = append(result, playbackActivityRow{
			embyUserID:   safeString(get(row, "userid", "user_id")),
			username:     safeString(get(row, "username", "user_name")),
			itemName:     safeString(get(row, "itemname", "name")),
			itemType:     safeString(get(row, "itemtype", "type")),
			playedAtRaw:  safeString(get(row, "datecreated", "playedat", "date")),
			deviceName:   safeString(get(row, "devicename", "device")),
			clientName:   safeString(get(row, "clientname", "client")),
			playDuration: duration,
		})
	}

	return result, nil
}

func (s *PlaybackHistoryService) loadUsersByEmbyIDs(ctx context.Context, rows []playbackActivityRow) (map[string]models.User, error) {
	embyIDSet := make(map[string]struct{}, len(rows))
	for _, row := range rows {
		if row.embyUserID == "" {
			continue
		}
		embyIDSet[row.embyUserID] = struct{}{}
	}

	if len(embyIDSet) == 0 {
		return map[string]models.User{}, nil
	}

	embyIDs := make([]string, 0, len(embyIDSet))
	for id := range embyIDSet {
		embyIDs = append(embyIDs, id)
	}

	var users []models.User
	if err := db.DB.WithContext(ctx).Select("id", "username", "\"embyId\"").Where("\"embyId\" IN ?", embyIDs).Find(&users).Error; err != nil {
		return nil, fmt.Errorf("查询本地用户映射失败: %w", err)
	}

	userMap := make(map[string]models.User, len(users))
	for _, user := range users {
		userMap[user.EmbyID] = user
	}

	return userMap, nil
}

func formatPlayDuration(seconds int64) string {
	if seconds <= 0 {
		return "0m"
	}

	minutes := seconds / 60
	if seconds%60 != 0 {
		minutes++
	}

	if minutes < 60 {
		return fmt.Sprintf("%dm", minutes)
	}

	hours := minutes / 60
	restMinutes := minutes % 60
	return fmt.Sprintf("%dh %dm", hours, restMinutes)
}

func parsePlaybackTime(raw string) time.Time {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Time{}
	}

	layoutsWithTimezone := []string{
		time.RFC3339Nano,
		time.RFC3339,
	}

	layoutsWithoutTimezone := []string{
		"2006-01-02 15:04:05",
		"2006-01-02T15:04:05",
		"2006-01-02 15:04:05.0000000",
	}

	for _, layout := range layoutsWithTimezone {
		if t, err := time.Parse(layout, raw); err == nil {
			return t.UTC()
		}
	}

	for _, layout := range layoutsWithoutTimezone {
		if t, err := time.ParseInLocation(layout, raw, time.Local); err == nil {
			return t.UTC()
		}
	}

	return time.Time{}
}

func safeString(v interface{}) string {
	switch t := v.(type) {
	case nil:
		return ""
	case string:
		return strings.TrimSpace(t)
	case []byte:
		return strings.TrimSpace(string(t))
	default:
		return strings.TrimSpace(fmt.Sprintf("%v", t))
	}
}

func escapeSQLLiteral(input string) string {
	return strings.ReplaceAll(input, "'", "''")
}

func escapeLikePattern(input string) string {
	escaped := strings.ReplaceAll(input, `\`, `\\`)
	escaped = strings.ReplaceAll(escaped, `%`, `\%`)
	escaped = strings.ReplaceAll(escaped, `_`, `\_`)
	return escapeSQLLiteral(escaped)
}
