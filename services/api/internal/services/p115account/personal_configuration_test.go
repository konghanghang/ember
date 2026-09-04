package p115account

import (
	"context"
	"errors"
	"testing"
	"time"

	p115integration "github.com/konghang/ember/backend/internal/integrations/p115"
	"github.com/konghang/ember/backend/internal/models"
)

func TestServiceGetPersonalAccountCalculatesEffectiveLimit(t *testing.T) {
	configured := 8
	store := &fakeAccountStore{
		accounts: map[string]*models.P115Account{
			"personal": {ID: "personal", OwnerUserID: stringPointer("user-1"), Role: models.P115AccountRolePlayback, Status: models.P115AccountStatusActive, MaxConcurrentStreams: &configured},
		},
		personalPolicy: PersonalPlanPolicy{
			PlanGroupKey: "VIP", PlaybackMode: models.P115PlaybackModePersonal,
			TransferHourlyLimit: 5, TransferDailyLimit: 10, SimultaneousStreamLimit: 3,
		},
	}
	service := newServiceWithDependencies(store, fakeCredentialCipher{})

	result, err := service.GetPersonalAccount(context.Background(), "user-1")
	if err != nil {
		t.Fatalf("GetPersonalAccount() error = %v", err)
	}
	if result.SimultaneousStreamLimit == nil || *result.SimultaneousStreamLimit != 3 ||
		result.EffectiveMaxConcurrentStreams == nil || *result.EffectiveMaxConcurrentStreams != 3 {
		t.Fatalf("summary limits = %+v", result)
	}
}

func TestServiceUpdatePersonalDirectoryUsesOwnedActiveCredential(t *testing.T) {
	updatedAt := time.Date(2026, 9, 3, 12, 0, 0, 0, time.UTC)
	store := &fakeAccountStore{accounts: map[string]*models.P115Account{
		"personal": {
			ID: "personal", OwnerUserID: stringPointer("user-1"), Role: models.P115AccountRolePlayback,
			CookieCiphertext: stringPointer("encrypted:cookie"), AppType: stringPointer("web"), UserAgent: stringPointer(personalProviderUserAgent),
			Status: models.P115AccountStatusActive, UpdatedAt: updatedAt,
		},
	}}
	resolver := &fakeDirectoryResolver{directory: &p115integration.Directory{ID: "200", Path: "/Playback"}}
	service := newServiceWithDependencies(store, fakeCredentialCipher{})
	service.directoryResolver = resolver

	result, err := service.UpdatePersonalDirectory(context.Background(), "user-1", " Playback ")
	if err != nil {
		t.Fatalf("UpdatePersonalDirectory() error = %v", err)
	}
	if resolver.query.RootID != "0" || resolver.query.RelativePath != "Playback" || resolver.credential.UserAgent != personalProviderUserAgent {
		t.Fatalf("resolver input = credential=%+v query=%+v", resolver.credential, resolver.query)
	}
	if store.personalDirectoryOwner != "user-1" || store.personalDirectoryPath != "/Playback" || store.personalDirectoryTargetID != "200" {
		t.Fatalf("stored directory owner=%q path=%q target=%q", store.personalDirectoryOwner, store.personalDirectoryPath, store.personalDirectoryTargetID)
	}
	if result.TargetParentPath == nil || *result.TargetParentPath != "/Playback" {
		t.Fatalf("result = %+v", result)
	}
}

func TestServiceUpdatePersonalDirectoryRejectsPendingAccountWithoutProviderCall(t *testing.T) {
	store := &fakeAccountStore{accounts: map[string]*models.P115Account{
		"personal": {ID: "personal", OwnerUserID: stringPointer("user-1"), Role: models.P115AccountRolePlayback, Status: models.P115AccountStatusPending},
	}}
	resolver := &fakeDirectoryResolver{}
	service := newServiceWithDependencies(store, fakeCredentialCipher{})
	service.directoryResolver = resolver

	_, err := service.UpdatePersonalDirectory(context.Background(), "user-1", "/Playback")
	if !errors.Is(err, ErrAccountUnavailable) || resolver.calls != 0 {
		t.Fatalf("UpdatePersonalDirectory() error=%v resolverCalls=%d", err, resolver.calls)
	}
}

func TestValidatePersonalConcurrentLimit(t *testing.T) {
	tests := []struct {
		name         string
		configured   int
		simultaneous int
		want         int
		wantErr      error
	}{
		{name: "unlimited template", configured: 100, simultaneous: 0, want: 100},
		{name: "positive cap", configured: 3, simultaneous: 3, want: 3},
		{name: "above plan", configured: 4, simultaneous: 3, wantErr: ErrMaxConcurrentStreamsExceedsPlan},
		{name: "zero", configured: 0, simultaneous: 3, wantErr: ErrMaxConcurrentStreamsInvalid},
		{name: "over account cap", configured: 101, simultaneous: 0, wantErr: ErrMaxConcurrentStreamsInvalid},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := effectivePersonalConcurrentLimit(tt.configured, tt.simultaneous)
			if !errors.Is(err, tt.wantErr) || got != tt.want {
				t.Fatalf("effectivePersonalConcurrentLimit() = %d, %v; want %d, %v", got, err, tt.want, tt.wantErr)
			}
		})
	}
}

func TestServiceUpdatePersonalConcurrencyUsesCurrentPlanPolicy(t *testing.T) {
	store := &fakeAccountStore{
		accounts: map[string]*models.P115Account{
			"personal": {ID: "personal", OwnerUserID: stringPointer("user-1"), Role: models.P115AccountRolePlayback, Status: models.P115AccountStatusActive},
		},
		personalPolicy: PersonalPlanPolicy{PlanGroupKey: "VIP", PlaybackMode: models.P115PlaybackModePersonal, TransferHourlyLimit: 5, TransferDailyLimit: 10, SimultaneousStreamLimit: 3},
	}
	service := newServiceWithDependencies(store, fakeCredentialCipher{})

	result, err := service.UpdatePersonalConcurrency(context.Background(), "user-1", 3)
	if err != nil {
		t.Fatalf("UpdatePersonalConcurrency() error = %v", err)
	}
	if store.personalConcurrencyOwner != "user-1" || store.personalConcurrencyMax != 3 || result.EffectiveMaxConcurrentStreams == nil || *result.EffectiveMaxConcurrentStreams != 3 {
		t.Fatalf("concurrency result=%+v storeOwner=%q storeMax=%d", result, store.personalConcurrencyOwner, store.personalConcurrencyMax)
	}
	if _, err := service.UpdatePersonalConcurrency(context.Background(), "user-1", 4); !errors.Is(err, ErrMaxConcurrentStreamsExceedsPlan) {
		t.Fatalf("above-plan error = %v", err)
	}
}

func TestServiceUpdatePersonalConcurrencyDoesNotWriteWhenAtomicPolicyCheckFails(t *testing.T) {
	store := &fakeAccountStore{
		accounts: map[string]*models.P115Account{
			"personal": {ID: "personal", OwnerUserID: stringPointer("user-1"), Role: models.P115AccountRolePlayback, Status: models.P115AccountStatusActive},
		},
		personalPolicy:    PersonalPlanPolicy{PlanGroupKey: "VIP", PlaybackMode: models.P115PlaybackModePersonal, TransferHourlyLimit: 5, TransferDailyLimit: 10, SimultaneousStreamLimit: 3},
		personalPolicyErr: ErrPersonalPlanPolicyUnavailable,
	}
	service := newServiceWithDependencies(store, fakeCredentialCipher{})

	if _, err := service.UpdatePersonalConcurrency(context.Background(), "user-1", 2); !errors.Is(err, ErrPersonalPlanPolicyUnavailable) {
		t.Fatalf("UpdatePersonalConcurrency() error = %v", err)
	}
	if store.personalConcurrencyOwner != "" {
		t.Fatalf("failed atomic policy check wrote account for owner %q", store.personalConcurrencyOwner)
	}
}

func TestServiceSetPersonalEnabledDelegatesAtomicPlanRecheck(t *testing.T) {
	store := &fakeAccountStore{
		accounts: map[string]*models.P115Account{
			"personal": {ID: "personal", OwnerUserID: stringPointer("user-1"), Role: models.P115AccountRolePlayback, Status: models.P115AccountStatusActive},
		},
		personalPolicy: PersonalPlanPolicy{PlanGroupKey: "VIP", PlaybackMode: models.P115PlaybackModePersonal, TransferHourlyLimit: 5, TransferDailyLimit: 10, SimultaneousStreamLimit: 3},
	}
	service := newServiceWithDependencies(store, fakeCredentialCipher{})

	result, err := service.SetPersonalEnabled(context.Background(), "user-1", true)
	if err != nil {
		t.Fatalf("SetPersonalEnabled() error = %v", err)
	}
	if store.personalEnabledOwner != "user-1" || !store.personalEnabledValue || !result.Enabled {
		t.Fatalf("enable result=%+v owner=%q value=%t", result, store.personalEnabledOwner, store.personalEnabledValue)
	}
	store.personalEnableErr = ErrMaxConcurrentStreamsExceedsPlan
	if _, err := service.SetPersonalEnabled(context.Background(), "user-1", true); !errors.Is(err, ErrMaxConcurrentStreamsExceedsPlan) {
		t.Fatalf("SetPersonalEnabled() error = %v", err)
	}
}
