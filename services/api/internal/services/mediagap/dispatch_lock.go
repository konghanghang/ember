package mediagap

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"fmt"
	"log"
	"time"

	"github.com/konghang/ember/backend/internal/db"
)

// acquireGapDispatchLock 持有工单级 session 锁直到外部请求及回写结束，覆盖所有 API 副本。
func acquireGapDispatchLock(ctx context.Context, id string) (func(), error) {
	database, err := db.DB.DB()
	if err != nil {
		return nil, err
	}
	conn, err := database.Conn(ctx)
	if err != nil {
		return nil, err
	}
	key := "ember:media-gap:dispatch:" + id
	var acquired bool
	if err = conn.QueryRowContext(ctx, "SELECT pg_try_advisory_lock(hashtextextended($1, 0))", key).Scan(&acquired); err != nil {
		// 请求取消时无法确认服务器是否已加锁，丢弃 session，不能归还池复用。
		_ = conn.Raw(func(interface{}) error { return driver.ErrBadConn })
		_ = conn.Close()
		return nil, fmt.Errorf("获取下发锁失败: %w", err)
	}
	if !acquired {
		_ = conn.Close()
		return nil, ErrMediaGapStateConflict
	}
	return func() { releaseSessionLock(conn, "SELECT pg_advisory_unlock(hashtextextended($1, 0))", key) }, nil
}

// releaseSessionLock 解锁失败时销毁物理连接，禁止把仍持有 session 锁的连接放回池。
func releaseSessionLock(conn *sql.Conn, query string, key interface{}) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var unlocked bool
	err := conn.QueryRowContext(ctx, query, key).Scan(&unlocked)
	if err != nil || !unlocked {
		log.Printf("[MediaGap] advisory lock 释放失败，丢弃连接 error=%v unlocked=%t", err, unlocked)
		_ = conn.Raw(func(interface{}) error { return driver.ErrBadConn })
	}
	_ = conn.Close()
}
