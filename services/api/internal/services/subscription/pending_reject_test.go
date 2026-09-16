package subscription

import (
	"context"
	"errors"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	dbpkg "github.com/konghang/ember/backend/internal/db"
	"github.com/konghang/ember/backend/internal/models"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestPendingRejectCompleteCommitsAndRetainsContext(t *testing.T) {
	database, mock, closeDB := newPendingRejectSQLMockDB(t)
	defer closeDB()
	withPendingRejectTestDB(t, database)
	restoreClock := setPendingRejectClock(t, time.Date(2026, 9, 16, 12, 0, 0, 0, time.UTC))
	defer restoreClock()

	notifications := 0
	originalNotify := runSubscriptionRejectedNotifications
	runSubscriptionRejectedNotifications = func(_ *SubscriptionService, subscription models.Subscription) {
		notifications++
		if subscription.ID != "sub_1" || subscription.Status != models.SubscriptionRejected ||
			subscription.RejectReason == nil || *subscription.RejectReason != "资源不合适" {
			t.Fatalf("unexpected notification subscription: %+v", subscription)
		}
	}
	t.Cleanup(func() { runSubscriptionRejectedNotifications = originalNotify })

	mock.ExpectBegin()
	expectPendingRejectRow(mock, "pending_1", 2002, "admin_1", "sub_1")
	expectSubscriptionRow(mock, "sub_1", models.SubscriptionPending, nil)
	mock.ExpectExec(regexp.QuoteMeta(`UPDATE "subscriptions"`)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	result, err := (&SubscriptionService{}).CompletePendingReject(context.Background(), "pending_1", 2002, "admin_1", "资源不合适")
	if err != nil {
		t.Fatalf("CompletePendingReject() error = %v", err)
	}
	if result.SubscriptionID != "sub_1" || result.Status != models.SubscriptionRejected || !result.Changed || result.RejectReason != "资源不合适" {
		t.Fatalf("unexpected result: %+v", result)
	}
	if notifications != 1 {
		t.Fatalf("expected one notification after commit, got %d", notifications)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations: %v", err)
	}
}

func TestPendingRejectCompleteRollsBackOnUpdateFailure(t *testing.T) {
	database, mock, closeDB := newPendingRejectSQLMockDB(t)
	defer closeDB()
	withPendingRejectTestDB(t, database)
	restoreClock := setPendingRejectClock(t, time.Date(2026, 9, 16, 12, 0, 0, 0, time.UTC))
	defer restoreClock()

	originalNotify := runSubscriptionRejectedNotifications
	runSubscriptionRejectedNotifications = func(_ *SubscriptionService, subscription models.Subscription) {
		t.Fatalf("notification must not run after rollback: %+v", subscription)
	}
	t.Cleanup(func() { runSubscriptionRejectedNotifications = originalNotify })

	mock.ExpectBegin()
	expectPendingRejectRow(mock, "pending_1", 2002, "admin_1", "sub_1")
	expectSubscriptionRow(mock, "sub_1", models.SubscriptionPending, nil)
	mock.ExpectExec(regexp.QuoteMeta(`UPDATE "subscriptions"`)).
		WillReturnError(errors.New("write failed"))
	mock.ExpectRollback()

	_, err := (&SubscriptionService{}).CompletePendingReject(context.Background(), "pending_1", 2002, "admin_1", "资源不合适")
	if err == nil {
		t.Fatal("expected update failure")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations: %v", err)
	}
}

func TestPendingRejectCompleteDoesNotNotifyOnCommitFailure(t *testing.T) {
	database, mock, closeDB := newPendingRejectSQLMockDB(t)
	defer closeDB()
	withPendingRejectTestDB(t, database)
	restoreClock := setPendingRejectClock(t, time.Date(2026, 9, 16, 12, 0, 0, 0, time.UTC))
	defer restoreClock()

	originalNotify := runSubscriptionRejectedNotifications
	runSubscriptionRejectedNotifications = func(_ *SubscriptionService, subscription models.Subscription) {
		t.Fatalf("notification must not run when commit fails: %+v", subscription)
	}
	t.Cleanup(func() { runSubscriptionRejectedNotifications = originalNotify })

	mock.ExpectBegin()
	expectPendingRejectRow(mock, "pending_1", 2002, "admin_1", "sub_1")
	expectSubscriptionRow(mock, "sub_1", models.SubscriptionPending, nil)
	mock.ExpectExec(regexp.QuoteMeta(`UPDATE "subscriptions"`)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit().WillReturnError(errors.New("commit failed"))

	_, err := (&SubscriptionService{}).CompletePendingReject(context.Background(), "pending_1", 2002, "admin_1", "资源不合适")
	if err == nil {
		t.Fatal("expected commit failure")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations: %v", err)
	}
}

func TestPendingRejectCompleteReplaysRejectedWithoutUpdateOrNotify(t *testing.T) {
	database, mock, closeDB := newPendingRejectSQLMockDB(t)
	defer closeDB()
	withPendingRejectTestDB(t, database)
	restoreClock := setPendingRejectClock(t, time.Date(2026, 9, 16, 12, 0, 0, 0, time.UTC))
	defer restoreClock()

	originalNotify := runSubscriptionRejectedNotifications
	runSubscriptionRejectedNotifications = func(_ *SubscriptionService, subscription models.Subscription) {
		t.Fatalf("notification must not repeat for already rejected subscription: %+v", subscription)
	}
	t.Cleanup(func() { runSubscriptionRejectedNotifications = originalNotify })

	reason := "已有原因"
	mock.ExpectBegin()
	expectPendingRejectRow(mock, "pending_1", 2002, "admin_1", "sub_1")
	expectSubscriptionRow(mock, "sub_1", models.SubscriptionRejected, &reason)
	mock.ExpectCommit()

	result, err := (&SubscriptionService{}).CompletePendingReject(context.Background(), "pending_1", 2002, "admin_1", "新的原因")
	if err != nil {
		t.Fatalf("CompletePendingReject() error = %v", err)
	}
	if result.SubscriptionID != "sub_1" || result.Status != models.SubscriptionRejected || result.Changed || result.RejectReason != reason {
		t.Fatalf("unexpected replay result: %+v", result)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations: %v", err)
	}
}

func TestPendingRejectCompleteReturnsFinalApprovedWithoutRejecting(t *testing.T) {
	database, mock, closeDB := newPendingRejectSQLMockDB(t)
	defer closeDB()
	withPendingRejectTestDB(t, database)
	restoreClock := setPendingRejectClock(t, time.Date(2026, 9, 16, 12, 0, 0, 0, time.UTC))
	defer restoreClock()

	originalNotify := runSubscriptionRejectedNotifications
	runSubscriptionRejectedNotifications = func(_ *SubscriptionService, subscription models.Subscription) {
		t.Fatalf("notification must not run for approved final state: %+v", subscription)
	}
	t.Cleanup(func() { runSubscriptionRejectedNotifications = originalNotify })

	mock.ExpectBegin()
	expectPendingRejectRow(mock, "pending_1", 2002, "admin_1", "sub_1")
	expectSubscriptionRow(mock, "sub_1", models.SubscriptionApproved, nil)
	mock.ExpectCommit()

	result, err := (&SubscriptionService{}).CompletePendingReject(context.Background(), "pending_1", 2002, "admin_1", "资源不合适")
	if err != nil {
		t.Fatalf("CompletePendingReject() error = %v", err)
	}
	if result.SubscriptionID != "sub_1" || result.Status != models.SubscriptionApproved || result.Changed || result.RejectReason != "" {
		t.Fatalf("unexpected final-state result: %+v", result)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations: %v", err)
	}
}

func TestPendingRejectCompleteRollsBackWhenRequestExpiresAfterLocks(t *testing.T) {
	database, mock, closeDB := newPendingRejectSQLMockDB(t)
	defer closeDB()
	withPendingRejectTestDB(t, database)
	frozenNow := time.Date(2026, 9, 16, 12, 2, 0, 0, time.UTC)
	restoreClock := setPendingRejectClock(t, frozenNow)
	defer restoreClock()

	originalNotify := runSubscriptionRejectedNotifications
	runSubscriptionRejectedNotifications = func(_ *SubscriptionService, subscription models.Subscription) {
		t.Fatalf("notification must not run after pending request expires: %+v", subscription)
	}
	t.Cleanup(func() { runSubscriptionRejectedNotifications = originalNotify })

	mock.ExpectBegin()
	expectPendingRejectRowWithExpiry(mock, "pending_1", 2002, "admin_1", "sub_1", frozenNow.Add(-time.Minute))
	expectSubscriptionRow(mock, "sub_1", models.SubscriptionPending, nil)
	mock.ExpectRollback()

	_, err := (&SubscriptionService{}).CompletePendingReject(context.Background(), "pending_1", 2002, "admin_1", "资源不合适")
	if !errors.Is(err, ErrPendingRejectNotFound) {
		t.Fatalf("expected ErrPendingRejectNotFound, got %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations: %v", err)
	}
}

func TestPendingRejectCompleteReturnsNotFoundForExpiredFinalState(t *testing.T) {
	database, mock, closeDB := newPendingRejectSQLMockDB(t)
	defer closeDB()
	withPendingRejectTestDB(t, database)
	frozenNow := time.Date(2026, 9, 16, 12, 2, 0, 0, time.UTC)
	restoreClock := setPendingRejectClock(t, frozenNow)
	defer restoreClock()

	originalNotify := runSubscriptionRejectedNotifications
	runSubscriptionRejectedNotifications = func(_ *SubscriptionService, subscription models.Subscription) {
		t.Fatalf("notification must not repeat for expired final state: %+v", subscription)
	}
	t.Cleanup(func() { runSubscriptionRejectedNotifications = originalNotify })

	reason := "已有原因"
	mock.ExpectBegin()
	expectPendingRejectRowWithExpiry(mock, "pending_1", 2002, "admin_1", "sub_1", frozenNow.Add(-time.Minute))
	expectSubscriptionRow(mock, "sub_1", models.SubscriptionRejected, &reason)
	mock.ExpectRollback()

	_, err := (&SubscriptionService{}).CompletePendingReject(context.Background(), "pending_1", 2002, "admin_1", "新的原因")
	if !errors.Is(err, ErrPendingRejectNotFound) {
		t.Fatalf("expected ErrPendingRejectNotFound, got %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations: %v", err)
	}
}

func TestPendingRejectCompleteRollsBackMissingRequest(t *testing.T) {
	database, mock, closeDB := newPendingRejectSQLMockDB(t)
	defer closeDB()
	withPendingRejectTestDB(t, database)
	restoreClock := setPendingRejectClock(t, time.Date(2026, 9, 16, 12, 0, 0, 0, time.UTC))
	defer restoreClock()

	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT \* FROM "bot_pending_reject_requests".*FOR UPDATE`).
		WithArgs("pending_1", int64(2002), "admin_1", 1).
		WillReturnRows(sqlmock.NewRows(pendingRejectColumns()))
	mock.ExpectRollback()

	_, err := (&SubscriptionService{}).CompletePendingReject(context.Background(), "pending_1", 2002, "admin_1", "资源不合适")
	if !errors.Is(err, ErrPendingRejectNotFound) {
		t.Fatalf("expected ErrPendingRejectNotFound, got %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations: %v", err)
	}
}

func newPendingRejectSQLMockDB(t *testing.T) (*gorm.DB, sqlmock.Sqlmock, func()) {
	t.Helper()
	sqlDB, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock.New(): %v", err)
	}
	database, err := gorm.Open(postgres.New(postgres.Config{Conn: sqlDB, PreferSimpleProtocol: true}), &gorm.Config{
		NowFunc: func() time.Time { return time.Now().UTC() },
	})
	if err != nil {
		_ = sqlDB.Close()
		t.Fatalf("gorm.Open(): %v", err)
	}
	return database, mock, func() { _ = sqlDB.Close() }
}

func withPendingRejectTestDB(t *testing.T, database *gorm.DB) {
	t.Helper()
	previousDB := dbpkg.DB
	dbpkg.DB = database
	t.Cleanup(func() { dbpkg.DB = previousDB })
}

func setPendingRejectClock(t *testing.T, now time.Time) func() {
	t.Helper()
	previousNow := subscriptionNow
	subscriptionNow = func() time.Time { return now }
	return func() { subscriptionNow = previousNow }
}

func expectPendingRejectRow(mock sqlmock.Sqlmock, id string, chatID int64, adminUserID, subscriptionID string) {
	expectPendingRejectRowWithExpiry(mock, id, chatID, adminUserID, subscriptionID, subscriptionNow().Add(time.Minute))
}

func expectPendingRejectRowWithExpiry(mock sqlmock.Sqlmock, id string, chatID int64, adminUserID, subscriptionID string, expiresAt time.Time) {
	mock.ExpectQuery(`SELECT \* FROM "bot_pending_reject_requests".*FOR UPDATE`).
		WithArgs(id, chatID, adminUserID, 1).
		WillReturnRows(sqlmock.NewRows(pendingRejectColumns()).AddRow(
			id,
			chatID,
			adminUserID,
			subscriptionID,
			nil,
			false,
			"原始消息",
			subscriptionNow().Add(-time.Minute),
			expiresAt,
		))
}

func expectSubscriptionRow(mock sqlmock.Sqlmock, id string, status models.SubscriptionStatus, rejectReason *string) {
	mock.ExpectQuery(`SELECT \* FROM "subscriptions".*FOR UPDATE`).
		WithArgs(id, 1).
		WillReturnRows(sqlmock.NewRows(subscriptionColumns()).AddRow(
			id,
			"user_1",
			models.MediaMovie,
			"测试片名",
			"100",
			0,
			nil,
			status,
			"",
			nil,
			rejectReason,
			nil,
			nil,
			nil,
			nil,
			nil,
			subscriptionNow().Add(-time.Minute),
			subscriptionNow(),
		))
}

func pendingRejectColumns() []string {
	return []string{
		"id",
		"chat_id",
		"admin_user_id",
		"subscription_id",
		"message_id",
		"has_photo",
		"original_text",
		"created_at",
		"expires_at",
	}
}

func subscriptionColumns() []string {
	return []string{
		"id",
		"user_id",
		"type",
		"name",
		"tmdb_id",
		"season",
		"poster_path",
		"status",
		"note",
		"mp_error",
		"reject_reason",
		"review_source",
		"retry_from_id",
		"ingest_progress",
		"reviewed_at",
		"ingested_at",
		"created_at",
		"updated_at",
	}
}
