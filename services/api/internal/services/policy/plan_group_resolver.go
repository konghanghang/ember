package policy

import (
	"errors"
	"regexp"
	"strings"

	"github.com/konghang/ember/backend/internal/db"
	"github.com/konghang/ember/backend/internal/models"
	"gorm.io/gorm"
)

var planGroupKeyPattern = regexp.MustCompile(`^[A-Z][A-Z0-9_-]{0,31}$`)

var (
	ErrPlanGroupInvalid         = errors.New("套餐分组标识无效")
	ErrPlanGroupNotFound        = errors.New("套餐分组不存在")
	ErrDefaultPlanGroupNotFound = errors.New("默认套餐分组不存在")
)

func normalizePlanGroupKey(raw string, allowEmpty bool) (string, error) {
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

func getPlanGroupByKey(tx *gorm.DB, key string) (*models.PlanGroup, error) {
	if tx == nil {
		tx = db.DB
	}
	key, err := normalizePlanGroupKey(key, false)
	if err != nil {
		return nil, err
	}
	var group models.PlanGroup
	if err := tx.Where("key = ?", key).First(&group).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrPlanGroupNotFound
		}
		return nil, err
	}
	return &group, nil
}

func getDefaultPlanGroup(tx *gorm.DB) (*models.PlanGroup, error) {
	if tx == nil {
		tx = db.DB
	}
	var group models.PlanGroup
	if err := tx.Where(`"is_default" = ?`, true).First(&group).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrDefaultPlanGroupNotFound
		}
		return nil, err
	}
	return &group, nil
}

func resolveEffectivePlanGroupKey(tx *gorm.DB, explicitPlanGroup *string) (string, error) {
	if explicitPlanGroup != nil {
		key, err := normalizePlanGroupKey(*explicitPlanGroup, false)
		if err != nil {
			return "", err
		}
		if _, err := getPlanGroupByKey(tx, key); err != nil {
			return "", err
		}
		return key, nil
	}
	group, err := getDefaultPlanGroup(tx)
	if err != nil {
		return "", err
	}
	return group.Key, nil
}
