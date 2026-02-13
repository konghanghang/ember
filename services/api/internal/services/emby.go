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

const (
	defaultUserSimultaneousStreamLimit  = 3
	defaultUserEnableContentDownloading = false
	defaultUserEnableContentDeletion    = false
	defaultUserEnableLiveTvAccess       = false
	defaultUserEnableSyncTranscoding    = false
	defaultUserEnableMediaPlayback      = true
	defaultUserEnableAudioTranscoding   = false
	defaultUserEnableVideoTranscoding   = false
	defaultUserEnablePlaybackRemuxing   = true
	defaultUserEnableRemoteAccess       = true
	defaultUserIsAdministrator          = false
)

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

	// 创建后立即收敛默认权限：禁止下载，最多 3 路同时播放。
	// 如果设置失败，为避免产生“半成品用户”，尽量回滚删除该 Emby 用户。
	if err := s.ApplyEmberDefaultUserPolicy(user.ID, false); err != nil {
		_ = s.DeleteUser(user.ID)
		return nil, fmt.Errorf("设置 Emby 用户默认权限失败: %w", err)
	}

	return &user, nil
}

// DeleteUser 删除 Emby 用户
func (s *EmbyService) DeleteUser(embyUserID string) error {
	if s.baseURL == "" || s.apiKey == "" {
		return errors.New("Emby 配置未设置")
	}

	url := fmt.Sprintf("%s/emby/Users/%s", s.baseURL, embyUserID)
	req, err := http.NewRequest("DELETE", url, nil)
	if err != nil {
		return err
	}

	req.Header.Set("X-Emby-Token", s.apiKey)

	resp, err := s.client.Do(req)
	if err != nil {
		return fmt.Errorf("无法连接到 Emby 服务器：%v", err)
	}
	defer resp.Body.Close()

	// 删除接口按幂等处理：用户不存在也视为成功
	if resp.StatusCode == 404 {
		return nil
	}

	if resp.StatusCode != 200 && resp.StatusCode != 204 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("删除 Emby 用户失败：状态码 %d，响应 %s", resp.StatusCode, string(body))
	}

	return nil
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
	IsDisabled                     bool  `json:"IsDisabled"`
	IsAdministrator                *bool `json:"IsAdministrator,omitempty"`
	EnableContentDownloading       *bool `json:"EnableContentDownloading,omitempty"`
	EnableContentDeletion          *bool `json:"EnableContentDeletion,omitempty"`
	EnableLiveTvAccess             *bool `json:"EnableLiveTvAccess,omitempty"`
	EnableSyncTranscoding          *bool `json:"EnableSyncTranscoding,omitempty"`
	EnableMediaPlayback            *bool `json:"EnableMediaPlayback,omitempty"`
	EnableAudioPlaybackTranscoding *bool `json:"EnableAudioPlaybackTranscoding,omitempty"`
	EnableVideoPlaybackTranscoding *bool `json:"EnableVideoPlaybackTranscoding,omitempty"`
	EnablePlaybackRemuxing         *bool `json:"EnablePlaybackRemuxing,omitempty"`
	EnableRemoteAccess             *bool `json:"EnableRemoteAccess,omitempty"`
	SimultaneousStreamLimit        *int  `json:"SimultaneousStreamLimit,omitempty"`
}

// NewDefaultUserPolicy 构建 Ember 默认用户策略。
// 规则：尽量只允许播放（禁下载/禁转码），最大同时播放 3，封禁状态由入参控制。
func NewDefaultUserPolicy(isDisabled bool) EmbyUserPolicy {
	allowDownloading := defaultUserEnableContentDownloading
	streamLimit := defaultUserSimultaneousStreamLimit
	allowDeletion := defaultUserEnableContentDeletion
	allowLiveTv := defaultUserEnableLiveTvAccess
	allowSyncTranscoding := defaultUserEnableSyncTranscoding
	allowPlayback := defaultUserEnableMediaPlayback
	allowAudioTranscoding := defaultUserEnableAudioTranscoding
	allowVideoTranscoding := defaultUserEnableVideoTranscoding
	allowRemux := defaultUserEnablePlaybackRemuxing
	allowRemote := defaultUserEnableRemoteAccess
	isAdmin := defaultUserIsAdministrator

	return EmbyUserPolicy{
		IsDisabled:                     isDisabled,
		IsAdministrator:                &isAdmin,
		EnableContentDownloading:       &allowDownloading,
		EnableContentDeletion:          &allowDeletion,
		EnableLiveTvAccess:             &allowLiveTv,
		EnableSyncTranscoding:          &allowSyncTranscoding,
		EnableMediaPlayback:            &allowPlayback,
		EnableAudioPlaybackTranscoding: &allowAudioTranscoding,
		EnableVideoPlaybackTranscoding: &allowVideoTranscoding,
		EnablePlaybackRemuxing:         &allowRemux,
		EnableRemoteAccess:             &allowRemote,
		SimultaneousStreamLimit:        &streamLimit,
	}
}

func (s *EmbyService) getUserPolicyRaw(embyUserID string) (map[string]any, error) {
	if s.baseURL == "" || s.apiKey == "" {
		return nil, errors.New("Emby 配置未设置")
	}

	url := fmt.Sprintf("%s/emby/Users/%s/Policy", s.baseURL, embyUserID)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("X-Emby-Token", s.apiKey)

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("无法连接到 Emby 服务器：%v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("获取用户策略失败：状态码 %d，响应 %s", resp.StatusCode, string(body))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var policy map[string]any
	if err := json.Unmarshal(body, &policy); err != nil {
		return nil, err
	}
	return policy, nil
}

func (s *EmbyService) setUserPolicyRaw(embyUserID string, policy map[string]any) error {
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
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("更新用户策略失败：状态码 %d，响应 %s", resp.StatusCode, string(body))
	}
	return nil
}

// ApplyEmberDefaultUserPolicy 尝试在不破坏其他策略字段的前提下，应用 Ember 默认用户权限。
// 首选：GET 完整 policy → patch → POST；失败则降级为直接 POST patch（兼容旧行为）。
func (s *EmbyService) ApplyEmberDefaultUserPolicy(embyUserID string, isDisabled bool) error {
	policy, err := s.getUserPolicyRaw(embyUserID)
	if err == nil {
		policy["IsDisabled"] = isDisabled
		policy["IsAdministrator"] = defaultUserIsAdministrator
		policy["EnableContentDownloading"] = defaultUserEnableContentDownloading
		policy["EnableContentDeletion"] = defaultUserEnableContentDeletion
		policy["EnableLiveTvAccess"] = defaultUserEnableLiveTvAccess
		policy["EnableSyncTranscoding"] = defaultUserEnableSyncTranscoding
		policy["EnableMediaPlayback"] = defaultUserEnableMediaPlayback
		policy["EnableAudioPlaybackTranscoding"] = defaultUserEnableAudioTranscoding
		policy["EnableVideoPlaybackTranscoding"] = defaultUserEnableVideoTranscoding
		policy["EnablePlaybackRemuxing"] = defaultUserEnablePlaybackRemuxing
		policy["EnableRemoteAccess"] = defaultUserEnableRemoteAccess

		// 并发播放限制字段名在不同 Emby 版本可能不同，只在字段存在时才写入，避免未知字段导致 Emby 拒绝。
		if _, ok := policy["SimultaneousStreamLimit"]; ok {
			policy["SimultaneousStreamLimit"] = defaultUserSimultaneousStreamLimit
		} else if _, ok := policy["MaxActiveSessions"]; ok {
			policy["MaxActiveSessions"] = defaultUserSimultaneousStreamLimit
		}

		if err := s.setUserPolicyRaw(embyUserID, policy); err == nil {
			return nil
		}
	}

	// 降级：旧行为（可能丢字段，但比“注册失败”更可接受）。
	return s.SetUserPolicy(embyUserID, NewDefaultUserPolicy(isDisabled))
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
