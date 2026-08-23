package playbackgateway

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestItemContainerSnapshotCachePrefersExactSourceAndExpires(t *testing.T) {
	now := time.Date(2026, 8, 23, 19, 0, 0, 0, time.UTC)
	cache := newItemContainerSnapshotCache(8, time.Minute)
	cache.now = func() time.Time { return now }
	written := cache.Record("mapping-1", "item-1", "mp4", []itemContainerSource{
		{MediaSourceID: "source-1", Container: "MKV"},
	})
	if written != 1 {
		t.Fatalf("written=%d, want 1 exact source", written)
	}
	if container, ok := cache.Lookup("mapping-1", "item-1", "source-1"); !ok || container != "mkv" {
		t.Fatalf("exact lookup=(%q,%t)", container, ok)
	}
	if _, ok := cache.Lookup("mapping-1", "item-1", "other-source"); ok {
		t.Fatal("top-level Container overrode a non-matching MediaSource")
	}
	if written := cache.Record("mapping-1", "item-2", "mp4", nil); written != 1 {
		t.Fatalf("top-level written=%d", written)
	}
	if container, ok := cache.Lookup("mapping-1", "item-2", "unknown-source"); !ok || container != "mp4" {
		t.Fatalf("top-level lookup=(%q,%t)", container, ok)
	}
	if _, ok := cache.Lookup("other-mapping", "item-1", "source-1"); ok {
		t.Fatal("snapshot crossed mapping identity")
	}
	now = now.Add(time.Minute)
	if _, ok := cache.Lookup("mapping-1", "item-1", "source-1"); ok {
		t.Fatal("expired snapshot remained available")
	}
}

func TestItemContainerSnapshotCacheRejectsAmbiguousOrUnsafeValues(t *testing.T) {
	cache := newItemContainerSnapshotCache(8, time.Minute)
	if written := cache.Record("mapping-1", "item-1", "", []itemContainerSource{
		{MediaSourceID: "source-1", Container: "mkv"},
		{MediaSourceID: "source-1", Container: "mp4"},
	}); written != 0 {
		t.Fatalf("ambiguous written=%d", written)
	}
	if written := cache.Record("mapping-1", "item-1", "mkv&api_key=value", nil); written != 0 {
		t.Fatalf("unsafe written=%d", written)
	}
}

func TestUserItemDetailPathRequiresExactDepthAndUnescapedIdentity(t *testing.T) {
	tests := []struct {
		path     string
		wantUser string
		wantItem string
		wantOK   bool
	}{
		{path: "/emby/Users/user-1/Items/item-1", wantUser: "user-1", wantItem: "item-1", wantOK: true},
		{path: "/emby/users/user-1/items/item-1", wantUser: "user-1", wantItem: "item-1", wantOK: true},
		{path: "/emby/Users/user-1/Items/item-1/LocalTrailers"},
		{path: "/emby/Users/user%2D1/Items/item-1"},
	}
	for _, test := range tests {
		request := httptest.NewRequest(http.MethodGet, test.path, nil)
		userID, itemID, ok := userItemDetailPath(request.URL)
		if userID != test.wantUser || itemID != test.wantItem || ok != test.wantOK {
			t.Fatalf("path=%s result=(%q,%q,%t)", test.path, userID, itemID, ok)
		}
	}
}

func TestGatewayDoesNotReuseContainerAcrossPrincipalUserMismatch(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if strings.Contains(request.URL.Path, "/Users/") {
			writer.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(writer, `{"Id":"item-1","Container":"mkv","MediaSources":[{"Id":"source-1","Container":"mkv"}]}`)
			return
		}
		writer.WriteHeader(http.StatusNotFound)
	}))
	defer upstream.Close()

	var logs bytes.Buffer
	gateway := newTestGateway(t, upstream.URL, &fakeTokenService{principal: fixturePrincipal()}, &logs)

	itemRequest := httptest.NewRequest(http.MethodGet, "/Users/other-user/Items/item-1", nil)
	itemRequest.Header.Set(accessTokenHeader, fixtureAccessToken)
	gateway.ServeHTTP(httptest.NewRecorder(), itemRequest)

	videoRequest := httptest.NewRequest(http.MethodGet, "/Videos/item-1/stream?MediaSourceId=source-1&Static=true", nil)
	videoRequest.Header.Set(accessTokenHeader, fixtureAccessToken)
	videoResponse := httptest.NewRecorder()
	gateway.ServeHTTP(videoResponse, videoRequest)

	if videoResponse.Code != http.StatusNotFound {
		t.Fatalf("video response=%d, want upstream 404", videoResponse.Code)
	}
	if strings.Contains(logs.String(), "code=item_container_snapshot_recorded") || !strings.Contains(logs.String(), "reasonCode=route_not_accelerated") {
		t.Fatalf("logs=%q", logs.String())
	}
}
