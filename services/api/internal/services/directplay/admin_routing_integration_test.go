package directplay

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/konghang/ember/backend/internal/models"
	"github.com/konghang/ember/backend/internal/services/p115account"
)

// TestIntegrationPlaybackRolesFollowPlan exercises the real SQL role filter:
// administrators and users follow their plan, without an implicit shared fallback.
func TestIntegrationPlaybackRolesFollowPlan(t *testing.T) {
	database := newDirectPlayIntegrationDatabase(t)
	accounts := seedDirectPlayAccounts(t, database)
	ctx := context.Background()
	group := models.PlanGroup{Key: "ROUTING", Name: "Routing fixture", P115PlaybackMode: models.P115PlaybackModeSystem,
		P115TransferHourlyLimit: 7, P115TransferDailyLimit: 19}
	if err := database.Create(&group).Error; err != nil {
		t.Fatal(err)
	}
	template := models.PlanGroupEmbyPolicyTemplate{PlanGroupKey: group.Key, SimultaneousStreamLimit: 1}
	if err := database.Create(&template).Error; err != nil {
		t.Fatal(err)
	}
	for _, role := range []string{"admin", "user"} {
		t.Run(role, func(t *testing.T) {
			user := models.User{ID: "routing-" + role, Username: "routing-" + role, Email: role + "@routing.example", Role: role, PlanGroup: &group.Key}
			if err := database.Create(&user).Error; err != nil {
				t.Fatal(err)
			}
			route, err := accounts.ResolvePlaybackRoute(ctx, user.ID, time.Now())
			if err != nil {
				t.Fatalf("system plan route: %v", err)
			}
			if route.PlaybackMode != models.P115PlaybackModeSystem || route.OwnerUserID != "" || route.AccountID == "" ||
				route.TransferHourlyLimit != 7 || route.TransferDailyLimit != 19 || route.EffectiveMaxConcurrentStreams != 3 {
				t.Fatalf("plan policy or shared account capacity lost: %+v", route)
			}
			if err := database.Model(&group).Update("p115_playback_mode", models.P115PlaybackModePersonal).Error; err != nil {
				t.Fatal(err)
			}
			if _, err := accounts.ResolvePlaybackRoute(ctx, user.ID, time.Now()); !errors.Is(err, p115account.ErrPersonalAccountMissing) {
				t.Fatalf("personal plan must not borrow shared account: %v", err)
			}
			// A pre-existing personal account must be selected by ownership, with
			// its effective concurrency capped by the same plan for both roles.
			var personal models.P115Account
			if err := database.Where("id = ?", route.AccountID).First(&personal).Error; err != nil {
				t.Fatal(err)
			}
			providerID := "provider-" + role
			personal.ID = "personal-" + role
			personal.OwnerUserID = &user.ID
			personal.ProviderUserID = &providerID
			if err := database.Create(&personal).Error; err != nil {
				t.Fatal(err)
			}
			if selected, err := accounts.ResolvePlaybackRoute(ctx, user.ID, time.Now()); err != nil || selected.AccountID != personal.ID || selected.OwnerUserID != user.ID || selected.EffectiveMaxConcurrentStreams != 1 {
				t.Fatalf("personal ownership/plan cap: route=%+v err=%v", selected, err)
			}
			if role == "admin" {
				if _, err := accounts.GetPersonalAccount(ctx, user.ID); !errors.Is(err, p115account.ErrPersonalPlanPolicyUnavailable) {
					t.Fatalf("personal management role boundary changed: %v", err)
				}
			}
			if err := database.Model(&group).Update("p115_playback_mode", models.P115PlaybackModeSystem).Error; err != nil {
				t.Fatal(err)
			}
		})
	}
	if _, err := accounts.ResolvePlaybackRoute(ctx, "missing-user", time.Now()); err == nil {
		t.Fatal("missing user admitted")
	}
	// An unset explicit group must use the configured default, including quotas.
	if err := database.Model(&models.PlanGroup{}).Where("is_default = ?", true).Update("is_default", false).Error; err != nil {
		t.Fatal(err)
	}
	if err := database.Model(&group).Update("is_default", true).Error; err != nil {
		t.Fatal(err)
	}
	if err := database.Model(&models.User{}).Where("id = ?", "routing-admin").Update("plan_group", nil).Error; err != nil {
		t.Fatal(err)
	}
	if route, err := accounts.ResolvePlaybackRoute(ctx, "routing-admin", time.Now()); err != nil || route.TransferHourlyLimit != 7 || route.PlaybackMode != models.P115PlaybackModeSystem {
		t.Fatalf("administrator default plan: route=%+v err=%v", route, err)
	}
	if err := database.Model(&models.P115Account{}).Where("role = ?", models.P115AccountRolePlayback).Update("enabled", false).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := accounts.ResolvePlaybackRoute(ctx, "routing-admin", time.Now()); !errors.Is(err, p115account.ErrAccountUnavailable) {
		t.Fatalf("administrator bypassed disabled shared account: %v", err)
	}
	if err := database.Delete(&template).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := accounts.ResolvePlaybackRoute(ctx, "routing-admin", time.Now()); !errors.Is(err, p115account.ErrPersonalPlanPolicyUnavailable) {
		t.Fatalf("missing template must not grant administrator defaults: %v", err)
	}
}
