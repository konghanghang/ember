package emby

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
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

func TestParseLibrariesFromBody_FiltersSystemCollections(t *testing.T) {
	body := []byte(`{"Items":[
		{"Guid":"lib_movies","Name":"电影","CollectionType":"movies","ItemCount":10},
		{"Guid":"lib_collections","Name":"合集","CollectionType":"boxsets","ItemCount":3},
		{"Guid":"lib_series","Name":"剧集","CollectionType":"tvshows","ItemCount":20}
	]}`)

	libraries, err := parseLibrariesFromBody(body)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if len(libraries) != 2 {
		t.Fatalf("expected 2 libraries, got %d: %+v", len(libraries), libraries)
	}
	if libraries[0].ID != "lib_movies" || libraries[1].ID != "lib_series" {
		t.Fatalf("expected system collections to be filtered, got %+v", libraries)
	}
}

func TestParseLibrariesFromBody_FiltersSystemCollectionsFromDirectArray(t *testing.T) {
	body := []byte(`[
		{"ItemId":"lib_movies","Name":"电影","CollectionType":"movies"},
		{"ItemId":"lib_collections","Name":"Collections","CollectionType":"BoxSets"}
	]`)

	libraries, err := parseLibrariesFromBody(body)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if len(libraries) != 1 || libraries[0].ID != "lib_movies" {
		t.Fatalf("expected only ordinary library, got %+v", libraries)
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
		client:  server.Client(),
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
		client:  server.Client(),
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

func TestGetUserLibraryItemsByIDsUsesLibraryScopeAndBatchesCandidates(t *testing.T) {
	requestCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/emby/Users/admin_1/Items" {
			http.NotFound(w, r)
			return
		}
		requestCount++
		if r.URL.Query().Get("ParentId") != "/data/movies" || r.URL.Query().Get("Recursive") != "true" {
			t.Fatalf("unexpected library scope query: %s", r.URL.RawQuery)
		}
		ids := strings.Split(r.URL.Query().Get("Ids"), ",")
		if len(ids) > maxItemIDsPerBatch {
			t.Fatalf("expected at most %d candidate IDs per request, got %d", maxItemIDsPerBatch, len(ids))
		}
		_ = json.NewEncoder(w).Encode(embyLibraryItemsResponse{
			Items:            []EmbyLibraryItem{{ID: ids[0], Type: "Movie"}},
			TotalRecordCount: 1,
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
	ids := make([]string, 0, maxItemIDsPerBatch+1)
	for i := 0; i <= maxItemIDsPerBatch; i++ {
		ids = append(ids, strconv.Itoa(i+1))
	}

	items, err := s.GetUserLibraryItemsByIDs("admin_1", "/data/movies", ids)
	if err != nil {
		t.Fatalf("get scoped candidate items: %v", err)
	}
	if requestCount != 2 {
		t.Fatalf("expected 2 batched requests, got %d", requestCount)
	}
	if len(items) != 2 || items[0].ID != "1" || items[1].ID != "101" {
		t.Fatalf("unexpected scoped candidate items: %+v", items)
	}
}

func TestGetAdminLibraryContextRejectsMissingAdministrator(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/emby/Users" {
			t.Fatalf("unexpected request path: %s", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode([]map[string]any{
			{"Id": "user_1", "Policy": map[string]any{"IsAdministrator": false}},
		})
	}))
	defer server.Close()

	t.Setenv("EMBY_URL", server.URL)
	t.Setenv("EMBY_API_KEY", "token_1")
	service := &EmbyService{baseURL: server.URL, apiKey: "token_1", client: server.Client()}
	if _, err := service.GetAdminLibraryContext(); err == nil {
		t.Fatal("expected missing administrator to fail")
	}
}

func TestGetUserLibraryItemsByIDsUsesUserScopedPath(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/emby/Users/admin_1/Items" {
			t.Fatalf("expected user-scoped items path, got %s", r.URL.Path)
		}
		if r.URL.Query().Get("ParentId") != "library_1" || r.URL.Query().Get("Ids") != "item_1,item_2" {
			t.Fatalf("unexpected query: %s", r.URL.RawQuery)
		}
		_ = json.NewEncoder(w).Encode(embyLibraryItemsResponse{
			Items:            []EmbyLibraryItem{{ID: "item_2", Type: "Movie"}},
			TotalRecordCount: 1,
		})
	}))
	defer server.Close()

	t.Setenv("EMBY_URL", server.URL)
	t.Setenv("EMBY_API_KEY", "token_1")
	service := &EmbyService{baseURL: server.URL, apiKey: "token_1", client: server.Client()}
	items, err := service.GetUserLibraryItemsByIDs("admin_1", "library_1", []string{"item_1", "item_2"})
	if err != nil {
		t.Fatalf("get user-scoped items: %v", err)
	}
	if len(items) != 1 || items[0].ID != "item_2" {
		t.Fatalf("unexpected items: %+v", items)
	}
}

func TestGetUserLibraryItemsByIDsRejectsUnexpectedResponseID(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(embyLibraryItemsResponse{
			Items:            []EmbyLibraryItem{{ID: "unexpected", Type: "Movie"}},
			TotalRecordCount: 1,
		})
	}))
	defer server.Close()

	t.Setenv("EMBY_URL", server.URL)
	t.Setenv("EMBY_API_KEY", "token_1")
	service := &EmbyService{baseURL: server.URL, apiKey: "token_1", client: server.Client()}
	if _, err := service.GetUserLibraryItemsByIDs("admin_1", "library_1", []string{"item_1"}); err == nil {
		t.Fatal("expected unexpected response ID to fail")
	}
}
