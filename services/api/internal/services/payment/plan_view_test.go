package payment

import (
	"errors"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/konghang/ember/backend/internal/db"
	"github.com/konghang/ember/backend/internal/models"
)

// TestGetPlansResolvesBenefitNames locks the read boundary for migrated and renamed groups.
func TestGetPlansResolvesBenefitNames(t *testing.T) {
	database, mock, cleanup := newPaymentSQLMockDB(t)
	defer cleanup()
	db.DB = database
	mock.ExpectQuery(`SELECT count\(\*\) FROM "plans"`).WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectQuery(`SELECT plans\.\*,`).WillReturnRows(sqlmock.NewRows([]string{"id", "benefits"}).AddRow("legacy", `[{"planGroup":"BASE","validityType":"permanent"},{"planGroup":"PLUS","planGroupName":"旧名称","validityType":"duration","durationDays":30}]`))
	mock.ExpectQuery(`SELECT .* FROM "plan_groups"`).WillReturnRows(sqlmock.NewRows([]string{"key", "name"}).AddRow("BASE", "基础组").AddRow("PLUS", "增强组"))
	result, err := (&PaymentService{}).GetPlans(&GetPlansRequest{ShowAll: true})
	if err != nil {
		t.Fatal(err)
	}
	benefits := result.Data[0].Benefits
	if benefits[0].PlanGroupName != "基础组" || benefits[1].PlanGroupName != "增强组" || benefits[1].DurationDays != 30 {
		t.Fatalf("unexpected benefits: %+v", benefits)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

// TestPopulatePlanBenefitNames checks read-only enrichment without mutating shared snapshots.
func TestPopulatePlanBenefitNames(t *testing.T) {
	database, mock, cleanup := newPaymentSQLMockDB(t)
	defer cleanup()
	snapshot := []models.PlanBenefit{{PlanGroup: "BASE", ValidityType: "permanent"}, {PlanGroup: "MISSING", PlanGroupName: "历史名称", ValidityType: "duration", DurationDays: 60}}
	plans := []PlanView{{Plan: models.Plan{Benefits: snapshot}}}
	mock.ExpectQuery(`SELECT .* FROM "plan_groups"`).WillReturnRows(sqlmock.NewRows([]string{"key", "name"}).AddRow("BASE", "基础组"))
	if err := populatePlanBenefitNames(database, plans); err != nil {
		t.Fatal(err)
	}
	if plans[0].Benefits[0].PlanGroupName != "基础组" || plans[0].Benefits[1].PlanGroupName != "历史名称" {
		t.Fatalf("unexpected names: %+v", plans)
	}
	if snapshot[0].PlanGroupName != "" {
		t.Fatal("mutated shared snapshot")
	}
	if err := populatePlanBenefitNames(database, nil); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

// TestPopulatePlanBenefitNamesFailure surfaces query errors instead of disguising failed enrichment.
func TestPopulatePlanBenefitNamesFailure(t *testing.T) {
	database, mock, cleanup := newPaymentSQLMockDB(t)
	defer cleanup()
	want := errors.New("lookup failed")
	mock.ExpectQuery(`SELECT .* FROM "plan_groups"`).WillReturnError(want)
	plans := []PlanView{{Plan: models.Plan{Benefits: []models.PlanBenefit{{PlanGroup: "BASE"}}}}}
	if err := populatePlanBenefitNames(database, plans); !errors.Is(err, want) {
		t.Fatalf("error = %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

// TestGetPlansForUserResolvesNames preserves per-user eligibility while enriching the shared catalog.
func TestGetPlansForUserResolvesNames(t *testing.T) {
	for _, owned := range []bool{false, true} {
		t.Run(map[bool]string{false: "new_user", true: "permanent_owner"}[owned], func(t *testing.T) {
			database, mock, cleanup := newPaymentSQLMockDB(t)
			defer cleanup()
			db.DB = database
			mock.ExpectQuery(`SELECT plans\.\*,`).WillReturnRows(sqlmock.NewRows([]string{"id", "benefits"}).AddRow("legacy", `[{"planGroup":"BASE","validityType":"duration","durationDays":30}]`))
			mock.ExpectQuery(`SELECT .* FROM "plan_groups"`).WillReturnRows(sqlmock.NewRows([]string{"key", "name"}).AddRow("BASE", "基础组"))
			holdings := sqlmock.NewRows([]string{"user_id", "plan_group", "validity_type"})
			if owned {
				holdings.AddRow("user_1", "BASE", "permanent")
			}
			mock.ExpectQuery(`SELECT .* FROM "user_entitlements"`).WithArgs("user_1").WillReturnRows(holdings)
			mock.ExpectQuery(`SELECT .* FROM "plan_groups"`).WillReturnRows(sqlmock.NewRows([]string{"key", "entitlement_rank"}).AddRow("BASE", 1))
			mock.ExpectQuery(`SELECT .* FROM "plan_groups"`).WillReturnRows(sqlmock.NewRows([]string{"key", "entitlement_rank"}).AddRow("BASE", 1))
			mock.ExpectQuery(`SELECT .* FROM "plan_group_media_libraries"`).WillReturnRows(sqlmock.NewRows([]string{"plan_group_key", "library_id"}))
			location, err := time.LoadLocation("Asia/Shanghai")
			if err != nil {
				t.Fatal(err)
			}
			result, err := (&PaymentService{loadTimezone: func() *time.Location { return location }}).GetPlansForUser("user_1")
			if err != nil {
				t.Fatal(err)
			}
			if len(result) != 1 || result[0].Benefits[0].PlanGroupName != "基础组" || result[0].Purchasable == owned {
				t.Fatalf("unexpected catalog: %+v", result)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

// TestGetPlanByIDResolvesNames covers the response shared by plan creation and editing.
func TestGetPlanByIDResolvesNames(t *testing.T) {
	database, mock, cleanup := newPaymentSQLMockDB(t)
	defer cleanup()
	db.DB = database
	mock.ExpectQuery(`SELECT plans\.\*,`).WillReturnRows(sqlmock.NewRows([]string{"id", "benefits"}).AddRow("legacy", `[{"planGroup":"BASE","validityType":"permanent"}]`))
	mock.ExpectQuery(`SELECT .* FROM "plan_groups"`).WillReturnRows(sqlmock.NewRows([]string{"key", "name"}).AddRow("BASE", "基础组"))
	result, err := (&PaymentService{}).getPlanByID("legacy")
	if err != nil {
		t.Fatal(err)
	}
	if result.Benefits[0].PlanGroupName != "基础组" {
		t.Fatalf("unexpected benefits: %+v", result.Benefits)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
