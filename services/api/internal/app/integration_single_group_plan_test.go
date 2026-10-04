package app

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/konghang/ember/backend/internal/models"
	"gorm.io/gorm"
)

// TestIntegrationSingleGroupPlanAPI exercises the real binding, service and SQL constraints without external traffic.
func TestIntegrationSingleGroupPlanAPI(t *testing.T) {
	h := newIntegrationHarness(t)
	for _, body := range []string{
		`{"name":"bad","price":100,"planGroup":"DEFAULT","validityType":"duration","days":0}`,
		`{"name":"bad","price":100,"planGroup":"DEFAULT","days":30}`,
		`{"name":"bad","price":100,"planGroup":"DEFAULT","validityType":"permanent","benefits":[]}`,
	} {
		r := h.performAdminRequest(http.MethodPost, "/api/v1/admin/plans", []byte(body))
		if r.Code != http.StatusBadRequest {
			t.Fatalf("invalid create status=%d body=%s", r.Code, r.Body.String())
		}
	}
	r := h.performAdminRequest(http.MethodPost, "/api/v1/admin/plans", []byte(`{"name":"single","price":100,"planGroup":"DEFAULT","validityType":"permanent","days":30}`))
	if r.Code != http.StatusCreated && r.Code != http.StatusOK {
		t.Fatalf("create status=%d body=%s", r.Code, r.Body.String())
	}
	var plan models.Plan
	if err := h.database.Where("name = ?", "single").First(&plan).Error; err != nil {
		t.Fatal(err)
	}
	if plan.ValidityType != "permanent" || plan.Days != 0 {
		t.Fatalf("plan=%+v", plan)
	}
	r = h.performAdminRequest(http.MethodPut, "/api/v1/admin/plans/"+plan.ID, []byte(`{"validityType":"duration","days":60}`))
	if r.Code != http.StatusOK {
		t.Fatalf("update status=%d body=%s", r.Code, r.Body.String())
	}
	var response map[string]json.RawMessage
	if err := json.Unmarshal(r.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if _, exists := response["benefits"]; exists {
		t.Fatal("obsolete product field exposed")
	}
	if err := h.database.First(&plan, "id = ?", plan.ID).Error; err != nil {
		t.Fatal(err)
	}
	if plan.ValidityType != "duration" || plan.Days != 60 {
		t.Fatalf("plan=%+v", plan)
	}
	r = h.performAdminRequest(http.MethodPut, "/api/v1/admin/plans/"+plan.ID, []byte(`{"benefits":null}`))
	if r.Code != http.StatusBadRequest {
		t.Fatalf("old update status=%d", r.Code)
	}
}

// TestIntegrationSingleGroupPlanMigration checks conversion and fail-closed handling of unpublished combination products.
func TestIntegrationSingleGroupPlanMigration(t *testing.T) {
	for _, scenario := range []string{"legacy_duration", "old_permanent", "combination_rejected"} {
		t.Run(scenario, func(t *testing.T) {
			h := newIntegrationHarness(t)
			p := seedBillingIntegrationPlan(t, h, "single-migration", "DEFAULT", 30)
			if err := h.database.Exec("ALTER TABLE plans DROP COLUMN validity_type CASCADE").Error; err != nil {
				t.Fatal(err)
			}
			if scenario != "legacy_duration" {
				if err := h.database.Exec("ALTER TABLE plans ADD COLUMN benefits jsonb").Error; err != nil {
					t.Fatal(err)
				}
				raw := `[{"planGroup":"DEFAULT","validityType":"permanent"}]`
				if scenario == "combination_rejected" {
					raw = `[{"planGroup":"DEFAULT","validityType":"permanent"},{"planGroup":"FULL","validityType":"duration","durationDays":30}]`
				}
				if err := h.database.Exec("UPDATE plans SET benefits = ?::jsonb WHERE id = ?", raw, p.ID).Error; err != nil {
					t.Fatal(err)
				}
			}
			migration, err := os.ReadFile(filepath.Join(integrationMigrationsDir(t), "20261004_01_plan_group_entitlements.sql"))
			if err != nil {
				t.Fatal(err)
			}
			err = h.database.Transaction(func(tx *gorm.DB) error { return tx.Exec(string(migration)).Error })
			if scenario == "combination_rejected" {
				if err == nil {
					t.Fatal("combination silently converted")
				}
				if !h.database.Migrator().HasColumn("plans", "benefits") {
					t.Fatal("failed transaction discarded source")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if err = h.database.First(&p, "id = ?", p.ID).Error; err != nil {
				t.Fatal(err)
			}
			if scenario == "old_permanent" && (p.ValidityType != "permanent" || p.Days != 0) {
				t.Fatalf("permanent=%+v", p)
			}
			if scenario == "legacy_duration" && (p.ValidityType != "duration" || p.Days != 30) {
				t.Fatalf("duration=%+v", p)
			}
			if h.database.Migrator().HasColumn("plans", "benefits") {
				t.Fatal("obsolete product column remains")
			}
			if err = h.database.Exec(string(migration)).Error; err != nil {
				t.Fatal(err)
			}
		})
	}
}

// TestIntegrationHistoricalCombinationSnapshot keeps old paid orders independent of the new single-group product.
func TestIntegrationHistoricalCombinationSnapshot(t *testing.T) {
	h := newIntegrationHarness(t)
	secret := "whsec_mock_history"
	t.Setenv("STRIPE_WEBHOOK_SECRET", secret)
	if err := h.database.Model(&models.PlanGroup{}).Where("key = ?", "DEFAULT").Update("entitlement_rank", 10).Error; err != nil {
		t.Fatal(err)
	}
	h.seedPlanGroup(t, models.PlanGroup{Key: "FULL", Name: "Full", EntitlementRank: intRank(20)})
	past := time.Now().AddDate(0, 0, -2)
	u := h.seedUser(t, models.User{Username: "snapshot_owner", Email: "snapshot@example.com", ExpiresAt: &past})
	plan := seedBillingIntegrationPlan(t, h, "history-single", "DEFAULT", 60)
	pay := seedBillingIntegrationPayment(t, h, u.ID, plan, models.PaymentPending, "cs_history_single", 30)
	original := []models.PlanBenefit{{PlanGroup: "DEFAULT", ValidityType: "permanent"}, {PlanGroup: "FULL", ValidityType: "duration", DurationDays: 30}}
	raw, err := json.Marshal(original)
	if err != nil {
		t.Fatal(err)
	}
	if err = h.database.Exec("UPDATE payments SET benefits = ?::jsonb WHERE id = ?", string(raw), pay.ID).Error; err != nil {
		t.Fatal(err)
	}
	if err = h.database.Model(&models.Plan{}).Where("id = ?", plan.ID).Updates(map[string]any{"is_active": false, "days": 180}).Error; err != nil {
		t.Fatal(err)
	}
	payload := billingStripeWebhookPayload("evt_history_single", "checkout.session.completed", "cs_history_single", "pi_history_single", true, time.Now())
	var first []models.UserEntitlement
	for i := 0; i < 2; i++ {
		r := performBillingStripeWebhook(h, payload, secret)
		if r.Code != http.StatusOK {
			t.Fatalf("webhook=%d %s", r.Code, r.Body.String())
		}
		var holdings []models.UserEntitlement
		if err = h.database.Where("user_id = ?", u.ID).Order("plan_group").Find(&holdings).Error; err != nil {
			t.Fatal(err)
		}
		if len(holdings) != 2 || holdings[0].ValidityType != "permanent" || holdings[1].ExpiresAt == nil {
			t.Fatalf("holdings=%+v", holdings)
		}
		if i == 0 {
			remaining := time.Until(*holdings[1].ExpiresAt)
			if remaining < 29*24*time.Hour || remaining > 31*24*time.Hour {
				t.Fatal("historical 30-day snapshot was replaced by current product days")
			}
			first = holdings
		} else if !first[1].ExpiresAt.Equal(*holdings[1].ExpiresAt) {
			t.Fatal("callback replay extended entitlement")
		}
	}
	if err = h.database.First(&pay, "id = ?", pay.ID).Error; err != nil {
		t.Fatal(err)
	}
	if len(pay.Benefits) != 2 || pay.Benefits[1].DurationDays != 30 || pay.Status != models.PaymentCompleted {
		t.Fatalf("snapshot/status mutated: %+v", pay.Benefits)
	}
}
