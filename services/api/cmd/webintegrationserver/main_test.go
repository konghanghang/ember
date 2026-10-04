package main

import (
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/konghang/ember/backend/internal/models"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// TestSeedBoundUserHasEntitlement verifies the web fixture without starting a project server or contacting Emby.
func TestSeedBoundUserHasEntitlement(t *testing.T) {
	conn, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	database, err := gorm.Open(postgres.New(postgres.Config{Conn: conn}), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	var user *models.User
	var holding *models.UserEntitlement
	if err = database.Callback().Create().Before("gorm:create").Register("test:capture_fixture", func(tx *gorm.DB) {
		switch row := tx.Statement.Dest.(type) {
		case *models.User:
			user = row
		case *models.UserEntitlement:
			holding = row
		}
	}); err != nil {
		t.Fatal(err)
	}
	mock.ExpectBegin()
	mock.ExpectExec(`INSERT INTO "users"`).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`INSERT INTO "user_entitlements"`).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	if err = seedBoundUser(database); err != nil {
		t.Fatal(err)
	}
	if user == nil || !user.ResourceAccessGranted {
		t.Fatal("fixture user has no resource access")
	}
	if holding == nil || holding.UserID != user.ID || holding.PlanGroup != testVIPGroupKey || holding.ValidityType != "permanent" || holding.ExpiresAt != nil {
		t.Fatal("fixture entitlement does not match its access projection")
	}
	if err = mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
