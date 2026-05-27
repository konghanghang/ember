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
	IsExpired               bool    `json:"isExpired" gorm:"-"`

	MediaLibraryPreferenceCustomized bool   `json:"mediaLibraryPreferenceCustomized" gorm:"column:mediaLibraryPreferenceCustomized"`
	MediaLibraryTemplateCount        int    `json:"mediaLibraryTemplateCount" gorm:"column:mediaLibraryTemplateCount"`
	MediaLibraryEnabledCount         int    `json:"mediaLibraryEnabledCount" gorm:"column:mediaLibraryEnabledCount"`
	PolicySyncStatus                 string `json:"policySyncStatus" gorm:"column:policySyncStatus"`
}

func (u *UserView) markUsingDefaultPlanGroup() {
	u.IsUsingDefaultPlanGroup = u.PlanGroup == nil && !u.IsPlanGroupMissing
	u.IsExpired = u.User.IsExpired()
	if u.PolicySyncStatus == "" {
		u.PolicySyncStatus = "synced"
	}
}
