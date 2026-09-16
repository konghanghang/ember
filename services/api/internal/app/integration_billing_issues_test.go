package app

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/konghang/ember/backend/internal/models"
	paymentpkg "github.com/konghang/ember/backend/internal/services/payment"
	redemptionpkg "github.com/konghang/ember/backend/internal/services/redemption"
	userpkg "github.com/konghang/ember/backend/internal/services/user"
)

func TestIntegrationBillingRenewalsAccumulateWithRowLocks(t *testing.T) {
	harness := newIntegrationHarness(t)

	baseExpiry := time.Now().UTC().AddDate(0, 0, 30).Truncate(time.Second)
	target := harness.seedUser(t, models.User{
		Username:  "billing_renewal_user",
		Email:     "billing-renewal@example.com",
		ExpiresAt: &baseExpiry,
	})
	code := models.RedemptionCode{
		Code:                  "BILLINGLOCK1",
		MaxUses:               1,
		DefaultDays:           10,
		RegistrationPlanGroup: "VIP",
	}
	if err := harness.database.Create(&code).Error; err != nil {
		t.Fatalf("create redemption code: %v", err)
	}

	start := make(chan struct{})
	errs := make(chan error, 2)
	var wait sync.WaitGroup
	wait.Add(2)
	go func() {
		defer wait.Done()
		<-start
		_, err := redemptionpkg.NewRedemptionService().RedeemCode(target.ID, &redemptionpkg.RedeemCodeRequest{Code: code.Code})
		errs <- err
	}()
	go func() {
		defer wait.Done()
		<-start
		_, err := userpkg.NewUserService().ExtendExpiry(target.ID, 5)
		errs <- err
	}()
	close(start)
	wait.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent renewal failed: %v", err)
		}
	}

	var refreshed models.User
	if err := harness.database.Where("id = ?", target.ID).First(&refreshed).Error; err != nil {
		t.Fatalf("reload user: %v", err)
	}
	want := baseExpiry.AddDate(0, 0, 15)
	if refreshed.ExpiresAt == nil || !refreshed.ExpiresAt.Equal(want) {
		t.Fatalf("expiresAt = %v, want %s", refreshed.ExpiresAt, want)
	}
}

func TestIntegrationBillingLatePaidWebhookFulfillsExpiredPayment(t *testing.T) {
	harness := newIntegrationHarness(t)
	secret := "whsec_billing_late_paid"
	t.Setenv("STRIPE_WEBHOOK_SECRET", secret)

	harness.seedPlanGroup(t, models.PlanGroup{Key: "BILLING_A", Name: "Billing A"})
	plan := seedBillingIntegrationPlan(t, harness, "plan_billing_a", "BILLING_A", 30)
	initialExpiry := time.Now().UTC().AddDate(0, 0, 7).Truncate(time.Second)
	planGroup := "BILLING_A"
	target := harness.seedUser(t, models.User{
		Username:  "late_paid_user",
		Email:     "late-paid@example.com",
		PlanGroup: &planGroup,
		ExpiresAt: &initialExpiry,
	})
	payment := seedBillingIntegrationPayment(t, harness, target.ID, plan, models.PaymentExpired, "cs_late_paid", 30)

	payload := billingStripeWebhookPayload("evt_late_paid", "checkout.session.completed", "cs_late_paid", "pi_late_paid", true, payment.UpdatedAt.Add(-time.Hour))
	recorder := performBillingStripeWebhook(harness, payload, secret)
	if recorder.Code != http.StatusOK {
		t.Fatalf("late paid webhook status = %d body=%s", recorder.Code, recorder.Body.String())
	}

	var refreshedUser models.User
	if err := harness.database.Where("id = ?", target.ID).First(&refreshedUser).Error; err != nil {
		t.Fatalf("reload user: %v", err)
	}
	wantExpiry := initialExpiry.AddDate(0, 0, 30)
	if refreshedUser.ExpiresAt == nil || !refreshedUser.ExpiresAt.Equal(wantExpiry) {
		t.Fatalf("expiresAt = %v, want %s", refreshedUser.ExpiresAt, wantExpiry)
	}

	var refreshedPayment models.Payment
	if err := harness.database.Where("id = ?", payment.ID).First(&refreshedPayment).Error; err != nil {
		t.Fatalf("reload payment: %v", err)
	}
	if refreshedPayment.Status != models.PaymentCompleted {
		t.Fatalf("payment status = %s, want completed", refreshedPayment.Status)
	}
}

func TestIntegrationBillingGroupMismatchKeepsWebhookRetryable(t *testing.T) {
	harness := newIntegrationHarness(t)
	secret := "whsec_billing_group_mismatch"
	t.Setenv("STRIPE_WEBHOOK_SECRET", secret)

	harness.seedPlanGroup(t, models.PlanGroup{Key: "BILLING_A", Name: "Billing A"})
	harness.seedPlanGroup(t, models.PlanGroup{Key: "BILLING_B", Name: "Billing B"})
	plan := seedBillingIntegrationPlan(t, harness, "plan_billing_mismatch", "BILLING_A", 30)
	initialExpiry := time.Now().UTC().AddDate(0, 0, 7).Truncate(time.Second)
	planGroup := "BILLING_B"
	target := harness.seedUser(t, models.User{
		Username:  "mismatch_paid_user",
		Email:     "mismatch-paid@example.com",
		PlanGroup: &planGroup,
		ExpiresAt: &initialExpiry,
	})
	seedBillingIntegrationPayment(t, harness, target.ID, plan, models.PaymentExpired, "cs_group_mismatch", 30)

	payload := billingStripeWebhookPayload("evt_group_mismatch", "checkout.session.completed", "cs_group_mismatch", "pi_group_mismatch", true, time.Now().UTC())
	recorder := performBillingStripeWebhook(harness, payload, secret)
	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("group mismatch webhook status = %d body=%s", recorder.Code, recorder.Body.String())
	}

	var webhook models.StripeWebhookEvent
	if err := harness.database.Where("event_id = ?", "evt_group_mismatch").First(&webhook).Error; err != nil {
		t.Fatalf("reload webhook event: %v", err)
	}
	if webhook.Status != models.StripeWebhookEventFailed {
		t.Fatalf("webhook status = %s, want failed", webhook.Status)
	}
	if webhook.Error == nil || !strings.Contains(*webhook.Error, paymentpkg.ErrPaymentFailed.Error()) {
		t.Fatalf("webhook errorMessage = %v, want payment failure", webhook.Error)
	}

	var refreshedUser models.User
	if err := harness.database.Where("id = ?", target.ID).First(&refreshedUser).Error; err != nil {
		t.Fatalf("reload user: %v", err)
	}
	if refreshedUser.ExpiresAt == nil || !refreshedUser.ExpiresAt.Equal(initialExpiry) {
		t.Fatalf("expiresAt = %v, want unchanged %s", refreshedUser.ExpiresAt, initialExpiry)
	}
}

func TestIntegrationBillingAdminGroupChangeAndPaidWebhookDoNotDeadlock(t *testing.T) {
	harness := newIntegrationHarness(t)
	secret := "whsec_billing_group_change_race"
	t.Setenv("STRIPE_WEBHOOK_SECRET", secret)
	t.Setenv("STRIPE_SECRET_KEY", "")

	harness.seedPlanGroup(t, models.PlanGroup{Key: "BILLING_A", Name: "Billing A"})
	harness.seedPlanGroup(t, models.PlanGroup{Key: "BILLING_B", Name: "Billing B"})
	plan := seedBillingIntegrationPlan(t, harness, "plan_billing_group_race", "BILLING_A", 30)
	initialExpiry := time.Now().UTC().AddDate(0, 0, 7).Truncate(time.Second)
	planGroup := "BILLING_A"
	target := harness.seedUser(t, models.User{
		Username:  "group_race_user",
		Email:     "group-race@example.com",
		PlanGroup: &planGroup,
		ExpiresAt: &initialExpiry,
	})
	seedBillingIntegrationPayment(t, harness, target.ID, plan, models.PaymentPending, "cs_group_race", 30)

	start := make(chan struct{})
	errs := make(chan string, 2)
	var wait sync.WaitGroup
	wait.Add(2)
	go func() {
		defer wait.Done()
		<-start
		recorder := harness.performAdminRequest(http.MethodPut, "/api/v1/admin/users/"+target.ID, []byte(`{"planGroup":"BILLING_B"}`))
		if recorder.Code != http.StatusOK {
			errs <- fmt.Sprintf("admin group update status=%d body=%s", recorder.Code, recorder.Body.String())
		}
	}()
	go func() {
		defer wait.Done()
		<-start
		payload := billingStripeWebhookPayload("evt_group_race", "checkout.session.completed", "cs_group_race", "pi_group_race", true, time.Now().UTC())
		recorder := performBillingStripeWebhook(harness, payload, secret)
		if recorder.Code != http.StatusOK && recorder.Code != http.StatusInternalServerError {
			errs <- fmt.Sprintf("webhook status=%d body=%s", recorder.Code, recorder.Body.String())
		}
	}()
	close(start)
	wait.Wait()
	close(errs)
	for err := range errs {
		t.Fatal(err)
	}

	var payment models.Payment
	if err := harness.database.Where("stripe_session_id = ?", "cs_group_race").First(&payment).Error; err != nil {
		t.Fatalf("reload payment: %v", err)
	}
	var webhook models.StripeWebhookEvent
	if err := harness.database.Where("event_id = ?", "evt_group_race").First(&webhook).Error; err != nil {
		t.Fatalf("reload webhook: %v", err)
	}
	switch webhook.Status {
	case models.StripeWebhookEventProcessed:
		if payment.Status != models.PaymentCompleted {
			t.Fatalf("processed webhook left payment status=%s, want completed", payment.Status)
		}
	case models.StripeWebhookEventFailed:
		if payment.Status == models.PaymentCompleted {
			t.Fatalf("failed webhook must not complete payment")
		}
		if webhook.Error == nil || !strings.Contains(*webhook.Error, "reasonCode=plan_group_mismatch") {
			t.Fatalf("failed webhook error = %v, want group mismatch reason", webhook.Error)
		}
	default:
		t.Fatalf("webhook status=%s, want processed or failed", webhook.Status)
	}
}

func seedBillingIntegrationPlan(t *testing.T, harness *integrationHarness, id, planGroup string, days int) models.Plan {
	t.Helper()
	plan := models.Plan{
		ID:        id,
		Name:      id,
		Days:      days,
		Price:     1200,
		Currency:  "usd",
		PlanGroup: planGroup,
		IsActive:  true,
	}
	if err := harness.database.Create(&plan).Error; err != nil {
		t.Fatalf("create plan %s: %v", id, err)
	}
	return plan
}

func seedBillingIntegrationPayment(t *testing.T, harness *integrationHarness, userID string, plan models.Plan, status models.PaymentStatus, sessionID string, days int) models.Payment {
	t.Helper()
	expiresAt := time.Now().UTC().Add(-time.Hour)
	payment := models.Payment{
		UserID:          userID,
		PlanID:          plan.ID,
		StripeSessionID: sessionID,
		Amount:          plan.Price,
		Currency:        plan.Currency,
		Days:            days,
		Status:          status,
		ExpiresAt:       &expiresAt,
	}
	if err := harness.database.Create(&payment).Error; err != nil {
		t.Fatalf("create payment %s: %v", sessionID, err)
	}
	return payment
}

func billingStripeWebhookPayload(eventID, eventType, sessionID, paymentIntentID string, paid bool, created time.Time) []byte {
	paymentStatus := "unpaid"
	if paid {
		paymentStatus = "paid"
	}
	return []byte(fmt.Sprintf(`{"id":%q,"type":%q,"created":%d,"livemode":false,"data":{"object":{"id":%q,"payment_status":%q,"payment_intent":%q,"metadata":{}}}}`,
		eventID, eventType, created.Unix(), sessionID, paymentStatus, paymentIntentID))
}

func performBillingStripeWebhook(harness *integrationHarness, payload []byte, secret string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/api/v1/webhooks/stripe", strings.NewReader(string(payload)))
	req.Header.Set("Stripe-Signature", billingStripeSignature(payload, secret))
	recorder := httptest.NewRecorder()
	harness.router.ServeHTTP(recorder, req)
	return recorder
}

func billingStripeSignature(payload []byte, secret string) string {
	timestamp := time.Now().Unix()
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(fmt.Sprintf("%d.%s", timestamp, payload)))
	return fmt.Sprintf("t=%d,v1=%s", timestamp, hex.EncodeToString(mac.Sum(nil)))
}
