package payment

import (
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/konghang/ember/backend/internal/db"
)

// TestGetPlansResolvesBenefitNames locks the read boundary for migrated and renamed groups.
func TestGetPlansResolvesBenefitNames(t *testing.T) {
	database, mock, cleanup := newPaymentSQLMockDB(t)
	defer cleanup()
	db.DB = database
	mock.ExpectQuery(`SELECT count\(\*\) FROM "plans"`).WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectQuery(`SELECT plans\.\*,`).WillReturnRows(sqlmock.NewRows([]string{"id", "plan_group", "validity_type", "days", "planGroupName"}).AddRow("legacy", "BASE", "duration", 30, "基础组"))
	result, err := (&PaymentService{}).GetPlans(&GetPlansRequest{ShowAll: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Data) != 1 || result.Data[0].PlanGroupName != "基础组" || result.Data[0].ValidityType != "duration" || result.Data[0].Days != 30 {
		t.Fatalf("unexpected plan: %+v", result.Data)
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
			mock.ExpectQuery(`SELECT plans\.\*,`).WillReturnRows(sqlmock.NewRows([]string{"id", "plan_group", "validity_type", "days", "planGroupName"}).AddRow("legacy", "BASE", "duration", 30, "基础组"))
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
			if len(result) != 1 || result[0].PlanGroupName != "基础组" || result[0].Purchasable == owned {
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
	mock.ExpectQuery(`SELECT plans\.\*,`).WillReturnRows(sqlmock.NewRows([]string{"id", "plan_group", "validity_type", "days", "planGroupName"}).AddRow("legacy", "BASE", "permanent", 0, "基础组"))
	result, err := (&PaymentService{}).getPlanByID("legacy")
	if err != nil {
		t.Fatal(err)
	}
	if result.PlanGroupName != "基础组" {
		t.Fatalf("unexpected benefits: %+v", result.Plan)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
