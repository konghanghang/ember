package app

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/konghang/ember/backend/internal/models"
)

func TestIntegrationCreateSubscriptionAutoApprovesWithinQuota(t *testing.T) {
	harness := newIntegrationHarness(t)

	if err := harness.database.Model(&models.PlanGroup{}).
		Where("key = ?", "DEFAULT").
		Update("subscription_auto_approve_daily_limit", 1).Error; err != nil {
		t.Fatalf("update default plan group auto approve limit: %v", err)
	}

	user := harness.seedUser(t, models.User{
		Username: "itest_user_auto",
		Email:    "itest-user-auto@example.com",
		EmbyID:   "emby_user_auto",
	})

	recorder := harness.performUserRequest(t, user, http.MethodPost, "/api/v1/subscriptions", []byte(`{
		"type":"MOVIE",
		"name":"Inception",
		"tmdbId":"27205",
		"confirmExisting":true
	}`))
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", recorder.Code, recorder.Body.String())
	}

	var resp struct {
		Success        bool   `json:"success"`
		SubscriptionID string `json:"subscriptionId"`
		Status         string `json:"status"`
		AutoApproved   bool   `json:"autoApproved"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !resp.Success || !resp.AutoApproved || resp.Status != string(models.SubscriptionApproved) || resp.SubscriptionID == "" {
		t.Fatalf("unexpected response: %+v", resp)
	}

	var subscription models.Subscription
	if err := harness.database.Where("id = ?", resp.SubscriptionID).First(&subscription).Error; err != nil {
		t.Fatalf("load created subscription: %v", err)
	}
	if subscription.Status != models.SubscriptionApproved {
		t.Fatalf("expected approved status, got %s", subscription.Status)
	}
	if subscription.ReviewedAt == nil {
		t.Fatal("expected reviewedAt to be persisted")
	}
	if subscription.ReviewSource == nil || *subscription.ReviewSource != models.SubscriptionReviewSourceAutoQuota {
		t.Fatalf("expected AUTO_QUOTA review source, got %+v", subscription.ReviewSource)
	}
}

func TestIntegrationCreateSubscriptionFallsBackToPendingWhenQuotaExhausted(t *testing.T) {
	harness := newIntegrationHarness(t)

	if err := harness.database.Model(&models.PlanGroup{}).
		Where("key = ?", "DEFAULT").
		Update("subscription_auto_approve_daily_limit", 1).Error; err != nil {
		t.Fatalf("update default plan group auto approve limit: %v", err)
	}

	user := harness.seedUser(t, models.User{
		Username: "itest_user_pending",
		Email:    "itest-user-pending@example.com",
		EmbyID:   "emby_user_pending",
	})

	reviewSource := models.SubscriptionReviewSourceAutoQuota
	reviewedAt := time.Now().UTC()
	if err := harness.database.Create(&models.Subscription{
		UserID:       user.ID,
		Type:         models.MediaMovie,
		Name:         "Earlier Auto Approved",
		TmdbID:       "10001",
		Status:       models.SubscriptionApproved,
		ReviewSource: &reviewSource,
		ReviewedAt:   &reviewedAt,
	}).Error; err != nil {
		t.Fatalf("seed existing auto approved subscription: %v", err)
	}

	recorder := harness.performUserRequest(t, user, http.MethodPost, "/api/v1/subscriptions", []byte(`{
		"type":"MOVIE",
		"name":"Interstellar",
		"tmdbId":"157336",
		"confirmExisting":true
	}`))
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", recorder.Code, recorder.Body.String())
	}

	var resp struct {
		Success        bool   `json:"success"`
		SubscriptionID string `json:"subscriptionId"`
		Status         string `json:"status"`
		AutoApproved   bool   `json:"autoApproved"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !resp.Success || resp.AutoApproved || resp.Status != string(models.SubscriptionPending) || resp.SubscriptionID == "" {
		t.Fatalf("unexpected response: %+v", resp)
	}

	var subscription models.Subscription
	if err := harness.database.Where("id = ?", resp.SubscriptionID).First(&subscription).Error; err != nil {
		t.Fatalf("load created subscription: %v", err)
	}
	if subscription.Status != models.SubscriptionPending {
		t.Fatalf("expected pending status, got %s", subscription.Status)
	}
	if subscription.ReviewedAt != nil {
		t.Fatalf("expected reviewedAt to stay nil, got %v", subscription.ReviewedAt)
	}
	if subscription.ReviewSource != nil {
		t.Fatalf("expected reviewSource to stay nil, got %+v", subscription.ReviewSource)
	}
}

// TestIntegrationSubscriptionAccessProjection covers both entry points against persisted grants and manual bans.
func TestIntegrationSubscriptionAccessProjection(t *testing.T) {
	for _, tc := range []struct {
		name              string
		granted, disabled bool
		status            int
	}{
		{"no_entitlement", false, false, http.StatusForbidden},
		{"manual_ban", true, true, http.StatusForbidden},
		{"granted", true, false, http.StatusOK},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := newIntegrationHarness(t)
			user := h.seedUser(t, models.User{Username: "access_" + tc.name, Email: tc.name + "@example.com", EmbyID: "fake_emby"})
			if !tc.granted {
				if err := h.database.Where("user_id = ?", user.ID).Delete(&models.UserEntitlement{}).Error; err != nil {
					t.Fatal(err)
				}
			}
			if err := h.database.Model(&models.User{}).Where("id = ?", user.ID).Updates(map[string]any{"resource_access_granted": tc.granted, "emby_access_disabled": tc.disabled}).Error; err != nil {
				t.Fatal(err)
			}
			created := h.performUserRequest(t, user, http.MethodPost, "/api/v1/subscriptions", []byte(`{"type":"MOVIE","name":"Access fixture","tmdbId":"990001","confirmExisting":true}`))
			if created.Code != tc.status {
				t.Fatalf("create status=%d body=%s", created.Code, created.Body.String())
			}
			rejected := models.Subscription{UserID: user.ID, Type: models.MediaMovie, Name: "Resubmit fixture", TmdbID: "990002", Status: models.SubscriptionRejected}
			if err := h.database.Create(&rejected).Error; err != nil {
				t.Fatal(err)
			}
			resubmitted := h.performUserRequest(t, user, http.MethodPost, "/api/v1/subscriptions/"+rejected.ID+"/resubmit", []byte(`{"note":"integration retry","confirmExisting":true}`))
			if resubmitted.Code != tc.status {
				t.Fatalf("resubmit status=%d body=%s", resubmitted.Code, resubmitted.Body.String())
			}
		})
	}
}
