package payment

import (
	"testing"
	"time"

	"github.com/konghang/ember/backend/internal/models"
)

func TestNormalizePlanCurrency(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{name: "default usd", input: "", want: "usd"},
		{name: "upper hkd", input: "HKD", want: "hkd"},
		{name: "trim cny", input: " cny ", want: "cny"},
		{name: "invalid", input: "eur", wantErr: true},
	}

	for _, tc := range tests {
		got, err := normalizePlanCurrency(tc.input)
		if tc.wantErr {
			if err == nil {
				t.Fatalf("%s: expected error, got nil", tc.name)
			}
			continue
		}
		if err != nil {
			t.Fatalf("%s: unexpected error: %v", tc.name, err)
		}
		if got != tc.want {
			t.Fatalf("%s: want %s, got %s", tc.name, tc.want, got)
		}
	}
}

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
