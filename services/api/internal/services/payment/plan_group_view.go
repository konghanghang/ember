package payment

import "github.com/konghang/ember/backend/internal/models"

// PlanGroupView 承载套餐分组管理页的聚合展示字段，避免污染持久化模型。
type PlanGroupView struct {
	models.PlanGroup
	PlanCount          int64 `json:"planCount,omitempty"`
	UserCount          int64 `json:"userCount,omitempty"`
	FollowingUserCount int64 `json:"followingUserCount,omitempty"`
}

func buildPlanGroupView(group models.PlanGroup) PlanGroupView {
	return PlanGroupView{PlanGroup: group}
}
