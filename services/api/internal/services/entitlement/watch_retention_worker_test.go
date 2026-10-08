package entitlement

import (
	"context"
	"errors"
	"github.com/DATA-DOG/go-sqlmock"
	"github.com/konghang/ember/backend/internal/models"
	"testing"
	"time"
)

// expectRetentionSnapshot supplies current server-side state without any external calls.
func expectRetentionSnapshot(mock sqlmock.Sqlmock, at time.Time, enabled bool) {
	mock.ExpectQuery(`SELECT .*users`).WithArgs("u1", 1).WillReturnRows(sqlmock.NewRows([]string{"id", "role", "plan_group", "resource_access_granted", "emby_id"}).AddRow("u1", "user", "A", true, "emby"))
	mock.ExpectQuery(`SELECT .*plan_groups`).WithArgs("A", 1).WillReturnRows(sqlmock.NewRows([]string{"key", "watch_retention_enabled", "watch_retention_days", "watch_retention_min_minutes", "watch_retention_reset_at"}).AddRow("A", enabled, 30, 60, at))
	if enabled {
		mock.ExpectQuery(`SELECT .*user_entitlements`).WithArgs("u1", "A", 1).WillReturnRows(sqlmock.NewRows([]string{"user_id", "plan_group", "validity_type", "watch_retention_started_at"}).AddRow("u1", "A", "permanent", at))
	}
}

// TestRetentionQueryFailureNeverInvalidates ensures a missing/upstream-error result cannot become zero viewing.
func TestRetentionQueryFailureNeverInvalidates(t *testing.T) {
	db, mock := mockDatabase(t)
	start := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	mock.ExpectQuery(`SELECT .*users.*JOIN plan_groups`).WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("u1"))
	expectRetentionSnapshot(mock, start, true)
	calls := 0
	worker := WatchRetentionWorker{DB: db, Location: time.UTC, Query: func(context.Context, string, time.Time, time.Time, *time.Location) (int64, error) {
		calls++
		return 0, errors.New("missing data")
	}, Sync: func(string) error { t.Fatal("must not sync"); return nil }}
	if err := worker.Run(context.Background(), start.AddDate(0, 0, 31)); err == nil || calls != 1 {
		t.Fatalf("err=%v calls=%d", err, calls)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

// TestRetentionInvalidationAndRollback exercises final revalidation, audit and fallback as one transaction.
func TestRetentionInvalidationAndRollback(t *testing.T) {
	for _, fail := range []bool{false, true} {
		t.Run(map[bool]string{false: "fallback", true: "rollback"}[fail], func(t *testing.T) {
			db, mock := mockDatabase(t)
			start := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
			at := start.AddDate(0, 0, 31)
			old := &retentionSnapshot{User: models.User{ID: "u1", EmbyID: "emby"}, Group: models.PlanGroup{Key: "A", WatchRetentionDays: 30, WatchRetentionMinMinutes: 60, WatchRetentionResetAt: &start}, Holding: models.UserEntitlement{ValidityType: Permanent}, Start: start}
			mock.ExpectBegin()
			expectRetentionSnapshot(mock, start, true)
			mock.ExpectExec(`INSERT INTO "watch_retention_checks"`).WillReturnResult(sqlmock.NewResult(0, 1))
			write := mock.ExpectExec(`UPDATE "user_entitlements" SET "watch_retention_invalidated_at"`)
			if fail {
				write.WillReturnError(errors.New("write failed"))
				mock.ExpectRollback()
			} else {
				write.WillReturnResult(sqlmock.NewResult(0, 1))
				mock.ExpectQuery(`SELECT .*plan_groups`).WillReturnRows(sqlmock.NewRows([]string{"key", "entitlement_rank"}).AddRow("A", 20).AddRow("B", 10))
				mock.ExpectQuery(`SELECT .*user_entitlements`).WillReturnRows(sqlmock.NewRows([]string{"plan_group", "validity_type", "watch_retention_invalidated_at"}).AddRow("A", "permanent", at).AddRow("B", "permanent", nil))
				mock.ExpectExec(`UPDATE "user_entitlements" SET "watch_retention_started_at"`).WithArgs(at, sqlmock.AnyArg(), "u1", "B").WillReturnResult(sqlmock.NewResult(0, 1))
				mock.ExpectExec(`UPDATE "users"`).WithArgs(nil, "B", true, sqlmock.AnyArg(), "u1").WillReturnResult(sqlmock.NewResult(0, 1))
				mock.ExpectCommit()
			}
			worker := WatchRetentionWorker{DB: db, Location: time.UTC}
			changed, err := worker.apply(context.Background(), old, start, at, 0)
			if (err != nil) != fail || (!fail && !changed) {
				t.Fatalf("changed=%t err=%v", changed, err)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

// TestRetentionPolicyDisabledDuringQueryDiscardsResult protects a concurrent operator change.
func TestRetentionPolicyDisabledDuringQueryDiscardsResult(t *testing.T) {
	db, mock := mockDatabase(t)
	at := time.Now()
	mock.ExpectBegin()
	expectRetentionSnapshot(mock, at, false)
	mock.ExpectCommit()
	worker := WatchRetentionWorker{DB: db}
	changed, err := worker.apply(context.Background(), &retentionSnapshot{User: models.User{ID: "u1"}}, at, at, 0)
	if err != nil || changed {
		t.Fatalf("changed=%t err=%v", changed, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

// TestRetentionDelayedCronCannotPrecedeDisplayedFirstCheck covers sub-second switching near a cron boundary.
func TestRetentionDelayedCronCannotPrecedeDisplayedFirstCheck(t *testing.T) {
	db, mock := mockDatabase(t)
	start := time.Date(2026, 9, 1, 2, 0, 0, 50_000_000, time.UTC)
	mock.ExpectQuery(`SELECT .*users.*JOIN plan_groups`).WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("u1"))
	expectRetentionSnapshot(mock, start, true)
	worker := WatchRetentionWorker{DB: db, Location: time.UTC, Schedule: "0 2 * * *", Query: func(context.Context, string, time.Time, time.Time, *time.Location) (int64, error) {
		t.Fatal("ran before displayed first check")
		return 0, nil
	}}
	if err := worker.Run(context.Background(), start.AddDate(0, 0, 30).Add(50*time.Millisecond)); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
