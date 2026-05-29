package policy

import (
	"errors"
	"fmt"
	"log"
	"strings"

	"github.com/konghang/ember/backend/internal/db"
	"github.com/konghang/ember/backend/internal/models"
	"gorm.io/gorm"
)

type embyPolicyClient interface {
	GetUserPolicyRaw(embyUserID string) (map[string]any, error)
	PatchUserPolicyFields(targetUserID string, sourcePolicy map[string]any, fields []string) error
}

// Service 在保留外部字段的前提下应用 Ember 托管的 Emby Policy 字段。
type Service struct {
	embyClient embyPolicyClient
	db         *gorm.DB
}

// NewService 使用当前数据库句柄创建有效 Policy 服务。
func NewService(embyClient embyPolicyClient) *Service {
	return NewServiceWithDB(db.DB, embyClient)
}

// NewServiceWithDB 为测试或限定事务创建有效 Policy 服务。
func NewServiceWithDB(database *gorm.DB, embyClient embyPolicyClient) *Service {
	return &Service{db: database, embyClient: embyClient}
}

// ApplyEffectiveUserPolicy 重算并写入某个用户所有 Ember 托管的 Emby Policy 字段。
func (s *Service) ApplyEffectiveUserPolicy(userID, reason string) error {
	if s == nil || s.db == nil {
		return errors.New("Policy 服务未配置数据库")
	}
	reason = strings.TrimSpace(reason)
	if reason == "" {
		reason = "unspecified"
	}

	var user models.User
	if err := s.db.Where("id = ?", userID).First(&user).Error; err != nil {
		return normalizePolicyError("读取用户失败", err)
	}
	if user.EmbyID == "" {
		log.Printf("[Policy] 跳过 Emby Policy 同步：用户未绑定 Emby userId=%s reason=%s", user.ID, reason)
		return nil
	}
	if s.embyClient == nil {
		return errors.New("Policy 服务未配置 Emby 客户端")
	}

	planGroupKey, err := resolveEffectivePlanGroupKey(s.db, user.PlanGroup)
	if err != nil {
		return normalizePolicyError("解析用户有效分组失败", err)
	}

	template, err := s.loadPolicyTemplate(planGroupKey)
	if err != nil {
		return err
	}
	libraryIDs, err := s.resolveEnabledLibraryIDs(user.ID, planGroupKey)
	if err != nil {
		return err
	}

	rawPolicy, err := s.embyClient.GetUserPolicyRaw(user.EmbyID)
	if err != nil {
		return normalizePolicyError("读取 Emby Policy 失败", err)
	}

	managedPolicy, fields := buildManagedPolicyFields(rawPolicy, user.IsExpired() || user.EmbyAccessDisabled, template, libraryIDs)
	log.Printf("[Policy] 应用用户有效 Emby Policy: userID=%s embyID=%s planGroup=%s reason=%s libraryCount=%d isDisabled=%t",
		user.ID, user.EmbyID, planGroupKey, reason, len(libraryIDs), managedPolicy["IsDisabled"])

	if err := s.embyClient.PatchUserPolicyFields(user.EmbyID, managedPolicy, fields); err != nil {
		return normalizePolicyError("写入 Emby Policy 失败", err)
	}

	if err := s.db.Model(&models.User{}).
		Where("id = ?", user.ID).
		Update("emby_disabled", managedPolicy["IsDisabled"]).Error; err != nil {
		return normalizePolicyError("更新本地 Emby 禁用缓存失败", err)
	}
	if err := s.resolveFailedUserPolicySyncTasks(user.ID); err != nil {
		return normalizePolicyError("收口用户 Emby Policy 失败任务失败", err)
	}

	return nil
}

// ApplyEffectiveUserPolicyOrRecordFailure 应用用户当前有效 Policy；失败时写入单用户 failed 处理记录。
func (s *Service) ApplyEffectiveUserPolicyOrRecordFailure(userID, reason string) error {
	if err := s.ApplyEffectiveUserPolicy(userID, reason); err != nil {
		if recordErr := s.RecordUserPolicySyncFailure(userID, reason, err); recordErr != nil {
			return fmt.Errorf("%w；记录同步失败任务失败：%v", err, recordErr)
		}
		return err
	}
	return nil
}

// RecordUserPolicySyncFailure 记录单用户 Emby Policy 同步失败，供管理员在后台人工重试。
func (s *Service) RecordUserPolicySyncFailure(userID, reason string, cause error) error {
	if s == nil || s.db == nil {
		return errors.New("Policy 服务未配置数据库")
	}
	var user models.User
	if err := s.db.Where("id = ?", userID).First(&user).Error; err != nil {
		return normalizePolicyError("读取用户失败", err)
	}
	if user.EmbyID == "" {
		log.Printf("[Policy] 跳过记录 Emby Policy 同步失败：用户未绑定 Emby userID=%s reason=%s", user.ID, strings.TrimSpace(reason))
		return nil
	}
	planGroupKey, err := resolveEffectivePlanGroupKey(s.db, user.PlanGroup)
	if err != nil {
		return normalizePolicyError("解析用户有效分组失败", err)
	}
	return s.recordUserPolicySyncFailure(&user, planGroupKey, reason, cause)
}

func (s *Service) loadPolicyTemplate(planGroupKey string) (models.PlanGroupEmbyPolicyTemplate, error) {
	var template models.PlanGroupEmbyPolicyTemplate
	if err := s.db.Where("plan_group_key = ?", planGroupKey).First(&template).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return template, fmt.Errorf("分组 %s 缺少 Emby 权益模板", planGroupKey)
		}
		return template, normalizePolicyError("读取分组 Emby 权益模板失败", err)
	}
	return template, nil
}

func (s *Service) resolveEnabledLibraryIDs(userID, planGroupKey string) ([]string, error) {
	var groupLibraries []models.PlanGroupMediaLibrary
	if err := s.db.
		Where("plan_group_key = ?", planGroupKey).
		Order("sort_order ASC, library_name ASC, library_id ASC").
		Find(&groupLibraries).Error; err != nil {
		return nil, normalizePolicyError("读取分组媒体库模板失败", err)
	}
	if len(groupLibraries) == 0 {
		log.Printf("[Policy] 分组媒体库模板为空：userID=%s planGroup=%s", userID, planGroupKey)
		return []string{}, nil
	}

	groupSet := make(map[string]struct{}, len(groupLibraries))
	groupOrder := make([]string, 0, len(groupLibraries))
	for _, library := range groupLibraries {
		groupSet[library.LibraryID] = struct{}{}
		groupOrder = append(groupOrder, library.LibraryID)
	}

	var preferences []models.UserMediaLibraryPreference
	if err := s.db.Where("user_id = ?", userID).Find(&preferences).Error; err != nil {
		return nil, normalizePolicyError("读取用户媒体库偏好失败", err)
	}
	if len(preferences) == 0 {
		return groupOrder, nil
	}

	enabledPreferenceSet := make(map[string]struct{}, len(preferences))
	for _, preference := range preferences {
		if preference.Enabled {
			enabledPreferenceSet[preference.LibraryID] = struct{}{}
		}
	}

	finalIDs := make([]string, 0, len(groupOrder))
	for _, libraryID := range groupOrder {
		if _, inGroup := groupSet[libraryID]; !inGroup {
			continue
		}
		if _, enabled := enabledPreferenceSet[libraryID]; enabled {
			finalIDs = append(finalIDs, libraryID)
		}
	}
	return finalIDs, nil
}

func buildManagedPolicyFields(
	rawPolicy map[string]any,
	isDisabled bool,
	template models.PlanGroupEmbyPolicyTemplate,
	enabledLibraryIDs []string,
) (map[string]any, []string) {
	managed := map[string]any{
		"IsDisabled":                     isDisabled,
		"IsAdministrator":                false,
		"EnableContentDeletion":          false,
		"EnableAllFolders":               false,
		"EnabledFolders":                 enabledLibraryIDs,
		"EnableContentDownloading":       template.EnableContentDownloading,
		"EnableLiveTvAccess":             template.EnableLiveTvAccess,
		"EnableSyncTranscoding":          template.EnableSyncTranscoding,
		"EnableAudioPlaybackTranscoding": template.EnableAudioPlaybackTranscoding,
		"EnableVideoPlaybackTranscoding": template.EnableVideoPlaybackTranscoding,
		"EnablePlaybackRemuxing":         template.EnablePlaybackRemuxing,
		"EnableRemoteAccess":             template.EnableRemoteAccess,
	}
	fields := []string{
		"IsDisabled",
		"IsAdministrator",
		"EnableContentDeletion",
		"EnableAllFolders",
		"EnabledFolders",
		"EnableContentDownloading",
		"EnableLiveTvAccess",
		"EnableSyncTranscoding",
		"EnableAudioPlaybackTranscoding",
		"EnableVideoPlaybackTranscoding",
		"EnablePlaybackRemuxing",
		"EnableRemoteAccess",
	}

	if _, ok := rawPolicy["SimultaneousStreamLimit"]; ok {
		managed["SimultaneousStreamLimit"] = template.SimultaneousStreamLimit
		fields = append(fields, "SimultaneousStreamLimit")
	} else if _, ok := rawPolicy["MaxActiveSessions"]; ok {
		managed["MaxActiveSessions"] = template.SimultaneousStreamLimit
		fields = append(fields, "MaxActiveSessions")
	} else {
		log.Printf("[Policy] Emby Policy 缺少并发播放限制字段，跳过写入 SimultaneousStreamLimit/MaxActiveSessions")
	}

	return managed, fields
}

func normalizePolicyError(prefix string, err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%s：%w", prefix, err)
}
