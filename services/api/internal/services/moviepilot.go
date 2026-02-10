package services

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"time"
)

// MoviePilotClient MoviePilot API 客户端
type MoviePilotClient struct {
	baseURL  string
	username string
	password string
	client   *http.Client
}

// NewMoviePilotClient 创建 MoviePilot 客户端
func NewMoviePilotClient() *MoviePilotClient {
	baseURL := os.Getenv("MOVIEPILOT_URL")
	username := os.Getenv("MOVIEPILOT_USERNAME")
	password := os.Getenv("MOVIEPILOT_PASSWORD")

	// 去除尾部斜杠
	if len(baseURL) > 0 && baseURL[len(baseURL)-1] == '/' {
		baseURL = baseURL[:len(baseURL)-1]
	}

	return &MoviePilotClient{
		baseURL:  baseURL,
		username: username,
		password: password,
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// IsConfigured 检查配置是否完整
func (c *MoviePilotClient) IsConfigured() bool {
	return c.baseURL != "" && c.username != "" && c.password != ""
}

// loginResponse MoviePilot 登录响应
type loginResponse struct {
	AccessToken string `json:"access_token"`
}

// login 登录获取 Access Token（每次都重新登录，不缓存）
//
// 不缓存 token 的原因：
// - 简化实现，无需处理 token 过期、刷新逻辑
// - 调用频率低（仅在审核通过时），性能影响可忽略
func (c *MoviePilotClient) login() (string, error) {
	// 构建表单数据
	formData := url.Values{}
	formData.Set("username", c.username)
	formData.Set("password", c.password)

	// 发送请求
	req, err := http.NewRequest("POST", c.baseURL+"/api/v1/login/access-token", bytes.NewBufferString(formData.Encode()))
	if err != nil {
		return "", fmt.Errorf("创建请求失败: %w", err)
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("登录请求失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("MoviePilot 登录失败: %d %s", resp.StatusCode, string(body))
	}

	// 解析响应
	var loginResp loginResponse
	if err := json.NewDecoder(resp.Body).Decode(&loginResp); err != nil {
		return "", fmt.Errorf("解析登录响应失败: %w", err)
	}

	return loginResp.AccessToken, nil
}

// SubscribeRequest 订阅请求
type SubscribeRequest struct {
	Type   string `json:"type"`   // "movie" | "tv"
	Name   string `json:"name"`   // 影视名称
	TmdbID string `json:"tmdbid"` // TMDB ID（字符串）
}

// CreateSubscription 创建订阅
//
// @param data.Type - 媒体类型（"movie" | "tv"）
// @param data.Name - 影视名称
// @param data.TmdbID - TMDB ID（字符串，会转为整数）
// @returns MoviePilot API 响应
// @throws 登录失败或 API 调用失败时返回错误
func (c *MoviePilotClient) CreateSubscription(data SubscribeRequest) error {
	// 1. 登录获取 token
	token, err := c.login()
	if err != nil {
		return err
	}

	// 2. 转换 TMDB ID 为整数
	tmdbIDInt, err := strconv.Atoi(data.TmdbID)
	if err != nil {
		return fmt.Errorf("无效的 TMDB ID: %s", data.TmdbID)
	}

	// 3. 转换类型为中文（MoviePilot 要求）
	typeZh := "电影"
	if data.Type == "tv" {
		typeZh = "电视剧"
	}

	// 4. 构建请求体
	requestBody := map[string]interface{}{
		"type":   typeZh,
		"name":   data.Name,
		"tmdbid": tmdbIDInt,
	}

	jsonData, err := json.Marshal(requestBody)
	if err != nil {
		return fmt.Errorf("构建请求失败: %w", err)
	}

	// 5. 发送订阅请求
	req, err := http.NewRequest("POST", c.baseURL+"/api/v1/subscribe/", bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("创建请求失败: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("订阅请求失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("MoviePilot API 错误: %d %s", resp.StatusCode, string(body))
	}

	return nil
}
