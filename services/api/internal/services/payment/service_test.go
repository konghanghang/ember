package payment

import (
	"testing"
	"time"

	"github.com/konghang/ember/backend/internal/models"
)

func TestShouldReusePendingPayment(t *testing.T) {
	now := time.Date(2026, 3, 14, 12, 0, 0, 0, time.UTC)

	reusable := models.Payment{
		Status:      models.PaymentPending,
		CheckoutURL: "https://checkout.stripe.com/c/pay/test",
		ExpiresAt:   timePtr(now.Add(10 * time.Minute)),
	}
	if !shouldReusePendingPayment(reusable, now) {
		t.Fatalf("expected unexpired pending payment to be reusable")
	}

	expired := reusable
	expired.ExpiresAt = timePtr(now.Add(-time.Minute))
	if shouldReusePendingPayment(expired, now) {
		t.Fatalf("expected expired pending payment to be skipped")
	}

	missingURL := reusable
	missingURL.CheckoutURL = ""
	if shouldReusePendingPayment(missingURL, now) {
		t.Fatalf("expected pending payment without checkout url to be skipped")
	}

	missingExpiry := reusable
	missingExpiry.ExpiresAt = nil
	if shouldReusePendingPayment(missingExpiry, now) {
		t.Fatalf("expected pending payment without expiresAt to be skipped")
	}

	completed := reusable
	completed.Status = models.PaymentCompleted
	if shouldReusePendingPayment(completed, now) {
		t.Fatalf("expected completed payment to be skipped")
	}
}
