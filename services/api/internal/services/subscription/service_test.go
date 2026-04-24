package subscription

import (
	"fmt"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
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
	responses  map[string]stubSubscriptionEmbyLookupResponse
}

type stubSubscriptionEmbyLookupResponse struct {
	expectedParams map[string]string
	body           []byte
	err            error
}

func (s *stubSubscriptionEmbyLookup) IsConfigured() bool {
	return s.configured
}

func (s *stubSubscriptionEmbyLookup) GetWithAPIKey(path string, params map[string]string) ([]byte, error) {
	resp, ok := s.responses[path]
	if !ok {
		return nil, fmt.Errorf("unexpected path: %s", path)
	}
	for key, expectedValue := range resp.expectedParams {
		if params[key] != expectedValue {
			return nil, fmt.Errorf("unexpected param %s: %s", key, params[key])
		}
	}
	return resp.body, resp.err
}

func TestResolveSeriesTMDBIDBySeriesID(t *testing.T) {
	originalFactory := newSubscriptionEmbyLookup
	t.Cleanup(func() {
		newSubscriptionEmbyLookup = originalFactory
	})

	newSubscriptionEmbyLookup = func() subscriptionEmbyLookup {
		return &stubSubscriptionEmbyLookup{
			configured: true,
			responses: map[string]stubSubscriptionEmbyLookupResponse{
				"/emby/Items": {
					expectedParams: map[string]string{
						"Ids":    "series_81812",
						"Fields": "ProviderIds",
					},
					body: []byte(`{"Items":[{"ProviderIds":{"Tmdb":"123456"}}]}`),
				},
			},
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

func TestResolveSeriesTMDBIDBySeriesIDReturnsEmptyWhenSeriesNotFound(t *testing.T) {
	originalFactory := newSubscriptionEmbyLookup
	t.Cleanup(func() {
		newSubscriptionEmbyLookup = originalFactory
	})

	newSubscriptionEmbyLookup = func() subscriptionEmbyLookup {
		return &stubSubscriptionEmbyLookup{
			configured: true,
			responses: map[string]stubSubscriptionEmbyLookupResponse{
				"/emby/Items": {
					expectedParams: map[string]string{
						"Ids":    "series_81812",
						"Fields": "ProviderIds",
					},
					body: []byte(`{"Items":[]}`),
				},
				"/emby/Items/series_81812": {
					expectedParams: map[string]string{
						"Fields": "ProviderIds",
					},
					body: []byte(`{"ProviderIds":{"Tmdb":"654321"}}`),
				},
			},
		}
	}

	got, err := resolveSeriesTMDBIDBySeriesID("series_81812")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if got != "654321" {
		t.Fatalf("expected 654321, got %s", got)
	}
}

func TestIsSubscriptionUniqueConflictDetectsPostgresDuplicateKey(t *testing.T) {
	err := &pgconn.PgError{Code: "23505"}
	if !isSubscriptionUniqueConflict(err) {
		t.Fatal("expected postgres duplicate key to be treated as subscription duplicate")
	}
}
