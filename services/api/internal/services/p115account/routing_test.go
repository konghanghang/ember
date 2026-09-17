package p115account

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/konghang/ember/backend/internal/models"
)

// TestServicePlaybackRouteSurvivesConcurrentSuccess reproduces two requests
// sharing a route while the first finishes its health write before the second acquires.
func TestServicePlaybackRouteSurvivesConcurrentSuccess(t *testing.T) {
	now := time.Date(2026, 9, 12, 0, 0, 0, 0, time.UTC)
	account := &models.P115Account{
		ID: "playback", Role: models.P115AccountRolePlayback,
		ProviderUserID: stringPointer("100"), CookieCiphertext: stringPointer("encrypted:cookie"),
		AppType: stringPointer("web"), UserAgent: stringPointer("fixture-agent"),
		TargetParentID: stringPointer("200"), TargetParentPath: stringPointer("/Playback"),
		MaxConcurrentStreams: intPointer(3), Enabled: true, Status: models.P115AccountStatusActive,
		UpdatedAt: now.Add(-time.Minute), ConfigVersion: 7,
	}
	store := &fakeAccountStore{accounts: map[string]*models.P115Account{account.ID: account},
		personalPolicy: PersonalPlanPolicy{PlaybackMode: models.P115PlaybackModeSystem}}
	service := newServiceWithDependencies(store, fakeCredentialCipher{})
	service.now = func() time.Time { return now }
	route, err := service.ResolvePlaybackRoute(context.Background(), "user-1", now)
	if err != nil {
		t.Fatal(err)
	}
	first, err := service.AcquirePlaybackRoute(context.Background(), route)
	if err != nil {
		t.Fatal(err)
	}
	if first.DownloadCacheVersion != account.ConfigVersion {
		t.Fatal("acquired credential lost cache generation")
	}
	if err := service.ReportRuntimeHealth(context.Background(), first, RuntimeHealthSucceeded); err != nil {
		t.Fatal(err)
	}
	if _, err := service.AcquirePlaybackRoute(context.Background(), route); err != nil {
		t.Fatalf("unmodified configuration rejected after another request succeeded: %v", err)
	}
	if err := service.ReportRuntimeHealth(context.Background(), first, RuntimeHealthCredentialRejected); !errors.Is(err, ErrRuntimeStateChanged) {
		t.Fatalf("old observation replaced success: %v", err)
	}
	account.Status = models.P115AccountStatusCoolingDown
	until := now.Add(time.Minute)
	account.CooldownUntil = &until
	if _, err := service.AcquirePlaybackRoute(context.Background(), route); !errors.Is(err, ErrAccountCoolingDown) {
		t.Fatalf("latest cooldown bypassed: %v", err)
	}
	account.Status = models.P115AccountStatusActive
	current, err := service.AcquirePlaybackRoute(context.Background(), route)
	if err != nil {
		t.Fatal(err)
	}

	// Acquisition must expose the latest half-open state, not the earlier active route.
	account.Status = models.P115AccountStatusCoolingDown
	expired := now.Add(-time.Second)
	account.CooldownUntil = &expired
	probe, err := service.AcquirePlaybackRoute(context.Background(), route)
	if err != nil || probe.DownloadCacheVersion != 0 {
		t.Fatalf("probe cache version=%d err=%v", probe.DownloadCacheVersion, err)
	}
	account.ConfigVersion++ // A control change is visible even at an identical timestamp.
	if _, err := service.AcquirePlaybackRoute(context.Background(), route); !errors.Is(err, ErrRuntimeStateChanged) {
		t.Fatalf("stale route accepted: %v", err)
	}
	if err := service.ReportRuntimeHealth(context.Background(), current, RuntimeHealthCredentialRejected); !errors.Is(err, ErrRuntimeStateChanged) {
		t.Fatalf("stale configuration health accepted: %v", err)
	}
}

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
		Status: models.P115AccountStatusActive, ConfigVersion: 0,
	})
	if err != nil {
		t.Fatalf("AcquirePlaybackRoute() error = %v", err)
	}
	if credential.Credential.Cookie != "cookie" || credential.Credential.AccountID != "personal" || credential.TargetParentID != "200" {
		t.Fatalf("credential = %+v", credential)
	}
}

// TestDownloadCacheVersionOnlyUsesCleanActiveSnapshots protects the metadata
// boundary used by DirectPlay; probes and stale error states force Provider I/O.
func TestDownloadCacheVersionOnlyUsesCleanActiveSnapshots(t *testing.T) {
	now := time.Now()
	for _, kind := range []string{"active", "disabled", "cooling", "cooldown-field", "error-code", "error-message", "unversioned"} {
		t.Run(kind, func(t *testing.T) {
			account := &models.P115Account{Enabled: true, Status: models.P115AccountStatusActive, ConfigVersion: 7}
			switch kind {
			case "disabled":
				account.Enabled = false
			case "cooling":
				account.Status = models.P115AccountStatusCoolingDown
			case "cooldown-field":
				account.CooldownUntil = &now
			case "error-code":
				account.LastErrorCode = stringPointer("fixture")
			case "error-message":
				account.LastErrorMessage = stringPointer("fixture")
			case "unversioned":
				account.ConfigVersion = 0
			}
			want := int64(0)
			if kind == "active" {
				want = 7
			}
			if got := downloadCacheVersion(account); got != want {
				t.Fatalf("version=%d want=%d", got, want)
			}
		})
	}
	if downloadCacheVersion(nil) != 0 {
		t.Fatal("nil account enabled cache")
	}
}
