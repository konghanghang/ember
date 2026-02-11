package services

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
)

// EmbyService Emby API 服务
type EmbyService struct {
	baseURL string
	apiKey  string
	client  *http.Client
}

// NewEmbyService 创建 Emby 服务
func NewEmbyService() *EmbyService {
	return &EmbyService{
		baseURL: os.Getenv("EMBY_URL"),
		apiKey:  os.Getenv("EMBY_API_KEY"),
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// EmbyUser Emby 用户信息
type EmbyUser struct {
	ID          string `json:"Id"`
	Name        string `json:"Name"`
	ServerId    string `json:"ServerId"`
	HasPassword bool   `json:"HasPassword"`
}

// AuthenticateUserRequest Emby 用户认证请求
type AuthenticateUserRequest struct {
	Username string `json:"Username"`
	Pw       string `json:"Pw"` // 密码
}

// AuthenticateUserResponse Emby 用户认证响应
type AuthenticateUserResponse struct {
	User        EmbyUser `json:"User"`
	AccessToken string   `json:"AccessToken"`
	ServerId    string   `json:"ServerId"`
}

// AuthenticateUser 验证 Emby 用户
func (s *EmbyService) AuthenticateUser(username, password string) (*EmbyUser, error) {
	if s.baseURL == "" || s.apiKey == "" {
		return nil, errors.New("Emby 配置未设置（EMBY_URL 或 EMBY_API_KEY）")
	}

	// 构建请求
	reqBody := AuthenticateUserRequest{
		Username: username,
		Pw:       password,
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, err
	}

	url := fmt.Sprintf("%s/emby/Users/AuthenticateByName", s.baseURL)
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Emby-Token", s.apiKey)

	// 发送请求
	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("无法连接到 Emby 服务器：%v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	// 检查响应状态
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("Emby 认证失败：用户名或密码错误")
	}

	// 解析响应
	var authResp AuthenticateUserResponse
	if err := json.Unmarshal(body, &authResp); err != nil {
		return nil, err
	}

	return &authResp.User, nil
}

// GetUserByID 通过 Emby ID 获取用户信息
func (s *EmbyService) GetUserByID(embyUserID string) (*EmbyUser, error) {
	if s.baseURL == "" || s.apiKey == "" {
		return nil, errors.New("Emby 配置未设置")
	}

	url := fmt.Sprintf("%s/emby/Users/%s?api_key=%s", s.baseURL, embyUserID, s.apiKey)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("无法连接到 Emby 服务器：%v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("Emby 用户不存在")
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var user EmbyUser
	if err := json.Unmarshal(body, &user); err != nil {
		return nil, err
	}

	return &user, nil
}

// UpdateUserPassword 更新 Emby 用户密码
func (s *EmbyService) UpdateUserPassword(embyUserID, newPassword string) error {
	if s.baseURL == "" || s.apiKey == "" {
		return errors.New("Emby 配置未设置")
	}

	url := fmt.Sprintf("%s/emby/Users/%s/Password", s.baseURL, embyUserID)

	reqBody := map[string]interface{}{
		"Id":            embyUserID,
		"NewPw":         newPassword,
		"ResetPassword": false,
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return err
	}

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Emby-Token", s.apiKey)

	resp, err := s.client.Do(req)
	if err != nil {
		return fmt.Errorf("无法连接到 Emby 服务器：%v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 204 && resp.StatusCode != 200 {
		return fmt.Errorf("Emby 密码更新失败")
	}

	return nil
}

// TestConnection 测试 Emby 连接
func (s *EmbyService) TestConnection() error {
	if s.baseURL == "" || s.apiKey == "" {
		return errors.New("Emby 配置未设置")
	}

	url := fmt.Sprintf("%s/emby/System/Info?api_key=%s", s.baseURL, s.apiKey)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return err
	}

	resp, err := s.client.Do(req)
	if err != nil {
		return fmt.Errorf("无法连接到 Emby 服务器：%v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return fmt.Errorf("Emby API 密钥无效")
	}

	return nil
}

// CreateEmbyUser 创建 Emby 用户
func (s *EmbyService) CreateEmbyUser(username, password string) (*EmbyUser, error) {
	if s.baseURL == "" || s.apiKey == "" {
		return nil, errors.New("Emby 配置未设置")
	}

	url := fmt.Sprintf("%s/emby/Users/New?api_key=%s", s.baseURL, s.apiKey)

	reqBody := map[string]interface{}{
		"Name":     username,
		"Password": password,
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("无法连接到 Emby 服务器：%v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != 200 && resp.StatusCode != 201 {
		return nil, fmt.Errorf("创建 Emby 用户失败：%s", string(body))
	}

	var user EmbyUser
	if err := json.Unmarshal(body, &user); err != nil {
		return nil, err
	}

	return &user, nil
}

// GetUsers 获取所有 Emby 用户
func (s *EmbyService) GetUsers() ([]EmbyUser, error) {
	if s.baseURL == "" || s.apiKey == "" {
		return nil, errors.New("Emby 配置未设置")
	}

	url := fmt.Sprintf("%s/emby/Users?api_key=%s", s.baseURL, s.apiKey)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("无法连接到 Emby 服务器：%v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("获取 Emby 用户列表失败")
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var users []EmbyUser
	if err := json.Unmarshal(body, &users); err != nil {
		return nil, err
	}

	return users, nil
}

// MediaStats 媒体库统计信息
type MediaStats struct {
	MovieCount   int `json:"MovieCount"`
	SeriesCount  int `json:"SeriesCount"`
	EpisodeCount int `json:"EpisodeCount"`
}

// GetMediaStats 获取媒体库统计信息
func (s *EmbyService) GetMediaStats() (*MediaStats, error) {
	if s.baseURL == "" || s.apiKey == "" {
		return nil, errors.New("Emby 配置未设置")
	}

	url := fmt.Sprintf("%s/emby/Items/Counts?api_key=%s", s.baseURL, s.apiKey)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("无法连接到 Emby 服务器：%v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("Emby API 返回异常状态码 %d: %s", resp.StatusCode, string(body))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var stats MediaStats
	if err := json.Unmarshal(body, &stats); err != nil {
		return nil, err
	}

	return &stats, nil
}

// EmbyUserPolicy Emby 用户权限策略
type EmbyUserPolicy struct {
	IsDisabled bool `json:"IsDisabled"`
}

// SetUserPolicy 更新用户权限策略
func (s *EmbyService) SetUserPolicy(embyUserID string, policy EmbyUserPolicy) error {
	if s.baseURL == "" || s.apiKey == "" {
		return errors.New("Emby 配置未设置")
	}

	url := fmt.Sprintf("%s/emby/Users/%s/Policy", s.baseURL, embyUserID)

	jsonData, err := json.Marshal(policy)
	if err != nil {
		return err
	}

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Emby-Token", s.apiKey)

	resp, err := s.client.Do(req)
	if err != nil {
		return fmt.Errorf("无法连接到 Emby 服务器：%v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 204 && resp.StatusCode != 200 {
		return fmt.Errorf("更新用户策略失败")
	}

	return nil
}
