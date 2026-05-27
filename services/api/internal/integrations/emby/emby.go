package emby

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	configpkg "github.com/konghang/ember/backend/internal/config"
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
	service := &EmbyService{
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
	service.refreshConfig()
	return service
}

var (
	sharedEmbyOnce    sync.Once
	sharedEmbyService *EmbyService
)

// ErrEmbyUserNotFound 表示目标 Emby 用户不存在或已被删除。
var ErrEmbyUserNotFound = errors.New("Emby 用户不存在")

// GetSharedService 返回进程内共享的 EmbyService 单例。
// 配置在首次调用时从数据库读取一次；后续调用直接返回已初始化实例。
func GetSharedService() *EmbyService {
	sharedEmbyOnce.Do(func() {
		sharedEmbyService = NewEmbyService()
	})
	return sharedEmbyService
}

func (s *EmbyService) refreshConfig() {
	configService := configpkg.NewConfigService()
	s.baseURL = strings.TrimRight(configService.GetString("EMBY_URL"), "/")
	s.apiKey = strings.TrimSpace(configService.GetString("EMBY_API_KEY"))
}

// ensureConfigured 在判断配置是否齐全前先从设置中心拉一次最新值，
// 与 TVCalendarService / MoviePilot 的"每次调用前 refresh"模式对齐，
// 让 admin 在设置中心改完 Emby 配置后无需重启 API 即可生效。
func (s *EmbyService) ensureConfigured() error {
	s.refreshConfig()
	if s.baseURL == "" || s.apiKey == "" {
		return errors.New("Emby 配置未设置（EMBY_URL 或 EMBY_API_KEY）")
	}
	return nil
}

func (s *EmbyService) IsConfigured() bool {
	return s.ensureConfigured() == nil
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
	if err := s.ensureConfigured(); err != nil {
		return nil, err
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
	if err := s.ensureConfigured(); err != nil {
		return nil, err
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

	if resp.StatusCode == http.StatusNotFound {
		return nil, ErrEmbyUserNotFound
	}

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("获取 Emby 用户失败：状态码 %d，响应 %s", resp.StatusCode, string(body))
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
	if err := s.ensureConfigured(); err != nil {
		return err
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
	if err := s.ensureConfigured(); err != nil {
		return err
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
	if err := s.ensureConfigured(); err != nil {
		return nil, err
	}

	createURL := fmt.Sprintf("%s/emby/Users/New?api_key=%s", s.baseURL, s.apiKey)

	reqBody := map[string]interface{}{
		"Name": username,
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest("POST", createURL, bytes.NewBuffer(jsonData))
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

	// /Users/New 不支持在请求体中设置密码，必须通过独立的 Password 端点设置
	if err := s.UpdateUserPassword(user.ID, password); err != nil {
		_ = s.DeleteUser(user.ID)
		return nil, fmt.Errorf("设置 Emby 用户密码失败: %w", err)
	}

	return &user, nil
}

// DeleteUser 删除 Emby 用户
func (s *EmbyService) DeleteUser(embyUserID string) error {
	if err := s.ensureConfigured(); err != nil {
		return err
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
	if err := s.ensureConfigured(); err != nil {
		return nil, err
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

// EmbyItem Emby 媒体项（/emby/Users/{userId}/Items/Latest 返回结构）
type EmbyItem struct {
	ID              string            `json:"Id"`
	Name            string            `json:"Name"`
	Type            string            `json:"Type"` // Movie / Series
	ProductionYear  int               `json:"ProductionYear"`
	DateCreated     string            `json:"DateCreated"`
	CommunityRating *float64          `json:"CommunityRating,omitempty"`
	OfficialRating  *string           `json:"OfficialRating,omitempty"`
	Overview        *string           `json:"Overview,omitempty"`
	ImageTags       map[string]string `json:"ImageTags"`
	ChildCount      int               `json:"ChildCount"` // GroupItems=true 时的新增子项数
}

func latestIncludeItemTypes(itemType string) string {
	if itemType == "Series" {
		// Emby 官方示例：最近更新的剧集应按 Episode 查询，再通过 GroupItems=true 聚合为剧集容器。
		return "Episode"
	}
	return itemType
}

func sanitizeEmbyErrorBody(b []byte) string {
	// Emby 有时会返回 HTML 编码的异常信息（例如 &#39;），这里做反转义以便阅读。
	// 同时截断，避免把大段 HTML/堆栈塞进日志/响应里。
	const maxLen = 2048
	s := strings.TrimSpace(string(b))
	s = html.UnescapeString(s)
	if len(s) > maxLen {
		return s[:maxLen] + "...(truncated)"
	}
	return s
}

// GetMediaStats 获取媒体库统计信息
func (s *EmbyService) GetMediaStats() (*MediaStats, error) {
	if err := s.ensureConfigured(); err != nil {
		return nil, err
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

// GetLatestItems 获取用户视角的最近入库媒体
// 使用 /emby/Users/{userId}/Items/Latest 端点（返回裸数组 []EmbyItem）
func (s *EmbyService) GetLatestItems(embyUserID string, itemType string, limit int) ([]EmbyItem, error) {
	if err := s.ensureConfigured(); err != nil {
		return nil, err
	}
	if strings.TrimSpace(embyUserID) == "" {
		return nil, errors.New("Emby 用户 ID 不能为空")
	}
	if itemType != "Movie" && itemType != "Series" {
		return nil, errors.New("无效的 itemType（仅支持 Movie / Series）")
	}
	if limit <= 0 {
		limit = 20
	}
	if limit > 50 {
		limit = 50
	}

	base, err := url.Parse(s.baseURL)
	if err != nil {
		return nil, fmt.Errorf("Emby URL 无效：%w", err)
	}

	escapedUserID := url.PathEscape(embyUserID)
	base.Path = strings.TrimRight(base.Path, "/") + "/emby/Users/" + escapedUserID + "/Items/Latest"

	q := base.Query()
	q.Set("IncludeItemTypes", latestIncludeItemTypes(itemType))
	q.Set("Limit", strconv.Itoa(limit))
	q.Set("GroupItems", "true")
	q.Set("Fields", "Overview,DateCreated,ProductionYear,CommunityRating,OfficialRating")
	q.Set("api_key", s.apiKey)
	base.RawQuery = q.Encode()

	req, err := http.NewRequest("GET", base.String(), nil)
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
		return nil, fmt.Errorf("Emby API 返回异常状态码 %d: %s", resp.StatusCode, sanitizeEmbyErrorBody(body))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var items []EmbyItem
	if err := json.Unmarshal(body, &items); err != nil {
		return nil, err
	}
	return items, nil
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
	if err := s.ensureConfigured(); err != nil {
		return nil, err
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

// GetUserPolicyRaw 获取 Emby 用户完整策略。
func (s *EmbyService) GetUserPolicyRaw(embyUserID string) (map[string]any, error) {
	return s.getUserPolicyRaw(embyUserID)
}

func (s *EmbyService) setUserPolicyRaw(embyUserID string, policy map[string]any) error {
	if err := s.ensureConfigured(); err != nil {
		return err
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

// PatchUserPolicyFields 仅按白名单字段覆盖目标用户策略，其他字段保持不变。
func (s *EmbyService) PatchUserPolicyFields(targetUserID string, sourcePolicy map[string]any, fields []string) error {
	targetPolicy, err := s.getUserPolicyRaw(targetUserID)
	if err != nil {
		return err
	}

	for _, field := range fields {
		if value, ok := sourcePolicy[field]; ok {
			targetPolicy[field] = value
		}
	}

	return s.setUserPolicyRaw(targetUserID, targetPolicy)
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

	// 降级：旧行为（可能丢字段，但比"注册失败"更可接受）。
	return s.SetUserPolicy(embyUserID, NewDefaultUserPolicy(isDisabled))
}

// SetUserPolicy 更新用户权限策略
func (s *EmbyService) SetUserPolicy(embyUserID string, policy EmbyUserPolicy) error {
	if err := s.ensureConfigured(); err != nil {
		return err
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

// CustomQueryResponse Emby user_usage_stats 插件自定义查询响应
// 注意：插件返回的字段名是 "colums"（非 "columns"），这是插件本身的拼写
type CustomQueryResponse struct {
	Colums  []string        `json:"colums"`
	Columns []string        `json:"columns"`
	Results [][]interface{} `json:"results"`
	Message string          `json:"message"`
}

// QueryPlaybackStats 通过 Playback Reporting 插件执行自定义 SQL 查询
// 插件 API：POST /emby/user_usage_stats/submit_custom_query
// 请求体：{"CustomQueryString": "SQL"}
func (s *EmbyService) QueryPlaybackStats(sql string) (*CustomQueryResponse, error) {
	if err := s.ensureConfigured(); err != nil {
		return nil, err
	}

	reqBody := map[string]string{
		"CustomQueryString": sql,
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, err
	}

	url := fmt.Sprintf("%s/emby/user_usage_stats/submit_custom_query", s.baseURL)
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Emby-Token", s.apiKey)

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("无法连接到 Emby 服务器：%v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Playback Reporting 查询失败：HTTP %d: %s", resp.StatusCode, string(body))
	}

	var out CustomQueryResponse
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, err
	}

	return &out, nil
}

// ==================== Sessions（活跃会话） ====================

// EmbyNowPlayingItem 当前播放媒体信息
type EmbyNowPlayingItem struct {
	Name              string `json:"Name"`
	ID                string `json:"Id"`
	Type              string `json:"Type"`                        // "Movie" 或 "Episode"
	MediaType         string `json:"MediaType"`                   // "Video"
	RunTimeTicks      int64  `json:"RunTimeTicks"`                // 总时长（ticks，÷10000000=秒）
	SeriesName        string `json:"SeriesName,omitempty"`        // 剧集名（仅 Episode）
	IndexNumber       int    `json:"IndexNumber,omitempty"`       // 集号（仅 Episode）
	ParentIndexNumber int    `json:"ParentIndexNumber,omitempty"` // 季号（仅 Episode）
	ProductionYear    int    `json:"ProductionYear,omitempty"`    // 年份
}

// EmbyPlayState 播放状态
type EmbyPlayState struct {
	PositionTicks int64  `json:"PositionTicks"` // 当前进度（ticks）
	IsPaused      bool   `json:"IsPaused"`
	IsMuted       bool   `json:"IsMuted"`
	PlayMethod    string `json:"PlayMethod"` // "DirectPlay" / "DirectStream" / "Transcode"
}

// EmbySession Emby 会话信息
type EmbySession struct {
	ID                 string              `json:"Id"`
	UserID             string              `json:"UserId"`
	UserName           string              `json:"UserName"`
	Client             string              `json:"Client"`     // 客户端名称（Emby Web, Infuse...）
	DeviceName         string              `json:"DeviceName"` // 设备名称
	DeviceID           string              `json:"DeviceId"`
	RemoteEndPoint     string              `json:"RemoteEndPoint"` // 客户端 IP
	ApplicationVersion string              `json:"ApplicationVersion"`
	LastActivityDate   string              `json:"LastActivityDate"`
	NowPlayingItem     *EmbyNowPlayingItem `json:"NowPlayingItem,omitempty"` // 仅播放中存在
	PlayState          *EmbyPlayState      `json:"PlayState,omitempty"`
}

func (s *EmbyService) fetchSessions(playingOnly bool) ([]EmbySession, error) {
	if err := s.ensureConfigured(); err != nil {
		return nil, err
	}

	endpoint := fmt.Sprintf("%s/emby/Sessions", s.baseURL)
	req, err := http.NewRequest("GET", endpoint, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("X-Emby-Token", s.apiKey)

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("无法连接到 Emby 服务器：%v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("获取 Emby 会话列表失败：HTTP %d: %s", resp.StatusCode, string(body))
	}

	var sessions []EmbySession
	if err := json.Unmarshal(body, &sessions); err != nil {
		return nil, err
	}

	if !playingOnly {
		return sessions, nil
	}

	// 只返回正在播放的会话（有 NowPlayingItem 的）
	playing := make([]EmbySession, 0)
	for _, session := range sessions {
		if session.NowPlayingItem != nil {
			playing = append(playing, session)
		}
	}

	return playing, nil
}

// GetSessions 获取 Emby 当前正在播放的会话
func (s *EmbyService) GetSessions() ([]EmbySession, error) {
	return s.fetchSessions(true)
}

// GetAllSessions 获取 Emby 所有会话（包含非播放中会话）
func (s *EmbyService) GetAllSessions() ([]EmbySession, error) {
	return s.fetchSessions(false)
}

// EmbyDevice Emby 设备信息
type EmbyDevice struct {
	ID               string `json:"Id"`
	Name             string `json:"Name"`
	AppName          string `json:"AppName"`
	AppVersion       string `json:"AppVersion"`
	LastUserName     string `json:"LastUserName"`
	LastUserID       string `json:"LastUserId"`
	LastActivityDate string `json:"LastActivityDate"`
	LastUsedDate     string `json:"LastUsedDate"`
	DateCreated      string `json:"DateCreated"`
}

type embyDeviceListResponse struct {
	Items   []EmbyDevice `json:"Items"`
	Devices []EmbyDevice `json:"Devices"`
}

// GetDevices 获取 Emby 设备列表
func (s *EmbyService) GetDevices() ([]EmbyDevice, error) {
	if err := s.ensureConfigured(); err != nil {
		return nil, err
	}

	endpoint := fmt.Sprintf("%s/emby/Devices?api_key=%s", s.baseURL, s.apiKey)
	req, err := http.NewRequest("GET", endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("X-Emby-Token", s.apiKey)

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("无法连接到 Emby 服务器：%v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("获取 Emby 设备列表失败：HTTP %d: %s", resp.StatusCode, string(body))
	}

	trimmed := strings.TrimSpace(string(body))
	if strings.HasPrefix(trimmed, "[") {
		var devices []EmbyDevice
		if err := json.Unmarshal(body, &devices); err != nil {
			return nil, err
		}
		return devices, nil
	}

	var wrapper embyDeviceListResponse
	if err := json.Unmarshal(body, &wrapper); err != nil {
		return nil, err
	}
	if len(wrapper.Items) > 0 {
		return wrapper.Items, nil
	}
	return wrapper.Devices, nil
}

// LogoutDevice 强制设备下线
func (s *EmbyService) LogoutDevice(deviceID string) error {
	if err := s.ensureConfigured(); err != nil {
		return err
	}
	deviceID = strings.TrimSpace(deviceID)
	if deviceID == "" {
		return errors.New("设备 ID 不能为空")
	}

	escaped := url.PathEscape(deviceID)
	endpoint := fmt.Sprintf("%s/emby/Devices/%s?api_key=%s", s.baseURL, escaped, s.apiKey)
	req, err := http.NewRequest("DELETE", endpoint, nil)
	if err != nil {
		return err
	}
	req.Header.Set("X-Emby-Token", s.apiKey)

	resp, err := s.client.Do(req)
	if err != nil {
		return fmt.Errorf("无法连接到 Emby 服务器：%v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusNoContent || resp.StatusCode == http.StatusNotFound {
		return nil
	}

	body, _ := io.ReadAll(resp.Body)
	return fmt.Errorf("强制设备下线失败：HTTP %d: %s", resp.StatusCode, string(body))
}
