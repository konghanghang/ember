package app

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/konghang/ember/backend/internal/models"
	redemptionpkg "github.com/konghang/ember/backend/internal/services/redemption"
)

// TestIntegrationRedemptionPermanentValidity verifies persistence, independent code expiry, and migration replay.
func TestIntegrationRedemptionPermanentValidity(t *testing.T) {
	h := newIntegrationHarness(t)
	if err := h.database.Model(&models.PlanGroup{}).Where("key = ?", "DEFAULT").Update("entitlement_rank", 10).Error; err != nil {
		t.Fatal(err)
	}
	expired := time.Now().AddDate(0, 0, -1)
	user := h.seedUser(t, models.User{Username: "permanent_code_fixture", Email: "permanent@example.com", ExpiresAt: &expired})
	deadline := time.Now().AddDate(0, 0, 2)
	code, err := (&redemptionpkg.RedemptionCodeService{}).CreateRedemptionCode(&redemptionpkg.CreateRedemptionCodeRequest{RedemptionCodeCreateOptions: redemptionpkg.RedemptionCodeCreateOptions{MaxUses: 2, RegistrationPlanGroup: "DEFAULT", ValidityType: "permanent", DefaultDays: 0, ExpiresAt: &deadline}})
	if err != nil {
		t.Fatal(err)
	}
	result, err := (&redemptionpkg.RedemptionService{}).RedeemCode(user.ID, &redemptionpkg.RedeemCodeRequest{Code: code.Code})
	if err != nil {
		t.Fatal(err)
	}
	if result.ValidityType != "permanent" || result.ExpiresAt != nil {
		t.Fatalf("bad grant %+v", result)
	}
	var holding models.UserEntitlement
	if err := h.database.Where("user_id = ?", user.ID).First(&holding).Error; err != nil {
		t.Fatal(err)
	}
	if holding.ValidityType != "permanent" || holding.ExpiresAt != nil {
		t.Fatalf("bad holding %+v", holding)
	}
	if _, err := (&redemptionpkg.RedemptionService{}).RedeemCode(user.ID, &redemptionpkg.RedeemCodeRequest{Code: code.Code}); !errors.Is(err, redemptionpkg.ErrRedemptionDuplicate) {
		t.Fatalf("duplicate: %v", err)
	}
	// Editing the remaining uses must not rewrite the redeemed validity snapshot.
	if _, err := (&redemptionpkg.RedemptionCodeService{}).UpdateRedemptionCode(code.ID, &redemptionpkg.UpdateRedemptionCodeRequest{MaxUses: 2, RegistrationPlanGroup: "DEFAULT", ValidityType: "duration", DefaultDays: 30}); err != nil {
		t.Fatal(err)
	}
	migration, err := os.ReadFile(filepath.Join(integrationMigrationsDir(t), "20261005_01_redemption_permanent_validity.sql"))
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if err := h.database.Exec(string(migration)).Error; err != nil {
			t.Fatal(err)
		}
	}
	var history models.Redemption
	if err := h.database.Where("user_id = ?", user.ID).First(&history).Error; err != nil {
		t.Fatal(err)
	}
	if history.ValidityType != "permanent" || history.Days != 0 {
		t.Fatalf("history changed: %+v", history)
	}
	var stored models.RedemptionCode
	if err := h.database.First(&stored, "id = ?", code.ID).Error; err != nil {
		t.Fatal(err)
	}
	if stored.UsedCount != 1 || stored.ValidityType != "duration" || stored.DefaultDays != 30 {
		t.Fatalf("replay changed code: %+v", stored)
	}
	for _, kind := range []string{"duration", "permanent", "invalid"} {
		days := 0
		if kind == "permanent" {
			days = 10
		}
		invalid := models.RedemptionCode{Code: "invalid-" + kind, RegistrationPlanGroup: "DEFAULT", ValidityType: kind, DefaultDays: days, MaxUses: 1}
		if err := h.database.Create(&invalid).Error; err == nil {
			t.Fatalf("constraint accepted %+v", invalid)
		}
	}
}

// TestIntegrationRedemptionValidityUpgrade verifies legacy rows are backfilled without changing usage or invalidation.
func TestIntegrationRedemptionValidityUpgrade(t *testing.T) {
	h := newIntegrationHarness(t)
	// Recreate the pre-migration column shape only inside this test's isolated schema.
	for _, sql := range []string{
		`ALTER TABLE redemption_codes DROP COLUMN validity_type CASCADE`,
		`ALTER TABLE redemptions DROP COLUMN validity_type CASCADE`,
		`INSERT INTO redemption_codes(id,code,max_uses,used_count,default_days,registration_plan_group,legacy_invalidated) VALUES ('legacy-code','legacy-fixture',3,1,45,'DEFAULT',true)`,
	} {
		if err := h.database.Exec(sql).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := h.database.Exec(`INSERT INTO redemptions(id,user_id,code,days) VALUES ('legacy-history',?,'legacy-fixture',45)`, h.adminUser.ID).Error; err != nil {
		t.Fatal(err)
	}
	migration, err := os.ReadFile(filepath.Join(integrationMigrationsDir(t), "20261005_01_redemption_permanent_validity.sql"))
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if err := h.database.Exec(string(migration)).Error; err != nil {
			t.Fatal(err)
		}
	}
	var code models.RedemptionCode
	if err := h.database.First(&code, "id = ?", "legacy-code").Error; err != nil {
		t.Fatal(err)
	}
	if code.ValidityType != "duration" || code.DefaultDays != 45 || code.UsedCount != 1 || !code.LegacyInvalidated {
		t.Fatalf("legacy code changed: %+v", code)
	}
	var history models.Redemption
	if err := h.database.First(&history, "id = ?", "legacy-history").Error; err != nil {
		t.Fatal(err)
	}
	if history.ValidityType != "duration" || history.Days != 45 {
		t.Fatalf("legacy history changed: %+v", history)
	}
}
