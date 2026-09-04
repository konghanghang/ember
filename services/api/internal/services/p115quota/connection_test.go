package p115quota

import (
	"testing"
	"time"

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
