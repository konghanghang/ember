package mediagap

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/konghang/ember/backend/internal/db"
	"github.com/konghang/ember/backend/internal/models"
)

// scanLockKey 是缺集扫描使用的 PG advisory lock key（hashtextextended('media_gap_scan', 0) 即可，
// 这里固化一个 int64，避免每次都计算）。任意 ember 副本同时尝试持有，互斥成立。
//
// 选取规则：用 hashtextextended('media_gap_scan', 0) 在本地计算并固化。如果以后需要
// 在同一库上额外占用别的 advisory lock 命名空间，注意避免与本值冲突。
const scanLockKey int64 = 7427891025463197841

// ErrMediaGapScanInProgress 表示另一个节点（或本节点上一次未释放的连接）已持有扫描锁。
var ErrMediaGapScanInProgress = errors.New("已有缺集扫描正在执行")

// scanLockHandle 是一次成功获取的扫描锁句柄，必须在扫描结束后调用 Release。
//
// PG advisory lock 绑定在 session（连接）上：连接归还到池或断开时自动释放，
// 即便进程 crash 也由 PG 端做最终回收。
type scanLockHandle struct {
	conn *sql.Conn
	ctx  context.Context
}

// tryAcquireScanLock 尝试以非阻塞方式获取 advisory lock。
//
// 成功：返回 handle，调用方负责在扫描结束后调用 handle.Release()。
// 已被占用：返回 (nil, ErrMediaGapScanInProgress)。
// 其他错误（连接获取失败等）：返回原始 error。
func tryAcquireScanLock(ctx context.Context) (*scanLockHandle, error) {
	sqlDB, err := db.DB.DB()
	if err != nil {
		return nil, fmt.Errorf("缺集扫描锁：获取 SQL DB 失败：%w", err)
	}
	conn, err := sqlDB.Conn(ctx)
	if err != nil {
		return nil, fmt.Errorf("缺集扫描锁：获取连接失败：%w", err)
	}

	row := conn.QueryRowContext(ctx, "SELECT pg_try_advisory_lock($1)", scanLockKey)
	var acquired bool
	if err := row.Scan(&acquired); err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("缺集扫描锁：调用失败：%w", err)
	}
	if !acquired {
		_ = conn.Close()
		return nil, ErrMediaGapScanInProgress
	}
	return &scanLockHandle{conn: conn, ctx: ctx}, nil
}

// Release 释放 advisory lock 并把底层连接归还。
//
// 在 panic / 异常路径上必须 defer 调用，否则连接被持有直到归还池才释放，
// 期间其他节点会一直拿到 ErrMediaGapScanInProgress。
func (h *scanLockHandle) Release() {
	if h == nil || h.conn == nil {
		return
	}
	if _, err := h.conn.ExecContext(h.ctx, "SELECT pg_advisory_unlock($1)", scanLockKey); err != nil {
		log.Printf("[MediaGap] 释放扫描 advisory lock 失败 err=%v", err)
	}
	if err := h.conn.Close(); err != nil {
		log.Printf("[MediaGap] 释放扫描连接失败 err=%v", err)
	}
	h.conn = nil
}

// MediaGapScanRecorder 在 advisory lock 之上提供 media_gap_scans 的写入辅助。
type MediaGapScanRecorder struct{}

// NewMediaGapScanRecorder 返回 recorder 实例（无状态，调用方可复用一份）。
func NewMediaGapScanRecorder() *MediaGapScanRecorder { return &MediaGapScanRecorder{} }

// MediaGapScanLockHandleHolder 是 advisory lock 的不透明句柄，跨包传递使用。
//
// 内部封装一个 *scanLockHandle，调用方不应该直接访问内部字段；
// 只能通过 MediaGapScanRecorder 的 FinishAndReleaseHolder / Release 接口处理。
type MediaGapScanLockHandleHolder struct {
	inner *scanLockHandle
}

// AcquireAndRecord 申请 advisory lock 并写入一条 running 记录。
//
// 返回值：handle / scanID（成功时）/ error。失败时 advisory lock 已被释放，调用方无需善后。
//
// 调用方在扫描结束后必须调用 FinishAndReleaseHolder(ctx, handle, scanID, status, errMsg)。
func (r *MediaGapScanRecorder) AcquireAndRecord(ctx context.Context) (*MediaGapScanLockHandleHolder, string, error) {
	handle, err := tryAcquireScanLock(ctx)
	if err != nil {
		return nil, "", err
	}

	scan := models.MediaGapScan{
		Status:    models.MediaGapScanStatusRunning,
		NodeID:    nodeIdentifier(),
		StartedAt: time.Now().UTC(),
	}
	if err := db.DB.WithContext(ctx).Create(&scan).Error; err != nil {
		handle.Release()
		return nil, "", fmt.Errorf("写入 media_gap_scans running 记录失败：%w", err)
	}
	return &MediaGapScanLockHandleHolder{inner: handle}, scan.ID, nil
}

// FinishAndReleaseHolder 把扫描记录写到终态，然后释放 advisory lock。
func (r *MediaGapScanRecorder) FinishAndReleaseHolder(ctx context.Context, holder *MediaGapScanLockHandleHolder, scanID string, status models.MediaGapScanStatus, errMsg string) {
	defer func() {
		if holder != nil && holder.inner != nil {
			holder.inner.Release()
			holder.inner = nil
		}
	}()

	updates := map[string]interface{}{
		"status":     status,
		"finishedAt": time.Now().UTC(),
	}
	if errMsg != "" {
		if len(errMsg) > 500 {
			errMsg = errMsg[:500]
		}
		updates["errorMessage"] = errMsg
	} else {
		updates["errorMessage"] = nil
	}
	if err := db.DB.WithContext(ctx).Model(&models.MediaGapScan{}).
		Where("id = ?", scanID).
		Updates(updates).Error; err != nil {
		log.Printf("[MediaGap] 写回 media_gap_scans 终态失败 scanId=%s status=%s err=%v", scanID, status, err)
	}
}

// CleanupOldScans 删除 7 天之前的 success / failed 记录。
//
// running 记录不被清理：可能是 crash 后未释放 advisory lock 的孤儿，需要运维显式排查。
func (r *MediaGapScanRecorder) CleanupOldScans(ctx context.Context) (int64, error) {
	cutoff := time.Now().UTC().Add(-7 * 24 * time.Hour)
	result := db.DB.WithContext(ctx).
		Where(`"startedAt" < ? AND status IN ?`, cutoff, []models.MediaGapScanStatus{
			models.MediaGapScanStatusSuccess,
			models.MediaGapScanStatusFailed,
		}).
		Delete(&models.MediaGapScan{})
	return result.RowsAffected, result.Error
}

func nodeIdentifier() string {
	host, err := os.Hostname()
	if err != nil || host == "" {
		host = "unknown"
	}
	return fmt.Sprintf("%s/%d", host, os.Getpid())
}
