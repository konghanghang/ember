package app

import (
	"net/http"
	"testing"
	"time"

	"github.com/konghang/ember/backend/internal/models"
)

// TestIntegrationAdminCurrentGroupExpiry checks wall-time editing and permanence without changing other holdings or manual bans.
func TestIntegrationAdminCurrentGroupExpiry(t *testing.T) {
	h := newIntegrationHarness(t)
	h.setSetting(t, "CRON_TIMEZONE", "Asia/Tokyo")
	h.seedPlanGroup(t, models.PlanGroup{Key: "FULL", Name: "全资源", EntitlementRank: intRank(20)})
	if err := h.database.Model(&models.PlanGroup{}).Where("key = ?", "DEFAULT").Update("entitlement_rank", 10).Error; err != nil {
		t.Fatal(err)
	}
	group := "FULL"
	expiry := time.Date(2099, 1, 1, 0, 0, 0, 0, time.UTC)
	user := h.seedUser(t, models.User{Username: "expiry_edit_fixture", Email: "expiry-edit@example.com", PlanGroup: &group, ExpiresAt: &expiry, EmbyAccessDisabled: true})
	if err := h.database.Create(&models.UserEntitlement{UserID: user.ID, PlanGroup: "DEFAULT", ValidityType: "permanent"}).Error; err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		body, kind string
		expiry     *time.Time
	}{
		{`{"expiresAt":"2099-02-01 12:00:00"}`, "duration", timePointer(time.Date(2099, 2, 1, 3, 0, 0, 0, time.UTC))},
		{`{"clearExpiresAt":true}`, "permanent", nil},
		{`{"expiresAt":"2099-03-01T12:00:00Z"}`, "duration", timePointer(time.Date(2099, 3, 1, 12, 0, 0, 0, time.UTC))},
	} {
		response := h.performAdminRequest(http.MethodPut, "/api/v1/admin/users/"+user.ID, []byte(tc.body))
		if response.Code != http.StatusOK {
			t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
		}
		var holdings []models.UserEntitlement
		if err := h.database.Where("user_id = ?", user.ID).Order("plan_group").Find(&holdings).Error; err != nil {
			t.Fatal(err)
		}
		if len(holdings) != 2 || holdings[0].PlanGroup != "DEFAULT" || holdings[0].ValidityType != "permanent" || holdings[0].ExpiresAt != nil {
			t.Fatalf("other group changed: %+v", holdings)
		}
		current := holdings[1]
		if current.PlanGroup != "FULL" || current.ValidityType != tc.kind || (current.ExpiresAt == nil) != (tc.expiry == nil) || tc.expiry != nil && !current.ExpiresAt.Equal(*tc.expiry) {
			t.Fatalf("wrong deadline: %+v", current)
		}
		var after models.User
		if err := h.database.First(&after, "id = ?", user.ID).Error; err != nil {
			t.Fatal(err)
		}
		if !after.EmbyAccessDisabled || after.PlanGroup == nil || *after.PlanGroup != "FULL" {
			t.Fatal("manual ban or current group changed")
		}
	}
	response := h.performAdminRequest(http.MethodPut, "/api/v1/admin/users/"+user.ID, []byte(`{"expiresAt":"invalid"}`))
	if response.Code != http.StatusBadRequest {
		t.Fatalf("invalid expiry status=%d", response.Code)
	}
}

// timePointer supplies an expected instant for nullable expiry comparisons.
func timePointer(value time.Time) *time.Time { return &value }

// TestIntegrationAdminEditExtension atomically saves profile and expiry and prevents repeated extension on retries.
func TestIntegrationAdminEditExtension(t *testing.T) {
	h := newIntegrationHarness(t)
	h.setSetting(t, "CRON_TIMEZONE", "Asia/Shanghai")
	if err := h.database.Model(&models.PlanGroup{}).Where("key = ?", "DEFAULT").Update("entitlement_rank", 10).Error; err != nil {
		t.Fatal(err)
	}
	expiry := time.Date(2099, 1, 1, 0, 0, 0, 0, time.UTC)
	user := h.seedUser(t, models.User{Username: "edit_extension", Email: "edit-extension@example.com", ExpiresAt: &expiry, EmbyAccessDisabled: true})
	h.seedUser(t, models.User{Username: "occupied_email", Email: "occupied@example.com", ExpiresAt: &expiry})
	body := []byte(`{"email":"updated@example.com","extendDays":45,"operationId":"same-request"}`)
	for i := 0; i < 2; i++ {
		response := h.performAdminRequest(http.MethodPut, "/api/v1/admin/users/"+user.ID, body)
		if response.Code != http.StatusOK {
			t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
		}
	}
	var after models.User
	if err := h.database.First(&after, "id = ?", user.ID).Error; err != nil {
		t.Fatal(err)
	}
	expected := expiry.AddDate(0, 0, 45)
	if after.Email != "updated@example.com" || after.ExpiresAt == nil || !after.ExpiresAt.Equal(expected) || !after.EmbyAccessDisabled {
		t.Fatalf("bad merged edit: expiry=%v emailMatches=%t banned=%t", after.ExpiresAt, after.Email == "updated@example.com", after.EmbyAccessDisabled)
	}
	// A uniqueness failure after granting must roll back the grant and its audit, not leave a partial save.
	response := h.performAdminRequest(http.MethodPut, "/api/v1/admin/users/"+user.ID, []byte(`{"email":"occupied@example.com","extendDays":7,"operationId":"conflicting-edit"}`))
	if response.Code != http.StatusBadRequest {
		t.Fatalf("conflict status=%d", response.Code)
	}
	if err := h.database.First(&after, "id = ?", user.ID).Error; err != nil {
		t.Fatal(err)
	}
	if after.Email != "updated@example.com" || after.ExpiresAt == nil || !after.ExpiresAt.Equal(expected) {
		t.Fatal("failed edit partially committed")
	}
	var holding models.UserEntitlement
	if err := h.database.First(&holding, "user_id = ? AND plan_group = ?", user.ID, "DEFAULT").Error; err != nil {
		t.Fatal(err)
	}
	if holding.ExpiresAt == nil || !holding.ExpiresAt.Equal(expected) {
		t.Fatal("failed edit changed holding")
	}
	var auditCount int64
	if err := h.database.Model(&models.EntitlementEvent{}).Where("user_id = ?", user.ID).Count(&auditCount).Error; err != nil || auditCount != 1 {
		t.Fatalf("audit count=%d err=%v", auditCount, err)
	}
}
