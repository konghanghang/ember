package directplay

import (
	"context"
	"sync"
)

// requestGate serializes overlapping work for an opaque key without keeping
// completed entries or allowing canceled waiters to consume a resource slot.
type requestGate struct {
	mu      sync.Mutex
	entries map[string]*requestGateEntry
}

type requestGateEntry struct {
	permit chan struct{}
	refs   int
}

// acquire returns a release function only after this caller owns the key.
func (gate *requestGate) acquire(ctx context.Context, key string) (func(), error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	gate.mu.Lock()
	if gate.entries == nil {
		gate.entries = make(map[string]*requestGateEntry)
	}
	entry := gate.entries[key]
	if entry == nil {
		entry = &requestGateEntry{permit: make(chan struct{}, 1)}
		entry.permit <- struct{}{}
		gate.entries[key] = entry
	}
	entry.refs++
	gate.mu.Unlock()
	select {
	case <-ctx.Done():
		gate.forget(key, entry)
		return nil, ctx.Err()
	case <-entry.permit:
	}
	var once sync.Once
	release := func() { once.Do(func() { entry.permit <- struct{}{}; gate.forget(key, entry) }) }
	if err := ctx.Err(); err != nil {
		release()
		return nil, err
	}
	return release, nil
}

// forget drops idle keys after both owners and canceled waiters leave.
func (gate *requestGate) forget(key string, entry *requestGateEntry) {
	gate.mu.Lock()
	defer gate.mu.Unlock()
	entry.refs--
	if entry.refs == 0 {
		delete(gate.entries, key)
	}
}
