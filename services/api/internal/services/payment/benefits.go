package payment

import (
	"github.com/konghang/ember/backend/internal/models"
	entitlementpkg "github.com/konghang/ember/backend/internal/services/entitlement"
	"gorm.io/gorm"
)

// validatePlanBenefits checks explicit grants; the single-group input remains an additive API compatibility boundary.
// Existing clients cannot overwrite a multi-benefit product through its legacy summary fields.
func validatePlanBenefits(tx *gorm.DB, benefits []models.PlanBenefit, legacyGroup string, legacyDays int) ([]models.PlanBenefit, error) {
	if len(benefits) == 0 {
		benefits = []models.PlanBenefit{{PlanGroup: legacyGroup, ValidityType: entitlementpkg.Duration, DurationDays: legacyDays}}
	}
	benefits = append([]models.PlanBenefit(nil), benefits...)
	for i, benefit := range benefits {
		group, err := GetPlanGroupByKey(tx, benefit.PlanGroup)
		if err != nil {
			return nil, err
		}
		benefits[i].PlanGroup = group.Key
		benefits[i].PlanGroupName = group.Name
	}
	if err := entitlementpkg.ValidateBenefits(benefits); err != nil {
		return nil, err
	}
	return benefits, nil
}
