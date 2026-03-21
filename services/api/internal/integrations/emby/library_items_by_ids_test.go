package emby

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
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
		client: server.Client(),
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
