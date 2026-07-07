package subscription

import (
	"testing"
	"time"

	"github.com/konghang/ember/backend/internal/models"
	"gorm.io/gorm"
)

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
