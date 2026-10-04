package payment

import (
	"encoding/json"
	"testing"

	"github.com/konghang/ember/backend/internal/models"
)

// TestPlanRequestsRejectBenefits prevents old multi-group payloads from being silently ignored.
func TestPlanRequestsRejectBenefits(t *testing.T) {
	for _, raw := range []string{`{"benefits":null}`, `{"benefits":[]}`, `{"benefits":[{"planGroup":"A"}]}`, `{"benefits":[{"planGroup":"A"},{"planGroup":"B"}]}`} {
		for _, target := range []any{&CreatePlanRequest{}, &UpdatePlanRequest{}} {
			if err := json.Unmarshal([]byte(raw), target); err == nil {
				t.Errorf("accepted obsolete input %s", raw)
			}
		}
	}
}

// TestSingleGroupPlanSnapshot checks explicit validity and one-item order snapshots.
func TestSingleGroupPlanSnapshot(t *testing.T) {
	for _, validity := range []string{"duration", "permanent"} {
		days := 30
		if validity == "permanent" {
			days = 0
		}
		plan := models.Plan{PlanGroup: "BASE", ValidityType: validity, Days: days}
		if err := validateSinglePlan(&plan); err != nil {
			t.Fatal(err)
		}
		benefits := plan.EntitlementBenefits()
		if len(benefits) != 1 || benefits[0].PlanGroup != "BASE" || benefits[0].ValidityType != validity || benefits[0].DurationDays != days {
			t.Fatalf("snapshot = %+v", benefits)
		}
	}
	for _, p := range []models.Plan{{PlanGroup: "BASE", Days: 0}, {PlanGroup: "BASE", ValidityType: "duration", Days: 0}, {PlanGroup: "BASE", ValidityType: "duration", Days: -1}, {PlanGroup: "BASE", ValidityType: "other", Days: 30}, {ValidityType: "duration", Days: 30}} {
		if err := validateSinglePlan(&p); err == nil {
			t.Errorf("accepted invalid plan %+v", p)
		}
	}
	p := models.Plan{PlanGroup: "BASE", ValidityType: "permanent", Days: 30}
	if err := validateSinglePlan(&p); err != nil || p.Days != 0 {
		t.Fatalf("permanent days must normalize: %+v, %v", p, err)
	}
}
