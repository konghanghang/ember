package subscription

import (
	"encoding/json"
	"fmt"
	"log"
	"net/url"
	"slices"
	"strconv"
	"strings"

	embyint "github.com/konghang/ember/backend/internal/integrations/emby"
	"github.com/konghang/ember/backend/internal/models"
)

type embyExistingItemsResponse struct {
	Items            []embyExistingItem `json:"Items"`
	TotalRecordCount int                `json:"TotalRecordCount"`
}

type embyExistingItem struct {
	ID                string            `json:"Id"`
	Name              string            `json:"Name"`
	Type              string            `json:"Type"`
	SeriesID          string            `json:"SeriesId"`
	ParentIndexNumber int               `json:"ParentIndexNumber"`
	IndexNumber       int               `json:"IndexNumber"`
	LocationType      string            `json:"LocationType"`
	Path              string            `json:"Path"`
	IsMissing         bool              `json:"IsMissing"`
	ProviderIDs       map[string]string `json:"ProviderIds"`
	MediaSources      []interface{}     `json:"MediaSources"`
}

func (s *SubscriptionService) CheckExisting(req CheckExistingRequest) (*CheckExistingResponse, error) {
	season, err := normalizeSubscriptionSeason(req.Type, req.Season)
	if err != nil {
		return nil, err
	}

	embyService := embyint.GetSharedService()
	if !embyService.IsConfigured() {
		log.Printf("[Subscription] 跳过库内检测：Emby 未配置 type=%s tmdbId=%s season=%d", req.Type, strings.TrimSpace(req.TmdbID), season)
		return detectionFailedResponse("库内检测暂时不可用，确认后仍可继续提交。"), nil
	}

	resp, err := checkExistingInLibrary(embyService, CheckExistingRequest{
		Type:   req.Type,
		TmdbID: req.TmdbID,
		Season: season,
	})
	if err != nil {
		if req.Type == models.MediaMovie {
			log.Printf("[Subscription] 电影库内检测失败 tmdbId=%s err=%v", strings.TrimSpace(req.TmdbID), err)
		} else {
			log.Printf("[Subscription] 剧集库内检测失败 tmdbId=%s season=%d err=%v", strings.TrimSpace(req.TmdbID), season, err)
		}
		return detectionFailedResponse("库内检测失败，确认后仍可继续提交。"), nil
	}

	return resp, nil
}

func checkExistingInLibrary(embyService *embyint.EmbyService, req CheckExistingRequest) (*CheckExistingResponse, error) {
	resp := &CheckExistingResponse{}

	if req.Type == models.MediaMovie {
		item, err := findProviderMatchedItem(embyService, req.TmdbID, "Movie")
		if err != nil {
			return nil, err
		}
		if item == nil {
			return resp, nil
		}
		resp.ExistsInLibrary = true
		resp.ExistingSummary = &ExistingSummary{
			MatchType:  ExistingMatchMovie,
			EmbyItemID: strings.TrimSpace(item.ID),
			Message:    "Emby 库中已存在该电影，确认后仍可继续提交。",
		}
		return resp, nil
	}

	seriesItem, err := findProviderMatchedItem(embyService, req.TmdbID, "Series")
	if err != nil {
		return nil, err
	}
	if seriesItem == nil {
		return resp, nil
	}

	episodes, err := getSeriesEpisodes(embyService, seriesItem.ID)
	if err != nil {
		return nil, fmt.Errorf("拉取 Emby 剧集失败: %w", err)
	}

	availableSeasons, episodeCountBySeason := summarizeEpisodeInventory(episodes)
	if req.Season <= 0 {
		totalEpisodes := totalEpisodeCount(episodeCountBySeason)
		if totalEpisodes == 0 {
			return resp, nil
		}
		resp.ExistsInLibrary = true
		resp.ExistingSummary = &ExistingSummary{
			MatchType:        ExistingMatchSeries,
			EmbyItemID:       strings.TrimSpace(seriesItem.ID),
			Message:          buildSeriesExistsMessage(availableSeasons, totalEpisodes),
			AvailableSeasons: availableSeasons,
			EpisodeCount:     totalEpisodes,
		}
		return resp, nil
	}

	if episodeCountBySeason[req.Season] == 0 {
		return resp, nil
	}

	resp.ExistsInLibrary = true
	resp.ExistingSummary = &ExistingSummary{
		MatchType:        ExistingMatchSeason,
		EmbyItemID:       strings.TrimSpace(seriesItem.ID),
		Message:          fmt.Sprintf("Emby 库中已存在第 %d 季内容（已入库 %d 集），确认后仍可继续提交。", req.Season, episodeCountBySeason[req.Season]),
		AvailableSeasons: availableSeasons,
		EpisodeCount:     episodeCountBySeason[req.Season],
	}
	return resp, nil
}

func detectionFailedResponse(message string) *CheckExistingResponse {
	return &CheckExistingResponse{
		DetectionFailed: true,
		ExistingSummary: &ExistingSummary{
			MatchType:       ExistingMatchUnknown,
			Message:         message,
			DetectionFailed: true,
		},
	}
}

func findProviderMatchedItem(embyService *embyint.EmbyService, tmdbID string, itemType string) (*embyExistingItem, error) {
	params := map[string]string{
		"Recursive":           "true",
		"IncludeItemTypes":    itemType,
		"Fields":              "ProviderIds,Path,LocationType,MediaSources",
		"AnyProviderIdEquals": "Tmdb." + strings.TrimSpace(tmdbID),
		"Limit":               "20",
	}

	body, err := embyService.GetWithAPIKey("/emby/Items", params)
	if err != nil {
		return nil, err
	}

	var resp embyExistingItemsResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("解析 Emby 检测响应失败: %w", err)
	}

	trimmedTmdbID := strings.TrimSpace(tmdbID)
	for _, item := range resp.Items {
		if !hasPhysicalExistingItem(item) {
			continue
		}
		if extractProviderID(item.ProviderIDs, "Tmdb") == trimmedTmdbID {
			copyItem := item
			return &copyItem, nil
		}
	}
	return nil, nil
}

func getSeriesEpisodes(embyService *embyint.EmbyService, seriesID string) ([]embyExistingItem, error) {
	episodes := make([]embyExistingItem, 0, 256)
	startIndex := 0
	limit := 500

	for {
		body, err := embyService.GetWithAPIKey("/emby/Shows/"+url.PathEscape(strings.TrimSpace(seriesID))+"/Episodes", map[string]string{
			"Fields":     "Path,LocationType,MediaSources,ParentIndexNumber,IndexNumber",
			"Limit":      strconv.Itoa(limit),
			"StartIndex": strconv.Itoa(startIndex),
		})
		if err != nil {
			return nil, err
		}

		var resp embyExistingItemsResponse
		if err := json.Unmarshal(body, &resp); err != nil {
			return nil, fmt.Errorf("解析 Emby 剧集响应失败: %w", err)
		}
		if len(resp.Items) == 0 {
			break
		}

		episodes = append(episodes, resp.Items...)
		startIndex += len(resp.Items)

		if resp.TotalRecordCount > 0 && startIndex >= resp.TotalRecordCount {
			break
		}
		if len(resp.Items) < limit {
			break
		}
	}

	return episodes, nil
}

func summarizeEpisodeInventory(items []embyExistingItem) ([]int, map[int]int) {
	availableSeasons := make([]int, 0)
	episodeCountBySeason := make(map[int]int)

	for _, item := range items {
		if !hasPhysicalExistingItem(item) {
			continue
		}
		if item.ParentIndexNumber <= 0 || item.IndexNumber <= 0 {
			continue
		}
		if _, exists := episodeCountBySeason[item.ParentIndexNumber]; !exists {
			availableSeasons = append(availableSeasons, item.ParentIndexNumber)
		}
		episodeCountBySeason[item.ParentIndexNumber]++
	}

	slices.Sort(availableSeasons)
	return availableSeasons, episodeCountBySeason
}

func totalEpisodeCount(episodeCountBySeason map[int]int) int {
	total := 0
	for _, count := range episodeCountBySeason {
		total += count
	}
	return total
}

func buildSeriesExistsMessage(availableSeasons []int, episodeCount int) string {
	if len(availableSeasons) == 0 {
		return "Emby 库中已存在该剧条目，确认后仍可继续提交。"
	}

	parts := make([]string, 0, len(availableSeasons))
	for _, season := range availableSeasons {
		parts = append(parts, strconv.Itoa(season))
	}
	return fmt.Sprintf(
		"Emby 库中已存在该剧的部分或全部内容（已入库季：%s，共 %d 集），确认后仍可继续提交。",
		strings.Join(parts, "、"),
		episodeCount,
	)
}

func extractProviderID(providerIDs map[string]string, key string) string {
	if len(providerIDs) == 0 {
		return ""
	}
	for providerKey, providerValue := range providerIDs {
		if strings.EqualFold(strings.TrimSpace(providerKey), key) {
			return strings.TrimSpace(providerValue)
		}
	}
	return ""
}

func hasPhysicalExistingItem(item embyExistingItem) bool {
	if item.IsMissing {
		return false
	}
	if strings.EqualFold(strings.TrimSpace(item.LocationType), "Virtual") {
		return false
	}
	if strings.TrimSpace(item.Path) != "" {
		return true
	}
	return len(item.MediaSources) > 0
}
