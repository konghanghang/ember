package payment

import "github.com/konghang/ember/backend/internal/models"

// PlanView 承载方案查询中的关联展示字段，避免污染持久化模型。
type PlanView struct {
	models.Plan
	PlanGroupName string `json:"planGroupName,omitempty" gorm:"column:planGroupName"`
}
