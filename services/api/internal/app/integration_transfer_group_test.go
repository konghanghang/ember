package app

import (
	"net/http"
	"testing"
	"time"

	"github.com/konghang/ember/backend/internal/models"
)

// TestIntegrationAdminTransferGroup checks the actual edit endpoint with no ranks and no external services.
func TestIntegrationAdminTransferGroup(t *testing.T) {
	h := newIntegrationHarness(t)
	h.seedPlanGroup(t, models.PlanGroup{Key: "FIRST", Name: "First"})
	source := "FIRST"
	past := time.Date(2020, 1, 1, 1, 2, 3, 123456000, time.UTC)
	future := time.Now().AddDate(1, 0, 0).Truncate(time.Microsecond)
	for i, expiry := range []*time.Time{&past, &future, nil} {
		u := h.seedUser(t, models.User{Username: []string{"expired_transfer", "active_transfer", "permanent_transfer"}[i], Email: []string{"past@example.com", "future@example.com", "permanent@example.com"}[i], PlanGroup: &source, ExpiresAt: expiry})
		if err := h.database.Model(&models.User{}).Where("id = ?", u.ID).Updates(map[string]any{"is_active": false, "emby_access_disabled": true}).Error; err != nil {
			t.Fatal(err)
		}
		for attempt := 0; attempt < 2; attempt++ {
			r := h.performAdminRequest(http.MethodPut, "/api/v1/admin/users/"+u.ID, []byte(`{"planGroup":"DEFAULT"}`))
			if r.Code != http.StatusOK {
				t.Fatalf("transfer %d status=%d body=%s", i, r.Code, r.Body.String())
			}
		}
		var after models.User
		if err := h.database.First(&after, "id = ?", u.ID).Error; err != nil {
			t.Fatal(err)
		}
		if after.PlanGroup == nil || *after.PlanGroup != "DEFAULT" || after.IsActive || !after.EmbyAccessDisabled || after.ResourceAccessGranted != u.ResourceAccessGranted {
			t.Fatal("projection or manual restrictions changed")
		}
		if (after.ExpiresAt == nil) != (expiry == nil) || expiry != nil && !after.ExpiresAt.Equal(*expiry) {
			t.Fatal("deadline changed")
		}
		var grants []models.UserEntitlement
		if err := h.database.Where("user_id = ?", u.ID).Find(&grants).Error; err != nil {
			t.Fatal(err)
		}
		if len(grants) != 1 || grants[0].PlanGroup != "DEFAULT" || (grants[0].ExpiresAt == nil) != (expiry == nil) || expiry != nil && !grants[0].ExpiresAt.Equal(*expiry) {
			t.Fatalf("grant changed: %+v", grants)
		}
		var events int64
		if err := h.database.Model(&models.EntitlementEvent{}).Where("user_id = ? AND reason = ?", u.ID, "admin_transfer").Count(&events).Error; err != nil || events != 1 {
			t.Fatalf("events=%d err=%v", events, err)
		}
	}
}

// TestIntegrationAdminTransferGroupRejectsConflict proves a conflicting target rolls back other edited fields too.
func TestIntegrationAdminTransferGroupRejectsConflict(t *testing.T) {
	h := newIntegrationHarness(t)
	h.seedPlanGroup(t, models.PlanGroup{Key: "FIRST", Name: "First"})
	source := "FIRST"
	u := h.seedUser(t, models.User{Username: "transfer_conflict", Email: "original@example.com", PlanGroup: &source})
	if err := h.database.Create(&models.UserEntitlement{UserID: u.ID, PlanGroup: "DEFAULT", ValidityType: "permanent"}).Error; err != nil {
		t.Fatal(err)
	}
	r := h.performAdminRequest(http.MethodPut, "/api/v1/admin/users/"+u.ID, []byte(`{"planGroup":"DEFAULT","email":"changed@example.com"}`))
	if r.Code != http.StatusConflict {
		t.Fatalf("status=%d body=%s", r.Code, r.Body.String())
	}
	var after models.User
	if err := h.database.First(&after, "id = ?", u.ID).Error; err != nil {
		t.Fatal(err)
	}
	if after.Email != u.Email || after.PlanGroup == nil || *after.PlanGroup != source {
		t.Fatal("conflict did not roll back")
	}
}

// TestIntegrationAdminTransferGroupSyncsTargetPolicy uses only the in-process Emby fake after the local commit.
func TestIntegrationAdminTransferGroupSyncsTargetPolicy(t *testing.T) {
	h := newIntegrationHarness(t)
	fake := newIntegrationFakeEmbyServer(t)
	h.setSetting(t, "EMBY_URL", fake.server.URL)
	h.setSetting(t, "EMBY_API_KEY", "integration-emby-key")
	h.seedPlanGroup(t, models.PlanGroup{Key: "FIRST", Name: "First"})
	h.seedPlanGroupLibraries(t, models.PlanGroupMediaLibrary{PlanGroupKey: "DEFAULT", LibraryID: "/data/movies", LibraryName: "电影", LibraryType: "movies"})
	source := "FIRST"
	u := h.seedUser(t, models.User{Username: "transfer_policy", Email: "policy@example.com", PlanGroup: &source, EmbyID: "emby_user_policy"})
	r := h.performAdminRequest(http.MethodPut, "/api/v1/admin/users/"+u.ID, []byte(`{"planGroup":"DEFAULT"}`))
	if r.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", r.Code, r.Body.String())
	}
	if fake.lastPolicyBody == nil || fake.lastPolicyBody["IsDisabled"] != false {
		t.Fatal("active account disabled during transfer")
	}
	folders, ok := fake.lastPolicyBody["EnabledFolders"].([]any)
	if !ok || len(folders) != 1 || folders[0] != "/data/movies" {
		t.Fatalf("target policy not applied: %v", folders)
	}
}
