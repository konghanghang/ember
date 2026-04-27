package tmdbcache

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestFetchJSONDeduplicatesInflightRequests(t *testing.T) {
	var requestCount atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount.Add(1)
		time.Sleep(50 * time.Millisecond)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":123}`))
	}))
	defer server.Close()

	store := NewStore()

	var wg sync.WaitGroup
	start := make(chan struct{})
	run := func() {
		defer wg.Done()
		<-start
		var out map[string]any
		if err := store.FetchJSON(context.Background(), nil, server.Client(), "detail:123", server.URL, time.Minute, true, &out); err != nil {
			t.Errorf("FetchJSON returned error: %v", err)
			return
		}
		if got := int(out["id"].(float64)); got != 123 {
			t.Errorf("expected id 123, got %d", got)
		}
	}

	wg.Add(2)
	go run()
	go run()
	close(start)
	wg.Wait()

	if got := requestCount.Load(); got != 1 {
		t.Fatalf("expected exactly 1 upstream request, got %d", got)
	}
}
