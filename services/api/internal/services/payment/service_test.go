package payment

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/konghang/ember/backend/internal/models"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

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

func TestSuccessfulPaymentFulfillmentSkipReason(t *testing.T) {
	updatedAt := time.Date(2026, 6, 17, 12, 0, 0, 0, time.UTC)
	olderEvent := updatedAt.Add(-time.Minute)
	newerEvent := updatedAt.Add(time.Minute)

	testCases := []struct {
		name         string
		payment      models.Payment
		eventCreated time.Time
		want         paymentWebhookSkipReason
	}{
		{
			name:    "completed payment is idempotent",
			payment: models.Payment{Status: models.PaymentCompleted, UpdatedAt: updatedAt},
			want:    paymentWebhookSkipCompleted,
		},
		{
			name:    "failed payment blocks success event",
			payment: models.Payment{Status: models.PaymentFailed, UpdatedAt: updatedAt},
			want:    paymentWebhookSkipFailed,
		},
		{
			name:    "expired payment blocks success event",
			payment: models.Payment{Status: models.PaymentExpired, UpdatedAt: updatedAt},
			want:    paymentWebhookSkipExpired,
		},
		{
			name:         "older success event is ignored",
			payment:      models.Payment{Status: models.PaymentPending, UpdatedAt: updatedAt},
			eventCreated: olderEvent,
			want:         paymentWebhookSkipOutOfOrder,
		},
		{
			name:         "newer pending success event can proceed",
			payment:      models.Payment{Status: models.PaymentPending, UpdatedAt: updatedAt},
			eventCreated: newerEvent,
			want:         paymentWebhookSkipNone,
		},
		{
			name:    "missing event time can proceed for pending payment",
			payment: models.Payment{Status: models.PaymentPending, UpdatedAt: updatedAt},
			want:    paymentWebhookSkipNone,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if got := successfulPaymentFulfillmentSkipReason(tc.payment, tc.eventCreated); got != tc.want {
				t.Fatalf("expected skip reason %q, got %q", tc.want, got)
			}
		})
	}
}

func TestFailedPaymentMarkSkipReason(t *testing.T) {
	updatedAt := time.Date(2026, 6, 17, 12, 0, 0, 0, time.UTC)
	olderEvent := updatedAt.Add(-time.Minute)
	newerEvent := updatedAt.Add(time.Minute)

	testCases := []struct {
		name         string
		payment      models.Payment
		eventCreated time.Time
		want         paymentWebhookSkipReason
	}{
		{
			name:    "completed payment blocks failure event",
			payment: models.Payment{Status: models.PaymentCompleted, UpdatedAt: updatedAt},
			want:    paymentWebhookSkipCompleted,
		},
		{
			name:    "expired payment blocks failure event",
			payment: models.Payment{Status: models.PaymentExpired, UpdatedAt: updatedAt},
			want:    paymentWebhookSkipExpired,
		},
		{
			name:    "failed payment is idempotent",
			payment: models.Payment{Status: models.PaymentFailed, UpdatedAt: updatedAt},
			want:    paymentWebhookSkipFailed,
		},
		{
			name:         "older failure event is ignored",
			payment:      models.Payment{Status: models.PaymentPending, UpdatedAt: updatedAt},
			eventCreated: olderEvent,
			want:         paymentWebhookSkipOutOfOrder,
		},
		{
			name:         "newer pending failure event can proceed",
			payment:      models.Payment{Status: models.PaymentPending, UpdatedAt: updatedAt},
			eventCreated: newerEvent,
			want:         paymentWebhookSkipNone,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if got := failedPaymentMarkSkipReason(tc.payment, tc.eventCreated); got != tc.want {
				t.Fatalf("expected skip reason %q, got %q", tc.want, got)
			}
		})
	}
}

func TestCalculateFulfilledPaymentExpiryStartsFromNowWithoutActiveExpiry(t *testing.T) {
	now := time.Date(2026, 6, 17, 10, 0, 0, 0, time.UTC)
	expiredAt := now.Add(-time.Second)

	tests := []struct {
		name          string
		currentExpiry *time.Time
	}{
		{name: "nil expiry", currentExpiry: nil},
		{name: "expired expiry", currentExpiry: &expiredAt},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := calculateFulfilledPaymentExpiry(now, tc.currentExpiry, 90)
			want := now.AddDate(0, 0, 90)

			if !got.Equal(want) {
				t.Fatalf("expected expiry from now %s, got %s", want, got)
			}
		})
	}
}

func TestCalculateFulfilledPaymentExpiryExtendsActiveExpiry(t *testing.T) {
	now := time.Date(2026, 6, 17, 10, 0, 0, 0, time.UTC)
	currentExpiry := now.AddDate(0, 0, 21)

	got := calculateFulfilledPaymentExpiry(now, &currentExpiry, 30)
	want := currentExpiry.AddDate(0, 0, 30)

	if !got.Equal(want) {
		t.Fatalf("expected extension from current expiry %s, got %s", want, got)
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

func TestCreateStripeCheckoutSessionBuildsRequest(t *testing.T) {
	var capturedForm url.Values
	var capturedAuth string
	var capturedContentType string
	var capturedIdempotencyKey string

	service := &PaymentService{
		httpClient: &http.Client{
			Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				if req.Method != http.MethodPost {
					t.Fatalf("expected POST request, got %s", req.Method)
				}
				if req.URL.String() != "https://api.stripe.com/v1/checkout/sessions" {
					t.Fatalf("unexpected Stripe URL: %s", req.URL.String())
				}

				body, err := io.ReadAll(req.Body)
				if err != nil {
					t.Fatalf("read request body: %v", err)
				}
				values, err := url.ParseQuery(string(body))
				if err != nil {
					t.Fatalf("parse request form: %v", err)
				}
				capturedForm = values
				capturedAuth = req.Header.Get("Authorization")
				capturedContentType = req.Header.Get("Content-Type")
				capturedIdempotencyKey = req.Header.Get("Idempotency-Key")

				return &http.Response{
					StatusCode: http.StatusOK,
					Body:       io.NopCloser(strings.NewReader(`{"id":"cs_test_1","url":"https://checkout.stripe.com/c/pay/test"}`)),
					Header:     make(http.Header),
				}, nil
			}),
		},
	}

	plan := &models.Plan{
		ID:          "plan_1",
		Name:        "季度方案",
		Description: "  90 天套餐  ",
		Days:        90,
		Price:       1999,
		Currency:    "hkd",
	}

	result, err := service.createStripeCheckoutSession(
		"sk_test",
		"https://ember.example.com/payment/success",
		"https://ember.example.com/payment/cancel",
		"user_1",
		plan,
		[]string{"card", "alipay"},
		"pay_1",
	)
	if err != nil {
		t.Fatalf("expected checkout session, got %v", err)
	}
	if result.ID != "cs_test_1" || result.URL != "https://checkout.stripe.com/c/pay/test" {
		t.Fatalf("unexpected response: %+v", result)
	}

	assertFormValue(t, capturedForm, "mode", "payment")
	assertFormValue(t, capturedForm, "success_url", "https://ember.example.com/payment/success")
	assertFormValue(t, capturedForm, "cancel_url", "https://ember.example.com/payment/cancel")
	assertFormValue(t, capturedForm, "line_items[0][price_data][currency]", "hkd")
	assertFormValue(t, capturedForm, "line_items[0][price_data][unit_amount]", "1999")
	assertFormValue(t, capturedForm, "line_items[0][price_data][product_data][name]", "季度方案")
	assertFormValue(t, capturedForm, "line_items[0][price_data][product_data][description]", "90 天套餐")
	assertFormValue(t, capturedForm, "line_items[0][quantity]", "1")
	assertFormValue(t, capturedForm, "metadata[user_id]", "user_1")
	assertFormValue(t, capturedForm, "metadata[plan_id]", "plan_1")
	assertFormValue(t, capturedForm, "metadata[days]", "90")
	assertFormValue(t, capturedForm, "metadata[payment_id]", "pay_1")
	if got := capturedForm["payment_method_types[]"]; len(got) != 2 || got[0] != "card" || got[1] != "alipay" {
		t.Fatalf("unexpected payment methods: %+v", got)
	}
	if capturedAuth != "Bearer sk_test" {
		t.Fatalf("unexpected Authorization header: %q", capturedAuth)
	}
	if capturedContentType != "application/x-www-form-urlencoded" {
		t.Fatalf("unexpected Content-Type header: %q", capturedContentType)
	}
	if capturedIdempotencyKey != "checkout:pay_1" {
		t.Fatalf("unexpected Idempotency-Key header: %q", capturedIdempotencyKey)
	}
}

func TestCreateStripeCheckoutSessionOmitsOptionalFields(t *testing.T) {
	service := &PaymentService{
		httpClient: &http.Client{
			Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				body, err := io.ReadAll(req.Body)
				if err != nil {
					t.Fatalf("read request body: %v", err)
				}
				values, err := url.ParseQuery(string(body))
				if err != nil {
					t.Fatalf("parse request form: %v", err)
				}
				if values.Has("line_items[0][price_data][product_data][description]") {
					t.Fatalf("expected blank description to be omitted, got %q", values.Get("line_items[0][price_data][product_data][description]"))
				}
				if _, ok := values["payment_method_types[]"]; ok {
					t.Fatalf("expected empty payment methods to be omitted, got %+v", values["payment_method_types[]"])
				}
				if got := req.Header.Get("Idempotency-Key"); got != "" {
					t.Fatalf("expected blank payment id to omit idempotency key, got %q", got)
				}
				return &http.Response{
					StatusCode: http.StatusOK,
					Body:       io.NopCloser(strings.NewReader(`{"id":"cs_test_2","url":"https://checkout.stripe.com/c/pay/test2"}`)),
					Header:     make(http.Header),
				}, nil
			}),
		},
	}

	_, err := service.createStripeCheckoutSession("sk_test", "success", "cancel", "user_1", &models.Plan{
		ID:       "plan_1",
		Name:     "月度方案",
		Days:     30,
		Price:    999,
		Currency: "usd",
	}, nil, "")
	if err != nil {
		t.Fatalf("expected checkout session without optional fields, got %v", err)
	}
}

func TestCreateStripeCheckoutSessionMapsUnsafeUpstreamFailures(t *testing.T) {
	testCases := []struct {
		name       string
		statusCode int
		body       string
	}{
		{name: "http error", statusCode: http.StatusBadGateway, body: `{"error":{"message":"secret leaked"}}`},
		{name: "invalid json", statusCode: http.StatusOK, body: `{`},
		{name: "missing id", statusCode: http.StatusOK, body: `{"url":"https://checkout.stripe.com/c/pay/test"}`},
		{name: "missing url", statusCode: http.StatusOK, body: `{"id":"cs_test_1"}`},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			service := &PaymentService{
				httpClient: &http.Client{
					Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
						return &http.Response{
							StatusCode: tc.statusCode,
							Body:       io.NopCloser(strings.NewReader(tc.body)),
							Header:     make(http.Header),
						}, nil
					}),
				},
			}

			_, err := service.createStripeCheckoutSession("sk_test", "success", "cancel", "user_1", &models.Plan{
				ID:       "plan_1",
				Name:     "月度方案",
				Days:     30,
				Price:    999,
				Currency: "usd",
			}, []string{"card"}, "pay_1")
			if err == nil {
				t.Fatalf("expected checkout session error")
			}
			if strings.Contains(err.Error(), "secret leaked") {
				t.Fatalf("expected upstream error to be sanitized, got %v", err)
			}
		})
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

func assertFormValue(t *testing.T, values url.Values, key, want string) {
	t.Helper()
	if got := values.Get(key); got != want {
		t.Fatalf("expected form %s=%q, got %q", key, want, got)
	}
}

func TestVerifyStripeSignature(t *testing.T) {
	payload := []byte(`{"id":"evt_1"}`)
	secret := "whsec_test"
	timestamp := time.Now().Unix()
	validSignature := buildStripeTestSignature(t, timestamp, payload, secret)

	header := fmt.Sprintf("t=%d,v1=bad_signature,v1=%s", timestamp, validSignature)
	if err := verifyStripeSignature(header, payload, secret); err != nil {
		t.Fatalf("expected valid Stripe signature, got %v", err)
	}

	if err := verifyStripeSignature(fmt.Sprintf("t=%d,v1=%s", timestamp, validSignature), []byte(`{"id":"evt_2"}`), secret); err == nil {
		t.Fatalf("expected changed payload to fail signature verification")
	}
	if err := verifyStripeSignature(fmt.Sprintf("t=%d,v1=%s", timestamp, validSignature), payload, "wrong_secret"); err == nil {
		t.Fatalf("expected wrong secret to fail signature verification")
	}
	if err := verifyStripeSignature("t=not-a-number,v1=sig", payload, secret); err == nil {
		t.Fatalf("expected invalid timestamp to fail")
	}

	expiredTimestamp := time.Now().Add(-10 * time.Minute).Unix()
	expiredSignature := buildStripeTestSignature(t, expiredTimestamp, payload, secret)
	if err := verifyStripeSignature(fmt.Sprintf("t=%d,v1=%s", expiredTimestamp, expiredSignature), payload, secret); err == nil {
		t.Fatalf("expected expired timestamp to fail")
	}
}

func buildStripeTestSignature(t *testing.T, timestamp int64, payload []byte, secret string) string {
	t.Helper()

	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(fmt.Sprintf("%d", timestamp)))
	_, _ = mac.Write([]byte("."))
	_, _ = mac.Write(payload)
	return hex.EncodeToString(mac.Sum(nil))
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
