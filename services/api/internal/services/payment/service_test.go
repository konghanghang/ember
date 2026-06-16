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

func TestNormalizePlanGroupKey(t *testing.T) {
	tests := []struct {
		name       string
		input      string
		allowEmpty bool
		want       string
		wantErr    bool
	}{
		{name: "uppercase key", input: "vip-a", want: "VIP-A"},
		{name: "blank allowed", input: " ", allowEmpty: true, want: ""},
		{name: "blank rejected", input: " ", wantErr: true},
		{name: "invalid chars", input: "vip a", wantErr: true},
	}

	for _, tc := range tests {
		got, err := NormalizePlanGroupKey(tc.input, tc.allowEmpty)
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

func TestPlanGroupsMatchForFulfillment(t *testing.T) {
	tests := []struct {
		name          string
		userPlanGroup *string
		planPlanGroup string
		want          bool
		wantErr       bool
	}{
		{name: "same group matches", userPlanGroup: strPtr("VIP-A"), planPlanGroup: "VIP-A", want: true},
		{name: "different groups rejected", userPlanGroup: strPtr("VIP-A"), planPlanGroup: "VIP-B", want: false},
		{name: "invalid group errors", userPlanGroup: strPtr("VIP A"), planPlanGroup: "VIP-A", wantErr: true},
	}

	for _, tc := range tests {
		got, err := planGroupsMatchForFulfillment(tc.userPlanGroup, tc.planPlanGroup)
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
			t.Fatalf("%s: want %t, got %t", tc.name, tc.want, got)
		}
	}
}

func strPtr(value string) *string {
	return &value
}

func TestShouldRedispatchWebhook(t *testing.T) {
	cases := []struct {
		status models.StripeWebhookEventStatus
		want   bool
	}{
		{status: models.StripeWebhookEventReceived, want: true},   // 进程崩溃后 Stripe 重试必须能驱动履约
		{status: models.StripeWebhookEventFailed, want: true},     // 上次业务返回 5xx，下次重试要重新分发
		{status: models.StripeWebhookEventProcessed, want: false}, // 真正幂等结束，无需重复履约
		{status: models.StripeWebhookEventSkipped, want: false},   // 非业务事件已忽略，重复事件无需再分发
	}

	for _, tc := range cases {
		got := shouldRedispatchWebhook(tc.status)
		if got != tc.want {
			t.Fatalf("status=%s want=%t got=%t", tc.status, tc.want, got)
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

func TestPaymentPureHelpers(t *testing.T) {
	if got := truncateString("abcdef", 3); got != "abc" {
		t.Fatalf("truncateString() = %q, want abc", got)
	}
	if got := truncateString("abc", 10); got != "abc" {
		t.Fatalf("truncateString() should keep short strings, got %q", got)
	}

	if absInt64(-42) != 42 || absInt64(42) != 42 || absInt64(0) != 0 {
		t.Fatalf("absInt64 returned unexpected values")
	}

	notifyAt := time.Date(2026, 6, 16, 12, 13, 14, 0, time.FixedZone("UTC+8", 8*60*60))
	formatted := formatNotifyTime(&notifyAt)
	if formatted == nil || *formatted != "2026-06-16T04:13:14Z" {
		t.Fatalf("formatNotifyTime() = %v", formatted)
	}
	if formatNotifyTime(nil) != nil {
		t.Fatalf("formatNotifyTime(nil) should return nil")
	}
}

func TestParseStripeSignature(t *testing.T) {
	timestamp, signatures, err := parseStripeSignature("t=1710000000, v1=sig_a, ignored, v1=sig_b")
	if err != nil {
		t.Fatalf("parseStripeSignature() error = %v", err)
	}
	if timestamp != "1710000000" {
		t.Fatalf("timestamp = %q", timestamp)
	}
	if len(signatures) != 2 || signatures[0] != "sig_a" || signatures[1] != "sig_b" {
		t.Fatalf("unexpected signatures: %+v", signatures)
	}

	for _, header := range []string{"", "v1=sig_only", "t=1710000000"} {
		t.Run(header, func(t *testing.T) {
			if _, _, err := parseStripeSignature(header); err == nil {
				t.Fatalf("expected parse error for %q", header)
			}
		})
	}
}

func TestNormalizePaymentStatusFilter(t *testing.T) {
	tests := []struct {
		raw  string
		want models.PaymentStatus
	}{
		{raw: " pending ", want: models.PaymentPending},
		{raw: "COMPLETED", want: models.PaymentCompleted},
		{raw: "expired", want: models.PaymentExpired},
		{raw: "failed", want: models.PaymentFailed},
		{raw: "unknown", want: ""},
		{raw: "", want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.raw, func(t *testing.T) {
			if got := normalizePaymentStatusFilter(tt.raw); got != tt.want {
				t.Fatalf("normalizePaymentStatusFilter(%q) = %q, want %q", tt.raw, got, tt.want)
			}
		})
	}
}
