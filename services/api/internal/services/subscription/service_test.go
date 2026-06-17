package subscription

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	moviepilotint "github.com/konghang/ember/backend/internal/integrations/moviepilot"
	"github.com/konghang/ember/backend/internal/models"
	"gorm.io/gorm"
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

func TestSubscriptionTimeAndStringHelpers(t *testing.T) {
	parsed, ok := parseSubscriptionTMDBAirDate(" 2026-04-19 ")
	if !ok {
		t.Fatal("expected valid TMDB air date")
	}
	wantDate := time.Date(2026, 4, 19, 0, 0, 0, 0, time.UTC)
	if !parsed.Equal(wantDate) {
		t.Fatalf("expected normalized UTC date %s, got %s", wantDate, parsed)
	}

	for _, value := range []string{"", "2026/04/19", "not-a-date"} {
		t.Run(value, func(t *testing.T) {
			if got, ok := parseSubscriptionTMDBAirDate(value); ok || !got.IsZero() {
				t.Fatalf("expected invalid date to return zero false, got %s %v", got, ok)
			}
		})
	}

	loc := time.FixedZone("UTC+8", 8*60*60)
	now := time.Date(2026, 4, 18, 18, 30, 0, 0, time.UTC)
	calendarDate := subscriptionCalendarDateInLocation(now, loc)
	wantCalendarDate := time.Date(2026, 4, 19, 0, 0, 0, 0, time.UTC)
	if !calendarDate.Equal(wantCalendarDate) {
		t.Fatalf("expected calendar date %s, got %s", wantCalendarDate, calendarDate)
	}
	if got := subscriptionCalendarDateInLocation(wantDate, nil); !got.Equal(wantDate) {
		t.Fatalf("expected nil location to use UTC date, got %s", got)
	}

	text := "value"
	if got := ptrToString(&text); got != "value" {
		t.Fatalf("expected ptrToString to unwrap value, got %q", got)
	}
	if got := ptrToString(nil); got != "" {
		t.Fatalf("expected nil ptrToString to return blank, got %q", got)
	}
	if got := stringPointerValue(&text); got != "value" {
		t.Fatalf("expected stringPointerValue to unwrap value, got %q", got)
	}
	if got := stringPointerValue(nil); got != "" {
		t.Fatalf("expected nil stringPointerValue to return blank, got %q", got)
	}
	if formatNotificationTime(nil) != nil {
		t.Fatal("expected nil notification time to stay nil")
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

func TestSeriesTMDBIDCacheStoresHitsAndEvictsExpiredEntries(t *testing.T) {
	originalNow := subscriptionSeriesTMDBLookupNow
	t.Cleanup(func() {
		subscriptionSeriesTMDBLookupNow = originalNow
		resetSubscriptionSeriesTMDBLookupCache()
	})
	resetSubscriptionSeriesTMDBLookupCache()

	currentTime := time.Date(2026, 4, 27, 10, 0, 0, 0, time.UTC)
	subscriptionSeriesTMDBLookupNow = func() time.Time {
		return currentTime
	}

	cacheSeriesTMDBID(" series_1 ", "1399")
	if got, ok := getCachedSeriesTMDBID(" series_1 "); !ok || got != "1399" {
		t.Fatalf("expected cached tmdb id 1399, got %q %v", got, ok)
	}

	currentTime = currentTime.Add(subscriptionSeriesTMDBLookupCacheTTL + time.Second)
	if got, ok := getCachedSeriesTMDBID(" series_1 "); ok || got != "" {
		t.Fatalf("expected expired cache miss, got %q %v", got, ok)
	}
}

func TestResolveWebhookMatchTMDBIDsUsesPayloadAndSeriesLookup(t *testing.T) {
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

	service := &SubscriptionService{}
	lookup := &stubSubscriptionEmbyLookup{
		configured: true,
		responses: map[string]stubSubscriptionEmbyLookupResponse{
			"/emby/Items": {
				expectedParams: map[string]string{
					"Ids":    "series_81812",
					"Fields": "ProviderIds",
				},
				body: []byte(`{"Items":[{"ProviderIds":{"Tmdb":"1399"}}]}`),
			},
		},
	}
	newSubscriptionEmbyLookup = func() subscriptionEmbyLookup {
		return lookup
	}

	got := service.resolveWebhookMatchTMDBIDs("episode", SubscriptionIngestWebhookPayload{
		TmdbID:   " 27205 ",
		SeriesID: " series_81812 ",
	})
	if !slices.Equal(got, []string{"27205", "1399"}) {
		t.Fatalf("expected payload tmdb id plus resolved series tmdb id, got %+v", got)
	}
	if lookup.callCount != 1 {
		t.Fatalf("expected one series lookup, got %d", lookup.callCount)
	}

	got = service.resolveWebhookMatchTMDBIDs("episode", SubscriptionIngestWebhookPayload{
		TmdbID:   "1399",
		SeriesID: "series_81812",
	})
	if !slices.Equal(got, []string{"1399"}) {
		t.Fatalf("expected duplicate series tmdb id to be deduped, got %+v", got)
	}
	if lookup.callCount != 1 {
		t.Fatalf("expected second call to use cache, got %d lookups", lookup.callCount)
	}

	got = service.resolveWebhookMatchTMDBIDs("movie", SubscriptionIngestWebhookPayload{
		TmdbID:   " 27205 ",
		SeriesID: "series_ignored",
	})
	if !slices.Equal(got, []string{"27205"}) {
		t.Fatalf("expected movie webhook to use payload tmdb id only, got %+v", got)
	}
}

func TestResolveWebhookMatchTMDBIDsKeepsPayloadWhenSeriesLookupUnavailable(t *testing.T) {
	originalFactory := newSubscriptionEmbyLookup
	t.Cleanup(func() {
		newSubscriptionEmbyLookup = originalFactory
		resetSubscriptionSeriesTMDBLookupCache()
	})
	resetSubscriptionSeriesTMDBLookupCache()

	newSubscriptionEmbyLookup = func() subscriptionEmbyLookup {
		return &stubSubscriptionEmbyLookup{configured: false}
	}

	service := &SubscriptionService{}
	got := service.resolveWebhookMatchTMDBIDs("episode", SubscriptionIngestWebhookPayload{
		TmdbID:   "27205",
		SeriesID: "series_unconfigured",
	})
	if !slices.Equal(got, []string{"27205"}) {
		t.Fatalf("expected payload tmdb id when lookup is unavailable, got %+v", got)
	}
}

func TestIsSubscriptionUniqueConflictDetectsPostgresDuplicateKey(t *testing.T) {
	err := &pgconn.PgError{Code: "23505"}
	if !isSubscriptionUniqueConflict(err) {
		t.Fatal("expected postgres duplicate key to be treated as subscription duplicate")
	}
}

func TestBuildConfirmationRequiredResultCopiesExistingSummary(t *testing.T) {
	checkResult := &CheckExistingResponse{
		ExistsInLibrary: true,
		DetectionFailed: true,
		ExistingSummary: &ExistingSummary{
			MatchType:       ExistingMatchMovie,
			EmbyItemID:      "emby_1",
			Message:         "已存在",
			DetectionFailed: true,
		},
	}

	result := buildConfirmationRequiredResult(checkResult)
	if result == nil || result.Success {
		t.Fatalf("expected unsuccessful confirmation result, got %+v", result)
	}
	if result.AlreadyExists || !result.ConfirmationRequired || !result.DetectionFailed {
		t.Fatalf("expected confirmation and detection flags without AlreadyExists, got %+v", result)
	}
	if result.ExistingSummary == nil || result.ExistingSummary.EmbyItemID != "emby_1" {
		t.Fatalf("expected existing summary to be copied, got %+v", result.ExistingSummary)
	}
}

func TestSubscriptionExistingHelpers(t *testing.T) {
	resp := detectionFailedResponse("检测失败")
	if !resp.DetectionFailed || resp.ExistsInLibrary {
		t.Fatalf("unexpected detection failed response: %+v", resp)
	}
	if resp.ExistingSummary == nil ||
		!resp.ExistingSummary.DetectionFailed ||
		resp.ExistingSummary.MatchType != ExistingMatchUnknown ||
		resp.ExistingSummary.Message != "检测失败" {
		t.Fatalf("unexpected detection summary: %+v", resp.ExistingSummary)
	}

	providerID := extractProviderID(map[string]string{" tmdb ": " 12345 "}, "Tmdb")
	if providerID != "12345" {
		t.Fatalf("extractProviderID() = %q, want 12345", providerID)
	}
	if extractProviderID(nil, "Tmdb") != "" {
		t.Fatalf("expected empty provider ID for nil map")
	}

	if !hasPhysicalExistingItem(embyExistingItem{Path: "/media/movie.mkv"}) {
		t.Fatalf("expected item with path to be physical")
	}
	if !hasPhysicalExistingItem(embyExistingItem{MediaSources: []interface{}{map[string]any{"id": "source"}}}) {
		t.Fatalf("expected item with media sources to be physical")
	}
	if hasPhysicalExistingItem(embyExistingItem{IsMissing: true, Path: "/media/movie.mkv"}) {
		t.Fatalf("expected missing item to be ignored")
	}
	if hasPhysicalExistingItem(embyExistingItem{LocationType: " virtual ", Path: "/media/movie.mkv"}) {
		t.Fatalf("expected virtual item to be ignored")
	}
}

func TestSummarizeEpisodeInventory(t *testing.T) {
	seasons, counts := summarizeEpisodeInventory([]embyExistingItem{
		{ParentIndexNumber: 2, IndexNumber: 1, Path: "/show/s2e1.mkv"},
		{ParentIndexNumber: 1, IndexNumber: 1, MediaSources: []interface{}{"source"}},
		{ParentIndexNumber: 1, IndexNumber: 2, Path: "/show/s1e2.mkv"},
		{ParentIndexNumber: 3, IndexNumber: 0, Path: "/invalid.mkv"},
		{ParentIndexNumber: 4, IndexNumber: 1, IsMissing: true, Path: "/missing.mkv"},
	})

	if !slices.Equal(seasons, []int{1, 2}) {
		t.Fatalf("unexpected seasons: %+v", seasons)
	}
	if counts[1] != 2 || counts[2] != 1 {
		t.Fatalf("unexpected episode counts: %+v", counts)
	}
	if totalEpisodeCount(counts) != 3 {
		t.Fatalf("unexpected total episode count: %+v", counts)
	}
}

func TestBuildSeriesExistsMessage(t *testing.T) {
	if got := buildSeriesExistsMessage(nil, 0); got != "Emby 库中已存在该剧条目，确认后仍可继续提交。" {
		t.Fatalf("unexpected empty season message: %s", got)
	}
	got := buildSeriesExistsMessage([]int{1, 3}, 18)
	if !strings.Contains(got, "已入库季：1、3") || !strings.Contains(got, "共 18 集") {
		t.Fatalf("unexpected season summary message: %s", got)
	}
}

func TestNormalizeSubscriptionSeason(t *testing.T) {
	season, err := normalizeSubscriptionSeason(models.MediaMovie, 9)
	if err != nil {
		t.Fatalf("movie season should not error: %v", err)
	}
	if season != 0 {
		t.Fatalf("movie season should normalize to 0, got %d", season)
	}

	season, err = normalizeSubscriptionSeason(models.MediaTV, 2)
	if err != nil {
		t.Fatalf("tv season should not error: %v", err)
	}
	if season != 2 {
		t.Fatalf("tv season should be preserved, got %d", season)
	}

	if _, err := normalizeSubscriptionSeason(models.MediaTV, -1); !errors.Is(err, ErrSubscriptionInvalidSeason) {
		t.Fatalf("expected invalid season error, got %v", err)
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

// stubFetchSubscriptionByID 替换 fetchSubscriptionByID 包级变量，让 prepareManualSubscription
// 在单测里不依赖真实 DB。usage 记录调用次数便于断言分支是否命中。
type stubFetchSubscriptionByID struct {
	subs  map[string]models.Subscription
	errs  map[string]error
	calls []string
}

func (s *stubFetchSubscriptionByID) install() func() {
	original := fetchSubscriptionByID
	fetchSubscriptionByID = func(subscriptionID string) (models.Subscription, error) {
		s.calls = append(s.calls, subscriptionID)
		if err, ok := s.errs[subscriptionID]; ok {
			return models.Subscription{}, err
		}
		return s.subs[subscriptionID], nil
	}
	return func() { fetchSubscriptionByID = original }
}

func newPrepareManualTestService() *SubscriptionService {
	return &SubscriptionService{}
}

type stubSubscriptionMoviePilotClient struct {
	dispatchFn func(moviepilotint.DownloadCandidateRequest) (*moviepilotint.DownloadCandidateResponse, error)
}

func (s *stubSubscriptionMoviePilotClient) IsConfigured() bool {
	return true
}

func (s *stubSubscriptionMoviePilotClient) CreateSubscription(moviepilotint.SubscribeRequest) error {
	return nil
}

func (s *stubSubscriptionMoviePilotClient) SearchMediaCandidates(moviepilotint.SearchMediaRequest) (*moviepilotint.SearchMediaResponse, error) {
	return nil, nil
}

func (s *stubSubscriptionMoviePilotClient) DispatchDownloadCandidate(req moviepilotint.DownloadCandidateRequest) (*moviepilotint.DownloadCandidateResponse, error) {
	if s.dispatchFn == nil {
		return &moviepilotint.DownloadCandidateResponse{Accepted: true}, nil
	}
	return s.dispatchFn(req)
}

type stubPersistMpError struct {
	calls []struct {
		subscriptionID string
		mpError        *string
	}
}

func (s *stubPersistMpError) install() func() {
	original := persistSubscriptionMpError
	persistSubscriptionMpError = func(subscriptionID string, mpError *string) error {
		s.calls = append(s.calls, struct {
			subscriptionID string
			mpError        *string
		}{subscriptionID: subscriptionID, mpError: mpError})
		return nil
	}
	return func() { persistSubscriptionMpError = original }
}

func TestPrepareManualSubscriptionReturnsNotFoundForMissing(t *testing.T) {
	stub := &stubFetchSubscriptionByID{errs: map[string]error{"sub_missing": gorm.ErrRecordNotFound}}
	restore := stub.install()
	defer restore()

	service := newPrepareManualTestService()
	_, _, err := service.prepareManualSubscription("sub_missing", nil, true)
	if !errors.Is(err, ErrSubscriptionNotFound) {
		t.Fatalf("expected ErrSubscriptionNotFound, got %v", err)
	}
}

func TestPrepareManualSubscriptionWrapsNonNotFoundDBError(t *testing.T) {
	dbErr := errors.New("connection refused")
	stub := &stubFetchSubscriptionByID{errs: map[string]error{"sub_db": dbErr}}
	restore := stub.install()
	defer restore()

	service := newPrepareManualTestService()
	_, _, err := service.prepareManualSubscription("sub_db", nil, true)
	if err == nil {
		t.Fatal("expected wrapped db error, got nil")
	}
	// 不能误判为 NotFound：错误链中不应包含 ErrSubscriptionNotFound。
	if errors.Is(err, ErrSubscriptionNotFound) {
		t.Fatalf("db failure must not be mapped to ErrSubscriptionNotFound, got %v", err)
	}
	// 原始 db 错误应保留在错误链中，让 handler default 分支走 500。
	if !errors.Is(err, dbErr) {
		t.Fatalf("expected wrapped error to preserve original dbErr, got %v", err)
	}
}

func TestPrepareManualSubscriptionRejectsNonApprovedStatus(t *testing.T) {
	stub := &stubFetchSubscriptionByID{subs: map[string]models.Subscription{
		"sub_pending": {ID: "sub_pending", Status: models.SubscriptionPending, TmdbID: "27205"},
	}}
	restore := stub.install()
	defer restore()

	service := newPrepareManualTestService()
	_, _, err := service.prepareManualSubscription("sub_pending", nil, true)
	if !errors.Is(err, ErrSubscriptionNotApproved) {
		t.Fatalf("expected ErrSubscriptionNotApproved, got %v", err)
	}
}

func TestPrepareManualSubscriptionRejectsEmptyTMDBID(t *testing.T) {
	// 空串会被 strconv.Atoi 失败，落入 ErrSubscriptionInvalidTMDBID（与历史空值校验语义一致）。
	stub := &stubFetchSubscriptionByID{subs: map[string]models.Subscription{
		"sub_empty": {ID: "sub_empty", Status: models.SubscriptionApproved, Type: models.MediaMovie, TmdbID: "  "},
	}}
	restore := stub.install()
	defer restore()

	service := newPrepareManualTestService()
	_, _, err := service.prepareManualSubscription("sub_empty", nil, false)
	if !errors.Is(err, ErrSubscriptionInvalidTMDBID) {
		t.Fatalf("expected ErrSubscriptionInvalidTMDBID for empty tmdbId, got %v", err)
	}
}

func TestPrepareManualSubscriptionRejectsNonNumericTMDBID(t *testing.T) {
	// P2-1：非数字 tmdbId 必须在 service 层拦截为 400，而不是落到 client.go Atoi 失败被包装成 500。
	stub := &stubFetchSubscriptionByID{subs: map[string]models.Subscription{
		"sub_bad": {ID: "sub_bad", Status: models.SubscriptionApproved, Type: models.MediaMovie, TmdbID: "tt1234"},
	}}
	restore := stub.install()
	defer restore()

	service := newPrepareManualTestService()
	_, _, err := service.prepareManualSubscription("sub_bad", nil, false)
	if !errors.Is(err, ErrSubscriptionInvalidTMDBID) {
		t.Fatalf("expected ErrSubscriptionInvalidTMDBID for non-numeric tmdbId, got %v", err)
	}
}

func TestPrepareManualSubscriptionMoviePassesWithSeasonZero(t *testing.T) {
	stub := &stubFetchSubscriptionByID{subs: map[string]models.Subscription{
		"sub_movie": {ID: "sub_movie", Status: models.SubscriptionApproved, Type: models.MediaMovie, TmdbID: "27205"},
	}}
	restore := stub.install()
	defer restore()

	service := newPrepareManualTestService()
	sub, season, err := service.prepareManualSubscription("sub_movie", nil, false)
	if err != nil {
		t.Fatalf("expected movie to pass, got %v", err)
	}
	if season != 0 {
		t.Fatalf("expected season 0 for movie, got %d", season)
	}
	if sub.ID != "sub_movie" {
		t.Fatalf("unexpected subscription returned: %s", sub.ID)
	}
}

func TestPrepareManualSubscriptionSingleSeasonTVPassesWithStoredSeason(t *testing.T) {
	stub := &stubFetchSubscriptionByID{subs: map[string]models.Subscription{
		"sub_season3": {ID: "sub_season3", Status: models.SubscriptionApproved, Type: models.MediaTV, TmdbID: "1399", Season: 3},
	}}
	restore := stub.install()
	defer restore()

	service := newPrepareManualTestService()
	_, season, err := service.prepareManualSubscription("sub_season3", nil, true)
	if err != nil {
		t.Fatalf("expected single season tv to pass, got %v", err)
	}
	if season != 3 {
		t.Fatalf("expected stored season 3, got %d", season)
	}
}

func TestPrepareManualSubscriptionWholeShowRequiresSeasonForSearch(t *testing.T) {
	// 整剧剧（season=0）+ requireTVSeason=true（搜索路径）且未传 requestedSeason → 必须报季号错误。
	stub := &stubFetchSubscriptionByID{subs: map[string]models.Subscription{
		"sub_whole": {ID: "sub_whole", Status: models.SubscriptionApproved, Type: models.MediaTV, TmdbID: "1399", Season: 0},
	}}
	restore := stub.install()
	defer restore()

	service := newPrepareManualTestService()
	_, _, err := service.prepareManualSubscription("sub_whole", nil, true)
	if !errors.Is(err, ErrSubscriptionManualSeason) {
		t.Fatalf("expected ErrSubscriptionManualSeason for whole show without season, got %v", err)
	}
}

func TestPrepareManualSubscriptionWholeShowUsesProvidedSeasonForSearch(t *testing.T) {
	// 整剧剧 + requireTVSeason=true + 显式传入 requestedSeason → 通过，season 用传入值。
	stub := &stubFetchSubscriptionByID{subs: map[string]models.Subscription{
		"sub_whole2": {ID: "sub_whole2", Status: models.SubscriptionApproved, Type: models.MediaTV, TmdbID: "1399", Season: 0},
	}}
	restore := stub.install()
	defer restore()

	requested := 2
	service := newPrepareManualTestService()
	_, season, err := service.prepareManualSubscription("sub_whole2", &requested, true)
	if err != nil {
		t.Fatalf("expected whole show with requested season to pass, got %v", err)
	}
	if season != 2 {
		t.Fatalf("expected season 2 from request, got %d", season)
	}
}

func TestPrepareManualSubscriptionRejectsNonPositiveRequestedSeason(t *testing.T) {
	// requestedSeason <= 0 同样应当触发 ErrSubscriptionManualSeason，避免传入 0 绕过季号约束。
	stub := &stubFetchSubscriptionByID{subs: map[string]models.Subscription{
		"sub_whole_zero": {ID: "sub_whole_zero", Status: models.SubscriptionApproved, Type: models.MediaTV, TmdbID: "1399", Season: 0},
	}}
	restore := stub.install()
	defer restore()

	zero := 0
	service := newPrepareManualTestService()
	_, _, err := service.prepareManualSubscription("sub_whole_zero", &zero, true)
	if !errors.Is(err, ErrSubscriptionManualSeason) {
		t.Fatalf("expected ErrSubscriptionManualSeason for non-positive requested season, got %v", err)
	}
}

func TestManualDispatchSubscriptionRequiresSeasonForWholeShow(t *testing.T) {
	stub := &stubFetchSubscriptionByID{subs: map[string]models.Subscription{
		"sub_dispatch": {ID: "sub_dispatch", Status: models.SubscriptionApproved, Type: models.MediaTV, TmdbID: "1399", Season: 0},
	}}
	restore := stub.install()
	defer restore()

	service := &SubscriptionService{moviepilot: &stubSubscriptionMoviePilotClient{}}
	_, err := service.ManualDispatchSubscription("sub_dispatch", ManualDispatchRequest{
		CandidatePayload: map[string]interface{}{"title": "Show S02"},
	})
	if !errors.Is(err, ErrSubscriptionManualSeason) {
		t.Fatalf("expected ErrSubscriptionManualSeason for whole show dispatch without season, got %v", err)
	}
}

func TestManualDispatchSubscriptionPassesSeasonToMoviePilotAndClearsMpError(t *testing.T) {
	stub := &stubFetchSubscriptionByID{subs: map[string]models.Subscription{
		"sub_dispatch": {ID: "sub_dispatch", Status: models.SubscriptionApproved, Type: models.MediaTV, TmdbID: "1399", Season: 0},
	}}
	restoreFetch := stub.install()
	defer restoreFetch()

	persistStub := &stubPersistMpError{}
	restorePersist := persistStub.install()
	defer restorePersist()

	requestedSeason := 2
	var captured moviepilotint.DownloadCandidateRequest
	service := &SubscriptionService{moviepilot: &stubSubscriptionMoviePilotClient{
		dispatchFn: func(req moviepilotint.DownloadCandidateRequest) (*moviepilotint.DownloadCandidateResponse, error) {
			captured = req
			return &moviepilotint.DownloadCandidateResponse{Accepted: true, Message: "ok"}, nil
		},
	}}

	result, err := service.ManualDispatchSubscription("sub_dispatch", ManualDispatchRequest{
		Season:           &requestedSeason,
		CandidatePayload: map[string]interface{}{"title": "Show S02"},
	})
	if err != nil {
		t.Fatalf("expected manual dispatch to succeed, got %v", err)
	}
	if result == nil || !result.Accepted {
		t.Fatalf("expected accepted result, got %#v", result)
	}
	if captured.TmdbID != "1399" || captured.Season != 2 {
		t.Fatalf("expected tmdbId=1399 season=2 passed to MoviePilot, got %#v", captured)
	}
	if len(persistStub.calls) != 1 {
		t.Fatalf("expected one mpError clear call, got %d", len(persistStub.calls))
	}
	if persistStub.calls[0].subscriptionID != "sub_dispatch" || persistStub.calls[0].mpError != nil {
		t.Fatalf("expected mpError clear for sub_dispatch, got %#v", persistStub.calls[0])
	}
}

func TestManualDispatchSubscriptionFailureDoesNotWriteMpError(t *testing.T) {
	stub := &stubFetchSubscriptionByID{subs: map[string]models.Subscription{
		"sub_movie": {ID: "sub_movie", Status: models.SubscriptionApproved, Type: models.MediaMovie, TmdbID: "27205"},
	}}
	restoreFetch := stub.install()
	defer restoreFetch()

	persistStub := &stubPersistMpError{}
	restorePersist := persistStub.install()
	defer restorePersist()

	service := &SubscriptionService{moviepilot: &stubSubscriptionMoviePilotClient{
		dispatchFn: func(req moviepilotint.DownloadCandidateRequest) (*moviepilotint.DownloadCandidateResponse, error) {
			return nil, fmt.Errorf("%w: 重复添加", moviepilotint.ErrMoviePilotBusinessRejected)
		},
	}}

	_, err := service.ManualDispatchSubscription("sub_movie", ManualDispatchRequest{
		CandidatePayload: map[string]interface{}{"title": "Inception"},
	})
	if !errors.Is(err, moviepilotint.ErrMoviePilotBusinessRejected) {
		t.Fatalf("expected ErrMoviePilotBusinessRejected, got %v", err)
	}
	if len(persistStub.calls) != 0 {
		t.Fatalf("manual dispatch failure must not write mpError, got %#v", persistStub.calls)
	}
}
