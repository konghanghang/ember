package directplay

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	p115integration "github.com/konghang/ember/backend/internal/integrations/p115"
)

// TestResolveTimingIncludesSuccessAndFailedStage protects timing propagation
// on both the reused path and early Provider failure without changing outcomes.
func TestResolveTimingIncludesSuccessAndFailedStage(t *testing.T) {
	for _, fail := range []bool{false, true} {
		provider := newFakeProvider()
		provider.searchResults = [][]p115integration.File{{provider.targetFile}}
		if fail {
			provider.resolveErr = p115integration.ErrProviderUnavailable
		}
		service := newServiceWithDependencies(fakeAccountLoader{}, provider, &fakeTaskStore{}, &fakeTaskLocker{})
		result, err := service.ResolveMediaPath(context.Background(), MediaPathResolveRequest{
			Path: "/mnt/cloudNAS/115lifetime/Media/fixture.mkv", ClientUserAgent: "Infuse-Fixture",
		})
		if fail != (err != nil) {
			t.Fatalf("failure=%t err=%v", fail, err)
		}
		fields := strings.Join(result.Timing.LogFields(), " ")
		for _, want := range []string{"directPlayMs=", "prepareCalls=1", "sourceResolveCalls=1", "otherMs="} {
			if !strings.Contains(fields, want) {
				t.Fatalf("timing=%s missing %s", fields, want)
			}
		}
		if fail {
			if !errors.Is(err, ErrProviderUnavailable) || strings.Contains(fields, "downloadURLCalls=") {
				t.Fatalf("failed path timing=%s err=%v", fields, err)
			}
		} else if !strings.Contains(fields, "targetSearchCalls=1") || !strings.Contains(fields, "downloadURLCalls=1") {
			t.Fatalf("reused path timing=%s", fields)
		}
	}
}

// TestTimingAggregationUsesElapsedDurations checks exact arithmetic without
// sleeps, unknown-stage filtering and omission of work that never ran.
func TestTimingAggregationUsesElapsedDurations(t *testing.T) {
	ctx, recorder := withTiming(context.Background())
	now := recorder.started
	recorder.now = func() time.Time { return now }
	now = now.Add(3 * time.Millisecond)
	finishPreparation(ctx)
	for range 2 {
		finish := measureStage(ctx, "targetSearch")
		now = now.Add(7 * time.Millisecond)
		finish()
	}
	now = now.Add(2 * time.Millisecond)
	timing := recorder.finish()
	timing.stages["cookie-secret\nforged=true"] = stageTiming{calls: 1}
	got := strings.Join(timing.LogFields(), " ")
	want := "directPlayMs=19 prepareMs=3 prepareCalls=1 targetSearchMs=14 targetSearchCalls=2 otherMs=2"
	if got != want {
		t.Fatalf("timing=%q want=%q", got, want)
	}
	if len((TimingDiagnostics{}).LogFields()) != 0 {
		t.Fatal("unmeasured request emitted timing")
	}
}
