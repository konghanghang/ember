package redemption

import (
	"errors"
	"fmt"
	configpkg "github.com/konghang/ember/backend/internal/config"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	dbpkg "github.com/konghang/ember/backend/internal/db"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestRedeemCodeUsesInjectedStoreWithTrimmedCode(t *testing.T) {
	expiresAt := time.Date(2026, 7, 17, 10, 0, 0, 0, time.UTC)
	var capturedUserID string
	var capturedCode string
	service := &RedemptionService{
		redeemCodeStore: func(userID string, req *RedeemCodeRequest) (*RedeemCodeResponse, error) {
			capturedUserID = userID
			capturedCode = req.Code
			return &RedeemCodeResponse{
				Message:   "兑换成功，有效期已延长 30 天",
				Days:      30,
				ExpiresAt: &expiresAt,
			}, nil
		},
	}

	resp, err := service.RedeemCode("user_1", &RedeemCodeRequest{Code: "  invite-code  "})

	if err != nil {
		t.Fatalf("expected redeem success, got %v", err)
	}
	if capturedUserID != "user_1" || capturedCode != "invite-code" {
		t.Fatalf("unexpected store args: userID=%s code=%s", capturedUserID, capturedCode)
	}
	if resp == nil || resp.Days != 30 || resp.ExpiresAt == nil || !resp.ExpiresAt.Equal(expiresAt) {
		t.Fatalf("unexpected redeem response: %+v", resp)
	}
}

func TestRedeemCodePropagatesInjectedStoreError(t *testing.T) {
	service := &RedemptionService{
		redeemCodeStore: func(userID string, req *RedeemCodeRequest) (*RedeemCodeResponse, error) {
			return nil, ErrRedemptionCodeInvalid
		},
	}

	resp, err := service.RedeemCode("user_1", &RedeemCodeRequest{Code: "bad-code"})

	if !errors.Is(err, ErrRedemptionCodeInvalid) {
		t.Fatalf("expected ErrRedemptionCodeInvalid, got resp=%+v err=%v", resp, err)
	}
	if resp != nil {
		t.Fatalf("expected nil response on redeem failure, got %+v", resp)
	}
}

func TestCalculateRedeemedExpiryStartsFromNowWithoutActiveExpiry(t *testing.T) {
	now := time.Date(2026, 6, 17, 10, 0, 0, 0, time.UTC)
	expiredAt := now.Add(-2 * time.Hour)

	tests := []struct {
		name          string
		currentExpiry *time.Time
	}{
		{name: "nil expiry", currentExpiry: nil},
		{name: "expired expiry", currentExpiry: &expiredAt},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := calculateRedeemedExpiry(now, tt.currentExpiry, 30)
			want := now.AddDate(0, 0, 30)

			if !got.Equal(want) {
				t.Fatalf("expected expiry from now %s, got %s", want, got)
			}
		})
	}
}

func TestCalculateRedeemedExpiryExtendsActiveExpiry(t *testing.T) {
	now := time.Date(2026, 6, 17, 10, 0, 0, 0, time.UTC)
	currentExpiry := now.AddDate(0, 0, 12)

	got := calculateRedeemedExpiry(now, &currentExpiry, 45)
	want := currentExpiry.AddDate(0, 0, 45)

	if !got.Equal(want) {
		t.Fatalf("expected active expiry extension %s, got %s", want, got)
	}
}

func TestRedeemCodeWithDBLocksUserAndCommitsRenewal(t *testing.T) {
	database, mock, cleanup := newRedemptionSQLMockDB(t)
	defer cleanup()
	dbpkg.DB = database

	now := time.Now().UTC()
	currentExpiry := now.AddDate(0, 0, 7)
	expectRedemptionRenewalPrefix(mock, currentExpiry)
	expectRenewalGrant(mock, currentExpiry, 10)
	mock.ExpectExec(`UPDATE "users" SET "expires_at"=\$1,"plan_group"=\$2,"resource_access_granted"=\$3,"updated_at"=\$4 WHERE id = \$5`).
		WithArgs(sqlmock.AnyArg(), "VIP_A", true, sqlmock.AnyArg(), "user_1").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(`SELECT .*user_entitlements`).WithArgs("user_1").WillReturnRows(sqlmock.NewRows([]string{"plan_group", "validity_type", "expires_at"}).AddRow("VIP_A", "duration", currentExpiry.AddDate(0, 0, 10)))
	mock.ExpectExec(`INSERT INTO "redemptions"`).
		WithArgs("duration", sqlmock.AnyArg(), "user_1", "renew-code", 10, sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(regexp.QuoteMeta(`UPDATE "redemption_codes" SET "used_count"="used_count" + 1 WHERE code = $1 AND NOT legacy_invalidated AND "used_count" < "max_uses" AND ("expires_at" IS NULL OR "expires_at" > $2)`)).
		WithArgs("renew-code", sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	resp, err := (&RedemptionService{}).RedeemCode("user_1", &RedeemCodeRequest{Code: " renew-code "})
	if err != nil {
		t.Fatalf("RedeemCode(): %v", err)
	}
	if resp == nil || resp.ExpiresAt == nil || !resp.ExpiresAt.After(currentExpiry) {
		t.Fatalf("expected extended expiry from locked user row, got %+v", resp)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations: %v", err)
	}
}

func TestRedeemCodeWithDBRollsBackWhenUserUpdateFails(t *testing.T) {
	database, mock, cleanup := newRedemptionSQLMockDB(t)
	defer cleanup()
	dbpkg.DB = database

	expiry := time.Now().UTC().AddDate(0, 0, 7)
	expectRedemptionRenewalPrefix(mock, expiry)
	expectRenewalGrant(mock, expiry, 10)
	mock.ExpectExec(`UPDATE "users" SET "expires_at"=\$1,"plan_group"=\$2,"resource_access_granted"=\$3,"updated_at"=\$4 WHERE id = \$5`).
		WithArgs(sqlmock.AnyArg(), "VIP_A", true, sqlmock.AnyArg(), "user_1").
		WillReturnError(errors.New("update failed"))
	mock.ExpectRollback()

	resp, err := (&RedemptionService{}).RedeemCode("user_1", &RedeemCodeRequest{Code: "renew-code"})
	if !errors.Is(err, ErrRedeemFailed) {
		t.Fatalf("expected ErrRedeemFailed, got resp=%+v err=%v", resp, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations: %v", err)
	}
}

func newRedemptionSQLMockDB(t *testing.T) (*gorm.DB, sqlmock.Sqlmock, func()) {
	configpkg.InvalidateCachedSetting("CRON_TIMEZONE")
	t.Cleanup(func() { configpkg.InvalidateCachedSetting("CRON_TIMEZONE") })
	t.Helper()
	sqlDB, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock.New(): %v", err)
	}
	database, err := gorm.Open(postgres.New(postgres.Config{Conn: sqlDB}), &gorm.Config{DisableAutomaticPing: true})
	if err != nil {
		_ = sqlDB.Close()
		t.Fatalf("gorm.Open(): %v", err)
	}
	previous := dbpkg.DB
	return database, mock, func() {
		dbpkg.DB = previous
		_ = sqlDB.Close()
	}
}

func expectRedemptionRenewalPrefix(mock sqlmock.Sqlmock, currentExpiry time.Time) {
	mock.ExpectQuery(`SELECT .*settings`).WillReturnRows(sqlmock.NewRows([]string{"key", "value"}).AddRow("CRON_TIMEZONE", "Asia/Shanghai"))
	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT \* FROM "redemption_codes" WHERE code = \$1 ORDER BY "redemption_codes"\."id" LIMIT \$2`).
		WithArgs("renew-code", 1).
		WillReturnRows(redemptionCodeRows("code_1", "renew-code", 10))
	mock.ExpectQuery(`SELECT \* FROM "redemptions" WHERE "user_id" = \$1 AND code = \$2 ORDER BY "redemptions"\."id" LIMIT \$3`).
		WithArgs("user_1", "renew-code", 1).
		WillReturnRows(sqlmock.NewRows(redemptionColumns()))
	mock.ExpectQuery(`SELECT \* FROM "users" WHERE id = \$1 ORDER BY "users"\."id" LIMIT \$2 FOR UPDATE`).
		WithArgs("user_1", 1).
		WillReturnRows(redemptionUserRows("user_1", currentExpiry))
}

func redemptionCodeRows(id, code string, days int) *sqlmock.Rows {
	return sqlmock.NewRows([]string{
		"id", "code", "max_uses", "used_count", "expires_at", "default_days", "registration_plan_group", "notes", "created_at",
	}).AddRow(id, code, 1, 0, nil, days, "VIP_A", "", time.Now().UTC())
}

func redemptionColumns() []string {
	return []string{"id", "user_id", "code", "days", "created_at"}
}

func redemptionUserRows(id string, expiresAt time.Time) *sqlmock.Rows {
	return sqlmock.NewRows([]string{
		"id", "username", "role", "password", "email", "emby_id", "emby_disabled", "emby_access_disabled", "telegram_id",
		"plan_group", "applied_media_library_template_version", "expires_at", "is_active", "password_reset_required", "created_at", "updated_at",
	}).AddRow(id, fmt.Sprintf("%s_name", id), "user", "", fmt.Sprintf("%s@example.com", id), "", false, false, nil, nil, int64(1), expiresAt, true, false, time.Now().UTC(), time.Now().UTC())
}

// expectRenewalGrant verifies the selected group's expiry is persisted with an audit before projecting the user.
func expectRenewalGrant(mock sqlmock.Sqlmock, expiry time.Time, days int) {
	mock.ExpectQuery(`SELECT .*entitlement_events`).WillReturnRows(sqlmock.NewRows([]string{"source_key"}))
	mock.ExpectQuery(`SELECT .*user_entitlements`).WithArgs("user_1").WillReturnRows(sqlmock.NewRows([]string{"user_id", "plan_group", "validity_type", "expires_at"}).AddRow("user_1", "VIP_A", "duration", expiry))
	mock.ExpectQuery(`SELECT .*plan_groups`).WillReturnRows(sqlmock.NewRows([]string{"key", "entitlement_rank"}).AddRow("VIP_A", 10))
	mock.ExpectExec(`DELETE FROM "user_entitlements"`).WithArgs("user_1").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`INSERT INTO "user_entitlements"`).WithArgs("user_1", "VIP_A", "duration", expiry.AddDate(0, 0, days), sqlmock.AnyArg(), sqlmock.AnyArg()).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`INSERT INTO "entitlement_events"`).WillReturnResult(sqlmock.NewResult(0, 1))
}

// TestPermanentRedemptionPersistsGrantAndHistory exercises the same transaction as duration redemption.
func TestPermanentRedemptionPersistsGrantAndHistory(t *testing.T) {
	database, mock, cleanup := newRedemptionSQLMockDB(t)
	defer cleanup()
	dbpkg.DB = database
	mock.ExpectQuery(`SELECT .*settings`).WillReturnRows(sqlmock.NewRows([]string{"key", "value"}).AddRow("CRON_TIMEZONE", "Asia/Shanghai"))
	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT .*redemption_codes`).WillReturnRows(sqlmock.NewRows([]string{"id", "code", "max_uses", "used_count", "validity_type", "default_days", "registration_plan_group"}).AddRow("code_1", "permanent-fixture", 2, 0, "permanent", 0, "VIP_A"))
	mock.ExpectQuery(`SELECT .*redemptions`).WillReturnRows(sqlmock.NewRows(redemptionColumns()))
	mock.ExpectQuery(`SELECT .*users.*FOR UPDATE`).WillReturnRows(redemptionUserRows("user_1", time.Now().AddDate(0, 0, 7)))
	mock.ExpectQuery(`SELECT .*entitlement_events`).WillReturnRows(sqlmock.NewRows([]string{"source_key"}))
	mock.ExpectQuery(`SELECT .*user_entitlements`).WillReturnRows(sqlmock.NewRows([]string{"plan_group", "validity_type", "expires_at"}).AddRow("VIP_A", "duration", time.Now().AddDate(0, 0, 7)))
	mock.ExpectQuery(`SELECT .*plan_groups`).WillReturnRows(sqlmock.NewRows([]string{"key", "entitlement_rank"}).AddRow("VIP_A", 10))
	mock.ExpectExec(`DELETE FROM "user_entitlements"`).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`INSERT INTO "user_entitlements"`).WithArgs("user_1", "VIP_A", "permanent", nil, sqlmock.AnyArg(), sqlmock.AnyArg()).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`INSERT INTO "entitlement_events"`).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`UPDATE "users" SET`).WithArgs(nil, "VIP_A", true, sqlmock.AnyArg(), "user_1").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(`SELECT .*user_entitlements`).WillReturnRows(sqlmock.NewRows([]string{"plan_group", "validity_type", "expires_at"}).AddRow("VIP_A", "permanent", nil))
	mock.ExpectExec(`INSERT INTO "redemptions"`).WithArgs("permanent", sqlmock.AnyArg(), "user_1", "permanent-fixture", 0, sqlmock.AnyArg()).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`UPDATE "redemption_codes" SET`).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	result, err := (&RedemptionService{}).RedeemCode("user_1", &RedeemCodeRequest{Code: "permanent-fixture"})
	if err != nil {
		t.Fatal(err)
	}
	if result.ValidityType != "permanent" || result.Days != 0 || result.ExpiresAt != nil || result.Message != "兑换成功，已获得对应分组的永久权益" {
		t.Fatalf("unexpected result: %+v", result)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

// TestPermanentCodeDuplicateDoesNotGrantOrConsume preserves one-use-per-user for permanent codes.
func TestPermanentCodeDuplicateDoesNotGrantOrConsume(t *testing.T) {
	database, mock, cleanup := newRedemptionSQLMockDB(t)
	defer cleanup()
	dbpkg.DB = database
	mock.ExpectQuery(`SELECT .*settings`).WillReturnRows(sqlmock.NewRows([]string{"key", "value"}).AddRow("CRON_TIMEZONE", "Asia/Shanghai"))
	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT .*redemption_codes`).WillReturnRows(sqlmock.NewRows([]string{"id", "code", "max_uses", "used_count", "validity_type", "default_days", "registration_plan_group"}).AddRow("code_1", "permanent-fixture", 2, 1, "permanent", 0, "VIP_A"))
	mock.ExpectQuery(`SELECT .*redemptions`).WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("used"))
	mock.ExpectRollback()
	_, err := (&RedemptionService{}).RedeemCode("user_1", &RedeemCodeRequest{Code: "permanent-fixture"})
	if !errors.Is(err, ErrRedemptionDuplicate) {
		t.Fatalf("got %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

// TestAdminRedemptionHistoryIncludesValidity ensures the joined history query returns the saved type.
func TestAdminRedemptionHistoryIncludesValidity(t *testing.T) {
	database, mock, cleanup := newRedemptionSQLMockDB(t)
	defer cleanup()
	dbpkg.DB = database
	mock.ExpectQuery(`SELECT count\(\*\) FROM redemptions`).WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectQuery(`SELECT .*r.validity_type.*FROM redemptions`).WillReturnRows(sqlmock.NewRows([]string{"id", "user_id", "code", "validity_type", "days", "username"}).AddRow("history1", "user_1", "fixture", "permanent", 0, "fixture"))
	result, err := (&RedemptionService{}).GetAllRedemptions(&GetAllRedemptionsRequest{Page: 1, PageSize: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Data) != 1 || result.Data[0].ValidityType != "permanent" || result.Data[0].Days != 0 {
		t.Fatalf("bad history %+v", result)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
