package subscription

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	configpkg "github.com/konghang/ember/backend/internal/config"
	notifierint "github.com/konghang/ember/backend/internal/integrations/notifier"
	"github.com/konghang/ember/backend/internal/models"
	"gorm.io/gorm"
)

type notifyRequest struct {
	path string
	body map[string]any
}

func TestInsertSubscriptionWithReviewPolicyAutoApprovesWithinQuota(t *testing.T) {
	origBegin := beginSubscriptionTx
	origCommit := commitSubscriptionTx
	origRollback := rollbackSubscriptionTx
	origResolvePlanGroup := resolveSubscriptionPlanGroup
	origLock := lockSubscriptionTx
	origFindActive := findActiveSubscriptionForInsert
	origCount := countAutoApprovedSubscriptionsInWindow
	origCreate := createSubscriptionRecord
	origNow := subscriptionNow
	t.Cleanup(func() {
		beginSubscriptionTx = origBegin
		commitSubscriptionTx = origCommit
		rollbackSubscriptionTx = origRollback
		resolveSubscriptionPlanGroup = origResolvePlanGroup
		lockSubscriptionTx = origLock
		findActiveSubscriptionForInsert = origFindActive
		countAutoApprovedSubscriptionsInWindow = origCount
		createSubscriptionRecord = origCreate
		subscriptionNow = origNow
	})

	fixedNow := time.Date(2026, 7, 7, 2, 30, 0, 0, time.UTC)
	subscriptionNow = func() time.Time { return fixedNow }
	beginSubscriptionTx = func() (*gorm.DB, error) { return nil, nil }
	commitSubscriptionTx = func(tx *gorm.DB) error { return nil }
	rollbackSubscriptionTx = func(tx *gorm.DB) {}

	var lockedKeys []string
	lockSubscriptionTx = func(tx *gorm.DB, lockKey string) error {
		lockedKeys = append(lockedKeys, lockKey)
		return nil
	}
	resolveSubscriptionPlanGroup = func(tx *gorm.DB, explicitPlanGroup *string) (*models.PlanGroup, error) {
		return &models.PlanGroup{
			Key:                               "VIP_A",
			Name:                              "VIP A",
			SubscriptionAutoApproveDailyLimit: 2,
		}, nil
	}
	findActiveSubscriptionForInsert = func(tx *gorm.DB, mediaType models.MediaType, tmdbID string, season int) (*models.Subscription, error) {
		return nil, nil
	}
	countAutoApprovedSubscriptionsInWindow = func(tx *gorm.DB, userID string, start, end time.Time) (int64, error) {
		if userID != "user_1" {
			t.Fatalf("unexpected user id: %s", userID)
		}
		if !start.Before(end) {
			t.Fatalf("expected valid window, got start=%s end=%s", start, end)
		}
		return 0, nil
	}
	createSubscriptionRecord = func(tx *gorm.DB, subscription *models.Subscription) error {
		subscription.ID = "sub_auto_1"
		return nil
	}

	service := &SubscriptionService{}
	subscription := &models.Subscription{
		UserID: "user_1",
		Type:   models.MediaMovie,
		Name:   "Inception",
		TmdbID: "27205",
		Season: 0,
		Status: models.SubscriptionPending,
	}

	result, err := service.insertSubscriptionWithReviewPolicy(&models.User{
		ID:        "user_1",
		Username:  "ember",
		EmbyID:    "emby_1",
		PlanGroup: nil,
	}, models.MediaMovie, "27205", 0, subscription)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if result == nil || !result.Created || !result.AutoApproved {
		t.Fatalf("expected created auto-approved result, got %+v", result)
	}
	if result.AutoApprovedOrdinal != 1 || result.DailyLimit != 2 {
		t.Fatalf("unexpected quota result: %+v", result)
	}
	if subscription.Status != models.SubscriptionApproved {
		t.Fatalf("expected approved status, got %s", subscription.Status)
	}
	if subscription.ReviewedAt == nil || !subscription.ReviewedAt.Equal(fixedNow) {
		t.Fatalf("expected reviewedAt=%s, got %v", fixedNow, subscription.ReviewedAt)
	}
	if subscription.ReviewSource == nil || *subscription.ReviewSource != models.SubscriptionReviewSourceAutoQuota {
		t.Fatalf("expected AUTO_QUOTA review source, got %+v", subscription.ReviewSource)
	}
	if len(lockedKeys) != 2 {
		t.Fatalf("expected 2 locks, got %v", lockedKeys)
	}
	if lockedKeys[0] != "subscription:auto-approve:user_1:2026-07-07" {
		t.Fatalf("unexpected auto-approve lock key: %s", lockedKeys[0])
	}
	if lockedKeys[1] != "subscription:MOVIE:27205:0" {
		t.Fatalf("unexpected resource lock key: %s", lockedKeys[1])
	}
}

func TestInsertSubscriptionWithReviewPolicyFallsBackToPendingWhenQuotaExhausted(t *testing.T) {
	origBegin := beginSubscriptionTx
	origCommit := commitSubscriptionTx
	origRollback := rollbackSubscriptionTx
	origResolvePlanGroup := resolveSubscriptionPlanGroup
	origLock := lockSubscriptionTx
	origFindActive := findActiveSubscriptionForInsert
	origCount := countAutoApprovedSubscriptionsInWindow
	origCreate := createSubscriptionRecord
	origNow := subscriptionNow
	t.Cleanup(func() {
		beginSubscriptionTx = origBegin
		commitSubscriptionTx = origCommit
		rollbackSubscriptionTx = origRollback
		resolveSubscriptionPlanGroup = origResolvePlanGroup
		lockSubscriptionTx = origLock
		findActiveSubscriptionForInsert = origFindActive
		countAutoApprovedSubscriptionsInWindow = origCount
		createSubscriptionRecord = origCreate
		subscriptionNow = origNow
	})

	subscriptionNow = func() time.Time { return time.Date(2026, 7, 7, 2, 30, 0, 0, time.UTC) }
	beginSubscriptionTx = func() (*gorm.DB, error) { return nil, nil }
	commitSubscriptionTx = func(tx *gorm.DB) error { return nil }
	rollbackSubscriptionTx = func(tx *gorm.DB) {}
	lockSubscriptionTx = func(tx *gorm.DB, lockKey string) error { return nil }
	resolveSubscriptionPlanGroup = func(tx *gorm.DB, explicitPlanGroup *string) (*models.PlanGroup, error) {
		return &models.PlanGroup{
			Key:                               "VIP_A",
			Name:                              "VIP A",
			SubscriptionAutoApproveDailyLimit: 2,
		}, nil
	}
	findActiveSubscriptionForInsert = func(tx *gorm.DB, mediaType models.MediaType, tmdbID string, season int) (*models.Subscription, error) {
		return nil, nil
	}
	countAutoApprovedSubscriptionsInWindow = func(tx *gorm.DB, userID string, start, end time.Time) (int64, error) {
		return 2, nil
	}
	createSubscriptionRecord = func(tx *gorm.DB, subscription *models.Subscription) error {
		subscription.ID = "sub_pending_1"
		return nil
	}

	service := &SubscriptionService{}
	subscription := &models.Subscription{
		UserID: "user_1",
		Type:   models.MediaTV,
		Name:   "Westworld",
		TmdbID: "63247",
		Season: 1,
		Status: models.SubscriptionPending,
	}

	result, err := service.insertSubscriptionWithReviewPolicy(&models.User{
		ID:        "user_1",
		Username:  "ember",
		EmbyID:    "emby_1",
		PlanGroup: nil,
	}, models.MediaTV, "63247", 1, subscription)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if result == nil || !result.Created || result.AutoApproved {
		t.Fatalf("expected created pending result, got %+v", result)
	}
	if subscription.Status != models.SubscriptionPending {
		t.Fatalf("expected pending status, got %s", subscription.Status)
	}
	if subscription.ReviewedAt != nil {
		t.Fatalf("expected reviewedAt to stay nil, got %v", subscription.ReviewedAt)
	}
	if subscription.ReviewSource != nil {
		t.Fatalf("expected review source to stay nil, got %+v", subscription.ReviewSource)
	}
}

func TestCreateSubscriptionWithResultRejectsAccessDisabledEmbyUser(t *testing.T) {
	origLoadSubmitter := loadSubscriptionSubmitter
	t.Cleanup(func() {
		loadSubscriptionSubmitter = origLoadSubmitter
	})

	loadSubscriptionSubmitter = func(userID string) (*models.User, error) {
		return &models.User{
			ID:                 userID,
			Username:           "ember",
			EmbyID:             "emby_1",
			EmbyAccessDisabled: true,
		}, nil
	}

	service := &SubscriptionService{}
	_, err := service.CreateSubscriptionWithResult("user_1", CreateSubscriptionRequest{
		Type:            models.MediaMovie,
		Name:            "Inception",
		TmdbID:          "27205",
		ConfirmExisting: true,
	})
	if err != ErrSubscriptionEmbyDisabled {
		t.Fatalf("expected ErrSubscriptionEmbyDisabled, got %v", err)
	}
}

func TestCreateSubscriptionWithResultRejectsExpiredEmbyUser(t *testing.T) {
	origLoadSubmitter := loadSubscriptionSubmitter
	t.Cleanup(func() {
		loadSubscriptionSubmitter = origLoadSubmitter
	})

	expiredAt := time.Date(2026, 7, 6, 12, 0, 0, 0, time.UTC)
	loadSubscriptionSubmitter = func(userID string) (*models.User, error) {
		return &models.User{
			ID:        userID,
			Username:  "ember",
			EmbyID:    "emby_1",
			ExpiresAt: &expiredAt,
		}, nil
	}

	service := &SubscriptionService{}
	_, err := service.CreateSubscriptionWithResult("user_1", CreateSubscriptionRequest{
		Type:            models.MediaMovie,
		Name:            "Inception",
		TmdbID:          "27205",
		ConfirmExisting: true,
	})
	if err != ErrSubscriptionEmbyDisabled {
		t.Fatalf("expected ErrSubscriptionEmbyDisabled, got %v", err)
	}
}

func TestEnqueueAutoApprovedSideEffectsNotifiesAdminsForCreateAndResubmit(t *testing.T) {
	configpkg.InvalidateCachedSetting("BOT_NOTIFY_URL")
	t.Cleanup(func() {
		configpkg.InvalidateCachedSetting("BOT_NOTIFY_URL")
	})
	t.Setenv("INTERNAL_API_SECRET", "test-internal-secret")

	requests := make(chan notifyRequest, 4)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()

		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode notifier payload: %v", err)
		}
		requests <- notifyRequest{path: r.URL.Path, body: payload}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()
	t.Setenv("BOT_NOTIFY_URL", server.URL)

	originalLoadSubscriptionUser := loadSubscriptionUser
	t.Cleanup(func() {
		loadSubscriptionUser = originalLoadSubscriptionUser
	})
	loadSubscriptionUser = func(userID string) (*models.User, bool) {
		telegramID := int64(9001)
		return &models.User{
			ID:         userID,
			Username:   "ember-user",
			TelegramID: &telegramID,
		}, true
	}

	persistStub := &stubPersistMpError{}
	restorePersist := persistStub.install()
	defer restorePersist()

	service := &SubscriptionService{
		moviepilot: &stubSubscriptionMoviePilotClient{},
		notifier:   notifierint.NewBotNotifier(),
	}
	reviewedAt := time.Date(2026, 7, 9, 7, 0, 0, 0, time.UTC)
	insertResult := &subscriptionInsertResult{
		AutoApproved:        true,
		AutoApprovedOrdinal: 1,
		DailyLimit:          2,
		PlanGroupKey:        "VIP_A",
		PlanGroupName:       "VIP A",
	}
	subscription := models.Subscription{
		ID:         "sub_auto_1",
		UserID:     "user_1",
		Type:       models.MediaMovie,
		Name:       "Inception",
		TmdbID:     "27205",
		Status:     models.SubscriptionApproved,
		ReviewedAt: &reviewedAt,
	}

	service.enqueueAutoApprovedSideEffects(subscription, "ember-user", insertResult, false)
	createRequests := waitForNotifierRequests(t, requests, 2)
	assertAutoApprovedNotificationRequests(t, createRequests, "sub_auto_1", "VIP_A", false)

	service.enqueueAutoApprovedSideEffects(subscription, "ember-user", insertResult, true)
	resubmitRequests := waitForNotifierRequests(t, requests, 2)
	assertAutoApprovedNotificationRequests(t, resubmitRequests, "sub_auto_1", "VIP_A", true)
}

func waitForNotifierRequests(t *testing.T, requests <-chan notifyRequest, want int) []notifyRequest {
	t.Helper()

	deadline := time.After(3 * time.Second)
	got := make([]notifyRequest, 0, want)
	for len(got) < want {
		select {
		case req := <-requests:
			got = append(got, req)
		case <-deadline:
			t.Fatalf("timeout waiting for %d notifier requests, got %d", want, len(got))
		}
	}
	return got
}

func assertAutoApprovedNotificationRequests(t *testing.T, requests []notifyRequest, subscriptionID, planGroupKey string, isResubmit bool) {
	t.Helper()

	paths := make([]string, 0, len(requests))
	for _, req := range requests {
		paths = append(paths, req.path)
	}
	if len(paths) != 2 {
		t.Fatalf("expected 2 notifier requests, got %v", paths)
	}

	var approvedPayload map[string]any
	var autoApprovedPayload map[string]any
	for _, req := range requests {
		switch req.path {
		case "/notify/subscription-result":
			approvedPayload = req.body
		case "/notify/subscription-auto-approved":
			autoApprovedPayload = req.body
		default:
			t.Fatalf("unexpected notifier path: %s", req.path)
		}
	}

	if approvedPayload["subscriptionId"] != subscriptionID {
		t.Fatalf("expected subscription-result payload to carry %s, got %+v", subscriptionID, approvedPayload)
	}
	if autoApprovedPayload["id"] != subscriptionID {
		t.Fatalf("expected auto-approved payload to carry %s, got %+v", subscriptionID, autoApprovedPayload)
	}
	if autoApprovedPayload["planGroupKey"] != planGroupKey {
		t.Fatalf("expected planGroupKey=%s, got %+v", planGroupKey, autoApprovedPayload)
	}
	if autoApprovedPayload["autoApprovedOrdinal"] != float64(1) || autoApprovedPayload["dailyLimit"] != float64(2) {
		t.Fatalf("unexpected quota payload: %+v", autoApprovedPayload)
	}
	if !strings.Contains(approvedPayload["reviewedAt"].(string), "2026-07-09") {
		t.Fatalf("expected reviewedAt in subscription-result payload, got %+v", approvedPayload)
	}
	if !strings.Contains(autoApprovedPayload["reviewedAt"].(string), "2026-07-09") {
		t.Fatalf("expected reviewedAt in auto-approved payload, got %+v", autoApprovedPayload)
	}
	if isResubmit && autoApprovedPayload["userName"] != "ember-user" {
		t.Fatalf("expected resubmit payload to preserve userName, got %+v", autoApprovedPayload)
	}
}
