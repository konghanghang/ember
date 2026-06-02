package emby

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestLatestIncludeItemTypes(t *testing.T) {
	if got := latestIncludeItemTypes("Movie"); got != "Movie" {
		t.Fatalf("expected Movie to stay Movie, got %s", got)
	}
	if got := latestIncludeItemTypes("Series"); got != "Episode" {
		t.Fatalf("expected Series latest query to use Episode, got %s", got)
	}
}

func TestGetUserPolicyRawConfiguredReadsPolicyFromUserDetail(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/emby/Users/emby_1" {
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.String())
		}
		if r.URL.Query().Get("api_key") != "token_1" {
			t.Fatalf("expected api_key query, got %q", r.URL.Query().Get("api_key"))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"Id":"emby_1","Name":"alice","Policy":{"EnableAllFolders":true,"EnabledFolders":["lib_a"]}}`))
	}))
	defer server.Close()

	service := &EmbyService{baseURL: server.URL, apiKey: "token_1", client: server.Client()}
	policy, err := service.getUserPolicyRawConfigured("emby_1")
	if err != nil {
		t.Fatalf("expected policy read success, got %v", err)
	}
	if got, ok := policy["EnableAllFolders"].(bool); !ok || !got {
		t.Fatalf("expected EnableAllFolders=true, got %+v", policy["EnableAllFolders"])
	}
}

func TestGetUserPolicyRawConfiguredFallsBackWhenUserDetailHasNoPolicy(t *testing.T) {
	requests := make([]string, 0, 2)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/emby/Users/emby_1":
			_, _ = w.Write([]byte(`{"Id":"emby_1","Name":"alice"}`))
		case "/emby/Users/emby_1/Policy":
			_, _ = w.Write([]byte(`{"EnableAllFolders":false,"EnabledFolders":["lib_b"]}`))
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()

	service := &EmbyService{baseURL: server.URL, apiKey: "token_1", client: server.Client()}
	policy, err := service.getUserPolicyRawConfigured("emby_1")
	if err != nil {
		t.Fatalf("expected fallback policy read success, got %v", err)
	}
	if got, ok := policy["EnableAllFolders"].(bool); !ok || got {
		t.Fatalf("expected EnableAllFolders=false, got %+v", policy["EnableAllFolders"])
	}
	if len(requests) != 2 || requests[0] != "/emby/Users/emby_1" || requests[1] != "/emby/Users/emby_1/Policy" {
		t.Fatalf("expected user detail then legacy policy endpoint, got %+v", requests)
	}
}

func TestGetUserPolicyRawConfiguredDoesNotFallbackWhenUserMissing(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/emby/Users/missing" {
			t.Fatalf("unexpected fallback request %s", r.URL.Path)
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	service := &EmbyService{baseURL: server.URL, apiKey: "token_1", client: server.Client()}
	_, err := service.getUserPolicyRawConfigured("missing")
	if !errors.Is(err, ErrEmbyUserNotFound) {
		t.Fatalf("expected ErrEmbyUserNotFound, got %v", err)
	}
}
