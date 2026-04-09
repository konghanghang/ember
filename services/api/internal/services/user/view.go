package user

import "github.com/konghang/ember/backend/internal/models"

// UserView 承载用户查询中的派生展示字段，避免污染持久化模型。
type UserView struct {
	models.User
	PlanGroupName           *string `json:"planGroupName,omitempty" gorm:"column:planGroupName"`
	EffectivePlanGroup      string  `json:"effectivePlanGroup,omitempty" gorm:"column:effectivePlanGroup"`
	EffectivePlanGroupName  string  `json:"effectivePlanGroupName,omitempty" gorm:"column:effectivePlanGroupName"`
	IsPlanGroupMissing      bool    `json:"isPlanGroupMissing" gorm:"column:isPlanGroupMissing"`
	IsUsingDefaultPlanGroup bool    `json:"isUsingDefaultPlanGroup" gorm:"-"`
}

func (u *UserView) markUsingDefaultPlanGroup() {
	u.IsUsingDefaultPlanGroup = u.PlanGroup == nil && !u.IsPlanGroupMissing
}
