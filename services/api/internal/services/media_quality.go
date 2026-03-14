package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/konghang/ember/backend/internal/db"
	embyint "github.com/konghang/ember/backend/internal/integrations/emby"
	"github.com/konghang/ember/backend/internal/models"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	mediaQualityCacheTTL      = 6 * time.Hour
	mediaQualityAllLibraries  = "all"
	mediaQualityAllLibrariesK = "__all__"
)

type MediaQualityService struct {
	embyService *embyint.EmbyService
}

type ResolutionDistributionItem struct {
	Resolution string `json:"resolution"`
	Count      int    `json:"count"`
}

type CodecDistributionItem struct {
	Codec string `json:"codec"`
	Count int    `json:"count"`
}

type HDRDistributionItem struct {
	Type  string `json:"type"`
	Count int    `json:"count"`
}

type LowQualityItem struct {
	ID           string `json:"id"`
	GroupID      string `json:"groupId"`
	Name         string `json:"name"`
	ItemType     string `json:"itemType"` // Movie / Series
	ItemCount    int    `json:"itemCount"`
	PosterItemID string `json:"posterItemId"`
	Resolution   string `json:"resolution"` // 汇总后最差分辨率
	Codec        string `json:"codec"`      // 代表最差项的编码
	Bitrate      int    `json:"bitrate"`    // 代表最差项的最低码率（kbps）
}

type LowQualityDetailItem struct {
	ID           string `json:"id"`
	GroupID      string `json:"groupId"`
	GroupName    string `json:"groupName"`
	ItemType     string `json:"itemType"` // Movie / Episode
	Name         string `json:"name"`
	PosterItemID string `json:"posterItemId"`
	Resolution   string `json:"resolution"`
	Codec        string `json:"codec"`
	Bitrate      int    `json:"bitrate"` // kbps
	Season       int    `json:"season,omitempty"`
	Episode      int    `json:"episode,omitempty"`
}

type QualityReport struct {
	ResolutionDistribution []ResolutionDistributionItem `json:"resolutionDistribution"`
	CodecDistribution      []CodecDistributionItem      `json:"codecDistribution"`
	HDRDistribution        []HDRDistributionItem        `json:"hdrDistribution"`
	LowQualityItems        []LowQualityItem             `json:"lowQualityItems"`
	LowQualityDetails      []LowQualityDetailItem       `json:"lowQualityDetails,omitempty"`
	LowQualityTotal        int                          `json:"lowQualityTotal"`
	Page                   int                          `json:"page"`
	PageSize               int                          `json:"pageSize"`
	ScanAt                 time.Time                    `json:"scanAt"`
}

func NewMediaQualityService() *MediaQualityService {
	return &MediaQualityService{
		embyService: embyint.NewEmbyService(),
	}
}

func (s *MediaQualityService) GetLibraries() ([]embyint.EmbyLibrary, error) {
	return s.embyService.GetLibraries()
}

func (s *MediaQualityService) GetPoster(_ context.Context, itemID string, maxHeight int, quality int) ([]byte, string, error) {
	return s.embyService.GetItemPrimaryImage(itemID, maxHeight, quality)
}

func (s *MediaQualityService) GetLibraryQuality(ctx context.Context, libraryID string, force bool) (*QualityReport, error) {
	libraryID = strings.TrimSpace(libraryID)
	if libraryID == "" {
		return nil, ErrMediaQualityLibraryIDRequired
	}
	cacheKey := normalizeMediaQualityCacheKey(libraryID)

	if !force {
		cached, err := s.getCachedReport(ctx, cacheKey)
		if err != nil {
			return nil, err
		}
		if cached != nil && !shouldRefreshLegacyMediaQualityCache(cached) {
			return cached, nil
		}
	}

	return s.ScanLibraryQuality(ctx, libraryID)
}

func (s *MediaQualityService) ScanLibraryQuality(ctx context.Context, libraryID string) (*QualityReport, error) {
	libraryID = strings.TrimSpace(libraryID)
	if libraryID == "" {
		return nil, ErrMediaQualityLibraryIDRequired
	}
	cacheKey := normalizeMediaQualityCacheKey(libraryID)

	items, err := s.loadQualityItems(libraryID)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrMediaQualityScanFailed, err)
	}

	report := buildQualityReport(items)
	if err := s.saveReportCache(ctx, cacheKey, report); err != nil {
		return nil, err
	}

	return report, nil
}

func (s *MediaQualityService) GetGroupLowQualityDetails(
	ctx context.Context,
	libraryID string,
	groupID string,
	force bool,
) ([]LowQualityDetailItem, error) {
	groupID = strings.TrimSpace(groupID)
	if groupID == "" {
		return nil, ErrMediaQualityGroupIDRequired
	}

	report, err := s.GetLibraryQuality(ctx, libraryID, force)
	if err != nil {
		return nil, err
	}
	if report == nil || len(report.LowQualityDetails) == 0 {
		return []LowQualityDetailItem{}, nil
	}

	details := make([]LowQualityDetailItem, 0)
	for _, item := range report.LowQualityDetails {
		if matchesGroupID(groupID, item.GroupID) {
			details = append(details, item)
		}
	}

	sort.Slice(details, func(i, j int) bool {
		ri := resolutionSeverity(details[i].Resolution)
		rj := resolutionSeverity(details[j].Resolution)
		if ri != rj {
			return ri < rj
		}
		if details[i].Bitrate != details[j].Bitrate {
			if details[i].Bitrate == 0 {
				return false
			}
			if details[j].Bitrate == 0 {
				return true
			}
			return details[i].Bitrate < details[j].Bitrate
		}
		return details[i].Name < details[j].Name
	})

	return details, nil
}

func (s *MediaQualityService) loadQualityItems(libraryID string) ([]embyint.EmbyLibraryItem, error) {
	if !isAllLibrariesID(libraryID) {
		return s.embyService.GetLibraryItems(libraryID, 0)
	}

	libraries, err := s.embyService.GetLibraries()
	if err != nil {
		return nil, err
	}

	allItems := make([]embyint.EmbyLibraryItem, 0)
	for _, library := range libraries {
		id := strings.TrimSpace(library.ID)
		if id == "" {
			continue
		}
		items, err := s.embyService.GetLibraryItems(id, 0)
		if err != nil {
			return nil, err
		}
		allItems = append(allItems, items...)
	}

	return allItems, nil
}

func (s *MediaQualityService) getCachedReport(ctx context.Context, cacheKey string) (*QualityReport, error) {
	var cache models.MediaQualityCache
	err := db.DB.WithContext(ctx).
		Where("\"libraryId\" = ? AND \"expiresAt\" > ?", cacheKey, time.Now().UTC()).
		First(&cache).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, fmt.Errorf("查询媒体质量缓存失败: %w", err)
	}

	var report QualityReport
	if err := json.Unmarshal([]byte(cache.Statistics), &report); err != nil {
		return nil, fmt.Errorf("解析媒体质量缓存失败: %w", err)
	}

	return &report, nil
}

func (s *MediaQualityService) saveReportCache(ctx context.Context, cacheKey string, report *QualityReport) error {
	reportJSON, err := json.Marshal(report)
	if err != nil {
		return fmt.Errorf("序列化媒体质量报告失败: %w", err)
	}

	now := time.Now().UTC()
	cache := models.MediaQualityCache{
		LibraryID:  cacheKey,
		Statistics: string(reportJSON),
		ExpiresAt:  now.Add(mediaQualityCacheTTL),
	}

	if err := db.DB.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "libraryId"}},
		DoUpdates: clause.Assignments(map[string]interface{}{
			"statistics": cache.Statistics,
			"expiresAt":  cache.ExpiresAt,
			"updatedAt":  now,
		}),
	}).Create(&cache).Error; err != nil {
		return fmt.Errorf("保存媒体质量缓存失败: %w", err)
	}

	return nil
}

func buildQualityReport(items []embyint.EmbyLibraryItem) *QualityReport {
	resolutionCount := map[string]int{}
	codecCount := map[string]int{}
	hdrCount := map[string]int{}
	lowQualityGroups := map[string]LowQualityItem{}
	lowQualityDetails := make([]LowQualityDetailItem, 0)
	now := time.Now().UTC()

	for _, item := range items {
		stream, ok := pickVideoStream(item)
		if !ok {
			continue
		}

		resolution := detectResolution(stream.Width, stream.Height)
		codec := normalizeCodec(stream.Codec)
		hdrType := normalizeHDR(stream)
		bitrateKbps := normalizeBitrateKbps(stream.BitRate)

		resolutionCount[resolution]++
		codecCount[codec]++
		hdrCount[hdrType]++

		if isLowQuality(resolution, bitrateKbps) {
			groupKey, base := buildLowQualityGroup(item)
			detail := buildLowQualityDetail(item, groupKey, base.Name, base.PosterItemID, resolution, codec, bitrateKbps)
			lowQualityDetails = append(lowQualityDetails, detail)

			current, exists := lowQualityGroups[groupKey]
			if !exists {
				base.Resolution = resolution
				base.Codec = codec
				base.Bitrate = bitrateKbps
				base.ItemCount = 1
				lowQualityGroups[groupKey] = base
				continue
			}

			current.ItemCount++
			if isWorseResolution(resolution, current.Resolution) {
				current.Resolution = resolution
				current.Codec = codec
				current.Bitrate = bitrateKbps
			} else {
				current.Bitrate = pickLowerKnownBitrate(current.Bitrate, bitrateKbps)
			}
			lowQualityGroups[groupKey] = current
		}
	}

	lowQualityItems := make([]LowQualityItem, 0, len(lowQualityGroups))
	for _, item := range lowQualityGroups {
		lowQualityItems = append(lowQualityItems, item)
	}

	sort.Slice(lowQualityItems, func(i, j int) bool {
		ri := resolutionSeverity(lowQualityItems[i].Resolution)
		rj := resolutionSeverity(lowQualityItems[j].Resolution)
		if ri != rj {
			return ri < rj
		}
		if lowQualityItems[i].Bitrate != lowQualityItems[j].Bitrate {
			if lowQualityItems[i].Bitrate == 0 {
				return false
			}
			if lowQualityItems[j].Bitrate == 0 {
				return true
			}
			return lowQualityItems[i].Bitrate < lowQualityItems[j].Bitrate
		}
		if lowQualityItems[i].ItemCount != lowQualityItems[j].ItemCount {
			return lowQualityItems[i].ItemCount > lowQualityItems[j].ItemCount
		}
		if lowQualityItems[i].ItemType != lowQualityItems[j].ItemType {
			return lowQualityItems[i].ItemType < lowQualityItems[j].ItemType
		}
		if lowQualityItems[i].Name == lowQualityItems[j].Name {
			return lowQualityItems[i].ID < lowQualityItems[j].ID
		}
		return lowQualityItems[i].Name < lowQualityItems[j].Name
	})

	return &QualityReport{
		ResolutionDistribution: toResolutionDistribution(resolutionCount),
		CodecDistribution:      toCodecDistribution(codecCount),
		HDRDistribution:        toHDRDistribution(hdrCount),
		LowQualityItems:        lowQualityItems,
		LowQualityDetails:      lowQualityDetails,
		LowQualityTotal:        len(lowQualityItems),
		Page:                   1,
		PageSize:               len(lowQualityItems),
		ScanAt:                 now,
	}
}

func normalizeMediaQualityCacheKey(libraryID string) string {
	if isAllLibrariesID(libraryID) {
		return mediaQualityAllLibrariesK
	}
	return libraryID
}

func isAllLibrariesID(libraryID string) bool {
	return strings.EqualFold(strings.TrimSpace(libraryID), mediaQualityAllLibraries)
}

func buildLowQualityGroup(item embyint.EmbyLibraryItem) (string, LowQualityItem) {
	name := strings.TrimSpace(item.Name)
	id := strings.TrimSpace(item.ID)

	if isEpisodeLikeItem(item) {
		seriesName := strings.TrimSpace(item.SeriesName)
		if seriesName == "" {
			seriesName = "未知剧集"
		}

		seriesID := strings.TrimSpace(item.SeriesID)
		groupID := ""
		displayID := ""

		if seriesID != "" {
			groupID = "series:" + seriesID
			displayID = seriesID
		} else {
			seriesNameKey := normalizeGroupKeyFragment(seriesName)
			if seriesNameKey != "" {
				groupID = "series_name:" + seriesNameKey
				displayID = "series_name:" + seriesNameKey
			}
		}
		if groupID == "" {
			parentID := strings.TrimSpace(item.ParentID)
			if parentID != "" {
				groupID = "series_parent:" + parentID
				displayID = "series_parent:" + parentID
			} else {
				groupID = "series_unknown"
				displayID = "series_unknown"
			}
		}

		return groupID, LowQualityItem{
			ID:           displayID,
			GroupID:      groupID,
			Name:         seriesName,
			ItemType:     "Series",
			PosterItemID: pickFirstNonEmpty(seriesID, strings.TrimSpace(item.ParentID), id, displayID),
		}
	}

	if id == "" {
		id = "item:" + name
	}
	groupID := "movie:" + id

	return groupID, LowQualityItem{
		ID:           id,
		GroupID:      groupID,
		Name:         name,
		ItemType:     "Movie",
		PosterItemID: id,
	}
}

func buildLowQualityDetail(
	item embyint.EmbyLibraryItem,
	groupID string,
	groupName string,
	posterItemID string,
	resolution string,
	codec string,
	bitrate int,
) LowQualityDetailItem {
	itemType := strings.TrimSpace(item.Type)
	name := strings.TrimSpace(item.Name)
	if strings.EqualFold(itemType, "Episode") {
		name = formatEpisodeDisplayName(item)
	}

	return LowQualityDetailItem{
		ID:           strings.TrimSpace(item.ID),
		GroupID:      groupID,
		GroupName:    groupName,
		ItemType:     itemType,
		Name:         name,
		PosterItemID: posterItemID,
		Resolution:   resolution,
		Codec:        codec,
		Bitrate:      bitrate,
		Season:       item.ParentIndexNumber,
		Episode:      item.IndexNumber,
	}
}

func formatEpisodeDisplayName(item embyint.EmbyLibraryItem) string {
	seriesName := strings.TrimSpace(item.SeriesName)
	if seriesName == "" {
		seriesName = "未知剧集"
	}
	episodeName := strings.TrimSpace(item.Name)

	season := item.ParentIndexNumber
	episode := item.IndexNumber
	if season > 0 && episode > 0 {
		if episodeName != "" {
			return fmt.Sprintf("%s S%02dE%02d - %s", seriesName, season, episode, episodeName)
		}
		return fmt.Sprintf("%s S%02dE%02d", seriesName, season, episode)
	}

	if episodeName != "" {
		return fmt.Sprintf("%s - %s", seriesName, episodeName)
	}
	return seriesName
}

func resolutionSeverity(resolution string) int {
	switch resolution {
	case "SD":
		return 1
	case "720p":
		return 2
	case "1080p":
		return 3
	case "4K":
		return 4
	default:
		return 5
	}
}

func isWorseResolution(current, existing string) bool {
	return resolutionSeverity(current) < resolutionSeverity(existing)
}

func pickLowerKnownBitrate(existing, current int) int {
	if existing <= 0 {
		return current
	}
	if current <= 0 {
		return existing
	}
	if current < existing {
		return current
	}
	return existing
}

func pickFirstNonEmpty(values ...string) string {
	for _, value := range values {
		v := strings.TrimSpace(value)
		if v != "" {
			return v
		}
	}
	return ""
}

func isEpisodeLikeItem(item embyint.EmbyLibraryItem) bool {
	if strings.EqualFold(strings.TrimSpace(item.Type), "Episode") {
		return true
	}
	return strings.TrimSpace(item.SeriesID) != "" || strings.TrimSpace(item.SeriesName) != ""
}

func normalizeGroupKeyFragment(raw string) string {
	normalized := strings.ToLower(strings.TrimSpace(raw))
	if normalized == "" {
		return ""
	}
	normalized = strings.Join(strings.Fields(normalized), " ")
	replacer := strings.NewReplacer(":", "_", "/", "_", "\\", "_")
	return replacer.Replace(normalized)
}

func matchesGroupID(requested, actual string) bool {
	req := strings.TrimSpace(requested)
	act := strings.TrimSpace(actual)
	if req == "" || act == "" {
		return false
	}
	if req == act {
		return true
	}

	reqRaw := req
	if idx := strings.Index(reqRaw, ":"); idx >= 0 && idx+1 < len(reqRaw) {
		reqRaw = reqRaw[idx+1:]
	}
	actRaw := act
	if idx := strings.Index(actRaw, ":"); idx >= 0 && idx+1 < len(actRaw) {
		actRaw = actRaw[idx+1:]
	}
	return reqRaw == actRaw
}

func shouldRefreshLegacyMediaQualityCache(report *QualityReport) bool {
	if report == nil {
		return false
	}
	for _, item := range report.LowQualityItems {
		if strings.TrimSpace(item.GroupID) == "" {
			return true
		}
	}
	return false
}

func pickVideoStream(item embyint.EmbyLibraryItem) (embyint.EmbyMediaStream, bool) {
	for _, stream := range item.MediaStreams {
		if strings.EqualFold(strings.TrimSpace(stream.Type), "Video") {
			return stream, true
		}
	}
	for _, source := range item.MediaSources {
		for _, stream := range source.MediaStreams {
			if strings.EqualFold(strings.TrimSpace(stream.Type), "Video") {
				return stream, true
			}
		}
	}
	return embyint.EmbyMediaStream{}, false
}

func detectResolution(width, height int) string {
	switch {
	case width >= 3840 || height >= 2160:
		return "4K"
	case width >= 1920 || height >= 1080:
		return "1080p"
	case width >= 1280 || height >= 720:
		return "720p"
	case width > 0 || height > 0:
		return "SD"
	default:
		return "Unknown"
	}
}

func normalizeCodec(codec string) string {
	c := strings.ToUpper(strings.TrimSpace(codec))
	switch c {
	case "":
		return "Unknown"
	case "H264", "AVC":
		return "H.264"
	case "H265", "HEVC":
		return "HEVC"
	case "MPEG2VIDEO":
		return "MPEG2"
	default:
		return c
	}
}

func normalizeHDR(stream embyint.EmbyMediaStream) string {
	v := strings.ToUpper(strings.TrimSpace(stream.VideoRangeType))
	if v == "" {
		v = strings.ToUpper(strings.TrimSpace(stream.VideoRange))
	}
	if v == "" || v == "SDR" {
		return "SDR"
	}
	if strings.Contains(v, "DOVI") || strings.Contains(v, "DOLBY") {
		return "Dolby Vision"
	}
	if strings.Contains(v, "HDR10+") {
		return "HDR10+"
	}
	if strings.Contains(v, "HDR10") {
		return "HDR10"
	}
	if strings.Contains(v, "HLG") {
		return "HLG"
	}
	return v
}

func normalizeBitrateKbps(raw int) int {
	if raw <= 0 {
		return 0
	}
	if raw >= 10000 {
		return raw / 1000
	}
	return raw
}

func isLowQuality(resolution string, bitrateKbps int) bool {
	if resolution == "SD" || resolution == "720p" {
		return true
	}
	if bitrateKbps > 0 && bitrateKbps < 2500 {
		return true
	}
	return false
}

func toResolutionDistribution(counter map[string]int) []ResolutionDistributionItem {
	order := map[string]int{
		"4K":      1,
		"1080p":   2,
		"720p":    3,
		"SD":      4,
		"Unknown": 5,
	}
	items := make([]ResolutionDistributionItem, 0, len(counter))
	for key, count := range counter {
		items = append(items, ResolutionDistributionItem{Resolution: key, Count: count})
	}
	sort.Slice(items, func(i, j int) bool {
		oi, okI := order[items[i].Resolution]
		oj, okJ := order[items[j].Resolution]
		if okI && okJ && oi != oj {
			return oi < oj
		}
		if items[i].Count != items[j].Count {
			return items[i].Count > items[j].Count
		}
		return items[i].Resolution < items[j].Resolution
	})
	return items
}

func toCodecDistribution(counter map[string]int) []CodecDistributionItem {
	items := make([]CodecDistributionItem, 0, len(counter))
	for key, count := range counter {
		items = append(items, CodecDistributionItem{Codec: key, Count: count})
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].Count != items[j].Count {
			return items[i].Count > items[j].Count
		}
		return items[i].Codec < items[j].Codec
	})
	return items
}

func toHDRDistribution(counter map[string]int) []HDRDistributionItem {
	items := make([]HDRDistributionItem, 0, len(counter))
	for key, count := range counter {
		items = append(items, HDRDistributionItem{Type: key, Count: count})
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].Count != items[j].Count {
			return items[i].Count > items[j].Count
		}
		return items[i].Type < items[j].Type
	})
	return items
}
