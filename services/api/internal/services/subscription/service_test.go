package subscription

import (
	"fmt"
	"testing"
	"time"
)

func TestFormatNotificationTimeUsesConfiguredTimezone(t *testing.T) {
	t.Setenv("CRON_TIMEZONE", "Asia/Shanghai")

	reviewedAt := time.Date(2026, 4, 18, 7, 0, 41, 0, time.UTC)
	formatted := formatNotificationTime(&reviewedAt)
	if formatted == nil {
		t.Fatal("expected formatted time, got nil")
	}

	const want = "2026-04-18T15:00:41+08:00"
	if *formatted != want {
		t.Fatalf("expected %s, got %s", want, *formatted)
	}
}

type stubSubscriptionEmbyLookup struct {
	configured bool
	body       []byte
	err        error
}

func (s *stubSubscriptionEmbyLookup) IsConfigured() bool {
	return s.configured
}

func (s *stubSubscriptionEmbyLookup) GetWithAPIKey(path string, params map[string]string) ([]byte, error) {
	if path != "/emby/Items/series_81812" {
		return nil, fmt.Errorf("unexpected path: %s", path)
	}
	if params["Fields"] != "ProviderIds" {
		return nil, fmt.Errorf("unexpected fields: %s", params["Fields"])
	}
	return s.body, s.err
}

func TestResolveSeriesTMDBIDBySeriesID(t *testing.T) {
	originalFactory := newSubscriptionEmbyLookup
	t.Cleanup(func() {
		newSubscriptionEmbyLookup = originalFactory
	})

	newSubscriptionEmbyLookup = func() subscriptionEmbyLookup {
		return &stubSubscriptionEmbyLookup{
			configured: true,
			body:       []byte(`{"ProviderIds":{"Tmdb":"123456"}}`),
		}
	}

	got, err := resolveSeriesTMDBIDBySeriesID("series_81812")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if got != "123456" {
		t.Fatalf("expected 123456, got %s", got)
	}
}
