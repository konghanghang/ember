package policy

import (
	"context"
	"database/sql/driver"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/konghang/ember/backend/internal/models"
	embytokenpkg "github.com/konghang/ember/backend/internal/services/embytoken"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestApplyEffectiveUserPolicySerializesAcrossServicesAndReloadsUserState(t *testing.T) {
	database, mock, closeDB := newPolicySQLMockDB(t, 4)
	defer closeDB()

	locker := newOrderedPolicyLocker()
	client := &recordingPolicyClient{raw: map[string]any{"SimultaneousStreamLimit": 1, "IsAdministrator": false}}
	serviceA := newTestSerializedPolicyService(database, client, locker)
	serviceB := newTestSerializedPolicyService(database, client, locker)

	expiredAt := time.Now().UTC().Add(-time.Hour)
	renewedAt := time.Now().UTC().Add(time.Hour)
	expectEffectivePolicySync(mock, "user_1", "emby_1", "VIP_A", &expiredAt, true, false)
	expectEffectivePolicySync(mock, "user_1", "emby_1", "VIP_A", &renewedAt, false, false)

	errs := make(chan error, 2)
	go func() { errs <- serviceA.ApplyEffectiveUserPolicy("user_1", "renewal_race_a") }()
	go func() { errs <- serviceB.ApplyEffectiveUserPolicy("user_1", "renewal_race_b") }()

	locker.releaseNext()
	locker.releaseNext()
	for i := 0; i < 2; i++ {
		if err := <-errs; err != nil {
			t.Fatalf("ApplyEffectiveUserPolicy() error = %v", err)
		}
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations: %v", err)
	}
	if got := client.finalDisabled(); got {
		t.Fatalf("expected renewed user's final remote IsDisabled=false, got %t patches=%+v", got, client.patchesSnapshot())
	}
	patches := client.patchesSnapshot()
	if len(patches) != 2 || patches[0] != true || patches[1] != false {
		t.Fatalf("expected stale then refreshed policy writes, got %+v", patches)
	}
}

func TestApplyEffectiveUserPolicySerializesRenewedFirstOrder(t *testing.T) {
	database, mock, closeDB := newPolicySQLMockDB(t, 4)
	defer closeDB()

	locker := newOrderedPolicyLocker()
	client := &recordingPolicyClient{raw: map[string]any{"SimultaneousStreamLimit": 1, "IsAdministrator": false}}
	serviceA := newTestSerializedPolicyService(database, client, locker)
	serviceB := newTestSerializedPolicyService(database, client, locker)

	renewedAt := time.Now().UTC().Add(time.Hour)
	expectEffectivePolicySync(mock, "user_1", "emby_1", "VIP_A", &renewedAt, false, false)
	expectEffectivePolicySync(mock, "user_1", "emby_1", "VIP_A", &renewedAt, false, false)

	errs := make(chan error, 2)
	go func() { errs <- serviceB.ApplyEffectiveUserPolicy("user_1", "renewal_race_b") }()
	go func() { errs <- serviceA.ApplyEffectiveUserPolicy("user_1", "renewal_race_a") }()

	locker.releaseNext()
	locker.releaseNext()
	for i := 0; i < 2; i++ {
		if err := <-errs; err != nil {
			t.Fatalf("ApplyEffectiveUserPolicy() error = %v", err)
		}
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations: %v", err)
	}
	patches := client.patchesSnapshot()
	if len(patches) != 2 || patches[0] != false || patches[1] != false {
		t.Fatalf("expected both lock orders to write renewed policy, got %+v", patches)
	}
}

func TestPostgresPolicySyncLockerReturnsConnectionBetweenTryLockAttempts(t *testing.T) {
	database, mock, closeDB := newPolicySQLMockDB(t, 2)
	defer closeDB()

	keyA, keyB := policySyncAdvisoryLockKeys("user_1")
	mock.ExpectQuery(regexp.QuoteMeta("SELECT pg_try_advisory_lock($1, $2)")).
		WithArgs(keyA, keyB).
		WillReturnRows(sqlmock.NewRows([]string{"locked"}).AddRow(false))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT pg_try_advisory_lock($1, $2)")).
		WithArgs(keyA, keyB).
		WillReturnRows(sqlmock.NewRows([]string{"locked"}).AddRow(true))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT pg_advisory_unlock($1, $2)")).
		WithArgs(keyA, keyB).
		WillReturnRows(sqlmock.NewRows([]string{"released"}).AddRow(true))

	called := false
	locker := postgresPolicySyncLocker{retryInterval: time.Millisecond}
	if err := locker.WithUserLock(context.Background(), database, "user_1", "small_pool", func(*gorm.DB) error {
		called = true
		return nil
	}); err != nil {
		t.Fatalf("WithUserLock() error = %v", err)
	}
	if !called {
		t.Fatal("expected lock body to run after second try-lock")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations: %v", err)
	}
}

func TestPostgresPolicySyncLockerFailsWhenNoSpareConnectionForEmbyConfigRefresh(t *testing.T) {
	database, mock, closeDB := newPolicySQLMockDB(t, 1)
	defer closeDB()

	keyA, keyB := policySyncAdvisoryLockKeys("user_1")
	mock.ExpectQuery(regexp.QuoteMeta("SELECT pg_try_advisory_lock($1, $2)")).
		WithArgs(keyA, keyB).
		WillReturnRows(sqlmock.NewRows([]string{"locked"}).AddRow(true))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT pg_advisory_unlock($1, $2)")).
		WithArgs(keyA, keyB).
		WillReturnRows(sqlmock.NewRows([]string{"released"}).AddRow(true))

	err := postgresPolicySyncLocker{}.WithUserLock(context.Background(), database, "user_1", "max_open_one", func(*gorm.DB) error {
		t.Fatal("lock body must not run when no spare connection is available")
		return nil
	})
	if !errors.Is(err, errPolicySyncConnectionCapacity) {
		t.Fatalf("expected capacity error, got %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations: %v", err)
	}
}

func TestPostgresPolicySyncLockerReturnsErrorWhenAcquireResponseIsUnknown(t *testing.T) {
	database, mock, closeDB := newPolicySQLMockDB(t, 1)
	defer closeDB()

	keyA, keyB := policySyncAdvisoryLockKeys("user_1")
	mock.ExpectQuery(regexp.QuoteMeta("SELECT pg_try_advisory_lock($1, $2)")).
		WithArgs(keyA, keyB).
		WillReturnError(errors.New("network lost after acquire"))

	locker := postgresPolicySyncLocker{retryInterval: time.Millisecond}
	err := locker.WithUserLock(context.Background(), database, "user_1", "acquire_unknown", func(*gorm.DB) error {
		t.Fatal("lock body must not run when acquire result is unknown")
		return nil
	})
	if err == nil || !strings.Contains(err.Error(), "获取 Emby Policy 用户锁失败") {
		t.Fatalf("expected acquire error, got %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations: %v", err)
	}
}

func TestPostgresPolicySyncLockerUnlocksBeforePropagatingPanic(t *testing.T) {
	database, mock, closeDB := newPolicySQLMockDB(t, 2)
	defer closeDB()

	keyA, keyB := policySyncAdvisoryLockKeys("user_1")
	mock.ExpectQuery(regexp.QuoteMeta("SELECT pg_try_advisory_lock($1, $2)")).
		WithArgs(keyA, keyB).
		WillReturnRows(sqlmock.NewRows([]string{"locked"}).AddRow(true))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT pg_advisory_unlock($1, $2)")).
		WithArgs(keyA, keyB).
		WillReturnRows(sqlmock.NewRows([]string{"released"}).AddRow(true))

	defer func() {
		recovered := recover()
		if recovered != "boom" {
			t.Fatalf("expected original panic to propagate, got %v", recovered)
		}
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Fatalf("sql expectations: %v", err)
		}
	}()
	_ = postgresPolicySyncLocker{}.WithUserLock(context.Background(), database, "user_1", "panic", func(*gorm.DB) error {
		panic("boom")
	})
}

func TestPostgresPolicySyncLockerCancellationWhileWaiting(t *testing.T) {
	database, mock, closeDB := newPolicySQLMockDB(t, 1)
	defer closeDB()

	keyA, keyB := policySyncAdvisoryLockKeys("user_1")
	mock.ExpectQuery(regexp.QuoteMeta("SELECT pg_try_advisory_lock($1, $2)")).
		WithArgs(keyA, keyB).
		WillReturnRows(sqlmock.NewRows([]string{"locked"}).AddRow(false))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		time.Sleep(10 * time.Millisecond)
		cancel()
	}()

	locker := postgresPolicySyncLocker{retryInterval: time.Hour}
	err := locker.WithUserLock(ctx, database, "user_1", "cancelled", func(*gorm.DB) error {
		t.Fatal("lock body must not run after cancellation")
		return nil
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations: %v", err)
	}
}

func TestPostgresPolicySyncLockerReturnsUnlockError(t *testing.T) {
	tests := []struct {
		name        string
		unlockRows  *sqlmock.Rows
		unlockErr   error
		wantMessage string
	}{
		{
			name:        "unlock error",
			unlockErr:   errors.New("unlock response lost"),
			wantMessage: "释放 Emby Policy 用户锁失败",
		},
		{
			name:        "unlock false",
			unlockRows:  sqlmock.NewRows([]string{"released"}).AddRow(false),
			wantMessage: "Emby Policy 用户锁释放结果异常",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			database, mock, closeDB := newPolicySQLMockDB(t, 2)
			defer closeDB()

			keyA, keyB := policySyncAdvisoryLockKeys("user_1")
			mock.ExpectQuery(regexp.QuoteMeta("SELECT pg_try_advisory_lock($1, $2)")).
				WithArgs(keyA, keyB).
				WillReturnRows(sqlmock.NewRows([]string{"locked"}).AddRow(true))
			unlock := mock.ExpectQuery(regexp.QuoteMeta("SELECT pg_advisory_unlock($1, $2)")).
				WithArgs(keyA, keyB)
			if tt.unlockErr != nil {
				unlock.WillReturnError(tt.unlockErr)
			} else {
				unlock.WillReturnRows(tt.unlockRows)
			}

			called := false
			err := postgresPolicySyncLocker{}.WithUserLock(context.Background(), database, "user_1", "unlock_failure", func(*gorm.DB) error {
				called = true
				return nil
			})
			if !called {
				t.Fatal("expected lock body to run before unlock failure")
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantMessage) {
				t.Fatalf("expected unlock error containing %q, got %v", tt.wantMessage, err)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatalf("sql expectations: %v", err)
			}
		})
	}
}

func TestApplyEffectiveUserPolicyRevokesTokensInsideSerializedSync(t *testing.T) {
	database, mock, closeDB := newPolicySQLMockDB(t, 4)
	defer closeDB()

	client := &recordingPolicyClient{raw: map[string]any{"SimultaneousStreamLimit": 1, "IsAdministrator": false}}
	service := NewServiceWithDB(database, client)
	service.syncLocker = passthroughPolicyLocker{}

	expiredAt := time.Now().UTC().Add(-time.Hour)
	expectEffectivePolicySync(mock, "user_1", "emby_1", "VIP_A", &expiredAt, true, true)

	if err := service.ApplyEffectiveUserPolicy("user_1", "expired_user_check"); err != nil {
		t.Fatalf("ApplyEffectiveUserPolicy() error = %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations: %v", err)
	}
	if patches := client.patchesSnapshot(); len(patches) != 1 || patches[0] != true {
		t.Fatalf("expected one disabled policy write, got %+v", patches)
	}
}

func TestApplyEffectiveUserPolicyPassesBoundedContextToTokenRevoker(t *testing.T) {
	database, mock, closeDB := newPolicySQLMockDB(t, 4)
	defer closeDB()

	client := &recordingPolicyClient{raw: map[string]any{"SimultaneousStreamLimit": 1, "IsAdministrator": false}}
	service := NewServiceWithDB(database, client)
	service.syncLocker = passthroughPolicyLocker{}
	service.defaultRevoker = false
	var revokerCtx context.Context
	service.revokeUserTokens = func(ctx context.Context, _ string, _ embytokenpkg.RevokeReason, _ string) (int64, error) {
		revokerCtx = ctx
		return 0, nil
	}

	expiredAt := time.Now().UTC().Add(-time.Hour)
	expectEffectivePolicySync(mock, "user_1", "emby_1", "VIP_A", &expiredAt, true, false)

	if err := service.ApplyEffectiveUserPolicy("user_1", "expired_user_check"); err != nil {
		t.Fatalf("ApplyEffectiveUserPolicy() error = %v", err)
	}
	if _, ok := revokerCtx.Deadline(); !ok {
		t.Fatal("expected token revoker context to inherit Policy sync deadline")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations: %v", err)
	}
}

func TestApplyEffectiveUserPolicyPassesBoundedProductionContextToLocker(t *testing.T) {
	database, _, closeDB := newPolicySQLMockDB(t, 1)
	defer closeDB()

	locker := &capturingPolicyLocker{}
	service := NewServiceWithDB(database, &recordingPolicyClient{})
	service.syncLocker = locker

	if err := service.ApplyEffectiveUserPolicy("user_1", "deadline_check"); err != nil {
		t.Fatalf("ApplyEffectiveUserPolicy() error = %v", err)
	}
	deadline, ok := locker.ctx.Deadline()
	if !ok {
		t.Fatal("expected production Policy sync context to have deadline")
	}
	remaining := time.Until(deadline)
	if remaining <= 0 || remaining > defaultPolicySyncTotalTimeout {
		t.Fatalf("unexpected deadline remaining=%s", remaining)
	}
}

func TestProcessPendingEmbyPolicySyncTasksCarriesContextIntoLockWait(t *testing.T) {
	database, mock, closeDB := newPolicySQLMockDB(t, 4)
	defer closeDB()

	ctx, cancel := context.WithCancel(context.Background())
	locker := &cancelingPolicyLocker{cancel: cancel, entered: make(chan struct{}, 1)}
	client := &recordingPolicyClient{raw: map[string]any{"SimultaneousStreamLimit": 1, "IsAdministrator": false}}
	service := NewServiceWithDB(database, client)
	service.syncLocker = locker
	service.defaultRevoker = false
	service.revokeUserTokens = func(context.Context, string, embytokenpkg.RevokeReason, string) (int64, error) {
		t.Fatal("token revoker must not run after worker context cancellation")
		return 0, nil
	}

	now := time.Now().UTC()
	expectWorkerClaimOneTask(mock, now)
	expectWorkerFinishTaskFailed(mock)

	result, err := service.ProcessPendingEmbyPolicySyncTasks(ctx, 1)
	if err != nil {
		t.Fatalf("ProcessPendingEmbyPolicySyncTasks() error = %v", err)
	}
	if result == nil || result.Claimed != 1 || result.Failed != 1 || result.Succeeded != 0 {
		t.Fatalf("unexpected worker result: %+v", result)
	}
	select {
	case <-locker.entered:
	default:
		t.Fatal("expected worker to enter locker before cancellation")
	}
	if patches := client.patchesSnapshot(); len(patches) != 0 {
		t.Fatalf("expected no Emby patch after cancellation, got %+v", patches)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations: %v", err)
	}
}

// TestProcessPendingEmbyPolicySyncTasksCancellationLeavesWaitingTasksUnclaimed verifies
// that a canceled sync finishes its own task without reserving the rest of the queue.
func TestProcessPendingEmbyPolicySyncTasksCancellationLeavesWaitingTasksUnclaimed(t *testing.T) {
	database, mock, closeDB := newPolicySQLMockDB(t, 4)
	defer closeDB()

	ctx, cancel := context.WithCancel(context.Background())
	locker := &cancelingPolicyLocker{cancel: cancel, entered: make(chan struct{}, 1)}
	client := &recordingPolicyClient{raw: map[string]any{"SimultaneousStreamLimit": 1, "IsAdministrator": false}}
	service := NewServiceWithDB(database, client)
	service.syncLocker = locker
	service.defaultRevoker = false
	service.revokeUserTokens = func(context.Context, string, embytokenpkg.RevokeReason, string) (int64, error) {
		t.Fatal("token revoker must not run after worker context cancellation")
		return 0, nil
	}

	now := time.Now().UTC()
	expectWorkerClaimOneTask(mock, now)
	expectWorkerFinishTaskFailed(mock)

	result, err := service.ProcessPendingEmbyPolicySyncTasks(ctx, 3)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected cancellation, got %v", err)
	}
	if result == nil || result.Claimed != 1 || result.Failed != 1 || result.Succeeded != 0 {
		t.Fatalf("unexpected worker result: %+v", result)
	}
	select {
	case <-locker.entered:
	default:
		t.Fatal("expected worker to enter locker before cancellation")
	}
	if patches := client.patchesSnapshot(); len(patches) != 0 {
		t.Fatalf("expected no Emby patch after cancellation, got %+v", patches)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations: %v", err)
	}
}

// TestProcessPendingEmbyPolicySyncTasksClaimsSequentially covers cancellation after
// one completion, queue exhaustion, explicit limits, and the default limit.
func TestProcessPendingEmbyPolicySyncTasksClaimsSequentially(t *testing.T) {
	for _, tc := range []struct {
		name         string
		limit        int
		count        int
		cancelSecond bool
		empty        bool
	}{
		{name: "cancel second of three", limit: 3, count: 2, cancelSecond: true},
		{name: "explicit limit", limit: 2, count: 2},
		{name: "queue exhausted", limit: 3, count: 1, empty: true},
		{name: "empty queue", limit: 3, count: 0, empty: true},
		{name: "default limit", limit: 0, count: defaultPolicySyncWorkerLimit},
	} {
		t.Run(tc.name, func(t *testing.T) {
			database, mock, closeDB := newPolicySQLMockDB(t, 4)
			defer closeDB()
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			service := NewServiceWithDB(database, &stubPolicyClient{})
			calls := 0
			service.syncLocker = workerPolicyLockerFunc(func(ctx context.Context) error {
				calls++
				if tc.cancelSecond && calls == 2 {
					cancel()
					return ctx.Err()
				}
				return nil
			})
			mock.ExpectQuery(`SELECT \* FROM "emby_policy_sync_tasks" WHERE status = \$1 AND updated_at <= \$2`).
				WithArgs(SyncStatusProcessing, anyTime{}).WillReturnRows(policyTaskRows())
			for i := 1; i <= tc.count; i++ {
				taskID := fmt.Sprintf("task_%d", i)
				expectWorkerClaimTask(mock, time.Now().UTC(), taskID)
				status := SyncStatusSynced
				var lastError driver.Value
				if tc.cancelSecond && i == 2 {
					status = SyncStatusFailed
					lastError = context.Canceled.Error()
				}
				mock.ExpectBegin()
				mock.ExpectExec(`UPDATE "emby_policy_sync_tasks" SET "last_error"=\$1,"next_retry_at"=\$2,"status"=\$3,"updated_at"=\$4 WHERE id = \$5 AND status = \$6`).
					WithArgs(lastError, nil, status, anyTime{}, taskID, SyncStatusProcessing).
					WillReturnResult(sqlmock.NewResult(0, 1))
				mock.ExpectCommit()
			}
			if tc.empty {
				expectWorkerClaimTask(mock, time.Now().UTC(), "")
			}
			result, err := service.ProcessPendingEmbyPolicySyncTasks(ctx, tc.limit)
			wantFailed := 0
			if tc.cancelSecond {
				wantFailed = 1
				if !errors.Is(err, context.Canceled) {
					t.Fatalf("expected cancellation, got %v", err)
				}
			} else if err != nil {
				t.Fatal(err)
			}
			if result == nil || result.Claimed != tc.count || result.Failed != wantFailed || result.Succeeded != tc.count-wantFailed || calls != tc.count {
				t.Fatalf("unexpected result=%+v calls=%d", result, calls)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

type workerPolicyLockerFunc func(context.Context) error

// WithUserLock substitutes only the sync outcome so worker queue behavior is isolated.
func (fn workerPolicyLockerFunc) WithUserLock(ctx context.Context, _ *gorm.DB, _ string, _ string, _ func(*gorm.DB) error) error {
	return fn(ctx)
}

type passthroughPolicyLocker struct{}

// WithUserLock runs the body immediately while preserving the production callback shape.
func (passthroughPolicyLocker) WithUserLock(ctx context.Context, database *gorm.DB, _ string, _ string, fn func(*gorm.DB) error) error {
	return fn(database.WithContext(ctx))
}

type capturingPolicyLocker struct {
	ctx context.Context
}

// WithUserLock records the production context without executing the sync body.
func (locker *capturingPolicyLocker) WithUserLock(ctx context.Context, _ *gorm.DB, _ string, _ string, _ func(*gorm.DB) error) error {
	locker.ctx = ctx
	return nil
}

type cancelingPolicyLocker struct {
	cancel  context.CancelFunc
	entered chan struct{}
}

// WithUserLock cancels after the worker has claimed a task but before the sync body can run.
func (locker *cancelingPolicyLocker) WithUserLock(ctx context.Context, _ *gorm.DB, _ string, _ string, _ func(*gorm.DB) error) error {
	locker.entered <- struct{}{}
	locker.cancel()
	return ctx.Err()
}

type orderedPolicyLocker struct {
	mu       sync.Mutex
	waiters  []chan struct{}
	entered  chan struct{}
	unlocked chan struct{}
}

func newOrderedPolicyLocker() *orderedPolicyLocker {
	return &orderedPolicyLocker{entered: make(chan struct{}, 8), unlocked: make(chan struct{}, 8)}
}

// WithUserLock is a fake cross-service lock used to force deterministic A/B ordering.
func (locker *orderedPolicyLocker) WithUserLock(_ context.Context, database *gorm.DB, _ string, _ string, fn func(*gorm.DB) error) error {
	wait := make(chan struct{})
	locker.mu.Lock()
	locker.waiters = append(locker.waiters, wait)
	locker.mu.Unlock()
	locker.entered <- struct{}{}
	<-wait
	err := fn(database)
	locker.unlocked <- struct{}{}
	return err
}

func (locker *orderedPolicyLocker) releaseNext() {
	<-locker.entered
	locker.mu.Lock()
	wait := locker.waiters[0]
	locker.waiters = locker.waiters[1:]
	locker.mu.Unlock()
	close(wait)
	<-locker.unlocked
}

type recordingPolicyClient struct {
	mu      sync.Mutex
	raw     map[string]any
	patches []bool
}

func (client *recordingPolicyClient) GetUserPolicyRaw(string) (map[string]any, error) {
	client.mu.Lock()
	defer client.mu.Unlock()
	copyRaw := make(map[string]any, len(client.raw))
	for key, value := range client.raw {
		copyRaw[key] = value
	}
	return copyRaw, nil
}

func (client *recordingPolicyClient) PatchUserPolicyFields(_ string, sourcePolicy map[string]any, _ []string) error {
	client.mu.Lock()
	defer client.mu.Unlock()
	disabled, _ := sourcePolicy["IsDisabled"].(bool)
	client.patches = append(client.patches, disabled)
	return nil
}

func (client *recordingPolicyClient) finalDisabled() bool {
	client.mu.Lock()
	defer client.mu.Unlock()
	if len(client.patches) == 0 {
		return false
	}
	return client.patches[len(client.patches)-1]
}

func (client *recordingPolicyClient) patchesSnapshot() []bool {
	client.mu.Lock()
	defer client.mu.Unlock()
	out := make([]bool, len(client.patches))
	copy(out, client.patches)
	return out
}

func newTestSerializedPolicyService(database *gorm.DB, client embyPolicyClient, locker policySyncLocker) *Service {
	service := NewServiceWithDB(database, client)
	service.syncLocker = locker
	service.defaultRevoker = false
	service.revokeUserTokens = func(context.Context, string, embytokenpkg.RevokeReason, string) (int64, error) {
		return 0, nil
	}
	return service
}

func newPolicySQLMockDB(t *testing.T, maxOpenConns int) (*gorm.DB, sqlmock.Sqlmock, func()) {
	t.Helper()
	sqlDB, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock.New(): %v", err)
	}
	sqlDB.SetMaxOpenConns(maxOpenConns)
	database, err := gorm.Open(postgres.New(postgres.Config{Conn: sqlDB, PreferSimpleProtocol: true}), &gorm.Config{
		NowFunc: func() time.Time { return time.Now().UTC() },
	})
	if err != nil {
		_ = sqlDB.Close()
		t.Fatalf("gorm.Open(): %v", err)
	}
	return database, mock, func() { _ = sqlDB.Close() }
}

func expectEffectivePolicySync(
	mock sqlmock.Sqlmock,
	userID string,
	embyID string,
	planGroup string,
	expiresAt *time.Time,
	wantDisabled bool,
	wantTokenRevocation bool,
) {
	mock.ExpectQuery(`SELECT \* FROM "users" WHERE id = \$1 ORDER BY "users"\."id" LIMIT \$2`).
		WithArgs(userID, 1).
		WillReturnRows(userRows(userID, embyID, planGroup, expiresAt))
	mock.ExpectQuery(`SELECT \* FROM "plan_groups" WHERE key = \$1 ORDER BY "plan_groups"\."key" LIMIT \$2`).
		WithArgs(planGroup, 1).
		WillReturnRows(planGroupRows(planGroup, 7))
	mock.ExpectQuery(`SELECT \* FROM "plan_groups" WHERE key = \$1 ORDER BY "plan_groups"\."key" LIMIT \$2`).
		WithArgs(planGroup, 1).
		WillReturnRows(planGroupRows(planGroup, 7))
	mock.ExpectQuery(`SELECT \* FROM "plan_group_emby_policy_templates" WHERE plan_group_key = \$1 ORDER BY "plan_group_emby_policy_templates"\."plan_group_key" LIMIT \$2`).
		WithArgs(planGroup, 1).
		WillReturnRows(policyTemplateRows(planGroup))
	mock.ExpectQuery(`SELECT \* FROM "plan_group_media_libraries" WHERE plan_group_key = \$1 AND LOWER\(COALESCE\(library_type, ''\)\) <> \$2 ORDER BY sort_order ASC, library_name ASC, library_id ASC`).
		WithArgs(planGroup, systemCollectionLibraryType).
		WillReturnRows(mediaLibraryRows())
	mock.ExpectQuery(`SELECT \* FROM "user_media_library_preferences" WHERE user_id = \$1`).
		WithArgs(userID).
		WillReturnRows(sqlmock.NewRows([]string{"id", "user_id", "library_id", "enabled"}))
	if wantTokenRevocation {
		mock.ExpectBegin()
		mock.ExpectExec(`UPDATE "emby_access_tokens" SET "revoked_at"=\$1,"revoked_by"=\$2,"revoked_reason"=\$3,"updated_at"=\$4 WHERE user_id = \$5 AND revoked_at IS NULL`).
			WithArgs(anyTime{}, "system:policy", string(embytokenpkg.RevokeReasonEmbyDisabled), anyTime{}, userID).
			WillReturnResult(sqlmock.NewResult(0, 1))
		mock.ExpectCommit()
	}
	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE "users" SET "applied_media_library_template_version"=\$1,"emby_disabled"=\$2,"updated_at"=\$3 WHERE id = \$4`).
		WithArgs(int64(7), wantDisabled, anyTime{}, userID).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE "emby_policy_sync_tasks" SET "last_error"=\$1,"status"=\$2,"updated_at"=\$3 WHERE user_id = \$4 AND batch_id IS NULL AND status = \$5`).
		WithArgs(nil, SyncStatusSynced, anyTime{}, userID, SyncStatusFailed).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectCommit()
}

// expectWorkerClaimOneTask covers stale recovery followed by a single claim.
func expectWorkerClaimOneTask(mock sqlmock.Sqlmock, now time.Time) {
	mock.ExpectQuery(`SELECT \* FROM "emby_policy_sync_tasks" WHERE status = \$1 AND updated_at <= \$2`).
		WithArgs(SyncStatusProcessing, anyTime{}).
		WillReturnRows(policyTaskRows())
	expectWorkerClaimTask(mock, now, "task_1")
}

// expectWorkerClaimTask models a queue that yields one task, or is empty for a blank ID.
func expectWorkerClaimTask(mock sqlmock.Sqlmock, now time.Time, taskID string) {
	rows := policyTaskRows()
	if taskID != "" {
		rows.AddRow(taskID, nil, "user_1", "emby_1", "VIP_A", "worker_retry", SyncStatusPending, 1, nil, nil, now.Add(-time.Minute), now.Add(-time.Minute))
	}
	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT \* FROM "emby_policy_sync_tasks" WHERE status = \$1 AND \(next_retry_at IS NULL OR next_retry_at <= \$2\) ORDER BY next_retry_at ASC NULLS FIRST, created_at ASC LIMIT \$3 FOR UPDATE SKIP LOCKED`).
		WithArgs(SyncStatusPending, anyTime{}, 1).
		WillReturnRows(rows)
	if taskID != "" {
		mock.ExpectExec(`UPDATE "emby_policy_sync_tasks" SET "attempts"=attempts \+ 1,"last_error"=\$1,"next_retry_at"=\$2,"status"=\$3,"updated_at"=\$4 WHERE id IN \(\$5\)`).
			WithArgs(nil, nil, SyncStatusProcessing, anyTime{}, taskID).
			WillReturnResult(sqlmock.NewResult(0, 1))
	}
	mock.ExpectCommit()
}

func expectWorkerFinishTaskFailed(mock sqlmock.Sqlmock) {
	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE "emby_policy_sync_tasks" SET "last_error"=\$1,"next_retry_at"=\$2,"status"=\$3,"updated_at"=\$4 WHERE id = \$5 AND status = \$6`).
		WithArgs(sqlmock.AnyArg(), nil, SyncStatusFailed, anyTime{}, "task_1", SyncStatusProcessing).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
}

func policyTaskRows() *sqlmock.Rows {
	return sqlmock.NewRows([]string{
		"id", "batch_id", "user_id", "emby_id", "plan_group_key", "reason",
		"status", "attempts", "last_error", "next_retry_at", "created_at", "updated_at",
	})
}

func userRows(userID, embyID, planGroup string, expiresAt *time.Time) *sqlmock.Rows {
	return sqlmock.NewRows([]string{
		"id", "username", "role", "password", "email", "emby_id", "emby_disabled", "emby_access_disabled",
		"telegram_id", "plan_group", "applied_media_library_template_version", "expires_at", "is_active",
		"password_reset_required", "created_at", "updated_at",
	}).AddRow(userID, "fixture-user", "user", "", "", embyID, false, false, nil, planGroup, int64(1), expiresAt, true, false, time.Now().UTC(), time.Now().UTC())
}

func planGroupRows(key string, templateVersion int64) *sqlmock.Rows {
	return sqlmock.NewRows([]string{
		"key", "name", "description", "is_default", "sort_order", "subscription_auto_approve_daily_limit",
		"p115_playback_mode", "p115_transfer_hourly_limit", "p115_transfer_daily_limit", "media_library_template_version",
		"created_at", "updated_at",
	}).AddRow(key, "VIP A", "", false, 1, 0, models.P115PlaybackModePersonal, 5, 10, templateVersion, time.Now().UTC(), time.Now().UTC())
}

func policyTemplateRows(planGroup string) *sqlmock.Rows {
	return sqlmock.NewRows([]string{
		"plan_group_key", "simultaneous_stream_limit", "enable_content_downloading", "enable_live_tv_access",
		"enable_sync_transcoding", "enable_audio_playback_transcoding", "enable_video_playback_transcoding",
		"enable_playback_remuxing", "enable_remote_access", "created_at", "updated_at",
	}).AddRow(planGroup, 3, false, false, false, false, true, true, true, time.Now().UTC(), time.Now().UTC())
}

func mediaLibraryRows() *sqlmock.Rows {
	return sqlmock.NewRows([]string{
		"id", "plan_group_key", "library_id", "library_name", "library_type", "sort_order", "created_at", "updated_at",
	}).AddRow("lib_row_1", "VIP_A", "lib_a", "Movies", "movies", 1, time.Now().UTC(), time.Now().UTC())
}

type anyTime struct{}

func (anyTime) Match(value driver.Value) bool {
	_, ok := value.(time.Time)
	return ok
}

var _ sqlmock.Argument = anyTime{}
