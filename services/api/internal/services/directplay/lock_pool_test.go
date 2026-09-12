package directplay

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"io"
	"strings"
	"sync"
	"testing"
	"time"
)

// auditLockDB implements only session advisory-lock results; connection
// acquisition, limits and wait cancellation use the real database/sql pool.
type auditLockDB struct {
	mu          sync.Mutex
	next        int64
	owner       int64
	waiting     chan struct{}
	failAcquire bool
}
type auditConnector struct{ state *auditLockDB }
type auditDriver struct{ state *auditLockDB }
type auditConn struct {
	state *auditLockDB
	id    int64
}
type auditRows struct {
	value bool
	sent  bool
}

// Connect gives database/sql an in-process physical connection for pool tests.
func (c auditConnector) Connect(context.Context) (driver.Conn, error) { return c.state.open(), nil }

// Driver supplies the connector's driver without a global SQL registration.
func (c auditConnector) Driver() driver.Driver { return auditDriver{c.state} }

// Open shares fake database state through the driver's fallback path.
func (d auditDriver) Open(string) (driver.Conn, error) { return d.state.open(), nil }

// open assigns physical connection identities for session lock ownership.
func (s *auditLockDB) open() *auditConn {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.next++
	return &auditConn{state: s, id: s.next}
}

// Prepare rejects SQL paths outside the fixed advisory-lock test contract.
func (c *auditConn) Prepare(string) (driver.Stmt, error) { return nil, errors.New("unsupported") }

// Begin rejects unexpected transactions; these tests exercise pool admission.
func (c *auditConn) Begin() (driver.Tx, error) { return nil, errors.New("unsupported") }

// Close releases simulated session locks when the physical connection closes.
func (c *auditConn) Close() error {
	c.state.mu.Lock()
	defer c.state.mu.Unlock()
	if c.state.owner == c.id {
		c.state.owner = 0
	}
	return nil
}

// QueryContext emulates both lock operations and a lost acquire response.
func (c *auditConn) QueryContext(_ context.Context, query string, _ []driver.NamedValue) (driver.Rows, error) {
	c.state.mu.Lock()
	defer c.state.mu.Unlock()
	switch {
	case strings.Contains(query, "pg_try_advisory_lock"):
		if c.state.failAcquire {
			c.state.failAcquire = false
			c.state.owner = c.id
			return nil, errors.New("lock response lost")
		}
		if c.state.owner == 0 || c.state.owner == c.id {
			c.state.owner = c.id
			return &auditRows{value: true}, nil
		}
		select {
		case c.state.waiting <- struct{}{}:
		default:
		}
		return &auditRows{value: false}, nil
	case strings.Contains(query, "pg_advisory_unlock"):
		unlocked := c.state.owner == c.id
		if unlocked {
			c.state.owner = 0
		}
		return &auditRows{value: unlocked}, nil
	default:
		return nil, errors.New("unexpected query")
	}
}

// Columns supplies the scalar boolean shape expected by the production scanner.
func (r *auditRows) Columns() []string { return []string{"result"} }

// Close owns no resources; the containing connection retains the session lock.
func (r *auditRows) Close() error { return nil }

// Next emits exactly one result before ending the fake result set.
func (r *auditRows) Next(dest []driver.Value) error {
	if r.sent {
		return io.EOF
	}
	r.sent = true
	dest[0] = r.value
	return nil
}

// TestLockWaiterReturnsConnectionBetweenAttempts leaves the shared SQL pool
// available for the holder's task writes while another locker polls.
func TestLockWaiterReturnsConnectionBetweenAttempts(t *testing.T) {
	state := &auditLockDB{waiting: make(chan struct{}, 1)}
	database := sql.OpenDB(auditConnector{state})
	defer database.Close()
	database.SetMaxOpenConns(2)
	holderLocker := &postgresTaskLocker{database: database, pollInterval: time.Millisecond}
	waiterLocker := &postgresTaskLocker{database: database, pollInterval: time.Millisecond}
	holder, err := holderLocker.Acquire(context.Background(), "account", directPlaySourceSHA1, 1024)
	if err != nil {
		t.Fatal(err)
	}
	defer holder.Release()
	waitCtx, cancelWait := context.WithCancel(context.Background())
	defer cancelWait()
	done := make(chan error, 1)
	go func() {
		lock, err := waiterLocker.Acquire(waitCtx, "account", directPlaySourceSHA1, 1024)
		if lock != nil {
			_ = lock.Release()
		}
		done <- err
	}()
	select {
	case <-state.waiting:
	case <-time.After(time.Second):
		t.Fatal("waiter never attempted lock")
	}
	queryCtx, cancelQuery := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancelQuery()
	conn, err := database.Conn(queryCtx)
	if err != nil {
		t.Errorf("lock waiter starved task SQL: %v", err)
	} else {
		_ = conn.Close()
	}
	cancelWait()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("waiter error=%v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("waiter did not cancel")
	}
}

// TestContentLockAdmissionReservesTaskConnection verifies that distinct local
// content locks cannot take the last connection needed by the holder's writes.
func TestContentLockAdmissionReservesTaskConnection(t *testing.T) {
	state := &auditLockDB{waiting: make(chan struct{}, 1)}
	database := sql.OpenDB(auditConnector{state})
	defer database.Close()
	database.SetMaxOpenConns(2)
	locker := &postgresTaskLocker{database: database, pollInterval: time.Millisecond}
	holder, err := locker.Acquire(context.Background(), "first", directPlaySourceSHA1, 1024)
	if err != nil {
		t.Fatal(err)
	}
	defer holder.Release()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		lock, err := locker.Acquire(ctx, "second", directPlaySourceSHA1, 1024)
		if lock != nil {
			_ = lock.Release()
		}
		done <- err
	}()
	deadline := time.Now().Add(time.Second)
	for {
		locker.gate.mu.Lock()
		keys := len(locker.gate.entries)
		locker.gate.mu.Unlock()
		if keys == 2 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("second key did not queue")
		}
		time.Sleep(time.Millisecond)
	}
	queryCtx, cancelQuery := context.WithTimeout(context.Background(), time.Second)
	defer cancelQuery()
	conn, err := database.Conn(queryCtx)
	if err != nil {
		t.Fatal(err)
	}
	_ = conn.Close()
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("waiter=%v", err)
	}
	if err := holder.Release(); err != nil {
		t.Fatal(err)
	}
	locker.gate.mu.Lock()
	keys := len(locker.gate.entries)
	locker.gate.mu.Unlock()
	if keys != 0 || len(locker.slots) != 0 {
		t.Fatalf("idle admission leaked: keys=%d slots=%d", keys, len(locker.slots))
	}
}

// TestFailedLockResponseDiscardsUncertainSession prevents an acquired server
// lock with a lost response from returning an unknown session to the pool.
func TestFailedLockResponseDiscardsUncertainSession(t *testing.T) {
	state := &auditLockDB{waiting: make(chan struct{}, 1), failAcquire: true}
	database := sql.OpenDB(auditConnector{state})
	defer database.Close()
	database.SetMaxOpenConns(2)
	locker := &postgresTaskLocker{database: database, pollInterval: time.Millisecond}
	if lock, err := locker.Acquire(context.Background(), "account", directPlaySourceSHA1, 1024); !errors.Is(err, ErrLockUnavailable) || lock != nil {
		t.Fatalf("lock=%v error=%v", lock, err)
	}
	state.mu.Lock()
	owner := state.owner
	state.mu.Unlock()
	if owner != 0 {
		t.Fatal("uncertain lock session was returned to the pool")
	}
	lock, err := locker.Acquire(context.Background(), "account", directPlaySourceSHA1, 1024)
	if err != nil {
		t.Fatal(err)
	}
	if err := lock.Release(); err != nil {
		t.Fatal(err)
	}
}
