package directplay

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"database/sql/driver"
	"encoding/binary"
	"errors"
	"fmt"
	"strconv"
	"sync"
	"time"

	"gorm.io/gorm"
)

const (
	// 0x454D4231 ("EMB1") is the two-key advisory-lock namespace reserved for
	// direct-play content locks, separate from one-key migration/media-gap locks.
	directPlayLockNamespace    int32 = 1162691121
	directPlayLockPollInterval       = 100 * time.Millisecond
	directPlayUnlockTimeout          = 5 * time.Second
)

type postgresTaskLocker struct {
	database     *sql.DB
	pollInterval time.Duration
	gate         requestGate
	slotsOnce    sync.Once
	slots        chan struct{}
}

func newPostgresTaskLocker(database *gorm.DB) (*postgresTaskLocker, error) {
	if database == nil {
		return nil, ErrStoreUnavailable
	}
	sqlDB, err := database.DB()
	if err != nil {
		return nil, fmt.Errorf("%w: sql_db", ErrStoreUnavailable)
	}
	return &postgresTaskLocker{database: sqlDB, pollInterval: directPlayLockPollInterval}, nil
}

// Acquire queues local contenders before borrowing SQL connections. Failed
// lock attempts return the connection; holders reserve pool room for task SQL.
func (locker *postgresTaskLocker) Acquire(ctx context.Context, playbackAccountID, sha1Value string, size int64) (taskLock, error) {
	key := directPlayLockKey(playbackAccountID, sha1Value, size)
	maxOpen := locker.database.Stats().MaxOpenConnections
	if maxOpen == 1 {
		return nil, fmt.Errorf("%w: pool requires task connection", ErrLockUnavailable)
	}
	locker.slotsOnce.Do(func() {
		if maxOpen > 1 {
			locker.slots = make(chan struct{}, maxOpen-1)
		}
	})
	releaseKey, err := locker.gate.acquire(ctx, strconv.FormatInt(int64(key), 10))
	if err != nil {
		return nil, err
	}
	if locker.slots != nil {
		select {
		case locker.slots <- struct{}{}:
		case <-ctx.Done():
			releaseKey()
			return nil, ctx.Err()
		}
	}
	release := func() {
		if locker.slots != nil {
			<-locker.slots
		}
		releaseKey()
	}
	held := false
	defer func() {
		if !held {
			release()
		}
	}()
	for {
		conn, err := locker.database.Conn(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			return nil, fmt.Errorf("%w: connection", ErrLockUnavailable)
		}
		var acquired bool
		if err := conn.QueryRowContext(ctx, "SELECT pg_try_advisory_lock($1, $2)", directPlayLockNamespace, key).Scan(&acquired); err != nil {
			// A missing response cannot prove the server did not acquire the lock.
			_ = conn.Raw(func(interface{}) error { return driver.ErrBadConn })
			_ = conn.Close()
			if ctxErr := ctx.Err(); ctxErr != nil {
				return nil, ctxErr
			}
			if errorsIsContext(err) {
				return nil, err
			}
			return nil, fmt.Errorf("%w: acquire", ErrLockUnavailable)
		}
		if acquired {
			held = true
			return &postgresTaskLock{conn: conn, namespace: directPlayLockNamespace, key: key, release: release}, nil
		}
		_ = conn.Close()
		timer := time.NewTimer(locker.pollInterval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil, ctx.Err()
		case <-timer.C:
		}
	}
}

type postgresTaskLock struct {
	conn      *sql.Conn
	namespace int32
	key       int32
	release   func()
}

// Release unlocks on the same physical connection using an independent
// timeout so a canceled request context cannot leak a session-level lock.
func (lock *postgresTaskLock) Release() error {
	if lock == nil || lock.conn == nil {
		return nil
	}
	conn := lock.conn
	lock.conn = nil
	if lock.release != nil {
		defer lock.release()
	}
	ctx, cancel := context.WithTimeout(context.Background(), directPlayUnlockTimeout)
	defer cancel()
	var unlocked bool
	err := conn.QueryRowContext(ctx, "SELECT pg_advisory_unlock($1, $2)", lock.namespace, lock.key).Scan(&unlocked)
	if err != nil || !unlocked {
		_ = conn.Raw(func(interface{}) error { return driver.ErrBadConn })
		_ = conn.Close()
		return fmt.Errorf("%w: release", ErrLockUnavailable)
	}
	if err := conn.Close(); err != nil {
		return fmt.Errorf("%w: close", ErrLockUnavailable)
	}
	return nil
}

// directPlayLockKey derives the second PostgreSQL int32 key. Hash collisions
// only over-serialize unrelated content; the exact database unique index still
// protects task identity.
func directPlayLockKey(playbackAccountID, sha1Value string, size int64) int32 {
	payload := playbackAccountID + "\x00" + sha1Value + "\x00" + strconv.FormatInt(size, 10)
	sum := sha256.Sum256([]byte(payload))
	return int32(binary.BigEndian.Uint32(sum[:4]))
}

func errorsIsContext(err error) bool {
	return errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)
}
