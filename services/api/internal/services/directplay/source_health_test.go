package directplay

import (
	"context"
	"errors"
	"testing"
	"time"

	p115 "github.com/konghang/ember/backend/internal/integrations/p115"
	"github.com/konghang/ember/backend/internal/services/p115account"
)

type failingSourceRangeProvider struct {
	*fakeProvider
	failAt       int
	invalid      bool
	callsToRange int
}

// HashFileRange fails only the selected source read, after path resolution
// succeeded, to test that subsequent failure wins over the earlier observation.
func (p *failingSourceRangeProvider) HashFileRange(ctx context.Context, credential p115.Credential, request p115.FileRangeRequest) (p115.FileRangeHash, error) {
	p.callsToRange++
	if p.callsToRange == p.failAt {
		if p.invalid {
			return p115.FileRangeHash{}, nil
		}
		return p115.FileRangeHash{}, p115.ErrProviderUnavailable
	}
	return p.fakeProvider.HashFileRange(ctx, credential, request)
}

type failingTerminalStore struct{ fakeTaskStore }

// MarkFailed simulates loss of task persistence after a real source failure.
func (*failingTerminalStore) MarkFailed(context.Context, string, string, string, time.Time) error {
	return errors.New("fixture store failure")
}

// TestSourceRecoveryDoesNotOverwriteLaterSourceFailure keeps real transport
// failures authoritative while isolating invalid file hashes, even if task persistence fails.
func TestSourceRecoveryDoesNotOverwriteLaterSourceFailure(t *testing.T) {
	for _, stage := range []int{1, 2} {
		for _, invalid := range []bool{false, true} {
			for _, failStore := range []bool{false, true} {
				health := &fakeAccountHealthReporter{}
				provider := &failingSourceRangeProvider{fakeProvider: newFakeProvider(), failAt: stage, invalid: invalid}
				if stage == 2 {
					provider.uploadResults = []p115.RapidUploadResult{{Status: p115.RapidUploadRangeChallenge, Challenge: &p115.RapidUploadChallenge{SignKey: "fixture-key", Range: p115.ByteRange{Start: 0, End: 99}}}}
				}
				service := newServiceWithDependencies(fakeAccountLoader{health: health}, provider, &fakeTaskStore{}, &fakeTaskLocker{})
				if failStore {
					service.store = &failingTerminalStore{}
				}
				_, err := service.Resolve(context.Background(), fixtureResolveRequest())
				want := p115account.RuntimeHealthProviderUnavailable
				if invalid {
					want = p115account.RuntimeHealthSucceeded
				}
				if err == nil || len(health.events) != 1 || health.events[0].outcome != want {
					t.Fatalf("stage=%d invalid=%t storeFailure=%t error=%v events=%+v", stage, invalid, failStore, err, health.events)
				}
			}
		}
	}
}

// TestSourceRangeHealthClassification rejects account-wide conclusions from
// file/CDN protocol errors but preserves explicit credential and transport failures.
func TestSourceRangeHealthClassification(t *testing.T) {
	for _, operation := range []string{failureOperationHashSourcePreID, failureOperationHashSourceChallenge} {
		for _, failure := range []error{p115.ErrProviderProtocol, p115.ErrProviderRejected, ErrSourceReadTimeout, context.Canceled} {
			if _, ok := runtimeHealthOutcome(operation, failure); ok {
				t.Fatalf("file error poisoned account: %s %v", operation, failure)
			}
		}
		for _, failure := range []error{p115.ErrCredentialRejected, p115.ErrProviderUnavailable} {
			if _, ok := runtimeHealthOutcome(operation, failure); !ok {
				t.Fatalf("account failure ignored: %s %v", operation, failure)
			}
		}
	}
}
