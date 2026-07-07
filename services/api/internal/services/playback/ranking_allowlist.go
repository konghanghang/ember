package playback

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"sort"
	"strings"
	"time"

	"github.com/konghang/ember/backend/internal/db"
	embyint "github.com/konghang/ember/backend/internal/integrations/emby"
	"github.com/konghang/ember/backend/internal/models"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const playbackRankingLibraryAllowlistSettingKey = "playback_ranking_library_allowlist"

var ErrRankingLibraryIDInvalid = errors.New("排行榜媒体库不在当前 Emby 媒体库列表中")

type RankingLibraryOption struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Type      string `json:"type"`
	ItemCount int    `json:"itemCount,omitempty"`
}

type RankingLibraryAllowlistSettings struct {
	AllowAll          bool                   `json:"allowAll"`
	LibraryIDs        []string               `json:"libraryIds"`
	InvalidLibraryIDs []string               `json:"invalidLibraryIds,omitempty"`
	Libraries         []RankingLibraryOption `json:"libraries"`
}

type rankingLibraryFilter struct {
	allowAll          bool
	libraryIDs        []string
	allowedLibraryIDs map[string]struct{}
}

func (s *PlaybackRankingService) GetRankingLibraryAllowlist() (*RankingLibraryAllowlistSettings, error) {
	libraries, err := s.getRankingLibraries()
	if err != nil {
		return nil, err
	}

	storedIDs, err := s.currentRankingLibraryAllowlistIDs()
	if err != nil {
		return nil, err
	}

	validIDs, invalidIDs := partitionRankingLibraryIDs(storedIDs, libraryOptionMap(libraries))
	if len(storedIDs) > 0 && len(validIDs) == 0 {
		log.Printf("[PlaybackRanking] allowlist stored IDs are obsolete, clearing old config stored=%v", storedIDs)
		if s.saveLibraryAllowlist != nil {
			if clearErr := s.saveLibraryAllowlist([]string{}, nil); clearErr != nil {
				return nil, clearErr
			}
		}
		storedIDs = []string{}
		validIDs = []string{}
		invalidIDs = []string{}
	}
	log.Printf(
		"[PlaybackRanking] allowlist read stored=%v valid=%v invalid=%v libraries=%s",
		storedIDs,
		validIDs,
		invalidIDs,
		formatRankingLibraryOptionsForLog(libraries),
	)
	return &RankingLibraryAllowlistSettings{
		AllowAll:          len(storedIDs) == 0,
		LibraryIDs:        validIDs,
		InvalidLibraryIDs: invalidIDs,
		Libraries:         libraries,
	}, nil
}

func (s *PlaybackRankingService) UpdateRankingLibraryAllowlist(libraryIDs []string, updatedByUserID *string) (*RankingLibraryAllowlistSettings, error) {
	libraries, err := s.getRankingLibraries()
	if err != nil {
		return nil, err
	}

	normalizedIDs := normalizeRankingLibraryIDs(libraryIDs)
	if len(normalizedIDs) == len(libraries) && len(libraries) > 0 {
		normalizedIDs = []string{}
	}
	libraryByID := libraryOptionMap(libraries)
	for _, id := range normalizedIDs {
		if _, ok := libraryByID[id]; !ok {
			return nil, ErrRankingLibraryIDInvalid
		}
	}

	if s.saveLibraryAllowlist == nil {
		return nil, errors.New("排行榜媒体库配置保存器未初始化")
	}
	log.Printf(
		"[PlaybackRanking] allowlist update requested=%v normalized=%v libraries=%s",
		libraryIDs,
		normalizedIDs,
		formatRankingLibraryOptionsForLog(libraries),
	)
	if err := s.saveLibraryAllowlist(normalizedIDs, updatedByUserID); err != nil {
		return nil, err
	}

	return &RankingLibraryAllowlistSettings{
		AllowAll:   len(normalizedIDs) == 0,
		LibraryIDs: normalizedIDs,
		Libraries:  libraries,
	}, nil
}

func (s *PlaybackRankingService) loadRankingLibraryFilter() (rankingLibraryFilter, error) {
	ids, err := s.currentRankingLibraryAllowlistIDs()
	if err != nil {
		return rankingLibraryFilter{}, err
	}
	if len(ids) == 0 {
		return rankingLibraryFilter{allowAll: true}, nil
	}

	libraries, err := s.getRankingLibraries()
	if err != nil {
		return rankingLibraryFilter{}, err
	}

	validIDs, invalidIDs := partitionRankingLibraryIDs(ids, libraryOptionMap(libraries))
	if len(invalidIDs) > 0 {
		log.Printf("[PlaybackRanking] 排行榜媒体库 allowlist 包含失效库，将忽略这些库: ids=%s", strings.Join(invalidIDs, ","))
	}
	if len(ids) > 0 && len(validIDs) == 0 {
		log.Printf("[PlaybackRanking] allowlist filter stored IDs are obsolete, clearing old config stored=%v", ids)
		if s.saveLibraryAllowlist != nil {
			if clearErr := s.saveLibraryAllowlist([]string{}, nil); clearErr != nil {
				return rankingLibraryFilter{}, clearErr
			}
		}
		return rankingLibraryFilter{allowAll: true}, nil
	}
	log.Printf(
		"[PlaybackRanking] allowlist filter stored=%v valid=%v invalid=%v libraries=%s",
		ids,
		validIDs,
		invalidIDs,
		formatRankingLibraryOptionsForLog(libraries),
	)
	allowedLibraryIDs := make(map[string]struct{}, len(validIDs))
	for _, libraryID := range validIDs {
		allowedLibraryIDs[libraryID] = struct{}{}
	}

	return rankingLibraryFilter{
		allowAll:          false,
		libraryIDs:        validIDs,
		allowedLibraryIDs: allowedLibraryIDs,
	}, nil
}

func (s *PlaybackRankingService) currentRankingLibraryAllowlistIDs() ([]string, error) {
	if s == nil || s.loadLibraryAllowlist == nil {
		return []string{}, nil
	}
	return s.loadLibraryAllowlist()
}

func (s *PlaybackRankingService) getRankingLibraries() ([]RankingLibraryOption, error) {
	if s == nil || s.embyService == nil {
		return nil, errors.New("排行榜服务未配置 Emby 客户端")
	}
	load := func() ([]RankingLibraryOption, error) {
		libraries, err := s.embyService.GetAdminViews()
		if err != nil {
			return nil, fmt.Errorf("读取 Emby 排行榜媒体库视图失败：%w", err)
		}
		return toRankingLibraryOptions(libraries), nil
	}

	if s.rankingLibrariesCache == nil {
		return load()
	}

	return s.rankingLibrariesCache.Get("all", "ranking-libraries", load)
}

func loadPlaybackRankingLibraryAllowlist() ([]string, error) {
	var setting models.Setting
	if err := db.DB.Where("key = ?", playbackRankingLibraryAllowlistSettingKey).First(&setting).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return []string{}, nil
		}
		return nil, err
	}
	return parsePlaybackRankingLibraryAllowlist(setting.Value)
}

func savePlaybackRankingLibraryAllowlist(libraryIDs []string, updatedByUserID *string) error {
	normalizedIDs := normalizeRankingLibraryIDs(libraryIDs)
	if len(normalizedIDs) == 0 {
		return db.DB.Where("key = ?", playbackRankingLibraryAllowlistSettingKey).Delete(&models.Setting{}).Error
	}

	encoded, err := json.Marshal(normalizedIDs)
	if err != nil {
		return err
	}

	setting := models.Setting{
		Key:             playbackRankingLibraryAllowlistSettingKey,
		Value:           string(encoded),
		IsEncrypted:     false,
		UpdatedByUserID: updatedByUserID,
		UpdatedAt:       time.Now().UTC(),
	}

	return db.DB.Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "key"}},
		DoUpdates: clause.Assignments(map[string]any{
			"value":              setting.Value,
			"is_encrypted":       false,
			"updated_by_user_id": updatedByUserID,
			"updated_at":         setting.UpdatedAt,
		}),
	}).Create(&setting).Error
}

func parsePlaybackRankingLibraryAllowlist(raw string) ([]string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return []string{}, nil
	}

	var libraryIDs []string
	if err := json.Unmarshal([]byte(raw), &libraryIDs); err != nil {
		return nil, fmt.Errorf("排行榜媒体库 allowlist 配置无效：%w", err)
	}
	return normalizeRankingLibraryIDs(libraryIDs), nil
}

func normalizeRankingLibraryIDs(libraryIDs []string) []string {
	seen := make(map[string]struct{}, len(libraryIDs))
	normalized := make([]string, 0, len(libraryIDs))
	for _, raw := range libraryIDs {
		id := strings.TrimSpace(raw)
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		normalized = append(normalized, id)
	}
	sort.Strings(normalized)
	return normalized
}

func toRankingLibraryOptions(libraries []embyint.EmbyLibrary) []RankingLibraryOption {
	options := make([]RankingLibraryOption, 0, len(libraries))
	for _, library := range libraries {
		id := strings.TrimSpace(library.ID)
		if id == "" {
			continue
		}
		libraryType := strings.TrimSpace(library.Type)
		if strings.EqualFold(libraryType, "boxsets") {
			continue
		}
		options = append(options, RankingLibraryOption{
			ID:        id,
			Name:      strings.TrimSpace(library.Name),
			Type:      libraryType,
			ItemCount: library.ItemCount,
		})
	}
	sort.SliceStable(options, func(i, j int) bool {
		return strings.ToLower(options[i].Name) < strings.ToLower(options[j].Name)
	})
	return options
}

func libraryOptionMap(options []RankingLibraryOption) map[string]RankingLibraryOption {
	out := make(map[string]RankingLibraryOption, len(options))
	for _, option := range options {
		out[option.ID] = option
	}
	return out
}

func partitionRankingLibraryIDs(storedIDs []string, libraryByID map[string]RankingLibraryOption) ([]string, []string) {
	validIDs := make([]string, 0, len(storedIDs))
	invalidIDs := make([]string, 0)
	for _, id := range normalizeRankingLibraryIDs(storedIDs) {
		if _, ok := libraryByID[id]; ok {
			validIDs = append(validIDs, id)
			continue
		}
		invalidIDs = append(invalidIDs, id)
	}
	return validIDs, invalidIDs
}

func formatRankingLibraryOptionsForLog(options []RankingLibraryOption) string {
	if len(options) == 0 {
		return "[]"
	}
	parts := make([]string, 0, len(options))
	for _, option := range options {
		parts = append(parts, fmt.Sprintf("%s(%s)", option.ID, option.Name))
	}
	return "[" + strings.Join(parts, ", ") + "]"
}
