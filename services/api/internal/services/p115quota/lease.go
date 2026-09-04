package p115quota

import (
	"context"
	"errors"
	"regexp"
	"sync"
	"time"
)

var (
	ErrLeaseIdentityInvalid       = errors.New("115 播放租约身份无效")
	ErrLeaseStateInvalid          = errors.New("115 播放租约状态无效")
	ErrAccountConcurrencyExceeded = errors.New("115 播放账号并发已满")
)

var (
	opaqueDigestPattern = regexp.MustCompile(`^[0-9a-f]{64}$`)
	internalIDPattern   = regexp.MustCompile(`^[A-Za-z0-9_-]{1,128}$`)
)

// ReserveRequest identifies one 115 playback session and the selected
// account-wide admission limit. User indexes are attribution-only.
type ReserveRequest struct {
	PlaybackAccountKey   string
	UserID               string
	SessionFingerprint   string
	MaxConcurrentStreams int
}

// LeaseUsage separates short reservations from confirmed active/paused
// sessions while retaining the total account occupancy used for admission.
type LeaseUsage struct {
	ReservedStreams int
	ActiveStreams   int
	OccupiedStreams int
}

// LeaseSession is the bounded reverse mapping needed by later Emby events.
type LeaseSession struct {
	PlaybackAccountKey string
	UserID             string
	Fingerprint        string
	State              LeaseState
	ExpiresAt          time.Time
}

// ReserveResult reports whether this request created a new reservation or
// reused the same session without downgrading its current state.
type ReserveResult struct {
	Created            bool
	PlaybackAccountKey string
	UserID             string
	State              LeaseState
	Account            LeaseUsage
	User               LeaseUsage
}

// TransitionResult describes an event-driven state change. Missing reverse
// sessions are a normal no-op and return Found=false.
type TransitionResult struct {
	Found         bool
	PreviousState LeaseState
	State         LeaseState
	Account       LeaseUsage
	User          LeaseUsage
}

// LeaseStore is the Redis/fake boundary used by the single Gateway process.
// Callers supply one injectable business clock for every operation.
type LeaseStore interface {
	Reserve(context.Context, ReserveRequest, time.Time) (ReserveResult, error)
	Session(context.Context, string, time.Time) (LeaseSession, bool, error)
	Advance(context.Context, string, LeaseState, time.Time) (TransitionResult, error)
	ReleaseReservation(context.Context, string, time.Time) (bool, error)
	Stop(context.Context, string, time.Time) (TransitionResult, error)
	AccountUsage(context.Context, string, time.Time) (LeaseUsage, error)
	UserUsage(context.Context, string, time.Time) (LeaseUsage, error)
}

type memoryLeaseIndex struct {
	members   map[string]time.Time
	expiresAt time.Time
}

type memoryLeaseSession struct {
	LeaseSession
}

// MemoryLeaseStore is a concurrency-safe deterministic fake for Gateway and
// service tests. It mirrors Redis score expiry and physical index TTLs.
type MemoryLeaseStore struct {
	*MemoryTransferQuotaStore
	mu            sync.Mutex
	sessions      map[string]memoryLeaseSession
	accountLeases map[string]*memoryLeaseIndex
	accountActive map[string]*memoryLeaseIndex
	userLeases    map[string]*memoryLeaseIndex
	userActive    map[string]*memoryLeaseIndex
}

// NewMemoryLeaseStore builds an empty in-process lease store without starting
// a server or background cleanup worker.
func NewMemoryLeaseStore() *MemoryLeaseStore {
	return &MemoryLeaseStore{
		MemoryTransferQuotaStore: NewMemoryTransferQuotaStore(),
		sessions:                 make(map[string]memoryLeaseSession),
		accountLeases:            make(map[string]*memoryLeaseIndex),
		accountActive:            make(map[string]*memoryLeaseIndex),
		userLeases:               make(map[string]*memoryLeaseIndex),
		userActive:               make(map[string]*memoryLeaseIndex),
	}
}

// Reserve atomically reuses an existing session or creates one short
// reservation when the selected account still has capacity.
func (s *MemoryLeaseStore) Reserve(ctx context.Context, request ReserveRequest, now time.Time) (ReserveResult, error) {
	if err := ctx.Err(); err != nil {
		return ReserveResult{}, err
	}
	if err := validateReserveRequest(request); err != nil {
		return ReserveResult{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	if existing, ok := s.sessionLocked(request.SessionFingerprint, now); ok {
		return ReserveResult{
			Created: false, PlaybackAccountKey: existing.PlaybackAccountKey, UserID: existing.UserID,
			State: existing.State, Account: s.accountUsageLocked(existing.PlaybackAccountKey, now),
			User: s.userUsageLocked(existing.UserID, now),
		}, nil
	}
	accountUsage := s.accountUsageLocked(request.PlaybackAccountKey, now)
	if accountUsage.OccupiedStreams >= request.MaxConcurrentStreams {
		return ReserveResult{
			PlaybackAccountKey: request.PlaybackAccountKey,
			UserID:             request.UserID,
			Account:            accountUsage,
			User:               s.userUsageLocked(request.UserID, now),
		}, ErrAccountConcurrencyExceeded
	}

	expiresAt := now.Add(ReservationTTL)
	s.setIndexMemberLocked(s.accountLeases, request.PlaybackAccountKey, request.SessionFingerprint, expiresAt, now)
	s.setIndexMemberLocked(s.userLeases, request.UserID, request.SessionFingerprint, expiresAt, now)
	s.touchIndexLocked(s.accountActive, request.PlaybackAccountKey, now)
	s.touchIndexLocked(s.userActive, request.UserID, now)
	s.sessions[request.SessionFingerprint] = memoryLeaseSession{LeaseSession: LeaseSession{
		PlaybackAccountKey: request.PlaybackAccountKey,
		UserID:             request.UserID,
		Fingerprint:        request.SessionFingerprint,
		State:              LeaseStateReservation,
		ExpiresAt:          expiresAt,
	}}
	return ReserveResult{
		Created: true, PlaybackAccountKey: request.PlaybackAccountKey, UserID: request.UserID,
		State: LeaseStateReservation, Account: s.accountUsageLocked(request.PlaybackAccountKey, now),
		User: s.userUsageLocked(request.UserID, now),
	}, nil
}

// Session returns a non-expired reverse mapping without refreshing its TTL.
func (s *MemoryLeaseStore) Session(ctx context.Context, fingerprint string, now time.Time) (LeaseSession, bool, error) {
	if err := ctx.Err(); err != nil {
		return LeaseSession{}, false, err
	}
	if !opaqueDigestPattern.MatchString(fingerprint) {
		return LeaseSession{}, false, ErrLeaseIdentityInvalid
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	session, ok := s.sessionLocked(fingerprint, now)
	return session, ok, nil
}

// Advance promotes or refreshes an existing session as active or paused and
// updates account/user leases plus active indexes together.
func (s *MemoryLeaseStore) Advance(ctx context.Context, fingerprint string, state LeaseState, now time.Time) (TransitionResult, error) {
	if err := ctx.Err(); err != nil {
		return TransitionResult{}, err
	}
	if !opaqueDigestPattern.MatchString(fingerprint) {
		return TransitionResult{}, ErrLeaseIdentityInvalid
	}
	if state != LeaseStateActive && state != LeaseStatePaused {
		return TransitionResult{}, ErrLeaseStateInvalid
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	session, ok := s.sessionLocked(fingerprint, now)
	if !ok {
		return TransitionResult{}, nil
	}
	previous := session.State
	ttl := ActiveTTL
	if state == LeaseStatePaused {
		ttl = PausedTTL
	}
	expiresAt := now.Add(ttl)
	s.setIndexMemberLocked(s.accountLeases, session.PlaybackAccountKey, fingerprint, expiresAt, now)
	s.setIndexMemberLocked(s.userLeases, session.UserID, fingerprint, expiresAt, now)
	s.setIndexMemberLocked(s.accountActive, session.PlaybackAccountKey, fingerprint, expiresAt, now)
	s.setIndexMemberLocked(s.userActive, session.UserID, fingerprint, expiresAt, now)
	session.State = state
	session.ExpiresAt = expiresAt
	s.sessions[fingerprint] = memoryLeaseSession{LeaseSession: session}
	return TransitionResult{
		Found: true, PreviousState: previous, State: state,
		Account: s.accountUsageLocked(session.PlaybackAccountKey, now),
		User:    s.userUsageLocked(session.UserID, now),
	}, nil
}

// ReleaseReservation removes only an existing reservation. Active or paused
// sessions survive a failed URL re-signing attempt.
func (s *MemoryLeaseStore) ReleaseReservation(ctx context.Context, fingerprint string, now time.Time) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if !opaqueDigestPattern.MatchString(fingerprint) {
		return false, ErrLeaseIdentityInvalid
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	session, ok := s.sessionLocked(fingerprint, now)
	if !ok || session.State != LeaseStateReservation {
		return false, nil
	}
	s.deleteSessionLocked(session)
	return true, nil
}

// Stop removes any existing reservation/active/paused session atomically.
func (s *MemoryLeaseStore) Stop(ctx context.Context, fingerprint string, now time.Time) (TransitionResult, error) {
	if err := ctx.Err(); err != nil {
		return TransitionResult{}, err
	}
	if !opaqueDigestPattern.MatchString(fingerprint) {
		return TransitionResult{}, ErrLeaseIdentityInvalid
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	session, ok := s.sessionLocked(fingerprint, now)
	if !ok {
		return TransitionResult{}, nil
	}
	s.deleteSessionLocked(session)
	return TransitionResult{
		Found: true, PreviousState: session.State,
		Account: s.accountUsageLocked(session.PlaybackAccountKey, now),
		User:    s.userUsageLocked(session.UserID, now),
	}, nil
}

// AccountUsage returns effective members with score strictly greater than now.
func (s *MemoryLeaseStore) AccountUsage(ctx context.Context, accountKey string, now time.Time) (LeaseUsage, error) {
	if err := ctx.Err(); err != nil {
		return LeaseUsage{}, err
	}
	if !opaqueDigestPattern.MatchString(accountKey) {
		return LeaseUsage{}, ErrLeaseIdentityInvalid
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.accountUsageLocked(accountKey, now), nil
}

// UserUsage returns attribution-only usage for one Ember user.
func (s *MemoryLeaseStore) UserUsage(ctx context.Context, userID string, now time.Time) (LeaseUsage, error) {
	if err := ctx.Err(); err != nil {
		return LeaseUsage{}, err
	}
	if !internalIDPattern.MatchString(userID) {
		return LeaseUsage{}, ErrLeaseIdentityInvalid
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.userUsageLocked(userID, now), nil
}

func validateReserveRequest(request ReserveRequest) error {
	if !opaqueDigestPattern.MatchString(request.PlaybackAccountKey) ||
		!opaqueDigestPattern.MatchString(request.SessionFingerprint) ||
		!internalIDPattern.MatchString(request.UserID) ||
		request.MaxConcurrentStreams < 1 {
		return ErrLeaseIdentityInvalid
	}
	return nil
}

func (s *MemoryLeaseStore) sessionLocked(fingerprint string, now time.Time) (LeaseSession, bool) {
	stored, ok := s.sessions[fingerprint]
	if !ok {
		return LeaseSession{}, false
	}
	if !stored.ExpiresAt.After(now) {
		s.deleteSessionLocked(stored.LeaseSession)
		return LeaseSession{}, false
	}
	return stored.LeaseSession, true
}

func (s *MemoryLeaseStore) deleteSessionLocked(session LeaseSession) {
	delete(s.sessions, session.Fingerprint)
	s.deleteIndexMemberLocked(s.accountLeases, session.PlaybackAccountKey, session.Fingerprint)
	s.deleteIndexMemberLocked(s.accountActive, session.PlaybackAccountKey, session.Fingerprint)
	s.deleteIndexMemberLocked(s.userLeases, session.UserID, session.Fingerprint)
	s.deleteIndexMemberLocked(s.userActive, session.UserID, session.Fingerprint)
}

func (s *MemoryLeaseStore) accountUsageLocked(accountKey string, now time.Time) LeaseUsage {
	return leaseUsageFromCounts(
		s.indexCountLocked(s.accountLeases, accountKey, now),
		s.indexCountLocked(s.accountActive, accountKey, now),
	)
}

func (s *MemoryLeaseStore) userUsageLocked(userID string, now time.Time) LeaseUsage {
	return leaseUsageFromCounts(
		s.indexCountLocked(s.userLeases, userID, now),
		s.indexCountLocked(s.userActive, userID, now),
	)
}

func leaseUsageFromCounts(occupied, active int) LeaseUsage {
	reserved := occupied - active
	if reserved < 0 {
		reserved = 0
	}
	return LeaseUsage{ReservedStreams: reserved, ActiveStreams: active, OccupiedStreams: occupied}
}

func (s *MemoryLeaseStore) setIndexMemberLocked(indexes map[string]*memoryLeaseIndex, key, member string, expiresAt, now time.Time) {
	index := s.liveIndexLocked(indexes, key, now, true)
	index.members[member] = expiresAt
	index.expiresAt = now.Add(LeaseIndexTTL)
}

func (s *MemoryLeaseStore) touchIndexLocked(indexes map[string]*memoryLeaseIndex, key string, now time.Time) {
	if index := s.liveIndexLocked(indexes, key, now, false); index != nil {
		index.expiresAt = now.Add(LeaseIndexTTL)
	}
}

func (s *MemoryLeaseStore) deleteIndexMemberLocked(indexes map[string]*memoryLeaseIndex, key, member string) {
	if index := indexes[key]; index != nil {
		delete(index.members, member)
	}
}

func (s *MemoryLeaseStore) indexCountLocked(indexes map[string]*memoryLeaseIndex, key string, now time.Time) int {
	index := s.liveIndexLocked(indexes, key, now, false)
	if index == nil {
		return 0
	}
	for member, expiresAt := range index.members {
		if !expiresAt.After(now) {
			delete(index.members, member)
		}
	}
	return len(index.members)
}

func (s *MemoryLeaseStore) liveIndexLocked(indexes map[string]*memoryLeaseIndex, key string, now time.Time, create bool) *memoryLeaseIndex {
	index := indexes[key]
	if index != nil && !index.expiresAt.After(now) {
		delete(indexes, key)
		index = nil
	}
	if index == nil && create {
		index = &memoryLeaseIndex{members: make(map[string]time.Time), expiresAt: now.Add(LeaseIndexTTL)}
		indexes[key] = index
	}
	return index
}
