package p115account

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/konghang/ember/backend/internal/models"
)

func TestServiceResolvePlaybackRouteUsesPersonalAccountAndEffectivePlanLimit(t *testing.T) {
	configured := 8
	providerUserID := "100"
	targetParentID := "200"
	targetParentPath := "/Playback"
	store := &fakeAccountStore{
		accounts: map[string]*models.P115Account{
			"personal": {
				ID: "personal", OwnerUserID: stringPointer("user-1"), Role: models.P115AccountRolePlayback,
				ProviderUserID: &providerUserID, TargetParentID: &targetParentID, TargetParentPath: &targetParentPath,
				MaxConcurrentStreams: &configured, Status: models.P115AccountStatusActive, Enabled: true,
			},
		},
		personalPolicy: PersonalPlanPolicy{
			PlanGroupKey: "VIP", PlaybackMode: models.P115PlaybackModePersonal,
			TransferHourlyLimit: 5, TransferDailyLimit: 10, SimultaneousStreamLimit: 3,
		},
	}
	service := newServiceWithDependencies(store, fakeCredentialCipher{})

	route, err := service.ResolvePlaybackRoute(context.Background(), "user-1", time.Now().UTC())
	if err != nil {
		t.Fatalf("ResolvePlaybackRoute() error = %v", err)
	}
	if route.PlaybackMode != models.P115PlaybackModePersonal || route.AccountID != "personal" || route.OwnerUserID != "user-1" ||
		route.ConfiguredMaxConcurrentStreams != 8 || route.EffectiveMaxConcurrentStreams != 3 || route.SimultaneousStreamLimit != 3 {
		t.Fatalf("route = %+v", route)
	}
}

func TestServiceResolvePlaybackRouteUsesSharedAccountWithoutPlanConcurrencyCap(t *testing.T) {
	sharedMax := 20
	providerUserID := "200"
	targetParentID := "300"
	targetParentPath := "/Shared"
	store := &fakeAccountStore{
		accounts: map[string]*models.P115Account{
			"shared": {
				ID: "shared", Role: models.P115AccountRolePlayback, ProviderUserID: &providerUserID,
				TargetParentID: &targetParentID, TargetParentPath: &targetParentPath, MaxConcurrentStreams: &sharedMax,
				Status: models.P115AccountStatusActive, Enabled: true,
			},
		},
		personalPolicy: PersonalPlanPolicy{
			PlanGroupKey: "VIP", PlaybackMode: models.P115PlaybackModeSystem,
			TransferHourlyLimit: 5, TransferDailyLimit: 10, SimultaneousStreamLimit: 1,
		},
	}
	service := newServiceWithDependencies(store, fakeCredentialCipher{})

	route, err := service.ResolvePlaybackRoute(context.Background(), "user-1", time.Now().UTC())
	if err != nil {
		t.Fatalf("ResolvePlaybackRoute() error = %v", err)
	}
	if route.PlaybackMode != models.P115PlaybackModeSystem || route.AccountID != "shared" || route.OwnerUserID != "" ||
		route.ConfiguredMaxConcurrentStreams != 20 || route.EffectiveMaxConcurrentStreams != 20 || route.SimultaneousStreamLimit != 1 {
		t.Fatalf("route = %+v", route)
	}
}

func TestServiceResolvePlaybackRouteFailsClosedForMissingOrUnusablePersonalAccount(t *testing.T) {
	policy := PersonalPlanPolicy{
		PlanGroupKey: "VIP", PlaybackMode: models.P115PlaybackModePersonal,
		TransferHourlyLimit: 5, TransferDailyLimit: 10, SimultaneousStreamLimit: 3,
	}
	service := newServiceWithDependencies(&fakeAccountStore{accounts: map[string]*models.P115Account{}, personalPolicy: policy}, fakeCredentialCipher{})
	missingRoute, err := service.ResolvePlaybackRoute(context.Background(), "user-1", time.Now().UTC())
	if !errors.Is(err, ErrPersonalAccountMissing) {
		t.Fatalf("missing personal route error = %v", err)
	}
	if missingRoute.PlaybackMode != models.P115PlaybackModePersonal || missingRoute.TransferHourlyLimit != 5 || missingRoute.TransferDailyLimit != 10 {
		t.Fatalf("missing personal route diagnostics = %+v", missingRoute)
	}

	maxStreams := 2
	providerUserID := "100"
	targetParentID := "200"
	targetParentPath := "/Playback"
	service = newServiceWithDependencies(&fakeAccountStore{
		accounts: map[string]*models.P115Account{
			"personal": {
				ID: "personal", OwnerUserID: stringPointer("user-1"), Role: models.P115AccountRolePlayback,
				ProviderUserID: &providerUserID, TargetParentID: &targetParentID, TargetParentPath: &targetParentPath,
				MaxConcurrentStreams: &maxStreams, Status: models.P115AccountStatusActive, Enabled: false,
			},
		},
		personalPolicy: policy,
	}, fakeCredentialCipher{})
	disabledRoute, err := service.ResolvePlaybackRoute(context.Background(), "user-1", time.Now().UTC())
	if !errors.Is(err, ErrAccountUnavailable) {
		t.Fatalf("disabled personal route error = %v", err)
	}
	if disabledRoute.PlaybackMode != models.P115PlaybackModePersonal {
		t.Fatalf("disabled personal route diagnostics = %+v", disabledRoute)
	}
}

func TestServiceAcquirePlaybackRouteLoadsExactCredentialAfterAdmission(t *testing.T) {
	now := time.Date(2026, 9, 4, 0, 0, 0, 0, time.UTC)
	maxStreams := 2
	providerUserID := "100"
	targetParentID := "200"
	targetParentPath := "/Playback"
	updatedAt := now.Add(-time.Minute)
	store := &fakeAccountStore{accounts: map[string]*models.P115Account{
		"personal": {
			ID: "personal", OwnerUserID: stringPointer("user-1"), Role: models.P115AccountRolePlayback,
			ProviderUserID: &providerUserID, CookieCiphertext: stringPointer("encrypted:cookie"),
			AppType: stringPointer("web"), UserAgent: stringPointer(personalProviderUserAgent),
			TargetParentID: &targetParentID, TargetParentPath: &targetParentPath, MaxConcurrentStreams: &maxStreams,
			Status: models.P115AccountStatusActive, Enabled: true, UpdatedAt: updatedAt,
		},
	}}
	service := newServiceWithDependencies(store, fakeCredentialCipher{})
	service.now = func() time.Time { return now }

	credential, err := service.AcquirePlaybackRoute(context.Background(), PlaybackRoute{
		AccountID: "personal", OwnerUserID: "user-1", ProviderUserID: "100",
		TargetParentID: "200", TargetParentPath: "/Playback", ConfiguredMaxConcurrentStreams: 2,
		Status: models.P115AccountStatusActive, UpdatedAt: updatedAt,
	})
	if err != nil {
		t.Fatalf("AcquirePlaybackRoute() error = %v", err)
	}
	if credential.Credential.Cookie != "cookie" || credential.Credential.AccountID != "personal" || credential.TargetParentID != "200" {
		t.Fatalf("credential = %+v", credential)
	}
}
