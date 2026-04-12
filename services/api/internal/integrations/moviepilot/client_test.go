package moviepilot

import (
	"io"
	"net/http"
	"strings"
	"testing"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestBuildSubscribeRequestBodyIncludesSeasonForTVSeasonSubscription(t *testing.T) {
	body, err := buildSubscribeRequestBody(SubscribeRequest{
		Type:   "tv",
		Name:   "Game of Thrones",
		TmdbID: "1399",
		Season: 3,
	})
	if err != nil {
		t.Fatalf("buildSubscribeRequestBody returned error: %v", err)
	}

	if got, ok := body["type"].(string); !ok || got != "电视剧" {
		t.Fatalf("expected type to be 电视剧, got %#v", body["type"])
	}
	if got, ok := body["name"].(string); !ok || got != "Game of Thrones" {
		t.Fatalf("expected name to be preserved, got %#v", body["name"])
	}
	if got, ok := body["tmdbid"].(int); !ok || got != 1399 {
		t.Fatalf("expected tmdbid to be 1399, got %#v", body["tmdbid"])
	}
	if got, ok := body["season"].(int); !ok || got != 3 {
		t.Fatalf("expected season to be 3, got %#v", body["season"])
	}
}

func TestBuildSubscribeRequestBodyOmitsSeasonForWholeSeries(t *testing.T) {
	body, err := buildSubscribeRequestBody(SubscribeRequest{
		Type:   "tv",
		Name:   "Game of Thrones",
		TmdbID: "1399",
		Season: 0,
	})
	if err != nil {
		t.Fatalf("buildSubscribeRequestBody returned error: %v", err)
	}

	if _, exists := body["season"]; exists {
		t.Fatalf("expected season to be omitted for whole-series subscription, got %#v", body["season"])
	}
}

func TestBuildSubscribeRequestBodyRejectsInvalidTMDBID(t *testing.T) {
	_, err := buildSubscribeRequestBody(SubscribeRequest{
		Type:   "movie",
		Name:   "Inception",
		TmdbID: "bad-id",
	})
	if err == nil {
		t.Fatal("expected invalid tmdb id to return error")
	}
}

func TestCreateSubscriptionUsesXAPIKeyHeader(t *testing.T) {
	t.Setenv("MOVIEPILOT_URL", "http://moviepilot.test")
	t.Setenv("MOVIEPILOT_API_KEY", "test-key")

	client := NewMoviePilotClient()
	client.client = &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.Method != http.MethodPost {
			t.Fatalf("expected POST method, got %s", req.Method)
		}
		if req.URL.String() != "http://moviepilot.test/api/v1/subscribe/" {
			t.Fatalf("unexpected request url: %s", req.URL.String())
		}
		if req.Header.Get("X-API-KEY") != "test-key" {
			t.Fatalf("expected X-API-KEY header to be set")
		}
		if got := req.Header.Get("Authorization"); got != "" {
			t.Fatalf("expected Authorization header to be empty, got %s", got)
		}

		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(`{"ok":true}`)),
			Header:     make(http.Header),
		}, nil
	})}

	err := client.CreateSubscription(SubscribeRequest{
		Type:   "movie",
		Name:   "Inception",
		TmdbID: "27205",
	})
	if err != nil {
		t.Fatalf("expected create subscription to succeed, got %v", err)
	}
}
