package auth

import (
	"errors"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	configpkg "github.com/konghang/ember/backend/internal/config"
	dbpkg "github.com/konghang/ember/backend/internal/db"
	embyint "github.com/konghang/ember/backend/internal/integrations/emby"
	"github.com/konghang/ember/backend/internal/models"
	redemptionpkg "github.com/konghang/ember/backend/internal/services/redemption"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// TestPermanentRegistrationTransaction protects permanent grant persistence and atomic invite consumption.
func TestPermanentRegistrationTransaction(t *testing.T) {
	for _, consumed := range []bool{false, true} {
		t.Run(map[bool]string{false: "exhausted_code_rolls_back", true: "permanent_history_before_commit"}[consumed], func(t *testing.T) {
			sqlDB, mock, err := sqlmock.New()
			if err != nil {
				t.Fatal(err)
			}
			defer sqlDB.Close()
			database, err := gorm.Open(postgres.New(postgres.Config{Conn: sqlDB}), &gorm.Config{DisableAutomaticPing: true})
			if err != nil {
				t.Fatal(err)
			}
			previous := dbpkg.DB
			dbpkg.DB = database
			defer func() { dbpkg.DB = previous }()
			configpkg.InvalidateCachedSetting("CRON_TIMEZONE")
			defer configpkg.InvalidateCachedSetting("CRON_TIMEZONE")
			mock.ExpectQuery(`SELECT .*settings`).WillReturnRows(sqlmock.NewRows([]string{"key", "value"}).AddRow("CRON_TIMEZONE", "Asia/Shanghai"))
			mock.ExpectBegin()
			mock.ExpectExec(`INSERT INTO "users"`).WillReturnResult(sqlmock.NewResult(0, 1))
			mock.ExpectQuery(`SELECT .*entitlement_events`).WillReturnRows(sqlmock.NewRows([]string{"source_key"}))
			mock.ExpectQuery(`SELECT .*user_entitlements`).WillReturnRows(sqlmock.NewRows([]string{"plan_group"}))
			mock.ExpectQuery(`SELECT .*plan_groups`).WillReturnRows(sqlmock.NewRows([]string{"key", "entitlement_rank"}).AddRow("BASE", 1))
			mock.ExpectExec(`DELETE FROM "user_entitlements"`).WillReturnResult(sqlmock.NewResult(0, 0))
			mock.ExpectExec(`INSERT INTO "user_entitlements"`).WithArgs(sqlmock.AnyArg(), "BASE", "permanent", nil, sqlmock.AnyArg(), sqlmock.AnyArg()).WillReturnResult(sqlmock.NewResult(0, 1))
			mock.ExpectExec(`INSERT INTO "entitlement_events"`).WillReturnResult(sqlmock.NewResult(0, 1))
			mock.ExpectExec(`UPDATE "users" SET`).WithArgs(nil, "BASE", true, sqlmock.AnyArg(), sqlmock.AnyArg()).WillReturnResult(sqlmock.NewResult(0, 1))
			affected := int64(0)
			if consumed {
				affected = 1
			}
			mock.ExpectExec(`UPDATE "redemption_codes" SET`).WillReturnResult(sqlmock.NewResult(0, affected))
			if consumed {
				mock.ExpectExec(`INSERT INTO "redemptions"`).WithArgs("permanent", sqlmock.AnyArg(), sqlmock.AnyArg(), "fixture-code", 0, sqlmock.AnyArg()).WillReturnResult(sqlmock.NewResult(0, 1))
				// Stop before external Policy sync and verify a failed commit cannot report successful registration.
				mock.ExpectCommit().WillReturnError(errors.New("fixture commit failure"))
			} else {
				mock.ExpectRollback()
			}
			group := "BASE"
			service := &AuthService{emailService: &stubAuthEmailVerifier{}}
			user, _, err := service.persistRegisteredUser(&RegisterUserRequest{Username: "fixture", Password: "fixture-password", Code: "fixture-code"}, &registerPreparation{mode: "invite", validityType: "permanent", registrationPlanGroup: &group, redemptionCode: &models.RedemptionCode{ID: "fixture-code"}}, &embyint.EmbyUser{ID: "fixture-emby"})
			if err == nil || user != nil {
				t.Fatalf("failed transaction returned user=%+v err=%v", user, err)
			}
			if !consumed && !errors.Is(err, redemptionpkg.ErrRedemptionCodeInvalid) {
				t.Fatalf("got %v", err)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}
