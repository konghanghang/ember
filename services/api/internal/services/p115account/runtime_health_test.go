package p115account

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/konghang/ember/backend/internal/models"
)

type runtimeHealthStore struct {
	fakeAccountStore
	acquireErr        error
	acquireNow        time.Time
	acquireProbeUntil time.Time
	mutation          runtimeHealthMutation
	mutationRef       runtimeCredentialRef
	mutationErr       error
}

func (store *runtimeHealthStore) AcquireRuntimeByRole(
	_ context.Context,
	role models.P115AccountRole,
	now time.Time,
	probeUntil time.Time,
) (*models.P115Account, error) {
	store.acquireNow = now
	store.acquireProbeUntil = probeUntil
	if store.acquireErr != nil {
		return nil, store.acquireErr
	}
	return store.fakeAccountStore.GetActiveByRole(context.Background(), role)
}

func (store *runtimeHealthStore) CompleteRuntimeHealth(
	_ context.Context,
	ref runtimeCredentialRef,
	mutation runtimeHealthMutation,
) error {
	store.mutationRef = ref
	store.mutation = mutation
	return store.mutationErr
}

func TestServiceLoadActiveCredentialByRoleUsesBoundedRuntimeProbe(t *testing.T) {
	now := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	providerUserID := "provider-source"
	embyPathPrefix := "/mnt/cloudNAS/115lifetime"
	sourceRootID := "0"
	store := &runtimeHealthStore{fakeAccountStore: fakeAccountStore{accounts: map[string]*models.P115Account{
		"source": {
			ID: "source", Role: models.P115AccountRoleSource, ProviderUserID: &providerUserID,
			CookieCiphertext: "encrypted:source-cookie", Status: models.P115AccountStatusActive, Enabled: true,
			EmbyPathPrefix: &embyPathPrefix, SourceRootID: &sourceRootID, UpdatedAt: now.Add(-time.Hour),
		},
	}}}
	service := newServiceWithDependencies(store, fakeCredentialCipher{})
	service.now = func() time.Time { return now }

	account, err := service.LoadActiveCredentialByRole(context.Background(), models.P115AccountRoleSource)
	if err != nil {
		t.Fatalf("LoadActiveCredentialByRole() error = %v", err)
	}
	if account.Credential.AccountID != "source" || account.runtimeRef.accountID != "source" ||
		account.runtimeRef.expectedCiphertext != "encrypted:source-cookie" ||
		!account.runtimeRef.expectedUpdatedAt.Equal(now.Add(-time.Hour)) {
		t.Fatalf("runtime account = %+v ref=%+v", account, account.runtimeRef)
	}
	if !store.acquireNow.Equal(now) || !store.acquireProbeUntil.Equal(now.Add(runtimeProviderCooldown)) {
		t.Fatalf("runtime acquire now=%s probeUntil=%s", store.acquireNow, store.acquireProbeUntil)
	}
}

func TestServiceLoadActiveCredentialByRolePreservesCoolingError(t *testing.T) {
	store := &runtimeHealthStore{acquireErr: ErrAccountCoolingDown}
	service := newServiceWithDependencies(store, fakeCredentialCipher{})

	if _, err := service.LoadActiveCredentialByRole(context.Background(), models.P115AccountRolePlayback); !errors.Is(err, ErrAccountCoolingDown) {
		t.Fatalf("LoadActiveCredentialByRole() error = %v, want ErrAccountCoolingDown", err)
	}
}

func TestServiceReportRuntimeHealthBuildsSafeStateMutation(t *testing.T) {
	now := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	account := ActiveAccountCredential{runtimeRef: runtimeCredentialRef{
		accountID: "playback", expectedCiphertext: "encrypted:cookie", expectedUpdatedAt: now.Add(-time.Minute),
	}}
	tests := []struct {
		name         string
		outcome      RuntimeHealthOutcome
		wantStatus   models.P115AccountStatus
		wantDisable  bool
		wantSuccess  bool
		wantCode     string
		wantMessage  string
		wantCooldown bool
	}{
		{name: "success", outcome: RuntimeHealthSucceeded, wantStatus: models.P115AccountStatusActive, wantSuccess: true},
		{name: "credential rejected", outcome: RuntimeHealthCredentialRejected, wantStatus: models.P115AccountStatusExpired, wantDisable: true, wantCode: "credential_rejected", wantMessage: "115 Cookie 已失效"},
		{name: "provider unavailable", outcome: RuntimeHealthProviderUnavailable, wantStatus: models.P115AccountStatusCoolingDown, wantCode: "provider_unavailable", wantMessage: "115 服务暂不可用", wantCooldown: true},
		{name: "provider protocol", outcome: RuntimeHealthProviderProtocol, wantStatus: models.P115AccountStatusError, wantCode: "provider_protocol_error", wantMessage: "115 响应格式不兼容"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := &runtimeHealthStore{}
			service := newServiceWithDependencies(store, fakeCredentialCipher{})
			service.now = func() time.Time { return now }

			if err := service.ReportRuntimeHealth(context.Background(), account, tt.outcome); err != nil {
				t.Fatalf("ReportRuntimeHealth() error = %v", err)
			}
			mutation := store.mutation
			if mutation.Status != tt.wantStatus || mutation.Disable != tt.wantDisable ||
				mutation.Succeeded != tt.wantSuccess || mutation.Code != tt.wantCode || mutation.Message != tt.wantMessage ||
				!mutation.At.Equal(now) {
				t.Fatalf("mutation = %+v", mutation)
			}
			if tt.wantCooldown {
				if mutation.CooldownUntil == nil || !mutation.CooldownUntil.Equal(now.Add(runtimeProviderCooldown)) {
					t.Fatalf("cooldownUntil = %v", mutation.CooldownUntil)
				}
			} else if mutation.CooldownUntil != nil {
				t.Fatalf("unexpected cooldownUntil = %v", mutation.CooldownUntil)
			}
			if store.mutationRef != account.runtimeRef {
				t.Fatalf("mutation ref = %+v, want %+v", store.mutationRef, account.runtimeRef)
			}
		})
	}
}

func TestServiceReportRuntimeHealthRejectsInvalidOrStaleUpdate(t *testing.T) {
	service := newServiceWithDependencies(&runtimeHealthStore{}, fakeCredentialCipher{})
	if err := service.ReportRuntimeHealth(context.Background(), ActiveAccountCredential{}, RuntimeHealthSucceeded); !errors.Is(err, ErrRuntimeStateChanged) {
		t.Fatalf("ReportRuntimeHealth(empty ref) error = %v, want ErrRuntimeStateChanged", err)
	}
	validAccount := ActiveAccountCredential{runtimeRef: runtimeCredentialRef{
		accountID: "source", expectedCiphertext: "encrypted:current", expectedUpdatedAt: time.Now().UTC(),
	}}
	if err := service.ReportRuntimeHealth(context.Background(), validAccount, RuntimeHealthOutcome("unknown")); !errors.Is(err, ErrRuntimeHealthOutcomeInvalid) {
		t.Fatalf("ReportRuntimeHealth(invalid outcome) error = %v, want ErrRuntimeHealthOutcomeInvalid", err)
	}

	store := &runtimeHealthStore{mutationErr: ErrRuntimeStateChanged}
	service = newServiceWithDependencies(store, fakeCredentialCipher{})
	account := ActiveAccountCredential{runtimeRef: runtimeCredentialRef{
		accountID: "source", expectedCiphertext: "encrypted:old", expectedUpdatedAt: time.Now().UTC(),
	}}
	if err := service.ReportRuntimeHealth(context.Background(), account, RuntimeHealthProviderUnavailable); !errors.Is(err, ErrRuntimeStateChanged) {
		t.Fatalf("ReportRuntimeHealth(stale) error = %v, want ErrRuntimeStateChanged", err)
	}
}
