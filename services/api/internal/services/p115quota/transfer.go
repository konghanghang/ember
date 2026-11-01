package p115quota

import (
	"context"
	"errors"
	"regexp"
	"sync"
	"time"
)

var (
	ErrTransferIdentityInvalid    = errors.New("115 转存配额身份无效")
	ErrTransferWindowInvalid      = errors.New("115 转存配额时间窗口无效")
	ErrTransferLimitInvalid       = errors.New("115 转存配额限制无效")
	ErrTransferQuotaExceeded      = errors.New("115 转存配额已用完")
	ErrCompatibilityQuotaExceeded = errors.New("115 兼容转存子限额已用完")
)

var transferAttemptPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,128}$`)

// TransferQuotaScope adds a bounded sublimit to the user's existing total.
// ID is an internal namespace, never a Provider UID or client session value.
type TransferQuotaScope struct {
	ID          string
	HourlyLimit int
	DailyLimit  int
}

// TransferReserveRequest carries both independent plan limits and the current
// CRON_TIMEZONE day boundary calculated by Gateway.
type TransferReserveRequest struct {
	UserID           string
	AttemptID        string
	HourlyLimit      int
	DailyLimit       int
	DayStart         time.Time
	DayEnd           time.Time
	AdditionalScopes []TransferQuotaScope
}

// TransferCommitRequest identifies the same opaque attempt after Provider
// target verification succeeded.
type TransferCommitRequest struct {
	UserID           string
	AttemptID        string
	DayStart         time.Time
	DayEnd           time.Time
	AdditionalScopes []TransferQuotaScope
}

// TransferUsageRequest selects the current user's configured business day.
type TransferUsageRequest struct {
	UserID   string
	DayStart time.Time
	DayEnd   time.Time
}

// TransferUsage separates provisional pending reservations from successful
// rolling-hour and natural-day counts.
type TransferUsage struct {
	Pending    int
	HourlyUsed int
	DailyUsed  int
}

// TransferReserveResult reports an idempotent pending reservation.
type TransferReserveResult struct {
	Created bool
	Usage   TransferUsage
}

// TransferCommitResult reports NX success bookkeeping and the accepted late
// success diagnostic when the five-minute pending reservation already expired.
type TransferCommitResult struct {
	Added                      bool
	PendingExpiredBeforeCommit bool
	Usage                      TransferUsage
}

// TransferQuotaStore is the atomic pending/succeeded boundary used inside the
// existing PostgreSQL transfer content lock.
type TransferQuotaStore interface {
	ReserveTransfer(context.Context, TransferReserveRequest, time.Time) (TransferReserveResult, error)
	ReleaseTransfer(context.Context, string, string, time.Time) (bool, error)
	CommitTransfer(context.Context, TransferCommitRequest, time.Time) (TransferCommitResult, error)
	TransferUsage(context.Context, TransferUsageRequest, time.Time) (TransferUsage, error)
}

// Store is the complete Redis contract shared by Gateway routing and transfer
// orchestration.
type Store interface {
	LeaseStore
	TransferQuotaStore
}

type memoryTransferUser struct {
	pending          map[string]time.Time
	succeeded        map[string]time.Time
	pendingExpires   time.Time
	succeededExpires time.Time
}

// MemoryTransferQuotaStore is a deterministic concurrency-safe fake for quota
// tests. It has no background worker and prunes on each operation.
type MemoryTransferQuotaStore struct {
	mu    sync.Mutex
	users map[string]*memoryTransferUser
}

// NewMemoryTransferQuotaStore builds an empty fake transfer quota store.
func NewMemoryTransferQuotaStore() *MemoryTransferQuotaStore {
	return &MemoryTransferQuotaStore{users: make(map[string]*memoryTransferUser)}
}

// DayWindow returns one natural-day interval in the configured global timezone.
func DayWindow(now time.Time, location *time.Location) (time.Time, time.Time) {
	if location == nil {
		location = time.UTC
	}
	local := now.In(location)
	start := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, location)
	return start, start.AddDate(0, 0, 1)
}

// ReserveTransfer checks every total and sublimit before publishing any pending reservation.
func (s *MemoryTransferQuotaStore) ReserveTransfer(ctx context.Context, request TransferReserveRequest, now time.Time) (TransferReserveResult, error) {
	if err := ctx.Err(); err != nil {
		return TransferReserveResult{}, err
	}
	if err := validateTransferReserveRequest(request, now); err != nil {
		return TransferReserveResult{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	user := s.userLocked(request.UserID, now, true)
	s.pruneUserLocked(user, now, request.DayStart)
	usage := transferUsageLocked(user, now, request.DayStart, request.DayEnd)
	if expiresAt, ok := user.pending[request.AttemptID]; ok && expiresAt.After(now) {
		return TransferReserveResult{Usage: usage}, nil
	}
	if _, ok := user.succeeded[request.AttemptID]; ok {
		return TransferReserveResult{Usage: usage}, nil
	}
	scopes := append([]TransferQuotaScope{{ID: request.UserID, HourlyLimit: request.HourlyLimit, DailyLimit: request.DailyLimit}}, request.AdditionalScopes...)
	for index, scope := range scopes {
		entry := s.userLocked(scope.ID, now, true)
		s.pruneUserLocked(entry, now, request.DayStart)
		scopeUsage := transferUsageLocked(entry, now, request.DayStart, request.DayEnd)
		if scopeUsage.HourlyUsed+scopeUsage.Pending >= scope.HourlyLimit || scopeUsage.DailyUsed+scopeUsage.Pending >= scope.DailyLimit {
			if index > 0 {
				return TransferReserveResult{Usage: usage}, ErrCompatibilityQuotaExceeded
			}
			return TransferReserveResult{Usage: usage}, ErrTransferQuotaExceeded
		}
	}
	for _, scope := range scopes {
		entry := s.userLocked(scope.ID, now, true)
		entry.pending[request.AttemptID] = now.Add(TransferPendingTTL)
		entry.pendingExpires = now.Add(TransferPendingKeyTTL)
	}
	usage = transferUsageLocked(s.users[request.UserID], now, request.DayStart, request.DayEnd)
	return TransferReserveResult{Created: true, Usage: usage}, nil
}

func (s *MemoryTransferQuotaStore) ReleaseTransfer(ctx context.Context, userID, attemptID string, now time.Time) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if !internalIDPattern.MatchString(userID) || !transferAttemptPattern.MatchString(attemptID) {
		return false, ErrTransferIdentityInvalid
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	user := s.userLocked(userID, now, false)
	if user == nil {
		return false, nil
	}
	expiresAt, exists := user.pending[attemptID]
	delete(user.pending, attemptID)
	return exists && expiresAt.After(now), nil
}

// CommitTransfer records all scopes under one lock, including idempotent late success.
func (s *MemoryTransferQuotaStore) CommitTransfer(ctx context.Context, request TransferCommitRequest, now time.Time) (TransferCommitResult, error) {
	if err := ctx.Err(); err != nil {
		return TransferCommitResult{}, err
	}
	if err := validateTransferWindow(request.UserID, request.AttemptID, request.DayStart, request.DayEnd, now); err != nil {
		return TransferCommitResult{}, err
	}
	if err := validateTransferScopes(request.UserID, request.AdditionalScopes); err != nil {
		return TransferCommitResult{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	var result TransferCommitResult
	scopes := append([]TransferQuotaScope{{ID: request.UserID}}, request.AdditionalScopes...)
	for index, scope := range scopes {
		user := s.userLocked(scope.ID, now, true)
		pendingExpiresAt, pendingExists := user.pending[request.AttemptID]
		pendingLive := pendingExists && pendingExpiresAt.After(now)
		s.pruneUserLocked(user, now, request.DayStart)
		delete(user.pending, request.AttemptID)
		_, alreadyCommitted := user.succeeded[request.AttemptID]
		if !alreadyCommitted {
			user.succeeded[request.AttemptID] = now
			user.succeededExpires = now.Add(succeededKeyTTL(now, request.DayEnd))
		}
		if index == 0 {
			result = TransferCommitResult{Added: !alreadyCommitted, PendingExpiredBeforeCommit: !alreadyCommitted && !pendingLive, Usage: transferUsageLocked(user, now, request.DayStart, request.DayEnd)}
		}
	}
	return result, nil
}

func (s *MemoryTransferQuotaStore) TransferUsage(ctx context.Context, request TransferUsageRequest, now time.Time) (TransferUsage, error) {
	if err := ctx.Err(); err != nil {
		return TransferUsage{}, err
	}
	if err := validateTransferWindow(request.UserID, "usage", request.DayStart, request.DayEnd, now); err != nil {
		return TransferUsage{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	user := s.userLocked(request.UserID, now, false)
	if user == nil {
		return TransferUsage{}, nil
	}
	s.pruneUserLocked(user, now, request.DayStart)
	return transferUsageLocked(user, now, request.DayStart, request.DayEnd), nil
}

func validateTransferReserveRequest(request TransferReserveRequest, now time.Time) error {
	if err := validateTransferWindow(request.UserID, request.AttemptID, request.DayStart, request.DayEnd, now); err != nil {
		return err
	}
	if request.HourlyLimit < 1 || request.HourlyLimit > 100 || request.DailyLimit < 1 || request.DailyLimit > 1000 {
		return ErrTransferLimitInvalid
	}
	return validateTransferScopes(request.UserID, request.AdditionalScopes)
}

// validateTransferScopes bounds script work and rejects colliding counters;
// callers carry the same scopes through reserve, refund and commit.
func validateTransferScopes(userID string, scopes []TransferQuotaScope) error {
	if len(scopes) > 2 {
		return ErrTransferLimitInvalid
	}
	seen := map[string]bool{userID: true}
	for _, scope := range scopes {
		if !internalIDPattern.MatchString(scope.ID) || seen[scope.ID] {
			return ErrTransferIdentityInvalid
		}
		if scope.HourlyLimit < 1 || scope.HourlyLimit > 100 || scope.DailyLimit < 1 || scope.DailyLimit > 1000 {
			return ErrTransferLimitInvalid
		}
		seen[scope.ID] = true
	}
	return nil
}

func validateTransferWindow(userID, attemptID string, dayStart, dayEnd, now time.Time) error {
	if !internalIDPattern.MatchString(userID) || !transferAttemptPattern.MatchString(attemptID) {
		return ErrTransferIdentityInvalid
	}
	if dayStart.IsZero() || dayEnd.IsZero() || !dayEnd.After(dayStart) || now.Before(dayStart) || !now.Before(dayEnd) {
		return ErrTransferWindowInvalid
	}
	return nil
}

func succeededKeyTTL(now, dayEnd time.Time) time.Duration {
	ttl := dayEnd.Sub(now)
	if ttl < time.Hour {
		ttl = time.Hour
	}
	return ttl + time.Minute
}

func (s *MemoryTransferQuotaStore) userLocked(userID string, now time.Time, create bool) *memoryTransferUser {
	user := s.users[userID]
	if user != nil && !user.pendingExpires.After(now) && !user.succeededExpires.After(now) {
		delete(s.users, userID)
		user = nil
	}
	if user == nil && create {
		user = &memoryTransferUser{pending: make(map[string]time.Time), succeeded: make(map[string]time.Time)}
		s.users[userID] = user
	}
	return user
}

func (s *MemoryTransferQuotaStore) pruneUserLocked(user *memoryTransferUser, now, dayStart time.Time) {
	if user == nil {
		return
	}
	if !user.pendingExpires.IsZero() && !user.pendingExpires.After(now) {
		user.pending = make(map[string]time.Time)
	}
	for attemptID, expiresAt := range user.pending {
		if !expiresAt.After(now) {
			delete(user.pending, attemptID)
		}
	}
	oldest := now.Add(-time.Hour)
	if dayStart.Before(oldest) {
		oldest = dayStart
	}
	if !user.succeededExpires.IsZero() && !user.succeededExpires.After(now) {
		user.succeeded = make(map[string]time.Time)
	}
	for attemptID, succeededAt := range user.succeeded {
		if succeededAt.Before(oldest) {
			delete(user.succeeded, attemptID)
		}
	}
}

func transferUsageLocked(user *memoryTransferUser, now, dayStart, dayEnd time.Time) TransferUsage {
	if user == nil {
		return TransferUsage{}
	}
	usage := TransferUsage{Pending: len(user.pending)}
	hourStart := now.Add(-time.Hour)
	for _, succeededAt := range user.succeeded {
		if !succeededAt.Before(hourStart) && !succeededAt.After(now) {
			usage.HourlyUsed++
		}
		if !succeededAt.Before(dayStart) && succeededAt.Before(dayEnd) {
			usage.DailyUsed++
		}
	}
	return usage
}
