package playbackgateway

import (
	"sync"
	"testing"
	"time"

	"github.com/konghang/ember/backend/internal/services/p115quota"
)

// bindingFixture keeps a real admitted session active beyond proof lifetime.
func bindingFixture(t *testing.T) (*playbackProofCache, *time.Time, PlaybackProof) {
	t.Helper()
	now := time.Now()
	cache := newPlaybackProofCache(4, time.Minute)
	cache.now = func() time.Time { return now }
	proof := fixturePlaybackProof("mapping-1", "item-1", "source-1", "client")
	proof.transferIntent = true
	cache.Record([]PlaybackProof{proof})
	proof, _ = cache.Lookup(proof.MappingID, proof.ItemID, proof.MediaSourceID, proof.PlaySessionID)
	cache.rememberPlaybackSession(proof, fixturePrincipal())
	cache.updatePlaybackBinding(fixturePrincipal(), proof.ItemID, proof.MediaSourceID, "client", p115quota.LeaseStateActive, true)
	now = now.Add(61 * time.Second)
	cache.Len()
	return cache, &now, proof
}

// TestPlaybackBindingRefreshRequiresExactMedia ensures independent fresh proof
// validation cannot borrow another principal/file's playback session.
func TestPlaybackBindingRefreshRequiresExactMedia(t *testing.T) {
	for _, kind := range []string{"matching", "device", "mapping", "item", "source", "path", "size", "server", "user", "container"} {
		t.Run(kind, func(t *testing.T) {
			cache, _, original := bindingFixture(t)
			proof := original
			proof.PlaySessionID = "new-internal"
			proof.transferIntent = false
			switch kind {
			case "device":
				proof.DeviceID = "other"
			case "mapping":
				proof.MappingID = "other"
			case "item":
				proof.ItemID = "other"
			case "source":
				proof.MediaSourceID = "other"
			case "path":
				proof.Path = "/mnt/media/other.mkv"
			case "size":
				proof.Size++
			case "server":
				proof.ServerID = "other"
			case "user":
				proof.UserID = "other"
			case "container":
				proof.Container = "mp4"
			}
			_, guard := cache.BeginOnDemand(fixturePrincipal(), proof.ItemID, proof.MediaSourceID)
			defer cache.ReleasePublication(guard)
			cache.PublishOnDemand(guard, []PlaybackProof{proof})
			current, _ := cache.Lookup(proof.MappingID, proof.ItemID, proof.MediaSourceID, proof.PlaySessionID)
			if (current.intentOriginSession == "client") != (kind == "matching") {
				t.Fatal("incorrect session inheritance")
			}
			if cache.CanCreateTransfer(current, fixturePrincipal()) {
				t.Fatal("playback continuity granted new transfer")
			}
		})
	}
}

// TestPlaybackBindingStopsAndExpiresWithoutResurrection covers delayed internal
// publications, delayed redirect completion and newer client observations.
func TestPlaybackBindingStopsAndExpiresWithoutResurrection(t *testing.T) {
	for _, kind := range []string{"stop", "expired", "new_client", "missing_lease"} {
		t.Run(kind, func(t *testing.T) {
			cache, now, original := bindingFixture(t)
			_, guard := cache.BeginOnDemand(fixturePrincipal(), original.ItemID, original.MediaSourceID)
			defer cache.ReleasePublication(guard)
			switch kind {
			case "stop":
				cache.stopPlaybackBinding(fixturePrincipal(), original.ItemID, "", "client")
			case "expired":
				*now = now.Add(2 * time.Minute)
				cache.updatePlaybackBinding(fixturePrincipal(), original.ItemID, "", "client", p115quota.LeaseStateActive, true)
			case "new_client":
				g := cache.BeginClient(original.MappingID, original.ItemID)
				cache.PublishClient(g, nil)
				cache.ReleasePublication(g)
			case "missing_lease":
				cache.updatePlaybackBinding(fixturePrincipal(), original.ItemID, "", "client", p115quota.LeaseStateActive, false)
			}
			cache.rememberPlaybackSession(original, fixturePrincipal())
			fresh := original
			fresh.PlaySessionID = "late-internal"
			fresh.transferIntent = false
			cache.PublishOnDemand(guard, []PlaybackProof{fresh})
			current, _ := cache.Lookup(fresh.MappingID, fresh.ItemID, fresh.MediaSourceID, fresh.PlaySessionID)
			if current.intentOriginSession != "" || len(cache.sessions) != 0 {
				t.Fatal("stale activity revived session binding")
			}
		})
	}
}

// TestStoppedInflightRedirectCannotRememberBinding guards the window between
// candidate confirmation and redirect publication while the proof is still live.
func TestStoppedInflightRedirectCannotRememberBinding(t *testing.T) {
	cache := newPlaybackProofCache(1, time.Minute)
	proof := fixturePlaybackProof("mapping-1", "item-1", "source-1", "client")
	proof.transferIntent = true
	cache.Record([]PlaybackProof{proof})
	proof, _ = cache.Lookup(proof.MappingID, proof.ItemID, proof.MediaSourceID, proof.PlaySessionID)
	cache.stopPlaybackBinding(fixturePrincipal(), proof.ItemID, proof.MediaSourceID, proof.PlaySessionID)
	cache.rememberPlaybackSession(proof, fixturePrincipal())
	if len(cache.sessions) != 0 {
		t.Fatal("late redirect restored stopped association")
	}
}

// TestPlaybackBindingsBoundCapacity rejects extra associations without evicting
// live sessions; expiration frees space without a background cleanup worker.
func TestPlaybackBindingsBoundCapacity(t *testing.T) {
	cache := newPlaybackProofCache(1, time.Minute)
	now := time.Now()
	cache.now = func() time.Time { return now }
	for _, item := range []string{"first", "second"} {
		proof := fixturePlaybackProof("mapping-1", item, "source-1", "client")
		proof.transferIntent = true
		cache.Record([]PlaybackProof{proof})
		proof, _ = cache.Lookup(proof.MappingID, proof.ItemID, proof.MediaSourceID, proof.PlaySessionID)
		cache.rememberPlaybackSession(proof, fixturePrincipal())
	}
	if len(cache.sessions) != 1 {
		t.Fatal("binding capacity exceeded")
	}
	now = now.Add(p115quota.ReservationTTL)
	cache.Len()
	if len(cache.sessions) != 0 {
		t.Fatal("expired binding retained capacity")
	}
}

// TestConcurrentStopWinsBindingPublication exercises both lock orderings of a
// late redirect and a successful stop, without sleeps or external dependencies.
func TestConcurrentStopWinsBindingPublication(t *testing.T) {
	for i := 0; i < 50; i++ {
		cache := newPlaybackProofCache(2, time.Minute)
		proof := fixturePlaybackProof("mapping-1", "item-1", "source-1", "client")
		proof.transferIntent = true
		cache.Record([]PlaybackProof{proof})
		proof, _ = cache.Lookup(proof.MappingID, proof.ItemID, proof.MediaSourceID, proof.PlaySessionID)
		var done sync.WaitGroup
		done.Add(2)
		go func() { defer done.Done(); cache.rememberPlaybackSession(proof, fixturePrincipal()) }()
		go func() {
			defer done.Done()
			cache.stopPlaybackBinding(fixturePrincipal(), proof.ItemID, proof.MediaSourceID, proof.PlaySessionID)
		}()
		done.Wait()
		if len(cache.sessions) != 0 {
			t.Fatal("concurrent redirect resurrected stopped association")
		}
	}
}
