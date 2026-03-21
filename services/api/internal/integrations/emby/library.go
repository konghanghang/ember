package emby

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

const (
	defaultLibraryItemPageSize = 200
	maxItemIDsPerBatch         = 100
)

// EmbyLibrary 媒体库信息
type EmbyLibrary struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Type      string `json:"type"`
	ItemCount int    `json:"itemCount"`
}

// EmbyMediaStream 媒体流信息
type EmbyMediaStream struct {
	Type           string `json:"Type"`
	Codec          string `json:"Codec"`
	Width          int    `json:"Width"`
	Height         int    `json:"Height"`
	BitRate        int    `json:"BitRate"`
	VideoRange     string `json:"VideoRange"`
	VideoRangeType string `json:"VideoRangeType"`
}

// EmbyMediaSource 媒体源信息
type EmbyMediaSource struct {
	MediaStreams []EmbyMediaStream `json:"MediaStreams"`
}

// EmbyLibraryItem 媒体库条目
type EmbyLibraryItem struct {
	ID                string            `json:"Id"`
	Name              string            `json:"Name"`
	Type              string            `json:"Type"`
	ParentID          string            `json:"ParentId"`
	SeriesID          string            `json:"SeriesId"`
	SeriesName        string            `json:"SeriesName"`
	ParentIndexNumber int               `json:"ParentIndexNumber"`
	IndexNumber       int               `json:"IndexNumber"`
	MediaStreams      []EmbyMediaStream `json:"MediaStreams"`
	MediaSources      []EmbyMediaSource `json:"MediaSources"`
}

type embyLibraryItemsResponse struct {
	Items            []EmbyLibraryItem `json:"Items"`
	TotalRecordCount int               `json:"TotalRecordCount"`
}

// GetItemsByIDs 按条目 ID 批量读取媒体详情。
// 目前主要给 PlaybackActivity 二阶段聚合使用：先拿 ItemId，再回查 SeriesId / SeriesName。
func (s *EmbyService) GetItemsByIDs(itemIDs []string) ([]EmbyLibraryItem, error) {
	if err := s.ensureConfigured(); err != nil {
		return nil, err
	}

	normalized := make([]string, 0, len(itemIDs))
	seen := make(map[string]struct{}, len(itemIDs))
	for _, rawID := range itemIDs {
		id := strings.TrimSpace(rawID)
		if id == "" {
			continue
		}
		if _, exists := seen[id]; exists {
			continue
		}
		seen[id] = struct{}{}
		normalized = append(normalized, id)
	}
	if len(normalized) == 0 {
		return []EmbyLibraryItem{}, nil
	}

	items := make([]EmbyLibraryItem, 0, len(normalized))
	for start := 0; start < len(normalized); start += maxItemIDsPerBatch {
		end := start + maxItemIDsPerBatch
		if end > len(normalized) {
			end = len(normalized)
		}

		params := map[string]string{
			"Ids":    strings.Join(normalized[start:end], ","),
			"Fields": "ParentId,SeriesName,SeriesId,ParentIndexNumber,IndexNumber",
			"Limit":  strconv.Itoa(end - start),
		}

		body, err := s.getWithAPIKey("/emby/Items", params)
		if err != nil {
			return nil, err
		}

		var out embyLibraryItemsResponse
		if err := json.Unmarshal(body, &out); err != nil {
			return nil, fmt.Errorf("解析 Emby 条目详情失败: %w", err)
		}

		items = append(items, out.Items...)
	}

	return items, nil
}

// GetLibraries 获取媒体库列表（兼容不同 Emby 版本响应结构）
func (s *EmbyService) GetLibraries() ([]EmbyLibrary, error) {
	if err := s.ensureConfigured(); err != nil {
		return nil, err
	}

	endpoints := []string{
		"/emby/Library/VirtualFolders/Query",
		"/emby/Library/VirtualFolders",
	}

	var lastErr error
	for _, endpoint := range endpoints {
		body, err := s.getWithAPIKey(endpoint, nil)
		if err != nil {
			lastErr = err
			continue
		}

		libraries, parseErr := parseLibrariesFromBody(body)
		if parseErr != nil {
			lastErr = parseErr
			continue
		}
		return libraries, nil
	}

	if lastErr != nil {
		return nil, lastErr
	}
	return []EmbyLibrary{}, nil
}

// GetLibraryItems 获取媒体库中的条目（分页抓取；maxItems<=0 表示全量）
func (s *EmbyService) GetLibraryItems(libraryID string, maxItems int) ([]EmbyLibraryItem, error) {
	if err := s.ensureConfigured(); err != nil {
		return nil, err
	}

	libraryID = strings.TrimSpace(libraryID)
	if libraryID == "" {
		return nil, errors.New("libraryId 不能为空")
	}

	items := make([]EmbyLibraryItem, 0)
	startIndex := 0

	for {
		limit := defaultLibraryItemPageSize
		if maxItems > 0 {
			if len(items) >= maxItems {
				break
			}
			remaining := maxItems - len(items)
			if remaining < limit {
				limit = remaining
			}
		}

		resp, err := s.getLibraryItemsPage(libraryID, startIndex, limit)
		if err != nil {
			return nil, err
		}
		fetchedCount := len(resp.Items)
		if fetchedCount == 0 {
			break
		}

		if maxItems > 0 && len(items)+fetchedCount > maxItems {
			allowed := maxItems - len(items)
			items = append(items, resp.Items[:allowed]...)
			break
		}

		items = append(items, resp.Items...)
		startIndex += fetchedCount

		if resp.TotalRecordCount > 0 && startIndex >= resp.TotalRecordCount {
			break
		}
		if fetchedCount < limit {
			break
		}
	}

	return items, nil
}

func (s *EmbyService) getLibraryItemsPage(libraryID string, startIndex, limit int) (*embyLibraryItemsResponse, error) {
	params := map[string]string{
		"ParentId":         libraryID,
		"Recursive":        "true",
		"IncludeItemTypes": "Movie,Episode",
		"Fields":           "MediaStreams,MediaSources,ParentId,SeriesName,SeriesId,ParentIndexNumber,IndexNumber",
		"StartIndex":       strconv.Itoa(startIndex),
		"Limit":            strconv.Itoa(limit),
	}

	body, err := s.getWithAPIKey("/emby/Items", params)
	if err != nil {
		return nil, err
	}

	var out embyLibraryItemsResponse
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, fmt.Errorf("解析 Emby 媒体库条目失败: %w", err)
	}

	return &out, nil
}

func (s *EmbyService) getWithAPIKey(path string, params map[string]string) ([]byte, error) {
	if err := s.ensureConfigured(); err != nil {
		return nil, err
	}

	base, err := url.Parse(s.baseURL)
	if err != nil {
		return nil, fmt.Errorf("Emby URL 无效：%w", err)
	}

	base.Path = strings.TrimRight(base.Path, "/") + path
	query := base.Query()
	query.Set("api_key", s.apiKey)
	for key, value := range params {
		query.Set(key, value)
	}
	base.RawQuery = query.Encode()

	req, err := http.NewRequest("GET", base.String(), nil)
	if err != nil {
		return nil, err
	}

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
		return nil, fmt.Errorf("Emby API 返回异常状态码 %d: %s", resp.StatusCode, sanitizeEmbyErrorBody(body))
	}

	return body, nil
}

func (s *EmbyService) GetWithAPIKey(path string, params map[string]string) ([]byte, error) {
	return s.getWithAPIKey(path, params)
}

// GetItemPrimaryImage 获取条目主图（二进制）
func (s *EmbyService) GetItemPrimaryImage(itemID string, maxHeight int, quality int) ([]byte, string, error) {
	if err := s.ensureConfigured(); err != nil {
		return nil, "", err
	}

	itemID = strings.TrimSpace(itemID)
	if itemID == "" {
		return nil, "", errors.New("itemId 不能为空")
	}

	if maxHeight <= 0 {
		maxHeight = 180
	}
	if quality <= 0 || quality > 100 {
		quality = 90
	}

	base, err := url.Parse(s.baseURL)
	if err != nil {
		return nil, "", fmt.Errorf("Emby URL 无效：%w", err)
	}

	base.Path = strings.TrimRight(base.Path, "/") + "/emby/Items/" + url.PathEscape(itemID) + "/Images/Primary"
	query := base.Query()
	query.Set("api_key", s.apiKey)
	query.Set("maxHeight", strconv.Itoa(maxHeight))
	query.Set("quality", strconv.Itoa(quality))
	base.RawQuery = query.Encode()

	req, err := http.NewRequest("GET", base.String(), nil)
	if err != nil {
		return nil, "", err
	}

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("无法连接到 Emby 服务器：%v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, "", err
	}

	if resp.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("Emby 图片接口异常 %d: %s", resp.StatusCode, sanitizeEmbyErrorBody(body))
	}

	contentType := strings.TrimSpace(resp.Header.Get("Content-Type"))
	if contentType == "" {
		contentType = "image/jpeg"
	}

	return body, contentType, nil
}

func parseLibrariesFromBody(body []byte) ([]EmbyLibrary, error) {
	type rawLibrary struct {
		Guid           string `json:"Guid"`
		ItemID         string `json:"ItemId"`
		ID             string `json:"Id"`
		Name           string `json:"Name"`
		CollectionType string `json:"CollectionType"`
		ItemCount      int    `json:"ItemCount"`
	}

	toLibrary := func(item rawLibrary) (EmbyLibrary, bool) {
		id := strings.TrimSpace(item.Guid)
		if id == "" {
			id = strings.TrimSpace(item.ItemID)
		}
		if id == "" {
			id = strings.TrimSpace(item.ID)
		}
		if id == "" {
			return EmbyLibrary{}, false
		}

		name := strings.TrimSpace(item.Name)
		if name == "" {
			name = id
		}

		libraryType := strings.TrimSpace(item.CollectionType)
		if libraryType == "" {
			libraryType = "unknown"
		}

		return EmbyLibrary{
			ID:        id,
			Name:      name,
			Type:      libraryType,
			ItemCount: item.ItemCount,
		}, true
	}

	var queryResp struct {
		Items []rawLibrary `json:"Items"`
	}
	if err := json.Unmarshal(body, &queryResp); err == nil {
		result := make([]EmbyLibrary, 0, len(queryResp.Items))
		for _, item := range queryResp.Items {
			library, ok := toLibrary(item)
			if ok {
				result = append(result, library)
			}
		}
		return result, nil
	}

	var directResp []rawLibrary
	if err := json.Unmarshal(body, &directResp); err != nil {
		return nil, fmt.Errorf("解析 Emby 媒体库列表失败: %w", err)
	}

	result := make([]EmbyLibrary, 0, len(directResp))
	for _, item := range directResp {
		library, ok := toLibrary(item)
		if ok {
			result = append(result, library)
		}
	}
	return result, nil
}
