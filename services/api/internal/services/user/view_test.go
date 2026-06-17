package user

import (
	"testing"
	"time"

	"github.com/konghang/ember/backend/internal/models"
)

func TestMarkUsingDefaultPlanGroup(t *testing.T) {
	explicitPlanGroup := "VIP_A"
	past := time.Now().UTC().Add(-time.Hour)

	testCases := []struct {
		name           string
		view           UserView
		wantDefault    bool
		wantExpired    bool
		wantPolicySync string
	}{
		{
			name: "default plan group",
			view: UserView{
				User: models.User{},
			},
			wantDefault:    true,
			wantPolicySync: "synced",
		},
		{
			name: "explicit plan group",
			view: UserView{
				User: models.User{PlanGroup: &explicitPlanGroup},
			},
			wantPolicySync: "synced",
		},
		{
			name: "missing explicit plan group is not default",
			view: UserView{
				User:               models.User{},
				IsPlanGroupMissing: true,
			},
			wantPolicySync: "synced",
		},
		{
			name: "preserves existing policy status and marks expired",
			view: UserView{
				User:             models.User{ExpiresAt: &past},
				PolicySyncStatus: "failed",
			},
			wantDefault:    true,
			wantExpired:    true,
			wantPolicySync: "failed",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			view := tc.view
			view.markUsingDefaultPlanGroup()

			if view.IsUsingDefaultPlanGroup != tc.wantDefault {
				t.Fatalf("expected IsUsingDefaultPlanGroup=%t, got %t", tc.wantDefault, view.IsUsingDefaultPlanGroup)
			}
			if view.IsExpired != tc.wantExpired {
				t.Fatalf("expected IsExpired=%t, got %t", tc.wantExpired, view.IsExpired)
			}
			if view.PolicySyncStatus != tc.wantPolicySync {
				t.Fatalf("expected PolicySyncStatus=%q, got %q", tc.wantPolicySync, view.PolicySyncStatus)
			}
		})
	}
}

func TestMarkUsersUsingDefaultPlanGroup(t *testing.T) {
	users := []UserView{
		{User: models.User{}},
		{User: models.User{}, IsPlanGroupMissing: true},
	}

	markUsersUsingDefaultPlanGroup(users)

	if !users[0].IsUsingDefaultPlanGroup {
		t.Fatal("expected first user to use default plan group")
	}
	if users[1].IsUsingDefaultPlanGroup {
		t.Fatal("expected missing plan group user not to be marked as using default")
	}
	if users[0].PolicySyncStatus != "synced" || users[1].PolicySyncStatus != "synced" {
		t.Fatalf("expected default policy sync status, got %q and %q", users[0].PolicySyncStatus, users[1].PolicySyncStatus)
	}
}
