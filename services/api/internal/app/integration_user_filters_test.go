package app

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/konghang/ember/backend/internal/models"
	userpkg "github.com/konghang/ember/backend/internal/services/user"
)

// TestIntegrationUserEntitlementFilter covers HTTP binding, active holdings, combined filters and pagination without external calls.
func TestIntegrationUserEntitlementFilter(t *testing.T) {
	h := newIntegrationHarness(t)
	h.seedPlanGroup(t, models.PlanGroup{Key: "SECOND", Name: "第二组"})
	second := "SECOND"
	now := time.Now()
	future, past := now.Add(time.Hour), now.Add(-time.Second)
	for _, tc := range []struct {
		name   string
		kind   string
		expiry *time.Time
	}{
		{"holding_permanent", "permanent", nil},
		{"holding_duration", "duration", &future},
		{"holding_expired", "duration", &past},
	} {
		u := h.seedUser(t, models.User{Username: tc.name, Email: tc.name + "@example.com"})
		if err := h.database.Create(&models.UserEntitlement{UserID: u.ID, PlanGroup: second, ValidityType: tc.kind, ExpiresAt: tc.expiry}).Error; err != nil {
			t.Fatal(err)
		}
	}
	h.seedUser(t, models.User{Username: "holding_current", Email: "current@example.com", PlanGroup: &second})
	stale := h.seedUser(t, models.User{Username: "holding_stale", Email: "stale@example.com", PlanGroup: &second})
	if err := h.database.Where("user_id = ?", stale.ID).Delete(&models.UserEntitlement{}).Error; err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		query string
		total int64
		count int
	}{
		{"planGroup=SECOND", 2, 2},
		{"entitlementGroup=SECOND", 3, 3},
		{"entitlementGroup=second&planGroup=DEFAULT", 2, 2},
		{"entitlementGroup=SECOND&search=holding_expired", 0, 0},
		{"entitlementGroup=SECOND&search=holding_stale", 0, 0},
		{"entitlementGroup=UNKNOWN", 0, 0},
		{"entitlementGroup=SECOND&pageSize=2&page=1", 3, 2},
		{"entitlementGroup=SECOND&pageSize=2&page=2", 3, 1},
	} {
		t.Run(tc.query, func(t *testing.T) {
			response := h.performAdminRequest(http.MethodGet, "/api/v1/admin/users?"+tc.query, nil)
			if response.Code != http.StatusOK {
				t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
			}
			var result userpkg.GetUsersResponse
			if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
				t.Fatal(err)
			}
			if result.Total != tc.total || len(result.Data) != tc.count {
				t.Fatalf("total=%d count=%d; want %d/%d", result.Total, len(result.Data), tc.total, tc.count)
			}
		})
	}
	response := h.performAdminRequest(http.MethodGet, "/api/v1/admin/users?entitlementGroup=invalid%20group", nil)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("invalid group status=%d", response.Code)
	}
}
