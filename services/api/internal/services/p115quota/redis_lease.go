package p115quota

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	redis "github.com/redis/go-redis/v9"
)

var ErrRedisUnavailable = errors.New("Redis 配额服务不可用")

const redisLeasePrefix = "{p115}"

var reserveLeaseScript = redis.NewScript(`
local now = tonumber(ARGV[4])
for index = 1, 4 do
  redis.call('ZREMRANGEBYSCORE', KEYS[index], '-inf', now)
end

local function counts(leasesKey, activeKey)
  local occupied = tonumber(redis.call('ZCOUNT', leasesKey, '(' .. now, '+inf'))
  local active = tonumber(redis.call('ZCOUNT', activeKey, '(' .. now, '+inf'))
  local reserved = occupied - active
  if reserved < 0 then reserved = 0 end
  return reserved, active, occupied
end

local existing = redis.call('GET', KEYS[5])
if existing then
  local accountKey, userId, state, expiresAt = string.match(existing, '^([^|]+)|([^|]+)|([^|]+)|([0-9]+)$')
  if not accountKey or not userId or not state or not expiresAt then
    return redis.error_reply('invalid p115 session value')
  end
  if tonumber(expiresAt) > now then
    if accountKey ~= ARGV[1] or userId ~= ARGV[2] then
      return {2, accountKey, userId, state}
    end
    local accountReserved, accountActive, accountOccupied = counts(KEYS[1], KEYS[3])
    local userReserved, userActive, userOccupied = counts(KEYS[2], KEYS[4])
    return {0, state, accountReserved, accountActive, accountOccupied, userReserved, userActive, userOccupied}
  end
  redis.call('DEL', KEYS[5])
end

local accountReserved, accountActive, accountOccupied = counts(KEYS[1], KEYS[3])
if accountOccupied >= tonumber(ARGV[5]) then
  local userReserved, userActive, userOccupied = counts(KEYS[2], KEYS[4])
  return {-1, '', accountReserved, accountActive, accountOccupied, userReserved, userActive, userOccupied}
end

redis.call('ZADD', KEYS[1], tonumber(ARGV[6]), ARGV[3])
redis.call('ZADD', KEYS[2], tonumber(ARGV[6]), ARGV[3])
for index = 1, 4 do
  redis.call('PEXPIRE', KEYS[index], tonumber(ARGV[8]))
end
redis.call('SET', KEYS[5], ARGV[9], 'PX', tonumber(ARGV[7]))

accountReserved, accountActive, accountOccupied = counts(KEYS[1], KEYS[3])
local userReserved, userActive, userOccupied = counts(KEYS[2], KEYS[4])
return {1, 'reservation', accountReserved, accountActive, accountOccupied, userReserved, userActive, userOccupied}
`)

var transitionLeaseScript = redis.NewScript(`
local now = tonumber(ARGV[3])
for index = 1, 4 do
  redis.call('ZREMRANGEBYSCORE', KEYS[index], '-inf', now)
end
local existing = redis.call('GET', KEYS[5])
if not existing or existing ~= ARGV[1] then
  return {0}
end
local accountKey, userId, previousState, previousExpiresAt = string.match(existing, '^([^|]+)|([^|]+)|([^|]+)|([0-9]+)$')
if not previousExpiresAt then
  return redis.error_reply('invalid p115 session value')
end
if tonumber(previousExpiresAt) <= now then
  for index = 1, 4 do redis.call('ZREM', KEYS[index], ARGV[2]) end
  redis.call('DEL', KEYS[5])
  return {0}
end

for index = 1, 4 do
  redis.call('ZADD', KEYS[index], tonumber(ARGV[4]), ARGV[2])
  redis.call('PEXPIRE', KEYS[index], tonumber(ARGV[6]))
end
redis.call('SET', KEYS[5], ARGV[7], 'PX', tonumber(ARGV[5]))

local accountOccupied = tonumber(redis.call('ZCOUNT', KEYS[1], '(' .. now, '+inf'))
local accountActive = tonumber(redis.call('ZCOUNT', KEYS[3], '(' .. now, '+inf'))
local userOccupied = tonumber(redis.call('ZCOUNT', KEYS[2], '(' .. now, '+inf'))
local userActive = tonumber(redis.call('ZCOUNT', KEYS[4], '(' .. now, '+inf'))
return {1, previousState, ARGV[8], accountOccupied - accountActive, accountActive, accountOccupied, userOccupied - userActive, userActive, userOccupied}
`)

var releaseReservationScript = redis.NewScript(`
local now = tonumber(ARGV[3])
for index = 1, 4 do
  redis.call('ZREMRANGEBYSCORE', KEYS[index], '-inf', now)
end
local existing = redis.call('GET', KEYS[5])
if not existing or existing ~= ARGV[1] then return 0 end
local accountKey, userId, state, expiresAt = string.match(existing, '^([^|]+)|([^|]+)|([^|]+)|([0-9]+)$')
if not state or not expiresAt then return redis.error_reply('invalid p115 session value') end
if state ~= 'reservation' or tonumber(expiresAt) <= now then return 0 end
for index = 1, 4 do redis.call('ZREM', KEYS[index], ARGV[2]) end
redis.call('DEL', KEYS[5])
return 1
`)

var stopLeaseScript = redis.NewScript(`
local now = tonumber(ARGV[3])
for index = 1, 4 do
  redis.call('ZREMRANGEBYSCORE', KEYS[index], '-inf', now)
end
local existing = redis.call('GET', KEYS[5])
if not existing or existing ~= ARGV[1] then return {0} end
local accountKey, userId, state, expiresAt = string.match(existing, '^([^|]+)|([^|]+)|([^|]+)|([0-9]+)$')
if not state or not expiresAt then return redis.error_reply('invalid p115 session value') end
for index = 1, 4 do redis.call('ZREM', KEYS[index], ARGV[2]) end
redis.call('DEL', KEYS[5])
if tonumber(expiresAt) <= now then return {0} end
local accountOccupied = tonumber(redis.call('ZCOUNT', KEYS[1], '(' .. now, '+inf'))
local accountActive = tonumber(redis.call('ZCOUNT', KEYS[3], '(' .. now, '+inf'))
local userOccupied = tonumber(redis.call('ZCOUNT', KEYS[2], '(' .. now, '+inf'))
local userActive = tonumber(redis.call('ZCOUNT', KEYS[4], '(' .. now, '+inf'))
return {1, state, accountOccupied - accountActive, accountActive, accountOccupied, userOccupied - userActive, userActive, userOccupied}
`)

var leaseUsageScript = redis.NewScript(`
local now = tonumber(ARGV[1])
redis.call('ZREMRANGEBYSCORE', KEYS[1], '-inf', now)
redis.call('ZREMRANGEBYSCORE', KEYS[2], '-inf', now)
local occupied = tonumber(redis.call('ZCOUNT', KEYS[1], '(' .. now, '+inf'))
local active = tonumber(redis.call('ZCOUNT', KEYS[2], '(' .. now, '+inf'))
local reserved = occupied - active
if reserved < 0 then reserved = 0 end
return {reserved, active, occupied}
`)

// RedisLeaseStore executes fixed Lua scripts against common Redis primitives.
// It performs no server version detection and is scoped to one Gateway.
type RedisLeaseStore struct {
	client redis.UniversalClient
}

// NewRedisLeaseStore builds the production lease adapter without performing a
// network probe. Runtime command failures are reported as Redis unavailable.
func NewRedisLeaseStore(client redis.UniversalClient) (*RedisLeaseStore, error) {
	if client == nil {
		return nil, ErrRedisUnavailable
	}
	return &RedisLeaseStore{client: client}, nil
}

// Reserve atomically cleans expired scores, enforces the account limit, and
// writes account/user/reverse indexes for one new reservation.
func (s *RedisLeaseStore) Reserve(ctx context.Context, request ReserveRequest, now time.Time) (ReserveResult, error) {
	if err := validateReserveRequest(request); err != nil {
		return ReserveResult{}, err
	}
	expiresAt := now.Add(ReservationTTL)
	raw := encodeLeaseSession(request.PlaybackAccountKey, request.UserID, LeaseStateReservation, expiresAt)
	values, err := reserveLeaseScript.Run(ctx, s.client, leaseKeys(request.PlaybackAccountKey, request.UserID, request.SessionFingerprint),
		request.PlaybackAccountKey, request.UserID, request.SessionFingerprint, now.UnixMilli(), request.MaxConcurrentStreams,
		expiresAt.UnixMilli(), ReservationTTL.Milliseconds(), LeaseIndexTTL.Milliseconds(), raw).Slice()
	if err != nil {
		return ReserveResult{}, mapRedisLeaseError(ctx, err)
	}
	code, err := redisInt(values, 0)
	if err != nil {
		return ReserveResult{}, err
	}
	if code == -1 {
		accountUsage, userUsage, usageErr := redisLeaseUsages(values, 2)
		if usageErr != nil {
			return ReserveResult{}, usageErr
		}
		return ReserveResult{
			PlaybackAccountKey: request.PlaybackAccountKey,
			UserID:             request.UserID,
			Account:            accountUsage,
			User:               userUsage,
		}, ErrAccountConcurrencyExceeded
	}
	if code == 2 {
		accountKey, valueErr := redisString(values, 1)
		if valueErr != nil {
			return ReserveResult{}, valueErr
		}
		userID, valueErr := redisString(values, 2)
		if valueErr != nil {
			return ReserveResult{}, valueErr
		}
		stateValue, valueErr := redisString(values, 3)
		if valueErr != nil {
			return ReserveResult{}, valueErr
		}
		accountUsage, valueErr := s.AccountUsage(ctx, accountKey, now)
		if valueErr != nil {
			return ReserveResult{}, valueErr
		}
		userUsage, valueErr := s.UserUsage(ctx, userID, now)
		if valueErr != nil {
			return ReserveResult{}, valueErr
		}
		return ReserveResult{PlaybackAccountKey: accountKey, UserID: userID, State: LeaseState(stateValue), Account: accountUsage, User: userUsage}, nil
	}
	if code != 0 && code != 1 {
		return ReserveResult{}, ErrRedisUnavailable
	}
	stateValue, err := redisString(values, 1)
	if err != nil {
		return ReserveResult{}, err
	}
	accountUsage, userUsage, err := redisLeaseUsages(values, 2)
	if err != nil {
		return ReserveResult{}, err
	}
	return ReserveResult{
		Created: code == 1, PlaybackAccountKey: request.PlaybackAccountKey, UserID: request.UserID,
		State: LeaseState(stateValue), Account: accountUsage, User: userUsage,
	}, nil
}

// Session reads a reverse mapping without renewing it. Business expiry is
// evaluated against the Gateway-supplied clock, not Redis server time.
func (s *RedisLeaseStore) Session(ctx context.Context, fingerprint string, now time.Time) (LeaseSession, bool, error) {
	if !opaqueDigestPattern.MatchString(fingerprint) {
		return LeaseSession{}, false, ErrLeaseIdentityInvalid
	}
	raw, err := s.client.Get(ctx, sessionKey(fingerprint)).Result()
	if errors.Is(err, redis.Nil) {
		return LeaseSession{}, false, nil
	}
	if err != nil {
		return LeaseSession{}, false, mapRedisLeaseError(ctx, err)
	}
	session, err := decodeLeaseSession(fingerprint, raw)
	if err != nil {
		return LeaseSession{}, false, err
	}
	if !session.ExpiresAt.After(now) {
		return LeaseSession{}, false, nil
	}
	return session, true, nil
}

// Advance promotes or refreshes one reverse-mapped session using an atomic
// compare-and-set script over all account/user indexes.
func (s *RedisLeaseStore) Advance(ctx context.Context, fingerprint string, state LeaseState, now time.Time) (TransitionResult, error) {
	if state != LeaseStateActive && state != LeaseStatePaused {
		return TransitionResult{}, ErrLeaseStateInvalid
	}
	session, raw, found, err := s.loadRawSession(ctx, fingerprint)
	if err != nil || !found {
		return TransitionResult{}, err
	}
	ttl := ActiveTTL
	if state == LeaseStatePaused {
		ttl = PausedTTL
	}
	expiresAt := now.Add(ttl)
	updatedRaw := encodeLeaseSession(session.PlaybackAccountKey, session.UserID, state, expiresAt)
	values, err := transitionLeaseScript.Run(ctx, s.client, leaseKeys(session.PlaybackAccountKey, session.UserID, fingerprint),
		raw, fingerprint, now.UnixMilli(), expiresAt.UnixMilli(), ttl.Milliseconds(), LeaseIndexTTL.Milliseconds(), updatedRaw, string(state)).Slice()
	if err != nil {
		return TransitionResult{}, mapRedisLeaseError(ctx, err)
	}
	code, err := redisInt(values, 0)
	if err != nil || code == 0 {
		return TransitionResult{}, err
	}
	previous, err := redisString(values, 1)
	if err != nil {
		return TransitionResult{}, err
	}
	accountUsage, userUsage, err := redisLeaseUsages(values, 3)
	if err != nil {
		return TransitionResult{}, err
	}
	return TransitionResult{Found: true, PreviousState: LeaseState(previous), State: state, Account: accountUsage, User: userUsage}, nil
}

// ReleaseReservation removes only the exact still-current reservation.
func (s *RedisLeaseStore) ReleaseReservation(ctx context.Context, fingerprint string, now time.Time) (bool, error) {
	session, raw, found, err := s.loadRawSession(ctx, fingerprint)
	if err != nil || !found {
		return false, err
	}
	result, err := releaseReservationScript.Run(ctx, s.client, leaseKeys(session.PlaybackAccountKey, session.UserID, fingerprint),
		raw, fingerprint, now.UnixMilli()).Int()
	if err != nil {
		return false, mapRedisLeaseError(ctx, err)
	}
	return result == 1, nil
}

// Stop removes the exact reverse-mapped session and returns post-delete usage.
func (s *RedisLeaseStore) Stop(ctx context.Context, fingerprint string, now time.Time) (TransitionResult, error) {
	session, raw, found, err := s.loadRawSession(ctx, fingerprint)
	if err != nil || !found {
		return TransitionResult{}, err
	}
	values, err := stopLeaseScript.Run(ctx, s.client, leaseKeys(session.PlaybackAccountKey, session.UserID, fingerprint),
		raw, fingerprint, now.UnixMilli()).Slice()
	if err != nil {
		return TransitionResult{}, mapRedisLeaseError(ctx, err)
	}
	code, err := redisInt(values, 0)
	if err != nil || code == 0 {
		return TransitionResult{}, err
	}
	previous, err := redisString(values, 1)
	if err != nil {
		return TransitionResult{}, err
	}
	accountUsage, userUsage, err := redisLeaseUsages(values, 2)
	if err != nil {
		return TransitionResult{}, err
	}
	return TransitionResult{Found: true, PreviousState: LeaseState(previous), Account: accountUsage, User: userUsage}, nil
}

// AccountUsage cleans and counts one opaque playback account index pair.
func (s *RedisLeaseStore) AccountUsage(ctx context.Context, accountKey string, now time.Time) (LeaseUsage, error) {
	if !opaqueDigestPattern.MatchString(accountKey) {
		return LeaseUsage{}, ErrLeaseIdentityInvalid
	}
	return s.usage(ctx, accountLeasesKey(accountKey), accountActiveKey(accountKey), now)
}

// UserUsage cleans and counts one attribution-only user index pair.
func (s *RedisLeaseStore) UserUsage(ctx context.Context, userID string, now time.Time) (LeaseUsage, error) {
	if !internalIDPattern.MatchString(userID) {
		return LeaseUsage{}, ErrLeaseIdentityInvalid
	}
	return s.usage(ctx, userLeasesKey(userID), userActiveKey(userID), now)
}

func (s *RedisLeaseStore) usage(ctx context.Context, leasesKey, activeKey string, now time.Time) (LeaseUsage, error) {
	values, err := leaseUsageScript.Run(ctx, s.client, []string{leasesKey, activeKey}, now.UnixMilli()).Slice()
	if err != nil {
		return LeaseUsage{}, mapRedisLeaseError(ctx, err)
	}
	reserved, err := redisInt(values, 0)
	if err != nil {
		return LeaseUsage{}, err
	}
	active, err := redisInt(values, 1)
	if err != nil {
		return LeaseUsage{}, err
	}
	occupied, err := redisInt(values, 2)
	if err != nil {
		return LeaseUsage{}, err
	}
	return LeaseUsage{ReservedStreams: reserved, ActiveStreams: active, OccupiedStreams: occupied}, nil
}

func (s *RedisLeaseStore) loadRawSession(ctx context.Context, fingerprint string) (LeaseSession, string, bool, error) {
	if !opaqueDigestPattern.MatchString(fingerprint) {
		return LeaseSession{}, "", false, ErrLeaseIdentityInvalid
	}
	raw, err := s.client.Get(ctx, sessionKey(fingerprint)).Result()
	if errors.Is(err, redis.Nil) {
		return LeaseSession{}, "", false, nil
	}
	if err != nil {
		return LeaseSession{}, "", false, mapRedisLeaseError(ctx, err)
	}
	session, err := decodeLeaseSession(fingerprint, raw)
	if err != nil {
		return LeaseSession{}, "", false, err
	}
	return session, raw, true, nil
}

func leaseKeys(accountKey, userID, fingerprint string) []string {
	return []string{
		accountLeasesKey(accountKey), userLeasesKey(userID),
		accountActiveKey(accountKey), userActiveKey(userID),
		sessionKey(fingerprint),
	}
}

func accountLeasesKey(accountKey string) string {
	return redisLeasePrefix + ":leases:account:" + accountKey
}
func userLeasesKey(userID string) string { return redisLeasePrefix + ":leases:user:" + userID }
func accountActiveKey(accountKey string) string {
	return redisLeasePrefix + ":active:account:" + accountKey
}
func userActiveKey(userID string) string   { return redisLeasePrefix + ":active:user:" + userID }
func sessionKey(fingerprint string) string { return redisLeasePrefix + ":session:" + fingerprint }

func encodeLeaseSession(accountKey, userID string, state LeaseState, expiresAt time.Time) string {
	return accountKey + "|" + userID + "|" + string(state) + "|" + strconv.FormatInt(expiresAt.UnixMilli(), 10)
}

func decodeLeaseSession(fingerprint, raw string) (LeaseSession, error) {
	parts := strings.Split(raw, "|")
	if len(parts) != 4 || !opaqueDigestPattern.MatchString(parts[0]) || !internalIDPattern.MatchString(parts[1]) {
		return LeaseSession{}, ErrRedisUnavailable
	}
	state := LeaseState(parts[2])
	if state != LeaseStateReservation && state != LeaseStateActive && state != LeaseStatePaused {
		return LeaseSession{}, ErrRedisUnavailable
	}
	expiresAtMillis, err := strconv.ParseInt(parts[3], 10, 64)
	if err != nil {
		return LeaseSession{}, ErrRedisUnavailable
	}
	return LeaseSession{
		PlaybackAccountKey: parts[0], UserID: parts[1], Fingerprint: fingerprint,
		State: state, ExpiresAt: time.UnixMilli(expiresAtMillis).UTC(),
	}, nil
}

func redisLeaseUsages(values []interface{}, offset int) (LeaseUsage, LeaseUsage, error) {
	items := make([]int, 6)
	for index := range items {
		value, err := redisInt(values, offset+index)
		if err != nil {
			return LeaseUsage{}, LeaseUsage{}, err
		}
		items[index] = value
	}
	return LeaseUsage{ReservedStreams: items[0], ActiveStreams: items[1], OccupiedStreams: items[2]},
		LeaseUsage{ReservedStreams: items[3], ActiveStreams: items[4], OccupiedStreams: items[5]}, nil
}

func redisInt(values []interface{}, index int) (int, error) {
	if index < 0 || index >= len(values) {
		return 0, ErrRedisUnavailable
	}
	switch value := values[index].(type) {
	case int64:
		return int(value), nil
	case string:
		parsed, err := strconv.Atoi(value)
		if err == nil {
			return parsed, nil
		}
	case []byte:
		parsed, err := strconv.Atoi(string(value))
		if err == nil {
			return parsed, nil
		}
	}
	return 0, ErrRedisUnavailable
}

func redisString(values []interface{}, index int) (string, error) {
	if index < 0 || index >= len(values) {
		return "", ErrRedisUnavailable
	}
	switch value := values[index].(type) {
	case string:
		return value, nil
	case []byte:
		return string(value), nil
	}
	return "", ErrRedisUnavailable
}

func mapRedisLeaseError(ctx context.Context, err error) error {
	if ctxErr := ctx.Err(); ctxErr != nil {
		return ctxErr
	}
	return fmt.Errorf("%w: %T", ErrRedisUnavailable, err)
}
