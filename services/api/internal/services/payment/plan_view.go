package payment

import "github.com/konghang/ember/backend/internal/models"

// PlanView 承载方案查询中的关联展示字段，避免污染持久化模型。
type PlanView struct {
	models.Plan
	Purchasable    bool   `json:"purchasable" gorm:"-"`
	PurchaseReason string `json:"purchaseReason,omitempty" gorm:"-"`
	PlanGroupName  string `json:"planGroupName,omitempty" gorm:"column:planGroupName"`
}
