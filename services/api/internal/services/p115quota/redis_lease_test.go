package p115quota

import (
	"context"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	miniredis "github.com/alicebob/miniredis/v2"
	redis "github.com/redis/go-redis/v9"
)

func newTestRedisLeaseStore(t *testing.T) (*RedisLeaseStore, *miniredis.Miniredis, *redis.Client) {
	t.Helper()
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	store, err := NewRedisLeaseStore(client)
	if err != nil {
		t.Fatalf("NewRedisLeaseStore() error = %v", err)
	}
	return store, server, client
}

func TestRedisLeaseStoreMaintainsOpaqueIndexesAndTTLs(t *testing.T) {
	store, server, _ := newTestRedisLeaseStore(t)
	now := time.Date(2026, 9, 4, 0, 0, 0, 0, time.UTC)
	request := ReserveRequest{
		PlaybackAccountKey:   strings.Repeat("a", 64),
		UserID:               "user-1",
		SessionFingerprint:   strings.Repeat("b", 64),
		MaxConcurrentStreams: 2,
	}
	result, err := store.Reserve(context.Background(), request, now)
	if err != nil || !result.Created || result.Account.ReservedStreams != 1 || result.User.OccupiedStreams != 1 {
		t.Fatalf("Reserve() = %+v, %v", result, err)
	}

	wantScore := float64(now.Add(ReservationTTL).UnixMilli())
	for _, key := range []string{accountLeasesKey(request.PlaybackAccountKey), userLeasesKey(request.UserID)} {
		score, err := server.ZScore(key, request.SessionFingerprint)
		if err != nil || score != wantScore {
			t.Fatalf("ZScore(%s) = %f, %v", key, score, err)
		}
		if ttl := server.TTL(key); ttl != LeaseIndexTTL {
			t.Fatalf("TTL(%s) = %v", key, ttl)
		}
	}
	reverseKey := sessionKey(request.SessionFingerprint)
	if ttl := server.TTL(reverseKey); ttl != ReservationTTL {
		t.Fatalf("session TTL = %v", ttl)
	}
	value, err := server.Get(reverseKey)
	if err != nil || strings.Contains(value, "provider") || !strings.Contains(value, request.PlaybackAccountKey) {
		t.Fatalf("session value = %q, %v", value, err)
	}
}

func TestRedisLeaseStoreHandlesScriptFlushAndStateTransitions(t *testing.T) {
	store, server, client := newTestRedisLeaseStore(t)
	now := time.Date(2026, 9, 4, 0, 0, 0, 0, time.UTC)
	request := ReserveRequest{
		PlaybackAccountKey:   strings.Repeat("c", 64),
		UserID:               "user-2",
		SessionFingerprint:   strings.Repeat("d", 64),
		MaxConcurrentStreams: 1,
	}
	if _, err := store.Reserve(context.Background(), request, now); err != nil {
		t.Fatalf("Reserve() error = %v", err)
	}
	if err := client.ScriptFlush(context.Background()).Err(); err != nil {
		t.Fatalf("ScriptFlush() error = %v", err)
	}

	active, err := store.Advance(context.Background(), request.SessionFingerprint, LeaseStateActive, now.Add(time.Second))
	if err != nil || !active.Found || active.PreviousState != LeaseStateReservation || active.Account.ActiveStreams != 1 {
		t.Fatalf("Advance(active) = %+v, %v", active, err)
	}
	if ttl := server.TTL(sessionKey(request.SessionFingerprint)); ttl != ActiveTTL {
		t.Fatalf("active session TTL = %v", ttl)
	}
	paused, err := store.Advance(context.Background(), request.SessionFingerprint, LeaseStatePaused, now.Add(2*time.Second))
	if err != nil || paused.State != LeaseStatePaused || paused.User.ActiveStreams != 1 {
		t.Fatalf("Advance(paused) = %+v, %v", paused, err)
	}
	if ttl := server.TTL(sessionKey(request.SessionFingerprint)); ttl != PausedTTL {
		t.Fatalf("paused session TTL = %v", ttl)
	}
	stopped, err := store.Stop(context.Background(), request.SessionFingerprint, now.Add(3*time.Second))
	if err != nil || !stopped.Found || stopped.Account.OccupiedStreams != 0 || stopped.User.ActiveStreams != 0 {
		t.Fatalf("Stop() = %+v, %v", stopped, err)
	}
	if server.Exists(sessionKey(request.SessionFingerprint)) {
		t.Fatal("stopped reverse session still exists")
	}
}

func TestRedisLeaseStoreAdmissionAndScoreExpiryAreAtomic(t *testing.T) {
	store, _, _ := newTestRedisLeaseStore(t)
	now := time.Date(2026, 9, 4, 0, 0, 0, 0, time.UTC)
	accountKey := strings.Repeat("e", 64)

	var admitted atomic.Int32
	var wait sync.WaitGroup
	for index := 0; index < 8; index++ {
		wait.Add(1)
		go func(index int) {
			defer wait.Done()
			_, err := store.Reserve(context.Background(), ReserveRequest{
				PlaybackAccountKey: accountKey, UserID: "user-3",
				SessionFingerprint:   strings.Repeat(string(rune('1'+index)), 64),
				MaxConcurrentStreams: 2,
			}, now)
			if err == nil {
				admitted.Add(1)
				return
			}
			if !errors.Is(err, ErrAccountConcurrencyExceeded) {
				t.Errorf("Reserve() error = %v", err)
			}
		}(index)
	}
	wait.Wait()
	if admitted.Load() != 2 {
		t.Fatalf("admitted = %d, want 2", admitted.Load())
	}

	usage, err := store.AccountUsage(context.Background(), accountKey, now.Add(ReservationTTL+time.Millisecond))
	if err != nil || usage != (LeaseUsage{}) {
		t.Fatalf("expired AccountUsage() = %+v, %v", usage, err)
	}
}

func TestRedisLeaseStoreLimitResultIncludesAtomicUsage(t *testing.T) {
	store, _, _ := newTestRedisLeaseStore(t)
	now := time.Date(2026, 9, 4, 0, 0, 0, 0, time.UTC)
	request := ReserveRequest{
		PlaybackAccountKey:   strings.Repeat("a", 64),
		UserID:               "user-limit",
		SessionFingerprint:   strings.Repeat("b", 64),
		MaxConcurrentStreams: 1,
	}
	if _, err := store.Reserve(context.Background(), request, now); err != nil {
		t.Fatalf("Reserve(first) error = %v", err)
	}

	request.SessionFingerprint = strings.Repeat("c", 64)
	result, err := store.Reserve(context.Background(), request, now)
	if !errors.Is(err, ErrAccountConcurrencyExceeded) {
		t.Fatalf("Reserve(limit) error = %v", err)
	}
	if result.Account != (LeaseUsage{ReservedStreams: 1, OccupiedStreams: 1}) ||
		result.User != (LeaseUsage{ReservedStreams: 1, OccupiedStreams: 1}) {
		t.Fatalf("Reserve(limit) result = %+v", result)
	}
}

func TestRedisLeaseStoreMissingAndCanceledOperations(t *testing.T) {
	store, _, _ := newTestRedisLeaseStore(t)
	now := time.Now().UTC()
	usage, err := store.UserUsage(context.Background(), "user-4", now)
	if err != nil || usage != (LeaseUsage{}) {
		t.Fatalf("missing UserUsage() = %+v, %v", usage, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := store.AccountUsage(ctx, strings.Repeat("a", 64), now); !errors.Is(err, context.Canceled) {
		t.Fatalf("AccountUsage(canceled) error = %v", err)
	}
}

func TestRedisLeaseStoreFallsBackDuringDisconnectAndRestartsFromCurrentData(t *testing.T) {
	store, server, _ := newTestRedisLeaseStore(t)
	now := time.Date(2026, 9, 4, 0, 0, 0, 0, time.UTC)
	accountKey := strings.Repeat("f", 64)
	request := ReserveRequest{
		PlaybackAccountKey: accountKey, UserID: "user-reconnect",
		SessionFingerprint: strings.Repeat("1", 64), MaxConcurrentStreams: 1,
	}
	if _, err := store.Reserve(context.Background(), request, now); err != nil {
		t.Fatalf("Reserve() error = %v", err)
	}

	server.Close()
	if _, err := store.AccountUsage(context.Background(), accountKey, now); !errors.Is(err, ErrRedisUnavailable) {
		t.Fatalf("AccountUsage(disconnected) error = %v", err)
	}
	if err := server.Restart(); err != nil {
		t.Fatalf("Restart() error = %v", err)
	}
	server.FlushAll()
	usage, err := store.AccountUsage(context.Background(), accountKey, now)
	if err != nil || usage != (LeaseUsage{}) {
		t.Fatalf("AccountUsage(after data loss) = %+v, %v", usage, err)
	}
}
