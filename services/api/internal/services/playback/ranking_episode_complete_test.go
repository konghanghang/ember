package playback

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	embyint "github.com/konghang/ember/backend/internal/integrations/emby"
)

// TestEpisodeAllowlistAggregatesEveryEpisode protects ranking and total duration
// from truncation before Series aggregation, including tails of already ranked shows.
func TestEpisodeAllowlistAggregatesEveryEpisode(t *testing.T) {
	for _, tc := range []struct{ extra, excluded int }{{0, 0}, {0, 20}, {20, 20}} {
		t.Run(fmt.Sprintf("ranked_series_tail=%d/excluded=%d", tc.extra, tc.excluded), func(t *testing.T) {
			extra := tc.extra
			type episode struct {
				id, series string
				duration   int64
			}
			var episodes []episode
			add := func(series string, count int, duration int64) {
				for i := 0; i < count; i++ {
					episodes = append(episodes, episode{fmt.Sprintf("%s_%d_%d", series, duration, i), series, duration})
				}
			}
			for i := 0; i < 10; i++ {
				add(fmt.Sprintf("A%d", i), 30, 100)
			}
			add("B", 40, 99)
			add("A0", extra, 98)
			add("excluded", tc.excluded, 101)
			sort.Slice(episodes, func(i, j int) bool {
				if episodes[i].duration != episodes[j].duration {
					return episodes[i].duration > episodes[j].duration
				}
				return episodes[i].id < episodes[j].id
			})
			byID := map[string]episode{}
			for _, ep := range episodes {
				byID[ep.id] = ep
			}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/emby/user_usage_stats/submit_custom_query":
					if r.Method != http.MethodPost {
						t.Errorf("unexpected query method %s", r.Method)
					}
					var req struct {
						SQL string `json:"CustomQueryString"`
					}
					if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
						t.Error(err)
						w.WriteHeader(400)
						return
					}
					limit := len(episodes)
					if matches := regexp.MustCompile(`LIMIT (\d+)`).FindStringSubmatch(req.SQL); len(matches) > 0 {
						n, _ := strconv.Atoi(matches[1])
						if n < limit {
							limit = n
						}
					}
					rows := make([][]string, 0, limit)
					for _, ep := range episodes[:limit] {
						rows = append(rows, []string{ep.id, ep.id, "episode_item", "1", strconv.FormatInt(ep.duration, 10)})
					}
					_ = json.NewEncoder(w).Encode(map[string]any{"colums": []string{"item_key", "item_name", "item_source_type", "play_count", "total_duration"}, "results": rows, "message": ""})
				case "/emby/Items":
					ids := strings.Split(r.URL.Query().Get("Ids"), ",")
					if len(ids) > 100 {
						t.Errorf("item lookup was not batched: %d", len(ids))
					}
					var items []map[string]any
					for _, id := range ids {
						if ep, ok := byID[id]; ok {
							items = append(items, map[string]any{"Id": id, "Type": "Episode", "SeriesId": ep.series, "SeriesName": ep.series})
						}
					}
					_ = json.NewEncoder(w).Encode(map[string]any{"Items": items, "TotalRecordCount": len(items)})
				case "/emby/Users/admin/Items":
					if r.URL.Query().Get("ParentId") != "allowed" {
						t.Errorf("unexpected library %s", r.URL.RawQuery)
					}
					var items []map[string]any
					for _, id := range strings.Split(r.URL.Query().Get("Ids"), ",") {
						if id != "excluded" {
							items = append(items, map[string]any{"Id": id, "Type": "Series"})
						}
					}
					_ = json.NewEncoder(w).Encode(map[string]any{"Items": items, "TotalRecordCount": len(items)})
				default:
					t.Errorf("unexpected path %s", r.URL.Path)
					w.WriteHeader(404)
				}
			}))
			defer server.Close()
			t.Setenv("EMBY_URL", server.URL)
			t.Setenv("EMBY_API_KEY", "test-key")
			t.Setenv("CRON_TIMEZONE", "Asia/Singapore")
			svc := &PlaybackRankingService{embyService: embyint.NewEmbyService()}
			start := time.Date(2026, 9, 14, 0, 0, 0, 0, loadCronTimezone())
			rankings, total, err := svc.fetchEpisodeRankingWithFilter(playbackActivityColumns{itemID: "ItemId", itemName: "ItemName"}, start, start.AddDate(0, 0, 1), rankingLibraryFilter{adminUserID: "admin", allowedLibraryIDs: map[string]struct{}{"allowed": {}}})
			if err != nil {
				t.Fatal(err)
			}
			if len(rankings) != 10 {
				t.Fatalf("got %d rankings", len(rankings))
			}
			wantFirst := "B"
			wantDuration := int64(3960)
			if extra > 0 {
				wantFirst = "A0"
				wantDuration = 3000 + int64(extra)*98
			}
			if rankings[0].ItemKey != wantFirst || rankings[0].Duration != wantDuration {
				t.Fatalf("top=%s/%d, want %s/%d", rankings[0].ItemKey, rankings[0].Duration, wantFirst, wantDuration)
			}
			if total != 33960+int64(extra)*98 {
				t.Fatalf("total=%d, want %d", total, 33960+int64(extra)*98)
			}
			for _, row := range rankings {
				if row.ItemKey == "excluded" {
					t.Fatal("non-allowlisted series ranked")
				}
			}
		})
	}
}
