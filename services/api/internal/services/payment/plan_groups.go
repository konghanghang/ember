package payment

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/konghang/ember/backend/internal/db"
	"github.com/konghang/ember/backend/internal/models"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var planGroupKeyPattern = regexp.MustCompile(`^[A-Z][A-Z0-9_-]{0,31}$`)

var (
	beginPlanGroupTx = func() (*gorm.DB, error) {
		tx := db.DB.Begin()
		return tx, tx.Error
	}
	commitPlanGroupTx = func(tx *gorm.DB) error {
		if tx == nil {
			return nil
		}
		return tx.Commit().Error
	}
	rollbackPlanGroupTx = func(tx *gorm.DB) {
		if tx != nil {
			tx.Rollback()
		}
	}
	paymentGetPlanGroupByKey          = GetPlanGroupByKey
	paymentGetPlanGroupByKeyForUpdate = GetPlanGroupByKeyForUpdate
	paymentCountPlanGroupsByKey       = func(tx *gorm.DB, key string) (int64, error) {
		var count int64
		err := tx.Model(&models.PlanGroup{}).Where("key = ?", key).Count(&count).Error
		return count, err
	}
	paymentCountDefaultPlanGroups = func(tx *gorm.DB) (int64, error) {
		var count int64
		err := tx.Model(&models.PlanGroup{}).Where(`"is_default" = ?`, true).Count(&count).Error
		return count, err
	}
	paymentCreatePlanGroup = func(tx *gorm.DB, group *models.PlanGroup) error {
		return tx.Create(group).Error
	}
	paymentCreateDefaultPlanGroupPolicyTemplate = func(tx *gorm.DB, key string) error {
		return tx.Create(&models.PlanGroupEmbyPolicyTemplate{
			PlanGroupKey:                   key,
			SimultaneousStreamLimit:        3,
			EnableContentDownloading:       false,
			EnableLiveTvAccess:             false,
			EnableSyncTranscoding:          false,
			EnableAudioPlaybackTranscoding: false,
			EnableVideoPlaybackTranscoding: false,
			EnablePlaybackRemuxing:         true,
			EnableRemoteAccess:             true,
		}).Error
	}
	paymentSavePlanGroup = func(tx *gorm.DB, group *models.PlanGroup) error {
		return tx.Model(&models.PlanGroup{}).
			Where("key = ?", group.Key).
			Select("watch_retention_enabled", "watch_retention_days", "watch_retention_min_minutes", "watch_retention_reset_at", "name", "description", "sort_order", "subscription_auto_approve_daily_limit", "p115_playback_mode", "p115_transfer_hourly_limit", "p115_transfer_daily_limit", "updated_at").
			Updates(map[string]any{
				"watch_retention_enabled":               group.WatchRetentionEnabled,
				"watch_retention_days":                  group.WatchRetentionDays,
				"watch_retention_min_minutes":           group.WatchRetentionMinMinutes,
				"watch_retention_reset_at":              group.WatchRetentionResetAt,
				"name":                                  group.Name,
				"description":                           group.Description,
				"sort_order":                            group.SortOrder,
				"subscription_auto_approve_daily_limit": group.SubscriptionAutoApproveDailyLimit,
				"p115_playback_mode":                    group.P115PlaybackMode,
				"p115_transfer_hourly_limit":            group.P115TransferHourlyLimit,
				"p115_transfer_daily_limit":             group.P115TransferDailyLimit,
				"updated_at":                            time.Now(),
			}).Error
	}
	paymentUnsetOtherPlanGroupDefaults = func(tx *gorm.DB, key string) error {
		return tx.Model(&models.PlanGroup{}).
			Where("key <> ?", key).
			Update(`"is_default"`, false).Error
	}
	paymentSetPlanGroupDefault = func(tx *gorm.DB, key string, isDefault bool) error {
		return tx.Model(&models.PlanGroup{}).
			Where("key = ?", key).
			Update(`"is_default"`, isDefault).Error
	}
	paymentCountPlansByGroup = func(tx *gorm.DB, key string) (int64, error) {
		var count int64
		err := tx.Model(&models.Plan{}).Where("plan_group = ?", key).Count(&count).Error
		if err != nil {
			return 0, err
		}
		var snapshots int64
		err = tx.Model(&models.Payment{}).Where(`benefits @> ?::jsonb`, fmt.Sprintf(`[{"planGroup":%q}]`, key)).Count(&snapshots).Error
		return count + snapshots, err
	}
	paymentCountUsersByGroup = func(tx *gorm.DB, key string) (int64, error) {
		var count int64
		err := tx.Model(&models.User{}).Where(`"plan_group" = ?`, key).Count(&count).Error
		if err != nil {
			return 0, err
		}
		var grants int64
		err = tx.Model(&models.UserEntitlement{}).Where("plan_group = ?", key).Count(&grants).Error
		return count + grants, err
	}
	paymentCountRedemptionCodesByRegistrationPlanGroup = func(tx *gorm.DB, key string) (int64, error) {
		var count int64
		err := tx.Model(&models.RedemptionCode{}).Where(`"registration_plan_group" = ?`, key).Count(&count).Error
		return count, err
	}
	paymentDeletePlanGroup = func(tx *gorm.DB, key string) error {
		return tx.Delete(&models.PlanGroup{}, "key = ?", key).Error
	}
	paymentDeletePlanGroupDependents = func(tx *gorm.DB, key string) error {
		if err := tx.Where("plan_group_key = ?", key).Delete(&models.PlanGroupMediaLibrary{}).Error; err != nil {
			return err
		}
		if err := tx.Where("plan_group_key = ?", key).Delete(&models.PlanGroupEmbyPolicyTemplate{}).Error; err != nil {
			return err
		}
		if err := tx.Where("plan_group_key = ?", key).Delete(&models.EmbyPolicySyncTask{}).Error; err != nil {
			return err
		}
		return tx.Where("plan_group_key = ?", key).Delete(&models.EmbyPolicySyncBatch{}).Error
	}
	paymentExpirePendingPaymentsForUsersFollowingDefault = ExpireAllPendingPaymentsForFollowingDefaultUsers
)

type CreatePlanGroupRequest struct {
	WatchRetentionEnabled    *bool `json:"watchRetentionEnabled"`
	WatchRetentionDays       *int  `json:"watchRetentionDays"`
	WatchRetentionMinMinutes *int  `json:"watchRetentionMinMinutes"`

	Key                               string  `json:"key" binding:"required"`
	Name                              string  `json:"name" binding:"required"`
	Description                       string  `json:"description"`
	IsDefault                         bool    `json:"isDefault"`
	SortOrder                         int     `json:"sortOrder"`
	SubscriptionAutoApproveDailyLimit int     `json:"subscriptionAutoApproveDailyLimit"`
	P115PlaybackMode                  *string `json:"p115PlaybackMode"`
	P115TransferHourlyLimit           *int    `json:"p115TransferHourlyLimit"`
	P115TransferDailyLimit            *int    `json:"p115TransferDailyLimit"`
}

type UpdatePlanGroupRequest struct {
	WatchRetentionEnabled    *bool `json:"watchRetentionEnabled"`
	WatchRetentionDays       *int  `json:"watchRetentionDays"`
	WatchRetentionMinMinutes *int  `json:"watchRetentionMinMinutes"`

	Name                              *string `json:"name"`
	Description                       *string `json:"description"`
	IsDefault                         *bool   `json:"isDefault"`
	SortOrder                         *int    `json:"sortOrder"`
	SubscriptionAutoApproveDailyLimit *int    `json:"subscriptionAutoApproveDailyLimit"`
	P115PlaybackMode                  *string `json:"p115PlaybackMode"`
	P115TransferHourlyLimit           *int    `json:"p115TransferHourlyLimit"`
	P115TransferDailyLimit            *int    `json:"p115TransferDailyLimit"`
}

type GetPlanGroupsResponse struct {
	Data []PlanGroupView `json:"data"`
}

func NormalizePlanGroupKey(raw string, allowEmpty bool) (string, error) {
	key := strings.ToUpper(strings.TrimSpace(raw))
	if key == "" {
		if allowEmpty {
			return "", nil
		}
		return "", ErrPlanGroupInvalid
	}
	if !planGroupKeyPattern.MatchString(key) {
		return "", ErrPlanGroupInvalid
	}
	return key, nil
}

func GetPlanGroupByKey(tx *gorm.DB, key string) (*models.PlanGroup, error) {
	return getPlanGroupByKey(tx, key, false)
}

func GetPlanGroupByKeyForUpdate(tx *gorm.DB, key string) (*models.PlanGroup, error) {
	return getPlanGroupByKey(tx, key, true)
}

func getPlanGroupByKey(tx *gorm.DB, key string, lockForUpdate bool) (*models.PlanGroup, error) {
	if tx == nil {
		tx = db.DB
	}
	key, err := NormalizePlanGroupKey(key, false)
	if err != nil {
		return nil, err
	}

	var group models.PlanGroup
	query := tx
	if lockForUpdate {
		query = query.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	if err := query.Where("key = ?", key).First(&group).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrPlanGroupNotFound
		}
		return nil, errors.New("获取套餐分组失败")
	}
	return &group, nil
}

func GetDefaultPlanGroup(tx *gorm.DB) (*models.PlanGroup, error) {
	if tx == nil {
		tx = db.DB
	}

	var group models.PlanGroup
	if err := tx.Where("\"is_default\" = ?", true).First(&group).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrDefaultPlanGroupNotFound
		}
		return nil, errors.New("获取默认套餐分组失败")
	}
	return &group, nil
}

func ResolveEffectivePlanGroupKey(tx *gorm.DB, explicitPlanGroup *string) (string, error) {
	if explicitPlanGroup != nil {
		key, err := NormalizePlanGroupKey(*explicitPlanGroup, false)
		if err != nil {
			return "", err
		}
		store := tx
		if store == nil {
			store = db.DB
		}
		if store == nil {
			return key, nil
		}
		if _, err := GetPlanGroupByKey(store, key); err != nil {
			return "", err
		}
		return key, nil
	}

	defaultGroup, err := GetDefaultPlanGroup(tx)
	if err != nil {
		return "", err
	}
	return defaultGroup.Key, nil
}

// ExpireAllPendingPaymentsForFollowingDefaultUsers 把所有"用户 planGroup 为 NULL（跟随默认分组）"
// 的 pending 订单一次性收口为 expired。
//
// 名称里 "All" + "FollowingDefaultUsers" 强调本函数会跨用户全表执行 EXISTS 子查询，
// 不限定单个 user / plan，因此被默认分组路由变更等场景调用时影响面极广；
// 调用方需明白这是全表收口动作，不能与按用户 / 按方案的局部收口混用。
func ExpireAllPendingPaymentsForFollowingDefaultUsers(tx *gorm.DB) (int64, error) {
	if tx == nil {
		tx = db.DB
	}

	result := tx.Model(&models.Payment{}).
		Where("status = ?", models.PaymentPending).
		Where(`EXISTS (
			SELECT 1
			FROM users
			WHERE users.id = payments."user_id"
			  AND users."plan_group" IS NULL
		)`).
		Update("status", models.PaymentExpired)
	if result.Error != nil {
		return 0, errors.New("更新支付记录失败")
	}
	return result.RowsAffected, nil
}

func pendingStripeSessionIDsForDefaultFollowers(tx *gorm.DB) ([]string, error) {
	if tx == nil {
		if db.DB == nil {
			return nil, nil
		}
		tx = db.DB
	}

	sessionIDs := make([]string, 0)
	if err := tx.Model(&models.Payment{}).
		Where("status = ?", models.PaymentPending).
		Where(`"stripe_session_id" <> ''`).
		Where(`EXISTS (
			SELECT 1
			FROM users
			WHERE users.id = payments."user_id"
			  AND users."plan_group" IS NULL
		)`).
		Pluck(`"stripe_session_id"`, &sessionIDs).Error; err != nil {
		return nil, errors.New("查询待失效 Stripe 会话失败")
	}
	return sessionIDs, nil
}

func (s *PaymentService) GetPlanGroups() (*GetPlanGroupsResponse, error) {
	var groups []models.PlanGroup
	if err := db.DB.Order(`"sort_order" ASC, key ASC`).Find(&groups).Error; err != nil {
		return nil, errors.New("获取套餐分组失败")
	}

	views := make([]PlanGroupView, 0, len(groups))
	for i := range groups {
		view := buildPlanGroupView(groups[i])
		if err := db.DB.Model(&models.Plan{}).Where("plan_group = ?", view.Key).Count(&view.PlanCount).Error; err != nil {
			return nil, errors.New("获取套餐分组失败")
		}
		if err := db.DB.Model(&models.User{}).Where(`"plan_group" = ?`, view.Key).Count(&view.UserCount).Error; err != nil {
			return nil, errors.New("获取套餐分组失败")
		}
		if view.IsDefault {
			if err := db.DB.Model(&models.User{}).Where(`"plan_group" IS NULL`).Count(&view.FollowingUserCount).Error; err != nil {
				return nil, errors.New("获取套餐分组失败")
			}
		}
		if err := db.DB.Model(&models.PlanGroupMediaLibrary{}).
			Where("plan_group_key = ?", view.Key).
			Where("LOWER(COALESCE(library_type, '')) <> ?", "boxsets").
			Count(&view.MediaLibraryCount).Error; err != nil {
			return nil, errors.New("获取套餐分组失败")
		}
		var templateCount int64
		if err := db.DB.Model(&models.PlanGroupEmbyPolicyTemplate{}).Where("plan_group_key = ?", view.Key).Count(&templateCount).Error; err != nil {
			return nil, errors.New("获取套餐分组失败")
		}
		view.EmbyPolicyTemplateConfigured = templateCount > 0
		view.PolicySyncStatus = planGroupPolicySyncStatus(view.Key)
		views = append(views, view)
	}

	return &GetPlanGroupsResponse{Data: views}, nil
}

func planGroupPolicySyncStatus(key string) string {
	var processingCount int64
	if err := planGroupManagedPolicyTaskQuery(db.DB, key).
		Where("tasks.status = ?", "processing").
		Count(&processingCount).Error; err != nil || processingCount > 0 {
		if processingCount > 0 {
			return "processing"
		}
		return ""
	}
	var pendingCount int64
	if err := planGroupManagedPolicyTaskQuery(db.DB, key).
		Where("tasks.status = ?", "pending").
		Count(&pendingCount).Error; err != nil || pendingCount > 0 {
		if pendingCount > 0 {
			return "pending"
		}
		return ""
	}
	var failedCount int64
	if err := planGroupManagedPolicyTaskQuery(db.DB, key).
		Where("tasks.status = ?", "failed").
		Count(&failedCount).Error; err != nil || failedCount == 0 {
		var outOfSyncCount int64
		if err := planGroupManagedUsersQuery(db.DB, key).
			Where(`users."applied_media_library_template_version" < COALESCE(explicit_pg.media_library_template_version, default_pg.media_library_template_version)`).
			Count(&outOfSyncCount).Error; err != nil || outOfSyncCount == 0 {
			return "synced"
		}
		return "out_of_sync"
	}
	var syncedCount int64
	_ = planGroupManagedPolicyTaskQuery(db.DB, key).
		Where("tasks.status = ?", "synced").
		Count(&syncedCount).Error
	if syncedCount > 0 {
		return "partial_failed"
	}
	return "failed"
}

func planGroupManagedPolicyTaskQuery(database *gorm.DB, key string) *gorm.DB {
	return database.Table("emby_policy_sync_tasks AS tasks").
		Joins("JOIN users ON users.id = tasks.user_id").
		Where("tasks.plan_group_key = ? AND users.role = ?", key, "user").
		Where("NOT (tasks.status = ? AND COALESCE(tasks.last_error, '') LIKE ?)", "failed", "%There must be at least one user in the system with administrative access%")
}

func planGroupManagedUsersQuery(database *gorm.DB, key string) *gorm.DB {
	return database.Table("users").
		Joins(`LEFT JOIN plan_groups explicit_pg ON explicit_pg.key = users."plan_group"`).
		Joins(`LEFT JOIN plan_groups default_pg ON default_pg."is_default" = ?`, true).
		Where(`users."role" = ?`, "user").
		Where(`COALESCE(users."emby_id", '') <> ''`).
		Where(`COALESCE(users."plan_group", default_pg.key) = ?`, key)
}

func (s *PaymentService) CreatePlanGroup(req *CreatePlanGroupRequest) (*PlanGroupView, error) {
	key, err := NormalizePlanGroupKey(req.Key, false)
	if err != nil {
		return nil, err
	}
	if req.SubscriptionAutoApproveDailyLimit < 0 {
		return nil, ErrPlanGroupSubscriptionAutoApproveDailyLimitInvalid
	}
	p115PlaybackMode, p115HourlyLimit, p115DailyLimit, err := resolveCreateP115Policy(req)
	if err != nil {
		return nil, err
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		return nil, ErrPlanGroupNameRequired
	}

	tx, err := beginPlanGroupTx()
	if err != nil {
		return nil, errors.New("创建套餐分组失败")
	}

	count, err := paymentCountPlanGroupsByKey(tx, key)
	if err != nil {
		rollbackPlanGroupTx(tx)
		return nil, errors.New("创建套餐分组失败")
	}
	if count > 0 {
		rollbackPlanGroupTx(tx)
		return nil, ErrPlanGroupKeyExists
	}
	shouldBeDefault := req.IsDefault
	defaultCount, err := paymentCountDefaultPlanGroups(tx)
	if err != nil {
		rollbackPlanGroupTx(tx)
		return nil, errors.New("创建套餐分组失败")
	}
	if defaultCount == 0 {
		shouldBeDefault = true
	}

	group := models.PlanGroup{
		Key:                               key,
		Name:                              name,
		Description:                       strings.TrimSpace(req.Description),
		IsDefault:                         false,
		SortOrder:                         req.SortOrder,
		SubscriptionAutoApproveDailyLimit: req.SubscriptionAutoApproveDailyLimit,
		P115PlaybackMode:                  p115PlaybackMode,
		P115TransferHourlyLimit:           p115HourlyLimit,
		P115TransferDailyLimit:            p115DailyLimit,
		MediaLibraryTemplateVersion:       1,
	}

	if err := applyWatchRetentionPolicy(&group, req.WatchRetentionEnabled, req.WatchRetentionDays, req.WatchRetentionMinMinutes, time.Now()); err != nil {
		rollbackPlanGroupTx(tx)
		return nil, err
	}
	if err := paymentCreatePlanGroup(tx, &group); err != nil {
		rollbackPlanGroupTx(tx)
		return nil, errors.New("创建套餐分组失败")
	}
	if err := paymentCreateDefaultPlanGroupPolicyTemplate(tx, group.Key); err != nil {
		rollbackPlanGroupTx(tx)
		return nil, errors.New("创建套餐分组失败")
	}

	if shouldBeDefault {
		if err := paymentUnsetOtherPlanGroupDefaults(tx, group.Key); err != nil {
			rollbackPlanGroupTx(tx)
			return nil, errors.New("创建套餐分组失败")
		}
		if err := paymentSetPlanGroupDefault(tx, group.Key, true); err != nil {
			rollbackPlanGroupTx(tx)
			return nil, errors.New("创建套餐分组失败")
		}
		group.IsDefault = true
	}

	if err := commitPlanGroupTx(tx); err != nil {
		return nil, errors.New("创建套餐分组失败")
	}
	view := buildPlanGroupView(group)
	return &view, nil
}

func (s *PaymentService) UpdatePlanGroup(key string, req *UpdatePlanGroupRequest) (*PlanGroupView, error) {
	normalizedKey, err := NormalizePlanGroupKey(key, false)
	if err != nil {
		return nil, err
	}
	if err := validateP115PolicyUpdate(req); err != nil {
		return nil, err
	}

	tx, err := beginPlanGroupTx()
	if err != nil {
		return nil, errors.New("更新套餐分组失败")
	}

	group, err := paymentGetPlanGroupByKeyForUpdate(tx, normalizedKey)
	if err != nil {
		rollbackPlanGroupTx(tx)
		return nil, err
	}

	requestDefault := group.IsDefault
	if req.Name != nil {
		name := strings.TrimSpace(*req.Name)
		if name == "" {
			rollbackPlanGroupTx(tx)
			return nil, ErrPlanGroupNameRequired
		}
		group.Name = name
	}
	if req.Description != nil {
		group.Description = strings.TrimSpace(*req.Description)
	}
	if req.SortOrder != nil {
		group.SortOrder = *req.SortOrder
	}
	if req.SubscriptionAutoApproveDailyLimit != nil {
		if *req.SubscriptionAutoApproveDailyLimit < 0 {
			rollbackPlanGroupTx(tx)
			return nil, ErrPlanGroupSubscriptionAutoApproveDailyLimitInvalid
		}
		group.SubscriptionAutoApproveDailyLimit = *req.SubscriptionAutoApproveDailyLimit
	}
	if req.P115PlaybackMode != nil {
		group.P115PlaybackMode = models.P115PlaybackMode(strings.ToLower(strings.TrimSpace(*req.P115PlaybackMode)))
	}
	if req.P115TransferHourlyLimit != nil {
		group.P115TransferHourlyLimit = *req.P115TransferHourlyLimit
	}
	if req.P115TransferDailyLimit != nil {
		group.P115TransferDailyLimit = *req.P115TransferDailyLimit
	}
	if req.IsDefault != nil {
		if *req.IsDefault {
			requestDefault = true
		} else if group.IsDefault {
			rollbackPlanGroupTx(tx)
			return nil, ErrDefaultPlanGroupRequired
		} else {
			requestDefault = false
		}
	}

	if err := applyWatchRetentionPolicy(group, req.WatchRetentionEnabled, req.WatchRetentionDays, req.WatchRetentionMinMinutes, time.Now()); err != nil {
		rollbackPlanGroupTx(tx)
		return nil, err
	}
	if err := paymentSavePlanGroup(tx, group); err != nil {
		rollbackPlanGroupTx(tx)
		return nil, errors.New("更新套餐分组失败")
	}

	if requestDefault {
		if err := paymentUnsetOtherPlanGroupDefaults(tx, group.Key); err != nil {
			rollbackPlanGroupTx(tx)
			return nil, errors.New("更新套餐分组失败")
		}
		if !group.IsDefault {
			if err := paymentSetPlanGroupDefault(tx, group.Key, true); err != nil {
				rollbackPlanGroupTx(tx)
				return nil, errors.New("更新套餐分组失败")
			}
			group.IsDefault = true
		}
	}

	if err := commitPlanGroupTx(tx); err != nil {
		return nil, errors.New("更新套餐分组失败")
	}
	view := buildPlanGroupView(*group)
	return &view, nil
}

func resolveCreateP115Policy(req *CreatePlanGroupRequest) (models.P115PlaybackMode, int, int, error) {
	mode := models.P115PlaybackModePersonal
	hourlyLimit := 5
	dailyLimit := 10
	if req.P115PlaybackMode != nil {
		mode = models.P115PlaybackMode(strings.ToLower(strings.TrimSpace(*req.P115PlaybackMode)))
	}
	if req.P115TransferHourlyLimit != nil {
		hourlyLimit = *req.P115TransferHourlyLimit
	}
	if req.P115TransferDailyLimit != nil {
		dailyLimit = *req.P115TransferDailyLimit
	}
	if err := validateP115Policy(mode, hourlyLimit, dailyLimit); err != nil {
		return "", 0, 0, err
	}
	return mode, hourlyLimit, dailyLimit, nil
}

func validateP115PolicyUpdate(req *UpdatePlanGroupRequest) error {
	if req.P115PlaybackMode != nil {
		mode := models.P115PlaybackMode(strings.ToLower(strings.TrimSpace(*req.P115PlaybackMode)))
		if mode != models.P115PlaybackModePersonal && mode != models.P115PlaybackModeSystem {
			return ErrPlanGroupP115PlaybackModeInvalid
		}
	}
	if req.P115TransferHourlyLimit != nil && (*req.P115TransferHourlyLimit < 1 || *req.P115TransferHourlyLimit > 100) {
		return ErrPlanGroupP115TransferHourlyLimitInvalid
	}
	if req.P115TransferDailyLimit != nil && (*req.P115TransferDailyLimit < 1 || *req.P115TransferDailyLimit > 1000) {
		return ErrPlanGroupP115TransferDailyLimitInvalid
	}
	return nil
}

func validateP115Policy(mode models.P115PlaybackMode, hourlyLimit, dailyLimit int) error {
	if mode != models.P115PlaybackModePersonal && mode != models.P115PlaybackModeSystem {
		return ErrPlanGroupP115PlaybackModeInvalid
	}
	if hourlyLimit < 1 || hourlyLimit > 100 {
		return ErrPlanGroupP115TransferHourlyLimitInvalid
	}
	if dailyLimit < 1 || dailyLimit > 1000 {
		return ErrPlanGroupP115TransferDailyLimitInvalid
	}
	return nil
}

func (s *PaymentService) DeletePlanGroup(key string) error {
	normalizedKey, err := NormalizePlanGroupKey(key, false)
	if err != nil {
		return err
	}

	tx, err := beginPlanGroupTx()
	if err != nil {
		return errors.New("删除套餐分组失败")
	}

	group, err := paymentGetPlanGroupByKeyForUpdate(tx, normalizedKey)
	if err != nil {
		rollbackPlanGroupTx(tx)
		return err
	}
	if group.IsDefault {
		rollbackPlanGroupTx(tx)
		return ErrDefaultPlanGroupDelete
	}

	planCount, err := paymentCountPlansByGroup(tx, group.Key)
	if err != nil {
		rollbackPlanGroupTx(tx)
		return errors.New("删除套餐分组失败")
	}
	userCount, err := paymentCountUsersByGroup(tx, group.Key)
	if err != nil {
		rollbackPlanGroupTx(tx)
		return errors.New("删除套餐分组失败")
	}
	redemptionCodeCount, err := paymentCountRedemptionCodesByRegistrationPlanGroup(tx, group.Key)
	if err != nil {
		rollbackPlanGroupTx(tx)
		return errors.New("删除套餐分组失败")
	}
	if planCount > 0 || userCount > 0 || redemptionCodeCount > 0 {
		rollbackPlanGroupTx(tx)
		return ErrPlanGroupDeleteBlocked
	}

	if err := paymentDeletePlanGroupDependents(tx, group.Key); err != nil {
		rollbackPlanGroupTx(tx)
		return errors.New("删除套餐分组失败")
	}

	if err := paymentDeletePlanGroup(tx, group.Key); err != nil {
		rollbackPlanGroupTx(tx)
		return errors.New("删除套餐分组失败")
	}

	if err := commitPlanGroupTx(tx); err != nil {
		return errors.New("删除套餐分组失败")
	}
	return nil
}
