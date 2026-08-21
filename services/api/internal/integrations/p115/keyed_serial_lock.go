package p115

import (
	"context"
	"sync"
)

var sharedCookieDeleteLocks = newKeyedSerialLocks()

type keyedSerialLocks struct {
	mu      sync.Mutex
	entries map[string]*keyedSerialLockEntry
}

type keyedSerialLockEntry struct {
	token chan struct{}
	refs  int
}

func newKeyedSerialLocks() *keyedSerialLocks {
	return &keyedSerialLocks{entries: make(map[string]*keyedSerialLockEntry)}
}

// acquire waits for one key without blocking unrelated keys and supports context cancellation.
func (locks *keyedSerialLocks) acquire(ctx context.Context, key string) (func(), error) {
	locks.mu.Lock()
	entry := locks.entries[key]
	if entry == nil {
		entry = &keyedSerialLockEntry{token: make(chan struct{}, 1)}
		entry.token <- struct{}{}
		locks.entries[key] = entry
	}
	entry.refs++
	locks.mu.Unlock()

	select {
	case <-ctx.Done():
		locks.releaseReference(key, entry)
		return nil, ctx.Err()
	case <-entry.token:
	}

	var once sync.Once
	return func() {
		once.Do(func() {
			entry.token <- struct{}{}
			locks.releaseReference(key, entry)
		})
	}, nil
}

func (locks *keyedSerialLocks) releaseReference(key string, entry *keyedSerialLockEntry) {
	locks.mu.Lock()
	defer locks.mu.Unlock()
	entry.refs--
	if entry.refs == 0 && locks.entries[key] == entry {
		delete(locks.entries, key)
	}
}
