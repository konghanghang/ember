package directplay

import (
	"context"
	"errors"
	"testing"
	"time"

	p115 "github.com/konghang/ember/backend/internal/integrations/p115"
	"github.com/konghang/ember/backend/internal/services/p115account"
)

type delayedSourceRange struct{ *fakeProvider }

// HashFileRange simulates a stalled read-only source fetch, never a retained write.
func (*delayedSourceRange) HashFileRange(ctx context.Context, _ p115.Credential, _ p115.FileRangeRequest) (p115.FileRangeHash, error) {
	<-ctx.Done()
	return p115.FileRangeHash{}, ctx.Err()
}

// TestSourceReadBudgetFallsBackWithoutAccountFailure covers both source stages
// and caller cancellation, with no actual network or production-duration wait.
func TestSourceReadBudgetFallsBackWithoutAccountFailure(t *testing.T) {
	for _, rangeRead := range []bool{false, true} {
		health := &fakeAccountHealthReporter{}
		provider := newFakeProvider()
		service := newServiceWithDependencies(fakeAccountLoader{health: health}, provider, &fakeTaskStore{}, &fakeTaskLocker{})
		service.sourceReadBudget = time.Millisecond
		if rangeRead {
			service.provider = &delayedSourceRange{provider}
		} else {
			service.provider = &lifecycleProvider{fakeProvider: provider, resolve: func(ctx context.Context) error { <-ctx.Done(); return ctx.Err() }}
		}
		_, err := service.Resolve(context.Background(), fixtureResolveRequest())
		if !errors.Is(err, ErrSourceReadTimeout) {
			t.Fatalf("range=%t error=%v", rangeRead, err)
		}
		if rangeRead {
			if len(health.events) != 1 || health.events[0].outcome != p115account.RuntimeHealthSucceeded {
				t.Fatalf("source observation lost: %+v", health.events)
			}
		} else if len(health.events) != 0 {
			t.Fatal("local budget marked account unhealthy")
		}
		if len(provider.uploadRequests) != 0 {
			t.Fatal("timed out read started retained upload")
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if !errors.Is(sourceReadResult(ctx, ctx, nil), context.Canceled) {
		t.Fatal("caller cancellation lost")
	}
}
