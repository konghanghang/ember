package app

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/konghang/ember/backend/internal/models"
	entitlementpkg "github.com/konghang/ember/backend/internal/services/entitlement"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// TestIntegrationEntitlementMigrationAndFallback exercises real SQL only in the dedicated test harness.
func TestIntegrationEntitlementMigrationAndFallback(t *testing.T) {
	h := newIntegrationHarness(t)
	group := "DEFAULT"
	past := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	user := h.seedUser(t, models.User{Username: "entitlement_fixture", Email: "entitlement@example.com", PlanGroup: &group, ExpiresAt: &past})
	if err := h.database.Model(&models.PlanGroup{}).Where("key = ?", group).Update("entitlement_rank", 10).Error; err != nil {
		t.Fatal(err)
	}
	if err := h.database.Create(&models.PlanGroup{Key: "FULL", Name: "全资源", EntitlementRank: intRank(20)}).Error; err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 10, 4, 12, 0, 0, 0, time.UTC)
	grant := func(key string, benefits []entitlementpkg.Benefit) {
		t.Helper()
		if err := h.database.Transaction(func(tx *gorm.DB) error {
			var locked models.User
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", user.ID).First(&locked).Error; err != nil {
				return err
			}
			return entitlementpkg.GrantLocked(tx, &locked, benefits, key, "test:fixture", now, time.UTC)
		}); err != nil {
			t.Fatal(err)
		}
	}
	grant("fixture:permanent", []entitlementpkg.Benefit{{PlanGroup: group, ValidityType: entitlementpkg.Permanent}})
	grant("fixture:month", []entitlementpkg.Benefit{{PlanGroup: "FULL", ValidityType: entitlementpkg.Duration, DurationDays: 30}})
	grant("fixture:month", []entitlementpkg.Benefit{{PlanGroup: "FULL", ValidityType: entitlementpkg.Duration, DurationDays: 30}})
	var full models.UserEntitlement
	if err := h.database.Where("user_id = ? AND plan_group = ?", user.ID, "FULL").First(&full).Error; err != nil {
		t.Fatal(err)
	}
	if full.ExpiresAt == nil || !full.ExpiresAt.Equal(now.AddDate(0, 0, 30)) {
		t.Fatalf("duplicate grant: %+v", full)
	}
	current, changed, err := entitlementpkg.Reconcile(h.database, user.ID, now.AddDate(0, 0, 30))
	if err != nil || !changed || !current.ResourceAccessGranted || current.PlanGroup == nil || *current.PlanGroup != group || current.ExpiresAt != nil {
		t.Fatalf("fallback=%+v changed=%t err=%v", current, changed, err)
	}
	// Replaying the SQL must not erase grants or invalidate a code created after migration.
	code := models.RedemptionCode{Code: "fixture-entitlements", MaxUses: 1, DefaultDays: 30, RegistrationPlanGroup: group}
	if err := h.database.Create(&code).Error; err != nil {
		t.Fatal(err)
	}
	migration, err := os.ReadFile(filepath.Join(integrationMigrationsDir(t), "20261004_01_plan_group_entitlements.sql"))
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if err := h.database.Exec(string(migration)).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := h.database.Where("id = ?", code.ID).First(&code).Error; err != nil {
		t.Fatal(err)
	}
	if code.LegacyInvalidated {
		t.Fatal("migration invalidated a new code")
	}
	var count int64
	if err := h.database.Model(&models.UserEntitlement{}).Where("user_id = ?", user.ID).Count(&count).Error; err != nil || count != 2 {
		t.Fatalf("grants=%d err=%v", count, err)
	}
}

// intRank supplies an explicit test rank rather than inferring from sort order.
func intRank(value int) *int { return &value }

// TestIntegrationLegacyEntitlementBackfill preserves bans/history and invalidates only pre-cutover codes.
func TestIntegrationLegacyEntitlementBackfill(t *testing.T) {
	h := newIntegrationHarness(t)
	group := "DEFAULT"
	user := h.seedUser(t, models.User{Username: "legacy_entitlement", Email: "legacy-entitlement@example.com", PlanGroup: &group})
	code := models.RedemptionCode{Code: "legacy-entitlement", MaxUses: 3, UsedCount: 1, DefaultDays: 30, RegistrationPlanGroup: group}
	if err := h.database.Create(&code).Error; err != nil {
		t.Fatal(err)
	}
	history := models.Redemption{UserID: user.ID, Code: code.Code, Days: 30}
	if err := h.database.Create(&history).Error; err != nil {
		t.Fatal(err)
	}
	if err := h.database.Where("user_id = ?", user.ID).Delete(&models.UserEntitlement{}).Error; err != nil {
		t.Fatal(err)
	}
	// Reproduce only the old schema's missing markers inside the isolated harness.
	for _, sql := range []string{"ALTER TABLE redemption_codes DROP COLUMN legacy_invalidated", "ALTER TABLE users ALTER COLUMN resource_access_granted DROP NOT NULL"} {
		if err := h.database.Exec(sql).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := h.database.Model(&models.User{}).Where("id = ?", user.ID).Updates(map[string]interface{}{"resource_access_granted": nil, "expires_at": nil, "is_active": false, "emby_access_disabled": true}).Error; err != nil {
		t.Fatal(err)
	}
	migration, err := os.ReadFile(filepath.Join(integrationMigrationsDir(t), "20261004_01_plan_group_entitlements.sql"))
	if err != nil {
		t.Fatal(err)
	}
	if err := h.database.Exec(string(migration)).Error; err != nil {
		t.Fatal(err)
	}
	if err := h.database.Where("id = ?", user.ID).First(&user).Error; err != nil {
		t.Fatal(err)
	}
	if user.IsActive || !user.EmbyAccessDisabled {
		t.Fatal("migration cleared manual restrictions")
	}
	if err := h.database.Where("id = ?", code.ID).First(&code).Error; err != nil {
		t.Fatal(err)
	}
	if !code.LegacyInvalidated || code.UsedCount != 1 {
		t.Fatalf("legacy code changed incorrectly %+v", code)
	}
	var holding models.UserEntitlement
	if err := h.database.Where("user_id = ?", user.ID).First(&holding).Error; err != nil {
		t.Fatal(err)
	}
	if holding.ValidityType != entitlementpkg.Permanent || holding.ExpiresAt != nil {
		t.Fatalf("permanent legacy access lost %+v", holding)
	}
	if err := h.database.Where("user_id = ?", user.ID).Delete(&models.UserEntitlement{}).Error; err != nil {
		t.Fatal(err)
	}
	if err := h.database.Model(&models.User{}).Where("id = ?", user.ID).Update("resource_access_granted", false).Error; err != nil {
		t.Fatal(err)
	}
	if err := h.database.Exec(string(migration)).Error; err != nil {
		t.Fatal(err)
	}
	var count int64
	if err := h.database.Model(&models.UserEntitlement{}).Where("user_id = ?", user.ID).Count(&count).Error; err != nil || count != 0 {
		t.Fatalf("migration regranted revoked access count=%d err=%v", count, err)
	}
	if err := h.database.Where("id = ?", history.ID).First(&history).Error; err != nil {
		t.Fatal("redemption history lost:", err)
	}
}
