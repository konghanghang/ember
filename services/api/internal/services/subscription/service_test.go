package subscription

import (
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
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
	t.Cleanup(func() {
		newSubscriptionEmbyLookup = originalFactory
	})

	newSubscriptionEmbyLookup = func() subscriptionEmbyLookup {
		return &stubSubscriptionEmbyLookup{
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
	}

	got, err := resolveSeriesTMDBIDBySeriesID("series_81812")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if got != "123456" {
		t.Fatalf("expected 123456, got %s", got)
	}
}

func TestResolveSeriesTMDBIDBySeriesIDReturnsEmptyWhenSeriesNotFound(t *testing.T) {
	originalFactory := newSubscriptionEmbyLookup
	t.Cleanup(func() {
		newSubscriptionEmbyLookup = originalFactory
	})

	newSubscriptionEmbyLookup = func() subscriptionEmbyLookup {
		return &stubSubscriptionEmbyLookup{
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
	}

	got, err := resolveSeriesTMDBIDBySeriesID("series_81812")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if got != "654321" {
		t.Fatalf("expected 654321, got %s", got)
	}
}

func TestIsSubscriptionUniqueConflictDetectsPostgresDuplicateKey(t *testing.T) {
	err := &pgconn.PgError{Code: "23505"}
	if !isSubscriptionUniqueConflict(err) {
		t.Fatal("expected postgres duplicate key to be treated as subscription duplicate")
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
