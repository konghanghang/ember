package entitlement

import (
	"errors"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/konghang/ember/backend/internal/models"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// mockDatabase keeps all persistence tests isolated from PostgreSQL and external services.
func mockDatabase(t *testing.T) (*gorm.DB, sqlmock.Sqlmock) {
	t.Helper()
	conn, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	db, err := gorm.Open(postgres.New(postgres.Config{Conn: conn}), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	return db, mock
}

// TestGrantReplayDoesNotWrite verifies the idempotency receipt stops a duplicate delivery.
func TestGrantReplayDoesNotWrite(t *testing.T) {
	db, mock := mockDatabase(t)
	mock.ExpectQuery(`SELECT .*entitlement_events`).WithArgs("payment:1", 1).
		WillReturnRows(sqlmock.NewRows([]string{"source_key", "user_id"}).AddRow("payment:1", "u1"))
	if err := GrantLocked(db, &models.User{ID: "u1"}, nil, "payment:1", "payment", time.Now(), time.UTC); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

// TestReconcileReReadsRenewal proves the scan's stale expiry cannot revoke a concurrent renewal.
func TestReconcileReReadsRenewal(t *testing.T) {
	db, mock := mockDatabase(t)
	now := time.Date(2026, 10, 4, 0, 0, 0, 0, time.UTC)
	expires := now.Add(24 * time.Hour)
	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT .*users.*FOR UPDATE`).WithArgs("u1", 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "role", "plan_group", "expires_at", "resource_access_granted"}).AddRow("u1", "user", "B", expires, true))
	mock.ExpectQuery(`SELECT .*user_entitlements`).WithArgs("u1").
		WillReturnRows(sqlmock.NewRows([]string{"user_id", "plan_group", "validity_type", "expires_at"}).AddRow("u1", "B", Duration, expires))
	mock.ExpectQuery(`SELECT .*plan_groups`).WillReturnRows(sqlmock.NewRows([]string{"key", "entitlement_rank"}).AddRow("B", 20))
	mock.ExpectExec(`UPDATE "user_entitlements"`).WithArgs(now, sqlmock.AnyArg(), "u1", "B").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	user, changed, err := Reconcile(db, "u1", now)
	if err != nil || changed || !user.ResourceAccessGranted {
		t.Fatalf("renewal lost: %+v %t %v", user, changed, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

// TestReconcileFallsBackWithoutClearingManualBan verifies projection and transaction failures.
func TestReconcileFallsBackWithoutClearingManualBan(t *testing.T) {
	for _, fail := range []bool{false, true} {
		t.Run(map[bool]string{false: "success", true: "write_failure"}[fail], func(t *testing.T) {
			db, mock := mockDatabase(t)
			now := time.Date(2026, 10, 4, 0, 0, 0, 0, time.UTC)
			mock.ExpectBegin()
			mock.ExpectQuery(`SELECT .*users.*FOR UPDATE`).WithArgs("u1", 1).WillReturnRows(sqlmock.NewRows([]string{"id", "role", "plan_group", "expires_at", "resource_access_granted", "emby_access_disabled"}).AddRow("u1", "user", "B", now, true, true))
			mock.ExpectQuery(`SELECT .*user_entitlements`).WithArgs("u1").WillReturnRows(sqlmock.NewRows([]string{"user_id", "plan_group", "validity_type", "expires_at"}).AddRow("u1", "A", Permanent, nil).AddRow("u1", "B", Duration, now))
			mock.ExpectQuery(`SELECT .*plan_groups`).WillReturnRows(sqlmock.NewRows([]string{"key", "entitlement_rank"}).AddRow("A", 10).AddRow("B", 20))
			mock.ExpectExec(`UPDATE "user_entitlements"`).WithArgs(now, sqlmock.AnyArg(), "u1", "A").WillReturnResult(sqlmock.NewResult(0, 1))
			write := mock.ExpectExec(`UPDATE "users" SET "expires_at"=\$1,"plan_group"=\$2,"resource_access_granted"=\$3,"updated_at"=\$4 WHERE id = \$5`).WithArgs(nil, "A", true, sqlmock.AnyArg(), "u1")
			if fail {
				write.WillReturnError(errors.New("write failed"))
				mock.ExpectRollback()
			} else {
				write.WillReturnResult(sqlmock.NewResult(0, 1))
				mock.ExpectCommit()
			}
			user, changed, err := Reconcile(db, "u1", now)
			if fail && err == nil {
				t.Fatal("write failure ignored")
			}
			if !fail && (err != nil || !changed || *user.PlanGroup != "A" || !user.EmbyAccessDisabled || user.ExpiresAt != nil) {
				t.Fatalf("fallback %+v %t %v", user, changed, err)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

// TestCatalogRequiresExplicitRanksAndNestedLibraries protects the upgrade purchase gate.
func TestCatalogRequiresExplicitRanksAndNestedLibraries(t *testing.T) {
	for _, tc := range []struct {
		name    string
		rank    interface{}
		include bool
		valid   bool
	}{{"unconfigured", nil, true, false}, {"not_nested", 20, false, false}, {"nested", 20, true, true}} {
		t.Run(tc.name, func(t *testing.T) {
			db, mock := mockDatabase(t)
			mock.ExpectQuery(`SELECT .*plan_groups`).WillReturnRows(sqlmock.NewRows([]string{"key", "entitlement_rank"}).AddRow("A", 10).AddRow("B", tc.rank))
			libraries := sqlmock.NewRows([]string{"plan_group_key", "library_id"}).AddRow("A", "library-1").AddRow("B", "library-2")
			if tc.include {
				libraries.AddRow("B", "library-1")
			}
			mock.ExpectQuery(`SELECT .*plan_group_media_libraries`).WillReturnRows(libraries)
			err := ValidateCatalog(db)
			if (err == nil) != tc.valid {
				t.Fatalf("valid=%t err=%v", tc.valid, err)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}
