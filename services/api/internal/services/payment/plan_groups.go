package payment

import (
	"errors"
	"log"
	"regexp"
	"strings"

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
		err := tx.Model(&models.PlanGroup{}).Where(`"isDefault" = ?`, true).Count(&count).Error
		return count, err
	}
	paymentCreatePlanGroup = func(tx *gorm.DB, group *models.PlanGroup) error {
		return tx.Create(group).Error
	}
	paymentSavePlanGroup = func(tx *gorm.DB, group *models.PlanGroup) error {
		return tx.Save(group).Error
	}
	paymentUnsetOtherPlanGroupDefaults = func(tx *gorm.DB, key string) error {
		return tx.Model(&models.PlanGroup{}).
			Where("key <> ?", key).
			Update(`"isDefault"`, false).Error
	}
	paymentSetPlanGroupDefault = func(tx *gorm.DB, key string, isDefault bool) error {
		return tx.Model(&models.PlanGroup{}).
			Where("key = ?", key).
			Update(`"isDefault"`, isDefault).Error
	}
	paymentCountPlansByGroup = func(tx *gorm.DB, key string) (int64, error) {
		var count int64
		err := tx.Model(&models.Plan{}).Where(`"planGroup" = ?`, key).Count(&count).Error
		return count, err
	}
	paymentCountUsersByGroup = func(tx *gorm.DB, key string) (int64, error) {
		var count int64
		err := tx.Model(&models.User{}).Where(`"planGroup" = ?`, key).Count(&count).Error
		return count, err
	}
	paymentCountRedemptionCodesByRegistrationPlanGroup = func(tx *gorm.DB, key string) (int64, error) {
		var count int64
		err := tx.Model(&models.RedemptionCode{}).Where(`"registrationPlanGroup" = ?`, key).Count(&count).Error
		return count, err
	}
	paymentDeletePlanGroup = func(tx *gorm.DB, key string) error {
		return tx.Delete(&models.PlanGroup{}, "key = ?", key).Error
	}
	paymentExpirePendingPaymentsForUsersFollowingDefault = ExpireAllPendingPaymentsForFollowingDefaultUsers
)

type CreatePlanGroupRequest struct {
	Key         string `json:"key" binding:"required"`
	Name        string `json:"name" binding:"required"`
	Description string `json:"description"`
	IsDefault   bool   `json:"isDefault"`
	SortOrder   int    `json:"sortOrder"`
}

type UpdatePlanGroupRequest struct {
	Name        *string `json:"name"`
	Description *string `json:"description"`
	IsDefault   *bool   `json:"isDefault"`
	SortOrder   *int    `json:"sortOrder"`
}

type GetPlanGroupsResponse struct {
	Data []models.PlanGroup `json:"data"`
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
	if err := tx.Where("\"isDefault\" = ?", true).First(&group).Error; err != nil {
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
			WHERE users.id = payments."userId"
			  AND users."planGroup" IS NULL
		)`).
		Update("status", models.PaymentExpired)
	if result.Error != nil {
		return 0, errors.New("更新支付记录失败")
	}
	return result.RowsAffected, nil
}

func (s *PaymentService) GetPlanGroups() (*GetPlanGroupsResponse, error) {
	var groups []models.PlanGroup
	if err := db.DB.Order(`"sortOrder" ASC, key ASC`).Find(&groups).Error; err != nil {
		return nil, errors.New("获取套餐分组失败")
	}

	for i := range groups {
		group := &groups[i]
		if err := db.DB.Model(&models.Plan{}).Where(`"planGroup" = ?`, group.Key).Count(&group.PlanCount).Error; err != nil {
			return nil, errors.New("获取套餐分组失败")
		}
		if err := db.DB.Model(&models.User{}).Where(`"planGroup" = ?`, group.Key).Count(&group.UserCount).Error; err != nil {
			return nil, errors.New("获取套餐分组失败")
		}
		if group.IsDefault {
			if err := db.DB.Model(&models.User{}).Where(`"planGroup" IS NULL`).Count(&group.FollowingUserCount).Error; err != nil {
				return nil, errors.New("获取套餐分组失败")
			}
		}
	}

	return &GetPlanGroupsResponse{Data: groups}, nil
}

func (s *PaymentService) CreatePlanGroup(req *CreatePlanGroupRequest) (*models.PlanGroup, error) {
	key, err := NormalizePlanGroupKey(req.Key, false)
	if err != nil {
		return nil, err
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		return nil, errors.New("套餐分组名称不能为空")
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
		return nil, errors.New("套餐分组标识已存在")
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
		Key:         key,
		Name:        name,
		Description: strings.TrimSpace(req.Description),
		IsDefault:   false,
		SortOrder:   req.SortOrder,
	}

	if err := paymentCreatePlanGroup(tx, &group); err != nil {
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
	return &group, nil
}

func (s *PaymentService) UpdatePlanGroup(key string, req *UpdatePlanGroupRequest) (*models.PlanGroup, error) {
	normalizedKey, err := NormalizePlanGroupKey(key, false)
	if err != nil {
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

	defaultChanged := false
	requestDefault := group.IsDefault
	if req.Name != nil {
		name := strings.TrimSpace(*req.Name)
		if name == "" {
			rollbackPlanGroupTx(tx)
			return nil, errors.New("套餐分组名称不能为空")
		}
		group.Name = name
	}
	if req.Description != nil {
		group.Description = strings.TrimSpace(*req.Description)
	}
	if req.SortOrder != nil {
		group.SortOrder = *req.SortOrder
	}
	if req.IsDefault != nil {
		if *req.IsDefault {
			defaultChanged = !group.IsDefault
			requestDefault = true
		} else if group.IsDefault {
			rollbackPlanGroupTx(tx)
			return nil, errors.New("默认套餐分组不能为空，请先设置其他默认分组")
		} else {
			requestDefault = false
		}
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

	if defaultChanged {
		expiredCount, err := paymentExpirePendingPaymentsForUsersFollowingDefault(tx)
		if err != nil {
			rollbackPlanGroupTx(tx)
			return nil, err
		}
		log.Printf("[Payment] 默认套餐分组已切换，已收口跟随默认用户的待支付订单: planGroup=%s expiredCount=%d", group.Key, expiredCount)
	}

	if err := commitPlanGroupTx(tx); err != nil {
		return nil, errors.New("更新套餐分组失败")
	}
	return group, nil
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

	if err := paymentDeletePlanGroup(tx, group.Key); err != nil {
		rollbackPlanGroupTx(tx)
		return errors.New("删除套餐分组失败")
	}

	if err := commitPlanGroupTx(tx); err != nil {
		return errors.New("删除套餐分组失败")
	}
	return nil
}
