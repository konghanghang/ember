package emby

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// TestRetentionAggregateRejectsIncompleteEvidence protects against accidental zero-watch invalidation.
func TestRetentionAggregateRejectsIncompleteEvidence(t *testing.T) {
	for _, tc := range []struct {
		values  []string
		valid   bool
		seconds int64
	}{
		{[]string{"120", "0"}, true, 120}, {[]string{"0", "0"}, true, 0},
		{[]string{"120", "1"}, false, 0}, {[]string{"-1", "0"}, false, 0}, {[]string{"bad", "0"}, false, 0}, {[]string{"120"}, false, 0},
	} {
		got, err := parseRetentionAggregate(tc.values)
		if (err == nil) != tc.valid || tc.valid && got != tc.seconds {
			t.Fatalf("%v => %d %v", tc.values, got, err)
		}
	}
}

// TestRetentionQueryContract locks the fixed plugin path, time boundary and strict response semantics.
func TestRetentionQueryContract(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		ok         bool
	}{
		{"success", `{"colums":["watched_seconds","invalid_rows"],"results":[["3600","0"]]}`, true},
		{"sql_error", `{"message":"Error Running Query</br>failure","colums":[],"results":[]}`, false},
		{"truncated", `{"colums":["watched_seconds","invalid_rows"],"results":[]}`, false},
		{"corrupt", `{"colums":["watched_seconds","invalid_rows"],"results":[["3600","1"]]}`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != "POST" || r.URL.Path != "/emby/user_usage_stats/submit_custom_query" || r.Header.Get("X-Emby-Token") != "fixture-key" {
					t.Errorf("wrong request")
				}
				var request struct {
					CustomQueryString string
					ReplaceUserId     bool
				}
				if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
					t.Error(err)
				}
				for _, want := range []string{"UserId = 'fixture''user'", "ItemType IN ('Movie','Episode')", "DateCreated >= '2026-10-01 00:00:00'", "DateCreated < '2026-10-02 00:00:00'"} {
					if !strings.Contains(request.CustomQueryString, want) {
						t.Errorf("missing %s", want)
					}
				}
				if request.ReplaceUserId {
					t.Error("must preserve user identity")
				}
				fmt.Fprint(w, tc.body)
			}))
			defer server.Close()
			t.Setenv("EMBY_URL", server.URL)
			t.Setenv("EMBY_API_KEY", "fixture-key")
			svc := &EmbyService{baseURL: server.URL, apiKey: "fixture-key", client: server.Client()}
			loc, _ := time.LoadLocation("Asia/Shanghai")
			start := time.Date(2026, 10, 1, 0, 0, 0, 0, loc)
			seconds, err := svc.WatchRetentionSeconds(context.Background(), "fixture'user", start, start.AddDate(0, 0, 1), loc)
			if (err == nil) != tc.ok || tc.ok && seconds != 3600 {
				t.Fatalf("seconds=%d err=%v", seconds, err)
			}
		})
	}
}
