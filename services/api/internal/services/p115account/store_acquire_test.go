package p115account

import (
	"context"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/konghang/ember/backend/internal/models"
)

// newAcquireStoreForTest opens the real GORM store over SQL fakes so the
// emitted statements (lock or not, transaction or not) are asserted exactly.
func newAcquireStoreForTest(t *testing.T) (*gormAccountStore, sqlmock.Sqlmock) {
	t.Helper()
	raw, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = raw.Close() })
	database, err := gorm.Open(postgres.New(postgres.Config{Conn: raw}), &gorm.Config{DisableAutomaticPing: true, Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	return &gormAccountStore{db: database}, mock
}

const (
	plainAccountSelect  = `SELECT .*FROM "p115_accounts" WHERE .*LIMIT \$\d+$`
	lockedAccountSelect = `SELECT .*FROM "p115_accounts" WHERE .*FOR UPDATE$`
)

// TestAcquireRuntimeActiveAccountSkipsRowLock keeps the hot active path off
// the global source row lock: one plain read, no transaction.
func TestAcquireRuntimeActiveAccountSkipsRowLock(t *testing.T) {
	now := time.Date(2026, 11, 19, 0, 0, 0, 0, time.UTC)
	store, mock := newAcquireStoreForTest(t)
	mock.ExpectQuery(plainAccountSelect).WillReturnRows(
		sqlmock.NewRows([]string{"id", "role", "status", "enabled"}).AddRow("source-1", "source", "active", true))

	account, err := store.AcquireRuntimeByRole(context.Background(), models.P115AccountRoleSource, now, now.Add(time.Minute))
	if err != nil || account == nil || account.ID != "source-1" {
		t.Fatalf("AcquireRuntimeByRole() account=%+v error=%v", account, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

// TestAcquireRuntimeExpiredCooldownStillLeasesProbeUnderLock keeps half-open
// probes serialized by the row lock after the unlocked read saw cooldown.
func TestAcquireRuntimeExpiredCooldownStillLeasesProbeUnderLock(t *testing.T) {
	now := time.Date(2026, 11, 19, 0, 0, 0, 0, time.UTC)
	expired := now.Add(-time.Second)
	updatedAt := now.Add(-time.Hour)
	store, mock := newAcquireStoreForTest(t)
	columns := []string{"id", "role", "status", "enabled", "cooldown_until", "updated_at"}
	mock.ExpectQuery(plainAccountSelect).WillReturnRows(
		sqlmock.NewRows(columns).AddRow("source-1", "source", "cooling_down", true, expired, updatedAt))
	mock.ExpectBegin()
	mock.ExpectQuery(lockedAccountSelect).WillReturnRows(
		sqlmock.NewRows(columns).AddRow("source-1", "source", "cooling_down", true, expired, updatedAt))
	mock.ExpectExec(`UPDATE "p115_accounts" SET .*cooldown_until`).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(plainAccountSelect).WillReturnRows(
		sqlmock.NewRows(columns).AddRow("source-1", "source", "cooling_down", true, now.Add(time.Minute), now))
	mock.ExpectCommit()

	account, err := store.AcquireRuntimeByRole(context.Background(), models.P115AccountRoleSource, now, now.Add(time.Minute))
	if err != nil || account == nil || account.CooldownUntil == nil || !account.CooldownUntil.Equal(now.Add(time.Minute)) {
		t.Fatalf("AcquireRuntimeByRole() account=%+v error=%v", account, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

// TestAcquirePlaybackRouteActiveAccountSkipsRowLock applies the same fast path
// to the shared/personal playback row selected by route and config version.
func TestAcquirePlaybackRouteActiveAccountSkipsRowLock(t *testing.T) {
	now := time.Date(2026, 11, 19, 0, 0, 0, 0, time.UTC)
	store, mock := newAcquireStoreForTest(t)
	mock.ExpectQuery(plainAccountSelect).WillReturnRows(
		sqlmock.NewRows([]string{"id", "role", "status", "enabled", "config_version"}).AddRow("playback-1", "playback", "active", true, 3))

	account, err := store.AcquirePlaybackRoute(context.Background(), PlaybackRoute{AccountID: "playback-1", ConfigVersion: 3}, now, now.Add(time.Minute))
	if err != nil || account == nil || account.ID != "playback-1" {
		t.Fatalf("AcquirePlaybackRoute() account=%+v error=%v", account, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
