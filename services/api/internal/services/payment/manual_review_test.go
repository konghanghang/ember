package payment

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	dbpkg "github.com/konghang/ember/backend/internal/db"
	"github.com/konghang/ember/backend/internal/models"
)

// TestPaidOrderCoveredByPermanentAccessRequiresReview preserves real payment without regranting.
func TestPaidOrderCoveredByPermanentAccessRequiresReview(t *testing.T) {
	database, mock, cleanup := newPaymentSQLMockDB(t)
	defer cleanup()
	dbpkg.DB = database
	payment := paymentFixture("pay_covered", models.PaymentPending, time.Now())
	mock.ExpectBegin()
	expectPaymentFulfillmentRef(mock, payment)
	expectPaymentUserLock(mock, "user_1", "VIP_A", time.Now())
	expectPaymentLock(mock, payment)
	expectPaymentPlanRead(mock, payment.PlanID, "VIP_A")
	mock.ExpectQuery(`SELECT .*entitlement_events`).WillReturnRows(sqlmock.NewRows([]string{"source_key"}))
	mock.ExpectQuery(`SELECT .*user_entitlements`).WillReturnRows(sqlmock.NewRows([]string{"plan_group", "validity_type"}).AddRow("VIP_B", "permanent"))
	mock.ExpectQuery(`SELECT .*plan_groups`).WillReturnRows(sqlmock.NewRows([]string{"key", "entitlement_rank"}).AddRow("VIP_A", 10).AddRow("VIP_B", 20))
	mock.ExpectExec(`UPDATE "payments" SET "manual_review_reason"=\$1,"paid_at"=\$2,"status"=\$3,"stripe_payment_intent_id"=\$4,"updated_at"=\$5 WHERE id = \$6`).WithArgs("already_owned", sqlmock.AnyArg(), models.PaymentManualReview, "pi_covered", sqlmock.AnyArg(), payment.ID).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	service := &PaymentService{loadTimezone: func() *time.Location { return time.UTC }}
	if err := service.fulfillPayment(payment.StripeSessionID, "pi_covered", time.Now(), nil); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

// TestManualRefundRecordingIsIdempotent records external action without calling Stripe or granting access.
func TestManualRefundRecordingIsIdempotent(t *testing.T) {
	database, mock, cleanup := newPaymentSQLMockDB(t)
	defer cleanup()
	dbpkg.DB = database
	service := &PaymentService{loadTimezone: func() *time.Location { return time.UTC }}
	for _, status := range []models.PaymentStatus{models.PaymentManualReview, models.PaymentResolved} {
		mock.ExpectQuery(`SELECT "id","user_id" FROM "payments"`).WithArgs("pay_1", 1).WillReturnRows(sqlmock.NewRows([]string{"id", "user_id"}).AddRow("pay_1", "user_1"))
		mock.ExpectBegin()
		expectPaymentUserLock(mock, "user_1", "VIP_A", time.Now())
		mock.ExpectQuery(`SELECT .*payments.*FOR UPDATE`).WithArgs("pay_1", "user_1", 1).WillReturnRows(sqlmock.NewRows([]string{"id", "user_id", "status"}).AddRow("pay_1", "user_1", status))
		if status == models.PaymentManualReview {
			mock.ExpectExec(`UPDATE "payments" SET "resolution"=\$1,"resolution_note"=\$2,"resolved_at"=\$3,"resolved_by"=\$4,"status"=\$5,"updated_at"=\$6 WHERE id = \$7`).WithArgs("external_refund", "已核对退款", sqlmock.AnyArg(), "admin-1", models.PaymentResolved, sqlmock.AnyArg(), "pay_1").WillReturnResult(sqlmock.NewResult(0, 1))
		}
		mock.ExpectCommit()
		if err := service.ResolvePayment(context.Background(), "pay_1", "admin-1", ResolvePaymentRequest{Resolution: "external_refund", Note: "已核对退款"}); err != nil {
			t.Fatal(err)
		}
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

// TestRankChangeRollsBackWhenProjectionFails prevents ranks committing with only part of the users updated.
func TestRankChangeRollsBackWhenProjectionFails(t *testing.T) {
	database, mock, cleanup := newPaymentSQLMockDB(t)
	defer cleanup()
	dbpkg.DB = database
	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT .*users.*ORDER BY id FOR UPDATE`).WithArgs("user").WillReturnRows(sqlmock.NewRows([]string{"id", "role", "plan_group", "resource_access_granted"}).AddRow("user_1", "user", "A", false))
	mock.ExpectQuery(`SELECT .*plan_groups.*ORDER BY key FOR UPDATE`).WillReturnRows(sqlmock.NewRows([]string{"key", "entitlement_rank"}).AddRow("A", 10))
	mock.ExpectExec(`UPDATE "plan_groups"`).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`UPDATE "plan_groups"`).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(`SELECT .*plan_groups`).WillReturnRows(sqlmock.NewRows([]string{"key", "entitlement_rank"}).AddRow("A", 20))
	mock.ExpectQuery(`SELECT .*plan_group_media_libraries`).WillReturnRows(sqlmock.NewRows([]string{"plan_group_key", "library_id"}))
	mock.ExpectQuery(`SELECT .*plan_groups`).WillReturnRows(sqlmock.NewRows([]string{"key", "entitlement_rank"}).AddRow("A", 20))
	mock.ExpectQuery(`SELECT .*user_entitlements`).WithArgs("user_1").WillReturnRows(sqlmock.NewRows([]string{"plan_group", "validity_type"}).AddRow("A", "permanent"))
	mock.ExpectExec(`UPDATE "user_entitlements"`).WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), "user_1", "A").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`UPDATE "users"`).WillReturnError(errors.New("projection write failed"))
	mock.ExpectRollback()
	if err := (&PaymentService{}).SetEntitlementRanks(context.Background(), SetEntitlementRanksRequest{Ranks: map[string]int{"A": 20}}); err == nil {
		t.Fatal("partial rank change committed")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
