package directplay

import (
	"context"
	"strings"
	"testing"

	p115 "github.com/konghang/ember/backend/internal/integrations/p115"
	"github.com/konghang/ember/backend/internal/services/p115quota"
)

// TestStepDiagnosticsPrecedeProviderAndSkipCacheWork covers successful and
// failing calls, while rejecting arbitrary diagnostic names or values.
func TestStepDiagnosticsPrecedeProviderAndSkipCacheWork(t *testing.T) {
	for _, fail := range []bool{false, true} {
		p := newFakeProvider()
		p.searchResults = [][]p115.File{{p.targetFile}}
		s := newRoutedDirectPlayForTest(t, &fakeRoutedAccountRuntime{}, p, p115quota.NewMemoryLeaseStore())
		var events []string
		ctx := WithStepObserver(context.Background(), func(step, phase string, ms int64) {
			if ms < 0 {
				t.Fatal("negative elapsed time")
			}
			events = append(events, step+":"+phase)
		})
		s.provider = &lifecycleProvider{fakeProvider: p, resolve: func(context.Context) error {
			if len(events) == 0 || events[len(events)-1] != "sourceResolve:started" {
				t.Fatal("Provider started before diagnostic")
			}
			if fail {
				return p115.ErrProviderUnavailable
			}
			return nil
		}}
		_, err := s.ResolveMediaPath(ctx, routedMediaPathRequest("GET", "first"))
		if (err != nil) != fail {
			t.Fatalf("err=%v", err)
		}
		joined := strings.Join(events, " ")
		for _, event := range []string{"sessionWait:started", "sessionWait:finished", "leaseAdmission:created", "sourceResolve:finished"} {
			if !strings.Contains(joined, event) {
				t.Fatalf("missing %s", event)
			}
		}
		if fail {
			continue
		}
		events = nil
		if _, err := s.ResolveMediaPath(ctx, routedMediaPathRequest("GET", "second")); err != nil {
			t.Fatal(err)
		}
		joined = strings.Join(events, " ")
		if !strings.Contains(joined, "mediaResolutionCache:hit") || !strings.Contains(joined, "leaseConfirm:finished") || strings.Contains(joined, "sourceResolve:") || strings.Contains(joined, "downloadURL:") {
			t.Fatalf("cache hit diagnostics=%s", joined)
		}
		before := len(events)
		observeStep(ctx, "secret\ninjected=true")()
		observeCache(ctx, "mediaResolutionCache", "secret")
		if len(events) != before {
			t.Fatal("unknown value emitted")
		}
	}
}
