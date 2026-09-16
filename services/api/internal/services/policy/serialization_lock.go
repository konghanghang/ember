package policy

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"hash/crc32"
	"log"
	"strings"
	"time"

	"gorm.io/gorm"
)

const (
	policySyncAdvisoryLockNamespace int32 = 0x456d4250
	defaultPolicySyncLockRetry            = 50 * time.Millisecond
	defaultPolicySyncTotalTimeout         = 2 * time.Minute
	policySyncUnlockTimeout               = 5 * time.Second
)

var errPolicySyncLockBusy = errors.New("Emby Policy 用户同步正在执行")
var errPolicySyncConnectionCapacity = errors.New("Emby Policy 用户锁持有者需要预留至少一个数据库连接")

type policySyncLocker interface {
	WithUserLock(ctx context.Context, database *gorm.DB, userID, reason string, fn func(*gorm.DB) error) error
}

type postgresPolicySyncLocker struct {
	retryInterval time.Duration
}

// WithUserLock serializes one user's full Policy sync on a PostgreSQL session advisory lock.
// Failed try-lock attempts return the connection to the pool before waiting, and an existing
// transaction handle is executed in place to avoid nested connection acquisition.
func (locker postgresPolicySyncLocker) WithUserLock(
	ctx context.Context,
	database *gorm.DB,
	userID string,
	reason string,
	fn func(*gorm.DB) error,
) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if database == nil {
		return errors.New("Policy 服务未配置数据库")
	}
	if fn == nil {
		return errors.New("Policy 同步函数不能为空")
	}
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return errors.New("用户不能为空")
	}
	if isTransactionalGormDB(database) {
		return locker.withTransactionUserLock(ctx, database, userID, reason, fn)
	}

	retryInterval := locker.retryInterval
	if retryInterval <= 0 {
		retryInterval = defaultPolicySyncLockRetry
	}
	keyA, keyB := policySyncAdvisoryLockKeys(userID)
	startedAt := time.Now()
	attempts := 0
	for {
		attempts++
		locked := false
		err := database.WithContext(ctx).Connection(func(conn *gorm.DB) error {
			if err := conn.Raw("SELECT pg_try_advisory_lock(?, ?)", keyA, keyB).Scan(&locked).Error; err != nil {
				discardPolicySyncConnection(conn)
				return normalizePolicyError("获取 Emby Policy 用户锁失败", err)
			}
			if !locked {
				return nil
			}
			waited := time.Since(startedAt)
			if attempts > 1 {
				log.Printf("[Policy] 已获得 Emby Policy 用户锁: userID=%s reason=%s attempts=%d waited=%s",
					userID, strings.TrimSpace(reason), attempts, waited)
			}
			return locker.executeSessionLockedBody(ctx, database, conn, keyA, keyB, userID, reason, fn)
		})
		if err != nil {
			return err
		}
		if locked {
			return nil
		}
		timer := time.NewTimer(retryInterval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return normalizePolicyError("等待 Emby Policy 用户锁失败", ctx.Err())
		case <-timer.C:
		}
	}
}

// withTransactionUserLock uses a transaction-scoped advisory lock when callers
// pass an existing *sql.Tx, so the sync path does not silently bypass mutual exclusion.
func (locker postgresPolicySyncLocker) withTransactionUserLock(
	ctx context.Context,
	tx *gorm.DB,
	userID string,
	reason string,
	fn func(*gorm.DB) error,
) error {
	keyA, keyB := policySyncAdvisoryLockKeys(userID)
	locked := false
	if err := tx.WithContext(ctx).Raw("SELECT pg_try_advisory_xact_lock(?, ?)", keyA, keyB).Scan(&locked).Error; err != nil {
		return normalizePolicyError("获取 Emby Policy 事务用户锁失败", err)
	}
	if !locked {
		log.Printf("[Policy] Emby Policy 事务用户锁正忙: userID=%s reason=%s", userID, strings.TrimSpace(reason))
		return errPolicySyncLockBusy
	}
	if err := ensurePolicySyncSpareConnection(tx); err != nil {
		return err
	}
	return fn(tx.WithContext(ctx))
}

// executeSessionLockedBody keeps unlock in a defer so panics cannot return a
// still-locked session to the pool; the original panic is propagated.
func (locker postgresPolicySyncLocker) executeSessionLockedBody(
	ctx context.Context,
	rootDB *gorm.DB,
	conn *gorm.DB,
	keyA int32,
	keyB int32,
	userID string,
	reason string,
	fn func(*gorm.DB) error,
) (err error) {
	defer func() {
		unlockErr := unlockPolicySyncSessionLock(conn, keyA, keyB, userID, reason)
		if recovered := recover(); recovered != nil {
			if unlockErr != nil {
				log.Printf("[Policy] panic 前释放 Emby Policy 用户锁失败: userID=%s reason=%s err=%v", userID, strings.TrimSpace(reason), unlockErr)
			}
			panic(recovered)
		}
		if unlockErr != nil {
			err = errors.Join(err, unlockErr)
		}
	}()
	if err := ensurePolicySyncSpareConnection(rootDB); err != nil {
		return err
	}
	return fn(conn.WithContext(ctx))
}

// ensurePolicySyncSpareConnection prevents known deadlocks where the held
// advisory-lock session is the only available pooled connection but the real
// Emby client still refreshes config through the shared db.DB.
func ensurePolicySyncSpareConnection(database *gorm.DB) error {
	sqlDB, err := database.DB()
	if err != nil {
		return normalizePolicyError("读取 Emby Policy 数据库连接池状态失败", err)
	}
	stats := sqlDB.Stats()
	if stats.MaxOpenConnections > 0 && stats.InUse >= stats.MaxOpenConnections {
		log.Printf("[Policy] Emby Policy 同步连接池容量不足: maxOpen=%d inUse=%d waitCount=%d",
			stats.MaxOpenConnections, stats.InUse, stats.WaitCount)
		return errPolicySyncConnectionCapacity
	}
	return nil
}

// isTransactionalGormDB detects callers that pass an active transaction handle
// so policy sync uses a transaction-scoped advisory lock instead of a session lock.
func isTransactionalGormDB(database *gorm.DB) bool {
	if database == nil || database.Statement == nil {
		return false
	}
	_, ok := database.Statement.ConnPool.(*sql.Tx)
	return ok
}

// unlockPolicySyncSessionLock releases a session advisory lock using a fresh
// timeout; failures mark the physical connection bad so it cannot re-enter the pool locked.
func unlockPolicySyncSessionLock(conn *gorm.DB, keyA, keyB int32, userID, reason string) error {
	unlockCtx, cancel := context.WithTimeout(context.Background(), policySyncUnlockTimeout)
	defer cancel()

	var released bool
	if err := conn.WithContext(unlockCtx).Raw("SELECT pg_advisory_unlock(?, ?)", keyA, keyB).Scan(&released).Error; err != nil {
		discardPolicySyncConnection(conn)
		log.Printf("[Policy] 释放 Emby Policy 用户锁失败: userID=%s reason=%s err=%v", userID, strings.TrimSpace(reason), err)
		return normalizePolicyError("释放 Emby Policy 用户锁失败", err)
	}
	if !released {
		discardPolicySyncConnection(conn)
		err := errors.New("Emby Policy 用户锁释放结果异常")
		log.Printf("[Policy] Emby Policy 用户锁释放结果异常: userID=%s reason=%s released=false", userID, strings.TrimSpace(reason))
		return err
	}
	return nil
}

// discardPolicySyncConnection asks database/sql to drop the physical connection
// when lock state is unknown or unlock failed.
func discardPolicySyncConnection(database *gorm.DB) {
	if database == nil || database.Statement == nil || database.Statement.ConnPool == nil {
		return
	}
	rawConn, ok := database.Statement.ConnPool.(interface {
		Raw(func(driverConn any) error) error
	})
	if !ok {
		return
	}
	_ = rawConn.Raw(func(any) error {
		return driver.ErrBadConn
	})
}

// policySyncOperationContext derives a bounded sync context from the request or
// DB context so production lock waits and SQL work cannot run forever.
func policySyncOperationContext(database *gorm.DB) (context.Context, context.CancelFunc) {
	base := policySyncDBContext(database)
	return context.WithTimeout(base, defaultPolicySyncTotalTimeout)
}

// policySyncDBContext returns the context currently attached to the DB handle,
// falling back to Background for fire-and-forget callers without a parent context.
func policySyncDBContext(database *gorm.DB) context.Context {
	if database != nil && database.Statement != nil && database.Statement.Context != nil {
		return database.Statement.Context
	}
	return context.Background()
}

// policySyncAdvisoryLockKeys maps one Ember user ID into the Policy advisory-lock namespace.
func policySyncAdvisoryLockKeys(userID string) (int32, int32) {
	return policySyncAdvisoryLockNamespace, int32(crc32.ChecksumIEEE([]byte(userID)))
}
