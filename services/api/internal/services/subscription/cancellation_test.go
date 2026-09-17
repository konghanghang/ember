package subscription

import (
	"errors"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/konghang/ember/backend/internal/models"
	"gorm.io/gorm"
)

// TestDeleteSubscriptionAtomicEligibility locks ownership and pending state into the write itself.
func TestDeleteSubscriptionAtomicEligibility(t *testing.T) {
	for _, tc := range []struct {
		name     string
		affected int64
		state    models.SubscriptionStatus
		missing  bool
		want     error
	}{
		{name: "pending cancelled", affected: 1},
		{name: "approval won", state: models.SubscriptionApproved, want: ErrSubscriptionStateConflict},
		{name: "rejection won", state: models.SubscriptionRejected, want: ErrSubscriptionStateConflict},
		{name: "already ingested", state: models.SubscriptionIngested, want: ErrSubscriptionStateConflict},
		{name: "missing or another owner", missing: true, want: ErrSubscriptionNotFound},
	} {
		t.Run(tc.name, func(t *testing.T) {
			database, mock, closeDB := newPendingRejectSQLMockDB(t)
			defer closeDB()
			withPendingRejectTestDB(t, database)
			expectPendingCancellation(mock, tc.affected)
			if tc.affected == 0 {
				rows := sqlmock.NewRows([]string{"id", "user_id", "status"})
				if !tc.missing {
					rows.AddRow("sub_1", "user_1", tc.state)
				}
				mock.ExpectQuery(`SELECT \* FROM "subscriptions" WHERE id = \$1 AND "user_id" = \$2`).WithArgs("sub_1", "user_1", 1).WillReturnRows(rows)
			}
			err := (&SubscriptionService{}).DeleteSubscription("sub_1", "user_1")
			if !errors.Is(err, tc.want) {
				t.Fatalf("got %v, want %v", err, tc.want)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

// TestCancellationWinsAfterReviewRead injects cancellation between the review read and conditional update.
// A losing review must return before dispatching MoviePilot or notification work.
func TestCancellationWinsAfterReviewRead(t *testing.T) {
	for _, review := range []string{"approve", "reject"} {
		t.Run(review, func(t *testing.T) {
			database, mock, closeDB := newPendingRejectSQLMockDB(t)
			defer closeDB()
			withPendingRejectTestDB(t, database)
			mock.ExpectQuery(`SELECT \* FROM "subscriptions" WHERE id = \$1`).WithArgs("sub_1", 1).WillReturnRows(sqlmock.NewRows([]string{"id", "user_id", "status"}).AddRow("sub_1", "user_1", models.SubscriptionPending))
			expectPendingCancellation(mock, 1)
			mock.ExpectBegin()
			mock.ExpectExec(`UPDATE "subscriptions" SET .* WHERE id = \$[0-9]+ AND status = \$[0-9]+`).WillReturnResult(sqlmock.NewResult(0, 0))
			mock.ExpectCommit()
			service := &SubscriptionService{}
			if err := database.Callback().Update().Before("gorm:begin_transaction").Register("test:cancel_before_review", func(tx *gorm.DB) {
				if err := service.DeleteSubscription("sub_1", "user_1"); err != nil {
					t.Errorf("cancellation failed: %v", err)
				}
			}); err != nil {
				t.Fatal(err)
			}
			var err error
			if review == "approve" {
				err = service.ApproveSubscription("sub_1")
			} else {
				err = service.RejectSubscription("sub_1", "reason")
			}
			if !errors.Is(err, ErrSubscriptionStateConflict) {
				t.Fatalf("got %v", err)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

// expectPendingCancellation rejects deletes that omit the owner or current-state predicate.
func expectPendingCancellation(mock sqlmock.Sqlmock, affected int64) {
	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta(`DELETE FROM "subscriptions" WHERE id = $1 AND "user_id" = $2 AND status = $3`)).WithArgs("sub_1", "user_1", models.SubscriptionPending).WillReturnResult(sqlmock.NewResult(0, affected))
	mock.ExpectCommit()
}

// TestDeleteSubscriptionDatabaseErrors preserves storage failures instead of concealing them as 404.
func TestDeleteSubscriptionDatabaseErrors(t *testing.T) {
	for _, stage := range []string{"delete", "lookup"} {
		t.Run(stage, func(t *testing.T) {
			database, mock, closeDB := newPendingRejectSQLMockDB(t)
			defer closeDB()
			withPendingRejectTestDB(t, database)
			failure := errors.New("database unavailable")
			if stage == "delete" {
				mock.ExpectBegin()
				mock.ExpectExec(`DELETE FROM "subscriptions"`).WillReturnError(failure)
				mock.ExpectRollback()
			} else {
				expectPendingCancellation(mock, 0)
				mock.ExpectQuery(`SELECT \* FROM "subscriptions"`).WithArgs("sub_1", "user_1", 1).WillReturnError(failure)
			}
			err := (&SubscriptionService{}).DeleteSubscription("sub_1", "user_1")
			if !errors.Is(err, failure) {
				t.Fatalf("got %v, want wrapped storage failure", err)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

// TestAdminDeleteSubscriptionPreservesUnrestrictedDeletion protects the separate administrator contract.
func TestAdminDeleteSubscriptionPreservesUnrestrictedDeletion(t *testing.T) {
	database, mock, closeDB := newPendingRejectSQLMockDB(t)
	defer closeDB()
	withPendingRejectTestDB(t, database)
	mock.ExpectQuery(`SELECT \* FROM "subscriptions" WHERE id = \$1`).WithArgs("sub_1", 1).WillReturnRows(sqlmock.NewRows([]string{"id", "user_id", "status"}).AddRow("sub_1", "another_user", models.SubscriptionApproved))
	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta(`DELETE FROM "subscriptions" WHERE "subscriptions"."id" = $1`)).WithArgs("sub_1").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	if err := (&SubscriptionService{}).DeleteSubscriptionAsAdmin("sub_1"); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
