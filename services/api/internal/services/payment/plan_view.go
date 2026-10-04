package payment

import (
	"github.com/konghang/ember/backend/internal/models"
	"gorm.io/gorm"
)

// PlanView 承载方案查询中的关联展示字段，避免污染持久化模型。
type PlanView struct {
	models.Plan
	Purchasable    bool   `json:"purchasable" gorm:"-"`
	PurchaseReason string `json:"purchaseReason,omitempty" gorm:"-"`
	PlanGroupName  string `json:"planGroupName,omitempty" gorm:"column:planGroupName"`
}

// populatePlanBenefitNames 批量补齐商品展示名称，包括旧迁移未保存名称及分组改名。
// 仅修改响应副本；不回写商品或订单快照，也不改变权益内容和购买资格。
func populatePlanBenefitNames(tx *gorm.DB, plans []PlanView) error {
	keys := []string{}
	seen := map[string]bool{}
	for _, plan := range plans {
		for _, benefit := range plan.Benefits {
			if !seen[benefit.PlanGroup] {
				keys = append(keys, benefit.PlanGroup)
				seen[benefit.PlanGroup] = true
			}
		}
	}
	if len(keys) == 0 {
		return nil
	}
	var groups []models.PlanGroup
	if err := tx.Select("key", "name").Where("key IN ?", keys).Find(&groups).Error; err != nil {
		return err
	}
	names := make(map[string]string, len(groups))
	for _, group := range groups {
		names[group.Key] = group.Name
	}
	for i := range plans {
		plans[i].Benefits = append([]models.PlanBenefit(nil), plans[i].Benefits...)
		for j := range plans[i].Benefits {
			if name, ok := names[plans[i].Benefits[j].PlanGroup]; ok {
				plans[i].Benefits[j].PlanGroupName = name
			}
		}
	}
	return nil
}
