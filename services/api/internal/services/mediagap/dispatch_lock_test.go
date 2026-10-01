package mediagap

import (
	"context"
	"errors"
	"github.com/DATA-DOG/go-sqlmock"
	"testing"
)

// TestDispatchSessionLock SQL 合同覆盖跨副本争锁、成功解锁及异常清理，不连接 PostgreSQL。
func TestDispatchSessionLock(t *testing.T) {
	for _, mode := range []string{"busy", "success", "unlock-error", "acquire-error"} {
		t.Run(mode, func(t *testing.T) {
			mock := newGapSQLMock(t)
			key := "ember:media-gap:dispatch:gap"
			acquire := mock.ExpectQuery(`SELECT pg_try_advisory_lock\(hashtextextended\(\$1, 0\)\)`).WithArgs(key)
			if mode == "acquire-error" {
				acquire.WillReturnError(errors.New("canceled"))
			} else {
				acquire.WillReturnRows(sqlmock.NewRows([]string{"locked"}).AddRow(mode != "busy"))
			}
			release, err := acquireGapDispatchLock(context.Background(), "gap")
			if mode == "busy" {
				if !errors.Is(err, ErrMediaGapStateConflict) {
					t.Fatal(err)
				}
				return
			}
			if mode == "acquire-error" {
				if err == nil {
					t.Fatal("expected lock failure")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			unlock := mock.ExpectQuery(`SELECT pg_advisory_unlock\(hashtextextended\(\$1, 0\)\)`).WithArgs(key)
			if mode == "unlock-error" {
				unlock.WillReturnError(errors.New("connection failed"))
			} else {
				unlock.WillReturnRows(sqlmock.NewRows([]string{"unlocked"}).AddRow(true))
			}
			release()
		})
	}
}

// TestScanLockReleaseAfterCancellation 使用已取消的获取上下文仍能独立解锁，重复释放无副作用。
func TestScanLockReleaseAfterCancellation(t *testing.T) {
	mock := newGapSQLMock(t)
	ctx, cancel := context.WithCancel(context.Background())
	mock.ExpectQuery(`SELECT pg_try_advisory_lock\(\$1\)`).WithArgs(scanLockKey).WillReturnRows(sqlmock.NewRows([]string{"locked"}).AddRow(true))
	handle, err := tryAcquireScanLock(ctx)
	if err != nil {
		t.Fatal(err)
	}
	cancel()
	mock.ExpectQuery(`SELECT pg_advisory_unlock\(\$1\)`).WithArgs(scanLockKey).WillReturnRows(sqlmock.NewRows([]string{"unlocked"}).AddRow(true))
	handle.Release()
	handle.Release()
}
