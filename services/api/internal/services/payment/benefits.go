package payment

import (
	"encoding/json"
	"github.com/konghang/ember/backend/internal/models"
	entitlementpkg "github.com/konghang/ember/backend/internal/services/entitlement"
	"strings"
)

// validateSinglePlan enforces explicit duration/permanent semantics, normalizing unused permanent days.
func validateSinglePlan(plan *models.Plan) error {
	if strings.TrimSpace(plan.PlanGroup) == "" {
		return entitlementpkg.ErrInvalidBenefit
	}
	if plan.ValidityType == entitlementpkg.Permanent {
		plan.Days = 0
	}
	return entitlementpkg.ValidateBenefits(plan.EntitlementBenefits())
}

// decodeSinglePlan rejects the removed product-array contract, including explicit null and empty arrays.
// It does not affect immutable payment snapshots or the separate compensation contract.
func decodeSinglePlan(data []byte, target any) error {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}
	for key := range fields {
		if strings.EqualFold(key, "benefits") {
			return entitlementpkg.ErrInvalidBenefit
		}
	}
	return json.Unmarshal(data, target)
}

// UnmarshalJSON keeps obsolete product payloads from silently losing granted groups.
func (r *CreatePlanRequest) UnmarshalJSON(data []byte) error {
	type request CreatePlanRequest
	return decodeSinglePlan(data, (*request)(r))
}

// UnmarshalJSON applies the same single-group boundary to partial product updates.
func (r *UpdatePlanRequest) UnmarshalJSON(data []byte) error {
	type request UpdatePlanRequest
	return decodeSinglePlan(data, (*request)(r))
}
