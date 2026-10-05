package redemption

import (
	"fmt"
	"testing"

	"github.com/konghang/ember/backend/internal/models"
)

// TestCodeBenefitValidity covers legacy payloads and rejects ambiguous zero-day or permanent-duration grants.
func TestCodeBenefitValidity(t *testing.T) {
	for _, tc := range []struct {
		kind    string
		days    int
		want    string
		invalid bool
	}{
		{"", 30, "duration", false}, {"duration", 1, "duration", false}, {"permanent", 0, "permanent", false},
		{"", 0, "", true}, {"duration", 0, "", true}, {"permanent", 30, "", true}, {"permanent", -1, "", true}, {"forever", 0, "", true},
	} {
		t.Run(fmt.Sprintf("%s_%d", tc.kind, tc.days), func(t *testing.T) {
			got, err := CodeBenefit(&models.RedemptionCode{RegistrationPlanGroup: "BASE", ValidityType: tc.kind, DefaultDays: tc.days})
			if tc.invalid {
				if err != ErrRedemptionValidityInvalid {
					t.Fatalf("got %v", err)
				}
				return
			}
			if err != nil || got.ValidityType != tc.want || got.DurationDays != tc.days || got.PlanGroup != "BASE" {
				t.Fatalf("got %+v %v", got, err)
			}
		})
	}
}
