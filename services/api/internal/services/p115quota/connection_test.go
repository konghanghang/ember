package p115quota

import (
	"context"
	"testing"
	"time"

	miniredis "github.com/alicebob/miniredis/v2"
	"github.com/alicebob/miniredis/v2/server"
	redis "github.com/redis/go-redis/v9"
)

func TestNewLeaseStoreFromURLBuildsFastFailingClientWithoutVersionProbe(t *testing.T) {
	store, closer := NewLeaseStoreFromURL("redis://127.0.0.1:6379/2")
	if closer == nil {
		t.Fatal("NewLeaseStoreFromURL() closer = nil")
	}
	t.Cleanup(func() { _ = closer.Close() })

	redisStore, ok := store.(*RedisLeaseStore)
	if !ok {
		t.Fatalf("NewLeaseStoreFromURL() store type = %T", store)
	}
	client, ok := redisStore.client.(*redis.Client)
	if !ok {
		t.Fatalf("RedisLeaseStore client type = %T", redisStore.client)
	}
	options := client.Options()
	if options.DB != 2 || options.DialTimeout != 500*time.Millisecond || options.ReadTimeout != 500*time.Millisecond ||
		options.WriteTimeout != 500*time.Millisecond || options.MaxRetries != 0 {
		t.Fatalf("redis options = DB %d dial %s read %s write %s retries %d", options.DB, options.DialTimeout, options.ReadTimeout, options.WriteTimeout, options.MaxRetries)
	}
}

// TestProductionTransferCommitHonorsDeadline exercises blocked Redis I/O with
// production client options, rather than an immediately failing store fake.
func TestProductionTransferCommitHonorsDeadline(t *testing.T) {
	fake := miniredis.RunT(t)
	store, closer := NewLeaseStoreFromURL("redis://" + fake.Addr())
	t.Cleanup(func() { _ = closer.Close() })
	client := store.(*RedisLeaseStore).client.(*redis.Client)
	if err := client.Ping(context.Background()).Err(); err != nil {
		t.Fatal(err)
	}
	unblock := make(chan struct{})
	defer close(unblock)
	fake.Server().SetPreHook(func(_ *server.Peer, cmd string, _ ...string) bool {
		if cmd == "EVALSHA" || cmd == "EVAL" {
			<-unblock
		}
		return false
	})
	now := time.Now()
	start, end := DayWindow(now, time.UTC)
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Millisecond)
	defer cancel()
	began := time.Now()
	_, err := store.(TransferQuotaStore).CommitTransfer(ctx, TransferCommitRequest{
		UserID: "user-1", AttemptID: "slow-commit", DayStart: start, DayEnd: end,
	}, now)
	if err == nil {
		t.Fatal("blocked commit unexpectedly succeeded")
	}
	if elapsed := time.Since(began); elapsed >= 250*time.Millisecond {
		t.Fatalf("commit ignored remaining deadline: elapsed=%s", elapsed)
	}
}

func TestNewLeaseStoreFromURLFailsClosedForMissingOrInvalidConfiguration(t *testing.T) {
	for _, rawURL := range []string{"", " redis://127.0.0.1:6379/0", "https://redis.invalid"} {
		store, closer := NewLeaseStoreFromURL(rawURL)
		if closer != nil {
			t.Fatalf("NewLeaseStoreFromURL(%q) closer = %T", rawURL, closer)
		}
		if _, ok := store.(UnavailableLeaseStore); !ok {
			t.Fatalf("NewLeaseStoreFromURL(%q) store type = %T", rawURL, store)
		}
	}
}
