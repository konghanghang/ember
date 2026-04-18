package moviepilot

import (
	"bytes"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

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
		return nil, err
	}
	defer resp.Body.Close()

	body, readErr := io.ReadAll(resp.Body)
	if readErr != nil {
		return nil, fmt.Errorf("读取响应失败: %w", readErr)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("MoviePilot API 错误: %d %s", resp.StatusCode, strings.TrimSpace(string(body)))
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
		return fmt.Errorf("创建请求失败: %w", err)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("请求失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("MoviePilot 连接失败: %d %s", resp.StatusCode, strings.TrimSpace(string(body)))
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

// GapSearchRequest 缺集搜索请求
type GapSearchRequest struct {
	SeriesName string `json:"seriesName"`
	Season     int    `json:"season"`
	Episode    int    `json:"episode"`
}

// GapSearchCandidate 缺集候选摘要
type GapSearchCandidate struct {
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
}

// GapDispatchResponse 缺集下发响应
type GapDispatchResponse struct {
	Accepted   bool                   `json:"accepted"`
	StatusCode int                    `json:"statusCode"`
	Message    string                 `json:"message,omitempty"`
	Response   map[string]interface{} `json:"response,omitempty"`
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
	results, err := c.searchTitle(episodeQuery)
	if err != nil {
		return nil, err
	}

	response := &GapSearchResponse{
		Query:      episodeQuery,
		MatchMode:  "episode",
		Candidates: normalizeGapCandidates(results, "episode"),
	}
	if len(response.Candidates) > 0 {
		return response, nil
	}

	seasonQuery := fmt.Sprintf("%s S%02d", seriesName, data.Season)
	results, err = c.searchTitle(seasonQuery)
	if err != nil {
		return nil, err
	}

	response.FallbackQuery = seasonQuery
	response.MatchMode = "season"
	response.Candidates = normalizeGapCandidates(results, "season")
	return response, nil
}

func (c *MoviePilotClient) searchTitle(keyword string) ([]map[string]interface{}, error) {
	req, err := c.newRequest(http.MethodGet, "/api/v1/search/title", url.Values{
		"keyword": []string{keyword},
	}, nil)
	if err != nil {
		return nil, fmt.Errorf("创建请求失败: %w", err)
	}
	req.Header.Set("Accept", "application/json")

	body, err := c.doRequest(req)
	if err != nil {
		return nil, fmt.Errorf("搜索候选失败: %w", err)
	}

	return parseSearchResults(body)
}

// DispatchGapCandidate 将缺集候选下发给 MoviePilot。
func (c *MoviePilotClient) DispatchGapCandidate(data GapDispatchRequest) (*GapDispatchResponse, error) {
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
	jsonData, err := json.Marshal(requestBody)
	if err != nil {
		return nil, fmt.Errorf("构建请求失败: %w", err)
	}

	req, err := c.newRequest(http.MethodPost, "/api/v1/download/add", nil, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("创建请求失败: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("下发请求失败: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("读取响应失败: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("MoviePilot API 错误: %d %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	result := &GapDispatchResponse{
		Accepted:   true,
		StatusCode: resp.StatusCode,
		Message:    extractResponseMessage(body),
	}
	if parsed := parseJSONMap(body); len(parsed) > 0 {
		result.Response = parsed
		if result.Message == "" {
			result.Message = extractMapString(parsed, "message", "msg", "detail")
		}
	}

	return result, nil
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
		return fmt.Errorf("创建请求失败: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	if _, err := c.doRequest(req); err != nil {
		return fmt.Errorf("订阅请求失败: %w", err)
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

func normalizeGapCandidates(results []map[string]interface{}, matchMode string) []GapSearchCandidate {
	candidates := make([]GapSearchCandidate, 0, len(results))
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

		candidates = append(candidates, GapSearchCandidate{
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
