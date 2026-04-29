package emby

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	defaultLibraryItemPageSize = 200
	maxItemIDsPerBatch         = 100
	getItemsByIDsMaxAttempts   = 3
)

var (
	getItemsByIDsBatchTimeout = 3 * time.Second
	getItemsByIDsRetryBackoff = 200 * time.Millisecond
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

type embyHTTPStatusError struct {
	StatusCode int
	Body       string
}

func (e *embyHTTPStatusError) Error() string {
	return fmt.Sprintf("Emby API 返回异常状态码 %d: %s", e.StatusCode, e.Body)
}

type getItemsByIDsBatchError struct {
	TotalBatches  int
	FailedBatches int
	LastErr       error
}

func (e *getItemsByIDsBatchError) Error() string {
	if e == nil {
		return ""
	}
	if e.FailedBatches >= e.TotalBatches {
		return fmt.Sprintf("Emby 条目详情批量查询全部失败 batches=%d: %v", e.TotalBatches, e.LastErr)
	}
	return fmt.Sprintf("Emby 条目详情批量查询部分失败 failed=%d/%d: %v", e.FailedBatches, e.TotalBatches, e.LastErr)
}

func (e *getItemsByIDsBatchError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.LastErr
}

func IsGetItemsByIDsPartialFailure(err error) bool {
	var batchErr *getItemsByIDsBatchError
	if !errors.As(err, &batchErr) {
		return false
	}
	return batchErr != nil && batchErr.FailedBatches > 0 && batchErr.FailedBatches < batchErr.TotalBatches
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
	totalBatches := 0
	failedBatches := 0
	var lastErr error
	for start := 0; start < len(normalized); start += maxItemIDsPerBatch {
		end := start + maxItemIDsPerBatch
		if end > len(normalized) {
			end = len(normalized)
		}
		totalBatches++

		batchItems, err := s.getItemsByIDsBatch(normalized[start:end])
		if err != nil {
			failedBatches++
			lastErr = err
			log.Printf(
				"[Emby] GetItemsByIDs batch failed batch=%d/%d size=%d err=%v",
				totalBatches,
				(len(normalized)+maxItemIDsPerBatch-1)/maxItemIDsPerBatch,
				end-start,
				err,
			)
			continue
		}
		items = append(items, batchItems...)
	}

	if failedBatches > 0 {
		return items, &getItemsByIDsBatchError{
			TotalBatches:  totalBatches,
			FailedBatches: failedBatches,
			LastErr:       lastErr,
		}
	}

	return items, nil
}

func (s *EmbyService) getItemsByIDsBatch(batchIDs []string) ([]EmbyLibraryItem, error) {
	params := map[string]string{
		"Ids":    strings.Join(batchIDs, ","),
		"Fields": "ParentId,SeriesName,SeriesId,ParentIndexNumber,IndexNumber",
		"Limit":  strconv.Itoa(len(batchIDs)),
	}

	var lastErr error
	for attempt := 1; attempt <= getItemsByIDsMaxAttempts; attempt++ {
		ctx, cancel := context.WithTimeout(context.Background(), getItemsByIDsBatchTimeout)
		body, err := s.getWithAPIKeyAndContext(ctx, "/emby/Items", params)
		cancel()
		if err == nil {
			var out embyLibraryItemsResponse
			if err := json.Unmarshal(body, &out); err != nil {
				return nil, fmt.Errorf("解析 Emby 条目详情失败: %w", err)
			}
			return out.Items, nil
		}

		lastErr = err
		if attempt == getItemsByIDsMaxAttempts || !shouldRetryGetItemsByIDs(err) {
			break
		}

		backoff := time.Duration(attempt) * getItemsByIDsRetryBackoff
		log.Printf(
			"[Emby] GetItemsByIDs retry attempt=%d/%d size=%d backoff=%s err=%v",
			attempt+1,
			getItemsByIDsMaxAttempts,
			len(batchIDs),
			backoff,
			err,
		)
		if backoff > 0 {
			time.Sleep(backoff)
		}
	}

	return nil, fmt.Errorf("查询 Emby 条目详情失败 ids=%d: %w", len(batchIDs), lastErr)
}

func shouldRetryGetItemsByIDs(err error) bool {
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}

	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return true
	}

	var statusErr *embyHTTPStatusError
	if errors.As(err, &statusErr) {
		return statusErr.StatusCode == http.StatusTooManyRequests || statusErr.StatusCode >= http.StatusInternalServerError
	}

	return false
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
	return s.getWithAPIKeyAndContext(context.Background(), path, params)
}

func (s *EmbyService) getWithAPIKeyAndContext(ctx context.Context, path string, params map[string]string) ([]byte, error) {
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

	req, err := http.NewRequestWithContext(ctx, "GET", base.String(), nil)
	if err != nil {
		return nil, err
	}

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("无法连接到 Emby 服务器：%w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		return nil, &embyHTTPStatusError{
			StatusCode: resp.StatusCode,
			Body:       sanitizeEmbyErrorBody(body),
		}
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
