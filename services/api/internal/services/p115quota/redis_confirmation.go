package p115quota

import (
	"context"
	"time"

	redis "github.com/redis/go-redis/v9"
)

var confirmLeaseScript = redis.NewScript(`
local now = tonumber(ARGV[4])
for index = 1, 4 do redis.call('ZREMRANGEBYSCORE', KEYS[index], '-inf', now) end
local existing = redis.call('GET', KEYS[5])
if not existing then return {0} end
local accountKey, userId, state, expiresAt = string.match(existing, '^([^|]+)|([^|]+)|([^|]+)|([0-9]+)$')
if not expiresAt or (state ~= 'reservation' and state ~= 'active' and state ~= 'paused') then
  return redis.error_reply('invalid p115 session value')
end
if accountKey ~= ARGV[1] or userId ~= ARGV[2] then return {-1} end
if tonumber(expiresAt) <= now then
  for index = 1, 4 do redis.call('ZREM', KEYS[index], ARGV[3]) end
  redis.call('DEL', KEYS[5])
  return {0}
end
if state == 'reservation' and ARGV[7] == '1' then
  expiresAt = ARGV[5]
  redis.call('ZADD', KEYS[1], tonumber(expiresAt), ARGV[3])
  redis.call('ZADD', KEYS[2], tonumber(expiresAt), ARGV[3])
  for index = 1, 4 do redis.call('PEXPIRE', KEYS[index], tonumber(ARGV[6])) end
  redis.call('SET', KEYS[5], accountKey .. '|' .. userId .. '|' .. state .. '|' .. expiresAt, 'PX', tonumber(ARGV[8]))
end
local accountOccupied = tonumber(redis.call('ZCOUNT', KEYS[1], '(' .. now, '+inf'))
local accountActive = tonumber(redis.call('ZCOUNT', KEYS[3], '(' .. now, '+inf'))
local userOccupied = tonumber(redis.call('ZCOUNT', KEYS[2], '(' .. now, '+inf'))
local userActive = tonumber(redis.call('ZCOUNT', KEYS[4], '(' .. now, '+inf'))
return {1, state, expiresAt, accountOccupied - accountActive, accountActive, accountOccupied, userOccupied - userActive, userActive, userOccupied}
`)

// Confirm checks identity, expiry and usage in one Lua operation. Only an
// existing reservation may be renewed; HEAD and event-owned states keep TTLs.
func (s *RedisLeaseStore) Confirm(ctx context.Context, request ConfirmRequest, now time.Time) (ConfirmResult, error) {
	if err := ctx.Err(); err != nil {
		return ConfirmResult{}, err
	}
	if err := validateConfirmRequest(request); err != nil {
		return ConfirmResult{}, err
	}
	renew := "0"
	if request.RenewReservation {
		renew = "1"
	}
	values, err := confirmLeaseScript.Run(ctx, s.client, leaseKeys(request.PlaybackAccountKey, request.UserID, request.SessionFingerprint),
		request.PlaybackAccountKey, request.UserID, request.SessionFingerprint, now.UnixMilli(), now.Add(ReservationTTL).UnixMilli(), LeaseIndexTTL.Milliseconds(), renew, ReservationTTL.Milliseconds()).Slice()
	if err != nil {
		return ConfirmResult{}, mapRedisLeaseError(ctx, err)
	}
	code, err := redisInt(values, 0)
	if err != nil || code == 0 {
		return ConfirmResult{}, err
	}
	if code == -1 {
		return ConfirmResult{}, ErrLeaseIdentityInvalid
	}
	if code != 1 {
		return ConfirmResult{}, ErrRedisUnavailable
	}
	state, err := redisString(values, 1)
	if err != nil {
		return ConfirmResult{}, err
	}
	expiresAt, err := redisInt(values, 2)
	if err != nil {
		return ConfirmResult{}, err
	}
	account, user, err := redisLeaseUsages(values, 3)
	if err != nil {
		return ConfirmResult{}, err
	}
	return ConfirmResult{Found: true, Session: LeaseSession{PlaybackAccountKey: request.PlaybackAccountKey, UserID: request.UserID, Fingerprint: request.SessionFingerprint, State: LeaseState(state), ExpiresAt: time.UnixMilli(int64(expiresAt)).UTC()}, Account: account, User: user}, nil
}
