package directplay

import (
	"context"
	"testing"
	"time"

	p115 "github.com/konghang/ember/backend/internal/integrations/p115"
	"github.com/konghang/ember/backend/internal/services/p115account"
	"github.com/konghang/ember/backend/internal/services/p115quota"
)

// TestActivePlaybackKeepsValidURLBeyondMetadataWindow requires both a live
// original session and unchanged configuration; unrelated sessions revalidate.
func TestActivePlaybackKeepsValidURLBeyondMetadataWindow(t *testing.T) {
	p := newFakeProvider()
	p.searchResults = [][]p115.File{{p.targetFile}}
	s := newRoutedDirectPlayForTest(t, &fakeRoutedAccountRuntime{}, p, p115quota.NewMemoryLeaseStore())
	now := s.now()
	s.now = func() time.Time { return now }
	p.download.ExpiresAt = now.Add(time.Hour)
	request := routedMediaPathRequest("GET", "playing")
	first, err := s.ResolveMediaPath(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	event := PlaybackSessionEvent{UserID: request.UserID, MappingID: request.MappingID, DeviceID: request.DeviceID, PlaySessionID: request.PlaySessionID, IsProgress: true}
	if _, err = s.HandlePlaybackSessionEvent(context.Background(), event); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 12; i++ {
		now = now.Add(time.Minute)
		result, err := s.HandlePlaybackSessionEvent(context.Background(), event)
		if err != nil || !result.Found {
			t.Fatal("fixture lost active lease")
		}
	}
	calls := len(p.calls)
	p.resolveErr = p115.ErrSourceFileNotFound
	again, err := s.ResolveMediaPath(context.Background(), request)
	if err != nil || again.URL != first.URL || len(p.calls) != calls {
		t.Fatalf("active cache re-read provider: err=%v calls=%v", err, p.calls)
	}
	request.PlaySessionID = "new-playback"
	if _, err := s.ResolveMediaPath(context.Background(), request); err == nil {
		t.Fatal("another session reused stale media identity")
	}

	// A live lease must still respect the original signed URL safety boundary.
	request.PlaySessionID = "playing"
	for i := 12; i < 59; i++ {
		now = now.Add(time.Minute)
		result, err := s.HandlePlaybackSessionEvent(context.Background(), event)
		if err != nil || !result.Found {
			t.Fatal("fixture lost active lease")
		}
	}
	now = now.Add(50 * time.Second)
	p.resolveErr = nil
	p.searchResults = [][]p115.File{{p.targetFile}}
	p.download.ExpiresAt = now.Add(time.Hour)
	p.download.URL = "https://cdn.115.com/fresh-fixture"
	refreshed, err := s.ResolveMediaPath(context.Background(), request)
	if err != nil || refreshed.URL == first.URL || countDownloadCalls(p) != 2 {
		t.Fatalf("expired URL retained: %v", err)
	}
	if refreshed.Routing.AccountUsage.OccupiedStreams != 1 {
		t.Fatal("URL refresh duplicated active lease")
	}
}

// TestActiveMediaStillChecksAccountConfiguration protects reuse beyond ten
// minutes from recovery probes and credential/configuration changes.
func TestActiveMediaStillChecksAccountConfiguration(t *testing.T) {
	for _, kind := range []string{"config", "probe", "stopped"} {
		t.Run(kind, func(t *testing.T) {
			p := newFakeProvider()
			p.searchResults = [][]p115.File{{p.targetFile}}
			accounts := &fakeRoutedAccountRuntime{route: routedPlaybackFixture()}
			s := newRoutedDirectPlayForTest(t, accounts, p, p115quota.NewMemoryLeaseStore())
			now := s.now()
			s.now = func() time.Time { return now }
			p.download.ExpiresAt = now.Add(time.Hour)
			r := routedMediaPathRequest("GET", "playing")
			if _, err := s.ResolveMediaPath(context.Background(), r); err != nil {
				t.Fatal(err)
			}
			e := PlaybackSessionEvent{UserID: r.UserID, MappingID: r.MappingID, DeviceID: r.DeviceID, PlaySessionID: r.PlaySessionID, IsProgress: true}
			s.HandlePlaybackSessionEvent(context.Background(), e)
			for i := 0; i < 11; i++ {
				now = now.Add(time.Minute)
				s.HandlePlaybackSessionEvent(context.Background(), e)
			}
			switch kind {
			case "config":
				accounts.route.ConfigVersion++
			case "probe":
				s.accounts = &mediaAccountRuntime{fakeRoutedAccountRuntime: accounts, change: func(a *p115account.ActiveAccountCredential) { a.DownloadCacheVersion = 0 }}
			case "stopped":
				e.Stopped = true
				s.HandlePlaybackSessionEvent(context.Background(), e)
			}
			p.resolveErr = p115.ErrSourceFileNotFound
			if _, err := s.ResolveMediaPath(context.Background(), r); err == nil {
				t.Fatal("stale cache bypassed control or lifecycle boundary")
			}
		})
	}
}

// TestPausedPlaybackKeepsItsValidatedMedia covers a long pause without sliding
// either URL expiration or the independent paused lease lifetime.
func TestPausedPlaybackKeepsItsValidatedMedia(t *testing.T) {
	p := newFakeProvider()
	p.searchResults = [][]p115.File{{p.targetFile}}
	s := newRoutedDirectPlayForTest(t, &fakeRoutedAccountRuntime{}, p, p115quota.NewMemoryLeaseStore())
	now := s.now()
	s.now = func() time.Time { return now }
	p.download.ExpiresAt = now.Add(time.Hour)
	r := routedMediaPathRequest("GET", "paused")
	if _, err := s.ResolveMediaPath(context.Background(), r); err != nil {
		t.Fatal(err)
	}
	e := PlaybackSessionEvent{UserID: r.UserID, MappingID: r.MappingID, DeviceID: r.DeviceID, PlaySessionID: r.PlaySessionID, IsProgress: true, IsPaused: true}
	if result, err := s.HandlePlaybackSessionEvent(context.Background(), e); err != nil || !result.Found {
		t.Fatal("fixture did not pause")
	}
	now = now.Add(11 * time.Minute)
	p.resolveErr = p115.ErrSourceFileNotFound
	if _, err := s.ResolveMediaPath(context.Background(), r); err != nil {
		t.Fatalf("valid paused session lost cache: %v", err)
	}
	now = now.Add(4 * time.Minute)
	if _, err := s.ResolveMediaPath(context.Background(), r); err == nil {
		t.Fatal("video request renewed expired paused activity")
	}
}
