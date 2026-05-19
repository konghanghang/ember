package notifier

import (
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return fn(req)
}

func TestNotifierEventName(t *testing.T) {
	tests := []struct {
		path string
		want string
	}{
		{path: "/notify/subscription", want: "notify.subscription"},
		{path: "notify/payment", want: "notify.payment"},
		{path: "/", want: "unknown"},
	}

	for _, tt := range tests {
		if got := notifierEventName(tt.path); got != tt.want {
			t.Fatalf("notifierEventName(%q) = %q, want %q", tt.path, got, tt.want)
		}
	}
}

func TestBotNotifierPostSetsRequestIDAndSecretHeader(t *testing.T) {
	var captured *http.Request
	notifier := &BotNotifier{
		botURL: "https://bot.example.com",
		secret: "test-secret",
		client: &http.Client{
			Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				captured = req
				return &http.Response{
					StatusCode: http.StatusOK,
					Body:       io.NopCloser(strings.NewReader("ok")),
					Header:     make(http.Header),
				}, nil
			}),
		},
		lastRefreshedAt: time.Now().UTC(),
	}

	notifier.post("/notify/payment", map[string]any{"paymentId": "pay_123"})

	if captured == nil {
		t.Fatal("expected request to be sent")
	}
	if got := captured.Header.Get("X-Internal-Secret"); got != "test-secret" {
		t.Fatalf("X-Internal-Secret = %q, want %q", got, "test-secret")
	}
	if got := captured.Header.Get("X-Request-Id"); !strings.HasPrefix(got, "bot-notify-") {
		t.Fatalf("X-Request-Id = %q, want prefix %q", got, "bot-notify-")
	}
}

func TestNotifyNewSubscriptionWithDeliveriesDecodesResponse(t *testing.T) {
	notifier := &BotNotifier{
		botURL: "https://bot.example.com",
		secret: "test-secret",
		client: &http.Client{
			Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				if req.URL.Path != "/notify/subscription" {
					t.Fatalf("unexpected path: %s", req.URL.Path)
				}
				return &http.Response{
					StatusCode: http.StatusOK,
					Body: io.NopCloser(strings.NewReader(`{
						"ok": true,
						"deliveries": [
							{"adminTelegramId":1001,"chatId":1001,"messageId":77,"hasPhoto":true,"deliveryStatus":"sent"}
						]
					}`)),
					Header: make(http.Header),
				}, nil
			}),
		},
		lastRefreshedAt: time.Now().UTC(),
	}

	deliveries, err := notifier.NotifyNewSubscriptionWithDeliveries(SubscriptionNotification{ID: "sub_123"})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(deliveries) != 1 {
		t.Fatalf("expected 1 delivery, got %d", len(deliveries))
	}
	if deliveries[0].AdminTelegramID != 1001 || deliveries[0].MessageID == nil || *deliveries[0].MessageID != 77 {
		t.Fatalf("unexpected delivery: %+v", deliveries[0])
	}
}
