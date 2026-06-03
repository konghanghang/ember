package policy

import (
	"strings"
	"testing"

	"github.com/konghang/ember/backend/internal/models"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestInsertUserMediaLibraryPreferenceTxIncludesDisabledFlag(t *testing.T) {
	database, err := gorm.Open(postgres.New(postgres.Config{
		DSN:                  "host=127.0.0.1 user=test dbname=test sslmode=disable",
		PreferSimpleProtocol: true,
	}), &gorm.Config{
		DryRun:               true,
		DisableAutomaticPing: true,
	})
	if err != nil {
		t.Fatalf("open dry-run database: %v", err)
	}

	preference := models.UserMediaLibraryPreference{
		UserID:    "user_1",
		LibraryID: "lib_disabled",
		Enabled:   false,
	}
	sql := database.ToSQL(func(tx *gorm.DB) *gorm.DB {
		return insertUserMediaLibraryPreferenceTx(tx, &preference)
	})
	sql = strings.Join(strings.Fields(sql), " ")

	assertSQLContains(t, sql, `"enabled"`)
	assertSQLContains(t, sql, "false")
}
