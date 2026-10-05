package redemption

import (
	"github.com/DATA-DOG/go-sqlmock"
	"github.com/konghang/ember/backend/internal/db"
	"testing"
)

// TestCreatePermanentCodesKeepsZeroDays guards against GORM replacing zero with the old 30-day default.
func TestCreatePermanentCodesKeepsZeroDays(t *testing.T) {
	database, mock, cleanup := newRedemptionSQLMockDB(t)
	defer cleanup()
	db.DB = database
	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT .*plan_groups.*FOR UPDATE`).WillReturnRows(sqlmock.NewRows([]string{"key", "name"}).AddRow("BASE", "基础"))
	for i := 0; i < 2; i++ {
		mock.ExpectExec(`INSERT INTO "redemption_codes"`).WithArgs("permanent", false, sqlmock.AnyArg(), sqlmock.AnyArg(), 1, 0, nil, 0, "BASE", "", sqlmock.AnyArg()).WillReturnResult(sqlmock.NewResult(0, 1))
	}
	mock.ExpectCommit()
	mock.ExpectQuery(`SELECT .*plan_groups`).WillReturnRows(sqlmock.NewRows([]string{"key", "name"}).AddRow("BASE", "基础"))
	result, err := (&RedemptionCodeService{}).CreateRedemptionCodesBatch(&CreateRedemptionCodesBatchRequest{Count: 2, RedemptionCodeCreateOptions: RedemptionCodeCreateOptions{MaxUses: 1, ValidityType: "permanent", DefaultDays: 0, RegistrationPlanGroup: "BASE"}})
	if err != nil {
		t.Fatal(err)
	}
	for _, code := range result.Data {
		if code.ValidityType != "permanent" || code.DefaultDays != 0 {
			t.Fatalf("bad code: %+v", code)
		}
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

// TestUpdatePermanentCodeKeepsHistory checks the update writes the explicit zero without rewriting past redemptions.
func TestUpdatePermanentCodeKeepsHistory(t *testing.T) {
	database, mock, cleanup := newRedemptionSQLMockDB(t)
	defer cleanup()
	db.DB = database
	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT .*redemption_codes`).WillReturnRows(sqlmock.NewRows([]string{"id", "code", "max_uses", "used_count", "default_days", "validity_type", "registration_plan_group"}).AddRow("code1", "fixture", 2, 1, 30, "duration", "BASE"))
	mock.ExpectQuery(`SELECT .*plan_groups.*FOR UPDATE`).WillReturnRows(sqlmock.NewRows([]string{"key", "name"}).AddRow("BASE", "基础"))
	mock.ExpectExec(`UPDATE "redemption_codes" SET "default_days"=\$1,"expires_at"=\$2,"max_uses"=\$3,"notes"=\$4,"registration_plan_group"=\$5,"validity_type"=\$6 WHERE id = \$7`).WithArgs(0, nil, 2, "", "BASE", "permanent", "code1").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	mock.ExpectQuery(`SELECT .*plan_groups`).WillReturnRows(sqlmock.NewRows([]string{"key", "name"}).AddRow("BASE", "基础"))
	result, err := (&RedemptionCodeService{}).UpdateRedemptionCode("code1", &UpdateRedemptionCodeRequest{MaxUses: 2, ValidityType: "permanent", RegistrationPlanGroup: "BASE"})
	if err != nil {
		t.Fatal(err)
	}
	if result.ValidityType != "permanent" || result.DefaultDays != 0 {
		t.Fatalf("bad code: %+v", result)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
