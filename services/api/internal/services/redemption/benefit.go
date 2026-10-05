package redemption

import (
	"github.com/konghang/ember/backend/internal/models"
	entitlementpkg "github.com/konghang/ember/backend/internal/services/entitlement"
)

// CodeBenefit maps the same code to a single group grant for both registration and redemption.
// Omitted validity preserves legacy duration requests; zero days never implicitly means permanent.
func CodeBenefit(code *models.RedemptionCode) (models.PlanBenefit, error) {
	kind := code.ValidityType
	if kind == "" {
		kind = entitlementpkg.Duration
	}
	benefit := models.PlanBenefit{PlanGroup: code.RegistrationPlanGroup, ValidityType: kind, DurationDays: code.DefaultDays}
	if err := entitlementpkg.ValidateBenefits([]entitlementpkg.Benefit{benefit}); err != nil {
		return models.PlanBenefit{}, ErrRedemptionValidityInvalid
	}
	return benefit, nil
}
