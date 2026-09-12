package p115account

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/konghang/ember/backend/internal/models"
)

// TestRuntimeSuccessSamplingUsesAcquiredSnapshot checks both credential paths,
// so sampling cannot accidentally depend on another read or an unbounded cache.
func TestRuntimeSuccessSamplingUsesAcquiredSnapshot(t *testing.T) {
	now := time.Date(2026, 9, 12, 12, 0, 0, 0, time.UTC)
	for _, role := range []models.P115AccountRole{models.P115AccountRoleSource, models.P115AccountRolePlayback} {
		t.Run(string(role), func(t *testing.T) {
			last := now.Add(-10 * time.Second)
			account := &models.P115Account{ID: "account", Role: role, Enabled: true, Status: models.P115AccountStatusActive,
				CookieCiphertext: stringPointer("encrypted:cookie"), AppType: stringPointer("web"), UserAgent: stringPointer("fixture-agent"),
				ProviderUserID: stringPointer("100"), ConfigVersion: 3, UpdatedAt: last, LastSucceededAt: &last}
			if role == models.P115AccountRoleSource {
				account.EmbyPathPrefix = stringPointer("/media")
				account.SourceRootID = stringPointer("0")
			} else {
				account.TargetParentID = stringPointer("200")
				account.TargetParentPath = stringPointer("/Playback")
				account.MaxConcurrentStreams = intPointer(3)
			}
			store := &runtimeHealthStore{fakeAccountStore: fakeAccountStore{accounts: map[string]*models.P115Account{account.ID: account}}}
			service := newServiceWithDependencies(store, fakeCredentialCipher{})
			service.now = func() time.Time { return now }
			acquire := func() ActiveAccountCredential {
				t.Helper()
				var credential ActiveAccountCredential
				var err error
				if role == models.P115AccountRoleSource {
					credential, err = service.LoadActiveCredentialByRole(context.Background(), role)
				} else {
					credential, err = service.AcquirePlaybackRoute(context.Background(), PlaybackRoute{AccountID: account.ID, ProviderUserID: "100", TargetParentID: "200", TargetParentPath: "/Playback", ConfiguredMaxConcurrentStreams: 3, ConfigVersion: 3})
				}
				if err != nil {
					t.Fatal(err)
				}
				return credential
			}
			credential := acquire()
			if err := service.ReportRuntimeHealth(context.Background(), credential, RuntimeHealthSucceeded); err != nil {
				t.Fatal(err)
			}
			if store.mutationCalls != 0 {
				t.Fatalf("recent healthy success issued %d writes", store.mutationCalls)
			}
			// Sampling never suppresses or caches a failure.
			if err := service.ReportRuntimeHealth(context.Background(), credential, RuntimeHealthProviderUnavailable); err != nil {
				t.Fatal(err)
			}
			if store.mutationCalls != 1 || store.mutation.Status != models.P115AccountStatusCoolingDown {
				t.Fatal("failure was sampled")
			}
			store.mutationCalls = 0
			now = last.Add(time.Minute)
			if err := service.ReportRuntimeHealth(context.Background(), credential, RuntimeHealthSucceeded); err != nil {
				t.Fatal(err)
			}
			if store.mutationCalls != 1 {
				t.Fatal("sampling boundary must write")
			}
			// A configuration write after the last success requires a fresh sample.
			now = last.Add(20 * time.Second)
			account.UpdatedAt = now.Add(-time.Second)
			account.LastSucceededAt = &last
			store.mutationCalls = 0
			if err := service.ReportRuntimeHealth(context.Background(), acquire(), RuntimeHealthSucceeded); err != nil {
				t.Fatal(err)
			}
			if store.mutationCalls != 1 {
				t.Fatal("configuration change hid the first success")
			}
		})
	}
}

// TestRuntimeSuccessSamplingAlwaysPersistsRecovery prevents a recent success
// from keeping an expired cooldown or uncleared error alive after a good probe.
func TestRuntimeSuccessSamplingAlwaysPersistsRecovery(t *testing.T) {
	now := time.Date(2026, 9, 12, 12, 0, 0, 0, time.UTC)
	for _, state := range []string{"first", "cooldown", "error", "write_failure"} {
		t.Run(state, func(t *testing.T) {
			last := now.Add(-10 * time.Second)
			account := &models.P115Account{ID: "source", Role: models.P115AccountRoleSource, Enabled: true,
				Status: models.P115AccountStatusActive, UpdatedAt: last, LastSucceededAt: &last,
				CookieCiphertext: stringPointer("encrypted:cookie"), ProviderUserID: stringPointer("100"),
				AppType: stringPointer("web"), UserAgent: stringPointer("fixture-agent"), EmbyPathPrefix: stringPointer("/media"), SourceRootID: stringPointer("0")}
			if state == "first" {
				account.LastSucceededAt = nil
			}
			if state == "cooldown" {
				account.Status = models.P115AccountStatusCoolingDown
				until := now.Add(-time.Second)
				account.CooldownUntil = &until
			}
			if state == "error" {
				account.LastErrorCode = stringPointer("provider_unavailable")
			}
			store := &runtimeHealthStore{fakeAccountStore: fakeAccountStore{accounts: map[string]*models.P115Account{account.ID: account}}}
			if state == "cooldown" {
				store.acquiredAccount = account
			}
			service := newServiceWithDependencies(store, fakeCredentialCipher{})
			service.now = func() time.Time { return now }
			if state == "write_failure" {
				account.LastSucceededAt = nil
				store.mutationErr = errors.New("fixture write failure")
			}
			credential, err := service.LoadActiveCredentialByRole(context.Background(), models.P115AccountRoleSource)
			if err != nil {
				t.Fatal(err)
			}
			for i := 0; i < 2; i++ {
				err = service.ReportRuntimeHealth(context.Background(), credential, RuntimeHealthSucceeded)
				if !errors.Is(err, store.mutationErr) {
					t.Fatal(err)
				}
			}
			if store.mutationCalls != 2 {
				t.Fatalf("recovery or retry sampled: calls=%d", store.mutationCalls)
			}
		})
	}
}

// TestSampledSuccessCannotClearConcurrentFailure preserves the CAS boundary:
// sampling skips only observations and cannot undo a newer cooling transition.
func TestSampledSuccessCannotClearConcurrentFailure(t *testing.T) {
	now := time.Date(2026, 9, 12, 12, 0, 0, 0, time.UTC)
	last := now.Add(-10 * time.Second)
	account := &models.P115Account{ID: "source", Role: models.P115AccountRoleSource, Status: models.P115AccountStatusActive, Enabled: true,
		ConfigVersion: 3, UpdatedAt: last, LastSucceededAt: &last, CookieCiphertext: stringPointer("encrypted:cookie"),
		AppType: stringPointer("web"), UserAgent: stringPointer("fixture-agent"), ProviderUserID: stringPointer("100"),
		EmbyPathPrefix: stringPointer("/media"), SourceRootID: stringPointer("0")}
	store := &fakeAccountStore{accounts: map[string]*models.P115Account{account.ID: account}}
	service := newServiceWithDependencies(store, fakeCredentialCipher{})
	service.now = func() time.Time { return now }
	credential, err := service.LoadActiveCredentialByRole(context.Background(), models.P115AccountRoleSource)
	if err != nil {
		t.Fatal(err)
	}
	if err := service.ReportRuntimeHealth(context.Background(), credential, RuntimeHealthSucceeded); err != nil {
		t.Fatal(err)
	}
	if err := service.ReportRuntimeHealth(context.Background(), credential, RuntimeHealthProviderUnavailable); err != nil {
		t.Fatal(err)
	}
	if err := service.ReportRuntimeHealth(context.Background(), credential, RuntimeHealthSucceeded); err != nil {
		t.Fatal(err)
	}
	if account.Status != models.P115AccountStatusCoolingDown || account.CooldownUntil == nil || !account.LastSucceededAt.Equal(last) {
		t.Fatal("sampled success cleared a newer failure")
	}
	now = last.Add(time.Minute)
	if err := service.ReportRuntimeHealth(context.Background(), credential, RuntimeHealthSucceeded); !errors.Is(err, ErrRuntimeStateChanged) {
		t.Fatalf("old snapshot bypassed CAS after sampling window: %v", err)
	}
}
