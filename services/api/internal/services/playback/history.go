package playback

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	configpkg "github.com/konghang/ember/backend/internal/config"
	"github.com/konghang/ember/backend/internal/db"
	embyint "github.com/konghang/ember/backend/internal/integrations/emby"
	"github.com/konghang/ember/backend/internal/models"
	"gorm.io/gorm"
)

const (
	defaultPlaybackHistoryPage     = 1
	defaultPlaybackHistoryPageSize = 20
	maxPlaybackHistoryPageSize     = 100
	playbackDateFormat             = "2006-01-02"
)

var playbackKeywordPattern = regexp.MustCompile(`^[\p{Han}\p{L}\p{N}._'&+\-!():,/ ]+$`)
var playbackUserIDPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,50}$`)

type PlaybackHistoryService struct {
	embyService        *embyint.EmbyService
	queryPlaybackStats func(sql string) (*embyint.CustomQueryResponse, error)
}

type PlaybackHistoryRequest struct {
	UserID    string `form:"userId" json:"userId"`
	Username  string `form:"username" json:"username"`
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
	startDate    *time.Time
	endDate      *time.Time
	hasStartTime bool
	hasEndTime   bool
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
	embyService := embyint.GetSharedService()
	return &PlaybackHistoryService{
		embyService:        embyService,
		queryPlaybackStats: embyService.QueryPlaybackStats,
	}
}

func (s *PlaybackHistoryService) GetPlaybackHistory(ctx context.Context, req PlaybackHistoryRequest) (*PlaybackHistoryResponse, error) {
	query, playbackUserIDs, err := s.normalizeRequest(ctx, req)
	if err != nil {
		return nil, err
	}

	if (query.UserID != "" || query.Username != "") && len(playbackUserIDs) == 0 {
		return &PlaybackHistoryResponse{
			Data:     []PlaybackHistoryItem{},
			Total:    0,
			Page:     query.Page,
			PageSize: query.PageSize,
		}, nil
	}

	whereClause := buildPlaybackWhereClause(query, playbackUserIDs)
	countSQL := fmt.Sprintf("SELECT COUNT(1) AS total FROM PlaybackActivity WHERE %s", whereClause)

	countResp, err := s.queryPlaybackStats(countSQL)
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
		if errors.Is(err, ErrPlaybackHistorySchemaUnsupported) {
			return nil, err
		}
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

func (s *PlaybackHistoryService) queryPlaybackDetails(whereClause string, pageSize int, offset int, includePauseDuration bool) (*embyint.CustomQueryResponse, error) {
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

	return s.queryPlaybackStats(detailSQL)
}

func (s *PlaybackHistoryService) queryPlaybackDetailsWildcard(whereClause string, pageSize int, offset int) (*embyint.CustomQueryResponse, error) {
	sqlWithOrder := fmt.Sprintf(`
SELECT *
FROM PlaybackActivity
WHERE %s
ORDER BY DateCreated DESC
LIMIT %d OFFSET %d
`, whereClause, pageSize, offset)

	resp, err := s.queryPlaybackStats(sqlWithOrder)
	if err == nil {
		return resp, nil
	}

	sqlNoOrder := fmt.Sprintf(`
SELECT *
FROM PlaybackActivity
WHERE %s
LIMIT %d OFFSET %d
`, whereClause, pageSize, offset)
	return s.queryPlaybackStats(sqlNoOrder)
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
			return nil, ErrPlaybackHistorySchemaUnsupported
		}
	}

	if shouldFallbackPlaybackDetailQuery(detailResp) {
		detailResp, err = s.queryPlaybackDetails(whereClause, pageSize, offset, false)
		if err != nil || shouldFallbackPlaybackDetailQuery(detailResp) {
			detailResp, err = s.queryPlaybackDetailsWildcard(whereClause, pageSize, offset)
		}
		if err != nil || shouldFallbackPlaybackDetailQuery(detailResp) {
			return nil, ErrPlaybackHistorySchemaUnsupported
		}
	}

	rows, err := parsePlaybackRows(detailResp)
	if err != nil {
		return nil, fmt.Errorf("解析播放历史明细失败: %w", err)
	}
	return rows, nil
}

func shouldFallbackPlaybackDetailQuery(resp *embyint.CustomQueryResponse) bool {
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

func (s *PlaybackHistoryService) normalizeRequest(ctx context.Context, req PlaybackHistoryRequest) (*playbackHistoryQuery, []string, error) {
	query := &playbackHistoryQuery{
		PlaybackHistoryRequest: PlaybackHistoryRequest{
			UserID:    strings.TrimSpace(req.UserID),
			Username:  strings.TrimSpace(req.Username),
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
			return nil, nil, ErrPlaybackHistoryInvalidKeyword
		}
	}

	if query.Username != "" {
		if len([]rune(query.Username)) > 100 || !playbackKeywordPattern.MatchString(query.Username) {
			return nil, nil, ErrPlaybackHistoryInvalidKeyword
		}
	}

	if query.StartDate != "" {
		start, hasTime, err := parsePlaybackProfileTime(query.StartDate, loadPlaybackTimezone())
		if err != nil {
			return nil, nil, ErrPlaybackHistoryInvalidDate
		}
		query.startDate = &start
		query.hasStartTime = hasTime
	}

	if query.EndDate != "" {
		end, hasTime, err := parsePlaybackProfileTime(query.EndDate, loadPlaybackTimezone())
		if err != nil {
			return nil, nil, ErrPlaybackHistoryInvalidDate
		}
		query.endDate = &end
		query.hasEndTime = hasTime
	}

	if query.startDate != nil && query.endDate != nil && query.endDate.Before(*query.startDate) {
		return nil, nil, ErrPlaybackHistoryInvalidDate
	}

	if query.UserID != "" {
		if !playbackUserIDPattern.MatchString(query.UserID) {
			return nil, nil, ErrPlaybackHistoryInvalidUserID
		}

		playbackUserID, err := s.resolvePlaybackUserIDByUserID(ctx, query.UserID)
		if err != nil {
			return nil, nil, err
		}
		return query, []string{playbackUserID}, nil
	}

	if query.Username != "" {
		playbackUserIDs, err := s.resolvePlaybackUserIDsByUsername(ctx, query.Username)
		if err != nil {
			return nil, nil, err
		}
		return query, playbackUserIDs, nil
	}

	return query, nil, nil
}

func (s *PlaybackHistoryService) resolvePlaybackUserIDByUserID(ctx context.Context, userID string) (string, error) {
	var user models.User
	err := db.DB.WithContext(ctx).Select("id", "username", "\"emby_id\"").Where("id = ?", userID).First(&user).Error
	if err == nil {
		embyID := strings.TrimSpace(user.EmbyID)
		if embyID != "" {
			return embyID, nil
		}
		// 本地用户未绑定 embyId 时，降级按原始 userId 直查 PlaybackActivity.UserId。
		return userID, nil
	}
	if err != nil && err != gorm.ErrRecordNotFound {
		return "", fmt.Errorf("查询用户失败: %w", err)
	}

	// 兼容无本地映射用户：按 Emby UserId 直接过滤。
	return userID, nil
}

func (s *PlaybackHistoryService) resolvePlaybackUserIDsByUsername(ctx context.Context, username string) ([]string, error) {
	var users []models.User
	if err := db.DB.WithContext(ctx).
		Select("id", "username", "\"emby_id\"").
		Where("username ILIKE ?", "%"+username+"%").
		Order("username ASC").
		Find(&users).Error; err != nil {
		return nil, fmt.Errorf("查询用户失败: %w", err)
	}

	if len(users) == 0 {
		return []string{}, nil
	}

	uniqueIDs := make(map[string]struct{}, len(users))
	playbackUserIDs := make([]string, 0, len(users))
	for _, user := range users {
		playbackUserID := strings.TrimSpace(user.EmbyID)
		if playbackUserID == "" {
			playbackUserID = strings.TrimSpace(user.ID)
		}
		if playbackUserID == "" {
			continue
		}
		if _, exists := uniqueIDs[playbackUserID]; exists {
			continue
		}
		uniqueIDs[playbackUserID] = struct{}{}
		playbackUserIDs = append(playbackUserIDs, playbackUserID)
	}

	return playbackUserIDs, nil
}

func buildPlaybackWhereClause(query *playbackHistoryQuery, playbackUserIDs []string) string {
	conditions := []string{"1=1"}

	if len(playbackUserIDs) > 0 {
		quotedUserIDs := make([]string, 0, len(playbackUserIDs))
		for _, playbackUserID := range playbackUserIDs {
			quotedUserIDs = append(quotedUserIDs, fmt.Sprintf("'%s'", escapeSQLLiteral(playbackUserID)))
		}
		conditions = append(conditions, fmt.Sprintf("UserId IN (%s)", strings.Join(quotedUserIDs, ", ")))
	}

	if query.Keyword != "" {
		keyword := escapeLikePattern(query.Keyword)
		conditions = append(conditions, fmt.Sprintf(
			"(ItemName LIKE '%%%s%%' ESCAPE '\\' OR UserName LIKE '%%%s%%' ESCAPE '\\' OR DeviceName LIKE '%%%s%%' ESCAPE '\\' OR ClientName LIKE '%%%s%%' ESCAPE '\\')",
			keyword, keyword, keyword, keyword,
		))
	}

	if query.startDate != nil {
		startAt := *query.startDate
		if !query.hasStartTime {
			startAt = time.Date(
				query.startDate.Year(),
				query.startDate.Month(),
				query.startDate.Day(),
				0, 0, 0, 0,
				query.startDate.Location(),
			)
		}
		start := formatPlaybackDatabaseTime(startAt)
		conditions = append(conditions, fmt.Sprintf("DateCreated >= '%s'", start))
	}

	if query.endDate != nil {
		endAt := *query.endDate
		if !query.hasEndTime {
			endAt = time.Date(
				query.endDate.Year(),
				query.endDate.Month(),
				query.endDate.Day(),
				0, 0, 0, 0,
				query.endDate.Location(),
			).AddDate(0, 0, 1)
			end := formatPlaybackDatabaseTime(endAt)
			conditions = append(conditions, fmt.Sprintf("DateCreated < '%s'", end))
		} else {
			end := formatPlaybackDatabaseTime(endAt)
			conditions = append(conditions, fmt.Sprintf("DateCreated <= '%s'", end))
		}
	}

	return strings.Join(conditions, " AND ")
}

func parsePlaybackCount(resp *embyint.CustomQueryResponse) (int64, error) {
	if resp == nil || len(resp.Results) == 0 || len(resp.Results[0]) == 0 {
		return 0, nil
	}
	return asInt64(resp.Results[0][0])
}

func parsePlaybackRows(resp *embyint.CustomQueryResponse) ([]playbackActivityRow, error) {
	if resp == nil || len(resp.Results) == 0 {
		return []playbackActivityRow{}, nil
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
	// 只校验聚合与展示真正依赖的列：键(userid) + 时间维(datecreated) + 度量(playduration) + 列表展示最小字段(itemname)。
	// username / devicename / clientname / itemtype 在老版 Playback Reporting 插件里可能缺失，缺失时 safeString(nil) 回退为空串。
	requiredColumns := []string{"userid", "itemname", "datecreated", "playduration"}
	for _, column := range requiredColumns {
		if _, ok := indexes[column]; !ok {
			return nil, fmt.Errorf("%w: missing column %s", ErrPlaybackHistorySchemaUnsupported, column)
		}
	}

	get := func(row []interface{}, keys ...string) interface{} {
		for _, key := range keys {
			key = strings.ToLower(strings.TrimSpace(key))
			idx, ok := indexes[key]
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
	if err := db.DB.WithContext(ctx).Select("id", "username", "\"emby_id\"").Where("\"emby_id\" IN ?", embyIDs).Find(&users).Error; err != nil {
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

	playbackTZ := loadPlaybackTimezone()

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
			return t.In(playbackTZ)
		}
	}

	for _, layout := range layoutsWithoutTimezone {
		if t, err := time.ParseInLocation(layout, raw, playbackTZ); err == nil {
			return t
		}
	}

	return time.Time{}
}

func loadPlaybackTimezone() *time.Location {
	return configpkg.LoadConfiguredTimezone()
}

func formatPlaybackDatabaseTime(value time.Time) string {
	return value.In(loadPlaybackTimezone()).Format("2006-01-02 15:04:05")
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
