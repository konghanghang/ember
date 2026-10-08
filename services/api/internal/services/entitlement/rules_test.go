package entitlement

import (
	"errors"
	"testing"
	"time"
)

// TestEntitlementLifecycle covers independent clocks, permanent fallback and exact expiry boundaries.
func TestEntitlementLifecycle(t *testing.T) {
	loc, _ := time.LoadLocation("Asia/Shanghai")
	now := time.Date(2026, 10, 4, 12, 0, 0, 0, loc)
	ranks := map[string]int{"A": 10, "B": 20}
	owned, err := Grant(nil, []Benefit{{PlanGroup: "A", ValidityType: Permanent}}, ranks, now, loc)
	if err != nil {
		t.Fatal(err)
	}
	owned, err = Grant(owned, []Benefit{{PlanGroup: "B", ValidityType: Duration, DurationDays: 30}}, ranks, now, loc)
	if err != nil {
		t.Fatal(err)
	}
	current, err := Resolve(owned, ranks, now)
	if err != nil || current == nil || current.PlanGroup != "B" {
		t.Fatalf("current=%+v err=%v", current, err)
	}
	current, err = Resolve(owned, ranks, now.AddDate(0, 0, 30))
	if err != nil || current == nil || current.PlanGroup != "A" || current.ValidityType != Permanent {
		t.Fatalf("fallback=%+v err=%v", current, err)
	}
	if _, err := Grant(owned, []Benefit{{PlanGroup: "A", ValidityType: Duration, DurationDays: 30}}, ranks, now, loc); !errors.Is(err, ErrAlreadyOwned) {
		t.Fatalf("permanent overwritten: %v", err)
	}
}

// TestFiniteFallbackAndRenewal verifies renewal never converts another group's remaining days.
func TestFiniteFallbackAndRenewal(t *testing.T) {
	loc, _ := time.LoadLocation("Asia/Shanghai")
	now := time.Date(2026, 10, 4, 12, 0, 0, 0, loc)
	ranks := map[string]int{"A": 10, "B": 20}
	owned, err := Grant(nil, []Benefit{{PlanGroup: "A", ValidityType: Duration, DurationDays: 60}, {PlanGroup: "B", ValidityType: Duration, DurationDays: 30}}, ranks, now, loc)
	if err != nil {
		t.Fatal(err)
	}
	current, _ := Resolve(owned, ranks, now.AddDate(0, 0, 30))
	if current == nil || current.PlanGroup != "A" || !current.ExpiresAt.Equal(now.AddDate(0, 0, 60)) {
		t.Fatalf("wrong fallback %+v", current)
	}
	current, _ = Resolve(owned, ranks, now.AddDate(0, 0, 60))
	if current != nil {
		t.Fatalf("expired access %+v", current)
	}
	owned, err = Grant(owned, []Benefit{{PlanGroup: "B", ValidityType: Duration, DurationDays: 180}}, ranks, now.AddDate(0, 0, 10), loc)
	if err != nil {
		t.Fatal(err)
	}
	current, _ = Resolve(owned, ranks, now.AddDate(0, 0, 10))
	if current == nil || !current.ExpiresAt.Equal(now.AddDate(0, 0, 210)) {
		t.Fatalf("renewal %+v", current)
	}
}

// TestPurchaseCoverage covers high-rank permanent coverage and partial bundle overlap.
func TestPurchaseCoverage(t *testing.T) {
	loc, _ := time.LoadLocation("Asia/Shanghai")
	now := time.Date(2026, 10, 4, 12, 0, 0, 0, loc)
	ranks := map[string]int{"A": 10, "B": 20}
	permanentB := []Holding{{PlanGroup: "B", ValidityType: Permanent}}
	for _, benefits := range [][]Benefit{{{PlanGroup: "A", ValidityType: Permanent, DurationDays: 0}}, {{PlanGroup: "B", ValidityType: Duration, DurationDays: 30}}, {{PlanGroup: "A", ValidityType: Permanent, DurationDays: 0}, {PlanGroup: "B", ValidityType: Permanent, DurationDays: 0}}} {
		if _, err := Grant(permanentB, benefits, ranks, now, loc); !errors.Is(err, ErrAlreadyOwned) {
			t.Fatalf("covered purchase allowed: %v", err)
		}
	}
	owned, err := Grant([]Holding{{PlanGroup: "A", ValidityType: Permanent}}, []Benefit{{PlanGroup: "A", ValidityType: Permanent, DurationDays: 0}, {PlanGroup: "B", ValidityType: Permanent, DurationDays: 0}}, ranks, now, loc)
	if err != nil || len(owned) != 2 {
		t.Fatalf("bundle failed %+v %v", owned, err)
	}
	expires := now.AddDate(0, 0, 30)
	owned, err = Grant([]Holding{{PlanGroup: "B", ValidityType: Duration, ExpiresAt: &expires}}, []Benefit{{PlanGroup: "A", ValidityType: Permanent, DurationDays: 0}}, ranks, now, loc)
	if err != nil || len(owned) != 2 {
		t.Fatalf("fallback purchase blocked: %v", err)
	}
}

// TestBenefitValidation rejects ambiguous duration and duplicates before any grant.
func TestBenefitValidation(t *testing.T) {
	for _, benefits := range [][]Benefit{nil, {{PlanGroup: "A", ValidityType: Duration, DurationDays: 0}}, {{PlanGroup: "A", ValidityType: Permanent, DurationDays: 1}}, {{PlanGroup: "A", ValidityType: "", DurationDays: 30}}, {{PlanGroup: "A", ValidityType: Permanent, DurationDays: 0}, {PlanGroup: "A", ValidityType: Duration, DurationDays: 30}}} {
		if err := ValidateBenefits(benefits); err == nil {
			t.Fatalf("accepted %+v", benefits)
		}
	}
	if _, err := Grant(nil, []Benefit{{PlanGroup: "A", ValidityType: Duration, DurationDays: int(^uint(0) >> 1)}}, map[string]int{"A": 1}, time.Now(), time.UTC); !errors.Is(err, ErrInvalidBenefit) {
		t.Fatalf("overflowing duration accepted: %v", err)
	}
}

// TestRenewalUsesBusinessCalendar preserves configured wall time across DST and restarts expired grants.
func TestRenewalUsesBusinessCalendar(t *testing.T) {
	loc, _ := time.LoadLocation("America/New_York")
	now := time.Date(2026, 3, 7, 12, 0, 0, 0, loc)
	ranks := map[string]int{"A": 1}
	owned, err := Grant(nil, []Benefit{{PlanGroup: "A", ValidityType: Duration, DurationDays: 1}}, ranks, now, loc)
	if err != nil {
		t.Fatal(err)
	}
	if got := owned[0].ExpiresAt.In(loc); got.Hour() != 12 || got.Sub(now) != 23*time.Hour {
		t.Fatalf("wrong business day %s", got)
	}
	later := now.AddDate(0, 0, 10)
	owned, err = Grant(owned, []Benefit{{PlanGroup: "A", ValidityType: Duration, DurationDays: 1}}, ranks, later, loc)
	if err != nil || !owned[0].ExpiresAt.Equal(later.AddDate(0, 0, 1)) {
		t.Fatalf("expired renewal %+v %v", owned, err)
	}
}

// TestMissingRankCannotSilentlyDiscardAnEntitlement protects migration and malformed data.
func TestMissingRankCannotSilentlyDiscardAnEntitlement(t *testing.T) {
	_, err := Resolve([]Holding{{PlanGroup: "A", ValidityType: Permanent}}, map[string]int{}, time.Now())
	if !errors.Is(err, ErrGroupsNotReady) {
		t.Fatalf("missing rank accepted: %v", err)
	}
}

// TestManualAdjustmentTargetsOneGroup keeps permanent fallback while changing or revoking an override.
func TestManualAdjustmentTargetsOneGroup(t *testing.T) {
	deadline := time.Date(2026, 11, 4, 12, 0, 0, 0, time.UTC)
	owned := []Holding{{PlanGroup: "A", ValidityType: Permanent}, {PlanGroup: "B", ValidityType: Duration, ExpiresAt: &deadline}}
	next, err := Adjust(owned, Holding{PlanGroup: "B", ValidityType: Permanent}, false)
	if err != nil || len(next) != 2 || next[0].ValidityType != Permanent || next[1].ValidityType != Permanent {
		t.Fatalf("adjust %+v %v", next, err)
	}
	next, err = Adjust(next, Holding{PlanGroup: "B"}, true)
	if err != nil || len(next) != 1 || next[0].PlanGroup != "A" {
		t.Fatalf("revoke %+v %v", next, err)
	}
	if _, err = Adjust(owned, Holding{PlanGroup: "B", ValidityType: Duration}, false); err == nil {
		t.Fatal("accepted missing deadline")
	}
}
