package moviepilot

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
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

// IsConfigured 检查配置是否完整
func (c *MoviePilotClient) IsConfigured() bool {
	c.refreshConfig()
	return c.baseURL != "" && c.apiKey != ""
}

func (c *MoviePilotClient) TestConnection() error {
	configService := configpkg.NewConfigService()
	c.refreshConfig()
	if c.baseURL != "" && c.apiKey == "" && configService.MoviePilotNeedsAPIKeyMigration() {
		return fmt.Errorf("检测到旧版 MoviePilot 用户名/密码配置，请迁移到 MOVIEPILOT_API_KEY")
	}
	if c.baseURL == "" || c.apiKey == "" {
		return fmt.Errorf("MoviePilot 未配置")
	}

	req, err := http.NewRequest(http.MethodGet, c.baseURL+"/api/v1/site/", nil)
	if err != nil {
		return fmt.Errorf("创建请求失败: %w", err)
	}
	req.Header.Set("X-API-KEY", c.apiKey)

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

// CreateSubscription 创建订阅
//
// @param data.Type - 媒体类型（"movie" | "tv"）
// @param data.Name - 影视名称
// @param data.TmdbID - TMDB ID（字符串，会转为整数）
// @returns MoviePilot API 响应
func (c *MoviePilotClient) CreateSubscription(data SubscribeRequest) error {
	configService := configpkg.NewConfigService()
	c.refreshConfig()
	if c.baseURL != "" && c.apiKey == "" && configService.MoviePilotNeedsAPIKeyMigration() {
		return fmt.Errorf("检测到旧版 MoviePilot 用户名/密码配置，请迁移到 MOVIEPILOT_API_KEY")
	}
	if c.baseURL == "" || c.apiKey == "" {
		return fmt.Errorf("MoviePilot 未配置")
	}

	requestBody, err := buildSubscribeRequestBody(data)
	if err != nil {
		return err
	}

	jsonData, err := json.Marshal(requestBody)
	if err != nil {
		return fmt.Errorf("构建请求失败: %w", err)
	}

	req, err := http.NewRequest(http.MethodPost, c.baseURL+"/api/v1/subscribe/", bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("创建请求失败: %w", err)
	}

	req.Header.Set("X-API-KEY", c.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("订阅请求失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("MoviePilot API 错误: %d %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	return nil
}
