package playbackgateway

import (
	"fmt"
	"sync"
	"testing"
	"time"
)

func TestPlaybackProofCacheRequiresExactCompositeKey(t *testing.T) {
	now := time.Date(2026, 8, 22, 10, 0, 0, 0, time.UTC)
	cache := newPlaybackProofCache(4, 5*time.Minute)
	cache.now = func() time.Time { return now }
	proof := fixturePlaybackProof("mapping-1", "item-1", "source-1", "session-1")
	if count := cache.Record([]PlaybackProof{proof}); count != 1 {
		t.Fatalf("Record() count = %d, want 1", count)
	}
	got, ok := cache.Lookup("mapping-1", "item-1", "source-1", "session-1")
	if !ok || got.MappingID != proof.MappingID || got.Path != proof.Path ||
		!got.AuthorizedAt.Equal(now) || !got.ExpiresAt.Equal(now.Add(5*time.Minute)) {
		t.Fatalf("Lookup() = (%+v, %t)", got, ok)
	}
	for _, key := range [][4]string{
		{"mapping-2", "item-1", "source-1", "session-1"},
		{"mapping-1", "item-2", "source-1", "session-1"},
		{"mapping-1", "item-1", "source-2", "session-1"},
		{"mapping-1", "item-1", "source-1", "session-2"},
	} {
		if _, ok := cache.Lookup(key[0], key[1], key[2], key[3]); ok {
			t.Fatalf("Lookup(%v) reused another proof", key)
		}
	}
}

func TestPlaybackProofCacheExpiresAndEvictsOldest(t *testing.T) {
	now := time.Date(2026, 8, 22, 10, 0, 0, 0, time.UTC)
	cache := newPlaybackProofCache(2, time.Minute)
	cache.now = func() time.Time { return now }
	cache.Record([]PlaybackProof{fixturePlaybackProof("mapping-1", "item-1", "source-1", "session-1")})
	now = now.Add(time.Second)
	cache.Record([]PlaybackProof{fixturePlaybackProof("mapping-1", "item-2", "source-2", "session-2")})
	now = now.Add(time.Second)
	cache.Record([]PlaybackProof{fixturePlaybackProof("mapping-1", "item-3", "source-3", "session-3")})
	if _, ok := cache.Lookup("mapping-1", "item-1", "source-1", "session-1"); ok {
		t.Fatal("oldest proof was not evicted")
	}
	if _, ok := cache.Lookup("mapping-1", "item-2", "source-2", "session-2"); !ok {
		t.Fatal("newer proof was evicted")
	}
	now = now.Add(2 * time.Minute)
	if _, ok := cache.Lookup("mapping-1", "item-2", "source-2", "session-2"); ok {
		t.Fatal("expired proof remained available")
	}
	if cache.Len() != 0 {
		t.Fatalf("Len() = %d, want 0 after lazy expiry", cache.Len())
	}
}

func TestPlaybackProofCacheRejectsInvalidProofWithoutMutation(t *testing.T) {
	cache := newPlaybackProofCache(2, time.Minute)
	proof := fixturePlaybackProof("", "item-1", "source-1", "session-1")
	if count := cache.Record([]PlaybackProof{proof}); count != 0 || cache.Len() != 0 {
		t.Fatalf("invalid Record() count=%d len=%d", count, cache.Len())
	}
}

func TestPlaybackProofCacheInvalidatesOnlyTargetMappingItem(t *testing.T) {
	cache := newPlaybackProofCache(8, time.Minute)
	cache.Record([]PlaybackProof{
		fixturePlaybackProof("mapping-1", "item-1", "source-1", "session-1"),
		fixturePlaybackProof("mapping-1", "item-1", "source-2", "session-2"),
		fixturePlaybackProof("mapping-1", "item-2", "source-1", "session-1"),
		fixturePlaybackProof("mapping-2", "item-1", "source-1", "session-1"),
	})
	cache.InvalidateItem("mapping-1", "item-1")
	if _, ok := cache.Lookup("mapping-1", "item-1", "source-1", "session-1"); ok {
		t.Fatal("target item proof remained after invalidation")
	}
	if _, ok := cache.Lookup("mapping-1", "item-2", "source-1", "session-1"); !ok {
		t.Fatal("different item was invalidated")
	}
	if _, ok := cache.Lookup("mapping-2", "item-1", "source-1", "session-1"); !ok {
		t.Fatal("different mapping was invalidated")
	}
}

func TestPlaybackProofCacheFindsLatestMediaSourceWithoutSessionKey(t *testing.T) {
	now := time.Date(2026, 8, 23, 19, 0, 0, 0, time.UTC)
	cache := newPlaybackProofCache(8, time.Minute)
	cache.now = func() time.Time { return now }
	cache.Record([]PlaybackProof{fixturePlaybackProof("mapping-1", "item-1", "source-1", "session-1")})
	now = now.Add(time.Second)
	cache.Record([]PlaybackProof{fixturePlaybackProof("mapping-1", "item-1", "source-1", "session-2")})
	proof, ok := cache.LookupLatestMediaSource("mapping-1", "item-1", "source-1")
	if !ok || proof.PlaySessionID != "session-2" {
		t.Fatalf("latest proof=(%+v,%t)", proof, ok)
	}
	if _, ok := cache.LookupLatestMediaSource("mapping-2", "item-1", "source-1"); ok {
		t.Fatal("latest lookup crossed mapping identity")
	}
	now = now.Add(time.Minute)
	if _, ok := cache.LookupLatestMediaSource("mapping-1", "item-1", "source-1"); ok {
		t.Fatal("expired latest proof remained available")
	}
}

func TestPlaybackProofCacheSupportsConcurrentRecordAndLookup(t *testing.T) {
	cache := newPlaybackProofCache(128, time.Minute)
	var workers sync.WaitGroup
	for index := 0; index < 32; index++ {
		workers.Add(1)
		go func(index int) {
			defer workers.Done()
			itemID := fmt.Sprintf("item-%d", index)
			proof := fixturePlaybackProof("mapping-1", itemID, "source-1", "session-1")
			cache.Record([]PlaybackProof{proof})
			if _, ok := cache.Lookup("mapping-1", itemID, "source-1", "session-1"); !ok {
				t.Errorf("proof %s missing after concurrent record", itemID)
			}
		}(index)
	}
	workers.Wait()
	if cache.Len() != 32 {
		t.Fatalf("Len() = %d, want 32", cache.Len())
	}
}

func fixturePlaybackProof(mappingID, itemID, mediaSourceID, playSessionID string) PlaybackProof {
	return PlaybackProof{
		MappingID: mappingID, ServerID: "server-1", UserID: "user-1", EmbyUserID: "emby-user-1",
		DeviceID: "device-1", ClientName: "Infuse", ItemID: itemID, MediaSourceID: mediaSourceID,
		PlaySessionID: playSessionID, Path: "/mnt/media/fixture.mkv", Size: 1024,
		Container: "mkv", SupportsDirectPlay: true,
	}
}
