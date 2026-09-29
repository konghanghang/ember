package notifier

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"strings"
	"testing"
	"testing/iotest"
	"time"
)

// TestNotifyRankingReportsActualOutcome 锁定单次发送合同及脱敏日志，不以 HTTP 200 推断送达。
func TestNotifyRankingReportsActualOutcome(t *testing.T) {
	for _, tt := range []struct {
		name       string
		period     string
		body       string
		status     int
		err        error
		bodyError  error
		configured bool
		want       string
	}{
		{name: "daily_sent", period: "daily", body: `{"ok":true,"sent":true}`, configured: true, want: "outcome=sent"},
		{name: "weekly_sent", period: "weekly", body: `{"ok":true,"sent":true}`, configured: true, want: "outcome=sent"},
		{name: "no_target", body: `{"ok":true,"sent":false,"reason":"chat_not_configured"}`, configured: true, want: "outcome=skipped reason=chat_not_configured"},
		{name: "bot_not_configured", want: "outcome=skipped reason=bot_not_configured"},
		{name: "legacy_response", body: `{"ok":true}`, configured: true, want: "outcome=unconfirmed reason=invalid_response"},
		{name: "invalid_json", body: `private-response-body`, configured: true, want: "outcome=unconfirmed reason=invalid_response"},
		{name: "missing_ok", body: `{"sent":true}`, configured: true, want: "outcome=unconfirmed reason=invalid_response"},
		{name: "bot_rejected", body: `{"ok":false,"sent":true}`, configured: true, want: "outcome=unconfirmed reason=invalid_response"},
		{name: "unknown_skip_reason", body: `{"ok":true,"sent":false,"reason":"private-response-body"}`, configured: true, want: "outcome=unconfirmed reason=invalid_response"},
		{name: "unauthorized", status: http.StatusUnauthorized, body: `private-response-body`, configured: true, want: "outcome=unconfirmed reason=http_error status=401"},
		{name: "bot_failure", status: http.StatusBadGateway, body: `private-response-body`, configured: true, want: "outcome=unconfirmed reason=http_error status=502"},
		{name: "network_error", err: errors.New("private-network-error"), configured: true, want: "outcome=unconfirmed reason=request_error"},
		{name: "timeout", err: context.DeadlineExceeded, configured: true, want: "outcome=unconfirmed reason=timeout"},
		{name: "response_body_timeout", bodyError: context.DeadlineExceeded, configured: true, want: "outcome=unconfirmed reason=timeout"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var output bytes.Buffer
			previous := log.Writer()
			log.SetOutput(&output)
			t.Cleanup(func() { log.SetOutput(previous) })
			calls := 0
			period := tt.period
			if period == "" {
				period = "daily"
			}
			client := &BotNotifier{
				secret:          "private-secret",
				lastRefreshedAt: time.Now(),
				client: &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
					calls++
					if request.Method != http.MethodPost || request.URL.Path != "/notify/ranking" {
						t.Fatalf("unexpected request: %s %s", request.Method, request.URL.Path)
					}
					if request.Header.Get("X-Internal-Secret") != "private-secret" || request.Header.Get("X-Request-Id") == "" {
						t.Fatal("missing notification authentication or request identity")
					}
					var payload RankingNotification
					if err := json.NewDecoder(request.Body).Decode(&payload); err != nil || payload.BatchID != "batch_fixture" || payload.Period != period || payload.TotalDuration != 120 {
						t.Fatalf("unexpected ranking payload: %+v err=%v", payload, err)
					}
					if tt.err != nil {
						return nil, tt.err
					}
					status := tt.status
					if status == 0 {
						status = http.StatusOK
					}
					if tt.bodyError != nil {
						return &http.Response{StatusCode: status, Header: make(http.Header), Body: io.NopCloser(iotest.ErrReader(tt.bodyError))}, nil
					}
					return &http.Response{StatusCode: status, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(tt.body))}, nil
				})},
			}
			if tt.configured {
				client.botURL = "https://private-bot.example"
			}
			client.NotifyRanking(RankingNotification{BatchID: "batch_fixture", Period: period, TotalDuration: 120})
			wantCalls := 0
			if tt.configured {
				wantCalls = 1
			}
			if calls != wantCalls {
				t.Fatalf("calls=%d want=%d; notification must not retry", calls, wantCalls)
			}
			if !strings.Contains(output.String(), tt.want) {
				t.Fatalf("log=%q want %q", output.String(), tt.want)
			}
			if !strings.Contains(output.String(), `batchId="batch_fixture"`) {
				t.Fatal("notification log cannot be correlated with its saved batch")
			}
			for _, secret := range []string{"private-response-body", "private-network-error", "private-secret", "private-bot.example"} {
				if strings.Contains(output.String(), secret) {
					t.Fatalf("log leaked private fixture %q", secret)
				}
			}
		})
	}
}
