package emby

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
)

func TestParseLibrariesFromBody_EmptyItemsObject(t *testing.T) {
	body := []byte(`{"Items":[]}`)

	libraries, err := parseLibrariesFromBody(body)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if libraries == nil {
		t.Fatalf("expected empty slice, got nil")
	}
	if len(libraries) != 0 {
		t.Fatalf("expected empty list, got: %d", len(libraries))
	}
}

func TestGetLibraryItemsFetchAllWhenMaxItemsIsNonPositive(t *testing.T) {
	total := 401
	requestCount := 0

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/emby/Items" {
			http.NotFound(w, r)
			return
		}
		requestCount++

		start, _ := strconv.Atoi(r.URL.Query().Get("StartIndex"))
		limit, _ := strconv.Atoi(r.URL.Query().Get("Limit"))

		if start > total {
			start = total
		}
		end := start + limit
		if end > total {
			end = total
		}

		items := make([]EmbyLibraryItem, 0, end-start)
		for i := start; i < end; i++ {
			items = append(items, EmbyLibraryItem{
				ID:   strconv.Itoa(i + 1),
				Type: "Movie",
			})
		}

		resp := embyLibraryItemsResponse{
			Items:            items,
			TotalRecordCount: total,
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	t.Setenv("EMBY_URL", server.URL)
	t.Setenv("EMBY_API_KEY", "test-key")

	s := &EmbyService{
		baseURL: server.URL,
		apiKey:  "test-key",
		client: server.Client(),
	}

	items, err := s.GetLibraryItems("library_1", 0)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if len(items) != total {
		t.Fatalf("expected %d items, got %d", total, len(items))
	}
	if requestCount != 3 {
		t.Fatalf("expected 3 page requests, got %d", requestCount)
	}
}

func TestGetLibraryItemsRespectsMaxItemsLimit(t *testing.T) {
	total := 401
	requestCount := 0

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/emby/Items" {
			http.NotFound(w, r)
			return
		}
		requestCount++

		start, _ := strconv.Atoi(r.URL.Query().Get("StartIndex"))
		limit, _ := strconv.Atoi(r.URL.Query().Get("Limit"))

		if start > total {
			start = total
		}
		end := start + limit
		if end > total {
			end = total
		}

		items := make([]EmbyLibraryItem, 0, end-start)
		for i := start; i < end; i++ {
			items = append(items, EmbyLibraryItem{
				ID:   strconv.Itoa(i + 1),
				Type: "Movie",
			})
		}

		resp := embyLibraryItemsResponse{
			Items:            items,
			TotalRecordCount: total,
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	t.Setenv("EMBY_URL", server.URL)
	t.Setenv("EMBY_API_KEY", "test-key")

	s := &EmbyService{
		baseURL: server.URL,
		apiKey:  "test-key",
		client: server.Client(),
	}

	items, err := s.GetLibraryItems("library_1", 250)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if len(items) != 250 {
		t.Fatalf("expected 250 items, got %d", len(items))
	}
	if requestCount != 2 {
		t.Fatalf("expected 2 page requests, got %d", requestCount)
	}
}
