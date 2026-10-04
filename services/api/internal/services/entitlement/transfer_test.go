package entitlement

import (
	"errors"
	"reflect"
	"testing"
	"time"
)

// TestTransferPreservesTerms locks the administrative rename semantics without rank or clock decisions.
func TestTransferPreservesTerms(t *testing.T) {
	past := time.Date(2020, 1, 1, 0, 0, 0, 123, time.UTC)
	for _, deadline := range []*time.Time{nil, &past} {
		kind := Duration
		if deadline == nil {
			kind = Permanent
		}
		before := []Holding{{PlanGroup: "FIRST", ValidityType: kind, ExpiresAt: deadline}, {PlanGroup: "OTHER", ValidityType: Permanent}}
		after, err := Transfer(before, "FIRST", "DEFAULT")
		if err != nil {
			t.Fatal(err)
		}
		if len(after) != 2 || after[0].PlanGroup != "DEFAULT" || after[0].ValidityType != kind || after[0].ExpiresAt != deadline || !reflect.DeepEqual(after[1], before[1]) || before[0].PlanGroup != "FIRST" {
			t.Fatalf("terms changed: %+v", after)
		}
	}
	for _, tc := range []struct {
		source, target string
		want           error
	}{
		{"FIRST", "FIRST", ErrInvalidHolding}, {"", "DEFAULT", ErrInvalidHolding},
		{"MISSING", "DEFAULT", ErrTransferSourceMissing}, {"FIRST", "OTHER", ErrTransferTargetExists},
	} {
		_, err := Transfer([]Holding{{PlanGroup: "FIRST", ValidityType: Permanent}, {PlanGroup: "OTHER", ValidityType: Permanent}}, tc.source, tc.target)
		if !errors.Is(err, tc.want) {
			t.Fatalf("%s -> %s error=%v", tc.source, tc.target, err)
		}
	}
}
