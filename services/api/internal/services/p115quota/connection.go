package p115quota

import (
	"io"
	"strings"
	"time"

	redis "github.com/redis/go-redis/v9"
)

const redisCommandTimeout = 500 * time.Millisecond

// NewLeaseStoreFromURL creates a standalone Redis adapter without connecting,
// pinging, or probing a server version. Missing/invalid values fail closed via
// UnavailableLeaseStore and are never included in returned errors.
func NewLeaseStoreFromURL(rawURL string) (LeaseStore, io.Closer) {
	if rawURL == "" || strings.TrimSpace(rawURL) != rawURL {
		return UnavailableLeaseStore{}, nil
	}
	options, err := redis.ParseURL(rawURL)
	if err != nil {
		return UnavailableLeaseStore{}, nil
	}
	options.DialTimeout = redisCommandTimeout
	options.ReadTimeout = redisCommandTimeout
	options.WriteTimeout = redisCommandTimeout
	options.MaxRetries = -1
	client := redis.NewClient(options)
	store, err := NewRedisLeaseStore(client)
	if err != nil {
		_ = client.Close()
		return UnavailableLeaseStore{}, nil
	}
	return store, client
}
