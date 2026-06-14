package moviepilot

import (
	"bytes"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/konghang/ember/backend/internal/common/upstream"
	configpkg "github.com/konghang/ember/backend/internal/config"
)

// MoviePilotClient MoviePilot API 客户端
type MoviePilotClient struct {
	baseURL string
	apiKey  string
	client  *http.Client
}

// NewMoviePilotClient 创建 MoviePilot 客户端
func NewMoviePilotClient() *MoviePilotClient {
	client := &MoviePilotClient{
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
	client.refreshConfig()
	return client
}

func (c *MoviePilotClient) refreshConfig() {
	configService := configpkg.NewConfigService()
	c.baseURL = strings.TrimRight(configService.GetString("MOVIEPILOT_URL"), "/")
	c.apiKey = strings.TrimSpace(configService.GetString("MOVIEPILOT_API_KEY"))
}

func (c *MoviePilotClient) ensureConfigured() error {
	configService := configpkg.NewConfigService()
	c.refreshConfig()
	if c.baseURL != "" && c.apiKey == "" && configService.MoviePilotNeedsAPIKeyMigration() {
		return fmt.Errorf("检测到旧版 MoviePilot 用户名/密码配置，请迁移到 MOVIEPILOT_API_KEY")
	}
	if c.baseURL == "" || c.apiKey == "" {
		return fmt.Errorf("MoviePilot 未配置")
	}
	return nil
}

func (c *MoviePilotClient) newRequest(method, path string, query url.Values, body io.Reader) (*http.Request, error) {
	endpoint := c.baseURL + path
	if len(query) > 0 {
		endpoint += "?" + query.Encode()
	}

	req, err := http.NewRequest(method, endpoint, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("X-API-KEY", c.apiKey)
	return req, nil
}

func (c *MoviePilotClient) doRequest(req *http.Request) ([]byte, error) {
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, upstream.SafeUpstreamError(err, "moviepilot")
	}
	defer resp.Body.Close()

	body, readErr := io.ReadAll(resp.Body)
	if readErr != nil {
		return nil, upstream.SafeUpstreamError(readErr, "moviepilot")
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, upstream.SafeUpstreamHTTPError("moviepilot", resp.StatusCode)
	}

	return body, nil
}

// IsConfigured 检查配置是否完整
func (c *MoviePilotClient) IsConfigured() bool {
	return c.ensureConfigured() == nil
}

func (c *MoviePilotClient) TestConnection() error {
	if err := c.ensureConfigured(); err != nil {
		return err
	}

	req, err := c.newRequest(http.MethodGet, "/api/v1/site/", nil, nil)
	if err != nil {
		return upstream.SafeUpstreamError(err, "moviepilot")
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return upstream.SafeUpstreamError(err, "moviepilot")
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return upstream.SafeUpstreamHTTPError("moviepilot", resp.StatusCode)
	}

	return nil
}

// SubscribeRequest 订阅请求
type SubscribeRequest struct {
	Type   string `json:"type"`   // "movie" | "tv"
	Name   string `json:"name"`   // 影视名称
	TmdbID string `json:"tmdbid"` // TMDB ID（字符串）
	Season int    `json:"season"` // 季号，0 表示整剧
}

func buildSubscribeRequestBody(data SubscribeRequest) (map[string]interface{}, error) {
	tmdbIDInt, err := strconv.Atoi(data.TmdbID)
	if err != nil {
		return nil, fmt.Errorf("无效的 TMDB ID: %s", data.TmdbID)
	}

	typeZh := "电影"
	if data.Type == "tv" {
		typeZh = "电视剧"
	}

	requestBody := map[string]interface{}{
		"type":   typeZh,
		"name":   data.Name,
		"tmdbid": tmdbIDInt,
	}
	if data.Season > 0 {
		requestBody["season"] = data.Season
	}

	return requestBody, nil
}

// toMoviePilotMediaType converts Ember's internal media type into the Chinese
// enum value accepted by MoviePilot's /api/v1/search/media endpoint.
func toMoviePilotMediaType(mediaType string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(mediaType)) {
	case "movie":
		return "电影", nil
	case "tv":
		return "电视剧", nil
	default:
		return "", fmt.Errorf("mediaType 必须为 movie 或 tv")
	}
}

// SearchMediaRequest 按 TMDB 精确搜索候选资源。
type SearchMediaRequest struct {
	TmdbID    string `json:"tmdbId"`
	MediaType string `json:"mediaType"` // "movie" | "tv"
	Season    int    `json:"season,omitempty"`
}

// SearchTitleRequest 按关键词搜索候选资源。
type SearchTitleRequest struct {
	Keyword string `json:"keyword"`
}

// SearchCandidate MoviePilot 候选资源摘要。
type SearchCandidate struct {
	ID          string                 `json:"id"`
	Title       string                 `json:"title"`
	Description string                 `json:"description,omitempty"`
	PublishDate string                 `json:"publishDate,omitempty"`
	Site        string                 `json:"site,omitempty"`
	Size        int64                  `json:"size,omitempty"`
	Seeders     int                    `json:"seeders,omitempty"`
	IsPack      bool                   `json:"isPack"`
	MatchMode   string                 `json:"matchMode"`
	Tags        []string               `json:"tags,omitempty"`
	Payload     map[string]interface{} `json:"payload,omitempty"`
}

// SearchMediaResponse MoviePilot 搜索响应。
type SearchMediaResponse struct {
	Query      string            `json:"query"`
	MatchMode  string            `json:"matchMode"`
	Candidates []SearchCandidate `json:"candidates"`
}

// DownloadCandidateRequest 下发下载候选。
type DownloadCandidateRequest struct {
	CandidatePayload map[string]interface{} `json:"candidatePayload"`
	TmdbID           string                 `json:"tmdbId,omitempty"`
	Season           int                    `json:"season,omitempty"`
}

// DownloadCandidateResponse 下载下发响应。
type DownloadCandidateResponse struct {
	Accepted   bool                   `json:"accepted"`
	StatusCode int                    `json:"statusCode"`
	Message    string                 `json:"message,omitempty"`
	Response   map[string]interface{} `json:"response,omitempty"`
}

// GapSearchRequest 缺集搜索请求
type GapSearchRequest struct {
	SeriesName string `json:"seriesName"`
	Season     int    `json:"season"`
	Episode    int    `json:"episode"`
}

// GapSearchCandidate 缺集候选摘要
type GapSearchCandidate = SearchCandidate

// GapSearchResponse 缺集搜索响应
type GapSearchResponse struct {
	Query         string               `json:"query"`
	FallbackQuery string               `json:"fallbackQuery,omitempty"`
	MatchMode     string               `json:"matchMode"`
	Candidates    []GapSearchCandidate `json:"candidates"`
}

// GapDispatchRequest 缺集下发请求
type GapDispatchRequest struct {
	CandidatePayload map[string]interface{} `json:"candidatePayload"`
	TmdbID           string                 `json:"tmdbId,omitempty"`
}

// GapDispatchResponse 缺集下发响应
type GapDispatchResponse = DownloadCandidateResponse

// SearchMediaCandidates 使用 MoviePilot TMDB 精确搜索入口搜索候选。
func (c *MoviePilotClient) SearchMediaCandidates(data SearchMediaRequest) (*SearchMediaResponse, error) {
	if err := c.ensureConfigured(); err != nil {
		return nil, err
	}

	tmdbID := strings.TrimSpace(data.TmdbID)
	if tmdbID == "" {
		return nil, fmt.Errorf("tmdbId 不能为空")
	}
	if _, err := strconv.Atoi(tmdbID); err != nil {
		return nil, fmt.Errorf("无效的 TMDB ID: %s", tmdbID)
	}

	mediaType := strings.ToLower(strings.TrimSpace(data.MediaType))
	moviePilotMediaType, err := toMoviePilotMediaType(mediaType)
	if err != nil {
		return nil, err
	}
	if mediaType == "tv" && data.Season <= 0 {
		return nil, fmt.Errorf("season 必须大于 0")
	}

	query := url.Values{"mtype": []string{moviePilotMediaType}}
	if mediaType == "tv" {
		query.Set("season", strconv.Itoa(data.Season))
	}
	path := "/api/v1/search/media/tmdb:" + url.PathEscape(tmdbID)
	req, err := c.newRequest(http.MethodGet, path, query, nil)
	if err != nil {
		return nil, upstream.SafeUpstreamError(err, "moviepilot")
	}
	req.Header.Set("Accept", "application/json")

	body, err := c.doRequest(req)
	if err != nil {
		return nil, err
	}

	results, err := parseSearchResults(body)
	if err != nil {
		return nil, err
	}

	matchMode := mediaType
	if mediaType == "tv" {
		matchMode = fmt.Sprintf("season-%d", data.Season)
	}
	return &SearchMediaResponse{
		Query:      fmt.Sprintf("tmdb:%s", tmdbID),
		MatchMode:  matchMode,
		Candidates: normalizeCandidates(results, matchMode),
	}, nil
}

// SearchGapCandidates 搜索缺集候选，优先搜单集，再回退整季包。
func (c *MoviePilotClient) SearchGapCandidates(data GapSearchRequest) (*GapSearchResponse, error) {
	if err := c.ensureConfigured(); err != nil {
		return nil, err
	}

	seriesName := strings.TrimSpace(data.SeriesName)
	if seriesName == "" {
		return nil, fmt.Errorf("seriesName 不能为空")
	}
	if data.Season <= 0 {
		return nil, fmt.Errorf("season 必须大于 0")
	}
	if data.Episode <= 0 {
		return nil, fmt.Errorf("episode 必须大于 0")
	}

	episodeQuery := fmt.Sprintf("%s S%02dE%02d", seriesName, data.Season, data.Episode)
	results, err := c.SearchTitleCandidates(SearchTitleRequest{Keyword: episodeQuery})
	if err != nil {
		return nil, err
	}

	response := &GapSearchResponse{
		Query:      episodeQuery,
		MatchMode:  "episode",
		Candidates: normalizeCandidates(results, "episode"),
	}
	if len(response.Candidates) > 0 {
		return response, nil
	}

	seasonQuery := fmt.Sprintf("%s S%02d", seriesName, data.Season)
	results, err = c.SearchTitleCandidates(SearchTitleRequest{Keyword: seasonQuery})
	if err != nil {
		return nil, err
	}

	response.FallbackQuery = seasonQuery
	response.MatchMode = "season"
	response.Candidates = normalizeCandidates(results, "season")
	return response, nil
}

// SearchTitleCandidates 使用 MoviePilot 标题搜索入口搜索候选。
func (c *MoviePilotClient) SearchTitleCandidates(data SearchTitleRequest) ([]map[string]interface{}, error) {
	if err := c.ensureConfigured(); err != nil {
		return nil, err
	}
	keyword := strings.TrimSpace(data.Keyword)
	if keyword == "" {
		return nil, fmt.Errorf("keyword 不能为空")
	}
	req, err := c.newRequest(http.MethodGet, "/api/v1/search/title", url.Values{
		"keyword": []string{keyword},
	}, nil)
	if err != nil {
		return nil, upstream.SafeUpstreamError(err, "moviepilot")
	}
	req.Header.Set("Accept", "application/json")

	body, err := c.doRequest(req)
	if err != nil {
		return nil, err
	}

	return parseSearchResults(body)
}

// DispatchDownloadCandidate 将候选资源下发给 MoviePilot 下载入口。
func (c *MoviePilotClient) DispatchDownloadCandidate(data DownloadCandidateRequest) (*DownloadCandidateResponse, error) {
	if err := c.ensureConfigured(); err != nil {
		return nil, err
	}
	if len(data.CandidatePayload) == 0 {
		return nil, fmt.Errorf("candidatePayload 不能为空")
	}

	payload := cloneMap(data.CandidatePayload)
	if size, exists := payload["size"]; exists {
		payload["size"] = normalizeNumericValue(size)
	}

	requestBody := map[string]interface{}{
		"torrent_in": payload,
	}
	tmdbID := strings.TrimSpace(data.TmdbID)
	if tmdbID != "" {
		tmdbIDInt, err := strconv.Atoi(tmdbID)
		if err != nil {
			return nil, fmt.Errorf("无效的 TMDB ID: %s", tmdbID)
		}
		requestBody["tmdbid"] = tmdbIDInt
	}
	if data.Season > 0 {
		requestBody["season"] = data.Season
	}
	jsonData, err := json.Marshal(requestBody)
	if err != nil {
		return nil, fmt.Errorf("构建请求失败: %w", err)
	}

	req, err := c.newRequest(http.MethodPost, "/api/v1/download/add", nil, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, upstream.SafeUpstreamError(err, "moviepilot")
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, upstream.SafeUpstreamError(err, "moviepilot")
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, upstream.SafeUpstreamError(err, "moviepilot")
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, upstream.SafeUpstreamHTTPError("moviepilot", resp.StatusCode)
	}

	parsed := parseJSONMap(body)
	result := &GapDispatchResponse{
		Accepted:   true,
		StatusCode: resp.StatusCode,
		Message:    extractResponseMessage(body),
	}
	if len(parsed) > 0 {
		result.Response = parsed
		if result.Message == "" {
			result.Message = extractMapString(parsed, "message", "msg", "detail")
		}
	}

	// MoviePilot v2 在业务失败时 HTTP 仍返回 200，仅靠顶层 success=false 区分
	// （如重复添加 / 校验失败 / 下载器异常）。必须显式判定 success，
	// 否则这些失败会被误判为已接受，污染 mediagap 与手动补偿两条下发链路。
	if present, isSuccess := lookupResponseSuccess(parsed); present && !isSuccess {
		result.Accepted = false
		// businessMsg 直接取自 MoviePilot 响应体的 message 文本字段，不含请求 URL / api_key。
		// MoviePilot download/add 在 success=false 时不会回显请求体，message 为业务原因描述。
		businessMsg := strings.TrimSpace(result.Message)
		if businessMsg == "" {
			businessMsg = "MoviePilot 拒绝下载请求"
		}
		// 关键路径排障日志：记录 HTTP 200 业务失败命中点，便于区分上游故障与业务拒绝。
		// 不打印原始响应体，避免回显可能包含的敏感片段。
		log.Printf("[MoviePilot] 下载下发被上游业务拒绝 path=/api/v1/download/add statusCode=200 success=false message=%q", businessMsg)
		return result, fmt.Errorf("%w: %s", ErrMoviePilotBusinessRejected, businessMsg)
	}

	return result, nil
}

// DispatchGapCandidate 将缺集候选下发给 MoviePilot。
func (c *MoviePilotClient) DispatchGapCandidate(data GapDispatchRequest) (*GapDispatchResponse, error) {
	return c.DispatchDownloadCandidate(DownloadCandidateRequest{
		CandidatePayload: data.CandidatePayload,
		TmdbID:           data.TmdbID,
	})
}

// CreateSubscription 创建订阅
//
// @param data.Type - 媒体类型（"movie" | "tv"）
// @param data.Name - 影视名称
// @param data.TmdbID - TMDB ID（字符串，会转为整数）
// @returns MoviePilot API 响应
func (c *MoviePilotClient) CreateSubscription(data SubscribeRequest) error {
	if err := c.ensureConfigured(); err != nil {
		return err
	}

	requestBody, err := buildSubscribeRequestBody(data)
	if err != nil {
		return err
	}

	jsonData, err := json.Marshal(requestBody)
	if err != nil {
		return fmt.Errorf("构建请求失败: %w", err)
	}

	req, err := c.newRequest(http.MethodPost, "/api/v1/subscribe/", nil, bytes.NewBuffer(jsonData))
	if err != nil {
		return upstream.SafeUpstreamError(err, "moviepilot")
	}
	req.Header.Set("Content-Type", "application/json")

	if _, err := c.doRequest(req); err != nil {
		return err
	}

	return nil
}

func parseSearchResults(body []byte) ([]map[string]interface{}, error) {
	var payload interface{}
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("解析搜索响应失败: %w", err)
	}

	switch typed := payload.(type) {
	case []interface{}:
		return normalizeResultList(typed), nil
	case map[string]interface{}:
		if list, ok := typed["data"].([]interface{}); ok {
			return normalizeResultList(list), nil
		}
		if list, ok := typed["results"].([]interface{}); ok {
			return normalizeResultList(list), nil
		}
	}

	return []map[string]interface{}{}, nil
}

func normalizeResultList(items []interface{}) []map[string]interface{} {
	results := make([]map[string]interface{}, 0, len(items))
	for _, item := range items {
		itemMap, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		results = append(results, itemMap)
	}
	return results
}

func normalizeCandidates(results []map[string]interface{}, matchMode string) []SearchCandidate {
	candidates := make([]SearchCandidate, 0, len(results))
	for _, item := range results {
		payload := extractCandidatePayload(item)
		title := strings.TrimSpace(extractString(item, "name", "title", "torrent_name"))
		if title == "" {
			title = "未命名资源"
		}

		description := strings.TrimSpace(extractString(item, "description", "desc", "detail", "subtitle"))
		publishDate := strings.TrimSpace(extractString(item, "publishDate", "pubDate", "pubdate", "publish_date", "publishedAt", "published_at", "date"))
		site := strings.TrimSpace(extractString(item, "site_name", "site", "indexer"))
		size := extractInt64(item, "size", "enclosure_size", "torrent_size")
		seeders := extractInt(item, "seeders", "seeder")

		candidates = append(candidates, SearchCandidate{
			ID:          buildCandidateID(payload, title, site),
			Title:       title,
			Description: description,
			PublishDate: publishDate,
			Site:        site,
			Size:        size,
			Seeders:     seeders,
			IsPack:      matchMode == "season",
			MatchMode:   matchMode,
			Tags:        extractCandidateTags(title, description),
			Payload:     payload,
		})
	}

	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].Seeders == candidates[j].Seeders {
			return candidates[i].Size > candidates[j].Size
		}
		return candidates[i].Seeders > candidates[j].Seeders
	})

	return candidates
}

func normalizeGapCandidates(results []map[string]interface{}, matchMode string) []GapSearchCandidate {
	return normalizeCandidates(results, matchMode)
}

func extractCandidatePayload(item map[string]interface{}) map[string]interface{} {
	if nested, ok := item["torrent_info"].(map[string]interface{}); ok && len(nested) > 0 {
		return cloneMap(nested)
	}
	return cloneMap(item)
}

func extractString(item map[string]interface{}, keys ...string) string {
	if value := extractValue(item, keys...); value != nil {
		switch typed := value.(type) {
		case string:
			return typed
		default:
			return fmt.Sprint(typed)
		}
	}
	return ""
}

func extractInt64(item map[string]interface{}, keys ...string) int64 {
	value := extractValue(item, keys...)
	switch typed := normalizeNumericValue(value).(type) {
	case int:
		return int64(typed)
	case int64:
		return typed
	case float64:
		return int64(typed)
	default:
		return 0
	}
}

func extractInt(item map[string]interface{}, keys ...string) int {
	value := extractValue(item, keys...)
	switch typed := normalizeNumericValue(value).(type) {
	case int:
		return typed
	case int64:
		return int(typed)
	case float64:
		return int(typed)
	default:
		return 0
	}
}

func extractValue(item map[string]interface{}, keys ...string) interface{} {
	for _, key := range keys {
		if value, ok := item[key]; ok && !isEmptyValue(value) {
			return value
		}
	}
	for _, nestedKey := range []string{"torrent", "torrent_info", "detail", "data", "info"} {
		nested, ok := item[nestedKey].(map[string]interface{})
		if !ok {
			continue
		}
		for _, key := range keys {
			if value, ok := nested[key]; ok && !isEmptyValue(value) {
				return value
			}
		}
	}
	return nil
}

func isEmptyValue(value interface{}) bool {
	if value == nil {
		return true
	}
	if text, ok := value.(string); ok {
		return strings.TrimSpace(text) == ""
	}
	return false
}

func extractCandidateTags(title, description string) []string {
	text := strings.ToUpper(strings.TrimSpace(title + " " + description))
	if text == "" {
		return nil
	}

	tags := make([]string, 0, 4)
	if strings.Contains(text, "2160P") || strings.Contains(text, "4K") {
		tags = append(tags, "4K")
	} else if strings.Contains(text, "1080P") {
		tags = append(tags, "1080P")
	}

	if strings.Contains(text, "DOVI") || strings.Contains(text, "DOLBY VISION") || strings.Contains(text, "VISION") {
		tags = append(tags, "DoVi")
	} else if strings.Contains(text, "HDR") {
		tags = append(tags, "HDR")
	}

	if strings.Contains(text, "WEB-DL") || strings.Contains(text, "WEBDL") || strings.Contains(text, "WEB") {
		tags = append(tags, "WEB-DL")
	}
	return tags
}

func buildCandidateID(payload map[string]interface{}, title, site string) string {
	serialized, err := json.Marshal(payload)
	if err != nil || len(serialized) == 0 {
		serialized = []byte(title + "|" + site)
	}
	sum := sha1.Sum(serialized)
	return hex.EncodeToString(sum[:])
}

func cloneMap(source map[string]interface{}) map[string]interface{} {
	if len(source) == 0 {
		return nil
	}

	cloned := make(map[string]interface{}, len(source))
	for key, value := range source {
		cloned[key] = value
	}
	return cloned
}

func normalizeNumericValue(value interface{}) interface{} {
	switch typed := value.(type) {
	case json.Number:
		if i, err := typed.Int64(); err == nil {
			return i
		}
		if f, err := typed.Float64(); err == nil {
			return f
		}
	case string:
		raw := strings.TrimSpace(typed)
		if raw == "" {
			return 0
		}
		if i, err := strconv.ParseInt(raw, 10, 64); err == nil {
			return i
		}
		if f, err := strconv.ParseFloat(raw, 64); err == nil {
			return f
		}
	case float32:
		return float64(typed)
	case float64:
		return typed
	case int:
		return typed
	case int8:
		return int64(typed)
	case int16:
		return int64(typed)
	case int32:
		return int64(typed)
	case int64:
		return typed
	case uint:
		return int64(typed)
	case uint8:
		return int64(typed)
	case uint16:
		return int64(typed)
	case uint32:
		return int64(typed)
	case uint64:
		return int64(typed)
	}
	return value
}

func parseJSONMap(body []byte) map[string]interface{} {
	if len(body) == 0 {
		return nil
	}

	var parsed map[string]interface{}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil
	}
	return parsed
}

// lookupResponseSuccess 解析 MoviePilot 响应顶层 success 字段。
//
// 返回 (present, value)：
//   - present=false：响应体不是 JSON 对象或不含 success 字段（保守处理缺省：维持原成功行为）
//   - present=true：success 存在，value 为其布尔语义
//
// MoviePilot v2 顶层 schema 为 {success, message, data}，success 一定是 bool；
// 兼容少数版本返回字符串 "true"/"false" 的情况。
func lookupResponseSuccess(parsed map[string]interface{}) (bool, bool) {
	if len(parsed) == 0 {
		return false, false
	}
	raw, ok := parsed["success"]
	if !ok || raw == nil {
		return false, false
	}
	switch typed := raw.(type) {
	case bool:
		return true, typed
	case string:
		switch strings.ToLower(strings.TrimSpace(typed)) {
		case "true":
			return true, true
		case "false":
			return true, false
		}
	}
	return false, false
}

func extractResponseMessage(body []byte) string {
	trimmed := strings.TrimSpace(string(body))
	if trimmed == "" || strings.HasPrefix(trimmed, "{") || strings.HasPrefix(trimmed, "[") {
		return ""
	}
	return trimmed
}

func extractMapString(data map[string]interface{}, keys ...string) string {
	for _, key := range keys {
		value, ok := data[key]
		if !ok || isEmptyValue(value) {
			continue
		}
		switch typed := value.(type) {
		case string:
			return strings.TrimSpace(typed)
		default:
			return strings.TrimSpace(fmt.Sprint(typed))
		}
	}
	return ""
}
