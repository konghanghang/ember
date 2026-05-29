package policy

import (
	"context"
	"errors"
	"fmt"
	"log"
	"sort"
	"strings"
	"time"

	embyint "github.com/konghang/ember/backend/internal/integrations/emby"
	"github.com/konghang/ember/backend/internal/models"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	SyncStatusPending       = "pending"
	SyncStatusProcessing    = "processing"
	SyncStatusSynced        = "synced"
	SyncStatusFailed        = "failed"
	SyncStatusPartialFailed = "partial_failed"
)

var (
	ErrLibraryClientUnavailable = errors.New("Emby 媒体库客户端不可用")
	ErrLibraryIDInvalid         = errors.New("媒体库不在当前 Emby 媒体库列表中")
	ErrLibraryOutsideTemplate   = errors.New("只能选择当前分组模板内的媒体库")
	ErrActivePolicySyncTask     = errors.New("该用户或分组存在未完成的 Emby Policy 同步任务，请等待同步完成")
	ErrUserEmbyNotBound         = errors.New("用户未绑定 Emby 账号")
	ErrTelegramNotBound         = errors.New("请求参数错误")
	ErrPolicyTemplateInvalid    = errors.New("Emby 权益模板参数错误")
	ErrBatchNotFound            = errors.New("同步批次不存在")
)

type libraryClient interface {
	GetLibraries() ([]embyint.EmbyLibrary, error)
}

type MediaLibraryOption struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Type      string `json:"type"`
	ItemCount int    `json:"itemCount,omitempty"`
}

type PlanGroupMediaLibrarySettings struct {
	PlanGroupKey      string               `json:"planGroupKey"`
	PlanGroupName     string               `json:"planGroupName"`
	Libraries         []MediaLibraryOption `json:"libraries"`
	LibraryCount      int                  `json:"libraryCount"`
	AffectedUserCount int                  `json:"affectedUserCount"`
}

type PlanGroupEmbyPolicyTemplateResponse struct {
	PlanGroupKey                   string `json:"planGroupKey"`
	PlanGroupName                  string `json:"planGroupName"`
	SimultaneousStreamLimit        int    `json:"simultaneousStreamLimit"`
	EnableContentDownloading       bool   `json:"enableContentDownloading"`
	EnableLiveTvAccess             bool   `json:"enableLiveTvAccess"`
	EnableSyncTranscoding          bool   `json:"enableSyncTranscoding"`
	EnableAudioPlaybackTranscoding bool   `json:"enableAudioPlaybackTranscoding"`
	EnableVideoPlaybackTranscoding bool   `json:"enableVideoPlaybackTranscoding"`
	EnablePlaybackRemuxing         bool   `json:"enablePlaybackRemuxing"`
	EnableRemoteAccess             bool   `json:"enableRemoteAccess"`
	AffectedUserCount              int    `json:"affectedUserCount"`
}

type PlanGroupEmbyPolicyTemplateUpdateRequest struct {
	SimultaneousStreamLimit        int  `json:"simultaneousStreamLimit"`
	EnableContentDownloading       bool `json:"enableContentDownloading"`
	EnableLiveTvAccess             bool `json:"enableLiveTvAccess"`
	EnableSyncTranscoding          bool `json:"enableSyncTranscoding"`
	EnableAudioPlaybackTranscoding bool `json:"enableAudioPlaybackTranscoding"`
	EnableVideoPlaybackTranscoding bool `json:"enableVideoPlaybackTranscoding"`
	EnablePlaybackRemuxing         bool `json:"enablePlaybackRemuxing"`
	EnableRemoteAccess             bool `json:"enableRemoteAccess"`
}

type EmbyPolicySyncBatchCreated struct {
	BatchID           string `json:"batchId"`
	PlanGroupKey      string `json:"planGroupKey,omitempty"`
	Reason            string `json:"reason,omitempty"`
	Status            string `json:"status"`
	AffectedUserCount int    `json:"affectedUserCount"`
}

type EmbyPolicySyncFailedUser struct {
	UserID   string `json:"userId"`
	Username string `json:"username,omitempty"`
	EmbyID   string `json:"embyId,omitempty"`
	Error    string `json:"error"`
}

type EmbyPolicySyncBatchDetail struct {
	ID              string                     `json:"id"`
	PlanGroupKey    string                     `json:"planGroupKey,omitempty"`
	Reason          string                     `json:"reason"`
	Status          string                     `json:"status"`
	TotalCount      int                        `json:"totalCount"`
	PendingCount    int                        `json:"pendingCount"`
	ProcessingCount int                        `json:"processingCount"`
	SyncedCount     int                        `json:"syncedCount"`
	FailedCount     int                        `json:"failedCount"`
	FailedUsers     []EmbyPolicySyncFailedUser `json:"failedUsers,omitempty"`
	CreatedAt       time.Time                  `json:"createdAt"`
	UpdatedAt       time.Time                  `json:"updatedAt"`
	FinishedAt      *time.Time                 `json:"finishedAt,omitempty"`
}

type UserMediaLibraryItem struct {
	ID              string `json:"id"`
	Name            string `json:"name"`
	Type            string `json:"type"`
	ItemCount       int    `json:"itemCount,omitempty"`
	InGroupTemplate bool   `json:"inGroupTemplate"`
	Enabled         bool   `json:"enabled"`
}

type UserMediaLibrarySettings struct {
	UserID            string                 `json:"userId"`
	EmbyID            string                 `json:"embyId,omitempty"`
	PlanGroup         string                 `json:"planGroup"`
	PlanGroupName     string                 `json:"planGroupName"`
	Customized        bool                   `json:"customized"`
	Libraries         []UserMediaLibraryItem `json:"libraries"`
	TemplateCount     int                    `json:"templateCount"`
	EnabledCount      int                    `json:"enabledCount"`
	PolicySyncStatus  string                 `json:"policySyncStatus"`
	PendingSyncTaskID string                 `json:"pendingSyncTaskId,omitempty"`
}

type MediaLibrarySyncPreviewResult struct {
	PlanGroupKey    string                           `json:"planGroupKey"`
	TotalUsers      int                              `json:"totalUsers"`
	ScannedUsers    int                              `json:"scannedUsers"`
	Consistent      bool                             `json:"consistent"`
	Candidates      []MediaLibrarySyncCandidate      `json:"candidates"`
	DifferenceUsers []MediaLibrarySyncDifferenceUser `json:"differenceUsers"`
	FailedItems     []MediaLibrarySyncFailedItem     `json:"failedItems"`
}

type MediaLibrarySyncCandidate struct {
	LibraryIDs    []string             `json:"libraryIds"`
	Libraries     []MediaLibraryOption `json:"libraries"`
	UserCount     int                  `json:"userCount"`
	SourceUserIDs []string             `json:"sourceUserIds"`
}

type MediaLibrarySyncDifferenceUser struct {
	UserID     string               `json:"userId"`
	Username   string               `json:"username"`
	EmbyID     string               `json:"embyId"`
	LibraryIDs []string             `json:"libraryIds"`
	Libraries  []MediaLibraryOption `json:"libraries"`
}

type MediaLibrarySyncFailedItem struct {
	UserID   string `json:"userId,omitempty"`
	Username string `json:"username,omitempty"`
	EmbyID   string `json:"embyId,omitempty"`
	Error    string `json:"error"`
}

type MediaLibrarySyncApplyRequest struct {
	LibraryIDs        []string `json:"libraryIds"`
	PreferenceUserIDs []string `json:"preferenceUserIds,omitempty"`
}

type MediaLibrarySyncApplyResult struct {
	BatchID           string                       `json:"batchId"`
	AffectedUserCount int                          `json:"affectedUserCount"`
	Status            string                       `json:"status"`
	FailedItems       []MediaLibrarySyncFailedItem `json:"failedItems,omitempty"`
}

func (s *Service) GetAdminMediaLibraries() ([]MediaLibraryOption, error) {
	client, ok := s.embyClient.(libraryClient)
	if !ok || client == nil {
		return nil, ErrLibraryClientUnavailable
	}
	libraries, err := client.GetLibraries()
	if err != nil {
		return nil, normalizePolicyError("读取 Emby 媒体库失败", err)
	}
	return toLibraryOptions(libraries), nil
}

func (s *Service) GetPlanGroupMediaLibraries(key string) (*PlanGroupMediaLibrarySettings, error) {
	group, err := getPlanGroupByKey(s.db, key)
	if err != nil {
		return nil, err
	}
	libraries, err := s.loadGroupLibraryOptions(group.Key)
	if err != nil {
		return nil, err
	}
	count, err := s.countAffectedUsers(group.Key)
	if err != nil {
		return nil, err
	}
	return &PlanGroupMediaLibrarySettings{
		PlanGroupKey:      group.Key,
		PlanGroupName:     group.Name,
		Libraries:         libraries,
		LibraryCount:      len(libraries),
		AffectedUserCount: count,
	}, nil
}

func (s *Service) UpdatePlanGroupMediaLibraries(key string, libraryIDs []string, createdBy *string) (*EmbyPolicySyncBatchCreated, error) {
	group, err := getPlanGroupByKey(s.db, key)
	if err != nil {
		return nil, err
	}
	if err := s.ensureNoActiveGroupTasks(group.Key); err != nil {
		return nil, err
	}
	options, err := s.GetAdminMediaLibraries()
	if err != nil {
		return nil, err
	}
	optionByID := make(map[string]MediaLibraryOption, len(options))
	for _, option := range options {
		optionByID[option.ID] = option
	}
	normalizedIDs, err := normalizeLibraryIDs(libraryIDs)
	if err != nil {
		return nil, err
	}
	for _, id := range normalizedIDs {
		if _, ok := optionByID[id]; !ok {
			return nil, ErrLibraryIDInvalid
		}
	}

	tx := s.db.Begin()
	if tx.Error != nil {
		return nil, tx.Error
	}
	if err := tx.Where("plan_group_key = ?", group.Key).Delete(&models.PlanGroupMediaLibrary{}).Error; err != nil {
		tx.Rollback()
		return nil, err
	}
	for i, id := range normalizedIDs {
		option := optionByID[id]
		record := models.PlanGroupMediaLibrary{
			PlanGroupKey: group.Key,
			LibraryID:    option.ID,
			LibraryName:  option.Name,
			LibraryType:  option.Type,
			SortOrder:    i,
		}
		if err := tx.Create(&record).Error; err != nil {
			tx.Rollback()
			return nil, err
		}
	}
	batch, err := s.createBatchWithTasks(tx, group.Key, "plan_group_media_libraries_update", createdBy)
	if err != nil {
		tx.Rollback()
		return nil, err
	}
	if err := tx.Commit().Error; err != nil {
		return nil, err
	}
	return s.buildBatchCreated(batch.ID)
}

func (s *Service) GetPlanGroupEmbyPolicyTemplate(key string) (*PlanGroupEmbyPolicyTemplateResponse, error) {
	group, err := getPlanGroupByKey(s.db, key)
	if err != nil {
		return nil, err
	}
	template, err := s.loadPolicyTemplate(group.Key)
	if err != nil {
		return nil, err
	}
	count, err := s.countAffectedUsers(group.Key)
	if err != nil {
		return nil, err
	}
	return buildPolicyTemplateResponse(group, template, count), nil
}

func (s *Service) UpdatePlanGroupEmbyPolicyTemplate(key string, req PlanGroupEmbyPolicyTemplateUpdateRequest, createdBy *string) (*EmbyPolicySyncBatchCreated, error) {
	if req.SimultaneousStreamLimit < 0 || req.SimultaneousStreamLimit > 100 {
		return nil, ErrPolicyTemplateInvalid
	}
	group, err := getPlanGroupByKey(s.db, key)
	if err != nil {
		return nil, err
	}
	if err := s.ensureNoActiveGroupTasks(group.Key); err != nil {
		return nil, err
	}
	tx := s.db.Begin()
	if tx.Error != nil {
		return nil, tx.Error
	}
	template := models.PlanGroupEmbyPolicyTemplate{
		PlanGroupKey:                   group.Key,
		SimultaneousStreamLimit:        req.SimultaneousStreamLimit,
		EnableContentDownloading:       req.EnableContentDownloading,
		EnableLiveTvAccess:             req.EnableLiveTvAccess,
		EnableSyncTranscoding:          req.EnableSyncTranscoding,
		EnableAudioPlaybackTranscoding: req.EnableAudioPlaybackTranscoding,
		EnableVideoPlaybackTranscoding: req.EnableVideoPlaybackTranscoding,
		EnablePlaybackRemuxing:         req.EnablePlaybackRemuxing,
		EnableRemoteAccess:             req.EnableRemoteAccess,
	}
	if err := tx.Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "plan_group_key"}},
		DoUpdates: clause.Assignments(map[string]any{
			"simultaneous_stream_limit":         template.SimultaneousStreamLimit,
			"enable_content_downloading":        template.EnableContentDownloading,
			"enable_live_tv_access":             template.EnableLiveTvAccess,
			"enable_sync_transcoding":           template.EnableSyncTranscoding,
			"enable_audio_playback_transcoding": template.EnableAudioPlaybackTranscoding,
			"enable_video_playback_transcoding": template.EnableVideoPlaybackTranscoding,
			"enable_playback_remuxing":          template.EnablePlaybackRemuxing,
			"enable_remote_access":              template.EnableRemoteAccess,
			"updated_at":                        time.Now().UTC(),
		}),
	}).Create(&template).Error; err != nil {
		tx.Rollback()
		return nil, err
	}
	batch, err := s.createBatchWithTasks(tx, group.Key, "plan_group_emby_policy_template_update", createdBy)
	if err != nil {
		tx.Rollback()
		return nil, err
	}
	if err := tx.Commit().Error; err != nil {
		return nil, err
	}
	return s.buildBatchCreated(batch.ID)
}

func (s *Service) GetUserMediaLibrarySettings(userID string) (*UserMediaLibrarySettings, error) {
	user, group, libraries, preferences, err := s.loadUserLibraryContext(userID)
	if err != nil {
		return nil, err
	}
	return s.buildUserMediaLibrarySettings(user, group, libraries, preferences)
}

func (s *Service) SaveUserMediaLibraryPreferences(userID string, enabledLibraryIDs []string) (*UserMediaLibrarySettings, error) {
	user, group, libraries, _, err := s.loadUserLibraryContext(userID)
	if err != nil {
		return nil, err
	}
	if user.EmbyID == "" {
		return nil, ErrUserEmbyNotBound
	}
	if err := s.ensureNoActiveUserTasks(user.ID); err != nil {
		return nil, err
	}
	enabledSet, err := normalizeEnabledSet(enabledLibraryIDs, libraries)
	if err != nil {
		return nil, err
	}
	if err := s.replaceUserPreferences(user.ID, libraries, enabledSet); err != nil {
		return nil, err
	}
	if err := s.applyPolicyOrRecordFailure(user, group.Key, "user_media_library_preferences_update"); err != nil {
		log.Printf("[Policy] 用户媒体库偏好已保存但 Emby 同步失败: userID=%s err=%v", user.ID, err)
	}
	return s.GetUserMediaLibrarySettings(user.ID)
}

func (s *Service) ResetUserMediaLibraryPreferences(userID string) (*UserMediaLibrarySettings, error) {
	user, group, _, _, err := s.loadUserLibraryContext(userID)
	if err != nil {
		return nil, err
	}
	if user.EmbyID == "" {
		return nil, ErrUserEmbyNotBound
	}
	if err := s.ensureNoActiveUserTasks(user.ID); err != nil {
		return nil, err
	}
	if err := s.db.Where("user_id = ?", user.ID).Delete(&models.UserMediaLibraryPreference{}).Error; err != nil {
		return nil, err
	}
	if err := s.applyPolicyOrRecordFailure(user, group.Key, "user_media_library_preferences_reset"); err != nil {
		log.Printf("[Policy] 用户媒体库偏好已清除但 Emby 同步失败: userID=%s err=%v", user.ID, err)
	}
	return s.GetUserMediaLibrarySettings(user.ID)
}

// SyncUserMediaLibraryPreferencesFromEmby 将用户当前 Emby Policy 中的媒体库集合保存为个人偏好。
// 只在用户所属分组模板范围内写入 preferences，不能借由 Emby 当前状态扩大分组权限。
func (s *Service) SyncUserMediaLibraryPreferencesFromEmby(userID string) (*UserMediaLibrarySettings, error) {
	user, group, libraries, _, err := s.loadUserLibraryContext(userID)
	if err != nil {
		return nil, err
	}
	if user.EmbyID == "" {
		return nil, ErrUserEmbyNotBound
	}
	if err := s.ensureNoActiveUserTasks(user.ID); err != nil {
		return nil, err
	}
	libraryIDs, err := s.readCurrentUserPolicyLibraryIDs(user.EmbyID, planGroupLibraryIDs(libraries))
	if err != nil {
		return nil, err
	}
	if err := s.replaceUserPreferences(user.ID, libraries, idSet(libraryIDs)); err != nil {
		return nil, err
	}
	if err := s.applyPolicyOrRecordFailure(user, group.Key, "admin_user_media_library_preferences_sync_from_emby"); err != nil {
		log.Printf("[Policy] 已从 Emby 同步用户媒体库偏好但重算 Policy 失败: userID=%s err=%v", user.ID, err)
	}
	return s.GetUserMediaLibrarySettings(user.ID)
}

func (s *Service) GetTelegramMediaLibrarySettings(telegramID int64) (*UserMediaLibrarySettings, error) {
	user, err := s.findUserByTelegramID(telegramID)
	if err != nil {
		return nil, err
	}
	return s.GetUserMediaLibrarySettings(user.ID)
}

func (s *Service) ToggleTelegramMediaLibrary(telegramID int64, libraryID string) (*UserMediaLibrarySettings, error) {
	user, group, libraries, preferences, err := s.loadTelegramLibraryContext(telegramID)
	if err != nil {
		return nil, err
	}
	if user.EmbyID == "" {
		return nil, ErrUserEmbyNotBound
	}
	if err := s.ensureNoActiveUserTasks(user.ID); err != nil {
		return nil, err
	}
	libraryID = strings.TrimSpace(libraryID)
	if libraryID == "" {
		return nil, ErrLibraryOutsideTemplate
	}
	groupSet := make(map[string]struct{}, len(libraries))
	for _, library := range libraries {
		groupSet[library.LibraryID] = struct{}{}
	}
	if _, ok := groupSet[libraryID]; !ok {
		return nil, ErrLibraryOutsideTemplate
	}
	enabledSet := enabledSetFromPreferences(libraries, preferences)
	if _, enabled := enabledSet[libraryID]; enabled {
		delete(enabledSet, libraryID)
	} else {
		enabledSet[libraryID] = struct{}{}
	}
	if err := s.replaceUserPreferences(user.ID, libraries, enabledSet); err != nil {
		return nil, err
	}
	if err := s.applyPolicyOrRecordFailure(user, group.Key, "telegram_media_library_toggle"); err != nil {
		log.Printf("[Policy] Telegram 媒体库偏好已保存但 Emby 同步失败: userID=%s err=%v", user.ID, err)
	}
	return s.GetUserMediaLibrarySettings(user.ID)
}

func (s *Service) ResetTelegramMediaLibraryPreferences(telegramID int64) (*UserMediaLibrarySettings, error) {
	user, err := s.findUserByTelegramID(telegramID)
	if err != nil {
		return nil, err
	}
	return s.ResetUserMediaLibraryPreferences(user.ID)
}

func (s *Service) UpdateUserEmbyAccess(userID string, disabled bool) error {
	var user models.User
	if err := s.db.Where("id = ?", userID).First(&user).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		return normalizePolicyError("读取用户失败", err)
	}
	if err := s.db.Model(&models.User{}).
		Where("id = ?", user.ID).
		Update("emby_access_disabled", disabled).Error; err != nil {
		return normalizePolicyError("更新 Emby 访问禁用意图失败", err)
	}
	if user.EmbyID == "" {
		return nil
	}
	planGroupKey, err := resolveEffectivePlanGroupKey(s.db, user.PlanGroup)
	if err != nil {
		return normalizePolicyError("解析用户有效分组失败", err)
	}
	return s.applyPolicyOrRecordFailure(&user, planGroupKey, "admin_emby_access_update")
}

// RetryUserPolicySyncFailure 由管理员手动重试用户当前有效 Emby Policy 同步。
// 失败任务保留为 failed 供后台识别；重试成功后由 ApplyEffectiveUserPolicy 统一收口旧失败任务。
func (s *Service) RetryUserPolicySyncFailure(userID string) error {
	var user models.User
	if err := s.db.Where("id = ?", userID).First(&user).Error; err != nil {
		return normalizePolicyError("读取用户失败", err)
	}
	if user.EmbyID == "" {
		return ErrUserEmbyNotBound
	}
	if err := s.ensureNoActiveUserTasks(user.ID); err != nil {
		return err
	}
	planGroupKey, err := resolveEffectivePlanGroupKey(s.db, user.PlanGroup)
	if err != nil {
		return normalizePolicyError("解析用户有效分组失败", err)
	}
	reason := "admin_user_policy_sync_retry"
	if err := s.ApplyEffectiveUserPolicy(user.ID, reason); err != nil {
		if recordErr := s.recordUserPolicySyncFailure(&user, planGroupKey, reason, err); recordErr != nil {
			return recordErr
		}
		return err
	}
	return nil
}

func (s *Service) GetEmbyPolicySyncBatch(id string) (*EmbyPolicySyncBatchDetail, error) {
	if err := s.refreshBatchStatus(id); err != nil {
		return nil, err
	}
	var batch models.EmbyPolicySyncBatch
	if err := s.db.Where("id = ?", id).First(&batch).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrBatchNotFound
		}
		return nil, err
	}
	detail := EmbyPolicySyncBatchDetail{
		ID:              batch.ID,
		PlanGroupKey:    batch.PlanGroupKey,
		Reason:          batch.Reason,
		Status:          batch.Status,
		TotalCount:      batch.TotalCount,
		PendingCount:    batch.PendingCount,
		ProcessingCount: batch.ProcessingCount,
		SyncedCount:     batch.SyncedCount,
		FailedCount:     batch.FailedCount,
		CreatedAt:       batch.CreatedAt,
		UpdatedAt:       batch.UpdatedAt,
		FinishedAt:      batch.FinishedAt,
	}
	var failed []struct {
		UserID    string
		Username  string
		EmbyID    string
		LastError string
	}
	if err := s.db.Table("emby_policy_sync_tasks AS tasks").
		Select(`tasks.user_id, users.username, tasks.emby_id, COALESCE(tasks.last_error, '') AS last_error`).
		Joins("LEFT JOIN users ON users.id = tasks.user_id").
		Where("tasks.batch_id = ? AND tasks.status = ?", id, SyncStatusFailed).
		Limit(20).
		Scan(&failed).Error; err != nil {
		return nil, err
	}
	for _, item := range failed {
		detail.FailedUsers = append(detail.FailedUsers, EmbyPolicySyncFailedUser{
			UserID:   item.UserID,
			Username: item.Username,
			EmbyID:   item.EmbyID,
			Error:    item.LastError,
		})
	}
	return &detail, nil
}

func (s *Service) RetryFailedEmbyPolicySyncBatch(id string) (*EmbyPolicySyncBatchCreated, error) {
	var tasks []models.EmbyPolicySyncTask
	if err := s.db.Where("batch_id = ? AND status = ?", id, SyncStatusFailed).Find(&tasks).Error; err != nil {
		return nil, err
	}
	if len(tasks) == 0 {
		return s.buildBatchCreated(id)
	}
	for _, task := range tasks {
		if err := s.ensureNoActiveUserTasks(task.UserID); err != nil {
			return nil, err
		}
	}
	if err := s.db.Model(&models.EmbyPolicySyncTask{}).
		Where("batch_id = ? AND status = ?", id, SyncStatusFailed).
		Updates(map[string]any{
			"status":        SyncStatusPending,
			"last_error":    nil,
			"next_retry_at": time.Now().UTC(),
			"updated_at":    time.Now().UTC(),
		}).Error; err != nil {
		return nil, err
	}
	return s.buildBatchCreated(id)
}

// BuildPlanGroupMediaLibrarySyncPreview 读取分组内历史用户当前 Emby Policy，聚合媒体库模板候选。
func (s *Service) BuildPlanGroupMediaLibrarySyncPreview(key string) (*MediaLibrarySyncPreviewResult, error) {
	group, err := getPlanGroupByKey(s.db, key)
	if err != nil {
		return nil, err
	}
	users, err := s.loadSyncPreviewUsers(group.Key)
	if err != nil {
		return nil, err
	}
	libraryOptions, err := s.GetAdminMediaLibraries()
	if err != nil {
		return nil, err
	}
	optionByID := mediaLibraryOptionMap(libraryOptions)
	allLibraryIDs := mediaLibraryOptionIDs(libraryOptions)
	result := &MediaLibrarySyncPreviewResult{
		PlanGroupKey: group.Key,
		TotalUsers:   len(users),
		Candidates:   []MediaLibrarySyncCandidate{},
		FailedItems:  []MediaLibrarySyncFailedItem{},
	}
	candidates := map[string]*MediaLibrarySyncCandidate{}
	for _, user := range users {
		libraryIDs, err := s.readCurrentUserPolicyLibraryIDs(user.EmbyID, allLibraryIDs)
		if err != nil {
			result.FailedItems = append(result.FailedItems, MediaLibrarySyncFailedItem{
				UserID:   user.ID,
				Username: user.Username,
				EmbyID:   user.EmbyID,
				Error:    truncateError(err),
			})
			continue
		}
		result.ScannedUsers++
		key := strings.Join(libraryIDs, "\x00")
		candidate, ok := candidates[key]
		if !ok {
			candidate = &MediaLibrarySyncCandidate{
				LibraryIDs: libraryIDs,
				Libraries:  buildLibraryOptionsFromIDs(libraryIDs, optionByID),
			}
			candidates[key] = candidate
			result.Candidates = append(result.Candidates, *candidate)
		}
		candidate.UserCount++
		candidate.SourceUserIDs = append(candidate.SourceUserIDs, user.ID)
	}
	for i := range result.Candidates {
		if candidate, ok := candidates[strings.Join(result.Candidates[i].LibraryIDs, "\x00")]; ok {
			result.Candidates[i] = *candidate
		}
	}
	result.Consistent = len(result.Candidates) <= 1 && len(result.FailedItems) == 0
	if len(result.Candidates) > 1 {
		for _, candidate := range result.Candidates {
			for _, userID := range candidate.SourceUserIDs {
				user := findUserInPreviewUsers(users, userID)
				if user == nil {
					continue
				}
				result.DifferenceUsers = append(result.DifferenceUsers, MediaLibrarySyncDifferenceUser{
					UserID:     user.ID,
					Username:   user.Username,
					EmbyID:     user.EmbyID,
					LibraryIDs: candidate.LibraryIDs,
					Libraries:  candidate.Libraries,
				})
			}
		}
	}
	return result, nil
}

// ApplyPlanGroupMediaLibrarySync 将管理员确认的历史媒体库集合写入分组模板，并创建同步批次。
func (s *Service) ApplyPlanGroupMediaLibrarySync(key string, req MediaLibrarySyncApplyRequest, createdBy *string) (*MediaLibrarySyncApplyResult, error) {
	group, err := getPlanGroupByKey(s.db, key)
	if err != nil {
		return nil, err
	}
	if err := s.ensureNoActiveGroupTasks(group.Key); err != nil {
		return nil, err
	}
	options, err := s.GetAdminMediaLibraries()
	if err != nil {
		return nil, err
	}
	optionByID := mediaLibraryOptionMap(options)
	allLibraryIDs := mediaLibraryOptionIDs(options)
	normalizedIDs, err := normalizeLibraryIDs(req.LibraryIDs)
	if err != nil {
		return nil, err
	}
	for _, id := range normalizedIDs {
		if _, ok := optionByID[id]; !ok {
			return nil, ErrLibraryIDInvalid
		}
	}
	preferenceUsers, failedItems, err := s.loadPreferenceSyncUsers(req.PreferenceUserIDs, group.Key, allLibraryIDs)
	if err != nil {
		return nil, err
	}

	tx := s.db.Begin()
	if tx.Error != nil {
		return nil, tx.Error
	}
	if err := tx.Where("plan_group_key = ?", group.Key).Delete(&models.PlanGroupMediaLibrary{}).Error; err != nil {
		tx.Rollback()
		return nil, err
	}
	libraries := make([]models.PlanGroupMediaLibrary, 0, len(normalizedIDs))
	for i, id := range normalizedIDs {
		option := optionByID[id]
		record := models.PlanGroupMediaLibrary{
			PlanGroupKey: group.Key,
			LibraryID:    option.ID,
			LibraryName:  option.Name,
			LibraryType:  option.Type,
			SortOrder:    i,
		}
		if err := tx.Create(&record).Error; err != nil {
			tx.Rollback()
			return nil, err
		}
		libraries = append(libraries, record)
	}
	for _, item := range preferenceUsers {
		if err := replaceUserPreferencesTx(tx, item.user.ID, libraries, idSet(item.libraryIDs)); err != nil {
			tx.Rollback()
			return nil, err
		}
	}
	batch, err := s.createBatchWithTasks(tx, group.Key, "plan_group_media_libraries_history_sync", createdBy)
	if err != nil {
		tx.Rollback()
		return nil, err
	}
	if err := tx.Commit().Error; err != nil {
		return nil, err
	}
	created, err := s.buildBatchCreated(batch.ID)
	if err != nil {
		return nil, err
	}
	return &MediaLibrarySyncApplyResult{
		BatchID:           created.BatchID,
		AffectedUserCount: created.AffectedUserCount,
		Status:            created.Status,
		FailedItems:       failedItems,
	}, nil
}

type policyPreferenceSyncUser struct {
	user       models.User
	libraryIDs []string
}

func (s *Service) loadSyncPreviewUsers(planGroupKey string) ([]models.User, error) {
	var users []models.User
	if err := usersInPlanGroupQuery(s.db.Model(&models.User{}), planGroupKey).
		Where("COALESCE(emby_id, '') <> ''").
		Order("username ASC, id ASC").
		Find(&users).Error; err != nil {
		return nil, err
	}
	return users, nil
}

func (s *Service) loadPreferenceSyncUsers(userIDs []string, planGroupKey string, allLibraryIDs []string) ([]policyPreferenceSyncUser, []MediaLibrarySyncFailedItem, error) {
	normalizedUserIDs := normalizeUserIDs(userIDs)
	if len(normalizedUserIDs) == 0 {
		return nil, nil, nil
	}
	var users []models.User
	if err := usersInPlanGroupQuery(s.db.Model(&models.User{}), planGroupKey).
		Where("id IN ? AND COALESCE(emby_id, '') <> ''", normalizedUserIDs).
		Find(&users).Error; err != nil {
		return nil, nil, err
	}
	userByID := make(map[string]models.User, len(users))
	for _, user := range users {
		userByID[user.ID] = user
	}
	out := make([]policyPreferenceSyncUser, 0, len(users))
	failed := []MediaLibrarySyncFailedItem{}
	for _, userID := range normalizedUserIDs {
		user, ok := userByID[userID]
		if !ok {
			failed = append(failed, MediaLibrarySyncFailedItem{
				UserID: userID,
				Error:  "用户不在当前分组或未绑定 Emby",
			})
			continue
		}
		libraryIDs, err := s.readCurrentUserPolicyLibraryIDs(user.EmbyID, allLibraryIDs)
		if err != nil {
			failed = append(failed, MediaLibrarySyncFailedItem{
				UserID:   user.ID,
				Username: user.Username,
				EmbyID:   user.EmbyID,
				Error:    truncateError(err),
			})
			continue
		}
		out = append(out, policyPreferenceSyncUser{user: user, libraryIDs: libraryIDs})
	}
	return out, failed, nil
}

func (s *Service) readCurrentUserPolicyLibraryIDs(embyID string, allLibraryIDs []string) ([]string, error) {
	if s.embyClient == nil {
		return nil, errors.New("Policy 服务未配置 Emby 客户端")
	}
	rawPolicy, err := s.embyClient.GetUserPolicyRaw(embyID)
	if err != nil {
		return nil, normalizePolicyError("读取 Emby Policy 失败", err)
	}
	if boolPolicyValue(rawPolicy["EnableAllFolders"]) {
		return normalizePolicyLibraryIDs(allLibraryIDs)
	}
	return normalizePolicyLibraryIDs(stringSlicePolicyValue(rawPolicy["EnabledFolders"]))
}

func normalizePolicyLibraryIDs(libraryIDs []string) ([]string, error) {
	normalized, err := normalizeLibraryIDs(libraryIDs)
	if err != nil {
		return nil, err
	}
	sort.Strings(normalized)
	return normalized, nil
}

func boolPolicyValue(value any) bool {
	got, ok := value.(bool)
	return ok && got
}

func stringSlicePolicyValue(value any) []string {
	switch typed := value.(type) {
	case []string:
		return typed
	case []any:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			if text, ok := item.(string); ok {
				out = append(out, text)
			}
		}
		return out
	default:
		return nil
	}
}

func mediaLibraryOptionMap(options []MediaLibraryOption) map[string]MediaLibraryOption {
	out := make(map[string]MediaLibraryOption, len(options))
	for _, option := range options {
		out[option.ID] = option
	}
	return out
}

func mediaLibraryOptionIDs(options []MediaLibraryOption) []string {
	out := make([]string, 0, len(options))
	for _, option := range options {
		out = append(out, option.ID)
	}
	return out
}

func buildLibraryOptionsFromIDs(ids []string, optionByID map[string]MediaLibraryOption) []MediaLibraryOption {
	out := make([]MediaLibraryOption, 0, len(ids))
	for _, id := range ids {
		if option, ok := optionByID[id]; ok {
			out = append(out, option)
			continue
		}
		out = append(out, MediaLibraryOption{ID: id, Name: id})
	}
	return out
}

func findUserInPreviewUsers(users []models.User, userID string) *models.User {
	for i := range users {
		if users[i].ID == userID {
			return &users[i]
		}
	}
	return nil
}

func normalizeUserIDs(userIDs []string) []string {
	seen := make(map[string]struct{}, len(userIDs))
	out := make([]string, 0, len(userIDs))
	for _, raw := range userIDs {
		id := strings.TrimSpace(raw)
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out
}

func idSet(ids []string) map[string]struct{} {
	out := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		out[id] = struct{}{}
	}
	return out
}

func planGroupLibraryIDs(libraries []models.PlanGroupMediaLibrary) []string {
	out := make([]string, 0, len(libraries))
	for _, library := range libraries {
		out = append(out, library.LibraryID)
	}
	return out
}

func (s *Service) loadGroupLibraryOptions(planGroupKey string) ([]MediaLibraryOption, error) {
	var records []models.PlanGroupMediaLibrary
	if err := s.db.Where("plan_group_key = ?", planGroupKey).
		Order("sort_order ASC, library_name ASC, library_id ASC").
		Find(&records).Error; err != nil {
		return nil, err
	}
	options := make([]MediaLibraryOption, 0, len(records))
	for _, record := range records {
		options = append(options, MediaLibraryOption{
			ID:   record.LibraryID,
			Name: record.LibraryName,
			Type: record.LibraryType,
		})
	}
	return options, nil
}

func (s *Service) countAffectedUsers(planGroupKey string) (int, error) {
	var count int64
	if err := usersInPlanGroupQuery(s.db.Model(&models.User{}), planGroupKey).
		Where("COALESCE(emby_id, '') <> ''").
		Count(&count).Error; err != nil {
		return 0, err
	}
	return int(count), nil
}

func (s *Service) createBatchWithTasks(tx *gorm.DB, planGroupKey, reason string, createdBy *string) (*models.EmbyPolicySyncBatch, error) {
	var users []models.User
	if err := usersInPlanGroupQuery(tx.Model(&models.User{}), planGroupKey).
		Where("COALESCE(emby_id, '') <> ''").
		Find(&users).Error; err != nil {
		return nil, err
	}
	batch := models.EmbyPolicySyncBatch{
		PlanGroupKey: planGroupKey,
		Reason:       reason,
		Status:       SyncStatusPending,
		TotalCount:   len(users),
		PendingCount: len(users),
		CreatedBy:    createdBy,
	}
	if len(users) == 0 {
		now := time.Now().UTC()
		batch.Status = SyncStatusSynced
		batch.FinishedAt = &now
	}
	if err := tx.Create(&batch).Error; err != nil {
		return nil, err
	}
	for _, user := range users {
		task := models.EmbyPolicySyncTask{
			BatchID:      &batch.ID,
			UserID:       user.ID,
			EmbyID:       user.EmbyID,
			PlanGroupKey: planGroupKey,
			Reason:       reason,
			Status:       SyncStatusPending,
			NextRetryAt:  ptrTime(time.Now().UTC()),
		}
		if err := tx.Create(&task).Error; err != nil {
			return nil, err
		}
	}
	return &batch, nil
}

// buildBatchCreated 只刷新并返回批次创建结果，不在 HTTP 请求内执行同步任务。
func (s *Service) buildBatchCreated(batchID string) (*EmbyPolicySyncBatchCreated, error) {
	if err := s.refreshBatchStatus(batchID); err != nil {
		return nil, err
	}
	var batch models.EmbyPolicySyncBatch
	if err := s.db.Where("id = ?", batchID).First(&batch).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrBatchNotFound
		}
		return nil, err
	}
	return &EmbyPolicySyncBatchCreated{
		BatchID:           batch.ID,
		PlanGroupKey:      batch.PlanGroupKey,
		Reason:            batch.Reason,
		Status:            batch.Status,
		AffectedUserCount: batch.TotalCount,
	}, nil
}

func (s *Service) refreshBatchStatus(batchID string) error {
	var batch models.EmbyPolicySyncBatch
	if err := s.db.Where("id = ?", batchID).First(&batch).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrBatchNotFound
		}
		return err
	}
	var tasks []models.EmbyPolicySyncTask
	if err := s.db.Where("batch_id = ?", batch.ID).Find(&tasks).Error; err != nil {
		return err
	}
	counts := map[string]int{}
	for _, task := range tasks {
		counts[task.Status]++
	}
	total := len(tasks)
	now := time.Now().UTC()
	status, finishedAt := resolveBatchStatus(total, counts[SyncStatusPending], counts[SyncStatusProcessing], counts[SyncStatusSynced], counts[SyncStatusFailed], batch.FinishedAt, now)
	return s.db.Model(&models.EmbyPolicySyncBatch{}).
		Where("id = ?", batch.ID).
		Updates(map[string]any{
			"status":           status,
			"total_count":      total,
			"pending_count":    counts[SyncStatusPending],
			"processing_count": counts[SyncStatusProcessing],
			"synced_count":     counts[SyncStatusSynced],
			"failed_count":     counts[SyncStatusFailed],
			"finished_at":      finishedAt,
			"updated_at":       time.Now().UTC(),
		}).Error
}

func (s *Service) loadUserLibraryContext(userID string) (*models.User, *models.PlanGroup, []models.PlanGroupMediaLibrary, []models.UserMediaLibraryPreference, error) {
	var user models.User
	if err := s.db.Where("id = ?", userID).First(&user).Error; err != nil {
		return nil, nil, nil, nil, err
	}
	planGroupKey, err := resolveEffectivePlanGroupKey(s.db, user.PlanGroup)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	group, err := getPlanGroupByKey(s.db, planGroupKey)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	var libraries []models.PlanGroupMediaLibrary
	if err := s.db.Where("plan_group_key = ?", group.Key).
		Order("sort_order ASC, library_name ASC, library_id ASC").
		Find(&libraries).Error; err != nil {
		return nil, nil, nil, nil, err
	}
	var preferences []models.UserMediaLibraryPreference
	if err := s.db.Where("user_id = ?", user.ID).Find(&preferences).Error; err != nil {
		return nil, nil, nil, nil, err
	}
	return &user, group, libraries, preferences, nil
}

func (s *Service) loadTelegramLibraryContext(telegramID int64) (*models.User, *models.PlanGroup, []models.PlanGroupMediaLibrary, []models.UserMediaLibraryPreference, error) {
	user, err := s.findUserByTelegramID(telegramID)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	return s.loadUserLibraryContext(user.ID)
}

func (s *Service) buildUserMediaLibrarySettings(
	user *models.User,
	group *models.PlanGroup,
	libraries []models.PlanGroupMediaLibrary,
	preferences []models.UserMediaLibraryPreference,
) (*UserMediaLibrarySettings, error) {
	enabledSet := enabledSetFromPreferences(libraries, preferences)
	customized := len(preferences) > 0
	items := make([]UserMediaLibraryItem, 0, len(libraries))
	enabledCount := 0
	for _, library := range libraries {
		_, enabled := enabledSet[library.LibraryID]
		if enabled {
			enabledCount++
		}
		items = append(items, UserMediaLibraryItem{
			ID:              library.LibraryID,
			Name:            library.LibraryName,
			Type:            library.LibraryType,
			InGroupTemplate: true,
			Enabled:         enabled,
		})
	}
	status, taskID, err := s.userPolicySyncStatus(user.ID)
	if err != nil {
		return nil, err
	}
	return &UserMediaLibrarySettings{
		UserID:            user.ID,
		EmbyID:            user.EmbyID,
		PlanGroup:         group.Key,
		PlanGroupName:     group.Name,
		Customized:        customized,
		Libraries:         items,
		TemplateCount:     len(libraries),
		EnabledCount:      enabledCount,
		PolicySyncStatus:  status,
		PendingSyncTaskID: taskID,
	}, nil
}

func (s *Service) findUserByTelegramID(telegramID int64) (*models.User, error) {
	var user models.User
	if err := s.db.Where("telegram_id = ?", telegramID).First(&user).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrTelegramNotBound
		}
		return nil, err
	}
	return &user, nil
}

func (s *Service) ensureNoActiveGroupTasks(planGroupKey string) error {
	if _, _, err := s.recoverStalePolicySyncTasks(context.Background(), defaultPolicySyncProcessingTTL); err != nil {
		return err
	}
	var count int64
	if err := s.db.Model(&models.EmbyPolicySyncTask{}).
		Where("plan_group_key = ? AND status IN ?", planGroupKey, []string{SyncStatusPending, SyncStatusProcessing}).
		Count(&count).Error; err != nil {
		return err
	}
	if count > 0 {
		return ErrActivePolicySyncTask
	}
	return nil
}

func (s *Service) ensureNoActiveUserTasks(userID string) error {
	if _, _, err := s.recoverStalePolicySyncTasks(context.Background(), defaultPolicySyncProcessingTTL); err != nil {
		return err
	}
	var count int64
	if err := s.db.Model(&models.EmbyPolicySyncTask{}).
		Where("user_id = ? AND status IN ?", userID, []string{SyncStatusPending, SyncStatusProcessing}).
		Count(&count).Error; err != nil {
		return err
	}
	if count > 0 {
		return ErrActivePolicySyncTask
	}
	return nil
}

func (s *Service) replaceUserPreferences(userID string, libraries []models.PlanGroupMediaLibrary, enabledSet map[string]struct{}) error {
	tx := s.db.Begin()
	if tx.Error != nil {
		return tx.Error
	}
	if err := replaceUserPreferencesTx(tx, userID, libraries, enabledSet); err != nil {
		tx.Rollback()
		return err
	}
	return tx.Commit().Error
}

func replaceUserPreferencesTx(tx *gorm.DB, userID string, libraries []models.PlanGroupMediaLibrary, enabledSet map[string]struct{}) error {
	if err := tx.Where("user_id = ?", userID).Delete(&models.UserMediaLibraryPreference{}).Error; err != nil {
		return err
	}
	for _, library := range libraries {
		_, enabled := enabledSet[library.LibraryID]
		preference := models.UserMediaLibraryPreference{
			UserID:    userID,
			LibraryID: library.LibraryID,
			Enabled:   enabled,
		}
		if err := tx.Create(&preference).Error; err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) applyPolicyOrRecordFailure(user *models.User, planGroupKey, reason string) error {
	if err := s.ApplyEffectiveUserPolicy(user.ID, reason); err != nil {
		if recordErr := s.recordUserPolicySyncFailure(user, planGroupKey, reason, err); recordErr != nil {
			return recordErr
		}
		return err
	}
	return nil
}

// recordUserPolicySyncFailure 记录单用户同步失败终态，供管理员在后台人工重试或处理。
func (s *Service) recordUserPolicySyncFailure(user *models.User, planGroupKey, reason string, cause error) error {
	task := buildUserPolicySyncFailureTask(user, planGroupKey, reason, cause)
	update := s.db.Model(&models.EmbyPolicySyncTask{}).
		Where("user_id = ? AND batch_id IS NULL AND status = ?", task.UserID, SyncStatusFailed).
		Updates(map[string]any{
			"emby_id":        task.EmbyID,
			"plan_group_key": task.PlanGroupKey,
			"reason":         task.Reason,
			"attempts":       gorm.Expr("attempts + ?", 1),
			"last_error":     task.LastError,
			"updated_at":     time.Now().UTC(),
		})
	if update.Error != nil {
		return fmt.Errorf("%w；更新同步失败任务失败：%v", cause, update.Error)
	}
	if update.RowsAffected > 0 {
		return nil
	}
	if createErr := s.db.Create(&task).Error; createErr != nil {
		return fmt.Errorf("%w；记录同步失败任务失败：%v", cause, createErr)
	}
	return nil
}

// buildUserPolicySyncFailureTask 构造人工处理语义的 failed 同步任务。
func buildUserPolicySyncFailureTask(user *models.User, planGroupKey, reason string, cause error) models.EmbyPolicySyncTask {
	msg := truncateError(cause)
	task := models.EmbyPolicySyncTask{
		UserID:       user.ID,
		EmbyID:       user.EmbyID,
		PlanGroupKey: planGroupKey,
		Reason:       strings.TrimSpace(reason),
		Status:       SyncStatusFailed,
		Attempts:     1,
		LastError:    &msg,
	}
	if task.Reason == "" {
		task.Reason = "unspecified"
	}
	return task
}

// resolveFailedUserPolicySyncTasks 在一次完整 Policy 同步成功后关闭该用户历史失败状态。
func (s *Service) resolveFailedUserPolicySyncTasks(userID string) error {
	return s.db.Model(&models.EmbyPolicySyncTask{}).
		Where("user_id = ? AND batch_id IS NULL AND status = ?", userID, SyncStatusFailed).
		Updates(map[string]any{
			"status":     SyncStatusSynced,
			"last_error": nil,
			"updated_at": time.Now().UTC(),
		}).Error
}

func (s *Service) userPolicySyncStatus(userID string) (string, string, error) {
	var task models.EmbyPolicySyncTask
	if err := s.db.Where("user_id = ? AND status IN ?", userID, []string{SyncStatusPending, SyncStatusProcessing}).
		Order("updated_at DESC").
		First(&task).Error; err == nil {
		return task.Status, task.ID, nil
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return "", "", err
	}
	if err := s.db.Where("user_id = ? AND batch_id IS NULL AND status = ?", userID, SyncStatusFailed).
		Order("updated_at DESC").
		First(&task).Error; err == nil {
		return SyncStatusFailed, task.ID, nil
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return "", "", err
	}
	return SyncStatusSynced, "", nil
}

func usersInPlanGroupQuery(query *gorm.DB, planGroupKey string) *gorm.DB {
	return query.Where(`plan_group = ?`, planGroupKey)
}

func buildPolicyTemplateResponse(group *models.PlanGroup, template models.PlanGroupEmbyPolicyTemplate, count int) *PlanGroupEmbyPolicyTemplateResponse {
	return &PlanGroupEmbyPolicyTemplateResponse{
		PlanGroupKey:                   group.Key,
		PlanGroupName:                  group.Name,
		SimultaneousStreamLimit:        template.SimultaneousStreamLimit,
		EnableContentDownloading:       template.EnableContentDownloading,
		EnableLiveTvAccess:             template.EnableLiveTvAccess,
		EnableSyncTranscoding:          template.EnableSyncTranscoding,
		EnableAudioPlaybackTranscoding: template.EnableAudioPlaybackTranscoding,
		EnableVideoPlaybackTranscoding: template.EnableVideoPlaybackTranscoding,
		EnablePlaybackRemuxing:         template.EnablePlaybackRemuxing,
		EnableRemoteAccess:             template.EnableRemoteAccess,
		AffectedUserCount:              count,
	}
}

func toLibraryOptions(libraries []embyint.EmbyLibrary) []MediaLibraryOption {
	options := make([]MediaLibraryOption, 0, len(libraries))
	for _, library := range libraries {
		id := strings.TrimSpace(library.ID)
		if id == "" {
			continue
		}
		options = append(options, MediaLibraryOption{
			ID:        id,
			Name:      strings.TrimSpace(library.Name),
			Type:      strings.TrimSpace(library.Type),
			ItemCount: library.ItemCount,
		})
	}
	sort.SliceStable(options, func(i, j int) bool {
		return strings.ToLower(options[i].Name) < strings.ToLower(options[j].Name)
	})
	return options
}

func normalizeLibraryIDs(libraryIDs []string) ([]string, error) {
	seen := make(map[string]struct{}, len(libraryIDs))
	out := make([]string, 0, len(libraryIDs))
	for _, raw := range libraryIDs {
		id := strings.TrimSpace(raw)
		if id == "" {
			continue
		}
		if _, exists := seen[id]; exists {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out, nil
}

func normalizeEnabledSet(enabledLibraryIDs []string, libraries []models.PlanGroupMediaLibrary) (map[string]struct{}, error) {
	groupSet := make(map[string]struct{}, len(libraries))
	for _, library := range libraries {
		groupSet[library.LibraryID] = struct{}{}
	}
	enabledSet := make(map[string]struct{}, len(enabledLibraryIDs))
	for _, raw := range enabledLibraryIDs {
		id := strings.TrimSpace(raw)
		if id == "" {
			continue
		}
		if _, ok := groupSet[id]; !ok {
			return nil, ErrLibraryOutsideTemplate
		}
		enabledSet[id] = struct{}{}
	}
	return enabledSet, nil
}

func enabledSetFromPreferences(libraries []models.PlanGroupMediaLibrary, preferences []models.UserMediaLibraryPreference) map[string]struct{} {
	enabledSet := make(map[string]struct{}, len(libraries))
	if len(preferences) == 0 {
		for _, library := range libraries {
			enabledSet[library.LibraryID] = struct{}{}
		}
		return enabledSet
	}
	for _, preference := range preferences {
		if preference.Enabled {
			enabledSet[preference.LibraryID] = struct{}{}
		}
	}
	return enabledSet
}

func ptrTime(value time.Time) *time.Time {
	return &value
}

func truncateError(err error) string {
	if err == nil {
		return ""
	}
	msg := err.Error()
	if len(msg) > 500 {
		return msg[:500]
	}
	return msg
}
