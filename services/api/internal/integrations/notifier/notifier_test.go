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
