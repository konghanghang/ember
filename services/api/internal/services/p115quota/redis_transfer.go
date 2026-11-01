package p115quota

import (
	"context"
	"time"

	redis "github.com/redis/go-redis/v9"
)

var reserveTransferScript = redis.NewScript(`
local now = tonumber(ARGV[2])
local hourStart = tonumber(ARGV[4])
local dayStart = tonumber(ARGV[5])
local dayEnd = tonumber(ARGV[6])
local oldest = math.min(hourStart, dayStart)
local function usage(index)
  return tonumber(redis.call('ZCOUNT', KEYS[index], '(' .. now, '+inf')),
    tonumber(redis.call('ZCOUNT', KEYS[index+1], hourStart, now)),
    tonumber(redis.call('ZCOUNT', KEYS[index+1], dayStart, '(' .. dayEnd))
end
for index=1,#KEYS,2 do
  redis.call('ZREMRANGEBYSCORE', KEYS[index], '-inf', now)
  redis.call('ZREMRANGEBYSCORE', KEYS[index+1], '-inf', '(' .. oldest)
end
local pending, hourly, daily = usage(1)
if redis.call('ZSCORE', KEYS[1], ARGV[1]) or redis.call('ZSCORE', KEYS[2], ARGV[1]) then
  return {0, pending, hourly, daily}
end
for index=1,#KEYS,2 do
  local p,h,d = usage(index)
  local limitIndex = 8 + index - 1
  if h+p >= tonumber(ARGV[limitIndex]) or d+p >= tonumber(ARGV[limitIndex+1]) then
    return {-index, pending, hourly, daily}
  end
end
for index=1,#KEYS,2 do
  redis.call('ZADD', KEYS[index], tonumber(ARGV[3]), ARGV[1])
  redis.call('PEXPIRE', KEYS[index], tonumber(ARGV[7]))
end
pending, hourly, daily = usage(1)
return {1, pending, hourly, daily}
`)

var releaseTransferScript = redis.NewScript(`
local now = tonumber(ARGV[2])
redis.call('ZREMRANGEBYSCORE', KEYS[1], '-inf', now)
return redis.call('ZREM', KEYS[1], ARGV[1])
`)

var commitTransferScript = redis.NewScript(`
local now = tonumber(ARGV[2])
local hourStart = tonumber(ARGV[3])
local dayStart = tonumber(ARGV[4])
local dayEnd = tonumber(ARGV[5])
local oldest = math.min(hourStart, dayStart)
local pendingScore = redis.call('ZSCORE', KEYS[1], ARGV[1])
local pendingLive = pendingScore and tonumber(pendingScore) > now
local added = 0
for index=1,#KEYS,2 do
  redis.call('ZREMRANGEBYSCORE', KEYS[index], '-inf', now)
  redis.call('ZREMRANGEBYSCORE', KEYS[index+1], '-inf', '(' .. oldest)
  redis.call('ZREM', KEYS[index], ARGV[1])
  local changed = redis.call('ZADD', KEYS[index+1], 'NX', now, ARGV[1])
  if changed == 1 then redis.call('PEXPIRE', KEYS[index+1], tonumber(ARGV[6])) end
  if index == 1 then added = changed end
end
local pending = tonumber(redis.call('ZCOUNT', KEYS[1], '(' .. now, '+inf'))
local hourly = tonumber(redis.call('ZCOUNT', KEYS[2], hourStart, now))
local daily = tonumber(redis.call('ZCOUNT', KEYS[2], dayStart, '(' .. dayEnd))
local late = 0
if added == 1 and not pendingLive then late = 1 end
return {added, late, pending, hourly, daily}
`)

var transferUsageScript = redis.NewScript(`
local now = tonumber(ARGV[1])
local hourStart = tonumber(ARGV[2])
local dayStart = tonumber(ARGV[3])
local dayEnd = tonumber(ARGV[4])
redis.call('ZREMRANGEBYSCORE', KEYS[1], '-inf', now)
local oldest = hourStart
if dayStart < oldest then oldest = dayStart end
redis.call('ZREMRANGEBYSCORE', KEYS[2], '-inf', '(' .. oldest)
local pending = tonumber(redis.call('ZCOUNT', KEYS[1], '(' .. now, '+inf'))
local hourly = tonumber(redis.call('ZCOUNT', KEYS[2], hourStart, now))
local daily = tonumber(redis.call('ZCOUNT', KEYS[2], dayStart, '(' .. dayEnd))
return {pending, hourly, daily}
`)

// ReserveTransfer atomically includes all non-expired pending attempts in both
// quota windows so concurrent Provider calls cannot penetrate the limits.
func (s *RedisLeaseStore) ReserveTransfer(ctx context.Context, request TransferReserveRequest, now time.Time) (TransferReserveResult, error) {
	if err := validateTransferReserveRequest(request, now); err != nil {
		return TransferReserveResult{}, err
	}
	keys := transferScopeKeys(request.UserID, request.AdditionalScopes)
	args := []interface{}{request.AttemptID, now.UnixMilli(), now.Add(TransferPendingTTL).UnixMilli(), now.Add(-time.Hour).UnixMilli(), request.DayStart.UnixMilli(), request.DayEnd.UnixMilli(), TransferPendingKeyTTL.Milliseconds(), request.HourlyLimit, request.DailyLimit}
	for _, scope := range request.AdditionalScopes {
		args = append(args, scope.HourlyLimit, scope.DailyLimit)
	}
	values, err := reserveTransferScript.Run(ctx, s.client, keys, args...).Slice()
	if err != nil {
		return TransferReserveResult{}, mapRedisLeaseError(ctx, err)
	}
	code, err := redisInt(values, 0)
	if err != nil {
		return TransferReserveResult{}, err
	}
	if code < 0 {
		usage, usageErr := redisTransferUsage(values, 1)
		if usageErr != nil {
			return TransferReserveResult{}, usageErr
		}
		if code < -1 {
			return TransferReserveResult{Usage: usage}, ErrCompatibilityQuotaExceeded
		}
		return TransferReserveResult{Usage: usage}, ErrTransferQuotaExceeded
	}
	usage, err := redisTransferUsage(values, 1)
	if err != nil {
		return TransferReserveResult{}, err
	}
	return TransferReserveResult{Created: code == 1, Usage: usage}, nil
}

// ReleaseTransfer removes a live pending reservation after a normal failure or
// a second target lookup proves the file already existed.
func (s *RedisLeaseStore) ReleaseTransfer(ctx context.Context, userID, attemptID string, now time.Time) (bool, error) {
	if !internalIDPattern.MatchString(userID) || !transferAttemptPattern.MatchString(attemptID) {
		return false, ErrTransferIdentityInvalid
	}
	removed, err := releaseTransferScript.Run(ctx, s.client, []string{transferPendingKey(userID)}, attemptID, now.UnixMilli()).Int()
	if err != nil {
		return false, mapRedisLeaseError(ctx, err)
	}
	return removed == 1, nil
}

// CommitTransfer idempotently moves one attempt to succeeded. A missing or
// expired pending member is diagnostic only and never suppresses late success.
func (s *RedisLeaseStore) CommitTransfer(ctx context.Context, request TransferCommitRequest, now time.Time) (TransferCommitResult, error) {
	if err := validateTransferWindow(request.UserID, request.AttemptID, request.DayStart, request.DayEnd, now); err != nil {
		return TransferCommitResult{}, err
	}
	if err := validateTransferScopes(request.UserID, request.AdditionalScopes); err != nil {
		return TransferCommitResult{}, err
	}
	values, err := commitTransferScript.Run(ctx, s.client,
		transferScopeKeys(request.UserID, request.AdditionalScopes),
		request.AttemptID, now.UnixMilli(), now.Add(-time.Hour).UnixMilli(), request.DayStart.UnixMilli(), request.DayEnd.UnixMilli(),
		succeededKeyTTL(now, request.DayEnd).Milliseconds(),
	).Slice()
	if err != nil {
		return TransferCommitResult{}, mapRedisLeaseError(ctx, err)
	}
	added, err := redisInt(values, 0)
	if err != nil {
		return TransferCommitResult{}, err
	}
	late, err := redisInt(values, 1)
	if err != nil {
		return TransferCommitResult{}, err
	}
	usage, err := redisTransferUsage(values, 2)
	if err != nil {
		return TransferCommitResult{}, err
	}
	return TransferCommitResult{Added: added == 1, PendingExpiredBeforeCommit: late == 1, Usage: usage}, nil
}

// TransferUsage returns successful usage and live pending reservations for the
// caller's current configured business day.
func (s *RedisLeaseStore) TransferUsage(ctx context.Context, request TransferUsageRequest, now time.Time) (TransferUsage, error) {
	if err := validateTransferWindow(request.UserID, "usage", request.DayStart, request.DayEnd, now); err != nil {
		return TransferUsage{}, err
	}
	values, err := transferUsageScript.Run(ctx, s.client,
		[]string{transferPendingKey(request.UserID), transferSucceededKey(request.UserID)},
		now.UnixMilli(), now.Add(-time.Hour).UnixMilli(), request.DayStart.UnixMilli(), request.DayEnd.UnixMilli(),
	).Slice()
	if err != nil {
		return TransferUsage{}, mapRedisLeaseError(ctx, err)
	}
	return redisTransferUsage(values, 0)
}

func transferPendingKey(userID string) string {
	return redisLeasePrefix + ":transfer:pending:" + userID
}
func transferSucceededKey(userID string) string {
	return redisLeasePrefix + ":transfer:succeeded:" + userID
}

func redisTransferUsage(values []interface{}, offset int) (TransferUsage, error) {
	pending, err := redisInt(values, offset)
	if err != nil {
		return TransferUsage{}, err
	}
	hourly, err := redisInt(values, offset+1)
	if err != nil {
		return TransferUsage{}, err
	}
	daily, err := redisInt(values, offset+2)
	if err != nil {
		return TransferUsage{}, err
	}
	return TransferUsage{Pending: pending, HourlyUsed: hourly, DailyUsed: daily}, nil
}

// transferScopeKeys keeps the primary user's diagnostic counters first and adds
// bounded internal sublimits to the same atomic Lua operation.
func transferScopeKeys(userID string, scopes []TransferQuotaScope) []string {
	keys := []string{transferPendingKey(userID), transferSucceededKey(userID)}
	for _, scope := range scopes {
		keys = append(keys, transferPendingKey(scope.ID), transferSucceededKey(scope.ID))
	}
	return keys
}
