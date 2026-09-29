package playback

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	embyint "github.com/konghang/ember/backend/internal/integrations/emby"
)

// TestRankingLibraryLookupOnlyQueriesUnresolvedIDs 比较同一批候选的请求数量，并保护缺失条目、缓存与分批边界。
func TestRankingLibraryLookupOnlyQueriesUnresolvedIDs(t *testing.T) {
	libraryIDs := []string{"lib_a", "lib_b", "lib_c", "lib_d"}
	allowed := map[string]struct{}{"lib_a": {}, "lib_b": {}, "lib_c": {}, "lib_d": {}}
	for _, kind := range []string{"movie", "series"} {
		for _, tc := range []struct {
			name      string
			matches   [4]int
			wantCalls int
			wantIDs   int
			missing   int
		}{
			{name: "all in first library", matches: [4]int{300}, wantCalls: 3, wantIDs: 300},
			{name: "spread across libraries", matches: [4]int{150, 100, 50}, wantCalls: 6, wantIDs: 500},
			{name: "some missing", matches: [4]int{150, 100, 40}, wantCalls: 7, wantIDs: 510, missing: 10},
			{name: "all missing", wantCalls: 12, wantIDs: 1200, missing: 300},
		} {
			t.Run(kind+"/"+tc.name, func(t *testing.T) {
				ids := make([]string, 300)
				want := make(map[string]string, len(ids))
				for i := range ids {
					ids[i] = fmt.Sprintf("item_%03d", i)
					want[ids[i]] = rankingUnknownLibraryID
					offset := 0
					for index, count := range tc.matches {
						if i >= offset && i < offset+count {
							want[ids[i]] = libraryIDs[index]
						}
						offset += count
					}
				}
				type lookup struct {
					libraryID string
					ids       []string
				}
				calls := make(chan lookup, 32)
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					query := r.URL.Query()
					requestedIDs := strings.Split(query.Get("Ids"), ",")
					if r.Method != http.MethodGet || r.URL.Path != "/emby/Users/admin/Items" ||
						query.Get("Recursive") != "true" || query.Get("Limit") != strconv.Itoa(len(requestedIDs)) || len(requestedIDs) > 100 {
						t.Error("invalid library lookup contract or batch size")
					}
					libraryID := query.Get("ParentId")
					calls <- lookup{libraryID: libraryID, ids: requestedIDs}
					items := []map[string]string{}
					for _, id := range requestedIDs {
						if want[id] == libraryID {
							items = append(items, map[string]string{"Id": id})
						}
					}
					_ = json.NewEncoder(w).Encode(map[string]any{"Items": items, "TotalRecordCount": len(items)})
				}))
				defer server.Close()
				t.Setenv("EMBY_URL", server.URL)
				t.Setenv("EMBY_API_KEY", "fixture-key")
				service := &PlaybackRankingService{embyService: embyint.NewEmbyService(), entityLibraryCache: newSFCache[string, string](time.Hour)}
				got, err := service.resolveEntityLibraries("admin", kind, ids, allowed)
				if err != nil || !reflect.DeepEqual(got, want) {
					t.Fatalf("lookup changed membership: err=%v got=%v", err, got)
				}
				requestCount, candidateCount, repeatedIDs := len(calls), 0, 0
				matched := map[string]bool{}
				for len(calls) > 0 {
					call := <-calls
					candidateCount += len(call.ids)
					for _, id := range call.ids {
						if matched[id] {
							repeatedIDs++
						}
						if want[id] == call.libraryID {
							matched[id] = true
						}
					}
				}
				if requestCount != tc.wantCalls || candidateCount != tc.wantIDs || repeatedIDs != 0 {
					t.Fatalf("calls=%d candidates=%d repeatedMatchedIDs=%d, want calls=%d candidates=%d", requestCount, candidateCount, repeatedIDs, tc.wantCalls, tc.wantIDs)
				}

				// 成功归属沿用已有缓存；缺失结果仍要重查，不能新增负缓存。
				got, err = service.resolveEntityLibraries("admin", kind, ids, allowed)
				wantWarmCalls := ((tc.missing + 99) / 100) * len(libraryIDs)
				if err != nil || !reflect.DeepEqual(got, want) || len(calls) != wantWarmCalls {
					t.Fatalf("unexpected warm lookup: calls=%d want=%d err=%v", len(calls), wantWarmCalls, err)
				}
			})
		}
	}
}

// TestRankingLibraryLookupStopsOnUnresolvedFailure 保留部分匹配后的错误语义，禁止失败变成缺失或继续查询。
func TestRankingLibraryLookupStopsOnUnresolvedFailure(t *testing.T) {
	calls := make(chan string, 3)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		libraryID := r.URL.Query().Get("ParentId")
		calls <- libraryID
		switch libraryID {
		case "lib_a":
			_ = json.NewEncoder(w).Encode(map[string]any{"Items": []map[string]string{{"Id": "resolved"}}, "TotalRecordCount": 1})
		case "lib_b":
			if r.URL.Query().Get("Ids") != "unresolved" {
				t.Error("only unresolved IDs should reach the failing library")
			}
			http.Error(w, "fixture failure", http.StatusForbidden)
		default:
			t.Error("lookup must stop after a request failure")
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	t.Setenv("EMBY_URL", server.URL)
	t.Setenv("EMBY_API_KEY", "fixture-key")
	service := &PlaybackRankingService{embyService: embyint.NewEmbyService()}
	got, err := service.resolveEntityLibraries("admin", "movie", []string{"resolved", "unresolved"}, map[string]struct{}{"lib_a": {}, "lib_b": {}, "lib_c": {}})
	if err == nil || got != nil || len(calls) != 2 {
		t.Fatalf("partial success must not hide failure: got=%v err=%v calls=%d", got, err, len(calls))
	}
}

// TestRankingLibraryLookupKeepsPositiveCacheScoped 验证提前结束后，成功缓存仍按用户、类型和完整选库范围隔离。
func TestRankingLibraryLookupKeepsPositiveCacheScoped(t *testing.T) {
	calls := make(chan struct{}, 32)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls <- struct{}{}
		items := []map[string]string{}
		if r.URL.Path == "/emby/Users/admin_1/Items" {
			items = append(items, map[string]string{"Id": "item"})
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"Items": items, "TotalRecordCount": len(items)})
	}))
	defer server.Close()
	t.Setenv("EMBY_URL", server.URL)
	t.Setenv("EMBY_API_KEY", "fixture-key")
	service := &PlaybackRankingService{embyService: embyint.NewEmbyService(), entityLibraryCache: newSFCache[string, string](time.Hour)}
	for _, tc := range []struct {
		name, user, kind, wantLibrary string
		libraries                     map[string]struct{}
		wantCalls                     int
	}{
		{"cold overlapping views", "admin_1", "movie", "lib_a", map[string]struct{}{"lib_a": {}, "lib_b": {}}, 1},
		{"same context cached", "admin_1", "movie", "lib_a", map[string]struct{}{"lib_a": {}, "lib_b": {}}, 0},
		{"narrowed selection", "admin_1", "movie", "lib_b", map[string]struct{}{"lib_b": {}}, 1},
		{"another user", "admin_2", "movie", rankingUnknownLibraryID, map[string]struct{}{"lib_a": {}, "lib_b": {}}, 2},
		{"another kind", "admin_1", "series", "lib_a", map[string]struct{}{"lib_a": {}, "lib_b": {}}, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			before := len(calls)
			got, err := service.resolveEntityLibraries(tc.user, tc.kind, []string{"item"}, tc.libraries)
			if err != nil || got["item"] != tc.wantLibrary || len(calls)-before != tc.wantCalls {
				t.Fatalf("membership=%v calls=%d, want library=%s calls=%d err=%v", got, len(calls)-before, tc.wantLibrary, tc.wantCalls, err)
			}
		})
	}
}
