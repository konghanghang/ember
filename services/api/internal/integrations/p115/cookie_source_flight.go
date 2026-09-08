package p115

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/konghang/ember/backend/internal/logging"
)

const sourceDirectoryResolveTimeout = 30 * time.Second

type sourceReadTimingKey struct{}
type sourceReadTiming struct {
	parent  time.Duration
	listing time.Duration
	pages   int
}

type sourceDirectoryFlight struct {
	done    chan struct{}
	cancel  context.CancelFunc
	waiters int
	joined  int
	entries []File // Immutable after done closes; callers return individual File copies.
	err     error
}

type sourceDirectoryFlights struct {
	mu      sync.Mutex
	calls   map[[sha256.Size]byte]*sourceDirectoryFlight
	timeout time.Duration // Test seam; production uses the fixed total budget.
}

// sourceDirectoryKey isolates accounts, exact credentials and root-relative
// directories without retaining Cookie text in the flight map or logging keys.
func sourceDirectoryKey(credential Credential, rootID, parentPath string) [sha256.Size]byte {
	hash := sha256.New()
	for _, value := range []string{credential.AccountID, credential.Cookie, credential.AppType, credential.UserAgent, rootID, parentPath} {
		_, _ = fmt.Fprintf(hash, "%d:%s", len(value), value)
	}
	var key [sha256.Size]byte
	copy(key[:], hash.Sum(nil))
	return key
}

// do shares only unfinished reads. Each waiter owns its cancellation; the last
// departing waiter removes and cancels the call so later requests can retry.
func (group *sourceDirectoryFlights) do(ctx context.Context, key [sha256.Size]byte, resolve func(context.Context) ([]File, error)) ([]File, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	group.mu.Lock()
	call := group.calls[key]
	if call == nil {
		if group.calls == nil {
			group.calls = make(map[[sha256.Size]byte]*sourceDirectoryFlight)
		}
		budget := group.timeout
		if budget <= 0 {
			budget = sourceDirectoryResolveTimeout
		}
		sharedCtx, cancel := context.WithTimeout(context.Background(), budget)
		call = &sourceDirectoryFlight{done: make(chan struct{}), cancel: cancel}
		group.calls[key] = call
		go group.run(sharedCtx, key, call, resolve)
	}
	call.waiters++
	call.joined++
	group.mu.Unlock()
	defer func() {
		group.mu.Lock()
		defer group.mu.Unlock()
		call.waiters--
		if call.waiters == 0 {
			if group.calls[key] == call {
				delete(group.calls, key)
			}
			call.cancel()
		}
	}()
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-call.done:
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		return call.entries, call.err
	}
}

// run publishes only complete snapshots and always removes the flight before
// waking waiters. A canceled older call cannot remove a replacement flight.
func (group *sourceDirectoryFlights) run(ctx context.Context, key [sha256.Size]byte, call *sourceDirectoryFlight, resolve func(context.Context) ([]File, error)) {
	defer call.cancel()
	started := time.Now()
	readTiming := &sourceReadTiming{}
	ctx = context.WithValue(ctx, sourceReadTimingKey{}, readTiming)
	entries, err := resolve(ctx)
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		// The shared Provider budget is not an individual client's deadline:
		// preserve ordinary Provider-unavailable fallback and health handling.
		err = ErrProviderUnavailable
	} else if ctx.Err() != nil {
		err = ctx.Err()
	}
	if err != nil {
		entries = nil
	}
	group.mu.Lock()
	call.entries, call.err = entries, err
	if group.calls[key] == call {
		delete(group.calls, key)
	}
	joined := call.joined
	close(call.done)
	group.mu.Unlock()
	logging.Debugf("[P115] code=source_directory_resolved shared=%t callers=%d entries=%d parentResolveMs=%d listMs=%d listPages=%d durationMs=%d success=%t",
		joined > 1, joined, len(entries), readTiming.parent.Milliseconds(), readTiming.listing.Milliseconds(), readTiming.pages,
		time.Since(started).Milliseconds(), err == nil)
}
