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
		CreatedAt:   now.Add(-2 * time.Hour),
	}
	if !shouldReusePendingPayment(reusable, now) {
		t.Fatalf("expected recent pending payment to be reusable")
	}

	expired := reusable
	expired.CreatedAt = now.Add(-25 * time.Hour)
	if shouldReusePendingPayment(expired, now) {
		t.Fatalf("expected stale pending payment to be skipped")
	}

	missingURL := reusable
	missingURL.CheckoutURL = ""
	if shouldReusePendingPayment(missingURL, now) {
		t.Fatalf("expected pending payment without checkout url to be skipped")
	}

	completed := reusable
	completed.Status = models.PaymentCompleted
	if shouldReusePendingPayment(completed, now) {
		t.Fatalf("expected completed payment to be skipped")
	}
}
