package emby

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
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

func TestCreateEmbyUserConfiguredInitialDisabledBeforePassword(t *testing.T) {
	requests := make([]string, 0, 4)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r.Method+" "+r.URL.Path)
		w.Header().Set("Content-Type", "application/json")

		switch r.URL.Path {
		case "/emby/Users/New":
			if r.Method != http.MethodPost {
				t.Fatalf("expected create POST, got %s", r.Method)
			}
			if r.URL.Query().Get("api_key") != "token_1" {
				t.Fatalf("expected create api_key query, got %q", r.URL.Query().Get("api_key"))
			}
			var payload map[string]any
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatalf("failed to decode create payload: %v", err)
			}
			if payload["Name"] != "ember" {
				t.Fatalf("unexpected create payload: %+v", payload)
			}
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"Id":"emby_1","Name":"ember"}`))
		case "/emby/Users/emby_1":
			if r.Method != http.MethodGet {
				t.Fatalf("expected policy read GET, got %s", r.Method)
			}
			_, _ = w.Write([]byte(`{"Id":"emby_1","Name":"ember","Policy":{"IsDisabled":false,"IsAdministrator":false,"EnableAllFolders":true,"SimultaneousStreamLimit":1}}`))
		case "/emby/Users/emby_1/Policy":
			if r.Method != http.MethodPost {
				t.Fatalf("expected policy write POST, got %s", r.Method)
			}
			var payload map[string]any
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatalf("failed to decode policy payload: %v", err)
			}
			if payload["IsDisabled"] != true {
				t.Fatalf("expected policy to disable user before password, got %+v", payload)
			}
			w.WriteHeader(http.StatusNoContent)
		case "/emby/Users/emby_1/Password":
			if r.Method != http.MethodPost {
				t.Fatalf("expected password POST, got %s", r.Method)
			}
			var payload map[string]any
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatalf("failed to decode password payload: %v", err)
			}
			if payload["NewPw"] != "secret123" {
				t.Fatalf("unexpected password payload: %+v", payload)
			}
			w.WriteHeader(http.StatusNoContent)
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()

	t.Setenv("EMBY_URL", server.URL)
	t.Setenv("EMBY_API_KEY", "token_1")

	service := &EmbyService{baseURL: server.URL, apiKey: "token_1", client: server.Client()}
	user, err := service.createEmbyUserConfigured("ember", "secret123", true)
	if err != nil {
		t.Fatalf("expected create success, got %v", err)
	}
	if user.ID != "emby_1" {
		t.Fatalf("unexpected user: %+v", user)
	}

	expected := []string{
		"POST /emby/Users/New",
		"GET /emby/Users/emby_1",
		"POST /emby/Users/emby_1/Policy",
		"POST /emby/Users/emby_1/Password",
	}
	if len(requests) != len(expected) {
		t.Fatalf("expected requests %+v, got %+v", expected, requests)
	}
	for i := range expected {
		if requests[i] != expected[i] {
			t.Fatalf("expected request order %+v, got %+v", expected, requests)
		}
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

	t.Setenv("EMBY_URL", server.URL)
	t.Setenv("EMBY_API_KEY", "token_1")
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

	t.Setenv("EMBY_URL", server.URL)
	t.Setenv("EMBY_API_KEY", "token_1")
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

	t.Setenv("EMBY_URL", server.URL)
	t.Setenv("EMBY_API_KEY", "token_1")
	service := &EmbyService{baseURL: server.URL, apiKey: "token_1", client: server.Client()}
	_, err := service.getUserPolicyRawConfigured("missing")
	if !errors.Is(err, ErrEmbyUserNotFound) {
		t.Fatalf("expected ErrEmbyUserNotFound, got %v", err)
	}
}

func TestQueryPlaybackStatsUsesPluginContract(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/emby/user_usage_stats/submit_custom_query" {
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.String())
		}
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if payload["CustomQueryString"] != "SELECT 1" {
			t.Fatalf("unexpected SQL payload: %+v", payload)
		}
		if replace, ok := payload["ReplaceUserId"].(bool); !ok || replace {
			t.Fatalf("expected ReplaceUserId=false, got %+v", payload)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"colums":  []string{"value"},
			"results": [][]string{{"1"}},
			"message": "",
		})
	}))
	defer server.Close()

	t.Setenv("EMBY_URL", server.URL)
	t.Setenv("EMBY_API_KEY", "token_1")
	service := &EmbyService{baseURL: server.URL, apiKey: "token_1", client: server.Client()}
	resp, err := service.QueryPlaybackStats("SELECT 1")
	if err != nil {
		t.Fatalf("query playback stats: %v", err)
	}
	if len(resp.Colums) != 1 || resp.Colums[0] != "value" {
		t.Fatalf("unexpected response: %+v", resp)
	}
}

func TestQueryPlaybackStatsReturnsTypedPluginSQLError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"colums":  []string{},
			"results": []any{},
			"message": "Error Running Query</br>no such column: PauseDuration<pre>sensitive stack</pre>",
		})
	}))
	defer server.Close()

	t.Setenv("EMBY_URL", server.URL)
	t.Setenv("EMBY_API_KEY", "token_1")
	service := &EmbyService{baseURL: server.URL, apiKey: "token_1", client: server.Client()}
	_, err := service.QueryPlaybackStats("SELECT PauseDuration FROM PlaybackActivity")
	var pluginErr *PlaybackReportingQueryError
	if !errors.As(err, &pluginErr) {
		t.Fatalf("expected PlaybackReportingQueryError, got %v", err)
	}
	if !strings.Contains(err.Error(), "PauseDuration") || strings.Contains(err.Error(), "sensitive stack") {
		t.Fatalf("expected safe actionable plugin error, got %q", err.Error())
	}
}

func TestQueryPlaybackStatsAllowsNoDataMessage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"colums":  []string{},
			"results": []any{},
			"message": "Query executed, no data returned.",
		})
	}))
	defer server.Close()

	t.Setenv("EMBY_URL", server.URL)
	t.Setenv("EMBY_API_KEY", "token_1")
	service := &EmbyService{baseURL: server.URL, apiKey: "token_1", client: server.Client()}
	resp, err := service.QueryPlaybackStats("SELECT 1 WHERE 0")
	if err != nil {
		t.Fatalf("expected no-data message to remain successful, got %v", err)
	}
	if resp.Message != "Query executed, no data returned." {
		t.Fatalf("unexpected response: %+v", resp)
	}
}
