package emby

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestGetItemsByIDsBatchesAndDeduplicates(t *testing.T) {
	requestCount := 0
	receivedIDGroups := make([][]string, 0)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/emby/Items" {
			http.NotFound(w, r)
			return
		}
		requestCount++

		rawIDs := strings.TrimSpace(r.URL.Query().Get("Ids"))
		ids := strings.Split(rawIDs, ",")
		receivedIDGroups = append(receivedIDGroups, ids)

		items := make([]EmbyLibraryItem, 0, len(ids))
		for _, id := range ids {
			id = strings.TrimSpace(id)
			if id == "" {
				continue
			}
			items = append(items, EmbyLibraryItem{
				ID:         id,
				SeriesID:   "series_" + id,
				SeriesName: "show_" + id,
			})
		}

		_ = json.NewEncoder(w).Encode(embyLibraryItemsResponse{
			Items:            items,
			TotalRecordCount: len(items),
		})
	}))
	defer server.Close()

	t.Setenv("EMBY_URL", server.URL)
	t.Setenv("EMBY_API_KEY", "test-key")

	itemIDs := []string{"item_1", "item_1", "item_2"}
	for i := 3; i <= 102; i++ {
		itemIDs = append(itemIDs, "item_"+strconv.Itoa(i))
	}

	s := &EmbyService{
		baseURL: server.URL,
		apiKey:  "test-key",
		client:  server.Client(),
	}

	items, err := s.GetItemsByIDs(itemIDs)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if requestCount != 2 {
		t.Fatalf("expected 2 batched requests, got %d", requestCount)
	}
	if len(items) != 102 {
		t.Fatalf("expected 102 unique items, got %d", len(items))
	}
	if len(receivedIDGroups) != 2 {
		t.Fatalf("expected 2 recorded batches, got %d", len(receivedIDGroups))
	}
	if len(receivedIDGroups[0]) != 100 {
		t.Fatalf("expected first batch size 100, got %d", len(receivedIDGroups[0]))
	}
	if len(receivedIDGroups[1]) != 2 {
		t.Fatalf("expected second batch size 2, got %d", len(receivedIDGroups[1]))
	}
}

func TestGetItemsByIDsReturnsPartialResultsWhenSomeBatchesFail(t *testing.T) {
	originalTimeout := getItemsByIDsBatchTimeout
	originalBackoff := getItemsByIDsRetryBackoff
	getItemsByIDsBatchTimeout = 200 * time.Millisecond
	getItemsByIDsRetryBackoff = 0
	defer func() {
		getItemsByIDsBatchTimeout = originalTimeout
		getItemsByIDsRetryBackoff = originalBackoff
	}()

	requestCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/emby/Items" {
			http.NotFound(w, r)
			return
		}

		requestCount++
		rawIDs := strings.TrimSpace(r.URL.Query().Get("Ids"))
		if strings.HasPrefix(rawIDs, "item_101") {
			http.Error(w, "upstream busy", http.StatusBadGateway)
			return
		}

		ids := strings.Split(rawIDs, ",")
		items := make([]EmbyLibraryItem, 0, len(ids))
		for _, id := range ids {
			id = strings.TrimSpace(id)
			if id == "" {
				continue
			}
			items = append(items, EmbyLibraryItem{
				ID:         id,
				SeriesID:   "series_" + id,
				SeriesName: "show_" + id,
			})
		}

		_ = json.NewEncoder(w).Encode(embyLibraryItemsResponse{
			Items:            items,
			TotalRecordCount: len(items),
		})
	}))
	defer server.Close()

	t.Setenv("EMBY_URL", server.URL)
	t.Setenv("EMBY_API_KEY", "test-key")

	s := &EmbyService{
		baseURL: server.URL,
		apiKey:  "test-key",
		client:  server.Client(),
	}

	itemIDs := make([]string, 0, 102)
	for i := 1; i <= 102; i++ {
		itemIDs = append(itemIDs, "item_"+strconv.Itoa(i))
	}

	items, err := s.GetItemsByIDs(itemIDs)
	if err == nil {
		t.Fatal("expected partial failure error, got nil")
	}
	if !IsGetItemsByIDsPartialFailure(err) {
		t.Fatalf("expected partial failure marker, got %v", err)
	}
	if len(items) != 100 {
		t.Fatalf("expected 100 successful items, got %d", len(items))
	}
	if requestCount != 4 {
		t.Fatalf("expected 4 requests with retries, got %d", requestCount)
	}
}

func TestGetItemsByIDsFailsWhenAllBatchesFail(t *testing.T) {
	originalBackoff := getItemsByIDsRetryBackoff
	getItemsByIDsRetryBackoff = 0
	defer func() {
		getItemsByIDsRetryBackoff = originalBackoff
	}()

	requestCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/emby/Items" {
			http.NotFound(w, r)
			return
		}
		requestCount++
		http.Error(w, "upstream down", http.StatusGatewayTimeout)
	}))
	defer server.Close()

	t.Setenv("EMBY_URL", server.URL)
	t.Setenv("EMBY_API_KEY", "test-key")

	s := &EmbyService{
		baseURL: server.URL,
		apiKey:  "test-key",
		client:  server.Client(),
	}

	items, err := s.GetItemsByIDs([]string{"item_1", "item_2"})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if IsGetItemsByIDsPartialFailure(err) {
		t.Fatalf("expected full failure, got partial marker: %v", err)
	}
	var batchErr *getItemsByIDsBatchError
	if !errors.As(err, &batchErr) {
		t.Fatalf("expected batch error wrapper, got %T", err)
	}
	if batchErr.FailedBatches != 1 || batchErr.TotalBatches != 1 {
		t.Fatalf("unexpected batch failure summary: %+v", batchErr)
	}
	if len(items) != 0 {
		t.Fatalf("expected no items, got %d", len(items))
	}
	if requestCount != getItemsByIDsMaxAttempts {
		t.Fatalf("expected %d retries, got %d", getItemsByIDsMaxAttempts, requestCount)
	}
}
