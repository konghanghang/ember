package p115account

import (
	"context"
	"testing"
	"time"

	"github.com/konghang/ember/backend/internal/models"
	"github.com/konghang/ember/backend/internal/services/p115quota"
)

func TestServiceAccountSummariesReadLeaseUsageWithoutExposingProviderUIDInKeys(t *testing.T) {
	now := time.Date(2026, 9, 4, 0, 0, 0, 0, time.UTC)
	providerUserID := "100"
	maxStreams := 3
	account := &models.P115Account{
		ID: "shared", Role: models.P115AccountRolePlayback, ProviderUserID: &providerUserID,
		MaxConcurrentStreams: &maxStreams, Status: models.P115AccountStatusActive, Enabled: true,
	}
	store := &fakeAccountStore{accounts: map[string]*models.P115Account{"shared": account}}
	leases := p115quota.NewMemoryLeaseStore()
	deriver, err := p115quota.NewKeyDeriver("fixture-root-key")
	if err != nil {
		t.Fatal(err)
	}
	accountKey, err := deriver.PlaybackAccountKey(providerUserID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := leases.Reserve(context.Background(), p115quota.ReserveRequest{
		PlaybackAccountKey: accountKey, UserID: "user-1",
		SessionFingerprint:   "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		MaxConcurrentStreams: 3,
	}, now); err != nil {
		t.Fatal(err)
	}

	service := newServiceWithDependencies(store, fakeCredentialCipher{})
	service.leases = leases
	service.keyDeriver = deriver
	service.now = func() time.Time { return now }
	items, err := service.List(context.Background())
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(items) != 1 || items[0].UsageAvailable == nil || !*items[0].UsageAvailable ||
		items[0].ReservedStreams != 1 || items[0].ActiveStreams != 0 || items[0].OccupiedStreams != 1 {
		t.Fatalf("summary = %+v", items)
	}
}

func TestServicePersonalSummaryDistinguishesUnavailableUsage(t *testing.T) {
	configured := 2
	providerUserID := "100"
	store := &fakeAccountStore{
		accounts: map[string]*models.P115Account{
			"personal": {
				ID: "personal", OwnerUserID: stringPointer("user-1"), Role: models.P115AccountRolePlayback,
				ProviderUserID: &providerUserID, MaxConcurrentStreams: &configured, Status: models.P115AccountStatusActive,
			},
		},
		personalPolicy: PersonalPlanPolicy{
			PlanGroupKey: "VIP", PlaybackMode: models.P115PlaybackModePersonal,
			TransferHourlyLimit: 5, TransferDailyLimit: 10, SimultaneousStreamLimit: 3,
		},
	}
	deriver, err := p115quota.NewKeyDeriver("fixture-root-key")
	if err != nil {
		t.Fatal(err)
	}
	service := newServiceWithDependencies(store, fakeCredentialCipher{})
	service.leases = p115quota.UnavailableLeaseStore{}
	service.keyDeriver = deriver

	summary, err := service.GetPersonalAccount(context.Background(), "user-1")
	if err != nil {
		t.Fatalf("GetPersonalAccount() error = %v", err)
	}
	if summary.UsageAvailable || summary.ReservedStreams != nil || summary.ActiveStreams != nil || summary.OccupiedStreams != nil {
		t.Fatalf("summary = %+v", summary)
	}
}

func TestServicePersonalSummaryIncludesAccountUserAndTransferUsage(t *testing.T) {
	now := time.Date(2026, 9, 4, 12, 0, 0, 0, time.UTC)
	configured := 2
	providerUserID := "100"
	store := &fakeAccountStore{
		accounts: map[string]*models.P115Account{
			"personal": {
				ID: "personal", OwnerUserID: stringPointer("user-1"), Role: models.P115AccountRolePlayback,
				ProviderUserID: &providerUserID, MaxConcurrentStreams: &configured, Status: models.P115AccountStatusActive,
			},
		},
		personalPolicy: PersonalPlanPolicy{
			PlanGroupKey: "VIP", PlaybackMode: models.P115PlaybackModePersonal,
			TransferHourlyLimit: 5, TransferDailyLimit: 10, SimultaneousStreamLimit: 3,
		},
	}
	quotas := p115quota.NewMemoryLeaseStore()
	deriver, err := p115quota.NewKeyDeriver("fixture-root-key")
	if err != nil {
		t.Fatal(err)
	}
	accountKey, _ := deriver.PlaybackAccountKey(providerUserID)
	fingerprint := "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	if _, err := quotas.Reserve(context.Background(), p115quota.ReserveRequest{
		PlaybackAccountKey: accountKey, UserID: "user-1", SessionFingerprint: fingerprint, MaxConcurrentStreams: 2,
	}, now); err != nil {
		t.Fatal(err)
	}
	if _, err := quotas.Advance(context.Background(), fingerprint, p115quota.LeaseStateActive, now); err != nil {
		t.Fatal(err)
	}
	dayStart, dayEnd := p115quota.DayWindow(now, time.UTC)
	if _, err := quotas.ReserveTransfer(context.Background(), p115quota.TransferReserveRequest{
		UserID: "user-1", AttemptID: "attempt-1", HourlyLimit: 5, DailyLimit: 10, DayStart: dayStart, DayEnd: dayEnd,
	}, now); err != nil {
		t.Fatal(err)
	}
	if _, err := quotas.CommitTransfer(context.Background(), p115quota.TransferCommitRequest{
		UserID: "user-1", AttemptID: "attempt-1", DayStart: dayStart, DayEnd: dayEnd,
	}, now); err != nil {
		t.Fatal(err)
	}

	service := newServiceWithDependencies(store, fakeCredentialCipher{})
	service.leases = quotas
	service.keyDeriver = deriver
	service.businessTimezone = time.UTC
	service.now = func() time.Time { return now }
	summary, err := service.GetPersonalAccount(context.Background(), "user-1")
	if err != nil {
		t.Fatalf("GetPersonalAccount() error = %v", err)
	}
	if !summary.UsageAvailable || summary.ActiveStreams == nil || *summary.ActiveStreams != 1 ||
		summary.UserActiveStreams == nil || *summary.UserActiveStreams != 1 ||
		summary.TransferHourlyUsed == nil || *summary.TransferHourlyUsed != 1 ||
		summary.TransferDailyUsed == nil || *summary.TransferDailyUsed != 1 {
		t.Fatalf("summary = %+v", summary)
	}
}

func TestServiceGetPersonalUsageWorksForSystemModeWithoutPersonalAccount(t *testing.T) {
	now := time.Date(2026, 9, 4, 12, 0, 0, 0, time.UTC)
	store := &fakeAccountStore{personalPolicy: PersonalPlanPolicy{
		PlanGroupKey: "VIP", PlaybackMode: models.P115PlaybackModeSystem,
		TransferHourlyLimit: 5, TransferDailyLimit: 10, SimultaneousStreamLimit: 3,
	}}
	quotas := p115quota.NewMemoryLeaseStore()
	service := newServiceWithDependencies(store, fakeCredentialCipher{})
	service.leases = quotas
	service.businessTimezone = time.UTC
	service.now = func() time.Time { return now }

	usage, err := service.GetPersonalUsage(context.Background(), "user-1")
	if err != nil {
		t.Fatalf("GetPersonalUsage() error = %v", err)
	}
	if !usage.UsageAvailable || usage.TransferHourlyLimit != 5 || usage.TransferDailyLimit != 10 ||
		usage.UserReservedStreams == nil || *usage.UserReservedStreams != 0 || usage.TransferHourlyUsed == nil || *usage.TransferHourlyUsed != 0 {
		t.Fatalf("usage = %+v", usage)
	}
}
