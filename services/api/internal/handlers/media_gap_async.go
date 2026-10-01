package handlers

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/konghang/ember/backend/internal/async"
	"github.com/konghang/ember/backend/internal/models"
	mediagappkg "github.com/konghang/ember/backend/internal/services/mediagap"
)

const mediaGapAsyncScanTimeout = 30 * time.Minute

type mediaGapScanState string

const (
	mediaGapScanStateIdle      mediaGapScanState = "idle"
	mediaGapScanStateRunning   mediaGapScanState = "running"
	mediaGapScanStateSucceeded mediaGapScanState = "succeeded"
	mediaGapScanStateFailed    mediaGapScanState = "failed"
	mediaGapScanStatePartial   mediaGapScanState = "partial"
)

type mediaGapAsyncScanStatus struct {
	Failures         []mediagappkg.ScanFailure `json:"failures,omitempty"`
	ScanID           string                    `json:"scanId,omitempty"`
	Scope            string                    `json:"scope,omitempty"`
	Status           mediaGapScanState         `json:"status"`
	Running          bool                      `json:"running"`
	StartedAt        *time.Time                `json:"startedAt,omitempty"`
	FinishedAt       *time.Time                `json:"finishedAt,omitempty"`
	Count            int                       `json:"count"`
	ScannedSeries    int                       `json:"scannedSeries"`
	SkippedSeries    int                       `json:"skippedSeries"`
	ExaminedEpisodes int                       `json:"examinedEpisodes"`
	Created          int                       `json:"created"`
	Updated          int                       `json:"updated"`
	Ingested         int                       `json:"ingested"`
	Error            string                    `json:"error,omitempty"`
	Message          string                    `json:"message,omitempty"`
}

// scanLockRecorder 抽象 advisory lock + media_gap_scans 持久化记录的依赖，
// 便于单测注入 mock，避免在 handlers 单测里依赖真实 DB。
type scanLockRecorder interface {
	AcquireAndRecord(ctx context.Context) (*mediagappkg.MediaGapScanLockHandleHolder, string, error)
	FinishAndReleaseHolder(ctx context.Context, holder *mediagappkg.MediaGapScanLockHandleHolder, scanID string, status models.MediaGapScanStatus, errMsg string)
}

type mediaGapScanManager struct {
	mu       sync.RWMutex
	status   mediaGapAsyncScanStatus
	scanFn   func(context.Context, mediagappkg.ScanRequest) (*mediagappkg.ScanResult, error)
	recorder scanLockRecorder
}

func newMediaGapScanManager(scanFn func(context.Context, mediagappkg.ScanRequest) (*mediagappkg.ScanResult, error)) *mediaGapScanManager {
	return newMediaGapScanManagerWithRecorder(scanFn, mediagappkg.NewMediaGapScanRecorder())
}

// newMediaGapScanManagerWithRecorder 用于注入自定义 recorder。生产路径使用上面的工厂；
// 测试在不连接真实 DB 的场景下用本函数注入内存版 recorder。
func newMediaGapScanManagerWithRecorder(scanFn func(context.Context, mediagappkg.ScanRequest) (*mediagappkg.ScanResult, error), recorder scanLockRecorder) *mediaGapScanManager {
	return &mediaGapScanManager{
		status: mediaGapAsyncScanStatus{
			Status:  mediaGapScanStateIdle,
			Message: "暂无扫描任务",
		},
		scanFn:   scanFn,
		recorder: recorder,
	}
}

// Start 申请跨副本扫描锁；锁被其他节点持有时返回 (status, false)，调用方映射为 409。
//
// 锁绑定在持有连接的 PG session 上，进程 crash 后由 PG 端回收，不会永久卡死。
func (m *mediaGapScanManager) Start(req mediagappkg.ScanRequest) (mediaGapAsyncScanStatus, bool) {
	ctx := context.Background()
	handle, scanID, err := m.recorder.AcquireAndRecord(ctx)
	if err != nil {
		m.mu.RLock()
		current := m.status
		m.mu.RUnlock()
		if errors.Is(err, mediagappkg.ErrMediaGapScanInProgress) {
			current.Message = "另一节点正在执行缺集扫描，请稍后查看状态"
			return current, false
		}
		current.Message = "缺集扫描启动失败：" + err.Error()
		current.Status = mediaGapScanStateFailed
		current.Error = err.Error()
		return current, false
	}

	m.mu.Lock()
	now := time.Now().UTC()
	scope := "all"
	if strings.TrimSpace(req.TMDBID) != "" {
		scope = "series"
	}
	m.status = mediaGapAsyncScanStatus{
		ScanID:    scanID,
		Scope:     scope,
		Status:    mediaGapScanStateRunning,
		Running:   true,
		StartedAt: cloneTimePointer(now),
		Message: func() string {
			if scope == "series" {
				return "单剧缺集扫描已启动，后台处理中"
			}
			return "全库缺集扫描已启动，后台处理中"
		}(),
	}
	startedStatus := m.status
	m.mu.Unlock()

	async.SafeGo("mediagap.scan", func() {
		m.run(scanID, req, handle)
	})
	return startedStatus, true
}

func (m *mediaGapScanManager) Status() mediaGapAsyncScanStatus {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.status
}

// run 无论正常返回、取消还是 panic，均使用独立上下文写终态并释放扫描锁。
func (m *mediaGapScanManager) run(scanID string, req mediagappkg.ScanRequest, handle *mediagappkg.MediaGapScanLockHandleHolder) {
	ctx, cancel := context.WithTimeout(context.Background(), mediaGapAsyncScanTimeout)
	defer cancel()
	var result *mediagappkg.ScanResult
	var err error
	defer func() {
		if recovered := recover(); recovered != nil {
			log.Printf("[MediaGap] 扫描异常 scanId=%s panicType=%T", scanID, recovered)
			err = errors.New("扫描异常中止，请重试")
		}
		m.finish(scanID, handle, result, err)
	}()
	result, err = m.scanFn(ctx, req)
	if err == nil && ctx.Err() != nil {
		err = errors.New("扫描已超时或取消")
	}
}

// finish 将不完整扫描与成功区分；持久化仍使用 failed 并保存摘要，避免伪成功审计。
func (m *mediaGapScanManager) finish(scanID string, handle *mediagappkg.MediaGapScanLockHandleHolder, result *mediagappkg.ScanResult, err error) {
	finishedAt := time.Now().UTC()
	status := mediaGapScanStateSucceeded
	message := "缺集扫描完成"
	errMsg := ""
	if result != nil && result.SkippedSeries > 0 {
		status = mediaGapScanStatePartial
		message = fmt.Sprintf("扫描部分完成：成功 %d 部，失败 %d 部", result.ScannedSeries, result.SkippedSeries)
		if result.ScannedSeries == 0 {
			status = mediaGapScanStateFailed
			message = "扫描失败：所有剧集均未完成"
		}
		errMsg = message
	}
	if err != nil {
		status = mediaGapScanStateFailed
		message = "缺集扫描失败"
		errMsg = err.Error()
	}
	finalStatus := models.MediaGapScanStatusSuccess
	if status != mediaGapScanStateSucceeded {
		finalStatus = models.MediaGapScanStatusFailed
	}
	finalizeCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	m.recorder.FinishAndReleaseHolder(finalizeCtx, handle, scanID, finalStatus, errMsg)
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.status.ScanID != scanID {
		return
	}
	m.status.Running = false
	m.status.FinishedAt = &finishedAt
	m.status.Status = status
	m.status.Message = message
	m.status.Error = errMsg
	if result != nil {
		m.status.Count = result.Created + result.Updated + result.Ingested
		m.status.ScannedSeries = result.ScannedSeries
		m.status.SkippedSeries = result.SkippedSeries
		m.status.ExaminedEpisodes = result.ExaminedEpisodes
		m.status.Created = result.Created
		m.status.Updated = result.Updated
		m.status.Ingested = result.Ingested
		m.status.Failures = result.Failures
	}
}

func cloneTimePointer(value time.Time) *time.Time {
	cloned := value
	return &cloned
}
