package subscription

import (
	"errors"
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/konghang/ember/backend/internal/models"
)

func TestFormatNotificationTimeUsesConfiguredTimezone(t *testing.T) {
	t.Setenv("CRON_TIMEZONE", "Asia/Shanghai")

	reviewedAt := time.Date(2026, 4, 18, 7, 0, 41, 0, time.UTC)
	formatted := formatNotificationTime(&reviewedAt)
	if formatted == nil {
		t.Fatal("expected formatted time, got nil")
	}

	const want = "2026-04-18T15:00:41+08:00"
	if *formatted != want {
		t.Fatalf("expected %s, got %s", want, *formatted)
	}
}

type stubSubscriptionEmbyLookup struct {
	configured bool
	responses  map[string]stubSubscriptionEmbyLookupResponse
	callCount  int
}

type stubSubscriptionEmbyLookupResponse struct {
	expectedParams map[string]string
	body           []byte
	err            error
}

func (s *stubSubscriptionEmbyLookup) IsConfigured() bool {
	return s.configured
}

func (s *stubSubscriptionEmbyLookup) GetWithAPIKey(path string, params map[string]string) ([]byte, error) {
	s.callCount++
	resp, ok := s.responses[path]
	if !ok {
		return nil, fmt.Errorf("unexpected path: %s", path)
	}
	for key, expectedValue := range resp.expectedParams {
		if params[key] != expectedValue {
			return nil, fmt.Errorf("unexpected param %s: %s", key, params[key])
		}
	}
	return resp.body, resp.err
}

func TestResolveSeriesTMDBIDBySeriesID(t *testing.T) {
	originalFactory := newSubscriptionEmbyLookup
	originalNow := subscriptionSeriesTMDBLookupNow
	t.Cleanup(func() {
		newSubscriptionEmbyLookup = originalFactory
		subscriptionSeriesTMDBLookupNow = originalNow
		resetSubscriptionSeriesTMDBLookupCache()
	})
	resetSubscriptionSeriesTMDBLookupCache()
	subscriptionSeriesTMDBLookupNow = func() time.Time {
		return time.Date(2026, 4, 27, 10, 0, 0, 0, time.UTC)
	}

	lookup := &stubSubscriptionEmbyLookup{
		configured: true,
		responses: map[string]stubSubscriptionEmbyLookupResponse{
			"/emby/Items": {
				expectedParams: map[string]string{
					"Ids":    "series_81812",
					"Fields": "ProviderIds",
				},
				body: []byte(`{"Items":[{"ProviderIds":{"Tmdb":"123456"}}]}`),
			},
		},
	}
	newSubscriptionEmbyLookup = func() subscriptionEmbyLookup {
		return lookup
	}

	got, err := resolveSeriesTMDBIDBySeriesID("series_81812")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if got != "123456" {
		t.Fatalf("expected 123456, got %s", got)
	}
	if lookup.callCount != 1 {
		t.Fatalf("expected 1 emby lookup, got %d", lookup.callCount)
	}

	got, err = resolveSeriesTMDBIDBySeriesID("series_81812")
	if err != nil {
		t.Fatalf("expected cached lookup without error, got %v", err)
	}
	if got != "123456" {
		t.Fatalf("expected cached 123456, got %s", got)
	}
	if lookup.callCount != 1 {
		t.Fatalf("expected cached lookup to avoid extra emby call, got %d", lookup.callCount)
	}
}

func TestResolveSeriesTMDBIDBySeriesIDReturnsEmptyWhenSeriesNotFound(t *testing.T) {
	originalFactory := newSubscriptionEmbyLookup
	originalNow := subscriptionSeriesTMDBLookupNow
	t.Cleanup(func() {
		newSubscriptionEmbyLookup = originalFactory
		subscriptionSeriesTMDBLookupNow = originalNow
		resetSubscriptionSeriesTMDBLookupCache()
	})
	resetSubscriptionSeriesTMDBLookupCache()
	subscriptionSeriesTMDBLookupNow = func() time.Time {
		return time.Date(2026, 4, 27, 10, 0, 0, 0, time.UTC)
	}

	lookup := &stubSubscriptionEmbyLookup{
		configured: true,
		responses: map[string]stubSubscriptionEmbyLookupResponse{
			"/emby/Items": {
				expectedParams: map[string]string{
					"Ids":    "series_81812",
					"Fields": "ProviderIds",
				},
				body: []byte(`{"Items":[]}`),
			},
			"/emby/Items/series_81812": {
				expectedParams: map[string]string{
					"Fields": "ProviderIds",
				},
				body: []byte(`{"ProviderIds":{"Tmdb":"654321"}}`),
			},
		},
	}
	newSubscriptionEmbyLookup = func() subscriptionEmbyLookup {
		return lookup
	}

	got, err := resolveSeriesTMDBIDBySeriesID("series_81812")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if got != "654321" {
		t.Fatalf("expected 654321, got %s", got)
	}
	if lookup.callCount != 2 {
		t.Fatalf("expected 2 emby lookups, got %d", lookup.callCount)
	}
}

func TestResolveSeriesTMDBIDBySeriesIDCacheExpires(t *testing.T) {
	originalFactory := newSubscriptionEmbyLookup
	originalNow := subscriptionSeriesTMDBLookupNow
	t.Cleanup(func() {
		newSubscriptionEmbyLookup = originalFactory
		subscriptionSeriesTMDBLookupNow = originalNow
		resetSubscriptionSeriesTMDBLookupCache()
	})
	resetSubscriptionSeriesTMDBLookupCache()

	currentTime := time.Date(2026, 4, 27, 10, 0, 0, 0, time.UTC)
	subscriptionSeriesTMDBLookupNow = func() time.Time {
		return currentTime
	}

	lookup := &stubSubscriptionEmbyLookup{
		configured: true,
		responses: map[string]stubSubscriptionEmbyLookupResponse{
			"/emby/Items": {
				expectedParams: map[string]string{
					"Ids":    "series_81812",
					"Fields": "ProviderIds",
				},
				body: []byte(`{"Items":[{"ProviderIds":{"Tmdb":"123456"}}]}`),
			},
		},
	}
	newSubscriptionEmbyLookup = func() subscriptionEmbyLookup {
		return lookup
	}

	if _, err := resolveSeriesTMDBIDBySeriesID("series_81812"); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if lookup.callCount != 1 {
		t.Fatalf("expected 1 emby lookup, got %d", lookup.callCount)
	}

	currentTime = currentTime.Add(subscriptionSeriesTMDBLookupCacheTTL + time.Second)
	if _, err := resolveSeriesTMDBIDBySeriesID("series_81812"); err != nil {
		t.Fatalf("expected no error after cache expiry, got %v", err)
	}
	if lookup.callCount != 2 {
		t.Fatalf("expected cache expiry to trigger second emby lookup, got %d", lookup.callCount)
	}
}

func resetSubscriptionSeriesTMDBLookupCache() {
	subscriptionSeriesTMDBLookupCache.mu.Lock()
	subscriptionSeriesTMDBLookupCache.entries = make(map[string]seriesTMDBLookupCacheEntry)
	subscriptionSeriesTMDBLookupCache.mu.Unlock()
}

func TestIsSubscriptionUniqueConflictDetectsPostgresDuplicateKey(t *testing.T) {
	err := &pgconn.PgError{Code: "23505"}
	if !isSubscriptionUniqueConflict(err) {
		t.Fatal("expected postgres duplicate key to be treated as subscription duplicate")
	}
}

func TestShouldSyncAdminNotificationsAfterDelivery(t *testing.T) {
	cases := []struct {
		name   string
		status models.SubscriptionStatus
		want   bool
	}{
		{name: "pending", status: models.SubscriptionPending, want: false},
		{name: "approved", status: models.SubscriptionApproved, want: true},
		{name: "rejected", status: models.SubscriptionRejected, want: true},
		{name: "ingested", status: models.SubscriptionIngested, want: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := shouldSyncAdminNotificationsAfterDelivery(tc.status); got != tc.want {
				t.Fatalf("expected %v, got %v", tc.want, got)
			}
		})
	}
}

func TestSyncAdminNotificationsAfterDeliveryIfHandledTriggersForHandledSubscription(t *testing.T) {
	originalLoad := loadSubscriptionForAdminNotificationSync
	originalSync := runAdminNotificationSync
	t.Cleanup(func() {
		loadSubscriptionForAdminNotificationSync = originalLoad
		runAdminNotificationSync = originalSync
	})

	var loadedID string
	loadSubscriptionForAdminNotificationSync = func(subscriptionID string) (models.Subscription, error) {
		loadedID = subscriptionID
		return models.Subscription{
			ID:     subscriptionID,
			Status: models.SubscriptionApproved,
		}, nil
	}

	var synced []models.Subscription
	runAdminNotificationSync = func(_ *SubscriptionService, subscription models.Subscription) {
		synced = append(synced, subscription)
	}

	service := &SubscriptionService{}
	service.syncAdminNotificationsAfterDeliveryIfHandled("sub_approved")

	if loadedID != "sub_approved" {
		t.Fatalf("expected loader to receive subscription id, got %s", loadedID)
	}
	if len(synced) != 1 {
		t.Fatalf("expected one admin notification sync, got %d", len(synced))
	}
	if synced[0].ID != "sub_approved" || synced[0].Status != models.SubscriptionApproved {
		t.Fatalf("unexpected synced subscription: %+v", synced[0])
	}
}

func TestSyncAdminNotificationsAfterDeliveryIfHandledSkipsPendingAndLoadError(t *testing.T) {
	originalLoad := loadSubscriptionForAdminNotificationSync
	originalSync := runAdminNotificationSync
	t.Cleanup(func() {
		loadSubscriptionForAdminNotificationSync = originalLoad
		runAdminNotificationSync = originalSync
	})

	syncCount := 0
	runAdminNotificationSync = func(_ *SubscriptionService, subscription models.Subscription) {
		syncCount++
	}

	loadSubscriptionForAdminNotificationSync = func(subscriptionID string) (models.Subscription, error) {
		return models.Subscription{
			ID:     subscriptionID,
			Status: models.SubscriptionPending,
		}, nil
	}

	service := &SubscriptionService{}
	service.syncAdminNotificationsAfterDeliveryIfHandled("sub_pending")
	if syncCount != 0 {
		t.Fatalf("expected pending subscription to skip sync, got %d", syncCount)
	}

	loadSubscriptionForAdminNotificationSync = func(subscriptionID string) (models.Subscription, error) {
		return models.Subscription{}, errors.New("load failed")
	}
	service.syncAdminNotificationsAfterDeliveryIfHandled("sub_missing")
	if syncCount != 0 {
		t.Fatalf("expected load error to skip sync, got %d", syncCount)
	}
}

func TestBuildWholeShowProgressStatsCountsFullAiredInventory(t *testing.T) {
	required := make([]wholeShowEpisodeRef, 0, 10)
	inventory := make(subscriptionEpisodeInventory)
	inventory[1] = make(map[int]struct{})
	for episode := 1; episode <= 10; episode++ {
		required = append(required, wholeShowEpisodeRef{Season: 1, Episode: episode})
		if episode != 9 {
			inventory[1][episode] = struct{}{}
		}
	}

	stats := buildWholeShowProgressStats(required, inventory, nil)
	if stats.Total != 10 {
		t.Fatalf("expected total aired episodes=10, got %d", stats.Total)
	}
	if stats.Done != 9 {
		t.Fatalf("expected done episodes=9, got %d", stats.Done)
	}
}

func TestBuildWholeShowProgressStatsExcludesIgnoredEpisodes(t *testing.T) {
	required := []wholeShowEpisodeRef{
		{Season: 1, Episode: 1},
		{Season: 1, Episode: 2},
		{Season: 1, Episode: 3},
		{Season: 1, Episode: 4},
	}
	inventory := subscriptionEpisodeInventory{
		1: {
			1: {},
			2: {},
			4: {},
		},
	}
	ignored := wholeShowIgnoredEpisodeSet{
		buildWholeShowEpisodeKey(1, 4): {},
	}

	stats := buildWholeShowProgressStats(required, inventory, ignored)
	if stats.Total != 3 {
		t.Fatalf("expected ignored episode to be excluded from total, got %d", stats.Total)
	}
	if stats.Done != 2 {
		t.Fatalf("expected done episodes without ignored item=2, got %d", stats.Done)
	}
}

func TestBuildSubscriptionEpisodeInventoryKeepsPhysicalEpisodesOnly(t *testing.T) {
	items := []embyExistingItem{
		{ParentIndexNumber: 1, IndexNumber: 1, Path: "/library/ep1.mkv"},
		{ParentIndexNumber: 1, IndexNumber: 2, LocationType: "Virtual"},
		{ParentIndexNumber: 1, IndexNumber: 3, IsMissing: true},
		{ParentIndexNumber: 2, IndexNumber: 1, MediaSources: []interface{}{"source"}},
	}

	inventory := buildSubscriptionEpisodeInventory(items)
	var got []string
	for season, episodes := range inventory {
		for episode := range episodes {
			got = append(got, fmt.Sprintf("%d:%d", season, episode))
		}
	}
	slices.Sort(got)

	want := []string{"1:1", "2:1"}
	if !slices.Equal(got, want) {
		t.Fatalf("expected inventory=%v, got %v", want, got)
	}
}
