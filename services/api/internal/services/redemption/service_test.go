package redemption

import (
	"errors"
	"fmt"
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
	mock.ExpectExec(`UPDATE "users" SET "expires_at"=\$1,"updated_at"=\$2 WHERE id = \$3`).
		WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), "user_1").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`INSERT INTO "redemptions"`).
		WithArgs(sqlmock.AnyArg(), "user_1", "renew-code", 10, sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(regexp.QuoteMeta(`UPDATE "redemption_codes" SET "used_count"="used_count" + 1 WHERE code = $1 AND "used_count" < "max_uses" AND ("expires_at" IS NULL OR "expires_at" > $2)`)).
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

	expectRedemptionRenewalPrefix(mock, time.Now().UTC().AddDate(0, 0, 7))
	mock.ExpectExec(`UPDATE "users" SET "expires_at"=\$1,"updated_at"=\$2 WHERE id = \$3`).
		WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), "user_1").
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
	}).AddRow(id, code, 1, 0, nil, days, "VIP", "", time.Now().UTC())
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
