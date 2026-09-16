package telegram

import (
	"context"
	"database/sql/driver"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	dbpkg "github.com/konghang/ember/backend/internal/db"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestPeekPendingRejectReadsLatestContextWithoutDeleting(t *testing.T) {
	database, mock, closeDB := newTelegramPendingRejectSQLMockDB(t)
	defer closeDB()
	previousDB := dbpkg.DB
	dbpkg.DB = database
	t.Cleanup(func() { dbpkg.DB = previousDB })

	expiresAt := time.Date(2026, 9, 16, 12, 5, 0, 0, time.UTC)
	messageID := int64(77)
	mock.ExpectQuery(`SELECT \* FROM "bot_pending_reject_requests" WHERE "chat_id" = \$1 AND "admin_user_id" = \$2 AND "expires_at" > \$3 ORDER BY "created_at" DESC,"bot_pending_reject_requests"\."id" LIMIT \$4`).
		WithArgs(int64(2002), "admin_1", anySQLTime{}, 1).
		WillReturnRows(sqlmock.NewRows([]string{
			"id",
			"chat_id",
			"admin_user_id",
			"subscription_id",
			"message_id",
			"has_photo",
			"original_text",
			"created_at",
			"expires_at",
		}).AddRow(
			"pending_newest_terminal",
			int64(2002),
			"admin_1",
			"sub_rejected",
			messageID,
			true,
			"终态订阅对应的最新 context",
			expiresAt.Add(-time.Minute),
			expiresAt,
		))

	record, err := PeekPendingReject(context.Background(), 2002, "admin_1")
	if err != nil {
		t.Fatalf("PeekPendingReject() error = %v", err)
	}
	if record == nil || record.ID != "pending_newest_terminal" || record.SubscriptionID != "sub_rejected" ||
		record.MessageID == nil || *record.MessageID != messageID {
		t.Fatalf("unexpected pending reject record: %+v", record)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations: %v", err)
	}
}

func newTelegramPendingRejectSQLMockDB(t *testing.T) (*gorm.DB, sqlmock.Sqlmock, func()) {
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

type anySQLTime struct{}

func (anySQLTime) Match(v driver.Value) bool {
	_, ok := v.(time.Time)
	return ok
}
