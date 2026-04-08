package payment

import (
	"errors"
	"log"
	"regexp"
	"strings"

	"github.com/konghang/ember/backend/internal/db"
	"github.com/konghang/ember/backend/internal/models"
	"gorm.io/gorm"
)

var planGroupKeyPattern = regexp.MustCompile(`^[A-Z][A-Z0-9_-]{0,31}$`)

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
	if tx == nil {
		tx = db.DB
	}
	key, err := NormalizePlanGroupKey(key, false)
	if err != nil {
		return nil, err
	}

	var group models.PlanGroup
	if err := tx.Where("key = ?", key).First(&group).Error; err != nil {
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

func ExpirePendingPaymentsForUsersFollowingDefault(tx *gorm.DB) (int64, error) {
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

	tx := db.DB.Begin()
	if tx.Error != nil {
		return nil, errors.New("创建套餐分组失败")
	}

	var count int64
	if err := tx.Model(&models.PlanGroup{}).Where("key = ?", key).Count(&count).Error; err != nil {
		tx.Rollback()
		return nil, errors.New("创建套餐分组失败")
	}
	if count > 0 {
		tx.Rollback()
		return nil, errors.New("套餐分组标识已存在")
	}
	shouldBeDefault := req.IsDefault
	var defaultCount int64
	if err := tx.Model(&models.PlanGroup{}).Where(`"isDefault" = ?`, true).Count(&defaultCount).Error; err != nil {
		tx.Rollback()
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

	if err := tx.Create(&group).Error; err != nil {
		tx.Rollback()
		return nil, errors.New("创建套餐分组失败")
	}

	if shouldBeDefault {
		if err := tx.Model(&models.PlanGroup{}).
			Where("key <> ?", group.Key).
			Update(`"isDefault"`, false).Error; err != nil {
			tx.Rollback()
			return nil, errors.New("创建套餐分组失败")
		}
		if err := tx.Model(&models.PlanGroup{}).
			Where("key = ?", group.Key).
			Update(`"isDefault"`, true).Error; err != nil {
			tx.Rollback()
			return nil, errors.New("创建套餐分组失败")
		}
		group.IsDefault = true
	}

	if err := tx.Commit().Error; err != nil {
		return nil, errors.New("创建套餐分组失败")
	}
	return &group, nil
}

func (s *PaymentService) UpdatePlanGroup(key string, req *UpdatePlanGroupRequest) (*models.PlanGroup, error) {
	normalizedKey, err := NormalizePlanGroupKey(key, false)
	if err != nil {
		return nil, err
	}

	tx := db.DB.Begin()
	if tx.Error != nil {
		return nil, errors.New("更新套餐分组失败")
	}

	group, err := GetPlanGroupByKey(tx, normalizedKey)
	if err != nil {
		tx.Rollback()
		return nil, err
	}

	defaultChanged := false
	requestDefault := group.IsDefault
	if req.Name != nil {
		name := strings.TrimSpace(*req.Name)
		if name == "" {
			tx.Rollback()
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
			tx.Rollback()
			return nil, errors.New("默认套餐分组不能为空，请先设置其他默认分组")
		} else {
			requestDefault = false
		}
	}

	if err := tx.Save(group).Error; err != nil {
		tx.Rollback()
		return nil, errors.New("更新套餐分组失败")
	}

	if requestDefault {
		if err := tx.Model(&models.PlanGroup{}).
			Where("key <> ?", group.Key).
			Update(`"isDefault"`, false).Error; err != nil {
			tx.Rollback()
			return nil, errors.New("更新套餐分组失败")
		}
		if !group.IsDefault {
			if err := tx.Model(&models.PlanGroup{}).
				Where("key = ?", group.Key).
				Update(`"isDefault"`, true).Error; err != nil {
				tx.Rollback()
				return nil, errors.New("更新套餐分组失败")
			}
			group.IsDefault = true
		}
	}

	if defaultChanged {
		expiredCount, err := ExpirePendingPaymentsForUsersFollowingDefault(tx)
		if err != nil {
			tx.Rollback()
			return nil, err
		}
		log.Printf("[Payment] 默认套餐分组已切换，已收口跟随默认用户的待支付订单: planGroup=%s expiredCount=%d", group.Key, expiredCount)
	}

	if err := tx.Commit().Error; err != nil {
		return nil, errors.New("更新套餐分组失败")
	}
	return group, nil
}

func (s *PaymentService) DeletePlanGroup(key string) error {
	normalizedKey, err := NormalizePlanGroupKey(key, false)
	if err != nil {
		return err
	}

	tx := db.DB.Begin()
	if tx.Error != nil {
		return errors.New("删除套餐分组失败")
	}

	group, err := GetPlanGroupByKey(tx, normalizedKey)
	if err != nil {
		tx.Rollback()
		return err
	}
	if group.IsDefault {
		tx.Rollback()
		return ErrDefaultPlanGroupDelete
	}

	var planCount int64
	if err := tx.Model(&models.Plan{}).Where(`"planGroup" = ?`, group.Key).Count(&planCount).Error; err != nil {
		tx.Rollback()
		return errors.New("删除套餐分组失败")
	}
	var userCount int64
	if err := tx.Model(&models.User{}).Where(`"planGroup" = ?`, group.Key).Count(&userCount).Error; err != nil {
		tx.Rollback()
		return errors.New("删除套餐分组失败")
	}
	if planCount > 0 || userCount > 0 {
		tx.Rollback()
		return ErrPlanGroupDeleteBlocked
	}

	if err := tx.Delete(&models.PlanGroup{}, "key = ?", group.Key).Error; err != nil {
		tx.Rollback()
		return errors.New("删除套餐分组失败")
	}

	if err := tx.Commit().Error; err != nil {
		return errors.New("删除套餐分组失败")
	}
	return nil
}
